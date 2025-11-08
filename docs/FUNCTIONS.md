

## Function Declaration Syntax

### Block Terminator Rule
**Multi-line function bodies REQUIRE `$` terminator.**

## Example of function declaration with parameters and multiple return type
```ahoy
func_name :: |name:string, phone:int| string, int:
	print|"Name: %s, Phone: %d\n", name, phone|;
	return name, phone
$
```

## store function with multiple returns
bob, bobs_phone: func_name|"Bob", 1234567890|


## feature list:
- function hoisting like JavaScript (can call functions before declaration )
