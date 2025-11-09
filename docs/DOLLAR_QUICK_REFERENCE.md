# $ Block Terminator - Quick Reference

## When $ is Required

✅ **ALWAYS need $:**
- Structs
- Enums
- Multi-line functions
- Multi-line if/anif/else with `:`
- Multi-line loops with `:`
- Multi-line switch with `on`

❌ **NEVER need $:**
- One-line if with `then`
- One-line loop with `do`
- Inline statements with `;`

## Quick Examples

### Conditionals
```ahoy
? One-line - no $
if x is 10 then print|"hello\n"|

? Multi-line - needs $
if x is 10:
    print|"hello\n"|
$
```

### Loops
```ahoy
? One-line - no $
loop i to 5 do print|f"i={i}\n"|

? Multi-line - needs $
loop i to 5:
    print|f"i={i}\n"|
$
```

### Multiple Block Closure
```ahoy
? Traditional way:
if outer:
    loop i to 5:
        if inner:
            action||
        $
    $
$

? Using $#N:
if outer:
    loop i to 5:
        if inner:
            action||
$#3  ? Close all 3 blocks
```

## Common Errors

### ❌ Superfluous $
```ahoy
if x then print|"test\n"|
$  ? ERROR: One-line doesn't need $
```

### ❌ Missing $
```ahoy
if x:
    print|"test\n"|
? ERROR: Missing $ to close if block
```

### ❌ Invalid $#N
```ahoy
if x:
    print|"test\n"|
$#0  ? ERROR: Must be positive integer
```

### ❌ Closing too many
```ahoy
if x:
    print|"test\n"|
$#5  ? ERROR: Only 1 block open
```

## Lint Mode

Check for $ errors without compiling:
```bash
./ahoy-bin -f myfile.ahoy -lint
```

## Syntax Highlighting

Keywords now highlighted:
- `do`, `to`, `till`, `in` - Loop keywords
- `on` - Switch keyword
- `halt`, `next` - Control flow

---

For detailed information, see `BLOCK_TERMINATORS.md`
