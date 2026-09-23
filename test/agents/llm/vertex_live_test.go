package llm_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/cosmscm/cosm/test/agents/llm"
)

func TestVertexAI_LiveGemini38Flash(t *testing.T) {
	out, err := exec.Command("gcloud", "auth", "application-default", "print-access-token").Output()
	if err != nil || len(out) == 0 {
		out, err = exec.Command("gcloud", "auth", "print-access-token").Output()
	}
	if err != nil || len(out) == 0 {
		t.Skip("skipping live Vertex AI test: gcloud auth not available")
	}

	provider, err := llm.NewGeminiProvider(llm.GeminiConfig{
		ProjectID: "davenport-boutique",
		Location:  "us",
		Model:     "gemini-3.8-flash",
	})
	if err != nil {
		t.Fatalf("failed creating GeminiProvider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := provider.Generate(ctx, []llm.Message{
		{Role: llm.RoleUser, Content: "Respond with the single word 'PONG' and nothing else."},
	}, nil, llm.GenerateOptions{})
	if err != nil {
		t.Fatalf("live Gemini 3.8 Flash generation failed: %v", err)
	}

	t.Logf("Live Gemini 3.8 Flash response: %q (tokens used: %d, model: %s)", resp.Content, resp.Usage.TotalTokens, resp.Model)
	if !strings.Contains(strings.ToUpper(resp.Content), "PONG") {
		t.Errorf("expected response to contain PONG, got %q", resp.Content)
	}
}
