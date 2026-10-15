package main

import (
	"strings"
	"testing"

	"ahoy"
)

// Tests for `%name` unique-name scene access (bell engine sugar).

func findUniqueNodes(node *ahoy.ASTNode, out *[]*ahoy.ASTNode) {
	if node == nil {
		return
	}
	if node.Type == ahoy.NODE_UNIQUE_ACCESS {
		*out = append(*out, node)
	}
	for _, child := range node.Children {
		findUniqueNodes(child, out)
	}
}

func TestUniqueAccessParsesName(t *testing.T) {
	ast, _ := lintMessages(t, "program p\n@ main ||:\n\tprint|%player|\n$\n")
	var found []*ahoy.ASTNode
	findUniqueNodes(ast, &found)
	if len(found) != 1 {
		t.Fatalf("expected 1 unique access, got %d", len(found))
	}
	if found[0].Value != "player" {
		t.Errorf("expected name player, got %q", found[0].Value)
	}
}

func TestUniqueAccessInExpression(t *testing.T) {
	ast, _ := lintMessages(t, "program p\n@ main ||:\n\tx: %a + %b\n$\n")
	var found []*ahoy.ASTNode
	findUniqueNodes(ast, &found)
	if len(found) != 2 {
		t.Fatalf("expected 2 unique accesses, got %d", len(found))
	}
}

func TestModuloStillParses(t *testing.T) {
	ast, _ := lintMessages(t, "program p\n@ main ||:\n\ta: 7 % 3\n\tb: a % 2\n\tc: a %- 1\n$\n")
	var found []*ahoy.ASTNode
	findUniqueNodes(ast, &found)
	if len(found) != 0 {
		t.Fatalf("modulo must not parse as unique access, got %d", len(found))
	}
}

func TestUniqueAccessNeedsNodesInScope(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ main ||:
	print|%player|
$
`)
	wantError(t, msgs, "Variable 'nodes' is not declared")
}

func TestUniqueAccessNeedsNodeUnique(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ main |nodes:array|:
	print|%player|
$
`)
	wantError(t, msgs, "Unique-name access needs 'node_unique'")
}

func TestUniqueAccessCleanWithNodesAndHelper(t *testing.T) {
	msgs := diagnosticsFor(`program p
@ node_unique |nodes:array, name:string| int:
	return 0
$
@ main |nodes:array|:
	print|%player|
$
`)
	wantNoError(t, msgs, "not declared")
	wantNoError(t, msgs, "node_unique")
}

func TestUniqueAccessGeneratesCall(t *testing.T) {
	src := "program p\n@ node_unique |nodes:array, name:string| int:\n\treturn 0\n$\n@ main |nodes:array|:\n\tprint|%player|\n$\n"
	tokens := ahoy.Tokenize(formatSource(src))
	ast := ahoy.Parse(tokens)
	c := generateC(ast, "test.ahoy")
	if !strings.Contains(c, `node_unique(nodes, "player")`) {
		t.Errorf("expected generated C to call node_unique(nodes, \"player\"), got:\n%s", c)
	}
}
