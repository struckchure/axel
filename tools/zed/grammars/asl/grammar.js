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
      choice($.scalar_type, $.enum_type, $.type_definition),

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
      choice($.computed_field, $.index, $.constraint, $.field_declaration),

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
      choice($.field_constraint, $.default, $.on_clause),

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

    // Specialised identifiers (distinct nodes for highlighting).
    type_identifier: ($) => $.identifier,
    field_identifier: ($) => $.identifier,

    identifier: ($) => /[a-zA-Z_][a-zA-Z0-9_]*/,

    string: ($) => /'[^']*'/,

    integer: ($) => /[0-9]+/,

    comment: ($) => token(seq("#", /.*/)),
  },
});
