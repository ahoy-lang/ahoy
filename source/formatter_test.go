package main

import (
	"strings"
	"testing"
)

// Tests for the source formatter. The rules are documented on formatSource:
// two-space indentation, a `$` at the level of the line that opened the block,
// a single-statement block joined onto one line when it fits, comments kept,
// and spacing normalised.

func expectFormat(t *testing.T, name, input, want string) {
	t.Helper()
	got := formatSource(input)
	if got != want {
		t.Errorf("%s:\n--- want ---\n%s\n--- got ---\n%s", name, want, got)
	}
}

func TestFormatterIndentsFunctionBody(t *testing.T) {
	expectFormat(t, "function body", `@ greet ::|name:string|:
ahoy|"Hello"|
$
`, `@ greet::|name: string|:
  ahoy|"Hello"|
$
`)
}

func TestFormatterIndentsNestedBlocks(t *testing.T) {
	expectFormat(t, "nested", `@ check ::|num:int|:
if num > 0 then
loop i:0 to num do
ahoy|i|
$
$
$
`, `@ check::|num: int|:
  if num > 0 then
    loop i:0 to num do
      ahoy|i|
    $
  $
$
`)
}

func TestFormatterKeepsDollarAtOpeningLevel(t *testing.T) {
	// The short if joins onto one line; the loop cannot, so its `$` stays at the
	// level of the line that opened it.
	expectFormat(t, "closers", `@ f ||:
if true then
ahoy|"a"|
$
loop i:0 to 1 do
ahoy|i|
$
$
`, `@ f ||:
  if true then ahoy|"a"| $
  loop i:0 to 1 do
    ahoy|i|
  $
$
`)
}

func TestFormatterCollapsesShortSingleStatementIf(t *testing.T) {
	expectFormat(t, "collapse if", `@ f ||:
if x > 0 then
print|"pos"|
$
$
`, `@ f ||:
  if x > 0 then print|"pos"| $
$
`)
}

func TestFormatterExpandsLongSingleStatementIf(t *testing.T) {
	long := strings.Repeat("a", formatterMaxWidth)
	input := "@ f ||:\nif x > 0 then\nprint|\"" + long + "\"|\n$\n$\n"
	got := formatSource(input)
	if strings.Contains(got, "then print|") {
		t.Errorf("a block longer than %d columns should not be joined:\n%s", formatterMaxWidth, got)
	}
	if !strings.Contains(got, "  if x > 0 then\n    print|") {
		t.Errorf("expected the body on its own indented line, got:\n%s", got)
	}
}

func TestFormatterNeverCollapsesLoops(t *testing.T) {
	// A `$` on the same line as a one-line loop is a parse error.
	got := formatSource("@ f ||:\nloop i:0 to 2 do\nprint|i|\n$\n$\n")
	if strings.Contains(got, "do print|") {
		t.Errorf("loops must not be collapsed:\n%s", got)
	}
}

func TestFormatterNeverCollapsesVoidFunctionWithBody(t *testing.T) {
	// The parser rejects `void: print|a| $`, so the `$` stays on its own line.
	got := formatSource("@ f |a:int| void:\nprint|a|\n$\n")
	if strings.Contains(got, "void: print|a| $") {
		t.Errorf("a void function must not be collapsed with a trailing $:\n%s", got)
	}
	if !strings.Contains(got, "  print|a|\n$\n") {
		t.Errorf("expected the body on its own line, got:\n%s", got)
	}
}

func TestFormatterKeepsSwitchCasesIndented(t *testing.T) {
	expectFormat(t, "switch", `@ f ||:
switch x:
on 1:
print|"one"|
on 2: print|"two"|
_:
print|"other"|
$
$
`, `@ f ||:
  switch x:
    on 1:
      print|"one"|
    on 2: print|"two"|
    _:
      print|"other"|
  $
$
`)
}

func TestFormatterKeepsElseAtIfLevel(t *testing.T) {
	// `else` must not be indented past its `if`, and its short body joins.
	expectFormat(t, "else", `@ f ||:
if a then
print|"a"|
else
print|"b"|
$
$
`, `@ f ||:
  if a then
    print|"a"|
  else print|"b"| $
$
`)
}

func TestFormatterNormalisesSpacing(t *testing.T) {
	expectFormat(t, "spacing", `@ f ||:
x:1
y : 2
z: 3+4
w: a,b
count:array[int]= [1,2]
$
`, `@ f ||:
  x: 1
  y: 2
  z: 3 + 4
  w: a, b
  count:array[int]= [1, 2]
$
`)
}

func TestFormatterKeepsCompoundAssignments(t *testing.T) {
	expectFormat(t, "compound", `@ f ||:
x: 0
x += 1
x -= 2
x *= 3
$
`, `@ f ||:
  x: 0
  x += 1
  x -= 2
  x *= 3
$
`)
}

func TestFormatterWritesLoopCounterWithoutSpace(t *testing.T) {
	got := formatSource("@ f ||:\nloop i:0 to 2 do\nprint|i|\n$\n$\n")
	if !strings.Contains(got, "loop i:0 to 2 do") {
		t.Errorf("a loop counter keeps its colon tight, got:\n%s", got)
	}
}

func TestFormatterPreservesComments(t *testing.T) {
	expectFormat(t, "comments", `? leading comment
@ f ||:
? inside the body
x: 1 ? trailing
$
`, `? leading comment
@ f ||:
  ? inside the body
  x: 1 ? trailing
$
`)
}

func TestFormatterCollapsesBlankRuns(t *testing.T) {
	expectFormat(t, "blank lines", `@ f ||:


x: 1



y: 2


$
`, `@ f ||:

  x: 1

  y: 2
$
`)
}

func TestFormatterDropsLeadingAndTrailingBlankLines(t *testing.T) {
	expectFormat(t, "edges", "\n\n@ f ||:\n  x: 1\n$\n\n\n", "@ f ||:\n  x: 1\n$\n")
}

func TestFormatterConvertsTabs(t *testing.T) {
	got := formatSource("@ f ||:\n\tx: 1\n$\n")
	if strings.Contains(got, "\t") {
		t.Errorf("tabs must be replaced by spaces, got %q", got)
	}
	if !strings.Contains(got, "  x: 1") {
		t.Errorf("expected two-space indentation, got %q", got)
	}
}

func TestFormatterEndsWithSingleNewline(t *testing.T) {
	got := formatSource("@ f ||:\n  x: 1\n$")
	if !strings.HasSuffix(got, "$\n") || strings.HasSuffix(got, "\n\n") {
		t.Errorf("expected exactly one trailing newline, got %q", got)
	}
}

func TestFormatterIsIdempotent(t *testing.T) {
	inputs := []string{
		"@ f ||:\nif a then\nprint|1|\n$\n$\n",
		"@ f ||:\nswitch x:\non 1:\nprint|1|\n$\n$\n",
		"? c\n@ f ||:\nx:1\nloop i:0 to 2 do\nprint|i|\n$\n$\n",
		"@ f |a:int| void:\nprint|a|\n$\n",
		"@ f ||:\nx += 1\ny:array[int]= [1,2]\n$\n",
	}
	for _, input := range inputs {
		once := formatSource(input)
		twice := formatSource(once)
		if once != twice {
			t.Errorf("formatting is not idempotent:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
		}
	}
}

func TestFormatterKeepsStringsIntact(t *testing.T) {
	// Spacing rules must not reach inside string literals.
	got := formatSource("@ f ||:\n  a: \"x  ,  y\"\n  b: f\"{a}+{a}\"\n$\n")
	if !strings.Contains(got, `"x  ,  y"`) {
		t.Errorf("a string literal was rewritten, got:\n%s", got)
	}
	if !strings.Contains(got, `f"{a}+{a}"`) {
		t.Errorf("an f-string was rewritten, got:\n%s", got)
	}
}

func TestFormatterKeepsCommentTextIntact(t *testing.T) {
	got := formatSource("@ f ||:\n  ? keep   this   spacing\n$\n")
	if !strings.Contains(got, "? keep   this   spacing") {
		t.Errorf("comment text was rewritten, got:\n%s", got)
	}
}
