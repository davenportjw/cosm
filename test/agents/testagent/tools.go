package testagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cosmscm/cosm/pkg/codecs/golang"
	"github.com/cosmscm/cosm/pkg/codecs/hcl"
	"github.com/cosmscm/cosm/pkg/codecs/python"
	"github.com/cosmscm/cosm/pkg/codecs/typescript"
	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/materialize"
	"github.com/cosmscm/cosm/pkg/mutation"
	"github.com/cosmscm/cosm/pkg/shipping"
	"github.com/cosmscm/cosm/pkg/storage"
	"github.com/cosmscm/cosm/pkg/target"
	"github.com/cosmscm/cosm/pkg/topocosm"
	"github.com/cosmscm/cosm/test/agents/framework"
	"github.com/cosmscm/cosm/test/agents/llm"
)

// StorageHelper opens the .cosm blobstore and graph engine inside a workspace.
func openWorkspaceStorage(workDir string) (*storage.BlobStore, *storage.GraphEngine, error) {
	cosmDir := filepath.Join(workDir, ".cosm")
	objectsDir := filepath.Join(cosmDir, "objects")
	if err := os.MkdirAll(objectsDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("failed to create .cosm/objects: %w", err)
	}

	blobStore, err := storage.NewBlobStore(objectsDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open blobstore: %w", err)
	}

	graphEngine, err := storage.NewGraphEngine(filepath.Join(cosmDir, "graph.db"))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open graph engine: %w", err)
	}

	return blobStore, graphEngine, nil
}

// Tool Definition Constants

var ToolDefFGInit = llm.ToolDefinition{
	Name:        "fg_init",
	Description: "Initialize a new .cosm/ content-addressed AST repository in the workspace.",
	Parameters: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"universe_id": map[string]interface{}{
				"type":        "string",
				"description": "Initial micro-universe ID (default: universe-main)",
			},
		},
	},
}

var ToolDefFGAdd = llm.ToolDefinition{
	Name:        "fg_add",
	Description: "Parse polyglot source code files into fine-grained AST symbol nodes and stage to content-addressed storage.",
	Parameters: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"files": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "List of relative file paths to parse and stage",
			},
			"universe_id": map[string]interface{}{
				"type":        "string",
				"description": "Target micro-universe ID (default: universe-main)",
			},
			"intent": map[string]interface{}{
				"type":        "string",
				"description": "Semantic change intent description",
			},
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "User prompt that generated or requested this code",
			},
		},
		"required": []interface{}{"files"},
	},
}

var ToolDefFGCommit = llm.ToolDefinition{
	Name:        "fg_commit",
	Description: "Discover cross-boundary contracts, compute pairwise Merkle root, and commit active manifest to micro-universe head.",
	Parameters: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"universe_id": map[string]interface{}{
				"type":        "string",
				"description": "Micro-universe ID to commit into (default: universe-main)",
			},
			"intent": map[string]interface{}{
				"type":        "string",
				"description": "Commit intent description",
			},
		},
		"required": []interface{}{"intent"},
	},
}

var ToolDefFGUniverseCreate = llm.ToolDefinition{
	Name:        "fg_universe_create",
	Description: "Branch a new micro-universe from an existing parent universe head.",
	Parameters: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"new_universe_id": map[string]interface{}{
				"type":        "string",
				"description": "New micro-universe ID to create",
			},
			"parent_universe_id": map[string]interface{}{
				"type":        "string",
				"description": "Parent universe ID to branch from (default: universe-main)",
			},
		},
		"required": []interface{}{"new_universe_id"},
	},
}

var ToolDefFGSymbolEdit = llm.ToolDefinition{
	Name:        "fg_symbol_edit",
	Description: "Surgically mutate a single AST symbol's payload, deduplicating sibling symbols and generating a new Merkle head.",
	Parameters: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"symbol_id": map[string]interface{}{
				"type":        "string",
				"description": "Target AST Symbol Node ID to mutate",
			},
			"new_payload": map[string]interface{}{
				"type":        "string",
				"description": "New AST payload / source code body for the symbol",
			},
			"universe_id": map[string]interface{}{
				"type":        "string",
				"description": "Micro-universe ID (default: universe-main)",
			},
			"intent": map[string]interface{}{
				"type":        "string",
				"description": "Intent describing the surgical change",
			},
		},
		"required": []interface{}{"symbol_id", "new_payload"},
	},
}

var ToolDefFGShip = llm.ToolDefinition{
	Name:        "fg_ship",
	Description: "Reconstitute universe ASTs, compile target artifacts, and validate preview health check.",
	Parameters: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"universe_id": map[string]interface{}{
				"type":        "string",
				"description": "Micro-universe ID to ship (default: universe-main)",
			},
		},
	},
}

var ToolDefFGProposalCreate = llm.ToolDefinition{
	Name:        "fg_proposal_create",
	Description: "Create a universe proposal comparing a feature micro-universe against the base universe.",
	Parameters: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"source_universe": map[string]interface{}{
				"type":        "string",
				"description": "Source micro-universe with proposed changes",
			},
			"target_universe": map[string]interface{}{
				"type":        "string",
				"description": "Base target micro-universe (default: universe-main)",
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Proposal title",
			},
		},
		"required": []interface{}{"source_universe"},
	},
}

var ToolDefCosmInit = llm.ToolDefinition{
	Name:        "cosm_init",
	Description: ToolDefFGInit.Description,
	Parameters:  ToolDefFGInit.Parameters,
}

var ToolDefCosmAdd = llm.ToolDefinition{
	Name:        "cosm_add",
	Description: ToolDefFGAdd.Description,
	Parameters:  ToolDefFGAdd.Parameters,
}

var ToolDefCosmCommit = llm.ToolDefinition{
	Name:        "cosm_commit",
	Description: ToolDefFGCommit.Description,
	Parameters:  ToolDefFGCommit.Parameters,
}

var ToolDefCosmUniverseCreate = llm.ToolDefinition{
	Name:        "cosm_universe_create",
	Description: ToolDefFGUniverseCreate.Description,
	Parameters:  ToolDefFGUniverseCreate.Parameters,
}

var ToolDefCosmSymbolEdit = llm.ToolDefinition{
	Name:        "cosm_symbol_edit",
	Description: ToolDefFGSymbolEdit.Description,
	Parameters:  ToolDefFGSymbolEdit.Parameters,
}

var ToolDefCosmShip = llm.ToolDefinition{
	Name:        "cosm_ship",
	Description: ToolDefFGShip.Description,
	Parameters:  ToolDefFGShip.Parameters,
}

var ToolDefCosmProposalCreate = llm.ToolDefinition{
	Name:        "cosm_proposal_create",
	Description: ToolDefFGProposalCreate.Description,
	Parameters:  ToolDefFGProposalCreate.Parameters,
}

var ToolDefFGASTEdit = llm.ToolDefinition{
	Name:        "fg_ast_edit",
	Description: "Execute declarative AST mutation operations (replace_function_body, replace_function, add_method, add_before, add_after, delete, add_import, replace_imports, replace_global) on target symbol nodes without file transcription errors.",
	Parameters: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"universe_id": map[string]interface{}{
				"type":        "string",
				"description": "Target micro-universe ID (default: universe-main)",
			},
			"operation": map[string]interface{}{
				"type":        "string",
				"description": "Declarative operation: replace_function_body, replace_function, add_method, add_before, add_after, delete, add_import, replace_imports, replace_global",
			},
			"target": map[string]interface{}{
				"type":        "string",
				"description": "Target symbol scoped path (e.g. 'LRUCache.get', 'services/auth::ValidateToken') or symbol node ID",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Source code snippet, function body, or new symbol content",
			},
			"operations": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "object"},
				"description": "Optional batch of multiple declarative operations",
			},
			"intent": map[string]interface{}{
				"type":        "string",
				"description": "Mutation intent description",
			},
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "User prompt that triggered the AST edit",
			},
		},
	},
}

var ToolDefCosmASTEdit = llm.ToolDefinition{
	Name:        "cosm_ast_edit",
	Description: ToolDefFGASTEdit.Description,
	Parameters:  ToolDefFGASTEdit.Parameters,
}

var ToolDefFGASTResolve = llm.ToolDefinition{
	Name:        "fg_ast_resolve",
	Description: "Resolve an AST symbol and its enclosing component by scoped path or identifier within a micro-universe.",
	Parameters: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"universe_id": map[string]interface{}{
				"type":        "string",
				"description": "Micro-universe ID (default: universe-main)",
			},
			"target": map[string]interface{}{
				"type":        "string",
				"description": "Target symbol scoped identifier or name",
			},
		},
		"required": []interface{}{"target"},
	},
}

var ToolDefCosmASTResolve = llm.ToolDefinition{
	Name:        "cosm_ast_resolve",
	Description: ToolDefFGASTResolve.Description,
	Parameters:  ToolDefFGASTResolve.Parameters,
}

var ToolDefWriteFile = llm.ToolDefinition{
	Name:        "write_file",
	Description: "Write content to a file in the workspace.",
	Parameters: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Relative file path",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "File content",
			},
		},
		"required": []interface{}{"path", "content"},
	},
}

var ToolDefReadFile = llm.ToolDefinition{
	Name:        "read_file",
	Description: "Read content from a workspace file.",
	Parameters: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Relative file path",
			},
		},
		"required": []interface{}{"path"},
	},
}

var ToolDefListFiles = llm.ToolDefinition{
	Name:        "list_files",
	Description: "List files and directories in the workspace.",
	Parameters: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"dir": map[string]interface{}{
				"type":        "string",
				"description": "Directory path relative to workspace (default: .)",
			},
		},
	},
}

var ToolDefCosmClaim = llm.ToolDefinition{
	Name:        "cosm_claim",
	Description: "Acquire or release a mutual exclusion domain lease on the Topocosm blackboard coordination service.",
	Parameters: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"hub_url": map[string]interface{}{
				"type":        "string",
				"description": "Topocosm Hub base URL (e.g. http://127.0.0.1:8080)",
			},
			"repo": map[string]interface{}{
				"type":        "string",
				"description": "Repository identifier (org/repo, e.g. demo-org/fintech-payment-mesh)",
			},
			"domain": map[string]interface{}{
				"type":        "string",
				"description": "Domain or component prefix to lock (e.g. services/orders)",
			},
			"purpose": map[string]interface{}{
				"type":        "string",
				"description": "Purpose or task goal justifying the claim",
			},
			"ttl_sec": map[string]interface{}{
				"type":        "integer",
				"description": "Lease duration in seconds (default: 300)",
			},
			"release": map[string]interface{}{
				"type":        "boolean",
				"description": "Set to true to release the domain lease instead of acquiring",
			},
			"agent_did": map[string]interface{}{
				"type":        "string",
				"description": "Agent decentralized identity",
			},
		},
		"required": []interface{}{"repo", "domain"},
	},
}

var ToolDefCosmClone = llm.ToolDefinition{
	Name:        "cosm_clone",
	Description: "Clone repository or perform sparse subtree fetch from remote Topocosm Hub.",
	Parameters: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"hub_url": map[string]interface{}{
				"type":        "string",
				"description": "Topocosm Hub base URL",
			},
			"repo": map[string]interface{}{
				"type":        "string",
				"description": "Repository identifier (org/repo)",
			},
			"universe_id": map[string]interface{}{
				"type":        "string",
				"description": "Universe ID to pull (default: universe-main)",
			},
			"components": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Optional component names for sparse extraction (e.g. ['apps/checkout'])",
			},
		},
		"required": []interface{}{"repo"},
	},
}

var ToolDefCosmPublish = llm.ToolDefinition{
	Name:        "cosm_publish",
	Description: "Publish local micro-universe AST Merkle-DAG and content blobs to remote Topocosm Hub.",
	Parameters: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"hub_url": map[string]interface{}{
				"type":        "string",
				"description": "Topocosm Hub base URL",
			},
			"repo": map[string]interface{}{
				"type":        "string",
				"description": "Repository identifier (org/repo)",
			},
			"universe_id": map[string]interface{}{
				"type":        "string",
				"description": "Micro-universe ID to publish",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Publication description",
			},
		},
		"required": []interface{}{"repo", "universe_id"},
	},
}

var ToolDefCosmStackCreate = llm.ToolDefinition{
	Name:        "cosm_stack_create",
	Description: "Open a stacked proposal on Topocosm Hub referencing a parent proposal or base branch.",
	Parameters: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"hub_url": map[string]interface{}{
				"type":        "string",
				"description": "Topocosm Hub base URL",
			},
			"repo": map[string]interface{}{
				"type":        "string",
				"description": "Repository identifier (org/repo)",
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Proposal title",
			},
			"source_universe": map[string]interface{}{
				"type":        "string",
				"description": "Branch universe containing the changes",
			},
			"target_universe": map[string]interface{}{
				"type":        "string",
				"description": "Target universe (e.g. universe-main)",
			},
			"parent_proposal_id": map[string]interface{}{
				"type":        "string",
				"description": "Parent proposal ID for Jujutsu-style stacking",
			},
		},
		"required": []interface{}{"repo", "title", "source_universe"},
	},
}

// RegisterAllCosmTools registers all cosm CLI tool handlers into the framework tool registry.
func RegisterAllCosmTools(registry *framework.ToolRegistry, workDir string, defaultAgentID string) {
	if defaultAgentID == "" {
		defaultAgentID = "cosm-autonomous-test-agent"
	}

	// 1. cosm_init / fg_init
	initHandler := func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			UniverseID string `json:"universe_id"`
		}
		_ = call.ParseArguments(&args)
		if args.UniverseID == "" {
			args.UniverseID = "universe-main"
		}

		blobStore, graphEngine, err := openWorkspaceStorage(workDir)
		if err != nil {
			return "", err
		}
		defer graphEngine.Close()

		mgr := storage.NewUniverseManager(graphEngine, blobStore)
		rec, err := mgr.CreateUniverse(args.UniverseID, "")
		if err != nil {
			return "", fmt.Errorf("failed to create universe: %w", err)
		}

		res := map[string]interface{}{
			"status":      "initialized",
			"universe_id": rec.UniverseID,
			"objects_dir": blobStore.ObjectsDir(),
			"message":     fmt.Sprintf("Initialized .cosm/ repository in %s with active universe %s", workDir, rec.UniverseID),
		}
		out, _ := json.Marshal(res)
		return string(out), nil
	}
	registry.Register(ToolDefCosmInit, initHandler)
	registry.Register(ToolDefFGInit, initHandler)

	// 2. cosm_add / fg_add
	addHandler := func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Files      []string `json:"files"`
			UniverseID string   `json:"universe_id"`
			Intent     string   `json:"intent"`
			Prompt     string   `json:"prompt"`
		}
		if err := call.ParseArguments(&args); err != nil {
			return "", err
		}
		if len(args.Files) == 0 {
			return "", fmt.Errorf("no files specified to add")
		}
		if args.UniverseID == "" {
			args.UniverseID = "universe-main"
		}
		if args.Intent == "" {
			args.Intent = "Stage polyglot source code"
		}

		blobStore, graphEngine, err := openWorkspaceStorage(workDir)
		if err != nil {
			return "", err
		}
		defer graphEngine.Close()

		lineageEnv := core.LineageEnvelope{
			UserID:           os.Getenv("USER"),
			UserPrompt:       args.Prompt,
			ExecutingAgentID: defaultAgentID,
			Intent:           args.Intent,
			Timestamp:        time.Now().UTC(),
		}

		var stagedComponents []string
		stagedSymbolsCount := 0

		for _, relPath := range args.Files {
			fullPath := filepath.Join(workDir, relPath)
			content, err := os.ReadFile(fullPath)
			if err != nil {
				return "", fmt.Errorf("failed to read %s: %w", relPath, err)
			}

			var comp *core.ComponentNode
			syms := make(map[string]*core.ASTSymbolNode)
			ext := filepath.Ext(relPath)

			switch ext {
			case ".go":
				parser := golang.NewGoParser()
				pkgRes, pErr := parser.ParseSource(relPath, content, lineageEnv)
				if pErr != nil {
					return "", fmt.Errorf("Go parse error on %s: %w", relPath, pErr)
				}
				comp, err = golang.BuildComponentNode(filepath.Base(relPath), core.CompService, pkgRes, lineageEnv)
				for _, s := range pkgRes.AllSymbols {
					syms[s.NodeID] = s
				}

			case ".py":
				parser := python.NewPythonParser()
				res, pErr := parser.ParseSource(relPath, content, lineageEnv)
				if pErr != nil {
					return "", fmt.Errorf("Python parse error on %s: %w", relPath, pErr)
				}
				var symList []*core.ASTSymbolNode
				comp, symList, err = parser.BuildComponentNode(res, filepath.Base(relPath), lineageEnv)
				for _, s := range symList {
					syms[s.NodeID] = s
				}

			case ".tf", ".hcl":
				parser := hcl.NewHCLParser()
				doc, pErr := parser.ParseSource(relPath, content)
				if pErr != nil {
					return "", fmt.Errorf("HCL parse error on %s: %w", relPath, pErr)
				}
				var symList []*core.ASTSymbolNode
				comp, symList, err = parser.BuildComponentNode(doc, filepath.Base(relPath), lineageEnv)
				for _, s := range symList {
					syms[s.NodeID] = s
				}

			case ".ts", ".tsx", ".js", ".jsx", ".vue":
				parser := typescript.NewTSParser()
				res, pErr := parser.ParseSource(relPath, content, lineageEnv)
				if pErr != nil {
					return "", fmt.Errorf("TypeScript parse error on %s: %w", relPath, pErr)
				}
				var symList []*core.ASTSymbolNode
				comp, symList, err = parser.BuildComponentNode(res, filepath.Base(relPath), lineageEnv)
				for _, s := range symList {
					syms[s.NodeID] = s
				}

			default:
				// Generic raw component
				sum := sha256.Sum256(content)
				hash := hex.EncodeToString(sum[:])
				rawSym := &core.ASTSymbolNode{
					NodeID:     hash,
					Language:   core.Language(strings.TrimPrefix(ext, ".")),
					NodeType:   "RawPayload",
					Identifier: filepath.Base(relPath),
					ASTPayload: content,
					Lineage:    lineageEnv,
				}
				syms[hash] = rawSym
				comp = &core.ComponentNode{
					ComponentID: hash,
					Name:        filepath.Base(relPath),
					Type:        core.CompService,
					Language:    rawSym.Language,
					SymbolNodes: []string{hash},
					Lineage:     lineageEnv,
				}
			}

			if err != nil {
				return "", fmt.Errorf("failed building component node for %s: %w", relPath, err)
			}

			if comp.Metadata == nil {
				comp.Metadata = make(map[string]string)
			}
			comp.Metadata["file_path"] = relPath

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
				stagedSymbolsCount++
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
				RecordID:         fmt.Sprintf("lin-%s", comp.ComponentID[:12]),
				NodeID:           comp.ComponentID,
				UserID:           lineageEnv.UserID,
				UserPrompt:       lineageEnv.UserPrompt,
				ExecutingAgentID: lineageEnv.ExecutingAgentID,
				Intent:           lineageEnv.Intent,
				Timestamp:        lineageEnv.Timestamp,
			})
			stagedComponents = append(stagedComponents, comp.ComponentID)
		}

		res := map[string]interface{}{
			"status":            "staged",
			"files_count":       len(args.Files),
			"components_count":  len(stagedComponents),
			"symbols_count":     stagedSymbolsCount,
			"staged_components": stagedComponents,
		}
		out, _ := json.Marshal(res)
		return string(out), nil
	}
	registry.Register(ToolDefCosmAdd, addHandler)
	registry.Register(ToolDefFGAdd, addHandler)

	// 3. cosm_commit / fg_commit
	commitHandler := func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			UniverseID string `json:"universe_id"`
			Intent     string `json:"intent"`
		}
		if err := call.ParseArguments(&args); err != nil {
			return "", err
		}
		if args.UniverseID == "" {
			args.UniverseID = "universe-main"
		}
		if args.Intent == "" {
			args.Intent = "Autonomous commit"
		}

		blobStore, graphEngine, err := openWorkspaceStorage(workDir)
		if err != nil {
			return "", err
		}
		defer graphEngine.Close()

		nodes := graphEngine.ListNodes("", "")
		var compIDs []string
		symbolMap := make(map[string]*core.ASTSymbolNode)
		var components []*core.ComponentNode

		for _, n := range nodes {
			if n.NodeType == "service" || n.NodeType == "infra" || n.NodeType == "frontend" || n.NodeType == "contract" {
				compIDs = append(compIDs, n.NodeID)
				data, err := blobStore.Get(n.MerkleHash)
				if err == nil {
					var c core.ComponentNode
					if e := json.Unmarshal(data, &c); e == nil {
						components = append(components, &c)
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

		linker := core.NewCrossBoundaryLinker()
		edges, _ := linker.LinkWorkspace(components, symbolMap)

		manifest := &core.WorkspaceManifestNode{
			WorkspaceID: filepath.Base(workDir),
			UniverseID:  args.UniverseID,
			Components:  compIDs,
			CrossEdges:  edges,
			Lineage: core.LineageEnvelope{
				UserID:           os.Getenv("USER"),
				ExecutingAgentID: defaultAgentID,
				Intent:           args.Intent,
				Timestamp:        time.Now().UTC(),
			},
			CreatedAt: time.Now().UTC(),
		}

		manifestHash, err := core.HashWorkspaceManifest(manifest)
		if err != nil {
			return "", fmt.Errorf("failed to hash manifest: %w", err)
		}
		manifest.MerkleRootHash = manifestHash

		universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
		_, err = universeMgr.CommitManifest(args.UniverseID, manifest)
		if err != nil {
			return "", fmt.Errorf("failed committing to universe: %w", err)
		}

		res := map[string]interface{}{
			"status":           "committed",
			"universe_id":      args.UniverseID,
			"merkle_root_hash": manifestHash,
			"components_count": len(compIDs),
			"cross_edges":      len(edges),
		}
		out, _ := json.Marshal(res)
		return string(out), nil
	}
	registry.Register(ToolDefCosmCommit, commitHandler)
	registry.Register(ToolDefFGCommit, commitHandler)

	// 4. cosm_universe_create / fg_universe_create
	univCreateHandler := func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			NewUniverseID    string `json:"new_universe_id"`
			ParentUniverseID string `json:"parent_universe_id"`
		}
		if err := call.ParseArguments(&args); err != nil {
			return "", err
		}
		if args.NewUniverseID == "" {
			return "", fmt.Errorf("new_universe_id is required")
		}
		if args.ParentUniverseID == "" {
			args.ParentUniverseID = "universe-main"
		}

		blobStore, graphEngine, err := openWorkspaceStorage(workDir)
		if err != nil {
			return "", err
		}
		defer graphEngine.Close()

		mgr := storage.NewUniverseManager(graphEngine, blobStore)
		rec, err := mgr.CreateUniverse(args.NewUniverseID, args.ParentUniverseID)
		if err != nil {
			return "", err
		}

		res := map[string]interface{}{
			"status":             "created",
			"universe_id":        rec.UniverseID,
			"parent_universe_id": rec.ParentUniverseID,
			"head_manifest_hash": rec.HeadManifestHash,
		}
		out, _ := json.Marshal(res)
		return string(out), nil
	}
	registry.Register(ToolDefCosmUniverseCreate, univCreateHandler)
	registry.Register(ToolDefFGUniverseCreate, univCreateHandler)

	// 5. cosm_symbol_edit / fg_symbol_edit
	symEditHandler := func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			SymbolID   string `json:"symbol_id"`
			NewPayload string `json:"new_payload"`
			UniverseID string `json:"universe_id"`
			Intent     string `json:"intent"`
		}
		if err := call.ParseArguments(&args); err != nil {
			return "", err
		}
		if args.SymbolID == "" || args.NewPayload == "" {
			return "", fmt.Errorf("symbol_id and new_payload are required")
		}
		if args.UniverseID == "" {
			args.UniverseID = "universe-main"
		}
		if args.Intent == "" {
			args.Intent = "Surgical AST Symbol Edit"
		}

		blobStore, graphEngine, err := openWorkspaceStorage(workDir)
		if err != nil {
			return "", err
		}
		defer graphEngine.Close()

		universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
		surgeryEngine := mutation.NewSurgeryEngine(blobStore, graphEngine, universeMgr)

		lineageEnv := core.LineageEnvelope{
			UserID:           os.Getenv("USER"),
			UserPrompt:       "Surgical AST symbol mutation",
			ExecutingAgentID: defaultAgentID,
			Intent:           args.Intent,
			Timestamp:        time.Now().UTC(),
		}

		mutationRes, err := surgeryEngine.MutateSymbol(args.UniverseID, args.SymbolID, []byte(args.NewPayload), lineageEnv)
		if err != nil {
			return "", fmt.Errorf("surgery error: %w", err)
		}

		res := map[string]interface{}{
			"status":                     "mutated",
			"universe_id":                mutationRes.UniverseID,
			"old_symbol_id":              mutationRes.OldSymbolID,
			"new_symbol_id":              mutationRes.NewSymbolID,
			"old_manifest_hash":          mutationRes.OldManifestHash,
			"new_manifest_hash":          mutationRes.NewManifestHash,
			"deduplicated_symbols_count": mutationRes.DeduplicatedSymbolsCount,
		}
		out, _ := json.Marshal(res)
		return string(out), nil
	}
	registry.Register(ToolDefCosmSymbolEdit, symEditHandler)
	registry.Register(ToolDefFGSymbolEdit, symEditHandler)

	// 6. cosm_ship / fg_ship
	shipHandler := func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			UniverseID string `json:"universe_id"`
		}
		_ = call.ParseArguments(&args)
		if args.UniverseID == "" {
			args.UniverseID = "universe-main"
		}

		blobStore, graphEngine, err := openWorkspaceStorage(workDir)
		if err != nil {
			return "", err
		}
		defer graphEngine.Close()

		universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
		head, err := universeMgr.GetUniverseManifest(args.UniverseID)
		if err != nil || head == nil {
			return "", fmt.Errorf("no manifest head found in universe %s", args.UniverseID)
		}

		compMap := make(map[string]*core.ComponentNode)
		symMap := make(map[string]*core.ASTSymbolNode)

		for _, compID := range head.Components {
			var compNode core.ComponentNode
			if data, gErr := blobStore.Get(compID); gErr == nil {
				_ = json.Unmarshal(data, &compNode)
			} else if nodeRec, nErr := graphEngine.GetNode(compID); nErr == nil && nodeRec != nil {
				if d, bErr := blobStore.Get(nodeRec.MerkleHash); bErr == nil {
					_ = json.Unmarshal(d, &compNode)
				}
			}
			if compNode.ComponentID != "" {
				compMap[compNode.ComponentID] = &compNode
				for _, symID := range compNode.SymbolNodes {
					var symNode core.ASTSymbolNode
					if sData, sErr := blobStore.Get(symID); sErr == nil {
						_ = json.Unmarshal(sData, &symNode)
					} else if sRec, snErr := graphEngine.GetNode(symID); snErr == nil && sRec != nil {
						if sd, sbErr := blobStore.Get(sRec.MerkleHash); sbErr == nil {
							_ = json.Unmarshal(sd, &symNode)
						}
					}
					if symNode.NodeID != "" {
						symMap[symNode.NodeID] = &symNode
					}
				}
			}
		}

		hydrator := materialize.NewHydrator()
		files, err := hydrator.HydrateWorkspace(head, compMap, symMap)
		if err != nil {
			return "", fmt.Errorf("failed to hydrate workspace files: %w", err)
		}

		distDir := filepath.Join(workDir, "dist")
		packager := shipping.NewPackager(nil)
		targetSpec := shipping.DefaultLocalServiceTarget("target:local-preview", nil)
		art, err := packager.BuildAndPackageTarget(targetSpec, files, distDir)
		if err != nil {
			return "", fmt.Errorf("target packaging failed: %w", err)
		}

		sandbox := target.NewPreviewSandbox()
		inst, err := sandbox.Start(targetSpec, files)
		if err != nil {
			return "", fmt.Errorf("failed starting preview sandbox: %w", err)
		}

		res := map[string]interface{}{
			"status":       "shipped",
			"artifact_id":  art.ArtifactID,
			"size_bytes":   art.SizeBytes,
			"preview_url":  inst.URL,
			"health_check": inst.HealthCheck(),
		}
		out, _ := json.Marshal(res)
		return string(out), nil
	}
	registry.Register(ToolDefCosmShip, shipHandler)
	registry.Register(ToolDefFGShip, shipHandler)

	// 7. cosm_proposal_create / fg_proposal_create
	proposalCreateHandler := func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			SourceUniverse string `json:"source_universe"`
			TargetUniverse string `json:"target_universe"`
			Title          string `json:"title"`
		}
		if err := call.ParseArguments(&args); err != nil {
			return "", err
		}
		if args.SourceUniverse == "" {
			return "", fmt.Errorf("source_universe is required")
		}
		if args.TargetUniverse == "" {
			args.TargetUniverse = "universe-main"
		}

		blobStore, graphEngine, err := openWorkspaceStorage(workDir)
		if err != nil {
			return "", err
		}
		defer graphEngine.Close()

		mgr := storage.NewUniverseManager(graphEngine, blobStore)
		diff, err := mgr.DiffUniverses(args.SourceUniverse, args.TargetUniverse)
		if err != nil {
			return "", fmt.Errorf("diff error: %w", err)
		}

		res := map[string]interface{}{
			"status":             "proposal_created",
			"proposal_id":        fmt.Sprintf("prop-%s-%s", args.SourceUniverse, args.TargetUniverse),
			"title":              args.Title,
			"source_universe":    args.SourceUniverse,
			"target_universe":    args.TargetUniverse,
			"added_components":   diff.AddedComponents,
			"removed_components": diff.RemovedComponents,
		}
		out, _ := json.Marshal(res)
		return string(out), nil
	}
	registry.Register(ToolDefCosmProposalCreate, proposalCreateHandler)
	registry.Register(ToolDefFGProposalCreate, proposalCreateHandler)

	// 8. write_file
	registry.Register(ToolDefWriteFile, func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := call.ParseArguments(&args); err != nil {
			return "", err
		}
		fullPath := filepath.Join(workDir, args.Path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(fullPath, []byte(args.Content), 0644); err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"status":"written","path":%q,"bytes":%d}`, args.Path, len(args.Content)), nil
	})

	// 9. read_file
	registry.Register(ToolDefReadFile, func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Path string `json:"path"`
		}
		if err := call.ParseArguments(&args); err != nil {
			return "", err
		}
		fullPath := filepath.Join(workDir, args.Path)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return "", err
		}
		return string(data), nil
	})

	// 10. list_files
	registry.Register(ToolDefListFiles, func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Dir string `json:"dir"`
		}
		_ = call.ParseArguments(&args)
		targetDir := filepath.Join(workDir, args.Dir)
		var files []string
		_ = filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(workDir, path)
			if !strings.HasPrefix(rel, ".cosm") && !strings.HasPrefix(rel, ".git") {
				files = append(files, rel)
			}
			return nil
		})
		out, _ := json.Marshal(files)
		return string(out), nil
	})

	// 11. cosm_ast_edit / fg_ast_edit
	astEditHandler := func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			UniverseID string                  `json:"universe_id"`
			Operation  string                  `json:"operation"`
			Target     string                  `json:"target"`
			Content    string                  `json:"content"`
			Operations []mutation.ASTOperation `json:"operations"`
			Intent     string                  `json:"intent"`
			Prompt     string                  `json:"prompt"`
		}
		if err := call.ParseArguments(&args); err != nil {
			return "", err
		}
		if args.UniverseID == "" {
			args.UniverseID = "universe-main"
		}
		if args.Intent == "" {
			args.Intent = "Declarative AST Edit"
		}

		blobStore, graphEngine, err := openWorkspaceStorage(workDir)
		if err != nil {
			return "", err
		}
		defer graphEngine.Close()

		universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
		surgeryEngine := mutation.NewSurgeryEngine(blobStore, graphEngine, universeMgr)

		lineageEnv := core.LineageEnvelope{
			ExecutingAgentID: defaultAgentID,
			Intent:           args.Intent,
			UserPrompt:       args.Prompt,
			Timestamp:        time.Now().UTC(),
		}

		var ops []mutation.ASTOperation
		if len(args.Operations) > 0 {
			ops = args.Operations
		} else if args.Target != "" {
			opType := mutation.ASTOperationType(args.Operation)
			if opType == "" {
				opType = mutation.OpReplaceFunctionBody
			}
			ops = []mutation.ASTOperation{
				{
					Operation: opType,
					Target:    args.Target,
					Content:   args.Content,
				},
			}
		} else {
			return "", fmt.Errorf("target or operations list required for ast_edit")
		}

		batch := &mutation.ASTEditBatch{
			UniverseID: args.UniverseID,
			Operations: ops,
			Lineage:    lineageEnv,
		}

		res, err := surgeryEngine.ApplyASTEditBatch(batch)
		if err != nil {
			return "", fmt.Errorf("ast edit batch failed: %w", err)
		}

		out, _ := json.Marshal(res)
		return string(out), nil
	}
	registry.Register(ToolDefCosmASTEdit, astEditHandler)
	registry.Register(ToolDefFGASTEdit, astEditHandler)

	// 12. cosm_ast_resolve / fg_ast_resolve
	astResolveHandler := func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			UniverseID string `json:"universe_id"`
			Target     string `json:"target"`
		}
		if err := call.ParseArguments(&args); err != nil {
			return "", err
		}
		if args.UniverseID == "" {
			args.UniverseID = "universe-main"
		}

		blobStore, graphEngine, err := openWorkspaceStorage(workDir)
		if err != nil {
			return "", err
		}
		defer graphEngine.Close()

		universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
		surgeryEngine := mutation.NewSurgeryEngine(blobStore, graphEngine, universeMgr)

		resolved, err := surgeryEngine.ResolveSymbol(args.UniverseID, args.Target)
		if err != nil {
			return "", fmt.Errorf("ast resolve failed: %w", err)
		}

		out, _ := json.Marshal(resolved)
		return string(out), nil
	}
	registry.Register(ToolDefCosmASTResolve, astResolveHandler)
	registry.Register(ToolDefFGASTResolve, astResolveHandler)

	// 13. cosm_claim
	claimHandler := func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			HubURL   string `json:"hub_url"`
			Repo     string `json:"repo"`
			Domain   string `json:"domain"`
			Purpose  string `json:"purpose"`
			TTLSec   int    `json:"ttl_sec"`
			Release  bool   `json:"release"`
			AgentDID string `json:"agent_did"`
		}
		if err := call.ParseArguments(&args); err != nil {
			return "", err
		}
		if args.HubURL == "" {
			args.HubURL = os.Getenv("TOPOCOSM_HUB_URL")
		}
		if args.HubURL == "" {
			args.HubURL = "http://127.0.0.1:8080"
		}
		if args.AgentDID == "" {
			args.AgentDID = defaultAgentID
		}
		if args.TTLSec <= 0 {
			args.TTLSec = 300
		}
		parts := strings.Split(args.Repo, "/")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid repo format %q, expected org/repo", args.Repo)
		}
		orgSlug, cosmName := parts[0], parts[1]

		client := topocosm.NewHubClient(args.HubURL, args.AgentDID)
		if tid := os.Getenv("COSM_TRACE_ID"); tid != "" {
			client.SetTraceID(tid)
		}
		if sid := os.Getenv("COSM_SESSION_ID"); sid != "" {
			client.SetSessionID(sid)
		}

		if args.Release {
			err := client.ReleaseDomain(ctx, orgSlug, cosmName, args.Domain)
			if err != nil {
				return "", fmt.Errorf("failed to release domain: %w", err)
			}
			res := map[string]interface{}{
				"status":  "released",
				"domain":  args.Domain,
				"message": fmt.Sprintf("Domain %s released successfully", args.Domain),
			}
			out, _ := json.Marshal(res)
			return string(out), nil
		}

		claimed, err := client.ClaimDomain(ctx, orgSlug, cosmName, args.Domain, args.Purpose, args.TTLSec)
		if err != nil {
			return "", fmt.Errorf("claim domain failed: %w", err)
		}
		res := map[string]interface{}{
			"status":  "claimed",
			"success": claimed,
			"domain":  args.Domain,
			"ttl_sec": args.TTLSec,
			"message": fmt.Sprintf("Domain %s claimed successfully for %d seconds", args.Domain, args.TTLSec),
		}
		out, _ := json.Marshal(res)
		return string(out), nil
	}
	registry.Register(ToolDefCosmClaim, claimHandler)

	// 14. cosm_clone
	cloneHandler := func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			HubURL     string   `json:"hub_url"`
			Repo       string   `json:"repo"`
			UniverseID string   `json:"universe_id"`
			Components []string `json:"components"`
		}
		if err := call.ParseArguments(&args); err != nil {
			return "", err
		}
		if args.HubURL == "" {
			args.HubURL = os.Getenv("TOPOCOSM_HUB_URL")
		}
		if args.HubURL == "" {
			args.HubURL = "http://127.0.0.1:8080"
		}
		if args.UniverseID == "" {
			args.UniverseID = "universe-main"
		}
		parts := strings.Split(args.Repo, "/")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid repo format %q, expected org/repo", args.Repo)
		}
		orgSlug, cosmName := parts[0], parts[1]

		agentDID := os.Getenv("COSM_AGENT_DID")
		if agentDID == "" {
			agentDID = defaultAgentID
		}
		client := topocosm.NewHubClient(args.HubURL, agentDID)
		if tid := os.Getenv("COSM_TRACE_ID"); tid != "" {
			client.SetTraceID(tid)
		}
		if sid := os.Getenv("COSM_SESSION_ID"); sid != "" {
			client.SetSessionID(sid)
		}

		blobStore, graphEngine, err := openWorkspaceStorage(workDir)
		if err != nil {
			return "", err
		}
		defer graphEngine.Close()

		mgr := storage.NewUniverseManager(graphEngine, blobStore)
		_, _ = mgr.CreateUniverse(args.UniverseID, "")

		req := &topocosm.SparsePullRequest{
			OrgSlug:        orgSlug,
			CosmName:       cosmName,
			UniverseID:     args.UniverseID,
			ComponentNames: args.Components,
		}
		var resp *topocosm.SparsePullResponse
		var pullErr error
		for attempt := 0; attempt < 15; attempt++ {
			resp, pullErr = client.SparsePullCosm(ctx, req)
			if pullErr == nil {
				break
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
		if pullErr != nil {
			return "", fmt.Errorf("sparse pull failed: %w", pullErr)
		}

		// Store blobs
		for _, data := range resp.Blobs {
			_, _ = blobStore.Put(data)
		}
		// Store symbols
		for _, sym := range resp.Symbols {
			symJSON, _ := json.Marshal(sym)
			symHash, _ := blobStore.Put(symJSON)
			nodeRec := storage.NodeRecord{
				NodeID:     sym.NodeID,
				NodeType:   sym.NodeType,
				Language:   sym.Language,
				Identifier: sym.Identifier,
				MerkleHash: symHash,
				CreatedAt:  time.Now(),
			}
			_ = graphEngine.PutNode(nodeRec)
		}
		// Store components
		for _, comp := range resp.Components {
			compJSON, _ := json.Marshal(comp)
			compHash, _ := blobStore.Put(compJSON)
			compRec := storage.NodeRecord{
				NodeID:     comp.ComponentID,
				NodeType:   string(comp.Type),
				Language:   comp.Language,
				Identifier: comp.Name,
				MerkleHash: compHash,
				CreatedAt:  time.Now(),
			}
			_ = graphEngine.PutNode(compRec)
		}
		if resp.Manifest != nil {
			_, _ = mgr.CommitManifest(args.UniverseID, resp.Manifest)
		}

		// Hydrate files to workspace disk
		hydrator := materialize.NewHydrator()
		files, err := hydrator.HydrateWorkspace(resp.Manifest, resp.Components, resp.Symbols)
		if err == nil {
			for relPath, content := range files {
				fullPath := filepath.Join(workDir, relPath)
				_ = os.MkdirAll(filepath.Dir(fullPath), 0755)
				_ = os.WriteFile(fullPath, content, 0644)
			}
		}

		res := map[string]interface{}{
			"status":           "pulled",
			"universe_id":      args.UniverseID,
			"components_count": len(resp.Components),
			"symbols_count":    len(resp.Symbols),
			"blobs_count":      len(resp.Blobs),
			"savings_percent":  resp.SavingsPercent,
		}
		out, _ := json.Marshal(res)
		return string(out), nil
	}
	registry.Register(ToolDefCosmClone, cloneHandler)

	// 15. cosm_publish
	publishHandler := func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			HubURL      string `json:"hub_url"`
			Repo        string `json:"repo"`
			UniverseID  string `json:"universe_id"`
			Description string `json:"description"`
		}
		if err := call.ParseArguments(&args); err != nil {
			return "", err
		}
		if args.HubURL == "" {
			args.HubURL = os.Getenv("TOPOCOSM_HUB_URL")
		}
		if args.HubURL == "" {
			args.HubURL = "http://127.0.0.1:8080"
		}
		if args.UniverseID == "" {
			args.UniverseID = "universe-main"
		}
		parts := strings.Split(args.Repo, "/")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid repo format %q, expected org/repo", args.Repo)
		}
		orgSlug, cosmName := parts[0], parts[1]

		agentDID := os.Getenv("COSM_AGENT_DID")
		if agentDID == "" {
			agentDID = defaultAgentID
		}
		client := topocosm.NewHubClient(args.HubURL, agentDID)
		if tid := os.Getenv("COSM_TRACE_ID"); tid != "" {
			client.SetTraceID(tid)
		}
		if sid := os.Getenv("COSM_SESSION_ID"); sid != "" {
			client.SetSessionID(sid)
		}

		blobStore, graphEngine, err := openWorkspaceStorage(workDir)
		if err != nil {
			return "", err
		}
		defer graphEngine.Close()

		mgr := storage.NewUniverseManager(graphEngine, blobStore)
		manifest, err := mgr.GetUniverseManifest(args.UniverseID)
		if err != nil || manifest == nil {
			return "", fmt.Errorf("failed to get universe manifest: %w", err)
		}

		compMap := make(map[string]*core.ComponentNode)
		symMap := make(map[string]*core.ASTSymbolNode)
		blobs := make(map[string][]byte)

		for _, compID := range manifest.Components {
			var compNode core.ComponentNode
			if data, gErr := blobStore.Get(compID); gErr == nil {
				_ = json.Unmarshal(data, &compNode)
			} else if nodeRec, nErr := graphEngine.GetNode(compID); nErr == nil && nodeRec != nil {
				if d, bErr := blobStore.Get(nodeRec.MerkleHash); bErr == nil {
					_ = json.Unmarshal(d, &compNode)
				}
			}
			if compNode.ComponentID != "" {
				compMap[compNode.ComponentID] = &compNode
				for _, symID := range compNode.SymbolNodes {
					var symNode core.ASTSymbolNode
					if sData, sErr := blobStore.Get(symID); sErr == nil {
						_ = json.Unmarshal(sData, &symNode)
					} else if sRec, snErr := graphEngine.GetNode(symID); snErr == nil && sRec != nil {
						if sd, sbErr := blobStore.Get(sRec.MerkleHash); sbErr == nil {
							_ = json.Unmarshal(sd, &symNode)
						}
					}
					if symNode.NodeID != "" {
						symMap[symNode.NodeID] = &symNode
						if len(symNode.ASTPayload) > 0 {
							payloadHash := core.HashBytes(symNode.ASTPayload)
							blobs[payloadHash] = symNode.ASTPayload
						}
					}
				}
			}
		}

		payload := &topocosm.PublishPayload{
			OrgSlug:     orgSlug,
			CosmName:    cosmName,
			UniverseID:  args.UniverseID,
			Manifest:    manifest,
			Components:  compMap,
			Symbols:     symMap,
			Blobs:       blobs,
			Lineage:     manifest.Lineage,
			Description: args.Description,
			Visibility:  topocosm.VisibilityPublic,
		}

		resp, err := client.PublishCosm(ctx, payload)
		if err != nil {
			return "", fmt.Errorf("publish failed: %w", err)
		}

		out, _ := json.Marshal(resp)
		return string(out), nil
	}
	registry.Register(ToolDefCosmPublish, publishHandler)

	// 16. cosm_stack_create
	stackCreateHandler := func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			HubURL           string `json:"hub_url"`
			Repo             string `json:"repo"`
			Title            string `json:"title"`
			SourceUniverse   string `json:"source_universe"`
			TargetUniverse   string `json:"target_universe"`
			ParentProposalID string `json:"parent_proposal_id"`
		}
		if err := call.ParseArguments(&args); err != nil {
			return "", err
		}
		if args.HubURL == "" {
			args.HubURL = os.Getenv("TOPOCOSM_HUB_URL")
		}
		if args.HubURL == "" {
			args.HubURL = "http://127.0.0.1:8080"
		}
		if args.TargetUniverse == "" {
			args.TargetUniverse = "universe-main"
		}
		parts := strings.Split(args.Repo, "/")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid repo format %q, expected org/repo", args.Repo)
		}
		orgSlug, cosmName := parts[0], parts[1]

		agentDID := os.Getenv("COSM_AGENT_DID")
		if agentDID == "" {
			agentDID = defaultAgentID
		}
		client := topocosm.NewHubClient(args.HubURL, agentDID)
		if tid := os.Getenv("COSM_TRACE_ID"); tid != "" {
			client.SetTraceID(tid)
		}
		if sid := os.Getenv("COSM_SESSION_ID"); sid != "" {
			client.SetSessionID(sid)
		}

		lineageEnv := core.LineageEnvelope{
			ExecutingAgentID: defaultAgentID,
			Intent:           args.Title,
			Timestamp:        time.Now().UTC(),
		}

		prop, err := client.CreateProposal(ctx, orgSlug, cosmName, args.Title, args.SourceUniverse, args.TargetUniverse, lineageEnv)
		if err != nil {
			return "", fmt.Errorf("create proposal failed: %w", err)
		}

		res := map[string]interface{}{
			"status":             "created",
			"proposal_id":        prop.ID,
			"title":              prop.Title,
			"target_universe":    prop.TargetUniverse,
			"parent_proposal_id": args.ParentProposalID,
		}
		out, _ := json.Marshal(res)
		return string(out), nil
	}
	registry.Register(ToolDefCosmStackCreate, stackCreateHandler)
}

// RegisterAllFGTools is a backwards-compatible alias for RegisterAllCosmTools.
func RegisterAllFGTools(registry *framework.ToolRegistry, workDir string, defaultAgentID string) {
	RegisterAllCosmTools(registry, workDir, defaultAgentID)
}

// ExecuteCLICommand runs an external CLI command within the workspace.
func ExecuteCLICommand(ctx context.Context, workDir string, cmdLine string) (string, error) {
	parts := strings.Fields(cmdLine)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
