package main

import "strings"

// Ahoy source formatter.
//
// The rules, in order:
//
//   - Indentation is two spaces per level. The `$` that closes a block sits at
//     the indentation of the line that opened it.
//   - A block whose body is a single simple statement is written on one line
//     ("if x > 0 then print|x| $") when that line fits within formatterMaxWidth.
//     If it does not fit, the body moves onto its own indented line. Loops are
//     never collapsed: the parser rejects a `$` on the same line as a one-line
//     loop, so a loop always keeps its `$` on a line of its own.
//   - Blank lines are collapsed to at most one, with none at the start or end of
//     the file. Comments are preserved verbatim.
//   - Spacing is normalised: runs of spaces become one, a comma is followed by
//     one space, `=` and the unambiguous binary operators are surrounded by one
//     space, and a declaration's `:` is followed by one space (`x: 1`) unless a
//     type follows (`x:int= 1`).
//
// Formatting is idempotent: formatting already-formatted source returns it
// unchanged. Nothing here may change what a program means.

const (
	formatterIndent   = "  "
	formatterMaxWidth = 100
)

// formatSource returns source reformatted according to the rules above.
func formatSource(source string) string {
	lines := normaliseLines(source)
	assignIndent(lines)
	return renderLines(collapseBlocks(lines))
}

// fLine is one logical line of the formatted output.
type fLine struct {
	// text is the content without indentation and without a trailing `$`.
	text string
	// indent is the nesting level.
	indent int
	blank  bool
	// closer marks the `$` that ends a block.
	closer bool
	// selfClosing marks a line that carried its own `$`, i.e. a block already
	// written on one line.
	selfClosing bool
}

// normaliseLines splits source into lines, expands tabs, strips trailing
// whitespace, normalises spacing and moves a trailing `$` into a marker.
func normaliseLines(source string) []*fLine {
	raw := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	out := make([]*fLine, 0, len(raw))
	for _, line := range raw {
		line = strings.ReplaceAll(line, "\t", formatterIndent)
		line = strings.TrimRight(line, " ")
		if strings.TrimSpace(line) == "" {
			out = append(out, &fLine{blank: true})
			continue
		}

		text := strings.TrimSpace(line)
		selfClosing := false
		if text == "$" {
			out = append(out, &fLine{text: "$", closer: true})
			continue
		}
		if strings.HasSuffix(text, "$") {
			text = strings.TrimSpace(strings.TrimSuffix(text, "$"))
			selfClosing = true
		}
		text = normaliseSpacing(text)
		if text == "" {
			out = append(out, &fLine{blank: true})
			continue
		}
		out = append(out, &fLine{text: text, selfClosing: selfClosing})
	}
	return out
}

// normaliseSpacing rewrites the spacing of one line. String literals and
// comments are copied through untouched.
func normaliseSpacing(line string) string {
	if strings.HasPrefix(line, "?") {
		return line // a comment line is left exactly as written
	}

	code, comment := splitComment(line)
	runes := []rune(code)

	// A loop counter is written without a space after its colon: loop i:0 to 9.
	counterColon := strings.HasPrefix(strings.TrimSpace(code), "loop ")

	var b strings.Builder
	b.Grow(len(line) + 8)
	pendingSpace := false
	// inTypeAnnotation is set while a `name:type` declaration is being written,
	// so the `=` that follows it keeps the canonical tight spelling
	// (name:type= value) instead of becoming `name:type = value`.
	inTypeAnnotation := false

	ensureSpace := func() {
		if b.Len() > 0 {
			s := b.String()
			if s[len(s)-1] != ' ' {
				b.WriteString(" ")
			}
		}
	}

	for i := 0; i < len(runes); i++ {
		ch := runes[i]

		// Copy string literals verbatim.
		if ch == '"' || ch == '`' || ch == '\'' {
			if pendingSpace {
				ensureSpace()
				pendingSpace = false
			}
			i = copyStringLiteral(&b, runes, i)
			continue
		}

		if ch == ' ' {
			pendingSpace = true
			continue
		}

		switch ch {
		case ',':
			trimTrailingSpace(&b)
			b.WriteString(", ")
			pendingSpace = false
			continue

		case ':':
			// `::` is the declaration marker: no spaces either side.
			if i+1 < len(runes) && runes[i+1] == ':' {
				trimTrailingSpace(&b)
				b.WriteString("::")
				i++
				pendingSpace = false
				continue
			}
			// `:=` is the walrus operator, spaced like an assignment.
			if i+1 < len(runes) && runes[i+1] == '=' {
				trimTrailingSpace(&b)
				b.WriteString(" := ")
				i++
				pendingSpace = false
				continue
			}
			trimTrailingSpace(&b)
			b.WriteString(":")
			inTypeAnnotation = !counterColon && isTypeAnnotationStart(runes[i+1:])
			if !inTypeAnnotation && !counterColon {
				b.WriteString(" ")
			}
			pendingSpace = false
			continue

		case '=':
			if inTypeAnnotation {
				// name:type= value - no space before the equals.
				b.WriteString("= ")
				inTypeAnnotation = false
				pendingSpace = false
				continue
			}
			// Two-character forms keep their own spelling.
			if i+1 < len(runes) && (runes[i+1] == '=' || runes[i+1] == '>') {
				trimTrailingSpace(&b)
				b.WriteString("=")
				b.WriteRune(runes[i+1])
				b.WriteString(" ")
				i++
				pendingSpace = false
				continue
			}
			if i > 0 && strings.ContainsRune("!<>:+-*/", runes[i-1]) {
				b.WriteString("=")
				pendingSpace = false
				continue
			}
			trimTrailingSpace(&b)
			b.WriteString(" = ")
			pendingSpace = false
			continue
		}

		// A binary operator gets one space on each side when a left operand
		// precedes it; otherwise it is unary or part of a pointer type. A
		// compound assignment keeps its operator and equals together.
		if isBinaryOperatorCandidate(ch) && isOperandEnd(lastNonSpaceRune(&b)) {
			ensureSpace()
			b.WriteRune(ch)
			if i+1 < len(runes) && runes[i+1] == '=' {
				b.WriteString("= ")
				i++
			} else {
				b.WriteString(" ")
			}
			pendingSpace = false
			continue
		}

		if pendingSpace {
			ensureSpace()
			pendingSpace = false
		}
		b.WriteRune(ch)
	}

	result := strings.TrimRight(b.String(), " ")
	if comment == "" {
		return result
	}
	if result == "" {
		return comment
	}
	return result + " " + comment
}

// isBinaryOperatorCandidate reports whether ch is an operator worth spacing.
// `<` and `>` are excluded: they also delimit dicts and generic types.
func isBinaryOperatorCandidate(ch rune) bool {
	switch ch {
	case '+', '-', '*', '/', '%':
		return true
	}
	return false
}

// isOperandEnd reports whether ch can end an operand, so an operator after it is
// binary rather than unary or part of a pointer type.
func isOperandEnd(ch rune) bool {
	if ch == ')' || ch == ']' || ch == '}' {
		return true
	}
	if ch >= '0' && ch <= '9' {
		return true
	}
	return isWordRune(ch)
}

func lastNonSpaceRune(b *strings.Builder) rune {
	s := b.String()
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] != ' ' {
			return rune(s[i])
		}
	}
	return 0
}

func trimTrailingSpace(b *strings.Builder) {
	s := strings.TrimRight(b.String(), " ")
	b.Reset()
	b.WriteString(s)
}

// isTypeAnnotationStart reports whether the text right after a declaration's
// colon begins a type annotation, in which case no space is inserted.
func isTypeAnnotationStart(rest []rune) bool {
	// Spaces between the type and its = / [ / < are allowed, which is what makes
	// `x:int = 5` and `x:int= 5` format to the same thing.
	followedByTypeMarker := func(from int) bool {
		j := from
		for j < len(rest) && rest[j] == ' ' {
			j++
		}
		if j >= len(rest) {
			return true // a bare type at end of line is still an annotation
		}
		switch rest[j] {
		case '=', '[', '<':
			return true
		}
		return false
	}

	for _, word := range []string{"int", "float", "string", "bool", "char", "array", "dict", "raw_string"} {
		wordRunes := []rune(word)
		if len(rest) < len(wordRunes) || string(rest[:len(wordRunes)]) != word {
			continue
		}
		return followedByTypeMarker(len(wordRunes))
	}

	// A capitalised identifier followed by = or [ or < is a struct type.
	if len(rest) > 0 && rest[0] >= 'A' && rest[0] <= 'Z' {
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case '=', '[', '<':
				return true
			case ' ':
				return followedByTypeMarker(i)
			}
		}
	}
	return false
}

// copyStringLiteral copies a string literal starting at runes[start] and returns
// the index of its closing quote.
func copyStringLiteral(b *strings.Builder, runes []rune, start int) int {
	quote := runes[start]
	b.WriteRune(quote)
	for i := start + 1; i < len(runes); i++ {
		b.WriteRune(runes[i])
		if runes[i] == '\\' && i+1 < len(runes) {
			b.WriteRune(runes[i+1])
			i++
			continue
		}
		if runes[i] == quote {
			return i
		}
	}
	return len(runes) - 1
}

// splitComment separates a line into its code and its trailing comment. A `?`
// starts a comment only outside a string literal.
func splitComment(line string) (code, comment string) {
	runes := []rune(line)
	inString := rune(0)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		if inString != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == inString {
				inString = 0
			}
			continue
		}
		switch ch {
		case '"', '`', '\'':
			inString = ch
		case '?':
			return strings.TrimRight(string(runes[:i]), " "), strings.TrimSpace(string(runes[i:]))
		}
	}
	return line, ""
}

// frameKind distinguishes the three ways a construct affects indentation.
type frameKind int

const (
	// frameBlock is a construct closed by its own `$`.
	frameBlock frameKind = iota
	// frameIf is a block whose branches (anif/elseif/else) share its single `$`.
	frameIf
	// frameCase is a switch case: it indents its body but is closed by the next
	// case or by the switch's `$`, never by a `$` of its own.
	frameCase
)

// assignIndent gives every line a nesting level. A line that already carries its
// own `$` is complete and does not open a block.
func assignIndent(lines []*fLine) {
	var stack []frameKind

	closeCases := func() {
		for len(stack) > 0 && stack[len(stack)-1] == frameCase {
			stack = stack[:len(stack)-1]
		}
	}

	for _, line := range lines {
		if line.blank {
			continue
		}
		trimmed := strings.TrimSpace(line.text)

		if line.closer {
			closeCases()
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			line.indent = len(stack)
			continue
		}

		// A branch of an if-chain sits at the level of its `if` and reuses that
		// frame, so it neither indents nor pushes.
		if isIfBranch(trimmed) {
			closeCases()
			line.indent = len(stack) - 1
			if line.indent < 0 {
				line.indent = 0
			}
			continue
		}

		// A switch case indents its body without owning a `$`.
		if isCaseLabel(trimmed) {
			closeCases()
			line.indent = len(stack)
			if !line.selfClosing && !hasCaseBody(trimmed) {
				stack = append(stack, frameCase)
			}
			continue
		}

		line.indent = len(stack)
		if line.selfClosing || !opensBlock(trimmed) {
			continue
		}
		if isIfHeader(trimmed) {
			stack = append(stack, frameIf)
		} else {
			stack = append(stack, frameBlock)
		}
	}
}

// isIfBranch reports whether a line continues an if-chain.
func isIfBranch(trimmed string) bool {
	if trimmed == "else" {
		return true
	}
	for _, prefix := range []string{"else ", "elseif ", "anif "} {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

// isIfHeader reports whether a line starts an if-chain.
func isIfHeader(trimmed string) bool {
	return strings.HasPrefix(trimmed, "if ") || strings.HasPrefix(trimmed, "if(")
}

// isCaseLabel reports whether a line is a `switch` case label.
func isCaseLabel(trimmed string) bool {
	if strings.HasPrefix(trimmed, "on ") || strings.HasPrefix(trimmed, "on(") {
		return true
	}
	return trimmed == "_:" || strings.HasPrefix(trimmed, "_: ")
}

// hasCaseBody reports whether a case label carries its body on the same line.
func hasCaseBody(trimmed string) bool {
	idx := strings.Index(trimmed, ":")
	if idx < 0 {
		return false
	}
	return strings.TrimSpace(trimmed[idx+1:]) != ""
}

// opensBlock reports whether a line starts a block that a `$` must close. A line
// that carries an inline body (`if x then print|y|`) still opens one.
func opensBlock(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || trimmed == "$" {
		return false
	}
	if strings.HasPrefix(trimmed, "@") {
		return endsWithSignatureColon(trimmed)
	}
	for _, keyword := range []string{"struct ", "enum ", "type ", "when "} {
		if strings.HasPrefix(trimmed, keyword) {
			return true
		}
	}
	// `switch` may be preceded by an assignment: x:string= switch expr:
	if hasKeywordOutsideStrings(trimmed, "switch") {
		return true
	}
	if hasKeywordOutsideStrings(trimmed, "then") || hasKeywordOutsideStrings(trimmed, "do") {
		return true
	}
	return isIfBranch(trimmed)
}

// endsWithSignatureColon reports whether a function header ends with the colon
// that introduces its body.
func endsWithSignatureColon(text string) bool {
	idx := signatureColonIndex(text)
	return idx >= 0 && idx == len([]rune(strings.TrimRight(text, " ")))-1
}

// signatureColonIndex returns the index of the colon that ends a function
// header: the first colon after the parameter list's closing bar. Colons inside
// the parameter list, and the `::` declaration marker, are skipped.
func signatureColonIndex(text string) int {
	runes := []rune(text)
	inString := rune(0)
	bars := 0
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		if inString != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == inString {
				inString = 0
			}
			continue
		}
		switch ch {
		case '"', '`', '\'':
			inString = ch
		case '|':
			bars++
		case ':':
			if i+1 < len(runes) && runes[i+1] == ':' {
				i++ // the `::` marker is not a signature colon
				continue
			}
			if bars >= 2 {
				return i
			}
		}
	}
	return -1
}

// hasKeywordOutsideStrings reports whether word appears in text as a whole word,
// outside any string literal or comment.
func hasKeywordOutsideStrings(text, word string) bool {
	return lastKeywordIndexOutsideStrings(text, word) >= 0
}

func lastKeywordIndexOutsideStrings(text, word string) int {
	code, _ := splitComment(text)
	runes := []rune(code)
	wordRunes := []rune(word)
	inString := rune(0)
	last := -1
	for i := 0; i+len(wordRunes) <= len(runes); i++ {
		ch := runes[i]
		if inString != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == inString {
				inString = 0
			}
			continue
		}
		switch ch {
		case '"', '`', '\'':
			inString = ch
			continue
		}
		if string(runes[i:i+len(wordRunes)]) != word {
			continue
		}
		before := i == 0 || !isWordRune(runes[i-1])
		after := i+len(wordRunes) >= len(runes) || !isWordRune(runes[i+len(wordRunes)])
		if before && after {
			last = i
		}
	}
	return last
}

func isWordRune(ch rune) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
}

// collapseBlocks joins a short single-statement block onto one line and expands
// one that does not fit. Loops are never joined.
func collapseBlocks(lines []*fLine) []*fLine {
	out := make([]*fLine, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// A block already written on one line: keep it if it fits, otherwise
		// give the body its own line.
		if line.selfClosing {
			header, body := splitHeaderBody(line.text)
			joined := strings.Repeat(formatterIndent, line.indent) + header
			if body != "" {
				joined += " " + body
			}
			joined += " $"
			if len(joined) <= formatterMaxWidth || body == "" {
				line.text = strings.TrimSpace(header + " " + body) + " $"
				out = append(out, line)
				continue
			}
			out = append(out, &fLine{text: header, indent: line.indent})
			out = append(out, &fLine{text: body, indent: line.indent + 1})
			out = append(out, &fLine{text: "$", indent: line.indent, closer: true})
			continue
		}

		// A block whose body is a single line, closed by its own `$`.
		if !line.blank && !line.closer && opensBlock(line.text) {
			bodyIdx := i + 1
			if bodyIdx < len(lines) && !lines[bodyIdx].blank && !lines[bodyIdx].closer {
				closeIdx := bodyIdx + 1
				if closeIdx < len(lines) && lines[closeIdx].closer &&
					lines[bodyIdx].indent == line.indent+1 &&
					!opensBlock(lines[bodyIdx].text) {
					header, inlineBody := splitHeaderBody(line.text)
					body := inlineBody
					if body == "" {
						body = lines[bodyIdx].text
					}
					if canCollapse(header) && body != "" && !strings.Contains(body, "?") {
						joined := strings.Repeat(formatterIndent, line.indent) + header + " " + body + " $"
						if len(joined) <= formatterMaxWidth {
							out = append(out, &fLine{text: header + " " + body + " $", indent: line.indent, selfClosing: true})
							i = closeIdx
							continue
						}
					}
					out = append(out, &fLine{text: header, indent: line.indent})
					out = append(out, &fLine{text: body, indent: line.indent + 1})
					out = append(out, &fLine{text: "$", indent: line.indent, closer: true})
					i = closeIdx
					continue
				}
			}
		}

		out = append(out, line)
	}
	return out
}

// canCollapse reports whether a construct may be written on one line with its
// body, closed by a `$` on that line.
//
// Two constructs cannot. A one-line loop is rejected outright. A function is
// only joined when it declares a concrete return type: the parser accepts
// `int: return a $` but rejects `void: print|a| $` and, with parameters, an
// implicit `|a:int|: print|a| $`. Those keep their `$` on its own line.
func canCollapse(header string) bool {
	trimmed := strings.TrimSpace(header)
	if hasKeywordOutsideStrings(trimmed, "do") {
		return false
	}
	if strings.HasPrefix(trimmed, "@") {
		return isConcreteReturnType(functionReturnType(trimmed))
	}
	for _, keyword := range []string{"struct ", "enum ", "type ", "switch ", "on ", "when "} {
		if strings.HasPrefix(trimmed, keyword) {
			return false
		}
	}
	return true
}

// functionReturnType returns the declared return type of a function header, or
// "" when none is written.
func functionReturnType(header string) string {
	runes := []rune(header)
	open := -1
	for i, ch := range runes {
		if ch == '|' {
			open = i
			break
		}
	}
	if open < 0 {
		return ""
	}
	closeBar := -1
	for i := open + 1; i < len(runes); i++ {
		if runes[i] == '|' {
			closeBar = i
			break
		}
	}
	if closeBar < 0 {
		return ""
	}
	end := signatureColonIndex(header)
	if end < 0 || end <= closeBar {
		return ""
	}
	return strings.TrimSpace(string(runes[closeBar+1 : end]))
}

// isConcreteReturnType reports whether a declared return type can follow an
// inline body on the same line as the function's `$`.
func isConcreteReturnType(returnType string) bool {
	switch returnType {
	case "", "void", "infer":
		return false
	}
	return true
}

// splitHeaderBody splits "if x then stmt" into "if x then" and "stmt", and a
// function header with an inline body at its signature colon.
func splitHeaderBody(text string) (header, body string) {
	trimmed := strings.TrimSpace(text)

	if strings.HasPrefix(trimmed, "@") {
		if idx := signatureColonIndex(trimmed); idx >= 0 {
			runes := []rune(trimmed)
			return strings.TrimSpace(string(runes[:idx+1])), strings.TrimSpace(string(runes[idx+1:]))
		}
		return trimmed, ""
	}

	if idx := lastKeywordIndexOutsideStrings(trimmed, "then"); idx >= 0 {
		runes := []rune(trimmed)
		return strings.TrimSpace(string(runes[:idx+len("then")])), strings.TrimSpace(string(runes[idx+len("then"):]))
	}
	if idx := lastKeywordIndexOutsideStrings(trimmed, "do"); idx >= 0 {
		runes := []rune(trimmed)
		return strings.TrimSpace(string(runes[:idx+len("do")])), strings.TrimSpace(string(runes[idx+len("do"):]))
	}
	if strings.HasPrefix(trimmed, "else ") {
		return "else", strings.TrimSpace(trimmed[len("else"):])
	}
	return trimmed, ""
}

// renderLines joins the lines, collapsing blank runs and trimming the ends.
func renderLines(lines []*fLine) string {
	var b strings.Builder
	blankPending := false
	wroteAny := false

	for _, line := range lines {
		if line.blank {
			if wroteAny {
				blankPending = true
			}
			continue
		}
		// A blank line directly before a `$` reads as a mistake, so drop it.
		if blankPending && !line.closer {
			b.WriteString("\n")
		}
		blankPending = false
		b.WriteString(strings.Repeat(formatterIndent, line.indent))
		b.WriteString(strings.TrimRight(line.text, " "))
		b.WriteString("\n")
		wroteAny = true
	}
	return b.String()
}
