package target

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/shipping"
)

// DriftType classifies the nature of target state divergence.
type DriftType string

const (
	DriftComponentAdded    DriftType = "COMPONENT_ADDED"
	DriftComponentRemoved  DriftType = "COMPONENT_REMOVED"
	DriftComponentModified DriftType = "COMPONENT_MODIFIED"
	DriftEnvVarMissing     DriftType = "ENV_VAR_MISSING"
	DriftEndpointChanged   DriftType = "ENDPOINT_CHANGED"
	DriftResourceDrift     DriftType = "RESOURCE_DRIFT"
)

// ComponentDrift represents a single drifted component or resource between Universe AST and Target state.
type ComponentDrift struct {
	ComponentName string    `json:"component_name"`
	Type          DriftType `json:"type"`
	Details       string    `json:"details"`
	OldHash       string    `json:"old_hash,omitempty"`
	NewHash       string    `json:"new_hash,omitempty"`
}

// TargetState represents the known deployed or materialized snapshot of a target environment.
type TargetState struct {
	TargetName      string            `json:"target_name"`
	LastDeployedAt  time.Time         `json:"last_deployed_at"`
	ComponentHashes map[string]string `json:"component_hashes"` // ComponentName -> ComponentID
	ResourceHashes  map[string]string `json:"resource_hashes"`  // ResourceIdentifier -> NodeID
	ActiveEnvVars   map[string]string `json:"active_env_vars"`  // EnvName -> Value
	ActiveEndpoints []string          `json:"active_endpoints"` // ["GET /api/v1/users", ...]
}

// ProjectionDiff captures the complete delta and pending plan between Universe AST and Target state.
type ProjectionDiff struct {
	TargetName       string           `json:"target_name"`
	UpToDate         bool             `json:"up_to_date"`
	TotalDrifts      int              `json:"total_drifts"`
	Drifts           []ComponentDrift `json:"drifts"`
	PendingActions   []string         `json:"pending_actions"`
	PlannedArtifacts []string         `json:"planned_artifacts"`
	GeneratedAt      time.Time        `json:"generated_at"`
}

// Projector computes deployment drift and migration plans by comparing Universe AST against Target state.
type Projector struct{}

// NewProjector creates a new Projector.
func NewProjector() *Projector {
	return &Projector{}
}

// DiffTarget compares a target specification and workspace manifest against an active TargetState snapshot.
func (p *Projector) DiffTarget(
	target *shipping.TargetSpec,
	manifest *core.WorkspaceManifestNode,
	compMap map[string]*core.ComponentNode,
	symbolMap map[string]*core.ASTSymbolNode,
	currentState *TargetState,
) (*ProjectionDiff, error) {
	if target == nil {
		return nil, fmt.Errorf("target spec cannot be nil")
	}

	diff := &ProjectionDiff{
		TargetName:  target.Name,
		GeneratedAt: time.Now().UTC(),
	}

	if currentState == nil {
		currentState = &TargetState{
			TargetName:      target.Name,
			ComponentHashes: make(map[string]string),
			ResourceHashes:  make(map[string]string),
			ActiveEnvVars:   make(map[string]string),
		}
	}

	// 1. Gather all components in manifest that apply to this target
	targetComps := make(map[string]*core.ComponentNode)
	targetCompNames := make(map[string]bool)
	for _, cName := range target.ComponentNames {
		targetCompNames[cName] = true
	}

	if manifest != nil {
		for _, cID := range manifest.Components {
			comp, ok := compMap[cID]
			if !ok {
				continue
			}
			// If target specifies component filter, apply it; otherwise include all matching kind
			if len(target.ComponentNames) == 0 || targetCompNames[comp.Name] {
				targetComps[comp.Name] = comp
			}
		}
	}

	// 2. Check for added/modified components
	for cName, comp := range targetComps {
		oldHash, exists := currentState.ComponentHashes[cName]
		if !exists {
			diff.Drifts = append(diff.Drifts, ComponentDrift{
				ComponentName: cName,
				Type:          DriftComponentAdded,
				Details:       fmt.Sprintf("Component '%s' (%s) is new and not yet deployed", cName, comp.Type),
				NewHash:       comp.ComponentID,
			})
			diff.PendingActions = append(diff.PendingActions, fmt.Sprintf("Build and deploy new component: %s", cName))
		} else if oldHash != comp.ComponentID {
			diff.Drifts = append(diff.Drifts, ComponentDrift{
				ComponentName: cName,
				Type:          DriftComponentModified,
				Details:       fmt.Sprintf("Component '%s' has modified AST symbols (old=%s, new=%s)", cName, short(oldHash), short(comp.ComponentID)),
				OldHash:       oldHash,
				NewHash:       comp.ComponentID,
			})
			diff.PendingActions = append(diff.PendingActions, fmt.Sprintf("Recompile and rolling update component: %s", cName))
		}
	}

	// 3. Check for removed components
	for oldName, oldHash := range currentState.ComponentHashes {
		if _, exists := targetComps[oldName]; !exists {
			diff.Drifts = append(diff.Drifts, ComponentDrift{
				ComponentName: oldName,
				Type:          DriftComponentRemoved,
				Details:       fmt.Sprintf("Component '%s' was removed from Universe manifest", oldName),
				OldHash:       oldHash,
			})
			diff.PendingActions = append(diff.PendingActions, fmt.Sprintf("Tear down / decommission removed component: %s", oldName))
		}
	}

	// 4. Check environment variables
	if len(target.Environment) > 0 {
		for k, expectedVal := range target.Environment {
			activeVal, exists := currentState.ActiveEnvVars[k]
			if !exists || activeVal != expectedVal {
				diff.Drifts = append(diff.Drifts, ComponentDrift{
					ComponentName: target.Name,
					Type:          DriftEnvVarMissing,
					Details:       fmt.Sprintf("Env var '%s' (expected='%s', active='%s') requires update", k, expectedVal, activeVal),
				})
				diff.PendingActions = append(diff.PendingActions, fmt.Sprintf("Inject environment variable: %s", k))
			}
		}
	}

	// Sort drifts deterministically
	sort.Slice(diff.Drifts, func(i, j int) bool {
		return diff.Drifts[i].ComponentName < diff.Drifts[j].ComponentName
	})

	diff.TotalDrifts = len(diff.Drifts)
	diff.UpToDate = diff.TotalDrifts == 0

	if target.ArtifactName != "" {
		diff.PlannedArtifacts = append(diff.PlannedArtifacts, target.ArtifactName)
	}

	return diff, nil
}

// ComputeTargetState creates a TargetState snapshot from a WorkspaceManifestNode.
func (p *Projector) ComputeTargetState(
	target *shipping.TargetSpec,
	manifest *core.WorkspaceManifestNode,
	compMap map[string]*core.ComponentNode,
	symbolMap map[string]*core.ASTSymbolNode,
) *TargetState {
	compHashes := make(map[string]string)
	resourceHashes := make(map[string]string)
	var activeEndpoints []string

	if manifest != nil {
		for _, cID := range manifest.Components {
			comp, ok := compMap[cID]
			if !ok {
				continue
			}
			compHashes[comp.Name] = comp.ComponentID

			for _, sID := range comp.SymbolNodes {
				sym, ok := symbolMap[sID]
				if !ok {
					continue
				}
				if sym.NodeType == "ResourceBlock" {
					resourceHashes[sym.Identifier] = sym.NodeID
				} else if sym.NodeType == "RouteBinding" {
					activeEndpoints = append(activeEndpoints, sym.Identifier)
				}
			}
		}
	}

	envCopy := make(map[string]string)
	if target != nil {
		for k, v := range target.Environment {
			envCopy[k] = v
		}
	}

	return &TargetState{
		TargetName:      target.Name,
		LastDeployedAt:  time.Now().UTC(),
		ComponentHashes: compHashes,
		ResourceHashes:  resourceHashes,
		ActiveEnvVars:   envCopy,
		ActiveEndpoints: activeEndpoints,
	}
}

func short(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

// GenerateStateFingerprint computes a deterministic SHA256 digest of a TargetState.
func GenerateStateFingerprint(state *TargetState) string {
	if state == nil {
		return ""
	}
	var keys []string
	for k, v := range state.ComponentHashes {
		keys = append(keys, fmt.Sprintf("%s:%s", k, v))
	}
	sort.Strings(keys)
	data := strings.Join(keys, ",")
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}
