package ahoy

import (
	"os"
	"path/filepath"
	"testing"
)

// A C callback typedef followed by a trailing comment must still parse, and the
// parameter type has to be preserved: the code generator needs it to hand a raw
// C function pointer to that parameter.
func TestCallbackTypedefIsParsed(t *testing.T) {
	dir := t.TempDir()
	header := "typedef void (*LogCallback)(int level, const char *text);  // trailing comment\n" +
		"typedef unsigned char *(*LoadCallback)(const char *name, int *size);\n" +
		"void set_logger(LogCallback cb);\n"
	path := filepath.Join(dir, "callbacks.h")
	if err := os.WriteFile(path, []byte(header), 0644); err != nil {
		t.Fatal(err)
	}

	info, err := ParseCHeader(path)
	if err != nil {
		t.Fatal(err)
	}

	td, ok := info.FuncPtrTypedefs["LogCallback"]
	if !ok {
		t.Fatalf("callback typedef not parsed: %v", info.FuncPtrTypedefs)
	}
	if td.ReturnType != "void" || len(td.Parameters) != 2 {
		t.Errorf("LogCallback signature wrong: %+v", td)
	}
	if _, ok := info.FuncPtrTypedefs["LoadCallback"]; !ok {
		t.Errorf("pointer-returning callback typedef not parsed")
	}

	f := info.Functions["set_logger"]
	if f == nil || len(f.Parameters) != 1 || f.Parameters[0].Type != "LogCallback" {
		t.Errorf("callback parameter type not preserved: %+v", f)
	}
}

// A call split across two lines must not register its second line as a
// declaration: the argument list used to become the callee's return type,
// so every variable declared from that call carried the argument list as
// its type (real case: a two-line fprintf in bell's perf sidecar poisoned
// ahoy_vram_total_kb).
func TestMultilineCallIsNotADeclaration(t *testing.T) {
	dir := t.TempDir()
	header := "static int real_total_kb(void);\n" +
		"static void writer(const char* path) {\n" +
		"    FILE *f;\n" +
		"    f = fopen(path, \"w\");\n" +
		"    fprintf(f, \"{\\\"frames\\\":%ld}\\n\",\n" +
		"        frame_no, fps, real_total_kb());\n" +
		"    fclose(f);\n" +
		"}\n" +
		"static int real_total_kb(void) {\n" +
		"    return 42;\n" +
		"}\n"
	path := filepath.Join(dir, "multi.h")
	if err := os.WriteFile(path, []byte(header), 0644); err != nil {
		t.Fatal(err)
	}

	info, err := ParseCHeader(path)
	if err != nil {
		t.Fatal(err)
	}

	f, ok := info.Functions["real_total_kb"]
	if !ok {
		t.Fatalf("real declaration missing: %v", info.Functions)
	}
	if f.ReturnType != "int" {
		t.Errorf("return type poisoned by call fragment: %q", f.ReturnType)
	}
	if _, ok := info.Functions["frame_no"]; ok {
		t.Errorf("call fragment registered as function")
	}
}

// A continuation line of a multi-line statement whose first ")" comes
// before any "(" must be skipped, not sliced: it used to panic the parser
// with a backwards slice ([51:26]) and would otherwise register the
// trailing call as a function.
func TestContinuationFragmentWithCloseParenIsSkipped(t *testing.T) {
	dir := t.TempDir()
	header := "static int ok_fn(void);\n" +
		"static void frag_user(int slot) {\n" +
		"    if (!table[slot].pos || !table[slot].nrm ||\n" +
		"        !table[slot].tex) return commit(slot, data);\n" +
		"}\n"
	path := filepath.Join(dir, "frag.h")
	if err := os.WriteFile(path, []byte(header), 0644); err != nil {
		t.Fatal(err)
	}

	info, err := ParseCHeader(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := info.Functions["ok_fn"]; !ok {
		t.Fatalf("real declaration missing: %v", info.Functions)
	}
	if _, ok := info.Functions["commit"]; ok {
		t.Errorf("continuation fragment registered as function")
	}
}
