package main

import (
	"strings"
	"testing"

	"ahoy"
)

// Tests for `when NAME then A $ else B $` (compile-time else branch).

func findWhenNodes(node *ahoy.ASTNode, out *[]*ahoy.ASTNode) {
	if node == nil {
		return
	}
	if node.Type == ahoy.NODE_WHEN_STATEMENT {
		*out = append(*out, node)
	}
	for _, child := range node.Children {
		findWhenNodes(child, out)
	}
}

func lintMessages(t *testing.T, src string) (*ahoy.ASTNode, []string) {
	t.Helper()
	ast, errs := ahoy.ParseLintWithPath(ahoy.Tokenize(src), "test.ahoy")
	return ast, fatalMessages(errs)
}

func TestWhenElseParsesTwoBranches(t *testing.T) {
	ast, _ := lintMessages(t, "program p\n@ main ||:\n\twhen FOO then\n\t\tprint|\"a\"|\n\t$\n\telse\n\t\tprint|\"b\"|\n\t$\n$\n")
	var found []*ahoy.ASTNode
	findWhenNodes(ast, &found)
	if len(found) != 1 {
		t.Fatalf("expected 1 when statement, got %d", len(found))
	}
	if len(found[0].Children) != 2 {
		t.Fatalf("expected 2 branches, got %d", len(found[0].Children))
	}
	if found[0].Value != "FOO" {
		t.Errorf("expected condition FOO, got %q", found[0].Value)
	}
}

func TestWhenWithoutElseParsesOneBranch(t *testing.T) {
	ast, _ := lintMessages(t, "program p\n@ main ||:\n\twhen FOO then\n\t\tprint|\"a\"|\n\t$\n$\n")
	var found []*ahoy.ASTNode
	findWhenNodes(ast, &found)
	if len(found) != 1 {
		t.Fatalf("expected 1 when statement, got %d", len(found))
	}
	if len(found[0].Children) != 1 {
		t.Fatalf("expected 1 branch, got %d", len(found[0].Children))
	}
}

func TestWhenElseReturnsSatisfyCoverage(t *testing.T) {
	_, msgs := lintMessages(t, "program p\n@ pick || int:\n\twhen FOO then\n\t\treturn 1\n\t$\n\telse\n\t\treturn 2\n\t$\n$\n@ main ||:\n\tv: pick||\n\tprint|v|\n$\n")
	for _, m := range msgs {
		if strings.Contains(m, "fall off") {
			t.Errorf("when/else with returns in both branches must satisfy coverage, got: %q", m)
		}
	}
}

func TestWhenWithoutElseStillNeedsTailReturn(t *testing.T) {
	_, msgs := lintMessages(t, "program p\n@ pick || int:\n\twhen FOO then\n\t\treturn 1\n\t$\n$\n@ main ||:\n\tv: pick||\n\tprint|v|\n$\n")
	wantError(t, msgs, "fall off")
}
