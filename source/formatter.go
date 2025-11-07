package main

import (
	"strings"
)

const INDENT_SIZE = 2

// formatSource formats Ahoy source code with proper indentation
func formatSource(source string) string {
	lines := strings.Split(source, "\n")
	var formatted []string
	indentLevel := 0
	structStack := []int{} // Stack to track struct indent levels

	for _, line := range lines {
		// Convert tabs to spaces
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

		// Apply current indentation (even to comments)
		indent := strings.Repeat(" ", indentLevel*INDENT_SIZE)
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
