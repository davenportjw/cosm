package shipping

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CommandResult records the exit code and outputs of a command executed in staging.
type CommandResult struct {
	Command    string        `json:"command"`
	Args       []string      `json:"args"`
	ExitCode   int           `json:"exit_code"`
	Stdout     string        `json:"stdout"`
	Stderr     string        `json:"stderr"`
	Duration   time.Duration `json:"duration"`
	Successful bool          `json:"successful"`
}

// StagingWorkspace represents an isolated ephemeral filesystem environment for running toolchain builds.
type StagingWorkspace struct {
	Target    *TargetSpec
	Directory string
	CreatedAt time.Time
	mu        sync.RWMutex
	cleanedUp bool
}

// WriteFiles writes a map of relative file paths and file bytes into the staging directory.
func (w *StagingWorkspace) WriteFiles(files map[string][]byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cleanedUp {
		return fmt.Errorf("staging workspace is already cleaned up")
	}

	for relPath, data := range files {
		destPath := filepath.Join(w.Directory, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create staging dir %s: %w", filepath.Dir(destPath), err)
		}
		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write staging file %s: %w", destPath, err)
		}
	}

	return nil
}

// ReadFile reads a file from the staging workspace.
func (w *StagingWorkspace) ReadFile(relPath string) ([]byte, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	destPath := filepath.Join(w.Directory, filepath.FromSlash(relPath))
	return os.ReadFile(destPath)
}

// ExecuteCommand executes a toolchain command within the staging workspace.
func (w *StagingWorkspace) ExecuteCommand(cmdName string, args ...string) (*CommandResult, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cleanedUp {
		return nil, fmt.Errorf("staging workspace is already cleaned up")
	}

	start := time.Now()
	cmd := exec.Command(cmdName, args...)
	cmd.Dir = w.Directory

	// Inject target environment variables
	cmd.Env = os.Environ()
	if w.Target != nil && len(w.Target.Environment) > 0 {
		for k, v := range w.Target.Environment {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return &CommandResult{
		Command:    cmdName,
		Args:       args,
		ExitCode:   exitCode,
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		Duration:   duration,
		Successful: exitCode == 0,
	}, nil
}

// ListFiles lists all relative files in the workspace.
func (w *StagingWorkspace) ListFiles() ([]string, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var files []string
	err := filepath.Walk(w.Directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			rel, _ := filepath.Rel(w.Directory, path)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	return files, err
}

// Cleanup safely removes the temporary staging directory.
func (w *StagingWorkspace) Cleanup() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cleanedUp {
		return nil
	}
	w.cleanedUp = true
	return os.RemoveAll(w.Directory)
}

// AutoStageDependencies copies project dependency files (go.mod, go.sum for Go;
// package.json, tsconfig.json for TypeScript/Node) from the repo root or working tree
// into the staging workspace if not already present.
func (w *StagingWorkspace) AutoStageDependencies(repoRoot string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cleanedUp {
		return fmt.Errorf("staging workspace is already cleaned up")
	}

	if repoRoot == "" {
		repoRoot = FindWorkingTreeRoot()
	}
	if repoRoot == "" {
		return nil
	}

	depFiles := []string{
		"go.mod",
		"go.sum",
		"package.json",
		"tsconfig.json",
	}

	for _, rel := range depFiles {
		destPath := filepath.Join(w.Directory, rel)
		if _, err := os.Stat(destPath); err == nil {
			// Already present in workspace
			continue
		}

		srcPath := filepath.Join(repoRoot, rel)
		if srcInfo, err := os.Stat(srcPath); err == nil && !srcInfo.IsDir() {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return fmt.Errorf("failed to read %s for auto-staging: %w", srcPath, err)
			}
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return fmt.Errorf("failed to create dir for %s: %w", destPath, err)
			}
			if err := os.WriteFile(destPath, data, 0644); err != nil {
				return fmt.Errorf("failed to copy %s to staging: %w", rel, err)
			}
		}
	}

	// Also check component-specific subdirectories if target specifies component names
	if w.Target != nil {
		for _, comp := range w.Target.ComponentNames {
			for _, rel := range depFiles {
				compRel := filepath.Join(comp, rel)
				destPath := filepath.Join(w.Directory, compRel)
				if _, err := os.Stat(destPath); err == nil {
					continue
				}
				srcPath := filepath.Join(repoRoot, compRel)
				if srcInfo, err := os.Stat(srcPath); err == nil && !srcInfo.IsDir() {
					data, err := os.ReadFile(srcPath)
					if err == nil {
						_ = os.MkdirAll(filepath.Dir(destPath), 0755)
						_ = os.WriteFile(destPath, data, 0644)
					}
				}
			}
		}
	}

	return nil
}

// FindWorkingTreeRoot searches the current working directory and its ancestors
// for a repository or project root indicator (.cosm, .git, or go.mod).
func FindWorkingTreeRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, ".cosm")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir || parent == "" {
			break
		}
		dir = parent
	}
	return cwd
}

// StagingManager manages lifecycle and creation of ephemeral staging workspaces.
type StagingManager struct {
	baseTempDir string
	repoRoot    string
}

// NewStagingManager returns a new StagingManager.
func NewStagingManager(baseTempDir string) *StagingManager {
	return NewStagingManagerWithRepo(baseTempDir, "")
}

// NewStagingManagerWithRepo returns a new StagingManager configured with an explicit repository root.
func NewStagingManagerWithRepo(baseTempDir, repoRoot string) *StagingManager {
	if baseTempDir == "" {
		baseTempDir = os.TempDir()
	}
	return &StagingManager{
		baseTempDir: baseTempDir,
		repoRoot:    repoRoot,
	}
}

// SetRepoRoot sets an explicit repository root directory for dependency auto-staging.
func (m *StagingManager) SetRepoRoot(repoRoot string) {
	m.repoRoot = repoRoot
}

// RepoRoot returns the configured repository root, or discovers it via FindWorkingTreeRoot.
func (m *StagingManager) RepoRoot() string {
	if m.repoRoot != "" {
		return m.repoRoot
	}
	return FindWorkingTreeRoot()
}

// Prepare creates a fresh isolated StagingWorkspace and populates it with files.
func (m *StagingManager) Prepare(target *TargetSpec, files map[string][]byte) (*StagingWorkspace, error) {
	prefix := "cosm_stage_"
	if target != nil && target.Name != "" {
		cleanName := strings.ReplaceAll(target.Name, ":", "_")
		prefix = fmt.Sprintf("cosm_stage_%s_", cleanName)
	}

	dir, err := os.MkdirTemp(m.baseTempDir, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to create staging workspace dir: %w", err)
	}

	workspace := &StagingWorkspace{
		Target:    target,
		Directory: dir,
		CreatedAt: time.Now().UTC(),
	}

	if len(files) > 0 {
		if err := workspace.WriteFiles(files); err != nil {
			_ = workspace.Cleanup()
			return nil, err
		}
	}

	// Auto-stage project dependencies (go.mod, go.sum, package.json, tsconfig.json)
	if err := workspace.AutoStageDependencies(m.RepoRoot()); err != nil {
		_ = workspace.Cleanup()
		return nil, fmt.Errorf("failed to auto-stage project dependencies: %w", err)
	}

	return workspace, nil
}
