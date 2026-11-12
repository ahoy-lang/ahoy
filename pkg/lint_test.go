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

func closeErrors(errs []ParseError) []ParseError {
	var out []ParseError
	for _, e := range errs {
		if strings.Contains(e.Message, "Expected '$' to close") {
			out = append(out, e)
		}
	}
	return out
}

// A missing inner '$' must report exactly one unclosed-block error naming
// the innermost opener — not a cascade ending at the function line.
func TestUnclosedBlockReportsInnermostOpener(t *testing.T) {
	src := "program p\n\n@ main ||:\n\tloop i:0 to 3 do\n\t\tif i > 1 then\n\t\t\tprint|i|\n\t$\n"
	_, errs := ParseLintWithPath(Tokenize(src), "test.ahoy")
	got := closeErrors(errs)
	if len(got) != 1 {
		t.Fatalf("want exactly 1 unclosed-block error, got %d: %v", len(got), errs)
	}
	if !strings.Contains(got[0].Message, "close loop at line 4") {
		t.Fatalf("want the innermost opener (loop at line 4), got %q at line %d", got[0].Message, got[0].Line)
	}
	if got[0].Line != 4 {
		t.Fatalf("want the diagnostic on line 4, got line %d", got[0].Line)
	}
}

// A function whose own '$' is missing (inners balanced) still blames the
// function itself.
func TestUnclosedFunctionStillReported(t *testing.T) {
	src := "program p\n\n@ main ||:\n\tprint|1|\n"
	_, errs := ParseLintWithPath(Tokenize(src), "test.ahoy")
	got := closeErrors(errs)
	if len(got) != 1 {
		t.Fatalf("want exactly 1 unclosed-block error, got %d: %v", len(got), errs)
	}
	if !strings.Contains(got[0].Message, "close function at line 3") {
		t.Fatalf("want the function opener (line 3), got %q", got[0].Message)
	}
}

// Balanced blocks report no unclosed-block errors.
func TestBalancedBlocksReportNothing(t *testing.T) {
	src := "program p\n\n@ main ||:\n\tprint|1|\n$\n"
	_, errs := ParseLintWithPath(Tokenize(src), "test.ahoy")
	if got := closeErrors(errs); len(got) != 0 {
		t.Fatalf("want no unclosed-block errors, got %v", got)
	}
}
