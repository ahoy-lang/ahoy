package main

import (
	"strings"
	"testing"

	"ahoy"
)

// Regression test for the editor `double free or corruption (fasttop)`
// crash (tree_panel `label`, Nov 2026): a borrowed declaration
// (`label: string = cur.name`) with a CONDITIONAL fresh reassignment
// (`label = ...concat...` inside an `if`) must not get an unconditional
// scope-exit free. The not-taken path still holds borrowed storage, so
// `free(label)` is a use-after-free of the scene node's name (observed
// via ASan as heap-use-after-free in feed_used/strcmp, freed from
// tree_panel). The taken path leaks one bounded value instead.
func TestConditionalFreshReassignSkipsBorrowedFree(t *testing.T) {
	src := `program p
struct N:
	name: string = ""
	sub: string = ""
$
@ pick |flag:bool| string:
	return "x"
$
@ main ||:
	nodes: array[N] = []
	loop i:0 to 3 do
		cur: nodes[i]
		label: string = cur.name
		if cur.sub is not "" then
			label = cur.name + " (" + cur.sub + ")"
		$
		n: label.byte_len||
		print|n|
	$
	print|"done"|
$
`
	ast := ahoy.ParseWithPath(ahoy.Tokenize(src), "test.ahoy")
	out := GenerateCWithFilename(ast, "test.ahoy")
	if strings.Contains(out, "free(label)") {
		t.Errorf("conditional fresh reassignment over a borrowed binding must not emit free(label): not-taken path holds borrowed storage")
	}
}

// The converse must keep working (Appendix A Program 2): a fresh
// reassignment over an OWNED binding (tracked from birth, e.g. scalar
// `strdup` declarations) still registers scope-exit cleanup, or every
// overwritten binding leaks.
func TestOwnedReassignStillTracked(t *testing.T) {
	src := `program p
@ main ||:
	s: string = "x"
	loop i:0 to 5 do
		s = "a" + "b"
	$
	print|s|
$
`
	ast := ahoy.ParseWithPath(ahoy.Tokenize(src), "test.ahoy")
	out := GenerateCWithFilename(ast, "test.ahoy")
	if !strings.Contains(out, "free(s)") {
		t.Errorf("fresh reassignment over an owned binding must still be freed at scope exit")
	}
}
