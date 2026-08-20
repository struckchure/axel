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
  "filter"
  "computed"
  "default"
  "func"
  "rewrite"
  "trigger"
  "function"
  "before"
  "after"
  "for"
  "each"
  "row"
  "statement"
  "when"
  "do"
  "execute"
  "use"
  "extension"
  "return"
  "policy"
  "using"
  "with"
  "check"
  "to"
  "global"
] @keyword

; Function attribute directives (@immutable, @language, @parallel, …)
(function_directive
  "@" @punctuation.special
  name: (identifier) @attribute)

; Dollar-quoted raw SQL bodies
(dollar_string) @string

; Inline AQL literals: aql`select …` — the tag reads as a keyword, the backticks
; as delimiters. The interior is highlighted by the AQL injection.
(inline_aql
  "aql" @keyword
  "`" @punctuation.delimiter)

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
    "bool" "uuid" "datetime" "date" "time" "json" "jsonb" "bytes" "decimal"))

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
