package testagent

// AgentRoleSpec defines the instructions, component scope, and Cosm tools for an agent task.
type AgentRoleSpec struct {
	RoleID          string                      `json:"role_id"`
	Title           string                      `json:"title"`
	ComponentScope  string                      `json:"component_scope"`
	CosmToolsNeeded []string                    `json:"cosm_tools_needed"`
	Goal            string                      `json:"goal"`
	SystemPrompt    string                      `json:"system_prompt"`
	TaskPrompt      string                      `json:"task_prompt"`
	TargetContracts []CrossBoundaryContractSpec `json:"target_contracts"`
}

// ScenarioCloudRunMesh is the full-stack polyglot fintech-payment-mesh scenario.
var ScenarioCloudRunMesh = Scenario{
	Name:        "cloudrun-polyglot-mesh",
	Description: "Fintech Payment Mesh with Go Orders API, React TS Checkout UI, Python Fraud Worker, and Terraform Cloud Run Infra",
	Prompt:      "Bootstrap the fintech-payment-mesh repository on universe-main, stage polyglot components, link cross-boundary contracts, and publish the baseline Merkle root to Topocosm Hub.",
	ExpectedComponents: []string{
		"services/orders",
		"apps/checkout",
		"workers/fraud",
		"infra/cloudrun",
	},
	TargetContracts: []CrossBoundaryContractSpec{
		{
			SourceComponent: "apps/checkout",
			TargetComponent: "services/orders",
			ContractType:    "CONSUMES_API",
			Identifier:      "/api/v2/orders/checkout",
		},
		{
			SourceComponent: "services/orders",
			TargetComponent: "infra/cloudrun",
			ContractType:    "BINDS_ENV",
			Identifier:      "PAYMENT_TOPIC_ID",
		},
		{
			SourceComponent: "workers/fraud",
			TargetComponent: "infra/cloudrun",
			ContractType:    "CONSUMES_TOPIC",
			Identifier:      "payment-events",
		},
	},
	Files: map[string]string{
		"services/orders/main.go": `package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

type CheckoutRequest struct {
	OrderID     string  ` + "`json:\"order_id\"`" + `
	Amount      float64 ` + "`json:\"amount\"`" + `
	Currency    string  ` + "`json:\"currency\"`" + `
	Idempotency string  ` + "`json:\"idempotency_key\"`" + `
}

type CheckoutResponse struct {
	Status      string ` + "`json:\"status\"`" + `
	OrderID     string ` + "`json:\"order_id\"`" + `
	ReferenceID string ` + "`json:\"reference_id\"`" + `
}

func checkoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CheckoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp := CheckoutResponse{
		Status:      "CONFIRMED",
		OrderID:     req.OrderID,
		ReferenceID: fmt.Sprintf("ref-%s-%s", req.Currency, req.OrderID),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func main() {
	topicID := os.Getenv("PAYMENT_TOPIC_ID")
	_ = topicID

	http.HandleFunc("/api/v2/orders/checkout", checkoutHandler)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	_ = http.ListenAndServe(":"+port, nil)
}
`,
		"apps/checkout/src/CheckoutForm.tsx": `import React, { useState } from "react";

interface CheckoutProps {
  orderId: string;
  defaultAmount: number;
}

export const CheckoutForm: React.FC<CheckoutProps> = ({ orderId, defaultAmount }) => {
  const [currency, setCurrency] = useState<string>("USD");
  const [loading, setLoading] = useState<boolean>(false);
  const [confirmedRef, setConfirmedRef] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);

    try {
      const res = await fetch("/api/v2/orders/checkout", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Idempotency-Key": "idem-" + orderId,
        },
        body: JSON.stringify({
          order_id: orderId,
          amount: defaultAmount,
          currency: currency,
          idempotency_key: "idem-" + orderId,
        }),
      });

      const data = await res.json();
      if (res.ok) {
        setConfirmedRef(data.reference_id);
      }
    } catch (err) {
      console.error("Checkout failed:", err);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="checkout-panel">
      <h2>Complete Payment</h2>
      <form onSubmit={handleSubmit}>
        <label>Currency:</label>
        <select value={currency} onChange={(e) => setCurrency(e.target.value)}>
          <option value="USD">USD</option>
          <option value="EUR">EUR</option>
          <option value="GBP">GBP</option>
        </select>
        <button type="submit" disabled={loading}>
          {loading ? "Processing..." : "Pay Now"}
        </button>
      </form>
      {confirmedRef && <p className="success">Payment Confirmed: {confirmedRef}</p>}
    </div>
  );
};

export default CheckoutForm;
`,
		"workers/fraud/analyzer.py": `import os
import json
from typing import Dict, Any

class FraudScorer:
    def __init__(self, topic_id: str = None):
        self.topic_id = topic_id or os.environ.get("PAYMENT_TOPIC_ID", "payment-events")
        self.risk_threshold = 0.85

    def score_transaction(self, tx: Dict[str, Any]) -> Dict[str, Any]:
        amount = float(tx.get("amount", 0.0))
        currency = tx.get("currency", "USD")
        
        # Simple heuristic risk assessment
        risk_score = 0.1
        if amount > 5000.0:
            risk_score += 0.5
        if currency not in ["USD", "EUR", "GBP"]:
            risk_score += 0.3
            
        action = "APPROVE" if risk_score < self.risk_threshold else "FLAG_MANUAL_REVIEW"
        return {
            "order_id": tx.get("order_id"),
            "risk_score": risk_score,
            "decision": action,
            "topic": self.topic_id
        }

if __name__ == "__main__":
    scorer = FraudScorer()
    sample = {"order_id": "ord-1001", "amount": 1200.0, "currency": "USD"}
    print(scorer.score_transaction(sample))
`,
		"infra/cloudrun/main.tf": `terraform {
  required_version = ">= 1.5.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

variable "project_id" {
  type    = string
  default = "demo-mesh-project"
}

variable "region" {
  type    = string
  default = "us-central1"
}

resource "google_pubsub_topic" "payment_events" {
  name                       = "payment-events"
  message_retention_duration = "86600s"
}

resource "google_cloud_run_v2_service" "orders_api" {
  name     = "orders-api-service"
  location = var.region

  template {
    containers {
      image = "us-central1-docker.pkg.dev/demo-mesh-project/repo/orders-api:latest"
      env {
        name  = "PAYMENT_TOPIC_ID"
        value = google_pubsub_topic.payment_events.name
      }
      env {
        name  = "PORT"
        value = "8080"
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

// GetSwarmAgentRoles returns the specialized role blueprints for the multi-agent Cloud Run swarm.
func GetSwarmAgentRoles() []AgentRoleSpec {
	return []AgentRoleSpec{
		{
			RoleID:         "agent-zero-bootstrap",
			Title:          "Agent Zero: Bootstrap & Initializer",
			ComponentScope: "all",
			CosmToolsNeeded: []string{
				"cosm_init",
				"cosm_add",
				"cosm_commit",
				"cosm_publish",
			},
			Goal: "Initialize repository, stage polyglot files, compute Merkle root on universe-main, and publish baseline to Topocosm Hub.",
			SystemPrompt: "You are Agent Zero, the expert Bootstrap and Initializer for Cosm and Topocosm. " +
				"You create clean Merkle-DAG baselines with zero-copy micro-universes, discovery manifests, and content-addressed storage.",
			TaskPrompt: "Initialize repository at universe-main, stage all 4 polyglot components, commit with intent, and publish to the remote Topocosm Hub with IsInitial=true.",
		},
		{
			RoleID:         "agent-alpha-backend",
			Title:          "Agent Alpha: Backend API Lead",
			ComponentScope: "services/orders",
			CosmToolsNeeded: []string{
				"cosm_claim",
				"cosm_universe_create",
				"cosm_ast_edit",
				"cosm_commit",
				"cosm_publish",
				"cosm_proposal_create",
				"cosm_release",
			},
			Goal: "Acquire domain lease for services/orders, branch into u/agent-alpha-orders-v2, perform surgical AST edit, publish to Topocosm Hub, open proposal, and release lease.",
			SystemPrompt: "You are Agent Alpha, the Backend API Lead and Cosm expert. " +
				"Always acquire a Blackboard domain lease before mutating shared service contracts, and always commit inside an isolated micro-universe.",
			TaskPrompt: "Claim domain services/orders, create micro-universe u/agent-alpha-orders-v2, edit Checkout method signature with --write-disk, commit, publish to Topocosm Hub, and open Proposal #101.",
		},
		{
			RoleID:         "agent-beta-frontend",
			Title:          "Agent Beta: Frontend Checkout Specialist",
			ComponentScope: "apps/checkout",
			CosmToolsNeeded: []string{
				"cosm_clone_sparse",
				"cosm_universe_create",
				"cosm_ast_edit",
				"cosm_stack_create",
				"cosm_publish",
				"cosm_proposal_create",
			},
			Goal: "Perform sparse clone of apps/checkout, branch into u/agent-beta-frontend, perform AST surgery on CheckoutForm, stack proposal on Proposal #101, and publish to Hub.",
			SystemPrompt: "You are Agent Beta, the Frontend UI Specialist and Cosm expert. " +
				"Leverage Cosm sparse cloning to minimize network egress, and anchor downstream UI proposals using Jujutsu-style stacked proposals.",
			TaskPrompt: "Sparse clone apps/checkout to save bandwidth, create micro-universe u/agent-beta-frontend, update CheckoutForm to call /api/v2/orders/checkout, stack on backend proposal, publish to Hub, and open Proposal #102.",
		},
		{
			RoleID:         "agent-gamma-infra",
			Title:          "Agent Gamma: Cloud Infrastructure Engineer",
			ComponentScope: "infra/cloudrun",
			CosmToolsNeeded: []string{
				"cosm_clone_sparse",
				"cosm_universe_create",
				"cosm_commit",
				"cosm_publish",
				"cosm_proposal_create",
			},
			Goal: "Sparse clone infra/cloudrun, branch into u/agent-gamma-infra, update Terraform HCL with payment-events topic, format & validate Terraform, commit, publish, and open proposal.",
			SystemPrompt: "You are Agent Gamma, the Cloud Infrastructure Engineer and Cosm expert. " +
				"Ensure all Terraform files are formatted and validated, and maintain explicit cross-boundary BINDS_ENV contracts.",
			TaskPrompt: "Sparse clone infra/cloudrun, create micro-universe u/agent-gamma-infra, verify terraform formatting & validation, commit, publish to Topocosm Hub, and open Proposal #103.",
		},
		{
			RoleID:         "agent-delta-contender",
			Title:          "Agent Delta: Rogue Contender & Concurrency Stressor",
			ComponentScope: "services/orders",
			CosmToolsNeeded: []string{
				"cosm_claim",
				"cosm_publish_blobs",
				"cosm_universe_create",
				"cosm_proposal_create",
			},
			Goal: "Stress the Topocosm Hub concurrency engine: attempt to claim services/orders (expecting 409 Conflict), flood CAS with concurrent blobs, and submit an uncoordinated proposal to trigger ASTConflictNode reification.",
			SystemPrompt: "You are Agent Delta, the Concurrency Stressor and Collision Probe. " +
				"Your mission is to rigorously validate that Topocosm rejects unauthorized claims with HTTP 409, deduplicates concurrent CAS blobs with zero bit-rot, and reifies semantic merge collisions as ASTConflictNodes.",
			TaskPrompt: "Attempt to claim services/orders while held by Agent Alpha, execute rapid concurrent CAS blob writes, and attempt to merge an uncoordinated proposal to verify ASTConflictNode generation.",
		},
	}
}
