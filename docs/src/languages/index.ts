import type { LanguageRegistration } from "@shikijs/types";

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
      match: "(@)(name|request|response)\\b",
      captures: {
        "1": { name: "keyword.operator.directive.aql" },
        "2": { name: "keyword.control.directive.aql" },
      },
    },
    param: {
      match: "(\\$)([A-Za-z_][A-Za-z0-9_]*)",
      captures: {
        "1": { name: "keyword.operator.parameter.aql" },
        "2": { name: "variable.parameter.aql" },
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
        "\\b(for|var|with|multi|select|insert|update|delete|filter|group|having|order|by|limit|offset|set|unless|conflict|on|else|global)\\b",
      name: "keyword.control.aql",
    },
    "operator-word": {
      match: "\\b(and|or|in|like|ilike|asc|desc|is|not)\\b",
      name: "keyword.operator.word.aql",
    },
    constant: {
      match: "\\b(true|false|null)\\b",
      name: "constant.language.aql",
    },
    "builtin-func": {
      match: "\\b(count|sum|avg|min|max)(?=\\s*\\()",
      name: "support.function.builtin.aql",
    },
    "func-call": {
      match: "\\b([a-z_][A-Za-z0-9_]*)(?=\\s*\\()",
      name: "support.function.aql",
    },
    "type-name": {
      match: "\\b[A-Z][A-Za-z0-9_]*\\b",
      name: "entity.name.type.aql",
    },
    splat: {
      match: "\\*",
      name: "keyword.operator.splat.aql",
    },
    operator: {
      match: ":=|\\?\\?|!=|<=|>=|=|<|>|\\+|\\-|\\*|\\/",
      name: "keyword.operator.aql",
    },
  },
};

export const asl: LanguageRegistration = {
  name: "asl",
  scopeName: "source.asl",
  embeddedLangs: ["aql"],
  patterns: [
    { include: "#comment" },
    { include: "#inline-aql" },
    { include: "#string" },
    { include: "#number" },
    { include: "#policy-predicate" },
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
        "\\b(scalar|type|model|enum|abstract|extends|extending|required|multi|single|property|link|constraint|index|on|computed|default|func|rewrite|trigger|function|before|after|for|each|row|statement|when|do|execute|use|extension|return|policy|using|with|check|to|global)\\b",
      name: "keyword.declaration.asl",
    },
    "builtin-type": {
      match:
        "\\b(str|int16|int32|int64|float32|float64|bool|uuid|datetime|date|time|json|jsonb|bytes|decimal)\\b",
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
      match: "\\b([a-z_][A-Za-z0-9_]*)(?=\\s*\\()",
      name: "support.function.asl",
    },
    "type-name": {
      match: "\\b[A-Z][A-Za-z0-9_]*\\b",
      name: "entity.name.type.asl",
    },
    operator: {
      match: ":=|\\?\\?|!=|<=|>=|=|<|>|\\+|\\-|\\*|\\/|@",
      name: "keyword.operator.asl",
    },
    "policy-predicate": {
      begin: "\\b(using|with\\s+check)\\s*(\\()",
      beginCaptures: {
        "1": { name: "keyword.declaration.asl" },
        "2": { name: "punctuation.section.parens.begin.asl" },
      },
      end: "\\)",
      endCaptures: {
        "0": { name: "punctuation.section.parens.end.asl" },
      },
      contentName: "meta.embedded.block.aql",
      patterns: [{ include: "#aql-parens" }, { include: "source.aql" }],
    },
    "aql-parens": {
      begin: "\\(",
      end: "\\)",
      patterns: [{ include: "#aql-parens" }, { include: "source.aql" }],
    },
    "inline-aql": {
      begin: "\\b(aql)\\s*(`)",
      beginCaptures: {
        "1": { name: "keyword.declaration.asl" },
        "2": { name: "punctuation.definition.string.begin.asl" },
      },
      end: "(`)",
      endCaptures: {
        "1": { name: "punctuation.definition.string.end.asl" },
      },
      contentName: "meta.embedded.block.aql",
      patterns: [{ include: "source.aql" }],
    },
  },
};
