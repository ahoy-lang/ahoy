package ahoy

import (
	"fmt"
	"strconv"
	"strings"
)

// Expression parsing: assignment, calls, operators and precedence.

func (p *Parser) parseAssignmentOrExpression() *ASTNode {
	// Check for unary expression assignment: ^ptr: value or &var: value
	if p.current().Type == TOKEN_CARET || p.current().Type == TOKEN_AMPERSAND {
		// Look ahead to see if this is an assignment pattern
		if p.pos+2 < len(p.tokens) &&
			p.tokens[p.pos+1].Type == TOKEN_IDENTIFIER &&
			p.tokens[p.pos+2].Type == TOKEN_ASSIGN {
			// Parse the unary expression
			target := p.parseUnaryExpression()
			p.expect(TOKEN_ASSIGN)
			value := p.parseExpression()

			return &ASTNode{
				Type:     NODE_ASSIGNMENT,
				Children: []*ASTNode{target, value},
				Line:     target.Line,
			}
		}
	}

	// Check for object property assignment: obj{'prop'}: value
	if p.pos+2 < len(p.tokens) && p.tokens[p.pos+1].Type == TOKEN_LBRACE {
		// Check if this is obj{'prop'}: pattern
		savedPos := p.pos
		p.advance() // skip identifier
		p.advance() // skip {
		// Skip the accessor
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
			// Parse as object property assignment
			target := p.parsePrimaryExpression() // This will parse obj{'prop'}
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

	// Check for dict property assignment: dict<key>: value
	if p.pos+2 < len(p.tokens) && p.tokens[p.pos+1].Type == TOKEN_LANGLE {
		// Check if this is dict<key>: pattern
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
			// Parse as dict property assignment
			target := p.parsePrimaryExpression() // This will parse dict<key>
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

	// Check for array index with member access assignment: arr[index].property: value
	// Also handles 2D array assignment: arr[i][j]: value
	// This must come BEFORE simple array index check to handle the more complex case first
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
		// Handle chained array access (2D arrays): arr[i][j]
		for p.current().Type == TOKEN_LBRACKET {
			p.advance() // skip [
			depth = 1
			for p.pos < len(p.tokens) && depth > 0 {
				if p.current().Type == TOKEN_LBRACKET {
					depth++
				} else if p.current().Type == TOKEN_RBRACKET {
					depth--
				}
				p.advance()
			}
		}
		// Now check if there's a dot followed by member access
		if p.current().Type == TOKEN_DOT {
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
			isCompoundAssignment := p.isCompoundAssignOp(p.current().Type)
			p.pos = savedPos // restore position

			if isAssignment || isCompoundAssignment {
				target := p.parsePrimaryExpression() // This will parse arr[index].property

				if isCompoundAssignment {
					// Handle +=, -=, *=, /=, %=
					opToken := p.current()
					p.advance() // consume compound operator
					value := p.parseExpression()

					// Convert to: target: target op value
					op := p.getCompoundAssignOp(opToken.Type)

					// Create a copy of target for the right side of the binary op
					targetCopy := p.copyASTNode(target)

					binaryOp := &ASTNode{
						Type:     NODE_BINARY_OP,
						Value:    op,
						Children: []*ASTNode{targetCopy, value},
						Line:     target.Line,
					}

					return &ASTNode{
						Type:     NODE_ASSIGNMENT,
						Children: []*ASTNode{target, binaryOp},
						Line:     target.Line,
					}
				} else {
					if p.current().Type == TOKEN_ASSIGN || p.current().Type == TOKEN_EQUALS {
						p.advance()
					} else {
						p.expect(TOKEN_ASSIGN)
					}
					value := p.parseExpression()

					return &ASTNode{
						Type:     NODE_ASSIGNMENT,
						Children: []*ASTNode{target, value},
						Line:     target.Line,
					}
				}
			}
		}
		// If no dot found, check for simple array assignment
		isAssignment := p.current().Type == TOKEN_ASSIGN || p.current().Type == TOKEN_EQUALS
		isCompoundAssignment := p.isCompoundAssignOp(p.current().Type)
		p.pos = savedPos // restore position

		if isAssignment || isCompoundAssignment {
			target := p.parsePrimaryExpression() // This will parse arr[index]

			if isCompoundAssignment {
				// Handle +=, -=, *=, /=, %=
				opToken := p.current()
				p.advance() // consume compound operator
				value := p.parseExpression()

				// Convert to: target: target op value
				op := p.getCompoundAssignOp(opToken.Type)

				// Create a copy of target for the right side of the binary op
				targetCopy := p.copyASTNode(target)

				binaryOp := &ASTNode{
					Type:     NODE_BINARY_OP,
					Value:    op,
					Children: []*ASTNode{targetCopy, value},
					Line:     target.Line,
				}

				return &ASTNode{
					Type:     NODE_ASSIGNMENT,
					Children: []*ASTNode{target, binaryOp},
					Line:     target.Line,
				}
			} else {
				if p.current().Type == TOKEN_ASSIGN || p.current().Type == TOKEN_EQUALS {
					p.advance()
				} else {
					p.expect(TOKEN_ASSIGN)
				}
				value := p.parseExpression()

				return &ASTNode{
					Type:     NODE_ASSIGNMENT,
					Children: []*ASTNode{target, value},
					Line:     target.Line,
				}
			}
		}
		p.pos = savedPos // restore position
	}

	// Check for member access assignment: obj.property: value or StructType.#static_field: value
	if p.pos+2 < len(p.tokens) && p.tokens[p.pos+1].Type == TOKEN_DOT {
		savedPos := p.pos
		p.advance() // skip identifier
		p.advance() // skip .
		// Skip optional # for static field access
		if p.current().Type == TOKEN_HASH {
			p.advance() // skip #
		}
		// Skip the property name(s)
		for p.pos < len(p.tokens) && p.current().Type == TOKEN_IDENTIFIER {
			p.advance()
			if p.current().Type == TOKEN_DOT {
				p.advance() // skip next dot
				// Skip optional # for chained static field access
				if p.current().Type == TOKEN_HASH {
					p.advance() // skip #
				}
			} else {
				break
			}
		}
		isAssignment := p.current().Type == TOKEN_ASSIGN || p.current().Type == TOKEN_EQUALS
		isCompoundAssignment := p.isCompoundAssignOp(p.current().Type)
		p.pos = savedPos // restore position

		if isAssignment || isCompoundAssignment {
			target := p.parsePrimaryExpression() // This will parse obj.property or StructType.#field

			if isCompoundAssignment {
				// Handle +=, -=, *=, /=, %=
				opToken := p.current()
				p.advance() // consume compound operator
				value := p.parseExpression()

				// Convert to: target: target op value
				op := p.getCompoundAssignOp(opToken.Type)

				// Create a copy of target for the right side of the binary op
				targetCopy := p.copyASTNode(target)

				binaryOp := &ASTNode{
					Type:     NODE_BINARY_OP,
					Value:    op,
					Children: []*ASTNode{targetCopy, value},
					Line:     target.Line,
				}

				// Validate property assignment in lint mode
				if p.LintMode && (target.Type == NODE_MEMBER_ACCESS || target.Type == NODE_STATIC_MEMBER_ACCESS) {
					p.validatePropertyAssignment(target, binaryOp, target.Line)
				}

				return &ASTNode{
					Type:     NODE_ASSIGNMENT,
					Children: []*ASTNode{target, binaryOp},
					Line:     target.Line,
				}
			} else {
				if p.current().Type == TOKEN_ASSIGN || p.current().Type == TOKEN_EQUALS {
					p.advance()
				} else {
					p.expect(TOKEN_ASSIGN)
				}
				value := p.parseExpression()

				// Validate property assignment in lint mode
				if p.LintMode && (target.Type == NODE_MEMBER_ACCESS || target.Type == NODE_STATIC_MEMBER_ACCESS) {
					p.validatePropertyAssignment(target, value, target.Line)
				}

				return &ASTNode{
					Type:     NODE_ASSIGNMENT,
					Children: []*ASTNode{target, value},
					Line:     target.Line,
				}
			}
		}
	}

	// OLD: dict property assignment (deprecated syntax, already handled above)
	// Check for dict property assignment: dict{"key"}: value - DEPRECATED
	if false && p.pos+2 < len(p.tokens) && p.tokens[p.pos+1].Type == TOKEN_LBRACE {
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

	// Check for compound assignment: identifier +=/-=/*=/=/%= value
	if p.pos+1 < len(p.tokens) && p.isCompoundAssignOp(p.tokens[p.pos+1].Type) {
		name := p.expect(TOKEN_IDENTIFIER)
		line := name.Line
		opToken := p.current()
		p.advance() // consume compound operator
		value := p.parseExpression()

		// Convert to: identifier: identifier op value
		op := p.getCompoundAssignOp(opToken.Type)

		// Create identifier node for the right side of binary op
		identNode := &ASTNode{
			Type:  NODE_IDENTIFIER,
			Value: name.Value,
			Line:  line,
		}

		// Create binary operation: identifier + value
		binaryOp := &ASTNode{
			Type:     NODE_BINARY_OP,
			Value:    op,
			Children: []*ASTNode{identNode, value},
			Line:     line,
		}

		// Compound assignment is a reassignment - check variable exists in lint mode
		if p.LintMode {
			exists := false
			if _, ok := p.variableTypes[name.Value]; ok {
				exists = true
			}
			if !exists {
				if _, ok := p.declaredVars[name.Value]; ok {
					exists = true
				}
			}
			if !exists {
				if p.functionScope != nil {
					if _, ok := p.functionScope[name.Value]; ok {
						exists = true
					}
				}
			}
			if !exists {
				errMsg := fmt.Sprintf("Cannot reassign to undeclared variable '%s'. Use '=' or ':=' to declare first", name.Value)
				p.recordErrorAtLine(errMsg, line)
			}
		}

		// Create assignment: identifier: (identifier + value)
		return &ASTNode{
			Type:     NODE_ASSIGNMENT,
			Value:    name.Value,           // Variable name goes in Value field
			Children: []*ASTNode{binaryOp}, // Binary op is the value being assigned
			Line:     line,
		}
	}

	// NEW SYNTAX: Check for `identifier = value` (reassignment) or `identifier := value` (tuple smart assign)
	if p.pos+1 < len(p.tokens) && (p.tokens[p.pos+1].Type == TOKEN_EQUALS || p.tokens[p.pos+1].Type == TOKEN_WALRUS) {
		name := p.expect(TOKEN_IDENTIFIER)
		line := name.Line
		varName := name.Value

		// Check if the variable name is in SCREAMING_SNAKE_CASE - if so, treat as constant
		if isScreamingSnakeCase(name.Value) {
			// Constants can use = or := syntax
			p.advance() // consume = or :=

			// In lint mode, check if constant is being redeclared
			if p.LintMode {
				if existingLine, exists := p.constants[name.Value]; exists {
					errMsg := fmt.Sprintf("Can't redeclare a constant declared on line %d", existingLine)
					p.recordError(errMsg)
				} else {
					p.constants[name.Value] = line
					// Track if constant is declared in main function
					if p.currentFunctionName == "main" {
						p.constantsInMain[name.Value] = true

						// Check if this constant was used earlier in main
						if usageLines, wasUsed := p.constantUsages[name.Value]; wasUsed {
							for _, usageLine := range usageLines {
								if usageLine < line {
									errMsg := fmt.Sprintf("use of const '%s' before its declared on line %d", name.Value, line)
									p.recordErrorAtLine(errMsg, usageLine)
								}
							}
						}
					}
				}
			}

			// Skip newlines before parsing value
			for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
				p.advance()
			}

			value := p.parseExpression()

			// In lint mode, track the constant's type
			if p.LintMode {
				inferredType := p.inferType(value)
				if inferredType != "unknown" {
					p.variableTypes[name.Value] = inferredType
				}
			}

			return &ASTNode{
				Type:     NODE_CONSTANT_DECLARATION,
				Value:    name.Value,
				DataType: "", // No explicit type for = or :=
				Line:     line,
				Children: []*ASTNode{value},
			}
		}

		// Check which operator we're consuming
		opToken := p.current()

		// '=' is for reassignment (not declaration)
		p.advance() // consume = or :=

		// Warn/Error if using := for single variable (should use : or :type=)
		if opToken.Type == TOKEN_WALRUS && p.LintMode {
			errMsg := "Can't use walrus ':=' without a type (for single var); use ':type=' for explicit type or ':' to declare or '=' to reassign"
			p.recordErrorAtLine(errMsg, line)
		}

		// '=' is reassignment - check if variable exists
		if opToken.Type == TOKEN_EQUALS {
			if p.LintMode {
				_, existsInFunc := p.functionScope[varName]
				_, existsGlobal := p.declaredVars[varName]
				alreadyDeclared := existsInFunc || existsGlobal

				if !alreadyDeclared && varName != "_" {
					errMsg := fmt.Sprintf("Cannot assign to undeclared variable '%s'. Use ':' or '%s:type=' for an explicit type", varName, varName)
					p.recordErrorAtLine(errMsg, line)
				}

				// Type check for reassignment
				if alreadyDeclared {
					existingType := ""
					if t, ok := p.functionScope[varName]; ok {
						existingType = t
					} else if t, ok := p.variableTypes[varName]; ok {
						existingType = t
					}

					// Parse the value to check type compatibility
					for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
						p.advance()
					}
					value := p.parseExpression()
					inferredType := p.inferType(value)

					if existingType != "" && inferredType != "unknown" && !p.checkTypeCompatibility(existingType, inferredType) {
						errMsg := fmt.Sprintf("Type mismatch: can't reassign %s (type %s) with value of type %s", varName, existingType, inferredType)
						p.recordErrorAtLine(errMsg, line)
					}

					// Return reassignment node
					return &ASTNode{
						Type:     NODE_ASSIGNMENT,
						Value:    name.Value,
						Children: []*ASTNode{value},
						Line:     line,
					}
				}
			}

			// If not in lint mode or variable doesn't exist, parse value and return reassignment node
			for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
				p.advance()
			}
			value := p.parseExpression()

			return &ASTNode{
				Type:     NODE_ASSIGNMENT,
				Value:    name.Value,
				Children: []*ASTNode{value},
				Line:     line,
			}
		}

		// Skip newlines before parsing value
		for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
			p.advance()
		}

		value := p.parseExpression()

		// Record the declaration in every mode; see nameIsKnownVariable.
		p.recordKnownVar(name.Value)

		// In lint mode, infer and track the variable's type (but not for _ placeholder)
		if p.LintMode && name.Value != "_" {
			inferredType := p.inferType(value)
			if inferredType != "unknown" {
				p.variableTypes[name.Value] = inferredType
			}
		}

		return &ASTNode{
			Type:     NODE_VARIABLE_DECLARATION,
			Value:    name.Value,
			DataType: "", // Empty means type is inferred
			Line:     line,
			Children: []*ASTNode{value},
		}
	}

	if p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Type == TOKEN_ASSIGN {
		// Assignment with possible type annotation
		name := p.expect(TOKEN_IDENTIFIER)
		line := name.Line

		// Check if the variable name is in SCREAMING_SNAKE_CASE - if so, treat as constant
		if isScreamingSnakeCase(name.Value) {
			// Convert to constant declaration (same as MY_CONST::"value")
			p.expect(TOKEN_ASSIGN) // consume :

			// In lint mode, check if constant is being redeclared
			if p.LintMode {
				if existingLine, exists := p.constants[name.Value]; exists {
					errMsg := fmt.Sprintf("Can't redeclare a constant declared on line %d",
						existingLine)
					p.recordError(errMsg)
				} else {
					// Register this constant
					p.constants[name.Value] = line
					// Track if constant is declared in main function
					if p.currentFunctionName == "main" {
						p.constantsInMain[name.Value] = true

						// Check if this constant was used earlier in main
						if usageLines, wasUsed := p.constantUsages[name.Value]; wasUsed {
							for _, usageLine := range usageLines {
								if usageLine < line {
									errMsg := fmt.Sprintf("use of const '%s' before its declared on line %d", name.Value, line)
									p.recordErrorAtLine(errMsg, usageLine)
								}
							}
						}
					}
				}
			}

			// Check for type annotation (type=)
			var explicitType string
			if p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
				p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_BOOL_TYPE ||
				p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
				p.current().Type == TOKEN_IDENTIFIER {

				possibleType := p.current().Value
				// Look ahead to see if there's an = after the type
				if p.peek(1).Type == TOKEN_EQUALS {
					explicitType = possibleType
					p.advance() // consume type
					p.advance() // consume =
				}
			}

			// Skip newlines before parsing value for multiline literals
			for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
				p.advance()
			}

			// Regular constant value
			value := p.parseExpression()

			// In lint mode, track the constant's type
			if p.LintMode {
				if explicitType != "" {
					resolvedType := p.resolveTypeAlias(explicitType)
					p.variableTypes[name.Value] = resolvedType
				} else {
					inferredType := p.inferType(value)
					if inferredType != "unknown" {
						p.variableTypes[name.Value] = inferredType
					}
				}
			}

			return &ASTNode{
				Type:     NODE_CONSTANT_DECLARATION,
				Value:    name.Value,
				DataType: explicitType,
				Line:     line,
				Children: []*ASTNode{value},
			}
		}

		p.expect(TOKEN_ASSIGN)

		// Skip newlines after colon for multiline expressions
		for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
			p.advance()
		}

		// NEW SYNTAX: ':' is ALWAYS a declaration
		// - var: value → declaration with inference
		// - var:type=value → declaration with explicit type
		var explicitType string
		isDeclaration := true

		// Check if next token looks like a type annotation
		if p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
			p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_BOOL_TYPE ||
			p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
			p.current().Type == TOKEN_IDENTIFIER {

			// A cast opens with a pipe: int|5|, string|n|. Parentheses are also
			// accepted. Without this the type keyword was taken as an annotation and
			// the pipe that followed was left dangling, so `s: string|x|` failed to
			// parse even though `s: string|42|` happened to work.
			if (p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
				p.current().Type == TOKEN_CHAR_TYPE || p.current().Type == TOKEN_STRING_TYPE ||
				p.current().Type == TOKEN_BOOL_TYPE) &&
				(p.peek(1).Type == TOKEN_PIPE || p.peek(1).Type == TOKEN_LPAREN) {
				// This is a cast, not a type annotation - treat as a declaration whose
				// type is inferred from the cast expression. isDeclaration is already
				// true, so just fall through.
			} else if p.current().Type == TOKEN_IDENTIFIER && p.peek(1).Type == TOKEN_LANGLE {
				// Check if this is a type annotation (Type<...>) or dict access (var<key>)
				// Types are capitalized, variables are lowercase
				// Exception: 'dict' is a type keyword even though lowercase
				identValue := p.current().Value
				isType := (len(identValue) > 0 && identValue[0] >= 'A' && identValue[0] <= 'Z') || identValue == "dict" || identValue == "array"

				if !isType {
					// This is a variable followed by <, not a type annotation
					// Fall through to parse as expression (dict access)
				} else {
					// This is a type annotation, continue with type parsing
					possibleType := p.current().Value

					// Handle type<...> syntax
					if p.peek(1).Type == TOKEN_EQUALS || p.peek(1).Type == TOKEN_LANGLE {
						// Non-collection types with = or <
						explicitType = possibleType
						isDeclaration = true
						p.advance() // consume type
						if p.current().Type == TOKEN_EQUALS {
							p.advance() // consume =
						}
						// If TOKEN_LANGLE, DON'T consume it - but we need to handle it specially below
					}
				}
			} else {
				// This might be a type annotation
				possibleType := p.current().Value

				// Check for typed collections: array[type]= or dict[key,value]= or dict<key,value>=
				if (p.current().Type == TOKEN_ARRAY_TYPE || p.current().Type == TOKEN_DICT_TYPE ||
					(p.current().Type == TOKEN_IDENTIFIER && p.current().Value == "dict")) &&
					(p.peek(1).Type == TOKEN_LBRACKET || p.peek(1).Type == TOKEN_LANGLE) {
					baseType := possibleType
					isDict := p.current().Type == TOKEN_DICT_TYPE || p.current().Value == "dict"
					bracketType := p.peek(1).Type
					p.advance() // consume array/dict
					p.advance() // consume [ or <

					if isDict {
						// dict[key_type, value_type]= or dict<key_type, value_type>=
						keyType := p.current().Value
						p.advance()
						if p.current().Type == TOKEN_COMMA {
							p.advance()
							valueType := p.current().Value
							p.advance()
							endBracket := TOKEN_RBRACKET
							if bracketType == TOKEN_LANGLE {
								endBracket = TOKEN_RANGLE
							}
							// Handle dict<string,string>= case where >= is tokenized as GREATER_EQUAL
							if p.current().Type == endBracket {
								p.advance() // consume ] or >
								if bracketType == TOKEN_LANGLE {
									possibleType = fmt.Sprintf("%s<%s,%s>", baseType, keyType, valueType)
								} else {
									possibleType = fmt.Sprintf("%s[%s,%s]", baseType, keyType, valueType)
								}
								explicitType = possibleType
								isDeclaration = true
								// Expect = after typed collection
								if p.current().Type == TOKEN_EQUALS {
									p.advance() // consume =
								}
							} else if bracketType == TOKEN_LANGLE && p.current().Type == TOKEN_GREATER_EQUAL {
								// Special case: dict<string,string>= where >= is tokenized as a single token
								// Treat >= as > followed by = in this context
								p.advance() // consume >=, which acts as both > and =
								possibleType = fmt.Sprintf("%s<%s,%s>", baseType, keyType, valueType)
								explicitType = possibleType
								isDeclaration = true
								// No need to consume = separately, it was part of >=
							}
						}
					} else {
						// array[element_type]= - supports nested arrays like array[array[int]]
						elementType := p.current().Value
						p.advance()
						// Check for nested array type: array[array[inner_type]]
						if elementType == "array" && p.current().Type == TOKEN_LBRACKET {
							p.advance() // consume inner [
							innerType := p.current().Value
							p.advance()
							if p.current().Type == TOKEN_RBRACKET {
								p.advance() // consume inner ]
								elementType = fmt.Sprintf("array[%s]", innerType)
							}
						}
						if p.current().Type == TOKEN_RBRACKET {
							p.advance() // consume ]
							possibleType = fmt.Sprintf("%s[%s]", baseType, elementType)
							explicitType = possibleType
							isDeclaration = true
							// Expect = after typed collection
							if p.current().Type == TOKEN_EQUALS {
								p.advance() // consume =
							}
						}
					}
					// After parsing typed collection, we're done with type annotation
				} else if p.peek(1).Type == TOKEN_EQUALS || p.peek(1).Type == TOKEN_LANGLE {
					// Non-collection types with = or <
					explicitType = possibleType
					isDeclaration = true
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
		// But NOT for dict types - those use <> for literals
		var value *ASTNode
		if explicitType != "" && p.current().Type == TOKEN_LANGLE && !strings.Contains(explicitType, "dict") {
			// Manually handle the object literal with type name (not dict)
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
				value.Line = line          // Set the line number
			} else {
				// Parse as regular expression (fallback)
				value = p.parseExpression()
			}
		} else {
			// Skip newlines before parsing value for multiline literals
			for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
				p.advance()
			}
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
			// Also check declaredVars (for variables declared with = or :=)
			if !exists {
				if _, ok := p.declaredVars[varName]; ok {
					for k, v := range p.declaredVars {
						fmt.Printf("  - %s at line %d\n", k, v)
					}
					// Variable was declared, get its type
					if t, ok := p.variableTypes[varName]; ok {
						existingType = t
						exists = true
					} else {
						exists = true // Declared but type unknown
					}
				}
			}

			// NEW SYNTAX: Check if this is reassignment without declaration
			// (This shouldn't happen with ':' since isDeclaration is always true for ':')
			if !isDeclaration && !exists && varName != "_" {
				errMsg := fmt.Sprintf("Cannot assign to undeclared variable '%s'. Use ':' or '%s:type=' for an explicit type", varName, varName)
				p.recordErrorAtLine(errMsg, line)
			}

			if !isDeclaration && exists {
				// Reassignment - check type compatibility
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
			} else if isDeclaration {
				// Declaration - store the type
				// If inside a function, store in function scope, otherwise global
				targetScope := p.variableTypes
				if p.inFunctionBody && p.functionScope != nil {
					targetScope = p.functionScope
				}

				// Check for redeclaration (but allow _ placeholder)
				if exists && varName != "_" {
					// Try to get the line where it was originally declared
					if existingLine, hasLine := p.declaredVars[varName]; hasLine {
						errMsg := fmt.Sprintf("Variable '%s' already declared on line %d; use '=' to update variable", varName, existingLine)
						p.recordErrorAtLine(errMsg, line)
					} else {
						errMsg := fmt.Sprintf("Variable '%s' already declared; use '=' to update variable", varName)
						p.recordErrorAtLine(errMsg, line)
					}
				}

				// Track in declaredVars for later reassignment checks (except for _ placeholder)
				if varName != "_" {
					p.declaredVars[varName] = line
				}

				if explicitType != "" {
					if varName != "_" {
						targetScope[varName] = explicitType
					}

					// Validate bool type - don't allow 0 or 1 literal assignments
					if explicitType == "bool" && value.Type == NODE_NUMBER {
						if value.Value == "0" || value.Value == "1" {
							errMsg := fmt.Sprintf("Boolean variable '%s' should use 'true' or 'false', not '%s' (line %d)", varName, value.Value, line)
							p.recordError(errMsg)
						}
					}

					// Validate struct initialization
					if value.Type == NODE_OBJECT_LITERAL {
						p.validateStructInitialization(explicitType, value, line)
						// Track object literal properties
						p.trackObjectLiteralProperties(varName, value)
					}

					// Validate switch statement return types match explicit type
					if value.Type == NODE_SWITCH_STATEMENT {
						p.validateSwitchReturnTypesWithExpected(value, explicitType, line)
					}

					// Validate typed collections match
					if strings.HasPrefix(explicitType, "array[") || strings.HasPrefix(explicitType, "dict[") ||
						strings.HasPrefix(explicitType, "dict<") {
						inferredType := p.inferType(value)
						if !p.collectionTypeMatches(explicitType, inferredType) {
							errMsg := fmt.Sprintf("Type mismatch (line %d): expected %s but got %s", line, explicitType, inferredType)
							p.recordError(errMsg)
						}
					}
				} else {
					// Check if this is a switch expression without explicit type
					if value.Type == NODE_SWITCH_STATEMENT {
						errMsg := fmt.Sprintf("Switch expression variable '%s' requires explicit type", varName)
						p.recordErrorAtLine(errMsg, line)
					}

					// Infer type from value (except for _ placeholder)
					inferredType := p.inferType(value)
					if inferredType != "unknown" && varName != "_" {
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

		// Record the name in every mode; see nameIsKnownVariable. This is the main
		// declaration path (`name: value` and `name:type= value`), and it builds
		// its node through nodeType rather than a literal, so it is easy to miss.
		p.recordKnownVar(name.Value)

		// Return appropriate node type based on whether this is declaration or reassignment
		nodeType := NODE_ASSIGNMENT
		if isDeclaration {
			nodeType = NODE_VARIABLE_DECLARATION
		}

		return &ASTNode{
			Type:     nodeType,
			Value:    name.Value,
			DataType: explicitType,
			Children: []*ASTNode{value},
			Line:     line,
		}
	}

	return p.parseExpression()
}

func (p *Parser) parseCallArgument() *ASTNode {
	// Check for named argument: name: value
	// Only parse as named arg if we're in a function call context
	// and not in an object/dict/array literal
	if !p.inObjectLiteral && !p.inDictLiteral && !p.inArrayLiteral &&
		p.current().Type == TOKEN_IDENTIFIER && p.peek(1).Type == TOKEN_ASSIGN {
		paramName := p.current().Value
		p.advance() // consume identifier
		p.advance() // consume :

		// Parse the value
		value := p.parseAdditiveExpression()

		// Create a special node to represent named argument
		return &ASTNode{
			Type:  NODE_BINARY_OP,
			Value: "named_arg",
			Children: []*ASTNode{
				{Type: NODE_IDENTIFIER, Value: paramName},
				value,
			},
		}
	}

	// A call used as an argument: print|len|items||
	//
	// Inside an argument list a pipe is ambiguous - it may open a nested call,
	// or it may be the enclosing call's closing pipe (print|x|). tryParseNestedCall
	// resolves it and rolls back when the pipe belonged to the enclosing call.
	if p.inFunctionCall > 0 && p.current().Type == TOKEN_IDENTIFIER &&
		p.peek(1).Type == TOKEN_PIPE {
		if nested := p.tryParseNestedCall(p.current()); nested != nil {
			return nested
		}
	}

	// Parse an expression but stop at comma, pipe, or comparison operators
	// Use parseAdditiveExpression to avoid treating <> as comparisons
	return p.parseAdditiveExpression()
}

// tryParseNestedCall parses `name|args|` as a call while the parser is inside
// another call's argument list, so that print|len|items|| works.
//
// A nested call is accepted only when its own closing pipe is immediately
// followed by another pipe, which can then only be the enclosing call's closing
// pipe. Anything else is rolled back - position and any diagnostics recorded
// while trying - so `print|x|` still parses as a plain identifier argument.
func (p *Parser) tryParseNestedCall(name Token) *ASTNode {
	savedPos := p.pos
	savedErrors := len(p.Errors)

	// The caller's current token is the callee name; step past it and past the
	// opening pipe, so the loop below sees the first argument.
	p.advance() // consume the identifier
	p.advance() // consume the opening pipe

	call := &ASTNode{
		Type:   NODE_CALL,
		Value:  name.Value,
		Line:   name.Line,
		Column: name.Column,
	}

	p.inFunctionCall++
	for p.current().Type != TOKEN_PIPE && p.current().Type != TOKEN_EOF &&
		p.current().Type != TOKEN_NEWLINE {
		arg := p.parseCallArgument()
		if arg == nil {
			break
		}
		call.Children = append(call.Children, arg)
		if p.current().Type == TOKEN_COMMA {
			p.advance()
			continue
		}
		break
	}
	p.inFunctionCall--

	// With arguments the shape is unambiguous: the nested call's closing pipe
	// must be followed by the enclosing call's closing pipe, as in
	// print|len|items||.
	if len(call.Children) > 0 {
		if p.current().Type == TOKEN_PIPE && p.peek(1).Type == TOKEN_PIPE {
			p.advance() // consume the nested call's closing pipe
			return call
		}
	} else if p.current().Type == TOKEN_PIPE && !p.nameIsKnownVariable(name.Value) {
		// Zero arguments: `name||` has exactly the same token shape as a plain
		// identifier argument followed by the enclosing call's closing pipe
		// (print|x|), so the name has to decide. A variable wins - in
		// print|len|items|| the two pipes belong to len and print, not to
		// items() - while anything that is not a variable is the call.
		p.advance()
		return call
	}

	// Not a nested call after all: the pipe belonged to the enclosing call.
	p.pos = savedPos
	p.Errors = p.Errors[:savedErrors]
	return nil
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

		// Stop if we encounter else/anif/elseif (for if statements)
		if constructName == "if" || constructName == "anif" {
			if p.current().Type == TOKEN_ELSE || p.current().Type == TOKEN_ANIF || p.current().Type == TOKEN_ELSEIF {
				// Decrement blockDepth since we're not consuming $ here
				// The else/anif will increment it again
				p.blockDepth--
				return block
			}
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
			} else if count > p.blockDepth+1 { // +1 because we already decremented once
				if p.LintMode {
					p.Errors = append(p.Errors, ParseError{
						Message: fmt.Sprintf("Cannot close %d blocks, only %d block(s) open", count, p.blockDepth+1),
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
			p.recordErrorAtLine(errMsg, startLine)
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

		// Parse the false branch (do NOT allow nested ternaries)
		falseBranch := p.parseOrExpression()

		// Check for nested/daisy-chained ternary (second ?? in false branch)
		if p.current().Type == TOKEN_TERNARY {
			if !p.LintMode {
				panic(fmt.Sprintf("Daisy chained/nested ternaries are not allowed in ahoy at line %d ; use inline switch e.g (row:int=switch rot:on 0: -1; on 2: 1; _: 0;$)", p.current().Line))
			}
			p.recordError("Daisy chained/nested ternaries are not allowed in ahoy; consider inline switch (row:int=switch rot:on 0: -1; on 2: 1; _: 0;$) ")
			return condition
		}

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

		// Check for "is not" pattern
		isNegated := false
		if p.current().Type == TOKEN_NOT {
			isNegated = true
			p.advance()
		}

		right := p.parseRelationalExpression()

		comparison := &ASTNode{
			Type:     NODE_BINARY_OP,
			Value:    op.Value,
			Children: []*ASTNode{left, right},
		}

		// If "is not", wrap in NOT node
		if isNegated {
			left = &ASTNode{
				Type:     NODE_UNARY_OP,
				Value:    "not",
				Children: []*ASTNode{comparison},
			}
		} else {
			left = comparison
		}
	}

	return left
}

func (p *Parser) parseRelationalExpression() *ASTNode {
	left := p.parseAdditiveExpression()

	for p.current().Type == TOKEN_LANGLE || p.current().Type == TOKEN_RANGLE ||
		p.current().Type == TOKEN_LESS_EQUAL || p.current().Type == TOKEN_GREATER_EQUAL ||
		p.current().Type == TOKEN_LESSER_WORD || p.current().Type == TOKEN_GREATER_WORD {

		// Check if this is dict access (identifier<"key">) instead of comparison
		if p.current().Type == TOKEN_LANGLE && left.Type == NODE_IDENTIFIER {
			// Lookahead to determine if this is dict access or comparison
			nextToken := p.peek(1)
			peek2 := p.peek(2)

			isDictAccess := false
			if nextToken.Type == TOKEN_STRING && peek2.Type == TOKEN_RANGLE {
				// dict<"key"> pattern
				isDictAccess = true
			} else if nextToken.Type == TOKEN_IDENTIFIER && peek2.Type == TOKEN_RANGLE {
				// dict<varKey> pattern
				isDictAccess = true
			} else if (nextToken.Type == TOKEN_STRING || nextToken.Type == TOKEN_IDENTIFIER) && peek2.Type == TOKEN_ASSIGN {
				// Special type or dict literal: <key: value>
				isDictAccess = true
			}

			if isDictAccess {
				// Parse as dict access, not comparison
				p.advance() // consume <

				// Parse dict access: dict<key>
				key := p.parseCallArgument()
				p.expect(TOKEN_RANGLE)

				left = &ASTNode{
					Type:     NODE_DICT_ACCESS,
					Value:    left.Value,
					Children: []*ASTNode{key},
					Line:     left.Line,
				}

				// Continue to check for more operations
				continue
			}
		}

		// Don't treat > as comparison operator if we're inside angle brackets for object/array access
		// Check if the next token suggests we're closing an access expression
		if p.current().Type == TOKEN_RANGLE && p.inFunctionCall > 0 {
			// If we're in a function call (like print|obj<"prop">|), stop parsing
			// The > is likely closing an object/array access, not a comparison
			break
		}

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
	left := p.parsePowerExpression()

	for p.current().Type == TOKEN_MULTIPLY || p.current().Type == TOKEN_DIVIDE ||
		p.current().Type == TOKEN_MODULO || p.current().Type == TOKEN_TIMES_WORD ||
		p.current().Type == TOKEN_DIV_WORD || p.current().Type == TOKEN_MOD_WORD {
		op := p.current()
		p.advance()
		right := p.parsePowerExpression()
		left = &ASTNode{
			Type:     NODE_BINARY_OP,
			Value:    op.Value,
			Children: []*ASTNode{left, right},
		}
	}

	return left
}

func (p *Parser) parsePowerExpression() *ASTNode {
	left := p.parseUnaryExpression()

	// Power operator is right-associative (2 ** 3 ** 2 = 2 ** 9 = 512)
	for p.current().Type == TOKEN_POWER || p.current().Type == TOKEN_POW_WORD {
		op := p.current()
		p.advance()
		right := p.parsePowerExpression() // Right-associative recursion
		left = &ASTNode{
			Type:     NODE_BINARY_OP,
			Value:    "pow", // Normalize to "pow" for code generation
			Children: []*ASTNode{left, right},
		}
		_ = op // Suppress unused warning
	}

	return left
}

func (p *Parser) parseUnaryExpression() *ASTNode {
	if p.current().Type == TOKEN_NOT || p.current().Type == TOKEN_MINUS ||
		p.current().Type == TOKEN_CARET || p.current().Type == TOKEN_AMPERSAND {
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
