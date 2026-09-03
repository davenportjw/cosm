package ruby

import (
	"strings"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

func TestRubyParser_FullFeatures(t *testing.T) {
	rubySrc := `# frozen_string_literal: true
require "json"
require_relative "concerns/authenticatable"

# User model managing customer accounts
class User < ApplicationRecord
  include Authenticatable
  extend ActiveSupport::Concern

  attr_accessor :temporary_token
  attr_reader :login_count

  has_many :posts, dependent: :destroy
  belongs_to :organization
  has_one :profile

  validates :email, presence: true, uniqueness: true
  validates :age, numericality: true

  # Full display name
  def full_name(prefix = "Mr.")
    "#{prefix} #{first_name} #{last_name}"
  end

  def self.active_users(limit: 10)
    where(active: true).limit(limit)
  end

  private

  def sanitize_credentials
    self.email = email.downcase.strip
  end
end

# Application helper module
module StringHelper
  include FormattingConcern

  def capitalize_words(str)
    str.split.map(&:capitalize).join(" ")
  end
end

# Top-level utility method
def ping_service(host, port: 8080, &callback)
  puts "Pinging #{host}:#{port}"
  callback.call if block_given?
end
`

	lineage := core.LineageEnvelope{
		UserID:           "ruby-dev",
		UserPrompt:       "Implement Ruby ActiveRecord User model and helpers",
		ExecutingAgentID: "agent-ruby",
		Timestamp:        time.Now().UTC(),
	}

	parser := NewRubyParser()
	res, err := parser.ParseSource("app/models/user.rb", []byte(rubySrc), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	// 1. Validate Requires
	if len(res.Requires) != 2 {
		t.Errorf("Expected 2 requires, got %d", len(res.Requires))
	}

	// 2. Validate Class
	if len(res.Classes) != 1 {
		t.Fatalf("Expected 1 class, got %d", len(res.Classes))
	}
	userCls := res.Classes[0]
	if userCls.Name != "User" {
		t.Errorf("Expected class User, got %s", userCls.Name)
	}
	if userCls.Superclass != "ApplicationRecord" {
		t.Errorf("Expected superclass ApplicationRecord, got %s", userCls.Superclass)
	}
	if !userCls.IsModel {
		t.Errorf("Expected IsModel to be true")
	}
	if len(userCls.ModulesIncluded) != 1 || userCls.ModulesIncluded[0] != "Authenticatable" {
		t.Errorf("Expected include Authenticatable, got %v", userCls.ModulesIncluded)
	}
	if len(userCls.AttrAccessors) != 1 || userCls.AttrAccessors[0] != "temporary_token" {
		t.Errorf("Expected attr_accessor temporary_token, got %v", userCls.AttrAccessors)
	}
	if len(userCls.Associations) != 3 {
		t.Errorf("Expected 3 associations, got %d", len(userCls.Associations))
	}
	if len(userCls.Validations) != 2 {
		t.Errorf("Expected 2 validations, got %d", len(userCls.Validations))
	}
	if len(userCls.Methods) != 3 {
		t.Fatalf("Expected 3 methods in User, got %d", len(userCls.Methods))
	}

	// Method 1: full_name (public, has default param)
	fn := userCls.Methods[0]
	if fn.Name != "full_name" || fn.Visibility != "public" || len(fn.Params) != 1 {
		t.Errorf("Unexpected method full_name: %+v", fn)
	}

	// Method 2: self.active_users (class method, keyword param)
	classFn := userCls.Methods[1]
	if classFn.Name != "active_users" || !classFn.IsClassMethod {
		t.Errorf("Unexpected class method: %+v", classFn)
	}

	// Method 3: sanitize_credentials (private)
	privFn := userCls.Methods[2]
	if privFn.Name != "sanitize_credentials" || privFn.Visibility != "private" {
		t.Errorf("Unexpected private method: %+v", privFn)
	}

	// 3. Validate Module
	if len(res.Modules) != 1 {
		t.Fatalf("Expected 1 module, got %d", len(res.Modules))
	}
	mod := res.Modules[0]
	if mod.Name != "StringHelper" || len(mod.Methods) != 1 {
		t.Errorf("Unexpected module: %+v", mod)
	}

	// 4. Validate Top-level Method
	if len(res.Methods) != 1 {
		t.Fatalf("Expected 1 top-level method, got %d", len(res.Methods))
	}
	topMethod := res.Methods[0]
	if topMethod.Name != "ping_service" || len(topMethod.Params) != 3 {
		t.Errorf("Unexpected top method: %+v", topMethod)
	}

	// 5. Validate Component Building
	comp, err := parser.BuildComponentNode(res, "user-domain", core.CompService, lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode failed: %v", err)
	}
	if comp.Name != "user-domain" || comp.Language != core.LangRuby {
		t.Errorf("Unexpected component: %+v", comp)
	}
	if len(comp.SymbolNodes) != len(res.AllSymbols) {
		t.Errorf("Component symbols mismatch: %d vs %d", len(comp.SymbolNodes), len(res.AllSymbols))
	}

	// 6. Validate Hydration
	hydrator := NewRubyHydrator()
	for _, sym := range res.AllSymbols {
		code, hErr := hydrator.HydrateSymbol(sym)
		if hErr != nil {
			t.Fatalf("Failed to hydrate %s: %v", sym.Identifier, hErr)
		}
		if code == "" {
			t.Fatalf("Hydrated code is empty for %s", sym.Identifier)
		}

		if sym.NodeType == "ClassDef" {
			if !strings.Contains(code, "class User < ApplicationRecord") {
				t.Errorf("Hydrated class header missing: %s", code)
			}
			if !strings.Contains(code, "has_many :posts") {
				t.Errorf("Hydrated association missing: %s", code)
			}
			if !strings.Contains(code, "private") {
				t.Errorf("Hydrated private section missing: %s", code)
			}
		}
	}
}

func TestRubyParser_RailsRoutes(t *testing.T) {
	routesSrc := `Rails.application.routes.draw do
  root to: "home#index"

  scope "/api/v1" do
    get "/health", to: "health#check"
    post "/auth/login", to: "auth#login"
    delete "/auth/logout", to: "auth#logout"
  end

  namespace :admin do
    resources :users
  end
end
`

	lineage := core.LineageEnvelope{
		UserID:           "ruby-dev",
		UserPrompt:       "Parse Rails routes",
		ExecutingAgentID: "agent-ruby",
		Timestamp:        time.Now().UTC(),
	}

	parser := NewRubyParser()
	res, err := parser.ParseSource("config/routes.rb", []byte(routesSrc), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	if len(res.Routes) < 5 {
		t.Fatalf("Expected at least 5 routes, got %d", len(res.Routes))
	}

	// Check root route
	root := res.Routes[0]
	if root.Path != "/" || root.HandlerName != "home#index" {
		t.Errorf("Unexpected root route: %+v", root)
	}

	// Check scoped GET route
	healthRoute := res.Routes[1]
	if healthRoute.Method != "GET" || healthRoute.Path != "/api/v1/health" || healthRoute.HandlerName != "health#check" {
		t.Errorf("Unexpected health route: %+v", healthRoute)
	}

	// Check resources route
	resRoute := res.Routes[4]
	if resRoute.Method != "RESOURCES" || resRoute.Path != "/admin/users" {
		t.Errorf("Unexpected admin resources route: %+v", resRoute)
	}
}

func TestRubyHydrator_RoundtripIsomorphism(t *testing.T) {
	originalSrc := `# User Controller handling API requests
class UsersController < ApplicationController
  include Authenticatable

  # List all users
  def index(limit = 20)
    @users = User.limit(limit)
    render json: @users
  end

  # Create user
  def create(user_params:)
    @user = User.create!(user_params)
    render json: @user, status: :created
  end

  private

  def authenticate_request
    head :unauthorized unless current_user
  end
end
`

	lineage := core.LineageEnvelope{
		UserID:           "ruby-dev",
		UserPrompt:       "Verify roundtrip isomorphism",
		ExecutingAgentID: "agent-ruby",
		Timestamp:        time.Now().UTC(),
	}

	parser := NewRubyParser()
	res1, err := parser.ParseSource("app/controllers/users_controller.rb", []byte(originalSrc), lineage)
	if err != nil {
		t.Fatalf("Parse 1 failed: %v", err)
	}

	hydrator := NewRubyHydrator()
	classSym := res1.Classes[0]
	astNode, err := ClassToASTSymbolNode(classSym, "controllers", lineage)
	if err != nil {
		t.Fatalf("ClassToASTSymbolNode failed: %v", err)
	}

	hydratedSrc, err := hydrator.HydrateSymbol(astNode)
	if err != nil {
		t.Fatalf("HydrateSymbol failed: %v", err)
	}

	// Re-parse the hydrated source
	res2, err := parser.ParseSource("app/controllers/users_controller.rb", []byte(hydratedSrc), lineage)
	if err != nil {
		t.Fatalf("Parse 2 failed: %v", err)
	}

	if len(res2.Classes) != 1 {
		t.Fatalf("Expected 1 class in re-parsed source, got %d", len(res2.Classes))
	}

	cls2 := res2.Classes[0]
	if cls2.Name != classSym.Name {
		t.Errorf("Class name mismatch: %s vs %s", cls2.Name, classSym.Name)
	}
	if cls2.Superclass != classSym.Superclass {
		t.Errorf("Superclass mismatch: %s vs %s", cls2.Superclass, classSym.Superclass)
	}
	if len(cls2.Methods) != len(classSym.Methods) {
		t.Fatalf("Method count mismatch: %d vs %d", len(cls2.Methods), len(classSym.Methods))
	}

	for i := range classSym.Methods {
		m1 := classSym.Methods[i]
		m2 := cls2.Methods[i]
		if m1.Name != m2.Name {
			t.Errorf("Method %d name mismatch: %s vs %s", i, m1.Name, m2.Name)
		}
		if m1.Visibility != m2.Visibility {
			t.Errorf("Method %d visibility mismatch: %s vs %s", i, m1.Visibility, m2.Visibility)
		}
	}
}
