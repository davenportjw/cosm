package dockerfile

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// DockerfileHydrator reconstitutes Dockerfile AST symbol nodes back into clean Containerfile syntax.
type DockerfileHydrator struct{}

// NewDockerfileHydrator creates a new DockerfileHydrator instance.
func NewDockerfileHydrator() *DockerfileHydrator {
	return &DockerfileHydrator{}
}

// HydrateSymbol reconstitutes a single ASTSymbolNode into Dockerfile text.
func (h *DockerfileHydrator) HydrateSymbol(node *core.ASTSymbolNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node cannot be nil")
	}

	if len(node.ASTPayload) == 0 {
		if node.Signature != "" {
			return node.Signature + "\n", nil
		}
		return fmt.Sprintf("# Instruction: %s\n", node.Identifier), nil
	}

	var inst DockerInstruction
	if err := json.Unmarshal(node.ASTPayload, &inst); err != nil {
		if node.Signature != "" {
			return node.Signature + "\n", nil
		}
		return "", fmt.Errorf("failed to unmarshal DockerInstruction: %w", err)
	}

	if inst.RawLine != "" {
		return inst.RawLine + "\n", nil
	}

	return fmt.Sprintf("%s %s\n", inst.Directive, inst.Arguments), nil
}

// HydrateModule reconstitutes a collection of Dockerfile symbols into a complete Dockerfile document.
func (h *DockerfileHydrator) HydrateModule(symbols []*core.ASTSymbolNode) (string, error) {
	var sb strings.Builder
	sb.WriteString("# Reconstituted Containerfile / Dockerfile\n\n")

	for _, sym := range symbols {
		code, err := h.HydrateSymbol(sym)
		if err != nil {
			return "", err
		}
		sb.WriteString(code)
	}

	return sb.String(), nil
}
