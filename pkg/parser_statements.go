package ahoy

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Statement parsing: blocks, control flow, loops and imports.

func (p *Parser) parseStatement() *ASTNode {
	switch p.current().Type {
	case TOKEN_PROGRAM:
		return p.parseProgramDeclaration()
	case TOKEN_ENUM:
		return p.parseEnumDeclaration()
	case TOKEN_STRUCT:
		// Check for struct:json syntax
		if p.peek(1).Type == TOKEN_ASSIGN && p.peek(2).Value == "json" {
			return p.parseJsonStructDeclaration()
		}
		return p.parseStructDeclaration()
	case TOKEN_ALIAS:
		return p.parseAliasDeclaration()
	case TOKEN_UNION:
		return p.parseUnionDeclaration()
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
	case TOKEN_LOG:
		return p.parseLogStatement()
	case TOKEN_PANIC:
		return p.parsePanicStatement()
	case TOKEN_RETURN:
		return p.parseReturnStatement()
	case TOKEN_HALT:
		p.advance()
		return &ASTNode{Type: NODE_HALT, Line: p.current().Line}
	case TOKEN_NEXT:
		p.advance()
		return &ASTNode{Type: NODE_NEXT, Line: p.current().Line}
	case TOKEN_GOTO:
		return p.parseGotoStatement()
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
	case TOKEN_AT:
		return p.parseFunctionDeclaration()
	case TOKEN_IDENTIFIER:
		// Check for constant declaration (name ::)
		nextType := p.peek(1).Type
		if nextType == TOKEN_DOUBLE_COLON {
			return p.parseConstantDeclaration()
		}
		// Check for walrus operator (name :=) - inferred type assignment
		if nextType == TOKEN_WALRUS {
			return p.parseWalrusAssignment()
		}
		// Check for tuple assignment (name, name :)
		if nextType == TOKEN_COMMA {
			return p.parseTupleAssignment()
		}
		// Check for label declaration (name: followed by newline/indent)
		if nextType == TOKEN_ASSIGN {
			nextToken := p.peek(1)
			if nextToken.Value == ":" {
				// Check if followed by newline/indent (label) or expression (ternary/assignment)
				afterColon := p.peek(2).Type
				if afterColon == TOKEN_NEWLINE || afterColon == TOKEN_INDENT {
					return p.parseLabelDeclaration()
				}
			}
		}
		stmt := p.parseAssignmentOrExpression()
		// Validate no function calls at global scope when program is declared
		if p.LintMode && p.hasProgramDecl && !p.inFunctionBody && stmt != nil {
			p.validateNoGlobalFunctionCalls(stmt)
		}
		return stmt
	case TOKEN_CARET, TOKEN_AMPERSAND:
		// Could be unary expression assignment like ^ptr: value
		return p.parseAssignmentOrExpression()
	case TOKEN_NEWLINE, TOKEN_SEMICOLON:
		p.advance()
		return nil
	default:
		return p.parseAssignmentOrExpression()
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

		// Skip any newlines after opening pipe
		for p.current().Type == TOKEN_NEWLINE {
			p.advance()
		}

		for p.current().Type != TOKEN_PIPE && p.current().Type != TOKEN_EOF {
			// Skip newlines between parameters
			for p.current().Type == TOKEN_NEWLINE {
				p.advance()
			}

			// Check if we've reached the closing pipe
			if p.current().Type == TOKEN_PIPE {
				break
			}

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

			// Skip any newlines before comma or closing pipe
			for p.current().Type == TOKEN_NEWLINE {
				p.advance()
			}

			if p.current().Type == TOKEN_COMMA {
				p.advance()
				// Skip any newlines after comma
				for p.current().Type == TOKEN_NEWLINE {
					p.advance()
				}
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

	var condition *ASTNode

	// Check for "is i not" error pattern (starts with IS)
	if p.current().Type == TOKEN_IS {
		p.recordError("expected is not got is i not")
		// Consume tokens to recover
		p.advance() // consume is
		if p.current().Type == TOKEN_IDENTIFIER {
			p.advance() // consume identifier
		}
		if p.current().Type == TOKEN_NOT {
			p.advance() // consume not
		}
		// Create dummy condition to continue parsing
		condition = &ASTNode{Type: NODE_BOOLEAN, Value: "false"}
	} else {
		condition = p.parseExpression()
	}

	// Only accept 'then' for if statements
	if p.current().Type == TOKEN_THEN {
		p.advance()
	} else if p.current().Type == TOKEN_NOT {
		// Check for "not is" error
		if p.peek(1).Type == TOKEN_IS {
			p.recordError("expected is not got not is")
			p.advance() // consume not
			p.advance() // consume is
		} else {
			current := p.current()
			errMsg := fmt.Sprintf("Expected 'then', got %s at line %d:%d",
				tokenTypeName(current.Type), current.Line, current.Column)
			if p.LintMode {
				p.recordError(errMsg)
			} else {
				panic(errMsg)
			}
		}
	} else {
		current := p.current()
		errMsg := fmt.Sprintf("Expected 'then', got %s at line %d:%d",
			tokenTypeName(current.Type), current.Line, current.Column)
		if p.LintMode {
			p.recordError(errMsg)
		} else {
			panic(errMsg)
		}
	}

	// Both inline and multiline now require $ to close
	// Skip optional newlines after then/do/colon
	for p.current().Type == TOKEN_NEWLINE {
		p.advance()
	}

	// Save parent scope ONCE for entire if/else statement
	// IMPORTANT: Must make a COPY since maps are reference types
	var parentScope map[string]int
	var parentFunctionScope map[string]string
	if p.LintMode {
		parentScope = make(map[string]int)
		for k, v := range p.declaredVars {
			parentScope[k] = v
		}
		// Also save functionScope
		if p.functionScope != nil {
			parentFunctionScope = make(map[string]string)
			for k, v := range p.functionScope {
				parentFunctionScope[k] = v
			}
		}
	}

	p.skipWhitespace()
	p.blockDepth++ // Opening a block (inline or multiline)

	// Create branch scope for if (copy of parent)
	if p.LintMode {
		branchScope := make(map[string]int)
		for k, v := range parentScope {
			branchScope[k] = v
		}
		p.declaredVars = branchScope

		// Also create branch function scope
		if parentFunctionScope != nil {
			branchFunctionScope := make(map[string]string)
			for k, v := range parentFunctionScope {
				branchFunctionScope[k] = v
			}
			p.functionScope = branchFunctionScope
		}
	}
	ifBody := p.parseBlockUntilEnd("if", startLine)

	// Restore parent scope after if branch (before checking for elseif/else)
	// Make a COPY since maps are reference types
	if p.LintMode {
		p.declaredVars = make(map[string]int)
		for k, v := range parentScope {
			p.declaredVars[k] = v
		}
		// Also restore functionScope
		if parentFunctionScope != nil {
			p.functionScope = make(map[string]string)
			for k, v := range parentFunctionScope {
				p.functionScope[k] = v
			}
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

		// Only accept 'then' for elseif
		if p.current().Type == TOKEN_THEN {
			p.advance()
		} else {
			current := p.current()
			errMsg := fmt.Sprintf("Expected 'then', got %s at line %d:%d",
				tokenTypeName(current.Type), current.Line, current.Column)
			if p.LintMode {
				p.recordError(errMsg)
			} else {
				panic(errMsg)
			}
		}

		// Both inline and multiline now require $ to close
		// Skip optional newlines after then
		for p.current().Type == TOKEN_NEWLINE {
			p.advance()
		}

		p.skipWhitespace()
		p.blockDepth++ // Opening a block (inline or multiline)

		// Create branch scope for elseif (copy of parent)
		if p.LintMode {
			branchScope := make(map[string]int)
			for k, v := range parentScope {
				branchScope[k] = v
			}
			p.declaredVars = branchScope

			if parentFunctionScope != nil {
				branchFunctionScope := make(map[string]string)
				for k, v := range parentFunctionScope {
					branchFunctionScope[k] = v
				}
				p.functionScope = branchFunctionScope
			}
		}
		elseifBody := p.parseBlockUntilEnd("anif", startLine)

		// Restore parent scope after elseif branch (make a copy)
		if p.LintMode {
			p.declaredVars = make(map[string]int)
			for k, v := range parentScope {
				p.declaredVars[k] = v
			}
			if parentFunctionScope != nil {
				p.functionScope = make(map[string]string)
				for k, v := range parentFunctionScope {
					p.functionScope[k] = v
				}
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

		// Optional ':' or 'then' after else
		if p.current().Type == TOKEN_ASSIGN || p.current().Type == TOKEN_THEN {
			p.advance()
		}

		// Both inline and multiline now require $ to close
		// Skip optional newlines after colon
		for p.current().Type == TOKEN_NEWLINE {
			p.advance()
		}

		p.skipWhitespace()
		p.blockDepth++ // Opening a block (inline or multiline)

		// Create branch scope for else (copy of parent)
		if p.LintMode {
			branchScope := make(map[string]int)
			for k, v := range parentScope {
				branchScope[k] = v
			}
			p.declaredVars = branchScope

			if parentFunctionScope != nil {
				branchFunctionScope := make(map[string]string)
				for k, v := range parentFunctionScope {
					branchFunctionScope[k] = v
				}
				p.functionScope = branchFunctionScope
			}
		}
		elseBody := p.parseBlockUntilEnd("else", startLine)

		ifStmt.Children = append(ifStmt.Children, elseBody)
	}

	// Restore parent scope after all branches
	if p.LintMode {
		p.declaredVars = parentScope
	}
	// Note: parseBlockUntilEnd already consumes the $ and decrements blockDepth

	return ifStmt
}

func (p *Parser) parseSwitchStatement() *ASTNode {
	startLine := p.current().Line
	p.expect(TOKEN_SWITCH)
	expr := p.parseExpression()

	// Expect ':' after switch expression
	if p.current().Type == TOKEN_ASSIGN { // colon
		p.advance()
	} else {
		errMsg := fmt.Sprintf("Expected ':' after switch expression at line %d", p.current().Line)
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

	// Save parent scope for all switch cases
	var switchParentScope map[string]int
	var switchParentFunctionScope map[string]string
	if p.LintMode {
		switchParentScope = make(map[string]int)
		for k, v := range p.declaredVars {
			switchParentScope[k] = v
		}
		if p.functionScope != nil {
			switchParentFunctionScope = make(map[string]string)
			for k, v := range p.functionScope {
				switchParentFunctionScope[k] = v
			}
		}
	}

	// Parse cases: each case starts with 'on' keyword (except default case with '_')
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

		// Skip newlines, semicolons, indents, and dedents (indentation is cosmetic in switch)
		for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_SEMICOLON ||
			p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
			p.advance()
		}

		// Check for end of switch
		if p.current().Type == TOKEN_END || p.current().Type == TOKEN_EOF {
			break
		}

		// Check if we have 'on' keyword or '_' for default case
		isDefaultCase := false
		if p.current().Type == TOKEN_ON {
			p.advance() // consume 'on'
		} else if p.current().Type == TOKEN_IDENTIFIER && p.current().Value == "_" {
			isDefaultCase = true
		} else {
			// No more cases to parse
			break
		}

		// Parse case values - could be single, list (with commas), or range (with 'to')
		caseValues := []*ASTNode{}

		if !isDefaultCase {
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
				} else if p.current().Type == TOKEN_DOT {
					// New syntax: .MEMBER (dot-prefixed enum member)
					p.advance() // consume .
					if p.current().Type == TOKEN_IDENTIFIER {
						member := p.current()
						p.advance()
						// Create a special node to indicate this is a dot-prefixed enum member
						// The codegen will need to infer the enum type from context
						caseValue = &ASTNode{
							Type:  NODE_IDENTIFIER,
							Value: "." + member.Value, // Store with dot prefix to indicate it's enum member
						}
					} else {
						// Malformed - skip
						break
					}
				} else if p.current().Type == TOKEN_IDENTIFIER {
					tok := p.current()
					p.advance()

					// Check if this is enum.MEMBER syntax
					if p.current().Type == TOKEN_DOT {
						p.advance() // consume .
						if p.current().Type == TOKEN_IDENTIFIER {
							member := p.current()
							p.advance()
							// Create member access node
							// Structure: Value = member name, Children[0] = object (enum name)
							caseValue = &ASTNode{
								Type:  NODE_MEMBER_ACCESS,
								Value: member.Value, // member name
								Children: []*ASTNode{
									{
										Type:  NODE_IDENTIFIER,
										Value: tok.Value, // enum name
									},
								},
							}
						} else {
							// Malformed - just use the enum name
							caseValue = &ASTNode{
								Type:  NODE_IDENTIFIER,
								Value: tok.Value,
							}
						}
					} else {
						// Just an identifier without prefix - this should now be an error for user-defined enums
						caseValue = &ASTNode{
							Type:  NODE_IDENTIFIER,
							Value: tok.Value,
						}

						// In lint mode, report error for bare enum members
						if p.LintMode {
							// Check if this looks like an enum member (all uppercase)
							if isScreamingSnakeCase(tok.Value) {
								p.recordErrorAtLine(fmt.Sprintf("Enum member '%s' must be prefixed with enum name (e.g., EnumName.%s) or dot (e.g., .%s)", tok.Value, tok.Value, tok.Value), tok.Line)
							}
						}
					}
				} else {
					// Unexpected token - break out to avoid infinite loop
					break
				}

				caseValues = append(caseValues, caseValue)

				// Check for range ('to' keyword) or multiple values (',')
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
				} else if p.current().Type == TOKEN_COMMA {
					// Multiple values: 'A','B','C'
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
		} else {
			// Default case with '_'
			tok := p.current()
			p.advance()
			caseValues = append(caseValues, &ASTNode{
				Type:  NODE_IDENTIFIER,
				Value: tok.Value,
			})
		}

		caseLine := p.current().Line // Track line number for error reporting
		p.expect(TOKEN_ASSIGN)       // Expect :

		// Skip optional whitespace/indent after colon
		p.skipWhitespace()

		// Skip whitespace/newlines after colon - indentation is cosmetic
		for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT {
			p.advance()
		}

		// Parse case body - could be multiple statements or single expression
		var caseBody *ASTNode

		// Create case scope (copy of parent) - allows same variable names in parallel cases
		if p.LintMode {
			caseScope := make(map[string]int)
			for k, v := range switchParentScope {
				caseScope[k] = v
			}
			p.declaredVars = caseScope

			if switchParentFunctionScope != nil {
				caseFunctionScope := make(map[string]string)
				for k, v := range switchParentFunctionScope {
					caseFunctionScope[k] = v
				}
				p.functionScope = caseFunctionScope
			}
		}

		// Check if we're starting a new case immediately (empty case)
		if p.current().Type == TOKEN_ON || (p.current().Type == TOKEN_IDENTIFIER && p.current().Value == "_") ||
			p.current().Type == TOKEN_END || p.current().Type == TOKEN_DEDENT {
			// Empty case body
			caseBody = &ASTNode{Type: NODE_BLOCK}
		} else {
			// Parse statements/expressions until we hit next case or end
			statements := []*ASTNode{}

			for {
				// Skip cosmetic tokens (including semicolons for inline case statements)
				for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT ||
					p.current().Type == TOKEN_DEDENT || p.current().Type == TOKEN_SEMICOLON {
					p.advance()
				}

				// Check for end of case
				if p.current().Type == TOKEN_ON || (p.current().Type == TOKEN_IDENTIFIER && p.current().Value == "_") ||
					p.current().Type == TOKEN_END || p.current().Type == TOKEN_EOF {
					break
				}

				// Parse one statement/expression
				var stmt *ASTNode
				switch p.current().Type {
				case TOKEN_AHOY:
					stmt = p.parseAhoyStatement()
				case TOKEN_PRINT:
					stmt = p.parsePrintStatement()
				case TOKEN_LOG:
					stmt = p.parseLogStatement()
				case TOKEN_PANIC:
					stmt = p.parsePanicStatement()
				case TOKEN_RETURN:
					stmt = p.parseReturnStatement()
				case TOKEN_IF:
					stmt = p.parseIfStatement()
				case TOKEN_SWITCH:
					stmt = p.parseSwitchStatement()
				case TOKEN_LOOP:
					stmt = p.parseLoop()
				case TOKEN_IDENTIFIER:
					// Could be assignment or expression
					stmt = p.parseAssignmentOrExpression()
				default:
					// Try to parse as expression (potentially tuple)
					stmt = p.parseSwitchCaseExpression()
				}

				// Append statement and check for end of case
				if stmt != nil {
					statements = append(statements, stmt)

					// Skip trailing cosmetic tokens (including semicolons)
					for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_DEDENT || p.current().Type == TOKEN_SEMICOLON {
						p.advance()
					}

					// Check if we're at the end of this case
					if p.current().Type == TOKEN_ON || (p.current().Type == TOKEN_IDENTIFIER && p.current().Value == "_") ||
						p.current().Type == TOKEN_END || p.current().Type == TOKEN_EOF {
						break
					}
				} else {
					break
				}
			}

			// If multiple statements, wrap in block; otherwise return single statement
			if len(statements) > 1 {
				caseBody = &ASTNode{
					Type:     NODE_BLOCK,
					Children: statements,
					Line:     caseLine,
				}
			} else if len(statements) == 1 {
				caseBody = statements[0]
			} else {
				caseBody = &ASTNode{Type: NODE_BLOCK}
			}
		}

		// Restore parent scope after case body (for next case)
		if p.LintMode {
			p.declaredVars = make(map[string]int)
			for k, v := range switchParentScope {
				p.declaredVars[k] = v
			}
			if switchParentFunctionScope != nil {
				p.functionScope = make(map[string]string)
				for k, v := range switchParentFunctionScope {
					p.functionScope[k] = v
				}
			}
		}

		// Create case node with line number for error reporting
		var caseNode *ASTNode
		if len(caseValues) == 1 {
			caseNode = &ASTNode{
				Type:     NODE_SWITCH_CASE,
				Children: []*ASTNode{caseValues[0], caseBody},
				Line:     caseLine,
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
				Line:     caseLine,
			}
		}

		switchStmt.Children = append(switchStmt.Children, caseNode)

		// Skip newlines between cases
		for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_SEMICOLON {
			p.advance()
		}
		// Continue to next case
	}

	// Consume final dedent and '$' to close switch statement
	if p.current().Type == TOKEN_DEDENT {
		p.advance()
	}
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

	return switchStmt
}

// validateSwitchReturnTypes checks that all cases in a switch return compatible types
func (p *Parser) validateSwitchReturnTypes(switchStmt *ASTNode, line int) {
	p.validateSwitchReturnTypesWithExpected(switchStmt, "", line)
}

// validateSwitchReturnTypesWithExpected checks switch return types against an expected type
func (p *Parser) validateSwitchReturnTypesWithExpected(switchStmt *ASTNode, expectedType string, line int) {
	if switchStmt == nil || len(switchStmt.Children) < 2 {
		return // Need at least expression + one case
	}

	// Get all case bodies with their line numbers (skip first child which is the switch expression)
	type caseInfo struct {
		typeName string
		line     int
	}
	var cases []caseInfo

	for i := 1; i < len(switchStmt.Children); i++ {
		caseNode := switchStmt.Children[i]
		if caseNode.Type != NODE_SWITCH_CASE || len(caseNode.Children) < 2 {
			continue
		}

		// The last child is the body
		body := caseNode.Children[len(caseNode.Children)-1]
		bodyType := p.inferCaseBodyType(body)
		caseLine := caseNode.Line
		if caseLine == 0 {
			caseLine = line // Fallback to switch line
		}
		cases = append(cases, caseInfo{typeName: bodyType, line: caseLine})
	}

	if len(cases) == 0 {
		return // No cases to validate
	}

	// If we have an expected type, validate each case against it
	if expectedType != "" {
		for _, caseData := range cases {
			if caseData.typeName == "void" {
				errMsg := fmt.Sprintf("Returns void but expected type '%s'", expectedType)
				p.recordErrorAtLine(errMsg, caseData.line)
			} else if !p.checkTypeCompatibility(expectedType, caseData.typeName) {
				errMsg := fmt.Sprintf("Returns type '%s' but expected type '%s'", caseData.typeName, expectedType)
				p.recordErrorAtLine(errMsg, caseData.line)
			}
		}
		return
	}

	// Otherwise, check if all types are compatible with each other
	firstType := cases[0].typeName
	for _, caseData := range cases {
		if caseData.typeName == "void" {
			errMsg := fmt.Sprintf("Returns void - switch expressions must return values")
			p.recordErrorAtLine(errMsg, caseData.line)
		} else if firstType != "unknown" && caseData.typeName != "unknown" && firstType != caseData.typeName {
			// Allow int to float promotion
			if !((firstType == "float" && caseData.typeName == "int") || (firstType == "int" && caseData.typeName == "float")) {
				errMsg := fmt.Sprintf("Returns type '%s' but other cases return '%s' - all cases must return the same type", caseData.typeName, firstType)
				p.recordErrorAtLine(errMsg, caseData.line)
			}
		}
	}
}

// parseSwitchCaseExpression parses a switch case body, supporting tuple expressions
func (p *Parser) parseSwitchCaseExpression() *ASTNode {
	// Parse first expression
	firstExpr := p.parseExpression()

	// Check if there's a comma (tuple expression)
	if p.current().Type == TOKEN_COMMA {
		// This is a tuple expression
		tupleNode := &ASTNode{
			Type:     NODE_BLOCK, // Use BLOCK to hold multiple expressions
			Children: []*ASTNode{firstExpr},
		}

		for p.current().Type == TOKEN_COMMA {
			p.advance() // consume comma

			// Check for newline (end of case) or other terminators
			if p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_DEDENT ||
				p.current().Type == TOKEN_ON || p.current().Type == TOKEN_END ||
				(p.current().Type == TOKEN_IDENTIFIER && p.current().Value == "_") {
				break
			}

			expr := p.parseExpression()
			tupleNode.Children = append(tupleNode.Children, expr)
		}

		return tupleNode
	}

	return firstExpr
}

// parseSwitchCaseBlock parses a multi-line switch case body
func (p *Parser) parseSwitchCaseBlock(startLine int) *ASTNode {
	block := &ASTNode{Type: NODE_BLOCK, Line: startLine}

	maxIterations := 10000
	iterations := 0

	for {
		iterations++
		if iterations > maxIterations {
			errMsg := fmt.Sprintf("Parser safety limit reached while parsing switch case at line %d", startLine)
			if p.LintMode {
				p.recordError(errMsg)
			} else {
				panic(errMsg)
			}
			break
		}

		// Check for end of case body
		if p.current().Type == TOKEN_DEDENT || p.current().Type == TOKEN_ON ||
			(p.current().Type == TOKEN_IDENTIFIER && p.current().Value == "_") ||
			p.current().Type == TOKEN_END || p.current().Type == TOKEN_EOF {
			break
		}

		// Skip newlines and indents (indentation is cosmetic in switch bodies)
		if p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT {
			p.advance()
			continue
		}

		// Parse statement
		stmt := p.parseStatement()
		if stmt != nil {
			block.Children = append(block.Children, stmt)
		}
	}

	// Don't consume dedent here - let main switch loop handle it
	return block
}

// inferSwitchReturnType infers the return type of a switch statement from its cases
func (p *Parser) inferSwitchReturnType(switchStmt *ASTNode) string {
	if switchStmt == nil || len(switchStmt.Children) < 2 {
		return "unknown"
	}

	// Get the type of the first case
	for i := 1; i < len(switchStmt.Children); i++ {
		caseNode := switchStmt.Children[i]
		if caseNode.Type != NODE_SWITCH_CASE || len(caseNode.Children) < 2 {
			continue
		}

		body := caseNode.Children[len(caseNode.Children)-1]
		bodyType := p.inferCaseBodyType(body)
		if bodyType != "unknown" && bodyType != "void" {
			return bodyType
		}
	}

	return "unknown"
}

// inferCaseBodyType infers the type returned by a case body
func (p *Parser) inferCaseBodyType(body *ASTNode) string {
	if body == nil {
		return "unknown"
	}

	switch body.Type {
	case NODE_BLOCK:
		// For multi-line blocks, infer from last statement
		if len(body.Children) == 0 {
			return "void"
		}
		// Check if all statements are void (like multiple prints)
		allVoid := true
		for _, stmt := range body.Children {
			stmtType := p.inferCaseBodyType(stmt)
			if stmtType != "void" {
				allVoid = false
				break
			}
		}
		if allVoid {
			return "void"
		}
		// Return type of last statement
		return p.inferCaseBodyType(body.Children[len(body.Children)-1])
	case NODE_CALL:
		// Check for built-in functions that return void
		if body.Value == "print" || body.Value == "ahoy" {
			return "void"
		}
		// Check if it's a known function
		if funcSig, ok := p.functions[body.Value]; ok {
			if len(funcSig.ReturnTypes) == 0 {
				return "void"
			}
			if len(funcSig.ReturnTypes) > 0 {
				return funcSig.ReturnTypes[0]
			}
		}
		return p.inferType(body)
	case NODE_SWITCH_STATEMENT:
		// For nested switches, infer the return type from its cases
		return p.inferSwitchReturnType(body)
	case NODE_IF_STATEMENT, NODE_WHILE_LOOP, NODE_FOR_LOOP:
		// These statements don't return values
		return "void"
	default:
		return p.inferType(body)
	}
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

			// Only accept 'do'
			if p.current().Type == TOKEN_DO {
				p.advance()
			} else {
				if !p.LintMode {
					panic(fmt.Sprintf("Expected 'do' after loop range at line %d", p.current().Line))
				}
				p.recordError("Expected 'do' after loop range")
			}

			// Register loop variable in scope
			if loopVar != nil {
				loopScope := make(map[string]string)
				loopScope[loopVar.Value] = "int"
				p.loopVarScopes = append(p.loopVarScopes, loopScope)
			}

			// Both inline and multiline now require $ to close
			// Skip optional newlines after do/colon
			for p.current().Type == TOKEN_NEWLINE {
				p.advance()
			}

			p.skipWhitespace()
			p.blockDepth++ // Opening a block (inline or multiline)
			body := p.parseBlockUntilEnd("loop", startLine)

			// Pop loop variable scope
			if loopVar != nil && len(p.loopVarScopes) > 0 {
				p.loopVarScopes = p.loopVarScopes[:len(p.loopVarScopes)-1]
			}

			loopVarNode := &ASTNode{
				Type:   NODE_IDENTIFIER,
				Value:  loopVar.Value,
				Line:   loopVar.Line,
				Column: loopVar.Column,
			}
			return &ASTNode{
				Type:     NODE_FOR_RANGE_LOOP,
				Children: []*ASTNode{loopVarNode, startExpr, endExpr, body},
				Line:     startLine,
			}
		} else if p.current().Type == TOKEN_TILL {
			// loop i:start till condition
			p.advance() // consume 'till'
			condition := p.parseExpression()

			// Only accept 'do'
			if p.current().Type == TOKEN_DO {
				p.advance()
			} else {
				if !p.LintMode {
					panic(fmt.Sprintf("Expected 'do' after loop condition at line %d", p.current().Line))
				}
				p.recordError("Expected 'do' after loop condition")
			}

			// Register loop variable in scope
			if loopVar != nil {
				loopScope := make(map[string]string)
				loopScope[loopVar.Value] = "int"
				p.loopVarScopes = append(p.loopVarScopes, loopScope)
			}

			// Both inline and multiline now require $ to close
			// Skip optional newlines after do/colon
			for p.current().Type == TOKEN_NEWLINE {
				p.advance()
			}

			p.skipWhitespace()
			p.blockDepth++ // Opening a block (inline or multiline)
			body := p.parseBlockUntilEnd("loop", startLine)

			// Pop loop variable scope
			if loopVar != nil && len(p.loopVarScopes) > 0 {
				p.loopVarScopes = p.loopVarScopes[:len(p.loopVarScopes)-1]
			}

			loopVarNode := &ASTNode{
				Type:   NODE_IDENTIFIER,
				Value:  loopVar.Value,
				Line:   loopVar.Line,
				Column: loopVar.Column,
			}
			return &ASTNode{
				Type:     NODE_WHILE_LOOP,
				Children: []*ASTNode{loopVarNode, startExpr, condition, body},
				Line:     startLine,
			}
		} else if p.current().Type == TOKEN_ASSIGN {
			// loop i:start: (forever loop with counter starting at start)
			p.advance() // consume second ':'

			// Both inline and multiline now require $ to close
			// Skip optional newlines after second colon
			for p.current().Type == TOKEN_NEWLINE {
				p.advance()
			}

			p.skipWhitespace()
			p.blockDepth++ // Opening a block (inline or multiline)
			body := p.parseBlockUntilEnd("loop", startLine)

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

		// Only accept 'do'
		if p.current().Type == TOKEN_DO {
			p.advance()
		} else if p.current().Type == TOKEN_ASSIGN {
			p.advance()
		} else {
			if !p.LintMode {
				panic(fmt.Sprintf("Expected 'do' after loop range at line %d", p.current().Line))
			}
			p.recordError("Expected 'do' after loop range")
		}

		// Both inline and multiline now require $ to close
		for p.current().Type == TOKEN_NEWLINE {
			p.advance()
		}
		p.skipWhitespace()
		p.blockDepth++
		body := p.parseBlockUntilEnd("loop", startLine)

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

		// Only accept 'do'
		if p.current().Type == TOKEN_DO {
			p.advance()
		} else if p.current().Type == TOKEN_ASSIGN {
			p.advance()
		} else {
			if !p.LintMode {
				panic(fmt.Sprintf("Expected 'do' after loop range at line %d", p.current().Line))
			}
			p.recordError("Expected 'do' after loop range")
		}

		// Both inline and multiline now require $ to close
		for p.current().Type == TOKEN_NEWLINE {
			p.advance()
		}
		p.skipWhitespace()
		p.blockDepth++
		body := p.parseBlockUntilEnd("loop", startLine)

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

		// Only accept 'do'
		if p.current().Type == TOKEN_DO {
			p.advance()
		} else if p.current().Type == TOKEN_ASSIGN {
			p.advance()
		} else {
			if !p.LintMode {
				panic(fmt.Sprintf("Expected 'do' after loop condition at line %d", p.current().Line))
			}
			p.recordError("Expected 'do' after loop condition")
		}

		// Both inline and multiline now require $ to close
		for p.current().Type == TOKEN_NEWLINE {
			p.advance()
		}
		p.skipWhitespace()
		p.blockDepth++
		body := p.parseBlockUntilEnd("loop", startLine)

		if loopVar != nil {
			// loop i till condition - i should be initialized to 0 locally
			loopVarNode := &ASTNode{
				Type:   NODE_IDENTIFIER,
				Value:  loopVar.Value,
				Line:   loopVar.Line,
				Column: loopVar.Column,
			}
			zeroNode := &ASTNode{Type: NODE_NUMBER, Value: "0"}
			return &ASTNode{
				Type:     NODE_WHILE_LOOP,
				Children: []*ASTNode{loopVarNode, zeroNode, condition, body},
				Line:     startLine,
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

		// Only accept 'do'
		if p.current().Type == TOKEN_DO {
			p.advance()
		} else if p.current().Type == TOKEN_ASSIGN {
			p.advance()
		} else {
			if !p.LintMode {
				panic(fmt.Sprintf("Expected 'do' after 'in' expression at line %d", p.current().Line))
			}
			p.recordError("Expected 'do' after 'in' expression")
		}

		// Infer element type from collection
		loopScope := make(map[string]string)
		collectionType := p.inferType(collectionExpr)

		// If it's an array type like "array[string]", extract the element type
		if strings.HasPrefix(collectionType, "array[") && strings.HasSuffix(collectionType, "]") {
			elementType := collectionType[6 : len(collectionType)-1] // Extract type between [ and ]
			loopScope[loopVar.Value] = elementType
		} else {
			// Default to unknown for non-typed arrays or other collections
			loopScope[loopVar.Value] = "unknown"
		}
		p.loopVarScopes = append(p.loopVarScopes, loopScope)

		// Both inline and multiline now require $ to close
		for p.current().Type == TOKEN_NEWLINE {
			p.advance()
		}
		p.skipWhitespace()
		p.blockDepth++
		body := p.parseBlockUntilEnd("loop", startLine)

		// Pop loop variable scope
		if len(p.loopVarScopes) > 0 {
			p.loopVarScopes = p.loopVarScopes[:len(p.loopVarScopes)-1]
		}

		elementNode := &ASTNode{
			Type:   NODE_IDENTIFIER,
			Value:  loopVar.Value,
			Line:   loopVar.Line,
			Column: loopVar.Column,
		}
		return &ASTNode{
			Type:     NODE_FOR_IN_ARRAY_LOOP,
			Children: []*ASTNode{elementNode, collectionExpr, body},
			Line:     startLine,
		}
	} else if loopVar != nil && p.current().Type == TOKEN_COMMA {
		// loop key,value in dict
		p.advance() // consume ','
		secondIdent := p.expect(TOKEN_IDENTIFIER)
		p.expect(TOKEN_IN)
		dictExpr := p.parseExpression()

		// Only accept 'do'
		if p.current().Type == TOKEN_DO {
			p.advance()
		} else if p.current().Type == TOKEN_ASSIGN {
			p.advance()
		} else {
			if !p.LintMode {
				panic(fmt.Sprintf("Expected 'do' after 'in' expression at line %d", p.current().Line))
			}
			p.recordError("Expected 'do' after 'in' expression")
		}

		// Both inline and multiline now require $ to close
		for p.current().Type == TOKEN_NEWLINE {
			p.advance()
		}
		p.skipWhitespace()
		p.blockDepth++
		body := p.parseBlockUntilEnd("loop", startLine)

		keyNode := &ASTNode{
			Type:   NODE_IDENTIFIER,
			Value:  loopVar.Value,
			Line:   loopVar.Line,
			Column: loopVar.Column,
		}
		valueNode := &ASTNode{
			Type:   NODE_IDENTIFIER,
			Value:  secondIdent.Value,
			Line:   secondIdent.Line,
			Column: secondIdent.Column,
		}
		return &ASTNode{
			Type:     NODE_FOR_IN_DICT_LOOP,
			Children: []*ASTNode{keyNode, valueNode, dictExpr, body},
			Line:     startLine,
		}
	} else if p.current().Type == TOKEN_DO || p.current().Type == TOKEN_ASSIGN {
		// loop [i] do or loop [i] : - infinite loop, optionally with counter
		p.advance() // consume 'do' or ':'

		// Both inline and multiline now require $ to close
		for p.current().Type == TOKEN_NEWLINE {
			p.advance()
		}
		p.skipWhitespace()
		p.blockDepth++
		body := p.parseBlockUntilEnd("loop", startLine)

		if loopVar != nil {
			// loop i do - i starts at 0, increments each iteration
			loopVarNode := &ASTNode{
				Type:   NODE_IDENTIFIER,
				Value:  loopVar.Value,
				Line:   loopVar.Line,
				Column: loopVar.Column,
			}
			zeroNode := &ASTNode{Type: NODE_NUMBER, Value: "0"}
			return &ASTNode{
				Type:     NODE_FOR_COUNT_LOOP,
				Value:    loopVar.Value,
				Children: []*ASTNode{loopVarNode, zeroNode, body},
				Line:     startLine,
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

	// Consume closing pipe
	if p.current().Type == TOKEN_PIPE {
		p.advance()
	}

	p.inFunctionCall--
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

	// Consume closing pipe
	if p.current().Type == TOKEN_PIPE {
		p.advance()
	}

	p.inFunctionCall--
	return call
}

func (p *Parser) parseLogStatement() *ASTNode {
	p.expect(TOKEN_LOG)

	// log takes two arguments: message and file_path
	p.expect(TOKEN_PIPE)

	call := &ASTNode{
		Type:  NODE_CALL,
		Value: "log",
		Line:  p.current().Line,
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

	// Consume closing pipe
	if p.current().Type == TOKEN_PIPE {
		p.advance()
	}

	p.inFunctionCall--
	return call
}

func (p *Parser) parsePanicStatement() *ASTNode {
	p.expect(TOKEN_PANIC)

	// panic is similar to print - takes any number of arguments
	p.expect(TOKEN_PIPE)

	call := &ASTNode{
		Type:  NODE_CALL,
		Value: "panic",
		Line:  p.current().Line,
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

	// Consume closing pipe
	if p.current().Type == TOKEN_PIPE {
		p.advance()
	}

	p.inFunctionCall--
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
		// Check for returning the current function (recursion via return)
		for _, returnVal := range returnValues {
			if returnVal.Type == NODE_IDENTIFIER && returnVal.Value == p.currentFunctionName {
				p.Errors = append(p.Errors, ParseError{
					Message: fmt.Sprintf("recursion not allowed: cannot return function '%s' itself; use a loop till condition instead", p.currentFunctionName),
					Line:    line,
					Column:  returnVal.Column,
				})
			}
		}

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
				// Use smart split that handles nested commas in dict<k,v>
				expectedTypes = splitReturnTypes(expectedRet)
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

func (p *Parser) parseGotoStatement() *ASTNode {
	gotoToken := p.expect(TOKEN_GOTO)
	line := gotoToken.Line

	// Expect a label name (identifier)
	if p.current().Type != TOKEN_IDENTIFIER {
		p.Errors = append(p.Errors, ParseError{
			Message: "expected label name after 'goto'",
			Line:    line,
			Column:  p.current().Column,
		})
		return &ASTNode{Type: NODE_GOTO_STATEMENT, Line: line}
	}

	labelName := p.current().Value
	p.advance()

	return &ASTNode{
		Type:  NODE_GOTO_STATEMENT,
		Value: labelName,
		Line:  line,
	}
}

func (p *Parser) parseLabelDeclaration() *ASTNode {
	// Label syntax: my_label: (current token is identifier)
	labelName := p.current().Value
	line := p.current().Line
	p.advance()

	// Expect a colon
	if p.current().Type != TOKEN_ASSIGN || p.current().Value != ":" {
		p.Errors = append(p.Errors, ParseError{
			Message: "expected ':' after label name",
			Line:    line,
			Column:  p.current().Column,
		})
		return &ASTNode{Type: NODE_LABEL_DECLARATION, Value: labelName, Line: line}
	}
	p.advance()

	// Skip newline/indent after colon
	for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT {
		p.advance()
	}

	// Parse the block body
	p.blockDepth++
	block := &ASTNode{Type: NODE_BLOCK, Line: line}
	for p.current().Type != TOKEN_END && p.current().Type != TOKEN_EOF {
		if p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_SEMICOLON ||
			p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
			p.advance()
			continue
		}
		stmt := p.parseStatement()
		if stmt != nil {
			block.Children = append(block.Children, stmt)
		}
	}

	if p.current().Type == TOKEN_END {
		p.advance() // Consume $
		p.blockDepth--
	}

	return &ASTNode{
		Type:     NODE_LABEL_DECLARATION,
		Value:    labelName,
		Line:     line,
		Children: []*ASTNode{block},
	}
}

func (p *Parser) parseDeferStatement() *ASTNode {
	deferToken := p.expect(TOKEN_DEFER)

	// Parse the deferred statement
	// Could be a function call, ahoy statement, print statement, etc.
	var statement *ASTNode
	if p.current().Type == TOKEN_PRINT {
		statement = p.parsePrintStatement()
	} else if p.current().Type == TOKEN_LOG {
		statement = p.parseLogStatement()
	} else if p.current().Type == TOKEN_PANIC {
		statement = p.parsePanicStatement()
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

// parseCHeadersParallel scans for C header imports and parses them concurrently
func (p *Parser) parseCHeadersParallel() {
	// Save current position
	savedPos := p.pos

	// Collect all C header imports
	var headers []struct{ Path, Namespace string }
	headerLines := make(map[string]int) // path -> line number for error reporting

	for p.current().Type != TOKEN_EOF {
		if p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_SEMICOLON || p.current().Type == TOKEN_DEDENT || p.current().Type == TOKEN_INDENT {
			p.advance()
			continue
		}

		// Stop scanning when we hit non-import statements (except program declaration)
		if p.current().Type != TOKEN_IMPORT && p.current().Type != TOKEN_PROGRAM {
			break
		}

		// Skip program declaration
		if p.current().Type == TOKEN_PROGRAM {
			// Advance past program declaration
			for p.current().Type != TOKEN_NEWLINE && p.current().Type != TOKEN_EOF {
				p.advance()
			}
			continue
		}

		// Parse import to get the path
		if p.current().Type == TOKEN_IMPORT {
			importLine := p.current().Line
			p.advance() // skip 'import'

			var namespace, path string
			if p.current().Type == TOKEN_IDENTIFIER {
				namespace = p.current().Value
				p.advance()
				if p.current().Type == TOKEN_STRING {
					path = p.current().Value
					p.advance()
				}
			} else if p.current().Type == TOKEN_STRING {
				path = p.current().Value
				p.advance()
			}

			// Only collect .h files
			if strings.HasSuffix(path, ".h") {
				// Resolve relative paths
				resolvedPath := path
				if !filepath.IsAbs(path) && p.sourceFilePath != "" {
					sourceDir := filepath.Dir(p.sourceFilePath)
					resolvedPath = filepath.Join(sourceDir, path)
					resolvedPath = filepath.Clean(resolvedPath)
				}

				// Check if file exists
				if _, err := os.Stat(resolvedPath); err == nil {
					headers = append(headers, struct{ Path, Namespace string }{resolvedPath, namespace})
					headerLines[resolvedPath] = importLine
				}
			}
		}
	}

	// Restore position
	p.pos = savedPos

	// If we have headers, parse them concurrently
	if len(headers) > 0 {
		results := ParseCHeadersConcurrently(headers)

		// Process results
		for _, result := range results {
			if result.Err != nil {
				if p.LintMode {
					errMsg := fmt.Sprintf("Failed to parse C header '%s': %v", result.Path, result.Err)
					p.recordErrorAtLine(errMsg, headerLines[result.Path])
				}
				continue
			}

			// Mark as pre-parsed so parseImportStatement skips re-parsing
			if p.preParsedHeaders == nil {
				p.preParsedHeaders = make(map[string]*CHeaderInfo)
			}
			p.preParsedHeaders[result.Path] = result.Info
		}
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

	// Resolve relative paths
	resolvedPath := path
	if !filepath.IsAbs(path) && p.sourceFilePath != "" {
		// Path is relative, resolve it relative to the source file
		sourceDir := filepath.Dir(p.sourceFilePath)
		resolvedPath = filepath.Join(sourceDir, path)
		resolvedPath = filepath.Clean(resolvedPath)
	}

	// Check if file exists (for linting)
	if p.LintMode {
		if _, err := os.Stat(resolvedPath); os.IsNotExist(err) {
			errMsg := fmt.Sprintf("Import path does not exist: %s", path)
			p.recordErrorAtLine(errMsg, importToken.Line)
			// Continue parsing, but don't try to load the header
			return &ASTNode{
				Type:     NODE_IMPORT_STATEMENT,
				Value:    path,
				DataType: namespace,
			}
		}
	}

	// Parse the C header file if it ends with .h
	if strings.HasSuffix(path, ".h") {
		// Check if header was pre-parsed (parallel parsing)
		var headerInfo *CHeaderInfo
		var err error
		if p.preParsedHeaders != nil {
			if preParsed, ok := p.preParsedHeaders[resolvedPath]; ok {
				headerInfo = preParsed
			}
		}
		// If not pre-parsed, parse now
		if headerInfo == nil {
			headerInfo, err = ParseCHeader(resolvedPath)
		}
		if err != nil {
			// Record error in lint mode
			if p.LintMode {
				errMsg := fmt.Sprintf("Failed to parse C header '%s': %v", path, err)
				p.recordErrorAtLine(errMsg, importToken.Line)
			}
		} else if headerInfo != nil {
			if namespace != "" {
				// Store with namespace
				p.cHeaders[namespace] = headerInfo
				// Register zero-arg functions from namespaced header
				for cFuncName, cFunc := range headerInfo.Functions {
					if len(cFunc.Parameters) == 0 {
						snakeName := PascalToSnake(cFuncName)
						p.zeroArgFunctions[namespace+"."+snakeName] = true
					}
				}
			} else {
				// Merge into global
				for name, fn := range headerInfo.Functions {
					p.cHeaderGlobal.Functions[name] = fn
					// Register zero-arg functions for O(1) lookup
					if len(fn.Parameters) == 0 {
						snakeName := PascalToSnake(name)
						p.zeroArgFunctions[snakeName] = true
					}
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
		Line:     importToken.Line,
	}
}

func (p *Parser) parseProgramDeclaration() *ASTNode {
	p.expect(TOKEN_PROGRAM)
	name := p.expect(TOKEN_IDENTIFIER)

	// Track that we have a program declaration
	p.hasProgramDecl = true

	// Don't skip newlines here - let parseProgram handle them

	return &ASTNode{
		Type:  NODE_PROGRAM_DECLARATION,
		Value: name.Value,
		Line:  name.Line,
	}
}

func (p *Parser) parseWalrusAssignment() *ASTNode {
	// Handle name := value (inferred type assignment)
	name := p.expect(TOKEN_IDENTIFIER)
	line := name.Line
	p.expect(TOKEN_WALRUS)

	// ERROR: Walrus with single variable is not allowed (should use : or :type=)
	if p.LintMode {
		errMsg := "Can't use walrus ':=' without a type (for single var); use ':type=' for explicit type or ':' to declare or '=' to reassign"
		p.recordErrorAtLine(errMsg, line)
	}

	// Check if value is a switch expression - not allowed without explicit type
	if p.current().Type == TOKEN_SWITCH {
		errMsg := fmt.Sprintf("Expected type/s between : and = for switch expression (e.g., :string= or :(string,int)=)")
		if p.LintMode {
			p.recordErrorAtLine(errMsg, line)
			// Skip the switch statement to continue parsing
			for p.current().Type != TOKEN_END && p.current().Type != TOKEN_EOF {
				p.advance()
			}
			if p.current().Type == TOKEN_END {
				p.advance()
			}
			return &ASTNode{
				Type:  NODE_VARIABLE_DECLARATION,
				Value: name.Value,
				Line:  line,
			}
		} else {
			panic(errMsg)
		}
	}

	// Parse the value
	value := p.parseExpression()

	// Record the declaration in every mode; see nameIsKnownVariable.
	p.recordKnownVar(name.Value)

	// Track the variable declaration (except for _ placeholder)
	if p.LintMode && name.Value != "_" {
		if existingLine, exists := p.declaredVars[name.Value]; exists {
			errMsg := fmt.Sprintf("Variable '%s' already declared on line %d; use '=' to update variable", name.Value, existingLine)
			p.recordErrorAtLine(errMsg, line)
		} else {
			p.declaredVars[name.Value] = line
		}

		// Infer and track the variable's type
		inferredType := p.inferType(value)
		if inferredType != "unknown" {
			p.variableTypes[name.Value] = inferredType
		}
	}

	return &ASTNode{
		Type:     NODE_VARIABLE_DECLARATION,
		Value:    name.Value,
		DataType: "", // Empty means inferred
		Children: []*ASTNode{value},
		Line:     line,
	}
}
