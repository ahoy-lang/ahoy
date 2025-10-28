package main

import (
	"fmt"
	"strings"
)

func snakeToPascal(s string) string {
	// If there are no underscores, return as-is (it's already in the correct format)
	if !strings.Contains(s, "_") {
		return s
	}

	parts := strings.Split(s, "_")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(string(part[0])) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

type CodeGenerator struct {
	output       strings.Builder
	indent       int
	varCounter   int
	funcDecls    strings.Builder
	includes     map[string]bool
	variables    map[string]string // variable name -> type
	arrayImpls   bool              // Track if we've added array implementation
	arrayMethods map[string]bool   // Track which array methods are used
	loopCounters []string          // Stack of loop counter variable names
}

func generateC(ast *ASTNode) string {
	gen := &CodeGenerator{
		includes:     make(map[string]bool),
		variables:    make(map[string]string),
		arrayImpls:   false,
		arrayMethods: make(map[string]bool),
	}

	// Add standard includes
	gen.includes["stdio.h"] = true
	gen.includes["stdlib.h"] = true
	gen.includes["string.h"] = true
	gen.includes["stdbool.h"] = true
	gen.includes["stdint.h"] = true

	// Generate hash map implementation
	gen.writeHashMapImplementation()

	// Generate main code
	gen.generateNode(ast)

	// Generate array helper functions if any array methods were used
	gen.writeArrayHelperFunctions()

	// Build final output
	var result strings.Builder

	// Write includes
	for include := range gen.includes {
		result.WriteString(fmt.Sprintf("#include <%s>\n", include))
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

	// Write function declarations
	result.WriteString(gen.funcDecls.String())
	result.WriteString("\n")

	// Write main program
	result.WriteString("int main() {\n")
	result.WriteString(gen.output.String())
	result.WriteString("    return 0;\n")
	result.WriteString("}\n")

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
// Hash Map Implementation
typedef struct HashMapEntry {
    char* key;
    void* value;
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

void hashMapPut(HashMap* map, const char* key, void* value) {
    unsigned int index = hash(key) % map->capacity;
    HashMapEntry* entry = map->buckets[index];

    while (entry != NULL) {
        if (strcmp(entry->key, key) == 0) {
            entry->value = value;
            return;
        }
        entry = entry->next;
    }

    HashMapEntry* newEntry = malloc(sizeof(HashMapEntry));
    newEntry->key = strdup(key);
    newEntry->value = value;
    newEntry->next = map->buckets[index];
    map->buckets[index] = newEntry;
    map->size++;
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
	return `
// Forward declarations
typedef struct HashMapEntry HashMapEntry;
typedef struct HashMap HashMap;
HashMap* createHashMap(int capacity);
void hashMapPut(HashMap* map, const char* key, void* value);
void* hashMapGet(HashMap* map, const char* key);
void freeHashMap(HashMap* map);
`
}

func (gen *CodeGenerator) writeIndent() {
	for i := 0; i < gen.indent; i++ {
		gen.output.WriteString("    ")
	}
}

func (gen *CodeGenerator) generateNode(node *ASTNode) {
	gen.generateNodeInternal(node, false)
}

func (gen *CodeGenerator) generateNodeInternal(node *ASTNode, isStatement bool) {
	if node == nil {
		return
	}

	switch node.Type {
	case NODE_PROGRAM:
		for _, child := range node.Children {
			gen.generateNodeInternal(child, true)
		}

	case NODE_FUNCTION:
		gen.generateFunction(node)

	case NODE_ASSIGNMENT:
		gen.generateAssignment(node)

	case NODE_IF_STATEMENT:
		gen.generateIfStatement(node)

	case NODE_SWITCH_STATEMENT:
		gen.generateSwitchStatement(node)

	case NODE_WHILE_LOOP:
		gen.generateWhileLoop(node)

	case NODE_FOR_LOOP:
		gen.generateForLoop(node)

	case NODE_FOR_RANGE_LOOP:
		gen.generateForRangeLoop(node)

	case NODE_FOR_COUNT_LOOP:
		gen.generateForCountLoop(node)

	case NODE_FOR_IN_ARRAY_LOOP:
		gen.generateForInArrayLoop(node)

	case NODE_FOR_IN_DICT_LOOP:
		gen.generateForInDictLoop(node)

	case NODE_WHEN_STATEMENT:
		gen.generateWhenStatement(node)

	case NODE_RETURN_STATEMENT:
		gen.generateReturnStatement(node)

	case NODE_IMPORT_STATEMENT:
		gen.generateImportStatement(node)

	case NODE_CALL:
		if isStatement {
			gen.writeIndent()
		}
		gen.generateCall(node)
		if isStatement {
			gen.output.WriteString(";\n")
		}

	case NODE_BINARY_OP:
		gen.generateBinaryOp(node)

	case NODE_UNARY_OP:
		gen.generateUnaryOp(node)

	case NODE_IDENTIFIER:
		// Check if it's the loop counter variable
		if node.Value == "__loop_counter" && len(gen.loopCounters) > 0 {
			gen.output.WriteString(gen.loopCounters[len(gen.loopCounters)-1])
		} else {
			// Check if it's a known constant/macro from raylib or other C libraries
			// These will be passed through directly to C
			// Don't convert variable names, only function names are converted
			gen.output.WriteString(node.Value)
		}

	case NODE_NUMBER:
		gen.output.WriteString(node.Value)

	case NODE_STRING:
		gen.output.WriteString(fmt.Sprintf("\"%s\"", node.Value))

	case NODE_CHAR:
		gen.output.WriteString(fmt.Sprintf("'%s'", node.Value))

	case NODE_BOOLEAN:
		if node.Value == "true" {
			gen.output.WriteString("true")
		} else {
			gen.output.WriteString("false")
		}

	case NODE_DICT_LITERAL:
		gen.generateDictLiteral(node)

	case NODE_ARRAY_LITERAL:
		gen.generateArrayLiteral(node)

	case NODE_ARRAY_ACCESS:
		gen.generateArrayAccess(node)

	case NODE_DICT_ACCESS:
		gen.generateDictAccess(node)

	case NODE_BLOCK:
		for _, child := range node.Children {
			gen.generateNodeInternal(child, true)
		}
	case NODE_ENUM_DECLARATION:
		gen.generateEnum(node)
	case NODE_CONSTANT_DECLARATION:
		gen.generateConstant(node)
	case NODE_TUPLE_ASSIGNMENT:
		gen.generateTupleAssignment(node)
	case NODE_STRUCT_DECLARATION:
		gen.generateStruct(node)
	case NODE_METHOD_CALL:
		gen.generateMethodCall(node)
	case NODE_MEMBER_ACCESS:
		gen.generateMemberAccess(node)
	case NODE_BREAK:
		gen.writeIndent()
		gen.output.WriteString("break;\n")
	case NODE_SKIP:
		gen.writeIndent()
		gen.output.WriteString("continue;\n")
	}
}

func (gen *CodeGenerator) generateFunction(node *ASTNode) {
	returnType := "void"
	if node.DataType != "" {
		returnType = gen.mapType(node.DataType)
	}

	gen.funcDecls.WriteString(fmt.Sprintf("%s %s(", returnType, node.Value))

	// Parameters
	params := node.Children[0]
	for i, param := range params.Children {
		if i > 0 {
			gen.funcDecls.WriteString(", ")
		}
		paramType := "int" // default
		if param.DataType != "" {
			paramType = gen.mapType(param.DataType)
		}
		gen.funcDecls.WriteString(fmt.Sprintf("%s %s", paramType, param.Value))
	}

	gen.funcDecls.WriteString(") {\n")

	// Function body
	body := node.Children[1]
	oldOutput := gen.output
	gen.output = strings.Builder{}
	gen.indent++

	gen.generateNodeInternal(body, false)

	gen.funcDecls.WriteString(gen.output.String())
	gen.funcDecls.WriteString("}\n\n")

	gen.indent--
	gen.output = oldOutput
}

func (gen *CodeGenerator) generateAssignment(node *ASTNode) {
	gen.writeIndent()

	// Check if variable already exists
	if _, exists := gen.variables[node.Value]; exists {
		// Just assignment
		gen.output.WriteString(fmt.Sprintf("%s = ", node.Value))
		gen.generateNode(node.Children[0])
		gen.output.WriteString(";\n")
	} else {
		// Type inference and declaration
		valueNode := node.Children[0]
		varType := gen.inferType(valueNode)
		gen.variables[node.Value] = varType

		cType := gen.mapType(varType)
		gen.output.WriteString(fmt.Sprintf("%s %s = ", cType, node.Value))
		gen.generateNode(valueNode)
		gen.output.WriteString(";\n")
	}
}

func (gen *CodeGenerator) generateIfStatement(node *ASTNode) {
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

func (gen *CodeGenerator) generateSwitchStatement(node *ASTNode) {
	gen.writeIndent()
	gen.output.WriteString("switch (")
	gen.generateNode(node.Children[0]) // Generate switch expression
	gen.output.WriteString(") {\n")

	// Generate cases (skip first child which is the switch expression)
	for i := 1; i < len(node.Children); i++ {
		caseNode := node.Children[i]
		if caseNode.Type == NODE_SWITCH_CASE {
			caseValue := caseNode.Children[0]
			
			// Check if it's a list of cases or a range
			if caseValue.Type == NODE_SWITCH_CASE_LIST {
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
			} else if caseValue.Type == NODE_SWITCH_CASE_RANGE {
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
				// Single case value
				gen.indent++
				gen.writeIndent()
				gen.output.WriteString("case ")
				gen.generateNode(caseValue) // Case value
				gen.output.WriteString(":\n")

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

func (gen *CodeGenerator) generateWhenStatement(node *ASTNode) {
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("#ifdef %s\n", node.Value))

	gen.indent++
	gen.generateNodeInternal(node.Children[0], false)
	gen.indent--

	gen.writeIndent()
	gen.output.WriteString("#endif\n")
}

func (gen *CodeGenerator) generateWhileLoop(node *ASTNode) {
	gen.writeIndent()
	gen.output.WriteString("while (")
	gen.generateNode(node.Children[0])
	gen.output.WriteString(") {\n")

	gen.indent++
	gen.generateNodeInternal(node.Children[1], false)
	gen.indent--

	gen.writeIndent()
	gen.output.WriteString("}\n")
}

func (gen *CodeGenerator) generateForLoop(node *ASTNode) {
	gen.writeIndent()
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

func (gen *CodeGenerator) generateAssignmentForFor(node *ASTNode) {
	if node.Type == NODE_ASSIGNMENT {
		// Type inference
		valueNode := node.Children[0]
		varType := gen.inferType(valueNode)
		gen.variables[node.Value] = varType

		cType := gen.mapType(varType)
		gen.output.WriteString(fmt.Sprintf("%s %s = ", cType, node.Value))
		gen.generateNode(valueNode)
	}
}

func (gen *CodeGenerator) generateAssignmentForUpdate(node *ASTNode) {
	if node.Type == NODE_ASSIGNMENT {
		// Just assignment, no declaration
		gen.output.WriteString(fmt.Sprintf("%s = ", node.Value))
		gen.generateNode(node.Children[0])
	}
}

func (gen *CodeGenerator) generateForRangeLoop(node *ASTNode) {
	gen.writeIndent()
	gen.output.WriteString("for (")

	// Generate loop variable
	loopVar := fmt.Sprintf("__loop_i_%d", gen.varCounter)
	gen.varCounter++

	// Push loop counter onto stack
	gen.loopCounters = append(gen.loopCounters, loopVar)

	var startVal, endVal string
	if len(node.Children) == 3 {
		// Variable start: node.Children[0] is start expr, [1] is end expr, [2] is body
		startVal = gen.nodeToString(node.Children[0])
		endVal = gen.nodeToString(node.Children[1])

		gen.output.WriteString(fmt.Sprintf("int %s = %s; %s < %s; %s++) {\n",
			loopVar, startVal, loopVar, endVal, loopVar))

		gen.indent++
		gen.generateNodeInternal(node.Children[2], false)
		gen.indent--
	} else {
		// Constant start and end stored in Value and DataType fields
		startVal = node.Value
		endVal = node.DataType

		gen.output.WriteString(fmt.Sprintf("int %s = %s; %s < %s; %s++) {\n",
			loopVar, startVal, loopVar, endVal, loopVar))

		gen.indent++
		gen.generateNodeInternal(node.Children[0], false)
		gen.indent--
	}

	// Pop loop counter from stack
	gen.loopCounters = gen.loopCounters[:len(gen.loopCounters)-1]

	gen.writeIndent()
	gen.output.WriteString("}\n")
}

func (gen *CodeGenerator) generateForCountLoop(node *ASTNode) {
	gen.writeIndent()

	// Generate loop variable
	loopVar := fmt.Sprintf("__loop_i_%d", gen.varCounter)
	gen.varCounter++

	// Push loop counter onto stack
	gen.loopCounters = append(gen.loopCounters, loopVar)

	startVal := node.Value
	if startVal == "" {
		startVal = "0"
	}

	gen.output.WriteString(fmt.Sprintf("for (int %s = %s; ; %s++) {\n",
		loopVar, startVal, loopVar))

	gen.indent++
	gen.generateNodeInternal(node.Children[0], false)
	gen.indent--

	// Pop loop counter from stack
	gen.loopCounters = gen.loopCounters[:len(gen.loopCounters)-1]

	gen.writeIndent()
	gen.output.WriteString("}\n")
}

func (gen *CodeGenerator) generateForInArrayLoop(node *ASTNode) {
	gen.writeIndent()

	// node.Children[0] is element variable name
	// node.Children[1] is array expression
	// node.Children[2] is body

	elementVar := node.Children[0].Value
	arrayExpr := node.Children[1]

	// Generate unique loop counter
	loopVar := fmt.Sprintf("__loop_i_%d", gen.varCounter)
	gen.varCounter++

	// Get array variable name for accessing size
	arrayName := gen.nodeToString(arrayExpr)

	// For now, assume arrays are DynamicArray* type
	gen.output.WriteString(fmt.Sprintf("for (int %s = 0; %s < %s->size; %s++) {\n",
		loopVar, loopVar, arrayName, loopVar))

	gen.indent++
	gen.writeIndent()

	// Cast from void* through intptr_t to int (handles stored integers correctly)
	gen.output.WriteString(fmt.Sprintf("int %s = (intptr_t)%s->data[%s];\n",
		elementVar, arrayName, loopVar))

	gen.generateNodeInternal(node.Children[2], false)
	gen.indent--

	gen.writeIndent()
	gen.output.WriteString("}\n")
}

func (gen *CodeGenerator) generateForInDictLoop(node *ASTNode) {
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
	gen.writeIndent()
	// Cast value through intptr_t for compatibility
	gen.output.WriteString(fmt.Sprintf("const char* %s = (const char*)(intptr_t)%s->value;\n", valueVar, entryVar))

	gen.generateNodeInternal(node.Children[3], false)

	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("%s = %s->next;\n", entryVar, entryVar))
	gen.indent--

	gen.writeIndent()
	gen.output.WriteString("}\n")
	gen.indent--

	gen.writeIndent()
	gen.output.WriteString("}\n")
}

func (gen *CodeGenerator) generateReturnStatement(node *ASTNode) {
	gen.writeIndent()
	gen.output.WriteString("return")
	if len(node.Children) > 0 {
		gen.output.WriteString(" ")
		gen.generateNode(node.Children[0])
	}
	gen.output.WriteString(";\n")
}

func (gen *CodeGenerator) generateImportStatement(node *ASTNode) {
	// Add include - check if it's a local or system include
	headerName := node.Value
	gen.includes[headerName] = true
}

func (gen *CodeGenerator) generateCall(node *ASTNode) {
	// Convert snake_case function names to PascalCase for C
	funcName := snakeToPascal(node.Value)

	// Handle special functions
	switch node.Value {
	case "print":
		gen.output.WriteString("printf(")

		// Process format string if first argument is a string literal
		if len(node.Children) > 0 && node.Children[0].Type == NODE_STRING {
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
			// No format string, just output arguments as-is
			for i, arg := range node.Children {
				if i > 0 {
					gen.output.WriteString(", ")
				}
				gen.generateNode(arg)
			}
		}
		gen.output.WriteString(")")

	case "sprintf":
		// sprintf returns a string - need to allocate buffer
		gen.output.WriteString("({ char* __str_buf = malloc(256); sprintf(__str_buf")

		// Process format string
		if len(node.Children) > 0 && node.Children[0].Type == NODE_STRING {
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

func (gen *CodeGenerator) generateBinaryOp(node *ASTNode) {
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

func (gen *CodeGenerator) generateUnaryOp(node *ASTNode) {
	switch node.Value {
	case "not":
		gen.output.WriteString("!")
	default:
		gen.output.WriteString(node.Value)
	}
	gen.generateNode(node.Children[0])
}

func (gen *CodeGenerator) generateArrayLiteral(node *ASTNode) {
	gen.arrayImpls = true

	// Create array with initial capacity
	arrName := fmt.Sprintf("arr_%d", gen.varCounter)
	gen.varCounter++

	// Use simple C array initialization
	gen.output.WriteString("({ ")
	gen.output.WriteString(fmt.Sprintf("AhoyArray* %s = malloc(sizeof(AhoyArray)); ", arrName))
	gen.output.WriteString(fmt.Sprintf("%s->length = %d; ", arrName, len(node.Children)))
	gen.output.WriteString(fmt.Sprintf("%s->capacity = %d; ", arrName, len(node.Children)))
	gen.output.WriteString(fmt.Sprintf("%s->data = malloc(%d * sizeof(int)); ", arrName, len(node.Children)))

	// Add elements
	for i, child := range node.Children {
		gen.output.WriteString(fmt.Sprintf("%s->data[%d] = ", arrName, i))
		gen.generateNode(child)
		gen.output.WriteString("; ")
	}

	gen.output.WriteString(fmt.Sprintf("%s; })", arrName))
}

func (gen *CodeGenerator) generateArrayAccess(node *ASTNode) {
	gen.output.WriteString(fmt.Sprintf("%s->data[", node.Value))
	gen.generateNode(node.Children[0])
	gen.output.WriteString("]")
}

func (gen *CodeGenerator) generateDictAccess(node *ASTNode) {
	gen.output.WriteString(fmt.Sprintf("hashMapGet(%s, ", node.Value))
	gen.generateNode(node.Children[0])
	gen.output.WriteString(")")
}

func (gen *CodeGenerator) generateDictLiteral(node *ASTNode) {
	dictName := fmt.Sprintf("dict_%d", gen.varCounter)
	gen.varCounter++

	gen.output.WriteString(fmt.Sprintf("({ HashMap* %s = createHashMap(16); ", dictName))

	// Add key-value pairs
	for i := 0; i < len(node.Children); i += 2 {
		key := node.Children[i]
		value := node.Children[i+1]

		gen.output.WriteString(fmt.Sprintf("hashMapPut(%s, ", dictName))
		gen.generateNode(key)
		gen.output.WriteString(", (void*)(intptr_t)")
		gen.generateNode(value)
		gen.output.WriteString("); ")
	}

	gen.output.WriteString(fmt.Sprintf("%s; })", dictName))
}

func (gen *CodeGenerator) mapType(langType string) string {
	switch langType {
	case "int":
		return "int"
	case "float":
		return "double"
	case "string":
		return "char*"
	case "bool":
		return "bool"
	case "dict":
		return "HashMap*"
	case "array":
		return "AhoyArray*"
	default:
		return "int"
	}
}

func (gen *CodeGenerator) inferType(node *ASTNode) string {
	switch node.Type {
	case NODE_NUMBER:
		if strings.Contains(node.Value, ".") {
			return "float"
		}
		return "int"
	case NODE_STRING:
		return "string"
	case NODE_BOOLEAN:
		return "bool"
	case NODE_DICT_LITERAL:
		return "dict"
	case NODE_ARRAY_LITERAL:
		return "array"
	case NODE_CALL:
		// Infer return type of function calls
		if node.Value == "sprintf" {
			return "string"
		}
		return "int"
	case NODE_METHOD_CALL:
		// Array methods that return arrays
		if node.Value == "map" || node.Value == "filter" ||
			node.Value == "sort" || node.Value == "reverse" ||
			node.Value == "shuffle" || node.Value == "push" {
			return "array"
		}
		// Methods that return int
		if node.Value == "length" || node.Value == "sum" ||
			node.Value == "pop" || node.Value == "pick" ||
			node.Value == "has" {
			return "int"
		}
		return "int"
	case NODE_BINARY_OP:
		// Simple inference - could be more sophisticated
		leftType := gen.inferType(node.Children[0])
		rightType := gen.inferType(node.Children[1])
		if leftType == "float" || rightType == "float" {
			return "float"
		}
		return "int"
	case NODE_IDENTIFIER:
		if varType, exists := gen.variables[node.Value]; exists {
			return varType
		}
		return "int"
	default:
		return "int"
	}
}

func (gen *CodeGenerator) nodeToString(node *ASTNode) string {
	oldOutput := gen.output
	gen.output = strings.Builder{}
	gen.generateNodeInternal(node, false)
	result := gen.output.String()
	gen.output = oldOutput
	return result
}

// Generate enum declaration
func (gen *CodeGenerator) generateEnum(node *ASTNode) {
	enumName := node.Value

	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("typedef enum {\n"))
	gen.indent++

	for i, member := range node.Children {
		gen.writeIndent()
		gen.output.WriteString(fmt.Sprintf("%s_%s = %d,\n", enumName, member.Value, i))
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("} %s;\n\n", enumName))
}

// Generate constant declaration
func (gen *CodeGenerator) generateConstant(node *ASTNode) {
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
func (gen *CodeGenerator) generateTupleAssignment(node *ASTNode) {
	leftSide := node.Children[0]
	rightSide := node.Children[1]

	// Generate temporary variables for swap
	temps := make([]string, len(rightSide.Children))
	for i, expr := range rightSide.Children {
		tempVar := fmt.Sprintf("__temp_%d", gen.varCounter)
		gen.varCounter++
		temps[i] = tempVar

		// Infer type from the expression
		exprType := gen.inferType(expr)
		gen.writeIndent()
		gen.output.WriteString(fmt.Sprintf("%s %s = ", exprType, tempVar))
		gen.generateNodeInternal(expr, false)
		gen.output.WriteString(";\n")
	}

	// Assign temps to left side variables
	for i, target := range leftSide.Children {
		if i < len(temps) {
			gen.writeIndent()
			gen.output.WriteString(fmt.Sprintf("%s = %s;\n", target.Value, temps[i]))
		}
	}
}

// Generate struct declaration
func (gen *CodeGenerator) generateStruct(node *ASTNode) {
	structName := node.Value

	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("typedef struct {\n"))
	gen.indent++

	for _, field := range node.Children {
		fieldType := gen.mapType(field.DataType)
		gen.writeIndent()
		gen.output.WriteString(fmt.Sprintf("%s %s;\n", fieldType, field.Value))
	}

	gen.indent--
	gen.writeIndent()
	gen.output.WriteString(fmt.Sprintf("} %s;\n\n", structName))
}

// Generate method call
func (gen *CodeGenerator) generateMethodCall(node *ASTNode) {
	object := node.Children[0]
	args := node.Children[1]
	methodName := node.Value

	// Handle map and filter with inline code generation
	if methodName == "map" || methodName == "filter" {
		if len(args.Children) > 0 && args.Children[0].Type == NODE_LAMBDA {
			if methodName == "map" {
				gen.generateMapInline(object, args.Children[0])
			} else {
				gen.generateFilterInline(object, args.Children[0])
			}
			return
		}
	}

	// Track which array method is used
	gen.arrayMethods[methodName] = true

	// Generate function call
	gen.output.WriteString(fmt.Sprintf("ahoy_array_%s(", methodName))
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
}

// Generate member access
func (gen *CodeGenerator) generateMemberAccess(node *ASTNode) {
	object := node.Children[0]
	memberName := node.Value

	gen.generateNodeInternal(object, false)
	gen.output.WriteString(".")
	gen.output.WriteString(memberName)
}

// Generate array helper functions
func (gen *CodeGenerator) writeArrayHelperFunctions() {
	if len(gen.arrayMethods) == 0 {
		return
	}

	gen.includes["time.h"] = true // For shuffle

	// Array structure definition
	gen.funcDecls.WriteString("\n// Array Helper Structure\n")
	gen.funcDecls.WriteString("typedef struct {\n")
	gen.funcDecls.WriteString("    int* data;\n")
	gen.funcDecls.WriteString("    int length;\n")
	gen.funcDecls.WriteString("    int capacity;\n")
	gen.funcDecls.WriteString("} AhoyArray;\n\n")

	// length method
	if gen.arrayMethods["length"] {
		gen.funcDecls.WriteString("int ahoy_array_length(AhoyArray* arr) {\n")
		gen.funcDecls.WriteString("    return arr->length;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// push method
	if gen.arrayMethods["push"] {
		gen.funcDecls.WriteString("AhoyArray* ahoy_array_push(AhoyArray* arr, int value) {\n")
		gen.funcDecls.WriteString("    if (arr->length >= arr->capacity) {\n")
		gen.funcDecls.WriteString("        arr->capacity = arr->capacity == 0 ? 4 : arr->capacity * 2;\n")
		gen.funcDecls.WriteString("        arr->data = realloc(arr->data, arr->capacity * sizeof(int));\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    arr->data[arr->length++] = value;\n")
		gen.funcDecls.WriteString("    return arr;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// pop method
	if gen.arrayMethods["pop"] {
		gen.funcDecls.WriteString("int ahoy_array_pop(AhoyArray* arr) {\n")
		gen.funcDecls.WriteString("    if (arr->length == 0) return 0;\n")
		gen.funcDecls.WriteString("    return arr->data[--arr->length];\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// sum method
	if gen.arrayMethods["sum"] {
		gen.funcDecls.WriteString("int ahoy_array_sum(AhoyArray* arr) {\n")
		gen.funcDecls.WriteString("    int total = 0;\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < arr->length; i++) {\n")
		gen.funcDecls.WriteString("        total += arr->data[i];\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    return total;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// has method
	if gen.arrayMethods["has"] {
		gen.funcDecls.WriteString("int ahoy_array_has(AhoyArray* arr, int value) {\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < arr->length; i++) {\n")
		gen.funcDecls.WriteString("        if (arr->data[i] == value) return 1;\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    return 0;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// sort method
	if gen.arrayMethods["sort"] {
		gen.funcDecls.WriteString("int __ahoy_compare_ints(const void* a, const void* b) {\n")
		gen.funcDecls.WriteString("    return (*(int*)a - *(int*)b);\n")
		gen.funcDecls.WriteString("}\n\n")
		gen.funcDecls.WriteString("AhoyArray* ahoy_array_sort(AhoyArray* arr) {\n")
		gen.funcDecls.WriteString("    qsort(arr->data, arr->length, sizeof(int), __ahoy_compare_ints);\n")
		gen.funcDecls.WriteString("    return arr;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// reverse method
	if gen.arrayMethods["reverse"] {
		gen.funcDecls.WriteString("AhoyArray* ahoy_array_reverse(AhoyArray* arr) {\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < arr->length / 2; i++) {\n")
		gen.funcDecls.WriteString("        int temp = arr->data[i];\n")
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
		gen.funcDecls.WriteString("        int temp = arr->data[i];\n")
		gen.funcDecls.WriteString("        arr->data[i] = arr->data[j];\n")
		gen.funcDecls.WriteString("        arr->data[j] = temp;\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    return arr;\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// pick method
	if gen.arrayMethods["pick"] {
		gen.funcDecls.WriteString("int ahoy_array_pick(AhoyArray* arr) {\n")
		gen.funcDecls.WriteString("    if (arr->length == 0) return 0;\n")
		gen.funcDecls.WriteString("    srand(time(NULL));\n")
		gen.funcDecls.WriteString("    return arr->data[rand() % arr->length];\n")
		gen.funcDecls.WriteString("}\n\n")
	}

	// print_array helper - formats array for printing
	if gen.arrayMethods["print_array"] {
		gen.funcDecls.WriteString("char* print_array_helper(AhoyArray* arr) {\n")
		gen.funcDecls.WriteString("    if (arr == NULL || arr->length == 0) return \"[]\";\n")
		gen.funcDecls.WriteString("    char* buffer = malloc(1024);\n")
		gen.funcDecls.WriteString("    int offset = 0;\n")
		gen.funcDecls.WriteString("    offset += sprintf(buffer + offset, \"[\");\n")
		gen.funcDecls.WriteString("    for (int i = 0; i < arr->length; i++) {\n")
		gen.funcDecls.WriteString("        if (i > 0) offset += sprintf(buffer + offset, \", \");\n")
		gen.funcDecls.WriteString("        offset += sprintf(buffer + offset, \"%d\", arr->data[i]);\n")
		gen.funcDecls.WriteString("    }\n")
		gen.funcDecls.WriteString("    offset += sprintf(buffer + offset, \"]\");\n")
		gen.funcDecls.WriteString("    return buffer;\n")
		gen.funcDecls.WriteString("}\n\n")
	}
}

// Process format string to replace %v and %t with appropriate C format specifiers
func (gen *CodeGenerator) processFormatString(formatStr string, args []*ASTNode) (string, []*ASTNode) {
	result := ""
	newArgs := []*ASTNode{}
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
						arrayArg := &ASTNode{
							Type:     NODE_CALL,
							Value:    "__print_array_helper", // Special marker
							Children: []*ASTNode{args[argIndex]},
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
					typeNode := &ASTNode{
						Type:  NODE_STRING,
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
func (gen *CodeGenerator) getNodeType(node *ASTNode) string {
	if node.DataType != "" {
		return node.DataType
	}

	switch node.Type {
	case NODE_NUMBER:
		if strings.Contains(node.Value, ".") {
			return "float"
		}
		return "int"
	case NODE_STRING:
		return "string"
	case NODE_CHAR:
		return "char"
	case NODE_BOOLEAN:
		return "bool"
	case NODE_ARRAY_LITERAL:
		return "array"
	case NODE_DICT_LITERAL:
		return "dict"
	case NODE_IDENTIFIER:
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

// Generate inline map code
func (gen *CodeGenerator) generateMapInline(arrayNode *ASTNode, lambda *ASTNode) {
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
func (gen *CodeGenerator) generateFilterInline(arrayNode *ASTNode, lambda *ASTNode) {
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
