package dockerfile

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// DockerInstruction represents an individual instruction in a Dockerfile/Containerfile.
type DockerInstruction struct {
	Directive  string            `json:"directive"` // FROM, RUN, COPY, ENV, EXPOSE, ENTRYPOINT, CMD, WORKDIR, ARG, LABEL
	Arguments  string            `json:"arguments"`
	StageName  string            `json:"stage_name,omitempty"`  // For multi-stage builds: FROM <image> AS <stage>
	Image      string            `json:"image,omitempty"`       // For FROM
	Port       string            `json:"port,omitempty"`        // For EXPOSE
	Protocol   string            `json:"protocol,omitempty"`    // tcp/udp
	EnvKey     string            `json:"env_key,omitempty"`     // For ENV
	EnvValue   string            `json:"env_value,omitempty"`   // For ENV
	Labels     map[string]string `json:"labels,omitempty"`      // For LABEL
	SourcePath string            `json:"source_path,omitempty"` // For COPY/ADD
	DestPath   string            `json:"dest_path,omitempty"`   // For COPY/ADD
	RawLine    string            `json:"raw_line"`
}

// DockerfileAST represents the parsed AST of a Dockerfile/Containerfile.
type DockerfileAST struct {
	Stages       []string            `json:"stages"`
	Instructions []DockerInstruction `json:"instructions"`
	ExposedPorts []string            `json:"exposed_ports"`
	EnvVariables map[string]string   `json:"env_variables"`
}

// DockerfileParser extracts ASTSymbolNodes from Dockerfile/Containerfile source bytes.
type DockerfileParser struct{}

// NewDockerfileParser creates a DockerfileParser instance.
func NewDockerfileParser() *DockerfileParser {
	return &DockerfileParser{}
}

// ParsedDockerfileResult contains extracted symbols and summary AST.
type ParsedDockerfileResult struct {
	Symbols      []*core.ASTSymbolNode `json:"symbols"`
	AST          DockerfileAST         `json:"ast"`
	ExposedPorts []string              `json:"exposed_ports"`
	EnvVariables map[string]string     `json:"env_variables"`
}

// ParseSource parses Dockerfile contents into structured ASTSymbolNodes.
func (p *DockerfileParser) ParseSource(filePath string, content []byte, env core.LineageEnvelope) (*ParsedDockerfileResult, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	var instructions []DockerInstruction
	var symbols []*core.ASTSymbolNode
	var stages []string
	exposedPorts := make([]string, 0)
	envVars := make(map[string]string)

	currentStage := "base"
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		rawLine := strings.TrimSpace(scanner.Text())
		if rawLine == "" || strings.HasPrefix(rawLine, "#") {
			continue
		}

		parts := strings.SplitN(rawLine, " ", 2)
		directive := strings.ToUpper(parts[0])
		args := ""
		if len(parts) > 1 {
			args = strings.TrimSpace(parts[1])
		}

		inst := DockerInstruction{
			Directive: directive,
			Arguments: args,
			RawLine:   rawLine,
		}

		nodeType := "Instruction"
		identifier := fmt.Sprintf("%s_%d", directive, lineNum)
		metadata := map[string]string{
			"directive": directive,
			"line":      fmt.Sprintf("%d", lineNum),
			"stage":     currentStage,
			"file_path": filePath,
		}

		switch directive {
		case "FROM":
			nodeType = "BaseImage"
			fromParts := strings.Fields(args)
			if len(fromParts) > 0 {
				inst.Image = fromParts[0]
				metadata["image"] = inst.Image
				identifier = fmt.Sprintf("FROM_%s", strings.ReplaceAll(strings.ReplaceAll(inst.Image, "/", "_"), ":", "_"))
			}
			if len(fromParts) >= 3 && strings.ToUpper(fromParts[1]) == "AS" {
				inst.StageName = fromParts[2]
				currentStage = inst.StageName
				stages = append(stages, currentStage)
				metadata["stage_name"] = currentStage
			}

		case "EXPOSE":
			nodeType = "PortExpose"
			portParts := strings.Split(args, "/")
			inst.Port = portParts[0]
			metadata["port"] = inst.Port
			if len(portParts) > 1 {
				inst.Protocol = portParts[1]
				metadata["protocol"] = inst.Protocol
			}
			identifier = fmt.Sprintf("EXPOSE_%s", inst.Port)
			exposedPorts = append(exposedPorts, inst.Port)

		case "ENV":
			nodeType = "EnvBinding"
			envParts := strings.SplitN(args, "=", 2)
			if len(envParts) == 2 {
				inst.EnvKey = strings.TrimSpace(envParts[0])
				inst.EnvValue = strings.TrimSpace(envParts[1])
			} else {
				envFields := strings.Fields(args)
				if len(envFields) >= 2 {
					inst.EnvKey = envFields[0]
					inst.EnvValue = envFields[1]
				}
			}
			if inst.EnvKey != "" {
				metadata["env_key"] = inst.EnvKey
				metadata["env_value"] = inst.EnvValue
				identifier = fmt.Sprintf("ENV_%s", inst.EnvKey)
				envVars[inst.EnvKey] = inst.EnvValue
			}

		case "ENTRYPOINT", "CMD":
			nodeType = "EntryPoint"
			identifier = fmt.Sprintf("%s_Command", directive)
			metadata["command"] = args

		case "COPY", "ADD":
			nodeType = "CopyInstruction"
			copyFields := strings.Fields(args)
			if len(copyFields) >= 2 {
				inst.SourcePath = copyFields[0]
				inst.DestPath = copyFields[1]
				metadata["src"] = inst.SourcePath
				metadata["dst"] = inst.DestPath
				identifier = fmt.Sprintf("COPY_%s_TO_%s", strings.ReplaceAll(inst.SourcePath, "/", "_"), strings.ReplaceAll(inst.DestPath, "/", "_"))
			}

		case "WORKDIR":
			nodeType = "WorkdirInstruction"
			metadata["workdir"] = args
			identifier = fmt.Sprintf("WORKDIR_%s", strings.ReplaceAll(args, "/", "_"))
		}

		instructions = append(instructions, inst)

		payloadBytes, _ := json.Marshal(inst)
		sum := sha256.Sum256(payloadBytes)
		nodeID := hex.EncodeToString(sum[:])

		symbol := &core.ASTSymbolNode{
			NodeID:      nodeID,
			Language:    core.LangDockerfile,
			NodeType:    nodeType,
			Identifier:  identifier,
			Signature:   rawLine,
			ASTPayload:  payloadBytes,
			ASTMetadata: metadata,
			Lineage:     env,
		}
		symbols = append(symbols, symbol)
	}

	result := &ParsedDockerfileResult{
		Symbols: symbols,
		AST: DockerfileAST{
			Stages:       stages,
			Instructions: instructions,
			ExposedPorts: exposedPorts,
			EnvVariables: envVars,
		},
		ExposedPorts: exposedPorts,
		EnvVariables: envVars,
	}

	return result, nil
}

// BuildComponentNode bundles parsed Dockerfile symbols into a core.ComponentNode.
func (p *DockerfileParser) BuildComponentNode(
	res *ParsedDockerfileResult,
	compName string,
	compType core.ComponentType,
	lineage core.LineageEnvelope,
) (*core.ComponentNode, error) {
	if compName == "" {
		compName = "container-image"
	}
	if !compType.IsValid() {
		compType = core.CompInfra
	}

	symbolIDs := make([]string, 0, len(res.Symbols))
	for _, sym := range res.Symbols {
		symbolIDs = append(symbolIDs, sym.NodeID)
	}

	metadata := map[string]string{
		"stages":       strings.Join(res.AST.Stages, ","),
		"symbol_count": fmt.Sprintf("%d", len(res.Symbols)),
	}
	if len(res.ExposedPorts) > 0 {
		metadata["ports"] = strings.Join(res.ExposedPorts, ",")
	}

	comp := &core.ComponentNode{
		Name:        compName,
		Type:        compType,
		Language:    core.LangDockerfile,
		SymbolNodes: symbolIDs,
		Metadata:    metadata,
		Lineage:     lineage,
	}

	compID, err := core.HashComponentNode(comp)
	if err != nil {
		return nil, fmt.Errorf("computing component node hash: %w", err)
	}
	comp.ComponentID = compID
	return comp, nil
}
