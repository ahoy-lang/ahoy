# Loop and F-String Quick Reference

## New Loop Syntax

### Range Loops

```ahoy
? Loop from 1 to 5 (inclusive)
loop i:0 1 to 5 do print|f"i = {i}\n"|

? Loop from 0 to 10 (implicit start at 0)
loop j to 10 do print|f"j = {j}\n"|
```

**Generated:** Standard C `for` loop with counter increment

### Conditional Loops

```ahoy
? Loop with explicit counter and condition
loop k till k less_than 10 do print|f"k = {k}\n"|

? Using comparison operators
loop m till m <= 5 do ahoy|"Still going\n"|
```

**Generated:** C `while` loop with auto-increment of counter variable

### Forever Loops

```ahoy
? Forever loop with counter (must use break)
loop n:
    print|f"Iteration {n}\n"|
    if n is 5 do halt
$

? Forever loop break first loop though
loop:
    ahoy|"Infinite!\n"|
    halt
$
```

**Generated:** C `for(;;)` infinite loop

### Array Iteration

```ahoy
? Iterate over array elements
numbers: [1, 2, 3, 4, 5]
loop num in numbers do print|f"Number: {num}\n"|
```

**Note:** Currently has known type mismatch issues

### Dictionary Iteration

```ahoy
? Iterate over key-value pairs
config: {"host":"localhost", "port":"8080"}
loop key, val in config do print|f"{key} = {val}\n"|
```

**Note:** String format specifiers need improvement

## F-Strings

### Basic Usage

```ahoy
name: "Alice"
age: 25
score: 98.5

? String interpolation
print|f"Name: {name}\n"|
print|f"Age: {age}\n"|
print|f"Score: {score}\n"|
```

### Format Specifiers

F-strings automatically detect the correct format:

| Type   | Format | Example                    |
|--------|--------|----------------------------|
| int    | `%d`   | `f"Count: {count}"`        |
| float  | `%f`   | `f"Pi: {pi}"`              |
| string | `%s`   | `f"Name: {name}"`          |
| char   | `%c`   | `f"Initial: {initial}"`    |

### Multiple Variables

```ahoy
x: 10
y: 20
result: x plus y
print|f"x={x}, y={y}, sum={result}\n"|
```

### In Loops

```ahoy
loop i to 5 do print|f"Iteration #{i}\n"|
```

## If Statement Flexibility

Both `then` and `do` keywords work:

```ahoy
? Using 'then' (traditional)
if x is 5 then ahoy|"Five!"|

? Using 'do' (new style)
if x is 5 do ahoy|"Five!"|

? Inline with 'do'
if done do break
if ready do return result
```

## Complete Example

```ahoy
? Comprehensive example
name: "Bob"
scores: [85, 90, 78, 92, 88]

print|f"Processing scores for {name}\n"|

total: 0
loop score in scores do
    total: total plus score
    print|f"Score: {score}, Running total: {total}\n"|
$

average: total div 5
print|f"Average: {average}\n"|

? Grade determination
loop i till i is 1:
  if average greater_than 90 do
      print|f"Grade: A\n"|
  anif average greater_than 80 do
      print|f"Grade: B\n"|
  else
      print|f"Grade: C\n"|
  $
$
```

## Best Practices

### 1. Always include newlines in print statements
```ahoy
? Good
print|f"Hello {name}\n"|

? Bad (no newline, output runs together)
print|f"Hello {name}"|
```

### 2. Use descriptive loop variable names
```ahoy
? Good
loop index to 10 do
loop item in items do
loop key, value in dict do

? Less clear
loop i to 10 do
loop x in arr do
```

### 3. Break out of infinite loops
```ahoy
? Good
loop counter:
    print|f"{counter}\n"|
    if counter is 100 do break

? Bad (truly infinite, program hangs)
loop do
    ahoy|"Forever!"|
```

### 4. Prefer new syntax for readability
```ahoy
? New style (clear intent)
loop i from 1 to 10 do
    print|f"{i}\n"|

? Old style (still works but less clear)
loop:1 to 10 then
    ahoy|"Count\n"|
```

## Syntax Comparison Table

| Feature              | Old Syntax             | New Syntax              |
|----------------------|------------------------|-------------------------|
| Range loop           | `loop:1 to 10 then`    | `loop i from 1 to 10 do`|
| While loop           | `loop:x < 10 then`     | `loop x till x < 10 do` |
| Forever loop         | `loop:0 then`          | `loop do` or `loop i:`  |
| If statement         | `if x is 5 then`       | `if x is 5 do`          |
| String interpolation | `ahoy|"x=%d\n", x|`    | `print|f"x={x}\n"|`     |

## Common Patterns

### Countdown
```ahoy
loop i from 10 to 1 do
    print|f"{i}...\n"|
print|f"Blast off!\n"|
```

### Accumulation
```ahoy
total: 0
loop i to 100 do total: total plus i
print|f"Sum: {total}\n"|
```

### Search with Early Exit
```ahoy
found: false
loop i to 100:
  if i is target:
      found: true
      halt
	$
$
$

```

### Nested Loops
```ahoy
loop row to 3:
    loop col to 3:
        print|f"({row},{col}) "|
    ahoy|"\n"|
  $
$
```

## Notes

- All new syntax is **fully backward compatible**
- Old loop syntax still works unchanged
- F-strings compile to C `sprintf` calls
- Loop counters auto-initialize to 0 and auto-increment
- Use `halt` to exit loops early
- Use `next` to continue to next iteration

## Status

✓ **Implemented and working** (see NEW_LOOP_AND_FSTRING_IMPLEMENTATION.md for details)
