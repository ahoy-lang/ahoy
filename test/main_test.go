package main

import (
"bytes"
"fmt"
"os"
"os/exec"
"path/filepath"
"strings"
"testing"
)

// TestConsolidatedFiles tests all consolidated test files
func TestConsolidatedFiles(t *testing.T) {
tests := []struct {
name           string
inputFile      string
expectedLines  []string
}{
{
name:      "Arrays Test",
inputFile: "input/arrays.ahoy",
expectedLines: []string{
"1",      // first
"world",  // word
"4",      // len
"8",      // last
"20",     // total
"1",      // has_20 (true)
"1",      // third (reversed[2])
},
},
{
name:      "Dictionaries Test",
inputFile: "input/dictionaries.ahoy",
expectedLines: []string{
"Alice",             // name
"NYC",               // city
"3",                 // size
"1",                 // has_theme (true)
"0",                 // has_font (false)
"8080",              // port
"Key: name",         // dict iteration
"Value: PyLang",     // dict iteration
"Key: version",      // dict iteration
"Value: 2",          // dict iteration
},
},
{
name:      "Objects Test",
inputFile: "input/objects.ahoy",
expectedLines: []string{
"{name: \"Alice\", age: 30}", // person object
"Alice",                       // person.name
"30",                          // person.age
"Test",                        // data.label
"localhost",                   // config.host
},
},
{
name:      "Tuples Test",
inputFile: "input/tuples.ahoy",
expectedLines: []string{
"65", // c = 55 + 10 after swap
},
},
{
name:      "Loops Test",
inputFile: "input/loops.ahoy",
expectedLines: []string{
"1",  // loop items
"2",
"3",
"10", // loop with halt
"20",
"1",  // loop with next (skips 2)
"3",
"4",
},
},
{
name:      "Conditionals Test",
inputFile: "input/conditionals.ahoy",
expectedLines: []string{
"ten",   // if x is 10
"small", // if y > 10 else
"Tue",   // switch day
"Good",  // switch grade
},
},
{
name:      "Enums Test",
inputFile: "input/enums.ahoy",
expectedLines: []string{
"1", // x
},
},
{
name:      "Functions Test",
inputFile: "input/functions.ahoy",
expectedLines: []string{
"15", // z = x + y
},
},
}

// Build the compiler first
compilerPath := "../source/ahoy-compiler"
if _, err := os.Stat(compilerPath); os.IsNotExist(err) {
// Try to build it
cmd := exec.Command("go", "build", "-o", compilerPath, "../source")
if err := cmd.Run(); err != nil {
t.Fatalf("Failed to build compiler: %v", err)
}
}

for _, tt := range tests {
t.Run(tt.name, func(t *testing.T) {
output, err := compileAndRun(t, tt.inputFile, compilerPath)
if err != nil {
t.Fatalf("Failed to compile and run: %v", err)
}

lines := strings.Split(strings.TrimSpace(output), "\n")
for _, expected := range tt.expectedLines {
found := false
for _, line := range lines {
if strings.Contains(line, expected) {
found = true
break
}
}
if !found {
t.Errorf("Expected output to contain '%s', got:\n%s", expected, output)
}
}
})
}
}

// compileAndRun compiles an Ahoy file using the compiler and returns output
func compileAndRun(t *testing.T, ahoyFile string, compilerPath string) (string, error) {
baseName := strings.TrimSuffix(filepath.Base(ahoyFile), ".ahoy")
outputDir := "output"
os.MkdirAll(outputDir, 0755)

cFile := filepath.Join(outputDir, baseName+".c")
executable := filepath.Join(outputDir, baseName)

// Compile with ahoy-compiler
cmd := exec.Command(compilerPath, "-f", ahoyFile)
var compileErr bytes.Buffer
cmd.Stderr = &compileErr
cmd.Stdout = &compileErr
err := cmd.Run()
if err != nil {
return "", fmt.Errorf("ahoy compilation failed: %v\n%s", err, compileErr.String())
}

// Compile C code
cmd = exec.Command("gcc", "-o", executable, cFile, "-lm")
compileErr.Reset()
cmd.Stderr = &compileErr
err = cmd.Run()
if err != nil {
return "", fmt.Errorf("C compilation failed: %v\n%s", err, compileErr.String())
}

// Run executable
cmd = exec.Command(executable)
var output bytes.Buffer
cmd.Stdout = &output
cmd.Stderr = &output
err = cmd.Run()
if err != nil {
return "", fmt.Errorf("execution failed: %v\n%s", err, output.String())
}

return output.String(), nil
}
