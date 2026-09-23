package golang

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// GoParser provides Go AST parsing, symbol extraction, and route analysis.
type GoParser struct {
	fset *token.FileSet
}

// NewGoParser creates an initialized GoParser instance.
func NewGoParser() *GoParser {
	return &GoParser{
		fset: token.NewFileSet(),
	}
}

// ParseSource parses a single in-memory Go source code buffer and extracts all symbols.
func (p *GoParser) ParseSource(filename string, src []byte, lineage core.LineageEnvelope) (*GoPackageResult, error) {
	if filename == "" {
		filename = "source.go"
	}

	fileNode, err := parser.ParseFile(p.fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Go source %s: %w", filename, err)
	}

	fileSymbols, err := p.ExtractSymbols(fileNode, src)
	if err != nil {
		return nil, fmt.Errorf("failed to extract symbols from %s: %w", filename, err)
	}
	fileSymbols.FilePath = filename
	fileSymbols.Trivia = ExtractTrivia(src)

	pkgResult := &GoPackageResult{
		PackageName: fileSymbols.PackageName,
		DirPath:     filepath.Dir(filename),
		Files: map[string]*GoFileSymbols{
			filename: fileSymbols,
		},
		AllRoutes:  fileSymbols.Routes,
		AllImports: fileSymbols.Imports,
		Trivia:     fileSymbols.Trivia,
	}

	// Convert file symbols to core.ASTSymbolNodes
	for _, s := range fileSymbols.Structs {
		node, err := StructToASTSymbolNode(s, fileSymbols.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		pkgResult.AllSymbols = append(pkgResult.AllSymbols, node)
	}

	for _, iface := range fileSymbols.Interfaces {
		node, err := InterfaceToASTSymbolNode(iface, fileSymbols.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		pkgResult.AllSymbols = append(pkgResult.AllSymbols, node)
	}

	for _, fn := range fileSymbols.Functions {
		node, err := FuncToASTSymbolNode(fn, fileSymbols.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		pkgResult.AllSymbols = append(pkgResult.AllSymbols, node)
	}

	for _, r := range fileSymbols.Routes {
		node, err := RouteToASTSymbolNode(r, fileSymbols.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		pkgResult.AllSymbols = append(pkgResult.AllSymbols, node)
	}

	for _, decl := range fileSymbols.GeneralDecls {
		node, err := GeneralDeclToASTSymbolNode(decl, fileSymbols.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		pkgResult.AllSymbols = append(pkgResult.AllSymbols, node)
	}

	return pkgResult, nil
}

// ParseFile parses a Go source file from disk.
func (p *GoParser) ParseFile(filePath string, lineage core.LineageEnvelope) (*GoPackageResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}
	return p.ParseSource(filePath, data, lineage)
}

// ParseDir parses all Go files in a directory belonging to the primary package.
func (p *GoParser) ParseDir(dirPath string, lineage core.LineageEnvelope) (*GoPackageResult, error) {
	pkgs, err := parser.ParseDir(p.fset, dirPath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse directory %s: %w", dirPath, err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no Go packages found in %s", dirPath)
	}

	// Select primary non-test package
	var targetPkg *ast.Package
	for name, pkg := range pkgs {
		if !strings.HasSuffix(name, "_test") {
			targetPkg = pkg
			break
		}
	}
	if targetPkg == nil {
		for _, pkg := range pkgs {
			targetPkg = pkg
			break
		}
	}

	pkgResult := &GoPackageResult{
		PackageName: targetPkg.Name,
		DirPath:     dirPath,
		Files:       make(map[string]*GoFileSymbols),
	}

	for filePath, fileNode := range targetPkg.Files {
		src, _ := os.ReadFile(filePath)
		fileSymbols, err := p.ExtractSymbols(fileNode, src)
		if err != nil {
			return nil, fmt.Errorf("failed to extract symbols from %s: %w", filePath, err)
		}
		fileSymbols.FilePath = filePath
		fileSymbols.Trivia = ExtractTrivia(src)
		if pkgResult.Trivia == nil && fileSymbols.Trivia != nil {
			pkgResult.Trivia = fileSymbols.Trivia
		}
		pkgResult.Files[filePath] = fileSymbols

		pkgResult.AllRoutes = append(pkgResult.AllRoutes, fileSymbols.Routes...)
		pkgResult.AllImports = append(pkgResult.AllImports, fileSymbols.Imports...)

		for _, s := range fileSymbols.Structs {
			node, err := StructToASTSymbolNode(s, targetPkg.Name, lineage)
			if err != nil {
				return nil, err
			}
			pkgResult.AllSymbols = append(pkgResult.AllSymbols, node)
		}

		for _, iface := range fileSymbols.Interfaces {
			node, err := InterfaceToASTSymbolNode(iface, targetPkg.Name, lineage)
			if err != nil {
				return nil, err
			}
			pkgResult.AllSymbols = append(pkgResult.AllSymbols, node)
		}

		for _, fn := range fileSymbols.Functions {
			node, err := FuncToASTSymbolNode(fn, targetPkg.Name, lineage)
			if err != nil {
				return nil, err
			}
			pkgResult.AllSymbols = append(pkgResult.AllSymbols, node)
		}

		for _, r := range fileSymbols.Routes {
			node, err := RouteToASTSymbolNode(r, targetPkg.Name, lineage)
			if err != nil {
				return nil, err
			}
			pkgResult.AllSymbols = append(pkgResult.AllSymbols, node)
		}

		for _, decl := range fileSymbols.GeneralDecls {
			node, err := GeneralDeclToASTSymbolNode(decl, targetPkg.Name, lineage)
			if err != nil {
				return nil, err
			}
			pkgResult.AllSymbols = append(pkgResult.AllSymbols, node)
		}
	}

	return pkgResult, nil
}

// ExtractSymbols inspects an AST File and extracts structs, interfaces, functions, routes, imports, and env lookups.
func (p *GoParser) ExtractSymbols(fileNode *ast.File, src []byte) (*GoFileSymbols, error) {
	syms := &GoFileSymbols{
		PackageName: fileNode.Name.Name,
	}

	// 1. Extract Imports
	for _, imp := range fileNode.Imports {
		pathVal := strings.Trim(imp.Path.Value, `"`)
		aliasVal := ""
		if imp.Name != nil {
			aliasVal = imp.Name.Name
		}
		syms.Imports = append(syms.Imports, GoImport{
			Path:  pathVal,
			Alias: aliasVal,
		})
	}

	// 2. Walk AST Declarations
	for _, decl := range fileNode.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			switch d.Tok {
			case token.CONST, token.VAR:
				start := p.fset.Position(d.Pos()).Offset
				end := p.fset.Position(d.End()).Offset
				if d.Doc != nil {
					start = p.fset.Position(d.Doc.Pos()).Offset
				}
				if start >= 0 && end <= len(src) && start < end {
					snippet := strings.TrimSpace(string(src[start:end]))
					kind := "ConstDecl"
					if d.Tok == token.VAR {
						kind = "VarDecl"
					}
					name := fmt.Sprintf("%s_%d", strings.ToLower(kind), d.Pos())
					if len(d.Specs) > 0 {
						if vs, ok := d.Specs[0].(*ast.ValueSpec); ok && len(vs.Names) > 0 {
							name = vs.Names[0].Name
						}
					}
					doc := ""
					if d.Doc != nil {
						doc = strings.TrimSpace(d.Doc.Text())
					}
					syms.GeneralDecls = append(syms.GeneralDecls, GoGeneralDecl{
						Name:       name,
						Kind:       kind,
						SourceCode: snippet,
						Doc:        doc,
					})
				}
			case token.TYPE:
				for _, spec := range d.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					doc := ""
					if d.Doc != nil {
						doc = strings.TrimSpace(d.Doc.Text())
					}
					if typeSpec.Doc != nil {
						doc = strings.TrimSpace(typeSpec.Doc.Text())
					}

					switch t := typeSpec.Type.(type) {
					case *ast.StructType:
						structSym := p.extractStruct(typeSpec.Name.Name, doc, t)
						syms.Structs = append(syms.Structs, structSym)
					case *ast.InterfaceType:
						ifaceSym := p.extractInterface(typeSpec.Name.Name, doc, t)
						syms.Interfaces = append(syms.Interfaces, ifaceSym)
					default:
						start := p.fset.Position(d.Pos()).Offset
						end := p.fset.Position(d.End()).Offset
						if d.Doc != nil {
							start = p.fset.Position(d.Doc.Pos()).Offset
						}
						if start >= 0 && end <= len(src) && start < end {
							snippet := strings.TrimSpace(string(src[start:end]))
							syms.GeneralDecls = append(syms.GeneralDecls, GoGeneralDecl{
								Name:       typeSpec.Name.Name,
								Kind:       "TypeDecl",
								SourceCode: snippet,
								Doc:        doc,
							})
						}
					}
				}
			}

		case *ast.FuncDecl:
			fnSym := p.extractFunction(d, src)
			syms.Functions = append(syms.Functions, fnSym)
		}
	}

	// 3. Extract Route Bindings & Env Vars
	syms.Routes = p.extractRoutes(fileNode)
	syms.EnvVarsAccessed = p.extractEnvVars(fileNode)

	return syms, nil
}

func (p *GoParser) extractStruct(name, doc string, st *ast.StructType) GoStructSymbol {
	var fields []GoField
	if st.Fields != nil {
		for _, f := range st.Fields.List {
			fieldType := p.typeToString(f.Type)
			tag := ""
			if f.Tag != nil {
				tag = strings.Trim(f.Tag.Value, "`")
			}
			if len(f.Names) == 0 {
				// Embedded field
				fields = append(fields, GoField{
					Name: fieldType,
					Type: fieldType,
					Tag:  tag,
				})
			} else {
				for _, fName := range f.Names {
					fields = append(fields, GoField{
						Name: fName.Name,
						Type: fieldType,
						Tag:  tag,
					})
				}
			}
		}
	}
	return GoStructSymbol{
		Name:   name,
		Doc:    doc,
		Fields: fields,
	}
}

func (p *GoParser) extractInterface(name, doc string, it *ast.InterfaceType) GoInterfaceSymbol {
	var methods []GoInterfaceMethod
	if it.Methods != nil {
		for _, m := range it.Methods.List {
			if len(m.Names) == 0 {
				continue
			}
			funcType, ok := m.Type.(*ast.FuncType)
			if !ok {
				continue
			}
			var params []GoFuncParam
			if funcType.Params != nil {
				for _, pItem := range funcType.Params.List {
					pType := p.typeToString(pItem.Type)
					for _, pName := range pItem.Names {
						params = append(params, GoFuncParam{
							Name: pName.Name,
							Type: pType,
						})
					}
				}
			}
			var results []string
			if funcType.Results != nil {
				for _, rItem := range funcType.Results.List {
					results = append(results, p.typeToString(rItem.Type))
				}
			}
			methods = append(methods, GoInterfaceMethod{
				Name:    m.Names[0].Name,
				Params:  params,
				Results: results,
			})
		}
	}
	return GoInterfaceSymbol{
		Name:    name,
		Doc:     doc,
		Methods: methods,
	}
}

func (p *GoParser) extractFunction(fn *ast.FuncDecl, src []byte) GoFuncSymbol {
	doc := ""
	if fn.Doc != nil {
		doc = strings.TrimSpace(fn.Doc.Text())
	}

	var receiverType, receiverName string
	isMethod := false
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		isMethod = true
		r := fn.Recv.List[0]
		receiverType = p.typeToString(r.Type)
		if len(r.Names) > 0 {
			receiverName = r.Names[0].Name
		}
	}

	var params []GoFuncParam
	if fn.Type.Params != nil {
		for _, param := range fn.Type.Params.List {
			pType := p.typeToString(param.Type)
			if len(param.Names) == 0 {
				params = append(params, GoFuncParam{
					Name: "",
					Type: pType,
				})
			} else {
				for _, name := range param.Names {
					params = append(params, GoFuncParam{
						Name: name.Name,
						Type: pType,
					})
				}
			}
		}
	}

	var results []string
	if fn.Type.Results != nil {
		for _, res := range fn.Type.Results.List {
			results = append(results, p.typeToString(res.Type))
		}
	}

	var calledSymbols []string
	var envVars []string
	if fn.Body != nil {
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					calledSymbols = append(calledSymbols, fun.Name)
				case *ast.SelectorExpr:
					if ident, ok := fun.X.(*ast.Ident); ok {
						calledSymbols = append(calledSymbols, ident.Name+"."+fun.Sel.Name)
						if ident.Name == "os" && (fun.Sel.Name == "Getenv" || fun.Sel.Name == "LookupEnv") {
							if len(call.Args) > 0 {
								if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
									envVar, _ := strconv.Unquote(lit.Value)
									envVars = append(envVars, envVar)
								}
							}
						}
					} else {
						calledSymbols = append(calledSymbols, fun.Sel.Name)
					}
				}
			}
			return true
		})
	}

	// Capture snippet of body source
	var bodySource string
	if len(src) > 0 && fn.Body != nil {
		start := p.fset.Position(fn.Pos()).Offset
		end := p.fset.Position(fn.End()).Offset
		if start >= 0 && end <= len(src) && start < end {
			bodySource = string(src[start:end])
		}
	}

	return GoFuncSymbol{
		Name:            fn.Name.Name,
		ReceiverType:    receiverType,
		ReceiverName:    receiverName,
		IsMethod:        isMethod,
		Params:          params,
		Results:         results,
		Doc:             doc,
		BodySource:      bodySource,
		IsExported:      ast.IsExported(fn.Name.Name),
		CalledSymbols:   calledSymbols,
		EnvVarsAccessed: envVars,
	}
}

// extractRoutes finds HTTP/gRPC route registration calls like http.HandleFunc, r.Get, router.POST, etc.
func (p *GoParser) extractRoutes(fileNode *ast.File) []GoRouteBinding {
	var routes []GoRouteBinding

	httpMethodRegex := regexp.MustCompile(`^(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD|HandleFunc|Handle|Route)$`)

	ast.Inspect(fileNode, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		funcName := sel.Sel.Name
		upperFunc := strings.ToUpper(funcName)

		// Check for gRPC registration: Register<Service>Server(s, &server{})
		if strings.HasPrefix(funcName, "Register") && strings.HasSuffix(funcName, "Server") {
			line := p.fset.Position(call.Pos()).Line
			routes = append(routes, GoRouteBinding{
				Method:      "GRPC",
				Path:        funcName,
				HandlerName: p.exprToString(call.Args[len(call.Args)-1]),
				Framework:   "grpc",
				LineNumber:  line,
			})
			return true
		}

		// Check for HTTP route registrations: http.HandleFunc("/api/...", handler) or router.GET("/api/...", handler)
		if httpMethodRegex.MatchString(upperFunc) || httpMethodRegex.MatchString(funcName) {
			if len(call.Args) >= 1 {
				var routePath string
				if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					routePath, _ = strconv.Unquote(lit.Value)
				}

				if routePath != "" {
					method := "ANY"
					framework := "stdlib"
					switch upperFunc {
					case "GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD":
						method = upperFunc
						framework = "mux/router"
					case "HANDLEFUNC", "HANDLE":
						method = "ANY"
						framework = "stdlib"
					}

					var handlerName string
					if len(call.Args) >= 2 {
						if _, isFuncLit := call.Args[1].(*ast.FuncLit); isFuncLit {
							handlerName = "inline_handler"
						} else {
							handlerName = p.exprToString(call.Args[1])
						}
					}

					line := p.fset.Position(call.Pos()).Line
					routes = append(routes, GoRouteBinding{
						Method:      method,
						Path:        routePath,
						HandlerName: handlerName,
						Framework:   framework,
						LineNumber:  line,
					})
				}
			}
		}

		return true
	})

	return routes
}

// extractEnvVars extracts all os.Getenv and os.LookupEnv calls in the file.
func (p *GoParser) extractEnvVars(fileNode *ast.File) []string {
	var envs []string
	seen := make(map[string]bool)

	ast.Inspect(fileNode, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if ident.Name == "os" && (sel.Sel.Name == "Getenv" || sel.Sel.Name == "LookupEnv") {
			if len(call.Args) > 0 {
				if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					val, _ := strconv.Unquote(lit.Value)
					if val != "" && !seen[val] {
						seen[val] = true
						envs = append(envs, val)
					}
				}
			}
		}
		return true
	})

	return envs
}

func (p *GoParser) typeToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	var buf bytes.Buffer
	_ = printer.Fprint(&buf, p.fset, expr)
	return buf.String()
}

func (p *GoParser) exprToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	var buf bytes.Buffer
	_ = printer.Fprint(&buf, p.fset, expr)
	return buf.String()
}
