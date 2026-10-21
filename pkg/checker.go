package ahoy

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
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

// StrictLint gates the -strict lint tier: cheap high-signal warnings for
// bug classes that segfault at runtime (element-type contradictions,
// literal division by zero). Default true; -strict=false opts out.
// Unconditional codegen self-checks (internal errors, never user noise)
// live separately in source/codegen.go and ignore this flag.
var StrictLint = true

type checker struct {
	p      *Parser
	errors []ParseError

	// scopes is the stack of open blocks; scopes[0] is the global scope.
	scopes []map[string]bool
	// scopeTypes mirrors scopes with the declared type of each name, so a
	// function-typed local (f: fn(int)->int) is known to be callable.
	scopeTypes []map[string]string
	// programNames holds names declared elsewhere in the same program, so a
	// multi-file program resolves names across its files.
	programNames map[string]bool
	// methods holds user-defined methods (this file plus programMethods from
	// other files), keyed by normalized receiver type, then method name.
	methods map[string]map[string]*MethodEntry

	funcName   string
	inFunction bool
	// funcParams holds the current function's parameter names (nil outside
	// functions). Push/store warnings skip parameters: the caller owns them.
	funcParams map[string]bool
	// freedNames holds locals explicitly freed with free|x| (not deferred).
	// Any later use warns (use-after-free); rebinding clears the entry.
	freedNames map[string]bool
	// inLoop counts enclosing loop bodies under check: a guard whose
	// else arm is a bare halt/next diverges only inside a loop (at top
	// level halt/next do not provably leave), so inLoop gates that
	// narrowing. Saved and reset per function like the sets above.
	inLoop int
	// nonNull holds names proved not-null by a guard or null test in the
	// straight-line flow (guard p is not null else return, or an if arm
	// entered under p is not null). Any assignment clears the entry, so
	// membership always means definitely-not-null here. Fresh per
	// function, like freedNames.
	nonNull map[string]bool
	// inDefer counts enclosing defer statements: free|x| inside one runs at
	// scope end, so later uses before then are fine and not recorded.
	inDefer int

	// suggestNames caches the static part of knownNames() for the whole check
	// run. suggest() is called once per undefined name, and rebuilding the
	// candidate list (which includes every C header symbol) on each call made
	// files with many undefined names quadratic. The parser maps, builtins and
	// C headers are fixed once parsing finishes, so they are snapshotted here;
	// live block scopes are appended per call (see suggest), which reproduces
	// exactly the candidate set knownNames() would compute at that moment.
	// suggestLens holds each candidate's rune length for the length prefilter,
	// and suggestMaxLen sizes the DP buffers suggest() reuses across candidates.
	suggestNames  []string
	suggestLens   []int
	suggestMaxLen int
}

// checkSemantics walks a parsed program and appends diagnostics to p.Errors.
// programNames may be nil; it carries the top-level names declared by the other
// files of the same program. programMethods carries the other files' methods
// the same way, so method calls validate across files.
func (p *Parser) checkSemantics(ast *ASTNode, programNames []string, programMethods map[string]map[string]*MethodEntry) {
	if ast == nil {
		return
	}
	c := &checker{
		p:            p,
		scopes:       []map[string]bool{{}},
		scopeTypes:   []map[string]string{{}},
		programNames: make(map[string]bool, len(programNames)),
		freedNames:   make(map[string]bool),
		nonNull:      make(map[string]bool),
	}
	for _, name := range programNames {
		c.programNames[name] = true
	}
	c.suggestNames = c.knownNames()
	c.suggestLens = make([]int, len(c.suggestNames))
	for i, name := range c.suggestNames {
		n := utf8.RuneCountInString(name)
		c.suggestLens[i] = n
		if n > c.suggestMaxLen {
			c.suggestMaxLen = n
		}
	}
	// Method tables: this file's methods plus the other files', so method
	// calls validate across the program the way free functions do via
	// programNames. Own-file entries win. Cross-file signatures are registered
	// under their mangled names so the ordinary argument checking applies to
	// method calls too.
	c.methods = map[string]map[string]*MethodEntry{}
	for recv, ms := range programMethods {
		cp := make(map[string]*MethodEntry, len(ms))
		for name, e := range ms {
			cp[name] = e
		}
		c.methods[recv] = cp
	}
	for recv, ms := range CollectMethods(ast) {
		if _, ok := c.methods[recv]; !ok {
			c.methods[recv] = map[string]*MethodEntry{}
		}
		for name, e := range ms {
			c.methods[recv][name] = e
		}
	}
	for recv, ms := range c.methods {
		for name, e := range ms {
			if e == nil || e.Sig == nil {
				continue
			}
			mangled := MangleMethodName(recv, name)
			if _, exists := c.p.functions[mangled]; !exists {
				c.p.functions[mangled] = e.Sig
			}
		}
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
		case NODE_WHEN_STATEMENT:
			// A `when NAME then` body declares real names -- the raylib backend
			// lives in one -- and the rest of the program has to see them, or
			// every call into the backend reads as undefined.
			for _, body := range child.Children {
				out = append(out, DeclaredNames(body)...)
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

// warningAt reports an advisory diagnostic at a node's position. Warnings are
// printed but never fail a build.
func (c *checker) warningAt(node *ASTNode, format string, args ...any) {
	if node == nil {
		return
	}
	line, column := node.Line, node.Column
	if line <= 0 {
		if pos := firstPositioned(node); pos != nil {
			line, column = pos.Line, pos.Column
		}
	}
	c.errors = append(c.errors, ParseError{
		Message:  fmt.Sprintf(format, args...),
		Line:     line,
		Column:   column,
		Severity: "warning",
	})
}

// warnPartialCallResult reports a function that returns several values when
// they are not all captured. A single-name declaration or assignment captures
// one; the tuple form captures one value per target.
func (c *checker) warnPartialCallResult(node *ASTNode, value *ASTNode, captured int) {
	if value == nil || value.Type != NODE_CALL {
		return
	}
	sig := c.p.functions[value.Value]
	if sig == nil || sig.IsInfer || len(sig.ReturnTypes) <= 1 {
		return
	}
	returned := len(sig.ReturnTypes)
	if captured == returned {
		return
	}
	if captured < returned {
		c.warningAt(node, "Function '%s' returns %d values but only %d is captured; assign them to %d variables (a, b : %s|...|)",
			value.Value, returned, captured, returned, value.Value)
	} else {
		c.warningAt(node, "Function '%s' returns %d values but %d targets are given",
			value.Value, returned, captured)
	}
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

func (c *checker) pushScope() {
	c.scopes = append(c.scopes, map[string]bool{})
	c.scopeTypes = append(c.scopeTypes, map[string]string{})
}

func (c *checker) popScope() {
	if len(c.scopes) > 1 {
		c.scopes = c.scopes[:len(c.scopes)-1]
		c.scopeTypes = c.scopeTypes[:len(c.scopeTypes)-1]
	}
}

func (c *checker) declare(name string) {
	if name == "" || name == "_" {
		return
	}
	c.scopes[len(c.scopes)-1][name] = true
}

// declareTyped is declare plus the declared type, when there is one.
func (c *checker) declareTyped(name, typeName string) {
	c.declare(name)
	if name == "" || name == "_" || typeName == "" {
		return
	}
	c.scopeTypes[len(c.scopeTypes)-1][name] = typeName
}

// lookupType returns the innermost declared type of a name, if any.
func (c *checker) lookupType(name string) string {
	for i := len(c.scopeTypes) - 1; i >= 0; i-- {
		if t, ok := c.scopeTypes[i][name]; ok && t != "" {
			return t
		}
	}
	return ""
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
	if _, ok := p.distinctTypes[name]; ok {
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
	// A variable holding a function value is called through it
	if c.isFunctionTypedVariable(name) {
		return true
	}
	// A variable holding a C callback typedef is called through it too
	if c.isCallbackTypedefVariable(name) {
		return true
	}
	// A distinct type name converts with one argument: Name(value).
	if _, ok := c.p.distinctTypes[name]; ok {
		return true
	}
	// A struct allocator: ahoy_alloc_NodeVis()
	if structName := AllocStructName(name); structName != "" {
		if _, ok := c.p.structs[structName]; ok {
			return true
		}
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

// isFunctionTypedVariable reports whether a local or global variable holds a
// function value, e.g. `f: fn(int)->int = &double_it`.
func (c *checker) isFunctionTypedVariable(name string) bool {
	if IsFunctionType(c.lookupType(name)) {
		return true
	}
	if t, ok := c.p.variableTypes[name]; ok && IsFunctionType(t) {
		return true
	}
	return false
}

// isCallbackTypedefVariable reports whether a variable holds a C callback
// typedef value (e.g. LogCallback), which is called like a function.
func (c *checker) isCallbackTypedefVariable(name string) bool {
	t := c.lookupType(name)
	if t == "" {
		t = c.p.variableTypes[name]
	}
	if t == "" {
		return false
	}
	for _, h := range c.cHeaders() {
		if h != nil && h.FuncPtrTypedefs[t] != nil {
			return true
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
// plausible typo. Candidates come from the precomputed suggestNames list, and
// each comparison stops early once it cannot beat the current best, so a file
// with hundreds of undefined names no longer costs hundreds of full scans.
// Control keywords that read as variables when misremembered from other
// languages: break is halt (exit loop), continue is next (skip iteration).
var controlKeywordHints = map[string]string{
	"break":    "halt (exit loop)",
	"continue": "next (skip iteration)",
}

func (c *checker) suggest(name string) string {
	// Only suggest genuinely similar names, and never for very short ones.
	limit := len(name) / 3
	if limit < 1 {
		limit = 1
	}
	best := ""
	bestDistance := limit + 1
	nameLen := utf8.RuneCountInString(name)
	nameASCII := isASCII(name)
	// Row buffers are reused across every candidate; they only need to cover
	// the shorter side of a comparison, which never exceeds the longest static
	// name (a longer block-scope name grows them via the buf helpers).
	prev := make([]int, c.suggestMaxLen+1)
	curr := make([]int, c.suggestMaxLen+1)
	consider := func(candidate string, candLen int) {
		if candidate == name {
			return
		}
		maxDist := bestDistance - 1
		// A length gap alone rules the candidate out (edit distance is at
		// least the length difference).
		if d := nameLen - candLen; d < 0 {
			if -d > maxDist {
				return
			}
		} else if d > maxDist {
			return
		}
		var d int
		if nameASCII && isASCII(candidate) {
			d = levenshteinBytesBuf(name, candidate, maxDist, prev, curr)
		} else {
			d = levenshteinBounded(name, candidate, maxDist)
		}
		if d < bestDistance {
			bestDistance = d
			best = candidate
		}
	}
	for i, candidate := range c.suggestNames {
		consider(candidate, c.suggestLens[i])
		if bestDistance == 0 {
			break
		}
	}
	// Block scopes change as the walk proceeds, so they are read live: this
	// keeps the candidate set identical to a fresh knownNames() call.
	if bestDistance != 0 {
	outer:
		for i := range c.scopes {
			for candidate := range c.scopes[i] {
				consider(candidate, utf8.RuneCountInString(candidate))
				if bestDistance == 0 {
					break outer
				}
			}
		}
	}
	if best != "" && bestDistance <= limit {
		return best
	}
	return ""
}

// levenshteinBounded reports the edit distance between a and b, but stops
// early and returns maxDist+1 once the distance must exceed maxDist. Callers
// that only care about beating a current best never pay for a full matrix.
func levenshteinBounded(a, b string, maxDist int) int {
	if a == b {
		return 0
	}
	if maxDist < 0 {
		return 0
	}
	// A length gap alone can rule the candidate out.
	if d := utf8.RuneCountInString(a) - utf8.RuneCountInString(b); d < 0 {
		if -d > maxDist {
			return maxDist + 1
		}
	} else if d > maxDist {
		return maxDist + 1
	}
	// Fast byte path for the usual all-ASCII identifiers.
	if isASCII(a) && isASCII(b) {
		return levenshteinBoundedBytes(a, b, maxDist)
	}
	return levenshteinBoundedRunes([]rune(a), []rune(b), maxDist)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func levenshteinBoundedBytes(a, b string, maxDist int) int {
	return levenshteinBytesBuf(a, b, maxDist, nil, nil)
}

// levenshteinBytesBuf is levenshteinBoundedBytes with caller-provided row
// buffers (sized at least the shorter string + 1); nil buffers are allocated.
func levenshteinBytesBuf(a, b string, maxDist int, prev, curr []int) int {
	if len(a) == 0 {
		if len(b) > maxDist {
			return maxDist + 1
		}
		return len(b)
	}
	if len(b) == 0 {
		if len(a) > maxDist {
			return maxDist + 1
		}
		return len(a)
	}
	// Keep the shorter string on the row axis for smaller buffers.
	if len(b) > len(a) {
		a, b = b, a
	}
	if prev == nil || len(prev) < len(b)+1 {
		prev = make([]int, len(b)+1)
	} else {
		prev = prev[:len(b)+1]
	}
	if curr == nil || len(curr) < len(b)+1 {
		curr = make([]int, len(b)+1)
	} else {
		curr = curr[:len(b)+1]
	}
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		rowMin := curr[0]
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			v := min(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
			curr[j] = v
			if v < rowMin {
				rowMin = v
			}
		}
		if rowMin > maxDist {
			return maxDist + 1
		}
		prev, curr = curr, prev
	}
	if prev[len(b)] > maxDist {
		return maxDist + 1
	}
	return prev[len(b)]
}

func levenshteinBoundedRunes(ar, br []rune, maxDist int) int {
	if len(ar) == 0 {
		if len(br) > maxDist {
			return maxDist + 1
		}
		return len(br)
	}
	if len(br) == 0 {
		if len(ar) > maxDist {
			return maxDist + 1
		}
		return len(ar)
	}
	if len(br) > len(ar) {
		ar, br = br, ar
	}
	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		curr[0] = i
		rowMin := curr[0]
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			v := min(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
			curr[j] = v
			if v < rowMin {
				rowMin = v
			}
		}
		if rowMin > maxDist {
			return maxDist + 1
		}
		prev, curr = curr, prev
	}
	if prev[len(br)] > maxDist {
		return maxDist + 1
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
	if node.Method != nil {
		c.checkMethodDeclaration(node)
	}
	params, body := node.Children[0], node.Children[1]

	savedName := c.funcName
	savedInFunc := c.inFunction
	savedParams := c.funcParams
	savedFreed := c.freedNames
	savedLoop := c.inLoop
	savedNonNull := c.nonNull
	c.funcName = node.Value
	c.inFunction = true
	c.funcParams = map[string]bool{}
	c.freedNames = map[string]bool{}
	c.inLoop = 0
	c.nonNull = map[string]bool{}

	c.pushScope()
	if params != nil {
		for _, param := range params.Children {
			c.declareTyped(param.Value, param.DataType)
			if strings.HasPrefix(param.DataType, "buffer[") {
				c.checkBufferSpelling(param.DataType, param)
			}
			if param.Value != "" {
				c.funcParams[param.Value] = true
			}
		}
	}
	c.checkBlock(body)
	c.popScope()

	// A function that promises a value must keep its promise on every path:
	// falling off the end returns whatever garbage the register holds (this
	// used to compile silently). Void functions may always fall through.
	if c.functionNeedsReturn(node.Value, body) && !blockAlwaysReturns(body) {
		c.errorAt(node, "Function '%s' may fall off the end without returning a value; add a return to every path (an `else` or a `_` default case)", node.Value)
	}

	c.funcName = savedName
	c.inFunction = savedInFunc
	c.funcParams = savedParams
	c.freedNames = savedFreed
	c.inLoop = savedLoop
	c.nonNull = savedNonNull
}

// functionNeedsReturn reports whether a function body must return a value on
// every path: explicit non-void return types always do. (`infer` is rejected
// as a return type at parse time, so that branch only ever sees
// already-broken code; it is handled defensively.)
func (c *checker) functionNeedsReturn(name string, body *ASTNode) bool {
	sig, ok := c.p.functions[name]
	if !ok || len(sig.ReturnTypes) == 0 {
		return false
	}
	switch rt := sig.ReturnTypes[0]; rt {
	case "", "void":
		return false
	case "infer":
		return bodyHasValueReturn(body)
	default:
		return true
	}
}

// bodyHasValueReturn reports whether any `return <value>` occurs, without
// crossing into nested functions or lambdas (their returns are not ours).
func bodyHasValueReturn(node *ASTNode) bool {
	if node == nil {
		return false
	}
	if node.Type == NODE_FUNCTION || node.Type == NODE_LAMBDA {
		return false
	}
	if node.Type == NODE_RETURN_STATEMENT {
		return len(node.Children) > 0
	}
	for _, child := range node.Children {
		if bodyHasValueReturn(child) {
			return true
		}
	}
	return false
}

// checkNullIf checks an if statement, threading null-state through its
// arms: an arm entered under `X is not null` knows X inside (guarded
// derefs stay clean); a diverging single arm entered under `X is null`
// proves X for what follows when there is no else to sneak around
// through (the guard shape, and the bare if-no-else shape). Every other
// arm checks exactly as before, with the set restored afterwards, so a
// conditional proof never leaks out of its arm.
func (c *checker) checkNullIf(node *ASTNode) {
	if len(node.Children) == 0 {
		return
	}
	n := len(node.Children)
	hasElse := (n-1)%2 == 0
	bodyEnd := n
	var elseBody *ASTNode
	if hasElse && n > 1 {
		elseBody = node.Children[n-1]
		bodyEnd = n - 1
	}
	// The if condition always checks as an expression.
	c.checkExpr(node.Children[0])
	// (condition, body) arms: bodies at odd indices, each governed by
	// the condition before it. A nested if child (an elif chain the
	// parser left nested) recurses and handles itself. Every arm body
	// checks under a snapshot: assignments inside a conditional arm
	// must not leak proofs out of it.
	for i := 1; i < bodyEnd; i += 2 {
		body := node.Children[i]
		if body.Type == NODE_IF_STATEMENT {
			c.checkStatement(body)
			continue
		}
		if body.Type != NODE_BLOCK {
			c.checkStatement(body)
			continue
		}
		saved := c.snapshotNull()
		if root, isNull, ok := nullImplies(node.Children[i-1], false); ok && !isNull {
			c.nonNull[root] = true
		}
		c.checkBlock(body)
		c.restoreNull(saved)
	}
	// Elif conditions between the arms check as plain expressions.
	for i := 2; i < bodyEnd; i += 2 {
		c.checkExpr(node.Children[i])
	}
	if elseBody != nil {
		if elseBody.Type != NODE_BLOCK {
			c.checkStatement(elseBody)
			return
		}
		// No elifs: the else runs with the if condition false.
		// Snapshot unconditionally: an else arm is conditional.
		saved := c.snapshotNull()
		if bodyEnd == 2 {
			if root, isNull, ok := nullImplies(node.Children[0], true); ok && !isNull {
				c.nonNull[root] = true
			}
		}
		c.checkBlock(elseBody)
		c.restoreNull(saved)
		return
	}
	// No else, single then-arm, diverging body: the fall-through proves
	// the condition false (the guard shape and its bare-if twin).
	// Divergence is `return` anywhere, or a bare halt/next inside a
	// loop (at top level neither provably leaves).
	if n == 2 && node.Children[1].Type == NODE_BLOCK &&
		c.guardDiverges(node.Children[1]) {
		if root, isNull, ok := nullImplies(node.Children[0], true); ok && !isNull {
			c.nonNull[root] = true
		}
	}
}

// blockAlwaysReturns reports whether execution of a block can never fall off
// its end: some statement always returns first. Anything unrecognized counts
// as falling through, so this never claims a return that is not there -
// at worst it misses an exotic hole, it never invents one.
func blockAlwaysReturns(node *ASTNode) bool {
	if node == nil {
		return false
	}
	if node.Type != NODE_BLOCK {
		return stmtAlwaysReturns(node)
	}
	for _, child := range node.Children {
		if stmtAlwaysReturns(child) {
			return true
		}
	}
	return false
}

// stmtAlwaysReturns reports whether a statement never falls through to the
// next one. Loops may not execute, `halt` only leaves its loop, and calls or
// assignments always continue - only `return` diverges, plus `if` and
// `switch` whose every branch (including a required `else`/`_`) diverges.
func stmtAlwaysReturns(node *ASTNode) bool {
	if node == nil {
		return false
	}
	switch node.Type {
	case NODE_RETURN_STATEMENT:
		return true
	case NODE_IF_STATEMENT:
		return ifAlwaysReturns(node)
	case NODE_WHEN_STATEMENT:
		return whenAlwaysReturns(node)
	case NODE_SWITCH_STATEMENT:
		return switchStmtAlwaysReturns(node)
	default:
		return false
	}
}

// whenAlwaysReturns: `when NAME then A $ else B $` diverges exactly when
// both bodies diverge. The C preprocessor compiles precisely one side, so
// a lone `when` without `else` always falls through (and returns false here).
// Sound because the checker verifies both bodies unconditionally.
func whenAlwaysReturns(node *ASTNode) bool {
	if len(node.Children) < 2 {
		return false
	}
	return blockAlwaysReturns(node.Children[0]) && blockAlwaysReturns(node.Children[1])
}

// children[1] the then-body, then condition/body pairs for each
// elseif/anif, and a lone trailing else-body when the remainder is even.
// Every branch must diverge, and an `else` must exist - otherwise the
// condition can simply be false.
func ifAlwaysReturns(node *ASTNode) bool {
	if len(node.Children) < 2 {
		return false
	}
	rest := node.Children[1:]
	hasElse := len(rest)%2 == 0
	end := len(rest)
	if hasElse {
		end--
	}
	for i := 0; i < end; i += 2 {
		if !blockAlwaysReturns(rest[i]) {
			return false
		}
	}
	if !hasElse {
		return false
	}
	return blockAlwaysReturns(rest[len(rest)-1])
}

// switchStmtAlwaysReturns requires a `_` default (without one a value can
// match nothing) and a diverging body in every case.
func switchStmtAlwaysReturns(node *ASTNode) bool {
	if len(node.Children) < 1 || !switchHasDefault(node) {
		return false
	}
	for _, caseNode := range node.Children[1:] {
		if caseNode.Type != NODE_SWITCH_CASE || len(caseNode.Children) < 2 {
			return false
		}
		if !blockAlwaysReturns(caseNode.Children[1]) {
			return false
		}
	}
	return true
}

// switchHasDefault reports whether a switch has a `_` default case.
func switchHasDefault(node *ASTNode) bool {
	for _, caseNode := range node.Children[1:] {
		if caseNode.Type == NODE_SWITCH_CASE && len(caseNode.Children) > 0 {
			if v := caseNode.Children[0]; v.Type == NODE_IDENTIFIER && v.Value == "_" {
				return true
			}
		}
	}
	return false
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
		c.warnPartialCallResult(node, childAt(node, 0), 1)
		if node.DataType != "" && IsFunctionType(node.DataType) {
			if refName, ok := fnRefName(childAt(node, 0)); ok {
				c.checkFnRef(childAt(node, 0), refName, node.DataType, "variable '"+node.Value+"'")
			}
		}
		declType := node.DataType
		if declType == "" {
			declType = c.inferVarType(childAt(node, 0))
		}
		if declType == "char" {
			c.checkCharSlot(childAt(node, 0), "variable '"+node.Value+"'")
		}
		c.declareTyped(node.Value, declType)
		// Buffer lanes: validate the spelling, then agree the value's
		// lane with the declaration's. Constructors carry their lane
		// (buffer|n| is f32, buffer_int|n| int), so a bare buffer|n|
		// into a buffer[int] declaration is an error here — not a
		// silent float store downstream.
		if strings.HasPrefix(declType, "buffer[") {
			if node.DataType != "" {
				c.checkBufferSpelling(node.DataType, node)
			}
			if rhs := childAt(node, 0); rhs != nil {
				if got := c.argActualType(rhs); got != "" {
					c.checkBufferLane(declType, got, node, "variable '"+node.Value+"'")
				}
			}
		}
		// Null tracking: a fresh declaration proves nothing yet (an
		// uninitialized pointer holds NULL), except taking an address,
		// which is never null.
		c.trackNullStore(node.Value, childAt(node, 0))
		// In-place array methods (push/sort/reverse/shuffle/fill/remove/insert/
		// clear plus the append/erase aliases) return the SAME array they
		// mutated. Binding that to a second name makes two scope-owned owners
		// of one allocation, which scope exit frees twice (SIGSEGV). Use the
		// call as a statement, or rebind the same name: xs.push|4|.
		if rhs := childAt(node, 0); rhs != nil && rhs.Type == NODE_METHOD_CALL && len(rhs.Children) > 0 {
			if recv := rhs.Children[0]; recv != nil && recv.Type == NODE_IDENTIFIER && recv.Value != node.Value {
				if recvT := c.lookupType(recv.Value); recvT == "array" || strings.HasPrefix(recvT, "array[") {
					switch rhs.Value {
					case "push", "append", "sort", "reverse", "shuffle", "fill",
						"remove", "erase", "insert", "clear":
						c.warningAt(node, "In-place '%s' returns the same array: '%s' and '%s' now own one allocation (double-free at scope exit); call it as a statement or rebind '%s' itself", rhs.Value, node.Value, recv.Value, recv.Value)
					}
				}
			}
		}
	// -strict: an explicit annotation contradicting an array read
		// (n: int = strs[i]) is shaped exactly like the lines[i]-as-int
		// segfault. Flowing inference stays silent; only an explicit
		// claim that disagrees with the array's element type warns.
		if StrictLint && node.DataType != "" {
			if rhs := childAt(node, 0); rhs != nil && rhs.Type == NODE_ARRAY_ACCESS && rhs.Value != "" {
				if arrT := c.lookupType(rhs.Value); arrT != "" {
					if u, ok := strictArrayElem(arrT); ok && strictElemMismatch(node.DataType, u) {
						c.warningAt(node, "Assigning array[%s] element to '%s' declared %s; this reads garbage at runtime", u, node.Value, node.DataType)
					}
				}
			}
		}
		// Rebinding heals a freed name: the new value owns fresh storage.
		delete(c.freedNames, node.Value)

	case NODE_ASSIGNMENT:
		// Slices are read-only views: assign to s[i] or rebuild the string.
		if target := childAt(node, 0); target != nil && target.Type == NODE_STRING_SLICE {
			c.errorAt(target, "cannot assign to a string slice s[lo:hi]")
		}
		// Buffer lanes: a whole-container field store must agree with the
		// field's lane (x.f = ...). Plain rebinds (x = ...) are already
		// covered by the reassign type check; element stores (x[i] = v)
		// convert at the lane's width instead.
		if target := childAt(node, 0); target != nil && target.Type == NODE_MEMBER_ACCESS {
			if val := childAt(node, 1); val != nil {
				if lane, ok := c.bufferFieldLane(target); ok {
					c.checkBufferLane("buffer["+lane+"]", c.argActualType(val), node, "field '"+target.Value+"'")
				}
			}
		}
		// Null tracking: a store invalidates what the flow knew about
		// the name. Taking an address is never null; anything else is
		// unknown again (element stores arr[i]/d<k> have no tracked
		// name and are skipped).
		if len(node.Children) == 1 && node.Value != "" {
			c.trackNullStore(node.Value, childAt(node, 0))
		} else if target := childAt(node, 0); target != nil && target.Type == NODE_MEMBER_ACCESS {
			c.trackNullStore(nullRoot(target), childAt(node, 1))
		}
		// The parser already rejects assignment to an undeclared variable.
		c.checkExpr(childAt(node, 0))
		c.warnPartialCallResult(node, childAt(node, 0), 1)
		if tgt := childAt(node, 0); tgt != nil && tgt.Type == NODE_IDENTIFIER {
			if c.lookupType(tgt.Value) == "char" {
				c.checkCharSlot(childAt(node, 1), "variable '"+tgt.Value+"'")
			}
		}
		// Same aliasing rule as declarations: rebinding an in-place array
		// call to a different name double-frees at scope exit.
		if tgt, val := childAt(node, 0), childAt(node, 1); tgt != nil && val != nil &&
			tgt.Type == NODE_IDENTIFIER && val.Type == NODE_METHOD_CALL && len(val.Children) > 0 {
			if recv := val.Children[0]; recv != nil && recv.Type == NODE_IDENTIFIER && recv.Value != tgt.Value {
				if recvT := c.lookupType(recv.Value); recvT == "array" || strings.HasPrefix(recvT, "array[") {
					switch val.Value {
					case "push", "append", "sort", "reverse", "shuffle", "fill",
						"remove", "erase", "insert", "clear":
						c.warningAt(node, "In-place '%s' returns the same array: '%s' and '%s' now own one allocation (double-free at scope exit); call it as a statement or rebind '%s' itself", val.Value, tgt.Value, recv.Value, recv.Value)
					}
				}
			}
		}
		// Heap stores into struct fields or dict slots may leak the previous
		// contents when autofree can't prove the store safe - but that is the
		// safe direction (vs dangling), and high-level code is not nagged
		// about ownership it cannot observe. Leak hunting is -show_leaks'
		// job (ASan), not the default build's.
		if target := childAt(node, 0); target == nil || (target.Type != NODE_ARRAY_ACCESS && target.Type != NODE_DICT_ACCESS) {
			// Plain rebind heals a freed name (element stores into a dead
			// array do not). Uses in the new value already warned above.
			delete(c.freedNames, node.Value)
		}

	case NODE_TUPLE_ASSIGNMENT:
		targets, values := childAt(node, 0), childAt(node, 1)
		if values != nil {
			for _, v := range values.Children {
				c.checkExpr(v)
			}
			if len(values.Children) == 1 && targets != nil {
				c.warnPartialCallResult(node, values.Children[0], len(targets.Children))
			}
		}
		if targets != nil {
			for _, t := range targets.Children {
				if t.Type == NODE_IDENTIFIER {
					c.declare(t.Value)
				}
			}
		}

	case NODE_FUNCTION:
		// A function declared inside a block -- the backend implementation in a
		// `when NAME then` body, for instance -- needs the same parameter and
		// body checking as a top-level one. Falling through to checkExpr only
		// visited its identifiers, so its own parameters read as undeclared.
		c.checkFunction(node)

	case NODE_CONSTANT_DECLARATION:
		c.checkExpr(childAt(node, 0))
		c.declare(node.Value)

	case NODE_BLOCK:
		c.checkBlock(node)

	case NODE_IF_STATEMENT:
		c.checkNullIf(node)

	case NODE_WHILE_LOOP:
		// `loop i:start while/until condition` and `loop i while/until condition` carry the
		// loop variable as the first child; it is in scope in the condition and
		// the body, so it has to be declared before either is checked.
		if len(node.Children) >= 3 && node.Children[0].Type == NODE_IDENTIFIER {
			c.pushScope()
			c.declare(node.Children[0].Value)
			for i := 1; i < len(node.Children)-1; i++ {
				c.checkExpr(childAt(node, i))
			}
			c.checkNullBlock(childAt(node, len(node.Children)-1))
			c.popScope()
			return
		}
		c.checkExpr(childAt(node, 0))
		c.checkNullBlock(childAt(node, 1))

	case NODE_FOR_RANGE_LOOP, NODE_FOR_COUNT_LOOP:
		// loop var: start to end
		c.pushScope()
		if v := childAt(node, 0); v != nil {
			c.declare(v.Value)
		}
		c.checkExpr(childAt(node, 1))
		c.checkExpr(childAt(node, 2))
		c.checkNullBlock(childAt(node, 3))
		c.popScope()

	case NODE_FOR_IN_ARRAY_LOOP:
		c.checkExpr(childAt(node, 1))
		c.pushScope()
		if v := childAt(node, 0); v != nil {
			c.declare(v.Value)
		}
		c.checkNullBlock(childAt(node, 2))
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
		c.checkNullBlock(childAt(node, 3))
		c.popScope()

	case NODE_FOR_LOOP:
		savedForNull := c.snapshotNull()
		c.inLoop++
		for _, child := range node.Children {
			c.checkStatement(child)
		}
		c.inLoop--
		c.restoreNull(savedForNull)

	case NODE_WHEN_STATEMENT:
		// `when NAME then ...` compiles only under -D NAME, but the body is
		// always parsed. It was skipped here, so a declaration inside the block
		// was invisible to the very next statement in the same block.
		savedWhenNull := c.snapshotNull()
		for _, child := range node.Children {
			if child.Type == NODE_BLOCK {
				c.checkBlock(child)
			} else {
				c.checkStatement(child)
			}
		}
		c.restoreNull(savedWhenNull)

	case NODE_SWITCH_STATEMENT:
		c.checkExpr(childAt(node, 0))
		for _, caseNode := range node.Children[1:] {
			c.checkSwitchCase(caseNode)
		}
		c.checkSwitchDuplicates(node)

	case NODE_RETURN_STATEMENT:
		for _, child := range node.Children {
			c.checkExpr(child)
		}
		// Returning an f-string hands out the shared format buffer: the next
		// call overwrites what the caller holds. Return a concatenation or
		// bind-then-return instead (bound f-strings own their copy).
		for _, child := range node.Children {
			if child != nil && child.Type == NODE_F_STRING {
				c.warningAt(child, "Returning an f-string shares the format buffer; bind it to a variable first so the caller owns its value")
				break
			}
		}
		// A multi-character literal returned from a char function is
		// garbage, not a conversion; single characters coerce.
		if len(node.Children) == 1 && c.inFunction {
			if sig, ok := c.p.functions[c.funcName]; ok && len(sig.ReturnTypes) == 1 && sig.ReturnTypes[0] == "char" {
				c.checkCharSlot(node.Children[0], "return of '"+c.funcName+"'")
			}
		}
		// A bare `return` in a function that promises a value dies later in
		// the C compiler; report it here instead. Void and untyped functions
		// may always return bare.
		if len(node.Children) == 0 && c.inFunction {
			if sig, ok := c.p.functions[c.funcName]; ok && len(sig.ReturnTypes) > 0 {
				rt := sig.ReturnTypes[0]
				if rt != "" && rt != "void" && rt != "infer" {
					c.errorAt(node, "Function '%s' returns %s but this return has no value", c.funcName, rt)
				}
			}
		}

	case NODE_ASSERT_STATEMENT:
		for _, child := range node.Children {
			c.checkStatement(child)
		}

	case NODE_DEFER_STATEMENT:
		// free|x| inside a defer runs at scope end: later uses before then
		// are fine, so deferred frees are not recorded (see checkCall).
		c.inDefer++
		for _, child := range node.Children {
			c.checkStatement(child)
		}
		c.inDefer--

	case NODE_HALT, NODE_NEXT,
		NODE_IMPORT_STATEMENT, NODE_PROGRAM_DECLARATION, NODE_ALIAS_DECLARATION,
		NODE_UNION_DECLARATION, NODE_STRUCT_DECLARATION, NODE_ENUM_DECLARATION:
		// Type declarations carry no expressions.

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
			// Case arms are conditional: snapshot so proofs established
			// inside one do not leak into the code after the switch.
			saved := c.snapshotNull()
			if child.Type == NODE_BLOCK {
				c.checkBlock(child)
			} else {
				c.checkStatement(child)
			}
			c.restoreNull(saved)
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

// checkSwitchDuplicates warns on `on` labels that can never match because
// an earlier case already matches them. C switches reject duplicate labels,
// but string/range switches compile to if/else chains where a second match
// is silently dead ("first match wins"), and int switches otherwise fail
// much later in the C compiler with no source context. This is a warning,
// not an error: the code is valid, just unlikely to mean what an if-chain
// conversion would (an if-chain runs every matching body, a switch runs
// only the first).
func (c *checker) checkSwitchDuplicates(node *ASTNode) {
	if node == nil || len(node.Children) < 2 {
		return
	}
	seen := map[string]bool{}
	var intervals []caseNumInterval
	for _, caseNode := range node.Children[1:] {
		if caseNode == nil || caseNode.Type != NODE_SWITCH_CASE || len(caseNode.Children) < 2 {
			continue
		}
		val := caseNode.Children[0]
		// Exact duplicates (lists expand to one key per element).
		warned := false
		for _, key := range c.caseValueKeys(val) {
			if seen[key] && !warned {
				c.warningAt(caseNode, "Duplicate switch case value %s; only the first matching 'on' runs", caseKeyDisplay(key))
				warned = true
			}
		}
		for _, key := range c.caseValueKeys(val) {
			seen[key] = true
		}
		// Numeric overlap: singles are points, ranges are intervals. Pairs
		// already reported as exact duplicates are skipped, as is any
		// further overlap report for a case that already has an exact-
		// duplicate warning (the duplicate is the actionable signal).
		cur := numericIntervals(val, caseNode)
		if !warned {
			for _, n := range cur {
				for _, prev := range intervals {
					if n.lo <= prev.hi && prev.lo <= n.hi &&
						!(n.lo == prev.lo && n.hi == prev.hi) {
						c.warningAt(caseNode, "Switch case %s overlaps earlier case %s; only the first matching 'on' runs", n.label, prev.label)
						break
					}
				}
			}
		}
		intervals = append(intervals, cur...)
	}
}

// caseValueKeys returns one canonical key per value an `on` label matches:
// singles yield one key, lists yield one per element, ranges yield a single
// lo..hi key. Unkeyable shapes (member chains, expressions) yield nothing.
func (c *checker) caseValueKeys(val *ASTNode) []string {
	if val == nil {
		return nil
	}
	switch val.Type {
	case NODE_SWITCH_CASE_LIST:
		var out []string
		for _, child := range val.Children {
			out = append(out, c.caseValueKeys(child)...)
		}
		return out
	case NODE_SWITCH_CASE_RANGE:
		if len(val.Children) == 2 {
			if lo, ok := c.caseAtomKey(val.Children[0]); ok {
				if hi, ok := c.caseAtomKey(val.Children[1]); ok {
					return []string{"range:" + lo + ".." + hi}
				}
			}
		}
		return nil
	default:
		if key, ok := c.caseAtomKey(val); ok {
			return []string{key}
		}
		return nil
	}
}

// caseAtomKey canonicalizes a single case value. Enum spellings that resolve
// to the same member (`.UP` and `Direction.UP`) share a key; everything
// else keys by its written form.
func (c *checker) caseAtomKey(val *ASTNode) (string, bool) {
	switch val.Type {
	case NODE_NUMBER:
		return "num:" + val.Value, true
	case NODE_STRING:
		return "str:" + val.Value, true
	case NODE_CHAR:
		return "char:" + val.Value, true
	case NODE_IDENTIFIER:
		if val.Value == "_" {
			return "default:", true
		}
		if strings.HasPrefix(val.Value, ".") {
			if enumName := c.enumForMember(val.Value[1:]); enumName != "" {
				return "enum:" + enumName + "." + val.Value[1:], true
			}
			return "id:" + val.Value, true
		}
		return "id:" + val.Value, true
	case NODE_MEMBER_ACCESS:
		if len(val.Children) > 0 && val.Children[0].Type == NODE_IDENTIFIER {
			return "enum:" + val.Children[0].Value + "." + val.Value, true
		}
		return "", false
	}
	return "", false
}

// enumForMember finds the (deterministically first, alphabetically) user
// enum holding this member, or "" when none does.
func (c *checker) enumForMember(member string) string {
	names := make([]string, 0, len(c.p.enums))
	for name := range c.p.enums {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		enum := c.p.enums[name]
		if enum == nil {
			continue
		}
		for _, m := range enum.Members {
			if m != nil && m.Value == member {
				return name
			}
		}
	}
	return ""
}

// caseKeyDisplay renders a canonical key back into source-like text.
func caseKeyDisplay(key string) string {
	if strings.HasPrefix(key, "num:") || strings.HasPrefix(key, "char:") {
		return "'" + strings.TrimPrefix(strings.TrimPrefix(key, "num:"), "char:") + "'"
	}
	if strings.HasPrefix(key, "str:") {
		return `"` + strings.TrimPrefix(key, "str:") + `"`
	}
	if strings.HasPrefix(key, "range:") {
		bounds := strings.SplitN(strings.TrimPrefix(key, "range:"), "..", 2)
		if len(bounds) == 2 {
			return caseKeyDisplay(bounds[0]) + " to " + caseKeyDisplay(bounds[1])
		}
	}
	if strings.HasPrefix(key, "enum:") {
		return strings.TrimPrefix(key, "enum:")
	}
	if strings.HasPrefix(key, "default:") {
		return "'_' (default)"
	}
	return "'" + strings.TrimPrefix(key, "id:") + "'"
}

// caseNumInterval is a numeric case value for overlap detection: singles
// are points (lo == hi), ranges are intervals.
type caseNumInterval struct {
	lo, hi float64
	label  string
	node   *ASTNode
}

// numericIntervals extracts numeric points/ranges from a case value for
// overlap detection. Non-numeric values yield nothing.
func numericIntervals(val *ASTNode, caseNode *ASTNode) []caseNumInterval {
	if val == nil {
		return nil
	}
	if val.Type == NODE_SWITCH_CASE_LIST {
		var out []caseNumInterval
		for _, child := range val.Children {
			out = append(out, numericIntervals(child, caseNode)...)
		}
		return out
	}
	if val.Type == NODE_SWITCH_CASE_RANGE && len(val.Children) == 2 {
		lo, okLo := caseNumber(val.Children[0])
		hi, okHi := caseNumber(val.Children[1])
		if okLo && okHi {
			if lo > hi {
				lo, hi = hi, lo
			}
			return []caseNumInterval{{lo: lo, hi: hi, label: caseValueText(val.Children[0]) + " to " + caseValueText(val.Children[1]), node: caseNode}}
		}
		return nil
	}
	if v, ok := caseNumber(val); ok {
		return []caseNumInterval{{lo: v, hi: v, label: caseValueText(val), node: caseNode}}
	}
	return nil
}

// caseNumber parses an integer or float case bound.
func caseNumber(val *ASTNode) (float64, bool) {
	if val == nil || val.Type != NODE_NUMBER {
		return 0, false
	}
	if f, err := strconv.ParseFloat(val.Value, 64); err == nil {
		return f, true
	}
	return 0, false
}

// caseValueText renders a case bound in source-like form.
func caseValueText(val *ASTNode) string {
	if val == nil {
		return "?"
	}
	switch val.Type {
	case NODE_NUMBER, NODE_CHAR:
		return "'" + val.Value + "'"
	case NODE_STRING:
		return `"` + val.Value + `"`
	case NODE_IDENTIFIER:
		return val.Value
	case NODE_MEMBER_ACCESS:
		if len(val.Children) > 0 && val.Children[0].Type == NODE_IDENTIFIER {
			return val.Children[0].Value + "." + val.Value
		}
		return val.Value
	}
	return val.Value
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

	case NODE_UNIQUE_ACCESS:
		// `%name` is bell's unique-name scene lookup: it desugars to
		// `node_unique(nodes, "name")`, so the enclosing scope must have
		// the scene array under its conventional name and the engine
		// import must provide node_unique.
		c.checkNameUse(node, "nodes")
		if !c.isCallable("node_unique") {
			c.errorAt(node, "Unique-name access needs 'node_unique' (import the bell engine)")
		}

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
		if node.Value == "bnot" {
			if operand := childAt(node, 0); operand != nil {
				t := c.p.inferType(operand)
				if operandIsKnown(t) && bitwiseOperandInvalid(t) {
					c.errorAt(node, "Bitwise operator 'bnot' requires an integer, got %s", t)
				}
			}
		}
		if node.Value == "^" {
			// Dereferencing a possibly-null pointer crashes. Guarded
			// (or address-taken) names are proved non-null; everything
			// else nullable warns here rather than segfaulting there.
			if operand := childAt(node, 0); operand != nil {
				if root := nullRoot(operand); root != "" {
					t := c.inferNodeType(operand)
					if t == "" {
						t = c.p.inferType(operand)
					}
					if nullableAhoyType(t) && !c.nonNull[root] {
						c.warningAt(node, "Dereferencing '%s', which may be null; guard %s is not null first", root, root)
					}
				}
			}
		}
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

	case NODE_STRING_SLICE:
		// Children are target, lo, hi (999999999 stands in for an
		// omitted hi; clamping makes it exact). The target must be a
		// string; arrays index with arr[i] instead.
		if target := childAt(node, 0); target != nil {
			t := c.inferNodeType(target)
			if t != "" && t != "unknown" && t != "any" && t != "generic" &&
				t != "string" && t != "char*" && t != "const char*" &&
				t != "char *" && t != "const char *" {
				c.errorAt(target, "string slices need a string target (s[lo:hi]); index arrays with arr[i]")
			}
		}
		for _, child := range node.Children {
			c.checkExpr(child)
		}

	case NODE_MEMBER_ACCESS, NODE_OBJECT_ACCESS, NODE_TYPE_PROPERTY:
		// Value is the field name; only the object is a reference.
		c.checkExpr(childAt(node, 0))
		// A field read on a buffer or string dies as "struct or union
		// expected" in C. Buffers expose length only (elements via buf[i]);
		// strings expose length/count (methods use call syntax: s.strip||).
		// Arrays, dicts (key lookup), structs and enums keep their routing.
		if node.Type == NODE_MEMBER_ACCESS {
			if obj := childAt(node, 0); obj != nil && obj.Type == NODE_IDENTIFIER {
				if _, isEnum := c.p.enums[obj.Value]; !isEnum {
					t := c.lookupType(obj.Value)
					if t == "buffer[f32]" || strings.HasPrefix(t, "buffer[") {
						if node.Value != "length" {
							c.errorAt(node, "Type '%s' has no field '%s' (buffers expose length only; read elements with buf[i])", t, node.Value)
						}
					} else if t == "string" || t == "char*" || t == "const char*" || t == "char *" || t == "const char *" {
						if node.Value != "length" && node.Value != "count" {
							c.errorAt(node, "Type 'string' has no field '%s' (string methods use call syntax: s.%s||)", node.Value, node.Value)
						}
					}
				}
			}
		}

	case NODE_STATIC_MEMBER_ACCESS:
		// StructType.#field - the object is a type name, not a variable.

	case NODE_OBJECT_LITERAL:
		// Value is the struct type; children are field: value pairs.
		c.checkObjectLiteral(node)
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

	case NODE_SWITCH_STATEMENT:
		// A switch used for its value must cover every input: without a `_`
		// default the result is uninitialized garbage when nothing matches
		// (this used to compile silently). Statement-position switches need
		// no default - they simply do nothing on no match.
		c.checkExpr(childAt(node, 0))
		for _, caseNode := range node.Children[1:] {
			c.checkSwitchCase(caseNode)
		}
		c.checkSwitchDuplicates(node)
		if !switchHasDefault(node) {
			c.errorAt(node, "switch expression has no default case (' _: ...'); add one so every value is covered")
		}

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
	// A lambda body runs when called, not when defined: flow facts from
	// the enclosing code (guard-narrowed non-nulls) do not hold inside.
	savedNull := c.snapshotNull()
	savedLambdaLoop := c.inLoop
	c.nonNull = map[string]bool{}
	c.inLoop = 0
	for i := paramCount; i < len(node.Children); i++ {
		c.checkExpr(node.Children[i])
	}
	c.inLoop = savedLambdaLoop
	c.restoreNull(savedNull)
	c.popScope()
}

// checkFString checks the identifiers interpolated into an f-string. The
// interpolated text is stored as part of the string value, not as child nodes,
// so the leading identifier of each {..} group is checked by hand.
func (c *checker) checkFString(node *ASTNode) {
	// Pre-parsed interpolation groups take the expression path, mirroring
	// codegen: every {expr} gets full validation (member access, calls,
	// indexing), so {b.x} on a buffer errors here instead of in C.
	if len(node.Children) > 0 {
		for _, child := range node.Children {
			if child == nil || child.Type == NODE_STRING {
				continue
			}
			c.checkExpr(child)
		}
		return
	}
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
	if c.freedNames[name] {
		c.warningAt(node, "Using '%s' after it was freed with free|%s|", name, name)
	}
	if c.isDefined(name) {
		return
	}
	c.reportUndefined(node, name)
}

func (c *checker) reportUndefined(node *ASTNode, name string) {
	msg := fmt.Sprintf("Variable '%s' is not declared", name)
	if keywordHint, ok := controlKeywordHints[name]; ok {
		msg += fmt.Sprintf("; did you mean '%s'?", keywordHint)
	} else if suggestion := c.suggest(name); suggestion != "" {
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
	// Distinct-type conversion: exactly one argument compatible with the
	// base (literals included) or the distinct type itself (identity).
	if base, ok := c.p.distinctTypes[name]; ok {
		args := node.Children
		if len(args) != 1 {
			c.errorAt(node, "Conversion '%s(...)' takes exactly one argument", name)
			return
		}
		actual := c.argActualType(args[0])
		if actual != name && !c.p.checkTypeCompatibility(base, actual, args[0]) {
			c.errorAt(args[0], "Cannot convert %s to %s", actual, name)
		}
		return
	}
	// grow|buf, n| needs a buffer source; the result keeps its type.
	if name == "grow" {
		if len(node.Children) != 2 {
			c.errorAt(node, "grow takes exactly two arguments: grow|buf, n|")
			return
		}
		if actual := c.argActualType(node.Children[0]); actual != "" && actual != "unknown" && actual != "any" && actual != "generic" && !strings.HasPrefix(actual, "buffer[") {
			c.errorAt(node.Children[0], "grow needs a buffer source, not %s", actual)
		}
	}
	// to_bytes|buf| needs a buffer source; from_bytes*|bytes| need an array.
	if name == "to_bytes" {
		if len(node.Children) != 1 {
			c.errorAt(node, "to_bytes takes exactly one argument: to_bytes|buf|")
			return
		}
		if actual := c.argActualType(node.Children[0]); actual != "" && actual != "unknown" && actual != "any" && actual != "generic" && !strings.HasPrefix(actual, "buffer[") {
			c.errorAt(node.Children[0], "to_bytes needs a buffer source, not %s", actual)
		}
	}
	if name == "from_bytes" || name == "from_bytes_int" || name == "from_bytes_ok" {
		if len(node.Children) != 1 {
			c.errorAt(node, "%s takes exactly one argument: %s|bytes|", name, name)
			return
		}
		if actual := c.argActualType(node.Children[0]); actual != "" && actual != "unknown" && actual != "any" && actual != "generic" && actual != "array" && !strings.HasPrefix(actual, "array[") {
			c.errorAt(node.Children[0], "%s needs an array of bytes, not %s", name, actual)
		}
	}
	c.checkArguments(node, name, node.Children)
	// free|x| frees immediately (recorded after the arguments are checked so
	// the call itself doesn't warn): any later use of x in this function
	// dangles. Deferred frees run at scope end and are excluded via inDefer.
	if name == "free" && c.inDefer == 0 && len(node.Children) == 1 &&
		node.Children[0] != nil && node.Children[0].Type == NODE_IDENTIFIER {
		if arg := node.Children[0].Value; arg != "" && arg != "_" {
			c.freedNames[arg] = true
		}
	}
}

// rejectNestedCall reports a call used as another call's argument. The parser
// can make sense of `print|len|items||`, but the nested pipes are easy to
// misread, so the result has to be named first: tmp: len|items| then print|tmp|.
//
// A cast is not a call: int(x), float(x), char(x) and string(x) are type
// conversions. They stay readable inline, so `draw_rectangle|..., int(w * p), ...|`
// is legal; only a call to a function is rejected.
func (c *checker) rejectNestedCall(arg *ASTNode, outer string) {
	if arg == nil {
		return
	}
	if arg.Type == NODE_CALL && isCastName(arg.Value) {
		return
	}
	if arg.Type != NODE_CALL && arg.Type != NODE_METHOD_CALL {
		return
	}
	c.errorAt(arg, "Function call '%s' is not allowed as an argument to '%s'; assign it to a variable first, then pass that variable",
		arg.Value, outer)
}

// isCastName reports whether a call-shaped node with this callee is one of the
// language's type conversions rather than a call to a function. The parser
// produces NODE_CALL for both, so the name is what tells them apart.
func isCastName(name string) bool {
	switch name {
	case "int", "float", "char", "string":
		return true
	}
	return false
}

// checkArguments validates the argument list of a call against a user-defined
// function's parameters. C functions and builtins are not checked: their
// signatures are not fully known here.
func (c *checker) checkArguments(node *ASTNode, name string, args []*ASTNode) {
	for _, arg := range args {
		if arg == nil {
			continue
		}
		if arg.Type == NODE_BINARY_OP && arg.Value == "named_arg" {
			c.rejectNestedCall(childAt(arg, 1), name)
			c.checkExpr(childAt(arg, 1))
			continue
		}
		c.rejectNestedCall(arg, name)
		c.checkExpr(arg)
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

	// Function references flowing into fn-typed parameters must fit them.
	// Only positional and explicitly named &refs are checked; anything else
	// keeps legacy behavior.
	posIdx := 0
	for _, arg := range args {
		if arg == nil {
			continue
		}
		if arg.Type == NODE_BINARY_OP && arg.Value == "named_arg" {
			label, value := childAt(arg, 0), childAt(arg, 1)
			if label != nil {
				for _, param := range params.Children {
					if param != nil && param.Value == label.Value {
						if IsFunctionType(param.DataType) {
							if refName, ok := fnRefName(value); ok {
								c.checkFnRef(value, refName, param.DataType, "parameter '"+label.Value+"'")
							}
						}
						if param.DataType == "char" {
							c.checkCharSlot(value, "parameter '"+label.Value+"'")
						}
						break
					}
				}
			}
			continue
		}
		if posIdx < len(params.Children) {
			if pnode := params.Children[posIdx]; pnode != nil && IsFunctionType(pnode.DataType) {
				if refName, ok := fnRefName(arg); ok {
					c.checkFnRef(arg, refName, pnode.DataType, "parameter '"+pnode.Value+"'")
				}
			}
			if pnode := params.Children[posIdx]; pnode != nil && pnode.DataType == "char" {
				c.checkCharSlot(arg, "parameter '"+pnode.Value+"'")
			}
		}
		posIdx++
	}

	// Distinct-typed parameters are checked strictly (plain call args are
	// otherwise unchecked): only the same distinct name, a literal of the
	// base kind, or an explicit conversion flows in.
	posIdx = 0
	for _, arg := range args {
		if arg == nil {
			continue
		}
		if arg.Type == NODE_BINARY_OP && arg.Value == "named_arg" {
			label, value := childAt(arg, 0), childAt(arg, 1)
			if label != nil && value != nil {
				for _, param := range params.Children {
					if param != nil && param.Value == label.Value {
						_, paramDistinct := c.p.distinctTypes[param.DataType]
						if paramDistinct {
							actual := c.argActualType(value)
							if !c.p.checkTypeCompatibility(param.DataType, actual, value) {
								c.errorAt(value, "Parameter '%s' expects %s but got %s", label.Value, param.DataType, actual)
							}
						} else {
							actual := c.argActualType(value)
							pt := param.DataType
							if _, argDistinct := c.p.distinctTypes[actual]; argDistinct && pt != "" && pt != "any" && pt != "generic" && pt != "unknown" {
								c.errorAt(value, "Parameter '%s' expects %s but got distinct %s (convert explicitly or widen the parameter)", label.Value, param.DataType, actual)
							}
						}
						break
					}
				}
			}
			continue
		}
		if posIdx < len(params.Children) {
			if pnode := params.Children[posIdx]; pnode != nil {
				if _, isDistinct := c.p.distinctTypes[pnode.DataType]; isDistinct {
					actual := c.argActualType(arg)
					if !c.p.checkTypeCompatibility(pnode.DataType, actual, arg) {
						c.errorAt(arg, "Parameter '%s' expects %s but got %s", pnode.Value, pnode.DataType, actual)
					}
				} else {
					actual := c.argActualType(arg)
					pt := pnode.DataType
					if _, argDistinct := c.p.distinctTypes[actual]; argDistinct && pt != "" && pt != "any" && pt != "generic" && pt != "unknown" {
						c.errorAt(arg, "Parameter '%s' expects %s but got distinct %s (convert explicitly or widen the parameter)", pnode.Value, pnode.DataType, actual)
					}
				}
			}
		}
		posIdx++
	}
}

// argActualType resolves a call argument's type for distinct checking: the
// checker's own live scope first (parser scopes are unwound by check time),
// parser inference as fallback.
func (c *checker) argActualType(arg *ASTNode) string {
	if arg != nil && arg.Type == NODE_IDENTIFIER {
		if t := c.lookupType(arg.Value); t != "" {
			return t
		}
	}
	return c.p.inferType(arg)
}

// isTypeName reports whether name denotes a user type (struct or enum) in
// this file's tables. Associated calls (Type.m|...|) resolve against these
// before falling back to legacy behavior.
func (c *checker) isTypeName(name string) bool {
	if _, ok := c.p.structs[name]; ok {
		return true
	}
	_, ok := c.p.enums[name]
	return ok
}

// bufferLaneOf splits a buffer spelling into its lane: "f32" or "int"
// with true, or "" and false when the spelling names no lane at all
// (unknown spellings are rejected where declarations are checked).
func bufferLaneOf(t string) (string, bool) {
	if !strings.HasPrefix(t, "buffer[") || !strings.HasSuffix(t, "]") {
		return "", false
	}
	switch strings.TrimSuffix(strings.TrimPrefix(t, "buffer["), "]") {
	case "f32":
		return "f32", true
	case "int":
		return "int", true
	}
	return "", false
}

// checkNullBlock checks a conditional body (loop body, switch/when
// arm) under a snapshot: proofs established inside must not leak into
// the enclosing flow, which may never execute the body. Loop bodies
// also count loop depth for the halt/next divergence rule below.
func (c *checker) checkNullBlock(node *ASTNode) {
	saved := c.snapshotNull()
	c.inLoop++
	c.checkBlock(node)
	c.inLoop--
	c.restoreNull(saved)
}

// guardDiverges reports whether a guard's else arm cannot fall through
// to the code after it: `return` always diverges; a bare `halt`/`next`
// diverges only inside a loop (at top level neither provably leaves).
func (c *checker) guardDiverges(body *ASTNode) bool {
	if blockAlwaysReturns(body) {
		return true
	}
	if c.inLoop > 0 && body != nil && body.Type == NODE_BLOCK && len(body.Children) == 1 {
		switch body.Children[0].Type {
		case NODE_HALT, NODE_NEXT:
			return true
		}
	}
	return false
}

// snapshotNull copies the null-state for scoped checking (if arms,
// lambdas); restoreNull puts it back. The maps are never mutated
// through an alias afterwards — every snapshot is fresh.
func (c *checker) snapshotNull() map[string]bool {
	m := make(map[string]bool, len(c.nonNull))
	for k, v := range c.nonNull {
		m[k] = v
	}
	return m
}

func (c *checker) restoreNull(m map[string]bool) {
	c.nonNull = m
}

// checkBufferSpelling rejects buffer element spellings with no lane.
// buffer[int] used to be accepted and silently stored as float32, so
// counts above 2^24 corrupted; an explicit error beats quiet loss.
func (c *checker) checkBufferSpelling(t string, node *ASTNode) {
	if !strings.HasPrefix(t, "buffer[") {
		return
	}
	if _, ok := bufferLaneOf(t); !ok {
		c.errorAt(node, "Unknown buffer type '%s'; write buffer[f32] or buffer[int]", t)
	}
}

// nullableAhoyType reports whether a value of this type can hold NULL:
// pointers of any spelling, plus the heap-container spellings. Scalars
// can never be null (comparing one to null is an error); anything
// unknown stays silent (lenient unknown, like the grow rule).
func nullableAhoyType(t string) bool {
	if t == "" || t == "unknown" || t == "any" || t == "generic" {
		return false
	}
	if strings.HasSuffix(t, "*") {
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

// scalarAhoyType reports whether a value of this type can never be null:
// comparing one to null is provably dead code, not a maybe.
func scalarAhoyType(t string) bool {
	switch t {
	case "int", "float", "double", "bool", "char", "f32", "f64",
		"i8", "i16", "i32", "i64", "u8", "u16", "u32", "u64":
		return true
	}
	return false
}

// nullRoot names the tracked holder behind an expression: a bare
// identifier, or a dotted member chain over identifiers
// (h.ext, cur.swarm.sx). Calls, indices and anything else have no
// stable name to track, so they read as untracked ("").
func nullRoot(node *ASTNode) string {
	if node == nil {
		return ""
	}
	if node.Type == NODE_IDENTIFIER {
		return node.Value
	}
	if node.Type == NODE_MEMBER_ACCESS && node.Value != "" && len(node.Children) > 0 {
		if base := nullRoot(node.Children[0]); base != "" {
			return base + "." + node.Value
		}
	}
	return ""
}

// nullImplies reads a null test the way the flow needs it. negate=false
// asks what the test's own arm knows (if X is null then A: X is null in
// A); negate=true asks what the fall-through knows (the arm diverged,
// so execution continues with the condition false). NOTs are counted,
// so `if p is not null` (parser: not(p is null)) and guard's double
// negation both resolve. Only a bare `is` against the null literal
// counts; match lists, method calls and anything else is not a test.
func nullImplies(cond *ASTNode, negate bool) (root string, isNull bool, ok bool) {
	n := 0
	for cond != nil && cond.Type == NODE_UNARY_OP && cond.Value == "not" && len(cond.Children) == 1 {
		n++
		cond = cond.Children[0]
	}
	if cond == nil || cond.Type != NODE_BINARY_OP || cond.Value != "is" || len(cond.Children) != 2 {
		return "", false, false
	}
	left, right := cond.Children[0], cond.Children[1]
	var subject *ASTNode
	if right.Type == NODE_NULL_LITERAL {
		subject = left
	} else if left.Type == NODE_NULL_LITERAL {
		subject = right
	} else {
		return "", false, false
	}
	root = nullRoot(subject)
	if root == "" {
		return "", false, false
	}
	// An even number of NOTs tests is-null; the fall-through inverts it.
	isNull = n%2 == 0
	if negate {
		isNull = !isNull
	}
	return root, isNull, true
}

// checkBufferLane agrees a stored buffer value with its destination's
// lane. A mismatched lane would compile to an incompatible C pointer:
// loud here, garbage there. Unknown sides stay silent (lenient unknown,
// like the grow rule).
func (c *checker) checkBufferLane(want, got string, node *ASTNode, what string) {
	wantLane, wantOk := bufferLaneOf(want)
	gotLane, gotOk := bufferLaneOf(got)
	if !wantOk || !gotOk || wantLane == gotLane {
		return
	}
	c.errorAt(node, "%s wants %s but receives %s; convert explicitly or match the lane", what, want, got)
}

// trackNullStore updates null-state for a store to root holding value:
// taking an address is never null, anything else is unknown again (a
// previous proof died with the old value). Untrackable targets ("")
// are ignored.
func (c *checker) trackNullStore(root string, value *ASTNode) {
	if root == "" {
		return
	}
	delete(c.nonNull, root)
	if value != nil && value.Type == NODE_UNARY_OP && value.Value == "&" {
		c.nonNull[root] = true
	}
}

// bufferFieldLane resolves the declared lane of a member-store target
// x.f (one level; deeper chains fall back to the checker's silence and
// the C compiler's pointer warning). Returns "", false when unknown.
func (c *checker) bufferFieldLane(target *ASTNode) (string, bool) {
	if target == nil || target.Type != NODE_MEMBER_ACCESS || target.Value == "" {
		return "", false
	}
	if len(target.Children) == 0 || target.Children[0].Type != NODE_IDENTIFIER {
		return "", false
	}
	holder := c.lookupType(target.Children[0].Value)
	if holder == "" {
		return "", false
	}
	holder = strings.TrimPrefix(holder, "struct:")
	for _, name := range []string{holder, strings.ToLower(holder[:1]) + holder[1:]} {
		if def, ok := c.p.structs[name]; ok {
			for _, f := range def.Fields {
				if f.Name == target.Value {
					if lane, ok := bufferLaneOf(f.Type); ok {
						return lane, true
					}
					return "", false
				}
			}
		}
	}
	return "", false
}

// checkMethodDeclaration validates the type side of a method or associated
// definition. The receiver (or associated target) must name something methods
// can attach to: a struct, an enum, a builtin scalar, a builtin nominal type
// (pool, job), a compound builtin shape (array[...], dict...), or an imported
// C struct. Anything else is a typo that would otherwise compile to a method
// that can never resolve.
func (c *checker) checkMethodDeclaration(node *ASTNode) {
	t := node.Method.Receiver
	if c.isTypeName(t) {
		return
	}
	switch t {
	case "int", "float", "string", "char", "bool", "any", "pool", "job":
		return
	}
	if strings.HasPrefix(t, "array[") || strings.HasPrefix(t, "dict<") || strings.HasPrefix(t, "dict[") {
		return
	}
	for _, h := range c.p.cHeaders {
		if h != nil {
			if _, ok := h.Structs[t]; ok {
				return
			}
		}
	}
	if c.p.cHeaderGlobal != nil {
		if _, ok := c.p.cHeaderGlobal.Structs[t]; ok {
			return
		}
	}
	what := "method receiver"
	if node.Method.Associated {
		what = "associated function target"
	}
	c.errorAt(node, "Unknown type '%s' in %s '%s.%s'; methods attach to structs, enums and builtin types", t, what, t, node.Method.Name)
}

// resolveMethodCall maps recv.m|args| onto the method table. It returns the
// mangled global name and whether the receiver is passed as the first
// argument (false for associated calls, which pass none). The receiver type
// prefers the parse-time record on the call node (scopes are accurate during
// parsing, reset afterwards), falling back to inference.
func (c *checker) resolveMethodCall(call *ASTNode) (string, bool, bool) {
	if call == nil || len(call.Children) < 1 {
		return "", false, false
	}
	object := call.Children[0]
	method := call.Value
	// Associated call: the receiver position names a type, not a value.
	if object != nil && object.Type == NODE_IDENTIFIER && c.isTypeName(object.Value) {
		if e, ok := c.methods[object.Value][method]; ok && e.Info.Associated {
			return MangleMethodName(object.Value, method), false, true
		}
		return "", false, false
	}
	recvType := call.RecvType
	if recvType == "" {
		recvType = NormalizeReceiverType(c.p.inferType(object))
	}
	if e, ok := c.methods[recvType][method]; ok && !e.Info.Associated {
		return MangleMethodName(recvType, method), true, true
	}
	return "", false, false
}

// structFieldIsFunction reports whether the named field of the struct holds a
// function value. Such a field is called through the field, not through a
// method of the same name.
func (c *checker) structFieldIsFunction(recvType, field string) bool {
	si, ok := c.p.structs[recvType]
	if !ok || si == nil {
		return false
	}
	for _, f := range si.Fields {
		if f.Name == field {
			return IsFunctionType(f.Type)
		}
	}
	return false
}

// methodHint lists a type's known methods for "no such method" diagnostics.
func (c *checker) methodHint(recvType string) string {
	ms, ok := c.methods[recvType]
	if !ok || len(ms) == 0 {
		return ""
	}
	names := make([]string, 0, len(ms))
	for name := range ms {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > 8 {
		names = append(names[:8], "...")
	}
	return " (methods: " + strings.Join(names, ", ") + ")"
}

func (c *checker) checkMethodCall(node *ASTNode) {
	if len(node.Children) < 2 {
		return
	}
	object := node.Children[0]
	method := node.Value

	// `namespace.func|..|` - the object is an imported C namespace, not a value.
	if object != nil && object.Type == NODE_IDENTIFIER {
		if _, isNamespace := c.p.cHeaders[object.Value]; isNamespace {
			c.checkNamespacedCall(node, object.Value)
			return
		}
	}

	// A field that holds a function value is called through the field:
	// `cur.ready|nodes, idx|` calls the Node's `ready` field. A struct that
	// has methods must not shadow its own function-valued fields, or every
	// such call is reported as a missing method.
	//
	// The receiver has to be a name: the code generator reads the callback
	// out of a named struct, so `nodes[i].ready|...|` is not a call the
	// generator can make (it would read the field off the array, not off
	// the element). Bind the element to a local first.
	fieldRecvType := node.RecvType
	if fieldRecvType == "" {
		fieldRecvType = NormalizeReceiverType(c.p.inferType(object))
	}
	if object.Type == NODE_IDENTIFIER && c.structFieldIsFunction(fieldRecvType, method) {
		c.checkExpr(object)
		if args := node.Children[1]; args != nil {
			for _, arg := range args.Children {
				c.checkExpr(arg)
			}
		}
		return
	}

	// A callback field reached through an expression (`nodes[i].ready|...|`)
	// is a call the generator cannot make: it reads the callback out of a
	// named struct. Say so, instead of calling it a missing method.
	if object.Type != NODE_IDENTIFIER {
		fieldRecvType := node.RecvType
		if fieldRecvType == "" {
			fieldRecvType = NormalizeReceiverType(c.p.inferType(object))
		}
		if c.structFieldIsFunction(fieldRecvType, method) {
			c.errorAt(node, "'%s' is a callback field: bind the value to a name first (ready: fn(...) = nodes[i].ready; ready|...|)", method)
			return
		}
	}

	// User-defined method or associated function: validate the call with the
	// receiver prepended (methods) or as written (associated), reusing the
	// ordinary argument checking (arity, defaults, named arguments).
	if mangled, withRecv, ok := c.resolveMethodCall(node); ok {
		var callArgs []*ASTNode
		if len(node.Children) > 1 && node.Children[1] != nil {
			callArgs = node.Children[1].Children
		}
		if withRecv {
			callArgs = append([]*ASTNode{object}, callArgs...)
		}
		c.checkArguments(node, mangled, callArgs)
		return
	}

	c.checkExpr(object)
	args := node.Children[1]
	if args != nil {
		for _, arg := range args.Children {
			c.checkExpr(arg)
		}
	}

	// Storing an f-string into a container keeps the shared format buffer:
	// every element aliases one static buffer and later evaluations
	// overwrite earlier ones. Bind to a variable first (bound f-strings own
	// their copy).
	if method == "push" || method == "fill" {
		if args != nil {
			for _, arg := range args.Children {
				if arg != nil && arg.Type == NODE_F_STRING {
					c.warningAt(arg, "Storing an f-string keeps the shared format buffer; bind it to a variable first so each element owns its value")
					break
				}
			}
		}
	}

	// A method-shaped call on a type with methods that missed the table is a
	// typo: without this the call falls through to legacy routing and fails
	// obscurely in the generated C.
	if object != nil && object.Type == NODE_IDENTIFIER && c.isTypeName(object.Value) {
		c.errorAt(node, "Type '%s' has no associated function '%s'", object.Value, method)
		return
	}
	recvType := node.RecvType
	if recvType == "" {
		recvType = NormalizeReceiverType(c.p.inferType(object))
	}
	// Fixed-size buffers expose len only; mutating methods (push/pop/fill
	// and friends) would corrupt the struct as an AhoyArray, so reject
	// them here instead of failing obscurely in generated C.
	if recvType == "buffer[f32]" || strings.HasPrefix(recvType, "buffer[") {
		if method != "len" && method != "length" {
			c.errorAt(node, "Type '%s' has no method '%s' (fixed-size buffer)", recvType, method)
			return
		}
		return
	}
	// each is deliberately not a method: a lambda body is a pure expression
	// (calls do not parse there), so each could only silently do nothing.
	// A missing name would otherwise fall through to codegen and die as a
	// C link error; say so here instead.
	if recvType == "array" || strings.HasPrefix(recvType, "array[") {
		if method == "each" {
			c.errorAt(node, "'each' is not supported: a lambda body is a pure expression (calls do not parse there), so each could only silently do nothing; write a loop instead")
			return
		}
	}
	if _, hasImpl := c.methods[recvType]; hasImpl {
		c.errorAt(node, "Type '%s' has no method '%s'%s", recvType, method, c.methodHint(recvType))
		return
	}
	if _, isStruct := c.p.structs[recvType]; isStruct {
		// A same-named field (notably a callback like ready/process) means
		// this is a field call, not a method miss.
		if c.p.structHasField(recvType, method) {
			return
		}
		c.errorAt(node, "Type '%s' has no method '%s'%s", recvType, method, c.methodHint(recvType))
		return
	}
	// Array push/fill intentionally don't warn: elements may be static
	// (literals) or caller-owned, so autofree cannot free them safely, and
	// long-lived log-style arrays would nag on every push. Same policy as
	// field stores above.
}

// checkCharSlot rejects a multi-character string literal flowing into a
// char slot: codegen would emit the pointer as a byte (garbage). Exactly
// one character coerces to 'x'; anything else needs s[i] or a string.
func (c *checker) checkCharSlot(value *ASTNode, targetDesc string) {
	if value != nil && value.Type == NODE_STRING {
		if r := []rune(value.Value); len(r) != 1 {
			c.errorAt(value, "Cannot store %d-character string in char (%s); use a 1-character literal or s[i]", len(r), targetDesc)
		}
	}
}

// fnRefName reports whether a value node is a &name function reference, and
// if so the referenced name. Address-of on variables shares the shape; only
// references that resolve to a known function are checked by callers.
func fnRefName(node *ASTNode) (string, bool) {
	if node == nil || node.Type != NODE_UNARY_OP || node.Value != "&" {
		return "", false
	}
	if len(node.Children) < 1 || node.Children[0] == nil || node.Children[0].Type != NODE_IDENTIFIER {
		return "", false
	}
	return node.Children[0].Value, true
}

// checkFnRef validates a &name value flowing into an fn-typed slot
// (slotFnType like `fn(int,int)->int`, slotDesc like `field 'draw'`).
// Unknown references stay silent here; undefined names are reported where
// the reference itself is checked.
func (c *checker) checkFnRef(node *ASTNode, refName, slotFnType, slotDesc string) {
	sig, ok := c.p.functions[refName]
	if !ok || sig == nil {
		return
	}
	sev, msg := CheckFnRefAgainstSlot(refName, sig, slotFnType, slotDesc)
	if sev == "warning" {
		c.warningAt(node, "%s", msg)
	} else if sev != "" {
		c.errorAt(node, "%s", msg)
	}
}

// checkObjectLiteral validates a typed struct literal in any position
// (declarations with or without annotation, returns, elements, arguments):
// every property must exist on the type, every fn-typed field must be
// present (a missing one would be a null call), and every &name flowing
// into an fn-typed field must fit it.
func (c *checker) checkObjectLiteral(node *ASTNode) {
	if node == nil || node.Value == "" {
		return
	}
	typeName := NormalizeReceiverType(node.Value)
	structDef, ok := c.p.structs[typeName]
	if !ok {
		return
	}
	present := map[string]*ASTNode{}
	for _, prop := range node.Children {
		if prop == nil || prop.Type != NODE_OBJECT_PROPERTY {
			continue
		}
		if !c.p.structHasField(typeName, prop.Value) {
			c.errorAt(prop, "Invalid property: '%s' does not exist in type '%s'", prop.Value, structDef.Name)
			continue
		}
		if len(prop.Children) > 0 {
			present[prop.Value] = prop.Children[0]
		}
	}
	for _, field := range structDef.Fields {
		if !IsFunctionType(field.Type) {
			continue
		}
		value, ok := present[field.Name]
		if !ok {
			// A field with a default (on_tick: fn()->int = &tick) is filled
			// from that default, so omitting it is not a null call.
			if field.DefaultValue != nil {
				continue
			}
			c.errorAt(node, "Struct '%s' literal is missing function field '%s' (%s); calling it would be a null call",
				structDef.Name, field.Name, field.Type)
			continue
		}
		if refName, ok := fnRefName(value); ok {
			c.checkFnRef(value, refName, field.Type, "field '"+field.Name+"'")
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

var bitwiseOps = map[string]bool{
	"band": true, "bor": true, "bxor": true, "shl": true, "shr": true,
}

// bitwiseOperandInvalid reports whether a type cannot take part in a bitwise
// operation. bool and char are left alone: they are integers in C.
func bitwiseOperandInvalid(t string) bool {
	return t == "float" || t == "double" ||
		isStringType(t) || isCollectionType(t) || isStructType(t)
}

// checkBitwiseOperands rejects bitwise operators on operands that are certainly
// not integers, so `1.5 band 2` does not silently truncate in C.
func (c *checker) checkBitwiseOperands(node *ASTNode, op string) {
	for _, n := range []*ASTNode{childAt(node, 0), childAt(node, 1)} {
		if n == nil {
			continue
		}
		t := c.p.inferType(n)
		if !operandIsKnown(t) {
			continue
		}
		if bitwiseOperandInvalid(t) {
			c.errorAt(node, "Bitwise operator '%s' requires integers, got %s", op, t)
			return
		}
	}
}

// checkBinaryOp rejects operand types that cannot be combined. Only cases that
// are certainly wrong are reported: an int plus a string currently compiles and
// prints the string's address, and string ordering compares pointers, so both
// produce a program that runs and silently does the wrong thing.
func (c *checker) checkBinaryOp(node *ASTNode) {
	op := node.Value
	if bitwiseOps[op] {
		c.checkBitwiseOperands(node, op)
		return
	}
	if op == "is" {
		// A scalar compared to null is provably dead: no int, float,
		// bool or char is ever null. Pointers, containers and unknown
		// shapes stay silent.
		left, right := childAt(node, 0), childAt(node, 1)
		if left != nil && right != nil {
			lNull := left.Type == NODE_NULL_LITERAL
			rNull := right.Type == NODE_NULL_LITERAL
			if lNull != rNull {
				other := right
				if rNull {
					other = left
				}
				t := c.inferNodeType(other)
				if t == "" {
					t = c.p.inferType(other)
				}
				if t == "" {
					switch other.Type {
					case NODE_NUMBER, NODE_BOOLEAN, NODE_CHAR:
						t = "int"
					}
				}
				if scalarAhoyType(t) {
					c.errorAt(node, "Type '%s' can never be null; comparing it to null is always false", t)
				}
			}
		}
		return
	}
	if !arithmeticOps[op] && !orderingOps[op] {
		return
	}
	left, right := childAt(node, 0), childAt(node, 1)
	if left == nil || right == nil {
		return
	}
	lt := c.p.inferType(left)
	rt := c.p.inferType(right)

	// String concatenation: + on two strings builds a new string.
	if (op == "+" || op == "plus") && isStringType(lt) && isStringType(rt) {
		return
	}

	if operandsUnusable(lt, rt) {
		c.errorAt(node, "Operator '%s' cannot be applied to %s and %s", op, lt, rt)
	}
	// -strict: integer division/modulo by a literal zero always SIGFPEs.
	// Float division stays silent (it yields inf, which is legal).
	if StrictLint && (op == "div" || op == "/" || op == "mod" || op == "%") {
		if right != nil && right.Type == NODE_NUMBER && strictZeroNumber(right.Value) {
			floatLHS := lt == "float" || lt == "double"
			if !floatLHS && left != nil && left.Type == NODE_IDENTIFIER {
				if t := c.lookupType(left.Value); t == "float" || t == "double" {
					floatLHS = true
				}
			}
			if !floatLHS {
				c.warningAt(node, "Division by zero always crashes; guard the divisor")
			}
		}
	}
}

// strictArrayElem extracts T from array[T].
func strictArrayElem(t string) (string, bool) {
	if strings.HasPrefix(t, "array[") && strings.HasSuffix(t, "]") {
		return strings.TrimSuffix(strings.TrimPrefix(t, "array["), "]"), true
	}
	return "", false
}

// strictElemGroup buckets element types by how a misread behaves: strings
// and heap-boxed floats are pointers (reading them as int yields garbage
// addresses; reading ints as them dereferences garbage). any/generic
// group to "" (never mismatches). Everything else must match exactly.
func strictElemGroup(t string) string {
	switch t {
	case "string", "char*", "const char*":
		return "ptr-str"
	case "float", "double":
		return "ptr-float"
	case "int", "intptr_t", "bool", "char":
		return "int"
	case "any", "generic", "unknown", "":
		return ""
	default:
		return "T:" + t
	}
}

func strictElemMismatch(declared, elem string) bool {
	g1, g2 := strictElemGroup(declared), strictElemGroup(elem)
	if g1 == "" || g2 == "" {
		return false
	}
	return g1 != g2
}

// strictZeroNumber reports whether a numeric literal is zero (0, 0.0;
// hex excluded so 0x10 never counts).
func strictZeroNumber(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" || strings.ContainsAny(t, "xX") {
		return false
	}
	f, err := strconv.ParseFloat(t, 64)
	return err == nil && f == 0
}

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
