


## Switch Statement Syntax

### Block Terminator Rules

**One-line switch with `then...end` does NOT require `$`:**
```ahoy
switch value then case1:action1| case2:action2| end
```

**Multi-line switch with `on` REQUIRES `$`:**
```ahoy
switch value on
    case1: action1|
    case2: action2|
$
```

## Examples

```ahoy
? Switch statements (single-line format) - no $ needed
? day: 3
? switch day then 1:ahoy|"5. Monday\n"| 2:ahoy|"5. Tuesday\n"| 3:ahoy|"5. Wednesday (switch works!)\n"|end

? . Switch with chars
testfunc ::|test| void: return print|"boris"|

grade: 'B'
switch grade:
	on 'A':ahoy|"6. Excellent!\n"|
	on 'B':ahoy|"6. Good job! (char switch works!)\n"|
  on 'C':ahoy|"6. Average\n"|
	_: ahoy|"6. Needs improvement\n"|
$

? 6. Switch with multiple cases
grade: 'B'
switch grade on
	'A','B':ahoy|"6. Excellent!\n"|
	'C','D':ahoy|" Average\n"|
	_: ahoy|"Needs improvement\n"|
	loop i:1 to 3 do print|'hello'|
$


```
