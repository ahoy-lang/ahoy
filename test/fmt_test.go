package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Golden-file tests for the source formatter.
//
// The formatter itself lives in source/formatter.go and is exercised here
// through the compiler's `-format` flag, so these tests cover the same code
// path a user gets. (This file used to carry its own copy of a formatter, which
// meant the golden files were never testing the real thing.)
//
// The rules the formatter guarantees are documented on formatSource in
// source/formatter.go: two-space indentation, a `$` at the level of the line
// that opened the block, a short single-statement block joined onto one line,
// comments preserved, spacing normalised, and idempotence.

const (
	fmtInputDir  = "fmt_input"
	fmtOutputDir = "fmt_output"
	compilerPath = "./ahoy-bin"
)

// formatFile copies src into a temporary directory, formats it with the
// compiler, and returns the formatted text.
func formatFile(t *testing.T, src string) string {
	t.Helper()

	if _, err := os.Stat(compilerPath); err != nil {
		t.Skipf("compiler not built at %s: %v", compilerPath, err)
	}

	dir := t.TempDir()
	target := filepath.Join(dir, filepath.Base(src))
	content, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("reading %s: %v", src, err)
	}
	if err := os.WriteFile(target, content, 0o644); err != nil {
		t.Fatalf("writing %s: %v", target, err)
	}

	cmd := exec.Command(compilerPath, "-format", "-f", target)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("formatting %s: %v\n%s", src, err, out.String())
	}

	formatted, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading formatted output: %v", err)
	}
	return string(formatted)
}

// TestFormatter checks each input file against its golden output.
func TestFormatter(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join(fmtInputDir, "*.ahoy"))
	if err != nil || len(inputs) == 0 {
		t.Fatalf("no input files found in %s", fmtInputDir)
	}

	for _, input := range inputs {
		name := strings.TrimSuffix(filepath.Base(input), ".ahoy")
		t.Run(name, func(t *testing.T) {
			expectedPath := filepath.Join(fmtOutputDir, name+".ahoy")
			expected, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("reading %s: %v", expectedPath, err)
			}

			got := formatFile(t, input)
			if got != string(expected) {
				t.Errorf("formatted output does not match %s", expectedPath)
				reportLineDiff(t, string(expected), got)
			}
		})
	}
}

// TestFormatterIsIdempotent formats each golden output again and expects no
// change, which is what makes the formatter safe to run on every save.
func TestFormatterIsIdempotent(t *testing.T) {
	outputs, err := filepath.Glob(filepath.Join(fmtOutputDir, "*.ahoy"))
	if err != nil || len(outputs) == 0 {
		t.Fatalf("no golden files found in %s", fmtOutputDir)
	}

	for _, golden := range outputs {
		name := strings.TrimSuffix(filepath.Base(golden), ".ahoy")
		t.Run(name, func(t *testing.T) {
			once := formatFile(t, golden)
			expected, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("reading %s: %v", golden, err)
			}
			if once != string(expected) {
				t.Errorf("formatting an already-formatted file changed it")
				reportLineDiff(t, string(expected), once)
			}
		})
	}
}

// TestFormattedCodeStillCompiles is the property that matters most: formatting
// must not change what a program means. The compiler rejects the formatted file
// if the formatter broke it.
func TestFormattedCodeStillCompiles(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join(fmtInputDir, "*.ahoy"))
	if err != nil || len(inputs) == 0 {
		t.Fatalf("no input files found in %s", fmtInputDir)
	}

	for _, input := range inputs {
		name := strings.TrimSuffix(filepath.Base(input), ".ahoy")
		t.Run(name, func(t *testing.T) {
			if _, err := os.Stat(compilerPath); err != nil {
				t.Skipf("compiler not built at %s", compilerPath)
			}
			dir := t.TempDir()
			target := filepath.Join(dir, filepath.Base(input))
			content, err := os.ReadFile(input)
			if err != nil {
				t.Fatalf("reading %s: %v", input, err)
			}
			if err := os.WriteFile(target, content, 0o644); err != nil {
				t.Fatalf("writing %s: %v", target, err)
			}

			for _, args := range [][]string{
				{"-format", "-f", target},
				{"-f", target}, // compile the formatted source
			} {
				cmd := exec.Command(compilerPath, args...)
				var out strings.Builder
				cmd.Stdout = &out
				cmd.Stderr = &out
				if err := cmd.Run(); err != nil {
					t.Fatalf("%v failed on formatted %s: %v\n%s", args, input, err, out.String())
				}
			}
		})
	}
}

func reportLineDiff(t *testing.T, expected, got string) {
	t.Helper()
	expLines := strings.Split(expected, "\n")
	gotLines := strings.Split(got, "\n")
	limit := len(expLines)
	if len(gotLines) > limit {
		limit = len(gotLines)
	}
	for i := 0; i < limit; i++ {
		var e, g string
		if i < len(expLines) {
			e = expLines[i]
		}
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if e != g {
			t.Errorf("  line %d:\n    expected: %q\n    got:      %q", i+1, e, g)
		}
	}
}

func BenchmarkFormatter(b *testing.B) {
	content, err := os.ReadFile(filepath.Join(fmtInputDir, "functions.ahoy"))
	if err != nil {
		b.Fatalf("reading input: %v", err)
	}
	src := string(content)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = formatSourceForBenchmark(src)
	}
}

// formatSourceForBenchmark shells out to the compiler's -format path. It exists
// so the benchmark measures the same code path as the tests.
func formatSourceForBenchmark(src string) string {
	dir, err := os.MkdirTemp("", "ahoy-fmt-bench")
	if err != nil {
		return ""
	}
	defer os.RemoveAll(dir)

	target := filepath.Join(dir, "bench.ahoy")
	if err := os.WriteFile(target, []byte(src), 0o644); err != nil {
		return ""
	}
	if err := exec.Command(compilerPath, "-format", "-f", target).Run(); err != nil {
		return ""
	}
	out, err := os.ReadFile(target)
	if err != nil {
		return ""
	}
	return string(out)
}
