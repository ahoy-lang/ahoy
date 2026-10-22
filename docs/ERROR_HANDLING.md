# Error handling in Ahoy: the `(value, err)` convention

*2026-10-22. Ahoy has no exceptions, no `Result`/`Option` type, and no
`try`/`catch` — and it is not getting them. This document is the whole
error-handling story: a Go-shaped convention built from features the
language already has (multi-return, `null`, `if`, guard clauses).*

## The rule

A fallible function returns its value **and** an error string, with **`null`
meaning ok**:

```ahoy
? ok content + null, or "" + the reason. Success MUST return null.
@ load_text_try |path:string| string, string:
    if not ahoy_path_exists|path| then
        return "", "missing file"
    $
    body: ahoy_read_text|path|
    if body is "" then
        return "", "unreadable file"
    $
    return body, null
$
```

Callers unpack with `:=` and branch on the error. Three shapes, all
equivalent — pick the one that reads best at the site:

```ahoy
? 1. error-first (Go shape): handle the failure, keep the happy path flat
body: string = ""
err: string = ""
body, err := load_text_try|"level.scene"|
if err then
    log|err|
    return
$
use|body|

? 2. success-continuation: proceed only when there is no error
body2: string = ""
err2: string = ""
body2, err2 := load_text_try|"level.scene"|
if not err2 then
    use|body2|
$

? 3. guard clause: early exit, flat instead of nested (see CONDITIONALS.md)
text: string = ""
probl: string = ""
text, probl := load_text_try|"level.scene"|
guard probl is null else return $
use|text|
```

`read_json|path|` and `write_json|path, data|` already follow this
convention (value + error), so this is the house style, not a new idea.

## Why `null` and not `""`

An empty string is **truthy** in Ahoy: conditions test for null, and `""`
is a non-null pointer. So the error slot has exactly one ok-value:

| success value | `if err` | `if not err` | `err is null` |
|---|---|---|---|
| `null` | false (skip handler) | true (proceed) | true |
| `""` | **true (wrong branch!)** | **false (wrong branch!)** | false |

Returning `""` for ok reads as *error present* everywhere. That is the
safe direction (fail closed), but it is still wrong: **success must
return `null`**. The checker rejects a multi-character literal flowing
into a `char` slot; it does not (and cannot cheaply) prove which string
means ok — this rule is a convention, enforced by reading, not by the
compiler.

## What not to do

- **Do not nest the call.** `use|load_text_try|p||` is rejected like every
  other nested call: name the results first. This is a feature — every
  fallible step already has a name, which is exactly where the check goes.
- **Do not use `""` as ok**, even though older engine code checks
  `if body is ""`. The explicit `is ""` comparison is correct; bare
  truthiness on it is not. New code uses `null`.
- **Do not invent a `Result` struct** with `ok/value/err` fields. Without
  type parameters it is either `any`-typed (worse than a second return)
  or duplicated per value type (worse than the convention). The second
  return value *is* the Result.
- **An empty file is an error** in the `*_try` helpers above: an existing
  but empty scene/text file returns `"unreadable file"`, matching the
  old `if body is not "" pass else ...` behavior (zero nodes loaded either way).

## Checklist for a new fallible function

1. Return `(T, string)`: value first, error second.
2. Every `return` on the failure paths yields `"reason"`;
   every `return` on success yields `null` as the error.
3. Name it `*_try` so call sites read as fallible.
4. At the call site, unpack with `:=` and check before use —
   `if err then`, `if not err then`, or `guard err is null else`.
