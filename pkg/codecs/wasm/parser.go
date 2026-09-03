package wasm

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// WasmParam represents a parameter in a WASM function or type.
type WasmParam struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type"`
}

// WasmLocal represents a local variable in a WASM function.
type WasmLocal struct {
	Name  string `json:"name,omitempty"`
	Type  string `json:"type"`
	Count uint32 `json:"count,omitempty"`
}

// WasmType represents a type definition `(type $name (func (param ...) (result ...)))`.
type WasmType struct {
	ID      string      `json:"id"`
	Params  []WasmParam `json:"params,omitempty"`
	Results []string    `json:"results,omitempty"`
	Doc     string      `json:"doc,omitempty"`
}

// WasmImport represents an imported symbol in a WASM module.
type WasmImport struct {
	Module        string      `json:"module"`
	Field         string      `json:"field"`
	Kind          string      `json:"kind"` // func, table, memory, global
	Name          string      `json:"name,omitempty"`
	TypeRef       string      `json:"type_ref,omitempty"`
	Params        []WasmParam `json:"params,omitempty"`
	Results       []string    `json:"results,omitempty"`
	MemoryMin     uint32      `json:"memory_min,omitempty"`
	MemoryMax     uint32      `json:"memory_max,omitempty"`
	TableMin      uint32      `json:"table_min,omitempty"`
	TableMax      uint32      `json:"table_max,omitempty"`
	TableElemType string      `json:"table_elem_type,omitempty"`
	GlobalType    string      `json:"global_type,omitempty"`
	GlobalMut     bool        `json:"global_mut,omitempty"`
	Doc           string      `json:"doc,omitempty"`
}

// WasmFunction represents a function in a WASM module.
type WasmFunction struct {
	Name         string      `json:"name"`
	ExportNames  []string    `json:"export_names,omitempty"`
	TypeRef      string      `json:"type_ref,omitempty"`
	Params       []WasmParam `json:"params,omitempty"`
	Results      []string    `json:"results,omitempty"`
	Locals       []WasmLocal `json:"locals,omitempty"`
	Instructions []string    `json:"instructions,omitempty"`
	BodySource   string      `json:"body_source,omitempty"`
	CalledFuncs  []string    `json:"called_funcs,omitempty"`
	Doc          string      `json:"doc,omitempty"`
}

// WasmMemory represents a memory declaration in a WASM module.
type WasmMemory struct {
	Name        string   `json:"name,omitempty"`
	ExportNames []string `json:"export_names,omitempty"`
	Min         uint32   `json:"min"`
	Max         uint32   `json:"max,omitempty"`
	Shared      bool     `json:"shared,omitempty"`
	Is64        bool     `json:"is_64,omitempty"`
	Doc         string   `json:"doc,omitempty"`
}

// WasmTable represents a table declaration in a WASM module.
type WasmTable struct {
	Name        string   `json:"name,omitempty"`
	ExportNames []string `json:"export_names,omitempty"`
	Min         uint32   `json:"min"`
	Max         uint32   `json:"max,omitempty"`
	ElemType    string   `json:"elem_type"` // funcref, externref
	Doc         string   `json:"doc,omitempty"`
}

// WasmGlobal represents a global variable in a WASM module.
type WasmGlobal struct {
	Name        string   `json:"name,omitempty"`
	ExportNames []string `json:"export_names,omitempty"`
	Type        string   `json:"type"`
	Mutable     bool     `json:"mutable"`
	InitExpr    string   `json:"init_expr"`
	Doc         string   `json:"doc,omitempty"`
}

// WasmExport represents an explicit export directive in a WASM module.
type WasmExport struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"` // func, memory, table, global
	Target string `json:"target"`
	Doc    string `json:"doc,omitempty"`
}

// WasmDataSegment represents an embedded data segment.
type WasmDataSegment struct {
	Name        string `json:"name,omitempty"`
	MemoryIndex uint32 `json:"memory_index,omitempty"`
	OffsetExpr  string `json:"offset_expr,omitempty"`
	Data        string `json:"data"` // string or hex encoded
	Passive     bool   `json:"passive,omitempty"`
	Doc         string `json:"doc,omitempty"`
}

// WasmElemSegment represents a table element segment.
type WasmElemSegment struct {
	Name       string   `json:"name,omitempty"`
	TableIndex uint32   `json:"table_index,omitempty"`
	OffsetExpr string   `json:"offset_expr,omitempty"`
	Funcs      []string `json:"funcs,omitempty"`
	Doc        string   `json:"doc,omitempty"`
}

// WitRecordField represents a field in a WIT record.
type WitRecordField struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Doc  string `json:"doc,omitempty"`
}

// WitRecord represents a record definition in WIT (WebAssembly Interface Types).
type WitRecord struct {
	Name   string           `json:"name"`
	Fields []WitRecordField `json:"fields"`
	Doc    string           `json:"doc,omitempty"`
}

// WitVariantCase represents a case in a WIT variant.
type WitVariantCase struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
	Doc  string `json:"doc,omitempty"`
}

// WitVariant represents a variant definition in WIT.
type WitVariant struct {
	Name  string           `json:"name"`
	Cases []WitVariantCase `json:"cases"`
	Doc   string           `json:"doc,omitempty"`
}

// WitEnum represents an enum definition in WIT.
type WitEnum struct {
	Name  string   `json:"name"`
	Cases []string `json:"cases"`
	Doc   string   `json:"doc,omitempty"`
}

// WitFunc represents an exported or imported function in a WIT interface.
type WitFunc struct {
	Name    string      `json:"name"`
	Params  []WasmParam `json:"params,omitempty"`
	Results []string    `json:"results,omitempty"`
	Doc     string      `json:"doc,omitempty"`
}

// WitInterface represents a WIT interface definition.
type WitInterface struct {
	Package   string       `json:"package,omitempty"`
	Name      string       `json:"name"`
	Records   []WitRecord  `json:"records,omitempty"`
	Variants  []WitVariant `json:"variants,omitempty"`
	Enums     []WitEnum    `json:"enums,omitempty"`
	Functions []WitFunc    `json:"functions,omitempty"`
	Doc       string       `json:"doc,omitempty"`
}

// WitWorld represents a WIT world definition.
type WitWorld struct {
	Package string   `json:"package,omitempty"`
	Name    string   `json:"name"`
	Imports []string `json:"imports,omitempty"`
	Exports []string `json:"exports,omitempty"`
	Doc     string   `json:"doc,omitempty"`
}

// WasmFileResult contains all extracted symbols and metadata from a WASM/WAT/WIT artifact.
type WasmFileResult struct {
	FilePath      string                `json:"file_path"`
	PackageName   string                `json:"package_name"`
	IsBinary      bool                  `json:"is_binary"`
	ModuleName    string                `json:"module_name,omitempty"`
	Types         []WasmType            `json:"types,omitempty"`
	Imports       []WasmImport          `json:"imports,omitempty"`
	Functions     []WasmFunction        `json:"functions,omitempty"`
	Tables        []WasmTable           `json:"tables,omitempty"`
	Memories      []WasmMemory          `json:"memories,omitempty"`
	Globals       []WasmGlobal          `json:"globals,omitempty"`
	Exports       []WasmExport          `json:"exports,omitempty"`
	DataSegments  []WasmDataSegment     `json:"data_segments,omitempty"`
	ElemSegments  []WasmElemSegment     `json:"elem_segments,omitempty"`
	WitInterfaces []WitInterface        `json:"wit_interfaces,omitempty"`
	WitWorlds     []WitWorld            `json:"wit_worlds,omitempty"`
	AllSymbols    []*core.ASTSymbolNode `json:"all_symbols"`
}

// WasmParser parses WASM binary (.wasm), WAT text (.wat), and WIT interface (.wit) files.
type WasmParser struct{}

// NewWasmParser creates a new WasmParser.
func NewWasmParser() *WasmParser {
	return &WasmParser{}
}

// ParseSource parses WebAssembly bytes (binary .wasm or text .wat / .wit) and extracts AST symbols.
func (p *WasmParser) ParseSource(filename string, src []byte, lineage core.LineageEnvelope) (*WasmFileResult, error) {
	if filename == "" {
		filename = "module.wat"
	}

	result := &WasmFileResult{
		FilePath:    filename,
		PackageName: p.derivePackageName(filename),
	}

	// 1. Detect WASM Binary Header: \x00asm (0x00 0x61 0x73 0x6D)
	if isWasmBinary(src) {
		result.IsBinary = true
		if err := p.parseBinary(src, result); err != nil {
			return nil, fmt.Errorf("parsing WASM binary %s: %w", filename, err)
		}
	} else if isWitSource(src, filename) {
		// 2. Parse WIT interface definition
		if err := p.parseWit(src, result); err != nil {
			return nil, fmt.Errorf("parsing WIT %s: %w", filename, err)
		}
	} else {
		// 3. Parse WAT S-expression text format
		if err := p.parseWat(src, result); err != nil {
			return nil, fmt.Errorf("parsing WAT %s: %w", filename, err)
		}
	}

	// Convert all extracted entities to ASTSymbolNodes
	symbols, err := p.toSymbolNodes(result, lineage)
	if err != nil {
		return nil, fmt.Errorf("converting WASM to ASTSymbolNodes: %w", err)
	}
	result.AllSymbols = symbols

	return result, nil
}

// ParseFile parses a WASM, WAT, or WIT file from disk.
func (p *WasmParser) ParseFile(filePath string, lineage core.LineageEnvelope) (*WasmFileResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filePath, err)
	}
	return p.ParseSource(filePath, data, lineage)
}

// ParseDir parses all .wasm, .wat, and .wit files in a directory.
func (p *WasmParser) ParseDir(dirPath string, lineage core.LineageEnvelope) ([]*WasmFileResult, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dirPath, err)
	}

	var results []*WasmFileResult
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext == ".wasm" || ext == ".wat" || ext == ".wit" {
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

func (p *WasmParser) derivePackageName(filename string) string {
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	if name == "" || name == "module" || name == "main" || name == "lib" {
		dir := filepath.Base(filepath.Dir(filename))
		if dir != "." && dir != "/" && dir != "" {
			return dir
		}
		return "wasm"
	}
	return name
}

func isWasmBinary(src []byte) bool {
	return len(src) >= 4 && src[0] == 0x00 && src[1] == 0x61 && src[2] == 0x73 && src[3] == 0x6D
}

func isWitSource(src []byte, filename string) bool {
	if strings.HasSuffix(filename, ".wit") {
		return true
	}
	text := string(src)
	return strings.Contains(text, "package ") || strings.Contains(text, "interface ") || strings.Contains(text, "world ")
}

// ----------------------------------------------------------------------------
// WASM Binary Parser
// ----------------------------------------------------------------------------

func (p *WasmParser) parseBinary(src []byte, res *WasmFileResult) error {
	r := bytes.NewReader(src)

	// Read magic (4 bytes) and version (4 bytes)
	magic := make([]byte, 4)
	if _, err := io.ReadFull(r, magic); err != nil {
		return fmt.Errorf("reading magic: %w", err)
	}
	var version uint32
	if err := binary.Read(r, binary.LittleEndian, &version); err != nil {
		return fmt.Errorf("reading version: %w", err)
	}

	var funcTypeIndices []uint32
	funcNames := make(map[uint32]string)

	// Parse sections
	for r.Len() > 0 {
		secID, err := r.ReadByte()
		if err != nil {
			break
		}

		secLen, err := readU32Leb128(r)
		if err != nil {
			return fmt.Errorf("reading section %d length: %w", secID, err)
		}

		secBytes := make([]byte, secLen)
		if _, err := io.ReadFull(r, secBytes); err != nil {
			return fmt.Errorf("reading section %d payload: %w", secID, err)
		}

		secReader := bytes.NewReader(secBytes)

		switch secID {
		case 0: // Custom Section (e.g. "name")
			p.parseBinaryCustomSection(secReader, funcNames)

		case 1: // Type Section
			if err := p.parseBinaryTypeSection(secReader, res); err != nil {
				return err
			}

		case 2: // Import Section
			if err := p.parseBinaryImportSection(secReader, res); err != nil {
				return err
			}

		case 3: // Function Section
			count, err := readU32Leb128(secReader)
			if err == nil {
				for i := uint32(0); i < count; i++ {
					typeIdx, err := readU32Leb128(secReader)
					if err == nil {
						funcTypeIndices = append(funcTypeIndices, typeIdx)
					}
				}
			}

		case 4: // Table Section
			p.parseBinaryTableSection(secReader, res)

		case 5: // Memory Section
			p.parseBinaryMemorySection(secReader, res)

		case 6: // Global Section
			p.parseBinaryGlobalSection(secReader, res)

		case 7: // Export Section
			p.parseBinaryExportSection(secReader, res)

		case 9: // Element Section
			p.parseBinaryElemSection(secReader, res)

		case 10: // Code Section
			p.parseBinaryCodeSection(secReader, res, funcTypeIndices, funcNames)

		case 11: // Data Section
			p.parseBinaryDataSection(secReader, res)
		}
	}

	// Link exports to functions/memories/globals/tables
	for _, exp := range res.Exports {
		switch exp.Kind {
		case "func":
			for i := range res.Functions {
				if res.Functions[i].Name == exp.Target || exp.Target == fmt.Sprintf("$%d", i) || exp.Target == fmt.Sprintf("%d", i) {
					res.Functions[i].ExportNames = append(res.Functions[i].ExportNames, exp.Name)
				}
			}
		case "memory":
			for i := range res.Memories {
				if res.Memories[i].Name == exp.Target || exp.Target == fmt.Sprintf("$%d", i) || exp.Target == fmt.Sprintf("%d", i) {
					res.Memories[i].ExportNames = append(res.Memories[i].ExportNames, exp.Name)
				}
			}
		case "global":
			for i := range res.Globals {
				if res.Globals[i].Name == exp.Target || exp.Target == fmt.Sprintf("$%d", i) || exp.Target == fmt.Sprintf("%d", i) {
					res.Globals[i].ExportNames = append(res.Globals[i].ExportNames, exp.Name)
				}
			}
		case "table":
			for i := range res.Tables {
				if res.Tables[i].Name == exp.Target || exp.Target == fmt.Sprintf("$%d", i) || exp.Target == fmt.Sprintf("%d", i) {
					res.Tables[i].ExportNames = append(res.Tables[i].ExportNames, exp.Name)
				}
			}
		}
	}

	return nil
}

func (p *WasmParser) parseBinaryCustomSection(r *bytes.Reader, funcNames map[uint32]string) {
	name, err := readString(r)
	if err != nil || name != "name" {
		return
	}

	for r.Len() > 0 {
		subID, err := r.ReadByte()
		if err != nil {
			break
		}
		subLen, err := readU32Leb128(r)
		if err != nil {
			break
		}
		subBytes := make([]byte, subLen)
		if _, err := io.ReadFull(r, subBytes); err != nil {
			break
		}

		if subID == 1 { // Function names subsection
			subR := bytes.NewReader(subBytes)
			count, err := readU32Leb128(subR)
			if err == nil {
				for i := uint32(0); i < count; i++ {
					fIdx, err := readU32Leb128(subR)
					if err != nil {
						break
					}
					fName, err := readString(subR)
					if err != nil {
						break
					}
					funcNames[fIdx] = fName
				}
			}
		}
	}
}

func (p *WasmParser) parseBinaryTypeSection(r *bytes.Reader, res *WasmFileResult) error {
	count, err := readU32Leb128(r)
	if err != nil {
		return err
	}

	for i := uint32(0); i < count; i++ {
		form, err := r.ReadByte()
		if err != nil {
			return err
		}
		if form != 0x60 { // func type
			continue
		}

		paramCount, err := readU32Leb128(r)
		if err != nil {
			return err
		}
		var params []WasmParam
		for j := uint32(0); j < paramCount; j++ {
			vt, err := readValType(r)
			if err != nil {
				return err
			}
			params = append(params, WasmParam{Name: fmt.Sprintf("$%d", j), Type: vt})
		}

		resultCount, err := readU32Leb128(r)
		if err != nil {
			return err
		}
		var results []string
		for j := uint32(0); j < resultCount; j++ {
			vt, err := readValType(r)
			if err != nil {
				return err
			}
			results = append(results, vt)
		}

		res.Types = append(res.Types, WasmType{
			ID:      fmt.Sprintf("$t%d", i),
			Params:  params,
			Results: results,
		})
	}
	return nil
}

func (p *WasmParser) parseBinaryImportSection(r *bytes.Reader, res *WasmFileResult) error {
	count, err := readU32Leb128(r)
	if err != nil {
		return err
	}

	for i := uint32(0); i < count; i++ {
		mod, err := readString(r)
		if err != nil {
			return err
		}
		field, err := readString(r)
		if err != nil {
			return err
		}
		kindByte, err := r.ReadByte()
		if err != nil {
			return err
		}

		imp := WasmImport{
			Module: mod,
			Field:  field,
			Name:   fmt.Sprintf("$%s_%s", mod, field),
		}

		switch kindByte {
		case 0x00: // Func
			imp.Kind = "func"
			typeIdx, err := readU32Leb128(r)
			if err == nil {
				imp.TypeRef = fmt.Sprintf("$t%d", typeIdx)
				if int(typeIdx) < len(res.Types) {
					imp.Params = res.Types[typeIdx].Params
					imp.Results = res.Types[typeIdx].Results
				}
			}
		case 0x01: // Table
			imp.Kind = "table"
			elemType, err := readValType(r)
			if err == nil {
				imp.TableElemType = elemType
			}
			min, max, _, _, err := readLimits(r)
			if err == nil {
				imp.TableMin = min
				imp.TableMax = max
			}
		case 0x02: // Memory
			imp.Kind = "memory"
			min, max, _, _, err := readLimits(r)
			if err == nil {
				imp.MemoryMin = min
				imp.MemoryMax = max
			}
		case 0x03: // Global
			imp.Kind = "global"
			gt, err := readValType(r)
			if err == nil {
				imp.GlobalType = gt
			}
			mut, err := r.ReadByte()
			if err == nil {
				imp.GlobalMut = (mut == 0x01)
			}
		}

		res.Imports = append(res.Imports, imp)
	}
	return nil
}

func (p *WasmParser) parseBinaryTableSection(r *bytes.Reader, res *WasmFileResult) {
	count, err := readU32Leb128(r)
	if err != nil {
		return
	}
	for i := uint32(0); i < count; i++ {
		elemType, err := readValType(r)
		if err != nil {
			break
		}
		min, max, _, _, err := readLimits(r)
		if err != nil {
			break
		}
		res.Tables = append(res.Tables, WasmTable{
			Name:     fmt.Sprintf("$tbl%d", i),
			ElemType: elemType,
			Min:      min,
			Max:      max,
		})
	}
}

func (p *WasmParser) parseBinaryMemorySection(r *bytes.Reader, res *WasmFileResult) {
	count, err := readU32Leb128(r)
	if err != nil {
		return
	}
	for i := uint32(0); i < count; i++ {
		min, max, _, isShared, err := readLimits(r)
		if err != nil {
			break
		}
		res.Memories = append(res.Memories, WasmMemory{
			Name:   fmt.Sprintf("$mem%d", i),
			Min:    min,
			Max:    max,
			Shared: isShared,
		})
	}
}

func (p *WasmParser) parseBinaryGlobalSection(r *bytes.Reader, res *WasmFileResult) {
	count, err := readU32Leb128(r)
	if err != nil {
		return
	}
	for i := uint32(0); i < count; i++ {
		vt, err := readValType(r)
		if err != nil {
			break
		}
		mut, err := r.ReadByte()
		if err != nil {
			break
		}
		initExpr := p.readInitExpr(r)
		res.Globals = append(res.Globals, WasmGlobal{
			Name:     fmt.Sprintf("$g%d", i),
			Type:     vt,
			Mutable:  (mut == 0x01),
			InitExpr: initExpr,
		})
	}
}

func (p *WasmParser) parseBinaryExportSection(r *bytes.Reader, res *WasmFileResult) {
	count, err := readU32Leb128(r)
	if err != nil {
		return
	}
	for i := uint32(0); i < count; i++ {
		name, err := readString(r)
		if err != nil {
			break
		}
		kindByte, err := r.ReadByte()
		if err != nil {
			break
		}
		targetIdx, err := readU32Leb128(r)
		if err != nil {
			break
		}

		kind := "func"
		switch kindByte {
		case 0x01:
			kind = "table"
		case 0x02:
			kind = "memory"
		case 0x03:
			kind = "global"
		}

		res.Exports = append(res.Exports, WasmExport{
			Name:   name,
			Kind:   kind,
			Target: fmt.Sprintf("$%d", targetIdx),
		})
	}
}

func (p *WasmParser) parseBinaryElemSection(r *bytes.Reader, res *WasmFileResult) {
	count, err := readU32Leb128(r)
	if err != nil {
		return
	}
	for i := uint32(0); i < count; i++ {
		tableIdx, err := readU32Leb128(r)
		if err != nil {
			break
		}
		offsetExpr := p.readInitExpr(r)
		funcCount, err := readU32Leb128(r)
		if err != nil {
			break
		}
		var funcs []string
		for j := uint32(0); j < funcCount; j++ {
			fIdx, err := readU32Leb128(r)
			if err != nil {
				break
			}
			funcs = append(funcs, fmt.Sprintf("$%d", fIdx))
		}
		res.ElemSegments = append(res.ElemSegments, WasmElemSegment{
			Name:       fmt.Sprintf("$elem%d", i),
			TableIndex: tableIdx,
			OffsetExpr: offsetExpr,
			Funcs:      funcs,
		})
	}
}

func (p *WasmParser) parseBinaryCodeSection(
	r *bytes.Reader,
	res *WasmFileResult,
	funcTypeIndices []uint32,
	funcNames map[uint32]string,
) {
	count, err := readU32Leb128(r)
	if err != nil {
		return
	}

	importFuncCount := 0
	for _, imp := range res.Imports {
		if imp.Kind == "func" {
			importFuncCount++
		}
	}

	for i := uint32(0); i < count; i++ {
		bodyLen, err := readU32Leb128(r)
		if err != nil {
			break
		}
		bodyBytes := make([]byte, bodyLen)
		if _, err := io.ReadFull(r, bodyBytes); err != nil {
			break
		}

		bodyR := bytes.NewReader(bodyBytes)
		localGroupCount, err := readU32Leb128(bodyR)
		var locals []WasmLocal
		if err == nil {
			for j := uint32(0); j < localGroupCount; j++ {
				lCount, err := readU32Leb128(bodyR)
				if err != nil {
					break
				}
				lType, err := readValType(bodyR)
				if err != nil {
					break
				}
				locals = append(locals, WasmLocal{
					Name:  fmt.Sprintf("$l%d", len(locals)),
					Type:  lType,
					Count: lCount,
				})
			}
		}

		// Read raw bytecode instructions
		var instrs []string
		var calledFuncs []string
		for bodyR.Len() > 0 {
			op, err := bodyR.ReadByte()
			if err != nil || op == 0x0B { // 0x0B = end
				break
			}
			opStr, calls := disassembleOpcode(op, bodyR)
			if opStr != "" {
				instrs = append(instrs, opStr)
			}
			if len(calls) > 0 {
				calledFuncs = append(calledFuncs, calls...)
			}
		}

		globalFuncIdx := uint32(importFuncCount) + i
		fnName := fmt.Sprintf("$func%d", globalFuncIdx)
		if customName, ok := funcNames[globalFuncIdx]; ok {
			fnName = "$" + customName
		}

		var params []WasmParam
		var results []string
		var typeRef string
		if int(i) < len(funcTypeIndices) {
			tIdx := funcTypeIndices[i]
			typeRef = fmt.Sprintf("$t%d", tIdx)
			if int(tIdx) < len(res.Types) {
				params = res.Types[tIdx].Params
				results = res.Types[tIdx].Results
			}
		}

		res.Functions = append(res.Functions, WasmFunction{
			Name:         fnName,
			TypeRef:      typeRef,
			Params:       params,
			Results:      results,
			Locals:       locals,
			Instructions: instrs,
			BodySource:   strings.Join(instrs, "\n"),
			CalledFuncs:  calledFuncs,
		})
	}
}

func (p *WasmParser) parseBinaryDataSection(r *bytes.Reader, res *WasmFileResult) {
	count, err := readU32Leb128(r)
	if err != nil {
		return
	}
	for i := uint32(0); i < count; i++ {
		flags, err := readU32Leb128(r)
		if err != nil {
			break
		}
		var memIdx uint32
		var offsetExpr string
		isPassive := false
		if flags == 0 {
			offsetExpr = p.readInitExpr(r)
		} else if flags == 1 {
			isPassive = true
		} else if flags == 2 {
			memIdx, _ = readU32Leb128(r)
			offsetExpr = p.readInitExpr(r)
		}

		dataLen, err := readU32Leb128(r)
		if err != nil {
			break
		}
		dataBytes := make([]byte, dataLen)
		if _, err := io.ReadFull(r, dataBytes); err != nil {
			break
		}

		res.DataSegments = append(res.DataSegments, WasmDataSegment{
			Name:        fmt.Sprintf("$data%d", i),
			MemoryIndex: memIdx,
			OffsetExpr:  offsetExpr,
			Data:        string(dataBytes),
			Passive:     isPassive,
		})
	}
}

func (p *WasmParser) readInitExpr(r *bytes.Reader) string {
	var parts []string
	for r.Len() > 0 {
		b, err := r.ReadByte()
		if err != nil || b == 0x0B { // 0x0B = end
			break
		}
		switch b {
		case 0x41: // i32.const
			val, _ := readI32Leb128(r)
			parts = append(parts, fmt.Sprintf("(i32.const %d)", val))
		case 0x42: // i64.const
			val, _ := readI64Leb128(r)
			parts = append(parts, fmt.Sprintf("(i64.const %d)", val))
		case 0x43: // f32.const
			var f float32
			_ = binary.Read(r, binary.LittleEndian, &f)
			parts = append(parts, fmt.Sprintf("(f32.const %v)", f))
		case 0x44: // f64.const
			var f float64
			_ = binary.Read(r, binary.LittleEndian, &f)
			parts = append(parts, fmt.Sprintf("(f64.const %v)", f))
		case 0x23: // global.get
			idx, _ := readU32Leb128(r)
			parts = append(parts, fmt.Sprintf("(global.get $%d)", idx))
		default:
			parts = append(parts, fmt.Sprintf("0x%02X", b))
		}
	}
	return strings.Join(parts, " ")
}

func readU32Leb128(r *bytes.Reader) (uint32, error) {
	var result uint32
	var shift uint
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		result |= uint32(b&0x7F) << shift
		if (b & 0x80) == 0 {
			break
		}
		shift += 7
		if shift >= 35 {
			return 0, fmt.Errorf("LEB128 integer overflow")
		}
	}
	return result, nil
}

func readI32Leb128(r *bytes.Reader) (int32, error) {
	var result int32
	var shift uint
	var b byte
	var err error
	for {
		b, err = r.ReadByte()
		if err != nil {
			return 0, err
		}
		result |= int32(b&0x7F) << shift
		shift += 7
		if (b & 0x80) == 0 {
			break
		}
	}
	if shift < 32 && (b&0x40) != 0 {
		result |= -(1 << shift)
	}
	return result, nil
}

func readI64Leb128(r *bytes.Reader) (int64, error) {
	var result int64
	var shift uint
	var b byte
	var err error
	for {
		b, err = r.ReadByte()
		if err != nil {
			return 0, err
		}
		result |= int64(b&0x7F) << shift
		shift += 7
		if (b & 0x80) == 0 {
			break
		}
	}
	if shift < 64 && (b&0x40) != 0 {
		result |= -(1 << shift)
	}
	return result, nil
}

func readString(r *bytes.Reader) (string, error) {
	length, err := readU32Leb128(r)
	if err != nil {
		return "", err
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func readValType(r *bytes.Reader) (string, error) {
	b, err := r.ReadByte()
	if err != nil {
		return "", err
	}
	switch b {
	case 0x7F:
		return "i32", nil
	case 0x7E:
		return "i64", nil
	case 0x7D:
		return "f32", nil
	case 0x7C:
		return "f64", nil
	case 0x7B:
		return "v128", nil
	case 0x70:
		return "funcref", nil
	case 0x6F:
		return "externref", nil
	default:
		return fmt.Sprintf("valtype(0x%02X)", b), nil
	}
}

func readLimits(r *bytes.Reader) (min uint32, max uint32, hasMax bool, isShared bool, err error) {
	flags, err := r.ReadByte()
	if err != nil {
		return 0, 0, false, false, err
	}
	min, err = readU32Leb128(r)
	if err != nil {
		return 0, 0, false, false, err
	}
	if (flags & 0x01) != 0 {
		hasMax = true
		max, err = readU32Leb128(r)
		if err != nil {
			return 0, 0, false, false, err
		}
	}
	if (flags & 0x02) != 0 {
		isShared = true
	}
	return min, max, hasMax, isShared, nil
}

func disassembleOpcode(op byte, r *bytes.Reader) (string, []string) {
	switch op {
	case 0x00:
		return "unreachable", nil
	case 0x01:
		return "nop", nil
	case 0x02: // block
		bt, _ := r.ReadByte()
		return fmt.Sprintf("block (result 0x%02X)", bt), nil
	case 0x03: // loop
		bt, _ := r.ReadByte()
		return fmt.Sprintf("loop (result 0x%02X)", bt), nil
	case 0x04: // if
		bt, _ := r.ReadByte()
		return fmt.Sprintf("if (result 0x%02X)", bt), nil
	case 0x05:
		return "else", nil
	case 0x0C: // br
		depth, _ := readU32Leb128(r)
		return fmt.Sprintf("br %d", depth), nil
	case 0x0D: // br_if
		depth, _ := readU32Leb128(r)
		return fmt.Sprintf("br_if %d", depth), nil
	case 0x0F:
		return "return", nil
	case 0x10: // call
		fIdx, _ := readU32Leb128(r)
		target := fmt.Sprintf("$%d", fIdx)
		return fmt.Sprintf("call %s", target), []string{target}
	case 0x11: // call_indirect
		typeIdx, _ := readU32Leb128(r)
		tableIdx, _ := readU32Leb128(r)
		return fmt.Sprintf("call_indirect (type $%d) (table $%d)", typeIdx, tableIdx), nil
	case 0x1A:
		return "drop", nil
	case 0x1B:
		return "select", nil
	case 0x20: // local.get
		idx, _ := readU32Leb128(r)
		return fmt.Sprintf("local.get $%d", idx), nil
	case 0x21: // local.set
		idx, _ := readU32Leb128(r)
		return fmt.Sprintf("local.set $%d", idx), nil
	case 0x22: // local.tee
		idx, _ := readU32Leb128(r)
		return fmt.Sprintf("local.tee $%d", idx), nil
	case 0x23: // global.get
		idx, _ := readU32Leb128(r)
		return fmt.Sprintf("global.get $%d", idx), nil
	case 0x24: // global.set
		idx, _ := readU32Leb128(r)
		return fmt.Sprintf("global.set $%d", idx), nil
	case 0x28: // i32.load
		align, _ := readU32Leb128(r)
		offset, _ := readU32Leb128(r)
		return fmt.Sprintf("i32.load offset=%d align=%d", offset, align), nil
	case 0x36: // i32.store
		align, _ := readU32Leb128(r)
		offset, _ := readU32Leb128(r)
		return fmt.Sprintf("i32.store offset=%d align=%d", offset, align), nil
	case 0x3F: // memory.size
		_, _ = r.ReadByte()
		return "memory.size", nil
	case 0x40: // memory.grow
		_, _ = r.ReadByte()
		return "memory.grow", nil
	case 0x41: // i32.const
		val, _ := readI32Leb128(r)
		return fmt.Sprintf("i32.const %d", val), nil
	case 0x42: // i64.const
		val, _ := readI64Leb128(r)
		return fmt.Sprintf("i64.const %d", val), nil
	case 0x43: // f32.const
		var val float32
		_ = binary.Read(r, binary.LittleEndian, &val)
		return fmt.Sprintf("f32.const %v", val), nil
	case 0x44: // f64.const
		var val float64
		_ = binary.Read(r, binary.LittleEndian, &val)
		return fmt.Sprintf("f64.const %v", val), nil
	case 0x45:
		return "i32.eqz", nil
	case 0x46:
		return "i32.eq", nil
	case 0x47:
		return "i32.ne", nil
	case 0x48:
		return "i32.lt_s", nil
	case 0x49:
		return "i32.lt_u", nil
	case 0x4A:
		return "i32.gt_s", nil
	case 0x4B:
		return "i32.gt_u", nil
	case 0x6A:
		return "i32.add", nil
	case 0x6B:
		return "i32.sub", nil
	case 0x6C:
		return "i32.mul", nil
	case 0x6D:
		return "i32.div_s", nil
	case 0x6E:
		return "i32.div_u", nil
	case 0x71:
		return "i32.and", nil
	case 0x72:
		return "i32.or", nil
	case 0x73:
		return "i32.xor", nil
	case 0x7C:
		return "i64.add", nil
	case 0x7D:
		return "i64.sub", nil
	case 0x7E:
		return "i64.mul", nil
	case 0x92:
		return "f32.add", nil
	case 0x93:
		return "f32.sub", nil
	case 0x94:
		return "f32.mul", nil
	case 0xA0:
		return "f64.add", nil
	case 0xA1:
		return "f64.sub", nil
	case 0xA2:
		return "f64.mul", nil
	default:
		return fmt.Sprintf("wasm_op(0x%02X)", op), nil
	}
}

// ----------------------------------------------------------------------------
// WAT (WebAssembly Text) Parser
// ----------------------------------------------------------------------------

// SExpr represents an S-expression node in WAT AST.
type SExpr struct {
	IsAtom   bool
	Atom     string
	Children []*SExpr
	Doc      string
}

func (p *WasmParser) parseWat(src []byte, res *WasmFileResult) error {
	sexprs, err := parseSExprList(string(src))
	if err != nil {
		return fmt.Errorf("parsing WAT S-expressions: %w", err)
	}

	for _, sexpr := range sexprs {
		if sexpr.IsAtom {
			continue
		}
		if len(sexpr.Children) == 0 {
			continue
		}

		head := sexpr.Children[0].Atom
		if head == "module" {
			p.processWatModule(sexpr, res)
		} else {
			p.processWatTopLevel(sexpr, res)
		}
	}

	return nil
}

func (p *WasmParser) processWatModule(modExpr *SExpr, res *WasmFileResult) {
	children := modExpr.Children[1:]
	if len(children) > 0 && children[0].IsAtom && strings.HasPrefix(children[0].Atom, "$") {
		res.ModuleName = children[0].Atom
		children = children[1:]
	}

	for _, item := range children {
		if !item.IsAtom {
			p.processWatTopLevel(item, res)
		}
	}
}

func (p *WasmParser) processWatTopLevel(sexpr *SExpr, res *WasmFileResult) {
	if len(sexpr.Children) == 0 {
		return
	}
	tag := sexpr.Children[0].Atom

	switch tag {
	case "type":
		p.parseWatType(sexpr, res)
	case "import":
		p.parseWatImport(sexpr, res)
	case "func":
		p.parseWatFunc(sexpr, res)
	case "memory":
		p.parseWatMemory(sexpr, res)
	case "table":
		p.parseWatTable(sexpr, res)
	case "global":
		p.parseWatGlobal(sexpr, res)
	case "export":
		p.parseWatExport(sexpr, res)
	case "data":
		p.parseWatData(sexpr, res)
	case "elem":
		p.parseWatElem(sexpr, res)
	}
}

func (p *WasmParser) parseWatType(sexpr *SExpr, res *WasmFileResult) {
	var id string
	var params []WasmParam
	var results []string

	for i := 1; i < len(sexpr.Children); i++ {
		child := sexpr.Children[i]
		if child.IsAtom && strings.HasPrefix(child.Atom, "$") {
			id = child.Atom
		} else if !child.IsAtom && len(child.Children) > 0 && child.Children[0].Atom == "func" {
			for _, fChild := range child.Children[1:] {
				if !fChild.IsAtom && len(fChild.Children) > 0 {
					switch fChild.Children[0].Atom {
					case "param":
						params = append(params, extractParams(fChild)...)
					case "result":
						results = append(results, extractTypes(fChild)...)
					}
				}
			}
		}
	}

	if id == "" {
		id = fmt.Sprintf("$t%d", len(res.Types))
	}

	res.Types = append(res.Types, WasmType{
		ID:      id,
		Params:  params,
		Results: results,
		Doc:     sexpr.Doc,
	})
}

func (p *WasmParser) parseWatImport(sexpr *SExpr, res *WasmFileResult) {
	var stringsFound []string
	var descNode *SExpr

	for i := 1; i < len(sexpr.Children); i++ {
		child := sexpr.Children[i]
		if child.IsAtom {
			cleanStr := strings.Trim(child.Atom, "\"")
			stringsFound = append(stringsFound, cleanStr)
		} else {
			descNode = child
		}
	}

	mod := ""
	field := ""
	if len(stringsFound) > 0 {
		mod = stringsFound[0]
	}
	if len(stringsFound) > 1 {
		field = stringsFound[1]
	}

	imp := WasmImport{
		Module: mod,
		Field:  field,
		Doc:    sexpr.Doc,
	}

	if descNode != nil && len(descNode.Children) > 0 {
		kind := descNode.Children[0].Atom
		imp.Kind = kind

		var innerChildren []*SExpr
		if len(descNode.Children) > 1 {
			innerChildren = descNode.Children[1:]
		}

		for _, child := range innerChildren {
			if child.IsAtom && strings.HasPrefix(child.Atom, "$") {
				imp.Name = child.Atom
			} else if !child.IsAtom && len(child.Children) > 0 {
				switch child.Children[0].Atom {
				case "type":
					if len(child.Children) > 1 {
						imp.TypeRef = child.Children[1].Atom
					}
				case "param":
					imp.Params = append(imp.Params, extractParams(child)...)
				case "result":
					imp.Results = append(imp.Results, extractTypes(child)...)
				}
			}
		}

		if kind == "memory" {
			for _, c := range innerChildren {
				if c.IsAtom && !strings.HasPrefix(c.Atom, "$") {
					val, err := strconv.ParseUint(c.Atom, 10, 32)
					if err == nil {
						if imp.MemoryMin == 0 {
							imp.MemoryMin = uint32(val)
						} else {
							imp.MemoryMax = uint32(val)
						}
					}
				}
			}
		} else if kind == "table" {
			for _, c := range innerChildren {
				if c.IsAtom && !strings.HasPrefix(c.Atom, "$") {
					if c.Atom == "funcref" || c.Atom == "externref" {
						imp.TableElemType = c.Atom
					} else {
						val, err := strconv.ParseUint(c.Atom, 10, 32)
						if err == nil {
							if imp.TableMin == 0 {
								imp.TableMin = uint32(val)
							} else {
								imp.TableMax = uint32(val)
							}
						}
					}
				}
			}
		} else if kind == "global" {
			for _, c := range innerChildren {
				if c.IsAtom && !strings.HasPrefix(c.Atom, "$") {
					imp.GlobalType = c.Atom
				} else if !c.IsAtom && len(c.Children) > 0 && c.Children[0].Atom == "mut" {
					imp.GlobalMut = true
					if len(c.Children) > 1 {
						imp.GlobalType = c.Children[1].Atom
					}
				}
			}
		}
	}

	if imp.Name == "" {
		imp.Name = fmt.Sprintf("$%s_%s", mod, field)
	}

	res.Imports = append(res.Imports, imp)
}

func (p *WasmParser) parseWatFunc(sexpr *SExpr, res *WasmFileResult) {
	var name string
	var exportNames []string
	var typeRef string
	var params []WasmParam
	var results []string
	var locals []WasmLocal
	var instrs []string
	var calledFuncs []string

	for i := 1; i < len(sexpr.Children); i++ {
		child := sexpr.Children[i]
		if child.IsAtom {
			if strings.HasPrefix(child.Atom, "$") && name == "" {
				name = child.Atom
			} else {
				instrs = append(instrs, child.Atom)
			}
		} else if len(child.Children) > 0 {
			tag := child.Children[0].Atom
			switch tag {
			case "export":
				if len(child.Children) > 1 {
					expName := strings.Trim(child.Children[1].Atom, "\"")
					exportNames = append(exportNames, expName)
				}
			case "type":
				if len(child.Children) > 1 {
					typeRef = child.Children[1].Atom
				}
			case "param":
				params = append(params, extractParams(child)...)
			case "result":
				results = append(results, extractTypes(child)...)
			case "local":
				locals = append(locals, extractLocals(child)...)
			case "call":
				if len(child.Children) > 1 {
					calledFuncs = append(calledFuncs, child.Children[1].Atom)
				}
				instrs = append(instrs, sexprToString(child))
			default:
				instrs = append(instrs, sexprToString(child))
				findCallsInSExpr(child, &calledFuncs)
			}
		}
	}

	if name == "" {
		name = fmt.Sprintf("$func%d", len(res.Functions))
	}

	sort.Strings(calledFuncs)

	fn := WasmFunction{
		Name:         name,
		ExportNames:  exportNames,
		TypeRef:      typeRef,
		Params:       params,
		Results:      results,
		Locals:       locals,
		Instructions: instrs,
		BodySource:   strings.Join(instrs, " "),
		CalledFuncs:  dedupStrings(calledFuncs),
		Doc:          sexpr.Doc,
	}

	res.Functions = append(res.Functions, fn)

	for _, exp := range exportNames {
		res.Exports = append(res.Exports, WasmExport{
			Name:   exp,
			Kind:   "func",
			Target: name,
		})
	}
}

func (p *WasmParser) parseWatMemory(sexpr *SExpr, res *WasmFileResult) {
	var name string
	var exportNames []string
	var min, max uint32
	var shared, is64 bool

	for i := 1; i < len(sexpr.Children); i++ {
		child := sexpr.Children[i]
		if child.IsAtom {
			if strings.HasPrefix(child.Atom, "$") {
				name = child.Atom
			} else if child.Atom == "shared" {
				shared = true
			} else if child.Atom == "i64" {
				is64 = true
			} else {
				val, err := strconv.ParseUint(child.Atom, 10, 32)
				if err == nil {
					if min == 0 {
						min = uint32(val)
					} else {
						max = uint32(val)
					}
				}
			}
		} else if len(child.Children) > 0 && child.Children[0].Atom == "export" {
			if len(child.Children) > 1 {
				exportNames = append(exportNames, strings.Trim(child.Children[1].Atom, "\""))
			}
		}
	}

	if name == "" {
		name = fmt.Sprintf("$mem%d", len(res.Memories))
	}

	res.Memories = append(res.Memories, WasmMemory{
		Name:        name,
		ExportNames: exportNames,
		Min:         min,
		Max:         max,
		Shared:      shared,
		Is64:        is64,
		Doc:         sexpr.Doc,
	})

	for _, exp := range exportNames {
		res.Exports = append(res.Exports, WasmExport{
			Name:   exp,
			Kind:   "memory",
			Target: name,
		})
	}
}

func (p *WasmParser) parseWatTable(sexpr *SExpr, res *WasmFileResult) {
	var name string
	var exportNames []string
	var min, max uint32
	var elemType = "funcref"

	for i := 1; i < len(sexpr.Children); i++ {
		child := sexpr.Children[i]
		if child.IsAtom {
			if strings.HasPrefix(child.Atom, "$") {
				name = child.Atom
			} else if child.Atom == "funcref" || child.Atom == "externref" {
				elemType = child.Atom
			} else {
				val, err := strconv.ParseUint(child.Atom, 10, 32)
				if err == nil {
					if min == 0 {
						min = uint32(val)
					} else {
						max = uint32(val)
					}
				}
			}
		} else if len(child.Children) > 0 && child.Children[0].Atom == "export" {
			if len(child.Children) > 1 {
				exportNames = append(exportNames, strings.Trim(child.Children[1].Atom, "\""))
			}
		}
	}

	if name == "" {
		name = fmt.Sprintf("$tbl%d", len(res.Tables))
	}

	res.Tables = append(res.Tables, WasmTable{
		Name:        name,
		ExportNames: exportNames,
		Min:         min,
		Max:         max,
		ElemType:    elemType,
		Doc:         sexpr.Doc,
	})

	for _, exp := range exportNames {
		res.Exports = append(res.Exports, WasmExport{
			Name:   exp,
			Kind:   "table",
			Target: name,
		})
	}
}

func (p *WasmParser) parseWatGlobal(sexpr *SExpr, res *WasmFileResult) {
	var name string
	var exportNames []string
	var gType string
	var mutable bool
	var initParts []string

	for i := 1; i < len(sexpr.Children); i++ {
		child := sexpr.Children[i]
		if child.IsAtom {
			if strings.HasPrefix(child.Atom, "$") && name == "" {
				name = child.Atom
			} else if child.Atom == "i32" || child.Atom == "i64" || child.Atom == "f32" || child.Atom == "f64" || child.Atom == "v128" {
				gType = child.Atom
			} else {
				initParts = append(initParts, child.Atom)
			}
		} else if len(child.Children) > 0 {
			tag := child.Children[0].Atom
			switch tag {
			case "export":
				if len(child.Children) > 1 {
					exportNames = append(exportNames, strings.Trim(child.Children[1].Atom, "\""))
				}
			case "mut":
				mutable = true
				if len(child.Children) > 1 {
					gType = child.Children[1].Atom
				}
			default:
				initParts = append(initParts, sexprToString(child))
			}
		}
	}

	if name == "" {
		name = fmt.Sprintf("$g%d", len(res.Globals))
	}

	res.Globals = append(res.Globals, WasmGlobal{
		Name:        name,
		ExportNames: exportNames,
		Type:        gType,
		Mutable:     mutable,
		InitExpr:    strings.Join(initParts, " "),
		Doc:         sexpr.Doc,
	})

	for _, exp := range exportNames {
		res.Exports = append(res.Exports, WasmExport{
			Name:   exp,
			Kind:   "global",
			Target: name,
		})
	}
}

func (p *WasmParser) parseWatExport(sexpr *SExpr, res *WasmFileResult) {
	var name, kind, target string
	if len(sexpr.Children) > 1 {
		name = strings.Trim(sexpr.Children[1].Atom, "\"")
	}
	if len(sexpr.Children) > 2 && !sexpr.Children[2].IsAtom {
		desc := sexpr.Children[2]
		if len(desc.Children) > 0 {
			kind = desc.Children[0].Atom
		}
		if len(desc.Children) > 1 {
			target = desc.Children[1].Atom
		}
	}

	if name != "" {
		res.Exports = append(res.Exports, WasmExport{
			Name:   name,
			Kind:   kind,
			Target: target,
			Doc:    sexpr.Doc,
		})
	}
}

func (p *WasmParser) parseWatData(sexpr *SExpr, res *WasmFileResult) {
	var name, offsetExpr, data string
	var memIdx uint32
	var passive bool

	for i := 1; i < len(sexpr.Children); i++ {
		child := sexpr.Children[i]
		if child.IsAtom {
			if strings.HasPrefix(child.Atom, "$") {
				name = child.Atom
			} else if strings.HasPrefix(child.Atom, "\"") {
				data = unquoteWatString(child.Atom)
			}
		} else if len(child.Children) > 0 {
			tag := child.Children[0].Atom
			if tag == "memory" {
				if len(child.Children) > 1 {
					v, _ := strconv.ParseUint(child.Children[1].Atom, 10, 32)
					memIdx = uint32(v)
				}
			} else if tag == "i32.const" || tag == "i64.const" || tag == "offset" {
				offsetExpr = sexprToString(child)
			}
		}
	}

	if offsetExpr == "" && !strings.Contains(sexprToString(sexpr), "const") {
		passive = true
	}

	if name == "" {
		name = fmt.Sprintf("$data%d", len(res.DataSegments))
	}

	res.DataSegments = append(res.DataSegments, WasmDataSegment{
		Name:        name,
		MemoryIndex: memIdx,
		OffsetExpr:  offsetExpr,
		Data:        data,
		Passive:     passive,
		Doc:         sexpr.Doc,
	})
}

func (p *WasmParser) parseWatElem(sexpr *SExpr, res *WasmFileResult) {
	var name, offsetExpr string
	var tableIdx uint32
	var funcs []string

	for i := 1; i < len(sexpr.Children); i++ {
		child := sexpr.Children[i]
		if child.IsAtom {
			if strings.HasPrefix(child.Atom, "$") {
				funcs = append(funcs, child.Atom)
			}
		} else if len(child.Children) > 0 {
			tag := child.Children[0].Atom
			if tag == "table" {
				if len(child.Children) > 1 {
					v, _ := strconv.ParseUint(child.Children[1].Atom, 10, 32)
					tableIdx = uint32(v)
				}
			} else if tag == "i32.const" || tag == "offset" {
				offsetExpr = sexprToString(child)
			} else if tag == "func" {
				for _, fc := range child.Children[1:] {
					if fc.IsAtom {
						funcs = append(funcs, fc.Atom)
					}
				}
			}
		}
	}

	if name == "" {
		name = fmt.Sprintf("$elem%d", len(res.ElemSegments))
	}

	res.ElemSegments = append(res.ElemSegments, WasmElemSegment{
		Name:       name,
		TableIndex: tableIdx,
		OffsetExpr: offsetExpr,
		Funcs:      funcs,
		Doc:        sexpr.Doc,
	})
}

// ----------------------------------------------------------------------------
// WIT (WebAssembly Interface Types) Parser
// ----------------------------------------------------------------------------

func (p *WasmParser) parseWit(src []byte, res *WasmFileResult) error {
	lines := strings.Split(string(src), "\n")
	var currentPkg string
	var pendingDoc []string

	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])

		if strings.HasPrefix(line, "///") {
			pendingDoc = append(pendingDoc, strings.TrimSpace(strings.TrimPrefix(line, "///")))
			i++
			continue
		}
		if strings.HasPrefix(line, "//") || line == "" {
			i++
			continue
		}

		if strings.HasPrefix(line, "package ") {
			pkg := strings.TrimPrefix(line, "package ")
			pkg = strings.TrimSuffix(pkg, ";")
			currentPkg = strings.TrimSpace(pkg)
			pendingDoc = nil
			i++
			continue
		}

		if strings.HasPrefix(line, "interface ") && strings.Contains(line, "{") {
			ifaceName := strings.TrimSpace(strings.TrimPrefix(line, "interface "))
			ifaceName = strings.TrimSuffix(ifaceName, "{")
			ifaceName = strings.TrimSpace(ifaceName)

			iface := WitInterface{
				Package: currentPkg,
				Name:    ifaceName,
				Doc:     strings.Join(pendingDoc, "\n"),
			}
			pendingDoc = nil

			i++
			for i < len(lines) {
				inLine := strings.TrimSpace(lines[i])
				if inLine == "}" {
					break
				}
				if strings.HasPrefix(inLine, "///") {
					pendingDoc = append(pendingDoc, strings.TrimSpace(strings.TrimPrefix(inLine, "///")))
					i++
					continue
				}

				if strings.HasPrefix(inLine, "record ") && strings.Contains(inLine, "{") {
					recName := strings.TrimSpace(strings.TrimPrefix(inLine, "record "))
					recName = strings.TrimSuffix(recName, "{")
					recName = strings.TrimSpace(recName)
					rec := WitRecord{
						Name: recName,
						Doc:  strings.Join(pendingDoc, "\n"),
					}
					pendingDoc = nil
					i++
					for i < len(lines) {
						rLine := strings.TrimSpace(lines[i])
						if rLine == "}" || strings.HasPrefix(rLine, "}") {
							break
						}
						parts := strings.SplitN(rLine, ":", 2)
						if len(parts) == 2 {
							fName := strings.TrimSpace(parts[0])
							fType := strings.TrimSpace(parts[1])
							fType = strings.TrimSuffix(fType, ",")
							rec.Fields = append(rec.Fields, WitRecordField{Name: fName, Type: fType})
						}
						i++
					}
					iface.Records = append(iface.Records, rec)
				} else if strings.HasPrefix(inLine, "variant ") && strings.Contains(inLine, "{") {
					varName := strings.TrimSpace(strings.TrimPrefix(inLine, "variant "))
					varName = strings.TrimSuffix(varName, "{")
					varName = strings.TrimSpace(varName)
					variant := WitVariant{
						Name: varName,
						Doc:  strings.Join(pendingDoc, "\n"),
					}
					pendingDoc = nil
					i++
					for i < len(lines) {
						vLine := strings.TrimSpace(lines[i])
						if vLine == "}" || strings.HasPrefix(vLine, "}") {
							break
						}
						vLine = strings.TrimSuffix(vLine, ",")
						if vLine != "" {
							caseName := vLine
							caseType := ""
							if openIdx := strings.Index(vLine, "("); openIdx != -1 && strings.HasSuffix(vLine, ")") {
								caseName = strings.TrimSpace(vLine[:openIdx])
								caseType = strings.TrimSpace(vLine[openIdx+1 : len(vLine)-1])
							}
							variant.Cases = append(variant.Cases, WitVariantCase{Name: caseName, Type: caseType})
						}
						i++
					}
					iface.Variants = append(iface.Variants, variant)
				} else if strings.HasPrefix(inLine, "enum ") && strings.Contains(inLine, "{") {
					eName := strings.TrimSpace(strings.TrimPrefix(inLine, "enum "))
					eName = strings.TrimSuffix(eName, "{")
					eName = strings.TrimSpace(eName)
					e := WitEnum{
						Name: eName,
						Doc:  strings.Join(pendingDoc, "\n"),
					}
					pendingDoc = nil
					i++
					for i < len(lines) {
						eLine := strings.TrimSpace(lines[i])
						if eLine == "}" || strings.HasPrefix(eLine, "}") {
							break
						}
						for _, token := range strings.Split(eLine, ",") {
							tClean := strings.TrimSpace(token)
							if tClean != "" && tClean != "}" {
								e.Cases = append(e.Cases, tClean)
							}
						}
						i++
					}
					iface.Enums = append(iface.Enums, e)
				} else if strings.Contains(inLine, ": func(") {
					fn := p.parseWitFunc(inLine, strings.Join(pendingDoc, "\n"))
					iface.Functions = append(iface.Functions, fn)
					pendingDoc = nil
				}
				i++
			}

			res.WitInterfaces = append(res.WitInterfaces, iface)
		} else if strings.HasPrefix(line, "world ") && strings.Contains(line, "{") {
			worldName := strings.TrimSpace(strings.TrimPrefix(line, "world "))
			worldName = strings.TrimSuffix(worldName, "{")
			worldName = strings.TrimSpace(worldName)

			world := WitWorld{
				Package: currentPkg,
				Name:    worldName,
				Doc:     strings.Join(pendingDoc, "\n"),
			}
			pendingDoc = nil

			i++
			for i < len(lines) {
				wLine := strings.TrimSpace(lines[i])
				if wLine == "}" {
					break
				}
				if strings.HasPrefix(wLine, "import ") {
					item := strings.TrimSuffix(strings.TrimPrefix(wLine, "import "), ";")
					world.Imports = append(world.Imports, strings.TrimSpace(item))
				} else if strings.HasPrefix(wLine, "export ") {
					item := strings.TrimSuffix(strings.TrimPrefix(wLine, "export "), ";")
					world.Exports = append(world.Exports, strings.TrimSpace(item))
				}
				i++
			}
			res.WitWorlds = append(res.WitWorlds, world)
		}
		i++
	}

	return nil
}

func (p *WasmParser) parseWitFunc(line string, doc string) WitFunc {
	colonIdx := strings.Index(line, ":")
	fnName := strings.TrimSpace(line[:colonIdx])

	rest := strings.TrimSpace(line[colonIdx+1:])
	rest = strings.TrimPrefix(rest, "func")
	rest = strings.TrimSuffix(rest, ";")

	var params []WasmParam
	var results []string

	if openParen := strings.Index(rest, "("); openParen != -1 {
		closeParen := strings.Index(rest, ")")
		if closeParen != -1 && closeParen > openParen {
			paramStr := rest[openParen+1 : closeParen]
			if strings.TrimSpace(paramStr) != "" {
				for _, p := range strings.Split(paramStr, ",") {
					p = strings.TrimSpace(p)
					pParts := strings.SplitN(p, ":", 2)
					if len(pParts) == 2 {
						params = append(params, WasmParam{Name: strings.TrimSpace(pParts[0]), Type: strings.TrimSpace(pParts[1])})
					} else {
						params = append(params, WasmParam{Type: p})
					}
				}
			}
			arrowIdx := strings.Index(rest[closeParen:], "->")
			if arrowIdx != -1 {
				retStr := strings.TrimSpace(rest[closeParen+arrowIdx+2:])
				results = append(results, retStr)
			}
		}
	}

	return WitFunc{
		Name:    fnName,
		Params:  params,
		Results: results,
		Doc:     doc,
	}
}

// ----------------------------------------------------------------------------
// Helpers & AST Node Conversions
// ----------------------------------------------------------------------------

func (p *WasmParser) toSymbolNodes(res *WasmFileResult, lineage core.LineageEnvelope) ([]*core.ASTSymbolNode, error) {
	var nodes []*core.ASTSymbolNode

	// Types
	for _, t := range res.Types {
		node, err := TypeToASTSymbolNode(t, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Imports
	for _, imp := range res.Imports {
		node, err := ImportToASTSymbolNode(imp, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Functions
	for _, fn := range res.Functions {
		node, err := FuncToASTSymbolNode(fn, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Memories
	for _, mem := range res.Memories {
		node, err := MemoryToASTSymbolNode(mem, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Tables
	for _, tbl := range res.Tables {
		node, err := TableToASTSymbolNode(tbl, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Globals
	for _, g := range res.Globals {
		node, err := GlobalToASTSymbolNode(g, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Exports
	for _, exp := range res.Exports {
		node, err := ExportToASTSymbolNode(exp, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Data Segments
	for _, d := range res.DataSegments {
		node, err := DataToASTSymbolNode(d, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// WIT Interfaces
	for _, iface := range res.WitInterfaces {
		node, err := WitInterfaceToASTSymbolNode(iface, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// WIT Worlds
	for _, world := range res.WitWorlds {
		node, err := WitWorldToASTSymbolNode(world, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// TypeToASTSymbolNode converts a WasmType to an ASTSymbolNode.
func TypeToASTSymbolNode(t WasmType, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("marshaling WASM type %s: %w", t.ID, err)
	}

	var paramTypes []string
	for _, p := range t.Params {
		paramTypes = append(paramTypes, p.Type)
	}
	sig := fmt.Sprintf("(type %s (func (param %s) (result %s)))", t.ID, strings.Join(paramTypes, " "), strings.Join(t.Results, " "))

	node := &core.ASTSymbolNode{
		Language:    core.LangWasm,
		NodeType:    "WasmTypeDef",
		Identifier:  fmt.Sprintf("%s::%s", pkgName, t.ID),
		Signature:   sig,
		Docstring:   t.Doc,
		Visibility:  "public",
		ASTPayload:  payload,
		ASTMetadata: map[string]string{"type_id": t.ID},
		Lineage:     lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// ImportToASTSymbolNode converts a WasmImport to an ASTSymbolNode.
func ImportToASTSymbolNode(imp WasmImport, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(imp)
	if err != nil {
		return nil, fmt.Errorf("marshaling WASM import %s.%s: %w", imp.Module, imp.Field, err)
	}

	sig := fmt.Sprintf("(import %q %q (%s %s))", imp.Module, imp.Field, imp.Kind, imp.Name)

	node := &core.ASTSymbolNode{
		Language:   core.LangWasm,
		NodeType:   "WasmImport",
		Identifier: fmt.Sprintf("%s::import::%s.%s", pkgName, imp.Module, imp.Field),
		Signature:  sig,
		Docstring:  imp.Doc,
		Visibility: "public",
		ASTPayload: payload,
		ASTMetadata: map[string]string{
			"module": imp.Module,
			"field":  imp.Field,
			"kind":   imp.Kind,
		},
		Lineage: lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// FuncToASTSymbolNode converts a WasmFunction to an ASTSymbolNode.
func FuncToASTSymbolNode(fn WasmFunction, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(fn)
	if err != nil {
		return nil, fmt.Errorf("marshaling WASM func %s: %w", fn.Name, err)
	}

	var paramStr []string
	for _, p := range fn.Params {
		if p.Name != "" {
			paramStr = append(paramStr, fmt.Sprintf("(param %s %s)", p.Name, p.Type))
		} else {
			paramStr = append(paramStr, fmt.Sprintf("(param %s)", p.Type))
		}
	}
	var resStr []string
	for _, r := range fn.Results {
		resStr = append(resStr, fmt.Sprintf("(result %s)", r))
	}
	sig := fmt.Sprintf("(func %s %s %s)", fn.Name, strings.Join(paramStr, " "), strings.Join(resStr, " "))

	vis := "private"
	if len(fn.ExportNames) > 0 {
		vis = "public"
	}

	meta := map[string]string{
		"func_name": fn.Name,
	}
	if len(fn.ExportNames) > 0 {
		meta["exports"] = strings.Join(fn.ExportNames, ",")
	}
	if fn.TypeRef != "" {
		meta["type_ref"] = fn.TypeRef
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangWasm,
		NodeType:          "WasmFunc",
		Identifier:        fmt.Sprintf("%s::%s", pkgName, fn.Name),
		Signature:         sig,
		Docstring:         fn.Doc,
		Visibility:        vis,
		ASTPayload:        payload,
		ASTMetadata:       meta,
		LocalDependencies: fn.CalledFuncs,
		Dependencies:      fn.CalledFuncs,
		Lineage:           lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// MemoryToASTSymbolNode converts a WasmMemory to an ASTSymbolNode.
func MemoryToASTSymbolNode(mem WasmMemory, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(mem)
	if err != nil {
		return nil, fmt.Errorf("marshaling WASM memory %s: %w", mem.Name, err)
	}

	sig := fmt.Sprintf("(memory %s %d %d)", mem.Name, mem.Min, mem.Max)
	vis := "private"
	if len(mem.ExportNames) > 0 {
		vis = "public"
	}

	node := &core.ASTSymbolNode{
		Language:   core.LangWasm,
		NodeType:   "WasmMemory",
		Identifier: fmt.Sprintf("%s::memory::%s", pkgName, mem.Name),
		Signature:  sig,
		Docstring:  mem.Doc,
		Visibility: vis,
		ASTPayload: payload,
		ASTMetadata: map[string]string{
			"min": fmt.Sprintf("%d", mem.Min),
			"max": fmt.Sprintf("%d", mem.Max),
		},
		Lineage: lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// TableToASTSymbolNode converts a WasmTable to an ASTSymbolNode.
func TableToASTSymbolNode(tbl WasmTable, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(tbl)
	if err != nil {
		return nil, fmt.Errorf("marshaling WASM table %s: %w", tbl.Name, err)
	}

	sig := fmt.Sprintf("(table %s %d %d %s)", tbl.Name, tbl.Min, tbl.Max, tbl.ElemType)
	vis := "private"
	if len(tbl.ExportNames) > 0 {
		vis = "public"
	}

	node := &core.ASTSymbolNode{
		Language:   core.LangWasm,
		NodeType:   "WasmTable",
		Identifier: fmt.Sprintf("%s::table::%s", pkgName, tbl.Name),
		Signature:  sig,
		Docstring:  tbl.Doc,
		Visibility: vis,
		ASTPayload: payload,
		ASTMetadata: map[string]string{
			"elem_type": tbl.ElemType,
			"min":       fmt.Sprintf("%d", tbl.Min),
			"max":       fmt.Sprintf("%d", tbl.Max),
		},
		Lineage: lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// GlobalToASTSymbolNode converts a WasmGlobal to an ASTSymbolNode.
func GlobalToASTSymbolNode(g WasmGlobal, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(g)
	if err != nil {
		return nil, fmt.Errorf("marshaling WASM global %s: %w", g.Name, err)
	}

	mutStr := g.Type
	if g.Mutable {
		mutStr = fmt.Sprintf("(mut %s)", g.Type)
	}
	sig := fmt.Sprintf("(global %s %s %s)", g.Name, mutStr, g.InitExpr)

	vis := "private"
	if len(g.ExportNames) > 0 {
		vis = "public"
	}

	node := &core.ASTSymbolNode{
		Language:   core.LangWasm,
		NodeType:   "WasmGlobal",
		Identifier: fmt.Sprintf("%s::global::%s", pkgName, g.Name),
		Signature:  sig,
		Docstring:  g.Doc,
		Visibility: vis,
		ASTPayload: payload,
		ASTMetadata: map[string]string{
			"type":    g.Type,
			"mutable": fmt.Sprintf("%v", g.Mutable),
		},
		Lineage: lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// ExportToASTSymbolNode converts a WasmExport to an ASTSymbolNode.
func ExportToASTSymbolNode(exp WasmExport, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(exp)
	if err != nil {
		return nil, fmt.Errorf("marshaling WASM export %s: %w", exp.Name, err)
	}

	sig := fmt.Sprintf("(export %q (%s %s))", exp.Name, exp.Kind, exp.Target)

	node := &core.ASTSymbolNode{
		Language:          core.LangWasm,
		NodeType:          "WasmExport",
		Identifier:        fmt.Sprintf("%s::export::%s", pkgName, exp.Name),
		Signature:         sig,
		Docstring:         exp.Doc,
		Visibility:        "public",
		ASTPayload:        payload,
		ASTMetadata:       map[string]string{"kind": exp.Kind, "target": exp.Target},
		LocalDependencies: []string{exp.Target},
		Dependencies:      []string{exp.Target},
		Lineage:           lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// DataToASTSymbolNode converts a WasmDataSegment to an ASTSymbolNode.
func DataToASTSymbolNode(d WasmDataSegment, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("marshaling WASM data %s: %w", d.Name, err)
	}

	sig := fmt.Sprintf("(data %s %s %q)", d.Name, d.OffsetExpr, d.Data)

	node := &core.ASTSymbolNode{
		Language:    core.LangWasm,
		NodeType:    "WasmData",
		Identifier:  fmt.Sprintf("%s::data::%s", pkgName, d.Name),
		Signature:   sig,
		Docstring:   d.Doc,
		Visibility:  "private",
		ASTPayload:  payload,
		ASTMetadata: map[string]string{"size": fmt.Sprintf("%d", len(d.Data))},
		Lineage:     lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// WitInterfaceToASTSymbolNode converts a WitInterface to an ASTSymbolNode.
func WitInterfaceToASTSymbolNode(iface WitInterface, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(iface)
	if err != nil {
		return nil, fmt.Errorf("marshaling WIT interface %s: %w", iface.Name, err)
	}

	node := &core.ASTSymbolNode{
		Language:   core.LangWasm,
		NodeType:   "WitInterface",
		Identifier: fmt.Sprintf("%s::interface::%s", pkgName, iface.Name),
		Signature:  fmt.Sprintf("interface %s", iface.Name),
		Docstring:  iface.Doc,
		Visibility: "public",
		ASTPayload: payload,
		ASTMetadata: map[string]string{
			"package":        iface.Package,
			"record_count":   fmt.Sprintf("%d", len(iface.Records)),
			"variant_count":  fmt.Sprintf("%d", len(iface.Variants)),
			"enum_count":     fmt.Sprintf("%d", len(iface.Enums)),
			"function_count": fmt.Sprintf("%d", len(iface.Functions)),
		},
		Lineage: lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// WitWorldToASTSymbolNode converts a WitWorld to an ASTSymbolNode.
func WitWorldToASTSymbolNode(world WitWorld, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(world)
	if err != nil {
		return nil, fmt.Errorf("marshaling WIT world %s: %w", world.Name, err)
	}

	node := &core.ASTSymbolNode{
		Language:   core.LangWasm,
		NodeType:   "WitWorld",
		Identifier: fmt.Sprintf("%s::world::%s", pkgName, world.Name),
		Signature:  fmt.Sprintf("world %s", world.Name),
		Docstring:  world.Doc,
		Visibility: "public",
		ASTPayload: payload,
		ASTMetadata: map[string]string{
			"package":      world.Package,
			"import_count": fmt.Sprintf("%d", len(world.Imports)),
			"export_count": fmt.Sprintf("%d", len(world.Exports)),
		},
		Lineage: lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// BuildComponentNode bundles parsed WASM symbols into a core.ComponentNode.
func (p *WasmParser) BuildComponentNode(
	res *WasmFileResult,
	compName string,
	compType core.ComponentType,
	lineage core.LineageEnvelope,
) (*core.ComponentNode, error) {
	if compName == "" {
		compName = res.PackageName
	}
	if !compType.IsValid() {
		compType = core.CompService
	}

	symbolIDs := make([]string, 0, len(res.AllSymbols))
	for _, sym := range res.AllSymbols {
		symbolIDs = append(symbolIDs, sym.NodeID)
	}

	metadata := map[string]string{
		"file_path":      res.FilePath,
		"package_name":   res.PackageName,
		"is_binary":      fmt.Sprintf("%v", res.IsBinary),
		"symbol_count":   fmt.Sprintf("%d", len(res.AllSymbols)),
		"function_count": fmt.Sprintf("%d", len(res.Functions)),
		"import_count":   fmt.Sprintf("%d", len(res.Imports)),
		"export_count":   fmt.Sprintf("%d", len(res.Exports)),
	}

	comp := &core.ComponentNode{
		Name:        compName,
		Type:        compType,
		Language:    core.LangWasm,
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

// ----------------------------------------------------------------------------
// S-Expression Parsing Utilities
// ----------------------------------------------------------------------------

func parseSExprList(text string) ([]*SExpr, error) {
	tokens := tokenizeWat(text)
	var sexprs []*SExpr
	pos := 0

	for pos < len(tokens) {
		token := tokens[pos]
		if token.val == "(" {
			expr, nextPos, err := parseSExpr(tokens, pos)
			if err != nil {
				return nil, err
			}
			sexprs = append(sexprs, expr)
			pos = nextPos
		} else {
			pos++
		}
	}

	return sexprs, nil
}

type watToken struct {
	val string
	doc string
}

func tokenizeWat(text string) []watToken {
	var tokens []watToken
	var curDoc strings.Builder

	i := 0
	n := len(text)

	for i < n {
		ch := text[i]

		// Skip whitespace
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			i++
			continue
		}

		// Line comment ;;
		if ch == ';' && i+1 < n && text[i+1] == ';' {
			end := strings.IndexByte(text[i:], '\n')
			var commentText string
			if end == -1 {
				commentText = strings.TrimSpace(strings.TrimPrefix(text[i:], ";;"))
				i = n
			} else {
				commentText = strings.TrimSpace(strings.TrimPrefix(text[i:i+end], ";;"))
				i += end + 1
			}
			if curDoc.Len() > 0 {
				curDoc.WriteString("\n")
			}
			curDoc.WriteString(commentText)
			continue
		}

		// Block comment (; ... ;)
		if ch == '(' && i+1 < n && text[i+1] == ';' {
			end := strings.Index(text[i:], ";)")
			if end == -1 {
				i = n
			} else {
				commentText := strings.TrimSpace(text[i+2 : i+end])
				if curDoc.Len() > 0 {
					curDoc.WriteString("\n")
				}
				curDoc.WriteString(commentText)
				i += end + 2
			}
			continue
		}

		// Parentheses
		if ch == '(' || ch == ')' {
			doc := curDoc.String()
			curDoc.Reset()
			tokens = append(tokens, watToken{val: string(ch), doc: doc})
			i++
			continue
		}

		// Quoted strings "..."
		if ch == '"' {
			start := i
			i++
			for i < n {
				if text[i] == '\\' && i+1 < n {
					i += 2
				} else if text[i] == '"' {
					i++
					break
				} else {
					i++
				}
			}
			doc := curDoc.String()
			curDoc.Reset()
			tokens = append(tokens, watToken{val: text[start:i], doc: doc})
			continue
		}

		// Regular atom token
		start := i
		for i < n {
			c := text[i]
			if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '(' || c == ')' || c == '"' || (c == ';' && i+1 < n && text[i+1] == ';') {
				break
			}
			i++
		}
		doc := curDoc.String()
		curDoc.Reset()
		tokens = append(tokens, watToken{val: text[start:i], doc: doc})
	}

	return tokens
}

func parseSExpr(tokens []watToken, start int) (*SExpr, int, error) {
	if start >= len(tokens) || tokens[start].val != "(" {
		return nil, start, fmt.Errorf("expected '(' at token %d", start)
	}

	expr := &SExpr{
		Doc: tokens[start].doc,
	}

	pos := start + 1
	for pos < len(tokens) {
		token := tokens[pos]
		if token.val == "(" {
			child, nextPos, err := parseSExpr(tokens, pos)
			if err != nil {
				return nil, pos, err
			}
			expr.Children = append(expr.Children, child)
			pos = nextPos
		} else if token.val == ")" {
			return expr, pos + 1, nil
		} else {
			expr.Children = append(expr.Children, &SExpr{
				IsAtom: true,
				Atom:   token.val,
				Doc:    token.doc,
			})
			pos++
		}
	}

	return expr, pos, nil
}

func extractParams(sexpr *SExpr) []WasmParam {
	var params []WasmParam
	var name string
	for _, c := range sexpr.Children[1:] {
		if c.IsAtom {
			if strings.HasPrefix(c.Atom, "$") {
				name = c.Atom
			} else {
				params = append(params, WasmParam{Name: name, Type: c.Atom})
				name = ""
			}
		}
	}
	return params
}

func extractLocals(sexpr *SExpr) []WasmLocal {
	var locals []WasmLocal
	var name string
	for _, c := range sexpr.Children[1:] {
		if c.IsAtom {
			if strings.HasPrefix(c.Atom, "$") {
				name = c.Atom
			} else {
				locals = append(locals, WasmLocal{Name: name, Type: c.Atom, Count: 1})
				name = ""
			}
		}
	}
	return locals
}

func extractTypes(sexpr *SExpr) []string {
	var types []string
	for _, c := range sexpr.Children[1:] {
		if c.IsAtom && !strings.HasPrefix(c.Atom, "$") {
			types = append(types, c.Atom)
		}
	}
	return types
}

func sexprToString(sexpr *SExpr) string {
	if sexpr.IsAtom {
		return sexpr.Atom
	}
	var parts []string
	for _, c := range sexpr.Children {
		parts = append(parts, sexprToString(c))
	}
	return "(" + strings.Join(parts, " ") + ")"
}

func findCallsInSExpr(sexpr *SExpr, calls *[]string) {
	if !sexpr.IsAtom && len(sexpr.Children) > 0 {
		if sexpr.Children[0].Atom == "call" && len(sexpr.Children) > 1 {
			*calls = append(*calls, sexpr.Children[1].Atom)
		}
		for _, c := range sexpr.Children {
			findCallsInSExpr(c, calls)
		}
	}
}

func unquoteWatString(s string) string {
	s = strings.Trim(s, "\"")
	hexRe := regexp.MustCompile(`\\([0-9a-fA-F]{2})`)
	s = hexRe.ReplaceAllStringFunc(s, func(m string) string {
		h := m[1:]
		b, err := hex.DecodeString(h)
		if err == nil && len(b) > 0 {
			return string(b)
		}
		return m
	})
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\t`, "\t")
	s = strings.ReplaceAll(s, `\r`, "\r")
	s = strings.ReplaceAll(s, `\"`, "\"")
	s = strings.ReplaceAll(s, `\\`, "\\")
	return s
}

func dedupStrings(in []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range in {
		if !seen[s] && s != "" {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
