

```ahoy
example_string : "Hello, Ahoy!"

? Get the length of the string
length : example_string.length||

? Convert the string to uppercase
uppercased : example_string.upper||

? Convert the string to lowercase
lowercased : example_string.lower||

? Replace a substring
replaced : example_string.replace|"Ahoy", "World"|

? Check if the string contains a substring
contains_ahoy : example_string.contains|"Ahoy"|

? Print results
print|f"Original String: {example_string}\n"|

? camelCase method example
camel_case_string : "this is an example".camel_case||

? snake_case method example
snake_case_string : "This Is An Example".snake_case||
print|f"Snake Case: {snake_case_string}\n"|

? pascal case method example
pascal_case_string : "this is an example".pascal_case||
Print|f"Pascal Case: {pascal_case_string}\n"|


? kebab case method example
kebab_case_string : "This Is An Example".kebab_case||

? match string
matched : example_string.match|"^Hello"||

? join string array
joined_string : ["Join", "these", "words"].join|" "|

? split string
split_string : example_string.split|", "|  # Splits into ["Hello", " Ahoy!"]

? count occurrences of a character
count_l : example_string.count|"l"||

? lpad a string
lpad_string : "42".lpad|5, "0"||  # Results in "00042"

? rpad a string
rpad_string : "42".rpad|5, "0"||  # Results in "42000"

? pad a string both sides
padded_string : "42".pad|6, "*"||  # Results in "**42**"

? strip trims string
trimmed_string : "   padded string   ".strip||

? get_file|| If the string is a valid file path, returns the file name, including the extension.
files_name : "/path/to/file.txt".get_file||

```
