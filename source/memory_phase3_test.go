package main

import (
	"strings"
	"testing"

	"ahoy"
)

// Phase 3: loop-local containers must be freed per iteration (block-exit
// cleanup), split pieces with them. Reads into scalars must not pin the
// read roots as escaping.
func loopProbeC(t *testing.T) string {
	t.Helper()
	src := `program p
@ frame_work |n:int| int:
	total: 0
	loop i:0 to n do
		s: "entity-" + "x"
		parts: s.split|"-"|
		arr: [i, i + 1, i + 2]
		arr.push|i|
		total = total + arr.length + parts.length
	$
	return total
$

@ main ||:
	r: frame_work|200|
	print|r|
$
`
	ast := ahoy.ParseWithPath(ahoy.Tokenize(src), "test.ahoy")
	return GenerateCWithFilename(ast, "test.ahoy")
}

func TestLoopTempsFreedPerIteration(t *testing.T) {
	out := loopProbeC(t)
	// Split-piece array shell freed...
	if !strings.Contains(out, "free(parts->data)") {
		t.Errorf("expected per-iteration free of split-result array shell")
	}
	// ...with its pieces (owned-strings deep free), not shell-only.
	if !strings.Contains(out, "for(int __i=0; __i<parts->length; __i++)") {
		t.Errorf("expected per-iteration deep free of split pieces")
	}
	// Plain int array shell freed (no piece loop needed, but must be freed).
	if !strings.Contains(out, "free(arr->data)") {
		t.Errorf("expected per-iteration free of loop-local array")
	}
	// The concat temp is freed per iteration too.
	if !strings.Contains(out, "free(s)") {
		t.Errorf("expected per-iteration free of concat temp")
	}
}

// The assignment `total = total + arr->length + parts->length` reads heap
// roots into an int. Ints retain nothing, so this must not escape-mark
// arr/parts (which would suppress the frees above forever).
func TestScalarReadDoesNotEscapeMark(t *testing.T) {
	out := loopProbeC(t)
	frees := strings.Count(out, "free(parts->data)")
	if frees < 1 {
		t.Errorf("parts free missing: scalar read likely escape-marked parts")
	}
}

// Promotion at the boundary: assigning a static literal into an array
// that owns strings duplicates elements so later iterations never free
// static memory.
func TestLiteralIntoOwnedArrayDuplicates(t *testing.T) {
	src := `program p
@ main ||:
	expected: []
	current: array[string] = []
	p: string = "a,b"
	current = p.split|","|
	loop i:0 to 3 do
		current = ["x", "y"]
	$
	print|current.length|
	expected.push|"2"|
	print|expected|
$
`
	ast := ahoy.ParseWithPath(ahoy.Tokenize(src), "test.ahoy")
	out := GenerateCWithFilename(ast, "test.ahoy")
	if !strings.Contains(out, "strdup((char*)(intptr_t)") {
		t.Errorf("expected strdup promotion fixup for literal assigned into owned-strings array")
	}
}

// Regression: borrowed element stored into an outer variable must be
// duplicated. scene_load_line did `qpiece = hp[0]` (hp block-scoped,
// qpiece loop-scoped): the scope-exit deep free of hp left qpiece
// dangling and the next concat read freed memory (ASan heap-use-after-free,
// user-visible as a tcache double-free abort in the game).
func TestOwnedReadStoredIntoOuterVarDuplicates(t *testing.T) {
	src := `program p
@ main ||:
	expected: []
	body: "a#b c#d"
	qsegs: array[string] = body.split|" "|
	rebuilt: ""
	loop i:0 to qsegs.length do
		qpiece: qsegs[i]
		if i % 2 is 0 then
			hp: array[string] = qpiece.split|"#"|
			qpiece = hp[0]
		$
		rebuilt = rebuilt + qpiece + ";"
	$
	print|rebuilt|
	expected.push|"a;c#d;"|
	print|expected|
$
`
	ast := ahoy.ParseWithPath(ahoy.Tokenize(src), "test.ahoy")
	out := GenerateCWithFilename(ast, "test.ahoy")
	// The assignment must duplicate (hp frees its own piece at block exit).
	if !strings.Contains(out, "qpiece = strdup(") && !strings.Contains(out, "qpiece=strdup(") {
		t.Errorf("expected strdup promotion for owned-read stored into outer var")
	}
	// hp's pieces are still freed (no immortalization regression).
	if !strings.Contains(out, "for(int __i=0; __i<hp->length; __i++)") {
		t.Errorf("expected hp pieces to still be freed at block exit")
	}
}

// Region lifecycle: main creates frame+scene arenas once and destroys them.
func TestMainOwnsRegionLifecycle(t *testing.T) {
	src := `program p
@ main ||:
	print|"hi"|
$
`
	ast := ahoy.ParseWithPath(ahoy.Tokenize(src), "test.ahoy")
	out := GenerateCWithFilename(ast, "test.ahoy")
	for _, want := range []string{
		"__ahoy_frame_arena = ahoy_arena_new",
		"__ahoy_scene_arena = ahoy_arena_new",
		"ahoy_arena_destroy(__ahoy_frame_arena)",
		"ahoy_arena_destroy(__ahoy_scene_arena)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected region lifecycle %q in main", want)
		}
	}
	if strings.Contains(out, "#define free(") {
		t.Errorf("generated C must never disable free()")
	}
}
