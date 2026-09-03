package elixir

import (
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

func TestElixirParser_FullFeatures(t *testing.T) {
	elixirSrc := `defmodule MyAppWeb.UserController do
  @moduledoc """
  Controller handling user accounts and REST endpoints.
  """

  use MyAppWeb, :controller
  import Plug.Conn
  alias MyApp.Accounts
  alias MyApp.Accounts.User
  plug :authenticate_user when action in [:show, :update]

  @doc """
  Renders a single user by ID.
  """
  @spec show(Plug.Conn.t(), map()) :: Plug.Conn.t()
  def show(conn, %{"id" => id}) when is_binary(id) do
    user = Accounts.get_user!(id)
    render(conn, :show, user: user)
  end

  @doc "Creates a new user"
  @spec create(Plug.Conn.t(), map()) :: Plug.Conn.t()
  def create(conn, %{"user" => user_params}) do
    case Accounts.create_user(user_params) do
      {:ok, user} ->
        conn
        |> put_status(:created)
        |> render(:show, user: user)

      {:error, %Ecto.Changeset{} = changeset} ->
        render(conn, :error, changeset: changeset)
    end
  end

  defp format_response(data, opts \\ []) do
    Map.merge(data, Enum.into(opts, %{}))
  end
end

defmodule MyApp.Accounts.User do
  use Ecto.Schema
  import Ecto.Changeset

  schema "users" do
    field :email, :string
    field :name, :string
    field :active, :boolean
    has_many :posts, MyApp.Blog.Post
    belongs_to :organization, MyApp.Accounts.Organization
  end
end
`

	lineage := core.LineageEnvelope{
		UserID:           "elixir-dev",
		UserPrompt:       "Parse Phoenix Controller and Ecto Schema",
		ExecutingAgentID: "agent-elixir",
		Timestamp:        time.Now().UTC(),
	}

	parser := NewElixirParser()
	res, err := parser.ParseSource("lib/my_app_web/controllers/user_controller.ex", []byte(elixirSrc), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	// 1. Module Count
	if len(res.Modules) != 2 {
		t.Fatalf("Expected 2 modules, got %d", len(res.Modules))
	}

	// Module 1: UserController
	m1 := res.Modules[0]
	if m1.Name != "MyAppWeb.UserController" {
		t.Errorf("Expected MyAppWeb.UserController, got %s", m1.Name)
	}
	if m1.Doc == "" {
		t.Errorf("Expected moduledoc to be populated")
	}
	if len(m1.Uses) != 1 || m1.Uses[0] != "MyAppWeb, :controller" {
		t.Errorf("Unexpected uses: %v", m1.Uses)
	}
	if len(m1.Aliases) != 2 {
		t.Errorf("Expected 2 aliases, got %d", len(m1.Aliases))
	}
	if len(m1.Plugs) != 1 {
		t.Errorf("Expected 1 plug, got %d", len(m1.Plugs))
	}
	if len(m1.Functions) != 3 {
		t.Fatalf("Expected 3 functions in UserController, got %d", len(m1.Functions))
	}

	// Function 1: show
	fnShow := m1.Functions[0]
	if fnShow.Name != "show" || fnShow.Arity != 2 || fnShow.Kind != "def" {
		t.Errorf("Unexpected show fn: %+v", fnShow)
	}
	if fnShow.GuardClause != "is_binary(id)" {
		t.Errorf("Expected guard clause 'is_binary(id)', got '%s'", fnShow.GuardClause)
	}
	if fnShow.Spec == "" {
		t.Errorf("Expected spec to be populated for show")
	}

	// Function 3: format_response (defp with default param)
	fnFormat := m1.Functions[2]
	if fnFormat.Name != "format_response" || fnFormat.Kind != "defp" || fnFormat.Arity != 2 {
		t.Errorf("Unexpected format_response fn: %+v", fnFormat)
	}
	if len(fnFormat.Params) == 2 && fnFormat.Params[1].DefaultValue != "[]" {
		t.Errorf("Expected default value '[]', got '%s'", fnFormat.Params[1].DefaultValue)
	}

	// Module 2: Ecto Schema User
	m2 := res.Modules[1]
	if m2.Name != "MyApp.Accounts.User" || !m2.IsSchema || m2.SchemaTable != "users" {
		t.Errorf("Unexpected schema module: %+v", m2)
	}
	if len(m2.SchemaFields) != 5 {
		t.Fatalf("Expected 5 schema fields/assocs, got %d", len(m2.SchemaFields))
	}
	if m2.SchemaFields[0].Name != "email" || m2.SchemaFields[0].Type != ":string" {
		t.Errorf("Unexpected field 0: %+v", m2.SchemaFields[0])
	}
	if m2.SchemaFields[3].Name != "posts" || m2.SchemaFields[3].AssociationType != "has_many" {
		t.Errorf("Unexpected assoc posts: %+v", m2.SchemaFields[3])
	}

	// 2. Component Building
	comp, err := parser.BuildComponentNode(res, "user-controller", core.CompService, lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode failed: %v", err)
	}
	if comp.Language != core.LangElixir || len(comp.SymbolNodes) != len(res.AllSymbols) {
		t.Errorf("Unexpected component: %+v", comp)
	}

	// 3. Hydration
	hydrator := NewElixirHydrator()
	for _, sym := range res.AllSymbols {
		code, hErr := hydrator.HydrateSymbol(sym)
		if hErr != nil {
			t.Fatalf("HydrateSymbol failed for %s: %v", sym.Identifier, hErr)
		}
		if code == "" {
			t.Fatalf("Hydrated code is empty for %s", sym.Identifier)
		}
	}
}

func TestElixirParser_PhoenixRouter(t *testing.T) {
	routerSrc := `defmodule MyAppWeb.Router do
  use MyAppWeb, :router

  pipeline :api do
    plug :accepts, ["json"]
  end

  scope "/api/v1", MyAppWeb do
    pipe_through :api

    get "/health", HealthController, :index
    post "/auth/login", AuthController, :login
    resources "/users", UserController
  end
end
`

	lineage := core.LineageEnvelope{
		UserID:           "elixir-dev",
		UserPrompt:       "Parse Phoenix router",
		ExecutingAgentID: "agent-elixir",
		Timestamp:        time.Now().UTC(),
	}

	parser := NewElixirParser()
	res, err := parser.ParseSource("lib/my_app_web/router.ex", []byte(routerSrc), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	if len(res.Routes) != 5 {
		t.Fatalf("Expected 5 Phoenix routes, got %d", len(res.Routes))
	}

	// Route 1: GET /api/v1/health
	r1 := res.Routes[0]
	if r1.Method != "GET" || r1.Path != "/api/v1/health" || r1.Controller != "HealthController" || r1.Action != "index" {
		t.Errorf("Unexpected route 1: %+v", r1)
	}

	// Route 2: POST /api/v1/auth/login
	r2 := res.Routes[1]
	if r2.Method != "POST" || r2.Path != "/api/v1/auth/login" || r2.Controller != "AuthController" || r2.Action != "login" {
		t.Errorf("Unexpected route 2: %+v", r2)
	}

	// Route 3: GET /api/v1/users (from resources)
	r3 := res.Routes[2]
	if r3.Method != "GET" || r3.Path != "/api/v1/users" || r3.Controller != "UserController" || r3.Action != "index" {
		t.Errorf("Unexpected resource route index: %+v", r3)
	}
}

func TestElixirHydrator_RoundtripIsomorphism(t *testing.T) {
	originalSrc := `defmodule MyApp.MathService do
  @moduledoc """
  Service calculating arithmetic algorithms.
  """

  @doc "Computes factorial of a non-negative integer"
  @spec factorial(integer()) :: integer()
  def factorial(0), do: 1

  def factorial(n) when n > 0 do
    n * factorial(n - 1)
  end
end
`

	lineage := core.LineageEnvelope{
		UserID:           "elixir-dev",
		UserPrompt:       "Verify roundtrip isomorphism",
		ExecutingAgentID: "agent-elixir",
		Timestamp:        time.Now().UTC(),
	}

	parser := NewElixirParser()
	res1, err := parser.ParseSource("lib/my_app/math_service.ex", []byte(originalSrc), lineage)
	if err != nil {
		t.Fatalf("Parse 1 failed: %v", err)
	}

	if len(res1.Modules) != 1 {
		t.Fatalf("Expected 1 module, got %d", len(res1.Modules))
	}

	hydrator := NewElixirHydrator()
	astNode, err := ModuleToASTSymbolNode(res1.Modules[0], lineage)
	if err != nil {
		t.Fatalf("ModuleToASTSymbolNode failed: %v", err)
	}

	hydratedCode, err := hydrator.HydrateSymbol(astNode)
	if err != nil {
		t.Fatalf("HydrateSymbol failed: %v", err)
	}

	// Re-parse hydrated code
	res2, err := parser.ParseSource("lib/my_app/math_service.ex", []byte(hydratedCode), lineage)
	if err != nil {
		t.Fatalf("Parse 2 failed: %v", err)
	}

	if len(res2.Modules) != 1 {
		t.Fatalf("Expected 1 module in re-parsed result, got %d", len(res2.Modules))
	}

	m1 := res1.Modules[0]
	m2 := res2.Modules[0]

	if m1.Name != m2.Name {
		t.Errorf("Module name mismatch: %s vs %s", m1.Name, m2.Name)
	}
	if len(m1.Functions) != len(m2.Functions) {
		t.Fatalf("Function count mismatch: %d vs %d", len(m1.Functions), len(m2.Functions))
	}

	for i := range m1.Functions {
		if m1.Functions[i].Name != m2.Functions[i].Name {
			t.Errorf("Function %d name mismatch: %s vs %s", i, m1.Functions[i].Name, m2.Functions[i].Name)
		}
	}
}
