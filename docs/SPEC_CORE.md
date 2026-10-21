# Ahoy core spec (scoped): memory, arrays, lifetimes

*2026-10-22. The smallest normative slice that lets us reason about
Bell crashes without reading codegen. Companion to
`docs/MEMORY_MODEL.md` (user-facing) and `docs/MEMORY_OWNERSHIP.md`
(compiler-side). Nothing here changes behavior; it writes down what
the compiler already does, so future features (guard, buffer[T],
typed queues, serialization) have something to be checked against.*

## 1. What owns what

1.1 Every heap allocation has exactly one owner: the scope that
declared it. Scope exit frees in reverse declaration order.
1.2 Three escape sites keep storage alive, and all three leak soundly
rather than dangle: `return`, store-into-longer-lived-container
(array push, dict put, struct-field store), capture by an escaping
closure, plus names mentioned by a pending `defer`.
1.3 Call arguments are **not** escape sites. A function that stores a
parameter into longer-lived state must duplicate at its own funnel
(`node_make` dups node names; see MEMORY_OWNERSHIP.md "Retaining
boundaries").
1.4 Manual `free|x|` on a scope-owned name the compiler also frees is
a double-free; on an escaping name it is a use-after-free. Prefer to
never free by hand.

## 2. Arrays

2.1 `array` is a boxed, tagged, realloc-growing vector:
`{ intptr_t* data, AhoyValueType* types, length, capacity,
is_typed, element_type }`. Pushing a struct heap-allocates the
element (`source/codegen.go` "pushing a struct value").
2.2 Growth may move the block. A pointer or reference into an array
does **not** survive a push. Ids, not pointers, are stable.
2.3 `buffer[f32]` is the exception: one contiguous `float32` block,
no tags, no per-element allocation. `grow|buf, n|` returns a **new**
buffer with the first elements copied. Seeded here for the future
`buffer[T]` generalization, which keeps the layout and fixes the
growth story (in-place grow, stable reads).
2.4 Every index is runtime bounds-checked unless `-fast`. There is no
elimination analysis yet (cf. Goose `checkfree fn`, item 7).

## 3. Lifetimes and loops

3.1 A container created inside a loop body is freed at the end of
each iteration. A closure capturing it past the iteration dangles;
hoist the container to the enclosing scope.
3.2 An `array[SomeStruct]` whose struct has string fields must not be
rebuilt by pop/push cycles: popping frees strings that copies already
taken still alias. Prefer plain string arrays + small numeric structs.
3.3 Returning a temporary is safe only through a local. `return
xs[0].strip||` used to free-then-read; the compiler now stashes first
(`test/input/return_temp.ahoy`), but naming the local is still the
clearest form.

## 4. Guard (item 3, normative)

4.1 `guard C else B $` is exactly `if not C then B $`. No new scope,
no new codegen, no checker rule: the parser desugars to
`NODE_IF_STATEMENT` with a `not` condition.
4.2 Intended use is early exit: `B` ends in `return`, `halt`, or
`next`. The code after the guard then knows `C` held, flat instead of
nested. A guard whose `B` falls through is legal but pointless; prefer
`if`.
4.3 One way: `else` opens the block directly. `else then` and `else:`
are errors everywhere (`if`, `when`, `guard`) — same rule as the
removed `elseif`/`anif` synonyms.
4.4 `null` is the nil reference: the zero value of every pointer
field, assignable to pointer targets, comparable with `is null` /
`is not null` (pointer comparison, never `strcmp`). `guard p else
return $` on a pointer therefore reads the check directly. Flow
narrowing (knowing `p` is non-null after the guard) is still future
work: the checker tracks no nullness, so the rule stays syntactic.

## 5. What is explicitly not promised (yet)

- No stable references into growing arrays (item 5).
- Serialization covers flat `buffer[f32]` values (item 4, shipped):
  `to_bytes` / `from_bytes` / `from_bytes_ok` with exact framing
  checks; hostile bytes fail closed to empty/false. `array` and
  `string` payloads need ownership-aware framing and are next.
- Typed job values (item 6, mostly shipped): tasks take one value
  (`submit`) or up to four (`p.run`), and `float`/`double`/`f32`
  arguments and results travel as bit patterns through marshal
  thunks; arity mismatches are compile errors. What remains is
  Goose-style typed queues (one queue per value type, flat values
  crossing as `memcpy`) rather than the untyped `intptr_t` slot.
- No index types / checkfree (item 7) and no long-distance return
  (item 8): early `return`/`halt`/`next` plus `guard` cover the
  structured uses; the rest is design-only until the checker is
  stronger.

## 6. How to check it

- `go test ./...` in `pkg/`, `source/`, `test/` (lint + end-to-end).
- `bash tools/mem_check.sh` (valgrind, fails on any lost byte).
- `bash tools/bell_mem_check.sh` (ASan balls run, fails on any
  memory error; exit leaks reported, not gated).
- `./tools/commit.sh "msg" <explicit paths>` — never `git add -A`;
  this tree is shared with concurrent sessions.
