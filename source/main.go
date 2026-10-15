package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	"ahoy"

	"github.com/fsnotify/fsnotify"
)

// findCompiler finds the appropriate C compiler based on mode and OS
// Returns: compiler path, args, isOptimized, error
func findCompiler(releaseMode bool, debugMode bool, executablePath string) (string, []string, bool, error) {
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
			args := []string{"-O3"}
			if debugMode {
				// Release mode with debug: use address sanitizer
				args = []string{"-g", "-fsanitize=address"}
			}
			return gccPath, args, true, nil
		}

		clangPath, err := exec.LookPath("clang")
		if err == nil {
			args := []string{"-O3"}
			if debugMode {
				// Release mode with debug: use address sanitizer
				args = []string{"-g", "-fsanitize=address"}
			}
			return clangPath, args, true, nil
		}

		// On Windows, also try MSVC (cl.exe)
		if runtime.GOOS == "windows" {
			clPath, err := exec.LookPath("cl")
			if err == nil {
				args := []string{"/O2"}
				if debugMode {
					args = []string{"/Zi", "/RTC1"}
				}
				return clPath, args, true, nil
			}
		}

		// Fall back to TCC with warning
		if tccPath != "" {
			if _, err := os.Stat(tccPath); err == nil {
				fmt.Println("⚠ Warning: gcc/clang not found, using TCC (code may not be optimized)")
				fmt.Println("  Install gcc or clang for optimized release builds")
				args := tccArgs
				if debugMode {
					// TCC debug mode: -g (generate debug info)
					// Note: -b (bounds checker) requires libtcc1.a which may not be available
					args = append(args, "-g")
				}
				return tccPath, args, false, nil
			}
		}

		return "", nil, false, fmt.Errorf("no C compiler found (tried gcc, clang, tcc)")
	}

	// Debug mode - prefer TCC for fast compilation
	if tccPath != "" {
		if _, err := os.Stat(tccPath); err == nil {
			args := tccArgs
			if debugMode {
				// TCC debug mode: -g (generate debug info)
				// Note: -b (bounds checker) requires libtcc1.a which may not be available
				args = append(args, "-g")
			}
			return tccPath, args, false, nil
		}
	}

	// TCC not available, try system compilers
	gccPath, err := exec.LookPath("gcc")
	if err == nil {
		args := []string{"-O0"}
		if debugMode {
			args = []string{"-g", "-fsanitize=address"}
		}
		return gccPath, args, false, nil
	}

	clangPath, err := exec.LookPath("clang")
	if err == nil {
		args := []string{"-O0"}
		if debugMode {
			args = []string{"-g", "-fsanitize=address"}
		}
		return clangPath, args, false, nil
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
			compiler, args, _, err := findCompiler(true, false, "")
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
			compiler, args, _, err := findCompiler(true, false, "")
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
			compiler, args, _, err := findCompiler(true, false, "")
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

// raylibStaticLinkArg returns the raylib linker argument for a normal build.
// When the static archive is present it is passed by path: plain "-lraylib"
// lets the linker prefer a libraylib.so sitting next to it (a shared build
// kept around for hot reload), and the produced binary would then need that
// shared library at run time. extraLibPath reports whether "-L<dir>" is still
// needed, i.e. whether we fell back to "-lraylib".
func raylibStaticLinkArg(raylibPath string) (arg string, extraLibPath bool) {
	if raylibPath == "" {
		return "-lraylib", false
	}
	if _, err := os.Stat(filepath.Join(raylibPath, "libraylib.a")); err == nil {
		return filepath.Join(raylibPath, "libraylib.a"), false
	}
	return "-lraylib", true
}

// sdlRootFromImport returns the SDL3 source root for an import path shaped
// like .../SDL3/include/SDL3/<Module>.h (which is how bell vendors it), or
// "" when the import isn't an SDL3 header. Bell imports SDL3 per-module
// headers because the C parser does not follow the umbrella SDL.h's
// angle-bracket includes.
func sdlRootFromImport(importPath string) string {
	if !strings.HasSuffix(importPath, ".h") {
		return ""
	}
	if idx := strings.Index(importPath, "/SDL3/"); idx >= 0 {
		return importPath[:idx+5]
	}
	return ""
}

// sdlRootFromContent finds an SDL3 module import in raw file source. The
// AST scan above misses imports from directory-pulled engine files, so
// this line-based fallback keeps backend files like engine/sdl.ahoy
// self-sufficient (only real import lines match, never ? comments).
func sdlRootFromContent(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "import ") {
			continue
		}
		p := strings.Trim(strings.TrimPrefix(line, "import "), "\"'")
		if r := sdlRootFromImport(p); r != "" {
			return r
		}
	}
	return ""
}

// detectSDL reports whether any file in the main package or its resolved
// imports (directory imports included) pulls an SDL3 module header shaped
// .../SDL3/include/SDL3/<Module>.h, plus the SDL3 root for sdlLinkArgs.
// pkg.Files alone misses directory imports (e.g. bell's engine/), whose
// backend files carry the SDL import.
func detectSDL(pkg *Package, imports map[string]*Package) (bool, string) {
	for _, file := range pkg.Files {
		if file.AST != nil {
			for _, child := range file.AST.Children {
				if child.Type == ahoy.NODE_IMPORT_STATEMENT {
					if r := sdlRootFromImport(child.Value); r != "" {
						return true, r
					}
				}
			}
		}
	}
	for _, file := range pkg.Files {
		if r := sdlRootFromContent(file.Content); r != "" {
			return true, r
		}
	}
	for _, importedPkg := range imports {
		if importedPkg == nil {
			continue
		}
		for _, file := range importedPkg.Files {
			if r := sdlRootFromContent(file.Content); r != "" {
				return true, r
			}
		}
	}
	return false, ""
}

// sdlLinkArgs returns the include dir, static archive (when built) and
// platform libraries for an SDL3 root. The X11 extension libs apply on
// Linux; other platforms get include + archive only (extend as needed).
func sdlLinkArgs(sdlRoot string) []string {
	if sdlRoot == "" {
		return nil
	}
	args := []string{"-I" + filepath.Join(sdlRoot, "include")}
	if _, err := os.Stat(filepath.Join(sdlRoot, "build", "libSDL3.a")); err == nil {
		args = append(args, filepath.Join(sdlRoot, "build", "libSDL3.a"))
	} else {
		args = append(args, "-lSDL3")
	}
	if runtime.GOOS == "linux" {
		args = append(args, "-lX11", "-lXext", "-lXrandr", "-lXcursor", "-lXi", "-lXinerama", "-lXxf86vm", "-lXfixes")
	}
	return args
}

// box2dRootFromImport returns the Box2D vendor root for an import path shaped
// like .../box2d/phys2d_abi.h or .../box2d/include/box2d/<Header>.h (which is
// how bell vendors it: gcc-prebuilt libbox2d.a + headers under
// bell/repos/box2d, Task 5), or "" when the import isn't a Box2D header.
// Ahoy code imports the parser-safe phys2d_abi.h, never the real Box2D
// headers (their macro/type surface is not parser-safe); the C side of the
// bridge in bell/engine/glue.h includes the real headers directly.
func box2dRootFromImport(importPath string) string {
	if !strings.HasSuffix(importPath, ".h") {
		return ""
	}
	if idx := strings.Index(importPath, "/box2d/"); idx >= 0 {
		return importPath[:idx+7]
	}
	return ""
}

// box2dRootFromContent finds a Box2D header import in raw file source. Same
// line-based fallback as sdlRootFromContent: the AST scan misses imports
// from directory-pulled engine files (e.g. bell's engine/physics2d.ahoy).
func box2dRootFromContent(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "import ") {
			continue
		}
		p := strings.Trim(strings.TrimPrefix(line, "import "), "\"'")
		if r := box2dRootFromImport(p); r != "" {
			return r
		}
	}
	return ""
}

// detectBox2D reports whether any file in the main package or its resolved
// imports pulls a Box2D header, plus the vendor root for box2dLinkArgs.
// imports may be nil (hot-reload recompile path reuses the threaded root).
func detectBox2D(pkg *Package, imports map[string]*Package) (bool, string) {
	for _, file := range pkg.Files {
		if file.AST != nil {
			for _, child := range file.AST.Children {
				if child.Type == ahoy.NODE_IMPORT_STATEMENT {
					if r := box2dRootFromImport(child.Value); r != "" {
						return true, r
					}
				}
			}
		}
	}
	for _, file := range pkg.Files {
		if r := box2dRootFromContent(file.Content); r != "" {
			return true, r
		}
	}
	for _, importedPkg := range imports {
		if importedPkg == nil {
			continue
		}
		for _, file := range importedPkg.Files {
			if r := box2dRootFromContent(file.Content); r != "" {
				return true, r
			}
		}
	}
	return false, ""
}

// box2dIncludeDir returns the header dir for a Box2D vendor root.
func box2dIncludeDir(box2dRoot string) string {
	return filepath.Join(box2dRoot, "include")
}

// box2dStaticArchive returns the gcc-prebuilt archive when staged, or "".
func box2dStaticArchive(box2dRoot string) string {
	if box2dRoot == "" {
		return ""
	}
		p := filepath.Join(box2dRoot, "lib", "linux", "libbox2d.a")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

// box2dLinkArgs returns the include dir plus the prebuilt static archive
// for a Box2D vendor root (linux native builds). The archive is passed by
// path so neither gcc nor the tcc hot-reload link can pick a stray shared
// build; without a staged archive it falls back to -lbox2d.
func box2dLinkArgs(box2dRoot string) []string {
	if box2dRoot == "" {
		return nil
	}
	args := []string{"-I" + box2dIncludeDir(box2dRoot)}
	if lib := box2dStaticArchive(box2dRoot); lib != "" {
		args = append(args, lib)
	} else {
		args = append(args, "-lbox2d")
	}
	return args
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
	var foundLibFile string
	for _, libPath := range libSearchPaths {
		if _, err := os.Stat(libPath); err == nil {
			foundLibPath = filepath.Dir(libPath)
			foundLibFile = libPath
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
	// Cross builds stage only the archive into libs/<target>/; the headers
	// (raylib.h, rlgl.h, raymath.h) still come from the raylib source the
	// program imports, so its directory stays on the include path too.
	if !isNativeBuild && raylibPath != "" && raylibPath != effectivePath {
		flags = append(flags, "-I"+raylibPath)
	}

	// Native builds link the static archive by path when one was found, so a
	// libraylib.so kept next to it (for hot reload) is not silently picked
	// instead and baked into the binary as a runtime dependency.
	raylibLinkArg := "-lraylib"
	if isNativeBuild && foundLibFile != "" {
		raylibLinkArg = foundLibFile
	}

	switch target.Name {
	case "linux":
		// Linux uses X11 and standard Unix libraries
		flags = append(flags, raylibLinkArg, "-lGL", "-lm", "-lpthread", "-ldl", "-lrt", "-lX11")

	case "windows":
		// Windows uses different system libraries
		flags = append(flags, raylibLinkArg, "-lopengl32", "-lgdi32", "-lwinmm", "-lm")

	case "macos":
		// macOS uses frameworks instead of libraries
		flags = append(flags, raylibLinkArg)
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
func buildForTarget(target *CrossCompileTarget, cFile, baseName, sourceDir string, hasRaylib bool, raylibPath string, hasSDL bool, sdlRoot string, hasBox2D bool, box2dRoot string) error {
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
	// SDL3 headers are needed whenever a backend-capable program imports an
	// SDL3 module header (bell games always do); the native archive links
	// only on linux targets, where it was built. Windows cross builds link
	// a windows-built SDL3 archive staged next to the raylib one; web
	// builds use emscripten's SDL3 port instead.
	if hasSDL {
		if target.Name == "linux" {
			args = append(args, sdlLinkArgs(sdlRoot)...)
		} else if sdlRoot != "" {
			args = append(args, "-I"+filepath.Join(sdlRoot, "include"))
		}
		if target.Name == "windows" {
			sdlWin := filepath.Join(sourceDir, "libs", "windows", "libSDL3.a")
			if _, err := os.Stat(sdlWin); err == nil {
				args = append(args, sdlWin)
				// Static SDL3 on Windows pulls the COM/MM/setup APIs.
				args = append(args, "-lole32", "-loleaut32", "-luuid", "-lsetupapi", "-limm32", "-lversion", "-ladvapi32", "-lshell32")
			}
		}
		// Emscripten ships SDL3 as a port; bell games always import the
		// headers, so web builds enable it (the export selects the raylib
		// backend, but the SDL translation unit still needs its symbols).
		if target.Name == "web" {
			args = append(args, "-sUSE_SDL=3")
		}
	}
	// Box2D (Task 5): the prebuilt archive exists for linux only; other
	// targets get the headers so the translation unit still parses, and a
	// platform archive can be staged later under bell/repos/box2d/lib.
	if hasBox2D {
		if target.Name == "linux" {
			args = append(args, box2dLinkArgs(box2dRoot)...)
		} else if box2dRoot != "" {
			args = append(args, "-I"+box2dIncludeDir(box2dRoot))
		}
	}
	// Web builds use the project's own shell page when it has one: the
	// stock emscripten shell carries http links (blocked as mixed content
	// on itch) and a fullscreen button that throws inside itch's iframe.
	// bell's export stages tools/web_shell.html next to the game on first
	// export; projects can edit their copy freely (it is never overwritten).
	if target.Name == "web" {
		shellFile := filepath.Join(sourceDir, "web_shell.html")
		if _, err := os.Stat(shellFile); err == nil {
			args = append(args, "--shell-file", shellFile)
		}
		// The export stages the packed assets next to the game; preload
		// the pak itself so the web build unpacks exactly like desktop
		// (loose sourceDir/assets still preload as before, as fallback).
		pakFile := filepath.Join(sourceDir, "game.pak")
		if _, err := os.Stat(pakFile); err == nil {
			args = append(args, "--preload-file", pakFile+"@/game.pak")
		}
	}
	args = withDefines(args)

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

// pthreadFlags returns the threading library flags for the host platform. The
// job system uses pthreads; Windows has no -lpthread.
func pthreadFlags() []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	return []string{"-lpthread"}
}

// defineList collects -D NAME flags. Names are made available to `when NAME:`
// blocks, which emit #ifdef NAME into the generated C.
type defineList []string

func (d *defineList) String() string { return strings.Join(*d, ",") }

func (d *defineList) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		if name := strings.TrimSpace(part); name != "" {
			*d = append(*d, name)
		}
	}
	return nil
}

// buildDefines is the resolved -D list, appended to every C compile.
var buildDefines []string

// withDefines appends the -D flags for `when` blocks to a compiler argument list.
func withDefines(args []string) []string {
	for _, name := range buildDefines {
		args = append(args, "-D"+name)
	}
	return args
}

// watchHeapMB samples the Go heap every interval; after two consecutive
// samples above capMB it calls onFire (in MB) and keeps sampling — the
// caller decides whether to exit. Two consecutive samples (rather than one)
// mean a single legitimate spike can't trip it. Returns a stop function.
func watchHeapMB(capMB int, interval time.Duration, onFire func(mb float64)) func() {
	capBytes := uint64(capMB) * 1024 * 1024
	stop := make(chan struct{})
	go func() {
		over := 0
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				if m.HeapAlloc > capBytes {
					over++
					if over >= 2 {
						onFire(float64(m.HeapAlloc) / 1048576.0)
					}
				} else {
					over = 0
				}
			}
		}
	}()
	return func() { close(stop) }
}

func main() {
	// Define CLI flags
	fileFlag := flag.String("f", "", "Input .ahoy source file")
	runFlag := flag.Bool("r", false, "Run the compiled C program after compilation")
	formatFlag := flag.Bool("format", false, "Format the source file")
	lintFlag := flag.Bool("lint", false, "Run linter to check for errors without compiling")
	releaseFlag := flag.Bool("release", false, "Use optimizing compiler (gcc/clang) for release build")
	debugFlag := flag.Bool("show_leaks", false, "Enable memory leak detection with AddressSanitizer (gcc: -fsanitize=address)")
	targetFlag := flag.String("target", "", "Cross-compile target: linux, windows, web, macos, or all")
	incFlag := flag.Bool("cache", false, "Enable incremental builds (cache parsed files)")
	maxmemFlag := flag.Int("maxmem", 2048, "Abort the build if the compiler heap exceeds this many MB (0 disables)")
	profCompFlag := flag.Bool("profile_compiler", false, "Enable CPU profiling of compiler (creates pprof file)")
	genStdlibFlag := flag.Bool("gen_stdlib_docs", false, "Generate stdlib documentation as .ahoy file")
	hotReloadFlag := flag.Bool("hotreload", false, "Enable hot code reloading (keeps window open, reloads on file changes) - development only")
	coldReloadFlag := flag.Bool("coldreload", false, "Enable cold reload (auto-recompile and restart on file changes) - development only")
	noSemanticsFlag := flag.Bool("no_semantics", false, "Skip semantic validation (type errors, undeclared names, duplicate definitions)")
	defines := defineList{}
	flag.Var(&defines, "D", "Define NAME for `when NAME:` blocks (repeatable, comma-separated)")
	helpFlag := flag.Bool("h", false, "Show help")

	flag.Parse()

	buildDefines = defines

	// Check for conflicting flags
	if *hotReloadFlag && *releaseFlag {
		fmt.Println("Error: -hotreload cannot be used with -release flag")
		fmt.Println("Hot reload is for development only. Remove -release flag.")
		os.Exit(1)
	}

	if *coldReloadFlag && *releaseFlag {
		fmt.Println("Error: -coldreload cannot be used with -release flag")
		fmt.Println("Cold reload is for development only. Remove -release flag.")
		os.Exit(1)
	}

	if *hotReloadFlag && *coldReloadFlag {
		fmt.Println("Error: Cannot use both -hotreload and -coldreload")
		fmt.Println("Choose one reload mode.")
		os.Exit(1)
	}

	// Heap watchdog: abort a runaway build instead of OOMing the machine
	// (a validation pathology once allocated gigabytes here). Fires only
	// after two consecutive over-cap samples, so one legitimate spike can't
	// kill a healthy build. Covers Go-heap phases (parse/lint/codegen);
	// the C compiler subprocess is outside it. -maxmem 0 disables it.
	if *maxmemFlag > 0 {
		stopWatch := watchHeapMB(*maxmemFlag, 200*time.Millisecond, func(mb float64) {
			fmt.Fprintf(os.Stderr, "fatal: compiler heap %.0f MB exceeds -maxmem %d MB sustained; aborted to protect the system (raise -maxmem if this build legitimately needs more)\n", mb, *maxmemFlag)
			os.Exit(3)
		})
		defer stopWatch()
	}

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

	// Upfront checks. The entry file is read and tokenized here only for
	// --format and --lint, which operate on it directly. A normal build loads
	// every file (including this one) through the package manager below, so
	// reading and tokenizing it here would just be thrown away; the manager's
	// TokenizeTime is reported as token= at the end.
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

	var lintTokens []ahoy.Token
	var lintContent string
	if *formatFlag || *lintFlag {
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

		// Tokenize for the lint pass below.
		lintContent = string(content)
		lintTokens = ahoy.Tokenize(lintContent)
	}
	tokenTime := time.Since(tokenStart)

	// Lint mode
	if *lintFlag {
		ast, diagnostics := ahoy.ParseLintWithPath(lintTokens, sourceFile)
		sources := map[string]string{sourceFile: lintContent}

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

		// Multi-file programs: validate the whole program, imports included.
		// This pass knows every name the program declares (sibling files and
		// directory/file imports like a game engine), so it supersedes the
		// single-file pass above - otherwise a function declared in another
		// file of the same program would be reported as undefined.
		if programName != "" {
			if absPath, absErr := filepath.Abs(sourceFile); absErr == nil {
				pm := NewPackageManager(filepath.Dir(absPath))
				pm.ValidateOnLoad = false
				if pkg, pkgErr := pm.LoadPackageFromFile(absPath); pkgErr == nil {
					imports, impErr := resolveImports(pkg, pm, absPath)
					if impErr != nil {
						fmt.Printf("Error resolving imports: %v\n", impErr)
						os.Exit(1)
					}
					packages := collectPackages(pkg, imports)
					allNames := map[string]bool{}
					for _, p := range packages {
						for _, file := range p.Files {
							for _, name := range ahoy.DeclaredNames(file.AST) {
								allNames[name] = true
							}
						}
					}
					programNames := make([]string, 0, len(allNames))
					for name := range allNames {
						programNames = append(programNames, name)
					}
					// Method tables from every file, so method calls validate across
					// files the way free functions do via programNames.
					var methodASTs []*ahoy.ASTNode
					for _, p := range packages {
						for i := range p.Files {
							if p.Files[i].AST != nil {
								methodASTs = append(methodASTs, p.Files[i].AST)
							}
						}
					}
					programMethods := ahoy.CollectMethods(methodASTs...)
					diagnostics = nil
					for _, p := range packages {
						for i := range p.Files {
							file := &p.Files[i]
							file.SemanticErrors = validateSemanticsWithTokens(file.lintTokens(), file.Path, programNames, programMethods)
							diagnostics = append(diagnostics, file.SemanticErrors...)
							sources[file.Path] = file.Content
						}
						diagnostics = append(diagnostics, findDuplicateFunctions(p)...)
					}
				}
			}
		}

		fatal := printDiagnostics(dedupeDiagnostics(diagnostics), sources)
		if fatal > 0 {
			os.Exit(1)
		}

		fmt.Printf("✓ No errors found in %s\n", sourceFile)
		if hasCImports {
			fmt.Printf("  Note: file uses C imports; C symbols are not validated.\n")
		}
		return
	}

	// Get absolute path for source file
	absPath, err := filepath.Abs(sourceFile)
	if err != nil {
		fmt.Printf("Error resolving file path: %v\n", err)
		os.Exit(1)
	}

	// Initialize package manager. Per-package validation is off on this path:
	// the whole-program pass below checks every file with all the names the
	// program declares (main package plus imports). Validating at load time
	// would run the same checks earlier with only one package's names in
	// scope, so every cross-package reference would read as undefined -
	// hundreds of spurious errors whose diagnostics are then discarded and
	// recomputed below anyway.
	pm := NewPackageManager(filepath.Dir(absPath))
	pm.ValidateOnLoad = false

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

	// Semantic validation. Code generation does not run these checks, so without
	// them a broken program either fails inside the C compiler with a message
	// about generated code, or compiles into something that misbehaves at run
	// time. `-no_semantics` bypasses this if the checks get in the way.
	lintStart := time.Now()
	if !*noSemanticsFlag {
		// A file's checks run against the whole program, not just its own
		// package: an imported package (the engine, for a game) declares names
		// its callers use. validateProgram can only see one package at load
		// time, so re-run the pass here with every name the program declares.
		packages := collectPackages(pkg, imports)
		allNames := map[string]bool{}
		for _, p := range packages {
			for _, file := range p.Files {
				for _, name := range ahoy.DeclaredNames(file.AST) {
					allNames[name] = true
				}
			}
		}
		programNames := make([]string, 0, len(allNames))
		for name := range allNames {
			programNames = append(programNames, name)
		}
		var methodASTs []*ahoy.ASTNode
		for _, p := range packages {
			for i := range p.Files {
				if p.Files[i].AST != nil {
					methodASTs = append(methodASTs, p.Files[i].AST)
				}
			}
		}
		programMethods := ahoy.CollectMethods(methodASTs...)

		// With -cache, skip the whole pass when every input is identical to
		// the last validated build: the fingerprint covers each file's
		// content, the -D defines, and the C headers validation reads.
		// Per-file reuse alone would be unsound (one file's errors can name
		// names declared in another), so only an exact whole-program match
		// reuses. Any change anywhere revalidates everything.
		lintReused := false
		var fingerprint string
		if *incFlag {
			fingerprint = validationFingerprint(packages, defines)
			if cached, ok := GetBuildCache().GetValidation(fingerprint); ok {
				complete := true
				for _, p := range packages {
					for i := range p.Files {
						file := &p.Files[i]
						if errs, ok := cached[file.Path]; ok {
							file.SemanticErrors = errs
						} else {
							complete = false
							break
						}
					}
					if !complete {
						break
					}
				}
				lintReused = complete
			}
		}
		if !lintReused {
			for _, p := range packages {
				for i := range p.Files {
					file := &p.Files[i]
					file.SemanticErrors = validateSemanticsWithTokens(file.lintTokens(), file.Path, programNames, programMethods)
				}
			}
			if *incFlag {
				collected := make(map[string][]ahoy.ParseError)
				for _, p := range packages {
					for _, file := range p.Files {
						collected[file.Path] = file.SemanticErrors
					}
				}
				GetBuildCache().SetValidation(fingerprint, collected)
			}
		} else {
			fmt.Println("✓ Reused cached validation (no changes)")
		}

		sources := map[string]string{}
		var diagnostics []ahoy.ParseError
		for _, p := range packages {
			for _, file := range p.Files {
				sources[file.Path] = file.Content
			}
			diagnostics = append(diagnostics, diagnosticsFromPackage(p)...)
		}
		if fatal := printDiagnostics(dedupeDiagnostics(diagnostics), sources); fatal > 0 {
			fmt.Fprintf(os.Stderr, "\n%d error(s) found; compilation aborted.\n", fatal)
			os.Exit(1)
		}
	}
	lintTime := time.Since(lintStart)

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

	// Determine if using raylib (needed for hot reload and compilation)
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

	// SDL3 opts the binary into includes + libSDL3.a (see sdlLinkArgs).
	hasSDL, sdlRoot := detectSDL(pkg, imports)

	// Box2D opts the binary into includes + libbox2d.a (see box2dLinkArgs).
	hasBox2D, box2dRoot := detectBox2D(pkg, imports)

	// Hot reload mode - generate shared library and host
	if *hotReloadFlag {
		handleHotReload(sourceFile, ast, absPath, hasRaylib, raylibPath, hasSDL, sdlRoot, hasBox2D, box2dRoot)
		return // Hot reload runs indefinitely
	}

	// Cold reload mode - watch and auto-recompile
	if *coldReloadFlag {
		handleColdReload(sourceFile, absPath)
		return // Cold reload runs indefinitely
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

	// Calculate total parse time (load + imports + merge). Validation has its
	// own counter now so a slow lint pass can no longer hide inside parse=.
	parseTime := loadTime + importsTime + mergeTime
	tokenTime += pm.TokenizeTime
	if len(pkg.Files) > 1 {
		fmt.Printf("✓ Compiled package '%s' (%d files) to %s\n", pkg.Name, len(pkg.Files), outputFile)
		fmt.Printf(" Time: token=%s, parse=%s, lint=%s, codegen=%s, cache=%s\n",
			formatTime(tokenTime), formatTime(parseTime), formatTime(lintTime), formatTime(codegenTime), formatTime(cacheTime))
	} else {
		fmt.Printf(" Time: token=%s, parse=%s, lint=%s, codegen=%s, cache=%s\n",
			formatTime(tokenTime), formatTime(parseTime), formatTime(lintTime), formatTime(codegenTime), formatTime(cacheTime))
	}

	// Compile C code if run flag is set
	if *runFlag {
		fmt.Println("Compiling C code...")

		// Start C compilation timing
		cCompileStart := time.Now()

		// Find the appropriate C compiler
		compiler, baseArgs, isOptimized, err := findCompiler(*releaseFlag, *debugFlag, executable)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		// Build compilation arguments
		compileArgs := append(baseArgs, "-o", executable, outputFile)

		// Add raylib include and linking flags if needed (already determined above)
		if hasRaylib {
			if raylibPath != "" {
				compileArgs = append(compileArgs, "-I"+raylibPath)
			}
			raylibArg, needsLibPath := raylibStaticLinkArg(raylibPath)
			if needsLibPath && raylibPath != "" {
				compileArgs = append(compileArgs, "-L"+raylibPath)
			}
			compileArgs = append(compileArgs, raylibArg, "-lm", "-lpthread", "-ldl", "-lrt", "-lX11")
		} else {
			compileArgs = append(compileArgs, "-lm")
			compileArgs = append(compileArgs, pthreadFlags()...)
		}
		if hasSDL {
			compileArgs = append(compileArgs, sdlLinkArgs(sdlRoot)...)
		}
		if hasBox2D {
			compileArgs = append(compileArgs, box2dLinkArgs(box2dRoot)...)
		}

		compileArgs = withDefines(compileArgs)
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

			if err := buildForTarget(target, outputFile, baseName, sourceDir, hasRaylib, raylibPath, hasSDL, sdlRoot, hasBox2D, box2dRoot); err != nil {
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

	// First, add all declarations from imported packages, in package-name
	// order: imports arrives as a map, and ranging over it would merge files
	// in a random order on every build (shuffling forward declarations and
	// other emitted order around).
	importNames := make([]string, 0, len(imports))
	for name := range imports {
		importNames = append(importNames, name)
	}
	sort.Strings(importNames)
	for _, name := range importNames {
		importedPkg := imports[name]
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

// validationFingerprint hashes every input the whole-program semantic pass
// depends on: each .ahoy file's path and content, the -D defines (they gate
// `when` blocks), and the C headers validation reads for signatures. Two
// builds with equal fingerprints validate identically, so the second may
// reuse the first's diagnostics instead of re-running the pass.
func validationFingerprint(packages []*Package, defines defineList) string {
	var sb strings.Builder
	paths := make([]string, 0)
	contents := make(map[string]string)
	headerSet := make(map[string]bool)
	for _, p := range packages {
		for _, file := range p.Files {
			paths = append(paths, file.Path)
			contents[file.Path] = file.Content
			if file.AST == nil {
				continue
			}
			for _, child := range file.AST.Children {
				if child.Type == ahoy.NODE_IMPORT_STATEMENT && strings.HasSuffix(child.Value, ".h") {
					h := child.Value
					if !filepath.IsAbs(h) {
						h = filepath.Clean(filepath.Join(filepath.Dir(file.Path), h))
					}
					headerSet[h] = true
				}
			}
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		sb.WriteString(path)
		sb.WriteString("\x00")
		sb.WriteString(computeChecksum(contents[path]))
		sb.WriteString("\x00")
	}
	flagCopy := make([]string, len(defines))
	copy(flagCopy, defines)
	sort.Strings(flagCopy)
	for _, d := range flagCopy {
		sb.WriteString("D:" + d + "\x00")
	}
	headers := make([]string, 0, len(headerSet))
	for h := range headerSet {
		headers = append(headers, h)
	}
	sort.Strings(headers)
	for _, h := range headers {
		sb.WriteString("H:" + h + "\x00")
		if content, err := os.ReadFile(h); err == nil {
			sb.WriteString(computeChecksum(string(content)))
		} else {
			sb.WriteString("missing")
		}
		sb.WriteString("\x00")
	}
	return computeChecksum(sb.String())
}

// collectPackages returns the main package plus every imported package, without
// duplicates.
func collectPackages(pkg *Package, imports map[string]*Package) []*Package {
	result := []*Package{pkg}
	seen := map[*Package]bool{pkg: true}
	for _, imported := range imports {
		if imported == nil || seen[imported] {
			continue
		}
		seen[imported] = true
		result = append(result, imported)
	}
	return result
}

// diagnosticsFromPackage gathers every diagnostic recorded while loading a
// package, including definitions that collide across the program's files.
func diagnosticsFromPackage(pkg *Package) []ahoy.ParseError {
	var diagnostics []ahoy.ParseError
	for _, file := range pkg.Files {
		diagnostics = append(diagnostics, file.SemanticErrors...)
	}
	diagnostics = append(diagnostics, findDuplicateFunctions(pkg)...)
	return diagnostics
}

// findDuplicateFunctions reports functions defined more than once across the
// files of a single program. Ahoy has no overloading, and compilation would
// otherwise silently keep whichever definition wins in the generated C.
func findDuplicateFunctions(pkg *Package) []ahoy.ParseError {
	type definition struct {
		file string
		line int
	}
	seen := make(map[string]definition)
	var diagnostics []ahoy.ParseError
	for _, file := range pkg.Files {
		if file.AST == nil {
			continue
		}
		for _, child := range file.AST.Children {
			if child.Type != ahoy.NODE_FUNCTION {
				continue
			}
			if first, exists := seen[child.Value]; exists {
				diagnostics = append(diagnostics, ahoy.ParseError{
					Message: fmt.Sprintf("Function '%s' is already declared in %s (line %d); Ahoy doesn't support function overloading",
						child.Value, filepath.Base(first.file), first.line),
					Line:   child.Line,
					Column: child.Column,
					File:   file.Path,
				})
				continue
			}
			seen[child.Value] = definition{file: file.Path, line: child.Line}
		}
	}
	return diagnostics
}

// dedupeDiagnostics removes repeated diagnostics, keeping the first occurrence.
// A file can be validated more than once: directly, and again as part of the
// multi-file program it belongs to.
func dedupeDiagnostics(diagnostics []ahoy.ParseError) []ahoy.ParseError {
	seen := make(map[string]bool, len(diagnostics))
	result := make([]ahoy.ParseError, 0, len(diagnostics))
	for _, d := range diagnostics {
		key := fmt.Sprintf("%s\x00%d\x00%d\x00%s", d.File, d.Line, d.Column, d.Message)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, d)
	}
	return result
}

// printDiagnostics writes diagnostics as file:line:column with the offending
// source line and a caret, and returns how many of them are fatal. Warnings are
// reported but never fail a build.
func printDiagnostics(diagnostics []ahoy.ParseError, sources map[string]string) (fatal int) {
	if len(diagnostics) == 0 {
		return 0
	}

	sorted := make([]ahoy.ParseError, len(diagnostics))
	copy(sorted, diagnostics)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].File != sorted[j].File {
			return sorted[i].File < sorted[j].File
		}
		if sorted[i].Line != sorted[j].Line {
			return sorted[i].Line < sorted[j].Line
		}
		return sorted[i].Column < sorted[j].Column
	})

	for _, d := range sorted {
		severity := "error"
		if d.IsWarning() {
			severity = "warning"
		} else {
			fatal++
		}
		file := d.File
		if file == "" {
			file = "<unknown>"
		}
		fmt.Fprintf(os.Stderr, "%s:%d:%d: %s: %s\n", file, d.Line, d.Column, severity, d.Message)
		if excerpt := sourceExcerpt(sources[file], d.Line, d.Column); excerpt != "" {
			fmt.Fprint(os.Stderr, excerpt)
		}
	}
	return fatal
}

// sourceExcerpt renders one source line with a caret under the reported column.
func sourceExcerpt(content string, line, column int) string {
	if content == "" || line <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if line > len(lines) {
		return ""
	}
	text := strings.TrimRight(lines[line-1], "\r")
	if strings.TrimSpace(text) == "" {
		return ""
	}

	// Tabs would misalign the caret, so measure the prefix with them expanded.
	const tabWidth = 4
	expand := func(s string) string {
		return strings.ReplaceAll(s, "\t", strings.Repeat(" ", tabWidth))
	}
	if column < 1 {
		column = 1
	}
	if column > len(text)+1 {
		column = len(text) + 1
	}
	caretOffset := len(expand(text[:column-1]))

	gutter := fmt.Sprintf("%5d | ", line)
	var b strings.Builder
	b.WriteString(gutter)
	b.WriteString(expand(text))
	b.WriteString("\n")
	b.WriteString(strings.Repeat(" ", len(gutter)+caretOffset))
	b.WriteString("^\n")
	return b.String()
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
	fmt.Println("  -show_leaks   Enable memory leak detection with AddressSanitizer")
	fmt.Println("  -target <t>   Cross-compile to target: linux, windows, macos, web, or all")
	fmt.Println("  -format       Format the source file")
	fmt.Println("  -lint         Check for errors without compiling")
	fmt.Println("  -no_semantics Skip semantic validation (types, undeclared names, duplicates)")
	fmt.Println("  -gen_stdlib_docs   Generate stdlib API reference as .ahoy file")
	fmt.Println("  -cache        Enable incremental builds (cache parsed files)")
	fmt.Println("  -D NAME       Define NAME for `when NAME then` blocks (repeatable, comma-separated)")
	fmt.Println("  -hotreload    Enable hot code reloading (development only, keeps window open)")
	fmt.Println("  -coldreload   Enable cold reload (development only, auto-recompile and restart)")
	fmt.Println("  -h            Show this help message")
	fmt.Println()
	fmt.Println("Diagnostics:")
	fmt.Println("  Type errors, undeclared variables, argument mismatches and duplicate")
	fmt.Println("  definitions are reported as file:line:column with the source line and")
	fmt.Println("  a caret. Use -no_semantics to compile without these checks.")
	fmt.Println()
	fmt.Println("Compilation modes:")
	fmt.Println("  Default (debug): Uses TCC for fast compilation (~5ms)")
	fmt.Println("  Release (-release): Uses gcc/clang with -O3 optimization")
	fmt.Println()
	fmt.Println("Development modes:")
	fmt.Println("  Hot Reload (-hotreload):  Keeps window open, reloads code on save (Raylib games)")
	fmt.Println("  Cold Reload (-coldreload): Auto-recompile and restart on save (any program)")
	fmt.Println("  Note: Both use TCC for instant compilation and cannot be used with -release")
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
	fmt.Println("  ahoy -f main.ahoy -hotreload         # Hot reload development mode")
	fmt.Println("  ahoy -f main.ahoy -coldreload        # Cold reload development mode")
	fmt.Println("  ahoy -f main.ahoy -target linux      # Build for Linux")
	fmt.Println("  ahoy -f main.ahoy -target all        # Build for all platforms")
	fmt.Println("  ahoy -f main.ahoy -lint              # Check for errors")
}

// handleHotReload compiles the program in hot reload mode
// This keeps the window open and reloads code on file changes
func handleHotReload(sourceFile string, ast *ahoy.ASTNode, absPath string, hasRaylib bool, raylibPath string, hasSDL bool, sdlRoot string, hasBox2D bool, box2dRoot string) {
	outputDir := "output"
	sourceDir := filepath.Dir(absPath)

	// Generate library and host code
	fmt.Println("🔥 Hot Reload Mode - Generating code...")
	libraryCode, hostCode := GenerateCWithHotReload(ast, sourceFile, hasRaylib, raylibPath)

	// Write files
	os.MkdirAll(outputDir, 0755)

	libSourceFile := filepath.Join(outputDir, "libgame.c")
	hostSourceFile := filepath.Join(outputDir, "hotreload_host.c")
	libFile := filepath.Join(outputDir, "libgame.so")
	hostBinary := filepath.Join(outputDir, "hotreload_host")

	err := os.WriteFile(libSourceFile, []byte(libraryCode), 0644)
	if err != nil {
		fmt.Printf("Error writing library source: %v\n", err)
		os.Exit(1)
	}

	err = os.WriteFile(hostSourceFile, []byte(hostCode), 0644)
	if err != nil {
		fmt.Printf("Error writing host source: %v\n", err)
		os.Exit(1)
	}

	// Compile library with TCC for fast iteration
	fmt.Println("⚡ Compiling game library with TCC...")

	tccPath := findTCCCompiler()
	var compiler string
	var libArgs []string

	if tccPath != "" {
		compiler = tccPath
		libArgs = []string{"-shared", "-o", libFile, libSourceFile}

		if hasRaylib {
			if raylibPath != "" {
				libArgs = append(libArgs, "-I"+raylibPath, "-L"+raylibPath)
			}
			libArgs = append(libArgs, "-lraylib", "-lm", "-lpthread", "-ldl")
		} else {
			libArgs = append(libArgs, "-lm")
			libArgs = append(libArgs, pthreadFlags()...)
		}
	} else {
		fmt.Println("⚠️  TCC not found, falling back to GCC...")
		compiler = "gcc"
		libArgs = []string{"-shared", "-fPIC", "-o", libFile, libSourceFile, "-O0", "-g"}

		if hasRaylib {
			if raylibPath != "" {
				libArgs = append(libArgs, "-I"+raylibPath, "-L"+raylibPath)
			}
			libArgs = append(libArgs, "-lraylib", "-lm", "-lpthread", "-ldl", "-lrt", "-lX11")
		} else {
			libArgs = append(libArgs, "-lm")
			libArgs = append(libArgs, pthreadFlags()...)
		}
	}

	libArgs = withDefines(libArgs)
	if hasSDL {
		libArgs = append(libArgs, sdlLinkArgs(sdlRoot)...)
	}
	if hasBox2D {
		libArgs = append(libArgs, box2dLinkArgs(box2dRoot)...)
	}
	cmd := exec.Command(compiler, libArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error compiling library:\n%s\n", output)
		os.Exit(1)
	}

	fmt.Printf("✓ Compiled library: %s\n", libFile)

	// Compile host with TCC (only once, doesn't change)
	fmt.Println("⚡ Compiling hot reload host...")

	var hostCompiler string
	var hostArgs []string

	if tccPath != "" {
		hostCompiler = tccPath
		hostArgs = []string{"-o", hostBinary, hostSourceFile, "-ldl", "-lpthread", "-lm"}
	} else {
		hostCompiler = "gcc"
		hostArgs = []string{"-o", hostBinary, hostSourceFile, "-ldl", "-lpthread", "-lm"}
	}

	cmd = exec.Command(hostCompiler, hostArgs...)
	output, err = cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error compiling host:\n%s\n", output)
		os.Exit(1)
	}

	fmt.Printf("✓ Compiled host: %s\n", hostBinary)

	// Start the host in background
	fmt.Println()
	// The host runs with cwd = source dir, so hand it the absolute library
	// path instead of making it guess a relative "output/" location.
	absLibFile, absErr := filepath.Abs(libFile)
	if absErr != nil {
		absLibFile = libFile
	}
	hostCmd := exec.Command(hostBinary, absLibFile)
	hostCmd.Stdout = os.Stdout
	hostCmd.Stderr = os.Stderr
	hostCmd.Dir = sourceDir

	// Start host but don't exit when it stops - let the watcher continue
	go func() {
		err := hostCmd.Run()
		if err != nil {
			fmt.Printf("Host exited: %v\n", err)
		} else {
			fmt.Println("\n\033[32m[Hot Reload]\033[0m Host exited normally")
		}
		// Don't call os.Exit() - let the watcher continue running
	}()

	// Give the host time to start
	time.Sleep(500 * time.Millisecond)

	// Watch for file changes and recompile
	watchAndRecompileHot(sourceFile, sourceDir, libSourceFile, libFile, hasRaylib, raylibPath, hasSDL, sdlRoot, hasBox2D, box2dRoot)
}

// handleColdReload watches files and auto-recompiles (restarts program)
func handleColdReload(sourceFile string, absPath string) {
	fmt.Println("\n❄️  Cold Reload Mode - Auto-recompile enabled")
	fmt.Println("📝 Edit and save to auto-recompile and restart")
	fmt.Println("⚡ Using fast compilation")
	fmt.Println("🛑 Press Ctrl+C to stop")
	fmt.Println()

	sourceDir := filepath.Dir(absPath)

	// Do initial compilation and run
	compileAndRunCold(sourceFile, true)

	// Watch for changes
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Printf("Error creating watcher: %v\n", err)
		return
	}
	defer watcher.Close()

	err = watcher.Add(sourceDir)
	if err != nil {
		fmt.Printf("Error watching directory: %v\n", err)
		return
	}

	var debounceTimer *time.Timer

	for {
		select {
		case event := <-watcher.Events:
			if !strings.HasSuffix(event.Name, ".ahoy") {
				continue
			}

			// A save is not always a plain write: editors (and `sed -i`) often
			// replace the file by writing a temp file and renaming it over the
			// original, which fsnotify reports as Create/Rename on the watched
			// directory rather than Write. Treat any of them as a change.
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 {
				if debounceTimer != nil {
					debounceTimer.Stop()
				}

				debounceTimer = time.AfterFunc(10*time.Millisecond, func() {
					fmt.Println("\n📝 Change detected, recompiling...")
					compileAndRunCold(sourceFile, false)
				})
			}

		case err := <-watcher.Errors:
			fmt.Printf("Watcher error: %v\n", err)
		}
	}
}

// watchAndRecompileHot watches for file changes and recompiles the library for hot reload
func watchAndRecompileHot(sourceFile, sourceDir string, libSourceFile, libFile string, hasRaylib bool, raylibPath string, hasSDL bool, sdlRoot string, hasBox2D bool, box2dRoot string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Printf("Error creating watcher: %v\n", err)
		return
	}
	defer watcher.Close()

	// Watch the source directory
	err = watcher.Add(sourceDir)
	if err != nil {
		fmt.Printf("Error watching directory: %v\n", err)
		return
	}

	// Also watch common shader directories if they exist
	shaderDirs := []string{
		filepath.Join(sourceDir, "shaders"),
		filepath.Join(sourceDir, "shader"),
		filepath.Join(sourceDir, "glsl"),
	}
	for _, dir := range shaderDirs {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			if err := watcher.Add(dir); err == nil {
				fmt.Printf("👁️  Also watching: %s\n", filepath.Base(dir))
			}
		}
	}

	var debounceTimer *time.Timer

	for {
		select {
		case event := <-watcher.Events:
			// Watch for .ahoy, .vs (vertex shader), and .fs (fragment shader) files
			isWatchedFile := strings.HasSuffix(event.Name, ".ahoy") ||
				strings.HasSuffix(event.Name, ".vs") ||
				strings.HasSuffix(event.Name, ".fs")

			if !isWatchedFile {
				continue
			}

			// A save is not always a plain write: editors (and `sed -i`) often
			// replace the file by writing a temp file and renaming it over the
			// original, which fsnotify reports as Create/Rename on the watched
			// directory rather than Write. Treat any of them as a change.
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 {
				if debounceTimer != nil {
					debounceTimer.Stop()
				}

				// Show which file triggered the reload
				fileName := filepath.Base(event.Name)
				fileType := "code"
				if strings.HasSuffix(event.Name, ".vs") || strings.HasSuffix(event.Name, ".fs") {
					fileType = "shader"
				}

				debounceTimer = time.AfterFunc(10*time.Millisecond, func() {
					fmt.Printf("\n📝 %s file changed: %s\n", fileType, fileName)
					recompileLibraryHot(sourceFile, libSourceFile, libFile, hasRaylib, raylibPath, hasSDL, sdlRoot, hasBox2D, box2dRoot)
				})
			}

		case err := <-watcher.Errors:
			fmt.Printf("Watcher error: %v\n", err)
		}
	}
}

// recompileLibraryHot recompiles the game library for hot reload
func recompileLibraryHot(sourceFile, libSourceFile, libFile string, hasRaylib bool, raylibPath string, hasSDL bool, sdlRoot string, hasBox2D bool, box2dRoot string) {
	fmt.Println("\n🔄 Change detected, recompiling...")
	start := time.Now()

	// Get absolute path
	absPath, err := filepath.Abs(sourceFile)
	if err != nil {
		fmt.Printf("❌ Error resolving file path: %v\n", err)
		return
	}

	// Load the full package (handles multi-file programs). Validation is off:
	// these paths never read the diagnostics and go straight to codegen.
	pm := NewPackageManager(filepath.Dir(absPath))
	pm.ValidateOnLoad = false
	pkg, err := pm.LoadPackageFromFile(absPath)
	if err != nil {
		fmt.Printf("❌ Error loading package: %v\n", err)
		return
	}

	// Resolve imports
	imports, err := resolveImports(pkg, pm, absPath)
	if err != nil {
		fmt.Printf("❌ Error resolving imports: %v\n", err)
		return
	}

	// Merge package with imports
	ast := MergeWithImports(pkg, imports)

	// Regenerate C code
	libraryCode, _ := GenerateCWithHotReload(ast, sourceFile, hasRaylib, raylibPath)

	err = os.WriteFile(libSourceFile, []byte(libraryCode), 0644)
	if err != nil {
		fmt.Printf("❌ Error writing library source: %v\n", err)
		return
	}

	// Recompile library with TCC for fast iteration
	// Use temporary file to avoid corrupting the working library on compilation errors
	tempLibFile := libFile + ".tmp"

	tccPath := findTCCCompiler()
	var compiler string
	var libArgs []string

	if tccPath != "" {
		compiler = tccPath
		libArgs = []string{"-shared", "-o", tempLibFile, libSourceFile}

		if hasRaylib {
			if raylibPath != "" {
				libArgs = append(libArgs, "-I"+raylibPath, "-L"+raylibPath)
			}
			libArgs = append(libArgs, "-lraylib", "-lm", "-lpthread", "-ldl")
		} else {
			libArgs = append(libArgs, "-lm")
			libArgs = append(libArgs, pthreadFlags()...)
		}
	} else {
		compiler = "gcc"
		libArgs = []string{"-shared", "-fPIC", "-o", tempLibFile, libSourceFile, "-O0", "-g"}

		if hasRaylib {
			if raylibPath != "" {
				libArgs = append(libArgs, "-I"+raylibPath, "-L"+raylibPath)
			}
			libArgs = append(libArgs, "-lraylib", "-lm", "-lpthread", "-ldl", "-lrt", "-lX11")
		} else {
			libArgs = append(libArgs, "-lm")
			libArgs = append(libArgs, pthreadFlags()...)
		}
	}

	libArgs = withDefines(libArgs)
	if hasSDL {
		libArgs = append(libArgs, sdlLinkArgs(sdlRoot)...)
	}
	if hasBox2D {
		libArgs = append(libArgs, box2dLinkArgs(box2dRoot)...)
	}
	cmd := exec.Command(compiler, libArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("❌ Compilation failed:\n%s\n", output)
		// Remove temp file if it exists
		os.Remove(tempLibFile)
		return
	}

	// Only replace the real library if compilation succeeded
	err = os.Rename(tempLibFile, libFile)
	if err != nil {
		fmt.Printf("❌ Error updating library: %v\n", err)
		os.Remove(tempLibFile)
		return
	}

	elapsed := time.Since(start)
	fmt.Printf("✅ Recompiled in %v (library will auto-reload)\n", elapsed)
}

// compileAndRunCold compiles and runs for cold reload mode
func compileAndRunCold(sourceFile string, isInitial bool) {
	start := time.Now()

	// Get absolute path
	absPath, err := filepath.Abs(sourceFile)
	if err != nil {
		fmt.Printf("❌ Error resolving file path: %v\n", err)
		return
	}

	// Load the full package (handles multi-file programs). Validation is off:
	// this path never reads the diagnostics and goes straight to codegen.
	pm := NewPackageManager(filepath.Dir(absPath))
	pm.ValidateOnLoad = false
	pkg, err := pm.LoadPackageFromFile(absPath)
	if err != nil {
		fmt.Printf("❌ Error loading package: %v\n", err)
		return
	}

	// Resolve imports
	imports, err := resolveImports(pkg, pm, absPath)
	if err != nil {
		fmt.Printf("❌ Error resolving imports: %v\n", err)
		return
	}

	// Merge package with imports
	ast := MergeWithImports(pkg, imports)

	// Generate C code
	cCode := generateC(ast, sourceFile)
	if cCode == "" {
		fmt.Println("❌ Code generation failed")
		return
	}

	// Write C file
	outputDir := "output"
	os.MkdirAll(outputDir, 0755)

	baseName := filepath.Base(sourceFile)
	baseName = strings.TrimSuffix(baseName, filepath.Ext(baseName))
	outputFile := filepath.Join(outputDir, baseName+".c")
	executable := filepath.Join(outputDir, baseName)

	err = os.WriteFile(outputFile, []byte(cCode), 0644)
	if err != nil {
		fmt.Printf("❌ Error writing C file: %v\n", err)
		return
	}

	// Check for raylib from the full package
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

	var compiler string
	// Cold reload also honours SDL3 imports (same scan as main).
	hasSDL, sdlRoot := detectSDL(pkg, imports)
	// Box2D likewise (Task 5): includes + libbox2d.a on every cold link.
	hasBox2D, box2dRoot := detectBox2D(pkg, imports)

	var compileArgs []string

	// Use TCC for fast compilation in cold reload mode
	if !isInitial {
		fmt.Println("⚡ Recompiling with TCC...")
	}

	tccPath := findTCCCompiler()
	if tccPath == "" {
		fmt.Println("❌ TCC compiler not found, falling back to GCC")
		compiler = "gcc"
		compileArgs = []string{"-o", executable, outputFile, "-O0", "-g"}

		if hasRaylib {
			if raylibPath != "" {
				compileArgs = append(compileArgs, "-I"+raylibPath)
			}
			raylibArg, needsLibPath := raylibStaticLinkArg(raylibPath)
			if needsLibPath && raylibPath != "" {
				compileArgs = append(compileArgs, "-L"+raylibPath)
			}
			compileArgs = append(compileArgs, raylibArg, "-lm", "-lpthread", "-ldl", "-lrt", "-lX11")
		} else {
			compileArgs = append(compileArgs, "-lm")
			compileArgs = append(compileArgs, pthreadFlags()...)
		}
	} else {
		compiler = tccPath
		compileArgs = []string{"-o", executable, outputFile}

		if hasRaylib {
			if raylibPath != "" {
				compileArgs = append(compileArgs, "-I"+raylibPath)
			}
			raylibArg, needsLibPath := raylibStaticLinkArg(raylibPath)
			if needsLibPath && raylibPath != "" {
				compileArgs = append(compileArgs, "-L"+raylibPath)
			}
			compileArgs = append(compileArgs, raylibArg, "-lm", "-lpthread", "-ldl")
		} else {
			compileArgs = append(compileArgs, "-lm")
			compileArgs = append(compileArgs, pthreadFlags()...)
		}
	}
	if hasSDL {
		compileArgs = append(compileArgs, sdlLinkArgs(sdlRoot)...)
	}
	if hasBox2D {
		compileArgs = append(compileArgs, box2dLinkArgs(box2dRoot)...)
	}

	compileArgs = withDefines(compileArgs)
	cmd := exec.Command(compiler, compileArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("❌ Compilation failed:\n%s\n", output)
		return
	}

	elapsed := time.Since(start)
	fmt.Printf("✅ Compiled in %v\n", elapsed)

	// Run the program
	if isInitial {
		fmt.Println("🚀 Running program...")
	} else {
		fmt.Println("🔄 Restarting program...")
	}
	fmt.Println("==================")

	runCmd := exec.Command(executable)
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	err = runCmd.Run()

	fmt.Println("==================")
	if err != nil {
		fmt.Printf("⚠️  Program exited with error: %v\n", err)
	} else {
		fmt.Println("✅ Program exited successfully")
	}
	fmt.Println("📝 Watching for changes...")
}

// findTCCCompiler finds the TCC compiler
func findTCCCompiler() string {
	// Try system PATH first
	if path, err := exec.LookPath("tcc"); err == nil {
		return path
	}

	// Try local tcc directory
	localPaths := []string{
		"./tcc/tcc",
		"../tcc/tcc",
		"../../tcc/tcc",
	}

	for _, path := range localPaths {
		if _, err := os.Stat(path); err == nil {
			absPath, _ := filepath.Abs(path)
			return absPath
		}
	}

	return ""
}
