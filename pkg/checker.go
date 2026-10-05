package ahoy

import (
	"fmt"
	"strconv"
	"strings"
)

// Semantic checks that need the whole program rather than a single statement.
//
// These run after parsing, from the lint/validation entry points, so that a
// call to a function declared later in the file, or in another file of the same
// program, is not mistaken for an undefined name.
//
// Scope model: Ahoy is block scoped - a variable declared inside an if or loop
// body is gone once that block ends, which is also what the generated C does.
// The code generator, however, emits a C declaration only the first time a name
// appears anywhere in a function, so re-declaring a name in a scope where an
// earlier declaration is no longer visible produces an assignment to a variable
// C cannot see. That case is reported here rather than left to the C compiler.

type checker struct {
	p      *Parser
	errors []ParseError

	// scopes is the stack of open blocks; scopes[0] is the global scope.
	scopes []map[string]bool
	// programNames holds names declared elsewhere in the same program, so a
	// multi-file program resolves names across its files.
	programNames map[string]bool

	funcName   string
	inFunction bool
}

// checkSemantics walks a parsed program and appends diagnostics to p.Errors.
// programNames may be nil; it carries the top-level names declared by the other
// files of the same program.
func (p *Parser) checkSemantics(ast *ASTNode, programNames []string) {
	if ast == nil {
		return
	}
	c := &checker{
		p:            p,
		scopes:       []map[string]bool{{}},
		programNames: make(map[string]bool, len(programNames)),
	}
	for _, name := range programNames {
		c.programNames[name] = true
	}
	for _, child := range ast.Children {
		c.checkTopLevel(child)
	}
	p.Errors = append(p.Errors, c.errors...)
}

// DeclaredNames returns the top-level names an AST declares: functions, structs,
// enums, constants, aliases, unions and global variables. It is how one file of
// a program learns the names the other files define.
func DeclaredNames(ast *ASTNode) []string {
	if ast == nil {
		return nil
	}
	out := make([]string, 0, len(ast.Children))
	for _, child := range ast.Children {
		switch child.Type {
		case NODE_FUNCTION, NODE_STRUCT_DECLARATION, NODE_ENUM_DECLARATION,
			NODE_CONSTANT_DECLARATION, NODE_ALIAS_DECLARATION,
			NODE_UNION_DECLARATION, NODE_VARIABLE_DECLARATION, NODE_ASSIGNMENT:
			if child.Value != "" {
				out = append(out, child.Value)
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- reporting

func (c *checker) errorAt(node *ASTNode, format string, args ...any) {
	if node == nil {
		return
	}
	line, column := node.Line, node.Column
	if line <= 0 {
		// Operator and ternary nodes carry no position of their own; fall back
		// to the first operand that does, so the diagnostic is not reported at
		// 0:0.
		if pos := firstPositioned(node); pos != nil {
			line, column = pos.Line, pos.Column
		}
	}
	c.errors = append(c.errors, ParseError{
		Message: fmt.Sprintf(format, args...),
		Line:    line,
		Column:  column,
	})
}

// firstPositioned returns the first node in a subtree that has a source line.
func firstPositioned(node *ASTNode) *ASTNode {
	for _, child := range node.Children {
		if child.Line > 0 {
			return child
		}
		if pos := firstPositioned(child); pos != nil {
			return pos
		}
	}
	return nil
}

// ---------------------------------------------------------------- scopes

func (c *checker) pushScope() { c.scopes = append(c.scopes, map[string]bool{}) }

func (c *checker) popScope() {
	if len(c.scopes) > 1 {
		c.scopes = c.scopes[:len(c.scopes)-1]
	}
}

func (c *checker) declare(name string) {
	if name == "" || name == "_" {
		return
	}
	c.scopes[len(c.scopes)-1][name] = true
}

func (c *checker) isVisible(name string) bool {
	for i := len(c.scopes) - 1; i >= 0; i-- {
		if c.scopes[i][name] {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- resolution

// isDefined reports whether a name resolves to something the generated C can
// reference.
func (c *checker) isDefined(name string) bool {
	if name == "" || name == "_" {
		return true
	}
	if c.isVisible(name) {
		return true
	}
	if c.programNames[name] {
		return true
	}
	p := c.p
	if _, ok := p.variableTypes[name]; ok {
		return true
	}
	if _, ok := p.constants[name]; ok {
		return true
	}
	if _, ok := p.structs[name]; ok {
		return true
	}
	if _, ok := p.enums[name]; ok {
		return true
	}
	if _, ok := p.typeAliases[name]; ok {
		return true
	}
	if _, ok := p.unionTypes[name]; ok {
		return true
	}
	if _, ok := p.functions[name]; ok {
		return true
	}
	if p.zeroArgFunctions[name] {
		return true
	}
	if builtinCallNames[name] || builtinMethodNames[name] {
		return true
	}
	return p.isCHeaderSymbol(name)
}

// isCHeaderSymbol reports whether name is exported by an imported C header.
func (p *Parser) isCHeaderSymbol(name string) bool {
	headers := make([]*CHeaderInfo, 0, len(p.cHeaders)+1)
	if p.cHeaderGlobal != nil {
		headers = append(headers, p.cHeaderGlobal)
	}
	for _, h := range p.cHeaders {
		if h != nil {
			headers = append(headers, h)
		}
	}
	for _, h := range headers {
		if _, ok := h.Defines[name]; ok {
			return true
		}
		if _, ok := h.Structs[name]; ok {
			return true
		}
		if _, ok := h.Typedefs[name]; ok {
			return true
		}
		if _, ok := h.Functions[name]; ok {
			return true
		}
		if _, ok := h.Enums[name]; ok {
			return true
		}
		// Enum members and #defines are referenced as bare names.
		for _, enum := range h.Enums {
			if _, ok := enum.Values[name]; ok {
				return true
			}
		}
	}
	return false
}

// isCallable reports whether name is a function the program can call.
func (c *checker) isCallable(name string) bool {
	if name == "" {
		return true
	}
	if builtinCallNames[name] {
		return true
	}
	if c.programNames[name] {
		return true
	}
	if _, ok := c.p.functions[name]; ok {
		return true
	}
	if c.p.zeroArgFunctions[name] {
		return true
	}
	// C functions are imported as PascalCase and called in snake_case.
	for _, h := range c.cHeaders() {
		if _, ok := h.Functions[name]; ok {
			return true
		}
		for cName := range h.Functions {
			if PascalToSnake(cName) == name {
				return true
			}
		}
	}
	return false
}

func (c *checker) cHeaders() []*CHeaderInfo {
	out := make([]*CHeaderInfo, 0, len(c.p.cHeaders)+1)
	if c.p.cHeaderGlobal != nil {
		out = append(out, c.p.cHeaderGlobal)
	}
	for _, h := range c.p.cHeaders {
		if h != nil {
			out = append(out, h)
		}
	}
	return out
}

// knownNames returns every name the program can refer to, for suggestions.
func (c *checker) knownNames() []string {
	seen := map[string]bool{}
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
		}
	}
	for name := range c.p.variableTypes {
		add(name)
	}
	for name := range c.p.constants {
		add(name)
	}
	for name := range c.p.functions {
		add(name)
	}
	for name := range c.scopes[0] {
		add(name)
	}
	for i := range c.scopes {
		for name := range c.scopes[i] {
			add(name)
		}
	}
	for name := range builtinCallNames {
		add(name)
	}
	for _, h := range c.cHeaders() {
		for name := range h.Functions {
			add(name)
			add(PascalToSnake(name))
		}
		for name := range h.Defines {
			add(name)
		}
		for _, enum := range h.Enums {
			for name := range enum.Values {
				add(name)
			}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	return out
}

// suggest returns the closest known name, when it is close enough to be a
// plausible typo.
func (c *checker) suggest(name string) string {
	best := ""
	bestDistance := 1 << 30
	for _, candidate := range c.knownNames() {
		if candidate == name {
			continue
		}
		d := levenshtein(name, candidate)
		if d < bestDistance {
			bestDistance = d
			best = candidate
		}
	}
	// Only suggest genuinely similar names, and never for very short ones.
	limit := len(name) / 3
	if limit < 1 {
		limit = 1
	}
	if best != "" && bestDistance <= limit {
		return best
	}
	return ""
}

func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		curr[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(br)]
}

// ---------------------------------------------------------------- top level

func (c *checker) checkTopLevel(node *ASTNode) {
	if node == nil {
		return
	}
	switch node.Type {
	case NODE_FUNCTION:
		c.checkFunction(node)
	case NODE_CONSTANT_DECLARATION:
		c.checkExpr(childAt(node, 0))
		c.declare(node.Value)
	case NODE_STRUCT_DECLARATION, NODE_ENUM_DECLARATION, NODE_ALIAS_DECLARATION,
		NODE_UNION_DECLARATION, NODE_PROGRAM_DECLARATION, NODE_IMPORT_STATEMENT:
		// Declarations only; nothing executable to check.
	case NODE_BLOCK:
		c.checkBlock(node)
	default:
		// Global-scope statements in programs without a main function.
		c.checkStatement(node)
	}
}

func (c *checker) checkFunction(node *ASTNode) {
	if len(node.Children) < 2 {
		return
	}
	params, body := node.Children[0], node.Children[1]

	savedName := c.funcName
	savedInFunc := c.inFunction
	c.funcName = node.Value
	c.inFunction = true

	c.pushScope()
	if params != nil {
		for _, param := range params.Children {
			c.declare(param.Value)
		}
	}
	c.checkBlock(body)
	c.popScope()

	c.funcName = savedName
	c.inFunction = savedInFunc
}

// ---------------------------------------------------------------- statements

func (c *checker) checkBlock(node *ASTNode) {
	if node == nil {
		return
	}
	if node.Type != NODE_BLOCK {
		c.checkStatement(node)
		return
	}
	c.pushScope()
	for _, child := range node.Children {
		c.checkStatement(child)
	}
	c.popScope()
}

func (c *checker) checkStatement(node *ASTNode) {
	if node == nil {
		return
	}
	switch node.Type {
	case NODE_VARIABLE_DECLARATION:
		c.checkExpr(childAt(node, 0))
		c.declare(node.Value)

	case NODE_ASSIGNMENT:
		// The parser already rejects assignment to an undeclared variable.
		c.checkExpr(childAt(node, 0))

	case NODE_TUPLE_ASSIGNMENT:
		targets, values := childAt(node, 0), childAt(node, 1)
		if values != nil {
			for _, v := range values.Children {
				c.checkExpr(v)
			}
		}
		if targets != nil {
			for _, t := range targets.Children {
				if t.Type == NODE_IDENTIFIER {
					c.declare(t.Value)
				}
			}
		}

	case NODE_BLOCK:
		c.checkBlock(node)

	case NODE_IF_STATEMENT:
		for i, child := range node.Children {
			if i == 0 {
				c.checkExpr(child)
				continue
			}
			switch child.Type {
			case NODE_BLOCK:
				c.checkBlock(child)
			case NODE_IF_STATEMENT:
				c.checkStatement(child)
			default:
				c.checkExpr(child)
			}
		}

	case NODE_WHILE_LOOP:
		c.checkExpr(childAt(node, 0))
		c.checkBlock(childAt(node, 1))

	case NODE_FOR_RANGE_LOOP, NODE_FOR_COUNT_LOOP:
		// loop var: start to end
		c.pushScope()
		if v := childAt(node, 0); v != nil {
			c.declare(v.Value)
		}
		c.checkExpr(childAt(node, 1))
		c.checkExpr(childAt(node, 2))
		c.checkBlock(childAt(node, 3))
		c.popScope()

	case NODE_FOR_IN_ARRAY_LOOP:
		c.checkExpr(childAt(node, 1))
		c.pushScope()
		if v := childAt(node, 0); v != nil {
			c.declare(v.Value)
		}
		c.checkBlock(childAt(node, 2))
		c.popScope()

	case NODE_FOR_IN_DICT_LOOP:
		c.checkExpr(childAt(node, 2))
		c.pushScope()
		if k := childAt(node, 0); k != nil {
			c.declare(k.Value)
		}
		if v := childAt(node, 1); v != nil {
			c.declare(v.Value)
		}
		c.checkBlock(childAt(node, 3))
		c.popScope()

	case NODE_FOR_LOOP:
		for _, child := range node.Children {
			c.checkStatement(child)
		}

	case NODE_SWITCH_STATEMENT:
		c.checkExpr(childAt(node, 0))
		for _, caseNode := range node.Children[1:] {
			c.checkSwitchCase(caseNode)
		}

	case NODE_RETURN_STATEMENT:
		for _, child := range node.Children {
			c.checkExpr(child)
		}

	case NODE_ASSERT_STATEMENT, NODE_DEFER_STATEMENT:
		for _, child := range node.Children {
			c.checkStatement(child)
		}

	case NODE_GOTO_STATEMENT, NODE_LABEL_DECLARATION, NODE_HALT, NODE_NEXT,
		NODE_IMPORT_STATEMENT, NODE_PROGRAM_DECLARATION, NODE_ALIAS_DECLARATION,
		NODE_UNION_DECLARATION, NODE_STRUCT_DECLARATION, NODE_ENUM_DECLARATION:
		// Labels are not variables; type declarations carry no expressions.

	default:
		c.checkExpr(node)
	}
}

func (c *checker) checkSwitchCase(node *ASTNode) {
	if node == nil {
		return
	}
	switch node.Type {
	case NODE_SWITCH_CASE, NODE_SWITCH_CASE_LIST, NODE_SWITCH_CASE_RANGE:
		for i, child := range node.Children {
			if i == 0 {
				c.checkCaseValue(child)
				continue
			}
			if child.Type == NODE_BLOCK {
				c.checkBlock(child)
			} else {
				c.checkStatement(child)
			}
		}
	default:
		c.checkStatement(node)
	}
}

// checkCaseValue checks an `on` label. The default label is `_`, and a case
// value may be a range or a list, so only plain identifiers are treated as
// references.
func (c *checker) checkCaseValue(node *ASTNode) {
	if node == nil {
		return
	}
	switch node.Type {
	case NODE_IDENTIFIER:
		// `_` is the default label; a leading dot means "this member of the
		// enum being switched on", e.g. `on .UP:`.
		if node.Value == "_" || strings.HasPrefix(node.Value, ".") {
			return
		}
		c.checkNameUse(node, node.Value)
	case NODE_SWITCH_CASE_LIST, NODE_SWITCH_CASE_RANGE:
		for _, child := range node.Children {
			c.checkCaseValue(child)
		}
	default:
		c.checkExpr(node)
	}
}

// ---------------------------------------------------------------- expressions

func (c *checker) checkExpr(node *ASTNode) {
	if node == nil {
		return
	}
	switch node.Type {
	case NODE_IDENTIFIER:
		c.checkNameUse(node, node.Value)

	case NODE_CALL:
		c.checkCall(node)

	case NODE_METHOD_CALL:
		c.checkMethodCall(node)

	case NODE_BINARY_OP:
		if node.Value == "named_arg" {
			// name: value - the label is a parameter name, not a reference.
			c.checkExpr(childAt(node, 1))
			return
		}
		c.checkBinaryOp(node)
		c.checkExpr(childAt(node, 0))
		c.checkExpr(childAt(node, 1))

	case NODE_UNARY_OP:
		c.checkExpr(childAt(node, 0))

	case NODE_TERNARY:
		for _, child := range node.Children {
			c.checkExpr(child)
		}

	case NODE_ARRAY_LITERAL:
		for _, child := range node.Children {
			c.checkExpr(child)
		}

	case NODE_DICT_LITERAL:
		// Children alternate key, value. A bare-word key ({name: "Alice"}) is
		// an identifier node but names a field, not a variable.
		for i, child := range node.Children {
			if i%2 == 0 && child.Type == NODE_IDENTIFIER {
				continue
			}
			c.checkExpr(child)
		}

	case NODE_ARRAY_ACCESS, NODE_DICT_ACCESS:
		// Value is the collection's variable name; children are the indices.
		if node.Value != "" {
			c.checkNameUse(node, node.Value)
		}
		for _, child := range node.Children {
			c.checkExpr(child)
		}

	case NODE_MEMBER_ACCESS, NODE_OBJECT_ACCESS, NODE_TYPE_PROPERTY:
		// Value is the field name; only the object is a reference.
		c.checkExpr(childAt(node, 0))

	case NODE_STATIC_MEMBER_ACCESS:
		// StructType.#field - the object is a type name, not a variable.

	case NODE_OBJECT_LITERAL:
		// Value is the struct type; children are field: value pairs.
		for _, prop := range node.Children {
			c.checkExpr(childAt(prop, 0))
		}

	case NODE_LAMBDA:
		c.checkLambda(node)

	case NODE_F_STRING:
		c.checkFString(node)

	case NODE_NUMBER, NODE_STRING, NODE_RAW_STRING, NODE_CHAR, NODE_BOOLEAN,
		NODE_TYPE:
		// Literals and type names.

	default:
		for _, child := range node.Children {
			c.checkExpr(child)
		}
	}
}

func (c *checker) checkLambda(node *ASTNode) {
	paramCount := len(node.Children) - 1
	if n, err := strconv.Atoi(node.Value); err == nil && n >= 0 && n <= len(node.Children) {
		paramCount = n
	}
	if paramCount < 0 {
		paramCount = 0
	}

	c.pushScope()
	for i := 0; i < paramCount && i < len(node.Children); i++ {
		child := node.Children[i]
		if child.Type == NODE_IDENTIFIER {
			c.declare(child.Value)
		}
	}
	for i := paramCount; i < len(node.Children); i++ {
		c.checkExpr(node.Children[i])
	}
	c.popScope()
}

// checkFString checks the identifiers interpolated into an f-string. The
// interpolated text is stored as part of the string value, not as child nodes,
// so the leading identifier of each {..} group is checked by hand.
func (c *checker) checkFString(node *ASTNode) {
	for _, expr := range fstringExpressions(node.Value) {
		if name := leadingIdentifier(expr); name != "" {
			if !c.isDefined(name) {
				c.reportUndefined(node, name)
			}
		}
	}
}

func (c *checker) checkNameUse(node *ASTNode, name string) {
	if name == "" || name == "_" {
		return
	}
	if c.isDefined(name) {
		return
	}
	c.reportUndefined(node, name)
}

func (c *checker) reportUndefined(node *ASTNode, name string) {
	msg := fmt.Sprintf("Variable '%s' is not declared", name)
	if suggestion := c.suggest(name); suggestion != "" {
		msg += fmt.Sprintf("; did you mean '%s'?", suggestion)
	}
	c.errorAt(node, "%s", msg)
}

// ---------------------------------------------------------------- calls

func (c *checker) checkCall(node *ASTNode) {
	name := node.Value
	if !c.isCallable(name) {
		msg := fmt.Sprintf("Function '%s' is not defined", name)
		if suggestion := c.suggest(name); suggestion != "" {
			msg += fmt.Sprintf("; did you mean '%s'?", suggestion)
		}
		c.errorAt(node, "%s", msg)
		return
	}
	c.checkArguments(node, name, node.Children)
}

// checkArguments validates the argument list of a call against a user-defined
// function's parameters. C functions and builtins are not checked: their
// signatures are not fully known here.
func (c *checker) checkArguments(node *ASTNode, name string, args []*ASTNode) {
	for _, arg := range args {
		if arg.Type == NODE_BINARY_OP && arg.Value == "named_arg" {
			c.checkExpr(childAt(arg, 1))
		} else {
			c.checkExpr(arg)
		}
	}

	sig, ok := c.p.functions[name]
	if !ok || sig == nil || sig.FunctionNode == nil || len(sig.FunctionNode.Children) == 0 {
		return
	}
	params := sig.FunctionNode.Children[0]
	if params == nil {
		return
	}

	required := 0
	names := map[string]bool{}
	for _, param := range params.Children {
		names[param.Value] = true
		if param.DefaultValue == nil {
			required++
		}
	}
	max := len(params.Children)
	got := len(args)

	if got < required || got > max {
		expected := fmt.Sprintf("%d", max)
		if required != max {
			expected = fmt.Sprintf("%d to %d", required, max)
		}
		c.errorAt(node, "Function '%s' expects %s argument(s) but %d given", name, expected, got)
		return
	}

	// Named arguments must name a real parameter.
	for _, arg := range args {
		if arg.Type == NODE_BINARY_OP && arg.Value == "named_arg" {
			label := childAt(arg, 0)
			if label != nil && !names[label.Value] {
				c.errorAt(label, "Function '%s' has no parameter named '%s'", name, label.Value)
			}
		}
	}
}

func (c *checker) checkMethodCall(node *ASTNode) {
	if len(node.Children) < 2 {
		return
	}
	object := node.Children[0]

	// `namespace.func|..|` - the object is an imported C namespace, not a value.
	if object != nil && object.Type == NODE_IDENTIFIER {
		if _, isNamespace := c.p.cHeaders[object.Value]; isNamespace {
			c.checkNamespacedCall(node, object.Value)
			return
		}
	}

	c.checkExpr(object)
	args := node.Children[1]
	if args != nil {
		for _, arg := range args.Children {
			c.checkExpr(arg)
		}
	}
}

// checkNamespacedCall validates `namespace.func|..|` against the imported
// header's functions.
func (c *checker) checkNamespacedCall(node *ASTNode, namespace string) {
	header := c.p.cHeaders[namespace]
	if header == nil {
		return
	}
	method := node.Value
	if _, ok := header.Functions[method]; ok {
		return
	}
	for cName := range header.Functions {
		if PascalToSnake(cName) == method {
			return
		}
	}
	// Unknown to the header, but the header may be only partially parsed.
	c.errorAt(node, "'%s' has no function named '%s'", namespace, method)
}

// ---------------------------------------------------------------- operators

var arithmeticOps = map[string]bool{
	"+": true, "-": true, "*": true, "/": true, "%": true,
	"plus": true, "minus": true, "times": true, "div": true, "mod": true,
}

var orderingOps = map[string]bool{
	">": true, "<": true, ">=": true, "<=": true,
	"greater_than": true, "lesser_than": true,
	"greater_than_or_equal": true, "lesser_than_or_equal": true,
}

// checkBinaryOp rejects operand types that cannot be combined. Only cases that
// are certainly wrong are reported: an int plus a string currently compiles and
// prints the string's address, and string ordering compares pointers, so both
// produce a program that runs and silently does the wrong thing.
func (c *checker) checkBinaryOp(node *ASTNode) {
	op := node.Value
	if !arithmeticOps[op] && !orderingOps[op] {
		return
	}
	left, right := childAt(node, 0), childAt(node, 1)
	if left == nil || right == nil {
		return
	}
	lt := c.p.inferType(left)
	rt := c.p.inferType(right)

	if operandsUnusable(lt, rt) {
		c.errorAt(node, "Operator '%s' cannot be applied to %s and %s", op, lt, rt)
	}
}

// operandsUnusable reports whether either operand type makes an arithmetic or
// ordering operator meaningless. Anything not confidently known is left alone.
func operandsUnusable(lt, rt string) bool {
	if !operandIsKnown(lt) || !operandIsKnown(rt) {
		return false
	}
	return isStringType(lt) || isCollectionType(lt) || isStructType(lt) ||
		isStringType(rt) || isCollectionType(rt) || isStructType(rt)
}

// operandIsKnown reports whether a type is specific enough to judge.
func operandIsKnown(t string) bool {
	switch t {
	case "", "unknown", "any", "generic", "infer", "void", "null":
		return false
	}
	return true
}

func isStringType(t string) bool {
	switch t {
	case "string", "char *", "char*", "const char *", "const char*":
		return true
	}
	return false
}

func isCollectionType(t string) bool {
	return strings.HasPrefix(t, "array") || strings.HasPrefix(t, "dict")
}

func isStructType(t string) bool {
	return strings.HasPrefix(t, "struct:") || t == "object"
}

// ---------------------------------------------------------------- helpers

func childAt(node *ASTNode, i int) *ASTNode {
	if node == nil || i < 0 || i >= len(node.Children) {
		return nil
	}
	return node.Children[i]
}

// fstringExpressions returns the text inside each {..} group of an f-string.
func fstringExpressions(value string) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '{':
			if depth == 0 {
				start = i + 1
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
				if depth == 0 && start <= i {
					out = append(out, value[start:i])
				}
			}
		}
	}
	return out
}

// leadingIdentifier returns the first identifier in an expression, which is the
// only part of an interpolated expression that can be resolved without
// re-parsing it.
func leadingIdentifier(expr string) string {
	expr = strings.TrimSpace(expr)
	end := 0
	for end < len(expr) {
		ch := expr[end]
		if ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(end > 0 && ch >= '0' && ch <= '9') {
			end++
			continue
		}
		break
	}
	name := expr[:end]
	switch name {
	case "true", "false", "null", "and", "or", "not", "in", "is":
		return ""
	}
	return name
}
