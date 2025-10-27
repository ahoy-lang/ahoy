# Inline Conditionals and anif Keyword

## Overview
The Ahoy language now supports inline conditionals and the `anif` keyword for more concise and readable code.

## Features

### 1. Inline If Statements
Put the condition and statement on the same line:
```ahoy
if x is 10 then ahoy|"x is 10\n"|
```

### 2. Inline If-Else
Chain if and else on a single line:
```ahoy
if y greater 10 then ahoy|"Big\n"| else ahoy|"Small\n"|
```

### 3. The anif Keyword
Use `anif` as a cleaner alternative to `elseif`:
```ahoy
if score greater 90 then ahoy|"A\n"| anif score greater 80 then ahoy|"B\n"| else ahoy|"C\n"|
```

Both `elseif` and `anif` work - use whichever you prefer!

### 4. Mixed Inline and Multi-line
Combine inline and multi-line formats in the same conditional chain:
```ahoy
if simple then action|| anif complex then
    multiple_lines||
    more_code||
else default||
```

## Examples

### Before (Multi-line only):
```ahoy
score: 85
if score greater_than 90 then ahoy|"Grade: A\n"|
anif score greater_than 80 then ahoy|"Grade: B\n"|
anif score greater_than 70 then ahoy|"Grade: C\n"|
else ahoy|"Grade: F\n"|
```

### After (Inline with anif):
```ahoy
score: 85
ifth score greater_than 90 then ahoy|"Grade: A\n"| anif score greater_than 80 then ahoy|"Grade: B\n"| anif score greater_than 70 then ahoy|"Grade: C\n"| else ahoy|"Grade: F\n"|
```

### Mixed Style:
```ahoy
size: 15
if size is 10 then ahoy|"Exactly 10\n"| anif size lesser_than 20 then
    ahoy|"Between 10 and 20\n"|
    ahoy|"Size is: %d\n", size|
else ahoy|"20 or more\n"|
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
- Fully backward compatible
- All existing code continues to work
- `elseif` and `anif` are interchangeable
- Multi-line format still fully supported
