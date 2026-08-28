package hcl

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FmtResult represents the outcome of a terraform fmt invocation.
type FmtResult struct {
	Success      bool     `json:"success"`
	Modified     bool     `json:"modified"`
	CheckedFiles []string `json:"checked_files"`
	Output       string   `json:"output"`
	UsedBinary   bool     `json:"used_binary"`
}

// ValidateResult represents the outcome of a terraform validate invocation.
type ValidateResult struct {
	Valid        bool     `json:"valid"`
	ErrorCount   int      `json:"error_count"`
	WarningCount int      `json:"warning_count"`
	Diagnostics  []string `json:"diagnostics"`
	Output       string   `json:"output"`
	UsedBinary   bool     `json:"used_binary"`
}

// FormatHCL performs in-process, pure-Go formatting of HCL source bytes.
// It normalizes indentation (2 spaces per nesting level), aligns '=' within attribute blocks,
// and removes trailing whitespace.
func FormatHCL(src []byte) ([]byte, error) {
	lines := strings.Split(string(src), "\n")
	var formattedLines []string

	indentLevel := 0
	type pendingAttr struct {
		key     string
		val     string
		comment string
		lineNum int
	}
	var attrGroup []pendingAttr

	flushAttrGroup := func() {
		if len(attrGroup) == 0 {
			return
		}
		maxKeyLen := 0
		for _, a := range attrGroup {
			if len(a.key) > maxKeyLen {
				maxKeyLen = len(a.key)
			}
		}

		indent := strings.Repeat("  ", indentLevel)
		for _, a := range attrGroup {
			pad := strings.Repeat(" ", maxKeyLen-len(a.key))
			line := fmt.Sprintf("%s%s%s = %s", indent, a.key, pad, a.val)
			if a.comment != "" {
				line += " " + a.comment
			}
			formattedLines = append(formattedLines, line)
		}
		attrGroup = nil
	}

	for _, rawLine := range lines {
		trimmed := strings.TrimSpace(rawLine)

		// Handle empty lines
		if trimmed == "" {
			flushAttrGroup()
			formattedLines = append(formattedLines, "")
			continue
		}

		// Handle comments
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			flushAttrGroup()
			indent := strings.Repeat("  ", indentLevel)
			formattedLines = append(formattedLines, indent+trimmed)
			continue
		}

		// Check closing brace
		if strings.HasPrefix(trimmed, "}") {
			flushAttrGroup()
			if indentLevel > 0 {
				indentLevel--
			}
			indent := strings.Repeat("  ", indentLevel)
			formattedLines = append(formattedLines, indent+trimmed)
			continue
		}

		// Check opening block: e.g. resource "..." "..." {
		if strings.HasSuffix(trimmed, "{") {
			flushAttrGroup()
			indent := strings.Repeat("  ", indentLevel)
			formattedLines = append(formattedLines, indent+trimmed)
			indentLevel++
			continue
		}

		// Check attribute assignment: key = value
		if eqIdx := strings.Index(trimmed, "="); eqIdx != -1 && !strings.HasPrefix(trimmed, "{") {
			key := strings.TrimSpace(trimmed[:eqIdx])
			valAndComment := strings.TrimSpace(trimmed[eqIdx+1:])

			comment := ""
			val := valAndComment
			if cIdx := strings.Index(valAndComment, " #"); cIdx != -1 {
				val = strings.TrimSpace(valAndComment[:cIdx])
				comment = valAndComment[cIdx+1:]
			} else if cIdx := strings.Index(valAndComment, " //"); cIdx != -1 {
				val = strings.TrimSpace(valAndComment[:cIdx])
				comment = valAndComment[cIdx+1:]
			}

			attrGroup = append(attrGroup, pendingAttr{
				key:     key,
				val:     val,
				comment: comment,
			})
			continue
		}

		// Any other line (e.g. array element or heredoc line)
		flushAttrGroup()
		indent := strings.Repeat("  ", indentLevel)
		formattedLines = append(formattedLines, indent+trimmed)
	}

	flushAttrGroup()

	// Trim trailing blank lines to single newline
	result := strings.Join(formattedLines, "\n")
	result = strings.TrimRight(result, "\n") + "\n"

	return []byte(result), nil
}

// ValidateHCL performs pure-Go structural validation of HCL syntax, block and brace balancing.
func ValidateHCL(src []byte) error {
	braceCount := 0
	inString := false
	var stringDelim rune

	lines := strings.Split(string(src), "\n")
	for lineIdx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}

		var prev rune
		for _, ch := range line {
			if inString {
				if ch == stringDelim && prev != '\\' {
					inString = false
				}
			} else {
				if ch == '"' || ch == '\'' {
					inString = true
					stringDelim = ch
				} else if ch == '{' {
					braceCount++
				} else if ch == '}' {
					braceCount--
					if braceCount < 0 {
						return fmt.Errorf("line %d: unexpected closing brace '}'", lineIdx+1)
					}
				}
			}
			prev = ch
		}
	}

	if inString {
		return fmt.Errorf("unclosed string literal at end of file")
	}
	if braceCount != 0 {
		return fmt.Errorf("unbalanced braces: missing %d closing brace(s)", braceCount)
	}

	return nil
}

// RunTerraformFmt formats Terraform files using the 'terraform' CLI if installed,
// or falls back cleanly to the pure-Go in-process formatter.
func RunTerraformFmt(targetPath string, checkOnly bool) (*FmtResult, error) {
	_, err := exec.LookPath("terraform")
	hasTerraform := (err == nil)

	if hasTerraform {
		args := []string{"fmt"}
		if checkOnly {
			args = append(args, "-check")
		}
		args = append(args, targetPath)

		cmd := exec.Command("terraform", args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		execErr := cmd.Run()
		output := stdout.String()
		if stderr.Len() > 0 {
			if output != "" {
				output += "\n"
			}
			output += stderr.String()
		}

		modified := false
		if checkOnly {
			modified = (execErr != nil)
		} else {
			modified = (strings.TrimSpace(output) != "")
		}

		var files []string
		for _, f := range strings.Split(output, "\n") {
			f = strings.TrimSpace(f)
			if f != "" {
				files = append(files, f)
			}
		}

		return &FmtResult{
			Success:      execErr == nil || (checkOnly && modified),
			Modified:     modified,
			CheckedFiles: files,
			Output:       output,
			UsedBinary:   true,
		}, nil
	}

	// Pure-Go in-process fallback
	stat, err := os.Stat(targetPath)
	if err != nil {
		return nil, fmt.Errorf("target path not found: %w", err)
	}

	var filesToProcess []string
	if stat.IsDir() {
		entries, err := os.ReadDir(targetPath)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() && (strings.HasSuffix(e.Name(), ".tf") || strings.HasSuffix(e.Name(), ".tfvars")) {
				filesToProcess = append(filesToProcess, filepath.Join(targetPath, e.Name()))
			}
		}
	} else {
		filesToProcess = append(filesToProcess, targetPath)
	}

	modified := false
	var modifiedFiles []string

	for _, f := range filesToProcess {
		content, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		formatted, err := FormatHCL(content)
		if err != nil {
			return nil, fmt.Errorf("failed to format %s: %w", f, err)
		}

		if !bytes.Equal(content, formatted) {
			modified = true
			modifiedFiles = append(modifiedFiles, f)
			if !checkOnly {
				if err := os.WriteFile(f, formatted, 0644); err != nil {
					return nil, fmt.Errorf("failed to write formatted file %s: %w", f, err)
				}
			}
		}
	}

	return &FmtResult{
		Success:      true,
		Modified:     modified,
		CheckedFiles: modifiedFiles,
		Output:       strings.Join(modifiedFiles, "\n"),
		UsedBinary:   false,
	}, nil
}

// RunTerraformValidate validates Terraform configuration using 'terraform validate' if installed and initialized,
// or falls back to pure-Go AST and structural validation.
func RunTerraformValidate(dirPath string) (*ValidateResult, error) {
	_, err := exec.LookPath("terraform")
	hasTerraform := (err == nil)

	if hasTerraform {
		cmd := exec.Command("terraform", "validate", "-json")
		cmd.Dir = dirPath
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		execErr := cmd.Run()
		output := stdout.String()
		if output == "" && stderr.Len() > 0 {
			output = stderr.String()
		}

		// Check if error is due to uninitialized provider in isolated temp test dirs
		if execErr != nil && strings.Contains(output, "Missing required provider") {
			// Fall back to pure-Go syntax validator for uninitialized directory
			return validatePureGo(dirPath)
		}

		valid := (execErr == nil)
		return &ValidateResult{
			Valid:       valid,
			Output:      output,
			Diagnostics: []string{output},
			UsedBinary:  true,
		}, nil
	}

	return validatePureGo(dirPath)
}

func validatePureGo(dirPath string) (*ValidateResult, error) {
	// Pure-Go fallback validator
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read dir %s: %w", dirPath, err)
	}

	var diags []string
	errorCount := 0

	parser := NewHCLParser()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tf") {
			continue
		}
		fPath := filepath.Join(dirPath, e.Name())
		data, err := os.ReadFile(fPath)
		if err != nil {
			diags = append(diags, fmt.Sprintf("%s: read error: %v", e.Name(), err))
			errorCount++
			continue
		}

		if err := ValidateHCL(data); err != nil {
			diags = append(diags, fmt.Sprintf("%s: syntax error: %v", e.Name(), err))
			errorCount++
			continue
		}

		doc, err := parser.ParseSource(e.Name(), data)
		if err != nil {
			diags = append(diags, fmt.Sprintf("%s: AST parse error: %v", e.Name(), err))
			errorCount++
			continue
		}

		if len(doc.Blocks) == 0 && len(strings.TrimSpace(string(data))) > 0 {
			diags = append(diags, fmt.Sprintf("%s: warning: no Terraform blocks recognized", e.Name()))
		}
	}

	valid := (errorCount == 0)
	return &ValidateResult{
		Valid:        valid,
		ErrorCount:   errorCount,
		WarningCount: 0,
		Diagnostics:  diags,
		Output:       strings.Join(diags, "\n"),
		UsedBinary:   false,
	}, nil
}
