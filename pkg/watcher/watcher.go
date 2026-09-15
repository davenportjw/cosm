package watcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cosmscm/cosm/pkg/codecs"
	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/ignore"
	"github.com/cosmscm/cosm/pkg/storage"
)

// fileSnapshot caches the timestamp and content hash of a tracked file.
type fileSnapshot struct {
	ModTime   time.Time
	SizeBytes int64
	Hash      string
}

// ScanReport summarizes changes discovered during a watcher scan.
type ScanReport struct {
	AddedFiles    []string `json:"added_files"`
	ModifiedFiles []string `json:"modified_files"`
	DeletedFiles  []string `json:"deleted_files"`
	ManifestHash  string   `json:"manifest_hash,omitempty"`
}

// Watcher monitors the workspace filesystem, re-parses modified AST components,
// and auto-stages them into the active micro-universe manifest.
type Watcher struct {
	workspaceDir string
	universeID   string
	pollInterval time.Duration
	blobStore    *storage.BlobStore
	graphEngine  *storage.GraphEngine
	universeMgr  *storage.UniverseManager
	ignoreEngine *ignore.IgnoreEngine
	tracked      map[string]fileSnapshot
	mu           sync.Mutex
}

// NewWatcher initializes a Watcher by opening the local .cosm storage in workspaceDir.
func NewWatcher(workspaceDir string, pollInterval time.Duration) (*Watcher, error) {
	cosmDir := filepath.Join(workspaceDir, ".cosm")
	blobStore, err := storage.NewBlobStore(filepath.Join(cosmDir, "objects"))
	if err != nil {
		return nil, fmt.Errorf("opening blob store: %w", err)
	}
	graphEngine, err := storage.NewGraphEngine(filepath.Join(cosmDir, "graph.db"))
	if err != nil {
		return nil, fmt.Errorf("opening graph engine: %w", err)
	}
	return NewWatcherWithStorage(workspaceDir, "universe-main", blobStore, graphEngine, pollInterval), nil
}

// NewWatcherWithStorage initializes a Watcher with supplied storage instances.
func NewWatcherWithStorage(
	workspaceDir string,
	universeID string,
	blobStore *storage.BlobStore,
	graphEngine *storage.GraphEngine,
	pollInterval time.Duration,
) *Watcher {
	if universeID == "" {
		universeID = "universe-main"
	}
	if pollInterval <= 0 {
		pollInterval = 200 * time.Millisecond
	}
	engine, err := ignore.LoadWorkspaceRules(workspaceDir)
	if err != nil {
		engine = ignore.NewIgnoreEngine(workspaceDir)
	}
	return &Watcher{
		workspaceDir: workspaceDir,
		universeID:   universeID,
		pollInterval: pollInterval,
		blobStore:    blobStore,
		graphEngine:  graphEngine,
		universeMgr:  storage.NewUniverseManager(graphEngine, blobStore),
		ignoreEngine: engine,
		tracked:      make(map[string]fileSnapshot),
	}
}

// Start runs the periodic scanning loop until ctx is cancelled.
func (w *Watcher) Start(ctx context.Context) error {
	// Initial baseline scan
	_, _ = w.ScanOnce()

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := w.ScanOnce(); err != nil {
				// Continue on error
				continue
			}
		}
	}
}

// ScanOnce performs a single pass over the workspace filesystem, detecting
// added, modified, and deleted files, auto-parsing AST symbols, and updating
// the active universe manifest.
func (w *Watcher) ScanOnce() (*ScanReport, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	currentFiles := make(map[string]fileSnapshot)
	report := &ScanReport{
		AddedFiles:    []string{},
		ModifiedFiles: []string{},
		DeletedFiles:  []string{},
	}

	err := filepath.WalkDir(w.workspaceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		relPath, rErr := filepath.Rel(w.workspaceDir, path)
		if rErr != nil {
			return nil
		}
		if relPath == "." {
			return nil
		}

		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "dist" ||
				name == "target" || name == "vendor" || name == "build" || name == "__pycache__" ||
				(w.ignoreEngine != nil && w.ignoreEngine.ShouldIgnorePath(relPath, true)) {
				return filepath.SkipDir
			}
			return nil
		}

		if w.ignoreEngine != nil && w.ignoreEngine.ShouldIgnorePath(relPath, false) {
			return nil
		}

		info, iErr := d.Info()
		if iErr != nil {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(relPath))
		if !codecs.SupportsExtension(ext) && !codecs.IsDockerfile(relPath) {
			return nil
		}

		currentFiles[relPath] = fileSnapshot{
			ModTime:   info.ModTime(),
			SizeBytes: info.Size(),
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking workspace: %w", err)
	}

	// 1. Detect Added and Modified files
	var filesToProcess []string

	for relPath, snap := range currentFiles {
		oldSnap, exists := w.tracked[relPath]
		if !exists {
			// Newly added
			content, err := os.ReadFile(filepath.Join(w.workspaceDir, relPath))
			if err != nil {
				continue
			}
			h := sha256.Sum256(content)
			hashStr := hex.EncodeToString(h[:])
			snap.Hash = hashStr
			currentFiles[relPath] = snap
			w.tracked[relPath] = snap
			report.AddedFiles = append(report.AddedFiles, relPath)
			filesToProcess = append(filesToProcess, relPath)
		} else if snap.SizeBytes != oldSnap.SizeBytes || !snap.ModTime.Equal(oldSnap.ModTime) {
			// Possibly modified, verify with sha256
			content, err := os.ReadFile(filepath.Join(w.workspaceDir, relPath))
			if err != nil {
				continue
			}
			h := sha256.Sum256(content)
			hashStr := hex.EncodeToString(h[:])
			if hashStr != oldSnap.Hash {
				snap.Hash = hashStr
				currentFiles[relPath] = snap
				w.tracked[relPath] = snap
				report.ModifiedFiles = append(report.ModifiedFiles, relPath)
				filesToProcess = append(filesToProcess, relPath)
			} else {
				// Content did not change, just touch time
				currentFiles[relPath] = oldSnap
			}
		} else {
			currentFiles[relPath] = oldSnap
		}
	}

	// 2. Detect Deleted files
	for relPath := range w.tracked {
		if _, exists := currentFiles[relPath]; !exists {
			report.DeletedFiles = append(report.DeletedFiles, relPath)
			delete(w.tracked, relPath)
		}
	}

	if len(report.AddedFiles) == 0 && len(report.ModifiedFiles) == 0 && len(report.DeletedFiles) == 0 {
		return report, nil
	}

	// 3. Process changes and update active Universe Manifest
	head, _ := w.universeMgr.GetUniverseManifest(w.universeID)
	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: filepath.Base(w.workspaceDir),
		UniverseID:  w.universeID,
		CreatedAt:   time.Now().UTC(),
	}
	if head != nil {
		manifest.Components = append([]string{}, head.Components...)
		manifest.CrossEdges = append([]core.CrossBoundaryEdge{}, head.CrossEdges...)
	}

	// Load existing components
	compMap := make(map[string]*core.ComponentNode)
	symMap := make(map[string]*core.ASTSymbolNode)
	if head != nil {
		for _, cID := range head.Components {
			var compData []byte
			if node, gErr := w.graphEngine.GetNode(cID); gErr == nil && node != nil && node.MerkleHash != "" {
				compData, _ = w.blobStore.Get(node.MerkleHash)
			} else {
				compData, _ = w.blobStore.Get(cID)
			}
			if len(compData) > 0 {
				var c core.ComponentNode
				if e := json.Unmarshal(compData, &c); e == nil {
					cCopy := c
					compMap[c.ComponentID] = &cCopy
					compMap[cID] = &cCopy
					for _, sID := range c.SymbolNodes {
						var sData []byte
						if sNode, sgErr := w.graphEngine.GetNode(sID); sgErr == nil && sNode != nil && sNode.MerkleHash != "" {
							sData, _ = w.blobStore.Get(sNode.MerkleHash)
						} else {
							sData, _ = w.blobStore.Get(sID)
						}
						if len(sData) > 0 {
							var s core.ASTSymbolNode
							if se := json.Unmarshal(sData, &s); se == nil {
								sCopy := s
								symMap[s.NodeID] = &sCopy
							}
						}
					}
				}
			}
		}
	}

	// Handle deleted files: remove components referencing deleted file
	if len(report.DeletedFiles) > 0 {
		var filteredComps []string
		for _, cID := range manifest.Components {
			c := compMap[cID]
			isDeleted := false
			if c != nil {
				filePath := c.Metadata["file_path"]
				for _, delFile := range report.DeletedFiles {
					if c.Name == delFile || filePath == delFile ||
						strings.HasSuffix(delFile, c.Name) ||
						(filePath != "" && strings.HasSuffix(delFile, filePath)) {
						isDeleted = true
						break
					}
				}
			}
			if isDeleted {
				for _, sID := range c.SymbolNodes {
					delete(symMap, sID)
				}
				delete(compMap, cID)
				continue
			}
			filteredComps = append(filteredComps, cID)
		}
		manifest.Components = filteredComps
	}

	// Handle added/modified files: parse and stage
	for _, relPath := range filesToProcess {
		content, err := os.ReadFile(filepath.Join(w.workspaceDir, relPath))
		if err != nil {
			continue
		}

		lineageEnv := core.LineageEnvelope{
			ExecutingAgentID: "cosm-watcher-daemon",
			Intent:           fmt.Sprintf("Auto-staged by watcher: %s", relPath),
			Timestamp:        time.Now().UTC(),
		}

		parsed, pErr := codecs.ParseSourceFile(relPath, content, lineageEnv)
		var comp *core.ComponentNode
		var syms map[string]*core.ASTSymbolNode

		if pErr == nil && parsed != nil && parsed.Component != nil {
			comp = parsed.Component
			syms = parsed.Symbols
		} else {
			h := sha256.Sum256([]byte(relPath))
			shortHash := hex.EncodeToString(h[:4])
			rawSymID := fmt.Sprintf("raw:%s", shortHash)
			rawSym := &core.ASTSymbolNode{
				NodeID:      rawSymID,
				Language:    core.LangRaw,
				NodeType:    "RawBlobNode",
				Identifier:  relPath,
				ASTPayload:  content,
				ASTMetadata: map[string]string{"file_path": relPath},
				Lineage:     lineageEnv,
			}
			syms = map[string]*core.ASTSymbolNode{rawSymID: rawSym}
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

		// Persist symbols
		for symID, sym := range syms {
			data, _ := json.Marshal(sym)
			sHash, _ := w.blobStore.Put(data)
			_ = w.graphEngine.PutNode(storage.NodeRecord{
				NodeID:     symID,
				Language:   sym.Language,
				NodeType:   sym.NodeType,
				MerkleHash: sHash,
				CreatedAt:  time.Now(),
			})
			_ = w.graphEngine.PutLineage(storage.LineageRecord{
				RecordID:         fmt.Sprintf("lin-%s", symID[:min(12, len(symID))]),
				NodeID:           symID,
				ExecutingAgentID: lineageEnv.ExecutingAgentID,
				Intent:           lineageEnv.Intent,
				Timestamp:        lineageEnv.Timestamp,
			})
			symMap[symID] = sym
		}

		// Persist component
		cData, _ := json.Marshal(comp)
		cHash, _ := w.blobStore.Put(cData)
		_ = w.graphEngine.PutNode(storage.NodeRecord{
			NodeID:     comp.ComponentID,
			Language:   comp.Language,
			NodeType:   string(comp.Type),
			MerkleHash: cHash,
			CreatedAt:  time.Now(),
		})

		// Update manifest component list
		replaced := false
		for i, cID := range manifest.Components {
			existing := compMap[cID]
			if existing != nil && (existing.Name == comp.Name || existing.Metadata["file_path"] == relPath) {
				manifest.Components[i] = comp.ComponentID
				replaced = true
				break
			}
		}
		if !replaced {
			manifest.Components = append(manifest.Components, comp.ComponentID)
		}
		compMap[comp.ComponentID] = comp
	}

	// 4. Re-infer Cross Boundary Edges
	var compList []*core.ComponentNode
	for _, cID := range manifest.Components {
		if c := compMap[cID]; c != nil {
			compList = append(compList, c)
		}
	}
	linker := core.NewCrossBoundaryLinker()
	if w.ignoreEngine != nil && w.ignoreEngine.EdgeFilter != nil {
		linker.SetFilter(w.ignoreEngine.EdgeFilter)
	}
	edges, eErr := linker.LinkWorkspace(compList, symMap)
	if eErr == nil {
		manifest.CrossEdges = edges
		for _, edge := range edges {
			_ = w.graphEngine.PutEdge(storage.EdgeRecord{
				SourceID: edge.SourceNodeID,
				TargetID: edge.TargetNodeID,
				EdgeType: edge.Type,
			})
		}
	}

	// 5. Commit updated manifest
	mHash, err := w.universeMgr.CommitManifest(w.universeID, manifest)
	if err == nil {
		report.ManifestHash = mHash
	}

	return report, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
