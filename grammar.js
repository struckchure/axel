/**
 * @file Axel is a modern database tool primarily designed for Go, with multi-language support.
 * @author Mohammed Al-Ameen <ameenmohammed2311@gmail.com>
 * @license MIT
 */

/// <reference types="tree-sitter-cli/dsl" />
// @ts-check

module.exports = grammar({
  name: "axel",

  rules: {
    // TODO: add the actual grammar rules
    source_file: $ => "hello"
  }
});
