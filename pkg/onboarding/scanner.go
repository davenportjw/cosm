package onboarding

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// ProjectManifestInfo holds detected polyglot project markers in a directory.
type ProjectManifestInfo struct {
	HasGoMod        bool
	HasPackageJSON  bool
	HasRequirements bool
	HasPyproject    bool
	HasTerraform    bool
	ComponentType   core.ComponentType
	PrimaryLanguage core.Language
}

// DirectoryScanner scans an existing repository directory, honoring exclusions and detecting polyglot boundaries.
type DirectoryScanner struct {
	ExcludedDirs map[string]bool
}

// NewDirectoryScanner initializes a new scanner with default exclusions.
func NewDirectoryScanner() *DirectoryScanner {
	return &DirectoryScanner{
		ExcludedDirs: map[string]bool{
			".git":          true,
			".cosm":         true,
			".fg":           true,
			"node_modules":  true,
			"vendor":        true,
			".terraform":    true,
			"__pycache__":   true,
			".pytest_cache": true,
			"dist":          true,
			"build":         true,
		},
	}
}

// ScanDirectory walks rootDir and categorizes discovered files into code files and raw blob files.
func (s *DirectoryScanner) ScanDirectory(rootDir string) ([]string, []string, error) {
	var codeFiles []string
	var rawFiles []string

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(rootDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		if info.IsDir() {
			base := filepath.Base(path)
			if s.ExcludedDirs[base] {
				return filepath.SkipDir
			}
			return nil
		}

		base := filepath.Base(path)
		ext := strings.ToLower(filepath.Ext(path))
		if base == "Dockerfile" || base == "Containerfile" || strings.HasPrefix(base, "Dockerfile.") {
			codeFiles = append(codeFiles, path)
			return nil
		}

		switch ext {
		case ".go", ".tf", ".hcl", ".ts", ".tsx", ".js", ".jsx", ".py",
			".rs", ".java", ".cpp", ".cc", ".cxx", ".c", ".h", ".hpp",
			".sql", ".proto", ".swift", ".kt", ".kts", ".cs",
			".wat", ".wasm", ".wit", ".zig", ".graphql", ".gql",
			".rb", ".php", ".ex", ".exs":
			codeFiles = append(codeFiles, path)
		default:
			rawFiles = append(rawFiles, path)
		}
		return nil
	})

	return codeFiles, rawFiles, err
}

// DetectManifests inspects a directory for build and dependency manifests to classify component boundaries.
func (s *DirectoryScanner) DetectManifests(dirPath string) ProjectManifestInfo {
	info := ProjectManifestInfo{
		ComponentType:   core.CompService,
		PrimaryLanguage: core.LangGo,
	}

	if _, err := os.Stat(filepath.Join(dirPath, "go.mod")); err == nil {
		info.HasGoMod = true
		info.ComponentType = core.CompService
		info.PrimaryLanguage = core.LangGo
	}
	if _, err := os.Stat(filepath.Join(dirPath, "package.json")); err == nil {
		info.HasPackageJSON = true
		info.ComponentType = core.CompFrontend
		info.PrimaryLanguage = core.LangTypeScript
	}
	if _, err := os.Stat(filepath.Join(dirPath, "requirements.txt")); err == nil {
		info.HasRequirements = true
		info.ComponentType = core.CompService
		info.PrimaryLanguage = core.LangPython
	}
	if _, err := os.Stat(filepath.Join(dirPath, "pyproject.toml")); err == nil {
		info.HasPyproject = true
		info.ComponentType = core.CompService
		info.PrimaryLanguage = core.LangPython
	}
	if _, err := os.Stat(filepath.Join(dirPath, "Cargo.toml")); err == nil {
		info.ComponentType = core.CompService
		info.PrimaryLanguage = core.LangRust
	}
	if _, err := os.Stat(filepath.Join(dirPath, "Package.swift")); err == nil {
		info.ComponentType = core.CompService
		info.PrimaryLanguage = core.LangSwift
	}
	if _, err := os.Stat(filepath.Join(dirPath, "build.gradle.kts")); err == nil {
		info.ComponentType = core.CompService
		info.PrimaryLanguage = core.LangKotlin
	}
	if _, err := os.Stat(filepath.Join(dirPath, "build.zig")); err == nil {
		info.ComponentType = core.CompService
		info.PrimaryLanguage = core.LangZig
	}
	if _, err := os.Stat(filepath.Join(dirPath, "Gemfile")); err == nil {
		info.ComponentType = core.CompService
		info.PrimaryLanguage = core.LangRuby
	}
	if _, err := os.Stat(filepath.Join(dirPath, "composer.json")); err == nil {
		info.ComponentType = core.CompService
		info.PrimaryLanguage = core.LangPHP
	}
	if _, err := os.Stat(filepath.Join(dirPath, "mix.exs")); err == nil {
		info.ComponentType = core.CompService
		info.PrimaryLanguage = core.LangElixir
	}

	// Check for any .tf files
	entries, err := os.ReadDir(dirPath)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".tf") {
				info.HasTerraform = true
				info.ComponentType = core.CompInfra
				info.PrimaryLanguage = core.LangHCL
				break
			}
		}
	}

	return info
}
