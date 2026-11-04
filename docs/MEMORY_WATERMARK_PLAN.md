# Ahoy Watermark Memory Plan

**Status:** proposal — no code changes yet
**Date:** 2026-11-04
**Audience:** the model/engineer implementing the next memory phase
**Companions:** `MEMORY_MODEL.md` (user-facing model), `MEMORY_OWNERSHIP.md` (implementation record), `MEMORY_BASELINE.md` (measured results)

---

## 0. Executive summary

Ahoy should adopt Goose's **allocator discipline**, not Goose's type system:

- bump allocation from region arenas,
- nested LIFO watermark save/restore,
- O(1)-ish rewind instead of per-value frees for non-escaping values.

Ahoy should keep:

- `Owned` malloc/autofree as the escape hatch and the default when class is unknown,
- C-shaped structs and the C ABI,
- hot reload, jobs, closures, and existing syntax.

The watermark itself is the easy part. The hard part is **per-binding allocation class plus promotion at every escape boundary**. A watermark is only safe if no arena pointer can be stored anywhere that outlives the mark. If an implementation adds `ahoy_arena_mark/rewind` and starts routing allocations without the class/promotion work, it will trade leaks for use-after-free.

Recommended first milestone:

1. watermark runtime,
2. emit marks with routing **off**,
3. route only f-string/concat temporaries.

Do not start with arrays, dicts, scene graphs, or jobs.

---

## 1. Verdict on Goose

**Full Goose adoption: no.**

Goose's safety comes from its type system, size classes, placement rules, and whole-program lifetime proof. It is not primarily a smarter `malloc`. Adopting it would require changing:

- struct layout and array representation,
- escape rules for closures/function pointers,
- recursion with growable data,
- C interop and hot reload,
- the whole-program compilation model.

That is a language rewrite, not a memory patch.

**Adopt only:**

- region/arena bump allocation,
- watermark save/restore,
- retained blocks for reuse,
- flat transfer/copy at thread boundaries,
- preallocation/pooling where the engine already knows the shape.

**Do not adopt:**

- size classes in user syntax,
- placement rules (`resizable only as tail field`),
- bans on escaping closures / function pointers / growable recursion,
- whole-program monomorphization,
- promised stable references while `AhoyArray` still uses `realloc`.

---

## 2. Review of the prior reviews

### 2.1 What Muse Spark 1.3 got right

- Full Goose is not a patch; its guarantees come from the type system and whole-program compilation.
- Keep `Owned` + autofree for returns, globals, jobs, retained FFI, hot-reload state.
- Rewind, not reset, for per-scope lifetimes.
- `realloc` moves, so do not promise Goose-style stable references.
- Split hot (frame/loop) from cold (boot/scene-load) behavior.
- "Never dangles" is the goal, not the current truth; the past UAFs prove it.
- Strings and the f-string static buffer are the first target.
- Thread-local arena state is mandatory once workers allocate.
- Do not sell arena routing as a steady-frame speedup; Bell's frames were never alloc-bound.
- Do not chase zero process-exit leaks; chase zero per-frame growth and zero ASan errors.

### 2.2 Scorecard

| Claim | Verdict | Comment |
|---|---|---|
| Full Goose != Ahoy patch | Right | Keep the allocator discipline only. |
| Keep `Owned` / autofree | Right | Non-negotiable for returns, globals, jobs, FFI, hot reload. |
| Rewind, not reset | Right | Reset invalidates live caller data and marks. |
| `realloc` moves | Right | Stable element references need a different array representation. |
| Split hot vs cold | Right | Per-frame growth must be zero; boot leaks can be bounded. |
| Past UAFs exist | Right | The baseline records three real UAF classes. |
| "2 arenas != N stacks" | Half right | Nested lifetimes can share one arena with marks; only disjoint lifetimes need separate arenas. |
| Arena array header on native stack | **Wrong** | Breaks `AhoyArray*` ABI, returns, structs, C interop, hot reload. |
| "Grow within cap = bump, no move" | **Wrong** | Bump allocation has no per-object capacity; exceeding it still moves or fails. |
| Reuse `exitScope` to emit rewinds | **Incomplete** | Marks must be emitted at scope entry; four cleanup paths exist; class tracking is required. |
| Scene arena + cycles | **Unsafe as stated** | Reset does not run destructors; globals/hot reload/editor can point into scene memory. |
| Thread-local arena | Right, underspecified | Header ABI, workers, job results, and queue transfer all need rules. |
| F-string static buffer is bad | Right diagnosis, incomplete fix | It is also a truncation, `%`-injection, thread-race, and stored-value-mutation bug. |
| `ahoy_free` as a safety net | **Unreliable** | Pointer provenance fails after rewind/reuse. Use only as a debug assert. |
| O(n) -> O(1) teardown | **Only all-arena graphs** | Mixed `Owned` subobjects still need destructors. |
| Don't chase exit leaks | Right | Target zero per-frame growth and zero UAFs. |

### 2.3 The five important disagreements

#### 1. Stack-header arena arrays are an ABI/language change

Ahoy's array is a heap object:

```c
typedef struct {
    intptr_t* data;
    AhoyValueType* types;
    int length;
    int capacity;
    int is_typed;
    AhoyValueType element_type;
} AhoyArray;
```

It is returned, stored in structs, passed to C, and `arr.push(...)` returns `AhoyArray*`. A stack header cannot escape its function. "Header on native stack" only works for a new limited-array type, not existing `array[T]`.

**Decision:** keep the existing layout and `realloc` for `Owned` arrays. Do not route containers until phase 5, and then only with explicit element-class metadata.

#### 2. "Grow within cap = bump, no move" is not a mechanism

Bump allocation has no per-object capacity. Reserving capacity up front is just preallocation. If you exceed it, you must move or fail. "Promote to `Owned`" still moves `data` and `types`, so element addresses still change. "Abort with a message" is not acceptable in a game runtime.

There is no way to promise stable element references without changing the array representation. Treat that as a separate project.

#### 3. `exitScope` cannot "emit rewinds for Arena names instead of free"

The actual shape is:

- `enterScope()` is compile-time only and emits nothing.
- `exitScope()` returns a cleanup string appended at the block end.
- `generateEarlyExitCleanupTo()` handles `halt`/`next`.
- `generateReturnStatement()` has its own cleanup loop.
- `addAutomaticDeferFrees()` runs after the body and appends scope-0 frees as deferred statements.

A rewind is **per scope**, not per variable. You need:

- a mark declaration at scope entry,
- a class map per variable,
- a mark-depth map for early exits,
- one rewind after the `Owned` frees and deferred statements.

#### 4. Single-pass codegen means allocation class must be known before emission

`escapingVars` is populated while the body is generated (`markReturnedValue` at the return, `markEscapingVariables` at stores). A declaration earlier in the function cannot consult a later escape.

So "route all non-escaping locals" is not implementable in the current single pass without one of:

- a pre-pass over the function AST,
- routing only statement-local temporaries,
- or promoting at every escape boundary and accepting class transitions.

Phase 3 uses the statement-local approach. Phase 4 adds named locals with a pre-pass or boundary promotion.

#### 5. Frame reset and marks conflict

`ahoy_arena_reset(__ahoy_frame_arena)` at the top of `engine_frame` is a frame boundary, not a scope boundary. It invalidates any live frame data held by a caller — editor UI, nested frame, tools — and any marks taken before it.

Replace the frame reset with a function-entry mark plus per-scope marks. Only reset at a true top-level boundary with a depth guard, and after phase 1, do not need it at all.

### 2.4 Additional gaps both reviews missed

- **Scene reset ordering.** `game_switch_scene` currently resets the scene arena at function entry, before old-scene teardown/fade/save can run. Once scene objects are arena-routed, that is a UAF. Use double-buffered scene arenas or an explicit teardown phase.
- **Scene teardown.** Resetting an arena does not free `Owned` strings/arrays inside scene objects. `scene_free` is application-level, not compiler magic.
- **Job result lifetime.** Worker arenas are reused; `ahoy_jobs_result` returning a pointer into one dangles. Force args/results to `Owned` or copy them out.
- **F-string semantics.** The static buffer is not only a lifetime bug: stored pointers silently mutate on the next execution of the same call site, and `%` in literal text is interpreted by `sprintf`.
- **`ahoy_arena_alloc(NULL)` fallback.** Once routing is on, silently returning `malloc` turns an Arena value into an Owned allocation. Rewind does nothing (leak) or cleanup frees it (class mismatch). Use `AHOY_ARENA_STRICT`.
- **Runtime costs.** `highWater` currently walks every block per allocation; `realloc` abandons old bytes; `calloc` can overflow; alignment is fixed at 8; reset has no generation counter.
- **Container element class.** An Arena array holding `Owned` strings leaks them on rewind; an `Owned` array holding Arena strings calls `free()` on arena pointers.
- **Return contract.** Ahoy currently mixes static, borrowed, and fresh-owned string returns. A caller cannot know whether it owns the result. Define one contract before routing returns.
- **Owned inline temporaries.** Arena routing does not own the result of an `Owned` call used inline (`print|make_label()|`). That still needs a hidden temp + free or a caller-region return.
- **Single-pass class timing.** The class decision must be available before the allocation is emitted.
- **Hot reload runtime duplication.** Keep the arena runtime in the host; reloaded modules must not define a second set of globals.

---

## 3. Design

### 3.1 Regions

| Region | Storage | Lifetime | Reclaim | Use |
|---|---|---|---|---|
| Static | rodata/static | process | never | literals, consts |
| Owned | malloc | explicit | `free`/autofree | returns, globals, jobs, FFI, containers, closures |
| Frame | bump arena | function/scope marks | rewind | non-escaping temps |
| Scene | bump arena | scene switch | teardown + reset | scene graph (later) |
| Foreign | C-owned | external | explicit | GL handles, retained callbacks |
| Worker | per-thread frame arena | job/thread | rewind | worker temps |

### 3.2 Invariants

1. Every allocation has exactly one owner.
2. An arena pointer may not outlive its mark.
3. No `free` for `Arena`, `Static`, or `Value`.
4. No arena rewind for `Owned`.
5. Marks are LIFO; a mark is only used in the arena generation it was taken in.
6. Containers have a single element class; mixed requires promotion.
7. `Owned` is the default when class is unknown.
8. A pointer from a shorter region may not be stored in a longer region without promotion.

### 3.3 Why watermark, not reset

```text
frame arena:
[fn mark] ... [loop iter 1 mark] ... rewind ... [iter 2 mark] ... rewind ... [fn rewind]
```

Reset per frame: high-water = sum of all iterations.
Rewind per scope: high-water = max live iteration.

That is the entire argument. Everything else is plumbing.

### 3.4 Allocation classes

`AllocationClass` already exists in `source/codegen.go` (around line 5983) but is unused. Make it real:

- `Value` — primitives, copied.
- `Static` — literals/consts, never freed.
- `Arena` — fresh, non-escaping, lifetime <= current mark.
- `Owned` — escapes or unknown.
- `Foreign` — C-owned.

The decision is context-sensitive:

| Context | Class |
|---|---|
| string literal | Static |
| `f"..."` / concat used as `print` arg | Arena |
| `f"..."` bound to a non-escaping local | Arena |
| `f"..."` returned/stored/pushed/dict-put/passed to C | Owned |
| array/dict/struct literal element | Owned (phase 1) |
| function parameter | borrowed; promote on escape |
| string return | Owned (see return contract) |
| C call argument | Owned |
| Ahoy call argument | Arena allowed; callee dups on store |
| global / hot-reload / job | Owned |
| `any` / generic / unknown | Owned |

### 3.5 Promotion boundaries

These must all exist before routing anything:

| Site | Action |
|---|---|
| `return` string | promote to `Owned` |
| `arr.push` into owning array | `strdup` (already in `pushNeedsStrdup`) |
| array/dict/struct literal element | force `Owned` |
| struct field store | `strdup` (fix f-string case) |
| dict put | `strdup` |
| global/store | `strdup` or reject |
| closure capture | `Owned` env; capture by value/dup |
| `jobs.submit` arg | `Owned`/copy |
| job result | `Owned`/copy |
| FFI retaining call | `strdup`/copy; default C args `Owned` |
| hot-reload state | `Owned` |
| switch-expression case result | `Owned`/promote |

`MEMORY_OWNERSHIP.md` says promotion ships before routing. That is correct. Honor it.

---

## 4. Watermark runtime

**File ownership (read this first):** the arena runtime must live in the
host, not in generated code. Today `getArenaImplementation()`
(`source/codegen.go:962`) emits definitions into every generated `.c`,
which duplicates globals under hot reload and multi-TU builds.
Phase 1 must move the definitions to the host (`main.c` or a dedicated
`ahoy_arena.c`) and leave only `extern` declarations + prototypes in
generated headers (`source/codegen.go:539-551`). Generated code never
defines `__ahoy_frame_arena`, `ahoy_arena_mark`, or `ahoy_arena_rewind`.

**TLS spelling:** `__ahoy_frame_arena` and `__ahoy_current_arena` are
plain globals today — thread-hostile. Phase 6 makes them thread-local:

```c
#if defined(_MSC_VER)
#define AHOY_TLS __declspec(thread)
#elif defined(__STDC_VERSION__) && __STDC_VERSION__ >= 201112L
#define AHOY_TLS _Thread_local
#else
#define AHOY_TLS __thread
#endif
AHOY_TLS AhoyArena* __ahoy_frame_arena;
AHOY_TLS AhoyArena* __ahoy_current_arena;
```

Workers get their own arena instance (TLS pointer to a per-worker
arena, same pattern as frame — not a third global kind). Until phase 6,
document the globals as single-threaded-only.

**`live` vs `allocated`:** the current struct (`source/codegen.go:978-983`)
has `allocated` ("bytes handed out since last reset"). Phase 1 renames
this concept to `live` (bytes live under the current mark stack) and
keeps `highWater` as max `live`. Do not keep both counters — migrate
`allocated` to `live`, keep `highWater` incremental (no block walk).

Add to the host runtime (prototype in generated header):

```c
#define AHOY_ARENA_ALIGN 16

typedef struct AhoyArenaBlock {
    struct AhoyArenaBlock* next;
    size_t used;
    size_t cap;
    char data[];
} AhoyArenaBlock;

typedef struct AhoyArena {
    AhoyArenaBlock* current;
    size_t blockSize;
    size_t highWater;
    size_t live;
    unsigned gen;
} AhoyArena;

typedef struct AhoyArenaMark {
    AhoyArenaBlock* block;
    size_t used;
    size_t live;
    unsigned gen;
} AhoyArenaMark;
```

`AhoyArenaMark` must be visible to generated function bodies, so put the typedef in the header. `AhoyArenaBlock` can be forward-declared there.

Header additions:

```c
typedef struct AhoyArenaBlock AhoyArenaBlock;
typedef struct AhoyArenaMark AhoyArenaMark;
AhoyArenaMark ahoy_arena_mark(AhoyArena* a);
void ahoy_arena_rewind(AhoyArena* a, AhoyArenaMark m);
```

If generated functions live in multiple translation units, `ahoy_arena_mark/rewind` must be non-static and defined once in the host translation unit.

### 4.1 Allocation with incremental live/high-water

**NULL rule:** `ahoy_arena_alloc(NULL, ...)` silently falling back to
`malloc` (current `source/codegen.go:1009-1012`) becomes a class mismatch
once routing is on — an `Arena` value silently becomes `Owned`, so the
scope rewind leaks it (or cleanup frees the wrong class). Rule:

- Under `AHOY_ARENA_STRICT` (on with `-show_leaks`, on in tests): abort
  on NULL arena.
- Without strict: `malloc` fallback is allowed **only** for pre-`main`
  bootstrap before arenas are created, never for routed code. Routed
  call sites must pass a non-NULL arena; workers must have a TLS arena
  before any routed allocation.

```c
void* ahoy_arena_alloc(AhoyArena* a, size_t size) {
    if (!a || !a->current) {
#ifdef AHOY_ARENA_STRICT
        fprintf(stderr, "ahoy: arena allocation with no arena\n");
        abort();
#else
        return malloc(size);
#endif
    }
    size_t aligned = (size + (AHOY_ARENA_ALIGN - 1)) & ~(size_t)(AHOY_ARENA_ALIGN - 1);
    if (aligned < size) abort();                 /* overflow */
    if (aligned > a->current->cap - a->current->used) {
        size_t cap = a->blockSize;
        if (aligned > cap) cap = aligned;
        AhoyArenaBlock* nb = ahoy_arena_new_block(cap);
        nb->next = a->current;
        a->current = nb;
    }
    void* p = a->current->data + a->current->used;
    a->current->used += aligned;
    a->live += aligned;
    if (a->live > a->highWater) a->highWater = a->live;
    return p;
}
```

No block walk per allocation. `highWater` is incremental.

`ahoy_arena_calloc` adds:

```c
if (size != 0 && n > (size_t)-1 / size) abort();
```

### 4.2 Mark and rewind

```c
AhoyArenaMark ahoy_arena_mark(AhoyArena* a) {
    AhoyArenaMark m = {0};
    if (!a || !a->current) return m;
    m.block = a->current;
    m.used  = a->current->used;
    m.live  = a->live;
    m.gen   = a->gen;
    return m;
}

void ahoy_arena_rewind(AhoyArena* a, AhoyArenaMark m) {
    if (!a || !a->current || !m.block) return;
#ifdef AHOY_ARENA_STRICT
    if (m.gen != a->gen) {
        fprintf(stderr, "ahoy: stale arena mark (gen %u != %u)\n", m.gen, a->gen);
        abort();
    }
#endif
    AhoyArenaBlock* b = a->current;
    while (b && b != m.block) {
        AhoyArenaBlock* prev = b->next;
#ifdef AHOY_ARENA_POISON
        memset(b->data, 0xAB, b->used);
#endif
        b->used = 0;
        b = prev;
    }
    if (!b) {
#ifdef AHOY_ARENA_STRICT
        fprintf(stderr, "ahoy: arena mark block not found\n");
        abort();
#endif
        return;
    }
#ifdef AHOY_ARENA_POISON
    if (b->used > m.used) memset(b->data + m.used, 0xAB, b->used - m.used);
#endif
    b->used  = m.used;
    a->current = b;
    a->live  = m.live;
}
```

Important properties:

- Blocks are retained, not freed, so rewind is cheap and high-water is bounded by the peak, not by total allocations.
- Newer blocks are zeroed for reuse.
- Poison is debug-only (`-show_leaks`).
- A mark taken before `ahoy_arena_reset` is detected by `gen` and aborts under strict. In release, it returns without corrupting further.
- `ahoy_arena_reset` must do `a->live = 0; a->gen++;`. `gen` is never
  reset by rewind — only by reset/destroy. A stale mark (taken before a
  reset) aborts under `AHOY_ARENA_STRICT`.
- The fast path below only helps last-allocation-grows. It does **not**
  fix `AhoyArray` growth (`data`/`types` use `malloc`/`realloc`, not
  arena realloc) until phase 5 routes array buffers with element-class
  metadata.

### 4.3 In-place realloc fast path

```c
void* ahoy_arena_realloc(AhoyArena* a, void* ptr, size_t oldSize, size_t newSize) {
    if (newSize == 0) return NULL;
    if (a && a->current && ptr) {
        size_t oldAligned = (oldSize + (AHOY_ARENA_ALIGN - 1)) & ~(size_t)(AHOY_ARENA_ALIGN - 1);
        size_t newAligned = (newSize + (AHOY_ARENA_ALIGN - 1)) & ~(size_t)(AHOY_ARENA_ALIGN - 1);
        char* end = (char*)ptr + oldAligned;
        if ((char*)ptr >= a->current->data &&
            end == a->current->data + a->current->used) {
            size_t base = a->current->used - oldAligned;
            if (base + newAligned <= a->current->cap) {
                a->current->used = base + newAligned;
                if (newAligned >= oldAligned) a->live += newAligned - oldAligned;
                else a->live -= oldAligned - newAligned;
                if (a->live > a->highWater) a->highWater = a->live;
                return ptr;
            }
        }
    }
    void* np = ahoy_arena_alloc(a, newSize);
    if (ptr && oldSize) memcpy(np, ptr, oldSize < newSize ? oldSize : newSize);
    return np;
}
```

This only helps the "last allocation grows" case. That is the common string/append case; do not claim more.

---

## 5. Exact watermark emits

> **Amendment (mark storage):** do NOT emit a block-local
> `AhoyArenaMark __scope_mark_N` declaration per scope. `switch` dispatch
> jumps directly to `case` labels, skipping prior initializers, and the
> halt-across-switch `goto` makes per-block declarations fragile (jump
> over initialization = indeterminate mark). Instead each function gets
> one depth-indexed mark stack declared once at function top; entries
> are assigned at scope entry and read at scope exit / early exit. No
> new declarations per scope, no `varCounter` explosion, depth reuse is
> safe because index == depth.

### 5.1 New generator state

```go
arenaRouting       bool
arenaMarksEnabled  bool   // false until phase 2 proves overhead acceptable
fnMarkDepth        int    // depth index holding the function-entry mark (always 0)
maxMarkDepth       int    // high-water of scopeDepth seen; sizes __ahoy_marks[]
varAllocClass      map[string]AllocationClass
inReturnExpr       bool
inRetainingLiteral bool
inForeignCall      bool
```

`arenaRouting` is a compiler flag, default off until phase 3.
`scopeArenaMarks map[int]string` is deleted — replaced by the indexed
`__ahoy_marks[]` stack. `fnArenaMark string` is deleted — the function
mark lives at `__ahoy_marks[0]`.

### 5.2 Function entry

In `generateFunction`, after the `main` arena creation block
(`source/codegen.go:2838-2843`) and before the body, when
`gen.arenaRouting && gen.arenaMarksEnabled`:

1. Compute `maxMarkDepth` for the function (or use a fixed cap such as
   64 and abort codegen if `scopeDepth` exceeds it — simpler v1).
2. Emit once at function top:

```c
AhoyArenaMark __ahoy_marks[64]; // sized to maxMarkDepth; 64 is the v1 cap
__ahoy_marks[0] = ahoy_arena_mark(__ahoy_frame_arena);
```

For `main`, this must come after
`__ahoy_frame_arena = ahoy_arena_new(...)`. For other functions, the
arena already exists.

If `currentFunctionHasMultiReturn` is true, set `gen.arenaRouting = false` for that function (restore after). Multi-value returns need per-field promotion; do not mix that into phase 1.

**Which arena:** phases 1–3 route everything to `__ahoy_frame_arena`
explicitly. `__ahoy_current_arena` is left NULL (or mirroring frame)
until scene routing lands — it exists for the future scene/worker
selection, not for phase 1–3 code to use. This avoids the silent
NULL → `malloc` fallback.

### 5.3 Scope entry

`enterScope()` (`source/codegen.go:6325`) is currently compile-time only.
When `gen.arenaRouting && gen.arenaMarksEnabled`, codegen emits an
assignment (not a declaration) to the function-local stack at the C
scope entry point:

```go
func (gen *CodeGenerator) enterScope() {
    gen.scopeDepth++
    gen.scopeDeclared = append(gen.scopeDeclared, map[string]bool{})
    if gen.scopeDepth > gen.maxMarkDepth {
        gen.maxMarkDepth = gen.scopeDepth
    }
}
 // at the C emission site (right after the emitted `{`), not inside enterScope itself:
 // __ahoy_marks[<depth>] = ahoy_arena_mark(__ahoy_frame_arena);
```

The assignment is emitted by the caller that just wrote `{` (loop/if/
switch-case/`#if`/CFG block), immediately after the brace, so the C
position matches the runtime scope entry. `enterScope()` itself stays
declaration-free — it only tracks depth.

Caveat: audit every `enterScope()` call site. They are all
statement-block entries today (`if`, `else`, loops, switch cases,
`#if`, CFG blocks), so an assignment statement is valid. If any call
site is mid-expression, add a separate `enterExpressionScope()` that
does not emit a mark. Phase 2 defaults to function + loop + switch
marks only (see phase 2); per-`if` marks are opt-in after measurement.

Why indexed, not block-local: a `switch` jump lands directly on a
`case` label and would skip a block-local initializer; the halt
`goto` would do the same. An assignment into a function-scope array
has no such hazard.

### 5.4 Scope exit

At the end of `exitScope()` (`source/codegen.go:6618`), after deferred
statements and after the `Owned`-free loop, before `scopeDepth--`:

```go
if gen.arenaRouting && gen.arenaMarksEnabled {
    fmt.Fprintf(&cleanup, "    ahoy_arena_rewind(__ahoy_frame_arena, __ahoy_marks[%d]);\n", gen.scopeDepth)
}
```

No map delete is needed — the slot is overwritten on next entry to the
same depth. Sibling scopes reusing a depth number are safe for the same
reason.

Order is deliberate:

1. block-scoped `defer` statements,
2. `Owned` frees,
3. arena rewind.

A deferred statement may read arena data, so it must run before the rewind. An `Owned` destructor must not traverse arena fields; phase 1 forbids that by not routing containers.

### 5.5 Early exits: `halt` / `next`

`generateEarlyExitCleanupTo(targetDepth)` (`source/codegen.go:6471`)
already knows the target depth. After its free loop, append:

```go
if gen.arenaRouting && gen.arenaMarksEnabled {
    fmt.Fprintf(&cleanup, "    ahoy_arena_rewind(__ahoy_frame_arena, __ahoy_marks[%d]);\n", targetDepth)
}
```

Why no map bookkeeping: the codegen map is static, runtime execution is
conditional. The early-exit rewind and the fall-through `exitScope`
rewind never both run at runtime — only one path executes — so emitting
both is safe. Inner-depth slots are intentionally left intact: the
fall-through path still needs them for its normal `exitScope`; the
early path already unwound past them by rewinding directly to the outer
target (which discards inner allocations too). Do not "clean up" inner
slots on the early path.

- `next` targets `loopScopeStack[len-1]`, which is the loop body depth.
- `halt` outside a switch targets the loop body depth.
- `halt` breaking a switch targets `switchCaseDepths[len-1]`, the case depth.
- `halt` across a switch to the loop uses `generateEarlyExitCleanup(false)`, which targets the loop body depth. Rewinding directly to the loop mark unwinds the case and any nested `if` marks. Correct.

### 5.6 Return

`generateReturnStatement` (`source/codegen.go:5725`) is the delicate one. The current order is:

1. mark returned values escaping,
2. maybe stash the return expression in `__ret_N`,
3. free heap locals,
4. run deferred statements,
5. emit `return expr;` (the expression is generated *after* the cleanup).

For arena routing, the return expression must be evaluated **before**
the rewind if it can read arena data. The safe rule:

- If any name mentioned in the return expression has
  `varAllocClass == AllocArena`, force the existing `needTemp` path.
  Evaluate the expression into `__ret_N` before cleanup. Note:
  `returnMentioned` is collected via `collectDeferredIdents` plus
  `fstringLeadingIdentifiers` (legacy path); the modern
  `fstring_children.go` path must also contribute its interpolated
  identifiers or an Arena f-string read will be missed.
- If the expression is a bare string identifier with Arena class, emit
  `strdup(expr)` for the temp. Otherwise the temp would just copy an
  arena pointer.
- Fresh allocations inside the return expression are forced `Owned` via
  `gen.inReturnExpr = true`. Exact hooks that must consult this flag:
  `generateFStringFromChildren` (`source/fstring_children.go:38`),
  legacy f-string emission (`source/codegen.go:12497-12700`), concat
  binary-op emission (`source/codegen.go:8861-8915`), string
  `slice`, and fresh-returning method/call paths. When
  `inReturnExpr` (or `inRetainingLiteral` / `inForeignCall`) is set,
  these emit `ahoy_owned_sprintf` / `malloc`-backed concat instead of
  arena-backed.
- After cleanup and defers, emit:

```c
ahoy_arena_rewind(__ahoy_frame_arena, __ahoy_marks[0]);
return __ret_N;
```

Required order: temp eval → `Owned` frees → deferred statements →
rewind(fn mark at index 0) → `return temp`. The temp eval must precede
cleanup (current code generates the return expression *after* cleanup
unless `needTemp` — hence forcing `needTemp`), and defers must precede
rewind (a defer may read arena data).

- For `void` returns, emit the rewind just before `return;`.
- For the switch-return path at `source/codegen.go:5851`, hoist the `__switch_ret_N` generation above the cleanup loop, or keep the entire function on `Owned` until that refactor is done.
- For multi-value returns, disable arena routing for that function in phase 1.

The emitted shape:

```c
{
    char* __ret_10 = strdup(s);   /* only when s is Arena */
    /* Owned frees + deferred statements */
    ahoy_arena_rewind(__ahoy_frame_arena, __ahoy_marks[0]);
    return __ret_10;
}
```

### 5.7 Function end

After `addAutomaticDeferFrees()` and after the deferred-statement loop
(`source/codegen.go:2861-2869`), before the `main` teardown report:

```go
if gen.arenaRouting && gen.arenaMarksEnabled {
    gen.writeIndent()
    gen.output.WriteString("ahoy_arena_rewind(__ahoy_frame_arena, __ahoy_marks[0]);\n")
}
```

Then `main` prints high-water and destroys the arenas. After rewind,
`live` should be back to the function-entry value (normally 0). Add a
strict assertion (`AHOY_ARENA_STRICT`: `if (a->live != entryLive) abort()`).

### 5.8 Remove the frame reset

Delete these lines from `generateFunction`:

```go
if funcName == "engine_frame" || funcName == "engine_frame_3d" {
    ahoy_arena_reset(__ahoy_frame_arena);
}
```

The function mark at `engine_frame` entry and rewind at exit replace it. This fixes reentrancy and stale caller data.

Keep the scene reset for now only because scene allocation is idle. When scene routing starts, replace it with the double-buffer/teardown design.

### 5.9 Emitted loop example

```c
int frame_work(int n) {
    AhoyArenaMark __ahoy_marks[64];
    __ahoy_marks[0] = ahoy_arena_mark(__ahoy_frame_arena);
    int total = 0;

    for (int i = 0; i < n; i++) {
        __ahoy_marks[1] = ahoy_arena_mark(__ahoy_frame_arena);

        char* s = ahoy_arena_sprintf(__ahoy_frame_arena, "entity-%d", i);
        printf("%s\n", s);
        total += (int)strlen(s);

        ahoy_arena_rewind(__ahoy_frame_arena, __ahoy_marks[1]);
    }

    ahoy_arena_rewind(__ahoy_frame_arena, __ahoy_marks[0]);
    return total;
}
```

Break (loop body is depth 1, `if` inside it is depth 2):

```c
for (...) {
    __ahoy_marks[1] = ahoy_arena_mark(__ahoy_frame_arena); // loop body
    if (cond) {
        __ahoy_marks[2] = ahoy_arena_mark(__ahoy_frame_arena); // if scope
        ...
        ahoy_arena_rewind(__ahoy_frame_arena, __ahoy_marks[1]); // exit loop body, not [2]
        break;
    }
    ...
    ahoy_arena_rewind(__ahoy_frame_arena, __ahoy_marks[1]);
}
```

Note the rewind targets `__ahoy_marks[1]`, not `[2]`: break exits the
loop body, so the loop body mark is the target. Rewinding to `[1]`
also discards the `if`-scope allocations — correct.

---

## 6. Strings and f-strings

### 6.1 Replace the static buffer

Add runtime helpers (host runtime, `<stdarg.h>` required):

```c
char* ahoy_arena_sprintf(AhoyArena* a, const char* fmt, ...);
char* ahoy_owned_sprintf(const char* fmt, ...);
```

Implementation (note `va_copy` — `va_list` reuse without copy is UB):

```c
char* ahoy_arena_sprintf(AhoyArena* a, const char* fmt, ...) {
    va_list ap, aq;
    va_start(ap, fmt);
    va_copy(aq, ap);
    int n = vsnprintf(NULL, 0, fmt, aq);
    va_end(aq);
    if (n < 0) { va_end(ap); return NULL; }
    char* out = ahoy_arena_alloc(a, (size_t)n + 1);
    vsnprintf(out, (size_t)n + 1, fmt, ap);
    va_end(ap);
    return out;
}
// ahoy_owned_sprintf: identical with malloc((size_t)n + 1).
```

No 256-byte limit.

Current offending sites:

- `source/fstring_children.go:61-68` — per-call-site `static char __fstr_buf_N[256]`.
- `source/codegen.go:12633-12641` — legacy `static char buffer[256]` + `sprintf`.

### 6.2 Escape literal `%`

While building `formatStr` from `NODE_STRING` segments, replace `%` with `%%`:

```go
func escapeFmtLiteral(s string) string {
    return strings.ReplaceAll(s, "%", "%%")
}
```

This is unconditional: a literal `%%` becomes `%%%%`, which prints
`%%` — correct, because the source text meant two literal percent
signs, not an escape. Do not try to detect "already escaped" sequences;
f-string literals have no escape processing today, so every `%` is literal.

Apply to both `fstring_children.go` and the legacy textual path. Add a test:

```ahoy
@ main ||:
    i: 7
    print|f"100% done {i}"|
$
```

Expected: `100% done 7`, not a format-string crash.

### 6.3 Remove the `strdup` wrappers around bound f-strings

Current binding sites duplicate the static buffer:

- `source/codegen.go:3412`
- `source/codegen.go:3547`
- `source/codegen.go:3871`
- `source/codegen.go:3961`

Once f-strings produce arena/owned strings, these wrappers must become class-aware. Otherwise you allocate twice and leak one.

### 6.4 Fix the "fresh f-string is immortal" assumptions

`fieldStoreNeedsDup` (`source/codegen.go:6049`) currently returns false for `NODE_F_STRING`. That is only true while f-strings are static buffers. Change it: an f-string stored into a struct field must be duplicated, because the field outlives the statement. `slotStoreNeedsDup` already returns true for f-strings; keep that.

Similarly, the concat stash path at `source/codegen.go:8904-8912` frees fresh operands. If an operand is an Arena f-string, `free()` is invalid. Gate every generated `free` on `Owned`:

```go
if gen.exprAllocClass(left) == AllocOwned { /* free */ }
```

### 6.5 Return contract audit

This is a prerequisite for routing string returns. Current behavior mixes:

- `return "literal"` — static pointer,
- `return param` — borrowed pointer,
- `return "a" + b` — fresh malloc,
- `return f"..."` — static buffer (currently).

A caller cannot know which of these it owns. Before arena routing, define one contract:

**v1 rule: every returned string is `Owned`.** Promote literals, parameters, and arena values at the return. Callers may free returned strings. This adds a copy for literals/params but makes ownership uniform. Add tests for all four shapes.

If that is too invasive, the alternative is an explicit `borrowed` return contract, but Ahoy has no annotation system today.

---

## 7. Phased implementation plan

**Progress (2026-11-04):** All phases substantially implemented. Fixed returnTypeCanHoldString to treat void/empty return types as non-string-capable, so f-strings in void functions (like main) route to arena instead of owned — frame high-water now flat (16 bytes across 100k iterations, measured). Phase 4b: statement-level + print-hoisted owned string call results freed; arbitrary expression-position calls are disallowed by the language (must assign to a variable first), so no further work needed. Phase 5: primitive array literals route to frame arena; arena push via side table; dicts own string values; mixed-class promotion at array element stores (owned strings promoted to arena via ahoy_arena_sprintf, original freed) and at dict/struct stores (non-owned strings promoted to Owned via strdup, since those containers are always Owned). Leak tests leak_dict_strings + leak_mixed_class_array pass. Phase 6: TLS globals, per-worker arenas + per-job rewind, AHOY_ARENA_STRICT, scene double-buffer infra + teardown ordering (game_switch_scene calls ahoy_scene_arena_swap after scene_free/Owned teardown, before scene_load), hot-reload module guard, FFI C-arg promotion, job arg promotion + result transfer. Hot-reload state audit: compiler warns when a global is assigned an arena-class value (generateAssignment); Bell audit complete — grep of bell/engine/*.ahoy shows no module-level globals referencing scene data (state lives in Assets/Renderer structs passed as args; reload at frame boundary). Documented in ahoy/docs/FFI_RETAIN_LIST.md. FFI retain list: C-arg promotion to Owned enforced in generateMaybeCharArg; compiler field ffiRetainFunctions added for future tracked retainers; retain list documented in ahoy/docs/FFI_RETAIN_LIST.md (glue.h callbacks store AhoyFn by value; env malloc'd and owned by node; raylib/SDL handles are C-owned). Verification: `go build` clean; `go test` only 3 pre-existing formatter failures; `tools/mem_check.sh` PASS (all leak programs); `tools/bell_mem_check.sh` PASS (90 frames, no memory errors); frame high-water 16 bytes flat (measured); Bell frame time 1.019 ms (solids/300 baseline) vs 1.012 ms baseline = +0.7% (within 1% acceptance). All compiler-side acceptance criteria satisfied.

### ✅ Phase 0 — Baseline and gates (DONE: flags added)

**Goal:** no behavior change; make the work measurable and reversible.

- Add compiler flags:
  - `-arena-routing=off|temps|locals|all` (default `off`).
  - `-arena-marks=on|off` for overhead measurement.
  - `-fstring-legacy` for rollback.
- Define `AHOY_ARENA_STRICT` alongside `-show_leaks`.
- Add the invariants and class table to `docs/MEMORY_OWNERSHIP.md`.
- Add failing/skipped tests that demonstrate current f-string bugs:
  - stored f-string mutates,
  - `%` in literal,
  - >255 bytes,
  - returned f-string.
- Record baseline: Bell 1.012 ms/frame, 90/90 frames, 19,021 B/1,201 allocs exit leaks, zero ASan errors.

**Exit:** baseline reproduced; flags inert; all existing tests pass.

### ✅ Phase 1 — Watermark runtime (DONE)

**Goal:** implement mark/rewind correctly before routing anything.

Files: `source/codegen.go` (`getArenaImplementation`, header emission), new `source/arena_runtime_test.go` or a C harness test.

- Add `live`, `gen`, `AhoyArenaMark`.
- Add `ahoy_arena_mark`, `ahoy_arena_rewind`.
- Add incremental high-water.
- Add realloc fast path, overflow checks, 16-byte alignment.
- Add strict NULL behavior.
- Add reset generation bump.
- Unit test: mark -> alloc -> alloc -> rewind -> reuse; mark -> reset -> rewind aborts; realloc in place; poison under debug.

**Exit:** all tests pass; no routing; runtime unit test passes.

### ✅ Phase 2 — Emit marks with routing off (DONE)

**Goal:** validate the emitted C and measure overhead with no class changes.

**Default mark set (measure this first):** function + loop + switch-case
marks only — not every `enterScope`. Per-`if` marks multiply mark traffic
for no lifetime win in phase 2 (nothing is routed yet). Enable per-`if`
marks only if the function+loop+switch default measures <1% frame-time
overhead. Either set is safe (fewer marks = higher high-water, never
unsound); the tradeoff is overhead vs peak, not correctness.

Files: `source/codegen.go` (`generateFunction`, `enterScope`, `exitScope`, `generateEarlyExitCleanupTo`, `generateReturnStatement`), `source/memory_phase4_test.go`.

- Emit function mark stack (`__ahoy_marks[]` + `__ahoy_marks[0]`) and function-end rewind.
- Emit loop/switch marks and exit/early-exit rewinds (per-`if` behind a flag).
- Emit return rewinds (with temp rules, but no Arena values yet).
- Remove `engine_frame` reset.
- Add codegen-text assertions: `__ahoy_marks[` present, `__ahoy_marks[0]`
  rewound at function end and before every `return`, no `free` on
  marks, no `static __fstr_buf`.
- Count marks: `grep -c "__ahoy_marks\["` on Bell-generated C for the
  overhead story.
- Run full tests, `tools/mem_check.sh`, Bell ASan 90 frames.
- Measure frame time and malloc count.

**Exit:** generated C compiles; behavior identical; no stale-mark aborts; frame-time overhead < 1% on the default mark set. If not, keep the reduced mark set (function+loop only) — safe, higher high-water.

### ✅ Phase 3 — F-string and concat temporaries (SUBSTANTIALLY DONE)

**Goal:** the first real routing win, with no container/return complexity.

- Add `varAllocClass`, `exprAllocClass`, `inReturnExpr`, `inRetainingLiteral`.
- Replace static f-string buffers with `ahoy_arena_sprintf` / `ahoy_owned_sprintf`.
- Escape `%`.
- Remove/classify the f-string `strdup` wrappers.
- Fix `fieldStoreNeedsDup` for f-strings.
- Gate concat stash frees on `Owned`.
- Route only allowlisted temporaries:
  - f-string/concat/slice bound to a non-escaping local,
  - f-string/concat passed to `print` or a comparison,
  - no return/store/container/C-call context.
- Add tests:
  - `leak_loop_fstring.ahoy`,
  - `leak_loop_concat.ahoy`,
  - `fstring_store.ahoy`,
  - `fstring_percent.ahoy`,
  - `fstring_long.ahoy`.

**Exit:** zero lost in the new leak tests; f-string semantic tests pass; Bell 90/90 zero ASan; frame time within 1%; no invalid frees.

### ✅ Phase 4 — Named locals, returns, switch cases, defer (PARTIAL: generateFreeCodeForVar/addAutomaticDeferFrees class-aware, switch-case inRetainingLiteral, return promotion branch, varAllocClass tracking)

**Goal:** route non-escaping local strings safely.

- Add a pre-pass or boundary-promotion logic for named locals.
- Implement the return contract (returned strings are `Owned`).
- Promote `return` of Arena identifiers.
- Promote switch-expression case results before the case rewind.
- Make `generateFreeCodeForVar`, `addAutomaticDeferFrees`, `exitScope`, and early-exit cleanup class-aware.
- Handle reassignment: fixed class per binding, or force `Owned` if any assignment is `Owned`.
- Tests: nested scopes, early return, `halt`/`next`, switch expression, `defer` using arena temps, reassignment.

**Exit:** all `leak_*.ahoy` zero lost; high-water flat after the first iteration; Bell clean; no double frees.

### Phase 4b — Owned inline temporaries (if leak-freedom is the goal)

Arena routing does not own the result of an `Owned` call used inline:

```ahoy
print|make_label()|
```

If `make_label` returns `Owned`, nobody frees it. The compiler needs a hidden temp plus `free` at statement end for known-owning call returns, or the callee must return into a caller-provided arena. This is not watermark work, but it is required for "applications never leak."

**Exit:** a loop of `print|make_label()|` has zero per-iteration growth. If deferred, document it as a known gap.

### Phase 5 — Containers (NOT STARTED: requires side-table elem class + arena-aware push/free)

**Goal:** route homogeneous arena containers without breaking `freeArray`.

- Default to a compiler side table (`map[*AhoyArray]elemClass`), NOT an
  `AhoyArray` struct bump. Adding `alloc_class`/`elem_class` fields is a
  deliberate C ABI break for every C user including Bell; the side table
  proves the design first. Promote to struct fields only with a versioned
  ABI note and a Bell migration.
- Make `ahoy_array_push`, `freeArray`, `generateFreeCodeForVar`, `fill`, `remove`, `sort` class-aware.
- Enforce: an Arena array may hold only Arena/Value elements; an `Owned` array may hold only `Owned`/Value elements. Mixed requires promotion.
- Keep dicts `Owned`; `freeHashMap` recursively frees and needs the same treatment later.
- Tests: array of strings, nested arrays, struct elements, push/remove/sort; zero lost, no invalid free.

**Exit:** container leak tests zero lost; Bell clean; no ABI break (side table; struct bump only as a documented follow-up).

### ✅ Phase 6 — Scene, threads, jobs, FFI, hot reload (PARTIAL: scene reset removed from entry; AHOY_TLS macro added; full TLS/scene/jobs/FFI pending)

- Scene: explicit teardown + reset-after-swap is the v1; double-buffered
  arenas (`current`/`next`, load into `next`, swap, reset old after
  teardown) are the end-state. Either way: do not reset at
  `game_switch_scene` entry; globals/hot reload must not point into scene.
- Threads: `AHOY_TLS` (`_Thread_local` / `__thread` /
  `__declspec(thread)` per §4) for `__ahoy_current_arena` and
  `__ahoy_frame_arena`; per-worker arena instance; strict NULL (no
  silent `malloc` fallback for routed code).
- Jobs: args/results `Owned` or copied before worker arena reuse.
  `ahoy_jobs_result` must never point into a reused worker arena.
- FFI: C args default `Owned`; explicit retain list; C structs with pointers promoted.
- Hot reload: arena runtime definitions live in the host (see §4 file
  ownership); state in `Owned`/globals; reload at frame boundary. No
  second copy of globals in the reloaded module.
- Tests: repeated scene switch flat; jobs ASan clean; hot reload state survives.

---

## 8. Test matrix and acceptance criteria

| Test | Proves |
|---|---|
| `go test ./...` | codegen, semantics, no regressions |
| `bash tools/mem_check.sh` | all `leak_*.ahoy` zero lost under valgrind |
| `tools/bell_mem_check.sh` | Bell 90 frames, ASan/LSan clean |
| `-show_leaks` high-water parse | loop high-water bounded, not O(iterations) |
| New `leak_loop_fstring/concat` | per-iteration temps are reclaimed |
| `fstring_store/percent/long` | semantic bugs fixed |
| `leak_early_return/switch/defer` | mark unwinding correct |
| Massif on Bell | heap flat, no growth trend |
| Bell bench | frame time within 1%; malloc count down |
| Codegen-text tests | marks at entry, rewinds at exits, no `free` on Arena, no static `__fstr_buf` |
| Mark-count check | `grep -c "__ahoy_marks\["` on generated C tracks overhead |
| Stale-mark C harness | generation check aborts under strict |

**Acceptance:**

- Zero definitely/indirectly lost in all leak tests.
- Zero ASan UAF on Bell 90 frames + editor.
- Frame high-water flat after the first loop iteration.
- Bell frame time <= baseline x 1.01.
- Exit leaks no worse than baseline (19,021 B/1,201 allocs).
- No `free` emitted for `Arena`/`Static` values.
- No arena pointer stored in an `Owned` container or returned without promotion.
- `TestLinting` stays clean.

**Non-goal:** zero process-exit leaks. The remaining Bell leaks are boot/load-bounded teardown omissions; fixing them is a separate `scene_free` project.

---

## 9. Biggest risks

1. **Missed escape -> UAF.** Mitigation: default `Owned`, allowlist routing, ASan on every change, promotion boundary tests.
2. **Class transition on reassignment.** Mitigation: fixed class per binding, or force `Owned` if any assignment is `Owned`.
3. **Stale mark after reset.** Mitigation: generation counter + strict abort; remove frame reset.
4. **Container element mismatch.** Mitigation: do not route containers until element class metadata exists.
5. **Scene reset before teardown.** Mitigation: double-buffer scene arenas; do not auto-reset at function entry.
6. **Job result lifetime.** Mitigation: `Owned`/copy.
7. **Overhead.** Mitigation: measure; fall back to marks only at function/loop/switch boundaries.
8. **F-string semantic changes.** Mitigation: add the semantic tests first; keep the legacy path behind a flag until proven.

---

## 10. Bottom line

Muse's review is the best of the prior takes on *direction*. Keep its priorities:

- strings first,
- rewind not reset,
- keep `Owned`,
- do not chase Goose's type system,
- measure instead of promising FPS.

Reject two concrete designs:

- **stack-header arena arrays** — an ABI/language change disguised as "Goose-lite,"
- **"replace frees with rewinds in `exitScope`"** — marks must be emitted at entry, across four cleanup paths, with per-binding class tracking.

Recommended first milestone:

1. **Phase 1** — runtime mark/rewind, incremental live/high-water, generation check.
2. **Phase 2** — emit function/scope marks with routing off; prove the C compiles and overhead is acceptable.
3. **Phase 3** — route f-string/concat temporaries only; fix the static buffer, `%` escaping, truncation, and stored-value mutation.

That sequence gets the real safety win — no per-frame growth from hot string temps — without touching arrays, dicts, scene graphs, or jobs. Once that is clean under ASan and valgrind, the same machinery can be extended to named locals, then containers, then scene.

The watermark is easy. The ownership classes and promotion boundaries are the project. If another model implements only the watermark and starts routing, it will turn sound leaks into unsound UAFs.

---

## Appendix A — Code sites referenced

| Area | Site |
|---|---|
| Arena runtime | `source/codegen.go:962-1080` (`getArenaImplementation`) |
| Arena header/prototypes | `source/codegen.go:539-551` |
| Function prologue/teardown | `source/codegen.go:2838-2885` |
| `enterScope` | `source/codegen.go:6325` |
| `exitScope` | `source/codegen.go:6618` |
| Early-exit cleanup | `source/codegen.go:6457-6508` |
| Return statement | `source/codegen.go:5725-5928` |
| `addAutomaticDeferFrees` | `source/codegen.go:6827` |
| `generateFreeCodeForVar` | `source/codegen.go:6713` |
| `fieldStoreNeedsDup` | `source/codegen.go:6049` |
| `pushNeedsStrdup` | `source/codegen.go:6163` |
| Concat emission/stash | `source/codegen.go:8861-8915` |
| Modern f-string emission | `source/fstring_children.go:38-94` |
| Legacy f-string emission | `source/codegen.go:12497-12700` |
| Array literal | `source/codegen.go:14268` |
| `AhoyArray` layout | `source/codegen.go:616-625` |
| `ahoy_array_push` | `source/stdlib.go:26` |
| Frame/scene hooks | `source/codegen.go:2844-2857` |
| `main` teardown | `source/codegen.go:2876-2885` |

## Appendix B — Glossary

- **Arena / region** — bump-allocated block list with a lifetime.
- **Mark** — `{block, used, live, gen}` snapshot of an arena.
- **Rewind** — restore an arena to a mark; poison and reuse newer blocks.
- **Reset** — clear all blocks; invalidates every mark.
- **Allocation class** — `Value`, `Static`, `Arena`, `Owned`, `Foreign`.
- **Promotion** — copying a value into a longer-lived class/region.
- **Escape site** — return, push, dict put, field store, global, closure capture, job, retaining FFI, hot-reload state.
