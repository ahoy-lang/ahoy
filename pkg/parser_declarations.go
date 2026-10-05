package ahoy

import (
	"fmt"
	"strings"
)

// Declarations: functions, structs, enums, constants, aliases and member access.

func (p *Parser) parseEnumDeclaration() *ASTNode {
	startLine := p.current().Line
	// Expect 'enum' keyword
	p.expect(TOKEN_ENUM)

	// Check for enum type specifier: enum:type
	var enumType string
	if p.current().Type == TOKEN_ASSIGN {
		p.advance() // consume ':'
		// Parse the type
		if p.current().Type == TOKEN_INT_TYPE {
			enumType = "int"
			p.advance()
		} else if p.current().Type == TOKEN_STRING_TYPE {
			enumType = "string"
			p.advance()
		} else if p.current().Type == TOKEN_FLOAT_TYPE {
			enumType = "float"
			p.advance()
		} else if p.current().Type == TOKEN_BOOL_TYPE {
			enumType = "bool"
			p.advance()
		} else if p.current().Type == TOKEN_ARRAY_TYPE {
			enumType = "array"
			p.advance()
		} else if p.current().Type == TOKEN_DICT_TYPE {
			enumType = "dict"
			p.advance()
		} else if p.current().Type == TOKEN_IDENTIFIER {
			// Custom type
			enumType = p.current().Value
			p.advance()
		} else {
			errMsg := fmt.Sprintf("Expected type after 'enum:' at line %d", p.current().Line)
			if p.LintMode {
				p.recordError(errMsg)
				enumType = "int" // default
			} else {
				panic(errMsg)
			}
		}
	} else {
		// No type specified - leave empty for auto-detection
		enumType = ""
	}

	// Get the enum name
	var name Token
	if p.current().Type == TOKEN_IDENTIFIER {
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

	// Expect ':' after enum name (optional for simple cases)
	if p.current().Type == TOKEN_ASSIGN {
		p.advance() // consume ':'
	}

	// Check if this is a one-line enum (no newline after name/colon)
	isOneLine := p.current().Type != TOKEN_NEWLINE && p.current().Type != TOKEN_INDENT

	if !isOneLine {
		p.skipNewlines()
		if p.current().Type == TOKEN_INDENT {
			p.advance()
		}
	}

	enum := &ASTNode{
		Type:     NODE_ENUM_DECLARATION,
		Value:    name.Value,
		Line:     name.Line,
		EnumType: enumType,
	}

	// Parse enum members based on type
	for p.current().Type != TOKEN_END && p.current().Type != TOKEN_DEDENT && p.current().Type != TOKEN_EOF {
		// Skip any leading newlines
		p.skipNewlines()

		// Check if we've reached the end
		if p.current().Type == TOKEN_END || p.current().Type == TOKEN_DEDENT || p.current().Type == TOKEN_EOF {
			break
		}

		var valueNode *ASTNode
		var memberName string
		var isMutable bool

		// Parse value expression first (if present)
		// Value can be: number, string, bool, array, dict, color, vector2
		if p.current().Type == TOKEN_NUMBER {
			valueNode = &ASTNode{
				Type:  NODE_NUMBER,
				Value: p.current().Value,
				Line:  p.current().Line,
			}
			p.advance()
		} else if p.current().Type == TOKEN_STRING {
			valueNode = &ASTNode{
				Type:  NODE_STRING,
				Value: p.current().Value,
				Line:  p.current().Line,
			}
			p.advance()
		} else if p.current().Type == TOKEN_TRUE || p.current().Type == TOKEN_FALSE {
			valueNode = &ASTNode{
				Type:  NODE_BOOLEAN,
				Value: p.current().Value,
				Line:  p.current().Line,
			}
			p.advance()
		} else if p.current().Type == TOKEN_LBRACKET {
			// Array literal
			valueNode = p.parseArrayLiteral()
		} else if p.current().Type == TOKEN_LBRACE {
			// Dict literal
			valueNode = p.parseDictLiteral()
		} else if p.current().Type == TOKEN_IDENTIFIER && (p.current().Value == "color" || p.current().Value == "vector2") {
			// Parse color<r,g,b,a> or vector2<x,y>
			typeName := p.current().Value
			typeToken := p.current()
			p.advance() // consume type name

			if p.current().Type == TOKEN_LANGLE {
				p.advance() // consume '<'
				values := []*ASTNode{}

				// Parse comma-separated values
				for p.current().Type != TOKEN_RANGLE && p.current().Type != TOKEN_EOF && p.current().Type != TOKEN_END {
					if p.current().Type == TOKEN_NUMBER {
						values = append(values, &ASTNode{
							Type:  NODE_NUMBER,
							Value: p.current().Value,
							Line:  p.current().Line,
						})
						p.advance()
					} else if p.current().Type == TOKEN_COMMA {
						p.advance() // skip comma
					} else {
						// Unexpected token
						p.advance()
					}
				}

				if p.current().Type == TOKEN_RANGLE {
					p.advance() // consume '>'
				}

				valueNode = &ASTNode{
					Type:     NODE_OBJECT_LITERAL,
					Value:    typeName,
					Children: values,
					Line:     typeToken.Line,
				}
			}
		}
		// If no value was parsed, it will be auto-assigned (for int enums) or use default

		// Now parse the member name (required)
		if p.current().Type != TOKEN_IDENTIFIER {
			// No identifier found, might be end of enum
			break
		}
		memberName = p.current().Value
		p.advance()

		// Check for :mutable modifier
		if p.current().Type == TOKEN_ASSIGN {
			p.advance() // consume ':'
			if p.current().Type == TOKEN_IDENTIFIER && p.current().Value == "mutable" {
				isMutable = true
				p.advance()
			} else {
				errMsg := fmt.Sprintf("Expected 'mutable' after ':' in enum member at line %d", p.current().Line)
				if p.LintMode {
					p.recordError(errMsg)
				}
			}
		}

		member := &ASTNode{
			Type:      NODE_IDENTIFIER,
			Value:     memberName,
			IsMutable: isMutable,
			Line:      p.current().Line,
			Children:  []*ASTNode{},
		}
		if valueNode != nil {
			member.Children = append(member.Children, valueNode)
		}
		enum.Children = append(enum.Children, member)

		// Skip optional delimiters (comma, semicolon, or newline)
		for p.current().Type == TOKEN_COMMA || p.current().Type == TOKEN_SEMICOLON || p.current().Type == TOKEN_NEWLINE {
			p.advance()
		}

		// Check for $ terminator
		if p.current().Type == TOKEN_END {
			break
		}

		// For one-line enums, stop at newline or EOF
		if isOneLine && (p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_EOF) {
			break
		}
	}

	if !isOneLine && p.current().Type == TOKEN_DEDENT {
		p.advance()
	}

	// Consume 'end' keyword ($) - now optional for one-line enums without explicit $
	if p.current().Type == TOKEN_END {
		p.advance()
	} else if !isOneLine {
		errMsg := fmt.Sprintf("Expected '$' to close enum at line %d", startLine)
		if p.LintMode {
			p.recordErrorAtLine(errMsg, startLine)
		} else {
			panic(errMsg)
		}
	}

	// Register enum definition in lint mode for validation
	if p.LintMode {
		members := make([]*ASTNode, len(enum.Children))
		copy(members, enum.Children)
		p.enums[name.Value] = &EnumDefinition{
			Name:    name.Value,
			Members: members,
			Line:    startLine,
		}
	}

	return enum
}

// Parse constant declaration (NAME :: value)
func (p *Parser) parseConstantDeclaration() *ASTNode {
	name := p.expect(TOKEN_IDENTIFIER)
	line := name.Line
	varName := name.Value
	p.expect(TOKEN_DOUBLE_COLON)

	// Check if this is a function declaration (has |) - this should use @ prefix
	if p.current().Type == TOKEN_PIPE {
		errMsg := fmt.Sprintf("Function declarations must use '@' prefix: @ %s :: |...", varName)
		if p.LintMode {
			p.recordErrorAtLine(errMsg, line)
			// Still parse it to continue checking for other errors
			return p.parseFunctionWithDoubleColon(name)
		} else {
			panic(errMsg)
		}
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
			// Track if constant is declared in main function
			if p.currentFunctionName == "main" {
				p.constantsInMain[varName] = true

				// Check if this constant was used earlier in main
				if usageLines, wasUsed := p.constantUsages[varName]; wasUsed {
					for _, usageLine := range usageLines {
						if usageLine < line {
							errMsg := fmt.Sprintf("use of const '%s' before its declared on line %d", varName, line)
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
			// Resolve type aliases
			resolvedType := p.resolveTypeAlias(explicitType)
			p.variableTypes[varName] = resolvedType
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

// Parse function declaration with @ prefix: @ name :: |params| type: body
func (p *Parser) parseFunctionDeclaration() *ASTNode {
	startLine := p.current().Line
	p.expect(TOKEN_AT) // consume @

	// Check for nested function definition
	if p.LintMode && p.functionDepth > 0 {
		errMsg := fmt.Sprintf("Function definitions cannot be nested inside other functions (line %d)", startLine)
		p.recordErrorAtLine(errMsg, startLine)
	}

	name := p.expect(TOKEN_IDENTIFIER)

	// Double colon is now optional
	if p.current().Type == TOKEN_DOUBLE_COLON {
		p.advance()
	}

	p.functionDepth++                    // Entering function definition
	defer func() { p.functionDepth-- }() // Exiting function definition

	result := p.parseFunctionWithDoubleColon(name)
	return result
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
	hasDefaultParam := false                // Track if we've seen a default parameter
	localParamNames := make(map[string]int) // Track params in THIS function only (name -> line)

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

			// Type is optional - if not present, treat as any
			if p.isTypeToken(p.current().Type) {
				// Parse complex types like array[int] or dict<string,int>
				paramType = p.parseComplexReturnType()
			} else {
				// No type specified - any (generic) parameter
				paramType = "any"
			}
		} else {
			// No colon, no type - any (generic) parameter
			paramType = "any"
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
			Line:         paramName.Line,
			Column:       paramName.Column,
		}
		params.Children = append(params.Children, param)

		// In lint mode, check for duplicate parameter names within THIS function
		if p.LintMode {
			// Check for duplicate in this function's parameter list
			if _, exists := localParamNames[paramName.Value]; exists {
				errMsg := fmt.Sprintf("parameter name '%s' declared twice; rename one of them", paramName.Value)
				p.recordErrorAtLine(errMsg, paramName.Line)
			} else {
				localParamNames[paramName.Value] = paramName.Line
			}

			// Also register in functionScope for type tracking
			if p.functionScope == nil {
				p.functionScope = make(map[string]string)
			}
			p.functionScope[paramName.Value] = paramType
		}

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
		} else if p.current().Type != TOKEN_PIPE && p.current().Type != TOKEN_EOF {
			// If we're not at a comma, pipe, or EOF, something is wrong - break to avoid infinite loop
			break
		}
	}
	p.expect(TOKEN_PIPE)

	// Return type (optional, can be multiple types separated by comma)
	var returnType string
	var returnTypesArray []string
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

			// Parse first return type (including complex types like array[int], dict<string,int>)
			if p.isTypeToken(p.current().Type) {
				returnTypes = append(returnTypes, p.parseComplexReturnType())

				// Parse additional return types (multiple returns)
				for p.current().Type == TOKEN_COMMA {
					p.advance()
					if p.isTypeToken(p.current().Type) {
						returnTypes = append(returnTypes, p.parseComplexReturnType())
					} else {
						break
					}
				}
			}

			// Join multiple return types with comma
			if len(returnTypes) > 0 {
				returnType = strings.Join(returnTypes, ",")
				returnTypesArray = returnTypes
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
	var savedInFunctionBody bool
	var savedFunctionName string
	var savedDeclaredVars map[string]int
	if p.LintMode {
		savedFunctionRet = p.currentFunctionRet
		savedFunctionScope = p.functionScope
		savedInFunctionBody = p.inFunctionBody
		savedFunctionName = p.currentFunctionName
		// Save and clear declaredVars for function scope
		savedDeclaredVars = p.declaredVars
		p.declaredVars = make(map[string]int)
		p.currentFunctionRet = returnType
		p.currentFunctionName = name.Value // Track current function name for recursion detection
		p.inFunctionBody = true            // Mark that we're inside function body
		// Create a COMPLETELY NEW function scope (don't copy from old one)
		// Parameters have already been added to functionScope before we get here,
		// so we need to extract just the current function's parameters and create a new scope
		currentParams := make(map[string]string)
		if params != nil {
			for _, paramNode := range params.Children {
				if paramNode != nil {
					currentParams[paramNode.Value] = paramNode.DataType
				}
			}
		}
		// Create new scope with only current function's parameters
		p.functionScope = currentParams
	}

	// Parse body (function with :: syntax always uses '$')
	p.blockDepth++ // Opening a multi-line block
	body := p.parseBlockUntilEnd("function", startLine)

	// parseBlockUntilEnd already consumes the '$' token and decrements blockDepth

	// In lint mode, restore previous function context
	if p.LintMode {
		p.currentFunctionRet = savedFunctionRet
		p.functionScope = savedFunctionScope
		p.inFunctionBody = savedInFunctionBody
		p.currentFunctionName = savedFunctionName
		// Restore declaredVars
		p.declaredVars = savedDeclaredVars

		// Register function signature for later validation
		paramInfos := []ParameterInfo{}
		if params != nil {
			for _, paramNode := range params.Children {
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
			// Use the parsed array instead of splitting the string to preserve complex types
			returnTypesList = returnTypesArray
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

		// Validate main function signature if program is declared
		if p.hasProgramDecl && name.Value == "main" {
			if len(paramInfos) > 0 {
				p.recordErrorAtLine("main function cannot have parameters when program is declared", name.Line)
			}
			if returnType != "void" && returnType != "" {
				p.recordErrorAtLine("main function must return void when program is declared", name.Line)
			}
		}
	}

	// Register zero-arg functions for O(1) lookup (must be outside LintMode block)
	// Count params from the parsed params node
	paramCount := 0
	if params != nil {
		paramCount = len(params.Children)
	}
	if paramCount == 0 {
		p.zeroArgFunctions[name.Value] = true
	}

	fn.Children = append(fn.Children, params)
	fn.Children = append(fn.Children, body)
	fn.DataType = returnType

	// Clear function scope after parsing function to prevent parameter names
	// from leaking into global scope
	if p.LintMode {
		p.functionScope = nil
	}

	return fn
}

// Parse tuple assignment (a, b : c, d) or tuple declaration (a, b = c, d)
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

		// Check for optional type annotation (identifier:type, ...)
		// Only consume colon if followed by a type keyword AND a comma
		// This prevents consuming the final colon before values
		if p.current().Type == TOKEN_ASSIGN {
			nextToken := p.peek(1)
			isTypeToken := nextToken.Type == TOKEN_INT_TYPE || nextToken.Type == TOKEN_FLOAT_TYPE ||
				nextToken.Type == TOKEN_STRING_TYPE || nextToken.Type == TOKEN_BOOL_TYPE ||
				nextToken.Type == TOKEN_DICT_TYPE || nextToken.Type == TOKEN_ARRAY_TYPE

			// Check if this is inline type annotation (has comma after type)
			isInlineTypeAnnotation := isTypeToken && p.peek(2).Type == TOKEN_COMMA

			if isInlineTypeAnnotation {
				p.advance() // consume :
				idNode.DataType = p.current().Value
				p.advance() // consume type
			}
			// Otherwise don't consume the colon - it's the main assignment colon
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

	// Determine assignment type
	assignOp := p.current().Type
	isWalrus := (assignOp == TOKEN_WALRUS)

	// Check for typed tuple declaration: :(type, type)=
	if assignOp == TOKEN_ASSIGN && p.peek(1).Type == TOKEN_LPAREN {
		// This is :(type1, type2)= syntax - parse types
		p.advance() // consume :
		p.advance() // consume (

		typeIndex := 0
		for p.current().Type != TOKEN_RPAREN && p.current().Type != TOKEN_EOF {
			// Parse type
			if p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
				p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_BOOL_TYPE ||
				p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
				p.current().Type == TOKEN_IDENTIFIER {

				if typeIndex < len(leftSide.Children) {
					leftSide.Children[typeIndex].DataType = p.current().Value
				}
				typeIndex++
				p.advance()
			}

			if p.current().Type == TOKEN_COMMA {
				p.advance()
			}
		}
		p.expect(TOKEN_RPAREN)

		// Now expect =
		if p.current().Type == TOKEN_EQUALS {
			p.advance()
		} else {
			p.recordError(fmt.Sprintf("Expected '=' after type specification at line %d", p.current().Line))
		}

		// In lint mode, track new variable declarations
		if p.LintMode {
			for _, idNode := range leftSide.Children {
				varName := idNode.Value
				// Skip _ placeholder - it's not a real variable
				if varName == "_" {
					continue
				}
				if existingLine, exists := p.declaredVars[varName]; exists {
					errMsg := fmt.Sprintf("Variable '%s' already declared on line %d; use '=' to update variable", varName, existingLine)
					p.recordErrorAtLine(errMsg, line)
				} else {
					p.declaredVars[varName] = line
				}
			}
		}
	} else if assignOp == TOKEN_ASSIGN || assignOp == TOKEN_EQUALS || assignOp == TOKEN_WALRUS {
		p.advance() // consume :, =, or :=

		// Check for typed tuple after = or := (shouldn't happen but handle for completeness)
		if p.current().Type == TOKEN_LPAREN {
			// This is :(type1, type2)= syntax - parse types
			p.advance() // consume (
			typeIndex := 0
			for p.current().Type != TOKEN_RPAREN && p.current().Type != TOKEN_EOF {
				// Parse type
				if p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
					p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_BOOL_TYPE ||
					p.current().Type == TOKEN_DICT_TYPE || p.current().Type == TOKEN_ARRAY_TYPE ||
					p.current().Type == TOKEN_IDENTIFIER {

					if typeIndex < len(leftSide.Children) {
						leftSide.Children[typeIndex].DataType = p.current().Value
					}
					typeIndex++
					p.advance()
				}

				if p.current().Type == TOKEN_COMMA {
					p.advance()
				}
			}
			p.expect(TOKEN_RPAREN)

			// Now expect =
			if p.current().Type == TOKEN_EQUALS {
				p.advance()
			}
		} else if isWalrus {
			// := for tuples: smart declare-or-reassign
			// For each variable, check if it exists, and declare if not
			// Store existing types for validation later
			if p.LintMode {
				for _, idNode := range leftSide.Children {
					varName := idNode.Value
					// Skip _ placeholder
					if varName == "_" {
						continue
					}
					exists := false
					var existingType string

					// Check function scope
					if p.functionScope != nil {
						if vType, ok := p.functionScope[varName]; ok {
							exists = true
							existingType = vType
						}
					}
					// Check global scope
					if !exists {
						if vType, ok := p.variableTypes[varName]; ok {
							exists = true
							existingType = vType
						}
					}
					// Check declaredVars
					if !exists {
						if _, ok := p.declaredVars[varName]; ok {
							exists = true
							// Try to get type
							if vType, ok := p.variableTypes[varName]; ok {
								existingType = vType
							} else if p.functionScope != nil {
								if vType, ok := p.functionScope[varName]; ok {
									existingType = vType
								}
							}
						}
					}

					// If doesn't exist, declare it
					if !exists {
						p.declaredVars[varName] = line
					}
					// Store existing type for validation (if reassigning)
					if exists && existingType != "" {
						idNode.DataType = existingType
					}
				}
			}
		} else if assignOp == TOKEN_ASSIGN {
			// : for tuples: pure declaration
			// In lint mode, track new variable declarations
			if p.LintMode {
				for _, idNode := range leftSide.Children {
					varName := idNode.Value
					// Skip _ placeholder
					if varName == "_" {
						continue
					}
					if existingLine, exists := p.declaredVars[varName]; exists {
						errMsg := fmt.Sprintf("Variable '%s' already declared on line %d; use '=' to update variable", varName, existingLine)
						p.recordErrorAtLine(errMsg, line)
					} else {
						p.declaredVars[varName] = line
					}
				}
			}
		} else {
			// = for tuples: pure reassignment
			// In lint mode, check that all variables exist
			if p.LintMode {
				for _, idNode := range leftSide.Children {
					varName := idNode.Value
					// Skip _ placeholder - it's always allowed
					if varName == "_" {
						continue
					}
					exists := false

					// Check function scope
					if p.functionScope != nil {
						if _, ok := p.functionScope[varName]; ok {
							exists = true
						}
					}
					// Check global scope
					if !exists {
						if _, ok := p.variableTypes[varName]; ok {
							exists = true
						}
					}
					// Check declaredVars
					if !exists {
						if _, ok := p.declaredVars[varName]; ok {
							exists = true
						}
					}

					if !exists {
						errMsg := "Can't assign to undeclared variable; Use ':' for declaration, or Walrus ':=' for smart declare-or-reassign of tuple values."
						p.recordErrorAtLine(errMsg, line)
					}
				}
			}
		}
	} else {
		p.recordError(fmt.Sprintf("Expected '=', ':=', or ':' in tuple assignment at line %d", line))
		return nil
	}

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
		// Check if any left-side variable lacks explicit type when right side is switch
		if len(rightSide.Children) == 1 && rightSide.Children[0].Type == NODE_SWITCH_STATEMENT {
			hasAllTypes := true
			for _, target := range leftSide.Children {
				if target.DataType == "" {
					hasAllTypes = false
					break
				}
			}
			if !hasAllTypes {
				errMsg := fmt.Sprintf("Tuple assignment from switch requires explicit types: :(type, type)= at line %d", line)
				p.recordError(errMsg)
			}
		}

		p.validateTupleAssignment(leftSide, rightSide, line)
	}

	return &ASTNode{
		Type:     NODE_TUPLE_ASSIGNMENT,
		Line:     line,
		Children: []*ASTNode{leftSide, rightSide},
	}
}

// Parse alias declaration: alias name: type
func (p *Parser) parseAliasDeclaration() *ASTNode {
	startLine := p.current().Line
	p.expect(TOKEN_ALIAS)

	name := p.expect(TOKEN_IDENTIFIER)
	p.expect(TOKEN_ASSIGN) // :

	// Parse the type being aliased
	var aliasedType string
	if p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
		p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_BOOL_TYPE ||
		p.current().Type == TOKEN_CHAR_TYPE || p.current().Type == TOKEN_DICT_TYPE ||
		p.current().Type == TOKEN_ARRAY_TYPE || p.current().Type == TOKEN_IDENTIFIER {
		aliasedType = p.current().Value
		p.advance()

		// Handle array[type] syntax - supports nested arrays like array[array[int]]
		if p.current().Type == TOKEN_LBRACKET {
			p.advance()
			// Check for nested array
			if p.current().Type == TOKEN_ARRAY_TYPE && p.peek(1).Type == TOKEN_LBRACKET {
				// Nested array - recursively build type string
				innerType := p.current().Value
				p.advance()
				p.advance() // consume [
				if p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
					p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_BOOL_TYPE ||
					p.current().Type == TOKEN_IDENTIFIER {
					innerType = innerType + "[" + p.current().Value + "]"
					p.advance()
				}
				p.expect(TOKEN_RBRACKET) // inner ]
				aliasedType = aliasedType + "[" + innerType + "]"
			} else if p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
				p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_BOOL_TYPE ||
				p.current().Type == TOKEN_IDENTIFIER {
				aliasedType = aliasedType + "[" + p.current().Value + "]"
				p.advance()
			}
			p.expect(TOKEN_RBRACKET)
		}
	} else {
		errMsg := fmt.Sprintf("Expected type after 'alias %s:' at line %d", name.Value, startLine)
		if p.LintMode {
			p.recordError(errMsg)
			aliasedType = "int" // default
		} else {
			panic(errMsg)
		}
	}

	// Register the alias in the type system
	if p.typeAliases == nil {
		p.typeAliases = make(map[string]string)
	}
	p.typeAliases[name.Value] = aliasedType

	return &ASTNode{
		Type:     NODE_ALIAS_DECLARATION,
		Value:    name.Value,
		DataType: aliasedType,
		Line:     startLine,
	}
}

// Parse union declaration: union name: type1, type2, ... $
func (p *Parser) parseUnionDeclaration() *ASTNode {
	startLine := p.current().Line
	p.expect(TOKEN_UNION)

	name := p.expect(TOKEN_IDENTIFIER)
	p.expect(TOKEN_ASSIGN) // :

	// Parse the types in the union
	types := []string{}
	for {
		if p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
			p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_BOOL_TYPE ||
			p.current().Type == TOKEN_CHAR_TYPE || p.current().Type == TOKEN_DICT_TYPE ||
			p.current().Type == TOKEN_ARRAY_TYPE || p.current().Type == TOKEN_IDENTIFIER {
			types = append(types, p.current().Value)
			p.advance()

			if p.current().Type == TOKEN_COMMA {
				p.advance()
			} else {
				break
			}
		} else {
			break
		}
	}

	if len(types) < 2 {
		errMsg := fmt.Sprintf("Union type '%s' must have at least 2 types at line %d", name.Value, startLine)
		if p.LintMode {
			p.recordError(errMsg)
		} else {
			panic(errMsg)
		}
	}

	// Register the union in the type system
	if p.unionTypes == nil {
		p.unionTypes = make(map[string][]string)
	}
	p.unionTypes[name.Value] = types

	unionNode := &ASTNode{
		Type:  NODE_UNION_DECLARATION,
		Value: name.Value,
		Line:  startLine,
	}

	// Add type nodes as children
	for _, typeName := range types {
		unionNode.Children = append(unionNode.Children, &ASTNode{
			Type:  NODE_TYPE,
			Value: typeName,
		})
	}

	return unionNode
}

// Parse struct declaration
func (p *Parser) parseJsonStructDeclaration() *ASTNode {
	// Parse struct:json name:
	firstToken := p.current()

	if firstToken.Type == TOKEN_STRUCT {
		// struct:json syntax
		p.expect(TOKEN_STRUCT)     // "struct"
		p.expect(TOKEN_ASSIGN)     // ":"
		p.expect(TOKEN_IDENTIFIER) // "json"

		// Now parse the struct name and body
		var name Token
		if p.current().Type == TOKEN_IDENTIFIER {
			name = p.current()
			p.advance()
		} else {
			name = p.expect(TOKEN_IDENTIFIER)
		}
		p.expect(TOKEN_ASSIGN)

		struc := &ASTNode{
			Type:     NODE_STRUCT_DECLARATION,
			Value:    name.Value,
			Line:     name.Line,
			DataType: "json", // Mark this as a JSON struct
		}

		// Parse struct fields (same as regular struct)
		p.skipNewlines()
		if p.current().Type == TOKEN_INDENT {
			p.advance()
		}

		// Parse fields with field field:type syntax
		for p.current().Type == TOKEN_IDENTIFIER || p.current().Type == TOKEN_NUMBER {
			fieldName := p.current()
			p.advance()

			// Expect the field name again (JSON mapping)
			if p.current().Type == TOKEN_IDENTIFIER && p.current().Value == fieldName.Value {
				p.advance() // Consume duplicate name
			}

			// Expect : and type
			p.expect(TOKEN_ASSIGN)
			fieldType := p.current().Value
			p.advance()

			field := &ASTNode{
				Type:     NODE_IDENTIFIER,
				Value:    fieldName.Value,
				DataType: fieldType,
				Line:     fieldName.Line,
			}
			struc.Children = append(struc.Children, field)

			// Skip optional delimiters
			for p.current().Type == TOKEN_COMMA || p.current().Type == TOKEN_SEMICOLON || p.current().Type == TOKEN_NEWLINE {
				p.advance()
			}

			if p.current().Type == TOKEN_DEDENT {
				p.advance()
				break
			}
		}

		// Consume $ if present
		if p.current().Type == TOKEN_END {
			p.advance()
		}

		return struc
	}

	return nil
}

func (p *Parser) parseStructDeclaration() *ASTNode {
	startLine := p.current().Line
	p.expect(TOKEN_STRUCT)

	var name Token
	if p.current().Type == TOKEN_IDENTIFIER {
		name = p.current()
		p.advance()
	} else {
		name = p.expect(TOKEN_IDENTIFIER)
	}
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
		p.current().Type == TOKEN_HASH ||
		p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
		p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_CHAR_TYPE ||
		p.current().Type == TOKEN_BOOL_TYPE || p.current().Type == TOKEN_DICT_TYPE ||
		p.current().Type == TOKEN_ARRAY_TYPE {
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
				for p.current().Type != TOKEN_EOF {
					if p.current().Type == TOKEN_NEWLINE {
						p.advance()
						continue
					}

					// If we encounter a DEDENT, check if there are more fields after it
					if p.current().Type == TOKEN_DEDENT {
						p.advance()
						// If next token is not a field starter, we're done with this type
						if p.current().Type != TOKEN_IDENTIFIER &&
							p.current().Type != TOKEN_INT_TYPE && p.current().Type != TOKEN_FLOAT_TYPE &&
							p.current().Type != TOKEN_STRING_TYPE && p.current().Type != TOKEN_CHAR_TYPE &&
							p.current().Type != TOKEN_BOOL_TYPE && p.current().Type != TOKEN_DICT_TYPE &&
							p.current().Type != TOKEN_ARRAY_TYPE {
							break
						}
						continue
					}

					// If we encounter another 'type' keyword, we're done with this nested type
					if p.current().Type == TOKEN_TYPE {
						break
					}

					if p.current().Type == TOKEN_IDENTIFIER ||
						p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
						p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_CHAR_TYPE ||
						p.current().Type == TOKEN_BOOL_TYPE || p.current().Type == TOKEN_DICT_TYPE ||
						p.current().Type == TOKEN_ARRAY_TYPE {

						// Get field name (could be identifier or type keyword used as name)
						var fieldName Token
						if p.current().Type == TOKEN_IDENTIFIER ||
							p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
							p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_CHAR_TYPE ||
							p.current().Type == TOKEN_BOOL_TYPE || p.current().Type == TOKEN_DICT_TYPE ||
							p.current().Type == TOKEN_ARRAY_TYPE {
							fieldName = p.current()
							p.advance()
						} else {
							fieldName = p.expect(TOKEN_IDENTIFIER)
						}

						// Check if type is provided (field: type) or use default type
						fieldType := ""
						var defaultValue *ASTNode

						if p.current().Type == TOKEN_ASSIGN {
							p.advance() // consume :

							// Get type - handle array[type] and dict<key,val> syntax
							if p.current().Type == TOKEN_ARRAY_TYPE {
								fieldType = "array"
								p.advance()
								if p.current().Type == TOKEN_LBRACKET {
									p.advance() // consume [
									fieldType += "[" + p.current().Value
									p.advance() // consume type
									p.expect(TOKEN_RBRACKET)
									fieldType += "]"
								}
							} else if p.current().Type == TOKEN_DICT_TYPE {
								fieldType = "dict"
								p.advance()
								if p.current().Type == TOKEN_LANGLE {
									p.advance() // consume <
									fieldType += "<" + p.current().Value
									p.advance() // consume key type
									if p.current().Type == TOKEN_COMMA {
										p.advance()
										fieldType += "," + p.current().Value
										p.advance() // consume value type
									}
									p.expect(TOKEN_RANGLE)
									fieldType += ">"
								}
							} else {
								fieldType = p.current().Value
								if p.current().Type == TOKEN_IDENTIFIER ||
									p.current().Type == TOKEN_INT_TYPE ||
									p.current().Type == TOKEN_FLOAT_TYPE ||
									p.current().Type == TOKEN_STRING_TYPE ||
									p.current().Type == TOKEN_BOOL_TYPE {
									p.advance()
								}
							}

							// Check for = and default value
							if p.current().Type == TOKEN_EQUALS {
								p.advance() // consume =

								// Parse default value
								if p.current().Type == TOKEN_NUMBER {
									defaultValue = &ASTNode{
										Type:  NODE_NUMBER,
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
								} else if p.current().Type == TOKEN_MINUS && p.peek(1).Type == TOKEN_NUMBER {
									line := p.current().Line
									p.advance() // consume minus
									defaultValue = &ASTNode{
										Type:  NODE_NUMBER,
										Value: "-" + p.current().Value,
										Line:  line,
									}
									p.advance()
								} else if p.current().Type == TOKEN_IDENTIFIER && p.peek(1).Type == TOKEN_LBRACE {
									typeName := p.current().Value
									p.advance() // consume type name
									p.advance() // consume {
									defaultValue = p.parseObjectLiteral()
									defaultValue.Value = typeName
								} else if p.current().Type == TOKEN_LBRACE {
									p.advance() // consume {
									defaultValue = p.parseObjectLiteral()
									defaultValue.Value = fieldType
								} else if p.current().Type == TOKEN_LANGLE {
									defaultValue = p.parseDictLiteral()
								} else if p.current().Type == TOKEN_STRING {
									defaultValue = &ASTNode{
										Type:  NODE_STRING,
										Value: p.current().Value,
										Line:  p.current().Line,
									}
									p.advance()
								} else if p.current().Type == TOKEN_LBRACKET {
									defaultValue = p.parseArrayLiteralBracket()
								}
							}
						} else {
							// No type specified - default to int
							fieldType = "int"
						}

						field := &ASTNode{
							Type:         NODE_IDENTIFIER,
							Value:        fieldName.Value,
							DataType:     fieldType,
							Line:         fieldName.Line,
							DefaultValue: defaultValue,
						}
						nestedType.Children = append(nestedType.Children, field)

						// Skip optional delimiters (comma, semicolon, or newline)
						for p.current().Type == TOKEN_COMMA || p.current().Type == TOKEN_SEMICOLON || p.current().Type == TOKEN_NEWLINE {
							p.advance()
						}
					} else {
						// Unknown token, skip to avoid infinite loop
						p.advance()
					}
				}
			}

			struc.Children = append(struc.Children, nestedType)
		} else {
			// Regular field - check for optional # prefix (static property) before field name
			isStatic := false

			// Check for # prefix (static property) before field name
			if p.current().Type == TOKEN_HASH {
				isStatic = true
				p.advance() // consume #
			}

			// Get field name (could be identifier or type keyword used as name)
			var fieldName Token
			if p.current().Type == TOKEN_IDENTIFIER ||
				p.current().Type == TOKEN_INT_TYPE || p.current().Type == TOKEN_FLOAT_TYPE ||
				p.current().Type == TOKEN_STRING_TYPE || p.current().Type == TOKEN_CHAR_TYPE ||
				p.current().Type == TOKEN_BOOL_TYPE || p.current().Type == TOKEN_DICT_TYPE ||
				p.current().Type == TOKEN_ARRAY_TYPE {
				fieldName = p.current()
				p.advance()
			} else {
				fieldName = p.expect(TOKEN_IDENTIFIER)
			}

			// Check if type is provided (field: type) or use default type
			fieldType := ""
			var defaultValue *ASTNode

			if p.current().Type == TOKEN_ASSIGN {
				p.advance() // consume :

				// Get type - handle array[type] and dict<key,val> syntax
				if p.current().Type == TOKEN_ARRAY_TYPE {
					fieldType = "array"
					p.advance()
					if p.current().Type == TOKEN_LBRACKET {
						p.advance() // consume [
						fieldType += "[" + p.current().Value
						p.advance() // consume type
						p.expect(TOKEN_RBRACKET)
						fieldType += "]"
					}
				} else if p.current().Type == TOKEN_DICT_TYPE {
					fieldType = "dict"
					p.advance()
					if p.current().Type == TOKEN_LANGLE {
						p.advance() // consume <
						fieldType += "<" + p.current().Value
						p.advance() // consume key type
						if p.current().Type == TOKEN_COMMA {
							p.advance()
							fieldType += "," + p.current().Value
							p.advance() // consume value type
						}
						p.expect(TOKEN_RANGLE)
						fieldType += ">"
					}
				} else {
					fieldType = p.current().Value
					if p.current().Type == TOKEN_IDENTIFIER ||
						p.current().Type == TOKEN_INT_TYPE ||
						p.current().Type == TOKEN_FLOAT_TYPE ||
						p.current().Type == TOKEN_STRING_TYPE ||
						p.current().Type == TOKEN_BOOL_TYPE {
						p.advance()
					}
				}

				// Check for = and default value
				if p.current().Type == TOKEN_EQUALS {
					p.advance() // consume =

					// Parse default value
					if p.current().Type == TOKEN_NUMBER {
						defaultValue = &ASTNode{
							Type:  NODE_NUMBER,
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
					} else if p.current().Type == TOKEN_MINUS && p.peek(1).Type == TOKEN_NUMBER {
						line := p.current().Line
						p.advance() // consume minus
						defaultValue = &ASTNode{
							Type:  NODE_NUMBER,
							Value: "-" + p.current().Value,
							Line:  line,
						}
						p.advance()
					} else if p.current().Type == TOKEN_IDENTIFIER && p.peek(1).Type == TOKEN_LBRACE {
						typeName := p.current().Value
						p.advance() // consume type name
						p.advance() // consume {
						defaultValue = p.parseObjectLiteral()
						defaultValue.Value = typeName
					} else if p.current().Type == TOKEN_LBRACE {
						p.advance() // consume {
						defaultValue = p.parseObjectLiteral()
						defaultValue.Value = fieldType
					} else if p.current().Type == TOKEN_LANGLE {
						defaultValue = p.parseDictLiteral()
					} else if p.current().Type == TOKEN_STRING {
						defaultValue = &ASTNode{
							Type:  NODE_STRING,
							Value: p.current().Value,
							Line:  p.current().Line,
						}
						p.advance()
					} else if p.current().Type == TOKEN_LBRACKET {
						defaultValue = p.parseArrayLiteralBracket()
					}
				}
			} else {
				// No type specified - default to int
				fieldType = "int"
			}

			// Check if field name is SCREAMING_SNAKE_CASE (const property)
			isConst := isScreamingSnakeCase(fieldName.Value)

			field := &ASTNode{
				Type:         NODE_IDENTIFIER,
				Value:        fieldName.Value,
				DataType:     fieldType,
				Line:         fieldName.Line,
				DefaultValue: defaultValue,
				IsStatic:     isStatic,
				IsConst:      isConst,
			}
			struc.Children = append(struc.Children, field)
		}

		// Skip optional delimiters (comma, semicolon, or newline)
		for p.current().Type == TOKEN_COMMA || p.current().Type == TOKEN_SEMICOLON || p.current().Type == TOKEN_NEWLINE {
			p.advance()
		}

		// Check for $ terminator (allows inline $ for structs)
		if p.current().Type == TOKEN_END {
			break
		}

		// For one-line structs, stop at newline or EOF
		if isOneLine && (p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_EOF) {
			break
		}

		if !isOneLine {
			p.skipNewlines()
		}
	}

	// Consume all dedents (may be multiple from nested types)
	if !isOneLine {
		for p.current().Type == TOKEN_DEDENT {
			p.advance()
		}
	}

	// Consume 'end' keyword ($) - now optional for one-line structs without explicit $
	if p.current().Type == TOKEN_END {
		p.advance()
	} else if !isOneLine {
		errMsg := fmt.Sprintf("Expected '$' to close struct at line %d", startLine)
		if p.LintMode {
			p.recordErrorAtLine(errMsg, startLine)
		} else {
			panic(errMsg)
		}
	}

	// Store struct definition for type checking (needed for nested type instantiation like Card.Assassin{})
	p.storeStructDefinition(struc, startLine)

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
			fullNestedName := structName + "." + nestedName // Full name like Card.Assassin
			nestedDef := &StructDefinition{
				Name:   fullNestedName,
				Parent: structName, // Track parent struct
				Fields: []StructField{},
				Line:   child.Line,
			}

			// Build a set of field names defined in the nested type (for override detection)
			nestedFieldNames := make(map[string]bool)
			for _, field := range child.Children {
				nestedFieldNames[field.Value] = true
			}

			// Add parent struct fields to nested type (skip those overridden)
			for _, field := range structDef.Fields {
				if nestedFieldNames[field.Name] {
					continue // Skip overridden fields
				}
				nestedDef.Fields = append(nestedDef.Fields, field)
			}

			// Add nested type fields
			for _, field := range child.Children {
				nestedDef.Fields = append(nestedDef.Fields, StructField{
					Name:         field.Value,
					Type:         field.DataType,
					DefaultValue: field.DefaultValue,
					IsStatic:     field.IsStatic,
					IsConst:      field.IsConst,
				})
			}

			// Register nested type with full name (Card.Assassin)
			p.structs[fullNestedName] = nestedDef
			// Also register with short name for backward compatibility
			p.structs[nestedName] = nestedDef
		} else {
			// Regular field
			structDef.Fields = append(structDef.Fields, StructField{
				Name:         child.Value,
				Type:         child.DataType,
				DefaultValue: child.DefaultValue,
				IsStatic:     child.IsStatic,
				IsConst:      child.IsConst,
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
	// Handle static member access assignments (StructType.#field)
	if target.Type == NODE_STATIC_MEMBER_ACCESS {
		// Check if the left side is a struct type name (valid) or an instance variable (invalid)
		if len(target.Children) > 0 && target.Children[0].Type == NODE_IDENTIFIER {
			leftSideName := target.Children[0].Value
			staticFieldName := target.Value

			// Check if left side is a known struct type
			_, isStructType := p.structs[leftSideName]

			// Check if left side is a variable (instance)
			_, isVariable := p.variableTypes[leftSideName]

			if isVariable && !isStructType {
				// It's an instance variable, not allowed with # syntax
				// Get the struct type to provide a helpful error message
				varType := p.variableTypes[leftSideName]
				structType := strings.TrimPrefix(varType, "struct:")
				errMsg := fmt.Sprintf("Cannot use '#' syntax on instance variable '%s' - use %s.#%s : value syntax instead (line %d)",
					leftSideName, structType, staticFieldName, line)
				p.recordError(errMsg)
				return
			}
		}
		// Static member access on struct type is valid - no error needed
		return
	}

	if target.Type != NODE_MEMBER_ACCESS || len(target.Children) == 0 {
		return
	}

	// For nested access like obj.position.x, we need to walk the chain
	// and validate each level
	current := target
	var accessChain []string

	// Build the access chain from right to left
	for current.Type == NODE_MEMBER_ACCESS {
		accessChain = append([]string{current.Value}, accessChain...)
		if len(current.Children) > 0 {
			current = current.Children[0]
		} else {
			break
		}
	}

	// Get the root variable
	varName := ""
	objectType := ""
	if current.Type == NODE_IDENTIFIER {
		varName = current.Value
		if vtype, ok := p.variableTypes[varName]; ok {
			objectType = vtype
		}
	}

	if objectType == "" {
		return
	}

	// Normalize struct type
	objectType = strings.TrimPrefix(objectType, "struct:")

	// Walk through each property in the chain
	for i, propName := range accessChain {
		isLastProp := i == len(accessChain)-1

		// Check if it's an object literal
		if objectType == "object" || objectType == "object_literal" {
			if props, ok := p.objectLiterals[varName]; ok {
				if !props[propName] {
					if isLastProp {
						errMsg := fmt.Sprintf("object literal can't have new properties added at runtime (line %d)", line)
						p.recordError(errMsg)
					}
					return
				}
			}
			// For object literals, we can't check nested types
			return
		}

		// Check if it's a struct type
		if structDef, ok := p.structs[objectType]; ok {
			if !p.structHasField(objectType, propName) {
				errMsg := fmt.Sprintf("Property not found: '%s' does not exist on type '%s' (line %d)",
					propName, structDef.Name, line)
				p.recordError(errMsg)
				return
			}

			// Get the type of this property for next iteration
			propType := p.getStructFieldType(objectType, propName)

			if isLastProp {
				// Check if this property is static
				field := p.getStructField(objectType, propName)
				if field != nil && field.IsStatic {
					errMsg := fmt.Sprintf("Cannot assign to static property '%s' via instance - use %s.#%s : value syntax instead (line %d)",
						propName, objectType, propName, line)
					p.recordError(errMsg)
					return
				}

				// Check if this property is const (SCREAMING_SNAKE_CASE)
				if field != nil && field.IsConst {
					errMsg := fmt.Sprintf("Cannot assign to const property '%s' - SCREAMING_SNAKE_CASE properties are immutable (line %d)",
						propName, line)
					p.recordError(errMsg)
					return
				}

				// Validate type compatibility for the final assignment
				valueType := p.inferType(value)
				if !p.checkTypeCompatibility(propType, valueType) {
					errMsg := fmt.Sprintf("Type mismatch: %s:%s cannot be assigned %s value (line %d)",
						propName, propType, valueType, line)
					p.recordError(errMsg)
				}
			} else {
				// Move to the next level in the chain
				objectType = propType
			}
		} else {
			// Unknown type, can't validate further
			return
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

// Get the full field info from struct
func (p *Parser) getStructField(typeName, fieldName string) *StructField {
	fields := p.getStructFields(typeName)
	for i := range fields {
		if fields[i].Name == fieldName {
			return &fields[i]
		}
	}
	return nil
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

// Parse member access chain (obj.prop or obj.method|| or StructType.#static_field)
