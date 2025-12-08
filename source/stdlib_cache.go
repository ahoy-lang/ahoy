package main

import (
	"crypto/md5"
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

// GetStdlibPath returns the path to the ahoy_stdlib.c file in cache
// DEPRECATED: This is kept for backward compatibility but no longer generates the file
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
		stdlibPath = filepath.Join(ahoyDir, "ahoy_stdlib.c")
		
		// No longer generate .c file - only .ahoy file is used
	})

	return stdlibPath, stdlibErr
}

// generateStdlibCFile generates the ahoy_stdlib.c content from stdlib definitions
// Uses C syntax with special comment markers for LSP
func generateStdlibCFile() string {
	var sb strings.Builder

	sb.WriteString(`/*
 * =============================================================================
 * AHOY STANDARD LIBRARY - Built-in Functions and Methods
 * =============================================================================
 * This file documents all built-in functions and methods available in Ahoy.
 * It is used by the LSP for goto definition, hover documentation, and autocomplete.
 * 
 * Comment format (multiline C comments):
 *   @ function_name |param:type, ...| return_type:
 *   ? Description of the function
 *   return [default_value]
 *   $
 * =============================================================================
 */

#ifndef AHOY_STDLIB_H
#define AHOY_STDLIB_H

#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <stdbool.h>
#include <ctype.h>
#include <time.h>
#include <regex.h>

`)

	// Array Methods
	sb.WriteString("/* =============================================================================\n")
	sb.WriteString(" * ARRAY METHODS\n")
	sb.WriteString(" * ============================================================================= */\n\n")

	arrayKeys := make([]string, 0, len(ArrayMethods))
	for k := range ArrayMethods {
		arrayKeys = append(arrayKeys, k)
	}
	sort.Strings(arrayKeys)

	for _, key := range arrayKeys {
		method := ArrayMethods[key]
		writeCMethodDoc(&sb, method)
	}

	// Dictionary Methods
	sb.WriteString("/* =============================================================================\n")
	sb.WriteString(" * DICTIONARY METHODS\n")
	sb.WriteString(" * ============================================================================= */\n\n")

	dictKeys := make([]string, 0, len(DictMethods))
	for k := range DictMethods {
		dictKeys = append(dictKeys, k)
	}
	sort.Strings(dictKeys)

	for _, key := range dictKeys {
		method := DictMethods[key]
		writeCMethodDoc(&sb, method)
	}

	// String Methods
	sb.WriteString("/* =============================================================================\n")
	sb.WriteString(" * STRING METHODS\n")
	sb.WriteString(" * ============================================================================= */\n\n")

	stringKeys := make([]string, 0, len(StringMethods))
	for k := range StringMethods {
		stringKeys = append(stringKeys, k)
	}
	sort.Strings(stringKeys)

	for _, key := range stringKeys {
		method := StringMethods[key]
		writeCMethodDoc(&sb, method)
	}

	// Built-in Functions
	sb.WriteString("/* =============================================================================\n")
	sb.WriteString(" * BUILT-IN FUNCTIONS\n")
	sb.WriteString(" * ============================================================================= */\n\n")

	builtinKeys := make([]string, 0, len(BuiltinFuncs))
	for k := range BuiltinFuncs {
		builtinKeys = append(builtinKeys, k)
	}
	sort.Strings(builtinKeys)

	for _, key := range builtinKeys {
		fn := BuiltinFuncs[key]
		writeCMethodDoc(&sb, fn)
	}

	sb.WriteString("/* =============================================================================\n")
	sb.WriteString(" * END OF AHOY STANDARD LIBRARY\n")
	sb.WriteString(" * ============================================================================= */\n\n")
	sb.WriteString("#endif /* AHOY_STDLIB_H */\n")

	return sb.String()
}

func writeCMethodDoc(sb *strings.Builder, method StdlibFunc) {
	// Write method signature in Ahoy format using multiline C comment
	funcName := fmt.Sprintf("%s_%s", method.Category, method.MethodName)
	if method.Category == "builtin" {
		funcName = method.MethodName
	}
	
	sb.WriteString("/*\n")
	sb.WriteString(fmt.Sprintf(" * @ %s |%s| %s:\n", funcName, method.Params, method.ReturnType))
	sb.WriteString(fmt.Sprintf(" * ? %s\n", method.Doc))
	
	// Write return statement marker based on return type
	switch method.ReturnType {
	case "void":
		sb.WriteString(" * return\n")
	case "int":
		sb.WriteString(" * return 0\n")
	case "bool":
		sb.WriteString(" * return false\n")
	case "string":
		sb.WriteString(" * return \"\"\n")
	case "array", "array[string]":
		sb.WriteString(" * return []\n")
	case "dict":
		sb.WriteString(" * return <>\n")
	case "any":
		sb.WriteString(" * return 0\n")
	default:
		if strings.HasPrefix(method.ReturnType, "array") {
			sb.WriteString(" * return []\n")
		} else {
			sb.WriteString(" * return\n")
		}
	}
	sb.WriteString(" * $\n")
	sb.WriteString(" */\n")

	// Write actual C implementation
	if method.Code != "" {
		sb.WriteString(method.Code)
	} else {
		// For builtins without code, write a stub
		sb.WriteString(fmt.Sprintf("/* %s is a compiler builtin */\n", funcName))
	}
	sb.WriteString("\n")
}

// EnsureStdlibExists ensures the stdlib file exists in cache
// This should be called during compiler initialization
func EnsureStdlibExists() error {
	_, err := GetStdlibPath()
	if err != nil {
		return err
	}
	
	// Also generate the .ahoy file for LSP
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		cacheDir = filepath.Join(homeDir, ".cache")
	}
	
	ahoyDir := filepath.Join(cacheDir, "ahoy")
	ahoyFilePath := filepath.Join(ahoyDir, "ahoy_stdlib.ahoy")
	
	// Generate .ahoy content
	ahoyContent := GenerateStdlibAhoyFile()
	newChecksum := md5.Sum([]byte(ahoyContent))
	
	// Check if file exists with same content
	if existingContent, err := os.ReadFile(ahoyFilePath); err == nil {
		existingChecksum := md5.Sum(existingContent)
		if existingChecksum == newChecksum {
			return nil // Already up to date
		}
	}
	
	// Write the .ahoy file
	if err := os.WriteFile(ahoyFilePath, []byte(ahoyContent), 0644); err != nil {
		return fmt.Errorf("failed to write stdlib .ahoy file: %w", err)
	}
	
	return nil
}

// GetStdlibChecksum returns the MD5 checksum of the current stdlib content
func GetStdlibChecksum() string {
	content := generateStdlibCFile()
	checksum := md5.Sum([]byte(content))
	return fmt.Sprintf("%x", checksum)
}
