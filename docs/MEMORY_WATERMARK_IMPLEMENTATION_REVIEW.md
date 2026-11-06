# Ahoy Watermark Memory Plan — Implementation Review

**Date:** 2026-11-05
**Reviewer:** master agent (Beast mode)
**Plan under review:** `ahoy/docs/MEMORY_WATERMARK_PLAN.md`
**Reviewed code:** working tree at commit `696961c` (source has uncommitted modifications)
**Companion docs:** `MEMORY_MODEL.md`, `MEMORY_OWNERSHIP.md`, `MEMORY_BASELINE.md`, `FFI_RETAIN_LIST.md`

---

## 12. Implementation progress (2026-11-06 → 2026-11-07)

Follow-up agent implemented the review's fix order §5 items 1–10 (core),
plus P1-1/P1-2/P1-6, P0-6/P0-7, and P2 hardening. Verified with
`go test`, `tools/mem_check.sh` (now 12 leak programs, all clean, now
with `-show_leaks`/strict+poison), the ten Appendix A repros (all zero
lost, zero errors), `-arena-routing=all` int-push/fill probes (previously
SIGSEGV / `realloc(): invalid pointer`, now clean), and editor codegen.

A second pass (2026-11-07) closed the f-string residual hazard, the
dict-formatting static buffers in its pipeline, the `allocated`
counter, and side-table hygiene; details at the end of §12.

### Fixed

- **P0-1** (`methods.go:valueAllocatesString`): user-function string
  returns now count as fresh/owned (with alias-callee guard), so
  declarations like `s: make||` register and free at scope exit.
  `isOwnedStringExpr` F-string case now mirrors the emitter's
  owned-helper condition (legacy/return/retaining/foreign/return-type),
  and `isArenaStringExpr` mirrors the complement.
- **P0-2** (`codegen.go` reassignment): removed the unconditional
  tracking deletes; the fresh-string branch now registers the new value
  at the lexical declaration depth (new `lexicalDeclDepth` helper, stale
  inner-depth entries pruned) and only static literals untrack.
  Scalar string-literal declarations now emit `strdup` + track, so the
  reassignment eager-free is always sound. F-string reassign class is
  always `AllocOwned` (emission is always `strdup`, now via
  `emitStringCopy`).
- **P0-3** (element stores): typed `array[string]` stores evaluate the
  new value into a temp, free the old slot when the array already owned
  it (pointer-inequality guard for self-store), and set the slot type.
  New arrays own string slots by construction (see below).
- **P0-4** (literal double-copy): declaration/reassignment F-strings,
  struct-literal F-strings, and `promoteOwnedStringArrayLiteral` (now
  evaluates ephemerality under the retaining context) all transfer or
  copy-and-free via `emitStringCopy`. Struct literals union literal-
  present string fields into `ownedStructDecls`. Dict puts already used
  `emitStringCopy` and are now correct via the fixed ephemerality test.
  Array-literal and reassignment sites promote string elements to owned
  by construction (dups non-ephemeral, transfers ephemeral).
- **P0-5** (float boxes): array teardown now always frees
  `AHOY_TYPE_FLOAT` slots by runtime type; `arrayLiteralAllPrimitive`
  excludes floats/doubles so arena arrays never hold heap boxes.
- **P0-6** (arena arrays): both push sites guard the arena-string copy
  with `isStringLikeType`, fixing the int-push SIGSEGV;
  `ahoy_array_fill` uses `ahoy_arena_realloc` when registered, fixing
  the `realloc(): invalid pointer` abort.
- **P0-7** (editor): `editor_open_scene` calls `scene_free|nodes|`
  before popping; live-sync restored to every-15-frames via `play_tick`
  (comment already promised this); removed 13 now-redundant manual
  `free|play_file[0]|` / `free|play_last[0]|` pre-store frees (element
  stores free the old slot themselves — keeping them double-frees).
- **P1-1**: return expressions naming `AllocArena` values force the
  stash-temp path; legacy textual f-string identifiers now join
  `returnMentioned`.
- **P1-2**: `arenaRouting` save/restore around multi-return functions.
- **P1-6**: new `ahoy_arena_high_water()` accessor; main no longer
  dereferences opaque `AhoyArena*` (fixes `-release -show_leaks`
  multi-TU gcc build).
- **P1-4 (partial)**: job workers save/restore the previous
  `__ahoy_frame_arena` instead of clearing it (fixes post-exit NULL
  clobber under tcc; in-flight sharing without real TLS remains —
  use gcc/clang for threaded workloads).
- **P2 runtime**: 16-byte header padding (`_rsvd`), block-size overflow
  check + NULL check, realloc alignment overflow checks, rewind treats
  stale gen as return-in-release/abort-in-strict, `enterScope` aborts
  codegen past depth 63.
- **Tests**: 7 new `test/input/leak_*.ahoy` programs (reassign, user
  call, element store, float array, array/struct/dict literal);
  `memory_slot_store_test.go` literal expectation updated to the
  own-by-construction model; `tools/mem_check.sh` now builds leak
  programs with `-show_leaks`.
- **Docs**: `FFI_RETAIN_LIST.md` corrected (C args are borrowed, not
  promoted; hot-reload module guard pending).

### Deferred (documented, not done)

- Full P1-5 (host-owns-arena for hot reload). Analyzed further and
  deliberately not half-done: the host loads `libgame.so` with
  `RTLD_LOCAL`, so host-defined arena globals would not resolve in the
  module without `-rdynamic` host linkage (dubious under tcc, which
  builds both sides in the common path); the game entry re-creates
  arenas per reload (lifecycle work beyond linkage); and the reload
  pipeline cannot be fully verified headless here. With routing off and
  reload at frame boundaries, duplicate empty arenas per reload are the
  only current cost.
- Rewind-time side-table staleness (shell memory reused after rewind
  keeps its entry): safe while the frame arena is the only allocating
  arena; revisit when scene routing lands.
- `bell_mem_check.sh` still does not fail on leaks (known 19KB boot
  baseline); `scene_free` remains shallow (names/strings inside
  components); per-open name leaks remain bounded, not per-frame.
- Same static-buffer pattern remains in `ahoy_json_stringify` (8KB) and
  the stdlib fs-listing cache — JSON printing / fs listing, not the
  f-string pipeline; same treatment applies when those are touched.

### Fixed (2026-11-07 follow-up)

- **F-string residual hazard**: the legacy textual path (`generateFString`
  with no pre-parsed `Children`, reached when a `{group}` fails sub-parse
  and in lint flows) no longer emits `static char __fstr_buf_N[256]` +
  `sprintf`. It now emits the same `ahoy_arena_sprintf` /
  `ahoy_owned_sprintf` call as the pre-parsed path, with the same
  lifetime-context helper selection. Verified: no `__fstr_buf_` remains
  in freshly generated C (a stale `main-windows.c` artifact from Nov 2
  still shows one; it predates the fix and regenerates clean).
- **Dict formatting hazards** (same bug class, in the f-string pipeline):
  `format_dict_value` and `format_hashmap_value` no longer return a
  shared static 256-byte buffer. Numeric values are vsnprintf-sized into
  frame-arena memory (transient/borrowed — consume immediately, never
  store); strings return the entry pointer directly. This fixes two live
  bugs, both reproduced before the fix: (1) two dict values formatted in
  one expression aliased (`print|a<"x">, b<"y">|` printed `20 20` /
  `beta beta`; now `10 20` / `alpha beta`), (2) >255-char values no
  longer truncate.
- **`allocated` counter removed**: `live` is now the single counter
  (incremental high-water unchanged).
- **Side table**: TLS-qualified head, new
  `ahoy_arena_array_unregister` (emitted from the arena free path so a
  freed shell can't resolve to a dead arena), and
  `ahoy_arena_arrays_drop_arena` called from `ahoy_arena_destroy`
  (previously every entry leaked). Verified zero lost under
  `-arena-routing=all`. Rewind-time staleness remains (see below).
- **Tests**: new `fstring_legacy_test.go` (legacy path emits the sized
  helper, never a static buffer; plain literals stay literal; dict
  formatting has no static buffer and routes through the arena).
  Pre-existing quirk noted while verifying: dict access with a *variable*
  key on a mixed-type dict formats as float (`f"{d1<k>}"` prints pointer
  bits as `%f`) — identical before/after (confirmed against a pristine
  HEAD binary), so it is an inference/dispatch issue, not a regression.

---

> The user referred to `wetermark_plan.md`; the actual file is `MEMORY_WATERMARK_PLAN.md`.

---

## 1. Executive summary

### Verdict

- The **watermark runtime** (mark/rewind, incremental high-water, generation checks) is implemented and works for the cases it actually routes.
- The **ownership-class/promotion layer** the plan calls “the project” is **incomplete**.
- The plan’s own progress note (“all phases substantially implemented”) is **optimistic**. Phases 4–6 are partial, and phase 5 is mostly not implemented.
- The arena is **not** the source of the steady RSS growth you are seeing. Arena high-water is flat (16 bytes over 100,000 iterations; 112 bytes in the editor; 0 in the game). The growth is **heap (`malloc`/`strdup`) leaks** from compiler ownership gaps.
- Two editor-side bugs (per-frame scene serialization and missing `scene_free`) account for most of the editor’s ~1 MB / 3 s. The game’s ~8 MB / 4 s comes from per-frame string/float-element leaks that scale with frame rate and node count.

### Key measured facts

| Probe | Result |
|---|---|
| `arena_probe` 100k f-string loop | frame high-water **16 bytes** |
| `-arena-marks=false` (same probe) | frame high-water **1,600,000 bytes** (unbounded) |
| Editor 40-frame shot (`valgrind`) | 298,058 B definite / 20,021 blocks + 232,361 B indirect / 1,212 blocks |
| Arena game 90-frame shot (`valgrind`) | 493,098 B definite / 44,104 blocks + 226,545 B indirect / 1,047 blocks |
| Editor 40-frame shot (`ArenaDebug`) | frame high-water **112 bytes**, scene **0** |
| Arena game 90-frame shot (`ArenaDebug`) | frame high-water **0 bytes**, scene **0** |

Per-frame valgrind stacks: `scene_kind_word <- tree_panel` (3,240 blocks), `kind_subicon_name <- tree_panel` (3,240), `node_anchor_world_canvas` (13,878 blocks of 8 bytes = boxed doubles), `assets_model_name`, `text_ellipsis`, `status_get`.

### Quick symptom mapping

- **Editor idle ~1 MB / 3 s:** per-frame tree/status/float string leaks + array element stores.
- **Editor while a scene plays (~1 MB / 3 s extra):** `live_text: scene_serialize` runs every frame and is leaked when unchanged (12 KB / frame).
- **Game ~8 MB / 4 s:** per-frame string-return declarations, reassignments, float boxes, and inline owned-call results; scales with FPS and scene size.
- **“Memory isn’t freed when closing the scene”:** editor keeps leaked heap strings, and `editor_open_scene` never calls `scene_free`; glibc also retains freed pages.

---

## 2. How I verified

### Build compiler

```bash
cd ahoy/source
go build -o /tmp/opencode/ahoy-bin .
```

### Run leak probes

```bash
cd /home/lee/Documents/ahoy-lang/bell
# editor, 40 frames, valgrind
timeout 2400 xvfb-run -a valgrind --leak-check=full --show-leak-kinds=definite,indirect \
  --error-exitcode=99 --log-file=/tmp/opencode/editor_valgrind.log ./output/main
# game, 90 frames, valgrind
timeout 3000 xvfb-run -a valgrind --leak-check=full --show-leak-kinds=definite,indirect \
  --error-exitcode=99 --log-file=/tmp/opencode/arena_valgrind.log ./output/main
```

### High-water probes

```bash
cd /tmp/opencode
/tmp/opencode/ahoy-bin -f arena_probe.ahoy -r -show_leaks
/tmp/opencode/ahoy-bin -f arena_probe.ahoy -r -show_leaks -arena-marks=false
```

### Minimal repros

All repros are listed in **Appendix A** with exact file contents and observed results.

### Important tooling caveat

- `-show_leaks` prefers tcc, which does **not** support `-fsanitize=address`. ASan only runs with `-release -show-leaks` or if tcc is unavailable.
- `-release -show-leaks` with multi-TU fails to compile because generated code dereferences opaque `AhoyArena*` (`->highWater`) in a non-host TU.
- `tools/mem_check.sh` and `tools/bell_mem_check.sh` both compile **without** `-show-leaks` (so `AHOY_ARENA_STRICT` is off) and `bell_mem_check.sh` does **not** fail on leaks.
- `go test ./...` passes except the 3 pre-existing formatter failures, exactly as the plan predicted.

---

## 3. Phase-by-phase compliance

| Phase | Plan says | Actual code | Verdict |
|---|---|---|---|
| 0 | flags inert | F-strings route even with `-arena-routing=off`; `-arena-marks=false` leaks unbounded; `-fstring-legacy` does not restore static buffers | ⚠️ partial |
| 1 | runtime + incremental live/highWater + gen + realloc + strict NULL | Implemented. `allocated` remains; 16-byte size alignment but 24-byte header so `data` is 8 mod 16; no runtime tests | ⚠️ mostly |
| 2 | marks with routing off | Works; marks at every scope (not just function/loop/switch); `__ahoy_marks[64]` has no depth guard; duplicate rewinds; no overhead measurement | ⚠️ mostly |
| 3 | f-string/concat temps only | Static buffer, `%`, long strings fixed. Bound f-strings still `strdup`’d; `returnTypeCanHoldString()` over-routes; tests missing | ⚠️ partial |
| 4 | named locals, returns, switch, defer | Reassignment leaks; user-call string returns untracked; return expr after rewind; switch path; multi-return state leak | ❌ partial |
| 4b | owned inline temps | Only top-level call args; declarations and nested expression results leak | ❌ partial |
| 5 | containers | `arrayElemClass` written but never read; element overwrite leaks; float boxes leak; arena arrays crash on `push`/`fill`; side table never pruned | ❌ mostly not done |
| 6 | scene/threads/jobs/FFI/hot reload | Scene swap dead (nothing allocates from scene arena); `scene_free` shallow; editor bypasses it; TLS empty under tcc; FFI doc claims false; hot reload duplicates runtime | ⚠️ partial |

---

## 4. Detailed findings

### P0 — these directly cause the growth

#### P0-1. User-function string returns are never tracked at the caller

**Files:** `ahoy/source/methods.go:55` (`valueAllocatesString`), `ahoy/source/codegen.go:4450` (declaration tracking), `ahoy/source/codegen.go:6659` (`isOwnedStringReturningCall`).

The v1 return contract makes every returned string `Owned` (often via `strdup`). But caller-side tracking only considers `valueAllocatesString`, `NODE_F_STRING`, and `ownedReadNeedsDup`. `valueAllocatesString` returns false for user calls; `isHeapAllocatedType` returns false for `"string"`.

Result:

```ahoy
s: make||            ? implicit type
s: string = make||   ? explicit type
```

both generate `char* s = make();` with **no `free(s)`** at scope exit. This is why `scene_kind_word`, `kind_subicon_name`, `assets_model_name`, `status_get`, and `scene_serialize` results leak.

**Repro:** Appendix A, Program 1. Observed: 3 bytes leaked per call.

**Fix:** include `gen.isOwnedStringReturningCall(valueNode)` in the registration conditions at `codegen.go:4450` and `4092`, and in `valueAllocatesString`. Exclude job-task functions that intentionally return without a copy (`isJobTaskParam`).

#### P0-2. Reassignment drops tracking for fresh strings

**File:** `ahoy/source/codegen.go:4092-4128`.

The fresh-string reassign branch tracks the new value, then unconditionally:

```go
delete(gen.heapAllocatedVars, varName)
delete(gen.heapVarTypes, varName)
delete(gen.heapVarScopes, varName)
```

So every `s = ...` with a fresh RHS leaks. The comment says “Reassigned to a static value”, but this branch is the fresh-value branch.

**Repro:** Appendix A, Program 2. Observed: `s = "a" + "b"` → 3,000 B / 1,000 blocks; `s = f"v-{i}"` → 5,890 B / 1,000 blocks.

**Fix:** free the old value when safe, keep/register the new one at the declaration depth, and only remove tracking for genuinely static/borrowed RHS values. If dominance cannot be proven, use an ownership flag or accept a documented bounded leak. Do not delete tracking for fresh assignments.

Also: set `varAllocClass` consistently with actual emission. Today it sets `AllocArena` for a reassign f-string while the emitted value is `strdup(...)` (Owned).

#### P0-3. Array element stores never free the old value and do not establish ownership

**Files:** `ahoy/source/codegen.go:3350-3523` (element store), `4379-4389` (literal ownership), `6609-6624` (`setArrayStringOwnership`), `7757-7770` (free path).

`arr[i] = value` for `array[string]` duplicates the RHS in some cases but:

- never frees the element being overwritten,
- never promotes the array to an owning array,
- never deep-frees literal arrays at scope exit.

**Repro:** Appendix A, Program 3. Observed: 3,000 B / 1,000 blocks.

This is also why `status_set` leaks: it does `status[0] = text` with `strdup(text)` and never frees the previous status; the caller’s `status: [""]` array is never deep-freed.

**Fix:** implement the plan’s “containers have one element class”. For typed `array[string]`, make the array own all elements by construction, free the old element before overwrite, and deep-free at scope exit. Alternatively, add per-slot ownership and free only owned slots. `arrayElemClass` exists but is never read; wire it into `generateFreeCodeForVar`, assignment, push, and call-site promotion.

#### P0-4. Literal stores double-copy and don’t free the ephemeral source

- `array[string] = [f"a-{i}"]` → elements are `ahoy_owned_sprintf(...)`, but the array is freed shell-only → **11,780 B / 2,000 blocks** lost for 1,000 iterations.
- `dict = <"k": f"v-{i}">` → dict put duplicates the owned f-string; one copy leaks → **5,890 B / 1,000 blocks** lost.
- `Card{text: f"v-{i}"}` → generates `strdup(ahoy_owned_sprintf(...))` and the struct field is not tracked, so both allocations leak → **11,780 B / 1,000 blocks** lost.

**Files:** `codegen.go:18663-18685` (object literal field store), `codegen.go:6801-6858` (`prepareOwnedStructLocals`), `codegen.go:4325-4333` (bound f-string strdup).

`prepareOwnedStructLocals` only tracks fields that are assigned *later* (`stores` scan), not string fields present in the literal. A `Card{text: f"..."}` with no later `c.text = ...` is never registered.

**Fix:** use `emitStringCopy` (which frees ephemeral sources) instead of raw `strdup` for owned f-string/call/concat sources in object/dict/array literal stores, or transfer ownership. Include literal-present string fields in `ownedStructDecls`.

#### P0-5. Boxed float array elements are never freed

Floats are stored as `malloc(sizeof(double))` boxes (`codegen.go:11580-11594` and in `push`), but array teardown only frees float boxes when `heapArrayOwnsStrings` is set (`codegen.go:7757`). Plain `array[float]` frees only `data`/`types`/shell.

**Repro:** Appendix A, Program 4. Observed: 24,000 B / 3,000 boxes lost for 1,000 iterations of `[1.5, 2.5, 3.5]`, with or without arena routing.

This is the `node_anchor_world_canvas` leak in the game: `sfx`/`sfy` arrays leak every boxed double every call.

**Fix:** free `AHOY_TYPE_FLOAT` elements in owning-array teardown, exactly as `freeHashMap` already does for dict values. For arena arrays, either arena-allocate the double boxes or exclude `float` from `arrayLiteralAllPrimitive` and keep the array `Owned`.

#### P0-6. Arena arrays are broken under `-arena-routing=all`

- `array[int].push|i|` generates a string copy: `ahoy_arena_sprintf(..., "%s", i)` → **SIGSEGV at address 0x1**. `isOwnedStringExpr` returns true for any identifier that isn’t arena/static, without checking `isStringLikeType(inferType(arg))` (`codegen.go:6905-6936`, used at `10821-10828`).
- `array[int].fill|7,5|` calls libc `realloc` on arena memory → **`realloc(): invalid pointer` abort**. `fill` was not made arena-aware (`stdlib.go` / `ahoy_array_fill`).
- The side table `__ahoy_arena_arrays` mallocs an entry per arena array and **never removes or frees entries** (`codegen.go:1169-1193`). Under routing this grows forever and makes every push O(entries). Valgrind reports these as “still reachable”, which is why the leak harness misses them.
- The table is a plain global, not TLS, and is never cleared on `ahoy_arena_destroy`.

**Fix:** type-check before string promotion in push; make `fill` arena-aware or reject arena arrays; prune side-table entries on array free / arena reset / destroy; make the table TLS. Do not enable `-arena-routing=all` until these are fixed.

#### P0-7. Editor-side leaks

**Files:** `bell/editor/main.ahoy:11912-11928`, `bell/editor/main.ahoy:18766-18780`.

1. `editor_open_scene` pops nodes without calling `scene_free`. Every scene open leaks the previous scene’s component structs, node names/text/groups/scripts, and component internals.
2. `live_text: scene_serialize|nodes, a|` runs every frame while a play is active, but:
   - the comment says “Throttled to every 15 frames”, but there is **no throttle**,
   - `scene_serialize` returns a fresh owned string,
   - when the text is unchanged, `live_text` is never stored and never freed,
   - when it changes, `play_last[0] = live_text` generates `strdup(live_text)`, leaking the original too.

The generated code contains **no `free(live_text)`** at all. `.bell_play_current.scene` is ~12 KB, so at 60 fps this alone is ~0.7 MB/s.

**Fixes:**
- call `scene_free|nodes|` before popping in `editor_open_scene`;
- restore the 15-frame throttle via `play_tick`, and explicitly `free|live_text|` when it is not transferred (or restructure ownership).

---

### P1 — correctness / latent UAFs

#### P1-1. Return expressions are evaluated after the function rewind

`generateReturnStatement` emits `ahoy_arena_rewind(...[0])` and then generates the return expression unless `needTemp`. The plan says: “If any name mentioned in the return expression has `varAllocClass == AllocArena`, force the existing `needTemp` path.” That check is not implemented; `needTemp` only considers `returnWouldFreeMentioned` (heap-tracked locals).

Today this is mostly masked because bound f-strings are always `strdup`’d (Owned), but it becomes a real UAF as soon as §6.3 is implemented and bound locals are actually routed. The switch-return path also rewinds before generating the switch, and multi-value returns only disable routing when `arenaRouting` is already on.

#### P1-2. Multi-return permanently disables arena routing

`codegen.go:3187-3189` sets `gen.arenaRouting = false` for a multi-return function and never restores it. After the first such function, all later functions in the compilation lose arena routing. The plan explicitly says “restore after”.

#### P1-3. Arena arrays returned from functions are not deep-promoted

The return branch (`codegen.go:6479-6488`) clones shell/data pointers into a heap array, but does not promote string elements. If an arena array holds arena strings, the returned array points into the function’s rewound arena. Latent today because array routing is off by default.

#### P1-4. tcc jobs share one arena pointer

`AHOY_TLS` is empty under `__TINYC__` (`codegen.go:580-585`). The job worker does:

```c
__ahoy_frame_arena = ahoy_arena_new(1 << 16);
...
ahoy_arena_destroy(__ahoy_frame_arena);
__ahoy_frame_arena = NULL;
```

With tcc this is the same global the main thread uses. A worker replaces it, main starts using the worker’s arena, then the worker destroys it and sets it to NULL. After that, all routed f-strings fall back to `malloc` and are never rewound. This affects demos/games using `jobs|N|` and async asset loading.

Fix: use `pthread_getspecific`/`pthread_setspecific` for the arena pointer, or refuse thread pools in tcc builds. Do not silently drop TLS.

#### P1-5. Hot reload still duplicates the arena runtime

Generated code is guarded by `#ifndef AHOY_HOT_RELOAD_MODULE` (`codegen.go:645-650`), but the hot-reload compile commands never define `AHOY_HOT_RELOAD_MODULE` (`main.go:2427, 2441`), and the generated host does not define the arena runtime. So the reloaded `.so` defines its own `__ahoy_frame_arena`/side table and creates new arenas on every reload; the old arenas and any live data are lost.

The FFI_RETAIN_LIST claim that “modules reference host globals via externs” is not true.

#### P1-6. `-show_leaks` multi-TU gcc build is broken

`ArenaDebug` emits `__ahoy_frame_arena->highWater` in the generated Ahoy TU (`codegen.go:3244`), but `main.h` only forward-declares `AhoyArena`. Multi-TU gcc fails with “invalid use of incomplete typedef ‘AhoyArena’”. That means the plan’s ASan + high-water verification cannot run in the normal multi-TU configuration. Add `size_t ahoy_arena_high_water(AhoyArena*)` in the host runtime and call that.

### P2 — runtime hardening and verification

- `ahoy_arena_rewind` only checks `m.gen` under `AHOY_ARENA_STRICT`; the plan says release should also return on a stale generation. Currently release proceeds and can restore a stale `used`/`live`.
- `AhoyArenaBlock` is 24 bytes of header before `data`, so `data` is not 16-byte aligned even though `AHOY_ARENA_ALIGN` is 16. Pad the header (or use `_Alignas(16)`).
- `ahoy_arena_realloc` adds the new size to `live` on a move and never subtracts the abandoned old size; `live`/`highWater` overcount.
- `ahoy_arena_alloc` does not check overflow of `sizeof(AhoyArenaBlock) + cap`.
- `__ahoy_marks[64]` has no compile-time depth check. The plan says abort codegen above the cap; currently a deeply nested program silently writes out of bounds. Bell’s max is 10, but the guard is missing.
- `allocated` still exists alongside `live`; the plan says migrate/remove it.
- `AHOY_ARENA_STRICT` is not enabled by `tools/mem_check.sh` or `tools/bell_mem_check.sh`. Those builds run without strict NULL checks or poison, so the tests do not exercise the failure modes the plan describes.
- `tools/bell_mem_check.sh` explicitly does not fail on leaks, and its comment “no site scales with frame count” is disproved by the valgrind runs above.
- The required leak tests (`leak_loop_fstring`, `leak_loop_concat`, `fstring_store/percent/long`, `leak_early_return/switch/defer`, reassignment, float array, arena array push/fill, scene switch) are missing.
- `FFI_RETAIN_LIST.md` claims C string args are promoted in `generateMaybeCharArg`, but that function explicitly does not promote (`codegen.go:9586-9591`).
- `MEMORY_BASELINE.md` claims “no site scales with frame count”; the valgrind runs above show several per-frame scaling sites.
- The plan’s progress note marks Phase 4b “done” and Phases 5/6 “partial”, but the code shows Phase 5 mostly not implemented and Phase 6 only infra-level.

---

## 5. Suggested fix order

**Stop the bleeding first (small, high impact):**

1. Register string-returning **user calls** as owned at declaration and reassignment sites (`valueAllocatesString` / `isOwnedStringReturningCall`).
2. Remove the unconditional tracking deletes in the reassignment path; free the old value and track the new one.
3. Make typed `array[string]` own its elements; free the old element on overwrite; deep-free at scope exit.
4. Free boxed `AHOY_TYPE_FLOAT` elements in array teardown.
5. Editor quick wins:
   - call `scene_free|nodes|` before popping in `editor_open_scene`;
   - restore the 15-frame `play_tick` throttle and explicitly `free|live_text|` when it is not transferred.

Those five should remove most of the 1 MB / 3 s and a large share of the game growth.

**Then fix correctness:**

6. Fix literal double-copy (`emitStringCopy` / ownership transfer) and track literal-present struct string fields.
7. Fix arena array `push` type guard and `fill`; prune the side-table entries and make the table TLS.
8. Implement the return-before-rewind temp rule, switch-return hoisting, and restore `arenaRouting` after multi-return functions.
9. Fix tcc TLS/jobs and hot-reload host ownership.
10. Add the missing leak tests and turn on `AHOY_ARENA_STRICT` in the harnesses.

---

## 6. Do not enable `-arena-routing=all` yet

It currently:

- crashes on primitive `push` (string copy of an int),
- crashes on `fill` (`realloc` on arena memory),
- leaks side-table entries forever,
- and makes every push O(entries).

Keep the default `off` until the P0-6 and P2 fixes land.

---

## 7. Test additions needed

### Leak programs (`ahoy/test/input/leak_*.ahoy`)

- `leak_reassign_string`: `s = "a" + "b"` and `s = f"v-{i}"` in a loop.
- `leak_array_element_store`: `arr[0] = s` in a loop.
- `leak_array_literal_string`: `a = [f"a-{i}", f"b-{i}"]` in a loop.
- `leak_struct_literal_string`: `Card{text: f"v-{i}"}` in a loop.
- `leak_dict_literal_string`: `d = <"k": f"v-{i}">` in a loop.
- `leak_float_array`: `a = [1.5, 2.5, 3.5]` in a loop.
- `leak_user_call_return`: declaration from a user function returning a string, used in a loop.
- `leak_early_return`, `leak_switch`, `leak_defer`: mark unwinding.
- `leak_loop_fstring`, `leak_loop_concat`: per-iteration temps.
- `fstring_store`, `fstring_percent`, `fstring_long`: semantic bugs.

### Codegen text assertions

- marks present at function/loop/switch entry and rewound at exits;
- no `free` on Arena values;
- no static `__fstr_buf`;
- `__ahoy_marks[` count tracks scope depth;
- no `->highWater` in non-host TUs;
- side table entries pruned on reset.

### Runtime harness tests

- mark -> alloc -> alloc -> rewind -> reuse;
- mark -> reset -> rewind aborts under strict;
- realloc in place;
- poison under debug;
- 16-byte alignment;
- stale mark returns (not corrupts) in release;
- overflow checks.

### Harness changes

- `tools/mem_check.sh`: add `-show-leaks` (or define strict in generated C for all debug builds) so NULL and stale-mark paths are exercised.
- `tools/bell_mem_check.sh`: fail on leaks, not just errors; run a repeated scene-switch variant.
- Add a job-pool ASan/valgrind test under both tcc and gcc.
- Add a hot-reload smoke test that checks arena pointer stability and no growth.

---

## 8. Verification commands

```bash
# compiler tests
cd ahoy/source
go test ./...

# leak suite
cd ahoy
bash tools/mem_check.sh

# high-water (single TU)
cd /tmp/opencode
/tmp/opencode/ahoy-bin -f arena_probe.ahoy -r -show-leaks
# expected: frame high-water: 16 bytes

# valgrind (editor, 40 frames)
cd /home/lee/Documents/ahoy-lang/bell
timeout 2400 xvfb-run -a valgrind --leak-check=full --show-leak-kinds=definite,indirect \
  --error-exitcode=99 --log-file=/tmp/opencode/editor_valgrind.log ./output/main
# expected after fixes: no definite/indirect leaks
```

---

## 9. Appendix A — Minimal repro programs

### Program 1 — user call return not tracked

```ahoy
program p

@ make || string:
    return "x" + "y"
$

@ main ||:
    s: make||
    print|s|
$
```

Observed: 3 bytes leaked per call.

### Program 2 — reassignment leak

```ahoy
program p

@ main ||:
    s: string = "x"
    loop i:0 to 1000 do
        s = "a" + "b"
    $
    print|s|
$
```

Observed: 3,000 bytes / 1,000 blocks definitely lost.

### Program 3 — array element store leak

```ahoy
program p

@ main ||:
    arr: array[string] = [""]
    loop i:0 to 1000 do
        s: "v" + "w"
        arr[0] = s
    $
    print|arr[0]|
$
```

Observed: 3,000 bytes / 1,000 blocks definitely lost.

### Program 4 — float box leak

```ahoy
program p

@ main ||:
    total: float = 0.0
    loop i:0 to 1000 do
        a: array[float] = [1.5, 2.5, 3.5]
        total = total + a[0]
    $
    print|"total=", total|
$
```

Observed: 24,000 bytes / 3,000 boxes lost, with or without arena routing.

### Program 5 — arena array primitive push crash

```ahoy
program p

@ main ||:
    total: int = 0
    loop i:0 to 1000 do
        a: array[int] = [1, 2, 3]
        a.push|i|
        total = total + a.length
    $
    print|"total=", total|
$
```

Run with `-arena-routing=all`: SIGSEGV at `ahoy_arena_sprintf(..., "%s", i)`.

### Program 6 — arena fill crash

```ahoy
program p

@ main ||:
    a: array[int] = []
    a.fill|7, 5|
    print|a.length|
$
```

Run with `-arena-routing=all`: `realloc(): invalid pointer` abort.

### Program 7 — marks off unbounded high-water

```ahoy
program p

@ work |n:int| int:
    total: 0
    loop i:0 to n do
        s: string = f"entity-{i}"
        total = total + s.byte_len||
    $
    return total
$

@ main ||:
    t: int = work|100000|
    print|"total=", t|
$
```

Run with `-arena-marks=false`: frame high-water 1,600,000 bytes.

### Program 8 — struct literal double-copy

```ahoy
program p

struct Card:
    text: string = ""
$

@ main ||:
    total: int = 0
    loop i:0 to 1000 do
        c: Card{text: f"v-{i}"}
        total = total + c.text.byte_len||
    $
    print|"total=", total|
$
```

Observed: 11,780 bytes / 2,000 blocks lost.

### Program 9 — dict literal f-string

```ahoy
program p

@ main ||:
    total: int = 0
    loop i:0 to 1000 do
        d: dict = <"k": f"v-{i}">
        total = total + d.size||
    $
    print|"total=", total|
$
```

Observed: 5,890 bytes / 1,000 blocks lost.

### Program 10 — array literal f-string

```ahoy
program p

@ main ||:
    total: int = 0
    loop i:0 to 1000 do
        a: array[string] = [f"a-{i}", f"b-{i}"]
        total = total + a.length
    $
    print|"total=", total|
$
```

Observed: 11,780 bytes / 2,000 blocks lost.

---

## 10. Appendix B — key code references

- Plan: `ahoy/docs/MEMORY_WATERMARK_PLAN.md`
- Runtime: `ahoy/source/codegen.go:1022-1235`
- `getArenaImplementation`: `ahoy/source/codegen.go:1022`
- `enterScope`: `ahoy/source/codegen.go:7259`
- `exitScope` rewind: `ahoy/source/codegen.go:7627-7632`
- Early exit rewind: `ahoy/source/codegen.go:7445-7448`
- Return rewind: `ahoy/source/codegen.go:6334-6340`
- Function entry/teardown: `ahoy/source/codegen.go:3186-3236`
- Multi-return disable: `ahoy/source/codegen.go:3187-3189`
- Bound f-string strdup: `ahoy/source/codegen.go:4325-4333`, `3986-3993`
- F-string class tracking: `ahoy/source/codegen.go:4115-4122`
- Reassign tracking/delete: `ahoy/source/codegen.go:4092-4129`
- Declaration tracking: `ahoy/source/codegen.go:4448-4460`
- `isHeapAllocatedType`: `ahoy/source/codegen.go:2432-2455`
- `isOwnedStringReturningCall`: `ahoy/source/codegen.go:6659-6688`
- `valueAllocatesString`: `ahoy/source/methods.go:55-100`
- `isOwnedStringExpr`: `ahoy/source/codegen.go:6905-6936`
- Array element store: `ahoy/source/codegen.go:3350-3523`
- Array literal ownership: `ahoy/source/codegen.go:4379-4389`
- `setArrayStringOwnership`: `ahoy/source/codegen.go:6609-6624`
- Array free path: `ahoy/source/codegen.go:7757-7770`
- Float boxing in literals: `ahoy/source/codegen.go:11580-11594`
- `arrayLiteralAllPrimitive`: `ahoy/source/codegen.go:11424-11438`
- Arena array push: `ahoy/source/codegen.go:10821-10828`
- Arena side table: `ahoy/source/codegen.go:1169-1193`
- Side table register: `ahoy/source/codegen.go:11685`
- TLS: `ahoy/source/codegen.go:580-585`
- Job worker: `ahoy/source/codegen.go:19588-19620`
- Scene arena swap: `ahoy/source/codegen.go:1162-1167`
- Arena creation in main: `ahoy/source/codegen.go:3177-3184`
- High-water print: `ahoy/source/codegen.go:3244`
- Hot-reload guard: `ahoy/source/codegen.go:645-650`
- `FFI_RETAIN_LIST.md`: `ahoy/docs/FFI_RETAIN_LIST.md`
- `generateMaybeCharArg`: `ahoy/source/codegen.go:9578-9592`
- `prepareOwnedStructLocals`: `ahoy/source/codegen.go:6801-6858`
- `registerOwnedStructVar`: `ahoy/source/codegen.go:6861-6876`
- Object literal field store: `ahoy/source/codegen.go:18660-18690`
- `fstring_children.go` helper: `ahoy/source/fstring_children.go:67-74`
- Scene free: `bell/engine/core.ahoy:2399-2404`
- node_components_free: `bell/engine/core.ahoy:2383-2395`
- game_switch_scene: `bell/engine/scene_file.ahoy:5128-5145`
- editor_open_scene: `bell/editor/main.ahoy:11912-11937`
- live sync: `bell/editor/main.ahoy:18766-18780`
- play_tick: `bell/editor/main.ahoy:13062`

---

## 11. Bottom line

The watermark runtime is working and high-water is flat. The steady RSS growth is not the arena; it is heap ownership gaps that the plan warned would be the hard part. The most impactful fixes are caller tracking for user-function string returns, reassignment tracking, container element ownership, float element freeing, and the editor’s live-sync throttle. Those five should remove most of the observed growth. Phases 4–6 should be treated as partial, not substantially done, and `-arena-routing=all` should not be enabled until the side-table/push/fill issues are fixed.

---

## 13. Supplement (2026-11-07): what §12 and `MEMORY_HANDOFF.md` missed

`MEMORY_HANDOFF.md` (2026-11-06) is the newer document: it records what
actually landed in `ahoy@bc420a6` / `bell@47402a5` and the two-crash
invariants. This section back-ports what this review never covered (or
left stale), so a reader of this file alone does not work from obsolete
guidance. All line numbers below are post-`bc420a6` unless noted.

### 13.1 Landed work this review does not describe

- **Return-ownership predicate** (`methods.go:63-180`): `freshStringBuiltin`
  (12 audited C helpers), `funcReturnsOwnedString` (mirrors return-site
  promotion per expression shape — owned: literals, identifiers,
  container accesses, concats, slices, owned f-strings, known-fresh
  calls; borrowed: C/foreign calls, ternaries, switch-expressions),
  `stringReturnYieldsOwned` (`:147`), wired into `valueAllocatesString`
  and `isOwnedStringReturningCall`. This is stricter than the §5 fix
  sketch ("include user calls in registration conditions") — wrappers
  like `loc_tr` (borrowed `ahoy_loc_tr`) and static switch-returns like
  `kind_icon_name` are deliberately untracked. Treating every
  string-returning call as owned would reintroduce the arena-game UAF.
- **Reassignment dominance rule** (`codegen.go:4277-4345`): a conditional
  fresh reassignment over a *borrowed* binding skips scope-exit
  registration (bounded leak, never UAF); straight-line and
  already-tracked cases still register. **Dup-on-tracked-static**
  (same branch): conditional `x = "lit"` over an *owned* binding emits
  `strdup` and stays tracked (fixed the 3,080-object `eye_name` /
  `coll_label` per-frame leak). §5 item 2 ("free the old, track the
  new") predates both rules and would double-free / free statics if
  applied literally today — follow the handoff §2 invariants instead.
- **Explicit disposal** (`free|s|` in `generateCall`): emits
  `if (s) { free(s); s = 0; }` + untracks + prunes; struct-pool and
  untracked-pointer behavior unchanged. This review has no section for
  explicit free; the handoff §1 does.
- **Struct own-by-construction** (`fieldStoreNeedsDup` at `:6937` via
  `slotStoreNeedsDup` at `:7291`, so element and dict sites come along):
  string literals into struct fields are now dup'd; all struct string
  fields are heap-or-null. The §5 item-6 sketch ("use emitStringCopy")
  landed in this form.
- **Bell fixes absent from §5:** `node_strings_free` (23 frees) +
  deep `scene_free` (`core.ahoy:2401/2429`), `free|live_text|` live-sync
  disposal (`editor/main.ahoy:18753`) with the 15-frame `play_tick`
  throttle restored, pose-memo owns-its-path (`glue.h: puts strdup on
  put, frees on overwrite` — was retaining Ahoy temporaries), and
  `read_file_owned` (`core.ahoy:5803`, 9 `LoadFileText` sites converted,
  ~12 KB/open raylib leak each). The §5 item-5 sketch is done; the LSan
  stacks that drove it (`tree_panel:3162`, 3,080 objects) are in the
  handoff, not here.
- **`-o NAME` flag** (`main.go:1171/1681-1705`): overrides the output
  executable name (exe + primary C rename; TU files stay `main_*`).
  `run.sh` passes `-o bell` / `-o <game>`; play-launches pass
  `-o <scene-stem>` / `-o <game>`; `bell-sea` forwards `-o` (implemented
  but uncommitted — see handoff §G). This review never mentions `-o`;
  the review's build commands (`output/main`) are stale for Bell runs.
- **Invariants (§2 of the handoff) have no counterpart here** and
  override several §5 sketches: (1) tracking must match the runtime
  binding on every path — lean to leak; (2) never gate the eager
  `autoFreedVars` mark on runtime conditions (a same-day double-free
  regression was reverted); (3) scope-exit cleanups never clear
  `heapAllocatedVars/Types/Scopes`; (4) the predicate is the truth, not
  the return contract — every new return shape updates BOTH promotion
  and `stringReturnYieldsOwned`; (5) callees that retain must own
  (`strdup` internally or `FFI_RETAIN_LIST.md` + call-site promotion);
  (6) exit-time totals ≠ per-frame slope (massif/LSan, not valgrind
  summaries). Read these before touching ownership.

### 13.2 Still true, still missing: write-only `arrayElemClass`

§4 P0-3 noted "`arrayElemClass` written but never read". Post-`bc420a6`
it is still write-only (8 writes at `codegen.go:3297/4180/4635/4645/
4719/6914/6921/7456`, zero reads; the live signal is
`heapArrayOwnsStrings`). The handoff continuation (§5.3) recommends
deleting it or wiring it as part of deep-teardown work — do not add a
second map alongside it.

### 13.3 Newly found gaps (neither this review nor the handoff covered)

Found by grepping every `scene_serialize` caller in `bell/editor/main.ahoy`
against its matching `free|` (handoff audited only the live-sync and
open paths). All follow the owned-return shape with no `free|`:

- `history_panel :696-701` — `live_text` + `top_text` +
  `live_name` per frame the panel draws (same ~12 KB magnitude as the
  fixed live-sync drip; fires through the panel family that topped LSan
  before). Per-frame; fix first.
- Play-start one-shots `:13690-13691` / `:13777-13778` —
  `play_last_tmp` / `gplay_last_tmp` stored into owning `play_last`
  (store dups; original never freed). One serialize per play press.
- Undo/redo/goto `:597-598` / `:616-620` / `:636-637` plus
  `editor_snapshot :548-559` — `current`/`snap` (serialize) and
  `cur_entry`/`entry` (`history_entry_make` concat at `:302-305`)
  duplicated into history arrays by `push`, originals never freed. Per
  user action; fix with `free|current|` + `free|cur_entry|` after each
  push.
- History rename `:553-556` — `old_text`/`act`/renamed entry, same class.

Recipe gap: load/open/close-only valgrind runs never top these; extend
the interactive recipe (§D of the handoff) with edit → undo → redo →
goto-history-row → start/stop play before reading LSan.

### 13.4 Stale in this review after `bc420a6`

- §12 "Deferred" subsection (bell_mem_check still errors-only, `scene_free`
  shallow, per-open leaks bounded): partially landed — `scene_free` is
  now deep, live-sync is throttled + disposed, 9 file-read sites are
  owned. `BELL_MEM_STRICT=1` is still red (exit-time teardown §C of the
  handoff), but the listed reasons are out of date.
- §8 verification commands: single-TU tcc probes and `output/main`
  paths predate `-o` renaming (`bell/output/<name>`) and multi-TU gcc
  ASan (`-release -show_leaks` + `ahoy_arena_high_water`, which fixed
  the old `->highWater` multi-TU break at P1-6). Use the handoff §4
  playbook instead.
- Appendix B line numbers drifted: `slotStoreNeedsDup` → `:7291`,
  `pushNeedsStrdup` → `:7392`, `lexicalDeclDepth` → `:7538`,
  `generateFreeCodeForVar` → `:8012` (struct branch `:8100-8110`,
  owning-string branch `:8111-8119`, float-shell branch `:8120-8129`),
  `fieldStoreNeedsDup` → `:6937`, `-o` → `main.go:1171/1681-1705`.
- Handoff correction (propagated here): the "three identical binaries"
  hash line is wrong — `ahoy/ahoy-bin` (`55cf5471…`, the copy
  `run.sh` uses) differs from `ahoy/test/ahoy-bin` and
  `~/.local/bin/ahoy-bin` (`3c5528a9…`). Rebuild + re-hash per handoff
  §5.1 before trusting binary state.
- `MEMORY_BASELINE.md` "no site scales with frame count" remains
  disproved (valgrind stacks in §1); like `FFI_RETAIN_LIST.md`'s old
  "C args promoted" claim (now corrected to borrowed-default), treat
  baseline prose as historical, not current.

---

## 14. Updated verification (what to run now)

Supersedes §8 for Bell runs; compiler-unit rows are unchanged
(`go test` → only the 3 pre-existing formatter failures).

```bash
# leak suite (strict+poison, must PASS)
cd ahoy && bash tools/mem_check.sh 2>&1 | tail -n 3

# rebuild + sync all three binaries (always after touching ahoy/source)
cd ahoy/source && go build -o ../ahoy-bin . \
  && cp ../ahoy-bin ../test/ahoy-bin \
  && cp ../ahoy-bin /home/lee/.local/bin/ahoy-bin

# headless functional runs (must close cleanly, no crash report)
cd bell && timeout 120 ./run.sh shot editor 2>&1 | tail -n 3
cd bell && timeout 120 ./run.sh shot games/arena 2>&1 | tail -n 3

# sanitizer run with stacks (multi-TU gcc; binary lands in bell/output/bell)
cd bell && ../ahoy/ahoy-bin -f editor/main.ahoy -r -release -show_leaks \
  -D BACKEND_RAYLIB -D DEMO_SHOT > /tmp/opencode/asan.log 2>&1
grep -E "ERROR|SUMMARY|closed after" /tmp/opencode/asan.log
```

Debts rule (from the handoff, repeated because it bit twice): a
privately-passing `go test` is not sufficient — the eye/gate regressions
passed unit tests and died only in shots/ASan. Re-run the full playbook
before ANY ownership change.