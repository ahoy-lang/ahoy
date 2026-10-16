
# New Loop Syntax - IMPLEMENTED ✓

This document describes the new loop syntax that has been implemented in Ahoy.

## Status

**Implementation Complete**: The new loop syntax and f-strings have been successfully implemented.

### Working Features ✓
- `loop i to 5:` - Range loop with explicit start and end
- `loop i to 5:` - Range loop from 0 to end
- `loop i while condition:` - Conditional loop with explicit counter variable
- `loop i do` - Forever loop with explicit counter (use with halt)
- `loop do` - Forever loop without counter (`loop while true do` says so explicitly; bare `loop:` was removed)
- `loop while COND do` - Condition loop: runs while COND holds
- `loop i while COND do` - Counter (from 0) plus condition

> There is one loop keyword: `loop`. The old `till` condition word was
> removed (it read as *until* but ran as *while*): `loop till` is a
> parse error pointing at `loop while`. Bare `while` is not a statement;
> conditions always hang off `loop`.
- F-strings: `print|f"hello{i}\n"|` - String interpolation with variables
- Support for both `then` keywords in if statements

### Block Terminator Rules for Loops

**One-line loops with `do` keyword do NOT require `$`:**
```ahoy
loop i:0 while i < 5 do print|f"Value: {i}\n"|
```

**Multi-line loops with `:` (colon) REQUIRE `$`:**
```ahoy
loop i to 5 do
    print|f"Count {i}\n"|
$
```

### Known Issues
- Array iteration (`loop val in values do`) has a type mismatch bug between `AhoyArray` and `DynamicArray`
- Dictionary iteration f-string format specifiers need adjustment for string types
- F-string compilation produces warnings (format security) but works correctly

## Examples

```ahoy
? Loop range from 1 to 5
loop i:1 to 5 do
    print|f"Iteration {i}\n"|
$

struct test:
	type sample:
		data:int
$

? Loop range from 0 to 5
loop i to 5 do
    print|f"Count {i}\n"|
$

? Loop range from 0 to 5
loop i:0 to 5 do
    print|f"Count {i}\n"|
$

? Loop conditional with word operators
loop i:0 while i < 5 do print|f"Value: {i}\n"|

? Loop conditional with symbol operators
number:2
loop while number <= 5 do
    ahoy|"Count\n"|
    number+=1

values: [0, 1, 2, 3, 4, 5, 6, 7, 8, 9]
loop val:0 to values.length
    print|f"Value: {val}\n"|

? Loop forever with explicit counter dont run this as it will infinite loop
loop i:0 do
    print|f"Loop {i}\n"|
    if i is 5 do halt

? Loop forever without counter; dont run this
loop do
    ahoy|"Forever!\n"|

? Alternative syntax (also works)
loop i:0 while i less_than 5 do
    ahoy|"Count\n"|

? Alternative syntax (also works)
loop i while i < 5 do
    ahoy|"Count\n"|

? Array iteration (TODO: Fix type mismatch)
values: [0, 1, 2, 3, 4, 5, 6, 7, 8, 9]
loop val in values do
    print|f"Value: {val}\n"|

? Dictionary Iteration
config: {"name":"Ahoy", "version":"1.0", "active":"yes"}
loop key,val in config do
    print|f"Key: {key}, Value: {val}\n"|
```

## F-String Syntax

F-strings provide Python-like string interpolation:

```ahoy
name: "World"
count: 42
print|f"Hello {name}, count is {count}\n"|
```

Format specifiers are automatically determined based on variable type:
- `int` → `%d`
- `float` → `%f`
- `string` → `%s`
- `char` → `%c`

## Implementation Details

### Parser Changes
- Added `TOKEN_TILL`, `TOKEN_PRINT`, `TOKEN_F_STRING` tokens
- Updated `parseLoop()` to handle all new syntax variations
- Added `parsePrintStatement()` for print statements
- Updated if statements to accept both `then` and `do` keywords

### Codegen Changes
- Updated `generateWhileLoop()` to handle explicit loop variables
- Updated `generateForRangeLoop()` to handle new 4-child pattern
- Updated `generateForCountLoop()` to handle forever loops
- Added `generateFString()` to convert f-strings to sprintf calls

### Test Coverage
See `test/input/new_loop_syntax_test.ahoy` for working examples.
