package topocosm

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/distributed"
	"github.com/cosmscm/cosm/pkg/topocosm/backplane"
)

// HubServer represents the Topocosm HTTP/REST and gRPC-compatible API server.
type HubServer struct {
	bp          backplane.BackplaneProvider
	repos       map[string]*CosmRepository                     // "org/cosm" -> CosmRepository
	users       map[string]*UserAccount                        // UserDID -> UserAccount
	orgs        map[string]*Organization                       // OrgSlug -> Organization
	agents      map[string]*AgentIdentity                      // AgentDID -> AgentIdentity
	proposals   map[string]map[string]*distributed.ProposalCOB // "org/cosm" -> (PropID -> ProposalCOB)
	blackboards map[string]*distributed.BlackboardCOB          // "org/cosm" -> BlackboardCOB
	startTime   time.Time
	mu          sync.RWMutex
}

// NewHubServer creates and initializes a new HubServer using the provided BackplaneProvider.
func NewHubServer(bp backplane.BackplaneProvider) *HubServer {
	s := &HubServer{
		bp:          bp,
		repos:       make(map[string]*CosmRepository),
		users:       make(map[string]*UserAccount),
		orgs:        make(map[string]*Organization),
		agents:      make(map[string]*AgentIdentity),
		proposals:   make(map[string]map[string]*distributed.ProposalCOB),
		blackboards: make(map[string]*distributed.BlackboardCOB),
		startTime:   time.Now().UTC(),
	}

	// Initialize default system organizations and critic agent
	s.orgs["cosm"] = &Organization{
		OrgID:         "org-cosm-core",
		Slug:          "cosm",
		DisplayName:   "Cosm Core Organization",
		PlanType:      "enterprise",
		Members:       make(map[string]Role),
		ComputeBudget: 100000,
		CreatedAt:     time.Now().UTC(),
	}

	s.agents["did:key:z6MkuCriticOracleGeminiFlash"] = &AgentIdentity{
		AgentDID:            "did:key:z6MkuCriticOracleGeminiFlash",
		AgentName:           "Gemini 3.7 Flash Critic Oracle",
		ModelVersion:        "gemini-3.7-flash",
		CapabilityToken:     "cap-oracle-system",
		MaxConcurrentClaims: 100,
		LeaseTimeoutSec:     600,
		CreatedAt:           time.Now().UTC(),
	}

	return s
}

// Handler returns the HTTP router for the Topocosm Hub server.
func (s *HubServer) Handler() http.Handler {
	mux := http.NewServeMux()

	// 1. Agent Discovery Protocol & Health
	mux.HandleFunc("/.well-known/cosm-agent.json", s.handleAgentDiscovery)
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.HandleFunc("/api/v1/stats", s.handleStats)

	// 2. Authentication & Settings
	mux.HandleFunc("/api/v1/auth/enroll", s.handleEnroll)
	mux.HandleFunc("/api/v1/auth/whoami", s.handleWhoAmI)
	mux.HandleFunc("/api/v1/settings/user", s.handleUserSettings)
	mux.HandleFunc("/api/v1/settings/orgs/", s.handleOrgSettings)
	mux.HandleFunc("/api/v1/settings/agents", s.handleAgentSettings)

	// 3. Cosm Repositories & Publishing
	mux.HandleFunc("/api/v1/cosms", s.handleCosms)
	mux.HandleFunc("/api/v1/cosms/", s.handleCosmScoped)

	// 4. Git Compatibility HTTP Shim
	mux.HandleFunc("/git/", s.handleGitShim)

	return mux
}

// handleAgentDiscovery serves the /.well-known/cosm-agent.json endpoint.
func (s *HubServer) handleAgentDiscovery(w http.ResponseWriter, r *http.Request) {
	manifest := AgentDiscoveryManifest{
		CosmVersion:  "1.0.0",
		HubURL:       "https://topocosm.dev",
		GRPCEndpoint: "grpc.topocosm.dev:443",
		RESTEndpoint: "https://topocosm.dev/api/v1",
		MCPEndpoint:  "https://topocosm.dev/mcp/v1",
		Capabilities: map[string]bool{
			"sparse_sync":          true,
			"blackboard_crdt":      true,
			"stacked_proposals":    true,
			"ephemeral_sandboxes":  true,
			"in_toto_attestations": true,
		},
		SupportedLanguages: []string{"go", "typescript", "python", "hcl", "rust", "java", "sql", "protobuf", "cpp"},
		AttestationPolicy: AttestationPolicy{
			RequirePromptProvenance:     true,
			RequireModelSignatures:      true,
			RequireCriticScoreThreshold: true,
			MinCriticScore:              80.0,
		},
		Timestamp: time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(manifest)
}

// handleHealth checks system status.
func (s *HubServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","hub":"topocosm.dev","uptime_sec":%d,"timestamp":"%s"}`,
		int64(time.Since(s.startTime).Seconds()), time.Now().UTC().Format(time.RFC3339))
}

// handleStats returns platform statistics.
func (s *HubServer) handleStats(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalProps := 0
	for _, m := range s.proposals {
		totalProps += len(m)
	}

	blobsList, _ := s.bp.BlobStore().List()
	activeLeases, _ := s.bp.LeaseManager().GetActiveLeases(r.Context())

	stats := HubStats{
		TotalCosms:       len(s.repos),
		TotalBlobs:       len(blobsList),
		TotalProposals:   totalProps,
		ActiveClaims:     len(activeLeases),
		RegisteredAgents: len(s.agents),
		UptimeSec:        int64(time.Since(s.startTime).Seconds()),
		Timestamp:        time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}

// handleEnroll handles user/agent enrollment with Ed25519 DID.
func (s *HubServer) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		DID          string `json:"did"`
		Username     string `json:"username"`
		Email        string `json:"email"`
		DisplayName  string `json:"display_name"`
		PublicKey    string `json:"public_key"`
		IsAgent      bool   `json:"is_agent"`
		ModelVersion string `json:"model_version,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	if req.DID == "" {
		http.Error(w, `{"error":"did is required"}`, http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if req.IsAgent {
		s.agents[req.DID] = &AgentIdentity{
			AgentDID:            req.DID,
			AgentName:           req.Username,
			ModelVersion:        req.ModelVersion,
			CapabilityToken:     fmt.Sprintf("cap-%s", req.DID[:16]),
			MaxConcurrentClaims: 10,
			LeaseTimeoutSec:     600,
			CreatedAt:           now,
		}
	} else {
		s.users[req.DID] = &UserAccount{
			UserDID:     req.DID,
			Username:    req.Username,
			Email:       req.Email,
			DisplayName: req.DisplayName,
			PublicKeys:  []string{req.PublicKey},
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		// Auto create personal organization namespace
		if req.Username != "" {
			s.orgs[req.Username] = &Organization{
				OrgID:         "org-" + req.Username,
				Slug:          req.Username,
				DisplayName:   req.DisplayName,
				PlanType:      "free",
				Members:       map[string]Role{req.DID: RoleOwner},
				ComputeBudget: 1000,
				CreatedAt:     now,
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"did":     req.DID,
		"message": "Successfully enrolled with Topocosm Hub",
	})
}

// handleWhoAmI returns caller identity.
func (s *HubServer) handleWhoAmI(w http.ResponseWriter, r *http.Request) {
	did := r.Header.Get("X-Cosm-DID")
	if did == "" {
		did = "did:key:anonymous"
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	user, isUser := s.users[did]
	agent, isAgent := s.agents[did]

	w.Header().Set("Content-Type", "application/json")
	if isUser {
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "user", "user": user})
	} else if isAgent {
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "agent", "agent": agent})
	} else {
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "anonymous", "did": did})
	}
}

// handleUserSettings manages user profile and signing keys.
func (s *HubServer) handleUserSettings(w http.ResponseWriter, r *http.Request) {
	did := r.Header.Get("X-Cosm-DID")
	s.mu.Lock()
	defer s.mu.Unlock()

	user, exists := s.users[did]
	if !exists {
		// Auto-provision demo user if absent
		user = &UserAccount{
			UserDID:     did,
			Username:    "cosm-dev",
			Email:       "dev@topocosm.dev",
			DisplayName: "Cosm Developer",
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}
		s.users[did] = user
	}

	if r.Method == http.MethodPut {
		var updates struct {
			DisplayName string   `json:"display_name"`
			Email       string   `json:"email"`
			PublicKeys  []string `json:"public_keys"`
			SSHKeys     []string `json:"ssh_keys"`
		}
		if err := json.NewDecoder(r.Body).Decode(&updates); err == nil {
			if updates.DisplayName != "" {
				user.DisplayName = updates.DisplayName
			}
			if updates.Email != "" {
				user.Email = updates.Email
			}
			if len(updates.PublicKeys) > 0 {
				user.PublicKeys = updates.PublicKeys
			}
			if len(updates.SSHKeys) > 0 {
				user.SSHKeys = updates.SSHKeys
			}
			user.UpdatedAt = time.Now().UTC()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

// handleOrgSettings manages organization policies and memberships.
func (s *HubServer) handleOrgSettings(w http.ResponseWriter, r *http.Request) {
	orgSlug := strings.TrimPrefix(r.URL.Path, "/api/v1/settings/orgs/")
	if orgSlug == "" {
		http.Error(w, `{"error":"org slug required"}`, http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	org, exists := s.orgs[orgSlug]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, fmt.Sprintf(`{"error":"organization %s not found"}`, orgSlug), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(org)
}

// handleAgentSettings lists or registers agent identities.
func (s *HubServer) handleAgentSettings(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*AgentIdentity, 0, len(s.agents))
	for _, a := range s.agents {
		list = append(list, a)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

// handleCosms lists or creates cosm repositories.
func (s *HubServer) handleCosms(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req struct {
			OrgSlug         string     `json:"org_slug"`
			CosmName        string     `json:"cosm_name"`
			Description     string     `json:"description"`
			Visibility      Visibility `json:"visibility"`
			DefaultUniverse string     `json:"default_universe"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		if req.OrgSlug == "" || req.CosmName == "" {
			http.Error(w, `{"error":"org_slug and cosm_name are required"}`, http.StatusBadRequest)
			return
		}

		s.mu.Lock()
		defer s.mu.Unlock()

		slug := req.OrgSlug + "/" + req.CosmName
		if _, exists := s.repos[slug]; exists {
			http.Error(w, fmt.Sprintf(`{"error":"cosm %s already exists"}`, slug), http.StatusConflict)
			return
		}

		if req.DefaultUniverse == "" {
			req.DefaultUniverse = "universe-main"
		}
		if req.Visibility == "" {
			req.Visibility = VisibilityPublic
		}

		repo := &CosmRepository{
			OrgSlug:         req.OrgSlug,
			CosmName:        req.CosmName,
			Description:     req.Description,
			Visibility:      req.Visibility,
			DefaultUniverse: req.DefaultUniverse,
			OwnerDID:        r.Header.Get("X-Cosm-DID"),
			CreatedAt:       time.Now().UTC(),
			UpdatedAt:       time.Now().UTC(),
		}
		s.repos[slug] = repo

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repo)
		return
	}

	// GET - List all cosms
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*CosmRepository, 0, len(s.repos))
	for _, repo := range s.repos {
		list = append(list, repo)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

// handleCosmScoped routes operations targeted at /api/v1/cosms/{org}/{cosm}/...
func (s *HubServer) handleCosmScoped(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/cosms/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.Error(w, `{"error":"invalid cosm path format (expected /api/v1/cosms/{org}/{cosm}/...)"}`, http.StatusBadRequest)
		return
	}

	orgSlug := parts[0]
	cosmName := parts[1]
	slug := orgSlug + "/" + cosmName

	action := ""
	if len(parts) > 2 {
		action = parts[2]
	}

	switch action {
	case "":
		// GET /api/v1/cosms/{org}/{cosm}
		s.mu.RLock()
		repo, exists := s.repos[slug]
		s.mu.RUnlock()

		if !exists {
			http.Error(w, fmt.Sprintf(`{"error":"cosm %s not found"}`, slug), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repo)

	case "publish":
		s.handlePublish(w, r, orgSlug, cosmName)

	case "pull":
		s.handlePull(w, r, orgSlug, cosmName)

	case "sparse-pull":
		s.handleSparsePull(w, r, orgSlug, cosmName)

	case "proposals":
		s.handleProposals(w, r, orgSlug, cosmName, parts[3:])

	case "blackboard":
		s.handleBlackboard(w, r, orgSlug, cosmName, parts[3:])

	default:
		http.Error(w, fmt.Sprintf(`{"error":"unknown action: %s"}`, action), http.StatusNotFound)
	}
}

// handlePublish processes incoming AST DAG uploads.
func (s *HubServer) handlePublish(w http.ResponseWriter, r *http.Request, orgSlug, cosmName string) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var payload PublishPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"payload decode failed: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	if payload.Manifest == nil {
		http.Error(w, `{"error":"workspace manifest is required"}`, http.StatusBadRequest)
		return
	}

	// 1. Store all received blobs in CAS
	storedBlobs := 0
	for hash, data := range payload.Blobs {
		storedHash, err := s.bp.BlobStore().Put(data)
		if err == nil && storedHash == hash {
			storedBlobs++
		}
	}

	// Also store serialized component nodes and symbol nodes if passed in map
	for _, comp := range payload.Components {
		compBytes, err := json.Marshal(comp)
		if err == nil {
			_, _ = s.bp.BlobStore().Put(compBytes)
			storedBlobs++
		}
	}

	for _, sym := range payload.Symbols {
		symBytes, err := json.Marshal(sym)
		if err == nil {
			_, _ = s.bp.BlobStore().Put(symBytes)
			storedBlobs++
		}
	}

	// 2. Commit Manifest to UniverseManager
	universeID := payload.UniverseID
	if universeID == "" {
		universeID = "universe-main"
	}

	merkleRoot, err := s.bp.UniverseManager().CommitManifest(universeID, payload.Manifest)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to commit manifest: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// 3. Register or update CosmRepository
	slug := orgSlug + "/" + cosmName
	s.mu.Lock()
	repo, exists := s.repos[slug]
	now := time.Now().UTC()
	if !exists {
		repo = &CosmRepository{
			OrgSlug:         orgSlug,
			CosmName:        cosmName,
			Description:     payload.Description,
			Visibility:      payload.Visibility,
			DefaultUniverse: universeID,
			OwnerDID:        r.Header.Get("X-Cosm-DID"),
			MerkleRootHash:  merkleRoot,
			Manifest:        payload.Manifest,
			Components:      payload.Components,
			Symbols:         payload.Symbols,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		s.repos[slug] = repo
	} else {
		if universeID == repo.DefaultUniverse || universeID == "universe-main" {
			repo.MerkleRootHash = merkleRoot
			repo.Manifest = payload.Manifest
		}
		if repo.Components == nil {
			repo.Components = make(map[string]*core.ComponentNode)
		}
		for k, v := range payload.Components {
			repo.Components[k] = v
		}
		if repo.Symbols == nil {
			repo.Symbols = make(map[string]*core.ASTSymbolNode)
		}
		for k, v := range payload.Symbols {
			repo.Symbols[k] = v
		}
		repo.UpdatedAt = now
	}
	s.mu.Unlock()

	// 4. Broadcast publish event
	_ = s.bp.EventStream().Publish(r.Context(), fmt.Sprintf("cosm.%s.published", slug), []byte(merkleRoot))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(PublishResponse{
		OrgSlug:        orgSlug,
		CosmName:       cosmName,
		UniverseID:     universeID,
		MerkleRootHash: merkleRoot,
		BlobsStored:    storedBlobs,
		Success:        true,
		Message:        "Cosm published successfully to Topocosm Hub",
		Timestamp:      now,
	})
}

// handlePull returns full manifest and all AST blobs.
func (s *HubServer) handlePull(w http.ResponseWriter, r *http.Request, orgSlug, cosmName string) {
	slug := orgSlug + "/" + cosmName
	s.mu.RLock()
	repo, exists := s.repos[slug]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, fmt.Sprintf(`{"error":"cosm %s not found"}`, slug), http.StatusNotFound)
		return
	}

	universeID := r.URL.Query().Get("universe")
	if universeID == "" {
		universeID = repo.DefaultUniverse
	}

	manifest := repo.Manifest
	if manifest == nil {
		var err error
		manifest, err = s.bp.UniverseManager().GetUniverseManifest(universeID)
		if err != nil || manifest == nil {
			http.Error(w, fmt.Sprintf(`{"error":"universe %s not found: %v"}`, universeID, err), http.StatusNotFound)
			return
		}
	}

	// Package full blobs
	components := make(map[string]*core.ComponentNode)
	symbols := make(map[string]*core.ASTSymbolNode)
	blobs := make(map[string][]byte)

	for _, compID := range manifest.Components {
		var comp *core.ComponentNode
		var compBytes []byte
		if repo.Components != nil {
			comp = repo.Components[compID]
		}
		if comp != nil {
			compBytes, _ = json.Marshal(comp)
		} else {
			compBytes, _ = s.bp.BlobStore().Get(compID)
			if compBytes != nil {
				var c core.ComponentNode
				if json.Unmarshal(compBytes, &c) == nil {
					comp = &c
				}
			}
		}

		if comp != nil {
			components[compID] = comp
			if len(compBytes) > 0 {
				blobs[compID] = compBytes
			}
			for _, symID := range comp.SymbolNodes {
				var sym *core.ASTSymbolNode
				var symBytes []byte
				if repo.Symbols != nil {
					sym = repo.Symbols[symID]
				}
				if sym != nil {
					symBytes, _ = json.Marshal(sym)
				} else {
					symBytes, _ = s.bp.BlobStore().Get(symID)
					if symBytes != nil {
						var s core.ASTSymbolNode
						if json.Unmarshal(symBytes, &s) == nil {
							sym = &s
						}
					}
				}
				if sym != nil {
					symbols[symID] = sym
					if len(symBytes) > 0 {
						blobs[symID] = symBytes
					}
				}
			}
		}
	}

	resp := SparsePullResponse{
		Manifest:        manifest,
		Components:      components,
		Symbols:         symbols,
		Blobs:           blobs,
		TotalBlobsCount: len(blobs),
		SavingsPercent:  0.0,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleSparsePull processes agent-filtered subtree extraction requests.
func (s *HubServer) handleSparsePull(w http.ResponseWriter, r *http.Request, orgSlug, cosmName string) {
	slug := orgSlug + "/" + cosmName
	s.mu.RLock()
	repo, exists := s.repos[slug]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, fmt.Sprintf(`{"error":"cosm %s not found"}`, slug), http.StatusNotFound)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req SparsePullRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	universeID := req.UniverseID
	if universeID == "" {
		universeID = repo.DefaultUniverse
	}

	manifest, err := s.bp.UniverseManager().GetUniverseManifest(universeID)
	if err != nil || manifest == nil {
		manifest = repo.Manifest
	}
	if manifest == nil {
		http.Error(w, fmt.Sprintf(`{"error":"universe %s manifest not found: %v"}`, universeID, err), http.StatusNotFound)
		return
	}

	components := make(map[string]*core.ComponentNode)
	symbols := make(map[string]*core.ASTSymbolNode)
	filteredBlobs := make(map[string][]byte)

	totalPossibleBlobs := len(manifest.Components)

	for _, compID := range manifest.Components {
		var comp *core.ComponentNode
		var compBytes []byte
		if repo.Components != nil {
			comp = repo.Components[compID]
		}
		if comp != nil {
			compBytes, _ = json.Marshal(comp)
		} else {
			compBytes, _ = s.bp.BlobStore().Get(compID)
			if compBytes != nil {
				var c core.ComponentNode
				if json.Unmarshal(compBytes, &c) == nil {
					comp = &c
				}
			}
		}

		if comp == nil {
			continue
		}

		totalPossibleBlobs += len(comp.SymbolNodes)

		// Filter matching
		matches := false
		if len(req.ComponentNames) == 0 && len(req.Languages) == 0 {
			matches = true
		} else {
			for _, name := range req.ComponentNames {
				if strings.Contains(comp.Name, name) || strings.Contains(name, comp.Name) {
					matches = true
					break
				}
				if comp.Metadata != nil {
					if fp, ok := comp.Metadata["file_path"]; ok && (strings.Contains(fp, name) || strings.Contains(name, fp)) {
						matches = true
						break
					}
				}
			}
			if !matches {
				for _, lang := range req.Languages {
					if comp.Language == lang {
						matches = true
						break
					}
				}
			}
		}

		if matches {
			components[compID] = comp
			if len(compBytes) > 0 {
				filteredBlobs[compID] = compBytes
			}

			for _, symID := range comp.SymbolNodes {
				var sym *core.ASTSymbolNode
				var symBytes []byte
				if repo.Symbols != nil {
					sym = repo.Symbols[symID]
				}
				if sym != nil {
					symBytes, _ = json.Marshal(sym)
				} else {
					symBytes, _ = s.bp.BlobStore().Get(symID)
					if symBytes != nil {
						var s core.ASTSymbolNode
						if json.Unmarshal(symBytes, &s) == nil {
							sym = &s
						}
					}
				}
				if sym != nil {
					symbols[symID] = sym
					if len(symBytes) > 0 {
						filteredBlobs[symID] = symBytes
					}
				}
			}
		}
	}

	savings := 0.0
	if totalPossibleBlobs > 0 && len(filteredBlobs) <= totalPossibleBlobs {
		savings = (1.0 - float64(len(filteredBlobs))/float64(totalPossibleBlobs)) * 100.0
	}

	resp := SparsePullResponse{
		Manifest:        manifest,
		Components:      components,
		Symbols:         symbols,
		Blobs:           filteredBlobs,
		TotalBlobsCount: len(filteredBlobs),
		SavingsPercent:  savings,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleProposals manages Universe Proposals (PR equivalent) and CRDT reviews.
func (s *HubServer) handleProposals(w http.ResponseWriter, r *http.Request, orgSlug, cosmName string, subPath []string) {
	slug := orgSlug + "/" + cosmName

	s.mu.Lock()
	if _, exists := s.proposals[slug]; !exists {
		s.proposals[slug] = make(map[string]*distributed.ProposalCOB)
	}
	props := s.proposals[slug]
	s.mu.Unlock()

	if len(subPath) == 0 {
		if r.Method == http.MethodPost {
			// Create Proposal
			var req struct {
				ID             string               `json:"id"`
				Title          string               `json:"title"`
				SourceUniverse string               `json:"source_universe"`
				TargetUniverse string               `json:"target_universe"`
				AuthorDID      string               `json:"author_did"`
				Lineage        core.LineageEnvelope `json:"lineage"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
				return
			}

			if req.ID == "" {
				req.ID = fmt.Sprintf("prop-%d", time.Now().UnixNano()%1000000)
			}
			if req.TargetUniverse == "" {
				req.TargetUniverse = "universe-main"
			}

			// Get head manifest hash of source universe
			sourceHead, _ := s.bp.UniverseManager().GetUniverse(req.SourceUniverse)
			manifestHash := ""
			if sourceHead != nil {
				manifestHash = sourceHead.HeadManifestHash
			}

			cob := distributed.NewProposalCOB(
				req.ID, req.Title, req.TargetUniverse, req.AuthorDID,
				req.SourceUniverse, manifestHash, req.Lineage,
			)

			// Auto evaluate with autonomous critic oracle
			evaluated, err := s.bp.CriticProvider().EvaluateProposal(r.Context(), cob, "AST Proposal Initial Submission")
			if err == nil && evaluated != nil {
				cob = evaluated
			}

			s.mu.Lock()
			props[req.ID] = cob
			s.mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(cob)
			return
		}

		// GET - List proposals
		s.mu.RLock()
		list := make([]*distributed.ProposalCOB, 0, len(props))
		for _, p := range props {
			list = append(list, p)
		}
		s.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
		return
	}

	propID := subPath[0]
	s.mu.RLock()
	cob, exists := props[propID]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, fmt.Sprintf(`{"error":"proposal %s not found"}`, propID), http.StatusNotFound)
		return
	}

	if len(subPath) == 1 {
		// GET /api/v1/cosms/{org}/{cosm}/proposals/{id}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cob)
		return
	}

	subAction := subPath[1]
	switch subAction {
	case "review":
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		var rev ReviewSubmission
		if err := json.NewDecoder(r.Body).Decode(&rev); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		for _, c := range rev.Comments {
			cob.AddComment(c)
		}
		if rev.ReviewerDID != "" {
			cob.SetApproval(rev.ReviewerDID, rev.Approved)
		}
		if rev.FitnessScore > 0 {
			cob.SetFitnessScore(rev.FitnessScore)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cob)

	case "merge":
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		if len(cob.Revisions) == 0 {
			http.Error(w, `{"error":"proposal has no revisions"}`, http.StatusBadRequest)
			return
		}

		latestRev := cob.Revisions[len(cob.Revisions)-1]
		mergedManifest, err := s.bp.UniverseManager().MergeUniverse(latestRev.SourceUniverse, cob.TargetUniverse, "union")
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"merge failed: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		cob.SetStatus(distributed.StatusMerged)
		s.mu.Lock()
		slug := orgSlug + "/" + cosmName
		if repo, exists := s.repos[slug]; exists && (cob.TargetUniverse == repo.DefaultUniverse || cob.TargetUniverse == "universe-main") {
			repo.MerkleRootHash = mergedManifest.MerkleRootHash
			repo.Manifest = mergedManifest
		}
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":          true,
			"status":           "MERGED",
			"target_universe":  cob.TargetUniverse,
			"merkle_root_hash": mergedManifest.MerkleRootHash,
		})

	default:
		http.Error(w, fmt.Sprintf(`{"error":"unknown proposal action: %s"}`, subAction), http.StatusNotFound)
	}
}

// handleBlackboard manages agent domain claim leases.
func (s *HubServer) handleBlackboard(w http.ResponseWriter, r *http.Request, orgSlug, cosmName string, subPath []string) {
	slug := orgSlug + "/" + cosmName

	s.mu.Lock()
	if _, exists := s.blackboards[slug]; !exists {
		s.blackboards[slug] = distributed.NewBlackboardCOB()
	}
	bb := s.blackboards[slug]
	s.mu.Unlock()

	if len(subPath) == 0 {
		// GET /api/v1/cosms/{org}/{cosm}/blackboard
		activeLeases, _ := s.bp.LeaseManager().GetActiveLeases(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"blackboard":    bb,
			"active_leases": activeLeases,
		})
		return
	}

	action := subPath[0]
	switch action {
	case "claim":
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		var req BlackboardClaimRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		ttl := time.Duration(req.LeaseTTLSec) * time.Second
		if ttl <= 0 {
			ttl = 10 * time.Minute
		}

		acquired, err := s.bp.LeaseManager().AcquireLease(r.Context(), req.Domain, req.AgentDID, req.Goal, ttl)
		if err != nil || !acquired {
			http.Error(w, fmt.Sprintf(`{"error":"domain %s already claimed by another agent"}`, req.Domain), http.StatusConflict)
			return
		}

		bb.ClaimDomain(req.Domain, req.AgentDID, req.Goal)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":   true,
			"domain":    req.Domain,
			"agent_did": req.AgentDID,
			"goal":      req.Goal,
			"lease_ttl": ttl.String(),
		})

	case "release":
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		var req BlackboardReleaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		_ = s.bp.LeaseManager().ReleaseLease(r.Context(), req.Domain, req.AgentDID)
		bb.ReleaseDomain(req.Domain, req.AgentDID)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"domain":  req.Domain,
			"message": "Domain lease successfully released",
		})

	default:
		http.Error(w, fmt.Sprintf(`{"error":"unknown blackboard action: %s"}`, action), http.StatusNotFound)
	}
}

// handleGitShim provides basic Git HTTP Smart protocol compatibility.
func (s *HubServer) handleGitShim(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/git/")
	if strings.HasSuffix(path, "/info/refs") {
		// Git Discovery
		repoPath := strings.TrimSuffix(path, "/info/refs")
		repoPath = strings.TrimSuffix(repoPath, ".git")

		s.mu.RLock()
		repo, exists := s.repos[repoPath]
		s.mu.RUnlock()

		w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
		w.Header().Set("Cache-Control", "no-cache")

		service := r.URL.Query().Get("service")
		if service == "" {
			service = "git-upload-pack"
		}

		headHash := "0000000000000000000000000000000000000000"
		if exists && repo.MerkleRootHash != "" {
			headHash = repo.MerkleRootHash
			if len(headHash) > 40 {
				headHash = headHash[:40]
			}
		}

		fmt.Fprintf(w, "# service=%s\n0000", service)
		fmt.Fprintf(w, "%04x%s HEAD\x00symref=HEAD:refs/heads/main\n", len(headHash)+10, headHash)
		fmt.Fprintf(w, "%04x%s refs/heads/main\n", len(headHash)+21, headHash)
		fmt.Fprintf(w, "0000")
		return
	}

	// For git-upload-pack / git-receive-pack, accept payload and return OK
	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "0000")
}

func shortenHex(h string) string {
	if len(h) <= 8 {
		return h
	}
	return h[:8]
}
