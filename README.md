# Ahoy Programming Language

A modern, expressive programming language with clean syntax and powerful features including default arguments, type annotations, assertions, and deferred execution.

## Table of Contents

- [Features](#features)
- [Quick Start](#quick-start)
  - [Installation](#installation)
  - [Build](#build)
  - [Usage](#usage)
- [Language Syntax](#language-syntax)
  - [Variables & Type Annotations](#variables--type-annotations)
  - [Print Statements & F-Strings](#print-statements--f-strings)
  - [Operators](#operators)
  - [Control Flow](#control-flow)
  - [Functions with Default Arguments](#functions-with-default-arguments-new)
  - [Assert Statements](#assert-statements-new)
  - [Defer Statements](#defer-statements-new)
  - [Arrays](#arrays)
  - [Objects/Structs](#objectsstructs)
  - [Dictionaries](#dictionaries)
  - [Enums](#enums)
- [Advanced Features](#advanced-features)
  - [Pattern Matching (Switch)](#pattern-matching-switch)
  - [Type System](#type-system)
  - [Programs (Multi-file)](#programs-multi-file)
  - [Importing C Headers](#importing-c-headers)
  - [Built-in Functions](#built-in-functions)
  - [LSP Features](#lsp-features)
- [CLI Reference](#cli-reference)
- [File Extension](#file-extension)
- [Language Reference Card](#language-reference-card)
- [Project Structure](#project-structure)
- [Features Summary](#features-summary)
- [Examples in Repository](#examples-in-repository)
- [Best Practices](#best-practices)
- [Editor Support](#editor-support)
- [Testing](#testing)
- [Contributing](#contributing)

## Features

- **Clean Syntax**: Python-inspired with whitespace indentation
- **Type System**: Optional type annotations with inference
- **Default Arguments**: Flexible function parameters
- **Assert & Defer**: Runtime validation and guaranteed cleanup
- **Pipe Syntax**: Function calls with `|args|`
- **Arrays**: Built-in with `[]` syntax
- **Objects/Structs**: C-style with `{}` syntax
- **Dictionaries**: Hash maps with `<>` syntax
- **F-Strings**: String interpolation with `f"...{var}..."`
- **LSP Support**: Real-time linting and validation
- **Single-Line Statements**: Use `;` to combine statements
- **`ahoy` Keyword**: Shorthand for print statements
- ** $ to close block scopes


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
items:array[int]= [1, 2, 3]
config:dict<string,string>= {"host": "localhost"}

? Constants with types
MAX_SIZE::int= 100
API_KEY::string= "secret"
TIMEOUT:: 30  ? Inferred as int
```

### Assignment Operators

Ahoy has three assignment operators with distinct purposes:

```ahoy
? Declaration with `:` (colon)
? - Creates a new variable
? - Can be used for both initial value and type annotation
x: 42
name: "Alice"
score:int= 100

? Reassignment with `=` (equals)
? - Updates an existing variable
? - Cannot create new variables
x = 50
name = "Bob"

? Walrus operator `:=` (declare-or-assign)
? - Smart operator that declares OR assigns
? - Useful for tuple unpacking and multiple returns
? - Automatically handles function returns with multiple values
a, b := function_returning_two_values||
x, y := 10, 20

? Examples showing the difference
counter: 0        ? Declaration
counter = counter + 1    ? Reassignment
counter = 10      ? Reassignment

? Multiple assignment with walrus
result, error := parse_int|"42"|
x, y, z := 1, 2, 3
```

**Key Rules:**
- Use `:` for declaring new variables
- Use `=` for updating existing variables  
- Use `:=` for tuple unpacking or when you want smart declare-or-assign behavior
- Constants use `::` and cannot be reassigned

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
$

? If anif statements
if   condition then action||
anif other_condition then another_action||
else no_action||
$


? Switch statements
switch value:
    on 1:
        ahoy |"One"|
    on 2:
        ahoy |"Two"|
    _:
        ahoy |"Other"|
$

? Loops (halt = break, next = continue)
loop i:0 to 10 do
    if i is 5
        next  ? Skip 5
    if i is 8
        halt  ? Stop at 8
    ahoy |f"i = {i}"|
$

? loops till condition met
loop i:0 till i is 5 do
    print|i|
$

? Loop over array
numbers: [1, 2, 3, 4, 5]
loop num in numbers
    ahoy |f"Number: {num}"|
$

? Loop over dict
data: {"x": 10, "y": 20}
loop key, value in data
    ahoy |f"{key} = {value}"|
$
```

### Functions with Default Arguments (NEW!)

```ahoy
? Function with default parameters
@ greet :: |name:string, greeting:string="Hello", punctuation:string="!"| string:
    return f"{greeting} {name}{punctuation}"
$

? Call with different argument counts
msg1: greet|"Alice"|                    ? Uses defaults
msg2: greet|"Bob", "Hi"|                ? Partial override
msg3: greet|"Charlie", "Hey", "!!!"|   ? All explicit

? Return type keywords
@ calculate :: |x:int, y:int| infer:     ? Infer return type
    return x * y, x + y
$

@ log :: |message:string| void:          ? No return value
    ahoy |message|
$

@ log2 :: |message:string|:              ? Implicit void
    ahoy |message|
$
```

### Assert Statements (NEW!)

```ahoy
? Basic assertions
assert x > 0
assert count is 10
assert name not is ""

? Precondition checking
@ divide :: |a:int, b:int| float:
    assert b not is 0           ? Prevent division by zero
    return a / b
$

? Validation pipeline
@ validate_user :: |user:dict|:
    assert "name" in user
    assert "email" in user
    assert len|user["name"]| > 0
    assert "@" in user["email"]
    ahoy |"User valid!"|
$
```

### Defer Statements (NEW!)

```ahoy
? Basic defer (executes at function exit)
@ greet :: |name:string|:
    defer ahoy |"Goodbye!"|
    ahoy |f"Hello, {name}!"|
    ahoy |"Nice to meet you"|
? Output: Hello, Alice! / Nice to meet you / Goodbye!
$

? Resource cleanup
@ process_file :: |filename:string|:
    ahoy |f"Opening {filename}"|
    defer ahoy |f"Closing {filename}"|
    ? ... file operations ...
    ? File automatically "closed" when function exits
$

? Multiple defers (LIFO - Last In First Out)
@ demo :: ||:
    defer ahoy |"Third (executes first)"|
    defer ahoy |"Second"|
    defer ahoy |"First (executes last)"|
    ahoy |"Main function"|
$

? Defer with return values
@ calculate :: |x:int, y:int| int:
    defer ahoy |"Calculation completed"|
    result: x * y
    return result
$
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
$
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
  x: float ? initialized to 0 by default
  y: float
  10 default_value:int ? Field with default value initilized to 10
  #my_static_value:int ? Static field accessable with Point.#my_static_value
  42 CONST_VALUE:int ? Constant field
  type Polygon: ? struct type inherits properties from Point; to create object Point.Polygon{}
    3 sides:int
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
? color with initial values
struct color_with_init_values:
  255 r: int,
  200 g: int,
  200 b: int,
  0   a: int
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
$

? Methods
has_theme: settings.has|"theme"|  ? Returns 1 or 0
```

### Enums

```ahoy
enum Color:
    RED
    GREEN
    BLUE
$

enum Status:
    0 PENDING
    1 ACTIVE
    2 DONE
$

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
@ process_data :: |data:dict, timeout:float=TIMEOUT, retries:int=MAX_RETRIES| bool:
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
    $
    
    ? Postcondition validation
    assert attempt <= retries
    assert attempt > 0
    
    return success
$

? Main execution
config:dict= <
    "id": 12345,
    "name": "test_data",
    "priority": "high"
>

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
$

? Ternary operator
max_retries: 5
mode: max_retries > 3 ?? "aggressive" : "normal"
ahoy |f"Mode: {mode}"|
```

## Advanced Features

### Pattern Matching (Switch)

```ahoy
switch expression:
    on value1:
        ? code for value1
    on value2, value3:
        ? code for multiple values
    on _:
        ? default case
$
```

### Type System

**Supported Types:**
- `int` - Integers
- `float` - Floating point
- `string` - Text strings
- `raw_string` - Raw strings (backticks, preserves newlines)
- `bool` - Booleans (true/false)
- `char` - Single characters
- `array` - Arrays/lists
- `dict` - Dictionaries/maps
- `vector2` - 2D vectors
- `color` - Color values
- `infer` - Inferred return type (functions)
- `void` - No return value (functions)

### Programs (Multi-file)

Programs allow you to organize code across multiple files. Use `program program_name` at the top of each file to group them together.

```ahoy
? File: main.ahoy
program my_game

@ main ||:
    ? Entry point - global vars must be declared here, not in global scope
    player_score: 0
    game_loop||
$
```

```ahoy
? File: utils.ahoy  
program my_game

@ game_loop ||:
    ? This function is available to main.ahoy because same program name
    ahoy |"Game running!"|
$
```

**Important Rules:**
- All files with the same `program` declaration in the same folder are compiled together
- The `main` function is the entry point
- Global variables are NOT allowed in program files - declare them in the `main` function
- Constants (`::`) ARE allowed in global scope

### Importing C Headers

You can import C header files directly to use C functions and types:

```ahoy
? Import a C header file
import "/home/user/Documents/raylib/src/raylib.h"

? Now you can use raylib functions (converted to snake_case)
init_window|800, 600, "My Game"|
```

The compiler automatically:
- Parses structs, functions, and enums from the header
- Converts PascalCase function names to snake_case (e.g., `InitWindow` → `init_window`)
- Makes types available for use in your code

### Built-in Functions

| Function | Description | Example |
|----------|-------------|---------|
| `print\|...\|` | Print to stdout | `print\|"Hello"\|` |
| `ahoy\|...\|` | Alias for print | `ahoy\|"World"\|` |
| `len\|x\|` | Get length of array, dict, or string | `len\|my_array\|` |
| `length\|x\|` | Alias for len | `length\|my_dict\|` |
| `int\|x\|` | Convert to integer | `int\|"42"\|` |
| `float\|x\|` | Convert to float | `float\|"3.14"\|` |
| `string\|x\|` | Convert to string | `string\|42\|` |

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
```

### Syntax Summary

| Feature | Syntax | Example |
|---------|--------|---------|
| Variable | `name: value` | `x: 42` |
| Variable (typed) | `name:type= value` | `age:int= 29` |
| Constant | `NAME: value` | `MAX:: 100` |
| Constant (typed) | `name::type= value` | `MAX::int= 100` |
| Function | `name :: \|params\| type:` | `add :: \|a:int, b:int\| int:` |
| Default arg | `param:type=default` | `timeout:float=30.0` |
| Array | `[item1, item2, ...]` | `[1, 2, 3]` |
| Dict | `<key: value, ...>` | `<name: "Alice", age: 30>` |
| Object | `{"key": value, ...}` | `{"x": 10, "y": 20}` |
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
@ process :: |data:dict, timeout:float=30.0| bool:
    assert data not is null
    defer cleanup||
    ? ... implementation ...
    return true
$
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
@ safe_divide :: |a:int, b:int| float:
    assert b not is 0
    return a / b
$

? Defer for cleanup

@ process_file :: |filename:string|:
    file: open|filename|
    defer close|file|
    ? ... safe file operations ...
$
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
- Autofree memory management (eventually)

## Testing

```bash
cd ahoy
go test -v    # Run compiler tests
```

## Notes

- Array use `[]`
- Objects/structs use `{}` for named fields
- Dictionaries use `<>` for key-value pairs
- Comments use `?`
- Keywords: `halt` (break), `next` (continue)
- Function syntax: `name |params| returnType:`
- Default args must come after required params
- Defer executes in LIFO order (Last-In-First-Out)
- Assertions halt execution if false

### Core Features
- ✅ F-strings with interpolation
- ✅ Ternary operator
- ✅ Enhanced loop syntax
- ✅ Pattern matching (switch)
- ✅ Enum declarations
- ✅ Multiple return values
- ✅ Default function arguments
- ✅ Explicit type annotations
- ✅ Assert statements
- ✅ Defer statements
- ✅ Enhanced LSP with type checking
- ✅ Function call validation

Ahoy! 🏴‍☠️

## Testing

### Run All Tests
```bash
cd into folder: ahoy/test
$ go test -v
```

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
5. Ensure tests pass: ahoy/test go run test -v
6. Submit a pull request

Tests run automatically on PRs via GitHub Actions.
