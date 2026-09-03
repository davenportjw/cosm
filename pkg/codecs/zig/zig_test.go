package zig

import (
	"strings"
	"testing"

	"github.com/cosmscm/cosm/pkg/core"
)

const sampleZigSource = `//! Cosm High-Performance Zig AST Sample Module
//! Demonstrates zero-CGO polyglot AST extraction.

const std = @import("std");
const c = @cImport({
    @cInclude("stdlib.h");
});

/// Network errors encountered during AST packet routing
pub const NetworkError = error{
    Timeout,
    ConnectionRefused,
    HostUnreachable,
};

/// Server runtime configuration
pub const ServerConfig = struct {
    /// Host address
    host: []const u8 = "127.0.0.1",
    /// Port number
    port: u16 = 8080,
    /// Keep-alive flag
    keep_alive: bool = true,

    /// Initialize default server configuration
    pub fn init(host: []const u8, port: u16) ServerConfig {
        return ServerConfig{
            .host = host,
            .port = port,
            .keep_alive = true,
        };
    }
};

/// Wire protocol header
pub const Header = packed struct {
    version: u4,
    flags: u4,
    length: u16,
};

/// Logging verbosity levels
pub const LogLevel = enum(u8) {
    debug = 0,
    info = 1,
    warn = 2,
    err = 3,
};

/// Event message variants
pub const Message = union(enum) {
    ping: u64,
    text: []const u8,
    disconnect: void,
};

/// Maximum packet size in bytes
pub const MAX_PACKET_SIZE: usize = 65536;

/// Global atomic sequence counter
pub comptime var SEQUENCE_COUNTER: usize = 0;

/// Fast inline multiplier
pub inline fn fastMultiply(x: u32, y: u32) u32 {
    return x * y;
}

/// Start high-performance network server
pub fn startServer(config: ServerConfig, allocator: std.mem.Allocator) !void {
    std.debug.print("Starting server on {s}:{d}\n", .{ config.host, config.port });
    _ = allocator;
}

/// Exported C-ABI calculation entrypoint
export fn compute_hash(seed: u64, data_ptr: [*]const u8, len: usize) u64 {
    _ = data_ptr;
    return seed + len;
}

test "server configuration initialization" {
    const cfg = ServerConfig.init("0.0.0.0", 9000);
    try std.testing.expectEqualStrings("0.0.0.0", cfg.host);
    try std.testing.expectEqual(@as(u16, 9000), cfg.port);
}
`

func TestZigParsingAndHydration(t *testing.T) {
	parser := NewZigParser()
	lineage := core.LineageEnvelope{
		UserPrompt:       "Parse and hydrate Zig server module",
		SessionID:        "sess-zig-001",
		ExecutingAgentID: "agent-zig",
	}

	res, err := parser.ParseSource("server.zig", []byte(sampleZigSource), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	// 1. Verify Module Docstring
	if !strings.Contains(res.Docstring, "Cosm High-Performance Zig AST") {
		t.Errorf("expected module docstring, got %q", res.Docstring)
	}

	// 2. Verify Imports
	if len(res.Imports) != 2 {
		t.Fatalf("expected 2 imports, got %d", len(res.Imports))
	}
	if res.Imports[0].Alias != "std" || res.Imports[0].Path != "std" {
		t.Errorf("unexpected import 0: %+v", res.Imports[0])
	}
	if res.Imports[1].Alias != "c" || !res.Imports[1].IsCImport {
		t.Errorf("unexpected cImport 1: %+v", res.Imports[1])
	}

	// 3. Verify ErrorSets
	if len(res.ErrorSets) != 1 {
		t.Fatalf("expected 1 error set, got %d", len(res.ErrorSets))
	}
	errSet := res.ErrorSets[0]
	if errSet.Name != "NetworkError" || len(errSet.Errors) != 3 {
		t.Errorf("unexpected error set: %+v", errSet)
	}

	// 4. Verify Structs
	if len(res.Structs) != 2 {
		t.Fatalf("expected 2 structs, got %d", len(res.Structs))
	}
	stConfig := res.Structs[0]
	if stConfig.Name != "ServerConfig" || len(stConfig.Fields) != 3 || len(stConfig.Methods) != 1 {
		t.Errorf("unexpected ServerConfig struct: %+v", stConfig)
	}
	if stConfig.Methods[0].Name != "init" {
		t.Errorf("expected init method in ServerConfig, got %s", stConfig.Methods[0].Name)
	}

	headerSt := res.Structs[1]
	if headerSt.Name != "Header" || headerSt.Kind != "packed struct" || len(headerSt.Fields) != 3 {
		t.Errorf("unexpected Header packed struct: %+v", headerSt)
	}

	// 5. Verify Enums
	if len(res.Enums) != 1 {
		t.Fatalf("expected 1 enum, got %d", len(res.Enums))
	}
	en := res.Enums[0]
	if en.Name != "LogLevel" || en.TagType != "u8" || len(en.Fields) != 4 {
		t.Errorf("unexpected LogLevel enum: %+v", en)
	}

	// 6. Verify Unions
	if len(res.Unions) != 1 {
		t.Fatalf("expected 1 union, got %d", len(res.Unions))
	}
	un := res.Unions[0]
	if un.Name != "Message" || un.TagType != "enum" || len(un.Fields) != 3 {
		t.Errorf("unexpected Message union: %+v", un)
	}

	// 7. Verify Variables
	if len(res.Variables) != 2 {
		t.Fatalf("expected 2 variables/constants, got %d", len(res.Variables))
	}
	if res.Variables[0].Name != "MAX_PACKET_SIZE" || !res.Variables[0].IsConst {
		t.Errorf("unexpected var 0: %+v", res.Variables[0])
	}
	if res.Variables[1].Name != "SEQUENCE_COUNTER" || !res.Variables[1].IsComptime {
		t.Errorf("unexpected var 1: %+v", res.Variables[1])
	}

	// 8. Verify Functions
	if len(res.Functions) != 3 {
		t.Fatalf("expected 3 top-level functions, got %d", len(res.Functions))
	}
	fnMult := res.Functions[0]
	if fnMult.Name != "fastMultiply" || !fnMult.IsInline {
		t.Errorf("unexpected fastMultiply fn: %+v", fnMult)
	}
	fnServer := res.Functions[1]
	if fnServer.Name != "startServer" || fnServer.Visibility != "public" {
		t.Errorf("unexpected startServer fn: %+v", fnServer)
	}
	fnHash := res.Functions[2]
	if fnHash.Name != "compute_hash" || !fnHash.IsExport {
		t.Errorf("unexpected compute_hash fn: %+v", fnHash)
	}

	// 9. Verify Tests
	if len(res.Tests) != 1 {
		t.Fatalf("expected 1 test, got %d", len(res.Tests))
	}
	if res.Tests[0].Name != "server configuration initialization" {
		t.Errorf("unexpected test name: %s", res.Tests[0].Name)
	}

	// 10. Verify Symbol Nodes
	if len(res.AllSymbols) == 0 {
		t.Fatalf("expected AST symbol nodes, got 0")
	}

	// 11. Test Hydration of Symbols
	hydrator := NewZigHydrator()
	for _, sym := range res.AllSymbols {
		hydrated, err := hydrator.HydrateSymbol(sym)
		if err != nil {
			t.Errorf("HydrateSymbol failed for %s (%s): %v", sym.Identifier, sym.NodeType, err)
		}
		if hydrated == "" {
			t.Errorf("HydrateSymbol returned empty string for %s", sym.Identifier)
		}
	}

	// 12. Test Full File Hydration
	hydratedCode := hydrator.HydrateFile(res)
	if !strings.Contains(hydratedCode, "pub const ServerConfig = struct {") {
		t.Errorf("HydrateFile missing ServerConfig struct: %s", hydratedCode)
	}
	if !strings.Contains(hydratedCode, "pub const LogLevel = enum(u8) {") {
		t.Errorf("HydrateFile missing LogLevel enum: %s", hydratedCode)
	}
	if !strings.Contains(hydratedCode, "pub fn startServer") {
		t.Errorf("HydrateFile missing startServer function: %s", hydratedCode)
	}

	// 13. Re-parse Hydrated Code to verify roundtrip isomorphism
	res2, err := parser.ParseSource("rehydrated.zig", []byte(hydratedCode), lineage)
	if err != nil {
		t.Fatalf("Re-parsing hydrated Zig code failed: %v", err)
	}

	if len(res2.Structs) != len(res.Structs) {
		t.Errorf("roundtrip structs mismatch: got %d, expected %d", len(res2.Structs), len(res.Structs))
	}
	if len(res2.Enums) != len(res.Enums) {
		t.Errorf("roundtrip enums mismatch: got %d, expected %d", len(res2.Enums), len(res.Enums))
	}
	if len(res2.Unions) != len(res.Unions) {
		t.Errorf("roundtrip unions mismatch: got %d, expected %d", len(res2.Unions), len(res.Unions))
	}
	if len(res2.Functions) != len(res.Functions) {
		t.Errorf("roundtrip functions mismatch: got %d, expected %d", len(res2.Functions), len(res.Functions))
	}
	if len(res2.ErrorSets) != len(res.ErrorSets) {
		t.Errorf("roundtrip error sets mismatch: got %d, expected %d", len(res2.ErrorSets), len(res.ErrorSets))
	}
	if len(res2.Tests) != len(res.Tests) {
		t.Errorf("roundtrip tests mismatch: got %d, expected %d", len(res2.Tests), len(res.Tests))
	}

	// 14. Test ComponentNode bundling
	comp, err := parser.BuildComponentNode(res, "server-engine", core.CompService, lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode failed: %v", err)
	}
	if comp.ComponentID == "" {
		t.Errorf("expected valid ComponentID")
	}
	if comp.Language != core.LangZig {
		t.Errorf("expected LangZig, got %s", comp.Language)
	}
	if len(comp.SymbolNodes) != len(res.AllSymbols) {
		t.Errorf("expected %d symbol nodes in component, got %d", len(res.AllSymbols), len(comp.SymbolNodes))
	}
}
