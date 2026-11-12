package main

import (
	"strings"
	"testing"

	"ahoy"
)

// Slot stores outlive their statement but a heap string value may not:
// block-local heap strings are freed at scope exit, f-string buffers are
// static and get overwritten, owned split-pieces die with their array.
// Storing any of those into an array element or dict entry without
// duplicating leaves a dangling slot (use-after-free). The compiler must
// duplicate the value at the boundary (and suppress the source's
// scope-end free, transferring ownership to the slot).
func slotProbeC(t *testing.T) string {
	t.Helper()
	src := `program p
@ main ||:
	arr: array[string] = ["", ""]
	if true then
		s: "hello_" + "world"
		arr[0] = s
	$
	d: dict<string,string> = <"k": "old">
	if true then
		v: "val_" + "y"
		d<"nk"> = v
	$
	print(arr[0])
$
`
	ast := ahoy.ParseWithPath(ahoy.Tokenize(src), "test.ahoy")
	return GenerateCWithFilename(ast, "test.ahoy")
}

func TestArrayElementStoreDuplicatesHeapString(t *testing.T) {
	out := slotProbeC(t)
	if !strings.Contains(out, "strdup(s)") {
		t.Errorf("array element store of a block-local heap string must strdup it")
	}
}

func TestArrayElementStoreKeepsSourceAlive(t *testing.T) {
	out := slotProbeC(t)
	// Duplicated at the boundary: the slot owns its copy, and the source
	// is still freed at scope exit (no leak). The strdup is the contract.
	if !strings.Contains(out, "strdup(s)") {
		t.Errorf("expected strdup promotion for array element store")
	}
}

func TestDictEntryStoreDuplicatesHeapString(t *testing.T) {
	out := slotProbeC(t)
	if !strings.Contains(out, "strdup(v)") {
		t.Errorf("dict entry store of a block-local heap string must strdup it")
	}
}

// String arrays own every string slot by construction (literal statics are
// duplicated into heap copies at creation), so storing even a static literal
// must duplicate: the raw static would otherwise be freed as heap at scope
// exit. Scope exit deep-frees all AHOY_TYPE_STRING slots.
func TestArrayElementStoreOfLiteralStaysRaw(t *testing.T) {
	src := `program p
@ main ||:
	arr: array[string] = ["", ""]
	arr[0] = "static"
	print(arr[0])
$
`
	ast := ahoy.ParseWithPath(ahoy.Tokenize(src), "test.ahoy")
	out := GenerateCWithFilename(ast, "test.ahoy")
	if !strings.Contains(out, "strdup(\"static\")") {
		t.Errorf("storing a static literal into an owning string array must strdup it")
	}
}
