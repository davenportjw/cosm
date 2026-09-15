package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cosmscm/cosm/pkg/codecs"
	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/ignore"
	"github.com/cosmscm/cosm/pkg/lineage"
	"github.com/cosmscm/cosm/pkg/materialize"
	"github.com/cosmscm/cosm/pkg/mutation"
	"github.com/cosmscm/cosm/pkg/shipping"
	"github.com/cosmscm/cosm/pkg/storage"
	"github.com/cosmscm/cosm/pkg/target"
)

// JSON-RPC 2.0 types
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

const (
	errCodeParseError     = -32700
	errCodeInvalidRequest = -32600
	errCodeMethodNotFound = -32601
	errCodeInvalidParams  = -32602
	errCodeInternalError  = -32603
)

// ToolDefinition defines an MCP tool.
type ToolDefinition struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

// ToolCallResult represents the result payload for tools/call.
type ToolCallResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError"`
}

// ToolContent holds text content for a tool call result.
type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Server is a pure-Go Model Context Protocol (MCP) server.
type Server struct {
	workspaceDir string
	blobStore    *storage.BlobStore
	graphEngine  *storage.GraphEngine
	universeMgr  *storage.UniverseManager
	surgery      *mutation.SurgeryEngine
	in           io.Reader
	out          io.Writer
	mu           sync.Mutex
	tools        []ToolDefinition
}

// NewServer creates a new MCP Server by opening storage in workspaceDir.
func NewServer(workspaceDir string, in io.Reader, out io.Writer) (*Server, error) {
	cosmDir := filepath.Join(workspaceDir, ".cosm")
	blobStore, err := storage.NewBlobStore(filepath.Join(cosmDir, "objects"))
	if err != nil {
		return nil, fmt.Errorf("opening blob store: %w", err)
	}
	graphEngine, err := storage.NewGraphEngine(filepath.Join(cosmDir, "graph.db"))
	if err != nil {
		return nil, fmt.Errorf("opening graph engine: %w", err)
	}
	return NewServerWithStorage(workspaceDir, blobStore, graphEngine, in, out), nil
}

// NewServerWithStorage creates an MCP Server with injected storage instances.
func NewServerWithStorage(
	workspaceDir string,
	blobStore *storage.BlobStore,
	graphEngine *storage.GraphEngine,
	in io.Reader,
	out io.Writer,
) *Server {
	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	surgeryEngine := mutation.NewSurgeryEngine(blobStore, graphEngine, universeMgr)

	s := &Server{
		workspaceDir: workspaceDir,
		blobStore:    blobStore,
		graphEngine:  graphEngine,
		universeMgr:  universeMgr,
		surgery:      surgeryEngine,
		in:           in,
		out:          out,
	}
	s.initTools()
	return s
}

func (s *Server) initTools() {
	s.tools = []ToolDefinition{
		{
			Name:        "cosm_status",
			Description: "Report active universe, staged components, untracked files, and cross-boundary edges.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"universe": map[string]interface{}{
						"type":        "string",
						"description": "Micro-universe ID (default: universe-main)",
					},
				},
			},
		},
		{
			Name:        "cosm_ast_resolve",
			Description: "Scoped symbol resolution (e.g. 'path::Class.Method' or node ID).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"target": map[string]interface{}{
						"type":        "string",
						"description": "Target symbol identifier, path, or node ID (e.g. 'services/auth::ValidateToken')",
					},
					"universe": map[string]interface{}{
						"type":        "string",
						"description": "Micro-universe ID (default: universe-main)",
					},
				},
				"required": []string{"target"},
			},
		},
		{
			Name:        "cosm_ast_edit",
			Description: "Execute surgical AST mutations with the 9 Geometric verbs (replace_function_body, replace_function, add_method, add_before, add_after, delete, add_import, replace_imports, replace_global). Synchronizes disk files when write_disk is true.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"op": map[string]interface{}{
						"type":        "string",
						"description": "AST operation verb (replace_function_body, replace_function, add_method, add_before, add_after, delete, add_import, replace_imports, replace_global)",
					},
					"target": map[string]interface{}{
						"type":        "string",
						"description": "Target symbol path or ID",
					},
					"content": map[string]interface{}{
						"type":        "string",
						"description": "New code payload or content",
					},
					"universe": map[string]interface{}{
						"type":        "string",
						"description": "Micro-universe ID (default: universe-main)",
					},
					"write_disk": map[string]interface{}{
						"type":        "boolean",
						"description": "Automatically write modified components to workspace disk files (default: true)",
					},
					"prompt": map[string]interface{}{
						"type":        "string",
						"description": "Originating user prompt or task requirement",
					},
					"agent_id": map[string]interface{}{
						"type":        "string",
						"description": "Authoring AI agent identifier",
					},
				},
				"required": []string{"op", "target"},
			},
		},
		{
			Name:        "cosm_blast_radius",
			Description: "Query downstream contract dependencies, affected components, and risk score.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"target": map[string]interface{}{
						"type":        "string",
						"description": "Node ID, symbol identifier, or agent ID to audit",
					},
					"max_depth": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum traversal depth in the semantic dependency graph (default: 5)",
					},
				},
				"required": []string{"target"},
			},
		},
		{
			Name:        "cosm_topology",
			Description: "Output multi-tier dependency topology (Frontend -> Backend -> Infra).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"universe": map[string]interface{}{
						"type":        "string",
						"description": "Micro-universe ID (default: universe-main)",
					},
					"format": map[string]interface{}{
						"type":        "string",
						"description": "Output format: 'ascii', 'mermaid', or 'json' (default: 'ascii')",
					},
				},
			},
		},
		{
			Name:        "cosm_universe_create",
			Description: "Create a zero-copy micro-universe branch.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"universe_id": map[string]interface{}{
						"type":        "string",
						"description": "New micro-universe ID to create",
					},
					"parent_universe_id": map[string]interface{}{
						"type":        "string",
						"description": "Parent micro-universe ID (optional, defaults to universe-main)",
					},
				},
				"required": []string{"universe_id"},
			},
		},
		{
			Name:        "cosm_commit",
			Description: "Commit staged AST symbols with causal lineage (prompt, intent, agent_id, tokens).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"universe": map[string]interface{}{
						"type":        "string",
						"description": "Micro-universe ID (default: universe-main)",
					},
					"intent": map[string]interface{}{
						"type":        "string",
						"description": "Commit message or intent description",
					},
					"prompt": map[string]interface{}{
						"type":        "string",
						"description": "Originating user prompt",
					},
					"agent_id": map[string]interface{}{
						"type":        "string",
						"description": "Author or agent identifier",
					},
					"prompt_tokens": map[string]interface{}{
						"type":        "integer",
						"description": "LLM prompt tokens consumed",
					},
					"completion_tokens": map[string]interface{}{
						"type":        "integer",
						"description": "LLM completion tokens consumed",
					},
					"reasoning_tokens": map[string]interface{}{
						"type":        "integer",
						"description": "LLM reasoning tokens consumed",
					},
					"cost_usd": map[string]interface{}{
						"type":        "number",
						"description": "Total inference cost in USD",
					},
				},
				"required": []string{"intent"},
			},
		},
		{
			Name:        "cosm_ship",
			Description: "Run isolated preview compilation and staging build.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"universe": map[string]interface{}{
						"type":        "string",
						"description": "Micro-universe ID (default: universe-main)",
					},
					"target": map[string]interface{}{
						"type":        "string",
						"description": "Target profile (e.g. 'cosm', 'local-preview', 'cloud-run')",
					},
				},
			},
		},
	}
}

// Run listens for incoming JSON-RPC messages and dispatches them until EOF or ctx cancel.
func (s *Server) Run(ctx context.Context) error {
	reader := bufio.NewReader(s.in)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		payload, err := s.readRawMessage(reader)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		if len(bytes.TrimSpace(payload)) == 0 {
			continue
		}

		resp := s.handleMessage(payload)
		if resp != nil {
			if err := s.writeResponse(resp); err != nil {
				return err
			}
		}
	}
}

func (s *Server) readRawMessage(r *bufio.Reader) ([]byte, error) {
	for {
		peek, err := r.Peek(1)
		if err != nil {
			return nil, err
		}
		if peek[0] == '\r' || peek[0] == '\n' || peek[0] == ' ' || peek[0] == '\t' {
			_, _ = r.ReadByte()
			continue
		}
		break
	}

	peekHeader, _ := r.Peek(14)
	headerStr := strings.ToLower(string(peekHeader))
	if strings.HasPrefix(headerStr, "content-length") {
		contentLength := 0
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return nil, err
			}
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				break
			}
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 && strings.EqualFold(strings.TrimSpace(parts[0]), "content-length") {
				cl, pErr := strconv.Atoi(strings.TrimSpace(parts[1]))
				if pErr == nil {
					contentLength = cl
				}
			}
		}
		if contentLength > 0 {
			body := make([]byte, contentLength)
			if _, err := io.ReadFull(r, body); err != nil {
				return nil, err
			}
			return body, nil
		}
	}

	line, err := r.ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return nil, err
	}
	return bytes.TrimSpace(line), nil
}

func (s *Server) writeResponse(resp *jsonRPCResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = s.out.Write(data)
	return err
}

func (s *Server) handleMessage(raw []byte) *jsonRPCResponse {
	var req jsonRPCRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      nil,
			Error: &jsonRPCError{
				Code:    errCodeParseError,
				Message: fmt.Sprintf("Parse error: %v", err),
			},
		}
	}

	switch req.Method {
	case "initialize":
		return s.handleInitialize(&req)
	case "notifications/initialized":
		return nil
	case "ping":
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]interface{}{},
		}
	case "tools/list":
		return s.handleToolsList(&req)
	case "tools/call":
		return s.handleToolsCall(&req)
	default:
		if req.ID != nil {
			return &jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &jsonRPCError{
					Code:    errCodeMethodNotFound,
					Message: fmt.Sprintf("Method not found: %s", req.Method),
				},
			}
		}
		return nil
	}
}

func (s *Server) handleInitialize(req *jsonRPCRequest) *jsonRPCResponse {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "cosm-mcp",
			"version": "1.0.0",
		},
	}
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func (s *Server) handleToolsList(req *jsonRPCRequest) *jsonRPCResponse {
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"tools": s.tools,
		},
	}
}

type toolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

func (s *Server) handleToolsCall(req *jsonRPCRequest) *jsonRPCResponse {
	var params toolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &jsonRPCError{
				Code:    errCodeInvalidParams,
				Message: fmt.Sprintf("Invalid params: %v", err),
			},
		}
	}

	var res *ToolCallResult
	var err error

	switch params.Name {
	case "cosm_status":
		res, err = s.callCosmStatus(params.Arguments)
	case "cosm_ast_resolve":
		res, err = s.callCosmASTResolve(params.Arguments)
	case "cosm_ast_edit":
		res, err = s.callCosmASTEdit(params.Arguments)
	case "cosm_blast_radius":
		res, err = s.callCosmBlastRadius(params.Arguments)
	case "cosm_topology":
		res, err = s.callCosmTopology(params.Arguments)
	case "cosm_universe_create":
		res, err = s.callCosmUniverseCreate(params.Arguments)
	case "cosm_commit":
		res, err = s.callCosmCommit(params.Arguments)
	case "cosm_ship":
		res, err = s.callCosmShip(params.Arguments)
	default:
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &jsonRPCError{
				Code:    errCodeMethodNotFound,
				Message: fmt.Sprintf("Unknown tool: %s", params.Name),
			},
		}
	}

	if err != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: &ToolCallResult{
				Content: []ToolContent{
					{
						Type: "text",
						Text: fmt.Sprintf("Error executing tool %s: %v", params.Name, err),
					},
				},
				IsError: true,
			},
		}
	}

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  res,
	}
}

// 1. cosm_status
func (s *Server) callCosmStatus(args map[string]interface{}) (*ToolCallResult, error) {
	universeID := "universe-main"
	if u, ok := args["universe"].(string); ok && u != "" {
		universeID = u
	}

	head, err := s.universeMgr.GetUniverseManifest(universeID)
	if err != nil {
		return nil, fmt.Errorf("getting universe manifest: %w", err)
	}

	type statusReport struct {
		UniverseID         string   `json:"universe_id"`
		MerkleRoot         string   `json:"merkle_root"`
		TrackedComponents  int      `json:"tracked_components"`
		ComponentNames     []string `json:"component_names,omitempty"`
		CrossBoundaryEdges int      `json:"cross_boundary_edges"`
		UntrackedFiles     []string `json:"untracked_files"`
	}

	rep := statusReport{
		UniverseID:     universeID,
		UntrackedFiles: []string{},
	}

	compMap := make(map[string]bool)
	if head != nil {
		rep.MerkleRoot = head.MerkleRootHash
		rep.TrackedComponents = len(head.Components)
		rep.CrossBoundaryEdges = len(head.CrossEdges)

		for _, cID := range head.Components {
			var compData []byte
			if node, gErr := s.graphEngine.GetNode(cID); gErr == nil && node != nil && node.MerkleHash != "" {
				compData, _ = s.blobStore.Get(node.MerkleHash)
			} else {
				compData, _ = s.blobStore.Get(cID)
			}
			if len(compData) > 0 {
				var comp core.ComponentNode
				if json.Unmarshal(compData, &comp) == nil {
					rep.ComponentNames = append(rep.ComponentNames, comp.Name)
					compMap[comp.Name] = true
				}
			}
		}
	}

	ignoreEngine, _ := ignore.LoadWorkspaceRules(s.workspaceDir)
	if ignoreEngine == nil {
		ignoreEngine = ignore.NewIgnoreEngine(s.workspaceDir)
	}

	// Scan workspace for untracked files
	_ = filepath.Walk(s.workspaceDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		rel, rErr := filepath.Rel(s.workspaceDir, path)
		if rErr != nil || rel == "." {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "dist" ||
				ignoreEngine.ShouldIgnorePath(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if ignoreEngine.ShouldIgnorePath(rel, false) {
			return nil
		}
		if codecs.SupportsExtension(filepath.Ext(rel)) || codecs.IsDockerfile(rel) {
			if !compMap[rel] {
				rep.UntrackedFiles = append(rep.UntrackedFiles, rel)
			}
		}
		return nil
	})

	data, _ := json.MarshalIndent(rep, "", "  ")
	return &ToolCallResult{
		Content: []ToolContent{
			{
				Type: "text",
				Text: string(data),
			},
		},
		IsError: false,
	}, nil
}

// 2. cosm_ast_resolve
func (s *Server) callCosmASTResolve(args map[string]interface{}) (*ToolCallResult, error) {
	target, _ := args["target"].(string)
	if target == "" {
		return nil, fmt.Errorf("target identifier is required")
	}
	universeID := "universe-main"
	if u, ok := args["universe"].(string); ok && u != "" {
		universeID = u
	}

	resolved, err := s.surgery.ResolveSymbol(universeID, target)
	if err != nil {
		return nil, fmt.Errorf("symbol resolution failed: %w", err)
	}

	data, err := json.MarshalIndent(resolved, "", "  ")
	if err != nil {
		return nil, err
	}

	return &ToolCallResult{
		Content: []ToolContent{
			{
				Type: "text",
				Text: string(data),
			},
		},
		IsError: false,
	}, nil
}

// 3. cosm_ast_edit
func (s *Server) callCosmASTEdit(args map[string]interface{}) (*ToolCallResult, error) {
	opStr, _ := args["op"].(string)
	target, _ := args["target"].(string)
	content, _ := args["content"].(string)
	universeID := "universe-main"
	if u, ok := args["universe"].(string); ok && u != "" {
		universeID = u
	}

	writeDisk := true
	if wd, ok := args["write_disk"].(bool); ok {
		writeDisk = wd
	}

	prompt := "Declarative AST edit via MCP"
	if p, ok := args["prompt"].(string); ok && p != "" {
		prompt = p
	}

	agentID := "cosm-mcp-agent"
	if a, ok := args["agent_id"].(string); ok && a != "" {
		agentID = a
	}

	op := mutation.ASTOperationType(opStr)
	if !op.IsValid() {
		return nil, fmt.Errorf("invalid AST operation: %s", opStr)
	}
	if target == "" {
		return nil, fmt.Errorf("target symbol is required")
	}

	batch := mutation.ASTEditBatch{
		UniverseID: universeID,
		Lineage: core.LineageEnvelope{
			UserID:           os.Getenv("USER"),
			UserPrompt:       prompt,
			ExecutingAgentID: agentID,
			Intent:           fmt.Sprintf("MCP Edit: %s on %s", op, target),
			Timestamp:        time.Now().UTC(),
		},
		Operations: []mutation.ASTOperation{
			{
				Operation: op,
				Target:    target,
				Content:   content,
			},
		},
	}

	res, err := s.surgery.ApplyASTEditBatch(&batch)
	if err != nil {
		return nil, fmt.Errorf("AST edit batch failed: %w", err)
	}

	diskSyncCount := 0
	if writeDisk {
		updatedHead, hErr := s.universeMgr.GetUniverseManifest(res.UniverseID)
		if hErr == nil && updatedHead != nil {
			compMap, symMap := s.loadManifest(updatedHead)
			hydrator := materialize.NewHydrator()
			allFiles, hydErr := hydrator.HydrateWorkspace(updatedHead, compMap, symMap)
			if hydErr == nil && len(allFiles) > 0 {
				changedFiles := make(map[string][]byte)
				for relPath, fContent := range allFiles {
					fullPath := filepath.Join(s.workspaceDir, relPath)
					existing, rErr := os.ReadFile(fullPath)
					if rErr != nil || string(existing) != string(fContent) {
						changedFiles[relPath] = fContent
					}
				}
				if len(changedFiles) > 0 {
					exporter := materialize.NewExporter()
					report, expErr := exporter.ExportToDisk(s.workspaceDir, changedFiles, true)
					if expErr == nil && report != nil {
						diskSyncCount = report.FilesWritten
					}
				}
			}
		}
	}

	output := map[string]interface{}{
		"universe_id":           res.UniverseID,
		"applied_operations":    res.AppliedOperations,
		"modified_symbols":      res.ModifiedSymbols,
		"added_symbols":         res.AddedSymbols,
		"deleted_symbols":       res.DeletedSymbols,
		"deduplicated_symbols":  res.DeduplicatedSymbols,
		"old_manifest_hash":     res.OldManifestHash,
		"new_manifest_hash":     res.NewManifestHash,
		"duration_ms":           res.Duration.Milliseconds(),
		"disk_files_sync_count": diskSyncCount,
	}

	data, _ := json.MarshalIndent(output, "", "  ")
	return &ToolCallResult{
		Content: []ToolContent{
			{
				Type: "text",
				Text: string(data),
			},
		},
		IsError: false,
	}, nil
}

// 4. cosm_blast_radius
func (s *Server) callCosmBlastRadius(args map[string]interface{}) (*ToolCallResult, error) {
	target, _ := args["target"].(string)
	if target == "" {
		return nil, fmt.Errorf("target is required")
	}

	maxDepth := 5
	if md, ok := args["max_depth"].(float64); ok && md > 0 {
		maxDepth = int(md)
	}

	var report *lineage.BlastRadiusReport
	var err error

	if s.graphEngine.HasNode(target) {
		trav := s.graphEngine.TraverseForward([]string{target}, maxDepth, nil)
		nodeRec, _ := s.graphEngine.GetNode(target)
		direct := []storage.NodeRecord{}
		if nodeRec != nil {
			direct = append(direct, *nodeRec)
		}
		downstream := []storage.NodeRecord{}
		for _, n := range trav.VisitedNodes {
			if n.NodeID != target {
				downstream = append(downstream, n)
			}
		}
		risk := float64(len(trav.VisitedNodes)) * 0.1
		if risk > 1.0 {
			risk = 1.0
		}
		report = &lineage.BlastRadiusReport{
			DirectlyTouchedNodes:    direct,
			DownstreamImpactedNodes: downstream,
			ImpactedEdges:           trav.VisitedEdges,
			TotalDirectNodes:        len(direct),
			TotalDownstreamNodes:    len(downstream),
			TotalImpactedNodes:      len(direct) + len(downstream),
			RiskScore:               risk,
			Summary:                 fmt.Sprintf("Blast radius for node %s: %d downstream nodes", target, len(downstream)),
		}
	} else {
		auditor := lineage.NewAuditEngine(s.graphEngine)
		report, err = auditor.ComputeBlastRadius(lineage.AuditFilter{ExecutingAgentID: target}, maxDepth)
		if err != nil {
			return nil, err
		}
	}

	data, _ := json.MarshalIndent(report, "", "  ")
	return &ToolCallResult{
		Content: []ToolContent{
			{
				Type: "text",
				Text: string(data),
			},
		},
		IsError: false,
	}, nil
}

// 5. cosm_topology
func (s *Server) callCosmTopology(args map[string]interface{}) (*ToolCallResult, error) {
	universeID := "universe-main"
	if u, ok := args["universe"].(string); ok && u != "" {
		universeID = u
	}
	format := "ascii"
	if f, ok := args["format"].(string); ok && f != "" {
		format = strings.ToLower(f)
	}

	head, err := s.universeMgr.GetUniverseManifest(universeID)
	if err != nil {
		return nil, fmt.Errorf("getting universe manifest: %w", err)
	}
	if head == nil {
		return &ToolCallResult{
			Content: []ToolContent{
				{
					Type: "text",
					Text: fmt.Sprintf("No manifest found for universe %s", universeID),
				},
			},
		}, nil
	}

	compMap, symMap := s.loadManifest(head)
	vis := target.NewTopologyVisualizer()
	topGraph := vis.BuildTopology(head, compMap, symMap)

	var text string
	switch format {
	case "mermaid":
		text = vis.RenderMermaid(topGraph)
	case "json":
		d, _ := json.MarshalIndent(topGraph, "", "  ")
		text = string(d)
	default:
		text = vis.RenderASCII(topGraph)
	}

	return &ToolCallResult{
		Content: []ToolContent{
			{
				Type: "text",
				Text: text,
			},
		},
		IsError: false,
	}, nil
}

// 6. cosm_universe_create
func (s *Server) callCosmUniverseCreate(args map[string]interface{}) (*ToolCallResult, error) {
	universeID, _ := args["universe_id"].(string)
	if universeID == "" {
		return nil, fmt.Errorf("universe_id is required")
	}
	parentID, _ := args["parent_universe_id"].(string)

	u, err := s.universeMgr.CreateUniverse(universeID, parentID)
	if err != nil {
		return nil, fmt.Errorf("creating universe: %w", err)
	}

	resp := map[string]interface{}{
		"universe_id":        u.UniverseID,
		"parent_universe_id": u.ParentUniverseID,
		"merkle_root":        u.HeadManifestHash,
		"created_at":         u.CreatedAt.Format(time.RFC3339),
		"status":             u.Status,
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	return &ToolCallResult{
		Content: []ToolContent{
			{
				Type: "text",
				Text: string(data),
			},
		},
		IsError: false,
	}, nil
}

// 7. cosm_commit
func (s *Server) callCosmCommit(args map[string]interface{}) (*ToolCallResult, error) {
	intent, _ := args["intent"].(string)
	if intent == "" {
		return nil, fmt.Errorf("intent is required")
	}
	universeID := "universe-main"
	if u, ok := args["universe"].(string); ok && u != "" {
		universeID = u
	}
	prompt, _ := args["prompt"].(string)
	agentID := "cosm-mcp-agent"
	if a, ok := args["agent_id"].(string); ok && a != "" {
		agentID = a
	}

	head, _ := s.universeMgr.GetUniverseManifest(universeID)
	var components []string
	var edges []core.CrossBoundaryEdge
	var wsID = "cosm-workspace"
	if head != nil {
		components = head.Components
		edges = head.CrossEdges
		wsID = head.WorkspaceID
	}

	lineageEnv := core.LineageEnvelope{
		UserID:           os.Getenv("USER"),
		UserPrompt:       prompt,
		ExecutingAgentID: agentID,
		Intent:           intent,
		Timestamp:        time.Now().UTC(),
	}

	if pt, ok := args["prompt_tokens"].(float64); ok {
		lineageEnv.Tokens.PromptTokens = int64(pt)
	}
	if ct, ok := args["completion_tokens"].(float64); ok {
		lineageEnv.Tokens.CompletionTokens = int64(ct)
	}
	if rt, ok := args["reasoning_tokens"].(float64); ok {
		lineageEnv.Tokens.ReasoningTokens = int64(rt)
	}
	if cost, ok := args["cost_usd"].(float64); ok {
		lineageEnv.Tokens.CostUSD = cost
	}
	lineageEnv.Tokens.TotalTokens = lineageEnv.Tokens.PromptTokens + lineageEnv.Tokens.CompletionTokens + lineageEnv.Tokens.ReasoningTokens

	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: wsID,
		UniverseID:  universeID,
		Components:  components,
		CrossEdges:  edges,
		Lineage:     lineageEnv,
		CreatedAt:   time.Now().UTC(),
	}

	manifestHash, err := s.universeMgr.CommitManifest(universeID, manifest)
	if err != nil {
		return nil, fmt.Errorf("committing manifest: %w", err)
	}

	rec, _ := s.universeMgr.GetUniverse(universeID)
	committedAt := time.Now().UTC().Format(time.RFC3339)
	if rec != nil {
		committedAt = rec.UpdatedAt.Format(time.RFC3339)
	}

	resp := map[string]interface{}{
		"universe_id":  universeID,
		"merkle_root":  manifestHash,
		"components":   len(components),
		"intent":       intent,
		"committed_at": committedAt,
	}

	resData, _ := json.MarshalIndent(resp, "", "  ")
	return &ToolCallResult{
		Content: []ToolContent{
			{
				Type: "text",
				Text: string(resData),
			},
		},
		IsError: false,
	}, nil
}

// 8. cosm_ship
func (s *Server) callCosmShip(args map[string]interface{}) (*ToolCallResult, error) {
	universeID := "universe-main"
	if u, ok := args["universe"].(string); ok && u != "" {
		universeID = u
	}
	targetName := "cosm"
	if t, ok := args["target"].(string); ok && t != "" {
		targetName = t
	}

	head, err := s.universeMgr.GetUniverseManifest(universeID)
	if err != nil {
		return nil, fmt.Errorf("getting universe manifest: %w", err)
	}
	if head == nil {
		return nil, fmt.Errorf("no manifest found for universe %s", universeID)
	}

	compMap, symMap := s.loadManifest(head)
	hydrator := materialize.NewHydrator()
	files, err := hydrator.HydrateWorkspace(head, compMap, symMap)
	if err != nil {
		return nil, fmt.Errorf("hydrating files: %w", err)
	}

	var targetSpec *shipping.TargetSpec
	switch targetName {
	case "cosm", "target:cosm":
		targetSpec = shipping.DefaultCosmTarget()
	case "cloud-run", "target:cloud-run":
		targetSpec = shipping.DefaultCloudRunTarget(targetName, nil)
	default:
		targetSpec = shipping.DefaultLocalServiceTarget(targetName, nil)
	}

	packager := shipping.NewPackager(nil)
	distDir := filepath.Join(s.workspaceDir, "dist")
	art, err := packager.BuildAndPackageTarget(targetSpec, files, distDir)
	if err != nil {
		return nil, fmt.Errorf("packaging target: %w", err)
	}

	resp := map[string]interface{}{
		"target_name":      art.TargetName,
		"artifact_id":      art.ArtifactID,
		"artifact_path":    art.ArtifactPath,
		"size_bytes":       art.SizeBytes,
		"sha256_checksum":  art.SHA256Checksum,
		"created_at":       art.CreatedAt.Format(time.RFC3339),
		"status":           "READY_FOR_PREVIEW",
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	return &ToolCallResult{
		Content: []ToolContent{
			{
				Type: "text",
				Text: string(data),
			},
		},
		IsError: false,
	}, nil
}

func (s *Server) loadManifest(manifest *core.WorkspaceManifestNode) (map[string]*core.ComponentNode, map[string]*core.ASTSymbolNode) {
	compMap := make(map[string]*core.ComponentNode)
	symMap := make(map[string]*core.ASTSymbolNode)

	if manifest == nil {
		return compMap, symMap
	}

	for _, cID := range manifest.Components {
		var compData []byte
		var err error
		if node, gErr := s.graphEngine.GetNode(cID); gErr == nil && node != nil && node.MerkleHash != "" {
			compData, err = s.blobStore.Get(node.MerkleHash)
		} else {
			compData, err = s.blobStore.Get(cID)
		}
		if err == nil {
			var comp core.ComponentNode
			if e := json.Unmarshal(compData, &comp); e == nil {
				compMap[comp.ComponentID] = &comp
				compMap[cID] = &comp
				for _, sID := range comp.SymbolNodes {
					var sData []byte
					var sErr error
					if sNode, sgErr := s.graphEngine.GetNode(sID); sgErr == nil && sNode != nil && sNode.MerkleHash != "" {
						sData, sErr = s.blobStore.Get(sNode.MerkleHash)
					} else {
						sData, sErr = s.blobStore.Get(sID)
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
