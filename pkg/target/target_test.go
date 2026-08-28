package target

import (
	"strings"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/codecs/golang"
	"github.com/cosmscm/cosm/pkg/codecs/hcl"
	"github.com/cosmscm/cosm/pkg/codecs/typescript"
	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/shipping"
)

func TestProjector_DiffTarget(t *testing.T) {
	projector := NewProjector()
	target := shipping.DefaultLocalServiceTarget("target:auth-service", []string{"auth"})

	compAuth := &core.ComponentNode{
		ComponentID: "comp_auth_v1",
		Name:        "auth",
		Type:        core.CompService,
		Language:    core.LangGo,
	}

	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws_01",
		Components:  []string{"comp_auth_v1"},
	}

	compMap := map[string]*core.ComponentNode{
		"comp_auth_v1": compAuth,
	}

	// Case 1: Initial state (target has nothing deployed)
	diff, err := projector.DiffTarget(target, manifest, compMap, nil, nil)
	if err != nil {
		t.Fatalf("DiffTarget failed: %v", err)
	}
	if diff.UpToDate {
		t.Fatalf("expected diff not up to date, got up to date")
	}
	if len(diff.Drifts) == 0 {
		t.Fatalf("expected drifts, got 0")
	}

	// Case 2: Target is up-to-date
	deployedState := projector.ComputeTargetState(target, manifest, compMap, nil)
	diff2, err := projector.DiffTarget(target, manifest, compMap, nil, deployedState)
	if err != nil {
		t.Fatalf("DiffTarget up-to-date failed: %v", err)
	}
	if !diff2.UpToDate {
		t.Fatalf("expected up-to-date diff, got drifts: %+v", diff2.Drifts)
	}
}

func TestTopologyVisualizer_AsciiAndMermaid(t *testing.T) {
	vis := NewTopologyVisualizer()
	lineage := core.LineageEnvelope{UserPrompt: "Topology test", Timestamp: time.Now().UTC()}

	// 1. Go Route
	goParser := golang.NewGoParser()
	goRes, _ := goParser.ParseSource("server/main.go", []byte("package main\nfunc HandleUsers(w http.ResponseWriter, r *http.Request) {}"), lineage)
	compGo, _ := golang.BuildComponentNode("api-server", core.CompService, goRes, lineage)

	// 2. TypeScript Fetch
	tsParser := typescript.NewTSParser()
	tsRes, _ := tsParser.ParseSource("src/UserView.tsx", []byte("export const UserView = () => { fetch('/api/users'); return <div></div>; };"), lineage)
	compTS, _, _ := tsParser.BuildComponentNode(tsRes, "frontend", lineage)

	// 3. HCL Cloud Run
	hclParser := hcl.NewHCLParser()
	hclDoc, _ := hclParser.ParseSource("infra/main.tf", []byte("resource \"google_cloud_run_service\" \"api\" { name = \"api-service\" }"))
	compHCL, hclSyms, _ := hclParser.BuildComponentNode(hclDoc, "infra", lineage)

	symbolMap := make(map[string]*core.ASTSymbolNode)
	for _, s := range goRes.AllSymbols {
		symbolMap[s.NodeID] = s
	}
	for _, s := range tsRes.AllSymbols {
		symbolMap[s.NodeID] = s
	}
	for _, s := range hclSyms {
		symbolMap[s.NodeID] = s
	}

	compMap := map[string]*core.ComponentNode{
		compGo.ComponentID:  compGo,
		compTS.ComponentID:  compTS,
		compHCL.ComponentID: compHCL,
	}

	// Link cross boundaries
	linker := core.NewCrossBoundaryLinker()
	edges, _ := linker.LinkWorkspace([]*core.ComponentNode{compGo, compTS, compHCL}, symbolMap)

	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws_topology",
		Components:  []string{compGo.ComponentID, compTS.ComponentID, compHCL.ComponentID},
		CrossEdges:  edges,
	}

	graph := vis.BuildTopology(manifest, compMap, symbolMap)
	if graph.TotalNodes == 0 {
		t.Fatalf("expected topology nodes > 0, got %d", graph.TotalNodes)
	}

	ascii := vis.RenderASCII(graph)
	if !strings.Contains(ascii, "TOPOLOGY MAP") {
		t.Fatalf("ASCII rendering missing header: %s", ascii)
	}

	mermaid := vis.RenderMermaid(graph)
	if !strings.Contains(mermaid, "flowchart TD") {
		t.Fatalf("Mermaid rendering missing flowchart: %s", mermaid)
	}
}

func TestPreviewSandbox_LifecycleAndHealthCheck(t *testing.T) {
	sandbox := NewPreviewSandbox()
	defer sandbox.StopAll()

	target := shipping.DefaultLocalServiceTarget("target:test-preview", nil)
	files := map[string][]byte{
		"index.html": []byte("<h1>Preview Running</h1>"),
	}

	inst, err := sandbox.Start(target, files)
	if err != nil {
		t.Fatalf("sandbox Start failed: %v", err)
	}

	if inst.Port == 0 || inst.URL == "" {
		t.Fatalf("invalid instance port/url: %d / %s", inst.Port, inst.URL)
	}

	// Wait for server to be responsive
	time.Sleep(50 * time.Millisecond)

	if !inst.HealthCheck() {
		t.Fatalf("health check failed for preview instance")
	}

	if err := inst.Stop(); err != nil {
		t.Fatalf("instance Stop failed: %v", err)
	}
}
