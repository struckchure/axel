import React, { useState } from "react";
import { Button } from "./ui/button";
import { Badge } from "./ui/badge";
import { Card, CardHeader, CardContent } from "./ui/card";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "./ui/tabs";
import {
  Sparkles,
  Terminal,
  ArrowRight,
  Shield,
  Cpu,
  Check,
  Copy,
  Code2,
  FileCode,
} from "lucide-react";

export default function LandingPage() {
  const [copied, setCopied] = useState(false);

  const copyInstall = () => {
    navigator.clipboard.writeText("axel init");
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="not-content w-full max-w-[1140px] mx-auto px-4 py-8 text-zinc-900 dark:text-slate-100 font-sans">
      {/* Hero Section */}
      <section className="flex flex-col items-center text-center pt-8 pb-12">
        <Badge variant="default" className="mb-6 px-4 py-1 text-xs cursor-pointer gap-2">
          <Sparkles className="w-3.5 h-3.5 text-orange-500 dark:text-orange-400" />
          <span>Announcing Axel — Ahead-of-Time PostgreSQL Compiler</span>
        </Badge>

        <h1 className="text-4xl sm:text-6xl md:text-7xl font-extrabold tracking-tight text-zinc-950 dark:text-white leading-[1.08] mb-6">
          SQL generation for <br className="hidden sm:inline" />
          <span className="bg-gradient-to-r from-zinc-900 via-orange-600 to-amber-500 dark:from-white dark:via-orange-300 dark:to-orange-500 bg-clip-text text-transparent">
            schemas + queries
          </span>
        </h1>

        <p className="max-w-2xl text-base sm:text-xl text-zinc-600 dark:text-slate-400 leading-relaxed mb-8">
          Write schemas in <strong className="text-orange-600 dark:text-orange-400 font-semibold">ASL</strong>, queries in <strong className="text-orange-600 dark:text-orange-400 font-semibold">AQL</strong>. Axel compiles both to clean PostgreSQL SQL — migrations and parameterized query strings. <span className="text-zinc-800 dark:text-slate-300">No ORM, no driver, no runtime magic.</span>
        </p>

        <div className="flex flex-wrap items-center justify-center gap-4 mb-14">
          <a
            href="/axel/installation/"
            className="inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-full text-base font-semibold transition-all duration-200 focus-visible:outline-none disabled:pointer-events-none disabled:opacity-50 h-11 px-7 bg-orange-600 !text-white hover:!text-white visited:!text-white shadow-md shadow-orange-600/30 hover:bg-orange-500 hover:shadow-lg hover:shadow-orange-600/40 hover:-translate-y-0.5 active:translate-y-0 cursor-pointer no-underline [&_svg]:size-4"
          >
            Get Started <ArrowRight className="w-4 h-4 text-white" />
          </a>

          <button
            onClick={copyInstall}
            type="button"
            className="inline-flex items-center gap-3 px-5 py-2.5 rounded-full bg-zinc-100 border border-zinc-300 text-zinc-800 font-mono text-sm shadow-sm hover:border-orange-500/50 hover:bg-zinc-200 dark:bg-[#12141c] dark:border-[#232738] dark:text-slate-300 dark:hover:bg-[#181b26] transition-all cursor-pointer"
          >
            <Terminal className="w-4 h-4 text-orange-500 dark:text-orange-400" />
            <span>axel init</span>
            {copied ? (
              <Check className="w-3.5 h-3.5 text-emerald-600 dark:text-emerald-400" />
            ) : (
              <Copy className="w-3.5 h-3.5 text-zinc-400 hover:text-zinc-600 dark:text-slate-500 dark:hover:text-slate-300" />
            )}
          </button>
        </div>
      </section>

      {/* Interactive App Window Mockup Frame */}
      <div className="relative mb-20">
        <div className="absolute inset-0 bg-gradient-to-r from-orange-500/10 via-amber-500/15 to-orange-600/10 blur-3xl -z-10 rounded-3xl" />
        
        <div className="rounded-xl border border-zinc-200 bg-white shadow-xl overflow-hidden dark:border-[#232738] dark:bg-[#0b0c10] dark:shadow-2xl">
          {/* Window Header */}
          <div className="flex items-center justify-between px-4 py-3 bg-zinc-100 border-b border-zinc-200 dark:bg-[#10121a] dark:border-[#1e2230]">
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 rounded-full bg-red-500/80" />
              <div className="w-3 h-3 rounded-full bg-yellow-500/80" />
              <div className="w-3 h-3 rounded-full bg-emerald-500/80" />
            </div>

            <div className="flex items-center gap-1 bg-zinc-200/70 p-1 rounded-full border border-zinc-300/80 dark:bg-[#090a0f] dark:border-[#1a1e2b]">
              <span className="px-3 py-0.5 rounded-full text-xs font-medium bg-white text-zinc-900 shadow-sm dark:bg-[#1e2230] dark:text-white">Documentation</span>
              <span className="px-3 py-0.5 rounded-full text-xs font-medium text-zinc-600 hover:text-zinc-950 dark:text-slate-400 dark:hover:text-slate-200 cursor-pointer">Schema (ASL)</span>
              <span className="px-3 py-0.5 rounded-full text-xs font-medium text-zinc-600 hover:text-zinc-950 dark:text-slate-400 dark:hover:text-slate-200 cursor-pointer">Queries (AQL)</span>
              <span className="px-3 py-0.5 rounded-full text-xs font-medium text-zinc-600 hover:text-zinc-950 dark:text-slate-400 dark:hover:text-slate-200 cursor-pointer">Reference</span>
            </div>

            <div className="text-xs text-zinc-500 dark:text-slate-500 font-mono">v1.0</div>
          </div>

          {/* Window Layout Grid */}
          <div className="grid grid-cols-1 md:grid-cols-[220px_1fr_200px] min-h-[380px] text-sm">
            {/* Sidebar */}
            <div className="hidden md:flex flex-col gap-1 p-4 bg-zinc-50 border-r border-zinc-200 text-zinc-600 dark:bg-[#090a0f] dark:border-[#1a1e2b] dark:text-slate-400">
              <span className="text-[0.7rem] uppercase tracking-wider font-semibold text-zinc-400 dark:text-slate-500 px-2 mb-1">Getting Started</span>
              <div className="px-3 py-1.5 rounded-lg text-zinc-600 hover:text-zinc-950 dark:text-slate-400 dark:hover:text-white cursor-pointer text-xs">Introduction</div>
              <div className="px-3 py-1.5 rounded-lg bg-zinc-200/80 text-zinc-950 border border-zinc-300 font-medium text-xs dark:bg-[#1c1f2b] dark:text-white dark:border-[#282c3f]">Quick look</div>
              <div className="px-3 py-1.5 rounded-lg text-zinc-600 hover:text-zinc-950 dark:text-slate-400 dark:hover:text-white cursor-pointer text-xs">Why Axel?</div>
              <div className="px-3 py-1.5 rounded-lg text-zinc-600 hover:text-zinc-950 dark:text-slate-400 dark:hover:text-white cursor-pointer text-xs">Installation</div>

              <span className="text-[0.7rem] uppercase tracking-wider font-semibold text-zinc-400 dark:text-slate-500 px-2 mt-4 mb-1">Schema Language</span>
              <div className="px-3 py-1.5 rounded-lg text-zinc-600 hover:text-zinc-950 dark:text-slate-400 dark:hover:text-white cursor-pointer text-xs">Types & Links</div>
              <div className="px-3 py-1.5 rounded-lg text-zinc-600 hover:text-zinc-950 dark:text-slate-400 dark:hover:text-white cursor-pointer text-xs">Indexes & Policies</div>
            </div>

            {/* Main Mockup Content */}
            <div className="p-6 bg-white dark:bg-[#0b0c10]">
              <div className="flex items-center justify-between mb-2">
                <h2 className="text-xl font-bold text-zinc-950 dark:text-white tracking-tight">Quick look</h2>
                <Badge variant="secondary" className="text-[0.7rem] gap-1 cursor-pointer">
                  <span>Copy code</span>
                </Badge>
              </div>
              <p className="text-xs text-zinc-600 dark:text-slate-400 mb-4 leading-relaxed">
                Define models in ASL, query shapes in AQL, and get optimal single-scan PostgreSQL queries.
              </p>

              <Tabs defaultValue="schema" className="w-full">
                <TabsList className="mb-3">
                  <TabsTrigger value="schema">schema.asl</TabsTrigger>
                  <TabsTrigger value="query">get_posts.aql</TabsTrigger>
                  <TabsTrigger value="sql">compiled.sql</TabsTrigger>
                </TabsList>

                <TabsContent value="schema">
                  <pre className="p-4 rounded-lg bg-zinc-50 border border-zinc-200 text-xs font-mono text-zinc-800 dark:bg-[#07080b] dark:border-[#1d202d] dark:text-slate-300 overflow-x-auto leading-relaxed">
                    <span className="text-pink-600 dark:text-pink-400">use extension</span> <span className="text-emerald-600 dark:text-emerald-400">'pgcrypto'</span>;
                    {"\n\n"}<span className="text-pink-600 dark:text-pink-400">type</span> <span className="text-purple-600 dark:text-purple-400">User</span> {"{"}
                    {"\n"}  <span className="text-pink-600 dark:text-pink-400">required</span> id: <span className="text-blue-600 dark:text-blue-400">uuid</span> {"{"} <span className="text-pink-600 dark:text-pink-400">default</span> := <span className="text-blue-600 dark:text-blue-400">gen_uuid</span>(); <span className="text-pink-600 dark:text-pink-400">constraint</span> pk; {"}"};
                    {"\n"}  <span className="text-pink-600 dark:text-pink-400">required</span> email: <span className="text-blue-600 dark:text-blue-400">str</span> {"{"} <span className="text-pink-600 dark:text-pink-400">constraint</span> exclusive; {"}"};
                    {"\n"}  name: <span className="text-blue-600 dark:text-blue-400">str</span>;
                    {"\n"}{"}"}
                    {"\n\n"}<span className="text-pink-600 dark:text-pink-400">type</span> <span className="text-purple-600 dark:text-purple-400">Post</span> {"{"}
                    {"\n"}  <span className="text-pink-600 dark:text-pink-400">required</span> title: <span className="text-blue-600 dark:text-blue-400">str</span>;
                    {"\n"}  <span className="text-pink-600 dark:text-pink-400">required link</span> author: <span className="text-purple-600 dark:text-purple-400">User</span>;
                    {"\n"}  <span className="text-pink-600 dark:text-pink-400">multi link</span> likes: <span className="text-purple-600 dark:text-purple-400">User</span>;
                    {"\n"}{"}"}
                  </pre>
                </TabsContent>

                <TabsContent value="query">
                  <pre className="p-4 rounded-lg bg-zinc-50 border border-zinc-200 text-xs font-mono text-zinc-800 dark:bg-[#07080b] dark:border-[#1d202d] dark:text-slate-300 overflow-x-auto leading-relaxed">
                    <span className="text-pink-600 dark:text-pink-400">select</span> <span className="text-purple-600 dark:text-purple-400">Post</span> {"{"}
                    {"\n"}  id,
                    {"\n"}  title,
                    {"\n"}  author: {"{"} id, email {"}"},
                    {"\n"}  likes: {"{"} id, email {"}"}
                    {"\n"}{"}"}
                    {"\n"}<span className="text-pink-600 dark:text-pink-400">filter</span> .author.id = <span className="text-orange-600 dark:text-orange-400">$author_id</span>
                    {"\n"}<span className="text-pink-600 dark:text-pink-400">order by</span> .created_at <span className="text-pink-600 dark:text-pink-400">desc</span>;
                  </pre>
                </TabsContent>

                <TabsContent value="sql">
                  <pre className="p-4 rounded-lg bg-zinc-50 border border-zinc-200 text-xs font-mono text-zinc-800 dark:bg-[#07080b] dark:border-[#1d202d] dark:text-slate-300 overflow-x-auto leading-relaxed">
                    <span className="text-pink-600 dark:text-pink-400">SELECT</span> p.id, p.title,
                    {"\n"}  (<span className="text-pink-600 dark:text-pink-400">SELECT</span> row_to_json(u) <span className="text-pink-600 dark:text-pink-400">FROM</span> "user" u <span className="text-pink-600 dark:text-pink-400">WHERE</span> u.id = p.author_id) <span className="text-pink-600 dark:text-pink-400">AS</span> author,
                    {"\n"}  (<span className="text-pink-600 dark:text-pink-400">SELECT</span> COALESCE(json_agg(l), '[]') <span className="text-pink-600 dark:text-pink-400">FROM</span> "post_likes" pl
                    {"\n"}   <span className="text-pink-600 dark:text-pink-400">JOIN</span> "user" l <span className="text-pink-600 dark:text-pink-400">ON</span> l.id = pl.user_id <span className="text-pink-600 dark:text-pink-400">WHERE</span> pl.post_id = p.id) <span className="text-pink-600 dark:text-pink-400">AS</span> likes
                    {"\n"}<span className="text-pink-600 dark:text-pink-400">FROM</span> "post" p <span className="text-pink-600 dark:text-pink-400">WHERE</span> p.author_id = <span className="text-blue-600 dark:text-blue-400">$1</span> <span className="text-pink-600 dark:text-pink-400">ORDER BY</span> p.created_at <span className="text-pink-600 dark:text-pink-400">DESC</span>;
                  </pre>
                </TabsContent>
              </Tabs>
            </div>

            {/* Right TOC */}
            <div className="hidden md:flex flex-col gap-2 p-4 bg-zinc-50 border-l border-zinc-200 text-xs text-zinc-600 dark:bg-[#090a0f] dark:border-[#1a1e2b] dark:text-slate-400">
              <span className="font-semibold text-zinc-900 dark:text-white mb-1">On this page</span>
              <div className="border-l-2 border-orange-500 text-orange-600 dark:text-orange-400 pl-2 font-medium">Quick look</div>
              <div className="pl-2.5 text-zinc-500 hover:text-zinc-900 dark:text-slate-500 dark:hover:text-slate-300 cursor-pointer">Schema definition</div>
              <div className="pl-2.5 text-zinc-500 hover:text-zinc-900 dark:text-slate-500 dark:hover:text-slate-300 cursor-pointer">AQL compilation</div>
              <div className="pl-2.5 text-zinc-500 hover:text-zinc-900 dark:text-slate-500 dark:hover:text-slate-300 cursor-pointer">PostgreSQL output</div>
            </div>
          </div>
        </div>
      </div>

      {/* Value Proof Divider */}
      <div className="text-center py-10 sm:py-14 border-y border-zinc-200 dark:border-[#181b26] my-24 sm:my-36 text-zinc-600 dark:text-slate-400 text-sm sm:text-base leading-relaxed max-w-4xl mx-auto">
        Built for teams creating modern, high-performance <strong className="text-zinc-950 dark:text-white font-semibold">PostgreSQL</strong> applications with <strong className="text-orange-600 dark:text-orange-400 font-semibold">zero runtime overhead</strong>.
      </div>

      {/* Feature Showcase Rows (docs.page style with corner notches) */}
      <div className="flex flex-col gap-24 sm:gap-32 mb-32 sm:mb-44">
        {/* Row 1: ASL */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-12 items-center">
          <div>
            <Badge variant="tag" className="mb-4">Schema Language (ASL)</Badge>
            <h2 className="text-2xl sm:text-3xl font-bold text-zinc-950 dark:text-white tracking-tight mb-4">
              Declarative Schemas, Zero Decorator Gymnastics
            </h2>
            <p className="text-zinc-600 dark:text-slate-400 leading-relaxed mb-6">
              Define your models, single/multi links, indexes, and custom constraints cleanly in <code className="text-orange-600 dark:text-orange-400 font-mono text-xs">.asl</code> files. Axel AST-diffs your schema and generates clean, deterministic PostgreSQL migration SQL automatically.
            </p>
            <a href="/axel/asl/" className="inline-flex items-center gap-2 text-sm font-semibold text-orange-600 dark:text-orange-400 hover:text-orange-500 transition-colors">
              Explore Schema Language (ASL) <ArrowRight className="w-4 h-4" />
            </a>
          </div>

          <Card className="before:absolute before:top-0 before:left-0 before:w-6 before:h-6 before:border-t-2 before:border-l-2 before:border-orange-500 before:rounded-tl-xl">
            <CardHeader>
              <div className="flex items-center gap-2">
                <FileCode className="w-3.5 h-3.5 text-orange-600 dark:text-orange-400" />
                <span>schema.asl</span>
              </div>
              <span className="text-[0.7rem] text-zinc-500 dark:text-slate-500 font-mono">ASL Model</span>
            </CardHeader>
            <CardContent>
              <pre className="text-xs font-mono text-zinc-800 dark:text-slate-300 leading-relaxed overflow-x-auto">
                <span className="text-pink-600 dark:text-pink-400">type</span> <span className="text-purple-600 dark:text-purple-400">Article</span> <span className="text-pink-600 dark:text-pink-400">extending</span> <span className="text-purple-600 dark:text-purple-400">Base</span> {"{"}
                {"\n"}  <span className="text-pink-600 dark:text-pink-400">required</span> title: <span className="text-blue-600 dark:text-blue-400">str</span>;
                {"\n"}  slug: <span className="text-blue-600 dark:text-blue-400">str</span> {"{"} <span className="text-pink-600 dark:text-pink-400">constraint</span> exclusive; {"}"};
                {"\n"}  <span className="text-pink-600 dark:text-pink-400">required link</span> author: <span className="text-purple-600 dark:text-purple-400">User</span>;
                {"\n"}  <span className="text-pink-600 dark:text-pink-400">multi link</span> tags: <span className="text-purple-600 dark:text-purple-400">Tag</span>;
                {"\n"}{"}"}
              </pre>
            </CardContent>
          </Card>
        </div>

        {/* Row 2: AQL */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-12 items-center">
          <div className="lg:order-2">
            <Badge variant="tag" className="mb-4">Query Compiler (AQL)</Badge>
            <h2 className="text-2xl sm:text-3xl font-bold text-zinc-950 dark:text-white tracking-tight mb-4">
              Nested Shapes in a Single Database Scan
            </h2>
            <p className="text-zinc-600 dark:text-slate-400 leading-relaxed mb-6">
              Select deeply nested relational graphs in one intuitive query. Axel compiles nested shapes directly into PostgreSQL lateral subqueries with <code className="text-orange-600 dark:text-orange-400 font-mono text-xs">json_agg</code> and <code className="text-orange-600 dark:text-orange-400 font-mono text-xs">row_to_json</code> — returning typed JSON trees in a single database round-trip with <strong>zero N+1 queries</strong>.
            </p>
            <a href="/axel/aql/" className="inline-flex items-center gap-2 text-sm font-semibold text-orange-600 dark:text-orange-400 hover:text-orange-500 transition-colors">
              Explore Query Language (AQL) <ArrowRight className="w-4 h-4" />
            </a>
          </div>

          <Card className="lg:order-1 before:absolute before:top-0 before:left-0 before:w-6 before:h-6 before:border-t-2 before:border-l-2 before:border-orange-500 before:rounded-tl-xl">
            <CardHeader>
              <div className="flex items-center gap-2">
                <Code2 className="w-3.5 h-3.5 text-orange-600 dark:text-orange-400" />
                <span>get_article.aql</span>
              </div>
              <span className="text-[0.7rem] text-zinc-500 dark:text-slate-500 font-mono">AQL Query</span>
            </CardHeader>
            <CardContent>
              <pre className="text-xs font-mono text-zinc-800 dark:text-slate-300 leading-relaxed overflow-x-auto">
                <span className="text-pink-600 dark:text-pink-400">select</span> <span className="text-purple-600 dark:text-purple-400">Article</span> {"{"}
                {"\n"}  id,
                {"\n"}  title,
                {"\n"}  author: {"{"} id, email {"}"},
                {"\n"}  tags: {"{"} id, name {"}"}
                {"\n"}{"}"}
                {"\n"}<span className="text-pink-600 dark:text-pink-400">filter</span> .slug = <span className="text-orange-600 dark:text-orange-400">$slug</span>;
              </pre>
            </CardContent>
          </Card>
        </div>

        {/* Row 3: AOT Zero Runtime */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-12 items-center">
          <div>
            <Badge variant="tag" className="mb-4">Ahead-of-Time Architecture</Badge>
            <h2 className="text-2xl sm:text-3xl font-bold text-zinc-950 dark:text-white tracking-tight mb-4">
              Ahead-of-Time Compiler, Zero Runtime Overhead
            </h2>
            <p className="text-zinc-600 dark:text-slate-400 leading-relaxed mb-6">
              Axel is a build-time compiler producing pure parameterized SQL strings and typed structs/interfaces. There is no query engine sidecar, no connection pooling layer, and zero driver wrapping.
            </p>
            <a href="/axel/codegen/" className="inline-flex items-center gap-2 text-sm font-semibold text-orange-600 dark:text-orange-400 hover:text-orange-500 transition-colors">
              Explore Code Generation <ArrowRight className="w-4 h-4" />
            </a>
          </div>

          <Card className="before:absolute before:top-0 before:left-0 before:w-6 before:h-6 before:border-t-2 before:border-l-2 before:border-orange-500 before:rounded-tl-xl">
            <CardHeader>
              <div className="flex items-center gap-2">
                <Cpu className="w-3.5 h-3.5 text-orange-600 dark:text-orange-400" />
                <span>Runtime Architecture</span>
              </div>
              <span className="text-[0.7rem] text-zinc-500 dark:text-slate-500 font-mono">Zero Footprint</span>
            </CardHeader>
            <CardContent className="space-y-3 text-xs leading-relaxed">
              <div>
                <strong className="text-orange-600 dark:text-orange-400 font-semibold block mb-0.5">✓ Axel (AOT):</strong>
                <span className="text-zinc-700 dark:text-slate-300">Compiles at build-time to plain SQL strings. Zero runtime CPU overhead, zero connection wrapper, zero cold-starts.</span>
              </div>
              <div>
                <strong className="text-zinc-500 dark:text-slate-500 font-semibold block mb-0.5">✗ Heavy Runtime Engines:</strong>
                <span className="text-zinc-500 dark:text-slate-400">Bundle Rust binaries (Prisma 6) or WASM compilers (Prisma 7) adding memory overhead and locking execution to Node.js.</span>
              </div>
            </CardContent>
          </Card>
        </div>

        {/* Row 4: Native Postgres */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-12 items-center">
          <div className="lg:order-2">
            <Badge variant="tag" className="mb-4">PostgreSQL Superpowers</Badge>
            <h2 className="text-2xl sm:text-3xl font-bold text-zinc-950 dark:text-white tracking-tight mb-4">
              Extensions, Row-Level Security, Conflicts & Triggers
            </h2>
            <p className="text-zinc-600 dark:text-slate-400 leading-relaxed mb-6">
              Axel is purpose-built for PostgreSQL. Declare database extensions (<code className="text-orange-600 dark:text-orange-400 font-mono text-xs">pgvector</code>, <code className="text-orange-600 dark:text-orange-400 font-mono text-xs">pg_trgm</code>), fine-grained Row-Level Security (<code className="text-orange-600 dark:text-orange-400 font-mono text-xs">policy</code>), triggers, and <code className="text-orange-600 dark:text-orange-400 font-mono text-xs">unless conflict</code> upsert semantics directly in your schema and queries.
            </p>
            <a href="/axel/asl/policies/" className="inline-flex items-center gap-2 text-sm font-semibold text-orange-600 dark:text-orange-400 hover:text-orange-500 transition-colors">
              Explore Policies & RLS Guide <ArrowRight className="w-4 h-4" />
            </a>
          </div>

          <Card className="lg:order-1 before:absolute before:top-0 before:left-0 before:w-6 before:h-6 before:border-t-2 before:border-l-2 before:border-orange-500 before:rounded-tl-xl">
            <CardHeader>
              <div className="flex items-center gap-2">
                <Shield className="w-3.5 h-3.5 text-orange-600 dark:text-orange-400" />
                <span>policies.asl</span>
              </div>
              <span className="text-[0.7rem] text-zinc-500 dark:text-slate-500 font-mono">PostgreSQL Native</span>
            </CardHeader>
            <CardContent>
              <pre className="text-xs font-mono text-zinc-800 dark:text-slate-300 leading-relaxed overflow-x-auto">
                <span className="text-pink-600 dark:text-pink-400">policy</span> tenant_isolation <span className="text-pink-600 dark:text-pink-400">on</span> <span className="text-purple-600 dark:text-purple-400">Organization</span>
                {"\n"}  <span className="text-pink-600 dark:text-pink-400">for all using</span> (.id = <span className="text-blue-600 dark:text-blue-400">global</span>.current_org_id);
              </pre>
            </CardContent>
          </Card>
        </div>
      </div>

      {/* Bottom CTA Banner (Image 2 style) */}
      <div className="rounded-2xl border border-zinc-200 bg-gradient-to-r from-zinc-50 via-orange-50/50 to-amber-50/60 p-8 sm:p-14 flex flex-col md:flex-row items-center justify-between gap-8 relative overflow-hidden shadow-lg dark:border-[#232738] dark:bg-gradient-to-r dark:from-[#0c0e14] dark:via-[#10131d] dark:to-[#1a1310] dark:shadow-xl mt-16 sm:mt-24 mb-16 sm:mb-24">
        <div className="max-w-xl text-center md:text-left">
          <h2 className="text-3xl sm:text-4xl font-extrabold text-zinc-950 dark:text-white tracking-tight mb-4">
            Bring your database <br />
            into the <span className="bg-gradient-to-r from-orange-600 to-amber-500 dark:from-orange-400 dark:to-amber-300 bg-clip-text text-transparent">AOT age</span>
          </h2>
          <p className="text-zinc-600 dark:text-slate-400 text-sm sm:text-base mb-8 leading-relaxed">
            Get started in seconds with Axel's CLI, schema modeling, and zero-runtime query compiler.
          </p>
          <a
            href="/axel/installation/"
            className="inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-full text-base font-semibold transition-all duration-200 focus-visible:outline-none disabled:pointer-events-none disabled:opacity-50 h-11 px-7 bg-orange-600 !text-white hover:!text-white visited:!text-white shadow-md shadow-orange-600/30 hover:bg-orange-500 hover:shadow-lg hover:shadow-orange-600/40 hover:-translate-y-0.5 active:translate-y-0 cursor-pointer no-underline [&_svg]:size-4"
          >
            Get Started <ArrowRight className="w-4 h-4 text-white" />
          </a>
        </div>

        <div className="shrink-0 flex items-center justify-center filter drop-shadow-[0_0_35px_rgba(249,115,22,0.4)]">
          <svg viewBox="0 0 32 32" width="120" height="120" fill="none">
            <path d="M16 2.5 L28 9.5 L28 22.5 L16 29.5 L4 22.5 L4 9.5 Z" className="fill-zinc-100 stroke-orange-500 dark:fill-[#141721] dark:stroke-[#F97316]" strokeWidth="2.5" strokeLinejoin="round" />
            <path d="M16 8 L22 21 L18.5 21 L16 15.5 L13.5 21 L10 21 Z" className="fill-orange-500 dark:fill-[#F97316]" />
            <path d="M14 18 L18 18" className="stroke-zinc-100 dark:stroke-[#141721]" strokeWidth="1.8" strokeLinecap="round" />
          </svg>
        </div>
      </div>
    </div>
  );
}
