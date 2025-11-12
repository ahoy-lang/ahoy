# Inline Conditionals and anif Keyword

## Overview
The Ahoy language supports inline conditionals and uses the `anif` keyword for else-if statements.

## Block Terminator Rules

**The `$` terminator is required for multi-line blocks using `:` (colon syntax):**
- Multi-line if/anif/else blocks with `:` require `$`
- One-line statements with `then` or `do` do NOT require `$`
- Inline statements with semicolons on the same line do NOT require `$`

## Features

### 1. Inline If Statements
Put the condition and statement on the same line:
```ahoy
if x is 10 then ahoy|"x is 10\n"|$
```

### 2. Inline If-Else
Chain if and else on a single line:
```ahoy
if y greater 10 then ahoy|"Big\n"| else ahoy|"Small\n"|$
```

### 3. The anif Keyword
Use `anif` for else-if statements (no more `elseif`):
```ahoy
if score greater 90 then ahoy|"A\n"| anif score greater 80 then ahoy|"B\n"| else ahoy|"C\n"|
```

**Note:** Only `anif` is supported for else-if statements. The keyword `elseif` is not available.

### 4. Multi-line with Colon Syntax (Requires `$`)
Multi-line blocks using `:` require a `$` terminator:
```ahoy
if x is 10:
    print|"x is 10\n"|
    print|"multi-line block\n"|
$

if score greater 90:
    print|"Grade A\n"|
anif score greater 80:
    print|"Grade B\n"|
else:
    print|"Grade F\n"|
$
```

## Examples

### Before (Multi-line only):
```ahoy
score: 85
if score greater_than 90: ahoy|"Grade: A\n"|;
anif score greater_than 80: ahoy|"Grade: B\n"|;
anif score greater_than 70: ahoy|"Grade: C\n"|;
else ahoy|"Grade: F\n"|;
$
```

### After (Inline with anif):
```ahoy
score: 85
if score greater_than 90 then ahoy|"Grade: A\n"| anif score greater_than 80 then ahoy|"Grade: B\n"| anif score greater_than 70 then ahoy|"Grade: C\n"| else ahoy|"Grade: F\n"|
```

### Mixed Style:
```ahoy
? One-line with 'then' - no $ needed
if size is 10 then ahoy|"Exactly 10\n"|

? Multi-line with : - $ required
if size less_than 20:
    ahoy|"Between 10 and 20\n"|
    ahoy|"Size is: %d\n", size|
$

? Inline with semicolons - no $ needed
if x is 10: print|"yes"|;
anif x is 20: print|"maybe"|;
else: print|"no"|;
```

## Formatter Support
The formatter automatically preserves inline conditionals:
```bash
./ahoy -f input/your_file.ahoy -format
```

One-line conditionals stay on one line, multi-line conditionals stay multi-line.

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
- Only `anif` is supported for else-if (not `elseif`)
- Multi-line format still fully supported
