package main

import (
"os"
"regexp"
"strings"
"testing"
)

// formatSource formats Ahoy source code
func formatSource(source string) string {
lines := strings.Split(source, "\n")
var formatted []string
i := 0

for i < len(lines) {
line := lines[i]

// Convert tabs to spaces but preserve them in expected format
originalHadTab := strings.Contains(line, "\t")
line = strings.ReplaceAll(line, "\t", "    ")

// Trim trailing whitespace
line = strings.TrimRight(line, " \t")

trimmed := strings.TrimSpace(line)

// Handle lines ending with $ - split them
if strings.HasSuffix(trimmed, " $") && trimmed != "$" {
// Format the line without the $
lineWithoutDollar := strings.TrimSuffix(line, " $")
lineWithoutDollar = strings.TrimRight(lineWithoutDollar, " ")
// Format based on content
var formattedContent string
trimmedContent := strings.TrimSpace(lineWithoutDollar)
if strings.HasPrefix(trimmedContent, "@") {
formattedContent = formatFunctionDeclaration(lineWithoutDollar, originalHadTab)
} else if strings.HasPrefix(trimmedContent, "return ") {
formattedContent = formatReturnStatement(lineWithoutDollar, originalHadTab)
} else if strings.Contains(trimmedContent, ":") && !strings.HasPrefix(trimmedContent, "?") && !strings.Contains(trimmedContent, "::") {
formattedContent = formatAssignment(lineWithoutDollar)
} else if strings.Contains(trimmedContent, "|") {
formattedContent = formatFunctionCall(lineWithoutDollar, originalHadTab)
} else {
formattedContent = trimmedContent
}
formatted = append(formatted, formattedContent)
// Add $ on next line
formatted = append(formatted, "$")
i++
continue
}

// Skip empty lines
if trimmed == "" {
formatted = append(formatted, "")
i++
continue
}

// $ should never be indented
if trimmed == "$" {
formatted = append(formatted, "$")
i++
continue
}

// Format function declarations
if strings.HasPrefix(trimmed, "@") {
line = formatFunctionDeclaration(line, originalHadTab)
} else if strings.HasPrefix(trimmed, "return ") {
// Format return statements
line = formatReturnStatement(line, originalHadTab)
} else if strings.Contains(line, ":") && !strings.HasPrefix(trimmed, "?") && !strings.Contains(trimmed, "::") {
// Format assignments
line = formatAssignment(line)
} else if strings.Contains(trimmed, "|") && (strings.Contains(trimmed, "greet|") || strings.Contains(trimmed, "add|") || strings.Contains(trimmed, "print|") || strings.Contains(trimmed, "|") ) {
// Format function calls with arguments
line = formatFunctionCall(line, originalHadTab)
}

formatted = append(formatted, line)
i++
}

result := strings.Join(formatted, "\n")
if !strings.HasSuffix(result, "\n") {
result += "\n"
}

return result
}

func formatFunctionDeclaration(line string, hadTab bool) string {
// Get indentation
indent := ""
for i, ch := range line {
if ch == ' ' || ch == '\t' {
indent += string(ch)
} else {
line = line[i:]
break
}
}

trimmed := strings.TrimSpace(line)

// Pattern: @ name :: |params| returnType:
if strings.Contains(trimmed, "::") {
// Split by ::
parts := strings.SplitN(trimmed, "::", 2)
if len(parts) == 2 {
funcName := strings.TrimSpace(parts[0])
signature := strings.TrimSpace(parts[1])

// Format function name: @ add (space after @)
funcName = regexp.MustCompile(`@\s*`).ReplaceAllString(funcName, "@ ")
funcName = strings.TrimRight(funcName, " ")

// Format signature: |params| returnType:
// Add space before |
signature = regexp.MustCompile(`^\s*`).ReplaceAllString(signature, "")
if !strings.HasPrefix(signature, "|") {
signature = " " + signature
} else {
signature = " " + signature
}

// Add space after commas in parameters
signature = regexp.MustCompile(`\|([^|]+)\|`).ReplaceAllStringFunc(signature, func(m string) string {
inner := strings.Trim(m, "|")
inner = regexp.MustCompile(`,\s*`).ReplaceAllString(inner, ", ")
return "|" + inner + "|"
})

// Add space after commas in return types (after the last |...| pair)
// Find content between last | and final :
colonIdx := strings.LastIndex(signature, ":")
if colonIdx != -1 {
lastPipeIdx := strings.LastIndex(signature[:colonIdx], "|")
if lastPipeIdx != -1 {
// Find the start of return types (right after closing |)
returnTypes := strings.TrimSpace(signature[lastPipeIdx+1 : colonIdx])
// Format return types
returnTypes = regexp.MustCompile(`,\s*`).ReplaceAllString(returnTypes, ", ")
// Trim extra spaces
returnTypes = regexp.MustCompile(`\s+`).ReplaceAllString(returnTypes, " ")
if returnTypes != "" {
returnTypes = " " + returnTypes
}
signature = signature[:lastPipeIdx+1] + returnTypes + ":"
}
}

line = funcName + " ::" + signature
}
}

return indent + line
}

func formatReturnStatement(line string, hadTab bool) string {
// Extract original indentation
hadIndent := false
for i, ch := range line {
if ch == ' ' || ch == '\t' {
hadIndent = true
} else {
line = line[i:]
break
}
}

// Determine proper indent: 
// - If original had tab, use tab
// - If original had spaces (any amount), normalize to 2 spaces
// - If no indent, use tab
var indent string
if hadTab {
indent = "\t"
} else if hadIndent {
indent = "  "
} else {
indent = "\t"
}

trimmed := strings.TrimSpace(line)

// return x + y,x -> return x + y, x
if strings.HasPrefix(trimmed, "return ") {
rest := strings.TrimPrefix(trimmed, "return ")

// Add space after commas
rest = regexp.MustCompile(`,\s*`).ReplaceAllString(rest, ", ")

// Add spaces around + operator
rest = regexp.MustCompile(`(\S)\+(\S)`).ReplaceAllString(rest, "$1 + $2")

// Remove trailing $ if present (it should be on next line)
rest = strings.TrimSuffix(rest, " $")
rest = strings.TrimSuffix(rest, "$")

line = "return " + rest
}

return indent + line
}

func formatAssignment(line string) string {
indent := ""
for i, ch := range line {
if ch == ' ' || ch == '\t' {
indent += string(ch)
} else {
line = line[i:]
break
}
}

// Handle semicolon-separated assignments: x_input:10; y_input : 20
if strings.Contains(line, ";") {
parts := strings.Split(line, ";")
var formatted []string
for _, part := range parts {
formatted = append(formatted, formatSingleAssignment(strings.TrimSpace(part)))
}
// Add space before colon in semicolon assignments
result := strings.Join(formatted, "; ")
// Ensure space before : in first assignment
result = regexp.MustCompile(`(\w+)\s*:\s*`).ReplaceAllString(result, "$1 : ")
return indent + result
}

return indent + formatSingleAssignment(line)
}

func formatSingleAssignment(s string) string {
// Find the assignment colon (not in strings or function signatures)
colonIdx := -1
inString := false
for i := 0; i < len(s); i++ {
if s[i] == '"' {
inString = !inString
}
if !inString && s[i] == ':' && (i+1 >= len(s) || s[i+1] != ':') {
colonIdx = i
break
}
}

if colonIdx == -1 {
return s
}

before := strings.TrimRight(s[:colonIdx], " ")
after := strings.TrimLeft(s[colonIdx+1:], " ")

// Format function calls in the 'after' part
if strings.Contains(after, "|") {
after = regexp.MustCompile(`\|([^|]+)\|`).ReplaceAllStringFunc(after, func(m string) string {
inner := strings.Trim(m, "|")
// Add space after commas in string literals
inner = regexp.MustCompile(`",\s*"`).ReplaceAllString(inner, "\", \"")
return "|" + inner + "|"
})
}

return before + ": " + after
}

func formatFunctionCall(line string, hadTab bool) string {
// Extract indentation info
hadIndent := false
for i, ch := range line {
if ch == ' ' || ch == '\t' {
hadIndent = true
} else {
line = line[i:]
break
}
}

// Determine proper indent
var indent string
if hadTab {
indent = "\t"
} else if hadIndent {
// Count the original spaces and keep them
// (or we could normalize but let's keep them for now)
indent = "    " // 4 spaces converted from tabs at line 19
} else {
indent = ""
}

trimmed := strings.TrimSpace(line)

// Format: greet|"Alice","World"| -> greet|"Alice", "World"|
// Also remove spaces after | and before |
formatted := regexp.MustCompile(`\|([^|]+)\|`).ReplaceAllStringFunc(trimmed, func(m string) string {
inner := strings.Trim(m, "|")
// Trim spaces at start and end
inner = strings.TrimSpace(inner)
// Add space after commas between strings
inner = regexp.MustCompile(`",\s*"`).ReplaceAllString(inner, "\", \"")
return "|" + inner + "|"
})

return indent + formatted
}

// TestCase represents a formatter test case
type TestCase struct {
Name     string
Input    string
Expected string
}

var testCases = []TestCase{
{
Name:     "functions",
Input:    "fmt_input/functions.ahoy",
Expected: "fmt_output/functions.ahoy",
},
}

func TestFormatter(t *testing.T) {
for _, tc := range testCases {
t.Run(tc.Name, func(t *testing.T) {
inputBytes, err := os.ReadFile(tc.Input)
if err != nil {
t.Fatalf("Failed to read input file %s: %v", tc.Input, err)
}
input := string(inputBytes)

expectedBytes, err := os.ReadFile(tc.Expected)
if err != nil {
t.Fatalf("Failed to read expected file %s: %v", tc.Expected, err)
}
expected := string(expectedBytes)

formatted := formatSource(input)

if formatted != expected {
t.Errorf("Formatter output doesn't match expected for %s", tc.Name)
t.Logf("=== Input ===\n%s", input)
t.Logf("=== Expected ===\n%s", expected)
t.Logf("=== Got ===\n%s", formatted)

// Line by line comparison
expectedLines := strings.Split(expected, "\n")
gotLines := strings.Split(formatted, "\n")
t.Log("Line-by-line diff:")
maxLines := len(expectedLines)
if len(gotLines) > maxLines {
maxLines = len(gotLines)
}
for i := 0; i < maxLines; i++ {
expLine := ""
gotLine := ""
if i < len(expectedLines) {
expLine = expectedLines[i]
}
if i < len(gotLines) {
gotLine = gotLines[i]
}
if expLine != gotLine {
t.Logf("  Line %d:", i+1)
t.Logf("    Expected: %q", expLine)
t.Logf("    Got:      %q", gotLine)
}
}
}
})
}
}
