package cpp

import (
	"strings"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

func TestCppParser_FullProgram(t *testing.T) {
	cppSrc := `#include <iostream>
#include <string>
#include <vector>
#include <memory>

// Status code enumeration
enum class StatusCode {
    OK = 200,
    NOT_FOUND = 404,
    INTERNAL_ERROR = 500,
};

// Base entity interface
class BaseEntity {
public:
    virtual ~BaseEntity() = default;
    virtual std::string GetId() const = 0;
};

// User entity model
class User : public BaseEntity {
private:
    std::string id_;
    std::string email_;
    int age_;

public:
    User(std::string id, std::string email) : id_(id), email_(email), age_(0) {}

    std::string GetId() const override {
        return id_;
    }

    std::string GetEmail() const {
        return email_;
    }

    void SetEmail(const std::string& email) {
        email_ = email;
    }
};

// Generic repository template
template<typename T>
class Repository {
public:
    virtual ~Repository() = default;
    virtual std::shared_ptr<T> FindById(const std::string& id) = 0;
    virtual void Save(const T& item) = 0;
};

// Global helper function
template<typename T>
void LogEntity(const T& entity) {
    std::cout << entity.GetId() << std::endl;
}
`

	lineage := core.LineageEnvelope{
		UserID:           "cpp-dev",
		UserPrompt:       "Implement C++ user domain entity and repository template",
		ExecutingAgentID: "agent-cpp",
		Timestamp:        time.Now().UTC(),
	}

	parser := NewCppParser()
	res, err := parser.ParseSource("src/user.hpp", []byte(cppSrc), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	// 1. Validate Includes
	if len(res.Includes) != 4 {
		t.Errorf("Expected 4 includes, got %d", len(res.Includes))
	}

	// 2. Validate Enums
	if len(res.Enums) != 1 {
		t.Fatalf("Expected 1 enum, got %d", len(res.Enums))
	}
	stEnum := res.Enums[0]
	if stEnum.Name != "StatusCode" || !stEnum.IsScoped || len(stEnum.Values) != 3 {
		t.Errorf("Unexpected enum: %+v", stEnum)
	}

	// 3. Validate Classes
	if len(res.Classes) < 3 {
		t.Fatalf("Expected at least 3 classes, got %d", len(res.Classes))
	}

	userClass := res.Classes[1]
	if userClass.Name != "User" {
		t.Errorf("Expected class User, got %s", userClass.Name)
	}
	if len(userClass.BaseClasses) != 1 || userClass.BaseClasses[0] != "public BaseEntity" {
		t.Errorf("Expected base class 'public BaseEntity', got %v", userClass.BaseClasses)
	}
	if len(userClass.Fields) != 3 {
		t.Errorf("Expected 3 fields in User, got %d", len(userClass.Fields))
	}
	if len(userClass.Methods) < 4 {
		t.Errorf("Expected at least 4 methods in User, got %d", len(userClass.Methods))
	}

	// 4. Validate Top-level Functions
	if len(res.Functions) < 1 {
		t.Fatalf("Expected at least 1 top-level function, got %d", len(res.Functions))
	}
	logFn := res.Functions[0]
	if logFn.Name != "LogEntity" {
		t.Errorf("Expected function LogEntity, got %s", logFn.Name)
	}

	// 5. Validate Component Node
	comp, err := parser.BuildComponentNode(res, "cpp-core", core.CompLibrary, lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode failed: %v", err)
	}
	if comp.Name != "cpp-core" || comp.Language != core.LangCpp {
		t.Errorf("Unexpected component: %+v", comp)
	}
	if len(comp.SymbolNodes) != len(res.AllSymbols) {
		t.Errorf("Component symbol count mismatch: %d vs %d", len(comp.SymbolNodes), len(res.AllSymbols))
	}

	// 6. Validate Hydrator Round-trip
	hydrator := NewCppHydrator()
	for _, sym := range res.AllSymbols {
		code, err := hydrator.HydrateSymbol(sym)
		if err != nil {
			t.Fatalf("HydrateSymbol failed for %s: %v", sym.Identifier, err)
		}
		if code == "" {
			t.Fatalf("Hydrated C++ is empty for %s", sym.Identifier)
		}
		if sym.NodeType == "ClassDeclaration" {
			if !strings.Contains(code, "class User") && !strings.Contains(code, "class BaseEntity") && !strings.Contains(code, "class Repository") {
				t.Errorf("Hydrated class missing declaration: %s", code)
			}
		}
	}
}
