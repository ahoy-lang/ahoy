package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestConsolidatedFiles tests all consolidated test files
func TestConsolidatedFiles(t *testing.T) {
	// Build the compiler first
	compilerPath := "../ahoy-bin"
	if _, err := os.Stat(compilerPath); os.IsNotExist(err) {
		// Try to build it
		cmd := exec.Command("go", "build", "-o", compilerPath, "../source")
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to build compiler: %v", err)
		}
	}

	// Discover all .ahoy files in input directory
	files, err := filepath.Glob("input/*.ahoy")
	if err != nil {
		t.Fatalf("Failed to read input directory: %v", err)
	}

	for _, file := range files {
		// Extract test name from file
		baseName := filepath.Base(file)
		testName := strings.TrimSuffix(baseName, ".ahoy")
		testName = strings.ReplaceAll(testName, "_", " ")
		testName = strings.Title(testName) + " Test"

		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			output, err := compileAndRun(t, file, compilerPath)
			if err != nil {
				t.Fatalf("Failed to compile and run: %v", err)
			}

			// Extract the expected array from the last line of output
			lines := strings.Split(strings.TrimSpace(output), "\n")
			if len(lines) == 0 {
				t.Fatalf("No output generated")
			}

			// The last line should be the expected array
			expectedArrayLine := lines[len(lines)-1]

			// Parse the expected array - it should be in format ["val1", "val2", ...]
			if !strings.HasPrefix(expectedArrayLine, "[") || !strings.HasSuffix(expectedArrayLine, "]") {
				t.Fatalf("Last line is not an array format: %s", expectedArrayLine)
			}

			// Extract expected values from the array
			// Simple parsing: split by ", " after removing brackets and quotes
			arrayContent := strings.TrimPrefix(expectedArrayLine, "[")
			arrayContent = strings.TrimSuffix(arrayContent, "]")

			if arrayContent == "" {
				// Empty expected array - just verify we got some output
				if len(lines) < 1 {
					t.Fatalf("Expected some output but got none")
				}
				return
			}

			// Parse expected values
			expectedValues := parseArrayContent(arrayContent)

			// Get all output lines except the last one (which is the expected array)
			actualOutput := lines[:len(lines)-1]

			// Validate output count matches expected count
			if len(actualOutput) != len(expectedValues) {
				t.Errorf("Output count mismatch:")
				t.Errorf("  Expected %d output lines", len(expectedValues))
				t.Errorf("  Got %d output lines", len(actualOutput))
				if len(expectedValues) > 0 && len(expectedValues) <= 10 {
					t.Errorf("  Expected values: %v", expectedValues)
				}
				if len(actualOutput) > 0 && len(actualOutput) <= 10 {
					t.Errorf("  Actual output: %v", actualOutput)
				}
			}

			// Verify each expected value appears in the actual output
			for i, expected := range expectedValues {
				if i >= len(actualOutput) {
					t.Errorf("Expected value #%d '%s' but only got %d output lines", i+1, expected, len(actualOutput))
					continue
				}

				actual := strings.TrimSpace(actualOutput[i])

				// For exact match comparison
				if actual != expected {
					t.Errorf("Line %d: expected '%s', got '%s'", i+1, expected, actual)
				}
			}

			// Check we don't have extra output lines
			if len(actualOutput) > len(expectedValues) {
				t.Errorf("Got %d output lines but only expected %d", len(actualOutput), len(expectedValues))
				t.Errorf("Extra lines: %v", actualOutput[len(expectedValues):])
			}
		})
	}
}

// parseArrayContent parses the content of an array string (without brackets)
// Handles quoted strings and preserves escapes
func parseArrayContent(content string) []string {
	var values []string
	var current strings.Builder
	inQuotes := false
	escaped := false

	for i := 0; i < len(content); i++ {
		ch := content[i]

		if escaped {
			current.WriteByte(ch)
			escaped = false
			continue
		}

		if ch == '\\' {
			current.WriteByte(ch)
			escaped = true
			continue
		}

		if ch == '"' {
			inQuotes = !inQuotes
			current.WriteByte(ch)
			continue
		}

		if !inQuotes && ch == ',' {
			// End of value
			val := strings.TrimSpace(current.String())
			// Remove surrounding quotes if present
			val = strings.Trim(val, "\"")
			values = append(values, val)
			current.Reset()
			// Skip following space
			if i+1 < len(content) && content[i+1] == ' ' {
				i++
			}
			continue
		}

		current.WriteByte(ch)
	}

	// Add the last value
	if current.Len() > 0 {
		val := strings.TrimSpace(current.String())
		val = strings.Trim(val, "\"")
		values = append(values, val)
	}

	return values
}

// compileAndRun compiles an Ahoy file using the compiler and returns output
func compileAndRun(t *testing.T, ahoyFile string, compilerPath string) (string, error) {
	baseName := strings.TrimSuffix(filepath.Base(ahoyFile), ".ahoy")
	outputDir := "output"
	os.MkdirAll(outputDir, 0755)

	cFile := filepath.Join(outputDir, baseName+".c")
	executable := filepath.Join(outputDir, baseName)

	// Compile with ahoy-compiler
	cmd := exec.Command(compilerPath, "-f", ahoyFile)
	var compileErr bytes.Buffer
	cmd.Stderr = &compileErr
	cmd.Stdout = &compileErr
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("ahoy compilation failed: %v\n%s", err, compileErr.String())
	}

	// Compile C code
	cmd = exec.Command("gcc", "-o", executable, cFile, "-lm")
	compileErr.Reset()
	cmd.Stderr = &compileErr
	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("C compilation failed: %v\n%s", err, compileErr.String())
	}

	// Run executable
	cmd = exec.Command(executable)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("execution failed: %v\n%s", err, output.String())
	}

	return output.String(), nil
}
