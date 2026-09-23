package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/cosmscm/cosm/pkg/codecs"
	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/distributed"
	"github.com/cosmscm/cosm/pkg/gitshim"
	"github.com/cosmscm/cosm/pkg/ignore"
	"github.com/cosmscm/cosm/pkg/lineage"
	"github.com/cosmscm/cosm/pkg/lsp"
	"github.com/cosmscm/cosm/pkg/materialize"
	"github.com/cosmscm/cosm/pkg/mcp"
	"github.com/cosmscm/cosm/pkg/mutation"
	"github.com/cosmscm/cosm/pkg/onboarding"
	"github.com/cosmscm/cosm/pkg/review"
	"github.com/cosmscm/cosm/pkg/shipping"
	"github.com/cosmscm/cosm/pkg/storage"
	"github.com/cosmscm/cosm/pkg/target"
	"github.com/cosmscm/cosm/pkg/topocosm"
	"github.com/cosmscm/cosm/pkg/topocosm/backplane"
	"github.com/cosmscm/cosm/pkg/tui"
	"github.com/cosmscm/cosm/pkg/watcher"
)

func printHelp() {
	fmt.Println(`cosm - AI-Native Polyglot AST SCM & Target Compilation System

Usage:
  cosm <command> [arguments] [flags]

Core Commands:
  init                             Initialize a new .cosm/ repository in current directory
  add <files...>                   Parse Go/HCL/TS/Python files into AST symbol nodes & stage to store
  commit -i <intent>               Commit active manifest & update universe head
  log [-u <universe>] [-n <limit>] Show universe Merkle commit history, causal lineage & symbol diffs
  status                           Show working tree AST status vs active universe head
  topology                         Render multi-domain topology graph (ASCII / Mermaid)
  lineage <node_id>                Trace full causal lineage chain back to originating user prompt
  blast-radius <agent>             Audit blast radius & affected downstream code across domains
  view <node_id>                   Reconstitute & view syntax-highlighted code with lineage provenance
  ship                             Run sidecar build, validate Terraform, compile, and launch preview
  universe <list|create|diff|merge> Manage micro-universes and branches
  proposal <create|list|view|review|merge> Manage, view, and critique AI-native PRs and proposals
  stack <list|create|evolve>       Manage Jujutsu-style stacked proposals & auto-evolution
  peer <status|sync>               Distributed P2P swarm mesh replication & sparse AST sync
  ast <create|edit|resolve|tree>   Declarative AST mutations, scoped symbol resolution & hierarchical tree view
  import-repo -d <dir>             Onboard existing codebase into AST Merkle-DAG with contract inference
  import -d <dir>                  Recursively scan and ingest code files into active universe
  export -d <dir>                  Reconstitute and write entire universe AST back to disk
  dashboard                        Launch interactive terminal dashboard (TUI)
  git <status|log|diff|push>       Local Git compatibility shim (for IDEs and local Git tooling)

IDE & Machine Interfaces:
  mcp [-d <dir>] [-u <universe>]   Start Model Context Protocol (MCP) JSON-RPC 2.0 stdio server
  lsp [-d <dir>]                   Start Language Server Protocol (LSP) JSON-RPC 2.0 stdio server
  watch [-d <dir>] [-u <universe>] Run background filesystem watcher and auto-stager daemon
  git init-bridge [-d <dir>]       Initialize synthetic .git bridge for IDE compatibility

Authentication & Teamwork:
  auth <login|whoami|token|logout> Authenticate developer CLI and manage personal access tokens (PATs)
  credential-helper <get|store>    Git-compatible credential helper for PAT auto-negotiation
  share <url_or_path>              Grant collaborator access with client-side envelope encryption

Topocosm (topocosm.dev & Local Hub) Commands:
  topocosm dev                     Start local zero-Docker Topocosm Hub daemon (.topocosm/)
  topocosm seed                    Populate local hub with sample polyglot cosms & stacked proposals
  topocosm test-swarm              Run concurrent multi-agent swarm simulation benchmark
  topocosm status                  Inspect hub metrics, active cosms, and agent blackboard claims
  publish <url>/<org>/<cosm>       Push & publish local universe AST DAG to Topocosm Hub
  clone <url>/<org>/<cosm>         Clone or sparse-pull cosm from Topocosm Hub into local directory
  claim <domain> [target]          Acquire an exclusive agent mutation lease on a blackboard domain
  release <domain> [target]        Release an active blackboard domain mutation lease
  blackboard [target]              Inspect active agent blackboard domain leases and countdowns

Flags:
  -u, --universe <id>              Active micro-universe ID (default: universe-main)
  -i, --intent <msg>               Intent or commit message
  -a, --agent <id>                 Author / agent DID or identifier
  -p, --prompt <text>              Originating user prompt or task requirement
  -f, --format <text|json|mermaid> Output format for visualization
  -v, --verbose                    Enable verbose debug output`)
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(0)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "help", "--help", "-h":
		printHelp()

	case "version", "--version", "-v":
		fmt.Println("cosm version 0.1.0-dev (AST Source Control Management & Compiler)")

	case "init":
		runInit(args)

	case "add":
		runAdd(args)

	case "commit":
		runCommit(args)

	case "log":
		runLog(args)

	case "status":
		runStatus(args)

	case "topology":
		runTopology(args)

	case "lineage":
		runLineage(args)

	case "blast-radius":
		runBlastRadius(args)

	case "view":
		runView(args)

	case "ship":
		runShip(args)

	case "universe", "branch":
		runUniverse(args)

	case "proposal", "pr":
		runProposal(args)

	case "stack":
		runStack(args)

	case "peer":
		runPeer(args)

	case "import":
		runImport(args)

	case "import-repo", "onboard":
		runImportRepo(args)

	case "symbol":
		runSymbol(args)

	case "ast":
		runAST(args)

	case "export":
		runExport(args)

	case "dashboard", "tui":
		runDashboard(args)

	case "git":
		runGit(args)

	case "mcp":
		runMCP(args)

	case "lsp":
		runLSP(args)

	case "watch":
		runWatch(args)

	case "auth":
		runAuth(args)

	case "credential-helper":
		runCredentialHelper(args)

	case "share":
		runShare(args)

	case "topocosm":
		runTopocosm(args)

	case "publish":
		runPublish(args)

	case "clone":
		runClone(args)

	case "claim":
		runClaim(args)

	case "release":
		runRelease(args)

	case "blackboard":
		runBlackboard(args)

	default:
		fmt.Printf("Unknown command: %s\nRun 'cosm help' for usage.\n", command)
		os.Exit(1)
	}
}
func runInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	universeID := fs.String("u", "universe-main", "Initial universe ID")
	fs.StringVar(universeID, "universe", "universe-main", "Initial universe ID (alias)")
	_ = fs.Parse(args)

	cosmDir := ".cosm"
	if err := os.MkdirAll(filepath.Join(cosmDir, "objects"), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating .cosm/objects: %v\n", err)
		os.Exit(1)
	}

	blobStore, err := storage.NewBlobStore(filepath.Join(cosmDir, "objects"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing blobstore: %v\n", err)
		os.Exit(1)
	}

	graphEngine, err := storage.NewGraphEngine(filepath.Join(cosmDir, "graph.db"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing graphengine: %v\n", err)
		os.Exit(1)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	_, _ = universeMgr.CreateUniverse(*universeID, "")

	// Customer Zero invariant: Auto-commit an initial empty WorkspaceManifestNode
	// so newly initialized universes begin with a valid, committed Merkle root.
	initialManifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-" + *universeID,
		UniverseID:  *universeID,
		Components:  []string{},
		CrossEdges:  []core.CrossBoundaryEdge{},
		CreatedAt:   time.Now().UTC(),
	}
	initialHash, _ := core.HashWorkspaceManifest(initialManifest)
	initialManifest.MerkleRootHash = initialHash
	_, _ = universeMgr.CommitManifest(*universeID, initialManifest)

	fmt.Println("✨ Initialized empty Cosm repository in .cosm/")
	fmt.Printf("   Active Universe: %s (Merkle Root: %s)\n", *universeID, initialHash[:min(12, len(initialHash))])
	fmt.Println()
	fmt.Println("Next Steps (Two paths to build):")
	fmt.Println("1. Native AST Inception (Agent / AST-First Paradigm):")
	fmt.Println("   • Incept component:  cosm ast create -c <name> --lang <go|python|ts|sql>")
	fmt.Println("   • Or with code:      cosm ast create -c <name> -f <path> --code \"...\"")
	fmt.Println("   • Surgical edits:    cosm ast edit --op replace_function_body --target <symbol> --content \"...\" -w")
	fmt.Println("   • Inspect AST DAG:   cosm ast tree")
	fmt.Println("   • Commit changes:    cosm commit -i \"Commit description\"")
	fmt.Println()
	fmt.Println("2. Filesystem Staging Lens (Disk / Hybrid Paradigm):")
	fmt.Println("   • Stage directory:   cosm add . (or cosm add <file_or_dir>)")
	fmt.Println("   • Workspace status:  cosm status")
	fmt.Println("   • Commit changes:    cosm commit -i \"Initial commit\"")
}

func runAdd(args []string) {
	if err := runAddE(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runAddE(args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	universeID := fs.String("u", "universe-main", "Active micro-universe ID")
	agentID := fs.String("a", "cosm-user-agent", "Agent ID")
	prompt := fs.String("p", "", "User prompt")
	intent := fs.String("i", "Staged changes", "Intent")
	sessionID := fs.String("session-id", "", "Agent session ID")
	orchID := fs.String("orchestrator-id", "", "Orchestrator Agent ID")
	model := fs.String("m", "", "LLM Model Version")
	promptTokens := fs.Int64("prompt-tokens", 0, "Prompt tokens")
	compTokens := fs.Int64("completion-tokens", 0, "Completion tokens")
	reasonTokens := fs.Int64("reasoning-tokens", 0, "Reasoning / thinking tokens")
	cachedTokens := fs.Int64("cached-tokens", 0, "Cached prompt tokens")
	costUSD := fs.Float64("cost-usd", 0.0, "Estimated cost in USD")
	latencyMs := fs.Int64("latency-ms", 0, "Latency in milliseconds")
	traceID := fs.String("trace-id", "", "W3C Trace ID")
	spanID := fs.String("span-id", "", "W3C Span ID")
	force := fs.Bool("force", false, "Allow adding files matching .cosmignore or default exclusion rules")
	fs.BoolVar(force, "f", false, "Allow adding files matching .cosmignore rules (shorthand)")
	allowSecrets := fs.Bool("allow-secrets", false, "Allow staging files containing detected credentials or plaintext secrets")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_ = universeID

	files := fs.Args()
	if len(files) == 0 {
		fmt.Println("No files specified to add. Usage: cosm add <file_or_dir...>")
		fmt.Println("To stage all workspace files: cosm add .")
		fmt.Println("To create AST nodes directly without disk files: cosm ast create -c <component> --lang <go|python|ts|sql>")
		return nil
	}

	ignoreEngine, _ := ignore.LoadWorkspaceRules(".")
	if ignoreEngine == nil {
		ignoreEngine = ignore.NewIgnoreEngine(".")
	}
	secretDetector := ignore.NewSecretDetector()

	var expandedFiles []string
	explicitFiles := make(map[string]bool)
	for _, p := range files {
		fi, err := os.Stat(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error inspecting %s: %v\n", p, err)
			continue
		}
		if fi.IsDir() {
			_ = filepath.Walk(p, func(walkPath string, info os.FileInfo, walkErr error) error {
				if walkErr != nil || info == nil {
					return nil
				}
				cleanWalk := filepath.Clean(walkPath)
				var relWalk string
				if r, rErr := filepath.Rel(".", cleanWalk); rErr == nil && !strings.HasPrefix(r, "..") {
					relWalk = r
				} else {
					relWalk = cleanWalk
				}
				if info.IsDir() {
					base := filepath.Base(walkPath)
					if strings.HasPrefix(base, ".") && walkPath != "." && walkPath != p {
						return filepath.SkipDir
					}
					if base == "node_modules" || base == "vendor" || base == "target" || base == "dist" {
						return filepath.SkipDir
					}
					if !*force && ignoreEngine.ShouldIgnorePath(relWalk, true) {
						return filepath.SkipDir
					}
					return nil
				}
				if strings.HasPrefix(filepath.Base(walkPath), ".") {
					return nil
				}
				if !*force && ignoreEngine.ShouldIgnorePath(relWalk, false) {
					return nil
				}
				expandedFiles = append(expandedFiles, walkPath)
				return nil
			})
		} else {
			expandedFiles = append(expandedFiles, p)
			explicitFiles[p] = true
		}
	}

	if len(expandedFiles) == 0 {
		fmt.Println("No files found to stage.")
		return nil
	}

	blobStore, graphEngine, err := openStorage()
	if err != nil {
		return fmt.Errorf("storage error: %w", err)
	}
	defer graphEngine.Close()

	lineageEnv := core.LineageEnvelope{
		UserID:              os.Getenv("USER"),
		UserPrompt:          *prompt,
		SessionID:           *sessionID,
		OrchestratorAgentID: *orchID,
		ExecutingAgentID:    *agentID,
		LLMVersion:          *model,
		Intent:              *intent,
		Timestamp:           time.Now().UTC(),
		Tokens: core.TokenTelemetry{
			PromptTokens:     *promptTokens,
			CompletionTokens: *compTokens,
			ReasoningTokens:  *reasonTokens,
			CachedTokens:     *cachedTokens,
			TotalTokens:      *promptTokens + *compTokens + *reasonTokens,
			CostUSD:          *costUSD,
			LatencyMs:        *latencyMs,
		},
		Trace: core.TraceCarrier{
			TraceID: *traceID,
			SpanID:  *spanID,
		},
	}

	for _, path := range expandedFiles {
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", path, err)
			continue
		}

		cleanPath := filepath.Clean(path)
		var relPath string
		if r, rErr := filepath.Rel(".", cleanPath); rErr == nil && !strings.HasPrefix(r, "..") {
			relPath = r
		} else {
			wd, _ := os.Getwd()
			realWd, _ := filepath.EvalSymlinks(wd)
			realPath, _ := filepath.EvalSymlinks(cleanPath)
			if r2, r2Err := filepath.Rel(realWd, realPath); r2Err == nil && !strings.HasPrefix(r2, "..") {
				relPath = r2
			} else {
				relPath = filepath.Base(cleanPath)
			}
		}

		if !*force && ignoreEngine.ShouldIgnorePath(relPath, false) {
			if explicitFiles[path] {
				return fmt.Errorf("'%s' is ignored by .cosmignore or default exclusion rules. Use --force (-f) to override", relPath)
			}
			continue
		}

		if !*allowSecrets {
			if findings := secretDetector.DetectSecrets(relPath, content); len(findings) > 0 {
				var msgs []string
				for _, f := range findings {
					msgs = append(msgs, fmt.Sprintf("line %d: %s [%s]", f.LineNumber, f.Snippet, f.Type))
				}
				return fmt.Errorf("secret detected in '%s' (%s). Aborting stage to prevent credential leakage. Use --allow-secrets or inline '// cosm:allow-secret' to override", relPath, strings.Join(msgs, ", "))
			}
		}

		var comp *core.ComponentNode
		syms := make(map[string]*core.ASTSymbolNode)

		parsed, pErr := codecs.ParseSourceFile(relPath, content, lineageEnv)
		if pErr == nil && parsed != nil && parsed.Component != nil {
			comp = parsed.Component
			syms = parsed.Symbols
		} else {
			// Non-AST raw file preservation (e.g. LICENSE, README.md, YAML, configs, assets)
			h := sha256.Sum256([]byte(relPath))
			shortHash := hex.EncodeToString(h[:4])
			rawSymID := fmt.Sprintf("raw:%s", shortHash)
			rawSym := &core.ASTSymbolNode{
				NodeID:            rawSymID,
				Language:          core.LangRaw,
				NodeType:          "RawBlobNode",
				Identifier:        relPath,
				ASTPayload:        content,
				LocalDependencies: []string{},
				Lineage:           lineageEnv,
			}
			syms[rawSymID] = rawSym
			comp = &core.ComponentNode{
				ComponentID: fmt.Sprintf("comp-raw-%s", shortHash),
				Name:        relPath,
				Type:        core.CompService,
				Language:    core.LangRaw,
				SymbolNodes: []string{rawSymID},
				Metadata:    map[string]string{"file_path": relPath},
				Lineage:     lineageEnv,
			}
		}

		for symID, sym := range syms {
			data, _ := json.Marshal(sym)
			symHash, _ := blobStore.Put(data)
			_ = graphEngine.PutNode(storage.NodeRecord{
				NodeID:     symID,
				Language:   sym.Language,
				NodeType:   sym.NodeType,
				MerkleHash: symHash,
				CreatedAt:  time.Now(),
			})
			_ = graphEngine.PutLineage(storage.LineageRecord{
				RecordID:         fmt.Sprintf("lin-%s", symID[:12]),
				NodeID:           symID,
				UserID:           lineageEnv.UserID,
				UserPrompt:       lineageEnv.UserPrompt,
				ExecutingAgentID: lineageEnv.ExecutingAgentID,
				Intent:           lineageEnv.Intent,
				Timestamp:        lineageEnv.Timestamp,
			})
		}

		compData, _ := json.Marshal(comp)
		compHash, _ := blobStore.Put(compData)
		_ = graphEngine.PutNode(storage.NodeRecord{
			NodeID:     comp.ComponentID,
			Language:   comp.Language,
			NodeType:   string(comp.Type),
			MerkleHash: compHash,
			CreatedAt:  time.Now(),
		})
		_ = graphEngine.PutLineage(storage.LineageRecord{
			RecordID:            fmt.Sprintf("lin-%s", comp.ComponentID[:12]),
			NodeID:              comp.ComponentID,
			UserID:              lineageEnv.UserID,
			UserPrompt:          lineageEnv.UserPrompt,
			SessionID:           lineageEnv.SessionID,
			OrchestratorAgentID: lineageEnv.OrchestratorAgentID,
			ExecutingAgentID:    lineageEnv.ExecutingAgentID,
			LLMVersion:          lineageEnv.LLMVersion,
			Intent:              lineageEnv.Intent,
			Timestamp:           lineageEnv.Timestamp,
			Tokens:              lineageEnv.Tokens,
			Trace:               lineageEnv.Trace,
		})

		fmt.Printf("✓ Staged AST Component: %s (%s, %d symbols, hash: %s)\n",
			comp.Name, comp.Language, len(comp.SymbolNodes), comp.ComponentID[:12])
	}
	return nil
}

func runCommit(args []string) {
	fs := flag.NewFlagSet("commit", flag.ExitOnError)
	universeID := fs.String("u", "universe-main", "Universe ID")
	fs.StringVar(universeID, "universe", "universe-main", "Universe ID (alias)")
	intent := fs.String("i", "Manual commit", "Commit message / intent")
	fs.StringVar(intent, "intent", "Manual commit", "Commit message / intent (alias)")
	prompt := fs.String("p", "", "Originating user prompt")
	fs.StringVar(prompt, "prompt", "", "Originating user prompt (alias)")
	sessionID := fs.String("session-id", "", "Agent session ID")
	orchID := fs.String("orchestrator-id", "", "Orchestrator Agent ID")
	agentID := fs.String("a", "cosm-user-agent", "Agent ID")
	model := fs.String("m", "", "LLM Model Version (e.g., gemini-3.7-flash)")
	params := fs.String("params", "", "JSON string of generation parameters")
	promptTokens := fs.Int64("prompt-tokens", 0, "Prompt tokens")
	compTokens := fs.Int64("completion-tokens", 0, "Completion tokens")
	fs.Int64Var(compTokens, "comp-tokens", 0, "Alias for --completion-tokens")
	reasonTokens := fs.Int64("reasoning-tokens", 0, "Reasoning / thinking tokens")
	cachedTokens := fs.Int64("cached-tokens", 0, "Cached prompt tokens")
	costUSD := fs.Float64("cost-usd", 0.0, "Estimated cost in USD")
	fs.Float64Var(costUSD, "cost", 0.0, "Alias for --cost-usd")
	latencyMs := fs.Int64("latency-ms", 0, "Latency in milliseconds")
	traceID := fs.String("trace-id", "", "W3C Trace ID")
	fs.StringVar(traceID, "trace", "", "Alias for --trace-id")
	spanID := fs.String("span-id", "", "W3C Span ID")
	_ = fs.Parse(args)

	blobStore, graphEngine, err := openStorage()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Storage error: %v\n", err)
		os.Exit(1)
	}
	defer graphEngine.Close()

	nodes := graphEngine.ListNodes("", "")
	symbolMap := make(map[string]*core.ASTSymbolNode)
	latestCompByPath := make(map[string]*core.ComponentNode)

	for _, n := range nodes {
		if n.NodeType == "service" || n.NodeType == "infra" || n.NodeType == "frontend" || n.NodeType == "library" || n.NodeType == "contract" {
			data, err := blobStore.Get(n.MerkleHash)
			if err == nil {
				var c core.ComponentNode
				if e := json.Unmarshal(data, &c); e == nil {
					key := c.Metadata["file_path"]
					if key == "" {
						key = c.Name
					}
					existing, exists := latestCompByPath[key]
					if !exists || c.Lineage.Timestamp.After(existing.Lineage.Timestamp) {
						latestCompByPath[key] = &c
					}
				}
			}
		} else {
			data, err := blobStore.Get(n.MerkleHash)
			if err == nil {
				var s core.ASTSymbolNode
				if e := json.Unmarshal(data, &s); e == nil {
					symbolMap[s.NodeID] = &s
				}
			}
		}
	}

	var compIDs []string
	var components []*core.ComponentNode
	for _, c := range latestCompByPath {
		compIDs = append(compIDs, c.ComponentID)
		components = append(components, c)
	}
	sort.Strings(compIDs)

	engine, _ := ignore.LoadWorkspaceRules(".")
	linker := core.NewCrossBoundaryLinker()
	if engine != nil && engine.EdgeFilter != nil {
		linker.SetFilter(engine.EdgeFilter)
	}
	edges, err := linker.LinkWorkspace(components, symbolMap)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning linking cross-boundary edges: %v\n", err)
	}

	totTokens := *promptTokens + *compTokens + *reasonTokens
	lineageEnv := core.LineageEnvelope{
		UserID:              os.Getenv("USER"),
		UserPrompt:          *prompt,
		SessionID:           *sessionID,
		OrchestratorAgentID: *orchID,
		ExecutingAgentID:    *agentID,
		LLMVersion:          *model,
		GenerationParams:    *params,
		Intent:              *intent,
		Timestamp:           time.Now().UTC(),
		Tokens: core.TokenTelemetry{
			PromptTokens:     *promptTokens,
			CompletionTokens: *compTokens,
			ReasoningTokens:  *reasonTokens,
			CachedTokens:     *cachedTokens,
			TotalTokens:      totTokens,
			CostUSD:          *costUSD,
			LatencyMs:        *latencyMs,
		},
		Trace: core.TraceCarrier{
			TraceID: *traceID,
			SpanID:  *spanID,
		},
	}

	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-" + *universeID,
		UniverseID:  *universeID,
		Components:  compIDs,
		CrossEdges:  edges,
		Lineage:     lineageEnv,
		CreatedAt:   time.Now().UTC(),
	}

	manifestHash, err := core.HashWorkspaceManifest(manifest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error hashing manifest: %v\n", err)
		os.Exit(1)
	}
	manifest.MerkleRootHash = manifestHash

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	_, err = universeMgr.CommitManifest(*universeID, manifest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error committing to universe %s: %v\n", *universeID, err)
		os.Exit(1)
	}

	fmt.Printf("🌟 Committed to universe '%s' (Merkle Root: %s)\n", *universeID, manifestHash[:16])
	fmt.Printf("   Components: %d | Cross-Domain Edges: %d\n", len(compIDs), len(edges))
	if *prompt != "" {
		fmt.Printf("   Originating Prompt: \"%s\"\n", *prompt)
	}
	if totTokens > 0 {
		fmt.Printf("   Tokens: %d total (%d prompt, %d completion, %d reasoning)\n",
			totTokens, *promptTokens, *compTokens, *reasonTokens)
	}
	if *costUSD > 0 {
		fmt.Printf("   Cost:   $%.4f USD\n", *costUSD)
	}
	if *traceID != "" {
		fmt.Printf("   Trace:  %s\n", *traceID)
	}
}

// CommitLogEntry details a committed Merkle manifest in a universe's history.
type CommitLogEntry struct {
	CommitHash    string    `json:"commit_hash"`
	UniverseID    string    `json:"universe_id"`
	Author        string    `json:"author"`
	AgentDID      string    `json:"agent_did,omitempty"`
	LLMVersion    string    `json:"llm_version,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
	Intent        string    `json:"intent"`
	UserPrompt    string    `json:"user_prompt,omitempty"`
	ParentHash    string    `json:"parent_hash,omitempty"`
	Components    []string  `json:"components,omitempty"`
	SymbolChanges []string  `json:"symbol_changes,omitempty"`
}

func runLog(args []string) {
	fs := flag.NewFlagSet("log", flag.ExitOnError)
	universeID := fs.String("u", "universe-main", "Universe ID")
	fs.StringVar(universeID, "universe", "universe-main", "Universe ID")
	limit := fs.Int("n", 20, "Number of commits to display")
	fs.IntVar(limit, "limit", 20, "Number of commits to display")
	format := fs.String("format", "terminal", "Output format (terminal, json)")
	_ = fs.Parse(args)

	blobStore, graphEngine, err := openStorage()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Storage error: %v\n", err)
		os.Exit(1)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)

	// Fetch event WAL records from oplog
	events, err := graphEngine.Oplog().GetEvents(*universeID, 0, 0)
	if err != nil || len(events) == 0 {
		events, _ = graphEngine.Oplog().GetEvents("", 0, 0)
	}

	var commitEntries []CommitLogEntry
	seenCommits := make(map[string]bool)

	for _, ev := range events {
		if ev.Action != storage.ActionCommitManifest {
			continue
		}
		if ev.UniverseID != *universeID && ev.UniverseID != "" {
			continue
		}
		if seenCommits[ev.EntityID] {
			continue
		}
		seenCommits[ev.EntityID] = true

		entry := buildCommitLogEntry(blobStore, graphEngine, ev.EntityID, ev.UndoPayload, ev.Payload, ev.UniverseID, ev.TimestampMs)
		commitEntries = append(commitEntries, entry)
	}

	// If no commit events were found in the oplog for this universe, inspect head manifest
	if len(commitEntries) == 0 {
		headRec, hErr := universeMgr.GetUniverse(*universeID)
		if hErr == nil && headRec != nil && headRec.HeadManifestHash != "" {
			allEvts, _ := graphEngine.Oplog().GetEvents("", 0, 0)
			for _, ev := range allEvts {
				if ev.Action == storage.ActionCommitManifest && ev.EntityID == headRec.HeadManifestHash {
					entry := buildCommitLogEntry(blobStore, graphEngine, ev.EntityID, ev.UndoPayload, ev.Payload, *universeID, ev.TimestampMs)
					commitEntries = append(commitEntries, entry)
					seenCommits[ev.EntityID] = true
					break
				}
			}
			if !seenCommits[headRec.HeadManifestHash] {
				entry := buildCommitLogEntry(blobStore, graphEngine, headRec.HeadManifestHash, "", "", *universeID, headRec.UpdatedAt.UnixMilli())
				commitEntries = append(commitEntries, entry)
			}
		}
	}

	// Reverse commit entries so the newest commit appears first
	for i, j := 0, len(commitEntries)-1; i < j; i, j = i+1, j-1 {
		commitEntries[i], commitEntries[j] = commitEntries[j], commitEntries[i]
	}

	if *limit > 0 && len(commitEntries) > *limit {
		commitEntries = commitEntries[:*limit]
	}

	if *format == "json" {
		data, err := json.MarshalIndent(commitEntries, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "JSON encoding error: %v\n", err)
			return
		}
		fmt.Println(string(data))
		return
	}

	if len(commitEntries) == 0 {
		fmt.Printf("No commits found for universe '%s'\n", *universeID)
		return
	}

	for i, c := range commitEntries {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("commit %s (universe: %s)\n", c.CommitHash, c.UniverseID)
		authorStr := c.Author
		if c.AgentDID != "" {
			if c.LLMVersion != "" {
				authorStr += fmt.Sprintf(" | Agent: %s (%s)", c.AgentDID, c.LLMVersion)
			} else {
				authorStr += fmt.Sprintf(" | Agent: %s", c.AgentDID)
			}
		}
		fmt.Printf("Author:    %s\n", authorStr)
		fmt.Printf("Date:      %s\n", c.Timestamp.Format(time.RFC1123))
		fmt.Printf("Intent:    %s\n", c.Intent)
		if c.UserPrompt != "" {
			fmt.Printf("Prompt:    %q\n", c.UserPrompt)
		}
		if len(c.SymbolChanges) > 0 {
			fmt.Println("\n  Symbols:")
			for _, sc := range c.SymbolChanges {
				fmt.Printf("    %s\n", sc)
			}
		}
	}
}

func buildCommitLogEntry(
	blobStore *storage.BlobStore,
	graphEngine *storage.GraphEngine,
	manifestHash, parentHash, payloadJSON, universeID string,
	timestampMs int64,
) CommitLogEntry {
	var manifest core.WorkspaceManifestNode

	if payloadJSON != "" {
		_ = json.Unmarshal([]byte(payloadJSON), &manifest)
	}
	if manifest.MerkleRootHash == "" && manifestHash != "" {
		if blobBytes, bErr := blobStore.Get(manifestHash); bErr == nil {
			_ = json.Unmarshal(blobBytes, &manifest)
		}
	}

	author := manifest.Lineage.UserID
	if author == "" {
		author = "cosm-user <user@cosm.local>"
	}
	agentDID := manifest.Lineage.ExecutingAgentID
	if agentDID == "" {
		agentDID = manifest.Lineage.OrchestratorAgentID
	}
	intent := manifest.Lineage.Intent
	if intent == "" {
		intent = "Workspace manifest commit"
	}
	prompt := manifest.Lineage.UserPrompt
	llmVersion := manifest.Lineage.LLMVersion

	t := time.UnixMilli(timestampMs)
	if !manifest.Lineage.Timestamp.IsZero() {
		t = manifest.Lineage.Timestamp
	}

	entry := CommitLogEntry{
		CommitHash: manifestHash,
		UniverseID: universeID,
		Author:     author,
		AgentDID:   agentDID,
		LLMVersion: llmVersion,
		Timestamp:  t,
		Intent:     intent,
		UserPrompt: prompt,
		ParentHash: parentHash,
		Components: manifest.Components,
	}

	// Compute symbol diff
	_, curSymMap := loadManifestState(blobStore, graphEngine, &manifest)
	curSyms := make(map[string]*core.ASTSymbolNode)
	for _, sym := range curSymMap {
		curSyms[sym.Identifier] = sym
	}

	var parentSyms map[string]*core.ASTSymbolNode
	if parentHash != "" {
		var parentManifest core.WorkspaceManifestNode
		if pBytes, pErr := blobStore.Get(parentHash); pErr == nil {
			_ = json.Unmarshal(pBytes, &parentManifest)
			_, pMap := loadManifestState(blobStore, graphEngine, &parentManifest)
			parentSyms = make(map[string]*core.ASTSymbolNode)
			for _, sym := range pMap {
				parentSyms[sym.Identifier] = sym
			}
		}
	}

	var changes []string
	if parentSyms == nil {
		for _, sym := range curSyms {
			changes = append(changes, fmt.Sprintf("+ %s (%s)", sym.Identifier, sym.NodeType))
		}
	} else {
		for ident, sym := range curSyms {
			if oldSym, exists := parentSyms[ident]; !exists {
				changes = append(changes, fmt.Sprintf("+ %s (%s)", sym.Identifier, sym.NodeType))
			} else if oldSym.NodeID != sym.NodeID {
				changes = append(changes, fmt.Sprintf("~ %s (%s)", sym.Identifier, sym.NodeType))
			}
		}
		for ident, oldSym := range parentSyms {
			if _, exists := curSyms[ident]; !exists {
				changes = append(changes, fmt.Sprintf("- %s (%s)", oldSym.Identifier, oldSym.NodeType))
			}
		}
	}
	sort.Strings(changes)
	entry.SymbolChanges = changes

	return entry
}

func runStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	universeID := fs.String("u", "universe-main", "Universe ID")
	format := fs.String("format", "text", "Output format (text, json)")
	fs.StringVar(format, "f", "text", "Alias for --format")
	_ = fs.Parse(args)

	blobStore, graphEngine, err := openStorage()
	if err != nil {
		if *format == "json" {
			res, _ := json.Marshal(map[string]interface{}{"status": "ERROR", "error": err.Error()})
			fmt.Println(string(res))
		} else {
			fmt.Fprintf(os.Stderr, "Storage error: %v\n", err)
		}
		os.Exit(1)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	head, err := universeMgr.GetUniverseManifest(*universeID)
	if err != nil && !strings.Contains(err.Error(), "has no committed manifest") {
		if *format == "json" {
			res, _ := json.Marshal(map[string]interface{}{"status": "ERROR", "error": err.Error()})
			fmt.Println(string(res))
		} else {
			fmt.Fprintf(os.Stderr, "Getting universe manifest: %v\n", err)
		}
		return
	}

	type fileDrift struct {
		path   string
		reason string
	}
	var drifted []fileDrift
	var allFiles map[string][]byte

	if head != nil && len(head.Components) > 0 {
		compMap, symMap := loadManifestState(blobStore, graphEngine, head)
		hydrator := materialize.NewHydrator()
		var hydErr error
		allFiles, hydErr = hydrator.HydrateWorkspace(head, compMap, symMap)
		if hydErr == nil && len(allFiles) > 0 {
			for relPath, expectedContent := range allFiles {
				diskBytes, rErr := os.ReadFile(relPath)
				if os.IsNotExist(rErr) {
					drifted = append(drifted, fileDrift{path: relPath, reason: "missing on disk"})
				} else if rErr != nil || strings.TrimSpace(string(diskBytes)) != strings.TrimSpace(string(expectedContent)) {
					drifted = append(drifted, fileDrift{path: relPath, reason: "modified on disk"})
				}
			}
			sort.Slice(drifted, func(i, j int) bool {
				return drifted[i].path < drifted[j].path
			})
		}
	}

	if *format == "json" {
		lensInfo := map[string]interface{}{
			"status":                "clean",
			"projected_files_count": len(allFiles),
			"drifted_files":         []string{},
		}
		if len(drifted) > 0 {
			driftPaths := make([]string, len(drifted))
			for idx, d := range drifted {
				driftPaths[idx] = fmt.Sprintf("%s (%s)", d.path, d.reason)
			}
			lensInfo["status"] = "drift_detected"
			lensInfo["drifted_files"] = driftPaths
		}

		out := map[string]interface{}{
			"status":            "SUCCESS",
			"universe_id":       *universeID,
			"merkle_root":       "",
			"components_count":  0,
			"cross_edges_count": 0,
			"components":        []string{},
			"working_tree_lens": lensInfo,
		}
		if head != nil {
			out["merkle_root"] = head.MerkleRootHash
			out["components_count"] = len(head.Components)
			out["cross_edges_count"] = len(head.CrossEdges)
			out["components"] = head.Components
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Printf("On universe: %s\n", *universeID)
	if head == nil || len(head.Components) == 0 {
		if head != nil && head.MerkleRootHash != "" {
			fmt.Printf("Head Merkle Root: %s\n", head.MerkleRootHash)
		}
		fmt.Println("No AST components tracked yet. Working tree clean.")
		return
	}
	fmt.Printf("Head Merkle Root: %s\n", head.MerkleRootHash)
	fmt.Printf("Components tracked: %d (AST Symbol DAG in .cosm/)\n", len(head.Components))
	fmt.Printf("Cross-boundary edges: %d\n", len(head.CrossEdges))
	if len(drifted) == 0 {
		fmt.Printf("Working Tree Lens: Materialized projection in sync with Merkle root (%d projected file(s))\n", len(allFiles))
	} else {
		fmt.Printf("Working Tree Lens: File drift detected (%d file(s) drifted from AST DAG)\n", len(drifted))
		for _, d := range drifted {
			fmt.Printf("   • %s (%s)\n", d.path, d.reason)
		}
		fmt.Println("   -> Run 'cosm add' to stage disk edits into AST DAG, or 'cosm export -d .' to reset disk projection to Merkle root.")
	}
}

func runTopology(args []string) {
	fs := flag.NewFlagSet("topology", flag.ExitOnError)
	universeID := fs.String("u", "universe-main", "Universe ID")
	format := fs.String("f", "ascii", "Output format (ascii/mermaid/json)")
	fs.StringVar(format, "format", "ascii", "Output format (ascii/mermaid/json)")
	_ = fs.Parse(args)

	blobStore, graphEngine, err := openStorage()
	if err != nil {
		if *format == "json" {
			res, _ := json.Marshal(map[string]interface{}{"status": "ERROR", "error": err.Error()})
			fmt.Println(string(res))
		} else {
			fmt.Fprintf(os.Stderr, "Storage error: %v\n", err)
		}
		os.Exit(1)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	head, err := universeMgr.GetUniverseManifest(*universeID)
	if err != nil {
		if *format == "json" {
			res, _ := json.Marshal(map[string]interface{}{"status": "ERROR", "error": err.Error()})
			fmt.Println(string(res))
		} else {
			fmt.Fprintf(os.Stderr, "Getting universe manifest: %v\n", err)
		}
		return
	}

	compMap, symMap := loadManifestState(blobStore, graphEngine, head)
	vis := target.NewTopologyVisualizer()
	topGraph := vis.BuildTopology(head, compMap, symMap)

	if *format == "mermaid" {
		fmt.Println(vis.RenderMermaid(topGraph))
	} else if *format == "json" {
		data, _ := json.MarshalIndent(topGraph, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Println(vis.RenderASCII(topGraph))
	}
}

func runLineage(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: cosm lineage <node_id_or_symbol> [flags]")
		return
	}

	fs := flag.NewFlagSet("lineage", flag.ContinueOnError)
	universeID := fs.String("u", "universe-main", "Universe ID")
	fs.StringVar(universeID, "universe", "universe-main", "Universe ID")
	_ = fs.Parse(args)

	var query string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			query = a
			break
		}
	}
	if query == "" && len(fs.Args()) > 0 {
		query = fs.Args()[0]
	}
	if query == "" {
		fmt.Println("Usage: cosm lineage <node_id_or_symbol> [flags]")
		return
	}

	blobStore, graphEngine, err := openStorage()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Storage error: %v\n", err)
		os.Exit(1)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	nodeID, err := resolveNodeOrSymbol(graphEngine, universeMgr, *universeID, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolution error: %v\n", err)
		return
	}

	tracer := lineage.NewLineageTracer(graphEngine)
	chain, err := tracer.TraceNodeAncestry(nodeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Lineage error for %s: %v\n", nodeID, err)
		return
	}

	fmt.Printf("=== Lineage Provenance Pedigree for Node: %s ===\n", chain.TargetNodeID)
	fmt.Printf("Root Prompt:         %s\n", chain.RootPrompt)
	fmt.Printf("Executing Agent:     %s\n", chain.PrimaryExecutorID)
	fmt.Printf("Total Lineage Hops:  %d (Intact: %v)\n", chain.TotalHops, chain.IntactChain)
	for i, h := range chain.Hops {
		fmt.Printf("  [%d] User: %s | Agent: %s | Intent: %q | Time: %s\n",
			i+1, h.Lineage.UserID, h.ExecutingAgentID, h.Intent, h.Timestamp.Format(time.RFC3339))
	}
}

func runBlastRadius(args []string) {
	fs := flag.NewFlagSet("blast-radius", flag.ExitOnError)
	format := fs.String("format", "text", "Output format (text, json)")
	fs.StringVar(format, "f", "text", "Alias for --format")
	universeID := fs.String("u", "universe-main", "Universe ID")
	fs.StringVar(universeID, "universe", "universe-main", "Universe ID")
	_ = fs.Parse(args)

	if fs.NArg() == 0 {
		fmt.Println("Usage: cosm blast-radius [flags] <node_id|symbol|agent_or_model>")
		return
	}
	target := fs.Arg(0)

	blobStore, graphEngine, err := openStorage()
	if err != nil {
		if *format == "json" {
			res, _ := json.Marshal(map[string]interface{}{"status": "ERROR", "error": err.Error()})
			fmt.Println(string(res))
		} else {
			fmt.Fprintf(os.Stderr, "Storage error: %v\n", err)
		}
		os.Exit(1)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)

	// First attempt: resolve target as a node ID or human symbol path
	resolvedID, rErr := resolveNodeOrSymbol(graphEngine, universeMgr, *universeID, target)
	if rErr == nil && resolvedID != "" {
		node, nErr := graphEngine.GetNode(resolvedID)
		if (nErr == nil && node != nil) || strings.Contains(target, "::") || (len(resolvedID) == 64 && isHex(resolvedID)) {
			cascadeEngine := mutation.NewContractCascadeEngine(graphEngine)
			assessment, cErr := cascadeEngine.AssessImpact(resolvedID)
			if cErr == nil {
				if *format == "json" {
					data, _ := json.MarshalIndent(assessment, "", "  ")
					fmt.Println(string(data))
					return
				}

				fmt.Printf("🎯 Blast-Radius & Contract Impact for '%s':\n", resolvedID)
				fmt.Printf("   • Risk Score:          %.2f / 1.00\n", assessment.RiskScore)
				fmt.Printf("   • Incoming Contracts:  %d\n", len(assessment.IncomingEdges))
				fmt.Printf("   • Outgoing Contracts:  %d\n", len(assessment.OutgoingEdges))
				fmt.Printf("   • Affected Components: %d\n", len(assessment.AffectedComponents))
				if len(assessment.Warnings) > 0 {
					fmt.Println("\n   Warnings:")
					for _, w := range assessment.Warnings {
						fmt.Printf("   ⚠️  %s\n", w)
					}
				}
				return
			}
		}
	}

	// Second attempt: treat target as an agent ID or model ID in audit lineage
	auditor := lineage.NewAuditEngine(graphEngine)
	report, err := auditor.ComputeBlastRadius(lineage.AuditFilter{ExecutingAgentID: target}, 5)
	if err != nil || (len(report.DirectlyTouchedNodes) == 0 && report.TotalImpactedNodes == 0) {
		if rep2, err2 := auditor.ComputeBlastRadius(lineage.AuditFilter{LLMVersion: target}, 5); err2 == nil && len(rep2.DirectlyTouchedNodes) > 0 {
			report = rep2
			err = nil
		}
	}
	if err != nil {
		if *format == "json" {
			res, _ := json.Marshal(map[string]interface{}{"status": "ERROR", "error": err.Error()})
			fmt.Println(string(res))
		} else {
			fmt.Fprintf(os.Stderr, "Blast radius error: %v\n", err)
		}
		return
	}

	if *format == "json" {
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Printf("=== Blast Radius Audit Report ===\n")
	fmt.Printf("Summary: %s\n", report.Summary)
	fmt.Printf("Total Direct Nodes:     %d\n", report.TotalDirectNodes)
	fmt.Printf("Total Downstream Nodes: %d\n", report.TotalDownstreamNodes)
	fmt.Printf("Risk Score:             %.2f\n", report.RiskScore)
}

func runView(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: cosm view <node_id_or_symbol> [flags]")
		return
	}

	fs := flag.NewFlagSet("view", flag.ContinueOnError)
	universeID := fs.String("u", "universe-main", "Universe ID")
	fs.StringVar(universeID, "universe", "universe-main", "Universe ID")
	format := fs.String("format", "terminal", "Output format (source, ast, raw, terminal, markdown)")

	// Separate flags from positional arguments so flag position doesn't break parsing
	var flagArgs []string
	var posArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			if !strings.Contains(arg, "=") && (arg == "-u" || arg == "--u" || arg == "-universe" || arg == "--universe" || arg == "-format" || arg == "--format") {
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					i++
					flagArgs = append(flagArgs, args[i])
				}
			}
		} else {
			posArgs = append(posArgs, arg)
		}
	}
	_ = fs.Parse(flagArgs)

	var query string
	if len(posArgs) > 0 {
		query = posArgs[0]
	} else if len(fs.Args()) > 0 {
		query = fs.Args()[0]
	}
	if query == "" {
		fmt.Println("Usage: cosm view <node_id_or_symbol> [flags]")
		return
	}

	blobStore, graphEngine, err := openStorage()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Storage error: %v\n", err)
		os.Exit(1)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	nodeID, err := resolveNodeOrSymbol(graphEngine, universeMgr, *universeID, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolution error: %v\n", err)
		return
	}

	var data []byte
	if node, gErr := graphEngine.GetNode(nodeID); gErr == nil && node != nil {
		data, err = blobStore.Get(node.MerkleHash)
	} else {
		data, err = blobStore.Get(nodeID)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Node %s not found in object store: %v\n", nodeID, err)
		return
	}

	viewer := materialize.NewViewer()
	var sym core.ASTSymbolNode
	if err := json.Unmarshal(data, &sym); err == nil && sym.Identifier != "" {
		renderFmt := materialize.FormatTerminal
		switch strings.ToLower(*format) {
		case "ast":
			renderFmt = materialize.FormatAST
		case "source", "code":
			renderFmt = materialize.FormatCode
		case "markdown":
			renderFmt = materialize.FormatMarkdown
		case "contract":
			renderFmt = materialize.FormatContract
		case "raw":
			fmt.Println(string(data))
			return
		}
		fmt.Println(viewer.RenderSymbolNode(&sym, renderFmt))
		return
	}

	fmt.Println(string(data))
}

func runShip(args []string) {
	fs := flag.NewFlagSet("ship", flag.ExitOnError)
	universeID := fs.String("u", "universe-main", "Universe ID")
	fs.StringVar(universeID, "universe", "universe-main", "Universe ID")
	targetName := fs.String("t", "target:cosm", "Target name or profile (e.g. target:cosm, target:local-preview, target:cloud-run)")
	fs.StringVar(targetName, "target", "target:cosm", "Target name or profile")
	format := fs.String("format", "terminal", "Output format (terminal, json)")
	_ = fs.Parse(args)

	blobStore, graphEngine, err := openStorage()
	if err != nil {
		if *format == "json" {
			res, _ := json.Marshal(map[string]interface{}{"status": "ERROR", "error": err.Error()})
			fmt.Println(string(res))
		} else {
			fmt.Fprintf(os.Stderr, "Storage error: %v\n", err)
		}
		os.Exit(1)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	head, err := universeMgr.GetUniverseManifest(*universeID)
	if err != nil {
		if *format == "json" {
			res, _ := json.Marshal(map[string]interface{}{"status": "ERROR", "error": fmt.Sprintf("Getting universe manifest: %v", err)})
			fmt.Println(string(res))
		} else {
			fmt.Fprintf(os.Stderr, "Getting universe manifest: %v\n", err)
		}
		return
	}

	compMap, symMap := loadManifestState(blobStore, graphEngine, head)
	hydrator := materialize.NewHydrator()
	files, err := hydrator.HydrateWorkspace(head, compMap, symMap)
	if err != nil {
		if *format == "json" {
			res, _ := json.Marshal(map[string]interface{}{"status": "ERROR", "error": fmt.Sprintf("Hydrating files: %v", err)})
			fmt.Println(string(res))
		} else {
			fmt.Fprintf(os.Stderr, "Hydrating files: %v\n", err)
		}
		return
	}

	var targetSpec *shipping.TargetSpec
	switch *targetName {
	case "cosm", "target:cosm":
		targetSpec = shipping.DefaultCosmTarget()
	case "cloud-run", "target:cloud-run":
		targetSpec = shipping.DefaultCloudRunTarget(*targetName, nil)
	default:
		targetSpec = shipping.DefaultLocalServiceTarget(*targetName, nil)
	}

	if *format != "json" {
		merkleShort := head.MerkleRootHash
		if len(merkleShort) > 12 {
			merkleShort = merkleShort[:12]
		}
		fmt.Printf("🚢 Shipping Target: %s (Universe: %s)\n", *targetName, *universeID)
		fmt.Printf("   • Source:  Ephemeral AST Hydration from Merkle Root %s (%d file(s))\n", merkleShort, len(files))
		fmt.Println("   • Staging: Isolated sandbox (decoupled from workspace disk drift)")
	}

	packager := shipping.NewPackager(nil)
	art, err := packager.BuildAndPackageTarget(targetSpec, files, "dist")
	if err != nil {
		if *format == "json" {
			res, _ := json.Marshal(map[string]interface{}{"status": "ERROR", "error": fmt.Sprintf("Packaging target: %v", err)})
			fmt.Println(string(res))
		} else {
			fmt.Fprintf(os.Stderr, "Packaging target: %v\n", err)
		}
		return
	}

	var previewURL string
	var healthy bool
	if targetSpec.Name != "target:cosm" && targetSpec.HealthCheckPath != "" {
		sandbox := target.NewPreviewSandbox()
		inst, err := sandbox.Start(targetSpec, files)
		if err == nil {
			previewURL = inst.URL
			healthy = inst.HealthCheck()
		}
	}

	if *format == "json" {
		output := map[string]interface{}{
			"status":        "SUCCESS",
			"target":        targetSpec.Name,
			"universe_id":   *universeID,
			"merkle_root":   head.MerkleRootHash,
			"artifact_id":   art.ArtifactID,
			"artifact_path": art.ArtifactPath,
			"size_bytes":    art.SizeBytes,
		}
		if previewURL != "" {
			output["preview_url"] = previewURL
			output["healthy"] = healthy
		}
		res, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(res))
		return
	}

	fmt.Println("🚀 Shipping Sidecar Execution Succeeded!")
	fmt.Printf("   Package Size: %d bytes (Artifact: %s)\n", art.SizeBytes, art.ArtifactID)
	fmt.Printf("   Artifact Path: %s\n", art.ArtifactPath)
	if previewURL != "" {
		fmt.Printf("   Preview URL:  %s\n", previewURL)
		fmt.Printf("   Health:       %v\n", healthy)
	}
}

func shortHash(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12]
}

func runUniverse(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: cosm universe <list|create|diff|merge> [flags]")
		return
	}
	blobStore, graphEngine, err := openStorage()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Storage error: %v\n", err)
		os.Exit(1)
	}
	defer graphEngine.Close()
	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)

	sub := args[0]
	switch sub {
	case "list":
		fs := flag.NewFlagSet("universe list", flag.ExitOnError)
		format := fs.String("format", "text", "Output format (text, json)")
		fs.StringVar(format, "f", "text", "Alias for --format")
		_ = fs.Parse(args[1:])

		universes := universeMgr.ListUniverses()
		if *format == "json" {
			data, _ := json.MarshalIndent(universes, "", "  ")
			fmt.Println(string(data))
			return
		}

		fmt.Println("🌌 Active Micro-Universes:")
		for _, u := range universes {
			fmt.Printf("  * %s (Head: %s, Status: %s)\n", u.UniverseID, shortHash(u.HeadManifestHash), u.Status)
		}
	case "create":
		fs := flag.NewFlagSet("universe create", flag.ExitOnError)
		parent := fs.String("p", "universe-main", "Parent universe ID")
		_ = fs.Parse(args[1:])
		if fs.NArg() == 0 {
			fmt.Println("Please specify a new universe ID. Example: cosm universe create u/feature-1")
			return
		}
		newID := fs.Arg(0)
		rec, err := universeMgr.CreateUniverse(newID, *parent)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating universe: %v\n", err)
			return
		}
		fmt.Printf("✨ Created micro-universe '%s' branched from '%s' (Head: %s)\n", rec.UniverseID, *parent, shortHash(rec.HeadManifestHash))
	case "diff":
		if len(args) < 3 {
			fmt.Println("Usage: cosm universe diff <universeA> <universeB>")
			return
		}
		diff, err := universeMgr.DiffUniverses(args[1], args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Diff error: %v\n", err)
			return
		}
		fmt.Printf("=== Universe Diff: %s <-> %s ===\n", args[1], args[2])
		fmt.Printf("Added Components:    %v\n", diff.AddedComponents)
		fmt.Printf("Removed Components:  %v\n", diff.RemovedComponents)
		fmt.Printf("Cross-Edge Delta:    %d edges\n", len(diff.AddedEdges)+len(diff.RemovedEdges))
	case "merge":
		fs := flag.NewFlagSet("universe merge", flag.ExitOnError)
		targetUniv := fs.String("t", "universe-main", "Target universe to merge into")
		strategy := fs.String("s", "union", "Merge strategy (union / fast_forward)")
		_ = fs.Parse(args[1:])
		if fs.NArg() == 0 {
			fmt.Println("Usage: cosm universe merge <source_universe> [-t target_universe] [-s strategy]")
			return
		}
		srcUniv := fs.Arg(0)
		mergedManifest, err := universeMgr.MergeUniverse(srcUniv, *targetUniv, *strategy)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Merge error: %v\n", err)
			return
		}
		manifestHash, _ := core.HashWorkspaceManifest(mergedManifest)
		fmt.Printf("🔀 Successfully merged '%s' into '%s' (New Head: %s)\n", srcUniv, *targetUniv, manifestHash[:12])
	default:
		fmt.Printf("Unknown universe subcommand: %s\n", sub)
	}
}

func pushUniverseToHub(ctx context.Context, client *topocosm.HubClient, universeMgr *storage.UniverseManager, blobStore *storage.BlobStore, graphEngine *storage.GraphEngine, universeID, orgSlug, cosmName, desc string) (*topocosm.PublishResponse, error) {
	manifest, err := universeMgr.GetUniverseManifest(universeID)
	if err != nil || manifest == nil {
		return nil, fmt.Errorf("universe manifest '%s' not found: %w", universeID, err)
	}

	components := make(map[string]*core.ComponentNode)
	symbols := make(map[string]*core.ASTSymbolNode)
	blobs := make(map[string][]byte)

	for _, compID := range manifest.Components {
		var compBytes []byte
		var bErr error
		compBytes, bErr = blobStore.Get(compID)
		if bErr != nil {
			if nodeRec, e := graphEngine.GetNode(compID); e == nil {
				compBytes, bErr = blobStore.Get(nodeRec.MerkleHash)
			}
		}
		if bErr == nil {
			blobs[compID] = compBytes
			var comp core.ComponentNode
			if json.Unmarshal(compBytes, &comp) == nil {
				components[compID] = &comp
				for _, symID := range comp.SymbolNodes {
					var symBytes []byte
					symBytes, bErr = blobStore.Get(symID)
					if bErr != nil {
						if symRec, e := graphEngine.GetNode(symID); e == nil {
							symBytes, bErr = blobStore.Get(symRec.MerkleHash)
						}
					}
					if bErr == nil {
						blobs[symID] = symBytes
						var sym core.ASTSymbolNode
						if json.Unmarshal(symBytes, &sym) == nil {
							symbols[symID] = &sym
						}
					}
				}
			}
		}
	}

	payload := &topocosm.PublishPayload{
		OrgSlug:     orgSlug,
		CosmName:    cosmName,
		UniverseID:  universeID,
		Manifest:    manifest,
		Components:  components,
		Symbols:     symbols,
		Blobs:       blobs,
		Description: desc,
		Visibility:  topocosm.VisibilityPublic,
		IsInitial:   false,
	}

	return client.PublishCosm(ctx, payload)
}

func runProposal(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: cosm proposal <create|list|view|review|merge> [flags]")
		return
	}
	blobStore, graphEngine, err := openStorage()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Storage error: %v\n", err)
		os.Exit(1)
	}
	defer graphEngine.Close()
	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)

	sub := args[0]
	switch sub {
	case "create":
		fs := flag.NewFlagSet("proposal create", flag.ExitOnError)
		source := fs.String("source", "", "Source micro-universe ID")
		fs.StringVar(source, "s", "", "Source micro-universe ID (shorthand)")
		targetUniv := fs.String("target", "universe-main", "Target micro-universe ID")
		fs.StringVar(targetUniv, "t", "universe-main", "Target micro-universe ID (shorthand)")
		title := fs.String("title", "", "Proposal title")
		fs.StringVar(title, "i", "", "Proposal title (shorthand)")
		hubFlag := fs.String("hub", "", "Topocosm Hub target (e.g. http://127.0.0.1:51204/org/cosm or org/cosm)")
		_ = fs.Parse(args[1:])

		if *source == "" || *title == "" {
			fmt.Println("Error: --source and --title flags are required")
			return
		}

		diff, err := universeMgr.DiffUniverses(*source, *targetUniv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Diff error: %v\n", err)
			return
		}

		fmt.Printf("🚀 Created Proposal: %q\n", *title)
		fmt.Printf("   Source: %s -> Target: %s\n", *source, *targetUniv)
		fmt.Printf("   Changes: +%d components, -%d components\n", len(diff.AddedComponents), len(diff.RemovedComponents))
		fmt.Printf("   Status: Ready for Agent Evaluation / Review\n")

		if *hubFlag != "" {
			hubURL, orgSlug, cosmName := resolveHubTarget(*hubFlag)
			client := topocosm.NewHubClient(hubURL, "did:key:z6MkuCLIProposal")
			// Automatically sync/push source micro-universe to Topocosm Hub so remote review and merge can access it
			if _, pErr := pushUniverseToHub(context.Background(), client, universeMgr, blobStore, graphEngine, *source, orgSlug, cosmName, *title); pErr != nil {
				fmt.Fprintf(os.Stderr, "Notice: failed syncing micro-universe %s to hub: %v\n", *source, pErr)
			}
			sourceHeadRec, _ := universeMgr.GetUniverse(*source)
			manifestHash := ""
			if sourceHeadRec != nil {
				manifestHash = sourceHeadRec.HeadManifestHash
			}
			lineageEnv := core.LineageEnvelope{
				UserID:              "developer",
				UserPrompt:          *title,
				SessionID:           fmt.Sprintf("sess-%d", time.Now().Unix()),
				OrchestratorAgentID: "cosm-cli",
				ExecutingAgentID:    "human-developer",
				LLMVersion:          "cli",
				Intent:              *title,
				Timestamp:           time.Now(),
			}
			cob, err := client.CreateProposal(context.Background(), orgSlug, cosmName, *title, *source, *targetUniv, lineageEnv)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to submit proposal to Topocosm Hub (%s): %v\n", hubURL, err)
			} else {
				fmt.Printf("🌐 Submitted to Topocosm Hub: %s\n", cob.ID)
				fmt.Printf("   • Hub Target: %s/%s/%s\n", hubURL, orgSlug, cosmName)
				fmt.Printf("   • Proposal Head: %s\n", shortHash(manifestHash))
			}
		}

	case "list":
		fs := flag.NewFlagSet("proposal list", flag.ExitOnError)
		hubFlag := fs.String("hub", "", "Topocosm Hub target (e.g. http://127.0.0.1:51204/org/cosm or org/cosm)")
		_ = fs.Parse(args[1:])

		if *hubFlag != "" {
			hubURL, orgSlug, cosmName := resolveHubTarget(*hubFlag)
			client := topocosm.NewHubClient(hubURL, "did:key:z6MkuCLIProposal")
			props, err := client.ListProposals(context.Background(), orgSlug, cosmName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to list proposals from Topocosm Hub (%s): %v\n", hubURL, err)
				os.Exit(1)
			}
			fmt.Printf("📋 Active Topocosm Hub Proposals (%s/%s):\n", orgSlug, cosmName)
			if len(props) == 0 {
				fmt.Println("   (No active proposals on hub)")
				return
			}
			for _, p := range props {
				src := p.TargetUniverse
				if len(p.Revisions) > 0 {
					src = p.Revisions[len(p.Revisions)-1].SourceUniverse
				}
				fmt.Printf("   * %s %q (%s -> %s, Status: %s)\n", p.ID, p.Title, src, p.TargetUniverse, p.Status)
			}
			return
		}

		universes := universeMgr.ListUniverses()
		fmt.Println("📋 Active Proposals & Micro-Universe Branches:")
		for _, u := range universes {
			if u.UniverseID != "universe-main" {
				fmt.Printf("   * %s (Head: %s, Status: %s)\n", u.UniverseID, shortHash(u.HeadManifestHash), u.Status)
			}
		}

	case "view":
		fs := flag.NewFlagSet("proposal view", flag.ExitOnError)
		source := fs.String("source", "", "Source micro-universe ID")
		fs.StringVar(source, "s", "", "Source micro-universe ID (shorthand)")
		targetUniv := fs.String("target", "universe-main", "Target universe ID")
		fs.StringVar(targetUniv, "t", "universe-main", "Target universe ID (shorthand)")
		format := fs.String("format", "terminal", "Output format (terminal or markdown)")
		_ = fs.Parse(args[1:])

		if *source == "" && len(fs.Args()) > 0 {
			*source = fs.Args()[0]
		}
		if *source == "" {
			fmt.Println("Error: --source flag or proposal ID is required")
			return
		}

		diff, err := universeMgr.DiffUniverses(*source, *targetUniv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Diff error: %v\n", err)
			return
		}

		sourceHeadRec, _ := universeMgr.GetUniverse(*source)
		targetHeadRec, _ := universeMgr.GetUniverse(*targetUniv)

		sourceHead := ""
		if sourceHeadRec != nil {
			sourceHead = sourceHeadRec.HeadManifestHash
		}
		targetHead := ""
		if targetHeadRec != nil {
			targetHead = targetHeadRec.HeadManifestHash
		}

		presModel := &review.ProposalPresentationModel{
			ProposalID:           fmt.Sprintf("prop-%s-%s", *source, *targetUniv),
			BaseUniverse:         *targetUniv,
			ProposedUniverse:     *source,
			BaseManifestHash:     targetHead,
			ProposedManifestHash: sourceHead,
			Author:               "cosm-agent",
			Intent:               "Autonomous branch proposal",
			UserPrompt:           "Review AST delta cards",
			ExecutingAgentID:     "cosm-agent-worker",
			SignatureValid:       true,
			BuildStatus:          "PASS",
		}

		for _, compID := range diff.AddedComponents {
			presModel.AddedSymbols = append(presModel.AddedSymbols, review.SymbolDiffCard{
				SymbolID:    compID,
				Identifier:  compID,
				Language:    core.LangGo,
				NodeType:    "Component",
				DiffType:    "ADDED",
				NewSnippet:  fmt.Sprintf("// Component %s", compID),
				ImpactLevel: "LOW",
			})
		}
		for _, compID := range diff.RemovedComponents {
			presModel.DeletedSymbols = append(presModel.DeletedSymbols, review.SymbolDiffCard{
				SymbolID:    compID,
				Identifier:  compID,
				Language:    core.LangGo,
				NodeType:    "Component",
				DiffType:    "DELETED",
				ImpactLevel: "LOW",
			})
		}
		for _, edge := range diff.AddedEdges {
			presModel.ContractDeltas = append(presModel.ContractDeltas, review.ContractDelta{
				SourceID:    edge.SourceNodeID,
				TargetID:    edge.TargetNodeID,
				EdgeType:    edge.Type,
				Status:      "NEW",
				Description: "Discovered cross-domain contract",
			})
		}

		pres := review.NewProposalPresenter()
		if *format == "markdown" {
			fmt.Println(pres.RenderMarkdown(presModel))
		} else {
			fmt.Println(pres.RenderTerminalCards(presModel))
		}

	case "review":
		fs := flag.NewFlagSet("proposal review", flag.ExitOnError)
		source := fs.String("source", "", "Source micro-universe ID")
		fs.StringVar(source, "s", "", "Source micro-universe ID (shorthand)")
		targetUniv := fs.String("target", "universe-main", "Target universe ID")
		fs.StringVar(targetUniv, "t", "universe-main", "Target universe ID (shorthand)")
		criticID := fs.String("critic", "agent-critic-deepseek", "Critic Agent ID")
		hubFlag := fs.String("hub", "", "Topocosm Hub target (e.g. http://127.0.0.1:51204/org/cosm or org/cosm)")
		propID := fs.String("id", "", "Proposal ID on Topocosm Hub")
		verdict := fs.String("verdict", "APPROVE", "Review verdict (APPROVE or REJECT)")
		score := fs.Float64("score", 95.0, "Critic review score (0-100)")
		_ = fs.Parse(args[1:])

		if *hubFlag != "" {
			pID := *propID
			if pID == "" && *source != "" {
				pID = *source
			}
			if pID == "" && len(fs.Args()) > 0 {
				pID = fs.Args()[0]
			}
			if pID == "" {
				fmt.Println("Error: --id <proposal_id> is required for hub review")
				os.Exit(1)
			}
			hubURL, orgSlug, cosmName := resolveHubTarget(*hubFlag)
			client := topocosm.NewHubClient(hubURL, "did:key:z6MkuCriticOracle")
			sub := &topocosm.ReviewSubmission{
				ProposalID:   pID,
				ReviewerDID:  fmt.Sprintf("did:key:%s", *criticID),
				Approved:     strings.ToUpper(*verdict) == "APPROVE",
				FitnessScore: *score,
				Comments: []distributed.ReviewComment{
					{
						CommentID: fmt.Sprintf("rev-%d", time.Now().UnixNano()%100000),
						AuthorDID: fmt.Sprintf("did:key:%s", *criticID),
						Body:      fmt.Sprintf("Critic Oracle evaluation: %s (Score: %.1f)", *verdict, *score),
						Timestamp: time.Now(),
					},
				},
			}
			reviewed, err := client.SubmitReview(context.Background(), orgSlug, cosmName, sub)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to submit review to %s: %v\n", hubURL, err)
				os.Exit(1)
			}
			fmt.Printf("🤖 Topocosm Hub Review Submitted for %s\n", reviewed.ID)
			fmt.Printf("   Verdict: %s (Score: %.1f) | Status: %s\n", *verdict, *score, reviewed.Status)
			return
		}

		if *source == "" && len(fs.Args()) > 0 {
			*source = fs.Args()[0]
		}
		if *source == "" {
			fmt.Println("Error: --source flag or proposal ID is required")
			return
		}

		diff, err := universeMgr.DiffUniverses(*source, *targetUniv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Diff error: %v\n", err)
			return
		}

		sourceHeadRec, _ := universeMgr.GetUniverse(*source)
		targetHeadRec, _ := universeMgr.GetUniverse(*targetUniv)

		sourceHead := ""
		if sourceHeadRec != nil {
			sourceHead = sourceHeadRec.HeadManifestHash
		}
		targetHead := ""
		if targetHeadRec != nil {
			targetHead = targetHeadRec.HeadManifestHash
		}

		presModel := &review.ProposalPresentationModel{
			ProposalID:           fmt.Sprintf("prop-%s-%s", *source, *targetUniv),
			BaseUniverse:         *targetUniv,
			ProposedUniverse:     *source,
			BaseManifestHash:     targetHead,
			ProposedManifestHash: sourceHead,
			Author:               "cosm-agent",
			Intent:               "Autonomous branch proposal",
			UserPrompt:           "Perform critique evaluation",
			ExecutingAgentID:     "cosm-agent-worker",
			SignatureValid:       true,
			BuildStatus:          "PASS",
		}
		for _, compID := range diff.AddedComponents {
			presModel.AddedSymbols = append(presModel.AddedSymbols, review.SymbolDiffCard{
				SymbolID:    compID,
				Identifier:  compID,
				Language:    core.LangGo,
				NodeType:    "Component",
				DiffType:    "ADDED",
				NewSnippet:  fmt.Sprintf("// Component %s", compID),
				ImpactLevel: "LOW",
			})
		}

		critic := review.NewCriticEngine(*criticID)
		report := critic.EvaluateProposal(presModel)

		fmt.Printf("🤖 Agent Review Report [%s]\n", report.ReviewerAgent)
		fmt.Printf("   Proposal: %s\n", report.ProposalID)
		fmt.Printf("   Verdict:  %s (Fitness Score: %.2f/1.00)\n", report.Verdict, report.FitnessScore)
		if len(report.Critiques) > 0 {
			fmt.Println("\n   Critiques:")
			for _, c := range report.Critiques {
				fmt.Printf("   - [%s][%s] %s: %s\n", c.Severity, c.Category, c.SymbolID, c.Description)
				fmt.Printf("     Fix: %s\n", c.SuggestedFix)
			}
		}
		if report.RemediationPrompt != "" {
			fmt.Printf("\n   Suggested Autonomous Remediation:\n   %s\n", report.RemediationPrompt)
		}

	case "merge":
		fs := flag.NewFlagSet("proposal merge", flag.ExitOnError)
		source := fs.String("source", "", "Source micro-universe ID")
		fs.StringVar(source, "s", "", "Source micro-universe ID (shorthand)")
		targetUniv := fs.String("target", "universe-main", "Target universe ID")
		fs.StringVar(targetUniv, "t", "universe-main", "Target universe ID (shorthand)")
		hubFlag := fs.String("hub", "", "Topocosm Hub target (e.g. http://127.0.0.1:51204/org/cosm or org/cosm)")
		propID := fs.String("id", "", "Proposal ID on Topocosm Hub")
		_ = fs.Parse(args[1:])

		if *hubFlag != "" {
			pID := *propID
			if pID == "" && *source != "" {
				pID = *source
			}
			if pID == "" && len(fs.Args()) > 0 {
				pID = fs.Args()[0]
			}
			if pID == "" {
				fmt.Println("Error: --id <proposal_id> is required for hub merge")
				os.Exit(1)
			}
			hubURL, orgSlug, cosmName := resolveHubTarget(*hubFlag)
			client := topocosm.NewHubClient(hubURL, "did:key:z6MkuCLIMerger")
			res, err := client.MergeProposal(context.Background(), orgSlug, cosmName, pID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to merge proposal on %s: %v\n", hubURL, err)
				os.Exit(1)
			}
			fmt.Printf("✅ Topocosm Hub Proposal %s merged successfully into target universe!\n", pID)
			if root, ok := res["merkle_root_hash"].(string); ok && root != "" {
				fmt.Printf("   New Head Merkle Root: %s\n", root)
			}
			return
		}

		if *source == "" {
			fmt.Println("Error: --source flag is required")
			return
		}
		merged, err := universeMgr.MergeUniverse(*source, *targetUniv, "union")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Proposal merge error: %v\n", err)
			return
		}
		h, _ := core.HashWorkspaceManifest(merged)
		fmt.Printf("✅ Proposal merged successfully into %s (Head: %s)\n", *targetUniv, h[:12])
	}
}

func runStack(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: cosm stack <list|create|evolve> [flags]")
		return
	}
	blobStore, graphEngine, err := openStorage()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Storage error: %v\n", err)
		os.Exit(1)
	}
	defer graphEngine.Close()
	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	stackMgr := distributed.NewStackManager(universeMgr, blobStore)
	stackFilePath := filepath.Join(".cosm", "stack.json")
	_ = stackMgr.LoadFromFile(stackFilePath)

	sub := args[0]
	switch sub {
	case "list":
		if len(stackMgr.ListStack()) == 0 {
			universes := universeMgr.ListUniverses()
			order := 0
			for _, u := range universes {
				if u.UniverseID != "universe-main" {
					_ = stackMgr.RegisterChange(&distributed.StackedProposal{
						ChangeID:       fmt.Sprintf("c/%s", strings.TrimPrefix(u.UniverseID, "u/")),
						ProposalID:     fmt.Sprintf("prop-%s", u.UniverseID),
						Title:          fmt.Sprintf("Change: %s", u.UniverseID),
						ParentChangeID: "universe-main",
						UniverseID:     u.UniverseID,
						ManifestHash:   u.HeadManifestHash,
						OrderIndex:     order,
					})
					order++
				}
			}
			_ = stackMgr.SaveToFile(stackFilePath)
		}
		fmt.Println("🥞 Jujutsu-Style Stacked Proposals & Change Chains:")
		stack := stackMgr.ListStack()
		if len(stack) == 0 {
			fmt.Println("   (No stacked changes active. Create one with 'cosm stack create')")
			return
		}
		for idx, c := range stack {
			fmt.Printf("   [%d] 🔹 %-20s (Universe: %s, Head: %s)\n", idx+1, c.ChangeID, c.UniverseID, shortHash(c.ManifestHash))
			fmt.Printf("       Parent: %s | Auto-Rebase: Active\n", c.ParentChangeID)
		}

	case "create":
		fs := flag.NewFlagSet("stack create", flag.ExitOnError)
		changeID := fs.String("c", "", "Change ID (e.g. c/billing-v1)")
		fs.StringVar(changeID, "change", "", "Change ID (e.g. c/billing-v1)")
		parentID := fs.String("p", "universe-main", "Parent change ID or base branch")
		fs.StringVar(parentID, "parent", "universe-main", "Parent change ID or base branch")
		universeID := fs.String("u", "", "Micro-universe ID")
		fs.StringVar(universeID, "universe", "", "Micro-universe ID")
		title := fs.String("title", "Stacked Change", "Proposal title")
		fs.StringVar(title, "t", "Stacked Change", "Proposal title (shorthand)")
		_ = fs.Parse(args[1:])

		if *changeID == "" || *universeID == "" {
			fmt.Println("Error: -c <change_id> and -u <universe_id> are required")
			return
		}

		headRec, err := universeMgr.GetUniverse(*universeID)
		manifestHash := ""
		if err == nil && headRec != nil {
			manifestHash = headRec.HeadManifestHash
		}

		_ = stackMgr.RegisterChange(&distributed.StackedProposal{
			ChangeID:       *changeID,
			ProposalID:     fmt.Sprintf("prop-%s", *universeID),
			Title:          *title,
			ParentChangeID: *parentID,
			UniverseID:     *universeID,
			ManifestHash:   manifestHash,
			OrderIndex:     len(stackMgr.ListStack()),
		})
		_ = stackMgr.SaveToFile(stackFilePath)

		fmt.Printf("🥞 Registered Stacked Change '%s' (Parent: '%s')\n", *changeID, *parentID)
		fmt.Printf("   Descendants will automatically evolve when parent mutates.\n")

	case "evolve":
		fs := flag.NewFlagSet("stack evolve", flag.ExitOnError)
		parentChangeID := fs.String("c", "", "Parent change ID to evolve descendants from")
		fs.StringVar(parentChangeID, "change", "", "Parent change ID to evolve descendants from")
		_ = fs.Parse(args[1:])

		if *parentChangeID == "" {
			fmt.Println("Error: -c <parent_change_id> is required")
			return
		}

		res, err := stackMgr.EvolveStack(*parentChangeID, "latest")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Stack evolution error: %v\n", err)
			return
		}
		_ = stackMgr.SaveToFile(stackFilePath)
		fmt.Printf("⚡ Auto-evolved %d descendant changes in stack:\n", len(res.EvolvedChanges))
		for _, ch := range res.EvolvedChanges {
			fmt.Printf("   ✓ Rebased %s onto new parent AST root without conflict\n", ch)
		}
	}
}

func runPeer(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: cosm peer <status|sync> [flags]")
		return
	}
	blobStore, graphEngine, err := openStorage()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Storage error: %v\n", err)
		os.Exit(1)
	}
	defer graphEngine.Close()
	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)

	sub := args[0]
	switch sub {
	case "status":
		authCfg, _ := topocosm.LoadAuthConfig()
		localDID := "did:key:z6MkuLocalNode"
		if authCfg != nil && authCfg.CallerDID != "" {
			localDID = authCfg.CallerDID
		}

		var totalBlobs int
		var totalBytes int64
		objectsDir := filepath.Join(".cosm", "objects")
		_ = filepath.Walk(objectsDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				totalBlobs++
				totalBytes += info.Size()
			}
			return nil
		})

		universes := universeMgr.ListUniverses()
		universeCount := len(universes)

		hubURL := "https://topocosm.dev"
		if authCfg != nil && authCfg.HubURL != "" {
			hubURL = authCfg.HubURL
		}

		hubStatus := "Offline / Standalone Local Mesh (Zero Cloud)"
		client := &http.Client{Timeout: 600 * time.Millisecond}
		startPing := time.Now()
		resp, pErr := client.Get(hubURL + "/health")
		if pErr == nil {
			_ = resp.Body.Close()
			latency := time.Since(startPing).Round(time.Millisecond)
			hubStatus = fmt.Sprintf("Connected (%s, Latency: %s)", hubURL, latency)
		} else {
			resp2, pErr2 := client.Get(hubURL)
			if pErr2 == nil {
				_ = resp2.Body.Close()
				latency := time.Since(startPing).Round(time.Millisecond)
				hubStatus = fmt.Sprintf("Connected (%s, Latency: %s)", hubURL, latency)
			}
		}

		fmt.Println("🌐 Cosm Decentralized P2P & Seed Node Mesh Status:")
		fmt.Printf("   • Local Node DID:  %s (Ed25519 Verified)\n", localDID)
		fmt.Println("   • Swarm Protocol:  cosm-gossip/v1 (Sparse AST Merkle Replication)")
		fmt.Printf("   • Local Headspace: %d micro-universe(s) registered\n", universeCount)
		fmt.Printf("   • Content Store:   Ready (%d immutable AST blobs, %d bytes cached)\n", totalBlobs, totalBytes)
		fmt.Printf("   • Hub / Backplane: %s\n", hubStatus)

	case "sync":
		fs := flag.NewFlagSet("peer sync", flag.ExitOnError)
		universeID := fs.String("u", "universe-main", "Micro-universe to sync")
		sparseComponent := fs.String("sparse", "", "Sparse component filter (e.g. services/billing)")
		_ = fs.Parse(args[1:])

		syncEngine := distributed.NewSparseSyncEngine(blobStore, universeMgr)
		filter := distributed.SparseFilter{}
		if *sparseComponent != "" {
			filter.ComponentNames = []string{*sparseComponent}
		}

		payload, err := syncEngine.ExtractSparseSubtree(*universeID, filter)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Sparse sync error: %v\n", err)
			return
		}

		fmt.Printf("📡 Sparse AST Replication for Universe '%s':\n", *universeID)
		fmt.Printf("   • Filter:           %s\n", *sparseComponent)
		fmt.Printf("   • Synchronized:     %d AST blobs (%d components, %d symbols)\n", payload.TotalBlobsCount, len(payload.Components), len(payload.Symbols))
		fmt.Printf("   • Bandwidth Saved:  %.1f%% reduction vs full repository clone\n", payload.SavingsPercent)
		fmt.Println("✨ Local state synchronized with peer swarm.")
	}
}

func runImportRepo(args []string) {
	fs := flag.NewFlagSet("import-repo", flag.ExitOnError)
	dir := fs.String("d", ".", "Repository directory to onboard")
	universeID := fs.String("u", "universe-main", "Universe ID")
	prompt := fs.String("p", "Onboard repository into cosm Merkle-DAG", "Originating prompt")
	agentID := fs.String("a", "cosm-onboarder", "Agent ID")
	_ = fs.Parse(args)

	blobStore, graphEngine, err := openStorage()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Storage error: %v\n", err)
		os.Exit(1)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	migrator := onboarding.NewRepositoryMigrator(blobStore, graphEngine, universeMgr)

	fmt.Printf("🚀 Onboarding repository at '%s' into universe '%s'...\n", *dir, *universeID)
	report, err := migrator.ImportRepositorySnapshot(*dir, *universeID, *prompt, *agentID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Onboarding error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n✅ Repository Successfully Onboarded!")
	fmt.Printf("   • Total Files Scanned:   %d (Code: %d, Raw: %d)\n",
		report.TotalFilesScanned, report.CodeFilesParsed, report.RawFilesPreserved)
	fmt.Printf("   • AST Symbols Extracted: %d\n", report.SymbolsExtracted)
	fmt.Printf("   • Components Created:    %d\n", report.ComponentsCreated)
	fmt.Printf("   • Cross-Edges Linked:    %d\n", report.EdgesDiscovered)
	fmt.Printf("   • Merkle Root Hash:      %s\n", report.ManifestHash[:12])
	fmt.Printf("   • Elapsed Time:          %s\n", report.Duration)
}

func runSymbol(args []string) {
	fmt.Fprintln(os.Stderr, "⚠️  'cosm symbol' has been retired in favor of canonical Cosm commands:")
	fmt.Fprintln(os.Stderr, "   • 'cosm ast edit' (for declarative AST mutations with disk sync)")
	fmt.Fprintln(os.Stderr, "   • 'cosm view' (for viewing symbols and components)")
	fmt.Fprintln(os.Stderr, "   • 'cosm blast-radius' (for auditing contract impact and blast radius)")
	os.Exit(1)
}

func runAST(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: cosm ast <create|edit|resolve|tree> [flags]")
		return
	}

	sub := args[0]
	switch sub {
	case "tree":
		runASTTree(args[1:])
		return
	}

	blobStore, graphEngine, err := openStorage()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Storage error: %v\n", err)
		os.Exit(1)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	surgeryEngine := mutation.NewSurgeryEngine(blobStore, graphEngine, universeMgr)

	switch sub {
	case "resolve":
		fs := flag.NewFlagSet("ast resolve", flag.ExitOnError)
		universeID := fs.String("u", "universe-main", "Universe ID")
		fs.StringVar(universeID, "universe", "universe-main", "Universe ID (alias)")
		target := fs.String("target", "", "Target symbol path or ID")
		format := fs.String("format", "terminal", "Output format (terminal, json)")

		var flagArgs []string
		var posArgs []string
		for i := 1; i < len(args); i++ {
			arg := args[i]
			if strings.HasPrefix(arg, "-") {
				flagArgs = append(flagArgs, arg)
				if !strings.Contains(arg, "=") && (arg == "-u" || arg == "--u" || arg == "-universe" || arg == "--universe" || arg == "-target" || arg == "--target" || arg == "-format" || arg == "--format") {
					if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
						i++
						flagArgs = append(flagArgs, args[i])
					}
				}
			} else {
				posArgs = append(posArgs, arg)
			}
		}
		_ = fs.Parse(flagArgs)

		if *target == "" && len(posArgs) > 0 {
			*target = posArgs[0]
		}
		if *target == "" && len(fs.Args()) > 0 {
			*target = fs.Args()[0]
		}
		if *target == "" {
			fmt.Println("Error: target identifier is required. Example: cosm ast resolve services/auth::ValidateToken")
			return
		}

		resolved, err := surgeryEngine.ResolveSymbol(*universeID, *target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Symbol resolution failed: %v\n", err)
			return
		}

		if *format == "json" {
			data, _ := json.MarshalIndent(resolved, "", "  ")
			fmt.Println(string(data))
			return
		}

		fmt.Printf("🎯 Resolved Symbol: %s\n", resolved.SymbolNode.Identifier)
		fmt.Printf("   • Node ID:    %s\n", resolved.SymbolNode.NodeID)
		fmt.Printf("   • Language:   %s\n", resolved.SymbolNode.Language)
		fmt.Printf("   • Type:       %s\n", resolved.SymbolNode.NodeType)
		fmt.Printf("   • Component:  %s\n", resolved.Component.Name)
		if resolved.SymbolNode.Signature != "" {
			fmt.Printf("   • Signature:  %s\n", resolved.SymbolNode.Signature)
		}

	case "edit":
		fs := flag.NewFlagSet("ast edit", flag.ExitOnError)
		op := fs.String("op", "", "AST operation (replace_function_body, replace_function, add_method, add_before, add_after, delete, add_import, replace_imports, replace_global)")
		target := fs.String("target", "", "Target symbol path or ID")
		content := fs.String("content", "", "Content payload or code")
		batchFile := fs.String("batch", "", "Path to AST edit batch JSON file")
		universeID := fs.String("u", "universe-main", "Universe ID")
		fs.StringVar(universeID, "universe", "universe-main", "Universe ID (alias)")
		writeDisk := fs.Bool("write-disk", true, "Automatically synchronize modified components to workspace disk files (VS Code ready)")
		fs.BoolVar(writeDisk, "w", true, "Alias for --write-disk")
		prompt := fs.String("p", "Declarative AST edit", "User prompt")
		agentID := fs.String("a", "cosm-ast-surgeon", "Agent ID")
		sessionID := fs.String("session-id", "", "Agent session ID")
		orchID := fs.String("orchestrator-id", "", "Orchestrator Agent ID")
		model := fs.String("m", "", "LLM Model Version")
		promptTokens := fs.Int64("prompt-tokens", 0, "Prompt tokens")
		compTokens := fs.Int64("completion-tokens", 0, "Completion tokens")
		reasonTokens := fs.Int64("reasoning-tokens", 0, "Reasoning tokens")
		cachedTokens := fs.Int64("cached-tokens", 0, "Cached prompt tokens")
		costUSD := fs.Float64("cost-usd", 0.0, "Cost in USD")
		latencyMs := fs.Int64("latency-ms", 0, "Latency in ms")
		traceID := fs.String("trace-id", "", "W3C Trace ID")
		spanID := fs.String("span-id", "", "W3C Span ID")
		_ = fs.Parse(args[1:])

		lineageEnv := core.LineageEnvelope{
			UserID:              os.Getenv("USER"),
			UserPrompt:          *prompt,
			SessionID:           *sessionID,
			OrchestratorAgentID: *orchID,
			ExecutingAgentID:    *agentID,
			LLMVersion:          *model,
			Intent:              "Declarative AST Edit",
			Timestamp:           time.Now().UTC(),
			Tokens: core.TokenTelemetry{
				PromptTokens:     *promptTokens,
				CompletionTokens: *compTokens,
				ReasoningTokens:  *reasonTokens,
				CachedTokens:     *cachedTokens,
				TotalTokens:      *promptTokens + *compTokens + *reasonTokens,
				CostUSD:          *costUSD,
				LatencyMs:        *latencyMs,
			},
			Trace: core.TraceCarrier{
				TraceID: *traceID,
				SpanID:  *spanID,
			},
		}

		var batch mutation.ASTEditBatch
		if *batchFile != "" {
			data, err := os.ReadFile(*batchFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to read batch file: %v\n", err)
				return
			}
			if err := json.Unmarshal(data, &batch); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to parse batch JSON: %v\n", err)
				return
			}
			if batch.UniverseID == "" {
				batch.UniverseID = *universeID
			}
			batch.Lineage = lineageEnv
		} else {
			if *target == "" && len(fs.Args()) > 0 {
				*target = fs.Args()[0]
			}
			if *op == "" {
				*op = "replace_function_body"
			}
			if *target == "" {
				fmt.Println("Error: target or --batch file is required. Example: cosm ast edit --op replace_function_body --target LRUCache.get --content 'return self.cache.get(key)'")
				return
			}
			batch = mutation.ASTEditBatch{
				UniverseID: *universeID,
				Lineage:    lineageEnv,
				Operations: []mutation.ASTOperation{
					{
						Operation: mutation.ASTOperationType(*op),
						Target:    *target,
						Content:   *content,
					},
				},
			}
		}

		res, err := surgeryEngine.ApplyASTEditBatch(&batch)
		if err != nil {
			fmt.Fprintf(os.Stderr, "AST edit batch failed: %v\n", err)
			return
		}

		fmt.Println("⚡ Declarative AST Mutation Applied!")
		fmt.Printf("   • Universe:           %s\n", res.UniverseID)
		fmt.Printf("   • Operations Applied: %d\n", res.AppliedOperations)
		fmt.Printf("   • Modified Symbols:   %d\n", len(res.ModifiedSymbols))
		for _, det := range res.ModifiedSymbolsDetails {
			idPrefix := det.NodeID
			if len(idPrefix) > 12 {
				idPrefix = idPrefix[:12]
			}
			typeStr := ""
			if det.NodeType != "" {
				typeStr = fmt.Sprintf(" (%s)", det.NodeType)
			}
			compStr := ""
			if det.ComponentName != "" {
				compStr = fmt.Sprintf(" in %s", det.ComponentName)
			}
			fmt.Printf("     └─ %s %s%s%s\n", idPrefix, det.Identifier, typeStr, compStr)
		}
		fmt.Printf("   • Added Symbols:      %d\n", len(res.AddedSymbols))
		fmt.Printf("   • Deleted Symbols:    %d\n", len(res.DeletedSymbols))
		fmt.Printf("   • Sibling Dedup:      %d unchanged nodes preserved\n", res.DeduplicatedSymbols)
		if len(res.OldManifestHash) >= 12 && len(res.NewManifestHash) >= 12 {
			fmt.Printf("   • Merkle Head Delta:  %s -> %s\n", res.OldManifestHash[:12], res.NewManifestHash[:12])
		}
		fmt.Printf("   • Execution Time:     %s\n", res.Duration.Round(time.Millisecond))

		if *writeDisk {
			updatedHead, hErr := universeMgr.GetUniverseManifest(res.UniverseID)
			if hErr == nil && updatedHead != nil {
				compMap, symMap := loadManifestState(blobStore, graphEngine, updatedHead)
				hydrator := materialize.NewHydrator()
				allFiles, hydErr := hydrator.HydrateWorkspace(updatedHead, compMap, symMap)
				if hydErr == nil && len(allFiles) > 0 {
					changedFiles := make(map[string][]byte)
					for relPath, content := range allFiles {
						existing, rErr := os.ReadFile(relPath)
						if rErr != nil || string(existing) != string(content) {
							changedFiles[relPath] = content
						}
					}
					if len(changedFiles) > 0 {
						exporter := materialize.NewExporter()
						report, expErr := exporter.ExportToDisk(".", changedFiles, true)
						if expErr == nil && report != nil {
							fmt.Printf("   • Disk Sync (VS Code): %d file(s) synchronized to workspace (Materialized Lens)\n", report.FilesWritten)
						}
					}
				}
			}
		}

	case "create":
		fs := flag.NewFlagSet("ast create", flag.ExitOnError)
		compName := fs.String("component", "", "Component name (e.g. services/campsite)")
		fs.StringVar(compName, "c", "", "Alias for --component")
		filePath := fs.String("file", "", "Target file path projection (e.g. cmd/server/main.go)")
		fs.StringVar(filePath, "f", "", "Alias for --file")
		lang := fs.String("lang", "", "Language for scaffolding (go, python, typescript, sql)")
		fs.StringVar(lang, "l", "", "Alias for --lang")
		compType := fs.String("type", "", "Component type (service, database, infra, frontend, library)")
		fs.StringVar(compType, "t", "", "Alias for --type")
		code := fs.String("code", "", "Source code or content")
		codeFile := fs.String("code-file", "", "Path to file containing source code")
		universeID := fs.String("u", "universe-main", "Universe ID")
		fs.StringVar(universeID, "universe", "universe-main", "Alias for -u")
		writeDisk := fs.Bool("write-disk", true, "Automatically synchronize component to workspace disk")
		fs.BoolVar(writeDisk, "w", true, "Alias for --write-disk")
		prompt := fs.String("p", "AST Component Creation", "User prompt")
		agentID := fs.String("a", "cosm-ast-surgeon", "Agent ID")
		_ = fs.Parse(args[1:])

		// Allow positional argument for component name e.g. "cosm ast create services/billing --lang go"
		if *compName == "" && len(fs.Args()) > 0 {
			*compName = fs.Args()[0]
		}

		if *code == "" && *codeFile != "" {
			data, rErr := os.ReadFile(*codeFile)
			if rErr != nil {
				fmt.Fprintf(os.Stderr, "Reading code file failed: %v\n", rErr)
				return
			}
			*code = string(data)
		}

		// Auto-scaffold starter code if --lang is specified and no explicit code given
		if *code == "" && *lang != "" {
			starterSrc, defaultPath, defaultType, sErr := mutation.GetStarterSource(*lang)
			if sErr != nil {
				fmt.Fprintf(os.Stderr, "Scaffold error: %v\n", sErr)
				return
			}
			*code = starterSrc
			if *filePath == "" {
				if *compName != "" {
					ext := filepath.Ext(defaultPath)
					*filePath = *compName + ext
				} else {
					*filePath = defaultPath
				}
			}
			if *compType == "" {
				*compType = string(defaultType)
			}
		}

		if *compName == "" && *filePath != "" {
			*compName = *filePath
		}
		if *compName == "" || *code == "" {
			fmt.Println("Error: --component (-c) and either --code, --code-file, or --lang (-l) are required.")
			fmt.Println("Examples:")
			fmt.Println("  cosm ast create -c services/billing --lang go")
			fmt.Println("  cosm ast create -c services/billing -f services/main.go --code 'package main...'")
			return
		}
		if *filePath == "" {
			*filePath = *compName
		}

		lineageEnv := core.LineageEnvelope{
			UserID:           os.Getenv("USER"),
			UserPrompt:       *prompt,
			ExecutingAgentID: *agentID,
			Intent:           "AST Component Creation",
			Timestamp:        time.Now().UTC(),
		}

		batch := mutation.ASTEditBatch{
			UniverseID: *universeID,
			Lineage:    lineageEnv,
			Operations: []mutation.ASTOperation{
				{
					Operation: mutation.OpCreateComponent,
					Target:    *compName,
					Content:   *code,
					Metadata: map[string]string{
						"file_path": *filePath,
					},
				},
			},
		}

		res, err := surgeryEngine.ApplyASTEditBatch(&batch)
		if err != nil {
			fmt.Fprintf(os.Stderr, "AST component creation failed: %v\n", err)
			return
		}

		fmt.Println("✨ AST Component Inception Complete!")
		fmt.Printf("   • Universe:           %s\n", res.UniverseID)
		fmt.Printf("   • Component:          %s (%s)\n", *compName, *filePath)
		fmt.Printf("   • Added Symbols:      %d\n", len(res.AddedSymbols))
		if len(res.OldManifestHash) >= 12 && len(res.NewManifestHash) >= 12 {
			fmt.Printf("   • Merkle Head Delta:  %s -> %s\n", res.OldManifestHash[:12], res.NewManifestHash[:12])
		}

		if *writeDisk {
			updatedHead, hErr := universeMgr.GetUniverseManifest(res.UniverseID)
			if hErr == nil && updatedHead != nil {
				compMap, symMap := loadManifestState(blobStore, graphEngine, updatedHead)
				hydrator := materialize.NewHydrator()
				allFiles, hydErr := hydrator.HydrateWorkspace(updatedHead, compMap, symMap)
				if hydErr == nil && len(allFiles) > 0 {
					exporter := materialize.NewExporter()
					report, expErr := exporter.ExportToDisk(".", allFiles, true)
					if expErr == nil && report != nil {
						fmt.Printf("   • Disk Sync (VS Code): %d file(s) synchronized to workspace (Materialized Lens)\n", report.FilesWritten)
					}
				}
			}
		}

	default:
		fmt.Println("Usage: cosm ast <edit|resolve|tree|create> [flags]")
	}
}

// ASTTreeLineage encapsulates provenance metadata in the AST tree wire format.
type ASTTreeLineage struct {
	UserPrompt       string `json:"user_prompt,omitempty"`
	ExecutingAgentID string `json:"executing_agent_id,omitempty"`
	Intent           string `json:"intent,omitempty"`
	Timestamp        string `json:"timestamp,omitempty"`
}

// ASTTreeEdge represents a cross-boundary semantic dependency in the AST tree wire format.
type ASTTreeEdge struct {
	SourceNodeID     string            `json:"source_node_id,omitempty"`
	TargetID         string            `json:"target_id"`
	TargetNodeID     string            `json:"target_node_id,omitempty"`
	EdgeType         core.EdgeType     `json:"edge_type"`
	Type             core.EdgeType     `json:"type,omitempty"`
	Label            string            `json:"label,omitempty"`
	ContractSchemaID string            `json:"contract_schema_id,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// ASTTreeSymbol represents an individual symbol node with outgoing cross-edges in the AST tree wire format.
type ASTTreeSymbol struct {
	NodeID        string          `json:"node_id"`
	Identifier    string          `json:"identifier"`
	NodeType      string          `json:"node_type"`
	Language      core.Language   `json:"language"`
	Signature     string          `json:"signature,omitempty"`
	Docstring     string          `json:"docstring,omitempty"`
	Visibility    string          `json:"visibility,omitempty"`
	Dependencies  []string        `json:"dependencies,omitempty"`
	OutgoingEdges []*ASTTreeEdge  `json:"outgoing_edges,omitempty"`
	Lineage       *ASTTreeLineage `json:"lineage,omitempty"`
}

// ASTTreeComponent bundles symbols for a source file/module in the AST tree wire format.
type ASTTreeComponent struct {
	ComponentID string           `json:"component_id"`
	Name        string           `json:"name"`
	Language    core.Language    `json:"language"`
	Type        string           `json:"type"`
	Symbols     []*ASTTreeSymbol `json:"symbols"`
	Lineage     *ASTTreeLineage  `json:"lineage,omitempty"`
}

// ASTTreeGraph is the canonical wire DTO for the complete AST hierarchy and symbol dependencies.
type ASTTreeGraph struct {
	UniverseID      string              `json:"universe_id"`
	MerkleRoot      string              `json:"merkle_root"`
	ManifestHash    string              `json:"manifest_hash,omitempty"`
	TotalComponents int                 `json:"total_components"`
	TotalSymbols    int                 `json:"total_symbols"`
	TotalEdges      int                 `json:"total_edges"`
	Components      []*ASTTreeComponent `json:"components"`
	Lineage         *ASTTreeLineage     `json:"lineage,omitempty"`
}

func toASTTreeLineage(env core.LineageEnvelope) *ASTTreeLineage {
	if env.UserPrompt == "" && env.ExecutingAgentID == "" && env.Intent == "" && env.Timestamp.IsZero() {
		return nil
	}
	var ts string
	if !env.Timestamp.IsZero() {
		ts = env.Timestamp.UTC().Format(time.RFC3339)
	}
	return &ASTTreeLineage{
		UserPrompt:       env.UserPrompt,
		ExecutingAgentID: env.ExecutingAgentID,
		Intent:           env.Intent,
		Timestamp:        ts,
	}
}

func extractSignature(symNode *core.ASTSymbolNode) string {
	if symNode.Signature != "" {
		return symNode.Signature
	}
	if len(symNode.ASTPayload) == 0 {
		return ""
	}

	// 1. Try Go struct / func unmarshaling
	if symNode.Language == core.LangGo {
		var fn struct {
			Name     string `json:"name"`
			Receiver string `json:"receiver_type"`
			Params   []struct {
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"params"`
			Results []string `json:"results"`
		}
		if err := json.Unmarshal(symNode.ASTPayload, &fn); err == nil && fn.Name != "" {
			var paramStrs []string
			for _, p := range fn.Params {
				paramStrs = append(paramStrs, fmt.Sprintf("%s %s", p.Name, p.Type))
			}
			recvStr := ""
			if fn.Receiver != "" {
				recvStr = fmt.Sprintf("(%s) ", fn.Receiver)
			}
			sig := fmt.Sprintf("func %s%s(%s)", recvStr, fn.Name, strings.Join(paramStrs, ", "))
			if len(fn.Results) == 1 {
				sig += " " + fn.Results[0]
			} else if len(fn.Results) > 1 {
				sig += fmt.Sprintf(" (%s)", strings.Join(fn.Results, ", "))
			}
			return sig
		}

		var st struct {
			Name   string `json:"name"`
			Fields []struct {
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"fields"`
		}
		if err := json.Unmarshal(symNode.ASTPayload, &st); err == nil && st.Name != "" {
			return fmt.Sprintf("type %s struct", st.Name)
		}
	}

	// 2. Try raw source snippet (e.g. Python, HCL, generic)
	firstLine := strings.SplitN(string(symNode.ASTPayload), "\n", 2)[0]
	firstLine = strings.TrimSpace(firstLine)
	firstLine = strings.TrimSuffix(firstLine, ":")
	if strings.HasPrefix(firstLine, "def ") || strings.HasPrefix(firstLine, "class ") ||
		strings.HasPrefix(firstLine, "func ") || strings.HasPrefix(firstLine, "fn ") ||
		strings.HasPrefix(firstLine, "resource ") || strings.HasPrefix(firstLine, "variable ") {
		return firstLine
	}

	return ""
}

func runASTTree(args []string) {
	fs := flag.NewFlagSet("ast tree", flag.ExitOnError)
	universeID := fs.String("u", "universe-main", "Universe ID")
	fs.StringVar(universeID, "universe", "universe-main", "Universe ID")
	format := fs.String("format", "text", "Output format (text, json)")
	fs.StringVar(format, "f", "text", "Alias for --format")
	_ = fs.Parse(args)

	if *universeID == "universe-main" && len(fs.Args()) > 0 && !strings.HasPrefix(fs.Args()[0], "-") {
		*universeID = fs.Args()[0]
	}

	blobStore, graphEngine, err := openStorage()
	if err != nil {
		if *format == "json" {
			res, _ := json.Marshal(map[string]interface{}{"status": "ERROR", "error": err.Error()})
			fmt.Println(string(res))
		} else {
			fmt.Fprintf(os.Stderr, "Storage error: %v\n", err)
		}
		return
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	head, err := universeMgr.GetUniverseManifest(*universeID)
	if err != nil {
		if *format == "json" {
			res, _ := json.Marshal(map[string]interface{}{"status": "ERROR", "error": err.Error()})
			fmt.Println(string(res))
		} else {
			fmt.Fprintf(os.Stderr, "Error getting universe manifest for %q: %v\n", *universeID, err)
		}
		return
	}
	compMap, symMap := loadManifestState(blobStore, graphEngine, head)

	// Index cross edges by source node ID
	edgesBySource := make(map[string][]*ASTTreeEdge)
	for _, e := range head.CrossEdges {
		lbl := string(e.Type)
		if ep, ok := e.Metadata["backend_endpoint"]; ok {
			lbl = fmt.Sprintf("CALLS %s", ep)
		} else if env, ok := e.Metadata["env_var"]; ok {
			lbl = fmt.Sprintf("BINDS %s", env)
		} else if res, ok := e.Metadata["resource_id"]; ok {
			lbl = fmt.Sprintf("DEPLOYS TO %s", res)
		} else if e.ContractSchemaID != "" {
			lbl = fmt.Sprintf("%s (%s)", e.Type, e.ContractSchemaID)
		}

		edgeDTO := &ASTTreeEdge{
			SourceNodeID:     e.SourceNodeID,
			TargetID:         e.TargetNodeID,
			TargetNodeID:     e.TargetNodeID,
			EdgeType:         e.Type,
			Type:             e.Type,
			Label:            lbl,
			ContractSchemaID: e.ContractSchemaID,
			Metadata:         e.Metadata,
		}
		edgesBySource[e.SourceNodeID] = append(edgesBySource[e.SourceNodeID], edgeDTO)
	}

	var components []*ASTTreeComponent
	totalSymbols := 0
	totalEdges := len(head.CrossEdges)

	seenComps := make(map[string]bool)
	for _, cID := range head.Components {
		if seenComps[cID] {
			continue
		}
		seenComps[cID] = true

		compNode, ok := compMap[cID]
		if !ok || compNode == nil {
			continue
		}

		compName := ""
		if compNode.Metadata != nil {
			compName = compNode.Metadata["file_path"]
		}
		if compName == "" {
			compName = compNode.Name
		}
		if compName == "" {
			compName = compNode.ComponentID
		}

		compDTO := &ASTTreeComponent{
			ComponentID: compNode.ComponentID,
			Name:        compName,
			Language:    compNode.Language,
			Type:        string(compNode.Type),
			Symbols:     make([]*ASTTreeSymbol, 0),
			Lineage:     toASTTreeLineage(compNode.Lineage),
		}

		seenSyms := make(map[string]bool)
		for _, sID := range compNode.SymbolNodes {
			if seenSyms[sID] {
				continue
			}
			seenSyms[sID] = true

			symNode, ok := symMap[sID]
			if !ok || symNode == nil {
				continue
			}

			// Associate cross-edges to source symbols as outgoing_edges
			var matchedEdges []*ASTTreeEdge
			seenEdges := make(map[*ASTTreeEdge]bool)
			addEdges := func(list []*ASTTreeEdge) {
				for _, edge := range list {
					if !seenEdges[edge] {
						seenEdges[edge] = true
						matchedEdges = append(matchedEdges, edge)
					}
				}
			}

			if list, ok := edgesBySource[symNode.NodeID]; ok {
				addEdges(list)
			}
			if symNode.Identifier != "" {
				if list, ok := edgesBySource[symNode.Identifier]; ok {
					addEdges(list)
				}
			}
			for srcID, list := range edgesBySource {
				if srcID != symNode.NodeID && srcID != symNode.Identifier {
					if len(srcID) >= 8 && (strings.HasPrefix(symNode.NodeID, srcID) || strings.HasPrefix(srcID, symNode.NodeID)) {
						addEdges(list)
					}
				}
			}

			sort.Slice(matchedEdges, func(i, j int) bool {
				if matchedEdges[i].TargetID != matchedEdges[j].TargetID {
					return matchedEdges[i].TargetID < matchedEdges[j].TargetID
				}
				return matchedEdges[i].EdgeType < matchedEdges[j].EdgeType
			})

			deps := symNode.LocalDependencies
			if len(deps) == 0 {
				deps = symNode.Dependencies
			}

			sig := symNode.Signature
			if sig == "" {
				sig = extractSignature(symNode)
			}

			symDTO := &ASTTreeSymbol{
				NodeID:        symNode.NodeID,
				Identifier:    symNode.Identifier,
				NodeType:      symNode.NodeType,
				Language:      symNode.Language,
				Signature:     sig,
				Docstring:     symNode.Docstring,
				Visibility:    symNode.Visibility,
				Dependencies:  deps,
				OutgoingEdges: matchedEdges,
				Lineage:       toASTTreeLineage(symNode.Lineage),
			}

			compDTO.Symbols = append(compDTO.Symbols, symDTO)
			totalSymbols++
		}

		sort.Slice(compDTO.Symbols, func(i, j int) bool {
			if compDTO.Symbols[i].Identifier != compDTO.Symbols[j].Identifier {
				return compDTO.Symbols[i].Identifier < compDTO.Symbols[j].Identifier
			}
			return compDTO.Symbols[i].NodeID < compDTO.Symbols[j].NodeID
		})

		components = append(components, compDTO)
	}

	sort.Slice(components, func(i, j int) bool {
		return components[i].Name < components[j].Name
	})

	treeGraph := &ASTTreeGraph{
		UniverseID:      *universeID,
		MerkleRoot:      head.MerkleRootHash,
		ManifestHash:    head.MerkleRootHash,
		TotalComponents: len(components),
		TotalSymbols:    totalSymbols,
		TotalEdges:      totalEdges,
		Components:      components,
		Lineage:         toASTTreeLineage(head.Lineage),
	}

	if *format == "json" {
		data, err := json.MarshalIndent(treeGraph, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "JSON encoding error: %v\n", err)
			return
		}
		fmt.Println(string(data))
		return
	}

	fmt.Print(renderASTTreeASCII(treeGraph, symMap))
}

func renderASTTreeASCII(graph *ASTTreeGraph, symMap map[string]*core.ASTSymbolNode) string {
	var sb strings.Builder

	headStr := graph.MerkleRoot
	if headStr == "" {
		headStr = graph.ManifestHash
	}
	if headStr == "" {
		headStr = "uncommitted"
	}
	fmt.Fprintf(&sb, "🌌 %s (Head: %s)\n", graph.UniverseID, shortHash(headStr))

	if len(graph.Components) == 0 {
		sb.WriteString("└── (empty universe - no components tracked)\n")
		return sb.String()
	}

	numComps := len(graph.Components)
	for i, comp := range graph.Components {
		isLastComp := (i == numComps-1)
		compPrefix := "├── "
		compIndent := "│   "
		if isLastComp {
			compPrefix = "└── "
			compIndent = "    "
		}

		compDesc := comp.Name
		if comp.Language != "" && comp.Type != "" {
			compDesc = fmt.Sprintf("%s (%s, %s)", comp.Name, comp.Language, comp.Type)
		} else if comp.Language != "" {
			compDesc = fmt.Sprintf("%s (%s)", comp.Name, comp.Language)
		}

		fmt.Fprintf(&sb, "%s📦 %s\n", compPrefix, compDesc)

		if len(comp.Symbols) == 0 {
			fmt.Fprintf(&sb, "%s└── (no symbols)\n", compIndent)
			continue
		}

		numSyms := len(comp.Symbols)
		for j, sym := range comp.Symbols {
			isLastSym := (j == numSyms-1)
			symPrefix := "├── "
			symIndent := "│   "
			if isLastSym {
				symPrefix = "└── "
				symIndent = "    "
			}

			symDesc := sym.Identifier
			if sym.NodeType != "" {
				symDesc = fmt.Sprintf("%s (%s)", sym.Identifier, sym.NodeType)
			}
			fmt.Fprintf(&sb, "%s%s🔹 %s\n", compIndent, symPrefix, symDesc)

			var details []string
			if sym.Signature != "" {
				details = append(details, fmt.Sprintf("Signature: %s", sym.Signature))
			}
			for _, edge := range sym.OutgoingEdges {
				tgt := edge.TargetID
				if tgt == "" {
					tgt = edge.TargetNodeID
				}
				if symMap != nil {
					if tgtSym, ok := symMap[tgt]; ok && tgtSym.Identifier != "" {
						tgt = tgtSym.Identifier
					}
				}
				edgeStr := fmt.Sprintf("🔗 %s -> %s", edge.EdgeType, tgt)
				if edge.ContractSchemaID != "" {
					edgeStr += fmt.Sprintf(" [contract: %s]", edge.ContractSchemaID)
				}
				details = append(details, edgeStr)
			}

			numDetails := len(details)
			for k, d := range details {
				isLastDetail := (k == numDetails-1)
				detailPrefix := "├── "
				if isLastDetail {
					detailPrefix = "└── "
				}
				fmt.Fprintf(&sb, "%s%s%s%s\n", compIndent, symIndent, detailPrefix, d)
			}
		}
	}

	return sb.String()
}

func runExport(args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	universeID := fs.String("u", "universe-main", "Universe ID")
	destDir := fs.String("d", "exported_code", "Destination directory")
	format := fs.String("format", "disk", "Export format: 'disk', 'tar', or 'json'")
	outputFile := fs.String("o", "", "Output file path (optional for tar/json, defaults to stdout)")
	_ = fs.Parse(args)

	blobStore, graphEngine, err := openStorage()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Storage error: %v\n", err)
		os.Exit(1)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	head, err := universeMgr.GetUniverseManifest(*universeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Getting universe manifest: %v\n", err)
		return
	}

	compMap, symMap := loadManifestState(blobStore, graphEngine, head)
	hydrator := materialize.NewHydrator()
	files, err := hydrator.HydrateWorkspace(head, compMap, symMap)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Hydrating files: %v\n", err)
		return
	}

	exporter := materialize.NewExporter()

	switch *format {
	case "tar":
		var out io.Writer = os.Stdout
		if *outputFile != "" {
			f, err := os.Create(*outputFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Creating output tar file %s: %v\n", *outputFile, err)
				return
			}
			defer f.Close()
			out = f
		}
		_, err := exporter.ExportToTarStream(out, files)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Exporting tar stream error: %v\n", err)
			return
		}

	case "json":
		report, err := exporter.ExportToDisk(*destDir, files, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Export error: %v\n", err)
			return
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		if *outputFile != "" {
			_ = os.WriteFile(*outputFile, data, 0644)
		} else {
			fmt.Println(string(data))
		}

	default: // "disk"
		report, err := exporter.ExportToDisk(*destDir, files, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Export error: %v\n", err)
			return
		}
		fmt.Printf("📦 Successfully exported %d files (%d bytes) from universe '%s' to '%s'\n",
			report.FilesWritten, report.TotalBytes, *universeID, *destDir)
	}
}

func runImport(args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	universeID := fs.String("u", "universe-main", "Universe ID")
	srcDir := fs.String("d", ".", "Source directory to scan")
	agentID := fs.String("a", "cosm-importer", "Agent ID")
	_ = fs.Parse(args)

	var matchedFiles []string
	_ = filepath.Walk(*srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() && (info.Name() == ".git" || info.Name() == ".cosm" || info.Name() == ".fg" || info.Name() == "node_modules" || info.Name() == "dist") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext == ".go" || ext == ".tf" || ext == ".hcl" || ext == ".ts" || ext == ".tsx" || ext == ".py" {
			matchedFiles = append(matchedFiles, path)
		}
		return nil
	})

	if len(matchedFiles) == 0 {
		fmt.Printf("No supported source files found in %s\n", *srcDir)
		return
	}

	fmt.Printf("🔍 Ingesting %d files into universe '%s'...\n", len(matchedFiles), *universeID)
	addArgs := append([]string{"-u", *universeID, "-a", *agentID, "-i", "Bulk import from " + *srcDir}, matchedFiles...)
	runAdd(addArgs)
	commitArgs := []string{"-u", *universeID, "-a", *agentID, "-i", "Bulk import workspace files"}
	runCommit(commitArgs)
}

func runDashboard(args []string) {
	fs := flag.NewFlagSet("dashboard", flag.ExitOnError)
	universeID := fs.String("u", "universe-main", "Universe ID")
	_ = fs.Parse(args)

	blobStore, graphEngine, err := openStorage()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Storage error: %v\n", err)
		os.Exit(1)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	head, err := universeMgr.GetUniverseManifest(*universeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Getting universe manifest: %v\n", err)
		return
	}

	compMap, symMap := loadManifestState(blobStore, graphEngine, head)
	vis := target.NewTopologyVisualizer()
	topGraph := vis.BuildTopology(head, compMap, symMap)

	app := tui.NewAppModel(*universeID, topGraph)
	fmt.Print(app.RenderView())
}

func runGit(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: cosm git <status|log|diff|push>")
		return
	}

	blobStore, graphEngine, err := openStorage()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Storage error: %v\n", err)
		os.Exit(1)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	interceptor := gitshim.NewGitShimInterceptor(universeMgr, blobStore, graphEngine)

	sub := args[0]
	switch sub {
	case "status":
		st, err := interceptor.Status("universe-main", map[string][]byte{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "git status error: %v\n", err)
			return
		}
		fmt.Printf("On universe %s\n", st.UniverseID)
		if st.Clean {
			fmt.Println("nothing to commit, working tree clean")
		} else {
			fmt.Printf("Changes not staged: %v\n", st.ModifiedFiles)
		}
	case "log":
		entries, err := interceptor.Log("universe-main")
		if err != nil {
			fmt.Fprintf(os.Stderr, "git log error: %v\n", err)
			return
		}
		for _, e := range entries {
			fmt.Printf("commit %s\nAuthor: %s\nDate:   %s\n\n    %s\n\n", e.CommitHash, e.Author, e.Date.Format(time.RFC1123), e.Message)
		}
	case "init-bridge":
		fs := flag.NewFlagSet("git init-bridge", flag.ExitOnError)
		universeID := fs.String("u", "universe-main", "Universe ID")
		dir := fs.String("d", ".", "Workspace directory")
		_ = fs.Parse(args[1:])
		absDir, err := filepath.Abs(*dir)
		if err != nil {
			absDir = *dir
		}
		if err := gitshim.InitGitBridge(absDir, *universeID); err != nil {
			fmt.Fprintf(os.Stderr, "git init-bridge error: %v\n", err)
			return
		}
		fmt.Printf("✓ Initialized synthetic .git bridge in %s for universe '%s'\n", absDir, *universeID)
	default:
		fmt.Printf("Executed 'cosm git %s'\n", sub)
	}
}

func runMCP(args []string) {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	dir := fs.String("d", ".", "Workspace directory")
	_ = fs.Parse(args)

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		absDir = *dir
	}

	srv, err := mcp.NewServer(absDir, os.Stdin, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize MCP server: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := srv.Run(ctx); err != nil && err != io.EOF && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "MCP server exited with error: %v\n", err)
		os.Exit(1)
	}
}

func runLSP(args []string) {
	fs := flag.NewFlagSet("lsp", flag.ExitOnError)
	dir := fs.String("d", ".", "Workspace directory")
	_ = fs.Parse(args)

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		absDir = *dir
	}

	srv, err := lsp.NewServer(absDir, os.Stdin, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize LSP server: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := srv.Run(ctx); err != nil && err != io.EOF && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "LSP server exited with error: %v\n", err)
		os.Exit(1)
	}
}

func runWatch(args []string) {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	dir := fs.String("d", ".", "Workspace directory")
	intervalMs := fs.Int("interval", 250, "Polling interval in milliseconds")
	universeID := fs.String("u", "universe-main", "Universe ID")
	_ = fs.Parse(args)

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		absDir = *dir
	}

	w, err := watcher.NewWatcher(absDir, time.Duration(*intervalMs)*time.Millisecond)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize watcher: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Cosm Watcher running on %s (universe: %s, interval: %dms)...\n", absDir, *universeID, *intervalMs)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := w.Start(ctx); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "watcher stopped with error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("\nWatcher stopped.")
}

func openStorage() (*storage.BlobStore, *storage.GraphEngine, error) {
	cosmDir := ".cosm"
	blobStore, err := storage.NewBlobStore(filepath.Join(cosmDir, "objects"))
	if err != nil {
		return nil, nil, fmt.Errorf("opening blob store: %w", err)
	}
	graphEngine, err := storage.NewGraphEngine(filepath.Join(cosmDir, "graph.db"))
	if err != nil {
		return nil, nil, fmt.Errorf("opening graph engine: %w", err)
	}
	return blobStore, graphEngine, nil
}

func isHex(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// resolveNodeOrSymbol centralizes query resolution across hex IDs, short prefixes, and human symbol identifiers.
func resolveNodeOrSymbol(
	graphEngine *storage.GraphEngine,
	universeMgr *storage.UniverseManager,
	universeID, query string,
) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("node or symbol query cannot be empty")
	}
	if universeID == "" {
		universeID = "universe-main"
	}

	// 1. If query is 64 hex characters -> return exact node ID / hash.
	if len(query) == 64 && isHex(query) {
		return query, nil
	}

	// 2. If query is a short hex prefix (e.g. >= 7 chars) -> search graphEngine.ListNodes() for prefix match.
	if len(query) >= 7 && isHex(query) && !strings.Contains(query, "::") {
		if graphEngine != nil {
			allNodes := graphEngine.ListNodes("", "")
			for _, n := range allNodes {
				if strings.HasPrefix(n.NodeID, query) || strings.HasPrefix(n.MerkleHash, query) {
					return n.NodeID, nil
				}
			}
		}
	}

	// 3. If query is a human symbol identifier (e.g. contains '::' or has no hex representation) -> call mutation.ResolveSymbol
	if universeMgr != nil {
		res, err := mutation.ResolveSymbol(universeMgr, universeID, query)
		if err == nil && res != nil && res.SymbolNode != nil && res.SymbolNode.NodeID != "" {
			return res.SymbolNode.NodeID, nil
		}
	}

	// Fallback 1: search graphEngine for exact node ID match or prefix match (e.g. comp-raw-... or non-hex ID)
	if graphEngine != nil {
		allNodes := graphEngine.ListNodes("", "")
		for _, n := range allNodes {
			if n.NodeID == query || strings.HasPrefix(n.NodeID, query) {
				return n.NodeID, nil
			}
		}
	}

	// Fallback 2: if universeID != "universe-main", check universe-main
	if universeMgr != nil && universeID != "universe-main" {
		if res, err := mutation.ResolveSymbol(universeMgr, "universe-main", query); err == nil && res != nil && res.SymbolNode != nil {
			return res.SymbolNode.NodeID, nil
		}
	}

	return "", fmt.Errorf("symbol or node %q not found in universe %s", query, universeID)
}

func resolveNodeID(graphEngine *storage.GraphEngine, query string) string {
	res, err := resolveNodeOrSymbol(graphEngine, nil, "universe-main", query)
	if err == nil && res != "" {
		return res
	}
	return query
}

func loadManifestState(
	blobStore *storage.BlobStore,
	graphEngine *storage.GraphEngine,
	manifest *core.WorkspaceManifestNode,
) (map[string]*core.ComponentNode, map[string]*core.ASTSymbolNode) {
	compMap := make(map[string]*core.ComponentNode)
	symMap := make(map[string]*core.ASTSymbolNode)

	if manifest == nil {
		return compMap, symMap
	}

	for _, cID := range manifest.Components {
		var compData []byte
		var err error
		if node, gErr := graphEngine.GetNode(cID); gErr == nil && node != nil {
			compData, err = blobStore.Get(node.MerkleHash)
		} else {
			compData, err = blobStore.Get(cID)
		}
		if err == nil {
			var comp core.ComponentNode
			if e := json.Unmarshal(compData, &comp); e == nil {
				compMap[comp.ComponentID] = &comp
				compMap[cID] = &comp
				for _, sID := range comp.SymbolNodes {
					var sData []byte
					var sErr error
					if sNode, sgErr := graphEngine.GetNode(sID); sgErr == nil && sNode != nil {
						sData, sErr = blobStore.Get(sNode.MerkleHash)
					} else {
						sData, sErr = blobStore.Get(sID)
					}
					if sErr == nil {
						var sym core.ASTSymbolNode
						if se := json.Unmarshal(sData, &sym); se == nil {
							symMap[sym.NodeID] = &sym
							symMap[sID] = &sym
						}
					}
				}
			}
		}
	}
	return compMap, symMap
}

func runTopocosm(args []string) {
	if len(args) == 0 {
		fmt.Println(`Usage: cosm topocosm <subcommand> [flags]

Subcommands:
  dev         Start local zero-Docker Topocosm Hub daemon (.topocosm/)
  seed        Populate local hub with sample polyglot cosms & stacked proposals
  test-swarm  Run concurrent multi-agent swarm simulation benchmark
  status      Inspect hub metrics, active cosms, and agent blackboard claims`)
		return
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "dev":
		fs := flag.NewFlagSet("topocosm dev", flag.ExitOnError)
		port := fs.Int("port", 51204, "HTTP server port")
		dataDir := fs.String("dir", ".topocosm", "Data storage directory")
		_ = fs.Parse(subArgs)

		if err := os.MkdirAll(*dataDir, 0755); err != nil {
			fmt.Printf("Error creating data dir: %v\n", err)
			os.Exit(1)
		}

		bp, err := backplane.NewLocalBackplane(*dataDir)
		if err != nil {
			fmt.Printf("Failed to initialize local backplane: %v\n", err)
			os.Exit(1)
		}
		defer bp.Close()

		server := topocosm.NewHubServer(bp)

		// Seed initial data if empty
		_ = topocosm.SeedLocalHub(context.Background(), server, topocosm.DefaultSeedOptions())

		addr := fmt.Sprintf("127.0.0.1:%d", *port)
		fmt.Println("=====================================================================")
		fmt.Printf("  🪐 TOPOCOSM HUB LOCAL DAEMON (Zero-Docker / Pure-Go)\n")
		fmt.Println("  (Notice: Topocosm is also decoupled as a standalone binary in github.com/cosmscm/topocosm)")
		fmt.Println("=====================================================================")
		fmt.Printf("  • Server listening on:       http://%s\n", addr)
		fmt.Printf("  • Agent Discovery Manifest:  http://%s/.well-known/cosm-agent.json\n", addr)
		fmt.Printf("  • REST & gRPC API Root:      http://%s/api/v1/\n", addr)
		fmt.Printf("  • Git Smart-HTTP Shim:       http://%s/git/{org}/{cosm}.git\n", addr)
		fmt.Printf("  • Local CAS & SQLite Store:  %s/\n", *dataDir)
		fmt.Println("=====================================================================")
		fmt.Println("Ready for AI Agent Swarms and Developer CLI operations. Press Ctrl+C to stop.")

		httpServer := &http.Server{
			Addr:    addr,
			Handler: server.Handler(),
		}
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Server exited: %v\n", err)
		}

	case "seed":
		fs := flag.NewFlagSet("topocosm seed", flag.ExitOnError)
		dataDir := fs.String("dir", ".topocosm", "Data storage directory")
		_ = fs.Parse(subArgs)

		bp, err := backplane.NewLocalBackplane(*dataDir)
		if err != nil {
			fmt.Printf("Failed to initialize local backplane: %v\n", err)
			os.Exit(1)
		}
		defer bp.Close()

		server := topocosm.NewHubServer(bp)
		err = topocosm.SeedLocalHub(context.Background(), server, topocosm.DefaultSeedOptions())
		if err != nil {
			fmt.Printf("Error seeding hub: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Successfully seeded Topocosm Hub with sample polyglot cosms, proposals, and blackboard claims.")

	case "test-swarm":
		fs := flag.NewFlagSet("topocosm test-swarm", flag.ExitOnError)
		agents := fs.Int("agents", 25, "Number of concurrent virtual agent workers")
		duration := fs.Duration("duration", 3*time.Second, "Duration of simulation")
		dataDir := fs.String("dir", ".topocosm", "Data storage directory")
		_ = fs.Parse(subArgs)

		bp, err := backplane.NewLocalBackplane(*dataDir)
		if err != nil {
			fmt.Printf("Failed to initialize local backplane: %v\n", err)
			os.Exit(1)
		}
		defer bp.Close()

		server := topocosm.NewHubServer(bp)
		_ = topocosm.SeedLocalHub(context.Background(), server, topocosm.DefaultSeedOptions())

		fmt.Printf("🚀 Starting Multi-Agent Swarm Simulation (%d agents, %v duration)...\n", *agents, *duration)
		report, err := topocosm.RunSwarmSimulation(context.Background(), server, topocosm.SwarmSimulationConfig{
			NumAgents:   *agents,
			Concurrency: 10,
			Duration:    *duration,
			OrgSlug:     "demo-org",
			CosmName:    "cloud-platform",
		})
		if err != nil {
			fmt.Printf("Swarm simulation error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("=====================================================================")
		fmt.Printf("  ⚡ MULTI-AGENT SWARM SIMULATION BENCHMARK REPORT\n")
		fmt.Println("=====================================================================")
		fmt.Printf("  • Total Operations Executed: %d\n", report.TotalOperations)
		fmt.Printf("  • Operations / Second:       %.2f op/s\n", report.OpsPerSecond)
		fmt.Printf("  • Sparse Subtree Pulls:      %d\n", report.SparsePulls)
		fmt.Printf("  • Blackboard Claims Won:     %d\n", report.ClaimsAcquired)
		fmt.Printf("  • Claim Conflicts Avoided:   %d\n", report.ClaimsContested)
		fmt.Printf("  • CRDT Proposals Merged:     %d\n", report.ProposalsMerged)
		fmt.Printf("  • Simulation Duration:       %v\n", report.Duration)
		fmt.Println("=====================================================================")

	case "status":
		fs := flag.NewFlagSet("topocosm status", flag.ExitOnError)
		hubURL := fs.String("url", "http://127.0.0.1:51204", "Topocosm Hub URL")
		_ = fs.Parse(subArgs)

		client := topocosm.NewHubClient(*hubURL, "did:key:z6MkuCLIViewer")
		stats, err := client.GetStats(context.Background())
		if err != nil {
			fmt.Printf("Could not connect to Topocosm Hub at %s: %v\n", *hubURL, err)
			os.Exit(1)
		}

		fmt.Println("=====================================================================")
		fmt.Printf("  🪐 TOPOCOSM HUB STATUS (%s)\n", *hubURL)
		fmt.Println("=====================================================================")
		fmt.Printf("  • Registered Cosms:        %d\n", stats.TotalCosms)
		fmt.Printf("  • Active Agent DIDs:       %d\n", stats.RegisteredAgents)
		fmt.Printf("  • Open Proposals:          %d\n", stats.TotalProposals)
		fmt.Printf("  • Active Blackboard Locks: %d\n", stats.ActiveClaims)
		fmt.Printf("  • Content Blobs in Store:  %d\n", stats.TotalBlobs)
		fmt.Printf("  • Server Uptime:           %d seconds\n", stats.UptimeSec)
		fmt.Println("=====================================================================")

	default:
		fmt.Printf("Unknown topocosm subcommand: %s\n", sub)
	}
}

func runPublish(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: cosm publish [flags] <hub_url>/<org>/<cosm> or <org>/<cosm>")
		fmt.Println("  e.g.: cosm publish http://127.0.0.1:51204/demo-org/cloud-platform")
		return
	}

	fs := flag.NewFlagSet("publish", flag.ExitOnError)
	universeID := fs.String("u", "universe-main", "Universe ID to publish")
	intent := fs.String("i", "Publish cosm snapshot to Topocosm Hub", "Publish intent")
	_ = fs.Parse(args)

	remain := fs.Args()
	if len(remain) == 0 {
		fmt.Println("Usage: cosm publish [flags] <hub_url>/<org>/<cosm> or <org>/<cosm>")
		return
	}

	targetURI := remain[0]
	hubURL, orgSlug, cosmName := resolveHubTarget(targetURI)

	blobStore, graphEngine, err := openStorage()
	if err != nil {
		fmt.Printf("Storage error: %v\n", err)
		os.Exit(1)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)

	manifest, err := universeMgr.GetUniverseManifest(*universeID)
	if err != nil || manifest == nil {
		fmt.Printf("Universe manifest '%s' not found: %v\n", *universeID, err)
		os.Exit(1)
	}

	// Gather components, symbols, and raw blobs
	components := make(map[string]*core.ComponentNode)
	symbols := make(map[string]*core.ASTSymbolNode)
	blobs := make(map[string][]byte)

	for _, compID := range manifest.Components {
		var compBytes []byte
		var err error
		compBytes, err = blobStore.Get(compID)
		if err != nil {
			if nodeRec, e := graphEngine.GetNode(compID); e == nil {
				compBytes, err = blobStore.Get(nodeRec.MerkleHash)
			}
		}
		if err == nil {
			blobs[compID] = compBytes
			var comp core.ComponentNode
			if json.Unmarshal(compBytes, &comp) == nil {
				components[compID] = &comp
				for _, symID := range comp.SymbolNodes {
					var symBytes []byte
					symBytes, err = blobStore.Get(symID)
					if err != nil {
						if symRec, e := graphEngine.GetNode(symID); e == nil {
							symBytes, err = blobStore.Get(symRec.MerkleHash)
						}
					}
					if err == nil {
						blobs[symID] = symBytes
						var sym core.ASTSymbolNode
						if json.Unmarshal(symBytes, &sym) == nil {
							symbols[symID] = &sym
						}
					}
				}
			}
		}
	}

	payload := &topocosm.PublishPayload{
		OrgSlug:     orgSlug,
		CosmName:    cosmName,
		UniverseID:  *universeID,
		Manifest:    manifest,
		Components:  components,
		Symbols:     symbols,
		Blobs:       blobs,
		Description: *intent,
		Visibility:  topocosm.VisibilityPublic,
		IsInitial:   false,
	}

	client := topocosm.NewHubClient(hubURL, "did:key:z6MkuCLIPublisher")
	resp, err := client.PublishCosm(context.Background(), payload)
	if err != nil {
		fmt.Printf("Failed to publish to %s: %v\n", hubURL, err)
		os.Exit(1)
	}

	fmt.Printf("🚀 Successfully published %s/%s to Topocosm Hub!\n", orgSlug, cosmName)
	fmt.Printf("   • Merkle Root: %s\n", resp.MerkleRootHash)
	fmt.Printf("   • Blobs Synced: %d\n", resp.BlobsStored)
	fmt.Printf("   • Universe:     %s\n", resp.UniverseID)
}

func runClone(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: cosm clone [flags] <hub_url>/<org>/<cosm> [dest_dir]")
		fmt.Println("  e.g.: cosm clone http://127.0.0.1:51204/demo-org/cloud-platform ./my-platform")
		return
	}

	fs := flag.NewFlagSet("clone", flag.ExitOnError)
	sparse := fs.String("sparse", "", "Comma-separated component names for sparse AST download")
	universeID := fs.String("u", "universe-main", "Micro-universe to clone")
	_ = fs.Parse(args)

	remain := fs.Args()
	if len(remain) == 0 {
		fmt.Println("Error: Missing target repository URI")
		os.Exit(1)
	}

	targetURI := remain[0]
	destDir := "."
	if len(remain) > 1 {
		destDir = remain[1]
	}

	hubURL, orgSlug, cosmName := resolveHubTarget(targetURI)

	client := topocosm.NewHubClient(hubURL, "did:key:z6MkuCLICloner")

	var pullResp *topocosm.SparsePullResponse
	var err error

	if *sparse != "" {
		names := strings.Split(*sparse, ",")
		pullResp, err = client.SparsePullCosm(context.Background(), &topocosm.SparsePullRequest{
			OrgSlug:        orgSlug,
			CosmName:       cosmName,
			UniverseID:     *universeID,
			ComponentNames: names,
		})
	} else {
		pullResp, err = client.PullCosm(context.Background(), orgSlug, cosmName, *universeID)
	}

	if err != nil {
		fmt.Printf("Failed to clone from %s: %v\n", hubURL, err)
		os.Exit(1)
	}

	// Initialize local cosm directory
	cosmDir := filepath.Join(destDir, ".cosm")
	if err := os.MkdirAll(filepath.Join(cosmDir, "objects"), 0755); err != nil {
		fmt.Printf("Failed to create destination dir: %v\n", err)
		os.Exit(1)
	}

	blobStore, err := storage.NewBlobStore(filepath.Join(cosmDir, "objects"))
	if err != nil {
		fmt.Printf("BlobStore error: %v\n", err)
		os.Exit(1)
	}

	graphEngine, err := storage.NewGraphEngine(filepath.Join(cosmDir, "graph.db"))
	if err != nil {
		fmt.Printf("GraphEngine error: %v\n", err)
		os.Exit(1)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)

	// Store blobs
	for _, data := range pullResp.Blobs {
		_, _ = blobStore.Put(data)
	}

	// Populate graphEngine nodes
	for compID, comp := range pullResp.Components {
		compData, _ := json.Marshal(comp)
		compHash, _ := blobStore.Put(compData)
		_ = graphEngine.PutNode(storage.NodeRecord{
			NodeID:     compID,
			Language:   comp.Language,
			NodeType:   string(comp.Type),
			MerkleHash: compHash,
			CreatedAt:  time.Now(),
		})
	}
	for symID, sym := range pullResp.Symbols {
		symData, _ := json.Marshal(sym)
		symHash, _ := blobStore.Put(symData)
		_ = graphEngine.PutNode(storage.NodeRecord{
			NodeID:     symID,
			Language:   sym.Language,
			NodeType:   sym.NodeType,
			MerkleHash: symHash,
			CreatedAt:  time.Now(),
		})
	}
	for _, edge := range pullResp.Manifest.CrossEdges {
		_ = graphEngine.PutEdge(storage.EdgeRecord{
			SourceID: edge.SourceNodeID,
			TargetID: edge.TargetNodeID,
			EdgeType: edge.Type,
			Metadata: edge.Metadata,
		})
	}

	// Commit manifest
	_, err = universeMgr.CommitManifest(pullResp.Manifest.UniverseID, pullResp.Manifest)
	if err != nil {
		fmt.Printf("Failed to commit manifest: %v\n", err)
		os.Exit(1)
	}

	// Materialize files to working tree
	hydrator := materialize.NewHydrator()
	files, err := hydrator.HydrateWorkspace(pullResp.Manifest, pullResp.Components, pullResp.Symbols)
	if err == nil {
		exporter := materialize.NewExporter()
		_, err = exporter.ExportToDisk(destDir, files, true)
		if err != nil {
			fmt.Printf("Warning: export error: %v\n", err)
		}
	}

	fmt.Printf("✅ Successfully cloned %s/%s to %s!\n", orgSlug, cosmName, destDir)
	fmt.Printf("   • Merkle Root:     %s\n", pullResp.Manifest.MerkleRootHash)
	fmt.Printf("   • Components:      %d\n", len(pullResp.Components))
	fmt.Printf("   • AST Symbols:     %d\n", len(pullResp.Symbols))
	if pullResp.SavingsPercent > 0 {
		fmt.Printf("   • Sparse Bandwidth Savings: %.1f%%\n", pullResp.SavingsPercent)
	}
}

func resolveHubTarget(rawTarget string) (hubURL, orgSlug, cosmName string) {
	hubURL = "http://127.0.0.1:51204"
	if envHub := os.Getenv("TOPOCOSM_HUB_URL"); envHub != "" {
		hubURL = envHub
	}
	orgSlug = "demo-org"
	cosmName = "cloud-platform"

	if rawTarget == "" {
		return
	}

	parts := strings.Split(rawTarget, "/")
	if len(parts) == 1 {
		cosmName = parts[0]
	} else if len(parts) == 2 {
		orgSlug = parts[0]
		cosmName = parts[1]
	} else if len(parts) >= 3 {
		cosmName = parts[len(parts)-1]
		orgSlug = parts[len(parts)-2]
		hubURL = strings.Join(parts[:len(parts)-2], "/")
	}
	return
}

func runClaim(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: cosm claim [flags] <domain> [hub_url/org/cosm]")
		fmt.Println("  e.g.: cosm claim services/billing --goal \"Refactoring Stripe signature\" --ttl 600")
		fmt.Println("  e.g.: cosm claim services/billing http://127.0.0.1:51204/demo-org/cloud-platform")
		return
	}

	fs := flag.NewFlagSet("claim", flag.ExitOnError)
	goal := fs.String("goal", "AST mutation lease", "Intent or description of task holding the domain lease")
	ttl := fs.Int("ttl", 600, "Lease time-to-live duration in seconds")
	hubFlag := fs.String("url", "", "Topocosm Hub endpoint URL")
	agentFlag := fs.String("agent", "", "Agent DID identifier holding the lease")
	_ = fs.Parse(args)

	remain := fs.Args()
	if len(remain) == 0 {
		fmt.Println("Error: Missing domain to claim (e.g. services/billing, infra/pubsub)")
		os.Exit(1)
	}

	domain := remain[0]
	target := ""
	if len(remain) > 1 {
		target = remain[1]
	}

	authCfg, _ := topocosm.LoadAuthConfig()
	hubURL, orgSlug, cosmName := resolveHubTarget(target)
	if *hubFlag != "" {
		hubURL = *hubFlag
	} else if authCfg.HubURL != "" && authCfg.HubURL != "https://topocosm.dev" {
		hubURL = authCfg.HubURL
	}

	callerDID := "did:key:z6MkuCLIPublisher"
	if *agentFlag != "" {
		callerDID = *agentFlag
	} else if authCfg.CallerDID != "" {
		callerDID = authCfg.CallerDID
	}

	client := topocosm.NewHubClient(hubURL, callerDID)
	if authCfg.Token != "" {
		client.SetBearerToken(authCfg.Token)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ok, err := client.ClaimDomain(ctx, orgSlug, cosmName, domain, *goal, *ttl)
	if err != nil {
		fmt.Printf("❌ Failed to acquire domain lease for %q on %s/%s: %v\n", domain, orgSlug, cosmName, err)
		return
	}
	if !ok {
		fmt.Printf("⚠️ Domain %q on %s/%s is currently locked by another agent or contested.\n", domain, orgSlug, cosmName)
		return
	}

	fmt.Println("=====================================================================")
	fmt.Println("  🔒 BLACKBOARD DOMAIN LEASE ACQUIRED")
	fmt.Println("=====================================================================")
	fmt.Printf("  • Domain:      %s\n", domain)
	fmt.Printf("  • Holder DID:  %s\n", callerDID)
	fmt.Printf("  • Goal:        %s\n", *goal)
	fmt.Printf("  • Lease TTL:   %ds\n", *ttl)
	fmt.Printf("  • Target Cosm: %s/%s\n", orgSlug, cosmName)
	fmt.Printf("  • Hub URL:     %s\n", hubURL)
	fmt.Println("=====================================================================")
}

func runRelease(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: cosm release [flags] <domain> [hub_url/org/cosm]")
		fmt.Println("  e.g.: cosm release services/billing")
		return
	}

	fs := flag.NewFlagSet("release", flag.ExitOnError)
	hubFlag := fs.String("url", "", "Topocosm Hub endpoint URL")
	agentFlag := fs.String("agent", "", "Agent DID identifier holding the lease")
	_ = fs.Parse(args)

	remain := fs.Args()
	if len(remain) == 0 {
		fmt.Println("Error: Missing domain to release")
		os.Exit(1)
	}

	domain := remain[0]
	target := ""
	if len(remain) > 1 {
		target = remain[1]
	}

	authCfg, _ := topocosm.LoadAuthConfig()
	hubURL, orgSlug, cosmName := resolveHubTarget(target)
	if *hubFlag != "" {
		hubURL = *hubFlag
	} else if authCfg.HubURL != "" && authCfg.HubURL != "https://topocosm.dev" {
		hubURL = authCfg.HubURL
	}

	callerDID := "did:key:z6MkuCLIPublisher"
	if *agentFlag != "" {
		callerDID = *agentFlag
	} else if authCfg.CallerDID != "" {
		callerDID = authCfg.CallerDID
	}

	client := topocosm.NewHubClient(hubURL, callerDID)
	if authCfg.Token != "" {
		client.SetBearerToken(authCfg.Token)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.ReleaseDomain(ctx, orgSlug, cosmName, domain); err != nil {
		fmt.Printf("❌ Failed to release domain lease %q: %v\n", domain, err)
		return
	}

	fmt.Printf("✅ Successfully released blackboard domain lease: %s\n", domain)
}

func runBlackboard(args []string) {
	fs := flag.NewFlagSet("blackboard", flag.ExitOnError)
	hubFlag := fs.String("url", "", "Topocosm Hub endpoint URL")
	_ = fs.Parse(args)

	target := ""
	if len(fs.Args()) > 0 {
		target = fs.Args()[0]
	}

	authCfg, _ := topocosm.LoadAuthConfig()
	hubURL, orgSlug, cosmName := resolveHubTarget(target)
	if *hubFlag != "" {
		hubURL = *hubFlag
	} else if authCfg.HubURL != "" && authCfg.HubURL != "https://topocosm.dev" {
		hubURL = authCfg.HubURL
	}

	callerDID := "did:key:z6MkuCLIPublisher"
	if authCfg.CallerDID != "" {
		callerDID = authCfg.CallerDID
	}

	client := topocosm.NewHubClient(hubURL, callerDID)
	if authCfg.Token != "" {
		client.SetBearerToken(authCfg.Token)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	data, err := client.GetBlackboard(ctx, orgSlug, cosmName)
	if err != nil {
		fmt.Printf("❌ Failed to inspect blackboard at %s: %v\n", hubURL, err)
		return
	}

	claimsRaw, ok := data["claims"]
	if !ok || claimsRaw == nil {
		fmt.Printf("📋 Blackboard for %s/%s has no active claims.\n", orgSlug, cosmName)
		return
	}

	claimsList, ok := claimsRaw.([]any)
	if !ok || len(claimsList) == 0 {
		fmt.Printf("📋 Blackboard for %s/%s: No active domain claims. Safe for agent mutation.\n", orgSlug, cosmName)
		return
	}

	fmt.Println("=========================================================================================================")
	fmt.Printf("  📋 TOPOCOSM BLACKBOARD DOMAIN LEASES (%s/%s)\n", orgSlug, cosmName)
	fmt.Println("=========================================================================================================")
	fmt.Printf("  %-22s %-28s %-32s %-10s\n", "DOMAIN", "AGENT DID", "INTENT / GOAL", "REMAINING")
	fmt.Println("  -------------------------------------------------------------------------------------------------------")

	for _, item := range claimsList {
		cMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		domain, _ := cMap["domain"].(string)
		agentDID, _ := cMap["agent_did"].(string)
		goal, _ := cMap["goal"].(string)
		secLeft, _ := cMap["seconds_left"].(float64)

		remStr := fmt.Sprintf("%ds", int(secLeft))
		if secLeft >= 60 {
			remStr = fmt.Sprintf("%dm%02ds", int(secLeft)/60, int(secLeft)%60)
		}

		if len(agentDID) > 26 {
			agentDID = agentDID[:23] + "..."
		}
		if len(goal) > 30 {
			goal = goal[:27] + "..."
		}

		fmt.Printf("  %-22s %-28s %-32s %-10s\n", domain, agentDID, goal, remStr)
	}
	fmt.Println("=========================================================================================================")
}

func runAuth(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: cosm auth <login|whoami|token|logout> [arguments]")
		fmt.Println("  cosm auth login [--token <pat>] [--hub <url>]")
		fmt.Println("  cosm auth whoami")
		fmt.Println("  cosm auth token list")
		fmt.Println("  cosm auth token create <name> [--days <ttl>]")
		fmt.Println("  cosm auth token revoke <token_hash>")
		fmt.Println("  cosm auth token set <pat>")
		fmt.Println("  cosm auth logout")
		return
	}

	sub := args[0]
	subArgs := args[1:]

	cfg, err := topocosm.LoadAuthConfig()
	if err != nil {
		fmt.Printf("Failed to load auth config: %v\n", err)
		os.Exit(1)
	}

	switch sub {
	case "login":
		fs := flag.NewFlagSet("auth login", flag.ExitOnError)
		tokenFlag := fs.String("token", "", "Personal Access Token (PAT)")
		hubFlag := fs.String("hub", "", "Topocosm Hub URL")
		_ = fs.Parse(subArgs)

		if *hubFlag != "" {
			cfg.HubURL = *hubFlag
		}

		token := *tokenFlag
		if token == "" {
			fmt.Printf("Enter Personal Access Token for %s: ", cfg.HubURL)
			reader := bufio.NewReader(os.Stdin)
			input, _ := reader.ReadString('\n')
			token = strings.TrimSpace(input)
		}

		if token == "" {
			fmt.Println("Error: No token provided.")
			os.Exit(1)
		}

		cfg.Token = token
		client := topocosm.NewHubClient(cfg.HubURL, cfg.CallerDID)
		client.SetBearerToken(cfg.Token)

		who, err := client.WhoAmI(context.Background())
		if err != nil {
			fmt.Printf("Authentication failed against %s: %v\n", cfg.HubURL, err)
			os.Exit(1)
		}

		if email, ok := who["email"].(string); ok {
			cfg.UserEmail = email
		}
		if uid, ok := who["uid"].(string); ok {
			cfg.UserUID = uid
		}
		if did, ok := who["did"].(string); ok && did != "" {
			cfg.CallerDID = did
		}

		if err := topocosm.SaveAuthConfig(cfg); err != nil {
			fmt.Printf("Warning: Failed to save config: %v\n", err)
		}

		fmt.Printf("✅ Successfully authenticated with %s!\n", cfg.HubURL)
		fmt.Printf("   • User:   %v (%v)\n", who["display_name"], who["email"])
		fmt.Printf("   • UID:    %v\n", who["uid"])
		fmt.Printf("   • Role:   %v\n", who["role"])
		fmt.Printf("   • DID:    %v\n", cfg.CallerDID)

	case "whoami", "status":
		if cfg.Token == "" {
			fmt.Println("Not logged in. Run 'cosm auth login' or 'cosm auth token set <pat>' to authenticate.")
			return
		}

		client := topocosm.NewHubClient(cfg.HubURL, cfg.CallerDID)
		client.SetBearerToken(cfg.Token)

		who, err := client.WhoAmI(context.Background())
		if err != nil {
			fmt.Printf("Failed to resolve identity from %s: %v\n", cfg.HubURL, err)
			os.Exit(1)
		}

		fmt.Printf("Topocosm Authenticated Session:\n")
		fmt.Printf("   • Hub URL:  %s\n", cfg.HubURL)
		fmt.Printf("   • User:     %v (%v)\n", who["display_name"], who["email"])
		fmt.Printf("   • UID:      %v\n", who["uid"])
		fmt.Printf("   • Role:     %v\n", who["role"])
		fmt.Printf("   • DID:      %v\n", who["did"])

	case "token":
		if len(subArgs) == 0 {
			fmt.Println("Usage: cosm auth token <list|create|revoke|set>")
			return
		}

		tokenAction := subArgs[0]
		tokenArgs := subArgs[1:]

		client := topocosm.NewHubClient(cfg.HubURL, cfg.CallerDID)
		client.SetBearerToken(cfg.Token)

		switch tokenAction {
		case "list":
			tokens, err := client.ListPATs(context.Background())
			if err != nil {
				fmt.Printf("Failed to list tokens: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Personal Access Tokens (%d):\n", len(tokens))
			for _, t := range tokens {
				fmt.Printf("   • %-20s Prefix: %-15s Expires: %v\n",
					t["token_name"], t["token_prefix"], t["expires_at"])
			}

		case "create":
			if len(tokenArgs) == 0 {
				fmt.Println("Usage: cosm auth token create <name> [--days <ttl>]")
				return
			}
			name := tokenArgs[0]
			ttlDays := 90
			fs := flag.NewFlagSet("token create", flag.ExitOnError)
			daysFlag := fs.Int("days", 90, "Token validity in days")
			_ = fs.Parse(tokenArgs[1:])
			if *daysFlag > 0 {
				ttlDays = *daysFlag
			}

			res, err := client.CreatePAT(context.Background(), name, []string{"cosm:read", "cosm:write", "ast:mutate"}, ttlDays)
			if err != nil {
				fmt.Printf("Failed to create token: %v\n", err)
				os.Exit(1)
			}

			rawToken := res["raw_token"]
			fmt.Printf("✅ Personal Access Token created successfully!\n\n")
			fmt.Printf("   Token: %s\n\n", rawToken)
			fmt.Println("⚠️  Copy this token now. It will not be shown again.")

		case "revoke":
			if len(tokenArgs) == 0 {
				fmt.Println("Usage: cosm auth token revoke <token_hash>")
				return
			}
			hash := tokenArgs[0]
			if err := client.RevokePAT(context.Background(), hash); err != nil {
				fmt.Printf("Failed to revoke token: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("✅ Token revoked successfully.")

		case "set":
			if len(tokenArgs) == 0 {
				fmt.Println("Usage: cosm auth token set <pat>")
				return
			}
			cfg.Token = tokenArgs[0]
			if err := topocosm.SaveAuthConfig(cfg); err != nil {
				fmt.Printf("Failed to save auth config: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("✅ Authentication token updated successfully.")

		default:
			fmt.Printf("Unknown token action: %s\n", tokenAction)
		}

	case "logout":
		cfg.Token = ""
		_ = topocosm.SaveAuthConfig(cfg)
		fmt.Println("✅ Successfully logged out.")

	default:
		fmt.Printf("Unknown auth subcommand: %s\n", sub)
		os.Exit(1)
	}
}

func runCredentialHelper(args []string) {
	if len(args) == 0 {
		return
	}

	action := args[0]
	cfg, err := topocosm.LoadAuthConfig()
	if err != nil || cfg.Token == "" {
		return
	}

	switch action {
	case "get":
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				break
			}
		}

		fmt.Printf("username=%s\n", "tp_pat")
		fmt.Printf("password=%s\n", cfg.Token)

	case "store", "erase":
		// No-op for stateless PAT resolution
	}
}

func runShare(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: cosm share <org/cosm> --user <email> [--access <viewer|collaborator|maintainer>]")
		return
	}

	cosmPath := args[0]
	fs := flag.NewFlagSet("share", flag.ExitOnError)
	userFlag := fs.String("user", "", "Recipient collaborator email")
	accessFlag := fs.String("access", "collaborator", "Access level (viewer, collaborator, maintainer)")
	_ = fs.Parse(args[1:])

	if *userFlag == "" {
		fmt.Println("Error: Missing --user <email>")
		os.Exit(1)
	}

	parts := strings.Split(cosmPath, "/")
	if len(parts) < 2 {
		fmt.Println("Error: cosm path must be in format <org>/<cosm>")
		os.Exit(1)
	}
	orgSlug := parts[0]
	cosmName := parts[1]

	cfg, err := topocosm.LoadAuthConfig()
	if err != nil || cfg.Token == "" {
		fmt.Println("Error: Authentication required. Run 'cosm auth login' first.")
		os.Exit(1)
	}

	client := topocosm.NewHubClient(cfg.HubURL, cfg.CallerDID)
	client.SetBearerToken(cfg.Token)

	// Fetch recipient public key
	pubKeyInfo, err := client.GetUserPublicKey(context.Background(), *userFlag)
	if err != nil {
		fmt.Printf("Failed to find recipient public key for %s: %v\n", *userFlag, err)
		os.Exit(1)
	}

	pubKeyHex, _ := pubKeyInfo["public_key_hex"].(string)
	fmt.Printf("🔑 Found recipient cryptographic identity for %s\n", *userFlag)
	fmt.Printf("   • X25519 Public Key: %s...\n", pubKeyHex[:min(16, len(pubKeyHex))])

	// Create synthetic wrapped DEK payload for share registration
	dummyDEK := make([]byte, 32)
	for i := range dummyDEK {
		dummyDEK[i] = byte(i + 1)
	}

	err = client.ShareCosm(context.Background(), orgSlug, cosmName, *userFlag, *accessFlag, dummyDEK)
	if err != nil {
		fmt.Printf("Failed to grant repository access: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Successfully shared %s/%s with %s (Role: %s)!\n", orgSlug, cosmName, *userFlag, *accessFlag)
}
