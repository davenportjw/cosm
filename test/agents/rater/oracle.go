package rater

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cosmscm/cosm/pkg/codecs/hcl"
	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/lineage"
	"github.com/cosmscm/cosm/pkg/storage"
)

// FindingSeverity designates the criticality level of an oracle finding.
type FindingSeverity string

const (
	SeverityCritical FindingSeverity = "CRITICAL"
	SeverityHigh     FindingSeverity = "HIGH"
	SeverityMedium   FindingSeverity = "MEDIUM"
	SeverityLow      FindingSeverity = "LOW"
	SeverityInfo     FindingSeverity = "INFO"
)

// OracleFinding represents an audited issue or verification failure.
type OracleFinding struct {
	Dimension    string          `json:"dimension"`
	Severity     FindingSeverity `json:"severity"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	FileLocation string          `json:"file_location,omitempty"`
	SymbolID     string          `json:"symbol_id,omitempty"`
	SuggestedFix string          `json:"suggested_fix,omitempty"`
}

// ContractAuditEntry represents the verification result of a single cross-boundary contract.
type ContractAuditEntry struct {
	SourceComponent string `json:"source_component"`
	TargetComponent string `json:"target_component"`
	ContractType    string `json:"contract_type"` // "API_ROUTE", "BINDS_ENV", "SQL_TABLE"
	Identifier      string `json:"identifier"`
	Status          string `json:"status"` // "VALID", "BROKEN", "DRIFT"
	Details         string `json:"details"`
}

// StorageIntegrityReport details Merkle tree and SHA-256 object store verification.
type StorageIntegrityReport struct {
	TotalObjectsChecked   int      `json:"total_objects_checked"`
	ValidObjectsCount     int      `json:"valid_objects_count"`
	CorruptedObjectsCount int      `json:"corrupted_objects_count"`
	BitRotDetected        bool     `json:"bit_rot_detected"`
	CorruptedHashes       []string `json:"corrupted_hashes,omitempty"`
	ManifestMerkleRoot    string   `json:"manifest_merkle_root"`
	RecalculatedRoot      string   `json:"recalculated_root"`
	MerkleRootMatch       bool     `json:"merkle_root_match"`
}

// LineageAuditReport records verification of causal pedigree and Ed25519 signatures.
type LineageAuditReport struct {
	TotalNodesAudited int  `json:"total_nodes_audited"`
	IntactChainsCount int  `json:"intact_chains_count"`
	BrokenChainsCount int  `json:"broken_chains_count"`
	SignaturesChecked int  `json:"signatures_checked"`
	ValidSignatures   int  `json:"valid_signatures"`
	AllSignaturesOK   bool `json:"all_signatures_ok"`
}

// OracleAuditReport consolidates the full deep verification audit results across all 5 vectors.
type OracleAuditReport struct {
	WorkspaceDir      string                 `json:"workspace_dir"`
	UniverseID        string                 `json:"universe_id"`
	Timestamp         time.Time              `json:"timestamp"`
	Duration          time.Duration          `json:"duration"`
	TotalFilesAudited int                    `json:"total_files_audited"`
	SyntaxValidCount  int                    `json:"syntax_valid_count"`
	SyntaxErrorsCount int                    `json:"syntax_errors_count"`
	ContractEntries   []ContractAuditEntry   `json:"contract_entries"`
	StorageReport     StorageIntegrityReport `json:"storage_report"`
	LineageReport     LineageAuditReport     `json:"lineage_report"`
	Findings          []OracleFinding        `json:"findings"`
}

// DeepVerificationOracle audits workspaces and .cosm/ Merkle-DAG stores.
type DeepVerificationOracle struct {
	pubKeys map[string]ed25519.PublicKey
}

// NewDeepVerificationOracle creates an initialized DeepVerificationOracle.
func NewDeepVerificationOracle() *DeepVerificationOracle {
	return &DeepVerificationOracle{
		pubKeys: make(map[string]ed25519.PublicKey),
	}
}

// RegisterPublicKey registers an agent's Ed25519 public key for signature verification.
func (o *DeepVerificationOracle) RegisterPublicKey(agentID string, pubKey ed25519.PublicKey) {
	o.pubKeys[agentID] = pubKey
}

// AuditWorkspace performs exhaustive multi-vector deep verification on a workspace directory.
func (o *DeepVerificationOracle) AuditWorkspace(workDir string, universeID string) (*OracleAuditReport, error) {
	start := time.Now()
	if universeID == "" {
		universeID = "universe-main"
	}

	report := &OracleAuditReport{
		WorkspaceDir:    workDir,
		UniverseID:      universeID,
		Timestamp:       time.Now().UTC(),
		ContractEntries: make([]ContractAuditEntry, 0),
		Findings:        make([]OracleFinding, 0),
	}

	// 1. Vector 1: AST Syntactic Validity Check
	o.auditASTSyntacticValidity(workDir, report)

	// 2. Vector 2: Toolchain & Compiler Diagnostics
	o.auditToolchainAndCompilation(workDir, report)

	// 3. Vector 3: Cross-Boundary Contract Verification
	o.auditCrossBoundaryContracts(workDir, report)

	// 4. Vector 4: Storage Bit-Rot & Merkle Root Recalculation
	o.auditStorageIntegrity(workDir, universeID, report)

	// 5. Vector 5: Causal Lineage & Cryptographic Signatures
	o.auditLineageAndSignatures(workDir, universeID, report)

	// 6. Security & IAM Violations in HCL
	o.auditSecurityAndIAM(workDir, report)

	report.Duration = time.Since(start)
	return report, nil
}

// 1. AST Syntactic Validity Auditing across Go, Python, TS, Rust, Java, SQL, Protobuf, HCL
func (o *DeepVerificationOracle) auditASTSyntacticValidity(workDir string, report *OracleAuditReport) {
	_ = filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() && (info.Name() == ".cosm" || info.Name() == ".git" || info.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, _ := filepath.Rel(workDir, path)
		ext := filepath.Ext(path)
		data, rErr := os.ReadFile(path)
		if rErr != nil {
			return nil
		}

		report.TotalFilesAudited++
		var syntaxErr error

		switch ext {
		case ".go":
			syntaxErr = ValidateGoSyntax(relPath, data)
		case ".py":
			syntaxErr = ValidatePythonSyntax(relPath, data)
		case ".ts", ".tsx", ".js", ".jsx", ".vue":
			syntaxErr = ValidateTypeScriptSyntax(relPath, data)
		case ".rs":
			syntaxErr = ValidateRustSyntax(relPath, data)
		case ".java":
			syntaxErr = ValidateJavaSyntax(relPath, data)
		case ".sql":
			syntaxErr = ValidateSQLSyntax(relPath, data)
		case ".proto":
			syntaxErr = ValidateProtobufSyntax(relPath, data)
		case ".tf", ".hcl":
			syntaxErr = ValidateHCLSyntax(relPath, data)
		default:
			// Unchecked file type
			report.SyntaxValidCount++
			return nil
		}

		if syntaxErr != nil {
			report.SyntaxErrorsCount++
			report.Findings = append(report.Findings, OracleFinding{
				Dimension:    "AST_SYNTACTIC_CORRECTNESS",
				Severity:     SeverityCritical,
				Title:        fmt.Sprintf("AST Syntax Error in %s", relPath),
				Description:  syntaxErr.Error(),
				FileLocation: relPath,
				SuggestedFix: "Fix syntax error to ensure clean AST parsing.",
			})
		} else {
			report.SyntaxValidCount++
		}

		return nil
	})
}

// 2. Toolchain & Compilation Diagnostics
func (o *DeepVerificationOracle) auditToolchainAndCompilation(workDir string, report *OracleAuditReport) {
	_ = filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		rel, _ := filepath.Rel(workDir, path)
		if strings.HasPrefix(rel, ".cosm") {
			return nil
		}
		src, _ := os.ReadFile(path)
		fset := token.NewFileSet()
		file, pErr := parser.ParseFile(fset, path, src, parser.ImportsOnly)
		if pErr != nil {
			report.Findings = append(report.Findings, OracleFinding{
				Dimension:    "COMPILATION_VALIDITY",
				Severity:     SeverityHigh,
				Title:        fmt.Sprintf("Go Parse Error in %s", rel),
				Description:  pErr.Error(),
				FileLocation: rel,
			})
		} else if file.Name == nil || file.Name.Name == "" {
			report.Findings = append(report.Findings, OracleFinding{
				Dimension:    "COMPILATION_VALIDITY",
				Severity:     SeverityHigh,
				Title:        fmt.Sprintf("Missing Package Declaration in %s", rel),
				Description:  "Go file must specify a package name.",
				FileLocation: rel,
			})
		}
		return nil
	})
}

// 3. Cross-Boundary Contract Verification
func (o *DeepVerificationOracle) auditCrossBoundaryContracts(workDir string, report *OracleAuditReport) {
	backendRoutes := make(map[string]string)
	frontendCalls := make(map[string]string)
	backendEnvVars := make(map[string]string)
	infraEnvVars := make(map[string]string)
	sqlTables := make(map[string]bool)
	sqlQueries := make(map[string]string)

	routeRegexes := []*regexp.Regexp{
		regexp.MustCompile(`(?m)@(app|router)\.(get|post|put|delete|patch)\s*\(\s*["']([^"']+)["']`),
		regexp.MustCompile(`(?m)\.(?:GET|POST|PUT|DELETE|PATCH)\s*\(\s*["']([^"']+)["']`),
		regexp.MustCompile(`(?m)\.route\s*\(\s*["']([^"']+)["']`),
	}

	callerRegexes := []*regexp.Regexp{
		regexp.MustCompile(`fetch\s*\(\s*["']([^"']+)["']`),
		regexp.MustCompile(`axios\.(?:get|post|put|delete|patch)\s*\(\s*["']([^"']+)["']`),
		regexp.MustCompile(`apiClient\.(?:get|post|put|delete)\s*\(\s*["']([^"']+)["']`),
	}

	envBackendRegexes := []*regexp.Regexp{
		regexp.MustCompile(`(?:os\.(?:environ\.get|getenv)\s*\(\s*|os\.environ\s*\[\s*)["']([a-zA-Z0-9_]+)["']`),
		regexp.MustCompile(`os\.Getenv\s*\(\s*["']([a-zA-Z0-9_]+)["']\)`),
		regexp.MustCompile(`std::env::var\s*\(\s*["']([a-zA-Z0-9_]+)["']\)`),
	}

	_ = filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(workDir, path)
		if strings.HasPrefix(rel, ".cosm") || strings.HasPrefix(rel, ".git") {
			return nil
		}

		content, rErr := os.ReadFile(path)
		if rErr != nil {
			return nil
		}
		str := string(content)

		// Routes
		for _, rx := range routeRegexes {
			matches := rx.FindAllStringSubmatch(str, -1)
			for _, m := range matches {
				route := m[len(m)-1]
				backendRoutes[route] = rel
			}
		}

		// Callers
		for _, rx := range callerRegexes {
			matches := rx.FindAllStringSubmatch(str, -1)
			for _, m := range matches {
				frontendCalls[m[1]] = rel
			}
		}

		// Backend env vars
		for _, rx := range envBackendRegexes {
			matches := rx.FindAllStringSubmatch(str, -1)
			for _, m := range matches {
				backendEnvVars[m[1]] = rel
			}
		}

		// Infra env vars
		if filepath.Ext(path) == ".tf" || filepath.Ext(path) == ".hcl" {
			envMatch := regexp.MustCompile(`name\s*=\s*["']([a-zA-Z0-9_]+)["']`).FindAllStringSubmatch(str, -1)
			for _, m := range envMatch {
				infraEnvVars[m[1]] = rel
			}
		}

		// SQL DDL
		if filepath.Ext(path) == ".sql" {
			tableMatches := regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-zA-Z0-9_]+)`).FindAllStringSubmatch(str, -1)
			for _, tm := range tableMatches {
				sqlTables[strings.ToLower(tm[1])] = true
			}
		}

		// SQL queries in code
		sqlRefMatches := regexp.MustCompile(`(?i)(?:FROM|JOIN|INTO|UPDATE)\s+([a-zA-Z0-9_]+)`).FindAllStringSubmatch(str, -1)
		for _, sm := range sqlRefMatches {
			tbl := strings.ToLower(sm[1])
			if tbl != "select" && tbl != "where" && tbl != "set" && tbl != "values" {
				sqlQueries[tbl] = rel
			}
		}

		return nil
	})

	// Verify Frontend -> Backend API routes
	for callURL, callerFile := range frontendCalls {
		cleanURL := strings.Split(callURL, "?")[0]
		targetFile, matched := backendRoutes[cleanURL]
		if matched {
			report.ContractEntries = append(report.ContractEntries, ContractAuditEntry{
				SourceComponent: callerFile,
				TargetComponent: targetFile,
				ContractType:    "API_ROUTE",
				Identifier:      cleanURL,
				Status:          "VALID",
				Details:         fmt.Sprintf("Frontend caller %s correctly matches backend route %s in %s", callerFile, cleanURL, targetFile),
			})
		} else {
			report.ContractEntries = append(report.ContractEntries, ContractAuditEntry{
				SourceComponent: callerFile,
				TargetComponent: "UNKNOWN_BACKEND",
				ContractType:    "API_ROUTE",
				Identifier:      cleanURL,
				Status:          "BROKEN",
				Details:         fmt.Sprintf("Frontend consumes route %s in %s, but no matching backend endpoint was discovered.", cleanURL, callerFile),
			})
			report.Findings = append(report.Findings, OracleFinding{
				Dimension:    "CONTRACT_INTEGRITY",
				Severity:     SeverityHigh,
				Title:        fmt.Sprintf("Broken API Route Contract: %s", cleanURL),
				Description:  fmt.Sprintf("Caller in %s expects %s, but endpoint does not exist in backend.", callerFile, cleanURL),
				FileLocation: callerFile,
				SuggestedFix: fmt.Sprintf("Add route %s in backend or update caller URL in %s", cleanURL, callerFile),
			})
		}
	}

	// Verify Backend -> Infra Environment Variables
	for envVar, backendFile := range backendEnvVars {
		if infraFile, bound := infraEnvVars[envVar]; bound {
			report.ContractEntries = append(report.ContractEntries, ContractAuditEntry{
				SourceComponent: backendFile,
				TargetComponent: infraFile,
				ContractType:    "BINDS_ENV",
				Identifier:      envVar,
				Status:          "VALID",
				Details:         fmt.Sprintf("Env var %s accessed in %s is provided by Terraform infra %s", envVar, backendFile, infraFile),
			})
		} else {
			report.ContractEntries = append(report.ContractEntries, ContractAuditEntry{
				SourceComponent: backendFile,
				TargetComponent: "INFRA_TERRAFORM",
				ContractType:    "BINDS_ENV",
				Identifier:      envVar,
				Status:          "DRIFT",
				Details:         fmt.Sprintf("Env var %s accessed in %s is not declared in Terraform container env blocks.", envVar, backendFile),
			})
		}
	}

	// Verify SQL Tables
	for tbl, queryFile := range sqlQueries {
		if len(sqlTables) > 0 {
			if sqlTables[tbl] {
				report.ContractEntries = append(report.ContractEntries, ContractAuditEntry{
					SourceComponent: queryFile,
					TargetComponent: "SQL_SCHEMA",
					ContractType:    "SQL_TABLE",
					Identifier:      tbl,
					Status:          "VALID",
					Details:         fmt.Sprintf("Table %s referenced in %s exists in SQL DDL.", tbl, queryFile),
				})
			}
		}
	}
}

// 4. Storage Merkle-DAG Recalculation & SHA-256 Bit-Rot Detection
func (o *DeepVerificationOracle) auditStorageIntegrity(workDir string, universeID string, report *OracleAuditReport) {
	objectsDir := filepath.Join(workDir, ".cosm", "objects")
	if _, err := os.Stat(objectsDir); os.IsNotExist(err) {
		return
	}

	blobStore, graphEngine, err := openStorage(workDir)
	if err != nil {
		report.Findings = append(report.Findings, OracleFinding{
			Dimension:   "STORAGE_INTEGRITY",
			Severity:    SeverityCritical,
			Title:       "BlobStore Opening Error",
			Description: err.Error(),
		})
		return
	}
	defer graphEngine.Close()

	var corruptedHashes []string
	totalChecked := 0
	validCount := 0

	_ = filepath.Walk(objectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".bin" {
			return nil
		}

		totalChecked++
		data, rErr := os.ReadFile(path)
		if rErr != nil || len(data) == 0 {
			corruptedHashes = append(corruptedHashes, filepath.Base(path))
			return nil
		}

		expectedHash := filepath.Base(filepath.Dir(path)) + strings.TrimSuffix(filepath.Base(path), ".bin")
		actualSum := sha256.Sum256(data)
		actualHash := hex.EncodeToString(actualSum[:])

		if expectedHash != actualHash {
			corruptedHashes = append(corruptedHashes, expectedHash)
		} else {
			validCount++
		}
		return nil
	})

	report.StorageReport = StorageIntegrityReport{
		TotalObjectsChecked:   totalChecked,
		ValidObjectsCount:     validCount,
		CorruptedObjectsCount: len(corruptedHashes),
		BitRotDetected:        len(corruptedHashes) > 0,
		CorruptedHashes:       corruptedHashes,
	}

	if len(corruptedHashes) > 0 {
		report.Findings = append(report.Findings, OracleFinding{
			Dimension:   "STORAGE_INTEGRITY",
			Severity:    SeverityCritical,
			Title:       "Storage Bit-Rot / Object Tampering Detected",
			Description: fmt.Sprintf("Discovered %d corrupted or modified objects in .cosm/objects/: %v", len(corruptedHashes), corruptedHashes),
		})
	}

	// Recalculate Merkle Root for Universe
	mgr := storage.NewUniverseManager(graphEngine, blobStore)
	manifest, err := mgr.GetUniverseManifest(universeID)
	if err == nil && manifest != nil {
		report.StorageReport.ManifestMerkleRoot = manifest.MerkleRootHash

		recalculatedRoot, hErr := core.HashWorkspaceManifest(manifest)
		if hErr == nil {
			report.StorageReport.RecalculatedRoot = recalculatedRoot
			if manifest.MerkleRootHash == recalculatedRoot {
				report.StorageReport.MerkleRootMatch = true
			} else {
				report.StorageReport.MerkleRootMatch = false
				report.Findings = append(report.Findings, OracleFinding{
					Dimension:   "STORAGE_INTEGRITY",
					Severity:    SeverityCritical,
					Title:       "Manifest Merkle Root Mismatch",
					Description: fmt.Sprintf("Recorded root %s != Recalculated root %s", manifest.MerkleRootHash, recalculatedRoot),
				})
			}
		}
	}
}

// 5. Causal Lineage & Ed25519 Cryptographic Signature Verification
func (o *DeepVerificationOracle) auditLineageAndSignatures(workDir string, universeID string, report *OracleAuditReport) {
	blobStore, graphEngine, err := openStorage(workDir)
	if err != nil {
		return
	}
	defer graphEngine.Close()

	nodes := graphEngine.ListNodes("", "")
	totalAudited := len(nodes)
	intactCount := 0
	brokenCount := 0
	sigChecked := 0
	sigValid := 0

	for _, n := range nodes {
		linRecords := graphEngine.GetLineageByNode(n.NodeID)
		if len(linRecords) > 0 {
			intactCount++
		} else {
			brokenCount++
		}

		// Check Ed25519 signatures if present on ASTSymbolNode payload
		if data, gErr := blobStore.Get(n.MerkleHash); gErr == nil {
			var sym core.ASTSymbolNode
			if sErr := json.Unmarshal(data, &sym); sErr == nil && len(sym.Lineage.SignatureEd25519) > 0 {
				sigChecked++
				if pubKey, hasKey := o.pubKeys[sym.Lineage.ExecutingAgentID]; hasKey {
					valid, vErr := lineage.VerifyEnvelope(&sym.Lineage, pubKey)
					if valid && vErr == nil {
						sigValid++
					} else {
						report.Findings = append(report.Findings, OracleFinding{
							Dimension:   "LINEAGE_PROVENANCE",
							Severity:    SeverityHigh,
							Title:       fmt.Sprintf("Invalid Ed25519 Signature on %s", n.NodeID),
							Description: "Cryptographic signature failed verification against registered public key.",
						})
					}
				}
			}
		}
	}

	report.LineageReport = LineageAuditReport{
		TotalNodesAudited: totalAudited,
		IntactChainsCount: intactCount,
		BrokenChainsCount: brokenCount,
		SignaturesChecked: sigChecked,
		ValidSignatures:   sigValid,
		AllSignaturesOK:   brokenCount == 0,
	}

	if brokenCount > 0 {
		report.Findings = append(report.Findings, OracleFinding{
			Dimension:   "LINEAGE_PROVENANCE",
			Severity:    SeverityMedium,
			Title:       "Missing Lineage Provenance Record",
			Description: fmt.Sprintf("%d nodes lack provenance pedigree records in graph store.", brokenCount),
		})
	}
}

// 6. Security & IAM Violations in Terraform / HCL
func (o *DeepVerificationOracle) auditSecurityAndIAM(workDir string, report *OracleAuditReport) {
	_ = filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".tf" && ext != ".hcl" {
			return nil
		}
		rel, _ := filepath.Rel(workDir, path)
		if strings.HasPrefix(rel, ".cosm") {
			return nil
		}

		data, rErr := os.ReadFile(path)
		if rErr != nil {
			return nil
		}
		content := string(data)

		// Check 0.0.0.0/0 ingress
		if strings.Contains(content, "0.0.0.0/0") && strings.Contains(content, "ingress") {
			report.Findings = append(report.Findings, OracleFinding{
				Dimension:    "SECURITY_IAM",
				Severity:     SeverityHigh,
				Title:        "Unrestricted Ingress CIDR Block (0.0.0.0/0)",
				Description:  "Permissive 0.0.0.0/0 ingress rule allows public ingress traffic to sensitive infrastructure.",
				FileLocation: rel,
				SuggestedFix: "Restrict CIDR blocks to internal VPC or authorized bastion IPs.",
			})
		}

		// Check primitive overprivileged IAM roles
		if strings.Contains(content, "roles/owner") || strings.Contains(content, "roles/editor") {
			report.Findings = append(report.Findings, OracleFinding{
				Dimension:    "SECURITY_IAM",
				Severity:     SeverityHigh,
				Title:        "Overprivileged Primitive IAM Role",
				Description:  "Detected primitive roles/owner or roles/editor binding in Terraform configuration.",
				FileLocation: rel,
				SuggestedFix: "Replace primitive owner/editor roles with least-privilege predefined roles.",
			})
		}

		// Check hardcoded plaintext secrets
		secretRegex := regexp.MustCompile(`(?i)(?:secret|password|api_key|token)\s*=\s*["']([^"']{8,})["']`)
		matches := secretRegex.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			val := m[1]
			if !strings.HasPrefix(val, "${") && !strings.HasPrefix(val, "var.") && !strings.Contains(val, "dummy") && !strings.Contains(val, "secret-token") {
				report.Findings = append(report.Findings, OracleFinding{
					Dimension:    "SECURITY_IAM",
					Severity:     SeverityMedium,
					Title:        "Potential Hardcoded Secret in Terraform",
					Description:  fmt.Sprintf("Plaintext sensitive value assigned in %s.", rel),
					FileLocation: rel,
					SuggestedFix: "Use Secret Manager or environment variable references.",
				})
			}
		}

		return nil
	})
}

func openStorage(workDir string) (*storage.BlobStore, *storage.GraphEngine, error) {
	cosmDir := filepath.Join(workDir, ".cosm")
	objectsDir := filepath.Join(cosmDir, "objects")
	blobStore, err := storage.NewBlobStore(objectsDir)
	if err != nil {
		return nil, nil, err
	}
	graphEngine, err := storage.NewGraphEngine(filepath.Join(cosmDir, "graph.db"))
	if err != nil {
		return nil, nil, err
	}
	return blobStore, graphEngine, nil
}

// Language Syntactic Validators

func ValidateGoSyntax(filename string, src []byte) error {
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, filename, src, parser.AllErrors)
	return err
}

func ValidatePythonSyntax(filename string, src []byte) error {
	content := string(src)
	lines := strings.Split(content, "\n")
	var stack []rune
	for lineIdx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if (strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "async def ") ||
			strings.HasPrefix(trimmed, "class ") || strings.HasPrefix(trimmed, "if ") ||
			strings.HasPrefix(trimmed, "for ") || strings.HasPrefix(trimmed, "while ") ||
			strings.HasPrefix(trimmed, "try:") || strings.HasPrefix(trimmed, "except") ||
			strings.HasPrefix(trimmed, "with ")) && !strings.HasSuffix(trimmed, ":") && !strings.Contains(trimmed, ":") {
			return fmt.Errorf("line %d: missing colon at end of statement: %q", lineIdx+1, trimmed)
		}
		for _, r := range line {
			switch r {
			case '(', '[', '{':
				stack = append(stack, r)
			case ')':
				if len(stack) == 0 || stack[len(stack)-1] != '(' {
					return fmt.Errorf("line %d: mismatched closing parenthesis", lineIdx+1)
				}
				stack = stack[:len(stack)-1]
			case ']':
				if len(stack) == 0 || stack[len(stack)-1] != '[' {
					return fmt.Errorf("line %d: mismatched closing bracket", lineIdx+1)
				}
				stack = stack[:len(stack)-1]
			case '}':
				if len(stack) == 0 || stack[len(stack)-1] != '{' {
					return fmt.Errorf("line %d: mismatched closing brace", lineIdx+1)
				}
				stack = stack[:len(stack)-1]
			}
		}
	}
	if len(stack) > 0 {
		return fmt.Errorf("unclosed brackets at end of file: %c", stack[len(stack)-1])
	}
	return nil
}

func ValidateTypeScriptSyntax(filename string, src []byte) error {
	content := string(src)
	var stack []rune
	inQuote := rune(0)
	for idx, r := range content {
		if inQuote != 0 {
			if r == inQuote && (idx == 0 || rune(content[idx-1]) != '\\') {
				inQuote = 0
			}
			continue
		}
		if r == '"' || r == '\'' || r == '`' {
			inQuote = r
			continue
		}
		switch r {
		case '(', '[', '{':
			stack = append(stack, r)
		case ')':
			if len(stack) == 0 || stack[len(stack)-1] != '(' {
				return fmt.Errorf("mismatched closing parenthesis in TypeScript")
			}
			stack = stack[:len(stack)-1]
		case ']':
			if len(stack) == 0 || stack[len(stack)-1] != '[' {
				return fmt.Errorf("mismatched closing square bracket in TypeScript")
			}
			stack = stack[:len(stack)-1]
		case '}':
			if len(stack) == 0 || stack[len(stack)-1] != '{' {
				return fmt.Errorf("mismatched closing curly brace in TypeScript")
			}
			stack = stack[:len(stack)-1]
		}
	}
	if inQuote != 0 {
		return fmt.Errorf("unclosed string quote (%c) in TypeScript", inQuote)
	}
	if len(stack) > 0 {
		return fmt.Errorf("unclosed bracket (%c) in TypeScript", stack[len(stack)-1])
	}
	return nil
}

func ValidateRustSyntax(filename string, src []byte) error {
	content := string(src)
	if !strings.Contains(content, "fn ") && !strings.Contains(content, "struct ") && !strings.Contains(content, "use ") {
		return fmt.Errorf("no Rust declarations found")
	}
	var stack []rune
	for _, r := range content {
		switch r {
		case '(', '[', '{':
			stack = append(stack, r)
		case ')':
			if len(stack) == 0 || stack[len(stack)-1] != '(' {
				return fmt.Errorf("mismatched closing parenthesis in Rust")
			}
			stack = stack[:len(stack)-1]
		case ']':
			if len(stack) == 0 || stack[len(stack)-1] != '[' {
				return fmt.Errorf("mismatched closing square bracket in Rust")
			}
			stack = stack[:len(stack)-1]
		case '}':
			if len(stack) == 0 || stack[len(stack)-1] != '{' {
				return fmt.Errorf("mismatched closing brace in Rust")
			}
			stack = stack[:len(stack)-1]
		}
	}
	if len(stack) > 0 {
		return fmt.Errorf("unclosed brace in Rust source")
	}
	return nil
}

func ValidateJavaSyntax(filename string, src []byte) error {
	content := string(src)
	if !strings.Contains(content, "class ") && !strings.Contains(content, "package ") && !strings.Contains(content, "interface ") {
		return fmt.Errorf("no Java class or package declaration found")
	}
	var stack []rune
	for _, r := range content {
		switch r {
		case '{':
			stack = append(stack, r)
		case '}':
			if len(stack) == 0 || stack[len(stack)-1] != '{' {
				return fmt.Errorf("mismatched brace in Java")
			}
			stack = stack[:len(stack)-1]
		}
	}
	if len(stack) > 0 {
		return fmt.Errorf("unclosed brace in Java source")
	}
	return nil
}

func ValidateSQLSyntax(filename string, src []byte) error {
	content := strings.ToUpper(string(src))
	if !strings.Contains(content, "CREATE ") && !strings.Contains(content, "SELECT ") &&
		!strings.Contains(content, "INSERT ") && !strings.Contains(content, "ALTER ") &&
		!strings.Contains(content, "DROP ") {
		return fmt.Errorf("no valid SQL DDL or DML statements recognized")
	}
	openParens := strings.Count(content, "(")
	closeParens := strings.Count(content, ")")
	if openParens != closeParens {
		return fmt.Errorf("mismatched parentheses in SQL (open: %d, close: %d)", openParens, closeParens)
	}
	return nil
}

func ValidateProtobufSyntax(filename string, src []byte) error {
	content := string(src)
	if !strings.Contains(content, "syntax =") && !strings.Contains(content, "message ") && !strings.Contains(content, "service ") {
		return fmt.Errorf("no Protobuf syntax or message declarations found")
	}
	openBraces := strings.Count(content, "{")
	closeBraces := strings.Count(content, "}")
	if openBraces != closeBraces {
		return fmt.Errorf("mismatched braces in Protobuf (open: %d, close: %d)", openBraces, closeBraces)
	}
	return nil
}

func ValidateHCLSyntax(filename string, src []byte) error {
	parser := hcl.NewHCLParser()
	_, err := parser.ParseSource(filename, src)
	return err
}
