package ahoy

// Diagnostic reporting types.
//
// These live in their own file rather than in parser.go. parser.go is large
// enough that some tooling refuses to analyze it at all, which left every
// consumer of these types (the CLI, the LSP, tests) seeing stale type
// information. Keeping the definitions in a small file also makes the shape of
// a diagnostic obvious at a glance.

// ParseError is a single diagnostic produced while parsing or validating a
// source file.
type ParseError struct {
	Message string
	Line    int
	Column  int
	// File is the source file the diagnostic belongs to. It is stamped on
	// every error once parsing finishes, so the many construction sites do not
	// each have to know the path.
	File string
	// Severity is "error" or "warning". The zero value means "error".
	Severity string
}

// IsWarning reports whether the diagnostic is advisory rather than fatal.
func (e ParseError) IsWarning() bool {
	return e.Severity == "warning"
}

// stampErrorFiles records the originating file on every diagnostic that does not
// already carry one. Diagnostics are created in many places, most of which have
// no idea which file they are parsing, so the path is filled in once here at the
// end of the parse instead.
func stampErrorFiles(errors []ParseError, sourceFilePath string) {
	for i := range errors {
		if errors[i].File == "" {
			errors[i].File = sourceFilePath
		}
	}
}

// stampNodeFiles records the source file on every top-level node. Code
// generation needs it to emit #line directives, so that C compiler errors,
// sanitizer reports and stack traces name the .ahoy file rather than a line in
// the generated C. Nodes nested inside a function or block inherit their file
// from the enclosing top-level node, which is sufficient because a whole
// function always comes from a single file.
func stampNodeFiles(ast *ASTNode, sourceFilePath string) {
	if ast == nil {
		return
	}
	for _, child := range ast.Children {
		if child.File == "" {
			child.File = sourceFilePath
		}
	}
}
