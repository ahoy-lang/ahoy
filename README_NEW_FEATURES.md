# PyLang - Complete Feature Guide

## New Features Summary

### 1. Word-Based Operators

You can now use readable word-based operators alongside symbols:

**Arithmetic:**
- `plus` instead of `+`
- `minus` instead of `-`
- `times` instead of `*`
- `div` instead of `/`
- `mod` instead of `%`

**Comparison:**
- `greater` instead of `>`
- `lesser` instead of `<`

**Example:**
```python
result: x plus y times z
if count greater 10 then
    print|"Too many!"|
```

### 2. Dynamic Arrays

Arrays with automatic resizing, compiled to efficient C code.

**Syntax:**
```python
# Declaration with angle brackets
my_array: <100, 200, 300, 400>

# Access with angle brackets
first: my_array<0>
last: my_array<3>
```

**Features:**
- Zero-indexed
- Automatic capacity management
- Compiled to C with malloc/realloc

### 3. Python-Like Dictionaries

Hash maps with Python-style syntax.

**Syntax:**
```python
# Declaration
config: {"name":"PyLang", "version":2, "active":true}

# Access with curly braces
app_name: config{"name"}
version_num: config{"version"}
is_active: config{"active"}
```

**Features:**
- String keys
- Mixed value types
- HashMap implementation in C

### 4. Inline Conditionals & anif Keyword

**Inline one-line conditionals** keep code concise when the condition and action are simple.

**Syntax:**
```python
# Simple inline if
if condition then statement

# Inline if-else
if condition then statement else other_statement

# Inline if-anif-else chain
if x greater 90 then ahoy|"A"| anif x greater 80 then ahoy|"B"| else ahoy|"C"|

# Mix inline and multi-line
if simple_condition then simple_action|| anif complex_condition then
    complex_action||
    more_code||
else default||
```

**Features:**
- Use `anif` instead of `elseif` for cleaner chaining (both work)
- Inline format puts condition and statement on same line
- Multi-line format still supported
- Can mix inline and multi-line in same if-chain
- Formatter preserves inline structure

**Example:**
```python
temperature: 25
if temperature greater 30 then ahoy|"Hot\n"| anif temperature greater 20 then ahoy|"Warm\n"| else ahoy|"Cold\n"|
```

### 5. Enhanced Loop Syntax

**Multiple loop styles** for different iteration needs.

**Syntax:**
```python
# Range loop: loop from start to end
loop:0 to 10 then
    process||

# While loop: loop with condition
loop:x lesser_than 100 then
    process||
    x: x plus 1

# Array iteration: loop through array elements
numbers: <10, 20, 30>
loop num in numbers then
    ahoy|"Number: %d\n", num|

# Dictionary iteration: loop through key-value pairs
config: {"name":"Ahoy", "version":"1.0"}
loop key,val in config then
    ahoy|"%s: %s\n", key, val|
```

**Features:**
- Range loops with `loop:start to end`
- While loops with `loop:condition`
- Array iteration with `loop element in array`
- Dictionary iteration with `loop key,value in dict`
- Count loops with `loop:start` (can specify starting point)
- Backward compatible with old loop syntax

**Example:**
```python
? Range loop
loop:0 to 5 then
    ahoy|"Iteration\n"|

? Array iteration
items: <100, 200, 300>
loop item in items then
    ahoy|"Item: %d\n", item|

? Dictionary iteration
data: {"status":"ok", "count":"42"}
loop k,v in data then
    ahoy|"Key=%s, Value=%s\n", k, v|
```

### 6. Compile-Time Conditionals

Use `when` keyword for C preprocessor directives.

**Syntax:**
```python
when DEBUG then
    print|"Debug info here\n"|

when PRODUCTION then
    optimize_code||
```

**Compiles to:**
```c
#ifdef DEBUG
    printf("Debug info here\n");
#endif
```

## Complete Syntax Reference

### Variables
```python
x: 42
name: "PyLang"
active: true
pi: 3.14
```

### Operators

**Arithmetic** (symbol or word):
```python
sum: a + b          # or: a plus b
diff: a - b         # or: a minus b
product: a * b      # or: a times b
quotient: a / b     # or: a div b
remainder: a % b    # or: a mod b
```

**Comparison** (symbol or word):
```python
if x > y then       # or: x greater y
if x < y then       # or: x lesser y
if x >= y then      # or: x >= y
if x <= y then      # or: x <= y
if x is y then      # equality
```

**Boolean:**
```python
result: flag and not other_flag
result: this or that
```

### Control Flow
```python
# Multi-line conditionals
if condition then
    action||
elseif other_condition then
    other_action||
else
    default_action||

# Inline conditionals (one-line)
if x is 10 then ahoy|"x is 10\n"|
if y greater 10 then ahoy|"Big\n"| else ahoy|"Small\n"|

# Using anif (alternative to elseif)
if score greater 90 then ahoy|"A\n"| anif score greater 80 then ahoy|"B\n"| else ahoy|"F\n"|

# Mix of inline and multi-line
if condition then action|| anif other_condition then
    multi_line_action||
    more_code||
else default||

# Range loops
loop:0 to 10 then
    process||

# While loops
loop:counter lesser_than 10 then
    process||
    counter: counter plus 1

# Array iteration
loop element in my_array then
    process_element||

# Dictionary iteration
loop key,value in my_dict then
    process_pair||
```

### Functions
```python
func calculate|x int, y int| int then
    return x times y
```

### Arrays
```python
numbers: <1, 2, 3, 4, 5>
first: numbers<0>
last: numbers<4>
```

### Dictionaries
```python
person: {"name":"Alice", "age":30, "city":"NYC"}
name: person{"name"}
age: person{"age"}
```

### Function Calls
```python
print|"Hello, %s!\n", name|
result: calculate|10, 20|
no_args_function||
```

## Usage

```bash
# Compile standard programs
./compile.sh your_program.py
./output/your_program

# Compile with Raylib
./compile_raylib.sh graphics_program.py
./output/graphics_program

# Run tests
go test -v

# With compile-time flags
gcc -DDEBUG -o output/program output/program.c -lm
```

## Notes

- `<` and `>` symbols work for both comparisons and arrays
- When ambiguity exists, use `lesser` and `greater` keywords
- Arrays and dictionaries are zero-indexed
- All code compiles to optimized C
- Memory management handled automatically in generated C code
