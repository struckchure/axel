// Package golang provides a built-in Go code generator for Axel.
// It generates Go structs from ASL types and typed query functions from AQL queries.
//
// Register via blank import in cmd/main.go:
//
//	import _ "github.com/struckchure/axel/generators/golang"
package golang

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"path/filepath"
	"strings"

	"github.com/struckchure/axel/core/codegen"
)

func init() {
	codegen.Register(&GoGenerator{})
}

// modulePath is the axel module import prefix used in generated imports.
const modulePath = "github.com/struckchure/axel"

// GoGenerator is the built-in Go codegen plugin.
// It emits models.go (one struct per type), one .go file per AQL query, and runner.go.
type GoGenerator struct {
	models     bytes.Buffer
	namedTypes bytes.Buffer // directive-named request/response types, hoisted into models.go
	queries    []codegen.QueryDescriptor
	schema     codegen.SchemaDescriptor

	// emitted tracks type names already declared in the package (schema types,
	// enums, and hoisted directive-named types) so shared @request/@response
	// names are emitted once and reused.
	emitted map[string]bool

	pkgName     string
	currentType codegen.TypeDescriptor
}

func (g *GoGenerator) Name() string { return "go" }

// BeginSchema writes package header for both output files.
func (g *GoGenerator) BeginSchema(ctx *codegen.Context, schema codegen.SchemaDescriptor) error {
	g.pkgName = ctx.Options["package"]
	if g.pkgName == "" {
		g.pkgName = "generated"
	}
	g.models.Reset()
	g.namedTypes.Reset()
	g.queries = g.queries[:0]
	g.schema = schema

	// Seed emitted names with everything that already occupies the type namespace.
	g.emitted = make(map[string]bool)
	for _, t := range schema.Types {
		if !t.IsAbstract {
			g.emitted[t.Name] = true
		}
	}
	for _, e := range schema.Enums {
		g.emitted[e.Name] = true
	}
	for _, s := range schema.Scalars {
		g.emitted[s.Name] = true
	}
	return nil
}

func (g *GoGenerator) OnScalar(_ *codegen.Context, s codegen.ScalarDescriptor) error {
	if len(s.Fields) > 0 {
		fmt.Fprintf(&g.models, "type %s struct {\n", s.Name)
		for _, f := range s.Fields {
			goType := aqlToGoJSONType(f.AQLType, !f.IsRequired)
			if f.IsMulti {
				goType = "[]" + aqlToGoJSONType(f.AQLType, false)
			}
			fieldName := toGoFieldName(f.Name)
			fmt.Fprintf(&g.models, "\t%s %s `json:%q`\n", fieldName, goType, f.Name)
		}
		fmt.Fprintf(&g.models, "}\n\n")
	} else {
		baseType := aqlToGoType(s.Base, false)
		fmt.Fprintf(&g.models, "type %s = %s\n\n", s.Name, baseType)
	}
	return nil
}
func (g *GoGenerator) OnEnum(_ *codegen.Context, e codegen.EnumDescriptor) error {
	// Emit a string-typed const block for each enum.
	fmt.Fprintf(&g.models, "type %s string\n\n", e.Name)
	fmt.Fprintf(&g.models, "const (\n")
	for _, v := range e.Values {
		fmt.Fprintf(&g.models, "\t%s%s %s = %q\n", e.Name, v, e.Name, v)
	}
	fmt.Fprintf(&g.models, ")\n\n")
	return nil
}

func (g *GoGenerator) BeginType(_ *codegen.Context, typ codegen.TypeDescriptor) error {
	g.currentType = typ
	if typ.IsAbstract {
		// Abstract types have no table and are never queried directly; their
		// properties are already inlined into the concrete types that extend
		// them, so don't emit a model struct for them.
		return nil
	}
	fmt.Fprintf(&g.models, "type %s struct {\n", typ.Name)
	return nil
}

func (g *GoGenerator) OnProperty(_ *codegen.Context, p codegen.PropertyDescriptor) error {
	if g.currentType.IsAbstract {
		return nil
	}
	goType := aqlToGoType(p.AQLType, !p.IsRequired)
	if p.EnumType != "" {
		// Enum-backed column → use the generated enum type (a string alias).
		goType = p.EnumType
		if !p.IsRequired {
			goType = "*" + goType
		}
	}
	if p.IsMulti {
		// `multi` scalars are array columns (TEXT[], …). A nil slice is the null,
		// so an optional one takes no pointer — build the element type clean.
		elem := aqlToGoType(p.AQLType, false)
		if p.EnumType != "" {
			elem = p.EnumType
		}
		goType = "[]" + elem
	}
	fieldName := toGoFieldName(p.Name)
	fmt.Fprintf(&g.models, "\t%s %s `json:%q db:%q`\n", fieldName, goType, p.Name, p.Column)
	return nil
}

func (g *GoGenerator) OnLink(_ *codegen.Context, l codegen.LinkDescriptor) error {
	if g.currentType.IsAbstract {
		return nil
	}
	if l.IsMulti {
		// Multi-links are not direct struct fields (they live in junction tables).
		// Generators that want them can handle the LinkDescriptor themselves.
		return nil
	}
	// Single link — expose as the FK column value (string UUID). The json and db
	// tags must both name the actual FK column so scanning and (de)serialization
	// stay consistent.
	fieldName := toGoFieldName(l.Name + "_id")
	col := l.JoinColumn
	nullable := !l.IsRequired
	goType := "string"
	if nullable {
		goType = "*string"
	}
	fmt.Fprintf(&g.models, "\t%s %s `json:%q db:%q`\n", fieldName, goType, col, col)
	return nil
}

func (g *GoGenerator) OnComputed(_ *codegen.Context, _ codegen.ComputedDescriptor) error {
	return nil
}
func (g *GoGenerator) OnIndex(_ *codegen.Context, _ codegen.IndexDescriptor) error { return nil }

func (g *GoGenerator) EndType(_ *codegen.Context) error {
	if g.currentType.IsAbstract {
		return nil
	}
	fmt.Fprintf(&g.models, "}\n\n")
	return nil
}

// EndSchema writes models.go and the queries.go header.
func (g *GoGenerator) EndSchema(ctx *codegen.Context) error {
	header := "// Code generated by axel codegen --generator go. DO NOT EDIT.\n\n"
	// Directive-named query types (@request/@response) are hoisted here so they
	// are declared once and shared across query files in the same package.
	modelsImports := neededImports(g.models.String() + g.namedTypes.String())

	var modelsSrc bytes.Buffer
	modelsSrc.WriteString(header)
	fmt.Fprintf(&modelsSrc, "package %s\n\n", g.pkgName)
	if len(modelsImports) > 0 {
		modelsSrc.WriteString("import (\n")
		for _, imp := range modelsImports {
			fmt.Fprintf(&modelsSrc, "\t%q\n", imp)
		}
		modelsSrc.WriteString(")\n\n")
	}
	modelsSrc.Write(g.models.Bytes())
	modelsSrc.Write(g.namedTypes.Bytes())

	formatted, err := format.Source(modelsSrc.Bytes())
	if err != nil {
		// Write unformatted if gofmt fails (e.g. empty schema)
		formatted = modelsSrc.Bytes()
	}
	if err := ctx.WriteFile("models.go", formatted); err != nil {
		return err
	}
	return g.emitRunner(ctx)
}

// OnQuery emits a typed query function into its own .go file named after the query.
// e.g. list_post.query.aql → list_post.go
func (g *GoGenerator) OnQuery(ctx *codegen.Context, q codegen.QueryDescriptor) error {
	var body bytes.Buffer

	funcName := toGoExportedName(q.Name)
	paramsType := q.RequestType(funcName + "Params")
	rowType := q.ResponseType(funcName + "Row")
	_, paramsNamed := q.Directive("request")
	_, rowNamed := q.Directive("response")

	// Params struct. A @request-named type is hoisted into models.go and emitted
	// once (deduped/reused); the default XxxParams stays inline in the query file.
	if len(q.Params) > 0 {
		if paramsNamed {
			if !g.emitted[paramsType] {
				emitParamsStruct(&g.namedTypes, paramsType, q.Params)
				g.emitted[paramsType] = true
			}
		} else {
			emitParamsStruct(&body, paramsType, q.Params)
		}
	}

	// Result types. A @response-named type is likewise hoisted + deduped.
	switch {
	case q.Result.IsScalar:
		// count/aggregate — no row struct needed
	case q.Operation == "delete":
		// DELETE returns no rows
	case rowNamed:
		if !g.emitted[rowType] {
			emitRowTypes(&g.namedTypes, rowType, q.Result.Fields)
			g.emitted[rowType] = true
		}
	default:
		emitRowTypes(&body, rowType, q.Result.Fields)
	}

	// Function body. When the schema declares globals, functions take a trailing
	// opts ...Option and can run inside a transaction that sets the globals.
	hasGlobals := len(g.schema.Globals) > 0
	switch q.Operation {
	case "select":
		if q.Result.IsScalar {
			emitScalarFunc(&body, funcName, paramsType, q, hasGlobals)
		} else if q.Result.IsMultiple {
			emitMultiRowFunc(&body, funcName, paramsType, rowType, q, hasGlobals)
		} else {
			emitOneRowFunc(&body, funcName, paramsType, rowType, q, hasGlobals)
		}
	case "insert", "update":
		emitOneRowFunc(&body, funcName, paramsType, rowType, q, hasGlobals)
	case "delete":
		emitDeleteFunc(&body, funcName, paramsType, q, hasGlobals)
	}

	// Build full file with package + imports.
	bs := body.String()
	importSet := map[string]bool{
		"context": true,
	}
	// The db parameter is now the in-package DBTX interface, so pgxpool is only
	// imported when the body actually references it (it no longer does).
	if strings.Contains(bs, "pgxpool.") {
		importSet["github.com/jackc/pgx/v5/pgxpool"] = true
	}
	if strings.Contains(bs, "pgx.") {
		importSet["github.com/jackc/pgx/v5"] = true
	}
	if strings.Contains(bs, "errors.Is") {
		importSet["errors"] = true
	}
	for _, imp := range neededImports(bs) {
		importSet[imp] = true
	}

	var src bytes.Buffer
	src.WriteString("// Code generated by axel codegen --generator go. DO NOT EDIT.\n\n")
	fmt.Fprintf(&src, "package %s\n\nimport (\n", g.pkgName)
	for imp := range importSet {
		fmt.Fprintf(&src, "\t%q\n", imp)
	}
	src.WriteString(")\n\n")
	src.Write(body.Bytes())

	formatted, err := format.Source(src.Bytes())
	if err != nil {
		formatted = src.Bytes()
	}

	outFile := queryGoFileName(q) + ".go"
	if err := ctx.WriteFile(outFile, formatted); err != nil {
		return err
	}
	g.queries = append(g.queries, q)
	return nil
}

// emitRunner writes runner.go — a Runner with Execute for inline AQL and typed convenience methods.
func (g *GoGenerator) emitRunner(ctx *codegen.Context) error {
	schemaJSON, err := json.Marshal(g.schema)
	if err != nil {
		return fmt.Errorf("marshaling schema: %w", err)
	}

	var body bytes.Buffer

	// Embedded schema constant
	fmt.Fprintf(&body, "const axelSchema = %s\n\n", backtick(string(schemaJSON)))

	hasGlobals := len(g.schema.Globals) > 0

	// DBTX — the query surface shared by *pgxpool.Pool and pgx.Tx, so the generated
	// query functions run on either. The global setters use it to run the caller's
	// queries inside a transaction.
	fmt.Fprintf(&body, "type DBTX interface {\n")
	fmt.Fprintf(&body, "\tQuery(context.Context, string, ...any) (pgx.Rows, error)\n")
	fmt.Fprintf(&body, "\tQueryRow(context.Context, string, ...any) pgx.Row\n")
	fmt.Fprintf(&body, "\tExec(context.Context, string, ...any) (pgconn.CommandTag, error)\n")
	if hasGlobals {
		fmt.Fprintf(&body, "\tBegin(context.Context) (pgx.Tx, error)\n")
	}
	fmt.Fprintf(&body, "}\n\n")

	// Global options — the functional-options form, for setting globals on the
	// standalone query functions (CreateUser(ctx, db, params, WithCurrentUser(id)))
	// without going through the Runner. beginWithGlobals opens a transaction and
	// applies each option's set_config; the query functions defer commit/rollback.
	if hasGlobals {
		fmt.Fprintf(&body, "type globalOpts struct{ settings map[string]string }\n\n")
		fmt.Fprintf(&body, "// Option sets a global for the duration of a single query call.\n")
		fmt.Fprintf(&body, "type Option func(*globalOpts)\n\n")
		for _, gl := range g.schema.Globals {
			ctor := "With" + toGoExportedName(gl.Name)
			valType := aqlToGoType(gl.AQLType, false)
			fmt.Fprintf(&body, "// %s sets the %q global for this call.\n", ctor, gl.Name)
			fmt.Fprintf(&body, "func %s(value %s) Option {\n", ctor, valType)
			fmt.Fprintf(&body, "\treturn func(o *globalOpts) { o.settings[%q] = fmt.Sprint(value) }\n", "app."+gl.Name)
			fmt.Fprintf(&body, "}\n\n")
		}
		fmt.Fprintf(&body, "func beginWithGlobals(ctx context.Context, db DBTX, opts []Option) (pgx.Tx, func(error) error, error) {\n")
		fmt.Fprintf(&body, "\to := globalOpts{settings: map[string]string{}}\n")
		fmt.Fprintf(&body, "\tfor _, opt := range opts {\n\t\topt(&o)\n\t}\n")
		fmt.Fprintf(&body, "\ttx, err := db.Begin(ctx)\n")
		fmt.Fprintf(&body, "\tif err != nil {\n\t\treturn nil, nil, err\n\t}\n")
		fmt.Fprintf(&body, "\tfor name, value := range o.settings {\n")
		fmt.Fprintf(&body, "\t\tif _, err := tx.Exec(ctx, \"select set_config($1, $2, true)\", name, value); err != nil {\n")
		fmt.Fprintf(&body, "\t\t\t_ = tx.Rollback(ctx)\n\t\t\treturn nil, nil, err\n\t\t}\n\t}\n")
		fmt.Fprintf(&body, "\tdone := func(err error) error {\n")
		fmt.Fprintf(&body, "\t\tif err != nil {\n\t\t\t_ = tx.Rollback(ctx)\n\t\t\treturn err\n\t\t}\n")
		fmt.Fprintf(&body, "\t\treturn tx.Commit(ctx)\n\t}\n")
		fmt.Fprintf(&body, "\treturn tx, done, nil\n")
		fmt.Fprintf(&body, "}\n\n")
	}

	// Queries struct — holds all typed compiled-query methods.
	fmt.Fprintf(&body, "type Queries struct {\n")
	fmt.Fprintf(&body, "\tdb DBTX\n")
	fmt.Fprintf(&body, "}\n\n")
	fmt.Fprintf(&body, "// NewQueries creates a new Queries instance bound to the given DBTX (e.g. a transaction or pool).\n")
	fmt.Fprintf(&body, "func NewQueries(db DBTX) *Queries {\n")
	fmt.Fprintf(&body, "\treturn &Queries{db: db}\n")
	fmt.Fprintf(&body, "}\n\n")
	fmt.Fprintf(&body, "// WithDB returns a new Queries instance bound to the given DBTX (e.g. a transaction).\n")
	fmt.Fprintf(&body, "func (q *Queries) WithDB(db DBTX) *Queries {\n")
	fmt.Fprintf(&body, "\treturn &Queries{db: db}\n")
	fmt.Fprintf(&body, "}\n\n")

	for _, q := range g.queries {
		funcName := toGoExportedName(q.Name)
		paramsType := q.RequestType(funcName + "Params")
		rowType := q.ResponseType(funcName + "Row")
		hasParams := len(q.Params) > 0

		if hasParams {
			fmt.Fprintf(&body, "func (q *Queries) %s(ctx context.Context, params %s) ", funcName, paramsType)
		} else {
			fmt.Fprintf(&body, "func (q *Queries) %s(ctx context.Context) ", funcName)
		}

		switch q.Operation {
		case "select":
			if q.Result.IsScalar {
				fmt.Fprintf(&body, "(int64, error) {\n")
			} else if q.Result.IsMultiple {
				fmt.Fprintf(&body, "([]%s, error) {\n", rowType)
			} else {
				fmt.Fprintf(&body, "(*%s, error) {\n", rowType)
			}
		case "insert", "update":
			fmt.Fprintf(&body, "(*%s, error) {\n", rowType)
		case "delete":
			fmt.Fprintf(&body, "error {\n")
		}

		if hasParams {
			fmt.Fprintf(&body, "\treturn %s(ctx, q.db, params)\n}\n\n", funcName)
		} else {
			fmt.Fprintf(&body, "\treturn %s(ctx, q.db)\n}\n\n", funcName)
		}
	}

	// Runner struct
	fmt.Fprintf(&body, "type Runner struct {\n")
	fmt.Fprintf(&body, "\tdb    *pgxpool.Pool\n")
	fmt.Fprintf(&body, "\tinner *runner.Runner\n")
	fmt.Fprintf(&body, "\tQuery *Queries\n")
	fmt.Fprintf(&body, "}\n\n")

	// NewRunner constructor — parses embedded schema, no arguments beyond db
	fmt.Fprintf(&body, "func NewRunner(db *pgxpool.Pool) *Runner {\n")
	fmt.Fprintf(&body, "\tvar sd codegen.SchemaDescriptor\n")
	fmt.Fprintf(&body, "\tif err := json.Unmarshal([]byte(axelSchema), &sd); err != nil {\n")
	fmt.Fprintf(&body, "\t\tpanic(\"axel: invalid embedded schema: \" + err.Error())\n")
	fmt.Fprintf(&body, "\t}\n")
	fmt.Fprintf(&body, "\treturn &Runner{\n")
	fmt.Fprintf(&body, "\t\tdb:    db,\n")
	fmt.Fprintf(&body, "\t\tinner: runner.New(db, codegen.ToSchemaIR(sd)),\n")
	fmt.Fprintf(&body, "\t\tQuery: &Queries{db: db},\n")
	fmt.Fprintf(&body, "\t}\n")
	fmt.Fprintf(&body, "}\n\n")

	// WithDB returns a Queries instance bound to the given DBTX (e.g. a transaction).
	fmt.Fprintf(&body, "func (r *Runner) WithDB(db DBTX) *Queries {\n")
	fmt.Fprintf(&body, "\treturn &Queries{db: db}\n")
	fmt.Fprintf(&body, "}\n\n")

	// Run — inline AQL execution
	fmt.Fprintf(&body, "func (r *Runner) Run(ctx context.Context, aqlQuery string, params map[string]any) ([]runner.Row, error) {\n")
	fmt.Fprintf(&body, "\tres, err := r.inner.Run(ctx, aqlQuery, params)\n")
	fmt.Fprintf(&body, "\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(&body, "\treturn res.Rows, nil\n")
	fmt.Fprintf(&body, "}\n\n")

	// Global setters — one With<Name> per global. Each opens a transaction, pushes
	// the value into the session via set_config('app.<name>', $1, true) (bound as a
	// parameter, transaction-local), then runs the caller's queries against a
	// Queries bound to that transaction. Rolls back on error. Safe under pooling.
	for _, gl := range g.schema.Globals {
		method := "With" + toGoExportedName(gl.Name)
		valType := aqlToGoType(gl.AQLType, false)
		setSQL := fmt.Sprintf("select set_config('app.%s', $1, true)", gl.Name)
		fmt.Fprintf(&body, "func (r *Runner) %s(ctx context.Context, value %s, fn func(*Queries) error) error {\n", method, valType)
		fmt.Fprintf(&body, "\ttx, err := r.db.Begin(ctx)\n")
		fmt.Fprintf(&body, "\tif err != nil {\n\t\treturn err\n\t}\n")
		fmt.Fprintf(&body, "\tdefer tx.Rollback(ctx)\n")
		fmt.Fprintf(&body, "\tif _, err := tx.Exec(ctx, %q, fmt.Sprint(value)); err != nil {\n\t\treturn err\n\t}\n", setSQL)
		fmt.Fprintf(&body, "\tif err := fn(&Queries{db: tx}); err != nil {\n\t\treturn err\n\t}\n")
		fmt.Fprintf(&body, "\treturn tx.Commit(ctx)\n")
		fmt.Fprintf(&body, "}\n\n")
	}

	var src bytes.Buffer
	src.WriteString("// Code generated by axel codegen --generator go. DO NOT EDIT.\n\n")
	fmt.Fprintf(&src, "package %s\n\n", g.pkgName)
	fmt.Fprintf(&src, "import (\n")
	fmt.Fprintf(&src, "\t\"context\"\n")
	fmt.Fprintf(&src, "\t\"encoding/json\"\n")
	if len(g.schema.Globals) > 0 {
		fmt.Fprintf(&src, "\t\"fmt\"\n")
	}
	fmt.Fprintf(&src, "\n")
	fmt.Fprintf(&src, "\t%q\n", "github.com/jackc/pgx/v5")
	fmt.Fprintf(&src, "\t%q\n", "github.com/jackc/pgx/v5/pgconn")
	fmt.Fprintf(&src, "\t%q\n", "github.com/jackc/pgx/v5/pgxpool")
	fmt.Fprintf(&src, "\t%q\n", modulePath+"/core/codegen")
	fmt.Fprintf(&src, "\t%q\n", modulePath+"/core/runner")
	fmt.Fprintf(&src, ")\n\n")
	src.Write(body.Bytes())

	formatted, err := format.Source(src.Bytes())
	if err != nil {
		formatted = src.Bytes()
	}
	return ctx.WriteFile("runner.go", formatted)
}

// queryGoFileName derives the output .go filename from the query descriptor.
// Uses the source file base name (all extensions stripped), falling back to the query name.
func queryGoFileName(q codegen.QueryDescriptor) string {
	if q.File != "" {
		base := filepath.Base(q.File)
		for ext := filepath.Ext(base); ext != ""; ext = filepath.Ext(base) {
			base = strings.TrimSuffix(base, ext)
		}
		return base
	}
	// Fallback: convert camelCase name to snake_case
	return camelToSnake(q.Name)
}

// camelToSnake converts camelCase to snake_case.
func camelToSnake(s string) string {
	var out strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' && i > 0 {
			out.WriteByte('_')
		}
		out.WriteRune(r | 0x20) // toLower
	}
	return out.String()
}

// --- emit helpers ---

// emitParamsStruct writes a params struct with one field per query parameter,
// preferring the enum type for enum-backed params.
func emitParamsStruct(buf *bytes.Buffer, name string, params []codegen.ParamDescriptor) {
	fmt.Fprintf(buf, "type %s struct {\n", name)
	for _, p := range params {
		goType := aqlToGoType(p.AQLType, p.IsOptional)
		if p.EnumType != "" {
			// Enum-backed param → use the generated enum type (a string alias).
			goType = p.EnumType
			if p.IsOptional {
				goType = "*" + goType
			}
		}
		if p.IsMulti {
			// A `multi` param binds one array value. A nil slice is the null, so an
			// optional one takes no pointer — build the element type clean.
			elem := aqlToGoType(p.AQLType, false)
			if p.EnumType != "" {
				elem = p.EnumType
			}
			goType = "[]" + elem
		}
		fmt.Fprintf(buf, "\t%s %s\n", toGoExportedName(p.Name), goType)
	}
	fmt.Fprintf(buf, "}\n\n")
}

func emitRowTypes(buf *bytes.Buffer, rootName string, fields []codegen.ResultField) {
	fmt.Fprintf(buf, "type %s struct {\n", rootName)
	for _, f := range fields {
		fieldName := toGoExportedName(f.Name)
		if len(f.SubFields) > 0 {
			subType := rootName + toGoExportedName(f.Name)
			if f.IsMultiple {
				fmt.Fprintf(buf, "\t%s []%s `json:%q db:%q`\n", fieldName, subType, f.Name, f.Name)
			} else {
				nullable := ""
				if f.IsNullable {
					nullable = "*"
				}
				fmt.Fprintf(buf, "\t%s %s%s `json:%q db:%q`\n", fieldName, nullable, subType, f.Name, f.Name)
			}
		} else {
			goType := aqlToGoType(f.AQLType, f.IsNullable)
			if f.EnumType != "" {
				// Enum-backed column → use the generated enum type (a string alias).
				goType = f.EnumType
				if f.IsNullable {
					goType = "*" + goType
				}
			}
			if f.IsMultiple {
				elem := aqlToGoType(f.AQLType, false)
				if f.EnumType != "" {
					elem = f.EnumType
				}
				goType = "[]" + elem
			}
			fmt.Fprintf(buf, "\t%s %s `json:%q db:%q`\n", fieldName, goType, f.Name, f.Name)
		}
	}
	fmt.Fprintf(buf, "}\n\n")

	// Emit nested sub-types
	for _, f := range fields {
		if len(f.SubFields) > 0 {
			emitRowTypes(buf, rootName+toGoExportedName(f.Name), f.SubFields)
		}
	}
}

func emitScalarFunc(buf *bytes.Buffer, funcName, paramsType string, q codegen.QueryDescriptor, hasGlobals bool) {
	sqlLit := backtick(q.SQL)
	paramArgs := buildParamArgs(q.Params)

	fmt.Fprintf(buf, "func %s(%s) %s {\n", funcName, funcSig(q, paramsType, hasGlobals), returnSig("int64", hasGlobals))
	emitGlobalsPrologue(buf, "0, ", hasGlobals)
	fmt.Fprintf(buf, "\tconst query = %s\n", sqlLit)
	fmt.Fprintf(buf, "\tvar count int64\n")
	fmt.Fprintf(buf, "\terr := db.QueryRow(ctx, query%s).Scan(&count)\n", paramArgs)
	fmt.Fprintf(buf, "\treturn count, err\n")
	fmt.Fprintf(buf, "}\n")
}

// emitOneRowFunc emits a function returning a single *Row via pgx struct
// scanning. Used for single-row selects and INSERT/UPDATE ... RETURNING.
func emitOneRowFunc(buf *bytes.Buffer, funcName, paramsType, rowType string, q codegen.QueryDescriptor, hasGlobals bool) {
	sqlLit := backtick(q.SQL)
	paramArgs := buildParamArgs(q.Params)

	fmt.Fprintf(buf, "func %s(%s) %s {\n", funcName, funcSig(q, paramsType, hasGlobals), returnSig("*"+rowType, hasGlobals))
	emitGlobalsPrologue(buf, "nil, ", hasGlobals)
	fmt.Fprintf(buf, "\tconst query = %s\n", sqlLit)
	fmt.Fprintf(buf, "\trows, err := db.Query(ctx, query%s)\n", paramArgs)
	fmt.Fprintf(buf, "\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(buf, "\tr, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[%s])\n", rowType)
	fmt.Fprintf(buf, "\tif err != nil {\n")
	fmt.Fprintf(buf, "\t\tif errors.Is(err, pgx.ErrNoRows) {\n\t\t\treturn nil, nil\n\t\t}\n")
	fmt.Fprintf(buf, "\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(buf, "\treturn &r, nil\n")
	fmt.Fprintf(buf, "}\n")
}

func emitMultiRowFunc(buf *bytes.Buffer, funcName, paramsType, rowType string, q codegen.QueryDescriptor, hasGlobals bool) {
	sqlLit := backtick(q.SQL)
	paramArgs := buildParamArgs(q.Params)

	fmt.Fprintf(buf, "func %s(%s) %s {\n", funcName, funcSig(q, paramsType, hasGlobals), returnSig("[]"+rowType, hasGlobals))
	emitGlobalsPrologue(buf, "nil, ", hasGlobals)
	fmt.Fprintf(buf, "\tconst query = %s\n", sqlLit)
	fmt.Fprintf(buf, "\trows, err := db.Query(ctx, query%s)\n", paramArgs)
	fmt.Fprintf(buf, "\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(buf, "\treturn pgx.CollectRows(rows, pgx.RowToStructByName[%s])\n", rowType)
	fmt.Fprintf(buf, "}\n")
}

func emitDeleteFunc(buf *bytes.Buffer, funcName, paramsType string, q codegen.QueryDescriptor, hasGlobals bool) {
	sqlLit := backtick(q.SQL)
	paramArgs := buildParamArgs(q.Params)

	ret := "error"
	if hasGlobals {
		ret = "(_err error)"
	}
	fmt.Fprintf(buf, "func %s(%s) %s {\n", funcName, funcSig(q, paramsType, hasGlobals), ret)
	emitGlobalsPrologue(buf, "", hasGlobals)
	fmt.Fprintf(buf, "\tconst query = %s\n", sqlLit)
	fmt.Fprintf(buf, "\t_, err := db.Exec(ctx, query%s)\n", paramArgs)
	fmt.Fprintf(buf, "\treturn err\n")
	fmt.Fprintf(buf, "}\n")
}

// returnSig builds the "(T, error)" result list, naming the returns (_res, _err)
// when globals are in play so the deferred commit/rollback can observe the error.
func returnSig(valType string, hasGlobals bool) string {
	if hasGlobals {
		return fmt.Sprintf("(_res %s, _err error)", valType)
	}
	return fmt.Sprintf("(%s, error)", valType)
}

// emitGlobalsPrologue writes the opts→transaction preamble: when any Option is
// passed, begin a transaction, apply the globals via set_config, rebind db to the
// tx, and defer commit-or-rollback. zeroPrefix is the value part of the early
// begin-error return ("nil, ", "0, ", or "" for the error-only signature).
func emitGlobalsPrologue(buf *bytes.Buffer, zeroPrefix string, hasGlobals bool) {
	if !hasGlobals {
		return
	}
	fmt.Fprintf(buf, "\tif len(opts) > 0 {\n")
	fmt.Fprintf(buf, "\t\ttx, done, gerr := beginWithGlobals(ctx, db, opts)\n")
	fmt.Fprintf(buf, "\t\tif gerr != nil {\n\t\t\treturn %sgerr\n\t\t}\n", zeroPrefix)
	fmt.Fprintf(buf, "\t\tdb = tx\n")
	fmt.Fprintf(buf, "\t\tdefer func() { _err = done(_err) }()\n")
	fmt.Fprintf(buf, "\t}\n")
}

// funcSig builds the parameter list for a generated query function, always taking
// a context and a DBTX (satisfied by both *pgxpool.Pool and pgx.Tx), plus params
// if any. Accepting the interface lets the same functions run on a pool or inside
// a transaction — used by the global setters. When globals exist, a trailing
// opts ...Option lets callers set globals without the Runner.
func funcSig(q codegen.QueryDescriptor, paramsType string, hasGlobals bool) string {
	var b strings.Builder
	b.WriteString("ctx context.Context, db DBTX")
	if len(q.Params) > 0 {
		fmt.Fprintf(&b, ", params %s", paramsType)
	}
	if hasGlobals {
		b.WriteString(", opts ...Option")
	}
	return b.String()
}

// buildParamArgs builds the variadic args for db.QueryRowContext/db.QueryContext.
// Returns ", params.ID, params.Email" etc.
func buildParamArgs(params []codegen.ParamDescriptor) string {
	if len(params) == 0 {
		return ""
	}
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = "params." + toGoExportedName(p.Name)
	}
	return ", " + strings.Join(parts, ", ")
}

// --- type helpers ---

// aqlToGoType maps an AQL type name to the appropriate Go type.
func aqlToGoType(aqlType string, nullable bool) string {
	base := map[string]string{
		"str":      "string",
		"int16":    "int16",
		"int32":    "int32",
		"int64":    "int64",
		"float32":  "float32",
		"float64":  "float64",
		"bool":     "bool",
		"uuid":     "string",
		"datetime": "time.Time",
		"date":     "time.Time",
		"time":     "time.Time",
		"json":     "json.RawMessage",
		"jsonb":    "json.RawMessage",
		"bytes":    "[]byte",
		"decimal":  "string",
	}[aqlType]

	if base == "" {
		if aqlType != "" {
			base = aqlType
		} else {
			base = "any"
		}
	}

	// These types don't get pointer-ified for nullable
	if base == "json.RawMessage" || base == "[]byte" || base == "any" || strings.HasPrefix(base, "[]") {
		return base
	}
	if nullable {
		return "*" + base
	}
	return base
}

// aqlToGoJSONType maps an AQL type name to a Go type for a field that lives
// inside a JSON/JSONB document rather than in its own column. Temporal types
// are carried as JSON strings (Postgres renders date/time/timestamptz into JSON
// as text, and encoding/json only parses RFC 3339 into time.Time), so they map
// to string here instead of time.Time.
func aqlToGoJSONType(aqlType string, nullable bool) string {
	switch aqlType {
	case "date", "time", "datetime":
		if nullable {
			return "*string"
		}
		return "string"
	}
	return aqlToGoType(aqlType, nullable)
}

// neededImports scans generated source for type references and returns needed imports.
func neededImports(src string) []string {
	var imports []string
	if strings.Contains(src, "time.Time") {
		imports = append(imports, "time")
	}
	if strings.Contains(src, "json.RawMessage") {
		imports = append(imports, "encoding/json")
	}
	return imports
}

// toGoFieldName converts a snake_case name to a Go exported PascalCase name.
func toGoFieldName(s string) string {
	return toGoExportedName(s)
}

// toGoExportedName converts snake_case → PascalCase.
func toGoExportedName(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) == 0 {
			continue
		}
		// Keep common acronyms uppercased
		upper := strings.ToUpper(p)
		if upper == "ID" || upper == "URL" || upper == "UUID" || upper == "API" || upper == "SQL" {
			parts[i] = upper
		} else {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

// backtick wraps a string in backtick quotes, escaping any backticks inside.
func backtick(s string) string {
	// Go raw string literals can't contain backticks; use concatenation if needed.
	if !strings.Contains(s, "`") {
		return "`" + s + "`"
	}
	// Fall back to interpreted string literal
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return `"` + s + `"`
}
