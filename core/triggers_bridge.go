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
}

// Trigger is a resolved-to-SQL trigger. Keyed by Table+Name.
type Trigger struct {
	Name      string // diff key: "<table>.<name>"
	CreateSQL string // full "CREATE TRIGGER … ;"
	DropSQL   string // full "DROP TRIGGER IF EXISTS … ON …;"
}

// SchemaIRToFunctionsAndTriggers lowers rewrites, inline trigger bodies, and
// declared functions into flat, fully-formed SQL Function/Trigger values (sorted
// for deterministic diffs). Inline AQL bodies are compiled here via the AQL
// compiler's trigger context.
func SchemaIRToFunctionsAndTriggers(ir *asl.SchemaIR) ([]Function, []Trigger, error) {
	var fns []Function
	var trigs []Trigger

	// Declared top-level functions.
	fnNames := make([]string, 0, len(ir.Functions))
	for name := range ir.Functions {
		fnNames = append(fnNames, name)
	}
	sort.Strings(fnNames)
	for _, name := range fnNames {
		fn, err := declaredFunctionSQL(ir, ir.Functions[name])
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
func declaredFunctionSQL(ir *asl.SchemaIR, fn *asl.ResolvedFunction) (Function, error) {
	var argDecls, argTypes []string
	var paramNames []string
	for _, p := range fn.Params {
		argDecls = append(argDecls, fmt.Sprintf("%s %s", p.Name, p.SQLType))
		argTypes = append(argTypes, p.SQLType)
		paramNames = append(paramNames, p.Name)
	}

	body, lang, err := functionBody(ir, fn, paramNames)
	if err != nil {
		return Function{}, err
	}

	create := fmt.Sprintf(
		"CREATE OR REPLACE FUNCTION %q(%s) RETURNS %s AS $$\n%s\n$$ LANGUAGE %s;",
		fn.Name, strings.Join(argDecls, ", "), fn.Returns, body, lang,
	)
	drop := fmt.Sprintf("DROP FUNCTION IF EXISTS %q(%s);", fn.Name, strings.Join(argTypes, ", "))
	return Function{Name: fn.Name, CreateSQL: create, DropSQL: drop}, nil
}

// functionBody returns the plpgsql/sql body and language for a declared function.
func functionBody(ir *asl.SchemaIR, fn *asl.ResolvedFunction, paramNames []string) (body, lang string, err error) {
	if fn.BodySQL != "" {
		return fn.BodySQL, fn.Language, nil
	}
	// AQL body: compile as a standalone function (no enclosing type bound).
	stmt, err := aql.ParseString(fn.BodyAQL)
	if err != nil {
		return "", "", fmt.Errorf("body: %w", err)
	}
	sql, err := compiler.CompileTriggerBody(stmt, ir, nil, paramNames)
	if err != nil {
		return "", "", fmt.Errorf("body: %w", err)
	}
	if fn.Returns == "trigger" {
		return fmt.Sprintf("BEGIN\n  %s;\n  RETURN COALESCE(NEW, OLD);\nEND;", sql), "plpgsql", nil
	}
	return fmt.Sprintf("BEGIN\n  RETURN (%s);\nEND;", sql), "plpgsql", nil
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
