package tui

import (
	"strings"
	"testing"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/lineage"
	"github.com/cosmscm/cosm/pkg/target"
)

func TestTUI_AppModelRender(t *testing.T) {
	top := &target.FullTopologyGraph{
		FrontendNodes: []*target.TopologyNode{
			{ID: "fe-1", Name: "AppHeader", Tier: "Frontend", Language: core.LangTypeScript, Type: "ReactComponent"},
		},
		BackendNodes: []*target.TopologyNode{
			{ID: "be-1", Name: "HandleGetUser", Tier: "API/Backend", Language: core.LangGo, Type: "RouteBinding"},
		},
		InfraNodes: []*target.TopologyNode{
			{ID: "inf-1", Name: "cloud_run_service", Tier: "Cloud Infra", Language: core.LangHCL, Type: "ResourceBlock"},
		},
		TotalNodes: 3,
		TotalEdges: 2,
	}

	app := NewAppModel("universe-test", top)
	view := app.RenderView()
	if !strings.Contains(view, "COSM") {
		t.Errorf("Expected view to contain COSM header, got: %s", view)
	}
	if !strings.Contains(view, "universe-test") {
		t.Errorf("Expected view to contain active universe ID")
	}

	// Test Lineage tab
	app.ActiveTab = 1
	app.AncestryChain = &lineage.AncestryChain{
		TargetNodeID:      "be-1",
		RootPrompt:        "create user API",
		PrimaryExecutorID: "coder-agent-1",
		TotalHops:         2,
		IntactChain:       true,
	}
	lineageView := app.RenderView()
	if !strings.Contains(lineageView, "create user API") {
		t.Errorf("Expected lineage view to contain root prompt")
	}

	// Test Universes tab
	app.ActiveTab = 2
	univView := app.RenderView()
	if !strings.Contains(univView, "Active Micro-Universes") {
		t.Errorf("Expected universes view to list universes")
	}
}
