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
