package typescript

import (
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

const sampleTSX = `
import React, { useState, useEffect } from 'react';
import { UserCard } from './UserCard';
import { useAuth } from '../hooks/useAuth';

export interface UserProfile {
  id: string;
  name: string;
  email: string;
  role: 'admin' | 'user';
}

export type UserStatus = 'active' | 'suspended';

export const UserList: React.FC = () => {
  const [users, setUsers] = useState<UserProfile[]>([]);
  const { token } = useAuth();

  useEffect(() => {
    fetch('/api/v1/users', {
      headers: { Authorization: token }
    })
      .then(res => res.json())
      .then(data => setUsers(data));
  }, [token]);

  const handleLogin = async () => {
    await apiClient.post('/api/auth/login', { username: 'test' });
  };

  return (
    <div className="user-list">
      {users.map(u => (
        <UserCard key={u.id} user={u} />
      ))}
    </div>
  );
};
`

func TestTSParser_ParseSource(t *testing.T) {
	parser := NewTSParser()
	lineage := core.LineageEnvelope{
		UserID:     "user-1",
		UserPrompt: "Implement UserList UI component with API fetch",
		SessionID:  "session-ts-1",
		Timestamp:  time.Now().UTC(),
	}

	result, err := parser.ParseSource("UserList.tsx", []byte(sampleTSX), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	// 1. Verify Imports
	if len(result.Imports) != 3 {
		t.Fatalf("expected 3 imports, got %d", len(result.Imports))
	}

	// 2. Verify Interfaces and Types
	if len(result.Interfaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(result.Interfaces))
	}
	iface := result.Interfaces[0]
	if iface.Name != "UserProfile" {
		t.Errorf("expected interface UserProfile, got %s", iface.Name)
	}
	if iface.Properties["email"] == "" {
		t.Errorf("missing property email in UserProfile")
	}

	if len(result.Types) != 1 {
		t.Fatalf("expected 1 type alias, got %d", len(result.Types))
	}
	if result.Types[0].Name != "UserStatus" {
		t.Errorf("expected type UserStatus, got %s", result.Types[0].Name)
	}

	// 3. Verify Components
	if len(result.Components) != 1 {
		t.Fatalf("expected 1 component (UserList), got %d", len(result.Components))
	}
	comp := result.Components[0]
	if comp.Name != "UserList" {
		t.Errorf("expected component name UserList, got %s", comp.Name)
	}
	if len(comp.HooksUsed) == 0 {
		t.Errorf("expected hooks used to be extracted")
	}
	if len(comp.SubComponents) == 0 || comp.SubComponents[0] != "UserCard" {
		t.Errorf("expected subcomponent UserCard, got %v", comp.SubComponents)
	}

	// 4. Verify API Calls
	if len(result.APICalls) < 2 {
		t.Fatalf("expected at least 2 API calls, got %d", len(result.APICalls))
	}

	apiMap := make(map[string]TSAPICall)
	for _, api := range result.APICalls {
		apiMap[api.URL] = api
	}

	if _, ok := apiMap["/api/v1/users"]; !ok {
		t.Errorf("missing fetch API call to /api/v1/users")
	}
	if _, ok := apiMap["/api/auth/login"]; !ok {
		t.Errorf("missing apiClient.post call to /api/auth/login")
	}

	// 5. Verify AST Symbol Nodes
	if len(result.AllSymbols) == 0 {
		t.Fatalf("expected AST symbol nodes")
	}
	for _, sym := range result.AllSymbols {
		if sym.Language != core.LangTypeScript {
			t.Errorf("expected LangTypeScript, got %s", sym.Language)
		}
		if sym.NodeID == "" {
			t.Errorf("empty NodeID for %s", sym.Identifier)
		}
	}

	// 6. Build Component Node
	compNode, symNodes, err := parser.BuildComponentNode(result, "frontend-users", lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode failed: %v", err)
	}
	if compNode.Name != "frontend-users" {
		t.Errorf("expected component frontend-users, got %s", compNode.Name)
	}
	if compNode.Type != core.CompFrontend {
		t.Errorf("expected CompFrontend, got %s", compNode.Type)
	}
	if len(symNodes) != len(result.AllSymbols) {
		t.Errorf("expected %d symbol nodes, got %d", len(result.AllSymbols), len(symNodes))
	}
}
