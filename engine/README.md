# ahoy engine

A small node/script engine for 2D games (3D is next), written in Ahoy and
running on raylib through a swappable renderer contract.

```text
engine/
  renderer.ahoy   Renderer contract (a table of function values) + raylib backend
  core.ahoy       Node, the scene array, scripts, draw helpers
  assets.ahoy     texture and text registry, async text loading on the job pool
  runtime.ahoy    the frame loop
demo/
  main.ahoy       the game: nodes + script functions only
```

## Build and run the demo

```bash
./ahoy-bin -f demo/main.ahoy -r -D BACKEND_RAYLIB
# headless check: renders 90 frames, screenshots at frame 60, then exits
./ahoy-bin -f demo/main.ahoy -r -D BACKEND_RAYLIB -D DEMO_SHOT
```

The `-D BACKEND_RAYLIB` define selects the raylib backend (`when BACKEND_RAYLIB`
in `renderer.ahoy`). Without it the demo builds against `make_null_renderer()`,
which is a no-op backend that keeps the game compiling for tests.

## How a game is written

A scene is a flat `array[Node]`; its index is the node id. A node carries a
transform, presentation fields and three scripts as function values:

```ahoy
@ player_process |nodes:array[Node], id:int, dt:float, r:Renderer| void:
    n: nodes[id]                 ? arrays hold structs by value: read one out
    n.x = n.x + 100.0 * dt
    nodes[id] = n                ? ... and write it back
$

@ build || array[Node]:
    nodes: array[Node] = []
    p: node_sprite|"player", tex_index|
    p.x = 100.0
    p.process = &player_process
    node_add|nodes, p|
    return nodes
$
```

`engine_run|nodes, renderer, textures|` owns the loop and calls
`ready`/`process`/`draw` for every visible, live node in list order.

Rules that follow from the language (see also "Limitations" below):

- **Read a node out, change it, write it back.** Struct values are copied;
  arrays and dicts are pointers, so their contents are shared.
- **Scripts cannot reach `main`'s locals.** Per-node state lives in the node's
  own fields (`x`, `rot`, `scale`, `text`, `vx`, `vy`, `hp`, `timer`, `data`),
  or in arrays/dicts declared in `main` and passed to the script.
- **`loop till COND do` loops *while* COND holds.** Waiting for a condition
  reads `loop till not COND do`.
- **An expression inside `f"...{expr}..."` is substituted only when it is a
  plain name.** Compute anything else into a local first.
- **No nested calls in a call's argument list.** `print|len|xs||` is not
  parsed; assign to a local and pass that.

## Renderer contract

`Renderer` is a struct of function values (`open`, `clear`, `draw_rect`,
`draw_sprite`, `draw_text`, `load_texture`, `key_down`, ...). Engine and game
code only ever talk to the table, so a backend swap is a different table.
`make_null_renderer()` returns a valid no-op table; `make_raylib_renderer()`
is the raylib one.

A backend must keep every draw call on the thread that opened the window:
raylib/OpenGL state lives there. That is why jobs only decode in memory and
`assets_drain_text` runs its handler on the main thread.

## Assets

`assets_load_texture|r, textures, names, name, path|` registers a texture and
returns the index a sprite node stores.

Text files can be read on the job pool instead of blocking a frame:

```ahoy
pool: pool = jobs|2|
pending: []
texts: []
assets_request_text|pool, pending, "demo/assets/level1.txt"|

loop till not r.should_close|| do
    drained: assets_drain_text|pool, pending, texts|   ? main thread
    ...
$
pool.stop||
```

Texture files still load synchronously: uploading needs the GL context, and a
job cannot hand back a byte buffer yet (the task ABI carries one `intptr_t`).

## Limitations found while building this

These are language/codegen gaps, not engine design choices. They are recorded
here because each one cost time to find:

| Area | Behaviour | Workaround used |
| --- | --- | --- |
| array field of a struct | `a.textures.push\|t\|` with a C struct element does not compile | pass the array as a parameter |
| `arr[i].field = v` | not parsed | read out, change, write back |
| fn-typed struct field defaults | not parsed | every node constructor passes the no-op scripts |
| uninitialized declarations | `x: array[int]` without `=` is a parse error | always initialize |
| zero-arg lambdas | `(): expr` is a parse error | take an unused parameter |
| f-string interpolation | only plain names are substituted | compute expressions into locals |
| closures | may capture ints, strings, arrays and dicts, but a container created inside a loop is freed while the closure still points at it | keep captured containers in `main`'s scope |
| `hashMapPutTyped` | fixed in this work — see the commit for details | – |
| string `is` | was pointer equality, now `strcmp` | – |

## Tests

`test/input/` holds the corpus the compiler test suite runs:
`nested_assign_strings.ahoy`, `closure_containers.ahoy`, `when_scope.ahoy`,
`zero_arg_refs.ahoy`, `array_push_bounds.ahoy` and `dict_typed_values.ahoy`
were added while building the engine.

```bash
cd test && go test ./...
```
