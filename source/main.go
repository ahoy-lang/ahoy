package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"ahoy"
)

// findCompiler finds the appropriate C compiler based on mode and OS
// Returns: compiler path, args, isOptimized, error
func findCompiler(releaseMode bool, executablePath string) (string, []string, bool, error) {
	// Try embedded TCC first
	tccPath, tccArgs, err := GetEmbeddedTCCPath()
	if err != nil {
		// Fall back to looking for TCC next to the binary
		ahoyDir, err := os.Executable()
		if err != nil {
			ahoyDir = "."
		} else {
			ahoyDir = filepath.Dir(ahoyDir)
		}

		switch runtime.GOOS {
		case "linux":
			tccPath = filepath.Join(ahoyDir, "tcc", "linux", "tcc")
			tccArgs = []string{"-B" + filepath.Join(ahoyDir, "tcc", "linux")}
		case "windows":
			if runtime.GOARCH == "386" {
				tccPath = filepath.Join(ahoyDir, "tcc", "windows", "i386-win32-tcc.exe")
			} else {
				tccPath = filepath.Join(ahoyDir, "tcc", "windows", "tcc.exe")
			}
			tccArgs = []string{"-B" + filepath.Join(ahoyDir, "tcc", "windows")}
		default:
			tccPath = ""
		}
	}

	if releaseMode {
		// Try to find gcc or clang for optimized builds
		gccPath, err := exec.LookPath("gcc")
		if err == nil {
			return gccPath, []string{"-O3"}, true, nil
		}

		clangPath, err := exec.LookPath("clang")
		if err == nil {
			return clangPath, []string{"-O3"}, true, nil
		}

		// On Windows, also try MSVC (cl.exe)
		if runtime.GOOS == "windows" {
			clPath, err := exec.LookPath("cl")
			if err == nil {
				return clPath, []string{"/O2"}, true, nil
			}
		}

		// Fall back to TCC with warning
		if tccPath != "" {
			if _, err := os.Stat(tccPath); err == nil {
				fmt.Println("⚠ Warning: gcc/clang not found, using TCC (code may not be optimized)")
				fmt.Println("  Install gcc or clang for optimized release builds")
				return tccPath, tccArgs, false, nil
			}
		}

		return "", nil, false, fmt.Errorf("no C compiler found (tried gcc, clang, tcc)")
	}

	// Debug mode - prefer TCC for fast compilation
	if tccPath != "" {
		if _, err := os.Stat(tccPath); err == nil {
			return tccPath, tccArgs, false, nil
		}
	}

	// TCC not available, try system compilers
	gccPath, err := exec.LookPath("gcc")
	if err == nil {
		return gccPath, []string{"-O0"}, false, nil
	}

	clangPath, err := exec.LookPath("clang")
	if err == nil {
		return clangPath, []string{"-O0"}, false, nil
	}

	// On Windows, also try MSVC (cl.exe)
	if runtime.GOOS == "windows" {
		clPath, err := exec.LookPath("cl")
		if err == nil {
			return clPath, []string{"/Od"}, false, nil
		}
	}

	// Provide helpful error message based on OS
	switch runtime.GOOS {
	case "linux", "windows":
		return "", nil, false, fmt.Errorf("TCC not found at %s and no system compiler (gcc/clang) available", tccPath)
	default:
		return "", nil, false, fmt.Errorf("unsupported OS '%s' - no bundled TCC available and no system compiler found\nInstall gcc or clang for your platform", runtime.GOOS)
	}
}

// CrossCompileTarget represents a target platform for cross-compilation
type CrossCompileTarget struct {
	Name      string
	OutputDir string
	Compiler  string
	Args      []string
	Extension string
}

// findCrossCompiler finds the compiler for cross-compilation
func findCrossCompiler(target string, ahoyDir string) (*CrossCompileTarget, error) {
	homeDir, _ := os.UserHomeDir()

	switch target {
	case "linux":
		if runtime.GOOS == "linux" {
			// Native compilation
			compiler, args, _, err := findCompiler(true, "")
			if err != nil {
				return nil, err
			}
			return &CrossCompileTarget{
				Name:      "linux",
				OutputDir: "build/linux",
				Compiler:  compiler,
				Args:      args,
				Extension: "",
			}, nil
		}
		// Cross-compile using zig cc
		zigPath, err := exec.LookPath("zig")
		if err != nil {
			return nil, fmt.Errorf("cross-compilation to Linux requires zig (zig cc)\nInstall zig: https://ziglang.org/download/")
		}
		return &CrossCompileTarget{
			Name:      "linux",
			OutputDir: "build/linux",
			Compiler:  zigPath,
			Args:      []string{"cc", "-target", "x86_64-linux-gnu", "-O3", "-Wno-int-conversion", "-Wno-format"},
			Extension: "",
		}, nil

	case "windows":
		if runtime.GOOS == "windows" {
			// Native compilation
			compiler, args, _, err := findCompiler(true, "")
			if err != nil {
				return nil, err
			}
			return &CrossCompileTarget{
				Name:      "windows",
				OutputDir: "build/windows",
				Compiler:  compiler,
				Args:      args,
				Extension: ".exe",
			}, nil
		}
		// Cross-compile using zig cc
		zigPath, err := exec.LookPath("zig")
		if err != nil {
			return nil, fmt.Errorf("cross-compilation to Windows requires zig (zig cc)\nInstall zig: https://ziglang.org/download/")
		}
		return &CrossCompileTarget{
			Name:      "windows",
			OutputDir: "build/windows",
			Compiler:  zigPath,
			Args:      []string{"cc", "-target", "x86_64-windows-gnu", "-O3", "-Wno-int-conversion", "-Wno-format"},
			Extension: ".exe",
		}, nil

	case "macos":
		if runtime.GOOS == "darwin" {
			// Native compilation
			compiler, args, _, err := findCompiler(true, "")
			if err != nil {
				return nil, err
			}
			return &CrossCompileTarget{
				Name:      "macos",
				OutputDir: "build/macos",
				Compiler:  compiler,
				Args:      args,
				Extension: "",
			}, nil
		}
		// Cross-compile using zig cc
		zigPath, err := exec.LookPath("zig")
		if err != nil {
			return nil, fmt.Errorf("cross-compilation to macOS requires zig (zig cc)\nInstall zig: https://ziglang.org/download/")
		}
		return &CrossCompileTarget{
			Name:      "macos",
			OutputDir: "build/macos",
			Compiler:  zigPath,
			Args:      []string{"cc", "-target", "x86_64-macos", "-O3", "-Wno-int-conversion", "-Wno-format"},
			Extension: "",
		}, nil

	case "web":
		// Check for emscripten
		emccPath := filepath.Join(homeDir, "Documents", "emsdk", "upstream", "emscripten", "emcc")
		if _, err := os.Stat(emccPath); os.IsNotExist(err) {
			// Try looking in PATH
			var pathErr error
			emccPath, pathErr = exec.LookPath("emcc")
			if pathErr != nil {
				return nil, fmt.Errorf("web compilation requires Emscripten (emcc)\nExpected at: %s/Documents/emsdk\nOr install and add emcc to PATH", homeDir)
			}
		}
		return &CrossCompileTarget{
			Name:      "web",
			OutputDir: "build/web",
			Compiler:  emccPath,
			Args:      []string{"-O3", "-s", "WASM=1", "-Wno-int-conversion", "-Wno-format"},
			Extension: ".html",
		}, nil

	default:
		return nil, fmt.Errorf("unknown target: %s\nValid targets: linux, windows, macos, web, all", target)
	}
}

// copyAssets copies the assets folder to the build directory
func copyAssets(sourceDir, destDir string) error {
	assetsDir := filepath.Join(sourceDir, "assets")
	if _, err := os.Stat(assetsDir); os.IsNotExist(err) {
		// No assets folder, skip
		return nil
	}

	destAssets := filepath.Join(destDir, "assets")

	// Remove existing assets in dest
	os.RemoveAll(destAssets)

	// Copy assets directory recursively
	return filepath.Walk(assetsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(assetsDir, path)
		destPath := filepath.Join(destAssets, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		// Copy file
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destPath, data, 0644)
	})
}

// getRaylibFlags returns the appropriate raylib linker flags for each target platform
func getRaylibFlags(target *CrossCompileTarget, raylibPath string, sourceDir string) ([]string, error) {
	var flags []string

	// Determine if this is a native or cross build
	isNativeBuild := (target.Name == "linux" && runtime.GOOS == "linux") ||
		(target.Name == "windows" && runtime.GOOS == "windows") ||
		(target.Name == "macos" && runtime.GOOS == "darwin")

	// Check for platform-specific raylib library
	var expectedLib string
	var libSearchPaths []string

	switch target.Name {
	case "linux":
		expectedLib = "libraylib.a"
		if isNativeBuild {
			// Native build can use the provided raylibPath
			libSearchPaths = []string{
				filepath.Join(raylibPath, "libraylib.a"),
				filepath.Join(sourceDir, "libs", "linux", "libraylib.a"),
			}
		} else {
			// Cross-compile: only look in platform-specific directory
			libSearchPaths = []string{
				filepath.Join(sourceDir, "libs", "linux", "libraylib.a"),
			}
		}
	case "windows":
		expectedLib = "raylib.lib or libraylib.a (Windows build)"
		if isNativeBuild {
			libSearchPaths = []string{
				filepath.Join(raylibPath, "raylib.lib"),
				filepath.Join(raylibPath, "libraylib.a"),
				filepath.Join(sourceDir, "libs", "windows", "raylib.lib"),
				filepath.Join(sourceDir, "libs", "windows", "libraylib.a"),
			}
		} else {
			// Cross-compile: only look in platform-specific directory
			libSearchPaths = []string{
				filepath.Join(sourceDir, "libs", "windows", "raylib.lib"),
				filepath.Join(sourceDir, "libs", "windows", "libraylib.a"),
			}
		}
	case "macos":
		expectedLib = "libraylib.a (macOS build)"
		if isNativeBuild {
			libSearchPaths = []string{
				filepath.Join(raylibPath, "libraylib.a"),
				filepath.Join(sourceDir, "libs", "macos", "libraylib.a"),
			}
		} else {
			// Cross-compile: only look in platform-specific directory
			libSearchPaths = []string{
				filepath.Join(sourceDir, "libs", "macos", "libraylib.a"),
			}
		}
	case "web":
		expectedLib = "libraylib.a (web/emscripten build)"
		libSearchPaths = []string{
			filepath.Join(sourceDir, "libs", "web", "libraylib.a"),
		}
	}

	// Find the raylib library for this platform
	var foundLibPath string
	for _, libPath := range libSearchPaths {
		if _, err := os.Stat(libPath); err == nil {
			foundLibPath = filepath.Dir(libPath)
			break
		}
	}

	// For non-native builds, require platform-specific lib
	if !isNativeBuild && foundLibPath == "" {
		var buildInstructions string
		if target.Name == "web" {
			buildInstructions = fmt.Sprintf(
				"  To build raylib for web using emscripten:\n"+
					"    cd %s\n"+
					"    source ~/Documents/emsdk/emsdk_env.sh\n"+
					"    mkdir build_web && cd build_web\n"+
					"    emcmake cmake .. -DPLATFORM=Web -DCMAKE_BUILD_TYPE=Release\n"+
					"    emmake make\n"+
					"    mkdir -p %s/libs/web\n"+
					"    cp libraylib.a %s/libs/web/",
				filepath.Dir(raylibPath), sourceDir, sourceDir)
		} else {
			buildInstructions = fmt.Sprintf(
				"  To build raylib for %s using zig:\n"+
					"    cd %s && zig build -Dtarget=%s -Doptimize=ReleaseFast\n"+
					"    mkdir -p %s/libs/%s\n"+
					"    cp zig-out/lib/libraylib.a %s/libs/%s/",
				target.Name, filepath.Dir(raylibPath), getZigTarget(target.Name),
				sourceDir, target.Name,
				sourceDir, target.Name)
		}

		return nil, fmt.Errorf("cross-compilation with raylib requires platform-specific library\n"+
			"  Expected: %s\n"+
			"  Searched: %v\n"+
			"  Solution: Build raylib for %s and place in libs/%s/\n"+
			"  \n%s",
			expectedLib, libSearchPaths, target.Name, target.Name, buildInstructions)
	}

	// Use found path or default raylibPath
	effectivePath := raylibPath
	if foundLibPath != "" {
		effectivePath = foundLibPath
	}

	if effectivePath != "" {
		flags = append(flags, "-L"+effectivePath)
		flags = append(flags, "-I"+effectivePath)
	}

	switch target.Name {
	case "linux":
		// Linux uses X11 and standard Unix libraries
		flags = append(flags, "-lraylib", "-lGL", "-lm", "-lpthread", "-ldl", "-lrt", "-lX11")

	case "windows":
		// Windows uses different system libraries
		flags = append(flags, "-lraylib", "-lopengl32", "-lgdi32", "-lwinmm", "-lm")

	case "macos":
		// macOS uses frameworks instead of libraries
		flags = append(flags, "-lraylib")
		flags = append(flags, "-framework", "IOKit")
		flags = append(flags, "-framework", "Cocoa")
		flags = append(flags, "-framework", "OpenGL")
		flags = append(flags, "-lm")

	case "web":
		// Web/Emscripten has special requirements
		flags = append(flags,
			"-s", "USE_GLFW=3",
			"-s", "ASYNCIFY",
			"-s", "TOTAL_MEMORY=67108864",
			"-s", "FORCE_FILESYSTEM=1",
		)
		// Add preload for assets if they exist
		assetsDir := filepath.Join(sourceDir, "assets")
		if _, err := os.Stat(assetsDir); err == nil {
			flags = append(flags, "--preload-file", assetsDir+"@/assets")
		}
		if effectivePath != "" {
			flags = append(flags, filepath.Join(effectivePath, "libraylib.a"))
		}
	}

	return flags, nil
}

// getZigTarget returns the zig target triple for a platform
func getZigTarget(platform string) string {
	switch platform {
	case "linux":
		return "x86_64-linux-gnu"
	case "windows":
		return "x86_64-windows-gnu"
	case "macos":
		return "x86_64-macos"
	default:
		return platform
	}
}

// buildForTarget compiles for a specific target
func buildForTarget(target *CrossCompileTarget, cFile, baseName, sourceDir string, hasRaylib bool, raylibPath string) error {
	// Create output directory
	if err := os.MkdirAll(target.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	executable := filepath.Join(target.OutputDir, baseName+target.Extension)

	// Build compile arguments
	args := append(target.Args, "-o", executable, cFile)

	// Add library flags
	if hasRaylib {
		raylibFlags, err := getRaylibFlags(target, raylibPath, sourceDir)
		if err != nil {
			return err
		}
		args = append(args, raylibFlags...)
	} else {
		args = append(args, "-lm")
	}

	// Run compiler
	cmd := exec.Command(target.Compiler, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compilation failed for %s:\n%s", target.Name, output)
	}

	// Copy assets
	if err := copyAssets(sourceDir, target.OutputDir); err != nil {
		fmt.Printf("⚠ Warning: Failed to copy assets to %s: %v\n", target.OutputDir, err)
	}

	fmt.Printf("✓ Built %s: %s\n", target.Name, executable)
	return nil
}

func main() {
	// Define CLI flags
	fileFlag := flag.String("f", "", "Input .ahoy source file")
	runFlag := flag.Bool("r", false, "Run the compiled C program after compilation")
	formatFlag := flag.Bool("format", false, "Format the source file")
	lintFlag := flag.Bool("lint", false, "Run linter to check for errors without compiling")
	releaseFlag := flag.Bool("release", false, "Use optimizing compiler (gcc/clang) for release build")
	targetFlag := flag.String("target", "", "Cross-compile target: linux, windows, web, macos, or all")
	incFlag := flag.Bool("cache", false, "Enable incremental builds (cache parsed files)")
	profCompFlag := flag.Bool("profile_compiler", false, "Enable CPU profiling of compiler (creates pprof file)")
	genStdlibFlag := flag.Bool("gen_stdlib_docs", false, "Generate stdlib documentation as .ahoy file")
	helpFlag := flag.Bool("h", false, "Show help")

	flag.Parse()

	// Start CPU profiling if requested
	if *profCompFlag {
		// Get current working directory name for profile filename
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Printf("Error getting current directory: %v\n", err)
			os.Exit(1)
		}
		dirName := filepath.Base(cwd)
		profileName := fmt.Sprintf("%s_cpu.pprof", dirName)

		f, err := os.Create(profileName)
		if err != nil {
			fmt.Printf("Error creating profile file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()

		if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Printf("Error starting CPU profile: %v\n", err)
			os.Exit(1)
		}
		defer pprof.StopCPUProfile()

		fmt.Printf("CPU profiling enabled, writing to %s\n", profileName)
	}

	// Start total timing
	totalStart := time.Now()

	// Initialize incremental build cache
	cacheStart := time.Now()
	InitBuildCache(*incFlag)
	defer GetBuildCache().SaveAndClose()
	cacheTime := time.Since(cacheStart)

	// Generate stdlib documentation if requested
	if *genStdlibFlag {
		output := GenerateStdlibAhoyFile()

		// Get cache directory
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				fmt.Printf("Error getting home directory: %v\n", err)
				os.Exit(1)
			}
			cacheDir = filepath.Join(homeDir, ".cache")
		}

		// Create ahoy cache directory
		ahoyCacheDir := filepath.Join(cacheDir, "ahoy")
		if err := os.MkdirAll(ahoyCacheDir, 0755); err != nil {
			fmt.Printf("Error creating cache directory: %v\n", err)
			os.Exit(1)
		}

		// Write to cache directory
		filename := filepath.Join(ahoyCacheDir, "ahoy_stdlib.ahoy")
		err = os.WriteFile(filename, []byte(output), 0644)
		if err != nil {
			fmt.Printf("Error writing stdlib documentation: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Generated stdlib documentation: %s\n", filename)
		return
	}

	// tokenize
	tokenStart := time.Now()
	// Ensure stdlib is in cache (for LSP goto definition support)
	_ = EnsureStdlibExists()

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
	// formattedContent := string(content)

	// Tokenize
	tokens := ahoy.Tokenize(string(content))
	tokenTime := time.Since(tokenStart)

	// Lint mode
	if *lintFlag {
		// Parse the code to check for C imports
		ast, errors := ahoy.ParseLintWithPath(tokens, sourceFile)

		// Check syntax errors
		if len(errors) > 0 {
			fmt.Printf("Found %d syntax error(s) in %s:\n", len(errors), sourceFile)
			for _, err := range errors {
				fmt.Printf("  Line %d, Column %d: %s\n", err.Line, err.Column, err.Message)
			}
			os.Exit(1)
		}

		// Check if this is a multi-file program and validate for duplicates
		programName := ""
		hasCImports := false
		if ast != nil {
			for _, child := range ast.Children {
				if child.Type == ahoy.NODE_PROGRAM_DECLARATION {
					programName = child.Value
				}
				if child.Type == ahoy.NODE_IMPORT_STATEMENT && strings.HasSuffix(child.Value, ".h") {
					hasCImports = true
				}
			}
		}

		// If this is a multi-file program, check for duplicate functions across files
		if programName != "" {
			absPath, _ := filepath.Abs(sourceFile)
			pm := NewPackageManager(filepath.Dir(absPath))
			pkg, err := pm.LoadPackageFromFile(absPath)
			if err == nil && len(pkg.Files) > 1 {
				// Track function definitions across files
				functionDefs := make(map[string]struct {
					file string
					line int
				})
				duplicateErrors := []string{}

				for _, file := range pkg.Files {
					if file.AST == nil {
						continue
					}
					for _, child := range file.AST.Children {
						if child.Type == ahoy.NODE_FUNCTION {
							funcName := child.Value
							if existing, exists := functionDefs[funcName]; exists {
								duplicateErrors = append(duplicateErrors,
									fmt.Sprintf("Function '%s' is already declared in %s (line %d); Ahoy doesn't support function overloading",
										funcName, filepath.Base(existing.file), existing.line))
							} else {
								functionDefs[funcName] = struct {
									file string
									line int
								}{file: file.Path, line: child.Line}
							}
						}
					}
				}

				if len(duplicateErrors) > 0 {
					fmt.Printf("Found %d error(s) in package '%s' (%d files):\n", len(duplicateErrors), programName, len(pkg.Files))
					for _, errMsg := range duplicateErrors {
						fmt.Printf("  %s\n", errMsg)
					}
					os.Exit(1)
				}
			}
		}

		// Try to use LSP for comprehensive validation if available
		_, err := exec.LookPath("ahoy-lsp")
		if err == nil && !hasCImports {
			// LSP is available and no C imports, use it for comprehensive linting
			// Note: LSP --validate mode not implemented yet
			fmt.Printf("✓ No syntax errors found in %s\n", sourceFile)
		} else if hasCImports {
			// Has C imports - basic validation only (C functions can't be validated without full header parsing)
			fmt.Printf("✓ No syntax errors found in %s\n", sourceFile)
			fmt.Printf("  Note: File uses C imports. Use LSP in your editor for full validation.\n")
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

	// Quick pre-parse to get program name for cache loading
	if *incFlag {
		if programName := getQuickProgramName(absPath); programName != "" {
			GetBuildCache().SetProgramName(programName)
		}
	}

	// Start load/parse timing
	loadStart := time.Now()

	// Load the package (tokenization + parsing)
	pkg, err := pm.LoadPackageFromFile(absPath)
	if err != nil {
		fmt.Printf("Error loading package: %v\n", err)
		os.Exit(1)
	}

	// Set the program name for the cache if not already set
	if GetBuildCache().ProgramName == "" {
		GetBuildCache().SetProgramName(pkg.Name)
	}

	loadTime := time.Since(loadStart)

	// Start imports timing
	importsStart := time.Now()

	// Resolve imports recursively
	imports, err := resolveImports(pkg, pm, absPath)
	if err != nil {
		fmt.Printf("Error resolving imports: %v\n", err)
		os.Exit(1)
	}

	importsTime := time.Since(importsStart)

	// Start merge timing
	mergeStart := time.Now()

	// Merge package with all imports into one AST
	ast := MergeWithImports(pkg, imports)

	mergeTime := time.Since(mergeStart)

	// Start codegen timing
	codegenStart := time.Now()

	// Generate C code with source filename for better error messages
	cCode := generateC(ast, sourceFile)

	codegenTime := time.Since(codegenStart)

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

	// Format time as milliseconds with 1 decimal
	formatTime := func(d time.Duration) string {
		ms := float64(d.Nanoseconds()) / 1e6
		if ms < 1 {
			return fmt.Sprintf("%.2fms", ms)
		}
		return fmt.Sprintf("%.1fms", ms)
	}

	// Calculate total parse time (load + imports + merge)
	parseTime := loadTime + importsTime + mergeTime
	if len(pkg.Files) > 1 {
		fmt.Printf("✓ Compiled package '%s' (%d files) to %s\n", pkg.Name, len(pkg.Files), outputFile)
		fmt.Printf(" Time: token=%s, parse=%s, codegen=%s , cache=%s \n",
			formatTime(tokenTime), formatTime(parseTime), formatTime(codegenTime), formatTime(cacheTime))
	} else {
		fmt.Printf(" Time: token=%s, parse=%s, codegen=%s , cache=%s \n",
			formatTime(tokenTime), formatTime(parseTime), formatTime(codegenTime), formatTime(cacheTime))
	}

	// Compile C code if run flag is set
	if *runFlag {
		fmt.Println("Compiling C code...")

		// Start C compilation timing
		cCompileStart := time.Now()

		// Find the appropriate C compiler
		compiler, baseArgs, isOptimized, err := findCompiler(*releaseFlag, executable)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		// Build compilation arguments
		compileArgs := append(baseArgs, "-o", executable, outputFile)

		// Check if raylib is imported
		hasRaylib := false
		raylibPath := ""
		for _, file := range pkg.Files {
			if file.AST != nil {
				for _, child := range file.AST.Children {
					if child.Type == ahoy.NODE_IMPORT_STATEMENT && strings.Contains(child.Value, "raylib.h") {
						hasRaylib = true
						raylibPath = filepath.Dir(child.Value)
						break
					}
				}
			}
			if hasRaylib {
				break
			}
		}

		// Add raylib linking flags if needed
		if hasRaylib {
			if raylibPath != "" {
				compileArgs = append(compileArgs, "-L"+raylibPath)
			}
			compileArgs = append(compileArgs, "-lraylib", "-lm", "-lpthread", "-ldl", "-lrt", "-lX11")
		} else {
			compileArgs = append(compileArgs, "-lm")
		}

		cmd := exec.Command(compiler, compileArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Error compiling C code:\n%s\n", output)
			os.Exit(1)
		}

		cCompileTime := time.Since(cCompileStart)
		totalTime := time.Since(totalStart)

		// Show compiler info
		compilerName := filepath.Base(compiler)
		modeStr := "debug"
		if isOptimized {
			modeStr = "release"
		}
		fmt.Printf("✓ Compiled C code to %s (%s %s: %s, total: %s)\n",
			executable, compilerName, modeStr, formatTime(cCompileTime), formatTime(totalTime))
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

	// Cross-compilation if target flag is set
	if *targetFlag != "" {
		ahoyDir, _ := os.Executable()
		if ahoyDir != "" {
			ahoyDir = filepath.Dir(ahoyDir)
		} else {
			ahoyDir = "."
		}

		// Check raylib for cross-compilation
		hasRaylib := false
		raylibPath := ""
		for _, file := range pkg.Files {
			if file.AST != nil {
				for _, child := range file.AST.Children {
					if child.Type == ahoy.NODE_IMPORT_STATEMENT && strings.Contains(child.Value, "raylib.h") {
						hasRaylib = true
						raylibPath = filepath.Dir(child.Value)
						break
					}
				}
			}
			if hasRaylib {
				break
			}
		}

		var targets []string
		if *targetFlag == "all" {
			targets = []string{"linux", "windows", "web"}
		} else {
			targets = []string{*targetFlag}
		}

		fmt.Println("\nBuilding release builds...")
		sourceDir := filepath.Dir(sourceFile)

		for _, targetName := range targets {
			target, err := findCrossCompiler(targetName, ahoyDir)
			if err != nil {
				fmt.Printf("✗ %s: %v\n", targetName, err)
				continue
			}

			if err := buildForTarget(target, outputFile, baseName, sourceDir, hasRaylib, raylibPath); err != nil {
				fmt.Printf("✗ %s: %v\n", targetName, err)
			}
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
					if child.Type == ahoy.NODE_PROGRAM_DECLARATION {
						continue
					}

					// Keep C header imports (.h files), skip .ahoy imports
					if child.Type == ahoy.NODE_IMPORT_STATEMENT {
						if strings.HasSuffix(child.Value, ".h") {
							// Keep C header imports for codegen
							merged.Children = append(merged.Children, child)
						}
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

				// Keep C header imports (.h files), skip .ahoy imports
				if child.Type == ahoy.NODE_IMPORT_STATEMENT {
					if strings.HasSuffix(child.Value, ".h") {
						// Keep C header imports for codegen
						merged.Children = append(merged.Children, child)
					}
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

// getQuickProgramName does a quick scan of a file to extract the program name
// without full parsing. This is used to load the correct cache before parsing.
func getQuickProgramName(filePath string) string {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}

	// Quick scan for "program " at the start of a line
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip comments
		if strings.HasPrefix(line, "?") {
			continue
		}
		// Check for program declaration
		if strings.HasPrefix(line, "program ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
		// Stop after first non-comment, non-empty line if it's not a program declaration
		if line != "" && !strings.HasPrefix(line, "program ") {
			break
		}
	}
	return ""
}

func showHelp() {
	fmt.Println("Ahoy Language Compiler")
	fmt.Println("======================")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  ahoy -f <file.ahoy> [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -f <file>     Input .ahoy source file (required)")
	fmt.Println("  -r            Run the compiled C program")
	fmt.Println("  -release      Use optimizing compiler (gcc/clang -O3) for release build")
	fmt.Println("  -target <t>   Cross-compile to target: linux, windows, macos, web, or all")
	fmt.Println("  -format       Format the source file")
	fmt.Println("  -lint         Check for syntax errors without compiling")
	fmt.Println("  -gen_stdlib_docs   Generate stdlib API reference as .ahoy file")
	fmt.Println("  -inc          Enable incremental builds (cache parsed files)")
	fmt.Println("  -h            Show this help message")
	fmt.Println()
	fmt.Println("Compilation modes:")
	fmt.Println("  Default (debug): Uses TCC for fast compilation (~5ms)")
	fmt.Println("  Release (-release): Uses gcc/clang with -O3 optimization")
	fmt.Println()
	fmt.Println("Cross-compilation targets:")
	fmt.Println("  linux    - Linux x86_64 (uses zig cc for cross-compilation)")
	fmt.Println("  windows  - Windows x86_64 (uses zig cc for cross-compilation)")
	fmt.Println("  macos    - macOS x86_64 (uses zig cc for cross-compilation)")
	fmt.Println("  web      - WebAssembly (requires Emscripten/emcc)")
	fmt.Println("  all      - Build for linux, windows, and web")
	fmt.Println()
	fmt.Println("Build output:")
	fmt.Println("  Without -target: output/<name>")
	fmt.Println("  With -target:    build/<target>/<name> (with assets folder)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  ahoy -f main.ahoy                    # Compile to C only")
	fmt.Println("  ahoy -f main.ahoy -r                 # Compile and run (debug)")
	fmt.Println("  ahoy -f main.ahoy -r -release        # Compile and run (optimized)")
	fmt.Println("  ahoy -f main.ahoy -r -inc            # Compile and run with caching")
	fmt.Println("  ahoy -f main.ahoy -target linux      # Build for Linux")
	fmt.Println("  ahoy -f main.ahoy -target all        # Build for all platforms")
	fmt.Println("  ahoy -f main.ahoy -lint              # Check for errors")
}
