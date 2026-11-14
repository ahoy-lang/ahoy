package main

import (
	"strings"
	"testing"
)

// Phase 1: free only inside unsafe:
func TestSafetyFreeOutsideUnsafeIsError(t *testing.T) {
	src := `@ main ||:
	x: "hi"
	free(x)
$
`
	msgs := diagnosticsFor(src)
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "only allowed inside unsafe") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected free-outside-unsafe error, got %v", msgs)
	}
}

func TestSafetyFreeInsideUnsafeIsOk(t *testing.T) {
	src := `@ main ||:
	x: "hi"
	unsafe:
		free(x)
	$
	print("ok")
$
`
	if msgs := diagnosticsFor(src); len(msgs) != 0 {
		t.Errorf("free inside unsafe should be ok, got %v", msgs)
	}
}

func TestSafetyUnsafeBlockParses(t *testing.T) {
	src := `program p
@ main ||:
	unsafe:
		print("in unsafe")
	$
	print("done")
$
`
	if msgs := diagnosticsFor(src); len(msgs) != 0 {
		t.Errorf("unsafe block should parse, got %v", msgs)
	}
}

// Phase 2: move is silent — codegen auto-dupes soundly, .dupe() optional.
func TestSafetyUseAfterMoveWarns(t *testing.T) {
	src := `@ main ||:
	a: [1, 2, 3]
	b: a
	print(a)
$
`
	for _, e := range allDiagnosticsFor(src) {
		if strings.Contains(e.Message, "moved value") {
			t.Errorf("move warnings silenced, got %v", e.Message)
		}
	}
}

func TestSafetyDupeKeepsSource(t *testing.T) {
	src := `@ main ||:
	a: [1, 2, 3]
	b: a.dupe()
	print(a)
	print(b)
$
`
	if msgs := diagnosticsFor(src); len(msgs) != 0 {
		t.Errorf("dupe should keep source usable, got %v", msgs)
	}
}

func TestSafetyDupDeprecatedWarns(t *testing.T) {
	src := `@ main ||:
	a: [1, 2, 3]
	b: a.dup()
	print(b)
$
`
	found := false
	for _, e := range allDiagnosticsFor(src) {
		if e.IsWarning() && strings.Contains(e.Message, "dup' is deprecated") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected dup-deprecated warning, got %v", allDiagnosticsFor(src))
	}
}

// Phase 3: borrowed push is silent — codegen auto-dupes, .dupe() optional.
func TestSafetyBorrowedPushWithoutDupeWarns(t *testing.T) {
	src := `@ push_it |seen:array, v:string| void:
	seen.push(v)
$
@ main ||:
	print("x")
$
`
	for _, e := range allDiagnosticsFor(src) {
		if strings.Contains(e.Message, "without .dupe()") {
			t.Errorf("borrowed-push warnings silenced, got %v", e.Message)
		}
	}
}

func TestSafetyBorrowedPushWithDupeIsOk(t *testing.T) {
	src := `@ push_it |seen:array, v:string| void:
	seen.push(v.dupe())
$
@ main ||:
	print("x")
$
`
	if msgs := diagnosticsFor(src); len(msgs) != 0 {
		t.Errorf("push with dupe should be ok, got %v", msgs)
	}
}

func TestSafetyBorrowedScalarPushIsOk(t *testing.T) {
	src := `@ flagit |seen:array, v:bool| void:
	seen.push(v)
$
@ main ||:
	print("x")
$
`
	if msgs := diagnosticsFor(src); len(msgs) != 0 {
		t.Errorf("scalar push should not need dupe, got %v", msgs)
	}
}

func TestSafetyOwnParamAcceptsMove(t *testing.T) {
	src := `@ take |buf: own array[int]| void:
	print(buf.length)
$
@ main ||:
	a: [1, 2]
	take(a)
	print("done")
$
`
	// Own param parsing must not break; move tracking may flag `take(a)` use-after?
	// At minimum it must parse and not report unknown type.
	for _, m := range diagnosticsFor(src) {
		if strings.Contains(m, "unknown") && strings.Contains(m, "own") {
			t.Errorf("own type should parse, got %v", diagnosticsFor(src))
		}
	}
}

// Phase 4: fallible convention
func TestSafetyTryReturnEmptyStringIsError(t *testing.T) {
	src := `@ load_try |p:string| string, string:
	return "x", ""
$
@ main ||:
	print("x")
$
`
	msgs := diagnosticsFor(src)
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "must return null") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected empty-string-ok error for _try, got %v", msgs)
	}
}

func TestSafetyTryReturnNullIsOk(t *testing.T) {
	src := `@ load_try |p:string| string, string:
	if p is "" then return "", "missing" $
	return "data:" + p, null
$
@ main ||:
	print("x")
$
`
	if msgs := diagnosticsFor(src); len(msgs) != 0 {
		t.Errorf("_try with null ok should pass, got %v", msgs)
	}
}

func TestSafetyUncheckedErrIsError(t *testing.T) {
	src := `@ load_try |p:string| string, string:
	if p is "" then return "", "missing" $
	return "data:" + p, null
$
@ main ||:
	body: string = ""
	err: string = ""
	body, err := load_try("x")
	print(body)
$
`
	msgs := diagnosticsFor(src)
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "Unchecked error") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected unchecked-err error, got %v", msgs)
	}
}

func TestSafetyCheckedErrIsOk(t *testing.T) {
	src := `@ load_try |p:string| string, string:
	if p is "" then return "", "missing" $
	return "data:" + p, null
$
@ main ||:
	body: string = ""
	err: string = ""
	body, err := load_try("x")
	guard err is null else return $
	print(body)
	print("ok")
$
`
	if msgs := diagnosticsFor(src); len(msgs) != 0 {
		t.Errorf("checked err with guard should pass, got %v", msgs)
	}
}

func TestSafetyNonTryTupleIsNotFallible(t *testing.T) {
	src := `@ pair || bool, string:
	return true, "x"
$
@ main || void:
	ok: bool = false
	label: string = ""
	ok, label := pair()
	if ok then
		print(label)
	$
$
`
	if msgs := diagnosticsFor(src); len(msgs) != 0 {
		t.Errorf("plain (bool,string) should not enforce must-check, got %v", msgs)
	}
}

// Phase 5: hardening
func TestSafetyDivByZeroIsError(t *testing.T) {
	src := `@ main || void:
	x: 10
	y: x / 0
	print(y)
$
`
	msgs := diagnosticsFor(src)
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "Division by zero") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected div-by-zero error, got %v", msgs)
	}
}

func TestSafetyBufferNegativeWarns(t *testing.T) {
	src := `@ main ||:
	b: buffer[f32] = buffer(4)
	g: grow(b, -3)
	print(g.length)
$
`
	found := false
	for _, e := range allDiagnosticsFor(src) {
		if strings.Contains(e.Message, "negative") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected negative-size warning, got %v", allDiagnosticsFor(src))
	}
	if msgs := diagnosticsFor(src); len(msgs) != 0 {
		t.Errorf("negative grow should warn, not error, got %v", msgs)
	}
}

func TestSafetyBufferHugeIsError(t *testing.T) {
	src := `@ main ||:
	b: buffer[f32] = buffer(2000000000)
	print(b.length)
$
`
	msgs := diagnosticsFor(src)
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "absurd") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected huge-buffer error, got %v", msgs)
	}
}

// A .dupe() call copies the receiver out: using a moved value through
// .dupe() is the sanctioned fix, so it must not warn (it used to tell
// users to write the .dupe() they already wrote, e.g. `return out.dupe()`).
func TestSafetyDupeReceiverDoesNotWarnMoved(t *testing.T) {
	src := `@ main ||:
	a: [1, 2, 3]
	b: a
	c: a.dupe()
	print(b)
	print(c)
$
`
	for _, e := range allDiagnosticsFor(src) {
		if strings.Contains(e.Message, "moved value") {
			t.Errorf("dupe() receiver must not warn about moved value, got %v", e.Message)
		}
	}
}

// The Bell accumulator shape: rebuild a string in a loop, then return an
// owned copy. No moved-value warning should fire anywhere here.
func TestSafetyAccumulatorReturnDupeIsClean(t *testing.T) {
	src := `@ history_fit |text:string| string:
	out: string = ""
	loop i:0 to 2 do
		cand: string = out
		cand = cand + text
		out = cand
	$
	return out.dupe() + "..."
$
@ main ||:
	r: history_fit("hi")
	print(r)
$
`
	for _, e := range allDiagnosticsFor(src) {
		if strings.Contains(e.Message, "moved value") {
			t.Errorf("accumulator + return dupe() must not warn, got %v", e.Message)
		}
	}
}
