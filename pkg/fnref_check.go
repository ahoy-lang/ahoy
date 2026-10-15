package ahoy

import "fmt"
import "strings"

// Function-reference checking: a &name value flowing into an fn-typed slot (a
// struct field, a declared fn variable, an fn-typed parameter) must fit that
// slot. These helpers are shared by every hook site so literals,
// declarations and call arguments report the same diagnostics.

// FnRequiredParams counts a signature's parameters without defaults. The
// parser rejects non-default parameters after default ones, so every
// defaulted parameter is trailing.
func FnRequiredParams(sig *FunctionSignature) int {
	if sig == nil {
		return 0
	}
	if sig.FunctionNode != nil && len(sig.FunctionNode.Children) > 0 && sig.FunctionNode.Children[0] != nil {
		required := 0
		for _, param := range sig.FunctionNode.Children[0].Children {
			if param != nil && param.DefaultValue == nil {
				required++
			}
		}
		return required
	}
	return len(sig.Parameters)
}

// CheckFnRefAgainstSlot validates a &name function reference against the fn
// type of the slot it flows into. sig describes the referenced function;
// slotFnType is the expected `fn(...)...->...` string; slotDesc names the
// slot for messages ("field 'draw'").
//
// It returns ("", "") when compatible, else (severity, message) with severity
// "error" or "warning". A function taking FEWER parameters than the slot
// passes is only warned about: extra arguments are ignored on every supported
// ABI, and null-object stubs deliberately serve several arities. Taking MORE
// than the slot passes, or a return mismatch, is an error. `any`/unknown
// sides are wildcards throughout.
func CheckFnRefAgainstSlot(refName string, sig *FunctionSignature, slotFnType, slotDesc string) (string, string) {
	if sig == nil {
		return "", ""
	}
	slotParams, slotRet, ok := ParseFunctionType(NormalizeReceiverType(slotFnType))
	if !ok {
		return "", ""
	}
	fnTypes := make([]string, 0, len(sig.Parameters))
	for _, p := range sig.Parameters {
		fnTypes = append(fnTypes, p.Type)
	}
	fnRequired := FnRequiredParams(sig)
	fnRet := ""
	if len(sig.ReturnTypes) > 0 {
		fnRet = sig.ReturnTypes[0]
	} else if sig.FunctionNode != nil && sig.FunctionNode.DataType == "void" {
		fnRet = "void"
	}
	slotN := len(slotParams)

	if fnRequired > slotN {
		return "error", fmt.Sprintf("Function reference '&%s' takes at least %d argument(s) but slot %s passes %d",
			refName, fnRequired, slotDesc, slotN)
	}
	if len(fnTypes) < slotN {
		return "warning", fmt.Sprintf("Function reference '&%s' takes only %d parameter(s) but slot %s passes %d arguments; extra arguments are ignored",
			refName, len(fnTypes), slotDesc, slotN)
	}
	for i := 0; i < slotN; i++ {
		if !fnSlotTypesCompatible(fnTypes[i], slotParams[i]) {
			return "error", fmt.Sprintf("Function reference '&%s' parameter %d is %s but slot %s passes %s",
				refName, i+1, displayType(fnTypes[i]), slotDesc, displayType(slotParams[i]))
		}
	}
	if slotRet != "" && slotRet != "void" && fnRet != "" && !fnSlotTypesCompatible(fnRet, slotRet) {
		return "error", fmt.Sprintf("Function reference '&%s' returns %s but slot %s expects %s",
			refName, displayType(fnRet), slotDesc, displayType(slotRet))
	}
	return "", ""
}

// fnSlotTypesCompatible reports whether a function-side type fits a slot-side
// type. Empty, unknown and any/generic spellings are wildcards.
func fnSlotTypesCompatible(fnType, slotType string) bool {
	if fnType == "" || slotType == "" {
		return true
	}
	a := NormalizeReceiverType(fnType)
	e := NormalizeReceiverType(slotType)
	if a == "any" || e == "any" || a == "generic" || e == "generic" ||
		a == "unknown" || e == "unknown" {
		return true
	}
	return a == e
}

func displayType(t string) string {
	if t == "" {
		return "unknown"
	}
	return t
}

// FreshStringMethods names the string methods that always return freshly
// heap-allocated strings (the caller owns the result). Every other
// string-typed value — literals, f-strings (static buffer), aliases,
// element/field access — is static or owned elsewhere and must never be
// freed. Shared by codegen tracking and checker warnings so both agree on
// what allocates.
var FreshStringMethods = map[string]bool{
	"upper": true, "lower": true, "replace": true, "strip": true,
	"lpad": true, "rpad": true, "pad": true,
	"camel_case": true, "snake_case": true, "pascal_case": true, "kebab_case": true,
	"get_file": true, "dup": true,
}

// isHeapTrackedType reports whether a type string denotes heap-owned data
// for autofree purposes: containers, strings, function values, pools and
// pointers. Struct *values* are excluded (stack copies); only struct
// pointers count. Unknown, empty and any/generic spellings are not heap.
func (p *Parser) isHeapTrackedType(t string) bool {
	if t == "" {
		return false
	}
	nt := NormalizeReceiverType(t)
	switch nt {
	case "", "unknown", "any", "generic", "infer", "void", "null":
		return false
	}
	if strings.HasPrefix(nt, "array") || strings.HasPrefix(nt, "dict") {
		return true
	}
	if nt == "string" || nt == "pool" {
		return true
	}
	if IsFunctionType(nt) {
		return true
	}
	if strings.Contains(nt, "*") {
		return true
	}
	return false
}

// heapPushValue reports whether pushing this value into an array stores heap
// memory the container will not free (untyped arrays free only their
// structure): allocating array/dict literals and heap-typed names and
// accesses. Struct *values* are excluded (push mallocs a copy the container
// owns); static literals and unknown shapes never allocate here. Calls and
// methods cannot appear (rejected as arguments), and f-strings ride a
// static buffer the container merely points at.
func (c *checker) heapPushValue(node *ASTNode) bool {
	if node == nil {
		return false
	}
	switch node.Type {
	case NODE_ARRAY_LITERAL, NODE_DICT_LITERAL:
		return true
	case NODE_STRING, NODE_NUMBER, NODE_CHAR, NODE_BOOLEAN, NODE_RAW_STRING, NODE_F_STRING:
		return false
	case NODE_IDENTIFIER, NODE_MEMBER_ACCESS, NODE_OBJECT_ACCESS, NODE_ARRAY_ACCESS, NODE_DICT_ACCESS:
		return c.p.isHeapTrackedType(c.valueStoredType(node))
	}
	return false
}

// heapStoreValue reports whether assigning this value into a struct field or
// dict slot leaves heap memory behind: everything heapPushValue covers, plus
// f-strings (member stores strdup them), string concatenation, sprintf,
// fresh-returning string methods, and calls with heap-typed results. Static
// literals and unknown shapes do not.
func (c *checker) heapStoreValue(node *ASTNode) bool {
	if node == nil {
		return false
	}
	if node.Type == NODE_F_STRING {
		return true
	}
	if node.Type == NODE_BINARY_OP && (node.Value == "+" || node.Value == "plus") &&
		len(node.Children) >= 2 &&
		c.exprIsString(node.Children[0]) && c.exprIsString(node.Children[1]) {
		return true
	}
	if node.Type == NODE_CALL && node.Value == "sprintf" {
		return true
	}
	if node.Type == NODE_METHOD_CALL && FreshStringMethods[node.Value] {
		return true
	}
	if node.Type == NODE_CALL {
		if sig, ok := c.p.functions[node.Value]; ok && sig != nil && len(sig.ReturnTypes) > 0 {
			return c.p.isHeapTrackedType(sig.ReturnTypes[0])
		}
		return false
	}
	return c.heapPushValue(node)
}

// valueStoredType infers a value's type for store checks. It delegates to
// inferNodeType, which sees the checker's lexical scopes.
func (c *checker) valueStoredType(node *ASTNode) string {
	return c.inferNodeType(node)
}

// inferNodeType infers an expression's type using the checker's lexical
// scopes first (accurate for locals declared earlier in the function),
// falling back to structural inference. Unlike p.inferType, whose
// per-function maps are reset after parsing, this sees the function being
// checked.
func (c *checker) inferNodeType(node *ASTNode) string {
	if node == nil {
		return ""
	}
	if node.Type == NODE_IDENTIFIER {
		if t := c.lookupType(node.Value); t != "" {
			return t
		}
		if t, ok := c.p.variableTypes[node.Value]; ok {
			return t
		}
		return ""
	}
	if node.Type == NODE_MEMBER_ACCESS && len(node.Children) > 0 {
		base := c.inferNodeType(node.Children[0])
		if def, ok := c.p.structs[NormalizeReceiverType(base)]; ok {
			for _, f := range def.Fields {
				if f.Name == node.Value {
					return f.Type
				}
			}
		}
		return ""
	}
	if node.Type == NODE_ARRAY_ACCESS {
		base := ""
		if node.Value != "" {
			base = c.inferNodeType(&ASTNode{Type: NODE_IDENTIFIER, Value: node.Value})
		} else if len(node.Children) > 0 {
			base = c.inferNodeType(node.Children[0])
		}
		base = NormalizeReceiverType(base)
		if strings.HasPrefix(base, "array[") && strings.HasSuffix(base, "]") {
			return strings.TrimSuffix(strings.TrimPrefix(base, "array["), "]")
		}
		return ""
	}
	if node.Type == NODE_BINARY_OP && (node.Value == "+" || node.Value == "plus") &&
		len(node.Children) >= 2 &&
		c.exprIsString(node.Children[0]) && c.exprIsString(node.Children[1]) {
		return "string"
	}
	t := c.p.inferType(node)
	if t == "unknown" {
		return ""
	}
	return t
}

// inferVarType infers a declaration value's type for scope tracking,
// extending the parser's inference to string concatenation (whose operands
// resolve through these same scopes). Anything unknowable stays "".
func (c *checker) inferVarType(node *ASTNode) string {
	if node == nil {
		return ""
	}
	if node.Type == NODE_BINARY_OP && (node.Value == "+" || node.Value == "plus") &&
		len(node.Children) >= 2 &&
		c.exprIsString(node.Children[0]) && c.exprIsString(node.Children[1]) {
		return "string"
	}
	if node.Type == NODE_CALL && node.Value == "sprintf" {
		return "string"
	}
	t := c.p.inferType(node)
	if t == "unknown" {
		return ""
	}
	return t
}

// exprIsString reports whether an expression is known to produce a string:
// literals, f-strings, string-typed names, sprintf, fresh-returning string
// methods, and calls with known string returns.
func (c *checker) exprIsString(node *ASTNode) bool {
	if node == nil {
		return false
	}
	switch node.Type {
	case NODE_STRING, NODE_RAW_STRING, NODE_F_STRING, NODE_CHAR:
		return true
	}
	if node.Type == NODE_IDENTIFIER {
		return NormalizeReceiverType(c.lookupType(node.Value)) == "string"
	}
	if node.Type == NODE_CALL && node.Value == "sprintf" {
		return true
	}
	if node.Type == NODE_METHOD_CALL && FreshStringMethods[node.Value] {
		return true
	}
	if node.Type == NODE_CALL {
		if sig, ok := c.p.functions[node.Value]; ok && sig != nil && len(sig.ReturnTypes) > 0 {
			return NormalizeReceiverType(sig.ReturnTypes[0]) == "string"
		}
	}
	// Member, array and dict access resolve through the checker's scopes.
	if node.Type == NODE_MEMBER_ACCESS || node.Type == NODE_OBJECT_ACCESS ||
		node.Type == NODE_ARRAY_ACCESS || node.Type == NODE_DICT_ACCESS {
		return NormalizeReceiverType(c.inferNodeType(node)) == "string"
	}
	return false
}
