package protobuf

import (
	"strings"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

func TestProtobufParser_FullServiceContract(t *testing.T) {
	protoSrc := `syntax = "proto3";

package user.v1;

import "google/api/annotations.proto";

option go_package = "github.com/example/proto/user/v1;userv1";

// User status enum
enum Status {
    STATUS_UNSPECIFIED = 0;
    STATUS_ACTIVE = 1;
    STATUS_SUSPENDED = 2;
}

// User representation message
message User {
    string id = 1;
    string email = 2;
    string name = 3;
    repeated string roles = 4;
    Status status = 5;
}

message GetUserRequest {
    string id = 1;
}

message CreateUserRequest {
    string email = 1;
    string name = 2;
}

// UserService API contract
service UserService {
    // Retrieves a user by ID
    rpc GetUser (GetUserRequest) returns (User) {
        option (google.api.http) = {
            get: "/v1/users/{id}"
        };
    }

    // Creates a new user
    rpc CreateUser (CreateUserRequest) returns (User) {
        option (google.api.http) = {
            post: "/v1/users"
        };
    }
}
`

	lineage := core.LineageEnvelope{
		UserID:           "api-architect",
		UserPrompt:       "Define gRPC UserService contract with HTTP annotations",
		ExecutingAgentID: "agent-proto",
		Timestamp:        time.Now().UTC(),
	}

	parser := NewProtobufParser()
	res, err := parser.ParseSource("proto/user/v1/user.proto", []byte(protoSrc), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	// 1. Validate Header Metadata
	if res.Syntax != "proto3" {
		t.Errorf("Expected syntax proto3, got %s", res.Syntax)
	}
	if res.Package != "user.v1" {
		t.Errorf("Expected package user.v1, got %s", res.Package)
	}
	if len(res.Imports) != 1 {
		t.Errorf("Expected 1 import, got %d", len(res.Imports))
	}
	if res.Options["go_package"] != "github.com/example/proto/user/v1;userv1" {
		t.Errorf("Expected go_package option, got %s", res.Options["go_package"])
	}

	// 2. Validate Enum
	if len(res.Enums) != 1 {
		t.Fatalf("Expected 1 enum, got %d", len(res.Enums))
	}
	if res.Enums[0].Name != "Status" || len(res.Enums[0].Values) != 3 {
		t.Errorf("Unexpected enum: %+v", res.Enums[0])
	}

	// 3. Validate Messages
	if len(res.Messages) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(res.Messages))
	}
	userMsg := res.Messages[0]
	if userMsg.Name != "User" || len(userMsg.Fields) != 5 {
		t.Errorf("Unexpected user message: %+v", userMsg)
	}
	if !userMsg.Fields[3].IsRepeated {
		t.Errorf("Expected roles field to be repeated")
	}

	// 4. Validate Services and RPCs
	if len(res.Services) != 1 {
		t.Fatalf("Expected 1 service, got %d", len(res.Services))
	}
	svc := res.Services[0]
	if svc.Name != "UserService" || len(svc.Methods) != 2 {
		t.Fatalf("Unexpected service: %+v", svc)
	}

	getUserRpc := svc.Methods[0]
	if getUserRpc.Name != "GetUser" || getUserRpc.InputType != "GetUserRequest" || getUserRpc.OutputType != "User" {
		t.Errorf("Unexpected GetUser RPC: %+v", getUserRpc)
	}
	if getUserRpc.HTTPMethod != "GET" || getUserRpc.HTTPPath != "/v1/users/{id}" {
		t.Errorf("Expected GET /v1/users/{id}, got %s %s", getUserRpc.HTTPMethod, getUserRpc.HTTPPath)
	}

	// 5. Validate Component Node
	comp, err := parser.BuildComponentNode(res, "user-contract", core.CompContract, lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode failed: %v", err)
	}
	if comp.Name != "user-contract" || comp.Type != core.CompContract {
		t.Errorf("Unexpected component: %+v", comp)
	}
	if len(comp.SymbolNodes) != len(res.AllSymbols) {
		t.Errorf("Symbol count mismatch: %d vs %d", len(comp.SymbolNodes), len(res.AllSymbols))
	}

	// 6. Validate Hydrator Round-trip
	hydrator := NewProtobufHydrator()
	for _, sym := range res.AllSymbols {
		code, err := hydrator.HydrateSymbol(sym)
		if err != nil {
			t.Fatalf("HydrateSymbol failed for %s: %v", sym.Identifier, err)
		}
		if code == "" {
			t.Fatalf("Hydrated protobuf is empty for %s", sym.Identifier)
		}
		if sym.NodeType == "Service" {
			if !strings.Contains(code, "service UserService") {
				t.Errorf("Hydrated service missing declaration: %s", code)
			}
		}
	}
}
