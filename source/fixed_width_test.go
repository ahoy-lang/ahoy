package main

import (
	"strings"
	"testing"
)

// Tests for the fixed-width integer types (i8/u8, i16/u16, i32/u32, i64/u64)
// and the identifier-spelled cast syntax (`i64(x)`, the paren form the docs
// use; the pipe form `i64|x|` has always worked through the call path).

func TestFixedWidthParenCastParses(t *testing.T) {
	// The paren form is the documented one: f32(3.7), i64(n).
	for _, src := range []string{
		"program p\n@ main ||:\n\tx: i64 = i64(5)\n\tprint|x|\n$\n",
		"program p\n@ main ||:\n\tx: u32 = u32(5)\n\tprint|x|\n$\n",
		"program p\n@ main ||:\n\tx: f32 = f32(1.5)\n\tprint|x|\n$\n",
	} {
		msgs := diagnosticsFor(src)
		wantNoError(t, msgs, "not declared")
	}
}

func TestFixedWidthDeclarationTypes(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ add |a:i64, b:i64| i64:
	return a + b
$
@ main ||:
	x: i64 = 5
	y: u32 = 6
	z: i8 = i8(-1)
	w: u64 = u64(7)
	print|x, y, z, w|
$
`)
	wantNoError(t, msgs, "not declared")
}

func TestFixedWidthMapToCTypes(t *testing.T) {
	src := `program p
@ main ||:
	a: i64 = 1
	b: u64 = 2
	c: i8 = i8(3)
	d: u16 = u16(4)
	e: i32 = i32(5)
	f: u8 = u8(6)
	print|a, b, c, d, e, f|
$
`
	c := generateForTest(t, src)
	for _, want := range []string{
		"int64_t a",
		"uint64_t b",
		"int8_t c",
		"uint16_t d",
		"int32_t e",
		"uint8_t f",
	} {
		if !strings.Contains(c, want) {
			t.Errorf("expected %q in generated C:\n%s", want, c)
		}
	}
}

func TestFixedWidthPrintWithFullWidth(t *testing.T) {
	// A 64-bit value must not be printed through %d, which truncates it to
	// 32 bits. The wide types need their own specifier and cast.
	c := generateForTest(t, `program p
@ main ||:
	a: i64 = 3000000000
	print|a|
$
`)
	if !strings.Contains(c, "%lld") {
		t.Errorf("expected the i64 print specifier %%lld in generated C:\n%s", c)
	}
	if strings.Contains(c, "(int)a") {
		t.Errorf("i64 must not be cast to (int) for printing:\n%s", c)
	}

	c = generateForTest(t, `program p
@ main ||:
	a: u32 = 4000000000
	print|a|
$
`)
	if !strings.Contains(c, "%u") {
		t.Errorf("expected the u32 print specifier %%u in generated C:\n%s", c)
	}
}
