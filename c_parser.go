package ahoy

import (
	"os"
	"strconv"
	"strings"
	"unicode"
)

// ParseCHeader parses a C header file and extracts function signatures, enums, defines, and structs
func ParseCHeader(path string) (*CHeaderInfo, error) {
	info := &CHeaderInfo{
		Functions: make(map[string]*CFunction),
		Enums:     make(map[string]*CEnum),
		Defines:   make(map[string]*CDefine),
		Structs:   make(map[string]*CStruct),
	}
	
	// Read the header file
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	
	lines := strings.Split(string(content), "\n")
	
	// Parse line by line
	for i, line := range lines {
		line = strings.TrimSpace(line)
		lineNum := i + 1
		
		// Parse RLAPI/RMAPI function declarations
		if strings.Contains(line, "RLAPI") || strings.Contains(line, "RMAPI") {
			parseRLAPIFunction(line, lineNum, info)
		} else if parseMacroFunction(line, lineNum, info) {
			// Successfully parsed as macro function (like CJSON_PUBLIC)
		} else if strings.Contains(line, "(") && strings.Contains(line, ");") && !strings.HasPrefix(line, "typedef") && !strings.HasPrefix(line, "#") {
			// Try to parse as generic function declaration
			parseRLAPIFunction(line, lineNum, info)
		}
		
		// Parse #define constants
		if strings.HasPrefix(line, "#define") {
			parseDefine(line, lineNum, info)
		}
		
		// Parse typedef struct
		if strings.HasPrefix(line, "typedef struct") {
			parseStruct(lines, i, info)
		}
		
		// Parse typedef enum
		if strings.HasPrefix(line, "typedef enum") {
			parseEnum(lines, i, info)
		}
		
		// Parse simple typedef aliases (e.g., typedef Texture Texture2D;)
		if strings.HasPrefix(line, "typedef ") && !strings.Contains(line, "{") && strings.HasSuffix(line, ";") {
			parseTypedefAlias(line, info)
		}
	}
	
	return info, nil
}

// parseRLAPIFunction parses a function declaration like: RLAPI void InitWindow(int width, int height, const char *title);
func parseRLAPIFunction(line string, lineNum int, info *CHeaderInfo) {
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
	
	funcName := parts[len(parts)-1]
	returnType := strings.Join(parts[:len(parts)-1], " ")
	
	// Extract parameters
	endParen := strings.Index(line, ")")
	if endParen == -1 {
		return
	}
	
	paramStr := line[parenIdx+1 : endParen]
	params := parseParameters(paramStr)
	
	info.Functions[funcName] = &CFunction{
		Name:       funcName,
		ReturnType: returnType,
		Parameters: params,
		Line:       lineNum,
	}
}

// parseMacroFunction parses function declarations with macro prefixes
// Examples: CJSON_PUBLIC(cJSON *) cJSON_Parse(const char *value);
//           SOME_MACRO(void) MyFunction(int x);
func parseMacroFunction(line string, lineNum int, info *CHeaderInfo) bool {
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
	returnType := strings.TrimSpace(line[firstParen+1 : macroEnd-1])
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
	
	funcName := strings.TrimSpace(rest[:funcParen])
	if funcName == "" {
		return false
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
			paramName = strings.TrimPrefix(paramName, "*")
			paramType = strings.Join(tokens[:len(tokens)-1], " ")
		}
		
		params = append(params, CParameter{
			Name: paramName,
			Type: paramType,
		})
	}
	
	return params
}

// parseDefine parses #define constants
func parseDefine(line string, lineNum int, info *CHeaderInfo) {
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
		}
	}
}

// parseStruct parses typedef struct definitions
func parseStruct(lines []string, startIdx int, info *CHeaderInfo) {
	line := lines[startIdx]
	
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
			if len(parts) >= 2 && structName == "" {
				structName = strings.TrimSuffix(parts[1], ";")
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
		}
	}
}

// parseStructField parses a struct field line
func parseStructField(line string, fields *[]CStructField) {
	if idx := strings.Index(line, "//"); idx != -1 {
		line = line[:idx]
	}
	line = strings.TrimSpace(line)
	line = strings.TrimSuffix(line, ";")
	
	parts := strings.Fields(line)
	if len(parts) >= 2 {
		fieldType := strings.Join(parts[:len(parts)-1], " ")
		fieldName := parts[len(parts)-1]
		
		*fields = append(*fields, CStructField{
			Name: fieldName,
			Type: fieldType,
		})
	}
}

// parseEnum parses typedef enum definitions
func parseEnum(lines []string, startIdx int, info *CHeaderInfo) {
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
	var result []rune
	runes := []rune(s)
	
	for i := 0; i < len(runes); i++ {
		if unicode.IsUpper(runes[i]) {
			// Add underscore before uppercase if not at start
			if i > 0 {
				// Check if previous was lowercase (normal case: myVariable)
				prevIsLower := unicode.IsLower(runes[i-1])
				
				// Check if we're transitioning from acronym to word
				// Example: "HTTPServer" -> at 'S', prev is 'P' (upper), next is 'e' (lower)
				transitionFromAcronym := false
				if i > 0 && i < len(runes)-1 {
					prevIsUpper := unicode.IsUpper(runes[i-1])
					nextIsLower := unicode.IsLower(runes[i+1])
					if prevIsUpper && nextIsLower {
						transitionFromAcronym = true
					}
				}
				
				// Add underscore if transitioning from lowercase or from acronym to word
				if prevIsLower || transitionFromAcronym {
					result = append(result, '_')
				}
			}
			result = append(result, unicode.ToLower(runes[i]))
		} else {
			result = append(result, runes[i])
		}
	}
	
	return string(result)
}

// parseTypedefAlias parses simple typedef aliases like: typedef Texture Texture2D;
func parseTypedefAlias(line string, info *CHeaderInfo) {
	// Remove typedef and semicolon
	line = strings.TrimPrefix(line, "typedef")
	line = strings.TrimSuffix(line, ";")
	line = strings.TrimSpace(line)
	
	// Split into base type and alias name
	parts := strings.Fields(line)
	if len(parts) == 2 {
		baseType := parts[0]
		aliasName := parts[1]
		
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
