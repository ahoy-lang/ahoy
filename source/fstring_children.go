package main

import (
	"fmt"
	"strings"

	"ahoy"
)

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
	if varType == "string" || varType == "char*" || varType == "intptr_t" ||
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
			formatStr.WriteString(child.Value)
			continue
		}
		spec, dictExpr := gen.fstringSpecFor(child)
		formatStr.WriteString(spec)
		args = append(args, farg{spec: spec, expr: child, dictExpr: dictExpr})
	}
	if len(args) == 0 {
		gen.output.WriteString(fmt.Sprintf("\"%s\"", formatStr.String()))
		return
	}
	bufferVar := fmt.Sprintf("__fstr_buf_%d", gen.varCounter)
	gen.varCounter++
	gen.output.WriteString("({\n")
	gen.indent++
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("static char %s[256];\n", bufferVar))
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("sprintf(%s, \"%s\"", bufferVar, formatStr.String()))
	for _, a := range args {
		gen.output.WriteString(", ")
		if a.dictExpr != "" {
			gen.output.WriteString(a.dictExpr)
			continue
		}
		// intptr_t carries string pointers; the bare value would print as a number.
		if normalizeCollectionType(gen.inferType(a.expr)) == "intptr_t" {
			gen.output.WriteString("(char*)")
		} else if a.spec == "%d" {
			gen.output.WriteString("(int)")
		} else if a.spec == "%f" {
			gen.output.WriteString("(double)")
		}
		gen.generateNode(a.expr)
	}
	gen.output.WriteString(");\n")
	gen.writeIndent()
	gen.output.WriteString(bufferVar)
	gen.indent--
	gen.output.WriteString("; })")
}
