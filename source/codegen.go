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
	Name string
	Type string
}

type StructInfo struct {
	Name   string
	Fields []StructField
}

type CodeGenerator struct {
	output                        strings.Builder
	indent                        int
	varCounter                    int
	funcDecls                     strings.Builder
	includes                      map[string]bool
	variables                     map[string]string          // variable name -> type (global scope)
	functionVars                  map[string]string          // variable name -> type (function scope)
	constants                     map[string]bool            // constant name -> declared
	enums                         map[string]map[string]bool // enum name -> {member names}
	userFunctions                 map[string]bool            // user-defined function names (keep snake_case)
	hasError                      bool                       // Track if error occurred
	arrayImpls                    bool                       // Track if we've added array implementation
	arrayMethods                  map[string]bool            // Track which array methods are used
	stringMethods                 map[string]bool            // Track which string methods are used
	dictMethods                   map[string]bool            // Track which dict methods are used
	loopCounters                  []string                   // Stack of loop counter variable names
	currentFunction               string                     // Current function being generated
	currentFunctionReturnType     string                     // Return type of current function
	currentFunctionHasMultiReturn bool                       // Whether current function has multiple returns
	hasMainFunc                   bool                       // Whether there's an Ahoy main function
	arrayElementTypes             map[string]string          // array variable name -> element type
	structs                       map[string]*StructInfo     // struct name -> struct info
	currentTypeContext            string                     // Current type annotation context (e.g., "array[int]")
}

// GenerateC generates C code from an AST (exported for testing)
func GenerateC(ast *ahoy.ASTNode) string {
	return generateC(ast)
}

func generateC(ast *ahoy.ASTNode) string {
	gen := &CodeGenerator{
		includes:          make(map[string]bool),
		variables:         make(map[string]string),
		constants:         make(map[string]bool),
		enums:             make(map[string]map[string]bool),
		userFunctions:     make(map[string]bool),
		hasError:          false,
		arrayImpls:        false,
		arrayMethods:      make(map[string]bool),
		stringMethods:     make(map[string]bool),
		dictMethods:       make(map[string]bool),
		hasMainFunc:       false,
		arrayElementTypes: make(map[string]string),
		structs:           make(map[string]*StructInfo),
	}

	// Add standard includes
	gen.includes["stdio.h"] = true
	gen.includes["stdlib.h"] = true
	gen.includes["string.h"] = true
	gen.includes["stdbool.h"] = true
	gen.includes["stdint.h"] = true

	// Generate hash map implementation
	gen.writeHashMapImplementation()

	// First pass: check if there's a main function
	gen.checkForMainFunction(ast)

	// Generate main code
	gen.generateNode(ast)

	// Check if there were any errors
	if gen.hasError {
		return "" // Return empty string to indicate error
	}

	// Generate type helper function if needed
	gen.writeTypeEnumToStringHelper()

	// Generate array helper functions if any array methods were used
	gen.writeArrayHelperFunctions()

	// Generate dict helper functions if any dict methods were used
	gen.writeDictHelperFunctions()

	// Generate string helper functions if any string methods were used
	gen.writeStringHelperFunctions()

	// Generate struct print helper functions
	gen.writeStructHelperFunctions()

	// Build final output
	var result strings.Builder

	// Write includes
	for include := range gen.includes {
		// Use angle brackets for system includes, quotes for local .h files
		if strings.HasSuffix(include, ".h") && (strings.HasPrefix(include, "/") || strings.HasPrefix(include, ".")) {
			result.WriteString(fmt.Sprintf("#include \"%s\"\n", include))
		} else {
			result.WriteString(fmt.Sprintf("#include <%s>\n", include))
		}
	}
	result.WriteString("\n")

	// Write array implementation if needed
	if gen.arrayImpls {
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
		result.WriteString("char* print_array_helper(AhoyArray* arr);\n")
		result.WriteString("\n")
	}

	// Write function declarations
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
			// Check if it's a known constant/macro from raylib or other C libraries
			// These will be passed through directly to C
			// Don't convert variable names, only function names are converted
			gen.output.WriteString(node.Value)
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
		if strings.Contains(node.DataType, ",") {
			// Multiple return types - create a struct
			parts := strings.Split(node.DataType, ",")
			for _, part := range parts {
				returnTypes = append(returnTypes, strings.TrimSpace(part))
			}

			// Generate struct definition for multi-return
			structName := fmt.Sprintf("%s_return", funcName)
			gen.funcDecls.WriteString(fmt.Sprintf("typedef struct {\n"))
			for i, rType := range returnTypes {
				mappedType := gen.mapType(rType)
				gen.funcDecls.WriteString(fmt.Sprintf("    %s ret%d;\n", mappedType, i))
			}
			gen.funcDecls.WriteString(fmt.Sprintf("} %s;\n\n", structName))
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
		paramType := "int" // default
		if param.DataType != "" {
			paramType = gen.mapType(param.DataType)
		}
		paramList += fmt.Sprintf("%s %s", paramType, param.Value)
	}

	gen.funcDecls.WriteString(fmt.Sprintf("%s %s(%s);\n", returnType, cFuncName, paramList))
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

	// Initialize function-local variable scope
	gen.functionVars = make(map[string]string)

	gen.generateNodeInternal(body, false)

	gen.funcDecls.WriteString(gen.output.String())
	gen.funcDecls.WriteString("}\n\n")

	gen.indent--
	gen.output = oldOutput
	gen.currentFunction = ""
	gen.currentFunctionReturnType = ""
	gen.currentFunctionHasMultiReturn = false
	gen.functionVars = nil // Clear function scope
}

func (gen *CodeGenerator) generateAssignment(node *ahoy.ASTNode) {
	gen.writeIndent()

	// Check if this is a property/element assignment (obj<'prop'>: value or dict{"key"}: value)
	// In this case, Children[0] is the access node, Children[1] is the value
	if len(node.Children) == 2 &&
		(node.Children[0].Type == ahoy.NODE_OBJECT_ACCESS ||
			node.Children[0].Type == ahoy.NODE_DICT_ACCESS ||
			node.Children[0].Type == ahoy.NODE_ARRAY_ACCESS) {

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

		// For object/array access, direct assignment works
		gen.generateNode(node.Children[0])
		gen.output.WriteString(" = ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(";\n")
		return
	}

	// Check if variable already exists (function scope first, then global)
	_, existsInFunc := gen.functionVars[node.Value]
	_, existsGlobal := gen.variables[node.Value]

	if existsInFunc || existsGlobal {
		// Just assignment
		valueNode := node.Children[0]
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

	// Iterate through hash map buckets
	gen.output.WriteString(fmt.Sprintf("for (int %s = 0; %s < %s->capacity; %s++) {\n",
		bucketVar, bucketVar, dictName, bucketVar))

	gen.indent++
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("HashMapEntry* %s = %s->buckets[%s];\n",
		entryVar, dictName, bucketVar))

	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("while (%s != NULL) {\n", entryVar))

	gen.indent++
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("const char* %s = %s->key;\n", keyVar, entryVar))
	
	// Generate a helper variable to convert value based on type
	valueHelperVar := fmt.Sprintf("__value_str_%d", gen.varCounter)
	gen.varCounter++
	
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("char %s[64];\n", valueHelperVar))
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("switch (%s->valueType) {\n", entryVar))
	
	gen.indent++
	gen.writeIndent()
	gen.output.WriteString("case AHOY_TYPE_INT:\n")
	gen.indent++
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("sprintf(%s, \"%%d\", (int)(intptr_t)%s->value);\n", valueHelperVar, entryVar))
	gen.writeIndent()
	gen.output.WriteString("break;\n")
	gen.indent--
	
	gen.writeIndent()
	gen.output.WriteString("case AHOY_TYPE_STRING:\n")
	gen.indent++
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("strcpy(%s, (const char*)(intptr_t)%s->value);\n", valueHelperVar, entryVar))
	gen.writeIndent()
	gen.output.WriteString("break;\n")
	gen.indent--
	
	gen.writeIndent()
	gen.output.WriteString("case AHOY_TYPE_FLOAT:\n")
	gen.indent++
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("sprintf(%s, \"%%f\", *(float*)(intptr_t)%s->value);\n", valueHelperVar, entryVar))
	gen.writeIndent()
	gen.output.WriteString("break;\n")
	gen.indent--
	
	gen.writeIndent()
	gen.output.WriteString("case AHOY_TYPE_CHAR:\n")
	gen.indent++
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("sprintf(%s, \"%%c\", (char)(intptr_t)%s->value);\n", valueHelperVar, entryVar))
	gen.writeIndent()
	gen.output.WriteString("break;\n")
	gen.indent--
	gen.indent--
	
	gen.writeIndent()
	gen.output.WriteString("}\n")
	
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("const char* %s = %s;\n", valueVar, valueHelperVar))

	// Register loop variables for type inference
	oldKeyType := gen.variables[keyVar]
	oldValType := gen.variables[valueVar]
	gen.variables[keyVar] = "char*"
	gen.variables[valueVar] = "char*"

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
			for i, child := range node.Children {
				if i > 0 {
					gen.output.WriteString(", ")
				}
				gen.output.WriteString(fmt.Sprintf(".ret%d = ", i))
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
	// Defer is complex in C - we need to use cleanup attribute or manually track
	// For simplicity, we'll just add a comment and execute the statement immediately
	// A proper implementation would require tracking deferred statements and executing them
	// before each return statement and at function end
	gen.writeIndent()
	gen.output.WriteString("/* DEFER: Execute before function return */ ")
	if len(node.Children) > 0 {
		// Generate the deferred statement
		// Remove the indent since we already added it
		savedIndent := gen.indent
		gen.indent = 0
		gen.generateNodeInternal(node.Children[0], false)
		gen.indent = savedIndent
	}
}

func (gen *CodeGenerator) generateImportStatement(node *ahoy.ASTNode) {
	// Add include - check if it's a local or system include
	headerName := node.Value
	gen.includes[headerName] = true
}

func (gen *CodeGenerator) generateCall(node *ahoy.ASTNode) {
	// Keep user-defined functions as snake_case
	// Convert C library functions to PascalCase
	funcName := node.Value

	// Special case: rename main to ahoy_main
	if funcName == "main" {
		funcName = "ahoy_main"
	} else if gen.userFunctions[funcName] {
		// Keep user-defined function names as-is (snake_case)
		funcName = node.Value
	} else if strings.Contains(funcName, "_") {
		// External C library function - convert to PascalCase
		funcName = snakeToPascal(funcName)
	}

	// Handle special functions
	switch node.Value {
	case "print":
		gen.output.WriteString("printf(")

		// Check if we have multiple arguments or if first arg is a format string
		hasMultipleArgs := len(node.Children) > 1
		firstIsString := len(node.Children) > 0 && node.Children[0].Type == ahoy.NODE_STRING

		// If first argument is a string AND it looks like a format string (has {} or %), treat it as one
		if firstIsString && !hasMultipleArgs {
			// Single string argument - just print it
			formatStr := node.Children[0].Value
			if !strings.HasSuffix(formatStr, "\\n") {
				formatStr += "\\n"
			}
			gen.output.WriteString(fmt.Sprintf("\"%s\"", formatStr))
		} else if firstIsString && (strings.Contains(node.Children[0].Value, "{}") || strings.Contains(node.Children[0].Value, "%")) {
			// First arg is a format string with placeholders
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
		} else {
			// Multiple arguments without format string, infer format from argument types
			// Build a single format string with spaces between arguments (Python-style)
			if len(node.Children) > 0 {
				formatParts := []string{}

				// Build format string with spaces between arguments
				for _, arg := range node.Children {
					argType := gen.inferType(arg)
					formatSpec := ""
					switch argType {
					case "string", "char*":
						formatSpec = "%s"
					case "int":
						formatSpec = "%d"
					case "float", "double":
						formatSpec = "%f"
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
					default:
						// Check for typed collections
						if strings.HasPrefix(argType, "array[") {
							formatSpec = "%s" // Will use print_array_helper
						} else if strings.HasPrefix(argType, "dict[") {
							formatSpec = "%s" // Will use print_dict_helper
						} else if _, isStruct := gen.structs[argType]; isStruct {
							formatSpec = "%s" // Will use print_struct_helper
						} else {
							formatSpec = "%d"
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
					} else if argType == "struct" || gen.structs[argType] != nil {
						// Struct type - use print helper
						gen.arrayMethods["print_struct"] = true
						gen.output.WriteString("print_struct_helper_")
						gen.output.WriteString(argType)
						gen.output.WriteString("(")
						gen.generateNode(arg)
						gen.output.WriteString(")")
					} else {
						gen.generateNode(arg)
					}
				}
			}
			gen.output.WriteString(")")
			return
		}
		gen.output.WriteString(")")

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

	default:
		gen.output.WriteString(fmt.Sprintf("%s(", funcName))
		for i, arg := range node.Children {
			if i > 0 {
				gen.output.WriteString(", ")
			}
			gen.generateNode(arg)
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

	gen.writeIndent()
	constType := gen.mapType(node.DataType)
	gen.output.WriteString(fmt.Sprintf("const %s %s = ", constType, constName))
	gen.generateNode(node.Children[0])
	gen.output.WriteString(";\n")
}

func (gen *CodeGenerator) generateMethodCall(node *ahoy.ASTNode) {
	object := node.Children[0]
	args := node.Children[1]
	methodName := node.Value

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
		gen.generateNodeInternal(object, false)

		if len(args.Children) > 0 {
			gen.output.WriteString(", ")
			for i, arg := range args.Children {
				if i > 0 {
					gen.output.WriteString(", ")
				}
				// For array methods like push, cast to intptr_t
				if methodName == "push" || methodName == "has" {
					gen.output.WriteString("(intptr_t)")
				}
				gen.generateNodeInternal(arg, false)
				// For push, also pass the type
				if methodName == "push" && i == 0 {
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

	// Determine if this is a mixed-type array or homogeneous
	isMixed := false
	var firstType string
	if explicitElementType != "" {
		// Use explicit type from annotation
		firstType = explicitElementType
		isMixed = false
	} else if len(node.Children) > 0 {
		firstType = gen.getValueType(node.Children[0])
		for _, child := range node.Children[1:] {
			if gen.getValueType(child) != firstType {
				isMixed = true
				break
			}
		}
	}

	// Use simple C array initialization
	gen.output.WriteString("({ ")
	gen.output.WriteString(fmt.Sprintf("AhoyArray* %s = malloc(sizeof(AhoyArray)); ", arrName))
	gen.output.WriteString(fmt.Sprintf("%s->length = %d; ", arrName, len(node.Children)))
	gen.output.WriteString(fmt.Sprintf("%s->capacity = %d; ", arrName, len(node.Children)))
	gen.output.WriteString(fmt.Sprintf("%s->data = malloc(%d * sizeof(intptr_t)); ", arrName, len(node.Children)))
	gen.output.WriteString(fmt.Sprintf("%s->types = malloc(%d * sizeof(AhoyValueType)); ", arrName, len(node.Children)))
	
	// Set typed/mixed flag
	if isMixed {
		gen.output.WriteString(fmt.Sprintf("%s->is_typed = 0; ", arrName))
	} else {
		gen.output.WriteString(fmt.Sprintf("%s->is_typed = 1; ", arrName))
		gen.output.WriteString(fmt.Sprintf("%s->element_type = %s; ", arrName, gen.getAhoyTypeEnum(firstType)))
	}

	// Add elements - cast to intptr_t for pointer safety and track types
	for i, child := range node.Children {
		valueType := gen.getValueType(child)
		gen.output.WriteString(fmt.Sprintf("%s->types[%d] = %s; ", arrName, i, gen.getAhoyTypeEnum(valueType)))
		gen.output.WriteString(fmt.Sprintf("%s->data[%d] = (intptr_t)", arrName, i))
		gen.generateNode(child)
		gen.output.WriteString("; ")
	}

	gen.output.WriteString(fmt.Sprintf("%s; })", arrName))
}

func (gen *CodeGenerator) generateArrayAccess(node *ahoy.ASTNode) {
	arrayName := node.Value

	// Check if we know the element type
	if elemType, exists := gen.arrayElementTypes[arrayName]; exists {
		cType := gen.mapType(elemType)
		// Cast to the appropriate type for non-int types (need intptr_t intermediate for pointer safety)
		if cType != "int" {
			gen.output.WriteString(fmt.Sprintf("((%s)(intptr_t)%s->data[", cType, arrayName))
			gen.generateNode(node.Children[0])
			gen.output.WriteString("])")
			return
		}
	}

	// Default: no cast needed for int
	gen.output.WriteString(fmt.Sprintf("%s->data[", arrayName))
	gen.generateNode(node.Children[0])
	gen.output.WriteString("]")
}

func (gen *CodeGenerator) generateDictAccess(node *ahoy.ASTNode) {
	// Cast to char* for string values (common case)
	// TODO: Track dict value types like we do for arrays
	gen.output.WriteString(fmt.Sprintf("((char*)hashMapGet(%s, ", node.Value))
	gen.generateNode(node.Children[0])
	gen.output.WriteString("))")
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

		gen.output.WriteString(", (void*)(intptr_t)")
		gen.generateNode(value)
		gen.output.WriteString(fmt.Sprintf(", %s); ", ahoyTypeEnum))
	}

	gen.output.WriteString(fmt.Sprintf("%s; })", dictName))
}

func (gen *CodeGenerator) mapType(langType string) string {
	// Check for typed collections first
	if strings.HasPrefix(langType, "array[") {
		return "AhoyArray*"
	}
	if strings.HasPrefix(langType, "dict[") {
		return "HashMap*"
	}
	
	switch langType {
	case "int":
		return "int"
	case "float":
		return "double"
	case "string", "char*":
		return "char*"
	case "bool":
		return "bool"
	case "dict":
		return "HashMap*"
	case "array":
		return "AhoyArray*"
	case "void":
		return "void"
	default:
		return "int"
	}
}

func (gen *CodeGenerator) inferType(node *ahoy.ASTNode) string {
	switch node.Type {
	case ahoy.NODE_TYPE_PROPERTY:
		return "char*" // .type property returns a string
	case ahoy.NODE_NUMBER:
		if strings.Contains(node.Value, ".") {
			return "float"
		}
		return "int"
	case ahoy.NODE_STRING:
		return "char*"
	case ahoy.NODE_F_STRING:
		return "char*"
	case ahoy.NODE_BOOLEAN:
		return "bool"
	case ahoy.NODE_DICT_LITERAL:
		return "dict"
	case ahoy.NODE_ARRAY_LITERAL:
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
		return "int"
	case ahoy.NODE_METHOD_CALL:
		// Check the object type to determine if it's a dict or array method
		objectType := ""
		if len(node.Children) > 0 {
			objectType = gen.inferType(node.Children[0])
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
			node.Value == "shuffle" || node.Value == "push" {
			return "array"
		}
		// Array methods that return int
		if node.Value == "sum" || node.Value == "pop" ||
			node.Value == "pick" || node.Value == "has" {
			return "int"
		}
		return "int"
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
		if varType, exists := gen.variables[node.Value]; exists {
			return varType
		}
		if varType, exists := gen.functionVars[node.Value]; exists {
			return varType
		}
		return "int"
	case ahoy.NODE_ARRAY_ACCESS:
		// Get the array variable name and look up its element type
		arrayName := node.Value
		if elemType, exists := gen.arrayElementTypes[arrayName]; exists {
			return elemType
		}
		// Default to int if we don't know the element type
		return "int"
	case ahoy.NODE_DICT_ACCESS:
		// Dictionary values are typically strings for now
		// TODO: Track dict value types
		return "char*"
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

			// Get the type of the object
			objectType := gen.inferType(objectNode)

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
				if varType == "string" || varType == "char*" {
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
			gen.output.WriteString(v)
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

	// Track enum members for validation
	if gen.enums[enumName] == nil {
		gen.enums[enumName] = make(map[string]bool)
	}

	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("typedef enum {\n"))
	gen.indent++

	nextAutoValue := 0
	for _, member := range node.Children {
		gen.writeIndent()

		// Track this member
		gen.enums[enumName][member.Value] = true

		// Check if member has a custom value (stored in DataType field)
		if member.DataType != "" {
			// Use the custom value
			gen.output.WriteString(fmt.Sprintf("%s_%s = %s,\n", enumName, member.Value, member.DataType))
			// Parse the value to set nextAutoValue for next member
			if val, err := strconv.Atoi(member.DataType); err == nil {
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
	gen.output.WriteString(fmt.Sprintf("} %s;\n\n", enumName))
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
		gen.output.WriteString(fmt.Sprintf("%s_return %s = ", funcName, tempVar))
		gen.generateNode(callNode)
		gen.output.WriteString(";\n")

		// Assign struct fields to left side variables
		for i, target := range leftSide.Children {
			gen.writeIndent()
			// Check if variable needs to be declared
			if _, exists := gen.variables[target.Value]; !exists {
				// Need to declare variable - infer type from function return
				// For now, use int as default (should lookup function signature)
				cType := "int"
				gen.output.WriteString(fmt.Sprintf("%s ", cType))
				gen.variables[target.Value] = "int"
			}
			gen.output.WriteString(fmt.Sprintf("%s = %s.ret%d;\n", target.Value, tempVar, i))
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
			if _, exists := gen.variables[target.Value]; !exists {
				// Need to declare variable - infer type from temp
				tempType := gen.inferType(rightSide.Children[i])
				cType := gen.mapType(tempType)
				gen.output.WriteString(fmt.Sprintf("%s ", cType))
				gen.variables[target.Value] = tempType
			}
			gen.output.WriteString(fmt.Sprintf("%s = %s;\n", target.Value, temps[i]))
		}
	}
}

// Generate struct declaration
func (gen *CodeGenerator) generateStruct(node *ahoy.ASTNode) {
	structName := node.Value

	// Track struct info
	structInfo := &StructInfo{
		Name:   structName,
		Fields: make([]StructField, 0),
	}

	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("typedef struct {\n"))
	gen.indent++

	for _, field := range node.Children {
		fieldType := gen.mapType(field.DataType)
		gen.writeIndent()
		gen.output.WriteString(fmt.Sprintf("%s %s;\n", fieldType, field.Value))

		// Track field info
		structInfo.Fields = append(structInfo.Fields, StructField{
			Name: field.Value,
			Type: fieldType,
		})
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("} %s;\n\n", structName))

	// Store struct info
	gen.structs[structName] = structInfo
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
			// Generate as: enum_name_MEMBER
			gen.output.WriteString(object.Value)
			gen.output.WriteString("_")
			gen.output.WriteString(memberName)
			return
		}
	}

	gen.generateNodeInternal(object, false)

	// Check if object is a pointer type (array, dict, or struct pointer)
	objectType := gen.inferType(object)
	if objectType == "AhoyArray*" || objectType == "HashMap*" || objectType == "array" || objectType == "dict" ||
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
		// Other types
		gen.output.WriteString(fmt.Sprintf("strcpy(__type_str, \"%s\"); ", varType))
	}
	
	gen.output.WriteString("__type_str; ")
	gen.output.WriteString("})")
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
		gen.funcDecls.WriteString("            // Try to print value as string if it looks like a pointer to string\n")
		gen.funcDecls.WriteString("            if (entry->value != NULL) {\n")
		gen.funcDecls.WriteString("                char* str_val = (char*)entry->value;\n")
		gen.funcDecls.WriteString("                // Simple heuristic: if it's a readable string, print as string\n")
		gen.funcDecls.WriteString("                offset += sprintf(buffer + offset, \"\\\"%s\\\"\", str_val);\n")
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
	}
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
	paramName := lambda.Value
	bodyExpr := lambda.Children[0]

	// Generate inline statement expression
	gen.output.WriteString("({ ")
	gen.output.WriteString("AhoyArray* __src = ")
	gen.generateNodeInternal(arrayNode, false)
	gen.output.WriteString("; ")
	gen.output.WriteString("AhoyArray* __result = malloc(sizeof(AhoyArray)); ")
	gen.output.WriteString("__result->length = __src->length; ")
	gen.output.WriteString("__result->capacity = __src->length; ")
	gen.output.WriteString("__result->data = malloc(__src->length * sizeof(int)); ")
	gen.output.WriteString("for (int __i = 0; __i < __src->length; __i++) { ")
	gen.output.WriteString(fmt.Sprintf("int %s = __src->data[__i]; ", paramName))
	gen.output.WriteString("__result->data[__i] = (")

	// Generate lambda body expression
	gen.generateNodeInternal(bodyExpr, false)

	gen.output.WriteString("); } ")
	gen.output.WriteString("__result; })")
}

// Generate inline filter code
func (gen *CodeGenerator) generateFilterInline(arrayNode *ahoy.ASTNode, lambda *ahoy.ASTNode) {
	paramName := lambda.Value
	condExpr := lambda.Children[0]

	// Generate inline statement expression
	gen.output.WriteString("({ ")
	gen.output.WriteString("AhoyArray* __src = ")
	gen.generateNodeInternal(arrayNode, false)
	gen.output.WriteString("; ")
	gen.output.WriteString("AhoyArray* __result = malloc(sizeof(AhoyArray)); ")
	gen.output.WriteString("__result->capacity = __src->length; ")
	gen.output.WriteString("__result->data = malloc(__src->length * sizeof(int)); ")
	gen.output.WriteString("__result->length = 0; ")
	gen.output.WriteString("for (int __i = 0; __i < __src->length; __i++) { ")
	gen.output.WriteString(fmt.Sprintf("int %s = __src->data[__i]; ", paramName))
	gen.output.WriteString("if (")

	// Generate lambda condition expression
	gen.generateNodeInternal(condExpr, false)

	gen.output.WriteString(") { ")
	gen.output.WriteString(fmt.Sprintf("__result->data[__result->length++] = %s; ", paramName))
	gen.output.WriteString("} } ")
	gen.output.WriteString("__result; })")
}

func (gen *CodeGenerator) writeStructHelperFunctions() {
	// Generate print helper for each struct type
	for _, structInfo := range gen.structs {
		gen.funcDecls.WriteString(fmt.Sprintf("\n// Print helper for %s\n", structInfo.Name))
		gen.funcDecls.WriteString(fmt.Sprintf("char* print_struct_helper_%s(%s obj) {\n", structInfo.Name, structInfo.Name))
		gen.funcDecls.WriteString("    static char buffer[512];\n")
		gen.funcDecls.WriteString("    sprintf(buffer, \"{")

		for i, field := range structInfo.Fields {
			if i > 0 {
				gen.funcDecls.WriteString(", ")
			}
			gen.funcDecls.WriteString(field.Name)
			gen.funcDecls.WriteString(": ")

			// Add format specifier based on field type
			switch field.Type {
			case "int":
				gen.funcDecls.WriteString("%d")
			case "float", "double":
				gen.funcDecls.WriteString("%.2f")
			case "char*", "const char*":
				gen.funcDecls.WriteString("\\\"%s\\\"")
			case "char":
				gen.funcDecls.WriteString("%c")
			case "bool":
				gen.funcDecls.WriteString("%s")
			default:
				gen.funcDecls.WriteString("%p")
			}
		}

		gen.funcDecls.WriteString("}\", ")

		// Add field values
		for i, field := range structInfo.Fields {
			if i > 0 {
				gen.funcDecls.WriteString(", ")
			}
			if field.Type == "bool" {
				gen.funcDecls.WriteString(fmt.Sprintf("obj.%s ? \"true\" : \"false\"", field.Name))
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
	// If node.Value is set, it's a typed literal (e.g., rectangle<...>)
	// Need to generate: (Rectangle){...}

	if node.Value != "" {
		// Typed object literal - capitalize first letter for C struct name
		structName := capitalizeFirst(node.Value)
		gen.output.WriteString(fmt.Sprintf("(%s)", structName))
	}

	gen.output.WriteString("{")

	// Generate field initializations
	first := true
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

	gen.output.WriteString("}")
}

// capitalizeFirst capitalizes the first letter of a string
func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (gen *CodeGenerator) generateObjectAccess(node *ahoy.ASTNode) {
	// Object property access: person<'name'> becomes person.name
	// The property name is in node.Children[0] as a string
	// Note: The tokenizer already strips quotes from strings
	gen.output.WriteString(node.Value)
	gen.output.WriteString(".")

	if len(node.Children) > 0 && node.Children[0].Type == ahoy.NODE_STRING {
		// Use the property name directly - quotes are already stripped by tokenizer
		gen.output.WriteString(node.Children[0].Value)
	}
}
