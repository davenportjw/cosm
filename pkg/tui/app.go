package tui

import (
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/lineage"
	"github.com/cosmscm/cosm/pkg/target"
)

// AppModel represents the TUI state for exploring AST graphs, universes, and lineage.
type AppModel struct {
	UniverseID     string
	ActiveTab      int // 0: Topology, 1: Lineage, 2: Universes
	Topology       *target.FullTopologyGraph
	SelectedNodeID string
	AncestryChain  *lineage.AncestryChain
	StatusMessage  string
}

// NewAppModel initializes the TUI state.
func NewAppModel(universeID string, top *target.FullTopologyGraph) *AppModel {
	return &AppModel{
		UniverseID:    universeID,
		ActiveTab:     0,
		Topology:      top,
		StatusMessage: fmt.Sprintf("Cosm (cosm) - Active Universe: %s", universeID),
	}
}

// RenderView produces the ANSI / text representation of the dashboard.
func (m *AppModel) RenderView() string {
	var b strings.Builder
	b.WriteString("================================================================================\n")
	b.WriteString(fmt.Sprintf("  🌌 COSM (cosm) | Universe: %s | Nodes: %d | Edges: %d\n",
		m.UniverseID, m.Topology.TotalNodes, m.Topology.TotalEdges))
	b.WriteString("================================================================================\n")

	tabs := []string{"[ 1: Cross-Domain Topology ]", "[ 2: Multi-Tier Lineage ]", "[ 3: Micro-Universes ]"}
	for i, tab := range tabs {
		if i == m.ActiveTab {
			b.WriteString(fmt.Sprintf(" >> %s << ", tab))
		} else {
			b.WriteString(fmt.Sprintf("    %s    ", tab))
		}
	}
	b.WriteString("\n--------------------------------------------------------------------------------\n\n")

	switch m.ActiveTab {
	case 0:
		vis := target.NewTopologyVisualizer()
		b.WriteString(vis.RenderASCII(m.Topology))
	case 1:
		if m.AncestryChain != nil {
			b.WriteString(fmt.Sprintf("  Lineage Ancestry for %s:\n", m.AncestryChain.TargetNodeID))
			b.WriteString(fmt.Sprintf("  Root Prompt: %s\n", m.AncestryChain.RootPrompt))
			b.WriteString(fmt.Sprintf("  Executing Agent: %s\n", m.AncestryChain.PrimaryExecutorID))
			b.WriteString(fmt.Sprintf("  Total Hops: %d (Intact: %v)\n", m.AncestryChain.TotalHops, m.AncestryChain.IntactChain))
		} else {
			b.WriteString("  No symbol node selected for lineage inspection.\n")
			b.WriteString("  Use 'cosm lineage <symbol_id>' or select a node in topology.\n")
		}
	case 2:
		b.WriteString("  Active Micro-Universes:\n")
		b.WriteString(fmt.Sprintf("  * %s (HEAD, active)\n", m.UniverseID))
		b.WriteString("    └── universe-feature-mcts-1 (fitness: 0.94, candidate)\n")
		b.WriteString("    └── universe-infra-preview  (fitness: 0.88, candidate)\n")
	}

	b.WriteString("\n--------------------------------------------------------------------------------\n")
	b.WriteString("  [Tab/1-3] Switch View | [q] Quit | [s] Ship Target | [v] View Source\n")
	return b.String()
}
