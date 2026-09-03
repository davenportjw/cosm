package swift

import (
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

func TestSwiftParser_FullLanguageEntities(t *testing.T) {
	src := `
import Foundation
import SwiftUI

/// User account model
public struct User: Codable, Identifiable {
    public let id: String
    public var name: String
    public var email: String
    private var token: String? = nil

    public func getDisplayName() -> String {
        return name
    }
}

/// Main application view
@MainActor
public struct ContentView: View {
    @State private var isPresented: Bool = false
    @ObservedObject var viewModel: UserViewModel

    var body: some View {
        VStack {
            Text("Welcome to Cosm")
                .font(.headline)
            Button("Fetch Users") {
                viewModel.fetchUsers()
            }
        }
    }
}

/// Service interface for users
public protocol UserServiceProtocol {
    func getUser(id: String) async throws -> User
    func deleteUser(id: String) async throws -> Bool
}

/// Concrete user service implementation
public class UserService: UserServiceProtocol {
    private let baseURL: String

    public init(baseURL: String) {
        self.baseURL = baseURL
    }

    public func getUser(id: String) async throws -> User {
        let url = URL(string: "/api/v1/users")!
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        let (data, _) = try await URLSession.shared.data(for: request)
        return try JSONDecoder().decode(User.self, from: data)
    }

    public func deleteUser(id: String) async throws -> Bool {
        let url = URL(string: "/api/v1/users/delete")!
        var request = URLRequest(url: url)
        request.httpMethod = "DELETE"
        let (_, response) = try await URLSession.shared.data(for: request)
        return true
    }
}

/// User role status
public enum UserStatus: String, Codable {
    case active = "ACTIVE"
    case pending = "PENDING"
    case suspended = "SUSPENDED"
}

/// Extension on User to add formatting helpers
extension User {
    public func formattedSummary() -> String {
        return "\(name) <\(email)>"
    }
}

/// Global helper function
public func calculateTax(amount: Double, rate: Double) -> Double {
    return amount * rate
}
`

	parser := NewSwiftParser()
	lineage := core.LineageEnvelope{
		UserID:     "user-swift-test",
		UserPrompt: "test swift parser",
		SessionID:  "sess-swift-1",
		Timestamp:  time.Now(),
	}

	result, err := parser.ParseSource("UserApp.swift", []byte(src), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	// 1. Verify Imports
	if len(result.Imports) != 2 {
		t.Errorf("expected 2 imports, got %d", len(result.Imports))
	}
	if result.Imports[0] != "Foundation" || result.Imports[1] != "SwiftUI" {
		t.Errorf("unexpected imports: %v", result.Imports)
	}

	// 2. Verify Types (User, ContentView, UserServiceProtocol, UserService, UserStatus, extension User)
	if len(result.Types) != 6 {
		t.Fatalf("expected 6 types, got %d", len(result.Types))
	}

	// User struct
	userStruct := result.Types[0]
	if userStruct.Name != "User" || userStruct.Kind != "struct" || userStruct.Visibility != "public" {
		t.Errorf("User struct mismatch: %+v", userStruct)
	}
	if len(userStruct.Properties) != 4 {
		t.Errorf("expected 4 properties on User, got %d", len(userStruct.Properties))
	}
	if len(userStruct.Methods) != 1 || userStruct.Methods[0].Name != "getDisplayName" {
		t.Errorf("expected getDisplayName method on User, got %+v", userStruct.Methods)
	}

	// ContentView SwiftUI View
	contentView := result.Types[1]
	if contentView.Name != "ContentView" || !contentView.IsSwiftUIView {
		t.Errorf("ContentView should be recognized as SwiftUI View: %+v", contentView)
	}
	if len(contentView.Attributes) == 0 || contentView.Attributes[0] != "@MainActor" {
		t.Errorf("ContentView should have @MainActor attribute: %+v", contentView.Attributes)
	}

	// UserServiceProtocol
	userProto := result.Types[2]
	if userProto.Name != "UserServiceProtocol" || userProto.Kind != "protocol" {
		t.Errorf("UserServiceProtocol mismatch: %+v", userProto)
	}

	// UserService Class
	userClass := result.Types[3]
	if userClass.Name != "UserService" || userClass.Kind != "class" {
		t.Errorf("UserService class mismatch: %+v", userClass)
	}
	if len(userClass.Methods) < 2 {
		t.Errorf("expected at least 2 methods on UserService, got %d", len(userClass.Methods))
	}

	// UserStatus Enum
	userStatus := result.Types[4]
	if userStatus.Name != "UserStatus" || userStatus.Kind != "enum" {
		t.Errorf("UserStatus enum mismatch: %+v", userStatus)
	}
	if len(userStatus.EnumCases) != 3 {
		t.Errorf("expected 3 enum cases on UserStatus, got %d", len(userStatus.EnumCases))
	}

	// User Extension
	userExt := result.Types[5]
	if userExt.Name != "User" || userExt.Kind != "extension" {
		t.Errorf("User extension mismatch: %+v", userExt)
	}
	if len(userExt.Methods) != 1 || userExt.Methods[0].Name != "formattedSummary" {
		t.Errorf("expected formattedSummary method on extension User, got %+v", userExt.Methods)
	}

	// 3. Verify Top-Level Functions
	if len(result.TopLevelFunctions) != 1 {
		t.Errorf("expected 1 top level function, got %d", len(result.TopLevelFunctions))
	} else if result.TopLevelFunctions[0].Name != "calculateTax" {
		t.Errorf("expected calculateTax function, got %s", result.TopLevelFunctions[0].Name)
	}

	// 4. Verify API Calls (URLSession -> CONSUMES_API)
	if len(result.APICalls) < 2 {
		t.Fatalf("expected at least 2 API calls extracted, got %d", len(result.APICalls))
	}
	foundGet := false
	foundDelete := false
	for _, call := range result.APICalls {
		if call.URL == "/api/v1/users" && call.Method == "GET" {
			foundGet = true
		}
		if call.URL == "/api/v1/users/delete" && call.Method == "DELETE" {
			foundDelete = true
		}
	}
	if !foundGet {
		t.Errorf("did not find GET /api/v1/users API call")
	}
	if !foundDelete {
		t.Errorf("did not find DELETE /api/v1/users/delete API call")
	}

	// 5. Verify ASTSymbolNodes
	if len(result.AllSymbols) == 0 {
		t.Fatalf("expected non-empty AllSymbols")
	}

	var hasViewNode, hasApiNode bool
	for _, sym := range result.AllSymbols {
		if sym.NodeType == "SwiftUIView" {
			hasViewNode = true
		}
		if sym.NodeType == "ApiClientCall" {
			hasApiNode = true
			if sym.ASTMetadata["consumes_api"] != "true" {
				t.Errorf("ApiClientCall missing consumes_api metadata")
			}
		}
	}
	if !hasViewNode {
		t.Errorf("expected SwiftUIView symbol node")
	}
	if !hasApiNode {
		t.Errorf("expected ApiClientCall symbol node")
	}

	// 6. Test BuildComponentNode
	comp, err := parser.BuildComponentNode(result, "ios-client", core.CompFrontend, lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode failed: %v", err)
	}
	if comp.ComponentID == "" {
		t.Errorf("ComponentID is empty")
	}
	if comp.Language != core.LangSwift {
		t.Errorf("Component language should be swift, got %s", comp.Language)
	}
}

func TestSwiftHydrator_RoundtripIsomorphism(t *testing.T) {
	src := `import Foundation
import SwiftUI

/// User profile card view
public struct ProfileView: View {
    public var username: String
    public var email: String

    public var body: some View {
        VStack {
            Text(username)
            Text(email)
        }
    }
}

public class NetworkClient {
    public func requestData() async throws -> String {
        let url = URL(string: "/api/v1/status")!
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        let (data, _) = try await URLSession.shared.data(for: request)
        return String(decoding: data, as: UTF8.self)
    }
}
`

	parser := NewSwiftParser()
	hydrator := NewSwiftHydrator()
	lineage := core.LineageEnvelope{
		UserID:    "roundtrip-user",
		SessionID: "sess-rt-1",
		Timestamp: time.Now(),
	}

	// 1. Initial Parse
	res1, err := parser.ParseSource("ProfileView.swift", []byte(src), lineage)
	if err != nil {
		t.Fatalf("Initial parse failed: %v", err)
	}

	// 2. Hydrate
	hydratedCode, err := hydrator.HydrateFile(res1)
	if err != nil {
		t.Fatalf("Hydration failed: %v", err)
	}

	if hydratedCode == "" {
		t.Fatalf("Hydrated code is empty")
	}

	// 3. Re-parse hydrated code
	res2, err := parser.ParseSource("ProfileView.swift", []byte(hydratedCode), lineage)
	if err != nil {
		t.Fatalf("Re-parse failed: %v\nHydrated code:\n%s", err, hydratedCode)
	}

	// 4. Verify AST isomorphism
	if len(res1.Imports) != len(res2.Imports) {
		t.Errorf("Imports count mismatch: %d vs %d", len(res1.Imports), len(res2.Imports))
	}

	if len(res1.Types) != len(res2.Types) {
		t.Fatalf("Types count mismatch: %d vs %d", len(res1.Types), len(res2.Types))
	}

	for i := range res1.Types {
		t1 := res1.Types[i]
		t2 := res2.Types[i]

		if t1.Name != t2.Name {
			t.Errorf("Type %d name mismatch: %s vs %s", i, t1.Name, t2.Name)
		}
		if t1.Kind != t2.Kind {
			t.Errorf("Type %d kind mismatch: %s vs %s", i, t1.Kind, t2.Kind)
		}
		if t1.IsSwiftUIView != t2.IsSwiftUIView {
			t.Errorf("Type %d IsSwiftUIView mismatch: %v vs %v", i, t1.IsSwiftUIView, t2.IsSwiftUIView)
		}
	}
}
