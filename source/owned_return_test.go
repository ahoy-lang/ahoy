package main

import (
	"strings"
	"testing"

	ahoy "ahoy"
)

// A wrapper around an unknown (C/foreign) call must classify as borrowed:
// the callee may hand out its argument or static storage (ahoy_loc_tr
// returns its key/table cell). Callers must not free the result (arena
// game heap-use-after-free, Nov 2026). Switch-expression returns are
// static literals with the same rule. Array-access returns are promoted
// to owned copies at the return site, so their callers still free.
func TestBorrowedVsOwnedCallShapes(t *testing.T) {
	src := `program p
@ wrap |text:string| string:
    return somecall|text|
$
@ kind_icon_name |kind:int| string:
    return switch kind:
        on 1: "icon_a"
        _: "icon_b"
    $
$
@ model_name |names:array[string], index:int| string:
    if index < 0 then return "" $
    return names[index]
$
@ main ||:
    names: array[string] = ["alpha", "beta"]
    loop i:0 to 2 do
        w: wrap|names[0]|
        print|w|
        k: kind_icon_name|1|
        print|k|
        m: model_name|names, 1|
        print|m|
    $
    print|"done"|
$
`
	ast := ahoy.ParseWithPath(ahoy.Tokenize(src), "test.ahoy")
	out := GenerateCWithFilename(ast, "test.ahoy")
	if strings.Contains(out, "free(w)") {
		t.Errorf("C-passthrough wrapper result is borrowed and must not be freed")
	}
	if strings.Contains(out, "free(k)") {
		t.Errorf("switch-expression string result is static and must not be freed")
	}
	if !strings.Contains(out, "free(m)") {
		t.Errorf("array-access string return is promoted to owned and must be freed")
	}
}
