package csharp

import (
	"strings"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

func TestCSharpParser_AspNetCoreAndEFCore(t *testing.T) {
	csSrc := `using System;
using System.Collections.Generic;
using System.Threading.Tasks;
using Microsoft.AspNetCore.Mvc;
using Microsoft.EntityFrameworkCore;
using System.ComponentModel.DataAnnotations.Schema;

namespace MyApp.Services;

/// <summary>
/// User persistent entity model mapped to database table
/// </summary>
[Table("users")]
public class UserEntity
{
    public int Id { get; set; }

    public string Email { get; set; }

    public string FullName { get; set; }

    public DateTime CreatedAt { get; set; }
}

/// <summary>
/// Application database context managing entity persistence
/// </summary>
public class AppDbContext : DbContext
{
    public DbSet<UserEntity> Users { get; set; }

    public DbSet<OrderEntity> Orders { get; set; }
}

/// <summary>
/// REST API Controller for managing users
/// </summary>
[ApiController]
[Route("api/v1/[controller]")]
public class UsersController : ControllerBase
{
    private readonly AppDbContext _db;

    public UsersController(AppDbContext db)
    {
        _db = db;
    }

    [HttpGet]
    public async Task<List<UserEntity>> GetUsers()
    {
        return await _db.Users.ToListAsync();
    }

    [HttpGet("{id}")]
    public async Task<ActionResult<UserEntity>> GetUserById(int id)
    {
        var user = await _db.Users.FindAsync(id);
        if (user == null) return NotFound();
        return user;
    }

    [HttpPost]
    public async Task<ActionResult<UserEntity>> CreateUser([FromBody] UserEntity user)
    {
        _db.Users.Add(user);
        await _db.SaveChangesAsync();
        return CreatedAtAction(nameof(GetUserById), new { id = user.Id }, user);
    }

    [HttpDelete("{id}")]
    public async Task<IActionResult> DeleteUser(int id)
    {
        var user = await _db.Users.FindAsync(id);
        if (user != null)
        {
            _db.Users.Remove(user);
            await _db.SaveChangesAsync();
        }
        return NoContent();
    }
}
`

	lineage := core.LineageEnvelope{
		UserID:           "csharp-dev",
		UserPrompt:       "Create ASP.NET Core controller and EF Core models",
		ExecutingAgentID: "agent-csharp",
		Timestamp:        time.Now().UTC(),
	}

	parser := NewCSharpParser()
	res, err := parser.ParseSource("Controllers/UsersController.cs", []byte(csSrc), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	// 1. Namespace & Usings
	if res.Namespace != "MyApp.Services" {
		t.Errorf("Expected namespace 'MyApp.Services', got '%s'", res.Namespace)
	}
	if len(res.Usings) < 5 {
		t.Errorf("Expected at least 5 usings, got %d", len(res.Usings))
	}

	// 2. Types Check
	if len(res.Types) != 3 {
		t.Fatalf("Expected 3 types, got %d", len(res.Types))
	}

	// UserEntity
	entity := res.Types[0]
	if entity.Name != "UserEntity" {
		t.Errorf("Expected UserEntity, got %s", entity.Name)
	}
	if !entity.IsEntity {
		t.Errorf("Expected entity.IsEntity to be true")
	}
	if entity.TableName != "users" {
		t.Errorf("Expected table name 'users', got '%s'", entity.TableName)
	}
	if len(entity.Properties) != 4 {
		t.Errorf("Expected 4 properties in UserEntity, got %d", len(entity.Properties))
	}

	// AppDbContext
	dbContext := res.Types[1]
	if dbContext.Name != "AppDbContext" {
		t.Errorf("Expected AppDbContext, got %s", dbContext.Name)
	}
	if !dbContext.IsDbContext {
		t.Errorf("Expected dbContext.IsDbContext to be true")
	}
	if len(dbContext.Properties) != 2 {
		t.Errorf("Expected 2 DbSet properties in AppDbContext, got %d", len(dbContext.Properties))
	}
	if !dbContext.Properties[0].IsDbSet || dbContext.Properties[0].DbSetEntity != "UserEntity" {
		t.Errorf("Expected DbSet<UserEntity>, got %+v", dbContext.Properties[0])
	}

	// UsersController
	controller := res.Types[2]
	if controller.Name != "UsersController" {
		t.Errorf("Expected UsersController, got %s", controller.Name)
	}
	if !controller.IsController {
		t.Errorf("Expected controller.IsController to be true")
	}
	if controller.BasePath != "api/v1/users" {
		t.Errorf("Expected BasePath 'api/v1/users', got '%s'", controller.BasePath)
	}
	if len(controller.Methods) != 5 { // 1 ctor + 4 methods
		t.Errorf("Expected 5 methods/ctors in UsersController, got %d", len(controller.Methods))
	}

	// 3. Controller Routes
	if len(res.Routes) != 4 {
		t.Fatalf("Expected 4 routes, got %d", len(res.Routes))
	}

	expectedRoutes := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/users"},
		{"GET", "/api/v1/users/{id}"},
		{"POST", "/api/v1/users"},
		{"DELETE", "/api/v1/users/{id}"},
	}

	for i, exp := range expectedRoutes {
		r := res.Routes[i]
		if r.Method != exp.method || r.Path != exp.path {
			t.Errorf("Route %d mismatch: expected %s %s, got %s %s", i, exp.method, exp.path, r.Method, r.Path)
		}
		if r.Framework != "aspnetcore" {
			t.Errorf("Expected framework aspnetcore, got %s", r.Framework)
		}
	}

	// 4. Component Node
	comp, err := parser.BuildComponentNode(res, "user-service", core.CompService, lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode failed: %v", err)
	}
	if comp.Language != core.LangCSharp {
		t.Errorf("Expected LangCSharp, got %s", comp.Language)
	}
	if len(comp.SymbolNodes) != len(res.AllSymbols) {
		t.Errorf("Component symbols mismatch: %d vs %d", len(comp.SymbolNodes), len(res.AllSymbols))
	}

	// 5. Hydration Round-trip Check
	hydrator := NewCSharpHydrator()
	for _, sym := range res.AllSymbols {
		code, err := hydrator.HydrateSymbol(sym)
		if err != nil {
			t.Fatalf("Failed to hydrate C# symbol %s: %v", sym.Identifier, err)
		}
		if code == "" {
			t.Fatalf("Hydrated C# code is empty for %s", sym.Identifier)
		}

		if sym.NodeType == "ClassDeclaration" {
			if !strings.Contains(code, "class UserEntity") && !strings.Contains(code, "class AppDbContext") && !strings.Contains(code, "class UsersController") {
				t.Errorf("Hydrated class missing name: %s", code)
			}
		}
	}
}

func TestCSharpParser_MinimalAPIsAndRecords(t *testing.T) {
	csSrc := `using System;
using Microsoft.AspNetCore.Builder;
using Microsoft.AspNetCore.Http;

namespace TodoApi;

public record TodoItem(int Id, string Title, bool IsComplete);

public enum TodoStatus
{
    Pending = 0,
    InProgress = 1,
    Completed = 2
}

public interface ITodoService
{
    Task<List<TodoItem>> GetAllAsync();
    Task<TodoItem?> GetByIdAsync(int id);
}

public class Program
{
    public static void Main(string[] args)
    {
        var builder = WebApplication.CreateBuilder(args);
        var app = builder.Build();

        app.MapGet("/api/v1/todos", () => Results.Ok(new List<TodoItem>()));
        app.MapGet("/api/v1/todos/{id}", (int id) => Results.Ok(new TodoItem(id, "Test", false)));
        app.MapPost("/api/v1/todos", (TodoItem todo) => Results.Created($"/api/v1/todos/{todo.Id}", todo));
        app.MapDelete("/api/v1/todos/{id}", (int id) => Results.NoContent());

        app.Run();
    }
}
`

	lineage := core.LineageEnvelope{
		UserID:           "csharp-dev",
		UserPrompt:       "Implement Minimal API with records, enums, and interfaces",
		ExecutingAgentID: "agent-csharp",
		Timestamp:        time.Now().UTC(),
	}

	parser := NewCSharpParser()
	res, err := parser.ParseSource("Program.cs", []byte(csSrc), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	// Verify Types
	if len(res.Types) != 4 {
		t.Fatalf("Expected 4 types (record, enum, interface, class), got %d", len(res.Types))
	}

	recordType := res.Types[0]
	if recordType.Kind != "record" || recordType.Name != "TodoItem" {
		t.Errorf("Expected record TodoItem, got %s %s", recordType.Kind, recordType.Name)
	}

	enumType := res.Types[1]
	if enumType.Kind != "enum" || enumType.Name != "TodoStatus" {
		t.Errorf("Expected enum TodoStatus, got %s %s", enumType.Kind, enumType.Name)
	}
	if len(enumType.EnumMembers) != 3 {
		t.Errorf("Expected 3 enum members, got %d", len(enumType.EnumMembers))
	}

	ifaceType := res.Types[2]
	if ifaceType.Kind != "interface" || ifaceType.Name != "ITodoService" {
		t.Errorf("Expected interface ITodoService, got %s %s", ifaceType.Kind, ifaceType.Name)
	}

	// Verify Minimal API routes
	if len(res.Routes) != 4 {
		t.Fatalf("Expected 4 minimal API routes, got %d", len(res.Routes))
	}

	for _, r := range res.Routes {
		if !r.IsMinimalAPI {
			t.Errorf("Expected route %s to be Minimal API", r.Path)
		}
	}

	// Hydrate and re-parse isomorphism
	hydrator := NewCSharpHydrator()
	for _, sym := range res.AllSymbols {
		code, err := hydrator.HydrateSymbol(sym)
		if err != nil {
			t.Fatalf("Hydration error: %v", err)
		}
		if code == "" {
			t.Fatalf("Empty hydrated code for %s", sym.Identifier)
		}
	}
}

func TestCSharpParser_GenericsAndStructs(t *testing.T) {
	csSrc := `namespace DataStore;

public struct Point3D
{
    public double X { get; set; }
    public double Y { get; set; }
    public double Z { get; set; }
}

public class Repository<T, TKey> : IRepository<T, TKey> where T : class
{
    private readonly List<T> _items = new();

    public async Task<T?> FindByIdAsync(TKey id)
    {
        return default;
    }

    public async Task AddAsync(T entity)
    {
        _items.Add(entity);
    }
}
`
	lineage := core.LineageEnvelope{
		UserID:           "csharp-dev",
		UserPrompt:       "Generic repository and 3D struct",
		ExecutingAgentID: "agent-csharp",
		Timestamp:        time.Now().UTC(),
	}

	parser := NewCSharpParser()
	res, err := parser.ParseSource("Data/Repository.cs", []byte(csSrc), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	if len(res.Types) != 2 {
		t.Fatalf("Expected 2 types, got %d", len(res.Types))
	}

	structType := res.Types[0]
	if structType.Kind != "struct" || structType.Name != "Point3D" {
		t.Errorf("Expected struct Point3D, got %s %s", structType.Kind, structType.Name)
	}
	if len(structType.Properties) != 3 {
		t.Errorf("Expected 3 properties in Point3D, got %d", len(structType.Properties))
	}

	genericType := res.Types[1]
	if genericType.Name != "Repository" {
		t.Errorf("Expected Repository, got %s", genericType.Name)
	}
	if len(genericType.GenericParams) != 2 || genericType.GenericParams[0] != "T" || genericType.GenericParams[1] != "TKey" {
		t.Errorf("Expected generic params [T, TKey], got %+v", genericType.GenericParams)
	}
	if len(genericType.Methods) != 2 {
		t.Errorf("Expected 2 methods in generic Repository, got %d", len(genericType.Methods))
	}

	hydrator := NewCSharpHydrator()
	for _, sym := range res.AllSymbols {
		code, err := hydrator.HydrateSymbol(sym)
		if err != nil {
			t.Fatalf("Hydration failed for %s: %v", sym.Identifier, err)
		}
		if code == "" {
			t.Fatalf("Hydration produced empty string for %s", sym.Identifier)
		}
	}
}
