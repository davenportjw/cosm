package materialize

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

// ExportedFileInfo captures metadata for each materialized file written to disk.
type ExportedFileInfo struct {
	RelativePath string `json:"relative_path"`
	AbsolutePath string `json:"absolute_path"`
	SizeBytes    int64  `json:"size_bytes"`
	SHA256       string `json:"sha256"`
	IsNew        bool   `json:"is_new"`
	Modified     bool   `json:"modified"`
}

// ExportReport summarizes a disk materialization operation.
type ExportReport struct {
	TargetDirectory string             `json:"target_directory"`
	FilesWritten    int                `json:"files_written"`
	TotalBytes      int64              `json:"total_bytes"`
	Files           []ExportedFileInfo `json:"files"`
	DurationMs      int64              `json:"duration_ms"`
	Timestamp       time.Time          `json:"timestamp"`
}

// Exporter writes in-memory or AST-reconstituted files into real disk directories.
type Exporter struct {
	hydrator *Hydrator
}

// NewExporter creates an Exporter instance.
func NewExporter() *Exporter {
	return &Exporter{
		hydrator: NewHydrator(),
	}
}

// ExportToDisk writes a map of relative file paths and content bytes to the specified target directory.
func (e *Exporter) ExportToDisk(targetDir string, files map[string][]byte, overwrite bool) (*ExportReport, error) {
	start := time.Now()

	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, fmt.Errorf("invalid target directory: %w", err)
	}

	if err := os.MkdirAll(absTarget, 0755); err != nil {
		return nil, fmt.Errorf("failed to create target directory %s: %w", absTarget, err)
	}

	report := &ExportReport{
		TargetDirectory: absTarget,
		Timestamp:       time.Now().UTC(),
	}

	// Sort file paths for deterministic export order
	var paths []string
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, relPath := range paths {
		content := files[relPath]
		destPath := filepath.Join(absTarget, filepath.FromSlash(relPath))

		// Check existing file
		var isNew, modified bool
		if existing, err := os.ReadFile(destPath); err == nil {
			if !overwrite {
				// Skip if not overwriting
				continue
			}
			isNew = false
			modified = string(existing) != string(content)
		} else {
			isNew = true
			modified = true
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory for %s: %w", destPath, err)
		}

		// Write file atomically via temp file
		tmpFile := fmt.Sprintf("%s.tmp.%d", destPath, time.Now().UnixNano())
		if err := os.WriteFile(tmpFile, content, 0644); err != nil {
			return nil, fmt.Errorf("failed to write temp file for %s: %w", destPath, err)
		}

		if err := os.Rename(tmpFile, destPath); err != nil {
			_ = os.Remove(tmpFile)
			return nil, fmt.Errorf("failed to materialize file %s: %w", destPath, err)
		}

		sum := sha256.Sum256(content)
		hash := hex.EncodeToString(sum[:])

		fileInfo := ExportedFileInfo{
			RelativePath: relPath,
			AbsolutePath: destPath,
			SizeBytes:    int64(len(content)),
			SHA256:       hash,
			IsNew:        isNew,
			Modified:     modified,
		}

		report.Files = append(report.Files, fileInfo)
		report.FilesWritten++
		report.TotalBytes += int64(len(content))
	}

	report.DurationMs = time.Since(start).Milliseconds()
	return report, nil
}

// ExportWorkspace materializes an entire WorkspaceManifestNode to physical disk.
func (e *Exporter) ExportWorkspace(
	targetDir string,
	manifest *core.WorkspaceManifestNode,
	compMap map[string]*core.ComponentNode,
	symbolMap map[string]*core.ASTSymbolNode,
	overwrite bool,
) (*ExportReport, error) {
	fileMap, err := e.hydrator.HydrateWorkspace(manifest, compMap, symbolMap)
	if err != nil {
		return nil, fmt.Errorf("failed to hydrate workspace for export: %w", err)
	}

	return e.ExportToDisk(targetDir, fileMap, overwrite)
}

// ExportComponent materializes a single ComponentNode to physical disk.
func (e *Exporter) ExportComponent(
	targetDir string,
	comp *core.ComponentNode,
	symbolMap map[string]*core.ASTSymbolNode,
	overwrite bool,
) (*ExportReport, error) {
	fileMap, err := e.hydrator.HydrateComponent(comp, symbolMap)
	if err != nil {
		return nil, fmt.Errorf("failed to hydrate component for export: %w", err)
	}

	return e.ExportToDisk(targetDir, fileMap, overwrite)
}
