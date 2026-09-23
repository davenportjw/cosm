package mutation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cosmscm/cosm/pkg/codecs"
	"github.com/cosmscm/cosm/pkg/codecs/golang"
	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

// ASTOperationType defines high-level declarative mutation operations on AST symbols.
type ASTOperationType string

const (
	// OpReplaceFunctionBody replaces only the inner body of a function/method, preserving signature, parameters, return types, and docstrings.
	OpReplaceFunctionBody ASTOperationType = "replace_function_body"

	// OpReplaceFunction replaces the entire function/method definition (signature + body).
	OpReplaceFunction ASTOperationType = "replace_function"

	// OpAddMethod adds a new method to a class, struct, or component.
	OpAddMethod ASTOperationType = "add_method"

	// OpAddBefore inserts a new AST symbol immediately preceding the target symbol.
	OpAddBefore ASTOperationType = "add_before"

	// OpAddAfter inserts a new AST symbol immediately succeeding the target symbol.
	OpAddAfter ASTOperationType = "add_after"

	// OpDelete removes an AST symbol from its enclosing component.
	OpDelete ASTOperationType = "delete"

	// OpAddImport adds an import statement/dependency to a component.
	OpAddImport ASTOperationType = "add_import"

	// OpReplaceImports replaces the imports section of a component.
	OpReplaceImports ASTOperationType = "replace_imports"

	// OpReplaceGlobal replaces a top-level global variable, constant, or resource block.
	OpReplaceGlobal ASTOperationType = "replace_global"

	// OpCreateComponent creates a brand new component and symbol tree directly in the universe manifest.
	OpCreateComponent ASTOperationType = "create_component"
)

// IsValid checks if the operation type is supported.
func (op ASTOperationType) IsValid() bool {
	switch op {
	case OpReplaceFunctionBody, OpReplaceFunction, OpAddMethod,
		OpAddBefore, OpAddAfter, OpDelete, OpAddImport,
		OpReplaceImports, OpReplaceGlobal, OpCreateComponent,
		"insert_after", "insert_before", "append_child":
		return true
	default:
		return false
	}
}

// ASTOperation encapsulates a single surgical AST mutation step.
type ASTOperation struct {
	Operation ASTOperationType  `json:"operation"`
	Target    string            `json:"target"` // Scoped path (e.g. "LRUCache.get", "services/auth::ValidateToken", or node ID)
	Content   string            `json:"content"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// ASTEditBatch encapsulates an atomic batch of AST operations to be applied to a micro-universe.
type ASTEditBatch struct {
	UniverseID string               `json:"universe_id"`
	Operations []ASTOperation       `json:"operations"`
	Lineage    core.LineageEnvelope `json:"lineage"`
}

// ModifiedSymbolDetail provides rich identification of a mutated symbol.
type ModifiedSymbolDetail struct {
	NodeID        string `json:"node_id"`
	Identifier    string `json:"identifier"`
	NodeType      string `json:"node_type,omitempty"`
	ComponentName string `json:"component_name,omitempty"`
}

// BatchMutationResult returns the Merkle tree delta and audit trail for an applied batch.
type BatchMutationResult struct {
	UniverseID             string                 `json:"universe_id"`
	OldManifestHash        string                 `json:"old_manifest_hash"`
	NewManifestHash        string                 `json:"new_manifest_hash"`
	AppliedOperations      int                    `json:"applied_operations"`
	ModifiedSymbols        []string               `json:"modified_symbols"`
	ModifiedSymbolsDetails []ModifiedSymbolDetail `json:"modified_symbols_details,omitempty"`
	AddedSymbols           []string               `json:"added_symbols"`
	DeletedSymbols         []string               `json:"deleted_symbols"`
	DeduplicatedSymbols    int                    `json:"deduplicated_symbols"`
	Duration               time.Duration          `json:"duration"`
}

// ResolvedSymbol contains the located symbol, its parent component, and index within the component.
type ResolvedSymbol struct {
	SymbolNode  *core.ASTSymbolNode
	Component   *core.ComponentNode
	CompIndex   int
	SymbolIndex int
	Manifest    *core.WorkspaceManifestNode
}

// ResolveSymbol resolves a target string (node ID, prefix, dotted name, or scoped path) within a universe.
func (e *SurgeryEngine) ResolveSymbol(universeID string, target string) (*ResolvedSymbol, error) {
	manifest, err := e.universeMgr.GetUniverseManifest(universeID)
	if err != nil || manifest == nil {
		return nil, fmt.Errorf("universe manifest not found for %s: %w", universeID, err)
	}

	target = strings.TrimSpace(target)
	if target == "" {
		return nil, fmt.Errorf("target identifier cannot be empty")
	}

	var bestMatch *ResolvedSymbol
	var candidates []*ResolvedSymbol

	// Split scoped target if using "::" e.g. "services/auth::ValidateToken"
	scopedComp := ""
	scopedSym := target
	if parts := strings.Split(target, "::"); len(parts) == 2 {
		scopedComp = strings.TrimSpace(parts[0])
		scopedSym = strings.TrimSpace(parts[1])
	}

	for cIdx, compID := range manifest.Components {
		var compBytes []byte
		var err error
		if nodeRec, nErr := e.graphEngine.GetNode(compID); nErr == nil && nodeRec != nil && nodeRec.MerkleHash != "" {
			compBytes, err = e.blobStore.Get(nodeRec.MerkleHash)
		} else {
			compBytes, err = e.blobStore.Get(compID)
		}
		if err != nil {
			continue
		}
		var comp core.ComponentNode
		if err := json.Unmarshal(compBytes, &comp); err != nil {
			continue
		}

		// If scoped component specified, check match
		if scopedComp != "" {
			matched := comp.Name == scopedComp ||
				comp.Metadata["dir_path"] == scopedComp ||
				comp.Metadata["file_path"] == scopedComp ||
				strings.Contains(comp.Name, scopedComp) ||
				strings.Contains(comp.Metadata["file_path"], scopedComp) ||
				strings.Contains(comp.Metadata["dir_path"], scopedComp)
			if !matched {
				continue
			}
		}

		for sIdx, sID := range comp.SymbolNodes {
			var symBytes []byte
			var sErr error
			if nodeRec, nErr := e.graphEngine.GetNode(sID); nErr == nil && nodeRec != nil && nodeRec.MerkleHash != "" {
				symBytes, sErr = e.blobStore.Get(nodeRec.MerkleHash)
			} else {
				symBytes, sErr = e.blobStore.Get(sID)
			}
			if sErr != nil {
				continue
			}
			var sym core.ASTSymbolNode
			if err := json.Unmarshal(symBytes, &sym); err != nil {
				continue
			}
			if sym.NodeID == "" {
				sym.NodeID = sID
			}

			resolved := &ResolvedSymbol{
				SymbolNode:  &sym,
				Component:   &comp,
				CompIndex:   cIdx,
				SymbolIndex: sIdx,
				Manifest:    manifest,
			}

			// 1. Exact Node ID match (64-char hex)
			if sym.NodeID == target || sID == target {
				return resolved, nil
			}

			// 2. Exact identifier match
			if sym.Identifier == target || sym.Identifier == scopedSym {
				return resolved, nil
			}

			// 3. Prefix match on Node ID
			if len(target) >= 6 && strings.HasPrefix(sym.NodeID, target) {
				candidates = append(candidates, resolved)
				continue
			}

			// 4. Dotted name suffix match (e.g. "get" matches "LRUCache.get" or "main.get")
			if strings.HasSuffix(sym.Identifier, "."+scopedSym) || strings.HasSuffix(sym.Identifier, "::"+scopedSym) {
				return resolved, nil
			}

			// 5. Case-insensitive identifier match
			if strings.EqualFold(sym.Identifier, scopedSym) {
				candidates = append(candidates, resolved)
				continue
			}

			// 6. Substring or receiver-stripped match (e.g. "HandleHealth" matches "main.(Server).HandleHealth" or vice versa)
			if strings.Contains(sym.Identifier, scopedSym) || strings.Contains(scopedSym, sym.Identifier) {
				candidates = append(candidates, resolved)
				continue
			}
			baseSym := scopedSym
			if idx := strings.LastIndexAny(scopedSym, ".)"); idx != -1 && idx < len(scopedSym)-1 {
				baseSym = scopedSym[idx+1:]
			}
			if baseSym != "" && (strings.HasSuffix(sym.Identifier, baseSym) || strings.Contains(sym.Identifier, baseSym)) {
				candidates = append(candidates, resolved)
				continue
			}
		}
	}

	if len(candidates) == 1 {
		return candidates[0], nil
	} else if len(candidates) > 1 {
		// Prefer candidate whose identifier has exact suffix
		for _, c := range candidates {
			if strings.HasSuffix(c.SymbolNode.Identifier, "::"+scopedSym) || strings.HasSuffix(c.SymbolNode.Identifier, "."+scopedSym) {
				return c, nil
			}
		}
		// Prefer candidate whose identifier has exact token
		for _, c := range candidates {
			if strings.Contains(c.SymbolNode.Identifier, scopedSym) {
				return c, nil
			}
		}
		return candidates[0], nil
	}

	if bestMatch != nil {
		return bestMatch, nil
	}

	// Fallback: If target matches a component name or file path, resolve to the last symbol of that component
	for cIdx, compID := range manifest.Components {
		var compBytes []byte
		if nodeRec, nErr := e.graphEngine.GetNode(compID); nErr == nil && nodeRec != nil && nodeRec.MerkleHash != "" {
			compBytes, _ = e.blobStore.Get(nodeRec.MerkleHash)
		} else {
			compBytes, _ = e.blobStore.Get(compID)
		}
		if compBytes == nil {
			continue
		}
		var comp core.ComponentNode
		if err := json.Unmarshal(compBytes, &comp); err != nil {
			continue
		}
		matched := comp.Name == target ||
			comp.Metadata["file_path"] == target ||
			strings.Contains(comp.Metadata["file_path"], target) ||
			strings.Contains(target, comp.Name)
		if matched && len(comp.SymbolNodes) > 0 {
			lastSymID := comp.SymbolNodes[len(comp.SymbolNodes)-1]
			var symBytes []byte
			if nodeRec, nErr := e.graphEngine.GetNode(lastSymID); nErr == nil && nodeRec != nil && nodeRec.MerkleHash != "" {
				symBytes, _ = e.blobStore.Get(nodeRec.MerkleHash)
			} else {
				symBytes, _ = e.blobStore.Get(lastSymID)
			}
			if symBytes != nil {
				var sym core.ASTSymbolNode
				if json.Unmarshal(symBytes, &sym) == nil {
					if sym.NodeID == "" {
						sym.NodeID = lastSymID
					}
					return &ResolvedSymbol{
						SymbolNode:  &sym,
						Component:   &comp,
						CompIndex:   cIdx,
						SymbolIndex: len(comp.SymbolNodes) - 1,
						Manifest:    manifest,
					}, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("symbol %q not found in universe %s", target, universeID)
}

// ResolveSymbol resolves a human symbol identifier or path in a universe using UniverseManager.
func ResolveSymbol(universeMgr *storage.UniverseManager, universeID, target string) (*ResolvedSymbol, error) {
	if universeMgr == nil {
		return nil, fmt.Errorf("universe manager is nil")
	}
	surgeryEngine := NewSurgeryEngine(universeMgr.BlobStore(), universeMgr.GraphEngine(), universeMgr)
	return surgeryEngine.ResolveSymbol(universeID, target)
}

// ApplyASTEditBatch executes a batch of declarative AST operations atomically on a micro-universe.
func (e *SurgeryEngine) ApplyASTEditBatch(batch *ASTEditBatch) (*BatchMutationResult, error) {
	start := time.Now()

	if batch == nil {
		return nil, fmt.Errorf("edit batch cannot be nil")
	}
	if batch.UniverseID == "" {
		batch.UniverseID = "universe-main"
	}
	if len(batch.Operations) == 0 {
		return nil, fmt.Errorf("no operations provided in batch")
	}

	manifest, err := e.universeMgr.GetUniverseManifest(batch.UniverseID)
	if err != nil || manifest == nil {
		// Check if batch contains OpCreateComponent to allow blank-slate inception
		hasCreate := false
		for _, o := range batch.Operations {
			if o.Operation == OpCreateComponent {
				hasCreate = true
				break
			}
		}
		if hasCreate {
			manifest = &core.WorkspaceManifestNode{
				WorkspaceID: "ws-" + batch.UniverseID,
				UniverseID:  batch.UniverseID,
				Components:  []string{},
				CrossEdges:  []core.CrossBoundaryEdge{},
				Lineage:     batch.Lineage,
				CreatedAt:   time.Now().UTC(),
			}
		} else {
			return nil, fmt.Errorf("active manifest not found for universe %s: %w", batch.UniverseID, err)
		}
	}
	oldManifestHash := manifest.MerkleRootHash

	var modifiedSymbols []string
	var modifiedDetails []ModifiedSymbolDetail
	var addedSymbols []string
	var deletedSymbols []string
	dedupCount := 0

	for opIdx, op := range batch.Operations {
		if !op.Operation.IsValid() {
			return nil, fmt.Errorf("operation #%d has invalid type: %s", opIdx, op.Operation)
		}

		opType := op.Operation
		switch opType {
		case "insert_after", "append_child":
			opType = OpAddAfter
		case "insert_before":
			opType = OpAddBefore
		}

		switch opType {
		case OpReplaceFunctionBody:
			resolved, err := e.ResolveSymbol(batch.UniverseID, op.Target)
			if err != nil {
				return nil, fmt.Errorf("operation #%d (%s): %w", opIdx, op.Operation, err)
			}
			newSym, err := e.applyReplaceFunctionBody(resolved.SymbolNode, op.Content, batch.Lineage)
			if err != nil {
				return nil, fmt.Errorf("operation #%d (%s): %w", opIdx, op.Operation, err)
			}
			mutRes, err := e.MutateSymbol(batch.UniverseID, resolved.SymbolNode.NodeID, newSym.ASTPayload, batch.Lineage)
			if err != nil {
				return nil, fmt.Errorf("operation #%d mutation failed: %w", opIdx, err)
			}
			modifiedSymbols = append(modifiedSymbols, mutRes.NewSymbolID)
			modifiedDetails = append(modifiedDetails, ModifiedSymbolDetail{
				NodeID:        mutRes.NewSymbolID,
				Identifier:    mutRes.SymbolIdentifier,
				NodeType:      mutRes.NodeType,
				ComponentName: mutRes.ComponentName,
			})
			dedupCount += mutRes.DeduplicatedSymbolsCount

		case OpReplaceFunction:
			resolved, err := e.ResolveSymbol(batch.UniverseID, op.Target)
			if err != nil {
				return nil, fmt.Errorf("operation #%d (%s): %w", opIdx, op.Operation, err)
			}
			mutRes, err := e.MutateSymbolFromCode(batch.UniverseID, resolved.SymbolNode.NodeID, op.Content, batch.Lineage)
			if err != nil {
				return nil, fmt.Errorf("operation #%d (%s): %w", opIdx, op.Operation, err)
			}
			modifiedSymbols = append(modifiedSymbols, mutRes.NewSymbolID)
			modifiedDetails = append(modifiedDetails, ModifiedSymbolDetail{
				NodeID:        mutRes.NewSymbolID,
				Identifier:    mutRes.SymbolIdentifier,
				NodeType:      mutRes.NodeType,
				ComponentName: mutRes.ComponentName,
			})
			dedupCount += mutRes.DeduplicatedSymbolsCount

		case OpAddMethod, OpAddAfter, OpAddBefore:
			resolved, err := e.ResolveSymbol(batch.UniverseID, op.Target)
			if err != nil {
				// If target cannot be resolved as symbol, try resolving as component
				return nil, fmt.Errorf("operation #%d (%s) target resolution failed: %w", opIdx, op.Operation, err)
			}

			// Parse new code into AST symbol
			newSymNode, err := e.parseCodeToSymbol(resolved.SymbolNode.Language, "new_symbol", op.Content, batch.Lineage)
			if err != nil {
				// Fallback to raw symbol node
				sum := sha256.Sum256([]byte(op.Content))
				newSymNode = &core.ASTSymbolNode{
					Language:   resolved.SymbolNode.Language,
					NodeType:   "MethodDecl",
					Identifier: fmt.Sprintf("%s.method", resolved.SymbolNode.Identifier),
					ASTPayload: []byte(op.Content),
					Lineage:    batch.Lineage,
				}
				newSymNode.NodeID = hex.EncodeToString(sum[:])
			}

			newSymBytes, _ := json.Marshal(newSymNode)
			symBlobHash, err := e.blobStore.Put(newSymBytes)
			if err != nil {
				return nil, fmt.Errorf("storing new symbol blob: %w", err)
			}
			newSymNode.NodeID = symBlobHash

			_ = e.graphEngine.PutNode(storage.NodeRecord{
				NodeID:     newSymNode.NodeID,
				Language:   newSymNode.Language,
				NodeType:   newSymNode.NodeType,
				MerkleHash: symBlobHash,
				CreatedAt:  time.Now().UTC(),
			})

			// Insert symbol into component
			insertIdx := resolved.SymbolIndex + 1
			if op.Operation == OpAddBefore {
				insertIdx = resolved.SymbolIndex
			}

			var newSymList []string
			for i, sID := range resolved.Component.SymbolNodes {
				if i == insertIdx {
					newSymList = append(newSymList, newSymNode.NodeID)
				}
				newSymList = append(newSymList, sID)
			}
			if insertIdx >= len(resolved.Component.SymbolNodes) {
				newSymList = append(newSymList, newSymNode.NodeID)
			}

			// Update Component
			updatedComp := &core.ComponentNode{
				Name:        resolved.Component.Name,
				Type:        resolved.Component.Type,
				Language:    resolved.Component.Language,
				SymbolNodes: newSymList,
				Metadata:    resolved.Component.Metadata,
				Lineage:     batch.Lineage,
			}
			compBytes, _ := json.Marshal(updatedComp)
			compHash, err := e.blobStore.Put(compBytes)
			if err != nil {
				return nil, fmt.Errorf("storing updated component: %w", err)
			}
			updatedComp.ComponentID = compHash

			// Update Manifest
			currManifest, _ := e.universeMgr.GetUniverseManifest(batch.UniverseID)
			newComponents := make([]string, len(currManifest.Components))
			copy(newComponents, currManifest.Components)
			newComponents[resolved.CompIndex] = compHash

			newManifest := &core.WorkspaceManifestNode{
				WorkspaceID: currManifest.WorkspaceID,
				UniverseID:  batch.UniverseID,
				Components:  newComponents,
				CrossEdges:  currManifest.CrossEdges,
				Lineage:     batch.Lineage,
				CreatedAt:   time.Now().UTC(),
			}
			mBytes, _ := json.Marshal(newManifest)
			mHash, _ := e.blobStore.Put(mBytes)
			newManifest.MerkleRootHash = mHash

			_, err = e.universeMgr.CommitManifest(batch.UniverseID, newManifest)
			if err != nil {
				return nil, fmt.Errorf("committing manifest after add: %w", err)
			}

			addedSymbols = append(addedSymbols, newSymNode.NodeID)

		case OpDelete:
			resolved, err := e.ResolveSymbol(batch.UniverseID, op.Target)
			if err != nil {
				return nil, fmt.Errorf("operation #%d (%s): %w", opIdx, op.Operation, err)
			}

			var newSymList []string
			for i, sID := range resolved.Component.SymbolNodes {
				if i == resolved.SymbolIndex || sID == resolved.SymbolNode.NodeID {
					continue
				}
				newSymList = append(newSymList, sID)
			}

			updatedComp := &core.ComponentNode{
				Name:        resolved.Component.Name,
				Type:        resolved.Component.Type,
				Language:    resolved.Component.Language,
				SymbolNodes: newSymList,
				Metadata:    resolved.Component.Metadata,
				Lineage:     batch.Lineage,
			}
			compBytes, _ := json.Marshal(updatedComp)
			compHash, _ := e.blobStore.Put(compBytes)
			updatedComp.ComponentID = compHash

			currManifest, _ := e.universeMgr.GetUniverseManifest(batch.UniverseID)
			newComponents := make([]string, len(currManifest.Components))
			copy(newComponents, currManifest.Components)
			newComponents[resolved.CompIndex] = compHash

			newManifest := &core.WorkspaceManifestNode{
				WorkspaceID: currManifest.WorkspaceID,
				UniverseID:  batch.UniverseID,
				Components:  newComponents,
				CrossEdges:  currManifest.CrossEdges,
				Lineage:     batch.Lineage,
				CreatedAt:   time.Now().UTC(),
			}
			mBytes, _ := json.Marshal(newManifest)
			mHash, _ := e.blobStore.Put(mBytes)
			newManifest.MerkleRootHash = mHash

			_, err = e.universeMgr.CommitManifest(batch.UniverseID, newManifest)
			if err != nil {
				return nil, fmt.Errorf("committing manifest after delete: %w", err)
			}

			deletedSymbols = append(deletedSymbols, resolved.SymbolNode.NodeID)

		case OpAddImport, OpReplaceImports:
			// Apply import mutations to first or targeted component
			resolved, err := e.ResolveSymbol(batch.UniverseID, op.Target)
			var targetComp *core.ComponentNode
			var compIdx int
			currManifest, _ := e.universeMgr.GetUniverseManifest(batch.UniverseID)

			if err == nil && resolved != nil {
				targetComp = resolved.Component
				compIdx = resolved.CompIndex
			} else if len(currManifest.Components) > 0 {
				compIdx = 0
				cBytes, _ := e.blobStore.Get(currManifest.Components[0])
				_ = json.Unmarshal(cBytes, &targetComp)
			}

			if targetComp != nil {
				if targetComp.Metadata == nil {
					targetComp.Metadata = make(map[string]string)
				}
				if op.Operation == OpAddImport {
					existing := targetComp.Metadata["imports"]
					if existing != "" {
						targetComp.Metadata["imports"] = existing + "," + op.Content
					} else {
						targetComp.Metadata["imports"] = op.Content
					}
				} else {
					targetComp.Metadata["imports"] = op.Content
				}

				compBytes, _ := json.Marshal(targetComp)
				compHash, _ := e.blobStore.Put(compBytes)
				targetComp.ComponentID = compHash

				newComponents := make([]string, len(currManifest.Components))
				copy(newComponents, currManifest.Components)
				newComponents[compIdx] = compHash

				newManifest := &core.WorkspaceManifestNode{
					WorkspaceID: currManifest.WorkspaceID,
					UniverseID:  batch.UniverseID,
					Components:  newComponents,
					CrossEdges:  currManifest.CrossEdges,
					Lineage:     batch.Lineage,
					CreatedAt:   time.Now().UTC(),
				}
				mBytes, _ := json.Marshal(newManifest)
				mHash, _ := e.blobStore.Put(mBytes)
				newManifest.MerkleRootHash = mHash
				_, _ = e.universeMgr.CommitManifest(batch.UniverseID, newManifest)
			}

		case OpReplaceGlobal:
			resolved, err := e.ResolveSymbol(batch.UniverseID, op.Target)
			if err == nil && resolved != nil {
				mutRes, mErr := e.MutateSymbol(batch.UniverseID, resolved.SymbolNode.NodeID, []byte(op.Content), batch.Lineage)
				if mErr == nil {
					modifiedSymbols = append(modifiedSymbols, mutRes.NewSymbolID)
					modifiedDetails = append(modifiedDetails, ModifiedSymbolDetail{
						NodeID:        mutRes.NewSymbolID,
						Identifier:    mutRes.SymbolIdentifier,
						NodeType:      mutRes.NodeType,
						ComponentName: mutRes.ComponentName,
					})
				}
			}

		case OpCreateComponent:
			relPath := op.Target
			if relPath == "" {
				relPath = "component"
			}
			if op.Metadata != nil && op.Metadata["file_path"] != "" {
				relPath = op.Metadata["file_path"]
			}

			parsed, pErr := codecs.ParseSourceFile(relPath, []byte(op.Content), batch.Lineage)
			var comp *core.ComponentNode
			syms := make(map[string]*core.ASTSymbolNode)

			if pErr == nil && parsed != nil && parsed.Component != nil {
				comp = parsed.Component
				syms = parsed.Symbols
				if op.Target != "" {
					comp.Name = op.Target
				}
			} else {
				// Raw component fallback
				h := sha256.Sum256([]byte(relPath))
				shortHash := hex.EncodeToString(h[:4])
				rawSymID := fmt.Sprintf("raw:%s", shortHash)
				rawSym := &core.ASTSymbolNode{
					NodeID:            rawSymID,
					Language:          core.LangRaw,
					NodeType:          "RawBlobNode",
					Identifier:        relPath,
					ASTPayload:        []byte(op.Content),
					LocalDependencies: []string{},
					Lineage:           batch.Lineage,
				}
				syms[rawSymID] = rawSym
				comp = &core.ComponentNode{
					ComponentID: fmt.Sprintf("comp-raw-%s", shortHash),
					Name:        op.Target,
					Type:        core.CompService,
					Language:    core.LangRaw,
					SymbolNodes: []string{rawSymID},
					Metadata:    map[string]string{"file_path": relPath},
					Lineage:     batch.Lineage,
				}
			}

			if op.Metadata != nil {
				if comp.Metadata == nil {
					comp.Metadata = make(map[string]string)
				}
				for k, v := range op.Metadata {
					comp.Metadata[k] = v
				}
			}

			// Store all symbols in blobStore and graphEngine
			for sID, sym := range syms {
				data, _ := json.Marshal(sym)
				sHash, _ := e.blobStore.Put(data)
				_ = e.graphEngine.PutNode(storage.NodeRecord{
					NodeID:     sID,
					Language:   sym.Language,
					NodeType:   sym.NodeType,
					MerkleHash: sHash,
					CreatedAt:  time.Now().UTC(),
				})
				linPrefixLen := len(sID)
				if linPrefixLen > 12 {
					linPrefixLen = 12
				}
				_ = e.graphEngine.PutLineage(storage.LineageRecord{
					RecordID:         fmt.Sprintf("lin-%s", sID[:linPrefixLen]),
					NodeID:           sID,
					UserID:           batch.Lineage.UserID,
					UserPrompt:       batch.Lineage.UserPrompt,
					ExecutingAgentID: batch.Lineage.ExecutingAgentID,
					Intent:           batch.Lineage.Intent,
					Timestamp:        batch.Lineage.Timestamp,
				})
				addedSymbols = append(addedSymbols, sID)
			}

			// Store component in blobStore and graphEngine
			compData, _ := json.Marshal(comp)
			compHash, _ := e.blobStore.Put(compData)
			comp.ComponentID = compHash
			_ = e.graphEngine.PutNode(storage.NodeRecord{
				NodeID:     comp.ComponentID,
				Language:   comp.Language,
				NodeType:   string(comp.Type),
				MerkleHash: compHash,
				CreatedAt:  time.Now().UTC(),
			})
			compPrefixLen := len(comp.ComponentID)
			if compPrefixLen > 12 {
				compPrefixLen = 12
			}
			_ = e.graphEngine.PutLineage(storage.LineageRecord{
				RecordID:         fmt.Sprintf("lin-%s", comp.ComponentID[:compPrefixLen]),
				NodeID:           comp.ComponentID,
				UserID:           batch.Lineage.UserID,
				UserPrompt:       batch.Lineage.UserPrompt,
				ExecutingAgentID: batch.Lineage.ExecutingAgentID,
				Intent:           batch.Lineage.Intent,
				Timestamp:        batch.Lineage.Timestamp,
			})

			// Add component to active manifest
			currManifest, mErr := e.universeMgr.GetUniverseManifest(batch.UniverseID)
			var newComponents []string
			var crossEdges []core.CrossBoundaryEdge
			wsID := "ws-" + batch.UniverseID
			if mErr == nil && currManifest != nil {
				wsID = currManifest.WorkspaceID
				newComponents = append(newComponents, currManifest.Components...)
				crossEdges = currManifest.CrossEdges
			}
			newComponents = append(newComponents, compHash)

			newManifest := &core.WorkspaceManifestNode{
				WorkspaceID: wsID,
				UniverseID:  batch.UniverseID,
				Components:  newComponents,
				CrossEdges:  crossEdges,
				Lineage:     batch.Lineage,
				CreatedAt:   time.Now().UTC(),
			}
			mBytes, _ := json.Marshal(newManifest)
			mHash, _ := e.blobStore.Put(mBytes)
			newManifest.MerkleRootHash = mHash
			_, _ = e.universeMgr.CommitManifest(batch.UniverseID, newManifest)
		}
	}

	finalManifest, err := e.universeMgr.GetUniverseManifest(batch.UniverseID)
	if err != nil || finalManifest == nil {
		return nil, fmt.Errorf("getting final manifest: %w", err)
	}

	return &BatchMutationResult{
		UniverseID:             batch.UniverseID,
		OldManifestHash:        oldManifestHash,
		NewManifestHash:        finalManifest.MerkleRootHash,
		AppliedOperations:      len(batch.Operations),
		ModifiedSymbols:        modifiedSymbols,
		ModifiedSymbolsDetails: modifiedDetails,
		AddedSymbols:           addedSymbols,
		DeletedSymbols:         deletedSymbols,
		DeduplicatedSymbols:    dedupCount,
		Duration:               time.Since(start),
	}, nil
}

// applyReplaceFunctionBody replaces the inner body while preserving signature and metadata.
func (e *SurgeryEngine) applyReplaceFunctionBody(sym *core.ASTSymbolNode, newBody string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	if sym == nil {
		return nil, fmt.Errorf("symbol cannot be nil")
	}

	switch sym.Language {
	case core.LangGo:
		if sym.NodeType == "FunctionDecl" {
			var fn golang.GoFuncSymbol
			if err := json.Unmarshal(sym.ASTPayload, &fn); err == nil && fn.Name != "" {
				fn.BodySource = newBody
				updatedPayload, err := json.Marshal(fn)
				if err != nil {
					return nil, fmt.Errorf("marshaling updated GoFuncSymbol: %w", err)
				}
				return &core.ASTSymbolNode{
					Language:          sym.Language,
					NodeType:          sym.NodeType,
					Identifier:        sym.Identifier,
					Signature:         sym.Signature,
					Docstring:         sym.Docstring,
					Visibility:        sym.Visibility,
					ASTPayload:        updatedPayload,
					ASTMetadata:       sym.ASTMetadata,
					LocalDependencies: sym.LocalDependencies,
					Dependencies:      sym.Dependencies,
					Lineage:           lineage,
				}, nil
			}
		}
		// Fallback for Go raw payload: splice body
		return e.spliceBodyString(sym, newBody, lineage)

	case core.LangPython:
		// Splicing for Python functions
		return e.splicePythonFunctionBody(sym, newBody, lineage)

	default:
		return e.spliceBodyString(sym, newBody, lineage)
	}
}

func (e *SurgeryEngine) splicePythonFunctionBody(sym *core.ASTSymbolNode, newBody string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	code := string(sym.ASTPayload)
	sigRegex := regexp.MustCompile(`(?m)^( *(?:async +)?def +[a-zA-Z0-9_]+\s*\([^)]*\)(?:\s*->\s*[^:]+)?:)`)
	loc := sigRegex.FindStringIndex(code)

	var newPayload string
	if len(loc) > 0 {
		sig := code[:loc[1]]
		// Ensure newBody is properly indented
		lines := strings.Split(strings.TrimSpace(newBody), "\n")
		var indentedLines []string
		for _, l := range lines {
			if strings.HasPrefix(l, "    ") || strings.HasPrefix(l, "\t") {
				indentedLines = append(indentedLines, l)
			} else {
				indentedLines = append(indentedLines, "    "+l)
			}
		}
		newPayload = sig + "\n" + strings.Join(indentedLines, "\n")
	} else {
		newPayload = newBody
	}

	return &core.ASTSymbolNode{
		Language:          sym.Language,
		NodeType:          sym.NodeType,
		Identifier:        sym.Identifier,
		Signature:         sym.Signature,
		Docstring:         sym.Docstring,
		Visibility:        sym.Visibility,
		ASTPayload:        []byte(newPayload),
		ASTMetadata:       sym.ASTMetadata,
		LocalDependencies: sym.LocalDependencies,
		Dependencies:      sym.Dependencies,
		Lineage:           lineage,
	}, nil
}

func (e *SurgeryEngine) spliceBodyString(sym *core.ASTSymbolNode, newBody string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	return &core.ASTSymbolNode{
		Language:          sym.Language,
		NodeType:          sym.NodeType,
		Identifier:        sym.Identifier,
		Signature:         sym.Signature,
		Docstring:         sym.Docstring,
		Visibility:        sym.Visibility,
		ASTPayload:        []byte(newBody),
		ASTMetadata:       sym.ASTMetadata,
		LocalDependencies: sym.LocalDependencies,
		Dependencies:      sym.Dependencies,
		Lineage:           lineage,
	}, nil
}

// GetStarterSource returns idiomatic starter source code, default relative file path,
// and component type for the requested language. Supported: go, python, typescript, sql.
func GetStarterSource(lang string) (starterSrc string, defaultPath string, defaultType core.ComponentType, err error) {
	normLang := strings.ToLower(strings.TrimSpace(lang))

	switch normLang {
	case "go", "golang":
		defaultPath = "services/main.go"
		defaultType = core.CompService
		starterSrc = `//go:build !ignore

package main

import (
	"context"
	"fmt"
)

// Service encapsulates standard microservice operations.
type Service struct {
	Name string
}

// Start boots the microservice lifecycle.
func (s *Service) Start(ctx context.Context) error {
	fmt.Printf("Starting %s\n", s.Name)
	return nil
}

func main() {
	svc := &Service{Name: "CosmStarter"}
	_ = svc.Start(context.Background())
}
`

	case "python", "py":
		defaultPath = "services/main.py"
		defaultType = core.CompService
		starterSrc = `# -*- coding: utf-8 -*-
"""Standard idiomatic Python microservice component."""

import os
from typing import Dict, Any, Optional

class ServiceHandler:
    def __init__(self, name: str = "default"):
        self.name = name

    def process_event(self, event: Dict[str, Any]) -> Dict[str, Any]:
        return {"status": "ok", "service": self.name}

def handler(event: Dict[str, Any], context: Optional[Any] = None) -> Dict[str, Any]:
    svc = ServiceHandler()
    return svc.process_event(event)
`

	case "typescript", "ts", "tsx", "javascript", "js", "jsx":
		defaultPath = "frontend/src/App.tsx"
		defaultType = core.CompFrontend
		starterSrc = `// @ts-check
"use client";

import React, { useState } from 'react';

export interface ComponentProps {
	title?: string;
}

export function App(props: ComponentProps) {
	const [count, setCount] = useState(0);
	return <div>Hello {props.title || "Cosm"}</div>;
}
`

	case "sql":
		defaultPath = "schema/schema.sql"
		defaultType = core.CompDatabase
		starterSrc = `-- dialect: postgresql
-- name: schema.sql

CREATE TABLE items (
    id VARCHAR(64) PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_items_name ON items (name);
`

	default:
		return "", "", "", fmt.Errorf("unsupported scaffolding language: %q (supported: go, python, typescript, sql)", lang)
	}

	return starterSrc, defaultPath, defaultType, nil
}

// ScaffoldComponent produces standard idiomatic starter AST symbol trees and a ComponentNode
// for Go, Python, TypeScript, and SQL.
func ScaffoldComponent(lang, compType, relPath string) (*core.ComponentNode, error) {
	starterSrc, defaultPath, defaultType, err := GetStarterSource(lang)
	if err != nil {
		return nil, err
	}

	targetPath := relPath
	if targetPath == "" {
		targetPath = defaultPath
	}

	// Ensure targetPath has the expected extension
	expectedExt := filepath.Ext(defaultPath)
	if filepath.Ext(targetPath) == "" {
		targetPath += expectedExt
	}

	lineage := core.LineageEnvelope{
		UserPrompt:       "cosm scaffold component",
		ExecutingAgentID: "cosm-scaffolder",
		SessionID:        "scaffold-session",
		Timestamp:        time.Now().UTC(),
	}

	parsed, err := codecs.ParseSourceFile(targetPath, []byte(starterSrc), lineage)
	if err != nil {
		return nil, fmt.Errorf("scaffolding parse failed: %w", err)
	}
	if parsed == nil || parsed.Component == nil {
		return nil, fmt.Errorf("scaffolding yielded nil component")
	}

	comp := parsed.Component
	if compType != "" {
		comp.Type = core.ComponentType(compType)
	} else if !comp.Type.IsValid() {
		comp.Type = defaultType
	}

	// Recompute Merkle hash with final attributes
	compID, err := core.HashComponentNode(comp)
	if err != nil {
		return nil, fmt.Errorf("failed to hash scaffolded component: %w", err)
	}
	comp.ComponentID = compID

	return comp, nil
}

// ScaffoldComponent creates and registers an idiomatic starter component directly within the target universe.
func (e *SurgeryEngine) ScaffoldComponent(universeID, lang, compType, relPath string, lineage core.LineageEnvelope) (*core.ComponentNode, error) {
	comp, err := ScaffoldComponent(lang, compType, relPath)
	if err != nil {
		return nil, err
	}
	if lineage.UserPrompt != "" {
		comp.Lineage = lineage
	}

	compBytes, err := json.Marshal(comp)
	if err != nil {
		return nil, fmt.Errorf("marshaling scaffolded component: %w", err)
	}
	blobHash, err := e.blobStore.Put(compBytes)
	if err != nil {
		return nil, fmt.Errorf("storing scaffolded component blob: %w", err)
	}
	comp.ComponentID = blobHash

	currManifest, err := e.universeMgr.GetUniverseManifest(universeID)
	if err != nil {
		return nil, fmt.Errorf("retrieving universe manifest: %w", err)
	}

	newComponents := append([]string{}, currManifest.Components...)
	newComponents = append(newComponents, blobHash)

	newManifest := &core.WorkspaceManifestNode{
		WorkspaceID: currManifest.WorkspaceID,
		UniverseID:  universeID,
		Components:  newComponents,
		CrossEdges:  currManifest.CrossEdges,
		Lineage:     lineage,
		CreatedAt:   time.Now().UTC(),
	}
	mBytes, _ := json.Marshal(newManifest)
	mHash, err := e.blobStore.Put(mBytes)
	if err != nil {
		return nil, fmt.Errorf("storing new manifest: %w", err)
	}
	newManifest.MerkleRootHash = mHash
	if _, err := e.universeMgr.CommitManifest(universeID, newManifest); err != nil {
		return nil, fmt.Errorf("committing universe manifest: %w", err)
	}

	return comp, nil
}

