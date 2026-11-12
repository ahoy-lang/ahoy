package main

import (
	"strings"
	"testing"

	ahoy "ahoy"
)

func compileAhoy(t *testing.T, src string) string {
	t.Helper()
	ast := ahoy.ParseWithPath(ahoy.Tokenize(src), "test.ahoy")
	return GenerateCWithFilename(ast, "test.ahoy")
}

// Operands of a fresh copy (concat, f-string) assigned into a retaining
// target must NOT be escape-marked: the copy owns its storage, the operands
// are transient reads. Marking them suppressed their scope-exit frees —
// the per-frame status-line/inspector drip (183B + 21B + 17B + 12B/frame).
func TestFreshCopyOperandsNotMarkedEscaping(t *testing.T) {
	src := `program p
@ mk || string:
	return "x" + "y"
$

@ main ||:
	a: string = mk()
	b: string = mk()
	c: string = "seed"
	c = a + b
	d: string = f"v-{a}"
	e: string = a.dup()
	f: string = mk()
	f = mk() + e
	print(c)
	print(d)
	print(e)
	print(f)
$
`
	out := compileAhoy(t, src)
	body := out
	if i := strings.Index(out, "void ahoy_main"); i >= 0 {
		body = out[i:]
	}
	if !strings.Contains(body, "free(a)") {
		t.Errorf("concat operand must still be freed at scope exit, got:\n%s", body)
	}
	if !strings.Contains(body, "free(b)") {
		t.Errorf("concat operand must still be freed at scope exit, got:\n%s", body)
	}
	if !strings.Contains(body, "free(e)") {
		t.Errorf("owned-call operand must still be freed at scope exit, got:\n%s", body)
	}
}

// Plain identifier aliases must STILL mark their source escaping (the
// target may outlive it): `c = a` leaves a untracked on purpose, so the
// owner a has to survive scope exit while the alias may still read it.
func TestPlainAliasStillMarksEscaping(t *testing.T) {
	src := `program p
@ mk || string:
	return "x" + "y"
$

@ main ||:
	a: string = mk()
	c: string = "seed"
	c = a
	print(c)
	print(a)
$
`
	out := compileAhoy(t, src)
	body := out
	if i := strings.Index(out, "void ahoy_main"); i >= 0 {
		body = out[i:]
	}
	if strings.Contains(body, "free(a)") {
		t.Errorf("plain alias must still escape-mark its source, got:\n%s", body)
	}
}

// Self-referential reassignment (`rebuilt = rebuilt + piece`) must
// evaluate the RHS before freeing the old binding: the eager free ran
// first and destroyed the memory the RHS was about to read
// (heap-use-after-free in scene_load). The fix evaluates into a temp,
// frees old, then takes over.
func TestSelfRefReassignEvaluatesBeforeFree(t *testing.T) {
	src := `program p
@ main ||:
	rebuilt: string = ""
	loop i:0 to 3 do
		qpiece: string = "x"
		rebuilt = rebuilt + qpiece
	$
	print(rebuilt)
$
`
	out := compileAhoy(t, src)
	body := out
	if i := strings.Index(out, "void ahoy_main"); i >= 0 {
		body = out[i:]
	}
	tmp := strings.Index(body, "__reassign_")
	if tmp < 0 {
		t.Errorf("self-referential reassignment must evaluate into a temp first, got:\n%s", body)
		return
	}
	concatPos := strings.Index(body[tmp:], "ahoy_string_concat(rebuilt, qpiece)")
	if concatPos < 0 {
		t.Errorf("temp must hold the concat of old binding and piece, got:\n%s", body)
		return
	}
	rest := body[tmp+concatPos:]
	freePos := strings.Index(rest, "free(rebuilt)")
	assignPos := strings.Index(rest, "rebuilt = __reassign_")
	if freePos < 0 || assignPos < 0 || freePos > assignPos {
		t.Errorf("eager free must run after RHS eval and before takeover, got:\n%s", rest)
	}
}

// A nested string concat's middle temporary must be freed by the outer
// concat: the stash decision has to recognise nested `+` results as fresh
// heap even when inference cannot see through the operand. Otherwise every
// evaluation of `a + b + c + d` leaks the middle buffer once (12 B/frame
// in the tree panel's subscene label).
func TestNestedConcatMiddleTempFreed(t *testing.T) {
	src := `program p
@ main ||:
	a: string = "n"
	b: string = "x"
	c: string = "y"
	d: string = "(" + a + " " + b + c + ")"
	print(d)
$
`
	out := compileAhoy(t, src)
	body := out
	if i := strings.Index(out, "void ahoy_main"); i >= 0 {
		body = out[i:]
	}
	// Every stashed _ct_ temp must have a matching free in the same
	// statement expression; count them textually.
	stashes := strings.Count(body, "char* _ct_")
	frees := 0
	for _, line := range strings.Split(body, ";") {
		if strings.Contains(line, "free(_ct_") {
			frees++
		}
	}
	if stashes == 0 {
		t.Errorf("expected stashed concat temps, got:\n%s", body)
		return
	}
	if frees < stashes {
		t.Errorf("every stashed concat temp must be freed (%d stashes, %d frees), got:\n%s", stashes, frees, body)
	}
}
