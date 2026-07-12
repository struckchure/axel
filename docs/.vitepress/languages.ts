// TextMate grammars for Axel's Schema (ASL) and Query (AQL) languages, so Shiki
// can syntax-highlight ```asl and ```aql fenced blocks in the docs. These mirror
// the Tree-sitter highlight queries under tools/zed/languages/<lang>/highlights.scm.
// Shiki uses TextMate grammars (not Tree-sitter), so the token vocabulary is kept
// in sync by hand — update both when the languages gain keywords.
import type { LanguageRegistration } from "@shikijs/types";

export const asl: LanguageRegistration = {
  name: "asl",
  scopeName: "source.asl",
  patterns: [
    { include: "#comment" },
    { include: "#string" },
    { include: "#number" },
    { include: "#keyword" },
    { include: "#builtin-type" },
    { include: "#constant" },
    { include: "#builtin-func" },
    { include: "#func-call" },
    { include: "#type-name" },
    { include: "#operator" },
  ],
  repository: {
    comment: {
      match: "#.*$",
      name: "comment.line.number-sign.asl",
    },
    string: {
      begin: "'",
      end: "'",
      name: "string.quoted.single.asl",
    },
    number: {
      match: "\\b\\d+(?:\\.\\d+)?\\b",
      name: "constant.numeric.asl",
    },
    keyword: {
      match:
        "\\b(scalar|type|model|enum|abstract|extends|extending|required|multi|single|property|link|constraint|index|on|computed|default|func)\\b",
      name: "keyword.declaration.asl",
    },
    "builtin-type": {
      match:
        "\\b(str|int16|int32|int64|float32|float64|bool|uuid|datetime|date|time|json|bytes|decimal)\\b",
      name: "support.type.builtin.asl",
    },
    constant: {
      match: "\\b(true|false)\\b",
      name: "constant.language.asl",
    },
    "builtin-func": {
      match: "\\b(gen_uuid|gen_random_uuid|now|datetime_current)\\b",
      name: "support.function.builtin.asl",
    },
    "func-call": {
      // constraint/computed calls: min_length(10), max_length(100), gen_uuid()
      match: "\\b([a-z_][A-Za-z0-9_]*)(?=\\s*\\()",
      name: "support.function.asl",
    },
    "type-name": {
      // ASL type identifiers are capitalized: Base, User, Post
      match: "\\b[A-Z][A-Za-z0-9_]*\\b",
      name: "entity.name.type.asl",
    },
    operator: {
      match: ":=|\\?\\?|@",
      name: "keyword.operator.asl",
    },
  },
};

export const aql: LanguageRegistration = {
  name: "aql",
  scopeName: "source.aql",
  patterns: [
    { include: "#comment" },
    { include: "#directive" },
    { include: "#param" },
    { include: "#string" },
    { include: "#number" },
    { include: "#keyword" },
    { include: "#operator-word" },
    { include: "#constant" },
    { include: "#builtin-func" },
    { include: "#func-call" },
    { include: "#type-name" },
    { include: "#splat" },
    { include: "#operator" },
  ],
  repository: {
    comment: {
      match: "#.*$",
      name: "comment.line.number-sign.aql",
    },
    directive: {
      // @name / @request / @response before a query
      match: "(@)(name|request|response)\\b",
      captures: {
        1: { name: "keyword.operator.directive.aql" },
        2: { name: "keyword.control.directive.aql" },
      },
    },
    param: {
      // $email, $limit, ...
      match: "(\\$)([A-Za-z_][A-Za-z0-9_]*)",
      captures: {
        1: { name: "keyword.operator.parameter.aql" },
        2: { name: "variable.parameter.aql" },
      },
    },
    string: {
      begin: "'",
      end: "'",
      name: "string.quoted.single.aql",
    },
    number: {
      match: "\\b\\d+(?:\\.\\d+)?\\b",
      name: "constant.numeric.aql",
    },
    keyword: {
      match:
        "\\b(multi|select|insert|update|delete|filter|order|by|limit|offset|set)\\b",
      name: "keyword.control.aql",
    },
    "operator-word": {
      match: "\\b(and|or|in|like|ilike|asc|desc)\\b",
      name: "keyword.operator.word.aql",
    },
    constant: {
      match: "\\b(true|false|null)\\b",
      name: "constant.language.aql",
    },
    "builtin-func": {
      match: "\\b(count)(?=\\s*\\()",
      name: "support.function.builtin.aql",
    },
    "func-call": {
      match: "\\b([a-z_][A-Za-z0-9_]*)(?=\\s*\\()",
      name: "support.function.aql",
    },
    "type-name": {
      // AQL type identifiers are capitalized: User, Post
      match: "\\b[A-Z][A-Za-z0-9_]*\\b",
      name: "entity.name.type.aql",
    },
    splat: {
      match: "\\*",
      name: "keyword.operator.splat.aql",
    },
    operator: {
      match: ":=|\\?\\?|!=|<=|>=|=|<|>",
      name: "keyword.operator.aql",
    },
  },
};
