package mutation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

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
)

// IsValid checks if the operation type is supported.
func (op ASTOperationType) IsValid() bool {
	switch op {
	case OpReplaceFunctionBody, OpReplaceFunction, OpAddMethod,
		OpAddBefore, OpAddAfter, OpDelete, OpAddImport,
		OpReplaceImports, OpReplaceGlobal:
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

// BatchMutationResult returns the Merkle tree delta and audit trail for an applied batch.
type BatchMutationResult struct {
	UniverseID          string        `json:"universe_id"`
	OldManifestHash     string        `json:"old_manifest_hash"`
	NewManifestHash     string        `json:"new_manifest_hash"`
	AppliedOperations   int           `json:"applied_operations"`
	ModifiedSymbols     []string      `json:"modified_symbols"`
	AddedSymbols        []string      `json:"added_symbols"`
	DeletedSymbols      []string      `json:"deleted_symbols"`
	DeduplicatedSymbols int           `json:"deduplicated_symbols"`
	Duration            time.Duration `json:"duration"`
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
			if comp.Name != scopedComp && comp.Metadata["dir_path"] != scopedComp && comp.Metadata["file_path"] != scopedComp {
				if !strings.Contains(comp.Name, scopedComp) {
					continue
				}
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
				candidates = append(candidates, resolved)
				continue
			}

			// 5. Case-insensitive identifier match
			if strings.EqualFold(sym.Identifier, scopedSym) {
				candidates = append(candidates, resolved)
				continue
			}
		}
	}

	if len(candidates) == 1 {
		return candidates[0], nil
	} else if len(candidates) > 1 {
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

	return nil, fmt.Errorf("symbol %q not found in universe %s", target, universeID)
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
		return nil, fmt.Errorf("active manifest not found for universe %s: %w", batch.UniverseID, err)
	}
	oldManifestHash := manifest.MerkleRootHash

	var modifiedSymbols []string
	var addedSymbols []string
	var deletedSymbols []string
	dedupCount := 0

	for opIdx, op := range batch.Operations {
		if !op.Operation.IsValid() {
			return nil, fmt.Errorf("operation #%d has invalid type: %s", opIdx, op.Operation)
		}

		switch op.Operation {
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
			for _, sID := range resolved.Component.SymbolNodes {
				if sID != resolved.SymbolNode.NodeID {
					newSymList = append(newSymList, sID)
				}
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
				}
			}
		}
	}

	finalManifest, err := e.universeMgr.GetUniverseManifest(batch.UniverseID)
	if err != nil || finalManifest == nil {
		return nil, fmt.Errorf("getting final manifest: %w", err)
	}

	return &BatchMutationResult{
		UniverseID:          batch.UniverseID,
		OldManifestHash:     oldManifestHash,
		NewManifestHash:     finalManifest.MerkleRootHash,
		AppliedOperations:   len(batch.Operations),
		ModifiedSymbols:     modifiedSymbols,
		AddedSymbols:        addedSymbols,
		DeletedSymbols:      deletedSymbols,
		DeduplicatedSymbols: dedupCount,
		Duration:            time.Since(start),
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
