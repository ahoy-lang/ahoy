package main

import (
	"regexp"
	"strings"
)

const INDENT_SIZE = 2

// formatSource formats Ahoy source code with proper indentation
func formatSource(source string) string {
	// First, preprocess to split lines with $ at the end
	source = preprocessDollarSigns(source)

	lines := strings.Split(source, "\n")
	var formatted []string
	indentLevel := 0
	structStack := []int{} // Stack to track struct indent levels

	for _, line := range lines {
		// Convert tabs to spaces initially for processing
		line = strings.ReplaceAll(line, "\t", "    ")

		// Trim trailing whitespace
		line = strings.TrimRight(line, " \t")

		trimmed := strings.TrimSpace(line)

		// Skip empty lines but still process them
		if trimmed == "" {
			formatted = append(formatted, "")
			continue
		}

		// Check if this line should decrease indent ($, ⚓, else, elseif)
		shouldDedentBefore := false
		if trimmed == "$" || trimmed == "⚓" || strings.HasPrefix(trimmed, "else ") ||
			trimmed == "else" || strings.HasPrefix(trimmed, "elseif ") {
			shouldDedentBefore = true
		}

		// Dedent before adding the line
		if shouldDedentBefore && indentLevel > 0 {
			// Special case: if this is '$' or '⚓' and we have a struct on the stack,
			// dedent all the way back to the struct level
			if (trimmed == "$" || trimmed == "⚓") && len(structStack) > 0 {
				// Check if we're closing a struct (dedent to struct's parent level)
				structLevel := structStack[len(structStack)-1]
				if indentLevel > structLevel {
					// We're inside a struct with type variants
					// Dedent all the way to the struct level
					indentLevel = structLevel
					structStack = structStack[:len(structStack)-1] // Pop the struct
				} else {
					indentLevel--
				}
			} else {
				indentLevel--
			}
		}

		// Check if this is a single-line construct (should NOT be indented on next line)
		isSingleLine := isSingleLineConstruct(trimmed)

		// Format the line content (spacing, etc.)
		trimmed = formatLine(trimmed)

		// Apply current indentation
		// Use spaces (2 per level) or tab (for level 1 only in some cases)
		var indent string
		if indentLevel > 0 {
			// Use 2 spaces per indent level
			indent = strings.Repeat(" ", indentLevel*INDENT_SIZE)
		}
		formattedLine := indent + trimmed
		formatted = append(formatted, formattedLine)

		// Check if we should increase indent after this line (skip for comments)
		if !strings.HasPrefix(trimmed, "?") {
			// For else/elseif, shouldIncreaseIndent already handles the indent increase
			// (elseif ends with "then", else is followed by a body)
			// So we don't need the special case logic - just use shouldIncreaseIndent
			if !isSingleLine && shouldIncreaseIndent(trimmed) {
				// Track when we enter a struct
				if (strings.HasPrefix(trimmed, "struct ") || strings.HasPrefix(trimmed, "enum ")) &&
					strings.HasSuffix(trimmed, ":") {
					structStack = append(structStack, indentLevel)
				}
				indentLevel++
			}
		}
	}

	result := strings.Join(formatted, "\n")

	// Ensure file ends with newline
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}

	return result
}

// preprocessDollarSigns splits lines ending with $ onto separate lines
func preprocessDollarSigns(source string) string {
	lines := strings.Split(source, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// If line ends with $ and has content before it, split
		if strings.HasSuffix(trimmed, " $") && trimmed != "$" {
			contentPart := strings.TrimSuffix(trimmed, " $")
			contentPart = strings.TrimSpace(contentPart)
			result = append(result, contentPart)
			result = append(result, "$")
		} else {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// 	if !strings.HasSuffix(result, "\n") {
// 		result += "\n"
// 	}

// 	return result
// }

// formatLine applies formatting rules to a single line
func formatLine(line string) string {
	// Skip comments and empty lines
	if strings.HasPrefix(line, "?") || line == "" {
		return line
	}

	// Handle function definitions with ::
	if strings.Contains(line, "::") {
		line = formatFunctionDefinition(line)
	}

	// Handle function calls - add space after commas in arguments
	line = formatFunctionCalls(line)

	// Add spaces around binary operators
	line = formatOperators(line)

	// Normalize spacing around colons in type annotations
	line = formatTypeAnnotations(line)

	return line
}

// formatFunctionDefinition formats function definition lines
func formatFunctionDefinition(line string) string {
	// Pattern: @ name :: |params| return:
	// Remove extra spaces around ::
	line = regexp.MustCompile(`\s*::\s*`).ReplaceAllString(line, " :: ")

	// Remove space immediately after opening | in parameters
	// This handles | x:int -> |x:int
	line = regexp.MustCompile(`\|\s+([a-zA-Z_])`).ReplaceAllString(line, "|$1")

	// Add space after commas in parameter list (between pipes)
	line = formatParameterList(line)

	// Add space after commas in return types
	line = formatReturnTypes(line)

	return line
}

// formatParameterList adds space after commas in parameter lists
func formatParameterList(line string) string {
	// Find content between | |
	re := regexp.MustCompile(`\|([^|]+)\|`)
	return re.ReplaceAllStringFunc(line, func(match string) string {
		// Remove the pipes temporarily
		content := strings.Trim(match, "|")
		// Add space after commas
		content = regexp.MustCompile(`,\s*`).ReplaceAllString(content, ", ")
		return "|" + content + "|"
	})
}

// formatReturnTypes adds space after commas in return type lists
func formatReturnTypes(line string) string {
	// Pattern: |params| type1,type2:
	// Find return types between last | and final :
	if !strings.Contains(line, "::") {
		return line
	}

	// Split on ::
	parts := strings.SplitN(line, "::", 2)
	if len(parts) != 2 {
		return line
	}

	rightPart := parts[1]

	// Find the last | and the final :
	lastPipe := strings.LastIndex(rightPart, "|")
	lastColon := strings.LastIndex(rightPart, ":")

	if lastPipe != -1 && lastColon != -1 && lastColon > lastPipe {
		// Extract return types between | and :
		returnTypes := rightPart[lastPipe+1 : lastColon]
		// Format commas
		returnTypes = regexp.MustCompile(`,\s*`).ReplaceAllString(returnTypes, ", ")
		// Reconstruct
		rightPart = rightPart[:lastPipe+1] + returnTypes + rightPart[lastColon:]
	}

	return parts[0] + "::" + rightPart
}

// formatFunctionCalls adds space after commas in function calls
func formatFunctionCalls(line string) string {
	// Remove space before | in function calls: print| -> print|
	line = regexp.MustCompile(`(\w+)\s+\|`).ReplaceAllString(line, "$1|")

	// Pattern: func|arg1,arg2|
	// Find all function calls (text followed by |...|)
	re := regexp.MustCompile(`(\w+)\|([^|]+)\|`)
	return re.ReplaceAllStringFunc(line, func(match string) string {
		// Extract function name and args
		parts := regexp.MustCompile(`(\w+)\|([^|]+)\|`).FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		funcName := parts[1]
		args := parts[2]

		// Don't add spaces inside string literals
		// Simple approach: format commas that are not inside quotes
		args = formatCommasOutsideStrings(args)

		return funcName + "|" + args + "|"
	})
}

// formatCommasOutsideStrings adds space after commas outside of string literals
func formatCommasOutsideStrings(s string) string {
	var result strings.Builder
	inString := false
	escapeNext := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if escapeNext {
			result.WriteByte(ch)
			escapeNext = false
			continue
		}

		if ch == '\\' {
			result.WriteByte(ch)
			escapeNext = true
			continue
		}

		if ch == '"' {
			inString = !inString
			result.WriteByte(ch)
			continue
		}

		if ch == ',' && !inString {
			result.WriteByte(',')
			// Skip any existing spaces
			for i+1 < len(s) && s[i+1] == ' ' {
				i++
			}
			result.WriteByte(' ')
			continue
		}

		result.WriteByte(ch)
	}

	return result.String()
}

// formatOperators adds spaces around binary operators
func formatOperators(line string) string {
	// Add space around + and - operators (but not inside strings or compound operators)
	// Simple pattern - add space around +/- when not in quotes and not part of += or -=
	line = formatOperatorOutsideStrings(line, '+')
	line = formatOperatorOutsideStrings(line, '-')
	return line
}

// formatOperatorOutsideStrings adds spaces around operator outside strings
func formatOperatorOutsideStrings(s string, op byte) string {
	var result strings.Builder
	inString := false
	escapeNext := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if escapeNext {
			result.WriteByte(ch)
			escapeNext = false
			continue
		}

		if ch == '\\' {
			result.WriteByte(ch)
			escapeNext = true
			continue
		}

		if ch == '"' {
			inString = !inString
			result.WriteByte(ch)
			continue
		}

		if ch == op && !inString {
			// Check if this is part of a compound operator (+=, -=, *=, /=, %=)
			// Look ahead past any spaces to see if there's an '='
			if ch == '+' || ch == '-' || ch == '*' || ch == '/' || ch == '%' {
				nextIdx := i + 1
				// Skip spaces
				for nextIdx < len(s) && s[nextIdx] == ' ' {
					nextIdx++
				}
				// If we found '=', this is a compound operator - don't format it
				if nextIdx < len(s) && s[nextIdx] == '=' {
					result.WriteByte(ch)
					continue
				}
			}
			
			// Add spaces around operator
			// Check if there's already a space before
			if result.Len() > 0 {
				lastByte := result.String()[result.Len()-1]
				if lastByte != ' ' {
					result.WriteByte(' ')
				}
			}
			result.WriteByte(ch)
			// Skip any existing spaces after
			for i+1 < len(s) && s[i+1] == ' ' {
				i++
			}
			// Add space after if next char exists and isn't space
			if i+1 < len(s) {
				result.WriteByte(' ')
			}
			continue
		}

		result.WriteByte(ch)
	}

	return result.String()
}

// formatTypeAnnotations normalizes spacing around colons in declarations
func formatTypeAnnotations(line string) string {
	// Pattern: varname:type or varname : type
	// Normalize to: varname : type (space before and after colon)

	// Skip function definitions (contain ::)
	if strings.Contains(line, "::") {
		return line
	}

	// Skip comments
	if strings.HasPrefix(line, "?") {
		return line
	}

	// For variable declarations: name:type or name : type
	// But NOT for the final : in function defs or case labels
	// Pattern: word followed by : followed by word (type)
	line = regexp.MustCompile(`(\w+)\s*:\s*(\w+)`).ReplaceAllString(line, "$1 : $2")

	return line
}

// isSingleLineConstruct checks if a line is a complete single-line construct

// isSingleLineConstruct checks if a line is a complete single-line construct
// Examples: "if x > 0 then ahoy|x|", "loop i to 10 do print|i|"
func isSingleLineConstruct(line string) bool {
	trimmed := strings.TrimSpace(line)

	// Single-line if: has "then" followed by statement on same line
	if (strings.HasPrefix(trimmed, "if ") || strings.HasPrefix(trimmed, "anif ") ||
		strings.HasPrefix(trimmed, "elseif ")) && strings.Contains(trimmed, " then ") {
		// Check if there's a statement after "then" on the same line
		parts := strings.Split(trimmed, " then ")
		if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
			return true
		}
	}

	// Single-line else: "else" followed by statement on same line
	if strings.HasPrefix(trimmed, "else ") {
		afterElse := strings.TrimPrefix(trimmed, "else ")
		if strings.TrimSpace(afterElse) != "" && !strings.HasPrefix(afterElse, "if ") {
			return true
		}
	}

	// Single-line loop: has "do" followed by statement on same line
	if strings.HasPrefix(trimmed, "loop ") && strings.Contains(trimmed, " do ") {
		parts := strings.Split(trimmed, " do ")
		if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
			return true
		}
	}

	// Single-line when: has "then" followed by statement on same line
	if strings.HasPrefix(trimmed, "when ") && strings.Contains(trimmed, " then ") {
		parts := strings.Split(trimmed, " then ")
		if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
			return true
		}
	}

	// Single-line function: has "::" and ":" with content after colon (not ending with colon)
	if strings.Contains(trimmed, "::") && strings.Contains(trimmed, ":") {
		// Check if line ends with content (not just colon)
		if !strings.HasSuffix(trimmed, ":") && !strings.HasSuffix(trimmed, " then") && !strings.HasSuffix(trimmed, " do") {
			return true
		}
	}

	// Single-line enum: "enum Name: value1 value2"
	if strings.HasPrefix(trimmed, "enum ") && strings.Contains(trimmed, ":") {
		colonIdx := strings.Index(trimmed, ":")
		afterColon := strings.TrimSpace(trimmed[colonIdx+1:])
		// If there's content after the colon, it's single-line
		if afterColon != "" {
			return true
		}
	}

	// Single-line struct: "struct Name: field1:type field2:type"
	if strings.HasPrefix(trimmed, "struct ") && strings.Contains(trimmed, ":") {
		colonIdx := strings.Index(trimmed, ":")
		afterColon := strings.TrimSpace(trimmed[colonIdx+1:])
		// If there's content after the colon, it's single-line
		if afterColon != "" {
			return true
		}
	}

	// Single-line switch: "switch x on 1: stmt1 2: stmt2"
	if strings.HasPrefix(trimmed, "switch ") && (strings.Contains(trimmed, " on ") || strings.Contains(trimmed, " then ")) {
		// Find the position after 'on' or 'then'
		var afterKeyword string
		if strings.Contains(trimmed, " on ") {
			parts := strings.Split(trimmed, " on ")
			if len(parts) >= 2 {
				afterKeyword = strings.TrimSpace(parts[1])
			}
		} else if strings.Contains(trimmed, " then ") {
			parts := strings.Split(trimmed, " then ")
			if len(parts) >= 2 {
				afterKeyword = strings.TrimSpace(parts[1])
			}
		}
		// If there's content after 'on'/'then', it's single-line
		if afterKeyword != "" {
			return true
		}
	}

	return false
}

// shouldIncreaseIndent checks if indent should increase after this line
func shouldIncreaseIndent(line string) bool {
	trimmed := strings.TrimSpace(line)

	// Function definition with ::
	if strings.Contains(trimmed, "::") && strings.HasSuffix(trimmed, ":") {
		return true
	}

	// Function definition with func keyword ending with "then" or "do"
	if strings.HasPrefix(trimmed, "func ") && (strings.HasSuffix(trimmed, " then") || strings.HasSuffix(trimmed, " do")) {
		return true
	}

	// Multi-line if/anif/elseif ending with "then"
	if (strings.HasPrefix(trimmed, "if ") || strings.HasPrefix(trimmed, "anif ") ||
		strings.HasPrefix(trimmed, "elseif ")) && strings.HasSuffix(trimmed, " then") {
		return true
	}

	// Standalone else (not followed by statement on same line)
	if trimmed == "else" {
		return true
	}

	// Multi-line loop ending with "do"
	if strings.HasPrefix(trimmed, "loop ") && strings.HasSuffix(trimmed, " do") {
		return true
	}

	// Multi-line when ending with "then"
	if strings.HasPrefix(trimmed, "when ") && strings.HasSuffix(trimmed, " then") {
		return true
	}

	// Switch ending with "on"
	if strings.HasPrefix(trimmed, "switch ") && strings.HasSuffix(trimmed, " on") {
		return true
	}

	// Struct/enum definition ending with ":"
	if (strings.HasPrefix(trimmed, "struct ") || strings.HasPrefix(trimmed, "enum ")) &&
		strings.HasSuffix(trimmed, ":") {
		return true
	}

	// Type variants in struct (e.g., "type smoke:")
	if strings.HasPrefix(trimmed, "type ") && strings.HasSuffix(trimmed, ":") {
		return true
	}

	// Switch case lines ending with ":"
	if strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "?") &&
		!strings.Contains(trimmed, "::") {
		// This could be a switch case, struct field, or variable declaration
		// For switch cases (e.g., "1: ahoy|x|"), if there's code after :, it's single-line
		colonIdx := strings.Index(trimmed, ":")
		afterColon := strings.TrimSpace(trimmed[colonIdx+1:])

		// If nothing after colon, or it's a type annotation, indent
		// If there's a statement after colon (switch case), don't indent
		if afterColon == "" {
			return true
		}

		// Check if it's a struct field with type (e.g., "x: float")
		// These should not cause indentation
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) == 2 {
			beforeColon := strings.TrimSpace(parts[0])

			// Variable/field declarations don't increase indent
			// Unless it's a struct type variant which we already handled above
			if !strings.HasPrefix(beforeColon, "type ") {
				return false
			}
		}
	}

	return false
}
