# Switch Statement

A switch compares one value against a list of cases. Every case starts with
`on` (except the default case, which is `_`), and the switch is closed with
`$` like any other block.

```ahoy
@ main || void:
  day: 2
  switch day:
    on 1: print|"Mon"|
    on 2: print|"Tue"|
    _: print|"Other"|
  $
$
```

## Case kinds

```ahoy
? Single values (numbers, strings, enum members)
switch grade:
  on 'A': print|"Excellent"|
  on Direction.UP: print|"Going up"|
  on .UP: print|"Going up"|
  _: print|"Other"|
$

? Multiple values on one case (comma-separated)
switch grade:
  on 'A', 'B': print|"Good"|
  on 'C': print|"OK"|
  _: print|"Poor"|
$

? Ranges with `to` (inclusive on both ends, first match wins)
switch x:
  on 1 to 5: print|"Small"|
  on 6 to 10: print|"Medium"|
  on 42: print|"Answer"|
  _: print|"Other"|
$

? String ranges compare lexicographically
switch c:
  on 'a' to 'z': print|"lower"|
  on 'A' to 'Z': print|"upper"|
  _: print|"other"|
$
```

Ranges and plain cases may be mixed freely, and cases are tried in source
order, so an earlier case wins over a later overlapping range.

## Switch as an expression

A switch can produce a value by assigning it. Each case body evaluates to its
last statement:

```ahoy
result: string = switch day:
  on 1: "Mon"
  on 2: "Tue"
  _: "Other"
$
```

A switch can also be returned directly:

```ahoy
@ name_for |day:int| string:
  return switch day:
    on 1: "Mon"
    _: "Other"
  $
$
```

## How it compiles

- Integer switches without ranges become a C `switch` with a `break` after
  every case.
- A switch containing any range becomes an `if` / `else if` / `else` chain
  (C labels can only test equality).
- String switches become `strcmp` chains; string ranges become a pair of
  `strcmp` bounds checks.

## Notes

- Close every switch with `$`, even a one-line switch:
  `switch x: on 1: print|"one"| _: print|"other"| $`
- There is no fallthrough: every case ends after its body.
- For an inline test (not a block) the same comma list reads
  `x is A, B` (`x is not A, B` for none-of); see the Comparison section in
  the README.
