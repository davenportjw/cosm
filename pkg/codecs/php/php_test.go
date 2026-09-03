package php

import (
	"fmt"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

func TestPHPParser_FullFeatures(t *testing.T) {
	phpSrc := `<?php

namespace App\Http\Controllers;

use App\Models\User;
use App\Services\UserService;
use Symfony\Component\Routing\Annotation\Route;

/**
 * Controller managing User accounts and REST operations
 */
#[Route('/api/v1/users')]
class UserController extends BaseController implements CrudInterface
{
    use NotifiableTrait;

    /**
     * User database identifier
     */
    #[ORM\Id]
    private string $id;

    public function __construct(
        private UserService $userService,
        public readonly int $timeout = 30
    ) {
        $this->init();
    }

    /**
     * Fetch user by identifier
     */
    #[Route('/{id}', methods: ['GET'])]
    public function show(string $id): UserResponse
    {
        return $this->userService->find($id);
    }

    #[Route('', methods: ['POST'])]
    public function store(CreateUserRequest $request): JsonResponse
    {
        return $this->userService->create($request->all());
    }
}

enum UserRole: string
{
    case Admin = 'ADMIN';
    case Member = 'MEMBER';
}

interface CrudInterface
{
    public function show(string $id): UserResponse;
}
`

	lineage := core.LineageEnvelope{
		UserID:           "php-dev",
		UserPrompt:       "Implement PHP 8 Symfony controller and enum",
		ExecutingAgentID: "agent-php",
		Timestamp:        time.Now().UTC(),
	}

	parser := NewPHPParser()
	res, err := parser.ParseSource("src/Controller/UserController.php", []byte(phpSrc), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	// 1. Namespace & Uses
	if res.Namespace != "App\\Http\\Controllers" {
		t.Errorf("Expected namespace App\\Http\\Controllers, got %s", res.Namespace)
	}
	if len(res.Uses) != 3 {
		t.Errorf("Expected 3 use statements, got %d", len(res.Uses))
	}

	// 2. Classes / Enums / Interfaces
	if len(res.Classes) != 3 {
		t.Fatalf("Expected 3 classes/entities, got %d", len(res.Classes))
	}

	// Class 1: UserController
	ctrl := res.Classes[0]
	if ctrl.Name != "UserController" || ctrl.Kind != "class" {
		t.Errorf("Unexpected controller: %+v", ctrl)
	}
	if ctrl.Extends != "BaseController" {
		t.Errorf("Expected extends BaseController, got %s", ctrl.Extends)
	}
	if len(ctrl.Implements) != 1 || ctrl.Implements[0] != "CrudInterface" {
		t.Errorf("Expected implements CrudInterface, got %v", ctrl.Implements)
	}
	if len(ctrl.TraitsUsed) != 1 || ctrl.TraitsUsed[0] != "NotifiableTrait" {
		t.Errorf("Expected trait NotifiableTrait, got %v", ctrl.TraitsUsed)
	}
	if ctrl.BasePath != "/api/v1/users" {
		t.Errorf("Expected base route /api/v1/users, got %s", ctrl.BasePath)
	}
	if len(ctrl.Methods) != 3 {
		t.Fatalf("Expected 3 methods in UserController, got %d", len(ctrl.Methods))
	}

	// Method 1: __construct (promoted properties)
	ctor := ctrl.Methods[0]
	if ctor.Name != "__construct" || len(ctor.Params) != 2 {
		t.Errorf("Unexpected constructor: %+v", ctor)
	}
	if !ctor.Params[0].IsPromoted || ctor.Params[0].Visibility != "private" {
		t.Errorf("Expected promoted private param, got %+v", ctor.Params[0])
	}

	// Method 2: show (with Symfony route attribute)
	show := ctrl.Methods[1]
	if show.Name != "show" || show.RoutePath != "/{id}" || show.RouteMethod != "GET" {
		t.Errorf("Unexpected show method: %+v", show)
	}

	// Class 2: UserRole (backed enum)
	enumRole := res.Classes[1]
	if enumRole.Kind != "enum" || enumRole.Name != "UserRole" || enumRole.BackedType != "string" {
		t.Errorf("Unexpected enum: %+v", enumRole)
	}
	if len(enumRole.EnumCases) != 2 {
		t.Fatalf("Expected 2 enum cases, got %d", len(enumRole.EnumCases))
	}
	if enumRole.EnumCases[0].Name != "Admin" || enumRole.EnumCases[0].Value != "ADMIN" {
		t.Errorf("Unexpected enum case Admin: %+v", enumRole.EnumCases[0])
	}

	// Class 3: CrudInterface
	iface := res.Classes[2]
	if iface.Kind != "interface" || iface.Name != "CrudInterface" {
		t.Errorf("Unexpected interface: %+v", iface)
	}

	// 3. Symfony Routes
	if len(res.Routes) != 2 {
		t.Fatalf("Expected 2 Symfony routes, got %d", len(res.Routes))
	}
	r1 := res.Routes[0]
	if r1.Method != "GET" || r1.Path != "/api/v1/users/{id}" {
		t.Errorf("Unexpected route 1: %+v", r1)
	}
	r2 := res.Routes[1]
	if r2.Method != "POST" || r2.Path != "/api/v1/users" {
		t.Errorf("Unexpected route 2: %+v", r2)
	}

	// 4. Component Building
	comp, err := parser.BuildComponentNode(res, "user-service", core.CompService, lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode failed: %v", err)
	}
	if comp.Language != core.LangPHP || len(comp.SymbolNodes) != len(res.AllSymbols) {
		t.Errorf("Unexpected component node: %+v", comp)
	}

	// 5. Hydration
	hydrator := NewPHPHydrator()
	for _, sym := range res.AllSymbols {
		code, hErr := hydrator.HydrateSymbol(sym)
		if hErr != nil {
			t.Fatalf("Failed to hydrate %s: %v", sym.Identifier, hErr)
		}
		if code == "" {
			t.Fatalf("Hydrated code is empty for %s", sym.Identifier)
		}
	}
}

func TestPHPParser_LaravelRoutes(t *testing.T) {
	routesSrc := `<?php

use Illuminate\Support\Facades\Route;
use App\Http\Controllers\UserController;
use App\Http\Controllers\OrderController;

Route::get('/health', 'HealthController@check');
Route::post('/login', [AuthController::class, 'login']);
Route::apiResource('users', UserController::class);
Route::delete('/orders/{id}', [OrderController::class, 'destroy']);
`

	lineage := core.LineageEnvelope{
		UserID:           "laravel-dev",
		UserPrompt:       "Parse Laravel routes",
		ExecutingAgentID: "agent-php",
		Timestamp:        time.Now().UTC(),
	}

	parser := NewPHPParser()
	res, err := parser.ParseSource("routes/api.php", []byte(routesSrc), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	if len(res.Routes) != 4 {
		t.Fatalf("Expected 4 Laravel routes, got %d", len(res.Routes))
	}

	if res.Routes[0].Method != "GET" || res.Routes[0].Path != "/health" || res.Routes[0].HandlerName != "HealthController@check" {
		t.Errorf("Unexpected route 0: %+v", res.Routes[0])
	}

	if res.Routes[1].Method != "POST" || res.Routes[1].Path != "/login" || res.Routes[1].HandlerName != "AuthController::login" {
		t.Errorf("Unexpected route 1: %+v", res.Routes[1])
	}

	if res.Routes[2].Method != "ANY" || res.Routes[2].Path != "/users" || res.Routes[2].HandlerName != "UserController" {
		t.Errorf("Unexpected route 2: %+v", res.Routes[2])
	}
}

func TestPHPHydrator_RoundtripIsomorphism(t *testing.T) {
	originalSrc := `<?php

namespace App\Services;

use App\Repositories\UserRepository;

/**
 * Service managing domain user operations
 */
class UserService
{
    private UserRepository $repo;

    public function __construct(UserRepository $repo)
    {
        $this->repo = $repo;
    }

    public function find(string $id): ?User
    {
        return $this->repo->find($id);
    }
}
`

	lineage := core.LineageEnvelope{
		UserID:           "php-dev",
		UserPrompt:       "Verify roundtrip isomorphism",
		ExecutingAgentID: "agent-php",
		Timestamp:        time.Now().UTC(),
	}

	parser := NewPHPParser()
	res1, err := parser.ParseSource("src/Services/UserService.php", []byte(originalSrc), lineage)
	if err != nil {
		t.Fatalf("Parse 1 failed: %v", err)
	}

	if len(res1.Classes) != 1 {
		t.Fatalf("Expected 1 class, got %d", len(res1.Classes))
	}

	hydrator := NewPHPHydrator()
	astNode, err := ClassToASTSymbolNode(res1.Classes[0], res1.Namespace, lineage)
	if err != nil {
		t.Fatalf("ClassToASTSymbolNode failed: %v", err)
	}

	hydratedCode, err := hydrator.HydrateSymbol(astNode)
	if err != nil {
		t.Fatalf("HydrateSymbol failed: %v", err)
	}

	// Re-parse hydrated code
	reparseSrc := fmt.Sprintf("<?php\n\nnamespace %s;\n\n%s", res1.Namespace, hydratedCode)
	res2, err := parser.ParseSource("src/Services/UserService.php", []byte(reparseSrc), lineage)
	if err != nil {
		t.Fatalf("Parse 2 failed: %v", err)
	}

	if len(res2.Classes) != 1 {
		t.Fatalf("Expected 1 class in re-parsed result, got %d", len(res2.Classes))
	}

	cls1 := res1.Classes[0]
	cls2 := res2.Classes[0]

	if cls1.Name != cls2.Name {
		t.Errorf("Class name mismatch: %s vs %s", cls1.Name, cls2.Name)
	}
	if len(cls1.Methods) != len(cls2.Methods) {
		t.Fatalf("Method count mismatch: %d vs %d", len(cls1.Methods), len(cls2.Methods))
	}

	for i := range cls1.Methods {
		if cls1.Methods[i].Name != cls2.Methods[i].Name {
			t.Errorf("Method %d name mismatch: %s vs %s", i, cls1.Methods[i].Name, cls2.Methods[i].Name)
		}
	}
}
