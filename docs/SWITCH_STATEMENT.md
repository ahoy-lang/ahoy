


```ahoy
? . Switch statements (single-line format)
day: 3
switch day then 1:ahoy|"5. Monday\n"| 2:ahoy|"5. Tuesday\n"| 3:ahoy|"5. Wednesday (switch works!)\n"|

? . Switch with chars
grade: 'B'
switch grade then
	'A':ahoy|"6. Excellent!\n"|
	'B':ahoy|"6. Good job! (char switch works!)\n"|
  'C':ahoy|"6. Average\n"|
	_: ahoy|"6. Needs improvement\n"|

? 6. Switch with multiple cases
grade: 'B'
switch grade then
	'A','B':ahoy|"6. Excellent!\n"|
	'C','D':ahoy|" Average\n"|
	_: ahoy|"Needs improvement\n"|
```
