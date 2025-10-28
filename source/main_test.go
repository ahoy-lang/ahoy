package main

import (
	"os"
	"strings"
	"testing"
)

func TestTokenizer(t *testing.T) {
	input := `x: 5
if x greater_than 3 then
    ahoy|"hello"|`

	tokens := tokenize(input)

	expectedTypes := []TokenType{
		TOKEN_IDENTIFIER, TOKEN_ASSIGN, TOKEN_NUMBER, TOKEN_NEWLINE,
		TOKEN_IF, TOKEN_IDENTIFIER, TOKEN_GREATER_WORD, TOKEN_NUMBER, TOKEN_THEN, TOKEN_NEWLINE,
		TOKEN_INDENT, TOKEN_AHOY, TOKEN_PIPE, TOKEN_STRING, TOKEN_PIPE, TOKEN_NEWLINE,
		TOKEN_DEDENT, TOKEN_EOF,
	}

	if len(tokens) != len(expectedTypes) {
		t.Fatalf("Expected %d tokens, got %d", len(expectedTypes), len(tokens))
	}

	for i, expectedType := range expectedTypes {
		if tokens[i].Type != expectedType {
			t.Errorf("Token %d: expected type %d, got %d (value: %s)", i, expectedType, tokens[i].Type, tokens[i].Value)
		}
	}
}

func TestParser(t *testing.T) {
	input := `x: 5
if x greater_than 3 then
    return x`

	tokens := tokenize(input)
	ast := parse(tokens)

	if ast.Type != NODE_PROGRAM {
		t.Errorf("Expected program node, got %d", ast.Type)
	}

	if len(ast.Children) != 2 {
		t.Errorf("Expected 2 statements, got %d", len(ast.Children))
	}

	// Test assignment
	assignment := ast.Children[0]
	if assignment.Type != NODE_ASSIGNMENT {
		t.Errorf("Expected assignment node, got %d", assignment.Type)
	}

	if assignment.Value != "x" {
		t.Errorf("Expected variable name 'x', got '%s'", assignment.Value)
	}

	// Test if statement
	ifStmt := ast.Children[1]
	if ifStmt.Type != NODE_IF_STATEMENT {
		t.Errorf("Expected if statement node, got %d", ifStmt.Type)
	}
}

func TestCodeGeneration(t *testing.T) {
	input := `x: 5
y: 10
result: x + y
print|"Result: %d\n", result|`

	tokens := tokenize(input)
	ast := parse(tokens)
	cCode := generateC(ast)

	// Check that C code contains expected elements
	expectedIncludes := []string{"#include <stdio.h>", "#include <stdlib.h>", "#include <stdbool.h>"}
	for _, include := range expectedIncludes {
		if !strings.Contains(cCode, include) {
			t.Errorf("Generated C code missing: %s", include)
		}
	}

	expectedCode := []string{"int x = 5;", "int y = 10;", "int result = (x + y);", "printf("}
	for _, code := range expectedCode {
		if !strings.Contains(cCode, code) {
			t.Errorf("Generated C code missing: %s", code)
		}
	}
}

func TestLanguageFeatures(t *testing.T) {
	// Test boolean operations
	input := `flag: true
if flag and not false then
    x: 1`

	tokens := tokenize(input)
	ast := parse(tokens)
	cCode := generateC(ast)

	// Should translate 'and' to '&&' and 'not' to '!'
	if !strings.Contains(cCode, "&&") {
		t.Error("'and' keyword not translated to '&&'")
	}

	if !strings.Contains(cCode, "!") {
		t.Error("'not' keyword not translated to '!'")
	}
}

func TestCompileAndRun(t *testing.T) {
	// Create a simple test file
	testContent := `x: 42
print|"The answer is: %d\n", x|`

	// Write test file
	err := os.WriteFile("simple_test.py", []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	defer os.Remove("simple_test.py")

	// Compile
	tokens := tokenize(testContent)
	ast := parse(tokens)
	cCode := generateC(ast)

	// Write C file to output directory
	os.MkdirAll("output", 0755)
	err = os.WriteFile("output/simple_test.c", []byte(cCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write C file: %v", err)
	}
	defer os.Remove("output/simple_test.c")

	// Verify C code contains expected elements
	if !strings.Contains(cCode, "int x = 42;") {
		t.Error("C code doesn't contain expected variable declaration")
	}

	if !strings.Contains(cCode, "printf(") {
		t.Error("C code doesn't contain printf call")
	}
}
