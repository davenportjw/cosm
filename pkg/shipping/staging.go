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

// StagingManager manages lifecycle and creation of ephemeral staging workspaces.
type StagingManager struct {
	baseTempDir string
}

// NewStagingManager returns a new StagingManager.
func NewStagingManager(baseTempDir string) *StagingManager {
	if baseTempDir == "" {
		baseTempDir = os.TempDir()
	}
	return &StagingManager{
		baseTempDir: baseTempDir,
	}
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

	return workspace, nil
}
