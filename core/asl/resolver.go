package asl

import (
	"fmt"
	"slices"
	"strings"

	"github.com/samber/lo"
)

// builtinTypes maps ASL scalar type names to their SQL equivalents.
var builtinTypes = map[string]string{
	"str":      "TEXT",
	"int16":    "SMALLINT",
	"int32":    "INTEGER",
	"int64":    "BIGINT",
	"float32":  "REAL",
	"float64":  "DOUBLE PRECISION",
	"bool":     "BOOLEAN",
	"uuid":     "UUID",
	"datetime": "TIMESTAMPTZ",
	"date":     "DATE",
	"time":     "TIME",
	"json":     "JSONB",
	"bytes":    "BYTEA",
	"decimal":  "NUMERIC",
}

// BuiltinSQLType returns the SQL type for a builtin scalar name (e.g. "str" →
// "TEXT") and whether name is a builtin scalar at all.
func BuiltinSQLType(name string) (string, bool) {
	s, ok := builtinTypes[name]
	return s, ok
}

// Resolver builds a SchemaIR from a parsed SourceFile.
type Resolver struct{}

// Resolve resolves a parsed SourceFile into a SchemaIR.
func (r *Resolver) Resolve(src *SourceFile) (*SchemaIR, error) {
	ir := &SchemaIR{
		ScalarTypes: make(map[string]*ResolvedScalar),
		EnumTypes:   make(map[string]*ResolvedEnum),
		ObjectTypes: make(map[string]*ResolvedType),
		Functions:   make(map[string]*ResolvedFunction),
	}

	// Pass 1: register scalar types, enum types, functions, extensions, globals.
	seenExt := map[string]bool{}
	seenGlobal := map[string]bool{}
	for _, def := range src.Definitions {
		if def.Global != nil {
			g := def.Global
			if seenGlobal[g.Name] {
				return nil, fmt.Errorf("global %q declared more than once", g.Name)
			}
			sqlType, err := r.resolveBaseType(g.Type, ir)
			if err != nil {
				return nil, fmt.Errorf("global %q: %w (globals must be a scalar type)", g.Name, err)
			}
			seenGlobal[g.Name] = true
			ir.Globals = append(ir.Globals, &ResolvedGlobal{
				Name:     g.Name,
				AQLType:  g.Type,
				SQLType:  sqlType,
				Required: g.Required,
			})
		}
		if def.Extension != nil {
			name := stripSingleQuotes(def.Extension.Name)
			if name == "" {
				return nil, fmt.Errorf("extension name may not be empty")
			}
			if !seenExt[name] {
				seenExt[name] = true
				ir.Extensions = append(ir.Extensions, name)
			}
		}
		if def.Function != nil {
			fn, err := r.resolveFunction(def.Function, ir)
			if err != nil {
				return nil, fmt.Errorf("function %q: %w", def.Function.Name, err)
			}
			if _, exists := ir.Functions[fn.Name]; exists {
				return nil, fmt.Errorf("function %q declared more than once", fn.Name)
			}
			ir.Functions[fn.Name] = fn
		}
		switch {
		case def.ScalarType != nil:
			s := def.ScalarType
			sqlType, err := r.resolveBaseType(s.Extends, ir)
			if err != nil {
				return nil, fmt.Errorf("scalar type %q: %w", s.Name, err)
			}
			ir.ScalarTypes[s.Name] = &ResolvedScalar{
				Name:    s.Name,
				Base:    s.Extends,
				SQLType: sqlType,
			}

		case def.EnumType != nil:
			e := def.EnumType
			ir.EnumTypes[e.Name] = &ResolvedEnum{
				Name:   e.Name,
				Values: e.Values,
			}
		}
	}

	// Pass 2: register object types (abstract and concrete) without members.
	for _, def := range src.Definitions {
		if def.TypeDef == nil {
			continue
		}
		t := def.TypeDef
		rt := &ResolvedType{
			Name:       t.Name,
			IsAbstract: t.Abstract,
			Table:      toSnakeCase(t.Name),
			Properties: make(map[string]*ResolvedProp),
			Links:      make(map[string]*ResolvedLink),
			Computed:   make(map[string]*ResolvedComputed),
		}
		if t.Abstract {
			rt.Table = "" // abstract types have no table
		}
		ir.ObjectTypes[t.Name] = rt
	}

	// Pass 2b: detect inheritance (extends) cycles before flattening. Flattening
	// copies each parent's already-resolved members into the child, so a cycle
	// would otherwise produce silently incomplete types. The link/foreign-key
	// graph is intentionally not checked here — self- and mutual references are
	// valid (see Validate).
	parents := make(map[string][]string)
	for _, def := range src.Definitions {
		if def.TypeDef != nil {
			parents[def.TypeDef.Name] = def.TypeDef.Extending
		}
	}
	{
		visited := make(map[string]bool)
		visiting := make(map[string]bool)
		var checkExtends func(name string) error
		checkExtends = func(name string) error {
			if visited[name] {
				return nil
			}
			if visiting[name] {
				return fmt.Errorf("inheritance cycle detected involving type %q", name)
			}
			visiting[name] = true
			for _, parent := range parents[name] {
				if _, known := parents[parent]; !known {
					continue // unknown parent is reported during flattening below
				}
				if err := checkExtends(parent); err != nil {
					return err
				}
			}
			visiting[name] = false
			visited[name] = true
			return nil
		}
		for _, def := range src.Definitions {
			if def.TypeDef == nil {
				continue
			}
			if err := checkExtends(def.TypeDef.Name); err != nil {
				return nil, err
			}
		}
	}

	// Pass 3: resolve members for each type (with inheritance).
	for _, def := range src.Definitions {
		if def.TypeDef == nil {
			continue
		}
		t := def.TypeDef
		rt := ir.ObjectTypes[t.Name]

		// Inherit from parent types first.
		for _, parentName := range t.Extending {
			parent, ok := ir.ObjectTypes[parentName]
			if !ok {
				return nil, fmt.Errorf("type %q extends unknown type %q", t.Name, parentName)
			}
			for k, v := range parent.Properties {
				rt.Properties[k] = v
			}
			for k, v := range parent.Links {
				rt.Links[k] = v
			}
			for k, v := range parent.Computed {
				rt.Computed[k] = v
			}
			rt.Indexes = append(rt.Indexes, parent.Indexes...)
			rt.Constraints = append(rt.Constraints, parent.Constraints...)
			rt.Triggers = append(rt.Triggers, parent.Triggers...)
			rt.Policies = append(rt.Policies, parent.Policies...)
		}

		// Resolve own members.
		for _, m := range t.Members {
			if err := r.resolveMember(m, rt, ir); err != nil {
				return nil, fmt.Errorf("type %q: %w", t.Name, err)
			}
		}

		// Policies resolve after all members so `.field` refs see every column.
		for _, m := range t.Members {
			if m.Policy == nil {
				continue
			}
			pol, err := r.resolvePolicy(m.Policy, rt)
			if err != nil {
				return nil, fmt.Errorf("type %q: %w", t.Name, err)
			}
			rt.Policies = append(rt.Policies, pol)
		}
	}

	// Pass 4: validate that every trigger's execute target is a declared function
	// returning trigger (functions are all registered by now).
	for _, rt := range ir.ObjectTypes {
		for _, trg := range rt.Triggers {
			if trg.Function == "" {
				continue
			}
			fn, ok := ir.Functions[trg.Function]
			if !ok {
				return nil, fmt.Errorf("type %q trigger %q executes unknown function %q", rt.Name, trg.Name, trg.Function)
			}
			if fn.Returns != "trigger" {
				return nil, fmt.Errorf("type %q trigger %q executes function %q which does not return trigger", rt.Name, trg.Name, trg.Function)
			}
		}
	}

	return ir, nil
}

func (r *Resolver) resolveMember(m *Member, rt *ResolvedType, ir *SchemaIR) error {
	switch {
	case m.Computed != nil:
		rt.Computed[m.Computed.Name] = &ResolvedComputed{
			Name: m.Computed.Name,
			Expr: strings.Join(m.Computed.Parts, ""),
		}

	case m.Index != nil:
		idx := &ResolvedIndex{}
		for _, f := range m.Index.Fields {
			idx.Columns = append(idx.Columns, toSnakeCase(f))
		}
		rt.Indexes = append(rt.Indexes, idx)

	case m.Constraint != nil:
		tc := &ResolvedTypeConstraint{
			Expression: m.Constraint.Expression,
			Args:       m.Constraint.Args,
		}
		if m.Constraint.Filter != nil {
			tc.FilterAQL = m.Constraint.Filter.AQL()
		}
		for _, f := range m.Constraint.Fields {
			tc.Columns = append(tc.Columns, toSnakeCase(f))
		}
		rt.Constraints = append(rt.Constraints, tc)

	case m.Trigger != nil:
		trg, err := r.resolveTrigger(m.Trigger)
		if err != nil {
			return err
		}
		rt.Triggers = append(rt.Triggers, trg)

	case m.Field != nil:
		if err := r.resolveField(m.Field, rt, ir); err != nil {
			return err
		}
	}
	return nil
}

// resolvePolicy resolves a PolicyDecl to a ResolvedPolicy. The predicates are
// captured as raw native AQL; they are parsed and lowered to SQL later, in the
// migration bridge (SchemaIRToPolicies), so field-reference errors surface there —
// the same way inline trigger AQL bodies are handled. rt is unused here for that
// reason.
func (r *Resolver) resolvePolicy(pd *PolicyDecl, _ *ResolvedType) (*ResolvedPolicy, error) {
	commands := make([]string, 0, len(pd.Commands))
	for _, raw := range pd.Commands {
		cmd := strings.ToLower(raw)
		switch cmd {
		case "select", "insert", "update", "delete", "all":
		default:
			return nil, fmt.Errorf("policy %q: invalid command %q (want select|insert|update|delete|all)", pd.Name, raw)
		}
		commands = append(commands, cmd)
	}

	pol := &ResolvedPolicy{Name: pd.Name, Commands: commands, Roles: pd.Roles}
	if pd.Using != nil {
		pol.UsingAQL = pd.Using.AQL()
	}
	if pd.Check != nil {
		pol.CheckAQL = pd.Check.AQL()
	}
	if pol.UsingAQL == "" && pol.CheckAQL == "" {
		return nil, fmt.Errorf("policy %q: needs a using ( … ) or with check ( … ) clause", pd.Name)
	}

	// Postgres restricts which clauses each command accepts. `using` reads existing
	// rows (select/update/delete); `with check` validates new rows (insert/update).
	// Reject invalid combinations here so a multi-command policy that expands to an
	// illegal CREATE POLICY (e.g. `for delete with check`) fails at validate time
	// rather than at apply time.
	for _, cmd := range commands {
		if pol.CheckAQL != "" && cmd != "insert" && cmd != "update" && cmd != "all" {
			return nil, fmt.Errorf("policy %q: `with check` is not allowed for %s (Postgres allows it only on insert, update, all)", pd.Name, cmd)
		}
		if pol.UsingAQL != "" && cmd != "select" && cmd != "update" && cmd != "delete" && cmd != "all" {
			return nil, fmt.Errorf("policy %q: `using` is not allowed for %s (Postgres allows it only on select, update, delete, all)", pd.Name, cmd)
		}
	}
	return pol, nil
}

// resolveTrigger resolves a TriggerDecl to a ResolvedTrigger (the execute-target
// existence check happens in a later pass, once all functions are registered).
func (r *Resolver) resolveTrigger(td *TriggerDecl) (*ResolvedTrigger, error) {
	events := make([]string, 0, len(td.Events))
	for _, ev := range td.Events {
		ev = canonEvent(ev)
		if ev != "insert" && ev != "update" && ev != "delete" {
			return nil, fmt.Errorf("trigger %q: invalid event %q (want insert|update|delete)", td.Name, ev)
		}
		events = append(events, ev)
	}
	forEach := td.ForEach
	if forEach == "" {
		forEach = "row"
	}
	trg := &ResolvedTrigger{
		Name:    td.Name,
		Timing:  td.Timing,
		Events:  events,
		ForEach: forEach,
	}
	if td.When != nil {
		trg.When = stripDollarQuotes(*td.When)
	}
	if td.Do != nil {
		trg.DoAQL = td.Do.Raw
	}
	if td.ExecFn != nil {
		trg.Function = *td.ExecFn
	}
	return trg, nil
}

// resolveFunction resolves a FunctionDecl to a ResolvedFunction.
func (r *Resolver) resolveFunction(fd *FunctionDecl, ir *SchemaIR) (*ResolvedFunction, error) {
	fn := &ResolvedFunction{Name: fd.Name, Language: "plpgsql"}

	if fd.Returns == "trigger" {
		fn.Returns = "trigger"
	} else {
		fn.Returns = resolveFunctionType(fd.Returns, fd.ReturnArray, ir)
	}

	for _, p := range fd.Params {
		fn.Params = append(fn.Params, ResolvedFuncParam{
			Name:    p.Name,
			SQLType: resolveFunctionType(p.Type, p.Array, ir),
		})
	}

	if err := applyFuncDirectives(fn, fd.Directives); err != nil {
		return nil, err
	}

	if fd.Return == nil {
		return nil, fmt.Errorf("missing body")
	}
	fn.ReturnSQL = fd.Return.Raw
	return fn, nil
}

// applyFuncDirectives folds leading @-directives into the resolved function's
// attribute fields, rejecting unknown names and conflicting volatilities.
func applyFuncDirectives(fn *ResolvedFunction, dirs []*FuncDirective) error {
	for _, d := range dirs {
		val := ""
		if d.Value != nil {
			val = stripSingleQuotes(*d.Value)
		}
		switch d.Name {
		case "language":
			if val == "" {
				return fmt.Errorf("@language requires a value (e.g. @language plpgsql)")
			}
			fn.Language = val
		case "immutable", "stable", "volatile":
			if fn.Volatility != "" && fn.Volatility != d.Name {
				return fmt.Errorf("conflicting volatility: @%s and @%s", fn.Volatility, d.Name)
			}
			fn.Volatility = d.Name
		case "strict":
			fn.Strict = true
		case "leakproof":
			fn.Leakproof = true
		case "parallel":
			if val != "safe" && val != "unsafe" && val != "restricted" {
				return fmt.Errorf("@parallel must be safe, unsafe, or restricted (got %q)", val)
			}
			fn.Parallel = val
		case "security":
			if val != "definer" && val != "invoker" {
				return fmt.Errorf("@security must be definer or invoker (got %q)", val)
			}
			fn.Security = val
		case "cost":
			if val == "" {
				return fmt.Errorf("@cost requires a value")
			}
			fn.Cost = val
		case "for":
			if val == "" {
				return fmt.Errorf("@for requires a type name (e.g. @for KV)")
			}
			fn.RunOnceFor = val
		default:
			return fmt.Errorf("unknown function directive @%s", d.Name)
		}
	}
	return nil
}

// resolveFunctionType maps a function parameter/return type to its SQL form. ASL
// builtins, scalar aliases, and enums map to their SQL equivalents; anything else
// is a raw Postgres type and passes through verbatim (functions "map directly to
// Postgres, no aliasing"). A trailing [] marks an array.
func resolveFunctionType(name string, array bool, ir *SchemaIR) string {
	sqlType := name
	if s, ok := builtinTypes[name]; ok {
		sqlType = s
	} else if scalar, ok := ir.ScalarTypes[name]; ok {
		sqlType = scalar.SQLType
	} else if _, ok := ir.EnumTypes[name]; ok {
		sqlType = "TEXT"
	}
	if array {
		sqlType += "[]"
	}
	return sqlType
}

// stripDollarQuotes removes the surrounding $$ from a DollarString token value.
func stripDollarQuotes(s string) string {
	s = strings.TrimPrefix(s, "$$")
	s = strings.TrimSuffix(s, "$$")
	return strings.TrimSpace(s)
}

func (r *Resolver) resolveField(f *FieldDecl, rt *ResolvedType, ir *SchemaIR) error {
	// Determine if this is a link or a property.
	isLink := false
	var targetTypeName string
	var linkField string // for old-style "on field"

	if f.TypeSpec == nil {
		return fmt.Errorf("field %q has no type annotation", f.Name)
	}

	if f.TypeSpec.PropType != nil {
		typeName := *f.TypeSpec.PropType

		// If the link keyword is present or the target is a known object type → link.
		if f.LinkKeyword {
			isLink = true
			targetTypeName = typeName
		} else if _, ok := ir.ObjectTypes[typeName]; ok {
			isLink = true
			targetTypeName = typeName
		}
		// Otherwise it's a property (builtin, scalar alias, or enum).
	}

	// Extract on-clause from body.
	if f.Body != nil {
		for _, item := range f.Body.Items {
			if item.OnClause != nil {
				linkField = item.OnClause.Field
			}
		}
	}

	if isLink {
		return r.resolveLink(f, rt, ir, targetTypeName, linkField)
	}
	return r.resolveProp(f, rt, ir)
}

func (r *Resolver) resolveProp(f *FieldDecl, rt *ResolvedType, ir *SchemaIR) error {
	typeName := *f.TypeSpec.PropType

	sqlType, err := r.resolveBaseType(typeName, ir)
	if err != nil {
		return fmt.Errorf("property %q: %w", f.Name, err)
	}

	enum, isEnum := ir.EnumTypes[typeName]

	prop := &ResolvedProp{
		Name:       f.Name,
		Column:     toSnakeCase(f.Name),
		SQLType:    sqlType,
		IsRequired: f.Required,
		IsMulti:    f.Multi,
	}
	if isEnum {
		prop.EnumType = typeName
	}

	// Extract default and constraints from body.
	if f.Body != nil {
		for _, item := range f.Body.Items {
			switch {
			case item.Default != nil:
				if isEnum {
					def, err := resolveEnumDefault(item.Default, typeName, enum)
					if err != nil {
						return fmt.Errorf("property %q: %w", f.Name, err)
					}
					prop.Default = def
				} else {
					if item.Default.QualEnum != nil {
						return fmt.Errorf("property %q: qualified enum default %s.%s used on non-enum type %q",
							f.Name, item.Default.QualEnum[0], item.Default.QualEnum[1], typeName)
					}
					prop.Default = resolveDefault(item.Default, sqlType)
				}
			case item.Constraint != nil:
				prop.Constraints = append(prop.Constraints, ResolvedConstraint{
					Name: item.Constraint.Name,
					Args: item.Constraint.Args,
				})
			case item.Rewrite != nil:
				rw, err := resolveRewrite(item.Rewrite, sqlType, rt.Name)
				if err != nil {
					return fmt.Errorf("property %q: %w", f.Name, err)
				}
				prop.Rewrites = append(prop.Rewrites, rw)
			}
		}
	}

	rt.Properties[f.Name] = prop
	return nil
}

// resolveRewrite resolves a field rewrite to its events and the SQL value that
// gets assigned to NEW.<column> on those events.
func resolveRewrite(rw *RewriteDecl, sqlType, origin string) (ResolvedRewrite, error) {
	events := make([]string, 0, len(rw.Events))
	for _, ev := range rw.Events {
		ev = canonEvent(ev)
		if ev != "insert" && ev != "update" {
			return ResolvedRewrite{}, fmt.Errorf("rewrite event %q not allowed (want insert|update)", ev)
		}
		events = append(events, ev)
	}
	out := ResolvedRewrite{Events: events, Origin: origin}
	switch {
	case rw.Call != nil:
		sql, err := resolveRewriteCall(rw.Call, sqlType)
		if err != nil {
			return ResolvedRewrite{}, err
		}
		out.ValueSQL = sql
	case rw.Row != nil:
		row, ok := triggerRowSQL(*rw.Row)
		if !ok {
			return ResolvedRewrite{}, fmt.Errorf("rewrite value %q.%s is not a row reference (use __new__, __old__, or __subject__)", *rw.Row, deref(rw.Field))
		}
		out.ValueSQL = fmt.Sprintf("%s.%q", row, toSnakeCase(*rw.Field))
	case rw.Lit != nil:
		out.ValueSQL = mapLitDefault(*rw.Lit, sqlType)
	default:
		return ResolvedRewrite{}, fmt.Errorf("rewrite has no value")
	}
	return out, nil
}

// resolveRewriteCall builds the SQL for a rewrite function call. A zero-arg call
// goes through mapFuncDefault so builtins keep their mapping (datetime_current()
// → now()); a call with arguments is emitted verbatim with resolved arguments
// (slugify(__new__.title) → slugify(NEW."title")).
func resolveRewriteCall(call *RewriteCall, sqlType string) (string, error) {
	if len(call.Args) == 0 {
		return mapFuncDefault(call.Func, sqlType), nil
	}
	args := make([]string, 0, len(call.Args))
	for _, a := range call.Args {
		s, err := resolveRewriteArg(a, sqlType)
		if err != nil {
			return "", err
		}
		args = append(args, s)
	}
	return fmt.Sprintf("%s(%s)", call.Func, strings.Join(args, ", ")), nil
}

// resolveRewriteArg resolves one rewrite call argument to SQL.
func resolveRewriteArg(a *RewriteArg, sqlType string) (string, error) {
	switch {
	case a.Row != nil:
		row, ok := triggerRowSQL(*a.Row)
		if !ok {
			return "", fmt.Errorf("rewrite argument %q.%s is not a row reference (use __new__, __old__, or __subject__)", *a.Row, deref(a.Field))
		}
		return fmt.Sprintf("%s.%q", row, toSnakeCase(*a.Field)), nil
	case a.Lit != nil:
		return mapLitDefault(*a.Lit, sqlType), nil
	}
	return "", fmt.Errorf("rewrite argument has no value")
}

// canonEvent maps friendly event aliases to their canonical names.
func canonEvent(ev string) string {
	if ev == "create" {
		return "insert"
	}
	return ev
}

// triggerRowSQL maps a magic row identifier to its SQL row keyword.
func triggerRowSQL(name string) (string, bool) {
	switch name {
	case "__new__", "__subject__":
		return "NEW", true
	case "__old__":
		return "OLD", true
	}
	return "", false
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// resolveEnumDefault resolves a default for an enum-typed property to a quoted
// SQL string literal, validating that the referenced member belongs to the enum.
// Accepts the qualified form (default := Enum.Member) or a quoted/bare literal.
func resolveEnumDefault(d *DefaultDecl, enumName string, enum *ResolvedEnum) (string, error) {
	var member string
	switch {
	case d.QualEnum != nil:
		if d.QualEnum[0] != enumName {
			return "", fmt.Errorf("default references enum %q but property is of enum type %q", d.QualEnum[0], enumName)
		}
		member = d.QualEnum[1]
	case d.NewLit != nil:
		member = stripSingleQuotes(*d.NewLit)
	case d.OldLit != nil:
		member = stripSingleQuotes(*d.OldLit)
	case d.NewFunc != nil, d.OldFunc != nil:
		return "", fmt.Errorf("function default is not valid for enum type %q", enumName)
	default:
		return "", nil
	}

	if !slices.Contains(enum.Values, member) {
		return "", fmt.Errorf("%q is not a value of enum %q (allowed: %s)", member, enumName, strings.Join(enum.Values, ", "))
	}
	return "'" + member + "'", nil
}

// stripSingleQuotes removes a single pair of surrounding single quotes, if present.
func stripSingleQuotes(s string) string {
	if len(s) >= 2 && strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'") {
		return s[1 : len(s)-1]
	}
	return s
}

func (r *Resolver) resolveLink(f *FieldDecl, rt *ResolvedType, ir *SchemaIR, targetType, joinField string) error {
	if _, ok := ir.ObjectTypes[targetType]; !ok {
		return fmt.Errorf("link %q references unknown type %q", f.Name, targetType)
	}

	link := &ResolvedLink{
		Name:       f.Name,
		TargetType: targetType,
		JoinField:  joinField,
		IsRequired: f.Required,
		IsMulti:    f.Multi,
	}

	if f.Multi {
		// Multi-link → junction table: source_linkname
		link.JunctionTable = fmt.Sprintf("%s_%s", toSnakeCase(rt.Name), toSnakeCase(f.Name))
	} else {
		// Single link → FK column: fieldname (matches migration SQL generator convention)
		link.JoinColumn = toSnakeCase(f.Name)
	}

	// Extract body constraints (e.g. exclusive) so they reach the FK column.
	if f.Body != nil {
		for _, item := range f.Body.Items {
			if item.Constraint != nil {
				link.Constraints = append(link.Constraints, ResolvedConstraint{
					Name: item.Constraint.Name,
					Args: item.Constraint.Args,
				})
			}
		}
	}

	rt.Links[f.Name] = link
	return nil
}

// resolveBaseType resolves a type name to its SQL equivalent.
func (r *Resolver) resolveBaseType(typeName string, ir *SchemaIR) (string, error) {
	if sqlType, ok := builtinTypes[typeName]; ok {
		return sqlType, nil
	}
	if scalar, ok := ir.ScalarTypes[typeName]; ok {
		return scalar.SQLType, nil
	}
	if enum, ok := ir.EnumTypes[typeName]; ok {
		_ = enum
		return "TEXT", nil // enums stored as TEXT with CHECK constraint
	}
	return "", fmt.Errorf("unknown type %q", typeName)
}

// resolveDefault converts a DefaultDecl to a SQL DEFAULT expression.
func resolveDefault(d *DefaultDecl, sqlType string) string {
	switch {
	case d.NewFunc != nil:
		return mapFuncDefault(*d.NewFunc, sqlType)
	case d.NewLit != nil:
		return mapLitDefault(*d.NewLit, sqlType)
	case d.OldFunc != nil:
		return mapFuncDefault(*d.OldFunc, sqlType)
	case d.OldLit != nil:
		return mapLitDefault(*d.OldLit, sqlType)
	}
	return ""
}

func mapFuncDefault(name, sqlType string) string {
	switch name {
	case "gen_uuid", "gen_random_uuid":
		return "gen_random_uuid()"
	case "now", "datetime_current":
		return "now()"
	default:
		return name + "()"
	}
}

func mapLitDefault(lit, sqlType string) string {
	// Strip surrounding single-quotes from string literals.
	if strings.HasPrefix(lit, "'") && strings.HasSuffix(lit, "'") {
		return lit // already SQL-compatible single-quoted string
	}
	// Boolean literals.
	if lit == "true" || lit == "false" {
		return lit
	}
	// Numeric literals.
	return lit
}

// toSnakeCase converts CamelCase or mixed identifiers to snake_case. It MUST
// match the DDL layer's identifier derivation (lo.SnakeCase, used by
// sql_generator.go / migration_sql_generator.go to name the physical tables and
// columns), so that every table/column/junction name the compiler emits resolves
// to the object CREATE TABLE actually made. lo.SnakeCase handles acronyms and
// digit boundaries (APIKey→api_key, Org2→org_2) where a naive split would drift
// from the physical schema. This is the single canonical snake_case chokepoint.
func toSnakeCase(s string) string {
	return lo.SnakeCase(s)
}
