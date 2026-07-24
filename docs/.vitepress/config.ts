import { defineConfig } from "vitepress";

import { aql, asl } from "./languages";

export default defineConfig({
  base: "/axel/",
  title: "Axel",
  description: "Schema and query language tool for PostgreSQL",

  head: [["link", { rel: "icon", type: "image/svg+xml", href: "/axel/logo.svg" }]],

  markdown: {
    languages: [asl, aql],
  },

  themeConfig: {
    logo: "/logo.svg",

    nav: [
      {
        text: "Guide",
        items: [
          { text: "Introduction", link: "/" },
          { text: "Installation", link: "/installation" },
          { text: "Tutorial", link: "/tutorial" },
          { text: "Editor setup", link: "/editors" },
        ],
      },
      {
        text: "Languages",
        items: [
          { text: "Schema Language (ASL)", link: "/asl/" },
          { text: "Query Language (AQL)", link: "/aql/" },
        ],
      },
      { text: "CLI", link: "/cli" },
    ],

    sidebar: [
      {
        text: "Getting Started",
        items: [
          { text: "Introduction", link: "/" },
          { text: "Installation", link: "/installation" },
          { text: "Tutorial", link: "/tutorial" },
          { text: "Editor setup", link: "/editors" },
        ],
      },
      {
        text: "Schema Language (ASL)",
        collapsed: false,
        items: [
          { text: "Overview", link: "/asl/" },
          {
            text: "Schema",
            link: "/asl/schema",
            collapsed: true,
            items: [
              { text: "Types", link: "/asl/schema/types" },
              { text: "Inheritance", link: "/asl/schema/inheritance" },
              { text: "Indexes", link: "/asl/schema/indexes" },
              { text: "Constraints", link: "/asl/schema/constraints" },
            ],
          },
          {
            text: "Data Types",
            link: "/asl/data-types",
            collapsed: true,
            items: [
              { text: "Scalars", link: "/asl/data-types/scalars" },
              { text: "Aliases", link: "/asl/data-types/aliases" },
              { text: "Enums", link: "/asl/data-types/enums" },
            ],
          },
          {
            text: "Fields",
            link: "/asl/fields",
            collapsed: true,
            items: [
              { text: "Properties", link: "/asl/fields/properties" },
              { text: "Rewrites", link: "/asl/fields/rewrites" },
              { text: "Constraints", link: "/asl/fields/constraints" },
              { text: "Links", link: "/asl/fields/links" },
              { text: "Computed Fields", link: "/asl/fields/computed" },
            ],
          },
          { text: "Functions", link: "/asl/functions" },
          { text: "Triggers", link: "/asl/triggers" },
        ],
      },
      {
        text: "Query Language (AQL)",
        collapsed: false,
        items: [
          { text: "Overview", link: "/aql/" },
          {
            text: "Parameters",
            link: "/aql/parameters",
            collapsed: true,
            items: [
              { text: "Named", link: "/aql/parameters/named" },
              { text: "Optional", link: "/aql/parameters/optional" },
              { text: "Typed", link: "/aql/parameters/typed" },
            ],
          },
          {
            text: "Select",
            link: "/aql/select",
            collapsed: true,
            items: [
              { text: "Basics", link: "/aql/select/basics" },
              { text: "Filtering", link: "/aql/select/filtering" },
              { text: "Ordering & Pagination", link: "/aql/select/ordering" },
              { text: "Computed Fields", link: "/aql/select/computed" },
              { text: "Nested Shapes", link: "/aql/select/nested" },
              { text: "Aggregates", link: "/aql/select/aggregates" },
            ],
          },
          {
            text: "Insert",
            link: "/aql/insert",
            collapsed: true,
            items: [
              { text: "Basics", link: "/aql/insert/basics" },
              { text: "Conflicts", link: "/aql/insert/conflicts" },
            ],
          },
          {
            text: "Update",
            link: "/aql/update",
            collapsed: true,
            items: [
              { text: "Basics", link: "/aql/update/basics" },
              { text: "Partial Updates", link: "/aql/update/partial" },
              { text: "Links", link: "/aql/update/links" },
            ],
          },
          { text: "Delete", link: "/aql/delete" },
          {
            text: "Expressions",
            link: "/aql/expressions",
            collapsed: true,
            items: [
              { text: "Operators", link: "/aql/expressions/operators" },
              { text: "Literals", link: "/aql/expressions/literals" },
              { text: "Path Expressions", link: "/aql/expressions/paths" },
              { text: "Casts & Types", link: "/aql/expressions/casts" },
            ],
          },
          { text: "Directives", link: "/aql/directives" },
          { text: "Grammar reference", link: "/aql/grammar" },
        ],
      },
      {
        text: "Reference",
        items: [
          { text: "CLI", link: "/cli" },
          { text: "Global flags", link: "/cli#global-flags" },
          { text: "Schema commands", link: "/cli#schema-commands" },
          { text: "Query commands", link: "/cli#query-commands" },
        ],
      },
    ],

    socialLinks: [
      { icon: "github", link: "https://github.com/struckchure/axel" },
      { icon: "x", link: "https://x.com/struckchure" },
    ],

    footer: {
      message: "Released under the MIT License.",
      copyright: "Copyright © 2026-present Axel Contributors",
    },

    search: {
      provider: "local",
    },
  },
});
