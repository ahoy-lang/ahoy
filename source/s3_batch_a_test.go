package main

import (
	"strings"
	"testing"

	"ahoy"
)

// Tests for the S3 batch-A compiler surface: the -fast flag (skip runtime
// array bounds checks) and the double spelling of float.

func generateForTest(t *testing.T, src string) string {
	t.Helper()
	tokens := ahoy.Tokenize(formatSource(src))
	ast := ahoy.Parse(tokens)
	return generateC(ast, "test.ahoy")
}

func TestFastFlagDropsBoundsChecks(t *testing.T) {
	src := "program p\n@ main ||:\n\ta: array[int] = [1, 2]\n\tprint|a[0]|\n$\n"
	base := generateForTest(t, src)
	if !strings.Contains(base, "bounds violation") {
		t.Fatalf("expected bounds check in default output")
	}
	old := noBoundsChecks
	noBoundsChecks = true
	defer func() { noBoundsChecks = old }()
	fast := generateForTest(t, src)
	if strings.Contains(fast, "bounds violation") {
		t.Errorf("expected no bounds checks with -fast, got:\n%s", fast)
	}
}

func TestDoubleSpellingParses(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ main ||:
	d: double(5)
	print|d|
$
`)
	wantNoError(t, msgs, "not declared")
}
