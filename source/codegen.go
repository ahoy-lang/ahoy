package main

import (
	"fmt"
	"strconv"
	"strings"

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

type StructField struct {
	Name         string
	Type         string
	DefaultValue string // C code for default value (if any)
}

type StructInfo struct {
	Name   string
	Fields []StructField
}

type CodeGenerator struct {
	output                        strings.Builder
	indent                        int
	varCounter                    int
	dictCounter                   int                        // Counter for inline dict/array literals
	funcReturnStructs             strings.Builder            // Struct definitions for multi-return functions
	funcForwardDecls              strings.Builder            // Forward declarations for user functions
	funcDecls                     strings.Builder
	structDecls                   strings.Builder
	includes                      map[string]bool
	orderedIncludes               []string                   // Keep track of include order
	variables                     map[string]string          // variable name -> type (global scope)
	functionVars                  map[string]string          // variable name -> type (function scope)
	nestedScopeVars               map[string]bool            // variables declared in nested scopes (loops/ifs)
	constants                     map[string]bool            // constant name -> declared
	enums                         map[string]map[string]bool // enum name -> {member names}
	enumMemberTypes               map[string]string          // "enumName.memberName" -> type
	enumTypes                     map[string]string          // enum name -> enum type (int, string, etc.)
	userFunctions                 map[string]bool            // user-defined function names (keep snake_case)
	hasError                      bool                       // Track if error occurred
	arrayImpls                    bool                       // Track if we've added array implementation
	arrayMethods                  map[string]bool            // Track which array methods are used
	stringMethods                 map[string]bool            // Track which string methods are used
	dictMethods                   map[string]bool            // Track which dict methods are used
	useJSON                       bool                       // Track if JSON functions are used
	jsonVariables                 map[string]bool            // Track which variables hold JSON data
	jsonStructs                   map[string]bool            // Track which structs are JSON schemas (not real C structs)
	loopCounters                  []string                   // Stack of loop counter variable names
	currentFunction               string                     // Current function being generated
	currentFunctionReturnType     string                     // Return type of current function
	currentFunctionHasMultiReturn bool                       // Whether current function has multiple returns
	hasMainFunc                   bool                       // Whether there's an Ahoy main function
	arrayElementTypes             map[string]string          // array variable name -> element type
	structs                       map[string]*StructInfo     // struct name -> struct info
	currentTypeContext            string                     // Current type annotation context (e.g., "array[int]")
	functionReturnTypes           map[string][]string        // function name -> return types (for inferred functions)
	deferredStatements            []string                   // Stack of deferred statements for current function
	functionParamTypes            map[string][]string        // function name -> parameter types
	functionParamNames            map[string][]string        // function name -> parameter names
	functionParamDefaults         map[string][]*ahoy.ASTNode // function name -> parameter default values
	dictSourcedVars               map[string]string          // variable name -> dict name (for dict-accessed vars)
	dictSourcedKeys               map[string]string          // variable name -> key (for dict-accessed vars)
	cFunctionNames                map[string]string          // snake_case name -> actual C name
	cNamespaces                   map[string]map[string]string // namespace -> (snake_case name -> actual C name)
}

// GenerateC generates C code from an AST (exported for testing)
func GenerateC(ast *ahoy.ASTNode) string {
	return generateC(ast)
}

func generateC(ast *ahoy.ASTNode) string {
	gen := &CodeGenerator{
		includes:            make(map[string]bool),
		orderedIncludes:     make([]string, 0),
		variables:           make(map[string]string),
		constants:           make(map[string]bool),
		enums:               make(map[string]map[string]bool),
		enumMemberTypes:     make(map[string]string),
		enumTypes:           make(map[string]string),
		userFunctions:       make(map[string]bool),
		hasError:            false,
		arrayImpls:          false,
		arrayMethods:        make(map[string]bool),
		stringMethods:       make(map[string]bool),
		dictMethods:         make(map[string]bool),
		hasMainFunc:         false,
		arrayElementTypes:   make(map[string]string),
		structs:             make(map[string]*StructInfo),
		functionReturnTypes:   make(map[string][]string),
		functionParamTypes:    make(map[string][]string),
		functionParamNames:    make(map[string][]string),
		functionParamDefaults: make(map[string][]*ahoy.ASTNode),
		dictSourcedVars:       make(map[string]string),
		dictSourcedKeys:       make(map[string]string),
		nestedScopeVars:       make(map[string]bool),
		cFunctionNames:        make(map[string]string),
		cNamespaces:           make(map[string]map[string]string),
		jsonVariables:         make(map[string]bool),
		jsonStructs:           make(map[string]bool),
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

	// Generate hash map implementation
	gen.writeHashMapImplementation()

	// First pass: check if there's a main function and collect function signatures
	gen.checkForMainFunction(ast)
	
	// Second pass: infer return types for all functions with infer keyword
	gen.inferAllFunctionReturnTypes(ast)
	
	// Third pass: scan for method calls to determine which helper functions we need
	gen.scanForMethodCalls(ast)

	// Generate main code
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
	result.WriteString("    AHOY_TYPE_CHAR\n")
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
		result.WriteString("char* print_array_helper(AhoyArray* arr);\n")
		
		// Define Color and Vector2 types for helpers only if raylib is not imported
		// raylib already defines these types
		hasRaylib := false
		for include := range gen.includes {
			if strings.Contains(include, "raylib") {
				hasRaylib = true
				break
			}
		}
		
		if !hasRaylib {
			result.WriteString("typedef struct Color { unsigned char r; unsigned char g; unsigned char b; unsigned char a; } Color;\n")
			result.WriteString("typedef struct Vector2 { float x; float y; } Vector2;\n")
		}
		result.WriteString("char* color_to_string(Color c);\n")
		result.WriteString("char* vector2_to_string(Vector2 v);\n")
		result.WriteString("\n")
	}
	
	// Add forward declarations for dict helper functions if needed
	if gen.dictMethods["print_dict"] {
		result.WriteString("char* print_dict_helper(HashMap* dict);\n")
		result.WriteString("char* format_hashmap_value(HashMap* dict, const char* key);\n")
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
	
	// Write function implementations
	result.WriteString(gen.funcDecls.String())
	result.WriteString("\n")

	// Write main program
	if gen.hasMainFunc {
		// If there's an Ahoy main function, just call it
		result.WriteString("int main() {\n")
		result.WriteString("    ahoy_main();\n")
		result.WriteString("    return 0;\n")
		result.WriteString("}\n")
	} else {
		// Legacy: no main function, use global scope code
		result.WriteString("int main() {\n")
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

// scanForMethodCalls scans the AST to find all method calls and mark which helper functions we need
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

	case ahoy.NODE_ASSIGNMENT:
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
		gen.output.WriteString(fmt.Sprintf("\"%s\"", node.Value))

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
	case ahoy.NODE_HALT:
		gen.writeIndent()
		gen.output.WriteString("break;\n")
	case ahoy.NODE_NEXT:
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
					// Use intptr_t for generic types (will be cast at call site)
					var mappedType string
					if rType == "generic" {
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
		} else if strings.Contains(node.DataType, ",") {
			// Multiple return types - create a struct
			// Use smart split that handles nested commas in dict<k,v>
			returnTypes = splitReturnTypes(node.DataType)

			// Generate struct definition for multi-return
			structName := fmt.Sprintf("%s_return", funcName)
			gen.funcReturnStructs.WriteString(fmt.Sprintf("typedef struct {\n"))
			for i, rType := range returnTypes {
				mappedType := gen.mapType(rType)
				gen.funcReturnStructs.WriteString(fmt.Sprintf("    %s ret%d;\n", mappedType, i))
			}
			gen.funcReturnStructs.WriteString(fmt.Sprintf("} %s;\n\n", structName))
			returnType = structName
		} else {
			returnType = gen.mapType(node.DataType)
		}
	}

	// Build parameter list for both declaration and forward declaration
	params := node.Children[0]
	paramList := ""
	for i, param := range params.Children {
		if i > 0 {
			paramList += ", "
		}
		paramType := "intptr_t" // default for untyped/generic parameters
		if param.DataType != "" {
			if param.DataType == "generic" {
				paramType = "intptr_t"  // Use intptr_t for generic parameters
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
			paramTypes = append(paramTypes, "generic")
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
	for _, param := range params.Children {
		if param.DataType != "" {
			gen.functionVars[param.Value] = param.DataType
			
			// Track array element types for typed array parameters
			if strings.HasPrefix(param.DataType, "array[") {
				elemType := strings.TrimSuffix(strings.TrimPrefix(param.DataType, "array["), "]")
				gen.arrayElementTypes[param.Value] = elemType
			}
		} else {
			// Parameters without explicit type are generic
			gen.functionVars[param.Value] = "generic"
		}
	}

	// Initialize deferred statements stack for this function
	gen.deferredStatements = []string{}

	gen.generateNodeInternal(body, false)

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
	gen.functionVars = nil // Clear function scope
	gen.deferredStatements = nil // Clear deferred statements
}

func (gen *CodeGenerator) generateAssignment(node *ahoy.ASTNode) {
	gen.writeIndent()

	// Check if this is a property/element/pointer assignment (obj<'prop'>: value or dict{"key"}: value or obj.prop: value or ^ptr: value)
	// In this case, Children[0] is the access node, Children[1] is the value
	if len(node.Children) == 2 &&
		(node.Children[0].Type == ahoy.NODE_OBJECT_ACCESS ||
			node.Children[0].Type == ahoy.NODE_DICT_ACCESS ||
			node.Children[0].Type == ahoy.NODE_ARRAY_ACCESS ||
			node.Children[0].Type == ahoy.NODE_MEMBER_ACCESS ||
			node.Children[0].Type == ahoy.NODE_UNARY_OP) {

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
			
			// If object is dict, HashMap*, generic, or intptr_t, use hashMapPut
			if objectType == "dict" || objectType == "HashMap*" || objectType == "generic" || objectType == "intptr_t" ||
			   strings.HasPrefix(objectType, "dict[") || strings.HasPrefix(objectType, "dict<") {
				gen.output.WriteString("hashMapPut(")
				// Cast generic/intptr_t to HashMap*
				if objectType == "generic" || objectType == "intptr_t" {
					gen.output.WriteString("(HashMap*)")
				}
				gen.output.WriteString(objectName)
				gen.output.WriteString(fmt.Sprintf(", \"%s\", (void*)(intptr_t)", propertyName))
				gen.generateNode(node.Children[1])
				gen.output.WriteString(");\n")
				return
			}
		}

		// For struct field/array/pointer access, direct assignment works
		gen.generateNode(node.Children[0])
		gen.output.WriteString(" = ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(";\n")
		return
	}

	// Check if variable already exists (function scope first, then global)
	_, existsInFunc := gen.functionVars[node.Value]
	_, existsGlobal := gen.variables[node.Value]
	isNestedScope := gen.nestedScopeVars[node.Value]

	valueNode := node.Children[0]
	
	// Special case: Variables from nested scopes or array/dict access can be redeclared
	isLoopLocalPattern := valueNode.Type == ahoy.NODE_ARRAY_ACCESS || valueNode.Type == ahoy.NODE_DICT_ACCESS
	canRedeclare := isLoopLocalPattern || (isNestedScope && gen.indent > 1)
	
	if (existsInFunc || existsGlobal) && !canRedeclare {
		// Just assignment
		if valueNode.Type == ahoy.NODE_SWITCH_STATEMENT {
			// Generate switch as expression (assign in each case)
			gen.generateSwitchExpression(valueNode, node.Value)
		} else {
			gen.output.WriteString(fmt.Sprintf("%s = ", node.Value))
			gen.generateNode(node.Children[0])
			gen.output.WriteString(";\n")
		}
	} else {
		// Type inference and declaration
		valueNode := node.Children[0]
		
		// Check if we have an explicit type annotation
		explicitType := node.DataType

		// Special handling for object literals - they define their own type inline
		if valueNode.Type == ahoy.NODE_OBJECT_LITERAL {
			// Check if this is a typed struct literal (e.g., rectangle<...>)
			if valueNode.Value != "" {
				// Use the C struct type name (capitalize first letter)
				structName := capitalizeFirst(valueNode.Value)
				gen.output.WriteString(fmt.Sprintf("%s %s = ", structName, node.Value))
				gen.generateNode(valueNode)
				gen.output.WriteString(";\n")

				// Track variable in appropriate scope
				if gen.currentFunction != "" && gen.functionVars != nil {
					gen.functionVars[node.Value] = valueNode.Value
				} else {
					gen.variables[node.Value] = valueNode.Value
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
				if gen.indent > 1 {
					gen.nestedScopeVars[node.Value] = true
				}
			} else {
				// Global scope
				gen.variables[node.Value] = varType
			}

			// If this is an array literal with typed annotation, track the element type
			if valueNode.Type == ahoy.NODE_ARRAY_LITERAL {
				if explicitType != "" && strings.HasPrefix(explicitType, "array[") {
					// Extract element type from array[type]
					elemType := strings.TrimSuffix(strings.TrimPrefix(explicitType, "array["), "]")
					gen.arrayElementTypes[node.Value] = elemType
					// Set context for array literal generation
					gen.currentTypeContext = explicitType
				} else if len(valueNode.Children) > 0 {
					elemType := gen.inferType(valueNode.Children[0])
					gen.arrayElementTypes[node.Value] = elemType
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
	gen.generateNodeInternal(node.Children[1], false)
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
			gen.generateNodeInternal(node.Children[i], false)
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
			gen.generateNodeInternal(node.Children[i+1], false)
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
					gen.generateNode(val)
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
					gen.generateNode(caseValue) // Case value
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

	if len(node.Children) == 4 && node.Children[0].Type == ahoy.NODE_IDENTIFIER {
		// Pattern 1 or 2: loop i:start till condition or loop i till condition
		loopVar = node.Children[0].Value
		startNode := node.Children[1]
		conditionNode = node.Children[2]
		bodyNode = node.Children[3]

		// Create block scope for loop variable
		gen.output.WriteString("{\n")
		gen.indent++
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
	gen.generateNodeInternal(bodyNode, false)

	// Increment loop variable if present
	if loopVar != "" {
		gen.writeIndent()
		gen.output.WriteString(fmt.Sprintf("%s++;\n", loopVar))
	}

	gen.indent--

	gen.writeIndent()
	gen.output.WriteString("}\n")

	// Close block scope if we created one
	if loopVar != "" {
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
		gen.generateNodeInternal(node.Children[3], false)
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
			gen.generateNodeInternal(node.Children[0], false)
			gen.indent--
		} else {
			// Pattern 2: Variable range (loop:start to end)
			gen.output.WriteString(fmt.Sprintf("for (int %s = ", loopVar))
			gen.generateNode(node.Children[0])
			gen.output.WriteString(fmt.Sprintf("; %s <= ", loopVar))
			gen.generateNode(node.Children[1])
			gen.output.WriteString(fmt.Sprintf("; %s++) {\n", loopVar))

			gen.indent++
			gen.generateNodeInternal(node.Children[2], false)
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
		gen.writeIndent()

		gen.output.WriteString(fmt.Sprintf("int %s = ", loopVar))
		gen.generateNode(node.Children[1])
		gen.output.WriteString(";\n")
		gen.writeIndent()
		gen.output.WriteString(fmt.Sprintf("for (; ; %s++) {\n", loopVar))

		gen.indent++
		gen.generateNodeInternal(node.Children[2], false)
		gen.indent--

		gen.writeIndent()
		gen.output.WriteString("}\n")

		gen.indent--
		gen.writeIndent()
		gen.output.WriteString("}\n")
	} else if len(node.Children) == 2 && node.Children[0].Type == ahoy.NODE_IDENTIFIER {
		// Old pattern: loop i do (forever loop with explicit variable starting at 0)
		loopVar := node.Children[0].Value

		// Use block scope to avoid variable redeclaration
		gen.output.WriteString("{\n")
		gen.indent++
		gen.writeIndent()

		gen.output.WriteString(fmt.Sprintf("int %s = 0;\n", loopVar))
		gen.writeIndent()
		gen.output.WriteString(fmt.Sprintf("for (; ; %s++) {\n", loopVar))

		gen.indent++
		gen.generateNodeInternal(node.Children[1], false)
		gen.indent--

		gen.writeIndent()
		gen.output.WriteString("}\n")

		gen.indent--
		gen.writeIndent()
		gen.output.WriteString("}\n")
	} else if len(node.Children) == 1 || node.Value == "0" {
		// Pattern 3: Forever loop without explicit variable (loop: or loop do)
		gen.output.WriteString("for (;;) {\n")

		gen.indent++
		gen.generateNodeInternal(node.Children[0], false)
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
		gen.generateNodeInternal(node.Children[3], false)
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
		gen.writeIndent()

		// Cast from void* through intptr_t to int (handles stored integers correctly)
		gen.output.WriteString(fmt.Sprintf("int %s = (intptr_t)%s->data[%s];\n",
			elementVar, arrayName, loopVar))

		// Register loop variable for type inference
		oldType := gen.variables[elementVar]
		gen.variables[elementVar] = "int"

		gen.generateNodeInternal(node.Children[2], false)

		// Restore old type
		if oldType != "" {
			gen.variables[elementVar] = oldType
		} else {
			delete(gen.variables, elementVar)
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
	
	// Check if we need to cast (for generic parameters)
	dictType := gen.inferType(dictExpr)
	dictRef := dictName
	if dictType == "generic" {
		dictRef = "((HashMap*)" + dictName + ")"
	}

	// Iterate through hash map buckets
	gen.output.WriteString(fmt.Sprintf("for (int %s = 0; %s < %s->capacity; %s++) {\n",
		bucketVar, bucketVar, dictRef, bucketVar))

	gen.indent++
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("HashMapEntry* %s = %s->buckets[%s];\n",
		entryVar, dictRef, bucketVar))

	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("while (%s != NULL) {\n", entryVar))

	gen.indent++
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
	gen.indent--

	gen.writeIndent()
	gen.output.WriteString("}\n")
	gen.indent--

	gen.writeIndent()
	gen.output.WriteString("}\n")
}

func (gen *CodeGenerator) generateReturnStatement(node *ahoy.ASTNode) {
	// Execute deferred statements in LIFO order before return
	if len(gen.deferredStatements) > 0 {
		for i := len(gen.deferredStatements) - 1; i >= 0; i-- {
			gen.output.WriteString(gen.deferredStatements[i])
		}
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
				
				// If this return type is generic (intptr_t) and value is string, cast
				if hasReturnTypes && i < len(returnTypes) && returnTypes[i] == "generic" {
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

func (gen *CodeGenerator) generateImportStatement(node *ahoy.ASTNode) {
	// Add include - check if it's a local or system include
	headerName := node.Value
	namespace := node.DataType // Namespace is stored in DataType field
	
	if !gen.includes[headerName] {
		gen.includes[headerName] = true
		gen.orderedIncludes = append(gen.orderedIncludes, headerName)
		
		// If it's a C header file, parse it to get function name mappings
		if strings.HasSuffix(headerName, ".h") {
			// Try to find and parse the header file
			headerPath := ""
			if strings.HasPrefix(headerName, "/") {
				headerPath = headerName
			} else {
				// Try common locations
				locations := []string{
					headerName,
					"/usr/include/" + headerName,
					"/usr/local/include/" + headerName,
					"repos/raylib/src/" + headerName,
				}
				for _, loc := range locations {
					if _, err := ahoy.ParseCHeader(loc); err == nil {
						headerPath = loc
						break
					}
				}
			}
			
			if headerPath != "" {
				if headerInfo, err := ahoy.ParseCHeader(headerPath); err == nil {
					// If there's a namespace, store functions under that namespace
					if namespace != "" {
						if gen.cNamespaces[namespace] == nil {
							gen.cNamespaces[namespace] = make(map[string]string)
						}
						for cFuncName := range headerInfo.Functions {
							snakeName := ahoy.PascalToSnake(cFuncName)
							gen.cNamespaces[namespace][snakeName] = cFuncName
						}
					} else {
						// No namespace - add to global scope
						for cFuncName := range headerInfo.Functions {
							snakeName := ahoy.PascalToSnake(cFuncName)
							gen.cFunctionNames[snakeName] = cFuncName
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
					
					// Check if this is HashMap member access - we can't determine type at codegen time
					isHashMapAccess := false
					if arg.Type == ahoy.NODE_MEMBER_ACCESS && len(arg.Children) > 0 {
						objType := gen.inferType(arg.Children[0])
						if objType == "HashMap*" || objType == "dict" {
							isHashMapAccess = true
							// For HashMap, format as string by default (will use print_dict_value helper)
							formatSpec = "%s"
						}
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
							case "color", "Color":
								formatSpec = "%s" // Will use color_to_string helper
							case "vector2", "Vector2":
								formatSpec = "%s" // Will use vector2_to_string helper
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
						case "color", "Color":
							formatSpec = "%s" // Will use color_to_string helper
						case "vector2", "Vector2":
							formatSpec = "%s" // Will use vector2_to_string helper
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
					} else if argType == "color" {
						// Color type - use helper
						gen.output.WriteString("color_to_string(")
						gen.generateNode(arg)
						gen.output.WriteString(")")
					} else if argType == "vector2" || argType == "Vector2" {
						// Vector2 type - use helper
						gen.output.WriteString("vector2_to_string(")
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
						// Convert C type to lowercase Ahoy type (Vector2 -> vector2)
						ahoyType := strings.ToLower(argType)
						gen.output.WriteString(ahoyType)
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
								if varType == "generic" {
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
						if hasParamInfo && i < len(paramTypes) && paramTypes[i] == "generic" {
							argType := gen.inferType(argNode)
							// Cast all pointer types to intptr_t for generic parameters
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
						if hasParamInfo && i < len(paramTypes) && paramTypes[i] == "generic" {
							argType := gen.inferType(argNode)
							// Cast all pointer types to intptr_t for generic parameters
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
				
				// If this parameter is generic, cast pointer types to intptr_t
				if hasParamInfo && i < len(paramTypes) && paramTypes[i] == "generic" {
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
	if gen.constants[constName] {
		fmt.Printf("\n❌ Error at line %d: Cannot redeclare constant '%s'\n", node.Line, constName)
		fmt.Printf("   Constants cannot be reassigned or redeclared.\n")
		fmt.Printf("   '%s' was already declared earlier in the code.\n\n", constName)
		gen.hasError = true
		return
	}

	// Mark constant as declared
	gen.constants[constName] = true

	// Determine the constant type - use explicit type if provided, otherwise infer
	var constType string
	if node.DataType != "" {
		constType = gen.mapType(node.DataType)
	} else {
		// Infer type from the value
		inferredType := gen.inferType(node.Children[0])
		constType = gen.mapType(inferredType)
	}

	// Constants at global scope (not in a function) should go into funcDecls
	if gen.currentFunction == "" {
		savedOutput := gen.output
		gen.output = strings.Builder{}
		
		gen.output.WriteString(fmt.Sprintf("const %s %s = ", constType, constName))
		gen.generateNode(node.Children[0])
		gen.output.WriteString(";\n")
		
		gen.funcDecls.WriteString(gen.output.String())
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
		// Cast generic parameters to HashMap*
		if object.Type == ahoy.NODE_IDENTIFIER {
			objType := gen.inferType(object)
			if objType == "generic" {
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
				if i > 0 {
					gen.output.WriteString("; ")
				}
				gen.output.WriteString("ahoy_array_push(")
				gen.generateNodeInternal(object, false)
				gen.output.WriteString(", (intptr_t)")
				gen.generateNodeInternal(arg, false)
				valueType := gen.getValueType(arg)
				gen.output.WriteString(fmt.Sprintf(", %s)", gen.getAhoyTypeEnum(valueType)))
			}
			return
		}

		// Generate array method function call
		gen.output.WriteString(fmt.Sprintf("ahoy_array_%s(", methodName))
		// Cast generic parameters to AhoyArray*
		if object.Type == ahoy.NODE_IDENTIFIER {
			objType := gen.inferType(object)
			if objType == "generic" {
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
					gen.output.WriteString("(intptr_t)")
				}
				gen.generateNodeInternal(arg, false)
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
		} else {
			gen.output.WriteString(fmt.Sprintf("%s->data[%d] = (intptr_t)", arrName, i))
			gen.generateNode(child)
			gen.output.WriteString("; ")
		}
	}

	gen.output.WriteString(fmt.Sprintf("%s; })", arrName))
}

func (gen *CodeGenerator) generateArrayAccess(node *ahoy.ASTNode) {
	arrayName := node.Value
	
	// Check if the variable type is intptr_t, void*, or generic (might need casting to AhoyArray*)
	needsArrayCast := false
	if varType, exists := gen.variables[arrayName]; exists {
		if varType == "intptr_t" || varType == "void*" || varType == "generic" {
			needsArrayCast = true
		}
	}
	if varType, exists := gen.functionVars[arrayName]; exists {
		if varType == "intptr_t" || varType == "void*" || varType == "generic" {
			needsArrayCast = true
		}
	}

	// Check if we know the element type
	if elemType, exists := gen.arrayElementTypes[arrayName]; exists {
		cType := gen.mapType(elemType)
		// Cast to the appropriate type for non-int types (need intptr_t intermediate for pointer safety)
		if cType != "int" {
			if needsArrayCast {
				gen.output.WriteString(fmt.Sprintf("((%s)(intptr_t)((AhoyArray*)%s)->data[", cType, arrayName))
			} else {
				gen.output.WriteString(fmt.Sprintf("((%s)(intptr_t)%s->data[", cType, arrayName))
			}
			gen.generateNode(node.Children[0])
			gen.output.WriteString("])")
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
	
	// Use hashMapGetDouble which converts values to double
	// If generic, cast to HashMap*
	if dictType == "generic" {
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
	
	// Handle known types first before pointer logic
	switch langType {
	case "generic":
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
	
	// Check if it's a struct type (capitalize first letter)
	if _, exists := gen.structs[langType]; exists {
		return capitalizeFirst(langType)
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
	case ahoy.NODE_STRING:
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
		// Check if we know the function's return type
		if returnTypes, exists := gen.functionReturnTypes[node.Value]; exists && len(returnTypes) > 0 {
			// For single return, return that type
			// For multiple returns, this will be used in tuple assignment context
			return returnTypes[0]
		}
		return "int"
	case ahoy.NODE_METHOD_CALL:
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
			node.Value == "fill" {
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
		if varType, exists := gen.variables[node.Value]; exists {
			// Normalize dict types
			if strings.HasPrefix(varType, "dict<") || strings.HasPrefix(varType, "dict[") {
				return "dict"
			}
			if strings.HasPrefix(varType, "array[") {
				return "array"
			}
			return varType
		}
		if varType, exists := gen.functionVars[node.Value]; exists {
			// Normalize dict types
			if strings.HasPrefix(varType, "dict<") || strings.HasPrefix(varType, "dict[") {
				return "dict"
			}
			if strings.HasPrefix(varType, "array[") {
				return "array"
			}
			return varType
		}
		return "int"
	case ahoy.NODE_ARRAY_ACCESS:
		// Get the array variable name and look up its element type
		arrayName := node.Value
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
		// If array is generic, elements are also generic (intptr_t)
		if arrayType == "generic" {
			return "generic"
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

			// Look up the struct definition
			if structInfo, exists := gen.structs[objectType]; exists {
				// Find the field type
				for _, field := range structInfo.Fields {
					if field.Name == memberName {
						return field.Type
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
	for _, param := range params.Children {
		if param.DataType != "" {
			gen.functionVars[param.Value] = param.DataType
		} else {
			// Parameters without explicit type are generic (will be inferred at call site)
			gen.functionVars[param.Value] = "generic"
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

	i := 0
	for i < len(fstring) {
		if fstring[i] == '{' {
			// Find closing brace
			j := i + 1
			for j < len(fstring) && fstring[j] != '}' {
				j++
			}
			if j < len(fstring) {
				// Extract variable name
				varName := fstring[i+1 : j]
				vars = append(vars, varName)

				// Determine format specifier based on variable type
				// For now, use %d for numbers, %s for strings
				// We'll need to look up the variable type
				varType := "int"
				if knownType, exists := gen.variables[varName]; exists {
					varType = knownType
				}

				formatSpec := "%d"
				if varType == "string" || varType == "char*" || varType == "intptr_t" {
					formatSpec = "%s"
				} else if varType == "float" {
					formatSpec = "%f"
				} else if varType == "char" {
					formatSpec = "%c"
				}

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

		for _, v := range vars {
			gen.output.WriteString(", ")
			// Cast intptr_t to char* for string formatting
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
	enumType := node.EnumType

	// Track enum members for validation
	if gen.enums[enumName] == nil {
		gen.enums[enumName] = make(map[string]bool)
	}

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

	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("typedef enum {\n"))
	gen.indent++

	nextAutoValue := 0
	for _, member := range node.Children {
		gen.writeIndent()

		// Track this member
		gen.enums[enumName][member.Value] = true
		// Track member type
		gen.enumMemberTypes[fmt.Sprintf("%s.%s", enumName, member.Value)] = "int"

		// Check if member has a custom value (in Children[0])
		if len(member.Children) > 0 && member.Children[0].Type == ahoy.NODE_NUMBER {
			value := member.Children[0].Value
			gen.output.WriteString(fmt.Sprintf("%s_%s = %s,\n", enumName, member.Value, value))
			// Parse the value to set nextAutoValue for next member
			if val, err := strconv.Atoi(value); err == nil {
				nextAutoValue = val + 1
			}
		} else {
			// Auto-increment value
			gen.output.WriteString(fmt.Sprintf("%s_%s = %d,\n", enumName, member.Value, nextAutoValue))
			nextAutoValue++
		}
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("} %s_enum;\n\n", enumName))

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

	// Generate access struct
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("typedef struct {\n"))
	gen.indent++

	for _, member := range node.Children {
		gen.writeIndent()
		gen.output.WriteString(fmt.Sprintf("const int %s;\n", member.Value))
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("} %s_struct;\n\n", enumName))

	// Generate instance
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("%s_struct %s = {\n", enumName, enumName))
	gen.indent++

	nextAutoValue := 0
	for _, member := range node.Children {
		gen.writeIndent()
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
		gen.output.WriteString(fmt.Sprintf(".%s = %d,\n", member.Value, value))
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString("};\n\n")
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
			gen.jsonVariables[jsonVarName] = true // Track this as a JSON variable
		}

		// Assign struct fields to left side variables
		for i, target := range leftSide.Children {
			gen.writeIndent()
			// Check if variable needs to be declared
			existsInFunc := false
			existsGlobal := false
			if gen.functionVars != nil {
				_, existsInFunc = gen.functionVars[target.Value]
			}
			_, existsGlobal = gen.variables[target.Value]
			
			if !existsInFunc && !existsGlobal {
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
					// If return type is "generic", infer from actual call arguments
					if retTypes[i] == "generic" && i < len(callNode.Children) {
						inferredType = gen.inferType(callNode.Children[i])
						needsCast = true  // Need to cast from intptr_t
					} else {
						inferredType = retTypes[i]
					}
					
					cType = gen.mapType(inferredType)
					if gen.functionVars != nil {
						gen.functionVars[target.Value] = inferredType
					} else {
						gen.variables[target.Value] = inferredType
					}
					// Track JSON variables
					if inferredType == "AhoyJSON*" {
						gen.jsonVariables[target.Value] = true
					}
				} else {
					if gen.functionVars != nil {
						gen.functionVars[target.Value] = "int"
					} else {
						gen.variables[target.Value] = "int"
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
	
	// Skip vector2 and color - they're predefined
	if structName == "vector2" || structName == "color" {
		// Still register struct info for type checking
		cStructName := capitalizeFirst(structName)
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
	cStructName := capitalizeFirst(structName)
	structInfo := &StructInfo{
		Name:   structName,
		Fields: make([]StructField, 0),
	}

	gen.structDecls.WriteString(fmt.Sprintf("typedef struct {\n"))

	for _, field := range baseFields {
		fieldType := gen.mapType(field.DataType)
		gen.structDecls.WriteString(fmt.Sprintf("    %s %s;\n", fieldType, field.Value))

		// Track field info with default value
		defaultValue := gen.generateDefaultValue(field.DefaultValue)
		structInfo.Fields = append(structInfo.Fields, StructField{
			Name:         field.Value,
			Type:         fieldType,
			DefaultValue: defaultValue,
		})
	}

	gen.structDecls.WriteString(fmt.Sprintf("} %s;\n\n", cStructName))

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
		// Handle object literal default values like vector2{x:10, y:20}
		if node.Value != "" {
			// Typed object literal
			typeName := capitalizeFirst(node.Value)
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
	cTypeName := capitalizeFirst(typeName)
	
	// Track struct info
	structInfo := &StructInfo{
		Name:   typeName,
		Fields: make([]StructField, 0),
	}

	gen.structDecls.WriteString(fmt.Sprintf("typedef struct {\n"))

	// First, include parent fields
	for _, field := range parentFields {
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

	gen.structDecls.WriteString(fmt.Sprintf("} %s;\n\n", cTypeName))

	// Store struct info with both lowercase and capitalized names
	gen.structs[typeName] = structInfo
	gen.structs[cTypeName] = structInfo
}

// Generate method call

// Generate member access
func (gen *CodeGenerator) generateMemberAccess(node *ahoy.ASTNode) {
	object := node.Children[0]
	memberName := node.Value

	// Check if this is enum member access (enum_name.MEMBER)
	if object.Type == ahoy.NODE_IDENTIFIER {
		// Check if the identifier is an enum name
		if gen.isEnumType(object.Value) {
			enumName := object.Value
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

	gen.generateNodeInternal(object, false)

	// Check if object is a pointer type (array or struct pointer)
	if objectType == "AhoyArray*" || objectType == "array" ||
		strings.HasSuffix(objectType, "*") {
		gen.output.WriteString("->")
	} else {
		gen.output.WriteString(".")
	}
	gen.output.WriteString(memberName)
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
			return cType  // Already in Ahoy format
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
	gen.funcDecls.WriteString("        default: return \"unknown\";\n")
	gen.funcDecls.WriteString("    }\n")
	gen.funcDecls.WriteString("}\n\n")
}

// Generate array helper functions
func (gen *CodeGenerator) writeArrayHelperFunctions() {
	// Note: AhoyArray structure is now defined in the header section
	
	if len(gen.arrayMethods) == 0 {
		return
	}

	gen.includes["time.h"] = true // For shuffle

	// length method
	if gen.arrayMethods["length"] {
		gen.funcDecls.WriteString("int ahoy_array_length(AhoyArray* arr) {\n")
		gen.funcDecls.WriteString("    return arr->length;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// push method
	if gen.arrayMethods["push"] {
		gen.funcDecls.WriteString("AhoyArray* ahoy_array_push(AhoyArray* arr, intptr_t value, AhoyValueType type) {\n")
		gen.funcDecls.WriteString("    if (arr->length >= arr->capacity) {\n")
		gen.funcDecls.WriteString("        arr->capacity = arr->capacity == 0 ? 4 : arr->capacity * 2;\n")
		gen.funcDecls.WriteString("        arr->data = realloc(arr->data, arr->capacity * sizeof(intptr_t));\n")
		gen.funcDecls.WriteString("        arr->types = realloc(arr->types, arr->capacity * sizeof(AhoyValueType));\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    arr->data[arr->length] = value;\n")
		gen.funcDecls.WriteString("    arr->types[arr->length] = type;\n")
		gen.funcDecls.WriteString("    arr->length++;\n")
		gen.funcDecls.WriteString("    return arr;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// pop method
	if gen.arrayMethods["pop"] {
		gen.funcDecls.WriteString("intptr_t ahoy_array_pop(AhoyArray* arr) {\n")
		gen.funcDecls.WriteString("    if (arr->length == 0) return 0;\n")
		gen.funcDecls.WriteString("    return arr->data[--arr->length];\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// sum method
	if gen.arrayMethods["sum"] {
		gen.funcDecls.WriteString("int ahoy_array_sum(AhoyArray* arr) {\n")
		gen.funcDecls.WriteString("    int total = 0;\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < arr->length; i++) {\n")
		gen.funcDecls.WriteString("        total += (int)arr->data[i];\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    return total;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// has method
	if gen.arrayMethods["has"] {
		gen.funcDecls.WriteString("int ahoy_array_has(AhoyArray* arr, intptr_t value) {\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < arr->length; i++) {\n")
		gen.funcDecls.WriteString("        if (arr->data[i] == value) return 1;\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    return 0;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// sort method
	if gen.arrayMethods["sort"] {
		gen.funcDecls.WriteString("int __ahoy_compare_ints(const void* a, const void* b) {\n")
		gen.funcDecls.WriteString("    return (*(intptr_t*)a - *(intptr_t*)b);\n")
		gen.funcDecls.WriteString("}\n\n")
		gen.funcDecls.WriteString("AhoyArray* ahoy_array_sort(AhoyArray* arr) {\n")
		gen.funcDecls.WriteString("    qsort(arr->data, arr->length, sizeof(intptr_t), __ahoy_compare_ints);\n")
		gen.funcDecls.WriteString("    return arr;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// reverse method
	if gen.arrayMethods["reverse"] {
		gen.funcDecls.WriteString("AhoyArray* ahoy_array_reverse(AhoyArray* arr) {\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < arr->length / 2; i++) {\n")
		gen.funcDecls.WriteString("        intptr_t temp = arr->data[i];\n")
		gen.funcDecls.WriteString("        arr->data[i] = arr->data[arr->length - 1 - i];\n")
		gen.funcDecls.WriteString("        arr->data[arr->length - 1 - i] = temp;\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    return arr;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// shuffle method
	if gen.arrayMethods["shuffle"] {
		gen.funcDecls.WriteString("AhoyArray* ahoy_array_shuffle(AhoyArray* arr) {\n")
		gen.funcDecls.WriteString("    srand(time(NULL));\n")
		gen.funcDecls.WriteString("    for (int i = arr->length - 1; i > 0; i--) {\n")
		gen.funcDecls.WriteString("        int j = rand() % (i + 1);\n")
		gen.funcDecls.WriteString("        intptr_t temp = arr->data[i];\n")
		gen.funcDecls.WriteString("        arr->data[i] = arr->data[j];\n")
		gen.funcDecls.WriteString("        arr->data[j] = temp;\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    return arr;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// pick method
	if gen.arrayMethods["pick"] {
		gen.funcDecls.WriteString("intptr_t ahoy_array_pick(AhoyArray* arr) {\n")
		gen.funcDecls.WriteString("    if (arr->length == 0) return 0;\n")
		gen.funcDecls.WriteString("    srand(time(NULL));\n")
		gen.funcDecls.WriteString("    return arr->data[rand() % arr->length];\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// fill method
	if gen.arrayMethods["fill"] {
		gen.funcDecls.WriteString("AhoyArray* ahoy_array_fill(AhoyArray* arr, intptr_t value, AhoyValueType type, int count) {\n")
		gen.funcDecls.WriteString("    if (count <= 0) return arr;\n")
		gen.funcDecls.WriteString("    if (arr->capacity < count) {\n")
		gen.funcDecls.WriteString("        arr->capacity = count;\n")
		gen.funcDecls.WriteString("        arr->data = realloc(arr->data, arr->capacity * sizeof(intptr_t));\n")
		gen.funcDecls.WriteString("        arr->types = realloc(arr->types, arr->capacity * sizeof(AhoyValueType));\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < count; i++) {\n")
		gen.funcDecls.WriteString("        arr->data[i] = value;\n")
		gen.funcDecls.WriteString("        arr->types[i] = type;\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    arr->length = count;\n")
		gen.funcDecls.WriteString("    return arr;\n")
		gen.funcDecls.WriteString("}\n\n")
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
	// Add color_to_string helper
	gen.funcDecls.WriteString("char* color_to_string(Color c) {\n")
	gen.funcDecls.WriteString("    char* buffer = malloc(64);\n")
	gen.funcDecls.WriteString("    sprintf(buffer, \"color(%d, %d, %d, %d)\", c.r, c.g, c.b, c.a);\n")
	gen.funcDecls.WriteString("    return buffer;\n")
	gen.funcDecls.WriteString("}\n\n")
	
	// Add vector2_to_string helper
	gen.funcDecls.WriteString("char* vector2_to_string(Vector2 v) {\n")
	gen.funcDecls.WriteString("    char* buffer = malloc(64);\n")
	gen.funcDecls.WriteString("    sprintf(buffer, \"vector2(%g, %g)\", v.x, v.y);\n")
	gen.funcDecls.WriteString("    return buffer;\n")
	gen.funcDecls.WriteString("}\n\n")
}

// Generate dictionary helper functions
func (gen *CodeGenerator) writeDictHelperFunctions() {
	if len(gen.dictMethods) == 0 {
		return
	}

	// HashMap structure (if not already defined - should be in stdlib)
	gen.funcDecls.WriteString("\n// Dictionary Helper Methods\n")

	// Check if we need array support for keys() or values() methods
	if gen.dictMethods["keys"] || gen.dictMethods["values"] {
		// Ensure AhoyArray structure is defined
		gen.arrayImpls = true
	}

	// size method
	if gen.dictMethods["size"] {
		gen.funcDecls.WriteString("int ahoy_dict_size(HashMap* dict) {\n")
		gen.funcDecls.WriteString("    if (dict == NULL) return 0;\n")
		gen.funcDecls.WriteString("    return dict->size;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// clear method
	if gen.dictMethods["clear"] {
		gen.funcDecls.WriteString("void ahoy_dict_clear(HashMap* dict) {\n")
		gen.funcDecls.WriteString("    if (dict == NULL) return;\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < dict->capacity; i++) {\n")
		gen.funcDecls.WriteString("        HashMapEntry* entry = dict->buckets[i];\n")
		gen.funcDecls.WriteString("        while (entry != NULL) {\n")
		gen.funcDecls.WriteString("            HashMapEntry* temp = entry;\n")
		gen.funcDecls.WriteString("            entry = entry->next;\n")
		gen.funcDecls.WriteString("            free(temp->key);\n")
		gen.funcDecls.WriteString("            free(temp);\n")
		gen.funcDecls.WriteString("        }\n")
		gen.funcDecls.WriteString("        dict->buckets[i] = NULL;\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    dict->size = 0;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// has method
	if gen.dictMethods["has"] {
		gen.funcDecls.WriteString("int ahoy_dict_has(HashMap* dict, char* key) {\n")
		gen.funcDecls.WriteString("    if (dict == NULL || key == NULL) return 0;\n")
		gen.funcDecls.WriteString("    return hashMapGet(dict, key) != NULL ? 1 : 0;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// has_all method
	if gen.dictMethods["has_all"] {
		gen.funcDecls.WriteString("int ahoy_dict_has_all(HashMap* dict, AhoyArray* keys) {\n")
		gen.funcDecls.WriteString("    if (dict == NULL || keys == NULL) return 0;\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < keys->length; i++) {\n")
		gen.funcDecls.WriteString("        char* key = (char*)(intptr_t)keys->data[i];\n")
		gen.funcDecls.WriteString("        if (hashMapGet(dict, key) == NULL) return 0;\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    return 1;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// keys method
	if gen.dictMethods["keys"] {
		gen.funcDecls.WriteString("AhoyArray* ahoy_dict_keys(HashMap* dict) {\n")
		gen.funcDecls.WriteString("    AhoyArray* arr = malloc(sizeof(AhoyArray));\n")
		gen.funcDecls.WriteString("    arr->length = 0;\n")
		gen.funcDecls.WriteString("    arr->capacity = dict->size;\n")
		gen.funcDecls.WriteString("    arr->data = malloc(arr->capacity * sizeof(int));\n")
		gen.funcDecls.WriteString("    \n")
		gen.funcDecls.WriteString("    for (int i = 0; i < dict->capacity; i++) {\n")
		gen.funcDecls.WriteString("        HashMapEntry* entry = dict->buckets[i];\n")
		gen.funcDecls.WriteString("        while (entry != NULL) {\n")
		gen.funcDecls.WriteString("            arr->data[arr->length++] = (int)(intptr_t)entry->key;\n")
		gen.funcDecls.WriteString("            entry = entry->next;\n")
		gen.funcDecls.WriteString("        }\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    return arr;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// values method
	if gen.dictMethods["values"] {
		gen.funcDecls.WriteString("AhoyArray* ahoy_dict_values(HashMap* dict) {\n")
		gen.funcDecls.WriteString("    AhoyArray* arr = malloc(sizeof(AhoyArray));\n")
		gen.funcDecls.WriteString("    arr->length = 0;\n")
		gen.funcDecls.WriteString("    arr->capacity = dict->size;\n")
		gen.funcDecls.WriteString("    arr->data = malloc(arr->capacity * sizeof(int));\n")
		gen.funcDecls.WriteString("    \n")
		gen.funcDecls.WriteString("    for (int i = 0; i < dict->capacity; i++) {\n")
		gen.funcDecls.WriteString("        HashMapEntry* entry = dict->buckets[i];\n")
		gen.funcDecls.WriteString("        while (entry != NULL) {\n")
		gen.funcDecls.WriteString("            arr->data[arr->length++] = (int)(intptr_t)entry->value;\n")
		gen.funcDecls.WriteString("            entry = entry->next;\n")
		gen.funcDecls.WriteString("        }\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    return arr;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// sort method
	if gen.dictMethods["sort"] {
		gen.funcDecls.WriteString("int __ahoy_compare_keys(const void* a, const void* b) {\n")
		gen.funcDecls.WriteString("    return strcmp((char*)a, (char*)b);\n")
		gen.funcDecls.WriteString("}\n\n")
		gen.funcDecls.WriteString("HashMap* ahoy_dict_sort(HashMap* dict) {\n")
		gen.funcDecls.WriteString("    if (dict == NULL || dict->size == 0) return dict;\n")
		gen.funcDecls.WriteString("    \n")
		gen.funcDecls.WriteString("    // Get all keys\n")
		gen.funcDecls.WriteString("    char** keys = malloc(dict->size * sizeof(char*));\n")
		gen.funcDecls.WriteString("    int idx = 0;\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < dict->capacity; i++) {\n")
		gen.funcDecls.WriteString("        HashMapEntry* entry = dict->buckets[i];\n")
		gen.funcDecls.WriteString("        while (entry != NULL) {\n")
		gen.funcDecls.WriteString("            keys[idx++] = entry->key;\n")
		gen.funcDecls.WriteString("            entry = entry->next;\n")
		gen.funcDecls.WriteString("        }\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    \n")
		gen.funcDecls.WriteString("    // Sort keys\n")
		gen.funcDecls.WriteString("    qsort(keys, dict->size, sizeof(char*), __ahoy_compare_keys);\n")
		gen.funcDecls.WriteString("    \n")
		gen.funcDecls.WriteString("    // Create new sorted dict\n")
		gen.funcDecls.WriteString("    HashMap* sorted = createHashMap(dict->capacity);\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < dict->size; i++) {\n")
		gen.funcDecls.WriteString("        void* value = hashMapGet(dict, keys[i]);\n")
		gen.funcDecls.WriteString("        hashMapPut(sorted, keys[i], value);\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    \n")
		gen.funcDecls.WriteString("    free(keys);\n")
		gen.funcDecls.WriteString("    return sorted;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// stable_sort method (same as sort for dictionaries)
	if gen.dictMethods["stable_sort"] {
		gen.funcDecls.WriteString("HashMap* ahoy_dict_stable_sort(HashMap* dict) {\n")
		gen.funcDecls.WriteString("    return ahoy_dict_sort(dict);\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// merge method
	if gen.dictMethods["merge"] {
		gen.funcDecls.WriteString("HashMap* ahoy_dict_merge(HashMap* dict1, HashMap* dict2) {\n")
		gen.funcDecls.WriteString("    if (dict1 == NULL) return dict2;\n")
		gen.funcDecls.WriteString("    if (dict2 == NULL) return dict1;\n")
		gen.funcDecls.WriteString("    \n")
		gen.funcDecls.WriteString("    HashMap* merged = createHashMap(dict1->capacity + dict2->capacity);\n")
		gen.funcDecls.WriteString("    \n")
		gen.funcDecls.WriteString("    // Copy all from dict1\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < dict1->capacity; i++) {\n")
		gen.funcDecls.WriteString("        HashMapEntry* entry = dict1->buckets[i];\n")
		gen.funcDecls.WriteString("        while (entry != NULL) {\n")
		gen.funcDecls.WriteString("            hashMapPut(merged, entry->key, entry->value);\n")
		gen.funcDecls.WriteString("            entry = entry->next;\n")
		gen.funcDecls.WriteString("        }\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    \n")
		gen.funcDecls.WriteString("    // Copy all from dict2 (overrides if keys exist)\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < dict2->capacity; i++) {\n")
		gen.funcDecls.WriteString("        HashMapEntry* entry = dict2->buckets[i];\n")
		gen.funcDecls.WriteString("        while (entry != NULL) {\n")
		gen.funcDecls.WriteString("            hashMapPut(merged, entry->key, entry->value);\n")
		gen.funcDecls.WriteString("            entry = entry->next;\n")
		gen.funcDecls.WriteString("        }\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    \n")
		gen.funcDecls.WriteString("    return merged;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// print_dict helper - formats dict for printing
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
	default:
		return "int"
	}
}

// Get AhoyValueType enum for a type string
func (gen *CodeGenerator) getAhoyTypeEnum(typeName string) string {
	switch typeName {
	case "int", "bool":
		return "AHOY_TYPE_INT"
	case "float":
		return "AHOY_TYPE_FLOAT"
	case "string":
		return "AHOY_TYPE_STRING"
	case "char":
		return "AHOY_TYPE_CHAR"
	default:
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
	// Generate Vector2 constructor
	gen.funcDecls.WriteString("\n// Vector2 constructor\n")
	gen.funcDecls.WriteString("Vector2 vector2(float x, float y) {\n")
	gen.funcDecls.WriteString("    Vector2 v = {x, y};\n")
	gen.funcDecls.WriteString("    return v;\n")
	gen.funcDecls.WriteString("}\n\n")
	
	// Generate Color constructor
	gen.funcDecls.WriteString("// Color constructor\n")
	gen.funcDecls.WriteString("Color color(unsigned char r, unsigned char g, unsigned char b, unsigned char a) {\n")
	gen.funcDecls.WriteString("    Color c = {r, g, b, a};\n")
	gen.funcDecls.WriteString("    return c;\n")
	gen.funcDecls.WriteString("}\n\n")
}

func (gen *CodeGenerator) writeStructHelperFunctions() {
	// Generate print helper for each struct type
	// Track which structs we've processed to avoid duplicates (since we store both lowercase and capitalized)
	processed := make(map[string]bool)
	
	// First pass: Add forward declarations
	for _, structInfo := range gen.structs {
		if processed[structInfo.Name] {
			continue
		}
		// Skip JSON structs - they don't have C representations
		if gen.jsonStructs[structInfo.Name] {
			continue
		}
		processed[structInfo.Name] = true
		cStructName := capitalizeFirst(structInfo.Name)
		gen.funcForwardDecls.WriteString(fmt.Sprintf("char* print_struct_helper_%s(%s obj);\n", structInfo.Name, cStructName))
	}
	
	// Second pass: Add implementations
	processed = make(map[string]bool)
	for _, structInfo := range gen.structs {
		// Skip if already processed (avoid duplicates from lowercase/capitalized pairs)
		if processed[structInfo.Name] {
			continue
		}
		// Skip JSON structs - they don't have C representations
		if gen.jsonStructs[structInfo.Name] {
			continue
		}
		processed[structInfo.Name] = true
		
		cStructName := capitalizeFirst(structInfo.Name)
		gen.funcDecls.WriteString(fmt.Sprintf("\n// Print helper for %s\n", structInfo.Name))
		gen.funcDecls.WriteString(fmt.Sprintf("char* print_struct_helper_%s(%s obj) {\n", structInfo.Name, cStructName))
		gen.funcDecls.WriteString("    static char buffer[512];\n")
		
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
			case "Vector2":
				gen.funcDecls.WriteString("%s")  // Will use vector2_to_string
			case "Color":
				gen.funcDecls.WriteString("%s")  // Will use color_to_string
			case "AhoyArray*":
				gen.funcDecls.WriteString("[]")  // Show as empty array
			case "HashMap*":
				gen.funcDecls.WriteString("<>")  // Show as empty dict
			default:
				gen.funcDecls.WriteString("%p")
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
			
			if field.Type == "bool" {
				gen.funcDecls.WriteString(fmt.Sprintf("obj.%s ? \"true\" : \"false\"", field.Name))
			} else if field.Type == "Vector2" {
				gen.funcDecls.WriteString(fmt.Sprintf("vector2_to_string(obj.%s)", field.Name))
			} else if field.Type == "Color" {
				gen.funcDecls.WriteString(fmt.Sprintf("color_to_string(obj.%s)", field.Name))
			} else if field.Type == "char*" || field.Type == "const char*" {
				gen.funcDecls.WriteString(fmt.Sprintf("(obj.%s ? obj.%s : \"\")", field.Name, field.Name))
			} else {
				gen.funcDecls.WriteString(fmt.Sprintf("obj.%s", field.Name))
			}
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
		// Typed object literal - capitalize first letter for C struct name
		structName = capitalizeFirst(node.Value)
		
		// Check if this is a known Ahoy struct type
		_, hasStructInfo := gen.structs[node.Value]
		if !hasStructInfo {
			_, hasStructInfo = gen.structs[structName]
		}
		
		// For typed object literals, generate C struct initialization even if we don't have
		// the full struct definition (e.g., C structs from imported headers like Vector2)
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
					gen.generateNodeInternal(prop.Children[0], false)
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
				gen.generateNodeInternal(prop.Children[0], false)
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
	
	// If object is dict, HashMap*, generic, or intptr_t, use hashMapGet
	if objectType == "dict" || objectType == "HashMap*" || objectType == "generic" || objectType == "intptr_t" ||
	   strings.HasPrefix(objectType, "dict[") || strings.HasPrefix(objectType, "dict<") {
		gen.output.WriteString(fmt.Sprintf("((char*)hashMapGet("))
		// Cast generic/intptr_t to HashMap*
		if objectType == "generic" || objectType == "intptr_t" {
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
// Check all registered enums
var foundEnum string
foundCount := 0

for enumName, members := range gen.enums {
if members[memberName] {
foundEnum = enumName
foundCount++
}
}

// Only resolve if found in exactly one enum (unambiguous)
if foundCount == 1 {
// Check if this is an int enum
if gen.enumTypes[foundEnum] == "int" {
return foundEnum + "_" + memberName
}
}

return ""
}
