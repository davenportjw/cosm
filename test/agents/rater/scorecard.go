package rater

import (
	"encoding/json"
	"fmt"
	"time"
)

// DimensionName constants for scorecard evaluation.
const (
	DimASTSyntax    = "AST_SYNTACTIC_CORRECTNESS"
	DimContract     = "CONTRACT_INTEGRITY"
	DimCompilation  = "COMPILATION_VALIDITY"
	DimStorage      = "STORAGE_INTEGRITY"
	DimSecurity     = "SECURITY_IAM"
	DimArchitecture = "ARCHITECTURE_EFFICIENCY"
)

// DimensionScore contains the calculated rating for one specific audit vector.
type DimensionScore struct {
	Name          string  `json:"name"`
	Weight        float64 `json:"weight"`
	Score         float64 `json:"score"`          // 0.0 to 100.0
	WeightedScore float64 `json:"weighted_score"` // Score * Weight
	Passed        bool    `json:"passed"`
	FindingsCount int     `json:"findings_count"`
	Summary       string  `json:"summary"`
}

// LetterGrade represents academic rating tiers.
type LetterGrade string

const (
	GradeAPlus LetterGrade = "A+"
	GradeA     LetterGrade = "A"
	GradeB     LetterGrade = "B"
	GradeC     LetterGrade = "C"
	GradeF     LetterGrade = "F"
)

// Scorecard aggregates deep verification oracle results and LLM critic findings into a weighted score.
type Scorecard struct {
	SessionID        string                    `json:"session_id,omitempty"`
	ScenarioName     string                    `json:"scenario_name,omitempty"`
	OverallScore     float64                   `json:"overall_score"` // 0.0 to 100.0
	LetterGrade      LetterGrade               `json:"letter_grade"`  // A+, A, B, C, F
	Dimensions       map[string]DimensionScore `json:"dimensions"`
	CriticalFindings int                       `json:"critical_findings"`
	HighFindings     int                       `json:"high_findings"`
	TotalFindings    int                       `json:"total_findings"`
	Findings         []OracleFinding           `json:"findings"`
	Timestamp        time.Time                 `json:"timestamp"`
	Summary          string                    `json:"summary"`
}

// ComputeLetterGrade translates numerical composite score (0-100) to LetterGrade.
func ComputeLetterGrade(score float64, criticalFindings int) LetterGrade {
	if criticalFindings > 0 && score < 85.0 {
		return GradeF
	}
	switch {
	case score >= 95.0:
		return GradeAPlus
	case score >= 85.0:
		return GradeA
	case score >= 70.0:
		return GradeB
	case score >= 50.0:
		return GradeC
	default:
		return GradeF
	}
}

// CalculateScorecard produces the evaluation scorecard from an Oracle audit report and optional critic findings.
func CalculateScorecard(audit *OracleAuditReport, critiques ...*CritiqueReport) *Scorecard {
	if audit == nil {
		audit = &OracleAuditReport{
			Timestamp: time.Now().UTC(),
		}
	}

	sc := &Scorecard{
		Dimensions: make(map[string]DimensionScore),
		Findings:   make([]OracleFinding, 0),
		Timestamp:  time.Now().UTC(),
	}

	// Group findings by dimension
	dimFindings := make(map[string][]OracleFinding)
	for _, f := range audit.Findings {
		dimFindings[f.Dimension] = append(dimFindings[f.Dimension], f)
		sc.Findings = append(sc.Findings, f)
		switch f.Severity {
		case SeverityCritical:
			sc.CriticalFindings++
		case SeverityHigh:
			sc.HighFindings++
		}
		sc.TotalFindings++
	}

	// 1. Dimension: AST Syntactic & Semantic Correctness (25%)
	astScore := 100.0
	astFindings := dimFindings[DimASTSyntax]
	if audit.SyntaxErrorsCount > 0 {
		astScore -= float64(audit.SyntaxErrorsCount) * 35.0
	}
	if astScore < 0 {
		astScore = 0
	}
	sc.Dimensions[DimASTSyntax] = DimensionScore{
		Name:          "AST Syntactic & Semantic Correctness",
		Weight:        0.25,
		Score:         astScore,
		WeightedScore: astScore * 0.25,
		Passed:        astScore >= 80.0,
		FindingsCount: len(astFindings),
		Summary:       fmt.Sprintf("%d/%d files with valid AST syntax", audit.SyntaxValidCount, audit.TotalFilesAudited),
	}

	// 2. Dimension: Cross-Boundary Contract Integrity (20%)
	contractScore := 100.0
	brokenContracts := 0
	driftContracts := 0
	for _, c := range audit.ContractEntries {
		if c.Status == "BROKEN" {
			brokenContracts++
			contractScore -= 30.0
		} else if c.Status == "DRIFT" {
			driftContracts++
			contractScore -= 10.0
		}
	}
	if contractScore < 0 {
		contractScore = 0
	}
	sc.Dimensions[DimContract] = DimensionScore{
		Name:          "Cross-Boundary Contract Integrity",
		Weight:        0.20,
		Score:         contractScore,
		WeightedScore: contractScore * 0.20,
		Passed:        contractScore >= 75.0 && brokenContracts == 0,
		FindingsCount: len(dimFindings[DimContract]),
		Summary:       fmt.Sprintf("%d contracts audited (%d broken, %d drift)", len(audit.ContractEntries), brokenContracts, driftContracts),
	}

	// 3. Dimension: Compilation & Toolchain Validity (15%)
	compScore := 100.0
	compFindings := dimFindings[DimCompilation]
	for _, f := range compFindings {
		if f.Severity == SeverityCritical || f.Severity == SeverityHigh {
			compScore -= 35.0
		} else {
			compScore -= 15.0
		}
	}
	if compScore < 0 {
		compScore = 0
	}
	sc.Dimensions[DimCompilation] = DimensionScore{
		Name:          "Compilation & Toolchain Validity",
		Weight:        0.15,
		Score:         compScore,
		WeightedScore: compScore * 0.15,
		Passed:        compScore >= 80.0,
		FindingsCount: len(compFindings),
		Summary:       fmt.Sprintf("%d compilation / package diagnostics", len(compFindings)),
	}

	// 4. Dimension: Storage Merkle-DAG & Cryptographic Provenance (15%)
	storageScore := 100.0
	if audit.StorageReport.BitRotDetected {
		storageScore -= 50.0
	}
	if audit.StorageReport.ManifestMerkleRoot != "" && !audit.StorageReport.MerkleRootMatch {
		storageScore -= 40.0
	}
	if audit.LineageReport.BrokenChainsCount > 0 {
		storageScore -= 20.0
	}
	if storageScore < 0 {
		storageScore = 0
	}
	sc.Dimensions[DimStorage] = DimensionScore{
		Name:          "Storage Merkle-DAG & Cryptographic Provenance",
		Weight:        0.15,
		Score:         storageScore,
		WeightedScore: storageScore * 0.15,
		Passed:        storageScore >= 85.0 && !audit.StorageReport.BitRotDetected,
		FindingsCount: len(dimFindings[DimStorage]) + len(dimFindings["LINEAGE_PROVENANCE"]),
		Summary: fmt.Sprintf("Objects: %d checked (corrupted: %d), Merkle Match: %v",
			audit.StorageReport.TotalObjectsChecked, audit.StorageReport.CorruptedObjectsCount, audit.StorageReport.MerkleRootMatch),
	}

	// 5. Dimension: Security, IAM & Blast-Radius Safety (15%)
	secScore := 100.0
	secFindings := dimFindings[DimSecurity]
	for _, f := range secFindings {
		switch f.Severity {
		case SeverityCritical:
			secScore -= 40.0
		case SeverityHigh:
			secScore -= 25.0
		case SeverityMedium:
			secScore -= 10.0
		}
	}
	if secScore < 0 {
		secScore = 0
	}
	sc.Dimensions[DimSecurity] = DimensionScore{
		Name:          "Security, IAM & Blast-Radius Safety",
		Weight:        0.15,
		Score:         secScore,
		WeightedScore: secScore * 0.15,
		Passed:        secScore >= 80.0,
		FindingsCount: len(secFindings),
		Summary:       fmt.Sprintf("%d security/IAM findings identified", len(secFindings)),
	}

	// 6. Dimension: Code Architecture & Deduplication Efficiency (10%)
	archScore := 95.0
	for _, crit := range critiques {
		if crit != nil && crit.FitnessScore < 1.0 {
			archScore = crit.FitnessScore * 100.0
		}
	}
	sc.Dimensions[DimArchitecture] = DimensionScore{
		Name:          "Code Architecture & Deduplication Efficiency",
		Weight:        0.10,
		Score:         archScore,
		WeightedScore: archScore * 0.10,
		Passed:        archScore >= 70.0,
		FindingsCount: len(dimFindings["ARCHITECTURE"]),
		Summary:       fmt.Sprintf("Architecture fitness score: %.1f%%", archScore),
	}

	// Calculate overall composite score
	totalWeighted := 0.0
	for _, d := range sc.Dimensions {
		totalWeighted += d.WeightedScore
	}
	sc.OverallScore = totalWeighted
	sc.LetterGrade = ComputeLetterGrade(sc.OverallScore, sc.CriticalFindings)
	sc.Summary = fmt.Sprintf("Evaluation completed with score %.1f/100 (Grade: %s, Findings: %d)",
		sc.OverallScore, sc.LetterGrade, sc.TotalFindings)

	return sc
}

// ToJSON exports the scorecard as formatted JSON.
func (s *Scorecard) ToJSON() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}
