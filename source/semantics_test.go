package main

import (
	"strings"
	"testing"

	"ahoy"
)

// Tests for the semantic checks the compiler runs during a build:
// undefined names, argument counts and operator operand types.

// diagnosticsFor validates a single-file program and returns its fatal
// messages.
func diagnosticsFor(src string) []string {
	_, errs := ahoy.ParseLintWithPath(ahoy.Tokenize(src), "test.ahoy")
	return fatalMessages(errs)
}

func fatalMessages(errs []ahoy.ParseError) []string {
	out := make([]string, 0, len(errs))
	for _, e := range errs {
		if !e.IsWarning() {
			out = append(out, e.Message)
		}
	}
	return out
}

func wantError(t *testing.T, msgs []string, substr string) {
	t.Helper()
	for _, m := range msgs {
		if strings.Contains(m, substr) {
			return
		}
	}
	t.Errorf("expected an error containing %q, got %v", substr, msgs)
}

func wantNoError(t *testing.T, msgs []string, substr string) {
	t.Helper()
	for _, m := range msgs {
		if strings.Contains(m, substr) {
			t.Errorf("did not expect an error containing %q, got %v", substr, msgs)
			return
		}
	}
}

// TestDeclaredNamesIsExported also proves the source module can see the symbol;
// it is the API one file of a program uses to learn the names its siblings
// declare.
func TestDeclaredNamesIsExported(t *testing.T) {
	src := `program p
struct Point:
	x: float
$
enum Color:
	RED
$
LIMIT::int= 10
@ helper ::|a:int| int:
	return a
$
`
	ast := ahoy.ParseWithPath(ahoy.Tokenize(src), "test.ahoy")
	got := map[string]bool{}
	for _, name := range ahoy.DeclaredNames(ast) {
		got[name] = true
	}
	for _, want := range []string{"Point", "Color", "LIMIT", "helper"} {
		if !got[want] {
			t.Errorf("DeclaredNames did not report %q; got %v", want, ahoy.DeclaredNames(ast))
		}
	}
}

func TestUndefinedFunctionIsReported(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ main ||:
	undefined_func|1, 2|
$
`)
	wantError(t, msgs, "Function 'undefined_func' is not defined")
}

func TestFunctionDeclaredLaterIsNotReported(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ main ||:
	r: helper|1|
	print|r|
$
@ helper ::|a:int| int:
	return a
$
`)
	wantNoError(t, msgs, "is not defined")
}

func TestBuiltinsAndCastsAreNotReported(t *testing.T) {
	// Note: a `string|..|` cast immediately after `name:` is ambiguous with a
	// typed declaration (`name: string`), so it is not used here.
	msgs := diagnosticsFor(`program p
@ main ||:
	items: [1, 2, 3]
	ahoy |"hi"|
	n: len|items|
	i: int|"5"|
	f: float|"1.5"|
	items.push|4|
	items.map|x: x * 2|
	print|n, i, f|
$
`)
	wantNoError(t, msgs, "is not defined")
}

func TestArgumentCountIsReported(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ add ::|a:int, b:int| int:
	return a + b
$
@ main ||:
	r: add|1|
$
`)
	wantError(t, msgs, "Function 'add' expects 2 argument(s) but 1 given")
}

func TestArgumentCountHonoursDefaults(t *testing.T) {
	const prog = `program p
@ greet ::|name:string, greeting:string="Hi"| void:
	print|name|
$
@ main ||:
%s
$
`
	ok := diagnosticsFor(strings.Replace(prog, "%s", `	greet|"a"|`, 1))
	wantNoError(t, ok, "expects")

	ok2 := diagnosticsFor(strings.Replace(prog, "%s", `	greet|"a", "b"|`, 1))
	wantNoError(t, ok2, "expects")

	bad := diagnosticsFor(strings.Replace(prog, "%s", `	greet|"a", "b", "c"|`, 1))
	wantError(t, bad, "Function 'greet' expects 1 to 2 argument(s) but 3 given")
}

func TestUnknownNamedArgumentIsReported(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ greet ::|name:string| void:
	print|name|
$
@ main ||:
	greet|nope: "x"|
$
`)
	wantError(t, msgs, "has no parameter named 'nope'")
}

func TestKnownNamedArgumentIsNotReported(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ greet ::|name:string, greeting:string="Hi"| void:
	print|name|
$
@ main ||:
	greet|greeting: "yo"|
$
`)
	wantNoError(t, msgs, "has no parameter named")
}

func TestUndefinedVariableIsReported(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ main ||:
	print|nope|
$
`)
	wantError(t, msgs, "Variable 'nope' is not declared")
}

func TestDeclaredNamesAreNotReported(t *testing.T) {
	msgs := diagnosticsFor(`program p
LIMIT::int= 3
@ helper ::|a:int| int:
	return a
$
@ main ||:
	items: [1, 2, 3]
	total: 0
	loop n in items do
		total = total + n
	$
	loop i:0 to LIMIT do
		total = total + helper|i|
	$
	print|total|
$
`)
	wantNoError(t, msgs, "is not declared")
}

func TestBlockScopedVariableIsReportedAfterBlock(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ main ||:
	if true then
		inner: 5
	$
	print|inner|
$
`)
	wantError(t, msgs, "Variable 'inner' is not declared")
}

func TestLoopVariableIsReportedAfterLoop(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ main ||:
	items: [1, 2]
	loop n in items do
		print|n|
	$
	print|n|
$
`)
	wantError(t, msgs, "Variable 'n' is not declared")
}

func TestStringArithmeticIsReported(t *testing.T) {
	cases := []string{
		`	a: 5 + "hello"`,
		`	a: "hello" + 5`,
		`	a: "a" * 2`,
		`	a: "a" - "b"`,
		`	a: [1] + [2]`,
	}
	for _, stmt := range cases {
		msgs := diagnosticsFor("program p\n@ main ||:\n" + stmt + "\n\tprint|a|\n$\n")
		wantError(t, msgs, "cannot be applied to")
	}
}

func TestNumericArithmeticIsNotReported(t *testing.T) {
	cases := []string{
		`	a: 1.5 + 1`,
		`	a: 1 + true`,
		`	a: 1 - 2`,
		`	a: 3 * 4`,
		`	a: 6 / 2`,
		`	a: 7 % 2`,
	}
	for _, stmt := range cases {
		msgs := diagnosticsFor("program p\n@ main ||:\n" + stmt + "\n\tprint|a|\n$\n")
		wantNoError(t, msgs, "cannot be applied to")
	}
}

// TestProgramNamesResolveAcrossFiles proves a file that belongs to a
// multi-file program is checked with its siblings' declarations in scope.
func TestProgramNamesResolveAcrossFiles(t *testing.T) {
	const mainSrc = `program p
@ main ||:
	r: helper|1|
	print|r|
$
`
	// Without the program's names, the call looks undefined.
	_, errs := ahoy.ParseLintWithPath(ahoy.Tokenize(mainSrc), "main.ahoy")
	wantError(t, fatalMessages(errs), "Function 'helper' is not defined")

	// With them, it resolves.
	_, errs = ahoy.ParseLintWithPathInProgram(ahoy.Tokenize(mainSrc), "main.ahoy", []string{"helper"})
	wantNoError(t, fatalMessages(errs), "is not defined")
}
