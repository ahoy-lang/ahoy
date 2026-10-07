# ahoy engine

A small node/script engine for 2D and 3D games, written in Ahoy and running on
raylib through a swappable renderer contract.

```text
engine/
  renderer.ahoy   Renderer contract (a table of function values) + raylib backend
  core.ahoy       Node, the scene array, scripts, node tree, groups, draw passes
  scene_file.ahoy loading nodes from a text level file
  assets.ahoy     texture/model/text registry, async text loading on the job pool
  runtime.ahoy    the frame loop (2D and 3D)
demo/             a 2D game: its level comes from a scene file loaded on the pool
demo3d/           cubes parented to a pivot node + spinning .obj meshes
```

## Build and run

```bash
./ahoy-bin -f demo/main.ahoy   -r -D BACKEND_RAYLIB              # 2D game
./ahoy-bin -f demo3d/main.ahoy -r -D BACKEND_RAYLIB              # 3D scene
# headless checks: render 90 frames, screenshot at frame 60, then exit
./ahoy-bin -f demo/main.ahoy   -r -D BACKEND_RAYLIB -D DEMO_SHOT
./ahoy-bin -f demo3d/main.ahoy -r -D BACKEND_RAYLIB -D DEMO_SHOT
```

`-D BACKEND_RAYLIB` selects the raylib backend (the `when BACKEND_RAYLIB` block
in `renderer.ahoy`). Without it a game builds against `make_null_renderer()`, a
no-op table that keeps everything compiling for tests.

## How a game is written

A scene is a flat `array[Node]`; its index is the node id. A node carries a
transform, presentation fields and three scripts as function values:

```ahoy
@ player_process |nodes:array[Node], id:int, dt:float, r:Renderer| void:
    n: nodes[id]                 ? structs are values: read one out
    n.x = n.x + 100.0 * dt
    nodes[id] = n                ? ... and write it back
$

@ build || array[Node]:
    nodes: array[Node] = []
    p: node_sprite|"player", tex_index|
    p.process = &player_process
    node_add|nodes, p|
    return nodes
$
```

`engine_run|nodes, renderer, textures|` owns the loop and calls
`ready`/`process`/`draw` for every visible, live node in layer order.

Rules that follow from the language (see also "Limitations"):

- **Read a node out, change it, write it back.** Structs are copied; arrays and
  dicts are pointers, so their contents are shared.
- **Scripts cannot reach `main`'s locals.** Per-node state lives in the node's
  own fields (`x`, `rot`, `scale`, `text`, `vx`, `vy`, `hp`, `timer`, `data`),
  or in arrays/dicts declared in `main` and passed to the script.
- **`loop till COND do` loops *while* COND holds.** Wait for a condition with
  `loop till not COND do`.
- **An expression inside `f"...{expr}..."` is substituted only when it is a
  plain name.** Compute anything else into a local first.
- **No nested calls in a call's argument list.** Assign to a local and pass it.

## Node tree, groups, draw order

- `node_add_child|nodes, parent, child|` parents a node. A node's `x/y/z` are
  local; `node_world_x/y/z|nodes, id|` walk the chain and `node_world_scale`
  composes scale. Parent rotation is not applied to children yet (that needs the
  trig helpers).
- `node_find_path|nodes, "pivot/orb3"|` walks names down the chain.
- Groups are a space-separated list on the node (`group=props` in a scene file):
  `node_in_group`, `group_find` (ids) and `group_process` (a whole group's
  process scripts in one call).
- Draw order is `(layer, list position)` via `build_draw_order`, a stable
  insertion sort over the node indices, so `layer=2` puts crates over a panel.

## 2D and 3D passes

The same nodes and scripts drive both:

- `scene_draw_2d|nodes, r, textures|` draws `sprite`, `label`, `rect` and
  `plain` nodes in screen space (plain nodes have no shape: their `draw` script
  draws them).
- `scene_draw_3d|nodes, r, camera|` draws a grid plus `cube`, `sphere` and
  `node3d` nodes at world (x, y, z), y up, inside `begin3d`/`end3d`.
- `engine_frame_3d` runs the 3D pass and then the 2D pass, so labels and rects
  are a HUD overlay. 2D text inside the 3D bracket would be projected by the
  camera, which is why the passes are split.
- `engine_run_3d|nodes, r, camera, textures|` wraps that loop.

### Meshes and models

`load_model` reads an .obj/.glb through the renderer and `node_mesh|name, index|`
puts it in the scene. A mesh node spins around its Y axis by its `rot` (degrees)
and scales with the node tree:

```ahoy
pyramid: assets_load_model|r, a.models, a.model_names, "pyramid", "demo3d/assets/pyramid.obj"|
rock: node_mesh|"rock", pyramid|
rock.x = 3.4
rock.process = &spin_process        ? n.rot = n.rot + 70.0 * dt
node_add|nodes, rock|
```

Scene files accept `mesh name x y z model=pyramid`.

`demo3d/main.ahoy` parents six cubes to a pivot node, orbits them by rotating
each cube's own local position with `vector3rotate_by_axis_angle` (raymath), and
spins two pyramids loaded from `demo3d/assets/pyramid.obj`.

## Scene files

A level is text, so it can arrive on the job pool while the game shows something
else (`demo/assets/level.scene`, loaded in `demo/main.ahoy`):

```text
sprite player 200 180 player layer=1
rect   panel  300 340 220 40 layer=0 tint=red
label  title  24  20 "ahoy engine demo"
cube   core   0   1   0    1.4
```

Positionals are `kind name x y [size|texture] [z]`, then optional
`layer=N group=NAME texture=NAME tint=red|green|blue|yellow|black|grey|white`.
A label's text is the quoted string. `scene_load` returns how many nodes it
added and skips malformed lines. Scripts are attached by the game afterwards
(`node_find` then `node_set_script`).

## Renderer contract

`Renderer` is a struct of function values: window and frame (`open`, `clear`,
`frame_time`, `key_down`, ...), 2D (`draw_rect`, `draw_sprite`, `draw_text`,
`text_width`), 3D (`begin3d`, `draw_grid`, `draw_cube`, `draw_sphere`,
`draw_line3d`, `end3d`) and assets (`load_texture`, `load_text_file`). Engine and
game code only talk to the table, so a backend swap is a different table.

A backend must keep every draw call on the thread that opened the window:
raylib/OpenGL state lives there. Jobs only decode in memory, and
`assets_drain_text` runs its handler back on the main thread.

## Assets

`assets_load_texture|r, textures, names, name, path|` registers a texture and
returns the index a sprite node stores. Text files can be read on the job pool:

```ahoy
pool: pool = jobs|1|
pending: []
texts: array[string] = []          ? typed: an untyped array reads back as int
assets_request_text|pool, pending, "demo/assets/level.scene"|

loop till not r.should_close|| do
    drained: assets_drain_text|pool, pending, texts|   ? main thread
    if texts.length > 0 then
        added: scene_load|nodes, texts[0], textures, tex_names|
    $
$
pool.stop||
```

`pool.drain` hands out **every** finished job of that pool, not just the ones
from a given pending list, so give each kind of asset its own pool (the demo
uses one for the scene and one for the level text) or route by content.

Texture files still load synchronously: uploading needs the GL context and the
task ABI hands back a single `intptr_t`, so a job cannot return a decode buffer
yet.

## Limitations found while building this

Language/codegen gaps, not engine design choices. Each one cost time to find:

| Area | Behaviour | Workaround |
| --- | --- | --- |
| array field of a struct | `a.textures.push\|t\|` with a C struct element does not compile | pass the array as a parameter |
| `arr[i].field = v` | not parsed | read out, change, write back |
| fn-typed struct field defaults | not parsed | constructors pass the no-op scripts |
| uninitialized declarations | `x: array[int]` without `=` is a parse error | always initialize |
| zero-arg lambdas | `(): expr` is a parse error | take an unused parameter |
| f-string interpolation | only plain names are substituted | compute into locals |
| untyped array read | an element of `out: []` reads back as int, truncating pointers | declare the result `array[string]` |
| closures | a container created inside a loop is freed while the closure still points at it | keep captured containers in `main`'s scope |

Fixed in this work (each has a fixture in `test/input/`): array mutation lint
tracking, typed dict stores, dict length, closure container captures, lambda
thunk signatures, `when` block parsing, zero-arg function references, the job
drain race, string `is`, string `split`, `int("42")`/`float("1.5")`,
`s.length` on strings, assignment-from-container shadowing, enum-member shadowing
of locals, and imported-package validation.

## Next slices (towards Godot-style node parity)

1. **Async textures** via a small C glue header (decode on the worker, upload on
   the main thread in the drain handler); models load synchronously for now.
3. **Signals**: an `array[Signal]` per node (a struct holding the handler, since
   calling a function value straight out of an array is rejected) plus `emit`.
4. **Hot reload** of the gameplay layer (`-hotreload`) with the engine kept in
   its own package: scripts are exactly the part that changes.
5. **Text/UI**: raygui widgets behind a small layout layer, mouse hit-testing and
   a `ui_button` helper.

## Tests

`test/input/` holds the corpus the compiler test suite runs.
`nested_assign_strings.ahoy`, `string_tools.ahoy`, `closure_containers.ahoy`,
`when_scope.ahoy`, `zero_arg_refs.ahoy`, `array_push_bounds.ahoy` and
`dict_typed_values.ahoy` were added while building the engine.

```bash
cd test && go test ./...
```
