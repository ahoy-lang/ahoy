# Break and Skip Implementation

## Syntax:
- `break` - Exit the innermost loop immediately
- `skip` - Skip to next iteration of current loop (like continue)

## Examples:

### Break Example:
```ahoy
loop:0 to 10 do
    if x is 5 do
        break
    ahoy|"x = %d", x|
```

### Skip Example:
```ahoy
loop:0 to 10 do
    if x mod 2 is 0 do
        skip
    ahoy|"Odd: %d", x|
```

## Implementation Plan:

1. Add TOKEN_BREAK and TOKEN_SKIP to tokenizer
2. Add NODE_BREAK and NODE_SKIP to parser
3. Parse break/skip statements
4. Generate C code: break -> break; skip -> continue;
