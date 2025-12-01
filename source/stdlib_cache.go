package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var (
	stdlibExtractOnce sync.Once
	stdlibPath        string
	stdlibErr         error
)

// GetStdlibPath returns the path to the ahoy_stdlib.ahoy file in cache
// Creates the file if it doesn't exist
func GetStdlibPath() (string, error) {
	stdlibExtractOnce.Do(func() {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				stdlibErr = fmt.Errorf("cannot find cache or home directory: %w", err)
				return
			}
			cacheDir = filepath.Join(homeDir, ".cache")
		}

		ahoyDir := filepath.Join(cacheDir, "ahoy")
		stdlibPath = filepath.Join(ahoyDir, "ahoy_stdlib.ahoy")

		// Check if already exists
		if _, err := os.Stat(stdlibPath); err == nil {
			return
		}

		// Create ahoy cache directory
		if err := os.MkdirAll(ahoyDir, 0755); err != nil {
			stdlibErr = fmt.Errorf("failed to create ahoy cache directory: %w", err)
			return
		}

		// Generate and write stdlib file
		content := generateStdlibFile()
		if err := os.WriteFile(stdlibPath, []byte(content), 0644); err != nil {
			stdlibErr = fmt.Errorf("failed to write stdlib file: %w", err)
			return
		}
	})

	return stdlibPath, stdlibErr
}

// generateStdlibFile generates the ahoy_stdlib.ahoy content from stdlib definitions
func generateStdlibFile() string {
	var sb strings.Builder

	sb.WriteString(`? =============================================================================
? AHOY STANDARD LIBRARY - Built-in Functions and Methods
? =============================================================================
? This file documents all built-in functions and methods available in Ahoy.
? It is used by the LSP for goto definition, hover documentation, and autocomplete.
? DO NOT modify this file - it is auto-generated from stdlib.go definitions.
? =============================================================================

`)

	// Array Methods
	sb.WriteString("? =============================================================================\n")
	sb.WriteString("? ARRAY METHODS\n")
	sb.WriteString("? =============================================================================\n\n")

	arrayKeys := make([]string, 0, len(ArrayMethods))
	for k := range ArrayMethods {
		arrayKeys = append(arrayKeys, k)
	}
	sort.Strings(arrayKeys)

	for _, key := range arrayKeys {
		method := ArrayMethods[key]
		writeMethodDoc(&sb, method)
	}

	// Dictionary Methods
	sb.WriteString("? =============================================================================\n")
	sb.WriteString("? DICTIONARY METHODS\n")
	sb.WriteString("? =============================================================================\n\n")

	dictKeys := make([]string, 0, len(DictMethods))
	for k := range DictMethods {
		dictKeys = append(dictKeys, k)
	}
	sort.Strings(dictKeys)

	for _, key := range dictKeys {
		method := DictMethods[key]
		writeMethodDoc(&sb, method)
	}

	// String Methods
	sb.WriteString("? =============================================================================\n")
	sb.WriteString("? STRING METHODS\n")
	sb.WriteString("? =============================================================================\n\n")

	stringKeys := make([]string, 0, len(StringMethods))
	for k := range StringMethods {
		stringKeys = append(stringKeys, k)
	}
	sort.Strings(stringKeys)

	for _, key := range stringKeys {
		method := StringMethods[key]
		writeMethodDoc(&sb, method)
	}

	// Built-in Functions
	sb.WriteString("? =============================================================================\n")
	sb.WriteString("? BUILT-IN FUNCTIONS\n")
	sb.WriteString("? =============================================================================\n\n")

	builtinKeys := make([]string, 0, len(BuiltinFuncs))
	for k := range BuiltinFuncs {
		builtinKeys = append(builtinKeys, k)
	}
	sort.Strings(builtinKeys)

	for _, key := range builtinKeys {
		fn := BuiltinFuncs[key]
		writeMethodDoc(&sb, fn)
	}

	sb.WriteString("? =============================================================================\n")
	sb.WriteString("? END OF AHOY STANDARD LIBRARY\n")
	sb.WriteString("? =============================================================================\n")

	return sb.String()
}

func writeMethodDoc(sb *strings.Builder, method StdlibFunc) {
	sb.WriteString("? -----------------------------------------------------------------------------\n")
	
	// Write method signature
	if method.Category == "builtin" {
		sb.WriteString(fmt.Sprintf("? %s|%s|\n", method.MethodName, method.Params))
	} else {
		sb.WriteString(fmt.Sprintf("? %s.%s|| -> %s\n", method.Category, method.MethodName, method.ReturnType))
	}
	
	// Write documentation
	sb.WriteString(fmt.Sprintf("? %s\n", method.Doc))
	
	// Write C implementation as comment if available
	if method.Code != "" {
		sb.WriteString("? C Implementation:\n")
		for _, line := range strings.Split(strings.TrimSpace(method.Code), "\n") {
			sb.WriteString(fmt.Sprintf("?   %s\n", line))
		}
	}
	
	// Write Ahoy function definition for LSP
	funcName := fmt.Sprintf("%s_%s", method.Category, method.MethodName)
	if method.Category == "builtin" {
		funcName = method.MethodName
	}
	
	sb.WriteString(fmt.Sprintf("@ %s |%s| %s:\n", funcName, method.Params, method.ReturnType))
	sb.WriteString(fmt.Sprintf("    ? %s\n", method.Doc))
	
	// Write return statement based on return type
	switch method.ReturnType {
	case "void":
		// No return for void
	case "int":
		sb.WriteString("    return 0\n")
	case "bool":
		sb.WriteString("    return false\n")
	case "string":
		sb.WriteString("    return \"\"\n")
	case "array", "array[string]":
		sb.WriteString("    return []\n")
	case "dict":
		sb.WriteString("    return {}\n")
	case "any":
		sb.WriteString("    return 0\n")
	default:
		if strings.HasPrefix(method.ReturnType, "array") {
			sb.WriteString("    return []\n")
		}
	}
	
	sb.WriteString("$\n\n")
}

// EnsureStdlibExists ensures the stdlib file exists in cache
// This should be called during compiler initialization
func EnsureStdlibExists() error {
	_, err := GetStdlibPath()
	return err
}
