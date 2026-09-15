package onboarding

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cosmscm/cosm/pkg/codecs"
	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/ignore"
	"github.com/cosmscm/cosm/pkg/storage"
)

// OnboardReport contains summary metrics for a repository onboarding run.
type OnboardReport struct {
	TotalFilesScanned int           `json:"total_files_scanned"`
	CodeFilesParsed   int           `json:"code_files_parsed"`
	RawFilesPreserved int           `json:"raw_files_preserved"`
	SymbolsExtracted  int           `json:"symbols_extracted"`
	ComponentsCreated int           `json:"components_created"`
	EdgesDiscovered   int           `json:"edges_discovered"`
	ManifestHash      string        `json:"manifest_hash"`
	Duration          time.Duration `json:"duration"`
}

// RepositoryMigrator coordinates importing an existing repository codebase into .cosm/.
type RepositoryMigrator struct {
	blobStore   *storage.BlobStore
	graphEngine *storage.GraphEngine
	universeMgr *storage.UniverseManager
	scanner     *DirectoryScanner
	detector    *ignore.SecretDetector
}

// NewRepositoryMigrator initializes a new migrator instance.
func NewRepositoryMigrator(
	blobStore *storage.BlobStore,
	graphEngine *storage.GraphEngine,
	universeMgr *storage.UniverseManager,
) *RepositoryMigrator {
	return &RepositoryMigrator{
		blobStore:   blobStore,
		graphEngine: graphEngine,
		universeMgr: universeMgr,
		scanner:     NewDirectoryScanner(),
		detector:    ignore.NewSecretDetector(),
	}
}

// ImportRepositorySnapshot scans, parses, links, and persists an entire repo into a micro-universe.
func (m *RepositoryMigrator) ImportRepositorySnapshot(
	repoPath string,
	universeID string,
	userPrompt string,
	agentID string,
) (*OnboardReport, error) {
	start := time.Now()

	codeFiles, rawFiles, err := m.scanner.ScanDirectory(repoPath)
	if err != nil {
		return nil, fmt.Errorf("scanning directory: %w", err)
	}

	report := &OnboardReport{
		TotalFilesScanned: len(codeFiles) + len(rawFiles),
		CodeFilesParsed:   len(codeFiles),
		RawFilesPreserved: len(rawFiles),
	}

	now := time.Now().UTC()
	lineageEnv := core.LineageEnvelope{
		UserID:           "repo-onboarding-user",
		UserPrompt:       userPrompt,
		SessionID:        fmt.Sprintf("onboard-sess-%d", start.Unix()),
		ExecutingAgentID: agentID,
		LLMVersion:       "cosm-migrator-v1",
		Intent:           "Onboard existing repository",
		Timestamp:        now,
	}

	var allComponents []*core.ComponentNode
	allSymbols := make(map[string]*core.ASTSymbolNode)
	var compIDs []string
	var allEdges []core.CrossBoundaryEdge

	// 1. Process Code Files
	for _, file := range codeFiles {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		relPath, _ := filepath.Rel(repoPath, file)

		var comp *core.ComponentNode
		fileSyms := make(map[string]*core.ASTSymbolNode)

		parsed, pErr := codecs.ParseSourceFile(relPath, content, lineageEnv)
		if pErr == nil && parsed != nil && parsed.Component != nil {
			comp = parsed.Component
			fileSyms = parsed.Symbols
		} else {
			// Fallback to raw preservation so no scanned file is dropped
			rawSymID := fmt.Sprintf("raw:%s", sanitizeID(relPath))
			rawSym := &core.ASTSymbolNode{
				NodeID:            rawSymID,
				Language:          "raw",
				NodeType:          "RawBlobNode",
				Identifier:        relPath,
				ASTPayload:        content,
				LocalDependencies: []string{},
				Lineage:           lineageEnv,
			}
			fileSyms[rawSymID] = rawSym
			comp = &core.ComponentNode{
				ComponentID: fmt.Sprintf("comp-raw-%s", sanitizeID(relPath)),
				Name:        relPath,
				Type:        core.CompService,
				Language:    "raw",
				SymbolNodes: []string{rawSymID},
				Metadata:    map[string]string{"file_path": relPath},
				Lineage:     lineageEnv,
			}
		}

		if comp == nil {
			continue
		}

		allComponents = append(allComponents, comp)
		compIDs = append(compIDs, comp.ComponentID)

		// Persist Symbols
		for symID, sym := range fileSyms {
			allSymbols[symID] = sym
			data, _ := json.Marshal(sym)
			symHash, _ := m.blobStore.Put(data)
			_ = m.graphEngine.PutNode(storage.NodeRecord{
				NodeID:     symID,
				Language:   sym.Language,
				NodeType:   sym.NodeType,
				MerkleHash: symHash,
				CreatedAt:  now,
			})
			_ = m.graphEngine.PutLineage(storage.LineageRecord{
				RecordID:         fmt.Sprintf("lin-%s", symID),
				NodeID:           symID,
				UserID:           lineageEnv.UserID,
				UserPrompt:       lineageEnv.UserPrompt,
				ExecutingAgentID: lineageEnv.ExecutingAgentID,
				Intent:           lineageEnv.Intent,
				Timestamp:        lineageEnv.Timestamp,
			})
		}

		// Persist Component
		compData, _ := json.Marshal(comp)
		compHash, _ := m.blobStore.Put(compData)
		_ = m.graphEngine.PutNode(storage.NodeRecord{
			NodeID:     comp.ComponentID,
			Language:   comp.Language,
			NodeType:   string(comp.Type),
			MerkleHash: compHash,
			CreatedAt:  now,
		})
	}

	// 2. Process Raw Non-AST Files (configs, markdown, binaries) for zero-loss preservation
	for _, file := range rawFiles {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		relPath, _ := filepath.Rel(repoPath, file)
		if m.detector != nil && len(m.detector.DetectSecrets(relPath, content)) > 0 {
			// Skip raw files containing detected plaintext secrets
			continue
		}
		rawSymID := fmt.Sprintf("raw:%s", sanitizeID(relPath))

		rawSym := &core.ASTSymbolNode{
			NodeID:            rawSymID,
			Language:          "raw",
			NodeType:          "RawBlobNode",
			Identifier:        relPath,
			ASTPayload:        content,
			LocalDependencies: []string{},
			Lineage:           lineageEnv,
		}

		allSymbols[rawSymID] = rawSym
		data, _ := json.Marshal(rawSym)
		symHash, _ := m.blobStore.Put(data)

		_ = m.graphEngine.PutNode(storage.NodeRecord{
			NodeID:     rawSymID,
			Language:   "raw",
			NodeType:   "RawBlobNode",
			MerkleHash: symHash,
			CreatedAt:  now,
		})

		rawCompID := fmt.Sprintf("comp-raw-%s", sanitizeID(relPath))
		rawComp := &core.ComponentNode{
			ComponentID: rawCompID,
			Name:        relPath,
			Type:        core.CompService,
			Language:    "raw",
			SymbolNodes: []string{rawSymID},
			Metadata:    map[string]string{"file_path": relPath},
			Lineage:     lineageEnv,
		}

		allComponents = append(allComponents, rawComp)
		compIDs = append(compIDs, rawCompID)

		compData, _ := json.Marshal(rawComp)
		compHash, _ := m.blobStore.Put(compData)
		_ = m.graphEngine.PutNode(storage.NodeRecord{
			NodeID:     rawCompID,
			Language:   "raw",
			NodeType:   string(core.CompService),
			MerkleHash: compHash,
			CreatedAt:  now,
		})
	}

	report.SymbolsExtracted = len(allSymbols)
	report.ComponentsCreated = len(allComponents)

	// 3. Discover Cross-Boundary Contract Edges
	engine, _ := ignore.LoadWorkspaceRules(repoPath)
	linker := core.NewCrossBoundaryLinker()
	if engine != nil && engine.EdgeFilter != nil {
		linker.SetFilter(engine.EdgeFilter)
	}
	discoveredEdges, err := linker.LinkWorkspace(allComponents, allSymbols)
	if err == nil {
		report.EdgesDiscovered = len(discoveredEdges)
		for _, edge := range discoveredEdges {
			allEdges = append(allEdges, edge)
			_ = m.graphEngine.PutEdge(storage.EdgeRecord{
				SourceID:   edge.SourceNodeID,
				TargetID:   edge.TargetNodeID,
				EdgeType:   edge.Type,
				ContractID: edge.ContractSchemaID,
			})
		}
	}

	// 4. Construct & Commit Workspace Manifest
	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-root",
		UniverseID:  universeID,
		Components:  compIDs,
		CrossEdges:  allEdges,
		Lineage:     lineageEnv,
		CreatedAt:   now,
	}

	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshaling manifest: %w", err)
	}

	manifestHash, err := m.blobStore.Put(manifestData)
	if err != nil {
		return nil, fmt.Errorf("storing manifest blob: %w", err)
	}
	manifest.MerkleRootHash = manifestHash

	_, err = m.universeMgr.CommitManifest(universeID, manifest)
	if err != nil {
		return nil, fmt.Errorf("committing universe manifest: %w", err)
	}

	report.ManifestHash = manifestHash
	report.Duration = time.Since(start)
	return report, nil
}

func sanitizeID(s string) string {
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:4])
}
