/**
 * Tree-sitter grammar for AQL (Axel Query Language).
 *
 * Mirrors the participle grammar in core/aql/ast.go. type_identifier /
 * field_identifier wrappers are used only in unambiguous keyword-led positions;
 * expression identifiers stay plain so one-token lookahead disambiguates
 * function calls / qualified names / bare identifiers.
 */

module.exports = grammar({
  name: "aql",

  word: ($) => $.identifier,

  extras: ($) => [/\s/, $.comment],

  rules: {
    source_file: ($) => repeat($._statement),

    _statement: ($) =>
      choice(
        $.select_statement,
        $.insert_statement,
        $.update_statement,
        $.delete_statement,
      ),

    // select ( count(...) | Type shape? filter? order? limit? offset? ) ;?
    select_statement: ($) =>
      seq(
        optional("multi"),
        "select",
        choice($.aggregate, $._object_select),
        optional(";"),
      ),

    _object_select: ($) =>
      seq(
        field("type", $.type_identifier),
        optional($.shape),
        optional($.filter),
        optional($.order_by),
        optional($.limit_clause),
        optional($.offset_clause),
      ),

    // count(User filter .active = true)
    aggregate: ($) =>
      seq(
        field("function", $.identifier),
        "(",
        field("type", $.type_identifier),
        optional($.filter),
        ")",
      ),

    // insert Type { field := expr, ... } ;?
    insert_statement: ($) =>
      seq(
        "insert",
        field("type", $.type_identifier),
        $.assignment_block,
        optional(";"),
      ),

    // update Type filter? set { ... } ;?
    update_statement: ($) =>
      seq(
        "update",
        field("type", $.type_identifier),
        optional($.filter),
        "set",
        $.assignment_block,
        optional(";"),
      ),

    // delete Type filter? ;?
    delete_statement: ($) =>
      seq(
        "delete",
        field("type", $.type_identifier),
        optional($.filter),
        optional(";"),
      ),

    assignment_block: ($) =>
      seq(
        "{",
        optional(
          seq($.assignment, repeat(seq(",", $.assignment)), optional(",")),
        ),
        "}",
      ),

    assignment: ($) =>
      seq(field("name", $.field_identifier), ":=", field("value", $.expression)),

    // { id, email, posts: { ... }, count := (...) }
    shape: ($) =>
      seq(
        "{",
        optional(
          seq($.shape_field, repeat(seq(",", $.shape_field)), optional(",")),
        ),
        "}",
      ),

    shape_field: ($) =>
      seq(
        field("name", $.field_identifier),
        optional(
          choice(
            seq(":", field("shape", $.shape)),
            seq(":=", field("value", $.expression)),
          ),
        ),
      ),

    filter: ($) => seq("filter", $.expression),

    order_by: ($) =>
      seq("order", "by", $.order_term, repeat(seq(",", $.order_term))),

    order_term: ($) =>
      seq($.expression, optional(choice("asc", "desc"))),

    limit_clause: ($) => seq("limit", $.expression),
    offset_clause: ($) => seq("offset", $.expression),

    // primary ( op primary )?
    expression: ($) =>
      seq($._primary, optional(seq($._binary_operator, $._primary))),

    _binary_operator: ($) =>
      choice(
        "!=",
        "<=",
        ">=",
        "=",
        "<",
        ">",
        "??",
        "and",
        "or",
        "in",
        "like",
        "ilike",
      ),

    _primary: ($) =>
      choice(
        $.subquery,
        $.insert_expression,
        $.parenthesized_expression,
        $.function_call,
        $.path,
        $.parameter,
        $.null,
        $.boolean,
        $.string,
        $.float,
        $.integer,
        $.qualified_identifier,
        $.identifier,
      ),

    subquery: ($) => seq("(", "select", $._object_select, ")"),

    insert_expression: ($) =>
      seq(
        "(",
        "insert",
        field("type", $.type_identifier),
        $.assignment_block,
        ")",
      ),

    parenthesized_expression: ($) => seq("(", $.expression, ")"),

    function_call: ($) =>
      seq(
        field("name", $.identifier),
        "(",
        optional(seq($.expression, repeat(seq(",", $.expression)))),
        ")",
      ),

    // .author.name
    path: ($) => repeat1(seq(".", field("step", $.field_identifier))),

    // $name, $name?, $name<type>, $name<type>?
    // prec.right so a "<" right after a parameter is greedily taken as the start
    // of a type annotation rather than reduced and treated as a binary operator.
    parameter: ($) =>
      prec.right(
        seq(
          "$",
          field("name", $.identifier),
          optional(seq("<", field("param_type", $.type_identifier), ">")),
          optional("?"),
        ),
      ),

    // User.id
    qualified_identifier: ($) =>
      seq(field("scope", $.identifier), ".", field("field", $.identifier)),

    null: ($) => "null",
    boolean: ($) => choice("true", "false"),

    // Specialised identifiers (distinct nodes for highlighting).
    type_identifier: ($) => $.identifier,
    field_identifier: ($) => $.identifier,

    identifier: ($) => /[a-zA-Z_][a-zA-Z0-9_]*/,

    string: ($) => /'[^']*'/,

    float: ($) => /[0-9]+\.[0-9]+/,

    integer: ($) => /[0-9]+/,

    comment: ($) => token(seq("#", /.*/)),
  },
});
