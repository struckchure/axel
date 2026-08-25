package axel

import (
	"fmt"
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
	"github.com/struckchure/axel/core/compiler"
)

// Function is a resolved-to-SQL Postgres function ready to emit. Everything
// (declared functions, rewrite functions, inline trigger bodies) is lowered to
// this shape so the diff layer only ever compares concrete SQL by string.
type Function struct {
	Name      string // diff key
	CreateSQL string // full "CREATE OR REPLACE FUNCTION … ;"
	DropSQL   string // full "DROP FUNCTION IF EXISTS …(argtypes);"
	RunOnce   bool   // @for setup function: invoked once (SELECT fn()) when first added
}

// Policy is a resolved-to-SQL row-level-security policy. Keyed by Table+Name.
// CreateSQL enables RLS on the table (idempotent) and creates the policy.
type Policy struct {
	Name      string // diff key: "<table>.<name>"
	CreateSQL string // "ALTER TABLE … ENABLE ROW LEVEL SECURITY;\nCREATE POLICY … ;"
	DropSQL   string // "DROP POLICY IF EXISTS … ON …;"
}

// SchemaIRToPolicies lowers each concrete type's policies into flat SQL Policy
// values (sorted for deterministic diffs). Each policy's raw AQL predicate is
// parsed and compiled to SQL here (not in the resolver) so this package can reach
// the AQL compiler without an import cycle — the same pattern used for inline
// trigger bodies.
func SchemaIRToPolicies(ir *asl.SchemaIR) ([]Policy, error) {
	typeNames := make([]string, 0, len(ir.ObjectTypes))
	for name, rt := range ir.ObjectTypes {
		if rt.IsAbstract || rt.Table == "" {
			continue
		}
		typeNames = append(typeNames, name)
	}
	sort.Strings(typeNames)

	var pols []Policy
	for _, name := range typeNames {
		rt := ir.ObjectTypes[name]
		// Use the same table-name derivation as CREATE TABLE (lo.SnakeCase on the
		// type name) so the policy targets the actual table.
		table := lo.SnakeCase(name)
		for _, pol := range rt.Policies {
			usingSQL, err := compilePolicyPredicate(pol.UsingAQL, rt, ir)
			if err != nil {
				return nil, fmt.Errorf("policy %q on %q using: %w", pol.Name, name, err)
			}
			checkSQL, err := compilePolicyPredicate(pol.CheckAQL, rt, ir)
			if err != nil {
				return nil, fmt.Errorf("policy %q on %q with check: %w", pol.Name, name, err)
			}

			// Postgres CREATE POLICY takes a single command, so a policy that
			// targets several (for update, delete) lowers to one statement per
			// command. Suffix the name per command only when there is more than
			// one, so single-command policies keep their declared name.
			for _, cmd := range pol.Commands {
				name := pol.Name
				if len(pol.Commands) > 1 {
					name = pol.Name + "_" + cmd
				}

				var b strings.Builder
				fmt.Fprintf(&b, "ALTER TABLE %q ENABLE ROW LEVEL SECURITY;\n", table)
				fmt.Fprintf(&b, "CREATE POLICY %q ON %q FOR %s", name, table, strings.ToUpper(cmd))
				if len(pol.Roles) > 0 {
					fmt.Fprintf(&b, " TO %s", strings.Join(pol.Roles, ", "))
				}
				if usingSQL != "" {
					fmt.Fprintf(&b, " USING (%s)", usingSQL)
				}
				if checkSQL != "" {
					fmt.Fprintf(&b, " WITH CHECK (%s)", checkSQL)
				}
				b.WriteString(";")
				pols = append(pols, Policy{
					Name:      table + "." + name,
					CreateSQL: b.String(),
					DropSQL:   fmt.Sprintf("DROP POLICY IF EXISTS %q ON %q;", name, table),
				})
			}
		}
	}
	return pols, nil
}

// compilePolicyPredicate parses a raw AQL policy predicate and lowers it to a SQL
// RLS predicate. An empty source (omitted clause) yields "".
func compilePolicyPredicate(src string, rt *asl.ResolvedType, ir *asl.SchemaIR) (string, error) {
	if src == "" {
		return "", nil
	}
	expr, err := aql.ParseExpr(src)
	if err != nil {
		return "", err
	}
	return compiler.CompilePolicyPredicate(expr, rt, ir)
}

// Trigger is a resolved-to-SQL trigger. Keyed by Table+Name.
type Trigger struct {
	Name      string // diff key: "<table>.<name>"
	CreateSQL string // full "CREATE TRIGGER … ;"
	DropSQL   string // full "DROP TRIGGER IF EXISTS … ON …;"
}

// Extension is a resolved-to-SQL Postgres extension. Keyed by Name.
type Extension struct {
	Name      string // diff key: the extension name (e.g. "unaccent")
	CreateSQL string // "CREATE EXTENSION IF NOT EXISTS \"…\";"
	DropSQL   string // "DROP EXTENSION IF EXISTS \"…\";"
}

// SchemaIRToExtensions lowers declared extensions into flat SQL Extension values,
// preserving declaration order.
func SchemaIRToExtensions(ir *asl.SchemaIR) []Extension {
	exts := make([]Extension, 0, len(ir.Extensions))
	for _, name := range ir.Extensions {
		exts = append(exts, Extension{
			Name:      name,
			CreateSQL: fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %q;", name),
			DropSQL:   fmt.Sprintf("DROP EXTENSION IF EXISTS %q;", name),
		})
	}
	return exts
}

// SchemaIRToFunctionsAndTriggers lowers rewrites, inline trigger bodies, and
// declared functions into flat, fully-formed SQL Function/Trigger values (sorted
// for deterministic diffs). Inline AQL bodies are compiled here via the AQL
// compiler's trigger context.
func SchemaIRToFunctionsAndTriggers(ir *asl.SchemaIR) ([]Function, []Trigger, error) {
	var fns []Function
	var trigs []Trigger

	// Declared top-level functions (scalar receiver methods are for compiler/codegen only).
	fnNames := make([]string, 0, len(ir.Functions))
	for name, fn := range ir.Functions {
		if fn.ReceiverType != "" {
			continue
		}
		fnNames = append(fnNames, name)
	}
	sort.Strings(fnNames)
	for _, name := range fnNames {
		fn, err := declaredFunctionSQL(ir.Functions[name], ir)
		if err != nil {
			return nil, nil, fmt.Errorf("function %q: %w", name, err)
		}
		fns = append(fns, fn)
	}

	// Concrete types, sorted for deterministic output.
	typeNames := make([]string, 0, len(ir.ObjectTypes))
	for name, rt := range ir.ObjectTypes {
		if rt.IsAbstract {
			continue // abstract types have no table
		}
		typeNames = append(typeNames, name)
	}
	sort.Strings(typeNames)

	// Rewrites → shared functions + per-table triggers.
	rwFns, rwTrigs := lowerRewrites(ir, typeNames)
	fns = append(fns, rwFns...)
	trigs = append(trigs, rwTrigs...)

	// Declared triggers (and their synthesized do-body functions).
	for _, name := range typeNames {
		rt := ir.ObjectTypes[name]
		for _, trg := range rt.Triggers {
			fn, t, err := triggerSQL(ir, rt, trg)
			if err != nil {
				return nil, nil, fmt.Errorf("type %q trigger %q: %w", name, trg.Name, err)
			}
			if fn != nil {
				fns = append(fns, *fn)
			}
			trigs = append(trigs, t)
		}
	}

	return fns, trigs, nil
}

// modelRewriteFn is one declaring model's folded rewrite function.
type modelRewriteFn struct {
	events []string // SQL events, e.g. ["INSERT", "UPDATE"]
	body   string   // plpgsql body
}

// lowerRewrites builds rewrite functions scoped per declaring model and the
// per-table triggers that execute them.
//
// A rewrite belongs to the model that declared it (abstract or concrete). Each
// such model gets ONE function — `axel_rw_<model>_<serial>` — built from that
// model's own rewrites and shared by every concrete type that inherits them. A
// concrete table gets one BEFORE trigger per contributing model. So an
// `updated_at` rewrite on abstract `Base` yields a single `axel_rw_base_1`
// executed by every descendant's trigger, and a concrete type's own rewrite is a
// separate `axel_rw_<that_type>_1`.
func lowerRewrites(ir *asl.SchemaIR, typeNames []string) ([]Function, []Trigger) {
	built := map[string]modelRewriteFn{}  // origin model → its function (built once)
	tableOrigins := map[string][]string{} // table → sorted origin models it uses
	var tableOrder []string               // concrete tables with rewrites, in sorted order

	for _, name := range typeNames {
		rt := ir.ObjectTypes[name]

		// Group this table's rewrites by the model that declared them.
		propNames := make([]string, 0, len(rt.Properties))
		for n := range rt.Properties {
			propNames = append(propNames, n)
		}
		sort.Strings(propNames)

		byOrigin := map[string]map[string][]string{} // origin → event → assignments
		for _, pn := range propNames {
			prop := rt.Properties[pn]
			for _, rw := range prop.Rewrites {
				o := rw.Origin
				if byOrigin[o] == nil {
					byOrigin[o] = map[string][]string{}
				}
				for _, ev := range rw.Events {
					byOrigin[o][ev] = append(byOrigin[o][ev], fmt.Sprintf("NEW.%q := %s;", prop.Column, rw.ValueSQL))
				}
			}
		}
		if len(byOrigin) == 0 {
			continue
		}

		origins := make([]string, 0, len(byOrigin))
		for o := range byOrigin {
			origins = append(origins, o)
			if _, ok := built[o]; !ok {
				events, body := buildRewriteBody(byOrigin[o])
				built[o] = modelRewriteFn{events: events, body: body}
			}
		}
		sort.Strings(origins)
		tableOrigins[rt.Table] = origins
		tableOrder = append(tableOrder, rt.Table)
	}

	// Name each model's function: <axel_rw_model>_<serial>, serial scoped per
	// model (one function per model today, so 1). Sorted for deterministic output.
	origins := make([]string, 0, len(built))
	for o := range built {
		origins = append(origins, o)
	}
	sort.Strings(origins)

	name := map[string]string{}
	serial := map[string]int{}
	var fns []Function
	for _, o := range origins {
		snake := lo.SnakeCase(o)
		serial[snake]++
		fnName := fmt.Sprintf("axel_rw_%s_%d", snake, serial[snake])
		name[o] = fnName
		fns = append(fns, Function{
			Name:      fnName,
			CreateSQL: fmt.Sprintf("CREATE OR REPLACE FUNCTION %q() RETURNS trigger AS $$\n%s\n$$ LANGUAGE plpgsql;", fnName, built[o].body),
			DropSQL:   fmt.Sprintf("DROP FUNCTION IF EXISTS %q();", fnName),
		})
	}

	var trigs []Trigger
	for _, table := range tableOrder {
		for _, o := range tableOrigins[table] {
			mf := built[o]
			trgName := fmt.Sprintf("trg_%s_rewrite_%s", table, lo.SnakeCase(o))
			trigs = append(trigs, Trigger{
				Name: table + ".rewrite." + lo.SnakeCase(o),
				CreateSQL: fmt.Sprintf(
					"CREATE TRIGGER %q BEFORE %s ON %q\n  FOR EACH ROW EXECUTE FUNCTION %q();",
					trgName, strings.Join(mf.events, " OR "), table, name[o],
				),
				DropSQL: fmt.Sprintf("DROP TRIGGER IF EXISTS %q ON %q;", trgName, table),
			})
		}
	}
	return fns, trigs
}

// buildRewriteBody assembles a plpgsql body from per-event assignments and
// returns the SQL events (INSERT/UPDATE) the trigger fires on.
func buildRewriteBody(byEvent map[string][]string) (events []string, body string) {
	var b strings.Builder
	b.WriteString("BEGIN\n")
	for _, ev := range []string{"insert", "update"} { // deterministic order
		if len(byEvent[ev]) == 0 {
			continue
		}
		events = append(events, strings.ToUpper(ev))
		fmt.Fprintf(&b, "  IF TG_OP = '%s' THEN\n", strings.ToUpper(ev))
		for _, a := range byEvent[ev] {
			fmt.Fprintf(&b, "    %s\n", a)
		}
		b.WriteString("  END IF;\n")
	}
	b.WriteString("  RETURN NEW;\nEND;")
	return events, b.String()
}

// declaredFunctionSQL builds the CREATE/DROP for a user-declared function.
func declaredFunctionSQL(fn *asl.ResolvedFunction, ir *asl.SchemaIR) (Function, error) {
	var argDecls, argTypes []string
	for _, p := range fn.Params {
		argDecls = append(argDecls, fmt.Sprintf("%s %s", p.Name, p.SQLType))
		argTypes = append(argTypes, p.SQLType)
	}

	returnSQL, err := inlineFunctionAQL(fn, ir)
	if err != nil {
		return Function{}, err
	}
	body, lang := functionBody(fn, returnSQL)

	create := fmt.Sprintf(
		"CREATE OR REPLACE FUNCTION %q(%s) RETURNS %s AS $$\n%s\n$$ LANGUAGE %s%s;",
		fn.Name, strings.Join(argDecls, ", "), fn.Returns, body, lang, funcAttrSuffix(fn),
	)
	drop := fmt.Sprintf("DROP FUNCTION IF EXISTS %q(%s);", fn.Name, strings.Join(argTypes, ", "))
	return Function{Name: fn.Name, CreateSQL: create, DropSQL: drop, RunOnce: fn.RunOnceFor != ""}, nil
}

// inlineFunctionAQL returns the function's return expression with every inline
// aql`…` literal replaced by the SQL it compiles to, quoted as a Postgres
// string literal. Functions without inline AQL pass through untouched.
func inlineFunctionAQL(fn *asl.ResolvedFunction, ir *asl.SchemaIR) (string, error) {
	if len(fn.InlineAQL) == 0 {
		return fn.ReturnSQL, nil
	}
	// ReturnSQL carries one NUL per literal, in the same order as InlineAQL.
	parts := strings.Split(fn.ReturnSQL, "\x00")
	if len(parts) != len(fn.InlineAQL)+1 {
		return "", fmt.Errorf("internal: %d inline aql markers for %d queries", len(parts)-1, len(fn.InlineAQL))
	}
	var b strings.Builder
	for i, part := range parts {
		b.WriteString(part)
		if i == len(fn.InlineAQL) {
			break
		}
		lit, err := compiler.CompileInline(fn.InlineAQL[i], ir)
		if err != nil {
			return "", fmt.Errorf("inline aql %q: %w", fn.InlineAQL[i], err)
		}
		b.WriteString(lit)
	}
	return b.String(), nil
}

// funcAttrSuffix renders a function's attributes as a SQL suffix (leading space
// on each present attribute) appended after `LANGUAGE <lang>`. Emitted in a fixed
// canonical order so the string-diff stays stable regardless of directive order.
func funcAttrSuffix(fn *asl.ResolvedFunction) string {
	var b strings.Builder
	switch fn.Volatility {
	case "immutable":
		b.WriteString(" IMMUTABLE")
	case "stable":
		b.WriteString(" STABLE")
	case "volatile":
		b.WriteString(" VOLATILE")
	}
	if fn.Strict {
		b.WriteString(" STRICT")
	}
	if fn.Leakproof {
		b.WriteString(" LEAKPROOF")
	}
	switch fn.Security {
	case "definer":
		b.WriteString(" SECURITY DEFINER")
	case "invoker":
		b.WriteString(" SECURITY INVOKER")
	}
	switch fn.Parallel {
	case "safe":
		b.WriteString(" PARALLEL SAFE")
	case "unsafe":
		b.WriteString(" PARALLEL UNSAFE")
	case "restricted":
		b.WriteString(" PARALLEL RESTRICTED")
	}
	if fn.Cost != "" {
		fmt.Fprintf(&b, " COST %s", fn.Cost)
	}
	return b.String()
}

// functionBody returns the plpgsql/sql body and language for a declared
// function. returnSQL is raw Postgres (inline AQL already lowered), wrapped per
// language.
func functionBody(fn *asl.ResolvedFunction, returnSQL string) (body, lang string) {
	if fn.Language == "sql" {
		return fmt.Sprintf("  SELECT %s;", returnSQL), "sql"
	}
	return fmt.Sprintf("BEGIN\n  RETURN %s;\nEND;", returnSQL), fn.Language
}

// triggerSQL builds the SQL for one declared trigger. For an inline do-body it
// also returns the synthesized function; for the execute-form fn is nil.
func triggerSQL(ir *asl.SchemaIR, rt *asl.ResolvedType, trg *asl.ResolvedTrigger) (*Function, Trigger, error) {
	execFn := trg.Function
	var fn *Function

	if trg.DoAQL != "" {
		stmt, err := aql.ParseString(trg.DoAQL)
		if err != nil {
			return nil, Trigger{}, fmt.Errorf("do body: %w", err)
		}
		inner, err := compiler.CompileTriggerBody(stmt, ir, rt, nil)
		if err != nil {
			return nil, Trigger{}, fmt.Errorf("do body: %w", err)
		}
		execFn = fmt.Sprintf("axel_trg_%s_%s", rt.Table, trg.Name)
		body := fmt.Sprintf("BEGIN\n  %s;\n  RETURN COALESCE(NEW, OLD);\nEND;", inner)
		fn = &Function{
			Name:      execFn,
			CreateSQL: fmt.Sprintf("CREATE OR REPLACE FUNCTION %q() RETURNS trigger AS $$\n%s\n$$ LANGUAGE plpgsql;", execFn, body),
			DropSQL:   fmt.Sprintf("DROP FUNCTION IF EXISTS %q();", execFn),
		}
	}

	var events []string
	for _, ev := range trg.Events {
		events = append(events, strings.ToUpper(ev))
	}
	trgName := fmt.Sprintf("trg_%s_%s", rt.Table, trg.Name)

	var when string
	if trg.When != "" {
		when = fmt.Sprintf(" WHEN (%s)", trg.When)
	}

	create := fmt.Sprintf(
		"CREATE TRIGGER %q %s %s ON %q\n  FOR EACH %s%s EXECUTE FUNCTION %q();",
		trgName, strings.ToUpper(trg.Timing), strings.Join(events, " OR "), rt.Table,
		strings.ToUpper(trg.ForEach), when, execFn,
	)
	t := Trigger{
		Name:      rt.Table + "." + trg.Name,
		CreateSQL: create,
		DropSQL:   fmt.Sprintf("DROP TRIGGER IF EXISTS %q ON %q;", trgName, rt.Table),
	}
	return fn, t, nil
}
