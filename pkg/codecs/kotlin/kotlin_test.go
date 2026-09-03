package kotlin

import (
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

func TestKotlinParser_FullLanguageEntities(t *testing.T) {
	src := `
package com.example.service

import io.ktor.client.*
import io.ktor.client.request.*
import org.springframework.web.bind.annotation.*
import androidx.compose.runtime.*

/** User entity data transfer object */
data class UserDto(
    val id: String,
    var username: String,
    val email: String,
    val isActive: Boolean = true
)

/** User repository contract */
interface UserRepository {
    suspend fun findById(id: String): UserDto?
    suspend fun save(user: UserDto): Boolean
}

/** Spring Boot REST Controller */
@RestController
@RequestMapping("/api/v1/users")
class UserController(
    private val repo: UserRepository
) {
    @GetMapping("/{id}")
    suspend fun getUserById(@PathVariable id: String): UserDto? {
        return repo.findById(id)
    }

    @PostMapping
    suspend fun createUser(@RequestBody user: UserDto): Boolean {
        return repo.save(user)
    }
}

/** Android Jetpack Compose UI component */
@Composable
fun UserProfileCard(user: UserDto, onEdit: () -> Unit) {
    // Composable UI rendering
}

/** Retrofit API client interface */
interface UserApiClient {
    @GET("/api/v1/remote-users")
    suspend fun fetchRemoteUsers(): List<UserDto>
}

/** Top-level helper function */
fun formatUserBadge(user: UserDto): String {
    return "[${user.id}] ${user.username}"
}
`

	parser := NewKotlinParser()
	lineage := core.LineageEnvelope{
		UserID:     "user-kt-test",
		UserPrompt: "test kotlin parser",
		SessionID:  "sess-kt-1",
		Timestamp:  time.Now(),
	}

	result, err := parser.ParseSource("UserService.kt", []byte(src), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	// 1. Verify Package and Imports
	if result.PackageName != "com.example.service" {
		t.Errorf("expected package com.example.service, got %s", result.PackageName)
	}
	if len(result.Imports) != 4 {
		t.Errorf("expected 4 imports, got %d", len(result.Imports))
	}

	// 2. Verify Classes (UserDto, UserRepository, UserController, UserApiClient)
	if len(result.Classes) != 4 {
		t.Fatalf("expected 4 classes, got %d", len(result.Classes))
	}

	// UserDto
	userDto := result.Classes[0]
	if userDto.Name != "UserDto" || userDto.Kind != "data class" {
		t.Errorf("UserDto mismatch: %+v", userDto)
	}
	if len(userDto.PrimaryConstructorParams) != 4 {
		t.Errorf("expected 4 primary constructor params on UserDto, got %d", len(userDto.PrimaryConstructorParams))
	}

	// UserRepository
	userRepo := result.Classes[1]
	if userRepo.Name != "UserRepository" || userRepo.Kind != "interface" {
		t.Errorf("UserRepository mismatch: %+v", userRepo)
	}
	if len(userRepo.Functions) != 2 {
		t.Errorf("expected 2 functions on UserRepository, got %d", len(userRepo.Functions))
	}

	// UserController
	userCtrl := result.Classes[2]
	if userCtrl.Name != "UserController" || !userCtrl.IsController {
		t.Errorf("UserController mismatch: %+v", userCtrl)
	}
	if userCtrl.BasePath != "/api/v1/users" {
		t.Errorf("expected BasePath /api/v1/users, got %s", userCtrl.BasePath)
	}
	if len(userCtrl.Functions) != 2 {
		t.Errorf("expected 2 functions on UserController, got %d", len(userCtrl.Functions))
	}

	// 3. Verify Top-Level Functions (UserProfileCard composable, formatUserBadge)
	if len(result.TopLevelFunctions) != 2 {
		t.Fatalf("expected 2 top-level functions, got %d", len(result.TopLevelFunctions))
	}
	cardFn := result.TopLevelFunctions[0]
	if cardFn.Name != "UserProfileCard" || !cardFn.IsComposable {
		t.Errorf("UserProfileCard should be composable: %+v", cardFn)
	}

	badgeFn := result.TopLevelFunctions[1]
	if badgeFn.Name != "formatUserBadge" {
		t.Errorf("expected formatUserBadge function, got %s", badgeFn.Name)
	}

	// 4. Verify Routes (Spring Boot)
	if len(result.Routes) != 2 {
		t.Fatalf("expected 2 Spring Boot routes, got %d", len(result.Routes))
	}
	if result.Routes[0].Method != "GET" || result.Routes[0].Path != "/api/v1/users/{id}" {
		t.Errorf("Route 0 mismatch: %+v", result.Routes[0])
	}
	if result.Routes[1].Method != "POST" || result.Routes[1].Path != "/api/v1/users" {
		t.Errorf("Route 1 mismatch: %+v", result.Routes[1])
	}

	// 5. Verify API Calls (Retrofit @GET)
	if len(result.APICalls) != 1 {
		t.Fatalf("expected 1 API call, got %d", len(result.APICalls))
	}
	if result.APICalls[0].URL != "/api/v1/remote-users" || result.APICalls[0].Method != "GET" {
		t.Errorf("API call mismatch: %+v", result.APICalls[0])
	}

	// 6. Verify ASTSymbolNodes
	if len(result.AllSymbols) == 0 {
		t.Fatalf("expected non-empty AllSymbols")
	}

	var hasComposableNode, hasRouteNode, hasApiNode bool
	for _, sym := range result.AllSymbols {
		if sym.NodeType == "ComposableFunction" {
			hasComposableNode = true
		}
		if sym.NodeType == "RouteBinding" {
			hasRouteNode = true
		}
		if sym.NodeType == "ApiClientCall" {
			hasApiNode = true
		}
	}
	if !hasComposableNode {
		t.Errorf("expected ComposableFunction symbol node")
	}
	if !hasRouteNode {
		t.Errorf("expected RouteBinding symbol node")
	}
	if !hasApiNode {
		t.Errorf("expected ApiClientCall symbol node")
	}

	// 7. Test BuildComponentNode
	comp, err := parser.BuildComponentNode(result, "backend-service", core.CompService, lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode failed: %v", err)
	}
	if comp.ComponentID == "" {
		t.Errorf("ComponentID is empty")
	}
	if comp.Language != core.LangKotlin {
		t.Errorf("Component language should be kotlin, got %s", comp.Language)
	}
}

func TestKotlinHydrator_RoundtripIsomorphism(t *testing.T) {
	src := `package com.example.models

import io.ktor.client.*

/** Item entity */
data class Item(
    val id: String,
    val name: String,
    val price: Double
)

class OrderService {
    suspend fun fetchOrders(): List<Item> {
        val client = HttpClient()
        val response = client.get("/api/v1/orders")
        return emptyList()
    }
}
`

	parser := NewKotlinParser()
	hydrator := NewKotlinHydrator()
	lineage := core.LineageEnvelope{
		UserID:    "kt-roundtrip-user",
		SessionID: "sess-kt-rt-1",
		Timestamp: time.Now(),
	}

	// 1. Initial Parse
	res1, err := parser.ParseSource("OrderService.kt", []byte(src), lineage)
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

	// 3. Re-parse
	res2, err := parser.ParseSource("OrderService.kt", []byte(hydratedCode), lineage)
	if err != nil {
		t.Fatalf("Re-parse failed: %v\nHydrated code:\n%s", err, hydratedCode)
	}

	// 4. Verify AST isomorphism
	if res1.PackageName != res2.PackageName {
		t.Errorf("Package mismatch: %s vs %s", res1.PackageName, res2.PackageName)
	}

	if len(res1.Classes) != len(res2.Classes) {
		t.Fatalf("Classes count mismatch: %d vs %d", len(res1.Classes), len(res2.Classes))
	}

	for i := range res1.Classes {
		c1 := res1.Classes[i]
		c2 := res2.Classes[i]

		if c1.Name != c2.Name {
			t.Errorf("Class %d name mismatch: %s vs %s", i, c1.Name, c2.Name)
		}
		if c1.Kind != c2.Kind {
			t.Errorf("Class %d kind mismatch: %s vs %s", i, c1.Kind, c2.Kind)
		}
	}
}
