package materialize

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

// RenderFormat designates the output medium for rendering.
type RenderFormat string

const (
	FormatTerminal RenderFormat = "terminal"
	FormatMarkdown RenderFormat = "markdown"
	FormatPlain    RenderFormat = "plain"
	FormatAST      RenderFormat = "ast"
	FormatCode     RenderFormat = "code"
	FormatContract RenderFormat = "contract"
)

// Terminal ANSI Color Escapes
const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiDim     = "\033[2m"
	ansiRed     = "\033[31m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiBlue    = "\033[34m"
	ansiMagenta = "\033[35m"
	ansiCyan    = "\033[36m"
	ansiGray    = "\033[90m"
	ansiBgDark  = "\033[48;5;235m"
)

// Viewer formats AST nodes, lineage provenance cards, diffs, and components.
type Viewer struct {
	hydrator *Hydrator
}

// NewViewer returns a new Viewer instance.
func NewViewer() *Viewer {
	return &Viewer{
		hydrator: NewHydrator(),
	}
}

// RenderSymbolNode formats an individual ASTSymbolNode with its metadata, lineage card, and source code.
func (v *Viewer) RenderSymbolNode(node *core.ASTSymbolNode, format RenderFormat) string {
	if node == nil {
		return "nil symbol node"
	}

	code, _ := v.hydrator.HydrateSymbol(node)

	switch format {
	case FormatCode:
		return strings.TrimSpace(code)

	case FormatAST:
		data, err := json.MarshalIndent(node, "", "  ")
		if err != nil {
			return fmt.Sprintf("error marshaling AST: %v", err)
		}
		return string(data)

	case FormatContract:
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("CONTRACT: %s\n", node.Identifier))
		sb.WriteString(fmt.Sprintf("Language:    %s\n", node.Language))
		sb.WriteString(fmt.Sprintf("Node Type:   %s\n", node.NodeType))
		if node.Signature != "" {
			sb.WriteString(fmt.Sprintf("Signature:   %s\n", node.Signature))
		}
		if node.Visibility != "" {
			sb.WriteString(fmt.Sprintf("Visibility:  %s\n", node.Visibility))
		}
		if len(node.Dependencies) > 0 {
			sb.WriteString(fmt.Sprintf("Deps:        %s\n", strings.Join(node.Dependencies, ", ")))
		}
		if len(node.ASTMetadata) > 0 {
			sb.WriteString("Metadata:\n")
			for k, val := range node.ASTMetadata {
				sb.WriteString(fmt.Sprintf("  - %s: %s\n", k, val))
			}
		}
		if node.Docstring != "" {
			sb.WriteString(fmt.Sprintf("Docstring:\n  %s\n", strings.ReplaceAll(node.Docstring, "\n", "\n  ")))
		}
		sb.WriteString("\nLineage:\n")
		sb.WriteString(v.RenderLineageCard(&node.Lineage, FormatPlain))
		return sb.String()

	case FormatMarkdown:
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("### AST Symbol: `%s`\n\n", node.Identifier))
		sb.WriteString(fmt.Sprintf("- **Node ID**: `%s`\n", node.NodeID))
		sb.WriteString(fmt.Sprintf("- **Language**: `%s`\n", node.Language))
		sb.WriteString(fmt.Sprintf("- **Node Type**: `%s`\n", node.NodeType))
		if node.Signature != "" {
			sb.WriteString(fmt.Sprintf("- **Signature**: `%s`\n", node.Signature))
		}
		if node.Visibility != "" {
			sb.WriteString(fmt.Sprintf("- **Visibility**: `%s`\n", node.Visibility))
		}
		if len(node.LocalDependencies) > 0 {
			sb.WriteString(fmt.Sprintf("- **Dependencies**: `%s`\n", strings.Join(node.LocalDependencies, "`, `")))
		}
		sb.WriteString("\n")
		sb.WriteString(v.RenderLineageCard(&node.Lineage, FormatMarkdown))
		sb.WriteString("\n\n```" + string(node.Language) + "\n")
		sb.WriteString(strings.TrimSpace(code))
		sb.WriteString("\n```\n")
		return sb.String()

	case FormatTerminal:
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%s%s┌── AST SYMBOL: %s%s\n", ansiBold, ansiCyan, node.Identifier, ansiReset))
		sb.WriteString(fmt.Sprintf("%s│%s Node ID:      %s%s%s\n", ansiCyan, ansiReset, ansiYellow, node.NodeID, ansiReset))
		sb.WriteString(fmt.Sprintf("%s│%s Language:     %s%s%s\n", ansiCyan, ansiReset, ansiGreen, node.Language, ansiReset))
		sb.WriteString(fmt.Sprintf("%s│%s Type:         %s%s%s\n", ansiCyan, ansiReset, ansiMagenta, node.NodeType, ansiReset))
		if node.Signature != "" {
			sb.WriteString(fmt.Sprintf("%s│%s Signature:    %s%s%s\n", ansiCyan, ansiReset, ansiBold, node.Signature, ansiReset))
		}
		if len(node.LocalDependencies) > 0 {
			sb.WriteString(fmt.Sprintf("%s│%s Dependencies: %s\n", ansiCyan, ansiReset, strings.Join(node.LocalDependencies, ", ")))
		}
		sb.WriteString(fmt.Sprintf("%s├── PROVENANCE PEDIGREE%s\n", ansiCyan, ansiReset))
		sb.WriteString(v.indentLines(v.RenderLineageCard(&node.Lineage, FormatTerminal), ansiCyan+"│ "+ansiReset))
		sb.WriteString(fmt.Sprintf("%s├── RECONSTITUTED SOURCE%s\n", ansiCyan, ansiReset))
		sb.WriteString(v.highlightCode(code, node.Language))
		sb.WriteString(fmt.Sprintf("%s└── END SYMBOL%s\n", ansiCyan, ansiReset))
		return sb.String()

	default: // Plain
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("=== Symbol: %s (%s) ===\n", node.Identifier, node.NodeType))
		sb.WriteString(fmt.Sprintf("Node ID: %s | Language: %s\n", node.NodeID, node.Language))
		if node.Signature != "" {
			sb.WriteString(fmt.Sprintf("Signature: %s\n", node.Signature))
		}
		if len(node.LocalDependencies) > 0 {
			sb.WriteString(fmt.Sprintf("Dependencies: %s\n", strings.Join(node.LocalDependencies, ", ")))
		}
		sb.WriteString("Provenance:\n")
		sb.WriteString(v.RenderLineageCard(&node.Lineage, FormatPlain))
		sb.WriteString("\nSource:\n")
		sb.WriteString(code)
		sb.WriteString("\n")
		return sb.String()
	}
}

// RenderLineageCard renders the causal provenance card for an envelope.
func (v *Viewer) RenderLineageCard(lin *core.LineageEnvelope, format RenderFormat) string {
	if lin == nil {
		return "No lineage provenance recorded."
	}

	sigStatus := "Unsigned"
	if len(lin.SignatureEd25519) > 0 {
		sigStatus = "Ed25519 Verified"
	}

	tsStr := lin.Timestamp.Format(time.RFC3339)
	if lin.Timestamp.IsZero() {
		tsStr = "N/A"
	}

	switch format {
	case FormatMarkdown:
		var sb strings.Builder
		sb.WriteString("> **Provenance Lineage Card**\n")
		sb.WriteString(fmt.Sprintf("> - **User Prompt**: *\"%s\"*\n", lin.UserPrompt))
		if lin.Intent != "" {
			sb.WriteString(fmt.Sprintf("> - **Intent**: %s\n", lin.Intent))
		}
		if lin.SessionID != "" {
			sb.WriteString(fmt.Sprintf("> - **Session ID**: `%s`\n", lin.SessionID))
		}
		if lin.OrchestratorAgentID != "" {
			sb.WriteString(fmt.Sprintf("> - **Orchestrator**: `%s`\n", lin.OrchestratorAgentID))
		}
		if lin.ExecutingAgentID != "" {
			sb.WriteString(fmt.Sprintf("> - **Executing Agent**: `%s`\n", lin.ExecutingAgentID))
		}
		if lin.LLMVersion != "" {
			sb.WriteString(fmt.Sprintf("> - **Model**: `%s`\n", lin.LLMVersion))
		}
		sb.WriteString(fmt.Sprintf("> - **Timestamp**: %s\n", tsStr))
		sb.WriteString(fmt.Sprintf("> - **Signature**: `%s`\n", sigStatus))
		return sb.String()

	case FormatTerminal:
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("User Prompt:  %s%s\"%s\"%s\n", ansiBold, ansiYellow, lin.UserPrompt, ansiReset))
		if lin.Intent != "" {
			sb.WriteString(fmt.Sprintf("Intent:       %s%s%s\n", ansiCyan, lin.Intent, ansiReset))
		}
		if lin.ExecutingAgentID != "" {
			sb.WriteString(fmt.Sprintf("Agent:        %s%s%s\n", ansiGreen, lin.ExecutingAgentID, ansiReset))
		}
		if lin.LLMVersion != "" {
			sb.WriteString(fmt.Sprintf("Model:        %s%s%s\n", ansiMagenta, lin.LLMVersion, ansiReset))
		}
		sb.WriteString(fmt.Sprintf("Timestamp:    %s\n", tsStr))
		sb.WriteString(fmt.Sprintf("Signature:    %s%s%s\n", ansiGreen, sigStatus, ansiReset))
		return sb.String()

	default:
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Prompt:    \"%s\"\n", lin.UserPrompt))
		if lin.Intent != "" {
			sb.WriteString(fmt.Sprintf("Intent:    %s\n", lin.Intent))
		}
		if lin.ExecutingAgentID != "" {
			sb.WriteString(fmt.Sprintf("Agent:     %s\n", lin.ExecutingAgentID))
		}
		sb.WriteString(fmt.Sprintf("Timestamp: %s | Sig: %s\n", tsStr, sigStatus))
		return sb.String()
	}
}

// RenderDiff computes and formats unified diff between old and new text.
func (v *Viewer) RenderDiff(oldText, newText, filename string, format RenderFormat) string {
	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")

	var sb strings.Builder
	if format == FormatMarkdown {
		sb.WriteString(fmt.Sprintf("```diff\n--- %s\n+++ %s\n", filename, filename))
	} else if format == FormatTerminal {
		sb.WriteString(fmt.Sprintf("%s--- %s%s\n%s+++ %s%s\n",
			ansiRed, filename, ansiReset,
			ansiGreen, filename, ansiReset))
	} else {
		sb.WriteString(fmt.Sprintf("--- %s\n+++ %s\n", filename, filename))
	}

	maxL := len(oldLines)
	if len(newLines) > maxL {
		maxL = len(newLines)
	}

	for i := 0; i < maxL; i++ {
		var oLine, nLine string
		hasOld := i < len(oldLines)
		hasNew := i < len(newLines)

		if hasOld {
			oLine = oldLines[i]
		}
		if hasNew {
			nLine = newLines[i]
		}

		if hasOld && hasNew && oLine == nLine {
			sb.WriteString(fmt.Sprintf("  %s\n", oLine))
		} else {
			if hasOld && oLine != "" {
				if format == FormatTerminal {
					sb.WriteString(fmt.Sprintf("%s- %s%s\n", ansiRed, oLine, ansiReset))
				} else {
					sb.WriteString(fmt.Sprintf("- %s\n", oLine))
				}
			}
			if hasNew && nLine != "" {
				if format == FormatTerminal {
					sb.WriteString(fmt.Sprintf("%s+ %s%s\n", ansiGreen, nLine, ansiReset))
				} else {
					sb.WriteString(fmt.Sprintf("+ %s\n", nLine))
				}
			}
		}
	}

	if format == FormatMarkdown {
		sb.WriteString("```\n")
	}

	return sb.String()
}

// RenderNodeDiff computes and formats unified diff between old and new ASTSymbolNodes.
func (v *Viewer) RenderNodeDiff(oldNode, newNode *core.ASTSymbolNode, format RenderFormat) string {
	oldCode := ""
	if oldNode != nil {
		oldCode, _ = v.hydrator.HydrateSymbol(oldNode)
	}
	newCode := ""
	if newNode != nil {
		newCode, _ = v.hydrator.HydrateSymbol(newNode)
	}

	label := "symbol"
	if oldNode != nil {
		label = oldNode.Identifier
	} else if newNode != nil {
		label = newNode.Identifier
	}

	return v.RenderDiff(oldCode, newCode, label, format)
}

func (v *Viewer) indentLines(text, prefix string) string {
	lines := strings.Split(text, "\n")
	var result []string
	for _, l := range lines {
		if l != "" {
			result = append(result, prefix+l)
		}
	}
	return strings.Join(result, "\n") + "\n"
}

func (v *Viewer) highlightCode(code string, lang core.Language) string {
	lines := strings.Split(code, "\n")
	var sb strings.Builder
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("%s│%s %s%4d%s │ %s\n", ansiCyan, ansiReset, ansiGray, i+1, ansiReset, l))
	}
	return sb.String()
}
