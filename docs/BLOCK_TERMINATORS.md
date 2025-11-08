# Block Terminator Rules ($)

## Overview
The `$` symbol terminates multi-line blocks in Ahoy. Understanding when it's required vs optional is crucial for proper syntax.

## General Rule
**Multi-line constructs using `:` (colon) syntax REQUIRE `$` at the end.**
**One-line constructs with `then` or `do` keywords do NOT require `$`.**

---

## Detailed Rules by Construct

### ✅ Conditionals (if/anif/else)

#### REQUIRES `$` - Multi-line with colon
```ahoy
if x is 10:
    print|"multi-line\n"|
    print|"block\n"|
$

if score greater 90:
    print|"A\n"|
anif score greater 80:
    print|"B\n"|
else:
    print|"F\n"|
$
```

#### NO `$` NEEDED - One-line with `then`
```ahoy
if x is 10 then print|"one line\n"|
if x is 10 then print|"yes\n"| else print|"no\n"|
if x is 10 then print|"yes\n"| anif x is 20 then print|"maybe\n"| else print|"no\n"|
```

#### NO `$` NEEDED - Inline with semicolons
```ahoy
if x is 10: print|"yes"|;
anif x is 20: print|"maybe"|;
else: print|"no"|;
```

---

### ✅ Loops

#### REQUIRES `$` - Multi-line with colon
```ahoy
loop i to 5:
    print|f"Count {i}\n"|
    doSomething||
$

loop i:1 to 10:
    print|f"Iteration {i}\n"|
$

loop till condition:
    print|"looping\n"|
$
```

#### NO `$` NEEDED - One-line with `do`
```ahoy
loop i:0 till i < 5 do print|f"Value: {i}\n"|
loop i to 5 do print|f"Count {i}\n"|
```

---

### ✅ Structs

#### ALWAYS REQUIRES `$`
Structs are multi-line constructs and **always** require `$`:

```ahoy
struct particle:
    position: vector2
    velocity: vector2
    type smoke_particle:
        size: float
        alpha: float
$

? Even one-line structs need $
struct simple: x: int; y: int
$
```

---

### ✅ Enums

#### ALWAYS REQUIRES `$`
Enums are multi-line constructs and **always** require `$`:

```ahoy
enum status:
    active
    inactive
    pending
$

enum numbers:
    1 one,
    5 five,
    10 ten,
$
```

---

### ✅ Functions

#### REQUIRES `$` - Multi-line function body
```ahoy
myFunc :: |param:int| void:
    print|"Function body\n"|
    return
$
```

#### NO `$` NEEDED - Single expression functions (if supported)
```ahoy
? One-line function (syntax may vary)
square :: |x:int| int: return x * x
```

---

### ✅ Switch Statements

#### REQUIRES `$` - Multi-line with `on`
```ahoy
switch value on
    'A': print|"Case A\n"|
    'B': print|"Case B\n"|
    _: print|"Default\n"|
$
```

#### NO `$` NEEDED - One-line with `then...end`
```ahoy
switch day then 1:print|"Mon"| 2:print|"Tue"| 3:print|"Wed"| end
```

---

## Nested Blocks

When nesting blocks, each multi-line block needs its own `$`:

```ahoy
? Outer if with nested loop
if condition:
    print|"Starting\n"|
    loop i to 5:
        print|f"Nested {i}\n"|
    $  ? Close inner loop
    print|"Done\n"|
$  ? Close outer if

? Nested struct types
struct container:
    type inner:
        x: int
        y: int
$  ? One $ closes entire struct (all types are part of same block)
```

---

## Common Mistakes

### ❌ Missing `$` on multi-line colon blocks
```ahoy
? WRONG - missing $
if x is 10:
    print|"hello\n"|
? Next statement will cause parse error

? CORRECT
if x is 10:
    print|"hello\n"|
$
```

### ❌ Extra `$` on one-line statements
```ahoy
? WRONG - unnecessary $
if x is 10 then print|"hello\n"|
$  ? This $ is superfluous

? CORRECT
if x is 10 then print|"hello\n"|
```

### ❌ Missing `$` on struct/enum
```ahoy
? WRONG
struct point:
    x: int
    y: int
? Missing $ causes parse errors

? CORRECT
struct point:
    x: int
    y: int
$
```

---

## Quick Reference

| Construct | One-line (`then`/`do`) | Multi-line (`:`) | Notes |
|-----------|----------------------|------------------|-------|
| `if/anif/else` | No `$` | Requires `$` | Inline with `;` also no `$` |
| `loop` | No `$` with `do` | Requires `$` with `:` | - |
| `struct` | - | Always `$` | Even one-line needs `$` |
| `enum` | - | Always `$` | Even one-line needs `$` |
| `function` | Maybe no `$` | Requires `$` | Depends on syntax |
| `switch` | No `$` with `then...end` | Requires `$` with `on` | - |

---

## Multi-Block Closure with `$#N`

To close multiple blocks at once, use `$#N` where N is the number of blocks to close:

```ahoy
? Instead of:
if outer:
    loop i to 5:
        if inner:
            print|"nested\n"|
        $
    $
$

? You can write:
if outer:
    loop i to 5:
        if inner:
            print|"nested\n"|
$#3  ? Closes all 3 blocks at once
```

This is especially useful with deep nesting. The syntax `$#3` is equivalent to writing three `$` symbols.

**Validation:**
- `$#0` or `$#-1` → Error: Invalid count
- `$#5` when only 3 blocks open → Error: Cannot close more blocks than are open

---

## Detection of Superfluous `$`

The parser will warn about superfluous `$` symbols that don't close any block:
- After one-line `then` statements
- After one-line `do` statements  
- Multiple `$` in a row
- `$` at top level with no open block

This helps catch errors while maintaining clean syntax.

### Example Errors:

```ahoy
? ERROR: Superfluous $
if x is 10 then print|"hello\n"|
$  ? ❌ One-line doesn't need $

? ERROR: Superfluous $
loop i to 5:
    print|f"Count {i}\n"|
$
$  ? ❌ No block to close
```
