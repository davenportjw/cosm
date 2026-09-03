package shipping

import (
	"encoding/json"
	"fmt"
)

// TargetKind represents the deployment runtime category.
type TargetKind string

const (
	TargetLocalService   TargetKind = "local-service"
	TargetCloudRun       TargetKind = "cloud-run"
	TargetStaticWeb      TargetKind = "static-web"
	TargetTerraformInfra TargetKind = "terraform-infra"
	TargetComposite      TargetKind = "composite"
)

// IsValid checks if target kind is valid.
func (k TargetKind) IsValid() bool {
	switch k {
	case TargetLocalService, TargetCloudRun, TargetStaticWeb, TargetTerraformInfra, TargetComposite:
		return true
	default:
		return false
	}
}

// TargetSpec defines a target environment, build hooks, ports, and packaging rules.
type TargetSpec struct {
	Name            string            `json:"name"` // e.g. "target:local-service", "target:cloud-run"
	Kind            TargetKind        `json:"kind"` // Runtime category
	Description     string            `json:"description,omitempty"`
	ComponentNames  []string          `json:"component_names"`             // Associated ComponentNode names
	BuildCommand    string            `json:"build_command,omitempty"`     // e.g. "go build -o server", "npm run build"
	Environment     map[string]string `json:"environment,omitempty"`       // Env vars for target
	Ports           []int             `json:"ports,omitempty"`             // Exposed port numbers (e.g. 8080)
	HealthCheckPath string            `json:"health_check_path,omitempty"` // e.g. "/healthz"
	OutputDir       string            `json:"output_dir,omitempty"`        // Output build dir
	ValidateHooks   []string          `json:"validate_hooks,omitempty"`    // ["terraform fmt -check", "terraform validate", "go test"]
	ArtifactName    string            `json:"artifact_name,omitempty"`     // Generated package name
}

// Validate verifies that the TargetSpec has required fields.
func (t *TargetSpec) Validate() error {
	if t.Name == "" {
		return fmt.Errorf("target name cannot be empty")
	}
	if !t.Kind.IsValid() {
		return fmt.Errorf("invalid target kind: %s", t.Kind)
	}
	return nil
}

// DefaultLocalServiceTarget returns standard target spec for local Go microservices.
func DefaultLocalServiceTarget(name string, components []string) *TargetSpec {
	if name == "" {
		name = "target:local-service"
	}
	return &TargetSpec{
		Name:           name,
		Kind:           TargetLocalService,
		Description:    "Local runnable Go service binary",
		ComponentNames: components,
		BuildCommand:   "go build -o bin/server ./services/...",
		Environment: map[string]string{
			"PORT":        "8080",
			"ENVIRONMENT": "local",
			"LOG_LEVEL":   "debug",
		},
		Ports:           []int{8080},
		HealthCheckPath: "/healthz",
		OutputDir:       "bin",
		ValidateHooks:   []string{"go test -v ./..."},
		ArtifactName:    "server",
	}
}

// DefaultCosmTarget returns the target spec for building the Cosm CLI binary itself (dogfooding target).
func DefaultCosmTarget() *TargetSpec {
	return &TargetSpec{
		Name:           "target:cosm",
		Kind:           TargetLocalService,
		Description:    "Cosm AI-Native AST SCM CLI binary",
		ComponentNames: []string{"cmd/cosm", "pkg"},
		BuildCommand:   "go build -o bin/cosm ./cmd/cosm",
		OutputDir:      "bin",
		ArtifactName:   "cosm",
	}
}

// DefaultCloudRunTarget returns standard target spec for GCP Cloud Run container services.
func DefaultCloudRunTarget(name string, components []string) *TargetSpec {
	if name == "" {
		name = "target:cloud-run"
	}
	return &TargetSpec{
		Name:           name,
		Kind:           TargetCloudRun,
		Description:    "Google Cloud Run Serverless Container deployment",
		ComponentNames: components,
		BuildCommand:   "go build -tags netgo -ldflags '-s -w' -o /app/server .",
		Environment: map[string]string{
			"PORT":        "8080",
			"ENVIRONMENT": "production",
		},
		Ports:           []int{8080},
		HealthCheckPath: "/healthz",
		OutputDir:       "dist/cloud-run",
		ValidateHooks:   []string{"terraform fmt -check", "terraform validate"},
		ArtifactName:    "cloud-run-bundle.tar.gz",
	}
}

// DefaultStaticWebTarget returns standard target spec for React/TypeScript static web frontends.
func DefaultStaticWebTarget(name string, components []string) *TargetSpec {
	if name == "" {
		name = "target:static-web"
	}
	return &TargetSpec{
		Name:           name,
		Kind:           TargetStaticWeb,
		Description:    "React / TypeScript single page static bundle",
		ComponentNames: components,
		BuildCommand:   "npm run build",
		Environment: map[string]string{
			"NODE_ENV": "production",
		},
		Ports:           []int{3000},
		HealthCheckPath: "/",
		OutputDir:       "dist/static",
		ValidateHooks:   []string{"tsc --noEmit"},
		ArtifactName:    "static-bundle.zip",
	}
}

// DefaultTerraformTarget returns standard target spec for Terraform cloud infrastructure.
func DefaultTerraformTarget(name string, components []string) *TargetSpec {
	if name == "" {
		name = "target:terraform-infra"
	}
	return &TargetSpec{
		Name:           name,
		Kind:           TargetTerraformInfra,
		Description:    "Terraform cloud infrastructure module",
		ComponentNames: components,
		Environment: map[string]string{
			"TF_IN_AUTOMATION": "1",
		},
		OutputDir:     "dist/terraform",
		ValidateHooks: []string{"terraform fmt -check", "terraform validate"},
		ArtifactName:  "terraform-plan.tar.gz",
	}
}

// DefaultPythonTarget returns standard target spec for Python services using uv.
func DefaultPythonTarget(name string, components []string) *TargetSpec {
	if name == "" {
		name = "target:python-service"
	}
	return &TargetSpec{
		Name:           name,
		Kind:           TargetLocalService,
		Description:    "Python service managed via uv runner",
		ComponentNames: components,
		BuildCommand:   "uv run python -m py_compile",
		Environment: map[string]string{
			"PORT":        "8000",
			"ENVIRONMENT": "local",
		},
		Ports:           []int{8000},
		HealthCheckPath: "/health",
		OutputDir:       "bin",
		ValidateHooks:   []string{"uv run pytest"},
		ArtifactName:    "app.pyc",
	}
}

// DefaultRustTarget returns standard target spec for Rust services using cargo.
func DefaultRustTarget(name string, components []string) *TargetSpec {
	if name == "" {
		name = "target:rust-service"
	}
	return &TargetSpec{
		Name:           name,
		Kind:           TargetLocalService,
		Description:    "Rust binary service compiled via cargo",
		ComponentNames: components,
		BuildCommand:   "cargo build --release",
		OutputDir:      "bin",
		ValidateHooks:  []string{"cargo test"},
		ArtifactName:   "app",
	}
}

// DefaultJavaTarget returns standard target spec for Java services using javac/mvn.
func DefaultJavaTarget(name string, components []string) *TargetSpec {
	if name == "" {
		name = "target:java-service"
	}
	return &TargetSpec{
		Name:           name,
		Kind:           TargetLocalService,
		Description:    "Java application compiled via javac / Maven",
		ComponentNames: components,
		BuildCommand:   "javac -d bin",
		OutputDir:      "bin",
		ArtifactName:   "app.jar",
	}
}

// DefaultCppTarget returns standard target spec for C/C++ services using clang++.
func DefaultCppTarget(name string, components []string) *TargetSpec {
	if name == "" {
		name = "target:cpp-service"
	}
	return &TargetSpec{
		Name:           name,
		Kind:           TargetLocalService,
		Description:    "C++ native binary compiled via clang++",
		ComponentNames: components,
		BuildCommand:   "clang++ -O3 -o bin/app",
		OutputDir:      "bin",
		ArtifactName:   "app",
	}
}

// ParseTargetSpec parses JSON bytes into a TargetSpec.
func ParseTargetSpec(data []byte) (*TargetSpec, error) {
	var spec TargetSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse TargetSpec: %w", err)
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	return &spec, nil
}
