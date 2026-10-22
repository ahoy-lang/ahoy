package ahoy

import (
	"strings"
	"testing"
)

const syntaxOnlySrc = `program p

@ main :: || void:
	a: undefined_fn|1|
$
`

func hasUndefinedError(errs []ParseError) bool {
	for _, err := range errs {
		if strings.Contains(err.Message, "not defined") {
			return true
		}
	}
	return false
}

// ParseSyntaxWithPath must produce the same declarations as a full lint parse
// without running the semantic checks. The LSP parses the syntax first, loads
// the program's imports, and only then runs the semantic pass with every name.
func TestParseSyntaxWithPathSkipsSemanticChecks(t *testing.T) {
	tokens := Tokenize(syntaxOnlySrc)

	ast, errs := ParseSyntaxWithPath(tokens, "test.ahoy")
	if ast == nil {
		t.Fatalf("ParseSyntaxWithPath produced no AST: %v", errs)
	}
	if hasUndefinedError(errs) {
		t.Fatalf("syntax pass ran semantic checks: %v", errs)
	}

	names := DeclaredNames(ast)
	if len(names) != 1 || names[0] != "main" {
		t.Fatalf("DeclaredNames = %v, want [main]", names)
	}

	// A full parse alone does report the unknown call.
	if _, errs := ParseLintWithPath(tokens, "test.ahoy"); !hasUndefinedError(errs) {
		t.Fatalf("ParseLintWithPath did not report the unknown call: %v", errs)
	}

	// With the program's names supplied, the same call resolves.
	if _, errs := ParseLintWithPathInProgram(tokens, "test.ahoy", append(names, "undefined_fn"), nil); hasUndefinedError(errs) {
		t.Fatalf("ParseLintWithPathInProgram still reported the call: %v", errs)
	}
}
