package ahoy

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Core parser: node types, token navigation, diagnostics and the program loop.

type NodeType int

const (
	NODE_PROGRAM NodeType = iota
	NODE_PROGRAM_DECLARATION
	NODE_FUNCTION
	NODE_VARIABLE_DECLARATION
	NODE_ASSIGNMENT
	NODE_IF_STATEMENT
	NODE_SWITCH_STATEMENT
	NODE_SWITCH_CASE
	NODE_SWITCH_CASE_LIST  // Multiple cases like 'A','B','C'
	NODE_SWITCH_CASE_RANGE // Range case like 'a' to 'z'
	NODE_WHILE_LOOP
	NODE_FOR_LOOP
	NODE_FOR_RANGE_LOOP    // loop:start to end
	NODE_FOR_COUNT_LOOP    // loop:start or loop (defaults to 0)
	NODE_FOR_IN_ARRAY_LOOP // loop element in array
	NODE_FOR_IN_DICT_LOOP  // loop key,value in dict
	NODE_RETURN_STATEMENT
	NODE_IMPORT_STATEMENT
	NODE_WHEN_STATEMENT
	NODE_EXPRESSION
	NODE_BINARY_OP
	NODE_UNARY_OP
	NODE_UNIQUE_ACCESS // %name: unique-name scene node lookup (bell engine)
	NODE_CALL
	NODE_IDENTIFIER
	NODE_NUMBER
	NODE_STRING
	NODE_RAW_STRING // Raw string with backticks - no escape sequences, preserves newlines
	NODE_F_STRING   // f-string with interpolation
	NODE_CHAR
	NODE_BOOLEAN
	NODE_DICT_LITERAL
	NODE_ARRAY_LITERAL
	NODE_ARRAY_ACCESS
	NODE_DICT_ACCESS
	NODE_BLOCK
	NODE_TYPE
	NODE_ENUM_DECLARATION
	NODE_CONSTANT_DECLARATION
	NODE_TUPLE_ASSIGNMENT
	NODE_STRUCT_DECLARATION
	NODE_ALIAS_DECLARATION
	NODE_DISTINCT_DECLARATION
	NODE_UNION_DECLARATION
	NODE_METHOD_CALL
	NODE_MEMBER_ACCESS
	NODE_STATIC_MEMBER_ACCESS // StructType.#static_field access
	NODE_HALT
	NODE_NEXT
	NODE_GOTO_STATEMENT
	NODE_LABEL_DECLARATION
	NODE_LAMBDA
	NODE_TERNARY
	NODE_ASSERT_STATEMENT
	NODE_DEFER_STATEMENT
	NODE_OBJECT_LITERAL
	NODE_OBJECT_PROPERTY
	NODE_OBJECT_ACCESS
	NODE_TYPE_PROPERTY // .type property access
	NODE_IS_LIST       // comma sugar: x is A, B, C (one subject, many values)
)

type ASTNode struct {
	Type         NodeType
	Value        string
	Children     []*ASTNode
	DataType     string
	Line         int
	Column       int         // Column position in source
	File         string      // Source file; set on top-level nodes only (see stampNodeFiles)
	DefaultValue *ASTNode    // For default parameter values
	Method       *MethodInfo // Non-nil for method/associated function declarations
	RecvType     string      // Method calls: receiver type recorded at parse time ("" if unknown)
	EnumType     string      // Type of enum (int, string, color, etc.) or "" for mixed
	IsMutable    bool        // For enum members marked as mutable
	IsStatic     bool        // For struct fields prefixed with # (static/shared)
	IsConst      bool        // For struct fields in SCREAMING_SNAKE_CASE (immutable)
}

// ParseError, its File/Severity fields and stampErrorFiles live in
// diagnostics.go.

type StructField struct {
	Name         string
	Type         string
	DefaultValue *ASTNode
	IsStatic     bool // Field prefixed with # is static (shared across instances)
	IsConst      bool // Field is SCREAMING_SNAKE_CASE (immutable)
}

type StructDefinition struct {
	Name   string
	Fields []StructField
	Parent string // For nested types like smoke_particle extends particle
	Line   int    // Line where struct is declared
}

// EnumDefinition stores information about an enum
type EnumDefinition struct {
	Name    string
	Members []*ASTNode
	Line    int // Line where enum is declared
}

// FunctionSignature stores information about a function
type FunctionSignature struct {
	Name         string
	Parameters   []ParameterInfo
	ReturnTypes  []string
	IsInfer      bool     // True if return type is "infer"
	FunctionNode *ASTNode // Reference to function AST for inference
	Line         int      // Line where function is declared
}

// ParameterInfo stores parameter information
type ParameterInfo struct {
	Name string
	Type string // "any" if no type specified
}

// ArrayInfo stores information about an array's length
type ArrayInfo struct {
	Length  int // -1 if unknown
	IsKnown bool
}

// C Header parsing types
type CFunction struct {
	Name       string
	ReturnType string
	Parameters []CParameter
	Line       int    // Line number in header file
	File       string // Path to the header file
}

type CParameter struct {
	Name string
	Type string
}

type CEnum struct {
	Name       string
	Values     map[string]int
	ValueLines map[string]int // Line number for each enum value
	Line       int            // Line number in header file
	File       string         // Path to the header file
}

type CDefine struct {
	Name  string
	Value string
	Line  int    // Line number in header file
	File  string // Path to the header file
}

type CStructField struct {
	Name string
	Type string
}

type CStruct struct {
	Name   string
	Fields []CStructField
	Line   int    // Line number in header file
	File   string // Path to the header file
}

type CTypedef struct {
	AliasName string
	BaseType  string
	Line      int
	File      string
}

// CFuncPtrTypedef is a C function-pointer typedef, e.g.
// typedef void (*TraceLogCallback)(int logLevel, const char *text);
// It lets an Ahoy function be passed to a C API that takes a callback.
type CFuncPtrTypedef struct {
	Name       string
	ReturnType string
	Parameters []CParameter
	Line       int
	File       string
}

type CHeaderInfo struct {
	Functions       map[string]*CFunction
	Enums           map[string]*CEnum
	Defines         map[string]*CDefine
	Structs         map[string]*CStruct
	Typedefs        map[string]*CTypedef        // alias name -> typedef info
	FuncPtrTypedefs map[string]*CFuncPtrTypedef // callback typedef name -> signature
}

type Parser struct {
	tokens              []Token
	pos                 int
	inFunctionCall      int // Depth counter for nested function calls
	inArrayLiteral      bool
	inObjectLiteral     bool
	inDictLiteral       bool
	LintMode            bool
	Errors              []ParseError
	variableTypes       map[string]string             // Track variable types
	constants           map[string]int                // Track constant declarations (name -> line number)
	constantsInMain     map[string]bool               // Track constants declared in main function
	constantUsages      map[string][]int              // Track where identifiers are used (name -> list of line numbers)
	declaredVars        map[string]int                // Track variable declarations (name -> line number) for error reporting
	functionScopeStack  []map[string]string           // Stack of function scopes for conditional blocks
	structs             map[string]*StructDefinition  // Track struct definitions
	enums               map[string]*EnumDefinition    // Track enum definitions
	typeAliases         map[string]string             // Track type aliases
	distinctTypes       map[string]string             // Distinct newtypes: name -> base type
	unionTypes          map[string][]string           // Track union types
	objectLiterals      map[string]map[string]bool    // Track object literal properties by variable name
	currentFunctionRet  string                        // Track current function return type
	currentFunctionName string                        // Track current function being defined (for recursion detection)
	functionScope       map[string]string             // Track function-local variables
	seenNonImport       bool                          // Track if we've seen non-import statements
	functions           map[string]*FunctionSignature // Track function signatures
	arrayLengths        map[string]ArrayInfo          // Track array lengths
	cHeaders            map[string]*CHeaderInfo       // Track imported C headers (namespace -> header info)
	cHeaderGlobal       *CHeaderInfo                  // Global C header imports (no namespace)
	preParsedHeaders    map[string]*CHeaderInfo       // Pre-parsed C headers (path -> header info) for parallel parsing
	blockDepth          int                           // Track nesting depth of multi-line blocks
	loopVarScopes       []map[string]string           // Stack of loop variable scopes
	functionDepth       int                           // Track nesting depth of function definitions
	hasProgramDecl      bool                          // Track if program declaration exists
	inFunctionBody      bool                          // Track if we're inside a function body
	sourceFilePath      string                        // Source file path for resolving relative imports
	zeroArgFunctions    map[string]bool               // O(1) lookup for zero-argument functions (snake_case name -> true)
	// inFunctionReference is set while parsing the operand of `&`: the
	// operand is a function value, so a zero-argument function must not be
	// auto-called into a call node there.
	inFunctionReference bool

	// knownVars records declared variable names in BOTH modes. declaredVars and
	// functionScope are only maintained while linting, but code generation also
	// needs to tell a variable reference from a zero-argument call while it
	// builds the AST - see nameIsKnownVariable.
	knownVars map[string]bool
	// scopeAliased is true while LintMode branch scopes alias the enclosing
	// maps instead of copying them (see ensureOwnScope). Branches that
	// declare nothing then cost no copies at all.
	scopeAliased bool
	// sourceLines caches the raw source file (split by newline) for
	// indentation measurements backing the dedent-swallowed-statement
	// warning. sourceLinesReady marks the load attempt (lint parses only).
	sourceLines      []string
	sourceLinesReady bool
	// lastFuncClose tracks the most recent function-level `$` so a later
	// Superfluous `$` can point back at the extra `$` that closed the
	// function early (the bell/editor/main.ahoy cascade: one surplus `$`
	// closes @main, the rest of the body parses at global scope).
	lastFuncName      string
	lastFuncStartLine int
	lastFuncCloseLine int
}

// The public parse entry points (Parse, ParseWithPath, ParseLint, ...)
// live in lint.go.

func (p *Parser) current() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TOKEN_EOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) peek(offset int) Token {
	pos := p.pos + offset
	if pos >= len(p.tokens) {
		return Token{Type: TOKEN_EOF}
	}
	return p.tokens[pos]
}

func (p *Parser) skipNewlines() {
	for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_SEMICOLON {
		p.advance()
	}
}

func (p *Parser) recordError(message string) {
	token := p.current()
	p.Errors = append(p.Errors, ParseError{
		Message: message,
		Line:    token.Line,
		Column:  token.Column,
	})
}

// recordWarningAtLine records an advisory (non-fatal) diagnostic at a
// specific line. Warnings are printed but never fail a build.
func (p *Parser) recordWarningAtLine(message string, line int, column int) {
	p.Errors = append(p.Errors, ParseError{
		Message:  message,
		Line:     line,
		Column:   column,
		Severity: "warning",
	})
}

// sourceLineIndent returns the indentation width (spaces, tabs count as 2 to
// match the tokenizer) of a 1-based source line, or -1 when the source is
// unavailable (non-lint parses, stdin, unreadable files). It backs the
// dedent-swallowed-statement warning: token columns are relative to the
// trimmed line, so only the raw source reveals real indentation.
func (p *Parser) sourceLineIndent(line int) int {
	if !p.LintMode || line <= 0 {
		return -1
	}
	if !p.sourceLinesReady {
		p.sourceLinesReady = true
		if p.sourceFilePath != "" {
			if data, err := os.ReadFile(p.sourceFilePath); err == nil {
				p.sourceLines = strings.Split(string(data), "\n")
			}
		}
	}
	if line > len(p.sourceLines) {
		return -1
	}
	text := p.sourceLines[line-1]
	width := 0
	for _, ch := range text {
		if ch == ' ' {
			width++
		} else if ch == '\t' {
			width += 2
		} else {
			break
		}
	}
	return width
}

// trackBlockIndent records a $-block statement's indentation and warns when
// a statement following a dedent sits below every statement seen so far in
// the block. Indentation is cosmetic in ahoy — only `$` closes a block — so
// such a statement still belongs to the block even though it looks escaped.
// minIndent holds the running minimum (-1 until the first statement); it is
// updated in place.
func (p *Parser) trackBlockIndent(construct string, sawDedent bool, stmtLine int, minIndent *int) {
	indent := p.sourceLineIndent(stmtLine)
	if indent < 0 {
		return
	}
	if *minIndent < 0 || indent < *minIndent {
		if sawDedent && *minIndent >= 0 {
			p.recordWarningAtLine(fmt.Sprintf("Statement is dedented below the '%s' block body but still belongs to it (only '$' closes a block); indent it to column %d or move it after '$'", construct, *minIndent), stmtLine, indent+1)
		}
		*minIndent = indent
	}
}
func (p *Parser) recordErrorAtLine(message string, line int) {
	p.Errors = append(p.Errors, ParseError{
		Message: message,
		Line:    line,
		Column:  1, // Default to column 1 for block-level errors
	})
}

// validateNoGlobalFunctionCalls checks if a statement contains function calls at global scope
func (p *Parser) validateNoGlobalFunctionCalls(node *ASTNode) {
	if node == nil {
		return
	}

	// Check if this node is a function call
	if node.Type == NODE_CALL {
		funcName := node.Value
		// Allow C functions from headers (they're in cHeaders or cHeaderGlobal)
		isCFunction := false
		if p.cHeaderGlobal != nil {
			if _, exists := p.cHeaderGlobal.Functions[funcName]; exists {
				isCFunction = true
			}
		}
		for _, header := range p.cHeaders {
			if _, exists := header.Functions[funcName]; exists {
				isCFunction = true
				break
			}
		}

		if !isCFunction {
			p.recordErrorAtLine(fmt.Sprintf("Function call '%s' not allowed at global scope when program is declared. Functions can only be called from within other functions.", funcName), node.Line)
		}
	}

	// Recursively check children
	for _, child := range node.Children {
		p.validateNoGlobalFunctionCalls(child)
	}
}

// validateFunctionCall checks for recursion and nested function calls (higher order functions)
// Returns true if the call is valid, false if an error was recorded
func (p *Parser) validateFunctionCall(funcName string, line int, column int) bool {
	if !p.LintMode {
		return true
	}

	// Check for recursion: function calling itself
	if p.currentFunctionName != "" && funcName == p.currentFunctionName {
		p.Errors = append(p.Errors, ParseError{
			Message: fmt.Sprintf("Recursion not allowed: function '%s' cannot call itself; use loop till condition instead.", funcName),
			Line:    line,
			Column:  column,
		})
		return false
	}

	return true
}

// inferType infers the type from an AST node
// AllocPrefix marks the heap allocator the compiler emits for every struct:
// `ahoy_alloc_NodeVis()` returns a zeroed NodeVis* on the heap. A struct
// field that holds a pointer to another struct needs one, and the C side
// knows the size - Ahoy has no way to ask for it.
const AllocPrefix = "ahoy_alloc_"

// AllocStructName returns the struct a call allocates, or "" when the name is
// not an allocator call.
func AllocStructName(callName string) string {
	if !strings.HasPrefix(callName, AllocPrefix) {
		return ""
	}
	return strings.TrimPrefix(callName, AllocPrefix)
}

func (p *Parser) inferType(node *ASTNode) string {
	if node == nil {
		return "unknown"
	}

	switch node.Type {
	case NODE_NUMBER:
		// Check if it contains a decimal point
		if strings.Contains(node.Value, ".") {
			return "float"
		}
		return "int"
	case NODE_STRING, NODE_F_STRING:
		return "string"
	case NODE_CHAR:
		return "char"
	case NODE_BOOLEAN:
		return "bool"
	case NODE_ARRAY_LITERAL:
		// Check if the array literal has an inferred element type
		if node.DataType != "" && node.DataType != "array" {
			return node.DataType
		}
		return "array"
	case NODE_DICT_LITERAL:
		return "dict"
	case NODE_OBJECT_LITERAL:
		// Check if it has a type name (struct initialization)
		if node.Value != "" {
			return "struct:" + node.Value
		}
		return "object"
	case NODE_IDENTIFIER:
		// Look up the variable's type - check loop scopes first, then function scope, then global
		for i := len(p.loopVarScopes) - 1; i >= 0; i-- {
			if varType, ok := p.loopVarScopes[i][node.Value]; ok {
				return varType
			}
		}
		if p.functionScope != nil {
			if varType, ok := p.functionScope[node.Value]; ok {
				return varType
			}
		}
		if varType, ok := p.variableTypes[node.Value]; ok {
			return varType
		}
		return "unknown"
	case NODE_IS_LIST:
		// Membership list sugar always produces a bool.
		return "bool"
	case NODE_MEMBER_ACCESS:
		// Handle struct field access: obj.field
		if len(node.Children) < 1 {
			return "unknown"
		}

		// Get the type of the object being accessed
		objectNode := node.Children[0]
		objectType := p.inferType(objectNode)

		// Remove "struct:" prefix if present
		objectType = strings.TrimPrefix(objectType, "struct:")

		// Look up the struct definition
		structDef, exists := p.structs[objectType]
		if !exists {
			return "unknown"
		}

		// Get the field name from the node value
		fieldName := node.Value

		// Look up the field type in the struct definition
		for _, field := range structDef.Fields {
			if field.Name == fieldName {
				return field.Type
			}
		}

		return "unknown"
	case NODE_ARRAY_ACCESS:
		// An element takes its array's element type: array[Node] -> Node.
		// (Single-level strip handles nesting: array[array[int]] -> array[int].)
		if node.Value == "" {
			return "unknown"
		}
		arrayType := ""
		for i := len(p.loopVarScopes) - 1; i >= 0; i-- {
			if varType, ok := p.loopVarScopes[i][node.Value]; ok {
				arrayType = varType
				break
			}
		}
		if arrayType == "" && p.functionScope != nil {
			if varType, ok := p.functionScope[node.Value]; ok {
				arrayType = varType
			}
		}
		if arrayType == "" {
			if varType, ok := p.variableTypes[node.Value]; ok {
				arrayType = varType
			}
		}
		if strings.HasPrefix(arrayType, "array[") && strings.HasSuffix(arrayType, "]") {
			return strings.TrimSuffix(strings.TrimPrefix(arrayType, "array["), "]")
		}
		if strings.HasPrefix(arrayType, "buffer[") && strings.HasSuffix(arrayType, "]") {
			return "float"
		}
		return "unknown"
	case NODE_CALL:
		// Handle function calls - infer return type from function signature
		funcName := node.Value

		// A distinct type name called with one argument is an explicit
		// conversion: the result has the distinct type.
		if _, isDistinct := p.distinctTypes[funcName]; isDistinct {
			return funcName
		}

		// A struct allocator: ahoy_alloc_NodeVis() -> NodeVis*
		if structName := AllocStructName(funcName); structName != "" {
			if _, ok := p.structs[structName]; ok {
				return structName + "*"
			}
		}

		// First check C headers (global namespace)
		if p.cHeaderGlobal != nil {
			if cFunc, exists := p.cHeaderGlobal.Functions[funcName]; exists {
				return cFunc.ReturnType
			}
		}

		// Check C headers (namespaced)
		for _, cHeader := range p.cHeaders {
			if cFunc, exists := cHeader.Functions[funcName]; exists {
				return cFunc.ReturnType
			}
		}

		// Check Ahoy function signatures
		if funcSig, exists := p.functions[funcName]; exists {
			if len(funcSig.ReturnTypes) > 0 {
				returnType := funcSig.ReturnTypes[0]

				// If the return type is "infer", try to infer it from the function body
				if returnType == "infer" && funcSig.FunctionNode != nil {
					// Infer the actual return types
					inferredTypes := p.inferReturnTypesFromFunction(funcSig, []string{})
					if len(inferredTypes) > 0 {
						return inferredTypes[0]
					}
				}

				return returnType
			}
		}

		return "unknown"
	default:
		// For expressions, we could recursively infer but for now return unknown
		return "unknown"
	}
}

// checkTypeCompatibility checks if a value type is compatible with expected type
// isLiteralNode reports whether n is a literal value (number, string,
// boolean, char) as opposed to a computed expression. F-strings do not
// count: their interpolations are runtime values.
func isLiteralNode(n *ASTNode) bool {
	if n == nil {
		return false
	}
	switch n.Type {
	case NODE_NUMBER, NODE_STRING, NODE_RAW_STRING, NODE_BOOLEAN, NODE_CHAR:
		return true
	}
	return false
}

func (p *Parser) checkTypeCompatibility(expectedType, actualType string, actualNode ...*ASTNode) bool {
	if expectedType == "unknown" || actualType == "unknown" {
		return true // Can't check unknown types
	}

	// Generic/any types are compatible with anything
	if expectedType == "generic" || actualType == "generic" ||
		expectedType == "any" || actualType == "any" {
		return true
	}

	// Distinct newtypes are exact: only the same distinct name flows in,
	// except a literal of the base kind (so `x: NodeId = 5` works but
	// `x: NodeId = someInt` does not). Anything else needs an explicit
	// `Name(value)` conversion.
	if base, isDistinct := p.distinctTypes[expectedType]; isDistinct {
		if actualType == expectedType {
			return true
		}
		if len(actualNode) > 0 && isLiteralNode(actualNode[0]) && p.checkTypeCompatibility(base, actualType) {
			return true
		}
		return false
	}
	// Mirror: a distinct value never flows implicitly into anything else,
	// not even its own base type. That is the whole point of distinct.
	if _, actualIsDistinct := p.distinctTypes[actualType]; actualIsDistinct {
		return actualType == expectedType
	}

	// Check if expectedType is a union type
	if unionTypes, isUnion := p.unionTypes[expectedType]; isUnion {
		// Check if actualType matches any of the union's types
		for _, unionType := range unionTypes {
			if p.checkTypeCompatibility(unionType, actualType) {
				return true
			}
		}
		return false
	}

	// Check if expectedType is a type alias - resolve it
	if aliasedType, isAlias := p.typeAliases[expectedType]; isAlias {
		return p.checkTypeCompatibility(aliasedType, actualType)
	}

	// A C function that returns char* hands back the same thing a string is:
	// glue headers (bell/engine/glue.h) return char* paths and names, and
	// assigning one to a string variable is not a type error.
	if (expectedType == "string" && (actualType == "char*" || actualType == "const char*")) ||
		(actualType == "string" && (expectedType == "char*" || expectedType == "const char*")) {
		return true
	}

	// Allow int to float conversion
	if expectedType == "float" && actualType == "int" {
		return true
	}

	// Allow string for char* (C string pointers)
	if (expectedType == "char *" || expectedType == "char*" || expectedType == "const char *" || expectedType == "const char*") && actualType == "string" {
		return true
	}

	// Check struct type compatibility
	// Both "struct:typename" and "typename" should match
	if strings.HasPrefix(expectedType, "struct:") || strings.HasPrefix(actualType, "struct:") {
		expectedBase := strings.TrimPrefix(expectedType, "struct:")
		actualBase := strings.TrimPrefix(actualType, "struct:")
		return expectedBase == actualBase
	}

	return expectedType == actualType
}

// splitCollectionType splits a collection type annotation into its base kind and
// its argument list. "array[int]" -> ("array", "int"),
// "dict<string,int>" -> ("dict", "string,int"), "array" -> ("array", "").
func splitCollectionType(typeName string) (string, string) {
	open := strings.IndexAny(typeName, "[<")
	if open < 0 {
		return typeName, ""
	}
	last := typeName[len(typeName)-1]
	if last != ']' && last != '>' {
		return typeName, ""
	}
	return typeName[:open], typeName[open+1 : len(typeName)-1]
}

// splitTypeArgs splits a collection argument list on top-level commas, so
// "string,int" -> ["string", "int"] and "string,array[int]" ->
// ["string", "array[int]"].
func splitTypeArgs(args string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case '[', '<':
			depth++
		case ']', '>':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(args[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(args[start:]))
	return parts
}

// collectionTypeMatches reports whether a value of type actualType can be
// assigned to a collection annotated as expectedType. The base kinds must
// agree, and element/key/value types are compared when the value carries them.
// A literal with no element information (empty or mixed, e.g. "array") is
// accepted: the annotation is what pins its element type down.
func (p *Parser) collectionTypeMatches(expectedType, actualType string) bool {
	if actualType == "unknown" {
		return true
	}

	expectedBase, expectedArgs := splitCollectionType(expectedType)
	actualBase, actualArgs := splitCollectionType(actualType)

	// dict[...] and dict<...> are the same base kind.
	if expectedBase != actualBase {
		return false
	}

	// The literal carries no element type - the annotation supplies it.
	if actualArgs == "" {
		return true
	}

	expectedParts := splitTypeArgs(expectedArgs)
	actualParts := splitTypeArgs(actualArgs)
	if len(expectedParts) != len(actualParts) {
		return false
	}
	for i := range expectedParts {
		if !p.checkTypeCompatibility(expectedParts[i], actualParts[i]) {
			return false
		}
	}
	return true
}

// trackStatementArrayMutation records that a bare method-call statement such as
// `xs.push|1|` or `xs.pop||` changes an array's length. The length is then no
// longer known: a pop and a conditional or repeated push cannot be bounded
// statically, and pretending otherwise made every literal index read after a
// push look out of bounds. Bounds checking still covers arrays that are only
// ever written by their literal.
func (p *Parser) trackStatementArrayMutation(stmt *ASTNode) {
	if stmt == nil || stmt.Type != NODE_METHOD_CALL || len(stmt.Children) == 0 {
		return
	}
	object := stmt.Children[0]
	if object == nil || object.Type != NODE_IDENTIFIER {
		return
	}

	switch stmt.Value {
	case "push", "pop", "append", "prepend", "remove", "insert", "clear":
		p.arrayLengths[object.Value] = ArrayInfo{IsKnown: false}
	}
}

// trackArrayMethodLength tracks array length after method calls
func (p *Parser) trackArrayMethodLength(varName string, methodCall *ASTNode) {
	if len(methodCall.Children) == 0 {
		return
	}

	object := methodCall.Children[0]
	methodName := methodCall.Value

	// Get the source array's length
	var sourceLength ArrayInfo
	if object.Type == NODE_IDENTIFIER {
		if info, ok := p.arrayLengths[object.Value]; ok {
			sourceLength = info
		} else {
			sourceLength = ArrayInfo{IsKnown: false}
		}
	} else if object.Type == NODE_ARRAY_LITERAL {
		sourceLength = ArrayInfo{
			Length:  len(object.Children),
			IsKnown: true,
		}
	} else {
		sourceLength = ArrayInfo{IsKnown: false}
	}

	// Track length based on method
	switch methodName {
	case "push":
		if sourceLength.IsKnown {
			p.arrayLengths[varName] = ArrayInfo{
				Length:  sourceLength.Length + 1,
				IsKnown: true,
			}
		}
	case "pop":
		if sourceLength.IsKnown && sourceLength.Length > 0 {
			p.arrayLengths[varName] = ArrayInfo{
				Length:  sourceLength.Length - 1,
				IsKnown: true,
			}
		}
	case "map", "sort", "reverse", "shuffle":
		// These preserve length
		p.arrayLengths[varName] = sourceLength
	case "filter":
		// Filter result length is unknown
		p.arrayLengths[varName] = ArrayInfo{IsKnown: false}
	default:
		// Unknown method - can't track length
		p.arrayLengths[varName] = ArrayInfo{IsKnown: false}
	}
}

// validateArrayAccess checks if array access is within bounds
func (p *Parser) validateArrayAccess(arrayNode *ASTNode, indexNode *ASTNode, line int) {
	if !p.LintMode {
		return
	}

	// Only validate if array is an identifier
	if arrayNode.Type != NODE_IDENTIFIER {
		return
	}

	arrayName := arrayNode.Value
	arrayInfo, exists := p.arrayLengths[arrayName]

	if !exists || !arrayInfo.IsKnown {
		return // Can't validate unknown array length
	}

	// Parse the index - handle both literal numbers and unary minus
	var index int
	var err error

	if indexNode.Type == NODE_NUMBER {
		index, err = strconv.Atoi(indexNode.Value)
		if err != nil {
			return
		}
	} else if indexNode.Type == NODE_UNARY_OP && indexNode.Value == "-" && len(indexNode.Children) > 0 {
		// Handle negative numbers like -4
		if indexNode.Children[0].Type == NODE_NUMBER {
			val, err := strconv.Atoi(indexNode.Children[0].Value)
			if err != nil {
				return
			}
			index = -val
		} else {
			return // Not a literal negative number
		}
	} else {
		return // Can't validate non-literal index
	}

	length := arrayInfo.Length

	// Validate bounds
	if index >= 0 {
		// Positive index
		if index >= length {
			errMsg := fmt.Sprintf("Array index out of bounds: accessing index %d of array '%s' with length %d",
				index, arrayName, length)
			// Add error directly to preserve correct line number
			p.Errors = append(p.Errors, ParseError{
				Message: errMsg,
				Line:    line,
				Column:  0,
			})
		}
	} else {
		// Negative index (Python-style)
		if index < -length {
			errMsg := fmt.Sprintf("Array index out of bounds: accessing index %d of array '%s' with length %d (valid range: -%d to -1)",
				index, arrayName, length, length)
			// Add error directly to preserve correct line number
			p.Errors = append(p.Errors, ParseError{
				Message: errMsg,
				Line:    line,
				Column:  0,
			})
		}
	}
}

// Helper function to get readable token name
func tokenTypeName(t TokenType) string {
	names := map[TokenType]string{
		TOKEN_EOF: "EOF", TOKEN_IDENTIFIER: "identifier", TOKEN_NUMBER: "number",
		TOKEN_STRING: "string", TOKEN_CHAR: "char", TOKEN_F_STRING: "f-string",
		TOKEN_ASSIGN: "':'", TOKEN_IS: "'is'", TOKEN_NOT: "'not'",
		TOKEN_OR: "'or'", TOKEN_AND: "'and'", TOKEN_THEN: "'then'",
		TOKEN_ON: "'on'", TOKEN_IF: "'if'", TOKEN_ELSE: "'else'",
		TOKEN_ELSEIF: "'elseif'", TOKEN_ANIF: "'anif'", TOKEN_SWITCH: "'switch'",
		TOKEN_LOOP: "'loop'", TOKEN_IN: "'in'", TOKEN_TO: "'to'",
		TOKEN_TILL: "'till'", TOKEN_WHILE: "'while'", TOKEN_UNTIL: "'until'", TOKEN_FUNC: "'func'",
		TOKEN_RETURN: "'return'", TOKEN_IMPORT: "'import'", TOKEN_PROGRAM: "'program'", TOKEN_WHEN: "'when'",
		TOKEN_AHOY: "'ahoy'", TOKEN_PRINT: "'print'", TOKEN_LOG: "'log'", TOKEN_PANIC: "'panic'", TOKEN_PLUS: "'+'",
		TOKEN_MINUS: "'-'", TOKEN_MULTIPLY: "'*'", TOKEN_DIVIDE: "'/'",
		TOKEN_MODULO: "'%'", TOKEN_PLUS_WORD: "'plus'", TOKEN_MINUS_WORD: "'minus'",
		TOKEN_TIMES_WORD: "'times'", TOKEN_DIV_WORD: "'div'", TOKEN_MOD_WORD: "'mod'",
		TOKEN_BAND: "'band'", TOKEN_BOR: "'bor'", TOKEN_BXOR: "'bxor'",
		TOKEN_BNOT: "'bnot'", TOKEN_SHL: "'shl'", TOKEN_SHR: "'shr'",
		TOKEN_LESS: "'<'", TOKEN_GREATER: "'>'", TOKEN_LESS_EQUAL: "'<='",
		TOKEN_GREATER_EQUAL: "'>='", TOKEN_LESSER_WORD: "'lesser'", TOKEN_GREATER_WORD: "'greater'",
		TOKEN_PIPE: "'|'", TOKEN_LPAREN: "'('", TOKEN_RPAREN: "')'",
		TOKEN_LBRACE: "'{'", TOKEN_RBRACE: "'}'",
		TOKEN_LBRACKET: "'['", TOKEN_RBRACKET: "']'", TOKEN_LANGLE: "'<'",
		TOKEN_RANGLE: "'>'", TOKEN_COMMA: "','", TOKEN_DOT: "'.'",
		TOKEN_SEMICOLON: "';'", TOKEN_NEWLINE: "newline", TOKEN_INDENT: "indent",
		TOKEN_DEDENT: "dedent", TOKEN_INT_TYPE: "type 'int'", TOKEN_FLOAT_TYPE: "type 'float'",
		TOKEN_STRING_TYPE: "type 'string'", TOKEN_BOOL_TYPE: "type 'bool'",
		TOKEN_DICT_TYPE: "type 'dict'", TOKEN_ARRAY_TYPE: "type 'array'",
		TOKEN_TRUE: "'true'", TOKEN_FALSE: "'false'",
		TOKEN_ENUM: "'enum'", TOKEN_STRUCT: "'struct'", TOKEN_TYPE: "'type'",
		TOKEN_DO: "'do'", TOKEN_HALT: "'halt'", TOKEN_NEXT: "'next'",
		TOKEN_ASSERT: "'assert'", TOKEN_DEFER: "'defer'",
		TOKEN_DOUBLE_COLON: "'::'", TOKEN_WALRUS: "':='", TOKEN_QUESTION: "'?'", TOKEN_TERNARY: "'??'",
		TOKEN_EQUALS: "'='", TOKEN_INFER: "'infer'", TOKEN_VOID: "'void'",
		TOKEN_AT: "'@'", TOKEN_END: "'$'",
		TOKEN_PLUS_ASSIGN: "'+='", TOKEN_MINUS_ASSIGN: "'-='",
		TOKEN_MULTIPLY_ASSIGN: "'*='", TOKEN_DIVIDE_ASSIGN: "'/='", TOKEN_MODULO_ASSIGN: "'%='",
	}
	if name, ok := names[t]; ok {
		return name
	}
	return fmt.Sprintf("token(%d)", t)
}

func (p *Parser) advance() {
	if p.pos < len(p.tokens) {
		p.pos++
	}
}

// isCompoundAssignOp checks if a token type is a compound assignment operator
func (p *Parser) isCompoundAssignOp(tokenType TokenType) bool {
	return tokenType == TOKEN_PLUS_ASSIGN ||
		tokenType == TOKEN_MINUS_ASSIGN ||
		tokenType == TOKEN_MULTIPLY_ASSIGN ||
		tokenType == TOKEN_DIVIDE_ASSIGN ||
		tokenType == TOKEN_MODULO_ASSIGN
}

// getCompoundAssignOp returns the binary operator for a compound assignment operator
func (p *Parser) getCompoundAssignOp(tokenType TokenType) string {
	switch tokenType {
	case TOKEN_PLUS_ASSIGN:
		return "+"
	case TOKEN_MINUS_ASSIGN:
		return "-"
	case TOKEN_MULTIPLY_ASSIGN:
		return "*"
	case TOKEN_DIVIDE_ASSIGN:
		return "/"
	case TOKEN_MODULO_ASSIGN:
		return "%"
	default:
		return ""
	}
}

// copyASTNode creates a deep copy of an AST node
func (p *Parser) copyASTNode(node *ASTNode) *ASTNode {
	if node == nil {
		return nil
	}

	// Copy children recursively
	children := make([]*ASTNode, len(node.Children))
	for i, child := range node.Children {
		children[i] = p.copyASTNode(child)
	}

	// Copy default value if present
	var defaultValue *ASTNode
	if node.DefaultValue != nil {
		defaultValue = p.copyASTNode(node.DefaultValue)
	}

	return &ASTNode{
		Type:         node.Type,
		Value:        node.Value,
		Children:     children,
		DataType:     node.DataType,
		Line:         node.Line,
		DefaultValue: defaultValue,
		EnumType:     node.EnumType,
		IsMutable:    node.IsMutable,
		IsStatic:     node.IsStatic,
		IsConst:      node.IsConst,
	}
}

// Skip optional newlines and indents
func (p *Parser) skipWhitespace() {
	for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT {
		p.advance()
	}
}

func (p *Parser) expect(tokenType TokenType) Token {
	if p.current().Type != tokenType {
		current := p.current()
		errMsg := fmt.Sprintf("Expected %s, got %s at line %d:%d",
			tokenTypeName(tokenType),
			tokenTypeName(current.Type),
			current.Line,
			current.Column)
		if p.LintMode {
			p.recordError(errMsg)
			// In lint mode, return current token and advance to continue parsing
			token := p.current()
			p.advance()
			return token
		} else {
			panic(errMsg)
		}
	}
	token := p.current()
	p.advance()
	return token
}

func (p *Parser) parseProgram() *ASTNode {
	program := &ASTNode{Type: NODE_PROGRAM}

	// First pass: collect all C header imports and parse them concurrently
	p.parseCHeadersParallel()

	for p.current().Type != TOKEN_EOF {
		if p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_SEMICOLON || p.current().Type == TOKEN_DEDENT {
			p.advance()
			continue
		}

		// Save position to detect if we're stuck
		oldPos := p.pos

		stmt := p.parseStatement()
		if stmt != nil {
			program.Children = append(program.Children, stmt)

			// Track if we've seen non-import statements
			if stmt.Type != NODE_IMPORT_STATEMENT && stmt.Type != NODE_PROGRAM_DECLARATION {
				p.seenNonImport = true
			}
		}

		// After a statement, accept either newline or semicolon
		if p.current().Type == TOKEN_SEMICOLON {
			p.advance()
			// Continue to parse next statement on same line
		}

		// Safety check: if position hasn't advanced, force advance to prevent infinite loop
		if p.pos == oldPos && p.current().Type != TOKEN_EOF {
			// We're stuck - skip this token to avoid infinite loop
			p.advance()
		}
	}

	return program
}
