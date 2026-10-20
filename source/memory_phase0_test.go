package main

import (
	"strings"
	"testing"

	"ahoy"
)

// Phase 0 regression: generated C must never disable free().
// The per-function arena attempt emitted `#define free(p) ((void)0)`,
// which silently turned every autofree/manual free into a leak.
func TestGeneratedCDoesNotDisableFree(t *testing.T) {
	src := `program p
@ main ||:
	print|"hi"|
$
`
	toks := ahoy.Tokenize(src)
	ast := ahoy.ParseWithPath(toks, "test.ahoy")
	out := GenerateCWithFilename(ast, "test.ahoy")
	if strings.Contains(out, "#define free(") {
		t.Errorf("generated C disables free(); autofree and manual free would silently leak")
	}
	// Arena runtime must still be present (kept for frame/scene phases).
	if !strings.Contains(out, "ahoy_arena_alloc") {
		t.Errorf("expected arena runtime in generated C")
	}
	// Per-function 64KB arenas must be gone (Phase 0 removal).
	if strings.Contains(out, "__ahoy_arena = ahoy_arena_new(1 << 16)") {
		t.Errorf("per-function arena prologue should have been removed in Phase 0")
	}
}

// Owned-heap growth must use realloc (frees old buffer), not
// malloc+memcpy without freeing (leaks on every array growth).
func TestStdlibGrowthUsesRealloc(t *testing.T) {
	push := ArrayMethods["push"].Code
	if strings.Contains(push, "__new_data") || strings.Contains(push, "__old_cap") {
		t.Errorf("array push growth leaks old buffer; must use realloc")
	}
	if !strings.Contains(push, "realloc") {
		t.Errorf("array push growth should use realloc")
	}
	fill := ArrayMethods["fill"].Code
	if !strings.Contains(fill, "realloc") {
		t.Errorf("array fill growth should use realloc")
	}
}
