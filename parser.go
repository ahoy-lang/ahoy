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

type Parser struct {
	tokens         []Token
	pos            int
	inFunctionCall bool
	inArrayLiteral bool
	inDictLiteral  bool
	LintMode       bool
	Errors         []ParseError
}

func Parse(tokens []Token) *ASTNode {
	parser := &Parser{tokens: tokens, pos: 0, LintMode: false, Errors: []ParseError{}}
	return parser.parseProgram()
}

func ParseLint(tokens []Token) (*ASTNode, []ParseError) {
	parser := &Parser{tokens: tokens, pos: 0, LintMode: true, Errors: []ParseError{}}
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
		TOKEN_FROM: "'from'", TOKEN_TILL: "'till'", TOKEN_FUNC: "'func'",
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
	p.expect(TOKEN_IF)
	condition := p.parseExpression()

	// Accept either 'then' or 'do'
	if p.current().Type == TOKEN_THEN {
		p.advance()
	} else if p.current().Type == TOKEN_DO {
		p.advance()
	} else {
		current := p.current()
		errMsg := fmt.Sprintf("Expected 'then' or 'do', got %s at line %d:%d", 
			tokenTypeName(current.Type), current.Line, current.Column)
		if p.LintMode {
			p.recordError(errMsg)
		} else {
			panic(errMsg)
		}
	}

	// Skip optional whitespace/indent after keyword
	p.skipWhitespace()

	// Check for inline if statement (no newline after then)
	var ifBody *ASTNode
	if p.current().Type != TOKEN_NEWLINE {
		// Inline: parse single statement
		ifBody = &ASTNode{Type: NODE_BLOCK}
		stmt := p.parseStatement()
		if stmt != nil {
			ifBody.Children = append(ifBody.Children, stmt)
		}
	} else {
		// Multi-line: parse block
		p.advance() // skip newline
		p.skipWhitespace()
		ifBody = p.parseBlock()
	}

	ifStmt := &ASTNode{
		Type:     NODE_IF_STATEMENT,
		Children: []*ASTNode{condition, ifBody},
	}

	// Handle elseif/anif chains
	for p.current().Type == TOKEN_ELSEIF || p.current().Type == TOKEN_ANIF {
		p.advance()
		elseifCondition := p.parseExpression()

		// Accept either 'then' or 'do'
		if p.current().Type == TOKEN_THEN {
			p.advance()
		} else if p.current().Type == TOKEN_DO {
			p.advance()
		} else {
			current := p.current()
			errMsg := fmt.Sprintf("Expected 'then' or 'do', got %s at line %d:%d", 
				tokenTypeName(current.Type), current.Line, current.Column)
			if p.LintMode {
				p.recordError(errMsg)
			} else {
				panic(errMsg)
			}
		}

		// Skip optional whitespace/indent after keyword
		p.skipWhitespace()

		// Check for inline elseif/anif
		var elseifBody *ASTNode
		if p.current().Type != TOKEN_NEWLINE {
			// Inline
			elseifBody = &ASTNode{Type: NODE_BLOCK}
			stmt := p.parseStatement()
			if stmt != nil {
				elseifBody.Children = append(elseifBody.Children, stmt)
			}
		} else {
			// Multi-line
			p.advance() // skip newline
			p.skipWhitespace()
			elseifBody = p.parseBlock()
		}

		// Add elseif as another condition-body pair
		ifStmt.Children = append(ifStmt.Children, elseifCondition, elseifBody)
	}

	// Handle else (no "then" after else)
	if p.current().Type == TOKEN_ELSE {
		p.advance()

		// Check for inline else
		var elseBody *ASTNode
		if p.current().Type != TOKEN_NEWLINE {
			// Inline
			elseBody = &ASTNode{Type: NODE_BLOCK}
			stmt := p.parseStatement()
			if stmt != nil {
				elseBody.Children = append(elseBody.Children, stmt)
			}
		} else {
			// Multi-line
			p.expect(TOKEN_NEWLINE)
			p.expect(TOKEN_INDENT)
			elseBody = p.parseBlock()
		}
		ifStmt.Children = append(ifStmt.Children, elseBody)
	}

	return ifStmt
}

func (p *Parser) parseSwitchStatement() *ASTNode {
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
	for {
		// Skip newlines and semicolons
		for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_SEMICOLON {
			p.advance()
		}

		// Check for end of switch
		if p.current().Type == TOKEN_DEDENT || p.current().Type == TOKEN_EOF ||
			p.current().Type == TOKEN_IF || p.current().Type == TOKEN_LOOP ||
			p.current().Type == TOKEN_FUNC || p.current().Type == TOKEN_RETURN ||
			p.current().Type == TOKEN_SWITCH {
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

	return switchStmt
}

func (p *Parser) parseLoop() *ASTNode {
	p.expect(TOKEN_LOOP)

	var loopVar *Token = nil
	
	// Check for optional loop variable: loop i ...
	if p.current().Type == TOKEN_IDENTIFIER {
		ident := p.current()
		loopVar = &ident
		p.advance()
	}

	// Now check what follows
	if p.current().Type == TOKEN_FROM {
		// loop [i] from:start to end
		p.advance() // consume 'from'
		p.expect(TOKEN_ASSIGN) // expect ':'
		
		startExpr := p.parseExpression()
		p.expect(TOKEN_TO)
		endExpr := p.parseExpression()
		p.expect(TOKEN_DO)
		p.skipWhitespace()
		body := p.parseBlock()

		if loopVar != nil {
			// loop i from:start to end - i is local variable
			loopVarNode := &ASTNode{Type: NODE_IDENTIFIER, Value: loopVar.Value}
			return &ASTNode{
				Type:     NODE_FOR_RANGE_LOOP,
				Children: []*ASTNode{loopVarNode, startExpr, endExpr, body},
			}
		} else {
			// loop from:start to end - no local variable, just count iterations
			return &ASTNode{
				Type:     NODE_FOR_RANGE_LOOP,
				Children: []*ASTNode{startExpr, endExpr, body},
			}
		}
	} else if p.current().Type == TOKEN_TILL {
		// loop [i] till condition
		p.advance() // consume 'till'
		condition := p.parseExpression()
		p.expect(TOKEN_DO)
		p.skipWhitespace()
		body := p.parseBlock()

		if loopVar != nil {
			// loop i till condition - i should be initialized to 0 locally
			loopVarNode := &ASTNode{Type: NODE_IDENTIFIER, Value: loopVar.Value}
			return &ASTNode{
				Type:     NODE_WHILE_LOOP,
				Children: []*ASTNode{loopVarNode, condition, body},
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
		p.expect(TOKEN_DO)
		p.skipWhitespace()
		body := p.parseBlock()

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
		p.expect(TOKEN_DO)
		p.skipWhitespace()
		body := p.parseBlock()

		keyNode := &ASTNode{Type: NODE_IDENTIFIER, Value: loopVar.Value}
		valueNode := &ASTNode{Type: NODE_IDENTIFIER, Value: secondIdent.Value}
		return &ASTNode{
			Type:     NODE_FOR_IN_DICT_LOOP,
			Children: []*ASTNode{keyNode, valueNode, dictExpr, body},
		}
	} else if p.current().Type == TOKEN_DO {
		// loop [i] do - infinite loop, optionally with counter
		p.advance() // consume 'do'
		p.skipWhitespace()
		body := p.parseBlock()

		if loopVar != nil {
			// loop i do - i starts at 0, increments each iteration
			loopVarNode := &ASTNode{Type: NODE_IDENTIFIER, Value: loopVar.Value}
			return &ASTNode{
				Type:     NODE_FOR_COUNT_LOOP,
				Value:    loopVar.Value,
				Children: []*ASTNode{loopVarNode, body},
			}
		} else {
			// loop do - infinite loop without counter
			return &ASTNode{
				Type:     NODE_FOR_COUNT_LOOP,
				Value:    "0",
				Children: []*ASTNode{body},
			}
		}
	} else if p.current().Type == TOKEN_ASSIGN {
		// ERROR: loop: or loop i: is invalid - colon should only come after 'from'
		if !p.LintMode {
			panic(fmt.Sprintf("Unexpected ':' after loop%s at line %d. Did you mean 'from:'?", 
				func() string { if loopVar != nil { return " " + loopVar.Value } else { return "" } }(), 
				p.current().Line))
		}
		p.recordError("Unexpected ':' - colon should only follow 'from' keyword")
		return &ASTNode{Type: NODE_WHILE_LOOP, Children: []*ASTNode{}}
	} else {
		// Unexpected token
		if !p.LintMode {
			panic(fmt.Sprintf("Expected 'from', 'till', 'in', or 'do' after loop%s at line %d", 
				func() string { if loopVar != nil { return " " + loopVar.Value } else { return "" } }(), 
				p.current().Line))
		}
		p.recordError("Expected 'from', 'till', 'in', or 'do' after loop")
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
	p.expect(TOKEN_RETURN)

	ret := &ASTNode{Type: NODE_RETURN_STATEMENT}

	if p.current().Type != TOKEN_NEWLINE {
		expr := p.parseExpression()
		ret.Children = append(ret.Children, expr)
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
	
	// Parse the deferred statement (typically a function call)
	statement := p.parseExpression()
	
	return &ASTNode{
		Type:     NODE_DEFER_STATEMENT,
		Line:     deferToken.Line,
		Children: []*ASTNode{statement},
	}
}

func (p *Parser) parseImportStatement() *ASTNode {
	p.expect(TOKEN_IMPORT)
	
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

	return &ASTNode{
		Type:  NODE_IMPORT_STATEMENT,
		Value: path,
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
	if p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Type == TOKEN_ASSIGN {
		// Assignment with possible type annotation
		name := p.expect(TOKEN_IDENTIFIER)
		p.expect(TOKEN_ASSIGN)
		
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
		
		value := p.parseExpression()

		return &ASTNode{
			Type:     NODE_ASSIGNMENT,
			Value:    name.Value,
			DataType: explicitType,
			Children: []*ASTNode{value},
			Line:     name.Line,
		}
	}

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
			node := &ASTNode{
				Type:     NODE_ARRAY_ACCESS,
				Value:    token.Value,
				Children: []*ASTNode{index},
				Line:     token.Line,
			}
			// Check for member access after array access
			if p.current().Type == TOKEN_DOT {
				return p.parseMemberAccessChain(node)
			}
			return node
		}

		// Check for old-style array access identifier<index>
		if p.current().Type == TOKEN_LANGLE {
			p.advance()
			index := p.parseCallArgument()
			p.expect(TOKEN_RANGLE)
			node := &ASTNode{
				Type:     NODE_ARRAY_ACCESS,
				Value:    token.Value,
				Children: []*ASTNode{index},
				Line:     token.Line,
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

	p.skipNewlines()
	if p.current().Type == TOKEN_INDENT {
		p.advance()
	}

	enum := &ASTNode{
		Type:  NODE_ENUM_DECLARATION,
		Value: name.Value,
		Line:  name.Line,
	}

	// Parse enum members - support multiple lines
	for p.current().Type == TOKEN_IDENTIFIER {
		member := &ASTNode{
			Type:  NODE_IDENTIFIER,
			Value: p.current().Value,
			Line:  p.current().Line,
		}
		enum.Children = append(enum.Children, member)
		p.advance()
		p.skipNewlines()
	}

	if p.current().Type == TOKEN_DEDENT {
		p.advance()
	}

	return enum
}

// Parse constant declaration (NAME :: value)
func (p *Parser) parseConstantDeclaration() *ASTNode {
	name := p.expect(TOKEN_IDENTIFIER)
	p.expect(TOKEN_DOUBLE_COLON)

	// Check if this is a function declaration (has |)
	if p.current().Type == TOKEN_PIPE {
		return p.parseFunctionWithDoubleColon(name)
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

	return &ASTNode{
		Type:     NODE_CONSTANT_DECLARATION,
		Value:    name.Value,
		DataType: explicitType,
		Line:     name.Line,
		Children: []*ASTNode{value},
	}
}

// Parse function with :: syntax: name :: |params| type: body
func (p *Parser) parseFunctionWithDoubleColon(name Token) *ASTNode {
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

		if p.current().Type == TOKEN_COMMA {
			p.advance()
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

	// Parse body
	body := p.parseBlock()

	fn.Children = append(fn.Children, params)
	fn.Children = append(fn.Children, body)
	fn.DataType = returnType

	return fn
}

// Parse tuple assignment (a, b : c, d)
func (p *Parser) parseTupleAssignment() *ASTNode {
	// Parse left side (identifiers)
	leftSide := &ASTNode{Type: NODE_BLOCK}

	for {
		oldPos := p.pos
		name := p.expect(TOKEN_IDENTIFIER)
		leftSide.Children = append(leftSide.Children, &ASTNode{
			Type:  NODE_IDENTIFIER,
			Value: name.Value,
			Line:  name.Line,
		})

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
	rightSide := &ASTNode{Type: NODE_BLOCK}

	for {
		oldPos := p.pos
		expr := p.parseExpression()
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

	return &ASTNode{
		Type:     NODE_TUPLE_ASSIGNMENT,
		Line:     leftSide.Children[0].Line,
		Children: []*ASTNode{leftSide, rightSide},
	}
}

// Parse struct declaration
func (p *Parser) parseStructDeclaration() *ASTNode {
	p.expect(TOKEN_STRUCT)
	name := p.expect(TOKEN_IDENTIFIER)
	p.expect(TOKEN_ASSIGN)

	p.skipNewlines()
	if p.current().Type == TOKEN_INDENT {
		p.advance()
	}

	struc := &ASTNode{
		Type:  NODE_STRUCT_DECLARATION,
		Value: name.Value,
		Line:  name.Line,
	}

	// Parse struct fields
	for p.current().Type == TOKEN_IDENTIFIER || p.current().Type == TOKEN_TYPE {
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
					if p.current().Type == TOKEN_IDENTIFIER {
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
							Type:     NODE_IDENTIFIER,
							Value:    fieldName.Value,
							DataType: fieldType,
							Line:     fieldName.Line,
						}
						nestedType.Children = append(nestedType.Children, field)
					}
					
					p.skipNewlines()
				}
				
				if p.current().Type == TOKEN_DEDENT {
					p.advance()
				}
			}
			
			struc.Children = append(struc.Children, nestedType)
		} else {
			// Regular field
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
				Type:     NODE_IDENTIFIER,
				Value:    fieldName.Value,
				DataType: fieldType,
				Line:     fieldName.Line,
			}
			struc.Children = append(struc.Children, field)
		}

		p.skipNewlines()
	}

	if p.current().Type == TOKEN_DEDENT {
		p.advance()
	}

	return struc
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
			// Simple member access
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
	if p.current().Type == TOKEN_IDENTIFIER {
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
		Value:    param.Value,      // Parameter name
		Children: []*ASTNode{expr}, // Lambda body
		Line:     param.Line,
	}
}

// Parse lambda body - stops at PIPE
func (p *Parser) parseLambdaBody() *ASTNode {
	// Save the inFunctionCall flag
	savedFlag := p.inFunctionCall
	p.inFunctionCall = true // Prevent parseExpression from consuming PIPE

	expr := p.parseOrExpression()

	p.inFunctionCall = savedFlag
	return expr
}
