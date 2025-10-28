// +build ignore

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// captureOutput compiles and runs an Ahoy file, returning the program output
func captureOutput(ahoyFile string) (string, error) {
	// Read the source file
	content, err := os.ReadFile(ahoyFile)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	// Format, tokenize, and parse
	formattedContent := formatSource(string(content))
	tokens := tokenize(formattedContent)
	ast := parse(tokens)

	// Generate C code
	cCode := generateC(ast)

	// Determine output paths
	baseName := strings.TrimSuffix(filepath.Base(ahoyFile), ".ahoy")
	outputDir := "test/output"
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

// generateTestCase generates Go test code for a given Ahoy file
func generateTestCase(ahoyFile string) {
	output, err := captureOutput(ahoyFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error capturing output for %s: %v\n", ahoyFile, err)
		return
	}

	baseName := strings.TrimSuffix(filepath.Base(ahoyFile), ".ahoy")
	testName := strings.ReplaceAll(strings.Title(strings.ReplaceAll(baseName, "_", " ")), " ", "")

	fmt.Printf(`		{
			Name:      "%s",
			InputFile: "../test/input/%s",
			ExpectedOutput: `+"`"+`%s`+"`"+`,
		},
`, testName, filepath.Base(ahoyFile), output)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run generate_tests.go <ahoy_file> [ahoy_file2 ...]")
		fmt.Println("\nExample:")
		fmt.Println("  go run source/generate_tests.go test/input/array_methods_test.ahoy")
		fmt.Println("\nThis will compile, run the file, and output Go test code you can paste into main_test.go")
		os.Exit(1)
	}

	fmt.Println("// Generated test cases - paste into TestProgramOutput in main_test.go")
	fmt.Println()

	for _, ahoyFile := range os.Args[1:] {
		generateTestCase(ahoyFile)
	}
}
