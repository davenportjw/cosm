package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/mutation"
	"github.com/cosmscm/cosm/pkg/storage"
)

// MutateNodeRequest payload for AI agent AST mutations.
type MutateNodeRequest struct {
	UniverseID  string              `json:"universe_id"`
	SymbolNode  *core.ASTSymbolNode `json:"symbol_node"`
	ComponentID string              `json:"component_id"`
}

// MutateNodeResponse returned after AST mutation.
type MutateNodeResponse struct {
	NodeID     string `json:"node_id"`
	UniverseID string `json:"universe_id"`
	Success    bool   `json:"success"`
	Message    string `json:"message"`
}

// QueryUniverseResponse holds current active universe status.
type QueryUniverseResponse struct {
	UniverseID     string   `json:"universe_id"`
	MerkleRootHash string   `json:"merkle_root_hash"`
	ComponentCount int      `json:"component_count"`
	Components     []string `json:"components"`
}

// AgentAPIServer provides high-speed HTTP / in-process REST API for AI agent swarms.
type AgentAPIServer struct {
	universeMgr   *storage.UniverseManager
	blobStore     *storage.BlobStore
	graphEngine   *storage.GraphEngine
	surgeryEngine *mutation.SurgeryEngine
	mu            sync.RWMutex
}

// NewAgentAPIServer creates a new AgentAPIServer instance.
func NewAgentAPIServer(u *storage.UniverseManager, b *storage.BlobStore, g *storage.GraphEngine) *AgentAPIServer {
	return &AgentAPIServer{
		universeMgr:   u,
		blobStore:     b,
		graphEngine:   g,
		surgeryEngine: mutation.NewSurgeryEngine(b, g, u),
	}
}

// Handler returns the HTTP mux for the agent API.
func (s *AgentAPIServer) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","time":"%s"}`, time.Now().UTC().Format(time.RFC3339))
	})

	mux.HandleFunc("/api/v1/universe/", func(w http.ResponseWriter, r *http.Request) {
		universeID := r.URL.Path[len("/api/v1/universe/"):]
		if universeID == "" {
			http.Error(w, `{"error":"universe_id required"}`, http.StatusBadRequest)
			return
		}

		manifest, err := s.universeMgr.GetUniverseManifest(universeID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(QueryUniverseResponse{
			UniverseID:     universeID,
			MerkleRootHash: manifest.MerkleRootHash,
			ComponentCount: len(manifest.Components),
			Components:     manifest.Components,
		})
	})

	mux.HandleFunc("/api/v1/mutate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var req MutateNodeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		if req.SymbolNode == nil {
			http.Error(w, `{"error":"symbol_node is required"}`, http.StatusBadRequest)
			return
		}

		payload, err := json.Marshal(req.SymbolNode)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"serialization failed: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		blobHash, err := s.blobStore.Put(payload)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"blobstore put failed: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		req.SymbolNode.NodeID = blobHash

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(MutateNodeResponse{
			NodeID:     blobHash,
			UniverseID: req.UniverseID,
			Success:    true,
			Message:    "AST node successfully persisted in object store",
		})
	})

	// POST /api/v1/ast/edit executes a batch of declarative AST mutations
	mux.HandleFunc("/api/v1/ast/edit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var batch mutation.ASTEditBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid request payload: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		res, err := s.surgeryEngine.ApplyASTEditBatch(&batch)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"ast edit batch execution failed: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	})

	// POST /api/v1/ast/resolve locates a symbol by scoped path or identifier
	mux.HandleFunc("/api/v1/ast/resolve", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			UniverseID string `json:"universe_id"`
			Target     string `json:"target"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid request: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		if req.UniverseID == "" {
			req.UniverseID = "universe-main"
		}

		resolved, err := s.surgeryEngine.ResolveSymbol(req.UniverseID, req.Target)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"symbol resolution failed: %s"}`, err.Error()), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resolved)
	})

	return mux
}
