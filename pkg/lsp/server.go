package lsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cosmscm/cosm/pkg/codecs"
	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

// LSP JSON-RPC message structures
type lspRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type lspResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *lspError       `json:"error,omitempty"`
}

type lspNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

type lspError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// LSP protocol primitives
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type Diagnostic struct {
	Range    Range       `json:"range"`
	Severity int         `json:"severity"` // 1: Error, 2: Warning, 3: Info, 4: Hint
	Code     interface{} `json:"code,omitempty"`
	Source   string      `json:"source,omitempty"`
	Message  string      `json:"message"`
}

type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

type Command struct {
	Title     string        `json:"title"`
	Command   string        `json:"command"`
	Arguments []interface{} `json:"arguments,omitempty"`
}

type CodeLens struct {
	Range   Range   `json:"range"`
	Command Command `json:"command"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type DidSaveTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Text         *string                `json:"text,omitempty"`
}

type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type CodeLensParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// Server implements a lightweight Language Server Protocol (LSP) AST Bridge over stdio.
type Server struct {
	workspaceDir string
	blobStore    *storage.BlobStore
	graphEngine  *storage.GraphEngine
	universeMgr  *storage.UniverseManager
	in           io.Reader
	out          io.Writer
	mu           sync.Mutex
	isShutdown   bool
}

// NewServer creates an LSP Server by opening storage in workspaceDir.
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

// NewServerWithStorage creates an LSP Server with injected storage instances.
func NewServerWithStorage(
	workspaceDir string,
	blobStore *storage.BlobStore,
	graphEngine *storage.GraphEngine,
	in io.Reader,
	out io.Writer,
) *Server {
	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	return &Server{
		workspaceDir: workspaceDir,
		blobStore:    blobStore,
		graphEngine:  graphEngine,
		universeMgr:  universeMgr,
		in:           in,
		out:          out,
	}
}

// Run listens for incoming LSP messages and dispatches them until EOF or ctx cancel.
func (s *Server) Run(ctx context.Context) error {
	reader := bufio.NewReader(s.in)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		payload, err := s.readMessage(reader)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		if len(bytes.TrimSpace(payload)) == 0 {
			continue
		}

		s.handleMessage(payload)
	}
}

func (s *Server) readMessage(r *bufio.Reader) ([]byte, error) {
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

	if contentLength == 0 {
		// Fallback for line-based clients
		line, err := r.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		return bytes.TrimSpace(line), nil
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

func (s *Server) writeMessage(msg interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := s.out.Write([]byte(header)); err != nil {
		return err
	}
	_, err = s.out.Write(data)
	return err
}

func (s *Server) handleMessage(raw []byte) {
	var req lspRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return
	}

	switch req.Method {
	case "initialize":
		s.handleInitialize(&req)
	case "initialized":
		// Notification, no response
	case "shutdown":
		s.isShutdown = true
		s.writeResponse(req.ID, nil)
	case "exit":
		// Client requested exit
	case "textDocument/didOpen":
		s.handleDidOpen(&req)
	case "textDocument/didSave":
		s.handleDidSave(&req)
	case "textDocument/definition":
		s.handleDefinition(&req)
	case "textDocument/codeLens":
		s.handleCodeLens(&req)
	default:
		if req.ID != nil {
			// Method not found or not supported
			s.writeError(req.ID, -32601, fmt.Sprintf("Method %s not supported", req.Method))
		}
	}
}

func (s *Server) writeResponse(id interface{}, result interface{}) {
	if id == nil {
		return
	}
	resp := lspResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	_ = s.writeMessage(resp)
}

func (s *Server) writeError(id interface{}, code int, message string) {
	if id == nil {
		return
	}
	resp := lspResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &lspError{
			Code:    code,
			Message: message,
		},
	}
	_ = s.writeMessage(resp)
}

func (s *Server) sendNotification(method string, params interface{}) {
	notif := lspNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	_ = s.writeMessage(notif)
}

func (s *Server) handleInitialize(req *lspRequest) {
	capabilities := map[string]interface{}{
		"textDocumentSync": 1, // 1: Full sync
		"definitionProvider": true,
		"codeLensProvider": map[string]interface{}{
			"resolveProvider": false,
		},
	}

	result := map[string]interface{}{
		"capabilities": capabilities,
		"serverInfo": map[string]interface{}{
			"name":    "cosm-lsp",
			"version": "1.0.0",
		},
	}

	s.writeResponse(req.ID, result)
}

func (s *Server) handleDidOpen(req *lspRequest) {
	var params DidOpenTextDocumentParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return
	}
	// Initial diagnostics verification on open
	s.validateDocument(params.TextDocument.URI, []byte(params.TextDocument.Text))
}

func (s *Server) handleDidSave(req *lspRequest) {
	var params DidSaveTextDocumentParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return
	}

	filePath := s.uriToPath(params.TextDocument.URI)
	var content []byte
	if params.Text != nil {
		content = []byte(*params.Text)
	} else {
		var err error
		content, err = os.ReadFile(filePath)
		if err != nil {
			return
		}
	}

	s.validateDocument(params.TextDocument.URI, content)
}

// validateDocument runs cross-boundary contract validation and publishes LSP diagnostics.
func (s *Server) validateDocument(uri string, content []byte) {
	relPath := s.uriToRelPath(uri)
	diagnostics := []Diagnostic{}

	head, err := s.universeMgr.GetUniverseManifest("universe-main")
	if err == nil && head != nil {
		compMap, symMap := s.loadManifest(head)

		// Identify all old symbols in this file
		fileOldSymIDs := make(map[string]bool)
		for _, comp := range compMap {
			compFile := comp.Metadata["file_path"]
			if compFile == "" {
				compFile = comp.Name
			}
			if compFile == relPath || strings.HasSuffix(relPath, compFile) {
				for _, sID := range comp.SymbolNodes {
					fileOldSymIDs[sID] = true
				}
			}
		}
		for sID, sym := range symMap {
			if sym.ASTMetadata["file_path"] == relPath || strings.HasSuffix(relPath, sym.ASTMetadata["file_path"]) {
				fileOldSymIDs[sID] = true
			}
		}

		// Parse the updated file to get new symbols
		lineageEnv := core.LineageEnvelope{
			ExecutingAgentID: "cosm-lsp-checker",
			Intent:           "Live LSP contract check",
			Timestamp:        time.Now().UTC(),
		}

		parsed, pErr := codecs.ParseSourceFile(relPath, content, lineageEnv)
		if pErr == nil && parsed != nil {
			newNodes := make(map[string]*core.ASTSymbolNode)
			for k, v := range symMap {
				if !fileOldSymIDs[k] {
					newNodes[k] = v
				}
			}
			for _, sNode := range parsed.Symbols {
				newNodes[sNode.NodeID] = sNode
			}

			// Filter out edges whose symbol in this file was removed
			var newEdges []core.CrossBoundaryEdge
			for _, edge := range head.CrossEdges {
				keep := true
				if fileOldSymIDs[edge.SourceNodeID] {
					srcOld := symMap[edge.SourceNodeID]
					found := false
					if srcOld != nil {
						for _, n := range parsed.Symbols {
							if n.Identifier == srcOld.Identifier && n.NodeType == srcOld.NodeType {
								found = true
								break
							}
						}
					}
					if !found {
						keep = false
					}
				}
				if fileOldSymIDs[edge.TargetNodeID] {
					tgtOld := symMap[edge.TargetNodeID]
					found := false
					if tgtOld != nil {
						for _, n := range parsed.Symbols {
							if n.Identifier == tgtOld.Identifier && n.NodeType == tgtOld.NodeType {
								found = true
								break
							}
						}
					}
					if !found {
						keep = false
					}
				}
				if keep {
					newEdges = append(newEdges, edge)
				}
			}

			// Check for contract breakages using DetectContractBreakages
			breakages := core.DetectContractBreakages(head.CrossEdges, newEdges, symMap, newNodes)
			for _, b := range breakages {
				diag := Diagnostic{
					Range: Range{
						Start: Position{Line: 0, Character: 0},
						End:   Position{Line: 0, Character: 80},
					},
					Severity: 1, // Error
					Source:   "cosm-contract",
					Message:  b.Description,
				}
				diagnostics = append(diagnostics, diag)
			}
		}
	}

	// Publish diagnostics
	s.sendNotification("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diagnostics,
	})
}

// handleDefinition performs cross-language jump (Frontend API string -> Backend handler -> Terraform resource).
func (s *Server) handleDefinition(req *lspRequest) {
	var params TextDocumentPositionParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeResponse(req.ID, nil)
		return
	}

	head, err := s.universeMgr.GetUniverseManifest("universe-main")
	if err != nil || head == nil {
		s.writeResponse(req.ID, nil)
		return
	}

	relPath := s.uriToRelPath(params.TextDocument.URI)
	compMap, symMap := s.loadManifest(head)

	// Find if any symbol in this file matches or if cross-boundary edges connect it
	var matchedTargetNodeID string
	for _, edge := range head.CrossEdges {
		srcSym := symMap[edge.SourceNodeID]
		if srcSym != nil {
			srcFile := srcSym.ASTMetadata["file_path"]
			if srcFile == relPath || strings.HasSuffix(relPath, srcSym.Identifier) {
				matchedTargetNodeID = edge.TargetNodeID
				break
			}
		}
		// Also check direct component association
		for _, comp := range compMap {
			if comp.Name == relPath || comp.Metadata["file_path"] == relPath {
				for _, sID := range comp.SymbolNodes {
					if sID == edge.SourceNodeID {
						matchedTargetNodeID = edge.TargetNodeID
						break
					}
				}
			}
		}
		if matchedTargetNodeID != "" {
			break
		}
	}

	if matchedTargetNodeID != "" {
		tgtSym := symMap[matchedTargetNodeID]
		var tgtRelPath string
		lineNum := 0
		if tgtSym != nil {
			tgtRelPath = tgtSym.ASTMetadata["file_path"]
			if sl, ok := tgtSym.ASTMetadata["start_line"]; ok {
				if l, err := strconv.Atoi(sl); err == nil && l > 0 {
					lineNum = l - 1
				}
			}
		}
		if tgtRelPath == "" {
			// Try finding component
			for _, comp := range compMap {
				for _, sID := range comp.SymbolNodes {
					if sID == matchedTargetNodeID {
						tgtRelPath = comp.Metadata["file_path"]
						if tgtRelPath == "" {
							tgtRelPath = comp.Name
						}
						break
					}
				}
			}
		}

		if tgtRelPath != "" {
			targetURI := s.pathToURI(filepath.Join(s.workspaceDir, tgtRelPath))
			loc := Location{
				URI: targetURI,
				Range: Range{
					Start: Position{Line: lineNum, Character: 0},
					End:   Position{Line: lineNum, Character: 40},
				},
			}
			s.writeResponse(req.ID, loc)
			return
		}
	}

	s.writeResponse(req.ID, nil)
}

// handleCodeLens returns causal lineage CodeLens above function/symbol declarations.
func (s *Server) handleCodeLens(req *lspRequest) {
	var params CodeLensParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeResponse(req.ID, []CodeLens{})
		return
	}

	relPath := s.uriToRelPath(params.TextDocument.URI)
	head, err := s.universeMgr.GetUniverseManifest("universe-main")
	if err != nil || head == nil {
		s.writeResponse(req.ID, []CodeLens{})
		return
	}

	compMap, symMap := s.loadManifest(head)
	var lenses []CodeLens

	for _, comp := range compMap {
		compFile := comp.Metadata["file_path"]
		if compFile == "" {
			compFile = comp.Name
		}
		if compFile != relPath && !strings.HasSuffix(relPath, compFile) {
			continue
		}

		for _, sID := range comp.SymbolNodes {
			sym := symMap[sID]
			if sym == nil {
				continue
			}

			startLine := 0
			if sl, ok := sym.ASTMetadata["start_line"]; ok {
				if l, err := strconv.Atoi(sl); err == nil && l > 0 {
					startLine = l - 1
				}
			}

			// Check lineage in GraphEngine or symbol lineage
			records := s.graphEngine.GetLineageByNode(sym.NodeID)
			var title string
			if len(records) > 0 {
				r := records[len(records)-1]
				agent := r.ExecutingAgentID
				if agent == "" {
					agent = "human"
				}
				intent := r.Intent
				if intent == "" {
					intent = "Causal modification"
				}
				title = fmt.Sprintf("⚡ Cosm Lineage: [%s] \"%s\"", agent, intent)
			} else if sym.Lineage.ExecutingAgentID != "" || sym.Lineage.Intent != "" {
				agent := sym.Lineage.ExecutingAgentID
				if agent == "" {
					agent = "human"
				}
				title = fmt.Sprintf("⚡ Cosm Lineage: [%s] \"%s\"", agent, sym.Lineage.Intent)
			} else {
				title = fmt.Sprintf("⚡ Cosm Symbol: %s (%s)", sym.Identifier, sym.NodeType)
			}

			lenses = append(lenses, CodeLens{
				Range: Range{
					Start: Position{Line: startLine, Character: 0},
					End:   Position{Line: startLine, Character: 0},
				},
				Command: Command{
					Title:     title,
					Command:   "cosm.showLineage",
					Arguments: []interface{}{sym.NodeID},
				},
			})
		}
	}

	s.writeResponse(req.ID, lenses)
}

func (s *Server) uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	if u.Scheme == "file" {
		return u.Path
	}
	return uri
}

func (s *Server) pathToURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return "file://" + abs
}

func (s *Server) uriToRelPath(uri string) string {
	absPath := s.uriToPath(uri)
	rel, err := filepath.Rel(s.workspaceDir, absPath)
	if err != nil {
		return filepath.Base(absPath)
	}
	return rel
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
