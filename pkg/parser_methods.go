package ahoy

import "fmt"

// Method definition headers: Go-style receivers (@ (recv Type) name) and
// associated functions (@ Type.name). Both reuse the ordinary @ function body
// parser; the only differences are the effective (mangled) global name, an
// optional prepended receiver parameter, and the AST method marker.

// Parse a Go-style method definition: @ (recv Type) name |params| rettype:
// The receiver becomes parameter zero, so inside the body it behaves exactly
// like any other parameter (including its type registration for lint).
func (p *Parser) parseMethodDeclaration() *ASTNode {
	p.expect(TOKEN_LPAREN)
	recvName := p.expect(TOKEN_IDENTIFIER)
	if !p.isTypeToken(p.current().Type) {
		errMsg := fmt.Sprintf("Method receiver needs an explicit type, e.g. @ (%s Point) at line %d",
			recvName.Value, recvName.Line)
		if p.LintMode {
			p.recordError(errMsg)
		} else {
			panic(errMsg)
		}
	}
	recvType := ""
	if p.isTypeToken(p.current().Type) {
		recvType = p.parseComplexReturnType()
	}
	p.expect(TOKEN_RPAREN)
	methodName := p.expectDefName("method")

	recvNorm := NormalizeReceiverType(recvType)
	mangled := Token{
		Type:   TOKEN_IDENTIFIER,
		Value:  MangleMethodName(recvNorm, methodName.Value),
		Line:   methodName.Line,
		Column: methodName.Column,
	}
	recvParam := &ASTNode{
		Type:     NODE_IDENTIFIER,
		Value:    recvName.Value,
		DataType: recvType,
		Line:     recvName.Line,
		Column:   recvName.Column,
	}
	return p.parseFunctionWithDoubleColon(mangled, recvParam,
		&MethodInfo{Receiver: recvNorm, Name: methodName.Value})
}

// Parse an associated function definition: @ Type.name |params| rettype:
// There is no receiver; the function is called as Type.name|args|. Whether
// Type names a struct or enum is validated by the checker (the definition may
// precede the type declaration in the file).
func (p *Parser) parseAssociatedDeclaration(typeName Token) *ASTNode {
	p.expect(TOKEN_DOT)
	methodName := p.expectDefName("associated function")
	mangled := Token{
		Type:   TOKEN_IDENTIFIER,
		Value:  MangleMethodName(typeName.Value, methodName.Value),
		Line:   methodName.Line,
		Column: methodName.Column,
	}
	return p.parseFunctionWithDoubleColon(mangled, nil,
		&MethodInfo{Receiver: NormalizeReceiverType(typeName.Value), Name: methodName.Value, Associated: true})
}
