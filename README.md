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
- Go 1.27+
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

**Bitwise** (words only; `&`, `^` and `|` already mean address-of,
dereference and a call's pipes). Precedence follows C: `band` binds tighter
than `bxor`, which binds tighter than `bor`; shifts bind tighter than
comparison and looser than `+`:
```ahoy
flags: LAYER_UI band MASK     ? and
merged: a bor b               ? or
flipped: a bxor TOGGLE_BIT    ? xor
inv: bnot flags               ? not
shifted: 1 shl 4              ? left  (16)
back: value shr 2             ? right
packed: r shl 16 bor g shl 8 bor b   ? colour packing
```

**Comparison** (symbols or words):
```ahoy
if x > y then       ? or: x greater_than y
if x < y then       ? or: x lesser_than y
if x is y then      ? equality
if x is not y then  ? inequality
```

**Match lists** (one subject, many values):
```ahoy
? x is A, B, C means x is A or x is B or x is C
if kind is NodeKind.rect, NodeKind.shape2d, NodeKind.button2d then

? x is not A, B means x is not A and x is not B (none of them)
if cur.kind is not NodeKind.dead, NodeKind.hidden then
```
The subject is evaluated once. Strings still compare by value, so
`name is "alice", "bob"` is a `strcmp` against each. Inside array/object/dict
literals commas separate elements, so write `x is A or x is B` there
(`[x is A, B]` stays two elements). For whole blocks of alternatives prefer
`switch` with `on A, B:` cases (see docs/SWITCH_STATEMENT.md).

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

? If/elif (else-if) statements
if   condition then action||
elif other_condition then another_action||
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

? Loops till condition met
loop i:0 until i is 5 do
    print|i|
$

? Loop over array
numbers: [1, 2, 3, 4, 5]
loop num in numbers do
    ahoy |f"Number: {num}"|
$

? Loop over dict
data: {"x": 10, "y": 20}
loop key, value in data do
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
@ calculate :: |x:int, y:int| int,int:  ? Explicit types; comma-separate multiple returns
    return x * y, x + y
$

@ log :: |message:string| void:          ? No return value
    ahoy |message|
$

@ log2 :: |message:string|:              ? Implicit void
    ahoy |message|
$

? A call is not allowed as an argument to another call: name the result first
count: len|items|
ahoy |f"items: {count}"|

? Inequality is written 'is not'. 'not is' is rejected.
if name is not "" then
    ahoy |"has a name"|
$
```

### Function Values

`&name` takes a reference to a function, and a lambda `(x:int): expression` is
a value too. A lambda captures the enclosing locals it uses, so a closure can
keep a value alive after the function that made it returns.

```ahoy
@ double_it :: |x:int| int:
    return x * 2
$

? A function value is a parameter like any other
@ apply :: |f:fn(int)->int, v:int| int:
    return f|v|
$

f: fn(int)->int = &double_it        ? reference to a named function
g: fn(int)->int = (x:int): x + 1    ? stored lambda

base: 10
add_base: fn(int)->int = (x:int): x + base   ? captures base

r1: apply|f, 3|      ? 6
r2: g|41|            ? 42
r3: add_base|5|      ? 15
```

`fn(params)->ret` names a function value. A reference to a named function (or a
lambda that captures nothing) is a plain C function pointer; a capturing lambda
carries a small environment with it, so it is only callable from Ahoy. Function
values must return a single value and named functions used as values need typed
parameters.

An imported C API that takes a callback typedef can be handed an Ahoy function
directly:

```ahoy
import "/path/to/raylib.h"

@ my_log :: |level:int, text:string| void:
    ahoy |f"log: {text}"|
$

? raylib.h declares: typedef void (*TraceLogCallback)(int, const char*, va_list);
set_trace_log_callback|&my_log|
```

A C callback typedef can also be a variable's type. The value is a raw C
function pointer, so it can be called directly and passed to C APIs:

```ahoy
? raylib.h declares: typedef char *(*LoadFileTextCallback)(const char *fileName);
@ my_load :: |name:string| string:
    return "hello from ahoy"
$

loader: LoadFileTextCallback = &my_load
text: loader|"file.txt"|              ? a direct call through the pointer
set_load_file_text_callback|loader|    ? or hand the value to a C API
```

### Callback Tables (backend contracts)

A struct whose fields are function values is a contract. Each backend fills in
the same struct, and engine code only calls the fields, so swapping a backend is
passing a different value:

```ahoy
struct Renderer:
    clear: fn(int,int,int)->int
    draw:  fn(int,int)->int
    name:  string
$

@ raylib_clear :: |r:int,g:int,b:int| int:
    return 100 + r + g + b
$

@ raylib_backend :: || Renderer:
    return Renderer{clear: &raylib_clear, draw: &raylib_draw, name: "raylib"}
$

@ render_frame :: |r:Renderer|:
    print|r.name|
    r.clear|0, 0, 0|
    r.draw|10, 20|
$
```

`r.clear|...|` dispatches through the stored function value, so `render_frame`
never mentions raylib, SDL or Vulkan: swapping the backend is passing a
different `Renderer`. Backends are selected at build time (they are different C
libraries): put each behind a compile-time branch and choose it with `-D`.

```ahoy
? ahoy -f game.ahoy -r -D BACKEND_RAYLIB
when BACKEND_RAYLIB then
    use_raylib_backend||
$
when BACKEND_VULKAN then
    use_vulkan_backend||
$
```

`-D` accepts several names (repeat the flag or comma-separate them).

### Jobs (worker pool)

`jobs|n|` starts a pool of `n` worker threads. `pool.submit|fn, arg|` queues a
task and returns a job; `pool.wait|job|` blocks until it finishes and returns
its result. A task is a function value taking one argument (pass `0` when it
needs none); results are pointer-sized, so ints, strings, arrays and pointers
work, while floats in the task ABI are not supported yet.

```ahoy
@ square :: |n:int| int:
    return n * n
$

pool: pool = jobs|4|
j1: job = pool.submit|&square, 12|
j2: job = pool.submit|&square, 13|
r1: pool.wait|j1|        ? 144
r2: pool.wait|j2|        ? 169
pool.stop||
```

This is the asset-loading pattern: workers read and decode files, and the main
thread calls `wait` and then uploads the result to the GPU. Workers must not
call raylib/OpenGL — the graphics context belongs to the main thread. A pool is
stopped when its variable leaves scope.

For a game loop, draining finished jobs is nicer than blocking. `j.done||`
polls without consuming, `j.result||` reads a finished result, and
`pool.drain|handler, context|` hands every job that finished since the last
drain to `handler(job, context)` on the calling thread, then consumes them:

```ahoy
@ collect :: |j:job, acc:array| void:
    r: j.result||
    acc.push|r|
$

acc: []
got: 0
loop while got is not 5 do
    got = got + pool.drain|&collect, acc|
$
? (You can also write `loop until got is 5 do ... $` instead of
? `loop while got is not 5 do ... $`.)
```

Jobs are owned by the pool until they are consumed: `wait` consumes its job,
`drain` consumes each job it hands out, and jobs nobody consumed are freed when
the pool is destroyed (its variable leaves scope). Do not use a job handle
after `wait` or after its drain handler has run.

### Assert Statements (NEW!)

```ahoy
? Basic assertions
assert x > 0
assert count is 10
assert name is not ""

? Precondition checking
@ divide :: |a:int, b:int| float:
    assert b is not 0           ? Prevent division by zero
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
loop num in numbers do
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
  default_value:int = 10 ? Field with default value initilized to 10
  #my_static_value:int ? Static field accessable with Point.#my_static_value
  CONST_VALUE:int = 42 ? Constant field
  type Polygon: ? struct type inherits properties from Point; to create object Point.Polygon{}
    sides:int = 3
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
  r: int = 255,
  g: int = 200,
  b: int = 200,
  a: int = 0
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
loop key, value in settings do
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

### Methods

Go-style receivers. A method is a function whose first parameter is the
receiver; `recv.m|args|` calls the mangled `Type_m` with the receiver
prepended. Associated functions (`@ T.name`, no receiver) are called as
`Type.name|args|`:

```ahoy
struct Point:
    x: float
    y: float
$

@ Point.make |x:float, y:float| Point:
    return Point{x: x, y: y}
$

@ (p Point) move |dx:float, dy:float| Point:
    p.x = p.x + dx
    p.y = p.y + dy
    return p
$

p: Point.make|1.0, 2.0|
q: p.move|3.0, 4.0|
```

Rules: the receiver type must be a struct, enum, builtin or compound
builtin shape (`array[Node]` works, so scene-style `nodes.set_pos|id, …|`
is expressible); the call resolves against struct callback fields first,
then user methods, then builtins, and an unknown method on a type with
methods is a compile error. Value receivers behave like value parameters:
return the new value (`n = n.moved|dx, dy|`); only handle receivers
(arrays, pools) mutate in place. See `docs/METHODS.md`.

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
    assert data is not null
    assert timeout > 0
    assert retries > 0
    
    ? Setup with guaranteed cleanup
    ahoy |f"Processing data with timeout={timeout}, retries={retries}"|
    defer ahoy |"Processing completed"|
    
    ? Type-annotated variables
    attempt:int= 0
    success:bool= false
    
    ? Processing loop
    loop i:1 to retries do
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
loop num in numbers do
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
    on 1 to 5:
        ? code for a range (inclusive, first match wins)
    on _:
        ? default case
$
```

A switch can also produce a value (`out: string = switch x: ...`) or be
returned directly (`return switch x: ...`). See
[docs/SWITCH_STATEMENT.md](docs/SWITCH_STATEMENT.md) for the full reference.

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

```text
./ahoy-bin -f <file.ahoy> [options]

Options:
  -f <file>        Input .ahoy source file (required)
  -r               Run the compiled program
  -lint            Run in lint-only mode (check for errors)
  -D NAME          Define NAME for `when NAME:` blocks (repeatable, comma-separated)
  -cache           Enable incremental builds (skips re-parse and re-validation when nothing changed)
  -no_semantics    Skip semantic validation (type errors, undeclared names, duplicate definitions)
  -format          Format the source file
  -release         Use optimizing compiler (gcc/clang) for release build
  -show_leaks      Enable memory leak detection with AddressSanitizer
  -target <t>      Cross-compile target: linux, windows, web, macos, or all
  -hotreload       Hot code reloading: keeps window open, reloads on file changes (development only)
  -coldreload      Cold reload: auto-recompile and restart on file changes (development only)
  -h               Show help message
```

Every build prints a timing breakdown so slow phases are visible:

```text
Time: token=7.6ms, parse=14.1ms, lint=27.4ms, codegen=12.9ms, cache=0.02ms
```

- `token` covers tokenizing all files, `parse` the parse plus package
  loading, `lint` the whole-program semantic validation, `codegen` C
  generation. With `-cache`, repeat builds with no changes skip re-parse
  and re-validation entirely.

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
| Loop | `loop var:start to end do` | `loop i:0 to 10 do` |
| Loop (array) | `loop item in array do` | `loop x in nums do` |
| Loop (dict) | `loop key, val in dict do` | `loop k, v in data do` |
| Break | `halt` | `if done halt` |
| Continue | `next` | `if skip next` |
| Switch | `switch x:` + `on` per case + `$` | `switch x:` / `on 1:` / `_: ...` / `$` |
| Switch range | `on lo to hi:` (inclusive) | `on 1 to 5:` |
| Match list | `x is A, B` / `x is not A, B` | `if k is 1, 2, 3 then` |
| Method | `@ (recv T) name \|…\|` + `recv.name\|…\|` | `@ (p Point) move \|dx\| Point:` / `p.move\|1.0\|` |
| Associated | `@ T.name \|…\|` + `T.name\|…\|` | `@ Point.make \|x, y\| Point:` / `Point.make\|1.0, 2.0\|` |
| Switch value | `out: T = switch x: ...` | `out: string = switch day: on 1: "Mon" _: "?" $` |

## Project Structure

```
ahoy-lang/
├── ahoy/                    # Core compiler
│   ├── source/              # Compiler frontend and codegen (main.go, package.go, codegen.go, ...)
│   ├── pkg/                 # Language core: tokenizer, parser, checker, C header parsing
│   ├── test/                # Test corpus (input/*.ahoy) and Go test suites
│   ├── docs/                # Per-feature language references
│   ├── ahoy-bin             # Compiled executable
│   └── README.md            # This file
├── ahoy-lsp/                # Language Server Protocol
│   ├── main.go              # LSP server entry
│   ├── diagnostics.go       # Linting and validation
│   └── ...
├── tree-sitter-ahoy/        # Syntax highlighting
├── vscode-ahoy/             # VS Code extension
├── zed-ahoy/                # Zed editor support
└── bell/                    # Example raylib engine, games, and scene editor
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
- ✅ **void keyword and explicit return types**: Return type control
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
    assert data is not null
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
    assert b is not 0
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

Ahoy compiles through C, and the frontend reports where every build spends
its time (`token` / `parse` / `lint` / `codegen`, see CLI Reference). For
reference, a ~6,000-line multi-file program compiles its frontend in about
65ms on a development machine. Two properties keep builds fast and
repeatable:

- **Incremental builds (`-cache`)**: parsed files are cached, and
  whole-program validation is skipped when no input changed (tracked by a
  fingerprint over file contents, `-D` defines, and C headers).
- **Deterministic output**: the generated C is byte-identical across runs
  for the same inputs, so rebuilds and diffs are trustworthy.

## Testing

```bash
cd ahoy/pkg && go test ./...      # Tokenizer, parser, checker unit tests
cd ahoy/source && go test ./...   # Compiler frontend tests
cd ahoy/test && go test ./...     # Lint + end-to-end: every input/*.ahoy is compiled with gcc and run
```

Test programs live in `ahoy/test/input/`: each declares `expected: []`
inside `@ main`, pushes expected values as it prints them, and ends with
`print|expected|` so the runner can compare output line by line.

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
