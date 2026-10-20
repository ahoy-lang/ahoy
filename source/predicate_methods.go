package main

import (
	"strconv"
	"strings"

	"ahoy"
)

// The predicate builtins: any, all, count_if, find_index and find.
//
// They read an array element by element and ask a lambda about each one:
//
//	nodes.any|n: n.visible|            - is there one that matches?
//	nodes.all|n: n.visible|            - do all of them match?
//	nodes.count_if|n: n.kind is 3|     - how many match?
//	nodes.find_index|n: n.name is "x"| - where is the first one that matches?
//	nodes.find|n: n.name is "x"|       - the first one that matches
//
// Without them, every one of those questions is a hand-written loop with a
// flag, and the flag is the reason the code nests: the loop body has to
// survive after the match to set the flag, so the "interesting" part ends up
// one level deeper than it needs to be.
//
// The code is generated inline (like map and filter) so the lambda is
// inlined into the loop rather than called through a function pointer.

// predicateMethods are the builtins this file generates.
var predicateMethods = map[string]bool{
	"any": true, "all": true, "count_if": true, "find_index": true, "find": true,
}

// lambdaParts splits a lambda into its parameters and its body.
//
// The parser has two shapes for a lambda: the old one puts the single
// parameter name in Value and the body in the only child, the new one puts
// the parameter count in Value, the parameters in the leading children and
// the body last.
func lambdaParts(lambda *ahoy.ASTNode) ([]string, *ahoy.ASTNode) {
	paramCount := 1
	if lambda.Value != "" {
		if count, err := strconv.Atoi(lambda.Value); err == nil {
			paramCount = count
		}
	}
	if paramCount == 1 && len(lambda.Children) == 1 {
		return []string{lambda.Value}, lambda.Children[0]
	}
	if len(lambda.Children) > paramCount {
		params := make([]string, 0, paramCount)
		for i := 0; i < paramCount; i++ {
			params = append(params, lambda.Children[i].Value)
		}
		return params, lambda.Children[paramCount]
	}
	return []string{"x"}, lambda.Children[0]
}

// predicateElementType is the type of the elements of the array the
// predicate reads.
func (gen *CodeGenerator) predicateElementType(arrayNode *ahoy.ASTNode) string {
	t := gen.inferType(arrayNode)
	if strings.HasPrefix(t, "array[") && strings.HasSuffix(t, "]") {
		return t[len("array[") : len(t)-1]
	}
	return "int"
}

// emitPredicateElement writes the C expression that reads element __i of the
// array bound to __src, following the same rules as an array access: a float
// element is a heap double, a struct element is a pointer to one.
func (gen *CodeGenerator) emitPredicateElement(elemType string) {
	cType := gen.mapType(elemType)
	switch {
	case cType == "double" || cType == "float":
		gen.output.WriteString("(*(double*)(intptr_t)__src->data[__i])")
	case cType == "char*" || cType == "const char*":
		gen.output.WriteString("((char*)(intptr_t)__src->data[__i])")
	case gen.isStructType(elemType, cType):
		gen.output.WriteString("(*(")
		gen.output.WriteString(cType)
		gen.output.WriteString("*)__src->data[__i])")
	default:
		gen.output.WriteString("((int)(intptr_t)__src->data[__i])")
	}
}

// isStructType reports whether the element is a struct (an Ahoy struct or a
// C struct), which the runtime stores as a pointer.
func (gen *CodeGenerator) isStructType(elemType, cType string) bool {
	if cType == "int" || cType == "double" || cType == "float" ||
		cType == "char*" || cType == "const char*" || cType == "bool" {
		return false
	}
	if strings.HasSuffix(cType, "*") {
		return false
	}
	if gen.cTypeDefinitions[cType] {
		return true
	}
	if _, ok := gen.structs[elemType]; ok {
		return true
	}
	_, ok := gen.structs[cType]
	return ok
}

// emitPredicateBinding writes the C that binds the lambda's parameter to the
// current element.
func (gen *CodeGenerator) emitPredicateBinding(params []string, elemType string) {
	cType := gen.mapType(elemType)
	if len(params) > 1 {
		// A tuple element: the elements sit in a nested array.
		gen.output.WriteString("AhoyArray* __elem = (AhoyArray*)__src->data[__i]; ")
		for i, name := range params {
			gen.output.WriteString("int ")
			gen.output.WriteString(name)
			gen.output.WriteString(" = __elem->data[")
			gen.output.WriteString(strconv.Itoa(i))
			gen.output.WriteString("]; ")
		}
		return
	}
	gen.output.WriteString(cType)
	gen.output.WriteString(" ")
	gen.output.WriteString(params[0])
	gen.output.WriteString(" = ")
	gen.emitPredicateElement(elemType)
	gen.output.WriteString("; ")
}

// generatePredicateInline emits the inline code for one of the predicate
// builtins.
func (gen *CodeGenerator) generatePredicateInline(kind string, arrayNode *ahoy.ASTNode, lambda *ahoy.ASTNode) {
	params, body := lambdaParts(lambda)
	elemType := gen.predicateElementType(arrayNode)

	gen.output.WriteString("({ AhoyArray* __src = ")
	gen.generateNodeInternal(arrayNode, false)
	gen.output.WriteString("; ")

	switch kind {
	case "any":
		gen.output.WriteString("int __found = 0; for (int __i = 0; __i < __src->length; __i++) { ")
		gen.emitPredicateBinding(params, elemType)
		gen.output.WriteString("if (")
		gen.generateNodeInternal(body, false)
		gen.output.WriteString(") { __found = 1; break; } } __found; })")
	case "all":
		gen.output.WriteString("int __all = 1; for (int __i = 0; __i < __src->length; __i++) { ")
		gen.emitPredicateBinding(params, elemType)
		gen.output.WriteString("if (!(")
		gen.generateNodeInternal(body, false)
		gen.output.WriteString(")) { __all = 0; break; } } __all; })")
	case "count_if":
		gen.output.WriteString("int __count = 0; for (int __i = 0; __i < __src->length; __i++) { ")
		gen.emitPredicateBinding(params, elemType)
		gen.output.WriteString("if (")
		gen.generateNodeInternal(body, false)
		gen.output.WriteString(") { __count++; } } __count; })")
	case "find_index":
		gen.output.WriteString("int __found = -1; for (int __i = 0; __i < __src->length; __i++) { ")
		gen.emitPredicateBinding(params, elemType)
		gen.output.WriteString("if (")
		gen.generateNodeInternal(body, false)
		gen.output.WriteString(") { __found = __i; break; } } __found; })")
	case "find":
		// The element itself, with the element's own type. When nothing
		// matches, the element is the zero value: find_index says where a
		// match was (and -1 when there was none), so a caller that has to
		// tell the two apart should use that one.
		cType := gen.mapType(elemType)
		gen.output.WriteString(cType)
		gen.output.WriteString(" __found = {0}; for (int __i = 0; __i < __src->length; __i++) { ")
		gen.emitPredicateBinding(params, elemType)
		gen.output.WriteString("if (")
		gen.generateNodeInternal(body, false)
		gen.output.WriteString(") { __found = ")
		gen.emitPredicateElement(elemType)
		gen.output.WriteString("; break; } } __found; })")
	}
}
