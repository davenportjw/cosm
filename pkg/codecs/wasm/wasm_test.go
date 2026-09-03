package wasm

import (
	"strings"
	"testing"

	"github.com/cosmscm/cosm/pkg/core"
)

const sampleWat = `(module $math_module
  ;; Type definitions
  (type $binop (func (param i32 i32) (result i32)))
  (type $unary (func (param i32) (result i32)))

  ;; Imports
  (import "env" "log_int" (func $log_int (param i32)))
  (import "env" "memory" (memory $imported_mem 1 2))

  ;; Global definitions
  (global $counter (mut i32) (i32.const 0))
  (global $max_val i32 (i32.const 1024))

  ;; Table definition
  (table $func_table 4 8 funcref)

  ;; Memory definition
  (memory $mem 2 4)

  ;; Function definitions
  ;; Adds two 32-bit integers
  (func $add (export "add") (type $binop) (param $a i32) (param $b i32) (result i32)
    (local $temp i32)
    (call $log_int)
    (local.get $a)
    (local.get $b)
    (i32.add)
  )

  ;; Factorial function
  (func $factorial (export "factorial") (param $n i32) (result i32)
    (local.get $n)
    (i32.const 1)
    (call $add)
  )

  ;; Explicit exports
  (export "counter" (global $counter))
  (export "mem" (memory $mem))
  (export "table" (table $func_table))

  ;; Elements and Data
  (elem $elem0 (table 0) (i32.const 0) $add $factorial)
  (data $data0 (i32.const 0) "Hello Cosm WASM")
)
`

const sampleWit = `package cosm:demo;

/// Key-value storage interface for WASM components
interface store {
    /// Result payload for store queries
    record entry {
        key: string,
        val: list<u8>,
        ttl: u64,
    }

    /// Error variants
    variant store-error {
        not-found,
        unauthorized(string),
        io-error(string),
    }

    /// Storage tier enum
    enum storage-tier {
        hot,
        warm,
        cold,
    }

    /// Get value by key
    get: func(key: string) -> result<entry, store-error>;

    /// Set value by key
    set: func(key: string, value: list<u8>) -> result<_, store-error>;
}

/// Component host world
world kv-service {
    import store;
    export store;
}
`

func TestWasmWatParsingAndHydration(t *testing.T) {
	parser := NewWasmParser()
	lineage := core.LineageEnvelope{
		UserPrompt:       "Parse and hydrate WAT test module",
		SessionID:        "sess-wasm-001",
		ExecutingAgentID: "agent-wasm",
	}

	res, err := parser.ParseSource("math.wat", []byte(sampleWat), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	if res.ModuleName != "$math_module" {
		t.Errorf("expected module name '$math_module', got %q", res.ModuleName)
	}

	// Verify Types
	if len(res.Types) != 2 {
		t.Fatalf("expected 2 types, got %d", len(res.Types))
	}
	if res.Types[0].ID != "$binop" {
		t.Errorf("expected type $binop, got %s", res.Types[0].ID)
	}
	if len(res.Types[0].Params) != 2 || len(res.Types[0].Results) != 1 {
		t.Errorf("unexpected params/results for $binop: %+v", res.Types[0])
	}

	// Verify Imports
	if len(res.Imports) != 2 {
		t.Fatalf("expected 2 imports, got %d", len(res.Imports))
	}
	if res.Imports[0].Module != "env" || res.Imports[0].Field != "log_int" {
		t.Errorf("unexpected import 0: %+v", res.Imports[0])
	}

	// Verify Globals
	if len(res.Globals) != 2 {
		t.Fatalf("expected 2 globals, got %d", len(res.Globals))
	}
	if res.Globals[0].Name != "$counter" || !res.Globals[0].Mutable {
		t.Errorf("unexpected global 0: %+v", res.Globals[0])
	}

	// Verify Functions
	if len(res.Functions) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(res.Functions))
	}
	addFn := res.Functions[0]
	if addFn.Name != "$add" {
		t.Errorf("expected func $add, got %s", addFn.Name)
	}
	if len(addFn.ExportNames) == 0 || addFn.ExportNames[0] != "add" {
		t.Errorf("expected export name 'add' for $add func, got %v", addFn.ExportNames)
	}
	if len(addFn.CalledFuncs) == 0 || addFn.CalledFuncs[0] != "$log_int" {
		t.Errorf("expected called func $log_int, got %v", addFn.CalledFuncs)
	}

	// Verify AllSymbols
	if len(res.AllSymbols) == 0 {
		t.Fatalf("expected AST symbol nodes, got 0")
	}

	// Test Hydration of Symbols
	hydrator := NewWasmHydrator()
	for _, sym := range res.AllSymbols {
		hydrated, err := hydrator.HydrateSymbol(sym)
		if err != nil {
			t.Errorf("HydrateSymbol failed for %s (%s): %v", sym.Identifier, sym.NodeType, err)
		}
		if hydrated == "" {
			t.Errorf("HydrateSymbol returned empty string for %s", sym.Identifier)
		}
	}

	// Test Hydration of Full Module
	fullWat := hydrator.HydrateModule(res)
	if !strings.Contains(fullWat, "(module $math_module") {
		t.Errorf("HydrateModule missing header: %s", fullWat)
	}
	if !strings.Contains(fullWat, "(func $add") {
		t.Errorf("HydrateModule missing $add func: %s", fullWat)
	}

	// Re-parse hydrated WAT to verify isomorphism
	res2, err := parser.ParseSource("rehydrated.wat", []byte(fullWat), lineage)
	if err != nil {
		t.Fatalf("Re-parsing hydrated WAT failed: %v", err)
	}
	if len(res2.Functions) != len(res.Functions) {
		t.Errorf("roundtrip functions count mismatch: got %d, expected %d", len(res2.Functions), len(res.Functions))
	}
	if len(res2.Imports) != len(res.Imports) {
		t.Errorf("roundtrip imports count mismatch: got %d, expected %d", len(res2.Imports), len(res.Imports))
	}

	// Test ComponentNode bundling
	comp, err := parser.BuildComponentNode(res, "math-service", core.CompService, lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode failed: %v", err)
	}
	if comp.ComponentID == "" {
		t.Errorf("expected valid ComponentID")
	}
	if comp.Language != core.LangWasm {
		t.Errorf("expected LangWasm, got %s", comp.Language)
	}
	if len(comp.SymbolNodes) != len(res.AllSymbols) {
		t.Errorf("expected %d symbol nodes in component, got %d", len(res.AllSymbols), len(comp.SymbolNodes))
	}
}

func TestWasmWitParsingAndHydration(t *testing.T) {
	parser := NewWasmParser()
	lineage := core.LineageEnvelope{
		UserPrompt:       "Parse and hydrate WIT store interface",
		SessionID:        "sess-wit-001",
		ExecutingAgentID: "agent-wit",
	}

	res, err := parser.ParseSource("store.wit", []byte(sampleWit), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed for WIT: %v", err)
	}

	if len(res.WitInterfaces) != 1 {
		t.Fatalf("expected 1 WIT interface, got %d", len(res.WitInterfaces))
	}
	iface := res.WitInterfaces[0]
	if iface.Name != "store" {
		t.Errorf("expected interface name 'store', got %s", iface.Name)
	}
	if iface.Package != "cosm:demo" {
		t.Errorf("expected package 'cosm:demo', got %s", iface.Package)
	}

	// Records
	if len(iface.Records) != 1 || iface.Records[0].Name != "entry" {
		t.Errorf("unexpected records: %+v", iface.Records)
	}
	if len(iface.Records[0].Fields) != 3 {
		t.Errorf("expected 3 fields in entry record, got %d", len(iface.Records[0].Fields))
	}

	// Variants
	if len(iface.Variants) != 1 || iface.Variants[0].Name != "store-error" {
		t.Errorf("unexpected variants: %+v", iface.Variants)
	}

	// Enums
	if len(iface.Enums) != 1 || iface.Enums[0].Name != "storage-tier" {
		t.Errorf("unexpected enums: %+v", iface.Enums)
	}

	// Functions
	if len(iface.Functions) != 2 {
		t.Fatalf("expected 2 WIT functions, got %d", len(iface.Functions))
	}
	if iface.Functions[0].Name != "get" || iface.Functions[1].Name != "set" {
		t.Errorf("unexpected functions: %+v", iface.Functions)
	}

	// Worlds
	if len(res.WitWorlds) != 1 || res.WitWorlds[0].Name != "kv-service" {
		t.Errorf("unexpected worlds: %+v", res.WitWorlds)
	}

	// Hydrate WIT
	hydrator := NewWasmHydrator()
	hydratedWit := hydrator.HydrateWit(res)
	if !strings.Contains(hydratedWit, "interface store {") {
		t.Errorf("hydrated WIT missing interface: %s", hydratedWit)
	}
	if !strings.Contains(hydratedWit, "world kv-service {") {
		t.Errorf("hydrated WIT missing world: %s", hydratedWit)
	}

	// Re-parse WIT
	res2, err := parser.ParseSource("rehydrated.wit", []byte(hydratedWit), lineage)
	if err != nil {
		t.Fatalf("Re-parsing hydrated WIT failed: %v", err)
	}
	if len(res2.WitInterfaces) != 1 || len(res2.WitWorlds) != 1 {
		t.Errorf("mismatch after WIT re-parsing: %d ifaces, %d worlds", len(res2.WitInterfaces), len(res2.WitWorlds))
	}
}

func TestWasmBinaryEmissionAndParsing(t *testing.T) {
	parser := NewWasmParser()
	lineage := core.LineageEnvelope{
		UserPrompt:       "Binary WASM encoding and decoding test",
		SessionID:        "sess-wasm-bin-001",
		ExecutingAgentID: "agent-wasm",
	}

	res, err := parser.ParseSource("math.wat", []byte(sampleWat), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	hydrator := NewWasmHydrator()
	binBytes, err := hydrator.HydrateToBinary(res)
	if err != nil {
		t.Fatalf("HydrateToBinary failed: %v", err)
	}

	if len(binBytes) < 8 {
		t.Fatalf("binary output too small: %d bytes", len(binBytes))
	}

	// Verify magic \x00asm
	if binBytes[0] != 0x00 || binBytes[1] != 0x61 || binBytes[2] != 0x73 || binBytes[3] != 0x6D {
		t.Errorf("invalid WASM binary magic: %v", binBytes[:4])
	}

	// Parse the binary payload back
	binRes, err := parser.ParseSource("math.wasm", binBytes, lineage)
	if err != nil {
		t.Fatalf("ParseSource on binary WASM failed: %v", err)
	}

	if !binRes.IsBinary {
		t.Errorf("expected IsBinary=true for .wasm")
	}
	if len(binRes.Types) != len(res.Types) {
		t.Errorf("binary types count mismatch: got %d, expected %d", len(binRes.Types), len(res.Types))
	}
	if len(binRes.Imports) != len(res.Imports) {
		t.Errorf("binary imports count mismatch: got %d, expected %d", len(binRes.Imports), len(res.Imports))
	}
	if len(binRes.Functions) != len(res.Functions) {
		t.Errorf("binary functions count mismatch: got %d, expected %d", len(binRes.Functions), len(res.Functions))
	}
}
