# Ahoy Programming Language

A modern programming language with Python-like syntax that compiles to C for high performance.

## Features

- **Clean Syntax**: Python-inspired with whitespace indentation
- **Type Inference**: Automatic type detection using `:`
- **Pipe Syntax**: Function calls with `|args|`
- **Word Operators**: Use `plus`, `minus`, `times`, etc.
- **Dynamic Arrays**: Built-in with C implementation
- **Dictionaries**: Python-style hash maps
- **Compile to C**: Native performance
- **Single-Line Statements**: Use `;` to combine statements
- **`ahoy` Keyword**: Shorthand for print statements

## Quick Start

### Installation

Requires:
- Go 1.25+
- GCC compiler

### Build

```bash
./build.sh
```

This creates the `ahoy-bin` executable.

### Usage

```bash
# Compile and run
./ahoy-bin -f input/test.ahoy -r

# Just compile
./ahoy-bin -f input/test.ahoy

# Format source file
./ahoy-bin -f input/test.ahoy -format

# Show help
./ahoy-bin -h
```

**Alternative**: Run from source directory:
```bash
cd source
go run . -f ../input/test.ahoy -r
```

**Important**: Use `go run .` (with dot) NOT `go run main.go`

## Language Syntax

### Variables & Assignment
```python
# Single line
x: 42; name: "Ahoy"; active: true

# Multiple lines
x: 42
name: "Ahoy"
active: true
```

### Print Statements
```python
# Using ahoy keyword (shorthand)
ahoy|"Hello, World!\n"|
ahoy|"Value: %d\n", x|

# Using print
print|"Hello, %s!\n", name|
```

### Operators

**Arithmetic** (symbols or words):
```python
result: a + b       # or: a plus b
result: a - b       # or: a minus b
result: a * b       # or: a times b
result: a / b       # or: a div b
result: a % b       # or: a mod b
```

**Comparison** (symbols or words):
```python
if x > y then       # or: x greater_than y
if x < y then       # or: x lesser_than y
if x is y then      # equality
```

**Boolean**:
```python
result: flag and not other
result: this or that
```

### Control Flow
```python
if condition then
    action||
else then
    other_action||

# Single-line if
if x greater_than 5 then ahoy|"Big!\n"|

# Loops
i: 0
loop i lesser_than 10 then
    ahoy|"Count: %d\n", i|
    i: i plus 1
```

### Functions
```python
func factorial|n int| int then
    if n <= 1 then
        return 1
    else then
        return n times factorial|n minus 1|

result: factorial|5|
ahoy|"5! = %d\n", result|
```

### Arrays
```python
# Declaration
numbers: <10, 20, 30, 40>

# Access
first: numbers<0>
last: numbers<3>

# Single line
arr: <1, 2, 3>; ahoy|"First: %d\n", arr<0>|
```

### Dictionaries
```python
# Declaration
person: {"name":"Alice", "age":30, "active":true}

# Access
name: person{"name"}
age: person{"age"}

# Single line
data: {"x":10, "y":20}; total: data{"x"} plus data{"y"}|
```

### Compile-Time Conditionals
```python
when DEBUG then
    ahoy|"Debug mode enabled\n"|

when PRODUCTION then
    optimize_code||
```

Compiles to C's `#ifdef` directive.

### Single-Line Statements
```python
# Multiple statements on one line with semicolons
x: 10; y: 20; result: x plus y; ahoy|"Result: %d\n", result|

# Mix declarations and calls
a: 5; b: 3; ahoy|"%d + %d = %d\n", a, b, a plus b|
```

## Complete Example

```python
# example.ahoy - Comprehensive feature demo
import "stdio.h"

# Fibonacci with recursion
func fib|n int| int then
    if n <= 1 then
        return n
    else then
        return fib|n minus 1| plus fib|n minus 2|

# Single-line variable declarations
x: 10; y: 20; name: "Ahoy"

# Word operators
sum: x plus y
product: x times y

# ahoy keyword (print shorthand)
ahoy|"Language: %s\n", name|
ahoy|"Sum: %d, Product: %d\n", sum, product|

# Arrays and loops
numbers: <5, 10, 15, 20>
i: 0
loop i lesser_than 4 then
    ahoy|"Number[%d] = %d\n", i, numbers<i>|
    i: i plus 1

# Dictionaries
config: {"debug":true, "version":2}
if config{"debug"} then
    ahoy|"Debug mode ON\n"|

# Compile-time conditionals
when BENCHMARK then
    ahoy|"Running benchmarks...\n"|

# Fibonacci demo
ahoy|"Fib(10) = %d\n", fib|10||
```

## CLI Reference

```
go run . -f <file.ahoy> [options]

Options:
  -f <file>     Input .ahoy source file (required)
  -r            Run the compiled C program
  -format       Format the source file (tabs → spaces, trim whitespace)
  -h            Show help message
```

## File Extension

All Ahoy source files use the `.ahoy` extension.

## Project Structure

```
programming_language/
├── input/              # .ahoy source files
│   ├── test.ahoy
│   ├── simple.ahoy
│   └── features_test.ahoy
├── source/             # Compiler source (Go)
│   ├── main.go         # CLI and main entry
│   ├── tokenizer.go    # Lexical analysis
│   ├── parser.go       # Syntax analysis
│   ├── codegen.go      # C code generation
│   ├── formatter.go    # Code formatter
│   └── main_test.go    # Unit tests
├── output/             # Generated C files and executables
└── README.md
```

## Features Summary

- ✅ Python-like whitespace syntax
- ✅ Type inference with `:`
- ✅ Pipe syntax `func|args|` for calls
- ✅ Word-based operators (plus, minus, times, div, mod)
- ✅ Comparison words (greater_than, lesser_than)
- ✅ `ahoy` print shorthand
- ✅ Single-line statements with `;`
- ✅ Dynamic arrays with `<...>`
- ✅ Python-style dictionaries with `{...}`
- ✅ Compile-time `when` blocks
- ✅ Static typing with inference
- ✅ C library imports
- ✅ Code formatter
- ✅ Compiles to optimized C

## Examples in Repository

- `input/simple.ahoy` - Basic features with semicolons
- `input/test.ahoy` - Fibonacci and loops
- `input/features_test.ahoy` - All language features

## Notes

- Tabs automatically converted to spaces by formatter
- Semicolons allow Python-style single-line statements
- `ahoy` is equivalent to `print` but shorter
- Keywords: `greater_than` and `lesser_than` (note the underscore)
- Arrays use `<>`, dictionaries use `{}`
- All code compiles to clean, optimized C

## Testing

```bash
cd source
go test -v    # Run all unit tests
```

All 5 unit tests pass.

## Performance

Ahoy compiles to C, giving you:
- Native execution speed
- Low memory footprint
- No runtime overhead
- Full C library access

Ahoy! 🏴‍☠️
