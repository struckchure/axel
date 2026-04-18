import { defineConfig } from "vitepress";

export default defineConfig({
  title: "Axel",
  description: "Schema and query language tool for PostgreSQL",

  head: [["link", { rel: "icon", href: "/favicon.ico" }]],

  themeConfig: {
    logo: "/logo.svg",

    nav: [
      {
        text: "Guide",
        items: [
          { text: "Introduction", link: "/" },
          { text: "Installation", link: "/installation" },
        ],
      },
      {
        text: "Languages",
        items: [
          { text: "Schema Language (ASL)", link: "/asl" },
          { text: "Query Language (AQL)", link: "/aql" },
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
        ],
      },
      {
        text: "Schema Language (ASL)",
        collapsed: false,
        items: [
          { text: "Overview", link: "/asl" },
          { text: "Types", link: "/asl#types" },
          { text: "Scalar types", link: "/asl#scalar-types" },
          { text: "Enum types", link: "/asl#enum-types" },
          { text: "Properties", link: "/asl#properties" },
          { text: "Links", link: "/asl#links" },
          { text: "Computed fields", link: "/asl#computed-fields" },
          { text: "Indexes", link: "/asl#indexes" },
        ],
      },
      {
        text: "Query Language (AQL)",
        collapsed: false,
        items: [
          { text: "Overview", link: "/aql" },
          { text: "Parameters", link: "/aql#parameters" },
          { text: "SELECT", link: "/aql#select" },
          { text: "Nested shapes", link: "/aql#nested-shapes-links" },
          { text: "INSERT", link: "/aql#insert" },
          { text: "UPDATE", link: "/aql#update" },
          { text: "DELETE", link: "/aql#delete" },
          { text: "Operators", link: "/aql#operators" },
          { text: "Grammar reference", link: "/aql#grammar-reference" },
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
