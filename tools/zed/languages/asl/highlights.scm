; Comments
(comment) @comment

; Keywords
[
  "scalar"
  "type"
  "model"
  "enum"
  "abstract"
  "extends"
  "extending"
  "required"
  "multi"
  "single"
  "property"
  "link"
  "constraint"
  "index"
  "on"
  "computed"
  "default"
  "func"
] @keyword

; Type names
(type_identifier) @type
(enum_value) @constant

; Field / property names
(field_identifier) @property

; Constraint names (exclusive, pk, min_length, ...)
(field_constraint name: (identifier) @function)
(constraint name: (identifier) @function)

; Literals
(string) @string
(integer) @number

; Built-in scalar types
((type_identifier) @type.builtin
  (#any-of? @type.builtin
    "str" "int16" "int32" "int64" "float32" "float64"
    "bool" "uuid" "datetime" "date" "time" "json" "bytes" "decimal"))

; Default functions and boolean literals (bare identifiers in a default)
((default (identifier) @function.builtin)
  (#any-of? @function.builtin
    "gen_uuid" "gen_random_uuid" "now" "datetime_current"))
((default (identifier) @boolean)
  (#any-of? @boolean "true" "false"))

; Operators
[
  ":="
  "??"
  "@"
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
