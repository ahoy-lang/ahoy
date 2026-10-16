# Variable Declaration
### type is inferred by variable declaration by default

## variable decleration
count: 1

## const declaration
PI :: 3.14

## distinct newtypes
distinct NodeId: int

Same layout as the base, checked strictly: only the same distinct name,
a literal of the base kind, or an explicit `Name(value)` conversion flows
in. Anything else is a compile error.

a: NodeId = 5
b: NodeId = NodeId|7|
take_node|a|
$
