package main

import (
	"strings"

	"ahoy"
)

// escapeFmtLiteral escapes literal text for use as a printf format segment.
// Every '%' in a literal is a format specifier to printf, so double it.
func escapeFmtLiteral(s string) string {
	return strings.ReplaceAll(s, "%", "%%")
}

// fstringSpecFor maps a pre-parsed interpolation to its printf spec and,
// for dict loop values, a ready-made runtime-typed expression. It mirrors
// the legacy textual spec logic, driven by the expression's inferred type.
func (gen *CodeGenerator) fstringSpecFor(child *ahoy.ASTNode) (spec string, dictExpr string) {
	if child.Type == ahoy.NODE_IDENTIFIER {
		if expr, ok := gen.dictLoopValueExpr[child.Value]; ok {
			return "%s", expr
		}
	}
	varType := normalizeCollectionType(gen.inferType(child))
	spec = "%d"
	if fw, _ := fixedWidthSpec(varType); fw != "" {
		spec = fw
	} else if varType == "string" || varType == "char*" || varType == "intptr_t" ||
			varType == "dict" || varType == "HashMap*" {
		spec = "%s"
	} else if varType == "float" || varType == "double" {
		spec = "%f"
	} else if varType == "char" {
		spec = "%c"
	}
	return spec, ""
}

// generateFStringFromChildren emits an f-string whose {expr} groups the
// parser pre-parsed into Children (literals as NODE_STRING). Each expression
// goes through the normal expression generator, so member access, calls,
// casts and indexing all format correctly.
func (gen *CodeGenerator) generateFStringFromChildren(node *ahoy.ASTNode) {
	type farg struct {
		spec     string
		expr     *ahoy.ASTNode
		dictExpr string
	}
	var formatStr strings.Builder
	var args []farg
	for _, child := range node.Children {
		if child.Type == ahoy.NODE_STRING {
			formatStr.WriteString(escapeFmtLiteral(child.Value))
			continue
		}
		spec, dictExpr := gen.fstringSpecFor(child)
		formatStr.WriteString(spec)
		args = append(args, farg{spec: spec, expr: child, dictExpr: dictExpr})
	}
	if len(args) == 0 {
		gen.output.WriteString("\"")
		gen.output.WriteString(formatStr.String())
		gen.output.WriteString("\"")
		return
	}

	// Choose the allocator. Arena by default (freed at scope end via the
	// watermark). Owned for values that must outlive the current scope:
	// return expressions, retaining literals, foreign-call args, or when
	// the legacy static-buffer path is requested.
	helper := "ahoy_arena_sprintf"
	if fstringLegacy || gen.inReturnExpr || gen.inRetainingLiteral || gen.inForeignCall || gen.returnTypeCanHoldString() {
		helper = "ahoy_owned_sprintf"
	}

	gen.output.WriteString(helper)
	gen.output.WriteString("(")
	if helper == "ahoy_arena_sprintf" {
		gen.output.WriteString("__ahoy_frame_arena, ")
	}
	gen.output.WriteString("\"")
	gen.output.WriteString(formatStr.String())
	gen.output.WriteString("\"")
	for _, a := range args {
		gen.output.WriteString(", ")
		if a.dictExpr != "" {
			gen.output.WriteString(a.dictExpr)
			continue
		}
		// intptr_t carries string pointers; the bare value would print as a number.
		if normalizeCollectionType(gen.inferType(a.expr)) == "intptr_t" {
			gen.output.WriteString("(char*)")
		} else if cast := printfCastForSpec(a.spec); cast != "" {
			gen.output.WriteString(cast)
		}
		gen.generateNode(a.expr)
	}
	gen.output.WriteString(")")
}