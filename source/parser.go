package main

import (
	"fmt"
	"strconv"
)

type NodeType int

const (
	NODE_PROGRAM NodeType = iota
	NODE_FUNCTION
	NODE_VARIABLE_DECLARATION
	NODE_ASSIGNMENT
	NODE_IF_STATEMENT
	NODE_SWITCH_STATEMENT
	NODE_SWITCH_CASE
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
	NODE_BREAK
	NODE_SKIP
	NODE_LAMBDA
)

type ASTNode struct {
	Type     NodeType
	Value    string
	Children []*ASTNode
	DataType string
	Line     int
}

type Parser struct {
	tokens         []Token
	pos            int
	inFunctionCall bool
	inArrayLiteral bool
	inDictLiteral  bool
}

func parse(tokens []Token) *ASTNode {
	parser := &Parser{tokens: tokens, pos: 0}
	return parser.parseProgram()
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

func (p *Parser) advance() {
	if p.pos < len(p.tokens) {
		p.pos++
	}
}

func (p *Parser) expect(tokenType TokenType) Token {
	if p.current().Type != tokenType {
		panic(fmt.Sprintf("Expected token type %d, got %d at line %d", tokenType, p.current().Type, p.current().Line))
	}
	token := p.current()
	p.advance()
	return token
}

func (p *Parser) parseProgram() *ASTNode {
	program := &ASTNode{Type: NODE_PROGRAM}

	for p.current().Type != TOKEN_EOF {
		if p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_SEMICOLON {
			p.advance()
			continue
		}
		stmt := p.parseStatement()
		if stmt != nil {
			program.Children = append(program.Children, stmt)
		}

		// After a statement, accept either newline or semicolon
		if p.current().Type == TOKEN_SEMICOLON {
			p.advance()
			// Continue to parse next statement on same line
		}
	}

	return program
}

func (p *Parser) parseStatement() *ASTNode {
	switch p.current().Type {
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
	case TOKEN_RETURN:
		return p.parseReturnStatement()
	case TOKEN_BREAK:
		p.advance()
		return &ASTNode{Type: NODE_BREAK, Line: p.current().Line}
	case TOKEN_SKIP:
		p.advance()
		return &ASTNode{Type: NODE_SKIP, Line: p.current().Line}
	case TOKEN_IMPORT:
		return p.parseImportStatement()
	case TOKEN_IDENTIFIER:
		// Check for enum declaration (name enum:)
		nextType := p.peek(1).Type
		if nextType == TOKEN_ENUM {
			return p.parseEnumDeclaration()
		}
		// Check for constant declaration (name ::)
		if nextType == TOKEN_DOUBLE_COLON {
			return p.parseConstantDeclaration()
		}
		// Check for tuple assignment (name, name :)
		if nextType == TOKEN_COMMA {
			return p.parseTupleAssignment()
		}
		return p.parseAssignmentOrExpression()
	case TOKEN_COLOR_TYPE, TOKEN_VECTOR2_TYPE:
		// These could be enum names
		if p.peek(1).Type == TOKEN_ENUM {
			return p.parseEnumDeclaration()
		}
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

	p.expect(TOKEN_PIPE)

	// Parameters
	params := &ASTNode{Type: NODE_BLOCK}
	for p.current().Type != TOKEN_PIPE {
		paramName := p.expect(TOKEN_IDENTIFIER)
		var paramType string

		// Check for type annotation
		if p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
			p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_BOOL_TYPE {
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

	// Return type
	var returnType string
	if p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
		p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_BOOL_TYPE {
		returnType = p.current().Value
		p.advance()
	}

	p.expect(TOKEN_THEN)
	p.expect(TOKEN_NEWLINE)
	p.expect(TOKEN_INDENT)

	body := p.parseBlock()

	fn.Children = append(fn.Children, params)
	fn.Children = append(fn.Children, body)
	fn.DataType = returnType

	return fn
}

func (p *Parser) parseIfStatement() *ASTNode {
	p.expect(TOKEN_IF)
	condition := p.parseExpression()
	p.expect(TOKEN_THEN)

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
		p.expect(TOKEN_NEWLINE)
		p.expect(TOKEN_INDENT)
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
		p.expect(TOKEN_THEN)

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
			p.expect(TOKEN_NEWLINE)
			p.expect(TOKEN_INDENT)
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
	p.expect(TOKEN_THEN)

	switchStmt := &ASTNode{
		Type:     NODE_SWITCH_STATEMENT,
		Children: []*ASTNode{expr}, // First child is the switch expression
	}

	// Parse cases: value:statement then value:statement then ...
	// Cases can be on the same line or on new lines
	for {
		// Skip newlines and semicolons
		for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_SEMICOLON {
			p.advance()
		}

		// Check for end of switch
		if p.current().Type == TOKEN_DEDENT || p.current().Type == TOKEN_EOF ||
			p.current().Type == TOKEN_IF || p.current().Type == TOKEN_LOOP ||
			p.current().Type == TOKEN_FUNC || p.current().Type == TOKEN_RETURN ||
			p.current().Type == TOKEN_SWITCH || p.current().Type == TOKEN_IDENTIFIER {
			break
		}

		// Check if we have a number or char (case value)
		if p.current().Type != TOKEN_NUMBER && p.current().Type != TOKEN_STRING &&
			p.current().Type != TOKEN_CHAR && p.current().Type != TOKEN_IDENTIFIER {
			break
		}

		// Parse case value - just the literal value
		caseValue := &ASTNode{}
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
		}

		p.expect(TOKEN_ASSIGN) // Expect :

		// Parse case body (single statement or expression)
		caseBody := p.parseStatement()
		if caseBody == nil {
			caseBody = p.parseExpression()
		}

		caseNode := &ASTNode{
			Type:     NODE_SWITCH_CASE,
			Children: []*ASTNode{caseValue, caseBody},
		}

		switchStmt.Children = append(switchStmt.Children, caseNode)

		// Check for "then" separator
		if p.current().Type == TOKEN_THEN {
			p.advance()
			// Continue to next case
		} else {
			// No more cases
			break
		}
	}

	return switchStmt
}

func (p *Parser) parseLoop() *ASTNode {
	p.expect(TOKEN_LOOP)

	// Check for colon (loop:number or loop:true)
	if p.current().Type == TOKEN_ASSIGN {
		p.advance() // consume ':'

		// Check what comes after the colon
		if p.current().Type == TOKEN_NUMBER {
			// Could be loop:10 then (count from 10) or loop:10 to 20 then (range)
			startNum := p.expect(TOKEN_NUMBER)

			if p.current().Type == TOKEN_TO {
				// Range loop: loop:10 to 20 then
				p.advance() // consume 'to'
				endNum := p.expect(TOKEN_NUMBER)
				p.expect(TOKEN_DO)
				p.expect(TOKEN_NEWLINE)
				p.expect(TOKEN_INDENT)
				body := p.parseBlock()

				return &ASTNode{
					Type:     NODE_FOR_RANGE_LOOP,
					Value:    startNum.Value,
					DataType: endNum.Value, // Store end value in DataType field
					Children: []*ASTNode{body},
				}
			} else {
				// Count loop: loop:10 then (starts at 10)
				p.expect(TOKEN_DO)
				p.expect(TOKEN_NEWLINE)
				p.expect(TOKEN_INDENT)
				body := p.parseBlock()

				return &ASTNode{
					Type:     NODE_FOR_COUNT_LOOP,
					Value:    startNum.Value,
					Children: []*ASTNode{body},
				}
			}
		} else if p.current().Type == TOKEN_TRUE || p.current().Type == TOKEN_FALSE {
			// While loop: loop:true then
			condition := p.parseExpression()
			p.expect(TOKEN_DO)
			p.expect(TOKEN_NEWLINE)
			p.expect(TOKEN_INDENT)
			body := p.parseBlock()

			return &ASTNode{
				Type:     NODE_WHILE_LOOP,
				Children: []*ASTNode{condition, body},
			}
		} else {
			// Expression after colon (e.g., loop:x < 10 then)
			condition := p.parseExpression()

			// Check for 'to' keyword (range with variable start)
			if p.current().Type == TOKEN_TO {
				p.advance() // consume 'to'
				endExpr := p.parseExpression()
				p.expect(TOKEN_DO)
				p.expect(TOKEN_NEWLINE)
				p.expect(TOKEN_INDENT)
				body := p.parseBlock()

				return &ASTNode{
					Type:     NODE_FOR_RANGE_LOOP,
					Children: []*ASTNode{condition, endExpr, body},
				}
			}

			p.expect(TOKEN_DO)
			p.expect(TOKEN_NEWLINE)
			p.expect(TOKEN_INDENT)
			body := p.parseBlock()

			return &ASTNode{
				Type:     NODE_WHILE_LOOP,
				Children: []*ASTNode{condition, body},
			}
		}
	} else if p.current().Type == TOKEN_IDENTIFIER {
		// Could be loop element in array or loop key,value in dict
		firstIdent := p.expect(TOKEN_IDENTIFIER)

		if p.current().Type == TOKEN_COMMA {
			// loop key,value in dict
			p.advance() // consume ','
			secondIdent := p.expect(TOKEN_IDENTIFIER)
			p.expect(TOKEN_IN)
			dictExpr := p.parseExpression()
			p.expect(TOKEN_DO)
			p.expect(TOKEN_NEWLINE)
			p.expect(TOKEN_INDENT)
			body := p.parseBlock()

			keyNode := &ASTNode{Type: NODE_IDENTIFIER, Value: firstIdent.Value}
			valueNode := &ASTNode{Type: NODE_IDENTIFIER, Value: secondIdent.Value}

			return &ASTNode{
				Type:     NODE_FOR_IN_DICT_LOOP,
				Children: []*ASTNode{keyNode, valueNode, dictExpr, body},
			}
		} else if p.current().Type == TOKEN_IN {
			// loop element in array
			p.advance() // consume 'in'
			arrayExpr := p.parseExpression()
			p.expect(TOKEN_DO)
			p.expect(TOKEN_NEWLINE)
			p.expect(TOKEN_INDENT)
			body := p.parseBlock()

			elementNode := &ASTNode{Type: NODE_IDENTIFIER, Value: firstIdent.Value}

			return &ASTNode{
				Type:     NODE_FOR_IN_ARRAY_LOOP,
				Children: []*ASTNode{elementNode, arrayExpr, body},
			}
		} else {
			// Old style: loop condition then (where condition is an expression starting with identifier)
			// Put the identifier back into an expression context
			identNode := &ASTNode{Type: NODE_IDENTIFIER, Value: firstIdent.Value, Line: firstIdent.Line}

			// Continue parsing the rest of the expression
			condition := p.parseExpressionContinuation(identNode)
			p.expect(TOKEN_DO)
			p.expect(TOKEN_NEWLINE)
			p.expect(TOKEN_INDENT)
			body := p.parseBlock()

			return &ASTNode{
				Type:     NODE_WHILE_LOOP,
				Children: []*ASTNode{condition, body},
			}
		}
	} else {
		// Default: loop then (infinite loop, same as loop:0 then)
		p.expect(TOKEN_DO)
		p.expect(TOKEN_NEWLINE)
		p.expect(TOKEN_INDENT)
		body := p.parseBlock()

		return &ASTNode{
			Type:     NODE_FOR_COUNT_LOOP,
			Value:    "0",
			Children: []*ASTNode{body},
		}
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

func (p *Parser) parseReturnStatement() *ASTNode {
	p.expect(TOKEN_RETURN)

	ret := &ASTNode{Type: NODE_RETURN_STATEMENT}

	if p.current().Type != TOKEN_NEWLINE {
		expr := p.parseExpression()
		ret.Children = append(ret.Children, expr)
	}

	return ret
}

func (p *Parser) parseImportStatement() *ASTNode {
	p.expect(TOKEN_IMPORT)
	name := p.expect(TOKEN_STRING)

	return &ASTNode{
		Type:  NODE_IMPORT_STATEMENT,
		Value: name.Value,
	}
}

func (p *Parser) parseAssignmentOrExpression() *ASTNode {
	if p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Type == TOKEN_ASSIGN {
		// Assignment
		name := p.expect(TOKEN_IDENTIFIER)
		p.expect(TOKEN_ASSIGN)
		value := p.parseExpression()

		return &ASTNode{
			Type:     NODE_ASSIGNMENT,
			Value:    name.Value,
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
	return p.parseOrExpression()
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
		return &ASTNode{
			Type:     NODE_STRING,
			Value:    token.Value,
			DataType: "string",
			Line:     token.Line,
		}

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

	default:
		panic(fmt.Sprintf("Unexpected token: %d at line %d", p.current().Type, p.current().Line))
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
	// First token is the enum name (might be identifier or type keyword being repurposed)
	var name Token
	if p.current().Type == TOKEN_IDENTIFIER ||
		p.current().Type == TOKEN_COLOR_TYPE ||
		p.current().Type == TOKEN_VECTOR2_TYPE ||
		p.current().Type == TOKEN_INT_TYPE ||
		p.current().Type == TOKEN_FLOAT_TYPE ||
		p.current().Type == TOKEN_STRING_TYPE ||
		p.current().Type == TOKEN_BOOL_TYPE {
		name = p.current()
		p.advance()
	} else {
		panic(fmt.Sprintf("Expected identifier for enum name at line %d", p.current().Line))
	}

	p.expect(TOKEN_ENUM)
	p.expect(TOKEN_ASSIGN)

	p.skipNewlines()
	if p.current().Type == TOKEN_INDENT {
		p.advance()
	}

	enum := &ASTNode{
		Type:  NODE_ENUM_DECLARATION,
		Value: name.Value,
		Line:  name.Line,
	}

	// Parse enum members
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

	value := p.parseExpression()

	return &ASTNode{
		Type:     NODE_CONSTANT_DECLARATION,
		Value:    name.Value,
		Line:     name.Line,
		Children: []*ASTNode{value},
	}
}

// Parse tuple assignment (a, b : c, d)
func (p *Parser) parseTupleAssignment() *ASTNode {
	// Parse left side (identifiers)
	leftSide := &ASTNode{Type: NODE_BLOCK}

	for {
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
	}

	p.expect(TOKEN_ASSIGN)

	// Parse right side (expressions)
	rightSide := &ASTNode{Type: NODE_BLOCK}

	for {
		expr := p.parseExpression()
		rightSide.Children = append(rightSide.Children, expr)

		if p.current().Type == TOKEN_COMMA {
			p.advance()
		} else {
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
			// Nested type - skip for now
			p.advance()
			p.expect(TOKEN_IDENTIFIER)
			p.expect(TOKEN_ASSIGN)
			p.skipNewlines()
			if p.current().Type == TOKEN_INDENT {
				p.advance()
				for p.current().Type != TOKEN_DEDENT && p.current().Type != TOKEN_EOF {
					p.advance()
				}
				if p.current().Type == TOKEN_DEDENT {
					p.advance()
				}
			}
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
						arg := p.parseExpression()
						args.Children = append(args.Children, arg)

						if p.current().Type == TOKEN_COMMA {
							p.advance()
						} else {
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
