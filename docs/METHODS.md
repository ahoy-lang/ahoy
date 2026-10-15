# Methods

User-defined methods use Go-style receivers. A method is an ordinary function
whose first parameter is the receiver; `recv.m|args|` calls the mangled
`Type_m` function with the receiver prepended. Associated functions
(`@ T.name`, no receiver) are called as `Type.name|args|`.

```ahoy
struct Point:
    x: float
    y: float
$

? Associated function (constructor): no receiver.
@ Point.make |x:float, y:float| Point:
    return Point{x: x, y: y}
$

? Method: the receiver is parameter zero.
@ (p Point) move |dx:float, dy:float| Point:
    p.x = p.x + dx
    p.y = p.y + dy
    return p
$

? Zero-argument method.
@ (p Point) sum || float:
    return p.x + p.y
$

p: Point.make|1.0, 2.0|
q: p.move|3.0, 4.0|
s: q.sum||
```

## Rules

- The receiver type is written explicitly: `@ (p Point)`, `@ (s array[Box])`.
  Inside the body it behaves exactly like a parameter (including its type).
- The receiver (or associated target) must name a struct, an enum, a builtin
  scalar (`int`, `float`, `string`, …), a builtin nominal type (`pool`,
  `job`), a compound builtin shape (`array[Node]`, `dict<string,int>`), or an
  imported C struct. Anything else is a compile error at the definition.
- Method functions live in the global namespace under their mangled name
  (`Point_move`, `array_Box_total_area`), so they can also be called directly
  and referenced as `&Point_move` for callbacks. Redefining a mangled name —
  by another method, an associated function, or a free function — is an error.
- Calls resolve in this order: struct callback fields, user methods,
  C namespaces, builtins. An unknown method on a type that has methods (or on
  any known struct) is a compile error listing the available methods:
  `Type 'Point' has no method 'nudge' (methods: move, sum)`.
- Arity, defaults and named arguments are checked with the receiver counted
  as the first argument.

## Value receivers vs handle receivers

Methods add no new semantics: a value receiver is a by-value parameter.

- Struct values: return the new value (`n = n.moved|dx, dy|`, then
  `nodes[id] = n`). A `void` method cannot mutate the caller's copy.
- Handles (arrays, pools, …) mutate in place:
  `nodes.set_pos|id, 100.0, 200.0|` works because the array is shared.
- Array elements resolve through the element type, so `boxes[0].move|…|`
  validates when `boxes` is `array[Box]`.

## Limits

- Inside array/object/dict literals the comma rules are unchanged; a method
  call itself is a normal expression and works anywhere one does (except as
  another call's argument — name the result first, like any call).
- A method and a same-named struct field cannot both be called with pipes:
  the field wins, as before. Bare `p.move` (no pipes) is still field access.
- Cross-file programs validate method calls against sibling files' methods,
  the way free functions validate against sibling names.

## Methods as callbacks

A method reference (`&Point_move`) fits any `fn`-typed slot whose signature
it matches, like any function reference. References are checked where they
flow into typed slots — struct literal fields, declared fn variables, and
call arguments into fn-typed parameters.

A function taking fewer parameters than the slot passes is only warned
about (extra arguments are ignored); taking more, or a return mismatch, is
an error. Struct literals must also set every fn-typed field — a missing one
would be a null call.
