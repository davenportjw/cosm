package rust

import (
	"strings"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

func TestRustParser_FullFeatures(t *testing.T) {
	rustSrc := `//! Auth and user service crate
use std::collections::HashMap;
use serde::{Serialize, Deserialize};
use actix_web::{get, post, web, HttpResponse, Responder};

/// User profile data model
#[derive(Serialize, Deserialize, Debug, Clone)]
#[serde(rename_all = "camelCase")]
pub struct UserProfile {
    pub id: String,
    pub email: String,
    pub age: u32,
}

/// Status enumeration
#[derive(Serialize, Deserialize, Debug, PartialEq)]
pub enum UserStatus {
    Active,
    Suspended,
    Deleted = 99,
}

/// Service trait defining user repository operations
pub trait UserRepository {
    fn find_by_id(&self, id: &str) -> Option<UserProfile>;
    async fn save(&mut self, user: UserProfile) -> Result<(), String>;
}

pub struct PgUserRepository {
    pool: String,
}

impl UserRepository for PgUserRepository {
    fn find_by_id(&self, id: &str) -> Option<UserProfile> {
        let q = sqlx::query!("SELECT id, email, age FROM users WHERE id = $1", id);
        None
    }

    async fn save(&mut self, user: UserProfile) -> Result<(), String> {
        Ok(())
    }
}

/// Get user endpoint
#[get("/api/v1/users/{id}")]
pub async fn get_user_handler(path: web::Path<String>) -> impl Responder {
    let user_id = path.into_inner();
    let query = sqlx::query!("SELECT id, email FROM users WHERE id = $1", user_id);
    HttpResponse::Ok().json(user_id)
}

/// Create user endpoint
#[post("/api/v1/users")]
pub async fn create_user_handler(body: web::Json<UserProfile>) -> impl Responder {
    HttpResponse::Created().finish()
}

macro_rules! log_info {
    ($msg:expr) => {
        println!("[INFO] {}", $msg);
    };
}
`

	lineage := core.LineageEnvelope{
		UserID:           "rust-dev",
		UserPrompt:       "Implement Rust auth service with Actix and SQLx",
		ExecutingAgentID: "agent-rust",
		Timestamp:        time.Now().UTC(),
	}

	parser := NewRustParser()
	res, err := parser.ParseSource("src/services/auth.rs", []byte(rustSrc), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	// 1. Validate Imports
	if len(res.Imports) < 3 {
		t.Errorf("Expected at least 3 imports, got %d", len(res.Imports))
	}

	// 2. Validate Structs
	if len(res.Structs) < 2 {
		t.Fatalf("Expected at least 2 structs, got %d", len(res.Structs))
	}
	userStruct := res.Structs[0]
	if userStruct.Name != "UserProfile" {
		t.Errorf("Expected struct name UserProfile, got %s", userStruct.Name)
	}
	if userStruct.Visibility != "pub" {
		t.Errorf("Expected visibility pub, got %s", userStruct.Visibility)
	}
	if len(userStruct.Derives) != 4 {
		t.Errorf("Expected 4 derives (Serialize, Deserialize, Debug, Clone), got %v", userStruct.Derives)
	}
	if len(userStruct.Fields) != 3 {
		t.Errorf("Expected 3 fields, got %d", len(userStruct.Fields))
	}

	// 3. Validate Enums
	if len(res.Enums) != 1 {
		t.Fatalf("Expected 1 enum, got %d", len(res.Enums))
	}
	statusEnum := res.Enums[0]
	if statusEnum.Name != "UserStatus" {
		t.Errorf("Expected enum UserStatus, got %s", statusEnum.Name)
	}
	if len(statusEnum.Variants) != 3 {
		t.Errorf("Expected 3 variants, got %d", len(statusEnum.Variants))
	}

	// 4. Validate Traits
	if len(res.Traits) != 1 {
		t.Fatalf("Expected 1 trait, got %d", len(res.Traits))
	}
	repoTrait := res.Traits[0]
	if repoTrait.Name != "UserRepository" {
		t.Errorf("Expected trait UserRepository, got %s", repoTrait.Name)
	}
	if len(repoTrait.Methods) != 2 {
		t.Errorf("Expected 2 trait methods, got %d", len(repoTrait.Methods))
	}

	// 5. Validate Impls
	if len(res.Impls) != 1 {
		t.Fatalf("Expected 1 impl block, got %d", len(res.Impls))
	}
	implBlock := res.Impls[0]
	if implBlock.TraitName != "UserRepository" || implBlock.TargetType != "PgUserRepository" {
		t.Errorf("Expected impl UserRepository for PgUserRepository, got %s for %s", implBlock.TraitName, implBlock.TargetType)
	}

	// 6. Validate Functions & Routes
	if len(res.Functions) < 2 {
		t.Fatalf("Expected at least 2 top-level functions, got %d", len(res.Functions))
	}
	getFn := res.Functions[0]
	if getFn.Name != "get_user_handler" {
		t.Errorf("Expected get_user_handler, got %s", getFn.Name)
	}
	if !getFn.IsAsync {
		t.Errorf("Expected get_user_handler to be async")
	}
	if getFn.RouteMethod != "GET" || getFn.RoutePath != "/api/v1/users/{id}" {
		t.Errorf("Expected GET /api/v1/users/{id}, got %s %s", getFn.RouteMethod, getFn.RoutePath)
	}

	// 7. Validate SQLx Query extraction
	if len(res.SQLxQueries) < 2 {
		t.Errorf("Expected at least 2 SQLx queries extracted, got %d (%v)", len(res.SQLxQueries), res.SQLxQueries)
	}

	// 8. Validate Macros
	if len(res.Macros) != 1 {
		t.Fatalf("Expected 1 macro, got %d", len(res.Macros))
	}
	if res.Macros[0].Name != "log_info" {
		t.Errorf("Expected macro log_info, got %s", res.Macros[0].Name)
	}

	// 9. Validate Route Bindings
	if len(res.Routes) != 2 {
		t.Errorf("Expected 2 route bindings, got %d", len(res.Routes))
	}

	// 10. Validate Component Node
	comp, err := parser.BuildComponentNode(res, "auth-service", core.CompService, lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode failed: %v", err)
	}
	if comp.Name != "auth-service" || comp.Language != core.LangRust {
		t.Errorf("Unexpected component: %+v", comp)
	}
	if len(comp.SymbolNodes) != len(res.AllSymbols) {
		t.Errorf("Component symbols count mismatch: %d vs %d", len(comp.SymbolNodes), len(res.AllSymbols))
	}

	// 11. Validate Hydrator Round-trip
	hydrator := NewRustHydrator()
	for _, sym := range res.AllSymbols {
		code, err := hydrator.HydrateSymbol(sym)
		if err != nil {
			t.Fatalf("Failed to hydrate symbol %s: %v", sym.Identifier, err)
		}
		if code == "" {
			t.Fatalf("Hydrated code is empty for %s", sym.Identifier)
		}
		switch sym.NodeType {
		case "StructDef":
			if !strings.Contains(code, "struct UserProfile") && !strings.Contains(code, "struct PgUserRepository") {
				t.Errorf("Hydrated struct missing name: %s", code)
			}
		case "EnumDef":
			if !strings.Contains(code, "enum UserStatus") {
				t.Errorf("Hydrated enum missing name: %s", code)
			}
		case "TraitDef":
			if !strings.Contains(code, "trait UserRepository") {
				t.Errorf("Hydrated trait missing name: %s", code)
			}
		case "FnDef":
			if !strings.Contains(code, "fn ") {
				t.Errorf("Hydrated fn missing fn keyword: %s", code)
			}
		}
	}
}
