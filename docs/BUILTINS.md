# Builtins: asking a collection, and the maths

Three families of builtins that were missing, all of which showed up in
hand-written code.

## The predicate builtins

They ask an array a question instead of a loop with a flag.

```
xs.any|predicate|         is there one that matches?
xs.all|predicate|         do all of them match?
xs.count_if|predicate|    how many match?
xs.find_index|predicate|  where is the first one that matches (-1 when none)?
xs.find|predicate|        the first one that matches
```

The predicate is a lambda, and the code is generated inline, so the lambda
is inlined into the loop rather than called through a function pointer.

```
items: array[Item] = []
items.push|Item{name: "sword", hp: 10, visible: true}|
items.push|Item{name: "potion", hp: 1, visible: true}|

? is there a visible one?
has_visible: bool = items.any|it: it.visible|

? how many have more than 2 hp?
strong: int = items.count_if|it: it.hp > 2|

? where is the potion?
i: int = items.find_index|it: it.name is "potion"|
if i >= 0 then
	potion: Item = items[i]
$
```

They read the element with the same rules as an array access, so they work
on an array of structs and not only on an array of numbers.

Two things to know:

- `find` returns the zero value when nothing matches, which for an array
  of structs is a zeroed element. When the difference between "found" and
  "not found" matters, use `find_index`, which says -1.
- `any` on an empty array is false and `all` is true.

## The maths builtins

```
abs|-3.5|          3.5
min|3.0, 7.0|      3
max|3.0, 7.0|      7
clamp|15.0, 0.0, 10.0|   10
lerp|0.0, 10.0, 0.25|    2.5
sign|-4.0|         -1
floor|2.7|         2
ceil|2.1|          3
round|2.5|         3
sqrt|16.0|         4
pow|2.0, 10.0|     1024
hypot|3.0, 4.0|    5
mod|7.0, 3.0|      1
```

They return a float, so `abs|-5|` is 5.0; write `int|abs|x||` for an
integer.

Each one is a C function in the runtime, written only when the program
uses it: a program that never takes a square root does not carry the
square root.

## The geometry builtins

```
rect_contains|x, y, rx, ry, rw, rh|          is the point inside the rectangle
rect_intersects|ax, ay, aw, ah, bx, by, bw, bh|   do the rectangles overlap
```

They answer the questions that used to be four nested `if`s each. A
rectangle is a position and a size: `x`, `y` is the top left, `w`, `h` the
size.

## The buffer builtins

```
buffer|n|            zeroed buffer[f32] of length n (contiguous float32)
buffer_int|n|        zeroed buffer[int] of length n (contiguous int32)
grow|buf, n|         new buffer of the source's lane, length n, min(len, n) copied
```

A buffer is one flat allocation: no boxing, no per-element tags, every
index bounds-checked. The two lanes are distinct types — `buffer|n|`
into a `buffer[int]` is a compile error, not a silent float. Use the
int lane for counts, grids and lookup tables (exact past 2^24, where
float storage rounded); the float lane for positions and physics.

## The serialization builtins

```
to_bytes|buf|          image of a buffer: LE u32 count, u8 lane tag, then LE payload
from_bytes|bytes|      the buffer[f32] rebuilt, verified; empty when framing or lane fails
from_bytes_int|bytes|  the buffer[int] rebuilt, verified; empty when framing or lane fails
from_bytes_ok|bytes|   the framing verifier alone: length, count, tag, ranges
```

They load what `to_bytes` wrote and nothing else — each reader demands
its own lane tag, so a cross-lane load fails closed too. A short image, a
count that disagrees with the length, a bad lane tag, a non-int tag, an
out-of-range byte, or an absurd count fails closed: the readers yield an
empty buffer and `from_bytes_ok` is false. A corrupt or hostile file is
never a wild reference. The format is a same-platform save image (the
floats are the host's LE encoding), not portable interchange; `array`
and `string` payloads are the next framing, not this one.

## The trigonometry and clock builtins

```
sin|x| cos|x| tan|x|        sine, cosine, tangent (radians)
asin|x| acos|x|             arcsine, arccosine (radians, input clamped to [-1, 1])
atan|x| atan2|y, x|         arctangent; two-argument angle in radians (-pi to pi)
fmod|x, y|                  floating-point remainder (0 when y is 0, like mod)
ln|x|                       natural logarithm (0 when x is not positive)
exp|x|                       e raised to x
time_now||                  seconds since the program started (CPU time, monotonic)
```

`asin`/`acos` clamp out-of-range input instead of returning NaN, and
`atan2|0, 0|` answers 0 — the total-function style the maths builtins
already use. `time_now` reads libc `clock()`: it never goes backwards,
but it is CPU time, not wall time — the frame clock for games is still
the renderer's `frame_time`.

## The random builtins

```
rand||               the whole libc range
rand_range|lo, hi|   inclusive on both ends (raylib GetRandomValue semantics)
```

Seeded once at program start, so every call shares one stream.
`shuffle` and `pick` draw from the same stream too (they used to reseed
per call, repeating the sequence when called twice in one second).

## The extended array builtins

```
xs.slice|lo, hi|        fresh array with elements [lo, hi), clamped
xs.index_of|v|          first index whose slot equals v, else -1 (slot
                        identity, like has; compare strings by value
                        with find_index|it: it is "x"| instead)
xs.reduce|init, f|      left fold over int slots: f(acc, el), e.g.
                        xs.reduce|0, (acc:int, v:int): acc + v|
xs.insert|i, v|         shift right and store (grows like push); same array
xs.join|sep|            concatenate the string slots with sep between
xs.append|v|            alias of push
xs.erase|i|             alias of remove
xs.size||               alias of length
xs.clear||              drop every element (length goes to 0; slots are
                        left for scope exit to reclaim — leak, never dangle)
xs.dup||                shallow copy: new buffers, shared slots (the same
                        aliasing push already has)
```

`insert` and `clear` mutate in place and return the same array, like
`push`/`sort`/`reverse`/`shuffle`. Bind the result to a second name and
both names own one allocation (double-free at scope exit) — the checker
warns. Call them as statements, or rebind the same name:
`xs.push|4|`, `xs = xs.sort||`.

There is deliberately no `each`: a lambda body is a pure expression
(calls do not parse there), so `each` could only silently do nothing.
A missing name is a clean "no method" error instead of a C link error —
write a loop instead.

## Fallible builtins

`read_json|path|` and `write_json|path, data|` return `(value, error)`
with `null` for ok — the `(T, err)` convention every fallible function
should follow. See `docs/ERROR_HANDLING.md`.
