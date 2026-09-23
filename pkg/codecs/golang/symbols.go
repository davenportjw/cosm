package golang

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// GoField represents a struct field definition.
type GoField struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Tag  string `json:"tag,omitempty"`
}

// GoStructSymbol represents an extracted Go struct definition.
type GoStructSymbol struct {
	Name    string    `json:"name"`
	Doc     string    `json:"doc,omitempty"`
	Fields  []GoField `json:"fields"`
	Methods []string  `json:"methods,omitempty"`
}

// GoInterfaceMethod represents a method in an interface.
type GoInterfaceMethod struct {
	Name    string        `json:"name"`
	Params  []GoFuncParam `json:"params,omitempty"`
	Results []string      `json:"results,omitempty"`
}

// GoInterfaceSymbol represents an extracted Go interface definition.
type GoInterfaceSymbol struct {
	Name    string              `json:"name"`
	Doc     string              `json:"doc,omitempty"`
	Methods []GoInterfaceMethod `json:"methods"`
}

// GoFuncParam represents a parameter in a function or method signature.
type GoFuncParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// GoFuncSymbol represents an extracted Go function or method declaration.
type GoFuncSymbol struct {
	Name            string        `json:"name"`
	ReceiverType    string        `json:"receiver_type,omitempty"`
	ReceiverName    string        `json:"receiver_name,omitempty"`
	IsMethod        bool          `json:"is_method"`
	Params          []GoFuncParam `json:"params"`
	Results         []string      `json:"results"`
	Doc             string        `json:"doc,omitempty"`
	BodySource      string        `json:"body_source,omitempty"`
	IsExported      bool          `json:"is_exported"`
	CalledSymbols   []string      `json:"called_symbols,omitempty"`
	EnvVarsAccessed []string      `json:"env_vars_accessed,omitempty"`
}

// GoRouteBinding represents an HTTP or gRPC route registration in Go.
type GoRouteBinding struct {
	Method      string `json:"method"`       // e.g. "GET", "POST", "GRPC"
	Path        string `json:"path"`         // e.g. "/api/v1/users", "/healthz"
	HandlerName string `json:"handler_name"` // e.g. "HandleGetUsers", "server.GetUsers"
	Framework   string `json:"framework"`    // e.g. "stdlib", "chi", "gin", "echo", "gorilla", "grpc"
	LineNumber  int    `json:"line_number"`
}

// GoImport represents an imported package.
type GoImport struct {
	Path  string `json:"path"`
	Alias string `json:"alias,omitempty"`
}

// GoFileSymbols encapsulates all extracted symbols from a single Go source file.
type GoFileSymbols struct {
	PackageName     string               `json:"package_name"`
	FilePath        string               `json:"file_path"`
	Structs         []GoStructSymbol     `json:"structs"`
	Interfaces      []GoInterfaceSymbol  `json:"interfaces"`
	Functions       []GoFuncSymbol       `json:"functions"`
	GeneralDecls    []GoGeneralDecl      `json:"general_decls,omitempty"`
	Routes          []GoRouteBinding     `json:"routes"`
	Imports         []GoImport           `json:"imports"`
	EnvVarsAccessed []string             `json:"env_vars_accessed"`
	Trivia          *core.TriviaEnvelope `json:"trivia,omitempty"`
}

// GoGeneralDecl encapsulates package-level variables, constants, or custom typedefs.
type GoGeneralDecl struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"` // "ConstDecl", "VarDecl", "TypeDecl"
	SourceCode string `json:"source_code"`
	Doc        string `json:"doc,omitempty"`
}

// GoPackageResult contains aggregated symbols across all files in a Go package.
type GoPackageResult struct {
	PackageName string                    `json:"package_name"`
	DirPath     string                    `json:"dir_path"`
	Files       map[string]*GoFileSymbols `json:"files"`
	AllSymbols  []*core.ASTSymbolNode     `json:"all_symbols"`
	AllRoutes   []GoRouteBinding          `json:"all_routes"`
	AllImports  []GoImport                `json:"all_imports"`
	Trivia      *core.TriviaEnvelope      `json:"trivia,omitempty"`
}

// StructToASTSymbolNode converts a GoStructSymbol to a core.ASTSymbolNode.
func StructToASTSymbolNode(s GoStructSymbol, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal struct symbol %s: %w", s.Name, err)
	}

	var localDeps []string
	for _, f := range s.Fields {
		cleanType := strings.TrimLeft(f.Type, "*[]")
		if cleanType != "" && !isBuiltinGoType(cleanType) {
			localDeps = append(localDeps, cleanType)
		}
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangGo,
		NodeType:          "StructDecl",
		Identifier:        fmt.Sprintf("%s.%s", pkgName, s.Name),
		ASTPayload:        payload,
		LocalDependencies: localDeps,
		Lineage:           lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, fmt.Errorf("failed to compute hash for struct %s: %w", s.Name, err)
	}
	node.NodeID = nodeID

	return node, nil
}

// InterfaceToASTSymbolNode converts a GoInterfaceSymbol to a core.ASTSymbolNode.
func InterfaceToASTSymbolNode(iface GoInterfaceSymbol, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(iface)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal interface symbol %s: %w", iface.Name, err)
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangGo,
		NodeType:          "InterfaceDecl",
		Identifier:        fmt.Sprintf("%s.%s", pkgName, iface.Name),
		ASTPayload:        payload,
		LocalDependencies: nil,
		Lineage:           lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, fmt.Errorf("failed to compute hash for interface %s: %w", iface.Name, err)
	}
	node.NodeID = nodeID

	return node, nil
}

// FuncToASTSymbolNode converts a GoFuncSymbol to a core.ASTSymbolNode.
func FuncToASTSymbolNode(fn GoFuncSymbol, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(fn)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal func symbol %s: %w", fn.Name, err)
	}

	ident := fmt.Sprintf("%s.%s", pkgName, fn.Name)
	if fn.IsMethod && fn.ReceiverType != "" {
		cleanRecv := strings.TrimLeft(fn.ReceiverType, "*")
		ident = fmt.Sprintf("%s.(%s).%s", pkgName, cleanRecv, fn.Name)
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangGo,
		NodeType:          "FunctionDecl",
		Identifier:        ident,
		ASTPayload:        payload,
		LocalDependencies: fn.CalledSymbols,
		Lineage:           lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, fmt.Errorf("failed to compute hash for func %s: %w", fn.Name, err)
	}
	node.NodeID = nodeID

	return node, nil
}

// RouteToASTSymbolNode converts a GoRouteBinding to a core.ASTSymbolNode.
func RouteToASTSymbolNode(r GoRouteBinding, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal route binding %s: %w", r.Path, err)
	}

	var localDeps []string
	if r.HandlerName != "" {
		localDeps = append(localDeps, r.HandlerName)
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangGo,
		NodeType:          "RouteBinding",
		Identifier:        fmt.Sprintf("%s:%s %s", pkgName, r.Method, r.Path),
		ASTPayload:        payload,
		LocalDependencies: localDeps,
		Lineage:           lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, fmt.Errorf("failed to compute hash for route %s: %w", r.Path, err)
	}
	node.NodeID = nodeID

	return node, nil
}

// GeneralDeclToASTSymbolNode converts a GoGeneralDecl to a core.ASTSymbolNode.
func GeneralDeclToASTSymbolNode(decl GoGeneralDecl, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	node := &core.ASTSymbolNode{
		Language:   core.LangGo,
		NodeType:   decl.Kind,
		Identifier: fmt.Sprintf("%s.%s", pkgName, decl.Name),
		ASTPayload: []byte(decl.SourceCode),
		Docstring:  decl.Doc,
		Lineage:    lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, fmt.Errorf("failed to compute hash for general decl %s: %w", decl.Name, err)
	}
	node.NodeID = nodeID

	return node, nil
}


// BuildComponentNode bundles package symbols into a core.ComponentNode.
func BuildComponentNode(compName string, compType core.ComponentType, pkgResult *GoPackageResult, lineage core.LineageEnvelope) (*core.ComponentNode, error) {
	if compName == "" {
		compName = pkgResult.PackageName
	}
	if !compType.IsValid() {
		compType = core.CompService
	}

	symbolIDs := make([]string, 0, len(pkgResult.AllSymbols))
	for _, sym := range pkgResult.AllSymbols {
		symbolIDs = append(symbolIDs, sym.NodeID)
	}

	metadata := map[string]string{
		"package_name": pkgResult.PackageName,
		"dir_path":     pkgResult.DirPath,
		"symbol_count": fmt.Sprintf("%d", len(pkgResult.AllSymbols)),
		"route_count":  fmt.Sprintf("%d", len(pkgResult.AllRoutes)),
	}
	if len(pkgResult.AllImports) > 0 {
		var imps []string
		for _, imp := range pkgResult.AllImports {
			if imp.Alias != "" {
				imps = append(imps, fmt.Sprintf("%s %s", imp.Alias, imp.Path))
			} else {
				imps = append(imps, imp.Path)
			}
		}
		metadata["imports"] = strings.Join(imps, ",")
	}
	if strings.HasSuffix(compName, ".go") {
		metadata["file_path"] = compName
	}

	comp := &core.ComponentNode{
		Name:        compName,
		Type:        compType,
		Language:    core.LangGo,
		SymbolNodes: symbolIDs,
		Trivia:      pkgResult.Trivia,
		Metadata:    metadata,
		Lineage:     lineage,
	}

	compID, err := core.HashComponentNode(comp)
	if err != nil {
		return nil, fmt.Errorf("failed to compute component node hash: %w", err)
	}
	comp.ComponentID = compID

	return comp, nil
}

func isBuiltinGoType(t string) bool {
	switch t {
	case "bool", "byte", "complex64", "complex128", "error", "float32", "float64",
		"int", "int8", "int16", "int32", "int64", "rune", "string",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "any":
		return true
	default:
		return false
	}
}
