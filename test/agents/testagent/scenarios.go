package testagent

import (
	"fmt"
	"os"
	"path/filepath"
)

// CrossBoundaryContractSpec describes an anticipated cross-language API route, env var, or DB query binding.
type CrossBoundaryContractSpec struct {
	SourceComponent string `json:"source_component"`
	TargetComponent string `json:"target_component"`
	ContractType    string `json:"contract_type"` // "API_ROUTE", "BINDS_ENV", "SQL_TABLE", "DEPENDS_ON"
	Identifier      string `json:"identifier"`    // e.g. "/api/v1/users" or "DATABASE_URL"
}

// Scenario encapsulates a polyglot test application blueprint and prompt instructions.
type Scenario struct {
	Name               string                      `json:"name"`
	Description        string                      `json:"description"`
	Prompt             string                      `json:"prompt"`
	Files              map[string]string           `json:"files"`
	ExpectedComponents []string                    `json:"expected_components"`
	TargetContracts    []CrossBoundaryContractSpec `json:"target_contracts"`
}

// ScenarioFastAPIReactTF is a full-stack Python FastAPI + TypeScript React + Terraform AWS/GCP template.
var ScenarioFastAPIReactTF = Scenario{
	Name:               "fastapi-react-tf",
	Description:        "Python FastAPI Backend, React TypeScript Frontend, and Terraform Cloud Infrastructure",
	Prompt:             "Initialize cosm repository, stage backend, frontend, and infra components, link cross-boundary contracts, and produce a committed Merkle root.",
	ExpectedComponents: []string{"backend", "frontend", "infra"},
	TargetContracts: []CrossBoundaryContractSpec{
		{
			SourceComponent: "frontend",
			TargetComponent: "backend",
			ContractType:    "CONSUMES_API",
			Identifier:      "/api/v1/users",
		},
		{
			SourceComponent: "backend",
			TargetComponent: "infra",
			ContractType:    "BINDS_ENV",
			Identifier:      "DATABASE_URL",
		},
	},
	Files: map[string]string{
		"backend/main.py": `from fastapi import FastAPI, HTTPException
import os

app = FastAPI(title="User Management Service", version="1.0.0")

database_url = os.environ.get("DATABASE_URL", "sqlite:///./test.db")
auth_token_secret = os.getenv("AUTH_SECRET", "default-dev-secret")

@app.get("/api/v1/users")
def list_users():
    return [{"id": 1, "username": "alice", "role": "admin"}, {"id": 2, "username": "bob", "role": "user"}]

@app.post("/api/v1/users")
def create_user(username: str, role: str = "user"):
    return {"id": 3, "username": username, "role": role, "status": "created"}

@app.get("/healthz")
def health_check():
    return {"status": "healthy", "service": "user-backend"}
`,
		"frontend/src/App.tsx": `import React, { useState, useEffect } from "react";

interface User {
  id: number;
  username: string;
  role: string;
}

export const UserList: React.FC = () => {
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState<boolean>(true);

  useEffect(() => {
    fetch("/api/v1/users")
      .then((res) => res.json())
      .then((data: User[]) => {
        setUsers(data);
        setLoading(false);
      })
      .catch((err) => {
        console.error("Failed fetching users", err);
        setLoading(false);
      });
  }, []);

  if (loading) return <div>Loading users...</div>;

  return (
    <div className="user-container">
      <h1>Active Users</h1>
      <ul>
        {users.map((u) => (
          <li key={u.id}>{u.username} ({u.role})</li>
        ))}
      </ul>
    </div>
  );
};

export default UserList;
`,
		"infra/main.tf": `terraform {
  required_version = ">= 1.5.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

provider "google" {
  project = "fg-polyglot-test"
  region  = "us-central1"
}

resource "google_cloud_run_v2_service" "api_backend" {
  name     = "user-backend-service"
  location = "us-central1"

  template {
    containers {
      image = "gcr.io/fg-polyglot-test/backend:latest"
      env {
        name  = "DATABASE_URL"
        value = "postgresql://app:secret@10.0.0.5:5432/userdb"
      }
      env {
        name  = "AUTH_SECRET"
        value = "production-secret-token"
      }
      ports {
        container_port = 8000
      }
    }
  }
}

resource "google_cloud_run_service_iam_member" "invoker" {
  location = google_cloud_run_v2_service.api_backend.location
  service  = google_cloud_run_v2_service.api_backend.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}
`,
	},
}

// ScenarioGoGinVueTF is a full-stack Go Gin + Vue 3 TypeScript + Terraform template.
var ScenarioGoGinVueTF = Scenario{
	Name:               "go-gin-vue-tf",
	Description:        "Go Gin REST Backend, Vue 3 TypeScript Frontend, and Terraform Cloud Run Infra",
	Prompt:             "Initialize repository, ingest Go backend and Vue frontend, establish route linkage and commit universe head.",
	ExpectedComponents: []string{"backend", "frontend", "infra"},
	TargetContracts: []CrossBoundaryContractSpec{
		{
			SourceComponent: "frontend",
			TargetComponent: "backend",
			ContractType:    "CONSUMES_API",
			Identifier:      "/api/v1/items",
		},
		{
			SourceComponent: "backend",
			TargetComponent: "infra",
			ContractType:    "BINDS_ENV",
			Identifier:      "REDIS_ADDR",
		},
	},
	Files: map[string]string{
		"backend/main.go": `package main

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

type Item struct {
	ID    string  ` + "`json:\"id\"`" + `
	Name  string  ` + "`json:\"name\"`" + `
	Price float64 ` + "`json:\"price\"`" + `
}

func main() {
	r := gin.Default()
	redisAddr := os.Getenv("REDIS_ADDR")
	_ = redisAddr

	r.GET("/api/v1/items", func(c *gin.Context) {
		items := []Item{
			{ID: "itm-1", Name: "Mechanical Keyboard", Price: 129.99},
			{ID: "itm-2", Name: "UltraWide Monitor", Price: 499.50},
		}
		c.JSON(http.StatusOK, items)
	})

	r.POST("/api/v1/items", func(c *gin.Context) {
		var newItem Item
		if err := c.ShouldBindJSON(&newItem); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, newItem)
	})

	_ = r.Run(":8080")
}
`,
		"frontend/src/App.vue": `<template>
  <div class="inventory-view">
    <h2>Warehouse Inventory</h2>
    <div v-if="loading">Loading items...</div>
    <ul v-else>
      <li v-for="item in items" :key="item.id">
        {{ item.name }} - ${{ item.price }}
      </li>
    </ul>
  </div>
</template>

<script lang="ts">
import { defineComponent, ref, onMounted } from 'vue';

interface Item {
  id: string;
  name: string;
  price: number;
}

export default defineComponent({
  name: 'InventoryList',
  setup() {
    const items = ref<Item[]>([]);
    const loading = ref(true);

    onMounted(async () => {
      try {
        const res = await fetch('/api/v1/items');
        items.value = await res.json();
      } finally {
        loading.value = false;
      }
    });

    return { items, loading };
  }
});
</script>
`,
		"infra/main.tf": `terraform {
  required_version = ">= 1.5.0"
}

resource "google_cloud_run_v2_service" "inventory_service" {
  name     = "inventory-api"
  location = "us-central1"

  template {
    containers {
      image = "gcr.io/fg-inventory/backend:v1"
      env {
        name  = "REDIS_ADDR"
        value = "redis.internal.vpc:6379"
      }
      ports {
        container_port = 8080
      }
    }
  }
}
`,
	},
}

// ScenarioRustAxumReactPostgresTF is a multi-tier Rust Axum + React + SQL Postgres + Terraform template.
var ScenarioRustAxumReactPostgresTF = Scenario{
	Name:               "rust-axum-react-postgres-tf",
	Description:        "Rust Axum Backend, React TypeScript Frontend, SQL DDL Schema, and Terraform Infrastructure",
	Prompt:             "Initialize repository, parse Rust Axum endpoints, link SQL table schema and React frontend, and build target.",
	ExpectedComponents: []string{"backend", "frontend", "db", "infra"},
	TargetContracts: []CrossBoundaryContractSpec{
		{
			SourceComponent: "frontend",
			TargetComponent: "backend",
			ContractType:    "CONSUMES_API",
			Identifier:      "/api/v1/orders",
		},
		{
			SourceComponent: "backend",
			TargetComponent: "db",
			ContractType:    "DEPENDS_ON",
			Identifier:      "orders_table",
		},
		{
			SourceComponent: "backend",
			TargetComponent: "infra",
			ContractType:    "BINDS_ENV",
			Identifier:      "DATABASE_URL",
		},
	},
	Files: map[string]string{
		"backend/src/main.rs": `use axum::{routing::{get, post}, Json, Router};
use serde::{Deserialize, Serialize};
use std::net::SocketAddr;

#[derive(Serialize, Deserialize)]
pub struct Order {
    pub id: i32,
    pub user_id: String,
    pub amount: f64,
    pub status: String,
}

pub async fn list_orders() -> Json<Vec<Order>> {
    let orders = vec![
        Order { id: 101, user_id: "usr_42".into(), amount: 99.95, status: "PAID".into() },
        Order { id: 102, user_id: "usr_88".into(), amount: 249.00, status: "PENDING".into() },
    ];
    Json(orders)
}

pub async fn create_order(Json(payload): Json<Order>) -> Json<Order> {
    Json(payload)
}

pub fn build_app() -> Router {
    let _db_url = std::env::var("DATABASE_URL").unwrap_or_else(|_| "postgres://localhost/orders".into());
    Router::new()
        .route("/api/v1/orders", get(list_orders).post(create_order))
}

#[tokio::main]
async fn main() {
    let app = build_app();
    let addr = SocketAddr::from(([0, 0, 0, 0], 3000));
    axum::Server::bind(&addr).serve(app.into_make_service()).await.unwrap();
}
`,
		"db/schema.sql": `-- SQL DDL Schema for Orders
CREATE TABLE IF NOT EXISTS orders (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL,
    amount NUMERIC(10, 2) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_orders_user_id ON orders(user_id);
`,
		"frontend/src/App.tsx": `import React, { useEffect, useState } from "react";

interface Order {
  id: number;
  user_id: string;
  amount: number;
  status: string;
}

export const OrderDashboard: React.FC = () => {
  const [orders, setOrders] = useState<Order[]>([]);

  useEffect(() => {
    fetch("/api/v1/orders")
      .then((res) => res.json())
      .then((data: Order[]) => setOrders(data))
      .catch((err) => console.error("Error loading orders", err));
  }, []);

  return (
    <div className="orders-container">
      <h2>Active Orders</h2>
      <table>
        <thead>
          <tr><th>ID</th><th>User</th><th>Amount</th><th>Status</th></tr>
        </thead>
        <tbody>
          {orders.map((o) => (
            <tr key={o.id}>
              <td>{o.id}</td><td>{o.user_id}</td><td>${o.amount.toFixed(2)}</td><td>{o.status}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};

export default OrderDashboard;
`,
		"infra/main.tf": `terraform {
  required_version = ">= 1.5.0"
}

resource "google_cloud_run_v2_service" "orders_backend" {
  name     = "orders-backend"
  location = "us-central1"

  template {
    containers {
      image = "gcr.io/fg-orders/rust-backend:v1"
      env {
        name  = "DATABASE_URL"
        value = "postgresql://dbuser:pass@10.0.0.8:5432/orders"
      }
      ports {
        container_port = 3000
      }
    }
  }
}
`,
	},
}

// ScenarioSwiftGoTF tests Swift iOS client, Go REST API backend, and Terraform Cloud Run infrastructure.
var ScenarioSwiftGoTF = Scenario{
	Name:               "swift-go-tf",
	Description:        "Swift iOS Client, Go REST API Backend, and Google Cloud Run Terraform Infrastructure",
	Prompt:             "Initialize cosm repository, stage Swift mobile app, Go HTTP backend, and Terraform Cloud Run infrastructure, verify cross-boundary API contracts, and produce a committed Merkle root.",
	ExpectedComponents: []string{"mobile", "backend", "infra"},
	TargetContracts: []CrossBoundaryContractSpec{
		{
			SourceComponent: "mobile",
			TargetComponent: "backend",
			ContractType:    "CONSUMES_API",
			Identifier:      "/api/v1/orders",
		},
		{
			SourceComponent: "backend",
			TargetComponent: "infra",
			ContractType:    "BINDS_ENV",
			Identifier:      "DATABASE_URL",
		},
	},
	Files: map[string]string{
		"mobile/ApiClient.swift": `import Foundation

public struct Order: Codable {
    public let id: String
    public let total: Double
}

public class OrderApiClient {
    private let baseURL = "http://localhost:8080"

    public func fetchOrders(completion: @escaping (Result<[Order], Error>) -> Void) {
        guard let url = URL(string: "\(baseURL)/api/v1/orders") else { return }
        URLSession.shared.dataTask(with: url) { data, response, error in
            if let error = error {
                completion(.failure(error))
                return
            }
            if let data = data, let orders = try? JSONDecoder().decode([Order].self, from: data) {
                completion(.success(orders))
            }
        }.resume()
    }
}
`,
		"backend/main.go": `package main

import (
	"encoding/json"
	"net/http"
	"os"
)

type Order struct {
	ID    string  ` + "`" + `json:"id"` + "`" + `
	Total float64 ` + "`" + `json:"total"` + "`" + `
}

func main() {
	_ = os.Getenv("DATABASE_URL")
	http.HandleFunc("/api/v1/orders", func(w http.ResponseWriter, r *http.Request) {
		orders := []Order{{ID: "ord-101", Total: 89.50}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(orders)
	})
	_ = http.ListenAndServe(":8080", nil)
}
`,
		"infra/main.tf": `resource "google_cloud_run_service" "api_backend" {
  name     = "orders-api"
  location = "us-central1"

  template {
    spec {
      containers {
        image = "gcr.io/demo-project/orders-api:latest"
        env {
          name  = "DATABASE_URL"
          value = "postgres://orders_user:secret@10.0.0.5:5432/orders_db"
        }
      }
    }
  }
}
`,
	},
}

// ScenarioKotlinPySQL tests Kotlin Ktor microservice, Python analytics processor, and PostgreSQL DDL.
var ScenarioKotlinPySQL = Scenario{
	Name:               "kotlin-py-sql",
	Description:        "Kotlin Ktor Microservice, Python Analytics Worker, and PostgreSQL DDL Schema",
	Prompt:             "Initialize cosm repository, stage Kotlin microservice, Python analytics processor, and PostgreSQL database schema, verify table query and route contracts, and commit to universe-main.",
	ExpectedComponents: []string{"service", "analytics", "db"},
	TargetContracts: []CrossBoundaryContractSpec{
		{
			SourceComponent: "service",
			TargetComponent: "db",
			ContractType:    "QUERIES_TABLE",
			Identifier:      "users",
		},
		{
			SourceComponent: "analytics",
			TargetComponent: "service",
			ContractType:    "CONSUMES_API",
			Identifier:      "/api/v1/metrics",
		},
	},
	Files: map[string]string{
		"service/Application.kt": `package com.cosm.service

data class User(val id: String, val email: String)

class UserService {
    fun queryUsers(): List<User> {
        val sql = "SELECT id, email FROM users WHERE active = true"
        return listOf(User("u1", "alice@example.com"))
    }
}
`,
		"analytics/worker.py": `import urllib.request
import json

def pull_metrics():
    url = "http://localhost:8080/api/v1/metrics"
    req = urllib.request.Request(url)
    with urllib.request.urlopen(req) as resp:
        data = json.loads(resp.read().decode())
        return data
`,
		"db/schema.sql": `CREATE TABLE users (
    id VARCHAR(64) PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
`,
	},
}

// ScenarioCSharpReactProto tests C# ASP.NET Core gRPC service, React TypeScript frontend, and Proto3 IDL.
var ScenarioCSharpReactProto = Scenario{
	Name:               "csharp-react-proto",
	Description:        "C# ASP.NET Core gRPC Backend, React TypeScript Frontend, and Proto3 IDL Contracts",
	Prompt:             "Initialize cosm repository, stage C# gRPC service, React TypeScript frontend, and Proto3 contract definitions, infer cross-boundary RPC and API dependencies, and produce an attested commit.",
	ExpectedComponents: []string{"backend", "frontend", "proto"},
	TargetContracts: []CrossBoundaryContractSpec{
		{
			SourceComponent: "backend",
			TargetComponent: "proto",
			ContractType:    "IMPLEMENTS_RPC",
			Identifier:      "InventoryService",
		},
		{
			SourceComponent: "frontend",
			TargetComponent: "backend",
			ContractType:    "CONSUMES_API",
			Identifier:      "/api/v1/inventory",
		},
	},
	Files: map[string]string{
		"backend/InventoryServer.cs": `using System;
using System.Threading.Tasks;

namespace Cosm.Backend {
    public class InventoryServer : InventoryService.InventoryServiceBase {
        public Task<InventoryResponse> GetInventory(InventoryRequest request) {
            return Task.FromResult(new InventoryResponse { ItemCount = 42 });
        }
    }
}
`,
		"frontend/InventoryView.tsx": `import React, { useEffect, useState } from "react";

export const InventoryView: React.FC = () => {
  const [items, setItems] = useState<any[]>([]);

  useEffect(() => {
    fetch("/api/v1/inventory")
      .then((res) => res.json())
      .then((data) => setItems(data));
  }, []);

  return (
    <div className="inventory-container">
      <h2>Inventory Dashboard</h2>
      <p>Items in stock: {items.length}</p>
    </div>
  );
};
`,
		"proto/inventory.proto": `syntax = "proto3";

package inventory.v1;

message InventoryRequest {
    string category = 1;
}

message InventoryResponse {
    int32 item_count = 1;
}

service InventoryService {
    rpc GetInventory (InventoryRequest) returns (InventoryResponse);
}
`,
	},
}

var allScenarios = map[string]Scenario{
	"fastapi-react-tf":            ScenarioFastAPIReactTF,
	"go-gin-vue-tf":               ScenarioGoGinVueTF,
	"rust-axum-react-postgres-tf": ScenarioRustAxumReactPostgresTF,
	"swift-go-tf":                 ScenarioSwiftGoTF,
	"kotlin-py-sql":               ScenarioKotlinPySQL,
	"csharp-react-proto":          ScenarioCSharpReactProto,
	"cloudrun-polyglot-mesh":      ScenarioCloudRunMesh,
}

// ListScenarios returns all preconfigured polyglot scenario templates.
func ListScenarios() []Scenario {
	return []Scenario{
		ScenarioFastAPIReactTF,
		ScenarioGoGinVueTF,
		ScenarioRustAxumReactPostgresTF,
		ScenarioSwiftGoTF,
		ScenarioKotlinPySQL,
		ScenarioCSharpReactProto,
		ScenarioCloudRunMesh,
	}
}

// GetScenario returns a scenario by name.
func GetScenario(name string) (*Scenario, error) {
	if s, ok := allScenarios[name]; ok {
		return &s, nil
	}
	return nil, fmt.Errorf("unknown scenario name: %q (available: fastapi-react-tf, go-gin-vue-tf, rust-axum-react-postgres-tf, swift-go-tf, kotlin-py-sql, csharp-react-proto, cloudrun-polyglot-mesh)", name)
}

// WriteScenarioFiles writes all scenario blueprint files into a specified target directory.
func WriteScenarioFiles(workDir string, s *Scenario) error {
	if s == nil {
		return fmt.Errorf("scenario cannot be nil")
	}
	for relPath, content := range s.Files {
		fullPath := filepath.Join(workDir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return fmt.Errorf("failed creating directory for %s: %w", relPath, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed writing file %s: %w", relPath, err)
		}
	}
	return nil
}
