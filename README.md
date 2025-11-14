# Ahoy Programming Language

A modern, expressive programming language with clean syntax and powerful features including default arguments, type annotations, assertions, and deferred execution.

## Features

- **Clean Syntax**: Python-inspired with whitespace indentation
- **Type System**: Optional type annotations with inference
- **Default Arguments**: Flexible function parameters
- **Assert & Defer**: Runtime validation and guaranteed cleanup
- **Pipe Syntax**: Function calls with `|args|`
- **Word Operators**: Use `plus`, `minus`, `times`, etc.
- **Arrays**: Built-in with `[]` syntax
- **Objects/Structs**: C-style with `{}` syntax
- **Dictionaries**: Hash maps with `<>` syntax
- **F-Strings**: String interpolation with `f"...{var}..."`
- **LSP Support**: Real-time linting and validation
- **Single-Line Statements**: Use `;` to combine statements
- **`ahoy` Keyword**: Shorthand for print statements

## Quick Start

### Installation

Requires:
- Go 1.25+
- GCC compiler (for C compilation)

### Build

```bash
cd ahoy
./build.sh
```

This creates the `ahoy-bin` executable.

### Usage

```bash
# Compile and run
./ahoy-bin -f test.ahoy -r

# Just compile
./ahoy-bin -f test.ahoy

# Show help
./ahoy-bin -h
```

## Language Syntax

### Variables & Type Annotations

```ahoy
? Type inference (original)
x: 42
name: "Alice"
active: true

? Explicit type annotations (NEW!)
age:int= 29
price:float= 19.99
items:array= [1, 2, 3]
config:dict= {"host": "localhost"}

? Constants with types
MAX_SIZE::int= 100
API_KEY::string= "secret"
TIMEOUT:: 30  ? Inferred as int
```

### Print Statements & F-Strings

```ahoy
? Using ahoy keyword (shorthand)
ahoy |"Hello, World!"|

? F-strings with interpolation (NEW!)
name: "Alice"
age: 30
ahoy |f"Hello {name}, you are {age} years old!"|

? Format expressions in f-strings
x: 10
y: 20
ahoy |f"The sum of {x} and {y} is {x + y}"|
```

### Operators

**Arithmetic** (symbols or words):
```ahoy
result: a + b       ? or: a plus b
result: a - b       ? or: a minus b
result: a * b       ? or: a times b
result: a / b       ? or: a div b
result: a % b       ? or: a mod b
```

**Comparison** (symbols or words):
```ahoy
if x > y then       ? or: x greater_than y
if x < y then       ? or: x lesser_than y
if x is y then      ? equality
if x not is y then  ? inequality
```

**Ternary Operator** (NEW!):
```ahoy
result: condition ?? true_value : false_value
max: x > y ?? x : y
```

**Boolean**:
```ahoy
result: flag and not other
result: this or that
```

### Control Flow

```ahoy
? If statements
if condition then
    action||
else then
    other_action||

? Switch statements
switch value on
    1:
        ahoy |"One"|
    2:
        ahoy |"Two"|
    default:
        ahoy |"Other"|

? Loops (halt = break, next = continue)
loop i:0 to 10
    if i is 5
        next  ? Skip 5
    if i is 8
        halt  ? Stop at 8
    ahoy |f"i = {i}"|

? Loop over array
numbers: [1, 2, 3, 4, 5]
loop num in numbers
    ahoy |f"Number: {num}"|

? Loop over dict
data: {"x": 10, "y": 20}
loop key, value in data
    ahoy |f"{key} = {value}"|
```

### Functions with Default Arguments (NEW!)

```ahoy
? Function with default parameters
greet :: |name:string, greeting:string="Hello", punctuation:string="!"| string:
    return f"{greeting} {name}{punctuation}"

? Call with different argument counts
msg1: greet|"Alice"|                    ? Uses defaults
msg2: greet|"Bob", "Hi"|                ? Partial override
msg3: greet|"Charlie", "Hey", "!!!"|   ? All explicit

? Return type keywords
calculate :: |x:int, y:int| infer:     ? Infer return type
    return x * y, x + y

log :: |message:string| void:          ? No return value
    ahoy |message|

log2 :: |message:string|:              ? Implicit void
    ahoy |message|
```

### Assert Statements (NEW!)

```ahoy
? Basic assertions
assert x > 0
assert count is 10
assert name not is ""

? Precondition checking
divide :: |a:int, b:int| float:
    assert b not is 0           ? Prevent division by zero
    return a / b

? Validation pipeline
validate_user :: |user:dict|:
    assert "name" in user
    assert "email" in user
    assert len|user["name"]| > 0
    assert "@" in user["email"]
    ahoy |"User valid!"|
```

### Defer Statements (NEW!)

```ahoy
? Basic defer (executes at function exit)
greet :: |name:string|:
    defer ahoy |"Goodbye!"|
    ahoy |f"Hello, {name}!"|
    ahoy |"Nice to meet you"|
? Output: Hello, Alice! / Nice to meet you / Goodbye!

? Resource cleanup
process_file :: |filename:string|:
    ahoy |f"Opening {filename}"|
    defer ahoy |f"Closing {filename}"|
    ? ... file operations ...
    ? File automatically "closed" when function exits

? Multiple defers (LIFO - Last In First Out)
demo :: ||:
    defer ahoy |"Third (executes first)"|
    defer ahoy |"Second"|
    defer ahoy |"First (executes last)"|
    ahoy |"Main function"|

? Defer with return values
calculate :: |x:int, y:int| int:
    defer ahoy |"Calculation completed"|
    result: x * y
    return result
```

### Arrays

```ahoy
? Declaration with [] (NEW SYNTAX!)
numbers: [10, 20, 30, 40]
mixed: [1, "hello", true]

? Access
first: numbers[0]
last: numbers[3]

? Type annotation
items:array= [1, 2, 3, 4, 5]

? Iteration
loop num in numbers
    ahoy |f"Number: {num}"|
```

### Objects/Structs

**NEW SYNTAX**: Objects now use `{}` braces!

```ahoy
? Anonymous object (uses HashMap internally)
person: {name: "Alice", age: 30, active: true}

? Access with dot notation
name: person.name
age: person.age

? Access with bracket notation (for dynamic keys)
key: "name"
value: person{key}

? Struct definitions
struct Point:
  x: float,
  y: float
$

? Typed object instantiation
origin: Point{x: 0.0, y: 0.0}
point: Point{x: 10.5, y: 20.3}

? Access nested properties
point.x: 15.0
print|point.x|

? Built-in structs (Raylib compatible)
struct vector2:
  x: float,
  y: float
$

struct color:
  r: int,
  g: int,
  b: int,
  a: int
$

position: vector2{x: 100.0, y: 200.0}
red: color{r: 255, g: 0, b: 0, a: 255}
```

### Dictionaries

**NEW SYNTAX**: Dictionaries now use `<>` angle brackets!

```ahoy
? Untyped dictionary (inferred)
settings: <"theme": "dark", "lang": "en">

? Typed dictionary (explicit key/value types)
scores:dict<string,int> = <"Alice": 100, "Bob": 95>

? Access with angle brackets
theme: settings<"theme">

? Update values
settings<"lang">: "es"

? Type annotation with dict keyword
config:dict = <"host": "localhost", "port": 8080>

? Iteration
loop key, value in settings
    print|f"{key}: {value}"|

? Methods
has_theme: settings.has|"theme"|  ? Returns 1 or 0
```

### Enums

```ahoy
enum Color:
    RED
    GREEN
    BLUE

enum Status:
    PENDING: 0
    ACTIVE: 1
    DONE: 2

? Usage
current_color: Color.RED
state: Status.ACTIVE
```

### Complete Example

```ahoy
program example

? Constants with types
MAX_RETRIES::int= 3
API_URL::string= "https://api.example.com"
TIMEOUT::float= 30.0

? Function with default arguments, types, assert, and defer
process_data :: |data:dict, timeout:float=TIMEOUT, retries:int=MAX_RETRIES| bool:
    ? Precondition validation
    assert data not is null
    assert timeout > 0
    assert retries > 0
    
    ? Setup with guaranteed cleanup
    ahoy |f"Processing data with timeout={timeout}, retries={retries}"|
    defer ahoy |"Processing completed"|
    
    ? Type-annotated variables
    attempt:int= 0
    success:bool= false
    
    ? Processing loop
    loop i:1 to retries
        defer ahoy |f"Attempt {i} finished"|
        
        attempt: attempt + 1
        result: try_process|data, timeout|
        
        if result is true
            success: true
            halt
    
    ? Postcondition validation
    assert attempt <= retries
    assert attempt > 0
    
    return success

? Main execution
config:dict= {
    "id": 12345,
    "name": "test_data",
    "priority": "high"
}

? Call with defaults
result: process_data|config|
ahoy |f"Result: {result}"|

? Call with custom timeout
result2: process_data|config, 45.0|
ahoy |f"Result2: {result2}"|

? Array operations
numbers:array= [1, 2, 3, 4, 5]
loop num in numbers
    square: num * num
    ahoy |f"{num}² = {square}"|

? Ternary operator
max_retries: 5
mode: max_retries > 3 ?? "aggressive" : "normal"
ahoy |f"Mode: {mode}"|
```

## Advanced Features

### Pattern Matching (Switch)

### Pattern Matching (Switch)

```ahoy
switch expression on
    value1:
        ? code for value1
    value2, value3:
        ? code for multiple values
    default:
        ? default case
```

### Type System

**Supported Types:**
- `int` - Integers
- `float` - Floating point
- `string` - Text strings
- `bool` - Booleans (true/false)
- `char` - Single characters
- `array` - Arrays/lists
- `dict` - Dictionaries/maps
- `vector2` - 2D vectors
- `color` - Color values
- `infer` - Inferred return type (functions)
- `void` - No return value (functions)

### LSP Features

The Ahoy LSP provides real-time diagnostics:

- **Type Checking**: Validates type annotations
- **Argument Validation**: Checks function call argument counts
- **Undefined Function Detection**: Warns about missing functions
- **Return Type Validation**: Ensures functions return correct types
- **Const Reassignment**: Prevents modifying constants
- **Enum Validation**: Checks for duplicate enum values

## CLI Reference

```
./ahoy-bin -f <file.ahoy> [options]

Options:
  -f <file>     Input .ahoy source file (required)
  -r            Run the compiled program
  -lint         Run in lint-only mode (check for errors)
  -h            Show help message
```

## File Extension

All Ahoy source files use the `.ahoy` extension.

## Language Reference Card

### Comments
```ahoy
? This is a comment (using ?)
# This is also a comment (using #)
```

### Syntax Summary

| Feature | Syntax | Example |
|---------|--------|---------|
| Variable | `name: value` | `x: 42` |
| Variable (typed) | `name:type= value` | `age:int= 29` |
| Constant | `name:: value` | `MAX:: 100` |
| Constant (typed) | `name::type= value` | `MAX::int= 100` |
| Function | `name :: \|params\| type:` | `add :: \|a:int, b:int\| int:` |
| Default arg | `param:type=default` | `timeout:float=30.0` |
| Array | `[item1, item2, ...]` | `[1, 2, 3]` |
| Object | `<key: value, ...>` | `<name: "Alice", age: 30>` |
| Dict | `{"key": value, ...}` | `{"x": 10, "y": 20}` |
| F-String | `f"text {expr} text"` | `f"Sum is {x + y}"` |
| Ternary | `cond ?? true : false` | `max: a > b ?? a : b` |
| Assert | `assert condition` | `assert x > 0` |
| Defer | `defer statement` | `defer cleanup\|\|` |
| Loop | `loop var:start to end` | `loop i:0 to 10` |
| Loop (array) | `loop item in array` | `loop x in nums` |
| Loop (dict) | `loop key, val in dict` | `loop k, v in data` |
| Break | `halt` | `if done halt` |
| Continue | `next` | `if skip next` |

## Project Structure

```
ahoy-lang/
├── ahoy/                    # Core compiler
│   ├── parser.go            # Syntax analysis
│   ├── tokenizer.go         # Lexical analysis
│   ├── ahoy-bin             # Compiled executable
│   └── README.md            # This file
├── ahoy-lsp/                # Language Server Protocol
│   ├── main.go              # LSP server entry
│   ├── diagnostics.go       # Linting and validation
│   └── ...
├── tree-sitter-ahoy/        # Syntax highlighting
├── vscode-ahoy/             # VS Code extension
├── zed-ahoy/                # Zed editor support
├── examples/                # Example programs
└── test_*.ahoy              # Test files
```

## Features Summary

### Core Language
- ✅ Python-like whitespace syntax
- ✅ Type inference with `:`
- ✅ Optional type annotations with `:type=`
- ✅ Pipe syntax `func|args|` for calls
- ✅ Word-based operators (plus, minus, times, div, mod)
- ✅ F-strings with `f"...{expr}..."`
- ✅ Ternary operator `condition ?? true : false`
- ✅ Arrays with `[...]` syntax
- ✅ Objects with `<key: value>` syntax
- ✅ Dictionaries with `{...}`
- ✅ Enums and pattern matching
- ✅ `ahoy` print shorthand
- ✅ Single-line statements with `;`

### Modern Features (NEW!)
- ✅ **Default arguments**: Optional function parameters
- ✅ **Type annotations**: Explicit typing for safety
- ✅ **Assert statements**: Runtime validation
- ✅ **Defer statements**: Guaranteed cleanup (like Go)
- ✅ **infer/void keywords**: Return type control
- ✅ **LSP support**: Real-time linting and diagnostics
- ✅ **Multiple return values**: `return x, y`
- ✅ **Struct/object syntax**: `<name: "Alice", age: 30>`

## Examples in Repository

### Basic Examples
- `test_simple_assert_defer.ahoy` - Assert and defer basics
- `test_simple_defaults.ahoy` - Default arguments
- `test_typing_simple.ahoy` - Type annotations

### Comprehensive Examples
- `test_assert_defer.ahoy` - All assert/defer patterns
- `test_default_args.ahoy` - Default arguments with validation
- `test_type_annotations.ahoy` - Type system features
- `test_comprehensive.ahoy` - All language features

## Best Practices

### Function Design
```ahoy
? Good: Type-safe with defaults
process :: |data:dict, timeout:float=30.0| bool:
    assert data not is null
    defer cleanup||
    ? ... implementation ...
    return true
```

### Type Annotations
```ahoy
? Use explicit types for:
? - Public APIs
? - Complex data structures
? - Function parameters

? Use inference for:
? - Local variables
? - Obvious literals
```

### Assert & Defer
```ahoy
? Assert for validation
safe_divide :: |a:int, b:int| float:
    assert b not is 0
    return a / b

? Defer for cleanup
process_file :: |filename:string|:
    file: open|filename|
    defer close|file|
    ? ... safe file operations ...
```

## Editor Support

### VS Code
Install the `vscode-ahoy` extension for:
- Syntax highlighting
- LSP integration
- Real-time diagnostics
- Code completion

### Zed
The `zed-ahoy` extension provides:
- Tree-sitter grammar
- Syntax highlighting
- LSP support

## Performance

Ahoy is designed for:
- Fast compilation
- Type safety with minimal overhead
- Optional runtime checks (assertions)
- Efficient resource management (defer)

## Testing

```bash
cd ahoy
go test -v    # Run compiler tests
```

## Community & Support

For more examples and documentation, see:
- `FEATURES_CHEAT_SHEET.md` - Quick syntax reference
- `COMPLETE_IMPLEMENTATION_SUMMARY.md` - All features
- `QUICK_REFERENCE_NEW_FEATURES.md` - New features guide

## Notes

- Arrays now use `[]` not `<>` (updated syntax!)
- Objects/structs use `<>` for named fields
- Dictionaries use `{}` for key-value pairs
- Comments use `?` or `#` 
- Keywords: `halt` (break), `next` (continue)
- Function syntax: `name :: |params| returnType:`
- Default args must come after required params
- Defer executes in LIFO order (Last-In-First-Out)
- Assertions halt execution if false

## Version History

### Latest Features (October 2024)
- ✅ Default function arguments
- ✅ Explicit type annotations
- ✅ Assert statements
- ✅ Defer statements
- ✅ Enhanced LSP with type checking
- ✅ Function call validation

### Core Features
- ✅ F-strings with interpolation
- ✅ Ternary operator
- ✅ Enhanced loop syntax
- ✅ Pattern matching (switch)
- ✅ Enum declarations
- ✅ Multiple return values

Ahoy! 🏴‍☠️

## Testing

Ahoy includes a comprehensive test suite with automated CI/CD.

### Run All Tests
```bash
./run_tests.sh
```

### Run Single Test
```bash
./source/ahoy-compiler -f test/input/arrays_test.ahoy -r
```

### Test Categories
- **arrays_test.ahoy** - Array operations
- **dictionaries_test.ahoy** - Dictionary methods
- **objects_structs_test.ahoy** - Object literals and structs
- **conditionals_test.ahoy** - If and switch statements
- **loops_test.ahoy** - All loop types
- **tuples_test.ahoy** - Tuple operations
- **enums_test.ahoy** - Enum declarations
- **functions_test.ahoy** - Function features

### Continuous Integration

GitHub Actions automatically runs tests on every push to `master`/`main`:
- Compiles all test files
- Executes and validates output
- Stores test artifacts

See [TESTING.md](TESTING.md) for detailed testing documentation.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add/update tests
5. Ensure tests pass: `./run_tests.sh`
6. Submit a pull request

Tests run automatically on PRs via GitHub Actions.

