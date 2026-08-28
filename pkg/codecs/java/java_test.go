package java

import (
	"strings"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

func TestJavaParser_SpringBootAndJPA(t *testing.T) {
	javaSrc := `package com.example.userservice;

import org.springframework.web.bind.annotation.*;
import javax.persistence.*;
import java.util.List;
import java.util.Optional;

/**
 * User persistent entity mapped to database table
 */
@Entity
@Table(name = "users")
public class UserEntity {
    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(name = "email", nullable = false, unique = true)
    private String email;

    @Column(name = "full_name")
    private String fullName;

    public Long getId() {
        return this.id;
    }

    public void setId(Long id) {
        this.id = id;
    }

    public String getEmail() {
        return this.email;
    }
}

/**
 * User REST Controller handling HTTP API endpoints
 */
@RestController
@RequestMapping("/api/v1/users")
public class UserController {

    private final UserRepository userRepository;

    public UserController(UserRepository userRepository) {
        this.userRepository = userRepository;
    }

    @GetMapping("/{id}")
    public UserEntity getUserById(@PathVariable("id") Long id) {
        return userRepository.findById(id).orElse(null);
    }

    @PostMapping("")
    public UserEntity createUser(@RequestBody UserEntity user) {
        return userRepository.save(user);
    }

    @DeleteMapping("/{id}")
    public void deleteUser(@PathVariable("id") Long id) {
        userRepository.deleteById(id);
    }
}
`

	lineage := core.LineageEnvelope{
		UserID:           "java-developer",
		UserPrompt:       "Implement Spring Boot UserController and JPA UserEntity",
		ExecutingAgentID: "agent-java",
		Timestamp:        time.Now().UTC(),
	}

	parser := NewJavaParser()
	res, err := parser.ParseSource("src/main/java/com/example/userservice/UserController.java", []byte(javaSrc), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	// 1. Validate Package and Imports
	if res.PackageName != "com.example.userservice" {
		t.Errorf("Expected package com.example.userservice, got %s", res.PackageName)
	}
	if len(res.Imports) < 4 {
		t.Errorf("Expected at least 4 imports, got %d", len(res.Imports))
	}

	// 2. Validate Classes
	if len(res.Classes) != 2 {
		t.Fatalf("Expected 2 classes, got %d", len(res.Classes))
	}

	entityClass := res.Classes[0]
	if entityClass.Name != "UserEntity" {
		t.Errorf("Expected class UserEntity, got %s", entityClass.Name)
	}
	if !entityClass.IsEntity {
		t.Errorf("Expected entityClass.IsEntity to be true")
	}
	if entityClass.TableName != "users" {
		t.Errorf("Expected table name 'users', got '%s'", entityClass.TableName)
	}
	if len(entityClass.Fields) != 3 {
		t.Errorf("Expected 3 fields in UserEntity, got %d", len(entityClass.Fields))
	}

	controllerClass := res.Classes[1]
	if controllerClass.Name != "UserController" {
		t.Errorf("Expected class UserController, got %s", controllerClass.Name)
	}
	if !controllerClass.IsController {
		t.Errorf("Expected controllerClass.IsController to be true")
	}
	if controllerClass.BasePath != "/api/v1/users" {
		t.Errorf("Expected basePath /api/v1/users, got %s", controllerClass.BasePath)
	}
	if len(controllerClass.Methods) != 4 {
		t.Errorf("Expected 4 methods in UserController, got %d", len(controllerClass.Methods))
	}

	// 3. Validate Routes
	if len(res.Routes) != 3 {
		t.Fatalf("Expected 3 routes, got %d", len(res.Routes))
	}

	getRoute := res.Routes[0]
	if getRoute.Method != "GET" || getRoute.Path != "/api/v1/users/{id}" {
		t.Errorf("Expected GET /api/v1/users/{id}, got %s %s", getRoute.Method, getRoute.Path)
	}

	postRoute := res.Routes[1]
	if postRoute.Method != "POST" || postRoute.Path != "/api/v1/users" {
		t.Errorf("Expected POST /api/v1/users, got %s %s", postRoute.Method, postRoute.Path)
	}

	delRoute := res.Routes[2]
	if delRoute.Method != "DELETE" || delRoute.Path != "/api/v1/users/{id}" {
		t.Errorf("Expected DELETE /api/v1/users/{id}, got %s %s", delRoute.Method, delRoute.Path)
	}

	// 4. Validate Component Node
	comp, err := parser.BuildComponentNode(res, "user-service", core.CompService, lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode failed: %v", err)
	}
	if comp.Name != "user-service" || comp.Language != core.LangJava {
		t.Errorf("Unexpected component node: %+v", comp)
	}
	if len(comp.SymbolNodes) != len(res.AllSymbols) {
		t.Errorf("Component symbols mismatch: %d vs %d", len(comp.SymbolNodes), len(res.AllSymbols))
	}

	// 5. Validate Hydrator Round-trip
	hydrator := NewJavaHydrator()
	for _, sym := range res.AllSymbols {
		code, err := hydrator.HydrateSymbol(sym)
		if err != nil {
			t.Fatalf("Failed to hydrate Java symbol %s: %v", sym.Identifier, err)
		}
		if code == "" {
			t.Fatalf("Hydrated Java code is empty for %s", sym.Identifier)
		}

		if sym.NodeType == "ClassOrInterfaceDeclaration" {
			if !strings.Contains(code, "class UserEntity") && !strings.Contains(code, "class UserController") {
				t.Errorf("Hydrated class missing name: %s", code)
			}
		}
	}
}
