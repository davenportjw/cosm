package wasm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// WasmHydrator reconstitutes WASM/WAT/WIT AST symbol nodes back into clean source text or binary payloads.
type WasmHydrator struct{}

// NewWasmHydrator creates a new WasmHydrator instance.
func NewWasmHydrator() *WasmHydrator {
	return &WasmHydrator{}
}

// HydrateSymbol reconstitutes a single ASTSymbolNode into WAT/WIT source representation.
func (h *WasmHydrator) HydrateSymbol(node *core.ASTSymbolNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node is nil")
	}

	if len(node.ASTPayload) == 0 {
		return fmt.Sprintf(";; WASM Symbol: %s (%s)\n", node.Identifier, node.NodeType), nil
	}

	switch node.NodeType {
	case "WasmTypeDef":
		var t WasmType
		if err := json.Unmarshal(node.ASTPayload, &t); err != nil {
			return "", fmt.Errorf("unmarshaling WasmType: %w", err)
		}
		return h.renderType(&t), nil

	case "WasmImport":
		var imp WasmImport
		if err := json.Unmarshal(node.ASTPayload, &imp); err != nil {
			return "", fmt.Errorf("unmarshaling WasmImport: %w", err)
		}
		return h.renderImport(&imp), nil

	case "WasmFunc", "FnDef":
		var fn WasmFunction
		if err := json.Unmarshal(node.ASTPayload, &fn); err != nil {
			return "", fmt.Errorf("unmarshaling WasmFunction: %w", err)
		}
		return h.renderFunction(&fn), nil

	case "WasmMemory":
		var mem WasmMemory
		if err := json.Unmarshal(node.ASTPayload, &mem); err != nil {
			return "", fmt.Errorf("unmarshaling WasmMemory: %w", err)
		}
		return h.renderMemory(&mem), nil

	case "WasmTable":
		var tbl WasmTable
		if err := json.Unmarshal(node.ASTPayload, &tbl); err != nil {
			return "", fmt.Errorf("unmarshaling WasmTable: %w", err)
		}
		return h.renderTable(&tbl), nil

	case "WasmGlobal":
		var g WasmGlobal
		if err := json.Unmarshal(node.ASTPayload, &g); err != nil {
			return "", fmt.Errorf("unmarshaling WasmGlobal: %w", err)
		}
		return h.renderGlobal(&g), nil

	case "WasmExport":
		var exp WasmExport
		if err := json.Unmarshal(node.ASTPayload, &exp); err != nil {
			return "", fmt.Errorf("unmarshaling WasmExport: %w", err)
		}
		return h.renderExport(&exp), nil

	case "WasmData":
		var d WasmDataSegment
		if err := json.Unmarshal(node.ASTPayload, &d); err != nil {
			return "", fmt.Errorf("unmarshaling WasmDataSegment: %w", err)
		}
		return h.renderData(&d), nil

	case "WitInterface":
		var iface WitInterface
		if err := json.Unmarshal(node.ASTPayload, &iface); err != nil {
			return "", fmt.Errorf("unmarshaling WitInterface: %w", err)
		}
		return h.renderWitInterface(&iface), nil

	case "WitWorld":
		var world WitWorld
		if err := json.Unmarshal(node.ASTPayload, &world); err != nil {
			return "", fmt.Errorf("unmarshaling WitWorld: %w", err)
		}
		return h.renderWitWorld(&world), nil

	default:
		return string(node.ASTPayload), nil
	}
}

// HydrateModule reconstitutes a complete WAT module document from a parsed WasmFileResult.
func (h *WasmHydrator) HydrateModule(res *WasmFileResult) string {
	var sb strings.Builder

	if res.ModuleName != "" {
		sb.WriteString(fmt.Sprintf("(module %s\n", res.ModuleName))
	} else {
		sb.WriteString("(module\n")
	}

	// 1. Types
	for _, t := range res.Types {
		sb.WriteString("  " + strings.TrimSpace(h.renderType(&t)) + "\n")
	}

	// 2. Imports
	for _, imp := range res.Imports {
		sb.WriteString("  " + strings.TrimSpace(h.renderImport(&imp)) + "\n")
	}

	// 3. Tables
	for _, tbl := range res.Tables {
		sb.WriteString("  " + strings.TrimSpace(h.renderTable(&tbl)) + "\n")
	}

	// 4. Memories
	for _, mem := range res.Memories {
		sb.WriteString("  " + strings.TrimSpace(h.renderMemory(&mem)) + "\n")
	}

	// 5. Globals
	for _, g := range res.Globals {
		sb.WriteString("  " + strings.TrimSpace(h.renderGlobal(&g)) + "\n")
	}

	// 6. Functions
	for _, fn := range res.Functions {
		fnStr := h.renderFunction(&fn)
		for _, line := range strings.Split(strings.TrimSpace(fnStr), "\n") {
			sb.WriteString("  " + line + "\n")
		}
	}

	// 7. Separate Exports (if not already embedded inline)
	for _, exp := range res.Exports {
		sb.WriteString("  " + strings.TrimSpace(h.renderExport(&exp)) + "\n")
	}

	// 8. Elem Segments
	for _, elem := range res.ElemSegments {
		sb.WriteString("  " + strings.TrimSpace(h.renderElem(&elem)) + "\n")
	}

	// 9. Data Segments
	for _, d := range res.DataSegments {
		sb.WriteString("  " + strings.TrimSpace(h.renderData(&d)) + "\n")
	}

	sb.WriteString(")\n")
	return sb.String()
}

// HydrateWit reconstitutes a complete WIT document from a WasmFileResult.
func (h *WasmHydrator) HydrateWit(res *WasmFileResult) string {
	var sb strings.Builder
	for _, iface := range res.WitInterfaces {
		sb.WriteString(h.renderWitInterface(&iface))
		sb.WriteString("\n")
	}
	for _, world := range res.WitWorlds {
		sb.WriteString(h.renderWitWorld(&world))
		sb.WriteString("\n")
	}
	return sb.String()
}

// HydrateToBinary encodes a parsed WASM structure back into a standard .wasm binary bytecode payload.
func (h *WasmHydrator) HydrateToBinary(res *WasmFileResult) ([]byte, error) {
	var buf bytes.Buffer

	// Magic: \x00asm
	buf.Write([]byte{0x00, 0x61, 0x73, 0x6D})
	// Version: 1
	buf.Write([]byte{0x01, 0x00, 0x00, 0x00})

	// Section 1: Types
	if len(res.Types) > 0 {
		var secBuf bytes.Buffer
		writeU32Leb128(&secBuf, uint32(len(res.Types)))
		for _, t := range res.Types {
			secBuf.WriteByte(0x60) // func type
			writeU32Leb128(&secBuf, uint32(len(t.Params)))
			for _, p := range t.Params {
				secBuf.WriteByte(valTypeToByte(p.Type))
			}
			writeU32Leb128(&secBuf, uint32(len(t.Results)))
			for _, r := range t.Results {
				secBuf.WriteByte(valTypeToByte(r))
			}
		}
		writeSection(&buf, 1, secBuf.Bytes())
	}

	// Section 2: Imports
	if len(res.Imports) > 0 {
		var secBuf bytes.Buffer
		writeU32Leb128(&secBuf, uint32(len(res.Imports)))
		for _, imp := range res.Imports {
			writeStringLeb128(&secBuf, imp.Module)
			writeStringLeb128(&secBuf, imp.Field)
			switch imp.Kind {
			case "func":
				secBuf.WriteByte(0x00)
				tIdx := uint32(0)
				if strings.HasPrefix(imp.TypeRef, "$t") {
					fmt.Sscanf(imp.TypeRef, "$t%d", &tIdx)
				}
				writeU32Leb128(&secBuf, tIdx)
			case "table":
				secBuf.WriteByte(0x01)
				secBuf.WriteByte(valTypeToByte(imp.TableElemType))
				writeLimitsLeb128(&secBuf, imp.TableMin, imp.TableMax)
			case "memory":
				secBuf.WriteByte(0x02)
				writeLimitsLeb128(&secBuf, imp.MemoryMin, imp.MemoryMax)
			case "global":
				secBuf.WriteByte(0x03)
				secBuf.WriteByte(valTypeToByte(imp.GlobalType))
				if imp.GlobalMut {
					secBuf.WriteByte(0x01)
				} else {
					secBuf.WriteByte(0x00)
				}
			default:
				secBuf.WriteByte(0x00)
				writeU32Leb128(&secBuf, 0)
			}
		}
		writeSection(&buf, 2, secBuf.Bytes())
	}

	// Section 3: Functions (type index vector)
	if len(res.Functions) > 0 {
		var secBuf bytes.Buffer
		writeU32Leb128(&secBuf, uint32(len(res.Functions)))
		for i, fn := range res.Functions {
			tIdx := uint32(i)
			if strings.HasPrefix(fn.TypeRef, "$t") {
				fmt.Sscanf(fn.TypeRef, "$t%d", &tIdx)
			}
			writeU32Leb128(&secBuf, tIdx)
		}
		writeSection(&buf, 3, secBuf.Bytes())
	}

	// Section 4: Tables
	if len(res.Tables) > 0 {
		var secBuf bytes.Buffer
		writeU32Leb128(&secBuf, uint32(len(res.Tables)))
		for _, tbl := range res.Tables {
			secBuf.WriteByte(valTypeToByte(tbl.ElemType))
			writeLimitsLeb128(&secBuf, tbl.Min, tbl.Max)
		}
		writeSection(&buf, 4, secBuf.Bytes())
	}

	// Section 5: Memories
	if len(res.Memories) > 0 {
		var secBuf bytes.Buffer
		writeU32Leb128(&secBuf, uint32(len(res.Memories)))
		for _, mem := range res.Memories {
			writeLimitsLeb128(&secBuf, mem.Min, mem.Max)
		}
		writeSection(&buf, 5, secBuf.Bytes())
	}

	// Section 6: Globals
	if len(res.Globals) > 0 {
		var secBuf bytes.Buffer
		writeU32Leb128(&secBuf, uint32(len(res.Globals)))
		for _, g := range res.Globals {
			secBuf.WriteByte(valTypeToByte(g.Type))
			if g.Mutable {
				secBuf.WriteByte(0x01)
			} else {
				secBuf.WriteByte(0x00)
			}
			// Encode init opcode
			secBuf.Write(encodeInitExpr(g.InitExpr))
		}
		writeSection(&buf, 6, secBuf.Bytes())
	}

	// Section 7: Exports
	if len(res.Exports) > 0 {
		var secBuf bytes.Buffer
		writeU32Leb128(&secBuf, uint32(len(res.Exports)))
		for _, exp := range res.Exports {
			writeStringLeb128(&secBuf, exp.Name)
			kindByte := byte(0x00)
			switch exp.Kind {
			case "table":
				kindByte = 0x01
			case "memory":
				kindByte = 0x02
			case "global":
				kindByte = 0x03
			}
			secBuf.WriteByte(kindByte)
			tIdx := uint32(0)
			targetClean := strings.TrimLeft(exp.Target, "$")
			fmt.Sscanf(targetClean, "%d", &tIdx)
			writeU32Leb128(&secBuf, tIdx)
		}
		writeSection(&buf, 7, secBuf.Bytes())
	}

	// Section 10: Code
	if len(res.Functions) > 0 {
		var secBuf bytes.Buffer
		writeU32Leb128(&secBuf, uint32(len(res.Functions)))
		for _, fn := range res.Functions {
			var bodyBuf bytes.Buffer
			writeU32Leb128(&bodyBuf, uint32(len(fn.Locals)))
			for _, loc := range fn.Locals {
				cnt := loc.Count
				if cnt == 0 {
					cnt = 1
				}
				writeU32Leb128(&bodyBuf, cnt)
				bodyBuf.WriteByte(valTypeToByte(loc.Type))
			}
			// Encode instructions
			bodyBuf.Write(encodeInstructions(fn.Instructions))
			bodyBuf.WriteByte(0x0B) // End opcode

			writeU32Leb128(&secBuf, uint32(bodyBuf.Len()))
			secBuf.Write(bodyBuf.Bytes())
		}
		writeSection(&buf, 10, secBuf.Bytes())
	}

	// Section 11: Data
	if len(res.DataSegments) > 0 {
		var secBuf bytes.Buffer
		writeU32Leb128(&secBuf, uint32(len(res.DataSegments)))
		for _, d := range res.DataSegments {
			if d.Passive {
				secBuf.WriteByte(0x01)
			} else {
				secBuf.WriteByte(0x00)
				secBuf.Write(encodeInitExpr(d.OffsetExpr))
			}
			writeU32Leb128(&secBuf, uint32(len(d.Data)))
			secBuf.WriteString(d.Data)
		}
		writeSection(&buf, 11, secBuf.Bytes())
	}

	return buf.Bytes(), nil
}

// ----------------------------------------------------------------------------
// WAT Renderers
// ----------------------------------------------------------------------------

func (h *WasmHydrator) renderType(t *WasmType) string {
	var sb strings.Builder
	if t.Doc != "" {
		for _, d := range strings.Split(t.Doc, "\n") {
			sb.WriteString(fmt.Sprintf(";; %s\n", d))
		}
	}
	var params []string
	for _, p := range t.Params {
		if p.Name != "" {
			params = append(params, fmt.Sprintf("(param %s %s)", p.Name, p.Type))
		} else {
			params = append(params, fmt.Sprintf("(param %s)", p.Type))
		}
	}
	var results []string
	for _, r := range t.Results {
		results = append(results, fmt.Sprintf("(result %s)", r))
	}
	sb.WriteString(fmt.Sprintf("(type %s (func", t.ID))
	if len(params) > 0 {
		sb.WriteString(" " + strings.Join(params, " "))
	}
	if len(results) > 0 {
		sb.WriteString(" " + strings.Join(results, " "))
	}
	sb.WriteString("))\n")
	return sb.String()
}

func (h *WasmHydrator) renderImport(imp *WasmImport) string {
	var sb strings.Builder
	if imp.Doc != "" {
		for _, d := range strings.Split(imp.Doc, "\n") {
			sb.WriteString(fmt.Sprintf(";; %s\n", d))
		}
	}

	var desc string
	switch imp.Kind {
	case "func":
		var pStr, rStr string
		if len(imp.Params) > 0 {
			var pList []string
			for _, p := range imp.Params {
				if p.Name != "" {
					pList = append(pList, fmt.Sprintf("(param %s %s)", p.Name, p.Type))
				} else {
					pList = append(pList, fmt.Sprintf("(param %s)", p.Type))
				}
			}
			pStr = " " + strings.Join(pList, " ")
		}
		if len(imp.Results) > 0 {
			var rList []string
			for _, r := range imp.Results {
				rList = append(rList, fmt.Sprintf("(result %s)", r))
			}
			rStr = " " + strings.Join(rList, " ")
		}
		typeRefStr := ""
		if imp.TypeRef != "" {
			typeRefStr = fmt.Sprintf(" (type %s)", imp.TypeRef)
		}
		desc = fmt.Sprintf("(func %s%s%s%s)", imp.Name, typeRefStr, pStr, rStr)
	case "memory":
		maxStr := ""
		if imp.MemoryMax > 0 {
			maxStr = fmt.Sprintf(" %d", imp.MemoryMax)
		}
		desc = fmt.Sprintf("(memory %s %d%s)", imp.Name, imp.MemoryMin, maxStr)
	case "table":
		maxStr := ""
		if imp.TableMax > 0 {
			maxStr = fmt.Sprintf(" %d", imp.TableMax)
		}
		elemType := imp.TableElemType
		if elemType == "" {
			elemType = "funcref"
		}
		desc = fmt.Sprintf("(table %s %d%s %s)", imp.Name, imp.TableMin, maxStr, elemType)
	case "global":
		mutStr := imp.GlobalType
		if imp.GlobalMut {
			mutStr = fmt.Sprintf("(mut %s)", imp.GlobalType)
		}
		desc = fmt.Sprintf("(global %s %s)", imp.Name, mutStr)
	default:
		desc = fmt.Sprintf("(%s %s)", imp.Kind, imp.Name)
	}

	sb.WriteString(fmt.Sprintf("(import %q %q %s)\n", imp.Module, imp.Field, desc))
	return sb.String()
}

func (h *WasmHydrator) renderFunction(fn *WasmFunction) string {
	var sb strings.Builder
	if fn.Doc != "" {
		for _, d := range strings.Split(fn.Doc, "\n") {
			sb.WriteString(fmt.Sprintf(";; %s\n", d))
		}
	}

	sb.WriteString(fmt.Sprintf("(func %s", fn.Name))
	for _, exp := range fn.ExportNames {
		sb.WriteString(fmt.Sprintf(" (export %q)", exp))
	}
	if fn.TypeRef != "" {
		sb.WriteString(fmt.Sprintf(" (type %s)", fn.TypeRef))
	}
	for _, p := range fn.Params {
		if p.Name != "" {
			sb.WriteString(fmt.Sprintf(" (param %s %s)", p.Name, p.Type))
		} else {
			sb.WriteString(fmt.Sprintf(" (param %s)", p.Type))
		}
	}
	for _, r := range fn.Results {
		sb.WriteString(fmt.Sprintf(" (result %s)", r))
	}
	for _, l := range fn.Locals {
		if l.Name != "" {
			sb.WriteString(fmt.Sprintf(" (local %s %s)", l.Name, l.Type))
		} else {
			sb.WriteString(fmt.Sprintf(" (local %s)", l.Type))
		}
	}

	if len(fn.Instructions) > 0 {
		sb.WriteString("\n")
		for _, inst := range fn.Instructions {
			sb.WriteString(fmt.Sprintf("    %s\n", inst))
		}
		sb.WriteString(")\n")
	} else if fn.BodySource != "" && !strings.HasPrefix(fn.BodySource, "(func") {
		sb.WriteString("\n")
		for _, line := range strings.Split(fn.BodySource, "\n") {
			if strings.TrimSpace(line) != "" {
				sb.WriteString(fmt.Sprintf("    %s\n", strings.TrimSpace(line)))
			}
		}
		sb.WriteString(")\n")
	} else {
		sb.WriteString(")\n")
	}

	return sb.String()
}

func (h *WasmHydrator) renderMemory(mem *WasmMemory) string {
	var sb strings.Builder
	if mem.Doc != "" {
		for _, d := range strings.Split(mem.Doc, "\n") {
			sb.WriteString(fmt.Sprintf(";; %s\n", d))
		}
	}
	sb.WriteString("(memory")
	if mem.Name != "" {
		sb.WriteString(fmt.Sprintf(" %s", mem.Name))
	}
	for _, exp := range mem.ExportNames {
		sb.WriteString(fmt.Sprintf(" (export %q)", exp))
	}
	if mem.Is64 {
		sb.WriteString(" i64")
	}
	sb.WriteString(fmt.Sprintf(" %d", mem.Min))
	if mem.Max > 0 {
		sb.WriteString(fmt.Sprintf(" %d", mem.Max))
	}
	if mem.Shared {
		sb.WriteString(" shared")
	}
	sb.WriteString(")\n")
	return sb.String()
}

func (h *WasmHydrator) renderTable(tbl *WasmTable) string {
	var sb strings.Builder
	if tbl.Doc != "" {
		for _, d := range strings.Split(tbl.Doc, "\n") {
			sb.WriteString(fmt.Sprintf(";; %s\n", d))
		}
	}
	sb.WriteString("(table")
	if tbl.Name != "" {
		sb.WriteString(fmt.Sprintf(" %s", tbl.Name))
	}
	for _, exp := range tbl.ExportNames {
		sb.WriteString(fmt.Sprintf(" (export %q)", exp))
	}
	sb.WriteString(fmt.Sprintf(" %d", tbl.Min))
	if tbl.Max > 0 {
		sb.WriteString(fmt.Sprintf(" %d", tbl.Max))
	}
	elemType := tbl.ElemType
	if elemType == "" {
		elemType = "funcref"
	}
	sb.WriteString(fmt.Sprintf(" %s)\n", elemType))
	return sb.String()
}

func (h *WasmHydrator) renderGlobal(g *WasmGlobal) string {
	var sb strings.Builder
	if g.Doc != "" {
		for _, d := range strings.Split(g.Doc, "\n") {
			sb.WriteString(fmt.Sprintf(";; %s\n", d))
		}
	}
	sb.WriteString("(global")
	if g.Name != "" {
		sb.WriteString(fmt.Sprintf(" %s", g.Name))
	}
	for _, exp := range g.ExportNames {
		sb.WriteString(fmt.Sprintf(" (export %q)", exp))
	}
	if g.Mutable {
		sb.WriteString(fmt.Sprintf(" (mut %s)", g.Type))
	} else {
		sb.WriteString(fmt.Sprintf(" %s", g.Type))
	}
	if g.InitExpr != "" {
		sb.WriteString(fmt.Sprintf(" %s", g.InitExpr))
	}
	sb.WriteString(")\n")
	return sb.String()
}

func (h *WasmHydrator) renderExport(exp *WasmExport) string {
	var sb strings.Builder
	if exp.Doc != "" {
		for _, d := range strings.Split(exp.Doc, "\n") {
			sb.WriteString(fmt.Sprintf(";; %s\n", d))
		}
	}
	sb.WriteString(fmt.Sprintf("(export %q (%s %s))\n", exp.Name, exp.Kind, exp.Target))
	return sb.String()
}

func (h *WasmHydrator) renderData(d *WasmDataSegment) string {
	var sb strings.Builder
	if d.Doc != "" {
		for _, doc := range strings.Split(d.Doc, "\n") {
			sb.WriteString(fmt.Sprintf(";; %s\n", doc))
		}
	}
	sb.WriteString("(data")
	if d.Name != "" {
		sb.WriteString(fmt.Sprintf(" %s", d.Name))
	}
	if d.MemoryIndex > 0 {
		sb.WriteString(fmt.Sprintf(" (memory %d)", d.MemoryIndex))
	}
	if d.OffsetExpr != "" {
		sb.WriteString(fmt.Sprintf(" %s", d.OffsetExpr))
	}
	sb.WriteString(fmt.Sprintf(" %q)\n", d.Data))
	return sb.String()
}

func (h *WasmHydrator) renderElem(elem *WasmElemSegment) string {
	var sb strings.Builder
	if elem.Doc != "" {
		for _, doc := range strings.Split(elem.Doc, "\n") {
			sb.WriteString(fmt.Sprintf(";; %s\n", doc))
		}
	}
	sb.WriteString("(elem")
	if elem.Name != "" {
		sb.WriteString(fmt.Sprintf(" %s", elem.Name))
	}
	if elem.TableIndex > 0 {
		sb.WriteString(fmt.Sprintf(" (table %d)", elem.TableIndex))
	}
	if elem.OffsetExpr != "" {
		sb.WriteString(fmt.Sprintf(" %s", elem.OffsetExpr))
	}
	if len(elem.Funcs) > 0 {
		sb.WriteString(" " + strings.Join(elem.Funcs, " "))
	}
	sb.WriteString(")\n")
	return sb.String()
}

// ----------------------------------------------------------------------------
// WIT Renderers
// ----------------------------------------------------------------------------

func (h *WasmHydrator) renderWitInterface(iface *WitInterface) string {
	var sb strings.Builder
	if iface.Package != "" {
		sb.WriteString(fmt.Sprintf("package %s;\n\n", iface.Package))
	}
	if iface.Doc != "" {
		for _, doc := range strings.Split(iface.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("/// %s\n", doc))
		}
	}
	sb.WriteString(fmt.Sprintf("interface %s {\n", iface.Name))

	// Records
	for _, rec := range iface.Records {
		if rec.Doc != "" {
			for _, doc := range strings.Split(rec.Doc, "\n") {
				sb.WriteString(fmt.Sprintf("    /// %s\n", doc))
			}
		}
		sb.WriteString(fmt.Sprintf("    record %s {\n", rec.Name))
		for _, f := range rec.Fields {
			sb.WriteString(fmt.Sprintf("        %s: %s,\n", f.Name, f.Type))
		}
		sb.WriteString("    }\n\n")
	}

	// Variants
	for _, v := range iface.Variants {
		if v.Doc != "" {
			for _, doc := range strings.Split(v.Doc, "\n") {
				sb.WriteString(fmt.Sprintf("    /// %s\n", doc))
			}
		}
		sb.WriteString(fmt.Sprintf("    variant %s {\n", v.Name))
		for _, c := range v.Cases {
			if c.Type != "" {
				sb.WriteString(fmt.Sprintf("        %s(%s),\n", c.Name, c.Type))
			} else {
				sb.WriteString(fmt.Sprintf("        %s,\n", c.Name))
			}
		}
		sb.WriteString("    }\n\n")
	}

	// Enums
	for _, e := range iface.Enums {
		if e.Doc != "" {
			for _, doc := range strings.Split(e.Doc, "\n") {
				sb.WriteString(fmt.Sprintf("    /// %s\n", doc))
			}
		}
		sb.WriteString(fmt.Sprintf("    enum %s {\n", e.Name))
		for _, c := range e.Cases {
			sb.WriteString(fmt.Sprintf("        %s,\n", c))
		}
		sb.WriteString("    }\n\n")
	}

	// Functions
	for _, fn := range iface.Functions {
		if fn.Doc != "" {
			for _, doc := range strings.Split(fn.Doc, "\n") {
				sb.WriteString(fmt.Sprintf("    /// %s\n", doc))
			}
		}
		var params []string
		for _, p := range fn.Params {
			if p.Name != "" {
				params = append(params, fmt.Sprintf("%s: %s", p.Name, p.Type))
			} else {
				params = append(params, p.Type)
			}
		}
		retStr := ""
		if len(fn.Results) > 0 {
			retStr = fmt.Sprintf(" -> %s", strings.Join(fn.Results, ", "))
		}
		sb.WriteString(fmt.Sprintf("    %s: func(%s)%s;\n", fn.Name, strings.Join(params, ", "), retStr))
	}

	sb.WriteString("}\n")
	return sb.String()
}

func (h *WasmHydrator) renderWitWorld(world *WitWorld) string {
	var sb strings.Builder
	if world.Doc != "" {
		for _, doc := range strings.Split(world.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("/// %s\n", doc))
		}
	}
	sb.WriteString(fmt.Sprintf("world %s {\n", world.Name))
	for _, imp := range world.Imports {
		sb.WriteString(fmt.Sprintf("    import %s;\n", imp))
	}
	for _, exp := range world.Exports {
		sb.WriteString(fmt.Sprintf("    export %s;\n", exp))
	}
	sb.WriteString("}\n")
	return sb.String()
}

// ----------------------------------------------------------------------------
// Binary Encoding Utilities
// ----------------------------------------------------------------------------

func writeSection(w *bytes.Buffer, secID byte, payload []byte) {
	w.WriteByte(secID)
	writeU32Leb128(w, uint32(len(payload)))
	w.Write(payload)
}

func writeU32Leb128(w *bytes.Buffer, val uint32) {
	for {
		b := byte(val & 0x7F)
		val >>= 7
		if val != 0 {
			b |= 0x80
		}
		w.WriteByte(b)
		if val == 0 {
			break
		}
	}
}

func writeI32Leb128(w *bytes.Buffer, val int32) {
	for {
		b := byte(val & 0x7F)
		val >>= 7
		signBit := (b & 0x40) != 0
		if (val == 0 && !signBit) || (val == -1 && signBit) {
			w.WriteByte(b)
			break
		} else {
			w.WriteByte(b | 0x80)
		}
	}
}

func writeStringLeb128(w *bytes.Buffer, s string) {
	writeU32Leb128(w, uint32(len(s)))
	w.WriteString(s)
}

func writeLimitsLeb128(w *bytes.Buffer, min, max uint32) {
	if max > 0 {
		w.WriteByte(0x01)
		writeU32Leb128(w, min)
		writeU32Leb128(w, max)
	} else {
		w.WriteByte(0x00)
		writeU32Leb128(w, min)
	}
}

func valTypeToByte(vt string) byte {
	switch vt {
	case "i32":
		return 0x7F
	case "i64":
		return 0x7E
	case "f32":
		return 0x7D
	case "f64":
		return 0x7C
	case "v128":
		return 0x7B
	case "funcref":
		return 0x70
	case "externref":
		return 0x6F
	default:
		return 0x7F
	}
}

func encodeInitExpr(expr string) []byte {
	var buf bytes.Buffer
	trimmed := strings.TrimSpace(expr)
	if strings.Contains(trimmed, "i32.const") {
		var val int32
		fmt.Sscanf(trimmed, "(i32.const %d)", &val)
		buf.WriteByte(0x41) // i32.const
		writeI32Leb128(&buf, val)
	} else if strings.Contains(trimmed, "i64.const") {
		var val int64
		fmt.Sscanf(trimmed, "(i64.const %d)", &val)
		buf.WriteByte(0x42)
		writeI32Leb128(&buf, int32(val))
	} else {
		buf.WriteByte(0x41)
		writeI32Leb128(&buf, 0)
	}
	buf.WriteByte(0x0B) // end
	return buf.Bytes()
}

func encodeInstructions(instrs []string) []byte {
	var buf bytes.Buffer
	for _, inst := range instrs {
		trimmed := strings.TrimSpace(inst)
		if strings.HasPrefix(trimmed, "local.get") {
			var idx uint32
			fmt.Sscanf(trimmed, "local.get $%d", &idx)
			buf.WriteByte(0x20)
			writeU32Leb128(&buf, idx)
		} else if strings.HasPrefix(trimmed, "local.set") {
			var idx uint32
			fmt.Sscanf(trimmed, "local.set $%d", &idx)
			buf.WriteByte(0x21)
			writeU32Leb128(&buf, idx)
		} else if strings.HasPrefix(trimmed, "i32.const") {
			var val int32
			fmt.Sscanf(trimmed, "i32.const %d", &val)
			buf.WriteByte(0x41)
			writeI32Leb128(&buf, val)
		} else if strings.HasPrefix(trimmed, "call") {
			var idx uint32
			fmt.Sscanf(trimmed, "call $%d", &idx)
			buf.WriteByte(0x10)
			writeU32Leb128(&buf, idx)
		} else if trimmed == "i32.add" {
			buf.WriteByte(0x6A)
		} else if trimmed == "i32.sub" {
			buf.WriteByte(0x6B)
		} else if trimmed == "i32.mul" {
			buf.WriteByte(0x6C)
		} else if trimmed == "i64.add" {
			buf.WriteByte(0x7C)
		} else if trimmed == "return" {
			buf.WriteByte(0x0F)
		} else if trimmed == "nop" {
			buf.WriteByte(0x01)
		}
	}
	if buf.Len() == 0 {
		buf.WriteByte(0x01) // nop
	}
	return buf.Bytes()
}
