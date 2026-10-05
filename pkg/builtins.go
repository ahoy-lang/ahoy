package ahoy

// Names that resolve without being declared in the program under compilation.
//
// The authoritative registry for code generation is source/stdlib.go; this is
// the name set the front end resolves against when checking calls. A test in
// source/stdlib_test.go asserts the two agree, so adding a stdlib function
// cannot silently make the checker reject calls to it.
//
// The set is deliberately permissive: a name that is accepted here but is not
// really callable costs a missed diagnostic, whereas a name that is missing
// here turns working code into a build failure.

// builtinCallNames may be used as a bare call: name|args|
var builtinCallNames = map[string]bool{}

// builtinMethodNames may be used with method syntax: value.name|args|
var builtinMethodNames = map[string]bool{}

func init() {
	// stdlib.go, Category "builtin".
	for _, name := range []string{
		"assert", "exit", "float", "free", "input", "int", "len", "log",
		"panic", "print", "rand", "rand_range", "read_json", "sleep", "str",
		"typeof", "write_json",
	} {
		builtinCallNames[name] = true
		builtinMethodNames[name] = true
	}

	// stdlib.go array/dict/string methods, callable with method syntax and
	// tolerated as bare calls.
	for _, name := range []string{
		// array
		"length", "push", "pop", "sum", "has", "sort", "reverse", "shuffle",
		"pick", "fill", "remove", "dup", "clear", "size",
		// dict
		"has_all", "keys", "values", "stable_sort", "merge",
		// string
		"upper", "lower", "replace", "contains", "strip", "count", "lpad",
		"rpad", "pad", "match", "get_file", "camel_case", "snake_case",
		"pascal_case", "kebab_case", "split",
	} {
		builtinMethodNames[name] = true
		builtinCallNames[name] = true
	}

	// Handled directly by the code generator rather than by stdlib.go.
	for _, name := range []string{
		"ahoy",      // alias for print
		"sprintf",   // C
		"malloc",    // C
		"char",      // cast
		"string",    // cast
		"map",       // collection pipeline
		"filter",
		"each",
		"reduce",
		"any",
		"all",
		"find",
		"index_of",
		"slice",
		"append",
		"insert",
		"erase",
		"join",
		"type",       // value.type property
		"dump_struct", // debug helper
	} {
		builtinCallNames[name] = true
		builtinMethodNames[name] = true
	}

	// JSON helpers emitted by the code generator.
	for _, name := range []string{
		"ahoy_json_string", "ahoy_json_number", "ahoy_json_int",
		"ahoy_json_bool", "ahoy_json_get", "ahoy_json_get_index",
	} {
		builtinCallNames[name] = true
		builtinMethodNames[name] = true
	}
}

// IsBuiltinCallName reports whether name is callable as a bare call.
func IsBuiltinCallName(name string) bool { return builtinCallNames[name] }

// IsBuiltinMethodName reports whether name is callable with method syntax.
func IsBuiltinMethodName(name string) bool { return builtinMethodNames[name] }
