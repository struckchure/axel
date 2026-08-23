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
        $.extension_definition,
        $.global_definition,
        $.function_definition,
        $.type_definition,
      ),

    // use extension 'unaccent';
    extension_definition: ($) =>
      seq("use", "extension", field("name", $.string), ";"),

    // global current_user: uuid;  /  global required tz: str;
    global_definition: ($) =>
      seq(
        "global",
        optional("required"),
        field("name", $.field_identifier),
        ":",
        field("type", $.type_identifier),
        ";",
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
        $.policy,
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

    // rewrite update := datetime_current();
    // rewrite create, update := slugify(__new__.title);
    // rewrite insert, update := __subject__.name;
    rewrite: ($) =>
      seq(
        "rewrite",
        field("event", $.identifier),
        repeat(seq(",", field("event", $.identifier))),
        ":=",
        choice(
          seq($.identifier, "(", optional(seq($._rewrite_arg, repeat(seq(",", $._rewrite_arg)))), ")"),
          seq($.identifier, optional(seq(".", field("field", $.identifier)))),
          $.string,
          $.integer,
        ),
        optional(";"),
      ),

    // A rewrite call argument: a row reference (__new__.field) or a literal.
    _rewrite_arg: ($) =>
      choice(
        seq($.identifier, ".", field("field", $.identifier)),
        $.string,
        $.integer,
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
    // computed total := (.quantity * .unit_price) - .discount;
    computed_field: ($) =>
      seq(
        "computed",
        field("name", $.field_identifier),
        ":=",
        repeat1($._computed_token),
        ";",
      ),

    _computed_token: ($) =>
      choice(
        $.identifier,
        $.string,
        $.float,
        $.integer,
        ".",
        "??",
        "+",
        "-",
        "*",
        "/",
        "(",
        ")",
        ",",
        "<",
        ">",
        "=",
        "!=",
      ),

    // constraint exclusive on (.email, .tenant_id);
    // constraint exclusive on (.name, .actor) filter .status = Status.Pending;
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
        optional(seq("filter", field("filter", $.filter_predicate))),
        ";",
      ),

    // An unparenthesized predicate that runs to the statement's `;`. Not parsed
    // structurally — this just brackets the span; the compiler parses the AQL.
    filter_predicate: ($) => repeat1($._filter_token),

    _filter_token: ($) =>
      choice(
        $.aql_block,
        $.dollar_string,
        $.identifier,
        $.string,
        $.float,
        $.integer,
        ".",
        ",",
        ":=",
        "??",
        "{",
        "}",
        ":",
        "$",
        "*",
        "+",
        "-",
        "/",
        "=",
        "!=",
        "<",
        ">",
        "<=",
        ">=",
        "@",
        "?",
        "|",
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

    // policy hide_expired for select using ( .expires_at >= now() );
    // policy owner_all for all to app_user using ( … ) with check ( … );
    // The predicate is a balanced parenthesized SQL span (reuses sql_paren).
    policy: ($) =>
      seq(
        "policy",
        field("name", $.field_identifier),
        "for",
        field("command", $.identifier),
        repeat(seq(",", field("command", $.identifier))),
        optional(
          seq("to", field("role", $.identifier), repeat(seq(",", field("role", $.identifier)))),
        ),
        optional(seq("using", field("using", $.aql_block))),
        optional(seq("with", "check", field("check", $.aql_block))),
        ";",
      ),

    // @language plpgsql @immutable  (decorator-style, above the declaration)
    // function name(params) -> ret[] { return <sql-expr>; };
    function_definition: ($) =>
      seq(
        repeat($.function_directive),
        "function",
        field("name", $.field_identifier),
        "(",
        optional(
          seq($.function_param, repeat(seq(",", $.function_param))),
        ),
        ")",
        "->",
        field("returns", $.type_identifier),
        optional($.array_marker),
        "{",
        "return",
        field("body", $.return_expression),
        ";",
        "}",
        optional(";"),
      ),

    // @name value?  — an attribute like @immutable / @language plpgsql / @parallel safe.
    function_directive: ($) =>
      seq(
        "@",
        field("name", $.identifier),
        optional(field("value", choice($.identifier, $.string, $.integer))),
      ),

    function_param: ($) =>
      seq(
        field("name", $.identifier),
        ":",
        field("type", $.type_identifier),
        optional($.array_marker),
      ),

    array_marker: ($) => seq("[", "]"),

    // A raw Postgres expression, bracketed for the editor but not parsed
    // structurally (the compiler wraps it in BEGIN RETURN …; END;).
    return_expression: ($) => repeat1($._sql_token),

    _sql_token: ($) =>
      choice(
        $.sql_paren,
        $.dollar_string,
        $.inline_aql,
        $.identifier,
        $.string,
        $.integer,
        ".", ",", ":", "::", "||",
        "+", "-", "*", "/", "%", "^", "&", "~",
        "=", "!=", "<", ">", "<=", ">=",
        "[", "]", "@", "?", "|",
      ),

    sql_paren: ($) => seq("(", repeat($._sql_token), ")"),

    // An inline AQL query inside a raw SQL expression: aql`select … `. The
    // compiler compiles it and inlines the SQL as a string literal. aql_source is
    // its own node so injections.scm can re-parse just the interior as AQL.
    inline_aql: ($) => seq("aql", "`", optional($.aql_source), "`"),

    aql_source: ($) => token.immediate(/[^`]+/),

    // A balanced parenthesized AQL body. Not parsed structurally — this just
    // brackets the span for the editor; the compiler parses the real AQL.
    aql_block: ($) => seq("(", repeat($._aql_token), ")"),

    _aql_token: ($) =>
      choice(
        $.aql_block,
        $.dollar_string,
        $.identifier,
        $.string,
        $.float,
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
        "+",
        "-",
        "/",
        "=",
        "!=",
        "<",
        ">",
        "<=",
        ">=",
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

    float: ($) => /[0-9]+\.[0-9]+/,

    integer: ($) => /[0-9]+/,

    comment: ($) => token(seq("#", /.*/)),
  },
});
