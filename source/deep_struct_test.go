package main

import (
	"strings"
	"testing"
)

// Deep struct teardown (Item A): an array of structs with direct string
// fields and pointer-to-struct components must free every string field
// (direct + nested) before releasing the shells. This lets Bell delete
// node_strings_free: the compiler owns what was manual.
func TestDeepStructArrayTeardown(t *testing.T) {
	gen := &CodeGenerator{
		structs: map[string]*StructInfo{
			"Inner": {Name: "Inner", Fields: []StructField{
				{Name: "feed", Type: "char*", AhoyType: "string"},
			}},
			"Outer": {Name: "Outer", Fields: []StructField{
				{Name: "name", Type: "char*", AhoyType: "string"},
				{Name: "vis", Type: "Inner*", AhoyType: "Inner*"},
			}},
		},
		varAllocClass: map[string]AllocationClass{},
	}
	out := gen.generateFreeCodeForVar("nodes", "array[Outer]")
	if !strings.Contains(out, "free(__el->name)") {
		t.Errorf("struct array teardown must free direct string fields, got:\n%s", out)
	}
	if !strings.Contains(out, "free(__el->vis->feed)") {
		t.Errorf("struct array teardown must free nested component strings, got:\n%s", out)
	}
	if !strings.Contains(out, "__el->vis->feed[0]") {
		t.Errorf("struct array teardown must guard empties (static defaults), got:\n%s", out)
	}
	if !strings.Contains(out, "AhoyReleaseInner(__el->vis)") {
		t.Errorf("struct array teardown must release the component shell, got:\n%s", out)
	}
}

// Components without strings must still be released (otherwise the
// component shells leak when the parent array dies).
func TestDeepStructReleasesStringlessComponents(t *testing.T) {
	gen := &CodeGenerator{
		structs: map[string]*StructInfo{
			"Plain": {Name: "Plain", Fields: []StructField{
				{Name: "x", Type: "int", AhoyType: "int"},
			}},
			"Outer2": {Name: "Outer2", Fields: []StructField{
				{Name: "name", Type: "char*", AhoyType: "string"},
				{Name: "sub", Type: "Plain*", AhoyType: "Plain*"},
			}},
		},
		varAllocClass: map[string]AllocationClass{},
	}
	out := gen.generateFreeCodeForVar("nodes", "array[Outer2]")
	if !strings.Contains(out, "free(__el->name)") {
		t.Errorf("must free direct strings, got:\n%s", out)
	}
	if !strings.Contains(out, "AhoyReleasePlain(__el->sub)") {
		t.Errorf("must release stringless component shells, got:\n%s", out)
	}
}
