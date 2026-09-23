package benchmarks

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TranscriptStep represents a parsed record from Antigravity's transcript JSONL format.
type TranscriptStep struct {
	StepIndex       int               `json:"step_index"`
	Source          string            `json:"source"`
	Type            string            `json:"type"`
	Status          string            `json:"status"`
	Content         string            `json:"content"`
	Thinking        string            `json:"thinking"`
	ToolCalls       []ToolCallRecord  `json:"tool_calls"`
	TruncatedFields []string          `json:"truncated_fields"`
}

type ToolCallRecord struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// SessionTelemetry encapsulates empirical token and character telemetry from a subagent transcript.
type SessionTelemetry struct {
	TotalSteps            int
	LLMCalls              int
	CumulativePromptChars int
	CumulativeGenChars    int
	FinalContextChars     int
	ToolOutputChars       int
	EstimatedPromptTokens int
	EstimatedGenTokens    int
	EstimatedTotalTokens  int
	AvgPromptCharsPerTurn float64
	AvgPromptTokensPerTurn float64
}

// parseTranscriptFile parses an Antigravity transcript JSONL file.
func parseTranscriptFile(path string) (*SessionTelemetry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var steps []TranscriptStep
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var step TranscriptStep
		if err := json.Unmarshal(line, &step); err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	tc := NewTokenCounter()
	var (
		llmCalls              int
		cumulativePromptChars int
		cumulativeGenChars    int
		toolOutputChars       int
		historyChars          int
	)

	for _, step := range steps {
		stepChars := len(step.Content)
		switch step.Type {
		case "USER_INPUT":
			historyChars += stepChars
		case "GENERIC": // Tool response
			historyChars += stepChars
			toolOutputChars += stepChars
		case "PLANNER_RESPONSE":
			llmCalls++
			promptForCall := historyChars
			cumulativePromptChars += promptForCall

			var argsLen int
			for _, tc := range step.ToolCalls {
				argsLen += len(tc.Args)
			}
			genThisTurn := len(step.Thinking) + argsLen + len(step.Content)
			cumulativeGenChars += genThisTurn

			historyChars += len(step.Content) + argsLen
		}
	}

	promptTokens := tc.EstimateTokens(string(make([]byte, cumulativePromptChars)))
	// Fallback to calibrated 3.6 chars/token ratio for code/prose mix
	if promptTokens == 0 && cumulativePromptChars > 0 {
		promptTokens = int(float64(cumulativePromptChars) / 3.6)
	}
	genTokens := int(float64(cumulativeGenChars) / 3.6)
	finalTokens := int(float64(historyChars) / 3.6)
	_ = finalTokens

	avgPromptChars := 0.0
	avgPromptTokens := 0.0
	if llmCalls > 0 {
		avgPromptChars = float64(cumulativePromptChars) / float64(llmCalls)
		avgPromptTokens = float64(promptTokens) / float64(llmCalls)
	}

	return &SessionTelemetry{
		TotalSteps:            len(steps),
		LLMCalls:              llmCalls,
		CumulativePromptChars: cumulativePromptChars,
		CumulativeGenChars:    cumulativeGenChars,
		FinalContextChars:     historyChars,
		ToolOutputChars:       toolOutputChars,
		EstimatedPromptTokens: promptTokens,
		EstimatedGenTokens:    genTokens,
		EstimatedTotalTokens:  promptTokens + genTokens,
		AvgPromptCharsPerTurn: avgPromptChars,
		AvgPromptTokensPerTurn: avgPromptTokens,
	}, nil
}

// TestEmpiricalSubagentTranscripts_FileVsAST audits real Antigravity subagent transcripts
// recorded during live autonomous coding runs on examples/camping_app/cmd/server/main.go (1,730 lines).
func TestEmpiricalSubagentTranscripts_FileVsAST(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatalf("Failed to locate repo root: %v", err)
	}

	fileTranscript := filepath.Join(root, "test", "agents", "transcripts", "subagent_a_file_full.jsonl")
	astTranscript := filepath.Join(root, "test", "agents", "transcripts", "subagent_b_ast_full.jsonl")

	if _, err := os.Stat(fileTranscript); os.IsNotExist(err) {
		t.Skipf("Subagent transcripts not found at %s; skipping empirical test", fileTranscript)
	}

	fileTel, err := parseTranscriptFile(fileTranscript)
	if err != nil {
		t.Fatalf("Failed to parse file-based subagent transcript: %v", err)
	}

	astTel, err := parseTranscriptFile(astTranscript)
	if err != nil {
		t.Fatalf("Failed to parse AST surgeon subagent transcript: %v", err)
	}

	t.Logf("=== EMPIRICAL SUBAGENT TELEMETRY COMPARISON ===")
	t.Logf("File-Based Agent: %d steps, %d LLM turns, %d tool output chars, %d final context chars",
		fileTel.TotalSteps, fileTel.LLMCalls, fileTel.ToolOutputChars, fileTel.FinalContextChars)
	t.Logf("Cosm AST Surgeon: %d steps, %d LLM turns, %d tool output chars, %d final context chars",
		astTel.TotalSteps, astTel.LLMCalls, astTel.ToolOutputChars, astTel.FinalContextChars)

	// Invariant 1: Tool Output Payload Reduction (AST avoids dumping 1,730-line file into context)
	toolPayloadReduction := float64(fileTel.ToolOutputChars-astTel.ToolOutputChars) / float64(fileTel.ToolOutputChars) * 100.0
	t.Logf("Tool Output Payload Reduction: %.2f%% (File: %d chars vs. AST: %d chars)",
		toolPayloadReduction, fileTel.ToolOutputChars, astTel.ToolOutputChars)
	if toolPayloadReduction < 50.0 {
		t.Errorf("Expected at least 50%% reduction in tool output payload, got %.2f%%", toolPayloadReduction)
	}

	// Invariant 2: Per-Turn Context Window Footprint
	avgPromptReduction := (fileTel.AvgPromptCharsPerTurn - astTel.AvgPromptCharsPerTurn) / fileTel.AvgPromptCharsPerTurn * 100.0
	t.Logf("Average Per-Turn Context Reduction: %.2f%% (File: %.0f chars vs. AST: %.0f chars)",
		avgPromptReduction, fileTel.AvgPromptCharsPerTurn, astTel.AvgPromptCharsPerTurn)
	if avgPromptReduction < 45.0 {
		t.Errorf("Expected at least 45%% reduction in average per-turn context, got %.2f%%", avgPromptReduction)
	}

	// Invariant 3: Final Context Window Size at Completion
	finalContextReduction := float64(fileTel.FinalContextChars-astTel.FinalContextChars) / float64(fileTel.FinalContextChars) * 100.0
	t.Logf("Final Context Window Reduction: %.2f%% (File: %d chars vs. AST: %d chars)",
		finalContextReduction, fileTel.FinalContextChars, astTel.FinalContextChars)
	if finalContextReduction < 40.0 {
		t.Errorf("Expected at least 40%% reduction in final context window, got %.2f%%", finalContextReduction)
	}

	// Invariant 4: Total Cumulative Billed Tokens (even with more AST turns, total billed tokens is lower)
	if astTel.CumulativePromptChars >= fileTel.CumulativePromptChars {
		t.Errorf("Expected Cosm AST cumulative prompt chars (%d) to be lower than file-based (%d)",
			astTel.CumulativePromptChars, fileTel.CumulativePromptChars)
	}
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ".", nil
}
