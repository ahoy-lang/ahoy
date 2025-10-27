# String Formatting Implementation Plan

## Problem:
User wants %v and %t format specifiers without runtime overhead.

## Solution:
Since C is statically typed, we can determine types at compile time!

## Approach:

1. When generating print statements, scan the format string for %v and %t
2. For each %v or %t, look at the corresponding argument
3. Determine the type of that argument from the AST
4. Replace %v with the appropriate C format specifier (%d, %f, %s, etc.)
5. Replace %t with a string literal of the type name

## Examples:

```ahoy
x: 42
ahoy|"Value: %v, Type: %t", x, x|
```

Becomes:
```c
printf("Value: %d, Type: int", x);
```

```ahoy
name: "Alice"
age: 30
ahoy|"%v is %v years old (%t, %t)", name, age, name, age|
```

Becomes:
```c
printf("%s is %d years old (string, int)", name, age);
```

## Implementation:

1. Update generatePrintStatement() in codegen.go
2. Add a function to process format strings
3. Match format specifiers to argument types
4. Replace %v and %t accordingly

No runtime functions needed!
