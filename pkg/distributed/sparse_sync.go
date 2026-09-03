package distributed

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

// SparseFilter specifies which components, languages, or contracts an agent needs to download.
type SparseFilter struct {
	ComponentNames   []string        `json:"component_names,omitempty"` // e.g. ["services/billing"]
	Languages        []core.Language `json:"languages,omitempty"`       // e.g. [LangGo, LangHCL]
	IncludeContracts bool            `json:"include_contracts"`         // Whether to pull cross-boundary contract nodes
}

// SparseSyncPayload contains only the necessary AST blobs and manifest subgraph for an agent task.
type SparseSyncPayload struct {
	Manifest        *core.WorkspaceManifestNode    `json:"manifest"`
	Components      map[string]*core.ComponentNode `json:"components"`
	Symbols         map[string]*core.ASTSymbolNode `json:"symbols"`
	FilteredBlobs   map[string][]byte              `json:"filtered_blobs"` // Hash -> Raw payload
	TotalBlobsCount int                            `json:"total_blobs_count"`
	SavingsPercent  float64                        `json:"savings_percent"`
}

// SparseSyncEngine extracts minimal content-addressed AST subgraphs for lightweight agent replication.
type SparseSyncEngine struct {
	blobStore   *storage.BlobStore
	universeMgr *storage.UniverseManager
}

// NewSparseSyncEngine creates a new SparseSyncEngine instance.
func NewSparseSyncEngine(b *storage.BlobStore, u *storage.UniverseManager) *SparseSyncEngine {
	return &SparseSyncEngine{
		blobStore:   b,
		universeMgr: u,
	}
}

// ExtractSparseSubtree queries the full repository DAG and packages only the filtered AST nodes.
func (s *SparseSyncEngine) ExtractSparseSubtree(
	universeID string,
	filter SparseFilter,
) (*SparseSyncPayload, error) {
	manifest, err := s.universeMgr.GetUniverseManifest(universeID)
	if err != nil || manifest == nil {
		return nil, fmt.Errorf("universe %s manifest not found: %w", universeID, err)
	}

	payload := &SparseSyncPayload{
		Manifest:      manifest,
		Components:    make(map[string]*core.ComponentNode),
		Symbols:       make(map[string]*core.ASTSymbolNode),
		FilteredBlobs: make(map[string][]byte),
	}

	totalPossibleBlobs := len(manifest.Components)

	// 1. Iterate over all components in manifest
	for _, compID := range manifest.Components {
		compBytes, err := s.blobStore.Get(compID)
		if err != nil {
			continue
		}
		var comp core.ComponentNode
		if err := json.Unmarshal(compBytes, &comp); err != nil {
			continue
		}

		totalPossibleBlobs += len(comp.SymbolNodes)

		// Check filter match
		matches := false
		if len(filter.ComponentNames) == 0 && len(filter.Languages) == 0 {
			matches = true
		} else {
			for _, name := range filter.ComponentNames {
				if strings.Contains(comp.Name, name) {
					matches = true
					break
				}
			}
			if !matches {
				for _, lang := range filter.Languages {
					if comp.Language == lang {
						matches = true
						break
					}
				}
			}
		}

		if matches {
			payload.Components[compID] = &comp
			payload.FilteredBlobs[compID] = compBytes

			// Collect symbols
			for _, symID := range comp.SymbolNodes {
				symBytes, err := s.blobStore.Get(symID)
				if err != nil {
					continue
				}
				var sym core.ASTSymbolNode
				if err := json.Unmarshal(symBytes, &sym); err == nil {
					payload.Symbols[symID] = &sym
					payload.FilteredBlobs[symID] = symBytes
				}
			}
		}
	}

	payload.TotalBlobsCount = len(payload.FilteredBlobs)
	if totalPossibleBlobs > 0 {
		filteredCount := len(payload.FilteredBlobs)
		if filteredCount <= totalPossibleBlobs {
			payload.SavingsPercent = (1.0 - float64(filteredCount)/float64(totalPossibleBlobs)) * 100.0
		}
	}

	return payload, nil
}

// IngestSparseSubtree imports a sparsely replicated payload into a local worker node's storage.
func (s *SparseSyncEngine) IngestSparseSubtree(payload *SparseSyncPayload) error {
	if payload == nil {
		return fmt.Errorf("payload cannot be nil")
	}

	for hash, data := range payload.FilteredBlobs {
		_, err := s.blobStore.Put(data)
		if err != nil {
			return fmt.Errorf("failed to write sparse blob %s: %w", hash, err)
		}
	}

	return nil
}
