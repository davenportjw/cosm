package target

import (
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// TopologyNode represents a node in the rendered multi-tier topology graph.
type TopologyNode struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Tier          string         `json:"tier"`     // "Frontend", "API/Backend", "Cloud Infra"
	Language      core.Language  `json:"language"` // typescript, go, hcl, etc.
	Type          string         `json:"type"`     // ReactComponent, RouteBinding, ResourceBlock
	FilePath      string         `json:"file_path,omitempty"`
	ComponentName string         `json:"component_name,omitempty"`
	Outgoing      []TopologyEdge `json:"outgoing"`
}

// TopologyEdge represents a semantic cross-domain link between nodes.
type TopologyEdge struct {
	TargetID string        `json:"target_id"`
	EdgeType core.EdgeType `json:"edge_type"` // CONSUMES_API, DEPLOYS_TO, BINDS_ENV
	Label    string        `json:"label"`
}

// FullTopologyGraph holds the entire cross-domain architecture graph.
type FullTopologyGraph struct {
	FrontendNodes []*TopologyNode `json:"frontend_nodes"`
	BackendNodes  []*TopologyNode `json:"backend_nodes"`
	InfraNodes    []*TopologyNode `json:"infra_nodes"`
	TotalNodes    int             `json:"total_nodes"`
	TotalEdges    int             `json:"total_edges"`
}

// TopologyVisualizer builds and formats cross-domain system topology views.
type TopologyVisualizer struct{}

// NewTopologyVisualizer creates a TopologyVisualizer instance.
func NewTopologyVisualizer() *TopologyVisualizer {
	return &TopologyVisualizer{}
}

// BuildTopology constructs a FullTopologyGraph from a workspace manifest and components.
func (v *TopologyVisualizer) BuildTopology(
	manifest *core.WorkspaceManifestNode,
	compMap map[string]*core.ComponentNode,
	symbolMap map[string]*core.ASTSymbolNode,
) *FullTopologyGraph {
	graph := &FullTopologyGraph{}
	nodeLookup := make(map[string]*TopologyNode)
	seenComponents := make(map[string]bool)
	seenSymbols := make(map[string]bool)

	// 1. Group symbols into tiers (process in reverse to pick newest version of each component)
	for i := len(manifest.Components) - 1; i >= 0; i-- {
		compID := manifest.Components[i]
		comp, ok := compMap[compID]
		if !ok {
			continue
		}
		if seenComponents[comp.Name] {
			continue
		}
		seenComponents[comp.Name] = true

		filePath := comp.Name
		if comp.Metadata != nil && comp.Metadata["file_path"] != "" {
			filePath = comp.Metadata["file_path"]
		}

		for _, sID := range comp.SymbolNodes {
			if seenSymbols[sID] {
				continue
			}
			sym, ok := symbolMap[sID]
			if !ok {
				continue
			}
			seenSymbols[sID] = true

			tier := "Backend"
			lowerPath := strings.ToLower(filePath)
			lowerIdent := strings.ToLower(sym.Identifier)

			isFrontend := sym.Language == core.LangTypeScript ||
				string(sym.Language) == "javascript" ||
				string(sym.Language) == "html" ||
				string(sym.Language) == "css" ||
				string(sym.Language) == "vue" ||
				string(sym.Language) == "svelte" ||
				strings.Contains(lowerPath, "web") ||
				strings.Contains(lowerPath, "frontend") ||
				strings.Contains(lowerPath, "static") ||
				strings.Contains(lowerPath, "templates") ||
				strings.Contains(lowerIdent, "template") ||
				strings.Contains(sym.Identifier, "HX-")

			isInfra := sym.Language == core.LangHCL ||
				sym.Language == core.LangDockerfile ||
				string(sym.Language) == "terraform" ||
				sym.NodeType == "ResourceBlock" ||
				sym.NodeType == "HCLBlock" ||
				sym.NodeType == "VariableBlock" ||
				sym.NodeType == "OutputBlock" ||
				sym.NodeType == "ProviderBlock" ||
				sym.NodeType == "BaseImage" ||
				sym.NodeType == "CopyInstruction" ||
				sym.NodeType == "WorkdirInstruction" ||
				sym.NodeType == "PortExpose" ||
				sym.NodeType == "EntryPoint" ||
				sym.NodeType == "EnvBinding"

			if isFrontend {
				tier = "Frontend"
			} else if isInfra {
				tier = "Cloud Infra"
			} else if sym.Language == core.LangGo || sym.Language == core.LangPython || sym.Language == core.LangRust || sym.Language == core.LangSQL {
				tier = "API/Backend"
			}

			symFilePath := filePath
			if sym.ASTMetadata != nil && sym.ASTMetadata["file_path"] != "" {
				symFilePath = sym.ASTMetadata["file_path"]
			}

			tNode := &TopologyNode{
				ID:            sym.NodeID,
				Name:          sym.Identifier,
				Tier:          tier,
				Language:      sym.Language,
				Type:          sym.NodeType,
				FilePath:      symFilePath,
				ComponentName: comp.Name,
			}
			nodeLookup[sym.NodeID] = tNode

			switch tier {
			case "Frontend":
				graph.FrontendNodes = append(graph.FrontendNodes, tNode)
			case "API/Backend", "Backend":
				graph.BackendNodes = append(graph.BackendNodes, tNode)
			case "Cloud Infra":
				graph.InfraNodes = append(graph.InfraNodes, tNode)
			}
		}
	}

	// 2. Attach cross-boundary edges
	for _, edge := range manifest.CrossEdges {
		srcNode, srcOk := nodeLookup[edge.SourceNodeID]
		_, tgtOk := nodeLookup[edge.TargetNodeID]

		label := string(edge.Type)
		if ep, ok := edge.Metadata["backend_endpoint"]; ok {
			label = fmt.Sprintf("CALLS %s", ep)
		} else if env, ok := edge.Metadata["env_var"]; ok {
			label = fmt.Sprintf("BINDS %s", env)
		} else if res, ok := edge.Metadata["resource_id"]; ok {
			label = fmt.Sprintf("DEPLOYS TO %s", res)
		}

		if srcOk && tgtOk {
			srcNode.Outgoing = append(srcNode.Outgoing, TopologyEdge{
				TargetID: edge.TargetNodeID,
				EdgeType: edge.Type,
				Label:    label,
			})
			graph.TotalEdges++
		}
	}

	graph.TotalNodes = len(graph.FrontendNodes) + len(graph.BackendNodes) + len(graph.InfraNodes)
	return graph
}

// RenderASCII produces a clean, readable ASCII topology graph.
func (v *TopologyVisualizer) RenderASCII(graph *FullTopologyGraph) string {
	var sb strings.Builder

	sb.WriteString("================================================================================\n")
	sb.WriteString("                            COSM: TOPOLOGY MAP                                  \n")
	sb.WriteString("================================================================================\n\n")

	// Tier 1: Frontend
	sb.WriteString("┌── [1] FRONTEND TIER (TypeScript / React)\n")
	if len(graph.FrontendNodes) == 0 {
		sb.WriteString("│   (No frontend components detected)\n")
	} else {
		for _, n := range graph.FrontendNodes {
			sb.WriteString(fmt.Sprintf("│   ├── %s (%s)\n", n.Name, n.Type))
			for _, edge := range n.Outgoing {
				sb.WriteString(fmt.Sprintf("│   │   └───[%s]───> (API Target)\n", edge.Label))
			}
		}
	}
	sb.WriteString("│\n")

	// Tier 2: Backend API
	sb.WriteString("├── [2] BACKEND / API TIER (Go Microservices)\n")
	if len(graph.BackendNodes) == 0 {
		sb.WriteString("│   (No backend services detected)\n")
	} else {
		for _, n := range graph.BackendNodes {
			sb.WriteString(fmt.Sprintf("│   ├── %s (%s)\n", n.Name, n.Type))
			for _, edge := range n.Outgoing {
				sb.WriteString(fmt.Sprintf("│   │   └───[%s]───> (Infra Target)\n", edge.Label))
			}
		}
	}
	sb.WriteString("│\n")

	// Tier 3: Cloud Infrastructure
	sb.WriteString("└── [3] CLOUD INFRASTRUCTURE TIER (Terraform HCL)\n")
	if len(graph.InfraNodes) == 0 {
		sb.WriteString("    (No infrastructure resources detected)\n")
	} else {
		for idx, n := range graph.InfraNodes {
			prefix := "├──"
			if idx == len(graph.InfraNodes)-1 {
				prefix = "└──"
			}
			sb.WriteString(fmt.Sprintf("    %s %s (%s)\n", prefix, n.Name, n.Type))
			for _, edge := range n.Outgoing {
				sb.WriteString(fmt.Sprintf("        └───[%s]───> (Binding)\n", edge.Label))
			}
		}
	}

	sb.WriteString(fmt.Sprintf("\nSummary: %d Nodes, %d Cross-Domain Edges\n", graph.TotalNodes, graph.TotalEdges))
	return sb.String()
}

// RenderMermaid generates GitHub-compatible Mermaid flow diagram markup.
func (v *TopologyVisualizer) RenderMermaid(graph *FullTopologyGraph) string {
	var sb strings.Builder
	sb.WriteString("```mermaid\nflowchart TD\n")

	// Subgraph: Frontend
	sb.WriteString("  subgraph Frontend[\"Frontend UI (TypeScript/React)\"]\n")
	for i, n := range graph.FrontendNodes {
		nodeID := fmt.Sprintf("FE_%d", i)
		cleanName := cleanLabel(n.Name)
		sb.WriteString(fmt.Sprintf("    %s[\"%s<br/>(%s)\"]\n", nodeID, cleanName, n.Type))
	}
	sb.WriteString("  end\n\n")

	// Subgraph: Backend
	sb.WriteString("  subgraph Backend[\"API Services (Go)\"]\n")
	for i, n := range graph.BackendNodes {
		nodeID := fmt.Sprintf("BE_%d", i)
		cleanName := cleanLabel(n.Name)
		sb.WriteString(fmt.Sprintf("    %s[\"%s<br/>(%s)\"]\n", nodeID, cleanName, n.Type))
	}
	sb.WriteString("  end\n\n")

	// Subgraph: Cloud Infra
	sb.WriteString("  subgraph Infra[\"Cloud Infrastructure (Terraform)\"]\n")
	for i, n := range graph.InfraNodes {
		nodeID := fmt.Sprintf("INFRA_%d", i)
		cleanName := cleanLabel(n.Name)
		sb.WriteString(fmt.Sprintf("    %s[\"%s<br/>(%s)\"]\n", nodeID, cleanName, n.Type))
	}
	sb.WriteString("  end\n\n")

	// Cross-boundary connections
	nodeIndexMap := make(map[string]string)
	for i, n := range graph.FrontendNodes {
		nodeIndexMap[n.ID] = fmt.Sprintf("FE_%d", i)
	}
	for i, n := range graph.BackendNodes {
		nodeIndexMap[n.ID] = fmt.Sprintf("BE_%d", i)
	}
	for i, n := range graph.InfraNodes {
		nodeIndexMap[n.ID] = fmt.Sprintf("INFRA_%d", i)
	}

	for _, n := range append(append(graph.FrontendNodes, graph.BackendNodes...), graph.InfraNodes...) {
		srcKey := nodeIndexMap[n.ID]
		for _, edge := range n.Outgoing {
			tgtKey, ok := nodeIndexMap[edge.TargetID]
			if ok {
				sb.WriteString(fmt.Sprintf("  %s -->|\"%s\"| %s\n", srcKey, cleanLabel(edge.Label), tgtKey))
			}
		}
	}

	sb.WriteString("```\n")
	return sb.String()
}

func cleanLabel(s string) string {
	s = strings.ReplaceAll(s, "\"", "'")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// FilterByComponent returns a subgraph focused on a single component and its directly connected edges.
func (v *TopologyVisualizer) FilterByComponent(graph *FullTopologyGraph, compName string) *FullTopologyGraph {
	filtered := &FullTopologyGraph{}
	filterLower := strings.ToLower(compName)

	matches := func(n *TopologyNode) bool {
		return strings.Contains(strings.ToLower(n.Name), filterLower)
	}

	for _, n := range graph.FrontendNodes {
		if matches(n) {
			filtered.FrontendNodes = append(filtered.FrontendNodes, n)
		}
	}
	for _, n := range graph.BackendNodes {
		if matches(n) {
			filtered.BackendNodes = append(filtered.BackendNodes, n)
		}
	}
	for _, n := range graph.InfraNodes {
		if matches(n) {
			filtered.InfraNodes = append(filtered.InfraNodes, n)
		}
	}

	filtered.TotalNodes = len(filtered.FrontendNodes) + len(filtered.BackendNodes) + len(filtered.InfraNodes)
	return filtered
}
