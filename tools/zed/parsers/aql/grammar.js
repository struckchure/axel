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

  // See _operand: `<` starts either a cast or a less-than comparison.
  conflicts: ($) => [[$._operand]],

  extras: ($) => [/\s/, $.comment],

  rules: {
    // A file is either a sequence of statements, or a single bare expression —
    // never both. The expression form lets an injected fragment (an RLS policy
    // predicate or a trigger `do (…)` body) highlight with this grammar without
    // colliding with the statement grammar (statements are keyword-led, so the
    // two alternatives are chosen by the first token).
    source_file: ($) =>
      optional(choice(repeat1(choice($.directive, $._statement)), $.expression)),

    // Leading metadata declaration: @name CreateUser / @request Foo / @response User
    directive: ($) =>
      seq(
        "@",
        field("name", $.identifier),
        field("value", choice($.type_identifier, $.string, $.integer)),
      ),

    _statement: ($) =>
      seq(
        repeat($.var_block),
        optional($.with_block),
        choice(
          $.select_statement,
          $.insert_statement,
          $.update_statement,
          $.delete_statement,
        ),
      ),

    var_block: ($) =>
      seq(
        "var",
        choice(
          seq(
            "(",
            repeat(seq($.parameter, optional(";"))),
            ")",
            optional(";"),
          ),
          seq($.parameter, ";"),
        ),
      ),

    // with ( name := (select ...); name := (multi select ...); )
    // Binds named subqueries for the statement that follows; each lowers to a CTE.
    with_block: ($) =>
      seq(
        "with",
        "(",
        repeat(seq($.with_binding, optional(choice(";", ",")))),
        ")",
        optional(";"),
      ),

    with_binding: ($) =>
      seq(field("name", $.identifier), ":=", field("value", $.expression)),

    // select ( count(...) | Type shape? filter? group? having? order? limit? offset? ) ;?
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
        optional($.group_by),
        optional($.having),
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

    // insert Type { field := expr, ... } [unless conflict [on ...] [else (...)]] ;?
    insert_statement: ($) =>
      seq(
        "insert",
        field("type", $.type_identifier),
        $.assignment_block,
        optional($.on_conflict),
        optional(";"),
      ),

    // unless conflict [on .field | on (.a, .b)] [else (update Type set { ... })]
    on_conflict: ($) =>
      seq(
        "unless",
        "conflict",
        optional($.conflict_target),
        optional($.conflict_update),
      ),

    conflict_target: ($) =>
      seq(
        "on",
        choice(
          seq(".", field("field", $.field_identifier)),
          seq(
            "(",
            ".",
            field("field", $.field_identifier),
            repeat(seq(",", ".", field("field", $.field_identifier))),
            ")",
          ),
        ),
      ),

    conflict_update: ($) =>
      seq(
        "else",
        "(",
        "update",
        field("type", $.type_identifier),
        "set",
        $.assignment_block,
        ")",
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
      seq(
        field("name", $.field_identifier),
        ":=",
        choice(
          field("value", $.expression),
          field("delta", $.link_delta),
        ),
      ),

    link_delta: ($) =>
      seq(
        "{",
        optional(
          seq($.link_delta_item, repeat(seq(",", $.link_delta_item)), optional(",")),
        ),
        "}",
      ),

    link_delta_item: ($) =>
      seq(
        field("op", choice($.string, "+", "-")),
        ":",
        field("value", $.expression),
      ),

    // { id, email, posts: { ... }, count := (...) }
    shape: ($) =>
      seq(
        "{",
        optional(
          seq($.shape_field, repeat(seq(",", $.shape_field)), optional(",")),
        ),
        "}",
      ),

    // "*" splat expands to all scalar props + single-link FK columns.
    // A computed value may carry a trailing per-field `filter` — an aggregate
    // field in an aggregation select, e.g. `total := sum(.amount) filter .x = 1`.
    shape_field: ($) =>
      choice(
        "*",
        seq(
          field("name", $.field_identifier),
          optional(
            choice(
              seq(":", field("shape", $.shape)),
              seq(
                ":=",
                field("value", $.expression),
                optional(field("agg_filter", $.filter)),
              ),
            ),
          ),
        ),
      ),

    filter: ($) => seq("filter", $.expression),

    group_by: ($) =>
      seq("group", "by", $.expression, repeat(seq(",", $.expression))),

    having: ($) => seq("having", $.expression),

    order_by: ($) =>
      seq("order", "by", $.order_term, repeat(seq(",", $.order_term))),

    order_term: ($) =>
      seq($.expression, optional(choice("asc", "desc"))),

    limit_clause: ($) => seq("limit", $.expression),
    offset_clause: ($) => seq("offset", $.expression),

    // and-group ( "or" and-group )*  —  `and` binds tighter than `or`.
    // Parenthesize (see parenthesized_expression) to group: (a or b) and c
    // The precedence levels are hidden rules, so an expression's children stay
    // flat: the operands and operators, in source order.
    expression: ($) =>
      seq($._and_expression, repeat(seq("or", $._and_expression))),

    _and_expression: ($) =>
      seq($._comparison, repeat(seq("and", $._comparison))),

    // primary ( op primary )?  |  primary is [not] null
    _comparison: ($) =>
      seq(
        $._operand,
        optional(choice(seq($._binary_operator, $._operand), $.null_test)),
      ),

    _binary_operator: ($) =>
      choice("!=", "<=", ">=", "=", "<", ">", "??", "in", "like", "ilike"),

    // postfix `is null` / `is not null`
    null_test: ($) => seq("is", optional("not"), "null"),

    // An operand is a primary with an optional trailing `<Type>` cast that applies
    // to any operand — a literal, path, (expr), or subquery projection.
    //
    // `<` is ambiguous here: `.age<int32>` is a cast but `.age < now()` is a
    // comparison, and one token of lookahead can't tell them apart. Resolving it
    // with precedence would swallow every `<` comparison whose right operand
    // starts with an identifier (`filter .expires_at < now()`), so both readings
    // are declared as a conflict instead and tree-sitter's GLR keeps whichever
    // one parses.
    _operand: ($) =>
      seq($._primary, optional(seq("<", field("cast", $.type_identifier), ">"))),

    _primary: ($) =>
      choice(
        $.subquery,
        $.insert_expression,
        $.parenthesized_expression,
        $.function_call,
        $.path,
        $.parameter,
        $.global,
        $.null,
        $.boolean,
        $.string,
        $.float,
        $.integer,
        $.qualified_identifier,
        $.identifier,
      ),

    // global reference: `global current_user`
    global: ($) => seq("global", field("name", $.identifier)),

    // An optional trailing `.field` projects a single column from the row. A
    // `<Type>` cast after it is captured by the enclosing operand.
    subquery: ($) =>
      seq(
        "(",
        optional("multi"),
        "select",
        $._object_select,
        ")",
        optional(seq(".", field("project", $.field_identifier))),
      ),

    insert_expression: ($) =>
      seq(
        "(",
        "insert",
        field("type", $.type_identifier),
        $.assignment_block,
        ")",
      ),

    // Parenthesized expression: (a ?? b). A trailing `<Type>` cast is captured by
    // the enclosing operand.
    parenthesized_expression: ($) => seq("(", $.expression, ")"),

    function_call: ($) =>
      seq(
        field("name", $.identifier),
        "(",
        optional(seq($.expression, repeat(seq(",", $.expression)))),
        ")",
      ),

    // .author.name — a trailing `<Type>` cast is captured by the enclosing operand.
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

    string: ($) => choice(/'[^']*'/, /"[^"]*"/),

    float: ($) => /[0-9]+\.[0-9]+/,

    integer: ($) => /[0-9]+/,

    comment: ($) => token(seq("#", /.*/)),
  },
});
