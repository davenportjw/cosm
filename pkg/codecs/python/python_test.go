package python

import (
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

func TestPythonParser_ParseFastAPIAndClasses(t *testing.T) {
	src := `import os
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

app = FastAPI()

class User(BaseModel):
    id: str
    name: str

@app.get("/api/v1/users")
async def get_users():
    db_url = os.environ.get("DATABASE_URL", "sqlite:///test.db")
    return [{"id": "1", "name": "Alice"}]

@app.post("/api/v1/users")
def create_user(user: User):
    return user
`
	lineage := core.LineageEnvelope{
		UserID:           "test-user",
		ExecutingAgentID: "agent-python",
		Intent:           "Create FastAPI server",
		Timestamp:        time.Now().UTC(),
	}

	parser := NewPythonParser()
	res, err := parser.ParseSource("main.py", []byte(src), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	if len(res.Classes) != 1 {
		t.Errorf("Expected 1 class, got %d", len(res.Classes))
	}
	if len(res.Routes) != 2 {
		t.Errorf("Expected 2 routes, got %d", len(res.Routes))
	}
	if len(res.Functions) != 2 {
		t.Errorf("Expected 2 functions, got %d", len(res.Functions))
	}
	if len(res.EnvVars) != 1 || res.EnvVars[0] != "DATABASE_URL" {
		t.Errorf("Expected env var DATABASE_URL, got %v", res.EnvVars)
	}

	comp, syms, err := parser.BuildComponentNode(res, "python-api", lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode failed: %v", err)
	}
	if comp.ComponentID == "" {
		t.Errorf("Expected non-empty ComponentID")
	}
	if len(syms) != 5 { // 1 class + 2 routes + 2 functions
		t.Errorf("Expected 5 total symbols, got %d", len(syms))
	}
}
