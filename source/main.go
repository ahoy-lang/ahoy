package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	// Define CLI flags
	fileFlag := flag.String("f", "", "Input .ahoy source file")
	runFlag := flag.Bool("r", false, "Run the compiled C program after compilation")
	formatFlag := flag.Bool("format", false, "Format the source file")
	helpFlag := flag.Bool("h", false, "Show help")

	flag.Parse()

	if *helpFlag || (*fileFlag == "" && !*formatFlag) {
		showHelp()
		return
	}

	sourceFile := *fileFlag

	// Check if file exists
	if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
		fmt.Printf("Error: File '%s' not found\n", sourceFile)
		os.Exit(1)
	}

	// Read source file
	content, err := os.ReadFile(sourceFile)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Format if requested
	if *formatFlag {
		formatted := formatSource(string(content))
		err = os.WriteFile(sourceFile, []byte(formatted), 0644)
		if err != nil {
			fmt.Printf("Error writing formatted file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Formatted %s\n", sourceFile)
		return
	}

	// Format source before compiling (tabs to spaces, etc)
	formattedContent := formatSource(string(content))

	// Tokenize
	tokens := tokenize(formattedContent)

	// Parse
	ast := parse(tokens)

	// Generate C code
	cCode := generateC(ast)

	// Determine output file name
	baseName := filepath.Base(sourceFile)
	baseName = strings.TrimSuffix(baseName, filepath.Ext(baseName))
	
	// Determine output directory based on source file location
	outputDir := "output"
	sourceDir := filepath.Dir(sourceFile)
	if strings.Contains(sourceDir, "test/input") || strings.Contains(sourceDir, "test\\input") {
		// If source is in test/input, output to test/output
		outputDir = filepath.Join(filepath.Dir(filepath.Dir(sourceDir)), "test", "output")
	}
	
	outputFile := filepath.Join(outputDir, baseName+".c")
	executable := filepath.Join(outputDir, baseName)

	// Create output directory if it doesn't exist
	os.MkdirAll(outputDir, 0755)

	// Write C file
	err = os.WriteFile(outputFile, []byte(cCode), 0644)
	if err != nil {
		fmt.Printf("Error writing C file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Compiled %s to %s\n", sourceFile, outputFile)

	// Compile C code if run flag is set
	if *runFlag {
		fmt.Println("Compiling C code...")
		cmd := exec.Command("gcc", "-o", executable, outputFile, "-lm")
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Error compiling C code:\n%s\n", output)
			os.Exit(1)
		}

		fmt.Printf("✓ Compiled C code to %s\n", executable)
		fmt.Println("Running program:")
		fmt.Println("==================")

		// Run the executable
		runCmd := exec.Command(executable)
		runCmd.Stdout = os.Stdout
		runCmd.Stderr = os.Stderr
		err = runCmd.Run()
		fmt.Println("==================")
		if err != nil {
			fmt.Printf("Program exited with error: %v\n", err)
			os.Exit(1)
		}
	}
}

func showHelp() {
	fmt.Println("Ahoy Language Compiler")
	fmt.Println("======================")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  go run main.go -f <file.ahoy> [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -f <file>     Input .ahoy source file (required)")
	fmt.Println("  -r            Run the compiled C program")
	fmt.Println("  -format       Format the source file")
	fmt.Println("  -h            Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  go run main.go -f input/main.ahoy")
	fmt.Println("  go run main.go -f input/main.ahoy -r")
	fmt.Println("  go run main.go -f input/main.ahoy -format")
}
