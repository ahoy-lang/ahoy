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

// newParser constructs a parser with the maps and scopes every parse mode
// needs. lintMode selects diagnostic collection (LintMode=true) instead of the
// panic-on-error behaviour used by code generation.
func newParser(tokens []Token, sourceFilePath string, lintMode bool) *Parser {
	return &Parser{
		tokens:              tokens,
		pos:                 0,
		LintMode:            lintMode,
		Errors:              []ParseError{},
		variableTypes:       make(map[string]string),
		constants:           make(map[string]int),
		constantsInMain:     make(map[string]bool),
		constantUsages:      make(map[string][]int),
		declaredVars:        make(map[string]int),
		functionScopeStack:  make([]map[string]string, 0),
		structs:             make(map[string]*StructDefinition),
		enums:               make(map[string]*EnumDefinition),
		typeAliases:         make(map[string]string),
		distinctTypes:       make(map[string]string),
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
}

func Parse(tokens []Token) *ASTNode {
	parser := newParser(tokens, "", false)
	ast := parser.parseProgram()
	stampNodeFiles(ast, "")
	return ast
}

func ParseWithPath(tokens []Token, sourceFilePath string) *ASTNode {
	parser := newParser(tokens, sourceFilePath, false)
	ast := parser.parseProgram()
	stampNodeFiles(ast, sourceFilePath)
	return ast
}

// ParseSyntaxWithPath parses a file in lint mode without running the semantic
// checks. It is for tools that must discover a program's imports and declared
// names before the real whole-program lint pass: the semantic checks are where
// names from imported files would otherwise be reported as undefined, and
// generating those suggestions is expensive.
func ParseSyntaxWithPath(tokens []Token, sourceFilePath string) (*ASTNode, []ParseError) {
	parser := newParser(tokens, sourceFilePath, true)
	ast := parser.parseProgram()
	stampErrorFiles(parser.Errors, sourceFilePath)
	stampNodeFiles(ast, sourceFilePath)
	return ast, parser.Errors
}

func ParseLint(tokens []Token) (*ASTNode, []ParseError) {
	return parseLint(tokens, "", nil, nil)
}

func ParseLintWithPath(tokens []Token, sourceFilePath string) (*ASTNode, []ParseError) {
	return parseLint(tokens, sourceFilePath, nil, nil)
}

// ParseLintWithPathInProgram is ParseLintWithPath for a file that belongs to a
// multi-file program. Names declared by the program's other files are treated
// as already defined, so a function that is used in one file and declared in
// another is not reported as undefined. programMethods carries the other
// files' methods the same way, so method calls validate across files.
func ParseLintWithPathInProgram(tokens []Token, sourceFilePath string, programNames []string, programMethods map[string]map[string]*MethodEntry) (*ASTNode, []ParseError) {
	return parseLint(tokens, sourceFilePath, programNames, programMethods)
}

// parseLint builds a validation parser, parses, runs the semantic checks, and
// stamps the source file onto the resulting diagnostics.
func parseLint(tokens []Token, sourceFilePath string, programNames []string, programMethods map[string]map[string]*MethodEntry) (*ASTNode, []ParseError) {
	parser := newParser(tokens, sourceFilePath, true)
	ast := parser.parseProgram()
	parser.checkSemantics(ast, programNames, programMethods)
	stampErrorFiles(parser.Errors, sourceFilePath)
	stampNodeFiles(ast, sourceFilePath)
	return ast, parser.Errors
}
