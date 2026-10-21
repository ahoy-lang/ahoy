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
