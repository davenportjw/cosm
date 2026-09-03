package dockerfile

import (
	"strings"
	"testing"

	"github.com/cosmscm/cosm/pkg/core"
)

func TestDockerfileParserAndHydratorRoundtrip(t *testing.T) {
	src := `FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o server ./cmd/server

FROM alpine:3.19
WORKDIR /root/
COPY --from=builder /app/server .
ENV PORT=8080
ENV APP_ENV=production
EXPOSE 8080/tcp
ENTRYPOINT ["./server"]
`

	parser := NewDockerfileParser()
	env := core.LineageEnvelope{Intent: "Test container build"}
	res, err := parser.ParseSource("Dockerfile", []byte(src), env)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	if len(res.Symbols) == 0 {
		t.Fatalf("expected extracted symbols, got 0")
	}

	if len(res.ExposedPorts) != 1 || res.ExposedPorts[0] != "8080" {
		t.Errorf("expected exposed port 8080, got %v", res.ExposedPorts)
	}

	if res.EnvVariables["PORT"] != "8080" || res.EnvVariables["APP_ENV"] != "production" {
		t.Errorf("expected ENV PORT=8080 and APP_ENV=production, got %v", res.EnvVariables)
	}

	hydrator := NewDockerfileHydrator()
	hydrated, err := hydrator.HydrateModule(res.Symbols)
	if err != nil {
		t.Fatalf("HydrateModule failed: %v", err)
	}

	if !strings.Contains(hydrated, "FROM golang:1.22-alpine AS builder") {
		t.Errorf("expected builder stage in hydrated text, got:\n%s", hydrated)
	}
	if !strings.Contains(hydrated, "EXPOSE 8080/tcp") {
		t.Errorf("expected EXPOSE in hydrated text, got:\n%s", hydrated)
	}

	// Re-parse hydrated output to verify AST isomorphism
	reparsed, err := parser.ParseSource("Dockerfile", []byte(hydrated), env)
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}

	if len(reparsed.ExposedPorts) != len(res.ExposedPorts) {
		t.Errorf("port count mismatch after roundtrip: %d vs %d", len(reparsed.ExposedPorts), len(res.ExposedPorts))
	}
}
