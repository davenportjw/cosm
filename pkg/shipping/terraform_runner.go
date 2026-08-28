package shipping

import (
	"fmt"
	"strings"
	"time"

	"github.com/cosmscm/cosm/pkg/codecs/hcl"
	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/materialize"
)

// TerraformCheckReport summarizes terraform fmt and terraform validate hook execution.
type TerraformCheckReport struct {
	Valid          bool            `json:"valid"`
	Formatted      bool            `json:"formatted"`
	CheckedFiles   []string        `json:"checked_files"`
	FmtResult      *hcl.FmtResult  `json:"fmt_result,omitempty"`
	ValidateResult *hcl.ValidateResult `json:"validate_result,omitempty"`
	Diagnostics    []string        `json:"diagnostics,omitempty"`
	DurationMs     int64           `json:"duration_ms"`
}

// TerraformRunner provides automated validation and formatting of Terraform AST subgraphs in ephemeral staging.
type TerraformRunner struct {
	stagingManager *StagingManager
	hydrator       *materialize.Hydrator
}

// NewTerraformRunner creates an initialized TerraformRunner.
func NewTerraformRunner(stagingManager *StagingManager) *TerraformRunner {
	if stagingManager == nil {
		stagingManager = NewStagingManager("")
	}
	return &TerraformRunner{
		stagingManager: stagingManager,
		hydrator:       materialize.NewHydrator(),
	}
}

// CheckFiles executes terraform fmt -check and terraform validate across a map of HCL file contents.
func (r *TerraformRunner) CheckFiles(hclFiles map[string][]byte) (*TerraformCheckReport, error) {
	start := time.Now()

	// Filter HCL files
	filtered := make(map[string][]byte)
	for p, content := range hclFiles {
		if strings.HasSuffix(p, ".tf") || strings.HasSuffix(p, ".tfvars") {
			filtered[p] = content
		}
	}

	if len(filtered) == 0 {
		return &TerraformCheckReport{
			Valid:        true,
			Formatted:    true,
			DurationMs:   time.Since(start).Milliseconds(),
			Diagnostics:  []string{"No Terraform (.tf) files to validate."},
		}, nil
	}

	// Prepare ephemeral staging directory
	target := DefaultTerraformTarget("target:tf-check", nil)
	workspace, err := r.stagingManager.Prepare(target, filtered)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare staging workspace: %w", err)
	}
	defer func() {
		_ = workspace.Cleanup()
	}()

	// 1. Run terraform fmt -check
	fmtRes, err := hcl.RunTerraformFmt(workspace.Directory, true)
	if err != nil {
		return nil, fmt.Errorf("terraform fmt execution failed: %w", err)
	}

	// 2. Run terraform validate
	valRes, err := hcl.RunTerraformValidate(workspace.Directory)
	if err != nil {
		return nil, fmt.Errorf("terraform validate execution failed: %w", err)
	}

	var checkedFiles []string
	for p := range filtered {
		checkedFiles = append(checkedFiles, p)
	}

	var diags []string
	if fmtRes.Modified {
		diags = append(diags, "terraform fmt: files require formatting")
		diags = append(diags, fmtRes.CheckedFiles...)
	}
	if !valRes.Valid {
		diags = append(diags, valRes.Diagnostics...)
	}

	isFormatted := !fmtRes.Modified
	isValid := valRes.Valid

	return &TerraformCheckReport{
		Valid:          isValid,
		Formatted:      isFormatted,
		CheckedFiles:   checkedFiles,
		FmtResult:      fmtRes,
		ValidateResult: valRes,
		Diagnostics:    diags,
		DurationMs:     time.Since(start).Milliseconds(),
	}, nil
}

// FormatAndFix auto-formats Terraform files in the given file map and returns the formatted map.
func (r *TerraformRunner) FormatAndFix(hclFiles map[string][]byte) (map[string][]byte, error) {
	formattedFiles := make(map[string][]byte)

	for p, content := range hclFiles {
		if strings.HasSuffix(p, ".tf") || strings.HasSuffix(p, ".tfvars") {
			formatted, err := hcl.FormatHCL(content)
			if err != nil {
				return nil, fmt.Errorf("failed to format %s: %w", p, err)
			}
			formattedFiles[p] = formatted
		} else {
			formattedFiles[p] = content
		}
	}

	return formattedFiles, nil
}

// CheckWorkspaceHCL extracts all HCL components from a WorkspaceManifestNode and validates them.
func (r *TerraformRunner) CheckWorkspaceHCL(
	manifest *core.WorkspaceManifestNode,
	compMap map[string]*core.ComponentNode,
	symbolMap map[string]*core.ASTSymbolNode,
) (*TerraformCheckReport, error) {
	hclFiles := make(map[string][]byte)

	for _, compID := range manifest.Components {
		comp, ok := compMap[compID]
		if !ok || comp.Language != core.LangHCL {
			continue
		}
		files, err := r.hydrator.HydrateComponent(comp, symbolMap)
		if err != nil {
			return nil, fmt.Errorf("failed to hydrate HCL component %s: %w", comp.Name, err)
		}
		for p, data := range files {
			hclFiles[p] = data
		}
	}

	return r.CheckFiles(hclFiles)
}
