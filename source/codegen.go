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
	Type         string
	DefaultValue string // C code for default value (if any)
	IsStatic     bool   // Static field (shared across instances)
	IsConst      bool   // Const field (SCREAMING_SNAKE_CASE - immutable)
	IsWeak       bool   // Weak reference (for ARC cycle breaking)
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
	orderedIncludes               []string                     // Keep track of include order
	variables                     map[string]string            // variable name -> type (global scope)
	functionVars                  map[string]string            // variable name -> type (function scope)
	nestedScopeVars               map[string]bool              // variables declared in nested scopes (loops/ifs)
	varDeclIndent                 map[string]int               // variable name -> indent level where it was declared
	constants                     map[string]string            // constant name -> type
	declaredConstants             map[string]bool              // constant name -> declared (for duplicate check)
	enums                         map[string]map[string]bool   // enum name -> {member names}
	enumOriginalNames             map[string]bool              // tracks original (lowercase) enum names vs aliases
	enumMemberTypes               map[string]string            // "enumName.memberName" -> type
	enumTypes                     map[string]string            // enum name -> enum type (int, string, etc.)
	userFunctions                 map[string]bool              // user-defined function names (keep snake_case)
	calledFunctions               map[string]bool              // functions that are actually called in the program
	hasError                      bool                         // Track if error occurred
	arrayImpls                    bool                         // Track if we've added array implementation
	arrayMethods                  map[string]bool              // Track which array methods are used
	stringMethods                 map[string]bool              // Track which string methods are used
	dictMethods                   map[string]bool              // Track which dict methods are used
	useJSON                       bool                         // Track if JSON functions are used
	jsonVariables                 map[string]bool              // Track which variables hold JSON data
	jsonStructs                   map[string]bool              // Track which structs are JSON schemas (not real C structs)
	loopCounters                  []string                     // Stack of loop counter variable names
	currentFunction               string                       // Current function being generated
	currentFunctionReturnType     string                       // Return type of current function
	currentFunctionHasMultiReturn bool                         // Whether current function has multiple returns
	hasMainFunc                   bool                         // Whether there's an Ahoy main function
	arrayElementTypes             map[string]string            // array variable name -> element type
	array2DElementTypes           map[string]string            // 2D array variable name -> nested element type (element type of inner arrays)
	structs                       map[string]*StructInfo       // struct name -> struct info
	currentTypeContext            string                       // Current type annotation context (e.g., "array[int]")
	functionReturnTypes           map[string][]string          // function name -> return types (for inferred functions)
	deferredStatements            []string                     // Stack of deferred statements for current function
	functionParamTypes            map[string][]string          // function name -> parameter types
	functionParamNames            map[string][]string          // function name -> parameter names
	functionParamDefaults         map[string][]*ahoy.ASTNode   // function name -> parameter default values
	dictSourcedVars               map[string]string            // variable name -> dict name (for dict-accessed vars)
	dictSourcedKeys               map[string]string            // variable name -> key (for dict-accessed vars)
	generatedTypes                map[string]bool              // Track which types have been generated
	cFunctionNames                map[string]string            // snake_case name -> actual C name
	cNamespaces                   map[string]map[string]string // namespace -> (snake_case name -> actual C name)
	cFunctionReturnTypes          map[string]string            // C function name (snake_case) -> return type
	cNamespaceReturnTypes         map[string]map[string]string // namespace -> (snake_case name -> return type)
	cTypeDefinitions              map[string]bool              // Track known C types from headers
	declaredGlobalVars            map[string]bool              // Track global variables that have been declared in C code
	declaredFunctionVars          map[string]bool              // Track function-local variables that have been declared in C code
	enableBoundsChecking          bool                         // Enable runtime array bounds checking
	enableSignalHandler           bool                         // Enable signal handler for crash reporting
	skipBoundsCheck               bool                         // Temporarily skip bounds check (for lvalue contexts)
	sourceFilename                string                       // Source filename for error messages
	heapAllocatedVars             map[string]bool              // Track heap-allocated variables in current function
	heapVarScopes                 map[string]int               // Track scope depth where each heap var was allocated
	heapVarTypes                  map[string]string            // Track type of each heap-allocated variable for proper freeing
	scopeDepth                    int                          // Current scope depth (incremented for loops/ifs/blocks)
	scopeAllocations              map[int][]string             // scope depth -> list of variables allocated at that scope
	loopScopeStack                []int                        // Stack of scope depths where loops begin (for break/continue cleanup)
	escapingVars                  map[string]bool              // Track variables that escape the function (returned or stored)
	manuallyFreedVars             map[string]bool              // Track variables with manual defer free
	autoFreedVars                 map[string]bool              // Track variables with automatic defer free
	functionParameters            map[string]bool              // Track function parameters (never auto-free)
	cStructFields                 map[string]map[string]string // C struct name -> (field name -> field type)
	cTypedefs                     map[string]string            // typedef alias -> base type
	cFunctionParamTypes           map[string][]string          // C function name (snake_case) -> parameter types
	parsedCHeaders                map[string]*ahoy.CHeaderInfo // In-memory cache for parsed C headers (path -> info)
	parsedCHeadersMu              sync.Mutex                   // Mutex for parsedCHeaders
	enableARC                     bool                         // Enable Automatic Reference Counting
	arcStructs                    map[string]bool              // Structs that use ARC (have reference counting)
	weakFields                    map[string]map[string]bool   // struct name -> (field name -> isWeak)
	parentChildRelations          map[string]map[string]bool   // parent struct -> (child field names)
	refCountedVars                map[string]bool              // Track variables that need retain/release
}

// GenerateC generates C code from an AST (exported for testing)
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
		array2DElementTypes:   make(map[string]string),
		structs:               make(map[string]*StructInfo),
		functionReturnTypes:   make(map[string][]string),
		functionParamTypes:    make(map[string][]string),
		functionParamNames:    make(map[string][]string),
		functionParamDefaults: make(map[string][]*ahoy.ASTNode),
		dictSourcedVars:       make(map[string]string),
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
		manuallyFreedVars:     make(map[string]bool),
		autoFreedVars:         make(map[string]bool),
		loopScopeStack:        make([]int, 0),
		cStructFields:         make(map[string]map[string]string),
		cTypedefs:             make(map[string]string),
		cFunctionParamTypes:   make(map[string][]string),
		parsedCHeaders:        make(map[string]*ahoy.CHeaderInfo),
		enableARC:             true, // Enable ARC by default
		arcStructs:            make(map[string]bool),
		weakFields:            make(map[string]map[string]bool),
		parentChildRelations:  make(map[string]map[string]bool),
		refCountedVars:        make(map[string]bool),
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

	// Third pass: check if there's a main function and collect function signatures
	gen.checkForMainFunction(ast)

	// Fourth pass: scan variable declarations to populate type information
	gen.scanVariableTypes(ast)

	// Fourth pass: infer parameter types from function call sites
	gen.inferParameterTypesFromCalls(ast)

	// Fifth pass: infer return types for all functions with infer keyword
	gen.inferAllFunctionReturnTypes(ast)

	// Sixth pass: scan for method calls to determine which helper functions we need
	gen.scanForMethodCalls(ast)

	// Sixth-and-a-half pass: scan for function calls to track which functions are actually used
	gen.scanForFunctionCalls(ast)

	// Seventh pass: Generate all type declarations (enums, structs) first
	gen.generateTypeDeclarations(ast)

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

	// Generate ARC helper functions if ARC is enabled
	gen.writeARCHelperFunctions()

	// Generate vector2 and color constructors
	gen.writeTypeConstructors()

	// Generate JSON helper functions if JSON is used
	gen.writeJSONHelperFunctions()

	// Build final output
	var result strings.Builder

	// Write includes
	for _, include := range gen.orderedIncludes {
		// Use angle brackets for system includes, quotes for local .h files
		if strings.HasSuffix(include, ".h") && (strings.HasPrefix(include, "/") || strings.HasPrefix(include, ".")) {
			result.WriteString(fmt.Sprintf("#include \"%s\"\n", include))
		} else {
			result.WriteString(fmt.Sprintf("#include <%s>\n", include))
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
		result.WriteString("\n")
	}

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

	// Write global constants before functions
	if gen.constantDecls.Len() > 0 {
		result.WriteString("// Global constants\n")
		result.WriteString(gen.constantDecls.String())
		result.WriteString("\n")
	}

	// Write function implementations
	result.WriteString(gen.funcDecls.String())
	result.WriteString("\n")

	// Write main program
	if gen.hasMainFunc {
		// If there's an Ahoy main function, just call it
		result.WriteString("int main() {\n")
		if gen.enableSignalHandler {
			result.WriteString("    ahoy_setup_signal_handlers();\n")
		}
		result.WriteString("    ahoy_main();\n")
		result.WriteString("    return 0;\n")
		result.WriteString("}\n")
	} else {
		// Legacy: no main function, use global scope code
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
	if node == nil {
		return
	}

	// Scan for variable declarations and track their types
	if node.Type == ahoy.NODE_VARIABLE_DECLARATION || node.Type == ahoy.NODE_ASSIGNMENT {
		varName := node.Value
		if len(node.Children) > 0 {
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
		gen.scanVariableTypes(child)
	}
}

// inferParameterTypesFromCalls analyzes function calls to infer parameter types
func (gen *CodeGenerator) inferParameterTypesFromCalls(node *ahoy.ASTNode) {
	if node == nil {
		return
	}

	// First collect all function definitions and their parameter names
	functionParams := make(map[string][]string) // function name -> parameter names
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
			}
		}
		for _, child := range n.Children {
			collectFuncs(child)
		}
	}
	collectFuncs(node)

	// Now scan for function calls and infer parameter types from arguments
	paramTypeInferences := make(map[string]map[int]string) // function name -> param index -> inferred type
	var analyzeCalls func(*ahoy.ASTNode)
	analyzeCalls = func(n *ahoy.ASTNode) {
		if n == nil {
			return
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
						argType := gen.inferType(arg)

						// If we already have an inferred type for this parameter, check consistency
						if existingType, hasType := paramTypeInferences[funcName][i]; hasType {
							// If types differ and one is more specific (not generic/any), prefer the specific one
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
			analyzeCalls(child)
		}
	}
	analyzeCalls(node)

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
		// Just register that this struct exists - actual generation happens later
		if gen.structs[structName] == nil {
			gen.structs[structName] = &StructInfo{Name: structName, Fields: make([]StructField, 0)}
			gen.structs[capitalizeFirst(structName)] = gen.structs[structName]
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
	}

	// Recursively scan children
	for _, child := range node.Children {
		gen.scanTypeDeclarations(child)
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

func (gen *CodeGenerator) isHeapAllocatedType(varType string) bool {
	// Check if a type requires heap allocation
	if strings.HasPrefix(varType, "array") || varType == "AhoyArray*" {
		return true
	}
	if strings.HasPrefix(varType, "dict") || varType == "HashMap*" {
		return true
	}
	// String methods that return new strings are heap-allocated
	// JSON objects are heap-allocated
	if varType == "AhoyJSON*" {
		return true
	}
	return false
}

// hasCircularDependency checks if structA has a field of type structB
// and structB has a field of type structA (circular dependency)
func (gen *CodeGenerator) hasCircularDependency(parentStruct, childStruct string) bool {
	// Check if childStruct has any fields that reference parentStruct
	childInfo, exists := gen.structs[childStruct]
	if !exists {
		return false
	}

	for _, field := range childInfo.Fields {
		// Direct reference back to parent
		if field.Type == parentStruct || field.Type == capitalizeFirst(parentStruct) {
			return true
		}
		// Pointer reference back to parent
		if field.Type == parentStruct+"*" || field.Type == capitalizeFirst(parentStruct)+"*" {
			return true
		}
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

	switch node.Type {
	case ahoy.NODE_PROGRAM:
		for _, child := range node.Children {
			gen.generateNodeInternal(child, true)
		}

	case ahoy.NODE_FUNCTION:
		gen.generateFunction(node)

	case ahoy.NODE_ASSIGNMENT, ahoy.NODE_VARIABLE_DECLARATION:
		gen.generateAssignment(node)

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

	case ahoy.NODE_BINARY_OP:
		gen.generateBinaryOp(node)

	case ahoy.NODE_UNARY_OP:
		gen.generateUnaryOp(node)

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
		} else {
			// Check if this identifier might be an enum member (for switch cases)
			// Try to resolve it to a fully qualified enum member name
			if resolvedName := gen.tryResolveEnumMember(node.Value); resolvedName != "" {
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
			gen.output.WriteString(fmt.Sprintf("\"%s\"", node.Value))
		}

	case ahoy.NODE_RAW_STRING:
		// Raw strings preserve newlines and don't interpret escape sequences
		// In C, we need to escape the actual newlines and special chars for the string literal
		escaped := escapeRawString(node.Value)
		gen.output.WriteString(fmt.Sprintf("\"%s\"", escaped))

	case ahoy.NODE_F_STRING:
		gen.generateFString(node)

	case ahoy.NODE_CHAR:
		gen.output.WriteString(fmt.Sprintf("'%s'", node.Value))

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
		// Generate cleanup for variables in nested scopes before breaking
		// Note: generateEarlyExitCleanup returns code WITH indentation/newlines
		cleanup := gen.generateEarlyExitCleanup(false)
		if cleanup != "" {
			gen.output.WriteString(cleanup)
		}
		gen.writeIndent()
		gen.output.WriteString("break;\n")
	case ahoy.NODE_NEXT:
		// Generate cleanup for variables in nested scopes before continuing
		// Note: generateEarlyExitCleanup returns code WITH indentation/newlines
		cleanup := gen.generateEarlyExitCleanup(false)
		if cleanup != "" {
			gen.output.WriteString(cleanup)
		}
		gen.writeIndent()
		gen.output.WriteString("continue;\n")
	case ahoy.NODE_GOTO_STATEMENT:
		gen.generateGotoStatement(node)
	case ahoy.NODE_LABEL_DECLARATION:
		gen.generateLabelDeclaration(node)
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
				gen.funcReturnStructs.WriteString(fmt.Sprintf("typedef struct {\n"))
				for i, rType := range returnTypes {
					// Use intptr_t for generic/any types (will be cast at call site)
					var mappedType string
					if rType == "generic" || rType == "any" {
						mappedType = "intptr_t"
					} else {
						mappedType = gen.mapType(rType)
					}
					gen.funcReturnStructs.WriteString(fmt.Sprintf("    %s ret%d;\n", mappedType, i))
				}
				gen.funcReturnStructs.WriteString(fmt.Sprintf("} %s;\n\n", structName))
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
				gen.funcReturnStructs.WriteString(fmt.Sprintf("typedef struct {\n"))
				for i, rType := range returnTypes {
					mappedType := gen.mapType(rType)
					gen.funcReturnStructs.WriteString(fmt.Sprintf("    %s ret%d;\n", mappedType, i))
				}
				gen.funcReturnStructs.WriteString(fmt.Sprintf("} %s;\n\n", structName))
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
	gen.funcForwardDecls.WriteString(fmt.Sprintf("%s %s(%s);\n", returnType, cFuncName, paramList))
	// Write function implementation
	gen.funcDecls.WriteString(fmt.Sprintf("%s %s(%s) {\n", returnType, cFuncName, paramList))

	// Function body
	body := node.Children[1]
	oldOutput := gen.output
	gen.output = strings.Builder{}
	gen.indent++

	// Store current function info for return statement generation
	gen.currentFunction = cFuncName
	gen.currentFunctionReturnType = returnType
	gen.currentFunctionHasMultiReturn = len(returnTypes) > 1

	// Initialize function-local variable scope and add parameters
	gen.functionVars = make(map[string]string)
	gen.dictSourcedVars = make(map[string]string)
	gen.dictSourcedKeys = make(map[string]string)
	gen.nestedScopeVars = make(map[string]bool)
	gen.varDeclIndent = make(map[string]int)

	// Clear function-local declared variables for this new function
	gen.declaredFunctionVars = make(map[string]bool)

	for _, param := range params.Children {
		if param.DataType != "" {
			gen.functionVars[param.Value] = param.DataType

			// Track array element types for typed array parameters
			if strings.HasPrefix(param.DataType, "array[") {
				elemType := strings.TrimSuffix(strings.TrimPrefix(param.DataType, "array["), "]")
				gen.arrayElementTypes[param.Value] = elemType
			}
		} else {
			// Parameters without explicit type are any (generic)
			gen.functionVars[param.Value] = "any"
		}
	}

	// Initialize deferred statements stack for this function
	gen.deferredStatements = []string{}
	gen.heapAllocatedVars = make(map[string]bool)
	gen.heapVarScopes = make(map[string]int)
	gen.heapVarTypes = make(map[string]string)
	gen.scopeDepth = 0
	gen.scopeAllocations = make(map[int][]string)
	gen.escapingVars = make(map[string]bool)
	gen.manuallyFreedVars = make(map[string]bool)
	gen.autoFreedVars = make(map[string]bool)
	gen.functionParameters = make(map[string]bool)

	// Mark function parameters so we never auto-free them
	for _, param := range params.Children {
		gen.functionParameters[param.Value] = true
	}

	gen.generateNodeInternal(body, false)

	// Add automatic defer free for heap-allocated variables that don't escape
	gen.addAutomaticDeferFrees()

	// Execute deferred statements in LIFO order before function end
	if len(gen.deferredStatements) > 0 {
		for i := len(gen.deferredStatements) - 1; i >= 0; i-- {
			gen.output.WriteString(gen.deferredStatements[i])
		}
	}

	gen.funcDecls.WriteString(gen.output.String())
	gen.funcDecls.WriteString("}\n\n")

	gen.indent--
	gen.output = oldOutput
	gen.currentFunction = ""
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
					gen.output.WriteString(fmt.Sprintf("AhoyArray* __arr = (AhoyArray*)%s; ", arrayName))
				} else {
					gen.output.WriteString(fmt.Sprintf("AhoyArray* __arr = %s; ", arrayName))
				}

				gen.output.WriteString("if (__idx < 0 || __idx >= __arr->length) { ")
				gen.output.WriteString("fprintf(stderr, \"RUNTIME ERROR: Array bounds violation\\n\"); ")
				gen.output.WriteString(fmt.Sprintf("fprintf(stderr, \"  File: %s\\n\"); ", gen.sourceFilename))
				gen.output.WriteString(fmt.Sprintf("fprintf(stderr, \"  Line: %d\\n\"); ", node.Children[0].Line))
				gen.output.WriteString(fmt.Sprintf("fprintf(stderr, \"  Array: %s\\n\"); ", arrayName))
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
						gen.output.WriteString(fmt.Sprintf("*(%s*)__arr->data[__idx] = ", cType))
					} else {
						if needsArrayCast {
							gen.output.WriteString(fmt.Sprintf("*(%s*)((AhoyArray*)%s)->data[", cType, arrayName))
						} else {
							gen.output.WriteString(fmt.Sprintf("*(%s*)%s->data[", cType, arrayName))
						}
						gen.generateNode(indexNode)
						gen.output.WriteString("] = ")
					}
					gen.generateNode(valueNode)
					gen.output.WriteString(";")
				} else if cType == "double" || cType == "float" {
					// For float/double, store via intptr_t cast
					if gen.enableBoundsChecking {
						gen.output.WriteString("__arr->data[__idx] = (void*)(intptr_t)(double)")
					} else {
						if needsArrayCast {
							gen.output.WriteString(fmt.Sprintf("((AhoyArray*)%s)->data[", arrayName))
						} else {
							gen.output.WriteString(fmt.Sprintf("%s->data[", arrayName))
						}
						gen.generateNode(indexNode)
						gen.output.WriteString("] = (void*)(intptr_t)(double)")
					}
					gen.generateNode(valueNode)
					gen.output.WriteString(";")
				} else if cType == "char*" {
					// For strings, store as pointer
					if gen.enableBoundsChecking {
						gen.output.WriteString("__arr->data[__idx] = (void*)")
					} else {
						if needsArrayCast {
							gen.output.WriteString(fmt.Sprintf("((AhoyArray*)%s)->data[", arrayName))
						} else {
							gen.output.WriteString(fmt.Sprintf("%s->data[", arrayName))
						}
						gen.generateNode(indexNode)
						gen.output.WriteString("] = (void*)")
					}
					gen.generateNode(valueNode)
					gen.output.WriteString(";")
				} else {
					// For int and other intptr_t-compatible types, direct assignment
					if gen.enableBoundsChecking {
						gen.output.WriteString("__arr->data[__idx] = (void*)(intptr_t)")
					} else {
						if needsArrayCast {
							gen.output.WriteString(fmt.Sprintf("((AhoyArray*)%s)->data[", arrayName))
						} else {
							gen.output.WriteString(fmt.Sprintf("%s->data[", arrayName))
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
					gen.output.WriteString("__arr->data[__idx] = (void*)(intptr_t)")
				} else {
					if needsArrayCast {
						gen.output.WriteString(fmt.Sprintf("((AhoyArray*)%s)->data[", arrayName))
					} else {
						gen.output.WriteString(fmt.Sprintf("%s->data[", arrayName))
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

		// Special handling for dict assignment - use hashMapPut
		if node.Children[0].Type == ahoy.NODE_DICT_ACCESS {
			dictName := node.Children[0].Value
			keyNode := node.Children[0].Children[0]
			valueNode := node.Children[1]

			gen.output.WriteString(fmt.Sprintf("hashMapPut(%s, ", dictName))
			gen.generateNode(keyNode)
			gen.output.WriteString(", (void*)(intptr_t)")
			gen.generateNode(valueNode)
			gen.output.WriteString(");\n")
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

			// If object is dict, HashMap*, generic, any, or intptr_t, use hashMapPut
			if objectType == "dict" || objectType == "HashMap*" || objectType == "generic" || objectType == "any" || objectType == "intptr_t" ||
				strings.HasPrefix(objectType, "dict[") || strings.HasPrefix(objectType, "dict<") {
				gen.output.WriteString("hashMapPut(")
				// Cast generic/any/intptr_t to HashMap*
				if objectType == "generic" || objectType == "any" || objectType == "intptr_t" {
					gen.output.WriteString("(HashMap*)")
				}
				gen.output.WriteString(objectName)
				gen.output.WriteString(fmt.Sprintf(", \"%s\", (void*)(intptr_t)", propertyName))
				gen.generateNode(node.Children[1])
				gen.output.WriteString(");\n")
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
					gen.output.WriteString(fmt.Sprintf("((%s*)(intptr_t)((AhoyArray*)", cType))
					gen.generateNode(innerAccess)
					gen.output.WriteString(")->data[")
					gen.generateNode(outerIndex)
					gen.output.WriteString(fmt.Sprintf("])->%s = ", memberName))
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
						gen.output.WriteString(fmt.Sprintf("AhoyArray* __arr = %s; ", arrayName))
						gen.output.WriteString("if (__idx < 0 || __idx >= __arr->length) { ")
						gen.output.WriteString("fprintf(stderr, \"RUNTIME ERROR: Array bounds violation\\n\"); ")
						gen.output.WriteString(fmt.Sprintf("fprintf(stderr, \"  File: %s\\n\"); ", gen.sourceFilename))
						gen.output.WriteString(fmt.Sprintf("fprintf(stderr, \"  Line: %d\\n\"); ", node.Line))
						gen.output.WriteString(fmt.Sprintf("fprintf(stderr, \"  Array: %s\\n\"); ", arrayName))
						gen.output.WriteString("fprintf(stderr, \"  Index: %d\\n\", __idx); ")
						gen.output.WriteString("fprintf(stderr, \"  Valid range: 0 to %d\\n\", __arr->length - 1); ")
						gen.output.WriteString("exit(1); } ")
						gen.output.WriteString(fmt.Sprintf("((%s*)__arr->data[__idx])->%s = ", cType, memberName))
						gen.generateNode(valueNode)
						gen.output.WriteString("; }\n")
					} else {
						gen.output.WriteString(fmt.Sprintf("((%s*)%s->data[", cType, arrayName))
						gen.generateNode(indexNode)
						gen.output.WriteString(fmt.Sprintf("])->%s = ", memberName))
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
	isLoopLocalPattern := valueNode.Type == ahoy.NODE_ARRAY_ACCESS || valueNode.Type == ahoy.NODE_DICT_ACCESS

	needsRedeclare := false

	// If variable was declared in nested scope and current indent is < declaration indent,
	// we've exited that scope and are in a parent block - need to redeclare
	if isNestedScope && gen.indent < declIndent {
		needsRedeclare = true
	}

	// If we're in a deeply nested block (indent > 2) and the indent differs from
	// where the variable was declared, we're likely in a different block structure
	// and should declare a new block-local variable
	if isDeclared && gen.indent > 2 && gen.indent != declIndent {
		needsRedeclare = true
	}

	canRedeclare := isLoopLocalPattern || needsRedeclare

	if isDeclared && !canRedeclare {
		// Just assignment - mark RHS variables as escaping if being assigned to existing var
		gen.markEscapingVariables(node.Children[0])

		// Check if we're reassigning a heap-allocated variable - need to free old value first
		// Only do this for variables at nested scopes (indent > 1) that won't be auto-freed
		// Function-level variables (indent == 1) are handled by auto-defer
		varName := node.Value
		if gen.currentFunction != "" && gen.indent > 1 && gen.heapAllocatedVars[varName] && !gen.functionParameters[varName] {
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
		} else {
			gen.output.WriteString(fmt.Sprintf("%s = ", node.Value))
			gen.generateNode(node.Children[0])
			gen.output.WriteString(";\n")
		}

		// After reassignment, track the new value if it's also heap-allocated
		if gen.currentFunction != "" {
			valueType := gen.inferType(valueNode)
			if gen.isHeapAllocatedType(valueType) {
				if valueNode.Type == ahoy.NODE_CALL || valueNode.Type != ahoy.NODE_IDENTIFIER {
					// Clear the autoFreed flag since we have a new allocation
					delete(gen.autoFreedVars, varName)
					// Track in current scope
					gen.heapVarScopes[varName] = gen.scopeDepth
					gen.heapVarTypes[varName] = valueType
					// Add to current scope's allocations if in nested scope
					if gen.scopeDepth > 0 {
						gen.scopeAllocations[gen.scopeDepth] = append(gen.scopeAllocations[gen.scopeDepth], varName)
					}
				}
			}
		}
	} else {
		// Type inference and declaration
		valueNode := node.Children[0]

		// Check if we have an explicit type annotation
		explicitType := node.DataType

		// Special handling for object literals - they define their own type inline
		if valueNode.Type == ahoy.NODE_OBJECT_LITERAL {
			// Check if this is a typed struct literal (e.g., Color{...})
			if valueNode.Value != "" {
				// Use mapType to get the correct C type name
				structName := gen.mapType(valueNode.Value)
				gen.output.WriteString(fmt.Sprintf("%s %s = ", structName, node.Value))
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
				gen.funcDecls.WriteString(fmt.Sprintf("typedef struct {\n"))

				// Track struct fields
				structInfo := &StructInfo{
					Name:   anonStructName,
					Fields: make([]StructField, 0),
				}

				for _, prop := range valueNode.Children {
					if prop.Type == ahoy.NODE_OBJECT_PROPERTY {
						propType := gen.inferType(prop.Children[0])
						cType := gen.mapType(propType)
						gen.funcDecls.WriteString(fmt.Sprintf("    %s %s;\n", cType, prop.Value))

						structInfo.Fields = append(structInfo.Fields, StructField{
							Name: prop.Value,
							Type: cType,
						})
					}
				}
				gen.funcDecls.WriteString(fmt.Sprintf("} %s;\n\n", anonStructName))
				gen.structs[anonStructName] = structInfo

				// Generate variable declaration
				gen.output.WriteString(fmt.Sprintf("%s %s = ", anonStructName, node.Value))
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
			if strings.HasPrefix(varType, "array[") && strings.HasSuffix(varType, "]") {
				elemType := strings.TrimSuffix(strings.TrimPrefix(varType, "array["), "]")
				// Only set if not already set by array literal handling above
				if _, exists := gen.arrayElementTypes[node.Value]; !exists {
					gen.arrayElementTypes[node.Value] = elemType

					// For 2D arrays (array[array[type]]), also extract inner element type
					if strings.HasPrefix(elemType, "array[") {
						nestedElemType := strings.TrimSuffix(strings.TrimPrefix(elemType, "array["), "]")
						gen.array2DElementTypes[node.Value] = nestedElemType
					}
				}
			}

			cType := gen.mapType(varType)

			// Check if value is a switch expression
			if valueNode.Type == ahoy.NODE_SWITCH_STATEMENT {
				// Generate switch as expression (assign in each case)
				gen.output.WriteString(fmt.Sprintf("%s %s;\n", cType, node.Value))
				gen.generateSwitchExpression(valueNode, node.Value)
			} else {
				gen.output.WriteString(fmt.Sprintf("%s %s = ", cType, node.Value))
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
					// Function call that returns heap-allocated type - track it
					gen.registerHeapAllocation(node.Value, varType)
				} else if valueNode.Type != ahoy.NODE_IDENTIFIER {
					// Literal (array, dict, object) - track it
					gen.registerHeapAllocation(node.Value, varType)
				}
			}

			// Clear type context after generation
			gen.currentTypeContext = ""
		}
	}
}

func (gen *CodeGenerator) generateIfStatement(node *ahoy.ASTNode) {
	gen.writeIndent()
	gen.output.WriteString("if (")
	gen.generateNode(node.Children[0])
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
			gen.generateNode(node.Children[i])
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

			// Check if it's a list of cases or range
			if caseValue.Type == ahoy.NODE_SWITCH_CASE_LIST {
				// Multiple cases
				for _, val := range caseValue.Children {
					gen.indent++
					gen.writeIndent()
					gen.output.WriteString("case ")
					gen.generateNode(val)
					gen.output.WriteString(":\n")
					gen.indent--
				}
				gen.indent++
				gen.indent++
				gen.generateSwitchCaseAssignment(caseBody, targetVar)
				gen.writeIndent()
				gen.output.WriteString("break;\n")
				gen.indent--
				gen.indent--
			} else if caseValue.Type == ahoy.NODE_SWITCH_CASE_RANGE {
				// Range case
				gen.indent++
				gen.writeIndent()
				gen.output.WriteString("default:\n")
				gen.indent++
				gen.writeIndent()
				gen.output.WriteString("if (")
				gen.generateNode(switchExpr)
				gen.output.WriteString(" >= ")
				gen.generateNode(caseValue.Children[0])
				gen.output.WriteString(" && ")
				gen.generateNode(switchExpr)
				gen.output.WriteString(" <= ")
				gen.generateNode(caseValue.Children[1])
				gen.output.WriteString(") {\n")
				gen.indent++
				gen.generateSwitchCaseAssignment(caseBody, targetVar)
				gen.writeIndent()
				gen.output.WriteString("break;\n")
				gen.indent--
				gen.writeIndent()
				gen.output.WriteString("}\n")
				gen.indent--
				gen.indent--
			} else {
				// Single case or default
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
				gen.generateSwitchCaseAssignment(caseBody, targetVar)
				gen.writeIndent()
				gen.output.WriteString("break;\n")
				gen.indent--
				gen.indent--
			}
		}
	}

	gen.writeIndent()
	gen.output.WriteString("}\n")
}

// generateSwitchCaseAssignment generates an assignment for a case body
func (gen *CodeGenerator) generateSwitchCaseAssignment(caseBody *ahoy.ASTNode, targetVar string) {
	// Check if body is a block with multiple statements
	if caseBody.Type == ahoy.NODE_BLOCK && len(caseBody.Children) > 0 {
		// Execute all statements except last, then assign last
		for i := 0; i < len(caseBody.Children)-1; i++ {
			gen.generateNodeInternal(caseBody.Children[i], true)
		}
		// Last statement is the return value
		lastStmt := caseBody.Children[len(caseBody.Children)-1]
		gen.writeIndent()
		gen.output.WriteString(fmt.Sprintf("%s = ", targetVar))
		gen.generateNode(lastStmt)
		gen.output.WriteString(";\n")
	} else {
		// Single expression
		gen.writeIndent()
		gen.output.WriteString(fmt.Sprintf("%s = ", targetVar))
		gen.generateNode(caseBody)
		gen.output.WriteString(";\n")
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
			gen.generateNodeInternal(caseBody, true) // Case body
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
		gen.generateNodeInternal(defaultBody, true) // Default body
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
				// Generate body after all case labels
				gen.indent++
				gen.indent++
				gen.generateNodeInternal(caseNode.Children[1], true) // Case body
				gen.writeIndent()
				gen.output.WriteString("break;\n")
				gen.indent--
				gen.indent--
			} else if caseValue.Type == ahoy.NODE_SWITCH_CASE_RANGE {
				// Range case - generate if-else ladder
				// We'll convert this to a default case with if statement
				gen.indent++
				gen.writeIndent()
				gen.output.WriteString("default:\n")
				gen.indent++
				gen.writeIndent()
				gen.output.WriteString("if (")
				gen.generateNode(node.Children[0]) // Switch expr
				gen.output.WriteString(" >= ")
				gen.generateNode(caseValue.Children[0]) // Start
				gen.output.WriteString(" && ")
				gen.generateNode(node.Children[0]) // Switch expr
				gen.output.WriteString(" <= ")
				gen.generateNode(caseValue.Children[1]) // End
				gen.output.WriteString(") {\n")
				gen.indent++
				gen.generateNodeInternal(caseNode.Children[1], true) // Case body
				gen.writeIndent()
				gen.output.WriteString("break;\n")
				gen.indent--
				gen.writeIndent()
				gen.output.WriteString("}\n")
				gen.indent--
				gen.indent--
			} else {
				// Single case value or default case
				gen.indent++
				gen.writeIndent()

				// Check if it's a default case (underscore)
				if caseValue.Type == ahoy.NODE_IDENTIFIER && caseValue.Value == "_" {
					gen.output.WriteString("default:\n")
				} else {
					gen.output.WriteString("case ")
					// Resolve dot-prefixed enum members
					gen.generateSwitchCaseValue(caseValue, switchExprType)
					gen.output.WriteString(":\n")
				}

				gen.indent++
				gen.generateNodeInternal(caseNode.Children[1], true) // Case body
				gen.writeIndent()
				gen.output.WriteString("break;\n")
				gen.indent--
				gen.indent--
			}
		}
	}

	gen.writeIndent()
	gen.output.WriteString("}\n")
}

// generateSwitchCaseValue generates a case value, resolving dot-prefixed enum members
func (gen *CodeGenerator) generateSwitchCaseValue(val *ahoy.ASTNode, switchExprType string) {
	// Check if this is a dot-prefixed enum member
	if val.Type == ahoy.NODE_IDENTIFIER && len(val.Value) > 0 && val.Value[0] == '.' {
		// Extract the member name (without the dot)
		memberName := val.Value[1:]

		// Try to find which enum this member belongs to
		// Only search using original enum names (lowercase)
		found := false
		for enumName := range gen.enumOriginalNames {
			if gen.enums[enumName] != nil && gen.enums[enumName][memberName] {
				// Generate the C enum constant name using the original enum name
				gen.output.WriteString(fmt.Sprintf("%s_%s", enumName, memberName))
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
	gen.output.WriteString(fmt.Sprintf("#ifdef %s\n", node.Value))

	gen.indent++
	gen.generateNodeInternal(node.Children[0], false)
	gen.indent--

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
		gen.output.WriteString(fmt.Sprintf("int %s = ", loopVar))
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
		gen.output.WriteString(fmt.Sprintf("int %s = 0;\n", loopVar))
		gen.writeIndent()
	} else {
		// Pattern 3: loop till condition (no loop variable)
		conditionNode = node.Children[0]
		bodyNode = node.Children[1]
	}

	gen.output.WriteString("while (")
	gen.generateNode(conditionNode)
	gen.output.WriteString(") {\n")

	gen.indent++
	gen.enterScope()
	gen.enterLoopScope() // Mark this as a loop boundary for break/continue
	gen.generateNodeInternal(bodyNode, false)

	// Increment loop variable if present
	if loopVar != "" {
		gen.writeIndent()
		gen.output.WriteString(fmt.Sprintf("%s++;\n", loopVar))
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

		gen.output.WriteString(fmt.Sprintf("for (int %s = ", loopVar))
		gen.generateNode(node.Children[1])
		gen.output.WriteString(fmt.Sprintf("; %s < ", loopVar))
		gen.generateNode(node.Children[2])
		gen.output.WriteString(fmt.Sprintf("; %s++) {\n", loopVar))

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

			gen.output.WriteString(fmt.Sprintf("for (int %s = %s; %s <= %s; %s++) {\n",
				loopVar, startVal, loopVar, endVal, loopVar))

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
		} else {
			// Pattern 2: Variable range (loop:start to end)
			gen.output.WriteString(fmt.Sprintf("for (int %s = ", loopVar))
			gen.generateNode(node.Children[0])
			gen.output.WriteString(fmt.Sprintf("; %s <= ", loopVar))
			gen.generateNode(node.Children[1])
			gen.output.WriteString(fmt.Sprintf("; %s++) {\n", loopVar))

			gen.indent++
			gen.enterScope()
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

		gen.output.WriteString(fmt.Sprintf("int %s = ", loopVar))
		gen.generateNode(node.Children[1])
		gen.output.WriteString(";\n")
		gen.writeIndent()
		gen.output.WriteString(fmt.Sprintf("for (; ; %s++) {\n", loopVar))

		gen.indent++
		gen.enterScope()
		gen.enterLoopScope()
		gen.generateNodeInternal(node.Children[2], false)
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
	} else if len(node.Children) == 2 && node.Children[0].Type == ahoy.NODE_IDENTIFIER {
		// Old pattern: loop i do (forever loop with explicit variable starting at 0)
		loopVar := node.Children[0].Value

		// Use block scope to avoid variable redeclaration
		gen.output.WriteString("{\n")
		gen.indent++
		gen.enterScope()
		gen.writeIndent()

		gen.output.WriteString(fmt.Sprintf("int %s = 0;\n", loopVar))
		gen.writeIndent()
		gen.output.WriteString(fmt.Sprintf("for (; ; %s++) {\n", loopVar))

		gen.indent++
		gen.enterScope()
		gen.enterLoopScope()
		gen.generateNodeInternal(node.Children[1], false)
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
	}
}

func (gen *CodeGenerator) generateAssignmentForFor(node *ahoy.ASTNode) {
	if node.Type == ahoy.NODE_ASSIGNMENT {
		// Type inference
		valueNode := node.Children[0]
		varType := gen.inferType(valueNode)
		gen.variables[node.Value] = varType

		cType := gen.mapType(varType)
		gen.output.WriteString(fmt.Sprintf("%s %s = ", cType, node.Value))
		gen.generateNode(valueNode)
	}
}

func (gen *CodeGenerator) generateAssignmentForUpdate(node *ahoy.ASTNode) {
	if node.Type == ahoy.NODE_ASSIGNMENT {
		// Just assignment, no declaration
		gen.output.WriteString(fmt.Sprintf("%s = ", node.Value))
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

		gen.output.WriteString(fmt.Sprintf("for (int %s = 0; %s[%s] != '\\0'; %s++) {\n",
			loopVar, iterableName, loopVar, loopVar))

		gen.indent++
		gen.enterScope()
		gen.enterLoopScope()
		gen.writeIndent()

		gen.output.WriteString(fmt.Sprintf("char %s = %s[%s];\n",
			elementVar, iterableName, loopVar))

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
	} else {
		// Array iteration
		loopVar := fmt.Sprintf("__loop_i_%d", gen.varCounter)
		gen.varCounter++

		arrayName := gen.nodeToString(iterableExpr)

		// AhoyArray uses 'length', not 'size'
		gen.output.WriteString(fmt.Sprintf("for (int %s = 0; %s < %s->length; %s++) {\n",
			loopVar, loopVar, arrayName, loopVar))

		gen.indent++
		gen.enterScope()
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
			gen.output.WriteString(fmt.Sprintf("%s %s = *(%s*)(intptr_t)%s->data[%s];\n",
				cElementType, elementVar, cElementType, arrayName, loopVar))
		} else if gen.isHeapAllocatedType(elementType) || strings.Contains(cElementType, "*") {
			// For pointers (strings, etc.), cast through intptr_t to pointer
			gen.output.WriteString(fmt.Sprintf("%s %s = (%s)(intptr_t)%s->data[%s];\n",
				cElementType, elementVar, cElementType, arrayName, loopVar))
		} else {
			// For primitives, cast through intptr_t to the appropriate type
			gen.output.WriteString(fmt.Sprintf("%s %s = (%s)(intptr_t)%s->data[%s];\n",
				cElementType, elementVar, cElementType, arrayName, loopVar))
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
	gen.output.WriteString(fmt.Sprintf("for (int %s = 0; %s < %s->capacity; %s++) {\n",
		bucketVar, bucketVar, dictRef, bucketVar))

	gen.indent++
	gen.enterScope()
	gen.enterLoopScope()
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("HashMapEntry* %s = %s->buckets[%s];\n",
		entryVar, dictRef, bucketVar))

	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("while (%s != NULL) {\n", entryVar))

	gen.indent++
	gen.enterScope()
	gen.enterLoopScope() // Inner loop for entry traversal
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("const char* %s = %s->key;\n", keyVar, entryVar))

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
		gen.output.WriteString(fmt.Sprintf("%s %s = (%s)%s->value;\n", valueCType, valueVar, valueCType, entryVar))

		// Register loop variables
		gen.variables[keyVar] = "char*"
		gen.variables[valueVar] = valueType
	} else {
		// For untyped dicts, expose value as intptr_t which can be cast as needed
		gen.writeIndent()
		gen.output.WriteString(fmt.Sprintf("intptr_t %s = (intptr_t)%s->value;\n", valueVar, entryVar))

		// Register loop variables
		gen.variables[keyVar] = "char*"
		gen.variables[valueVar] = "intptr_t"
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

	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("%s = %s->next;\n", entryVar, entryVar))

	gen.exitLoopScope() // Exit inner loop scope
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
}

func (gen *CodeGenerator) generateReturnStatement(node *ahoy.ASTNode) {
	// Mark returned variables as escaping
	if len(node.Children) > 0 {
		for _, child := range node.Children {
			gen.markEscapingVariables(child)
		}
	}

	// Execute deferred statements in LIFO order before return
	if len(gen.deferredStatements) > 0 {
		for i := len(gen.deferredStatements) - 1; i >= 0; i-- {
			gen.output.WriteString(gen.deferredStatements[i])
		}
		// Clear deferred statements so they don't execute again at function end
		gen.deferredStatements = nil
	}

	gen.writeIndent()
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
				gen.output.WriteString(fmt.Sprintf(".ret%d = ", i))

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

func (gen *CodeGenerator) generateGotoStatement(node *ahoy.ASTNode) {
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("goto %s;\n", node.Value))
}

func (gen *CodeGenerator) generateLabelDeclaration(node *ahoy.ASTNode) {
	// Generate the label (C requires a statement after a label, so we'll add one if needed)
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("%s:;\n", node.Value)) // Note the ; after : for empty label

	// Generate the block body if present
	if len(node.Children) > 0 && node.Children[0].Type == ahoy.NODE_BLOCK {
		for _, stmt := range node.Children[0].Children {
			gen.generateNodeInternal(stmt, true)
		}
	}
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
		gen.deferredStatements = append(gen.deferredStatements, deferredCode)
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
}

// enterLoopScope marks the current scope as a loop boundary for break/continue handling
func (gen *CodeGenerator) enterLoopScope() {
	gen.loopScopeStack = append(gen.loopScopeStack, gen.scopeDepth)
}

// exitLoopScope removes the loop boundary marker
func (gen *CodeGenerator) exitLoopScope() {
	if len(gen.loopScopeStack) > 0 {
		gen.loopScopeStack = gen.loopScopeStack[:len(gen.loopScopeStack)-1]
	}
}

// generateEarlyExitCleanup generates cleanup code for variables from current scope down to loop scope
// Used for break/continue/return statements
func (gen *CodeGenerator) generateEarlyExitCleanup(includeLoopScope bool) string {
	var cleanup strings.Builder

	// Determine the target depth (where to stop cleaning up)
	targetDepth := 0
	if len(gen.loopScopeStack) > 0 {
		loopScope := gen.loopScopeStack[len(gen.loopScopeStack)-1]
		if includeLoopScope {
			// Clean including the loop scope
			targetDepth = loopScope
		} else {
			// Clean down to but not including the loop scope
			// This means clean the loop scope but not outer scopes
			targetDepth = loopScope
		}
	}

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
func (gen *CodeGenerator) exitScope() string {
	var cleanup strings.Builder

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

	gen.scopeDepth--
	return cleanup.String()
}

// generateFreeCodeForVar generates the appropriate free() code for a variable based on its type
func (gen *CodeGenerator) generateFreeCodeForVar(varName string, varType string) string {
	// Check if this is an ARC-enabled struct - use release instead of free
	if gen.enableARC {
		// Remove pointer suffix to get base type
		baseType := strings.TrimSuffix(varType, "*")
		if gen.arcStructs[baseType] || gen.arcStructs[strings.ToLower(baseType)] {
			funcName := toCFuncName(strings.ToLower(baseType))
			return fmt.Sprintf("    ahoy_release_%s(%s);\n", funcName, varName)
		}
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
			gen.deferredStatements = append(gen.deferredStatements, freeCode)
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
						}
					}
				}
			}
		}
	}
}

func (gen *CodeGenerator) generateCall(node *ahoy.ASTNode) {
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
	case "print":
		// Check if we have multiple arguments or if first arg is a format string
		hasMultipleArgs := len(node.Children) > 1
		firstIsString := len(node.Children) > 0 && node.Children[0].Type == ahoy.NODE_STRING

		// If first argument is a string AND it looks like a format string (has {} or %), treat it as one
		if firstIsString && !hasMultipleArgs {
			// Single string argument - just print it
			gen.output.WriteString("printf(")
			formatStr := node.Children[0].Value
			if !strings.HasSuffix(formatStr, "\\n") {
				formatStr += "\\n"
			}
			gen.output.WriteString(fmt.Sprintf("\"%s\"", formatStr))
			gen.output.WriteString(")")
			return
		} else if firstIsString && (strings.Contains(node.Children[0].Value, "{}") || strings.Contains(node.Children[0].Value, "%")) {
			// First arg is a format string with placeholders
			gen.output.WriteString("printf(")
			formatStr := node.Children[0].Value
			args := node.Children[1:]

			// Process %v and %t in format string
			processedFormat, processedArgs := gen.processFormatString(formatStr, args)

			// Auto-add newline if not present
			if !strings.HasSuffix(processedFormat, "\\n") {
				processedFormat += "\\n"
			}

			// Output processed format string
			gen.output.WriteString(fmt.Sprintf("\"%s\"", processedFormat))

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
			if len(node.Children) > 0 {
				formatParts := []string{}

				// Build format string with spaces between arguments
				for _, arg := range node.Children {
					argType := gen.inferType(arg)
					formatSpec := ""

					// Check if this is HashMap member access or dict access - we can't determine type at codegen time
					isHashMapAccess := false
					if arg.Type == ahoy.NODE_MEMBER_ACCESS && len(arg.Children) > 0 {
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
				gen.output.WriteString(fmt.Sprintf("\"%s\"", formatStr))

				// Output all arguments
				for _, arg := range node.Children {
					gen.output.WriteString(", ")
					argType := gen.inferType(arg)

					// Check if argument is an enum itself (print the whole enum)
					if arg.Type == ahoy.NODE_IDENTIFIER && gen.isEnumType(arg.Value) {
						gen.output.WriteString(fmt.Sprintf("print_%s()", arg.Value))
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
									gen.output.WriteString(fmt.Sprintf("format_dict_value(%s, \"%s\")", dictName, key))
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
							if objType == "HashMap*" || objType == "dict" {
								// Use format_dict_value helper
								gen.output.WriteString("format_dict_value(")
								gen.generateNode(arg.Children[0])
								gen.output.WriteString(fmt.Sprintf(", \"%s\")", arg.Value))
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
				gen.output.WriteString(fmt.Sprintf("fprintf(__log_file, \"%s\\n\"", formatStr))
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
				gen.output.WriteString(fmt.Sprintf("printf(\"%s\")", formatStr))
			} else if firstIsString && (strings.Contains(node.Children[0].Value, "{}") || strings.Contains(node.Children[0].Value, "%")) {
				// Format string with placeholders
				gen.output.WriteString("printf(")
				formatStr := node.Children[0].Value
				args := node.Children[1:]

				processedFormat, processedArgs := gen.processFormatString(formatStr, args)

				if !strings.HasSuffix(processedFormat, "\\n") {
					processedFormat += "\\n"
				}

				gen.output.WriteString(fmt.Sprintf("\"%s\"", processedFormat))

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
					gen.output.WriteString(fmt.Sprintf("\"%s\"", format))

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

			gen.output.WriteString(fmt.Sprintf(", \"%s\"", processedFormat))

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
		gen.output.WriteString("((int)(")
		if len(node.Children) > 0 {
			gen.generateNode(node.Children[0])
		}
		gen.output.WriteString("))")

	case "float":
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
		gen.output.WriteString(fmt.Sprintf("%s(", funcName))

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

func (gen *CodeGenerator) generateBinaryOp(node *ahoy.ASTNode) {
	switch node.Value {
	case "is":
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
		gen.output.WriteString(fmt.Sprintf(" %s ", node.Value))
		gen.generateNode(node.Children[1])
		gen.output.WriteString(")")
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

	// Constants at global scope (not in a function) should go into constantDecls
	if gen.currentFunction == "" {
		savedOutput := gen.output
		gen.output = strings.Builder{}

		gen.output.WriteString(fmt.Sprintf("const %s %s = ", constType, constName))
		gen.generateNode(node.Children[0])
		gen.output.WriteString(";\n")

		gen.constantDecls.WriteString(gen.output.String())
		gen.output = savedOutput
	} else {
		// Local constants in functions
		gen.writeIndent()
		gen.output.WriteString(fmt.Sprintf("const %s %s = ", constType, constName))
		gen.generateNode(node.Children[0])
		gen.output.WriteString(";\n")
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

		// Generate string method function call
		gen.output.WriteString(fmt.Sprintf("ahoy_string_%s(", methodName))
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
		gen.output.WriteString(fmt.Sprintf("ahoy_dict_%s(", methodName))
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
					gen.output.WriteString(fmt.Sprintf("({ %s* __tmp = malloc(sizeof(%s)); *__tmp = ", structType, structType))
					gen.generateNodeInternal(arg, false)
					gen.output.WriteString("; __tmp; })")
				} else {
					gen.generateNodeInternal(arg, false)
				}

				valueType := gen.getValueType(arg)
				gen.output.WriteString(fmt.Sprintf(", %s)", gen.getAhoyTypeEnum(valueType)))
			}
			return
		}

		// Generate array method function call
		gen.output.WriteString(fmt.Sprintf("ahoy_array_%s(", methodName))
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
					// Mark heap-allocated variables as escaping when pushed to arrays
					if methodName == "push" && arg.Type == ahoy.NODE_IDENTIFIER && gen.heapAllocatedVars[arg.Value] {
						gen.escapingVars[arg.Value] = true
					}

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
							gen.output.WriteString(fmt.Sprintf("({ %s* __tmp = malloc(sizeof(%s)); *__tmp = ", structType, structType))
							gen.generateNodeInternal(arg, false)
							gen.output.WriteString("; __tmp; })")
						} else {
							gen.generateNodeInternal(arg, false)
						}
					} else {
						gen.generateNodeInternal(arg, false)
					}
				} else {
					gen.generateNodeInternal(arg, false)
				}
				// For push and fill, also pass the type
				if methodName == "push" && i == 0 {
					valueType := gen.getValueType(arg)
					gen.output.WriteString(fmt.Sprintf(", %s", gen.getAhoyTypeEnum(valueType)))
				}
				if methodName == "fill" && i == 0 {
					valueType := gen.getValueType(arg)
					gen.output.WriteString(fmt.Sprintf(", %s", gen.getAhoyTypeEnum(valueType)))
				}
			}
		}
		gen.output.WriteString(")")
	}
}

func (gen *CodeGenerator) generateUnaryOp(node *ahoy.ASTNode) {
	switch node.Value {
	case "not":
		gen.output.WriteString("!")
	case "^":
		// Pointer dereference - convert ^ to *
		gen.output.WriteString("*")
	case "&":
		// Address-of operator
		gen.output.WriteString("&")
	default:
		gen.output.WriteString(node.Value)
	}
	gen.generateNode(node.Children[0])
}

func (gen *CodeGenerator) generateTernary(node *ahoy.ASTNode) {
	// C ternary: condition ? true_expr : false_expr
	gen.output.WriteString("(")
	gen.generateNode(node.Children[0]) // condition
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
	gen.output.WriteString(fmt.Sprintf("AhoyArray* %s = malloc(sizeof(AhoyArray)); ", arrName))
	gen.output.WriteString(fmt.Sprintf("%s->length = %d; ", arrName, len(node.Children)))
	gen.output.WriteString(fmt.Sprintf("%s->capacity = %d; ", arrName, len(node.Children)))
	gen.output.WriteString(fmt.Sprintf("%s->data = malloc(%d * sizeof(intptr_t)); ", arrName, len(node.Children)))
	gen.output.WriteString(fmt.Sprintf("%s->types = malloc(%d * sizeof(AhoyValueType)); ", arrName, len(node.Children)))

	// Set typed/mixed flag - only typed if explicitly annotated
	if isTyped {
		gen.output.WriteString(fmt.Sprintf("%s->is_typed = 1; ", arrName))
		gen.output.WriteString(fmt.Sprintf("%s->element_type = %s; ", arrName, gen.getAhoyTypeEnum(elementType)))
	} else {
		gen.output.WriteString(fmt.Sprintf("%s->is_typed = 0; ", arrName))
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
		gen.output.WriteString(fmt.Sprintf("%s->types[%d] = %s; ", arrName, i, gen.getAhoyTypeEnum(valueType)))

		// Special handling for floats - need to allocate heap memory
		if valueType == "float" || valueType == "double" {
			gen.output.WriteString(fmt.Sprintf("%s->data[%d] = (intptr_t)({ double* __float_ptr_%d = malloc(sizeof(double)); *__float_ptr_%d = ", arrName, i, gen.varCounter, gen.varCounter))
			gen.varCounter++
			gen.generateNode(child)
			gen.output.WriteString(fmt.Sprintf("; __float_ptr_%d; }); ", gen.varCounter-1))
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
			gen.output.WriteString(fmt.Sprintf("%s->data[%d] = (intptr_t)({ %s* __struct_ptr_%d = malloc(sizeof(%s)); *__struct_ptr_%d = ",
				arrName, i, actualCType, gen.varCounter, actualCType, gen.varCounter))
			gen.varCounter++
			gen.generateNode(child)
			gen.output.WriteString(fmt.Sprintf("; __struct_ptr_%d; }); ", gen.varCounter-1))
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
				gen.output.WriteString(fmt.Sprintf("%s->data[%d] = (intptr_t)({ %s* __struct_ptr_%d = malloc(sizeof(%s)); *__struct_ptr_%d = ",
					arrName, i, childCType, gen.varCounter, childCType, gen.varCounter))
				gen.varCounter++
				gen.generateNode(child)
				gen.output.WriteString(fmt.Sprintf("; __struct_ptr_%d; }); ", gen.varCounter-1))
			} else {
				gen.output.WriteString(fmt.Sprintf("%s->data[%d] = (intptr_t)", arrName, i))
				gen.generateNode(child)
				gen.output.WriteString("; ")
			}
		}
	}

	gen.output.WriteString(fmt.Sprintf("%s; })", arrName))
}

func (gen *CodeGenerator) generateArrayAccess(node *ahoy.ASTNode) {
	// Check if this is a chained array access (2D array): grid[r][c]
	// In this case, Value is empty and Children[0] is the inner array access, Children[1] is the outer index
	if node.Value == "" && len(node.Children) == 2 {
		gen.generateChainedArrayAccess(node)
		return
	}

	arrayName := node.Value

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
			gen.output.WriteString(fmt.Sprintf("AhoyArray* __arr = (AhoyArray*)%s; ", arrayName))
		} else {
			gen.output.WriteString(fmt.Sprintf("AhoyArray* __arr = %s; ", arrayName))
		}

		gen.output.WriteString("if (__idx < 0 || __idx >= __arr->length) { ")
		gen.output.WriteString("fprintf(stderr, \"RUNTIME ERROR: Array bounds violation\\n\"); ")
		gen.output.WriteString(fmt.Sprintf("fprintf(stderr, \"  File: %s\\n\"); ", gen.sourceFilename))
		gen.output.WriteString(fmt.Sprintf("fprintf(stderr, \"  Line: %d\\n\"); ", node.Line))
		gen.output.WriteString(fmt.Sprintf("fprintf(stderr, \"  Array: %s\\n\"); ", arrayName))
		gen.output.WriteString("fprintf(stderr, \"  Index: %d\\n\", __idx); ")
		gen.output.WriteString("fprintf(stderr, \"  Valid range: 0 to %d\\n\", __arr->length - 1); ")
		gen.output.WriteString("exit(1); ")
		gen.output.WriteString("} ")

		// Check if we know the element type
		if elemType, exists := gen.arrayElementTypes[arrayName]; exists {
			cType := gen.mapType(elemType)
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
					gen.output.WriteString(fmt.Sprintf("(*(%s*)__arr->data[__idx])", cType))
				} else {
					gen.output.WriteString(fmt.Sprintf("((%s)(intptr_t)__arr->data[__idx])", cType))
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
	if elemType, exists := gen.arrayElementTypes[arrayName]; exists {
		cType := gen.mapType(elemType)
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
					gen.output.WriteString(fmt.Sprintf("(*(%s*)((AhoyArray*)%s)->data[", cType, arrayName))
				} else {
					gen.output.WriteString(fmt.Sprintf("(*(%s*)%s->data[", cType, arrayName))
				}
				gen.generateNode(node.Children[0])
				gen.output.WriteString("])")
			} else {
				if needsArrayCast {
					gen.output.WriteString(fmt.Sprintf("((%s)(intptr_t)((AhoyArray*)%s)->data[", cType, arrayName))
				} else {
					gen.output.WriteString(fmt.Sprintf("((%s)(intptr_t)%s->data[", cType, arrayName))
				}
				gen.generateNode(node.Children[0])
				gen.output.WriteString("])")
			}
			return
		}
	}

	// Default: no cast for int/intptr_t values to preserve lvalue for assignments
	if needsArrayCast {
		gen.output.WriteString(fmt.Sprintf("((AhoyArray*)%s)->data[", arrayName))
	} else {
		gen.output.WriteString(fmt.Sprintf("%s->data[", arrayName))
	}
	gen.generateNode(node.Children[0])
	gen.output.WriteString("]")
}

// generateChainedArrayAccess handles 2D array access: grid[r][c]
// Children[0] = inner array access (grid[r]), Children[1] = outer index (c)
func (gen *CodeGenerator) generateChainedArrayAccess(node *ahoy.ASTNode) {
	innerAccess := node.Children[0]
	outerIndex := node.Children[1]

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
		gen.output.WriteString(dictName)
		gen.output.WriteString(", ")
	} else {
		gen.output.WriteString(fmt.Sprintf("hashMapGetDouble(%s, ", dictName))
	}

	gen.generateNode(node.Children[0])
	gen.output.WriteString(")")
}

func (gen *CodeGenerator) generateDictLiteral(node *ahoy.ASTNode) {
	dictName := fmt.Sprintf("dict_%d", gen.varCounter)
	gen.varCounter++

	gen.output.WriteString(fmt.Sprintf("({ HashMap* %s = createHashMap(16); ", dictName))

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

		gen.output.WriteString(fmt.Sprintf("hashMapPutTyped(%s, ", dictName))

		// If key is an identifier, convert to string literal
		if key.Type == ahoy.NODE_IDENTIFIER {
			gen.output.WriteString(fmt.Sprintf("\"%s\"", key.Value))
		} else {
			gen.generateNode(key)
		}

		// For floats, allocate heap memory to store the value properly
		if valueType == "float" {
			floatVar := fmt.Sprintf("__float_ptr_%d", gen.varCounter)
			gen.varCounter++
			gen.output.WriteString(fmt.Sprintf(", (void*)({ double* %s = malloc(sizeof(double)); *%s = ", floatVar, floatVar))
			gen.generateNode(value)
			gen.output.WriteString(fmt.Sprintf("; %s; }), %s); ", floatVar, ahoyTypeEnum))
		} else {
			gen.output.WriteString(", (void*)(intptr_t)")
			gen.generateNode(value)
			gen.output.WriteString(fmt.Sprintf(", %s); ", ahoyTypeEnum))
		}
	}

	gen.output.WriteString(fmt.Sprintf("%s; })", dictName))
}

func (gen *CodeGenerator) mapType(langType string) string {
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

	// Check if any C type matches case-insensitively for backward compatibility
	lowerLangType := strings.ToLower(langType)
	for cType := range gen.cTypeDefinitions {
		if strings.ToLower(cType) == lowerLangType {
			// Return the properly-cased version from the type definitions
			return cType
		}
	}

	return "int"
}

func (gen *CodeGenerator) inferType(node *ahoy.ASTNode) string {
	switch node.Type {
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
		// Don't infer element type from contents - only use explicit type annotations
		// Untyped arrays are just "array"
		return "array"
	case ahoy.NODE_OBJECT_LITERAL:
		// Check if it's a typed object literal
		if node.Value != "" {
			return node.Value
		}
		return "struct"
	case ahoy.NODE_CALL:
		// Infer return type of function calls
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
		// String method split returns array
		if node.Value == "split" {
			return "array"
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
		if leftType == "float" || rightType == "float" {
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
		if varType, exists := gen.variables[node.Value]; exists {
			// Normalize dict types
			if strings.HasPrefix(varType, "dict<") || strings.HasPrefix(varType, "dict[") {
				return "dict"
			}
			// Return full array type including element type for proper iteration
			// DON'T normalize array[ElementType] to just "array"
			return varType
		}
		if varType, exists := gen.functionVars[node.Value]; exists {
			// Normalize dict types
			if strings.HasPrefix(varType, "dict<") || strings.HasPrefix(varType, "dict[") {
				return "dict"
			}
			// Return full array type including element type
			return varType
		}
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

		if elemType, exists := gen.arrayElementTypes[arrayName]; exists {
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
				// Find the field type
				for _, field := range structInfo.Fields {
					if field.Name == memberName {
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
	// Parse f-string and extract variables
	// Example: "hello{i}" -> format string "hello%d" and variables [i]
	fstring := node.Value
	var formatStr strings.Builder
	var vars []string
	var formatSpecs []string

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
				}

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
		gen.output.WriteString(fmt.Sprintf("\"%s\"", formatStr.String()))
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
		gen.output.WriteString(fmt.Sprintf("static char %s[256];\n", bufferVar))
		gen.writeIndent()
		gen.output.WriteString(fmt.Sprintf("sprintf(%s, \"%s\"", bufferVar, formatStr.String()))

		for idx, v := range vars {
			gen.output.WriteString(", ")

			// Get the format spec for this variable
			formatSpec := "%d"
			if idx < len(formatSpecs) {
				formatSpec = formatSpecs[idx]
			}

			// Wrap the variable expression in a casting expression if needed
			needsCast := false
			castType := ""
			if formatSpec == "%d" {
				// Cast to int for %d
				needsCast = true
				castType = "(int)"
			} else if formatSpec == "%f" {
				// Cast to double for %f
				needsCast = true
				castType = "(double)"
			}

			if needsCast {
				gen.output.WriteString(castType)
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

					// Use -> for pointer types
					if strings.HasSuffix(objectType, "*") {
						gen.output.WriteString(fmt.Sprintf("%s->%s", objectName, memberName))
					} else {
						gen.output.WriteString(v)
					}
				} else {
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
						gen.output.WriteString(fmt.Sprintf("(%s)->length", argsStr))
					} else if argType == "dict" || strings.HasPrefix(argType, "dict[") || argType == "HashMap*" {
						gen.output.WriteString(fmt.Sprintf("(%s)->size", argsStr))
					} else if argType == "string" || argType == "char*" || argType == "const char*" {
						gen.output.WriteString(fmt.Sprintf("strlen(%s)", argsStr))
					} else {
						// Default to ->length
						gen.output.WriteString(fmt.Sprintf("(%s)->length", argsStr))
					}
				} else {
					// Other function calls - convert pipe syntax to C function call
					gen.output.WriteString(fmt.Sprintf("%s(%s)", funcName, argsStr))
				}
			} else {
				// Simple variable
				varType := "int"
				if knownType, exists := gen.variables[v]; exists {
					varType = knownType
				}
				if varType == "intptr_t" {
					gen.output.WriteString(fmt.Sprintf("(char*)%s", v))
				} else {
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
	gen.enumDecls.WriteString(fmt.Sprintf("typedef enum {\n"))

	nextAutoValue := 0
	for _, member := range node.Children {
		// Track this member
		gen.enums[enumName][member.Value] = true
		// Track member type
		gen.enumMemberTypes[fmt.Sprintf("%s.%s", enumName, member.Value)] = "int"

		// Check if member has a custom value (in Children[0])
		if len(member.Children) > 0 && member.Children[0].Type == ahoy.NODE_NUMBER {
			value := member.Children[0].Value
			gen.enumDecls.WriteString(fmt.Sprintf("    %s_%s = %s,\n", enumName, member.Value, value))
			// Parse the value to set nextAutoValue for next member
			if val, err := strconv.Atoi(value); err == nil {
				nextAutoValue = val + 1
			}
		} else {
			// Auto-increment value
			gen.enumDecls.WriteString(fmt.Sprintf("    %s_%s = %d,\n", enumName, member.Value, nextAutoValue))
			nextAutoValue++
		}
	}

	gen.enumDecls.WriteString(fmt.Sprintf("} %s_enum;\n\n", enumName))

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
	gen.output.WriteString(fmt.Sprintf("typedef struct {\n"))
	gen.indent++

	for _, member := range node.Children {
		gen.writeIndent()
		gen.enums[enumName][member.Value] = true
		// Track member type
		gen.enumMemberTypes[fmt.Sprintf("%s.%s", enumName, member.Value)] = "char*"

		// Check if member is mutable
		if member.IsMutable {
			gen.output.WriteString(fmt.Sprintf("char* %s;\n", member.Value))
		} else {
			gen.output.WriteString(fmt.Sprintf("const char* %s;\n", member.Value))
		}
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("} %s_struct;\n\n", enumName))

	// Generate instance with initializer
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("%s_struct %s = {\n", enumName, enumName))
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
		gen.output.WriteString(fmt.Sprintf(".%s = %s,\n", member.Value, value))
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString("};\n\n")
}

// Generate float enum using struct
func (gen *CodeGenerator) generateFloatEnum(node *ahoy.ASTNode) {
	enumName := node.Value

	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("typedef struct {\n"))
	gen.indent++

	for _, member := range node.Children {
		gen.writeIndent()
		gen.enums[enumName][member.Value] = true

		if member.IsMutable {
			gen.output.WriteString(fmt.Sprintf("float %s;\n", member.Value))
		} else {
			gen.output.WriteString(fmt.Sprintf("const float %s;\n", member.Value))
		}
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("} %s_struct;\n\n", enumName))

	// Generate instance
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("%s_struct %s = {\n", enumName, enumName))
	gen.indent++

	for _, member := range node.Children {
		gen.writeIndent()
		var value string
		if len(member.Children) > 0 && member.Children[0].Type == ahoy.NODE_NUMBER {
			value = member.Children[0].Value
		} else {
			value = "0.0"
		}
		gen.output.WriteString(fmt.Sprintf(".%s = %s,\n", member.Value, value))
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
	gen.output.WriteString(fmt.Sprintf("typedef struct {\n"))
	gen.indent++

	for _, member := range node.Children {
		gen.writeIndent()
		gen.enums[enumName][member.Value] = true
		// Track member type
		gen.enumMemberTypes[fmt.Sprintf("%s.%s", enumName, member.Value)] = enumType

		// Check if member is mutable
		if member.IsMutable {
			gen.output.WriteString(fmt.Sprintf("%s %s;\n", enumType, member.Value))
		} else {
			gen.output.WriteString(fmt.Sprintf("const %s %s;\n", enumType, member.Value))
		}
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("} %s_struct;\n\n", enumName))

	// Generate instance with initializer
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("%s_struct %s = {\n", enumName, enumName))
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
		gen.output.WriteString(fmt.Sprintf(".%s = %s,\n", member.Value, value))
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
	gen.output.WriteString(fmt.Sprintf("typedef struct {\n"))
	gen.indent++

	for _, member := range node.Children {
		gen.writeIndent()
		gen.enums[enumName][member.Value] = true
		// Collections are always mutable (pointer-based)
		gen.output.WriteString(fmt.Sprintf("%s %s;\n", cType, member.Value))
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("} %s_struct;\n\n", enumName))

	// Generate instance - initialized later or in init function
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("%s_struct %s;\n\n", enumName, enumName))

	// TODO: Generate initialization function for arrays/dicts
}

// Generate mixed enum using struct with generic types
func (gen *CodeGenerator) generateMixedEnum(node *ahoy.ASTNode) {
	enumName := node.Value

	// Track enum type
	gen.enumTypes[enumName] = "mixed"

	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("typedef struct {\n"))
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
			gen.output.WriteString(fmt.Sprintf("%s %s;\n", memberType, member.Value))
		} else {
			// Add const for immutable non-pointer types
			if !strings.Contains(memberType, "*") {
				gen.output.WriteString(fmt.Sprintf("const %s %s;\n", memberType, member.Value))
			} else {
				gen.output.WriteString(fmt.Sprintf("%s %s;\n", memberType, member.Value))
			}
		}
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("} %s_struct;\n\n", enumName))

	// Generate instance with initializers
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("%s_struct %s = {\n", enumName, enumName))
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
					tempBuf.WriteString(fmt.Sprintf("arr->length = %d; ", len(arrayNode.Children)))
					tempBuf.WriteString(fmt.Sprintf("arr->capacity = %d; ", len(arrayNode.Children)))
					tempBuf.WriteString("arr->data = malloc(")
					tempBuf.WriteString(fmt.Sprintf("%d * sizeof(intptr_t)); ", len(arrayNode.Children)))
					tempBuf.WriteString("arr->types = malloc(")
					tempBuf.WriteString(fmt.Sprintf("%d * sizeof(AhoyValueType)); ", len(arrayNode.Children)))
					tempBuf.WriteString("arr->is_typed = 0; ")

					// Initialize elements
					for i, elem := range arrayNode.Children {
						if elem.Type == ahoy.NODE_NUMBER {
							tempBuf.WriteString(fmt.Sprintf("arr->data[%d] = %s; ", i, elem.Value))
							tempBuf.WriteString(fmt.Sprintf("arr->types[%d] = AHOY_TYPE_INT; ", i))
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
		gen.output.WriteString(fmt.Sprintf(".%s = %s,\n", member.Value, value))
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString("};\n\n")
}

// Generate helper struct for enum member access (for int enums)
func (gen *CodeGenerator) generateEnumAccessStruct(node *ahoy.ASTNode, baseType string) {
	enumName := node.Value

	// Generate access struct typedef to enumDecls
	gen.enumDecls.WriteString(fmt.Sprintf("typedef struct {\n"))

	for _, member := range node.Children {
		gen.enumDecls.WriteString(fmt.Sprintf("    const int %s;\n", member.Value))
	}

	gen.enumDecls.WriteString(fmt.Sprintf("} %s_struct;\n\n", enumName))

	// Generate instance to constantDecls (it's a global constant)
	gen.constantDecls.WriteString(fmt.Sprintf("%s_struct %s = {\n", enumName, enumName))

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
		gen.constantDecls.WriteString(fmt.Sprintf("    .%s = %d,\n", member.Value, value))
	}

	gen.constantDecls.WriteString("};\n\n")
}

// Generate enum print helper function
func (gen *CodeGenerator) generateEnumPrintHelper(node *ahoy.ASTNode, enumName string, enumType string) {
	// Generate a helper function that returns a string representation of the enum
	funcName := fmt.Sprintf("print_%s", enumName)

	gen.funcDecls.WriteString(fmt.Sprintf("char* %s() {\n", funcName))
	gen.funcDecls.WriteString("    char* buffer = malloc(512);\n")
	gen.funcDecls.WriteString("    int offset = 0;\n")
	gen.funcDecls.WriteString(fmt.Sprintf("    offset += sprintf(buffer + offset, \"enum:%s %s(\");\n", enumType, enumName))

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

		gen.funcDecls.WriteString(fmt.Sprintf("    offset += sprintf(buffer + offset, \"%s:%s\");\n",
			member.Value, valueStr))
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
						gen.output.WriteString(fmt.Sprintf("%s %s;\n", cType, target.Value))
						gen.variables[target.Value] = exprType
						continue
					}
				}
			}
			// Fallback type
			gen.writeIndent()
			gen.output.WriteString(fmt.Sprintf("int %s;\n", target.Value))
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
						gen.output.WriteString(fmt.Sprintf("%s = ", leftSide.Children[j].Value))
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
						gen.output.WriteString(fmt.Sprintf("%s = ", leftSide.Children[j].Value))
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
					gen.output.WriteString(fmt.Sprintf("%s = ", leftSide.Children[j].Value))
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
		gen.output.WriteString(fmt.Sprintf("%s_return %s = ", structName, tempVar))
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
				gen.output.WriteString(fmt.Sprintf("%s ", cType))

				// If we need to cast from intptr_t (for generic types), do it here
				if needsCast {
					if cType == "char*" {
						gen.output.WriteString(fmt.Sprintf("%s = (char*)%s.ret%d;\n", target.Value, tempVar, i))
					} else {
						gen.output.WriteString(fmt.Sprintf("%s = (%s)%s.ret%d;\n", target.Value, cType, tempVar, i))
					}
				} else {
					gen.output.WriteString(fmt.Sprintf("%s = %s.ret%d;\n", target.Value, tempVar, i))
				}
			} else {
				gen.output.WriteString(fmt.Sprintf("%s = %s.ret%d;\n", target.Value, tempVar, i))
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
		gen.output.WriteString(fmt.Sprintf("%s %s = ", cType, tempVar))
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
				gen.output.WriteString(fmt.Sprintf("%s ", cType))
				if gen.functionVars != nil {
					gen.functionVars[target.Value] = tempType
				} else {
					gen.variables[target.Value] = tempType
				}
			}
			gen.output.WriteString(fmt.Sprintf("%s = %s;\n", target.Value, temps[i]))
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

	gen.structDecls.WriteString(fmt.Sprintf("typedef struct {\n"))

	// Add ARC reference count field if enabled
	if gen.enableARC {
		gen.structDecls.WriteString("    int __arc_refcount;\n")
		gen.arcStructs[structName] = true
		gen.arcStructs[cStructName] = true
	}

	// Track static fields separately - they will be generated as global variables
	var staticFields []*ahoy.ASTNode

	// First pass: detect parent-child relationships for weak references
	if gen.enableARC {
		for _, field := range baseFields {
			if field.IsStatic {
				continue
			}
			fieldType := field.DataType
			// Check if this field is a struct type (potential child relationship)
			if _, isStruct := gen.structs[fieldType]; isStruct {
				// Check if the field type has fields that reference this struct (circular dependency)
				if gen.hasCircularDependency(structName, fieldType) {
					// Mark this field as weak to break the cycle
					if gen.weakFields[structName] == nil {
						gen.weakFields[structName] = make(map[string]bool)
					}
					gen.weakFields[structName][field.Value] = true

					// Track parent-child relationship
					if gen.parentChildRelations[structName] == nil {
						gen.parentChildRelations[structName] = make(map[string]bool)
					}
					gen.parentChildRelations[structName][field.Value] = true
				}
			}
		}
	}

	for _, field := range baseFields {
		// Skip static fields - they are not part of the struct
		if field.IsStatic {
			staticFields = append(staticFields, field)
			continue
		}

		fieldType := gen.mapType(field.DataType)
		gen.structDecls.WriteString(fmt.Sprintf("    %s %s;\n", fieldType, field.Value))

		// Track field info with default value
		defaultValue := gen.generateDefaultValue(field.DefaultValue)
		isWeak := gen.enableARC && gen.weakFields[structName] != nil && gen.weakFields[structName][field.Value]
		structInfo.Fields = append(structInfo.Fields, StructField{
			Name:         field.Value,
			Type:         fieldType,
			DefaultValue: defaultValue,
			IsStatic:     false,
			IsConst:      field.IsConst,
			IsWeak:       isWeak,
		})
	}

	gen.structDecls.WriteString(fmt.Sprintf("} %s;\n\n", cStructName))

	// Generate static fields as global variables
	for _, field := range staticFields {
		fieldType := gen.mapType(field.DataType)
		defaultValue := "0"
		if field.DefaultValue != nil {
			defaultValue = gen.generateDefaultValue(field.DefaultValue)
		}
		// Static field name format: StructName_fieldname
		staticFieldName := fmt.Sprintf("%s_%s", cStructName, field.Value)
		gen.structDecls.WriteString(fmt.Sprintf("static %s %s = %s;\n", fieldType, staticFieldName, defaultValue))

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
			builder.WriteString(fmt.Sprintf("(%s){", typeName))

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
			builder.WriteString(fmt.Sprintf(", %s); ", gen.getAhoyTypeEnum(valueType)))
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
				builder.WriteString(fmt.Sprintf(", %s); ", gen.getAhoyTypeEnum(valueType)))
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

	gen.structDecls.WriteString(fmt.Sprintf("typedef struct {\n"))

	// First, include parent fields (skip those overridden by nested type)
	for _, field := range parentFields {
		// Skip if the nested type overrides this field
		if nestedFieldNames[field.Value] {
			continue
		}

		fieldType := gen.mapType(field.DataType)
		gen.structDecls.WriteString(fmt.Sprintf("    %s %s;\n", fieldType, field.Value))

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
		gen.structDecls.WriteString(fmt.Sprintf("    %s %s;\n", fieldType, field.Value))

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
	gen.structDecls.WriteString(fmt.Sprintf("} %s;\n\n", cTypeName))

	// Also generate a typedef alias for the full name syntax
	gen.structDecls.WriteString(fmt.Sprintf("typedef %s %s;\n\n", cTypeName, fullCTypeName))

	// Store struct info with multiple names for lookup
	gen.structs[typeName] = structInfo
	gen.structs[cTypeName] = structInfo
	gen.structs[fullTypeName] = structInfo
	gen.structs[fullCTypeName] = structInfo
}

// Generate method call

// Generate member access
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
		gen.output.WriteString(fmt.Sprintf(", \"%s\")", memberName))
		return
	}

	if objectType == "HashMap*" || objectType == "dict" {
		// Anonymous object stored in HashMap - use hashMapGet
		// Note: returns void*, caller needs to cast appropriately
		gen.output.WriteString("hashMapGet(")
		gen.generateNodeInternal(object, false)
		gen.output.WriteString(fmt.Sprintf(", \"%s\")", memberName))
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
				gen.output.WriteString(fmt.Sprintf("((%s*)((AhoyArray*)", cType))
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
					gen.output.WriteString(fmt.Sprintf("\"%s\"", ahoyType))
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
				gen.output.WriteString(fmt.Sprintf("\"enum:%s\"", enumType))
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
		gen.output.WriteString(fmt.Sprintf("if (%s != NULL && %s->is_typed) { ", objectName, objectName))
		gen.output.WriteString(fmt.Sprintf("const char* elem_type = ahoy_type_enum_to_string(%s->element_type); ", objectName))
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
		gen.output.WriteString(fmt.Sprintf("strcpy(__type_str, \"%s\"); ", ahoyType))
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

// Generate signal handler for better crash reporting
func (gen *CodeGenerator) getSignalHandler() string {
	return `// Signal handler for crash reporting
#include <signal.h>

void ahoy_signal_handler(int sig) {
    fprintf(stderr, "\n");
    fprintf(stderr, "========================================\n");
    fprintf(stderr, "  Ahoy Program Crashed\n");
    fprintf(stderr, "========================================\n");
    fprintf(stderr, "\n");

    switch(sig) {
        case SIGSEGV:
            fprintf(stderr, "Error: Memory access violation (segmentation fault)\n");
            fprintf(stderr, "This usually happens when:\n");
            fprintf(stderr, "  - Accessing memory that doesn't belong to your program\n");
            fprintf(stderr, "  - Using a null pointer\n");
            fprintf(stderr, "  - Accessing freed memory\n");
            break;
        case SIGABRT:
            fprintf(stderr, "Error: Program aborted\n");
            fprintf(stderr, "This usually happens when:\n");
            fprintf(stderr, "  - An assertion failed\n");
            fprintf(stderr, "  - A serious error was detected\n");
            break;
        case SIGFPE:
            fprintf(stderr, "Error: Arithmetic error (floating point exception)\n");
            fprintf(stderr, "This usually happens when:\n");
            fprintf(stderr, "  - Dividing by zero\n");
            fprintf(stderr, "  - Integer overflow\n");
            break;
        case SIGILL:
            fprintf(stderr, "Error: Illegal instruction\n");
            fprintf(stderr, "This usually happens when:\n");
            fprintf(stderr, "  - Corrupted memory\n");
            fprintf(stderr, "  - Invalid code execution\n");
            break;
        default:
            fprintf(stderr, "Error: Program received signal %d\n", sig);
            break;
    }

    fprintf(stderr, "\n");
    fprintf(stderr, "Tips for debugging:\n");
    fprintf(stderr, "  - Check array accesses are within bounds\n");
    fprintf(stderr, "  - Ensure variables are initialized before use\n");
    fprintf(stderr, "  - Verify pointers are not null\n");
    fprintf(stderr, "\n");
    fprintf(stderr, "========================================\n");

    exit(1);
}

void ahoy_setup_signal_handlers() {
    signal(SIGSEGV, ahoy_signal_handler);
    signal(SIGABRT, ahoy_signal_handler);
    signal(SIGFPE, ahoy_signal_handler);
    signal(SIGILL, ahoy_signal_handler);
}
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
			return varType
		}
		return "int" // Default
	default:
		return "int" // Default
	}
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
			gen.output.WriteString(fmt.Sprintf("int %s = __elem->data[%d]; ", paramName, i))
		}
	} else {
		gen.output.WriteString(fmt.Sprintf("int %s = __src->data[__i]; ", params[0]))
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
			gen.output.WriteString(fmt.Sprintf("int %s = __elem->data[%d]; ", paramName, i))
		}
	} else {
		gen.output.WriteString(fmt.Sprintf("int %s = __src->data[__i]; ", params[0]))
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
		gen.output.WriteString(fmt.Sprintf("__result->data[__result->length++] = (intptr_t)%s; ", params[0]))
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
		gen.funcForwardDecls.WriteString(fmt.Sprintf("char* print_struct_helper_%s(%s obj);\n", funcName, cStructName))
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
		gen.funcDecls.WriteString(fmt.Sprintf("\n// Print helper for %s\n", structInfo.Name))
		gen.funcDecls.WriteString(fmt.Sprintf("char* print_struct_helper_%s(%s obj) {\n", funcName, cStructName))
		// Use malloc instead of static buffer to avoid overwrites in nested calls
		gen.funcDecls.WriteString("    char* buffer = malloc(1024);\n")
		gen.funcDecls.WriteString("    if (!buffer) return \"<out of memory>\";\n")

		// Anonymous structs use {} format, named structs use name{} format
		if strings.HasPrefix(structInfo.Name, "__anon_struct_") {
			gen.funcDecls.WriteString("    sprintf(buffer, \"{")
		} else {
			gen.funcDecls.WriteString(fmt.Sprintf("    sprintf(buffer, \"%s{", structInfo.Name))
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
		gen.funcDecls.WriteString("}\", ")

		// Add field values (only for non-array/dict fields)
		firstValue := true
		for _, field := range structInfo.Fields {
			// Skip arrays and dicts - they're already in the format string
			if field.Type == "AhoyArray*" || field.Type == "HashMap*" {
				continue
			}

			if !firstValue {
				gen.funcDecls.WriteString(", ")
			}
			firstValue = false

			// For static fields, access the global variable instead of obj.field
			if field.IsStatic {
				staticFieldName := fmt.Sprintf("%s_%s", cStructName, field.Name)
				if field.Type == "bool" {
					gen.funcDecls.WriteString(fmt.Sprintf("%s ? \"true\" : \"false\"", staticFieldName))
				} else {
					gen.funcDecls.WriteString(staticFieldName)
				}
			} else if field.Type == "bool" {
				gen.funcDecls.WriteString(fmt.Sprintf("obj.%s ? \"true\" : \"false\"", field.Name))
			} else if field.Type == "char*" || field.Type == "const char*" {
				gen.funcDecls.WriteString(fmt.Sprintf("(obj.%s ? obj.%s : \"\")", field.Name, field.Name))
			} else if _, isStruct := gen.structs[strings.ToLower(field.Type)]; isStruct {
				// For struct fields, call their print helper
				gen.funcDecls.WriteString(fmt.Sprintf("print_struct_helper_%s(obj.%s)", strings.ToLower(field.Type), field.Name))
			} else {
				gen.funcDecls.WriteString(fmt.Sprintf("obj.%s", field.Name))
			}
		}

		gen.funcDecls.WriteString(");\n")
		gen.funcDecls.WriteString("    return buffer;\n")
		gen.funcDecls.WriteString("}\n")
	}
}

// writeARCHelperFunctions generates retain/release functions for ARC-enabled structs
func (gen *CodeGenerator) writeARCHelperFunctions() {
	if !gen.enableARC || len(gen.arcStructs) == 0 {
		return
	}

	// Track which structs we've processed to avoid duplicates
	processed := make(map[string]bool)

	// Sort struct names for deterministic output
	structNames := make([]string, 0, len(gen.structs))
	for name := range gen.structs {
		if gen.arcStructs[name] {
			structNames = append(structNames, name)
		}
	}
	sort.Strings(structNames)

	gen.funcDecls.WriteString("\n// ===== ARC (Automatic Reference Counting) Helper Functions =====\n\n")

	for _, name := range structNames {
		structInfo := gen.structs[name]
		if processed[structInfo.Name] {
			continue
		}
		// Skip JSON structs
		if gen.jsonStructs[structInfo.Name] {
			continue
		}
		processed[structInfo.Name] = true

		cStructName := toCStructName(structInfo.Name)
		funcName := toCFuncName(structInfo.Name)

		// Generate retain function
		gen.funcDecls.WriteString(fmt.Sprintf("// Retain (increment reference count) for %s\n", structInfo.Name))
		gen.funcDecls.WriteString(fmt.Sprintf("%s* ahoy_retain_%s(%s* obj) {\n", cStructName, funcName, cStructName))
		gen.funcDecls.WriteString("    if (obj == NULL) return NULL;\n")
		gen.funcDecls.WriteString("    obj->__arc_refcount++;\n")
		gen.funcDecls.WriteString("    return obj;\n")
		gen.funcDecls.WriteString("}\n\n")

		// Generate release function
		gen.funcDecls.WriteString(fmt.Sprintf("// Release (decrement reference count and free if zero) for %s\n", structInfo.Name))
		gen.funcDecls.WriteString(fmt.Sprintf("void ahoy_release_%s(%s* obj) {\n", funcName, cStructName))
		gen.funcDecls.WriteString("    if (obj == NULL) return;\n")
		gen.funcDecls.WriteString("    obj->__arc_refcount--;\n")
		gen.funcDecls.WriteString("    if (obj->__arc_refcount <= 0) {\n")

		// Release nested ARC objects (non-weak references, only pointers)
		for _, field := range structInfo.Fields {
			if field.IsWeak {
				continue // Skip weak references - don't release them
			}

			// Only release if it's a pointer field
			if !strings.HasSuffix(field.Type, "*") {
				continue
			}

			// Check if field is an ARC struct pointer
			fieldBaseName := strings.TrimSuffix(field.Type, "*")
			if gen.arcStructs[fieldBaseName] || gen.arcStructs[strings.ToLower(fieldBaseName)] {
				fieldFuncName := toCFuncName(strings.ToLower(fieldBaseName))
				gen.funcDecls.WriteString(fmt.Sprintf("        ahoy_release_%s(obj->%s);\n", fieldFuncName, field.Name))
			}
		}

		gen.funcDecls.WriteString("        free(obj);\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("}\n\n")

		// Generate autorelease function (for use with auto-defer)
		gen.funcDecls.WriteString(fmt.Sprintf("// Autorelease (defer release) for %s\n", structInfo.Name))
		gen.funcDecls.WriteString(fmt.Sprintf("%s* ahoy_autorelease_%s(%s* obj) {\n", cStructName, funcName, cStructName))
		gen.funcDecls.WriteString("    // Will be released at end of scope via defer\n")
		gen.funcDecls.WriteString("    return obj;\n")
		gen.funcDecls.WriteString("}\n\n")
	}
}

func (gen *CodeGenerator) writeStringHelperFunctions() {
	if len(gen.stringMethods) == 0 {
		return
	}

	gen.includes["ctype.h"] = true // For tolower/toupper
	gen.includes["regex.h"] = true // For regex matching

	// Helper function to duplicate strings
	gen.funcDecls.WriteString("\n// String Helper Functions\n")
	gen.funcDecls.WriteString("char* ahoy_string_dup(const char* src) {\n")
	gen.funcDecls.WriteString("    if (!src) return NULL;\n")
	gen.funcDecls.WriteString("    char* dest = malloc(strlen(src) + 1);\n")
	gen.funcDecls.WriteString("    strcpy(dest, src);\n")
	gen.funcDecls.WriteString("    return dest;\n")
	gen.funcDecls.WriteString("}\n\n")

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
		gen.funcDecls.WriteString("// Note: split returns a NULL-terminated array of strings\n")
		gen.funcDecls.WriteString("char** ahoy_string_split(const char* str, const char* delim) {\n")
		gen.funcDecls.WriteString("    if (!str || !delim) return NULL;\n")
		gen.funcDecls.WriteString("    char* str_copy = ahoy_string_dup(str);\n")
		gen.funcDecls.WriteString("    int count = 1;\n")
		gen.funcDecls.WriteString("    for (const char* p = str; *p; p++) {\n")
		gen.funcDecls.WriteString("        if (strstr(p, delim) == p) count++;\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    char** result = malloc((count + 1) * sizeof(char*));\n")
		gen.funcDecls.WriteString("    char* token = strtok(str_copy, delim);\n")
		gen.funcDecls.WriteString("    int i = 0;\n")
		gen.funcDecls.WriteString("    while (token != NULL) {\n")
		gen.funcDecls.WriteString("        result[i++] = ahoy_string_dup(token);\n")
		gen.funcDecls.WriteString("        token = strtok(NULL, delim);\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    result[i] = NULL;\n")
		gen.funcDecls.WriteString("    free(str_copy);\n")
		gen.funcDecls.WriteString("    return result;\n")
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
		gen.output.WriteString(fmt.Sprintf("(%s)", structName))
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

		// Initialize ARC refcount if enabled for this struct
		if gen.enableARC && gen.arcStructs[node.Value] {
			if !first {
				gen.output.WriteString(", ")
			}
			gen.output.WriteString(".__arc_refcount = 1")
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

	gen.output.WriteString(fmt.Sprintf("({ HashMap* %s = createHashMap(16); ", dictName))

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

			gen.output.WriteString(fmt.Sprintf("hashMapPutTyped(%s, \"%s\", (void*)(intptr_t)",
				dictName, prop.Value))
			if len(prop.Children) > 0 {
				gen.generateNode(prop.Children[0])
			} else {
				gen.output.WriteString("0")
			}
			gen.output.WriteString(fmt.Sprintf(", %s); ", ahoyTypeEnum))
		}
	}

	gen.output.WriteString(fmt.Sprintf("%s; })", dictName))
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
		gen.output.WriteString(fmt.Sprintf("((char*)hashMapGet("))
		// Cast generic/any/intptr_t to HashMap*
		if objectType == "generic" || objectType == "any" || objectType == "intptr_t" {
			gen.output.WriteString("(HashMap*)")
		}
		gen.output.WriteString(objectName)
		gen.output.WriteString(fmt.Sprintf(", \"%s\"))", propertyName))
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
	// Check if this is a dot-prefixed enum member
	if len(memberName) > 0 && memberName[0] == '.' {
		// Extract member name without dot
		actualMemberName := memberName[1:]

		// Search only original enum names (not capitalized aliases) for deterministic results
		var foundEnum string

		for enumName := range gen.enumOriginalNames {
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
	for enumName := range gen.enumOriginalNames {
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
