package materialize

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cosmscm/cosm/pkg/codecs/cpp"
	"github.com/cosmscm/cosm/pkg/codecs/csharp"
	"github.com/cosmscm/cosm/pkg/codecs/dockerfile"
	"github.com/cosmscm/cosm/pkg/codecs/elixir"
	"github.com/cosmscm/cosm/pkg/codecs/golang"
	"github.com/cosmscm/cosm/pkg/codecs/graphql"
	"github.com/cosmscm/cosm/pkg/codecs/hcl"
	"github.com/cosmscm/cosm/pkg/codecs/java"
	"github.com/cosmscm/cosm/pkg/codecs/kotlin"
	"github.com/cosmscm/cosm/pkg/codecs/php"
	"github.com/cosmscm/cosm/pkg/codecs/protobuf"
	"github.com/cosmscm/cosm/pkg/codecs/ruby"
	"github.com/cosmscm/cosm/pkg/codecs/rust"
	"github.com/cosmscm/cosm/pkg/codecs/sql"
	"github.com/cosmscm/cosm/pkg/codecs/swift"
	"github.com/cosmscm/cosm/pkg/codecs/typescript"
	"github.com/cosmscm/cosm/pkg/codecs/wasm"
	"github.com/cosmscm/cosm/pkg/codecs/zig"
	"github.com/cosmscm/cosm/pkg/core"
)

// Hydrator reconstitutes raw AST symbol nodes back into clean, formatted source text
// for all supported polyglot languages.
type Hydrator struct {
	rustHydrator       *rust.RustHydrator
	javaHydrator       *java.JavaHydrator
	sqlHydrator        *sql.SQLHydrator
	protoHydrator      *protobuf.ProtobufHydrator
	cppHydrator        *cpp.CppHydrator
	swiftHydrator      *swift.SwiftHydrator
	kotlinHydrator     *kotlin.KotlinHydrator
	wasmHydrator       *wasm.WasmHydrator
	zigHydrator        *zig.ZigHydrator
	csharpHydrator     *csharp.CSharpHydrator
	graphqlHydrator    *graphql.GraphQLHydrator
	rubyHydrator       *ruby.RubyHydrator
	phpHydrator        *php.PHPHydrator
	elixirHydrator     *elixir.ElixirHydrator
	dockerfileHydrator *dockerfile.DockerfileHydrator
}

// NewHydrator returns a new Hydrator instance.
func NewHydrator() *Hydrator {
	return &Hydrator{
		rustHydrator:       rust.NewRustHydrator(),
		javaHydrator:       java.NewJavaHydrator(),
		sqlHydrator:        sql.NewSQLHydrator(),
		protoHydrator:      protobuf.NewProtobufHydrator(),
		cppHydrator:        cpp.NewCppHydrator(),
		swiftHydrator:      swift.NewSwiftHydrator(),
		kotlinHydrator:     kotlin.NewKotlinHydrator(),
		wasmHydrator:       wasm.NewWasmHydrator(),
		zigHydrator:        zig.NewZigHydrator(),
		csharpHydrator:     csharp.NewCSharpHydrator(),
		graphqlHydrator:    graphql.NewGraphQLHydrator(),
		rubyHydrator:       ruby.NewRubyHydrator(),
		phpHydrator:        php.NewPHPHydrator(),
		elixirHydrator:     elixir.NewElixirHydrator(),
		dockerfileHydrator: dockerfile.NewDockerfileHydrator(),
	}
}

// HydrateSymbol reconstitutes a single ASTSymbolNode into source text across all supported languages.
func (h *Hydrator) HydrateSymbol(node *core.ASTSymbolNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node cannot be nil")
	}

	switch node.Language {
	case core.LangGo:
		return h.hydrateGoSymbol(node)
	case core.LangHCL:
		return h.hydrateHCLSymbol(node)
	case core.LangTypeScript:
		return h.hydrateTSSymbol(node)
	case core.LangPython:
		return h.hydratePythonSymbol(node)
	case core.LangRust:
		return h.rustHydrator.HydrateSymbol(node)
	case core.LangJava:
		return h.javaHydrator.HydrateSymbol(node)
	case core.LangSQL:
		return h.sqlHydrator.HydrateSymbol(node)
	case core.LangProtobuf:
		return h.protoHydrator.HydrateSymbol(node)
	case core.LangCpp, core.LangC:
		return h.cppHydrator.HydrateSymbol(node)
	case core.LangSwift:
		return h.swiftHydrator.HydrateSymbol(node)
	case core.LangKotlin:
		return h.kotlinHydrator.HydrateSymbol(node)
	case core.LangWasm:
		return h.wasmHydrator.HydrateSymbol(node)
	case core.LangZig:
		return h.zigHydrator.HydrateSymbol(node)
	case core.LangCSharp:
		return h.csharpHydrator.HydrateSymbol(node)
	case core.LangGraphQL:
		return h.graphqlHydrator.HydrateSymbol(node)
	case core.LangRuby:
		return h.rubyHydrator.HydrateSymbol(node)
	case core.LangPHP:
		return h.phpHydrator.HydrateSymbol(node)
	case core.LangElixir:
		return h.elixirHydrator.HydrateSymbol(node)
	case core.LangDockerfile:
		return h.dockerfileHydrator.HydrateSymbol(node)
	default:
		if len(node.ASTPayload) > 0 {
			return string(node.ASTPayload), nil
		}
		return fmt.Sprintf("// Symbol: %s (%s)", node.Identifier, node.NodeType), nil
	}
}

// hydrateGoSymbol reconstitutes a Go symbol from its ASTSymbolNode.
func (h *Hydrator) hydrateGoSymbol(node *core.ASTSymbolNode) (string, error) {
	if len(node.ASTPayload) == 0 {
		return fmt.Sprintf("// Go Symbol %s\n", node.Identifier), nil
	}

	switch node.NodeType {
	case "StructDecl":
		var s golang.GoStructSymbol
		if err := json.Unmarshal(node.ASTPayload, &s); err != nil {
			return "", fmt.Errorf("failed to unmarshal GoStructSymbol: %w", err)
		}
		var sb strings.Builder
		if s.Doc != "" {
			sb.WriteString(fmt.Sprintf("// %s\n", s.Doc))
		}
		sb.WriteString(fmt.Sprintf("type %s struct {\n", s.Name))
		for _, f := range s.Fields {
			tagStr := ""
			if f.Tag != "" {
				tagStr = fmt.Sprintf(" `%s`", f.Tag)
			}
			sb.WriteString(fmt.Sprintf("\t%s %s%s\n", f.Name, f.Type, tagStr))
		}
		sb.WriteString("}\n")
		return sb.String(), nil

	case "InterfaceDecl":
		var iface golang.GoInterfaceSymbol
		if err := json.Unmarshal(node.ASTPayload, &iface); err != nil {
			return "", fmt.Errorf("failed to unmarshal GoInterfaceSymbol: %w", err)
		}
		var sb strings.Builder
		if iface.Doc != "" {
			sb.WriteString(fmt.Sprintf("// %s\n", iface.Doc))
		}
		sb.WriteString(fmt.Sprintf("type %s interface {\n", iface.Name))
		for _, m := range iface.Methods {
			var params []string
			for _, p := range m.Params {
				if p.Name != "" {
					params = append(params, fmt.Sprintf("%s %s", p.Name, p.Type))
				} else {
					params = append(params, p.Type)
				}
			}
			resStr := ""
			if len(m.Results) == 1 {
				resStr = " " + m.Results[0]
			} else if len(m.Results) > 1 {
				resStr = fmt.Sprintf(" (%s)", strings.Join(m.Results, ", "))
			}
			sb.WriteString(fmt.Sprintf("\t%s(%s)%s\n", m.Name, strings.Join(params, ", "), resStr))
		}
		sb.WriteString("}\n")
		return sb.String(), nil

	case "FunctionDecl":
		var fn golang.GoFuncSymbol
		if err := json.Unmarshal(node.ASTPayload, &fn); err != nil {
			return "", fmt.Errorf("failed to unmarshal GoFuncSymbol: %w", err)
		}
		var sb strings.Builder
		if fn.Doc != "" {
			sb.WriteString(fmt.Sprintf("// %s\n", fn.Doc))
		}
		recvStr := ""
		if fn.IsMethod && fn.ReceiverType != "" {
			if fn.ReceiverName != "" {
				recvStr = fmt.Sprintf("(%s %s) ", fn.ReceiverName, fn.ReceiverType)
			} else {
				recvStr = fmt.Sprintf("(%s) ", fn.ReceiverType)
			}
		}
		var params []string
		for _, p := range fn.Params {
			if p.Name != "" {
				params = append(params, fmt.Sprintf("%s %s", p.Name, p.Type))
			} else {
				params = append(params, p.Type)
			}
		}
		resStr := ""
		if len(fn.Results) == 1 {
			resStr = " " + fn.Results[0]
		} else if len(fn.Results) > 1 {
			resStr = fmt.Sprintf(" (%s)", strings.Join(fn.Results, ", "))
		}

		trimmedBody := strings.TrimSpace(fn.BodySource)
		if trimmedBody != "" {
			if strings.HasPrefix(trimmedBody, "func ") || strings.HasPrefix(trimmedBody, "func(") {
				sb.WriteString(trimmedBody + "\n")
			} else if strings.HasPrefix(trimmedBody, "{") {
				sb.WriteString(fmt.Sprintf("func %s%s(%s)%s %s\n", recvStr, fn.Name, strings.Join(params, ", "), resStr, trimmedBody))
			} else {
				sb.WriteString(fmt.Sprintf("func %s%s(%s)%s {\n%s\n}\n", recvStr, fn.Name, strings.Join(params, ", "), resStr, trimmedBody))
			}
		} else {
			sb.WriteString(fmt.Sprintf("func %s%s(%s)%s {\n\t// Reconstituted function\n}\n", recvStr, fn.Name, strings.Join(params, ", "), resStr))
		}
		return sb.String(), nil

	case "RouteBinding":
		var route golang.GoRouteBinding
		if err := json.Unmarshal(node.ASTPayload, &route); err != nil {
			return "", fmt.Errorf("failed to unmarshal GoRouteBinding: %w", err)
		}
		handlerName := strings.ReplaceAll(route.HandlerName, "\n", " ")
		return fmt.Sprintf("// Route Binding: %s %s -> %s\n", route.Method, route.Path, handlerName), nil

	default:
		return string(node.ASTPayload), nil
	}
}

// hydratePythonSymbol reconstitutes Python AST symbols.
func (h *Hydrator) hydratePythonSymbol(node *core.ASTSymbolNode) (string, error) {
	if len(node.ASTPayload) == 0 {
		return fmt.Sprintf("# Python Symbol %s\n", node.Identifier), nil
	}
	return string(node.ASTPayload), nil
}

// hydrateHCLSymbol reconstitutes an HCL block from an ASTSymbolNode.
func (h *Hydrator) hydrateHCLSymbol(node *core.ASTSymbolNode) (string, error) {
	if len(node.ASTPayload) == 0 {
		return fmt.Sprintf("# HCL Symbol %s\n", node.Identifier), nil
	}

	var block hcl.HCLBlock
	if err := json.Unmarshal(node.ASTPayload, &block); err != nil {
		return "", fmt.Errorf("failed to unmarshal HCLBlock: %w", err)
	}

	var sb strings.Builder
	h.renderHCLBlock(&block, &sb, 0)
	return sb.String(), nil
}

func (h *Hydrator) renderHCLBlock(b *hcl.HCLBlock, sb *strings.Builder, indentLevel int) {
	if b == nil {
		return
	}
	indent := strings.Repeat("  ", indentLevel)

	var labelStrs []string
	for _, l := range b.Labels {
		labelStrs = append(labelStrs, fmt.Sprintf("\"%s\"", l))
	}
	labelPart := ""
	if len(labelStrs) > 0 {
		labelPart = " " + strings.Join(labelStrs, " ")
	}

	sb.WriteString(fmt.Sprintf("%s%s%s {\n", indent, b.BlockType, labelPart))

	var attrKeys []string
	for k := range b.Attributes {
		attrKeys = append(attrKeys, k)
	}
	sort.Strings(attrKeys)

	for _, k := range attrKeys {
		attr := b.Attributes[k]
		sb.WriteString(fmt.Sprintf("%s  %s = %s\n", indent, k, attr.Value))
	}

	for _, nested := range b.NestedBlocks {
		h.renderHCLBlock(nested, sb, indentLevel+1)
	}

	sb.WriteString(fmt.Sprintf("%s}\n", indent))
}

// hydrateTSSymbol reconstitutes a TypeScript symbol from an ASTSymbolNode.
func (h *Hydrator) hydrateTSSymbol(node *core.ASTSymbolNode) (string, error) {
	if len(node.ASTPayload) == 0 {
		return fmt.Sprintf("// TS Symbol %s\n", node.Identifier), nil
	}

	switch node.NodeType {
	case "InterfaceDeclaration":
		var iface typescript.TSInterface
		if err := json.Unmarshal(node.ASTPayload, &iface); err != nil {
			return "", fmt.Errorf("failed to unmarshal TSInterface: %w", err)
		}
		var sb strings.Builder
		if iface.IsExported {
			sb.WriteString("export ")
		}
		sb.WriteString(fmt.Sprintf("interface %s {\n", iface.Name))
		var propKeys []string
		for k := range iface.Properties {
			propKeys = append(propKeys, k)
		}
		sort.Strings(propKeys)
		for _, k := range propKeys {
			sb.WriteString(fmt.Sprintf("  %s: %s;\n", k, iface.Properties[k]))
		}
		sb.WriteString("}\n")
		return sb.String(), nil

	case "TypeAliasDeclaration":
		var t typescript.TSTypeAlias
		if err := json.Unmarshal(node.ASTPayload, &t); err != nil {
			return "", fmt.Errorf("failed to unmarshal TSTypeAlias: %w", err)
		}
		exp := ""
		if t.IsExported {
			exp = "export "
		}
		return fmt.Sprintf("%stype %s = %s;\n", exp, t.Name, t.Definition), nil

	case "ReactComponent":
		var comp typescript.TSComponent
		if err := json.Unmarshal(node.ASTPayload, &comp); err != nil {
			return "", fmt.Errorf("failed to unmarshal TSComponent: %w", err)
		}
		var sb strings.Builder
		if comp.IsExported {
			sb.WriteString("export ")
		}
		propsStr := ""
		if comp.PropsType != "" {
			propsStr = fmt.Sprintf("props: %s", comp.PropsType)
		}
		sb.WriteString(fmt.Sprintf("const %s: React.FC<%s> = (%s) => {\n", comp.Name, comp.PropsType, propsStr))
		for _, hook := range comp.HooksUsed {
			sb.WriteString(fmt.Sprintf("  // Hook: %s\n", hook))
		}
		sb.WriteString("  return (\n    <div>\n")
		sb.WriteString(fmt.Sprintf("      <h1>%s</h1>\n", comp.Name))
		sb.WriteString("    </div>\n  );\n};\n")
		return sb.String(), nil

	case "ApiClientCall":
		var api typescript.TSAPICall
		if err := json.Unmarshal(node.ASTPayload, &api); err != nil {
			return "", fmt.Errorf("failed to unmarshal TSAPICall: %w", err)
		}
		return fmt.Sprintf("// API Client Request: %s %s via %s\n", api.Method, api.URL, api.Caller), nil

	default:
		return string(node.ASTPayload), nil
	}
}

// HydrateComponent reconstitutes all symbols within a ComponentNode into a map of relative file paths to content bytes.
func (h *Hydrator) HydrateComponent(comp *core.ComponentNode, symbolMap map[string]*core.ASTSymbolNode) (map[string][]byte, error) {
	if comp == nil {
		return nil, fmt.Errorf("component is nil")
	}

	files := make(map[string][]byte)

	switch comp.Language {
	case core.LangGo:
		mainFile := comp.Metadata["file_path"]
		if mainFile == "" {
			dirPath := comp.Metadata["dir_path"]
			if dirPath == "" {
				dirPath = fmt.Sprintf("services/%s", comp.Name)
			}
			mainFile = fmt.Sprintf("%s/main.go", strings.TrimSuffix(dirPath, "/"))
		}
		if !strings.HasSuffix(mainFile, ".go") {
			mainFile = fmt.Sprintf("%s/main.go", strings.TrimSuffix(mainFile, "/"))
		}
		pkgName := comp.Metadata["package_name"]
		if pkgName == "" {
			pkgName = "main"
		}

		var bodyBuilder strings.Builder
		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			code, err := h.HydrateSymbol(sym)
			if err != nil {
				return nil, err
			}
			bodyBuilder.WriteString(code)
			bodyBuilder.WriteString("\n")
		}

		bodyStr := bodyBuilder.String()
		importSet := make(map[string]bool)
		if impStr, ok := comp.Metadata["imports"]; ok && impStr != "" {
			for _, imp := range strings.Split(impStr, ",") {
				imp = strings.TrimSpace(imp)
				if imp != "" {
					if !strings.HasPrefix(imp, "\"") {
						imp = fmt.Sprintf("\"%s\"", imp)
					}
					importSet[imp] = true
				}
			}
		}

		candidates := map[string]string{
			"context.": "\"context\"",
			"json.":    "\"encoding/json\"",
			"fmt.":     "\"fmt\"",
			"http.":    "\"net/http\"",
			"os.":      "\"os\"",
			"time.":    "\"time\"",
			"io.":      "\"io\"",
			"strings.": "\"strings\"",
			"bytes.":   "\"bytes\"",
			"sync.":    "\"sync\"",
			"log.":     "\"log\"",
			"errors.":  "\"errors\"",
		}
		for pattern, imp := range candidates {
			if strings.Contains(bodyStr, pattern) {
				importSet[imp] = true
			}
		}
		var imports []string
		for imp := range importSet {
			imports = append(imports, imp)
		}
		sort.Strings(imports)

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("package %s\n\n", pkgName))
		if len(imports) > 0 {
			sb.WriteString("import (\n")
			for _, imp := range imports {
				sb.WriteString(fmt.Sprintf("\t%s\n", imp))
			}
			sb.WriteString(")\n\n")
		}
		sb.WriteString(bodyStr)

		files[mainFile] = []byte(sb.String())

	case core.LangHCL:
		filePath := comp.Metadata["file_path"]
		if filePath == "" {
			filePath = fmt.Sprintf("infra/%s/main.tf", comp.Name)
		}
		if !strings.HasSuffix(filePath, ".tf") {
			filePath = fmt.Sprintf("%s/main.tf", strings.TrimSuffix(filePath, "/"))
		}

		var sb strings.Builder
		sb.WriteString("# Terraform Infrastructure Configuration\n")
		sb.WriteString(fmt.Sprintf("# Component: %s\n\n", comp.Name))

		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			code, err := h.HydrateSymbol(sym)
			if err != nil {
				return nil, err
			}
			sb.WriteString(code)
			sb.WriteString("\n")
		}

		formatted, err := hcl.FormatHCL([]byte(sb.String()))
		if err == nil {
			files[filePath] = formatted
		} else {
			files[filePath] = []byte(sb.String())
		}

	case core.LangTypeScript:
		filePath := comp.Metadata["file_path"]
		if filePath == "" {
			filePath = fmt.Sprintf("frontend/src/%s.tsx", comp.Name)
		}

		var bodyBuilder strings.Builder
		hasReact := false
		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			if sym.NodeType == "ReactComponent" || strings.Contains(string(sym.ASTPayload), "React") {
				hasReact = true
			}
			code, err := h.HydrateSymbol(sym)
			if err != nil {
				return nil, err
			}
			bodyBuilder.WriteString(code)
			bodyBuilder.WriteString("\n")
		}

		var sb strings.Builder
		if hasReact {
			sb.WriteString("import React, { useState, useEffect } from 'react';\n\n")
		}
		sb.WriteString(bodyBuilder.String())
		files[filePath] = []byte(sb.String())

	case core.LangPython:
		filePath := comp.Metadata["file_path"]
		if filePath == "" {
			filePath = fmt.Sprintf("services/%s/main.py", comp.Name)
		}
		if !strings.HasSuffix(filePath, ".py") {
			filePath = fmt.Sprintf("%s/main.py", strings.TrimSuffix(filePath, "/"))
		}

		var bodyBuilder strings.Builder
		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			code, err := h.HydrateSymbol(sym)
			if err != nil {
				return nil, err
			}
			bodyBuilder.WriteString(code)
			bodyBuilder.WriteString("\n\n")
		}
		bodyStr := bodyBuilder.String()

		var sb strings.Builder
		sb.WriteString("# Python Service\n")
		sb.WriteString(fmt.Sprintf("# Component: %s\n\n", comp.Name))
		if (strings.Contains(bodyStr, "FastAPI") || strings.Contains(bodyStr, "@app.")) && !strings.Contains(bodyStr, "import FastAPI") {
			sb.WriteString("from fastapi import FastAPI\napp = FastAPI()\n\n")
		}
		sb.WriteString(bodyStr)

		files[filePath] = []byte(sb.String())

	case core.LangRust:
		filePath := comp.Metadata["file_path"]
		if filePath == "" {
			filePath = fmt.Sprintf("services/%s/src/lib.rs", comp.Name)
		}

		var sb strings.Builder
		sb.WriteString("// Rust Module\n\n")
		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			code, err := h.HydrateSymbol(sym)
			if err != nil {
				return nil, err
			}
			sb.WriteString(code)
			sb.WriteString("\n")
		}
		files[filePath] = []byte(sb.String())

	case core.LangJava:
		filePath := comp.Metadata["file_path"]
		if filePath == "" {
			filePath = fmt.Sprintf("src/main/java/%s.java", comp.Name)
		}

		var sb strings.Builder
		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			code, err := h.HydrateSymbol(sym)
			if err != nil {
				return nil, err
			}
			sb.WriteString(code)
			sb.WriteString("\n")
		}
		files[filePath] = []byte(sb.String())

	case core.LangSQL:
		filePath := comp.Metadata["file_path"]
		if filePath == "" {
			filePath = fmt.Sprintf("schema/%s.sql", comp.Name)
		}

		var sb strings.Builder
		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			code, err := h.HydrateSymbol(sym)
			if err != nil {
				return nil, err
			}
			sb.WriteString(code)
			sb.WriteString("\n")
		}
		files[filePath] = []byte(sb.String())

	case core.LangProtobuf:
		filePath := comp.Metadata["file_path"]
		if filePath == "" {
			filePath = fmt.Sprintf("proto/%s.proto", comp.Name)
		}

		var sb strings.Builder
		sb.WriteString("syntax = \"proto3\";\n\n")
		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			code, err := h.HydrateSymbol(sym)
			if err != nil {
				return nil, err
			}
			sb.WriteString(code)
			sb.WriteString("\n")
		}
		files[filePath] = []byte(sb.String())

	case core.LangCpp, core.LangC:
		filePath := comp.Metadata["file_path"]
		if filePath == "" {
			filePath = fmt.Sprintf("src/%s.cpp", comp.Name)
		}

		var sb strings.Builder
		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			code, err := h.HydrateSymbol(sym)
			if err != nil {
				return nil, err
			}
			sb.WriteString(code)
			sb.WriteString("\n")
		}
		files[filePath] = []byte(sb.String())

	case core.LangSwift:
		filePath := comp.Metadata["file_path"]
		if filePath == "" {
			filePath = fmt.Sprintf("Sources/%s.swift", comp.Name)
		}

		var sb strings.Builder
		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			code, err := h.HydrateSymbol(sym)
			if err != nil {
				return nil, err
			}
			sb.WriteString(code)
			sb.WriteString("\n")
		}
		files[filePath] = []byte(sb.String())

	case core.LangKotlin:
		filePath := comp.Metadata["file_path"]
		if filePath == "" {
			filePath = fmt.Sprintf("src/main/kotlin/%s.kt", comp.Name)
		}

		var sb strings.Builder
		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			code, err := h.HydrateSymbol(sym)
			if err != nil {
				return nil, err
			}
			sb.WriteString(code)
			sb.WriteString("\n")
		}
		files[filePath] = []byte(sb.String())

	case core.LangRuby:
		filePath := comp.Metadata["file_path"]
		if filePath == "" {
			filePath = fmt.Sprintf("lib/%s.rb", comp.Name)
		}
		var sb strings.Builder
		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			code, err := h.HydrateSymbol(sym)
			if err != nil {
				return nil, err
			}
			sb.WriteString(code)
			sb.WriteString("\n")
		}
		files[filePath] = []byte(sb.String())

	case core.LangPHP:
		filePath := comp.Metadata["file_path"]
		if filePath == "" {
			filePath = fmt.Sprintf("src/%s.php", comp.Name)
		}
		var sb strings.Builder
		sb.WriteString("<?php\n\n")
		if ns := comp.Metadata["namespace"]; ns != "" {
			sb.WriteString(fmt.Sprintf("namespace %s;\n\n", ns))
		}
		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			code, err := h.HydrateSymbol(sym)
			if err != nil {
				return nil, err
			}
			sb.WriteString(code)
			sb.WriteString("\n")
		}
		files[filePath] = []byte(sb.String())

	case core.LangElixir:
		filePath := comp.Metadata["file_path"]
		if filePath == "" {
			filePath = fmt.Sprintf("lib/%s.ex", comp.Name)
		}
		var sb strings.Builder
		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			code, err := h.HydrateSymbol(sym)
			if err != nil {
				return nil, err
			}
			sb.WriteString(code)
			sb.WriteString("\n")
		}
		files[filePath] = []byte(sb.String())

	case core.LangCSharp:
		filePath := comp.Metadata["file_path"]
		if filePath == "" {
			filePath = fmt.Sprintf("src/%s.cs", comp.Name)
		}
		var sb strings.Builder
		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			code, err := h.HydrateSymbol(sym)
			if err != nil {
				return nil, err
			}
			sb.WriteString(code)
			sb.WriteString("\n")
		}
		files[filePath] = []byte(sb.String())

	case core.LangGraphQL:
		filePath := comp.Metadata["file_path"]
		if filePath == "" {
			filePath = fmt.Sprintf("schema/%s.graphql", comp.Name)
		}
		var sb strings.Builder
		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			code, err := h.HydrateSymbol(sym)
			if err != nil {
				return nil, err
			}
			sb.WriteString(code)
			sb.WriteString("\n")
		}
		files[filePath] = []byte(sb.String())

	case core.LangWasm:
		filePath := comp.Metadata["file_path"]
		if filePath == "" {
			filePath = fmt.Sprintf("wasm/%s.wat", comp.Name)
		}
		var sb strings.Builder
		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			code, err := h.HydrateSymbol(sym)
			if err != nil {
				return nil, err
			}
			sb.WriteString(code)
			sb.WriteString("\n")
		}
		files[filePath] = []byte(sb.String())

	case core.LangZig:
		filePath := comp.Metadata["file_path"]
		if filePath == "" {
			filePath = fmt.Sprintf("src/%s.zig", comp.Name)
		}
		var sb strings.Builder
		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			code, err := h.HydrateSymbol(sym)
			if err != nil {
				return nil, err
			}
			sb.WriteString(code)
			sb.WriteString("\n")
		}
		files[filePath] = []byte(sb.String())

	case core.LangDockerfile:
		filePath := comp.Metadata["file_path"]
		if filePath == "" {
			filePath = "Dockerfile"
		}
		var sb strings.Builder
		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			code, err := h.HydrateSymbol(sym)
			if err != nil {
				return nil, err
			}
			sb.WriteString(code)
		}
		files[filePath] = []byte(sb.String())

	case core.LangRaw:
		targetPath := comp.Metadata["file_path"]
		if targetPath == "" {
			targetPath = comp.Name
		}
		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			if sym.Identifier != "" {
				targetPath = sym.Identifier
			}
			files[targetPath] = sym.ASTPayload
		}

	default:
		defaultFile := fmt.Sprintf("src/%s.txt", comp.Name)
		var sb strings.Builder
		for _, symID := range comp.SymbolNodes {
			sym, ok := symbolMap[symID]
			if !ok {
				continue
			}
			code, _ := h.HydrateSymbol(sym)
			sb.WriteString(code)
			sb.WriteString("\n")
		}
		files[defaultFile] = []byte(sb.String())
	}

	return files, nil
}

// HydrateWorkspace reconstitutes all components in a WorkspaceManifestNode into a unified virtual file tree map.
func (h *Hydrator) HydrateWorkspace(
	manifest *core.WorkspaceManifestNode,
	compMap map[string]*core.ComponentNode,
	symbolMap map[string]*core.ASTSymbolNode,
) (map[string][]byte, error) {
	if manifest == nil {
		return nil, fmt.Errorf("manifest is nil")
	}

	allFiles := make(map[string][]byte)

	for _, compID := range manifest.Components {
		comp, ok := compMap[compID]
		if !ok {
			continue
		}
		compFiles, err := h.HydrateComponent(comp, symbolMap)
		if err != nil {
			return nil, fmt.Errorf("failed to hydrate component %s: %w", comp.Name, err)
		}
		for path, content := range compFiles {
			allFiles[path] = content
		}
	}

	return allFiles, nil
}
