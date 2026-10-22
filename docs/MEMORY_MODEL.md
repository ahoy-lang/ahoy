# The Ahoy memory model

*2026-10-21. This is the reference for how memory is managed in Ahoy: who
frees what, when, and what the rules are for code that keeps values alive
longer than the scope that made them.*

> Implementation companion: `MEMORY_OWNERSHIP.md` records the
> compiler-side changes (allocation classes, frame/scene region
> lifecycle, split-piece ownership, borrow promotion at store sites,
> retaining-boundary ownership) and the ASan-validated Bell results in
> `MEMORY_BASELINE.md`.

## The short version

**Ahoy has no garbage collector and no reference counting. Containers are
owned by the scope that declares them and are freed when that scope ends.**
The compiler inserts the `free` calls; you do not write them.

```ahoy
@ main ||:
    items: array[int] = [1, 2, 3]   ? malloc'd here
    print|items|                      ? still alive
$                                     ? freed here, at the end of main
```

There is exactly one heap for the values a program makes, and the scope
boundary is where it is reclaimed. That is the whole model.

## What the compiler tracks

The code generator keeps a set of the names it has allocated on the heap
(`heapAllocatedVars`) together with their type, and frees them at the end of
the scope that declared them, in reverse declaration order. It also tracks
the names whose storage *escapes* the scope, so those are not freed:

* a value that is **returned** - the returned pointer must outlive the frame,
  so the variable it came from is marked as escaping (it leaks, soundly,
  rather than dangling);
* a value that is **stored into** a container that outlives the scope;
* a value a **closure captures** - the closure's environment holds the
  pointer, so the captured name is marked as escaping;
* a name **mentioned by a pending `defer`** - the deferral runs at scope
  exit, so the name has to stay alive until then.

Freeing is skipped for a name that was freed by hand (`free|x|`), already
auto-freed, or is marked as escaping. The compiler errs towards leaking: a
leak is a wasted allocation, a dangling pointer is a crash.

One sharp edge, fixed: a whole container stored into a **struct field**
(`h.box.items = vals`, including chains through pointer fields like
`cur.swarm.flow_dir = dirs`) aliases the value — no copy — so the field
dangles if the scope exit frees it. Function-scope cleanup used to free
containers even when marked escaping (only the elements were thought to
escape), which corrupted exactly this pattern (length read back fine,
contents garbage). Struct-field stores now carry their own retain mark
(`storedVars` in `source/codegen.go`) that the function-scope cleanup
honors. Call-arg and push marks stay borrow-only: passing an array to a
function that only reads it must still free at scope exit.

## The rules that follow

1. **A container dies with its scope.** If a value has to outlive the scope
   that made it, return it, store it in a longer-lived container, or capture
   it in a closure - the compiler notices those three cases and keeps the
   storage alive.
2. **Do not free by hand unless you know what you are doing.** The compiler
   frees scope-owned containers; a manual `free` of a name the compiler also
   frees is a double free, and a manual `free` of a name the compiler does
   *not* free (because it escapes) is a use-after-free.
3. **A container created inside a loop is freed at the end of each
   iteration.** A closure that captured it and outlives the iteration is
   then pointing at freed memory. Keep the captured container in the
   enclosing scope, not in the loop body.
4. **An `array[SomeStruct]` whose struct has string fields is not safe to
   rebuild.** Popping a struct frees its strings, and the copies that were
   already taken dangle. Use plain string arrays and a small struct for
   numeric data, or keep the strings out of the array.
5. **Returning a temporary is safe only through a local.** `return
   xs[0].strip||` used to be a use-after-free: the return-path cleanup
   freed the container before the return expression read it. The compiler
   now stashes the value first (see `test/input/return_temp.ahoy`), but
   assigning to a local first is still the clearest thing to write.

## Checking it

* `-show_leaks` builds the program with AddressSanitizer
  (`-fsanitize=address`). ASan reports both leaks and use-after-free, with
  a stack trace. This is the tool for finding the mistakes above; the
  default build is not instrumented because ASan is slow.
* The compiler's own lint has a set of rules around the model (an array
  reassigned inside a loop, a closure capturing a loop-local container, a
  freed name reused). They are warnings, not errors, because the compiler
  cannot always prove the shape of the program.

## What this costs, and what it buys

**Buys:** no GC pause, no reference-count overhead, deterministic frees, and
a C-shaped memory profile - which matters for a game engine that wants to
hand buffers to C without a translation layer. Fallible operations use the
`(value, err)` convention (`docs/ERROR_HANDLING.md`), not exceptions, so
failure handling is data flow that composes with pass-through checks.

**Costs:** lifetimes are a thing you have to think about. There is no
`Option`, no `Result`, no borrow checker, and no "this is still alive"
question the compiler can always answer. A game with dynamic lifetimes -
spawning and despawning entities, pooling, async asset loads - will hit the
edges of this model, and the failure mode is a use-after-free rather than a
diagnostic.

This is the single biggest reason the language would benefit from an
explicit ownership story (arena handles, optional refcounting for
containers, or a `Result`/`Option` pair for fallible operations). It is
also the reason the engine's `Node` struct is so awkward: a 205-field struct
with value semantics cannot be moved out of an array without a copy, and the
copy has to be written back by hand.
