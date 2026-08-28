package rater

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cosmscm/cosm/test/agents/framework"
)

// Reporter formats and exports evaluation results and scorecard summaries.
type Reporter struct{}

// NewReporter creates a new Reporter instance.
func NewReporter() *Reporter {
	return &Reporter{}
}

// GenerateMarkdownScorecard creates a comprehensive GitHub-flavored Markdown scorecard.
func (r *Reporter) GenerateMarkdownScorecard(sc *Scorecard, audit *OracleAuditReport, session *framework.AgentSession) string {
	if sc == nil {
		return "# Cosm Evaluation Scorecard\n\nNo evaluation data available.\n"
	}

	var sb strings.Builder

	// Header
	sb.WriteString("# 🛡️ Cosm (cosm) Agent Evaluation Scorecard\n\n")

	// Grade Badge & Overview
	gradeBadge := ""
	switch sc.LetterGrade {
	case GradeAPlus:
		gradeBadge = "🟢 **Grade: A+ (Outstanding)**"
	case GradeA:
		gradeBadge = "🟢 **Grade: A (Excellent)**"
	case GradeB:
		gradeBadge = "🟡 **Grade: B (Good)**"
	case GradeC:
		gradeBadge = "🟠 **Grade: C (Needs Improvement)**"
	default:
		gradeBadge = "🔴 **Grade: F (Critical Failures)**"
	}

	sb.WriteString(fmt.Sprintf("### %s\n\n", gradeBadge))
	sb.WriteString(fmt.Sprintf("- **Composite Score:** `%.1f / 100.0`\n", sc.OverallScore))
	if sc.ScenarioName != "" {
		sb.WriteString(fmt.Sprintf("- **Scenario:** `%s`\n", sc.ScenarioName))
	}
	if session != nil {
		sb.WriteString(fmt.Sprintf("- **Agent ID:** `%s` | **Model:** `%s`\n", session.AgentID, session.Model))
		sb.WriteString(fmt.Sprintf("- **Execution Steps:** `%d` | **Tokens Used:** `%d` | **Duration:** `%s`\n",
			session.TotalSteps, session.TotalTokens.TotalTokens, session.Duration.Round(time.Millisecond)))
	}
	sb.WriteString(fmt.Sprintf("- **Evaluated At:** `%s`\n\n", sc.Timestamp.Format(time.RFC3339)))

	// Dimension Breakdown Table
	sb.WriteString("## 📊 Dimension Breakdown\n\n")
	sb.WriteString("| Dimension | Weight | Score | Status | Findings | Details |\n")
	sb.WriteString("| :--- | :---: | :---: | :---: | :---: | :--- |\n")

	orderedDims := []string{
		DimASTSyntax,
		DimContract,
		DimCompilation,
		DimStorage,
		DimSecurity,
		DimArchitecture,
	}

	for _, dimKey := range orderedDims {
		dim, exists := sc.Dimensions[dimKey]
		if !exists {
			continue
		}
		statusIcon := "✅ PASS"
		if !dim.Passed {
			statusIcon = "❌ FAIL"
		}
		sb.WriteString(fmt.Sprintf("| **%s** | %.0f%% | `%.1f` | %s | %d | %s |\n",
			dim.Name, dim.Weight*100, dim.Score, statusIcon, dim.FindingsCount, dim.Summary))
	}
	sb.WriteString("\n")

	// Cross-Boundary Contracts Table
	if audit != nil && len(audit.ContractEntries) > 0 {
		sb.WriteString("## 🔗 Cross-Boundary Contract Verification\n\n")
		sb.WriteString("| Source | Target | Type | Contract ID | Status | Details |\n")
		sb.WriteString("| :--- | :--- | :---: | :--- | :---: | :--- |\n")
		for _, c := range audit.ContractEntries {
			statusBadge := "✅ VALID"
			if c.Status == "BROKEN" {
				statusBadge = "❌ BROKEN"
			} else if c.Status == "DRIFT" {
				statusBadge = "⚠️ DRIFT"
			}
			sb.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` | `%s` | %s | %s |\n",
				c.SourceComponent, c.TargetComponent, c.ContractType, c.Identifier, statusBadge, c.Details))
		}
		sb.WriteString("\n")
	}

	// Storage & Merkle Tree Integrity
	if audit != nil && audit.StorageReport.TotalObjectsChecked > 0 {
		sb.WriteString("## 🗄️ Storage Merkle-DAG & Cryptographic Integrity\n\n")
		sb.WriteString(fmt.Sprintf("- **Total Objects Checked:** `%d` (Valid: `%d`, Corrupted: `%d`)\n",
			audit.StorageReport.TotalObjectsChecked, audit.StorageReport.ValidObjectsCount, audit.StorageReport.CorruptedObjectsCount))
		sb.WriteString(fmt.Sprintf("- **Bit-Rot Status:** `%v`\n", audit.StorageReport.BitRotDetected))
		sb.WriteString(fmt.Sprintf("- **Manifest Merkle Root:** `%s`\n", audit.StorageReport.ManifestMerkleRoot))
		sb.WriteString(fmt.Sprintf("- **Recalculated Root:** `%s` (Match: `%v`)\n\n",
			audit.StorageReport.RecalculatedRoot, audit.StorageReport.MerkleRootMatch))
	}

	// Audited Findings & Issues
	if len(sc.Findings) > 0 {
		sb.WriteString("## ⚠️ Audited Findings & Remediation\n\n")
		for i, f := range sc.Findings {
			sevIcon := "ℹ️"
			switch f.Severity {
			case SeverityCritical:
				sevIcon = "🛑 [CRITICAL]"
			case SeverityHigh:
				sevIcon = "🔴 [HIGH]"
			case SeverityMedium:
				sevIcon = "🟡 [MEDIUM]"
			case SeverityLow:
				sevIcon = "🔵 [LOW]"
			}
			sb.WriteString(fmt.Sprintf("### %d. %s %s\n\n", i+1, sevIcon, f.Title))
			sb.WriteString(fmt.Sprintf("- **Dimension:** `%s`\n", f.Dimension))
			if f.FileLocation != "" {
				sb.WriteString(fmt.Sprintf("- **Location:** `%s`\n", f.FileLocation))
			}
			sb.WriteString(fmt.Sprintf("- **Description:** %s\n", f.Description))
			if f.SuggestedFix != "" {
				sb.WriteString(fmt.Sprintf("- **Suggested Remediation:** %s\n", f.SuggestedFix))
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("## ✨ Zero Defect Verification\n\nAll AST syntax, cross-boundary contracts, and Merkle digests verified with 100% integrity.\n\n")
	}

	return sb.String()
}

// GenerateRemediationReport produces an actionable step-by-step checklist for resolving issues.
func (r *Reporter) GenerateRemediationReport(sc *Scorecard) string {
	if sc == nil || len(sc.Findings) == 0 {
		return "## ✅ No Remediation Required\n\nWorkspace is fully compliant with all architectural and security requirements.\n"
	}

	var sb strings.Builder
	sb.WriteString("# 🛠️ Autonomous Remediation Action Plan\n\n")
	sb.WriteString(fmt.Sprintf("Found **%d issues** (Critical: %d, High: %d) requiring attention:\n\n",
		sc.TotalFindings, sc.CriticalFindings, sc.HighFindings))

	for i, f := range sc.Findings {
		sb.WriteString(fmt.Sprintf("### Step %d: %s\n", i+1, f.Title))
		sb.WriteString(fmt.Sprintf("- **Target File:** `%s`\n", f.FileLocation))
		sb.WriteString(fmt.Sprintf("- **Action:** %s\n", f.SuggestedFix))
		sb.WriteString(fmt.Sprintf("- **Problem:** %s\n\n", f.Description))
	}

	return sb.String()
}

// ExportJSON marshals the scorecard to JSON.
func (r *Reporter) ExportJSON(sc *Scorecard) ([]byte, error) {
	return json.MarshalIndent(sc, "", "  ")
}
