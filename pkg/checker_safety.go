package ahoy

import (
	"strconv"
	"strings"
)

// Safety helpers for Phases 1-6: ownership (move/dupe/own), error-convention,
// and C-bug hardening. All checks are intraprocedural O(n); no lifetime solver.

func isContainerAhoyType(t string) bool {
	if t == "" || t == "unknown" || t == "any" || t == "generic" {
		return false
	}
	t = strings.TrimPrefix(t, "own ")
	t = strings.TrimSpace(t)
	if t == "string" || t == "error" || t == "char*" || t == "const char*" || t == "char *" || t == "const char *" {
		return true
	}
	if t == "array" || strings.HasPrefix(t, "array[") {
		return true
	}
	if t == "dict" || strings.HasPrefix(t, "dict[") || strings.HasPrefix(t, "dict<") {
		return true
	}
	if strings.HasPrefix(t, "buffer[") {
		return true
	}
	return false
}

func isOwnAhoyType(t string) bool {
	return strings.HasPrefix(strings.TrimSpace(t), "own ")
}

func stripOwnAhoyType(t string) string {
	t = strings.TrimSpace(t)
	if strings.HasPrefix(t, "own ") {
		return strings.TrimSpace(strings.TrimPrefix(t, "own "))
	}
	return t
}

func safetyIsFallible(sig *FunctionSignature) bool {
	if sig == nil || len(sig.ReturnTypes) != 2 {
		return false
	}
	rt := strings.TrimSpace(sig.ReturnTypes[1])
	return rt == "string" || rt == "error"
}

// safetyIsErrName reports whether a tuple-err variable reads as an error.
// Fallible (value, err) is signature-inferred, but (bool, string) pairs like
// `ok, label := pair()` are not errors — only err-named or *_try calls enforce must-check.
func safetyIsErrName(s string) bool {
	ls := strings.ToLower(strings.TrimSpace(s))
	if ls == "err" || ls == "error" || ls == "errmsg" || ls == "err_msg" || ls == "e" || ls == "problem" || ls == "probl" {
		return true
	}
	if strings.HasPrefix(ls, "err") || strings.HasSuffix(ls, "_err") || strings.Contains(ls, "error") {
		return true
	}
	return false
}

// safetyIsTryName reports *_try fallible naming.
func safetyIsTryName(s string) bool {
	return len(s) > 4 && s[len(s)-4:] == "_try"
}

func (c *checker) safetyFallible(name string) bool {
	if sig, ok := c.p.functions[name]; ok {
		return safetyIsFallible(sig)
	}
	return false
}

// containsIdent reports whether subtree mentions identifier name.
func safetyContainsIdent(node *ASTNode, name string) bool {
	if node == nil || name == "" {
		return false
	}
	if node.Type == NODE_IDENTIFIER && node.Value == name {
		return true
	}
	for _, ch := range node.Children {
		if safetyContainsIdent(ch, name) {
			return true
		}
	}
	if node.DefaultValue != nil && safetyContainsIdent(node.DefaultValue, name) {
		return true
	}
	return false
}

// safetyIsDupeCall reports x.dupe() / x.dup() wrapping.
func safetyIsDupeCall(node *ASTNode) bool {
	if node == nil {
		return false
	}
	if node.Type == NODE_METHOD_CALL && (node.Value == "dupe" || node.Value == "dup") {
		return true
	}
	if node.Type == NODE_CALL && (node.Value == "dupe" || node.Value == "dup") {
		return true
	}
	return false
}

// safetyTrackMove: b: <ident a> where a is a container => mark a moved.
func (c *checker) safetyTrackMove(dst string, rhs *ASTNode) {
	if rhs == nil || rhs.Type != NODE_IDENTIFIER || dst == "" || dst == "_" {
		return
	}
	src := rhs.Value
	if src == "" || src == "_" || src == dst {
		return
	}
	// Borrowed params are aliases, not moves: `b: path` where path is a param
	// must not kill `path` (Bell aliases params freely; codegen dups soundly).
	if c.safetyBorrowedParam(src) {
		return
	}
	t := c.lookupType(src)
	if t == "" {
		t = c.p.variableTypes[src]
	}
	if !isContainerAhoyType(t) {
		return
	}
	// Parameters are borrowed; moving from them is allowed only into own slots.
	// Still record the move so later use warns.
	c.movedNames[src] = true
}

// safetyCheckMoveSrc: tracking kept, warning silenced (codegen auto-dupes).
func (c *checker) safetyCheckMoveSrc(rhs *ASTNode) {
	if rhs == nil {
		return
	}
	var src string
	if rhs.Type == NODE_IDENTIFIER {
		src = rhs.Value
	}
	if src == "" {
		return
	}
	// Silenced: use-after-move no longer warns. .dupe() optional.
	_ = src
}

// safetyHealMove clears moved state on rebind.
func (c *checker) safetyHealMove(name string) {
	if name == "" {
		return
	}
	delete(c.movedNames, name)
}

// safetyBorrowedParam reports whether name is a borrowed (non-own) param.
func (c *checker) safetyBorrowedParam(name string) bool {
	if name == "" || c.funcParams == nil {
		return false
	}
	if !c.funcParams[name] {
		return false
	}
	t := c.lookupType(name)
	if t == "" {
		t = c.p.variableTypes[name]
		// parser functionScope holds param types during parse; checker scopeTypes holds them now
	}
	// Own params start with "own ". Anything else container-typed is borrowed.
	if isOwnAhoyType(t) {
		return false
	}
	// If type unknown, treat any-typed params as borrowed for string/array/dict flows?
	// Conservative: only flag when we know it is a container OR when stored value is identifier.
	// Caller passes the decision; here unknown => borrowed (must dupe to store).
	return true
}

// safetyCheckBorrowStore: no-op (codegen auto-dupes soundly).
// Kept for back-compat; .dupe() optional, never required.
func (c *checker) safetyCheckBorrowStore(src *ASTNode, where string) {
	return
}

// safetyIsErrCheckCond reports whether cond mentions err (if err / guard err / err is null ...).
func safetyIsErrCheckCond(cond *ASTNode, err string) bool {
	return safetyContainsIdent(cond, err)
}

func safetyIsNullLit(node *ASTNode) bool {
	return node != nil && node.Type == NODE_NULL_LITERAL
}

func safetyIsEmptyStringLit(node *ASTNode) bool {
	return node != nil && node.Type == NODE_STRING && node.Value == ""
}

// safetyCheckFallibleReturn enforces (val, null|string) shape for fallible functions.
func (c *checker) safetyCheckFallibleReturn(node *ASTNode) {
	if !c.inFunction || c.funcName == "" {
		return
	}
	sig, ok := c.p.functions[c.funcName]
	if !safetyIsFallible(sig) || !ok {
		return
	}
	// Only *_try functions enforce the null-on-success convention. Plain
	// (bool, string) returns like `return true, "x"` are ordinary values.
	if !safetyIsTryName(c.funcName) {
		return
	}
	if len(node.Children) == 0 {
		c.errorAt(node, "Function '%s' is fallible (returns (value, string)): use 'return value, err' with err null on success", c.funcName)
		return
	}
	if len(node.Children) != 2 {
		// Let existing arity checks handle it; just require 2 values.
		if len(node.Children) == 1 {
			c.errorAt(node, "Function '%s' is fallible: return 2 values (value, err); success must return null as err", c.funcName)
		}
		return
	}
	errNode := node.Children[1]
	if safetyIsEmptyStringLit(errNode) {
		c.errorAt(errNode, "Fallible '%s': success must return null, not \"\" (empty string is truthy and reads as an error); failure returns a message", c.funcName)
	}
}

// safetyNoteFallibleCall records val/err pair for must-check.
func (c *checker) safetyNoteFallibleCall(targets *ASTNode, call *ASTNode) {
	if targets == nil || call == nil || call.Type != NODE_CALL {
		return
	}
	if !c.safetyFallible(call.Value) {
		return
	}
	if len(targets.Children) != 2 {
		return
	}
	val, err := targets.Children[0], targets.Children[1]
	if val == nil || err == nil || val.Type != NODE_IDENTIFIER || err.Type != NODE_IDENTIFIER {
		return
	}
	if err.Value == "" || err.Value == "_" {
		return
	}
	// Only err-named or *_try calls enforce must-check. Plain (bool, string)
	// pairs like `ok, label := pair()` are ordinary multi-returns, not errors.
	if !safetyIsTryName(call.Value) && !safetyIsErrName(err.Value) {
		return
	}
	// _ discards the check explicitly.
	if val.Value != "" && val.Value != "_" {
		c.valFromFallible[val.Value] = err.Value
	}
	c.errUnchecked[err.Value] = true
	delete(c.errChecked, err.Value)
}

// safetyNoteErrChecked marks err as checked when a condition mentions it.
func (c *checker) safetyNoteErrChecked(cond *ASTNode) {
	if cond == nil {
		return
	}
	for err := range c.errUnchecked {
		if safetyIsErrCheckCond(cond, err) {
			c.errChecked[err] = true
			delete(c.errUnchecked, err)
			// value is now safe to use after the check
			for v, e := range c.valFromFallible {
				if e == err {
					delete(c.valFromFallible, v)
				}
			}
		}
	}
}

// safetyCheckValUse warns when a fallible value is used before its err is checked.
// It is a warning (not error) because the value may be used inside the
// `if not err` success branch where it is already safe; the end-of-function
// unchecked-err error is the enforcing check.
func (c *checker) safetyCheckValUse(node *ASTNode, name string) {
	if name == "" {
		return
	}
	if err, ok := c.valFromFallible[name]; ok {
		if _, stillUnchecked := c.errUnchecked[err]; stillUnchecked {
			c.warningAt(node, "Value '%s' from fallible call is used before checking '%s'; add 'guard %s is null else return ... $' or 'if %s then ...' first", name, err, err, err)
		}
	}
}

// safetyCheckUnconsumedErr reports unchecked errs at function end.
func (c *checker) safetyCheckUnconsumedErr(fnNode *ASTNode) {
	for err := range c.errUnchecked {
		if _, ok := c.errChecked[err]; !ok {
			c.errorAt(fnNode, "Unchecked error '%s' from fallible call; check it with 'if %s then ... $' or 'guard %s is null else ... $' before returning", err, err, err)
		}
	}
}

func safetyIsNegativeNumber(node *ASTNode) bool {
	if node == nil {
		return false
	}
	if node.Type == NODE_NUMBER {
		v := strings.TrimSpace(node.Value)
		if strings.HasPrefix(v, "-") {
			return true
		}
		if n, err := strconv.ParseFloat(v, 64); err == nil && n < 0 {
			return true
		}
	}
	if node.Type == NODE_UNARY_OP && node.Value == "-" && len(node.Children) > 0 {
		if node.Children[0].Type == NODE_NUMBER {
			return true
		}
	}
	return false
}

func safetyIsHugeNumber(node *ASTNode) bool {
	if node == nil || node.Type != NODE_NUMBER {
		return false
	}
	v := strings.TrimSpace(node.Value)
	// 0x prefix already hex; parse generously
	var n float64
	var err error
	if strings.HasPrefix(v, "0x") || strings.HasPrefix(v, "0X") {
		var u uint64
		u, err = strconv.ParseUint(v[2:], 16, 64)
		n = float64(u)
	} else {
		n, err = strconv.ParseFloat(v, 64)
	}
	if err != nil {
		return false
	}
	return n > 1073741824 // 1GiB elements/bytes is never a legit single buffer
}

// Valid builtin methods per type (must match pkg/builtins.go + source/stdlib.go + LSP).
func validStringMethods() map[string]bool {
	return map[string]bool{
		"length": true, "upper": true, "lower": true, "replace": true, "contains": true, "contains_ci": true,
		"camel_case": true, "snake_case": true, "pascal_case": true, "kebab_case": true,
		"match": true, "split": true, "count": true, "lpad": true, "rpad": true, "pad": true,
		"strip": true, "get_file": true, "dup": true, "dupe": true, "prefix": true,
		"byte_at": true, "byte_len": true, "byte_slice": true, "insert_at": true, "delete_range": true, "utf8_len": true,
	}
}

func validArrayMethods() map[string]bool {
	return map[string]bool{
		"length": true, "push": true, "pop": true, "sort": true, "sort_strings": true, "reverse": true, "contains": true,
		"find": true, "filter": true, "map": true, "join": true, "slice": true, "sum": true, "has": true,
		"fill": true, "shuffle": true, "pick": true, "index_of": true, "reduce": true, "insert": true,
		"append": true, "erase": true, "dup": true, "dupe": true, "clear": true, "size": true,
		"any": true, "all": true, "count_if": true, "find_index": true, "remove": true,
	}
}

func validDictMethods() map[string]bool {
	return map[string]bool{
		"size": true, "clear": true, "has": true, "has_all": true, "keys": true, "values": true,
		"sort": true, "stable_sort": true, "merge": true,
	}
}

func isValidBuiltinMethod(recvType, method string) bool {
	// Normalize error -> string, own prefix, etc.
	base := recvType
	if len(base) > 4 && base[:4] == "own " {
		base = base[4:]
	}
	if base == "error" {
		base = "string"
	}
	if base == "string" || base == "char*" || base == "const char*" {
		return validStringMethods()[method]
	}
	if base == "array" || len(base) >= 6 && base[:6] == "array[" {
		return validArrayMethods()[method]
	}
	if base == "dict" || len(base) >= 5 && (base[:5] == "dict[" || base[:5] == "dict<") {
		return validDictMethods()[method]
	}
	if len(base) >= 7 && base[:7] == "buffer[" {
		return method == "len" || method == "length"
	}
	return true // unknown types skip (lenient)
}
