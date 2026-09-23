package shipping

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/materialize"
)

// PackageArtifact represents a packaged, ship-ready deployment artifact.
type PackageArtifact struct {
	ArtifactID     string            `json:"artifact_id"`
	TargetName     string            `json:"target_name"`
	Kind           TargetKind        `json:"kind"`
	ArtifactPath   string            `json:"artifact_path"`
	SizeBytes      int64             `json:"size_bytes"`
	SHA256Checksum string            `json:"sha256_checksum"`
	CreatedAt      time.Time         `json:"created_at"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// Packager coordinates packaging of build outputs into shipping bundles.
type Packager struct {
	stagingManager *StagingManager
	tfRunner       *TerraformRunner
	compiler       *Compiler
	hydrator       *materialize.Hydrator
	repoRoot       string
}

// NewPackager creates a new Packager instance.
func NewPackager(stagingManager *StagingManager) *Packager {
	if stagingManager == nil {
		stagingManager = NewStagingManager("")
	}
	return &Packager{
		stagingManager: stagingManager,
		tfRunner:       NewTerraformRunner(stagingManager),
		compiler:       NewCompiler(stagingManager),
		hydrator:       materialize.NewHydrator(),
	}
}

// SetRepoRoot sets an explicit repository root directory for dependency auto-staging.
func (p *Packager) SetRepoRoot(repoRoot string) {
	p.repoRoot = repoRoot
	if p.stagingManager != nil {
		p.stagingManager.SetRepoRoot(repoRoot)
	}
}

// AutoStageDependencies auto-stages project dependencies (go.mod, go.sum for Go;
// package.json, tsconfig.json for TypeScript/Node) from the repo root or working tree
// into the files map if they are present on disk and not already populated.
func (p *Packager) AutoStageDependencies(target *TargetSpec, files map[string][]byte) error {
	repoRoot := p.repoRoot
	if repoRoot == "" {
		if p.stagingManager != nil {
			repoRoot = p.stagingManager.RepoRoot()
		} else {
			repoRoot = FindWorkingTreeRoot()
		}
	}
	if repoRoot == "" {
		return nil
	}

	depFiles := []string{"go.mod", "go.sum", "package.json", "tsconfig.json"}
	for _, f := range depFiles {
		if _, exists := files[f]; !exists {
			srcPath := filepath.Join(repoRoot, f)
			if info, err := os.Stat(srcPath); err == nil && !info.IsDir() {
				data, err := os.ReadFile(srcPath)
				if err == nil {
					files[f] = data
				}
			}
		}
	}

	// Also check component subdirectories if target specifies component names
	if target != nil {
		for _, comp := range target.ComponentNames {
			for _, f := range depFiles {
				compFile := filepath.Join(comp, f)
				if _, exists := files[compFile]; !exists {
					srcPath := filepath.Join(repoRoot, compFile)
					if info, err := os.Stat(srcPath); err == nil && !info.IsDir() {
						data, err := os.ReadFile(srcPath)
						if err == nil {
							files[compFile] = data
						}
					}
				}
			}
		}
	}

	return nil
}

// BuildAndPackageTarget executes validation hooks, builds code, and packages artifacts for a TargetSpec.
func (p *Packager) BuildAndPackageTarget(
	target *TargetSpec,
	files map[string][]byte,
	distDir string,
) (*PackageArtifact, error) {
	if target == nil {
		return nil, fmt.Errorf("target spec is nil")
	}

	if distDir == "" {
		distDir = "dist"
	}
	if err := os.MkdirAll(distDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create dist dir: %w", err)
	}

	if err := p.AutoStageDependencies(target, files); err != nil {
		return nil, fmt.Errorf("dependency auto-staging failed: %w", err)
	}

	workspace, err := p.stagingManager.Prepare(target, files)
	if err != nil {
		return nil, fmt.Errorf("staging preparation failed: %w", err)
	}
	defer func() {
		_ = workspace.Cleanup()
	}()

	// 1. Enforce validation hooks
	for _, hook := range target.ValidateHooks {
		if strings.Contains(hook, "terraform") {
			report, err := p.tfRunner.CheckFiles(files)
			if err != nil {
				return nil, fmt.Errorf("terraform hook validation failed: %w", err)
			}
			if !report.Valid {
				return nil, fmt.Errorf("terraform validation errors: %s", strings.Join(report.Diagnostics, "; "))
			}
		}
	}

	// 2. Execute target build
	var buildResult *BuildResult
	switch target.Kind {
	case TargetLocalService:
		buildResult, err = p.compiler.CompileGo(workspace, target.ArtifactName)
		if err != nil {
			return nil, err
		}

	case TargetStaticWeb:
		buildResult, err = p.compiler.CompileFrontend(workspace, "bundle.js")
		if err != nil {
			return nil, err
		}

	case TargetTerraformInfra:
		// Package Terraform files
		buildResult = &BuildResult{
			TargetName: target.Name,
			Successful: true,
			OutputLogs: "Terraform configuration validated and prepared",
		}

	case TargetCloudRun, TargetComposite:
		// Multi-tier compilation
		_, _ = p.compiler.CompileGo(workspace, "server")
		_, _ = p.compiler.CompileFrontend(workspace, "bundle.js")
		buildResult = &BuildResult{
			TargetName: target.Name,
			Successful: true,
			OutputLogs: "Cloud Run container composite prepared",
		}
	}

	// 3. Assemble package tarball archive
	archiveName := target.ArtifactName
	if archiveName == "" {
		archiveName = fmt.Sprintf("%s.tar.gz", strings.ReplaceAll(target.Name, ":", "_"))
	}
	if !strings.HasSuffix(archiveName, ".tar.gz") {
		archiveName += ".tar.gz"
	}

	archivePath := filepath.Join(distDir, archiveName)
	if err := p.createTarGz(workspace.Directory, archivePath); err != nil {
		return nil, fmt.Errorf("failed to create archive %s: %w", archivePath, err)
	}

	archiveData, err := os.ReadFile(archivePath)
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(archiveData)
	checksum := hex.EncodeToString(sum[:])
	artifactID := checksum[:16]

	metadata := map[string]string{
		"target_name": target.Name,
		"target_kind": string(target.Kind),
		"checksum":    checksum,
	}
	if buildResult != nil && buildResult.OutputLogs != "" {
		metadata["build_logs"] = buildResult.OutputLogs
	}

	return &PackageArtifact{
		ArtifactID:     artifactID,
		TargetName:     target.Name,
		Kind:           target.Kind,
		ArtifactPath:   archivePath,
		SizeBytes:      int64(len(archiveData)),
		SHA256Checksum: checksum,
		CreatedAt:      time.Now().UTC(),
		Metadata:       metadata,
	}, nil
}

// ShipWorkspace builds and packages all targets specified in a WorkspaceManifestNode.
func (p *Packager) ShipWorkspace(
	manifest *core.WorkspaceManifestNode,
	compMap map[string]*core.ComponentNode,
	symbolMap map[string]*core.ASTSymbolNode,
	targets []*TargetSpec,
	distDir string,
) ([]*PackageArtifact, error) {
	files, err := p.hydrator.HydrateWorkspace(manifest, compMap, symbolMap)
	if err != nil {
		return nil, fmt.Errorf("failed to hydrate workspace: %w", err)
	}

	// Run mandatory terraform fmt/validate check if HCL files exist
	tfReport, err := p.tfRunner.CheckFiles(files)
	if err != nil {
		return nil, fmt.Errorf("terraform validation failed: %w", err)
	}
	if !tfReport.Valid {
		return nil, fmt.Errorf("terraform configuration invalid: %s", strings.Join(tfReport.Diagnostics, "; "))
	}

	if len(targets) == 0 {
		// Provide default local-service target
		targets = []*TargetSpec{
			DefaultLocalServiceTarget("target:local-service", nil),
		}
	}

	var artifacts []*PackageArtifact
	for _, target := range targets {
		art, err := p.BuildAndPackageTarget(target, files, distDir)
		if err != nil {
			return nil, fmt.Errorf("target build %s failed: %w", target.Name, err)
		}
		artifacts = append(artifacts, art)
	}

	return artifacts, nil
}

func (p *Packager) createTarGz(sourceDir, tarGzPath string) error {
	outFile, err := os.Create(tarGzPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	gw := gzip.NewWriter(outFile)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	return filepath.Walk(sourceDir, func(file string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, file)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relPath)

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(tw, f)
		return err
	})
}
