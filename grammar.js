/**
 * @file Axel is a modern database tool primarily designed for Go, with multi-language support.
 * @author Mohammed Al-Ameen <ameenmohammed2311@gmail.com>
 * @license MIT
 */

/// <reference types="tree-sitter-cli/dsl" />
// @ts-check

module.exports = grammar({
  name: "axel",

  extras: ($) => [/\s/, $.comment],

  word: ($) => $.identifier,

  rules: {
    source_file: ($) => repeat($._definition),

    _definition: ($) => choice($.abstract_model, $.model, $.comment),

    comment: ($) => token(choice(seq("#", /.*/), seq("//", /.*/))),

    // Models
    abstract_model: ($) =>
      seq(
        "abstract",
        "model",
        field("name", $.identifier),
        field("body", $.model_body)
      ),

    model: ($) =>
      seq(
        "model",
        field("name", $.identifier),
        optional($.extends_clause),
        field("body", $.model_body)
      ),

    extends_clause: ($) =>
      seq("extends", $.identifier, repeat(seq(",", $.identifier))),

    model_body: ($) => seq("{", repeat($._model_member), "}"),

    _model_member: ($) => choice($.field_declaration, $.trigger),

    // Field declaration (covers both properties and links)
    field_declaration: ($) =>
      seq(
        optional($.cardinality),
        field("name", $.identifier),
        ":",
        field("type", $.type_expr),
        optional($.field_body),
        ";"
      ),

    cardinality: ($) =>
      choice(
        "required",
        "optional",
        "multi",
        seq("required", "multi"),
        seq("multi", "required"),
        "single",
        seq("required", "single"),
        seq("single", "required")
      ),

    field_body: ($) => seq("{", repeat($._field_attribute), "}"),

    _field_attribute: ($) =>
      choice($.default_clause, $.constraint, $.on_clause),

    default_clause: ($) => seq("default", $.expression, ";"),

    constraint: ($) =>
      seq(
        "constraint",
        field("name", $.identifier),
        optional($.argument_list),
        ";"
      ),

    on_clause: ($) => seq("on", $.identifier, ";"),

    // Triggers
    trigger: ($) =>
      seq(
        "@",
        field("timing", choice("before", "after")),
        field("event", $.identifier),
        field("name", $.identifier),
        $.trigger_body,
        ";"
      ),

    trigger_body: ($) => seq("{", repeat($.assignment), "}"),

    assignment: ($) => seq($.expression, ":=", $.expression, ";"),

    // Type expressions
    type_expr: ($) => choice($.identifier, $.builtin_type),

    builtin_type: ($) =>
      choice(
        "str",
        "int16",
        "int32",
        "int64",
        "float32",
        "float64",
        "bool",
        "uuid",
        "datetime",
        "json",
        "bytes"
      ),

    // Expressions
    expression: ($) =>
      choice(
        $.identifier,
        $.member_expression,
        $.function_call,
        $.string_literal,
        $.number_literal,
        $.boolean_literal
      ),

    member_expression: ($) =>
      prec.left(
        2,
        seq(field("object", $.expression), ".", field("property", $.identifier))
      ),

    function_call: ($) => seq("@func", "(", field("name", $.identifier), ")"),

    argument_list: ($) =>
      seq(
        "(",
        optional(
          seq($.expression, repeat(seq(",", $.expression)), optional(","))
        ),
        ")"
      ),

    // Literals
    string_literal: ($) =>
      token(
        choice(
          seq('"', repeat(choice(/[^"\\]/, /\\./)), '"'),
          seq("'", repeat(choice(/[^'\\]/, /\\./)), "'")
        )
      ),

    number_literal: ($) =>
      token(seq(optional("-"), /\d+/, optional(seq(".", /\d+/)))),

    boolean_literal: ($) => choice("true", "false"),

    identifier: ($) => /[a-zA-Z_][a-zA-Z0-9_]*/,
  },
});
