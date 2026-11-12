package ahoy

import (
	"fmt"
	"strconv"
)

// Primary expressions and literals: arrays, objects, dicts, lambdas.

// castTypeNames are the identifier-spelled cast types. `int`, `float`, `char`
// and `string` are real keywords (handled by the type-token cast path); these
// ride the identifier path and are recognised in parsePrimaryExpression when
// followed by `(`. They are types too, so `x: i64 = ...` and `|x:u32(` work.
var castTypeNames = map[string]bool{
	"f32": true,
	"i8":  true, "u8": true,
	"i16": true, "u16": true,
	"i32": true, "u32": true,
	"i64": true, "u64": true,
}

func isCastTypeName(name string) bool {
	return castTypeNames[name]
}

// IsCastTypeName reports whether name is an identifier-spelled cast type.
// Exported for the codegen package (inferType).
func IsCastTypeName(name string) bool {
	return castTypeNames[name]
}

// tryParseStringSlice parses the inside of [...] as a Go/Odin-style
// string slice (s[lo:hi], s[lo:], s[:hi], s[:]) instead of a plain index.
// Call with the current token right after '[' was consumed; target is the
// value being indexed. Omitted bounds become 0 (lo) and 999999999 (hi):
// codegen clamps both, so the defaults are exact and no nil children or
// sentinel states exist. Returns (nil, false) with the position rewound
// when this is not a slice, so the plain-index path proceeds untouched.
func (p *Parser) tryParseStringSlice(target *ASTNode, line int) (*ASTNode, bool) {
	intLit := func(v string) *ASTNode {
		return &ASTNode{Type: NODE_NUMBER, Value: v, DataType: "int", Line: line}
	}
	saved := p.pos
	var lo, hi *ASTNode
	if p.current().Type == TOKEN_ASSIGN {
		// s[:hi] / s[:]
		lo = intLit("0")
		p.advance()
	} else {
		lo = p.parseExpression()
		if p.current().Type != TOKEN_ASSIGN {
			p.pos = saved
			return nil, false
		}
		p.advance() // consume :
	}
	if p.current().Type == TOKEN_RBRACKET {
		hi = intLit("999999999")
	} else {
		hi = p.parseExpression()
	}
	if p.current().Type != TOKEN_RBRACKET {
		p.pos = saved
		return nil, false
	}
	p.advance() // consume ]
	return &ASTNode{
		Type:     NODE_STRING_SLICE,
		Value:    "",
		Children: []*ASTNode{target, lo, hi},
		Line:     line,
	}, true
}

func (p *Parser) parsePrimaryExpression() *ASTNode {
	// A lambda used as a value: (x:int): x * 2, (a, b): a + b. It has to be
	// recognised before the parenthesized-expression case below.
	if p.current().Type == TOKEN_LPAREN && p.looksLikeLambda() {
		return p.parseLambda()
	}

	switch p.current().Type {
	case TOKEN_NUMBER:
		token := p.current()
		p.advance()
		node := &ASTNode{
			Type:  NODE_NUMBER,
			Value: token.Value,
			Line:  token.Line,
		}
		// Determine if it's int or float. Prefixed integers (0x hex from
		// 0x/0b literals, leading-0 octal from 0o) are always int;
		// Atoi only understands decimal.
		if _, err := strconv.Atoi(token.Value); err == nil {
			node.DataType = "int"
		} else if len(token.Value) > 2 && token.Value[0] == '0' &&
			(token.Value[1] == 'x' || token.Value[1] == 'X') {
			node.DataType = "int"
		} else if len(token.Value) > 1 && token.Value[0] == '0' &&
			token.Value[1] >= '0' && token.Value[1] <= '7' {
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
		// Pre-parse each {expr} group so codegen emits it through the
		// normal expression path. Groups that fail keep Children unset and
		// fall back to the legacy textual handling.
		if children := p.parseFStringChildren(token.Value, token.Line); children != nil {
			node.Children = children
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

	case TOKEN_NULL:
		// null is a nil reference: the zero value of every pointer field,
		// comparable with `is null` / `is not null` (pointer comparison,
		// never strcmp). Assignable to pointer-typed targets.
		token := p.current()
		p.advance()
		return &ASTNode{
			Type:     NODE_NULL_LITERAL,
			Value:    "null",
			DataType: "null",
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

		// Check for array/dict access identifier[index] — `[]` is the unified
		// index operator. Arrays (`arr[i]`) and dicts (`d["k"]`, `d[var]`,
		// `d[expr]`) share it; the node type is routed by key shape and by
		// the base variable's known type. `<>` access is deprecated (warning)
		// but still parses; see the TOKEN_LANGLE branch below.
		if p.current().Type == TOKEN_LBRACKET {
			p.advance()
			identNode := &ASTNode{Type: NODE_IDENTIFIER, Value: token.Value, Line: token.Line}
			var node *ASTNode
			// String slice (s[lo:hi]) before plain index
			if slice, ok := p.tryParseStringSlice(identNode, token.Line); ok {
				node = slice
			} else {
				index := p.parseExpression()
				p.expect(TOKEN_RBRACKET)

				// Route `x[...]` to dict access when the base is known dict,
				// or (unknown base) when the key is string-like. A known
				// array with a string key is an error — arrays take ints.
				isDict := false
				if p.isDictVar(token.Value) {
					isDict = true
				} else if p.isArrayVar(token.Value) {
					if p.LintMode && (index.Type == NODE_STRING || index.Type == NODE_F_STRING || index.Type == NODE_RAW_STRING) {
						p.recordErrorAtLine(fmt.Sprintf("array '%s' takes an int index, use a dict for string keys", token.Value), token.Line)
					}
					isDict = false
				} else if index.Type == NODE_STRING || index.Type == NODE_F_STRING || index.Type == NODE_RAW_STRING || index.Type == NODE_CHAR {
					isDict = true
				}
				// Unknown base + string key prefers dict so `d["k"]` works
				// even when the dict's type hasn't been recorded yet
				// (function params, forward refs). The codegen delegates
				// array-typed `[]` with dict bases to the dict path too.

				if isDict {
					node = &ASTNode{
						Type:     NODE_DICT_ACCESS,
						Value:    token.Value,
						Children: []*ASTNode{index},
						Line:     token.Line,
					}
				} else {
					node = &ASTNode{
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
				}
			}

			// Dict access does not chain with `[]` (dict of arrays is read
			// via a temp). Array chaining stays here for grid[r][c].
			if node.Type == NODE_DICT_ACCESS {
				// Check for member access after dict access
				if p.current().Type == TOKEN_DOT {
					return p.parseMemberAccessChain(node)
				}
				return node
			}

			// Check for chained array access (2D arrays): grid[r][c].
			// A slice composes here too (s[0:4][1:2] slices the fresh
			// string); a plain index into one (s[0:4][0]) fails downstream
			// exactly like indexing a string does today.
			for p.current().Type == TOKEN_LBRACKET {
				p.advance()
				if slice, ok := p.tryParseStringSlice(node, token.Line); ok {
					node = slice
					continue
				}
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
				obj := p.parseObjectLiteralRaw() // Will consume the closing }
				if obj.Type == NODE_OBJECT_LITERAL {
					obj.Value = token.Value // Set the type name
					// A method or field chain on a struct literal:
					// V{...}.twice(), V{...}.x — chained after stamping
					// so the receiver carries the struct type.
					if p.current().Type == TOKEN_DOT {
						return p.parseMemberAccessChain(obj)
					}
				}
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

			// It's dict access: dict<key> — deprecated in favor of dict["key"].
			// Still parses, but lint warns so `<>` can stay for literals only.
			accessor := p.parseCallArgument()
			p.expect(TOKEN_RANGLE)

			p.recordWarningAtLine(fmt.Sprintf("dict access with <> is deprecated, use %s[\"key\"] instead of %s<\"key\">", token.Value, token.Value), token.Line, token.Column)

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

		// Identifier-spelled cast types (f32, i64, u32, ...): `f32(3.7)`,
		// `i64(n)`. They are not reserved keywords, so they ride the identifier
		// path; recognise them when followed by `(` and produce the same
		// NODE_CALL shape the type-token cast path (`int(x)`) builds.
		if p.current().Type == TOKEN_LPAREN && isCastTypeName(token.Value) {
			p.advance() // consume opening (
			arg := p.parseExpression()
			p.expect(TOKEN_RPAREN)
			return &ASTNode{
				Type:     NODE_CALL,
				Value:    token.Value,
				Children: []*ASTNode{arg},
				Line:     token.Line,
			}
		}

		// Function calls use parens: name(args), name(). Definitions keep
		// |...|. A bare `name(` always opens a call; there is no ambiguity
		// with grouping because grouping starts with `(`, not an identifier.
		if p.current().Type == TOKEN_LPAREN {
			// Distinct-type conversions ride the same shape: NodeId(7).
			// Cast-type names (f32, i64, ...) are handled above but produce
			// the same node; general identifiers fall through here.
			p.advance() // consume (

			// Validate no recursion
			p.validateFunctionCall(token.Value, token.Line, token.Column)

			call := &ASTNode{
				Type:   NODE_CALL,
				Value:  token.Value,
				Line:   token.Line,
				Column: token.Column,
			}

			// Increment depth so bare zero-arg names inside args stay variables
			p.inFunctionCall++

			// Parse arguments until closing paren (allow newlines for multiline calls)
			for p.current().Type == TOKEN_NEWLINE {
				p.advance()
			}
			if p.current().Type != TOKEN_RPAREN && p.current().Type != TOKEN_EOF {
				for {
					for p.current().Type == TOKEN_NEWLINE {
						p.advance()
					}
					if p.current().Type == TOKEN_RPAREN || p.current().Type == TOKEN_EOF {
						break
					}
					arg := p.parseCallArgument()
					call.Children = append(call.Children, arg)

					for p.current().Type == TOKEN_NEWLINE {
						p.advance()
					}
					if p.current().Type == TOKEN_COMMA {
						p.advance()
						continue
					}
					break
				}
			}
			for p.current().Type == TOKEN_NEWLINE {
				p.advance()
			}

			// Consume closing paren
			if p.current().Type == TOKEN_RPAREN {
				p.advance()
			} else if p.LintMode {
				p.recordError(fmt.Sprintf("Expected ')' to close call '%s(' at line %d", token.Value, token.Line))
			}

			p.inFunctionCall--
			// A chain on the call result: foo(a).bar(b), foo(a)[0].
			if p.current().Type == TOKEN_DOT {
				return p.parseMemberAccessChain(call)
			}
			if p.current().Type == TOKEN_LBRACKET {
				// Indexing into a call result shares the chained shape.
				node := call
				for p.current().Type == TOKEN_LBRACKET {
					p.advance()
					if slice, ok := p.tryParseStringSlice(node, token.Line); ok {
						node = slice
						continue
					}
					idx := p.parseExpression()
					p.expect(TOKEN_RBRACKET)
					node = &ASTNode{
						Type:     NODE_ARRAY_ACCESS,
						Value:    "",
						Children: []*ASTNode{node, idx},
						Line:     token.Line,
					}
				}
				if p.current().Type == TOKEN_DOT {
					return p.parseMemberAccessChain(node)
				}
				return node
			}
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

		// Check if this is a zero-arg function that can be called without ()
		// Only do this at top-level (not inside another function call, and not
		// under & where the name itself is the function value). A declared
		// variable shadows the C function (same rule as the call guard
		// above): without this, every libc zero-arg function the
		// header follower registers (clock, system, ...) hijacks same-named
		// locals, emitting clock() for a char* variable.
		if p.inFunctionCall == 0 && !p.inFunctionReference && p.zeroArgFunctions[token.Value] && !p.nameIsKnownVariable(token.Value) {
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
		// A chain on the parenthesized value: (V{...}).twice(), (a+b).x.
		if p.current().Type == TOKEN_DOT {
			return p.parseMemberAccessChain(expr)
		}
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
				obj := p.parseObjectLiteralRaw()
				if obj.Type == NODE_OBJECT_LITERAL {
					obj.Value = token.Value // Set the type name
					obj.Line = token.Line   // Set the line number
					if p.current().Type == TOKEN_DOT {
						return p.parseMemberAccessChain(obj)
					}
				}
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
				obj := p.parseObjectLiteralRaw()
				if obj.Type == NODE_OBJECT_LITERAL {
					obj.Value = token.Value // Set the type name
					obj.Line = token.Line   // Set the line number
					if p.current().Type == TOKEN_DOT {
						return p.parseMemberAccessChain(obj)
					}
				}
				return obj
			}

			// Otherwise parse as comma-separated values: OLD vector2<10,20> syntax
			args := []*ASTNode{}
			for p.current().Type != TOKEN_RANGLE && p.current().Type != TOKEN_EOF {
				arg := p.parseCallArgumentExpression() // bitwise words included; > is still not a comparison here
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

		// Check if this is a cast (followed by parens): int(x), float(x), etc.
		// Pipe casts (`int(x)`) were removed; calls and casts both use `()`.
		if p.current().Type == TOKEN_PIPE {
			p.recordError(fmt.Sprintf("cast with |...| was removed, use %s(x) instead of %s(x) (line %d)", token.Value, token.Value, token.Line))
			p.advance() // consume | to recover
			args := []*ASTNode{}
			for p.current().Type != TOKEN_PIPE && p.current().Type != TOKEN_EOF && p.current().Type != TOKEN_NEWLINE {
				arg := p.parseCallArgumentExpression()
				args = append(args, arg)
				if p.current().Type == TOKEN_COMMA {
					p.advance()
				} else {
					break
				}
			}
			if p.current().Type == TOKEN_PIPE {
				p.advance()
			}
			if len(args) == 0 {
				args = append(args, &ASTNode{Type: NODE_IDENTIFIER, Value: "error"})
			}
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

// reportCallInCast names the fix when a pipe-call is used where only plain
// values parse — inside a cast's paren form (n: int(helper())). Calls are now
// `name(...)`; a stale `name(...)` there is always a temp-variable fix.
// Lint records and continues, parse raises for the caller.
func (p *Parser) reportCallInCast(name Token) {
	errMsg := fmt.Sprintf("function call '%s(...)' cannot be used inside a cast (line %d:%d): assign the result to a variable first, then cast the variable", name.Value, name.Line, name.Column)
	if p.LintMode {
		p.recordError(errMsg)
	} else {
		panic(errMsg)
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
	obj := p.parseObjectLiteralRaw()
	// A chain on the literal itself (anonymous receiver): {k: v}.method.
	// Named instantiation (V{...}.method) parses through the raw form so
	// the type name is stamped before chaining, not onto the chained node.
	if p.current().Type == TOKEN_DOT {
		return p.parseMemberAccessChain(obj)
	}
	return obj
}

// parseObjectLiteralRaw parses {k: v, ...} without chaining a trailing
// .member: instantiation sites stamp the type name first and chain after,
// so the receiver carries the struct type instead of "object".
func (p *Parser) parseObjectLiteralRaw() *ASTNode {
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

	return object
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

	// Record every string key globally so `=` on a never-seen key can warn
	// even when the dict comes from `return <...>` (no varName at this site).
	if p.globalDictKeys == nil {
		p.globalDictKeys = make(map[string]bool)
	}
	for i := 0; i+1 < len(dict.Children); i += 2 {
		if k := dict.Children[i]; k.Type == NODE_STRING {
			p.globalDictKeys[k.Value] = true
		}
	}

	// Check for member access after dict literal
	if p.current().Type == TOKEN_DOT {
		return p.parseMemberAccessChain(dict)
	}

	return dict
}

// Parse enum declaration
