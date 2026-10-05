package ahoy

// Public parse entry points.
//
// These live here rather than in parser.go, which is large enough that some
// tooling refuses to analyze it and then serves stale type information for the
// whole package - including these exported functions.
//
// Parse/ParseWithPath are for code generation. ParseLint/ParseLintWithPath/
// ParseLintWithPathInProgram are for validation: they collect diagnostics
// instead of panicking, and they run the semantic checks.

func Parse(tokens []Token) *ASTNode {
	parser := &Parser{
		tokens:              tokens,
		pos:                 0,
		LintMode:            false,
		Errors:              []ParseError{},
		variableTypes:       make(map[string]string),
		constants:           make(map[string]int),
		constantsInMain:     make(map[string]bool),
		constantUsages:      make(map[string][]int),
		declaredVars:        make(map[string]int),
		scopeStack:          make([]map[string]int, 0),
		functionScopeStack:  make([]map[string]string, 0),
		inConditionalScope:  false,
		structs:             make(map[string]*StructDefinition),
		enums:               make(map[string]*EnumDefinition),
		typeAliases:         make(map[string]string),
		unionTypes:          make(map[string][]string),
		objectLiterals:      make(map[string]map[string]bool),
		currentFunctionRet:  "",
		currentFunctionName: "",
		functionScope:       make(map[string]string),
		functions:           make(map[string]*FunctionSignature),
		arrayLengths:        make(map[string]ArrayInfo),
		cHeaders:            make(map[string]*CHeaderInfo),
		cHeaderGlobal:       &CHeaderInfo{Functions: make(map[string]*CFunction), Enums: make(map[string]*CEnum), Defines: make(map[string]*CDefine), Structs: make(map[string]*CStruct)},
		blockDepth:          0,
		loopVarScopes:       make([]map[string]string, 0),
		functionDepth:       0,
		hasProgramDecl:      false,
		inFunctionBody:      false,
		sourceFilePath:      "",
		zeroArgFunctions:    make(map[string]bool),
	}
	ast := parser.parseProgram()
	stampNodeFiles(ast, "")
	return ast
}

func ParseWithPath(tokens []Token, sourceFilePath string) *ASTNode {
	parser := &Parser{
		tokens:              tokens,
		pos:                 0,
		LintMode:            false,
		Errors:              []ParseError{},
		variableTypes:       make(map[string]string),
		constants:           make(map[string]int),
		constantsInMain:     make(map[string]bool),
		constantUsages:      make(map[string][]int),
		declaredVars:        make(map[string]int),
		scopeStack:          make([]map[string]int, 0),
		functionScopeStack:  make([]map[string]string, 0),
		inConditionalScope:  false,
		structs:             make(map[string]*StructDefinition),
		enums:               make(map[string]*EnumDefinition),
		typeAliases:         make(map[string]string),
		unionTypes:          make(map[string][]string),
		objectLiterals:      make(map[string]map[string]bool),
		currentFunctionRet:  "",
		currentFunctionName: "",
		functionScope:       make(map[string]string),
		functions:           make(map[string]*FunctionSignature),
		arrayLengths:        make(map[string]ArrayInfo),
		cHeaders:            make(map[string]*CHeaderInfo),
		cHeaderGlobal:       &CHeaderInfo{Functions: make(map[string]*CFunction), Enums: make(map[string]*CEnum), Defines: make(map[string]*CDefine), Structs: make(map[string]*CStruct)},
		blockDepth:          0,
		loopVarScopes:       make([]map[string]string, 0),
		functionDepth:       0,
		hasProgramDecl:      false,
		inFunctionBody:      false,
		sourceFilePath:      sourceFilePath,
		zeroArgFunctions:    make(map[string]bool),
	}
	ast := parser.parseProgram()
	stampNodeFiles(ast, sourceFilePath)
	return ast
}

func ParseLint(tokens []Token) (*ASTNode, []ParseError) {
	return parseLint(tokens, "", nil)
}

func ParseLintWithPath(tokens []Token, sourceFilePath string) (*ASTNode, []ParseError) {
	return parseLint(tokens, sourceFilePath, nil)
}

// ParseLintWithPathInProgram is ParseLintWithPath for a file that belongs to a
// multi-file program. Names declared by the program's other files are treated
// as already defined, so a function that is used in one file and declared in
// another is not reported as undefined.
func ParseLintWithPathInProgram(tokens []Token, sourceFilePath string, programNames []string) (*ASTNode, []ParseError) {
	return parseLint(tokens, sourceFilePath, programNames)
}

// parseLint builds a validation parser, parses, runs the semantic checks, and
// stamps the source file onto the resulting diagnostics.
func parseLint(tokens []Token, sourceFilePath string, programNames []string) (*ASTNode, []ParseError) {
	parser := &Parser{
		tokens:              tokens,
		pos:                 0,
		LintMode:            true,
		Errors:              []ParseError{},
		variableTypes:       make(map[string]string),
		constants:           make(map[string]int),
		constantsInMain:     make(map[string]bool),
		constantUsages:      make(map[string][]int),
		declaredVars:        make(map[string]int),
		scopeStack:          make([]map[string]int, 0),
		functionScopeStack:  make([]map[string]string, 0),
		inConditionalScope:  false,
		structs:             make(map[string]*StructDefinition),
		enums:               make(map[string]*EnumDefinition),
		typeAliases:         make(map[string]string),
		unionTypes:          make(map[string][]string),
		objectLiterals:      make(map[string]map[string]bool),
		currentFunctionRet:  "",
		currentFunctionName: "",
		functionScope:       make(map[string]string),
		functions:           make(map[string]*FunctionSignature),
		arrayLengths:        make(map[string]ArrayInfo),
		cHeaders:            make(map[string]*CHeaderInfo),
		cHeaderGlobal:       &CHeaderInfo{Functions: make(map[string]*CFunction), Enums: make(map[string]*CEnum), Defines: make(map[string]*CDefine), Structs: make(map[string]*CStruct)},
		blockDepth:          0,
		loopVarScopes:       make([]map[string]string, 0),
		functionDepth:       0,
		hasProgramDecl:      false,
		inFunctionBody:      false,
		sourceFilePath:      sourceFilePath,
		zeroArgFunctions:    make(map[string]bool),
	}
	ast := parser.parseProgram()
	parser.checkSemantics(ast, programNames)
	stampErrorFiles(parser.Errors, sourceFilePath)
	stampNodeFiles(ast, sourceFilePath)
	return ast, parser.Errors
}
