# Enhanced Loop Syntax

## Overview
The Ahoy language now supports multiple loop syntaxes for different use cases, making iteration more intuitive and expressive.

## Loop Syntax Options

### 1. Range Loop: `loop:start to end`
Loop from a start value to an end value (exclusive).

```ahoy
? Loop from 0 to 5
loop:0 to 5 then
    ahoy|"Iteration\n"|

? Loop from 10 to 15
loop:10 to 15 then
    ahoy|"Count\n"|
```

### 2. While Loop: `loop:condition`
Loop while a condition is true (traditional while loop).

```ahoy
x: 0
loop:x lesser_than 10 then
    ahoy|"x is %d\n", x|
    x: x plus 1

? Or use boolean directly
flag: true
loop:flag then
    ? Do something
    flag: false
```

### 3. Count Loop: `loop:number`
Loop starting from a specific number (infinite, or use with break).

```ahoy
? Start counting from 5
loop:5 to 10 then
    ahoy|"Counting\n"|
```

### 4. Default Loop: `loop`
Default loop (starts at 0, equivalent to `loop:0`).

```ahoy
? Defaults to starting at 0
loop:0 to 5 then
    ahoy|"Default loop\n"|
```

### 5. Array Iteration: `loop element in array`
Iterate through array elements.

```ahoy
numbers: <10, 20, 30, 40, 50>

loop num in numbers then
    ahoy|"Number: %d\n", num|
```

### 6. Dictionary Iteration: `loop key,value in dict`
Iterate through dictionary key-value pairs.

```ahoy
config: {"name":"Ahoy", "version":"1.0", "active":"yes"}

loop key,val in config then
    ahoy|"Key: %s, Value: %s\n", key, val|
```

## Examples

### Example 1: Different Range Loops
```ahoy
? Print numbers 0 to 4
loop:0 to 5 then
    ahoy|"Number\n"|

? Print numbers 10 to 14
loop:10 to 15 then
    ahoy|"Number\n"|
```

### Example 2: While Loop with Condition
```ahoy
counter: 0
loop:counter lesser_than 10 then
    ahoy|"Counter: %d\n", counter|
    counter: counter plus 1
```

### Example 3: Array Iteration
```ahoy
scores: <95, 87, 92, 88, 91>

loop score in scores then
    if score greater_than 90 then
        ahoy|"Excellent: %d\n", score|
    else
        ahoy|"Good: %d\n", score|
```

### Example 4: Dictionary Iteration
```ahoy
user: {"name":"Alice", "role":"admin", "status":"active"}

loop k,v in user then
    ahoy|"%s: %s\n", k, v|
```

### Example 5: Nested Loops
```ahoy
? Nested range loops
loop:0 to 3 then
    loop:0 to 2 then
        ahoy|"Inner loop\n"|
    ahoy|"Outer loop\n"|
```

## Comparison with Old Syntax

### Before:
```ahoy
? Old while loop style
i: 0
loop i lesser_than 10 then
    ahoy|"Value: %d\n", i|
    i: i plus 1
```

### After (Multiple Options):
```ahoy
? Option 1: Range loop
loop:0 to 10 then
    ahoy|"Value\n"|

? Option 2: While loop (still supported)
i: 0
loop:i lesser_than 10 then
    ahoy|"Value: %d\n", i|
    i: i plus 1

? Option 3: Array iteration (if using array)
values: <0, 1, 2, 3, 4, 5, 6, 7, 8, 9>
loop val in values then
    ahoy|"Value: %d\n", val|
```

## Notes

- **Range loops** use `loop:start to end` where end is exclusive
- **While loops** use `loop:condition` where condition is any boolean expression
- **Array iteration** automatically extracts elements (no indexing needed)
- **Dictionary iteration** provides both key and value
- Old loop syntax (`loop condition then`) still works for backward compatibility
- Loop variables in array/dict iteration are automatically declared

## Generated C Code

The new loop syntax generates efficient C code:

- Range loops → `for (int i = start; i < end; i++)`
- While loops → `while (condition)`
- Array iteration → `for` loop with array size check
- Dict iteration → nested loops through hash map buckets

## Compatibility

- ✅ Fully backward compatible
- ✅ All existing loop code continues to work
- ✅ New syntax can be mixed with old syntax
- ✅ No breaking changes
