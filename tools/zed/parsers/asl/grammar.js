/**
 * Tree-sitter grammar for ASL (Axel Schema Language).
 *
 * Mirrors the participle grammar in core/asl/ast.go. The grammar stays generic
 * (type_identifier / field_identifier wrappers + plain identifier); highlights.scm
 * picks out built-in scalar types, default functions and true/false with predicates.
 */

module.exports = grammar({
  name: "asl",

  word: ($) => $.identifier,

  extras: ($) => [/\s/, $.comment],

  rules: {
    source_file: ($) => repeat($._definition),

    _definition: ($) =>
      choice(
        $.scalar_type,
        $.enum_type,
        $.function_definition,
        $.type_definition,
      ),

    // scalar type EmailStr extending str;
    scalar_type: ($) =>
      seq(
        "scalar",
        "type",
        field("name", $.type_identifier),
        "extending",
        field("base", $.type_identifier),
        ";",
      ),

    // enum Role { Admin, Member, Guest }
    enum_type: ($) =>
      seq(
        "enum",
        field("name", $.type_identifier),
        "{",
        optional(
          seq($.enum_value, repeat(seq(",", $.enum_value)), optional(",")),
        ),
        "}",
      ),

    enum_value: ($) => $.identifier,

    // [abstract] (type|model) Name [(extends|extending) A, B] { members }
    type_definition: ($) =>
      seq(
        optional("abstract"),
        choice("type", "model"),
        field("name", $.type_identifier),
        optional(
          seq(
            choice("extends", "extending"),
            field("parent", $.type_identifier),
            repeat(seq(",", field("parent", $.type_identifier))),
          ),
        ),
        "{",
        repeat($._member),
        "}",
      ),

    _member: ($) =>
      choice(
        $.computed_field,
        $.index,
        $.constraint,
        $.trigger,
        $.field_declaration,
      ),

    // required multi link author: User { ... };
    field_declaration: ($) =>
      seq(
        optional("required"),
        optional(choice("multi", "single")),
        optional(choice("property", "link")),
        field("name", $.field_identifier),
        optional($.type_annotation),
        optional(seq("{", repeat($._field_body_item), "}")),
        ";",
      ),

    type_annotation: ($) => seq(":", field("type", $.type_identifier)),

    _field_body_item: ($) =>
      choice($.field_constraint, $.rewrite, $.default, $.on_clause),

    // rewrite update := datetime_current();  /  rewrite insert, update := __subject__.name;
    rewrite: ($) =>
      seq(
        "rewrite",
        field("event", $.identifier),
        repeat(seq(",", field("event", $.identifier))),
        ":=",
        choice(
          seq($.identifier, "(", ")"),
          seq($.identifier, optional(seq(".", field("field", $.identifier)))),
          $.string,
          $.integer,
        ),
        optional(";"),
      ),

    // constraint exclusive;  /  constraint min_length(10);
    field_constraint: ($) =>
      seq(
        "constraint",
        field("name", $.identifier),
        optional(seq("(", $._arg, repeat(seq(",", $._arg)), ")")),
        optional(";"),
      ),

    _arg: ($) => choice($.identifier, $.integer, $.string),

    // default := gen_uuid();  /  default := Role.Admin;  /  default := 'n/a';
    // default @func(name);    /  default true;
    default: ($) =>
      seq(
        "default",
        choice(
          seq(":=", $._new_default),
          seq("@", "func", "(", $.identifier, ")"),
          $._literal,
        ),
        optional(";"),
      ),

    _new_default: ($) =>
      choice(
        seq($.identifier, "(", ")"),
        seq($.identifier, ".", $.identifier),
        $._literal,
      ),

    _literal: ($) => choice($.string, $.integer, $.identifier),

    // on id;
    on_clause: ($) => seq("on", field("field", $.identifier), optional(";")),

    // computed display_name := .name ?? .email;
    computed_field: ($) =>
      seq(
        "computed",
        field("name", $.field_identifier),
        ":=",
        repeat1(
          choice($.field_identifier, ".", "??", $.string, $.integer),
        ),
        ";",
      ),

    // constraint exclusive on (.email, .tenant_id);
    constraint: ($) =>
      seq(
        "constraint",
        field("name", $.identifier),
        optional(seq("(", $._arg, repeat(seq(",", $._arg)), ")")),
        "on",
        "(",
        $._field_ref,
        repeat(seq(",", $._field_ref)),
        ")",
        ";",
      ),

    // index on (.email, .age);
    index: ($) =>
      seq(
        "index",
        "on",
        "(",
        $._field_ref,
        repeat(seq(",", $._field_ref)),
        ")",
        ";",
      ),

    _field_ref: ($) => seq(".", field("field", $.field_identifier)),

    // trigger audit after insert, update do ( … );
    // trigger touch before update execute my_fn();
    trigger: ($) =>
      seq(
        "trigger",
        field("name", $.field_identifier),
        field("timing", choice("before", "after")),
        field("event", $.identifier),
        repeat(seq(",", field("event", $.identifier))),
        optional(seq("for", "each", choice("row", "statement"))),
        optional(seq("when", "(", $.dollar_string, ")")),
        choice(
          seq("do", $.aql_block),
          seq("execute", field("function", $.identifier), "(", ")"),
        ),
        ";",
      ),

    // function name(params) -> ret { language := …; body := ( aql ) | $$ sql $$; };
    function_definition: ($) =>
      seq(
        "function",
        field("name", $.field_identifier),
        "(",
        optional(
          seq($.function_param, repeat(seq(",", $.function_param))),
        ),
        ")",
        "->",
        field("returns", $.type_identifier),
        "{",
        repeat($._function_item),
        "}",
        optional(";"),
      ),

    function_param: ($) =>
      seq(field("name", $.identifier), ":", field("type", $.type_identifier)),

    _function_item: ($) =>
      choice(
        seq("language", ":=", field("language", $.identifier), optional(";")),
        seq("body", ":=", choice($.dollar_string, $.aql_block), optional(";")),
      ),

    // A balanced parenthesized AQL body. Not parsed structurally — this just
    // brackets the span for the editor; the compiler parses the real AQL.
    aql_block: ($) => seq("(", repeat($._aql_token), ")"),

    _aql_token: ($) =>
      choice(
        $.aql_block,
        $.dollar_string,
        $.identifier,
        $.string,
        $.integer,
        ".",
        ",",
        ":=",
        "??",
        "{",
        "}",
        ":",
        ";",
        "$",
        "*",
        "=",
        "!=",
        "<",
        ">",
        "@",
        "?",
        "|",
      ),

    // Postgres dollar-quoting: $$ … $$. The body may contain single '$' but not
    // the '$$' terminator.
    dollar_string: ($) => token(seq("$$", /([^$]|\$[^$])*/, "$$")),

    // Specialised identifiers (distinct nodes for highlighting).
    type_identifier: ($) => $.identifier,
    field_identifier: ($) => $.identifier,

    identifier: ($) => /[a-zA-Z_][a-zA-Z0-9_]*/,

    string: ($) => /'[^']*'/,

    integer: ($) => /[0-9]+/,

    comment: ($) => token(seq("#", /.*/)),
  },
});
