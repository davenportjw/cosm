package ignore

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// SecretFinding reports a detected secret in source content.
type SecretFinding struct {
	LineNumber  int    `json:"line_number"`
	Type        string `json:"type"`
	Snippet     string `json:"snippet"`
	Description string `json:"description"`
}

// SecretDetector inspects file contents for credentials and plaintext secrets.
type SecretDetector struct {
	patterns []compiledSecretPattern
}

type compiledSecretPattern struct {
	name        string
	regex       *regexp.Regexp
	description string
}

// NewSecretDetector initializes a detector with industry-standard token and key signatures.
func NewSecretDetector() *SecretDetector {
	return &SecretDetector{
		patterns: []compiledSecretPattern{
			{
				name:        "Google AI / GCP API Key",
				regex:       regexp.MustCompile(`AIza[0-9A-Za-z-_]{35}`),
				description: "Detected potential Google AI / Cloud API Key",
			},
			{
				name:        "AWS Access Key ID",
				regex:       regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
				description: "Detected AWS Access Key ID",
			},
			{
				name:        "GitHub Personal Access Token",
				regex:       regexp.MustCompile(`gh[pousr]_[0-9a-zA-Z]{36}`),
				description: "Detected GitHub Personal Access Token",
			},
			{
				name:        "Slack Token",
				regex:       regexp.MustCompile(`xox[baprs]-[0-9a-zA-Z]{10,48}`),
				description: "Detected Slack API Token",
			},
			{
				name:        "Private Key Block",
				regex:       regexp.MustCompile(`-----BEGIN (?:[A-Z ]+ )?PRIVATE KEY-----`),
				description: "Detected cryptographic private key block",
			},
			{
				name:        "Hardcoded Credential Assignment",
				regex:       regexp.MustCompile(`(?i)(?:api_key|apikey|secret_key|auth_token|client_secret|password)\s*[:=]\s*["']([^"']{8,})["']`),
				description: "Detected hardcoded credential assignment",
			},
		},
	}
}

// DetectSecrets scans content for secrets, respecting inline suppression comments.
func (d *SecretDetector) DetectSecrets(path string, content []byte) []SecretFinding {
	var findings []SecretFinding

	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Inline suppression check
		if strings.Contains(line, "cosm:allow-secret") {
			continue
		}

		matchedSpecific := false
		for _, p := range d.patterns {
			if p.name == "Hardcoded Credential Assignment" && matchedSpecific {
				continue
			}

			matches := p.regex.FindAllStringSubmatch(line, -1)
			if len(matches) == 0 {
				continue
			}

			for _, m := range matches {
				token := m[0]
				// For assignment pattern, check the captured value
				if len(m) > 1 {
					tokenVal := m[1]
					// Filter out template variables, mocks, dummy values
					if isBenignPlaceholder(tokenVal) {
						continue
					}
				}

				if p.name != "Hardcoded Credential Assignment" {
					matchedSpecific = true
				}

				findings = append(findings, SecretFinding{
					LineNumber:  lineNum,
					Type:        p.name,
					Snippet:     maskSecret(token),
					Description: p.description,
				})
			}
		}
	}

	return findings
}

func isBenignPlaceholder(val string) bool {
	lower := strings.ToLower(val)
	if strings.HasPrefix(val, "${") || strings.HasPrefix(val, "var.") ||
		strings.HasPrefix(val, "{{") || strings.HasPrefix(val, "%{") {
		return true
	}
	if strings.Contains(lower, "dummy") || strings.Contains(lower, "example") ||
		strings.Contains(lower, "test-token") || strings.Contains(lower, "secret-token") ||
		strings.Contains(lower, "default-dev-secret") || strings.Contains(lower, "placeholder") ||
		strings.Contains(lower, "changeme") || strings.Contains(lower, "your-api-key") {
		return true
	}
	return false
}

func maskSecret(token string) string {
	if len(token) <= 8 {
		return strings.Repeat("*", len(token))
	}
	prefix := token[:4]
	suffix := token[len(token)-3:]
	return fmt.Sprintf("%s...%s", prefix, suffix)
}
