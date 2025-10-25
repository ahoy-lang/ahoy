package main

import (
	"strings"
)

// formatSource formats Ahoy source code
func formatSource(source string) string {
	lines := strings.Split(source, "\n")
	var formatted []string
	inSwitch := false
	
	for _, line := range lines {
		// Convert tabs to 4 spaces
		line = strings.ReplaceAll(line, "\t", "    ")
		
		// Trim trailing whitespace
		line = strings.TrimRight(line, " \t")
		
		trimmed := strings.TrimSpace(line)
		
		// Detect switch statements
		if strings.HasPrefix(trimmed, "switch ") {
			inSwitch = true
		}
		
		// Format switch case lines (keep on one line if under 110 chars)
		if inSwitch && strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "switch") {
			// This is a case line
			if len(line) <= 110 {
				// Keep on one line - already good
			} else {
				// Line too long, but keep for now (could add wrapping logic)
			}
		}
		
		// Exit switch detection on dedent or another statement
		if inSwitch && (strings.HasPrefix(trimmed, "if ") || 
			strings.HasPrefix(trimmed, "loop ") || 
			strings.HasPrefix(trimmed, "func ") ||
			(trimmed != "" && !strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "switch"))) {
			inSwitch = false
		}
		
		// Format inline conditionals - keep on one line if all parts are inline
		// Pattern: "if condition then statement" or "anif/elseif condition then statement" or "else statement"
		if (strings.HasPrefix(trimmed, "if ") || strings.HasPrefix(trimmed, "anif ") || 
			strings.HasPrefix(trimmed, "elseif ") || strings.HasPrefix(trimmed, "else ")) &&
			!strings.HasSuffix(trimmed, "then") {
			// This is an inline conditional - keep it as is
		}
		
		formatted = append(formatted, line)
	}
	
	result := strings.Join(formatted, "\n")
	
	// Ensure file ends with newline
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	
	return result
}
