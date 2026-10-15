package main

import (
	"os"
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
	_, errs = ahoy.ParseLintWithPathInProgram(ahoy.Tokenize(mainSrc), "main.ahoy", []string{"helper"}, nil)
	wantNoError(t, fatalMessages(errs), "is not defined")
}

// allDiagnosticsFor returns every diagnostic, warnings included.
func allDiagnosticsFor(src string) []ahoy.ParseError {
	_, errs := ahoy.ParseLintWithPath(ahoy.Tokenize(src), "test.ahoy")
	return errs
}

func TestIsNotIsAccepted(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ main ||:
	x: 1
	y: 2
	if x is not y then print|"ne"| $
$
`)
	if len(msgs) != 0 {
		t.Errorf("expected no diagnostics for 'is not', got %v", msgs)
	}
}

func TestNotIsIsReported(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ main ||:
	x: 1
	y: 2
	if x not is y then print|"ne"| $
$
`)
	wantError(t, msgs, "use 'is not' instead of 'not is'")
}

func TestInferReturnTypeIsReported(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ calc ::|x:int| infer:
	return x
$
`)
	wantError(t, msgs, "'infer' is not allowed")
}

func TestNestedCallArgumentIsReported(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ g ::|n:int| int:
	return n
$
@ main ||:
	a: g|g|1||
	print|a|
$
`)
	wantError(t, msgs, "is not allowed as an argument")
}

// A cast is not a call, so int(x)/float(x)/char(x)/string(x) stay legal as an
// argument even though the parser builds a NODE_CALL for them.
func TestCastArgumentIsNotReported(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ take ::|n:int| void:
	print|n|
$
@ main ||:
	v: 3
	pct: 0.5
	take|int(float(v) * pct)|
$
`)
	wantNoError(t, msgs, "is not allowed as an argument")
	wantNoError(t, msgs, "is not defined")
}

// Constants declared in main are emitted as global constants, so functions can
// reference them.
func TestMainConstantIsVisibleInFunctions(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ helper ::|| int:
	return OFFSET + 1
$
@ main ||:
	OFFSET: int = 4
	print|helper||
$
`)
	wantNoError(t, msgs, "is not declared")
}

// ParseWithPath is the code-generation parser. A call with a bare identifier
// argument used inside a larger expression used to panic there while linting
// accepted it (`if g|v| then`, `g|v| + 1`).
func TestCallInExpressionParsesWithoutPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ParseWithPath panicked: %v", r)
		}
	}()
	ast := ahoy.ParseWithPath(ahoy.Tokenize(`program p
@ g ::|n:int| int:
	return n
$
@ main ||:
	v: 3
	a: g|v| + 1
	if g|v| > 2 then print|a| $
$
`), "test.ahoy")
	if ast == nil {
		t.Fatal("expected an AST")
	}
}

func TestPartialMultiReturnIsWarned(t *testing.T) {
	errs := allDiagnosticsFor(`program p
@ pair ::|n:int| int,string:
	return n, "s"
$
@ main ||:
	a: pair|1|
	print|a|
$
`)
	warned := false
	for _, e := range errs {
		if e.IsWarning() && strings.Contains(e.Message, "returns 2 values but only 1 is captured") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("expected a warning about capturing 1 of 2 returned values, got %v", errs)
	}
	// And the warning must not be fatal.
	wantNoError(t, fatalMessages(errs), "returns 2 values")
}

func TestFullMultiReturnIsNotWarned(t *testing.T) {
	errs := allDiagnosticsFor(`program p
@ pair ::|n:int| int,string:
	return n, "s"
$
@ main ||:
	a, b : pair|1|
	print|a, b|
$
`)
	for _, e := range errs {
		if strings.Contains(e.Message, "values but") {
			t.Errorf("did not expect a multi-return warning, got %q", e.Message)
		}
	}
}

// A loop's own variable is in scope in its condition: `loop i:0 till i < 5`
// must not report `i` as undeclared.
func TestLoopVariableIsInScopeInCondition(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ main ||:
	loop i:0 till i < 5 do
		print|i|
	$
$
`)
	wantNoError(t, msgs, "is not declared")
}

func TestBitwiseOnIntegersIsNotReported(t *testing.T) {
	cases := []string{
		`	a: 5 band 3`,
		`	a: 5 bor 3`,
		`	a: 5 bxor 3`,
		`	a: bnot 5`,
		`	a: 1 shl 4`,
		`	a: 16 shr 2`,
	}
	for _, stmt := range cases {
		msgs := diagnosticsFor("program p\n@ main ||:\n" + stmt + "\n\tprint|a|\n$\n")
		wantNoError(t, msgs, "requires integers")
	}
}

func TestBitwiseOnNonIntegerIsReported(t *testing.T) {
	cases := []string{
		`	a: 1.5 band 2`,
		`	a: "x" bor 1`,
		`	a: bnot 1.5`,
	}
	for _, stmt := range cases {
		msgs := diagnosticsFor("program p\n@ main ||:\n" + stmt + "\n\tprint|a|\n$\n")
		wantError(t, msgs, "requires")
	}
}

func TestFunctionTypedVariableIsCallable(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ double_it |x:int| int:
	return x * 2
$
@ main ||:
	f: fn(int)->int = &double_it
	r: f|3|
	print|r|
$
`)
	wantNoError(t, msgs, "is not defined")
}

func TestFunctionValueParameterIsCallable(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ double_it |x:int| int:
	return x * 2
$
@ apply |f:fn(int)->int, v:int| int:
	return f|v|
$
@ main ||:
	r: apply|&double_it, 3|
	print|r|
$
`)
	wantNoError(t, msgs, "is not defined")
}

func TestLambdaCaptureIsResolved(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ main ||:
	base: 10
	f: fn(int)->int = (x:int): x + base
	r: f|1|
	print|r|
$
`)
	wantNoError(t, msgs, "is not declared")
	wantNoError(t, msgs, "is not defined")
}

// TestLintSeesDirectoryImports mirrors the -lint whole-program pass: a main
// file importing a directory (the bell engine layout) must resolve the
// imported names instead of reporting every engine call as undefined.
func TestLintSeesDirectoryImports(t *testing.T) {
	dir := t.TempDir()
	engineDir := dir + "/engine"
	if err := os.Mkdir(engineDir, 0755); err != nil {
		t.Fatal(err)
	}
	engineSrc := "program eng\n@ engine_fn |x:int| int:\n\treturn x\n$\n"
	if err := os.WriteFile(engineDir+"/engine.ahoy", []byte(engineSrc), 0644); err != nil {
		t.Fatal(err)
	}
	mainSrc := "program p\nimport \"./engine\"\n@ main ||:\n\tr: engine_fn|1|\n\tprint|r|\n$\n"
	mainPath := dir + "/main.ahoy"
	if err := os.WriteFile(mainPath, []byte(mainSrc), 0644); err != nil {
		t.Fatal(err)
	}
	pm := NewPackageManager(dir)
	pm.ValidateOnLoad = false
	pkg, err := pm.LoadPackageFromFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	imports, err := resolveImports(pkg, pm, mainPath)
	if err != nil {
		t.Fatal(err)
	}
	packages := collectPackages(pkg, imports)
	allNames := map[string]bool{}
	for _, p := range packages {
		for _, file := range p.Files {
			for _, name := range ahoy.DeclaredNames(file.AST) {
				allNames[name] = true
			}
		}
	}
	programNames := make([]string, 0, len(allNames))
	for name := range allNames {
		programNames = append(programNames, name)
	}
	var methodASTs []*ahoy.ASTNode
	for _, p := range packages {
		for i := range p.Files {
			if p.Files[i].AST != nil {
				methodASTs = append(methodASTs, p.Files[i].AST)
			}
		}
	}
	programMethods := ahoy.CollectMethods(methodASTs...)
	var msgs []string
	for _, p := range packages {
		for _, file := range p.Files {
			_, errs := ahoy.ParseLintWithPathInProgram(file.lintTokens(), file.Path, programNames, programMethods)
			msgs = append(msgs, fatalMessages(errs)...)
		}
	}
	wantNoError(t, msgs, "engine_fn")
}

// TestLintStillCatchesUndefinedAcrossImports proves the test above is not
// vacuous: an unknown name through the same import layout must be reported.
func TestLintStillCatchesUndefinedAcrossImports(t *testing.T) {
	dir := t.TempDir()
	engineDir := dir + "/engine"
	if err := os.Mkdir(engineDir, 0755); err != nil {
		t.Fatal(err)
	}
	engineSrc := "program eng\n@ engine_fn |x:int| int:\n\treturn x\n$\n"
	if err := os.WriteFile(engineDir+"/engine.ahoy", []byte(engineSrc), 0644); err != nil {
		t.Fatal(err)
	}
	mainSrc := "program p\nimport \"./engine\"\n@ main ||:\n\tr: missing_fn_xyz|1|\n\tprint|r|\n$\n"
	mainPath := dir + "/main.ahoy"
	if err := os.WriteFile(mainPath, []byte(mainSrc), 0644); err != nil {
		t.Fatal(err)
	}
	pm := NewPackageManager(dir)
	pm.ValidateOnLoad = false
	pkg, err := pm.LoadPackageFromFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	imports, err := resolveImports(pkg, pm, mainPath)
	if err != nil {
		t.Fatal(err)
	}
	packages := collectPackages(pkg, imports)
	allNames := map[string]bool{}
	for _, p := range packages {
		for _, file := range p.Files {
			for _, name := range ahoy.DeclaredNames(file.AST) {
				allNames[name] = true
			}
		}
	}
	programNames := make([]string, 0, len(allNames))
	for name := range allNames {
		programNames = append(programNames, name)
	}
	var methodASTs []*ahoy.ASTNode
	for _, p := range packages {
		for i := range p.Files {
			if p.Files[i].AST != nil {
				methodASTs = append(methodASTs, p.Files[i].AST)
			}
		}
	}
	programMethods := ahoy.CollectMethods(methodASTs...)
	var msgs []string
	for _, p := range packages {
		for _, file := range p.Files {
			_, errs := ahoy.ParseLintWithPathInProgram(file.lintTokens(), file.Path, programNames, programMethods)
			msgs = append(msgs, fatalMessages(errs)...)
		}
	}
	wantError(t, msgs, "missing_fn_xyz")
}

const methodsPrelude = `struct Point:
	x: float
	y: float
$

@ Point.make |x:float, y:float| Point:
	return Point{x: x, y: y}
$

@ (p Point) move |dx:float, dy:float| Point:
	p.x = p.x + dx
	p.y = p.y + dy
	return p
$

@ (p Point) sum || float:
	return p.x + p.y
$
`

func TestUserMethodsValid(t *testing.T) {
	src := methodsPrelude + `@ main || void:
	p: Point.make|1.0, 2.0|
	q: p.move|3.0, 4.0|
	s: q.sum||
	print|s|
$
`
	wantNoError(t, diagnosticsFor(src), "method")
	wantNoError(t, diagnosticsFor(src), "Point_move")
}

func TestUserMethodsUnknown(t *testing.T) {
	src := methodsPrelude + `@ main || void:
	p: Point{x: 1.0, y: 2.0}
	q: p.nudge|3.0|
	print|q|
$
`
	wantError(t, diagnosticsFor(src), "has no method 'nudge'")
}

func TestUserMethodsArity(t *testing.T) {
	src := methodsPrelude + `@ main || void:
	p: Point{x: 1.0, y: 2.0}
	q: p.move|3.0|
	print|q|
$
`
	wantError(t, diagnosticsFor(src), "expects 3 argument")
}

func TestUserMethodsAssociatedUnknown(t *testing.T) {
	src := methodsPrelude + `@ main || void:
	p: Point.x|1.0|
	print|p|
$
`
	wantError(t, diagnosticsFor(src), "has no associated function 'x'")
}

func TestUserMethodsDuplicate(t *testing.T) {
	src := methodsPrelude + `@ (p Point) move |dx:float| Point:
	return p
$

@ main || void:
	print|"hi"|
$
`
	wantError(t, diagnosticsFor(src), "Duplicate definition of 'Point.move'")
}

func TestUserMethodsUnknownReceiver(t *testing.T) {
	src := `struct Point:
	x: float
$

@ (p Nope) foo || int:
	return 1
$

@ main || void:
	print|"hi"|
$
`
	wantError(t, diagnosticsFor(src), "Unknown type 'Nope'")
}

func TestUserMethodsDynamicReceiverStaysPermissive(t *testing.T) {
	src := `@ f |x| int:
	q: x.nudge|3.0|
	return 1
$

@ main || void:
	print|"hi"|
$
`
	wantNoError(t, diagnosticsFor(src), "has no method")
}

func TestUserMethodsAcrossFiles(t *testing.T) {
	engineSrc := "program eng\n" + methodsPrelude
	mainSrc := `program eng
@ main || void:
	p: Point{x: 1.0, y: 2.0}
	q: p.move|3.0, 4.0|
	print|q|
$
`
	engineAST, _ := ahoy.ParseLintWithPath(ahoy.Tokenize(engineSrc), "engine.ahoy")
	methods := ahoy.CollectMethods(engineAST)
	_, errs := ahoy.ParseLintWithPathInProgram(ahoy.Tokenize(mainSrc), "main.ahoy",
		[]string{"Point", "Point_make", "Point_move", "Point_sum"}, methods)
	wantNoError(t, fatalMessages(errs), "has no method")
	wantNoError(t, fatalMessages(errs), "expects")

	badSrc := `program eng
@ main || void:
	p: Point{x: 1.0, y: 2.0}
	q: p.nudge|3.0|
	print|q|
$
`
	_, errs = ahoy.ParseLintWithPathInProgram(ahoy.Tokenize(badSrc), "main.ahoy",
		[]string{"Point", "Point_make", "Point_move", "Point_sum"}, methods)
	wantError(t, fatalMessages(errs), "has no method 'nudge'")
}

const fnrefPrelude = `struct Renderer:
	clear: fn(int,int,int)->int
	draw: fn(int,int)->int
	name: string
$

@ raylib_clear |r:int, g:int, b:int| int:
	return 100
$

@ sdl_draw |x:int| int:
	return 400
$
`

func TestFnRefMismatchInLiteral(t *testing.T) {
	src := fnrefPrelude + `@ main || void:
	s: Renderer{clear: &raylib_clear, draw: &sdl_draw, name: "sdl"}
	print|s.name|
$
`
	warned := false
	for _, e := range allDiagnosticsFor(src) {
		if e.IsWarning() && strings.Contains(e.Message, "takes only 1 parameter") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("expected a fewer-parameters warning, got %v", allDiagnosticsFor(src))
	}
	wantNoError(t, diagnosticsFor(src), "takes only")
}

func TestFnRefMatchInLiteral(t *testing.T) {
	src := fnrefPrelude + `@ sdl_draw2 |x:int, y:int| int:
	return x + y
$

@ main || void:
	s: Renderer{clear: &raylib_clear, draw: &sdl_draw2, name: "sdl"}
	print|s.name|
$
`
	wantNoError(t, diagnosticsFor(src), "Function reference")
}

func TestFnRefMissingField(t *testing.T) {
	src := fnrefPrelude + `@ main || void:
	r: Renderer{clear: &raylib_clear, name: "raylib"}
	print|r.name|
$
`
	wantError(t, diagnosticsFor(src), "missing function field 'draw'")
}

func TestLiteralUnknownFieldInferred(t *testing.T) {
	// Unknown fields are rejected in inferred literals too (returns,
	// elements, arguments), not just explicitly annotated declarations.
	src := fnrefPrelude + `@ make || Renderer:
	return Renderer{clear: &raylib_clear, name: "x", badfield: 1}
$

@ main || void:
	r: make||
	print|r.name|
$
`
	wantError(t, diagnosticsFor(src), "'badfield' does not exist in type 'Renderer'")
}

func TestFnRefArgMismatch(t *testing.T) {
	src := `@ takes_cb |cb:fn(int,int)->int| int:
	return cb|1, 2|
$

@ one_arg |x:int| int:
	return x
$

@ main || void:
	y: takes_cb|&one_arg|
	print|y|
$
`
	warned := false
	for _, e := range allDiagnosticsFor(src) {
		if e.IsWarning() && strings.Contains(e.Message, "takes only 1 parameter") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("expected a fewer-parameters warning, got %v", allDiagnosticsFor(src))
	}
	wantNoError(t, diagnosticsFor(src), "takes only")
}

func TestFnRefNeedsMoreArgs(t *testing.T) {
	src := `@ takes_cb |cb:fn(int,int)->int| int:
	return cb|1, 2|
$

@ main || void:
	y: takes_cb|&needs_two|
	print|y|
$

@ needs_two |a:int, b:int, c:int| int:
	return a + b + c
$
`
	wantError(t, diagnosticsFor(src), "takes at least 3 argument")
}

func TestFnRefReturnMismatch(t *testing.T) {
	src := `@ takes_cb |cb:fn(int)->int| int:
	return cb|1|
$

@ no_ret |x:int| void:
	print|x|
$

@ main || void:
	y: takes_cb|&no_ret|
	print|y|
$
`
	wantError(t, diagnosticsFor(src), "returns void but slot")
}

// If-chains are valid code: no switch suggestion (removed warning).
func TestRepeatedIfSuggestsSwitch(t *testing.T) {
	src := `@ shape_name |shape:int| string:
	if shape is 1 then
		return "circle"
	$
	if shape is 2 then
		return "line"
	$
	if shape is 3 then
		return "outline"
	$
	return "rect"
$
`
	for _, e := range allDiagnosticsFor(src) {
		if strings.Contains(e.Message, "consider 'switch") {
			t.Errorf("did not expect a switch suggestion, got %q", e.Message)
		}
	}
}

func TestRepeatedIfShortChainSilent(t *testing.T) {
	src := `@ f |a:int| string:
	if a is 1 then
		return "one"
	$
	if a is 2 then
		return "two"
	$
	return "other"
$
`
	for _, e := range allDiagnosticsFor(src) {
		if strings.Contains(e.Message, "consider 'switch") {
			t.Errorf("did not expect a switch suggestion for two checks, got %q", e.Message)
		}
	}
}

func TestRepeatedIfMixedSilent(t *testing.T) {
	src := `@ f |a:int, b:int| string:
	if a is 1 then
		return "one"
	$
	if b is 1 then
		return "bee-one"
	$
	if a is not 2 then
		return "not-two"
	$
	if a is 3 and b is 1 then
		return "mixed"
	$
	return "other"
$
`
	for _, e := range allDiagnosticsFor(src) {
		if strings.Contains(e.Message, "consider 'switch") {
			t.Errorf("did not expect a switch suggestion for mixed checks, got %q", e.Message)
		}
	}
}

func TestStoreWarnPushHeap(t *testing.T) {
	src := `@ main || void:
	outer: []
	loop i:0 to 10 do
		inner: [i]
		outer.push|inner|
	$
	print|outer.length|
$
`
	// Array push intentionally doesn't warn (elements may be static or
	// caller-owned; leak hunting is -show_leaks' job).
	for _, e := range allDiagnosticsFor(src) {
		if strings.Contains(e.Message, "does not free") {
			t.Errorf("did not expect a push-store warning, got %q", e.Message)
		}
	}
}

func TestStoreNoWarnPushPrimitives(t *testing.T) {
	src := `@ main || void:
	outer: []
	loop i:0 to 10 do
		outer.push|i|
	$
	names: []
	names.push|"hi"|
	print|outer.length|
	print|names.length|
$
`
	for _, e := range allDiagnosticsFor(src) {
		if strings.Contains(e.Message, "does not free") {
			t.Errorf("did not expect a push warning, got %q", e.Message)
		}
	}
}

func TestStoreWarnFieldHeap(t *testing.T) {
	src := `struct Card:
	text: string
	count: int
$

@ main || void:
	n: Card{text: "", count: 0}
	n.text = n.text + "!"
	print|n.text|
$
`
	// Field/dict overwrites intentionally don't warn (autofree frees the
	// old value wherever it can prove it safe; the rest is the safe
	// direction and not nagged about).
	for _, e := range allDiagnosticsFor(src) {
		if strings.Contains(e.Message, "previous contents") {
			t.Errorf("did not expect a field-store warning, got %q", e.Message)
		}
	}
}

func TestStoreNoWarnFieldLiteral(t *testing.T) {
	src := `struct Card:
	text: string
	count: int
$

@ main || void:
	n: Card{text: "", count: 0}
	n.text = "hello"
	n.count = 5
	print|n.text|
$
`
	for _, e := range allDiagnosticsFor(src) {
		if strings.Contains(e.Message, "previous contents") {
			t.Errorf("did not expect a field-store warning, got %q", e.Message)
		}
	}
}
