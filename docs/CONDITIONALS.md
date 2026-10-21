# Inline Conditionals and elif Keyword

## Overview
The Ahoy language supports inline conditionals and uses the `elif` keyword for else-if statements.

## Block Terminator Rules

**The `$` terminator is required for multi-line blocks using `:` (colon syntax):**
- Multi-line if/elif/else blocks with `:` require `$`
- One-line statements with `then` or `do` do NOT require `$`
- Inline statements with semicolons on the same line do NOT require `$`

## Match Lists (comma sugar)

A comma after `is` tests one subject against several values. This is often the
compact alternative to writing the same subject again with `or`/`and`:

```ahoy
? any-of: x is A, B, C  ->  x is A or x is B or x is C
if kind is NodeKind.rect, NodeKind.shape2d, NodeKind.button2d then
    print|"box"|
$

? none-of: x is not A, B  ->  x is not A and x is not B
if cur.kind is not NodeKind.dead, NodeKind.hidden then
    print|"visible"|
$
```

The subject is evaluated once, even when it is a call (`pick|| is 4, 5`), and
strings compare by value. For whole alternatives prefer `switch` with
`on A, B:` cases. Inside array/object/dict literals commas separate elements,
so `[x is A, B]` stays two elements - write `x is A or x is B` there.

## Features

### 1. Inline If Statements
Put the condition and statement on the same line:
```ahoy
if x is 10 then ahoy|"x is 10\n"|$
```

### 2. Inline If-Else
Chain if and else on a single line:
```ahoy
if y > 10 then ahoy|"Big\n"| else ahoy|"Small\n"|$
```

### 3. The elif Keyword
Use `elif` for else-if statements (no more `elif`):
```ahoy
if score > 90 then ahoy|"A\n"| elif score > 80 then ahoy|"B\n"| else ahoy|"C\n"|
```

**Note:** Only `elif` is supported for else-if statements. The keyword `elif` is not available.

### 4. Multi-line with Colon Syntax (Requires `$`)
Multi-line blocks using `:` require a `$` terminator:
```ahoy
if x is 10:
    print|"x is 10\n"|
    print|"multi-line block\n"|
$

if score > 90:
    print|"Grade A\n"|
elif score > 80:
    print|"Grade B\n"|
else:
    print|"Grade F\n"|
$
```

## Examples

### Before (Multi-line only):
```ahoy
score: 85
if score > 90: ahoy|"Grade: A\n"|;
elif score > 80: ahoy|"Grade: B\n"|;
elif score > 70: ahoy|"Grade: C\n"|;
else ahoy|"Grade: F\n"|;
$
```

### After (Inline with elif):
```ahoy
score: 85
if score > 90 then ahoy|"Grade: A\n"| elif score > 80 then ahoy|"Grade: B\n"| elif score > 70 then ahoy|"Grade: C\n"| else ahoy|"Grade: F\n"|
```

### Mixed Style:
```ahoy
? One-line with 'then' - no $ needed
if size is 10 then ahoy|"Exactly 10\n"|

? Multi-line with : - $ required
if size < 20:
    ahoy|"Between 10 and 20\n"|
    ahoy|"Size is: %d\n", size|
$

? Inline with semicolons - no $ needed
if x is 10: print|"yes"|;
elif x is 20: print|"maybe"|;
else: print|"no"|;
```

## Formatter Support
The formatter automatically preserves inline conditionals:
```bash
./ahoy -f input/your_file.ahoy -format
```

One-line conditionals stay on one line, multi-line conditionals stay multi-line.

## Guard (early exit, flat instead of nested)

`guard C else then B $` is exactly `if not C then B $` — the parser
desugars it, so the checker and codegen never see a new construct.
Use it when the `B` block exits (`return`, `halt`, `next`): the code
after the guard then knows `C` held, without indenting the rest of
the function one level deeper:

```ahoy
@ node_set_slot_w :: |nodes:array[Node], id:int, v:float| void:
    guard id >= 0 else then return $
    guard v >= 8.0 else then v = 8.0 $
    cur: nodes[id]
    cur.slot.slot_w = v
    nodes[id] = cur
$
```

`guard` is contextual, not reserved: a variable named `guard` (Bell
uses it as a loop-counter name) still declares and assigns normally.
Only statement-start `guard … else` takes the new path. A guard whose
block falls through is legal but pointless — prefer `if` there.
See `docs/SPEC_CORE.md` §4 for the normative rule.

## When to Use

**Use inline format when:**
- Condition and action are both simple
- You want concise, readable code
- The entire statement fits comfortably on one line

**Use multi-line format when:**
- Action requires multiple statements
- Condition is complex
- Better readability for longer code blocks

## Compatibility
- Fully backward compatible with most code
- Only `elif` is supported for else-if (not `elif`)
- Multi-line format still fully supported
