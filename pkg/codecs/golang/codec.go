package golang

import (
	"bufio"
	"bytes"
	"go/ast"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// ExtractTrivia extracts top-of-file build tags (//go:build, //go:generate) and compiler pragmas
// into a core.TriviaEnvelope.
func ExtractTrivia(src []byte) *core.TriviaEnvelope {
	if len(src) == 0 {
		return nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(src))
	var directives []string
	var licenseLines []string
	inBlockComment := false
	var blockCommentLines []string

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Stop when reaching the package declaration
		if !inBlockComment && (strings.HasPrefix(trimmed, "package ") || strings.HasPrefix(trimmed, "package\t")) {
			break
		}

		if inBlockComment {
			blockCommentLines = append(blockCommentLines, line)
			if strings.Contains(trimmed, "*/") {
				inBlockComment = false
				blockText := strings.Join(blockCommentLines, "\n")
				if isLicenseText(blockText) {
					licenseLines = append(licenseLines, blockText)
				}
				blockCommentLines = nil
			}
			continue
		}

		if strings.HasPrefix(trimmed, "/*") {
			if strings.Contains(trimmed, "*/") {
				if isLicenseText(trimmed) {
					licenseLines = append(licenseLines, trimmed)
				}
			} else {
				inBlockComment = true
				blockCommentLines = append(blockCommentLines, line)
			}
			continue
		}

		// Directives (//go:build, //go:generate, // +build, //go:embed, compiler pragmas)
		if isGoDirective(trimmed) {
			directives = append(directives, trimmed)
			continue
		}

		// Top-of-file line comments
		if strings.HasPrefix(trimmed, "//") {
			licenseLines = append(licenseLines, trimmed)
			continue
		}
	}

	var licenseHeader string
	if len(licenseLines) > 0 {
		fullLicense := strings.Join(licenseLines, "\n")
		if isLicenseText(fullLicense) {
			licenseHeader = strings.TrimSpace(fullLicense)
		}
	}

	if len(directives) == 0 && licenseHeader == "" {
		return nil
	}

	return &core.TriviaEnvelope{
		HeaderDirectives: directives,
		LicenseHeader:    licenseHeader,
	}
}

// ExtractTriviaFromAST extracts build tags, compiler pragmas, and license headers using the parsed AST.
func ExtractTriviaFromAST(fileNode *ast.File, src []byte) *core.TriviaEnvelope {
	// Fall back to scanning bytes directly for precise line-level fidelity
	return ExtractTrivia(src)
}

// ExtractTriviaFromComponent extracts trivia from source bytes and updates the component node.
func ExtractTriviaFromComponent(comp *core.ComponentNode, src []byte) {
	if comp == nil {
		return
	}
	comp.Trivia = ExtractTrivia(src)
}

func isGoDirective(line string) bool {
	return strings.HasPrefix(line, "//go:build") ||
		strings.HasPrefix(line, "// +build") ||
		strings.HasPrefix(line, "//go:generate") ||
		strings.HasPrefix(line, "//go:embed") ||
		strings.HasPrefix(line, "//go:linkname") ||
		strings.HasPrefix(line, "//go:noinline") ||
		strings.HasPrefix(line, "//go:nosplit") ||
		strings.HasPrefix(line, "//go:nowritebarrier") ||
		strings.HasPrefix(line, "//go:uintptrescapes") ||
		strings.HasPrefix(line, "//go:")
}

func isLicenseText(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "copyright") ||
		strings.Contains(lower, "license") ||
		strings.Contains(lower, "licensed") ||
		strings.Contains(lower, "mozilla public") ||
		strings.Contains(lower, "apache") ||
		strings.Contains(lower, "mit license") ||
		strings.Contains(lower, "spdx-license-identifier") ||
		strings.Contains(lower, "all rights reserved")
}
