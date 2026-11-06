# FFI Retain List (Watermark Plan Phase 6)

## Policy
C arguments default to borrowed pointers (see `generateMaybeCharArg`): C
calls in Bell are non-retaining for text (raylib reads immediately), so
promoting every argument to `Owned` leaked per frame with nobody to free
it. A C function that retains an Ahoy pointer beyond the call must be
listed here so the compiler and reviewers know the value must be promoted
(`strdup`/copy) at the call boundary rather than passed as an
arena/static pointer. (This differs from MEMORY_WATERMARK_PLAN.md §3.4's
"Owned by default" — measured per-frame leaks forced the borrowed
default; the plan's direction is retained for f-strings, which are
`Owned` in foreign-call context.)

## Compiler-enforced promotions
- F-strings passed to C calls: forced `Owned` via `inForeignCall` (ahoy_owned_sprintf).
- String identifiers passed to C calls: passed by pointer (borrowed).
  Retaining C callees must `strdup` explicitly; see the list below.
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
- `ahoy_pose_memo_put` (pose cache, `glue.h`): OWNS its path (dups on put,
  frees the overwritten slot). Callers may pass per-frame temporaries;
  previously it retained the pointer and dangled when Ahoy freed the
  temporary on scope exit (arena UAF, Nov 2026). `ahoy_anim_slot` copies
  into a fixed buffer — no retain.

## C string-result ownership (audited Nov 2026, enforced via
## `freshStringBuiltin` in `ahoy/source/methods.go`)
- Caller-owned (tracked and freed at scope exit): `ahoy_text_prefix`,
  `ahoy_text_from`, `ahoy_str_append_char`, `ahoy_str_backspace`,
  `ahoy_fs_path`, `ahoy_path_join`, `ahoy_lint_capture`,
  `ahoy_strip_comments`, `ahoy_rename_entry`, `ahoy_rename_varying`,
  `ahoy_strndup`, `ahoy_model_bone_name`. Each was verified to
  malloc/strdup on every return path.
- Borrowed, never tracked (caller must `strdup` to retain):
  `ahoy_sb_text` (builder memory), `ahoy_loc_tr` (key/table cell),
  `ahoy_fs_name` (listing cache), `ahoy_read_text` / `ahoy_text_copy`
  (static buffers), `ahoy_loc_lang_name`, `ahoy_anim_name`,
  `ahoy_audio_key`, `ahoy_shader_error`, `ahoy_vram_row_name`,
  `ahoy_vram_row_kind`, `ahoy_shader_uniform_name`,
  `ahoy_shader_uniform_default` (statics or interior pointers).
- Mixed (static on some paths, heap on others — deliberately untracked;
  success paths leak boundedly, tracking them would free statics):
  `ahoy_model_convert`, `ahoy_text_alloc` (static only on failure/
  OOM paths). Revisit if callers ever go per-frame.
- `ahoy_json_stringify` (8 KB shared static buffer): same hazard class
  as the old f-string static (truncation, aliasing, thread race). No Bell
  callers today; fix with a sized heap buffer when first used.

## Hot-reload state audit
- Arena runtime definitions are guarded by `AHOY_HOT_RELOAD_MODULE` in
  generated code, but the macro is not yet passed on module compile lines
  (`source/main.go` hot-reload paths), so reloaded modules still define
  their own arena globals today. Host-owns-arena (host defines the
  runtime, module externs it via main.h) is pending work.
- `__ahoy_frame_arena` / `__ahoy_scene_arena` / `__ahoy_scene_arena_next` are
  TLS (real under gcc/clang; empty qualifier under tcc — see the jobs note
  in MEMORY_WATERMARK_IMPLEMENTATION_REVIEW.md).
- Bell globals: no Bell `.ahoy` declares module-level mutable state pointing
  into the scene arena (scene graph is passed as `nodes:array[Node]` and freed
  via `scene_free` on switch). Engine state lives in `Assets`/`Renderer`
  structs (Owned, malloc'd), not arena memory.
- Reload happens at frame boundary (engine_run loop), so no in-flight arena
  mark is active during reload.

## Verification
- `tools/bell_mem_check.sh`: PASS (90 frames, no memory errors, 2026-11-04).
- `tools/mem_check.sh`: PASS (all leak programs).
