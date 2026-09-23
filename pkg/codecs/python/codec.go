package python

import (
	"regexp"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

var (
	encodingPragmaRegex = regexp.MustCompile(`^[ \t]*#.*coding[:=][ \t]*([-\w.]+)`)
)

// ExtractTrivia extracts encoding pragmas (# -*- coding: utf-8 -*-), shebangs, license headers,
// and module docstrings from Python source code into a core.TriviaEnvelope.
func ExtractTrivia(src []byte) *core.TriviaEnvelope {
	if len(src) == 0 {
		return nil
	}

	lines := strings.Split(string(src), "\n")
	var directives []string
	var licenseLines []string
	var docstringLines []string
	inDocstring := false
	docstringQuote := ""
	docstringFound := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if inDocstring {
			docstringLines = append(docstringLines, line)
			if strings.Contains(trimmed, docstringQuote) {
				inDocstring = false
				docstringFound = true
				break
			}
			continue
		}

		if trimmed == "" {
			continue
		}

		// Shebang line
		if strings.HasPrefix(trimmed, "#!") {
			directives = append(directives, trimmed)
			continue
		}

		// Encoding pragma line e.g. # -*- coding: utf-8 -*- or # coding=utf-8
		if encodingPragmaRegex.MatchString(trimmed) || strings.Contains(trimmed, "-*- coding") {
			directives = append(directives, trimmed)
			continue
		}

		// Comment line before docstring or code
		if strings.HasPrefix(trimmed, "#") {
			if isPythonLicenseText(trimmed) {
				licenseLines = append(licenseLines, trimmed)
			}
			continue
		}

		// Module docstring (starts with """ or ''')
		if !docstringFound {
			if strings.HasPrefix(trimmed, `"""`) {
				docstringQuote = `"""`
				afterQuote := strings.TrimPrefix(trimmed, `"""`)
				if strings.Contains(afterQuote, `"""`) {
					docstringLines = append(docstringLines, trimmed)
					docstringFound = true
					break
				} else {
					inDocstring = true
					docstringLines = append(docstringLines, line)
					continue
				}
			} else if strings.HasPrefix(trimmed, `'''`) {
				docstringQuote = `'''`
				afterQuote := strings.TrimPrefix(trimmed, `'''`)
				if strings.Contains(afterQuote, `'''`) {
					docstringLines = append(docstringLines, trimmed)
					docstringFound = true
					break
				} else {
					inDocstring = true
					docstringLines = append(docstringLines, line)
					continue
				}
			}
		}

		// First non-comment, non-docstring code token (def, class, import, etc.)
		break
	}

	var moduleDoc string
	if len(docstringLines) > 0 {
		moduleDoc = strings.TrimSpace(strings.Join(docstringLines, "\n"))
	}

	var licenseHeader string
	if len(licenseLines) > 0 {
		licenseHeader = strings.TrimSpace(strings.Join(licenseLines, "\n"))
	} else if moduleDoc != "" && isPythonLicenseText(moduleDoc) {
		licenseHeader = moduleDoc
	}

	if len(directives) == 0 && licenseHeader == "" && moduleDoc == "" {
		return nil
	}

	return &core.TriviaEnvelope{
		HeaderDirectives: directives,
		LicenseHeader:    licenseHeader,
		ModuleDocstring:  moduleDoc,
	}
}

// ExtractTriviaFromComponent extracts trivia from source bytes and updates the component node.
func ExtractTriviaFromComponent(comp *core.ComponentNode, src []byte) {
	if comp == nil {
		return
	}
	comp.Trivia = ExtractTrivia(src)
}

func isPythonLicenseText(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "copyright") ||
		strings.Contains(lower, "license") ||
		strings.Contains(lower, "licensed") ||
		strings.Contains(lower, "apache") ||
		strings.Contains(lower, "mit license") ||
		strings.Contains(lower, "spdx-license-identifier") ||
		strings.Contains(lower, "all rights reserved")
}
