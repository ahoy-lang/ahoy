package ahoy

import "strings"

// Method support: user-defined methods (@ (recv T) name) and associated
// functions (@ T.name).
//
// A method function is an ordinary NODE_FUNCTION node whose Value is the
// mangled global name (MangleMethodName) and whose Method marker describes the
// method view. Calls use the existing dot-pipe syntax (recv.m|args|) and lower
// to a plain call with the receiver prepended: Type_m|recv, args|.

// MethodInfo describes a user-defined method (@ (recv T) name) or an associated
// function (@ T.name).
type MethodInfo struct {
	Receiver   string // normalized receiver type ("Point", "array[Node]")
	Name       string // method name as written ("move")
	Associated bool   // true for @ T.name (called without a receiver)
}

// MethodEntry pairs a method's declaration info with its signature.
type MethodEntry struct {
	Info *MethodInfo
	Sig  *FunctionSignature
}

// MangleMethodName maps (receiver, method) to the global function name the
// method compiles as. Genuinely ambiguous pairs (A_B + C versus A + B_C) land
// on one C symbol and are rejected at registration.
func MangleMethodName(receiver, name string) string {
	return sanitizeMethodTypeName(receiver) + "_" + name
}

func sanitizeMethodTypeName(t string) string {
	var b strings.Builder
	prevUnderscore := true
	for _, r := range t {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevUnderscore = false
		} else if !prevUnderscore {
			b.WriteRune('_')
			prevUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

// NormalizeReceiverType canonicalizes a type string for method-table keys so
// the spelling at the definition site matches the spelling inferred at call
// sites: whitespace is dropped, the `struct:` prefix some paths add is
// stripped, and C string spellings fold to `string`.
func NormalizeReceiverType(t string) string {
	t = strings.Join(strings.Fields(t), "")
	t = strings.TrimPrefix(t, "struct:")
	switch t {
	case "char*", "char *", "const char*", "const char *":
		return "string"
	}
	return t
}

// CollectMethods walks roots and returns user-defined methods keyed by
// normalized receiver type, then method name. Later roots override earlier
// ones on exact (type, method) duplicates. Associated functions (@ T.name)
// are keyed under their type with Associated set.
func CollectMethods(roots ...*ASTNode) map[string]map[string]*MethodEntry {
	out := map[string]map[string]*MethodEntry{}
	var walk func(n *ASTNode)
	walk = func(n *ASTNode) {
		if n == nil {
			return
		}
		if n.Type == NODE_FUNCTION && n.Method != nil {
			if _, ok := out[n.Method.Receiver]; !ok {
				out[n.Method.Receiver] = map[string]*MethodEntry{}
			}
			out[n.Method.Receiver][n.Method.Name] = &MethodEntry{
				Info: n.Method,
				Sig:  signatureOfFunctionNode(n),
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, r := range roots {
		walk(r)
	}
	return out
}

// signatureOfFunctionNode rebuilds a FunctionSignature from a parsed function
// node: parameters from the first child (the params block), return types from
// the node's data type.
func signatureOfFunctionNode(n *ASTNode) *FunctionSignature {
	sig := &FunctionSignature{
		Name:         n.Value,
		FunctionNode: n,
		Line:         n.Line,
	}
	if len(n.Children) > 0 && n.Children[0] != nil {
		for _, param := range n.Children[0].Children {
			if param == nil {
				continue
			}
			sig.Parameters = append(sig.Parameters, ParameterInfo{
				Name: param.Value,
				Type: param.DataType,
			})
		}
	}
	if n.DataType != "" && n.DataType != "void" && n.DataType != "infer" {
		for _, rt := range strings.Split(n.DataType, ",") {
			if rt = strings.TrimSpace(rt); rt != "" {
				sig.ReturnTypes = append(sig.ReturnTypes, rt)
			}
		}
	}
	return sig
}

// methodReceiverType notes a method call's receiver type for validation and
// dispatch. Parse-time scopes are accurate here; by check/generate time the
// parser's per-function maps have been reset, so without this record locals
// would read as unknown. Returns "" when the type is not known — dynamic
// receivers stay permissive — and in build mode, where type maps are
// lint-only and codegen falls back to its own inference.
func (p *Parser) methodReceiverType(object *ASTNode) string {
	if object == nil {
		return ""
	}
	t := NormalizeReceiverType(p.inferType(object))
	switch t {
	case "", "unknown", "any", "generic", "infer", "void", "null":
		return ""
	}
	return t
}
