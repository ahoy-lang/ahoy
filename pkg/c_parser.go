package ahoy

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// CHeaderParseResult holds the result of parsing a C header file
type CHeaderParseResult struct {
	Path      string
	Namespace string
	Info      *CHeaderInfo
	Err       error
}

// ParseCHeadersConcurrently parses multiple C header files in parallel using goroutines
func ParseCHeadersConcurrently(headers []struct{ Path, Namespace string }) []CHeaderParseResult {
	results := make([]CHeaderParseResult, len(headers))
	var wg sync.WaitGroup

	for i, h := range headers {
		wg.Add(1)
		go func(idx int, header struct{ Path, Namespace string }) {
			defer wg.Done()
			info, err := ParseCHeader(header.Path)
			results[idx] = CHeaderParseResult{
				Path:      header.Path,
				Namespace: header.Namespace,
				Info:      info,
				Err:       err,
			}
		}(i, h)
	}

	wg.Wait()
	return results
}

// headerMemEntry is one cached header parse result with the file identity it
// was parsed from.
type headerMemEntry struct {
	modTime time.Time
	size    int64
	info    *CHeaderInfo
}

// headerMemCache keeps header parses alive for the process. Every Ahoy file
// that imports raylib.h or glue.h re-parses them during its own parse, and
// each file is parsed several times (parse + validation passes), so without
// this the same headers are re-read and re-parsed dozens of times per build.
// (The on-disk BuildCache in the compiler frontend covers repeated *builds*
// when -cache is on; this covers repeats *within* one build, which always
// happen.) Entries are validated by mod time and size, so edited headers are
// never served stale.
var headerMemCache = struct {
	sync.RWMutex
	m map[string]*headerMemEntry
}{m: make(map[string]*headerMemEntry)}

// headerCacheKey normalises a header path so relative and absolute spellings
// of the same file share one cache entry.
func headerCacheKey(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// ParseCHeader parses a C header file and extracts function signatures, enums, defines, and structs
func ParseCHeader(path string) (*CHeaderInfo, error) {
	key := headerCacheKey(path)
	if fi, statErr := os.Stat(path); statErr == nil {
		headerMemCache.RLock()
		if e, ok := headerMemCache.m[key]; ok && e.modTime.Equal(fi.ModTime()) && e.size == fi.Size() {
			info := e.info
			headerMemCache.RUnlock()
			return info, nil
		}
		headerMemCache.RUnlock()
	}

	info, err := parseCHeaderUncached(path)
	if err != nil {
		return nil, err
	}
	if fi, statErr := os.Stat(path); statErr == nil {
		headerMemCache.Lock()
		headerMemCache.m[key] = &headerMemEntry{modTime: fi.ModTime(), size: fi.Size(), info: info}
		headerMemCache.Unlock()
	}
	return info, nil
}

// parseCHeaderUncached does the actual line-by-line header parsing.
func parseCHeaderUncached(path string) (*CHeaderInfo, error) {
	info := &CHeaderInfo{
		Functions: make(map[string]*CFunction),
		Enums:     make(map[string]*CEnum),
		Defines:   make(map[string]*CDefine),
		Structs:   make(map[string]*CStruct),
		Typedefs:  make(map[string]*CTypedef),
	}

	// Read the header file
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")

	// Parse line by line
	for i, line := range lines {		line = strings.TrimSpace(line)
		lineNum := i + 1

		// Parse RLAPI/RMAPI function declarations
		if strings.Contains(line, "RLAPI") || strings.Contains(line, "RMAPI") {
			parseRLAPIFunction(line, lineNum, path, info)
		} else if parseMacroFunction(line, lineNum, path, info) {
			// Successfully parsed as macro function (like CJSON_PUBLIC)
		} else if strings.Contains(line, "(") && strings.Contains(line, ");") && !strings.HasPrefix(line, "typedef") && !strings.HasPrefix(line, "#") && !looksLikeCStatement(line) {
			// Try to parse as generic function declaration
			parseRLAPIFunction(line, lineNum, path, info)
		}

		// Parse #define constants
		if strings.HasPrefix(line, "#define") {
			parseDefine(line, lineNum, path, info)
		}

		// Parse typedef struct
		if strings.HasPrefix(line, "typedef struct") {
			parseStruct(lines, i, path, info)
		}

		// Plain `struct NAME { ... };` (Tier-3 Feature 3): system headers
		// define `struct dirent`, `struct stat` without typedef. The
		// typedef pass above misses them, so member typing (`e.d_name`,
		// `st.st_mode`) cannot resolve.
		if strings.HasPrefix(line, "struct ") && !strings.Contains(line, "(") && !strings.Contains(line, ";") {
			parsePlainStruct(lines, i, path, info)
		}

		// Parse typedef enum
		if strings.HasPrefix(line, "typedef enum") {
			parseEnum(lines, i, path, info)
		}

		// Typedefs often carry a trailing comment; strip it before deciding what
		// kind of typedef this is.
		typedefLine := line
		if idx := strings.Index(typedefLine, "//"); idx != -1 {
			typedefLine = strings.TrimSpace(typedefLine[:idx])
		}

		// Parse function pointer typedefs first: the simple-alias branch would
		// misread `typedef void (*LogCallback)(int);`.
		if strings.HasPrefix(typedefLine, "typedef ") && strings.Contains(typedefLine, "(*") && strings.HasSuffix(typedefLine, ";") {
			parseFuncPtrTypedef(typedefLine, lineNum, path, info)
		} else if strings.HasPrefix(typedefLine, "typedef ") && !strings.Contains(typedefLine, "{") && strings.HasSuffix(typedefLine, ";") {
			parseTypedefAlias(typedefLine, lineNum, path, info)
		}
	}

	// Follow #includes (Tier-3 Feature 1 + math-wrapper removal): the header
	// comment's gap #1 was "the C header parser does not follow #include",
	// so `sinf`/`cosf` (via math.h) and `struct dirent` (via bits/dirent.h)
	// were invisible when only the top header was imported. Merge one level
	// of direct includes (with cycle guard); deeper chains resolve through
	// the cache on their own pass.
	followHeaderIncludes(path, string(content), info, make(map[string]bool))

	// Second pass for system headers (Tier-3 Feature 1): GNU libc splits
	// declarations across lines (`extern DIR *opendir (...)` + attributes
	// on the next line) and decorates them (`__nonnull`, `__THROW`,
	// `__REDIRECT`). The line loop above only sees single-line `...);`,
	// so multi-line decls are missed and user syscalls (`opendir`,
	// `readdir`, `stat`, `mkdir`) report "not defined". Re-split the file
	// into `;`-terminated statements, collapse newlines, and retry the
	// ones the first pass could not have seen.
	for _, stmt := range strings.Split(string(content), ";") {
		// Strip block comments: `/* Open a directory ... , or NULL ... */`
		// carries a top-level comma that looksLikeCStatement rejects.
		clean := stmt
		for {
			s := strings.Index(clean, "/*")
			e := strings.Index(clean, "*/")
			if s == -1 || e == -1 || e < s {
				break
			}
			clean = clean[:s] + " " + clean[e+2:]
		}
		// Drop preprocessor lines (`# endif`, `# if ...`): a declaration
		// following one shares its `;`-chunk, so the flat form starts
		// with `#` and is skipped as a directive.
		kept := make([]string, 0, 8)
		for _, ln := range strings.Split(clean, "\n") {
			if strings.HasPrefix(strings.TrimSpace(ln), "#") {
				continue
			}
			kept = append(kept, ln)
		}
		clean = strings.Join(kept, "\n")
		flat := strings.Join(strings.Fields(clean), " ")
		flat = strings.TrimSpace(flat)
		if flat == "" || !strings.Contains(flat, "(") || !strings.Contains(flat, ")") {
			continue
		}
		if strings.HasPrefix(flat, "typedef ") || strings.HasPrefix(flat, "#") {
			continue
		}
		// System multi-line decls are `extern ...`; anything else is a
		// macro fragment or comment residue (the first pass owns the
		// single-line cases).
		if !strings.Contains(flat, "extern ") {
			continue
		}
		if looksLikeCStatement(flat) {
			continue
		}
		// Skip if already captured (single-line pass handles the common case).
		parenIdx := strings.Index(flat, "(")
		before := strings.TrimSpace(flat[:parenIdx])
		parts := strings.Fields(before)
		if len(parts) == 0 {
			continue
		}
		cand, _ := splitCFuncName(parts[len(parts)-1])
		if cand == "" {
			continue
		}
		if _, ok := info.Functions[cand]; ok {
			continue
		}
		// Only retry statements that span lines (the first pass had no
		// chance); single-line misses are real non-declarations.
		if !strings.Contains(stmt, "\n") {
			continue
		}
		parseRLAPIFunction(flat, 1, path, info)
	}

	return info, nil
}

// followHeaderIncludes merges one level of #include'd headers into info.
// Quoted includes resolve relative to the including file; angle includes
// resolve via /usr/include (+ multi-arch). Guarded by visited so diamond
// includes (raylib.h via glue.h + direct) parse once.
func followHeaderIncludes(path, content string, info *CHeaderInfo, visited map[string]bool) {
	if visited[path] {
		return
	}
	visited[path] = true
	baseDir := filepath.Dir(path)
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, "#include") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(t, "#include"))
		var inc string
		var isSystem bool
		if strings.HasPrefix(rest, "\"") {
			end := strings.Index(rest[1:], "\"")
			if end == -1 {
				continue
			}
			inc = rest[1 : 1+end]
		} else if strings.HasPrefix(rest, "<") {
			end := strings.Index(rest, ">")
			if end == -1 {
				continue
			}
			inc = rest[1:end]
			isSystem = true
		} else {
			continue
		}
		// Skip C++-only or heavy toolchain headers that never carry Ahoy
		// declarations (they slow the parse and add garbage functions).
		if strings.HasSuffix(inc, ".hh") || strings.HasSuffix(inc, ".hpp") {
			continue
		}
		var cand string
		if !isSystem && !filepath.IsAbs(inc) {
			cand = filepath.Join(baseDir, inc)
			if _, err := os.Stat(cand); err != nil {
				cand = ""
			}
		}
		if cand == "" && isSystem {
			for _, loc := range []string{
				"/usr/include/" + inc,
				"/usr/local/include/" + inc,
				"/usr/include/x86_64-linux-gnu/" + inc,
			} {
				if _, err := os.Stat(loc); err == nil {
					cand = loc
					break
				}
			}
		}
		if cand == "" {
			continue
		}
		// Never recurse into the including file itself.
		if cand == path {
			continue
		}
		sub, err := ParseCHeader(cand)
		if err != nil || sub == nil {
			continue
		}
		for n, f := range sub.Functions {
			if _, ok := info.Functions[n]; !ok {
				info.Functions[n] = f
			}
		}
		for n, s := range sub.Structs {
			if _, ok := info.Structs[n]; !ok {
				info.Structs[n] = s
			}
		}
		for n, e := range sub.Enums {
			if _, ok := info.Enums[n]; !ok {
				info.Enums[n] = e
			}
		}
		for n, d := range sub.Defines {
			if _, ok := info.Defines[n]; !ok {
				info.Defines[n] = d
			}
		}
		for n, td := range sub.Typedefs {
			if _, ok := info.Typedefs[n]; !ok {
				info.Typedefs[n] = td
			}
		}
	}
}

// looksLikeCStatement reports whether a line that happens to contain "(" and
// ");" is really a statement rather than a declaration. Header bodies are
// scanned line by line, so `return ahoy_path_join(dir, name);` inside a
// function used to be registered as a declaration of ahoy_path_join with the
// return type "return" -- and overwrote the real signature, which made every
// variable declared from that call carry the type "return".
func looksLikeCStatement(line string) bool {
	if strings.HasPrefix(line, "}") || strings.HasPrefix(line, "{") {
		return true
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return true
	}
	switch fields[0] {
	case "return", "if", "else", "while", "for", "switch", "case", "default",
		"do", "goto", "break", "continue", "sizeof", "assert", "static_assert":
		return true
	}
	// a type followed by an assignment before the first "(" is a body statement
	paren := strings.Index(line, "(")
	if paren > 0 && strings.Contains(line[:paren], "=") {
		return true
	}
	// a top-level comma before the first "(" is a fragment of a call split
	// across lines, not a declaration: the argument line of a two-line
	// fprintf used to register its callee with the argument list as the
	// return type ("ahoy_perf_frame_no, fps, ms, ..., ahoy_vram_total_kb());"
	// made every variable declared from ahoy_vram_total_kb carry that list
	// as its type). Declarations never separate anything with a top-level
	// comma before the parameter list opens.
	if paren > 0 {
		depth := 0
		for _, ch := range line[:paren] {
			switch ch {
			case '(', '[', '{':
				depth++
			case ')', ']', '}':
				depth--
			case ',':
				if depth == 0 {
					return true
				}
			}
		}
	}
	return false
}

// normaliseCReturnType removes the parts of a C return type that do not change
// how the value is used. A header's `const char *GetWorkingDirectory(void)` was
// registered as "const char *", which the type mapping did not recognise as a
// string, so the result came back as int* and printed a pointer. Space before
// the star is closed up too, so "char *" and "char*" are the same type.
func normaliseCReturnType(t string) string {
	t = strings.TrimSpace(t)
	for {
		stripped := false
		for _, qualifier := range []string{"const ", "volatile ", "restrict ", "struct ", "enum ", "union "} {
			if strings.HasPrefix(t, qualifier) {
				t = strings.TrimSpace(t[len(qualifier):])
				stripped = true
			}
		}
		if !stripped {
			break
		}
	}
	t = strings.ReplaceAll(t, " *", "*")
	t = strings.ReplaceAll(t, "  ", " ")
	return strings.TrimSpace(t)
}

// parseRLAPIFunction parses a function declaration like: RLAPI void InitWindow(int width, int height, const char *title);
func parseRLAPIFunction(line string, lineNum int, filePath string, info *CHeaderInfo) {
	// Remove RLAPI/RMAPI prefix and comments
	line = strings.Replace(line, "RLAPI", "", 1)
	line = strings.Replace(line, "RMAPI", "", 1)
	if idx := strings.Index(line, "//"); idx != -1 {
		line = line[:idx]
	}
	line = strings.TrimSpace(line)

	// Extract return type and function signature
	parenIdx := strings.Index(line, "(")
	if parenIdx == -1 {
		return
	}

	beforeParen := strings.TrimSpace(line[:parenIdx])
	parts := strings.Fields(beforeParen)
	if len(parts) < 2 {
		return
	}

	funcName, stars := splitCFuncName(parts[len(parts)-1])
	// A declaration's storage class is not part of the return type. A header
	// with `static int bell_count(void);` registered the type as "static int",
	// and a variable declared from that call came out as `static int n = ...`
	// (a static local cannot take a non-constant initializer), while
	// `static const char* bell_name(...)` became `int*`.
	typeFields := []string{}
	for _, part := range parts[:len(parts)-1] {
		switch part {
		case "static", "extern", "inline", "register", "_Noreturn":
			continue
		}
		typeFields = append(typeFields, part)
	}
	returnType := normaliseCReturnType(strings.Join(typeFields, " "))
	if stars != "" {
		returnType = normaliseCReturnType(returnType + " " + stars)
	}

	// Extract parameters
	endParen := strings.Index(line, ")")
	if endParen == -1 {
		return
	}
	// A ")" before the first "(" is a continuation fragment of a statement
	// split across lines (e.g. `!table[i].tex) return commit(slot, data);`),
	// never a declaration: slicing [parenIdx+1:endParen] would panic.
	if endParen < parenIdx {
		return
	}

	paramStr := line[parenIdx+1 : endParen]
	params := parseParameters(paramStr)

	info.Functions[funcName] = &CFunction{
		Name:       funcName,
		ReturnType: returnType,
		Parameters: params,
		Line:       lineNum,
		File:       filePath,
	}
}

// parseMacroFunction parses function declarations with macro prefixes
// Examples: CJSON_PUBLIC(cJSON *) cJSON_Parse(const char *value);
//
// splitCFuncName separates a function name from the pointer stars that belong
// to its return type. raylib declares string-returning functions as
// `char *LoadFileText(...)`, and taking the last whitespace field verbatim
// registered the function as "*load_file_text": every Ahoy call to it was then
// reported as undefined, with "*load_file_text" offered as the suggestion.
func splitCFuncName(field string) (name string, stars string) {
	name = strings.TrimLeft(field, "*")
	stars = strings.Repeat("*", len(field)-len(name))
	return name, stars
}

// SOME_MACRO(void) MyFunction(int x);
func parseMacroFunction(line string, lineNum int, filePath string, info *CHeaderInfo) bool {
	// Skip #define lines
	if strings.HasPrefix(line, "#define") || strings.HasPrefix(line, "typedef") {
		return false
	}

	// Remove comments
	if idx := strings.Index(line, "//"); idx != -1 {
		line = line[:idx]
	}
	line = strings.TrimSpace(line)

	// Must end with ");
	if !strings.HasSuffix(line, ");") {
		return false
	}

	// Look for pattern: MACRO_NAME(return_type) function_name(params);
	// Find first parenthesis (for macro)
	firstParen := strings.Index(line, "(")
	if firstParen == -1 {
		return false
	}

	// Check if what's before the first paren looks like a macro name
	// (all uppercase or contains underscore, no spaces)
	macroPart := line[:firstParen]
	if !isMacroName(macroPart) {
		return false
	}

	// Find the matching closing paren for the macro
	depth := 1
	macroEnd := firstParen + 1
	for macroEnd < len(line) && depth > 0 {
		if line[macroEnd] == '(' {
			depth++
		} else if line[macroEnd] == ')' {
			depth--
		}
		macroEnd++
	}

	if depth != 0 {
		return false
	}

	// Extract return type from inside macro parentheses
	returnType := normaliseCReturnType(line[firstParen+1 : macroEnd-1])
	if returnType == "" {
		return false
	}

	// Now parse the rest as: function_name(params);
	rest := strings.TrimSpace(line[macroEnd:])

	// Find function name and parameters
	funcParen := strings.Index(rest, "(")
	if funcParen == -1 {
		return false
	}

	funcName, stars := splitCFuncName(strings.TrimSpace(rest[:funcParen]))
	if funcName == "" {
		return false
	}
	if stars != "" {
		returnType = strings.TrimSpace(returnType + " " + stars)
	}

	// Extract parameters
	endParen := strings.LastIndex(rest, ")")
	if endParen == -1 {
		return false
	}

	paramStr := rest[funcParen+1 : endParen]
	params := parseParameters(paramStr)

	info.Functions[funcName] = &CFunction{
		Name:       funcName,
		ReturnType: returnType,
		Parameters: params,
		Line:       lineNum,
		File:       filePath,
	}

	return true
}

// isMacroName checks if a string looks like a C macro name
// (typically all uppercase or contains underscores, no spaces)
func isMacroName(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}

	// Check for spaces (macros shouldn't have spaces)
	if strings.Contains(s, " ") {
		return false
	}

	// Must contain at least one uppercase letter or underscore
	hasUpperOrUnderscore := false
	for _, ch := range s {
		if ch == '_' || (ch >= 'A' && ch <= 'Z') {
			hasUpperOrUnderscore = true
			break
		}
	}

	return hasUpperOrUnderscore
}

// parseParameters parses function parameters
func parseParameters(paramStr string) []CParameter {
	var params []CParameter

	if strings.TrimSpace(paramStr) == "" || strings.TrimSpace(paramStr) == "void" {
		return params
	}

	// Split by comma
	parts := strings.Split(paramStr, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Extract parameter name and type
		tokens := strings.Fields(part)
		if len(tokens) == 0 {
			continue
		}

		var paramName, paramType string
		if len(tokens) == 1 {
			paramType = tokens[0]
			paramName = ""
		} else {
			paramName = tokens[len(tokens)-1]
			// Check if param name starts with * (pointer declaration like "const void *value")
			if strings.HasPrefix(paramName, "*") {
				// Move * to the type
				paramType = strings.Join(tokens[:len(tokens)-1], " ") + " *"
				paramName = strings.TrimPrefix(paramName, "*")
			} else {
				paramType = strings.Join(tokens[:len(tokens)-1], " ")
			}
		}

		params = append(params, CParameter{
			Name: paramName,
			Type: paramType,
		})
	}

	return params
}

// parseDefine parses #define constants
func parseDefine(line string, lineNum int, filePath string, info *CHeaderInfo) {
	if strings.Contains(line, "RAYLIB_VERSION") || strings.Contains(line, "__declspec") {
		return
	}

	parts := strings.Fields(line)
	if len(parts) < 3 {
		return
	}

	name := parts[1]
	value := strings.Join(parts[2:], " ")

	if strings.Contains(value, "CLITERAL(Color)") || strings.Contains(value, "Color{") {
		info.Defines[name] = &CDefine{
			Name:  name,
			Value: value,
			Line:  lineNum,
			File:  filePath,
		}
	}
}

// parseStruct parses typedef struct definitions
func parseStruct(lines []string, startIdx int, filePath string, info *CHeaderInfo) {
	line := lines[startIdx]

	// parseStruct only runs on `typedef struct` lines, so a closing alias
	// is a real typedef name (not a plain-struct tag).
	isTypedef := strings.HasPrefix(strings.TrimSpace(line), "typedef")

	var structName string
	if strings.Contains(line, "{") {
		parts := strings.Fields(strings.Replace(line, "{", "", 1))
		if len(parts) >= 3 {
			structName = parts[2]
		}
	}

	var fields []CStructField
	for i := startIdx + 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])

		if strings.HasPrefix(line, "}") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				alias := strings.TrimSuffix(parts[1], ";")
				if structName == "" {
					structName = alias
				}
				// `typedef struct Tag {...} Alias;`: record the alias.
				// Without this, typedef'd structs (Model, Font, Vector3
				// in raylib.h) look like plain structs downstream and
				// mapType emits `struct Tag`, breaking every bare-name
				// registry comparison (struct array push/access degrades
				// to int casts). True plain structs (`struct stat`, no
				// typedef) keep the prefix, correctly.
				if isTypedef && alias != "" && info.Typedefs != nil {
					if _, exists := info.Typedefs[alias]; !exists {
						info.Typedefs[alias] = &CTypedef{
							AliasName: alias,
							BaseType:  structName,
							Line:      i + 1,
							File:      filePath,
						}
					}
				}
			}
			break
		}

		if line != "" && !strings.HasPrefix(line, "//") {
			parseStructField(line, &fields)
		}
	}

	if structName != "" {
		info.Structs[structName] = &CStruct{
			Name:   structName,
			Fields: fields,
			Line:   startIdx + 1,
			File:   filePath,
		}
	}
}

// parsePlainStruct parses `struct NAME { ... };` without typedef (system
// headers: `struct dirent`, `struct stat`). Brace may sit on the next line.
func parsePlainStruct(lines []string, startIdx int, filePath string, info *CHeaderInfo) {
	line := strings.TrimSpace(lines[startIdx])
	// `struct NAME`, `struct NAME {`, or `struct NAME __attr {` — name is the
	// first identifier after `struct`.
	rest := strings.TrimSpace(strings.TrimPrefix(line, "struct "))
	if rest == "" {
		return
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return
	}
	structName := fields[0]
	// Skip forward declarations (`struct foo;`) and function params.
	if strings.Contains(rest, ";") && !strings.Contains(rest, "{") {
		return
	}
	structName = strings.Trim(structName, "{")
	if structName == "" || strings.Contains(structName, "(") {
		return
	}
	if _, ok := info.Structs[structName]; ok {
		return
	}
	var collected []CStructField
	// Start after the opening brace (same line or a later one).
	started := strings.Contains(line, "{")
	for i := startIdx + 1; i < len(lines) && i < startIdx+80; i++ {
		l := strings.TrimSpace(lines[i])
		if !started {
			if strings.Contains(l, "{") {
				started = true
			}
			continue
		}
		if strings.HasPrefix(l, "}") {
			break
		}
		if l == "" || strings.HasPrefix(l, "//") || strings.HasPrefix(l, "#") {
			continue
		}
		// Skip nested struct/union openings; their fields still parse as lines.
		if strings.HasPrefix(l, "struct ") || strings.HasPrefix(l, "union ") {
			continue
		}
		parseStructField(l, &collected)
	}
	if len(collected) == 0 {
		return
	}
	info.Structs[structName] = &CStruct{
		Name:   structName,
		Fields: collected,
		Line:   startIdx + 1,
		File:   filePath,
	}
}

// parseStructField parses a struct field line
func parseStructField(line string, fields *[]CStructField) {
	if idx := strings.Index(line, "//"); idx != -1 {
		line = line[:idx]
	}
	// Strip block comments (`/* ... */`): system headers annotate array
	// sizes (`char d_name[256]; /* ... */`) and the tail would otherwise
	// register as the field name.
	for {
		s := strings.Index(line, "/*")
		e := strings.Index(line, "*/")
		if s == -1 || e == -1 || e < s {
			break
		}
		line = line[:s] + " " + line[e+2:]
	}
	line = strings.TrimSpace(line)
	line = strings.TrimSuffix(line, ";")

	parts := strings.Fields(line)
	if len(parts) >= 2 {
		fieldType := strings.Join(parts[:len(parts)-1], " ")
		fieldName := parts[len(parts)-1]
		// Leading stars belong to the type (`char **paths` or `char *paths`):
		// without this the field registers as `**paths` and member typing
		// (char** indexing, dirent/stat reads) cannot resolve it.
		stars := 0
		for strings.HasPrefix(fieldName, "*") {
			stars++
			fieldName = strings.TrimPrefix(fieldName, "*")
		}
		if stars > 0 {
			fieldType = strings.TrimSpace(fieldType) + strings.Repeat("*", stars)
		}
		// Trailing array size belongs to the type (`char d_name[256]`):
		// keep the bare name so `e.d_name` resolves, and record the
		// array shape for direct indexing.
		if idx := strings.Index(fieldName, "["); idx != -1 {
			arrSuffix := fieldName[idx:]
			fieldName = fieldName[:idx]
			fieldType = strings.TrimSpace(fieldType) + arrSuffix
		}

		*fields = append(*fields, CStructField{
			Name: fieldName,
			Type: fieldType,
		})
	}
}

// parseEnum parses typedef enum definitions
func parseEnum(lines []string, startIdx int, filePath string, info *CHeaderInfo) {
	var enumName string
	values := make(map[string]int)
	valueLines := make(map[string]int)
	currentValue := 0

	for i := startIdx + 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])

		if strings.HasPrefix(line, "}") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				enumName = strings.TrimSuffix(parts[1], ";")
			}
			break
		}

		if line != "" && !strings.HasPrefix(line, "//") {
			parseEnumValue(line, &values, &valueLines, &currentValue, i+1)
		}
	}

	if enumName != "" {
		info.Enums[enumName] = &CEnum{
			Name:       enumName,
			Values:     values,
			ValueLines: valueLines,
			Line:       startIdx + 1,
			File:       filePath,
		}
	}
}

// parseEnumValue parses an enum value line
func parseEnumValue(line string, values *map[string]int, valueLines *map[string]int, currentValue *int, lineNum int) {
	if idx := strings.Index(line, "//"); idx != -1 {
		line = line[:idx]
	}
	line = strings.TrimSpace(line)
	line = strings.TrimSuffix(line, ",")

	parts := strings.Split(line, "=")
	name := strings.TrimSpace(parts[0])

	if name != "" {
		(*valueLines)[name] = lineNum

		if len(parts) > 1 {
			valueStr := strings.TrimSpace(parts[1])
			// Try parsing as int (supports both decimal and hex with 0x prefix)
			if val, err := strconv.ParseInt(valueStr, 0, 64); err == nil {
				(*values)[name] = int(val)
				*currentValue = int(val) + 1
			}
		} else {
			(*values)[name] = *currentValue
			*currentValue++
		}
	}
}

// Helper functions for case conversion
func ToLowerFirst(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func PascalToSnake(s string) string {
	// Optimized for ASCII-only input (C function names)
	n := len(s)
	if n == 0 {
		return s
	}

	// Pre-allocate with extra space for underscores
	result := make([]byte, 0, n+n/2)

	for i := 0; i < n; i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			// Add underscore before uppercase if not at start
			if i > 0 {
				prev := s[i-1]
				prevIsLower := prev >= 'a' && prev <= 'z'

				// Check if we're transitioning from acronym to word
				// Example: "HTTPServer" -> at 'S', prev is 'P' (upper), next is 'e' (lower)
				transitionFromAcronym := false
				if i < n-1 {
					prevIsUpper := prev >= 'A' && prev <= 'Z'
					next := s[i+1]
					nextIsLower := next >= 'a' && next <= 'z'
					if prevIsUpper && nextIsLower {
						transitionFromAcronym = true
					}
				}

				if prevIsLower || transitionFromAcronym {
					result = append(result, '_')
				}
			}
			result = append(result, c+32) // ASCII lowercase
		} else {
			result = append(result, c)
		}
	}

	return string(result)
}

// parseFuncPtrTypedef parses a C callback typedef:
//
//	typedef void (*TraceLogCallback)(int logLevel, const char *text);
//
// The name and signature are kept so a call to an imported function that takes
// this parameter type can pass a raw C function pointer.
func parseFuncPtrTypedef(line string, lineNum int, filePath string, info *CHeaderInfo) {
	open := strings.Index(line, "(*")
	if open < 0 {
		return
	}
	rest := line[open+2:]
	closeIdx := strings.Index(rest, ")")
	if closeIdx < 0 {
		return
	}
	name := strings.TrimSpace(rest[:closeIdx])
	if name == "" {
		return
	}

	// Return type is what sits between `typedef` and the `(*`.
	ret := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "typedef"))
	if paren := strings.Index(ret, "("); paren >= 0 {
		ret = strings.TrimSpace(ret[:paren])
	}
	// Normalise pointer spacing so "char *" is the "char*" the type checks expect
	ret = strings.ReplaceAll(ret, " *", "*")

	rest2 := rest[closeIdx+1:]
	lp := strings.Index(rest2, "(")
	rp := strings.LastIndex(rest2, ")")
	if lp < 0 || rp < lp {
		return
	}
	params := parseParameters(rest2[lp+1 : rp])

	if info.FuncPtrTypedefs == nil {
		info.FuncPtrTypedefs = make(map[string]*CFuncPtrTypedef)
	}
	info.FuncPtrTypedefs[name] = &CFuncPtrTypedef{
		Name:       name,
		ReturnType: ret,
		Parameters: params,
		Line:       lineNum,
		File:       filePath,
	}

	// Treat the name as a known C type as well, so a variable can be declared
	// with it (the code generator registers struct names as types).
	if info.Structs[name] == nil {
		info.Structs[name] = &CStruct{Name: name, Fields: []CStructField{}, Line: lineNum, File: filePath}
	}
}

// parseTypedefAlias parses simple typedef aliases like: typedef Texture Texture2D;
func parseTypedefAlias(line string, lineNum int, filePath string, info *CHeaderInfo) {
	// Remove typedef and semicolon
	line = strings.TrimPrefix(line, "typedef")
	line = strings.TrimSuffix(line, ";")
	line = strings.TrimSpace(line)

	// Split into base type and alias name
	parts := strings.Fields(line)
	if len(parts) == 2 {
		baseType := parts[0]
		aliasName := parts[1]

		// Store the alias -> typedef info with line and file
		if info.Typedefs != nil {
			info.Typedefs[aliasName] = &CTypedef{
				AliasName: aliasName,
				BaseType:  baseType,
				Line:      lineNum,
				File:      filePath,
			}
		}

		// Store as a struct entry so it's treated as a known C type
		// We don't need the full struct definition, just the name
		info.Structs[aliasName] = &CStruct{
			Name:   aliasName,
			Fields: []CStructField{},
			Line:   0,
		}

		// If the base type is also a struct/typedef, copy it
		if baseStruct, exists := info.Structs[baseType]; exists {
			info.Structs[aliasName] = &CStruct{
				Name:   aliasName,
				Fields: baseStruct.Fields,
				Line:   0,
			}
		}
	}
}
