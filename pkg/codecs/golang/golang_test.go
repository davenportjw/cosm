package golang

import (
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

func TestGoParser_ExtractSymbols(t *testing.T) {
	src := `package service

import (
	"fmt"
	"net/http"
	"os"
)

// User represents a system user.
type User struct {
	ID        string ` + "`json:\"id\"`" + `
	Email     string ` + "`json:\"email\"`" + `
	Role      string ` + "`json:\"role\"`" + `
	CreatedAt int64  ` + "`json:\"created_at\"`" + `
}

// UserService defines user operations.
type UserService interface {
	GetUser(id string) (*User, error)
	CreateUser(u *User) error
}

type Server struct {
	port string
}

func NewServer() *Server {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return &Server{port: port}
}

func (s *Server) HandleGetUser(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "user details")
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/users", s.HandleGetUser)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}
`

	parser := NewGoParser()
	lineage := core.LineageEnvelope{
		UserID:     "user-1",
		UserPrompt: "Implement User service with routes",
		SessionID:  "session-1",
		Timestamp:  time.Now().UTC(),
	}

	result, err := parser.ParseSource("service.go", []byte(src), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	if result.PackageName != "service" {
		t.Errorf("expected package name 'service', got '%s'", result.PackageName)
	}

	// Verify Structs
	fileSyms := result.Files["service.go"]
	if len(fileSyms.Structs) != 2 {
		t.Fatalf("expected 2 structs (User, Server), got %d", len(fileSyms.Structs))
	}
	userStruct := fileSyms.Structs[0]
	if userStruct.Name != "User" {
		t.Errorf("expected struct name User, got %s", userStruct.Name)
	}
	if len(userStruct.Fields) != 4 {
		t.Errorf("expected 4 fields in User, got %d", len(userStruct.Fields))
	}

	// Verify Interfaces
	if len(fileSyms.Interfaces) != 1 {
		t.Fatalf("expected 1 interface (UserService), got %d", len(fileSyms.Interfaces))
	}
	iface := fileSyms.Interfaces[0]
	if iface.Name != "UserService" {
		t.Errorf("expected interface UserService, got %s", iface.Name)
	}
	if len(iface.Methods) != 2 {
		t.Errorf("expected 2 methods in UserService, got %d", len(iface.Methods))
	}

	// Verify Functions & Methods
	if len(fileSyms.Functions) != 3 {
		t.Fatalf("expected 3 functions/methods, got %d", len(fileSyms.Functions))
	}

	// Verify Route Bindings
	if len(fileSyms.Routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(fileSyms.Routes))
	}
	routeMap := make(map[string]GoRouteBinding)
	for _, r := range fileSyms.Routes {
		routeMap[r.Path] = r
	}
	if _, ok := routeMap["/api/v1/users"]; !ok {
		t.Errorf("missing route /api/v1/users")
	}
	if _, ok := routeMap["/healthz"]; !ok {
		t.Errorf("missing route /healthz")
	}

	// Verify Env Vars
	if len(fileSyms.EnvVarsAccessed) == 0 || fileSyms.EnvVarsAccessed[0] != "PORT" {
		t.Errorf("expected env var 'PORT' to be extracted, got %v", fileSyms.EnvVarsAccessed)
	}

	// Verify AST Symbol Nodes generated
	if len(result.AllSymbols) == 0 {
		t.Errorf("expected non-empty AllSymbols")
	}
	for _, symNode := range result.AllSymbols {
		if symNode.NodeID == "" {
			t.Errorf("symbol node %s has empty NodeID", symNode.Identifier)
		}
		if symNode.Language != core.LangGo {
			t.Errorf("expected LangGo, got %s", symNode.Language)
		}
	}

	// Test Component Building
	comp, err := BuildComponentNode("auth-service", core.CompService, result, lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode failed: %v", err)
	}
	if comp.Name != "auth-service" {
		t.Errorf("expected component name auth-service, got %s", comp.Name)
	}
	if comp.ComponentID == "" {
		t.Errorf("expected non-empty ComponentID")
	}
	if len(comp.SymbolNodes) != len(result.AllSymbols) {
		t.Errorf("expected %d symbol nodes in component, got %d", len(result.AllSymbols), len(comp.SymbolNodes))
	}
}
