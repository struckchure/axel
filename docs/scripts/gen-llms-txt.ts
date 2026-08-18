#!/usr/bin/env bun
/**
 * Generates the llms.txt pair served from the docs site:
 *
 *   public/llms.txt       an index of every page, per https://llmstxt.org
 *   public/llms-full.txt  the entire documentation as one plain-text file
 *
 * Both are written from the same source of truth as the site itself
 * (src/content/docs/**), and the build runs this before `astro build`, so they
 * cannot drift from the docs.
 */

import { Glob } from "bun";
import { basename, dirname, join } from "node:path";

const DOCS_ROOT = join(import.meta.dir, "..");
const CONTENT_DIR = join(DOCS_ROOT, "src/content/docs");
const PUBLIC_DIR = join(DOCS_ROOT, "public");
const CONFIG = join(DOCS_ROOT, "astro.config.mjs");

const TAGLINE =
  "Axel is an ahead-of-time compiler for PostgreSQL. You define your data model in ASL " +
  "(Axel Schema Language) and write queries in AQL (Axel Query Language); Axel compiles " +
  "ASL to migration SQL and AQL to parameterized query strings. It is not an ORM: it never " +
  "wraps a driver, holds a connection, or executes anything on your behalf.";

/** Sections of the index, in order. The first prefix that matches wins. */
const SECTIONS: { title: string; match: (slug: string) => boolean }[] = [
  { title: "Getting started", match: (s) => ["", "installation", "tutorial", "why-axel", "comparison"].includes(s) },
  { title: "Schema Language (ASL)", match: (s) => s === "asl" || s.startsWith("asl/") },
  { title: "Query Language (AQL)", match: (s) => s === "aql" || s.startsWith("aql/") },
  { title: "Examples", match: (s) => s === "examples" || s.startsWith("examples/") },
  { title: "Integrations", match: (s) => s === "integrations" || s.startsWith("integrations/") },
  { title: "Tooling", match: () => true },
];

type Page = {
  slug: string;
  url: string;
  title: string;
  description: string;
  body: string;
};

/**
 * Page order, taken from the sidebar in astro.config.mjs so the two agree.
 * Pages the sidebar does not list (404, the landing page) fall to the end.
 */
async function sidebarOrder(): Promise<Map<string, number>> {
  const text = await Bun.file(CONFIG).text();
  const order = new Map<string, number>();
  for (const m of text.matchAll(/slug:\s*"([^"]+)"/g)) {
    if (!order.has(m[1])) order.set(m[1], order.size);
  }
  return order;
}

/** Splits YAML frontmatter off the top of a document. */
function splitFrontmatter(raw: string): { front: string; body: string } {
  if (!raw.startsWith("---")) return { front: "", body: raw };
  const end = raw.indexOf("\n---", 3);
  if (end === -1) return { front: "", body: raw };
  return { front: raw.slice(3, end), body: raw.slice(end + 4) };
}

function frontmatterValue(front: string, key: string): string {
  const m = front.match(new RegExp(`^${key}:\\s*(.+)$`, "m"));
  if (!m) return "";
  return m[1].trim().replace(/^["']|["']$/g, "");
}

/**
 * Strips the MDX-only syntax that means nothing as plain text: component
 * imports, and the component tags themselves (their children are kept, since
 * <Tabs>/<TabItem> wrap real prose).
 */
function stripMdx(body: string): string {
  return body
    .replace(/^import\s.+?;?\s*$/gm, "")
    .replace(/^<\/?[A-Z][\w.]*(\s[^>]*)?\/?>\s*$/gm, "")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

/** file path under src/content/docs → route slug ("asl/index.md" → "asl"). */
function slugOf(path: string): string {
  const withoutExt = path.replace(/\.mdx?$/, "");
  if (basename(withoutExt) === "index") {
    const dir = dirname(withoutExt);
    return dir === "." ? "" : dir;
  }
  return withoutExt;
}

async function collectPages(baseUrl: string): Promise<Page[]> {
  const order = await sidebarOrder();
  const pages: Page[] = [];

  for await (const file of new Glob("**/*.{md,mdx}").scan(CONTENT_DIR)) {
    const slug = slugOf(file);
    if (slug === "404") continue;

    const raw = await Bun.file(join(CONTENT_DIR, file)).text();
    const { front, body } = splitFrontmatter(raw);
    pages.push({
      slug,
      url: `${baseUrl}/${slug}`.replace(/\/$/, "/"),
      title: frontmatterValue(front, "title") || slug,
      description: frontmatterValue(front, "description"),
      body: stripMdx(body),
    });
  }

  const rank = (p: Page) => order.get(p.slug) ?? Number.MAX_SAFE_INTEGER;
  return pages.sort((a, b) => rank(a) - rank(b) || a.slug.localeCompare(b.slug));
}

function sectionOf(page: Page): string {
  return SECTIONS.find((s) => s.match(page.slug))!.title;
}

function renderIndex(pages: Page[]): string {
  const out = ["# Axel", "", `> ${TAGLINE}`, ""];
  out.push(
    "Axel compiles, it does not execute. Every command turns `.asl` / `.aql` source into SQL",
    "you run yourself with your own driver. The full text of these docs is also available as",
    "one file at /llms-full.txt.",
    "",
  );

  for (const section of SECTIONS) {
    const inSection = pages.filter((p) => sectionOf(p) === section.title);
    if (inSection.length === 0) continue;
    out.push(`## ${section.title}`, "");
    for (const p of inSection) {
      out.push(`- [${p.title}](${p.url})${p.description ? `: ${p.description}` : ""}`);
    }
    out.push("");
  }
  return out.join("\n");
}

function renderFull(pages: Page[]): string {
  const out = [
    "# Axel — full documentation",
    "",
    `> ${TAGLINE}`,
    "",
    "This file concatenates every page of the Axel documentation. Pages are separated by a",
    "horizontal rule and labelled with their source URL.",
    "",
  ];
  for (const p of pages) {
    // The landing page is a single React component — nothing to say in text.
    if (p.body === "") continue;
    out.push("---", "", `Source: ${p.url}`, "", p.body, "");
  }
  return out.join("\n");
}

const baseUrl = await (async () => {
  const text = await Bun.file(CONFIG).text();
  const site = text.match(/site:\s*"([^"]+)"/)?.[1] ?? "";
  const base = text.match(/base:\s*"([^"]+)"/)?.[1] ?? "/";
  return `${site.replace(/\/$/, "")}${base.replace(/\/$/, "")}`;
})();

const pages = await collectPages(baseUrl);
await Bun.write(join(PUBLIC_DIR, "llms.txt"), renderIndex(pages));
await Bun.write(join(PUBLIC_DIR, "llms-full.txt"), renderFull(pages));

console.log(`llms.txt: ${pages.length} pages indexed`);
