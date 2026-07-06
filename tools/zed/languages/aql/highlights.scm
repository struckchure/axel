; Comments
(comment) @comment

; Statement keywords
[
  "multi"
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

; Query parameters ($name, $name<type>). The <type> annotation's name is a
; type_identifier (highlighted as @type above); its "<" ">" fall through to the
; operator rule below.
(parameter "$" @punctuation.special)
(parameter name: (identifier) @variable.parameter)

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
