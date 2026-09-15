package ignore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// DefaultExcludedDirs are directories ignored by default in every Cosm workspace.
var DefaultExcludedDirs = []string{
	".git",
	".cosm",
	".fg",
	"node_modules",
	"vendor",
	".terraform",
	"__pycache__",
	".pytest_cache",
	"dist",
	"build",
	"target",
	".venv",
	".idea",
	".vscode",
}

// DefaultSecretPatterns are file patterns ignored by default to prevent secret leakage.
var DefaultSecretPatterns = []string{
	".env",
	".env.*",
	"*.env",
	"*.pem",
	"*.key",
	"*id_rsa*",
	"*id_ed25519*",
	"*id_ecdsa*",
	"*id_dsa*",
	"*credentials*.json",
	"*service_account*.json",
	"*service-account*.json",
	"*.tfvars",
	"*.pfx",
	"*.p12",
}

// PatternRule represents a parsed ignore pattern.
type PatternRule struct {
	Pattern  string
	Negation bool
	DirOnly  bool
	Anchored bool
	Raw      string
}

// IgnoreEngine manages file, directory, and secret exclusions for a workspace.
type IgnoreEngine struct {
	WorkspaceDir string
	Rules        []PatternRule
	EdgeFilter   *EdgeFilter
	Detector     *SecretDetector
}

// NewIgnoreEngine creates an IgnoreEngine pre-populated with built-in default rules.
func NewIgnoreEngine(workspaceDir string) *IgnoreEngine {
	engine := &IgnoreEngine{
		WorkspaceDir: workspaceDir,
		Rules:        make([]PatternRule, 0),
		EdgeFilter:   NewEdgeFilter(),
		Detector:     NewSecretDetector(),
	}

	// Add default directory exclusions
	for _, dir := range DefaultExcludedDirs {
		engine.AddRule(dir+"/", false)
		engine.AddRule(dir, false)
	}

	// Add default secret file patterns
	for _, pattern := range DefaultSecretPatterns {
		engine.AddRule(pattern, false)
	}

	return engine
}

// LoadWorkspaceRules loads rules from .cosmignore (or .gitignore if .cosmignore is missing).
func LoadWorkspaceRules(workspaceDir string) (*IgnoreEngine, error) {
	engine := NewIgnoreEngine(workspaceDir)

	cosmIgnorePath := filepath.Join(workspaceDir, ".cosmignore")
	gitIgnorePath := filepath.Join(workspaceDir, ".gitignore")

	var targetPath string
	if _, err := os.Stat(cosmIgnorePath); err == nil {
		targetPath = cosmIgnorePath
	} else if _, err := os.Stat(gitIgnorePath); err == nil {
		targetPath = gitIgnorePath
	}

	if targetPath != "" {
		if err := engine.LoadFromFile(targetPath); err != nil {
			return nil, err
		}
	}

	return engine, nil
}

// LoadFromFile parses a .cosmignore or .gitignore file.
func (e *IgnoreEngine) LoadFromFile(filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inEdgesSection := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for sections
		if strings.EqualFold(line, "[edges]") {
			inEdgesSection = true
			continue
		} else if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inEdgesSection = false
			continue
		}

		if inEdgesSection || strings.HasPrefix(line, "edge:") {
			e.EdgeFilter.ParseRuleLine(line)
			continue
		}

		e.AddRule(line, false)
	}

	return scanner.Err()
}

// AddRule parses and appends an ignore pattern string.
func (e *IgnoreEngine) AddRule(pattern string, prepend bool) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || strings.HasPrefix(pattern, "#") {
		return
	}

	negation := false
	if strings.HasPrefix(pattern, "!") {
		negation = true
		pattern = strings.TrimPrefix(pattern, "!")
	}

	dirOnly := false
	if strings.HasSuffix(pattern, "/") {
		dirOnly = true
		pattern = strings.TrimSuffix(pattern, "/")
	}

	anchored := false
	if strings.HasPrefix(pattern, "/") {
		anchored = true
		pattern = strings.TrimPrefix(pattern, "/")
	}

	rule := PatternRule{
		Pattern:  pattern,
		Negation: negation,
		DirOnly:  dirOnly,
		Anchored: anchored,
		Raw:      pattern,
	}

	if prepend {
		e.Rules = append([]PatternRule{rule}, e.Rules...)
	} else {
		e.Rules = append(e.Rules, rule)
	}
}

// ShouldIgnorePath checks if a relative file or directory path matches ignore rules.
func (e *IgnoreEngine) ShouldIgnorePath(relPath string, isDir bool) bool {
	// Normalize path separators to forward slash
	clean := filepath.ToSlash(filepath.Clean(relPath))
	if clean == "." || clean == "" {
		return false
	}
	clean = strings.TrimPrefix(clean, "./")

	ignored := false

	for _, rule := range e.Rules {
		if rule.DirOnly && !isDir {
			continue
		}

		if matchPathPattern(clean, rule.Pattern, rule.Anchored, isDir) {
			if rule.Negation {
				ignored = false
			} else {
				ignored = true
			}
		}
	}

	return ignored
}

// matchPathPattern matches a relative path against a gitignore pattern.
func matchPathPattern(path string, pattern string, anchored bool, isDir bool) bool {
	pattern = filepath.ToSlash(pattern)

	// If pattern contains a slash (other than leading/trailing), it is implicitly anchored
	if strings.Contains(pattern, "/") {
		anchored = true
	}

	if anchored {
		return globMatch(path, pattern) || (isDir && strings.HasPrefix(path, pattern+"/"))
	}

	// Unanchored pattern: can match the basename or any segment
	base := filepath.Base(path)
	if globMatch(base, pattern) {
		return true
	}

	// Check if any path segment or tail matches
	segments := strings.Split(path, "/")
	for _, seg := range segments {
		if globMatch(seg, pattern) {
			return true
		}
	}

	return globMatch(path, pattern)
}

// globMatch evaluates wildcard globs including `**`, `*`, and `?`.
func globMatch(str, pattern string) bool {
	if pattern == "**" || pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "**") {
		matched, err := filepath.Match(pattern, str)
		return err == nil && matched
	}

	// Handle `**` wildcard
	parts := strings.Split(pattern, "**")
	if len(parts) == 2 {
		prefix := parts[0]
		suffix := parts[1]

		if prefix != "" && !strings.HasPrefix(str, strings.TrimSuffix(prefix, "/")) {
			return false
		}
		if suffix != "" {
			cleanSuffix := strings.TrimPrefix(suffix, "/")
			if cleanSuffix != "" && !strings.HasSuffix(str, cleanSuffix) {
				return false
			}
		}
		return true
	}

	matched, err := filepath.Match(pattern, str)
	return err == nil && matched
}
