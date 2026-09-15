package codecs

import (
	"errors"
	"path/filepath"
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
	"github.com/cosmscm/cosm/pkg/codecs/python"
	"github.com/cosmscm/cosm/pkg/codecs/ruby"
	"github.com/cosmscm/cosm/pkg/codecs/rust"
	"github.com/cosmscm/cosm/pkg/codecs/sql"
	"github.com/cosmscm/cosm/pkg/codecs/swift"
	"github.com/cosmscm/cosm/pkg/codecs/typescript"
	"github.com/cosmscm/cosm/pkg/codecs/wasm"
	"github.com/cosmscm/cosm/pkg/codecs/zig"
	"github.com/cosmscm/cosm/pkg/core"
)

// ErrUnsupportedLanguage is returned when no AST codec exists for the file type.
var ErrUnsupportedLanguage = errors.New("unsupported language for AST parsing")

// ParsedFileResult bundles the created ComponentNode and all extracted ASTSymbolNodes.
type ParsedFileResult struct {
	Component *core.ComponentNode
	Symbols   map[string]*core.ASTSymbolNode
}

// SupportsExtension returns true if the extension has a dedicated AST codec.
func SupportsExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case ".go", ".py", ".tf", ".hcl", ".ts", ".tsx", ".js", ".jsx",
		".rs", ".java", ".cpp", ".cc", ".cxx", ".c", ".h", ".hpp",
		".cs", ".swift", ".kt", ".kts", ".sql", ".proto", ".protobuf",
		".zig", ".wat", ".wasm", ".graphql", ".gql", ".rb", ".php",
		".ex", ".exs", ".dockerfile":
		return true
	default:
		return false
	}
}

// IsDockerfile checks if a filename represents a Dockerfile/Containerfile.
func IsDockerfile(filename string) bool {
	base := strings.ToLower(filepath.Base(filename))
	return base == "dockerfile" || base == "containerfile" || strings.HasPrefix(base, "dockerfile.") || strings.HasSuffix(base, ".dockerfile")
}

// ParseSourceFile parses a source file into a ComponentNode and ASTSymbolNodes using the appropriate codec.
func ParseSourceFile(relPath string, content []byte, lineageEnv core.LineageEnvelope) (*ParsedFileResult, error) {
	res, err := parseSourceFileInternal(relPath, content, lineageEnv)
	if err != nil {
		return nil, err
	}
	return attachFilePath(res, relPath), nil
}

func attachFilePath(res *ParsedFileResult, relPath string) *ParsedFileResult {
	if res == nil {
		return nil
	}
	if res.Component != nil {
		if res.Component.Metadata == nil {
			res.Component.Metadata = make(map[string]string)
		}
		res.Component.Metadata["file_path"] = relPath
	}
	for _, s := range res.Symbols {
		if s != nil {
			if s.ASTMetadata == nil {
				s.ASTMetadata = make(map[string]string)
			}
			s.ASTMetadata["file_path"] = relPath
		}
	}
	return res
}

func parseSourceFileInternal(relPath string, content []byte, lineageEnv core.LineageEnvelope) (*ParsedFileResult, error) {
	baseName := filepath.Base(relPath)
	ext := strings.ToLower(filepath.Ext(relPath))

	if IsDockerfile(relPath) {
		p := dockerfile.NewDockerfileParser()
		res, err := p.ParseSource(relPath, content, lineageEnv)
		if err != nil {
			return nil, err
		}
		comp, err := p.BuildComponentNode(res, baseName, core.CompInfra, lineageEnv)
		if err != nil {
			return nil, err
		}
		syms := make(map[string]*core.ASTSymbolNode, len(res.Symbols))
		for _, s := range res.Symbols {
			syms[s.NodeID] = s
		}
		return &ParsedFileResult{Component: comp, Symbols: syms}, nil
	}

	switch ext {
	case ".go":
		p := golang.NewGoParser()
		pkgRes, err := p.ParseSource(relPath, content, lineageEnv)
		if err != nil {
			return nil, err
		}
		comp, err := golang.BuildComponentNode(baseName, core.CompService, pkgRes, lineageEnv)
		if err != nil {
			return nil, err
		}
		syms := make(map[string]*core.ASTSymbolNode, len(pkgRes.AllSymbols))
		for _, s := range pkgRes.AllSymbols {
			syms[s.NodeID] = s
		}
		return &ParsedFileResult{Component: comp, Symbols: syms}, nil

	case ".py":
		p := python.NewPythonParser()
		res, err := p.ParseSource(relPath, content, lineageEnv)
		if err != nil {
			return nil, err
		}
		comp, symList, err := p.BuildComponentNode(res, baseName, lineageEnv)
		if err != nil {
			return nil, err
		}
		syms := make(map[string]*core.ASTSymbolNode, len(symList))
		for _, s := range symList {
			syms[s.NodeID] = s
		}
		return &ParsedFileResult{Component: comp, Symbols: syms}, nil

	case ".tf", ".hcl":
		p := hcl.NewHCLParser()
		doc, err := p.ParseSource(relPath, content)
		if err != nil {
			return nil, err
		}
		comp, symList, err := p.BuildComponentNode(doc, baseName, lineageEnv)
		if err != nil {
			return nil, err
		}
		syms := make(map[string]*core.ASTSymbolNode, len(symList))
		for _, s := range symList {
			syms[s.NodeID] = s
		}
		return &ParsedFileResult{Component: comp, Symbols: syms}, nil

	case ".ts", ".tsx", ".js", ".jsx":
		p := typescript.NewTSParser()
		res, err := p.ParseSource(relPath, content, lineageEnv)
		if err != nil {
			return nil, err
		}
		comp, symList, err := p.BuildComponentNode(res, baseName, lineageEnv)
		if err != nil {
			return nil, err
		}
		syms := make(map[string]*core.ASTSymbolNode, len(symList))
		for _, s := range symList {
			syms[s.NodeID] = s
		}
		return &ParsedFileResult{Component: comp, Symbols: syms}, nil

	case ".rs":
		p := rust.NewRustParser()
		res, err := p.ParseSource(relPath, content, lineageEnv)
		if err != nil {
			return nil, err
		}
		comp, err := p.BuildComponentNode(res, baseName, core.CompService, lineageEnv)
		if err != nil {
			return nil, err
		}
		syms := make(map[string]*core.ASTSymbolNode, len(res.AllSymbols))
		for _, s := range res.AllSymbols {
			syms[s.NodeID] = s
		}
		return &ParsedFileResult{Component: comp, Symbols: syms}, nil

	case ".java":
		p := java.NewJavaParser()
		res, err := p.ParseSource(relPath, content, lineageEnv)
		if err != nil {
			return nil, err
		}
		comp, err := p.BuildComponentNode(res, baseName, core.CompService, lineageEnv)
		if err != nil {
			return nil, err
		}
		syms := make(map[string]*core.ASTSymbolNode, len(res.AllSymbols))
		for _, s := range res.AllSymbols {
			syms[s.NodeID] = s
		}
		return &ParsedFileResult{Component: comp, Symbols: syms}, nil

	case ".cpp", ".cc", ".cxx", ".c", ".h", ".hpp":
		p := cpp.NewCppParser()
		res, err := p.ParseSource(relPath, content, lineageEnv)
		if err != nil {
			return nil, err
		}
		comp, err := p.BuildComponentNode(res, baseName, core.CompService, lineageEnv)
		if err != nil {
			return nil, err
		}
		syms := make(map[string]*core.ASTSymbolNode, len(res.AllSymbols))
		for _, s := range res.AllSymbols {
			syms[s.NodeID] = s
		}
		return &ParsedFileResult{Component: comp, Symbols: syms}, nil

	case ".cs":
		p := csharp.NewCSharpParser()
		res, err := p.ParseSource(relPath, content, lineageEnv)
		if err != nil {
			return nil, err
		}
		comp, err := p.BuildComponentNode(res, baseName, core.CompService, lineageEnv)
		if err != nil {
			return nil, err
		}
		syms := make(map[string]*core.ASTSymbolNode, len(res.AllSymbols))
		for _, s := range res.AllSymbols {
			syms[s.NodeID] = s
		}
		return &ParsedFileResult{Component: comp, Symbols: syms}, nil

	case ".swift":
		p := swift.NewSwiftParser()
		res, err := p.ParseSource(relPath, content, lineageEnv)
		if err != nil {
			return nil, err
		}
		comp, err := p.BuildComponentNode(res, baseName, core.CompFrontend, lineageEnv)
		if err != nil {
			return nil, err
		}
		syms := make(map[string]*core.ASTSymbolNode, len(res.AllSymbols))
		for _, s := range res.AllSymbols {
			syms[s.NodeID] = s
		}
		return &ParsedFileResult{Component: comp, Symbols: syms}, nil

	case ".kt", ".kts":
		p := kotlin.NewKotlinParser()
		res, err := p.ParseSource(relPath, content, lineageEnv)
		if err != nil {
			return nil, err
		}
		comp, err := p.BuildComponentNode(res, baseName, core.CompService, lineageEnv)
		if err != nil {
			return nil, err
		}
		syms := make(map[string]*core.ASTSymbolNode, len(res.AllSymbols))
		for _, s := range res.AllSymbols {
			syms[s.NodeID] = s
		}
		return &ParsedFileResult{Component: comp, Symbols: syms}, nil

	case ".sql":
		p := sql.NewSQLParser()
		res, err := p.ParseSource(relPath, content, lineageEnv)
		if err != nil {
			return nil, err
		}
		comp, err := p.BuildComponentNode(res, baseName, core.CompDatabase, lineageEnv)
		if err != nil {
			return nil, err
		}
		syms := make(map[string]*core.ASTSymbolNode, len(res.AllSymbols))
		for _, s := range res.AllSymbols {
			syms[s.NodeID] = s
		}
		return &ParsedFileResult{Component: comp, Symbols: syms}, nil

	case ".proto", ".protobuf":
		p := protobuf.NewProtobufParser()
		res, err := p.ParseSource(relPath, content, lineageEnv)
		if err != nil {
			return nil, err
		}
		comp, err := p.BuildComponentNode(res, baseName, core.CompContract, lineageEnv)
		if err != nil {
			return nil, err
		}
		syms := make(map[string]*core.ASTSymbolNode, len(res.AllSymbols))
		for _, s := range res.AllSymbols {
			syms[s.NodeID] = s
		}
		return &ParsedFileResult{Component: comp, Symbols: syms}, nil

	case ".zig":
		p := zig.NewZigParser()
		res, err := p.ParseSource(relPath, content, lineageEnv)
		if err != nil {
			return nil, err
		}
		comp, err := p.BuildComponentNode(res, baseName, core.CompService, lineageEnv)
		if err != nil {
			return nil, err
		}
		syms := make(map[string]*core.ASTSymbolNode, len(res.AllSymbols))
		for _, s := range res.AllSymbols {
			syms[s.NodeID] = s
		}
		return &ParsedFileResult{Component: comp, Symbols: syms}, nil

	case ".wat", ".wasm":
		p := wasm.NewWasmParser()
		res, err := p.ParseSource(relPath, content, lineageEnv)
		if err != nil {
			return nil, err
		}
		comp, err := p.BuildComponentNode(res, baseName, core.CompService, lineageEnv)
		if err != nil {
			return nil, err
		}
		syms := make(map[string]*core.ASTSymbolNode, len(res.AllSymbols))
		for _, s := range res.AllSymbols {
			syms[s.NodeID] = s
		}
		return &ParsedFileResult{Component: comp, Symbols: syms}, nil

	case ".graphql", ".gql":
		p := graphql.NewGraphQLParser()
		res, err := p.ParseSource(relPath, content, lineageEnv)
		if err != nil {
			return nil, err
		}
		comp, err := p.BuildComponentNode(res, baseName, core.CompContract, lineageEnv)
		if err != nil {
			return nil, err
		}
		syms := make(map[string]*core.ASTSymbolNode, len(res.AllSymbols))
		for _, s := range res.AllSymbols {
			syms[s.NodeID] = s
		}
		return &ParsedFileResult{Component: comp, Symbols: syms}, nil

	case ".rb":
		p := ruby.NewRubyParser()
		res, err := p.ParseSource(relPath, content, lineageEnv)
		if err != nil {
			return nil, err
		}
		comp, err := p.BuildComponentNode(res, baseName, core.CompService, lineageEnv)
		if err != nil {
			return nil, err
		}
		syms := make(map[string]*core.ASTSymbolNode, len(res.AllSymbols))
		for _, s := range res.AllSymbols {
			syms[s.NodeID] = s
		}
		return &ParsedFileResult{Component: comp, Symbols: syms}, nil

	case ".php":
		p := php.NewPHPParser()
		res, err := p.ParseSource(relPath, content, lineageEnv)
		if err != nil {
			return nil, err
		}
		comp, err := p.BuildComponentNode(res, baseName, core.CompService, lineageEnv)
		if err != nil {
			return nil, err
		}
		syms := make(map[string]*core.ASTSymbolNode, len(res.AllSymbols))
		for _, s := range res.AllSymbols {
			syms[s.NodeID] = s
		}
		return &ParsedFileResult{Component: comp, Symbols: syms}, nil

	case ".ex", ".exs":
		p := elixir.NewElixirParser()
		res, err := p.ParseSource(relPath, content, lineageEnv)
		if err != nil {
			return nil, err
		}
		comp, err := p.BuildComponentNode(res, baseName, core.CompService, lineageEnv)
		if err != nil {
			return nil, err
		}
		syms := make(map[string]*core.ASTSymbolNode, len(res.AllSymbols))
		for _, s := range res.AllSymbols {
			syms[s.NodeID] = s
		}
		return &ParsedFileResult{Component: comp, Symbols: syms}, nil

	default:
		return nil, ErrUnsupportedLanguage
	}
}
