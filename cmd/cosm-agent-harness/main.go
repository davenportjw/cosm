package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cosmscm/cosm/test/agents/framework"
	"github.com/cosmscm/cosm/test/agents/llm"
	"github.com/cosmscm/cosm/test/agents/rater"
	"github.com/cosmscm/cosm/test/agents/testagent"
)

func printUsage() {
	fmt.Print(`🛡️ cosm-agent-harness - External Autonomous Test Agent & Rater Harness

Usage:
  cosm-agent-harness <subcommand> [flags]

Subcommands:
  run              Run an autonomous agent session on a polyglot test scenario
  rate             Audit a workspace with the Deep Verification Oracle and generate a scorecard
  suite            Execute the full polyglot benchmark test suite end-to-end
  list-scenarios   List available polyglot application templates

Flags:
  --scenario <name>       Scenario name (e.g. fastapi-react-tf, go-gin-vue-tf, rust-axum-react-postgres-tf, swift-go-tf, kotlin-py-sql, csharp-react-proto)
  --model <model>         LLM model to use (default: gemini-3.7-flash)
  --workdir <dir>         Workspace directory (default: temp directory)
  --universe <id>         Target universe ID for verification (default: universe-main)
  --mock                  Use hermetic Mock LLM provider without network access
  --max-steps <int>       Maximum reasoning steps (default: 25)
  --json                  Output results in JSON format
  --out <file>            Write output Markdown / JSON to file
  --out-dir <dir>         Directory for suite run outputs

Examples:
  cosm-agent-harness run --scenario fastapi-react-tf --mock
  cosm-agent-harness rate --workdir ./my-workspace
  cosm-agent-harness suite --mock
`)
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	subcommand := os.Args[1]
	args := os.Args[2:]

	switch subcommand {
	case "run":
		runAgentCommand(args)
	case "rate":
		rateWorkspaceCommand(args)
	case "suite":
		runSuiteCommand(args)
	case "list-scenarios":
		listScenariosCommand()
	case "--help", "-h", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand: %q\n\n", subcommand)
		printUsage()
		os.Exit(1)
	}
}

func listScenariosCommand() {
	fmt.Println("Available Polyglot Application Test Scenarios:")
	fmt.Println("-------------------------------------------------------------------------------")
	scenarios := testagent.ListScenarios()
	for i, s := range scenarios {
		fmt.Printf("%d. %s\n", i+1, s.Name)
		fmt.Printf("   Description: %s\n", s.Description)
		fmt.Printf("   Files (%d):   %s\n", len(s.Files), formatFileList(s.Files))
		fmt.Printf("   Contracts:  %d cross-boundary contracts\n\n", len(s.TargetContracts))
	}
}

func formatFileList(files map[string]string) string {
	var list []string
	for k := range files {
		list = append(list, k)
	}
	return strings.Join(list, ", ")
}

func runAgentCommand(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	scenarioName := fs.String("scenario", "fastapi-react-tf", "Target scenario name")
	model := fs.String("model", llm.DefaultGeminiModel, "LLM model to use")
	workDir := fs.String("workdir", "", "Workspace directory (default: auto temp)")
	mockMode := fs.Bool("mock", false, "Use hermetic MockProvider")
	maxSteps := fs.Int("max-steps", 25, "Maximum agent turns")
	outFile := fs.String("out", "", "Output file path")
	jsonOut := fs.Bool("json", false, "Output in JSON")
	_ = fs.Parse(args)

	scenario, err := testagent.GetScenario(*scenarioName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	targetDir := *workDir
	if targetDir == "" {
		tmp, err := os.MkdirTemp("", fmt.Sprintf("cosm-harness-%s-*", *scenarioName))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed creating temp dir: %v\n", err)
			os.Exit(1)
		}
		targetDir = tmp
		defer os.RemoveAll(tmp)
	}

	var provider llm.LLMProvider
	if *mockMode {
		mock := llm.NewMockProvider("mock-gemini-3.7-flash")
		mock.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
		var files []string
		for k := range scenario.Files {
			files = append(files, k)
		}
		filesJSON, _ := strings.Join(files, `","`), `["`+strings.Join(files, `","`)+`"]`
		_ = filesJSON
		mock.EnqueueToolCall("cosm_add", fmt.Sprintf(`{"files":%s}`, toJSONSlice(files)))
		mock.EnqueueToolCall("cosm_commit", fmt.Sprintf(`{"intent":"Autonomous ingest of %s"}`, scenario.Name))
		mock.EnqueueToolCall("cosm_ship", `{"universe_id":"universe-main"}`)
		mock.EnqueueText(fmt.Sprintf("Scenario %s successfully initialized, committed, and shipped.", scenario.Name))
		provider = mock
	} else {
		gemini, gErr := llm.NewGeminiProvider(llm.GeminiConfig{
			Model: *model,
		})
		if gErr != nil {
			fmt.Fprintf(os.Stderr, "Error creating Gemini provider: %v\n", gErr)
			os.Exit(1)
		}
		provider = gemini
	}

	fmt.Printf("🚀 Running scenario %q with model %q in %s...\n", scenario.Name, *model, targetDir)
	agent := testagent.NewTestAgent("agent-harness", provider, targetDir, *model)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	session, err := agent.RunScenario(ctx, *scenario, framework.LoopOptions{
		MaxSteps: *maxSteps,
		OnStepCallback: func(trace framework.StepTrace) {
			if len(trace.ToolCalls) > 0 {
				fmt.Printf("   [Step %d] Calling tool: %s\n", trace.StepIndex, trace.ToolCalls[0].Name)
			} else {
				fmt.Printf("   [Step %d] Agent response: %s\n", trace.StepIndex, trace.ResponseText)
			}
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Scenario execution encountered error: %v\n", err)
	}

	fmt.Printf("\n✨ Session completed with status: %s (Duration: %s, Steps: %d, Tokens: %d)\n\n",
		session.Status, session.Duration.Round(time.Millisecond), session.TotalSteps, session.TotalTokens.TotalTokens)

	// Rate the resulting workspace
	oracle := rater.NewDeepVerificationOracle()
	audit, _ := oracle.AuditWorkspace(targetDir, "universe-main")
	scorecard := rater.CalculateScorecard(audit)
	reporter := rater.NewReporter()

	if *jsonOut {
		data, _ := scorecard.ToJSON()
		if *outFile != "" {
			_ = os.WriteFile(*outFile, data, 0644)
			fmt.Printf("Wrote scorecard JSON to %s\n", *outFile)
		} else {
			fmt.Println(string(data))
		}
	} else {
		md := reporter.GenerateMarkdownScorecard(scorecard, audit, session)
		if *outFile != "" {
			_ = os.WriteFile(*outFile, []byte(md), 0644)
			fmt.Printf("Wrote scorecard Markdown to %s\n", *outFile)
		} else {
			fmt.Println(md)
		}
	}
}

func rateWorkspaceCommand(args []string) {
	fs := flag.NewFlagSet("rate", flag.ExitOnError)
	workDir := fs.String("workdir", ".", "Target workspace directory to audit")
	universeID := fs.String("universe", "universe-main", "Universe ID to inspect")
	outFile := fs.String("out", "", "Output file path")
	jsonOut := fs.Bool("json", false, "Output in JSON format")
	_ = fs.Parse(args)

	absDir, err := filepath.Abs(*workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid workdir path: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("🔍 Auditing workspace %s (Universe: %s)...\n", absDir, *universeID)
	oracle := rater.NewDeepVerificationOracle()
	audit, err := oracle.AuditWorkspace(absDir, *universeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Audit failure: %v\n", err)
		os.Exit(1)
	}

	scorecard := rater.CalculateScorecard(audit)
	reporter := rater.NewReporter()

	if *jsonOut {
		data, _ := scorecard.ToJSON()
		if *outFile != "" {
			_ = os.WriteFile(*outFile, data, 0644)
			fmt.Printf("Wrote scorecard JSON to %s\n", *outFile)
		} else {
			fmt.Println(string(data))
		}
	} else {
		md := reporter.GenerateMarkdownScorecard(scorecard, audit, nil)
		if *outFile != "" {
			_ = os.WriteFile(*outFile, []byte(md), 0644)
			fmt.Printf("Wrote scorecard Markdown to %s\n", *outFile)
		} else {
			fmt.Println(md)
		}
	}
}

func runSuiteCommand(args []string) {
	fs := flag.NewFlagSet("suite", flag.ExitOnError)
	model := fs.String("model", llm.DefaultGeminiModel, "LLM model to use")
	mockMode := fs.Bool("mock", true, "Use hermetic mock LLM provider")
	outDir := fs.String("out-dir", "", "Output directory for benchmark reports")
	_ = fs.Parse(args)

	scenarios := testagent.ListScenarios()
	fmt.Printf("🧪 Launching Cosm Benchmark Test Suite (%d scenarios)...\n\n", len(scenarios))

	type SuiteResult struct {
		Scenario  testagent.Scenario
		Scorecard *rater.Scorecard
		Session   *framework.AgentSession
	}

	var results []SuiteResult
	allPassed := true

	for i, sc := range scenarios {
		fmt.Printf("[%d/%d] Executing Scenario: %s\n", i+1, len(scenarios), sc.Name)

		tmpDir, err := os.MkdirTemp("", fmt.Sprintf("suite-%s-*", sc.Name))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed creating temp dir: %v\n", err)
			continue
		}
		defer os.RemoveAll(tmpDir)

		var provider llm.LLMProvider
		if *mockMode {
			mock := llm.NewMockProvider("mock-gemini-3.7-flash")
			mock.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
			var files []string
			for k := range sc.Files {
				files = append(files, k)
			}
			mock.EnqueueToolCall("cosm_add", fmt.Sprintf(`{"files":%s}`, toJSONSlice(files)))
			mock.EnqueueToolCall("cosm_commit", fmt.Sprintf(`{"intent":"Suite ingest %s"}`, sc.Name))
			mock.EnqueueToolCall("cosm_ship", `{"universe_id":"universe-main"}`)
			mock.EnqueueText(fmt.Sprintf("Suite run for %s finished.", sc.Name))
			provider = mock
		} else {
			gemini, _ := llm.NewGeminiProvider(llm.GeminiConfig{Model: *model})
			provider = gemini
		}

		agent := testagent.NewTestAgent("suite-runner", provider, tmpDir, *model)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		session, _ := agent.RunScenario(ctx, sc, framework.LoopOptions{MaxSteps: 15})
		cancel()

		oracle := rater.NewDeepVerificationOracle()
		audit, _ := oracle.AuditWorkspace(tmpDir, "universe-main")
		scorecard := rater.CalculateScorecard(audit)
		scorecard.ScenarioName = sc.Name

		if scorecard.OverallScore < 80.0 {
			allPassed = false
		}

		results = append(results, SuiteResult{
			Scenario:  sc,
			Scorecard: scorecard,
			Session:   session,
		})

		fmt.Printf("    -> Score: %.1f/100 (Grade: %s, Status: %s)\n\n",
			scorecard.OverallScore, scorecard.LetterGrade, session.Status)
	}

	// Print Master Summary Table
	fmt.Println("===============================================================================")
	fmt.Println("                     COSM AGENT BENCHMARK SUITE REPORT                         ")
	fmt.Println("===============================================================================")
	fmt.Printf("%-30s | %-8s | %-6s | %-12s | %-8s\n", "Scenario", "Score", "Grade", "Status", "Findings")
	fmt.Println("-------------------------------------------------------------------------------")
	for _, res := range results {
		fmt.Printf("%-30s | %6.1f/100 | %-6s | %-12s | %-8d\n",
			res.Scenario.Name, res.Scorecard.OverallScore, res.Scorecard.LetterGrade, res.Session.Status, res.Scorecard.TotalFindings)
	}
	fmt.Println("-------------------------------------------------------------------------------")

	if *outDir != "" {
		_ = os.MkdirAll(*outDir, 0755)
		reporter := rater.NewReporter()
		for _, res := range results {
			md := reporter.GenerateMarkdownScorecard(res.Scorecard, nil, res.Session)
			p := filepath.Join(*outDir, fmt.Sprintf("scorecard-%s.md", res.Scenario.Name))
			_ = os.WriteFile(p, []byte(md), 0644)
		}
		fmt.Printf("Exported scorecard reports to %s\n", *outDir)
	}

	if !allPassed {
		fmt.Println("⚠️  Some scenarios scored below threshold (<80.0).")
		os.Exit(1)
	}
	fmt.Println("🎉 All scenarios passed with high scores!")
}

func toJSONSlice(items []string) string {
	b, _ := json.Marshal(items)
	return string(b)
}
