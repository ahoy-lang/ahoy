package ahoy

import (
	"fmt"
	"strings"
)

// Type inference and validation helpers used across the parser.

func (p *Parser) parseMemberAccessChain(object *ASTNode) *ASTNode {
	for p.current().Type == TOKEN_DOT {
		p.advance()

		// Check for static member access: .#field_name
		if p.current().Type == TOKEN_HASH {
			p.advance() // consume #
			member := p.expect(TOKEN_IDENTIFIER)

			// This is a static member access
			object = &ASTNode{
				Type:     NODE_STATIC_MEMBER_ACCESS,
				Value:    member.Value,
				Line:     member.Line,
				Children: []*ASTNode{object},
				IsStatic: true,
			}
			continue
		}

		// Allow 'type' keyword as a member name (special case for .type property)
		var member Token
		if p.current().Type == TOKEN_TYPE {
			// Only Value and Line of this synthetic token are used.
			member = Token{
				Value: "type",
				Line:  p.current().Line,
			}
			p.advance()
		} else {
			member = p.expect(TOKEN_IDENTIFIER)
		}

		// Check if this is the special .type property first (before method call check)
		if member.Value == "type" {
			// This is a .type property access, not a method call
			object = &ASTNode{
				Type:     NODE_TYPE_PROPERTY,
				Value:    "type",
				Line:     member.Line,
				Children: []*ASTNode{object},
			}
			continue // Don't process as method call
		}

		// Check if this is a method call
		// Allow method calls even inside function calls, but need to look ahead
		// to ensure we have a matching closing pipe
		if p.current().Type == TOKEN_PIPE {
			// Look ahead to see if there's a closing pipe (for method call)
			// or if this pipe is the closing pipe of the outer call
			savedPos := p.pos
			p.advance() // consume opening pipe

			// If immediately followed by another pipe, it's an empty method call
			isMethodCall := false
			if p.current().Type == TOKEN_PIPE {
				isMethodCall = true
			} else if p.current().Type != TOKEN_COMMA && p.current().Type != TOKEN_NEWLINE && p.current().Type != TOKEN_EOF {
				// There's content between pipes, so it's a method call
				isMethodCall = true
			}

			// Reset position
			p.pos = savedPos

			if isMethodCall {
				p.advance() // consume opening pipe

				// Increment depth for nested call handling
				p.inFunctionCall++

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
							arg := p.parseCallArgument()
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

				// Decrement depth
				p.inFunctionCall--

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
		} else {
			// Check if this is the special .type property
			if member.Value == "type" {
				object = &ASTNode{
					Type:     NODE_TYPE_PROPERTY,
					Value:    "type",
					Line:     member.Line,
					Children: []*ASTNode{object},
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
	}

	// Check if this is a nested struct type instantiation (e.g., Card.Assassin{})
	if object.Type == NODE_MEMBER_ACCESS && p.current().Type == TOKEN_LBRACE {
		// Build the full type name from the member access chain
		fullTypeName := p.buildMemberAccessTypeName(object)

		// Check if this looks like a nested type instantiation pattern (Parent.Child{})
		// We accept it if:
		// 1. The full type name is known, OR
		// 2. The parent struct name is known (for multi-file packages), OR
		// 3. It matches the pattern of Identifier.Identifier{ (for multi-file packages where
		//    the parent struct is defined in another file)
		parentName := ""
		isNestedPattern := false
		if len(object.Children) > 0 && object.Children[0].Type == NODE_IDENTIFIER {
			parentName = object.Children[0].Value
			// Check if the pattern is Identifier.Identifier (not a.b.c or expression.field)
			// The child should be an identifier, and object.Value should be the nested type name
			if object.Value != "" {
				isNestedPattern = true
			}
		}

		_, fullTypeExists := p.structs[fullTypeName]
		_, parentExists := p.structs[parentName]

		if fullTypeExists || parentExists || isNestedPattern {
			p.advance() // consume {

			// Skip any newlines/indents
			for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT {
				p.advance()
			}

			// Check for empty instantiation
			if p.current().Type == TOKEN_RBRACE {
				p.advance() // consume }
				obj := &ASTNode{
					Type:     NODE_OBJECT_LITERAL,
					DataType: "object",
					Value:    fullTypeName, // Set the full type name
					Children: []*ASTNode{},
					Line:     object.Line,
				}
				return obj
			}

			// Check if this is object instantiation with properties
			if (p.current().Type == TOKEN_IDENTIFIER || p.current().Type == TOKEN_STRING) &&
				p.peek(1).Type == TOKEN_ASSIGN {
				obj := p.parseObjectLiteral()
				obj.Value = fullTypeName
				return obj
			}

			// Handle other cases - for now just parse as object literal
			obj := p.parseObjectLiteral()
			obj.Value = fullTypeName
			return obj
		}
	}

	return object
}

// inferMemberAccessType infers the type of a member access chain
func (p *Parser) inferMemberAccessType(node *ASTNode) string {
	if node.Type == NODE_IDENTIFIER {
		if vtype, ok := p.variableTypes[node.Value]; ok {
			return vtype
		}
		return ""
	}

	if node.Type == NODE_MEMBER_ACCESS && len(node.Children) > 0 {
		// Get the type of the object being accessed
		objType := p.inferMemberAccessType(node.Children[0])
		if objType == "" {
			return ""
		}

		// Normalize struct type
		objType = strings.TrimPrefix(objType, "struct:")

		// Look up the field type in the struct
		if structDef, ok := p.structs[objType]; ok {
			memberName := node.Value
			for _, field := range structDef.Fields {
				if field.Name == memberName {
					return field.Type
				}
			}
		}

		// Check for built-in types with fields (vector2, color)
		if objType == "vector2" {
			if node.Value == "x" || node.Value == "y" {
				return "float"
			}
		} else if objType == "color" {
			if node.Value == "r" || node.Value == "g" || node.Value == "b" || node.Value == "a" {
				return "int"
			}
		}
	}

	return ""
}

// buildMemberAccessTypeName builds a full type name from a member access chain
// e.g., for Card.Assassin it returns "Card.Assassin"
func (p *Parser) buildMemberAccessTypeName(node *ASTNode) string {
	if node.Type == NODE_IDENTIFIER {
		return node.Value
	}

	if node.Type == NODE_MEMBER_ACCESS && len(node.Children) > 0 {
		parentName := p.buildMemberAccessTypeName(node.Children[0])
		return parentName + "." + node.Value
	}

	return ""
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
		// For chained access like obj.field1.field2, resolve the intermediate type
		objectType = p.inferMemberAccessType(object)
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

	// Check for built-in types with fields (vector2, color)
	if objectType == "vector2" {
		if memberName != "x" && memberName != "y" {
			errMsg := fmt.Sprintf("Property not found: '%s' does not exist on type 'vector2' (line %d)",
				memberName, line)
			p.recordError(errMsg)
		}
		return
	} else if objectType == "color" {
		if memberName != "r" && memberName != "g" && memberName != "b" && memberName != "a" {
			errMsg := fmt.Sprintf("Property not found: '%s' does not exist on type 'color' (line %d)",
				memberName, line)
			p.recordError(errMsg)
		}
		return
	}

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

	// Skip leading newlines/indents for multiline arrays
	for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT {
		p.advance()
	}

	for p.current().Type != TOKEN_RBRACKET {
		// Skip newlines/indents before element
		for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
			p.advance()
		}

		// Check if we've reached the closing bracket after whitespace
		if p.current().Type == TOKEN_RBRACKET {
			break
		}

		element := p.parseExpression()
		array.Children = append(array.Children, element)

		// Skip trailing whitespace after element
		for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
			p.advance()
		}

		if p.current().Type == TOKEN_COMMA {
			p.advance()
			// Skip whitespace after comma
			for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
				p.advance()
			}
		} else if p.current().Type == TOKEN_SEMICOLON {
			// Also allow semicolon as delimiter in arrays
			p.advance()
			// Skip whitespace after semicolon
			for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_INDENT || p.current().Type == TOKEN_DEDENT {
				p.advance()
			}
		} else if p.current().Type != TOKEN_RBRACKET {
			break
		}
	}

	p.inArrayLiteral = false
	p.expect(TOKEN_RBRACKET)

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

// Check if the current position is a lambda expression (param: expr)
func (p *Parser) isLambda() bool {
	// Look ahead for pattern: IDENTIFIER ASSIGN expression or (params): expression
	saved := p.pos

	// Check for multi-param lambda: (param1, param2): expression
	if p.current().Type == TOKEN_LPAREN {
		p.advance()
		// Look for identifiers and commas until )
		for p.current().Type != TOKEN_RPAREN && p.current().Type != TOKEN_EOF {
			if p.current().Type == TOKEN_IDENTIFIER || p.current().Type == TOKEN_COMMA {
				p.advance()
			} else {
				p.pos = saved
				return false
			}
		}
		if p.current().Type == TOKEN_RPAREN {
			p.advance()
			isLambdaSyntax := p.current().Type == TOKEN_ASSIGN
			p.pos = saved
			return isLambdaSyntax
		}
		p.pos = saved
		return false
	}

	// Check for single-param lambda: param: expression
	if p.pos+1 < len(p.tokens) {
		p.advance()
		isLambdaSyntax := p.current().Type == TOKEN_ASSIGN
		p.pos = saved
		return isLambdaSyntax
	}

	p.pos = saved
	return false
}

// Parse lambda expression: param: expression or (param1, param2): expression
func (p *Parser) parseLambda() *ASTNode {
	startLine := p.current().Line
	params := []*ASTNode{}

	// Check for multi-parameter lambda with parentheses
	if p.current().Type == TOKEN_LPAREN {
		p.advance() // consume (

		// Parse parameters
		for p.current().Type != TOKEN_RPAREN && p.current().Type != TOKEN_EOF {
			param := p.expect(TOKEN_IDENTIFIER)
			params = append(params, &ASTNode{
				Type:  NODE_IDENTIFIER,
				Value: param.Value,
				Line:  param.Line,
			})

			if p.current().Type == TOKEN_COMMA {
				p.advance()
			} else if p.current().Type != TOKEN_RPAREN {
				break
			}
		}

		p.expect(TOKEN_RPAREN)
	} else {
		// Single parameter (no parentheses)
		param := p.expect(TOKEN_IDENTIFIER)
		params = append(params, &ASTNode{
			Type:  NODE_IDENTIFIER,
			Value: param.Value,
			Line:  param.Line,
		})
	}

	// Expect colon
	p.expect(TOKEN_ASSIGN)

	// Parse expression until we hit PIPE
	expr := p.parseLambdaBody()

	// Create lambda node with parameters as children followed by body
	lambda := &ASTNode{
		Type:  NODE_LAMBDA,
		Value: fmt.Sprintf("%d", len(params)), // Store param count in Value
		Line:  startLine,
	}

	// Add parameters and body as children
	lambda.Children = append(lambda.Children, params...)
	lambda.Children = append(lambda.Children, expr)

	return lambda
}

// Parse lambda body - stops at PIPE
func (p *Parser) parseLambdaBody() *ASTNode {
	// Increment depth to allow nested function calls
	p.inFunctionCall++

	expr := p.parseOrExpression()

	p.inFunctionCall--
	return expr
}

// lookupVariableType looks up the type of a variable in the current scope
func (p *Parser) lookupVariableType(varName string) string {
	// Check function scope first if we're in a function
	if p.inFunctionBody && p.functionScope != nil {
		if varType, ok := p.functionScope[varName]; ok {
			return varType
		}
	}
	// Check global scope
	if varType, ok := p.variableTypes[varName]; ok {
		return varType
	}
	return ""
}

// nameIsKnownVariable reports whether a name currently resolves to a variable
// rather than to a function or a type. It is what disambiguates a zero-argument
// call from a variable reference inside an argument list, where both have the
// same token shape.
func (p *Parser) nameIsKnownVariable(name string) bool {
	if p.knownVars[name] {
		return true
	}
	for i := len(p.loopVarScopes) - 1; i >= 0; i-- {
		if _, ok := p.loopVarScopes[i][name]; ok {
			return true
		}
	}
	if p.functionScope != nil {
		if _, ok := p.functionScope[name]; ok {
			return true
		}
	}
	for i := len(p.functionScopeStack) - 1; i >= 0; i-- {
		if _, ok := p.functionScopeStack[i][name]; ok {
			return true
		}
	}
	if _, ok := p.variableTypes[name]; ok {
		return true
	}
	return false
}

// recordKnownVar notes that a name is a declared variable. Unlike declaredVars
// and functionScope, which are only maintained while linting, this is recorded
// in every mode: code generation also needs to tell a variable reference from a
// zero-argument call while it builds the AST.
func (p *Parser) recordKnownVar(name string) {
	if name == "" || name == "_" {
		return
	}
	if p.knownVars == nil {
		p.knownVars = make(map[string]bool)
	}
	p.knownVars[name] = true
}

// resolveTypeAlias resolves a type alias to its underlying type
func (p *Parser) resolveTypeAlias(typeName string) string {
	// Recursively resolve aliases
	if aliasedType, exists := p.typeAliases[typeName]; exists {
		return p.resolveTypeAlias(aliasedType)
	}
	return typeName
}

// validateTupleAssignment validates that tuple assignment types match
func (p *Parser) validateTupleAssignment(leftSide, rightSide *ASTNode, line int) {
	if leftSide == nil || rightSide == nil {
		return
	}

	// Check if this is a simple tuple swap (a, b : b, a)
	// where right side contains only identifiers
	if len(leftSide.Children) == len(rightSide.Children) {
		isSimpleSwap := true
		for _, rightExpr := range rightSide.Children {
			if rightExpr.Type != NODE_IDENTIFIER {
				isSimpleSwap = false
				break
			}
		}

		if isSimpleSwap {
			// Validate that types match for swap
			for i, leftVar := range leftSide.Children {
				if i >= len(rightSide.Children) {
					break
				}
				rightVar := rightSide.Children[i]

				// Get or infer types
				leftType := leftVar.DataType
				if leftType == "" {
					leftType = p.lookupVariableType(leftVar.Value)
				}

				rightType := rightVar.DataType
				if rightType == "" {
					rightType = p.lookupVariableType(rightVar.Value)
				}

				// Both must have known types for validation
				if leftType != "" && rightType != "" {
					if !p.checkTypeCompatibility(leftType, rightType) {
						errMsg := fmt.Sprintf("Tuple swap type mismatch at position %d: '%s' is type %s but '%s' is type %s",
							i+1, leftVar.Value, leftType, rightVar.Value, rightType)
						p.recordError(errMsg)
					}
				}

				// Register/update variable type with inferred type from right side
				varType := leftType
				if varType == "" {
					varType = rightType
				}
				if varType != "" {
					targetScope := p.variableTypes
					if p.inFunctionBody && p.functionScope != nil {
						targetScope = p.functionScope
					}
					targetScope[leftVar.Value] = varType
				}
			}
			return
		}
	}

	// Check if right side is a single switch statement returning tuples
	if len(rightSide.Children) == 1 && rightSide.Children[0].Type == NODE_SWITCH_STATEMENT {
		switchNode := rightSide.Children[0]
		expectedCount := len(leftSide.Children)

		// Validate all cases return the expected number of values
		for i := 1; i < len(switchNode.Children); i++ {
			caseNode := switchNode.Children[i]
			if caseNode.Type == NODE_SWITCH_CASE && len(caseNode.Children) > 1 {
				caseLine := caseNode.Line
				if caseLine == 0 {
					caseLine = line
				}

				caseBody := caseNode.Children[1]

				// Determine actual count: BLOCK means tuple, anything else is single value
				actualCount := 1
				if caseBody.Type == NODE_BLOCK {
					actualCount = len(caseBody.Children)
				}

				if actualCount != expectedCount {
					errMsg := fmt.Sprintf("Expected %d return values but got %d",
						expectedCount, actualCount)
					p.recordErrorAtLine(errMsg, caseLine)
					continue // Skip type validation if count mismatch
				}

				// Validate types if explicit types are provided
				if caseBody.Type == NODE_BLOCK {
					for j, expr := range caseBody.Children {
						if j < len(leftSide.Children) && leftSide.Children[j].DataType != "" {
							expectedType := leftSide.Children[j].DataType
							actualType := p.inferType(expr)
							if !p.checkTypeCompatibility(expectedType, actualType) {
								errMsg := fmt.Sprintf("Tuple position %d expects type %s but got %s",
									j+1, expectedType, actualType)
								p.recordErrorAtLine(errMsg, caseLine)
							}
						}
					}
				} else if expectedCount > 1 {
					// Single expression but multiple expected - handle type check for position 1
					if leftSide.Children[0].DataType != "" {
						expectedType := leftSide.Children[0].DataType
						actualType := p.inferType(caseBody)
						if !p.checkTypeCompatibility(expectedType, actualType) {
							errMsg := fmt.Sprintf("Tuple position 1 expects type %s but got %s (missing %d values)",
								expectedType, actualType, expectedCount-1)
							p.recordErrorAtLine(errMsg, caseLine)
						}
					}
				}
			}
		}

		// Register variables with their declared types
		for _, leftVar := range leftSide.Children {
			if leftVar.DataType != "" {
				targetScope := p.variableTypes
				if p.inFunctionBody && p.functionScope != nil {
					targetScope = p.functionScope
				}
				targetScope[leftVar.Value] = leftVar.DataType
			}
		}
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
				if p.inFunctionBody && p.functionScope != nil {
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
		expectedCount := len(leftSide.Children)
		actualCount := len(returnTypes)

		// If function signature is found but returnTypes is empty, determine actual count
		if actualCount == 0 && funcSig != nil && !funcSig.IsInfer {
			// Check the function's DataType to see if it's void or single return
			// If the function node exists, check its DataType
			if funcSig.FunctionNode != nil {
				funcReturnType := funcSig.FunctionNode.DataType
				if funcReturnType == "void" || funcReturnType == "" {
					// Void function - returns 0 values
					actualCount = 0
				} else {
					// Single return function
					actualCount = 1
				}
			} else {
				// Fallback: assume single return if not void
				actualCount = 1
			}
		}

		if expectedCount != actualCount {
			var errMsg string
			if actualCount == 0 {
				errMsg = fmt.Sprintf("returning more return values than expected; expected void got %d return value(s)", expectedCount)
			} else {
				errMsg = fmt.Sprintf("expected %d return value(s) got %d value(s)", actualCount, expectedCount)
			}
			p.recordError(errMsg)
			return
		}

		// Validate and register each variable
		for i, leftVar := range leftSide.Children {
			if i >= len(returnTypes) {
				break
			}

			// Skip _ placeholder
			if leftVar.Value == "_" {
				continue
			}

			expectedType := leftVar.DataType
			actualType := returnTypes[i]

			// If left side has type annotation (from existing variable or explicit type), validate it matches
			if expectedType != "" && actualType != "" {
				if !p.checkTypeCompatibility(expectedType, actualType) {
					errMsg := fmt.Sprintf("Tuple assignment type mismatch at position %d: variable '%s' declared as %s cannot be reassigned as %s",
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
			if p.inFunctionBody && p.functionScope != nil {
				targetScope = p.functionScope
			}
			targetScope[leftVar.Value] = varType
		}
	}
}

// substituteGenericTypes substitutes generic/any parameter types with actual argument types
func (p *Parser) substituteGenericTypes(funcSig *FunctionSignature, argTypes []string) []string {
	// Create a map of parameter name to actual type
	genericSubstitutions := make(map[string]string)

	for i, param := range funcSig.Parameters {
		if i < len(argTypes) {
			if param.Type == "generic" || param.Type == "any" || param.Type == "" {
				genericSubstitutions[param.Name] = argTypes[i]
			}
		}
	}

	// Substitute in return types
	result := make([]string, len(funcSig.ReturnTypes))
	for i, retType := range funcSig.ReturnTypes {
		// Check if this return type is a parameter name (generic/any)
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

	// Create substitutions for generic/any parameters
	genericSubstitutions := make(map[string]string)
	for i, param := range funcSig.Parameters {
		if i < len(argTypes) {
			if param.Type == "generic" || param.Type == "any" || param.Type == "" {
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

// splitReturnTypes splits a comma-separated list of return types, handling nested commas in dict<k,v>
func splitReturnTypes(typeStr string) []string {
	if typeStr == "" {
		return []string{}
	}

	var types []string
	var current strings.Builder
	depth := 0 // Track nesting level in <> or []

	for i := 0; i < len(typeStr); i++ {
		ch := typeStr[i]
		switch ch {
		case '<', '[':
			depth++
			current.WriteByte(ch)
		case '>', ']':
			depth--
			current.WriteByte(ch)
		case ',':
			if depth == 0 {
				// Top-level comma, split here
				types = append(types, strings.TrimSpace(current.String()))
				current.Reset()
			} else {
				// Nested comma, keep it
				current.WriteByte(ch)
			}
		default:
			current.WriteByte(ch)
		}
	}

	// Add the last type
	if current.Len() > 0 {
		types = append(types, strings.TrimSpace(current.String()))
	}

	return types
}

// isTypeToken checks if the given token type represents a type
func (p *Parser) isTypeToken(tokenType TokenType) bool {
	return tokenType == TOKEN_INT_TYPE || tokenType == TOKEN_FLOAT_TYPE ||
		tokenType == TOKEN_STRING_TYPE || tokenType == TOKEN_BOOL_TYPE ||
		tokenType == TOKEN_DICT_TYPE || tokenType == TOKEN_ARRAY_TYPE ||
		tokenType == TOKEN_ANY_TYPE || tokenType == TOKEN_IDENTIFIER
}

// parseComplexReturnType parses a return type that may include complex types like array[int] or dict<string,int>
func (p *Parser) parseComplexReturnType() string {
	baseType := p.current().Value
	p.advance()

	// Map 'any' to internal 'any' type (generic)
	if baseType == "any" {
		return "any"
	}

	// Check for array[type] syntax - supports nested arrays like array[array[int]]
	if baseType == "array" && p.current().Type == TOKEN_LBRACKET {
		p.advance() // consume [
		// Check if element type is also an array (nested)
		if p.current().Value == "array" && p.peek(1).Type == TOKEN_LBRACKET {
			// Recursively parse nested array type
			elementType := p.parseComplexReturnType()
			p.expect(TOKEN_RBRACKET)
			return fmt.Sprintf("array[%s]", elementType)
		}
		elementType := p.current().Value
		p.advance() // consume type
		p.expect(TOKEN_RBRACKET)
		return fmt.Sprintf("array[%s]", elementType)
	}

	// Check for dict<key,value> or dict[key,value] syntax
	if baseType == "dict" && (p.current().Type == TOKEN_LANGLE || p.current().Type == TOKEN_LBRACKET) {
		bracketType := p.current().Type
		p.advance() // consume < or [
		keyType := p.current().Value
		p.advance() // consume key type
		p.expect(TOKEN_COMMA)
		valueType := p.current().Value
		p.advance() // consume value type
		if bracketType == TOKEN_LANGLE {
			p.expect(TOKEN_RANGLE)
			return fmt.Sprintf("dict<%s,%s>", keyType, valueType)
		} else {
			p.expect(TOKEN_RBRACKET)
			return fmt.Sprintf("dict[%s,%s]", keyType, valueType)
		}
	}

	return baseType
}

// isScreamingSnakeCase checks if a string is in SCREAMING_SNAKE_CASE format
// (all uppercase with underscores, at least one uppercase letter)
func isScreamingSnakeCase(s string) bool {
	if len(s) == 0 {
		return false
	}

	hasUpper := false
	for _, ch := range s {
		if ch >= 'a' && ch <= 'z' {
			// Has lowercase letter - not screaming snake case
			return false
		}
		if ch >= 'A' && ch <= 'Z' {
			hasUpper = true
		}
		// Allow underscores, digits, and uppercase letters
		if !((ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_') {
			return false
		}
	}

	// Must have at least one uppercase letter to be considered screaming snake case
	return hasUpper
}
