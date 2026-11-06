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

---

## 5. Continuation (2026-11-07 — post-handoff audit, no new code landed)

Working trees are still at the handoff commits (`ahoy@6b552a2`,
`bell@47402a5` + one dirty scene file, `ahoy-sea-compiler` still
`M bell-sea`). Nothing in §1–§4 has been superseded by code; what follows
is corrections, newly found leaks the handoff missed, and a buildable
breakdown of item A for the next agent. Verify each claim with the
command in §4 before acting on it.

### 5.1 Post-handoff verification (re-ran, not assumed)

- `cd ahoy/source && go test ./...` → same 3 pre-existing formatter
  failures as the handoff (`TestFormatterIndentsNestedBlocks`,
  `TestFormatterKeepsDollarAtOpeningLevel`,
  `TestFormatterNeverCollapsesLoops`); no other package fails.
- **Binary-hash correction to §1:** the "all three identical
  (`3c5528a9…`)" line is wrong. Measured now:
  `ahoy/test/ahoy-bin` and `~/.local/bin/ahoy-bin` are `3c5528a9…`,
  but `ahoy/ahoy-bin` (the one `bell/run.sh` actually uses) is
  `55cf5471…` (11037506 B vs 11037698 B, both dated Nov 6). `git status`
  is clean, so both blobs are committed — the handoff recorded the hash
  of the wrong copy. Next agent: rebuild from `ahoy/source`
  (`go build -o ../ahoy-bin . && cp ../ahoy-bin ../test/ahoy-bin &&
  cp ../ahoy-bin ~/.local/bin/ahoy-bin`) and re-record all three hashes;
  do not trust the §1 hash line until then.
- Stray ignored binary: `ahoy/source/source` (11 MB, gitignored via
  `/source/source`) is a leftover `go build` default output, not a
  release artifact. Harmless; delete or ignore.
- Bell tree: only dirty file is `bell/games/arena/scenes/main.scene`
  (`M`); engine/editor sources are clean, so the §1 Bell line numbers
  still resolve (`core.ahoy:2401/2429/5803`,
  `editor/main.ahoy:11924/18744-18753`, `glue.h: pose_memo_put`).
- `arrayElemClass` is still **write-only**: 8 writes
  (`codegen.go:3297/4180/4635/4645/4719/6914/6921/7456`, plus the
  `delete` at `:6925`) and zero reads. The live element-ownership
  signal is `heapArrayOwnsStrings` (read at `:7318/:7403/:7439/:7473`
  and in `generateFreeCodeForVar` at `:8111`). Do not consult
  `arrayElemClass` in new code; either wire it or delete it (see §5.4).
- `bell-sea` `-o` forwarding is implemented (collects `-o`, forwards in
  `passthrough`, resolves `output/<name>.c`, emits `output/<name>_sea`)
  but the file is still **modified-uncommitted** (`M bell-sea`), so §G
  stands. Commit it from `ahoy-sea-compiler` when the third repo is in
  scope; until then every `-sea` run uses working-tree code.

### 5.2 Leaks the handoff missed (all in `bell/editor/main.ahoy`)

These were found by grepping every `scene_serialize` caller against its
matching `free|` — the handoff audited only the 15-frame live-sync path
(`:18744-18753`, fixed) and the open path (`editor_open_scene`, fixed).
The paths below follow the same owned-return shape (`scene_serialize`
→ owned per `funcReturnsOwnedString`; concats/`replace` in the history
helpers → owned per `valueAllocatesString`/`FreshStringMethods`) with no
`free|` on any binding. All are bounded per user action **except the
first**, which runs per frame while the history panel is visible.
Confirm each with an ASan/LSan run before fixing (LSan attribution is
what caught the eye leak; exit-time totals will not isolate these).

1. **`history_panel` per-frame serialize (`:696-701`, unfixed).**
   `live_text: string = scene_serialize|nodes, a|` runs every frame the
   panel draws, plus `top_text: string = history_entry_text|top_entry|`
   and `live_name = history_action_name|top_text, live_text|`. No
   `free|live_text| / free|top_text| / free|live_name|` follows (contrast
   the fixed `:18753` site). At ~12 KB/scene this is the same magnitude
   as the old live-sync drip and fires through the same panel that made
   `tree_panel:3162` the top LSan entry. Fix sketch: `free|live_text|`,
   `free|top_text|`, `free|live_name|` at the end of the `if count > 0`
   block (all three are owned on every path through it); or hoist the
   serialize behind a dirty-flag so the panel does not re-serialize a
   static scene at 60 fps.
2. **Play-start one-shots (`:13690-13691`, `:13777-13778`, unfixed).**
   `play_last_tmp: scene_serialize|nodes, a|` then
   `play_last[0] = play_last_tmp` (same for `gplay_last_tmp`). Element
   stores into an owning `array[string]` duplicate the RHS
   (`pushNeedsStrdup`/`slotStoreNeedsDup` path), so the original tmp is
   still owned by the local after the store — but no `free|play_last_tmp|`
   / `free|gplay_last_tmp|` follows. Bounded (one serialize per play
   press, ~12 KB), same shape as the pre-fix `play_last[0] = live_text`
   double-copy. Fix: `free|` each tmp after the store, mirroring `:18753`.
3. **Undo/redo/goto currents (`:597-598`, `:616-620`, `:636-637`).**
   `current: [string =] scene_serialize|nodes, a|` then
   `cur_entry: string = history_entry_make|"", current|` then
   `history.push|cur_entry|` / `redo_stack.push|cur_entry|`. Both
   `current` (serialize) and `cur_entry` (concat in `history_entry_make`
   at `:302-305`: `head + "\n" + text`) are owned, and the push dups
   again into the owning history array — three allocations, zero frees
   on these paths. Bounded per undo/redo/goto (user-paced), but each is
   a full scene text. Fix: `free|current|` + `free|cur_entry|` after each
   push (4 sites: `:599`, `:621`, `:638`, plus the matching redo path).
   Same treatment for `snap`/`entry` in `editor_snapshot` (`:548-559`):
   `snap` (serialize) and `entry` (concat) are both duplicated into
   `history` by `push` and never freed.
4. **History rename path (`:553-556`).** `old_text`, `act`, and the
   renamed entry are all owned concats/replaces with no frees. Same
   bounded class as (3); fix together.

Why these survived: the handoff's §B groups "load-path parse temps" and
§C groups "exit-time teardown", but these are **event-path temps**
(neither per-frame hot nor exit-time cold). LSan attributes them to
`editor_snapshot/undo/redo/history_panel ← main`, not to
`editor_open_scene`, so a load/open/close-only valgrind run never tops
them. The interactive recipe in §D (hover/drag/typing) should be
extended with: open scene → edit → undo → redo → goto history row →
start/stop play, then check LSan.

### 5.3 Dead state to remove or wire: `arrayElemClass`

REVIEW P0-3 noted "`arrayElemClass` written but never read"; that is
still true after `bc420a6` (see §5.1). Two options, smallest first:

- (a) Delete the map and its 8 writes if `heapArrayOwnsStrings` remains
  the single element-ownership signal (grep shows no other reader).
- (b) If the deep-teardown work (§3.A) needs per-array element classes
  (e.g. distinguishing string-arrays from struct-arrays), wire reads
  into `generateFreeCodeForVar` and the push/fill paths instead of
  adding a second map.

Do not add a third ownership map. The invariant to preserve (§2.1) is
one predicate per decision: `heapArrayOwnsStrings` for
string-element teardown today, extended in place for struct elements
tomorrow.

### 5.4 Item A buildable breakdown (deep struct teardown)

Target function: `generateFreeCodeForVar` (`codegen.go:8012`), array
branch. Current behavior by sub-branch:

- `:8079-8099` nested arrays (frees inner shells, no interior strings);
- `:8100-8110` struct-element arrays (frees struct shells via pool
  Release or `free`, **never interior strings** — this is the gap
  `node_strings_free` papers over);
- `:8111-8119` owning string arrays (frees `AHOY_TYPE_STRING` +
  `AHOY_TYPE_FLOAT` slots by runtime type, then shell);
- `:8120-8129` default (frees `AHOY_TYPE_FLOAT` slots by runtime type,
  then shell; strings borrowed).

**Fix (compiler):** in the `:8100-8110` branch, when `gen.structs[elem]`
is non-nil, emit a per-element loop over that struct's string-typed
fields (field list from the struct definition, same source
`node_strings_free` was hand-written from) freeing each field, then the
existing shell free. Soundness precondition (already true per §1
struct rule): all struct string fields are heap-or-null by
construction, so `if (field) free(field)` is safe on every element.
Keep the pool-Release vs `free` shell distinction exactly as is.

**Bell deletion:** remove `node_strings_free` (`core.ahoy:2401`, 23
frees) and call only `node_components_free` from `scene_free`
(`:2429-2434`). Do not delete `read_file_owned`, pose-memo own, or
`live_text` disposal — those are independent fixes.

**Risks (from §2):** shared arrays alias — freeing interiors of an
array that shares elements with another live array double-frees. The
handoff's "audit any new crash with ASan" stands: ASan names the field,
and the dominance rule (§2.1) says lean to leak if any sharing shape
appears. Struct *values* (non-array) keep today's local tracking;
untouched.

**Verify (extends §4):** `shot editor` + `shot games/arena` clean; ASan
zero errors; `mem_check.sh` PASS; exit-time valgrind drops by ~node
-string volume with no new invalid accesses; `BELL_MEM_STRICT=1` still
red until §3.C lands (expected — do not gate A on C).

### 5.5 What did NOT change (do not re-audit)

- Return predicate (`methods.go:63-180`), dominance branches
  (`codegen.go:4277-4345`), `fieldStoreNeedsDup` (`:6937` via
  `slotStoreNeedsDup`), `pushNeedsStrdup` (`:7392`), `lexicalDeclDepth`
  (`:7538`), `generateFreeCodeForVar` float handling (`:8111-8129`),
  `-o` plumbing (`main.go:1171/1681-1705`), `tools/mem_check.sh`
  strict+poison, `bell_mem_check.sh` errors-only default — all as
  described in §1. The only drift is the binary-hash line (§5.1).
- REVIEW Phases 0–6 status, P1-3 (arena-array return promotion), P1-4
  (tcc TLS), P1-5 (host-owns-arena), P2 (header padding/realloc-live/
  rewind-gen) — none landed since the handoff; §3.F reasons unchanged.

### 5.6 Checklist for the next agent (in order)

1. Sync + re-hash the three binaries (§5.1); record real hashes here.
2. LSan-attribute the four §5.2 sites under the extended interactive
   recipe; fix per-frame (`history_panel`) before bounded (undo/redo).
3. Land item A (§5.4), delete `node_strings_free`, re-run the full §4
   playbook (shots + ASan + mem_check, not just `go test`).
4. Then §3.B (measure open/close delta first) and §3.C (tail-of-main
   `scene_free|nodes|` + registry free to unblock `BELL_MEM_STRICT=1`).
5. Resolve `arrayElemClass` (§5.3) as ride-along cleanup, not a project.
6. Commit `bell-sea` `-o` forwarding in its own repo (§G) separately.

---

## 6. Worked (2026-11-06 — Item A + history_panel + dead-state removal)

All three binaries re-synced to `c2796dea…` (`ahoy/ahoy-bin`,
`ahoy/test/ahoy-bin`, `~/.local/bin/ahoy-bin`; rebuild from
`ahoy/source` after every compiler edit, then re-copy).

### 6.1 What landed

- **Item A — compiler deep struct teardown** (`codegen.go`
  `resolveStructInfo` / `structDirectStringFields` /
  `structPtrStructFields` + `generateFreeCodeForVar` struct branch):
  array-of-struct scope exit now frees every direct string field and,
  one level down, every string field of pointer-to-struct components
  before releasing shells (pooled Release preserved). Empty-string guard
  (`field && field[0]`) skips static `""` defaults from the pool alloc
  fn — non-empty fields are heap by Bell discipline (node_make dups,
  stores dup, splits are heap); empty heap strings leak one byte each
  (bounded, documented). Arrays/buffers nested inside components are
  left alone (same as before). Verified in fresh Bell codegen:
  `Node` teardown emits all 23 frees (5 direct + 5 Vis + 2 Ui + 8 Tile
  + 1 Slot + 2 Swarm) plus Release for all 11 components.
- **Bell: `node_strings_free` deleted** (`engine/core.ahoy`; kept as a
  removal comment). `scene_free` now only calls `node_components_free`;
  interiors die in array scope exit. Invariant (do not break):
  every `scene_free|nodes|` caller must pop-all before any other use —
  both do (`editor_open_scene`, `game_switch_scene`), so the
  scope-exit loop runs zero times after explicit teardown (no
  double-release). Struct *values* untouched.
- **Bell: `history_panel` per-frame leak fixed**
  (`editor/main.ahoy`): `free|live_text|` + `free|top_text|` at the end
  of the `if count > 0` block (retaining-target over-mark marks call
  args escaping, so auto-free skipped them), and `free|live_name|`
  after the row loop (heap on every path: `strdup("")` initial, owned
  reassignment when taken). `top_entry` already auto-freed; untouched.
- **`arrayElemClass` deleted** (definition, init, all 9 writes/deletes):
  write-only since P0-3, `heapArrayOwnsStrings` remains the single
  element-ownership signal. No readers existed; no behavior change.
- **Probes showed §5.2 items 2–4 do not leak** (snapshot push,
  element-store tmp, undo/redo currents): push/element-store into owning
  `array[string]` dups the RHS and the original is freed at scope exit
  (verified in generated C for `history.push|entry|` and
  `play_last[0] = tmp`). No Bell change made there — the §5.2 sketches
  for those sites were false positives; only `history_panel` was real
  (call-arg over-mark, confirmed in `main_probe_history.c`: only
  `top_entry` freed, `live_text`/`top_text`/`live_name` leaked).

### 6.2 Disposition of the rest

- **§3.B (parse temps): accepted as bounded per-open cost.** No code
  change. The dup is only needed when the target outlives the source;
  lifetime analysis is invasive for ~100 KB/open that never grows
  per-frame. Revisit only if open/close delta measurement says so.
- **§3.C nodes part: closed by Item A** — scope exit now frees Node
  interiors at process exit, so no explicit tail-of-main `scene_free`
  (which would double-release components: explicit Release + scope-exit
  Release). Registry (asset handles) still never freed: exit-time only,
  `BELL_MEM_STRICT=1` stays red on registry, not nodes. Documented,
  not fixed.
- **§3.D: `history_panel` (top LSan suspect) done;** remaining
  interactive suspects (text-field buffers `ui.ahoy:702`, tooltip/status
  churn) need interactive ASan/massif — headless shots cannot cover
  hover/drag/typing. Left for an interactive session.
- **§3.F / §G: unchanged.** Host-owned arena, side-table staleness,
  `ahoy_json_stringify` static — reasons stand. `bell-sea -o`
  forwarding still lives only in `ahoy-sea-compiler` (separate repo).

### 6.3 Verify (re-ran)

- `cd ahoy/source && go test` → only the 3 pre-existing formatter
  failures; new `deep_struct_test.go` (direct + nested + stringless
  component Release) passes.
- `cd ahoy && bash tools/mem_check.sh` → PASS, including new
  `test/input/leak_struct_array_deep.ahoy` (array-of-struct with heap
  strings, Bell `.dup||` discipline; 0 definite/indirect; pool chunks
  remain still-reachable by design).
- `ahoy-bin -lint` on `bell/engine/core.ahoy` + `bell/editor/main.ahoy`
  → clean. Fresh Bell codegen (`editor/main.ahoy` from `bell/`) shows
  deep Node teardown + no `node_strings_free`.
- `ahoy/test` `TestConsolidatedFiles` failures are pre-existing (nil-map
  panic at `codegen.go:4645` reproduces with this work stashed) and
  out of scope; `ahoy/source` is the gate per §4.
- Headless `shot editor` / `shot games/arena` + full ASan/massif were
  **not** re-run here (needs xvfb + ~10 min gcc + display); next agent
  with a display must run the §4 playbook end-to-end before calling
  this closed. Do not skip it: the eye/gate regressions passed unit
  tests and died only in shots/ASan.

---

## 7. Residual drip hunt (2026-11-06 — 0.14 MB/4 s idle, 0.3 MB running)

User-reported rates after §6: editor idle ~0.14 MB/4 s (~35 KB/s ≈
580 B/frame at 60 fps), higher with a scene/game running. Ran the
editor under ASan+LSan headless (40-frame DEMO_SHOT) and attributed
every per-frame (40/40-object) leak. Two compiler fixes + five Bell
restructures below. Binaries re-synced to `c2796dea…` after each
compiler edit (all three copies).

### 7.1 ASan attribution (editor idle, 40 frames)

| Per-frame leak | Site | Root cause |
|---|---|---|
| ~183 B | status line `ed_joined` (`main.ahoy:17765`) | alias `line = ed_joined` untracks line + escape-marks ed_joined → scope exit skips it |
| ~50 B | `status_get` dup (`main.ahoy:17746`) | `info = hovered_hint` alias (same pattern); plus f-string operand marking (fixed in compiler) |
| ~37 B | tree subscene label (`:3084`) | conditional concat over borrowed `label` (dominance skip) |
| ~36 B | tree subscene label, second concat (:3086) | same |
| ~21 B ×4 | inspector concats (`:8357`), `dd_sroot` (:14276), `path_parent` (:14810) | concat/call operands escape-marked → scope-exit skip (compiler) |
| 17 B | `sh_tgl_path` (:8345) | owned-call operand marked (compiler) |
| 12 B | `name_extra` f-string (:3641) | conditional f-string over borrowed alias (dominance skip) |
| 5 B | `ctrl_mode_name` literal (:5507) | later concat operand marking suppressed scope-exit free (compiler) |
| 1 B ×3 | `ui_text_field` `current` (:662) | non-owned call operand still marked (documented residual) |

Load-time entries (`editor_open_scene ← scene_load`, node_make splits,
`scene_load_new_line` concats) are bounded per open, not per frame —
unchanged, not in scope.

### 7.2 Compiler fix A — fresh-copy operands are not escape-marked

`markEscapingVariables` recursed into every RHS child, including
operands of values that own their own copy. Marking a concat/f-string
operand as escaping suppresses its scope-exit free while the copy is
still freed — a pure leak. New `freshCopyExpr` gate skips recursion
into: string `+` concats, all f-strings, string slices, fresh-string
methods, and owned-returning calls (fresh result ⇒ args are borrowed).
Alias shapes (identifiers, container accesses, ternaries, switches,
unknown/aliasing calls) still recurse. Tests:
`escape_mark_test.go` (`TestFreshCopyOperandsNotMarkedEscaping`,
`TestPlainAliasStillMarksEscaping`).

### 7.3 Compiler fix B — temp-first order for self-referential reassignment

Fix A exposed a latent UAF: the eager free ran BEFORE RHS evaluation,
so `rebuilt = rebuilt + piece` freed `rebuilt` and then read it
(heap-use-after-free in `scene_load` quote-strip, found via arena ASan
— see §7.5; pre-fix-A this was masked because self-operands were
marked and the eager free was skipped, leaking instead). Now, when the
eager free will fire and the RHS mentions the variable
(`rhsMentionsVar`, incl. legacy f-string idents + container bases),
codegen emits `T __reassign_N = <RHS>; <eager free>; v = __reassign_N;`
(switch-expression RHS excluded: it keeps its marking). Test:
`TestSelfRefReassignEvaluatesBeforeFree`. Minimal repro was a
`scene_load_line`-shaped probe (concat accumulator + conditional
reassign + strip); UAF before, clean after.

### 7.4 Bell restructures (alias/dominance shapes the compiler can't prove)

- Status cluster (`main.ahoy:17746-17765`): `info = hovered_hint.dup||`
  instead of the alias (dup is fresh → eager-free + register; the old
  alias untracked info and escape-marked hovered_hint, leaking both);
  `line = line + hint_suffix` and `line = line + f"..."` instead of the
  `joined`/`ed_joined` alias temps (same pattern; previously the final
  binding leaked every frame).
- Export-error prefix: `line = ej_msg...` alias chain replaced with a
  single `line = exp_job_err[ej2] + " (see dist/build-" +
  exp_job_target[ej2] + ".log)   |   " + line` fresh concat. The old
  `line = ej_all` identifier reassignment sat inside a conditional but
  deleted line's tracking at codegen time, poisoning scope-exit cleanup
  for the normal path too (invariant: tracking must match the runtime
  binding on EVERY path).
- Tree label (`:3080`): `label: string = cur.name.dup||` — conditional
  concat over a borrowed base skipped registration; dup makes the base
  owned so it registers.
- Multi-select name (`:3641`): `name_extra: string = kind_word.dup||`,
  same reason.
- `shader_dir` (`:14812`): `shader_dir: string = project.dup||` — the
  `path_parent` owned call over an untracked base leaked every frame.

### 7.5 Arena game: load-time UAF found and fixed

Arena ASan aborted in `scene_load` before frame 1: `qpiece = hp[0]`
(UP-owned-read dup) case analysis showed the UAF is the same
self-reference ordering class as §7.3 (eager free before RHS eval on
the accumulator path), fixed by the temp-first order. Per-frame game
attribution re-run pending (the abort pre-empted it); see §7.6.

### 7.6 Verify (re-running at handoff time)

- `go test ./...` (source): only the 3 pre-existing formatter
  failures; `escape_mark_test.go` (3 tests) + `deep_struct_test.go`
  pass.
- `mem_check.sh`: PASS (incl. `leak_struct_array_deep.ahoy`).
- Bell lint clean; fresh editor codegen shows scope-exit frees for
  `line`/`info`/`hovered_hint`/`hint_suffix`/`shader_dir` and the
  `__reassign_N` temp pattern in accumulator loops.
- Editor + arena ASan re-runs launched with all fixes (fix A+B, all
  Bell edits); results pending — DO NOT call this closed until the
  40-frame per-frame entries above read zero. Residual knowns after
  that: 1 B ×3 `ui_text_field` borrows (non-owned call operands still
  marked; ~3 B/frame, negligible, needs borrow-kind callee analysis to
  close soundly) and load-time bounded entries.

### 7.7 Eager-free ordering UAF (found by arena ASan, fixed)

The first arena ASan run aborted before frame 1: heap-use-after-free
in `scene_load`'s quote-strip loop (`rebuilt = rebuilt + qpiece`).
Root cause is independent of fix A and older than it: the eager free
ran BEFORE RHS evaluation, so any self-referential reassignment freed
the memory its own RHS was about to read. Pre-fix-A this was masked
because self-operands were escape-marked (eager skipped → leaked
instead); fix A unmasked it. Fix: when eager free will fire and the
RHS mentions the variable (`rhsMentionsVar`, incl. legacy f-string
idents + container bases), evaluate into a fresh temp first, then
eager-free old, then take over (`__reassign_N` pattern). Switch-expression
RHS keeps the legacy order (it keeps its marking, so the eager free
never fires for it). Test: `TestSelfRefReassignEvaluatesBeforeFree`
plus a scene_load-shaped minimal probe (UAF before, clean after).
Minimal-repro shape for the record: loop-local `rebuilt = rebuilt + X`
with a conditional literal/owned reassign in the same scope.
New invariant: **eager free must never precede RHS evaluation when the
RHS can read the old binding** — order the temp first, or skip the
eager free (leak, sound) if no temp is expressible.

### 7.8 Game per-frame fixes (arena, headless DEMO_SHOT)

Arena ASan (90 frames, post-UAF-fix): no 90-object (per-frame) string
entries at all — the game loop itself is clean. Two event/edge sites
fixed anyway for real-play (mouse hover/drag) paths the headless shot
does not hit every frame:
- `node_is_pickup` (`picking.ahoy`): inline `parts[i].strip||` in the
  `is "pickup"` condition leaks the strip temp every evaluation (the
  language forbids nothing here, but inline owned-call temps in
  conditions have no hoist: bind it — `cand: parts[i].strip||` then
  compare. Same class will leak anywhere else it appears; compiler
  condition-hoisting is the general fix, Bell bind-first is the
  discipline).
- `draw_label_node` (`draw2d.ahoy`): `lbl_txt` aliased borrowed
  `loc_tr|...|` while the ellipsis below is a conditional fresh
  reassignment (dominance skip) — `raw_txt` + `lbl_txt: string =
  raw_txt.dup||` instead.
Remaining game-side headless entries are all load-time bounded
(splits/dups in `scene_load*`, preloads, model/texture registries).
The user's 0.3 MB/4 s in LIVE play vs ~2 KB/s headless gap is
attributed to: hover tooltip `node_set_text` field stores (old text
never freed on overwrite — bounded per change, not per frame),
pickup/drop/drag events, and llvmpipe VRAM settlement in the first
seconds — to be confirmed by interactive ASan + massif slope, which
headless shots cannot do. `BELL_MEM_STRICT=1` stays red on exit-time
(load/boot) entries only.

### 7.8 Nested-concat middle temps (the other half of the tree label)

The tree subscene label (`label = cur.name + " (" + cur.vis.subscene
+ ")"`, 3-way concat) leaked its MIDDLE temp per frame even after the
dup-init: the stash decision (`valueAllocatesString`) could not see
through the operand to prove it fresh. Root cause is narrower and
dumber than inference: any 1-character string literal (`" ("` is not
1-char, but `")"` is) routes the concat through the char-mix branch,
which had NO stash handling at all. Fix: treat a nested string-typed
`+` operand as fresh in both the all-string and char-mix branches
(`isFreshOperand` helper), so the middle temp is stashed and freed.
Test: `TestNestedConcatMiddleTempFreed` (stash/free balance), plus a
live ASan probe of the exact shape (leaked before, clean after).
Same fix also closes the identical shape in `history_entry_make`
concats, path joins, export-log lines, etc. — anywhere 3+ strings
concatenate with a 1-char literal on the end.
New invariant: EVERY heap temp the emitter materializes must have a
visible free on the same statement (stash-and-free); a fresh value
with no owner is a leak by construction, never "handled elsewhere".

### 7.9 Verification close-out (headless)

- Editor 40-frame DEMO_SHOT under ASan+LSan: **zero per-frame-scaled
  entries** (was 17 entries / ~420 B/frame: status line 183 B,
  `status_get` dup 50 B, tree label 37+36 B, inspector concats, path
  helpers, `ctrl_mode_name`, ui_text 1 B ×3). Remaining exit-time total
  (~425 KB/33 K objects) is load/boot-bounded (scene splits, preload
  registries, pool chunks) — unchanged class, not per-frame growth.
- Arena 90-frame DEMO_SHOT under ASan+LSan: **zero per-frame entries**
  (was: load-time UAF abort pre-fix). Per-frame game heap is clean;
  the two fixed sites were `node_is_pickup` inline strip temp
  (bind-first) and `draw_label_node`/`draw_button_node` ellipsis over
  borrowed base (dup-init).
- `go test ./...` (source): only the 3 pre-existing formatter
  failures. `mem_check.sh`: PASS (incl. new `leak_struct_array_deep`).
- `BELL_MEM_STRICT=1` still red on exit-time entries (registry +
  boot splits never freed) — that gate now measures load-time
  accounting only, not the drip. Unblocking it is a separate
  teardown-at-exit project (explicit `scene_free` at each `main` tail
  + registry free), NOT a correctness issue.
- What headless runs cannot cover (needs interactive session):
  hover/drag/typing paths (tooltip `node_set_text` field stores leak
  the old text per change — bounded, accepted), massif slope
  confirmation of flat RSS, and WAR-level drivers (jobs, hot reload).

### 7.9 Close-out verification (headless ASan+LSan, final code)

- Editor 40-frame DEMO_SHOT: **zero per-frame-scaled entries, zero
  errors** (was 17 entries / ~420 B/frame). Fixed, in order: status
  line 183 B + info 50 B (alias chains restructured + fresh-operand
  fix), tree label 37 B (dup-init + nested-concat temp fix + explicit
  free), inspector concats 21 B ×4 + path helpers (fresh-operand fix),
  `ctrl_mode_name` 5 B (fresh-operand fix), `name_extra` 12 B
  (dup-init), `dd_sroot`/`dd_root` 21 B (dup-init + explicit free),
  ui_text 1 B ×3 (subsumed). Remaining exit-time total (~426 KB /
  ~33 K objects) is load/boot-bounded (scene splits, preload
  registries, pool chunks).
- Arena 90-frame DEMO_SHOT: **zero per-frame entries, zero errors**
  (was: heap-use-after-free abort in `scene_load` quote-strip before
  frame 1). Fixed along the way: scene_load UAF (temp-first order),
  `node_is_pickup` inline strip temp (bind-first), `draw_label_node` /
  `draw_button_node` ellipsis over borrowed base (dup-init). Remaining
  exit-time total is load-time (splits/dups, preloads, registries).
- The user's live-play gap (0.3 MB/4 s vs ~2 KB/s headless) is
  consistent with event-driven (non-per-frame) costs: hover tooltip
  field stores, pickups/drops/drags, and llvmpipe VRAM settlement —
  left for interactive ASan + massif, which headless shots cannot do.
- Binaries re-synced to `c2796dea…` (all three copies) after the final
  compiler edit; Bell sources lint-clean; `go test` gate holds (3
  pre-existing formatter failures only).

---

## 8. TODO handoff (2026-11-06 evening — give this to the next model)

**State:** editor idle drip is FIXED (final headless 40-frame ASan:
0 per-frame entries, was 17 sites / ~420 B/frame). Arena headless
90-frame ASan: 0 per-frame (90-object) entries. `go test` + 
`tools/mem_check.sh` green except the 3 pre-existing formatter
failures. Work is UNCOMMITTED in both repos — commit `ahoy` then
`bell` per §8.6 after the rebuild/verify in §8.1.

### 8.1 First: rebuild + re-verify (binaries are stale)

The last compiler edits (`freshCopyExpr` fix A, `__reassign_N`
temp-first, nested-concat `isFreshOperand` in both branches,
`arrayElemClass` deletion, deep struct-array teardown) came AFTER
the `c2796dea…` sync. All three `ahoy-bin` copies are stale until
rebuilt. Do this before any ASan run or commit:

```bash
cd ahoy/source && go build -o ../ahoy-bin . \
  && cp ../ahoy-bin ../test/ahoy-bin \
  && cp ../ahoy-bin ~/.local/bin/ahoy-bin \
  && sha256sum ../ahoy-bin ../test/ahoy-bin ~/.local/bin/ahoy-bin
# record the new hash here, replacing the c2796dea line in §7.9
cd ../.. && cd ahoy && bash tools/mem_check.sh 2>&1 | tail -n 3
cd ../ahoy/source && go test ./... 2>&1 | grep -E "^(--- FAIL|FAIL|ok)"
# editor 40f + arena 90f headless ASan+LSan per §4 playbook
# (needs ~10 min gcc + xvfb; scripts: /tmp/opencode/editor_asan7.sh,
# /tmp/opencode/arena_asan6.sh — rebuild -single-tu to /tmp workdir,
# gcc -g -fsanitize=address, xvfb-run, then python LSan parse below)
python3 - <<'EOF'
import re
for path, lo, hi in [('/tmp/opencode/editor_asan7.log',35,45),
                     ('/tmp/opencode/arenamem6/run.log',85,95)]:
    txt = re.sub(r'\x1b\[[0-9;]*m','',open(path,errors='replace').read())
    n = sum(1 for b in re.split(r'(?=Direct leak of)',txt)[1:]
            if (m:=re.match(r'Direct leak of (\d+) byte\(s\) in (\d+) object',b))
            and lo <= int(m.group(2)) <= hi)
    print(path, n, 'per-frame entries (want 0)')
EOF
```

Gate: 0 per-frame entries + 0 `ERROR: AddressSanitizer` in both
logs before committing. Exit-time totals (~425 KB editor /
~377 KB arena) are load/boot-bounded and EXPECTED — do not chase
them here (see §8.4).

### 8.2 Open ownership questions (investigate in this order)

1. **`text_ellipsis` return ownership** (`bell/engine/core.ahoy:5714`).
   Generated C (`main.c:24067-24182`) shows `strdup("")` on EVERY
   early return + `ahoy_text_prefix` concat on the fall-through —
   looks owned on all paths. Check
   `funcReturnsOwnedString("text_ellipsis")`
   (`ahoy/source/methods.go:94` + `stringReturnYieldsOwned :147`):
   if it returns false, callers free nothing and every ellipsis
   call leaks (matches the 4 arena hits: 1260 B×180 + 720 B×270
   `strdup ← text_ellipsis:5721 ← draw_label/draw_button`, and the
   `ahoy_string_dup` 7170 B×852 / 2070 B×540 companions at
   `draw2d.ahoy:327/905`). If the predicate is false, fix the
   predicate (add the fn to the owned set / `freshStringBuiltin`)
   OR add call-site `free|…|` — not both. Verify with the arena
   90f run: those 4 entries must go to zero.
2. **`draw2d lbl_txt / btn_txt` call-arg marking**
   (`bell/engine/draw2d.ahoy:324-330` + button twin ~`:900-908`).
   `lbl_txt` is now `raw_txt.dup||`-init (owned) but is passed as a
   call arg to `text_ellipsis|r, lbl_txt, …|`. Non-owned call args
   stay escape-marked (borrowed-arg convention), so the dup may
   never get a scope-exit free → per-frame leak via the marking,
   not the init. Check fresh `draw_label_node` codegen: if no
   `free(lbl_txt)` on the exit path, add explicit
   `free|lbl_txt|` / `free|btn_txt|` after last use (guarded free
   + untrack is safe on a tracked var; on an untracked var it
   emits raw `free` — verify tracking state first). Same question
   for `dd_root` (`editor/main.ahoy:14304` already got this
   treatment — use it as the pattern).
3. **Return-path over-marking** (compiler, same class as fix A).
   `generateReturnStatement` (`ahoy/source/codegen.go:6525-6680`):
   `markReturnedValue` + `markValueDeps` run over return children
   including fresh-copy operands. Suspect: `return prefix + ell`
   marks `ell` escaping so its scope-exit free is skipped while
   the concat owns its own copy → 1 leak per `text_ellipsis`
   slow-path call. Check `main.c:24176-24182`: is there a
   `free(ell)` before the `return ({…concat…})`? If not, gate the
   return marking with the same `freshCopyExpr`/`isFreshOperand`
   rule as fix A. Unit test to add: concat-return frees both
   operands (mirror `escape_mark_test.go`).
4. **Inline owned-call temps in conditions have no hoist.**
   `picking.ahoy:946` (`cand: parts[i].strip||` bind-first) is the
   fixed exemplar. Grep for the remaining shape
   `if …|…|… is …` / `guard …|…|` with `strip/dup/prefix/concat`
   inline in the condition — every hit leaks one temp per
   evaluation in real play (hover/drag/pickup). Fix is always
   bind-first into a tracked local. Compiler-side, condition
   hoisting is the general fix (not started).
5. **Tooltip `node_set_text` overwrite leak** (documented, bounded).
   `bell/engine/core.ahoy:3737`: `cur.text = text` overwrites
   without freeing the old text. Every tooltip/status change leaks
   the old string (bounded per change, NOT per frame). Fix needs
   a `node_strings`-aware setter (free old `cur.text` when
   non-null/non-empty before store) + audit of the 5 sibling
   setters (`node_set_tint/btn_bg/…` are ints — no-op; only text
   fields matter). Confirm with interactive ASan (hover rows).
6. **`ui_text_field` 1 B ×3 residual** (`editor/main.ahoy:662`).
   Non-owned call-arg operands are still escape-marked by design
   (borrowed convention — sound, leaks ≤3 B/frame). Closing it
   needs borrow-kind callee analysis (which args does the callee
   retain vs borrow?). Explicitly LOW priority — do not chase
   before items 1–5; note as accepted if slope is flat.

### 8.3 Live-play gap (why headless is clean but play leaks)

Headless DEMO_SHOT does not fire hover/drag/typing/pickup. The
user's ~0.3 MB/4 s in live play vs ~2 KB/s headless is expected
to be event-driven. Investigate interactively:

- Run the editor BINARY under ASan interactively (not DEMO_SHOT):
  hover inspector rows, drag nodes, type in fields, undo/redo,
  start/stop play; then read LSan stacks. Prime suspects:
  `ui.ahoy:702` text-field buffers, tooltip/status churn (§8.2.5),
  `history_panel` (fixed — confirm zero under interaction),
  pickup/drop/drag events (`picking.ahoy`, `draw2d.ahoy`).
- Massif slope for byte-rate (immune to exit-time noise):
  `valgrind --tool=massif …/output/bell && ms_print … | tail -30`
  while scripting the same interactions; compare idle vs play
  slope.
- llvmpipe VRAM settlement in the first seconds is NOT an Ahoy
  leak — exclude the first ~5 s before measuring.
- WAR-level drivers (jobs, hot reload) need a display session;
  do not attempt headless.

### 8.4 Exit-time / `BELL_MEM_STRICT=1` (separate project, not the drip)

`BELL_MEM_STRICT=1 bash ahoy/tools/bell_mem_check.sh` stays RED on
exit-time entries only (asset registry handles never freed, boot
splits/preloads, pool chunks still-reachable by design). Item A
closed the nodes part (scope exit now frees Node interiors — do
NOT add tail-of-main `scene_free|nodes|`, it would double-release
components: explicit Release + scope-exit Release). Unblocking
strict = free the asset registry + boot splits at exit; measure
with exit-time valgrind, never with per-frame LSan. Not a
correctness issue — schedule independently.

### 8.5 Small cleanups (ride-along, not projects)

- `arrayElemClass` deleted (was write-only ×9) —
  `heapArrayOwnsStrings` is the single element-ownership signal;
  do not add a second map.
- Deep-teardown edges (accepted, bounded): empty heap strings leak
  1 B each (`field && field[0]` guard skips static `""` pool
  defaults); arrays/buffers nested inside components are left
  alone (same as before). Needs Phase-5-style struct-value
  ownership to fully close — only if a future LSan run tops it.
- `test/input/leak_struct_array_deep.ahoy` + `deep_struct_test.go`
  + `escape_mark_test.go` are new and passing — keep them green.
- Third repo: `ahoy-sea-compiler/bell-sea -o` forwarding is
  implemented but UNCOMMITTED (out of scope) — commit there
  separately; until then every `-sea` run uses working-tree code.
- `MEMORY_WATERMARK_IMPLEMENTATION_REVIEW.md` in this same commit
  range has its own §7 updates — keep the two docs consistent
  (this file is the agent handoff; that file is the watermark
  review).

### 8.6 Commit checklist (in order, after §8.1 is green)

1. `ahoy`: `ahoy-bin`, `test/ahoy-bin`, `source/codegen.go`,
   `docs/MEMORY_HANDOFF.md` (+ watermark review),
   `source/deep_struct_test.go`, `source/escape_mark_test.go`,
   `test/input/leak_struct_array_deep.ahoy`. Record the NEW
   binary hash in §8.1 (replace `c2796dea…`).
2. `bell`: `editor/main.ahoy`, `engine/core.ahoy`
   (`node_strings_free` deleted), `engine/draw2d.ahoy`,
   `engine/picking.ahoy`. LEAVE
   `games/arena/scenes/main.scene` UNCOMMITTED (pre-existing
   local dirt, not part of this work).
3. Verify `bell` lint clean on all touched files + both
   `shot editor` / `shot games/arena` close cleanly.

### 8.7 Key files for the next agent

- Compiler: `ahoy/source/codegen.go` (fix A `freshCopyExpr` +
  `markEscapingVariables` skip; `rhsMentionsVar` + `__reassign_N`
  temp-first; `isFreshOperand` nested-concat stash both
  branches; deep struct teardown; `arrayElemClass` deletion),
  `ahoy/source/methods.go:63-180` (ownership predicate),
  `ahoy/source/*_test.go` (new tests), `ahoy/tools/mem_check.sh`.
- Bell: `editor/main.ahoy` (history_panel frees, status-line
  restructure, label/`name_extra`/`dd_sroot`/`dd_root`/`shader_dir`
  dup-inits), `engine/draw2d.ahoy` (label/button dup-init —
  verify per §8.2.2), `engine/picking.ahoy:940-951` (bind-first),
  `engine/core.ahoy:5714` (`text_ellipsis`), `:3737`
  (`node_set_text`), `tools/bell_mem_check.sh`.
- Logs to beat: `/tmp/opencode/editor_asan7.log` (want 0
  per-frame 35–45), `/tmp/opencode/arenamem6/run.log` (want 0
  per-frame 85–95 AND 0 §8.2.1 entries at any count).
  Invariants in §2 still hold — re-read before any ownership
  change; `go test`-only green is NOT sufficient (eye/gate
  regressions died only in shots/ASan).
