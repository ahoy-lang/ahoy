package ahoy

import "fmt"

// Pre-parsed f-string interpolation.
//
// An f-string's {expr} groups used to reach code generation as raw text, so
// only plain names (and a few hard-coded shapes) survived: {arr.length} or
// {int(y)} were emitted verbatim into the C and broke the build. Instead the
// parser now splits each f-string into literal and expression segments and
// parses every group into an expression AST, which code generation emits
// through the normal expression path.

// fstringSegment is one piece of an f-string: literal text or an {expr}
// group (without the braces).
type fstringSegment struct {
	isExpr  bool
	text    string
	braceAt int // byte offset of '{' in the raw value, for error columns
}

// splitFStringSegments splits value on top-level {..} groups, respecting
// nested braces and quoted strings. An unmatched '{' runs to the end as
// literal text, matching the legacy codegen behaviour.
func splitFStringSegments(value string) []fstringSegment {
	var parts []fstringSegment
	i := 0
	for i < len(value) {
		if value[i] != '{' {
			j := i
			for j < len(value) && value[j] != '{' {
				j++
			}
			parts = append(parts, fstringSegment{text: value[i:j]})
			i = j
			continue
		}
		braceAt := i
		j := i + 1
		depth := 1
		var inStr byte
		for j < len(value) && depth > 0 {
			c := value[j]
			if inStr != 0 {
				if c == '\\' {
					j++
				} else if c == inStr {
					inStr = 0
				}
			} else if c == '"' || c == '\'' {
				inStr = c
			} else if c == '{' {
				depth++
			} else if c == '}' {
				depth--
			}
			if depth > 0 {
				j++
			}
		}
		if depth == 0 && j < len(value) {
			parts = append(parts, fstringSegment{isExpr: true, text: value[i+1 : j], braceAt: braceAt})
			i = j + 1
		} else {
			// Unmatched '{': literal to the end, as before.
			parts = append(parts, fstringSegment{text: value[i:]})
			break
		}
	}
	return parts
}

// parseFStringChildren parses every {expr} group of an f-string value into
// expression AST nodes. Literals become NODE_STRING children. It returns nil
// when any group fails to parse, after recording a diagnostic; callers then
// leave Children unset so codegen falls back to the legacy textual path
// (which will surface the problem the same way it always has).
func (p *Parser) parseFStringChildren(value string, line int) []*ASTNode {
	parts := splitFStringSegments(value)
	hasExpr := false
	for _, pt := range parts {
		if pt.isExpr {
			hasExpr = true
			break
		}
	}
	if !hasExpr {
		return nil
	}
	var children []*ASTNode
	for _, pt := range parts {
		if !pt.isExpr {
			if pt.text != "" {
				children = append(children, &ASTNode{
					Type:     NODE_STRING,
					Value:    pt.text,
					DataType: "string",
					Line:     line,
				})
			}
			continue
		}
		expr, ok := p.parseFStringExpr(pt.text, line, pt.braceAt)
		if !ok {
			return nil
		}
		children = append(children, expr)
	}
	return children
}

// parseFStringExpr parses one {..} group with a sub-parser sharing this
// parser's scope, so names resolve exactly as they would outside the string.
func (p *Parser) parseFStringExpr(expr string, line, braceAt int) (node *ASTNode, ok bool) {
	toks := Tokenize(expr)
	sub := *p // shallow copy: shares scope maps, fresh token stream below
	sub.tokens = toks
	sub.pos = 0
	sub.Errors = nil
	// A syntax failure panics outside lint mode; turn it into a diagnostic.
	defer func() {
		if r := recover(); r != nil {
			node = nil
			ok = false
			msg := fmt.Sprintf("cannot interpolate {%s} in f-string", expr)
			if p.LintMode {
				p.Errors = append(p.Errors, ParseError{
					Message: msg,
					Line:    line,
					Column:  braceAt + 1,
				})
			} else {
				panic(fmt.Sprintf("%s at line %d", msg, line))
			}
		}
	}()
	node = sub.parseExpression()
	if node == nil {
		return nil, false
	}
	// The whole group must be one expression; trailing tokens mean the
	// braces hold something the expression grammar cannot take.
	for sub.pos < len(sub.tokens) {
		t := sub.tokens[sub.pos]
		if t.Type == TOKEN_NEWLINE || t.Type == TOKEN_SEMICOLON || t.Type == TOKEN_EOF {
			sub.pos++
			continue
		}
		msg := fmt.Sprintf("cannot interpolate {%s} in f-string", expr)
		if p.LintMode {
			p.Errors = append(p.Errors, ParseError{
				Message: msg,
				Line:    line,
				Column:  braceAt + 1,
			})
		} else {
			panic(fmt.Sprintf("%s at line %d", msg, line))
		}
		return nil, false
	}
	// Merge diagnostics (e.g. undeclared names in lint mode), retargeted at
	// the f-string's line: sub-tokens are numbered from the group itself.
	for _, e := range sub.Errors {
		e.Line = line
		if e.Column == 0 {
			e.Column = braceAt + 1
		}
		p.Errors = append(p.Errors, e)
	}
	if len(sub.Errors) > 0 {
		return nil, false
	}
	node.Line = line
	return node, true
}
