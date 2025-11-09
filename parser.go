package ahoy

import (
	"fmt"
	"strconv"
	"strings"
)

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
	NODE_CALL
	NODE_IDENTIFIER
	NODE_NUMBER
	NODE_STRING
	NODE_F_STRING // f-string with interpolation
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
	NODE_METHOD_CALL
	NODE_MEMBER_ACCESS
	NODE_HALT
	NODE_NEXT
	NODE_LAMBDA
	NODE_TERNARY
	NODE_ASSERT_STATEMENT
	NODE_DEFER_STATEMENT
	NODE_OBJECT_LITERAL
	NODE_OBJECT_PROPERTY
	NODE_OBJECT_ACCESS
)

type ASTNode struct {
	Type         NodeType
	Value        string
	Children     []*ASTNode
	DataType     string
	Line         int
	DefaultValue *ASTNode // For default parameter values
}

type ParseError struct {
	Message string
	Line    int
	Column  int
}

type StructField struct {
	Name         string
	Type         string
	DefaultValue *ASTNode
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
	Type string // "generic" if no type specified
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
	Line       int // Line number in header file
}

type CParameter struct {
	Name string
	Type string
}

type CEnum struct {
	Name   string
	Values map[string]int
	Line   int // Line number in header file
}

type CDefine struct {
	Name  string
	Value string
	Line  int // Line number in header file
}

type CStructField struct {
	Name string
	Type string
}

type CStruct struct {
	Name   string
	Fields []CStructField
	Line   int // Line number in header file
}

type CHeaderInfo struct {
	Functions map[string]*CFunction
	Enums     map[string]*CEnum
	Defines   map[string]*CDefine
	Structs   map[string]*CStruct
}

type Parser struct {
	tokens             []Token
	pos                int
	inFunctionCall     bool
	inArrayLiteral     bool
	inObjectLiteral    bool
	inDictLiteral      bool
	LintMode           bool
	Errors             []ParseError
	variableTypes      map[string]string             // Track variable types
	constants          map[string]int                // Track constant declarations (name -> line number)
	structs            map[string]*StructDefinition  // Track struct definitions
	enums              map[string]*EnumDefinition    // Track enum definitions
	objectLiterals     map[string]map[string]bool    // Track object literal properties by variable name
	currentFunctionRet string                        // Track current function return type
	functionScope      map[string]string             // Track function-local variables
	seenNonImport      bool                          // Track if we've seen non-import statements
	functions          map[string]*FunctionSignature // Track function signatures
	arrayLengths       map[string]ArrayInfo          // Track array lengths
	cHeaders           map[string]*CHeaderInfo       // Track imported C headers (namespace -> header info)
	cHeaderGlobal      *CHeaderInfo                  // Global C header imports (no namespace)
	blockDepth         int                           // Track nesting depth of multi-line blocks
	loopVarScopes      []map[string]string           // Stack of loop variable scopes
}

func Parse(tokens []Token) *ASTNode {
	parser := &Parser{
		tokens:             tokens,
		pos:                0,
		LintMode:           false,
		Errors:             []ParseError{},
		variableTypes:      make(map[string]string),
		constants:          make(map[string]int),
		structs:            make(map[string]*StructDefinition),
		enums:              make(map[string]*EnumDefinition),
		objectLiterals:     make(map[string]map[string]bool),
		currentFunctionRet: "",
		functionScope:      make(map[string]string),
		functions:          make(map[string]*FunctionSignature),
		arrayLengths:       make(map[string]ArrayInfo),
		cHeaders:           make(map[string]*CHeaderInfo),
		cHeaderGlobal:      &CHeaderInfo{Functions: make(map[string]*CFunction), Enums: make(map[string]*CEnum), Defines: make(map[string]*CDefine), Structs: make(map[string]*CStruct)},
		blockDepth:         0,
		loopVarScopes:      make([]map[string]string, 0),
	}
	return parser.parseProgram()
}

func ParseLint(tokens []Token) (*ASTNode, []ParseError) {
	parser := &Parser{
		tokens:             tokens,
		pos:                0,
		LintMode:           true,
		Errors:             []ParseError{},
		variableTypes:      make(map[string]string),
		constants:          make(map[string]int),
		structs:            make(map[string]*StructDefinition),
		enums:              make(map[string]*EnumDefinition),
		objectLiterals:     make(map[string]map[string]bool),
		currentFunctionRet: "",
		functionScope:      make(map[string]string),
		functions:          make(map[string]*FunctionSignature),
		arrayLengths:       make(map[string]ArrayInfo),
		cHeaders:           make(map[string]*CHeaderInfo),
		cHeaderGlobal:      &CHeaderInfo{Functions: make(map[string]*CFunction), Enums: make(map[string]*CEnum), Defines: make(map[string]*CDefine), Structs: make(map[string]*CStruct)},
		blockDepth:         0,
		loopVarScopes:      make([]map[string]string, 0),
	}
	ast := parser.parseProgram()
	return ast, parser.Errors
}

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

// recordErrorAtLine records an error at a specific line number
func (p *Parser) recordErrorAtLine(message string, line int) {
	p.Errors = append(p.Errors, ParseError{
		Message: message,
		Line:    line,
		Column:  1, // Default to column 1 for block-level errors
	})
}

// inferType infers the type from an AST node
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
	default:
		// For expressions, we could recursively infer but for now return unknown
		return "unknown"
	}
}

// checkTypeCompatibility checks if a value type is compatible with expected type
func (p *Parser) checkTypeCompatibility(expectedType, actualType string) bool {
	if expectedType == "unknown" || actualType == "unknown" {
		return true // Can't check unknown types
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
		TOKEN_TILL: "'till'", TOKEN_FUNC: "'func'",
		TOKEN_RETURN: "'return'", TOKEN_IMPORT: "'import'", TOKEN_PROGRAM: "'program'", TOKEN_WHEN: "'when'",
		TOKEN_AHOY: "'ahoy'", TOKEN_PRINT: "'print'", TOKEN_PLUS: "'+'",
		TOKEN_MINUS: "'-'", TOKEN_MULTIPLY: "'*'", TOKEN_DIVIDE: "'/'",
		TOKEN_MODULO: "'%'", TOKEN_PLUS_WORD: "'plus'", TOKEN_MINUS_WORD: "'minus'",
		TOKEN_TIMES_WORD: "'times'", TOKEN_DIV_WORD: "'div'", TOKEN_MOD_WORD: "'mod'",
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
		TOKEN_VECTOR2_TYPE: "type 'vector2'", TOKEN_COLOR_TYPE: "type 'color'",
		TOKEN_TRUE: "'true'", TOKEN_FALSE: "'false'",
		TOKEN_ENUM: "'enum'", TOKEN_STRUCT: "'struct'", TOKEN_TYPE: "'type'",
		TOKEN_DO: "'do'", TOKEN_HALT: "'halt'", TOKEN_NEXT: "'next'",
		TOKEN_ASSERT: "'assert'", TOKEN_DEFER: "'defer'",
		TOKEN_DOUBLE_COLON: "'::'", TOKEN_QUESTION: "'?'", TOKEN_TERNARY: "'??'",
		TOKEN_EQUALS: "'='", TOKEN_INFER: "'infer'", TOKEN_VOID: "'void'",
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

func (p *Parser) parseStatement() *ASTNode {
	switch p.current().Type {
	case TOKEN_PROGRAM:
		return p.parseProgramDeclaration()
	case TOKEN_ENUM:
		return p.parseEnumDeclaration()
	case TOKEN_STRUCT:
		return p.parseStructDeclaration()
	case TOKEN_FUNC:
		return p.parseFunction()
	case TOKEN_IF:
		return p.parseIfStatement()
	case TOKEN_SWITCH:
		return p.parseSwitchStatement()
	case TOKEN_LOOP:
		return p.parseLoop()
	case TOKEN_WHEN:
		return p.parseWhenStatement()
	case TOKEN_AHOY:
		return p.parseAhoyStatement()

	case TOKEN_PRINT:
		return p.parsePrintStatement()
	case TOKEN_RETURN:
		return p.parseReturnStatement()
	case TOKEN_HALT:
		p.advance()
		return &ASTNode{Type: NODE_HALT, Line: p.current().Line}
	case TOKEN_NEXT:
		p.advance()
		return &ASTNode{Type: NODE_NEXT, Line: p.current().Line}
	case TOKEN_END:
		// Handle $ block terminator
		token := p.current()
		p.advance() // Consume the TOKEN_END
		
		// Check for $#N syntax
		if strings.HasPrefix(token.Value, "$#") {
			countStr := strings.TrimPrefix(token.Value, "$#")
			count, err := strconv.Atoi(countStr)
			if err != nil || count <= 0 {
				if p.LintMode {
					p.Errors = append(p.Errors, ParseError{
						Message: fmt.Sprintf("Invalid $# syntax: %s", token.Value),
						Line:    token.Line,
						Column:  token.Column,
					})
				}
			} else if count > p.blockDepth {
				if p.LintMode {
					p.Errors = append(p.Errors, ParseError{
						Message: fmt.Sprintf("Cannot close %d blocks, only %d block(s) open", count, p.blockDepth),
						Line:    token.Line,
						Column:  token.Column,
					})
				}
			} else {
				// Decrease block depth by count
				p.blockDepth -= count
			}
		} else {
			// Regular $ terminator
			if p.blockDepth <= 0 {
				if p.LintMode {
					p.Errors = append(p.Errors, ParseError{
						Message: "Superfluous $ - no block to close",
						Line:    token.Line,
						Column:  token.Column,
					})
				}
			} else {
				p.blockDepth--
				// Pop loop variable scope if we're closing a loop block
				if len(p.loopVarScopes) > p.blockDepth {
					p.loopVarScopes = p.loopVarScopes[:len(p.loopVarScopes)-1]
				}
			}
		}
		return nil
	case TOKEN_ASSERT:
		return p.parseAssertStatement()
	case TOKEN_DEFER:
		return p.parseDeferStatement()
	case TOKEN_IMPORT:
		return p.parseImportStatement()
	case TOKEN_IDENTIFIER:
		// Check for constant declaration (name ::)
		nextType := p.peek(1).Type
		if nextType == TOKEN_DOUBLE_COLON {
			return p.parseConstantDeclaration()
		}
		// Check for tuple assignment (name, name :)
		if nextType == TOKEN_COMMA {
			return p.parseTupleAssignment()
		}
		return p.parseAssignmentOrExpression()
	case TOKEN_COLOR_TYPE, TOKEN_VECTOR2_TYPE:
		return p.parseExpression()
	case TOKEN_NEWLINE, TOKEN_SEMICOLON:
		p.advance()
		return nil
	default:
		return p.parseExpression()
	}
}

func (p *Parser) parseFunction() *ASTNode {
	p.expect(TOKEN_FUNC)
	name := p.expect(TOKEN_IDENTIFIER)

	fn := &ASTNode{
		Type:  NODE_FUNCTION,
		Value: name.Value,
		Line:  name.Line,
	}

	// Check for pipe-based syntax |params| or space-separated params
	params := &ASTNode{Type: NODE_BLOCK}

	if p.current().Type == TOKEN_PIPE {
		// Old syntax: func name |param1 type1, param2 type2| then
		p.advance()

		for p.current().Type != TOKEN_PIPE && p.current().Type != TOKEN_EOF {
			paramName := p.expect(TOKEN_IDENTIFIER)
			var paramType string

			// Check for type annotation
			if p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
				p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_BOOL_TYPE ||
				p.current().Type == TOKEN_COLOR_TYPE || p.current().Type == TOKEN_VECTOR2_TYPE {
				paramType = p.current().Value
				p.advance()
			}

			param := &ASTNode{
				Type:     NODE_IDENTIFIER,
				Value:    paramName.Value,
				DataType: paramType,
			}
			params.Children = append(params.Children, param)

			if p.current().Type == TOKEN_COMMA {
				p.advance()
			}
		}
		p.expect(TOKEN_PIPE)
	} else {
		// New syntax: func name param1 type1 param2 type2 do
		for p.current().Type == TOKEN_IDENTIFIER {
			paramName := p.current()
			p.advance()

			var paramType string
			// Check for type annotation
			if p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
				p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_BOOL_TYPE ||
				p.current().Type == TOKEN_COLOR_TYPE || p.current().Type == TOKEN_VECTOR2_TYPE ||
				p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
				p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
				p.current().Type == TOKEN_IDENTIFIER {
				paramType = p.current().Value
				p.advance()
			}

			param := &ASTNode{
				Type:     NODE_IDENTIFIER,
				Value:    paramName.Value,
				DataType: paramType,
			}
			params.Children = append(params.Children, param)
		}
	}

	// Return type (optional, using -> syntax)
	var returnType string
	if p.current().Type == TOKEN_MINUS {
		// Check for -> (minus followed by greater)
		if p.peek(1).Type == TOKEN_GREATER {
			p.advance() // skip -
			p.advance() // skip >
			if p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
				p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_BOOL_TYPE ||
				p.current().Type == TOKEN_COLOR_TYPE || p.current().Type == TOKEN_VECTOR2_TYPE ||
				p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
				p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
				p.current().Type == TOKEN_IDENTIFIER {
				returnType = p.current().Value
				p.advance()
			}
		}
	}

	// Accept either 'then' or 'do'
	if p.current().Type == TOKEN_THEN {
		p.advance()
	} else if p.current().Type == TOKEN_DO {
		p.advance()
	} else {
		current := p.current()
		errMsg := fmt.Sprintf("Expected 'then' or 'do' in function, got %s at line %d:%d",
			tokenTypeName(current.Type), current.Line, current.Column)
		if p.LintMode {
			p.recordError(errMsg)
		} else {
			panic(errMsg)
		}
	}

	// Skip optional whitespace/indent after keyword
	p.skipWhitespace()

	// Parse body
	var body *ASTNode
	if p.current().Type == TOKEN_NEWLINE {
		p.advance()
		if p.current().Type == TOKEN_INDENT {
			p.advance()
		}
		body = p.parseBlock()

		// Consume 'end' keyword for multi-line functions
		if p.current().Type == TOKEN_END {
			p.advance()
		} else {
			errMsg := fmt.Sprintf("Expected '$' to close function at line %d", p.current().Line)
			if p.LintMode {
				p.recordError(errMsg)
			} else {
				panic(errMsg)
			}
		}
	} else {
		// Inline function body
		body = &ASTNode{Type: NODE_BLOCK}
		stmt := p.parseStatement()
		if stmt != nil {
			body.Children = append(body.Children, stmt)
		}
	}

	fn.Children = append(fn.Children, params)
	fn.Children = append(fn.Children, body)
	fn.DataType = returnType

	return fn
}

func (p *Parser) parseIfStatement() *ASTNode {
	startLine := p.current().Line
	p.expect(TOKEN_IF)
	condition := p.parseExpression()

	// Accept 'then', 'do', or ':'
	usesColon := false
	if p.current().Type == TOKEN_THEN {
		p.advance()
	} else if p.current().Type == TOKEN_DO {
		p.advance()
	} else if p.current().Type == TOKEN_ASSIGN {
		usesColon = true
		p.advance()
	} else {
		current := p.current()
		errMsg := fmt.Sprintf("Expected 'then', 'do', or ':', got %s at line %d:%d",
			tokenTypeName(current.Type), current.Line, current.Column)
		if p.LintMode {
			p.recordError(errMsg)
		} else {
			panic(errMsg)
		}
	}

	// Check for inline if statement (no newline after then/do/colon)
	isMultiLine := false
	var ifBody *ASTNode
	if p.current().Type != TOKEN_NEWLINE {
		// Inline: parse single statement
		p.skipWhitespace()
		ifBody = &ASTNode{Type: NODE_BLOCK}
		stmt := p.parseStatement()
		if stmt != nil {
			ifBody.Children = append(ifBody.Children, stmt)
		}
	} else {
		// Multi-line: parse block
		isMultiLine = true
		p.advance() // skip newline
		p.skipWhitespace()
		if usesColon {
			p.blockDepth++ // Opening a multi-line block
			ifBody = p.parseBlockUntilEnd("if", startLine)
		} else {
			ifBody = p.parseBlock()
		}
	}

	ifStmt := &ASTNode{
		Type:     NODE_IF_STATEMENT,
		Children: []*ASTNode{condition, ifBody},
	}

	// Skip any newlines, semicolons, or dedents before checking for anif/elseif
	for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_DEDENT || p.current().Type == TOKEN_SEMICOLON {
		p.advance()
	}

	// Handle elseif/anif chains
	for p.current().Type == TOKEN_ELSEIF || p.current().Type == TOKEN_ANIF {
		p.advance()
		elseifCondition := p.parseExpression()

		// Accept 'then', 'do', or ':'
		elseifUsesColon := false
		if p.current().Type == TOKEN_THEN {
			p.advance()
		} else if p.current().Type == TOKEN_DO {
			p.advance()
		} else if p.current().Type == TOKEN_ASSIGN {
			elseifUsesColon = true
			p.advance()
		} else {
			current := p.current()
			errMsg := fmt.Sprintf("Expected 'then', 'do', or ':', got %s at line %d:%d",
				tokenTypeName(current.Type), current.Line, current.Column)
			if p.LintMode {
				p.recordError(errMsg)
			} else {
				panic(errMsg)
			}
		}

		// Check for inline elseif/anif
		var elseifBody *ASTNode
		if p.current().Type != TOKEN_NEWLINE {
			// Inline
			p.skipWhitespace()
			elseifBody = &ASTNode{Type: NODE_BLOCK}
			stmt := p.parseStatement()
			if stmt != nil {
				elseifBody.Children = append(elseifBody.Children, stmt)
			}
		} else {
			// Multi-line
			isMultiLine = true
			p.advance() // skip newline
			p.skipWhitespace()
			if elseifUsesColon {
				p.blockDepth++ // Opening a multi-line block
				elseifBody = p.parseBlockUntilEnd("anif", startLine)
			} else {
				elseifBody = p.parseBlock()
			}
		}

		// Add elseif as another condition-body pair
		ifStmt.Children = append(ifStmt.Children, elseifCondition, elseifBody)

		// Skip any newlines, semicolons, or dedents before checking for next anif/elseif
		for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_DEDENT || p.current().Type == TOKEN_SEMICOLON {
			p.advance()
		}
	}

	// Handle else (can optionally use ':')
	if p.current().Type == TOKEN_ELSE {
		p.advance()

		// Check if using ':' syntax
		elseUsesColon := false
		if p.current().Type == TOKEN_ASSIGN {
			elseUsesColon = true
			p.advance()
		}

		// Check for inline else
		var elseBody *ASTNode
		if p.current().Type != TOKEN_NEWLINE {
			// Inline
			p.skipWhitespace()
			elseBody = &ASTNode{Type: NODE_BLOCK}
			stmt := p.parseStatement()
			if stmt != nil {
				elseBody.Children = append(elseBody.Children, stmt)
			}
		} else {
			// Multi-line
			isMultiLine = true
			p.advance() // skip newline
			p.skipWhitespace()
			if elseUsesColon {
				p.blockDepth++ // Opening a multi-line block
				elseBody = p.parseBlockUntilEnd("else", startLine)
			} else {
				elseBody = p.parseBlock()
			}
		}
		ifStmt.Children = append(ifStmt.Children, elseBody)

		// Skip any newlines or dedents before 'end' (only if not using colon syntax)
		if !elseUsesColon {
			for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_DEDENT {
				p.advance()
			}
		}
	}

	// Multi-line if statements require 'end' (only if not using colon syntax which already consumed it)
	if isMultiLine && !usesColon {
		if p.current().Type == TOKEN_END {
			p.advance() // consume 'end'
		} else {
			errMsg := fmt.Sprintf("Expected '$' to close if statement at line %d", startLine)
			if p.LintMode {
				p.recordError(errMsg)
			} else {
				panic(errMsg)
			}
		}
	}

	return ifStmt
}

func (p *Parser) parseSwitchStatement() *ASTNode {
	startLine := p.current().Line
	p.expect(TOKEN_SWITCH)
	expr := p.parseExpression()

	// Accept either "then", "on", or ":" keyword
	if p.current().Type == TOKEN_ON {
		p.advance()
	} else if p.current().Type == TOKEN_THEN {
		p.advance()
	} else if p.current().Type == TOKEN_ASSIGN { // colon
		p.advance()
	} else {
		errMsg := fmt.Sprintf("Expected 'on', 'then', or ':' after switch expression at line %d", p.current().Line)
		if p.LintMode {
			p.recordError(errMsg)
		} else {
			panic(errMsg)
		}
	}

	// Check if this is a one-line switch (no newline after 'on'/'then')
	isOneLine := p.current().Type != TOKEN_NEWLINE && p.current().Type != TOKEN_INDENT

	// Expect and consume indent after switch keyword
	p.skipNewlines()
	if p.current().Type == TOKEN_INDENT {
		p.advance()
	}

	switchStmt := &ASTNode{
		Type:     NODE_SWITCH_STATEMENT,
		Children: []*ASTNode{expr}, // First child is the switch expression
	}

	// Parse cases: value:statement then value:statement then ...
	// Now supports: 'A','B','C':statement or 'a' to 'z':statement or 'A' or 'B':statement
	maxSwitchIterations := 10000 // Safety limit
	switchIterations := 0
	for {
		switchIterations++
		if switchIterations > maxSwitchIterations {
			errMsg := fmt.Sprintf("Parser safety limit reached while parsing switch cases at line %d - possible infinite loop", startLine)
			if p.LintMode {
				p.recordError(errMsg)
			} else {
				panic(errMsg)
			}
			break
		}

		// Skip newlines and semicolons
		for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_SEMICOLON {
			p.advance()
		}

		// Check for end of switch
		if p.current().Type == TOKEN_END || p.current().Type == TOKEN_DEDENT ||
			p.current().Type == TOKEN_EOF || p.current().Type == TOKEN_IF ||
			p.current().Type == TOKEN_LOOP || p.current().Type == TOKEN_FUNC ||
			p.current().Type == TOKEN_RETURN || p.current().Type == TOKEN_SWITCH {
			break
		}

		// Check if we have a case value (number, char, string, or underscore for default)
		if p.current().Type != TOKEN_NUMBER && p.current().Type != TOKEN_STRING &&
			p.current().Type != TOKEN_CHAR && p.current().Type != TOKEN_IDENTIFIER {
			break
		}

		// Parse case values - could be single, list (with commas), list (with 'or'), or range (with 'to')
		caseValues := []*ASTNode{}

		for {
			// Save position to detect stuck loop
			oldPos := p.pos

			// Parse single case value
			var caseValue *ASTNode
			if p.current().Type == TOKEN_NUMBER {
				tok := p.current()
				p.advance()
				caseValue = &ASTNode{
					Type:  NODE_NUMBER,
					Value: tok.Value,
				}
			} else if p.current().Type == TOKEN_CHAR {
				tok := p.current()
				p.advance()
				caseValue = &ASTNode{
					Type:  NODE_CHAR,
					Value: tok.Value,
				}
			} else if p.current().Type == TOKEN_STRING {
				tok := p.current()
				p.advance()
				caseValue = &ASTNode{
					Type:  NODE_STRING,
					Value: tok.Value,
				}
			} else if p.current().Type == TOKEN_IDENTIFIER {
				tok := p.current()
				p.advance()
				caseValue = &ASTNode{
					Type:  NODE_IDENTIFIER,
					Value: tok.Value,
				}
			} else {
				// Unexpected token - break out to avoid infinite loop
				break
			}

			caseValues = append(caseValues, caseValue)

			// Check for range ('to' keyword) or multiple values (',' or 'or')
			if p.current().Type == TOKEN_TO {
				// This is a range: 'a' to 'z'
				p.advance()
				var endValue *ASTNode
				if p.current().Type == TOKEN_NUMBER {
					tok := p.current()
					p.advance()
					endValue = &ASTNode{
						Type:  NODE_NUMBER,
						Value: tok.Value,
					}
				} else if p.current().Type == TOKEN_CHAR {
					tok := p.current()
					p.advance()
					endValue = &ASTNode{
						Type:  NODE_CHAR,
						Value: tok.Value,
					}
				} else {
					errMsg := fmt.Sprintf("Expected end value for range at line %d", p.current().Line)
					if p.LintMode {
						p.recordError(errMsg)
						// Create a dummy node to continue parsing
						endValue = &ASTNode{
							Type:  NODE_NUMBER,
							Value: "0",
						}
					} else {
						panic(errMsg)
					}
				}

				// Create range node
				rangeNode := &ASTNode{
					Type:     NODE_SWITCH_CASE_RANGE,
					Children: []*ASTNode{caseValue, endValue},
				}
				caseValues = []*ASTNode{rangeNode}
				break
			} else if p.current().Type == TOKEN_COMMA || p.current().Type == TOKEN_OR {
				// Multiple values: 'A','B','C' or 'A' or 'B'
				p.advance()
				continue
			} else {
				// Single value or end of value list
				break
			}

			// Safety check: if position hasn't advanced, break to avoid infinite loop
			if p.pos == oldPos {
				break
			}
		}

		p.expect(TOKEN_ASSIGN) // Expect :

		// Skip optional whitespace/indent after colon
		p.skipWhitespace()

		// Parse case body - must be a statement or call, not an assignment
		// We need to be careful here because parseStatement might misinterpret
		// patterns like "ahoy|...|" if preceded by an identifier
		var caseBody *ASTNode

		// Explicitly handle known statement types
		switch p.current().Type {
		case TOKEN_AHOY:
			caseBody = p.parseAhoyStatement()
		case TOKEN_PRINT:
			caseBody = p.parsePrintStatement()
		case TOKEN_RETURN:
			caseBody = p.parseReturnStatement()
		case TOKEN_IF:
			caseBody = p.parseIfStatement()
		case TOKEN_SWITCH:
			caseBody = p.parseSwitchStatement()
		case TOKEN_LOOP:
			caseBody = p.parseLoop()
		case TOKEN_IDENTIFIER:
			// Could be a function call or method call
			caseBody = p.parseExpression()
		default:
			// Try to parse as expression
			caseBody = p.parseExpression()
		}

		// Create case node
		var caseNode *ASTNode
		if len(caseValues) == 1 {
			caseNode = &ASTNode{
				Type:     NODE_SWITCH_CASE,
				Children: []*ASTNode{caseValues[0], caseBody},
			}
		} else {
			// Multiple case values
			listNode := &ASTNode{
				Type:     NODE_SWITCH_CASE_LIST,
				Children: caseValues,
			}
			caseNode = &ASTNode{
				Type:     NODE_SWITCH_CASE,
				Children: []*ASTNode{listNode, caseBody},
			}
		}

		switchStmt.Children = append(switchStmt.Children, caseNode)

		// Skip newlines between cases
		for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_SEMICOLON {
			p.advance()
		}

		// Check for "then" separator (optional now with newlines)
		if p.current().Type == TOKEN_THEN {
			p.advance()
		}
		// Continue to next case (don't break - newlines separate cases now)
	}

	// Consume 'end' keyword only for multi-line switch statements
	if p.current().Type == TOKEN_DEDENT {
		p.advance()
	}
	if !isOneLine {
		if p.current().Type == TOKEN_END {
			p.advance()
		} else {
			errMsg := fmt.Sprintf("Expected '$' to close switch statement at line %d", startLine)
			if p.LintMode {
				p.recordError(errMsg)
			} else {
				panic(errMsg)
			}
		}
	}

	return switchStmt
}

func (p *Parser) parseLoop() *ASTNode {
	startLine := p.current().Line
	p.expect(TOKEN_LOOP)

	var loopVar *Token = nil

	// Check for optional loop variable: loop i ...
	if p.current().Type == TOKEN_IDENTIFIER {
		ident := p.current()
		loopVar = &ident
		p.advance()
	}

	// Now check what follows
	if p.current().Type == TOKEN_ASSIGN && loopVar != nil {
		// New syntax: loop i:start ...
		p.advance() // consume ':'
		startExpr := p.parseExpression()

		if p.current().Type == TOKEN_TO {
			// loop i:start to end
			p.advance() // consume 'to'
			endExpr := p.parseExpression()

			// Accept either 'do' or ':'
			usesColon := false
			if p.current().Type == TOKEN_DO {
				p.advance()
			} else if p.current().Type == TOKEN_ASSIGN {
				usesColon = true
				p.advance()
			} else {
				if !p.LintMode {
					panic(fmt.Sprintf("Expected 'do' or ':' after loop range at line %d", p.current().Line))
				}
				p.recordError("Expected 'do' or ':' after loop range")
			}

			// Register loop variable in scope
			if loopVar != nil {
				loopScope := make(map[string]string)
				loopScope[loopVar.Value] = "int"
				p.loopVarScopes = append(p.loopVarScopes, loopScope)
			}
			
			// Check for inline loop (no newline after do/colon)
			isMultiLine := false
			var body *ASTNode
			if p.current().Type != TOKEN_NEWLINE {
				// Inline loop - skip whitespace before parsing statement
				p.skipWhitespace()
				// Inline: parse single statement
				body = &ASTNode{Type: NODE_BLOCK}
				stmt := p.parseStatement()
				if stmt != nil {
					body.Children = append(body.Children, stmt)
				}
			} else {
				// Multi-line: parse block
				isMultiLine = true
				p.advance()        // skip the newline
				p.skipWhitespace() // skip any indents
				if usesColon {
					p.blockDepth++ // Opening a multi-line block
					body = p.parseBlockUntilEnd("loop", startLine)
				} else {
					body = p.parseBlock()
				}
			}
			
			// Pop loop variable scope
			if loopVar != nil && len(p.loopVarScopes) > 0 {
				p.loopVarScopes = p.loopVarScopes[:len(p.loopVarScopes)-1]
			}

			// Consume 'end' for multi-line loops (only if not using colon syntax)
			if isMultiLine && !usesColon {
				if p.current().Type == TOKEN_DEDENT {
					p.advance()
				}
				if p.current().Type == TOKEN_END {
					p.advance()
				} else {
					errMsg := fmt.Sprintf("Expected '$' to close loop at line %d", startLine)
					if p.LintMode {
						p.recordError(errMsg)
					} else {
						panic(errMsg)
					}
				}
			}

			loopVarNode := &ASTNode{Type: NODE_IDENTIFIER, Value: loopVar.Value}
			return &ASTNode{
				Type:     NODE_FOR_RANGE_LOOP,
				Children: []*ASTNode{loopVarNode, startExpr, endExpr, body},
			}
		} else if p.current().Type == TOKEN_TILL {
			// loop i:start till condition
			p.advance() // consume 'till'
			condition := p.parseExpression()

			// Accept either 'do' or ':'
			usesColon := false
			if p.current().Type == TOKEN_DO {
				p.advance()
			} else if p.current().Type == TOKEN_ASSIGN {
				usesColon = true
				p.advance()
			} else {
				if !p.LintMode {
					panic(fmt.Sprintf("Expected 'do' or ':' after loop condition at line %d", p.current().Line))
				}
				p.recordError("Expected 'do' or ':' after loop condition")
			}

			// Register loop variable in scope
			if loopVar != nil {
				loopScope := make(map[string]string)
				loopScope[loopVar.Value] = "int"
				p.loopVarScopes = append(p.loopVarScopes, loopScope)
			}
			
			// Check for inline loop
			isMultiLine := false
			var body *ASTNode
			if p.current().Type != TOKEN_NEWLINE {
				p.skipWhitespace()
				body = &ASTNode{Type: NODE_BLOCK}
				stmt := p.parseStatement()
				if stmt != nil {
					body.Children = append(body.Children, stmt)
				}
			} else {
				isMultiLine = true
				p.advance()        // skip newline
				p.skipWhitespace() // skip indents
				if usesColon {
					p.blockDepth++ // Opening a multi-line block
					body = p.parseBlockUntilEnd("loop", startLine)
				} else {
					body = p.parseBlock()
				}
			}
			
			// Pop loop variable scope
			if loopVar != nil && len(p.loopVarScopes) > 0 {
				p.loopVarScopes = p.loopVarScopes[:len(p.loopVarScopes)-1]
			}

			// Consume 'end' for multi-line loops (only if not using colon syntax)
			if isMultiLine && !usesColon {
				if p.current().Type == TOKEN_DEDENT {
					p.advance()
				}
				if p.current().Type == TOKEN_END {
					p.advance()
				} else {
					errMsg := fmt.Sprintf("Expected '$' to close loop at line %d", startLine)
					if p.LintMode {
						p.recordError(errMsg)
					} else {
						panic(errMsg)
					}
				}
			}

			loopVarNode := &ASTNode{Type: NODE_IDENTIFIER, Value: loopVar.Value}
			return &ASTNode{
				Type:     NODE_WHILE_LOOP,
				Children: []*ASTNode{loopVarNode, startExpr, condition, body},
			}
		} else if p.current().Type == TOKEN_ASSIGN {
			// loop i:start: (forever loop with counter starting at start)
			p.advance() // consume second ':'
			p.skipWhitespace()

			// Check for inline loop
			isMultiLine := false
			var body *ASTNode
			if p.current().Type != TOKEN_NEWLINE {
				body = &ASTNode{Type: NODE_BLOCK}
				stmt := p.parseStatement()
				if stmt != nil {
					body.Children = append(body.Children, stmt)
				}
			} else {
				isMultiLine = true
				body = p.parseBlock()
			}

			// Consume 'end' for multi-line loops
			if isMultiLine {
				if p.current().Type == TOKEN_DEDENT {
					p.advance()
				}
				if p.current().Type == TOKEN_END {
					p.advance()
				} else {
					errMsg := fmt.Sprintf("Expected '$' to close loop at line %d", startLine)
					if p.LintMode {
						p.recordError(errMsg)
					} else {
						panic(errMsg)
					}
				}
			}

			loopVarNode := &ASTNode{Type: NODE_IDENTIFIER, Value: loopVar.Value}
			return &ASTNode{
				Type:     NODE_FOR_COUNT_LOOP,
				Value:    loopVar.Value,
				Children: []*ASTNode{loopVarNode, startExpr, body},
			}
		} else {
			if !p.LintMode {
				panic(fmt.Sprintf("Expected 'to', 'till', or ':' after loop variable initialization at line %d", p.current().Line))
			}
			p.recordError("Expected 'to', 'till', or ':' after loop variable initialization")
			return &ASTNode{Type: NODE_WHILE_LOOP, Children: []*ASTNode{}}
		}
	} else if p.current().Type == TOKEN_TO && loopVar != nil {
		// loop i to end (starts at 0)
		p.advance() // consume 'to'
		endExpr := p.parseExpression()

		// Accept either 'do' or ':'
		usesColon := false
		if p.current().Type == TOKEN_DO {
			p.advance()
		} else if p.current().Type == TOKEN_ASSIGN {
			usesColon = true
			p.advance()
		} else {
			if !p.LintMode {
				panic(fmt.Sprintf("Expected 'do' or ':' after loop range at line %d", p.current().Line))
			}
			p.recordError("Expected 'do' or ':' after loop range")
		}

		// Check for inline loop
		isMultiLine := false
		var body *ASTNode
		if p.current().Type != TOKEN_NEWLINE {
			p.skipWhitespace()
			body = &ASTNode{Type: NODE_BLOCK}
			stmt := p.parseStatement()
			if stmt != nil {
				body.Children = append(body.Children, stmt)
			}
		} else {
			isMultiLine = true
			p.advance()        // skip newline
			p.skipWhitespace() // skip indents
			if usesColon {
				p.blockDepth++ // Opening a multi-line block
				body = p.parseBlockUntilEnd("loop", startLine)
			} else {
				body = p.parseBlock()
			}
		}

		// Consume 'end' for multi-line loops (only if not using colon syntax)
		if isMultiLine && !usesColon {
			if p.current().Type == TOKEN_DEDENT {
				p.advance()
			}
			if p.current().Type == TOKEN_END {
				p.advance()
			} else {
				errMsg := fmt.Sprintf("Expected '$' to close loop at line %d", startLine)
				if p.LintMode {
					p.recordError(errMsg)
				} else {
					panic(errMsg)
				}
			}
		}

		loopVarNode := &ASTNode{Type: NODE_IDENTIFIER, Value: loopVar.Value}
		zeroNode := &ASTNode{Type: NODE_NUMBER, Value: "0"}
		return &ASTNode{
			Type:     NODE_FOR_RANGE_LOOP,
			Children: []*ASTNode{loopVarNode, zeroNode, endExpr, body},
		}
	} else if p.current().Type == TOKEN_TO && loopVar == nil {
		// loop to end (no variable, starts at 0)
		p.advance() // consume 'to'
		endExpr := p.parseExpression()

		// Accept either 'do' or ':'
		usesColon := false
		if p.current().Type == TOKEN_DO {
			p.advance()
		} else if p.current().Type == TOKEN_ASSIGN {
			usesColon = true
			p.advance()
		} else {
			if !p.LintMode {
				panic(fmt.Sprintf("Expected 'do' or ':' after loop range at line %d", p.current().Line))
			}
			p.recordError("Expected 'do' or ':' after loop range")
		}

		// Check for inline loop
		isMultiLine := false
		var body *ASTNode
		if p.current().Type != TOKEN_NEWLINE {
			p.skipWhitespace()
			body = &ASTNode{Type: NODE_BLOCK}
			stmt := p.parseStatement()
			if stmt != nil {
				body.Children = append(body.Children, stmt)
			}
		} else {
			isMultiLine = true
			p.advance()        // skip newline
			p.skipWhitespace() // skip indents
			if usesColon {
				p.blockDepth++ // Opening a multi-line block
				body = p.parseBlockUntilEnd("loop", startLine)
			} else {
				body = p.parseBlock()
			}
		}

		// Consume 'end' for multi-line loops (only if not using colon syntax)
		if isMultiLine && !usesColon {
			if p.current().Type == TOKEN_DEDENT {
				p.advance()
			}
			if p.current().Type == TOKEN_END {
				p.advance()
			} else {
				errMsg := fmt.Sprintf("Expected '$' to close loop at line %d", startLine)
				if p.LintMode {
					p.recordError(errMsg)
				} else {
					panic(errMsg)
				}
			}
		}

		// Create anonymous loop variable "_loop_i"
		loopVarNode := &ASTNode{Type: NODE_IDENTIFIER, Value: "_loop_counter"}
		zeroNode := &ASTNode{Type: NODE_NUMBER, Value: "0"}
		return &ASTNode{
			Type:     NODE_FOR_RANGE_LOOP,
			Children: []*ASTNode{loopVarNode, zeroNode, endExpr, body},
		}
	} else if p.current().Type == TOKEN_TILL {
		// loop [i] till condition
		p.advance() // consume 'till'
		condition := p.parseExpression()

		// Accept either 'do' or ':'
		usesColon := false
		if p.current().Type == TOKEN_DO {
			p.advance()
		} else if p.current().Type == TOKEN_ASSIGN {
			usesColon = true
			p.advance()
		} else {
			if !p.LintMode {
				panic(fmt.Sprintf("Expected 'do' or ':' after loop condition at line %d", p.current().Line))
			}
			p.recordError("Expected 'do' or ':' after loop condition")
		}

		// Check for inline loop
		isMultiLine := false
		var body *ASTNode
		if p.current().Type != TOKEN_NEWLINE {
			p.skipWhitespace()
			body = &ASTNode{Type: NODE_BLOCK}
			stmt := p.parseStatement()
			if stmt != nil {
				body.Children = append(body.Children, stmt)
			}
		} else {
			isMultiLine = true
			p.advance()        // skip newline
			p.skipWhitespace() // skip indents
			if usesColon {
				p.blockDepth++ // Opening a multi-line block
				body = p.parseBlockUntilEnd("loop", startLine)
			} else {
				body = p.parseBlock()
			}
		}

		// Consume 'end' for multi-line loops (only if not using colon syntax)
		if isMultiLine && !usesColon {
			if p.current().Type == TOKEN_DEDENT {
				p.advance()
			}
			if p.current().Type == TOKEN_END {
				p.advance()
			} else {
				errMsg := fmt.Sprintf("Expected '$' to close loop at line %d", startLine)
				if p.LintMode {
					p.recordError(errMsg)
				} else {
					panic(errMsg)
				}
			}
		}

		if loopVar != nil {
			// loop i till condition - i should be initialized to 0 locally
			loopVarNode := &ASTNode{Type: NODE_IDENTIFIER, Value: loopVar.Value}
			zeroNode := &ASTNode{Type: NODE_NUMBER, Value: "0"}
			return &ASTNode{
				Type:     NODE_WHILE_LOOP,
				Children: []*ASTNode{loopVarNode, zeroNode, condition, body},
			}
		} else {
			// loop till condition - no local var, should check outer scope in linting
			return &ASTNode{
				Type:     NODE_WHILE_LOOP,
				Children: []*ASTNode{condition, body},
			}
		}
	} else if p.current().Type == TOKEN_IN {
		// loop element in array OR loop key,value in dict
		if loopVar == nil {
			if !p.LintMode {
				panic(fmt.Sprintf("Expected loop variable before 'in' at line %d", p.current().Line))
			}
			p.recordError("Expected loop variable before 'in'")
			return &ASTNode{Type: NODE_WHILE_LOOP, Children: []*ASTNode{}}
		}

		p.advance() // consume 'in'

		// Check if we need to go back and parse key,value
		// Actually, we need to handle this differently - check if there was a comma after first identifier
		// For now, simple case: loop element in array
		collectionExpr := p.parseExpression()

		// Accept either 'do' or ':'
		usesColon := false
		if p.current().Type == TOKEN_DO {
			p.advance()
		} else if p.current().Type == TOKEN_ASSIGN {
			usesColon = true
			p.advance()
		} else {
			if !p.LintMode {
				panic(fmt.Sprintf("Expected 'do' or ':' after 'in' expression at line %d", p.current().Line))
			}
			p.recordError("Expected 'do' or ':' after 'in' expression")
		}

		// Check for inline loop
		isMultiLine := false
		var body *ASTNode
		if p.current().Type != TOKEN_NEWLINE {
			p.skipWhitespace()
			body = &ASTNode{Type: NODE_BLOCK}
			stmt := p.parseStatement()
			if stmt != nil {
				body.Children = append(body.Children, stmt)
			}
		} else {
			isMultiLine = true
			p.advance()        // skip newline
			p.skipWhitespace() // skip indents
			if usesColon {
				p.blockDepth++ // Opening a multi-line block
				body = p.parseBlockUntilEnd("loop", startLine)
			} else {
				body = p.parseBlock()
			}
		}

		// Consume 'end' for multi-line loops (only if not using colon syntax)
		if isMultiLine && !usesColon {
			if p.current().Type == TOKEN_DEDENT {
				p.advance()
			}
			if p.current().Type == TOKEN_END {
				p.advance()
			} else {
				errMsg := fmt.Sprintf("Expected '$' to close loop at line %d", startLine)
				if p.LintMode {
					p.recordError(errMsg)
				} else {
					panic(errMsg)
				}
			}
		}

		elementNode := &ASTNode{Type: NODE_IDENTIFIER, Value: loopVar.Value}
		return &ASTNode{
			Type:     NODE_FOR_IN_ARRAY_LOOP,
			Children: []*ASTNode{elementNode, collectionExpr, body},
		}
	} else if loopVar != nil && p.current().Type == TOKEN_COMMA {
		// loop key,value in dict
		p.advance() // consume ','
		secondIdent := p.expect(TOKEN_IDENTIFIER)
		p.expect(TOKEN_IN)
		dictExpr := p.parseExpression()

		// Accept either 'do' or ':'
		usesColon := false
		if p.current().Type == TOKEN_DO {
			p.advance()
		} else if p.current().Type == TOKEN_ASSIGN {
			usesColon = true
			p.advance()
		} else {
			if !p.LintMode {
				panic(fmt.Sprintf("Expected 'do' or ':' after 'in' expression at line %d", p.current().Line))
			}
			p.recordError("Expected 'do' or ':' after 'in' expression")
		}

		// Check for inline loop
		isMultiLine := false
		var body *ASTNode
		if p.current().Type != TOKEN_NEWLINE {
			p.skipWhitespace()
			body = &ASTNode{Type: NODE_BLOCK}
			stmt := p.parseStatement()
			if stmt != nil {
				body.Children = append(body.Children, stmt)
			}
		} else {
			isMultiLine = true
			p.advance()        // skip newline
			p.skipWhitespace() // skip indents
			if usesColon {
				p.blockDepth++ // Opening a multi-line block
				body = p.parseBlockUntilEnd("loop", startLine)
			} else {
				body = p.parseBlock()
			}
		}

		// Consume 'end' for multi-line loops (only if not using colon syntax)
		if isMultiLine && !usesColon {
			if p.current().Type == TOKEN_DEDENT {
				p.advance()
			}
			if p.current().Type == TOKEN_END {
				p.advance()
			} else {
				errMsg := fmt.Sprintf("Expected '$' to close loop at line %d", startLine)
				if p.LintMode {
					p.recordError(errMsg)
				} else {
					panic(errMsg)
				}
			}
		}

		keyNode := &ASTNode{Type: NODE_IDENTIFIER, Value: loopVar.Value}
		valueNode := &ASTNode{Type: NODE_IDENTIFIER, Value: secondIdent.Value}
		return &ASTNode{
			Type:     NODE_FOR_IN_DICT_LOOP,
			Children: []*ASTNode{keyNode, valueNode, dictExpr, body},
		}
	} else if p.current().Type == TOKEN_DO || p.current().Type == TOKEN_ASSIGN {
		// loop [i] do or loop [i] : - infinite loop, optionally with counter
		p.advance() // consume 'do' or ':'
		p.skipWhitespace()

		// Check for inline loop
		isMultiLine := false
		var body *ASTNode
		if p.current().Type != TOKEN_NEWLINE {
			body = &ASTNode{Type: NODE_BLOCK}
			stmt := p.parseStatement()
			if stmt != nil {
				body.Children = append(body.Children, stmt)
			}
		} else {
			isMultiLine = true
			body = p.parseBlock()
		}

		// Consume 'end' for multi-line loops
		if isMultiLine {
			if p.current().Type == TOKEN_DEDENT {
				p.advance()
			}
			if p.current().Type == TOKEN_END {
				p.advance()
			} else {
				errMsg := fmt.Sprintf("Expected '$' to close loop at line %d", startLine)
				if p.LintMode {
					p.recordError(errMsg)
				} else {
					panic(errMsg)
				}
			}
		}

		if loopVar != nil {
			// loop i do - i starts at 0, increments each iteration
			loopVarNode := &ASTNode{Type: NODE_IDENTIFIER, Value: loopVar.Value}
			zeroNode := &ASTNode{Type: NODE_NUMBER, Value: "0"}
			return &ASTNode{
				Type:     NODE_FOR_COUNT_LOOP,
				Value:    loopVar.Value,
				Children: []*ASTNode{loopVarNode, zeroNode, body},
			}
		} else {
			// loop do - infinite loop without counter
			return &ASTNode{
				Type:     NODE_FOR_COUNT_LOOP,
				Value:    "0",
				Children: []*ASTNode{body},
			}
		}
	} else {
		// Unexpected token
		if !p.LintMode {
			panic(fmt.Sprintf("Expected 'to', 'till', 'in', 'do', or ':' after loop%s at line %d",
				func() string {
					if loopVar != nil {
						return " " + loopVar.Value
					} else {
						return ""
					}
				}(),
				p.current().Line))
		}
		p.recordError("Expected 'to', 'till', 'in', 'do', or ':' after loop")
		return &ASTNode{Type: NODE_WHILE_LOOP, Children: []*ASTNode{}}
	}
}

func (p *Parser) parseWhenStatement() *ASTNode {
	p.expect(TOKEN_WHEN)
	condition := p.expect(TOKEN_IDENTIFIER) // Compile-time condition like DEBUG, RELEASE
	p.expect(TOKEN_THEN)
	p.expect(TOKEN_NEWLINE)
	p.expect(TOKEN_INDENT)

	body := p.parseBlock()

	// Consume 'end' keyword for when statement
	if p.current().Type == TOKEN_END {
		p.advance()
	} else {
		errMsg := fmt.Sprintf("Expected '$' to close when statement at line %d", p.current().Line)
		if p.LintMode {
			p.recordError(errMsg)
		} else {
			panic(errMsg)
		}
	}

	return &ASTNode{
		Type:     NODE_WHEN_STATEMENT,
		Value:    condition.Value,
		Children: []*ASTNode{body},
	}
}

func (p *Parser) parseAhoyStatement() *ASTNode {
	p.expect(TOKEN_AHOY)

	// ahoy is just a shorthand for print
	p.expect(TOKEN_PIPE)

	call := &ASTNode{
		Type:  NODE_CALL,
		Value: "print", // Translate ahoy to print
		Line:  p.current().Line,
	}

	// Set flag to prevent nested parsing issues
	p.inFunctionCall = true

	// Parse arguments until closing pipe
	for p.current().Type != TOKEN_PIPE && p.current().Type != TOKEN_NEWLINE && p.current().Type != TOKEN_EOF {
		arg := p.parseCallArgument()
		call.Children = append(call.Children, arg)

		if p.current().Type == TOKEN_COMMA {
			p.advance()
		} else {
			break
		}
	}

	// Consume closing pipe
	if p.current().Type == TOKEN_PIPE {
		p.advance()
	}

	p.inFunctionCall = false
	return call
}

func (p *Parser) parsePrintStatement() *ASTNode {
	p.expect(TOKEN_PRINT)

	// print is similar to ahoy
	p.expect(TOKEN_PIPE)

	call := &ASTNode{
		Type:  NODE_CALL,
		Value: "print",
		Line:  p.current().Line,
	}

	// Set flag to prevent nested parsing issues
	p.inFunctionCall = true

	// Parse arguments until closing pipe
	for p.current().Type != TOKEN_PIPE && p.current().Type != TOKEN_NEWLINE && p.current().Type != TOKEN_EOF {
		arg := p.parseCallArgument()
		call.Children = append(call.Children, arg)

		if p.current().Type == TOKEN_COMMA {
			p.advance()
		} else {
			break
		}
	}

	// Consume closing pipe
	if p.current().Type == TOKEN_PIPE {
		p.advance()
	}

	p.inFunctionCall = false
	return call
}

func (p *Parser) parseReturnStatement() *ASTNode {
	returnToken := p.expect(TOKEN_RETURN)
	line := returnToken.Line

	ret := &ASTNode{Type: NODE_RETURN_STATEMENT, Line: line}

	// Parse return values
	returnValues := []*ASTNode{}
	if p.current().Type != TOKEN_NEWLINE {
		// Parse first expression
		expr := p.parseOrExpression() // Use parseOrExpression to avoid consuming comma as part of expression
		ret.Children = append(ret.Children, expr)
		returnValues = append(returnValues, expr)

		// Parse additional return values (multiple returns)
		for p.current().Type == TOKEN_COMMA {
			p.advance()
			expr = p.parseOrExpression()
			ret.Children = append(ret.Children, expr)
			returnValues = append(returnValues, expr)
		}
	}

	// In lint mode, validate return types against function signature
	if p.LintMode {
		expectedRet := p.currentFunctionRet

		// Skip validation if not in a function or return type is "infer"
		if expectedRet == "infer" {
			return ret
		}

		// If currentFunctionRet is empty but we're in a function, treat as void
		// We can tell we're in a function if functionScope is not nil and has entries
		if expectedRet == "" && p.functionScope != nil && len(p.functionScope) > 0 {
			expectedRet = "void"
		}

		// Only validate if we have a return type expectation
		if expectedRet != "" {
			// Parse expected return types
			expectedTypes := []string{}
			if expectedRet == "void" {
				// Expecting no return value
				expectedTypes = []string{}
			} else {
				expectedTypes = strings.Split(expectedRet, ",")
			}

			// Get actual return types
			actualTypes := []string{}
			for _, returnVal := range returnValues {
				actualTypes = append(actualTypes, p.inferType(returnVal))
			}

			// Check if counts match
			if len(expectedTypes) != len(actualTypes) {
				if len(expectedTypes) == 0 && len(actualTypes) > 0 {
					typesStr := "[" + strings.Join(actualTypes, ", ") + "]"
					errMsg := fmt.Sprintf("Expected return type void but got multiple return types %s", typesStr)
					p.recordError(errMsg)
				} else if len(expectedTypes) > 0 && len(actualTypes) == 0 {
					errMsg := fmt.Sprintf("Expected return type(s) but got none")
					p.recordError(errMsg)
				} else {
					errMsg := fmt.Sprintf("Expected %d return value(s) but got %d", len(expectedTypes), len(actualTypes))
					p.recordError(errMsg)
				}
			} else {
				// Check each type
				for i := 0; i < len(expectedTypes); i++ {
					if !p.checkTypeCompatibility(expectedTypes[i], actualTypes[i]) {
						errMsg := fmt.Sprintf("Return type mismatch at position %d: expected %s but got %s",
							i+1, expectedTypes[i], actualTypes[i])
						p.recordError(errMsg)
					}
				}
			}
		}
	}

	return ret
}

func (p *Parser) parseAssertStatement() *ASTNode {
	assertToken := p.expect(TOKEN_ASSERT)

	// Parse the condition expression
	condition := p.parseExpression()

	return &ASTNode{
		Type:     NODE_ASSERT_STATEMENT,
		Line:     assertToken.Line,
		Children: []*ASTNode{condition},
	}
}

func (p *Parser) parseDeferStatement() *ASTNode {
	deferToken := p.expect(TOKEN_DEFER)

	// Parse the deferred statement
	// Could be a function call, ahoy statement, print statement, etc.
	var statement *ASTNode
	if p.current().Type == TOKEN_PRINT {
		statement = p.parsePrintStatement()
	} else if p.current().Type == TOKEN_AHOY {
		statement = p.parseAhoyStatement()
	} else {
		statement = p.parseExpression()
	}

	return &ASTNode{
		Type:     NODE_DEFER_STATEMENT,
		Line:     deferToken.Line,
		Children: []*ASTNode{statement},
	}
}

func (p *Parser) parseImportStatement() *ASTNode {
	importToken := p.current()
	p.expect(TOKEN_IMPORT)

	// Validate that import is at top level (after program declaration, before other code)
	if p.seenNonImport {
		errMsg := fmt.Sprintf("Import statements must be at the top of the file, after the program declaration at line %d", importToken.Line)
		if p.LintMode {
			p.recordError(errMsg)
		} else {
			panic(errMsg)
		}
	}

	// Check if there's an identifier (namespace) before the string path
	var namespace string
	var path string

	if p.current().Type == TOKEN_IDENTIFIER {
		namespace = p.current().Value
		p.advance()
		path = p.expect(TOKEN_STRING).Value
	} else if p.current().Type == TOKEN_STRING {
		path = p.expect(TOKEN_STRING).Value
		namespace = "" // No namespace means import all into global scope
	} else {
		if p.LintMode {
			p.recordError(fmt.Sprintf("Expected identifier or string path after import at line %d", p.current().Line))
			return &ASTNode{Type: NODE_IMPORT_STATEMENT}
		}
		panic(fmt.Sprintf("Expected identifier or string path after import at line %d", p.current().Line))
	}

	// Parse the C header file if it ends with .h
	if strings.HasSuffix(path, ".h") {
		headerInfo, err := ParseCHeader(path)
		if err == nil {
			if namespace != "" {
				// Store with namespace
				p.cHeaders[namespace] = headerInfo
			} else {
				// Merge into global
				for name, fn := range headerInfo.Functions {
					p.cHeaderGlobal.Functions[name] = fn
				}
				for name, enum := range headerInfo.Enums {
					p.cHeaderGlobal.Enums[name] = enum

					// Add enum values as constants
					for valueName := range enum.Values {
						// Enum values are already in snake_case style (KEY_RIGHT, etc.)
						// Make them available as identifiers
						p.variableTypes[valueName] = "int" // Enums are integers
					}
				}
				for name, def := range headerInfo.Defines {
					p.cHeaderGlobal.Defines[name] = def

					// Add defines as constants (color constants like RAYWHITE)
					// Determine type based on value
					defType := "Color" // Most defines in raylib are colors
					if strings.Contains(def.Value, "CLITERAL(Color)") {
						defType = "Color"
					}
					p.variableTypes[name] = defType
				}
				for name, str := range headerInfo.Structs {
					p.cHeaderGlobal.Structs[name] = str

					// Also add to p.structs for validation (lowercase first letter)
					lowerName := ToLowerFirst(name)
					fields := []StructField{}
					for _, cField := range str.Fields {
						fields = append(fields, StructField{
							Name: cField.Name,
							Type: cField.Type,
						})
					}
					p.structs[lowerName] = &StructDefinition{
						Name:   name,
						Fields: fields,
					}
					// Also add with original name for case-insensitive matching
					p.structs[name] = &StructDefinition{
						Name:   name,
						Fields: fields,
					}
				}
			}
		}
	}

	return &ASTNode{
		Type:     NODE_IMPORT_STATEMENT,
		Value:    path,
		DataType: namespace, // Use DataType field to store namespace
	}
}

func (p *Parser) parseProgramDeclaration() *ASTNode {
	p.expect(TOKEN_PROGRAM)
	name := p.expect(TOKEN_IDENTIFIER)

	// Don't skip newlines here - let parseProgram handle them

	return &ASTNode{
		Type:  NODE_PROGRAM_DECLARATION,
		Value: name.Value,
		Line:  name.Line,
	}
}

func (p *Parser) parseAssignmentOrExpression() *ASTNode {
	// Check for object/dict property assignment: obj<'prop'>: value or dict{"key"}: value
	if p.pos+2 < len(p.tokens) && p.tokens[p.pos+1].Type == TOKEN_LANGLE {
		// Check if this is obj<'prop'>: pattern
		savedPos := p.pos
		p.advance() // skip identifier
		p.advance() // skip <
		// Skip the accessor
		depth := 1
		for p.pos < len(p.tokens) && depth > 0 {
			if p.current().Type == TOKEN_LANGLE {
				depth++
			} else if p.current().Type == TOKEN_RANGLE {
				depth--
			}
			p.advance()
		}
		isAssignment := p.current().Type == TOKEN_ASSIGN
		p.pos = savedPos // restore position

		if isAssignment {
			// Parse as object/dict property assignment
			target := p.parsePrimaryExpression() // This will parse obj<'prop'> or dict{"key"}
			p.expect(TOKEN_ASSIGN)
			value := p.parseExpression()

			// Convert to assignment node
			return &ASTNode{
				Type:     NODE_ASSIGNMENT,
				Children: []*ASTNode{target, value},
				Line:     target.Line,
			}
		}
	}

	// Check for array index assignment: arr[index]: value
	if p.pos+2 < len(p.tokens) && p.tokens[p.pos+1].Type == TOKEN_LBRACKET {
		savedPos := p.pos
		p.advance() // skip identifier
		p.advance() // skip [
		// Skip the index
		depth := 1
		for p.pos < len(p.tokens) && depth > 0 {
			if p.current().Type == TOKEN_LBRACKET {
				depth++
			} else if p.current().Type == TOKEN_RBRACKET {
				depth--
			}
			p.advance()
		}
		isAssignment := p.current().Type == TOKEN_ASSIGN
		p.pos = savedPos // restore position

		if isAssignment {
			target := p.parsePrimaryExpression() // This will parse arr[index]
			p.expect(TOKEN_ASSIGN)
			value := p.parseExpression()

			return &ASTNode{
				Type:     NODE_ASSIGNMENT,
				Children: []*ASTNode{target, value},
				Line:     target.Line,
			}
		}
	}

	// Check for member access assignment: obj.property: value
	if p.pos+2 < len(p.tokens) && p.tokens[p.pos+1].Type == TOKEN_DOT {
		savedPos := p.pos
		p.advance() // skip identifier
		p.advance() // skip .
		// Skip the property name(s)
		for p.pos < len(p.tokens) && p.current().Type == TOKEN_IDENTIFIER {
			p.advance()
			if p.current().Type == TOKEN_DOT {
				p.advance() // skip next dot
			} else {
				break
			}
		}
		isAssignment := p.current().Type == TOKEN_ASSIGN
		p.pos = savedPos // restore position

		if isAssignment {
			target := p.parsePrimaryExpression() // This will parse obj.property
			p.expect(TOKEN_ASSIGN)
			value := p.parseExpression()

			// Validate property assignment in lint mode
			if p.LintMode && target.Type == NODE_MEMBER_ACCESS {
				p.validatePropertyAssignment(target, value, target.Line)
			}

			return &ASTNode{
				Type:     NODE_ASSIGNMENT,
				Children: []*ASTNode{target, value},
				Line:     target.Line,
			}
		}
	}

	// Check for dict property assignment: dict{"key"}: value
	if p.pos+2 < len(p.tokens) && p.tokens[p.pos+1].Type == TOKEN_LBRACE {
		savedPos := p.pos
		p.advance() // skip identifier
		p.advance() // skip {
		// Skip the key
		depth := 1
		for p.pos < len(p.tokens) && depth > 0 {
			if p.current().Type == TOKEN_LBRACE {
				depth++
			} else if p.current().Type == TOKEN_RBRACE {
				depth--
			}
			p.advance()
		}
		isAssignment := p.current().Type == TOKEN_ASSIGN
		p.pos = savedPos // restore position

		if isAssignment {
			target := p.parsePrimaryExpression() // This will parse dict{"key"}
			p.expect(TOKEN_ASSIGN)
			value := p.parseExpression()

			return &ASTNode{
				Type:     NODE_ASSIGNMENT,
				Children: []*ASTNode{target, value},
				Line:     target.Line,
			}
		}
	}

	if p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Type == TOKEN_ASSIGN {
		// Assignment with possible type annotation
		name := p.expect(TOKEN_IDENTIFIER)
		line := name.Line
		p.expect(TOKEN_ASSIGN)

		// Check for type annotation (type=) or inferred type (:=)
		var explicitType string
		if p.current().Type == TOKEN_EQUALS {
			// := syntax - inferred type
			p.advance()       // consume =
			explicitType = "" // Empty means inferred
		} else if p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
			p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_BOOL_TYPE ||
			p.current().Type == TOKEN_COLOR_TYPE || p.current().Type == TOKEN_VECTOR2_TYPE ||
			p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
			p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
			p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
			p.current().Type == TOKEN_IDENTIFIER {

			// Check if this is a cast (type followed by parenthesis) - if so, don't treat as type annotation
			if (p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
				p.current().Type == TOKEN_CHAR_TYPE || p.current().Type == TOKEN_STRING_TYPE) &&
				p.peek(1).Type == TOKEN_LPAREN {
				// This is a cast like int(5), not a type annotation
				// Fall through to parse it as an expression
			} else {
				// This might be a type annotation
				possibleType := p.current().Value

				// Look ahead to see if there's an = or < after the type
				// < indicates struct initialization: name : type<...>
				if p.peek(1).Type == TOKEN_EQUALS || p.peek(1).Type == TOKEN_LANGLE {
					explicitType = possibleType
					p.advance() // consume type
					if p.current().Type == TOKEN_EQUALS {
						p.advance() // consume =
					}
					// If TOKEN_LANGLE, DON'T consume it - but we need to handle it specially below
				}
			}
		}

		// Special handling: if we have explicitType and current token is LANGLE,
		// this is struct initialization like: name : type<...>
		// We need to parse it as identifier<...> pattern to preserve the type name
		var value *ASTNode
		if explicitType != "" && p.current().Type == TOKEN_LANGLE {
			// Manually handle the object literal with type name
			p.advance() // consume <

			// Check for empty <>
			if p.current().Type == TOKEN_RANGLE {
				p.advance()
				value = &ASTNode{
					Type:     NODE_OBJECT_LITERAL,
					DataType: "object",
					Value:    explicitType, // Set the type name
					Children: []*ASTNode{},
					Line:     line,
				}
			} else if (p.current().Type == TOKEN_IDENTIFIER || p.current().Type == TOKEN_STRING) &&
				p.peek(1).Type == TOKEN_ASSIGN {
				// Parse object literal with properties
				value = p.parseObjectLiteral()
				value.Value = explicitType // Set the type name
			} else {
				// Parse as regular expression (fallback)
				value = p.parseExpression()
			}
		} else {
			value = p.parseExpression()
		}

		// Type checking in lint mode
		if p.LintMode {
			varName := name.Value

			// Check function scope first, then global scope
			existingType := ""
			exists := false

			if p.functionScope != nil {
				if t, ok := p.functionScope[varName]; ok {
					existingType = t
					exists = true
				}
			}
			if !exists {
				if t, ok := p.variableTypes[varName]; ok {
					existingType = t
					exists = true
				}
			}

			if exists {
				// Variable already declared - check type compatibility
				inferredType := p.inferType(value)

				// For struct types, normalize the comparison
				// existingType might be "typename" while inferredType is "struct:typename"
				expectedType := existingType
				if inferredType != "unknown" && strings.HasPrefix(inferredType, "struct:") {
					// If inferred type has struct: prefix but existing doesn't, add it
					if !strings.HasPrefix(expectedType, "struct:") {
						expectedType = "struct:" + expectedType
					}
				}

				if !p.checkTypeCompatibility(expectedType, inferredType) {
					errMsg := fmt.Sprintf("Type mismatch (line %d): can't use %s:%s as %s",
						line, varName, existingType, inferredType)
					p.recordError(errMsg)
				}
			} else {
				// First declaration - store the type
				// If inside a function, store in function scope, otherwise global
				targetScope := p.variableTypes
				if p.currentFunctionRet != "" && p.functionScope != nil {
					targetScope = p.functionScope
				}

				if explicitType != "" {
					targetScope[varName] = explicitType

					// Validate struct initialization
					if value.Type == NODE_OBJECT_LITERAL {
						p.validateStructInitialization(explicitType, value, line)
						// Track object literal properties
						p.trackObjectLiteralProperties(varName, value)
					}
				} else {
					// Infer type from value
					inferredType := p.inferType(value)
					if inferredType != "unknown" {
						targetScope[varName] = inferredType

						// Track object literal properties
						if value.Type == NODE_OBJECT_LITERAL {
							p.trackObjectLiteralProperties(varName, value)
						}
					}
				}

				// Validate property assignment for struct reassignments
				if value.Type == NODE_MEMBER_ACCESS && len(value.Children) > 0 {
					// This is a property assignment like obj.prop: value
					// We'll handle this in the reassignment validation below
				}

				// Track array lengths for bounds checking
				if value.Type == NODE_ARRAY_LITERAL {
					p.arrayLengths[varName] = ArrayInfo{
						Length:  len(value.Children),
						IsKnown: true,
					}
				} else if value.Type == NODE_METHOD_CALL {
					// Track arrays created by methods
					p.trackArrayMethodLength(varName, value)
				}
			}
		}

		return &ASTNode{
			Type:     NODE_ASSIGNMENT,
			Value:    name.Value,
			DataType: explicitType,
			Children: []*ASTNode{value},
			Line:     line,
		}
	}

	// Check for property assignment: identifier.property: value (DUPLICATE - already handled above at line 1277)
	// This code can be removed as it's redundant
	/*
		/*
		if p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Type == TOKEN_DOT {
			// This is redundant - already handled at line 1277
			target := p.parsePrimaryExpression()

			if p.current().Type == TOKEN_ASSIGN {
				p.expect(TOKEN_ASSIGN)
				value := p.parseExpression()

				if p.LintMode && target.Type == NODE_MEMBER_ACCESS {
					p.validatePropertyAssignment(target, value, target.Line)
				}

				return &ASTNode{
					Type:     NODE_ASSIGNMENT,
					Children: []*ASTNode{target, value},
					Line:     target.Line,
				}
			}
		}
	*/

	return p.parseExpression()
}

func (p *Parser) parseCallArgument() *ASTNode {
	// Parse an expression but stop at comma or pipe
	return p.parseAdditiveExpression()
}

func (p *Parser) parseBlock() *ASTNode {
	block := &ASTNode{Type: NODE_BLOCK}

	for p.current().Type != TOKEN_DEDENT && p.current().Type != TOKEN_EOF {
		if p.current().Type == TOKEN_NEWLINE {
			p.advance()
			continue
		}
		stmt := p.parseStatement()
		if stmt != nil {
			block.Children = append(block.Children, stmt)
		}
	}

	if p.current().Type == TOKEN_DEDENT {
		p.advance()
	}

	return block
}

// parseBlockUntilEnd parses statements until encountering '$' keyword
func (p *Parser) parseBlockUntilEnd(constructName string, startLine int) *ASTNode {
	block := &ASTNode{Type: NODE_BLOCK}
	
	// Remember the depth when entering - if it decreases, a nested $#N closed us
	entryDepth := p.blockDepth

	maxIterations := 10000 // Safety limit to prevent infinite loops
	iterations := 0

	for p.current().Type != TOKEN_END && p.current().Type != TOKEN_EOF {
		// Check if a nested $#N closed this block
		if p.blockDepth < entryDepth {
			// This block was closed by a nested $#N
			return block
		}
		
		iterations++
		if iterations > maxIterations {
			errMsg := fmt.Sprintf("Parser safety limit reached while parsing %s at line %d - possible infinite loop", constructName, startLine)
			if p.LintMode {
				p.recordError(errMsg)
			} else {
				panic(errMsg)
			}
			break
		}

		// Save position to detect if we're stuck
		oldPos := p.pos

		if p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_DEDENT {
			p.advance()
			continue
		}
		stmt := p.parseStatement()
		if stmt != nil {
			block.Children = append(block.Children, stmt)
		}

		// Safety check: if position hasn't advanced, force advance to prevent infinite loop
		if p.pos == oldPos && p.current().Type != TOKEN_EOF && p.current().Type == TOKEN_END {
			p.advance()
		}
	}

	if p.current().Type == TOKEN_END {
		token := p.current()
		p.advance() // consume 'end'
		
		// Decrement blockDepth for this block closure
		p.blockDepth--
		
		// Handle $#N syntax - this TOKEN_END closes this block, but may close additional parent blocks
		if strings.HasPrefix(token.Value, "$#") {
			countStr := strings.TrimPrefix(token.Value, "$#")
			count, err := strconv.Atoi(countStr)
			
			// Validate $#N syntax
			if err != nil || count <= 0 {
				if p.LintMode {
					p.Errors = append(p.Errors, ParseError{
						Message: fmt.Sprintf("Invalid $# syntax: %s (must be positive integer)", token.Value),
						Line:    token.Line,
						Column:  token.Column,
					})
				}
			} else if count > p.blockDepth + 1 { // +1 because we already decremented once
				if p.LintMode {
					p.Errors = append(p.Errors, ParseError{
						Message: fmt.Sprintf("Cannot close %d blocks, only %d block(s) open", count, p.blockDepth + 1),
						Line:    token.Line,
						Column:  token.Column,
					})
				}
			}
			
			if err == nil && count > 1 {
				// This $ closed the current block (already decremented above)
				// Need to close count-1 more blocks
				p.blockDepth -= (count - 1)
				
				// Pop additional loop var scopes
				for i := 1; i < count && len(p.loopVarScopes) > 0; i++ {
					if p.blockDepth >= 0 && len(p.loopVarScopes) > p.blockDepth {
						p.loopVarScopes = p.loopVarScopes[:len(p.loopVarScopes)-1]
					} else if len(p.loopVarScopes) > 0 {
						p.loopVarScopes = p.loopVarScopes[:len(p.loopVarScopes)-1]
					}
				}
			}
		}
	} else {
		errMsg := fmt.Sprintf("Expected '$' to close %s at line %d", constructName, startLine)
		if p.LintMode {
			p.recordError(errMsg)
		} else {
			panic(errMsg)
		}
	}

	return block
}

func (p *Parser) parseExpression() *ASTNode {
	return p.parseTernaryExpression()
}

func (p *Parser) parseTernaryExpression() *ASTNode {
	// Parse the condition
	condition := p.parseOrExpression()

	// Check for ?? operator
	if p.current().Type == TOKEN_TERNARY {
		p.advance() // consume ??

		// Parse the true branch
		trueBranch := p.parseOrExpression()

		// Expect : before false branch
		if p.current().Type != TOKEN_ASSIGN {
			if !p.LintMode {
				panic(fmt.Sprintf("Expected ':' after ternary true branch at line %d", p.current().Line))
			}
			p.recordError("Expected ':' after ternary true branch")
			return condition
		}
		p.advance() // consume :

		// Parse the false branch
		falseBranch := p.parseTernaryExpression() // Allow nested ternaries

		return &ASTNode{
			Type:     NODE_TERNARY,
			Children: []*ASTNode{condition, trueBranch, falseBranch},
			Line:     condition.Line,
		}
	}

	return condition
}

func (p *Parser) parseExpressionContinuation(leftNode *ASTNode) *ASTNode {
	// Continue parsing from the given left node through the expression hierarchy
	// This is used when we've already consumed an identifier in loop parsing
	left := leftNode

	// Check for relational operators that might follow
	for p.current().Type == TOKEN_LANGLE || p.current().Type == TOKEN_RANGLE ||
		p.current().Type == TOKEN_LESS_EQUAL || p.current().Type == TOKEN_GREATER_EQUAL ||
		p.current().Type == TOKEN_LESSER_WORD || p.current().Type == TOKEN_GREATER_WORD ||
		p.current().Type == TOKEN_IS {
		op := p.current()
		p.advance()
		right := p.parseAdditiveExpression()
		left = &ASTNode{
			Type:     NODE_BINARY_OP,
			Value:    op.Value,
			Children: []*ASTNode{left, right},
		}
	}

	return left
}

func (p *Parser) parseOrExpression() *ASTNode {
	left := p.parseAndExpression()

	for p.current().Type == TOKEN_OR {
		op := p.current()
		p.advance()
		right := p.parseAndExpression()
		left = &ASTNode{
			Type:     NODE_BINARY_OP,
			Value:    op.Value,
			Children: []*ASTNode{left, right},
		}
	}

	return left
}

func (p *Parser) parseAndExpression() *ASTNode {
	left := p.parseEqualityExpression()

	for p.current().Type == TOKEN_AND {
		op := p.current()
		p.advance()
		right := p.parseEqualityExpression()
		left = &ASTNode{
			Type:     NODE_BINARY_OP,
			Value:    op.Value,
			Children: []*ASTNode{left, right},
		}
	}

	return left
}

func (p *Parser) parseEqualityExpression() *ASTNode {
	left := p.parseRelationalExpression()

	for p.current().Type == TOKEN_IS {
		op := p.current()
		p.advance()
		right := p.parseRelationalExpression()
		left = &ASTNode{
			Type:     NODE_BINARY_OP,
			Value:    op.Value,
			Children: []*ASTNode{left, right},
		}
	}

	return left
}

func (p *Parser) parseRelationalExpression() *ASTNode {
	left := p.parseAdditiveExpression()

	for p.current().Type == TOKEN_LANGLE || p.current().Type == TOKEN_RANGLE ||
		p.current().Type == TOKEN_LESS_EQUAL || p.current().Type == TOKEN_GREATER_EQUAL ||
		p.current().Type == TOKEN_LESSER_WORD || p.current().Type == TOKEN_GREATER_WORD {
		op := p.current()
		p.advance()
		right := p.parseAdditiveExpression()
		left = &ASTNode{
			Type:     NODE_BINARY_OP,
			Value:    op.Value,
			Children: []*ASTNode{left, right},
		}
	}

	return left
}

func (p *Parser) parseAdditiveExpression() *ASTNode {
	left := p.parseMultiplicativeExpression()

	for p.current().Type == TOKEN_PLUS || p.current().Type == TOKEN_MINUS ||
		p.current().Type == TOKEN_PLUS_WORD || p.current().Type == TOKEN_MINUS_WORD {
		op := p.current()
		p.advance()
		right := p.parseMultiplicativeExpression()
		left = &ASTNode{
			Type:     NODE_BINARY_OP,
			Value:    op.Value,
			Children: []*ASTNode{left, right},
		}
	}

	return left
}

func (p *Parser) parseMultiplicativeExpression() *ASTNode {
	left := p.parseUnaryExpression()

	for p.current().Type == TOKEN_MULTIPLY || p.current().Type == TOKEN_DIVIDE ||
		p.current().Type == TOKEN_MODULO || p.current().Type == TOKEN_TIMES_WORD ||
		p.current().Type == TOKEN_DIV_WORD || p.current().Type == TOKEN_MOD_WORD {
		op := p.current()
		p.advance()
		right := p.parseUnaryExpression()
		left = &ASTNode{
			Type:     NODE_BINARY_OP,
			Value:    op.Value,
			Children: []*ASTNode{left, right},
		}
	}

	return left
}

func (p *Parser) parseUnaryExpression() *ASTNode {
	if p.current().Type == TOKEN_NOT || p.current().Type == TOKEN_MINUS {
		op := p.current()
		p.advance()
		expr := p.parseUnaryExpression()
		return &ASTNode{
			Type:     NODE_UNARY_OP,
			Value:    op.Value,
			Children: []*ASTNode{expr},
		}
	}

	return p.parsePrimaryExpression()
}

func (p *Parser) parsePrimaryExpression() *ASTNode {
	switch p.current().Type {
	case TOKEN_NUMBER:
		token := p.current()
		p.advance()
		node := &ASTNode{
			Type:  NODE_NUMBER,
			Value: token.Value,
			Line:  token.Line,
		}
		// Determine if it's int or float
		if _, err := strconv.Atoi(token.Value); err == nil {
			node.DataType = "int"
		} else {
			node.DataType = "float"
		}
		return node

	case TOKEN_STRING:
		token := p.current()
		p.advance()
		node := &ASTNode{
			Type:     NODE_STRING,
			Value:    token.Value,
			DataType: "string",
			Line:     token.Line,
		}
		// Check for method call on string literal
		if p.current().Type == TOKEN_DOT {
			return p.parseMemberAccessChain(node)
		}
		return node

	case TOKEN_F_STRING:
		token := p.current()
		p.advance()
		node := &ASTNode{
			Type:     NODE_F_STRING,
			Value:    token.Value,
			DataType: "string",
			Line:     token.Line,
		}
		// Check for method call on f-string
		if p.current().Type == TOKEN_DOT {
			return p.parseMemberAccessChain(node)
		}
		return node

	case TOKEN_CHAR:
		token := p.current()
		p.advance()
		return &ASTNode{
			Type:     NODE_CHAR,
			Value:    token.Value,
			DataType: "char",
			Line:     token.Line,
		}

	case TOKEN_TRUE, TOKEN_FALSE:
		token := p.current()
		p.advance()
		return &ASTNode{
			Type:     NODE_BOOLEAN,
			Value:    token.Value,
			DataType: "bool",
			Line:     token.Line,
		}

	case TOKEN_QUESTION:
		// Loop counter variable ?
		token := p.current()
		p.advance()
		return &ASTNode{
			Type:  NODE_IDENTIFIER,
			Value: "__loop_counter",
			Line:  token.Line,
		}

	case TOKEN_IDENTIFIER:
		token := p.current()
		p.advance()

		// Check for array access identifier[index]
		if p.current().Type == TOKEN_LBRACKET {
			p.advance()
			index := p.parseExpression()
			p.expect(TOKEN_RBRACKET)

			// Validate access syntax in lint mode
			if p.LintMode {
				if varType, ok := p.variableTypes[token.Value]; ok {
					if varType == "dict" {
						errMsg := fmt.Sprintf("Invalid dict access syntax, use dict{} instead of array[]")
						p.recordError(errMsg)
					} else if varType == "object" {
						errMsg := fmt.Sprintf("Invalid object access syntax, use object<> instead of array[]")
						p.recordError(errMsg)
					}
				}
			}

			node := &ASTNode{
				Type:     NODE_ARRAY_ACCESS,
				Value:    token.Value,
				Children: []*ASTNode{index},
				Line:     token.Line,
			}

			// Validate array bounds in lint mode
			if p.LintMode {
				identNode := &ASTNode{Type: NODE_IDENTIFIER, Value: token.Value}
				p.validateArrayAccess(identNode, index, token.Line)
			}

			// Check for member access after array access
			if p.current().Type == TOKEN_DOT {
				return p.parseMemberAccessChain(node)
			}
			return node
		}

		// Check for object instantiation identifier<...> or old-style access identifier<index>
		if p.current().Type == TOKEN_LANGLE {
			p.advance()

			// Check for empty object instantiation identifier<>
			if p.current().Type == TOKEN_RANGLE {
				p.advance() // consume >
				obj := &ASTNode{
					Type:     NODE_OBJECT_LITERAL,
					DataType: "object",
					Value:    token.Value, // Set the type name
					Children: []*ASTNode{},
					Line:     token.Line,
				}
				// Check for member access
				if p.current().Type == TOKEN_DOT {
					return p.parseMemberAccessChain(obj)
				}
				return obj
			}

			// Check if this is object instantiation with properties (identifier: or "string":)
			if (p.current().Type == TOKEN_IDENTIFIER || p.current().Type == TOKEN_STRING) &&
				p.peek(1).Type == TOKEN_ASSIGN {
				// This is object instantiation with named properties
				obj := p.parseObjectLiteral() // Will consume the closing >
				obj.Value = token.Value       // Set the type name
				return obj
			}

			// Otherwise, it's old-style array/object access
			accessor := p.parseCallArgument()
			p.expect(TOKEN_RANGLE)

			// Determine if this is object property access or array access
			// Object property access uses string literals: obj<'prop'> or obj<"prop">
			// Array access uses numbers/variables: arr<0> or arr<i>
			nodeType := NODE_ARRAY_ACCESS
			if accessor.Type == NODE_STRING {
				nodeType = NODE_OBJECT_ACCESS
			}

			// Validate access syntax in lint mode
			if p.LintMode {
				if varType, ok := p.variableTypes[token.Value]; ok {
					if varType == "array" && nodeType == NODE_OBJECT_ACCESS {
						errMsg := fmt.Sprintf("Invalid array access syntax, use array[] instead of object<>")
						p.recordError(errMsg)
					} else if varType == "dict" {
						errMsg := fmt.Sprintf("Invalid dict access syntax, use dict{} instead of object<>")
						p.recordError(errMsg)
					} else if varType == "array" && nodeType == NODE_ARRAY_ACCESS {
						errMsg := fmt.Sprintf("Invalid array access syntax, use array[] instead of object<>")
						p.recordError(errMsg)
					}
				}
			}

			node := &ASTNode{
				Type:     nodeType,
				Value:    token.Value,
				Children: []*ASTNode{accessor},
				Line:     token.Line,
			}

			// Validate array bounds for array access
			if p.LintMode && nodeType == NODE_ARRAY_ACCESS {
				identNode := &ASTNode{Type: NODE_IDENTIFIER, Value: token.Value}
				p.validateArrayAccess(identNode, accessor, token.Line)
			}

			// Check for member access
			if p.current().Type == TOKEN_DOT {
				return p.parseMemberAccessChain(node)
			}
			return node
		}

		// Check for dict access identifier{"key"}
		if p.current().Type == TOKEN_LBRACE {
			p.advance()
			key := p.parseExpression()
			p.expect(TOKEN_RBRACE)

			// Validate access syntax in lint mode
			if p.LintMode {
				if varType, ok := p.variableTypes[token.Value]; ok {
					if varType == "array" {
						errMsg := fmt.Sprintf("Invalid array access syntax, use array[] instead of dict{}")
						p.recordError(errMsg)
					} else if varType == "object" {
						errMsg := fmt.Sprintf("Invalid object access syntax, use object<> instead of dict{}")
						p.recordError(errMsg)
					}
				}
			}

			return &ASTNode{
				Type:     NODE_DICT_ACCESS,
				Value:    token.Value,
				Children: []*ASTNode{key},
				Line:     token.Line,
			}
		}

		// Check for function call with pipes (but not if we're already in a function call)
		if p.current().Type == TOKEN_PIPE && !p.inFunctionCall {
			p.advance()
			call := &ASTNode{
				Type:  NODE_CALL,
				Value: token.Value,
				Line:  token.Line,
			}

			// Set flag to prevent nested parsing issues
			p.inFunctionCall = true

			// Parse arguments until closing pipe
			for p.current().Type != TOKEN_PIPE && p.current().Type != TOKEN_NEWLINE && p.current().Type != TOKEN_EOF {
				arg := p.parseCallArgument()
				call.Children = append(call.Children, arg)

				if p.current().Type == TOKEN_COMMA {
					p.advance()
				} else {
					break
				}
			}

			// Consume closing pipe
			if p.current().Type == TOKEN_PIPE {
				p.advance()
			}

			p.inFunctionCall = false
			return call
		}

		// Check if this could be a zero-argument function call (no || needed)
		// This happens when:
		// - It's a C function with zero params
		// - Or it's an Ahoy function with zero params
		// - And it's NOT followed by || (which would make it explicit)
		isLikelyZeroArgFunc := false

		// Check C functions (global and namespaced)
		if p.cHeaderGlobal != nil {
			for cFuncName, cFunc := range p.cHeaderGlobal.Functions {
				snakeName := PascalToSnake(cFuncName)
				if snakeName == token.Value && len(cFunc.Parameters) == 0 {
					isLikelyZeroArgFunc = true
					break
				}
			}
		}

		if !isLikelyZeroArgFunc {
			for _, headerInfo := range p.cHeaders {
				for cFuncName, cFunc := range headerInfo.Functions {
					snakeName := PascalToSnake(cFuncName)
					if snakeName == token.Value && len(cFunc.Parameters) == 0 {
						isLikelyZeroArgFunc = true
						break
					}
				}
				if isLikelyZeroArgFunc {
					break
				}
			}
		}

		// Check Ahoy functions - look for function declarations with zero params
		if !isLikelyZeroArgFunc && p.LintMode {
			// In lint mode, we track function signatures
			// For now, just check if it's a known function name
			// (We could enhance this by tracking function parameter counts)
		}

		// If it's a zero-arg function, create a call node
		if isLikelyZeroArgFunc {
			return &ASTNode{
				Type:     NODE_CALL,
				Value:    token.Value,
				Line:     token.Line,
				Children: []*ASTNode{}, // Empty args
			}
		}

		// Check for member access (property or method)
		if p.current().Type == TOKEN_DOT {
			node := &ASTNode{
				Type:  NODE_IDENTIFIER,
				Value: token.Value,
				Line:  token.Line,
			}
			return p.parseMemberAccessChain(node)
		}

		return &ASTNode{
			Type:  NODE_IDENTIFIER,
			Value: token.Value,
			Line:  token.Line,
		}

	case TOKEN_LBRACE:
		return p.parseDictLiteral()

	case TOKEN_LANGLE:
		return p.parseArrayLiteral()

	case TOKEN_LBRACKET:
		return p.parseArrayLiteralBracket()

	case TOKEN_LPAREN:
		// Parenthesized expression
		p.advance() // consume '('
		expr := p.parseExpression()
		p.expect(TOKEN_RPAREN)
		return expr

	// Type casts: int(value), float(value), char(value), string(value)
	// Or type instantiation: vector2<x,y>, color<r,g,b,a>
	case TOKEN_INT_TYPE, TOKEN_FLOAT_TYPE, TOKEN_CHAR_TYPE, TOKEN_STRING_TYPE, TOKEN_VECTOR2_TYPE, TOKEN_COLOR_TYPE:
		token := p.current()
		p.advance()

		// Check if this is object instantiation with <>
		if p.current().Type == TOKEN_LANGLE {
			p.advance()

			// Check if this has named properties (for instantiation)
			if (p.current().Type == TOKEN_IDENTIFIER || p.current().Type == TOKEN_STRING) &&
				p.peek(1).Type == TOKEN_ASSIGN {
				// Object instantiation with properties: vector2<"x": 10, "y": 20>
				obj := p.parseObjectLiteral()
				obj.Value = token.Value // Set the type name
				return obj
			}

			// Otherwise parse as comma-separated values: vector2<10,20>
			args := []*ASTNode{}
			for p.current().Type != TOKEN_RANGLE && p.current().Type != TOKEN_EOF {
				arg := p.parseAdditiveExpression() // Use additive to avoid consuming > as comparison
				args = append(args, arg)

				if p.current().Type == TOKEN_COMMA {
					p.advance()
				} else if p.current().Type != TOKEN_RANGLE {
					break
				}
			}
			p.expect(TOKEN_RANGLE)

			return &ASTNode{
				Type:     NODE_CALL,
				Value:    token.Value,
				Children: args,
				Line:     token.Line,
			}
		}

		// Check if this is a cast (followed by parenthesis)
		if p.current().Type != TOKEN_LPAREN {
			// Not a cast or instantiation - treat as unexpected
			errMsg := fmt.Sprintf("Unexpected type keyword '%s' at line %d:%d",
				token.Value, token.Line, token.Column)
			if p.LintMode {
				p.recordError(errMsg)
				p.advance()
				return &ASTNode{Type: NODE_IDENTIFIER, Value: "error"}
			} else {
				panic(errMsg)
			}
		}

		// It's a cast - parse the argument in parentheses
		p.advance() // consume opening (

		arg := p.parseExpression()

		p.expect(TOKEN_RPAREN)

		return &ASTNode{
			Type:     NODE_CALL,
			Value:    token.Value, // "int", "float", "char", or "string"
			Children: []*ASTNode{arg},
			Line:     token.Line,
		}

	default:
		current := p.current()
		errMsg := fmt.Sprintf("Unexpected token %s at line %d:%d",
			tokenTypeName(current.Type), current.Line, current.Column)
		if p.LintMode {
			p.recordError(errMsg)
			// Return a dummy node to continue parsing
			p.advance()
			return &ASTNode{
				Type:  NODE_IDENTIFIER,
				Value: "error",
			}
		} else {
			panic(errMsg)
		}
	}
}

func (p *Parser) parseArrayLiteral() *ASTNode {
	p.expect(TOKEN_LANGLE)

	// Check if this is an object literal by looking for "identifier:" or "string:"
	// Save position to restore if it's not an object
	if (p.current().Type == TOKEN_IDENTIFIER || p.current().Type == TOKEN_STRING) && p.peek(1).Type == TOKEN_ASSIGN {
		// This is an object literal, not an array
		return p.parseObjectLiteral()
	}

	array := &ASTNode{
		Type:     NODE_ARRAY_LITERAL,
		DataType: "array",
	}

	p.inArrayLiteral = true

	for p.current().Type != TOKEN_RANGLE {
		element := p.parseCallArgument() // Use call argument parser to avoid consuming >
		array.Children = append(array.Children, element)

		if p.current().Type == TOKEN_COMMA {
			p.advance()
		} else if p.current().Type != TOKEN_RANGLE {
			break
		}
	}

	p.inArrayLiteral = false
	p.expect(TOKEN_RANGLE)

	// Check for member access after array literal
	if p.current().Type == TOKEN_DOT {
		return p.parseMemberAccessChain(array)
	}

	return array
}

func (p *Parser) parseObjectLiteral() *ASTNode {
	// TOKEN_LANGLE already consumed by parseArrayLiteral

	object := &ASTNode{
		Type:     NODE_OBJECT_LITERAL,
		DataType: "object",
		Children: []*ASTNode{},
	}

	p.inObjectLiteral = true

	for p.current().Type != TOKEN_RANGLE && p.current().Type != TOKEN_EOF {
		// Parse property name (can be identifier or string)
		if p.current().Type != TOKEN_IDENTIFIER && p.current().Type != TOKEN_STRING {
			if p.LintMode {
				p.recordError(fmt.Sprintf("Expected property name at line %d", p.current().Line))
				p.advance()
				continue
			} else {
				panic(fmt.Sprintf("Expected property name, got %s at line %d",
					tokenTypeName(p.current().Type), p.current().Line))
			}
		}

		propName := p.current().Value
		p.advance()

		// Expect ':'
		if p.current().Type != TOKEN_ASSIGN {
			if p.LintMode {
				p.recordError(fmt.Sprintf("Expected ':' after property name at line %d", p.current().Line))
			} else {
				panic(fmt.Sprintf("Expected ':' after property name at line %d", p.current().Line))
			}
		}
		p.advance()

		// Parse property value - use parseAdditiveExpression to avoid comparison operators
		propValue := p.parseAdditiveExpression()

		// Create property node
		prop := &ASTNode{
			Type:     NODE_OBJECT_PROPERTY,
			Value:    propName,
			Children: []*ASTNode{propValue},
			Line:     propValue.Line,
		}
		object.Children = append(object.Children, prop)

		// Check for comma or end
		if p.current().Type == TOKEN_COMMA {
			p.advance()
		} else if p.current().Type != TOKEN_RANGLE {
			if p.LintMode {
				p.recordError(fmt.Sprintf("Expected ',' or '>' in object literal at line %d", p.current().Line))
				break
			} else {
				break
			}
		}
	}

	p.inObjectLiteral = false
	p.expect(TOKEN_RANGLE)

	// Check for member access after object literal
	if p.current().Type == TOKEN_DOT {
		return p.parseMemberAccessChain(object)
	}

	return object
}

func (p *Parser) parseObjectOrVector2Literal() *ASTNode {
	// Parse < ... > which could be vector2 or object literal
	p.expect(TOKEN_LANGLE)

	// Peek ahead to determine if it's a simple vector2 <x,y> or object literal
	savedPos := p.pos
	isVector2 := false

	// Check for simple vector2 pattern: <number,number>
	if p.current().Type == TOKEN_NUMBER {
		p.advance()
		if p.current().Type == TOKEN_COMMA {
			p.advance()
			if p.current().Type == TOKEN_NUMBER {
				p.advance()
				if p.current().Type == TOKEN_RANGLE {
					isVector2 = true
				}
			}
		}
	} else if p.current().Type == TOKEN_IDENTIFIER {
		// Could be <x:10,y:20> object literal or variable reference
		p.advance()
		if p.current().Type == TOKEN_ASSIGN {
			// It's an object literal with named properties <x:10>
			isVector2 = false
		} else if p.current().Type == TOKEN_COMMA {
			// Could be <var1,var2> - check next
			p.advance()
			if p.current().Type == TOKEN_IDENTIFIER {
				p.advance()
				if p.current().Type == TOKEN_RANGLE {
					// Simple <x,y> with identifiers - treat as vector2-like
					isVector2 = true
				}
			}
		}
	}

	// Restore position
	p.pos = savedPos

	if isVector2 {
		// Parse simple vector2: <number,number>
		x := p.expect(TOKEN_NUMBER)
		p.expect(TOKEN_COMMA)
		y := p.expect(TOKEN_NUMBER)
		p.expect(TOKEN_RANGLE)

		xNode := &ASTNode{Type: NODE_NUMBER, Value: x.Value, Line: x.Line}
		yNode := &ASTNode{Type: NODE_NUMBER, Value: y.Value, Line: y.Line}

		return &ASTNode{
			Type:     NODE_OBJECT_LITERAL,
			DataType: "vector2",
			Children: []*ASTNode{xNode, yNode},
			Line:     x.Line,
		}
	} else {
		// Parse as full object literal
		return p.parseObjectLiteral()
	}
}

func (p *Parser) parseDictLiteral() *ASTNode {
	p.expect(TOKEN_LBRACE)

	dict := &ASTNode{
		Type:     NODE_DICT_LITERAL,
		DataType: "dict",
	}

	p.inDictLiteral = true

	for p.current().Type != TOKEN_RBRACE {
		// Parse key (can be string or identifier)
		key := p.parseCallArgument()
		p.expect(TOKEN_ASSIGN) // Using : as separator between key and value
		value := p.parseCallArgument()

		// Store key-value pair as two consecutive children
		dict.Children = append(dict.Children, key, value)

		if p.current().Type == TOKEN_COMMA {
			p.advance()
		} else if p.current().Type != TOKEN_RBRACE {
			break
		}
	}

	p.inDictLiteral = false
	p.expect(TOKEN_RBRACE)

	// Check for member access after dict literal
	if p.current().Type == TOKEN_DOT {
		return p.parseMemberAccessChain(dict)
	}

	return dict
}

// Parse enum declaration
func (p *Parser) parseEnumDeclaration() *ASTNode {
	startLine := p.current().Line
	// Expect 'enum' keyword
	p.expect(TOKEN_ENUM)

	// Get the enum name
	var name Token
	if p.current().Type == TOKEN_IDENTIFIER ||
		p.current().Type == TOKEN_COLOR_TYPE ||
		p.current().Type == TOKEN_VECTOR2_TYPE ||
		p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
		p.current().Type == TOKEN_INT_TYPE ||
		p.current().Type == TOKEN_FLOAT_TYPE ||
		p.current().Type == TOKEN_STRING_TYPE ||
		p.current().Type == TOKEN_BOOL_TYPE {
		name = p.current()
		p.advance()
	} else {
		errMsg := fmt.Sprintf("Expected identifier for enum name at line %d", p.current().Line)
		if p.LintMode {
			p.recordError(errMsg)
			// Use a dummy name to continue parsing
			name = Token{Type: TOKEN_IDENTIFIER, Value: "error_enum", Line: p.current().Line}
		} else {
			panic(errMsg)
		}
	}

	p.expect(TOKEN_ASSIGN) // colon

	// Check if this is a one-line enum (no newline after colon)
	isOneLine := p.current().Type != TOKEN_NEWLINE && p.current().Type != TOKEN_INDENT

	if !isOneLine {
		p.skipNewlines()
		if p.current().Type == TOKEN_INDENT {
			p.advance()
		}
	}

	enum := &ASTNode{
		Type:  NODE_ENUM_DECLARATION,
		Value: name.Value,
		Line:  name.Line,
	}

	// Parse enum members - support multiple lines with optional values
	for p.current().Type == TOKEN_IDENTIFIER || p.current().Type == TOKEN_NUMBER {
		var value string
		var memberName string

		// Check if we have a number first (custom value)
		if p.current().Type == TOKEN_NUMBER {
			value = p.current().Value
			p.advance()

			// Now expect the identifier
			if p.current().Type != TOKEN_IDENTIFIER {
				errMsg := fmt.Sprintf("Expected identifier after enum value at line %d", p.current().Line)
				if p.LintMode {
					p.recordError(errMsg)
					break
				} else {
					panic(errMsg)
				}
			}
			memberName = p.current().Value
			p.advance()
		} else {
			// Just an identifier (auto-increment value)
			memberName = p.current().Value
			value = "" // Will be auto-assigned during codegen
			p.advance()
		}

		member := &ASTNode{
			Type:     NODE_IDENTIFIER,
			Value:    memberName,
			DataType: value, // Store the enum value in DataType field
			Line:     p.current().Line,
		}
		enum.Children = append(enum.Children, member)

		// Skip optional delimiters (comma, semicolon, or newline)
		for p.current().Type == TOKEN_COMMA || p.current().Type == TOKEN_SEMICOLON || p.current().Type == TOKEN_NEWLINE {
			p.advance()
		}

		// For one-line enums, stop at newline or EOF
		if isOneLine && (p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_EOF) {
			break
		}

		if !isOneLine {
			p.skipNewlines()
		}
	}

	if !isOneLine && p.current().Type == TOKEN_DEDENT {
		p.advance()
	}

	// Consume 'end' keyword only for multi-line enums
	if !isOneLine {
		if p.current().Type == TOKEN_END {
			p.advance()
		} else {
			errMsg := fmt.Sprintf("Expected '$' to close enum at line %d", startLine)
			if p.LintMode {
				p.recordErrorAtLine(errMsg, startLine)
			} else {
				panic(errMsg)
			}
		}
	}

	// Check for duplicate enum declaration
	if p.LintMode {
		if existing, exists := p.enums[name.Value]; exists {
			errMsg := fmt.Sprintf("Redeclaration of enum '%s' (previously declared at line %d)", name.Value, existing.Line)
			p.recordErrorAtLine(errMsg, startLine)
		}
		// Check if name conflicts with struct
		if existing, exists := p.structs[name.Value]; exists {
			errMsg := fmt.Sprintf("Redeclaration of '%s' as enum (previously declared as struct at line %d)", name.Value, existing.Line)
			p.recordErrorAtLine(errMsg, startLine)
		}
		// Check if name conflicts with function
		if existing, exists := p.functions[name.Value]; exists {
			errMsg := fmt.Sprintf("Redeclaration of '%s' as enum (previously declared as function at line %d)", name.Value, existing.Line)
			p.recordErrorAtLine(errMsg, startLine)
		}
		// Check if name conflicts with constant
		if existingLine, exists := p.constants[name.Value]; exists {
			errMsg := fmt.Sprintf("Redeclaration of '%s' as enum (previously declared as constant at line %d)", name.Value, existingLine)
			p.recordErrorAtLine(errMsg, startLine)
		}
	}

	// Always track enum members (needed for codegen and validation)
	p.enums[name.Value] = &EnumDefinition{
		Name:    name.Value,
		Members: enum.Children,
		Line:    startLine,
	}

	return enum
}

// Parse constant declaration (NAME :: value)
func (p *Parser) parseConstantDeclaration() *ASTNode {
	name := p.expect(TOKEN_IDENTIFIER)
	line := name.Line
	varName := name.Value
	p.expect(TOKEN_DOUBLE_COLON)

	// Check if this is a function declaration (has |)
	if p.current().Type == TOKEN_PIPE {
		return p.parseFunctionWithDoubleColon(name)
	}

	// In lint mode, check if constant is being redeclared
	if p.LintMode {
		if existingLine, exists := p.constants[varName]; exists {
			errMsg := fmt.Sprintf("Can't redeclare a constant declared on line %d",
				existingLine)
			p.recordError(errMsg)
		} else {
			// Register this constant
			p.constants[varName] = line
		}
	}

	// Check for type annotation (type=)
	var explicitType string
	if p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
		p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_BOOL_TYPE ||
		p.current().Type == TOKEN_COLOR_TYPE || p.current().Type == TOKEN_VECTOR2_TYPE ||
		p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
		p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
		p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
		p.current().Type == TOKEN_IDENTIFIER {

		// This might be a type annotation
		possibleType := p.current().Value

		// Look ahead to see if there's an = after the type
		if p.peek(1).Type == TOKEN_EQUALS {
			explicitType = possibleType
			p.advance() // consume type
			p.advance() // consume =
		}
	}

	// Regular constant
	value := p.parseExpression()

	// In lint mode, track the constant's type
	if p.LintMode {
		if explicitType != "" {
			p.variableTypes[varName] = explicitType
		} else {
			inferredType := p.inferType(value)
			if inferredType != "unknown" {
				p.variableTypes[varName] = inferredType
			}
		}
	}

	return &ASTNode{
		Type:     NODE_CONSTANT_DECLARATION,
		Value:    varName,
		DataType: explicitType,
		Line:     line,
		Children: []*ASTNode{value},
	}
}

// Parse function with :: syntax: name :: |params| type: body
func (p *Parser) parseFunctionWithDoubleColon(name Token) *ASTNode {
	startLine := name.Line
	fn := &ASTNode{
		Type:  NODE_FUNCTION,
		Value: name.Value,
		Line:  name.Line,
	}

	p.expect(TOKEN_PIPE)

	// Parameters
	params := &ASTNode{Type: NODE_BLOCK}
	hasDefaultParam := false // Track if we've seen a default parameter

	for p.current().Type != TOKEN_PIPE && p.current().Type != TOKEN_EOF {
		// Safety check: if current token is not an identifier, break to avoid infinite loop
		if p.current().Type != TOKEN_IDENTIFIER {
			break
		}

		paramName := p.expect(TOKEN_IDENTIFIER)

		var paramType string
		var defaultValue *ASTNode

		// Check for optional type annotation (after colon)
		if p.current().Type == TOKEN_ASSIGN { // :
			p.advance()

			// Type is optional - if not present, treat as generic
			if p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
				p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_BOOL_TYPE ||
				p.current().Type == TOKEN_COLOR_TYPE || p.current().Type == TOKEN_VECTOR2_TYPE ||
				p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
				p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
				p.current().Type == TOKEN_IDENTIFIER {
				paramType = p.current().Value
				p.advance()
			} else {
				// No type specified - generic parameter
				paramType = "generic"
			}
		} else {
			// No colon, no type - generic parameter
			paramType = "generic"
		}

		// Check for default value (= expression)
		if p.current().Type == TOKEN_EQUALS {
			p.advance()
			hasDefaultParam = true

			// Parse the default value expression
			defaultValue = p.parseExpression()
		} else {
			// Non-default parameter after default parameter is an error
			if hasDefaultParam {
				errMsg := fmt.Sprintf("Non-default parameter '%s' cannot follow default parameters at line %d",
					paramName.Value, paramName.Line)
				if p.LintMode {
					p.recordError(errMsg)
				} else {
					panic(errMsg)
				}
			}
		}

		param := &ASTNode{
			Type:         NODE_IDENTIFIER,
			Value:        paramName.Value,
			DataType:     paramType,
			DefaultValue: defaultValue,
		}
		params.Children = append(params.Children, param)

		// In lint mode, register parameters in function scope
		if p.LintMode {
			if p.functionScope == nil {
				p.functionScope = make(map[string]string)
			}
			p.functionScope[paramName.Value] = paramType
		}

		if p.current().Type == TOKEN_COMMA {
			p.advance()
		} else if p.current().Type != TOKEN_PIPE && p.current().Type != TOKEN_EOF {
			// If we're not at a comma, pipe, or EOF, something is wrong - break to avoid infinite loop
			break
		}
	}
	p.expect(TOKEN_PIPE)

	// Return type (optional, can be multiple types separated by comma)
	var returnType string
	if p.current().Type != TOKEN_ASSIGN {
		// Check for 'infer' keyword
		if p.current().Type == TOKEN_INFER {
			returnType = "infer"
			p.advance()
		} else if p.current().Type == TOKEN_VOID {
			returnType = "void"
			p.advance()
		} else {
			returnTypes := []string{}

			// Parse first return type
			if p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
				p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_BOOL_TYPE ||
				p.current().Type == TOKEN_COLOR_TYPE || p.current().Type == TOKEN_VECTOR2_TYPE ||
				p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
				p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
				p.current().Type == TOKEN_IDENTIFIER {
				returnTypes = append(returnTypes, p.current().Value)
				p.advance()

				// Parse additional return types (multiple returns)
				for p.current().Type == TOKEN_COMMA {
					p.advance()
					if p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
						p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_BOOL_TYPE ||
						p.current().Type == TOKEN_COLOR_TYPE || p.current().Type == TOKEN_VECTOR2_TYPE ||
						p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
						p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
						p.current().Type == TOKEN_IDENTIFIER {
						returnTypes = append(returnTypes, p.current().Value)
						p.advance()
					} else {
						break
					}
				}
			}

			// Join multiple return types with comma
			if len(returnTypes) > 0 {
				returnType = strings.Join(returnTypes, ",")
			}
		}
	}

	p.expect(TOKEN_ASSIGN) // :

	// Skip newline after :
	if p.current().Type == TOKEN_NEWLINE {
		p.advance()
	}

	// Expect indent for function body
	if p.current().Type == TOKEN_INDENT {
		p.advance()
	}

	// In lint mode, save the function return type and clear function scope
	var savedFunctionRet string
	var savedFunctionScope map[string]string
	if p.LintMode {
		savedFunctionRet = p.currentFunctionRet
		savedFunctionScope = p.functionScope
		p.currentFunctionRet = returnType
		// Function scope already set up with parameters above
	}

	// Parse body (function with :: syntax always uses '$')
	body := p.parseBlockUntilEnd("function", startLine)

	// parseBlockUntilEnd already consumes the '$' token

	// In lint mode, restore previous function context
	if p.LintMode {
		p.currentFunctionRet = savedFunctionRet
		p.functionScope = savedFunctionScope

		// Register function signature for later validation
		paramInfos := []ParameterInfo{}
		if len(fn.Children) > 0 && fn.Children[0] != nil {
			for _, paramNode := range fn.Children[0].Children {
				if paramNode != nil {
					paramInfos = append(paramInfos, ParameterInfo{
						Name: paramNode.Value,
						Type: paramNode.DataType,
					})
				}
			}
		}

		returnTypesList := []string{}
		isInfer := false
		if returnType == "infer" {
			isInfer = true
		} else if returnType != "" && returnType != "void" {
			returnTypesList = strings.Split(returnType, ",")
		}

		// Check for duplicate function declaration
		if existing, exists := p.functions[name.Value]; exists {
			errMsg := fmt.Sprintf("Redeclaration of function '%s' (previously declared at line %d)", name.Value, existing.Line)
			p.recordErrorAtLine(errMsg, name.Line)
		}
		// Check if name conflicts with struct
		if existing, exists := p.structs[name.Value]; exists {
			errMsg := fmt.Sprintf("Redeclaration of '%s' as function (previously declared as struct at line %d)", name.Value, existing.Line)
			p.recordErrorAtLine(errMsg, name.Line)
		}
		// Check if name conflicts with enum
		if existing, exists := p.enums[name.Value]; exists {
			errMsg := fmt.Sprintf("Redeclaration of '%s' as function (previously declared as enum at line %d)", name.Value, existing.Line)
			p.recordErrorAtLine(errMsg, name.Line)
		}
		// Check if name conflicts with constant
		if existingLine, exists := p.constants[name.Value]; exists {
			errMsg := fmt.Sprintf("Redeclaration of '%s' as function (previously declared as constant at line %d)", name.Value, existingLine)
			p.recordErrorAtLine(errMsg, name.Line)
		}
		
		p.functions[name.Value] = &FunctionSignature{
			Name:         name.Value,
			Parameters:   paramInfos,
			ReturnTypes:  returnTypesList,
			IsInfer:      isInfer,
			FunctionNode: fn,
			Line:         name.Line,
		}
	}

	fn.Children = append(fn.Children, params)
	fn.Children = append(fn.Children, body)
	fn.DataType = returnType

	return fn
}

// Parse tuple assignment (a, b : c, d)
func (p *Parser) parseTupleAssignment() *ASTNode {
	// Parse left side (identifiers with optional type annotations)
	leftSide := &ASTNode{Type: NODE_BLOCK}
	line := p.current().Line

	for {
		oldPos := p.pos
		name := p.expect(TOKEN_IDENTIFIER)

		// Create identifier node
		idNode := &ASTNode{
			Type:  NODE_IDENTIFIER,
			Value: name.Value,
			Line:  name.Line,
		}

		// Check for optional type annotation (identifier:type)
		if p.current().Type == TOKEN_ASSIGN {
			p.advance()
			// Check if this is a type annotation (but NOT a generic identifier, as that would consume the right side)
			if p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
				p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_BOOL_TYPE ||
				p.current().Type == TOKEN_COLOR_TYPE || p.current().Type == TOKEN_VECTOR2_TYPE ||
				p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE {
				idNode.DataType = p.current().Value
				p.advance()
			} else {
				// Not a type annotation, put back the colon
				p.pos--
			}
		}

		leftSide.Children = append(leftSide.Children, idNode)

		if p.current().Type == TOKEN_COMMA {
			p.advance()
		} else {
			break
		}

		// Safety check
		if p.pos == oldPos {
			break
		}
	}

	p.expect(TOKEN_ASSIGN)

	// Parse right side (expressions)
	// Use parsePrimaryExpression to avoid triggering assignment checks in parseExpression
	rightSide := &ASTNode{Type: NODE_BLOCK}

	for {
		oldPos := p.pos
		var expr *ASTNode

		// Directly parse primary expression to avoid assignment parsing issues
		expr = p.parsePrimaryExpression()

		rightSide.Children = append(rightSide.Children, expr)

		if p.current().Type == TOKEN_COMMA {
			p.advance()
		} else {
			break
		}

		// Safety check
		if p.pos == oldPos {
			break
		}
	}

	// Validate tuple assignment in lint mode
	if p.LintMode {
		p.validateTupleAssignment(leftSide, rightSide, line)
	}

	return &ASTNode{
		Type:     NODE_TUPLE_ASSIGNMENT,
		Line:     line,
		Children: []*ASTNode{leftSide, rightSide},
	}
}

// Parse struct declaration
func (p *Parser) parseStructDeclaration() *ASTNode {
	startLine := p.current().Line
	p.expect(TOKEN_STRUCT)
	name := p.expect(TOKEN_IDENTIFIER)
	p.expect(TOKEN_ASSIGN)

	// Check if this is a one-line struct (no newline after colon)
	isOneLine := p.current().Type != TOKEN_NEWLINE && p.current().Type != TOKEN_INDENT

	if !isOneLine {
		p.skipNewlines()
		if p.current().Type == TOKEN_INDENT {
			p.advance()
		}
	}

	struc := &ASTNode{
		Type:  NODE_STRUCT_DECLARATION,
		Value: name.Value,
		Line:  name.Line,
	}

	// Parse struct fields
	for p.current().Type == TOKEN_IDENTIFIER || p.current().Type == TOKEN_TYPE ||
		p.current().Type == TOKEN_NUMBER || p.current().Type == TOKEN_LANGLE {
		if p.current().Type == TOKEN_TYPE {
			// Nested type (e.g., "type smoke_particle:")
			p.advance() // consume 'type'
			typeName := p.expect(TOKEN_IDENTIFIER)
			p.expect(TOKEN_ASSIGN)

			// Create nested type node
			nestedType := &ASTNode{
				Type:  NODE_TYPE,
				Value: typeName.Value,
				Line:  typeName.Line,
			}

			p.skipNewlines()
			if p.current().Type == TOKEN_INDENT {
				p.advance()

				// Parse fields of nested type
				for p.current().Type != TOKEN_DEDENT && p.current().Type != TOKEN_EOF {
					if p.current().Type == TOKEN_NEWLINE {
						p.advance()
						continue
					}

					// If we encounter another 'type' keyword, we're done with this nested type
					if p.current().Type == TOKEN_TYPE {
						break
					}

					if p.current().Type == TOKEN_IDENTIFIER || p.current().Type == TOKEN_NUMBER ||
						p.current().Type == TOKEN_LANGLE || p.current().Type == TOKEN_STRING ||
						p.current().Type == TOKEN_TRUE || p.current().Type == TOKEN_FALSE ||
						p.current().Type == TOKEN_LBRACKET || p.current().Type == TOKEN_LBRACE {
						// Check for default value syntax: "value field: type"
						var defaultValue *ASTNode

						// Check if this might be a default value (number, vector2 literal, etc.)
						if p.current().Type == TOKEN_NUMBER {
							defaultValue = &ASTNode{
								Type:  NODE_NUMBER,
								Value: p.current().Value,
								Line:  p.current().Line,
							}
							p.advance()
						} else if p.current().Type == TOKEN_LANGLE {
							// Parse vector2 or object literal default value
							defaultValue = p.parseObjectOrVector2Literal()
						} else if p.current().Type == TOKEN_STRING {
							defaultValue = &ASTNode{
								Type:  NODE_STRING,
								Value: p.current().Value,
								Line:  p.current().Line,
							}
							p.advance()
						} else if p.current().Type == TOKEN_TRUE || p.current().Type == TOKEN_FALSE {
							defaultValue = &ASTNode{
								Type:  NODE_BOOLEAN,
								Value: p.current().Value,
								Line:  p.current().Line,
							}
							p.advance()
						} else if p.current().Type == TOKEN_LBRACKET {
							// Parse array literal default value
							defaultValue = p.parseArrayLiteralBracket()
						} else if p.current().Type == TOKEN_LBRACE {
							// Parse dict literal default value
							defaultValue = p.parseDictLiteral()
						}

						fieldName := p.expect(TOKEN_IDENTIFIER)
						p.expect(TOKEN_ASSIGN)

						// Get type
						fieldType := p.current().Value
						if p.current().Type == TOKEN_IDENTIFIER ||
							p.current().Type == TOKEN_INT_TYPE ||
							p.current().Type == TOKEN_FLOAT_TYPE ||
							p.current().Type == TOKEN_STRING_TYPE ||
							p.current().Type == TOKEN_BOOL_TYPE ||
							p.current().Type == TOKEN_VECTOR2_TYPE ||
							p.current().Type == TOKEN_DICT_TYPE ||
							p.current().Type == TOKEN_ARRAY_TYPE ||
							p.current().Type == TOKEN_COLOR_TYPE {
							p.advance()
						}

						field := &ASTNode{
							Type:         NODE_IDENTIFIER,
							Value:        fieldName.Value,
							DataType:     fieldType,
							Line:         fieldName.Line,
							DefaultValue: defaultValue,
						}
						nestedType.Children = append(nestedType.Children, field)

						p.skipNewlines()
					} else {
						// Unknown token, skip to avoid infinite loop
						p.advance()
					}
				}

				if p.current().Type == TOKEN_DEDENT {
					p.advance()
				}
			}

			struc.Children = append(struc.Children, nestedType)
		} else {
			// Regular field - check for default value syntax: "value field: type"
			var defaultValue *ASTNode

			// Check if this might be a default value (number, vector2 literal, etc.)
			if p.current().Type == TOKEN_NUMBER {
				defaultValue = &ASTNode{
					Type:  NODE_NUMBER,
					Value: p.current().Value,
					Line:  p.current().Line,
				}
				p.advance()
			} else if p.current().Type == TOKEN_LANGLE {
				// Parse vector2 or object literal default value
				defaultValue = p.parseObjectOrVector2Literal()
			} else if p.current().Type == TOKEN_STRING {
				defaultValue = &ASTNode{
					Type:  NODE_STRING,
					Value: p.current().Value,
					Line:  p.current().Line,
				}
				p.advance()
			} else if p.current().Type == TOKEN_TRUE || p.current().Type == TOKEN_FALSE {
				defaultValue = &ASTNode{
					Type:  NODE_BOOLEAN,
					Value: p.current().Value,
					Line:  p.current().Line,
				}
				p.advance()
			} else if p.current().Type == TOKEN_LBRACKET {
				// Parse array literal default value
				defaultValue = p.parseArrayLiteralBracket()
			} else if p.current().Type == TOKEN_LBRACE {
				// Parse dict literal default value
				defaultValue = p.parseDictLiteral()
			}

			fieldName := p.expect(TOKEN_IDENTIFIER)
			p.expect(TOKEN_ASSIGN)

			// Get type
			fieldType := p.current().Value
			if p.current().Type == TOKEN_IDENTIFIER ||
				p.current().Type == TOKEN_INT_TYPE ||
				p.current().Type == TOKEN_FLOAT_TYPE ||
				p.current().Type == TOKEN_STRING_TYPE ||
				p.current().Type == TOKEN_BOOL_TYPE ||
				p.current().Type == TOKEN_VECTOR2_TYPE ||
				p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
				p.current().Type == TOKEN_COLOR_TYPE {
				p.advance()
			}

			field := &ASTNode{
				Type:         NODE_IDENTIFIER,
				Value:        fieldName.Value,
				DataType:     fieldType,
				Line:         fieldName.Line,
				DefaultValue: defaultValue,
			}
			struc.Children = append(struc.Children, field)
		}

		// Skip optional delimiters (comma, semicolon, or newline)
		for p.current().Type == TOKEN_COMMA || p.current().Type == TOKEN_SEMICOLON || p.current().Type == TOKEN_NEWLINE {
			p.advance()
		}

		// For one-line structs, stop at newline or EOF
		if isOneLine && (p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_EOF) {
			break
		}

		if !isOneLine {
			p.skipNewlines()
		}
	}

	if !isOneLine && p.current().Type == TOKEN_DEDENT {
		p.advance()
	}

	// Consume 'end' keyword only for multi-line structs
	if !isOneLine {
		if p.current().Type == TOKEN_END {
			p.advance()
		} else {
			errMsg := fmt.Sprintf("Expected '$' to close struct at line %d", startLine)
			if p.LintMode {
				p.recordErrorAtLine(errMsg, startLine)
			} else {
				panic(errMsg)
			}
		}
	}

	// Store struct definition for linting
	if p.LintMode {
		p.storeStructDefinition(struc, startLine)
	}

	return struc
}

// Store struct definition for later validation
func (p *Parser) storeStructDefinition(struc *ASTNode, startLine int) {
	structName := struc.Value
	
	// Check for duplicate struct declaration
	if existing, exists := p.structs[structName]; exists {
		errMsg := fmt.Sprintf("Redeclaration of struct '%s' (previously declared at line %d)", structName, existing.Line)
		p.recordErrorAtLine(errMsg, startLine)
	}
	// Check if name conflicts with enum
	if existing, exists := p.enums[structName]; exists {
		errMsg := fmt.Sprintf("Redeclaration of '%s' as struct (previously declared as enum at line %d)", structName, existing.Line)
		p.recordErrorAtLine(errMsg, startLine)
	}
	// Check if name conflicts with function
	if existing, exists := p.functions[structName]; exists {
		errMsg := fmt.Sprintf("Redeclaration of '%s' as struct (previously declared as function at line %d)", structName, existing.Line)
		p.recordErrorAtLine(errMsg, startLine)
	}
	// Check if name conflicts with constant
	if existingLine, exists := p.constants[structName]; exists {
		errMsg := fmt.Sprintf("Redeclaration of '%s' as struct (previously declared as constant at line %d)", structName, existingLine)
		p.recordErrorAtLine(errMsg, startLine)
	}

	structDef := &StructDefinition{
		Name:   structName,
		Fields: []StructField{},
		Line:   startLine,
	}

	// Process all fields and nested types
	for _, child := range struc.Children {
		if child.Type == NODE_TYPE {
			// This is a nested type
			nestedName := child.Value
			nestedDef := &StructDefinition{
				Name:   nestedName,
				Parent: structName, // Track parent struct
				Fields: []StructField{},
				Line:   child.Line,
			}

			// Add parent struct fields to nested type
			for _, field := range structDef.Fields {
				nestedDef.Fields = append(nestedDef.Fields, field)
			}

			// Add nested type fields
			for _, field := range child.Children {
				nestedDef.Fields = append(nestedDef.Fields, StructField{
					Name:         field.Value,
					Type:         field.DataType,
					DefaultValue: field.DefaultValue,
				})
			}

			p.structs[nestedName] = nestedDef
		} else {
			// Regular field
			structDef.Fields = append(structDef.Fields, StructField{
				Name:         child.Value,
				Type:         child.DataType,
				DefaultValue: child.DefaultValue,
			})
		}
	}

	p.structs[structName] = structDef
}

// Track object literal properties for validation
func (p *Parser) trackObjectLiteralProperties(varName string, object *ASTNode) {
	if object.Type != NODE_OBJECT_LITERAL {
		return
	}

	props := make(map[string]bool)
	for _, prop := range object.Children {
		if prop.Type == NODE_OBJECT_PROPERTY {
			props[prop.Value] = true
		}
	}
	p.objectLiterals[varName] = props
}

// Validate struct initialization
func (p *Parser) validateStructInitialization(typeName string, value *ASTNode, line int) {
	if value.Type != NODE_OBJECT_LITERAL {
		return
	}

	// Check if it's a struct type
	structDef, ok := p.structs[typeName]
	if !ok {
		return // Not a struct type, could be regular object
	}

	// Validate each property in the initialization
	for _, prop := range value.Children {
		if prop.Type == NODE_OBJECT_PROPERTY {
			propName := prop.Value
			if !p.structHasField(typeName, propName) {
				errMsg := fmt.Sprintf("Invalid property: '%s' does not exist in type '%s' (line %d)",
					propName, structDef.Name, line)
				p.recordError(errMsg)
			}
		}
	}
}

// Validate property assignment
func (p *Parser) validatePropertyAssignment(target *ASTNode, value *ASTNode, line int) {
	if target.Type != NODE_MEMBER_ACCESS || len(target.Children) == 0 {
		return
	}

	// Get the root object
	root := target.Children[0]
	propertyName := target.Value

	varName := ""
	objectType := ""

	if root.Type == NODE_IDENTIFIER {
		varName = root.Value
		if vtype, ok := p.variableTypes[varName]; ok {
			objectType = vtype
		}
	}

	if objectType == "" {
		return
	}

	// Normalize struct type
	objectType = strings.TrimPrefix(objectType, "struct:")

	// Check if it's an object literal
	if objectType == "object" || objectType == "object_literal" {
		if props, ok := p.objectLiterals[varName]; ok {
			if !props[propertyName] {
				errMsg := fmt.Sprintf("object literal can't have new properties added at runtime (line %d)", line)
				p.recordError(errMsg)
			}
		}
		return
	}

	// Check if it's a struct type
	if structDef, ok := p.structs[objectType]; ok {
		if !p.structHasField(objectType, propertyName) {
			errMsg := fmt.Sprintf("Object property not found: %s does not exist on type %s (line %d)",
				propertyName, structDef.Name, line)
			p.recordError(errMsg)
		} else {
			// Validate type compatibility
			fieldType := p.getStructFieldType(objectType, propertyName)
			valueType := p.inferType(value)

			if !p.checkTypeCompatibility(fieldType, valueType) {
				errMsg := fmt.Sprintf("Type mismatch: %s:%s cannot be assigned %s value (line %d)",
					propertyName, fieldType, valueType, line)
				p.recordError(errMsg)
			}
		}
	}
}

// Get all fields for a struct type (including parent fields)
func (p *Parser) getStructFields(typeName string) []StructField {
	if structDef, ok := p.structs[typeName]; ok {
		return structDef.Fields
	}
	return nil
}

// Check if a struct has a field
func (p *Parser) structHasField(typeName, fieldName string) bool {
	fields := p.getStructFields(typeName)
	for _, field := range fields {
		if field.Name == fieldName {
			return true
		}
	}
	return false
}

// Get field type from struct
func (p *Parser) getStructFieldType(typeName, fieldName string) string {
	fields := p.getStructFields(typeName)
	for _, field := range fields {
		if field.Name == fieldName {
			return field.Type
		}
	}
	return ""
}

// Parse member access chain (obj.prop or obj.method||)
func (p *Parser) parseMemberAccessChain(object *ASTNode) *ASTNode {
	for p.current().Type == TOKEN_DOT {
		p.advance()
		member := p.expect(TOKEN_IDENTIFIER)

		// Check if this is a method call
		if p.current().Type == TOKEN_PIPE {
			p.advance()

			// Parse arguments
			args := &ASTNode{Type: NODE_BLOCK}
			if p.current().Type != TOKEN_PIPE {
				// Check if this is a lambda (param: expression)
				if p.isLambda() {
					lambda := p.parseLambda()
					args.Children = append(args.Children, lambda)
				} else {
					for {
						oldPos := p.pos
						arg := p.parseExpression()
						args.Children = append(args.Children, arg)

						if p.current().Type == TOKEN_COMMA {
							p.advance()
						} else {
							break
						}

						// Safety check
						if p.pos == oldPos {
							break
						}
					}
				}
			}
			p.expect(TOKEN_PIPE)

			object = &ASTNode{
				Type:     NODE_METHOD_CALL,
				Value:    member.Value,
				Line:     member.Line,
				Children: []*ASTNode{object, args},
			}
		} else {
			// Simple member access - validate in lint mode
			if p.LintMode {
				p.validateMemberAccess(object, member.Value, member.Line)
			}

			object = &ASTNode{
				Type:     NODE_MEMBER_ACCESS,
				Value:    member.Value,
				Line:     member.Line,
				Children: []*ASTNode{object},
			}
		}
	}

	return object
}

// Validate member access for struct types and object literals
func (p *Parser) validateMemberAccess(object *ASTNode, memberName string, line int) {
	// Get the type of the object
	objectType := ""
	varName := ""

	if object.Type == NODE_IDENTIFIER {
		varName = object.Value
		if vtype, ok := p.variableTypes[varName]; ok {
			objectType = vtype
		}

		// Check if it's an enum
		if enumDef, isEnum := p.enums[varName]; isEnum {
			// Validate enum member
			found := false
			for _, member := range enumDef.Members {
				if member.Value == memberName {
					found = true
					break
				}
			}
			if !found {
				errMsg := fmt.Sprintf("Field '%s' does not exist on enum '%s' (line %d)",
					memberName, varName, line)
				p.recordError(errMsg)
			}
			return
		}
	} else if object.Type == NODE_MEMBER_ACCESS {
		// For chained access, get the root variable
		root := object
		for root.Type == NODE_MEMBER_ACCESS && len(root.Children) > 0 {
			root = root.Children[0]
		}
		if root.Type == NODE_IDENTIFIER {
			varName = root.Value
			if vtype, ok := p.variableTypes[varName]; ok {
				objectType = vtype
			}
		}
	} else if object.Type == NODE_OBJECT_LITERAL {
		objectType = "object_literal"
		// For object literals, check if this is a known variable
		// We'll handle this when the literal is assigned to a variable
	}

	if objectType == "" {
		return // Can't validate without type info
	}

	// Normalize struct type
	objectType = strings.TrimPrefix(objectType, "struct:")

	// Check if it's a struct type
	if structDef, ok := p.structs[objectType]; ok {
		if !p.structHasField(objectType, memberName) {
			errMsg := fmt.Sprintf("Property not found: '%s' does not exist on type '%s' (line %d)",
				memberName, structDef.Name, line)
			p.recordError(errMsg)
		}
	} else if objectType == "object_literal" || strings.HasPrefix(objectType, "object") {
		// For object literals, check if the property was defined in the literal
		if props, ok := p.objectLiterals[varName]; ok {
			if !props[memberName] {
				errMsg := fmt.Sprintf("Property not found: '%s' does not exist on object literal (line %d)",
					memberName, line)
				p.recordError(errMsg)
			}
		}
	}
}

// Parse array literal with brackets [...]
func (p *Parser) parseArrayLiteralBracket() *ASTNode {
	p.expect(TOKEN_LBRACKET)

	array := &ASTNode{
		Type:     NODE_ARRAY_LITERAL,
		DataType: "array",
	}

	p.inArrayLiteral = true

	for p.current().Type != TOKEN_RBRACKET {
		element := p.parseExpression()
		array.Children = append(array.Children, element)

		if p.current().Type == TOKEN_COMMA {
			p.advance()
		} else if p.current().Type != TOKEN_RBRACKET {
			break
		}
	}

	p.inArrayLiteral = false
	p.expect(TOKEN_RBRACKET)

	// Check for member access after array literal
	if p.current().Type == TOKEN_DOT {
		return p.parseMemberAccessChain(array)
	}

	return array
}

// Check if the current position is a lambda expression (param: expr)
func (p *Parser) isLambda() bool {
	// Look ahead for pattern: IDENTIFIER ASSIGN expression
	if p.pos+1 < len(p.tokens) {
		// Save position
		saved := p.pos
		p.advance()

		// Check for colon (ASSIGN)
		isLambdaSyntax := p.current().Type == TOKEN_ASSIGN

		// Restore position
		p.pos = saved
		return isLambdaSyntax
	}
	return false
}

// Parse lambda expression: param: expression
func (p *Parser) parseLambda() *ASTNode {
	// Get parameter name
	param := p.expect(TOKEN_IDENTIFIER)

	// Expect colon
	p.expect(TOKEN_ASSIGN)

	// Parse expression until we hit PIPE
	expr := p.parseLambdaBody()

	return &ASTNode{
		Type:     NODE_LAMBDA,
		Value:    param.Value,
		Children: []*ASTNode{expr},
		Line:     param.Line,
	}
}

// Parse lambda body - stops at PIPE
func (p *Parser) parseLambdaBody() *ASTNode {
	// Save the inFunctionCall flag
	savedFlag := p.inFunctionCall
	p.inFunctionCall = true

	expr := p.parseOrExpression()

	p.inFunctionCall = savedFlag
	return expr
}

// validateTupleAssignment validates that tuple assignment types match
func (p *Parser) validateTupleAssignment(leftSide, rightSide *ASTNode, line int) {
	if leftSide == nil || rightSide == nil {
		return
	}

	// Check if right side is a single function call
	if len(rightSide.Children) == 1 && rightSide.Children[0].Type == NODE_CALL {
		callNode := rightSide.Children[0]
		funcName := callNode.Value

		// Look up function signature
		funcSig := p.functions[funcName]
		if funcSig == nil {
			// Function not found, try to register variables anyway
			for _, leftVar := range leftSide.Children {
				targetScope := p.variableTypes
				if p.currentFunctionRet != "" && p.functionScope != nil {
					targetScope = p.functionScope
				}
				if leftVar.DataType != "" {
					targetScope[leftVar.Value] = leftVar.DataType
				}
			}
			return
		}

		// Get argument types from the call
		argTypes := []string{}
		for _, arg := range callNode.Children {
			argTypes = append(argTypes, p.inferType(arg))
		}

		// Determine actual return types
		returnTypes := []string{}

		if funcSig.IsInfer {
			// Infer return types from function with inferred return
			returnTypes = p.inferReturnTypesFromFunction(funcSig, argTypes)
		} else if len(funcSig.ReturnTypes) > 0 {
			// Use declared return types, but substitute generic parameters
			returnTypes = p.substituteGenericTypes(funcSig, argTypes)
		}

		// Validate count matches
		if len(leftSide.Children) != len(returnTypes) {
			errMsg := fmt.Sprintf("Tuple assignment mismatch: expected %d values but function returns %d",
				len(leftSide.Children), len(returnTypes))
			p.recordError(errMsg)
			return
		}

		// Validate and register each variable
		for i, leftVar := range leftSide.Children {
			if i >= len(returnTypes) {
				break
			}

			expectedType := leftVar.DataType
			actualType := returnTypes[i]

			// If left side has type annotation, validate it matches
			if expectedType != "" && actualType != "" {
				if !p.checkTypeCompatibility(expectedType, actualType) {
					errMsg := fmt.Sprintf("Tuple assignment type mismatch at position %d: variable '%s' expects %s but got %s",
						i+1, leftVar.Value, expectedType, actualType)
					p.recordError(errMsg)
				}
			}

			// Register variable with its type
			varType := expectedType
			if varType == "" {
				varType = actualType
			}

			// Store in appropriate scope
			targetScope := p.variableTypes
			if p.currentFunctionRet != "" && p.functionScope != nil {
				targetScope = p.functionScope
			}
			targetScope[leftVar.Value] = varType
		}
	}
}

// substituteGenericTypes substitutes generic parameter types with actual argument types
func (p *Parser) substituteGenericTypes(funcSig *FunctionSignature, argTypes []string) []string {
	// Create a map of parameter name to actual type
	genericSubstitutions := make(map[string]string)

	for i, param := range funcSig.Parameters {
		if i < len(argTypes) {
			if param.Type == "generic" || param.Type == "" {
				genericSubstitutions[param.Name] = argTypes[i]
			}
		}
	}

	// Substitute in return types
	result := make([]string, len(funcSig.ReturnTypes))
	for i, retType := range funcSig.ReturnTypes {
		// Check if this return type is a parameter name (generic)
		if actualType, ok := genericSubstitutions[retType]; ok {
			result[i] = actualType
		} else {
			result[i] = retType
		}
	}

	return result
}

// inferReturnTypesFromFunction infers return types from a function with "infer" return type
func (p *Parser) inferReturnTypesFromFunction(funcSig *FunctionSignature, argTypes []string) []string {
	if funcSig.FunctionNode == nil || len(funcSig.FunctionNode.Children) < 2 {
		return []string{}
	}

	// Create substitutions for generic parameters
	genericSubstitutions := make(map[string]string)
	for i, param := range funcSig.Parameters {
		if i < len(argTypes) {
			if param.Type == "generic" || param.Type == "" {
				genericSubstitutions[param.Name] = argTypes[i]
			} else {
				genericSubstitutions[param.Name] = param.Type
			}
		}
	}

	// Find return statements in function body
	body := funcSig.FunctionNode.Children[1]
	returnTypes := p.findReturnTypes(body, genericSubstitutions)

	return returnTypes
}

// findReturnTypes finds return statement types in a function body
func (p *Parser) findReturnTypes(node *ASTNode, substitutions map[string]string) []string {
	if node == nil {
		return []string{}
	}

	if node.Type == NODE_RETURN_STATEMENT {
		// Found a return statement - infer types of returned expressions
		types := []string{}
		for _, child := range node.Children {
			typ := p.inferTypeWithSubstitutions(child, substitutions)
			types = append(types, typ)
		}
		return types
	}

	// Recursively search children
	for _, child := range node.Children {
		types := p.findReturnTypes(child, substitutions)
		if len(types) > 0 {
			return types
		}
	}

	return []string{}
}

// inferTypeWithSubstitutions infers type with generic parameter substitutions
func (p *Parser) inferTypeWithSubstitutions(node *ASTNode, substitutions map[string]string) string {
	if node == nil {
		return "unknown"
	}

	// If this is an identifier, check if it's a parameter with a substitution
	if node.Type == NODE_IDENTIFIER {
		if actualType, ok := substitutions[node.Value]; ok {
			return actualType
		}
	}

	// Otherwise use normal type inference
	return p.inferType(node)
}

// parseCallWithName parses a function call when the name has already been consumed
func (p *Parser) parseCallWithName(funcName Token) *ASTNode {
	// Expect opening pipe
	p.expect(TOKEN_PIPE)

	call := &ASTNode{
		Type:  NODE_CALL,
		Value: funcName.Value,
		Line:  funcName.Line,
	}

	// Set flag to prevent nested parsing issues
	p.inFunctionCall = true

	// Parse arguments until closing pipe
	for p.current().Type != TOKEN_PIPE && p.current().Type != TOKEN_NEWLINE && p.current().Type != TOKEN_EOF {
		arg := p.parseCallArgument()
		call.Children = append(call.Children, arg)

		if p.current().Type == TOKEN_COMMA {
			p.advance()
		} else {
			break
		}
	}

	// Consume closing pipe
	if p.current().Type == TOKEN_PIPE {
		p.advance()
	}

	p.inFunctionCall = false
	return call
}
