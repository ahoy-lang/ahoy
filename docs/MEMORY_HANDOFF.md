# Memory Work — Agent Handoff

**Date:** 2026-11-06 (follow-up to `MEMORY_WATERMARK_IMPLEMENTATION_REVIEW.md`,
which covers the earlier watermark phase — read that first for P0/P1/P2 context).
**Commits:** `ahoy@bc420a6`, `bell@47402a5` (both trees clean at handoff).
**Binaries:** all three `ahoy-bin` copies identical (`3c5528a9…`):
`ahoy/ahoy-bin` (used by `bell/run.sh`), `ahoy/test/ahoy-bin`,
`~/.local/bin/ahoy-bin` (command line).
**Outputs renamed:** editor builds `bell/output/bell`, games build
`bell/output/<game>` (`arena`, …) instead of `output/main`.
**Measured state:** editor idle 0.7 → **0.2 MB / 4 s** (System Monitor);
exit-time valgrind 307K → 236K definite (−23%); ASan zero errors on full
editor + arena runs; `tools/mem_check.sh` PASS; `go test` clean except 3
pre-existing formatter failures.

---

## 1. What landed (do not regress)

### Compiler (`ahoy/source/`)
- **`funcReturnsOwnedString` predicate** (`methods.go:94`, helpers at `:63`
  `freshStringBuiltin`, `:147` `stringReturnYieldsOwned`): decides caller-side
  ownership by mirroring return-site promotion per expression shape.
  Owned: literals, identifiers (dup'd or escape-transferred), container
  accesses, concats, slices, owned f-strings, known-fresh calls.
  Borrowed: C/foreign calls, ternaries, switch-expressions. Wired into
  `valueAllocatesString`, `isOwnedStringReturningCall` (covers declarations,
  reassignments, inline/print temps, ephemerality). This fixed the arena
  UAF (`loc_tr` wraps borrowed `ahoy_loc_tr`) and the `assets_model_name` /
  `status_get` declaration leaks, and un-tracked static switch returns
  (`kind_icon_name`).
- **Reassignment dominance rule** (`codegen.go` fresh-reassign branch): a
  conditional fresh reassignment over a *borrowed* binding skips scope-exit
  registration (bounded leak, never UAF). Straight-line and already-tracked
  cases still register. Fixed the editor `tree_panel label` double-free.
- **Dup-on-tracked-static rule** (same branch + emission): conditional
  `x = "lit"` over an *owned* binding emits `strdup` and stays tracked, so
  scope exit is correct on taken and not-taken paths. Fixed the 3,080-object
  `eye_name` / `coll_label` per-frame leak with zero-loss (probe: 208/208).
- **Explicit disposal** (`generateCall`, `free` case): `free|s|` on a tracked
  string emits `if (s) { free(s); s = 0; }` + untracks + prunes. Struct-pool
  and untracked-pointer behavior unchanged. Used by live-sync.
- **Struct own-by-construction**: `fieldStoreNeedsDup` (`codegen.go:6937`)
  now dups string literals (delegates via `slotStoreNeedsDup`, so element
  and dict sites come along). All struct string fields are heap-or-null.
- **12 audited-fresh C helpers** in `freshStringBuiltin`: `ahoy_text_prefix`,
  `ahoy_text_from`, `ahoy_str_append_char/backspace`, `ahoy_fs_path`,
  `ahoy_path_join`, `ahoy_lint_capture`, `ahoy_strip_comments`,
  `ahoy_rename_entry/varying`, `ahoy_strndup`, `ahoy_model_bone_name`.
  Deliberately excluded (borrowed/mixed): `ahoy_sb_text`, `ahoy_loc_tr`,
  `ahoy_fs_name`, `ahoy_read_text`, `ahoy_text_copy`, `ahoy_anim_name`,
  `ahoy_audio_key`, `ahoy_shader_error`, `ahoy_vram_row_*`,
  `ahoy_shader_uniform_*`, `ahoy_model_convert`, `ahoy_text_alloc`.
- **`-o NAME` flag** (`main.go`): overrides the output executable name
  (default: input basename). Only the exe + primary C rename; TU files stay
  `main_*`. `run.sh` passes `-o bell` / `-o <game>`; editor play-launches
  pass `-o <scene-stem>` / `-o <game>`. `bell-sea` wrapper (separate repo
  `ahoy-sea-compiler`, **uncommitted**) forwards `-o`.
- Regression tests (all passing): `reassign_dominance_test.go`,
  `owned_return_test.go`, `explicit_free_test.go`, `static_reassign_test.go`,
  plus 7 `test/input/leak_*.ahoy` programs exercised by `tools/mem_check.sh`
  (strict + poison).

### Bell (`bell/`)
- `engine/core.ahoy:2401` **`node_strings_free`** (23 frees: Node 5, Vis 5,
  Ui 2, Tile 8, Slot 1, Swarm 2) + deep `scene_free` (`:2429`, strings then
  components). Kills the ~2 MB open/close string leak.
- `editor/main.ahoy:18753` **`free|live_text|`** (live-sync discard; element
  stores dup so the original is owned on all paths).
- `engine/glue.h` **pose memo owns its path** (`strdup` on put, free on
  overwrite) — was retaining Ahoy temporaries (arena UAF).
- `engine/core.ahoy:5803` **`read_file_owned`** (dup + `UnloadFileText` at
  once); 9 bare `LoadFileText` sites converted (12 KB/open raylib leak each).
- `tools/bell_mem_check.sh`: `BELL_MEM_STRICT=1` opt-in fails on definite
  leaks (default stays errors-only; strict is red until §3 lands).

### Verified results (how)
- `shot editor` 40/40 clean, `shot games/arena` clean incl. gameplay checks.
- ASan+LSan full runs, zero errors; LSan attribution drove the eye fix
  (top per-frame entry `tree_panel:3162`, 3,080 objects, now zero occurrences).
- Standalone probes valgrind-clean (t2/t4/t8/t9/t10/t11 pattern repros).

---

## 2. Invariants (learned from two crashes — do not violate)

1. **Tracking must match the runtime binding on EVERY path, or lean to leak.**
   Unconditional scope-exit `free(V)` is sound only if V is owned on all
   paths reaching the exit. Conditional assignment + static tracking =
   use-after-free one way, leak the other. When in doubt, leak (bounded).
2. **`autoFreedVars` is codegen-time, eager-free is runtime-conditional.**
   Never gate the eager mark on runtime conditions: alias reassignments
   (`candidate = kv[1]`-class: eager frees old, new aliases its owner) rely
   on the unconditional mark for scope-exit suppression. Gating it caused a
   double-free regression (reverted same day).
3. **Scope-exit cleanups never clear `heapAllocatedVars/Types/Scopes`.**
   Only `scopeAllocations` entries, alias edges and `scopeDeclared` are
   popped. Any new per-iteration/per-block free must account for the next
   iteration seeing stale tracking (C initializers re-run per iteration;
   maps do not reset).
4. **Return promotion is incomplete — the predicate is the truth, not the
   contract.** `return <c-call>`, ternaries and switch-expressions hand out
   borrowed/static pointers. Any new return shape must update BOTH
   `generateReturnStatement` promotion branches AND `stringReturnYieldsOwned`,
   or callers will free borrowed memory.
5. **Callees that retain must own.** Any C function storing an Ahoy string
   pointer past the call must `strdup` internally (pose-memo pattern) or be
   listed in `FFI_RETAIN_LIST.md` with a call-site promotion. Array/dict
   shells, struct fields and globals all alias — freeing a local that shares
   storage with them is a double-free.
6. **Exit-time totals ≠ per-frame slope.** Exit teardown pool-releases Node
   shells without freeing strings and never frees the asset registry, so
   valgrind exit summaries are dominated by load/boot. Measure the drip with
   LSan stacks (attribution) and massif slope (rate), not exit totals.

---

## 3. Future work (ordered)

### A. Compiler deep-struct teardown — deletes `node_strings_free` (do first)
**Problem:** 23 manual frees in Bell that the compiler should own.
**Fix:** in `generateFreeCodeForVar` (`codegen.go:8012`), array branch: when
the element type is an Ahoy struct (`gen.structs[elem]`), emit a per-element
loop freeing every string-typed field (field list from the struct definition),
before freeing `data`/`types`/shell. Sound now because fields are
heap-or-null by construction (§1 struct rule).
**Why manual this batch:** implicit teardown of shared arrays is exactly the
change class that caused this week's two crashes — it needs its own verify
cycle (shots + ASan + mem_check), not a ride-along.
**Verify:** `node_strings_free` deleted from `core.ahoy`; `shot editor` +
`shot games/arena` clean; ASan zero errors; `mem_check` PASS; valgrind
exit-time drops by ~node-string volume with no new invalid accesses.
**Risks:** arrays whose struct elements hold borrowed/C strings from paths
outside Bell discipline (audit any new crash with ASan — it will name the
field); struct *values* (non-array) keep today's local tracking, untouched.

### B. Load-path parse temps (~100 KB/open, bounded, sound)
**Sites (LSan-attributed, all under `editor_open_scene ← main:13203`):**
`scene_file.ahoy:183` (34 KB split-push dups), `:1955`/`:1964`/`:3338`
(concat/strdup temps), `shaders.ahoy:171` (preload strings).
**Root cause:** `pushNeedsStrdup` (`codegen.go:7392`) duplicates owned-read
pieces even when the target array dies in the same scope as the source
(the dup is only needed when the target outlives the source). The comment
admits the trade.
**Fix options:** (1) compiler lifetime check — mark the dup unnecessary when
target scope ⊆ source scope (invasive); (2) Bell-side restructure of the
parse loops to avoid push-of-owned-read (e.g. index-fill a pre-sized array
— same dup logic applies, so likely no win); (3) accept as bounded per-open
cost. Recommended: measure open/close delta first; only pursue if material.
**Verify:** LSan load entries shrink; shots clean.

### C. Exit-time teardown (unblocks `BELL_MEM_STRICT=1`)
**Problem:** at process exit, nodes-array teardown pool-links shells without
freeing the 23 strings/element, and the asset registry never frees — the
bulk of the ~460 KB/40f exit-time total.
**Fix:** call `scene_free|nodes|` (+ asset registry free, if added) at the
tail of each `main` before teardown/return. Must be LAST: after it, nodes
hold dangling interiors. Audit each main's tail for post-teardown node use.
**Verify:** `BELL_MEM_STRICT=1 bash ahoy/tools/bell_mem_check.sh` goes green;
normal shots unaffected.

### D. Interactive-only paths (if the 0.2 MB drip persists)
Headless shots don't fire hover/drag/typing. If System Monitor still shows
growth after A–C: run the editor under ASan **interactively** (or
`valgrind --tool=massif` for byte-slope) while hovering rows, dragging
nodes and typing in fields; fix whatever tops the list. Prime suspects:
text-field buffers (`ui.ahoy:702` region), tooltip/status churn.

### E. Known bounded residuals (documented, low priority)
- Transient struct temps with fresh fields (e.g. a function-local struct
  literal whose field is dup'd but the struct value is untracked) — same
  class identifier stores already had; needs Phase-5-style struct-value
  ownership to fully close.
- Node shells (~25 KB/open): `pop` decrements length without pool release;
  needs an explicit release path if per-open accounting must be exact.
- `ahoy_model_convert` / `ahoy_text_alloc` (static on some paths):
  deliberately untracked; revisit only if callers go per-frame.

### F. Deferred (reasons unchanged)
- **P1-5 host-owned arena:** needs `-rdynamic`/linkage + reload-lifecycle
  work, unverifiable headless; reload-paced dup'd empty arenas only.
- **Side-table rewind staleness:** safe while the frame arena is the only
  allocating arena; revisit when scene routing lands.
- **`ahoy_json_stringify`** (8 KB shared static): no Bell callers; fix with
  a sized heap buffer when first used.

### G. Third-repo leftover
`ahoy-sea-compiler/bell-sea` `-o` forwarding is implemented but that repo
is **uncommitted** (out of the "both repos" scope).

---

## 4. Verification playbook (commands)

```bash
# compiler unit + regression tests (expect only the 3 pre-existing formatter fails)
cd ahoy/source && go test ./... 2>&1 | grep -E "^(--- FAIL|FAIL|ok)"

# leak suite (strict+poison, must PASS)
cd ahoy && bash tools/mem_check.sh 2>&1 | tail -n 3

# rebuild + sync all three binaries (always after touching ahoy/source)
cd ahoy/source && go build -o ../ahoy-bin . \
  && cp ../ahoy-bin ../test/ahoy-bin \
  && cp ../ahoy-bin /home/lee/.local/bin/ahoy-bin

# headless functional runs (must close cleanly, no crash report)
cd bell && timeout 120 ./run.sh shot editor 2>&1 | tail -n 3
cd bell && timeout 120 ./run.sh shot games/arena 2>&1 | tail -n 3

# sanitizer run with stacks (needs ~10 min gcc; binary lands in bell/output/main)
cd bell && ../ahoy/ahoy-bin -f editor/main.ahoy -r -release -show_leaks \
  -D BACKEND_RAYLIB -D DEMO_SHOT > /tmp/opencode/asan.log 2>&1
grep -E "ERROR|SUMMARY|closed after" /tmp/opencode/asan.log

# exit-time accounting (totals only; stripped binary, no stacks)
cd bell && valgrind --leak-check=full --show-leak-kinds=definite,indirect \
  --errors-for-leak-kinds=none ./output/main 2>&1 | grep -E "lost:|ERROR SUMMARY"

# per-frame slope (rate, immune to exit-time noise)
cd bell && valgrind --tool=massif --massif-out-file=/tmp/opencode/massif.out ./output/main \
  && ms_print /tmp/opencode/massif.out | tail -n 30
```

** debts to check before ANY ownership change:** re-run all of the above;
a privately-passing `go test` is not sufficient (the eye/gate regressions
passed unit tests and died only in shots/ASan).
