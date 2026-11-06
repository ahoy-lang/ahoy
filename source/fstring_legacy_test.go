package main

import (
	"strings"
	"testing"

	"ahoy"
)

// The legacy textual f-string path (a NODE_F_STRING with no pre-parsed
// Children, reached when a {group} fails sub-parse and in lint flows)
// must lower through the same vsnprintf-sized arena/owned helpers as the
// pre-parsed path. It used to emit `static char __fstr_buf_N[256]` +
// sprintf: truncation past 255 bytes, aliasing across evaluations of the
// same call site, and thread races.
func legacyFStringC(t *testing.T, value string) string {
	t.Helper()
	gen := &CodeGenerator{}
	node := &ahoy.ASTNode{
		Type:  ahoy.NODE_F_STRING,
		Value: value,
	}
	gen.generateFString(node)
	return gen.output.String()
}

func TestLegacyFStringUsesSizedHelper(t *testing.T) {
	out := legacyFStringC(t, "hello{name}, you are {age}.")
	if !strings.Contains(out, "ahoy_arena_sprintf(__ahoy_frame_arena,") {
		t.Errorf("legacy f-string should route through ahoy_arena_sprintf, got:\n%s", out)
	}
	if strings.Contains(out, "__fstr_buf_") {
		t.Errorf("legacy f-string must not emit a static per-site buffer, got:\n%s", out)
	}
	if strings.Contains(out, "static char") {
		t.Errorf("legacy f-string must not emit a static buffer, got:\n%s", out)
	}
	if strings.Contains(out, "sprintf(bufferVar") || strings.Contains(out, "sprintf(__fstr_buf") {
		t.Errorf("legacy f-string must not emit raw sprintf into a fixed buffer, got:\n%s", out)
	}
}

func TestLegacyFStringPlainLiteralStaysLiteral(t *testing.T) {
	out := legacyFStringC(t, "no interpolation here")
	if strings.Contains(out, "sprintf") {
		t.Errorf("interpolation-free f-string should stay a plain literal, got:\n%s", out)
	}
	if !strings.Contains(out, `"no interpolation here"`) {
		t.Errorf("expected the literal text verbatim, got:\n%s", out)
	}
}

// Dict values formatted for output (f-string interpolation, print args)
// must not pass through a shared static buffer: two values formatted in
// one expression aliased (second call overwrote the first before the
// outer call consumed it) and long values truncated at 256 bytes. The
// helpers now return vsnprintf-sized frame-arena memory.
func TestDictFormattingHasNoStaticBuffer(t *testing.T) {
	t.Helper()
	src := `program p
@ main ||:
	a: {"x": 10}
	b: {"y": 20}
	print|a<"x">, b<"y">|
	loop k, v in a do
		print|f"{k}={v}"|
	$
$
`
	ast := ahoy.ParseWithPath(ahoy.Tokenize(src), "test.ahoy")
	out := GenerateCWithFilename(ast, "test.ahoy")
	if strings.Contains(out, "static char buffer[256]") {
		t.Errorf("dict formatting must not use a static buffer:\n%s", out)
	}
	if !strings.Contains(out, "ahoy_arena_alloc(__ahoy_frame_arena") {
		t.Errorf("expected arena-sized dict formatting in output:\n%s", out)
	}
}
