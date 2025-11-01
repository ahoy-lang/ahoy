package main

import (
	"fmt"
	"strings"

	"ahoy"
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
	output        strings.Builder
	indent        int
	varCounter    int
	funcDecls     strings.Builder
	includes      map[string]bool
	variables     map[string]string // variable name -> type
	constants     map[string]bool   // constant name -> declared
	hasError      bool              // Track if error occurred
	arrayImpls    bool              // Track if we've added array implementation
	arrayMethods  map[string]bool   // Track which array methods are used
	stringMethods map[string]bool   // Track which string methods are used
	dictMethods   map[string]bool   // Track which dict methods are used
	loopCounters  []string          // Stack of loop counter variable names
}

func generateC(ast *ahoy.ASTNode) string {
	gen := &CodeGenerator{
		includes:      make(map[string]bool),
		variables:     make(map[string]string),
		constants:     make(map[string]bool),
		hasError:      false,
		arrayImpls:    false,
		arrayMethods:  make(map[string]bool),
		stringMethods: make(map[string]bool),
		dictMethods:   make(map[string]bool),
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
	
	// Check if there were any errors
	if gen.hasError {
		return "" // Return empty string to indicate error
	}

	// Generate array helper functions if any array methods were used
	gen.writeArrayHelperFunctions()

	// Generate dict helper functions if any dict methods were used
	gen.writeDictHelperFunctions()

	// Generate string helper functions if any string methods were used
	gen.writeStringHelperFunctions()

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
		gen.generateTernary(node)

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
		gen.generateMethodCall(node)
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

func (gen *CodeGenerator) generateAssignment(node *ahoy.ASTNode) {
	gen.writeIndent()

	// Check if this is a property/element assignment (obj<'prop'>: value or dict{"key"}: value)
	// In this case, Children[0] is the access node, Children[1] is the value
	if len(node.Children) == 2 && 
	   (node.Children[0].Type == ahoy.NODE_OBJECT_ACCESS || 
	    node.Children[0].Type == ahoy.NODE_DICT_ACCESS ||
	    node.Children[0].Type == ahoy.NODE_ARRAY_ACCESS) {
		// Generate the access expression
		gen.generateNode(node.Children[0])
		gen.output.WriteString(" = ")
		gen.generateNode(node.Children[1])
		gen.output.WriteString(";\n")
		return
	}

	// Check if variable already exists
	if _, exists := gen.variables[node.Value]; exists {
		// Just assignment
		gen.output.WriteString(fmt.Sprintf("%s = ", node.Value))
		gen.generateNode(node.Children[0])
		gen.output.WriteString(";\n")
	} else {
		// Type inference and declaration
		valueNode := node.Children[0]
		
		// Special handling for object literals - they define their own type inline
		if valueNode.Type == ahoy.NODE_OBJECT_LITERAL {
			gen.output.WriteString("struct { ")
			// Generate field declarations
			for _, prop := range valueNode.Children {
				if prop.Type == ahoy.NODE_OBJECT_PROPERTY {
					propType := gen.inferType(prop.Children[0])
					gen.output.WriteString(propType)
					gen.output.WriteString(" ")
					gen.output.WriteString(prop.Value)
					gen.output.WriteString("; ")
				}
			}
			gen.output.WriteString("} ")
			gen.output.WriteString(node.Value)
			gen.output.WriteString(" = ")
			gen.generateNode(valueNode)
			gen.output.WriteString(";\n")
			gen.variables[node.Value] = "object"
		} else {
			varType := gen.inferType(valueNode)
			gen.variables[node.Value] = varType

			cType := gen.mapType(varType)
			gen.output.WriteString(fmt.Sprintf("%s %s = ", cType, node.Value))
			gen.generateNode(valueNode)
			gen.output.WriteString(";\n")
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

func (gen *CodeGenerator) generateSwitchStatement(node *ahoy.ASTNode) {
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

	// Check if we have an explicit loop variable (loop i till condition)
	// Pattern: Children[0] is loop var, Children[1] is condition, Children[2] is body
	// Old pattern: Children[0] is condition, Children[1] is body
	var loopVar string
	var conditionNode *ahoy.ASTNode
	var bodyNode *ahoy.ASTNode

	if len(node.Children) == 3 && node.Children[0].Type == ahoy.NODE_IDENTIFIER {
		// New syntax: loop i till condition
		loopVar = node.Children[0].Value
		conditionNode = node.Children[1]
		bodyNode = node.Children[2]

		// Initialize loop variable
		gen.output.WriteString(fmt.Sprintf("int %s = 0;\n", loopVar))
		gen.writeIndent()
	} else {
		// Old syntax: loop condition
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
		gen.output.WriteString(fmt.Sprintf("; %s <= ", loopVar))
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

	// Check if this is the new forever loop syntax (loop i: or loop:)
	// Pattern 1: Children[0] is identifier (loop var), Children[1] is body
	// Pattern 2: Children[0] is body only (old syntax, infinite loop)
	// Old pattern: Children[0-3] are init/condition/update/body

	if len(node.Children) == 2 && node.Children[0].Type == ahoy.NODE_IDENTIFIER {
		// New syntax: loop i: (forever loop with explicit variable)
		loopVar := node.Children[0].Value
		gen.output.WriteString(fmt.Sprintf("for (int %s = 0; ; %s++) {\n", loopVar, loopVar))

		gen.indent++
		gen.generateNodeInternal(node.Children[1], false)
		gen.indent--

		gen.writeIndent()
		gen.output.WriteString("}\n")
	} else if len(node.Children) == 1 || node.Value == "0" {
		// Forever loop without explicit variable (loop: or loop do)
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
	// node.Children[1] is array expression
	// node.Children[2] is body

	elementVar := node.Children[0].Value
	arrayExpr := node.Children[1]

	// Generate unique loop counter
	loopVar := fmt.Sprintf("__loop_i_%d", gen.varCounter)
	gen.varCounter++

	// Get array variable name for accessing size
	arrayName := gen.nodeToString(arrayExpr)

	// AhoyArray uses 'length', not 'size'
	gen.output.WriteString(fmt.Sprintf("for (int %s = 0; %s < %s->length; %s++) {\n",
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

func (gen *CodeGenerator) generateReturnStatement(node *ahoy.ASTNode) {
	gen.writeIndent()
	gen.output.WriteString("return")
	if len(node.Children) > 0 {
		gen.output.WriteString(" ")
		// Handle multiple return values
		if len(node.Children) > 1 {
			// Multiple returns - create an inline struct
			// Note: This requires the function to return a struct type
			// For now, we'll just return the first value and add a comment
			gen.output.WriteString("/* TODO: Multiple returns not fully supported in C */ ")
		}
		gen.generateNode(node.Children[0])
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
	// Convert snake_case function names to PascalCase for C
	funcName := snakeToPascal(node.Value)

	// Handle special functions
	switch node.Value {
	case "print":
		gen.output.WriteString("printf(")

		// Process format string if first argument is a string literal
		if len(node.Children) > 0 && node.Children[0].Type == ahoy.NODE_STRING {
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
			// No format string, infer format from argument types
			if len(node.Children) > 0 {
				for i, arg := range node.Children {
					if i > 0 {
						gen.output.WriteString("; printf(")
					}
					
					// Infer type and generate appropriate printf call
					argType := gen.inferType(arg)
					formatSpec := ""
					switch argType {
					case "string":
						formatSpec = "%s"
					case "int":
						formatSpec = "%d"
					case "float":
						formatSpec = "%f"
					case "bool":
						formatSpec = "%d"
					case "char":
						formatSpec = "%c"
					default:
						formatSpec = "%d"
					}
					
					gen.output.WriteString(fmt.Sprintf("\"%s\\n\", ", formatSpec))
					gen.generateNode(arg)
					gen.output.WriteString(")")
				}
			}
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

	// List of string methods
	stringMethodsList := []string{
		"length", "upper", "lower", "replace", "contains",
		"camel_case", "snake_case", "pascal_case", "kebab_case",
		"match", "split", "count", "lpad", "rpad", "pad",
		"strip", "get_file",
	}

	// List of dictionary methods
	dictMethodsList := []string{
		"size", "clear", "has", "has_all", "keys", "values",
		"sort", "stable_sort", "merge",
	}

	// Check if this is a string method
	isStringMethod := false
	for _, m := range stringMethodsList {
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

	if isStringMethod {
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
	} else if isDictMethod {
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

		// Generate array method function call
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

func (gen *CodeGenerator) generateArrayAccess(node *ahoy.ASTNode) {
	gen.output.WriteString(fmt.Sprintf("%s->data[", node.Value))
	gen.generateNode(node.Children[0])
	gen.output.WriteString("]")
}

func (gen *CodeGenerator) generateDictAccess(node *ahoy.ASTNode) {
	gen.output.WriteString(fmt.Sprintf("hashMapGet(%s, ", node.Value))
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

func (gen *CodeGenerator) inferType(node *ahoy.ASTNode) string {
	switch node.Type {
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
	case ahoy.NODE_IDENTIFIER:
		if varType, exists := gen.variables[node.Value]; exists {
			return varType
		}
		return "int"
	default:
		return "int"
	}
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
				if varType == "string" {
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
func (gen *CodeGenerator) generateTupleAssignment(node *ahoy.ASTNode) {
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
func (gen *CodeGenerator) generateStruct(node *ahoy.ASTNode) {
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

// Generate member access
func (gen *CodeGenerator) generateMemberAccess(node *ahoy.ASTNode) {
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
		if !gen.arrayMethods["__dummy__"] {
			// Add array structure if not already added
			gen.funcDecls.WriteString("// Array Helper Structure\n")
			gen.funcDecls.WriteString("typedef struct {\n")
			gen.funcDecls.WriteString("    int* data;\n")
			gen.funcDecls.WriteString("    int length;\n")
			gen.funcDecls.WriteString("    int capacity;\n")
			gen.funcDecls.WriteString("} AhoyArray;\n\n")
		}
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

func (gen *CodeGenerator) writeStringHelperFunctions() {
	if len(gen.stringMethods) == 0 {
		return
	}

	gen.includes["ctype.h"] = true  // For tolower/toupper
	gen.includes["regex.h"] = true  // For regex matching

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
	// The struct definition is already generated in generateAssignment
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
