package codecs_test

import (
	"testing"

	"github.com/cosmscm/cosm/pkg/codecs"
	"github.com/cosmscm/cosm/pkg/core"
)

func TestParseSourceFile_PolyglotCoverage(t *testing.T) {
	lineage := core.LineageEnvelope{Intent: "Test polyglot dispatcher"}

	cases := []struct {
		filename string
		content  string
		lang     core.Language
	}{
		{"main.go", "package main\nfunc Hello() string { return \"hi\" }", core.LangGo},
		{"app.py", "def hello():\n    return 'hi'", core.LangPython},
		{"main.tf", "resource \"aws_s3_bucket\" \"b\" { bucket = \"my-bucket\" }", core.LangHCL},
		{"index.ts", "export interface UserProfile {\n  id: string;\n}\nexport const App = () => { return null; };", core.LangTypeScript},
		{"lib.rs", "pub fn hello() -> &'static str { \"hi\" }", core.LangRust},
		{"App.java", "public class App {\n    private String name;\n}", core.LangJava},
		{"main.cpp", "int add(int a, int b) { return a + b; }", core.LangCpp},
		{"Service.cs", "namespace App {\n    public class Service {\n        public void Run() {}\n    }\n}", core.LangCSharp},
		{"View.swift", "import SwiftUI\nstruct ContentView: View { var body: some View { Text(\"Hi\") } }", core.LangSwift},
		{"App.kt", "fun main() { println(\"Hello\") }", core.LangKotlin},
		{"schema.sql", "CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT);", core.LangSQL},
		{"user.proto", "syntax = \"proto3\";\nmessage User { string id = 1; }", core.LangProtobuf},
		{"main.zig", "pub fn main() void {}", core.LangZig},
		{"schema.graphql", "type Query { hello: String }", core.LangGraphQL},
		{"app.rb", "def hello\n  'hi'\nend", core.LangRuby},
		{"index.php", "<?php\nfunction hello() { return 'hi'; }", core.LangPHP},
		{"user.ex", "defmodule User do\n  def hello, do: :hi\nend", core.LangElixir},
		{"Dockerfile", "FROM alpine:3.19\nCMD [\"echo\", \"hello\"]", core.LangDockerfile},
	}

	for _, tc := range cases {
		t.Run(tc.filename, func(t *testing.T) {
			res, err := codecs.ParseSourceFile(tc.filename, []byte(tc.content), lineage)
			if err != nil {
				t.Fatalf("ParseSourceFile failed for %s: %v", tc.filename, err)
			}
			if res.Component == nil {
				t.Fatalf("expected non-nil component for %s", tc.filename)
			}
			if res.Component.Language != tc.lang {
				t.Errorf("expected language %s for %s, got %s", tc.lang, tc.filename, res.Component.Language)
			}
			if len(res.Symbols) == 0 {
				t.Errorf("expected symbols for %s, got 0", tc.filename)
			}
		})
	}
}
