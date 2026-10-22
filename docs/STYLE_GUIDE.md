# Ahoy Style Guide

Short, opinionated rules for writing readable Ahoy. The compiler enforces the
hard ones (errors); the rest are conventions the checker warns about.

## Conditionals

### `guard` — preconditions only
`guard C else B $` is exactly `if not C then B $`. Use it to state a
condition that **must** hold, where `B` exits (`return`, `halt`, `next`).
The code after the guard then knows `C` held, without indenting the rest
of the function.

```ahoy
@ node_set_slot :: |nodes:array[Node], id:int, v:float| void:
    guard id >= 0 else return $
    guard v >= 8.0 else return $
    cur: nodes[id]
    cur.slot.slot_w = v
    nodes[id] = cur
$
```

Rules (enforced):
- `guard` takes **no** `then` and **no** `elif` (`guard C then`, `guard C elif ...` are errors).
- The `else` **must diverge** (`return`/`halt`/`next`); otherwise the compiler errors.
- A bare `guard C` with no `else` is an error.
- Guard conditions support the same comma lists: `guard x is not 1, 2, 3 else next $`.

Do **not** use `guard` as a dispatch table. A chain of guards that each
return a different value is a dispatch in disguise and reads backwards:

```ahoy
? BAD - dispatch disguised as guards
@ kind_icon_name |kind:int| string:
    guard kind is not NodeKind.sprite else return "icon_sprite" $
    guard kind is not NodeKind.label else return "icon_label" $
    ...
    return "icon_node"
$
```

The checker warns on this shape (3+ consecutive guards returning different
values via `is not`). Prefer:

### `switch` — dispatch tables
```ahoy
@ kind_icon_name |kind:int| string:
    return switch kind:
        on NodeKind.sprite, NodeKind.sprite3d: "icon_sprite"
        on NodeKind.label: "icon_label"
        on NodeKind.rect: "icon_rect"
        ...
        _: "icon_node"
    $
$
```

Group related cases with commas (`on A, B:`). A switch is an expression, so
it can be returned or assigned directly, and the compiler checks that every
case returns a compatible type.

### `if` / `elif` / `else` — value-producing conditionals
Use for a handful of branches that produce values, when the conditions are
not simple equality on one subject:

```ahoy
@ classify |n:int| string:
    if n < 0 then return "negative"
    elif n is 0 then return "zero"
    else return "positive"
    $
$
```

### `x is A, B, C` — membership tests
`is` (any-of) and `is not` (none-of) take comma lists. They return a bool,
so prefer them over a guard or if chain when you only need a yes/no answer:

```ahoy
@ is_text_file |name:string| bool:
    lname: name.lower||
    return lname is ".txt", ".md", ".sh", ".json", ".ahoy", ...
$
```

### `guard … else next $` — flatten nested skips
Inside a loop, `next` continues the enclosing loop, so a guard can replace
deeply nested `if not X then if not Y then …`:

```ahoy
loop i:0 to names.length do
    nm: names[i]
    if d is 1 then
        guard nm is not "output" else next $
        guard nm is not ".git" else next $
        guard nm is not ".scratch" else next $
        work.push|full|
        depths.push|dep - 1|
    $
$
```

## Inline conditionals
Short `if COND then STMT $` (under ~100 chars) should stay on one line.
The formatter does this automatically:

```ahoy
if x is 10 then print|"yes"| $
```

Multi-statement or long bodies get the multi-line form.

## Functions
- Name functions in `snake_case`; types in `PascalCase`.
- Keep the happy path un-indented with `guard` preconditions at the top.
- Prefer returning a value over mutating an out-parameter.

## Warnings vs errors
- **Errors** (compile fails): syntax, type mismatches, undeclared names,
  non-diverging guard `else`, `guard … then`/`elif`.
- **Warnings** (build succeeds): guard chains that look like dispatch tables,
  double-free aliasing, deprecated `pass if`, dedented statements, etc.
