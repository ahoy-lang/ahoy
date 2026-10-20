# Ahoy Memory Ownership (unified plan)

> Companion to `MEMORY_MODEL.md` (the user-facing model: one heap,
> scope-owned, escape-marks leak soundly). This document is the
> compiler-side implementation record: allocation classes, regions,
> promotion rules, and what changed when.

One rule governs everything: **every allocation has exactly one owner.**
Arena-owned values are never individually freed. Owned-heap values are
freed by autofree or explicit `free`. When the owner is unknown, the
compiler falls back to owned heap. There is deliberately no borrow
checker: the language stays high-level and the failure mode is always a
bounded leak, never a dangling reference.

This replaces the per-value ownership idea that ARC proved unworkable
for Ahoy (`ARC_IMPLEMENTATION.md`): structs are copied by value
(`cur: nodes[i]` … `nodes[id] = cur` throughout Bell), so a refcount
field on the value is incoherent. Ownership lives at the **region**,
not the value.

## Allocation classes

`AllocationClass` (`ahoy/source/codegen.go`) is the single decision
point. The autofree emitter and any arena router must both read it;
they must never disagree.

| Class | Contents | Lifetime | Freed by |
|---|---|---|---|
| `Value` | `int/float/bool/char`, POD structs, borrowed views | stack/registers | nothing (copied) |
| `Arena` | frame scratch, scene bulk (future routing) | region reset | region reset, never `free` |
| `Owned` | arrays, dicts, strings, closures that cross regions; globals; job args; FFI-retained | scope/function or longer | autofree / manual `free` |
| `Foreign` | C-owned (GL handles, retained callbacks) | external | explicit `free` only |

## Copy and alias rules

- **POD / trivially copyable**: copied (`b = a` copies).
- **Aggregates** (struct with strings/pointers): copying copies the
  **reference**, not the payload. `cur: nodes[i]` stays cheap; the
  container owns, the copy borrows, the borrower never frees.
- **Containers**: alias within a region; `copy`/`move` only at region
  boundaries (promotion below clones **references**, never deep-copies
  payloads — deep copy on every store would be O(n) everywhere).
- **Unknown escape** (`any`, generics, dynamic dispatch, FFI): owned
  heap, with no warning today (lint-clean constraint; see Diagnostics).

## Escape sites (promotion before routing)

A value escapes through: `return`; `arr.push`/`fill`, `dict[k] = v`;
`struct.field = v`; capture by an escaping closure; `jobs.submit` args
(the pool retains them — frame arena is invalid); globals; FFI calls
that retain pointers; hot-reload state. At each site the compiler
**promotes**: `strdup` for strings, reference takeover for containers,
`malloc+memcpy` for escaping closure envs. Promotion ships before any
allocation is routed to an arena — routing first would hand callers
dangling region pointers (the `callback_table` class of bug).

String-element ownership today:

- `split` results **own** their pieces (`heapArrayOwnsStrings`);
  literals borrow static strings and never free elements.
- **Element borrows must be promoted at store sites.** A read like
  `hp[0]` aliases a piece the array frees at scope exit, so every store
  of it duplicates (`ownedReadNeedsDup`): declarations (`x: arr[i]`),
  assignments (`x = arr[i]`, guarded to string-holding targets),
  push args (also into borrowed targets — the copy then leaks via
  shell-only freeing: sound, bounded), returns, struct-field stores,
  dict puts, and array-element stores. Pure reads (conditions,
  receivers, arithmetic) never duplicate. This fixed a live game crash
  (scene_load UAF → tcache double-free abort); see MEMORY_BASELINE.md.
- Pushing a borrowed static into an owned array **duplicates** it at
  the boundary (`pushNeedsStrdup`); ownership is monotonic and static
  emission stays sound across loop iterations.
- `fill` stores one value N times (one `strdup` cannot serve N slots),
  so fill into an owned array degrades to shell-only freeing
  (leak-safe; documented limitation).
- Returning an owned array transfers piece ownership to the caller by
  reference (`funcReturnsOwnedStrings`); unknown callees default to
  borrowed elements (leak-safe).
- Struct-field stores duplicate retainable sources (`fieldStoreNeedsDup`):
  bare identifiers (including parameters) and borrowed container reads
  whose type reads as string. Fresh values, literals and already-handled
  shapes stay raw. Same C type either way; contradictory-type stores
  keep their shape.
- Reads into primitives never escape-mark (`total = total +
  arr->length` retains nothing — the over-marking that leaked every
  loop-local container). Struct targets stay conservative (struct
  copies share references).

## Regions

- **Frame arena**: created once in `main`, reset at the top of
  `engine_frame` / `engine_frame_3d` (name-matched hooks, inert for
  non-engine programs). Nested tool/UI regions push/pop via
  `ahoy_region_push/pop`.
- **Scene arena**: created once in `main` (1MB block), reset at the top
  of `game_switch_scene`. Cycles are allowed **only** in arenas (scene
  graph, UI tree, nav) — the region dies whole, so cycles need no
  collector. Heap-owned cycles get no diagnostic yet (future work).
- **Status**: lifecycle + high-water reporting ride along today;
  allocation routing is explicitly future work. The arenas sit idle
  (owned heap serves everything) until temps are provenanced.

## Diagnostics

- `-show_leaks`: builds with ASan, defines `AHOY_ARENA_POISON`
  (0xAB fill on reset — use-after-reset becomes detectable) and prints
  `[ahoy-memory] frame/scene high-water` to stderr at exit.
- Leak suite: `../test/input/leak_*.ahoy` (functional pins) +
  `../tools/mem_check.sh` (valgrind, fails on any lost byte) +
  codegen-text tests (`memory_phase0_test.go`, `memory_phase3_test.go`).
- No new lint warnings were added: every `input/*.ahoy` must stay
  lint-clean (`TestLinting`), so diagnostics ride in `-show_leaks`
  and the test suite, not the linter.

## Baseline

See `MEMORY_BASELINE.md`: the frame-temp probe went from 1,600 leaked
blocks (38.6KB) to 2,201 allocs / 2,201 frees, 0 in use.

## Non-goals (explicit)

- Compiler-managed slab/SoA: that is engine design (Bell), not language
  codegen. Slabs live *inside* the scene arena; the compiler never
  rewrites struct layout (C interop, hot reload, debuggability).
- `move` / copy-on-write / per-element ownership metadata: performance
  work after correctness, per the incremental rollout.

## Retaining boundaries (engine side)

Call arguments are not escape sites in the compiler analysis (marking
every heap argument escaping would leak every call). So a function that
stores a parameter into a longer-lived struct must duplicate at its own
funnel. `node_make` owns node names via `dup`, covering all ~20
`node_*` constructors in one edit; callers keep no burden. The dups are
never freed (scene teardown frees components, not names) — bounded leak
per node, never a dangling name. Full teardown (freeing names in
`scene_free`) needs an all-names-owned invariant and is future work.

Compiler gap closed alongside: `dup` was missing from the string-method
dispatch and string-return inference lists (calls fell through to a
nonexistent `ahoy_array_dup` link error). Both lists now include it.

## Known limitations (sound leaks, documented)

- Static-declared string vars (`s: ""`) assigned owned reads skip
  tracking (no declaration entry): one copy leaks per assignment.
  Same pre-existing class as the `clean = clean + piece` pattern.
  A declaration-scope map was tried for this and reverted: scheduling
  from branch-conditional registrations breaks single-pass dominance
  (the registered scope may never have run while an unconditional
  eager-free fires — the editor scene-path crash). Only
  straight-line-dominated heap declarations schedule; the rest leak
  soundly.
- Copies pushed into borrowed (non-owning) arrays leak via shell-only
  freeing. Bounded per push; never dangles.
- Borrowed borrows that later escape (`q: owned[i]` then `return q`)
  rely on future borrow-provenance tracking; push-escape is covered,
  return/field-escape of pure borrows is not yet.
- Struct-element borrows have the same shape as string pieces and
  predate this work; same promotion treatment applies when touched.

## Scheduling invariants (learned the hard way)

- Scope-allocation lists hold each name **once** (`appendScopeAllocation`
  dedups). Early-return cleanups emit per entry without marking, so a
  duplicate entry is a dynamic double-free.
- Registration schedules only straight-line-dominated values. A
  branch-conditional registration paired with an unconditional
  eager-free frees values the branch never ran for.
- Never free from a pool with plain `free`: pooled chunk slots crash
  under it. All codegen frees of known struct pointers route to the
  per-struct Release fns; C code must do the same.
