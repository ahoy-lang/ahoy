# Print Statements

## Overview
Ahoy provides flexible print statements with automatic newline handling and Go-style format specifiers.

## Print Statement Types

### `print||` and `ahoy||` - Print with Newline
Both `print||` and `ahoy||` automatically insert a newline after printing (similar to `println` in other languages).

```ahoy
print|"Hello, World!"|
? Output: Hello, World!\n

ahoy|"Welcome to Ahoy!"|
? Output: Welcome to Ahoy!\n

ahoy|"Count: %d", 42|
? Output: Count: 42\n
```

### `printf||` and `ahoyf||` - Print without Newline
Use `printf||` and `ahoyf||` when you need explicit control over newlines. These require manual `\n` insertion.

```ahoy
printf|"Hello, "|
printf|"World!\n"|
? Output: Hello, World!\n

ahoyf|"Loading"|
ahoyf|"."|
ahoyf|"."|
ahoyf|".\n"|
? Output: Loading...\n
```

### `sprintf||` and `sahoyf||` - Format to String Variable
Use `sprintf||` and `sahoyf||` to format strings and store them in variables (like `fmt.Sprintf` in Go).

```ahoy
name: "Alice"
age: 25
message: sprintf|"Name: %s, Age: %d", name, age|
? message now contains: "Name: Alice, Age: 25"

formatted: sahoyf|"Value: %v", some_var|
? Store formatted output in variable
```

## Format Specifiers

Ahoy supports standard C format specifiers plus Go-style verbs:

### Standard Specifiers
- `%d` - Integer (decimal)
- `%f` - Float
- `%s` - String
- `%c` - Character
- `%.2f` - Float with precision (e.g., 2 decimal places)

### Go-Style Specifiers
- `%v` - Default format for any value (like Go's %v)
- `%t` - Type of the value (like Go's %T)

```ahoy
name: "Alice"
age: 25
score: 95.5
active: true

? Using %v (value) - prints the value in default format
print|"Name: %v", name|        ? Output: Name: Alice
print|"Age: %v", age|          ? Output: Age: 25
print|"Score: %v", score|      ? Output: Score: 95.5
print|"Active: %v", active|    ? Output: Active: true

? Using %t (type) - prints the type
print|"Type of name: %t", name|      ? Output: Type of name: string
print|"Type of age: %t", age|        ? Output: Type of age: int
print|"Type of score: %t", score|    ? Output: Type of score: float
print|"Type of active: %t", active|  ? Output: Type of active: bool
```

## Examples

### Multiple Values
```ahoy
x: 10
y: 20
print|"x: %d, y: %d", x, y|
? Output: x: 10, y: 20\n
```

### Using %v for Generic Values
```ahoy
values: [1, 2, 3, 4, 5]
config: {"debug": "true"}

print|"Values: %v", values|
? Output: Values: [1, 2, 3, 4, 5]\n

print|"Config: %v", config|
? Output: Config: {debug: true}\n
```

### Using %t for Type Information
```ahoy
x: 42
name: "Bob"

print|"x is type %t with value %v", x, x|
? Output: x is type int with value 42\n

print|"%v is a %t", name, name|
? Output: Bob is a string\n
```

### String Formatting with sprintf
```ahoy
? Format strings and store in variables
user: "Alice"
score: 95

message: sprintf|"User %s scored %d points", user, score|
print|"%v", message|
? Output: User Alice scored 95 points\n

? Build complex strings
status: sprintf|"[%t] %v", score, score|
? status = "[int] 95"
```

### Without Newline
```ahoy
printf|"Progress: "|
printf|"["|
printf|"====>"|
printf|"]"|
printf|" 100%%\n"|
? Output: Progress: [====>] 100%\n
```

### Building Output Line by Line
```ahoy
print|"Line 1"|
print|"Line 2"|
print|"Line 3"|
? Output:
? Line 1
? Line 2
? Line 3
```

## Summary

| Function | Newline | Returns | Use Case |
|----------|---------|---------|----------|
| `print\|\|` | Yes | void | Print and add newline |
| `ahoy\|\|` | Yes | void | Print and add newline |
| `printf\|\|` | No | void | Print without newline |
| `ahoyf\|\|` | No | void | Print without newline |
| `sprintf\|\|` | N/A | string | Format to variable |
| `sahoyf\|\|` | N/A | string | Format to variable |

## Format Specifier Reference

| Specifier | Type | Example |
|-----------|------|---------|
| `%d` | Integer | `42` |
| `%f` | Float | `3.14` |
| `%s` | String | `"hello"` |
| `%c` | Character | `'a'` |
| `%v` | Any (value) | `42`, `"text"`, `true`, `[1,2,3]` |
| `%t` | Type name | `int`, `string`, `float`, `bool` |
