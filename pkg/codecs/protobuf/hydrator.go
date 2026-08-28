package protobuf

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// ProtobufHydrator converts Protobuf ASTSymbolNodes back into clean, formatted .proto source code.
type ProtobufHydrator struct{}

// NewProtobufHydrator creates an initialized ProtobufHydrator instance.
func NewProtobufHydrator() *ProtobufHydrator {
	return &ProtobufHydrator{}
}

// HydrateSymbol reconstitutes an ASTSymbolNode into Protobuf source text.
func (h *ProtobufHydrator) HydrateSymbol(node *core.ASTSymbolNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node is nil")
	}

	if len(node.ASTPayload) == 0 {
		return fmt.Sprintf("// Protobuf Symbol: %s (%s)\n", node.Identifier, node.NodeType), nil
	}

	switch node.NodeType {
	case "Service":
		var svc ProtoService
		if err := json.Unmarshal(node.ASTPayload, &svc); err != nil {
			return "", fmt.Errorf("unmarshaling ProtoService: %w", err)
		}
		return h.renderService(&svc), nil

	case "RPCMethod":
		var m ProtoRPCMethod
		if err := json.Unmarshal(node.ASTPayload, &m); err != nil {
			return "", fmt.Errorf("unmarshaling ProtoRPCMethod: %w", err)
		}
		return h.renderRPCMethod(&m, 0), nil

	case "Message":
		var msg ProtoMessage
		if err := json.Unmarshal(node.ASTPayload, &msg); err != nil {
			return "", fmt.Errorf("unmarshaling ProtoMessage: %w", err)
		}
		return h.renderMessage(&msg), nil

	case "Enum":
		var en ProtoEnum
		if err := json.Unmarshal(node.ASTPayload, &en); err != nil {
			return "", fmt.Errorf("unmarshaling ProtoEnum: %w", err)
		}
		return h.renderEnum(&en), nil

	default:
		return string(node.ASTPayload), nil
	}
}

func (h *ProtobufHydrator) renderService(svc *ProtoService) string {
	var sb strings.Builder

	if svc.Doc != "" {
		for _, line := range strings.Split(svc.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("// %s\n", line))
		}
	}

	sb.WriteString(fmt.Sprintf("service %s {\n", svc.Name))
	for _, m := range svc.Methods {
		sb.WriteString(h.renderRPCMethod(&m, 1))
	}
	sb.WriteString("}\n")

	return sb.String()
}

func (h *ProtobufHydrator) renderRPCMethod(m *ProtoRPCMethod, indentLevel int) string {
	indent := strings.Repeat("  ", indentLevel)
	var sb strings.Builder

	if m.Doc != "" {
		for _, line := range strings.Split(m.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("%s// %s\n", indent, line))
		}
	}

	inStream := ""
	if m.ClientStream {
		inStream = "stream "
	}
	outStream := ""
	if m.ServerStream {
		outStream = "stream "
	}

	if m.HTTPMethod != "" && m.HTTPPath != "" {
		sb.WriteString(fmt.Sprintf("%srpc %s (%s%s) returns (%s%s) {\n", indent, m.Name, inStream, m.InputType, outStream, m.OutputType))
		httpMethodLower := strings.ToLower(m.HTTPMethod)
		sb.WriteString(fmt.Sprintf("%s  option (google.api.http) = {\n", indent))
		sb.WriteString(fmt.Sprintf("%s    %s: \"%s\"\n", indent, httpMethodLower, m.HTTPPath))
		sb.WriteString(fmt.Sprintf("%s  };\n", indent))
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
	} else {
		sb.WriteString(fmt.Sprintf("%srpc %s (%s%s) returns (%s%s);\n", indent, m.Name, inStream, m.InputType, outStream, m.OutputType))
	}

	return sb.String()
}

func (h *ProtobufHydrator) renderMessage(msg *ProtoMessage) string {
	var sb strings.Builder

	if msg.Doc != "" {
		for _, line := range strings.Split(msg.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("// %s\n", line))
		}
	}

	sb.WriteString(fmt.Sprintf("message %s {\n", msg.Name))
	for _, f := range msg.Fields {
		if f.Doc != "" {
			for _, line := range strings.Split(f.Doc, "\n") {
				sb.WriteString(fmt.Sprintf("  // %s\n", line))
			}
		}

		modifier := ""
		if f.IsRepeated {
			modifier = "repeated "
		} else if f.IsOptional {
			modifier = "optional "
		}

		sb.WriteString(fmt.Sprintf("  %s%s %s = %d;\n", modifier, f.Type, f.Name, f.Number))
	}
	sb.WriteString("}\n")

	return sb.String()
}

func (h *ProtobufHydrator) renderEnum(en *ProtoEnum) string {
	var sb strings.Builder

	if en.Doc != "" {
		for _, line := range strings.Split(en.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("// %s\n", line))
		}
	}

	sb.WriteString(fmt.Sprintf("enum %s {\n", en.Name))
	for _, v := range en.Values {
		if v.Doc != "" {
			for _, line := range strings.Split(v.Doc, "\n") {
				sb.WriteString(fmt.Sprintf("  // %s\n", line))
			}
		}
		sb.WriteString(fmt.Sprintf("  %s = %d;\n", v.Name, v.Number))
	}
	sb.WriteString("}\n")

	return sb.String()
}
