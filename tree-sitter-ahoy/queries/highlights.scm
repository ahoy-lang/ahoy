; Keywords

[
  "if"
  "then"
  "else"
  "elseif"
  "anif"
  "loop"
  "to"
  "in"
  "switch"
  "func"
  "return"
  "import"
  "when"
] @keyword

; Function calls

(call_expression
  function: (identifier) @function)

; Function definitions

(function_declaration
  name: (identifier) @function)

; Built-in functions

(call_expression
  function: (identifier) @function.builtin
  (#match? @function.builtin "^(ahoy|print)$"))

; Types

(type) @type

[
  "int"
  "float"
  "string"
  "bool"
  "dict"
] @type.builtin

; Variables

(identifier) @variable

(parameter
  name: (identifier) @variable.parameter)

(variable_declaration
  name: (identifier) @variable)

; Operators

[
  "plus"
  "minus"
  "times"
  "div"
  "mod"
  "greater_than"
  "lesser_than"
  "is"
  "and"
  "or"
  "not"
  "+"
  "-"
  "*"
  "/"
  "%"
  ">"
  "<"
  ">="
  "<="
] @operator

; Punctuation

[
  ":"
  ";"
  ","
  "|"
] @punctuation.delimiter

[
  "("
  ")"
  "{"
  "}"
  "<"
  ">"
] @punctuation.bracket

; Literals

(string) @string
(char) @string.special
(number) @number
(boolean) @constant.builtin

; Comments

(comment) @comment

; Imports

(import_statement
  path: (string) @string.special)
