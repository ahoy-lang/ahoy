package ahoy

// Repeated-`is` lint: runs of bare `if SUBJ is LITERAL then` checks against
// the same side-effect-free subject read as a dispatch table, so the checker
// suggests `switch SUBJ:` with `on` cases. It is a warning, never an error:
// the if-chain is valid code, just verbose.

// repeatedIfThreshold is how many consecutive `if SUBJ is LITERAL` checks
// against one subject it takes to suggest a switch. Two is a judgment call;
// three is clearly a dispatch table.
const repeatedIfThreshold = 3

// suggestSwitchForIfChains warns on runs of bare `if SUBJ is LITERAL then`
// checks against the same side-effect-free subject. Only direct block
// siblings count (anif chains nest, so they never qualify), and only pure
// equality with a case-compatible literal — mixed conditions stay exactly as
// expressive as they are.
func (c *checker) suggestSwitchForIfChains(children []*ASTNode) {
	var runSubject *ASTNode
	runStart, runLen := -1, 0
	flush := func() {
		if runLen >= repeatedIfThreshold && runStart >= 0 {
			subj := renderSubject(runSubject)
			c.warningAt(children[runStart], "Found %d consecutive 'if %s is ...' checks; consider 'switch %s:' with 'on' cases (and '_' for the default)",
				runLen, subj, subj)
		}
		runSubject, runStart, runLen = nil, -1, 0
	}
	for i, child := range children {
		subj, ok := singleIsSubject(child)
		if !ok {
			flush()
			continue
		}
		if runSubject != nil && !sameExpr(runSubject, subj) {
			flush()
		}
		if runSubject == nil {
			runSubject = subj
			runStart = i
		}
		runLen++
	}
	flush()
}

// singleIsSubject matches a bare `if SUBJ is LITERAL then ...` — no else,
// no boolean wrapping — and returns the subject when it is side-effect-free
// (a switch would evaluate it once) and the value is case-compatible.
func singleIsSubject(node *ASTNode) (*ASTNode, bool) {
	if node == nil || node.Type != NODE_IF_STATEMENT || len(node.Children) != 2 {
		return nil, false
	}
	cond := node.Children[0]
	if cond == nil || cond.Type != NODE_BINARY_OP || cond.Value != "is" || len(cond.Children) != 2 {
		return nil, false
	}
	subj, val := cond.Children[0], cond.Children[1]
	if subj == nil || val == nil || !isPureSubject(subj) {
		return nil, false
	}
	switch val.Type {
	case NODE_NUMBER, NODE_STRING, NODE_CHAR, NODE_BOOLEAN,
		NODE_IDENTIFIER, NODE_MEMBER_ACCESS, NODE_STATIC_MEMBER_ACCESS:
		return subj, true
	}
	return nil, false
}

// isPureSubject reports whether an expression can be evaluated twice without
// changing its value: no calls or anything lowering to one anywhere inside.
func isPureSubject(node *ASTNode) bool {
	if node == nil {
		return true
	}
	switch node.Type {
	case NODE_CALL, NODE_METHOD_CALL, NODE_UNIQUE_ACCESS, NODE_F_STRING,
		NODE_LAMBDA, NODE_TERNARY, NODE_SWITCH_STATEMENT:
		return false
	}
	for _, child := range node.Children {
		if !isPureSubject(child) {
			return false
		}
	}
	return true
}

// sameExpr reports structural equality for suggestion purposes: type, value
// and children must match; positions are ignored (a run spans lines).
func sameExpr(a, b *ASTNode) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Type != b.Type || a.Value != b.Value || len(a.Children) != len(b.Children) {
		return false
	}
	for i := range a.Children {
		if !sameExpr(a.Children[i], b.Children[i]) {
			return false
		}
	}
	return true
}

// renderSubject renders a subject for diagnostics: identifiers and member
// chains read naturally, anything else becomes "this value".
func renderSubject(node *ASTNode) string {
	if node == nil {
		return "this value"
	}
	switch node.Type {
	case NODE_IDENTIFIER:
		if node.Value != "" {
			return node.Value
		}
	case NODE_MEMBER_ACCESS, NODE_OBJECT_ACCESS:
		if len(node.Children) > 0 {
			return renderSubject(node.Children[0]) + "." + node.Value
		}
		if node.Value != "" {
			return node.Value
		}
	case NODE_ARRAY_ACCESS, NODE_DICT_ACCESS:
		if node.Value != "" {
			return node.Value + "[...]"
		}
	}
	return "this value"
}
