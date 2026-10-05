package ahoy

import (
	"fmt"
	"strconv"
)

// Primary expressions and literals: arrays, objects, dicts, lambdas.

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

	case TOKEN_RAW_STRING:
		token := p.current()
		p.advance()
		node := &ASTNode{
			Type:     NODE_RAW_STRING,
			Value:    token.Value,
			DataType: "string",
			Line:     token.Line,
		}
		// Check for method call on raw string literal
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

			// Check for chained array access (2D arrays): grid[r][c]
			for p.current().Type == TOKEN_LBRACKET {
				p.advance()
				nextIndex := p.parseExpression()
				p.expect(TOKEN_RBRACKET)
				// Wrap the previous node as a chained array access
				// Children[0] = previous array access, Children[1] = new index
				node = &ASTNode{
					Type:     NODE_ARRAY_ACCESS,
					Value:    "", // Empty value indicates chained access
					Children: []*ASTNode{node, nextIndex},
					Line:     token.Line,
				}
			}

			// Check for member access after array access
			if p.current().Type == TOKEN_DOT {
				return p.parseMemberAccessChain(node)
			}
			return node
		}

		// Check for object instantiation identifier{...} or object property access identifier{'key'}
		if p.current().Type == TOKEN_LBRACE {
			p.advance()

			// Skip any newlines and indents after opening brace (for multiline objects)
			for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT {
				p.advance()
			}

			// Check for empty object instantiation identifier{}
			if p.current().Type == TOKEN_RBRACE {
				p.advance() // consume }
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
				obj := p.parseObjectLiteral() // Will consume the closing }
				obj.Value = token.Value       // Set the type name
				return obj
			}

			// Otherwise, it's object property access: obj{'key'}
			accessor := p.parseCallArgument()
			p.expect(TOKEN_RBRACE)

			// Object property access uses string literals: obj{'prop'}
			nodeType := NODE_OBJECT_ACCESS

			// Validate access syntax in lint mode
			if p.LintMode {
				if varType, ok := p.variableTypes[token.Value]; ok {
					if varType == "array" {
						errMsg := fmt.Sprintf("Invalid array access syntax, use array[] instead of object{}")
						p.recordError(errMsg)
					} else if varType == "dict" {
						errMsg := fmt.Sprintf("Invalid dict access syntax, use dict<> instead of object{}")
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

			// Check for member access
			if p.current().Type == TOKEN_DOT {
				return p.parseMemberAccessChain(node)
			}
			return node
		}

		// Check for dict access identifier<key>
		if p.current().Type == TOKEN_LANGLE {
			// Lookahead to determine if this is a comparison or dict access
			nextToken := p.peek(1)
			peek2 := p.peek(2)
			isLikelyComparison := false

			{
				// Check for patterns that suggest comparison:
				// 1. Next token is a number: i < 10
				// 2. Next token is an identifier that's likely a value: i < max
				// 3. Next token is a string/bool literal - but check context
				if nextToken.Type == TOKEN_NUMBER {
					isLikelyComparison = true
				} else if nextToken.Type == TOKEN_TRUE || nextToken.Type == TOKEN_FALSE {
					isLikelyComparison = true
				} else if nextToken.Type == TOKEN_STRING {
					// dict<"key"> has STRING followed by RANGLE
					// comparison d < "x" has STRING followed by something else
					if peek2.Type == TOKEN_RANGLE {
						// This is dict access: dict<"key">
						isLikelyComparison = false
					} else if peek2.Type == TOKEN_ASSIGN {
						// This is dict literal property: <"key": value>
						isLikelyComparison = false
					} else {
						// Something else after string, likely comparison
						isLikelyComparison = true
					}
				} else if nextToken.Type == TOKEN_IDENTIFIER {
					// Check peek2 to distinguish cases
					if peek2.Type == TOKEN_RANGLE {
						// dict<key> - dict access with variable key
						isLikelyComparison = false
					} else if peek2.Type == TOKEN_ASSIGN {
						// <key: value> - dict literal property
						isLikelyComparison = false
					} else {
						// Likely comparison: x < max
						isLikelyComparison = true
					}
				}
			}

			// If this looks like a comparison, don't parse as dict access
			// Return the identifier and let the relational expression parser handle <
			if isLikelyComparison {
				node := &ASTNode{
					Type:  NODE_IDENTIFIER,
					Value: token.Value,
					Line:  token.Line,
				}
				// Check for member access
				if p.current().Type == TOKEN_DOT {
					return p.parseMemberAccessChain(node)
				}
				return node
			}

			// Otherwise, proceed with dict access
			p.advance()

			// It's dict access: dict<key>
			accessor := p.parseCallArgument()
			p.expect(TOKEN_RANGLE)

			node := &ASTNode{
				Type:     NODE_DICT_ACCESS,
				Value:    token.Value,
				Children: []*ASTNode{accessor},
				Line:     token.Line,
			}

			// Check for member access
			if p.current().Type == TOKEN_DOT {
				return p.parseMemberAccessChain(node)
			}
			return node
		}

		// OLD: Check for dict access identifier{"key"} - deprecated, keeping for backward compatibility
		if false && p.current().Type == TOKEN_LBRACE {
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

		// Check for function call with pipes
		// We need to be careful: identifier| could be:
		// 1. A function call: func|| or func|args|
		// 2. An identifier inside a call: print|x| where x is followed by closing |
		// Only parse as function call if we're NOT inside another call OR if there's actual content
		if p.current().Type == TOKEN_PIPE {
			// If we're inside a function call, don't treat trailing | as function call start
			// This prevents print|x| from parsing x as a function call
			if p.inFunctionCall == 0 {
				// Top-level: always parse as function call
				p.advance()

				// Validate no recursion
				p.validateFunctionCall(token.Value, token.Line, token.Column)

				call := &ASTNode{
					Type:   NODE_CALL,
					Value:  token.Value,
					Line:   token.Line,
					Column: token.Column,
				}

				// Increment depth to allow nested function calls
				p.inFunctionCall++

				// Parse arguments until closing pipe (allow newlines for multiline calls)
				for p.current().Type != TOKEN_PIPE && p.current().Type != TOKEN_EOF {
					// Skip newlines between arguments
					for p.current().Type == TOKEN_NEWLINE {
						p.advance()
					}

					if p.current().Type == TOKEN_PIPE || p.current().Type == TOKEN_EOF {
						break
					}

					arg := p.parseCallArgument()


					call.Children = append(call.Children, arg)

					if p.current().Type == TOKEN_COMMA {
						p.advance()
						// Skip newlines after comma
						for p.current().Type == TOKEN_NEWLINE {
							p.advance()
						}
					} else {
						// Skip trailing newlines before closing pipe
						for p.current().Type == TOKEN_NEWLINE {
							p.advance()
						}
						if p.current().Type != TOKEN_PIPE {
							break
						}
					}
				}

				// Check for duplicate arguments in lint mode
				if p.LintMode && len(call.Children) > 1 {
					p.checkDuplicateArguments(call, token.Value)
				}

				// Consume closing pipe
				if p.current().Type == TOKEN_PIPE {
					p.advance()
				}

				p.inFunctionCall--
				return call
			}
			// If we're inside a function call, fall through to return identifier
			// UNLESS we detect this is actually a nested call (identifier||)
			nextToken := p.peek(1)
			if nextToken.Type == TOKEN_PIPE && !p.nameIsKnownVariable(token.Value) {
				// This is identifier|| - definitely a nested zero-arg function call
				// Nested calls are allowed; parseCallArgument handles the ones that
				// carry arguments, this handles a bare identifier||.
				p.advance() // consume first |
				p.advance() // consume second |
				return &ASTNode{
					Type:     NODE_CALL,
					Value:    token.Value,
					Line:     token.Line,
					Column:   token.Column,
					Children: []*ASTNode{},
				}
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

		// Check if this is a zero-arg function that can be called without ||
		// Only do this at top-level (not inside another function call)
		if p.inFunctionCall == 0 && p.zeroArgFunctions[token.Value] {
			return &ASTNode{
				Type:     NODE_CALL,
				Value:    token.Value,
				Line:     token.Line,
				Children: []*ASTNode{},
			}
		}

		// In lint mode, track identifier usage in main function for later validation
		// We only care about forward references within the same scope (main)
		if p.LintMode && p.inFunctionBody && p.currentFunctionName == "main" {
			p.constantUsages[token.Value] = append(p.constantUsages[token.Value], token.Line)
		}

		return &ASTNode{
			Type:  NODE_IDENTIFIER,
			Value: token.Value,
			Line:  token.Line,
		}

	case TOKEN_LBRACE:
		// Could be object literal or empty block - need context
		// For now, treat as dict literal (will be handled by identifier{} case)
		return p.parseDictLiteral()

	case TOKEN_LANGLE:
		// Dict literal with <> syntax
		return p.parseDictLiteral()

	case TOKEN_LBRACKET:
		return p.parseArrayLiteralBracket()

	case TOKEN_LPAREN:
		// Parenthesized expression
		p.advance() // consume '('
		expr := p.parseExpression()
		p.expect(TOKEN_RPAREN)
		return expr

	case TOKEN_SWITCH:
		// Switch expression (can be used in assignments)
		return p.parseSwitchStatement()

	// Type casts: int(value), float(value), char(value), string(value)
	case TOKEN_INT_TYPE, TOKEN_FLOAT_TYPE, TOKEN_CHAR_TYPE, TOKEN_STRING_TYPE:
		token := p.current()
		p.advance()

		// Check if this is object instantiation with {}
		if p.current().Type == TOKEN_LBRACE {
			p.advance()

			// Check for empty object: vector2{}
			if p.current().Type == TOKEN_RBRACE {
				p.advance()
				return &ASTNode{
					Type:     NODE_OBJECT_LITERAL,
					DataType: "object",
					Value:    token.Value,
					Children: []*ASTNode{},
					Line:     token.Line,
				}
			}

			// Check if this has named properties (for instantiation)
			if (p.current().Type == TOKEN_IDENTIFIER || p.current().Type == TOKEN_STRING) &&
				p.peek(1).Type == TOKEN_ASSIGN {
				// Object instantiation with properties: vector2{x: 10, y: 20}
				obj := p.parseObjectLiteral()
				obj.Value = token.Value // Set the type name
				obj.Line = token.Line   // Set the line number
				return obj
			}

			// Otherwise error - unexpected syntax
			panic(fmt.Sprintf("Unexpected token in object literal at line %d", p.current().Line))
		}

		// Check if this is old dict literal syntax with <>
		if p.current().Type == TOKEN_LANGLE {
			p.advance()

			// Check if this has named properties (for instantiation)
			if (p.current().Type == TOKEN_IDENTIFIER || p.current().Type == TOKEN_STRING) &&
				p.peek(1).Type == TOKEN_ASSIGN {
				// Old object instantiation: vector2<x: 10, y: 20> - convert to new syntax warning
				obj := p.parseObjectLiteral()
				obj.Value = token.Value // Set the type name
				obj.Line = token.Line   // Set the line number
				return obj
			}

			// Otherwise parse as comma-separated values: OLD vector2<10,20> syntax
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

		// Check if this is a cast (followed by parenthesis or pipe)
		if p.current().Type == TOKEN_PIPE {
			// vector2|x,y| syntax
			p.advance() // consume |
			args := []*ASTNode{}
			for p.current().Type != TOKEN_PIPE && p.current().Type != TOKEN_EOF {
				arg := p.parseAdditiveExpression()
				args = append(args, arg)

				if p.current().Type == TOKEN_COMMA {
					p.advance()
				} else if p.current().Type != TOKEN_PIPE {
					break
				}
			}
			p.expect(TOKEN_PIPE)

			return &ASTNode{
				Type:     NODE_CALL,
				Value:    token.Value,
				Children: args,
				Line:     token.Line,
			}
		}

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

	// Infer element type from array contents if all elements have the same type
	if len(array.Children) > 0 {
		firstType := p.inferType(array.Children[0])
		allSameType := true

		for i := 1; i < len(array.Children); i++ {
			elemType := p.inferType(array.Children[i])
			if elemType != firstType {
				allSameType = false
				break
			}
		}

		if allSameType && firstType != "" && firstType != "unknown" {
			array.DataType = "array[" + firstType + "]"
		}
	}

	// Check for member access after array literal
	if p.current().Type == TOKEN_DOT {
		return p.parseMemberAccessChain(array)
	}

	return array
}

func (p *Parser) parseObjectLiteral() *ASTNode {
	// TOKEN_LBRACE already consumed by caller
	// Save line before advancing
	line := p.current().Line

	// Skip any newlines and indents after opening brace
	for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT {
		p.advance()
	}

	object := &ASTNode{
		Type:     NODE_OBJECT_LITERAL,
		DataType: "object",
		Children: []*ASTNode{},
		Line:     line,
	}

	p.inObjectLiteral = true

	for p.current().Type != TOKEN_RBRACE && p.current().Type != TOKEN_EOF {
		// Skip any indents/dedents before property name
		for p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
			p.advance()
		}

		// Check if we reached the closing brace
		if p.current().Type == TOKEN_RBRACE {
			break
		}

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

		// Parse property value
		propValue := p.parseCallArgument()

		// Create property node
		prop := &ASTNode{
			Type:     NODE_OBJECT_PROPERTY,
			Value:    propName,
			Children: []*ASTNode{propValue},
			Line:     propValue.Line,
		}
		object.Children = append(object.Children, prop)

		// Check for comma or end (comma is optional, newlines are allowed)
		// Skip any whitespace tokens (newlines, indents, dedents)
		for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
			p.advance()
		}

		// Now check if we need a comma
		if p.current().Type == TOKEN_COMMA {
			p.advance()
			// Skip any whitespace after comma
			for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
				p.advance()
			}
		} else if p.current().Type != TOKEN_RBRACE {
			// No comma and not at closing brace - this might be OK if we just consumed dedents
			// In strict mode we could error here, but for flexibility allow it
			if p.LintMode {
				// Only error if we're clearly missing something
				if p.current().Type == TOKEN_IDENTIFIER || p.current().Type == TOKEN_STRING {
					p.recordError(fmt.Sprintf("Missing comma between properties in object literal at line %d", p.current().Line))
				}
			}
		}
	}

	p.inObjectLiteral = false

	// Skip any remaining dedents before closing brace
	for p.current().Type == TOKEN_DEDENT {
		p.advance()
	}

	p.expect(TOKEN_RBRACE)

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
	// Dict literals now use <> syntax
	startToken := p.current().Type
	var endToken TokenType

	if startToken == TOKEN_LANGLE {
		p.advance()
		endToken = TOKEN_RANGLE
	} else if startToken == TOKEN_LBRACE {
		// Legacy support for old {} syntax (backward compatibility)
		p.advance()
		endToken = TOKEN_RBRACE
	} else {
		panic(fmt.Sprintf("Expected '<' or '{' for dict literal at line %d", p.current().Line))
	}

	dict := &ASTNode{
		Type:     NODE_DICT_LITERAL,
		DataType: "dict",
	}

	p.inDictLiteral = true

	// Skip leading newlines/indents for multiline dicts
	for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
		p.advance()
	}

	for p.current().Type != endToken && p.current().Type != TOKEN_EOF {
		// Skip newlines/indents between entries
		for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
			p.advance()
		}

		// Check if we've reached the end
		if p.current().Type == endToken {
			break
		}

		// Parse key (can be string or identifier)
		key := p.parseCallArgument()

		// Skip newlines/indents before colon
		for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
			p.advance()
		}

		p.expect(TOKEN_ASSIGN) // Using : as separator between key and value

		// Skip newlines/indents after colon
		for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
			p.advance()
		}

		value := p.parseCallArgument()

		// Store key-value pair as two consecutive children
		dict.Children = append(dict.Children, key, value)

		// Skip newlines/indents before comma or end
		for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
			p.advance()
		}

		if p.current().Type == TOKEN_COMMA {
			p.advance()
			// Skip newlines/indents after comma
			for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
				p.advance()
			}
		} else if p.current().Type != endToken {
			break
		}
	}

	p.inDictLiteral = false
	p.expect(endToken)

	// Check for member access after dict literal
	if p.current().Type == TOKEN_DOT {
		return p.parseMemberAccessChain(dict)
	}

	return dict
}

// Parse enum declaration
