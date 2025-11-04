package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ahoy"
)

func main() {
	// Define CLI flags
	fileFlag := flag.String("f", "", "Input .ahoy source file")
	runFlag := flag.Bool("r", false, "Run the compiled C program after compilation")
	formatFlag := flag.Bool("format", false, "Format the source file")
	lintFlag := flag.Bool("lint", false, "Run linter to check for errors without compiling")
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
	tokens := ahoy.Tokenize(formattedContent)

	// Lint mode
	if *lintFlag {
		// First check syntax errors
		_, errors := ahoy.ParseLint(tokens)
		if len(errors) > 0 {
			fmt.Printf("Found %d syntax error(s) in %s:\n", len(errors), sourceFile)
			for _, err := range errors {
				fmt.Printf("  Line %d, Column %d: %s\n", err.Line, err.Column, err.Message)
			}
			os.Exit(1)
		}

		// Try to use LSP for comprehensive validation if available
		lspPath, err := exec.LookPath("ahoy-lsp")
		if err == nil {
			// LSP is available, use it for comprehensive linting
			cmd := exec.Command(lspPath, "--validate", sourceFile)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err := cmd.Run()
			if err != nil {
				// LSP found errors
				os.Exit(1)
			}
		} else {
			// LSP not available, only syntax checking done
			fmt.Printf("✓ No syntax errors found in %s\n", sourceFile)
			fmt.Printf("  (Install ahoy-lsp to PATH for comprehensive validation)\n")
		}
		return
	}

	// Get absolute path for source file
	absPath, err := filepath.Abs(sourceFile)
	if err != nil {
		fmt.Printf("Error resolving file path: %v\n", err)
		os.Exit(1)
	}

	// Initialize package manager
	pm := NewPackageManager(filepath.Dir(absPath))
	
	// Load the package
	pkg, err := pm.LoadPackageFromFile(absPath)
	if err != nil {
		fmt.Printf("Error loading package: %v\n", err)
		os.Exit(1)
	}

	// Resolve imports recursively
	imports, err := resolveImports(pkg, pm, absPath)
	if err != nil {
		fmt.Printf("Error resolving imports: %v\n", err)
		os.Exit(1)
	}

	// Merge package with all imports into one AST
	ast := MergeWithImports(pkg, imports)

	// Generate C code
	cCode := generateC(ast)
	
	// Check if code generation failed
	if cCode == "" {
		fmt.Println("✗ Code generation failed due to errors")
		os.Exit(1)
	}

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

	if len(pkg.Files) > 1 {
		fmt.Printf("✓ Compiled package '%s' (%d files) to %s\n", pkg.Name, len(pkg.Files), outputFile)
	} else {
		fmt.Printf("✓ Compiled %s to %s\n", sourceFile, outputFile)
	}

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

// resolveImports recursively resolves all imports in a package
// and merges them into a unified set of imports
func resolveImports(pkg *Package, pm *PackageManager, fromFile string) (map[string]*Package, error) {
	allImports := make(map[string]*Package)
	
	for _, file := range pkg.Files {
		if file.AST != nil {
			for _, child := range file.AST.Children {
				if child.Type == ahoy.NODE_IMPORT_STATEMENT {
					importPath := child.Value
					importedPkg, err := pm.ResolveImport(importPath, fromFile)
					if err != nil {
						return nil, fmt.Errorf("failed to resolve import '%s': %v", importPath, err)
					}
					
					// Store with namespace key
					namespace := child.DataType
					if namespace == "" {
						namespace = importedPkg.Name
					}
					allImports[namespace] = importedPkg
					
					// Recursively resolve imports in the imported package
					nestedImports, err := resolveImports(importedPkg, pm, file.Path)
					if err != nil {
						return nil, err
					}
					
					// Merge nested imports
					for ns, nestedPkg := range nestedImports {
						if _, exists := allImports[ns]; !exists {
							allImports[ns] = nestedPkg
						}
					}
				}
			}
		}
	}
	return allImports, nil
}

// MergeWithImports merges the package with all imported packages into a single AST
func MergeWithImports(pkg *Package, imports map[string]*Package) *ahoy.ASTNode {
	merged := &ahoy.ASTNode{Type: ahoy.NODE_PROGRAM}
	processedFunctions := make(map[string]bool) // Deduplicate functions
	processedStructs := make(map[string]bool)   // Deduplicate structs
	processedEnums := make(map[string]bool)     // Deduplicate enums
	
	// First, add all declarations from imported packages
	for _, importedPkg := range imports {
		for _, file := range importedPkg.Files {
			if file.AST != nil {
				for _, child := range file.AST.Children {
					// Skip program declarations and imports
					if child.Type == ahoy.NODE_PROGRAM_DECLARATION || child.Type == ahoy.NODE_IMPORT_STATEMENT {
						continue
					}
					
					// Deduplicate by name
					name := child.Value
					shouldAdd := false
					
					switch child.Type {
					case ahoy.NODE_FUNCTION:
						if !processedFunctions[name] {
							processedFunctions[name] = true
							shouldAdd = true
						}
					case ahoy.NODE_STRUCT_DECLARATION:
						if !processedStructs[name] {
							processedStructs[name] = true
							shouldAdd = true
						}
					case ahoy.NODE_ENUM_DECLARATION:
						if !processedEnums[name] {
							processedEnums[name] = true
							shouldAdd = true
						}
					default:
						shouldAdd = true
					}
					
					if shouldAdd {
						merged.Children = append(merged.Children, child)
					}
				}
			}
		}
	}
	
	// Then add declarations from the main package
	for _, file := range pkg.Files {
		if file.AST != nil {
			for _, child := range file.AST.Children {
				// Skip program declarations
				if child.Type == ahoy.NODE_PROGRAM_DECLARATION {
					continue
				}
				
				// Skip imports (we've already processed them)
				if child.Type == ahoy.NODE_IMPORT_STATEMENT {
					continue
				}
				
				// Deduplicate by name
				name := child.Value
				shouldAdd := false
				
				switch child.Type {
				case ahoy.NODE_FUNCTION:
					if !processedFunctions[name] {
						processedFunctions[name] = true
						shouldAdd = true
					}
				case ahoy.NODE_STRUCT_DECLARATION:
					if !processedStructs[name] {
						processedStructs[name] = true
						shouldAdd = true
					}
				case ahoy.NODE_ENUM_DECLARATION:
					if !processedEnums[name] {
						processedEnums[name] = true
						shouldAdd = true
					}
				default:
					shouldAdd = true
				}
				
				if shouldAdd {
					merged.Children = append(merged.Children, child)
				}
			}
		}
	}
	
	return merged
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
	fmt.Println("  -lint         Check for syntax errors without compiling")
	fmt.Println("  -h            Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  go run main.go -f input/main.ahoy")
	fmt.Println("  go run main.go -f input/main.ahoy -r")
	fmt.Println("  go run main.go -f input/main.ahoy -format")
	fmt.Println("  go run main.go -f input/main.ahoy -lint")
}
