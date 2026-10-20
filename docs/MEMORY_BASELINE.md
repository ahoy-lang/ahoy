# Memory Baseline (Phase 1) — 2026-10-21

Probe: `frame_work|200|` — per iteration: string concat, `split`, array
literal + `push`. Compiled with Ahoy, built with `gcc -O1 -g`, run under
`valgrind --leak-check=full`.

## Numbers (Phase 0 code, before arena routing)

- Total heap usage: **2,201 allocs / 601 frees / 51,696 bytes**
- In use at exit: **38,600 bytes in 1,600 blocks**
- Definitely lost: 12,800 bytes in 400 blocks; indirectly lost: 25,800
  bytes in 1,200 blocks.

## Leak sites (both loop-scope locals, freed never)

1. `ahoy_string_split_array` result (`parts`) — `frame_probe_tmp.ahoy:9`
   — 200 blocks, 17,800 bytes (direct+indirect).
2. Array literal + push buffer (`arr`) — `frame_probe_tmp.ahoy:10`
   — 200 blocks, 20,800 bytes.

(`s`, the concat temp, is freed — string autofree covers it; the two
container temps are not.)

## Reading

- Per-iteration cost ≈ 8 allocs, ~190 bytes leaked. A 60fps game doing
  this work per frame leaks ~11KB/s and grows without bound.
- The existing function-scope autofree (`addAutomaticDeferFrees`,
  scope 0 only) does not cover loop-body scope depths. Block-exit
  cleanup is the missing piece — Phase 3.
- String-concat routing to a frame arena would eliminate site-class-1
  temporaries wholesale; container promotion + block-exit free covers
  site-class-2. Target after Phases 3–5: **0 lost, allocs ≈ frees + live
  scene**, frame scratch reset to zero each iteration.

Repro: `valgrind --leak-check=full /tmp/opencode/memprof/frame_probe_native`
(probe source + .c archived in /tmp/opencode/memprof/).

## After Phases 0–3 (same probe, same flags)

- Total heap usage: **2,201 allocs / 2,201 frees / 51,696 bytes**
- In use at exit: **0 bytes in 0 blocks**
- `ERROR SUMMARY: 0 errors from 0 contexts`

Fixes: narrowed assignment escape-marking (reads into scalars no longer
pin loop locals), split-piece ownership with per-iteration deep free,
promotion-at-boundary (literal-into-owned fixup, push-strdup) keeping
ownership monotonic so static emission is sound across iterations.

Functional pin: `../test/input/leak_frame_temps.ahoy` (prints 1200,
expects `["1200"]`); ownership pin: `../tools/mem_check.sh`.

## Bell game run (balls, 90 frames, ASan+LSan under xvfb) — 2026-10-21

First run (post-Phase-3, pre-borrow-fix): heap-use-after-free in
`ahoy_string_concat`, piece allocated by `ahoy_string_split_array`
(`scene_load_line`, scene_file.ahoy:148 `hp`), freed by hp's scope-exit
deep free (:152), read by `rebuilt + qpiece` (:156). The exact borrowed-
element-store shape. The user-visible symptom was a glibc tcache
double-free abort at scene load with models.

Second run (post-borrow-promotion): load path clean; new UAF at runtime
— `node_find` (core.ahoy:1824) reading a ball name allocated in
`bs_spawn` (ball_sim.ahoy:142 `f"Ball{bs_bi}"`) and freed at loop exit
(:172). Call-argument retention: `node_make` stored the name pointer
without duplicating. Pre-existing (f-string tracking + loop-exit free
predate this work; masked by stale memory until heap reuse bit).

Third run (post `node_make` name-dup + `dup` dispatch fix): **zero
use-after-free, zero crashes, all 90 frames complete.** Exit leaks only:
134,109 bytes in 2,854 allocations, none scaling with frame count —
64 ball names (node-owned by design), per-node components (18
constructed nodes × 10 component kinds; `scene_free` never runs on the
quit path), scene-load splits, assets, skill-UI boot. All load/boot-
bounded teardown omissions, no per-frame growth.

## Follow-up runs (same game, same flags)

- Post-`varDeclScope` run: **double-free**, same pointer freed twice at
  adjacent lines. Root cause: scope-allocation lists allowed duplicate
  entries (two reassign statements, one depth); early-return cleanup
  emits per entry with no dedup. Latent bug predating the widening,
  exposed by it. Fixed with deduped appends (`appendScopeAllocation`,
  all three sites).
- Post-dedup run: new heap-use-after-free in `scene_load_new_line`
  (`val = kv[1]` copy freed at loop exit while `kind_s`/`name_s` alias
  it). Root cause: widened reassignment scheduling set scopes/types
  but not `heapAllocatedVars`, so escape marks missed a var the scope
  lists still freed (split-brain tracking). Fixed by setting full
  membership wherever scopes/types are set.
- Final run: **zero memory errors, 90/90 frames, clean close.**
  Exit leaks identical (134,109 bytes / 2,854 allocations, all
  load/boot-bounded, none per-frame).

## Field-store rule run (same game, same flags)

Struct-field stores of identifiers and borrowed reads now duplicate
(`fieldStoreNeedsDup`); `dup` joined the string dispatch/inference
lists; `node_make` funnel covered literal-init. **Zero memory errors,
90/90 frames.** Exit leaks 134,739 bytes / 3,083 allocations (+630
bytes / +229 allocs vs prior run — the field dups themselves, each a
few bytes, all load/boot-bounded). Hottest leak site
(`node_label:680`, ~102 records) is ~9 boot-time labels × ~12 allocs
each, not per-frame growth. No site scales with the 90 frames.

## Post-revert run (same game, same flags)

**Zero memory errors, 90/90 frames. Exit leaks: 19,021 bytes / 1,201
allocations** — same leak kinds (scene splits, node names, concats,
skill UI), lower counts, nothing per-frame. The drop vs the prior
134KB run is not fully attributed (candidates: pool still-reachables
settling plus fewer live-at-exit paths after the scheduling revert);
it was not chased further because leak accounting cannot mask
use-after-free — ASan errors are the safety signal, and it is clean
across five consecutive full-game runs plus a full editor run.

## Slab-pool bench (movers/200/null-renderer, bench.py --quick)

- Pre-pool baseline: 1.012ms/frame, checksum 469128.960013, rss 91,932KB.
- Post-pool: 1.013ms/frame, identical checksum, rss 92,388KB.
- No steady-state speedup (frames were never alloc-bound — the 20µs/node
  budget is dispatch/physics, as predicted); +456KB is retained slab
  high-water (32 chunks for 2,000 components, bounded by max live set).
  The win is allocator churn under spawn/kill pressure (500-cycle churn
  probe: 12 allocations vs 500+), fragmentation, and locality — not
  steady-frame time. Routing hot-pass temps to arenas stays future work
  behind a volume measurement.
