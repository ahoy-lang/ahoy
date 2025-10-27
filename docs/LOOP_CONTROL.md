# Loop Control Statements

Ahoy provides two keywords for controlling loop execution flow: `break` and `skip`.

## Break Statement

The `break` keyword immediately exits the innermost loop.

### Syntax:
```ahoy
break
```

### Examples:

**Exit loop when condition is met:**
```ahoy
x: 0
loop:x less_than 10 do
    if x is 5 then
        break
    print|"x = %d", x|
    x: x plus 1
? Output: x = 0, 1, 2, 3, 4
```

**Search in array:**
```ahoy
numbers: [10, 20, 30, 40, 50]
target: 30
found: false

loop num in numbers do
    if num is target then
        found: true
        print|"Found %d!", target|
        break
```

**Nested loops - breaks only inner:**
```ahoy
i: 0
loop:i less_than 3 do
    j: 0
    loop:j less_than 5 do
        if j is 3 then
            break  ? Exits inner loop only
        print|"[%d,%d]", i, j|
        j: j plus 1
    i: i plus 1
? Outer loop continues
```

## Skip Statement

The `skip` keyword skips the rest of the current iteration and continues with the next iteration.

### Syntax:
```ahoy
skip
```

### Examples:

**Skip even numbers:**
```ahoy
i: 0
loop:i less_than 10 do
    i: i plus 1
    if i mod 2 is 0 then
        skip
    print|"Odd: %d", i|
? Output: Odd: 1, 3, 5, 7, 9
```

**Filter invalid values:**
```ahoy
values: [-5, 10, 0, 15, -3, 20]

loop val in values do
    if val less_than 0 then
        skip  ? Skip negative values
    print|"Valid: %d", val|
? Output: Valid: 10, 0, 15, 20
```

**Skip specific conditions:**
```ahoy
loop:0 to 20 do
    ? Skip multiples of 3
    if ? mod 3 is 0 then
        skip
    
    ? Skip multiples of 5
    if ? mod 5 is 0 then
        skip
    
    print|"Number: %d", ?|
? Prints numbers that aren't multiples of 3 or 5
```

## Combined Usage

You can use both `break` and `skip` together:

```ahoy
count: 0
loop:count less_than 20 do
    count: count plus 1
    
    ? Skip multiples of 3
    if count mod 3 is 0 then
        skip
    
    ? Stop at 15
    if count is 15 then
        print|"Stopping at 15"|
        break
    
    print|"Count: %d", count|
? Output: 1, 2, 4, 5, 7, 8, 10, 11, 13, 14, Stopping at 15
```

## Loop Types

Both `break` and `skip` work with all loop types:

### While-style loops:
```ahoy
x: 0
loop:x less_than 10 do
    x: x plus 1
    if x mod 2 is 0 then
        skip
    print|"%d", x|
```

### Range loops:
```ahoy
loop:0 to 100 do
    if ? is 50 then
        break
    print|"%d", ?|
```

### Array iteration:
```ahoy
items: [1, 2, 3, 4, 5]
loop item in items do
    if item is 3 then
        skip
    print|"%d", item|
```

### Dictionary iteration:
```ahoy
data: {"a": "1", "b": "2", "c": "3"}
loop key, val in data do
    if key is "b" then
        skip
    print|"%s = %s", key, val|
```

## Important Notes

1. **Scope:** `break` only exits the innermost loop it's contained in
2. **Skip vs Continue:** `skip` is equivalent to `continue` in other languages
3. **Must be in loop:** Both keywords can only be used inside loop bodies
4. **Immediate effect:** Both statements execute immediately when encountered
5. **Works everywhere:** Compatible with all loop types and nesting levels

## Common Patterns

### Early exit on condition:
```ahoy
loop item in large_array do
    if item_matches_criteria(item) then
        process(item)
        break  ? Found what we need, exit
```

### Filter loop iterations:
```ahoy
loop value in dataset do
    if not_valid(value) then
        skip  ? Skip invalid data
    process(value)
```

### Search with fallback:
```ahoy
found: false
loop item in collection do
    if item is search_target then
        found: true
        break

if not found then
    print|"Item not found"|
```
