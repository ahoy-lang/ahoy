# PyLang - Complete Feature Guide

> **Historical:** this guide predates several syntax changes (`anif`,
> `loop:`/`then` loops, `<>` arrays). See `README.md` for the current
> syntax. The word operators it describes were removed outright.

## New Features Summary

### 1. Word-Based Operators Were Removed

Ahoy used to accept readable word operators (`plus`, `minus`, `times`,
`div`, `mod`, `greater_than`, `lesser_than`, ...). They are gone: the
parser rejects them and points at the symbol. Use:

- `+`, `-`, `*`, `/`, `%`, `**`
- `>`, `<`, `>=`, `<=`

**Example:**
```ahoy
result: x + y * z
if count > 10 then
    print|"Too many!"|
$
```



### 4. Inline Conditionals & elif Keyword

**Inline one-line conditionals** keep code concise when the condition and action are simple.

**Syntax:**
```python
# Simple inline if
if condition then statement

# Inline if-else
if condition then statement else other_statement

# Inline if-elif-else chain
if x > 90 then ahoy|"A"|
elif x > 80 then ahoy|"B"|
else ahoy|"C"|

# Mix inline and multi-line
if simple_condition then simple_action|| elif complex_condition then
    complex_action||
    more_code||
else default||
```

**Features:**
- Use `elif` for chaining
- Inline format puts condition and statement on same line
- Multi-line format still supported
- Can mix inline and multi-line in same if-chain
- Formatter preserves inline structure

**Example:**
```ahoy
temperature: 25
if temperature > 30 then ahoy|"Hot\n"| elif temperature > 20 then ahoy|"Warm\n"| else ahoy|"Cold\n"|
```

### 5. Enhanced Loop Syntax

**Multiple loop styles** for different iteration needs.

**Syntax:**
```python
# Range loop: loop from start to end
loop:0 to 10 then
    process||

# While loop: loop with condition
loop:x < 100 then
    process||
    x: x + 1

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

An `else` branch (`else`, `else:` or `else then`, closed by `$` like
every other block) compiles to `#else` — the fallback when the name is
not defined:

```python
when VULKAN then
    use_vulkan_backend||
else
    use_opengl_backend||
```

```c
#ifdef VULKAN
    use_vulkan_backend();
#else
    use_opengl_backend();
#endif
```

Both bodies are parsed and checked regardless of `-D`, so declarations
inside either branch stay visible (same rule as a lone `when` body).
A value-returning function whose `when` and `else` bodies both return
satisfies the must-return check; a lone `when` without `else` still
falls through and needs its tail return.

### 7. Unique-Name Scene Access (`%name`)

Inside a program that has a scene array called `nodes` in scope (the
bell engine convention), `%name` reads the first node with that name
— Godot's `%UniqueName` without the `$` path syntax:

```python
node_set_visible|nodes, %healthbar, false|
heal_node|nodes, %player|
```

It desugars to `node_unique(nodes, "name")` (an integer node id, so it
composes with every `node_*` helper and with `nodes[%x]` reads), which
means the bell engine import must provide `node_unique` and a variable
called `nodes` must be visible, or the checker reports exactly that.
`%` glued to an identifier only parses where a unary expression can
start, so `a % b`, `x % 2` and `a %- 1` keep parsing as modulo. Keep
`%` names unique in a scene (first match wins); the lookup is a plain
linear scan, so unlike a cache it can never go stale across edits,
renames and frees.

## Complete Syntax Reference

### Variables
```python
x: 42
name: "PyLang"
active: true
pi: 3.14
```

### Operators

**Arithmetic** (symbols only):
```ahoy
sum: a + b
diff: a - b
product: a * b
quotient: a / b
remainder: a % b
```

**Comparison** (symbols only):
```ahoy
if x > y then       # greater than
if x < y then       # less than
if x >= y then      # greater than or equal
if x <= y then      # less than or equal
if x is y then      # equality
```

**Boolean:**
```python
result: flag and not other_flag
result: this or that
```

### Control Flow
```ahoy
# Multi-line conditionals
if condition then action||
anif other_condition then other_action||
else default_action||

# Inline conditionals (one-line)
if x is 10 then ahoy|"x is 10\n"|
if y > 10 then ahoy|"Big\n"| else ahoy|"Small\n"|

# Using anif (alternative to elseif)
if score > 90 then ahoy|"A\n"| elif score > 80 then ahoy|"B\n"| else ahoy|"F\n"|

# Mix of inline and multi-line
if condition then action|| anif other_condition then
    multi_line_action||
    more_code||
else default||

# Range loops
loop:0 to 10 then
    process||

# While loops
loop:counter < 10 then
    process||
    counter: counter + 1

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
    return x * y
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

<!--- `<` and `>` symbols work for both comparisons and arrays-->
- When ambiguity exists, use `lesser` and `greater` keywords
- Arrays and dictionaries are zero-indexed
- All code compiles to optimized C
- Memory management handled automatically in generated C code
