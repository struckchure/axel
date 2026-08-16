import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";
import react from "@astrojs/react";
import tailwindcss from "@tailwindcss/vite";
import { aql, asl } from "./src/languages/index.ts";

// https://astro.build/config
export default defineConfig({
  base: "/axel/",
  site: "https://struckchure.github.io",
  vite: {
    plugins: [tailwindcss()],
  },
  integrations: [
    react(),
    starlight({
      title: "Axel",
      description: "Schema and query language tool for PostgreSQL",
      logo: {
        src: "./src/assets/logo.svg",
      },
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/struckchure/axel",
        },
        {
          icon: "x.com",
          label: "X",
          href: "https://x.com/struckchure",
        },
      ],
      customCss: ["./src/styles/custom.css"],
      head: [
        {
          tag: "meta",
          attrs: {
            name: "keywords",
            content:
              "PostgreSQL, AOT, SQL compiler, database modeling, ASL, AQL, SQL generator, ORM alternative, database migrations, type-safety, zero-runtime, Golang, TypeScript",
          },
        },
        {
          tag: "meta",
          attrs: {
            name: "author",
            content: "Axel Authors",
          },
        },
        {
          tag: "meta",
          attrs: {
            name: "theme-color",
            content: "#09090b",
          },
        },
        {
          tag: "meta",
          attrs: {
            property: "og:image",
            content: "https://struckchure.github.io/axel/logo.svg",
          },
        },
        {
          tag: "meta",
          attrs: {
            name: "twitter:card",
            content: "summary",
          },
        },
        {
          tag: "meta",
          attrs: {
            name: "twitter:image",
            content: "https://struckchure.github.io/axel/logo.svg",
          },
        },
        {
          tag: "script",
          attrs: {
            type: "application/ld+json",
          },
          content: JSON.stringify({
            "@context": "https://schema.org",
            "@type": "SoftwareApplication",
            name: "Axel",
            operatingSystem: "All",
            applicationCategory: "DeveloperApplication",
            description:
              "Ahead-of-Time Schema & Query Compiler for PostgreSQL",
            url: "https://struckchure.github.io/axel/",
            offers: {
              "@type": "Offer",
              price: "0",
              priceCurrency: "USD",
            },
            author: {
              "@type": "Organization",
              name: "Axel",
              url: "https://github.com/struckchure/axel",
            },
          }),
        },
      ],
      expressiveCode: {
        defaultProps: {
          showLineNumbers: true,
        },
        shiki: {
          langs: [asl, aql],
        },
        themes: ["github-dark", "github-light"],
      },
      sidebar: [
        {
          label: "Getting Started",
          items: [
            { label: "Introduction", link: "/axel/" },
            { label: "Why Axel?", slug: "why-axel" },
            { label: "Axel vs. Alternatives", slug: "comparison" },
            { label: "Installation", slug: "installation" },
            { label: "Tutorial", slug: "tutorial" },
            { label: "Editor setup", slug: "editors" },
            { label: "Studio", slug: "studio" },
            { label: "Code Generation", slug: "codegen" },
          ],
        },
        {
          label: "Schema Language (ASL)",
          items: [
            { label: "Overview", slug: "asl" },
            {
              label: "Schema",
              collapsed: true,
              items: [
                { label: "Types", slug: "asl/schema/types" },
                { label: "Inheritance", slug: "asl/schema/inheritance" },
                { label: "Indexes", slug: "asl/schema/indexes" },
                { label: "Constraints", slug: "asl/schema/constraints" },
              ],
            },
            {
              label: "Data Types",
              collapsed: true,
              items: [
                { label: "Scalars", slug: "asl/data-types/scalars" },
                { label: "Aliases", slug: "asl/data-types/aliases" },
                { label: "Enums", slug: "asl/data-types/enums" },
              ],
            },
            {
              label: "Fields",
              collapsed: true,
              items: [
                { label: "Properties", slug: "asl/fields/properties" },
                { label: "Rewrites", slug: "asl/fields/rewrites" },
                { label: "Constraints", slug: "asl/fields/constraints" },
                { label: "Links", slug: "asl/fields/links" },
                { label: "Computed Fields", slug: "asl/fields/computed" },
              ],
            },
            { label: "Functions", slug: "asl/functions" },
            { label: "Triggers", slug: "asl/triggers" },
            { label: "Policies", slug: "asl/policies" },
            { label: "Globals", slug: "asl/globals" },
            { label: "Extensions", slug: "asl/extensions" },
          ],
        },
        {
          label: "Query Language (AQL)",
          items: [
            { label: "Overview", slug: "aql" },
            {
              label: "Parameters",
              collapsed: true,
              items: [
                { label: "Named", slug: "aql/parameters/named" },
                { label: "Optional", slug: "aql/parameters/optional" },
                { label: "Typed", slug: "aql/parameters/typed" },
              ],
            },
            {
              label: "Select",
              collapsed: true,
              items: [
                { label: "Basics", slug: "aql/select/basics" },
                { label: "Filtering", slug: "aql/select/filtering" },
                { label: "Ordering & Pagination", slug: "aql/select/ordering" },
                { label: "Computed Fields", slug: "aql/select/computed" },
                { label: "Nested Shapes", slug: "aql/select/nested" },
                { label: "Aggregates", slug: "aql/select/aggregates" },
                { label: "Group By & Having", slug: "aql/select/group-by" },
              ],
            },
            {
              label: "Insert",
              collapsed: true,
              items: [
                { label: "Basics", slug: "aql/insert/basics" },
                { label: "Conflicts", slug: "aql/insert/conflicts" },
              ],
            },
            {
              label: "Update",
              collapsed: true,
              items: [
                { label: "Basics", slug: "aql/update/basics" },
                { label: "Partial Updates", slug: "aql/update/partial" },
                { label: "Links", slug: "aql/update/links" },
              ],
            },
            { label: "Delete", slug: "aql/delete" },
            { label: "With", slug: "aql/with" },
            {
              label: "Expressions",
              collapsed: true,
              items: [
                { label: "Operators", slug: "aql/expressions/operators" },
                { label: "Literals", slug: "aql/expressions/literals" },
                { label: "Path Expressions", slug: "aql/expressions/paths" },
                { label: "Casts & Types", slug: "aql/expressions/casts" },
              ],
            },
            { label: "Directives", slug: "aql/directives" },
            { label: "Grammar reference", slug: "aql/grammar" },
          ],
        },
        {
          label: "Examples",
          items: [
            { label: "Overview", slug: "examples" },
            { label: "Audit timestamps & UUID keys", slug: "examples/timestamps" },
            { label: "Soft deletes", slug: "examples/soft-delete" },
            { label: "Slugs from titles", slug: "examples/slugs" },
            { label: "Multi-tenant row ownership", slug: "examples/multi-tenancy" },
            { label: "Expiring rows + cleanup", slug: "examples/expiring-rows" },
            { label: "Append-only event log", slug: "examples/append-only-log" },
            { label: "Job queue with a single claim", slug: "examples/job-queue" },
            { label: "Upserts", slug: "examples/upsert" },
            { label: "Nested data in one query", slug: "examples/nested-data" },
          ],
        },
        {
          label: "Integrations",
          items: [
            { label: "Overview", slug: "integrations" },
            {
              label: "Database providers",
              collapsed: true,
              items: [
                { label: "Supabase", slug: "integrations/supabase" },
                { label: "Neon", slug: "integrations/neon" },
              ],
            },
            {
              label: "Language clients",
              collapsed: true,
              items: [
                { label: "TypeScript", slug: "integrations/typescript" },
                { label: "Go", slug: "integrations/golang" },
              ],
            },
          ],
        },
        {
          label: "Reference",
          items: [{ label: "CLI Reference", slug: "cli" }],
        },
      ],
    }),
  ],
});
