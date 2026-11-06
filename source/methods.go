package main

import (
	ahoy "ahoy"
	"fmt"
	"strings"
)

// User-defined method resolution for generateMethodCall.
//
// A call recv.m|args| resolves against the method table (built once from the
// whole program AST) when the receiver's inferred type names a type with that
// method; it emits the mangled Type_method function with the receiver
// prepended. A call Type.m|args| where the receiver position names a type
// calls an associated function with no receiver.

// resolveUserMethod maps recv.m|args| onto a user-defined method or associated
// function. It returns the mangled global name and whether the receiver is
// passed as the first argument. The receiver type prefers the parse-time
// record on the call node, falling back to inference.
func (gen *CodeGenerator) resolveUserMethod(call *ahoy.ASTNode) (string, bool, bool) {
	if call == nil || len(call.Children) < 1 {
		return "", false, false
	}
	object := call.Children[0]
	methodName := call.Value
	// Associated call: the receiver position names a type, not a value.
	if object.Type == ahoy.NODE_IDENTIFIER {
		if _, isStruct := gen.structs[object.Value]; isStruct {
			if e, ok := gen.methodTable[object.Value][methodName]; ok && e.Info.Associated {
				return ahoy.MangleMethodName(object.Value, methodName), false, true
			}
		} else if _, isEnum := gen.enums[object.Value]; isEnum {
			if e, ok := gen.methodTable[object.Value][methodName]; ok && e.Info.Associated {
				return ahoy.MangleMethodName(object.Value, methodName), false, true
			}
		}
	}
	recvType := call.RecvType
	if recvType == "" {
		recvType = ahoy.NormalizeReceiverType(gen.inferType(object))
	}
	if e, ok := gen.methodTable[recvType][methodName]; ok && !e.Info.Associated {
		return ahoy.MangleMethodName(recvType, methodName), true, true
	}
	return "", false, false
}

// freshStringBuiltin reports whether a free-function call by this name
// always returns caller-owned heap (the sprintf family, fresh-string
// stdlib builtins, and fresh-returning C helpers). Shared with
// valueAllocatesString and the funcReturnsOwnedString predicate below.
//
// Every name here was audited to malloc/strdup on ALL paths: a borrowed or
// static return on any path would turn the caller's scope-exit free into a
// use-after-free. Deliberately ABSENT (borrowed or mixed, never tracked):
// ahoy_sb_text (builder memory), ahoy_loc_tr / ahoy_fs_name /
// ahoy_read_text / ahoy_text_copy / ahoy_loc_lang_name / ahoy_anim_name /
// ahoy_audio_key / ahoy_shader_error / ahoy_vram_row_name /
// ahoy_vram_row_kind / ahoy_shader_uniform_name /
// ahoy_shader_uniform_default (statics or interior pointers), and
// ahoy_model_convert / ahoy_text_alloc (static on some paths).
func freshStringBuiltin(name string) bool {
	switch name {
	case "sprintf", "format", "from_code", "errno_str", "time_format",
		"path_join", "path_parent", "path_name",
		"fs_read_text",
		"clock_string", "cwd", "exe_dir",
		"ahoy_text_prefix", "ahoy_text_from",
		"ahoy_str_append_char", "ahoy_str_backspace",
		"ahoy_fs_path", "ahoy_path_join",
		"ahoy_lint_capture", "ahoy_strip_comments",
		"ahoy_rename_entry", "ahoy_rename_varying",
		"ahoy_strndup", "ahoy_model_bone_name":
		return true
	}
	return false
}

// funcReturnsOwnedString reports whether every string value returned by the
// free function fn is caller-owned heap, so callers must free the result.
// This mirrors the return-site promotion in generateReturnStatement:
// literals, identifiers, container accesses, concats, slices, owned
// f-strings, and known-fresh calls are promoted or fresh; C/foreign calls,
// ternaries, and switch-expression returns are borrowed (or static) and
// must NOT be freed by the caller.
//
// In particular a wrapper like `loc_tr` (`return ahoy_loc_tr|text|`, where
// the C helper returns its key or a table cell) is BORROWED: tracking and
// freeing its result corrupts the owner (arena game heap-use-after-free,
// Nov 2026). Likewise switch-returners like `kind_icon_name` hand out
// static literals. Unknown shapes are conservatively borrowed (bounded
// leak, never use-after-free).
func (gen *CodeGenerator) funcReturnsOwnedString(fn string) bool {
	if gen.funcOwnedStringCache == nil {
		gen.funcOwnedStringCache = map[string]bool{}
	}
	if gen.funcOwnedStringBusy == nil {
		gen.funcOwnedStringBusy = map[string]bool{}
	}
	if v, ok := gen.funcOwnedStringCache[fn]; ok {
		return v
	}
	if gen.funcOwnedStringBusy[fn] {
		return false
	}
	gen.funcOwnedStringBusy[fn] = true
	owned := gen.computeFuncReturnsOwnedString(fn)
	delete(gen.funcOwnedStringBusy, fn)
	gen.funcOwnedStringCache[fn] = owned
	return owned
}

func (gen *CodeGenerator) computeFuncReturnsOwnedString(fn string) bool {
	body, ok := gen.funcBodies[fn]
	if !ok || len(body.Children) < 2 {
		return false
	}
	returns := 0
	owned := true
	var walk func(n *ahoy.ASTNode, depth int)
	walk = func(n *ahoy.ASTNode, depth int) {
		if n == nil || depth > 16 || !owned {
			return
		}
		if n.Type == ahoy.NODE_RETURN_STATEMENT {
			returns++
			// Bare or multi-value returns are not single owned strings.
			if len(n.Children) != 1 || !gen.stringReturnYieldsOwned(n.Children[0], depth+1) {
				owned = false
			}
			return
		}
		for _, c := range n.Children {
			walk(c, depth)
		}
	}
	walk(body.Children[1], 0)
	return returns > 0 && owned
}

// stringReturnYieldsOwned reports whether a returned expression evaluates
// to caller-owned heap under the v1 return contract (see
// funcReturnsOwnedString). Direct identifier returns are included: the
// return site duplicates borrowed/static names (or transfers an already
// owned local, whose callee-side free is suppressed by escape marking).
func (gen *CodeGenerator) stringReturnYieldsOwned(n *ahoy.ASTNode, depth int) bool {
	if n == nil || depth > 16 {
		return false
	}
	switch n.Type {
	case ahoy.NODE_STRING, ahoy.NODE_RAW_STRING,
		ahoy.NODE_IDENTIFIER,
		ahoy.NODE_MEMBER_ACCESS, ahoy.NODE_DICT_ACCESS,
		ahoy.NODE_OBJECT_ACCESS, ahoy.NODE_ARRAY_ACCESS,
		ahoy.NODE_STRING_SLICE:
		return true
	case ahoy.NODE_BINARY_OP:
		return n.Value == "+" && isStringLikeType(gen.inferType(n))
	case ahoy.NODE_F_STRING:
		// Return position forces the owned helper (inReturnExpr), so a
		// returned f-string is always a heap copy in string functions.
		return true
	case ahoy.NODE_METHOD_CALL:
		return ahoy.FreshStringMethods[n.Value]
	case ahoy.NODE_CALL:
		if freshStringBuiltin(n.Value) {
			return true
		}
		if gen.userFunctions[n.Value] {
			return gen.funcReturnsOwnedString(n.Value)
		}
		// C/foreign/unknown calls may hand out borrowed storage.
		return false
	default:
		// Ternaries, switch expressions, and unknown shapes: borrowed-or-
		// unknown, never reported owned.
		return false
	}
}

// valueAllocatesString reports whether evaluating this expression produces a
// freshly heap-allocated string the owner must free: string concatenation
// (string operands, including the string + char form), sprintf, and
// fresh-returning string methods.
// Literals, f-strings (static buffer), aliases, member/array/dict access,
// user and C calls (which may return shared statics) never allocate here.
func (gen *CodeGenerator) valueAllocatesString(node *ahoy.ASTNode) bool {
	if node == nil {
		return false
	}
	switch node.Type {
	case ahoy.NODE_BINARY_OP:
		if node.Value != "+" {
			return false
		}
		if len(node.Children) < 2 {
			return false
		}
		left, right := node.Children[0], node.Children[1]
		if gen.isStringExpr(left) && gen.isStringExpr(right) {
			return true
		}
		return gen.isCharExpr(left) != gen.isCharExpr(right) &&
			(gen.isStringExpr(left) || gen.isStringExpr(right))
	case ahoy.NODE_STRING_SLICE:
		return true
	case ahoy.NODE_CALL:
		// sprintf plus the fresh-string stdlib builtins: every one of
		// these returns caller-owned heap (never a static buffer), so
		// the result is tracked and freed at scope exit like any other
		// fresh string. Predicates and int/float builtins are absent:
		// they allocate nothing.
		if freshStringBuiltin(node.Value) {
			return true
		}
		// User functions: ownership follows the return-site promotion,
		// modeled by funcReturnsOwnedString (emission-consistent). Wrappers
		// around C calls, ternaries, and switch-expression returns are
		// borrowed even though they return strings. Unknown callees fall
		// back to the alias verdict below (borrowed => not tracked).
		if _, known := gen.funcBodies[node.Value]; known {
			return gen.funcReturnsOwnedString(node.Value)
		}
		// Alias/passthrough callees borrow their argument or a global:
		// the caller must NOT free the result (see callAliasKind).
		if gen.callAliasKind(node.Value).isAlias {
			return false
		}
		if gen.isOwnedStringReturningCall(node) {
			return true
		}
		return false
	case ahoy.NODE_METHOD_CALL:
		return ahoy.FreshStringMethods[node.Value]
	}
	return false
}

// isStringLikeType reports whether a declared or inferred type string is a
// string flavor that can hold heap memory.
func isStringLikeType(t string) bool {
	switch t {
	case "string", "char*", "char *", "const char*", "const char *":
		return true
	}
	return false
}

// collectDeferredIdents gathers every identifier name mentioned in a deferred
// statement. Anything pending must outlive the cleanup that would otherwise
// free it, so these names suppress scope cleanups until their deferral runs.
// Over-approximate by design (a name mentioned anywhere counts): the failure
// mode is a bounded leak, never a dangling reference.
func collectDeferredIdents(node *ahoy.ASTNode, into map[string]bool) {
	if node == nil || into == nil {
		return
	}
	if node.Type == ahoy.NODE_IDENTIFIER && node.Value != "" && node.Value != "_" {
		into[node.Value] = true
	}
	// A container access carries its base variable in Value, not in a child
	// identifier (`xs[0]`, `d<"k">`, `o.field`). Without this the base is
	// missed, so a return like `return xs[0].strip||` does not mark the
	// receiver: the return-path cleanup then frees the container before the
	// return expression reads it (use-after-free).
	if (node.Type == ahoy.NODE_ARRAY_ACCESS || node.Type == ahoy.NODE_DICT_ACCESS ||
		node.Type == ahoy.NODE_MEMBER_ACCESS || node.Type == ahoy.NODE_OBJECT_ACCESS) &&
		node.Value != "" && node.Value != "_" {
		into[node.Value] = true
	}
	for _, child := range node.Children {
		collectDeferredIdents(child, into)
	}
}

// markReturnedValue marks the variables whose storage escapes through a
// return value: bare identifiers, values stored into returned literals, and
// ternary branches. Reads (member/array access of primitives, operators,
// calls) do not transfer storage — except accesses that may alias heap,
// whose base must survive (see accessYieldsPrimitive).
func (gen *CodeGenerator) markReturnedValue(node *ahoy.ASTNode) {
	if node == nil {
		return
	}
	switch node.Type {
	case ahoy.NODE_IDENTIFIER:
		gen.escapingVars[node.Value] = true
		gen.markLambdaCapturesOf(node.Value)
	case ahoy.NODE_LAMBDA:
		// A directly returned closure carries its captures with it.
		gen.markLambdaCaptures(node)
	case ahoy.NODE_ARRAY_LITERAL, ahoy.NODE_DICT_LITERAL, ahoy.NODE_OBJECT_LITERAL:
		for _, child := range node.Children {
			gen.markReturnedValue(child)
		}
	case ahoy.NODE_TERNARY:
		for _, child := range node.Children {
			gen.markReturnedValue(child)
		}
	case ahoy.NODE_MEMBER_ACCESS, ahoy.NODE_OBJECT_ACCESS, ahoy.NODE_ARRAY_ACCESS, ahoy.NODE_DICT_ACCESS:
		if !gen.accessYieldsPrimitive(node) {
			if root := accessRootIdentifier(node); root != "" {
				gen.escapingVars[root] = true
			}
		}
	}
}

// accessRootIdentifier walks an access chain to its base identifier, or ""
// when the base is not a plain name (e.g. a call result).
func accessRootIdentifier(node *ahoy.ASTNode) string {
	for node != nil {
		if node.Type == ahoy.NODE_IDENTIFIER {
			return node.Value
		}
		if node.Type == ahoy.NODE_ARRAY_ACCESS && node.Value != "" {
			return node.Value
		}
		if node.Type == ahoy.NODE_DICT_ACCESS && node.Value != "" {
			return node.Value
		}
		if node.Type == ahoy.NODE_OBJECT_ACCESS && node.Value != "" {
			return node.Value
		}
		if len(node.Children) == 0 {
			return ""
		}
		node = node.Children[0]
	}
	return ""
}

// accessYieldsPrimitive reports whether reading through this access yields a
// plain value (int/float/bool/char) rather than possibly-heap storage. Only
// proven primitives skip protection; anything else (strings, containers,
// unknown) keeps its base alive.
func (gen *CodeGenerator) accessYieldsPrimitive(node *ahoy.ASTNode) bool {
	switch gen.inferType(node) {
	case "int", "float", "double", "bool", "char", "intptr_t", "void":
		return true
	}
	return false
}

// returnWouldFreeMentioned reports whether any name in the set is a heap
// local the return-path cleanups would free. Callers stash the return value
// first when true, so the cleanups cannot run before the value is read.
// The conditions mirror the cleanup skips exactly.
func (gen *CodeGenerator) returnWouldFreeMentioned(names map[string]bool) bool {
	for name := range names {
		if !gen.heapAllocatedVars[name] {
			continue
		}
		if gen.escapingVars[name] || gen.manuallyFreedVars[name] || gen.autoFreedVars[name] || gen.deferredUsers[name] {
			continue
		}
		if gen.heapVarTypes[name] == "" {
			continue
		}
		return true
	}
	return false
}

// bufferedNode generates an expression node into a string instead of the
// output stream, preserving evaluation order and side effects. Callers emit
// the captured text exactly once.
func (gen *CodeGenerator) bufferedNode(node *ahoy.ASTNode) string {
	savedOutput := gen.output
	savedIndent := gen.indent
	gen.output = strings.Builder{}
	gen.indent = 0
	gen.generateNode(node)
	text := gen.output.String()
	gen.output = savedOutput
	gen.indent = savedIndent
	return text
}

// markLambdaCaptures marks the heap variables a lambda value closes over.
// Closure environments copy pointers shallowly, so when the lambda value
// escapes its captures' scope (returned, stored, pushed, submitted) the
// captures must survive like any escaping value (they leak, soundly).
// Only currently heap-tracked names are marked, so stale entries and
// non-heap names (params, ints, globals) are harmless no-ops.
func (gen *CodeGenerator) markLambdaCaptures(lambdaNode *ahoy.ASTNode) {
	if lambdaNode == nil || lambdaNode.Type != ahoy.NODE_LAMBDA {
		return
	}
	// Lambda params live in the first Value children; ahoy has no shadowing
	// so every param name anywhere in the value is bound, never captured.
	params := map[string]bool{}
	var walkParams func(n *ahoy.ASTNode)
	walkParams = func(n *ahoy.ASTNode) {
		if n == nil {
			return
		}
		if n.Type == ahoy.NODE_LAMBDA {
			count := 0
			fmt.Sscanf(n.Value, "%d", &count)
			for i := 0; i < count && i < len(n.Children); i++ {
				if n.Children[i].Type == ahoy.NODE_IDENTIFIER {
					params[n.Children[i].Value] = true
				}
			}
		}
		for _, c := range n.Children {
			walkParams(c)
		}
	}
	walkParams(lambdaNode)
	var walk func(n *ahoy.ASTNode)
	walk = func(n *ahoy.ASTNode) {
		if n == nil {
			return
		}
		if n.Type == ahoy.NODE_IDENTIFIER && n.Value != "" && n.Value != "_" &&
			!params[n.Value] && gen.heapAllocatedVars[n.Value] {
			gen.escapingVars[n.Value] = true
		}
		// A container access carries its variable in Value, not in a
		// child identifier (xs[0], d<"k">) — same as capture collection.
		if (n.Type == ahoy.NODE_ARRAY_ACCESS || n.Type == ahoy.NODE_DICT_ACCESS ||
			n.Type == ahoy.NODE_MEMBER_ACCESS || n.Type == ahoy.NODE_OBJECT_ACCESS) &&
			n.Value != "" && !params[n.Value] && gen.heapAllocatedVars[n.Value] {
			gen.escapingVars[n.Value] = true
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(lambdaNode)
	walk = func(n *ahoy.ASTNode) {
		if n == nil {
			return
		}
		if n.Type == ahoy.NODE_IDENTIFIER && n.Value != "" && n.Value != "_" &&
			!params[n.Value] && gen.heapAllocatedVars[n.Value] {
			gen.escapingVars[n.Value] = true
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(lambdaNode)
}

// markLambdaCapturesOf marks the captures of the lambda value bound to a
// variable name, if any. Call after marking the name itself escaping.
func (gen *CodeGenerator) markLambdaCapturesOf(varName string) {
	if node, ok := gen.lambdaValues[varName]; ok {
		gen.markLambdaCaptures(node)
	}
}

// collectStoredNames gathers the variable names whose storage a value node
// may keep a pointer to: identifiers, literal elements, ternary/switch
// branch values, and (transitively) anything those resolve to. Skipped:
// calls/methods (callees borrow transiently), accesses (elements are owned
// by whoever filled the container, independently of its structure),
// unary/binary/f-strings (borrow or copy immediately), dict keys (copied),
// ternary conditions and switch subjects/matchers (compared, not kept).
// Lambdas are descended (their captures are kept) and reported separately
// so callers can run markLambdaCaptures over them (access-base captures).
func collectStoredNames(node *ahoy.ASTNode, out map[string]bool, lambdas *[]*ahoy.ASTNode) {
	if node == nil || out == nil {
		return
	}
	switch node.Type {
	case ahoy.NODE_IDENTIFIER:
		if node.Value != "" && node.Value != "_" {
			out[node.Value] = true
		}
	case ahoy.NODE_ARRAY_LITERAL, ahoy.NODE_DICT_LITERAL, ahoy.NODE_OBJECT_LITERAL:
		for _, child := range node.Children {
			collectStoredNames(child, out, lambdas)
		}
	case ahoy.NODE_OBJECT_PROPERTY:
		for _, child := range node.Children {
			collectStoredNames(child, out, lambdas)
		}
	case ahoy.NODE_TERNARY:
		for i, child := range node.Children {
			if i == 0 {
				continue
			}
			collectStoredNames(child, out, lambdas)
		}
	case ahoy.NODE_SWITCH_STATEMENT:
		for i, child := range node.Children {
			if i == 0 {
				continue
			}
			if len(child.Children) > 1 {
				collectStoredNames(child.Children[1], out, lambdas)
			} else {
				collectStoredNames(child, out, lambdas)
			}
		}
	case ahoy.NODE_LAMBDA:
		if lambdas != nil {
			*lambdas = append(*lambdas, node)
		}
		for _, child := range node.Children {
			collectStoredNames(child, out, lambdas)
		}
	case ahoy.NODE_CALL, ahoy.NODE_METHOD_CALL,
		ahoy.NODE_MEMBER_ACCESS, ahoy.NODE_ARRAY_ACCESS,
		ahoy.NODE_DICT_ACCESS, ahoy.NODE_OBJECT_ACCESS,
		ahoy.NODE_UNARY_OP, ahoy.NODE_BINARY_OP, ahoy.NODE_F_STRING:
		return
	default:
		for _, child := range node.Children {
			collectStoredNames(child, out, lambdas)
		}
	}
}

// selectHasHeapAlias reports whether a ternary/switch value may alias a heap
// variable through one of its branches. Such a value owns no storage of its
// own: tracking it as fresh would free its owner's memory (double-free) or
// dangle on escape. Fresh-branch-only selects own their (untracked) temps.
func (gen *CodeGenerator) selectHasHeapAlias(node *ahoy.ASTNode) bool {
	if node == nil || (node.Type != ahoy.NODE_TERNARY && node.Type != ahoy.NODE_SWITCH_STATEMENT) {
		return false
	}
	names := map[string]bool{}
	collectStoredNames(node, names, nil)
	for m := range names {
		if gen.heapAllocatedVars[m] {
			return true
		}
	}
	return false
}

// keep a pointer to, following value bindings transitively (aliases of
// aliases). Stale bindings are harmless: names no longer heap-tracked are
// skipped, and use-before-declaration rules out reference cycles.
func (gen *CodeGenerator) markValueDeps(node *ahoy.ASTNode) {
	if node == nil {
		return
	}
	visited := map[string]bool{}
	stack := []*ahoy.ASTNode{node}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		// Alias-callee results borrow: protect heap identifier arguments
		// (the result may alias any of them; over-marking is leak-safe).
		// Fresh callees borrow transiently and need nothing.
		if n.Type == ahoy.NODE_CALL && n.Value != "" {
			if info := gen.callAliasKind(n.Value); info.isAlias && info.isParam {
				for _, a := range n.Children {
					if a != nil && a.Type == ahoy.NODE_IDENTIFIER && a.Value != "" && a.Value != "_" {
						if !visited[a.Value] {
							visited[a.Value] = true
							if gen.heapAllocatedVars[a.Value] {
								gen.escapingVars[a.Value] = true
							}
							if b, ok := gen.valueBindings[a.Value]; ok && b != nil {
								stack = append(stack, b)
							}
						}
					}
				}
			}
			continue
		}
		names := map[string]bool{}
		var lambdas []*ahoy.ASTNode
		collectStoredNames(n, names, &lambdas)
		for _, l := range lambdas {
			gen.markLambdaCaptures(l)
		}
		for m := range names {
			if visited[m] {
				continue
			}
			visited[m] = true
			if gen.heapAllocatedVars[m] {
				gen.escapingVars[m] = true
			}
			if b, ok := gen.valueBindings[m]; ok && b != nil {
				stack = append(stack, b)
			}
		}
	}
}

// callAliasInfo classifies what a call result may alias: fresh (default,
// caller owns), param (aliases an argument — mark args on escape), or
// static (aliases a global/field — immortal, nothing to mark).
type callAliasInfo struct {
	isAlias  bool
	isParam  bool
	isStatic bool
}

// collectFuncBodies records free-function ASTs for alias-return analysis.
// Methods are skipped (method calls resolve separately without bodies).
func (gen *CodeGenerator) collectFuncBodies(root *ahoy.ASTNode) {
	if root == nil {
		return
	}
	for _, child := range root.Children {
		if child != nil && child.Type == ahoy.NODE_FUNCTION && child.Method == nil {
			if child.Value != "" {
				gen.funcBodies[child.Value] = child
			}
		}
	}
}

// callAliasKind determines whether calls to fn may return storage owned
// elsewhere: a parameter (borrowed from the caller), a global/field
// (immortal), or fresh values (caller owns). Conservative: any alias
// return poisons; single-hop call results inherit transitively.
func (gen *CodeGenerator) callAliasKind(fn string) callAliasInfo {
	if cached, ok := gen.callAliasCache[fn]; ok {
		return cached
	}
	info := callAliasInfo{}
	gen.callAliasCache[fn] = info
	body, ok := gen.funcBodies[fn]
	if !ok || len(body.Children) < 2 {
		return info
	}
	params := map[string]bool{}
	if len(body.Children) > 0 && body.Children[0] != nil {
		for _, p := range body.Children[0].Children {
			if p != nil && p.Type == ahoy.NODE_IDENTIFIER && p.Value != "" {
				params[p.Value] = true
			}
		}
	}
	locals := map[string]bool{}
	foundParam, foundStatic := false, false
	var walk func(n *ahoy.ASTNode, depth int)
	walk = func(n *ahoy.ASTNode, depth int) {
		if n == nil || depth > 8 {
			return
		}
		if n.Type == ahoy.NODE_RETURN_STATEMENT {
			for _, child := range n.Children {
				gen.classifyReturnValue(child, params, locals, body, &foundParam, &foundStatic, depth+1)
			}
			return
		}
		if n.Type == ahoy.NODE_VARIABLE_DECLARATION {
			if n.Value != "" {
				locals[n.Value] = true
			}
		}
		for _, c := range n.Children {
			walk(c, depth)
		}
	}
	walk(body.Children[1], 0)
	if foundParam {
		info = callAliasInfo{isAlias: true, isParam: true}
	} else if foundStatic {
		info = callAliasInfo{isAlias: true, isStatic: true}
	}
	gen.callAliasCache[fn] = info
	return info
}

// classifyReturnValue checks one returned expression for aliasing.
// Param identifiers (or accesses rooted in one) borrow the caller's
// argument; globals/fields are immortal; locals transfer fresh ownership.
// Selects recurse branches; calls inherit single-hop via the cache.
func (gen *CodeGenerator) classifyReturnValue(n *ahoy.ASTNode, params, locals map[string]bool, body *ahoy.ASTNode, foundParam, foundStatic *bool, depth int) {
	if n == nil || depth > 8 || (*foundParam) {
		return
	}
	switch n.Type {
	case ahoy.NODE_IDENTIFIER:
		if n.Value == "" || n.Value == "_" {
			return
		}
		if params[n.Value] {
			*foundParam = true
		} else if !locals[n.Value] {
			*foundStatic = true
		}
	case ahoy.NODE_MEMBER_ACCESS, ahoy.NODE_DICT_ACCESS, ahoy.NODE_OBJECT_ACCESS:
		if n.Value != "" {
			if params[n.Value] {
				*foundParam = true
			} else if !locals[n.Value] {
				*foundStatic = true
			}
		}
	case ahoy.NODE_ARRAY_ACCESS:
		*foundStatic = true
	case ahoy.NODE_TERNARY:
		for i, child := range n.Children {
			if i == 0 {
				continue
			}
			gen.classifyReturnValue(child, params, locals, body, foundParam, foundStatic, depth+1)
		}
	case ahoy.NODE_CALL:
		if n.Value != "" && n.Value != body.Value {
			sub := gen.callAliasKind(n.Value)
			if sub.isParam {
				*foundParam = true
			} else if sub.isStatic {
				*foundStatic = true
			}
		}
	default:
		for _, c := range n.Children {
			gen.classifyReturnValue(c, params, locals, body, foundParam, foundStatic, depth+1)
		}
	}
}
