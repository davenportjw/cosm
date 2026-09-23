package core

// TriviaEnvelope encapsulates non-semantic file/component trivia, such as compiler directives,
// build tags, encoding pragmas, and license headers.
type TriviaEnvelope struct {
	HeaderDirectives []string `json:"header_directives,omitempty"` // e.g. //go:build, //go:embed, # -*- coding
	LicenseHeader    string   `json:"license_header,omitempty"`
	ModuleDocstring  string   `json:"module_docstring,omitempty"`
}
