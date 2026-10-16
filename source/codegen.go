package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"ahoy"
)

// CrashLineTracking controls whether generated C records the current Ahoy
// file/line/function in the __ahoy_file/__ahoy_line/__ahoy_func globals
// before each statement. The crash handler reads them back to report the
// exact Ahoy location of a segfault/abort instead of a generic message.
// The stores are volatile (never reordered away) and cost ~2 stores per
// statement, so main.go disables this for -release builds (zero overhead)
// while dev builds (tcc, hot/cold reload) keep it on.
var CrashLineTracking = true

func snakeToPascal(s string) string {
	// If there are no underscores, return as-is (it's already in the correct format)
	if !strings.Contains(s, "_") {
		return s
	}

	// Common acronyms that should be uppercase
	acronyms := map[string]string{
		"fps":   "FPS",
		"api":   "API",
		"url":   "URL",
		"http":  "HTTP",
		"https": "HTTPS",
		"rgb":   "RGB",
		"rgba":  "RGBA",
		"gpu":   "GPU",
		"cpu":   "CPU",
		"ui":    "UI",
		"id":    "ID",
		"uuid":  "UUID",
	}

	parts := strings.Split(s, "_")
	for i, part := range parts {
		if len(part) > 0 {
			// Check if this part is a known acronym
			if acronymUpper, ok := acronyms[strings.ToLower(part)]; ok {
				parts[i] = acronymUpper
			} else {
				// Normal word - capitalize first letter
				parts[i] = strings.ToUpper(string(part[0])) + part[1:]
			}
		}
	}
	return strings.Join(parts, "")
}

// isScreamingSnakeCase checks if a string is in SCREAMING_SNAKE_CASE (all uppercase with underscores)
func isScreamingSnakeCase(s string) bool {
	if len(s) == 0 {
		return false
	}

	// Must have at least one uppercase letter
	hasUpper := false
	for _, ch := range s {
		if ch >= 'A' && ch <= 'Z' {
			hasUpper = true
		} else if ch >= 'a' && ch <= 'z' {
			// Has lowercase, not screaming snake case
			return false
		} else if ch != '_' && (ch < '0' || ch > '9') {
			// Has character that's not uppercase, underscore, or digit
			return false
		}
	}

	return hasUpper
}

// escapeRawString converts a raw string (with actual newlines) to a C string literal
// Newlines become \n, backslashes become \\, etc.
func escapeRawString(s string) string {
	var result strings.Builder
	for _, ch := range s {
		switch ch {
		case '\n':
			result.WriteString("\\n")
		case '\r':
			result.WriteString("\\r")
		case '\t':
			result.WriteString("\\t")
		case '\\':
			result.WriteString("\\\\")
		case '"':
			result.WriteString("\\\"")
		default:
			result.WriteRune(ch)
		}
	}
	return result.String()
}

// toCStructName converts a struct name to a valid C identifier
// e.g., "Card.Assassin" -> "Card_Assassin", "player_info" -> "Player_info", "particle.wind_particle" -> "Particle_Wind_particle"
func toCStructName(name string) string {
	// Split by dots for nested types
	if strings.Contains(name, ".") {
		parts := strings.Split(name, ".")
		for i := range parts {
			parts[i] = capitalizeFirst(parts[i])
		}
		return strings.Join(parts, "_")
	}
	// Simple type name - just capitalize first letter
	return capitalizeFirst(name)
}

// toCFuncName converts a name to a valid C function name (lowercase with underscores)
// e.g., "Card.Assassin" -> "card_assassin"
func toCFuncName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, ".", "_"))
}

type StructField struct {
	Name         string
	Type         string // C type after mapType
	AhoyType     string // Ahoy-level type before mapType, e.g. fn(int)->int
	DefaultValue string // C code for default value (if any)
	IsStatic     bool   // Static field (shared across instances)
	IsConst      bool   // Const field (SCREAMING_SNAKE_CASE - immutable)
}

type StructInfo struct {
	Name   string
	Fields []StructField
}

type CodeGenerator struct {
	output                        strings.Builder
	indent                        int
	varCounter                    int
	dictCounter                   int             // Counter for inline dict/array literals
	funcReturnStructs             strings.Builder // Struct definitions for multi-return functions
	funcForwardDecls              strings.Builder // Forward declarations for user functions
	funcDecls                     strings.Builder
	constantDecls                 strings.Builder // Global constant declarations
	enumDecls                     strings.Builder // Enum type declarations
	structDecls                   strings.Builder
	includes                      map[string]bool
	orderedIncludes               []string                                // Keep track of include order
	variables                     map[string]string                       // variable name -> type (global scope)
	functionVars                  map[string]string                       // variable name -> type (function scope)
	nestedScopeVars               map[string]bool                         // variables declared in nested scopes (loops/ifs)
	varDeclIndent                 map[string]int                          // variable name -> indent level where it was declared
	constants                     map[string]string                       // constant name -> type
	declaredConstants             map[string]bool                         // constant name -> declared (for duplicate check)
	enums                         map[string]map[string]bool              // enum name -> {member names}
	enumOriginalNames             map[string]bool                         // tracks original (lowercase) enum names vs aliases
	enumMemberTypes               map[string]string                       // "enumName.memberName" -> type
	enumTypes                     map[string]string                       // enum name -> enum type (int, string, etc.)
	userFunctions                 map[string]bool                         // user-defined function names (keep snake_case)
	calledFunctions               map[string]bool                         // functions that are actually called in the program
	hasError                      bool                                    // Track if error occurred
	arrayImpls                    bool                                    // Track if we've added array implementation
	arrayMethods                  map[string]bool                         // Track which array methods are used
	stringMethods                 map[string]bool                         // Track which string methods are used
	dictMethods                   map[string]bool                         // Track which dict methods are used
	useJSON                       bool                                    // Track if JSON functions are used
	useJobs                       bool                                    // Track if the job system is used
	jobResultTypes                map[string]string                       // job variable -> task result Ahoy type
	jsonVariables                 map[string]bool                         // Track which variables hold JSON data
	jsonStructs                   map[string]bool                         // Track which structs are JSON schemas (not real C structs)
	loopCounters                  []string                                // Stack of loop counter variable names
	currentFile                   string                                  // Source file of the top-level node being generated
	crashPathRoot                 string                                  // Longest common dir of all sources + trailing sep; stripped from crash-location paths so release binaries don't leak absolute build paths
	currentFunction               string                                  // Current function being generated
	currentAhoyFunction           string                                  // Ahoy-level name of currentFunction (for crash reports; currentFunction holds the C name)
	currentFunctionReturnType     string                                  // Return type of current function
	currentFunctionHasMultiReturn bool                                    // Whether current function has multiple returns
	hasMainFunc                   bool                                    // Whether there's an Ahoy main function
	arrayElementTypes             map[string]string                       // array variable name -> element type
	distinctTypes                map[string]string                       // distinct newtype name -> base Ahoy type
	reportedElemMismatch          map[string]bool                         // arrays already flagged for tracked-vs-declared element disagreement (dedupes the internal error)
	dictLoopValueExpr             map[string]string                       // dict-loop value var -> C expression that formats it by its runtime type
	array2DElementTypes           map[string]string                       // 2D array variable name -> nested element type (element type of inner arrays)
	structs                       map[string]*StructInfo                  // struct name -> struct info
	currentTypeContext            string                                  // Current type annotation context (e.g., "array[int]")
	functionReturnTypes           map[string][]string                     // function name -> return types (for inferred functions)
	methodTable                   map[string]map[string]*ahoy.MethodEntry // receiver type -> method -> user method
	deferredStatements            []deferredStmt                          // Stack of deferred statements for current function
	functionParamTypes            map[string][]string                     // function name -> parameter types
	functionParamNames            map[string][]string                     // function name -> parameter names
	functionParamDefaults         map[string][]*ahoy.ASTNode              // function name -> parameter default values
	closureDecls                  strings.Builder                         // hoisted thunks for lambdas and function references
	funcRefThunks                 map[string]bool                         // function names whose reference thunk has been emitted
	closureCaptures               map[string]string                       // captured name -> C expression while generating a thunk body
	pendingFuncType               string                                  // declared fn(...) type of the lambda being initialized
	pendingCType                  string                                  // declared C callback typedef of the value being assigned
	dictSourcedVars               map[string]string                       // variable name -> dict name (for dict-accessed vars)
	dictSourcedKeys               map[string]string                       // variable name -> key (for dict-accessed vars)
	generatedTypes                map[string]bool                         // Track which types have been generated
	cFunctionNames                map[string]string                       // snake_case name -> actual C name
	cNamespaces                   map[string]map[string]string            // namespace -> (snake_case name -> actual C name)
	cFunctionReturnTypes          map[string]string                       // C function name (snake_case) -> return type
	cNamespaceReturnTypes         map[string]map[string]string            // namespace -> (snake_case name -> return type)
	cTypeDefinitions              map[string]bool                         // Track known C types from headers
	declaredGlobalVars            map[string]bool                         // Track global variables that have been declared in C code
	declaredFunctionVars          map[string]bool                         // Track function-local variables that have been declared in C code
	scopeDeclared                 []map[string]bool                       // Stack of open blocks -> names declared in each
	enableBoundsChecking          bool                                    // Enable runtime array bounds checking
	enableSignalHandler           bool                                    // Enable signal handler for crash reporting
	skipBoundsCheck               bool                                    // Temporarily skip bounds check (for lvalue contexts)
	sourceFilename                string                                  // Source filename for error messages
	heapAllocatedVars             map[string]bool                         // Track heap-allocated variables in current function
	heapVarScopes                 map[string]int                          // Track scope depth where each heap var was allocated
	heapVarTypes                  map[string]string                       // Track type of each heap-allocated variable for proper freeing
	scopeDepth                    int                                     // Current scope depth (incremented for loops/ifs/blocks)
	scopeAllocations              map[int][]string                        // scope depth -> list of variables allocated at that scope
	loopScopeStack                []int                                   // Stack of scope depths where loops begin (for break/continue cleanup)
	loopEndLabels                 []loopEndLabel                          // User-loop end labels for halt-across-switch (parallel to loopScopeStack, minus generated inner traversals)
	pendingLoopEndLabel           string                                  // Label stashed by exitLoopScope, emitted by the loop tail (emitLoopEndLabel)
	switchCaseDepths              []int                                   // Scope depths of open switch-case bodies (halt needs to know a switch intervenes)
	escapingVars                  map[string]bool                         // Track variables that escape the function (returned or stored)
	deferredUsers                 map[string]bool                         // Vars mentioned in pending defer statements (must outlive them)
	aliasEdges                    map[string]map[string]int               // heap var -> aliases declared from it -> scope depth where declared
	manuallyFreedVars             map[string]bool                         // Track variables with manual defer free
	autoFreedVars                 map[string]bool                         // Track variables with automatic defer free
	functionParameters            map[string]bool                         // Track function parameters (never auto-free)
	cStructFields                 map[string]map[string]string            // C struct name -> (field name -> field type)
	cTypedefs                     map[string]string                       // typedef alias -> base type
	cFunctionParamTypes           map[string][]string                     // C function name (snake_case) -> parameter types
	cFuncPtrTypedefs              map[string]*ahoy.CFuncPtrTypedef        // C callback typedef name -> signature
	parsedCHeaders                map[string]*ahoy.CHeaderInfo            // In-memory cache for parsed C headers (path -> info)
	parsedCHeadersMu              sync.Mutex                              // Mutex for parsedCHeaders
	lambdaValues                  map[string]*ahoy.ASTNode                // var name -> lambda value node (for capture propagation)
	// valueBindings records every declaration/reassignment RHS by target
	// name (append-only like lambdaValues): markValueDeps walks it
	// transitively so escaping aliases protect their ultimate owners.
	valueBindings map[string]*ahoy.ASTNode // var name -> bound value node
	// funcBodies records free-function ASTs by name for alias-return
	// analysis (append-only pre-pass; methods/callees without bodies read
	// as fresh). callAliasCache memoizes the per-callee verdict.
	funcBodies            map[string]*ahoy.ASTNode // func name -> function node
	callAliasCache        map[string]callAliasInfo // func name -> alias verdict
	constantValues        map[string]interface{}   // constant name -> computed value (for const expressions)
	globalConstantGetters map[string]ConstantInfo  // constant name -> getter function info for hot reload
	// globalConstantGetterOrder records first-seen order so emission is
	// deterministic: ranging over the map above shuffles `_const_` lines on
	// every build. Source order also keeps any const-to-const reference
	// working, where alphabetical order might not.
	globalConstantGetterOrder []string
}

type ConstantInfo struct {
	Name       string
	CType      string
	AhoyType   string
	InitValue  string // The initialization expression
	IsComputed bool
	Value      interface{} // Computed value if available
}

// loopEndLabel tracks one user loop for halt-across-switch: a `halt`
// directly in a switch case cannot use C `break` (it would only exit the
// switch), so it jumps to the loop's end label instead.
type loopEndLabel struct {
	scopeDepth int    // loopScopeStack depth of this loop (cleanup boundary)
	label      string // C label after the loop ("" until a halt needs it)
}

func GenerateC(ast *ahoy.ASTNode) string {
	return generateC(ast, "<source>")
}

// GenerateCWithFilename generates C code from an AST with a source filename
func GenerateCWithFilename(ast *ahoy.ASTNode, filename string) string {
	return generateC(ast, filename)
}

func generateC(ast *ahoy.ASTNode, filename string) string {
	gen := &CodeGenerator{
		includes:              make(map[string]bool),
		orderedIncludes:       make([]string, 0),
		variables:             make(map[string]string),
		constants:             make(map[string]string),
		declaredConstants:     make(map[string]bool),
		enums:                 make(map[string]map[string]bool),
		enumOriginalNames:     make(map[string]bool),
		enumMemberTypes:       make(map[string]string),
		enumTypes:             make(map[string]string),
		userFunctions:         make(map[string]bool),
		calledFunctions:       make(map[string]bool),
		hasError:              false,
		arrayImpls:            false,
		arrayMethods:          make(map[string]bool),
		stringMethods:         make(map[string]bool),
		dictMethods:           make(map[string]bool),
		hasMainFunc:           false,
		arrayElementTypes:     make(map[string]string),
		distinctTypes:        make(map[string]string),
		array2DElementTypes:   make(map[string]string),
		structs:               make(map[string]*StructInfo),
		functionReturnTypes:   make(map[string][]string),
		functionParamTypes:    make(map[string][]string),
		functionParamNames:    make(map[string][]string),
		functionParamDefaults: make(map[string][]*ahoy.ASTNode),
		funcRefThunks:         make(map[string]bool),
		closureCaptures:       make(map[string]string),
		dictSourcedVars:       make(map[string]string),
		dictLoopValueExpr:     make(map[string]string),
		dictSourcedKeys:       make(map[string]string),
		nestedScopeVars:       make(map[string]bool),
		varDeclIndent:         make(map[string]int),
		cFunctionNames:        make(map[string]string),
		cNamespaces:           make(map[string]map[string]string),
		cFunctionReturnTypes:  make(map[string]string),
		cNamespaceReturnTypes: make(map[string]map[string]string),
		cTypeDefinitions:      make(map[string]bool),
		declaredGlobalVars:    make(map[string]bool),
		declaredFunctionVars:  make(map[string]bool),
		generatedTypes:        make(map[string]bool),
		jsonVariables:         make(map[string]bool),
		jsonStructs:           make(map[string]bool),
		enableBoundsChecking:  true, // Re-enabled with lvalue context handling
		enableSignalHandler:   true, // Enable by default for better error messages
		skipBoundsCheck:       false,
		sourceFilename:        filename, // Source file for error messages
		heapAllocatedVars:     make(map[string]bool),
		escapingVars:          make(map[string]bool),
		deferredUsers:         make(map[string]bool),
		aliasEdges:            make(map[string]map[string]int),
		manuallyFreedVars:     make(map[string]bool),
		autoFreedVars:         make(map[string]bool),
		loopScopeStack:        make([]int, 0),
		loopEndLabels:         make([]loopEndLabel, 0),
		cStructFields:         make(map[string]map[string]string),
		cTypedefs:             make(map[string]string),
		cFunctionParamTypes:   make(map[string][]string),
		cFuncPtrTypedefs:      make(map[string]*ahoy.CFuncPtrTypedef),
		jobResultTypes:        make(map[string]string),
		parsedCHeaders:        make(map[string]*ahoy.CHeaderInfo),
		// lambdaValues is append-only (never reset per function): per-function
		// sequential generation keeps each function's entries current while
		// it generates, and stale entries are harmless (their names are no
		// longer heap-tracked, so the membership check skips them).
		lambdaValues:          make(map[string]*ahoy.ASTNode),
		valueBindings:         make(map[string]*ahoy.ASTNode),
		funcBodies:            make(map[string]*ahoy.ASTNode),
		callAliasCache:        make(map[string]callAliasInfo),
		constantValues:        make(map[string]interface{}),
		globalConstantGetters: make(map[string]ConstantInfo),
	}
	// Add standard includes
	gen.includes["stdio.h"] = true
	gen.orderedIncludes = append(gen.orderedIncludes, "stdio.h")
	gen.includes["stdlib.h"] = true
	gen.orderedIncludes = append(gen.orderedIncludes, "stdlib.h")
	gen.includes["string.h"] = true
	gen.orderedIncludes = append(gen.orderedIncludes, "string.h")
	gen.includes["stdbool.h"] = true
	gen.orderedIncludes = append(gen.orderedIncludes, "stdbool.h")
	gen.includes["stdint.h"] = true
	gen.orderedIncludes = append(gen.orderedIncludes, "stdint.h")
	gen.includes["assert.h"] = true
	gen.orderedIncludes = append(gen.orderedIncludes, "assert.h")

	// Generate hash map implementation
	gen.writeHashMapImplementation()

	// First pass: scan imports to populate C type definitions BEFORE code generation
	gen.scanImports(ast)

	// Second pass: scan all enums, structs, and constants for forward declaration
	gen.scanTypeDeclarations(ast)

	// Method table: user-defined methods (@ (recv T) name, @ T.name) keyed by
	// receiver type, built once from the whole (possibly merged) program AST.
	gen.methodTable = ahoy.CollectMethods(ast)

	// Third pass: check if there's a main function and collect function signatures
	gen.checkForMainFunction(ast)

	// Fourth pass: infer return types for all functions with infer keyword
	gen.inferAllFunctionReturnTypes(ast)

	// Fourth-and-a-half pass: record every function's signature. A `&name`
	// reference to a function defined later in the file needs the parameter
	// types now; recording them only while generating that function's own body
	// produced thunks with no parameters at all, which the C compiler rejected.
	gen.scanFunctionSignatures(ast)

	// Fifth pass: scan variable declarations to populate type information
	// Imported C headers first: the variable-type pre-pass needs to know a
	// callback typedef (LogCallback) before it types a variable holding one.
	gen.scanCHeaderTypes(ast)
	gen.scanVariableTypes(ast)

	// Crash locations are reported relative to the longest common directory
	// of the program's sources, so shipped binaries don't embed absolute
	// build-machine paths (and reports read as engine/core.ahoy:4708).
	gen.crashPathRoot = commonSourceRoot(ast, filename)

	// Sixth pass: infer parameter types from function call sites
	gen.inferParameterTypesFromCalls(ast)

	// Sixth pass: scan for method calls to determine which helper functions we need
	gen.scanForMethodCalls(ast)

	// Sixth-and-a-half pass: scan for function calls to track which functions are actually used
	gen.scanForFunctionCalls(ast)

	// Seventh pass: Generate all type declarations (enums, structs) first
	gen.generateTypeDeclarations(ast)

	// Record function bodies for alias-return analysis (passthrough calls):
	// a callee returning its parameter/global/field aliases it, so callers
	// must not track the result as fresh. Pre-pass covers forward refs.
	gen.collectFuncBodies(ast)

	// Generate main code (skipping type declarations since they're already generated)
	gen.generateNode(ast)

	// Check if there were any errors
	if gen.hasError {
		return "" // Return empty string to indicate error
	}

	// Generate type helper function if needed
	gen.writeTypeEnumToStringHelper()

	// Generate built-in type helpers (color, vector2)
	gen.writeBuiltinTypeHelpers()

	// Generate array helper functions if any array methods were used
	gen.writeArrayHelperFunctions()

	// Generate dict helper functions if any dict methods were used
	gen.writeDictHelperFunctions()

	// Generate string helper functions if any string methods were used
	gen.writeStringHelperFunctions()

	// Generate struct print helper functions
	gen.writeStructHelperFunctions()

	// Generate vector2 and color constructors
	gen.writeTypeConstructors()

	// Generate JSON helper functions if JSON is used
	gen.writeJSONHelperFunctions()

	// Build final output
	var result strings.Builder

	// Fix raylib/raymath include order - raylib.h must come before raymath.h
	// to avoid duplicate Vector2/Vector3/Vector4/Matrix definitions
	// raylib.h defines RL_VECTOR2_TYPE which raymath.h checks before defining types
	gen.reorderRaylibIncludes()

	// Write includes
	for _, include := range gen.orderedIncludes {
		// Use angle brackets for system includes, quotes for local .h files
		if strings.HasSuffix(include, ".h") && (strings.HasPrefix(include, "/") || strings.HasPrefix(include, ".")) {
			result.WriteString("#include \"")
			result.WriteString(include)
			result.WriteString("\"\n")
		} else {
			result.WriteString("#include <")
			result.WriteString(include)
			result.WriteString(">\n")
		}
	}
	result.WriteString("\n")

	// Write signal handler if enabled
	if gen.enableSignalHandler {
		result.WriteString(gen.getSignalHandler())
		result.WriteString("\n")
	}

	// Write array implementation if needed (or if JSON needs it)
	if gen.arrayImpls || gen.useJSON {
		result.WriteString(gen.getArrayImplementation())
		result.WriteString("\n")
	}

	// Write hash map declarations
	result.WriteString(gen.getHashMapDeclarations())
	result.WriteString("\n")

	// Write AhoyValueType enum (needed by both HashMap and AhoyArray)
	result.WriteString("\n// Value type tracking\n")
	result.WriteString("typedef enum {\n")
	result.WriteString("    AHOY_TYPE_INT,\n")
	result.WriteString("    AHOY_TYPE_STRING,\n")
	result.WriteString("    AHOY_TYPE_FLOAT,\n")
	result.WriteString("    AHOY_TYPE_CHAR,\n")
	result.WriteString("    AHOY_TYPE_STRUCT\n")
	result.WriteString("} AhoyValueType;\n\n")

	// Function values: a code pointer plus an optional captured environment.
	// Non-capturing values (named function references, lambdas with no captured
	// locals) leave env NULL.
	result.WriteString("// Function values\n")
	result.WriteString("typedef struct {\n")
	result.WriteString("    void (*fn)(void);\n")
	result.WriteString("    void* env;\n")
	result.WriteString("} AhoyFn;\n\n")
	result.WriteString("// Job system (implemented after the user functions when used)\n")
	result.WriteString("typedef struct AhoyPool AhoyPool;\n")
	result.WriteString("typedef struct AhoyJob AhoyJob;\n")
	result.WriteString("AhoyPool* ahoy_jobs_new(int count);\n")
	result.WriteString("AhoyJob* ahoy_jobs_submit(AhoyPool* pool, AhoyFn fn, intptr_t arg);\n")
	result.WriteString("intptr_t ahoy_jobs_wait(AhoyJob* job);\n")
	result.WriteString("intptr_t ahoy_jobs_result(AhoyJob* job);\n")
	result.WriteString("int ahoy_jobs_done(AhoyJob* job);\n")
	result.WriteString("int ahoy_jobs_drain(AhoyPool* pool, AhoyFn handler, intptr_t context);\n")
	result.WriteString("void ahoy_jobs_stop(AhoyPool* pool);\n")
	result.WriteString("void ahoy_jobs_destroy(AhoyPool* pool);\n\n")

	// Write AhoyArray struct definition if arrays are used (must come after AhoyValueType)
	if gen.arrayImpls || len(gen.arrayMethods) > 0 {
		result.WriteString("// Array Helper Structure\n")
		result.WriteString("typedef struct {\n")
		result.WriteString("    intptr_t* data;\n")
		result.WriteString("    AhoyValueType* types;  // Type for each element\n")
		result.WriteString("    int length;\n")
		result.WriteString("    int capacity;\n")
		result.WriteString("    int is_typed;  // 0 = mixed types allowed, 1 = single type enforced\n")
		result.WriteString("    AhoyValueType element_type;  // If is_typed=1, this is the enforced type\n")
		result.WriteString("} AhoyArray;\n\n")

		// Add forward declarations for array helper functions
		if gen.arrayMethods["push"] {
			result.WriteString("AhoyArray* ahoy_array_push(AhoyArray* arr, intptr_t value, AhoyValueType type);\n")
		}
		if gen.arrayMethods["pop"] {
			result.WriteString("intptr_t ahoy_array_pop(AhoyArray* arr);\n")
		}
		if gen.arrayMethods["length"] {
			result.WriteString("int ahoy_array_length(AhoyArray* arr);\n")
		}
		if gen.arrayMethods["fill"] {
			result.WriteString("AhoyArray* ahoy_array_fill(AhoyArray* arr, intptr_t value, AhoyValueType type, int count);\n")
		}
		if gen.arrayMethods["remove"] {
			result.WriteString("AhoyArray* ahoy_array_remove(AhoyArray* arr, int index);\n")
		}
		result.WriteString("char* print_array_helper(AhoyArray* arr);\n")
		result.WriteString("AhoyArray* ahoy_string_split_array(const char* str, const char* delim);\n")
		result.WriteString("\n")
	}

	// String concatenation (+ on two strings) is emitted on demand during
	// codegen, which runs after this preamble; the prototype is unconditional
	// so string-only programs (no arrays) still see a declaration.
	result.WriteString("char* ahoy_string_concat(const char* a, const char* b);\n")

	// Forward declarations for the string helpers. The helpers are written
	// after all user functions, so a call site sees only an implicit
	// declaration: harmless for int returns, a hard error for bool and
	// pointer returns (the implicit int conflicts with the definition).
	result.WriteString("char* ahoy_string_dup(const char* src);\n")
	result.WriteString("int ahoy_string_length(const char* str);\n")
	result.WriteString("char* ahoy_string_upper(const char* str);\n")
	result.WriteString("char* ahoy_string_lower(const char* str);\n")
	result.WriteString("char* ahoy_string_replace(const char* str, const char* old, const char* new_str);\n")
	result.WriteString("bool ahoy_string_contains(const char* str, const char* substr);\n")
	result.WriteString("bool ahoy_string_match(const char* str, const char* pattern);\n")
	result.WriteString("int ahoy_string_count(const char* str, const char* substr);\n")
	result.WriteString("char* ahoy_string_strip(const char* str);\n")
	result.WriteString("char* ahoy_string_camel_case(const char* str);\n")
	result.WriteString("char* ahoy_string_snake_case(const char* str);\n")
	result.WriteString("char* ahoy_string_pascal_case(const char* str);\n")
	result.WriteString("char* ahoy_string_kebab_case(const char* str);\n")
	result.WriteString("char* ahoy_string_lpad(const char* str, int length, const char* pad);\n")
	result.WriteString("char* ahoy_string_rpad(const char* str, int length, const char* pad);\n")
	result.WriteString("char* ahoy_string_pad(const char* str, int length, const char* pad);\n")
	result.WriteString("char* ahoy_string_get_file(const char* path);\n")

	// Add forward declarations for dict helper functions if needed
	if gen.dictMethods["print_dict"] {
		result.WriteString("char* print_dict_helper(HashMap* dict);\n")
		result.WriteString("char* format_hashmap_value(HashMap* dict, const char* key);\n")
	}

	// Write enum declarations (typedefs)
	if gen.enumDecls.Len() > 0 {
		result.WriteString("// Enum declarations\n")
		result.WriteString(gen.enumDecls.String())
		result.WriteString("\n")
	}

	// Write struct declarations (typedefs)
	result.WriteString(gen.structDecls.String())
	result.WriteString("\n")

	// Write function return struct definitions (for multi-return functions)
	if gen.funcReturnStructs.Len() > 0 {
		result.WriteString("// Return type structs for multi-return functions\n")
		result.WriteString(gen.funcReturnStructs.String())
		result.WriteString("\n")
	}

	// Write function forward declarations
	if gen.funcForwardDecls.Len() > 0 {
		result.WriteString("// User function forward declarations\n")
		result.WriteString(gen.funcForwardDecls.String())
		result.WriteString("\n")
	}

	// Write hoisted function values: thunks for &name references and lambdas.
	// They call user functions, whose forward declarations are directly above.
	if gen.closureDecls.Len() > 0 {
		result.WriteString("// Function values (lambdas and function references)\n")
		result.WriteString(gen.closureDecls.String())
		result.WriteString("\n")
	}

	// Write constant getter functions for hot reload
	if len(gen.globalConstantGetters) > 0 {
		result.WriteString("// Hot-reloadable constant getters\n")
		result.WriteString("// These allow constants to update during hot reload\n\n")

		// Generate static storage for constant values
		result.WriteString("// Internal storage for constant values\n")
		for _, name := range gen.globalConstantGetterOrder {
			info := gen.globalConstantGetters[name]
			result.WriteString("static ")
			result.WriteString(info.CType)
			result.WriteString(" _const_")
			result.WriteString(name)
			result.WriteString(" = ")
			result.WriteString(info.InitValue)
			result.WriteString(";\n")
		}
		result.WriteString("\n")

		// Generate getter functions
		result.WriteString("// Getter functions for constants\n")
		for _, name := range gen.globalConstantGetterOrder {
			info := gen.globalConstantGetters[name]
			result.WriteString("static inline ")
			result.WriteString(info.CType)
			result.WriteString(" get_")
			result.WriteString(name)
			result.WriteString("(void) { return _const_")
			result.WriteString(name)
			result.WriteString("; }\n")
		}

		// Generate update function for hot reload
		result.WriteString("\n// Update function for hot reload\n")
		result.WriteString("void ahoy_update_constants(void) {\n")
		for _, name := range gen.globalConstantGetterOrder {
			info := gen.globalConstantGetters[name]
			result.WriteString("    _const_")
			result.WriteString(name)
			result.WriteString(" = ")
			result.WriteString(info.InitValue)
			result.WriteString(";\n")
		}
		result.WriteString("}\n\n")
	}

	// Write global constants before functions
	if gen.constantDecls.Len() > 0 {
		result.WriteString("// Global constants\n")
		result.WriteString(gen.constantDecls.String())
		result.WriteString("\n")
	}

	// Write function implementations
	result.WriteString(gen.funcDecls.String())
	result.WriteString("\n")

	// Write the job system runtime only when a pool is created
	if gen.useJobs {
		result.WriteString(getJobRuntime())
		result.WriteString("\n")
	}

	// Write main program. This scaffolding is not user code, so point the
	// diagnostic location back at the generated file rather than leaving it on
	// whichever source line happened to be emitted last.
	if gen.hasMainFunc {
		// If there's an Ahoy main function, just call it
		result.WriteString("#line 1 \"<ahoy-generated>\"\n")
		result.WriteString("int main() {\n")
		if gen.enableSignalHandler {
			result.WriteString("    ahoy_setup_signal_handlers();\n")
		}
		result.WriteString("    ahoy_main();\n")
		result.WriteString("    return 0;\n")
		result.WriteString("}\n")
	} else {
		// Legacy: no main function, use global scope code
		result.WriteString("#line 1 \"<ahoy-generated>\"\n")
		result.WriteString("int main() {\n")
		if gen.enableSignalHandler {
			result.WriteString("    ahoy_setup_signal_handlers();\n")
		}
		result.WriteString(gen.output.String())
		result.WriteString("    return 0;\n")
		result.WriteString("}\n")
	}

	return result.String()
}

func (gen *CodeGenerator) getArrayImplementation() string {
	return `
// Dynamic Array Implementation
typedef struct {
    void** data;
    int size;
    int capacity;
} DynamicArray;

DynamicArray* createArray(int initialCapacity) {
    DynamicArray* arr = malloc(sizeof(DynamicArray));
    arr->data = malloc(sizeof(void*) * initialCapacity);
    arr->size = 0;
    arr->capacity = initialCapacity;
    return arr;
}

void arrayPush(DynamicArray* arr, void* value) {
    if (arr->size >= arr->capacity) {
        arr->capacity *= 2;
        arr->data = realloc(arr->data, sizeof(void*) * arr->capacity);
    }
    arr->data[arr->size++] = value;
}

void* arrayGet(DynamicArray* arr, int index) {
    if (index >= 0 && index < arr->size) {
        return arr->data[index];
    }
    return NULL;
}

void arraySet(DynamicArray* arr, int index, void* value) {
    if (index >= 0 && index < arr->size) {
        arr->data[index] = value;
    }
}

void freeArray(DynamicArray* arr) {
    free(arr->data);
    free(arr);
}
`
}

func (gen *CodeGenerator) writeHashMapImplementation() {
	hashMapCode := `
// Hash Map Implementation with type tracking

typedef struct HashMapEntry {
    char* key;
    void* value;
    AhoyValueType valueType;
    struct HashMapEntry* next;
} HashMapEntry;

typedef struct HashMap {
    HashMapEntry** buckets;
    int size;
    int capacity;
} HashMap;

unsigned int hash(const char* key) {
    unsigned int hash = 5381;
    int c;
    while ((c = *key++)) {
        hash = ((hash << 5) + hash) + c;
    }
    return hash;
}

HashMap* createHashMap(int capacity) {
    HashMap* map = malloc(sizeof(HashMap));
    map->capacity = capacity;
    map->size = 0;
    map->buckets = calloc(capacity, sizeof(HashMapEntry*));
    return map;
}

void hashMapPutTyped(HashMap* map, const char* key, void* value, AhoyValueType valueType) {
    unsigned int index = hash(key) % map->capacity;
    HashMapEntry* entry = map->buckets[index];

    while (entry != NULL) {
        if (strcmp(entry->key, key) == 0) {
            entry->value = value;
            entry->valueType = valueType;
            return;
        }
        entry = entry->next;
    }

    HashMapEntry* newEntry = malloc(sizeof(HashMapEntry));
    newEntry->key = strdup(key);
    newEntry->value = value;
    newEntry->valueType = valueType;
    newEntry->next = map->buckets[index];
    map->buckets[index] = newEntry;
    map->size++;
}

void hashMapPut(HashMap* map, const char* key, void* value) {
    hashMapPutTyped(map, key, value, AHOY_TYPE_STRING);
}

void* hashMapGet(HashMap* map, const char* key) {
    unsigned int index = hash(key) % map->capacity;
    HashMapEntry* entry = map->buckets[index];

    while (entry != NULL) {
        if (strcmp(entry->key, key) == 0) {
            return entry->value;
        }
        entry = entry->next;
    }
    return NULL;
}

// Get value with automatic type conversion - dereferences floats to actual double bits
intptr_t hashMapGetTyped(HashMap* map, const char* key) {
    unsigned int index = hash(key) % map->capacity;
    HashMapEntry* entry = map->buckets[index];

    while (entry != NULL) {
        if (strcmp(entry->key, key) == 0) {
            // For floats, dereference the pointer and return as bits in intptr_t
            if (entry->valueType == AHOY_TYPE_FLOAT) {
                union { double d; intptr_t i; } u;
                u.d = *(double*)entry->value;
                return u.i;
            }
            // For other types, return the value as-is
            return (intptr_t)(entry->value);
        }
        entry = entry->next;
    }
    return 0;
}

// Get value as double (for arithmetic operations and generic access)
double hashMapGetDouble(HashMap* map, const char* key) {
    unsigned int index = hash(key) % map->capacity;
    HashMapEntry* entry = map->buckets[index];

    while (entry != NULL) {
        if (strcmp(entry->key, key) == 0) {
            switch (entry->valueType) {
                case AHOY_TYPE_INT:
                    return (double)(intptr_t)entry->value;
                case AHOY_TYPE_FLOAT:
                    return *(double*)entry->value;
                case AHOY_TYPE_STRING:
                    // For strings, return the pointer cast to double (for later casting back)
                    return (double)(intptr_t)entry->value;
                default:
                    return (double)(intptr_t)entry->value;
            }
        }
        entry = entry->next;
    }
    return 0.0;
}

// Helper to print dict values with proper type handling
char* format_dict_value(HashMap* map, const char* key) {
    unsigned int index = hash(key) % map->capacity;
    HashMapEntry* entry = map->buckets[index];
    static char buffer[256];

    while (entry != NULL) {
        if (strcmp(entry->key, key) == 0) {
            switch (entry->valueType) {
                case AHOY_TYPE_INT:
                    sprintf(buffer, "%ld", (long)(intptr_t)entry->value);
                    break;
                case AHOY_TYPE_FLOAT:
                    sprintf(buffer, "%g", *(double*)entry->value);
                    break;
                case AHOY_TYPE_STRING:
                    sprintf(buffer, "%s", (char*)entry->value);
                    break;
                case AHOY_TYPE_CHAR:
                    sprintf(buffer, "%c", (char)(intptr_t)entry->value);
                    break;
                default:
                    sprintf(buffer, "%ld", (long)(intptr_t)entry->value);
            }
            return buffer;
        }
        entry = entry->next;
    }
    return "";
}

void freeHashMap(HashMap* map) {
    for (int i = 0; i < map->capacity; i++) {
        HashMapEntry* entry = map->buckets[i];
        while (entry != NULL) {
            HashMapEntry* temp = entry;
            entry = entry->next;
            free(temp->key);
            // Free value if it's a heap-allocated float (double*)
            if (temp->valueType == AHOY_TYPE_FLOAT && temp->value != NULL) {
                free(temp->value);
            }
            free(temp);
        }
    }
    free(map->buckets);
    free(map);
}
`
	gen.funcDecls.WriteString(hashMapCode)
}

func (gen *CodeGenerator) getHashMapDeclarations() string {
	var decls strings.Builder
	decls.WriteString("\n// Forward declarations\n")
	decls.WriteString("typedef struct HashMapEntry HashMapEntry;\n")
	decls.WriteString("typedef struct HashMap HashMap;\n")
	decls.WriteString("HashMap* createHashMap(int capacity);\n")
	decls.WriteString("void hashMapPut(HashMap* map, const char* key, void* value);\n")
	decls.WriteString("void* hashMapGet(HashMap* map, const char* key);\n")
	decls.WriteString("intptr_t hashMapGetTyped(HashMap* map, const char* key);\n")
	decls.WriteString("double hashMapGetDouble(HashMap* map, const char* key);\n")
	decls.WriteString("char* format_dict_value(HashMap* map, const char* key);\n")
	decls.WriteString("void freeHashMap(HashMap* map);\n")

	return decls.String()
}

// checkForMainFunction scans the AST for a main function and registers all user functions
func (gen *CodeGenerator) checkForMainFunction(node *ahoy.ASTNode) {
	if node == nil {
		return
	}

	if node.Type == ahoy.NODE_FUNCTION {
		// Register this as a user-defined function
		funcName := node.Value
		gen.userFunctions[funcName] = true

		// Check if it's the main function
		if funcName == "main" {
			gen.hasMainFunc = true
		}
	}

	for _, child := range node.Children {
		gen.checkForMainFunction(child)
	}
}

// scanVariableTypes scans all variable declarations to populate type information
func (gen *CodeGenerator) scanVariableTypes(node *ahoy.ASTNode) {
	gen.scanVariableTypesInScope(node, false)
}

// scanVariableTypesInScope is the recursive worker behind scanVariableTypes.
// Declarations inside function bodies are deliberately NOT recorded in the
// global variables map: that map is a fallback for names no scope knows, and
// a local `k: string` in one function used to decide the type of an unrelated
// `k` (e.g. an int loop counter) in another function - producing atof() on an
// int and a segfault. Function bodies register their own locals in
// functionVars as codegen reaches them, so nothing is lost by skipping them
// here. (recordJobResultType still runs for every node either way.)
func (gen *CodeGenerator) scanVariableTypesInScope(node *ahoy.ASTNode, inFunction bool) {
	if node == nil {
		return
	}

	// First pass: collect function return types from explicit annotations
	if node.Type == ahoy.NODE_FUNCTION {
		// Everything under a function node is function scope, even though the
		// node itself is visited at top level.
		inFunction = true
		funcName := node.Value
		if node.DataType != "" && node.DataType != "infer" {
			// Store return type - check if it's a multi-return (contains comma)
			if strings.Contains(node.DataType, ",") {
				// Multi-return function
				types := strings.Split(node.DataType, ",")
				for i := range types {
					types[i] = strings.TrimSpace(types[i])
				}
				gen.functionReturnTypes[funcName] = types
			} else {
				// Single return
				gen.functionReturnTypes[funcName] = []string{node.DataType}
			}
		}
	}

	// Scan for variable declarations and track their types. Top-level (true
	// global) declarations go in the map; function-body locals are skipped
	// (see above) - except the job-result bookkeeping, which is scope-free.
	if node.Type == ahoy.NODE_VARIABLE_DECLARATION || node.Type == ahoy.NODE_ASSIGNMENT {
		varName := node.Value
		// This pass runs before the body is generated, so a submitted job's
		// result type has to be recorded here too: otherwise a later
		// `pool.wait|j|` is typed before the submit has been seen.
		gen.recordJobResultType(node)
		if !inFunction && len(node.Children) > 0 {
			// Check for explicit type annotation
			if node.DataType != "" && node.DataType != "generic" && node.DataType != "any" {
				gen.variables[varName] = node.DataType
			} else {
				// Try to infer from value
				valueType := gen.inferType(node.Children[0])
				if valueType != "unknown" && valueType != "generic" && valueType != "any" {
					gen.variables[varName] = valueType
				}
			}
		}
	}

	// Recursively scan children
	for _, child := range node.Children {
		gen.scanVariableTypesInScope(child, inFunction)
	}
}

// commonSourceRoot returns the longest common directory (with trailing
// separator) of the merged program's source files, or "" when there is no
// usable common root. Only absolute paths participate: relative ones are
// already leak-free and display as-is.
func commonSourceRoot(ast *ahoy.ASTNode, filename string) string {
	var dirs [][]string
	add := func(f string) {
		if f == "" || !filepath.IsAbs(f) {
			return
		}
		// Split into components for exact segment matching.
		clean := filepath.Clean(filepath.Dir(f))
		dirs = append(dirs, strings.Split(clean, string(filepath.Separator)))
	}
	add(filename)
	if ast != nil {
		for _, child := range ast.Children {
			add(child.File)
		}
	}
	if len(dirs) == 0 {
		return ""
	}
	common := dirs[0]
	for _, d := range dirs[1:] {
		n := len(common)
		if len(d) < n {
			n = len(d)
		}
		i := 0
		for i < n && common[i] == d[i] {
			i++
		}
		common = common[:i]
	}
	if len(common) == 0 {
		return ""
	}
	root := strings.Join(common, string(filepath.Separator))
	if root == "" || root == string(filepath.Separator) {
		return ""
	}
	return root + string(filepath.Separator)
}

// scanCHeaderTypes parses imported C headers before the variable-type
// pre-pass, registering callback typedefs and C type names. The generator
// caches parsed headers, so the real import processing reuses this work.
func (gen *CodeGenerator) scanCHeaderTypes(node *ahoy.ASTNode) {
	if node == nil {
		return
	}
	if node.Type == ahoy.NODE_IMPORT_STATEMENT {
		headerName := node.Value
		if strings.HasSuffix(headerName, ".h") {
			resolved := headerName
			if strings.HasPrefix(headerName, "./") || strings.HasPrefix(headerName, "../") {
				if abs, err := filepath.Abs(filepath.Join(filepath.Dir(gen.sourceFilename), headerName)); err == nil {
					if _, statErr := os.Stat(abs); statErr == nil {
						resolved = abs
					}
				}
			}
			headerPath := resolved
			if !strings.HasPrefix(resolved, "/") {
				for _, loc := range []string{resolved, "/usr/include/" + resolved, "/usr/local/include/" + resolved, "repos/raylib/src/" + resolved} {
					if _, err := os.Stat(loc); err == nil {
						headerPath = loc
						break
					}
				}
			}
			if info := gen.getOrParseCHeader(headerPath); info != nil {
				for name, td := range info.FuncPtrTypedefs {
					gen.cFuncPtrTypedefs[name] = td
				}
				for typeName := range info.Structs {
					gen.cTypeDefinitions[typeName] = true
					gen.cTypeDefinitions[strings.ToLower(typeName)] = true
				}
			}
		}
	}
	for _, child := range node.Children {
		gen.scanCHeaderTypes(child)
	}
}

// inferParameterTypesFromCalls analyzes function calls to infer parameter types
func (gen *CodeGenerator) inferParameterTypesFromCalls(node *ahoy.ASTNode) {
	if node == nil {
		return
	}

	// First collect all function definitions and their parameter names
	functionParams := make(map[string][]string)             // function name -> parameter names
	functionLocalVars := make(map[string]map[string]string) // function name -> (var name -> type)

	var collectFuncs func(*ahoy.ASTNode)
	collectFuncs = func(n *ahoy.ASTNode) {
		if n == nil {
			return
		}
		if n.Type == ahoy.NODE_FUNCTION {
			funcName := n.Value
			if len(n.Children) > 0 && n.Children[0].Type == ahoy.NODE_BLOCK {
				params := n.Children[0]
				paramNames := []string{}
				for _, param := range params.Children {
					if param.Type == ahoy.NODE_IDENTIFIER {
						paramNames = append(paramNames, param.Value)
					}
				}
				functionParams[funcName] = paramNames

				// Scan for local variable declarations in this function
				localVars := make(map[string]string)
				if len(n.Children) > 1 {
					var scanVarsInFunc func(*ahoy.ASTNode)
					scanVarsInFunc = func(node *ahoy.ASTNode) {
						if node == nil {
							return
						}
						if node.Type == ahoy.NODE_VARIABLE_DECLARATION || node.Type == ahoy.NODE_ASSIGNMENT {
							varName := node.Value
							if len(node.Children) > 0 {
								if node.DataType != "" && node.DataType != "generic" && node.DataType != "any" {
									localVars[varName] = node.DataType
								} else {
									// Try to infer from the right-hand side
									valueType := gen.inferType(node.Children[0])
									if valueType != "unknown" && valueType != "generic" && valueType != "any" {
										localVars[varName] = valueType
									}
								}
							}
						}
						for _, child := range node.Children {
							scanVarsInFunc(child)
						}
					}
					for i := 1; i < len(n.Children); i++ {
						scanVarsInFunc(n.Children[i])
					}
				}
				functionLocalVars[funcName] = localVars
			}
		}
		for _, child := range n.Children {
			collectFuncs(child)
		}
	}
	collectFuncs(node)

	// Now scan for function calls and infer parameter types from arguments
	paramTypeInferences := make(map[string]map[int]string) // function name -> param index -> inferred type

	var analyzeCalls func(*ahoy.ASTNode, string) // node, current function name
	analyzeCalls = func(n *ahoy.ASTNode, currentFunc string) {
		if n == nil {
			return
		}

		// Track when we enter a new function
		if n.Type == ahoy.NODE_FUNCTION {
			currentFunc = n.Value
		}

		if n.Type == ahoy.NODE_CALL {
			funcName := n.Value
			if paramNames, exists := functionParams[funcName]; exists {
				// This is a user-defined function
				if _, ok := paramTypeInferences[funcName]; !ok {
					paramTypeInferences[funcName] = make(map[int]string)
				}

				// Infer types from arguments
				for i, arg := range n.Children {
					if i < len(paramNames) {
						// Temporarily set functionVars to the local vars of current function
						// so inferType can resolve local variables
						savedFuncVars := gen.functionVars
						if currentFunc != "" {
							if localVars, ok := functionLocalVars[currentFunc]; ok {
								gen.functionVars = localVars
							}
						}

						argType := gen.inferType(arg)

						// Restore functionVars
						gen.functionVars = savedFuncVars

						// If we already have an inferred type for this parameter, check consistency
						if existingType, hasType := paramTypeInferences[funcName][i]; hasType {
							// If types differ and one is more specific (not generic/any/int), prefer the specific one
							if existingType != argType {
								if existingType == "int" || existingType == "generic" || existingType == "any" {
									paramTypeInferences[funcName][i] = argType
								}
								// Otherwise keep existing type if it's more specific
							}
						} else {
							paramTypeInferences[funcName][i] = argType
						}
					}
				}
			}
		}
		for _, child := range n.Children {
			analyzeCalls(child, currentFunc)
		}
	}
	analyzeCalls(node, "")

	// Apply inferred types to function parameters in the AST
	var applyTypes func(*ahoy.ASTNode)
	applyTypes = func(n *ahoy.ASTNode) {
		if n == nil {
			return
		}
		if n.Type == ahoy.NODE_FUNCTION {
			funcName := n.Value
			if inferences, exists := paramTypeInferences[funcName]; exists {
				if len(n.Children) > 0 && n.Children[0].Type == ahoy.NODE_BLOCK {
					params := n.Children[0]
					for i, param := range params.Children {
						if param.Type == ahoy.NODE_IDENTIFIER {
							// Only apply inferred type if parameter doesn't already have an explicit type
							if param.DataType == "" || param.DataType == "generic" || param.DataType == "any" {
								if inferredType, hasInference := inferences[i]; hasInference {
									// Don't override with generic/any/int/char* (char* is a fallback when type is unknown)
									if inferredType != "generic" && inferredType != "any" && inferredType != "int" && inferredType != "char*" {
										param.DataType = inferredType
									}
								}
							}
						}
					}
				}
			}
		}
		for _, child := range n.Children {
			applyTypes(child)
		}
	}
	applyTypes(node)
}

// getOrParseCHeader returns a cached C header or parses it (thread-safe)
func (gen *CodeGenerator) getOrParseCHeader(headerPath string) *ahoy.CHeaderInfo {
	// Check in-memory cache first (thread-safe)
	gen.parsedCHeadersMu.Lock()
	if cached, ok := gen.parsedCHeaders[headerPath]; ok {
		gen.parsedCHeadersMu.Unlock()
		return cached
	}
	gen.parsedCHeadersMu.Unlock()

	// Check file cache
	cache := GetBuildCache()
	if cachedInfo, ok := cache.GetCachedCHeader(headerPath); ok {
		gen.parsedCHeadersMu.Lock()
		gen.parsedCHeaders[headerPath] = cachedInfo
		gen.parsedCHeadersMu.Unlock()
		return cachedInfo
	}

	// Parse the header
	headerInfo, err := ahoy.ParseCHeader(headerPath)
	if err != nil {
		return nil
	}

	// Store in both caches
	cache.CacheCHeader(headerPath, headerInfo)
	gen.parsedCHeadersMu.Lock()
	gen.parsedCHeaders[headerPath] = headerInfo
	gen.parsedCHeadersMu.Unlock()

	return headerInfo
}

// scanImports scans imports to populate C type definitions before code generation
func (gen *CodeGenerator) scanImports(node *ahoy.ASTNode) {
	if node == nil {
		return
	}

	// First, collect all C header imports
	type headerImport struct {
		headerName string
		namespace  string
		index      int
	}
	var headerImports []headerImport

	var collectImports func(n *ahoy.ASTNode)
	collectImports = func(n *ahoy.ASTNode) {
		if n == nil {
			return
		}
		if n.Type == ahoy.NODE_IMPORT_STATEMENT && strings.HasSuffix(n.Value, ".h") {
			headerImports = append(headerImports, headerImport{
				headerName: n.Value,
				namespace:  n.DataType,
				index:      len(headerImports),
			})
		}
		for _, child := range n.Children {
			collectImports(child)
		}
	}
	collectImports(node)

	if len(headerImports) == 0 {
		return
	}

	// Parse headers in parallel with caching
	type headerResult struct {
		index      int
		headerPath string
		headerInfo *ahoy.CHeaderInfo
		namespace  string
	}

	// Results array preserves original order
	results := make([]*headerResult, len(headerImports))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, imp := range headerImports {
		wg.Add(1)
		go func(headerName, namespace string, idx int) {
			defer wg.Done()

			// Resolve relative paths to absolute paths
			resolvedHeaderName := headerName
			if strings.HasPrefix(headerName, "./") || strings.HasPrefix(headerName, "../") {
				sourceDir := filepath.Dir(gen.sourceFilename)
				absPath, err := filepath.Abs(filepath.Join(sourceDir, headerName))
				if err == nil {
					if _, statErr := os.Stat(absPath); statErr == nil {
						resolvedHeaderName = absPath
					}
				}
			}

			// Try to find the header file
			headerPath := ""
			if strings.HasPrefix(resolvedHeaderName, "/") {
				headerPath = resolvedHeaderName
			} else {
				// Try common locations
				locations := []string{
					resolvedHeaderName,
					"/usr/include/" + resolvedHeaderName,
					"/usr/local/include/" + resolvedHeaderName,
					"repos/raylib/src/" + resolvedHeaderName,
				}
				for _, loc := range locations {
					if _, err := os.Stat(loc); err == nil {
						headerPath = loc
						break
					}
				}
			}

			if headerPath == "" {
				return
			}

			// Use the shared helper to get or parse the header
			headerInfo := gen.getOrParseCHeader(headerPath)
			if headerInfo != nil {
				mu.Lock()
				results[idx] = &headerResult{idx, headerPath, headerInfo, namespace}
				mu.Unlock()
			}
		}(imp.headerName, imp.namespace, imp.index)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Process results in original order
	for _, result := range results {
		if result == nil || result.headerInfo == nil {
			continue
		}

		headerInfo := result.headerInfo
		namespace := result.namespace

		// Track struct/typedef names as known C types and store struct fields
		for typeName, structInfo := range headerInfo.Structs {
			gen.cTypeDefinitions[typeName] = true
			// Also register lowercase version for easier matching
			gen.cTypeDefinitions[strings.ToLower(typeName)] = true

			// Store struct fields for member access type inference
			if gen.cStructFields[typeName] == nil {
				gen.cStructFields[typeName] = make(map[string]string)
			}
			for _, field := range structInfo.Fields {
				gen.cStructFields[typeName][field.Name] = field.Type
			}
			// Also store lowercase version
			if gen.cStructFields[strings.ToLower(typeName)] == nil {
				gen.cStructFields[strings.ToLower(typeName)] = make(map[string]string)
			}
			for _, field := range structInfo.Fields {
				gen.cStructFields[strings.ToLower(typeName)][field.Name] = field.Type
			}
		}

		// Store typedef aliases
		for aliasName, typedefInfo := range headerInfo.Typedefs {
			gen.cTypedefs[aliasName] = typedefInfo.BaseType
			gen.cTypedefs[strings.ToLower(aliasName)] = typedefInfo.BaseType
			// Also mark the alias as a known type
			gen.cTypeDefinitions[aliasName] = true
			gen.cTypeDefinitions[strings.ToLower(aliasName)] = true
		}

		// Store function return types and register them as C types if they're structs
		if namespace != "" {
			if gen.cNamespaceReturnTypes[namespace] == nil {
				gen.cNamespaceReturnTypes[namespace] = make(map[string]string)
			}
			for cFuncName, funcInfo := range headerInfo.Functions {
				snakeName := ahoy.PascalToSnake(cFuncName)
				gen.cNamespaceReturnTypes[namespace][snakeName] = funcInfo.ReturnType

				// Register return type as a known C type if it's a struct
				if funcInfo.ReturnType != "" && funcInfo.ReturnType != "void" && funcInfo.ReturnType != "int" &&
					funcInfo.ReturnType != "float" && funcInfo.ReturnType != "double" && funcInfo.ReturnType != "char*" {
					gen.cTypeDefinitions[funcInfo.ReturnType] = true
					gen.cTypeDefinitions[strings.ToLower(funcInfo.ReturnType)] = true
				}

				// Store function parameter types
				paramTypes := []string{}
				for _, param := range funcInfo.Parameters {
					paramTypes = append(paramTypes, param.Type)
				}
				gen.cFunctionParamTypes[snakeName] = paramTypes
			}
		} else {
			for cFuncName, funcInfo := range headerInfo.Functions {
				snakeName := ahoy.PascalToSnake(cFuncName)
				gen.cFunctionReturnTypes[snakeName] = funcInfo.ReturnType

				// Register return type as a known C type if it's a struct
				if funcInfo.ReturnType != "" && funcInfo.ReturnType != "void" && funcInfo.ReturnType != "int" &&
					funcInfo.ReturnType != "float" && funcInfo.ReturnType != "double" && funcInfo.ReturnType != "char*" {
					gen.cTypeDefinitions[funcInfo.ReturnType] = true
					gen.cTypeDefinitions[strings.ToLower(funcInfo.ReturnType)] = true
				}

				// Store function parameter types
				paramTypes := []string{}
				for _, param := range funcInfo.Parameters {
					paramTypes = append(paramTypes, param.Type)
				}
				gen.cFunctionParamTypes[snakeName] = paramTypes
			}
		}
	}
}

func (gen *CodeGenerator) scanTypeDeclarations(node *ahoy.ASTNode) {
	if node == nil {
		return
	}

	// Scan for enum declarations
	if node.Type == ahoy.NODE_ENUM_DECLARATION {
		enumName := node.Value
		enumType := node.EnumType

		// Store enum type
		if enumType != "" {
			gen.enumTypes[enumName] = enumType
			gen.enumTypes[capitalizeFirst(enumName)] = enumType
		} else {
			gen.enumTypes[enumName] = "int"
			gen.enumTypes[capitalizeFirst(enumName)] = "int"
		}

		// Store enum members
		gen.enums[enumName] = make(map[string]bool)
		gen.enumOriginalNames[enumName] = true // Mark this as the original name
		// Note: Don't create capitalized duplicate here anymore to avoid ambiguity in resolution

		for _, member := range node.Children {
			memberName := member.Value
			gen.enums[enumName][memberName] = true
			gen.enumMemberTypes[enumName+"."+memberName] = enumName
			gen.enumMemberTypes[capitalizeFirst(enumName)+"."+memberName] = enumName
		}
	}

	// Scan for struct declarations
	if node.Type == ahoy.NODE_STRUCT_DECLARATION {
		structName := node.Value
		// Register the struct and its fields. The field types are needed when a
		// field holds a function value (r.draw_sprite|...|).
		if gen.structs[structName] == nil {
			info := &StructInfo{Name: structName, Fields: make([]StructField, 0)}
			for _, field := range node.Children {
				if field.Type == ahoy.NODE_OBJECT_PROPERTY || field.Type == ahoy.NODE_IDENTIFIER {
					info.Fields = append(info.Fields, StructField{Name: field.Value, Type: field.DataType, AhoyType: field.DataType})
				}
			}
			gen.structs[structName] = info
			gen.structs[capitalizeFirst(structName)] = info
		}
	}

	// Scan for constant declarations to track their types
	if node.Type == ahoy.NODE_CONSTANT_DECLARATION {
		constName := node.Value
		// Determine the constant type
		var constType string
		if node.DataType != "" {
			constType = node.DataType
		} else if len(node.Children) > 0 {
			// Infer type from the value
			constType = gen.inferType(node.Children[0])
		} else {
			constType = "int" // default
		}
		// Store constant type
		gen.constants[constName] = constType

		// Try to evaluate the constant expression at compile-time
		computedValue, canCompute := interface{}(nil), false
		if len(node.Children) > 0 {
			if value, ok := gen.evaluateConstantExpression(node.Children[0]); ok {
				gen.constantValues[constName] = value
				computedValue = value
				canCompute = true
			}
		}

		// All global and main-scope constants should use getters for hot reload
		// We can't check gen.currentFunction during scan, so we check the init expression
		// If it's simple or can be computed, make it a getter
		isSimpleValue := len(node.Children) > 0 && gen.isSimpleConstantValue(node.Children[0])
		if isSimpleValue || canCompute {
			gen.recordConstantGetterOrder(constName)
			gen.globalConstantGetters[constName] = ConstantInfo{
				Name:       constName,
				CType:      gen.mapType(constType),
				AhoyType:   constType,
				IsComputed: canCompute,
				Value:      computedValue,
				InitValue:  "", // Will be filled during code generation
			}
		}
	}

	// Recursively scan children
	for _, child := range node.Children {
		gen.scanTypeDeclarations(child)
	}
}

// recordConstantGetterOrder notes a constant's first-seen position for
// deterministic emission (see globalConstantGetterOrder). Updates to an
// existing entry keep the original position.
func (gen *CodeGenerator) recordConstantGetterOrder(constName string) {
	if _, exists := gen.globalConstantGetters[constName]; !exists {
		gen.globalConstantGetterOrder = append(gen.globalConstantGetterOrder, constName)
	}
}

func (gen *CodeGenerator) generateTypeDeclarations(node *ahoy.ASTNode) {
	if node == nil {
		return
	}

	// Generate enum declarations first
	if node.Type == ahoy.NODE_ENUM_DECLARATION {
		gen.generateEnum(node)
	}

	// Generate struct declarations
	if node.Type == ahoy.NODE_STRUCT_DECLARATION {
		gen.generateStruct(node)
	}

	// Recursively process children
	for _, child := range node.Children {
		gen.generateTypeDeclarations(child)
	}
}

func (gen *CodeGenerator) scanForMethodCalls(node *ahoy.ASTNode) {
	if node == nil {
		return
	}

	// Check for read_json or write_json calls
	if node.Type == ahoy.NODE_CALL && (node.Value == "read_json" || node.Value == "write_json") {
		if !gen.useJSON {
			gen.useJSON = true
			gen.registerJSONFunctionTypes()
		}
	}

	if node.Type == ahoy.NODE_METHOD_CALL && len(node.Children) > 0 {
		// Extract method name
		methodName := node.Value

		// Check the object type to determine if it's array, dict, or string method
		objectType := ""
		if len(node.Children) > 0 {
			objectType = gen.inferType(node.Children[0])
		}

		// Mark the method as used
		if objectType == "array" || strings.HasPrefix(objectType, "array[") {
			gen.arrayMethods[methodName] = true
		} else if objectType == "dict" || strings.HasPrefix(objectType, "dict[") {
			gen.dictMethods[methodName] = true
		} else if objectType == "string" {
			gen.stringMethods[methodName] = true
		}
	}

	for _, child := range node.Children {
		gen.scanForMethodCalls(child)
	}
}

// scanForFunctionCalls scans the AST to track which user-defined functions are actually called
// scanFunctionSignatures records the parameter and return types of every
// function before any body is generated. Signatures used to be recorded while
// generating the function's own body, so a function value (&​name) taken before
// the definition produced a thunk with no parameters: the C compiler then saw
// E.g. `null_clear()` for a three-argument function.
func (gen *CodeGenerator) scanFunctionSignatures(node *ahoy.ASTNode) {
	if node == nil {
		return
	}
	if node.Type == ahoy.NODE_FUNCTION {
		funcName := node.Value
		if len(node.Children) > 0 {
			params := node.Children[0]
			if params != nil && params.Type == ahoy.NODE_BLOCK {
				paramTypes := []string{}
				paramNames := []string{}
				paramDefaults := []*ahoy.ASTNode{}
				for _, param := range params.Children {
					paramNames = append(paramNames, param.Value)
					paramDefaults = append(paramDefaults, param.DefaultValue)
					if param.DataType != "" {
						paramTypes = append(paramTypes, param.DataType)
					} else {
						paramTypes = append(paramTypes, "any")
					}
				}
				gen.functionParamTypes[funcName] = paramTypes
				gen.functionParamNames[funcName] = paramNames
				gen.functionParamDefaults[funcName] = paramDefaults
			}
		}
		if node.DataType != "" && node.DataType != "void" && node.DataType != "infer" {
			if strings.Contains(node.DataType, ",") {
				gen.functionReturnTypes[funcName] = splitReturnTypes(node.DataType)
			} else {
				gen.functionReturnTypes[funcName] = []string{node.DataType}
			}
		}
	}
	for _, child := range node.Children {
		gen.scanFunctionSignatures(child)
	}
}

func (gen *CodeGenerator) scanForFunctionCalls(node *ahoy.ASTNode) {
	if node == nil {
		return
	}

	// Check for function calls to user-defined functions
	if node.Type == ahoy.NODE_CALL {
		funcName := node.Value
		// Mark this function as called if it's a user-defined function
		if gen.userFunctions[funcName] {
			gen.calledFunctions[funcName] = true
		}
	}

	// Recursively scan children
	for _, child := range node.Children {
		gen.scanForFunctionCalls(child)
	}
}

// inferAllFunctionReturnTypes pre-processes all functions with infer return type
func (gen *CodeGenerator) inferAllFunctionReturnTypes(node *ahoy.ASTNode) {
	if node == nil {
		return
	}

	if node.Type == ahoy.NODE_FUNCTION {
		funcName := node.Value
		// Check if this function has infer return type
		if node.DataType == "infer" {
			inferredTypes := gen.inferReturnTypes(node)
			if len(inferredTypes) > 0 {
				gen.functionReturnTypes[funcName] = inferredTypes
			}
		} else if node.DataType != "" && node.DataType != "void" {
			// For explicitly typed functions, store the return types
			if strings.Contains(node.DataType, ",") {
				gen.functionReturnTypes[funcName] = splitReturnTypes(node.DataType)
			} else {
				gen.functionReturnTypes[funcName] = []string{node.DataType}
			}
		}
	}

	for _, child := range node.Children {
		gen.inferAllFunctionReturnTypes(child)
	}
}

func (gen *CodeGenerator) writeIndent() {
	for i := 0; i < gen.indent; i++ {
		gen.output.WriteString("    ")
	}
}

// emitLineDirective writes a C #line directive so that anything reported about
// the generated code - compiler errors, sanitizer output, stack traces - names
// the .ahoy file and line instead of a line in output/*.c.
//
// Directives are emitted per statement rather than per node: statements are
// what carry meaningful line numbers, and expressions inside a statement would
// only add noise. A statement that generates several lines of C leaves the
// following generated lines attributed to consecutive Ahoy lines, which is
// close enough to be useful.
func (gen *CodeGenerator) emitLineDirective(node *ahoy.ASTNode) {
	if node == nil || node.Line <= 0 {
		return
	}
	file := node.File
	if file == "" {
		// Nested statements inherit the file of the top-level node they sit in.
		file = gen.currentFile
	}
	if file == "" {
		return
	}
	gen.output.WriteString("#line ")
	gen.output.WriteString(strconv.Itoa(node.Line))
	gen.output.WriteString(" \"")
	gen.output.WriteString(escapeCFilename(file))
	gen.output.WriteString("\"\n")
}

// escapeCFilename escapes a path for use inside a C string literal.
func escapeCFilename(path string) string {
	path = strings.ReplaceAll(path, "\\", "\\\\")
	return strings.ReplaceAll(path, "\"", "\\\"")
}

// emitCrashLocation stores the statement's Ahoy file/line in globals the
// crash handler reads back. It is emitted next to the #line directive, so it
// travels with the statement into every buffer bodies are built in (function
// bodies, switch cases, deferred-statement temporaries). Deferred code keeps
// its own line, which is correct: if it crashes at function exit, the defer
// statement is what faulted.
func (gen *CodeGenerator) emitCrashLocation(node *ahoy.ASTNode) {
	if node == nil || node.Line <= 0 {
		return
	}
	if !gen.enableSignalHandler || !CrashLineTracking {
		return
	}
	file := node.File
	if file == "" {
		// Same inheritance rule as emitLineDirective.
		file = gen.currentFile
	}
	if file == "" {
		return
	}
	// Report paths relative to the project root: absolute build-machine paths
	// must not leak into shipped binaries, and engine/core.ahoy:4708 reads
	// better in a gamer's crash report anyway. Files outside the common root
	// (shouldn't happen) keep their full path.
	display := file
	if gen.crashPathRoot != "" && strings.HasPrefix(file, gen.crashPathRoot) {
		display = file[len(gen.crashPathRoot):]
	}
	gen.writeIndent()
	gen.output.WriteString("__ahoy_file = \"")
	gen.output.WriteString(escapeCFilename(display))
	gen.output.WriteString("\"; __ahoy_line = ")
	gen.output.WriteString(strconv.Itoa(node.Line))
	gen.output.WriteString(";\n")
	// The lexically enclosing function travels with the statement too: a
	// bare entry marker goes stale as soon as the function calls anything
	// (a crash kept reporting the last-entered callee instead of the
	// function whose statement faulted). Empty when outside any function.
	if gen.currentAhoyFunction != "" {
		gen.writeIndent()
		gen.output.WriteString("__ahoy_func = \"")
		gen.output.WriteString(gen.currentAhoyFunction)
		gen.output.WriteString("\";\n")
	}
}

// isHeapAllocatedType reports whether a declaration of this type owns heap
// memory that must be released when its scope exits. A fresh function value
// owns its closure environment; a copy of one does not (the declaration path
// only registers calls and literals, not identifiers).
func (gen *CodeGenerator) isHeapAllocatedType(varType string) bool {
	// Check if a type requires heap allocation
	if strings.HasPrefix(varType, "array") || varType == "AhoyArray*" {
		return true
	}
	if strings.HasPrefix(varType, "dict") || varType == "HashMap*" {
		return true
	}
	if ahoy.IsFunctionType(varType) {
		return true
	}
	if varType == "pool" {
		return true
	}
	// String methods that return new strings are heap-allocated
	// JSON objects are heap-allocated
	if varType == "AhoyJSON*" {
		return true
	}
	return false
}

func (gen *CodeGenerator) generate(node *ahoy.ASTNode) {
	gen.generateNodeInternal(node, false)
}

func (gen *CodeGenerator) generateNode(node *ahoy.ASTNode) {
	gen.generateNodeInternal(node, false)
}

func (gen *CodeGenerator) generateNodeInternal(node *ahoy.ASTNode, isStatement bool) {
	if node == nil {
		return
	}

	// Statements carry a #line directive so that anything the C compiler,
	// sanitizers or a debugger report points at the .ahoy source rather than at
	// the generated C. They also record their Ahoy file/line in globals so a
	// segfault can be attributed to the exact statement (see emitCrashLocation).
	if isStatement {
		gen.emitLineDirective(node)
		gen.emitCrashLocation(node)
	}

	switch node.Type {
	case ahoy.NODE_PROGRAM:
		for _, child := range node.Children {
			// Top-level nodes are the only ones that know their file; statements
			// nested inside them inherit it through gen.currentFile.
			if child.File != "" {
				gen.currentFile = child.File
			}
			gen.generateNodeInternal(child, true)
		}

	case ahoy.NODE_FUNCTION:
		gen.generateFunction(node)

	case ahoy.NODE_ASSIGNMENT, ahoy.NODE_VARIABLE_DECLARATION:
		// A lambda initialised into a variable with a declared fn(...) type takes
		// its parameter and return types from that declaration.
		savedPending := gen.pendingFuncType
		if ahoy.IsFunctionType(node.DataType) {
			for _, c := range node.Children {
				if c.Type == ahoy.NODE_LAMBDA {
					gen.pendingFuncType = node.DataType
					break
				}
			}
		}
		gen.recordJobResultType(node)
		// Storing into a C callback typedef variable: &name must be emitted as
		// the raw C function pointer the typedef expects.
		savedCType := gen.pendingCType
		targetType := node.DataType
		if targetType == "" {
			if t, ok := gen.functionVars[node.Value]; ok {
				targetType = t
			} else if t, ok := gen.variables[node.Value]; ok {
				targetType = t
			}
		}
		if _, ok := gen.cFuncPtrTypedefs[targetType]; ok {
			gen.pendingCType = targetType
		}
		gen.generateAssignment(node)
		gen.pendingCType = savedCType
		gen.pendingFuncType = savedPending

	case ahoy.NODE_IF_STATEMENT:
		gen.generateIfStatement(node)

	case ahoy.NODE_SWITCH_STATEMENT:
		gen.generateSwitchStatement(node)

	case ahoy.NODE_WHILE_LOOP:
		gen.generateWhileLoop(node)

	case ahoy.NODE_FOR_LOOP:
		gen.generateForLoop(node)

	case ahoy.NODE_FOR_RANGE_LOOP:
		gen.generateForRangeLoop(node)

	case ahoy.NODE_FOR_COUNT_LOOP:
		gen.generateForCountLoop(node)

	case ahoy.NODE_FOR_IN_ARRAY_LOOP:
		gen.generateForInArrayLoop(node)

	case ahoy.NODE_FOR_IN_DICT_LOOP:
		gen.generateForInDictLoop(node)

	case ahoy.NODE_WHEN_STATEMENT:
		gen.generateWhenStatement(node)

	case ahoy.NODE_RETURN_STATEMENT:
		gen.generateReturnStatement(node)

	case ahoy.NODE_IMPORT_STATEMENT:
		gen.generateImportStatement(node)

	case ahoy.NODE_PROGRAM_DECLARATION:
		// Skip program declarations in code generation
		return

	case ahoy.NODE_CALL:
		if isStatement {
			gen.writeIndent()
		}
		gen.generateCall(node)
		if isStatement {
			gen.output.WriteString(";\n")
		}

	case ahoy.NODE_UNIQUE_ACCESS:
		// `%name` desugars to `node_unique(nodes, "name")`, reusing the
		// ordinary call path (name conversion, hot-reload getters, arity
		// checks already ran in the checker).
		if isStatement {
			gen.writeIndent()
		}
		gen.generateCall(&ahoy.ASTNode{
			Type:  ahoy.NODE_CALL,
			Value: "node_unique",
			Children: []*ahoy.ASTNode{
				{Type: ahoy.NODE_IDENTIFIER, Value: "nodes", Line: node.Line, Column: node.Column},
				{Type: ahoy.NODE_STRING, Value: node.Value, DataType: "string", Line: node.Line, Column: node.Column},
			},
			Line: node.Line, Column: node.Column,
		})
		if isStatement {
			gen.output.WriteString(";\n")
		}

	case ahoy.NODE_BINARY_OP:
		gen.generateBinaryOp(node)

	case ahoy.NODE_IS_LIST:
		gen.generateIsList(node)

	case ahoy.NODE_UNARY_OP:
		gen.generateUnaryOp(node)

	case ahoy.NODE_LAMBDA:
		gen.generateLambdaValue(node)

	case ahoy.NODE_TERNARY:
		if isStatement {
			gen.writeIndent()
		}
		gen.generateTernary(node)
		if isStatement {
			gen.output.WriteString(";\n")
		}

	case ahoy.NODE_IDENTIFIER:
		// Check if it's the loop counter variable
		if node.Value == "__loop_counter" && len(gen.loopCounters) > 0 {
			gen.output.WriteString(gen.loopCounters[len(gen.loopCounters)-1])
		} else if expr, isCapture := gen.closureCaptures[node.Value]; isCapture {
			// Within a closure thunk a captured local lives in the environment
			gen.output.WriteString(expr)
		} else {
			// Check if this is a global constant with a getter function
			if _, isGlobalConst := gen.globalConstantGetters[node.Value]; isGlobalConst {
				// Use getter function for hot reload support
				gen.output.WriteString("get_")
				gen.output.WriteString(node.Value)
				gen.output.WriteString("()")
			} else if resolvedName := gen.tryResolveEnumMember(node.Value); resolvedName != "" {
				// Check if this identifier might be an enum member (for switch cases)
				gen.output.WriteString(resolvedName)
			} else {
				// Check if it's a known constant/macro from raylib or other C libraries
				// These will be passed through directly to C
				// Don't convert variable names, only function names are converted
				gen.output.WriteString(node.Value)
			}
		}

	case ahoy.NODE_NUMBER:
		gen.output.WriteString(node.Value)

	case ahoy.NODE_STRING:
		// If used as a statement (isStatement == true), ignore it (no side effects)
		if !isStatement {
			gen.output.WriteString("\"")
			gen.output.WriteString(node.Value)
			gen.output.WriteString("\"")
		}

	case ahoy.NODE_RAW_STRING:
		// Raw strings preserve newlines and don't interpret escape sequences
		// In C, we need to escape the actual newlines and special chars for the string literal
		escaped := escapeRawString(node.Value)
		gen.output.WriteString("\"")
		gen.output.WriteString(escaped)
		gen.output.WriteString("\"")

	case ahoy.NODE_F_STRING:
		gen.generateFString(node)

	case ahoy.NODE_CHAR:
		gen.output.WriteString("'")
		gen.output.WriteString(node.Value)
		gen.output.WriteString("'")

	case ahoy.NODE_BOOLEAN:
		if node.Value == "true" {
			gen.output.WriteString("true")
		} else {
			gen.output.WriteString("false")
		}

	case ahoy.NODE_DICT_LITERAL:
		gen.generateDictLiteral(node)

	case ahoy.NODE_ARRAY_LITERAL:
		gen.generateArrayLiteral(node)

	case ahoy.NODE_OBJECT_LITERAL:
		gen.generateObjectLiteral(node)

	case ahoy.NODE_ARRAY_ACCESS:
		gen.generateArrayAccess(node)

	case ahoy.NODE_DICT_ACCESS:
		gen.generateDictAccess(node)

	case ahoy.NODE_OBJECT_ACCESS:
		gen.generateObjectAccess(node)

	case ahoy.NODE_BLOCK:
		for _, child := range node.Children {
			gen.generateNodeInternal(child, true)
		}
	case ahoy.NODE_ENUM_DECLARATION:
		gen.generateEnum(node)
	case ahoy.NODE_CONSTANT_DECLARATION:
		gen.generateConstant(node)
	case ahoy.NODE_TUPLE_ASSIGNMENT:
		gen.generateTupleAssignment(node)
	case ahoy.NODE_STRUCT_DECLARATION:
		gen.generateStruct(node)
	case ahoy.NODE_ALIAS_DECLARATION:
		// Type aliases are compile-time only, no C code needed
		return
	case ahoy.NODE_DISTINCT_DECLARATION:
		// Distinct newtypes share the base layout: record base for mapType.
		// DataType carries the base (set by parseDistinctDeclaration).
		if node.Value != "" && node.DataType != "" {
			gen.distinctTypes[node.Value] = node.DataType
		}
		return
	case ahoy.NODE_UNION_DECLARATION:
		// Union types are compile-time only, no C code needed
		return
	case ahoy.NODE_METHOD_CALL:
		if isStatement {
			gen.writeIndent()
		}
		gen.generateMethodCall(node)
		if isStatement {
			gen.output.WriteString(";\n")
		}
	case ahoy.NODE_TYPE_PROPERTY:
		gen.generateTypeProperty(node)
	case ahoy.NODE_MEMBER_ACCESS:
		gen.generateMemberAccess(node)
	case ahoy.NODE_STATIC_MEMBER_ACCESS:
		gen.generateStaticMemberAccess(node)
	case ahoy.NODE_HALT:
		// In-flight block-scoped defers would otherwise be skipped: the
		// early exit jumps over their block end. Flush them first
		// (use-then-free), over exactly the scopes being left.
		if label, ok := gen.loopEndLabelAcrossSwitch(); ok {
			// A switch-case boundary sits between halt and its loop: plain
			// `break` would only exit the switch, so clean through the loop
			// scope and jump to the loop end (cleanup already ran above the
			// goto — unlike user-level goto, removed for bypassing it).
			gen.flushBlockScopedDefers(gen.loopScopeStack[len(gen.loopScopeStack)-1])
			cleanup := gen.generateEarlyExitCleanup(false)
			if cleanup != "" {
				gen.output.WriteString(cleanup)
			}
			gen.writeIndent()
			gen.output.WriteString("goto ")
			gen.output.WriteString(label)
			gen.output.WriteString(";\n")
		} else if len(gen.switchCaseDepths) > 0 {
			// Breaking the switch itself: free only the case (and anything
			// deeper, e.g. an if around the halt) — never the enclosing
			// function locals, which outlive the switch.
			caseDepth := gen.switchCaseDepths[len(gen.switchCaseDepths)-1]
			gen.flushBlockScopedDefers(caseDepth)
			cleanup := gen.generateEarlyExitCleanupTo(caseDepth)
			if cleanup != "" {
				gen.output.WriteString(cleanup)
			}
			gen.writeIndent()
			gen.output.WriteString("break;\n")
		} else {
			// Generate cleanup for variables in nested scopes before breaking
			// Note: generateEarlyExitCleanup returns code WITH indentation/newlines
			if len(gen.loopScopeStack) > 0 {
				gen.flushBlockScopedDefers(gen.loopScopeStack[len(gen.loopScopeStack)-1])
			}
			cleanup := gen.generateEarlyExitCleanup(false)
			if cleanup != "" {
				gen.output.WriteString(cleanup)
			}
			gen.writeIndent()
			gen.output.WriteString("break;\n")
		}
	case ahoy.NODE_NEXT:
		// Generate cleanup for variables in nested scopes before continuing
		// Note: generateEarlyExitCleanup returns code WITH indentation/newlines.
		// Like halt, flush in-flight block-scoped defers first: `continue`
		// also skips their block end (function-level defers still run once
		// at function end).
		if len(gen.loopScopeStack) > 0 {
			gen.flushBlockScopedDefers(gen.loopScopeStack[len(gen.loopScopeStack)-1])
		} else if len(gen.switchCaseDepths) > 0 {
			gen.flushBlockScopedDefers(gen.switchCaseDepths[len(gen.switchCaseDepths)-1])
		}
		cleanup := gen.generateEarlyExitCleanup(false)
		if cleanup != "" {
			gen.output.WriteString(cleanup)
		}
		gen.writeIndent()
		gen.output.WriteString("continue;\n")
	case ahoy.NODE_ASSERT_STATEMENT:
		gen.generateAssertStatement(node)
	case ahoy.NODE_DEFER_STATEMENT:
		gen.generateDeferStatement(node)
	}
}

func (gen *CodeGenerator) generateFunction(node *ahoy.ASTNode) {
	funcName := node.Value

	// Rename main to ahoy_main to avoid conflict with C's main
	cFuncName := funcName
	if funcName == "main" {
		cFuncName = "ahoy_main"
	}

	// Track this as a user-defined function (keep snake_case)
	gen.userFunctions[funcName] = true

	// Check if this function should be generated
	// Skip if: (1) never called AND (2) has untyped params or inferred return type
	// This prevents type inference errors in unused functions
	if !gen.calledFunctions[funcName] && funcName != "main" {
		// Check if function has untyped parameters
		hasUntypedParams := false
		params := node.Children[0]
		for _, param := range params.Children {
			if param.DataType == "" || param.DataType == "any" || param.DataType == "generic" {
				hasUntypedParams = true
				break
			}
		}

		// Check if function has inferred return type or void/no return type
		hasInferredReturn := node.DataType == "" || node.DataType == "infer"

		// Skip generation if both conditions are met
		if hasUntypedParams || hasInferredReturn {
			// Don't generate this function - it's never called and would cause type inference issues
			return
		}
	}

	returnType := "void"
	returnTypes := []string{}

	// Check if we have multiple return types (comma-separated in DataType)
	if node.DataType != "" {
		if node.DataType == "infer" {
			// Need to infer return types from the return statement
			inferredTypes := gen.inferReturnTypes(node)

			if len(inferredTypes) > 1 {
				// Multiple inferred return types
				returnTypes = inferredTypes

				// Generate struct definition for multi-return
				structName := fmt.Sprintf("%s_return", funcName)
				gen.funcReturnStructs.WriteString("typedef struct {\n")
				for i, rType := range returnTypes {
					// Use intptr_t for generic/any types (will be cast at call site)
					var mappedType string
					if rType == "generic" || rType == "any" {
						mappedType = "intptr_t"
					} else {
						mappedType = gen.mapType(rType)
					}
					gen.funcReturnStructs.WriteString("    ")
					gen.funcReturnStructs.WriteString(mappedType)
					gen.funcReturnStructs.WriteString(" ret")
					gen.funcReturnStructs.WriteString(strconv.Itoa(i))
					gen.funcReturnStructs.WriteString(";\n")
				}
				gen.funcReturnStructs.WriteString("} ")
				gen.funcReturnStructs.WriteString(structName)
				gen.funcReturnStructs.WriteString(";\n\n")
				returnType = structName
			} else if len(inferredTypes) == 1 {
				// Single inferred return type
				returnTypes = inferredTypes // Store for lookup
				returnType = gen.mapType(inferredTypes[0])
			} else {
				// No return statement found, default to void
				returnType = "void"
			}
		} else {
			// Use smart split that handles nested commas in dict<k,v>
			returnTypes = splitReturnTypes(node.DataType)

			if len(returnTypes) > 1 {
				// Multiple return types - create a struct
				structName := fmt.Sprintf("%s_return", funcName)
				gen.funcReturnStructs.WriteString("typedef struct {\n")
				for i, rType := range returnTypes {
					mappedType := gen.mapType(rType)
					gen.funcReturnStructs.WriteString("    ")
					gen.funcReturnStructs.WriteString(mappedType)
					gen.funcReturnStructs.WriteString(" ret")
					gen.funcReturnStructs.WriteString(strconv.Itoa(i))
					gen.funcReturnStructs.WriteString(";\n")
				}
				gen.funcReturnStructs.WriteString("} ")
				gen.funcReturnStructs.WriteString(structName)
				gen.funcReturnStructs.WriteString(";\n\n")
				returnType = structName
			} else {
				// Single return type (even if it contains commas like dict<k,v>)
				returnType = gen.mapType(node.DataType)
			}
		}
	}

	// Build parameter list for both declaration and forward declaration
	params := node.Children[0]
	paramList := ""
	for i, param := range params.Children {
		if i > 0 {
			paramList += ", "
		}
		paramType := "intptr_t" // default for untyped/generic/any parameters
		if param.DataType != "" {
			if param.DataType == "generic" || param.DataType == "any" {
				paramType = "intptr_t" // Use intptr_t for generic/any parameters
			} else {
				paramType = gen.mapType(param.DataType)
			}
		}
		paramList += fmt.Sprintf("%s %s", paramType, param.Value)
	}

	// Store return types and parameter types for this function (for later lookup)
	if len(returnTypes) > 0 {
		gen.functionReturnTypes[funcName] = returnTypes
	}

	// Store parameter types, names, and default values
	paramTypes := []string{}
	paramNames := []string{}
	paramDefaults := []*ahoy.ASTNode{}
	for _, param := range params.Children {
		paramNames = append(paramNames, param.Value)
		paramDefaults = append(paramDefaults, param.DefaultValue)
		if param.DataType != "" {
			paramTypes = append(paramTypes, param.DataType)
		} else {
			paramTypes = append(paramTypes, "any")
		}
	}
	gen.functionParamTypes[funcName] = paramTypes
	gen.functionParamNames[funcName] = paramNames
	gen.functionParamDefaults[funcName] = paramDefaults

	// Write forward declaration
	gen.funcForwardDecls.WriteString(returnType)
	gen.funcForwardDecls.WriteString(" ")
	gen.funcForwardDecls.WriteString(cFuncName)
	gen.funcForwardDecls.WriteString("(")
	gen.funcForwardDecls.WriteString(paramList)
	gen.funcForwardDecls.WriteString(");\n")
	// Write function implementation. A signature error (an unknown parameter or
	// return type) must be attributed to this function, not to whatever was
	// generated before it, so name the source line here too.
	if node.File != "" {
		gen.funcDecls.WriteString("#line ")
		gen.funcDecls.WriteString(strconv.Itoa(node.Line))
		gen.funcDecls.WriteString(" \"")
		gen.funcDecls.WriteString(escapeCFilename(node.File))
		gen.funcDecls.WriteString("\"\n")
	}
	gen.funcDecls.WriteString(returnType)
	gen.funcDecls.WriteString(" ")
	gen.funcDecls.WriteString(cFuncName)
	gen.funcDecls.WriteString("(")
	gen.funcDecls.WriteString(paramList)
	gen.funcDecls.WriteString(") {\n")

	// Function body
	body := node.Children[1]
	oldOutput := gen.output
	gen.output = strings.Builder{}
	gen.indent++

	// Store current function info for return statement generation
	gen.currentFunction = cFuncName
	gen.currentAhoyFunction = funcName
	gen.currentFunctionReturnType = returnType
	gen.currentFunctionHasMultiReturn = len(returnTypes) > 1

	// Initialize function-local variable scope and add parameters
	gen.functionVars = make(map[string]string)
	gen.dictSourcedVars = make(map[string]string)
	gen.dictSourcedKeys = make(map[string]string)
	gen.dictLoopValueExpr = make(map[string]string)
	gen.nestedScopeVars = make(map[string]bool)
	gen.varDeclIndent = make(map[string]int)
	// Array element types are function-local too: a `lines: array[int]`
	// parameter (or local) in one function used to poison every later
	// same-named array program-wide (the stale entry won via the
	// first-wins guard), miscompiling element accesses into segfaults.
	gen.arrayElementTypes = make(map[string]string)
	gen.array2DElementTypes = make(map[string]string)

	// Clear function-local declared variables for this new function
	gen.declaredFunctionVars = make(map[string]bool)
	// Start a fresh block-scope chain; the outermost entry is the function body.
	gen.scopeDeclared = []map[string]bool{{}}

	for _, param := range params.Children {
		if param.DataType != "" {
			gen.functionVars[param.Value] = param.DataType

			// Parameters arrive declared (they are C function parameters):
			// an assignment to one must emit a plain store. Without these
			// marks the assign path treated params as undeclared and
			// emitted a fresh C declaration, which collided with the
			// parameter (or silently shadowed it).
			gen.declaredFunctionVars[param.Value] = true
			gen.markDeclaredInCurrentScope(param.Value)

			// Track array element types for typed array parameters
			if strings.HasPrefix(param.DataType, "array[") {
				elemType := strings.TrimSuffix(strings.TrimPrefix(param.DataType, "array["), "]")
				gen.arrayElementTypes[param.Value] = elemType
			}
		} else {
			// Parameters without explicit type are any (generic)
			gen.functionVars[param.Value] = "any"
			gen.declaredFunctionVars[param.Value] = true
			gen.markDeclaredInCurrentScope(param.Value)
		}
	}

	// Initialize deferred statements stack for this function
	gen.deferredStatements = []deferredStmt{}
	gen.heapAllocatedVars = make(map[string]bool)
	gen.heapVarScopes = make(map[string]int)
	gen.heapVarTypes = make(map[string]string)
	gen.scopeDepth = 0
	gen.scopeAllocations = make(map[int][]string)
	gen.loopScopeStack = make([]int, 0)
	gen.loopEndLabels = make([]loopEndLabel, 0)
	gen.pendingLoopEndLabel = ""
	gen.switchCaseDepths = nil
	gen.escapingVars = make(map[string]bool)
	gen.deferredUsers = make(map[string]bool)
	gen.aliasEdges = make(map[string]map[string]int)
	gen.manuallyFreedVars = make(map[string]bool)
	gen.autoFreedVars = make(map[string]bool)
	gen.functionParameters = make(map[string]bool)

	// Mark function parameters so we never auto-free them. They are visible for
	// the whole function body, so they belong in the outermost block scope.
	for _, param := range params.Children {
		gen.functionParameters[param.Value] = true
		gen.markDeclaredInCurrentScope(param.Value)
	}

	// Crash reporting: record which Ahoy function is executing. Per-statement
	// stores keep file/line current; this covers entry and bodies with no
	// tracked statements. The Ahoy-level name reads better than the C name.
	if gen.enableSignalHandler && CrashLineTracking {
		gen.writeIndent()
		gen.output.WriteString("__ahoy_func = \"")
		gen.output.WriteString(funcName)
		gen.output.WriteString("\";\n")
	}

	gen.generateNodeInternal(body, false)

	// Add automatic defer free for heap-allocated variables that don't escape
	gen.addAutomaticDeferFrees()

	// Execute deferred statements in LIFO order before function end
	if len(gen.deferredStatements) > 0 {
		for i := len(gen.deferredStatements) - 1; i >= 0; i-- {
			gen.output.WriteString(gen.deferredStatements[i].code)
		}
	}

	gen.funcDecls.WriteString(gen.output.String())
	gen.funcDecls.WriteString("}\n\n")

	gen.indent--
	gen.output = oldOutput
	gen.currentFunction = ""
	gen.currentAhoyFunction = ""
	gen.currentFunctionReturnType = ""
	gen.currentFunctionHasMultiReturn = false
	gen.functionVars = nil                           // Clear function scope
	gen.deferredStatements = nil                     // Clear deferred statements
	gen.declaredFunctionVars = make(map[string]bool) // Clear function-local declarations
}

func (gen *CodeGenerator) generateAssignment(node *ahoy.ASTNode) {
	gen.writeIndent()

	// Check if this is a static member access assignment (StructType.#static_field: value)
	if len(node.Children) == 2 && node.Children[0].Type == ahoy.NODE_STATIC_MEMBER_ACCESS {
		gen.generateStaticMemberAccess(node.Children[0])
		gen.output.WriteString(" = ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(";\n")
		return
	}

	// Check if this is a property/element/pointer assignment (obj<'prop'>: value or dict{"key"}: value or obj.prop: value or ^ptr: value)
	// In this case, Children[0] is the access node, Children[1] is the value
	if len(node.Children) == 2 &&
		(node.Children[0].Type == ahoy.NODE_OBJECT_ACCESS ||
			node.Children[0].Type == ahoy.NODE_DICT_ACCESS ||
			node.Children[0].Type == ahoy.NODE_ARRAY_ACCESS ||
			node.Children[0].Type == ahoy.NODE_MEMBER_ACCESS ||
			node.Children[0].Type == ahoy.NODE_UNARY_OP) {

		// Special handling for chained array assignment (2D arrays): grid[i][j]: value
		if node.Children[0].Type == ahoy.NODE_ARRAY_ACCESS && node.Children[0].Value == "" {
			gen.generateChainedArrayAssignment(node)
			return
		}

		// Special handling for array assignment
		if node.Children[0].Type == ahoy.NODE_ARRAY_ACCESS {
			arrayName := node.Children[0].Value
			indexNode := node.Children[0].Children[0]
			valueNode := node.Children[1]

			// Check if the variable type is intptr_t, void*, generic, or any (might need casting to AhoyArray*)
			needsArrayCast := false
			if varType, exists := gen.variables[arrayName]; exists {
				if varType == "intptr_t" || varType == "void*" || varType == "generic" || varType == "any" {
					needsArrayCast = true
				}
			}
			if varType, exists := gen.functionVars[arrayName]; exists {
				if varType == "intptr_t" || varType == "void*" || varType == "generic" || varType == "any" {
					needsArrayCast = true
				}
			}

			// Check element type for proper assignment
			var elemType string
			if et, exists := gen.arrayElementTypes[arrayName]; exists {
				elemType = et
			}

			if gen.enableBoundsChecking {
				// Generate bounds check before assignment
				gen.output.WriteString("{ int __idx = ")
				gen.generateNode(indexNode)
				gen.output.WriteString("; ")

				if needsArrayCast {
					gen.output.WriteString("AhoyArray* __arr = (AhoyArray*)")
					gen.output.WriteString(arrayName)
					gen.output.WriteString("; ")
				} else {
					gen.output.WriteString("AhoyArray* __arr = ")
					gen.output.WriteString(arrayName)
					gen.output.WriteString("; ")
				}

				gen.output.WriteString("if (__idx < 0 || __idx >= __arr->length) { ")
				gen.output.WriteString("fprintf(stderr, \"RUNTIME ERROR: Array bounds violation\\n\"); ")
				gen.output.WriteString("fprintf(stderr, \"  File: ")
				gen.output.WriteString(gen.sourceFilename)
				gen.output.WriteString("\\n\"); ")
				gen.output.WriteString("fprintf(stderr, \"  Line: ")
				gen.output.WriteString(strconv.Itoa(node.Children[0].Line))
				gen.output.WriteString("\\n\"); ")
				gen.output.WriteString("fprintf(stderr, \"  Array: ")
				gen.output.WriteString(arrayName)
				gen.output.WriteString("\\n\"); ")
				gen.output.WriteString("fprintf(stderr, \"  Index: %d\\n\", __idx); ")
				gen.output.WriteString("fprintf(stderr, \"  Valid range: 0 to %d\\n\", __arr->length - 1); ")
				gen.output.WriteString("exit(1); ")
				gen.output.WriteString("} ")
			}

			// Handle assignment based on element type
			if elemType != "" {
				cType := gen.mapType(elemType)
				// Check if it's a C struct OR an Ahoy user-defined struct
				isCStruct := gen.cTypeDefinitions[cType] && !strings.HasSuffix(cType, "*") &&
					cType != "int" && cType != "double" && cType != "char*" && cType != "bool"
				isAhoyStruct := false
				if !isCStruct {
					// Check if it's an Ahoy struct
					if _, exists := gen.structs[elemType]; exists {
						isAhoyStruct = true
					} else if _, exists := gen.structs[cType]; exists {
						isAhoyStruct = true
					}
				}

				if isCStruct || isAhoyStruct {
					// For C structs, assign to the dereferenced pointer
					if gen.enableBoundsChecking {
						gen.output.WriteString("*(")
						gen.output.WriteString(cType)
						gen.output.WriteString("*)__arr->data[__idx] = ")
					} else {
						if needsArrayCast {
							gen.output.WriteString("*(")
							gen.output.WriteString(cType)
							gen.output.WriteString("*)((AhoyArray*)")
							gen.output.WriteString(arrayName)
							gen.output.WriteString(")->data[")
						} else {
							gen.output.WriteString("*(")
							gen.output.WriteString(cType)
							gen.output.WriteString("*)")
							gen.output.WriteString(arrayName)
							gen.output.WriteString("->data[")
						}
						gen.generateNode(indexNode)
						gen.output.WriteString("] = ")
					}
					gen.generateNode(valueNode)
					gen.output.WriteString(";")
				} else if cType == "double" || cType == "float" {
					// A float element is a heap double: the runtime reads
					// AHOY_TYPE_FLOAT elements as double*. A numeric cast stored
					// 4.25 as 4, and the next read dereferenced address 4.
					if gen.enableBoundsChecking {
						gen.output.WriteString("__arr->data[__idx] = (intptr_t)({ double* __fp = malloc(sizeof(double)); *__fp = ")
					} else {
						if needsArrayCast {
							gen.output.WriteString("((AhoyArray*)")
							gen.output.WriteString(arrayName)
							gen.output.WriteString(")->data[")
						} else {
							gen.output.WriteString(arrayName)
							gen.output.WriteString("->data[")
						}
						gen.generateNode(indexNode)
						gen.output.WriteString("] = (intptr_t)({ double* __fp = malloc(sizeof(double)); *__fp = ")
					}
					gen.generateNode(valueNode)
					gen.output.WriteString("; __fp; });")
				} else if cType == "char*" {
					// For strings, store as pointer
					if gen.enableBoundsChecking {
						gen.output.WriteString("__arr->data[__idx] = (intptr_t)")
					} else {
						if needsArrayCast {
							gen.output.WriteString("((AhoyArray*)")
							gen.output.WriteString(arrayName)
							gen.output.WriteString(")->data[")
						} else {
							gen.output.WriteString(arrayName)
							gen.output.WriteString("->data[")
						}
						gen.generateNode(indexNode)
						gen.output.WriteString("] = (void*)")
					}
					gen.generateNode(valueNode)
					gen.output.WriteString(";")
				} else {
					// For int and other intptr_t-compatible types, direct assignment
					if gen.enableBoundsChecking {
						gen.output.WriteString("__arr->data[__idx] = (intptr_t)(intptr_t)")
					} else {
						if needsArrayCast {
							gen.output.WriteString("((AhoyArray*)")
							gen.output.WriteString(arrayName)
							gen.output.WriteString(")->data[")
						} else {
							gen.output.WriteString(arrayName)
							gen.output.WriteString("->data[")
						}
						gen.generateNode(indexNode)
						gen.output.WriteString("] = (void*)(intptr_t)")
					}
					gen.generateNode(valueNode)
					gen.output.WriteString(";")
				}
			} else {
				// Unknown type - assume intptr_t
				if gen.enableBoundsChecking {
					gen.output.WriteString("__arr->data[__idx] = (intptr_t)(intptr_t)")
				} else {
					if needsArrayCast {
						gen.output.WriteString("((AhoyArray*)")
						gen.output.WriteString(arrayName)
						gen.output.WriteString(")->data[")
					} else {
						gen.output.WriteString(arrayName)
						gen.output.WriteString("->data[")
					}
					gen.generateNode(indexNode)
					gen.output.WriteString("] = (void*)(intptr_t)")
				}
				gen.generateNode(valueNode)
				gen.output.WriteString(";")
			}

			if gen.enableBoundsChecking {
				gen.output.WriteString(" }\n")
			} else {
				gen.output.WriteString("\n")
			}
			return
		}

		// Special handling for dict assignment - d<"key">: value
		if node.Children[0].Type == ahoy.NODE_DICT_ACCESS {
			dictName := gen.nameExpr(node.Children[0].Value)
			keyNode := node.Children[0].Children[0]
			valueNode := node.Children[1]

			gen.generateDictPutTyped(dictName, keyNode, valueNode)
			return
		}

		// Special handling for object access assignment - use hashMapPut if it's a HashMap/dict/generic
		if node.Children[0].Type == ahoy.NODE_OBJECT_ACCESS {
			objectName := node.Children[0].Value
			propertyName := ""
			if len(node.Children[0].Children) > 0 && node.Children[0].Children[0].Type == ahoy.NODE_STRING {
				propertyName = node.Children[0].Children[0].Value
			}

			// Check if this is a HashMap/dict or generic parameter
			objectType := ""
			if varType, exists := gen.variables[objectName]; exists {
				objectType = varType
			} else if varType, exists := gen.functionVars[objectName]; exists {
				objectType = varType
			}

			// If object is dict, HashMap*, generic, any, or intptr_t, store the value
			// through the typed putter (see generateDictPutTyped)
			if objectType == "dict" || objectType == "HashMap*" || objectType == "generic" || objectType == "any" || objectType == "intptr_t" ||
				strings.HasPrefix(objectType, "dict[") || strings.HasPrefix(objectType, "dict<") {
				keyNode := &ahoy.ASTNode{Type: ahoy.NODE_STRING, Value: propertyName, Line: node.Line}
				dictExpr := objectName
				if objectType == "generic" || objectType == "any" || objectType == "intptr_t" {
					dictExpr = "(HashMap*)" + objectName
				}
				gen.generateDictPutTyped(dictExpr, keyNode, node.Children[1])
				return
			}
		}

		// Special handling for member access on 2D array elements: arr[i][j].field: value
		if node.Children[0].Type == ahoy.NODE_MEMBER_ACCESS &&
			len(node.Children[0].Children) > 0 &&
			node.Children[0].Children[0].Type == ahoy.NODE_ARRAY_ACCESS &&
			node.Children[0].Children[0].Value == "" &&
			len(node.Children[0].Children[0].Children) == 2 {

			chainedAccess := node.Children[0].Children[0]
			memberName := node.Children[0].Value
			innerAccess := chainedAccess.Children[0]
			outerIndex := chainedAccess.Children[1]
			valueNode := node.Children[1]

			// Get the 2D array's nested element type
			var elemType string
			if innerAccess.Type == ahoy.NODE_ARRAY_ACCESS && innerAccess.Value != "" {
				if et, exists := gen.array2DElementTypes[innerAccess.Value]; exists {
					elemType = et
				}
			}

			if elemType != "" {
				cType := gen.mapType(elemType)
				// Check if it's a struct type
				isStruct := false
				if _, exists := gen.structs[elemType]; exists {
					isStruct = true
				} else if _, exists := gen.structs[cType]; exists {
					isStruct = true
				}

				if isStruct {
					gen.writeIndent()
					// Generate: ((StructType*)(intptr_t)((AhoyArray*)inner_access)->data[outer_idx])->member = value
					gen.output.WriteString("((")
					gen.output.WriteString(cType)
					gen.output.WriteString("*)(intptr_t)((AhoyArray*)")
					gen.generateNode(innerAccess)
					gen.output.WriteString(")->data[")
					gen.generateNode(outerIndex)
					gen.output.WriteString("])->")
					gen.output.WriteString(memberName)
					gen.output.WriteString(" = ")
					gen.generateNode(valueNode)
					gen.output.WriteString(";\n")
					return
				}
			}
		}

		// Special handling for member access on array elements: arr[i].field: value
		if node.Children[0].Type == ahoy.NODE_MEMBER_ACCESS &&
			len(node.Children[0].Children) > 0 &&
			node.Children[0].Children[0].Type == ahoy.NODE_ARRAY_ACCESS {

			arrayAccessNode := node.Children[0].Children[0]
			memberName := node.Children[0].Value
			arrayName := arrayAccessNode.Value
			indexNode := arrayAccessNode.Children[0]
			valueNode := node.Children[1]

			// Get array element type
			var elemType string
			if et, exists := gen.arrayElementTypes[arrayName]; exists {
				elemType = et
			}

			if elemType != "" {
				cType := gen.mapType(elemType)
				// Check if it's a struct type
				isCStruct := gen.cTypeDefinitions[cType] && !strings.HasSuffix(cType, "*") &&
					cType != "int" && cType != "double" && cType != "char*" && cType != "bool"
				isAhoyStruct := false
				if !isCStruct {
					if _, exists := gen.structs[elemType]; exists {
						isAhoyStruct = true
					} else if _, exists := gen.structs[cType]; exists {
						isAhoyStruct = true
					}
				}

				if isCStruct || isAhoyStruct {
					// For struct arrays, we need to access the pointer directly
					gen.writeIndent()
					if gen.enableBoundsChecking {
						gen.output.WriteString("{ int __idx = ")
						gen.generateNode(indexNode)
						gen.output.WriteString("; ")
						gen.output.WriteString("AhoyArray* __arr = ")
						gen.output.WriteString(arrayName)
						gen.output.WriteString("; ")
						gen.output.WriteString("if (__idx < 0 || __idx >= __arr->length) { ")
						gen.output.WriteString("fprintf(stderr, \"RUNTIME ERROR: Array bounds violation\\n\"); ")
						gen.output.WriteString("fprintf(stderr, \"  File: ")
						gen.output.WriteString(gen.sourceFilename)
						gen.output.WriteString("\\n\"); ")
						gen.output.WriteString("fprintf(stderr, \"  Line: ")
						gen.output.WriteString(strconv.Itoa(node.Line))
						gen.output.WriteString("\\n\"); ")
						gen.output.WriteString("fprintf(stderr, \"  Array: ")
						gen.output.WriteString(arrayName)
						gen.output.WriteString("\\n\"); ")
						gen.output.WriteString("fprintf(stderr, \"  Index: %d\\n\", __idx); ")
						gen.output.WriteString("fprintf(stderr, \"  Valid range: 0 to %d\\n\", __arr->length - 1); ")
						gen.output.WriteString("exit(1); } ")
						gen.output.WriteString("((")
						gen.output.WriteString(cType)
						gen.output.WriteString("*)__arr->data[__idx])->")
						gen.output.WriteString(memberName)
						gen.output.WriteString(" = ")
						gen.generateNode(valueNode)
						gen.output.WriteString("; }\n")
					} else {
						gen.output.WriteString("((")
						gen.output.WriteString(cType)
						gen.output.WriteString("*)")
						gen.output.WriteString(arrayName)
						gen.output.WriteString("->data[")
						gen.generateNode(indexNode)
						gen.output.WriteString("])->")
						gen.output.WriteString(memberName)
						gen.output.WriteString(" = ")
						gen.generateNode(valueNode)
						gen.output.WriteString(";\n")
					}
					return
				}
			}
		}

		// For struct field/array/pointer access, direct assignment works
		// But skip assignment to const properties (SCREAMING_SNAKE_CASE)
		if node.Children[0].Type == ahoy.NODE_MEMBER_ACCESS && len(node.Children[0].Children) > 0 {
			memberName := node.Children[0].Value
			// Check if this is a const property (SCREAMING_SNAKE_CASE)
			if isScreamingSnakeCase(memberName) {
				// Skip generating assignment for const properties
				// This effectively makes the assignment a no-op
				return
			}
		}

		// Mark any variables in the value as escaping (being stored)
		gen.markEscapingVariables(node.Children[1])
		// Transitive stored dependencies (aliases of aliases).
		gen.markValueDeps(node.Children[1])

		gen.generateNode(node.Children[0])
		gen.output.WriteString(" = ")

		// Check if RHS is an f-string - if so, wrap in strdup to avoid use-after-free
		// F-strings use static buffers that get overwritten, so struct fields need ownership
		if node.Children[1].Type == ahoy.NODE_F_STRING {
			gen.output.WriteString("strdup(")
			gen.generateNode(node.Children[1])
			gen.output.WriteString(")")
		} else {
			gen.generateNode(node.Children[1])
		}

		gen.output.WriteString(";\n")
		return
	}

	// Check if variable has been actually declared in C code
	// Check both global and function-local scopes
	inFunction := gen.currentFunction != ""
	_, isDeclaredGlobal := gen.declaredGlobalVars[node.Value]
	_, isDeclaredLocal := gen.declaredFunctionVars[node.Value]
	isDeclared := isDeclaredGlobal || isDeclaredLocal
	isNestedScope := gen.nestedScopeVars[node.Value]
	declIndent := gen.varDeclIndent[node.Value] // indent level where var was declared

	valueNode := node.Children[0]

	// In ahoy, `var: value` always creates a new variable in the current block scope
	// In C, we need to declare variables in each block where they're used

	// Determine if we need to redeclare the variable:
	// 1. Loop local patterns (array/dict access) should redeclare
	// 2. If we're at a different indent level than where the variable was declared,
	//    and we're in a nested block (indent > 2), we need to declare a new block-local variable
	// Exception: if we're inside the same block chain (deeper indent but contiguous),
	//    we should reassign not redeclare - but detecting this is hard, so we just
	//    redeclare when indent differs and we're deep enough
	// A declaration whose value comes out of a container is per-iteration: each
	// pass through the block declares its own C variable. An *assignment* from a
	// container is not: `text = quoted[1]` used to be redeclared too, so the
	// outer variable kept its old value (a shadowing `char* text = ...` inside
	// the if body) and every parsed string came back empty.
	isLoopLocalPattern := node.Type == ahoy.NODE_VARIABLE_DECLARATION &&
		(valueNode.Type == ahoy.NODE_ARRAY_ACCESS || valueNode.Type == ahoy.NODE_DICT_ACCESS)

	needsRedeclare := false

	// If variable was declared in nested scope and current indent is < declaration indent,
	// we've exited that scope and are in a parent block - need to redeclare
	if isNestedScope && gen.indent < declIndent {
		needsRedeclare = true
	}

	// If we're in a deeply nested block (indent > 2) and the indent differs from
	// where the variable was declared, we're likely in a different block structure
	// and should declare a new block-local variable.
	//
	// The indent rule alone emitted a shadowing declaration for an ordinary
	// assignment to an outer variable from inside `loop ... if`: the variable was
	// still in scope, so the C compiler saw a second `int x = ...` and the outer
	// value never changed. nameVisibleInOpenScope answers the question directly,
	// so the indentation guess only applies when it cannot (a name this pass has
	// not seen declared, such as one from a pre-pass).
	if isDeclared && gen.indent > 2 && gen.indent != declIndent && !gen.nameVisibleInOpenScope(node.Value) {
		needsRedeclare = true
	}

	// A declaration in a block that has already ended must be declared again,
	// even when it sits at the same indent as the original: sibling blocks are
	// separate C scopes, so the earlier declaration is not visible here. The
	// indent heuristic below cannot see that, and emitted an assignment to a
	// variable that C could not resolve.
	if node.Type == ahoy.NODE_VARIABLE_DECLARATION && isDeclared && !gen.nameVisibleInOpenScope(node.Value) {
		needsRedeclare = true
	}

	canRedeclare := isLoopLocalPattern || needsRedeclare

	if !isDeclared || canRedeclare {
		gen.markDeclaredInCurrentScope(node.Value)
	}

	if isDeclared && !canRedeclare {
		// Just assignment - mark RHS variables as escaping if being assigned to existing var
		gen.markEscapingVariables(node.Children[0])
		// Rebinding record: a later escape of this name protects owners
		// transitively. (Fresh RHS shapes are covered by the blanket mark
		// above; alias shapes resolve through this record on escape.)
		gen.valueBindings[node.Value] = node.Children[0]

		// Check if we're reassigning a heap-allocated variable - need to free old value first.
		// This runs at every scope depth including function level: the
		// end-of-function cleanup only frees the latest binding, so without
		// this every overwritten binding leaks. Never free eagerly when the
		// old value may still be referenced: an
		// escaped value (stored, returned, pushed) or a live declaration alias
		// (`y: x`) would dangle. Skipping leaks the old value instead, which
		// is always the safe direction.
		varName := node.Value
		if gen.currentFunction != "" && gen.heapAllocatedVars[varName] && !gen.functionParameters[varName] && !gen.escapingVars[varName] && len(gen.aliasEdges[varName]) == 0 {
			// Get the variable type
			varType := gen.heapVarTypes[varName]
			if varType == "" {
				if t, exists := gen.functionVars[varName]; exists {
					varType = t
				}
			}

			// Generate free code before reassignment
			if varType != "" {
				freeCode := gen.generateFreeCodeForVar(varName, varType)
				if freeCode != "" {
					gen.writeIndent()
					gen.output.WriteString(freeCode)
					// Mark as already freed so we don't free again at function end
					gen.autoFreedVars[varName] = true
				}
			}
		}

		if valueNode.Type == ahoy.NODE_SWITCH_STATEMENT {
			// Generate switch as expression (assign in each case)
			gen.generateSwitchExpression(valueNode, node.Value)
		} else if valueNode.Type == ahoy.NODE_F_STRING && gen.currentFunction != "" {
			// F-strings evaluate to a per-site static buffer: binding it
			// directly aliases memory the next evaluation overwrites
			// (every loop iteration sees the last value). Duplicate so the
			// variable owns its copy (tracked as a fresh string below).
			gen.output.WriteString(node.Value)
			gen.output.WriteString(" = strdup(")
			gen.generateNode(node.Children[0])
			gen.output.WriteString(");\n")
		} else {
			gen.output.WriteString(node.Value)
			gen.output.WriteString(" = ")
			gen.generateNode(node.Children[0])
			gen.output.WriteString(";\n")
		}

		// After reassignment, track the new value if it's also heap-allocated
		if gen.currentFunction != "" {
			valueType := gen.inferType(valueNode)
			if gen.isHeapAllocatedType(valueType) {
				borrowSelect := gen.selectHasHeapAlias(valueNode)
				borrowCall := valueNode.Type == ahoy.NODE_CALL && gen.callAliasKind(valueNode.Value).isAlias
				if (valueNode.Type == ahoy.NODE_CALL && !borrowCall) || (valueNode.Type != ahoy.NODE_IDENTIFIER &&
					valueNode.Type != ahoy.NODE_MEMBER_ACCESS && valueNode.Type != ahoy.NODE_ARRAY_ACCESS &&
					valueNode.Type != ahoy.NODE_DICT_ACCESS && valueNode.Type != ahoy.NODE_OBJECT_ACCESS &&
					!borrowSelect) {
					// Without a declaration entry (literal/static-initialized
					// or global) there is no known safe scope: skip
					// registration (leak, sound). Registering at the
					// current inner depth would free memory the variable's
					// scope outlives (its owner, or a later read).
					if _, hasDecl := gen.heapVarScopes[varName]; hasDecl {
						// Clear the autoFreed flag since we have a new allocation
						delete(gen.autoFreedVars, varName)
						// Schedule cleanup at the scope where the variable was
						// declared, never the current inner scope: a variable
						// assigned inside a nested block outlives that block, so
						// freeing at the block exit leaves a dangling reference
						// behind (heap use-after-free on the next use).
						targetDepth := gen.scopeDepth
						if declDepth, ok := gen.heapVarScopes[varName]; ok && declDepth < targetDepth {
							targetDepth = declDepth
						}
						gen.heapVarScopes[varName] = targetDepth
						gen.heapVarTypes[varName] = valueType
						// Add to the declaration scope's allocations if in nested scope
						if targetDepth > 0 {
							gen.scopeAllocations[targetDepth] = append(gen.scopeAllocations[targetDepth], varName)
						}
					}
				} else if valueNode.Type == ahoy.NODE_MEMBER_ACCESS || valueNode.Type == ahoy.NODE_ARRAY_ACCESS ||
					valueNode.Type == ahoy.NODE_DICT_ACCESS || valueNode.Type == ahoy.NODE_OBJECT_ACCESS ||
					borrowSelect || borrowCall {
					// Borrowing a container's storage (or aliasing a select
					// branch): stop tracking, or scope exit would free memory
					// owned elsewhere. (Stale allocation entries are skipped
					// once the type is gone.)
					delete(gen.heapAllocatedVars, varName)
					delete(gen.heapVarTypes, varName)
					delete(gen.heapVarScopes, varName)
				}
			} else if gen.valueAllocatesString(valueNode) || valueNode.Type == ahoy.NODE_F_STRING {
				// Fresh string assigned (concatenation, sprintf, methods —
				// or a bound f-string, duplicated at emission above): track it unless the variable is
				// explicitly a non-string type (a type error reported
				// elsewhere; don't compound it with a bad free).
				if oldType, ok := gen.heapVarTypes[varName]; !ok || isStringLikeType(oldType) ||
					oldType == "" || oldType == "unknown" || oldType == "any" || oldType == "generic" {
					// Without a declaration entry there is no known safe
					// scope: skip registration (leak, sound) — see above.
					if _, hasDecl := gen.heapVarScopes[varName]; hasDecl {
						delete(gen.autoFreedVars, varName)
						targetDepth := gen.scopeDepth
						if declDepth, ok := gen.heapVarScopes[varName]; ok && declDepth < targetDepth {
							targetDepth = declDepth
						}
						gen.heapVarScopes[varName] = targetDepth
						gen.heapVarTypes[varName] = "string"
						if targetDepth > 0 {
							gen.scopeAllocations[targetDepth] = append(gen.scopeAllocations[targetDepth], varName)
						}
					}
				}
			} else if oldType, ok := gen.heapVarTypes[varName]; ok && isStringLikeType(oldType) {
				// Reassigned to a static value: stop tracking, or scope
				// exit would free non-heap memory. (Stale allocation
				// entries are skipped once the type is gone.)
				delete(gen.heapAllocatedVars, varName)
				delete(gen.heapVarTypes, varName)
				delete(gen.heapVarScopes, varName)
			}
		}
	} else {
		// Type inference and declaration
		valueNode := node.Children[0]

		// Check if we have an explicit type annotation
		explicitType := node.DataType

		// Special handling for object literals - they define their own type inline
		if valueNode.Type == ahoy.NODE_OBJECT_LITERAL {
			// Record for transitive escape protection (struct-held
			// closures/aliases resolve through this on escape).
			gen.valueBindings[node.Value] = valueNode
			// Check if this is a typed struct literal (e.g., Color{...})
			if valueNode.Value != "" {
				// Use mapType to get the correct C type name
				structName := gen.mapType(valueNode.Value)
				gen.output.WriteString(structName)
				gen.output.WriteString(" ")
				gen.output.WriteString(node.Value)
				gen.output.WriteString(" = ")
				gen.generateNode(valueNode)
				gen.output.WriteString(";\n")

				// Track variable in appropriate scope
				if gen.currentFunction != "" && gen.functionVars != nil {
					gen.functionVars[node.Value] = valueNode.Value
				} else {
					gen.variables[node.Value] = valueNode.Value
				}
				// Mark as declared
				if inFunction {
					gen.declaredFunctionVars[node.Value] = true
				} else {
					gen.declaredGlobalVars[node.Value] = true
				}
			} else {
				// Anonymous struct - create a named struct for it
				anonStructName := fmt.Sprintf("__anon_struct_%s", node.Value)

				// Generate typedef struct
				gen.funcDecls.WriteString("typedef struct {\n")

				// Track struct fields
				structInfo := &StructInfo{
					Name:   anonStructName,
					Fields: make([]StructField, 0),
				}

				for _, prop := range valueNode.Children {
					if prop.Type == ahoy.NODE_OBJECT_PROPERTY {
						propType := gen.inferType(prop.Children[0])
						cType := gen.mapType(propType)
						gen.funcDecls.WriteString("    ")
						gen.funcDecls.WriteString(cType)
						gen.funcDecls.WriteString(" ")
						gen.funcDecls.WriteString(prop.Value)
						gen.funcDecls.WriteString(";\n")

						structInfo.Fields = append(structInfo.Fields, StructField{
							Name: prop.Value,
							Type: cType,
						})
					}
				}
				gen.funcDecls.WriteString("} ")
				gen.funcDecls.WriteString(anonStructName)
				gen.funcDecls.WriteString(";\n\n")
				gen.structs[anonStructName] = structInfo

				// Generate variable declaration
				gen.output.WriteString(anonStructName)
				gen.output.WriteString(" ")
				gen.output.WriteString(node.Value)
				gen.output.WriteString(" = ")
				gen.generateNode(valueNode)
				gen.output.WriteString(";\n")

				// Track variable in appropriate scope
				if gen.currentFunction != "" && gen.functionVars != nil {
					gen.functionVars[node.Value] = anonStructName
				} else {
					gen.variables[node.Value] = anonStructName
				}
				// Mark as declared
				if inFunction {
					gen.declaredFunctionVars[node.Value] = true
				} else {
					gen.declaredGlobalVars[node.Value] = true
				}
			}
		} else {
			varType := gen.inferType(valueNode)

			// Use explicit type if provided, otherwise infer
			if explicitType != "" {
				varType = explicitType
			}

			// Track variable in appropriate scope
			if gen.currentFunction != "" && gen.functionVars != nil {
				// Inside a function - use function scope
				gen.functionVars[node.Value] = varType
				// Mark as nested scope if indent > 1 (inside loops/ifs)
				// and track the indent level where this variable was declared
				if gen.indent > 1 {
					gen.nestedScopeVars[node.Value] = true
					gen.varDeclIndent[node.Value] = gen.indent
				}
			} else {
				// Global scope
				gen.variables[node.Value] = varType
			}
			// Mark as declared
			if gen.currentFunction != "" {
				gen.declaredFunctionVars[node.Value] = true
			} else {
				gen.declaredGlobalVars[node.Value] = true
			}

			// If this is an array literal with typed annotation, track the element type
			if valueNode.Type == ahoy.NODE_ARRAY_LITERAL {
				if explicitType != "" && strings.HasPrefix(explicitType, "array[") {
					// Extract element type from array[type]
					elemType := strings.TrimSuffix(strings.TrimPrefix(explicitType, "array["), "]")
					gen.arrayElementTypes[node.Value] = elemType
					// Set context for array literal generation
					gen.currentTypeContext = explicitType

					// For 2D arrays (array[array[type]]), also extract inner element type
					if strings.HasPrefix(elemType, "array[") {
						nestedElemType := strings.TrimSuffix(strings.TrimPrefix(elemType, "array["), "]")
						gen.array2DElementTypes[node.Value] = nestedElemType
					}
				} else if len(valueNode.Children) > 0 {
					elemType := gen.inferType(valueNode.Children[0])
					gen.arrayElementTypes[node.Value] = elemType

					// For 2D arrays: if the first element is an array variable, track its element type
					firstChild := valueNode.Children[0]
					if firstChild.Type == ahoy.NODE_IDENTIFIER {
						if nestedElemType, exists := gen.arrayElementTypes[firstChild.Value]; exists {
							gen.array2DElementTypes[node.Value] = nestedElemType
						}
					}
				}
			}

			// For any variable with array type (from inference or explicit type), track element type
			// This handles cases like: new_texts: execute_combat_phase|...| where return type is array[FloatingText]
			// Last write wins (matching functionVars): an earlier same-named
			// array must not shadow the current declaration's type.
			if strings.HasPrefix(varType, "array[") && strings.HasSuffix(varType, "]") {
				elemType := strings.TrimSuffix(strings.TrimPrefix(varType, "array["), "]")
				gen.arrayElementTypes[node.Value] = elemType

				// For 2D arrays (array[array[type]]), also extract inner element type
				if strings.HasPrefix(elemType, "array[") {
					nestedElemType := strings.TrimSuffix(strings.TrimPrefix(elemType, "array["), "]")
					gen.array2DElementTypes[node.Value] = nestedElemType
				}
			}

			cType := gen.mapType(varType)

			// Check if value is a switch expression
			if valueNode.Type == ahoy.NODE_SWITCH_STATEMENT {
				// Generate switch as expression (assign in each case)
				gen.output.WriteString(cType)
				gen.output.WriteString(" ")
				gen.output.WriteString(node.Value)
				gen.output.WriteString(";\n")
				gen.generateSwitchExpression(valueNode, node.Value)
			} else if valueNode.Type == ahoy.NODE_F_STRING && gen.currentFunction != "" {
				// Bound f-string: duplicate the static buffer so the
				// variable owns its copy (tracked as a fresh string below).
				gen.output.WriteString(cType)
				gen.output.WriteString(" ")
				gen.output.WriteString(node.Value)
				gen.output.WriteString(" = strdup(")
				gen.generateNode(valueNode)
				gen.output.WriteString(");\n")
			} else {
				gen.output.WriteString(cType)
				gen.output.WriteString(" ")
				gen.output.WriteString(node.Value)
				gen.output.WriteString(" = ")
				gen.generateNode(valueNode)
				gen.output.WriteString(";\n")
			}

			// Track if this variable came from dict access
			if valueNode.Type == ahoy.NODE_DICT_ACCESS {
				gen.dictSourcedVars[node.Value] = valueNode.Value // dict name
				if len(valueNode.Children) > 0 && valueNode.Children[0].Type == ahoy.NODE_STRING {
					gen.dictSourcedKeys[node.Value] = valueNode.Children[0].Value // key
				}
			}

			// Track heap-allocated variables in function scope at any scope level
			// Variables will be freed when their scope exits
			// Only track if the value is actually allocating new memory, not just aliasing
			if gen.currentFunction != "" && gen.isHeapAllocatedType(varType) {
				// Check if valueNode is creating new memory (literal or function call) vs just aliasing (identifier)
				if valueNode.Type == ahoy.NODE_CALL {
					// Function call that returns heap-allocated type - track it,
					// unless the callee aliases (passthrough/getter): the
					// result shares storage with an argument or immortal, so
					// freeing it here would corrupt its owner. Escape sites
					// protect alias arguments transitively instead.
					if !gen.callAliasKind(valueNode.Value).isAlias {
						gen.registerHeapAllocation(node.Value, varType)
					}
				} else if valueNode.Type != ahoy.NODE_IDENTIFIER &&
					valueNode.Type != ahoy.NODE_MEMBER_ACCESS && valueNode.Type != ahoy.NODE_ARRAY_ACCESS &&
					valueNode.Type != ahoy.NODE_DICT_ACCESS && valueNode.Type != ahoy.NODE_OBJECT_ACCESS &&
					!gen.selectHasHeapAlias(valueNode) {
					// Literal (array, dict, object) - track it. Accesses borrow
					// their container's storage (no copy), so they alias and
					// must never be freed: freeing one corrupts its owner.
					// Selects/switches that may alias a branch likewise own
					// nothing (the branch owner frees); all-fresh ones own
					// their selected temp and are tracked.
					gen.registerHeapAllocation(node.Value, varType)
				}
			}
			// Strings need allocation-site precision: literals, f-strings and
			// aliases are static or shared and must never be freed; only
			// fresh values (concatenation, sprintf, fresh-returning
			// methods) are tracked. The type guard keeps explicitly
			// non-string declarations out even if the value allocates.
			if gen.currentFunction != "" && (gen.valueAllocatesString(valueNode) || valueNode.Type == ahoy.NODE_F_STRING) &&
				(isStringLikeType(varType) || varType == "" || varType == "unknown" || varType == "any" || varType == "generic") {
				gen.registerHeapAllocation(node.Value, "string")
			}

			// Declaration-alias tracking for the reassign path below: `y: x`
			// shares x's storage, so reassigning x while y is live must not
			// free eagerly. Edges are pruned when the alias's scope exits.
			if gen.currentFunction != "" && valueNode.Type == ahoy.NODE_IDENTIFIER && gen.heapAllocatedVars[valueNode.Value] {
				edges := gen.aliasEdges[valueNode.Value]
				if edges == nil {
					edges = make(map[string]int)
					gen.aliasEdges[valueNode.Value] = edges
				}
				edges[node.Value] = gen.scopeDepth
			}

			// Closure tracking for the escape path: `f: (x): ... + y`
			// binds a value whose environment borrows y. If f later escapes
			// (returned, stored, pushed, submitted), y must survive it.
			// Aliases propagate (`g: f` carries f's captures).
			if valueNode.Type == ahoy.NODE_LAMBDA {
				gen.lambdaValues[node.Value] = valueNode
			} else if valueNode.Type == ahoy.NODE_IDENTIFIER {
				if src, ok := gen.lambdaValues[valueNode.Value]; ok {
					gen.lambdaValues[node.Value] = src
				}
			}
			// Value bindings for transitive escape protection: every bound
			// RHS is recorded so a later escape of this name can protect
			// its ultimate owners (aliases of aliases, select branches).
			gen.valueBindings[node.Value] = valueNode

			// Clear type context after generation
			gen.currentTypeContext = ""
		}
	}
}

// generateTestCondition emits a boolean-test expression. Bool-typed calls
// in test position go through a bool temporary: some C compilers misread
// an indirect external _Bool call tested directly (high garbage bits), so
// `if fn()|` could read true while `t: fn()| then if t` reads correctly.
// The 1-byte store+reload of the temporary narrows to the true low byte.
// Only bool-typed calls are rewritten (an int condition like `if f()` keeps
// full-width truthiness); `not`/`and`/`or` recurse so each tested call is
// still evaluated lazily in place, preserving short-circuit behavior.
// Anything else (comparisons, arithmetic, identifiers) emits unchanged.
func (gen *CodeGenerator) generateTestCondition(node *ahoy.ASTNode) {
	if node == nil {
		return
	}
	if (node.Type == ahoy.NODE_CALL || node.Type == ahoy.NODE_METHOD_CALL) && gen.inferType(node) == "bool" {
		tmp := fmt.Sprintf("__tcc_cond_%d", gen.varCounter)
		gen.varCounter++
		gen.output.WriteString("({ bool " + tmp + " = ")
		gen.generateNode(node)
		gen.output.WriteString("; " + tmp + "; })")
		return
	}
	if node.Type == ahoy.NODE_UNARY_OP && node.Value == "not" && len(node.Children) == 1 {
		gen.output.WriteString("(!")
		gen.generateTestCondition(node.Children[0])
		gen.output.WriteString(")")
		return
	}
	if node.Type == ahoy.NODE_BINARY_OP && len(node.Children) == 2 && (node.Value == "and" || node.Value == "or") {
		op := " && "
		if node.Value == "or" {
			op = " || "
		}
		gen.output.WriteString("(")
		gen.generateTestCondition(node.Children[0])
		gen.output.WriteString(op)
		gen.generateTestCondition(node.Children[1])
		gen.output.WriteString(")")
		return
	}
	gen.generateNode(node)
}

func (gen *CodeGenerator) generateIfStatement(node *ahoy.ASTNode) {
	gen.writeIndent()
	gen.output.WriteString("if (")
	gen.generateTestCondition(node.Children[0])
	gen.output.WriteString(") {\n")

	gen.indent++
	gen.enterScope()
	gen.generateNodeInternal(node.Children[1], false)
	cleanup := gen.exitScope()
	if cleanup != "" {
		gen.output.WriteString(cleanup)
	}
	gen.indent--

	gen.writeIndent()
	gen.output.WriteString("}")

	// Handle elseif and else
	i := 2
	for i < len(node.Children) {
		// Check if this is an else (no condition)
		if i == len(node.Children)-1 {
			// Last child is else body
			gen.output.WriteString(" else {\n")
			gen.indent++
			gen.enterScope()
			gen.generateNodeInternal(node.Children[i], false)
			cleanup := gen.exitScope()
			if cleanup != "" {
				gen.output.WriteString(cleanup)
			}
			gen.indent--
			gen.writeIndent()
			gen.output.WriteString("}")
			break
		} else {
			// Elseif: condition and body pair
			gen.output.WriteString(" else if (")
			gen.generateTestCondition(node.Children[i])
			gen.output.WriteString(") {\n")
			gen.indent++
			gen.enterScope()
			gen.generateNodeInternal(node.Children[i+1], false)
			cleanup := gen.exitScope()
			if cleanup != "" {
				gen.output.WriteString(cleanup)
			}
			gen.indent--
			gen.writeIndent()
			gen.output.WriteString("}")
			i += 2
		}
	}

	gen.output.WriteString("\n")
}

// generateSwitchExpression generates a switch that assigns to a variable (expression context)
func (gen *CodeGenerator) generateSwitchExpression(node *ahoy.ASTNode, targetVar string) {
	switchExpr := node.Children[0]
	switchExprType := gen.inferType(switchExpr)

	// Check if this is a string switch - need to use if-else with strcmp
	if switchExprType == "char*" || switchExprType == "string" {
		gen.generateStringSwitchExpression(node, targetVar)
		return
	}

	// A C switch can only test equality, so any range case turns the whole
	// switch into an if/else chain (see generateRangeSwitchExpression).
	if switchHasRange(node) {
		gen.generateRangeSwitchExpression(node, targetVar)
		return
	}

	// Generate normal switch with assignments in each case
	gen.writeIndent()
	gen.output.WriteString("switch (")
	gen.generateNode(switchExpr)
	gen.output.WriteString(") {\n")

	// Generate cases
	for i := 1; i < len(node.Children); i++ {
		caseNode := node.Children[i]
		if caseNode.Type == ahoy.NODE_SWITCH_CASE {
			caseValue := caseNode.Children[0]
			caseBody := caseNode.Children[1]

			// Check if it's a list of cases
			if caseValue.Type == ahoy.NODE_SWITCH_CASE_LIST {
				// Multiple cases
				for _, val := range caseValue.Children {
					gen.indent++
					gen.writeIndent()
					gen.output.WriteString("case ")
					// Resolve constants/dot-enums like statement switches
					gen.generateSwitchCaseValue(val, switchExprType)
					gen.output.WriteString(":\n")
					gen.indent--
				}
				// Case bodies get braces: a C label may only precede a statement,
				// and the braces scope heap locals declared in the body (see
				// generateSwitchCaseAssignment).
				gen.indent++
				gen.writeIndent()
				gen.output.WriteString("{\n")
				gen.indent++
				gen.generateSwitchCaseAssignment(caseBody, targetVar)
				gen.writeIndent()
				gen.output.WriteString("break;\n")
				gen.indent--
				gen.writeIndent()
				gen.output.WriteString("}\n")
				gen.indent--
			} else {
				// Single case or default
				gen.indent++
				gen.writeIndent()

				if caseValue.Type == ahoy.NODE_IDENTIFIER && caseValue.Value == "_" {
					gen.output.WriteString("default: {\n")
				} else {
					gen.output.WriteString("case ")
					// Resolve constants/dot-enums like statement switches
					gen.generateSwitchCaseValue(caseValue, switchExprType)
					gen.output.WriteString(": {\n")
				}

				gen.indent++
				gen.generateSwitchCaseAssignment(caseBody, targetVar)
				gen.writeIndent()
				gen.output.WriteString("break;\n")
				gen.indent--
				gen.writeIndent()
				gen.output.WriteString("}\n")
				gen.indent--
			}
		}
	}

	gen.writeIndent()
	gen.output.WriteString("}\n")
}

// switchHasRange reports whether any case of a switch is a range case
// (`on 1 to 5:`). C switch labels can only test equality, so a switch with
// ranges must be generated as an if/else chain instead.
func switchHasRange(node *ahoy.ASTNode) bool {
	for i := 1; i < len(node.Children); i++ {
		caseNode := node.Children[i]
		if caseNode.Type == ahoy.NODE_SWITCH_CASE && len(caseNode.Children) > 0 &&
			caseNode.Children[0].Type == ahoy.NODE_SWITCH_CASE_RANGE {
			return true
		}
	}
	return false
}

// switchResultType picks the C type for a switch-evaluation temporary, as
// used by `return switch x: ...`. It prefers the enclosing function's declared
// return type (the cases must conform to it, just like the supported
// `out: T = switch...; return out` pattern) and falls back to the first case
// body's value type.
func (gen *CodeGenerator) switchResultType(sw *ahoy.ASTNode) string {
	if gen.currentFunction != "" {
		if types, ok := gen.functionReturnTypes[gen.currentFunction]; ok && len(types) > 0 {
			if t := gen.mapType(types[0]); t != "" && t != "void" {
				return t
			}
		}
	}
	for i := 1; i < len(sw.Children); i++ {
		caseNode := sw.Children[i]
		if caseNode.Type != ahoy.NODE_SWITCH_CASE || len(caseNode.Children) < 2 {
			continue
		}
		body := caseNode.Children[1]
		valNode := body
		if body.Type == ahoy.NODE_BLOCK && len(body.Children) > 0 {
			valNode = body.Children[len(body.Children)-1]
		}
		if t := gen.inferType(valNode); t != "" && t != "unknown" {
			if mapped := gen.mapType(t); mapped != "" && mapped != "void" {
				return mapped
			}
		}
	}
	return "int"
}

// generateRangeSwitchExpression generates an if/else chain for a switch that
// contains range cases, assigning the matching body to targetVar. Plain cases
// keep their source order, so an earlier case wins over a later overlapping
// range - the same order a C switch would try them in.
func (gen *CodeGenerator) generateRangeSwitchExpression(node *ahoy.ASTNode, targetVar string) {
	switchExpr := node.Children[0]
	switchExprType := gen.inferType(switchExpr)

	first := true
	hasDefault := false
	var defaultBody *ahoy.ASTNode

	for i := 1; i < len(node.Children); i++ {
		caseNode := node.Children[i]
		if caseNode.Type == ahoy.NODE_SWITCH_CASE {
			caseValue := caseNode.Children[0]
			caseBody := caseNode.Children[1]

			// Check for default case
			if caseValue.Type == ahoy.NODE_IDENTIFIER && caseValue.Value == "_" {
				hasDefault = true
				defaultBody = caseBody
				continue
			}

			gen.writeIndent()
			if first {
				gen.output.WriteString("if (")
				first = false
			} else {
				gen.output.WriteString("else if (")
			}

			gen.generateRangeCondition(switchExpr, switchExprType, caseValue)

			gen.output.WriteString(") {\n")
			gen.indent++
			gen.generateSwitchCaseAssignment(caseBody, targetVar)
			gen.indent--
			gen.writeIndent()
			gen.output.WriteString("}")
		}
	}

	// Handle default case
	if hasDefault {
		if !first {
			gen.output.WriteString(" else {\n")
		} else {
			gen.writeIndent()
			gen.output.WriteString("{\n")
		}
		gen.indent++
		gen.generateSwitchCaseAssignment(defaultBody, targetVar)
		gen.indent--
		gen.writeIndent()
		gen.output.WriteString("}\n")
	} else if !first {
		gen.output.WriteString("\n")
	}
}

// generateRangeCondition emits the C condition matching one case value:
// equality for single/list values, a bounded comparison for ranges.
func (gen *CodeGenerator) generateRangeCondition(switchExpr *ahoy.ASTNode, switchExprType string, caseValue *ahoy.ASTNode) {
	if caseValue.Type == ahoy.NODE_SWITCH_CASE_LIST {
		for j, val := range caseValue.Children {
			if j > 0 {
				gen.output.WriteString(" || ")
			}
			gen.generateNode(switchExpr)
			gen.output.WriteString(" == ")
			gen.generateSwitchCaseValue(val, switchExprType)
		}
		return
	}
	if caseValue.Type == ahoy.NODE_SWITCH_CASE_RANGE {
		gen.output.WriteString("(")
		gen.generateNode(switchExpr)
		gen.output.WriteString(" >= ")
		gen.generateNode(caseValue.Children[0])
		gen.output.WriteString(" && ")
		gen.generateNode(switchExpr)
		gen.output.WriteString(" <= ")
		gen.generateNode(caseValue.Children[1])
		gen.output.WriteString(")")
		return
	}
	gen.generateNode(switchExpr)
	gen.output.WriteString(" == ")
	gen.generateSwitchCaseValue(caseValue, switchExprType)
}

// generateSwitchCaseAssignment generates an assignment for a case body.
// The body runs in its own scope: heap locals declared by intermediate
// statements are freed after the assignment, so the function-end cleanup
// never names a variable that is already out of scope. The yielded value
// itself escapes into targetVar (which outlives the case), so its storage
// is protected exactly like a return value instead of being freed here.
func (gen *CodeGenerator) generateSwitchCaseAssignment(caseBody *ahoy.ASTNode, targetVar string) {
	gen.enterSwitchCaseScope()
	valueNode := caseBody
	// Check if body is a block with multiple statements
	if caseBody.Type == ahoy.NODE_BLOCK && len(caseBody.Children) > 0 {
		// Execute all statements except last, then assign last
		for i := 0; i < len(caseBody.Children)-1; i++ {
			gen.generateNodeInternal(caseBody.Children[i], true)
		}
		// Last statement is the return value
		valueNode = caseBody.Children[len(caseBody.Children)-1]
	}
	// A halt/next yields no value (control leaves the case): run it as a
	// statement with no `targetVar = ` prefix, which would be invalid C.
	// (Like `return` in this position, later uses of targetVar on the
	// halted path are the author's responsibility.)
	if valueNode.Type == ahoy.NODE_HALT || valueNode.Type == ahoy.NODE_NEXT {
		gen.generateNodeInternal(valueNode, true)
	} else {
		gen.writeIndent()
		gen.output.WriteString(targetVar)
		gen.output.WriteString(" = ")
		gen.generateNode(valueNode)
		gen.output.WriteString(";\n")
	}
	// Ownership of the yielded value transfers to targetVar: protect its
	// storage (and its transitive dependencies) like a return value. Anything
	// else the body allocated is freed by the scope exit below.
	gen.markReturnedValue(valueNode)
	gen.markValueDeps(valueNode)
	if cleanup := gen.exitSwitchCaseScope(); cleanup != "" {
		gen.output.WriteString(cleanup)
	}
}

// generateStringSwitchExpression generates if-else chain for string switches
func (gen *CodeGenerator) generateStringSwitchExpression(node *ahoy.ASTNode, targetVar string) {
	switchExpr := node.Children[0]

	first := true
	hasDefault := false
	var defaultBody *ahoy.ASTNode

	for i := 1; i < len(node.Children); i++ {
		caseNode := node.Children[i]
		if caseNode.Type == ahoy.NODE_SWITCH_CASE {
			caseValue := caseNode.Children[0]
			caseBody := caseNode.Children[1]

			// Check for default case
			if caseValue.Type == ahoy.NODE_IDENTIFIER && caseValue.Value == "_" {
				hasDefault = true
				defaultBody = caseBody
				continue
			}

			gen.writeIndent()
			if first {
				gen.output.WriteString("if (")
				first = false
			} else {
				gen.output.WriteString("else if (")
			}

			// Handle multiple cases
			if caseValue.Type == ahoy.NODE_SWITCH_CASE_LIST {
				for j, val := range caseValue.Children {
					if j > 0 {
						gen.output.WriteString(" || ")
					}
					gen.output.WriteString("strcmp(")
					gen.generateNode(switchExpr)
					gen.output.WriteString(", ")
					gen.generateNode(val)
					gen.output.WriteString(") == 0")
				}
			} else if caseValue.Type == ahoy.NODE_SWITCH_CASE_RANGE {
				// Lexicographic range: lo <= x <= hi.
				gen.output.WriteString("(strcmp(")
				gen.generateNode(switchExpr)
				gen.output.WriteString(", ")
				gen.generateNode(caseValue.Children[0])
				gen.output.WriteString(") >= 0 && strcmp(")
				gen.generateNode(switchExpr)
				gen.output.WriteString(", ")
				gen.generateNode(caseValue.Children[1])
				gen.output.WriteString(") <= 0)")
			} else {
				gen.output.WriteString("strcmp(")
				gen.generateNode(switchExpr)
				gen.output.WriteString(", ")
				gen.generateNode(caseValue)
				gen.output.WriteString(") == 0")
			}

			gen.output.WriteString(") {\n")
			gen.indent++
			gen.generateSwitchCaseAssignment(caseBody, targetVar)
			gen.indent--
			gen.writeIndent()
			gen.output.WriteString("}")
		}
	}

	// Handle default case
	if hasDefault {
		if !first {
			gen.output.WriteString(" else {\n")
		} else {
			gen.writeIndent()
			gen.output.WriteString("{\n")
		}
		gen.indent++
		gen.generateSwitchCaseAssignment(defaultBody, targetVar)
		gen.indent--
		gen.writeIndent()
		gen.output.WriteString("}\n")
	} else if !first {
		gen.output.WriteString("\n")
	}
}

// generateStringSwitchStatement generates if-else chain for string/char switches in statement context
func (gen *CodeGenerator) generateStringSwitchStatement(node *ahoy.ASTNode) {
	switchExpr := node.Children[0]
	switchExprType := gen.inferType(switchExpr)

	first := true
	hasDefault := false
	var defaultBody *ahoy.ASTNode

	for i := 1; i < len(node.Children); i++ {
		caseNode := node.Children[i]
		if caseNode.Type == ahoy.NODE_SWITCH_CASE {
			caseValue := caseNode.Children[0]
			caseBody := caseNode.Children[1]

			// Check for default case
			if caseValue.Type == ahoy.NODE_IDENTIFIER && caseValue.Value == "_" {
				hasDefault = true
				defaultBody = caseBody
				continue
			}

			gen.writeIndent()
			if first {
				gen.output.WriteString("if (")
				first = false
			} else {
				gen.output.WriteString("else if (")
			}

			// Handle multiple cases
			if caseValue.Type == ahoy.NODE_SWITCH_CASE_LIST {
				for j, val := range caseValue.Children {
					if j > 0 {
						gen.output.WriteString(" || ")
					}
					// For char type, use direct comparison
					if switchExprType == "char" {
						gen.generateNode(switchExpr)
						gen.output.WriteString(" == ")
						gen.generateNode(val)
					} else {
						// For string type, use strcmp
						gen.output.WriteString("strcmp(")
						gen.generateNode(switchExpr)
						gen.output.WriteString(", ")
						gen.generateNode(val)
						gen.output.WriteString(") == 0")
					}
				}
			} else if caseValue.Type == ahoy.NODE_SWITCH_CASE_RANGE {
				// Range case. Chars compare directly; strings compare
				// lexicographically, mirroring the `to` ordering.
				if switchExprType == "char" {
					gen.output.WriteString("(")
					gen.generateNode(switchExpr)
					gen.output.WriteString(" >= ")
					gen.generateNode(caseValue.Children[0])
					gen.output.WriteString(" && ")
					gen.generateNode(switchExpr)
					gen.output.WriteString(" <= ")
					gen.generateNode(caseValue.Children[1])
					gen.output.WriteString(")")
				} else {
					gen.output.WriteString("(strcmp(")
					gen.generateNode(switchExpr)
					gen.output.WriteString(", ")
					gen.generateNode(caseValue.Children[0])
					gen.output.WriteString(") >= 0 && strcmp(")
					gen.generateNode(switchExpr)
					gen.output.WriteString(", ")
					gen.generateNode(caseValue.Children[1])
					gen.output.WriteString(") <= 0)")
				}
			} else {
				// Single case value
				if switchExprType == "char" {
					gen.generateNode(switchExpr)
					gen.output.WriteString(" == ")
					gen.generateNode(caseValue)
				} else {
					gen.output.WriteString("strcmp(")
					gen.generateNode(switchExpr)
					gen.output.WriteString(", ")
					gen.generateNode(caseValue)
					gen.output.WriteString(") == 0")
				}
			}

			gen.output.WriteString(") {\n")
			gen.indent++
			gen.enterSwitchCaseScope()
			gen.generateNodeInternal(caseBody, true) // Case body
			cleanup := gen.exitSwitchCaseScope()
			if cleanup != "" {
				gen.output.WriteString(cleanup)
			}
			gen.indent--
			gen.writeIndent()
			gen.output.WriteString("}")
		}
	}

	// Handle default case
	if hasDefault {
		if !first {
			gen.output.WriteString(" else {\n")
		} else {
			gen.writeIndent()
			gen.output.WriteString("{\n")
		}
		gen.indent++
		gen.enterSwitchCaseScope()
		gen.generateNodeInternal(defaultBody, true) // Default body
		cleanup := gen.exitSwitchCaseScope()
		if cleanup != "" {
			gen.output.WriteString(cleanup)
		}
		gen.indent--
		gen.writeIndent()
		gen.output.WriteString("}\n")
	} else if !first {
		gen.output.WriteString("\n")
	}
}

// generateRangeSwitchStatement generates an if/else chain for a switch that
// contains range cases (statement context). See generateRangeSwitchExpression
// for the ordering rationale.
func (gen *CodeGenerator) generateRangeSwitchStatement(node *ahoy.ASTNode) {
	switchExpr := node.Children[0]
	switchExprType := gen.inferType(switchExpr)

	first := true
	hasDefault := false
	var defaultBody *ahoy.ASTNode

	for i := 1; i < len(node.Children); i++ {
		caseNode := node.Children[i]
		if caseNode.Type == ahoy.NODE_SWITCH_CASE {
			caseValue := caseNode.Children[0]
			caseBody := caseNode.Children[1]

			// Check for default case
			if caseValue.Type == ahoy.NODE_IDENTIFIER && caseValue.Value == "_" {
				hasDefault = true
				defaultBody = caseBody
				continue
			}

			gen.writeIndent()
			if first {
				gen.output.WriteString("if (")
				first = false
			} else {
				gen.output.WriteString("else if (")
			}

			gen.generateRangeCondition(switchExpr, switchExprType, caseValue)

			gen.output.WriteString(") {\n")
			gen.indent++
			gen.enterSwitchCaseScope()
			gen.generateNodeInternal(caseBody, true) // Case body
			cleanup := gen.exitSwitchCaseScope()
			if cleanup != "" {
				gen.output.WriteString(cleanup)
			}
			gen.indent--
			gen.writeIndent()
			gen.output.WriteString("}")
		}
	}

	// Handle default case
	if hasDefault {
		if !first {
			gen.output.WriteString(" else {\n")
		} else {
			gen.writeIndent()
			gen.output.WriteString("{\n")
		}
		gen.indent++
		gen.enterSwitchCaseScope()
		gen.generateNodeInternal(defaultBody, true) // Default body
		cleanup := gen.exitSwitchCaseScope()
		if cleanup != "" {
			gen.output.WriteString(cleanup)
		}
		gen.indent--
		gen.writeIndent()
		gen.output.WriteString("}\n")
	} else if !first {
		gen.output.WriteString("\n")
	}
}

func (gen *CodeGenerator) generateSwitchStatement(node *ahoy.ASTNode) {
	switchExpr := node.Children[0]
	switchExprType := gen.inferType(switchExpr)

	// Check if this is a string or char switch - need to use if-else
	if switchExprType == "char*" || switchExprType == "string" || switchExprType == "char" {
		gen.generateStringSwitchStatement(node)
		return
	}

	// A C switch can only test equality, so any range case turns the whole
	// switch into an if/else chain. Ranges used to be emitted as extra
	// `default:` labels, which is invalid C as soon as two appear.
	if switchHasRange(node) {
		gen.generateRangeSwitchStatement(node)
		return
	}

	// Generate normal C switch statement for integers
	gen.writeIndent()
	gen.output.WriteString("switch (")
	gen.generateNode(node.Children[0]) // Generate switch expression
	gen.output.WriteString(") {\n")

	// Generate cases (skip first child which is the switch expression)
	for i := 1; i < len(node.Children); i++ {
		caseNode := node.Children[i]
		if caseNode.Type == ahoy.NODE_SWITCH_CASE {
			caseValue := caseNode.Children[0]

			// Check if it's a list of cases or a range
			if caseValue.Type == ahoy.NODE_SWITCH_CASE_LIST {
				// Multiple cases - generate multiple case labels
				for _, val := range caseValue.Children {
					gen.indent++
					gen.writeIndent()
					gen.output.WriteString("case ")
					// Resolve dot-prefixed enum members
					gen.generateSwitchCaseValue(val, switchExprType)
					gen.output.WriteString(":\n")
					gen.indent--
				}
				// Generate body after all case labels. The body gets its own
				// braces: a C label may only precede a statement, so a case whose
				// first statement declares a local (a shared sprite branch, say)
				// would not compile without its own scope. The braces are a
				// scope too: heap locals declared in the body are freed here,
				// before break, so the function-end cleanup never names a
				// variable that is already out of scope.
				gen.indent++
				gen.writeIndent()
				gen.output.WriteString("{\n")
				gen.indent++
				gen.enterSwitchCaseScope()
				gen.generateNodeInternal(caseNode.Children[1], true) // Case body
				cleanup := gen.exitSwitchCaseScope()
				if cleanup != "" {
					gen.output.WriteString(cleanup)
				}
				gen.writeIndent()
				gen.output.WriteString("break;\n")
				gen.indent--
				gen.writeIndent()
				gen.output.WriteString("}\n")
				gen.indent--
			} else {
				// Single case value or default case
				gen.indent++
				gen.writeIndent()

				// Check if it's a default case (underscore)
				if caseValue.Type == ahoy.NODE_IDENTIFIER && caseValue.Value == "_" {
					gen.output.WriteString("default: {\n")
				} else {
					gen.output.WriteString("case ")
					// Resolve dot-prefixed enum members
					gen.generateSwitchCaseValue(caseValue, switchExprType)
					gen.output.WriteString(": {\n")
				}

				gen.indent++
				gen.enterSwitchCaseScope()
				gen.generateNodeInternal(caseNode.Children[1], true) // Case body
				cleanup := gen.exitSwitchCaseScope()
				if cleanup != "" {
					gen.output.WriteString(cleanup)
				}
				gen.writeIndent()
				gen.output.WriteString("break;\n")
				gen.indent--
				gen.writeIndent()
				gen.output.WriteString("}\n")
				gen.indent--
			}
		}
	}

	gen.writeIndent()
	gen.output.WriteString("}\n")
}

// caseLabelConstant folds a known constant to a C integer literal for use in
// a `case` label. Only integer (and bool) constants fold; floats, strings
// and unknown names return false. String switches generate if/else chains
// where the getter call is valid, so they never need this.
func (gen *CodeGenerator) caseLabelConstant(name string) (string, bool) {
	val, ok := gen.constantValues[name]
	if !ok {
		return "", false
	}
	switch v := val.(type) {
	case int64:
		return strconv.FormatInt(v, 10), true
	case int:
		return strconv.Itoa(v), true
	case bool:
		if v {
			return "1", true
		}
		return "0", true
	}
	return "", false
}

// sortedEnumNames returns the known enum names in sorted order, so that
// lookups spanning several enums resolve deterministically instead of taking
// whichever match map iteration finds first.
func (gen *CodeGenerator) sortedEnumNames() []string {
	names := make([]string, 0, len(gen.enumOriginalNames))
	for name := range gen.enumOriginalNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// generateSwitchCaseValue generates a case value, resolving dot-prefixed enum members
func (gen *CodeGenerator) generateSwitchCaseValue(val *ahoy.ASTNode, switchExprType string) {
	// A known integer constant folds to its literal: ahoy constants emit as
	// variables/getters (hot reload), which are not valid C `case` labels,
	// so `on TINT_RED:` must emit the value. C #defines and enum members
	// pass through untouched. Folding in conditions is equally valid.
	if val.Type == ahoy.NODE_IDENTIFIER && len(val.Value) > 0 && val.Value[0] != '.' {
		if lit, ok := gen.caseLabelConstant(val.Value); ok {
			gen.output.WriteString(lit)
			return
		}
	}

	// Check if this is a dot-prefixed enum member
	if val.Type == ahoy.NODE_IDENTIFIER && len(val.Value) > 0 && val.Value[0] == '.' {
		// Extract the member name (without the dot)
		memberName := val.Value[1:]

		// Try to find which enum this member belongs to
		// Only search using original enum names (lowercase)
		found := false
		for _, enumName := range gen.sortedEnumNames() {
			if gen.enums[enumName] != nil && gen.enums[enumName][memberName] {
				// Generate the C enum constant name using the original enum name
				gen.output.WriteString(enumName)
				gen.output.WriteString("_")
				gen.output.WriteString(memberName)
				found = true
				break
			}
		}

		if !found {
			// Fallback: just output the member name without dot
			gen.output.WriteString(memberName)
		}
		return
	}

	if val.Type == ahoy.NODE_MEMBER_ACCESS {
		// Handle direction.UP syntax - this goes through generateMemberAccess
		gen.generateMemberAccess(val)
		return
	}

	// Normal case value - generate as usual
	gen.generateNode(val)
}

func (gen *CodeGenerator) generateWhenStatement(node *ahoy.ASTNode) {
	gen.writeIndent()
	gen.output.WriteString("#ifdef ")
	gen.output.WriteString(node.Value)
	gen.output.WriteString("\n")

	gen.indent++
	gen.generateNodeInternal(node.Children[0], false)
	gen.indent--

	// Optional else branch (Children[1]) compiles to `#else`. Older ASTs
	// (and cache entries) carry only the body, so the length is checked.
	if len(node.Children) > 1 {
		gen.writeIndent()
		gen.output.WriteString("#else\n")

		gen.indent++
		gen.generateNodeInternal(node.Children[1], false)
		gen.indent--
	}

	gen.writeIndent()
	gen.output.WriteString("#endif\n")
}

func (gen *CodeGenerator) generateWhileLoop(node *ahoy.ASTNode) {
	gen.writeIndent()

	// Check if we have an explicit loop variable with initialization
	// Pattern 1: Children[0] is loop var, Children[1] is start, Children[2] is condition, Children[3] is body (loop i:start till condition)
	// Pattern 2: Children[0] is loop var, Children[1] is start (0), Children[2] is condition, Children[3] is body (loop i till condition)
	// Pattern 3: Children[0] is condition, Children[1] is body (loop till condition)
	var loopVar string
	var conditionNode *ahoy.ASTNode
	var bodyNode *ahoy.ASTNode
	var outerScopeStarted bool

	if len(node.Children) == 4 && node.Children[0].Type == ahoy.NODE_IDENTIFIER {
		// Pattern 1 or 2: loop i:start till condition or loop i till condition
		loopVar = node.Children[0].Value
		startNode := node.Children[1]
		conditionNode = node.Children[2]
		bodyNode = node.Children[3]

		// Create block scope for loop variable
		gen.output.WriteString("{\n")
		gen.indent++
		gen.enterScope()
		outerScopeStarted = true
		gen.writeIndent()

		// Initialize loop variable with start value
		gen.output.WriteString("int ")
		gen.output.WriteString(loopVar)
		gen.output.WriteString(" = ")
		gen.generateNode(startNode)
		gen.output.WriteString(";\n")
		gen.writeIndent()
	} else if len(node.Children) == 3 && node.Children[0].Type == ahoy.NODE_IDENTIFIER {
		// Old syntax: loop i till condition (without start value)
		loopVar = node.Children[0].Value
		conditionNode = node.Children[1]
		bodyNode = node.Children[2]

		// Create block scope for loop variable
		gen.output.WriteString("{\n")
		gen.indent++
		gen.enterScope()
		outerScopeStarted = true
		gen.writeIndent()

		// Initialize loop variable to 0
		gen.output.WriteString("int ")
		gen.output.WriteString(loopVar)
		gen.output.WriteString(" = 0;\n")
		gen.writeIndent()
	} else {
		// Pattern 3: loop till condition (no loop variable)
		conditionNode = node.Children[0]
		bodyNode = node.Children[1]
	}

	gen.output.WriteString("while (")
	gen.generateTestCondition(conditionNode)
	gen.output.WriteString(") {\n")

	gen.indent++
	gen.enterScope()
	gen.enterLoopScope() // Mark this as a loop boundary for break/continue
	gen.generateNodeInternal(bodyNode, false)

	// Increment loop variable if present
	if loopVar != "" {
		gen.writeIndent()
		gen.output.WriteString(loopVar)
		gen.output.WriteString("++;\n")
	}

	gen.exitLoopScope()
	cleanup := gen.exitScope()
	if cleanup != "" {
		gen.output.WriteString(cleanup)
	}
	gen.indent--

	gen.writeIndent()
	gen.output.WriteString("}\n")

	// Close block scope if we created one
	if outerScopeStarted {
		outerCleanup := gen.exitScope()
		if outerCleanup != "" {
			gen.output.WriteString(outerCleanup)
		}
		gen.indent--
		gen.writeIndent()
		gen.output.WriteString("}\n")
	}
	// A halt directly in a switch case jumps here (plain `break` would only
	// exit the switch). Emits nothing when no halt needed one.
	gen.emitLoopEndLabel()
}

// pushLoopVarType registers a counted-loop variable as int in function scope
// so type inference inside the loop body sees the loop variable's type, not
// whatever same-named local the flat maps remember (a stale string entry made
// float(k) emit atof(k) - a segfault). Returns the previous entry for restore.
// The dict/array loop generators already do the same save/restore dance.
func (gen *CodeGenerator) pushLoopVarType(loopVar string) (string, bool) {
	if gen.functionVars == nil {
		return "", false
	}
	old, had := gen.functionVars[loopVar]
	gen.functionVars[loopVar] = "int"
	return old, had
}

func (gen *CodeGenerator) popLoopVarType(loopVar string, old string, had bool) {
	if gen.functionVars == nil {
		return
	}
	if had {
		gen.functionVars[loopVar] = old
	} else {
		delete(gen.functionVars, loopVar)
	}
}

func (gen *CodeGenerator) generateForRangeLoop(node *ahoy.ASTNode) {
	gen.writeIndent()

	var loopVar string

	// Multiple patterns:
	// 1. Constant range: Value has start, DataType has end, Children[0] is body (old syntax)
	// 2. Variable range: Children[0] is start, Children[1] is end, Children[2] is body (old syntax)
	// 3. New syntax: Children[0] is loop var, Children[1] is start, Children[2] is end, Children[3] is body

	if len(node.Children) == 4 && node.Children[0].Type == ahoy.NODE_IDENTIFIER {
		// Pattern 3: New syntax (loop i from 1 to 5 or loop i to 5)
		loopVar = node.Children[0].Value

		gen.output.WriteString("for (int ")
		gen.output.WriteString(loopVar)
		gen.output.WriteString(" = ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString("; ")
		gen.output.WriteString(loopVar)
		gen.output.WriteString(" < ")
		gen.generateNode(node.Children[2])
		gen.output.WriteString("; ")
		gen.output.WriteString(loopVar)
		gen.output.WriteString("++) {\n")

		gen.indent++
		gen.enterScope()
		gen.markDeclaredInCurrentScope(loopVar)
		gen.enterLoopScope()
		oldLoopType, hadLoopType := gen.pushLoopVarType(loopVar)
		gen.generateNodeInternal(node.Children[3], false)
		gen.popLoopVarType(loopVar, oldLoopType, hadLoopType)
		gen.exitLoopScope()
		cleanup := gen.exitScope()
		if cleanup != "" {
			gen.output.WriteString(cleanup)
		}
		gen.indent--
	} else {
		// Old patterns - generate anonymous loop variable
		loopVar = fmt.Sprintf("__loop_i_%d", gen.varCounter)
		gen.varCounter++

		// Push loop counter onto stack
		gen.loopCounters = append(gen.loopCounters, loopVar)

		if len(node.Children) == 1 {
			// Pattern 1: Constant range (loop:0 to 10)
			startVal := node.Value
			endVal := node.DataType

			gen.output.WriteString("for (int ")
			gen.output.WriteString(loopVar)
			gen.output.WriteString(" = ")
			gen.output.WriteString(startVal)
			gen.output.WriteString("; ")
			gen.output.WriteString(loopVar)
			gen.output.WriteString(" <= ")
			gen.output.WriteString(endVal)
			gen.output.WriteString("; ")
			gen.output.WriteString(loopVar)
			gen.output.WriteString("++) {\n")

			gen.indent++
			gen.enterScope()
			gen.markDeclaredInCurrentScope(loopVar)
			gen.enterLoopScope()
			gen.generateNodeInternal(node.Children[0], false)
			gen.exitLoopScope()
			cleanup := gen.exitScope()
			if cleanup != "" {
				gen.output.WriteString(cleanup)
			}
			gen.indent--
		} else {
			// Pattern 2: Variable range (loop:start to end)
			gen.output.WriteString("for (int ")
			gen.output.WriteString(loopVar)
			gen.output.WriteString(" = ")
			gen.generateNode(node.Children[0])
			gen.output.WriteString("; ")
			gen.output.WriteString(loopVar)
			gen.output.WriteString(" <= ")
			gen.generateNode(node.Children[1])
			gen.output.WriteString("; ")
			gen.output.WriteString(loopVar)
			gen.output.WriteString("++) {\n")

			gen.indent++
			gen.enterScope()
			gen.markDeclaredInCurrentScope(loopVar)
			gen.enterLoopScope()
			gen.generateNodeInternal(node.Children[2], false)
			gen.exitLoopScope()
			cleanup := gen.exitScope()
			if cleanup != "" {
				gen.output.WriteString(cleanup)
			}
			gen.indent--
		}

		// Pop loop counter from stack
		gen.loopCounters = gen.loopCounters[:len(gen.loopCounters)-1]
	}

	gen.writeIndent()
	gen.output.WriteString("}\n")
	gen.emitLoopEndLabel()
}

func (gen *CodeGenerator) generateForLoop(node *ahoy.ASTNode) {
	// For now, treat it like ForCountLoop
	gen.generateForCountLoop(node)
}

func (gen *CodeGenerator) generateForCountLoop(node *ahoy.ASTNode) {
	gen.writeIndent()

	// Check patterns:
	// Pattern 1: Children[0] is identifier, Children[1] is start value, Children[2] is body (loop i:start:)
	// Pattern 2: Children[0] is identifier, Children[1] is start (0), Children[2] is body (loop i: or loop i do)
	// Pattern 3: Children[0] is body only (loop: or loop do - infinite loop without variable)

	if len(node.Children) == 3 && node.Children[0].Type == ahoy.NODE_IDENTIFIER {
		// Pattern 1 or 2: loop i:start: (forever loop with explicit variable and start value)
		loopVar := node.Children[0].Value

		// Use block scope to avoid variable redeclaration
		gen.output.WriteString("{\n")
		gen.indent++
		gen.enterScope()
		gen.writeIndent()

		gen.output.WriteString("int ")
		gen.output.WriteString(loopVar)
		gen.output.WriteString(" = ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(";\n")
		gen.writeIndent()
		gen.output.WriteString("for (; ; ")
		gen.output.WriteString(loopVar)
		gen.output.WriteString("++) {\n")

		gen.indent++
		gen.enterScope()
		gen.markDeclaredInCurrentScope(loopVar)
		gen.enterLoopScope()
		oldLoopType, hadLoopType := gen.pushLoopVarType(loopVar)
		gen.generateNodeInternal(node.Children[2], false)
		gen.popLoopVarType(loopVar, oldLoopType, hadLoopType)
		gen.exitLoopScope()
		cleanup := gen.exitScope()
		if cleanup != "" {
			gen.output.WriteString(cleanup)
		}
		gen.indent--

		gen.writeIndent()
		gen.output.WriteString("}\n")

		outerCleanup := gen.exitScope()
		if outerCleanup != "" {
			gen.output.WriteString(outerCleanup)
		}
		gen.indent--
		gen.writeIndent()
		gen.output.WriteString("}\n")
		gen.emitLoopEndLabel()
	} else if len(node.Children) == 2 && node.Children[0].Type == ahoy.NODE_IDENTIFIER {
		// Old pattern: loop i do (forever loop with explicit variable starting at 0)
		loopVar := node.Children[0].Value

		// Use block scope to avoid variable redeclaration
		gen.output.WriteString("{\n")
		gen.indent++
		gen.enterScope()
		gen.writeIndent()

		gen.output.WriteString("int ")
		gen.output.WriteString(loopVar)
		gen.output.WriteString(" = 0;\n")
		gen.writeIndent()
		gen.output.WriteString("for (; ; ")
		gen.output.WriteString(loopVar)
		gen.output.WriteString("++) {\n")

		gen.indent++
		gen.enterScope()
		gen.markDeclaredInCurrentScope(loopVar)
		gen.enterLoopScope()
		oldLoopType, hadLoopType := gen.pushLoopVarType(loopVar)
		gen.generateNodeInternal(node.Children[1], false)
		gen.popLoopVarType(loopVar, oldLoopType, hadLoopType)
		gen.exitLoopScope()
		cleanup := gen.exitScope()
		if cleanup != "" {
			gen.output.WriteString(cleanup)
		}
		gen.indent--

		gen.writeIndent()
		gen.output.WriteString("}\n")

		outerCleanup := gen.exitScope()
		if outerCleanup != "" {
			gen.output.WriteString(outerCleanup)
		}
		gen.indent--
		gen.writeIndent()
		gen.output.WriteString("}\n")
		gen.emitLoopEndLabel()
	} else if len(node.Children) == 1 || node.Value == "0" {
		// Pattern 3: Forever loop without explicit variable (loop: or loop do)
		gen.output.WriteString("for (;;) {\n")

		gen.indent++
		gen.enterScope()
		gen.enterLoopScope()
		gen.generateNodeInternal(node.Children[0], false)
		gen.exitLoopScope()
		cleanup := gen.exitScope()
		if cleanup != "" {
			gen.output.WriteString(cleanup)
		}
		gen.indent--

		gen.writeIndent()
		gen.output.WriteString("}\n")
		gen.emitLoopEndLabel()
	} else {
		// Old syntax: standard for loop with init/condition/update
		gen.output.WriteString("for (")

		// Init - with variable declaration
		gen.generateAssignmentForFor(node.Children[0])
		gen.output.WriteString("; ")

		// Condition
		gen.generateNode(node.Children[1])
		gen.output.WriteString("; ")

		// Update - without variable declaration, just assignment
		gen.generateAssignmentForUpdate(node.Children[2])

		gen.output.WriteString(") {\n")

		gen.indent++
		gen.enterScope()
		gen.enterLoopScope()
		gen.generateNodeInternal(node.Children[3], false)
		gen.exitLoopScope()
		cleanup := gen.exitScope()
		if cleanup != "" {
			gen.output.WriteString(cleanup)
		}
		gen.indent--

		gen.writeIndent()
		gen.output.WriteString("}\n")
		gen.emitLoopEndLabel()
	}
}

func (gen *CodeGenerator) generateAssignmentForFor(node *ahoy.ASTNode) {
	if node.Type == ahoy.NODE_ASSIGNMENT {
		// Type inference
		valueNode := node.Children[0]
		varType := gen.inferType(valueNode)
		gen.variables[node.Value] = varType

		cType := gen.mapType(varType)
		gen.output.WriteString(cType)
		gen.output.WriteString(" ")
		gen.output.WriteString(node.Value)
		gen.output.WriteString(" = ")
		gen.generateNode(valueNode)
	}
}

func (gen *CodeGenerator) generateAssignmentForUpdate(node *ahoy.ASTNode) {
	if node.Type == ahoy.NODE_ASSIGNMENT {
		// Just assignment, no declaration
		gen.output.WriteString(node.Value)
		gen.output.WriteString(" = ")
		gen.generateNode(node.Children[0])
	}
}

func (gen *CodeGenerator) generateForInArrayLoop(node *ahoy.ASTNode) {
	gen.writeIndent()

	// node.Children[0] is element variable name
	// node.Children[1] is array/string expression
	// node.Children[2] is body

	elementVar := node.Children[0].Value
	iterableExpr := node.Children[1]

	// Check if we're iterating over a string
	iterableType := gen.inferType(iterableExpr)

	if iterableType == "char*" || iterableType == "string" {
		// String iteration - iterate over characters
		iterableName := gen.nodeToString(iterableExpr)
		loopVar := fmt.Sprintf("__loop_i_%d", gen.varCounter)
		gen.varCounter++

		gen.output.WriteString("for (int ")
		gen.output.WriteString(loopVar)
		gen.output.WriteString(" = 0; ")
		gen.output.WriteString(iterableName)
		gen.output.WriteString("[")
		gen.output.WriteString(loopVar)
		gen.output.WriteString("] != '\\0'; ")
		gen.output.WriteString(loopVar)
		gen.output.WriteString("++) {\n")

		gen.indent++
		gen.enterScope()
		gen.markDeclaredInCurrentScope(elementVar)
		gen.enterLoopScope()
		gen.writeIndent()

		gen.output.WriteString("char ")
		gen.output.WriteString(elementVar)
		gen.output.WriteString(" = ")
		gen.output.WriteString(iterableName)
		gen.output.WriteString("[")
		gen.output.WriteString(loopVar)
		gen.output.WriteString("];\n")

		// Register loop variable for type inference
		oldType := gen.variables[elementVar]
		gen.variables[elementVar] = "char"

		gen.generateNodeInternal(node.Children[2], false)

		// Restore old type
		if oldType != "" {
			gen.variables[elementVar] = oldType
		} else {
			delete(gen.variables, elementVar)
		}

		gen.exitLoopScope()
		cleanup := gen.exitScope()
		if cleanup != "" {
			gen.output.WriteString(cleanup)
		}
		gen.indent--

		gen.writeIndent()
		gen.output.WriteString("}\n")
		gen.emitLoopEndLabel()
	} else {
		// Array iteration
		loopVar := fmt.Sprintf("__loop_i_%d", gen.varCounter)
		gen.varCounter++

		arrayName := gen.nodeToString(iterableExpr)

		// AhoyArray uses 'length', not 'size'
		gen.output.WriteString("for (int ")
		gen.output.WriteString(loopVar)
		gen.output.WriteString(" = 0; ")
		gen.output.WriteString(loopVar)
		gen.output.WriteString(" < ")
		gen.output.WriteString(arrayName)
		gen.output.WriteString("->length; ")
		gen.output.WriteString(loopVar)
		gen.output.WriteString("++) {\n")

		gen.indent++
		gen.enterScope()
		gen.markDeclaredInCurrentScope(elementVar)
		gen.enterLoopScope()
		gen.writeIndent()

		// Determine element type
		elementType := "int"
		cElementType := "int"
		isStructValue := false

		// Check if we know the element type for this array
		if iterableExpr.Type == ahoy.NODE_IDENTIFIER {
			arrayVarName := iterableExpr.Value
			if elemType, exists := gen.arrayElementTypes[arrayVarName]; exists {
				elementType = elemType
				cElementType = gen.mapType(elemType)

				// For struct types stored by value (not pointer), we need special handling
				// Check if it's a known struct OR if it looks like a struct type (external C structs)
				_, isDefinedStruct := gen.structs[elementType]
				isExternalStruct := !strings.HasSuffix(cElementType, "*") &&
					cElementType != "int" &&
					cElementType != "double" &&
					cElementType != "char*" &&
					cElementType != "void" &&
					cElementType != "bool" &&
					len(cElementType) > 0 &&
					unicode.IsUpper(rune(cElementType[0]))

				if isDefinedStruct || isExternalStruct {
					isStructValue = true
					// Don't add * here - we'll handle it in the cast
				}
			} else {
				// Try to infer from variable type (e.g., array[FloatingText])
				varType := gen.inferType(iterableExpr)
				if strings.HasPrefix(varType, "array[") && strings.HasSuffix(varType, "]") {
					elemType := varType[6 : len(varType)-1]
					elementType = elemType
					cElementType = gen.mapType(elemType)

					// Check if it's a struct type
					_, isDefinedStruct := gen.structs[elementType]
					isExternalStruct := !strings.HasSuffix(cElementType, "*") &&
						cElementType != "int" &&
						cElementType != "double" &&
						cElementType != "char*" &&
						cElementType != "void" &&
						cElementType != "bool" &&
						len(cElementType) > 0 &&
						unicode.IsUpper(rune(cElementType[0]))

					if isDefinedStruct || isExternalStruct {
						isStructValue = true
					}
				}
			}
		}

		// Generate appropriate cast based on element type
		if isStructValue {
			// For struct values: cast to pointer, then dereference
			// struct values are stored as pointers internally
			gen.output.WriteString(cElementType)
			gen.output.WriteString(" ")
			gen.output.WriteString(elementVar)
			gen.output.WriteString(" = *(")
			gen.output.WriteString(cElementType)
			gen.output.WriteString("*)(intptr_t)")
			gen.output.WriteString(arrayName)
			gen.output.WriteString("->data[")
			gen.output.WriteString(loopVar)
			gen.output.WriteString("];\n")
		} else if gen.isHeapAllocatedType(elementType) || strings.Contains(cElementType, "*") {
			// For pointers (strings, etc.), cast through intptr_t to pointer
			gen.output.WriteString(cElementType)
			gen.output.WriteString(" ")
			gen.output.WriteString(elementVar)
			gen.output.WriteString(" = (")
			gen.output.WriteString(cElementType)
			gen.output.WriteString(")(intptr_t)")
			gen.output.WriteString(arrayName)
			gen.output.WriteString("->data[")
			gen.output.WriteString(loopVar)
			gen.output.WriteString("];\n")
		} else {
			// For primitives, cast through intptr_t to the appropriate type
			gen.output.WriteString(cElementType)
			gen.output.WriteString(" ")
			gen.output.WriteString(elementVar)
			gen.output.WriteString(" = (")
			gen.output.WriteString(cElementType)
			gen.output.WriteString(")(intptr_t)")
			gen.output.WriteString(arrayName)
			gen.output.WriteString("->data[")
			gen.output.WriteString(loopVar)
			gen.output.WriteString("];\n")
		}

		// Register loop variable for type inference
		// For struct types, store the pointer type
		oldType := gen.variables[elementVar]
		if _, isStruct := gen.structs[elementType]; isStruct {
			gen.variables[elementVar] = cElementType
		} else {
			gen.variables[elementVar] = elementType
		}

		gen.generateNodeInternal(node.Children[2], false)

		// Restore old type
		if oldType != "" {
			gen.variables[elementVar] = oldType
		} else {
			delete(gen.variables, elementVar)
		}

		gen.exitLoopScope()
		cleanup := gen.exitScope()
		if cleanup != "" {
			gen.output.WriteString(cleanup)
		}
		gen.indent--

		gen.writeIndent()
		gen.output.WriteString("}\n")
		gen.emitLoopEndLabel()
	}
}

func (gen *CodeGenerator) generateForInDictLoop(node *ahoy.ASTNode) {
	gen.writeIndent()

	// node.Children[0] is key variable name
	// node.Children[1] is value variable name
	// node.Children[2] is dict expression
	// node.Children[3] is body

	keyVar := node.Children[0].Value
	valueVar := node.Children[1].Value
	dictExpr := node.Children[2]

	// Generate unique loop counters
	bucketVar := fmt.Sprintf("__bucket_%d", gen.varCounter)
	entryVar := fmt.Sprintf("__entry_%d", gen.varCounter)
	gen.varCounter++

	dictName := gen.nodeToString(dictExpr)

	// Check if we need to cast (for generic/any parameters)
	dictType := gen.inferType(dictExpr)
	dictRef := dictName
	if dictType == "generic" || dictType == "any" {
		dictRef = "((HashMap*)" + dictName + ")"
	}

	// Iterate through hash map buckets
	gen.output.WriteString("for (int ")
	gen.output.WriteString(bucketVar)
	gen.output.WriteString(" = 0; ")
	gen.output.WriteString(bucketVar)
	gen.output.WriteString(" < ")
	gen.output.WriteString(dictRef)
	gen.output.WriteString("->capacity; ")
	gen.output.WriteString(bucketVar)
	gen.output.WriteString("++) {\n")

	gen.indent++
	gen.enterScope()
	gen.markDeclaredInCurrentScope(keyVar)
	gen.enterLoopScope()
	gen.writeIndent()
	gen.output.WriteString("HashMapEntry* ")
	gen.output.WriteString(entryVar)
	gen.output.WriteString(" = ")
	gen.output.WriteString(dictRef)
	gen.output.WriteString("->buckets[")
	gen.output.WriteString(bucketVar)
	gen.output.WriteString("];\n")

	gen.writeIndent()
	gen.output.WriteString("while (")
	gen.output.WriteString(entryVar)
	gen.output.WriteString(" != NULL) {\n")

	gen.indent++
	gen.enterScope()
	gen.markDeclaredInCurrentScope(valueVar)
	gen.enterInnerLoopScope() // Inner traversal loop: halt/next target the enclosing user loop
	gen.writeIndent()
	gen.output.WriteString("const char* ")
	gen.output.WriteString(keyVar)
	gen.output.WriteString(" = ")
	gen.output.WriteString(entryVar)
	gen.output.WriteString("->key;\n")

	// Try to infer the dict value type from the dict variable
	valueType := "" // Will determine based on dict type
	valueCType := ""
	hasKnownType := false

	// Check if dict variable has a known type like dict<string,string>
	if dictExpr.Type == ahoy.NODE_IDENTIFIER {
		dictVarName := dictExpr.Value
		var dictVarType string
		if varType, exists := gen.variables[dictVarName]; exists {
			dictVarType = varType
		} else if varType, exists := gen.functionVars[dictVarName]; exists {
			dictVarType = varType
		}

		// Check if it's a typed dict
		if strings.HasPrefix(dictVarType, "dict<") || strings.HasPrefix(dictVarType, "dict[") {
			// Extract value type
			startIdx := strings.IndexAny(dictVarType, "<[")
			endIdx := strings.LastIndexAny(dictVarType, ">]")
			if startIdx >= 0 && endIdx > startIdx {
				types := dictVarType[startIdx+1 : endIdx]
				parts := strings.Split(types, ",")
				if len(parts) == 2 {
					valueType = strings.TrimSpace(parts[1])
					valueCType = gen.mapType(valueType)
					hasKnownType = true
				}
			}
		}
	}

	// Save old types before registering loop variables
	oldKeyType, _ := gen.variables[keyVar]
	oldValType, _ := gen.variables[valueVar]
	// For typed dicts, use the specific type
	// For untyped dicts (object literals), use intptr_t (can be cast to arrays/dicts/etc)
	if hasKnownType {
		gen.writeIndent()
		gen.output.WriteString(valueCType)
		gen.output.WriteString(" ")
		gen.output.WriteString(valueVar)
		gen.output.WriteString(" = (")
		gen.output.WriteString(valueCType)
		gen.output.WriteString(")")
		gen.output.WriteString(entryVar)
		gen.output.WriteString("->value;\n")

		// Register loop variables
		gen.variables[keyVar] = "char*"
		gen.variables[valueVar] = valueType
	} else {
		// For untyped dicts, expose value as intptr_t which can be cast as needed
		gen.writeIndent()
		gen.output.WriteString("intptr_t ")
		gen.output.WriteString(valueVar)
		gen.output.WriteString(" = (intptr_t)")
		gen.output.WriteString(entryVar)
		gen.output.WriteString("->value;\n")

		// Register loop variables
		gen.variables[keyVar] = "char*"
		gen.variables[valueVar] = "intptr_t"

		// An untyped dict value has no static type: it may hold an integer or a
		// pointer. Formatting it as a string would dereference an integer and
		// crash, so remember an expression that consults the entry's runtime type
		// instead. The key is a variable here, not a literal.
		gen.dictLoopValueExpr[valueVar] = fmt.Sprintf("format_dict_value(%s, %s)", dictName, keyVar)
	}

	gen.generateNodeInternal(node.Children[3], false)

	// Restore old types (cleanup)
	if oldKeyType != "" {
		gen.variables[keyVar] = oldKeyType
	} else {
		delete(gen.variables, keyVar)
	}
	if oldValType != "" {
		gen.variables[valueVar] = oldValType
	} else {
		delete(gen.variables, valueVar)
	}
	delete(gen.dictLoopValueExpr, valueVar)

	gen.writeIndent()
	gen.output.WriteString(entryVar)
	gen.output.WriteString(" = ")
	gen.output.WriteString(entryVar)
	gen.output.WriteString("->next;\n")

	gen.exitInnerLoopScope() // Exit inner traversal loop
	innerCleanup := gen.exitScope()
	if innerCleanup != "" {
		gen.output.WriteString(innerCleanup)
	}
	gen.indent--

	gen.writeIndent()
	gen.output.WriteString("}\n")

	gen.exitLoopScope() // Exit outer loop scope
	outerCleanup := gen.exitScope()
	if outerCleanup != "" {
		gen.output.WriteString(outerCleanup)
	}
	gen.indent--

	gen.writeIndent()
	gen.output.WriteString("}\n")
	gen.emitLoopEndLabel()
}

func (gen *CodeGenerator) generateReturnStatement(node *ahoy.ASTNode) {
	// Mark storage that escapes through the return value: bare names, values
	// stored into returned literals, and ternary branches. Reads of primitives
	// (lengths, indices, arithmetic) do not transfer ownership, so the scope
	// cleanups below can free those locals before returning.
	if len(node.Children) > 0 {
		for _, child := range node.Children {
			gen.markReturnedValue(child)
		}
		// Transitive stored dependencies (aliases of aliases, select
		// branches): an escaping value protects its ultimate owners.
		for _, child := range node.Children {
			gen.markValueDeps(child)
		}
	}

	// Names read by the return value itself. The cleanups below run before
	// the `return` statement evaluates it, so without care they would free
	// storage the value still reads.
	returnMentioned := map[string]bool{}
	for _, child := range node.Children {
		collectDeferredIdents(child, returnMentioned)
	}

	// Single-value returns with a known C type stash the value in a temp
	// first when a cleanup would otherwise free a variable it reads.
	// (Multi-value and switch returns keep the direct shape; their mentioned
	// names stay protected by the leak-safe skip in the cleanup loop.)
	needTemp := len(node.Children) == 1 && !gen.currentFunctionHasMultiReturn &&
		node.Children[0].Type != ahoy.NODE_SWITCH_STATEMENT &&
		gen.currentFunctionReturnType != "" && gen.currentFunctionReturnType != "void" &&
		gen.returnWouldFreeMentioned(returnMentioned)
	tmpVar := ""
	if needTemp {
		tmpVar = fmt.Sprintf("__ret_%d", gen.varCounter)
		gen.varCounter++
		gen.writeIndent()
		gen.output.WriteString("{\n")
		gen.indent++
		gen.writeIndent()
		gen.output.WriteString(gen.currentFunctionReturnType)
		gen.output.WriteString(" ")
		gen.output.WriteString(tmpVar)
		gen.output.WriteString(" = ")
		gen.generateNode(node.Children[0])
		gen.output.WriteString(";\n")
	}

	// Free heap locals live at this return. Auto-free entries are appended
	// only at function end, so without this an early return leaks everything
	// declared above it. Cleanups run deepest-scope first, skipping escaped,
	// manually freed and already-freed variables (mirroring exitScope). No
	// marks are set: this code runs only on the exiting path, while later
	// block and function ends still clean up for paths that fall through.
	for depth := gen.scopeDepth; depth >= 0; depth-- {
		varsAtScope, exists := gen.scopeAllocations[depth]
		if !exists {
			continue
		}
		sortedVars := make([]string, len(varsAtScope))
		copy(sortedVars, varsAtScope)
		sort.Strings(sortedVars)
		for i := len(sortedVars) - 1; i >= 0; i-- {
			varName := sortedVars[i]
			if gen.escapingVars[varName] || gen.manuallyFreedVars[varName] || gen.autoFreedVars[varName] {
				continue
			}
			// Names mentioned in still-pending defers must outlive them;
			// the flush below runs those defers before returning.
			if gen.deferredUsers[varName] {
				continue
			}
			// Without a stash temp, names the return value reads must also
			// survive until the `return` statement evaluates it below.
			// (With a temp the value is already computed, so free them.)
			if !needTemp && returnMentioned[varName] {
				continue
			}
			varType := gen.heapVarTypes[varName]
			if varType == "" {
				continue
			}
			if freeCode := gen.generateFreeCodeForVar(varName, varType); freeCode != "" {
				gen.output.WriteString(freeCode)
			}
		}
	}

	// Execute deferred statements in LIFO order before return
	if len(gen.deferredStatements) > 0 {
		for i := len(gen.deferredStatements) - 1; i >= 0; i-- {
			gen.output.WriteString(gen.deferredStatements[i].code)
		}
		// Clear deferred statements so they don't execute again at function end
		gen.deferredStatements = nil
		// All pending deferrals ran: their names need no more protection.
		gen.deferredUsers = make(map[string]bool)
	}

	if needTemp {
		gen.writeIndent()
		gen.output.WriteString("return ")
		gen.output.WriteString(tmpVar)
		gen.output.WriteString(";\n")
		gen.indent--
		gen.writeIndent()
		gen.output.WriteString("}\n")
		return
	}

	gen.writeIndent()
	// `return switch x: ...` — a switch assigns per case rather than producing
	// a value, so it is evaluated into a temporary which is then returned.
	// This is handled before the `return` prefix is written, and mirrors the
	// supported `out: T = switch...; return out` shape. Ends with a newline
	// instead of the `;` a plain `return expr;` takes.
	if len(node.Children) == 1 && node.Children[0].Type == ahoy.NODE_SWITCH_STATEMENT {
		switchNode := node.Children[0]
		tmpVar := fmt.Sprintf("__switch_ret_%d", gen.varCounter)
		gen.varCounter++
		gen.output.WriteString("{\n")
		gen.indent++
		gen.writeIndent()
		gen.output.WriteString(gen.switchResultType(switchNode))
		gen.output.WriteString(" ")
		gen.output.WriteString(tmpVar)
		gen.output.WriteString(";\n")
		gen.generateSwitchExpression(switchNode, tmpVar)
		gen.writeIndent()
		gen.output.WriteString("return ")
		gen.output.WriteString(tmpVar)
		gen.output.WriteString(";\n")
		gen.indent--
		gen.writeIndent()
		gen.output.WriteString("}\n")
		return
	}
	gen.output.WriteString("return")
	if len(node.Children) > 0 {
		gen.output.WriteString(" ")
		// Handle multiple return values
		if len(node.Children) > 1 && gen.currentFunctionHasMultiReturn {
			// Multiple returns - return a struct literal with correct type
			gen.output.WriteString("(")
			gen.output.WriteString(gen.currentFunctionReturnType)
			gen.output.WriteString("){")

			// Get the return types for casting
			returnTypes, hasReturnTypes := gen.functionReturnTypes[gen.currentFunction]

			for i, child := range node.Children {
				if i > 0 {
					gen.output.WriteString(", ")
				}
				gen.output.WriteString(".ret")
				gen.output.WriteString(strconv.Itoa(i))
				gen.output.WriteString(" = ")

				// If this return type is generic/any (intptr_t) and value is string, cast
				if hasReturnTypes && i < len(returnTypes) && (returnTypes[i] == "generic" || returnTypes[i] == "any") {
					childType := gen.inferType(child)
					if childType == "string" || childType == "char*" || childType == "const char*" {
						gen.output.WriteString("(intptr_t)")
					}
				}

				gen.generateNode(child)
			}
			gen.output.WriteString("}")
		} else {
			gen.generateNode(node.Children[0])
		}
	}
	gen.output.WriteString(";\n")
}

func (gen *CodeGenerator) generateAssertStatement(node *ahoy.ASTNode) {
	// Generate assert as C assert macro
	gen.includes["assert.h"] = true
	gen.writeIndent()
	gen.output.WriteString("assert(")
	if len(node.Children) > 0 {
		gen.generateNode(node.Children[0])
	}
	gen.output.WriteString(");\n")
}

func (gen *CodeGenerator) generateDeferStatement(node *ahoy.ASTNode) {
	// Collect deferred statements to execute in LIFO order at function end
	if len(node.Children) > 0 {
		// Check if this is a manual defer free
		child := node.Children[0]
		if child.Type == ahoy.NODE_CALL && child.Value == "free" && len(child.Children) > 0 {
			// Track manual defer free
			if child.Children[0].Type == ahoy.NODE_IDENTIFIER {
				varName := child.Children[0].Value
				gen.manuallyFreedVars[varName] = true
			}
		}

		// Generate the deferred statement into a temporary buffer
		savedOutput := gen.output
		gen.output = strings.Builder{}
		savedIndent := gen.indent
		gen.indent = 0

		gen.generateNodeInternal(node.Children[0], true) // Generate as statement

		deferredCode := gen.output.String()
		gen.output = savedOutput
		gen.indent = savedIndent

		// Add to deferred statements stack
		// A defer that mentions a loop counter or a nested-block local cannot run
		// at function exit: those names are gone by then. Such a statement runs at
		// the end of the block it was written in, which is where its names still
		// exist. Deferred statements that only mention function-level names keep
		// the documented function-exit behaviour.
		if len(node.Children) > 0 {
			collectDeferredIdents(node.Children[0], gen.deferredUsers)
		}
		gen.deferredStatements = append(gen.deferredStatements, deferredStmt{
			depth:       gen.scopeDepth,
			code:        deferredCode,
			blockScoped: gen.referencesOutOfScopeName(node.Children[0]),
		})
	}
}

func (gen *CodeGenerator) markEscapingVariables(node *ahoy.ASTNode) {
	if node == nil {
		return
	}

	if node.Type == ahoy.NODE_IDENTIFIER {
		varName := node.Value
		// Check if this is a function-local variable that's heap-allocated
		if gen.heapAllocatedVars[varName] {
			gen.escapingVars[varName] = true
		}
		// A closure name escaping carries its captures' scope with it.
		gen.markLambdaCapturesOf(varName)
	}

	// Recursively check children
	for _, child := range node.Children {
		gen.markEscapingVariables(child)
	}
}

// registerHeapAllocation tracks a heap-allocated variable for automatic freeing
func (gen *CodeGenerator) registerHeapAllocation(varName string, varType string) {
	// Skip if it's a function parameter
	if gen.functionParameters[varName] {
		return
	}

	gen.heapAllocatedVars[varName] = true
	gen.heapVarScopes[varName] = gen.scopeDepth
	gen.heapVarTypes[varName] = varType
	gen.scopeAllocations[gen.scopeDepth] = append(gen.scopeAllocations[gen.scopeDepth], varName)
}

// enterScope increments scope depth for tracking nested allocations
func (gen *CodeGenerator) enterScope() {
	gen.scopeDepth++
	gen.scopeDeclared = append(gen.scopeDeclared, map[string]bool{})
}

// markDeclaredInCurrentScope records that a name was declared in the innermost
// open block, so a later block can tell whether it is still visible.
func (gen *CodeGenerator) markDeclaredInCurrentScope(name string) {
	if name == "" || name == "_" || len(gen.scopeDeclared) == 0 {
		return
	}
	gen.scopeDeclared[len(gen.scopeDeclared)-1][name] = true
}

// nameVisibleInOpenScope reports whether a name was declared in the innermost
// block or in any block still enclosing it.
func (gen *CodeGenerator) nameVisibleInOpenScope(name string) bool {
	for i := len(gen.scopeDeclared) - 1; i >= 0; i-- {
		if gen.scopeDeclared[i][name] {
			return true
		}
	}
	return false
}

// enterLoopScope marks the current scope as a loop boundary for break/continue handling.
// It also opens a label slot for halt-across-switch (see loopEndLabelAcrossSwitch).
func (gen *CodeGenerator) enterLoopScope() {
	gen.loopScopeStack = append(gen.loopScopeStack, gen.scopeDepth)
	gen.loopEndLabels = append(gen.loopEndLabels, loopEndLabel{scopeDepth: gen.scopeDepth})
}

// exitLoopScope removes the loop boundary marker, stashing any pending end
// label for the loop tail (emitLoopEndLabel writes it after the loop's
// closing brace).
func (gen *CodeGenerator) exitLoopScope() {
	if len(gen.loopEndLabels) > 0 {
		gen.pendingLoopEndLabel = gen.loopEndLabels[len(gen.loopEndLabels)-1].label
		gen.loopEndLabels = gen.loopEndLabels[:len(gen.loopEndLabels)-1]
	} else {
		gen.pendingLoopEndLabel = ""
	}
	if len(gen.loopScopeStack) > 0 {
		gen.loopScopeStack = gen.loopScopeStack[:len(gen.loopScopeStack)-1]
	}
}

// enterInnerLoopScope marks a generated (non-user) nested traversal loop —
// currently the dict entry-while inside the bucket-for. Cleanup still treats
// it as a boundary, but halt/next target the enclosing user loop, so no
// separate end label is tracked for it.
func (gen *CodeGenerator) enterInnerLoopScope() {
	gen.loopScopeStack = append(gen.loopScopeStack, gen.scopeDepth)
}

// exitInnerLoopScope removes a generated inner-traversal boundary marker.
func (gen *CodeGenerator) exitInnerLoopScope() {
	if len(gen.loopScopeStack) > 0 {
		gen.loopScopeStack = gen.loopScopeStack[:len(gen.loopScopeStack)-1]
	}
}

// flushBlockScopedDefers emits code running pending block-scoped defers for
// scopes an early exit (halt/next) leaves, from fromDepth upward. Entries
// stay listed: exactly one runtime path reaches any flush (the early exit
// skips the block end, all other paths skip this), while block-end, return
// and function-end handling is unchanged. Function-level defers are never
// flushed here — the function has not ended. Runs before scope cleanup
// (use-then-free), mirroring return's deferredUsers protection in reverse.
func (gen *CodeGenerator) flushBlockScopedDefers(fromDepth int) {
	for i := len(gen.deferredStatements) - 1; i >= 0; i-- {
		d := gen.deferredStatements[i]
		if d.blockScoped && d.depth >= fromDepth {
			gen.output.WriteString(d.code)
		}
	}
}

// emitLoopEndLabel writes a pending halt-across-switch end label after a
// loop's closing brace ("" = no halt needed one: emits nothing). The
// labeled null statement keeps the label valid C even at function end.
func (gen *CodeGenerator) emitLoopEndLabel() {
	if gen.pendingLoopEndLabel == "" {
		return
	}
	gen.writeIndent()
	gen.output.WriteString(gen.pendingLoopEndLabel)
	gen.output.WriteString(": ;\n")
	gen.pendingLoopEndLabel = ""
}

// loopEndLabelAcrossSwitch returns the end label of the innermost user loop
// when a switch-case scope sits between the halt point and that loop. A
// plain C `break` there would only exit the switch, so the caller emits a
// compiler-generated `goto` instead (cleanup already ran above it — unlike
// user-level goto, which was removed for bypassing cleanup).
func (gen *CodeGenerator) loopEndLabelAcrossSwitch() (string, bool) {
	if len(gen.loopEndLabels) == 0 {
		return "", false
	}
	top := &gen.loopEndLabels[len(gen.loopEndLabels)-1]
	for _, d := range gen.switchCaseDepths {
		if d > top.scopeDepth {
			if top.label == "" {
				top.label = fmt.Sprintf("__ahoy_loop_end_%d", gen.varCounter)
				gen.varCounter++
			}
			return top.label, true
		}
	}
	return "", false
}

// enterSwitchCaseScope opens the scope of one switch-case body: heap locals
// are freed when the case ends (see exitScope). The depth is recorded so a
// `halt` inside knows a switch boundary intervenes (plain `break` would only
// exit the switch, not the loop).
func (gen *CodeGenerator) enterSwitchCaseScope() {
	gen.enterScope()
	gen.switchCaseDepths = append(gen.switchCaseDepths, gen.scopeDepth)
}

// exitSwitchCaseScope closes a switch-case scope, returning its cleanup.
func (gen *CodeGenerator) exitSwitchCaseScope() string {
	if len(gen.switchCaseDepths) > 0 {
		gen.switchCaseDepths = gen.switchCaseDepths[:len(gen.switchCaseDepths)-1]
	}
	return gen.exitScope()
}

// generateEarlyExitCleanup generates cleanup code for variables from current scope down to loop scope
// Used for break/continue/return statements
func (gen *CodeGenerator) generateEarlyExitCleanup(includeLoopScope bool) string {
	// Determine the target depth (where to stop cleaning up)
	targetDepth := 0
	if len(gen.loopScopeStack) > 0 {
		// Both branches clean through the loop scope; the flag is vestigial.
		targetDepth = gen.loopScopeStack[len(gen.loopScopeStack)-1]
	}
	return gen.generateEarlyExitCleanupTo(targetDepth)
}

// generateEarlyExitCleanupTo frees heap locals from the current scope down
// to (and including) targetDepth. Used when an exit leaves an exact scope —
// a `halt` breaking a switch frees only the case (deeper ifs included),
// never the function locals the loop-breaking path below must free.
func (gen *CodeGenerator) generateEarlyExitCleanupTo(targetDepth int) string {
	var cleanup strings.Builder

	// Clean up from current depth down to target depth (inclusive)
	for depth := gen.scopeDepth; depth >= targetDepth; depth-- {
		if varsAtScope, exists := gen.scopeAllocations[depth]; exists {
			// Sort for deterministic output
			sortedVars := make([]string, len(varsAtScope))
			copy(sortedVars, varsAtScope)
			sort.Strings(sortedVars)

			// Generate cleanup in reverse order (LIFO)
			for i := len(sortedVars) - 1; i >= 0; i-- {
				varName := sortedVars[i]

				// Skip if escaping or manually freed
				// NOTE: Don't skip if already auto-freed, since this is conditional cleanup
				if gen.escapingVars[varName] || gen.manuallyFreedVars[varName] {
					continue
				}

				varType := gen.heapVarTypes[varName]
				if varType == "" {
					continue
				}

				freeCode := gen.generateFreeCodeForVar(varName, varType)
				if freeCode != "" {
					cleanup.WriteString(freeCode)
					// DON'T mark as autoFreedVars - this cleanup is conditional (inside if/break/etc)
					// The normal exitScope() will still generate cleanup for non-early-exit paths
				}
			}
		}
	}

	return cleanup.String()
}

// exitScope generates cleanup code for variables allocated in the current scope
// and decrements scope depth. Returns the cleanup code to be inserted before '}'
// deferredStmt is one deferred statement waiting to be emitted.
type deferredStmt struct {
	// depth is the scope depth the statement was written at.
	depth int
	// code is the generated C for the statement.
	code string
	// blockScoped means the statement mentions a name that only exists inside
	// the block it was written in, so it must run when that block ends.
	blockScoped bool
}

// nameVisibleAtFunctionExit reports whether a name declared now will still
// exist when the function returns.
func (gen *CodeGenerator) nameVisibleAtFunctionExit(name string) bool {
	if len(gen.scopeDeclared) > 0 && gen.scopeDeclared[0][name] {
		return true // a parameter or a function-level local
	}
	return gen.declaredGlobalVars[name]
}

// referencesOutOfScopeName reports whether node mentions a name that is declared
// in a block (a loop counter, a nested-block local) and so will not exist at
// function exit. Names that are not declared locally at all - globals, types,
// C symbols - are left alone.
func (gen *CodeGenerator) referencesOutOfScopeName(node *ahoy.ASTNode) bool {
	if node == nil {
		return false
	}
	if node.Type == ahoy.NODE_IDENTIFIER {
		return gen.nameIsOutOfScopeAtExit(node.Value)
	}
	// An f-string keeps its interpolated expressions as text rather than as
	// child nodes, so they have to be read out of the value.
	if node.Type == ahoy.NODE_F_STRING {
		for _, name := range fstringLeadingIdentifiers(node.Value) {
			if gen.nameIsOutOfScopeAtExit(name) {
				return true
			}
		}
	}
	for _, child := range node.Children {
		if gen.referencesOutOfScopeName(child) {
			return true
		}
	}
	return false
}

// nameIsOutOfScopeAtExit reports whether a name is declared in a block, and so
// will not exist when the function returns.
func (gen *CodeGenerator) nameIsOutOfScopeAtExit(name string) bool {
	if name == "" || name == "_" || name == "__loop_counter" {
		return false
	}
	if gen.nameVisibleAtFunctionExit(name) {
		return false
	}
	// Only a name that is actually in an enclosing block counts; anything else is
	// a global, a type or a C symbol.
	return gen.nameVisibleInOpenScope(name)
}

// fstringLeadingIdentifiers returns the first identifier of each {..} group in
// an f-string.
func fstringLeadingIdentifiers(value string) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '{':
			if depth == 0 {
				start = i + 1
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
				if depth == 0 && start <= i {
					if name := leadingIdentifierText(value[start:i]); name != "" {
						out = append(out, name)
					}
				}
			}
		}
	}
	return out
}

// leadingIdentifierText returns the first identifier in an interpolated
// expression such as "i", "i + 1" or "item.name".
func leadingIdentifierText(expr string) string {
	expr = strings.TrimSpace(expr)
	end := 0
	for end < len(expr) {
		ch := expr[end]
		if ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(end > 0 && ch >= '0' && ch <= '9') {
			end++
			continue
		}
		break
	}
	return expr[:end]
}

func (gen *CodeGenerator) exitScope() string {
	var cleanup strings.Builder

	// Run any deferred statements written in this block now, while the names
	// they mention are still in scope. LIFO within the block.
	if len(gen.deferredStatements) > 0 {
		kept := gen.deferredStatements[:0]
		var flushed []deferredStmt
		for _, d := range gen.deferredStatements {
			if d.blockScoped && d.depth >= gen.scopeDepth {
				flushed = append(flushed, d)
				continue
			}
			kept = append(kept, d)
		}
		gen.deferredStatements = kept
		for i := len(flushed) - 1; i >= 0; i-- {
			cleanup.WriteString(flushed[i].code)
		}
	}

	// Get variables allocated at current scope
	if varsAtScope, exists := gen.scopeAllocations[gen.scopeDepth]; exists {
		// Sort for deterministic output
		sortedVars := make([]string, len(varsAtScope))
		copy(sortedVars, varsAtScope)
		sort.Strings(sortedVars)

		// Generate cleanup in reverse order (LIFO)
		for i := len(sortedVars) - 1; i >= 0; i-- {
			varName := sortedVars[i]

			// Skip if escaping or manually freed
			if gen.escapingVars[varName] || gen.manuallyFreedVars[varName] || gen.autoFreedVars[varName] {
				continue
			}

			varType := gen.heapVarTypes[varName]
			if varType == "" {
				continue
			}

			freeCode := gen.generateFreeCodeForVar(varName, varType)
			if freeCode != "" {
				cleanup.WriteString(freeCode)
				gen.autoFreedVars[varName] = true
			}
		}

		// Clear the scope allocations
		delete(gen.scopeAllocations, gen.scopeDepth)
	}

	// Drop alias edges recorded in the scope being exited: those aliases
	// are dead, so remaining edges always point at live aliases.
	for target, edges := range gen.aliasEdges {
		for alias, depth := range edges {
			if depth >= gen.scopeDepth {
				delete(edges, alias)
			}
		}
		if len(edges) == 0 {
			delete(gen.aliasEdges, target)
		}
	}

	gen.scopeDepth--
	if len(gen.scopeDeclared) > 0 {
		gen.scopeDeclared = gen.scopeDeclared[:len(gen.scopeDeclared)-1]
	}
	return cleanup.String()
}

// generateFreeCodeForVar generates the appropriate free() code for a variable based on its type
func (gen *CodeGenerator) generateFreeCodeForVar(varName string, varType string) string {
	// A function value owns the environment of a capturing closure; the value
	// itself is a {code, env} pair, so the env is freed, not the variable.
	if ahoy.IsFunctionType(varType) {
		return fmt.Sprintf("    if (%s.env) free(%s.env);\n", varName, varName)
	}
	if varType == "pool" {
		return fmt.Sprintf("    if (%s) ahoy_jobs_destroy(%s);\n", varName, varName)
	}

	if strings.HasPrefix(varType, "array") || varType == "AhoyArray*" {
		// For arrays, need to free the array structure and possibly contents
		// Check if array contains heap-allocated structs (Ahoy or C structs)
		hasStructElements := false
		hasNestedArrays := false
		nestedElemType := ""

		if strings.HasPrefix(varType, "array[") {
			elemType := strings.TrimSuffix(strings.TrimPrefix(varType, "array["), "]")

			// Check if this is a 2D array (array of arrays)
			if strings.HasPrefix(elemType, "array[") {
				hasNestedArrays = true
				// Extract the inner element type
				nestedElemType = strings.TrimSuffix(strings.TrimPrefix(elemType, "array["), "]")
			} else {
				// Check if it's an Ahoy-defined struct
				if gen.structs[elemType] != nil {
					hasStructElements = true
				} else {
					// Check if it's a C struct type
					cType := gen.mapType(elemType)
					if gen.cTypeDefinitions[cType] && !strings.HasSuffix(cType, "*") &&
						cType != "int" && cType != "double" && cType != "char*" &&
						cType != "bool" && cType != "void" && cType != "char" && cType != "float" {
						hasStructElements = true
					}
				}
			}
		}

		if hasNestedArrays {
			// For 2D arrays, free inner arrays first
			// Check if inner arrays contain structs (Ahoy or C structs)
			hasInnerStructs := gen.structs[nestedElemType] != nil
			if !hasInnerStructs {
				// Check if it's a C struct
				cType := gen.mapType(nestedElemType)
				hasInnerStructs = gen.cTypeDefinitions[cType] && !strings.HasSuffix(cType, "*") &&
					cType != "int" && cType != "double" && cType != "char*" &&
					cType != "bool" && cType != "void" && cType != "char" && cType != "float"
			}

			if hasInnerStructs {
				// Free struct elements in inner arrays, then inner arrays, then outer array
				return fmt.Sprintf("    if (%s) { for(int __i=0; __i<%s->length; __i++) { AhoyArray* __inner = (AhoyArray*)%s->data[__i]; if(__inner) { for(int __j=0; __j<__inner->length; __j++) { if(__inner->data[__j]) free((void*)__inner->data[__j]); } free(__inner->data); free(__inner->types); free(__inner); } } free(%s->data); free(%s->types); free(%s); }\n",
					varName, varName, varName, varName, varName, varName)
			} else {
				// Free inner arrays, then outer array
				return fmt.Sprintf("    if (%s) { for(int __i=0; __i<%s->length; __i++) { AhoyArray* __inner = (AhoyArray*)%s->data[__i]; if(__inner) { free(__inner->data); free(__inner->types); free(__inner); } } free(%s->data); free(%s->types); free(%s); }\n",
					varName, varName, varName, varName, varName, varName)
			}
		} else if hasStructElements {
			// Free struct elements (Ahoy or C structs) first, then array structure
			return fmt.Sprintf("    if (%s) { for(int __i=0; __i<%s->length; __i++) { if(%s->data[__i]) free((void*)%s->data[__i]); } free(%s->data); free(%s->types); free(%s); }\n",
				varName, varName, varName, varName, varName, varName, varName)
		} else {
			// Just free array structure (elements are either primitives or managed elsewhere)
			return fmt.Sprintf("    if (%s) { free(%s->data); free(%s->types); free(%s); }\n",
				varName, varName, varName, varName)
		}
	} else if strings.HasPrefix(varType, "dict") || varType == "HashMap*" {
		// For dicts/HashMaps, use the dict cleanup function
		return fmt.Sprintf("    if (%s) { freeHashMap(%s); }\n", varName, varName)
	} else if varType == "AhoyJSON*" {
		// For JSON objects
		return fmt.Sprintf("    if (%s) { free(%s); }\n", varName, varName)
	} else if varType == "char*" || varType == "string" {
		// For heap-allocated strings
		return fmt.Sprintf("    if (%s) { free(%s); }\n", varName, varName)
	} else if strings.Contains(varType, "*") || gen.structs[varType] != nil || gen.cTypeDefinitions[varType] {
		// For struct pointers or other pointer types
		return fmt.Sprintf("    if (%s) { free(%s); }\n", varName, varName)
	}
	return ""
}

func (gen *CodeGenerator) addAutomaticDeferFrees() {
	// Add automatic defer free for heap-allocated variables at scope 0 that:
	// 1. Are heap-allocated at function scope (scope 0)
	// 2. Don't escape the function (not returned or stored globally)
	// 3. Aren't manually freed
	// 4. Aren't function parameters
	// 5. Haven't already been freed by exitScope (nested scope vars)

	// Sort heap allocated vars for deterministic output
	heapVars := make([]string, 0, len(gen.heapAllocatedVars))
	for varName := range gen.heapAllocatedVars {
		heapVars = append(heapVars, varName)
	}
	sort.Strings(heapVars)

	for _, varName := range heapVars {
		// Skip if not at function scope (scope 0) - nested scopes handled by exitScope
		if gen.heapVarScopes[varName] != 0 {
			continue
		}

		// Get the variable type first (needed for container check below)
		varType := gen.heapVarTypes[varName]
		if varType == "" {
			// Fallback to checking function/global vars
			if t, exists := gen.functionVars[varName]; exists {
				varType = t
			} else if t, exists := gen.variables[varName]; exists {
				varType = t
			}
		}

		if varType == "" {
			continue
		}

		// Check if this is a container type (array, dict)
		isContainer := strings.HasPrefix(varType, "array") || varType == "AhoyArray*" ||
			strings.HasPrefix(varType, "dict") || varType == "HashMap*"

		// Skip if variable escapes, UNLESS it's a container
		// Container variables should always be freed even if their contents escape
		if gen.escapingVars[varName] && !isContainer {
			continue
		}

		// Skip if manually freed
		if gen.manuallyFreedVars[varName] {
			continue
		}

		// Skip names mentioned in still-pending defers: those run after
		// this cleanup would, so freeing here would dangle them.
		if gen.deferredUsers[varName] {
			continue
		}

		// Skip if it's a function parameter
		if gen.functionParameters[varName] {
			continue
		}

		// Skip if already auto-freed (by exitScope)
		if gen.autoFreedVars[varName] {
			continue
		}

		// Generate the appropriate free call based on type
		freeCode := gen.generateFreeCodeForVar(varName, varType)

		if freeCode != "" {
			gen.deferredStatements = append(gen.deferredStatements, deferredStmt{code: freeCode})
			gen.autoFreedVars[varName] = true
		}
	}
}

func (gen *CodeGenerator) generateImportStatement(node *ahoy.ASTNode) {
	// Add include - check if it's a local or system include
	headerName := node.Value
	namespace := node.DataType // Namespace is stored in DataType field

	// Resolve relative paths to absolute paths
	resolvedHeaderName := headerName
	if strings.HasSuffix(headerName, ".h") && (strings.HasPrefix(headerName, "./") || strings.HasPrefix(headerName, "../")) {
		// It's a relative path - resolve it based on the source file location
		sourceDir := filepath.Dir(gen.sourceFilename)
		absPath, err := filepath.Abs(filepath.Join(sourceDir, headerName))
		if err == nil {
			// Verify the file exists
			if _, statErr := os.Stat(absPath); statErr == nil {
				resolvedHeaderName = absPath
			}
		}
	}

	if !gen.includes[resolvedHeaderName] {
		gen.includes[resolvedHeaderName] = true
		gen.orderedIncludes = append(gen.orderedIncludes, resolvedHeaderName)

		// If it's a C header file, parse it to get function name mappings
		if strings.HasSuffix(resolvedHeaderName, ".h") {
			// Try to find and parse the header file
			headerPath := ""
			if strings.HasPrefix(resolvedHeaderName, "/") {
				headerPath = resolvedHeaderName
			} else {
				// Try common locations
				locations := []string{
					resolvedHeaderName,
					"/usr/include/" + resolvedHeaderName,
					"/usr/local/include/" + resolvedHeaderName,
					"repos/raylib/src/" + resolvedHeaderName,
				}
				for _, loc := range locations {
					if _, err := os.Stat(loc); err == nil {
						headerPath = loc
						break
					}
				}
			}

			if headerPath != "" {
				// Use the shared helper to get or parse the header (uses in-memory cache)
				headerInfo := gen.getOrParseCHeader(headerPath)

				if headerInfo != nil {
					// Track struct/typedef names as known C types and store struct fields
					for typeName, structInfo := range headerInfo.Structs {
						gen.cTypeDefinitions[typeName] = true
						// Also register lowercase version for easier matching
						gen.cTypeDefinitions[strings.ToLower(typeName)] = true

						// Store struct fields for member access type inference
						if gen.cStructFields[typeName] == nil {
							gen.cStructFields[typeName] = make(map[string]string)
						}
						for _, field := range structInfo.Fields {
							gen.cStructFields[typeName][field.Name] = field.Type
						}
						// Also store lowercase version
						if gen.cStructFields[strings.ToLower(typeName)] == nil {
							gen.cStructFields[strings.ToLower(typeName)] = make(map[string]string)
						}
						for _, field := range structInfo.Fields {
							gen.cStructFields[strings.ToLower(typeName)][field.Name] = field.Type
						}
					}

					// Store typedef aliases
					for aliasName, typedefInfo := range headerInfo.Typedefs {
						gen.cTypedefs[aliasName] = typedefInfo.BaseType
						gen.cTypedefs[strings.ToLower(aliasName)] = typedefInfo.BaseType
						// Also mark the alias as a known type
						gen.cTypeDefinitions[aliasName] = true
						gen.cTypeDefinitions[strings.ToLower(aliasName)] = true
					}

					// If there's a namespace, store functions under that namespace
					if namespace != "" {
						if gen.cNamespaces[namespace] == nil {
							gen.cNamespaces[namespace] = make(map[string]string)
						}
						if gen.cNamespaceReturnTypes[namespace] == nil {
							gen.cNamespaceReturnTypes[namespace] = make(map[string]string)
						}
						for cFuncName, funcInfo := range headerInfo.Functions {
							snakeName := ahoy.PascalToSnake(cFuncName)
							gen.cNamespaces[namespace][snakeName] = cFuncName
							gen.cNamespaceReturnTypes[namespace][snakeName] = funcInfo.ReturnType

							// Also register the return type as a known C type if it's a struct
							if funcInfo.ReturnType != "" && funcInfo.ReturnType != "void" && funcInfo.ReturnType != "int" &&
								funcInfo.ReturnType != "float" && funcInfo.ReturnType != "double" && funcInfo.ReturnType != "char*" {
								gen.cTypeDefinitions[funcInfo.ReturnType] = true
								gen.cTypeDefinitions[strings.ToLower(funcInfo.ReturnType)] = true
							}

							// Store function parameter types
							paramTypes := []string{}
							for _, param := range funcInfo.Parameters {
								paramTypes = append(paramTypes, param.Type)
							}
							gen.cFunctionParamTypes[snakeName] = paramTypes
							for typedefName, td := range headerInfo.FuncPtrTypedefs {
								gen.cFuncPtrTypedefs[typedefName] = td
							}
						}
					} else {
						// No namespace - add to global scope
						for cFuncName, funcInfo := range headerInfo.Functions {
							snakeName := ahoy.PascalToSnake(cFuncName)
							gen.cFunctionNames[snakeName] = cFuncName
							gen.cFunctionReturnTypes[snakeName] = funcInfo.ReturnType

							// Also register the return type as a known C type if it's a struct
							if funcInfo.ReturnType != "" && funcInfo.ReturnType != "void" && funcInfo.ReturnType != "int" &&
								funcInfo.ReturnType != "float" && funcInfo.ReturnType != "double" && funcInfo.ReturnType != "char*" {
								gen.cTypeDefinitions[funcInfo.ReturnType] = true
								gen.cTypeDefinitions[strings.ToLower(funcInfo.ReturnType)] = true
							}

							// Store function parameter types
							paramTypes := []string{}
							for _, param := range funcInfo.Parameters {
								paramTypes = append(paramTypes, param.Type)
							}
							gen.cFunctionParamTypes[snakeName] = paramTypes
							for typedefName, td := range headerInfo.FuncPtrTypedefs {
								gen.cFuncPtrTypedefs[typedefName] = td
							}
						}
					}
				}
			}
		}
	}
}

// hoistCallArguments evaluates arguments that are themselves calls into
// temporary variables, and returns the replacement argument list.
//
// Print-style formatting reads each argument's inferred type and writes it out
// as a simple expression. A nested call is not one: print|len|items|| would
// otherwise emit the bare identifier 'len'. Storing the result first means the
// temporary is an ordinary variable of the call's return type, so every
// existing type branch - scalars, arrays, dicts, structs - keeps working.
func (gen *CodeGenerator) hoistCallArguments(args []*ahoy.ASTNode) []*ahoy.ASTNode {
	needsHoist := false
	for _, arg := range args {
		if arg != nil && arg.Type == ahoy.NODE_CALL {
			needsHoist = true
			break
		}
	}
	if !needsHoist {
		return args
	}

	out := make([]*ahoy.ASTNode, len(args))
	copy(out, args)

	wroteOne := false
	for i, arg := range out {
		if arg == nil || arg.Type != ahoy.NODE_CALL {
			continue
		}
		resultType := gen.inferType(arg)
		if resultType == "" || resultType == "void" || resultType == "unknown" {
			// No usable result to store; leave the argument alone.
			continue
		}

		tmp := fmt.Sprintf("__call_result_%d", gen.varCounter)
		gen.varCounter++

		if wroteOne {
			gen.writeIndent()
		}
		gen.output.WriteString(gen.mapType(resultType))
		gen.output.WriteString(" ")
		gen.output.WriteString(tmp)
		gen.output.WriteString(" = ")
		gen.generateNodeInternal(arg, false)
		gen.output.WriteString(";\n")
		wroteOne = true

		gen.functionVars[tmp] = resultType
		out[i] = &ahoy.ASTNode{Type: ahoy.NODE_IDENTIFIER, Value: tmp, DataType: resultType}
	}

	if wroteOne {
		// Indent whatever follows the temporaries (the printf itself).
		gen.writeIndent()
	}
	return out
}

func (gen *CodeGenerator) generateCall(node *ahoy.ASTNode) {
	// A variable holding a C function pointer is called directly through it.
	if td, ok := gen.cFuncPtrTypedefVar(node.Value); ok {
		gen.generateCallbackTypedefCall(node, td)
		return
	}
	// A variable of function type (fn(int)->int) is called indirectly.
	if ft, ok := gen.functionValueType(node.Value); ok {
		gen.generateIndirectCall(node, ft)
		return
	}

	// A distinct type name converts with one argument: same layout as the
	// base, so emit the argument directly (checked at parse/check time).
	if _, ok := gen.distinctTypes[node.Value]; ok {
		if len(node.Children) > 0 {
			gen.generateNode(node.Children[0])
		}
		return
	}

	// Keep user-defined functions as snake_case
	// Convert C library functions to their original names
	funcName := node.Value

	// Special case: rename main to ahoy_main
	if funcName == "main" {
		funcName = "ahoy_main"
	} else if gen.userFunctions[funcName] {
		// Keep user-defined function names as-is (snake_case)
		funcName = node.Value
	} else if cFuncName, exists := gen.cFunctionNames[funcName]; exists {
		// Use the actual C function name from the header
		funcName = cFuncName
	} else if strings.HasPrefix(funcName, "ahoy_json_") {
		// Keep JSON helper functions as-is (they're built-in)
		funcName = node.Value
	} else if strings.Contains(funcName, "_") {
		// External C library function not in headers - convert to PascalCase as fallback
		funcName = snakeToPascal(funcName)
	}

	// Handle special functions
	switch node.Value {
	case "len", "length":
		// len/length function - returns length of array, dict, string, etc.
		if len(node.Children) > 0 {
			arg := node.Children[0]
			argType := gen.inferType(arg)

			// Check if it's an array type
			if argType == "array" || strings.HasPrefix(argType, "array[") || argType == "AhoyArray*" {
				gen.output.WriteString("(")
				gen.generateNode(arg)
				gen.output.WriteString(")->length")
				return
			}

			// Check if it's a dict type
			if argType == "dict" || strings.HasPrefix(argType, "dict[") || argType == "HashMap*" {
				gen.output.WriteString("(")
				gen.generateNode(arg)
				gen.output.WriteString(")->size")
				return
			}

			// Check if it's a string type
			if argType == "string" || argType == "char*" || argType == "const char*" {
				gen.output.WriteString("strlen(")
				gen.generateNode(arg)
				gen.output.WriteString(")")
				return
			}

			// For unknown types, try to call ->length as fallback
			gen.output.WriteString("(")
			gen.generateNode(arg)
			gen.output.WriteString(")->length")
			return
		}
		return
	case "jobs":
		gen.useJobs = true
		gen.output.WriteString("ahoy_jobs_new(")
		if len(node.Children) > 0 {
			gen.generateNode(node.Children[0])
		} else {
			gen.output.WriteString("4")
		}
		gen.output.WriteString(")")
		return

	case "print":
		// Arguments that are themselves calls are evaluated into temporaries first;
		// see hoistCallArguments.
		printArgs := gen.hoistCallArguments(node.Children)
		// Check if we have multiple arguments or if first arg is a format string
		hasMultipleArgs := len(printArgs) > 1
		firstIsString := len(printArgs) > 0 && printArgs[0].Type == ahoy.NODE_STRING

		// If first argument is a string AND it looks like a format string (has {} or %), treat it as one
		if firstIsString && !hasMultipleArgs {
			// Single string argument - just print it
			gen.output.WriteString("printf(")
			formatStr := printArgs[0].Value
			if !strings.HasSuffix(formatStr, "\\n") {
				formatStr += "\\n"
			}
			gen.output.WriteString("\"")
			gen.output.WriteString(formatStr)
			gen.output.WriteString("\"")
			gen.output.WriteString(")")
			return
		} else if firstIsString && (strings.Contains(printArgs[0].Value, "{}") || strings.Contains(printArgs[0].Value, "%")) {
			// First arg is a format string with placeholders
			gen.output.WriteString("printf(")
			formatStr := printArgs[0].Value
			args := printArgs[1:]

			// Process %v and %t in format string
			processedFormat, processedArgs := gen.processFormatString(formatStr, args)

			// Auto-add newline if not present
			if !strings.HasSuffix(processedFormat, "\\n") {
				processedFormat += "\\n"
			}

			// Output processed format string
			gen.output.WriteString("\"")
			gen.output.WriteString(processedFormat)
			gen.output.WriteString("\"")

			// Output processed arguments
			for _, arg := range processedArgs {
				gen.output.WriteString(", ")
				gen.generateNode(arg)
			}
			gen.output.WriteString(")")
			return
		} else {
			// Multiple arguments without format string - print on one line with spaces (Python-style)
			gen.output.WriteString("printf(")
			if len(printArgs) > 0 {
				formatParts := []string{}

				// Build format string with spaces between arguments
				for _, arg := range printArgs {
					argType := gen.inferType(arg)
					formatSpec := ""

					// Check if this is HashMap member access or dict access - we can't determine type at codegen time
					isHashMapAccess := false
					if arg.Type == ahoy.NODE_MEMBER_ACCESS && len(arg.Children) > 0 && !isDictSizeMember(arg.Value) {
						objType := gen.inferType(arg.Children[0])
						if objType == "HashMap*" || objType == "dict" {
							isHashMapAccess = true
							// For HashMap, format as string by default (will use print_dict_value helper)
							formatSpec = "%s"
						}
					} else if arg.Type == ahoy.NODE_DICT_ACCESS {
						// Direct dict access like dict<key> - will use format_dict_value
						isHashMapAccess = true
						formatSpec = "%s"
					}

					// Check if argument is an enum itself (needs special handling)
					if !isHashMapAccess && arg.Type == ahoy.NODE_IDENTIFIER && gen.isEnumType(arg.Value) {
						formatSpec = "%s" // enum print function returns string
					} else if !isHashMapAccess && arg.Type == ahoy.NODE_IDENTIFIER {
						// Check if this variable came from dict access
						if _, isDictSourced := gen.dictSourcedVars[arg.Value]; isDictSourced {
							formatSpec = "%s" // Will use format_dict_value
						} else {
							switch argType {
							case "string", "char*", "const char*":
								formatSpec = "%s"
							case "int":
								formatSpec = "%d"
							case "intptr_t":
								formatSpec = "%ld"
							case "float", "double":
								formatSpec = "%g"
							case "bool":
								formatSpec = "%d"
							case "char":
								formatSpec = "%c"
							case "array":
								formatSpec = "%s" // Will use print_array_helper
							case "dict":
								formatSpec = "%s" // Will use print_dict_helper
							case "struct":
								formatSpec = "%s" // Will use print_struct_helper
							case "AhoyJSON*", "json":
								formatSpec = "%s" // Will use ahoy_json_stringify
							default:
								// Check for typed collections
								if strings.HasPrefix(argType, "array[") {
									formatSpec = "%s" // Will use print_array_helper
								} else if strings.HasPrefix(argType, "dict[") {
									formatSpec = "%s" // Will use print_dict_helper
								} else if _, isStruct := gen.structs[argType]; isStruct {
									formatSpec = "%s" // Will use print_struct_helper
								} else if _, isStruct := gen.structs[strings.ToLower(argType)]; isStruct {
									// Check lowercase version for built-in types
									formatSpec = "%s" // Will use print_struct_helper
								} else {
									formatSpec = "%d"
								}
							}
						}
					} else if !isHashMapAccess {
						switch argType {
						case "string", "char*", "const char*":
							formatSpec = "%s"
						case "int":
							formatSpec = "%d"
						case "intptr_t":
							formatSpec = "%ld"
						case "float", "double":
							formatSpec = "%g"
						case "bool":
							formatSpec = "%d"
						case "char":
							formatSpec = "%c"
						case "array":
							formatSpec = "%s" // Will use print_array_helper
						case "dict":
							formatSpec = "%s" // Will use print_dict_helper
						case "struct":
							formatSpec = "%s" // Will use print_struct_helper
						case "AhoyJSON*", "json":
							formatSpec = "%s" // Will use ahoy_json_stringify
						default:
							// Check for typed collections
							if strings.HasPrefix(argType, "array[") {
								formatSpec = "%s" // Will use print_array_helper
							} else if strings.HasPrefix(argType, "dict[") {
								formatSpec = "%s" // Will use print_dict_helper
							} else if _, isStruct := gen.structs[argType]; isStruct {
								formatSpec = "%s" // Will use print_struct_helper
							} else if _, isStruct := gen.structs[strings.ToLower(argType)]; isStruct {
								// Check lowercase version for built-in types
								formatSpec = "%s" // Will use print_struct_helper
							} else {
								formatSpec = "%d"
							}
						}
					}

					formatParts = append(formatParts, formatSpec)
				}

				// Join with spaces and add newline
				formatStr := strings.Join(formatParts, " ") + "\\n"
				gen.output.WriteString("\"")
				gen.output.WriteString(formatStr)
				gen.output.WriteString("\"")

				// Output all arguments
				for _, arg := range printArgs {
					gen.output.WriteString(", ")
					argType := gen.inferType(arg)

					// Check if argument is an enum itself (print the whole enum)
					if arg.Type == ahoy.NODE_IDENTIFIER && gen.isEnumType(arg.Value) {
						gen.output.WriteString("print_")
						gen.output.WriteString(arg.Value)
						gen.output.WriteString("()")
						continue
					}

					// Special handling for arrays and dicts
					if argType == "array" || strings.HasPrefix(argType, "array[") {
						// Check if we know the element type for this array
						if arg.Type == ahoy.NODE_IDENTIFIER {
							if elemType, exists := gen.arrayElementTypes[arg.Value]; exists {
								if elemType == "char*" || elemType == "string" {
									// String array - use special helper
									gen.arrayMethods["print_string_array"] = true
									gen.output.WriteString("print_string_array_helper(")
									gen.generateNode(arg)
									gen.output.WriteString(")")
								} else {
									// Int/numeric array - use regular helper
									gen.arrayMethods["print_array"] = true
									gen.output.WriteString("print_array_helper(")
									gen.generateNode(arg)
									gen.output.WriteString(")")
								}
							} else {
								// Unknown type, use default
								gen.arrayMethods["print_array"] = true
								gen.output.WriteString("print_array_helper(")
								gen.generateNode(arg)
								gen.output.WriteString(")")
							}
						} else {
							gen.arrayMethods["print_array"] = true
							gen.output.WriteString("print_array_helper(")
							gen.generateNode(arg)
							gen.output.WriteString(")")
						}
					} else if argType == "dict" || strings.HasPrefix(argType, "dict[") {
						gen.dictMethods["print_dict"] = true
						gen.output.WriteString("print_dict_helper(")
						gen.generateNode(arg)
						gen.output.WriteString(")")
					} else if argType == "AhoyJSON*" || argType == "json" {
						// JSON type - use stringify helper
						gen.output.WriteString("ahoy_json_stringify(")
						gen.generateNode(arg)
						gen.output.WriteString(")")
					} else if argType == "struct" || gen.structs[argType] != nil || gen.structs[strings.ToLower(argType)] != nil {
						// Struct type - use print helper
						gen.arrayMethods["print_struct"] = true
						gen.output.WriteString("print_struct_helper_")
						// Look up struct info to get the canonical name for print helper
						funcName := ""
						if structInfo := gen.structs[argType]; structInfo != nil {
							funcName = toCFuncName(structInfo.Name)
						} else if structInfo := gen.structs[strings.ToLower(argType)]; structInfo != nil {
							funcName = toCFuncName(structInfo.Name)
						} else {
							funcName = toCFuncName(argType)
						}
						gen.output.WriteString(funcName)
						gen.output.WriteString("(")
						gen.generateNode(arg)
						gen.output.WriteString(")")
					} else {
						// Check if this is dict access (returns double but may be string)
						if arg.Type == ahoy.NODE_DICT_ACCESS {
							// Dict access returns double, but could be string - use format_dict_value
							gen.output.WriteString("format_dict_value(")
							// Cast dict to HashMap* if needed
							dictType := gen.inferType(arg)
							if dictType == "float" {
								// Check if the dict itself is generic
								dictName := arg.Value
								varType := ""
								if vt, exists := gen.variables[dictName]; exists {
									varType = vt
								} else if vt, exists := gen.functionVars[dictName]; exists {
									varType = vt
								}
								if varType == "generic" || varType == "any" {
									gen.output.WriteString("(HashMap*)")
								}
							}
							gen.output.WriteString(arg.Value)
							gen.output.WriteString(", ")
							gen.generateNode(arg.Children[0])
							gen.output.WriteString(")")
						} else if arg.Type == ahoy.NODE_IDENTIFIER {
							// Check if this variable came from dict access
							if dictName, isDictSourced := gen.dictSourcedVars[arg.Value]; isDictSourced {
								if key, hasKey := gen.dictSourcedKeys[arg.Value]; hasKey {
									gen.output.WriteString("format_dict_value(")
									gen.output.WriteString(dictName)
									gen.output.WriteString(", \"")
									gen.output.WriteString(key)
									gen.output.WriteString("\")")
								} else {
									gen.generateNode(arg)
								}
							} else {
								// Check if it's a double variable (from dict) that might be a string
								argType := gen.inferType(arg)
								if argType == "float" {
									// Could be a string from dict - cast via format helper if available
									// For now just generate normally, DrawText will handle casting
									gen.generateNode(arg)
								} else {
									gen.generateNode(arg)
								}
							}
						} else if arg.Type == ahoy.NODE_MEMBER_ACCESS && len(arg.Children) > 0 {
							objType := gen.inferType(arg.Children[0])
							if (objType == "HashMap*" || objType == "dict") && !isDictSizeMember(arg.Value) {
								// Use format_dict_value helper
								gen.output.WriteString("format_dict_value(")
								gen.generateNode(arg.Children[0])
								gen.output.WriteString(", \"")
								gen.output.WriteString(arg.Value)
								gen.output.WriteString("\")")
							} else {
								gen.generateNode(arg)
							}
						} else {
							gen.generateNode(arg)
						}
					}
				}
			}
			gen.output.WriteString(")")
			return
		}

	case "log":
		// log|message, file_path| - logs to file with timestamp
		gen.includes["time.h"] = true
		if !contains(gen.orderedIncludes, "time.h") {
			gen.orderedIncludes = append(gen.orderedIncludes, "time.h")
		}

		gen.output.WriteString("({ ")
		gen.output.WriteString("FILE* __log_file = fopen(")
		if len(node.Children) >= 2 {
			gen.generateNode(node.Children[1]) // file_path
		} else {
			gen.output.WriteString("\"ahoy.log\"")
		}
		gen.output.WriteString(", \"a\"); ")
		gen.output.WriteString("if (__log_file) { ")
		gen.output.WriteString("time_t __now = time(NULL); ")
		gen.output.WriteString("char __time_buf[26]; ")
		gen.output.WriteString("struct tm* __tm_info = localtime(&__now); ")
		gen.output.WriteString("strftime(__time_buf, 26, \"%Y-%m-%d %H:%M:%S\", __tm_info); ")
		gen.output.WriteString("fprintf(__log_file, \"[%s] \", __time_buf); ")

		// Handle message formatting similar to print
		if len(node.Children) > 0 {
			firstArg := node.Children[0]
			if firstArg.Type == ahoy.NODE_STRING {
				formatStr := firstArg.Value
				gen.output.WriteString("fprintf(__log_file, \"")
				gen.output.WriteString(formatStr)
				gen.output.WriteString("\\n\"")
				// Add additional arguments if any (before file_path)
				for i := 1; i < len(node.Children)-1; i++ {
					gen.output.WriteString(", ")
					gen.generateNode(node.Children[i])
				}
				gen.output.WriteString("); ")
			} else {
				// Non-string message, convert to string representation
				gen.output.WriteString("fprintf(__log_file, \"%s\\n\", ")
				gen.generateNode(firstArg)
				gen.output.WriteString("); ")
			}
		}

		gen.output.WriteString("fclose(__log_file); } ")
		gen.output.WriteString("})")
		return

	case "panic":
		// panic|error| - prints error and exits
		gen.output.WriteString("({ printf(\"PANIC: \"); ")

		// Handle panic arguments similar to print
		if len(node.Children) > 0 {
			hasMultipleArgs := len(node.Children) > 1
			firstIsString := node.Children[0].Type == ahoy.NODE_STRING

			if firstIsString && !hasMultipleArgs {
				// Single string argument
				formatStr := node.Children[0].Value
				if !strings.HasSuffix(formatStr, "\\n") {
					formatStr += "\\n"
				}
				gen.output.WriteString("printf(\"")
				gen.output.WriteString(formatStr)
				gen.output.WriteString("\")")
			} else if firstIsString && (strings.Contains(node.Children[0].Value, "{}") || strings.Contains(node.Children[0].Value, "%")) {
				// Format string with placeholders
				gen.output.WriteString("printf(")
				formatStr := node.Children[0].Value
				args := node.Children[1:]

				processedFormat, processedArgs := gen.processFormatString(formatStr, args)

				if !strings.HasSuffix(processedFormat, "\\n") {
					processedFormat += "\\n"
				}

				gen.output.WriteString("\"")
				gen.output.WriteString(processedFormat)
				gen.output.WriteString("\"")

				for _, arg := range processedArgs {
					gen.output.WriteString(", ")
					gen.generateNode(arg)
				}
				gen.output.WriteString(")")
			} else {
				// Multiple arguments or single non-string
				gen.output.WriteString("printf(")
				if len(node.Children) > 0 {
					formatParts := []string{}

					for _, arg := range node.Children {
						argType := gen.inferType(arg)
						formatSpec := ""

						switch argType {
						case "string", "char*", "const char*":
							formatSpec = "%s"
						case "int":
							formatSpec = "%d"
						case "intptr_t":
							formatSpec = "%ld"
						case "float", "double":
							formatSpec = "%g"
						case "bool":
							formatSpec = "%d"
						case "char":
							formatSpec = "%c"
						default:
							formatSpec = "%d"
						}

						formatParts = append(formatParts, formatSpec)
					}

					format := strings.Join(formatParts, " ") + "\\n"
					gen.output.WriteString("\"")
					gen.output.WriteString(format)
					gen.output.WriteString("\"")

					for _, arg := range node.Children {
						gen.output.WriteString(", ")
						gen.generateNode(arg)
					}
				}
				gen.output.WriteString(")")
			}
		}

		gen.output.WriteString("; exit(1); })")
		return

	case "sprintf":
		// sprintf returns a string - need to allocate buffer
		gen.output.WriteString("({ char* __str_buf = malloc(256); sprintf(__str_buf")

		// Process format string
		if len(node.Children) > 0 && node.Children[0].Type == ahoy.NODE_STRING {
			formatStr := node.Children[0].Value
			args := node.Children[1:]

			processedFormat, processedArgs := gen.processFormatString(formatStr, args)

			gen.output.WriteString(", \"")
			gen.output.WriteString(processedFormat)
			gen.output.WriteString("\"")

			for _, arg := range processedArgs {
				gen.output.WriteString(", ")
				gen.generateNode(arg)
			}
		}
		gen.output.WriteString("); __str_buf; })")

	case "__print_array_helper":
		// Special case for array printing - don't convert to PascalCase
		gen.output.WriteString("print_array_helper(")
		for i, arg := range node.Children {
			if i > 0 {
				gen.output.WriteString(", ")
			}
			gen.generateNode(arg)
		}
		gen.output.WriteString(")")

	// Type casts
	case "int":
		// int("42") parses the text. The plain C cast returned the pointer as an
		// int, so int("42") printed a huge address-like number.
		if len(node.Children) > 0 && gen.isStringExpr(node.Children[0]) {
			gen.output.WriteString("atoi(")
			gen.generateNode(node.Children[0])
			gen.output.WriteString(")")
			return
		}
		gen.output.WriteString("((int)(")
		if len(node.Children) > 0 {
			gen.generateNode(node.Children[0])
		}
		gen.output.WriteString("))")

	case "float":
		// float("1.5") parses the text, like int(string).
		if len(node.Children) > 0 && gen.isStringExpr(node.Children[0]) {
			gen.output.WriteString("atof(")
			gen.generateNode(node.Children[0])
			gen.output.WriteString(")")
			return
		}
		gen.output.WriteString("((float)(")
		if len(node.Children) > 0 {
			gen.generateNode(node.Children[0])
		}
		gen.output.WriteString("))")

	case "char":
		gen.output.WriteString("((char)(")
		if len(node.Children) > 0 {
			gen.generateNode(node.Children[0])
		}
		gen.output.WriteString("))")

	case "string":
		// String cast - convert number to string
		if len(node.Children) > 0 {
			argType := gen.inferType(node.Children[0])
			gen.output.WriteString("({ char* __cast_buf = malloc(32); ")

			switch argType {
			case "int":
				gen.output.WriteString("sprintf(__cast_buf, \"%d\", ")
				gen.generateNode(node.Children[0])
				gen.output.WriteString("); __cast_buf; })")
			case "float":
				gen.output.WriteString("sprintf(__cast_buf, \"%f\", ")
				gen.generateNode(node.Children[0])
				gen.output.WriteString("); __cast_buf; })")
			case "char":
				gen.output.WriteString("sprintf(__cast_buf, \"%c\", ")
				gen.generateNode(node.Children[0])
				gen.output.WriteString("); __cast_buf; })")
			case "bool":
				gen.output.WriteString("sprintf(__cast_buf, \"%s\", ")
				gen.generateNode(node.Children[0])
				gen.output.WriteString(" ? \"true\" : \"false\"); __cast_buf; })")
			default:
				// Already a string or unknown - just pass through
				gen.generateNode(node.Children[0])
			}
		}

	case "read_json":
		// Mark that JSON is used
		if !gen.useJSON {
			gen.useJSON = true
			// Register JSON function return types immediately
			gen.registerJSONFunctionTypes()
		}
		// read_json returns (AhoyJSON*, char* error)
		gen.output.WriteString("ahoy_json_read(")
		if len(node.Children) > 0 {
			gen.generateNode(node.Children[0])
		}
		gen.output.WriteString(")")

	case "write_json":
		// Mark that JSON is used
		if !gen.useJSON {
			gen.useJSON = true
			// Register JSON function return types immediately
			gen.registerJSONFunctionTypes()
		}
		// write_json(filename, json) returns char* error
		gen.output.WriteString("ahoy_json_write(")
		if len(node.Children) > 0 {
			gen.generateNode(node.Children[0])
		}
		if len(node.Children) > 1 {
			gen.output.WriteString(", ")
			gen.generateNode(node.Children[1])
		}
		gen.output.WriteString(")")

	default:
		gen.output.WriteString(funcName)
		gen.output.WriteString("(")

		// Check if we have parameter type information for this function
		paramTypes, hasParamInfo := gen.functionParamTypes[node.Value]

		// Check for implicit named arguments (all args are identifiers matching param names)
		paramNames, hasParamNames := gen.functionParamNames[node.Value]
		if hasParamNames && hasParamInfo && len(node.Children) == len(paramNames) {
			allArgsAreIdentifiers := true
			allMatchParamNames := true

			for _, arg := range node.Children {
				if arg.Type != ahoy.NODE_IDENTIFIER {
					allArgsAreIdentifiers = false
					break
				}
			}

			if allArgsAreIdentifiers {
				// Check if all argument names match parameter names (in any order)
				argNamesMap := make(map[string]bool)
				for _, arg := range node.Children {
					argNamesMap[arg.Value] = true
				}

				for _, paramName := range paramNames {
					if !argNamesMap[paramName] {
						allMatchParamNames = false
						break
					}
				}

				// If all arguments are identifiers matching parameter names, reorder them
				if allMatchParamNames {
					reorderedChildren := make([]*ahoy.ASTNode, len(paramNames))
					for i, paramName := range paramNames {
						// Find the argument with this parameter name
						for _, arg := range node.Children {
							if arg.Value == paramName {
								reorderedChildren[i] = arg
								break
							}
						}
					}
					node.Children = reorderedChildren
				}
			}
		}

		// Check if any arguments are named (node.Value == "named_arg")
		hasNamedArgs := false
		for _, arg := range node.Children {
			if arg.Type == ahoy.NODE_BINARY_OP && arg.Value == "named_arg" {
				hasNamedArgs = true
				break
			}
		}

		if hasNamedArgs {
			// Handle named arguments by reordering based on function signature
			paramNames, hasParamNames := gen.functionParamNames[node.Value]

			if hasParamNames && hasParamInfo {
				// Create a map to store arguments by name
				namedArgs := make(map[string]*ahoy.ASTNode)
				positionalArgs := []*ahoy.ASTNode{}
				positionalIndex := 0

				// Separate named and positional arguments
				for _, arg := range node.Children {
					if arg.Type == ahoy.NODE_BINARY_OP && arg.Value == "named_arg" {
						argName := arg.Children[0].Value
						namedArgs[argName] = arg.Children[1]
					} else {
						positionalArgs = append(positionalArgs, arg)
					}
				}

				// Generate arguments in the order defined by function signature
				for i, paramName := range paramNames {
					if i > 0 {
						gen.output.WriteString(", ")
					}

					// Check if this parameter was provided as named argument
					if argNode, exists := namedArgs[paramName]; exists {
						// Automatic numeric type casting for user-defined function parameters
						if hasParamInfo && i < len(paramTypes) {
							paramType := strings.TrimSpace(paramTypes[i])
							argType := gen.inferType(argNode)

							isIntParam := paramType == "int"
							isFloatParam := paramType == "float" || paramType == "double"
							isIntArg := argType == "int"
							isFloatArg := argType == "float" || argType == "double"

							// Cast int to float/double when needed
							if isFloatParam && isIntArg {
								gen.output.WriteString("(")
								gen.output.WriteString(paramType)
								gen.output.WriteString(")")
							}
							// Cast float/double to int when needed
							if isIntParam && isFloatArg {
								gen.output.WriteString("(")
								gen.output.WriteString(paramType)
								gen.output.WriteString(")")
							}
						}

						if hasParamInfo && i < len(paramTypes) && (paramTypes[i] == "generic" || paramTypes[i] == "any") {
							argType := gen.inferType(argNode)
							// Cast all pointer types to intptr_t for generic/any parameters
							if argType == "string" || argType == "char*" || argType == "const char*" ||
								argType == "array" || strings.HasPrefix(argType, "array[") ||
								argType == "dict" || strings.HasPrefix(argType, "dict[") || strings.HasPrefix(argType, "dict<") ||
								argType == "HashMap*" || strings.HasSuffix(argType, "*") {
								gen.output.WriteString("(intptr_t)")
							}
						}
						gen.generateNode(argNode)
					} else if positionalIndex < len(positionalArgs) {
						// Use positional argument
						argNode := positionalArgs[positionalIndex]
						positionalIndex++

						// Automatic numeric type casting for user-defined function parameters
						if hasParamInfo && i < len(paramTypes) {
							paramType := strings.TrimSpace(paramTypes[i])
							argType := gen.inferType(argNode)

							isIntParam := paramType == "int"
							isFloatParam := paramType == "float" || paramType == "double"
							isIntArg := argType == "int"
							isFloatArg := argType == "float" || argType == "double"

							// Cast int to float/double when needed
							if isFloatParam && isIntArg {
								gen.output.WriteString("(")
								gen.output.WriteString(paramType)
								gen.output.WriteString(")")
							}
							// Cast float/double to int when needed
							if isIntParam && isFloatArg {
								gen.output.WriteString("(")
								gen.output.WriteString(paramType)
								gen.output.WriteString(")")
							}
						}

						if hasParamInfo && i < len(paramTypes) && (paramTypes[i] == "generic" || paramTypes[i] == "any") {
							argType := gen.inferType(argNode)
							// Cast all pointer types to intptr_t for generic/any parameters
							if argType == "string" || argType == "char*" || argType == "const char*" ||
								argType == "array" || strings.HasPrefix(argType, "array[") ||
								argType == "dict" || strings.HasPrefix(argType, "dict[") || strings.HasPrefix(argType, "dict<") ||
								argType == "HashMap*" || strings.HasSuffix(argType, "*") {
								gen.output.WriteString("(intptr_t)")
							}
						}
						gen.generateNode(argNode)
					} else {
						// Parameter not provided - use default value
						paramDefaults, hasDefaults := gen.functionParamDefaults[node.Value]
						if hasDefaults && i < len(paramDefaults) && paramDefaults[i] != nil {
							// Generate the default value
							gen.generateNode(paramDefaults[i])
						} else {
							// No default value - error case
							gen.output.WriteString("0 /* missing arg */")
						}
					}
				}
			} else {
				// No parameter info - generate in order provided
				for i, arg := range node.Children {
					if i > 0 {
						gen.output.WriteString(", ")
					}

					if arg.Type == ahoy.NODE_BINARY_OP && arg.Value == "named_arg" {
						gen.generateNode(arg.Children[1])
					} else {
						gen.generateNode(arg)
					}
				}
			}
		} else {
			// Regular positional arguments
			// Check if we have C function parameter type information
			cParamTypes, hasCParamInfo := gen.cFunctionParamTypes[node.Value]

			for i, arg := range node.Children {
				if i > 0 {
					gen.output.WriteString(", ")
				}

				// An imported function's callback parameter takes a raw C function
				// pointer, not an AhoyFn.
				if hasCParamInfo && i < len(cParamTypes) && gen.cFuncPtrTypedefs[cParamTypes[i]] != nil {
					if gen.emitCCallbackArg(arg) {
						continue
					}
				}

				// Special case: DrawText first parameter expects char*, cast doubles from dict
				if funcName == "DrawText" && i == 0 {
					argType := gen.inferType(arg)
					if argType == "float" {
						// Dict access returns double, cast to char* for string values
						gen.output.WriteString("(char*)(intptr_t)")
					}
				}

				// Check if C function parameter is void* or const void* - add & for non-pointer arguments
				if hasCParamInfo && i < len(cParamTypes) {
					paramType := cParamTypes[i]
					if paramType == "void *" || paramType == "const void *" ||
						paramType == "void*" || paramType == "const void*" {
						argType := gen.inferType(arg)
						// Only add & if the argument is not already a pointer type
						// AND the argument is a variable (not a literal, constant, or enum)
						isLValue := arg.Type == ahoy.NODE_IDENTIFIER &&
							!gen.isConstantOrEnum(arg.Value)
						if isLValue && !strings.HasSuffix(argType, "*") && argType != "array" &&
							!strings.HasPrefix(argType, "array[") && argType != "dict" &&
							!strings.HasPrefix(argType, "dict[") && argType != "HashMap*" &&
							argType != "AhoyArray*" && argType != "string" && argType != "char*" {
							gen.output.WriteString("&")
						}
					}
				}

				// Automatic numeric type casting for C function parameters
				if hasCParamInfo && i < len(cParamTypes) {
					paramType := strings.TrimSpace(cParamTypes[i])
					argType := gen.inferType(arg)

					// Map C types to their normalized forms
					normalizeType := func(t string) string {
						t = strings.TrimSpace(t)
						t = strings.TrimPrefix(t, "const ")
						t = strings.TrimSpace(t)
						return t
					}

					paramType = normalizeType(paramType)
					argType = normalizeType(argType)

					isIntParam := paramType == "int" || paramType == "int32_t" ||
						paramType == "int16_t" || paramType == "int8_t" ||
						paramType == "uint32_t" || paramType == "uint16_t" ||
						paramType == "uint8_t" || paramType == "unsigned int" ||
						paramType == "short" || paramType == "unsigned short" ||
						paramType == "long" || paramType == "unsigned long"

					isFloatParam := paramType == "float" || paramType == "double"

					isIntArg := argType == "int" || argType == "int32_t" ||
						argType == "int16_t" || argType == "int8_t" ||
						argType == "uint32_t" || argType == "uint16_t" ||
						argType == "uint8_t"

					isFloatArg := argType == "float" || argType == "double"

					// Cast int to float/double when needed
					if isFloatParam && isIntArg {
						gen.output.WriteString("(")
						gen.output.WriteString(paramType)
						gen.output.WriteString(")")
					}
					// Cast float/double to int when needed
					if isIntParam && isFloatArg {
						gen.output.WriteString("(")
						gen.output.WriteString(paramType)
						gen.output.WriteString(")")
					}
				}

				// Automatic numeric type casting for user-defined function parameters
				if hasParamInfo && i < len(paramTypes) {
					paramType := strings.TrimSpace(paramTypes[i])
					argType := gen.inferType(arg)

					isIntParam := paramType == "int"
					isFloatParam := paramType == "float" || paramType == "double"
					isIntArg := argType == "int"
					isFloatArg := argType == "float" || argType == "double"

					// Cast int to float/double when needed
					if isFloatParam && isIntArg {
						gen.output.WriteString("(")
						gen.output.WriteString(paramType)
						gen.output.WriteString(")")
					}
					// Cast float/double to int when needed
					if isIntParam && isFloatArg {
						gen.output.WriteString("(")
						gen.output.WriteString(paramType)
						gen.output.WriteString(")")
					}
				}

				// If this parameter is generic/any, cast pointer types to intptr_t
				if hasParamInfo && i < len(paramTypes) && (paramTypes[i] == "generic" || paramTypes[i] == "any") {
					argType := gen.inferType(arg)
					// Cast all pointer types (strings, arrays, dicts, structs) to intptr_t
					if argType == "string" || argType == "char*" || argType == "const char*" ||
						argType == "array" || strings.HasPrefix(argType, "array[") ||
						argType == "dict" || strings.HasPrefix(argType, "dict[") || strings.HasPrefix(argType, "dict<") ||
						argType == "HashMap*" || strings.HasSuffix(argType, "*") {
						gen.output.WriteString("(intptr_t)")
					}
				}

				gen.generateNode(arg)
			}
		}
		gen.output.WriteString(")")
	}
}

// isStringExpr reports whether an operand is a string (or a char* from C), so
// equality on it has to compare contents rather than pointers.
func (gen *CodeGenerator) isStringExpr(n *ahoy.ASTNode) bool {
	if n == nil {
		return false
	}
	return isStringTypeName(gen.inferType(n))
}

// isStringTypeName reports whether an inferred type name is a string type.
func isStringTypeName(t string) bool {
	return t == "string" || t == "char*" || t == "const char*"
}

// checkedArrayElem returns the tracked element type for an array variable,
// verifying it against the variable's declared type. A tracked entry that
// disagrees with an explicit array[T] declaration means the compiler lost
// information (the stale-entry class that read string elements as int and
// segfaulted); that is a loud internal error, never a silent fallback.
// Genuinely unknown arrays (no entry, untyped/any declarations) still fall
// through quietly - only provable disagreement aborts. On mismatch the
// declared type is returned so any further output stays consistent, but
// compilation always stops via hasError.
func (gen *CodeGenerator) checkedArrayElem(arrayName string, node *ahoy.ASTNode) (string, bool) {
	elem, ok := gen.arrayElementTypes[arrayName]
	if !ok {
		return "", false
	}
	declared := ""
	if t, exists := gen.functionVars[arrayName]; exists {
		declared = t
	} else if t, exists := gen.variables[arrayName]; exists {
		declared = t
	}
	want := ""
	if strings.HasPrefix(declared, "array[") && strings.HasSuffix(declared, "]") {
		want = strings.TrimSuffix(strings.TrimPrefix(declared, "array["), "]")
	}
	if want == "" || want == "any" || want == "generic" || elem == "any" || elem == "generic" || want == elem {
		return elem, true
	}
	if gen.reportedElemMismatch == nil {
		gen.reportedElemMismatch = make(map[string]bool)
	}
	if !gen.reportedElemMismatch[arrayName] {
		gen.reportedElemMismatch[arrayName] = true
		file := ""
		line := 0
		if node != nil {
			file = node.File
			line = node.Line
		}
		if file == "" {
			file = gen.currentFile
		}
		if file == "" {
			file = gen.sourceFilename
		}
		fmt.Printf("\n❌ Internal compiler error at %s:%d: array '%s' is declared %s but tracked as array[%s]; refusing to guess (this stale-entry bug segfaults). Please report this.\n\n", file, line, arrayName, declared, elem)
		gen.hasError = true
	}
	return want, true
}

func (gen *CodeGenerator) generateBinaryOp(node *ahoy.ASTNode) {
	switch node.Value {
	case "is":
		if gen.isStringExpr(node.Children[0]) && gen.isStringExpr(node.Children[1]) {
			// Strings compare by value. `a is b` on two char* was pointer
			// equality, so two equal strings read as different and a name
			// lookup over an array of strings never matched.
			gen.output.WriteString("(strcmp(")
			gen.generateNode(node.Children[0])
			gen.output.WriteString(", ")
			gen.generateNode(node.Children[1])
			gen.output.WriteString(") == 0)")
			return
		}
		gen.output.WriteString("(")
		gen.generateNode(node.Children[0])
		gen.output.WriteString(" == ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(")")
	case "or":
		gen.output.WriteString("(")
		gen.generateNode(node.Children[0])
		gen.output.WriteString(" || ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(")")
	case "and":
		gen.output.WriteString("(")
		gen.generateNode(node.Children[0])
		gen.output.WriteString(" && ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(")")
	case "plus":
		gen.output.WriteString("(")
		gen.generateNode(node.Children[0])
		gen.output.WriteString(" + ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(")")
	case "minus":
		gen.output.WriteString("(")
		gen.generateNode(node.Children[0])
		gen.output.WriteString(" - ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(")")
	case "times":
		gen.output.WriteString("(")
		gen.generateNode(node.Children[0])
		gen.output.WriteString(" * ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(")")
	case "div":
		gen.output.WriteString("(")
		gen.generateNode(node.Children[0])
		gen.output.WriteString(" / ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(")")
	case "mod":
		gen.output.WriteString("(")
		gen.generateNode(node.Children[0])
		gen.output.WriteString(" % ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(")")
	case "band":
		gen.output.WriteString("(")
		gen.generateNode(node.Children[0])
		gen.output.WriteString(" & ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(")")
	case "bor":
		gen.output.WriteString("(")
		gen.generateNode(node.Children[0])
		gen.output.WriteString(" | ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(")")
	case "bxor":
		gen.output.WriteString("(")
		gen.generateNode(node.Children[0])
		gen.output.WriteString(" ^ ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(")")
	case "shl":
		gen.output.WriteString("(")
		gen.generateNode(node.Children[0])
		gen.output.WriteString(" << ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(")")
	case "shr":
		gen.output.WriteString("(")
		gen.generateNode(node.Children[0])
		gen.output.WriteString(" >> ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(")")
	case "+":
		if len(node.Children) >= 2 && gen.isStringExpr(node.Children[0]) && gen.isStringExpr(node.Children[1]) {
			// String concatenation: + on two strings builds a new string.
			gen.stringMethods["concat"] = true
			left, right := node.Children[0], node.Children[1]
			if !gen.valueAllocatesString(left) && !gen.valueAllocatesString(right) {
				gen.output.WriteString("ahoy_string_concat(")
				gen.generateNode(left)
				gen.output.WriteString(", ")
				gen.generateNode(right)
				gen.output.WriteString(")")
				return
			}
			// A fresh operand (nested concat, sprintf, fresh method) would
			// leak: the outer concat copies it and nobody frees it.
			// Stash fresh operands, concat, then free the stashes.
			// Statement-expression keeps this usable in expression position.
			leftText := gen.bufferedNode(left)
			rightText := gen.bufferedNode(right)
			leftUse, rightUse := leftText, rightText
			gen.output.WriteString("({ ")
			if gen.valueAllocatesString(left) {
				leftUse = fmt.Sprintf("_ct_%d", gen.varCounter)
				gen.varCounter++
				gen.output.WriteString("char* ")
				gen.output.WriteString(leftUse)
				gen.output.WriteString(" = ")
				gen.output.WriteString(leftText)
				gen.output.WriteString("; ")
			}
			if gen.valueAllocatesString(right) {
				rightUse = fmt.Sprintf("_ct_%d", gen.varCounter)
				gen.varCounter++
				gen.output.WriteString("char* ")
				gen.output.WriteString(rightUse)
				gen.output.WriteString(" = ")
				gen.output.WriteString(rightText)
				gen.output.WriteString("; ")
			}
			gen.output.WriteString("char* _cr = ahoy_string_concat(")
			gen.output.WriteString(leftUse)
			gen.output.WriteString(", ")
			gen.output.WriteString(rightUse)
			gen.output.WriteString("); ")
			if gen.valueAllocatesString(left) {
				gen.output.WriteString("free(")
				gen.output.WriteString(leftUse)
				gen.output.WriteString("); ")
			}
			if gen.valueAllocatesString(right) {
				gen.output.WriteString("free(")
				gen.output.WriteString(rightUse)
				gen.output.WriteString("); ")
			}
			gen.output.WriteString("_cr; })")
			return
		}
		gen.output.WriteString("(")
		gen.generateNode(node.Children[0])
		gen.output.WriteString(" + ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(")")
	case "pow", "**":
		// Use pow() function from math.h for exponentiation
		if !gen.includes["math.h"] {
			gen.includes["math.h"] = true
			gen.orderedIncludes = append(gen.orderedIncludes, "math.h")
		}
		gen.output.WriteString("pow(")
		gen.generateNode(node.Children[0])
		gen.output.WriteString(", ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(")")
	case "greater_than":
		gen.output.WriteString("(")
		gen.generateNode(node.Children[0])
		gen.output.WriteString(" > ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(")")
	case "lesser_than", "less_than":
		gen.output.WriteString("(")
		gen.generateNode(node.Children[0])
		gen.output.WriteString(" < ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(")")
	default:
		gen.output.WriteString("(")
		gen.generateNode(node.Children[0])
		gen.output.WriteString(" ")
		gen.output.WriteString(node.Value)
		gen.output.WriteString(" ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(")")
	}
}

// generateIsList emits the comma sugar `x is A, B` (or `x is not A, B`), which
// compares one subject against every listed value. The subject is evaluated
// once: when it contains calls (or anything that lowers to a call, such as
// f-strings and %-unique lookups) it is bound to a temporary inside a GNU
// statement expression; plain names and paths are inlined so the construct
// stays usable in file-scope initialisers too. Strings reuse the `is` rules and
// compare with strcmp per value.
func (gen *CodeGenerator) generateIsList(node *ahoy.ASTNode) {
	if len(node.Children) < 2 {
		return
	}
	subject := node.Children[0]
	values := node.Children[1:]
	negated := node.Value == "is not"

	if negated {
		gen.output.WriteString("!(")
	}

	if gen.isSimpleExpression(subject) {
		gen.generateIsListComparisons(subject, values, "")
	} else {
		gen.output.WriteString("({ __typeof__(")
		gen.generateNode(subject)
		gen.output.WriteString(") __ahoy_is_subject = ")
		gen.generateNode(subject)
		gen.output.WriteString("; ")
		gen.generateIsListComparisons(subject, values, "__ahoy_is_subject")
		gen.output.WriteString("; })")
	}

	if negated {
		gen.output.WriteString(")")
	}
}

// generateIsListComparisons writes `(subj == A || subj == B || ...)`. When
// subjectOverride is set the subject is read from that C expression instead of
// being generated again.
func (gen *CodeGenerator) generateIsListComparisons(subject *ahoy.ASTNode, values []*ahoy.ASTNode, subjectOverride string) {
	gen.output.WriteString("(")
	for i, value := range values {
		if i > 0 {
			gen.output.WriteString(" || ")
		}
		gen.output.WriteString("(")
		if gen.isStringExpr(subject) && gen.isStringExpr(value) {
			gen.output.WriteString("strcmp(")
			gen.emitIsListSubject(subject, subjectOverride)
			gen.output.WriteString(", ")
			gen.generateNode(value)
			gen.output.WriteString(") == 0)")
		} else {
			gen.emitIsListSubject(subject, subjectOverride)
			gen.output.WriteString(" == ")
			gen.generateNode(value)
			gen.output.WriteString(")")
		}
	}
	gen.output.WriteString(")")
}

func (gen *CodeGenerator) emitIsListSubject(subject *ahoy.ASTNode, override string) {
	if override != "" {
		gen.output.WriteString(override)
		return
	}
	gen.generateNode(subject)
}

// isSimpleExpression reports whether generating an expression twice cannot
// change its value: no calls anywhere inside it.
func (gen *CodeGenerator) isSimpleExpression(node *ahoy.ASTNode) bool {
	if node == nil {
		return true
	}
	switch node.Type {
	case ahoy.NODE_CALL, ahoy.NODE_METHOD_CALL, ahoy.NODE_UNIQUE_ACCESS, ahoy.NODE_F_STRING:
		return false
	}
	for _, child := range node.Children {
		if !gen.isSimpleExpression(child) {
			return false
		}
	}
	return true
}

// evaluateConstantExpression evaluates a constant expression at compile-time
// Returns the computed value and true if successful, or 0 and false if not computable
func (gen *CodeGenerator) evaluateConstantExpression(node *ahoy.ASTNode) (interface{}, bool) {
	if node == nil {
		return nil, false
	}

	switch node.Type {
	case ahoy.NODE_NUMBER:
		// Parse number - check if it's float or int
		if strings.Contains(node.Value, ".") {
			if val, err := strconv.ParseFloat(node.Value, 64); err == nil {
				return val, true
			}
		} else {
			if val, err := strconv.ParseInt(node.Value, 10, 64); err == nil {
				return val, true
			}
		}
		return nil, false

	case ahoy.NODE_IDENTIFIER:
		// Look up constant value
		if val, exists := gen.constantValues[node.Value]; exists {
			return val, true
		}
		return nil, false

	case ahoy.NODE_BINARY_OP:
		// Evaluate binary operations
		if len(node.Children) < 2 {
			return nil, false
		}

		left, leftOk := gen.evaluateConstantExpression(node.Children[0])
		if !leftOk {
			return nil, false
		}

		right, rightOk := gen.evaluateConstantExpression(node.Children[1])
		if !rightOk {
			return nil, false
		}

		// Convert to float64 for operations, track if both were integers
		var leftFloat, rightFloat float64
		leftIsInt := false
		rightIsInt := false

		switch v := left.(type) {
		case int64:
			leftFloat = float64(v)
			leftIsInt = true
		case float64:
			leftFloat = v
		default:
			return nil, false
		}

		switch v := right.(type) {
		case int64:
			rightFloat = float64(v)
			rightIsInt = true
		case float64:
			rightFloat = v
		default:
			return nil, false
		}

		var result float64
		switch node.Value {
		case "+":
			result = leftFloat + rightFloat
		case "-":
			result = leftFloat - rightFloat
		case "*":
			result = leftFloat * rightFloat
		case "/":
			if rightFloat == 0 {
				return nil, false
			}
			result = leftFloat / rightFloat
		case "%":
			// Modulo only works on integers
			if !leftIsInt || !rightIsInt {
				return nil, false
			}
			result = float64(int64(leftFloat) % int64(rightFloat))
		default:
			return nil, false
		}

		// If both operands were integers and operation preserves integers, return int
		if leftIsInt && rightIsInt && (node.Value == "+" || node.Value == "-" || node.Value == "*" || node.Value == "%") {
			return int64(result), true
		}

		return result, true

	case ahoy.NODE_UNARY_OP:
		if len(node.Children) < 1 {
			return nil, false
		}

		operand, ok := gen.evaluateConstantExpression(node.Children[0])
		if !ok {
			return nil, false
		}

		switch node.Value {
		case "-":
			switch v := operand.(type) {
			case int64:
				return -v, true
			case float64:
				return -v, true
			}
		case "+":
			return operand, true
		}
		return nil, false

	default:
		return nil, false
	}
}

func (gen *CodeGenerator) generateConstant(node *ahoy.ASTNode) {
	constName := node.Value

	// Check if constant already declared
	if gen.declaredConstants[constName] {
		fmt.Printf("\n❌ Error at line %d: Cannot redeclare constant '%s'\n", node.Line, constName)
		fmt.Printf("   Constants cannot be reassigned or redeclared.\n")
		fmt.Printf("   '%s' was already declared earlier in the code.\n\n", constName)
		gen.hasError = true
		return
	}

	// Mark constant as declared
	gen.declaredConstants[constName] = true

	// Try to evaluate the constant expression at compile-time
	if len(node.Children) > 0 {
		if value, ok := gen.evaluateConstantExpression(node.Children[0]); ok {
			gen.constantValues[constName] = value
		}
	}

	// Determine the constant type - use explicit type if provided, otherwise infer
	var constType string
	var ahoyType string
	if node.DataType != "" {
		ahoyType = node.DataType
		constType = gen.mapType(node.DataType)
	} else {
		// Infer type from the value
		ahoyType = gen.inferType(node.Children[0])
		constType = gen.mapType(ahoyType)
	}

	// Update constant type for inference (may have been set in scan pass)
	gen.constants[constName] = ahoyType

	// Check if we can compute the constant value at compile-time
	computedValue, canCompute := gen.evaluateConstantExpression(node.Children[0])

	// Constants at global scope (not in a function) should go into constantDecls
	// Constants in main function that might be accessed by other functions should also be global
	// but need special handling if they're initialized with function calls
	if gen.currentFunction == "" {
		savedOutput := gen.output
		gen.output = strings.Builder{}

		// For hot reload: generate getter function instead of const global
		// Store the constant info for later generation
		gen.recordConstantGetterOrder(constName)
		gen.globalConstantGetters[constName] = ConstantInfo{
			Name:       constName,
			CType:      constType,
			AhoyType:   ahoyType,
			IsComputed: canCompute,
			Value:      computedValue,
		}

		// Capture the initialization expression
		if canCompute {
			gen.writeComputedValue(computedValue, ahoyType)
		} else {
			gen.generateNode(node.Children[0])
		}
		initExpr := gen.output.String()

		info := gen.globalConstantGetters[constName]
		info.InitValue = initExpr
		gen.globalConstantGetters[constName] = info

		gen.output = savedOutput
	} else if gen.currentFunction == "main" || gen.currentFunction == "ahoy_main" {
		// Constants in main: declare as static global (can be accessed by all functions)
		// If we can compute the value or it's simple, declare at global scope
		isSimpleValue := gen.isSimpleConstantValue(node.Children[0])

		if isSimpleValue || canCompute {
			// For hot reload: generate getter function instead of const global
			savedOutput := gen.output
			gen.output = strings.Builder{}

			gen.recordConstantGetterOrder(constName)
			gen.globalConstantGetters[constName] = ConstantInfo{
				Name:       constName,
				CType:      constType,
				AhoyType:   ahoyType,
				IsComputed: canCompute,
				Value:      computedValue,
			}

			// Capture the initialization expression
			if canCompute {
				gen.writeComputedValue(computedValue, ahoyType)
			} else {
				gen.generateNode(node.Children[0])
			}
			initExpr := gen.output.String()

			info := gen.globalConstantGetters[constName]
			info.InitValue = initExpr
			gen.globalConstantGetters[constName] = info

			gen.output = savedOutput
		} else {
			// Complex value (function call) - declare as static variable at global scope
			// and initialize it in main

			// Add global declaration (uninitialized)
			gen.constantDecls.WriteString("static ")
			gen.constantDecls.WriteString(constType)
			gen.constantDecls.WriteString(" ")
			gen.constantDecls.WriteString(constName)
			gen.constantDecls.WriteString(";\n")

			// Generate initialization in main
			gen.writeIndent()
			gen.output.WriteString(constName)
			gen.output.WriteString(" = ")
			gen.generateNode(node.Children[0])
			gen.output.WriteString(";\n")
		}
	} else {
		// Local constants in other functions
		gen.writeIndent()
		gen.output.WriteString("const ")
		gen.output.WriteString(constType)
		gen.output.WriteString(" ")
		gen.output.WriteString(constName)
		gen.output.WriteString(" = ")

		if canCompute {
			gen.writeComputedValue(computedValue, ahoyType)
		} else {
			gen.generateNode(node.Children[0])
		}
		gen.output.WriteString(";\n")
	}
}

// writeComputedValue writes a computed constant value to the output
func (gen *CodeGenerator) writeComputedValue(value interface{}, ahoyType string) {
	switch v := value.(type) {
	case int64:
		// If the target type is float, write as float
		if ahoyType == "float" || ahoyType == "double" || ahoyType == "f32" || ahoyType == "f64" {
			gen.output.WriteString(fmt.Sprintf("%g", float64(v)))
		} else {
			gen.output.WriteString(strconv.FormatInt(v, 10))
		}
	case float64:
		gen.output.WriteString(fmt.Sprintf("%g", v))
	default:
		// Fallback - shouldn't happen
		gen.output.WriteString(fmt.Sprintf("%v", v))
	}
}

// isSimpleConstantValue checks if a node represents a simple constant value
// that can be initialized at global scope (literals, not function calls)
func (gen *CodeGenerator) isSimpleConstantValue(node *ahoy.ASTNode) bool {
	if node == nil {
		return false
	}

	switch node.Type {
	case ahoy.NODE_NUMBER, ahoy.NODE_STRING, ahoy.NODE_CHAR, ahoy.NODE_BOOLEAN:
		return true
	case ahoy.NODE_BINARY_OP:
		// Binary operations on simple values are simple
		return gen.isSimpleConstantValue(node.Children[0]) && gen.isSimpleConstantValue(node.Children[1])
	case ahoy.NODE_UNARY_OP:
		return gen.isSimpleConstantValue(node.Children[0])
	case ahoy.NODE_IDENTIFIER:
		// References to other constants are simple
		_, isConst := gen.constants[node.Value]
		return isConst
	default:
		// Everything else (function calls, method calls, etc.) is not simple
		return false
	}
}

func (gen *CodeGenerator) generateMethodCall(node *ahoy.ASTNode) {
	object := node.Children[0]
	args := node.Children[1]
	methodName := node.Value

	// Handle dump_struct - returns type information as a string constant
	if methodName == "dump_struct" {
		objectType := gen.inferType(object)
		varName := ""
		if object.Type == ahoy.NODE_IDENTIFIER {
			varName = object.Value
		}

		// Generate a string literal describing the type
		structDesc := fmt.Sprintf("\"Type: %s", objectType)
		if structInfo, exists := gen.structs[objectType]; exists {
			structDesc += "\\nFields:"
			for _, field := range structInfo.Fields {
				structDesc += fmt.Sprintf("\\n  %s: %s", field.Name, field.Type)
			}
		} else if varName != "" {
			structDesc += fmt.Sprintf(" (variable: %s)", varName)
		}
		structDesc += "\""
		gen.output.WriteString(structDesc)
		return
	}

	// Check if this is a namespaced C function call (e.g., math.lerp)
	if object.Type == ahoy.NODE_IDENTIFIER {
		namespace := object.Value
		if funcMap, exists := gen.cNamespaces[namespace]; exists {
			// This is a namespaced C function call
			if cFuncName, found := funcMap[methodName]; found {
				// Generate the C function call
				gen.output.WriteString(cFuncName)
				gen.output.WriteString("(")
				for i, arg := range args.Children {
					if i > 0 {
						gen.output.WriteString(", ")
					}
					gen.generateNode(arg)
				}
				gen.output.WriteString(")")
				return
			}
		}
	}

	// Job pool methods: p.submit|fn, arg|, p.wait|job|, p.stop||
	if object.Type == ahoy.NODE_IDENTIFIER && gen.inferType(object) == "pool" {
		gen.useJobs = true
		switch methodName {
		case "submit":
			gen.output.WriteString("ahoy_jobs_submit(" + object.Value + ", ")
			argType := ""
			if len(args.Children) > 0 {
				argType = gen.emitTaskFn(args.Children[0])
			} else {
				gen.output.WriteString("(AhoyFn){0, 0}")
			}
			// The job holds the argument pointer until a worker runs it:
			// a heap local submitted here must survive its scope, like a
			// pushed value (the pool never frees it; it leaks, soundly).
			// Submitted closures carry their captures the same way — both
			// as the task (Children[0]) and as the data (Children[1]).
			for _, child := range args.Children {
				if child.Type == ahoy.NODE_IDENTIFIER {
					if gen.heapAllocatedVars[child.Value] {
						gen.escapingVars[child.Value] = true
					}
					gen.markLambdaCapturesOf(child.Value)
				} else if child.Type == ahoy.NODE_LAMBDA {
					gen.markLambdaCaptures(child)
				}
				// Transitive stored dependencies (selects, alias chains).
				gen.markValueDeps(child)
			}
			gen.output.WriteString(", (intptr_t)(")
			if len(args.Children) > 1 {
				gen.emitTaskArg(args.Children[1], argType)
			} else {
				gen.output.WriteString("0")
			}
			gen.output.WriteString("))")
			return
		case "wait":
			if len(args.Children) > 0 {
				arg := args.Children[0]
				if arg.Type == ahoy.NODE_IDENTIFIER {
					gen.output.WriteString(gen.jobResultExpr("ahoy_jobs_wait("+arg.Value+")", arg.Value))
				} else {
					gen.output.WriteString("((intptr_t)ahoy_jobs_wait(")
					gen.generateNode(arg)
					gen.output.WriteString("))")
				}
			}
			return
		case "drain":
			gen.emitJobHandler(args, "ahoy_jobs_drain("+object.Value+", ")
			return
		case "stop":
			gen.output.WriteString("ahoy_jobs_stop(" + object.Value + ")")
			return
		}
	}

	// Job methods: j.result|| reads a finished result without blocking,
	// j.done|| polls
	if object.Type == ahoy.NODE_IDENTIFIER && gen.inferType(object) == "job" {
		gen.useJobs = true
		switch methodName {
		case "result":
			gen.output.WriteString(gen.jobResultExpr("ahoy_jobs_result("+object.Value+")", object.Value))
			return
		case "done":
			gen.output.WriteString("ahoy_jobs_done(" + object.Value + ")")
			return
		}
	}

	// A struct field holding a function value: r.draw_sprite|args| calls
	// through the stored callback table.
	if object.Type == ahoy.NODE_IDENTIFIER {
		objType := strings.TrimPrefix(gen.inferType(object), "struct:")
		if info := gen.structs[objType]; info != nil {
			for _, f := range info.Fields {
				fieldType := f.AhoyType
				if fieldType == "" {
					fieldType = f.Type
				}
				if f.Name == methodName && ahoy.IsFunctionType(fieldType) {
					gen.generateStructCallbackCall(object, methodName, fieldType, args)
					return
				}
			}
		}
	}

	// User-defined methods: recv.m|args| dispatches to the mangled Type_method
	// function with the receiver prepended; Type.m|args| (the receiver names a
	// type) calls an associated function with no receiver. This sits after
	// callback fields and C namespaces so existing programs keep their exact
	// behavior; builtins below only see calls no user method claims.
	if mangled, withRecv, ok := gen.resolveUserMethod(node); ok {
		gen.output.WriteString(mangled)
		gen.output.WriteString("(")
		if withRecv {
			gen.generateNode(object)
			if len(args.Children) > 0 {
				gen.output.WriteString(", ")
			}
		}
		for i, arg := range args.Children {
			if i > 0 {
				gen.output.WriteString(", ")
			}
			gen.generateNode(arg)
		}
		gen.output.WriteString(")")
		return
	}

	// Handle map and filter with inline code generation
	if methodName == "map" || methodName == "filter" {
		if len(args.Children) > 0 && args.Children[0].Type == ahoy.NODE_LAMBDA {
			if methodName == "map" {
				gen.generateMapInline(object, args.Children[0])
			} else {
				gen.generateFilterInline(object, args.Children[0])
			}
			return
		}
	}

	// Infer the object type to determine correct method routing
	objectType := gen.inferType(object)

	// List of string-only methods (not ambiguous)
	stringOnlyMethods := []string{
		"upper", "lower", "replace", "contains",
		"camel_case", "snake_case", "pascal_case", "kebab_case",
		"match", "split", "count", "lpad", "rpad", "pad",
		"strip", "get_file",
	}

	// List of dictionary-only methods (not ambiguous)
	dictMethodsList := []string{
		"size", "clear", "has_all", "keys", "values",
		"stable_sort", "merge",
	}

	// Check if this is a string-only method
	isStringMethod := false
	for _, m := range stringOnlyMethods {
		if methodName == m {
			isStringMethod = true
			break
		}
	}

	// Check if this is a dictionary method
	isDictMethod := false
	for _, m := range dictMethodsList {
		if methodName == m {
			isDictMethod = true
			break
		}
	}

	// Support "len" as alias for "length"
	if methodName == "len" {
		methodName = "length"
	}

	// For "length" method, route based on object type
	if methodName == "length" {
		if objectType == "char*" || objectType == "string" {
			isStringMethod = true
		}
		// Otherwise it's an array method (default)
	}

	// For ambiguous methods (sort, has), route based on object type
	if methodName == "sort" || methodName == "has" || methodName == "reverse" {
		if objectType == "dict" || objectType == "HashMap*" {
			isDictMethod = true
			isStringMethod = false
		} else {
			// Default to array method
			isDictMethod = false
		}
	}

	if isStringMethod || (objectType == "char*" && methodName == "length") {
		// Track which string method is used
		gen.stringMethods[methodName] = true

		// split's result is indexed and iterated like any array, so it has to be
		// an AhoyArray*: the helper used to return char** while the type of the
		// expression was array, and the two were not interchangeable.
		if methodName == "split" {
			// The result is a real array, so the AhoyArray runtime has to be in
			// the output even when nothing else in the program uses arrays.
			gen.arrayImpls = true
			gen.output.WriteString("ahoy_string_split_array(")
			gen.generateNodeInternal(object, false)
			if len(args.Children) > 0 {
				gen.output.WriteString(", ")
				gen.generateNodeInternal(args.Children[0], false)
			}
			gen.output.WriteString(")")
			return
		}

		// Generate string method function call
		gen.output.WriteString("ahoy_string_")
		gen.output.WriteString(methodName)
		gen.output.WriteString("(")
		gen.generateNodeInternal(object, false)

		if len(args.Children) > 0 {
			gen.output.WriteString(", ")
			for i, arg := range args.Children {
				if i > 0 {
					gen.output.WriteString(", ")
				}
				gen.generateNodeInternal(arg, false)
			}
		}
		gen.output.WriteString(")")
	} else if isDictMethod || objectType == "dict" {
		// Track which dict method is used
		gen.dictMethods[methodName] = true

		// Generate dict method function call
		gen.output.WriteString("ahoy_dict_")
		gen.output.WriteString(methodName)
		gen.output.WriteString("(")
		// Cast generic/any parameters to HashMap*
		if object.Type == ahoy.NODE_IDENTIFIER {
			objType := gen.inferType(object)
			if objType == "generic" || objType == "any" {
				gen.output.WriteString("(HashMap*)")
			}
		}
		gen.generateNodeInternal(object, false)

		if len(args.Children) > 0 {
			gen.output.WriteString(", ")
			for i, arg := range args.Children {
				if i > 0 {
					gen.output.WriteString(", ")
				}
				gen.generateNodeInternal(arg, false)
			}
		}
		gen.output.WriteString(")")
	} else {
		// Track which array method is used
		gen.arrayMethods[methodName] = true

		// Special handling for push with multiple arguments - generate multiple calls
		if methodName == "push" && len(args.Children) > 1 {
			for i, arg := range args.Children {
				// Mark heap-allocated variables as escaping when pushed to arrays
				if arg.Type == ahoy.NODE_IDENTIFIER && gen.heapAllocatedVars[arg.Value] {
					gen.escapingVars[arg.Value] = true
				}
				// Pushed closures (by name or directly) carry their captures.
				if arg.Type == ahoy.NODE_IDENTIFIER {
					gen.markLambdaCapturesOf(arg.Value)
				} else if arg.Type == ahoy.NODE_LAMBDA {
					gen.markLambdaCaptures(arg)
				}
				// Transitive stored dependencies (selects, alias chains).
				gen.markValueDeps(arg)

				if i > 0 {
					gen.output.WriteString("; ")
				}
				gen.output.WriteString("ahoy_array_push(")
				gen.generateNodeInternal(object, false)
				gen.output.WriteString(", (intptr_t)")

				// Check if we're pushing a struct value (needs heap allocation)
				argType := gen.inferType(arg)
				needsHeapAlloc := false
				structType := ""

				// For struct literals
				if arg.Type == ahoy.NODE_OBJECT_LITERAL && arg.Value != "" {
					needsHeapAlloc = true
					structType = gen.mapType(arg.Value)
				} else if _, isStruct := gen.structs[argType]; isStruct {
					// For Ahoy-defined struct variables
					needsHeapAlloc = true
					structType = gen.mapType(argType)
				} else {
					// Check if it's a C struct type from imported headers
					cType := gen.mapType(argType)
					isCStruct := gen.cTypeDefinitions[cType] && !strings.HasSuffix(cType, "*") &&
						cType != "int" && cType != "float" && cType != "double" &&
						cType != "char" && cType != "bool" && cType != "void" &&
						cType != "char*" && cType != "intptr_t"
					if isCStruct {
						needsHeapAlloc = true
						structType = cType
					}
				}

				if needsHeapAlloc {
					gen.output.WriteString("({ ")
					gen.output.WriteString(structType)
					gen.output.WriteString("* __tmp = malloc(sizeof(")
					gen.output.WriteString(structType)
					gen.output.WriteString(")); *__tmp = ")
					gen.generateNodeInternal(arg, false)
					gen.output.WriteString("; __tmp; })")
				} else {
					gen.generateNodeInternal(arg, false)
				}

				valueType := gen.getValueType(arg)
				gen.output.WriteString(", ")
				gen.output.WriteString(gen.getAhoyTypeEnum(valueType))
				gen.output.WriteString(")")
			}
			return
		}

		// Generate array method function call
		gen.output.WriteString("ahoy_array_")
		gen.output.WriteString(methodName)
		gen.output.WriteString("(")
		// Cast generic/any parameters to AhoyArray*
		if object.Type == ahoy.NODE_IDENTIFIER {
			objType := gen.inferType(object)
			if objType == "generic" || objType == "any" {
				gen.output.WriteString("(AhoyArray*)")
			}
		}
		gen.generateNodeInternal(object, false)

		if len(args.Children) > 0 {
			gen.output.WriteString(", ")
			for i, arg := range args.Children {
				if i > 0 {
					gen.output.WriteString(", ")
				}
				// For array methods like push, cast to intptr_t
				if methodName == "push" || methodName == "has" || methodName == "fill" {
					// Mark heap-allocated variables as escaping when pushed to arrays.
					// fill stores the value N times: same ownership as push.
					if (methodName == "push" || methodName == "fill") && arg.Type == ahoy.NODE_IDENTIFIER && gen.heapAllocatedVars[arg.Value] {
						gen.escapingVars[arg.Value] = true
					}
					// Pushed closures (by name or directly) carry their captures.
					if (methodName == "push" || methodName == "fill") && arg.Type == ahoy.NODE_IDENTIFIER {
						gen.markLambdaCapturesOf(arg.Value)
					} else if (methodName == "push" || methodName == "fill") && arg.Type == ahoy.NODE_LAMBDA {
						gen.markLambdaCaptures(arg)
					}
					// Transitive stored dependencies (selects, alias chains).
					if methodName == "push" || methodName == "fill" {
						gen.markValueDeps(arg)
					}

					// A float goes in as a heap double: the array runtime reads
					// AHOY_TYPE_FLOAT elements as double* (same as dicts).
					argValueType := gen.inferType(arg)
					if methodName == "push" && (argValueType == "float" || argValueType == "double") {
						gen.output.WriteString("(intptr_t)({ double* __fp = malloc(sizeof(double)); *__fp = ")
						gen.generateNodeInternal(arg, false)
						gen.output.WriteString("; __fp; })")
					} else {
						gen.output.WriteString("(intptr_t)")

						// Check if we're pushing a struct value (needs heap allocation)
						if methodName == "push" {
							argType := gen.inferType(arg)
							needsHeapAlloc := false
							structType := ""

							// For struct literals
							if arg.Type == ahoy.NODE_OBJECT_LITERAL && arg.Value != "" {
								needsHeapAlloc = true
								structType = gen.mapType(arg.Value)
							} else if _, isStruct := gen.structs[argType]; isStruct {
								// For Ahoy-defined struct variables
								needsHeapAlloc = true
								structType = gen.mapType(argType)
							} else {
								// Check if it's a C struct type from imported headers
								cType := gen.mapType(argType)
								isCStruct := gen.cTypeDefinitions[cType] && !strings.HasSuffix(cType, "*") &&
									cType != "int" && cType != "float" && cType != "double" &&
									cType != "char" && cType != "bool" && cType != "void" &&
									cType != "char*" && cType != "intptr_t"
								if isCStruct {
									needsHeapAlloc = true
									structType = cType
								}
							}

							if needsHeapAlloc {
								gen.output.WriteString("({ ")
								gen.output.WriteString(structType)
								gen.output.WriteString("* __tmp = malloc(sizeof(")
								gen.output.WriteString(structType)
								gen.output.WriteString(")); *__tmp = ")
								gen.generateNodeInternal(arg, false)
								gen.output.WriteString("; __tmp; })")
							} else {
								gen.generateNodeInternal(arg, false)
							}
						} else {
							gen.generateNodeInternal(arg, false)
						}
					}
				} else {
					gen.generateNodeInternal(arg, false)
				}
				// For push and fill, also pass the type
				if methodName == "push" && i == 0 {
					valueType := gen.getValueType(arg)
					gen.output.WriteString(", ")
					gen.output.WriteString(gen.getAhoyTypeEnum(valueType))
				}
				if methodName == "fill" && i == 0 {
					valueType := gen.getValueType(arg)
					gen.output.WriteString(", ")
					gen.output.WriteString(gen.getAhoyTypeEnum(valueType))
				}
			}
		}
		gen.output.WriteString(")")
	}
}

// generateStructCallbackCall calls a function-typed struct field:
// ((ret(*)(void*,params))obj.field.fn)(obj.field.env, args...).
func (gen *CodeGenerator) generateStructCallbackCall(object *ahoy.ASTNode, fieldName, funcType string, args *ahoy.ASTNode) {
	params, ret, _ := ahoy.ParseFunctionType(funcType)
	paramList := "void*"
	for _, p := range params {
		paramList += ", " + gen.mapType(p)
	}
	obj := object.Value
	gen.output.WriteString("((")
	gen.output.WriteString(gen.mapType(ret))
	gen.output.WriteString("(*)(")
	gen.output.WriteString(paramList)
	gen.output.WriteString("))")
	gen.output.WriteString(obj)
	gen.output.WriteString(".")
	gen.output.WriteString(fieldName)
	gen.output.WriteString(".fn)(")
	gen.output.WriteString(obj)
	gen.output.WriteString(".")
	gen.output.WriteString(fieldName)
	gen.output.WriteString(".env")
	for _, arg := range args.Children {
		gen.output.WriteString(", ")
		gen.generateNode(arg)
	}
	gen.output.WriteString(")")
}

// emitCCallbackArg writes a raw C function pointer for an argument that lands
// in an imported function's callback parameter. It reports false when the
// argument is not a function value, so the normal argument path handles it.
//
// Only `&name` and non-capturing values can be handed to C: a capturing
// closure's environment has nowhere to go in a C callback signature.
func (gen *CodeGenerator) emitCCallbackArg(arg *ahoy.ASTNode) bool {
	// &name - a reference to a named Ahoy function
	if arg.Type == ahoy.NODE_UNARY_OP && arg.Value == "&" && len(arg.Children) == 1 &&
		arg.Children[0].Type == ahoy.NODE_IDENTIFIER {
		name := arg.Children[0].Value
		if gen.userFunctions[name] {
			if name == "main" {
				name = "ahoy_main"
			}
			gen.output.WriteString(name)
			return true
		}
	}

	// A variable holding a C callback typedef is already a raw pointer.
	if arg.Type == ahoy.NODE_IDENTIFIER {
		if _, ok := gen.cFuncPtrTypedefVar(arg.Value); ok {
			gen.output.WriteString(arg.Value)
			return true
		}
	}

	// A function-typed variable: pass its code pointer. If it captures, the
	// environment is silently dropped, so prefer &name for C callbacks.
	if arg.Type == ahoy.NODE_IDENTIFIER {
		if _, ok := gen.functionValueType(arg.Value); ok {
			gen.output.WriteString(arg.Value + ".fn")
			return true
		}
	}

	return false
}

// cFuncPtrTypedefVar returns the C callback typedef a variable holds, if any.
// Such a variable is a raw C function pointer, not an AhoyFn.
func (gen *CodeGenerator) cFuncPtrTypedefVar(name string) (*ahoy.CFuncPtrTypedef, bool) {
	t := gen.functionVars[name]
	if t == "" {
		t = gen.variables[name]
	}
	if t == "" {
		return nil, false
	}
	td := gen.cFuncPtrTypedefs[t]
	return td, td != nil
}

// generateCallbackTypedefCall calls a variable that holds a C function pointer:
// the C call is direct because the value is already a raw pointer.
func (gen *CodeGenerator) generateCallbackTypedefCall(node *ahoy.ASTNode, td *ahoy.CFuncPtrTypedef) {
	gen.output.WriteString(node.Value)
	gen.output.WriteString("(")
	for i, arg := range node.Children {
		if i > 0 {
			gen.output.WriteString(", ")
		}
		gen.generateNode(arg)
	}
	gen.output.WriteString(")")
}

// functionValueType returns the fn(...) type of a variable, if it holds one.
func (gen *CodeGenerator) functionValueType(name string) (string, bool) {
	if t, ok := gen.functionVars[name]; ok && ahoy.IsFunctionType(t) {
		return t, true
	}
	if t, ok := gen.variables[name]; ok && ahoy.IsFunctionType(t) {
		return t, true
	}
	return "", false
}

// generateIndirectCall calls through a function value:
// ((ret(*)(void*,params))f.fn)(f.env, args...).
func (gen *CodeGenerator) generateIndirectCall(node *ahoy.ASTNode, funcType string) {
	params, ret, _ := ahoy.ParseFunctionType(funcType)
	paramList := "void*"
	for _, p := range params {
		paramList += ", " + gen.mapType(p)
	}
	gen.output.WriteString("((")
	gen.output.WriteString(gen.mapType(ret))
	gen.output.WriteString("(*)(")
	gen.output.WriteString(paramList)
	gen.output.WriteString("))")
	gen.output.WriteString(node.Value)
	gen.output.WriteString(".fn)(")
	gen.output.WriteString(node.Value)
	gen.output.WriteString(".env")
	for _, arg := range node.Children {
		gen.output.WriteString(", ")
		gen.generateNode(arg)
	}
	gen.output.WriteString(")")
}

// emitFunctionReference writes an AhoyFn for a named function. A thunk adapts
// the function to the (void* env, params...) convention every function value
// uses, so a call through a value has exactly one shape.
func (gen *CodeGenerator) emitFunctionReference(name string) {
	thunk := "__ahoy_ref_" + name
	if !gen.funcRefThunks[name] {
		gen.funcRefThunks[name] = true

		retC := "void"
		retTypes := gen.functionReturnTypes[name]
		if len(retTypes) == 1 {
			retC = gen.mapType(retTypes[0])
		} else if len(retTypes) > 1 {
			fmt.Fprintf(os.Stderr, "error: '%s' returns multiple values and cannot be used as a function value\n", name)
			gen.hasError = true
		}

		var sig, args []string
		for i, pt := range gen.functionParamTypes[name] {
			ct := "intptr_t"
			if pt != "" && pt != "any" && pt != "generic" {
				ct = gen.mapType(pt)
			}
			sig = append(sig, fmt.Sprintf("%s p%d", ct, i))
			args = append(args, fmt.Sprintf("p%d", i))
		}

		cName := name
		if name == "main" {
			cName = "ahoy_main"
		}
		call := fmt.Sprintf("%s(%s)", cName, strings.Join(args, ", "))
		body := call + ";"
		if retC != "void" {
			body = "return " + call + ";"
		}
		// A zero-parameter function must not leave a trailing comma in the
		// thunk's parameter list (`void* __env, )` is not a C parameter list).
		thunkParams := "void* __env"
		if len(sig) > 0 {
			thunkParams += ", " + strings.Join(sig, ", ")
		}
		gen.closureDecls.WriteString("static ")
		gen.closureDecls.WriteString(retC)
		gen.closureDecls.WriteString(" ")
		gen.closureDecls.WriteString(thunk)
		gen.closureDecls.WriteString("(")
		gen.closureDecls.WriteString(thunkParams)
		gen.closureDecls.WriteString(") { (void)__env; ")
		gen.closureDecls.WriteString(body)
		gen.closureDecls.WriteString(" }\n")
	}
	gen.output.WriteString("((AhoyFn){ (void(*)(void))")
	gen.output.WriteString(thunk)
	gen.output.WriteString(", NULL })")
}

// generateLambdaValue emits a lambda as an AhoyFn. The lambda becomes a static
// C function with the (void* env, params...) convention; captured locals are
// copied into a heap environment it reads through env.
func (gen *CodeGenerator) generateLambdaValue(node *ahoy.ASTNode) {
	paramCount := 0
	for _, ch := range node.Value {
		if ch >= '0' && ch <= '9' {
			paramCount = paramCount*10 + int(ch-'0')
		}
	}
	if paramCount > len(node.Children) {
		paramCount = len(node.Children)
	}
	params := node.Children[:paramCount]
	body := node.Children[paramCount]

	// The declared fn(...) type of the variable being initialised supplies the
	// parameter and return types when the lambda itself is unannotated.
	var declaredParams []string
	declaredRet := ""
	if gen.pendingFuncType != "" {
		if dp, dr, ok := ahoy.ParseFunctionType(gen.pendingFuncType); ok {
			declaredParams = dp
			declaredRet = dr
		}
	}

	paramNames := map[string]bool{}
	var sig []string
	for i, param := range params {
		paramNames[param.Value] = true
		pt := param.DataType
		if pt == "" && i < len(declaredParams) {
			pt = declaredParams[i]
		}
		if pt == "" {
			pt = "int"
		}
		sig = append(sig, fmt.Sprintf("%s p%d", gen.mapType(pt), i))
	}

	// Which enclosing locals does the body use?
	var captures []string
	seen := map[string]bool{}
	var collect func(n *ahoy.ASTNode)
	collect = func(n *ahoy.ASTNode) {
		if n == nil {
			return
		}
		if n.Type == ahoy.NODE_IDENTIFIER && !paramNames[n.Value] && !seen[n.Value] {
			if !gen.userFunctions[n.Value] {
				if _, ok := gen.functionVars[n.Value]; ok {
					seen[n.Value] = true
					captures = append(captures, n.Value)
				} else if _, ok := gen.variables[n.Value]; ok {
					seen[n.Value] = true
					captures = append(captures, n.Value)
				}
			}
		}
		// A container access carries the variable it reads in Value rather than
		// in a child identifier (xs[0], d<"k">), so those have to be collected
		// here too or a closure over an array/dict names a local that does not
		// exist inside the thunk.
		if (n.Type == ahoy.NODE_ARRAY_ACCESS || n.Type == ahoy.NODE_DICT_ACCESS) &&
			!paramNames[n.Value] && !seen[n.Value] && !gen.userFunctions[n.Value] {
			if _, ok := gen.functionVars[n.Value]; ok {
				seen[n.Value] = true
				captures = append(captures, n.Value)
			} else if _, ok := gen.variables[n.Value]; ok {
				seen[n.Value] = true
				captures = append(captures, n.Value)
			}
		}
		for _, c := range n.Children {
			collect(c)
		}
	}
	collect(body)

	id := gen.varCounter
	gen.varCounter++
	thunk := fmt.Sprintf("__ahoy_lambda_%d", id)

	// The declared type of the field/variable being initialised wins over the
	// inferred one: the thunk is called through the declared function pointer,
	// so a thunk that returns double where the type says int is undefined
	// behaviour at the call site (the double bits came back as a garbage int).
	retType := gen.inferType(body)
	if declaredRet != "" && declaredRet != "unknown" {
		retType = declaredRet
	}
	if retType == "" || retType == "unknown" {
		retType = "int"
	}
	retC := gen.mapType(retType)

	envName := ""
	if len(captures) > 0 {
		envName = fmt.Sprintf("__ahoy_env_%d", id)
		gen.closureDecls.WriteString("typedef struct {\n")
		for _, c := range captures {
			t := gen.functionVars[c]
			if t == "" {
				t = gen.variables[c]
			}
			gen.closureDecls.WriteString("    ")
			gen.closureDecls.WriteString(gen.mapType(t))
			gen.closureDecls.WriteString(" ")
			gen.closureDecls.WriteString(c)
			gen.closureDecls.WriteString(";\n")
		}
		gen.closureDecls.WriteString("} ")
		gen.closureDecls.WriteString(envName)
		gen.closureDecls.WriteString(";\n")
	}

	// Emit the body with captured names redirected into the environment. The
	// output builder is swapped for a fresh one while the body is generated.
	savedOutput := gen.output
	savedIndent := gen.indent
	savedCaptures := gen.closureCaptures
	gen.output = strings.Builder{}
	gen.indent = 0
	gen.closureCaptures = map[string]string{}
	// The thunk's C parameters are p0, p1, ...; the body uses the source names.
	for i, param := range params {
		gen.closureCaptures[param.Value] = fmt.Sprintf("p%d", i)
	}
	for _, c := range captures {
		gen.closureCaptures[c] = fmt.Sprintf("((%s*)__env)->%s", envName, c)
	}
	if retC == "void" {
		gen.generateNodeInternal(body, false)
	} else {
		// Cast to the thunk's return type so a body that infers differently
		// (a dict read is double, the declared return is int) still returns
		// the value the caller's function pointer expects.
		gen.output.WriteString("return (")
		gen.output.WriteString(retC)
		gen.output.WriteString(")(")
		gen.generateNodeInternal(body, false)
		gen.output.WriteString(")")
	}
	bodyCode := gen.output.String()
	gen.output = savedOutput
	gen.indent = savedIndent
	gen.closureCaptures = savedCaptures

	if envName == "" {
		gen.closureDecls.WriteString("static ")
		gen.closureDecls.WriteString(retC)
		gen.closureDecls.WriteString(" ")
		gen.closureDecls.WriteString(thunk)
		gen.closureDecls.WriteString("(void* __env, ")
		gen.closureDecls.WriteString(strings.Join(sig, ", "))
		gen.closureDecls.WriteString(") { (void)__env; ")
		gen.closureDecls.WriteString(bodyCode)
		gen.closureDecls.WriteString("; }\n")
		gen.output.WriteString("((AhoyFn){ (void(*)(void))")
		gen.output.WriteString(thunk)
		gen.output.WriteString(", NULL })")
		return
	}

	gen.closureDecls.WriteString("static ")
	gen.closureDecls.WriteString(retC)
	gen.closureDecls.WriteString(" ")
	gen.closureDecls.WriteString(thunk)
	gen.closureDecls.WriteString("(void* __env, ")
	gen.closureDecls.WriteString(strings.Join(sig, ", "))
	gen.closureDecls.WriteString(") { ")
	gen.closureDecls.WriteString(bodyCode)
	gen.closureDecls.WriteString("; }\n")
	var alloc strings.Builder
	fmt.Fprintf(&alloc, "({ %s* __e = (%s*)malloc(sizeof(%s)); ", envName, envName, envName)
	for _, c := range captures {
		fmt.Fprintf(&alloc, "__e->%s = %s; ", c, gen.nameExpr(c))
	}
	fmt.Fprintf(&alloc, "(AhoyFn){ (void(*)(void))%s, __e }; })", thunk)
	gen.output.WriteString(alloc.String())
}

func (gen *CodeGenerator) generateUnaryOp(node *ahoy.ASTNode) {
	switch node.Value {
	case "not":
		gen.output.WriteString("!")
	case "bnot":
		gen.output.WriteString("~")
	case "^":
		// Pointer dereference - convert ^ to *
		gen.output.WriteString("*")
	case "&":
		// A value stored into a C callback typedef variable is a raw C
		// function pointer, so the reference is the function's C name.
		if gen.pendingCType != "" && len(node.Children) == 1 && node.Children[0].Type == ahoy.NODE_IDENTIFIER &&
			gen.userFunctions[node.Children[0].Value] {
			name := node.Children[0].Value
			if name == "main" {
				name = "ahoy_main"
			}
			gen.output.WriteString(name)
			return
		}
		// Address-of a variable, or a reference to a named function.
		if len(node.Children) == 1 && node.Children[0].Type == ahoy.NODE_IDENTIFIER &&
			gen.userFunctions[node.Children[0].Value] {
			gen.emitFunctionReference(node.Children[0].Value)
			return
		}
		gen.output.WriteString("&")
	default:
		gen.output.WriteString(node.Value)
	}
	gen.generateNode(node.Children[0])
}

func (gen *CodeGenerator) generateTernary(node *ahoy.ASTNode) {
	// C ternary: condition ? true_expr : false_expr
	gen.output.WriteString("(")
	gen.generateTestCondition(node.Children[0]) // condition
	gen.output.WriteString(" ? ")
	gen.generateNode(node.Children[1]) // true branch
	gen.output.WriteString(" : ")
	gen.generateNode(node.Children[2]) // false branch
	gen.output.WriteString(")")
}

func (gen *CodeGenerator) generateArrayLiteral(node *ahoy.ASTNode) {
	gen.arrayImpls = true

	// Create array with initial capacity
	arrName := fmt.Sprintf("arr_%d", gen.varCounter)
	gen.varCounter++

	// Check if we have an explicit type from context
	var explicitElementType string
	if gen.currentTypeContext != "" && strings.HasPrefix(gen.currentTypeContext, "array[") {
		explicitElementType = strings.TrimSuffix(strings.TrimPrefix(gen.currentTypeContext, "array["), "]")
	}

	// Determine if this is a typed array (only if explicitly annotated)
	isTyped := explicitElementType != ""
	var elementType string
	if explicitElementType != "" {
		elementType = explicitElementType
	}

	// Use simple C array initialization
	gen.output.WriteString("({ ")
	gen.output.WriteString("AhoyArray* ")
	gen.output.WriteString(arrName)
	gen.output.WriteString(" = malloc(sizeof(AhoyArray)); ")
	gen.output.WriteString(arrName)
	gen.output.WriteString("->length = ")
	gen.output.WriteString(strconv.Itoa(len(node.Children)))
	gen.output.WriteString("; ")
	gen.output.WriteString(arrName)
	gen.output.WriteString("->capacity = ")
	gen.output.WriteString(strconv.Itoa(len(node.Children)))
	gen.output.WriteString("; ")
	gen.output.WriteString(arrName)
	gen.output.WriteString("->data = malloc(")
	gen.output.WriteString(strconv.Itoa(len(node.Children)))
	gen.output.WriteString(" * sizeof(intptr_t)); ")
	gen.output.WriteString(arrName)
	gen.output.WriteString("->types = malloc(")
	gen.output.WriteString(strconv.Itoa(len(node.Children)))
	gen.output.WriteString(" * sizeof(AhoyValueType)); ")

	// Set typed/mixed flag - only typed if explicitly annotated
	if isTyped {
		gen.output.WriteString(arrName)
		gen.output.WriteString("->is_typed = 1; ")
		gen.output.WriteString(arrName)
		gen.output.WriteString("->element_type = ")
		gen.output.WriteString(gen.getAhoyTypeEnum(elementType))
		gen.output.WriteString("; ")
	} else {
		gen.output.WriteString(arrName)
		gen.output.WriteString("->is_typed = 0; ")
	}

	// Check if array element type is a C struct or Ahoy struct (for typed arrays)
	var elemCType string
	var isElemStruct bool
	if isTyped && elementType != "" {
		elemCType = gen.mapType(elementType)
		isCStruct := gen.cTypeDefinitions[elemCType] && !strings.HasSuffix(elemCType, "*") &&
			elemCType != "int" && elemCType != "double" && elemCType != "char*" && elemCType != "bool"
		isAhoyStruct := false
		if !isCStruct {
			if _, exists := gen.structs[elementType]; exists {
				isAhoyStruct = true
			} else if _, exists := gen.structs[elemCType]; exists {
				isAhoyStruct = true
			}
		}
		isElemStruct = isCStruct || isAhoyStruct
	}

	// Add elements - cast to intptr_t for pointer safety and track types
	for i, child := range node.Children {
		valueType := gen.getValueType(child)
		gen.output.WriteString(arrName)
		gen.output.WriteString("->types[")
		gen.output.WriteString(strconv.Itoa(i))
		gen.output.WriteString("] = ")
		gen.output.WriteString(gen.getAhoyTypeEnum(valueType))
		gen.output.WriteString("; ")

		// Special handling for floats - need to allocate heap memory
		if valueType == "float" || valueType == "double" {
			gen.output.WriteString(arrName)
			gen.output.WriteString("->data[")
			gen.output.WriteString(strconv.Itoa(i))
			gen.output.WriteString("] = (intptr_t)({ double* __float_ptr_")
			gen.output.WriteString(strconv.Itoa(gen.varCounter))
			gen.output.WriteString(" = malloc(sizeof(double)); *__float_ptr_")
			gen.output.WriteString(strconv.Itoa(gen.varCounter))
			gen.output.WriteString(" = ")
			gen.varCounter++
			gen.generateNode(child)
			gen.output.WriteString("; __float_ptr_")
			gen.output.WriteString(strconv.Itoa(gen.varCounter - 1))
			gen.output.WriteString("; }); ")
		} else if isElemStruct {
			// For structs (C or Ahoy), determine actual C type from element
			actualCType := elemCType
			if child.Type == ahoy.NODE_IDENTIFIER {
				// Look up the variable's actual declared type
				if varType, exists := gen.variables[child.Value]; exists {
					actualCType = gen.mapType(varType)
				} else if varType, exists := gen.functionVars[child.Value]; exists {
					actualCType = gen.mapType(varType)
				}
			}

			// For structs (C or Ahoy), allocate heap memory and copy the struct
			gen.output.WriteString(arrName)
			gen.output.WriteString("->data[")
			gen.output.WriteString(strconv.Itoa(i))
			gen.output.WriteString("] = (intptr_t)({ ")
			gen.output.WriteString(actualCType)
			gen.output.WriteString("* __struct_ptr_")
			gen.output.WriteString(strconv.Itoa(gen.varCounter))
			gen.output.WriteString(" = malloc(sizeof(")
			gen.output.WriteString(actualCType)
			gen.output.WriteString(")); *__struct_ptr_")
			gen.output.WriteString(strconv.Itoa(gen.varCounter))
			gen.output.WriteString(" = ")
			gen.varCounter++
			gen.generateNode(child)
			gen.output.WriteString("; __struct_ptr_")
			gen.output.WriteString(strconv.Itoa(gen.varCounter - 1))
			gen.output.WriteString("; }); ")
		} else {
			// Check if this element is a struct (even in untyped arrays)
			isChildStruct := false
			childCType := ""
			if child.Type == ahoy.NODE_IDENTIFIER {
				if varType, exists := gen.variables[child.Value]; exists {
					childCType = gen.mapType(varType)
					// Check if it's a known struct
					if _, isStruct := gen.structs[varType]; isStruct {
						isChildStruct = true
					} else if _, isStruct := gen.structs[childCType]; isStruct {
						isChildStruct = true
					}
				} else if varType, exists := gen.functionVars[child.Value]; exists {
					childCType = gen.mapType(varType)
					if _, isStruct := gen.structs[varType]; isStruct {
						isChildStruct = true
					} else if _, isStruct := gen.structs[childCType]; isStruct {
						isChildStruct = true
					}
				}
			} else if child.Type == ahoy.NODE_OBJECT_LITERAL && child.Value != "" {
				// Object literal with type name
				childCType = gen.mapType(child.Value)
				if _, isStruct := gen.structs[child.Value]; isStruct {
					isChildStruct = true
				}
			}

			if isChildStruct && childCType != "" {
				// Struct in untyped array - need to allocate heap memory
				gen.output.WriteString(arrName)
				gen.output.WriteString("->data[")
				gen.output.WriteString(strconv.Itoa(i))
				gen.output.WriteString("] = (intptr_t)({ ")
				gen.output.WriteString(childCType)
				gen.output.WriteString("* __struct_ptr_")
				gen.output.WriteString(strconv.Itoa(gen.varCounter))
				gen.output.WriteString(" = malloc(sizeof(")
				gen.output.WriteString(childCType)
				gen.output.WriteString(")); *__struct_ptr_")
				gen.output.WriteString(strconv.Itoa(gen.varCounter))
				gen.output.WriteString(" = ")
				gen.varCounter++
				gen.generateNode(child)
				gen.output.WriteString("; __struct_ptr_")
				gen.output.WriteString(strconv.Itoa(gen.varCounter - 1))
				gen.output.WriteString("; }); ")
			} else {
				gen.output.WriteString(arrName)
				gen.output.WriteString("->data[")
				gen.output.WriteString(strconv.Itoa(i))
				gen.output.WriteString("] = (intptr_t)")
				gen.generateNode(child)
				gen.output.WriteString("; ")
			}
		}
	}

	gen.output.WriteString(arrName)
	gen.output.WriteString("; })")
}

// nameExpr returns the C expression for a variable reference. Accesses that
// name their object (xs[0], d<"k">, d<"k">: v) emit that object directly
// instead of generating the identifier node, so they must ask here: inside a
// closure thunk a captured local lives in the environment, and the raw name
// would not compile.
func (gen *CodeGenerator) nameExpr(name string) string {
	if expr, isCapture := gen.closureCaptures[name]; isCapture {
		return expr
	}
	return name
}

func (gen *CodeGenerator) generateArrayAccess(node *ahoy.ASTNode) {
	// Check if this is a chained array access (2D array): grid[r][c]
	// In this case, Value is empty and Children[0] is the inner array access, Children[1] is the outer index
	if node.Value == "" && len(node.Children) == 2 {
		gen.generateChainedArrayAccess(node)
		return
	}

	arrayName := node.Value
	objExpr := gen.nameExpr(arrayName)

	// Check if the variable type is intptr_t, void*, generic, or any (might need casting to AhoyArray*)
	needsArrayCast := false
	if varType, exists := gen.variables[arrayName]; exists {
		if varType == "intptr_t" || varType == "void*" || varType == "generic" || varType == "any" {
			needsArrayCast = true
		}
	}
	if varType, exists := gen.functionVars[arrayName]; exists {
		if varType == "intptr_t" || varType == "void*" || varType == "generic" || varType == "any" {
			needsArrayCast = true
		}
	}

	// If bounds checking is enabled and not skipped (lvalue context handled separately)
	if gen.enableBoundsChecking && !gen.skipBoundsCheck {
		// For rvalue contexts, wrap in compound expression with bounds check
		gen.output.WriteString("({ ")
		gen.output.WriteString("int __idx = ")
		gen.generateNode(node.Children[0])
		gen.output.WriteString("; ")

		if needsArrayCast {
			gen.output.WriteString("AhoyArray* __arr = (AhoyArray*)")
			gen.output.WriteString(objExpr)
			gen.output.WriteString("; ")
		} else {
			gen.output.WriteString("AhoyArray* __arr = ")
			gen.output.WriteString(objExpr)
			gen.output.WriteString("; ")
		}

		gen.output.WriteString("if (__idx < 0 || __idx >= __arr->length) { ")
		gen.output.WriteString("fprintf(stderr, \"RUNTIME ERROR: Array bounds violation\\n\"); ")
		gen.output.WriteString("fprintf(stderr, \"  File: ")
		gen.output.WriteString(gen.sourceFilename)
		gen.output.WriteString("\\n\"); ")
		gen.output.WriteString("fprintf(stderr, \"  Line: ")
		gen.output.WriteString(strconv.Itoa(node.Line))
		gen.output.WriteString("\\n\"); ")
		gen.output.WriteString("fprintf(stderr, \"  Array: ")
		gen.output.WriteString(arrayName)
		gen.output.WriteString("\\n\"); ")
		gen.output.WriteString("fprintf(stderr, \"  Index: %d\\n\", __idx); ")
		gen.output.WriteString("fprintf(stderr, \"  Valid range: 0 to %d\\n\", __arr->length - 1); ")
		gen.output.WriteString("exit(1); ")
		gen.output.WriteString("} ")

		// Check if we know the element type
		if elemType, exists := gen.checkedArrayElem(arrayName, node); exists {
			cType := gen.mapType(elemType)
			if cType == "double" || cType == "float" {
				// A float element is a heap double: the runtime stores a pointer
				// (see print_array_helper), so reading has to dereference it.
				gen.output.WriteString("(*(double*)(intptr_t)__arr->data[__idx])")
			} else if cType != "int" {
				// Check if this is a C struct OR Ahoy struct type stored as pointer
				isCStruct := gen.cTypeDefinitions[cType] && !strings.HasSuffix(cType, "*") &&
					cType != "int" && cType != "double" && cType != "char*" && cType != "bool"
				isAhoyStruct := false
				if !isCStruct {
					if _, exists := gen.structs[elemType]; exists {
						isAhoyStruct = true
					} else if _, exists := gen.structs[cType]; exists {
						isAhoyStruct = true
					}
				}
				if isCStruct || isAhoyStruct {
					// Dereference the pointer to get the struct value
					gen.output.WriteString("(*(")
					gen.output.WriteString(cType)
					gen.output.WriteString("*)__arr->data[__idx])")
				} else {
					gen.output.WriteString("((")
					gen.output.WriteString(cType)
					gen.output.WriteString(")(intptr_t)__arr->data[__idx])")
				}
			} else {
				// For int, cast from void* through intptr_t
				gen.output.WriteString("((int)(intptr_t)__arr->data[__idx])")
			}
		} else {
			// Unknown type, assume int
			gen.output.WriteString("((int)(intptr_t)__arr->data[__idx])")
		}

		gen.output.WriteString("; })")
		return
	}

	// Check if we know the element type
	if elemType, exists := gen.checkedArrayElem(arrayName, node); exists {
		cType := gen.mapType(elemType)
		if cType == "double" || cType == "float" {
			// A float element is a heap double; dereference the pointer.
			if needsArrayCast {
				gen.output.WriteString("(*(double*)(intptr_t)((AhoyArray*)")
				gen.output.WriteString(objExpr)
				gen.output.WriteString(")->data[")
			} else {
				gen.output.WriteString("(*(double*)(intptr_t)")
				gen.output.WriteString(objExpr)
				gen.output.WriteString("->data[")
			}
			gen.generateNode(node.Children[0])
			gen.output.WriteString("])")
			return
		}
		// Cast to the appropriate type for non-int types (need intptr_t intermediate for pointer safety)
		if cType != "int" {
			// Check if this is a C struct OR Ahoy struct type stored as pointer
			isCStruct := gen.cTypeDefinitions[cType] && !strings.HasSuffix(cType, "*") &&
				cType != "int" && cType != "double" && cType != "char*" && cType != "bool"
			isAhoyStruct := false
			if !isCStruct {
				if _, exists := gen.structs[elemType]; exists {
					isAhoyStruct = true
				} else if _, exists := gen.structs[cType]; exists {
					isAhoyStruct = true
				}
			}
			if isCStruct || isAhoyStruct {
				// Dereference the pointer to get the struct value
				if needsArrayCast {
					gen.output.WriteString("(*(")
					gen.output.WriteString(cType)
					gen.output.WriteString("*)((AhoyArray*)")
					gen.output.WriteString(objExpr)
					gen.output.WriteString(")->data[")
				} else {
					gen.output.WriteString("(*(")
					gen.output.WriteString(cType)
					gen.output.WriteString("*)")
					gen.output.WriteString(objExpr)
					gen.output.WriteString("->data[")
				}
				gen.generateNode(node.Children[0])
				gen.output.WriteString("])")
			} else {
				if needsArrayCast {
					gen.output.WriteString("((")
					gen.output.WriteString(cType)
					gen.output.WriteString(")(intptr_t)((AhoyArray*)")
					gen.output.WriteString(objExpr)
					gen.output.WriteString(")->data[")
				} else {
					gen.output.WriteString("((")
					gen.output.WriteString(cType)
					gen.output.WriteString(")(intptr_t)")
					gen.output.WriteString(objExpr)
					gen.output.WriteString("->data[")
				}
				gen.generateNode(node.Children[0])
				gen.output.WriteString("])")
			}
			return
		}
	}

	// Default: no cast for int/intptr_t values to preserve lvalue for assignments
	if needsArrayCast {
		gen.output.WriteString("((AhoyArray*)")
		gen.output.WriteString(objExpr)
		gen.output.WriteString(")->data[")
	} else {
		gen.output.WriteString(objExpr)
		gen.output.WriteString("->data[")
	}
	gen.generateNode(node.Children[0])
	gen.output.WriteString("]")
}

// generateChainedArrayAccess handles 2D array access: grid[r][c]
// Children[0] = inner array access (grid[r]), Children[1] = outer index (c)
func (gen *CodeGenerator) generateChainedArrayAccess(node *ahoy.ASTNode) {
	innerAccess := node.Children[0]
	outerIndex := node.Children[1]

	// Member-base index (a.models[i]): the inner access is a member
	// expression, not another index. Type the elements off the member
	// (mirrors the typed path in generateArrayAccess; no bounds check,
	// same as 2D chained access).
	if innerAccess.Type == ahoy.NODE_MEMBER_ACCESS {
		arrayType := gen.inferType(innerAccess)
		elemType := arrayType
		if strings.HasPrefix(arrayType, "array[") && strings.HasSuffix(arrayType, "]") {
			elemType = strings.TrimSuffix(strings.TrimPrefix(arrayType, "array["), "]")
		}
		cType := gen.mapType(elemType)
		if cType == "double" || cType == "float" {
			gen.output.WriteString("(*(double*)(intptr_t)((AhoyArray*)")
			gen.generateNode(innerAccess)
			gen.output.WriteString(")->data[")
			gen.generateNode(outerIndex)
			gen.output.WriteString("])")
			return
		}
		isCStruct := gen.cTypeDefinitions[cType] && !strings.HasSuffix(cType, "*") &&
			cType != "int" && cType != "double" && cType != "char*" && cType != "bool"
		isAhoyStruct := false
		if !isCStruct {
			if _, exists := gen.structs[elemType]; exists {
				isAhoyStruct = true
			} else if _, exists := gen.structs[cType]; exists {
				isAhoyStruct = true
			}
		}
		if isCStruct || isAhoyStruct {
			gen.output.WriteString("(*(")
			gen.output.WriteString(cType)
			gen.output.WriteString("*)(intptr_t)((AhoyArray*)")
			gen.generateNode(innerAccess)
			gen.output.WriteString(")->data[")
			gen.generateNode(outerIndex)
			gen.output.WriteString("])")
			return
		}
		gen.output.WriteString("((AhoyArray*)")
		gen.generateNode(innerAccess)
		gen.output.WriteString(")->data[")
		gen.generateNode(outerIndex)
		gen.output.WriteString("]")
		return
	}

	// The inner access gives us an AhoyArray*, then we access its element
	// Cast result to AhoyArray* and access
	gen.output.WriteString("((AhoyArray*)")
	gen.generateNode(innerAccess)
	gen.output.WriteString(")->data[")
	gen.generateNode(outerIndex)
	gen.output.WriteString("]")
}

// generateChainedArrayAssignment handles 2D array assignment: grid[r][c]: value
func (gen *CodeGenerator) generateChainedArrayAssignment(node *ahoy.ASTNode) {
	accessNode := node.Children[0]
	valueNode := node.Children[1]

	// For chained access, Children[0] is the inner array access, Children[1] is the outer index
	innerAccess := accessNode.Children[0]
	outerIndex := accessNode.Children[1]

	// Generate: ((AhoyArray*)(inner_access))->data[outer_index] = value
	gen.output.WriteString("((AhoyArray*)")
	gen.generateNode(innerAccess)
	gen.output.WriteString(")->data[")
	gen.generateNode(outerIndex)
	gen.output.WriteString("] = (void*)(intptr_t)")
	gen.generateNode(valueNode)
	gen.output.WriteString(";\n")
}

func (gen *CodeGenerator) generateDictAccess(node *ahoy.ASTNode) {
	// Check if the dict variable is generic (intptr_t) and needs casting
	dictName := node.Value
	dictExpr := gen.nameExpr(dictName)
	dictType := ""

	// Check variable type
	if varType, exists := gen.variables[dictName]; exists {
		dictType = varType
	} else if varType, exists := gen.functionVars[dictName]; exists {
		dictType = varType
	}

	// Use hashMapGetDouble which converts values to double (works for all numeric types)
	// If generic/any, cast to HashMap*
	if dictType == "generic" || dictType == "any" {
		gen.output.WriteString("hashMapGetDouble((HashMap*)")
		gen.output.WriteString(dictExpr)
		gen.output.WriteString(", ")
	} else {
		gen.output.WriteString("hashMapGetDouble(")
		gen.output.WriteString(dictExpr)
		gen.output.WriteString(", ")
	}

	gen.generateNode(node.Children[0])
	gen.output.WriteString(")")
}

// generateDictPutTyped emits a hashMapPutTyped call for one key/value pair.
// The runtime tags every entry with its value type, and readers (format_dict_value,
// hashMapGetDouble) believe that tag, so an int stored through the untyped
// hashMapPut was read back as a char* and crashed. Floats are heap-allocated
// because the runtime keeps AHOY_TYPE_FLOAT values as double* (see hashMapGetTyped).
// dictExpr is emitted verbatim, so a caller that needs a cast must include it.
func (gen *CodeGenerator) generateDictPutTyped(dictExpr string, keyNode, valueNode *ahoy.ASTNode) {
	valueType := gen.inferType(valueNode)
	ahoyTypeEnum := gen.getAhoyTypeEnum(valueType)

	gen.output.WriteString("hashMapPutTyped(")
	gen.output.WriteString(dictExpr)
	gen.output.WriteString(", ")
	gen.generateNode(keyNode)
	gen.output.WriteString(", ")

	if valueType == "float" || valueType == "double" {
		floatVar := fmt.Sprintf("__float_ptr_%d", gen.varCounter)
		gen.varCounter++
		gen.output.WriteString("(void*)({ double* ")
		gen.output.WriteString(floatVar)
		gen.output.WriteString(" = malloc(sizeof(double)); *")
		gen.output.WriteString(floatVar)
		gen.output.WriteString(" = ")
		gen.generateNode(valueNode)
		gen.output.WriteString("; ")
		gen.output.WriteString(floatVar)
		gen.output.WriteString("; })")
	} else {
		gen.output.WriteString("(void*)(intptr_t)")
		gen.generateNode(valueNode)
	}

	gen.output.WriteString(", ")
	gen.output.WriteString(ahoyTypeEnum)
	gen.output.WriteString(");\n")
}

func (gen *CodeGenerator) generateDictLiteral(node *ahoy.ASTNode) {
	dictName := fmt.Sprintf("dict_%d", gen.varCounter)
	gen.varCounter++

	gen.output.WriteString("({ HashMap* ")
	gen.output.WriteString(dictName)
	gen.output.WriteString(" = createHashMap(16); ")

	// Add key-value pairs
	for i := 0; i < len(node.Children); i += 2 {
		key := node.Children[i]
		value := node.Children[i+1]

		// Determine value type
		valueType := gen.inferType(value)
		ahoyTypeEnum := "AHOY_TYPE_STRING"
		switch valueType {
		case "int":
			ahoyTypeEnum = "AHOY_TYPE_INT"
		case "float":
			ahoyTypeEnum = "AHOY_TYPE_FLOAT"
		case "char":
			ahoyTypeEnum = "AHOY_TYPE_CHAR"
		default:
			ahoyTypeEnum = "AHOY_TYPE_STRING"
		}

		gen.output.WriteString("hashMapPutTyped(")
		gen.output.WriteString(dictName)
		gen.output.WriteString(", ")

		// If key is an identifier, convert to string literal
		if key.Type == ahoy.NODE_IDENTIFIER {
			gen.output.WriteString("\"")
			gen.output.WriteString(key.Value)
			gen.output.WriteString("\"")
		} else {
			gen.generateNode(key)
		}

		// For floats, allocate heap memory to store the value properly
		if valueType == "float" {
			floatVar := fmt.Sprintf("__float_ptr_%d", gen.varCounter)
			gen.varCounter++
			gen.output.WriteString(", (void*)({ double* ")
			gen.output.WriteString(floatVar)
			gen.output.WriteString(" = malloc(sizeof(double)); *")
			gen.output.WriteString(floatVar)
			gen.output.WriteString(" = ")
			gen.generateNode(value)
			gen.output.WriteString("; ")
			gen.output.WriteString(floatVar)
			gen.output.WriteString("; }), ")
			gen.output.WriteString(ahoyTypeEnum)
			gen.output.WriteString("); ")
		} else {
			gen.output.WriteString(", (void*)(intptr_t)")
			gen.generateNode(value)
			gen.output.WriteString(", ")
			gen.output.WriteString(ahoyTypeEnum)
			gen.output.WriteString("); ")
		}
	}

	gen.output.WriteString(dictName)
	gen.output.WriteString("; })")
}

func (gen *CodeGenerator) mapType(langType string) string {
	// Function values are {code pointer, environment} pairs
	if ahoy.IsFunctionType(langType) {
		return "AhoyFn"
	}

	// A C callback typedef (typedef void (*LogCallback)(int)) is its own type
	if _, ok := gen.cFuncPtrTypedefs[langType]; ok {
		return langType
	}

	// Check for typed collections first
	if strings.HasPrefix(langType, "array[") {
		return "AhoyArray*"
	}
	if strings.HasPrefix(langType, "dict[") || strings.HasPrefix(langType, "dict<") {
		return "HashMap*"
	}

	// Handle nested type names (e.g., Card.Assassin -> Card_Assassin, particle.wind_particle -> Particle_Wind_particle)
	if strings.Contains(langType, ".") {
		return toCStructName(langType)
	}

	// Distinct newtypes share their base's C layout: resolve and map on.
	if base, ok := gen.distinctTypes[langType]; ok {
		return gen.mapType(base)
	}

	// Handle known types first before pointer logic
	switch langType {
	case "generic", "any":
		return "intptr_t"
	case "int":
		return "int"
	case "float":
		return "double"
	case "string", "char*", "char":
		return "char*"
	case "bool":
		return "bool"
	case "dict":
		return "HashMap*"
	case "array":
		return "AhoyArray*"
	case "AhoyJSON*", "json":
		return "AhoyJSON*"
	case "void":
		return "void"
	case "vector2":
		return "Vector2"
	case "color":
		return "Color"
	case "pool":
		return "AhoyPool*"
	case "job":
		return "AhoyJob*"
	}

	// Check for pointer types (e.g., "int*") but not already mapped types like "char*"
	if strings.HasSuffix(langType, "*") {
		baseType := strings.TrimSuffix(langType, "*")
		// Recursively map the base type
		mappedBase := gen.mapType(baseType)
		return mappedBase + "*"
	}

	// Check if it's an Ahoy struct type (user-defined)
	if _, exists := gen.structs[langType]; exists {
		return capitalizeFirst(langType)
	}

	// Check if exact match exists in C types (case-sensitive)
	if gen.cTypeDefinitions[langType] {
		return langType
	}

	// Check if any C type matches case-insensitively for backward compatibility.
	// Sorted so the match is deterministic when several types collide.
	lowerLangType := strings.ToLower(langType)
	cTypes := make([]string, 0, len(gen.cTypeDefinitions))
	for cType := range gen.cTypeDefinitions {
		cTypes = append(cTypes, cType)
	}
	sort.Strings(cTypes)
	for _, cType := range cTypes {
		if strings.ToLower(cType) == lowerLangType {
			// Return the properly-cased version from the type definitions
			return cType
		}
	}

	return "int"
}

func (gen *CodeGenerator) inferType(node *ahoy.ASTNode) string {
	switch node.Type {
	case ahoy.NODE_IS_LIST:
		return "bool"
	case ahoy.NODE_TYPE_PROPERTY:
		return "string" // .type property returns a string
	case ahoy.NODE_NUMBER:
		if strings.Contains(node.Value, ".") {
			return "float"
		}
		return "int"
	case ahoy.NODE_STRING, ahoy.NODE_RAW_STRING:
		return "string"
	case ahoy.NODE_F_STRING:
		return "string"
	case ahoy.NODE_BOOLEAN:
		return "bool"
	case ahoy.NODE_DICT_LITERAL:
		return "dict"
	case ahoy.NODE_ARRAY_LITERAL:
		// Try to infer element type from array literal contents
		if len(node.Children) > 0 {
			// Get the type of the first element
			firstElemType := gen.inferType(node.Children[0])
			if firstElemType != "unknown" {
				return "array[" + firstElemType + "]"
			}
		}
		// Fallback: untyped arrays are just "array"
		return "array"
	case ahoy.NODE_OBJECT_LITERAL:
		// Check if it's a typed object literal
		if node.Value != "" {
			return node.Value
		}
		return "struct"
	case ahoy.NODE_CALL:
		// Infer return type of function calls
		if td, ok := gen.cFuncPtrTypedefVar(node.Value); ok {
			return td.ReturnType
		}
		// A distinct conversion yields the distinct type.
		if _, ok := gen.distinctTypes[node.Value]; ok {
			return node.Value
		}
		if node.Value == "jobs" {
			return "pool"
		}
		if node.Value == "sprintf" {
			return "string"
		}
		// Type casts
		if node.Value == "int" {
			return "int"
		}
		if node.Value == "float" {
			return "float"
		}
		if node.Value == "char" {
			return "char"
		}
		if node.Value == "string" {
			return "string"
		}
		// Check if it's a C function and we know its return type
		if returnType, exists := gen.cFunctionReturnTypes[node.Value]; exists {
			return returnType
		}
		// Check if we know the function's return type (user-defined functions)
		if returnTypes, exists := gen.functionReturnTypes[node.Value]; exists && len(returnTypes) > 0 {
			// For single return, return that type
			// For multiple returns, this will be used in tuple assignment context
			return returnTypes[0]
		}
		return "int"
	case ahoy.NODE_METHOD_CALL:
		// Job pool methods
		if node.Value == "submit" && len(node.Children) > 0 &&
			gen.inferType(node.Children[0]) == "pool" {
			return "job"
		}
		if node.Value == "drain" && len(node.Children) > 0 &&
			gen.inferType(node.Children[0]) == "pool" {
			return "int"
		}
		if (node.Value == "result" || node.Value == "done") && len(node.Children) > 0 &&
			gen.inferType(node.Children[0]) == "job" {
			if node.Value == "done" {
				return "int"
			}
			if t, ok := gen.jobResultTypes[node.Children[0].Value]; ok && t != "" {
				return t
			}
			return "intptr_t"
		}
		if node.Value == "wait" && len(node.Children) > 1 && len(node.Children[1].Children) > 0 {
			if t, ok := gen.jobResultTypes[node.Children[1].Children[0].Value]; ok && t != "" {
				return t
			}
			return "intptr_t"
		}
		// A callback-table field (r.draw_sprite|...|) returns the field's own
		// return type.
		if len(node.Children) > 0 && node.Children[0].Type == ahoy.NODE_IDENTIFIER {
			objType := strings.TrimPrefix(gen.inferType(node.Children[0]), "struct:")
			if info := gen.structs[objType]; info != nil {
				for _, f := range info.Fields {
					ft := f.AhoyType
					if ft == "" {
						ft = f.Type
					}
					if f.Name == node.Value && ahoy.IsFunctionType(ft) {
						if _, ret, ok := ahoy.ParseFunctionType(ft); ok {
							return ret
						}
					}
				}
			}
		}

		// User-defined methods: the result type is the method's declared
		// return type. This mirrors the generateMethodCall dispatch order
		// (callback fields first, then the method table).
		if len(node.Children) > 1 {
			if mangled, _, ok := gen.resolveUserMethod(node); ok {
				if types, exists := gen.functionReturnTypes[mangled]; exists && len(types) > 0 {
					return types[0]
				}
				if e, ok2 := gen.methodTable[ahoy.NormalizeReceiverType(gen.inferType(node.Children[0]))][node.Value]; ok2 {
					if len(e.Sig.ReturnTypes) > 0 {
						return e.Sig.ReturnTypes[0]
					}
				}
			}
		}

		// Check if this is a namespaced C function call
		if len(node.Children) > 0 && node.Children[0].Type == ahoy.NODE_IDENTIFIER {
			namespace := node.Children[0].Value
			methodName := node.Value
			if returnTypeMap, exists := gen.cNamespaceReturnTypes[namespace]; exists {
				if returnType, found := returnTypeMap[methodName]; found {
					return returnType
				}
			}
		}

		// Check the object type to determine if it's a dict or array method
		objectType := ""
		if len(node.Children) > 0 {
			objectType = gen.inferType(node.Children[0])
		}

		// dump_struct returns string
		if node.Value == "dump_struct" {
			return "string"
		}

		// String methods that return string
		if node.Value == "upper" || node.Value == "lower" ||
			node.Value == "replace" || node.Value == "camel_case" ||
			node.Value == "snake_case" || node.Value == "pascal_case" ||
			node.Value == "kebab_case" || node.Value == "strip" ||
			node.Value == "lpad" || node.Value == "rpad" ||
			node.Value == "pad" || node.Value == "get_file" {
			return "string"
		}
		// String methods that return int
		if node.Value == "length" || node.Value == "count" {
			return "int"
		}
		// String methods that return bool
		if node.Value == "contains" || node.Value == "match" {
			return "bool"
		}
		// String method split always yields strings.
		if node.Value == "split" {
			return "array[string]"
		}

		// Dictionary-specific methods
		if objectType == "dict" {
			if node.Value == "size" {
				return "int"
			}
			if node.Value == "has" || node.Value == "has_all" {
				return "bool"
			}
			if node.Value == "keys" || node.Value == "values" {
				return "array"
			}
			if node.Value == "sort" || node.Value == "stable_sort" || node.Value == "merge" {
				return "dict"
			}
		}

		// Array methods that return arrays
		if node.Value == "map" || node.Value == "filter" ||
			node.Value == "sort" || node.Value == "reverse" ||
			node.Value == "shuffle" || node.Value == "push" ||
			node.Value == "fill" || node.Value == "remove" {
			return "array"
		}
		// Array methods that return int
		if node.Value == "sum" || node.Value == "pop" ||
			node.Value == "pick" || node.Value == "has" {
			return "int"
		}
		return "int"
	case ahoy.NODE_UNARY_OP:
		// Handle unary operators
		if node.Value == "&" {
			// Address-of operator - return pointer to operand type
			operandType := gen.inferType(node.Children[0])
			return operandType + "*"
		}
		if node.Value == "^" || node.Value == "*" {
			// Pointer dereference - return type pointed to
			operandType := gen.inferType(node.Children[0])
			// Remove trailing * if present
			if strings.HasSuffix(operandType, "*") {
				return strings.TrimSuffix(operandType, "*")
			}
			return "int" // fallback
		}
		// Other unary operators preserve type
		return gen.inferType(node.Children[0])
	case ahoy.NODE_BINARY_OP:
		// Simple inference - could be more sophisticated
		leftType := gen.inferType(node.Children[0])
		rightType := gen.inferType(node.Children[1])
		if node.Value == "+" && isStringTypeName(leftType) && isStringTypeName(rightType) {
			return "string"
		}
		// Either spelling of float promotes (member access can yield
		// "double" for the same Ahoy float, depending on inference order).
		if leftType == "float" || leftType == "double" || rightType == "float" || rightType == "double" {
			return "float"
		}
		// Power always yields float: codegen emits C pow(), which returns
		// double. Inferring int here miscompiled every consumer that trusts
		// the type (print used %d on a double and showed garbage).
		if node.Value == "pow" || node.Value == "**" {
			return "float"
		}
		return "int"
	case ahoy.NODE_TERNARY:
		// Ternary returns the type of its branches (assume both branches have same type)
		trueType := gen.inferType(node.Children[1])
		falseType := gen.inferType(node.Children[2])
		// If types differ, try to find common type
		if trueType == "float" || falseType == "float" {
			return "float"
		}
		if trueType == "string" || falseType == "string" {
			return "string"
		}
		return trueType
	case ahoy.NODE_SWITCH_STATEMENT:
		// Infer type from first case body
		if len(node.Children) > 1 {
			firstCase := node.Children[1]
			if firstCase.Type == ahoy.NODE_SWITCH_CASE && len(firstCase.Children) > 1 {
				caseBody := firstCase.Children[1]
				return gen.inferSwitchCaseType(caseBody)
			}
		}
		return "int"
	case ahoy.NODE_IDENTIFIER:
		// Check if this is a JSON variable
		if gen.jsonVariables[node.Value] {
			return "AhoyJSON*"
		}
		// Check if this is a constant
		if constType, exists := gen.constants[node.Value]; exists {
			return constType
		}
		// Parameters of the function being generated come first: a parameter
		// shadows any same-named variable elsewhere in the program, and the flat
		// variable map is filled from every function's body, so a local `n` in
		// one script used to decide the type of `n` in another function (a Node
		// parameter read as int, and pushing it into an array emitted a struct
		// cast that C rejected).
		if varType, exists := gen.functionVars[node.Value]; exists {
			// Normalize dict types
			if strings.HasPrefix(varType, "dict<") || strings.HasPrefix(varType, "dict[") {
				return "dict"
			}
			// Return full array type including element type
			return varType
		}
		if varType, exists := gen.variables[node.Value]; exists {
			// Normalize dict types
			if strings.HasPrefix(varType, "dict<") || strings.HasPrefix(varType, "dict[") {
				return "dict"
			}
			// Return full array type including element type for proper iteration
			// DON'T normalize array[ElementType] to just "array"
			return varType
		}
		// Check if this variable is sourced from a dict access
		if dictName, exists := gen.dictSourcedVars[node.Value]; exists {
			// It's sourced from a dict, so we need to check the dict access type
			// For now, return int as a safe default for dict values
			// In the future, we could track the value types more precisely
			_ = dictName // avoid unused variable error
			return "int"
		}
		// Fallback: return int as default (could be untyped identifier)
		return "int"
	case ahoy.NODE_ARRAY_ACCESS:
		// Get the array variable name and look up its element type
		arrayName := node.Value

		// Handle nested array access (e.g., grid[row][col])
		// If arrayName is empty, this is accessing the result of another expression
		if arrayName == "" && len(node.Children) > 0 {
			// The first child is the array being accessed (could be another array access)
			arrayExpr := node.Children[0]
			// Infer the type of the array expression
			arrayType := gen.inferType(arrayExpr)

			// If it's an array type, extract the element type
			if strings.HasPrefix(arrayType, "array[") {
				elemType := strings.TrimSuffix(strings.TrimPrefix(arrayType, "array["), "]")
				return elemType
			}
			// Check if the inner array is a 2D array - return its element type
			if arrayExpr.Type == ahoy.NODE_ARRAY_ACCESS && arrayExpr.Value != "" {
				if elemType, exists := gen.array2DElementTypes[arrayExpr.Value]; exists {
					return elemType
				}
			}
			// Otherwise return the type as-is (might be a struct or other type)
			return arrayType
		}

		// Check if this is a 2D array - return array[elementType] for first dimension access
		if inner2DType, exists := gen.array2DElementTypes[arrayName]; exists {
			return "array[" + inner2DType + "]"
		}

		if elemType, exists := gen.checkedArrayElem(arrayName, node); exists {
			return elemType
		}
		// Check if the array itself is a generic parameter
		arrayType := ""
		if varType, exists := gen.variables[arrayName]; exists {
			arrayType = varType
		} else if varType, exists := gen.functionVars[arrayName]; exists {
			arrayType = varType
		}

		// Check if the variable has an array type annotation
		if strings.HasPrefix(arrayType, "array[") {
			elemType := strings.TrimSuffix(strings.TrimPrefix(arrayType, "array["), "]")
			return elemType
		}

		// If array is generic/any, elements are also generic/any (intptr_t)
		if arrayType == "generic" || arrayType == "any" {
			return "any"
		}
		// Default to int if we don't know the element type
		return "int"
	case ahoy.NODE_DICT_ACCESS:
		// Dictionary values - use hashMapGetDouble which handles type conversion
		return "float"
	case ahoy.NODE_OBJECT_ACCESS:
		// Object property access with angle brackets - look up struct field type
		if len(node.Children) > 0 {
			objectName := node.Value
			propertyName := node.Children[0].Value // String literal with property name

			// Get the type of the object variable
			objectType := ""
			if varType, exists := gen.variables[objectName]; exists {
				objectType = varType
			} else if varType, exists := gen.functionVars[objectName]; exists {
				objectType = varType
			}

			// Look up the struct definition
			if structInfo, exists := gen.structs[objectType]; exists {
				// Find the field type
				for _, field := range structInfo.Fields {
					if field.Name == propertyName {
						return field.Type
					}
				}
			}
		}
		return "char*"
	case ahoy.NODE_MEMBER_ACCESS:
		// Member access (dot notation) - look up struct field type
		if len(node.Children) > 0 {
			objectNode := node.Children[0]
			memberName := node.Value

			// Check if this is enum member access
			if objectNode.Type == ahoy.NODE_IDENTIFIER {
				enumMemberKey := fmt.Sprintf("%s.%s", objectNode.Value, memberName)
				if memberType, exists := gen.enumMemberTypes[enumMemberKey]; exists {
					return memberType
				}
			}

			// Get the type of the object
			objectType := gen.inferType(objectNode)

			// If object is JSON, member access returns AhoyJSON*
			if objectType == "AhoyJSON*" {
				return "AhoyJSON*"
			}

			// Strip pointer suffix for struct lookup
			lookupType := strings.TrimSuffix(objectType, "*")

			// Resolve typedef aliases (e.g., RenderTexture2D -> RenderTexture)
			if baseType, exists := gen.cTypedefs[lookupType]; exists {
				lookupType = baseType
			}

			// Look up the Ahoy struct definition
			if structInfo, exists := gen.structs[lookupType]; exists {
				// Find the field type. Prefer the stable Ahoy-level type:
				// field.Type is overwritten with the C type when the struct
				// is emitted, so inferences after emission would otherwise
				// see "double" where earlier ones saw "float".
				for _, field := range structInfo.Fields {
					if field.Name == memberName {
						if field.AhoyType != "" {
							return field.AhoyType
						}
						return field.Type
					}
				}
			}

			// Look up C struct fields
			if fields, exists := gen.cStructFields[lookupType]; exists {
				if fieldType, found := fields[memberName]; found {
					return fieldType
				}
			}

			// Check if the object is a local variable and try to infer its actual struct type
			if objectNode.Type == ahoy.NODE_IDENTIFIER {
				varName := objectNode.Value
				var varType string
				if vt, exists := gen.functionVars[varName]; exists {
					varType = vt
				} else if vt, exists := gen.variables[varName]; exists {
					varType = vt
				}

				// If we found a type, look up the struct for it
				if varType != "" {
					lookupType = strings.TrimSuffix(varType, "*")
					lookupType = strings.TrimPrefix(lookupType, "struct:")

					// Try Ahoy structs again
					if structInfo, exists := gen.structs[lookupType]; exists {
						for _, field := range structInfo.Fields {
							if field.Name == memberName {
								if field.AhoyType != "" {
									return field.AhoyType
								}
								return field.Type
							}
						}
					}

					// Try C structs again
					if fields, exists := gen.cStructFields[lookupType]; exists {
						if fieldType, found := fields[memberName]; found {
							return fieldType
						}
					}
				}
			}

			// Built-in collection property: every array, dict and string has a
			// length. Without this the fallback below tags it as a string, and
			// print|arr.length| formats the int with %s.
			if memberName == "length" {
				return "int"
			}
		}
		return "char*"
	default:
		return "int"
	}
}

// inferReturnTypes finds return statements in a function body and infers their types
// It takes the full function node to have access to parameter information
func (gen *CodeGenerator) inferReturnTypes(funcNode *ahoy.ASTNode) []string {
	if funcNode == nil || len(funcNode.Children) < 2 {
		return []string{}
	}

	// Get parameters and body
	params := funcNode.Children[0]
	body := funcNode.Children[1]

	// Temporarily set up functionVars with parameter types for inference
	savedFunctionVars := gen.functionVars
	gen.functionVars = make(map[string]string)
	gen.dictSourcedVars = make(map[string]string)
	gen.dictSourcedKeys = make(map[string]string)
	gen.dictLoopValueExpr = make(map[string]string)
	gen.nestedScopeVars = make(map[string]bool)
	gen.varDeclIndent = make(map[string]int)
	for _, param := range params.Children {
		if param.DataType != "" {
			gen.functionVars[param.Value] = param.DataType
		} else {
			// Parameters without explicit type are any (generic) (will be inferred at call site)
			gen.functionVars[param.Value] = "any"
		}
	}

	// Scan function body for variable declarations to track their types
	gen.scanVariableDeclarations(body)

	// Find the first return statement
	returnStmt := gen.findReturnStatement(body)

	if returnStmt == nil {
		// Restore functionVars
		gen.functionVars = savedFunctionVars
		return []string{}
	}

	// Infer types from each returned expression
	types := []string{}
	for _, child := range returnStmt.Children {
		inferredType := gen.inferType(child)
		types = append(types, inferredType)
	}

	// Restore functionVars
	gen.functionVars = savedFunctionVars

	return types
}

// scanVariableDeclarations scans a node tree and tracks variable declarations in functionVars
func (gen *CodeGenerator) scanVariableDeclarations(node *ahoy.ASTNode) {
	if node == nil {
		return
	}

	// Check if this is a variable declaration (assignment with no prior declaration)
	if node.Type == ahoy.NODE_VARIABLE_DECLARATION || node.Type == ahoy.NODE_ASSIGNMENT {
		varName := node.Value
		if len(node.Children) > 0 {
			valueNode := node.Children[0]
			// Infer the type from the assigned value
			varType := gen.inferType(valueNode)
			gen.functionVars[varName] = varType
		}
	}

	// Handle tuple assignments - e.g., new_dict,new_dict2: test_dictionary||
	if node.Type == ahoy.NODE_TUPLE_ASSIGNMENT && len(node.Children) >= 2 {
		leftSide := node.Children[0]
		rightSide := node.Children[1]

		// Check if right side is a function call
		if len(rightSide.Children) == 1 && rightSide.Children[0].Type == ahoy.NODE_CALL {
			callNode := rightSide.Children[0]
			funcName := callNode.Value

			// Look up function return types
			if returnTypes, exists := gen.functionReturnTypes[funcName]; exists {
				// Assign each variable its corresponding return type
				for i, leftChild := range leftSide.Children {
					if leftChild.Type == ahoy.NODE_IDENTIFIER && i < len(returnTypes) {
						varName := leftChild.Value
						varType := returnTypes[i]
						gen.functionVars[varName] = varType
					}
				}
			}
		}
	}

	// Recursively scan children
	for _, child := range node.Children {
		gen.scanVariableDeclarations(child)
	}
}

// findReturnStatement recursively finds the first return statement in a node tree
func (gen *CodeGenerator) findReturnStatement(node *ahoy.ASTNode) *ahoy.ASTNode {
	if node == nil {
		return nil
	}

	if node.Type == ahoy.NODE_RETURN_STATEMENT {
		return node
	}

	// Recursively search children
	for _, child := range node.Children {
		if result := gen.findReturnStatement(child); result != nil {
			return result
		}
	}

	return nil
}

// inferSwitchCaseType infers the type of a switch case body
func (gen *CodeGenerator) inferSwitchCaseType(body *ahoy.ASTNode) string {
	if body == nil {
		return "int"
	}

	// If it's a block, infer from last statement
	if body.Type == ahoy.NODE_BLOCK && len(body.Children) > 0 {
		return gen.inferType(body.Children[len(body.Children)-1])
	}

	return gen.inferType(body)
}

// isEnumType checks if a name is an enum type
func (gen *CodeGenerator) isEnumType(name string) bool {
	_, exists := gen.enums[name]
	return exists
}

// reorderRaylibIncludes ensures raylib.h comes before raymath.h and raygui.h to avoid duplicate type definitions
// raylib.h defines RL_VECTOR2_TYPE, RL_VECTOR3_TYPE, etc. which raymath.h checks
// raygui.h also depends on raylib.h being included first
func (gen *CodeGenerator) reorderRaylibIncludes() {
	var raylibIdx, raymathIdx, rayguiIdx int = -1, -1, -1

	// Find indices of raylib.h, raymath.h, and raygui.h
	for i, include := range gen.orderedIncludes {
		if strings.Contains(include, "raylib.h") {
			raylibIdx = i
		}
		if strings.Contains(include, "raymath.h") {
			raymathIdx = i
		}
		if strings.Contains(include, "raygui.h") {
			rayguiIdx = i
		}
	}

	// If raylib.h exists, ensure it comes before raymath.h and raygui.h
	if raylibIdx != -1 {
		// If raymath comes before raylib, swap them
		if raymathIdx != -1 && raymathIdx < raylibIdx {
			gen.orderedIncludes[raylibIdx], gen.orderedIncludes[raymathIdx] =
				gen.orderedIncludes[raymathIdx], gen.orderedIncludes[raylibIdx]
			// Update raylibIdx after swap
			raylibIdx, raymathIdx = raymathIdx, raylibIdx
		}

		// If raygui comes before raylib, swap them
		if rayguiIdx != -1 && rayguiIdx < raylibIdx {
			gen.orderedIncludes[raylibIdx], gen.orderedIncludes[rayguiIdx] =
				gen.orderedIncludes[rayguiIdx], gen.orderedIncludes[raylibIdx]
		}
	}
}

// isConstantOrEnum checks if a name is a constant (Ahoy constant, C #define, or enum value)
func (gen *CodeGenerator) isConstantOrEnum(name string) bool {
	// Check if it's an Ahoy constant
	if _, exists := gen.constants[name]; exists {
		return true
	}
	// Check if it's an enum type name
	if _, exists := gen.enums[name]; exists {
		return true
	}
	// Check if it looks like an enum member (uppercase with underscores - likely C define or enum)
	if isScreamingSnakeCase(name) {
		return true
	}
	return false
}

func (gen *CodeGenerator) nodeToString(node *ahoy.ASTNode) string {
	oldOutput := gen.output
	gen.output = strings.Builder{}
	gen.generateNodeInternal(node, false)
	result := gen.output.String()
	gen.output = oldOutput
	return result
}

func (gen *CodeGenerator) generateFString(node *ahoy.ASTNode) {
	// Pre-parsed interpolation groups take the expression path; anything
	// else keeps the legacy textual handling below.
	if len(node.Children) > 0 {
		gen.generateFStringFromChildren(node)
		return
	}
	// Parse f-string and extract variables
	// Example: "hello{i}" -> format string "hello%d" and variables [i]
	fstring := node.Value
	var formatStr strings.Builder
	var vars []string
	var formatSpecs []string
	// dictExprs[i] is a ready-made C expression for argument i, used when the
	// value comes from an untyped dict and must be formatted at run time.
	var dictExprs []string

	i := 0
	for i < len(fstring) {
		if fstring[i] == '{' {
			// Find closing brace - need to handle nested braces for dict access
			j := i + 1
			braceDepth := 1
			for j < len(fstring) && braceDepth > 0 {
				if fstring[j] == '{' {
					braceDepth++
				} else if fstring[j] == '}' {
					braceDepth--
				}
				if braceDepth > 0 {
					j++
				}
			}
			if j < len(fstring) {
				// Extract variable name/expression
				varName := fstring[i+1 : j]
				vars = append(vars, varName)

				// Determine format specifier based on variable type
				// For now, use %d for numbers, %s for strings
				// We'll need to look up the variable type
				varType := "int"
				// Check for simple variable name (no braces/dots)
				simpleVarName := varName
				if idx := strings.IndexAny(varName, ".{<"); idx != -1 {
					simpleVarName = varName[:idx]
				}
				if knownType, exists := gen.variables[simpleVarName]; exists {
					varType = knownType
				} else if knownType, exists := gen.functionVars[simpleVarName]; exists {
					// Parameters and function locals live in functionVars. Locals also
					// get picked up by the scanVariableTypes pre-pass, but parameters
					// are not declarations and are only visible here.
					varType = knownType
				}

				// A member access ({node.texture}) must use the field's type, not the
				// object's: the object type falls through to %d, so a string field
				// was printed as a pointer.
				if dot := strings.Index(varName, "."); dot > 0 {
					fieldPart := varName[dot+1:]
					if !strings.ContainsAny(fieldPart, ".<{[") {
						objType := normalizeCollectionType(varType)
						objType = strings.TrimPrefix(objType, "struct:")
						if info := gen.structs[objType]; info != nil {
							for _, f := range info.Fields {
								if f.Name == fieldPart && f.Type != "" {
									varType = f.Type
									break
								}
							}
						}
					}
				}

				// dict<string,int> / dict[string,int] are all printed via the dict helper
				varType = normalizeCollectionType(varType)

				// A value read from an untyped dict is formatted through the helper that
				// switches on the entry's runtime type, exactly as a dict access is.
				dictExpr := ""
				if expr, isDictLoopValue := gen.dictLoopValueExpr[simpleVarName]; isDictLoopValue {
					dictExpr = expr
					formatSpecs = append(formatSpecs, "%s")
					dictExprs = append(dictExprs, dictExpr)
					formatStr.WriteString("%s")
					i = j + 1
					continue
				}
				dictExprs = append(dictExprs, "")

				formatSpec := "%d"
				if varType == "string" || varType == "char*" || varType == "intptr_t" ||
					varType == "dict" || varType == "HashMap*" {
					formatSpec = "%s"
				} else if varType == "float" || varType == "double" {
					formatSpec = "%f"
				} else if varType == "char" {
					formatSpec = "%c"
				}

				formatSpecs = append(formatSpecs, formatSpec)
				formatStr.WriteString(formatSpec)
				i = j + 1
			} else {
				formatStr.WriteByte(fstring[i])
				i++
			}
		} else {
			formatStr.WriteByte(fstring[i])
			i++
		}
	}

	// Generate sprintf call or simple string if no variables
	if len(vars) == 0 {
		gen.output.WriteString("\"")
		gen.output.WriteString(formatStr.String())
		gen.output.WriteString("\"")
	} else {
		// For now, we'll need to allocate a buffer
		// Generate: (char[]){sprintf format, vars...}
		// Actually, let's use a simpler approach with a static buffer
		bufferVar := fmt.Sprintf("__fstr_buf_%d", gen.varCounter)
		gen.varCounter++

		// We need to emit this as a statement, not an expression
		// For simplicity in expressions, we'll use a compound literal approach
		// But C doesn't support that well for strings, so we'll generate a helper

		// For now, emit inline sprintf - this works in some contexts
		gen.output.WriteString("({\n")
		gen.indent++
		gen.writeIndent()
		gen.output.WriteString("static char ")
		gen.output.WriteString(bufferVar)
		gen.output.WriteString("[256];\n")
		gen.writeIndent()
		gen.output.WriteString("sprintf(")
		gen.output.WriteString(bufferVar)
		gen.output.WriteString(", \"")
		gen.output.WriteString(formatStr.String())
		gen.output.WriteString("\"")

		for idx, v := range vars {
			gen.output.WriteString(", ")

			// A dict value is formatted by its runtime type.
			if idx < len(dictExprs) && dictExprs[idx] != "" {
				gen.output.WriteString(dictExprs[idx])
				continue
			}

			// Get the format spec for this variable
			formatSpec := "%d"
			if idx < len(formatSpecs) {
				formatSpec = formatSpecs[idx]
			}

			// Determine cast type if needed
			castType := ""
			if formatSpec == "%d" {
				castType = "(int)"
			} else if formatSpec == "%f" {
				castType = "(double)"
			}

			// Check if this is a member access expression (contains a dot)
			if strings.Contains(v, ".") {
				// Parse member access: object.member
				parts := strings.Split(v, ".")
				if len(parts) == 2 {
					objectName := parts[0]
					memberName := parts[1]

					// Determine if we need -> or .
					objectType := ""
					if knownType, exists := gen.variables[objectName]; exists {
						objectType = knownType
					}

					// Apply cast if needed
					if castType != "" {
						gen.output.WriteString(castType)
					}

					// Use -> for pointer types
					if strings.HasSuffix(objectType, "*") {
						gen.output.WriteString(objectName)
						gen.output.WriteString("->")
						gen.output.WriteString(memberName)
					} else {
						gen.output.WriteString(v)
					}
				} else {
					if castType != "" {
						gen.output.WriteString(castType)
					}
					gen.output.WriteString(v)
				}
			} else if strings.Contains(v, "|") {
				// This looks like a function call with pipe syntax (e.g., len|my_array|)
				// Parse and convert to C function call
				pipeIdx := strings.Index(v, "|")
				funcName := v[:pipeIdx]
				argsStr := ""
				if pipeIdx+1 < len(v) && v[len(v)-1] == '|' {
					argsStr = v[pipeIdx+1 : len(v)-1]
				}

				// Handle built-in len/length functions
				if funcName == "len" || funcName == "length" {
					argType := "int"
					if knownType, exists := gen.variables[argsStr]; exists {
						argType = knownType
					}

					// Generate appropriate length access
					if argType == "array" || strings.HasPrefix(argType, "array[") || argType == "AhoyArray*" {
						gen.output.WriteString("(")
						gen.output.WriteString(argsStr)
						gen.output.WriteString(")->length")
					} else if argType == "dict" || strings.HasPrefix(argType, "dict[") || argType == "HashMap*" {
						gen.output.WriteString("(")
						gen.output.WriteString(argsStr)
						gen.output.WriteString(")->size")
					} else if argType == "string" || argType == "char*" || argType == "const char*" {
						gen.output.WriteString("strlen(")
						gen.output.WriteString(argsStr)
						gen.output.WriteString(")")
					} else {
						// Default to ->length
						gen.output.WriteString("(")
						gen.output.WriteString(argsStr)
						gen.output.WriteString(")->length")
					}
				} else {
					// Other function calls - convert pipe syntax to C function call
					gen.output.WriteString(funcName)
					gen.output.WriteString("(")
					gen.output.WriteString(argsStr)
					gen.output.WriteString(")")
				}
			} else {
				// Simple variable
				varType := "int"
				if knownType, exists := gen.variables[v]; exists {
					varType = knownType
				}

				// Also check constants map for type
				if constType, exists := gen.constants[v]; exists && varType == "int" {
					varType = constType
				}

				// Check if this is a global constant that needs getter
				if _, isConst := gen.globalConstantGetters[v]; isConst {
					// Apply cast if needed, then use getter function
					if varType == "intptr_t" {
						gen.output.WriteString("(char*)get_")
						gen.output.WriteString(v)
						gen.output.WriteString("()")
					} else if castType != "" {
						gen.output.WriteString(castType)
						gen.output.WriteString("get_")
						gen.output.WriteString(v)
						gen.output.WriteString("()")
					} else {
						gen.output.WriteString("get_")
						gen.output.WriteString(v)
						gen.output.WriteString("()")
					}
				} else if varType == "intptr_t" {
					gen.output.WriteString("(char*)")
					gen.output.WriteString(v)
				} else {
					// Apply cast if needed
					if castType != "" {
						gen.output.WriteString(castType)
					}
					gen.output.WriteString(v)
				}
			}
		}

		gen.output.WriteString(");\n")
		gen.writeIndent()
		gen.output.WriteString(bufferVar)
		gen.indent--
		gen.output.WriteString("; })")
	}
}

// Generate enum declaration
func (gen *CodeGenerator) generateEnum(node *ahoy.ASTNode) {
	enumName := node.Value

	// Skip if already generated
	if gen.generatedTypes[enumName] {
		return
	}
	gen.generatedTypes[enumName] = true

	enumType := node.EnumType

	// Track enum members for validation
	if gen.enums[enumName] == nil {
		gen.enums[enumName] = make(map[string]bool)
	}
	// Always mark as original name (in case it was created by another path)
	gen.enumOriginalNames[enumName] = true

	// Determine generation strategy based on type
	// If no type specified AND no explicit type, analyze members to determine type
	if enumType == "" || enumType == "int" {
		// Check if all members are compatible with int enum
		allInt := true
		for _, member := range node.Children {
			if len(member.Children) > 0 {
				if member.Children[0].Type != ahoy.NODE_NUMBER {
					allInt = false
					break
				}
				// Check if it's a float
				if strings.Contains(member.Children[0].Value, ".") {
					allInt = false
					break
				}
			}
		}

		if allInt && enumType == "int" {
			// Pure int enum - use C enum
			gen.generateIntEnum(node)
		} else if !allInt && enumType == "" {
			// Mixed types - use flexible struct
			gen.generateMixedEnum(node)
		} else {
			// enumType is explicitly "int" - use int enum
			gen.generateIntEnum(node)
		}
	} else if enumType == "string" {
		// Use struct for string enums
		gen.generateStringEnum(node)
	} else if enumType == "array" || enumType == "dict" {
		// Use struct for collection enums
		gen.generateCollectionEnum(node, enumType)
	} else if enumType == "float" {
		// Use struct for float enums
		gen.generateFloatEnum(node)
	} else if enumType == "color" || enumType == "vector2" {
		// Use struct for color/vector2 enums
		gen.generateColorEnum(node, enumType)
	} else {
		// Custom types or explicitly mixed - use flexible struct
		gen.generateMixedEnum(node)
	}
}

// Generate int enum using C typedef enum
func (gen *CodeGenerator) generateIntEnum(node *ahoy.ASTNode) {
	enumName := node.Value

	// Track enum type
	gen.enumTypes[enumName] = "int"

	// Write enum typedef to enumDecls
	gen.enumDecls.WriteString("typedef enum {\n")

	nextAutoValue := 0
	for _, member := range node.Children {
		// Track this member
		gen.enums[enumName][member.Value] = true
		// Track member type
		gen.enumMemberTypes[fmt.Sprintf("%s.%s", enumName, member.Value)] = "int"

		// Check if member has a custom value (in Children[0])
		if len(member.Children) > 0 && member.Children[0].Type == ahoy.NODE_NUMBER {
			value := member.Children[0].Value
			gen.enumDecls.WriteString("    ")
			gen.enumDecls.WriteString(enumName)
			gen.enumDecls.WriteString("_")
			gen.enumDecls.WriteString(member.Value)
			gen.enumDecls.WriteString(" = ")
			gen.enumDecls.WriteString(value)
			gen.enumDecls.WriteString(",\n")
			// Parse the value to set nextAutoValue for next member
			if val, err := strconv.Atoi(value); err == nil {
				nextAutoValue = val + 1
			}
		} else {
			// Auto-increment value
			gen.enumDecls.WriteString("    ")
			gen.enumDecls.WriteString(enumName)
			gen.enumDecls.WriteString("_")
			gen.enumDecls.WriteString(member.Value)
			gen.enumDecls.WriteString(" = ")
			gen.enumDecls.WriteString(strconv.Itoa(nextAutoValue))
			gen.enumDecls.WriteString(",\n")
			nextAutoValue++
		}
	}

	gen.enumDecls.WriteString("} ")
	gen.enumDecls.WriteString(enumName)
	gen.enumDecls.WriteString("_enum;\n\n")

	// Also generate a struct instance for member access (e.g., numbers.one)
	gen.generateEnumAccessStruct(node, "int")

	// Generate enum print helper
	gen.generateEnumPrintHelper(node, enumName, "int")
}

// Generate string enum using struct
func (gen *CodeGenerator) generateStringEnum(node *ahoy.ASTNode) {
	enumName := node.Value

	// Track enum type
	gen.enumTypes[enumName] = "string"

	// Generate struct typedef
	gen.writeIndent()
	gen.output.WriteString("typedef struct {\n")
	gen.indent++

	for _, member := range node.Children {
		gen.writeIndent()
		gen.enums[enumName][member.Value] = true
		// Track member type
		gen.enumMemberTypes[fmt.Sprintf("%s.%s", enumName, member.Value)] = "char*"

		// Check if member is mutable
		if member.IsMutable {
			gen.output.WriteString("char* ")
			gen.output.WriteString(member.Value)
			gen.output.WriteString(";\n")
		} else {
			gen.output.WriteString("const char* ")
			gen.output.WriteString(member.Value)
			gen.output.WriteString(";\n")
		}
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString("} ")
	gen.output.WriteString(enumName)
	gen.output.WriteString("_struct;\n\n")

	// Generate instance with initializer
	gen.writeIndent()
	gen.output.WriteString(enumName)
	gen.output.WriteString("_struct ")
	gen.output.WriteString(enumName)
	gen.output.WriteString(" = {\n")
	gen.indent++

	for _, member := range node.Children {
		gen.writeIndent()
		var value string
		if len(member.Children) > 0 && member.Children[0].Type == ahoy.NODE_STRING {
			// String value - make sure it has quotes
			rawValue := member.Children[0].Value
			if !strings.HasPrefix(rawValue, "\"") {
				value = fmt.Sprintf("\"%s\"", rawValue)
			} else {
				value = rawValue
			}
		} else {
			// Default value for string is empty string
			value = "\"\""
		}
		gen.output.WriteString(".")
		gen.output.WriteString(member.Value)
		gen.output.WriteString(" = ")
		gen.output.WriteString(value)
		gen.output.WriteString(",\n")
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString("};\n\n")
}

// Generate float enum using struct
func (gen *CodeGenerator) generateFloatEnum(node *ahoy.ASTNode) {
	enumName := node.Value

	gen.writeIndent()
	gen.output.WriteString("typedef struct {\n")
	gen.indent++

	for _, member := range node.Children {
		gen.writeIndent()
		gen.enums[enumName][member.Value] = true

		if member.IsMutable {
			gen.output.WriteString("float ")
			gen.output.WriteString(member.Value)
			gen.output.WriteString(";\n")
		} else {
			gen.output.WriteString("const float ")
			gen.output.WriteString(member.Value)
			gen.output.WriteString(";\n")
		}
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString("} ")
	gen.output.WriteString(enumName)
	gen.output.WriteString("_struct;\n\n")

	// Generate instance
	gen.writeIndent()
	gen.output.WriteString(enumName)
	gen.output.WriteString("_struct ")
	gen.output.WriteString(enumName)
	gen.output.WriteString(" = {\n")
	gen.indent++

	for _, member := range node.Children {
		gen.writeIndent()
		var value string
		if len(member.Children) > 0 && member.Children[0].Type == ahoy.NODE_NUMBER {
			value = member.Children[0].Value
		} else {
			value = "0.0"
		}
		gen.output.WriteString(".")
		gen.output.WriteString(member.Value)
		gen.output.WriteString(" = ")
		gen.output.WriteString(value)
		gen.output.WriteString(",\n")
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString("};\n\n")
}

// Generate color/vector2 enum using struct
func (gen *CodeGenerator) generateColorEnum(node *ahoy.ASTNode, enumType string) {
	enumName := node.Value

	// Track enum type
	gen.enumTypes[enumName] = enumType

	// Color and Vector2 types are defined globally

	gen.writeIndent()
	gen.output.WriteString("typedef struct {\n")
	gen.indent++

	for _, member := range node.Children {
		gen.writeIndent()
		gen.enums[enumName][member.Value] = true
		// Track member type
		gen.enumMemberTypes[fmt.Sprintf("%s.%s", enumName, member.Value)] = enumType

		// Check if member is mutable
		if member.IsMutable {
			gen.output.WriteString(enumType)
			gen.output.WriteString(" ")
			gen.output.WriteString(member.Value)
			gen.output.WriteString(";\n")
		} else {
			gen.output.WriteString("const ")
			gen.output.WriteString(enumType)
			gen.output.WriteString(" ")
			gen.output.WriteString(member.Value)
			gen.output.WriteString(";\n")
		}
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString("} ")
	gen.output.WriteString(enumName)
	gen.output.WriteString("_struct;\n\n")

	// Generate instance with initializer
	gen.writeIndent()
	gen.output.WriteString(enumName)
	gen.output.WriteString("_struct ")
	gen.output.WriteString(enumName)
	gen.output.WriteString(" = {\n")
	gen.indent++

	for _, member := range node.Children {
		gen.writeIndent()
		var value string
		if len(member.Children) > 0 && member.Children[0].Type == ahoy.NODE_OBJECT_LITERAL {
			// Has color/vector2 value
			valueNode := member.Children[0]
			if enumType == "color" && len(valueNode.Children) == 4 {
				// Color<r,g,b,a>
				r := valueNode.Children[0].Value
				g := valueNode.Children[1].Value
				b := valueNode.Children[2].Value
				a := valueNode.Children[3].Value
				value = fmt.Sprintf("(Color){%s, %s, %s, %s}", r, g, b, a)
			} else if enumType == "vector2" && len(valueNode.Children) == 2 {
				// Vector2<x,y>
				x := valueNode.Children[0].Value
				y := valueNode.Children[1].Value
				value = fmt.Sprintf("(Vector2){%s, %s}", x, y)
			} else {
				// Default to zero
				if enumType == "color" {
					value = "(Color){0, 0, 0, 0}"
				} else {
					value = "(Vector2){0, 0}"
				}
			}
		} else {
			// Default to zero
			if enumType == "color" {
				value = "(Color){0, 0, 0, 0}"
			} else {
				value = "(Vector2){0, 0}"
			}
		}
		gen.output.WriteString(".")
		gen.output.WriteString(member.Value)
		gen.output.WriteString(" = ")
		gen.output.WriteString(value)
		gen.output.WriteString(",\n")
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString("};\n\n")
}

// Generate collection (array/dict) enum using struct
func (gen *CodeGenerator) generateCollectionEnum(node *ahoy.ASTNode, enumType string) {
	enumName := node.Value
	cType := "AhoyArray*"
	if enumType == "dict" {
		cType = "HashMap*"
	}

	gen.writeIndent()
	gen.output.WriteString("typedef struct {\n")
	gen.indent++

	for _, member := range node.Children {
		gen.writeIndent()
		gen.enums[enumName][member.Value] = true
		// Collections are always mutable (pointer-based)
		gen.output.WriteString(cType)
		gen.output.WriteString(" ")
		gen.output.WriteString(member.Value)
		gen.output.WriteString(";\n")
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString("} ")
	gen.output.WriteString(enumName)
	gen.output.WriteString("_struct;\n\n")

	// Generate instance - initialized later or in init function
	gen.writeIndent()
	gen.output.WriteString(enumName)
	gen.output.WriteString("_struct ")
	gen.output.WriteString(enumName)
	gen.output.WriteString(";\n\n")

	// TODO: Generate initialization function for arrays/dicts
}

// Generate mixed enum using struct with generic types
func (gen *CodeGenerator) generateMixedEnum(node *ahoy.ASTNode) {
	enumName := node.Value

	// Track enum type
	gen.enumTypes[enumName] = "mixed"

	gen.writeIndent()
	gen.output.WriteString("typedef struct {\n")
	gen.indent++

	for _, member := range node.Children {
		gen.writeIndent()
		gen.enums[enumName][member.Value] = true

		// Infer type from value
		var memberType string
		if len(member.Children) > 0 {
			switch member.Children[0].Type {
			case ahoy.NODE_NUMBER:
				// Check if it's float or int
				if strings.Contains(member.Children[0].Value, ".") {
					memberType = "float"
				} else {
					memberType = "intptr_t"
				}
			case ahoy.NODE_STRING:
				memberType = "const char*"
			case ahoy.NODE_BOOLEAN:
				memberType = "int"
			case ahoy.NODE_ARRAY_LITERAL:
				memberType = "AhoyArray*"
			case ahoy.NODE_DICT_LITERAL:
				memberType = "HashMap*"
			default:
				memberType = "intptr_t" // generic fallback
			}
		} else {
			memberType = "intptr_t" // default
		}

		// Track member type for proper formatting in print
		gen.enumMemberTypes[fmt.Sprintf("%s.%s", enumName, member.Value)] = memberType

		// Make mutable if specified
		if member.IsMutable || memberType == "AhoyArray*" || memberType == "HashMap*" {
			gen.output.WriteString(memberType)
			gen.output.WriteString(" ")
			gen.output.WriteString(member.Value)
			gen.output.WriteString(";\n")
		} else {
			// Add const for immutable non-pointer types
			if !strings.Contains(memberType, "*") {
				gen.output.WriteString("const ")
				gen.output.WriteString(memberType)
				gen.output.WriteString(" ")
				gen.output.WriteString(member.Value)
				gen.output.WriteString(";\n")
			} else {
				gen.output.WriteString(memberType)
				gen.output.WriteString(" ")
				gen.output.WriteString(member.Value)
				gen.output.WriteString(";\n")
			}
		}
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString("} ")
	gen.output.WriteString(enumName)
	gen.output.WriteString("_struct;\n\n")

	// Generate instance with initializers
	gen.writeIndent()
	gen.output.WriteString(enumName)
	gen.output.WriteString("_struct ")
	gen.output.WriteString(enumName)
	gen.output.WriteString(" = {\n")
	gen.indent++

	for _, member := range node.Children {
		gen.writeIndent()
		var value string
		if len(member.Children) > 0 {
			switch member.Children[0].Type {
			case ahoy.NODE_NUMBER:
				value = member.Children[0].Value
			case ahoy.NODE_STRING:
				// Make sure string has quotes
				rawValue := member.Children[0].Value
				if !strings.HasPrefix(rawValue, "\"") {
					value = fmt.Sprintf("\"%s\"", rawValue)
				} else {
					value = rawValue
				}
			case ahoy.NODE_BOOLEAN:
				if member.Children[0].Value == "true" {
					value = "1"
				} else {
					value = "0"
				}
			case ahoy.NODE_ARRAY_LITERAL:
				// Generate array initialization inline
				arrayNode := member.Children[0]
				if len(arrayNode.Children) > 0 {
					// Create array literal
					tempBuf := &strings.Builder{}
					tempBuf.WriteString("({ AhoyArray* arr = malloc(sizeof(AhoyArray)); ")
					tempBuf.WriteString("arr->length = ")
					tempBuf.WriteString(strconv.Itoa(len(arrayNode.Children)))
					tempBuf.WriteString("; ")
					tempBuf.WriteString("arr->capacity = ")
					tempBuf.WriteString(strconv.Itoa(len(arrayNode.Children)))
					tempBuf.WriteString("; ")
					tempBuf.WriteString("arr->data = malloc(")
					tempBuf.WriteString(strconv.Itoa(len(arrayNode.Children)))
					tempBuf.WriteString(" * sizeof(intptr_t)); ")
					tempBuf.WriteString("arr->types = malloc(")
					tempBuf.WriteString(strconv.Itoa(len(arrayNode.Children)))
					tempBuf.WriteString(" * sizeof(AhoyValueType)); ")
					tempBuf.WriteString("arr->is_typed = 0; ")

					// Initialize elements
					for i, elem := range arrayNode.Children {
						if elem.Type == ahoy.NODE_NUMBER {
							tempBuf.WriteString("arr->data[")
							tempBuf.WriteString(strconv.Itoa(i))
							tempBuf.WriteString("] = ")
							tempBuf.WriteString(elem.Value)
							tempBuf.WriteString("; ")
							tempBuf.WriteString("arr->types[")
							tempBuf.WriteString(strconv.Itoa(i))
							tempBuf.WriteString("] = AHOY_TYPE_INT; ")
						}
					}

					tempBuf.WriteString("arr; })")
					value = tempBuf.String()
				} else {
					value = "NULL"
				}
			case ahoy.NODE_DICT_LITERAL:
				value = "NULL" // TODO: proper dict initialization
			default:
				value = "0"
			}
		} else {
			value = "0"
		}
		gen.output.WriteString(".")
		gen.output.WriteString(member.Value)
		gen.output.WriteString(" = ")
		gen.output.WriteString(value)
		gen.output.WriteString(",\n")
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString("};\n\n")
}

// Generate helper struct for enum member access (for int enums)
func (gen *CodeGenerator) generateEnumAccessStruct(node *ahoy.ASTNode, baseType string) {
	enumName := node.Value

	// Generate access struct typedef to enumDecls
	gen.enumDecls.WriteString("typedef struct {\n")

	for _, member := range node.Children {
		gen.enumDecls.WriteString("    const int ")
		gen.enumDecls.WriteString(member.Value)
		gen.enumDecls.WriteString(";\n")
	}

	gen.enumDecls.WriteString("} ")
	gen.enumDecls.WriteString(enumName)
	gen.enumDecls.WriteString("_struct;\n\n")

	// Generate instance to constantDecls (it's a global constant)
	gen.constantDecls.WriteString(enumName)
	gen.constantDecls.WriteString("_struct ")
	gen.constantDecls.WriteString(enumName)
	gen.constantDecls.WriteString(" = {\n")

	nextAutoValue := 0
	for _, member := range node.Children {
		var value int
		if len(member.Children) > 0 && member.Children[0].Type == ahoy.NODE_NUMBER {
			if val, err := strconv.Atoi(member.Children[0].Value); err == nil {
				value = val
				nextAutoValue = val + 1
			} else {
				value = nextAutoValue
				nextAutoValue++
			}
		} else {
			value = nextAutoValue
			nextAutoValue++
		}
		gen.constantDecls.WriteString("    .")
		gen.constantDecls.WriteString(member.Value)
		gen.constantDecls.WriteString(" = ")
		gen.constantDecls.WriteString(strconv.Itoa(value))
		gen.constantDecls.WriteString(",\n")
	}

	gen.constantDecls.WriteString("};\n\n")
}

// Generate enum print helper function
func (gen *CodeGenerator) generateEnumPrintHelper(node *ahoy.ASTNode, enumName string, enumType string) {
	// Generate a helper function that returns a string representation of the enum
	funcName := fmt.Sprintf("print_%s", enumName)

	gen.funcDecls.WriteString("char* ")
	gen.funcDecls.WriteString(funcName)
	gen.funcDecls.WriteString("() {\n")
	gen.funcDecls.WriteString("    char* buffer = malloc(512);\n")
	gen.funcDecls.WriteString("    int offset = 0;\n")
	gen.funcDecls.WriteString("    offset += sprintf(buffer + offset, \"enum:")
	gen.funcDecls.WriteString(enumType)
	gen.funcDecls.WriteString(" ")
	gen.funcDecls.WriteString(enumName)
	gen.funcDecls.WriteString("(\");\n")

	for i, member := range node.Children {
		if i > 0 {
			gen.funcDecls.WriteString("    offset += sprintf(buffer + offset, \", \");\n")
		}

		// Get member value
		var valueStr string
		if len(member.Children) > 0 && member.Children[0].Type == ahoy.NODE_NUMBER {
			valueStr = member.Children[0].Value
		} else {
			valueStr = fmt.Sprintf("%d", i) // Auto-value
		}

		gen.funcDecls.WriteString("    offset += sprintf(buffer + offset, \"")
		gen.funcDecls.WriteString(member.Value)
		gen.funcDecls.WriteString(":")
		gen.funcDecls.WriteString(valueStr)
		gen.funcDecls.WriteString("\");\n")
	}

	gen.funcDecls.WriteString("    offset += sprintf(buffer + offset, \")\");\n")
	gen.funcDecls.WriteString("    return buffer;\n")
	gen.funcDecls.WriteString("}\n\n")
}

// Generate constant declaration
func (gen *CodeGenerator) generateEnumDeclaration(node *ahoy.ASTNode) {
	constantName := node.Value
	value := node.Children[0]

	// Generate as #define
	gen.output.WriteString("#define ")
	gen.output.WriteString(constantName)
	gen.output.WriteString(" ")
	gen.generateNodeInternal(value, false)
	gen.output.WriteString("\n")
}

// Generate tuple assignment
// generateTupleSwitchAssignment handles tuple assignment from switch expressions
func (gen *CodeGenerator) generateTupleSwitchAssignment(leftSide *ahoy.ASTNode, switchNode *ahoy.ASTNode) {
	// Declare all left-side variables first
	for i, target := range leftSide.Children {
		if _, exists := gen.variables[target.Value]; !exists {
			// Infer type from first case of switch
			if len(switchNode.Children) > 1 {
				firstCase := switchNode.Children[1]
				if firstCase.Type == ahoy.NODE_SWITCH_CASE && len(firstCase.Children) > 1 {
					caseBody := firstCase.Children[1]
					// Case body should be a BLOCK node with tuple expressions
					if caseBody.Type == ahoy.NODE_BLOCK && i < len(caseBody.Children) {
						exprType := gen.inferType(caseBody.Children[i])
						cType := gen.mapType(exprType)
						gen.writeIndent()
						gen.output.WriteString(cType)
						gen.output.WriteString(" ")
						gen.output.WriteString(target.Value)
						gen.output.WriteString(";\n")
						gen.variables[target.Value] = exprType
						continue
					}
				}
			}
			// Fallback type
			gen.writeIndent()
			gen.output.WriteString("int ")
			gen.output.WriteString(target.Value)
			gen.output.WriteString(";\n")
			gen.variables[target.Value] = "int"
		}
	}

	// Generate switch with tuple assignments in each case
	switchExpr := switchNode.Children[0]
	switchExprType := gen.inferType(switchExpr)

	// Check if this is a string switch
	if switchExprType == "char*" || switchExprType == "string" {
		gen.generateTupleStringSwitchExpression(switchNode, leftSide)
		return
	}

	// Generate normal switch with tuple assignments
	gen.writeIndent()
	gen.output.WriteString("switch (")
	gen.generateNode(switchExpr)
	gen.output.WriteString(") {\n")

	// Generate cases
	for i := 1; i < len(switchNode.Children); i++ {
		caseNode := switchNode.Children[i]
		if caseNode.Type == ahoy.NODE_SWITCH_CASE {
			caseValue := caseNode.Children[0]
			caseBody := caseNode.Children[1]

			// Generate case label
			gen.indent++
			gen.writeIndent()
			if caseValue.Type == ahoy.NODE_IDENTIFIER && caseValue.Value == "_" {
				gen.output.WriteString("default:\n")
			} else {
				gen.output.WriteString("case ")
				gen.generateNode(caseValue)
				gen.output.WriteString(":\n")
			}

			gen.indent++
			// Generate tuple assignments
			if caseBody.Type == ahoy.NODE_BLOCK {
				for j, expr := range caseBody.Children {
					if j < len(leftSide.Children) {
						gen.writeIndent()
						gen.output.WriteString(leftSide.Children[j].Value)
						gen.output.WriteString(" = ")
						gen.generateNode(expr)
						gen.output.WriteString(";\n")
					}
				}
			}
			gen.writeIndent()
			gen.output.WriteString("break;\n")
			gen.indent--
			gen.indent--
		}
	}

	gen.writeIndent()
	gen.output.WriteString("}\n")
}

// generateTupleStringSwitchExpression handles tuple assignment from string switch
func (gen *CodeGenerator) generateTupleStringSwitchExpression(switchNode *ahoy.ASTNode, leftSide *ahoy.ASTNode) {
	switchExpr := switchNode.Children[0]

	first := true
	hasDefault := false
	var defaultBody *ahoy.ASTNode

	for i := 1; i < len(switchNode.Children); i++ {
		caseNode := switchNode.Children[i]
		if caseNode.Type == ahoy.NODE_SWITCH_CASE {
			caseValue := caseNode.Children[0]
			caseBody := caseNode.Children[1]

			// Check for default case
			if caseValue.Type == ahoy.NODE_IDENTIFIER && caseValue.Value == "_" {
				hasDefault = true
				defaultBody = caseBody
				continue
			}

			gen.writeIndent()
			if first {
				gen.output.WriteString("if (")
				first = false
			} else {
				gen.output.WriteString("else if (")
			}

			gen.output.WriteString("strcmp(")
			gen.generateNode(switchExpr)
			gen.output.WriteString(", ")
			gen.generateNode(caseValue)
			gen.output.WriteString(") == 0) {\n")

			gen.indent++
			// Generate tuple assignments
			if caseBody.Type == ahoy.NODE_BLOCK {
				for j, expr := range caseBody.Children {
					if j < len(leftSide.Children) {
						gen.writeIndent()
						gen.output.WriteString(leftSide.Children[j].Value)
						gen.output.WriteString(" = ")
						gen.generateNode(expr)
						gen.output.WriteString(";\n")
					}
				}
			}
			gen.indent--
			gen.writeIndent()
			gen.output.WriteString("}")
		}
	}

	// Handle default case
	if hasDefault {
		gen.output.WriteString(" else {\n")
		gen.indent++
		if defaultBody.Type == ahoy.NODE_BLOCK {
			for j, expr := range defaultBody.Children {
				if j < len(leftSide.Children) {
					gen.writeIndent()
					gen.output.WriteString(leftSide.Children[j].Value)
					gen.output.WriteString(" = ")
					gen.generateNode(expr)
					gen.output.WriteString(";\n")
				}
			}
		}
		gen.indent--
		gen.writeIndent()
		gen.output.WriteString("}\n")
	} else {
		gen.output.WriteString("\n")
	}
}

func (gen *CodeGenerator) generateTupleAssignment(node *ahoy.ASTNode) {
	leftSide := node.Children[0]
	rightSide := node.Children[1]

	// Check if right side is a single function call that returns multiple values
	if len(rightSide.Children) == 1 && rightSide.Children[0].Type == ahoy.NODE_CALL {
		callNode := rightSide.Children[0]
		funcName := callNode.Value

		// Generate the function call into a temp struct
		tempVar := fmt.Sprintf("__multi_ret_%d", gen.varCounter)
		gen.varCounter++

		gen.writeIndent()
		// Special case: read_json uses json_read_return struct
		structName := funcName
		if funcName == "read_json" {
			structName = "json_read"
		}
		gen.output.WriteString(structName)
		gen.output.WriteString("_return ")
		gen.output.WriteString(tempVar)
		gen.output.WriteString(" = ")
		gen.generateNode(callNode)
		gen.output.WriteString(";\n")

		// Special handling for read_json - track that first return value is AhoyJSON*
		if funcName == "read_json" && len(leftSide.Children) >= 1 {
			jsonVarName := leftSide.Children[0].Value
			if jsonVarName != "_" { // Don't track underscore placeholders
				gen.jsonVariables[jsonVarName] = true // Track this as a JSON variable
			}
		}

		// Assign struct fields to left side variables
		for i, target := range leftSide.Children {
			// Skip underscore placeholders - they indicate the value should be discarded
			if target.Value == "_" {
				continue
			}

			gen.writeIndent()
			// Check if variable needs to be declared in CURRENT scope
			// Variables can shadow between scopes (global vs function)
			existsInCurrentScope := false
			if gen.functionVars != nil {
				// In a function - only check function scope
				_, existsInCurrentScope = gen.functionVars[target.Value]
			} else {
				// In global scope - check if it's been declared in C code already
				// Don't check gen.variables as it may contain function-local variables
				if gen.currentFunction == "" {
					_, existsInCurrentScope = gen.declaredGlobalVars[target.Value]
				}
			}

			if !existsInCurrentScope {
				// Need to declare variable - look up function return types
				cType := "int" // default
				inferredType := "int"
				needsCast := false

				// Special case for read_json return values
				if funcName == "read_json" {
					if i == 0 {
						cType = "AhoyJSON*"
						inferredType = "AhoyJSON*"
						// Track as JSON variable
						gen.jsonVariables[target.Value] = true
					} else if i == 1 {
						cType = "char*"
						inferredType = "char*"
					}
				} else if retTypes, ok := gen.functionReturnTypes[funcName]; ok && i < len(retTypes) {
					// If return type is "generic" or "any", infer from actual call arguments
					if (retTypes[i] == "generic" || retTypes[i] == "any") && i < len(callNode.Children) {
						inferredType = gen.inferType(callNode.Children[i])
						needsCast = true // Need to cast from intptr_t
					} else {
						inferredType = retTypes[i]
					}

					cType = gen.mapType(inferredType)
					if gen.functionVars != nil {
						gen.functionVars[target.Value] = inferredType
						gen.declaredFunctionVars[target.Value] = true
						// Track heap-allocated return values at function level
						if gen.indent == 1 && gen.isHeapAllocatedType(inferredType) {
							gen.heapAllocatedVars[target.Value] = true
						}
					} else {
						gen.variables[target.Value] = inferredType
						gen.declaredGlobalVars[target.Value] = true
					}
					// Track JSON variables
					if inferredType == "AhoyJSON*" {
						gen.jsonVariables[target.Value] = true
					}
				} else {
					if gen.functionVars != nil {
						gen.functionVars[target.Value] = "int"
						gen.declaredFunctionVars[target.Value] = true
					} else {
						gen.variables[target.Value] = "int"
						gen.declaredGlobalVars[target.Value] = true
					}
				}
				gen.output.WriteString(cType)
				gen.output.WriteString(" ")

				// If we need to cast from intptr_t (for generic types), do it here
				if needsCast {
					if cType == "char*" {
						gen.output.WriteString(target.Value)
						gen.output.WriteString(" = (char*)")
						gen.output.WriteString(tempVar)
						gen.output.WriteString(".ret")
						gen.output.WriteString(strconv.Itoa(i))
						gen.output.WriteString(";\n")
					} else {
						gen.output.WriteString(target.Value)
						gen.output.WriteString(" = (")
						gen.output.WriteString(cType)
						gen.output.WriteString(")")
						gen.output.WriteString(tempVar)
						gen.output.WriteString(".ret")
						gen.output.WriteString(strconv.Itoa(i))
						gen.output.WriteString(";\n")
					}
				} else {
					gen.output.WriteString(target.Value)
					gen.output.WriteString(" = ")
					gen.output.WriteString(tempVar)
					gen.output.WriteString(".ret")
					gen.output.WriteString(strconv.Itoa(i))
					gen.output.WriteString(";\n")
				}
			} else {
				gen.output.WriteString(target.Value)
				gen.output.WriteString(" = ")
				gen.output.WriteString(tempVar)
				gen.output.WriteString(".ret")
				gen.output.WriteString(strconv.Itoa(i))
				gen.output.WriteString(";\n")
			}
		}
		return
	}

	// Check if right side is a single switch statement returning a tuple
	if len(rightSide.Children) == 1 && rightSide.Children[0].Type == ahoy.NODE_SWITCH_STATEMENT {
		gen.generateTupleSwitchAssignment(leftSide, rightSide.Children[0])
		return
	}

	// Generate temporary variables for regular tuple assignment
	temps := make([]string, len(rightSide.Children))
	for i, expr := range rightSide.Children {
		tempVar := fmt.Sprintf("__temp_%d", gen.varCounter)
		gen.varCounter++
		temps[i] = tempVar

		// Infer type from the expression
		exprType := gen.inferType(expr)
		cType := gen.mapType(exprType)
		gen.writeIndent()
		gen.output.WriteString(cType)
		gen.output.WriteString(" ")
		gen.output.WriteString(tempVar)
		gen.output.WriteString(" = ")
		gen.generateNodeInternal(expr, false)
		gen.output.WriteString(";\n")
	}

	// Assign temps to left side variables
	for i, target := range leftSide.Children {
		if i < len(temps) {
			gen.writeIndent()
			// Check if variable needs to be declared
			existsInFunc := false
			existsGlobal := false
			if gen.functionVars != nil {
				_, existsInFunc = gen.functionVars[target.Value]
			}
			_, existsGlobal = gen.variables[target.Value]

			if !existsInFunc && !existsGlobal {
				// Need to declare variable - infer type from temp
				tempType := gen.inferType(rightSide.Children[i])
				cType := gen.mapType(tempType)
				gen.output.WriteString(cType)
				gen.output.WriteString(" ")
				if gen.functionVars != nil {
					gen.functionVars[target.Value] = tempType
				} else {
					gen.variables[target.Value] = tempType
				}
			}
			gen.output.WriteString(target.Value)
			gen.output.WriteString(" = ")
			gen.output.WriteString(temps[i])
			gen.output.WriteString(";\n")
		}
	}
}

// Generate struct declaration
func (gen *CodeGenerator) generateStruct(node *ahoy.ASTNode) {
	structName := node.Value

	// Skip if already generated
	if gen.generatedTypes[structName] {
		return
	}
	gen.generatedTypes[structName] = true

	// Handle JSON structs - just store schema, don't generate C code
	if node.DataType == "json" {
		structInfo := &StructInfo{
			Name:   structName,
			Fields: make([]StructField, 0),
		}

		for _, field := range node.Children {
			if field.Type != ahoy.NODE_TYPE {
				fieldType := field.DataType
				if fieldType == "" {
					fieldType = "string" // Default to string for JSON
				}
				structInfo.Fields = append(structInfo.Fields, StructField{
					Name: field.Value,
					Type: fieldType,
				})
			}
		}

		// Store in structs map for type validation
		gen.structs[structName] = structInfo
		gen.structs["json_"+structName] = structInfo // Also store with json_ prefix
		// Mark as JSON struct so we don't generate C code for it
		gen.jsonStructs[structName] = true
		gen.jsonStructs["json_"+structName] = true
		return
	}

	// Check if vector2 or color are already defined from C imports
	// Only skip if they're actually registered as C types
	cStructName := capitalizeFirst(structName)
	if (structName == "vector2" || structName == "color") && gen.cTypeDefinitions[cStructName] {
		// Already defined from C header - just register struct info for type checking
		structInfo := &StructInfo{
			Name:   structName,
			Fields: make([]StructField, 0),
		}

		// Add fields for tracking
		for _, field := range node.Children {
			if field.Type != ahoy.NODE_TYPE {
				fieldType := gen.mapType(field.DataType)
				defaultValue := gen.generateDefaultValue(field.DefaultValue)
				structInfo.Fields = append(structInfo.Fields, StructField{
					Name:         field.Value,
					Type:         fieldType,
					DefaultValue: defaultValue,
				})
			}
		}

		gen.structs[structName] = structInfo
		gen.structs[cStructName] = structInfo
		return
	}

	// Separate regular fields from nested types
	var baseFields []*ahoy.ASTNode
	var nestedTypes []*ahoy.ASTNode

	for _, child := range node.Children {
		if child.Type == ahoy.NODE_TYPE {
			nestedTypes = append(nestedTypes, child)
		} else {
			baseFields = append(baseFields, child)
		}
	}

	// Generate nested types first (they inherit from base struct)
	for _, nestedType := range nestedTypes {
		gen.generateNestedStruct(nestedType, structName, baseFields)
	}

	// Generate base struct - write to structDecls instead of output
	// cStructName already declared above
	structInfo := &StructInfo{
		Name:   structName,
		Fields: make([]StructField, 0),
	}

	gen.structDecls.WriteString("typedef struct {\n")

	// Track static fields separately - they will be generated as global variables
	var staticFields []*ahoy.ASTNode

	for _, field := range baseFields {
		// Skip static fields - they are not part of the struct
		if field.IsStatic {
			staticFields = append(staticFields, field)
			continue
		}

		fieldType := gen.mapType(field.DataType)
		gen.structDecls.WriteString("    ")
		gen.structDecls.WriteString(fieldType)
		gen.structDecls.WriteString(" ")
		gen.structDecls.WriteString(field.Value)
		gen.structDecls.WriteString(";\n")

		// Track field info with default value
		defaultValue := gen.generateDefaultValue(field.DefaultValue)
		structInfo.Fields = append(structInfo.Fields, StructField{
			Name:         field.Value,
			Type:         fieldType,
			AhoyType:     field.DataType,
			DefaultValue: defaultValue,
			IsStatic:     false,
			IsConst:      field.IsConst,
		})
	}

	gen.structDecls.WriteString("} ")
	gen.structDecls.WriteString(cStructName)
	gen.structDecls.WriteString(";\n\n")

	// Generate static fields as global variables
	for _, field := range staticFields {
		fieldType := gen.mapType(field.DataType)
		defaultValue := "0"
		if field.DefaultValue != nil {
			defaultValue = gen.generateDefaultValue(field.DefaultValue)
		}
		// Static field name format: StructName_fieldname
		staticFieldName := fmt.Sprintf("%s_%s", cStructName, field.Value)
		gen.structDecls.WriteString("static ")
		gen.structDecls.WriteString(fieldType)
		gen.structDecls.WriteString(" ")
		gen.structDecls.WriteString(staticFieldName)
		gen.structDecls.WriteString(" = ")
		gen.structDecls.WriteString(defaultValue)
		gen.structDecls.WriteString(";\n")

		// Also track in struct info for type checking (marked as static)
		structInfo.Fields = append(structInfo.Fields, StructField{
			Name:         field.Value,
			Type:         fieldType,
			DefaultValue: defaultValue,
			IsStatic:     true,
			IsConst:      field.IsConst,
		})
	}
	if len(staticFields) > 0 {
		gen.structDecls.WriteString("\n")
	}

	// Store struct info with both lowercase and capitalized names
	gen.structs[structName] = structInfo
	gen.structs[cStructName] = structInfo
}

// Helper to generate C code for a default value
func (gen *CodeGenerator) generateDefaultValue(node *ahoy.ASTNode) string {
	if node == nil {
		return ""
	}

	switch node.Type {
	case ahoy.NODE_NUMBER:
		return node.Value
	case ahoy.NODE_STRING:
		return fmt.Sprintf("\"%s\"", node.Value)
	case ahoy.NODE_BOOLEAN:
		if node.Value == "true" {
			return "true"
		}
		return "false"
	case ahoy.NODE_CALL:
		// Handle old-style function calls (backward compatibility)
		if node.Value == "vector2" && len(node.Children) == 2 {
			x := gen.generateDefaultValue(node.Children[0])
			y := gen.generateDefaultValue(node.Children[1])
			return fmt.Sprintf("(Vector2){.x = %s, .y = %s}", x, y)
		}
		if node.Value == "color" && len(node.Children) == 4 {
			r := gen.generateDefaultValue(node.Children[0])
			g := gen.generateDefaultValue(node.Children[1])
			b := gen.generateDefaultValue(node.Children[2])
			a := gen.generateDefaultValue(node.Children[3])
			return fmt.Sprintf("(Color){.r = %s, .g = %s, .b = %s, .a = %s}", r, g, b, a)
		}
	case ahoy.NODE_OBJECT_LITERAL:
		// Handle object literal default values like Color{r:10, g:20, b:30, a:255}
		if node.Value != "" {
			// Typed object literal - use mapType to get correct C type
			typeName := gen.mapType(node.Value)
			var builder strings.Builder
			builder.WriteString("(")
			builder.WriteString(typeName)
			builder.WriteString("){")

			first := true
			for _, prop := range node.Children {
				if prop.Type == ahoy.NODE_OBJECT_PROPERTY {
					if !first {
						builder.WriteString(", ")
					}
					builder.WriteString(".")
					builder.WriteString(prop.Value)
					builder.WriteString(" = ")
					if len(prop.Children) > 0 {
						builder.WriteString(gen.generateDefaultValue(prop.Children[0]))
					}
					first = false
				}
			}
			builder.WriteString("}")
			return builder.String()
		}
	case ahoy.NODE_ARRAY_LITERAL:
		// Generate array literal inline
		var builder strings.Builder
		dictName := fmt.Sprintf("arr_%d", gen.dictCounter)
		gen.dictCounter++
		builder.WriteString("({ AhoyArray* ")
		builder.WriteString(dictName)
		builder.WriteString(" = malloc(sizeof(AhoyArray)); ")
		builder.WriteString(dictName)
		builder.WriteString("->length = 0; ")
		builder.WriteString(dictName)
		builder.WriteString("->capacity = 0; ")
		builder.WriteString(dictName)
		builder.WriteString("->data = malloc(0 * sizeof(intptr_t)); ")
		builder.WriteString(dictName)
		builder.WriteString("->types = malloc(0 * sizeof(AhoyValueType)); ")
		builder.WriteString(dictName)
		builder.WriteString("->is_typed = 0; ")
		for _, elem := range node.Children {
			builder.WriteString("ahoy_array_push(")
			builder.WriteString(dictName)
			builder.WriteString(", (intptr_t)")
			builder.WriteString(gen.generateDefaultValue(elem))
			valueType := gen.inferType(elem)
			builder.WriteString(", ")
			builder.WriteString(gen.getAhoyTypeEnum(valueType))
			builder.WriteString("); ")
		}
		builder.WriteString(dictName)
		builder.WriteString("; })")
		return builder.String()
	case ahoy.NODE_DICT_LITERAL:
		// Generate dict literal inline
		var builder strings.Builder
		dictName := fmt.Sprintf("dict_%d", gen.dictCounter)
		gen.dictCounter++
		builder.WriteString("({ HashMap* ")
		builder.WriteString(dictName)
		builder.WriteString(" = createHashMap(16); ")
		for i := 0; i < len(node.Children); i += 2 {
			if i+1 < len(node.Children) {
				key := node.Children[i]
				value := node.Children[i+1]
				builder.WriteString("hashMapPutTyped(")
				builder.WriteString(dictName)
				builder.WriteString(", ")
				builder.WriteString(gen.generateDefaultValue(key))
				builder.WriteString(", (void*)(intptr_t)")
				builder.WriteString(gen.generateDefaultValue(value))
				valueType := gen.inferType(value)
				builder.WriteString(", ")
				builder.WriteString(gen.getAhoyTypeEnum(valueType))
				builder.WriteString("); ")
			}
		}
		builder.WriteString(dictName)
		builder.WriteString("; })")
		return builder.String()
	}
	return ""
}

// Get default value for a type
func (gen *CodeGenerator) getTypeDefault(cType string) string {
	switch cType {
	case "int":
		return "0"
	case "double", "float":
		return "0.0"
	case "char*", "const char*":
		return "\"\""
	case "bool":
		return "false"
	case "Vector2":
		return "(Vector2){.x = 0, .y = 0}"
	case "Color":
		return "(Color){.r = 0, .g = 0, .b = 0, .a = 0}"
	case "AhoyArray*":
		return "({ AhoyArray* arr = malloc(sizeof(AhoyArray)); arr->length = 0; arr->capacity = 0; arr->data = malloc(0 * sizeof(intptr_t)); arr->types = malloc(0 * sizeof(AhoyValueType)); arr->is_typed = 0; arr; })"
	case "HashMap*":
		return "createHashMap(16)"
	default:
		return ""
	}
}

// Generate a nested struct type that inherits fields from parent
func (gen *CodeGenerator) generateNestedStruct(node *ahoy.ASTNode, parentName string, parentFields []*ahoy.ASTNode) {
	typeName := node.Value
	fullTypeName := parentName + "." + typeName // Full name like Card.Assassin
	// Use short name for C type (preserving original behavior)
	cTypeName := capitalizeFirst(typeName) // Assassin for C
	// Also create the full C type name for explicit Parent.Child syntax
	fullCTypeName := capitalizeFirst(parentName) + "_" + capitalizeFirst(typeName) // Card_Assassin

	// Track struct info
	structInfo := &StructInfo{
		Name:   typeName, // Use short name for print output
		Fields: make([]StructField, 0),
	}

	// Build a set of field names defined in the nested type (for override detection)
	nestedFieldNames := make(map[string]bool)
	for _, field := range node.Children {
		nestedFieldNames[field.Value] = true
	}

	gen.structDecls.WriteString("typedef struct {\n")

	// First, include parent fields (skip those overridden by nested type)
	for _, field := range parentFields {
		// Skip if the nested type overrides this field
		if nestedFieldNames[field.Value] {
			continue
		}

		fieldType := gen.mapType(field.DataType)
		gen.structDecls.WriteString("    ")
		gen.structDecls.WriteString(fieldType)
		gen.structDecls.WriteString(" ")
		gen.structDecls.WriteString(field.Value)
		gen.structDecls.WriteString(";\n")

		// Track field info with default value
		defaultValue := ""
		if field.DefaultValue != nil {
			defaultValue = gen.generateDefaultValue(field.DefaultValue)
		} else {
			defaultValue = gen.getTypeDefault(fieldType)
		}

		structInfo.Fields = append(structInfo.Fields, StructField{
			Name:         field.Value,
			Type:         fieldType,
			DefaultValue: defaultValue,
		})
	}

	// Then, add nested type's own fields
	for _, field := range node.Children {
		fieldType := gen.mapType(field.DataType)
		gen.structDecls.WriteString("    ")
		gen.structDecls.WriteString(fieldType)
		gen.structDecls.WriteString(" ")
		gen.structDecls.WriteString(field.Value)
		gen.structDecls.WriteString(";\n")

		// Track field info with default value if present
		defaultValue := ""
		if field.DefaultValue != nil {
			defaultValue = gen.generateDefaultValue(field.DefaultValue)
		} else {
			// Apply type-specific defaults
			defaultValue = gen.getTypeDefault(fieldType)
		}

		structInfo.Fields = append(structInfo.Fields, StructField{
			Name:         field.Value,
			Type:         fieldType,
			DefaultValue: defaultValue,
		})
	}

	// Generate typedef with short name (original behavior)
	gen.structDecls.WriteString("} ")
	gen.structDecls.WriteString(cTypeName)
	gen.structDecls.WriteString(";\n\n")

	// Also generate a typedef alias for the full name syntax
	gen.structDecls.WriteString("typedef ")
	gen.structDecls.WriteString(cTypeName)
	gen.structDecls.WriteString(" ")
	gen.structDecls.WriteString(fullCTypeName)
	gen.structDecls.WriteString(";\n\n")

	// Store struct info with multiple names for lookup
	gen.structs[typeName] = structInfo
	gen.structs[cTypeName] = structInfo
	gen.structs[fullTypeName] = structInfo
	gen.structs[fullCTypeName] = structInfo
}

// Generate method call

// Generate member access
// isDictSizeMember reports whether a member name asks for a collection's entry
// count rather than a key stored in it. A dict answers `length`/`size` with
// HashMap::size; anything else is a key lookup.
func isDictSizeMember(name string) bool {
	return name == "length" || name == "size"
}

func (gen *CodeGenerator) generateMemberAccess(node *ahoy.ASTNode) {
	object := node.Children[0]
	memberName := node.Value

	// Check if this is enum member access (enum_name.MEMBER)
	if object.Type == ahoy.NODE_IDENTIFIER {
		// Check if the identifier is an enum name (try both as-is and lowercase)
		enumName := object.Value
		isEnum := gen.isEnumType(enumName)

		// If not found, try lowercase version
		if !isEnum {
			lowerName := strings.ToLower(string(enumName[0])) + enumName[1:]
			if gen.isEnumType(lowerName) {
				enumName = lowerName
				isEnum = true
			}
		}

		if isEnum {
			// Always use the original enum name (should be lowercase)
			// Double-check that we have the original name
			if !gen.enumOriginalNames[enumName] {
				// This might be a capitalized version, try lowercase
				lowerName := strings.ToLower(string(enumName[0])) + enumName[1:]
				if gen.enumOriginalNames[lowerName] {
					enumName = lowerName
				}
			}

			enumType := gen.enumTypes[enumName]

			// For int enums, use the C enum format: enum_name_MEMBER
			// This is needed for switch cases and constant expressions
			if enumType == "int" {
				gen.output.WriteString(enumName)
				gen.output.WriteString("_")
				gen.output.WriteString(memberName)
				return
			}

			// For other enum types (string, etc.), use struct member access: enum_name.member
			gen.output.WriteString(enumName)
			gen.output.WriteString(".")
			gen.output.WriteString(memberName)
			return
		}
	}

	// Check if object is a HashMap (anonymous object) - need special handling
	objectType := gen.inferType(object)

	// Check if this is JSON object access
	if objectType == "AhoyJSON*" || objectType == "json" {
		// JSON object - use ahoy_json_get
		gen.output.WriteString("ahoy_json_get(")
		gen.generateNodeInternal(object, false)
		gen.output.WriteString(", \"")
		gen.output.WriteString(memberName)
		gen.output.WriteString("\")")
		return
	}

	// A string answers length/count with its character count. The generic path
	// below emitted `s.length` in C, which is not a member of char*.
	if (memberName == "length" || memberName == "count") && gen.isStringExpr(object) {
		gen.stringMethods["length"] = true
		gen.output.WriteString("ahoy_string_length(")
		gen.generateNodeInternal(object, false)
		gen.output.WriteString(")")
		return
	}

	if objectType == "HashMap*" || objectType == "dict" {
		// A dict answers length/size with its entry count. Without this the member
		// was looked up as a key, which printed an empty value for `d.length`.
		if isDictSizeMember(memberName) {
			gen.output.WriteString("(")
			gen.generateNodeInternal(object, false)
			gen.output.WriteString(")->size")
			return
		}
		// Anonymous object stored in HashMap - use hashMapGet
		// Note: returns void*, caller needs to cast appropriately
		gen.output.WriteString("hashMapGet(")
		gen.generateNodeInternal(object, false)
		gen.output.WriteString(", \"")
		gen.output.WriteString(memberName)
		gen.output.WriteString("\")")
		return
	}

	// Special handling for chained array access (2D arrays) with member access
	// e.g., card_grid[0][1].suit -> need to cast the element to proper struct type
	if object.Type == ahoy.NODE_ARRAY_ACCESS && object.Value == "" && len(object.Children) == 2 {
		// This is a chained array access: arr[i][j]
		// We need to determine the element type and cast properly
		innerAccess := object.Children[0]
		outerIndex := object.Children[1]

		// Try to find the nested element type (what the inner arrays contain)
		var elemType string
		if innerAccess.Type == ahoy.NODE_ARRAY_ACCESS && innerAccess.Value != "" {
			// The inner access is grid[i] where grid is the 2D array name
			// Look up what the inner arrays contain
			if et, exists := gen.array2DElementTypes[innerAccess.Value]; exists {
				elemType = et
			}
		}

		if elemType != "" {
			cType := gen.mapType(elemType)
			// Check if it's a struct
			isStruct := false
			if _, exists := gen.structs[elemType]; exists {
				isStruct = true
			} else if _, exists := gen.structs[cType]; exists {
				isStruct = true
			}

			if isStruct {
				// Generate: ((StructType*)((AhoyArray*)inner_access)->data[outer_idx])->member
				gen.output.WriteString("((")
				gen.output.WriteString(cType)
				gen.output.WriteString("*)((AhoyArray*)")
				gen.generateNode(innerAccess)
				gen.output.WriteString(")->data[")
				gen.generateNode(outerIndex)
				gen.output.WriteString("])->")
				gen.output.WriteString(memberName)
				return
			}
		}
	}

	gen.generateNodeInternal(object, false)

	// Check if object is a pointer type (array or struct pointer)
	if objectType == "AhoyArray*" || objectType == "array" ||
		strings.HasPrefix(objectType, "array[") ||
		strings.HasSuffix(objectType, "*") {
		gen.output.WriteString("->")
	} else {
		gen.output.WriteString(".")
	}
	gen.output.WriteString(memberName)
}

// generateStaticMemberAccess handles StructType.#static_field access
func (gen *CodeGenerator) generateStaticMemberAccess(node *ahoy.ASTNode) {
	object := node.Children[0]
	memberName := node.Value

	// Get the struct type name
	structTypeName := ""
	if object.Type == ahoy.NODE_IDENTIFIER {
		structTypeName = object.Value
	}

	if structTypeName == "" {
		// Fallback - just use the identifier
		gen.generateNodeInternal(object, false)
		gen.output.WriteString("_")
		gen.output.WriteString(memberName)
		return
	}

	// Generate the static variable name: StructName_fieldname
	// Use capitalizeFirst to match struct naming convention
	cStructName := capitalizeFirst(structTypeName)
	staticFieldName := fmt.Sprintf("%s_%s", cStructName, memberName)
	gen.output.WriteString(staticFieldName)
}

func (gen *CodeGenerator) generateTypeProperty(node *ahoy.ASTNode) {
	// Generate code to return type string for .type property
	object := node.Children[0]
	objectName := ""

	// Extract object name/identifier
	if object.Type == ahoy.NODE_IDENTIFIER {
		objectName = object.Value
	} else if object.Type == ahoy.NODE_MEMBER_ACCESS {
		// For enum.member.type
		if len(object.Children) > 0 && object.Children[0].Type == ahoy.NODE_IDENTIFIER {
			enumName := object.Children[0].Value
			memberName := object.Value
			if gen.isEnumType(enumName) {
				// Get the member type
				memberKey := fmt.Sprintf("%s.%s", enumName, memberName)
				if memberType, exists := gen.enumMemberTypes[memberKey]; exists {
					ahoyType := gen.cTypeToAhoyType(memberType)
					gen.output.WriteString("\"")
					gen.output.WriteString(ahoyType)
					gen.output.WriteString("\"")
					return
				}
			}
		}
	}

	// Check if this is an enum type itself (enum.type)
	if object.Type == ahoy.NODE_IDENTIFIER && gen.isEnumType(objectName) {
		if enumType, exists := gen.enumTypes[objectName]; exists {
			// For mixed enums, just print "enum"
			if enumType == "mixed" || enumType == "" {
				gen.output.WriteString("\"enum\"")
			} else {
				gen.output.WriteString("\"enum:")
				gen.output.WriteString(enumType)
				gen.output.WriteString("\"")
			}
			return
		}
	}

	// Generate inline expression that returns type string
	gen.output.WriteString("({")
	gen.output.WriteString("char* __type_str = malloc(64); ")

	// Check variable type to determine how to get type info
	varType := gen.inferType(object)

	if varType == "array" || varType == "AhoyArray*" || strings.HasPrefix(varType, "array[") {
		// Array type - check if typed
		gen.output.WriteString("if (")
		gen.output.WriteString(objectName)
		gen.output.WriteString(" != NULL && ")
		gen.output.WriteString(objectName)
		gen.output.WriteString("->is_typed) { ")
		gen.output.WriteString("const char* elem_type = ahoy_type_enum_to_string(")
		gen.output.WriteString(objectName)
		gen.output.WriteString("->element_type); ")
		gen.output.WriteString("sprintf(__type_str, \"array[%s]\", elem_type); ")
		gen.output.WriteString("} else { ")
		gen.output.WriteString("strcpy(__type_str, \"array\"); ")
		gen.output.WriteString("} ")
	} else if varType == "dict" || varType == "HashMap*" {
		// Dict type - for now just return "dict"
		// TODO: Add typed dict support
		gen.output.WriteString("strcpy(__type_str, \"dict\"); ")
	} else {
		// Other types - convert C type back to Ahoy type name
		ahoyType := gen.cTypeToAhoyType(varType)
		gen.output.WriteString("strcpy(__type_str, \"")
		gen.output.WriteString(ahoyType)
		gen.output.WriteString("\"); ")
	}

	gen.output.WriteString("__type_str; ")
	gen.output.WriteString("})")
}

// cTypeToAhoyType converts C type names back to Ahoy type names
func (gen *CodeGenerator) cTypeToAhoyType(cType string) string {
	switch cType {
	case "char*":
		return "string"
	case "int":
		return "int"
	case "double", "float":
		return "float"
	case "bool":
		return "bool"
	case "AhoyArray*":
		return "array"
	case "HashMap*":
		return "dict"
	default:
		if strings.HasPrefix(cType, "array[") {
			return cType // Already in Ahoy format
		}
		return cType
	}
}

// Helper function to convert AhoyValueType enum to string
func (gen *CodeGenerator) writeTypeEnumToStringHelper() {
	gen.funcDecls.WriteString("const char* ahoy_type_enum_to_string(AhoyValueType type) {\n")
	gen.funcDecls.WriteString("    switch(type) {\n")
	gen.funcDecls.WriteString("        case AHOY_TYPE_INT: return \"int\";\n")
	gen.funcDecls.WriteString("        case AHOY_TYPE_STRING: return \"string\";\n")
	gen.funcDecls.WriteString("        case AHOY_TYPE_FLOAT: return \"float\";\n")
	gen.funcDecls.WriteString("        case AHOY_TYPE_CHAR: return \"char\";\n")
	gen.funcDecls.WriteString("        case AHOY_TYPE_STRUCT: return \"struct\";\n")
	gen.funcDecls.WriteString("        default: return \"unknown\";\n")
	gen.funcDecls.WriteString("    }\n")
	gen.funcDecls.WriteString("}\n\n")
}

// Generate signal handler for crash reporting.
//
// Generated code records the current Ahoy file/line/function in the globals
// below before every statement (debug builds; see emitCrashLocation). The
// handler reads them back, so a segfault reports the exact Ahoy location
// instead of a generic message. All handler output uses write(), never
// fprintf/malloc, because almost nothing is async-signal-safe. After
// reporting, the signal is re-raised with default disposition, so the process
// still dies by signal (exit status 139 for SIGSEGV, core dumps intact).
func (gen *CodeGenerator) getSignalHandler() string {
	return `// Crash-location tracking: generated code stores the current Ahoy file/line/
// function here before every statement (debug builds only). Volatile so the C
// compiler can neither eliminate the stores nor reorder them past each other.
// NOTE: these are process-global, so a crash in a job thread may report the
// wrong thread's last statement. Main-thread crashes (the common case) are exact.
const char* volatile __ahoy_file = "";
volatile int __ahoy_line = 0;
const char* volatile __ahoy_func = "";

#include <signal.h>
#include <stdlib.h>
#include <unistd.h>
#include <string.h>
#include <stdint.h>
#ifndef _WIN32
#include <ucontext.h>
#endif

static void ahoy_crash_write(const char* s) {
    size_t n = 0;
    size_t off = 0;
    ssize_t w;
    while (s[n] != '\0') n++;
    while (off < n) {
        w = write(STDERR_FILENO, s + off, n - off);
        if (w <= 0) return;
        off += (size_t)w;
    }
}

static void ahoy_crash_write_long(long v) {
    char buf[24];
    unsigned long u;
    int neg = 0;
    int i = 0;
    int a, b;
    char t;
    size_t off = 0;
    ssize_t w;
    if (v < 0) { neg = 1; u = (unsigned long)(-(v + 1)) + 1UL; }
    else { u = (unsigned long)v; }
    if (u == 0) buf[i++] = '0';
    while (u > 0 && i < 23) { buf[i++] = (char)('0' + (u % 10)); u /= 10; }
    if (neg && i < 23) buf[i++] = '-';
    for (a = 0, b = i - 1; a < b; a++, b--) { t = buf[a]; buf[a] = buf[b]; buf[b] = t; }
    while (off < (size_t)i) {
        w = write(STDERR_FILENO, buf + off, (size_t)i - off);
        if (w <= 0) return;
        off += (size_t)w;
    }
}

static void ahoy_crash_write_hex(uintptr_t v) {
    static const char digits[] = "0123456789abcdef";
    char buf[16];
    int i = 0;
    int a, b;
    char t;
    size_t off = 0;
    ssize_t w;
    if (v == 0) { ahoy_crash_write("0x0"); return; }
    while (v > 0 && i < 16) { buf[i++] = digits[v & 0xF]; v >>= 4; }
    ahoy_crash_write("0x");
    for (a = 0, b = i - 1; a < b; a++, b--) { t = buf[a]; buf[a] = buf[b]; buf[b] = t; }
    while (off < (size_t)i) {
        w = write(STDERR_FILENO, buf + off, (size_t)i - off);
        if (w <= 0) return;
        off += (size_t)w;
    }
}

// have_addr: si_addr available. rw: 1 = write fault, 0 = read fault,
// 2 = permission fault (SEGV_ACCERR, often a write to read-only memory),
// -1 = unknown. fpe: si_code for SIGFPE (FPE_INTDIV etc.), 0 otherwise.
static void ahoy_report_crash(int sig, int have_addr, uintptr_t addr, int rw, int fpe) {
    const char* file = (const char*)__ahoy_file;
    int line = __ahoy_line;
    const char* func = (const char*)__ahoy_func;
    ahoy_crash_write("\n========================================\n");
    ahoy_crash_write("  Ahoy Program Crashed (");
    switch (sig) {
        case SIGSEGV: ahoy_crash_write("SIGSEGV"); break;
        case SIGABRT: ahoy_crash_write("SIGABRT"); break;
        case SIGFPE: ahoy_crash_write("SIGFPE"); break;
        case SIGILL: ahoy_crash_write("SIGILL"); break;
#ifdef SIGBUS
        case SIGBUS: ahoy_crash_write("SIGBUS"); break;
#endif
        default: ahoy_crash_write("signal "); ahoy_crash_write_long(sig); break;
    }
    ahoy_crash_write(")\n========================================\n\n");
    if (line > 0 && file[0] != '\0') {
        ahoy_crash_write("  Location: ");
        ahoy_crash_write(file);
        ahoy_crash_write(":");
        ahoy_crash_write_long(line);
        if (func[0] != '\0') {
            ahoy_crash_write(" in ");
            ahoy_crash_write(func);
        }
        ahoy_crash_write("\n");
    } else {
        ahoy_crash_write("  Location: (before any tracked Ahoy statement -- likely inside a C library)\n");
    }
    if (sig == SIGSEGV
#ifdef SIGBUS
        || sig == SIGBUS
#endif
    ) {
        ahoy_crash_write("  Fault: ");
        if (rw == 1) ahoy_crash_write("write to ");
        else if (rw == 0) ahoy_crash_write("read of ");
        else ahoy_crash_write("access to ");
        if (have_addr) {
            ahoy_crash_write("address ");
            ahoy_crash_write_hex(addr);
            if (rw == 2) {
                ahoy_crash_write(" -- permission fault (often a write to read-only memory)");
            } else if (addr < 0x1000) {
                ahoy_crash_write(" -- null pointer dereference");
            } else if (addr < 0x10000) {
                ahoy_crash_write(" -- near-null pointer (small offset from null)");
            } else {
                ahoy_crash_write(" -- unmapped or freed memory");
            }
        } else {
            ahoy_crash_write("unmapped memory");
        }
        ahoy_crash_write("\n");
    } else if (sig == SIGABRT) {
        ahoy_crash_write("  Fault: abort() called -- failed assertion or explicit abort\n");
    } else if (sig == SIGFPE) {
        ahoy_crash_write("  Fault: ");
        if (fpe == FPE_INTDIV) ahoy_crash_write("integer division by zero\n");
        else if (fpe == FPE_INTOVF) ahoy_crash_write("integer overflow\n");
#ifdef FPE_FLTDIV
        else if (fpe == FPE_FLTDIV) ahoy_crash_write("floating-point division by zero\n");
#endif
        else ahoy_crash_write("arithmetic fault\n");
    } else if (sig == SIGILL) {
        ahoy_crash_write("  Fault: illegal instruction -- corrupted code path or bad jump\n");
    }
    ahoy_crash_write("\n  Tips:\n");
    ahoy_crash_write("  - Check array accesses are within bounds\n");
    ahoy_crash_write("  - Ensure variables are initialized before use\n");
    ahoy_crash_write("  - Verify pointers are not null\n");
    ahoy_crash_write("\n========================================\n");
}

#ifndef _WIN32
static void ahoy_signal_handler(int sig, siginfo_t* info, void* uctx) {
    static volatile sig_atomic_t entered = 0;
    struct sigaction sa;
    int have_addr = 0;
    int rw = -1;
    int fpe = 0;
    uintptr_t addr = 0;
    (void)uctx;
    if (entered) _exit(134);
    entered = 1;
    if ((sig == SIGSEGV
#ifdef SIGBUS
        || sig == SIGBUS
#endif
    ) && info != 0) {
        addr = (uintptr_t)info->si_addr;
        have_addr = 1;
#if defined(__linux__) && defined(__x86_64__)
#  ifdef REG_ERR
        if (uctx != 0) {
            ucontext_t* uc = (ucontext_t*)uctx;
            rw = (uc->uc_mcontext.gregs[REG_ERR] & 2) ? 1 : 0;
        }
#  endif
#endif
        if (rw < 0 && info->si_code == SEGV_ACCERR) rw = 2;
    }
    if (sig == SIGFPE && info != 0) fpe = info->si_code;
    ahoy_report_crash(sig, have_addr, addr, rw, fpe);
    memset(&sa, 0, sizeof(sa));
    sa.sa_handler = SIG_DFL;
    sigemptyset(&sa.sa_mask);
    sigaction(sig, &sa, 0);
    raise(sig);
    _exit(134);
}

void ahoy_setup_signal_handlers(void) {
    struct sigaction sa;
    memset(&sa, 0, sizeof(sa));
    sigemptyset(&sa.sa_mask);
    sa.sa_sigaction = ahoy_signal_handler;
    sa.sa_flags = SA_SIGINFO;
    sigaction(SIGSEGV, &sa, 0);
    sigaction(SIGABRT, &sa, 0);
    sigaction(SIGFPE, &sa, 0);
    sigaction(SIGILL, &sa, 0);
#ifdef SIGBUS
    sigaction(SIGBUS, &sa, 0);
#endif
}
#else
// Windows has no sigaction/siginfo: still report the Ahoy location.
static void ahoy_signal_handler(int sig) {
    static volatile sig_atomic_t entered = 0;
    if (entered) _exit(134);
    entered = 1;
    ahoy_report_crash(sig, 0, 0, -1, 0);
    signal(sig, SIG_DFL);
    raise(sig);
    _exit(134);
}

void ahoy_setup_signal_handlers(void) {
    signal(SIGSEGV, ahoy_signal_handler);
    signal(SIGABRT, ahoy_signal_handler);
    signal(SIGFPE, ahoy_signal_handler);
    signal(SIGILL, ahoy_signal_handler);
}
#endif
`
}

// Generate array helper functions
func (gen *CodeGenerator) writeArrayHelperFunctions() {
	// Note: AhoyArray structure is now defined in the header section

	if len(gen.arrayMethods) == 0 {
		return
	}

	gen.includes["time.h"] = true // For shuffle

	// Use stdlib definitions for standard array methods
	stdlibMethods := []string{"length", "push", "pop", "sum", "has", "sort", "reverse", "shuffle", "pick", "fill", "remove"}
	for _, method := range stdlibMethods {
		if gen.arrayMethods[method] {
			if stdlibFunc, ok := ArrayMethods[method]; ok {
				gen.funcDecls.WriteString(stdlibFunc.Code)
				gen.funcDecls.WriteString("\n")
			}
		}
	}

	// print_array helper - formats array for printing with type support
	if gen.arrayMethods["print_array"] {
		gen.funcDecls.WriteString("char* print_array_helper(AhoyArray* arr) {\n")
		gen.funcDecls.WriteString("    if (arr == NULL || arr->length == 0) return \"[]\";\n")
		gen.funcDecls.WriteString("    char* buffer = malloc(4096);\n")
		gen.funcDecls.WriteString("    int offset = 0;\n")
		gen.funcDecls.WriteString("    offset += sprintf(buffer + offset, \"[\");\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < arr->length; i++) {\n")
		gen.funcDecls.WriteString("        if (i > 0) offset += sprintf(buffer + offset, \", \");\n")
		gen.funcDecls.WriteString("        switch (arr->types[i]) {\n")
		gen.funcDecls.WriteString("            case AHOY_TYPE_INT:\n")
		gen.funcDecls.WriteString("                offset += sprintf(buffer + offset, \"%d\", (int)arr->data[i]);\n")
		gen.funcDecls.WriteString("                break;\n")
		gen.funcDecls.WriteString("            case AHOY_TYPE_FLOAT:\n")
		gen.funcDecls.WriteString("                offset += sprintf(buffer + offset, \"%f\", *((double*)(intptr_t)arr->data[i]));\n")
		gen.funcDecls.WriteString("                break;\n")
		gen.funcDecls.WriteString("            case AHOY_TYPE_STRING:\n")
		gen.funcDecls.WriteString("                offset += sprintf(buffer + offset, \"\\\"%s\\\"\", (char*)(intptr_t)arr->data[i]);\n")
		gen.funcDecls.WriteString("                break;\n")
		gen.funcDecls.WriteString("            case AHOY_TYPE_CHAR:\n")
		gen.funcDecls.WriteString("                offset += sprintf(buffer + offset, \"'%c'\", (char)arr->data[i]);\n")
		gen.funcDecls.WriteString("                break;\n")
		gen.funcDecls.WriteString("        }\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    offset += sprintf(buffer + offset, \"]\");\n")
		gen.funcDecls.WriteString("    return buffer;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// print_string_array helper - formats string array for printing
	if gen.arrayMethods["print_string_array"] {
		gen.funcDecls.WriteString("char* print_string_array_helper(AhoyArray* arr) {\n")
		gen.funcDecls.WriteString("    if (arr == NULL || arr->length == 0) return \"[]\";\n")
		gen.funcDecls.WriteString("    char* buffer = malloc(4096);\n")
		gen.funcDecls.WriteString("    int offset = 0;\n")
		gen.funcDecls.WriteString("    offset += sprintf(buffer + offset, \"[\");\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < arr->length; i++) {\n")
		gen.funcDecls.WriteString("        if (i > 0) offset += sprintf(buffer + offset, \", \");\n")
		gen.funcDecls.WriteString("        char* str = (char*)(intptr_t)arr->data[i];\n")
		gen.funcDecls.WriteString("        offset += sprintf(buffer + offset, \"\\\"%s\\\"\", str);\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    offset += sprintf(buffer + offset, \"]\");\n")
		gen.funcDecls.WriteString("    return buffer;\n")
		gen.funcDecls.WriteString("}\n\n")
	}
}

// Generate built-in type helpers (always available)
func (gen *CodeGenerator) writeBuiltinTypeHelpers() {
	// Note: color_to_string and vector2_to_string helpers removed
	// These types should be provided by imported libraries (e.g., raylib)
	// If needed, users can define their own helper functions
}

// Generate dictionary helper functions
func (gen *CodeGenerator) writeDictHelperFunctions() {
	if len(gen.dictMethods) == 0 {
		return
	}

	gen.funcDecls.WriteString("\n// Dictionary Helper Methods\n")

	// Check if we need array support for keys() or values() methods
	if gen.dictMethods["keys"] || gen.dictMethods["values"] {
		gen.arrayImpls = true
	}

	// Use stdlib definitions for standard dict methods
	stdlibMethods := []string{"size", "clear", "has", "has_all", "keys", "values", "sort", "stable_sort", "merge"}
	for _, method := range stdlibMethods {
		if gen.dictMethods[method] {
			if stdlibFunc, ok := DictMethods[method]; ok {
				gen.funcDecls.WriteString(stdlibFunc.Code)
				gen.funcDecls.WriteString("\n")
			}
		}
	}

	// print_dict helper - formats dict for printing (keep inline since it's complex)
	if gen.dictMethods["print_dict"] {
		gen.funcDecls.WriteString("char* print_dict_helper(HashMap* dict) {\n")
		gen.funcDecls.WriteString("    if (dict == NULL || dict->size == 0) return \"{}\";\n")
		gen.funcDecls.WriteString("    char* buffer = malloc(4096);\n")
		gen.funcDecls.WriteString("    int offset = 0;\n")
		gen.funcDecls.WriteString("    offset += sprintf(buffer + offset, \"{\");\n")
		gen.funcDecls.WriteString("    int count = 0;\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < dict->capacity; i++) {\n")
		gen.funcDecls.WriteString("        HashMapEntry* entry = dict->buckets[i];\n")
		gen.funcDecls.WriteString("        while (entry != NULL) {\n")
		gen.funcDecls.WriteString("            if (count > 0) offset += sprintf(buffer + offset, \", \");\n")
		gen.funcDecls.WriteString("            offset += sprintf(buffer + offset, \"\\\"%s\\\": \", entry->key);\n")
		gen.funcDecls.WriteString("            // Print value based on type\n")
		gen.funcDecls.WriteString("            if (entry->value != NULL) {\n")
		gen.funcDecls.WriteString("                switch(entry->valueType) {\n")
		gen.funcDecls.WriteString("                    case AHOY_TYPE_INT:\n")
		gen.funcDecls.WriteString("                        offset += sprintf(buffer + offset, \"%d\", (int)(intptr_t)entry->value);\n")
		gen.funcDecls.WriteString("                        break;\n")
		gen.funcDecls.WriteString("                    case AHOY_TYPE_FLOAT:\n")
		gen.funcDecls.WriteString("                        offset += sprintf(buffer + offset, \"%g\", *((double*)&entry->value));\n")
		gen.funcDecls.WriteString("                        break;\n")
		gen.funcDecls.WriteString("                    case AHOY_TYPE_STRING:\n")
		gen.funcDecls.WriteString("                        offset += sprintf(buffer + offset, \"\\\"%s\\\"\", (char*)entry->value);\n")
		gen.funcDecls.WriteString("                        break;\n")
		gen.funcDecls.WriteString("                    default:\n")
		gen.funcDecls.WriteString("                        offset += sprintf(buffer + offset, \"%p\", entry->value);\n")
		gen.funcDecls.WriteString("                        break;\n")
		gen.funcDecls.WriteString("                }\n")
		gen.funcDecls.WriteString("            } else {\n")
		gen.funcDecls.WriteString("                offset += sprintf(buffer + offset, \"null\");\n")
		gen.funcDecls.WriteString("            }\n")
		gen.funcDecls.WriteString("            count++;\n")
		gen.funcDecls.WriteString("            entry = entry->next;\n")
		gen.funcDecls.WriteString("        }\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    offset += sprintf(buffer + offset, \"}\");\n")
		gen.funcDecls.WriteString("    return buffer;\n")
		gen.funcDecls.WriteString("}\n\n")

		// Helper to format a single HashMap value as string
		gen.funcDecls.WriteString("char* format_hashmap_value(HashMap* dict, const char* key) {\n")
		gen.funcDecls.WriteString("    static char buffer[256];\n")
		gen.funcDecls.WriteString("    // Find the entry\n")
		gen.funcDecls.WriteString("    unsigned int index = hash(key) % dict->capacity;\n")
		gen.funcDecls.WriteString("    HashMapEntry* entry = dict->buckets[index];\n")
		gen.funcDecls.WriteString("    while (entry != NULL) {\n")
		gen.funcDecls.WriteString("        if (strcmp(entry->key, key) == 0) {\n")
		gen.funcDecls.WriteString("            switch(entry->valueType) {\n")
		gen.funcDecls.WriteString("                case AHOY_TYPE_INT:\n")
		gen.funcDecls.WriteString("                    sprintf(buffer, \"%d\", (int)(intptr_t)entry->value);\n")
		gen.funcDecls.WriteString("                    break;\n")
		gen.funcDecls.WriteString("                case AHOY_TYPE_FLOAT:\n")
		gen.funcDecls.WriteString("                    sprintf(buffer, \"%g\", *((double*)&entry->value));\n")
		gen.funcDecls.WriteString("                    break;\n")
		gen.funcDecls.WriteString("                case AHOY_TYPE_STRING:\n")
		gen.funcDecls.WriteString("                    return (char*)entry->value;\n")
		gen.funcDecls.WriteString("                default:\n")
		gen.funcDecls.WriteString("                    sprintf(buffer, \"%p\", entry->value);\n")
		gen.funcDecls.WriteString("                    break;\n")
		gen.funcDecls.WriteString("            }\n")
		gen.funcDecls.WriteString("            return buffer;\n")
		gen.funcDecls.WriteString("        }\n")
		gen.funcDecls.WriteString("        entry = entry->next;\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    return \"(null)\";\n")
		gen.funcDecls.WriteString("}\n\n")
	}
}

// registerJSONFunctionTypes registers return types for JSON functions
func (gen *CodeGenerator) registerJSONFunctionTypes() {
	// Mark all JSON functions as user functions so they don't get converted to PascalCase
	gen.userFunctions["ahoy_json_read"] = true
	gen.userFunctions["ahoy_json_write"] = true
	gen.userFunctions["ahoy_json_get"] = true
	gen.userFunctions["ahoy_json_get_index"] = true
	gen.userFunctions["ahoy_json_string"] = true
	gen.userFunctions["ahoy_json_number"] = true
	gen.userFunctions["ahoy_json_int"] = true
	gen.userFunctions["ahoy_json_bool"] = true

	// Register return types for JSON helper functions
	gen.functionReturnTypes["ahoy_json_string"] = []string{"string"}
	gen.functionReturnTypes["ahoy_json_number"] = []string{"float"}
	gen.functionReturnTypes["ahoy_json_int"] = []string{"int"}
	gen.functionReturnTypes["ahoy_json_bool"] = []string{"bool"}
	gen.functionReturnTypes["ahoy_json_get"] = []string{"AhoyJSON*"}
	gen.functionReturnTypes["ahoy_json_get_index"] = []string{"AhoyJSON*"}
}

// writeJSONHelperFunctions generates JSON parsing and writing functions
func (gen *CodeGenerator) writeJSONHelperFunctions() {
	if !gen.useJSON {
		return
	}

	// Add JSON type definition and functions
	gen.funcDecls.WriteString("\n// JSON Support\n")
	gen.funcDecls.WriteString("struct AhoyJSON {\n")
	gen.funcDecls.WriteString("    HashMap* data;  // For objects\n")
	gen.funcDecls.WriteString("    DynamicArray* array_data;  // For arrays\n")
	gen.funcDecls.WriteString("    char* string_value;  // For strings\n")
	gen.funcDecls.WriteString("    double number_value;  // For numbers\n")
	gen.funcDecls.WriteString("    int bool_value;  // For booleans\n")
	gen.funcDecls.WriteString("    int is_null;\n")
	gen.funcDecls.WriteString("    enum { JSON_OBJECT, JSON_ARRAY, JSON_STRING, JSON_NUMBER, JSON_BOOL, JSON_NULL } type;\n")
	gen.funcDecls.WriteString("};\n\n")

	// Forward declarations
	gen.funcDecls.WriteString("AhoyJSON* ahoy_json_parse_value(const char** p);\n")
	gen.funcDecls.WriteString("void ahoy_json_skip_whitespace(const char** p);\n\n")

	// Skip whitespace
	gen.funcDecls.WriteString("void ahoy_json_skip_whitespace(const char** p) {\n")
	gen.funcDecls.WriteString("    while (**p == ' ' || **p == '\\t' || **p == '\\n' || **p == '\\r') (*p)++;\n")
	gen.funcDecls.WriteString("}\n\n")

	// Parse string
	gen.funcDecls.WriteString("char* ahoy_json_parse_string(const char** p) {\n")
	gen.funcDecls.WriteString("    (*p)++;  // Skip opening quote\n")
	gen.funcDecls.WriteString("    const char* start = *p;\n")
	gen.funcDecls.WriteString("    while (**p && **p != '\"') {\n")
	gen.funcDecls.WriteString("        if (**p == '\\\\') (*p)++;  // Skip escaped char\n")
	gen.funcDecls.WriteString("        (*p)++;\n")
	gen.funcDecls.WriteString("    }\n")
	gen.funcDecls.WriteString("    int len = *p - start;\n")
	gen.funcDecls.WriteString("    char* result = malloc(len + 1);\n")
	gen.funcDecls.WriteString("    strncpy(result, start, len);\n")
	gen.funcDecls.WriteString("    result[len] = 0;\n")
	gen.funcDecls.WriteString("    (*p)++;  // Skip closing quote\n")
	gen.funcDecls.WriteString("    return result;\n")
	gen.funcDecls.WriteString("}\n\n")

	// Parse number
	gen.funcDecls.WriteString("double ahoy_json_parse_number(const char** p) {\n")
	gen.funcDecls.WriteString("    return strtod(*p, (char**)p);\n")
	gen.funcDecls.WriteString("}\n\n")

	// Parse object
	gen.funcDecls.WriteString("AhoyJSON* ahoy_json_parse_object(const char** p) {\n")
	gen.funcDecls.WriteString("    AhoyJSON* json = malloc(sizeof(AhoyJSON));\n")
	gen.funcDecls.WriteString("    json->type = JSON_OBJECT;\n")
	gen.funcDecls.WriteString("    json->data = createHashMap(16);\n")
	gen.funcDecls.WriteString("    (*p)++;  // Skip '{'\n")
	gen.funcDecls.WriteString("    ahoy_json_skip_whitespace(p);\n")
	gen.funcDecls.WriteString("    if (**p == '}') { (*p)++; return json; }\n")
	gen.funcDecls.WriteString("    while (1) {\n")
	gen.funcDecls.WriteString("        ahoy_json_skip_whitespace(p);\n")
	gen.funcDecls.WriteString("        if (**p != '\"') break;\n")
	gen.funcDecls.WriteString("        char* key = ahoy_json_parse_string(p);\n")
	gen.funcDecls.WriteString("        ahoy_json_skip_whitespace(p);\n")
	gen.funcDecls.WriteString("        if (**p == ':') (*p)++;\n")
	gen.funcDecls.WriteString("        ahoy_json_skip_whitespace(p);\n")
	gen.funcDecls.WriteString("        AhoyJSON* value = ahoy_json_parse_value(p);\n")
	gen.funcDecls.WriteString("        hashMapPut(json->data, key, value);\n")
	gen.funcDecls.WriteString("        ahoy_json_skip_whitespace(p);\n")
	gen.funcDecls.WriteString("        if (**p == ',') { (*p)++; continue; }\n")
	gen.funcDecls.WriteString("        if (**p == '}') break;\n")
	gen.funcDecls.WriteString("    }\n")
	gen.funcDecls.WriteString("    if (**p == '}') (*p)++;\n")
	gen.funcDecls.WriteString("    return json;\n")
	gen.funcDecls.WriteString("}\n\n")

	// Parse array
	gen.funcDecls.WriteString("AhoyJSON* ahoy_json_parse_array(const char** p) {\n")
	gen.funcDecls.WriteString("    AhoyJSON* json = malloc(sizeof(AhoyJSON));\n")
	gen.funcDecls.WriteString("    json->type = JSON_ARRAY;\n")
	gen.funcDecls.WriteString("    json->array_data = createArray(16);\n")
	gen.funcDecls.WriteString("    (*p)++;  // Skip '['\n")
	gen.funcDecls.WriteString("    ahoy_json_skip_whitespace(p);\n")
	gen.funcDecls.WriteString("    if (**p == ']') { (*p)++; return json; }\n")
	gen.funcDecls.WriteString("    while (1) {\n")
	gen.funcDecls.WriteString("        ahoy_json_skip_whitespace(p);\n")
	gen.funcDecls.WriteString("        AhoyJSON* value = ahoy_json_parse_value(p);\n")
	gen.funcDecls.WriteString("        arrayPush(json->array_data, value);\n")
	gen.funcDecls.WriteString("        ahoy_json_skip_whitespace(p);\n")
	gen.funcDecls.WriteString("        if (**p == ',') { (*p)++; continue; }\n")
	gen.funcDecls.WriteString("        if (**p == ']') break;\n")
	gen.funcDecls.WriteString("    }\n")
	gen.funcDecls.WriteString("    if (**p == ']') (*p)++;\n")
	gen.funcDecls.WriteString("    return json;\n")
	gen.funcDecls.WriteString("}\n\n")

	// Parse value (main parser)
	gen.funcDecls.WriteString("AhoyJSON* ahoy_json_parse_value(const char** p) {\n")
	gen.funcDecls.WriteString("    ahoy_json_skip_whitespace(p);\n")
	gen.funcDecls.WriteString("    AhoyJSON* json = malloc(sizeof(AhoyJSON));\n")
	gen.funcDecls.WriteString("    if (**p == '{') return ahoy_json_parse_object(p);\n")
	gen.funcDecls.WriteString("    if (**p == '[') return ahoy_json_parse_array(p);\n")
	gen.funcDecls.WriteString("    if (**p == '\"') {\n")
	gen.funcDecls.WriteString("        json->type = JSON_STRING;\n")
	gen.funcDecls.WriteString("        json->string_value = ahoy_json_parse_string(p);\n")
	gen.funcDecls.WriteString("        return json;\n")
	gen.funcDecls.WriteString("    }\n")
	gen.funcDecls.WriteString("    if (strncmp(*p, \"true\", 4) == 0) {\n")
	gen.funcDecls.WriteString("        json->type = JSON_BOOL; json->bool_value = 1; *p += 4; return json;\n")
	gen.funcDecls.WriteString("    }\n")
	gen.funcDecls.WriteString("    if (strncmp(*p, \"false\", 5) == 0) {\n")
	gen.funcDecls.WriteString("        json->type = JSON_BOOL; json->bool_value = 0; *p += 5; return json;\n")
	gen.funcDecls.WriteString("    }\n")
	gen.funcDecls.WriteString("    if (strncmp(*p, \"null\", 4) == 0) {\n")
	gen.funcDecls.WriteString("        json->type = JSON_NULL; json->is_null = 1; *p += 4; return json;\n")
	gen.funcDecls.WriteString("    }\n")
	gen.funcDecls.WriteString("    // Number\n")
	gen.funcDecls.WriteString("    json->type = JSON_NUMBER;\n")
	gen.funcDecls.WriteString("    json->number_value = ahoy_json_parse_number(p);\n")
	gen.funcDecls.WriteString("    return json;\n")
	gen.funcDecls.WriteString("}\n\n")

	// JSON type must be forward declared first, then define return struct
	gen.funcReturnStructs.WriteString("// Forward declare JSON type\n")
	gen.funcReturnStructs.WriteString("typedef struct AhoyJSON AhoyJSON;\n\n")

	// json_read function - use multi-return struct naming convention
	gen.funcReturnStructs.WriteString("// JSON read return type\n")
	gen.funcReturnStructs.WriteString("typedef struct {\n")
	gen.funcReturnStructs.WriteString("    AhoyJSON* ret0;\n")
	gen.funcReturnStructs.WriteString("    char* ret1;\n")
	gen.funcReturnStructs.WriteString("} json_read_return;\n\n")

	// Forward declare the read_json function and helpers
	gen.funcReturnStructs.WriteString("json_read_return ahoy_json_read(const char* filename);\n")
	gen.funcReturnStructs.WriteString("char* ahoy_json_write(const char* filename, AhoyJSON* json);\n")
	gen.funcReturnStructs.WriteString("AhoyJSON* ahoy_json_get(AhoyJSON* json, const char* key);\n")
	gen.funcReturnStructs.WriteString("AhoyJSON* ahoy_json_get_index(AhoyJSON* json, int index);\n")
	gen.funcReturnStructs.WriteString("char* ahoy_json_string(AhoyJSON* json);\n")
	gen.funcReturnStructs.WriteString("double ahoy_json_number(AhoyJSON* json);\n")
	gen.funcReturnStructs.WriteString("int ahoy_json_int(AhoyJSON* json);\n")
	gen.funcReturnStructs.WriteString("int ahoy_json_bool(AhoyJSON* json);\n")
	gen.funcReturnStructs.WriteString("char* ahoy_json_stringify(AhoyJSON* json);\n\n")

	gen.funcDecls.WriteString("json_read_return ahoy_json_read(const char* filename) {\n")
	gen.funcDecls.WriteString("    json_read_return result = {NULL, NULL};\n")
	gen.funcDecls.WriteString("    FILE* f = fopen(filename, \"r\");\n")
	gen.funcDecls.WriteString("    if (!f) {\n")
	gen.funcDecls.WriteString("        result.ret1 = \"Failed to open file\";\n")
	gen.funcDecls.WriteString("        return result;\n")
	gen.funcDecls.WriteString("    }\n")
	gen.funcDecls.WriteString("    fseek(f, 0, SEEK_END);\n")
	gen.funcDecls.WriteString("    long size = ftell(f);\n")
	gen.funcDecls.WriteString("    fseek(f, 0, SEEK_SET);\n")
	gen.funcDecls.WriteString("    char* content = malloc(size + 1);\n")
	gen.funcDecls.WriteString("    fread(content, 1, size, f);\n")
	gen.funcDecls.WriteString("    content[size] = 0;\n")
	gen.funcDecls.WriteString("    fclose(f);\n")
	gen.funcDecls.WriteString("    const char* p = content;\n")
	gen.funcDecls.WriteString("    result.ret0 = ahoy_json_parse_value(&p);\n")
	gen.funcDecls.WriteString("    return result;\n")
	gen.funcDecls.WriteString("}\n\n")

	// json_write function (simplified - just converts to string)
	gen.funcDecls.WriteString("char* ahoy_json_write(const char* filename, AhoyJSON* json) {\n")
	gen.funcDecls.WriteString("    // TODO: Implement JSON serialization\n")
	gen.funcDecls.WriteString("    return \"Not implemented yet\";\n")
	gen.funcDecls.WriteString("}\n\n")

	// Helper to access JSON properties
	gen.funcDecls.WriteString("AhoyJSON* ahoy_json_get(AhoyJSON* json, const char* key) {\n")
	gen.funcDecls.WriteString("    if (!json || json->type != JSON_OBJECT) return NULL;\n")
	gen.funcDecls.WriteString("    return (AhoyJSON*)hashMapGet(json->data, key);\n")
	gen.funcDecls.WriteString("}\n\n")

	gen.funcDecls.WriteString("AhoyJSON* ahoy_json_get_index(AhoyJSON* json, int index) {\n")
	gen.funcDecls.WriteString("    if (!json || json->type != JSON_ARRAY) return NULL;\n")
	gen.funcDecls.WriteString("    if (index < 0 || index >= json->array_data->size) return NULL;\n")
	gen.funcDecls.WriteString("    return (AhoyJSON*)json->array_data->data[index];\n")
	gen.funcDecls.WriteString("}\n\n")

	// Add value extraction helpers
	gen.funcDecls.WriteString("// Extract string value from JSON\n")
	gen.funcDecls.WriteString("char* ahoy_json_string(AhoyJSON* json) {\n")
	gen.funcDecls.WriteString("    if (!json) return \"\";\n")
	gen.funcDecls.WriteString("    if (json->type == JSON_STRING) return json->string_value;\n")
	gen.funcDecls.WriteString("    return \"\";\n")
	gen.funcDecls.WriteString("}\n\n")

	gen.funcDecls.WriteString("// Extract number value from JSON\n")
	gen.funcDecls.WriteString("double ahoy_json_number(AhoyJSON* json) {\n")
	gen.funcDecls.WriteString("    if (!json) return 0.0;\n")
	gen.funcDecls.WriteString("    if (json->type == JSON_NUMBER) return json->number_value;\n")
	gen.funcDecls.WriteString("    return 0.0;\n")
	gen.funcDecls.WriteString("}\n\n")

	gen.funcDecls.WriteString("// Extract int value from JSON\n")
	gen.funcDecls.WriteString("int ahoy_json_int(AhoyJSON* json) {\n")
	gen.funcDecls.WriteString("    if (!json) return 0;\n")
	gen.funcDecls.WriteString("    if (json->type == JSON_NUMBER) return (int)json->number_value;\n")
	gen.funcDecls.WriteString("    return 0;\n")
	gen.funcDecls.WriteString("}\n\n")

	gen.funcDecls.WriteString("// Extract bool value from JSON\n")
	gen.funcDecls.WriteString("int ahoy_json_bool(AhoyJSON* json) {\n")
	gen.funcDecls.WriteString("    if (!json) return 0;\n")
	gen.funcDecls.WriteString("    if (json->type == JSON_BOOL) return json->bool_value;\n")
	gen.funcDecls.WriteString("    return 0;\n")
	gen.funcDecls.WriteString("}\n\n")

	// JSON stringify function (forward declare helper)
	gen.funcDecls.WriteString("// Forward declare recursive helper\n")
	gen.funcDecls.WriteString("void ahoy_json_stringify_helper(AhoyJSON* json, char* buffer, int* pos, int max_size);\n\n")

	gen.funcDecls.WriteString("// Recursive helper for stringify\n")
	gen.funcDecls.WriteString("void ahoy_json_stringify_helper(AhoyJSON* json, char* buffer, int* pos, int max_size) {\n")
	gen.funcDecls.WriteString("    if (!json || *pos >= max_size - 1) return;\n")
	gen.funcDecls.WriteString("    \n")
	gen.funcDecls.WriteString("    switch(json->type) {\n")
	gen.funcDecls.WriteString("        case JSON_STRING:\n")
	gen.funcDecls.WriteString("            *pos += snprintf(buffer + *pos, max_size - *pos, \"\\\"%s\\\"\", json->string_value ? json->string_value : \"\");\n")
	gen.funcDecls.WriteString("            break;\n")
	gen.funcDecls.WriteString("        case JSON_NUMBER:\n")
	gen.funcDecls.WriteString("            if (json->number_value == (int)json->number_value) {\n")
	gen.funcDecls.WriteString("                *pos += snprintf(buffer + *pos, max_size - *pos, \"%d\", (int)json->number_value);\n")
	gen.funcDecls.WriteString("            } else {\n")
	gen.funcDecls.WriteString("                *pos += snprintf(buffer + *pos, max_size - *pos, \"%g\", json->number_value);\n")
	gen.funcDecls.WriteString("            }\n")
	gen.funcDecls.WriteString("            break;\n")
	gen.funcDecls.WriteString("        case JSON_BOOL:\n")
	gen.funcDecls.WriteString("            *pos += snprintf(buffer + *pos, max_size - *pos, \"%s\", json->bool_value ? \"true\" : \"false\");\n")
	gen.funcDecls.WriteString("            break;\n")
	gen.funcDecls.WriteString("        case JSON_NULL:\n")
	gen.funcDecls.WriteString("            *pos += snprintf(buffer + *pos, max_size - *pos, \"null\");\n")
	gen.funcDecls.WriteString("            break;\n")
	gen.funcDecls.WriteString("        case JSON_OBJECT:\n")
	gen.funcDecls.WriteString("            // For objects, we'd need to iterate the internal HashMap\n")
	gen.funcDecls.WriteString("            // For now, just show it's an object\n")
	gen.funcDecls.WriteString("            *pos += snprintf(buffer + *pos, max_size - *pos, \"{...}\");\n")
	gen.funcDecls.WriteString("            break;\n")
	gen.funcDecls.WriteString("        case JSON_ARRAY: {\n")
	gen.funcDecls.WriteString("            *pos += snprintf(buffer + *pos, max_size - *pos, \"[\");\n")
	gen.funcDecls.WriteString("            for (int i = 0; i < json->array_data->size && *pos < max_size - 1; i++) {\n")
	gen.funcDecls.WriteString("                if (i > 0) *pos += snprintf(buffer + *pos, max_size - *pos, \",\");\n")
	gen.funcDecls.WriteString("                ahoy_json_stringify_helper((AhoyJSON*)json->array_data->data[i], buffer, pos, max_size);\n")
	gen.funcDecls.WriteString("            }\n")
	gen.funcDecls.WriteString("            *pos += snprintf(buffer + *pos, max_size - *pos, \"]\");\n")
	gen.funcDecls.WriteString("            break;\n")
	gen.funcDecls.WriteString("        }\n")
	gen.funcDecls.WriteString("    }\n")
	gen.funcDecls.WriteString("}\n\n")

	gen.funcDecls.WriteString("// Stringify JSON object for printing\n")
	gen.funcDecls.WriteString("char* ahoy_json_stringify(AhoyJSON* json) {\n")
	gen.funcDecls.WriteString("    static char buffer[8192];\n")
	gen.funcDecls.WriteString("    int pos = 0;\n")
	gen.funcDecls.WriteString("    if (!json) return \"null\";\n")
	gen.funcDecls.WriteString("    ahoy_json_stringify_helper(json, buffer, &pos, 8192);\n")
	gen.funcDecls.WriteString("    buffer[pos] = '\\0';\n")
	gen.funcDecls.WriteString("    return buffer;\n")
	gen.funcDecls.WriteString("}\n\n")
}

// Process format string to replace %v and %t with appropriate C format specifiers
func (gen *CodeGenerator) processFormatString(formatStr string, args []*ahoy.ASTNode) (string, []*ahoy.ASTNode) {
	result := ""
	newArgs := []*ahoy.ASTNode{}
	argIndex := 0
	i := 0

	for i < len(formatStr) {
		if formatStr[i] == '%' && i+1 < len(formatStr) {
			if formatStr[i+1] == 'v' {
				// %v - replace with appropriate format specifier based on argument type
				if argIndex < len(args) {
					argType := gen.getNodeType(args[argIndex])
					if argType == "array" {
						// For arrays, we need to call a helper function
						gen.arrayMethods["print_array"] = true
						result += "%s"
						// Mark this argument as needing array helper
						arrayArg := &ahoy.ASTNode{
							Type:     ahoy.NODE_CALL,
							Value:    "__print_array_helper", // Special marker
							Children: []*ahoy.ASTNode{args[argIndex]},
						}
						newArgs = append(newArgs, arrayArg)
					} else {
						result += gen.getFormatSpec(argType)
						newArgs = append(newArgs, args[argIndex])
					}
					argIndex++
				} else {
					result += "%v" // Keep if no argument
				}
				i += 2
			} else if formatStr[i+1] == 't' {
				// %t - replace with type name as string
				if argIndex < len(args) {
					argType := gen.getNodeType(args[argIndex])
					result += "%s"
					// Create a string literal node for the type name
					typeNode := &ahoy.ASTNode{
						Type:  ahoy.NODE_STRING,
						Value: argType,
					}
					newArgs = append(newArgs, typeNode)
					argIndex++
				} else {
					result += "%t" // Keep if no argument
				}
				i += 2
			} else {
				// Regular format specifier or escaped %
				result += string(formatStr[i])
				if i+1 < len(formatStr) {
					result += string(formatStr[i+1])
				}
				// Add the corresponding argument
				if argIndex < len(args) {
					newArgs = append(newArgs, args[argIndex])
					argIndex++
				}
				i += 2
			}
		} else {
			result += string(formatStr[i])
			i++
		}
	}

	// Add any remaining arguments
	for argIndex < len(args) {
		newArgs = append(newArgs, args[argIndex])
		argIndex++
	}

	return result, newArgs
}

// Get the type of a node
func (gen *CodeGenerator) getNodeType(node *ahoy.ASTNode) string {
	if node.DataType != "" {
		return node.DataType
	}

	switch node.Type {
	case ahoy.NODE_NUMBER:
		if strings.Contains(node.Value, ".") {
			return "float"
		}
		return "int"
	case ahoy.NODE_STRING:
		return "string"
	case ahoy.NODE_F_STRING:
		return "string"
	case ahoy.NODE_CHAR:
		return "char"
	case ahoy.NODE_BOOLEAN:
		return "bool"
	case ahoy.NODE_ARRAY_LITERAL:
		return "array"
	case ahoy.NODE_DICT_LITERAL:
		return "dict"
	case ahoy.NODE_IDENTIFIER:
		// Look up in variables map
		if varType, ok := gen.variables[node.Value]; ok {
			return normalizeCollectionType(varType)
		}
		// Parameters are not declarations, so the scanVariableTypes pre-pass
		// misses them; they only appear in functionVars.
		if varType, ok := gen.functionVars[node.Value]; ok {
			return normalizeCollectionType(varType)
		}
		return "int" // Default
	default:
		return "int" // Default
	}
}

// normalizeCollectionType collapses a precise collection type to the base kind
// that print/format handling understands: "array[int]" -> "array",
// "dict<string,int>" -> "dict". Non-collection types pass through unchanged.
func normalizeCollectionType(typeName string) string {
	if strings.HasPrefix(typeName, "array") {
		return "array"
	}
	if strings.HasPrefix(typeName, "dict") {
		return "dict"
	}
	return typeName
}

// Get C format specifier for a type
func (gen *CodeGenerator) getFormatSpec(typeName string) string {
	switch typeName {
	case "int":
		return "%d"
	case "float":
		return "%f"
	case "string":
		return "%s"
	case "char":
		return "%c"
	case "bool":
		return "%d" // C prints bool as 0/1
	case "array":
		return "%p" // Pointer
	case "dict":
		return "%p" // Pointer
	default:
		return "%d" // Default to int
	}
}

// Get value type for an AST node (simpler version of inferType)
func (gen *CodeGenerator) getValueType(node *ahoy.ASTNode) string {
	switch node.Type {
	case ahoy.NODE_NUMBER:
		// Check if it contains a decimal point
		if strings.Contains(node.Value, ".") {
			return "float"
		}
		return "int"
	case ahoy.NODE_STRING, ahoy.NODE_F_STRING:
		return "string"
	case ahoy.NODE_CHAR:
		return "char"
	case ahoy.NODE_BOOLEAN:
		return "int" // bool stored as int
	case ahoy.NODE_BINARY_OP:
		// Mirror inferType promotion so e.g. a pushed or literal
		// `a.x * b.y` boxes as float instead of truncating to int.
		if len(node.Children) >= 2 {
			leftType := gen.getValueType(node.Children[0])
			rightType := gen.getValueType(node.Children[1])
			if leftType == "float" || leftType == "double" || rightType == "float" || rightType == "double" {
				return "float"
			}
			if (node.Value == "+" || node.Value == "plus") && leftType == "string" && rightType == "string" {
				return "string"
			}
		}
		return "int"
	case ahoy.NODE_ARRAY_LITERAL:
		return "array"
	case ahoy.NODE_DICT_LITERAL:
		return "dict"
	case ahoy.NODE_OBJECT_LITERAL:
		// Struct literal - use the struct type
		if node.Value != "" {
			return node.Value
		}
		return "int"
	case ahoy.NODE_IDENTIFIER:
		// Check if identifier is a variable with a known type
		if varType, exists := gen.variables[node.Value]; exists {
			return varType
		}
		// Parameters are not declarations, so the scanVariableTypes pre-pass
		// misses them; they only appear in functionVars.
		if varType, exists := gen.functionVars[node.Value]; exists {
			return varType
		}
		return "int"
	case ahoy.NODE_MEMBER_ACCESS:
		// obj.field: resolve through the struct so a string field is tagged as a
		// string when it is pushed into an array or dict.
		if len(node.Children) > 0 && node.Children[0].Type == ahoy.NODE_IDENTIFIER {
			objType := strings.TrimPrefix(gen.inferType(node.Children[0]), "struct:")
			if info := gen.structs[objType]; info != nil {
				for _, f := range info.Fields {
					if f.Name != node.Value {
						continue
					}
					ft := f.AhoyType
					if ft == "" {
						ft = f.Type
					}
					switch ft {
					case "char*", "const char*":
						return "string"
					case "double":
						return "float"
					}
					return ft
				}
			}
		}
		return "int"
	case ahoy.NODE_UNARY_OP:
		// A leading minus preserves the operand type: `-10.0` is a float
		// and must be boxed like one in typed arrays. Without this the
		// element falls to the int default and reads back as a boxed
		// double* over an int bit pattern (segfault). Other unary
		// operators keep the historical int fallback.
		if node.Value == "-" && len(node.Children) > 0 {
			return gen.getValueType(node.Children[0])
		}
		return "int"
	default:
		return "int"
	}
}

// Get AhoyValueType enum for a type string
func (gen *CodeGenerator) getAhoyTypeEnum(typeName string) string {
	switch typeName {
	case "int", "bool":
		return "AHOY_TYPE_INT"
	case "float", "double":
		return "AHOY_TYPE_FLOAT"
	case "string", "char*":
		return "AHOY_TYPE_STRING"
	case "char":
		return "AHOY_TYPE_CHAR"
	default:
		// Check if it's a known struct type
		if _, isStruct := gen.structs[typeName]; isStruct {
			return "AHOY_TYPE_STRUCT"
		}
		// Check if the mapped C type is a struct
		cType := gen.mapType(typeName)
		if _, isStruct := gen.structs[cType]; isStruct {
			return "AHOY_TYPE_STRUCT"
		}
		// Check if it's a C struct (defined in cTypeDefinitions)
		if gen.cTypeDefinitions[cType] && !strings.HasSuffix(cType, "*") &&
			cType != "int" && cType != "double" && cType != "char*" && cType != "bool" {
			return "AHOY_TYPE_STRUCT"
		}
		// For unknown types, treat as int (pointer stored as intptr_t)
		return "AHOY_TYPE_INT"
	}
}

// Generate inline map code
func (gen *CodeGenerator) generateMapInline(arrayNode *ahoy.ASTNode, lambda *ahoy.ASTNode) {
	// Parse lambda structure: Value contains param count, first N children are params, last child is body
	paramCount := 1
	if lambda.Value != "" {
		if count, err := strconv.Atoi(lambda.Value); err == nil {
			paramCount = count
		}
	}

	// Extract parameters and body
	params := []string{}
	var bodyExpr *ahoy.ASTNode

	if paramCount == 1 && len(lambda.Children) == 1 {
		// Old format: single param in Value, body is first child
		params = []string{lambda.Value}
		bodyExpr = lambda.Children[0]
	} else if len(lambda.Children) > paramCount {
		// New format: first paramCount children are params, last is body
		for i := 0; i < paramCount; i++ {
			params = append(params, lambda.Children[i].Value)
		}
		bodyExpr = lambda.Children[paramCount]
	} else {
		// Fallback
		params = []string{"x"}
		bodyExpr = lambda.Children[0]
	}

	// Generate inline statement expression
	gen.output.WriteString("({ ")
	gen.output.WriteString("AhoyArray* __src = ")
	gen.generateNodeInternal(arrayNode, false)
	gen.output.WriteString("; ")
	gen.output.WriteString("AhoyArray* __result = malloc(sizeof(AhoyArray)); ")
	gen.output.WriteString("__result->length = __src->length; ")
	gen.output.WriteString("__result->capacity = __src->length; ")
	gen.output.WriteString("__result->data = malloc(__src->length * sizeof(intptr_t)); ")
	gen.output.WriteString("__result->types = malloc(__src->length * sizeof(AhoyValueType)); ")
	gen.output.WriteString("__result->is_typed = 0; ")
	gen.output.WriteString("for (int __i = 0; __i < __src->length; __i++) { ")

	// For multi-param lambdas, extract from nested array
	if len(params) > 1 {
		gen.output.WriteString("AhoyArray* __elem = (AhoyArray*)__src->data[__i]; ")
		for i, paramName := range params {
			gen.output.WriteString("int ")
			gen.output.WriteString(paramName)
			gen.output.WriteString(" = __elem->data[")
			gen.output.WriteString(strconv.Itoa(i))
			gen.output.WriteString("]; ")
		}
	} else {
		gen.output.WriteString("int ")
		gen.output.WriteString(params[0])
		gen.output.WriteString(" = __src->data[__i]; ")
	}

	gen.output.WriteString("__result->types[__i] = AHOY_TYPE_INT; ")
	gen.output.WriteString("__result->data[__i] = (intptr_t)(")

	// Generate lambda body expression
	gen.generateNodeInternal(bodyExpr, false)

	gen.output.WriteString("); } ")
	gen.output.WriteString("__result; })")
}

// Generate inline filter code
func (gen *CodeGenerator) generateFilterInline(arrayNode *ahoy.ASTNode, lambda *ahoy.ASTNode) {
	// Parse lambda structure: Value contains param count, first N children are params, last child is body
	paramCount := 1
	if lambda.Value != "" {
		if count, err := strconv.Atoi(lambda.Value); err == nil {
			paramCount = count
		}
	}

	// Extract parameters and condition
	params := []string{}
	var condExpr *ahoy.ASTNode

	if paramCount == 1 && len(lambda.Children) == 1 {
		// Old format: single param in Value, body is first child
		params = []string{lambda.Value}
		condExpr = lambda.Children[0]
	} else if len(lambda.Children) > paramCount {
		// New format: first paramCount children are params, last is body
		for i := 0; i < paramCount; i++ {
			params = append(params, lambda.Children[i].Value)
		}
		condExpr = lambda.Children[paramCount]
	} else {
		// Fallback
		params = []string{"x"}
		condExpr = lambda.Children[0]
	}

	// Generate inline statement expression
	gen.output.WriteString("({ ")
	gen.output.WriteString("AhoyArray* __src = ")
	gen.generateNodeInternal(arrayNode, false)
	gen.output.WriteString("; ")
	gen.output.WriteString("AhoyArray* __result = malloc(sizeof(AhoyArray)); ")
	gen.output.WriteString("__result->capacity = __src->length; ")
	gen.output.WriteString("__result->data = malloc(__src->length * sizeof(intptr_t)); ")
	gen.output.WriteString("__result->types = malloc(__src->length * sizeof(AhoyValueType)); ")
	gen.output.WriteString("__result->is_typed = 0; ")
	gen.output.WriteString("__result->length = 0; ")
	gen.output.WriteString("for (int __i = 0; __i < __src->length; __i++) { ")

	// For multi-param lambdas, extract from nested array
	if len(params) > 1 {
		gen.output.WriteString("AhoyArray* __elem = (AhoyArray*)__src->data[__i]; ")
		for i, paramName := range params {
			gen.output.WriteString("int ")
			gen.output.WriteString(paramName)
			gen.output.WriteString(" = __elem->data[")
			gen.output.WriteString(strconv.Itoa(i))
			gen.output.WriteString("]; ")
		}
	} else {
		gen.output.WriteString("int ")
		gen.output.WriteString(params[0])
		gen.output.WriteString(" = __src->data[__i]; ")
	}

	gen.output.WriteString("if (")

	// Generate lambda condition expression
	gen.generateNodeInternal(condExpr, false)

	gen.output.WriteString(") { ")
	if len(params) > 1 {
		gen.output.WriteString("__result->types[__result->length] = AHOY_TYPE_INT; ")
		gen.output.WriteString("__result->data[__result->length++] = (intptr_t)__elem; ")
	} else {
		gen.output.WriteString("__result->types[__result->length] = AHOY_TYPE_INT; ")
		gen.output.WriteString("__result->data[__result->length++] = (intptr_t)")
		gen.output.WriteString(params[0])
		gen.output.WriteString("; ")
	}
	gen.output.WriteString("} } ")
	gen.output.WriteString("__result; })")
}

func (gen *CodeGenerator) writeTypeConstructors() {
	// Note: Vector2 and Color constructors removed
	// These types should be provided by imported libraries (e.g., raylib)
	// If needed, users can define their own constructor functions
}

func (gen *CodeGenerator) writeStructHelperFunctions() {
	// Generate print helper for each struct type
	// Track which structs we've processed to avoid duplicates (since we store both lowercase and capitalized)
	processed := make(map[string]bool)

	// Sort struct names for deterministic output
	structNames := make([]string, 0, len(gen.structs))
	for name := range gen.structs {
		structNames = append(structNames, name)
	}
	sort.Strings(structNames)

	// First pass: Add forward declarations
	for _, name := range structNames {
		structInfo := gen.structs[name]
		if processed[structInfo.Name] {
			continue
		}
		// Skip JSON structs - they don't have C representations
		if gen.jsonStructs[structInfo.Name] {
			continue
		}
		processed[structInfo.Name] = true
		cStructName := toCStructName(structInfo.Name)
		funcName := toCFuncName(structInfo.Name)
		gen.funcForwardDecls.WriteString("char* print_struct_helper_")
		gen.funcForwardDecls.WriteString(funcName)
		gen.funcForwardDecls.WriteString("(")
		gen.funcForwardDecls.WriteString(cStructName)
		gen.funcForwardDecls.WriteString(" obj);\n")
	}

	// Second pass: Add implementations
	processed = make(map[string]bool)
	for _, name := range structNames {
		structInfo := gen.structs[name]
		// Skip if already processed (avoid duplicates from lowercase/capitalized pairs)
		if processed[structInfo.Name] {
			continue
		}
		// Skip JSON structs - they don't have C representations
		if gen.jsonStructs[structInfo.Name] {
			continue
		}
		processed[structInfo.Name] = true

		cStructName := toCStructName(structInfo.Name)
		funcName := toCFuncName(structInfo.Name)
		gen.funcDecls.WriteString("\n// Print helper for ")
		gen.funcDecls.WriteString(structInfo.Name)
		gen.funcDecls.WriteString("\n")
		gen.funcDecls.WriteString("char* print_struct_helper_")
		gen.funcDecls.WriteString(funcName)
		gen.funcDecls.WriteString("(")
		gen.funcDecls.WriteString(cStructName)
		gen.funcDecls.WriteString(" obj) {\n")
		// Use malloc instead of static buffer to avoid overwrites in nested calls
		gen.funcDecls.WriteString("    char* buffer = malloc(1024);\n")
		gen.funcDecls.WriteString("    if (!buffer) return \"<out of memory>\";\n")

		// Anonymous structs use {} format, named structs use name{} format
		if strings.HasPrefix(structInfo.Name, "__anon_struct_") {
			gen.funcDecls.WriteString("    sprintf(buffer, \"{")
		} else {
			gen.funcDecls.WriteString("    sprintf(buffer, \"")
			gen.funcDecls.WriteString(structInfo.Name)
			gen.funcDecls.WriteString("{")
		}

		for i, field := range structInfo.Fields {
			if i > 0 {
				gen.funcDecls.WriteString(", ")
			}
			gen.funcDecls.WriteString(field.Name)
			// Anonymous structs use ": " (space), named structs use ":" (no space)
			if strings.HasPrefix(structInfo.Name, "__anon_struct_") {
				gen.funcDecls.WriteString(": ")
			} else {
				gen.funcDecls.WriteString(":")
			}

			// Add format specifier based on field type
			switch field.Type {
			case "int":
				gen.funcDecls.WriteString("%d")
			case "float", "double":
				gen.funcDecls.WriteString("%g")
			case "char*", "const char*":
				gen.funcDecls.WriteString("\\\"%s\\\"")
			case "char":
				gen.funcDecls.WriteString("%c")
			case "bool":
				gen.funcDecls.WriteString("%s")
			case "AhoyArray*":
				gen.funcDecls.WriteString("[]") // Show as empty array
			case "HashMap*":
				gen.funcDecls.WriteString("<>") // Show as empty dict
			default:
				// Check if it's a struct type that has a print helper
				if _, isStruct := gen.structs[strings.ToLower(field.Type)]; isStruct {
					gen.funcDecls.WriteString("%s")
				} else {
					gen.funcDecls.WriteString("%p")
				}
			}
		}

		// Close with } for all structs
		gen.funcDecls.WriteString("}\"")

		// Collect field values (arrays and dicts are already in the format string).
		// The separator must only be written when a value follows: a struct whose
		// fields are all arrays emitted `sprintf(buffer, "...", )` and the C
		// compiler rejected the empty expression.
		var values []string
		for _, field := range structInfo.Fields {
			// Skip arrays and dicts - they're already in the format string
			if field.Type == "AhoyArray*" || field.Type == "HashMap*" {
				continue
			}

			// For static fields, access the global variable instead of obj.field
			if field.IsStatic {
				staticFieldName := fmt.Sprintf("%s_%s", cStructName, field.Name)
				if field.Type == "bool" {
					values = append(values, fmt.Sprintf("%s ? \"true\" : \"false\"", staticFieldName))
				} else {
					values = append(values, staticFieldName)
				}
			} else if field.Type == "bool" {
				values = append(values, fmt.Sprintf("obj.%s ? \"true\" : \"false\"", field.Name))
			} else if field.Type == "char*" || field.Type == "const char*" {
				values = append(values, fmt.Sprintf("(obj.%s ? obj.%s : \"\")", field.Name, field.Name))
			} else if _, isStruct := gen.structs[strings.ToLower(field.Type)]; isStruct {
				// For struct fields, call their print helper
				values = append(values, fmt.Sprintf("print_struct_helper_%s(obj.%s)", strings.ToLower(field.Type), field.Name))
			} else {
				values = append(values, fmt.Sprintf("obj.%s", field.Name))
			}
		}
		if len(values) > 0 {
			gen.funcDecls.WriteString(", ")
			gen.funcDecls.WriteString(strings.Join(values, ", "))
		}

		gen.funcDecls.WriteString(");\n")
		gen.funcDecls.WriteString("    return buffer;\n")
		gen.funcDecls.WriteString("}\n")
	}
}

func (gen *CodeGenerator) writeStringHelperFunctions() {
	if len(gen.stringMethods) == 0 {
		return
	}

	gen.includes["ctype.h"] = true // For tolower/toupper/isspace in strip
	if !contains(gen.orderedIncludes, "ctype.h") {
		gen.orderedIncludes = append(gen.orderedIncludes, "ctype.h")
	}
	gen.includes["regex.h"] = true // For regex matching

	// Helper function to duplicate strings
	gen.funcDecls.WriteString("\n// String Helper Functions\n")
	gen.funcDecls.WriteString("char* ahoy_string_dup(const char* src) {\n")
	gen.funcDecls.WriteString("    if (!src) return NULL;\n")
	gen.funcDecls.WriteString("    char* dest = malloc(strlen(src) + 1);\n")
	gen.funcDecls.WriteString("    strcpy(dest, src);\n")
	gen.funcDecls.WriteString("    return dest;\n")
	gen.funcDecls.WriteString("}\n\n")

	// Concatenation helper (+ on two strings)
	if gen.stringMethods["concat"] {
		gen.funcDecls.WriteString("char* ahoy_string_concat(const char* a, const char* b) {\n")
		gen.funcDecls.WriteString("    if (!a) a = \"\";\n")
		gen.funcDecls.WriteString("    if (!b) b = \"\";\n")
		gen.funcDecls.WriteString("    size_t la = strlen(a), lb = strlen(b);\n")
		gen.funcDecls.WriteString("    char* out = malloc(la + lb + 1);\n")
		gen.funcDecls.WriteString("    memcpy(out, a, la);\n")
		gen.funcDecls.WriteString("    memcpy(out + la, b, lb + 1);\n")
		gen.funcDecls.WriteString("    return out;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// length method
	if gen.stringMethods["length"] {
		gen.funcDecls.WriteString("int ahoy_string_length(const char* str) {\n")
		gen.funcDecls.WriteString("    return str ? strlen(str) : 0;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// upper method
	if gen.stringMethods["upper"] {
		gen.funcDecls.WriteString("char* ahoy_string_upper(const char* str) {\n")
		gen.funcDecls.WriteString("    if (!str) return NULL;\n")
		gen.funcDecls.WriteString("    char* result = ahoy_string_dup(str);\n")
		gen.funcDecls.WriteString("    for (int i = 0; result[i]; i++) {\n")
		gen.funcDecls.WriteString("        result[i] = toupper(result[i]);\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    return result;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// lower method
	if gen.stringMethods["lower"] {
		gen.funcDecls.WriteString("char* ahoy_string_lower(const char* str) {\n")
		gen.funcDecls.WriteString("    if (!str) return NULL;\n")
		gen.funcDecls.WriteString("    char* result = ahoy_string_dup(str);\n")
		gen.funcDecls.WriteString("    for (int i = 0; result[i]; i++) {\n")
		gen.funcDecls.WriteString("        result[i] = tolower(result[i]);\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    return result;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// replace method
	if gen.stringMethods["replace"] {
		gen.funcDecls.WriteString("char* ahoy_string_replace(const char* str, const char* old, const char* new_str) {\n")
		gen.funcDecls.WriteString("    if (!str || !old || !new_str) return ahoy_string_dup(str);\n")
		gen.funcDecls.WriteString("    int count = 0;\n")
		gen.funcDecls.WriteString("    const char* tmp = str;\n")
		gen.funcDecls.WriteString("    while ((tmp = strstr(tmp, old))) {\n")
		gen.funcDecls.WriteString("        count++;\n")
		gen.funcDecls.WriteString("        tmp += strlen(old);\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    int old_len = strlen(old);\n")
		gen.funcDecls.WriteString("    int new_len = strlen(new_str);\n")
		gen.funcDecls.WriteString("    int result_len = strlen(str) + count * (new_len - old_len);\n")
		gen.funcDecls.WriteString("    char* result = malloc(result_len + 1);\n")
		gen.funcDecls.WriteString("    char* ptr = result;\n")
		gen.funcDecls.WriteString("    while (*str) {\n")
		gen.funcDecls.WriteString("        if (strstr(str, old) == str) {\n")
		gen.funcDecls.WriteString("            strcpy(ptr, new_str);\n")
		gen.funcDecls.WriteString("            ptr += new_len;\n")
		gen.funcDecls.WriteString("            str += old_len;\n")
		gen.funcDecls.WriteString("        } else {\n")
		gen.funcDecls.WriteString("            *ptr++ = *str++;\n")
		gen.funcDecls.WriteString("        }\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    *ptr = '\\0';\n")
		gen.funcDecls.WriteString("    return result;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// contains method
	if gen.stringMethods["contains"] {
		gen.funcDecls.WriteString("bool ahoy_string_contains(const char* str, const char* substr) {\n")
		gen.funcDecls.WriteString("    if (!str || !substr) return false;\n")
		gen.funcDecls.WriteString("    return strstr(str, substr) != NULL;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// strip method
	if gen.stringMethods["strip"] {
		gen.funcDecls.WriteString("char* ahoy_string_strip(const char* str) {\n")
		gen.funcDecls.WriteString("    if (!str) return NULL;\n")
		gen.funcDecls.WriteString("    while (*str && isspace(*str)) str++;\n")
		gen.funcDecls.WriteString("    if (!*str) return ahoy_string_dup(\"\");\n")
		gen.funcDecls.WriteString("    const char* end = str + strlen(str) - 1;\n")
		gen.funcDecls.WriteString("    while (end > str && isspace(*end)) end--;\n")
		gen.funcDecls.WriteString("    int len = end - str + 1;\n")
		gen.funcDecls.WriteString("    char* result = malloc(len + 1);\n")
		gen.funcDecls.WriteString("    strncpy(result, str, len);\n")
		gen.funcDecls.WriteString("    result[len] = '\\0';\n")
		gen.funcDecls.WriteString("    return result;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// count method
	if gen.stringMethods["count"] {
		gen.funcDecls.WriteString("int ahoy_string_count(const char* str, const char* substr) {\n")
		gen.funcDecls.WriteString("    if (!str || !substr) return 0;\n")
		gen.funcDecls.WriteString("    int count = 0;\n")
		gen.funcDecls.WriteString("    const char* tmp = str;\n")
		gen.funcDecls.WriteString("    while ((tmp = strstr(tmp, substr))) {\n")
		gen.funcDecls.WriteString("        count++;\n")
		gen.funcDecls.WriteString("        tmp += strlen(substr);\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    return count;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// lpad method
	if gen.stringMethods["lpad"] {
		gen.funcDecls.WriteString("char* ahoy_string_lpad(const char* str, int length, const char* pad) {\n")
		gen.funcDecls.WriteString("    if (!str || !pad) return ahoy_string_dup(str);\n")
		gen.funcDecls.WriteString("    int str_len = strlen(str);\n")
		gen.funcDecls.WriteString("    if (str_len >= length) return ahoy_string_dup(str);\n")
		gen.funcDecls.WriteString("    int pad_len = length - str_len;\n")
		gen.funcDecls.WriteString("    char* result = malloc(length + 1);\n")
		gen.funcDecls.WriteString("    int pad_char_len = strlen(pad);\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < pad_len; i++) {\n")
		gen.funcDecls.WriteString("        result[i] = pad[i % pad_char_len];\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    strcpy(result + pad_len, str);\n")
		gen.funcDecls.WriteString("    return result;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// rpad method
	if gen.stringMethods["rpad"] {
		gen.funcDecls.WriteString("char* ahoy_string_rpad(const char* str, int length, const char* pad) {\n")
		gen.funcDecls.WriteString("    if (!str || !pad) return ahoy_string_dup(str);\n")
		gen.funcDecls.WriteString("    int str_len = strlen(str);\n")
		gen.funcDecls.WriteString("    if (str_len >= length) return ahoy_string_dup(str);\n")
		gen.funcDecls.WriteString("    int pad_len = length - str_len;\n")
		gen.funcDecls.WriteString("    char* result = malloc(length + 1);\n")
		gen.funcDecls.WriteString("    strcpy(result, str);\n")
		gen.funcDecls.WriteString("    int pad_char_len = strlen(pad);\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < pad_len; i++) {\n")
		gen.funcDecls.WriteString("        result[str_len + i] = pad[i % pad_char_len];\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    result[length] = '\\0';\n")
		gen.funcDecls.WriteString("    return result;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// pad method
	if gen.stringMethods["pad"] {
		gen.funcDecls.WriteString("char* ahoy_string_pad(const char* str, int length, const char* pad) {\n")
		gen.funcDecls.WriteString("    if (!str || !pad) return ahoy_string_dup(str);\n")
		gen.funcDecls.WriteString("    int str_len = strlen(str);\n")
		gen.funcDecls.WriteString("    if (str_len >= length) return ahoy_string_dup(str);\n")
		gen.funcDecls.WriteString("    int total_pad = length - str_len;\n")
		gen.funcDecls.WriteString("    int left_pad = total_pad / 2;\n")
		gen.funcDecls.WriteString("    int right_pad = total_pad - left_pad;\n")
		gen.funcDecls.WriteString("    char* result = malloc(length + 1);\n")
		gen.funcDecls.WriteString("    int pad_char_len = strlen(pad);\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < left_pad; i++) {\n")
		gen.funcDecls.WriteString("        result[i] = pad[i % pad_char_len];\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    strcpy(result + left_pad, str);\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < right_pad; i++) {\n")
		gen.funcDecls.WriteString("        result[left_pad + str_len + i] = pad[i % pad_char_len];\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    result[length] = '\\0';\n")
		gen.funcDecls.WriteString("    return result;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// match method (regex)
	if gen.stringMethods["match"] {
		gen.funcDecls.WriteString("bool ahoy_string_match(const char* str, const char* pattern) {\n")
		gen.funcDecls.WriteString("    if (!str || !pattern) return false;\n")
		gen.funcDecls.WriteString("    regex_t regex;\n")
		gen.funcDecls.WriteString("    int ret = regcomp(&regex, pattern, REG_EXTENDED | REG_NOSUB);\n")
		gen.funcDecls.WriteString("    if (ret) return false;\n")
		gen.funcDecls.WriteString("    ret = regexec(&regex, str, 0, NULL, 0);\n")
		gen.funcDecls.WriteString("    regfree(&regex);\n")
		gen.funcDecls.WriteString("    return ret == 0;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// get_file method
	if gen.stringMethods["get_file"] {
		gen.funcDecls.WriteString("char* ahoy_string_get_file(const char* path) {\n")
		gen.funcDecls.WriteString("    if (!path) return NULL;\n")
		gen.funcDecls.WriteString("    const char* filename = strrchr(path, '/');\n")
		gen.funcDecls.WriteString("    if (!filename) filename = strrchr(path, '\\\\');\n")
		gen.funcDecls.WriteString("    if (!filename) return ahoy_string_dup(path);\n")
		gen.funcDecls.WriteString("    return ahoy_string_dup(filename + 1);\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// Case conversion methods - these are more complex, so provide simplified versions
	if gen.stringMethods["camel_case"] {
		gen.funcDecls.WriteString("char* ahoy_string_camel_case(const char* str) {\n")
		gen.funcDecls.WriteString("    if (!str) return NULL;\n")
		gen.funcDecls.WriteString("    char* result = malloc(strlen(str) + 1);\n")
		gen.funcDecls.WriteString("    int j = 0;\n")
		gen.funcDecls.WriteString("    bool capitalize_next = false;\n")
		gen.funcDecls.WriteString("    bool first = true;\n")
		gen.funcDecls.WriteString("    for (int i = 0; str[i]; i++) {\n")
		gen.funcDecls.WriteString("        if (str[i] == ' ' || str[i] == '_' || str[i] == '-') {\n")
		gen.funcDecls.WriteString("            capitalize_next = true;\n")
		gen.funcDecls.WriteString("        } else if (capitalize_next) {\n")
		gen.funcDecls.WriteString("            result[j++] = toupper(str[i]);\n")
		gen.funcDecls.WriteString("            capitalize_next = false;\n")
		gen.funcDecls.WriteString("        } else if (first) {\n")
		gen.funcDecls.WriteString("            result[j++] = tolower(str[i]);\n")
		gen.funcDecls.WriteString("            first = false;\n")
		gen.funcDecls.WriteString("        } else {\n")
		gen.funcDecls.WriteString("            result[j++] = str[i];\n")
		gen.funcDecls.WriteString("        }\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    result[j] = '\\0';\n")
		gen.funcDecls.WriteString("    return result;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	if gen.stringMethods["snake_case"] {
		gen.funcDecls.WriteString("char* ahoy_string_snake_case(const char* str) {\n")
		gen.funcDecls.WriteString("    if (!str) return NULL;\n")
		gen.funcDecls.WriteString("    char* result = malloc(strlen(str) * 2 + 1);\n")
		gen.funcDecls.WriteString("    int j = 0;\n")
		gen.funcDecls.WriteString("    for (int i = 0; str[i]; i++) {\n")
		gen.funcDecls.WriteString("        if (str[i] == ' ' || str[i] == '-') {\n")
		gen.funcDecls.WriteString("            result[j++] = '_';\n")
		gen.funcDecls.WriteString("        } else if (isupper(str[i]) && i > 0) {\n")
		gen.funcDecls.WriteString("            result[j++] = '_';\n")
		gen.funcDecls.WriteString("            result[j++] = tolower(str[i]);\n")
		gen.funcDecls.WriteString("        } else {\n")
		gen.funcDecls.WriteString("            result[j++] = tolower(str[i]);\n")
		gen.funcDecls.WriteString("        }\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    result[j] = '\\0';\n")
		gen.funcDecls.WriteString("    return result;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	if gen.stringMethods["pascal_case"] {
		gen.funcDecls.WriteString("char* ahoy_string_pascal_case(const char* str) {\n")
		gen.funcDecls.WriteString("    if (!str) return NULL;\n")
		gen.funcDecls.WriteString("    char* result = malloc(strlen(str) + 1);\n")
		gen.funcDecls.WriteString("    int j = 0;\n")
		gen.funcDecls.WriteString("    bool capitalize_next = true;\n")
		gen.funcDecls.WriteString("    for (int i = 0; str[i]; i++) {\n")
		gen.funcDecls.WriteString("        if (str[i] == ' ' || str[i] == '_' || str[i] == '-') {\n")
		gen.funcDecls.WriteString("            capitalize_next = true;\n")
		gen.funcDecls.WriteString("        } else if (capitalize_next) {\n")
		gen.funcDecls.WriteString("            result[j++] = toupper(str[i]);\n")
		gen.funcDecls.WriteString("            capitalize_next = false;\n")
		gen.funcDecls.WriteString("        } else {\n")
		gen.funcDecls.WriteString("            result[j++] = tolower(str[i]);\n")
		gen.funcDecls.WriteString("        }\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    result[j] = '\\0';\n")
		gen.funcDecls.WriteString("    return result;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	if gen.stringMethods["kebab_case"] {
		gen.funcDecls.WriteString("char* ahoy_string_kebab_case(const char* str) {\n")
		gen.funcDecls.WriteString("    if (!str) return NULL;\n")
		gen.funcDecls.WriteString("    char* result = malloc(strlen(str) * 2 + 1);\n")
		gen.funcDecls.WriteString("    int j = 0;\n")
		gen.funcDecls.WriteString("    for (int i = 0; str[i]; i++) {\n")
		gen.funcDecls.WriteString("        if (str[i] == ' ' || str[i] == '_') {\n")
		gen.funcDecls.WriteString("            result[j++] = '-';\n")
		gen.funcDecls.WriteString("        } else if (isupper(str[i]) && i > 0) {\n")
		gen.funcDecls.WriteString("            result[j++] = '-';\n")
		gen.funcDecls.WriteString("            result[j++] = tolower(str[i]);\n")
		gen.funcDecls.WriteString("        } else {\n")
		gen.funcDecls.WriteString("            result[j++] = tolower(str[i]);\n")
		gen.funcDecls.WriteString("        }\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    result[j] = '\\0';\n")
		gen.funcDecls.WriteString("    return result;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// split method - returns array of strings (simplified)
	if gen.stringMethods["split"] {
		// Generates elements as an array; an empty piece stays an element:
		// "a,,b" splits into three. strtok used to collapse runs and returned
		// char**, which the array-typed result could not use.
		gen.funcDecls.WriteString("// Note: split returns an AhoyArray of strings\n")
		gen.funcDecls.WriteString("AhoyArray* ahoy_string_split_array(const char* str, const char* delim) {\n")
		gen.funcDecls.WriteString("    AhoyArray* arr = (AhoyArray*)malloc(sizeof(AhoyArray));\n")
		gen.funcDecls.WriteString("    arr->length = 0;\n")
		gen.funcDecls.WriteString("    arr->capacity = 4;\n")
		gen.funcDecls.WriteString("    arr->data = (intptr_t*)malloc(4 * sizeof(intptr_t));\n")
		gen.funcDecls.WriteString("    arr->types = (AhoyValueType*)malloc(4 * sizeof(AhoyValueType));\n")
		gen.funcDecls.WriteString("    arr->is_typed = 1;\n")
		gen.funcDecls.WriteString("    arr->element_type = AHOY_TYPE_STRING;\n")
		gen.funcDecls.WriteString("    if (!str) return arr;\n")
		gen.funcDecls.WriteString("    size_t dlen = delim ? strlen(delim) : 0;\n")
		gen.funcDecls.WriteString("    const char* start = str;\n")
		gen.funcDecls.WriteString("    for (;;) {\n")
		gen.funcDecls.WriteString("        const char* hit = dlen ? strstr(start, delim) : NULL;\n")
		gen.funcDecls.WriteString("        size_t len = hit ? (size_t)(hit - start) : strlen(start);\n")
		gen.funcDecls.WriteString("        char* piece = (char*)malloc(len + 1);\n")
		gen.funcDecls.WriteString("        memcpy(piece, start, len);\n")
		gen.funcDecls.WriteString("        piece[len] = '\\0';\n")
		gen.funcDecls.WriteString("        if (arr->length == arr->capacity) {\n")
		gen.funcDecls.WriteString("            arr->capacity *= 2;\n")
		gen.funcDecls.WriteString("            arr->data = (intptr_t*)realloc(arr->data, arr->capacity * sizeof(intptr_t));\n")
		gen.funcDecls.WriteString("            arr->types = (AhoyValueType*)realloc(arr->types, arr->capacity * sizeof(AhoyValueType));\n")
		gen.funcDecls.WriteString("        }\n")
		gen.funcDecls.WriteString("        arr->data[arr->length] = (intptr_t)piece;\n")
		gen.funcDecls.WriteString("        arr->types[arr->length] = AHOY_TYPE_STRING;\n")
		gen.funcDecls.WriteString("        arr->length++;\n")
		gen.funcDecls.WriteString("        if (!hit) break;\n")
		gen.funcDecls.WriteString("        start = hit + dlen;\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    return arr;\n")
		gen.funcDecls.WriteString("}\n\n")
	}
}

func (gen *CodeGenerator) generateObjectLiteral(node *ahoy.ASTNode) {
	// Generate compound literal initialization
	// If node.Value is set, it's a typed literal (e.g., rectangle{...} or vector2{...})
	// If node.Value is empty, it's an anonymous object - use HashMap

	structName := ""
	if node.Value != "" {
		// Typed object literal - use mapType to get correct C struct name
		structName = gen.mapType(node.Value)

		// Check if this is a known Ahoy struct type
		_, hasStructInfo := gen.structs[node.Value]
		if !hasStructInfo {
			_, hasStructInfo = gen.structs[structName]
		}

		// For typed object literals, generate C struct initialization even if we don't have
		// the full struct definition (e.g., C structs from imported headers like Color, Texture2D)
		// Trust that if node.Value is set, the parser validated it's a valid type
		gen.output.WriteString("(")
		gen.output.WriteString(structName)
		gen.output.WriteString(")")
	} else {
		// Anonymous object - use HashMap
		gen.generateAnonymousObject(node)
		return
	}

	gen.output.WriteString("{")

	// Collect explicitly set properties
	explicitProps := make(map[string]bool)
	for _, prop := range node.Children {
		if prop.Type == ahoy.NODE_OBJECT_PROPERTY {
			explicitProps[prop.Value] = true
		}
	}

	// If this is a typed literal with a struct definition, apply defaults
	structInfo, hasStructInfo := gen.structs[node.Value]
	if !hasStructInfo && structName != "" {
		structInfo, hasStructInfo = gen.structs[structName]
	}

	first := true
	if hasStructInfo {
		// Generate all fields with defaults or explicit values
		for _, field := range structInfo.Fields {
			// Skip static fields - they are not part of the struct instance
			if field.IsStatic {
				continue
			}

			if !first {
				gen.output.WriteString(", ")
			}
			gen.output.WriteString(".")
			gen.output.WriteString(field.Name)
			gen.output.WriteString(" = ")

			// Check if this field was explicitly set
			fieldSet := false
			for _, prop := range node.Children {
				if prop.Type == ahoy.NODE_OBJECT_PROPERTY && prop.Value == field.Name {
					// A lambda stored in a function-typed field needs the field's
					// signature: the declared type supplies the thunk's parameter
					// and return types, and without it a body that infers wide
					// (a dict read is double) is called through the field's
					// function pointer with the wrong ABI.
					savedPending := gen.pendingFuncType
					if ft := field.AhoyType; ahoy.IsFunctionType(ft) {
						gen.pendingFuncType = ft
					} else {
						gen.pendingFuncType = ""
					}
					// If the value is an f-string, wrap in strdup (f-strings use static buffers)
					// This is critical for struct literals that escape or are stored
					valueNode := prop.Children[0]
					if valueNode.Type == ahoy.NODE_F_STRING {
						gen.output.WriteString("strdup(")
						gen.generateNodeInternal(valueNode, false)
						gen.output.WriteString(")")
					} else {
						gen.generateNodeInternal(valueNode, false)
					}
					gen.pendingFuncType = savedPending
					fieldSet = true
					break
				}
			}

			// If not explicitly set, use default value or type default
			if !fieldSet {
				if field.DefaultValue != "" {
					gen.output.WriteString(field.DefaultValue)
				} else {
					// Use type-specific zero value
					gen.output.WriteString(gen.getTypeDefault(field.Type))
				}
			}
			first = false
		}
	} else {
		// No struct info, just output explicit properties
		for _, prop := range node.Children {
			if prop.Type == ahoy.NODE_OBJECT_PROPERTY {
				if !first {
					gen.output.WriteString(", ")
				}
				gen.output.WriteString(".")
				gen.output.WriteString(prop.Value)
				gen.output.WriteString(" = ")

				// If the value is an f-string, wrap in strdup (for string fields)
				// This is a heuristic - we assume text/name/message fields are strings
				propertyName := strings.ToLower(prop.Value)
				isLikelyString := strings.Contains(propertyName, "text") ||
					strings.Contains(propertyName, "name") ||
					strings.Contains(propertyName, "message") ||
					strings.Contains(propertyName, "str")

				if isLikelyString && prop.Children[0].Type == ahoy.NODE_F_STRING {
					gen.output.WriteString("strdup(")
					gen.generateNodeInternal(prop.Children[0], false)
					gen.output.WriteString(")")
				} else {
					gen.generateNodeInternal(prop.Children[0], false)
				}
				first = false
			}
		}
	}

	gen.output.WriteString("}")
}

// generateAnonymousObject generates a HashMap for anonymous object literals
func (gen *CodeGenerator) generateAnonymousObject(node *ahoy.ASTNode) {
	dictName := fmt.Sprintf("dict_%d", gen.varCounter)
	gen.varCounter++

	gen.output.WriteString("({ HashMap* ")
	gen.output.WriteString(dictName)
	gen.output.WriteString(" = createHashMap(16); ")

	// Add properties
	for _, prop := range node.Children {
		if prop.Type == ahoy.NODE_OBJECT_PROPERTY {
			// Determine value type
			var valueType string
			if len(prop.Children) > 0 {
				valueType = gen.inferType(prop.Children[0])
			} else {
				valueType = "string"
			}

			ahoyTypeEnum := "AHOY_TYPE_STRING"
			switch valueType {
			case "int":
				ahoyTypeEnum = "AHOY_TYPE_INT"
			case "float":
				ahoyTypeEnum = "AHOY_TYPE_FLOAT"
			case "char":
				ahoyTypeEnum = "AHOY_TYPE_CHAR"
			default:
				ahoyTypeEnum = "AHOY_TYPE_STRING"
			}

			gen.output.WriteString("hashMapPutTyped(")
			gen.output.WriteString(dictName)
			gen.output.WriteString(", \"")
			gen.output.WriteString(prop.Value)
			gen.output.WriteString("\", (void*)(intptr_t)")
			if len(prop.Children) > 0 {
				gen.generateNode(prop.Children[0])
			} else {
				gen.output.WriteString("0")
			}
			gen.output.WriteString(", ")
			gen.output.WriteString(ahoyTypeEnum)
			gen.output.WriteString("); ")
		}
	}

	gen.output.WriteString(dictName)
	gen.output.WriteString("; })")
}

// capitalizeFirst capitalizes the first letter of a string
func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (gen *CodeGenerator) generateObjectAccess(node *ahoy.ASTNode) {
	// Object property access: person<'name'>
	// If the object is a HashMap (dict or generic), use hashMapGet
	// Otherwise use struct field access (person.name)

	objectName := node.Value
	propertyName := ""
	if len(node.Children) > 0 && node.Children[0].Type == ahoy.NODE_STRING {
		propertyName = node.Children[0].Value
	}

	// Check if this is a HashMap/dict or generic parameter
	objectType := ""
	if varType, exists := gen.variables[objectName]; exists {
		objectType = varType
	} else if varType, exists := gen.functionVars[objectName]; exists {
		objectType = varType
	}

	// If object is dict, HashMap*, generic, any, or intptr_t, use hashMapGet
	if objectType == "dict" || objectType == "HashMap*" || objectType == "generic" || objectType == "any" || objectType == "intptr_t" ||
		strings.HasPrefix(objectType, "dict[") || strings.HasPrefix(objectType, "dict<") {
		gen.output.WriteString("((char*)hashMapGet(")
		// Cast generic/any/intptr_t to HashMap*
		if objectType == "generic" || objectType == "any" || objectType == "intptr_t" {
			gen.output.WriteString("(HashMap*)")
		}
		gen.output.WriteString(objectName)
		gen.output.WriteString(", \"")
		gen.output.WriteString(propertyName)
		gen.output.WriteString("\"))")
	} else {
		// Struct field access
		gen.output.WriteString(objectName)
		gen.output.WriteString(".")
		gen.output.WriteString(propertyName)
	}
}

// splitReturnTypes splits a comma-separated list of return types, handling nested commas in dict<k,v>
func splitReturnTypes(typeStr string) []string {
	if typeStr == "" {
		return []string{}
	}

	var types []string
	var current strings.Builder
	depth := 0 // Track nesting level in <> or []

	for i := 0; i < len(typeStr); i++ {
		ch := typeStr[i]
		switch ch {
		case '<', '[':
			depth++
			current.WriteByte(ch)
		case '>', ']':
			depth--
			current.WriteByte(ch)
		case ',':
			if depth == 0 {
				// Top-level comma, split here
				types = append(types, strings.TrimSpace(current.String()))
				current.Reset()
			} else {
				// Nested comma, keep it
				current.WriteByte(ch)
			}
		default:
			current.WriteByte(ch)
		}
	}

	// Add the last type
	if current.Len() > 0 {
		types = append(types, strings.TrimSpace(current.String()))
	}

	return types
}

// tryResolveEnumMember attempts to resolve a simple identifier to an enum member
// Returns the fully qualified name (enumName_MEMBER) if found, empty string otherwise
func (gen *CodeGenerator) tryResolveEnumMember(memberName string) string {
	// A name that the program actually declares wins over an enum member of the
	// same name: `rect: node_rect|...|` in a game compiled to NodeKind_rect.x,
	// the enum member, and the C compiler rejected the assignment.
	if memberName != "" && memberName[0] != '.' {
		if _, isVar := gen.functionVars[memberName]; isVar {
			return ""
		}
		if _, isVar := gen.variables[memberName]; isVar {
			return ""
		}
		if gen.userFunctions[memberName] {
			return ""
		}
	}

	// Check if this is a dot-prefixed enum member
	if len(memberName) > 0 && memberName[0] == '.' {
		// Extract member name without dot
		actualMemberName := memberName[1:]

		// Search only original enum names (not capitalized aliases) for deterministic results
		var foundEnum string

		for _, enumName := range gen.sortedEnumNames() {
			if gen.enums[enumName] != nil && gen.enums[enumName][actualMemberName] {
				foundEnum = enumName
				break
			}
		}

		if foundEnum != "" {
			// Check if this is an int enum
			if gen.enumTypes[foundEnum] == "int" || gen.enumTypes[foundEnum] == "" {
				return foundEnum + "_" + actualMemberName
			}
			// For struct-based enums, use the struct accessor
			return foundEnum + "." + actualMemberName
		}

		return ""
	}

	// Check only original enum names for deterministic results
	for _, enumName := range gen.sortedEnumNames() {
		if gen.enums[enumName] != nil && gen.enums[enumName][memberName] {
			// Check if this is an int enum
			if gen.enumTypes[enumName] == "int" || gen.enumTypes[enumName] == "" {
				return enumName + "_" + memberName
			}
			// For struct-based enums, use the struct accessor
			return enumName + "." + memberName
		}
	}

	return ""
}

// Helper function to check if a string slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// recordJobResultType remembers the result type of a submitted task so that
// pool.wait|job| can cast the runtime's intptr_t back to an Ahoy type.
func (gen *CodeGenerator) recordJobResultType(node *ahoy.ASTNode) {
	if node == nil || node.Value == "" {
		return
	}
	var value *ahoy.ASTNode
	if node.Type == ahoy.NODE_VARIABLE_DECLARATION {
		if len(node.Children) > 0 {
			value = node.Children[0]
		}
	} else if len(node.Children) > 0 {
		value = node.Children[len(node.Children)-1]
	}
	if value == nil || value.Type != ahoy.NODE_METHOD_CALL || value.Value != "submit" {
		return
	}
	if len(value.Children) < 2 || len(value.Children[1].Children) == 0 {
		return
	}
	task := value.Children[1].Children[0]
	ret := ""
	if task.Type == ahoy.NODE_UNARY_OP && task.Value == "&" && len(task.Children) == 1 &&
		task.Children[0].Type == ahoy.NODE_IDENTIFIER {
		if rets := gen.functionReturnTypes[task.Children[0].Value]; len(rets) == 1 {
			ret = rets[0]
		}
	} else if task.Type == ahoy.NODE_IDENTIFIER {
		if ft, ok := gen.functionValueType(task.Value); ok {
			if _, r, ok := ahoy.ParseFunctionType(ft); ok {
				ret = r
			}
		}
	} else if task.Type == ahoy.NODE_LAMBDA {
		ret = gen.inferType(task.Children[len(task.Children)-1])
	}
	if ret != "" && ret != "unknown" {
		gen.jobResultTypes[node.Value] = ret
	}
}

// isFloatAhoyType reports whether a type travels as a C double.
func isFloatAhoyType(t string) bool {
	return t == "float" || t == "double"
}

// taskSignature returns the parameter and return Ahoy types of a task value:
// a named function reference, a function-typed variable or a lambda.
func (gen *CodeGenerator) taskSignature(task *ahoy.ASTNode) (string, string, bool) {
	if task == nil {
		return "", "", false
	}
	if task.Type == ahoy.NODE_UNARY_OP && task.Value == "&" && len(task.Children) == 1 &&
		task.Children[0].Type == ahoy.NODE_IDENTIFIER {
		name := task.Children[0].Value
		params := gen.functionParamTypes[name]
		rets := gen.functionReturnTypes[name]
		if len(params) == 1 && len(rets) == 1 {
			return params[0], rets[0], true
		}
		return "", "", false
	}
	if task.Type == ahoy.NODE_IDENTIFIER {
		if ft, ok := gen.functionValueType(task.Value); ok {
			if ps, r, ok := ahoy.ParseFunctionType(ft); ok && len(ps) == 1 {
				return ps[0], r, true
			}
		}
		return "", "", false
	}
	if task.Type == ahoy.NODE_LAMBDA {
		count := 0
		for _, ch := range task.Value {
			if ch >= '0' && ch <= '9' {
				count = count*10 + int(ch-'0')
			}
		}
		if count < 1 || count > len(task.Children) {
			return "", "", false
		}
		argType := task.Children[0].DataType
		body := task.Children[count]
		retType := gen.inferType(body)
		if argType == "" {
			return "", "", false
		}
		return argType, retType, true
	}
	return "", "", false
}

// emitTaskFn writes the AhoyFn a submit call should run and returns the task's
// argument type (for marshalling the argument). A task whose argument or result
// is a double gets a small marshalling thunk: the runtime passes everything as
// intptr_t, so doubles travel as their bit pattern.
func (gen *CodeGenerator) emitTaskFn(task *ahoy.ASTNode) string {
	argType, retType, known := gen.taskSignature(task)
	if known && (isFloatAhoyType(argType) || isFloatAhoyType(retType)) {
		gen.emitTaskMarshalThunk(task, argType, retType)
		return argType
	}
	if task.Type == ahoy.NODE_UNARY_OP && task.Value == "&" && len(task.Children) == 1 &&
		task.Children[0].Type == ahoy.NODE_IDENTIFIER && gen.userFunctions[task.Children[0].Value] {
		gen.emitFunctionReference(task.Children[0].Value)
	} else {
		gen.generateNode(task)
	}
	return argType
}

// emitTaskMarshalThunk writes a thunk that bit-casts a double argument and/or
// result between the task's real signature and the runtime's intptr_t ABI, then
// emits the AhoyFn value that carries the task in a single-use environment.
func (gen *CodeGenerator) emitTaskMarshalThunk(task *ahoy.ASTNode, argType, retType string) {
	id := gen.varCounter
	gen.varCounter++
	envName := fmt.Sprintf("__ahoy_task_env_%d", id)
	thunk := fmt.Sprintf("__ahoy_task_%d", id)

	argC := gen.mapType(argType)
	retC := gen.mapType(retType)

	gen.closureDecls.WriteString("typedef struct { AhoyFn fn; } ")
	gen.closureDecls.WriteString(envName)
	gen.closureDecls.WriteString(";\n")
	gen.closureDecls.WriteString("static intptr_t ")
	gen.closureDecls.WriteString(thunk)
	gen.closureDecls.WriteString("(void* env, intptr_t arg) {\n")
	gen.closureDecls.WriteString("    ")
	gen.closureDecls.WriteString(envName)
	gen.closureDecls.WriteString("* e = (")
	gen.closureDecls.WriteString(envName)
	gen.closureDecls.WriteString("*)env;\n")
	gen.closureDecls.WriteString("    AhoyFn fn = e->fn;\n    free(e);\n")
	argExpr := "arg"
	if isFloatAhoyType(argType) {
		gen.closureDecls.WriteString("    double a0; memcpy(&a0, &arg, sizeof(double));\n")
		argExpr = "a0"
	}
	call := fmt.Sprintf("((%s(*)(void*, %s))fn.fn)(fn.env, %s)", retC, argC, argExpr)
	switch {
	case isFloatAhoyType(retType):
		gen.closureDecls.WriteString("    double r0 = " + call + ";\n")
		gen.closureDecls.WriteString("    intptr_t out; memcpy(&out, &r0, sizeof(double)); return out;\n")
	case retType == "void" || retType == "":
		gen.closureDecls.WriteString("    " + call + ";\n    return 0;\n")
	default:
		gen.closureDecls.WriteString("    return (intptr_t)(")
		gen.closureDecls.WriteString(call)
		gen.closureDecls.WriteString(");\n")
	}
	gen.closureDecls.WriteString("}\n")

	gen.output.WriteString("({ ")
	gen.output.WriteString(envName)
	gen.output.WriteString("* __e = (")
	gen.output.WriteString(envName)
	gen.output.WriteString("*)malloc(sizeof(")
	gen.output.WriteString(envName)
	gen.output.WriteString(")); __e->fn = ")
	if task.Type == ahoy.NODE_UNARY_OP && task.Value == "&" && len(task.Children) == 1 &&
		task.Children[0].Type == ahoy.NODE_IDENTIFIER && gen.userFunctions[task.Children[0].Value] {
		gen.emitFunctionReference(task.Children[0].Value)
	} else {
		gen.generateNode(task)
	}
	gen.output.WriteString("; (AhoyFn){ (void(*)(void))")
	gen.output.WriteString(thunk)
	gen.output.WriteString(", __e }; })")
}

// emitTaskArg writes a task argument, packing a double into the intptr_t slot
// the runtime reserves for it.
func (gen *CodeGenerator) emitTaskArg(arg *ahoy.ASTNode, argType string) {
	if isFloatAhoyType(argType) {
		gen.output.WriteString("({ double __a = (double)(")
		gen.generateNode(arg)
		gen.output.WriteString("); intptr_t __o; memcpy(&__o, &__a, sizeof(double)); __o; })")
		return
	}
	gen.generateNode(arg)
}

// jobResultExpr renders a read of a job's result, casting the runtime's
// intptr_t back to the Ahoy type. A double result travels as its bit pattern,
// so it is memcpy'd back rather than cast.
func (gen *CodeGenerator) jobResultExpr(callExpr, jobName string) string {
	ret := gen.jobResultTypes[jobName]
	if ret == "float" || ret == "double" {
		return fmt.Sprintf("({ intptr_t __jr = %s; double __jd; memcpy(&__jd, &__jr, sizeof(double)); __jd; })", callExpr)
	}
	if ret != "" {
		return fmt.Sprintf("((%s)%s)", gen.mapType(ret), callExpr)
	}
	return callExpr
}

// emitJobHandler writes the handler and context arguments of a drain call:
// ahoy_jobs_drain(pool, handler, context). The handler takes (job, context).
func (gen *CodeGenerator) emitJobHandler(args *ahoy.ASTNode, prefix string) {
	gen.output.WriteString(prefix)
	if len(args.Children) > 0 {
		task := args.Children[0]
		if task.Type == ahoy.NODE_UNARY_OP && task.Value == "&" && len(task.Children) == 1 &&
			task.Children[0].Type == ahoy.NODE_IDENTIFIER && gen.userFunctions[task.Children[0].Value] {
			name := task.Children[0].Value
			if params := gen.functionParamTypes[name]; len(params) > 0 && len(params) != 2 {
				fmt.Fprintf(os.Stderr, "error: drain handler '%s' must take (job, context), got %d parameter(s)\n", name, len(params))
				gen.hasError = true
			}
			gen.emitFunctionReference(name)
		} else {
			gen.generateNode(task)
		}
	} else {
		gen.output.WriteString("(AhoyFn){0, 0}")
	}
	gen.output.WriteString(", (intptr_t)(")
	if len(args.Children) > 1 {
		gen.generateNode(args.Children[1])
	} else {
		gen.output.WriteString("0")
	}
	gen.output.WriteString("))")
}

// getJobRuntime returns the worker-pool implementation. It is emitted only when
// a program creates a pool; the prototypes live in the generated preamble.
func getJobRuntime() string {
	return `#include <pthread.h>

struct AhoyJob {
    AhoyFn fn;
    intptr_t arg;
    intptr_t result;
    int done;
    int in_completed;            /* linked into the pool's completion queue */
    struct AhoyPool* pool;       /* back pointer, for unlinking */
    pthread_mutex_t mutex;
    pthread_cond_t cond;
    struct AhoyJob* next;        /* task queue */
    struct AhoyJob* next_completed;
    struct AhoyJob* next_all;    /* registry, so destroy() frees leftovers */
};

struct AhoyPool {
    pthread_t* threads;
    int thread_count;
    struct AhoyJob* head;
    struct AhoyJob* tail;
    struct AhoyJob* completed;   /* finished jobs nobody waited for */
    struct AhoyJob* completed_tail;
    struct AhoyJob* all;         /* every job this pool has handed out */
    struct AhoyJob* all_tail;
    int stop;
    int stopped;
    pthread_mutex_t mutex;
    pthread_cond_t work;
};

/* Remove a job from the completion queue. The caller holds pool->mutex. */
static void ahoy_jobs_unlink_completed(struct AhoyPool* pool, struct AhoyJob* job) {
    if (pool == NULL || job == NULL || !job->in_completed) return;
    struct AhoyJob* prev = NULL;
    struct AhoyJob* cur = pool->completed;
    while (cur != NULL && cur != job) {
        prev = cur;
        cur = cur->next_completed;
    }
    if (cur == NULL) { job->in_completed = 0; return; }
    if (prev == NULL) pool->completed = job->next_completed; else prev->next_completed = job->next_completed;
    if (pool->completed_tail == job) pool->completed_tail = prev;
    job->next_completed = NULL;
    job->in_completed = 0;
}

/* Remove a job from the pool's registry. The caller holds pool->mutex. */
static void ahoy_jobs_unlink_all(struct AhoyPool* pool, struct AhoyJob* job) {
    if (pool == NULL || job == NULL) return;
    struct AhoyJob* prev = NULL;
    struct AhoyJob* cur = pool->all;
    while (cur != NULL && cur != job) {
        prev = cur;
        cur = cur->next_all;
    }
    if (cur == NULL) return;
    if (prev == NULL) pool->all = job->next_all; else prev->next_all = job->next_all;
    if (pool->all_tail == job) pool->all_tail = prev;
    job->next_all = NULL;
}

static void ahoy_jobs_free(struct AhoyPool* pool, struct AhoyJob* job) {
    if (job == NULL) return;
    if (pool != NULL) {
        pthread_mutex_lock(&pool->mutex);
        ahoy_jobs_unlink_completed(pool, job);
        ahoy_jobs_unlink_all(pool, job);
        pthread_mutex_unlock(&pool->mutex);
    }
    pthread_mutex_destroy(&job->mutex);
    pthread_cond_destroy(&job->cond);
    free(job);
}

static void* ahoy_job_worker(void* data) {
    struct AhoyPool* pool = (struct AhoyPool*)data;
    for (;;) {
        pthread_mutex_lock(&pool->mutex);
        while (pool->head == NULL && !pool->stop) {
            pthread_cond_wait(&pool->work, &pool->mutex);
        }
        if (pool->head == NULL && pool->stop) {
            pthread_mutex_unlock(&pool->mutex);
            return NULL;
        }
        struct AhoyJob* job = pool->head;
        pool->head = job->next;
        if (pool->head == NULL) pool->tail = NULL;
        pthread_mutex_unlock(&pool->mutex);

        job->result = ((intptr_t(*)(void*, intptr_t))job->fn.fn)(job->fn.env, job->arg);

        /* queue it for drain() before signalling done, so a waiter that sees
           done is guaranteed the queue entry already exists (or was skipped) */
        pthread_mutex_lock(&pool->mutex);
        job->in_completed = 1;
        job->next_completed = NULL;
        if (pool->completed_tail) pool->completed_tail->next_completed = job; else pool->completed = job;
        pool->completed_tail = job;
        pthread_mutex_unlock(&pool->mutex);

        pthread_mutex_lock(&job->mutex);
        job->done = 1;
        pthread_cond_broadcast(&job->cond);
        pthread_mutex_unlock(&job->mutex);
    }
}

AhoyPool* ahoy_jobs_new(int count) {
    if (count < 1) count = 1;
    AhoyPool* pool = (AhoyPool*)calloc(1, sizeof(AhoyPool));
    pool->thread_count = count;
    pool->threads = (pthread_t*)calloc(count, sizeof(pthread_t));
    pthread_mutex_init(&pool->mutex, NULL);
    pthread_cond_init(&pool->work, NULL);
    for (int i = 0; i < count; i++) {
        pthread_create(&pool->threads[i], NULL, ahoy_job_worker, pool);
    }
    return pool;
}

AhoyJob* ahoy_jobs_submit(AhoyPool* pool, AhoyFn fn, intptr_t arg) {
    AhoyJob* job = (AhoyJob*)calloc(1, sizeof(AhoyJob));
    job->fn = fn;
    job->arg = arg;
    job->pool = pool;
    pthread_mutex_init(&job->mutex, NULL);
    pthread_cond_init(&job->cond, NULL);
    pthread_mutex_lock(&pool->mutex);
    if (pool->tail) pool->tail->next = job; else pool->head = job;
    pool->tail = job;
    if (pool->all_tail) pool->all_tail->next_all = job; else pool->all = job;
    pool->all_tail = job;
    pthread_cond_signal(&pool->work);
    pthread_mutex_unlock(&pool->mutex);
    return job;
}

/* Blocks for the result and consumes the job: do not use the handle after
   wait. Jobs are owned by the pool until waited for or drained. */
intptr_t ahoy_jobs_wait(AhoyJob* job) {
    if (job == NULL) return 0;
    struct AhoyPool* pool = job->pool;
    pthread_mutex_lock(&job->mutex);
    while (!job->done) pthread_cond_wait(&job->cond, &job->mutex);
    pthread_mutex_unlock(&job->mutex);
    intptr_t result = job->result;
    ahoy_jobs_free(pool, job);
    return result;
}

/* Non-blocking read; the handle stays valid until waited for or drained. */
intptr_t ahoy_jobs_result(AhoyJob* job) {
    if (job == NULL || !job->done) return 0;
    return job->result;
}

int ahoy_jobs_done(AhoyJob* job) {
    return (job != NULL && job->done) ? 1 : 0;
}

/* Hand every finished job that nobody waited for to handler(job, context),
   then consume those jobs. Returns how many were handed out. */
int ahoy_jobs_drain(AhoyPool* pool, AhoyFn handler, intptr_t context) {
    if (pool == NULL) return 0;
    pthread_mutex_lock(&pool->mutex);
    struct AhoyJob* list = pool->completed;
    pool->completed = NULL;
    pool->completed_tail = NULL;
    for (struct AhoyJob* j = list; j != NULL; j = j->next_completed) {
        /* A job enters the completed queue only after its result is written,
           but the worker publishes done afterwards. drain() promises finished
           jobs, and j.result|| refuses a job whose done is still 0, so publish
           it here: without this a drained job's result read back as 0. */
        pthread_mutex_lock(&j->mutex);
        j->done = 1;
        pthread_mutex_unlock(&j->mutex);
        j->in_completed = 0;
    }
    pthread_mutex_unlock(&pool->mutex);

    int count = 0;
    struct AhoyJob* job = list;
    while (job != NULL) {
        struct AhoyJob* next = job->next_completed;
        job->next_completed = NULL;
        ((void(*)(void*, intptr_t, intptr_t))handler.fn)(handler.env, (intptr_t)job, context);
        ahoy_jobs_free(pool, job);
        job = next;
        count++;
    }
    return count;
}

void ahoy_jobs_stop(AhoyPool* pool) {
    if (pool == NULL) return;
    pthread_mutex_lock(&pool->mutex);
    if (pool->stopped) {   /* stop is idempotent: explicit stop + scope exit */
        pthread_mutex_unlock(&pool->mutex);
        return;
    }
    pool->stopped = 1;
    pool->stop = 1;
    pthread_cond_broadcast(&pool->work);
    pthread_mutex_unlock(&pool->mutex);
    for (int i = 0; i < pool->thread_count; i++) pthread_join(pool->threads[i], NULL);
    free(pool->threads);
    pool->threads = NULL;
}

void ahoy_jobs_destroy(AhoyPool* pool) {
    if (pool == NULL) return;
    ahoy_jobs_stop(pool);
    /* every job the pool handed out is owned by it until consumed */
    struct AhoyJob* job = pool->all;
    while (job != NULL) {
        struct AhoyJob* next = job->next_all;
        pthread_mutex_destroy(&job->mutex);
        pthread_cond_destroy(&job->cond);
        free(job);
        job = next;
    }
    pool->all = NULL;
    pool->all_tail = NULL;
    pthread_mutex_destroy(&pool->mutex);
    pthread_cond_destroy(&pool->work);
    free(pool);
}
`
}
