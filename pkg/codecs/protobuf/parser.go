package protobuf

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// ProtoField represents a single field inside a Protobuf message.
type ProtoField struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Number     int    `json:"number"`
	IsRepeated bool   `json:"is_repeated"`
	IsOptional bool   `json:"is_optional"`
	Doc        string `json:"doc,omitempty"`
}

// ProtoEnumVal represents an individual value in a Protobuf enum.
type ProtoEnumVal struct {
	Name   string `json:"name"`
	Number int    `json:"number"`
	Doc    string `json:"doc,omitempty"`
}

// ProtoEnum represents an extracted Protobuf enum definition.
type ProtoEnum struct {
	Name   string         `json:"name"`
	Doc    string         `json:"doc,omitempty"`
	Values []ProtoEnumVal `json:"values"`
}

// ProtoMessage represents an extracted Protobuf message definition.
type ProtoMessage struct {
	Name   string       `json:"name"`
	Doc    string       `json:"doc,omitempty"`
	Fields []ProtoField `json:"fields"`
	Enums  []ProtoEnum  `json:"enums,omitempty"`
}

// ProtoRPCMethod represents an RPC method within a gRPC service.
type ProtoRPCMethod struct {
	Name         string `json:"name"`
	InputType    string `json:"input_type"`
	OutputType   string `json:"output_type"`
	ClientStream bool   `json:"client_stream"`
	ServerStream bool   `json:"server_stream"`
	Doc          string `json:"doc,omitempty"`
	HTTPMethod   string `json:"http_method,omitempty"` // GET, POST, etc. from google.api.http
	HTTPPath     string `json:"http_path,omitempty"`   // /v1/users/{id}
}

// ProtoService represents an extracted gRPC service definition.
type ProtoService struct {
	Name    string           `json:"name"`
	Doc     string           `json:"doc,omitempty"`
	Methods []ProtoRPCMethod `json:"methods"`
}

// ProtoFileResult contains all extracted Protobuf definitions from a .proto file.
type ProtoFileResult struct {
	FilePath   string                `json:"file_path"`
	Syntax     string                `json:"syntax"`
	Package    string                `json:"package"`
	Imports    []string              `json:"imports"`
	Options    map[string]string     `json:"options"`
	Services   []ProtoService        `json:"services"`
	Messages   []ProtoMessage        `json:"messages"`
	Enums      []ProtoEnum           `json:"enums"`
	AllSymbols []*core.ASTSymbolNode `json:"all_symbols"`
}

// ProtobufParser parses Proto3 files into structured AST symbol nodes.
type ProtobufParser struct{}

// NewProtobufParser creates a new ProtobufParser instance.
func NewProtobufParser() *ProtobufParser {
	return &ProtobufParser{}
}

// ParseSource parses Proto3 source text bytes.
func (p *ProtobufParser) ParseSource(filename string, src []byte, lineage core.LineageEnvelope) (*ProtoFileResult, error) {
	if filename == "" {
		filename = "service.proto"
	}

	result := &ProtoFileResult{
		FilePath: filename,
		Syntax:   "proto3",
		Options:  make(map[string]string),
	}

	lines := splitProtoLines(src)

	// 1. Extract Package, Imports, Options, Syntax
	p.extractHeaderMetadata(lines, result)

	// 2. Extract Services & RPCs
	result.Services = p.extractServices(lines)

	// 3. Extract Messages
	result.Messages = p.extractMessages(lines)

	// 4. Extract Top-level Enums
	result.Enums = p.extractEnums(lines)

	// 5. Convert to ASTSymbolNodes
	symbols, err := p.toSymbolNodes(result, lineage)
	if err != nil {
		return nil, fmt.Errorf("converting to ASTSymbolNodes: %w", err)
	}
	result.AllSymbols = symbols

	return result, nil
}

// ParseFile parses a .proto file from disk.
func (p *ProtobufParser) ParseFile(filePath string, lineage core.LineageEnvelope) (*ProtoFileResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filePath, err)
	}
	return p.ParseSource(filePath, data, lineage)
}

// ParseDir parses all .proto files in a directory.
func (p *ProtobufParser) ParseDir(dirPath string, lineage core.LineageEnvelope) ([]*ProtoFileResult, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dirPath, err)
	}

	var results []*ProtoFileResult
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".proto" {
			fPath := filepath.Join(dirPath, entry.Name())
			res, err := p.ParseFile(fPath, lineage)
			if err != nil {
				return nil, err
			}
			results = append(results, res)
		}
	}
	return results, nil
}

type protoLine struct {
	num  int
	text string
}

func splitProtoLines(src []byte) []protoLine {
	var lines []protoLine
	scanner := bufio.NewScanner(bytes.NewReader(src))
	num := 1
	for scanner.Scan() {
		lines = append(lines, protoLine{
			num:  num,
			text: scanner.Text(),
		})
		num++
	}
	return lines
}

var (
	protoSyntaxRegex  = regexp.MustCompile(`^syntax\s*=\s*["']([^"']+)["']\s*;`)
	protoPackageRegex = regexp.MustCompile(`^package\s+([a-zA-Z0-9_.]+)\s*;`)
	protoImportRegex  = regexp.MustCompile(`^import\s+["']([^"']+)["']\s*;`)
	protoOptionRegex  = regexp.MustCompile(`^option\s+([a-zA-Z0-9_.]+)\s*=\s*["']([^"']+)["']\s*;|^option\s+([a-zA-Z0-9_.]+)\s*=\s*([^;]+)\s*;`)
	protoServiceRegex = regexp.MustCompile(`^service\s+([a-zA-Z0-9_]+)`)
	protoRpcRegex     = regexp.MustCompile(`^rpc\s+([a-zA-Z0-9_]+)\s*\(\s*(stream\s+)?([a-zA-Z0-9_.]+)\s*\)\s*returns\s*\(\s*(stream\s+)?([a-zA-Z0-9_.]+)\s*\)`)
	protoMsgRegex     = regexp.MustCompile(`^message\s+([a-zA-Z0-9_]+)`)
	protoEnumRegex    = regexp.MustCompile(`^enum\s+([a-zA-Z0-9_]+)`)
	protoFieldRegex   = regexp.MustCompile(`^(repeated\s+|optional\s+)?([a-zA-Z0-9_.]+)\s+([a-zA-Z0-9_]+)\s*=\s*([0-9]+)\s*;`)
	protoEnumValRegex = regexp.MustCompile(`^([a-zA-Z0-9_]+)\s*=\s*([0-9]+)\s*;`)
	protoHttpRegex    = regexp.MustCompile(`\b(get|post|put|delete|patch)\s*:\s*["']([^"']+)["']`)
)

func (p *ProtobufParser) extractHeaderMetadata(lines []protoLine, res *ProtoFileResult) {
	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if sm := protoSyntaxRegex.FindStringSubmatch(trimmed); sm != nil {
			res.Syntax = sm[1]
		}
		if pm := protoPackageRegex.FindStringSubmatch(trimmed); pm != nil {
			res.Package = pm[1]
		}
		if im := protoImportRegex.FindStringSubmatch(trimmed); im != nil {
			res.Imports = append(res.Imports, im[1])
		}
		if om := protoOptionRegex.FindStringSubmatch(trimmed); om != nil {
			key := om[1]
			val := om[2]
			if key == "" {
				key = om[3]
				val = om[4]
			}
			res.Options[key] = strings.TrimSpace(val)
		}
	}
}

func (p *ProtobufParser) extractServices(lines []protoLine) []ProtoService {
	var services []ProtoService
	var pendingDoc []string

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)

		if strings.HasPrefix(trimmed, "//") {
			pendingDoc = append(pendingDoc, strings.TrimSpace(strings.TrimPrefix(trimmed, "//")))
			i++
			continue
		}

		if m := protoServiceRegex.FindStringSubmatch(trimmed); m != nil {
			sName := m[1]
			var methods []ProtoRPCMethod

			braceCount := 0
			if strings.Contains(trimmed, "{") {
				braceCount = 1
			}

			var methodDoc []string
			i++
			for i < len(lines) {
				sLine := strings.TrimSpace(lines[i].text)

				if strings.HasPrefix(sLine, "//") {
					methodDoc = append(methodDoc, strings.TrimSpace(strings.TrimPrefix(sLine, "//")))
					i++
					continue
				}

				if rm := protoRpcRegex.FindStringSubmatch(sLine); rm != nil {
					mName := rm[1]
					clientStream := strings.TrimSpace(rm[2]) == "stream"
					inType := rm[3]
					serverStream := strings.TrimSpace(rm[4]) == "stream"
					outType := rm[5]

					var httpMethod, httpPath string

					// Check if there are options within the RPC block
					if strings.Contains(sLine, "{") {
						rpcBraces := 1
						for i+1 < len(lines) {
							i++
							optLine := strings.TrimSpace(lines[i].text)
							if hm := protoHttpRegex.FindStringSubmatch(optLine); hm != nil {
								httpMethod = strings.ToUpper(hm[1])
								httpPath = hm[2]
							}
							for _, ch := range optLine {
								if ch == '{' {
									rpcBraces++
								} else if ch == '}' {
									rpcBraces--
								}
							}
							if rpcBraces <= 0 {
								break
							}
						}
					}

					methods = append(methods, ProtoRPCMethod{
						Name:         mName,
						InputType:    inType,
						OutputType:   outType,
						ClientStream: clientStream,
						ServerStream: serverStream,
						Doc:          strings.Join(methodDoc, "\n"),
						HTTPMethod:   httpMethod,
						HTTPPath:     httpPath,
					})
					methodDoc = nil
				} else {
					if strings.Contains(sLine, "{") {
						braceCount++
					}
					if strings.Contains(sLine, "}") {
						braceCount--
						if braceCount <= 0 {
							break
						}
					}
					if sLine != "" && !strings.HasPrefix(sLine, "//") {
						methodDoc = nil
					}
				}
				i++
			}

			services = append(services, ProtoService{
				Name:    sName,
				Doc:     strings.Join(pendingDoc, "\n"),
				Methods: methods,
			})
			pendingDoc = nil
		} else {
			if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
				pendingDoc = nil
			}
		}
		i++
	}

	return services
}

func (p *ProtobufParser) extractMessages(lines []protoLine) []ProtoMessage {
	var messages []ProtoMessage
	var pendingDoc []string

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)

		if strings.HasPrefix(trimmed, "//") {
			pendingDoc = append(pendingDoc, strings.TrimSpace(strings.TrimPrefix(trimmed, "//")))
			i++
			continue
		}

		if m := protoMsgRegex.FindStringSubmatch(trimmed); m != nil {
			msgName := m[1]
			var fields []ProtoField

			braceCount := 0
			if strings.Contains(trimmed, "{") {
				braceCount = 1
			}

			var fieldDoc []string
			i++
			for i < len(lines) {
				mLine := strings.TrimSpace(lines[i].text)

				if strings.HasPrefix(mLine, "//") {
					fieldDoc = append(fieldDoc, strings.TrimSpace(strings.TrimPrefix(mLine, "//")))
					i++
					continue
				}

				if strings.Contains(mLine, "{") {
					braceCount++
				}
				if strings.Contains(mLine, "}") {
					braceCount--
					if braceCount <= 0 {
						break
					}
				}

				if fm := protoFieldRegex.FindStringSubmatch(mLine); fm != nil {
					mod := strings.TrimSpace(fm[1])
					isRepeated := mod == "repeated"
					isOptional := mod == "optional"
					fType := fm[2]
					fName := fm[3]
					fNum, _ := strconv.Atoi(fm[4])

					fields = append(fields, ProtoField{
						Name:       fName,
						Type:       fType,
						Number:     fNum,
						IsRepeated: isRepeated,
						IsOptional: isOptional,
						Doc:        strings.Join(fieldDoc, "\n"),
					})
					fieldDoc = nil
				} else {
					if mLine != "" && !strings.HasPrefix(mLine, "//") {
						fieldDoc = nil
					}
				}
				i++
			}

			messages = append(messages, ProtoMessage{
				Name:   msgName,
				Doc:    strings.Join(pendingDoc, "\n"),
				Fields: fields,
			})
			pendingDoc = nil
		} else {
			if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
				pendingDoc = nil
			}
		}
		i++
	}

	return messages
}

func (p *ProtobufParser) extractEnums(lines []protoLine) []ProtoEnum {
	var enums []ProtoEnum
	var pendingDoc []string

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)

		if strings.HasPrefix(trimmed, "//") {
			pendingDoc = append(pendingDoc, strings.TrimSpace(strings.TrimPrefix(trimmed, "//")))
			i++
			continue
		}

		if m := protoEnumRegex.FindStringSubmatch(trimmed); m != nil {
			eName := m[1]
			var values []ProtoEnumVal

			braceCount := 0
			if strings.Contains(trimmed, "{") {
				braceCount = 1
			}

			var valDoc []string
			i++
			for i < len(lines) {
				eLine := strings.TrimSpace(lines[i].text)

				if strings.HasPrefix(eLine, "//") {
					valDoc = append(valDoc, strings.TrimSpace(strings.TrimPrefix(eLine, "//")))
					i++
					continue
				}

				if strings.Contains(eLine, "{") {
					braceCount++
				}
				if strings.Contains(eLine, "}") {
					braceCount--
					if braceCount <= 0 {
						break
					}
				}

				if vm := protoEnumValRegex.FindStringSubmatch(eLine); vm != nil {
					vName := vm[1]
					vNum, _ := strconv.Atoi(vm[2])

					values = append(values, ProtoEnumVal{
						Name:   vName,
						Number: vNum,
						Doc:    strings.Join(valDoc, "\n"),
					})
					valDoc = nil
				}
				i++
			}

			enums = append(enums, ProtoEnum{
				Name:   eName,
				Doc:    strings.Join(pendingDoc, "\n"),
				Values: values,
			})
			pendingDoc = nil
		} else {
			if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
				pendingDoc = nil
			}
		}
		i++
	}

	return enums
}

func (p *ProtobufParser) toSymbolNodes(res *ProtoFileResult, lineage core.LineageEnvelope) ([]*core.ASTSymbolNode, error) {
	var nodes []*core.ASTSymbolNode

	// Services and RPC Methods
	for _, svc := range res.Services {
		svcPayload, err := json.Marshal(svc)
		if err != nil {
			return nil, err
		}

		var deps []string
		for _, m := range svc.Methods {
			deps = append(deps, m.InputType, m.OutputType)
		}
		sort.Strings(deps)

		ident := svc.Name
		if res.Package != "" {
			ident = fmt.Sprintf("%s.%s", res.Package, svc.Name)
		}

		svcNode := &core.ASTSymbolNode{
			Language:          core.LangProtobuf,
			NodeType:          "Service",
			Identifier:        fmt.Sprintf("service:%s", ident),
			Signature:         fmt.Sprintf("service %s", svc.Name),
			Docstring:         svc.Doc,
			Visibility:        "public",
			ASTPayload:        svcPayload,
			ASTMetadata:       map[string]string{"method_count": fmt.Sprintf("%d", len(svc.Methods))},
			LocalDependencies: deps,
			Dependencies:      deps,
			Lineage:           lineage,
		}

		svcNodeID, err := core.HashASTSymbolNode(svcNode)
		if err != nil {
			return nil, err
		}
		svcNode.NodeID = svcNodeID
		nodes = append(nodes, svcNode)

		// Individual RPC Methods
		for _, m := range svc.Methods {
			mPayload, err := json.Marshal(m)
			if err != nil {
				continue
			}

			mIdent := fmt.Sprintf("%s.%s", ident, m.Name)
			mMeta := map[string]string{
				"service":     ident,
				"input_type":  m.InputType,
				"output_type": m.OutputType,
			}
			if m.HTTPMethod != "" {
				mMeta["http_method"] = m.HTTPMethod
				mMeta["http_path"] = m.HTTPPath
			}

			sig := fmt.Sprintf("rpc %s (%s) returns (%s)", m.Name, m.InputType, m.OutputType)
			if m.ClientStream || m.ServerStream {
				inStream := ""
				if m.ClientStream {
					inStream = "stream "
				}
				outStream := ""
				if m.ServerStream {
					outStream = "stream "
				}
				sig = fmt.Sprintf("rpc %s (%s%s) returns (%s%s)", m.Name, inStream, m.InputType, outStream, m.OutputType)
			}

			mDeps := []string{m.InputType, m.OutputType}
			mNode := &core.ASTSymbolNode{
				Language:          core.LangProtobuf,
				NodeType:          "RPCMethod",
				Identifier:        fmt.Sprintf("rpc:%s", mIdent),
				Signature:         sig,
				Docstring:         m.Doc,
				Visibility:        "public",
				ASTPayload:        mPayload,
				ASTMetadata:       mMeta,
				LocalDependencies: mDeps,
				Dependencies:      mDeps,
				Lineage:           lineage,
			}
			mNodeID, err := core.HashASTSymbolNode(mNode)
			if err == nil {
				mNode.NodeID = mNodeID
				nodes = append(nodes, mNode)
			}
		}
	}

	// Messages
	for _, msg := range res.Messages {
		msgPayload, err := json.Marshal(msg)
		if err != nil {
			return nil, err
		}

		var deps []string
		for _, f := range msg.Fields {
			deps = append(deps, f.Type)
		}
		sort.Strings(deps)

		ident := msg.Name
		if res.Package != "" {
			ident = fmt.Sprintf("%s.%s", res.Package, msg.Name)
		}

		msgNode := &core.ASTSymbolNode{
			Language:          core.LangProtobuf,
			NodeType:          "Message",
			Identifier:        fmt.Sprintf("message:%s", ident),
			Signature:         fmt.Sprintf("message %s", msg.Name),
			Docstring:         msg.Doc,
			Visibility:        "public",
			ASTPayload:        msgPayload,
			ASTMetadata:       map[string]string{"field_count": fmt.Sprintf("%d", len(msg.Fields))},
			LocalDependencies: deps,
			Dependencies:      deps,
			Lineage:           lineage,
		}

		msgNodeID, err := core.HashASTSymbolNode(msgNode)
		if err != nil {
			return nil, err
		}
		msgNode.NodeID = msgNodeID
		nodes = append(nodes, msgNode)
	}

	// Enums
	for _, en := range res.Enums {
		enPayload, err := json.Marshal(en)
		if err != nil {
			return nil, err
		}

		ident := en.Name
		if res.Package != "" {
			ident = fmt.Sprintf("%s.%s", res.Package, en.Name)
		}

		enNode := &core.ASTSymbolNode{
			Language:    core.LangProtobuf,
			NodeType:    "Enum",
			Identifier:  fmt.Sprintf("enum:%s", ident),
			Signature:   fmt.Sprintf("enum %s", en.Name),
			Docstring:   en.Doc,
			Visibility:  "public",
			ASTPayload:  enPayload,
			ASTMetadata: map[string]string{"val_count": fmt.Sprintf("%d", len(en.Values))},
			Lineage:     lineage,
		}

		enNodeID, err := core.HashASTSymbolNode(enNode)
		if err != nil {
			return nil, err
		}
		enNode.NodeID = enNodeID
		nodes = append(nodes, enNode)
	}

	return nodes, nil
}

// BuildComponentNode bundles parsed Protobuf symbols into a core.ComponentNode.
func (p *ProtobufParser) BuildComponentNode(
	res *ProtoFileResult,
	compName string,
	compType core.ComponentType,
	lineage core.LineageEnvelope,
) (*core.ComponentNode, error) {
	if compName == "" {
		compName = res.Package
		if compName == "" {
			compName = "protobuf-contract"
		}
	}
	if !compType.IsValid() {
		compType = core.CompContract
	}

	symbolIDs := make([]string, 0, len(res.AllSymbols))
	for _, sym := range res.AllSymbols {
		symbolIDs = append(symbolIDs, sym.NodeID)
	}

	metadata := map[string]string{
		"file_path":     res.FilePath,
		"package":       res.Package,
		"syntax":        res.Syntax,
		"service_count": fmt.Sprintf("%d", len(res.Services)),
		"message_count": fmt.Sprintf("%d", len(res.Messages)),
		"symbol_count":  fmt.Sprintf("%d", len(res.AllSymbols)),
	}

	comp := &core.ComponentNode{
		Name:        compName,
		Type:        compType,
		Language:    core.LangProtobuf,
		SymbolNodes: symbolIDs,
		Metadata:    metadata,
		Lineage:     lineage,
	}

	compID, err := core.HashComponentNode(comp)
	if err != nil {
		return nil, fmt.Errorf("computing component hash: %w", err)
	}
	comp.ComponentID = compID
	return comp, nil
}
