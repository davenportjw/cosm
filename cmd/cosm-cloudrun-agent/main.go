package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cosmscm/cosm/pkg/topocosm"
	"github.com/cosmscm/cosm/test/agents/framework"
	"github.com/cosmscm/cosm/test/agents/llm"
	"github.com/cosmscm/cosm/test/agents/rater"
	"github.com/cosmscm/cosm/test/agents/testagent"
)

// CloudLogEntry represents a Google Cloud Logging structured payload.
type CloudLogEntry struct {
	Severity  string `json:"severity"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
	Trace     string `json:"logging.googleapis.com/trace,omitempty"`
	SpanID    string `json:"logging.googleapis.com/spanId,omitempty"`
	AgentRole string `json:"agent_role,omitempty"`
	TaskIndex int    `json:"task_index"`
}

func logStructured(severity, msg, traceID, role string, taskIdx int) {
	entry := CloudLogEntry{
		Severity:  severity,
		Message:   msg,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Trace:     traceID,
		AgentRole: role,
		TaskIndex: taskIdx,
	}
	data, _ := json.Marshal(entry)
	fmt.Println(string(data))
}

func main() {
	var (
		roleFlag        = flag.String("role", "", "Agent role (bootstrap, backend, frontend, infra, contender, judge)")
		hubURLFlag      = flag.String("hub-url", "", "Topocosm Hub URL (env: TOPOCOSM_HUB_URL)")
		repoFlag        = flag.String("repo", "cosm/fintech-mesh", "Cosm repository (env: COSM_REPO)")
		workDirFlag     = flag.String("workdir", "", "Working directory for agent operations")
		sessionsDirFlag = flag.String("sessions-dir", "", "Directory to read/write agent session telemetry (env: COSM_SESSIONS_DIR, default: /tmp/cosm-sessions)")
		modelFlag       = flag.String("model", "gemini-3.8-flash", "LLM model (env: GEMINI_MODEL)")
		mockFlag        = flag.Bool("mock", false, "Use deterministic mock provider")
		traceIDFlag     = flag.String("trace-id", "", "W3C Trace ID (env: COSM_TRACE_ID)")
		sessionFlag     = flag.String("session-id", "", "Agent Session ID (env: COSM_SESSION_ID)")
		taskIdxFlag     = flag.Int("task-index", -1, "Cloud Run task index (env: CLOUD_RUN_TASK_INDEX)")
		tokenFlag       = flag.String("token", "", "Topocosm Personal Access Token (env: TOPOCOSM_TOKEN, COSM_AUTH_TOKEN)")
		taskCountFlag   = flag.Int("tasks", 100, "Total number of swarm tasks (default: 100)")
	)
	flag.Parse()

	// Environment variable fallback
	role := *roleFlag
	if role == "" {
		role = os.Getenv("AGENT_ROLE")
	}

	taskIndex := *taskIdxFlag
	if taskIndex < 0 {
		if idxStr := os.Getenv("CLOUD_RUN_TASK_INDEX"); idxStr != "" {
			if parsed, err := strconv.Atoi(idxStr); err == nil {
				taskIndex = parsed
			}
		}
	}
	if taskIndex < 0 {
		taskIndex = 0
	}

	// Auto-select role based on Cloud Run Task Index if unspecified
	if role == "" {
		switch {
		case taskIndex <= 0:
			role = "bootstrap"
		case taskIndex >= 1 && taskIndex <= 20:
			role = "backend"
		case taskIndex >= 21 && taskIndex <= 40:
			role = "frontend"
		case taskIndex >= 41 && taskIndex <= 60:
			role = "infra"
		case taskIndex >= 61 && taskIndex <= 80:
			role = "contender"
		case taskIndex >= 81 && taskIndex <= 99:
			role = "observer"
		default:
			role = "bootstrap"
		}
	}

	hubURL := *hubURLFlag
	if hubURL == "" {
		hubURL = os.Getenv("TOPOCOSM_HUB_URL")
	}
	if hubURL == "" {
		hubURL = "https://topocosm-hub-default.run.app"
	}
	os.Setenv("TOPOCOSM_HUB_URL", hubURL)

	repo := *repoFlag
	if envRepo := os.Getenv("COSM_REPO"); envRepo != "" {
		repo = envRepo
	}

	traceID := *traceIDFlag
	if traceID == "" {
		traceID = os.Getenv("COSM_TRACE_ID")
	}
	if traceID == "" {
		traceID = fmt.Sprintf("trace-%d", time.Now().UnixNano())
	}
	os.Setenv("COSM_TRACE_ID", traceID)

	sessionID := *sessionFlag
	if sessionID == "" {
		sessionID = os.Getenv("COSM_SESSION_ID")
	}
	if sessionID == "" {
		sessionID = fmt.Sprintf("sess-%s-%d", role, time.Now().UnixNano())
	}
	os.Setenv("COSM_SESSION_ID", sessionID)

	token := *tokenFlag
	if token == "" {
		token = os.Getenv("TOPOCOSM_TOKEN")
	}
	if token == "" {
		token = os.Getenv("COSM_AUTH_TOKEN")
	}
	if token == "" {
		token = os.Getenv("TOPOCOSM_PAT")
	}
	if token != "" {
		os.Setenv("COSM_AUTH_TOKEN", token)
		os.Setenv("TOPOCOSM_TOKEN", token)
	}

	agentDID := fmt.Sprintf("did:key:z6MkuAgent%s%d", strings.Title(role), taskIndex)
	os.Setenv("COSM_AGENT_DID", agentDID)

	workDir := *workDirFlag
	if workDir == "" {
		workDir = os.Getenv("COSM_WORKDIR")
	}
	if workDir == "" {
		temp, err := os.MkdirTemp("", fmt.Sprintf("cosm-agent-%s-%d-*", role, taskIndex))
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed creating workdir: %v\n", err)
			os.Exit(1)
		}
		workDir = temp
	}

	sessionsDir := *sessionsDirFlag
	if sessionsDir == "" {
		sessionsDir = os.Getenv("COSM_SESSIONS_DIR")
	}
	if sessionsDir == "" {
		sessionsDir = "/tmp/cosm-sessions"
	}
	os.Setenv("COSM_SESSIONS_DIR", sessionsDir)
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		logStructured("ERROR", fmt.Sprintf("failed creating sessions directory %s: %v", sessionsDir, err), traceID, role, taskIndex)
		fmt.Fprintf(os.Stderr, "failed creating sessions directory %s: %v\n", sessionsDir, err)
		os.Exit(1)
	}

	logStructured("INFO", fmt.Sprintf("Starting Cloud Run Agent role=%s did=%s task_index=%d hub=%s repo=%s sessions_dir=%s has_token=%t", role, agentDID, taskIndex, hubURL, repo, sessionsDir, token != ""), traceID, role, taskIndex)

	// Determine LLM provider
	var provider llm.LLMProvider
	apiKey := os.Getenv("GEMINI_API_KEY")
	if *mockFlag {
		logStructured("INFO", "Using hermetic deterministic mock LLM provider", traceID, role, taskIndex)
		mock := llm.NewMockProvider(*modelFlag)
		setupMockPlan(mock, role, repo, hubURL, taskIndex)
		provider = mock
	} else {
		logStructured("INFO", fmt.Sprintf("Using live Gemini API model=%s via Google Cloud Project Auth (Vertex AI us multi-region)", *modelFlag), traceID, role, taskIndex)
		var err error
		provider, err = llm.NewGeminiProvider(llm.GeminiConfig{
			APIKey:    apiKey,
			Model:     *modelFlag,
			ProjectID: "davenport-boutique",
			Location:  "us",
		})
		if err != nil {
			logStructured("ERROR", fmt.Sprintf("failed initializing Gemini provider: %v", err), traceID, role, taskIndex)
			os.Exit(1)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	agent := testagent.NewTestAgent(fmt.Sprintf("agent-%s-%d", role, taskIndex), provider, workDir, *modelFlag)

	// Auto-enroll agent DID with Topocosm Hub (if not standalone judge)
	if role != "judge" {
		initClient := topocosm.NewHubClient(hubURL, agentDID)
		if err := initClient.Enroll(ctx, agentDID, fmt.Sprintf("agent-%s-%d", role, taskIndex), "", fmt.Sprintf("Autonomous Agent (%s #%d)", role, taskIndex), "", true); err != nil {
			logStructured("WARN", fmt.Sprintf("Agent enroll attempt returned: %v (continuing)", err), traceID, role, taskIndex)
		} else {
			logStructured("INFO", fmt.Sprintf("Agent enrolled with Topocosm Hub did=%s", agentDID), traceID, role, taskIndex)
		}
	}

	var (
		session *framework.AgentSession
		err     error
	)

	switch role {
	case "bootstrap":
		if err := testagent.WriteScenarioFiles(workDir, &testagent.ScenarioCloudRunMesh); err != nil {
			logStructured("ERROR", fmt.Sprintf("failed writing scenario files: %v", err), traceID, role, taskIndex)
			os.Exit(1)
		}
		session, err = agent.RunPrompt(ctx, "Initialize cosm repository, stage polyglot components, commit baseline, and publish to Topocosm Hub.", framework.LoopOptions{MaxSteps: 10})
	case "backend":
		session, err = agent.RunPrompt(ctx, fmt.Sprintf("Claim services/orders-%d, clone universe-main, create branch u/agent-backend-%d, edit Checkout method, commit, publish, open proposal, and release claim.", taskIndex%4, taskIndex), framework.LoopOptions{MaxSteps: 12})
	case "frontend":
		session, err = agent.RunPrompt(ctx, fmt.Sprintf("Sparse pull apps/checkout from Topocosm Hub, branch to u/agent-frontend-%d, commit, publish, and open stacked proposal.", taskIndex), framework.LoopOptions{MaxSteps: 10})
	case "infra":
		session, err = agent.RunPrompt(ctx, fmt.Sprintf("Sparse pull infra/cloudrun, branch to u/agent-infra-%d, format and validate Terraform, commit, publish, and open stacked proposal.", taskIndex), framework.LoopOptions{MaxSteps: 10})
	case "contender":
		session, err = agent.RunPrompt(ctx, fmt.Sprintf("Validate concurrency domain leases on services/orders-%d and micro-universe isolation under contention.", taskIndex%4), framework.LoopOptions{MaxSteps: 8})
	case "observer":
		session, err = agent.RunPrompt(ctx, "Query proposals and inspect Merkle DAG topology across the mesh.", framework.LoopOptions{MaxSteps: 6})
	case "judge":
		runJudgeStandalone(ctx, *modelFlag, apiKey, *mockFlag, traceID, *taskCountFlag, sessionsDir)
		return
	default:
		logStructured("ERROR", fmt.Sprintf("Unknown role: %s", role), traceID, role, taskIndex)
		os.Exit(1)
	}

	if err != nil || session == nil {
		logStructured("ERROR", fmt.Sprintf("Agent run failed: %v", err), traceID, role, taskIndex)
		os.Exit(1)
	}

	if session.Status != framework.StatusSuccess {
		logStructured("ERROR", fmt.Sprintf("Agent finished with non-success status: %s", session.Status), traceID, role, taskIndex)
		os.Exit(1)
	}

	logStructured("INFO", fmt.Sprintf("Agent completed successfully. TotalSteps=%d Tokens=%d Duration=%v",
		session.TotalSteps, session.TotalTokens.TotalTokens, session.Duration), traceID, role, taskIndex)

	// Persist authentic session telemetry for judge evaluation
	sessionFile := filepath.Join(sessionsDir, fmt.Sprintf("session-task-%d.json", taskIndex))
	sessData, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		logStructured("ERROR", fmt.Sprintf("failed serializing session JSON: %v", err), traceID, role, taskIndex)
		os.Exit(1)
	}
	if err := os.WriteFile(sessionFile, sessData, 0644); err != nil {
		logStructured("ERROR", fmt.Sprintf("failed writing session file %s: %v", sessionFile, err), traceID, role, taskIndex)
		os.Exit(1)
	}
	logStructured("INFO", fmt.Sprintf("Persisted authentic session telemetry to %s", sessionFile), traceID, role, taskIndex)
}

func setupMockPlan(mock *llm.MockProvider, role, repo, hubURL string, taskIndex int) {
	domainIdx := taskIndex % 4
	domain := fmt.Sprintf("services/orders-%d", domainIdx)
	switch role {
	case "bootstrap":
		mock.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
		mock.EnqueueToolCall("cosm_add", `{"files":["services/orders/main.go","apps/checkout/CheckoutForm.tsx","workers/fraud/analyzer.py","infra/cloudrun/main.tf"]}`)
		mock.EnqueueToolCall("cosm_commit", `{"universe_id":"universe-main","intent":"Initial Merkle root commit"}`)
		mock.EnqueueToolCall("cosm_publish", fmt.Sprintf(`{"repo":%q,"universe_id":"universe-main"}`, repo))
		mock.EnqueueText("Bootstrap completed and published to Topocosm Hub.")
	case "backend":
		uID := fmt.Sprintf("u/agent-backend-%d", taskIndex)
		mock.EnqueueToolCall("cosm_claim", fmt.Sprintf(`{"repo":%q,"domain":%q,"purpose":"V2 Checkout Batch %d"}`, repo, domain, taskIndex))
		mock.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
		mock.EnqueueToolCall("cosm_clone", fmt.Sprintf(`{"repo":%q,"universe_id":"universe-main"}`, repo))
		mock.EnqueueToolCall("cosm_universe_create", fmt.Sprintf(`{"universe_id":%q,"parent_id":"universe-main"}`, uID))
		mock.EnqueueToolCall("cosm_commit", fmt.Sprintf(`{"universe_id":%q,"intent":"V2 checkout idempotency batch %d"}`, uID, taskIndex))
		mock.EnqueueToolCall("cosm_publish", fmt.Sprintf(`{"repo":%q,"universe_id":%q}`, repo, uID))
		mock.EnqueueToolCall("cosm_stack_create", fmt.Sprintf(`{"repo":%q,"title":"Orders V2 Checkout Batch %d","source_universe":%q,"target_universe":"universe-main"}`, repo, taskIndex, uID))
		mock.EnqueueToolCall("cosm_claim", fmt.Sprintf(`{"repo":%q,"domain":%q,"release":true}`, repo, domain))
		mock.EnqueueText("Backend orders API changes committed, published, and proposal opened.")
	case "frontend":
		uID := fmt.Sprintf("u/agent-frontend-%d", taskIndex)
		mock.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
		mock.EnqueueToolCall("cosm_clone", fmt.Sprintf(`{"repo":%q,"universe_id":"universe-main","components":["apps/checkout"]}`, repo))
		mock.EnqueueToolCall("cosm_universe_create", fmt.Sprintf(`{"universe_id":%q,"parent_id":"universe-main"}`, uID))
		mock.EnqueueToolCall("cosm_commit", fmt.Sprintf(`{"universe_id":%q,"intent":"Update CheckoutForm %d"}`, uID, taskIndex))
		mock.EnqueueToolCall("cosm_publish", fmt.Sprintf(`{"repo":%q,"universe_id":%q}`, repo, uID))
		mock.EnqueueToolCall("cosm_stack_create", fmt.Sprintf(`{"repo":%q,"title":"Frontend V2 Integration %d","source_universe":%q,"target_universe":"universe-main"}`, repo, taskIndex, uID))
		mock.EnqueueText("Frontend changes committed, published, and proposal stacked.")
	case "infra":
		uID := fmt.Sprintf("u/agent-infra-%d", taskIndex)
		mock.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
		mock.EnqueueToolCall("cosm_clone", fmt.Sprintf(`{"repo":%q,"universe_id":"universe-main","components":["infra/cloudrun"]}`, repo))
		mock.EnqueueToolCall("cosm_universe_create", fmt.Sprintf(`{"universe_id":%q,"parent_id":"universe-main"}`, uID))
		mock.EnqueueToolCall("cosm_commit", fmt.Sprintf(`{"universe_id":%q,"intent":"Terraform DLQ bindings %d"}`, uID, taskIndex))
		mock.EnqueueToolCall("cosm_publish", fmt.Sprintf(`{"repo":%q,"universe_id":%q}`, repo, uID))
		mock.EnqueueToolCall("cosm_stack_create", fmt.Sprintf(`{"repo":%q,"title":"Terraform DLQ Proposal %d","source_universe":%q,"target_universe":"universe-main"}`, repo, taskIndex, uID))
		mock.EnqueueText("Infrastructure changes validated, committed, and proposal opened.")
	case "contender":
		uID := fmt.Sprintf("u/agent-contender-%d", taskIndex)
		mock.EnqueueToolCall("cosm_claim", fmt.Sprintf(`{"repo":%q,"domain":%q,"purpose":"Contender lease stress %d"}`, repo, domain, taskIndex))
		mock.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
		mock.EnqueueToolCall("cosm_universe_create", fmt.Sprintf(`{"universe_id":%q,"parent_id":"universe-main"}`, uID))
		mock.EnqueueToolCall("cosm_claim", fmt.Sprintf(`{"repo":%q,"domain":%q,"release":true}`, repo, domain))
		mock.EnqueueText("Contender test validated.")
	case "observer":
		mock.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
		mock.EnqueueToolCall("cosm_clone", fmt.Sprintf(`{"repo":%q,"universe_id":"universe-main"}`, repo))
		mock.EnqueueText("Observer topology inspection complete.")
	}
}

func runJudgeStandalone(ctx context.Context, model, apiKey string, mock bool, traceID string, totalTasks int, sessionsDir string) {
	logStructured("INFO", fmt.Sprintf("Running Gemini 3.8 Flash Rater Judge evaluation for %d tasks", totalTasks), traceID, "judge", 100)
	var judgeProvider llm.LLMProvider
	if mock {
		mockP := llm.NewMockProvider(rater.DefaultJudgeModel)
		mockP.EnqueueText("VERDICT: PASSED\nCONFIDENCE: HIGH\nQUALITY: 25 / 25\nREASONING: High quality AST execution across all swarm workers.")
		judgeProvider = mockP
	} else {
		logStructured("INFO", "Using live Gemini 3.8 Flash Rater Judge via Google Cloud Project Auth (Vertex AI us multi-region)", traceID, "judge", 100)
		var err error
		judgeProvider, err = llm.NewGeminiProvider(llm.GeminiConfig{
			APIKey:    apiKey,
			Model:     rater.DefaultJudgeModel,
			ProjectID: "davenport-boutique",
			Location:  "us",
		})
		if err != nil {
			logStructured("ERROR", fmt.Sprintf("failed initializing Gemini judge provider: %v", err), traceID, "judge", 100)
			os.Exit(1)
		}
	}

	var sessions []*framework.AgentSession
	if mock {
		logStructured("INFO", "Running Deterministic Concurrency Benchmark Replay (MOCK MODE enabled for SCM backplane stress testing)", traceID, "judge", 100)
		loaded, err := loadWorkerSessions(sessionsDir)
		if err == nil && len(loaded) > 0 {
			sessions = loaded
			logStructured("INFO", fmt.Sprintf("Loaded %d worker sessions from %s for mock evaluation", len(sessions), sessionsDir), traceID, "judge", 100)
		} else {
			sessions = replayMockBenchmarkSessions(totalTasks)
		}
	} else {
		loaded, err := loadWorkerSessions(sessionsDir)
		if err != nil {
			logStructured("ERROR", fmt.Sprintf("error reading worker sessions from %s: %v", sessionsDir, err), traceID, "judge", 100)
		}
		if len(loaded) == 0 {
			errMsg := fmt.Sprintf("no genuine worker sessions found in %s: live judge requires authentic execution sessions from completed workers (STRICT NEVER MOCK DIRECTIVE)", sessionsDir)
			logStructured("ERROR", errMsg, traceID, "judge", 100)
			fmt.Fprintf(os.Stderr, "FATAL: %s\n", errMsg)
			os.Exit(1)
		}
		sessions = loaded
		logStructured("INFO", fmt.Sprintf("Loaded %d authentic worker sessions from %s for evaluation", len(sessions), sessionsDir), traceID, "judge", 100)
	}

	judge := rater.NewRaterJudge(judgeProvider, rater.DefaultJudgeModel)
	evalReq := &rater.JudgeEvaluationRequest{
		ScenarioName: "cloudrun-polyglot-mesh-100",
		Sessions:     sessions,
	}
	report, err := judge.Evaluate(ctx, evalReq)
	if err != nil {
		logStructured("ERROR", fmt.Sprintf("Judge evaluation failed: %v", err), traceID, "judge", 100)
		os.Exit(1)
	}

	data, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(data))
}

func loadWorkerSessions(sessionsDir string) ([]*framework.AgentSession, error) {
	pattern := filepath.Join(sessionsDir, "session-task-*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob error in %s: %w", sessionsDir, err)
	}
	if len(files) == 0 {
		return nil, nil
	}

	sort.Slice(files, func(i, j int) bool {
		var idxI, idxJ int
		fmt.Sscanf(filepath.Base(files[i]), "session-task-%d.json", &idxI)
		fmt.Sscanf(filepath.Base(files[j]), "session-task-%d.json", &idxJ)
		return idxI < idxJ
	})

	sessions := make([]*framework.AgentSession, 0, len(files))
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("reading session file %s: %w", file, err)
		}
		var sess framework.AgentSession
		if err := json.Unmarshal(data, &sess); err != nil {
			return nil, fmt.Errorf("unmarshaling session file %s: %w", file, err)
		}
		sessions = append(sessions, &sess)
	}
	return sessions, nil
}

func replayMockBenchmarkSessions(totalTasks int) []*framework.AgentSession {
	if totalTasks <= 0 {
		totalTasks = 100
	}
	sessions := make([]*framework.AgentSession, 0, totalTasks)
	for taskIdx := 0; taskIdx < totalTasks; taskIdx++ {
		var role string
		switch {
		case taskIdx == 0:
			role = "bootstrap"
		case taskIdx >= 1 && taskIdx <= 20:
			role = "backend"
		case taskIdx >= 21 && taskIdx <= 40:
			role = "frontend"
		case taskIdx >= 41 && taskIdx <= 60:
			role = "infra"
		case taskIdx >= 61 && taskIdx <= 80:
			role = "contender"
		case taskIdx >= 81 && taskIdx <= 99:
			role = "observer"
		default:
			role = "bootstrap"
		}

		sess := &framework.AgentSession{
			SessionID:    fmt.Sprintf("cloudrun-session-%s-%d", role, taskIdx),
			AgentID:      fmt.Sprintf("agent-%s-%d", role, taskIdx),
			ScenarioName: "cloudrun-polyglot-mesh-100",
			Status:       framework.StatusSuccess,
			TotalTokens:  framework.TokenUsage{PromptTokens: 250, CompletionTokens: 100, TotalTokens: 350},
			Duration:     time.Duration(100+taskIdx*15) * time.Millisecond,
		}
		var calls []llm.ToolCall
		switch role {
		case "bootstrap":
			calls = []llm.ToolCall{
				{Name: "cosm_init"},
				{Name: "cosm_add"},
				{Name: "cosm_commit"},
				{Name: "cosm_publish"},
			}
		case "backend":
			calls = []llm.ToolCall{
				{Name: "cosm_claim", Arguments: fmt.Sprintf(`{"domain":"services/orders-%d"}`, taskIdx%4)},
				{Name: "cosm_init"},
				{Name: "cosm_clone"},
				{Name: "cosm_universe_create"},
				{Name: "cosm_commit"},
				{Name: "cosm_publish"},
				{Name: "cosm_stack_create"},
				{Name: "cosm_claim", Arguments: fmt.Sprintf(`{"domain":"services/orders-%d","release":true}`, taskIdx%4)},
			}
		case "frontend":
			calls = []llm.ToolCall{
				{Name: "cosm_init"},
				{Name: "cosm_clone", Arguments: `{"components":["apps/checkout"]}`},
				{Name: "cosm_universe_create"},
				{Name: "cosm_commit"},
				{Name: "cosm_publish"},
				{Name: "cosm_stack_create"},
			}
		case "infra":
			calls = []llm.ToolCall{
				{Name: "cosm_init"},
				{Name: "cosm_clone", Arguments: `{"components":["infra/cloudrun"]}`},
				{Name: "cosm_universe_create"},
				{Name: "cosm_commit", Arguments: `{"intent":"Terraform DLQ bindings"}`},
				{Name: "cosm_publish"},
				{Name: "cosm_stack_create", Arguments: `{"title":"Terraform DLQ Proposal"}`},
			}
		case "contender":
			calls = []llm.ToolCall{
				{Name: "cosm_claim", Arguments: fmt.Sprintf(`{"domain":"services/orders-%d","purpose":"Contender edit attempt"}`, taskIdx%4)},
				{Name: "cosm_init"},
				{Name: "cosm_universe_create"},
				{Name: "cosm_claim", Arguments: fmt.Sprintf(`{"domain":"services/orders-%d","release":true}`, taskIdx%4)},
			}
		case "observer":
			calls = []llm.ToolCall{
				{Name: "cosm_init"},
				{Name: "cosm_clone"},
			}
		}
		sess.TotalSteps = len(calls) + 1
		sess.Traces = []framework.StepTrace{
			{
				StepIndex: 1,
				ToolCalls: calls,
			},
		}
		sessions = append(sessions, sess)
	}
	return sessions
}
