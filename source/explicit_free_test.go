package main

import (
	"strings"
	"testing"

	ahoy "ahoy"
)

// Explicit early disposal: free|s| on a tracked string local must emit
// exactly one free, null the slot, and suppress the scope-exit free.
// (Bell live-sync discard path: live_text is owned on every path and the
// element store duplicates into the array, so the original must be
// released exactly once.)
func TestExplicitFreeDisposesOnce(t *testing.T) {
	src := `program p
@ main ||:
	loop i:0 to 2 do
		s: "a" + "b"
		if s is not "" then
			print|s|
		$
		free|s|
	$
	print|"done"|
$
`
	ast := ahoy.ParseWithPath(ahoy.Tokenize(src), "test.ahoy")
	out := GenerateCWithFilename(ast, "test.ahoy")
	if !strings.Contains(out, "free(s); s = 0;") {
		t.Errorf("explicit free|s| on a tracked string must free and null it")
	}
	if strings.Count(out, "free(s)") != 1 {
		t.Errorf("tracked string with explicit free must be freed exactly once, got:\n%s", out)
	}
}

// free|x| on an untracked (borrowed/C) pointer keeps the plain free with
// no ownership bookkeeping.
func TestExplicitFreeUntrackedStaysRaw(t *testing.T) {
	src := `program p
@ main ||:
	p: somecall||
	free|p|
	print|"done"|
$
`
	ast := ahoy.ParseWithPath(ahoy.Tokenize(src), "test.ahoy")
	out := GenerateCWithFilename(ast, "test.ahoy")
	if !strings.Contains(out, "free(p)") {
		t.Errorf("explicit free on untracked pointer must stay a plain free")
	}
	if strings.Contains(out, "p = 0;") {
		t.Errorf("untracked free must not null or untrack anything")
	}
}
