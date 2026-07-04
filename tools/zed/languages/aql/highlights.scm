; Comments
(comment) @comment

; Statement keywords
[
  "select"
  "insert"
  "update"
  "delete"
  "filter"
  "order"
  "by"
  "limit"
  "offset"
  "set"
] @keyword

; Operator keywords
[
  "and"
  "or"
  "in"
  "like"
  "ilike"
  "asc"
  "desc"
] @keyword.operator

; Type names
(type_identifier) @type
(qualified_identifier scope: (identifier) @type)

; Field / property names
(field_identifier) @property
(qualified_identifier field: (identifier) @property)

; Function and aggregate call names
(function_call name: (identifier) @function)
(aggregate function: (identifier) @function)
((aggregate function: (identifier) @function.builtin)
  (#eq? @function.builtin "count"))

; Query parameters ($name)
(parameter "$" @punctuation.special)
(parameter (identifier) @variable.parameter)

; Literals
(string) @string
(integer) @number
(float) @number
(null) @constant.builtin
(boolean) @boolean

; Operators
[
  "="
  "!="
  "<"
  ">"
  "<="
  ">="
  ":="
  "??"
] @operator

; Punctuation
[
  "{"
  "}"
  "("
  ")"
] @punctuation.bracket

[
  ";"
  ","
  ":"
  "."
] @punctuation.delimiter
