package ahoy

// renderSubject renders a subject for diagnostics: identifiers and member
// chains read naturally, anything else becomes "this value".
// (The repeated-`if` switch suggestion that used to live here was removed:
// if-chains are valid code and nagging about them contradicts the
// easy-high-level-language policy.)
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
	case NODE_STRING_SLICE:
		if len(node.Children) > 0 {
			return renderSubject(node.Children[0]) + "[lo:hi]"
		}
	}
	return "this value"
}
