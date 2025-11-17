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
		
		// Parse RLAPI function declarations
		if strings.Contains(line, "RLAPI") {
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
	}
	
	return info, nil
}

// parseRLAPIFunction parses a function declaration like: RLAPI void InitWindow(int width, int height, const char *title);
func parseRLAPIFunction(line string, lineNum int, info *CHeaderInfo) {
	// Remove RLAPI prefix and comments
	line = strings.Replace(line, "RLAPI", "", 1)
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
