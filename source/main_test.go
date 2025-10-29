package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ahoy"
)

func TestTokenizer(t *testing.T) {
	input := `x: 5
if x greater_than 3 then
    ahoy|"hello"|`

	tokens := ahoy.Tokenize(input)

	expectedTypes := []ahoy.TokenType{
		ahoy.TOKEN_IDENTIFIER, ahoy.TOKEN_ASSIGN, ahoy.TOKEN_NUMBER, ahoy.TOKEN_NEWLINE,
		ahoy.TOKEN_IF, ahoy.TOKEN_IDENTIFIER, ahoy.TOKEN_GREATER_WORD, ahoy.TOKEN_NUMBER, ahoy.TOKEN_THEN, ahoy.TOKEN_NEWLINE,
		ahoy.TOKEN_INDENT, ahoy.TOKEN_AHOY, ahoy.TOKEN_PIPE, ahoy.TOKEN_STRING, ahoy.TOKEN_PIPE, ahoy.TOKEN_NEWLINE,
		ahoy.TOKEN_DEDENT, ahoy.TOKEN_EOF,
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

	tokens := ahoy.Tokenize(input)
	ast := ahoy.Parse(tokens)

	if ast.Type != ahoy.NODE_PROGRAM {
		t.Errorf("Expected program node, got %d", ast.Type)
	}

	if len(ast.Children) != 2 {
		t.Errorf("Expected 2 statements, got %d", len(ast.Children))
	}

	// Test assignment
	assignment := ast.Children[0]
	if assignment.Type != ahoy.NODE_ASSIGNMENT {
		t.Errorf("Expected assignment node, got %d", assignment.Type)
	}

	if assignment.Value != "x" {
		t.Errorf("Expected variable name 'x', got '%s'", assignment.Value)
	}

	// Test if statement
	ifStmt := ast.Children[1]
	if ifStmt.Type != ahoy.NODE_IF_STATEMENT {
		t.Errorf("Expected if statement node, got %d", ifStmt.Type)
	}
}

func TestCodeGeneration(t *testing.T) {
	input := `x: 5
y: 10
result: x + y
print|"Result: %d\n", result|`

	tokens := ahoy.Tokenize(input)
	ast := ahoy.Parse(tokens)
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

	tokens := ahoy.Tokenize(input)
	ast := ahoy.Parse(tokens)
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
	tokens := ahoy.Tokenize(testContent)
	ast := ahoy.Parse(tokens)
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

// OutputTest represents a test case with expected output
type OutputTest struct {
	Name           string
	InputFile      string
	ExpectedOutput string
}

// compileAndRun compiles an Ahoy file and returns the program output
func compileAndRun(t *testing.T, ahoyFile string) (string, error) {
	// Read the source file
	content, err := os.ReadFile(ahoyFile)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	// Format, tokenize, and parse
	formattedContent := formatSource(string(content))
	tokens := ahoy.Tokenize(formattedContent)
	ast := ahoy.Parse(tokens)

	// Generate C code
	cCode := generateC(ast)

	// Determine output paths
	baseName := strings.TrimSuffix(filepath.Base(ahoyFile), ".ahoy")
	outputDir := "../test/output"
	os.MkdirAll(outputDir, 0755)

	cFile := filepath.Join(outputDir, baseName+".c")
	executable := filepath.Join(outputDir, baseName)

	// Write C file
	err = os.WriteFile(cFile, []byte(cCode), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write C file: %v", err)
	}

	// Compile C code
	cmd := exec.Command("gcc", "-o", executable, cFile, "-lm")
	var compileErr bytes.Buffer
	cmd.Stderr = &compileErr
	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("compilation failed: %v\n%s", err, compileErr.String())
	}

	// Run the executable
	runCmd := exec.Command(executable)
	var output bytes.Buffer
	runCmd.Stdout = &output
	runCmd.Stderr = &output
	err = runCmd.Run()
	if err != nil {
		return "", fmt.Errorf("execution failed: %v\n%s", err, output.String())
	}

	return output.String(), nil
}

// TestProgramOutput tests programs against expected output
func TestProgramOutput(t *testing.T) {
	tests := []OutputTest{
		{
			Name:      "Simple Output Test",
			InputFile: "../test/input/simple_output_test.ahoy",
			ExpectedOutput: `=== Simple Output Test ===
x = 10
y = 20
x + y = 30
=== Test Complete ===
`,
		},
		{
			Name:      "Array Methods",
			InputFile: "../test/input/array_methods_test.ahoy",
			ExpectedOutput: `Array: [4, 5, 5, 2]
Length: 4
Original: [1, 2, 3, 4]
Popped: 4
After pop: [1, 2, 3]
After push(5): [1, 2, 3, 5]
Ordered: [1, 2, 3, 4, 5]
Shuffled: [5, 1, 4, 2, 3]
Random pick from [10, 20, 30, 40, 50]: 30
Sum of [10, 20, 30, 40]: 100
Original: [1, 2, 3, 4, 5]
Doubled: [2, 4, 6, 8, 10]
Squared: [1, 4, 9, 16, 25]
All numbers: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]
Even numbers: [2, 4, 6, 8, 10]
Greater than 5: [6, 7, 8, 9, 10]
Unsorted: [5, 2, 8, 1, 9, 3]
Sorted: [1, 2, 3, 5, 8, 9]
Reversed: [9, 8, 5, 3, 2, 1]
Fruits: [-1556213472, -1556213466, -1556213459]
Has banana: 1
Has grape: 0
Original data: [5, 2, 8, 1, 9, 3, 7, 4, 6]
Sorted desc, filtered >5: [9, 8, 7, 6]
Chained operations: [10, 20, 30, 40, 50]
Array methods test complete
`,
		},
		{
			Name:      "Simple Showcase",
			InputFile: "../test/input/simple_showcase.ahoy",
			ExpectedOutput: `=== Ahoy Language Showcase ===
Array: [5,2,8,1,9,3]
Length: 6
Sum: 38
Map [1-5] * 2: [2,4,6,8,10]
Filter evens: [2,4,6]
Chain filter>2 then *10: [30,40,50]
[1,2,3].push(4).push(5): length 5
Loop 0-10 break at 5: 0 1 2 3 4
Loop 0-6 skip evens: 1 3 5
Tuple x=100, y=200
Constant MAX_SIZE: 1000
Filter>5 then *2: length 4
Values: 20 16 24 14
=== All Features Working! ===
`,
		},
		{
			Name:      "Conditionals",
			InputFile: "../test/input/conditionals_test.ahoy",
			ExpectedOutput: `=== Conditionals Test ===
x is 10
a is greater than 5
Testing multiple conditions:
b is not zero and c is positive
Value is medium
Grade: B
Letter is a vowel
Count: 5
=== All Conditionals Working! ===
`,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			// Check if input file exists
			if _, err := os.Stat(test.InputFile); os.IsNotExist(err) {
				t.Skipf("Input file not found: %s", test.InputFile)
				return
			}

			// Compile and run
			output, err := compileAndRun(t, test.InputFile)
			if err != nil {
				t.Fatalf("Failed to compile and run: %v", err)
			}

			// Compare output
			if output != test.ExpectedOutput {
				t.Errorf("Output mismatch:\n=== Expected ===\n%s\n=== Got ===\n%s\n=== End ===",
					test.ExpectedOutput, output)
			}
		})
	}
}

// TestProgramOutputFlexible tests programs with flexible output matching
func TestProgramOutputFlexible(t *testing.T) {
	tests := []struct {
		Name              string
		InputFile         string
		ExpectedLines     []string // Lines that must appear in output
		ForbiddenLines    []string // Lines that must NOT appear
		ExpectedLineCount int      // Expected number of lines (0 = don't check)
	}{
		{
			Name:      "Break and Skip",
			InputFile: "../test/input/break_skip_test.ahoy",
			ExpectedLines: []string{
				"=== Break and Skip Test ===",
				"=== All Tests Complete ===",
			},
		},
		{
			Name:      "Loops Test",
			InputFile: "../test/input/loops_test.ahoy",
			ExpectedLines: []string{
				"=== Loop Tests ===",
			},
		},
		{
			Name:      "Array Methods (Random Tolerant)",
			InputFile: "../test/input/array_methods_test.ahoy",
			ExpectedLines: []string{
				"Array: [4, 5, 5, 2]",
				"Length: 4",
				"Original: [1, 2, 3, 4]",
				"Popped: 4",
				"After pop: [1, 2, 3]",
				"After push(5): [1, 2, 3, 5]",
				"Sum of [10, 20, 30, 40]: 100",
				"Doubled: [2, 4, 6, 8, 10]",
				"Squared: [1, 4, 9, 16, 25]",
				"Even numbers: [2, 4, 6, 8, 10]",
				"Sorted: [1, 2, 3, 5, 8, 9]",
				"Array methods test complete",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			// Check if input file exists
			if _, err := os.Stat(test.InputFile); os.IsNotExist(err) {
				t.Skipf("Input file not found: %s", test.InputFile)
				return
			}

			// Compile and run
			output, err := compileAndRun(t, test.InputFile)
			if err != nil {
				t.Fatalf("Failed to compile and run: %v", err)
			}

			// Check line count if specified
			if test.ExpectedLineCount > 0 {
				actualLines := len(strings.Split(strings.TrimSpace(output), "\n"))
				if actualLines != test.ExpectedLineCount {
					t.Errorf("Expected %d lines of output, got %d", test.ExpectedLineCount, actualLines)
				}
			}

			// Check for expected lines
			for _, expectedLine := range test.ExpectedLines {
				if !strings.Contains(output, expectedLine) {
					t.Errorf("Expected output to contain: %q\nGot: %s", expectedLine, output)
				}
			}

			// Check for forbidden lines
			for _, forbiddenLine := range test.ForbiddenLines {
				if strings.Contains(output, forbiddenLine) {
					t.Errorf("Output should not contain: %q\nGot: %s", forbiddenLine, output)
				}
			}
		})
	}
}

// TestProgramCompilation tests that programs compile without errors
func TestProgramCompilation(t *testing.T) {
	testFiles := []string{
		"../test/input/simple_showcase.ahoy",
		"../test/input/array_methods_test.ahoy",
		"../test/input/break_skip_test.ahoy",
		"../test/input/conditionals_test.ahoy",
		"../test/input/loops_test.ahoy",
	}

	for _, testFile := range testFiles {
		t.Run(filepath.Base(testFile), func(t *testing.T) {
			// Check if input file exists
			if _, err := os.Stat(testFile); os.IsNotExist(err) {
				t.Skipf("Input file not found: %s", testFile)
				return
			}

			// Just check that it compiles and runs without error
			_, err := compileAndRun(t, testFile)
			if err != nil {
				t.Errorf("Failed to compile and run: %v", err)
			}
		})
	}
}
