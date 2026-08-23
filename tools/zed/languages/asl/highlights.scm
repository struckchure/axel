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
  "sql"
  "as"
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
(float) @number

; Built-in scalar types
((type_identifier) @type.builtin
  (#any-of? @type.builtin
    "str" "int16" "int32" "int64" "float32" "float64"
    "bool" "uuid" "datetime" "date" "time" "json" "jsonb" "bytes" "decimal"))

; Default and SQL functions and boolean literals
((default (identifier) @function.builtin)
  (#any-of? @function.builtin
    "gen_uuid" "gen_random_uuid" "now" "datetime_current"
    "ST_Distance" "ST_3DDistance" "ST_DWithin" "ST_MakePoint" "ST_SetSRID"
    "ST_GeogFromText" "ST_GeomFromText" "ST_Y" "ST_X" "ST_Z" "ST_M" "haversine"))
((default (identifier) @boolean)
  (#any-of? @boolean "true" "false"))

; Operators
[
  ":="
  "??"
  "@"
  "="
  "!="
  "<"
  ">"
  "<="
  ">="
  "+"
  "-"
  "*"
  "/"
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
