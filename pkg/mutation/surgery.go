package mutation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cosmscm/cosm/pkg/codecs/cpp"
	"github.com/cosmscm/cosm/pkg/codecs/golang"
	"github.com/cosmscm/cosm/pkg/codecs/hcl"
	"github.com/cosmscm/cosm/pkg/codecs/java"
	"github.com/cosmscm/cosm/pkg/codecs/protobuf"
	"github.com/cosmscm/cosm/pkg/codecs/python"
	"github.com/cosmscm/cosm/pkg/codecs/rust"
	"github.com/cosmscm/cosm/pkg/codecs/sql"
	"github.com/cosmscm/cosm/pkg/codecs/typescript"
	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

// MutationResult contains details of an AST surgical edit.
type MutationResult struct {
	UniverseID               string        `json:"universe_id"`
	OldSymbolID              string        `json:"old_symbol_id"`
	NewSymbolID              string        `json:"new_symbol_id"`
	AffectedComponentID      string        `json:"affected_component_id"`
	NewComponentID           string        `json:"new_component_id"`
	OldManifestHash          string        `json:"old_manifest_hash"`
	NewManifestHash          string        `json:"new_manifest_hash"`
	DeduplicatedSymbolsCount int           `json:"deduplicated_symbols_count"`
	Duration                 time.Duration `json:"duration"`
}

// SurgeryEngine coordinates fine-grained AST symbol mutations and Merkle tree cascades.
type SurgeryEngine struct {
	blobStore   *storage.BlobStore
	graphEngine *storage.GraphEngine
	universeMgr *storage.UniverseManager
}

// NewSurgeryEngine creates a new SurgeryEngine instance.
func NewSurgeryEngine(
	blobStore *storage.BlobStore,
	graphEngine *storage.GraphEngine,
	universeMgr *storage.UniverseManager,
) *SurgeryEngine {
	return &SurgeryEngine{
		blobStore:   blobStore,
		graphEngine: graphEngine,
		universeMgr: universeMgr,
	}
}

// MutateSymbol performs surgical replacement of a single AST symbol's payload, producing a new Merkle root while deduplicating unchanged siblings.
func (e *SurgeryEngine) MutateSymbol(
	universeID string,
	targetSymbolID string,
	newPayload []byte,
	lineage core.LineageEnvelope,
) (*MutationResult, error) {
	start := time.Now()

	// 1. Get current Universe manifest
	manifest, err := e.universeMgr.GetUniverseManifest(universeID)
	if err != nil || manifest == nil {
		return nil, fmt.Errorf("active manifest not found for universe %s: %w", universeID, err)
	}
	oldManifestHash := manifest.MerkleRootHash

	// 2. Locate target symbol in BlobStore
	var oldSymData []byte
	if nodeRec, nErr := e.graphEngine.GetNode(targetSymbolID); nErr == nil && nodeRec != nil && nodeRec.MerkleHash != "" {
		oldSymData, err = e.blobStore.Get(nodeRec.MerkleHash)
	} else {
		oldSymData, err = e.blobStore.Get(targetSymbolID)
	}
	var oldSym core.ASTSymbolNode
	if err == nil {
		_ = json.Unmarshal(oldSymData, &oldSym)
	}

	// 3. Construct new ASTSymbolNode
	newSym := &core.ASTSymbolNode{
		Language:          oldSym.Language,
		NodeType:          oldSym.NodeType,
		Identifier:        oldSym.Identifier,
		Signature:         oldSym.Signature,
		Docstring:         oldSym.Docstring,
		Visibility:        oldSym.Visibility,
		ASTPayload:        newPayload,
		ASTMetadata:       oldSym.ASTMetadata,
		LocalDependencies: oldSym.LocalDependencies,
		Dependencies:      oldSym.Dependencies,
		Lineage:           lineage,
	}
	if newSym.Language == "" {
		newSym.Language = core.LangGo
	}
	if newSym.NodeType == "" {
		newSym.NodeType = "FunctionDecl"
	}

	newSymBytes, err := json.Marshal(newSym)
	if err != nil {
		return nil, fmt.Errorf("marshaling mutated symbol: %w", err)
	}
	blobHash, err := e.blobStore.Put(newSymBytes)
	if err != nil {
		return nil, fmt.Errorf("storing mutated symbol blob: %w", err)
	}
	newSymHash := blobHash
	newSym.NodeID = newSymHash

	_ = e.graphEngine.PutNode(storage.NodeRecord{
		NodeID:     newSymHash,
		Language:   newSym.Language,
		NodeType:   newSym.NodeType,
		MerkleHash: blobHash,
		CreatedAt:  time.Now().UTC(),
	})
	_ = e.graphEngine.PutLineage(storage.LineageRecord{
		RecordID:         fmt.Sprintf("lin-%s", newSymHash),
		NodeID:           newSymHash,
		UserID:           lineage.UserID,
		UserPrompt:       lineage.UserPrompt,
		ExecutingAgentID: lineage.ExecutingAgentID,
		Intent:           lineage.Intent,
		Timestamp:        lineage.Timestamp,
	})

	// 4. Find which component contains the old symbol
	var targetComp *core.ComponentNode
	var targetCompIdx int
	var oldCompID string

	for idx, compID := range manifest.Components {
		var compBytes []byte
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

		for _, sID := range comp.SymbolNodes {
			if sID == targetSymbolID {
				targetComp = &comp
				targetCompIdx = idx
				oldCompID = compID
				break
			}
		}
		if targetComp != nil {
			break
		}
	}

	if targetComp == nil {
		return nil, fmt.Errorf("symbol %s not found in any component of universe %s", targetSymbolID, universeID)
	}

	// 5. Replace symbol in component, deduplicating unchanged siblings
	var newSymbolList []string
	dedupCount := 0
	for _, sID := range targetComp.SymbolNodes {
		if sID == targetSymbolID {
			newSymbolList = append(newSymbolList, newSymHash)
		} else {
			newSymbolList = append(newSymbolList, sID)
			dedupCount++
		}
	}

	updatedComp := &core.ComponentNode{
		Name:        targetComp.Name,
		Type:        targetComp.Type,
		Language:    targetComp.Language,
		SymbolNodes: newSymbolList,
		Metadata:    targetComp.Metadata,
		Lineage:     lineage,
	}

	newCompBytes, err := json.Marshal(updatedComp)
	if err != nil {
		return nil, fmt.Errorf("marshaling updated component: %w", err)
	}
	newCompHash, err := e.blobStore.Put(newCompBytes)
	if err != nil {
		return nil, fmt.Errorf("storing updated component blob: %w", err)
	}
	updatedComp.ComponentID = newCompHash

	_ = e.graphEngine.PutNode(storage.NodeRecord{
		NodeID:     newCompHash,
		Language:   updatedComp.Language,
		NodeType:   string(updatedComp.Type),
		MerkleHash: newCompHash,
		CreatedAt:  time.Now().UTC(),
	})

	// 6. Update manifest components list
	newComponents := make([]string, len(manifest.Components))
	copy(newComponents, manifest.Components)
	newComponents[targetCompIdx] = newCompHash

	newManifest := &core.WorkspaceManifestNode{
		WorkspaceID: manifest.WorkspaceID,
		UniverseID:  universeID,
		Components:  newComponents,
		CrossEdges:  manifest.CrossEdges,
		Lineage:     lineage,
		CreatedAt:   time.Now().UTC(),
	}

	newManifestBytes, err := json.Marshal(newManifest)
	if err != nil {
		return nil, fmt.Errorf("marshaling new manifest: %w", err)
	}
	newManifestHash, err := e.blobStore.Put(newManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("storing new manifest blob: %w", err)
	}
	newManifest.MerkleRootHash = newManifestHash

	_, err = e.universeMgr.CommitManifest(universeID, newManifest)
	if err != nil {
		return nil, fmt.Errorf("committing new manifest to universe: %w", err)
	}

	return &MutationResult{
		UniverseID:               universeID,
		OldSymbolID:              targetSymbolID,
		NewSymbolID:              newSymHash,
		AffectedComponentID:      oldCompID,
		NewComponentID:           newCompHash,
		OldManifestHash:          oldManifestHash,
		NewManifestHash:          newManifestHash,
		DeduplicatedSymbolsCount: dedupCount,
		Duration:                 time.Since(start),
	}, nil
}

// MutateSymbolFromCode parses raw code in the symbol's native language, performs surgical replacement, and cascades Merkle roots.
func (e *SurgeryEngine) MutateSymbolFromCode(
	universeID string,
	targetSymbolID string,
	newCode string,
	lineage core.LineageEnvelope,
) (*MutationResult, error) {
	oldSymData, err := e.blobStore.Get(targetSymbolID)
	if err != nil {
		return nil, fmt.Errorf("target symbol %s not found: %w", targetSymbolID, err)
	}
	var oldSym core.ASTSymbolNode
	if err := json.Unmarshal(oldSymData, &oldSym); err != nil {
		return nil, fmt.Errorf("unmarshaling target symbol: %w", err)
	}

	parsedSym, err := e.parseCodeToSymbol(oldSym.Language, oldSym.Identifier, newCode, lineage)
	if err != nil {
		// Fallback: use raw bytes as payload
		return e.MutateSymbol(universeID, targetSymbolID, []byte(newCode), lineage)
	}

	return e.MutateSymbol(universeID, targetSymbolID, parsedSym.ASTPayload, lineage)
}

func (e *SurgeryEngine) parseCodeToSymbol(lang core.Language, ident string, code string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	codeBytes := []byte(code)

	switch lang {
	case core.LangGo:
		p := golang.NewGoParser()
		res, err := p.ParseSource("symbol.go", codeBytes, lineage)
		if err == nil && len(res.AllSymbols) > 0 {
			return res.AllSymbols[0], nil
		}
	case core.LangRust:
		p := rust.NewRustParser()
		res, err := p.ParseSource("symbol.rs", codeBytes, lineage)
		if err == nil && len(res.AllSymbols) > 0 {
			return res.AllSymbols[0], nil
		}
	case core.LangJava:
		p := java.NewJavaParser()
		res, err := p.ParseSource("Symbol.java", codeBytes, lineage)
		if err == nil && len(res.AllSymbols) > 0 {
			return res.AllSymbols[0], nil
		}
	case core.LangSQL:
		p := sql.NewSQLParser()
		res, err := p.ParseSource("schema.sql", codeBytes, lineage)
		if err == nil && len(res.AllSymbols) > 0 {
			return res.AllSymbols[0], nil
		}
	case core.LangProtobuf:
		p := protobuf.NewProtobufParser()
		res, err := p.ParseSource("service.proto", codeBytes, lineage)
		if err == nil && len(res.AllSymbols) > 0 {
			return res.AllSymbols[0], nil
		}
	case core.LangCpp, core.LangC:
		p := cpp.NewCppParser()
		res, err := p.ParseSource("symbol.cpp", codeBytes, lineage)
		if err == nil && len(res.AllSymbols) > 0 {
			return res.AllSymbols[0], nil
		}
	case core.LangTypeScript:
		p := typescript.NewTSParser()
		res, err := p.ParseSource("symbol.ts", codeBytes, lineage)
		if err == nil && len(res.AllSymbols) > 0 {
			return res.AllSymbols[0], nil
		}
	case core.LangPython:
		p := python.NewPythonParser()
		res, err := p.ParseSource("symbol.py", codeBytes, lineage)
		if err == nil && len(res.AllSymbols) > 0 {
			return res.AllSymbols[0], nil
		}
	case core.LangHCL:
		p := hcl.NewHCLParser()
		doc, err := p.ParseSource("main.tf", codeBytes)
		if err == nil {
			syms, err := p.ToASTSymbolNodes(doc, lineage)
			if err == nil && len(syms) > 0 {
				return syms[0], nil
			}
		}
	}

	return nil, fmt.Errorf("unable to parse symbol for language %s", lang)
}

func computeSymbolHash(sym *core.ASTSymbolNode) string {
	h := sha256.New()
	h.Write([]byte(sym.Language))
	h.Write([]byte(sym.NodeType))
	h.Write([]byte(sym.Identifier))
	h.Write(sym.ASTPayload)
	return hex.EncodeToString(h.Sum(nil))
}
