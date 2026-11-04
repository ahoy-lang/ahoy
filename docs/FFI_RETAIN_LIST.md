# FFI Retain List (Watermark Plan Phase 6)

## Policy
C arguments default to `Owned` (see MEMORY_WATERMARK_PLAN.md §3.4). A C function
that retains an Ahoy pointer beyond the call must be listed here so the
compiler and reviewers know the value must be promoted (`strdup`/copy) at the
call boundary rather than passed as an arena/static pointer.

## Compiler-enforced promotions
- F-strings passed to C calls: forced `Owned` via `inForeignCall` (ahoy_owned_sprintf).
- String identifiers passed to C calls: promoted to `Owned` via `strdup` in
  `generateMaybeCharArg` when not already `Owned`.
- String results returned from C calls: treated as borrowed; caller must
  `strdup` if it needs to retain.

## Bell C functions that retain Ahoy pointers
- `glue.h` node callbacks (`ready`, `process`, `draw`, `physics`, `vis`, ...):
  store `AhoyFn` (code+env) by value into `Node`; env is malloc'd and owned
  by the node (released in `scene_free`).
- `assets_bind_textures` / `assets_load_model`: store texture/model handles
  (C-owned, not Ahoy pointers) — no retain.
- `ahoy_sb_*` (string builder): stores char* internally; caller frees via
  `ahoy_sb_free`.
- Raylib/SDL: handles are C-owned (foreign); Ahoy never frees them.

## Hot-reload state audit
- Arena runtime definitions are guarded by `AHOY_HOT_RELOAD_MODULE` so a
  reloaded module does not redefine host globals.
- `__ahoy_frame_arena` / `__ahoy_scene_arena` / `__ahoy_scene_arena_next` are
  TLS and live in the host; reloaded modules reference them via main.h externs.
- Bell globals: no Bell `.ahoy` declares module-level mutable state pointing
  into the scene arena (scene graph is passed as `nodes:array[Node]` and freed
  via `scene_free` on switch). Engine state lives in `Assets`/`Renderer`
  structs (Owned, malloc'd), not arena memory.
- Reload happens at frame boundary (engine_run loop), so no in-flight arena
  mark is active during reload.

## Verification
- `tools/bell_mem_check.sh`: PASS (90 frames, no memory errors, 2026-11-04).
- `tools/mem_check.sh`: PASS (all leak programs).
