package asl

import (
	"fmt"
	"slices"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
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
	"json":     "JSON",
	"jsonb":    "JSONB",
	"bytes":    "BYTEA",
	"decimal":  "NUMERIC",
}

// allowedJsonScalarFieldTypes restricts field types inside typed JSON scalars
// to strings, numbers and temporal types. Temporal fields live in the document
// as JSON strings and are cast on extraction (e.g. (col->>'opening')::TIME).
var allowedJsonScalarFieldTypes = map[string]bool{
	"str":      true,
	"int16":    true,
	"int32":    true,
	"int64":    true,
	"float32":  true,
	"float64":  true,
	"decimal":  true,
	"date":     true,
	"time":     true,
	"datetime": true,
}

// BuiltinSQLType returns the SQL type for a builtin scalar name (e.g. "str" →
// "TEXT") and whether name is a builtin scalar at all.
func BuiltinSQLType(name string) (string, bool) {
	s, ok := builtinTypes[name]
	return s, ok
}

// Resolver builds a SchemaIR from a parsed SourceFile.
type Resolver struct{}

// declSite records where a name was declared, so a redeclaration can point at
// both locations.
type declSite struct {
	kind string
	pos  lexer.Position
}

// posStr renders a declaration site as file:line:col, degrading to line:col for
// sources parsed without a filename (Parse rather than ParseNamed).
func posStr(p lexer.Position) string {
	if p.Filename == "" {
		return fmt.Sprintf("%d:%d", p.Line, p.Column)
	}
	return fmt.Sprintf("%s:%d:%d", p.Filename, p.Line, p.Column)
}

func redeclared(prev declSite, kind, name string, pos lexer.Position) error {
	if prev.kind == kind {
		return fmt.Errorf("%s %q declared more than once (%s and %s)", kind, name, posStr(prev.pos), posStr(pos))
	}
	return fmt.Errorf("%s %q conflicts with %s %q declared at %s (%s)", kind, name, prev.kind, name, posStr(prev.pos), posStr(pos))
}

// Resolve resolves a parsed SourceFile into a SchemaIR.
//
// The SourceFile may be a single parsed file or several merged by Merge (see
// Load). Declarations are collected before anything is resolved, so order —
// within a file, and between files — never affects the result.
func (r *Resolver) Resolve(src *SourceFile) (*SchemaIR, error) {
	ir := &SchemaIR{
		ScalarTypes: make(map[string]*ResolvedScalar),
		EnumTypes:   make(map[string]*ResolvedEnum),
		ObjectTypes: make(map[string]*ResolvedType),
		Functions:   make(map[string]*ResolvedFunction),
	}

	// Pass 0: collect every declaration, rejecting redeclarations. Scalars,
	// enums and object types share one type namespace, so a clash between kinds
	// is reported too.
	var (
		typeNames  = map[string]declSite{}
		scalarDefs = map[string]*ScalarTypeDef{}
		enumDefs   = map[string]*EnumTypeDef{}
		objDefs    = map[string]*TypeDef{}
		globalSeen = map[string]declSite{}
		funcSeen   = map[string]declSite{}
		seenExt    = map[string]bool{}

		scalarOrder []string
		enumOrder   []string
		objOrder    []string
		globalDecls []*GlobalDecl
		funcDecls   []*FunctionDecl
	)

	for _, def := range src.Definitions {
		switch {
		case def.Extension != nil:
			// Extensions are deduped rather than rejected: repeating
			// `use extension 'uuid-ossp';` in each file of a split schema is
			// expected, and the emitted SQL is idempotent either way.
			name := stripSingleQuotes(def.Extension.Name)
			if name == "" {
				return nil, fmt.Errorf("extension name may not be empty")
			}
			if !seenExt[name] {
				seenExt[name] = true
				ir.Extensions = append(ir.Extensions, name)
			}

		case def.Global != nil:
			g := def.Global
			if prev, ok := globalSeen[g.Name]; ok {
				return nil, redeclared(prev, "global", g.Name, g.Pos)
			}
			globalSeen[g.Name] = declSite{"global", g.Pos}
			globalDecls = append(globalDecls, g)

		case def.Function != nil:
			f := def.Function
			funcKey := f.Name
			if f.Receiver != nil {
				funcKey = fmt.Sprintf("(%s).%s", f.Receiver.Type, f.Name)
			}
			if prev, ok := funcSeen[funcKey]; ok {
				return nil, redeclared(prev, "function", funcKey, f.Pos)
			}
			funcSeen[funcKey] = declSite{"function", f.Pos}
			funcDecls = append(funcDecls, f)

		case def.ScalarType != nil:
			s := def.ScalarType
			if prev, ok := typeNames[s.Name]; ok {
				return nil, redeclared(prev, "scalar type", s.Name, s.Pos)
			}
			typeNames[s.Name] = declSite{"scalar type", s.Pos}
			scalarDefs[s.Name] = s
			scalarOrder = append(scalarOrder, s.Name)

		case def.EnumType != nil:
			e := def.EnumType
			if prev, ok := typeNames[e.Name]; ok {
				return nil, redeclared(prev, "enum", e.Name, e.Pos)
			}
			typeNames[e.Name] = declSite{"enum", e.Pos}
			enumDefs[e.Name] = e
			enumOrder = append(enumOrder, e.Name)

		case def.TypeDef != nil:
			t := def.TypeDef
			if prev, ok := typeNames[t.Name]; ok {
				return nil, redeclared(prev, "type", t.Name, t.Pos)
			}
			typeNames[t.Name] = declSite{"type", t.Pos}
			objDefs[t.Name] = t
			objOrder = append(objOrder, t.Name)
		}
	}

	// Pass 1: enums. They have no dependencies, so they come first — a scalar or
	// a global may name one.
	for _, name := range enumOrder {
		e := enumDefs[name]
		ir.EnumTypes[name] = &ResolvedEnum{Name: name, Values: e.Values}
	}

	// Pass 2: scalars, resolved depth-first so `scalar type A extending B` works
	// with B declared later (or in another file).
	{
		resolving := map[string]bool{}
		var resolveScalar func(name string) error
		resolveScalar = func(name string) error {
			if _, done := ir.ScalarTypes[name]; done {
				return nil
			}
			s := scalarDefs[name]
			if resolving[name] {
				return fmt.Errorf("scalar type %q: cycle detected in extending chain", name)
			}
			resolving[name] = true
			defer delete(resolving, name)

			var (
				sqlType     string
				base        string
				isMulti     bool
				isCustomSQL bool
				constraints []ResolvedConstraint
				defaultVal  string
				rewrites    []ResolvedRewrite
			)

			if s.ExtendsSQL != nil {
				rawSQL := strings.TrimSpace(*s.ExtendsSQL)
				rawSQL = strings.Trim(rawSQL, "\"'")
				if rawSQL == "" {
					return fmt.Errorf("scalar type %q: sql type cannot be empty", s.Name)
				}
				sqlType = rawSQL
				base = "str"
				if s.AsBase != "" {
					if _, isBuiltin := builtinTypes[s.AsBase]; !isBuiltin {
						return fmt.Errorf("scalar type %q: 'as %s' must name a valid scalar type", s.Name, s.AsBase)
					}
					base = s.AsBase
				}
				isMulti = s.AsMulti
				isCustomSQL = true
			} else {
				// A scalar extending another user scalar needs that one resolved first.
				if _, isBuiltin := builtinTypes[s.Extends]; !isBuiltin {
					if _, isDeclared := scalarDefs[s.Extends]; isDeclared {
						if err := resolveScalar(s.Extends); err != nil {
							return err
						}
					}
				}
				st, err := r.resolveBaseType(s.Extends, ir)
				if err != nil {
					return fmt.Errorf("scalar type %q: %w", s.Name, err)
				}
				sqlType = st
				base = s.Extends
				if parentScalar, isScalar := ir.ScalarTypes[s.Extends]; isScalar {
					if len(parentScalar.Constraints) > 0 {
						constraints = append(constraints, parentScalar.Constraints...)
					}
					defaultVal = parentScalar.Default
					if len(parentScalar.Rewrites) > 0 {
						rewrites = append(rewrites, parentScalar.Rewrites...)
					}
				}
			}

			var fields map[string]*ResolvedScalarField
			body := s.Body
			if s.AsBody != nil {
				body = s.AsBody
			}
			if body != nil {
				if len(body.Items) > 0 {
					for _, item := range body.Items {
						switch {
						case item.Default != nil:
							if item.Default.QualEnum != nil {
								return fmt.Errorf("scalar type %q: qualified enum default %s.%s used on non-enum type %q",
									s.Name, item.Default.QualEnum[0], item.Default.QualEnum[1], s.Extends)
							}
							def, err := resolveDefault(item.Default, sqlType, ir)
							if err != nil {
								return fmt.Errorf("scalar type %q: %w", s.Name, err)
							}
							defaultVal = def
						case item.Constraint != nil:
							constraints = append(constraints, ResolvedConstraint{
								Name: item.Constraint.Name,
								Args: item.Constraint.Args,
							})
						case item.Rewrite != nil:
							rw, err := resolveRewrite(item.Rewrite, sqlType, s.Name)
							if err != nil {
								return fmt.Errorf("scalar type %q: %w", s.Name, err)
							}
							rewrites = append(rewrites, rw)
						}
					}
				}

				if len(body.Fields) > 0 {
					if !isCustomSQL && sqlType != "JSON" && sqlType != "JSONB" {
						return fmt.Errorf("scalar type %q extending %q cannot define fields: fields are only supported on json/jsonb scalars or custom sql scalars", s.Name, s.Extends)
					}
					fields = make(map[string]*ResolvedScalarField)
					for _, f := range body.Fields {
						if _, exists := fields[f.Name]; exists {
							return fmt.Errorf("scalar type %q: field %q declared more than once", s.Name, f.Name)
						}
						if !allowedJsonScalarFieldTypes[f.Type] {
							return fmt.Errorf("scalar type %q field %q: type %q is not allowed; typed JSON/custom scalar fields currently only support strings (str), numbers (int16, int32, int64, float32, float64, decimal) and temporal types (date, time, datetime)", s.Name, f.Name, f.Type)
						}
						fSQLType, ok := builtinTypes[f.Type]
						if !ok {
							return fmt.Errorf("scalar type %q field %q: unknown type %q", s.Name, f.Name, f.Type)
						}
						exprSQL := ""
						if f.Computed != nil {
							exprSQL = strings.TrimSpace(f.Computed.Raw)
						}
						fields[f.Name] = &ResolvedScalarField{
							Name:       f.Name,
							AQLType:    f.Type,
							SQLType:    fSQLType,
							IsRequired: f.Required,
							IsMulti:    f.Multi,
							ExprSQL:    exprSQL,
						}
					}
					if isCustomSQL {
						base = "jsonb"
					}
				}
			}
			ir.ScalarTypes[s.Name] = &ResolvedScalar{
				Name:          s.Name,
				Base:          base,
				SQLType:       sqlType,
				IsMulti:       isMulti,
				IsCustomSQL:   isCustomSQL,
				Fields:        fields,
				ExtendKeyword: s.ExtendKeyword,
				Default:       defaultVal,
				Constraints:   constraints,
				Rewrites:      rewrites,
			}
			return nil
		}
		for _, name := range scalarOrder {
			if err := resolveScalar(name); err != nil {
				return nil, err
			}
		}
	}

	// Pass 3: globals (scalar-typed, so every named type is registered by now).
	for _, g := range globalDecls {
		sqlType, err := r.resolveBaseType(g.Type, ir)
		if err != nil {
			return nil, fmt.Errorf("global %q: %w (globals must be a scalar type)", g.Name, err)
		}
		ir.Globals = append(ir.Globals, &ResolvedGlobal{
			Name:     g.Name,
			AQLType:  g.Type,
			SQLType:  sqlType,
			Required: g.Required,
		})
	}

	// Pass 4: functions (parameter and return types may name scalars or enums).
	for _, fd := range funcDecls {
		if fd.Receiver != nil {
			recType := fd.Receiver.Type
			if scalar, ok := ir.ScalarTypes[recType]; ok {
				if fd.Name == "deserialize" && fd.Return != nil {
					scalar.DeserializeSQL = fd.Return.Raw
					scalar.ReceiverName = fd.Receiver.Name
					structFields := parseStructReturn(fd.Return.Raw)
					if scalar.Fields == nil && len(structFields) > 0 {
						scalar.Fields = make(map[string]*ResolvedScalarField)
					}
					for fName, fExpr := range structFields {
						exprSelf := replaceParamWithSelf(fExpr, fd.Receiver.Name)
						if sf, ok := scalar.Fields[fName]; ok {
							sf.ExprSQL = exprSelf
						}
					}
				} else if fd.Name == "serialize" && fd.Return != nil {
					scalar.SerializeSQL = fd.Return.Raw
					scalar.ReceiverName = fd.Receiver.Name
				}
			}
		}
		fn, err := r.resolveFunction(fd, ir)
		if err != nil {
			return nil, fmt.Errorf("function %q: %w", fd.Name, err)
		}
		ir.Functions[fn.Name] = fn
	}

	// Validate scalar defaults against resolved functions
	for _, name := range scalarOrder {
		s := scalarDefs[name]
		if s.Body != nil {
			for _, item := range s.Body.Items {
				if item.Default != nil && item.Default.NewCall != nil {
					sqlType := ir.ScalarTypes[name].SQLType
					if _, err := resolveDefault(item.Default, sqlType, ir); err != nil {
						return nil, fmt.Errorf("scalar type %q: %w", name, err)
					}
				}
			}
		}
	}

	// Pass 5: register object types (abstract and concrete) without members.
	for _, name := range objOrder {
		t := objDefs[name]
		rt := &ResolvedType{
			Name:          t.Name,
			IsAbstract:    t.Abstract,
			Table:         toSnakeCase(t.Name),
			ExtendKeyword: t.ExtendKeyword,
			Properties:    make(map[string]*ResolvedProp),
			Links:         make(map[string]*ResolvedLink),
			Computed:      make(map[string]*ResolvedComputed),
		}
		if t.Abstract {
			rt.Table = "" // abstract types have no table
		}
		ir.ObjectTypes[t.Name] = rt
	}

	// Pass 6: detect inheritance (extends) cycles before flattening. Flattening
	// copies each parent's already-resolved members into the child, so a cycle
	// would otherwise produce silently incomplete types. The link/foreign-key
	// graph is intentionally not checked here — self- and mutual references are
	// valid (see Validate).
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
			for _, parent := range objDefs[name].Extending {
				if _, known := objDefs[parent]; !known {
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
		for _, name := range objOrder {
			if err := checkExtends(name); err != nil {
				return nil, err
			}
		}
	}

	// Pass 7: resolve members for each type. Parents are flattened into children
	// by copying already-resolved members, so each type's parents must be
	// resolved first — done depth-first rather than in declaration order, which
	// is what lets a child live in a file that sorts before its parent's.
	{
		done := map[string]bool{}
		var resolveType func(name string) error
		resolveType = func(name string) error {
			if done[name] {
				return nil
			}
			done[name] = true // pass 6 already rejected cycles

			t := objDefs[name]
			rt := ir.ObjectTypes[name]

			// Inherit from parent types first.
			for _, parentName := range t.Extending {
				parent, ok := ir.ObjectTypes[parentName]
				if !ok {
					return fmt.Errorf("type %q extends unknown type %q", t.Name, parentName)
				}
				if err := resolveType(parentName); err != nil {
					return err
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
					return fmt.Errorf("type %q: %w", t.Name, err)
				}
			}

			// Policies resolve after all members so `.field` refs see every column.
			for _, m := range t.Members {
				if m.Policy == nil {
					continue
				}
				pol, err := r.resolvePolicy(m.Policy, rt)
				if err != nil {
					return fmt.Errorf("type %q: %w", t.Name, err)
				}
				rt.Policies = append(rt.Policies, pol)
			}

			// Validate index and constraint field references.
			for _, m := range t.Members {
				if m.Index != nil {
					for _, f := range m.Index.Fields {
						if _, ok := rt.Properties[f]; ok {
							continue
						}
						if link, ok := rt.Links[f]; ok {
							if link.IsMulti {
								return fmt.Errorf("type %q: index cannot reference multi-link %q (stored in junction table)", t.Name, f)
							}
							continue
						}
						if _, ok := rt.Computed[f]; ok {
							continue
						}
						return fmt.Errorf("type %q: index on unknown field %q", t.Name, f)
					}
				}
				if m.Constraint != nil {
					for _, f := range m.Constraint.Fields {
						if _, ok := rt.Properties[f]; ok {
							continue
						}
						if link, ok := rt.Links[f]; ok {
							if link.IsMulti {
								return fmt.Errorf("type %q: constraint %q cannot reference multi-link %q (stored in junction table)", t.Name, m.Constraint.Expression, f)
							}
							continue
						}
						if _, ok := rt.Computed[f]; ok {
							continue
						}
						return fmt.Errorf("type %q: constraint %q on unknown field %q", t.Name, m.Constraint.Expression, f)
					}
				}
			}
			return nil
		}
		for _, name := range objOrder {
			if err := resolveType(name); err != nil {
				return nil, err
			}
		}
	}

	// Pass 8: validate that every trigger's execute target is a declared function
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
	fnName := fd.Name
	if fd.Receiver != nil {
		fnName = fmt.Sprintf("%s_%s", toSnakeCase(fd.Receiver.Type), toSnakeCase(fd.Name))
	}
	fn := &ResolvedFunction{Name: fnName, Language: "plpgsql"}

	if fd.Receiver != nil {
		fn.ReceiverType = fd.Receiver.Type
		fn.ReceiverName = fd.Receiver.Name
		fn.Params = append(fn.Params, ResolvedFuncParam{
			Name:    fd.Receiver.Name,
			SQLType: resolveFunctionType(fd.Receiver.Type, fd.Receiver.Array, ir),
		})
	}

	if fd.Returns == "trigger" {
		fn.Returns = "trigger"
	} else if fd.Returns != "" {
		fn.Returns = resolveFunctionType(fd.Returns, fd.ReturnArray, ir)
	} else if fd.Receiver != nil {
		fn.Returns = resolveFunctionType(fd.Receiver.Type, false, ir)
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
	fn.ReturnSQL = fd.Return.Lowered
	fn.InlineAQL = fd.Return.AQL
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
		AQLType:    typeName,
		IsRequired: f.Required,
		IsMulti:    f.Multi,
	}
	if isEnum {
		prop.EnumType = typeName
	} else if scalar, isScalar := ir.ScalarTypes[typeName]; isScalar {
		if scalar.Default != "" {
			prop.Default = scalar.Default
		}
		if len(scalar.Constraints) > 0 {
			prop.Constraints = append(prop.Constraints, scalar.Constraints...)
		}
		for _, rw := range scalar.Rewrites {
			prop.Rewrites = append(prop.Rewrites, ResolvedRewrite{
				Events:   append([]string(nil), rw.Events...),
				ValueSQL: rw.ValueSQL,
				Origin:   rt.Name,
			})
		}
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
					def, err := resolveDefault(item.Default, sqlType, ir)
					if err != nil {
						return fmt.Errorf("property %q: %w", f.Name, err)
					}
					prop.Default = def
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
	case d.NewCall != nil, d.OldFunc != nil:
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
		link.JunctionSourceColumn, link.JunctionTargetColumn = JunctionColumns(rt.Name, targetType, f.Name)
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
func resolveDefault(d *DefaultDecl, sqlType string, ir *SchemaIR) (string, error) {
	switch {
	case d.NewCall != nil:
		if ir != nil {
			if declFn, isLocal := ir.Functions[d.NewCall.Func]; isLocal {
				if len(d.NewCall.Args) != len(declFn.Params) {
					return "", fmt.Errorf("function %q expects %d argument(s), got %d", d.NewCall.Func, len(declFn.Params), len(d.NewCall.Args))
				}
				for i, a := range d.NewCall.Args {
					if a.Lit != nil {
						argType := literalType(*a.Lit)
						expected := declFn.Params[i].SQLType
						if !isTypeCompatible(argType, expected) {
							return "", fmt.Errorf("function %q argument %d expects %s, got %s (%s)",
								d.NewCall.Func, i+1, sqlToAQL(expected), sqlToAQL(argType), *a.Lit)
						}
					}
				}
			}
		}
		if len(d.NewCall.Args) == 0 {
			return mapFuncDefault(d.NewCall.Func, sqlType), nil
		}
		args := make([]string, len(d.NewCall.Args))
		for i, a := range d.NewCall.Args {
			args[i] = mapLitDefault(*a.Lit, sqlType)
		}
		return fmt.Sprintf("%s(%s)", d.NewCall.Func, strings.Join(args, ", ")), nil
	case d.NewLit != nil:
		return mapLitDefault(*d.NewLit, sqlType), nil
	case d.OldFunc != nil:
		return mapFuncDefault(*d.OldFunc, sqlType), nil
	case d.OldLit != nil:
		return mapLitDefault(*d.OldLit, sqlType), nil
	}
	return "", nil
}

func literalType(lit string) string {
	lit = strings.TrimSpace(lit)
	if strings.HasPrefix(lit, "'") && strings.HasSuffix(lit, "'") {
		return "TEXT"
	}
	if lit == "true" || lit == "false" {
		return "BOOLEAN"
	}
	if strings.Contains(lit, ".") {
		return "NUMERIC"
	}
	isDigit := true
	for i, r := range lit {
		if i == 0 && (r == '-' || r == '+') {
			continue
		}
		if r < '0' || r > '9' {
			isDigit = false
			break
		}
	}
	if isDigit && len(lit) > 0 {
		return "INTEGER"
	}
	return "UNKNOWN"
}

func isTypeCompatible(argType, expectedSQL string) bool {
	expectedSQL = strings.ToUpper(strings.TrimSpace(expectedSQL))
	if argType == "UNKNOWN" || expectedSQL == "" {
		return true
	}
	switch argType {
	case "TEXT":
		return expectedSQL == "TEXT" || expectedSQL == "VARCHAR" || strings.HasPrefix(expectedSQL, "VARCHAR") || expectedSQL == "CHAR"
	case "INTEGER":
		return expectedSQL == "INTEGER" || expectedSQL == "SMALLINT" || expectedSQL == "BIGINT" || expectedSQL == "NUMERIC" || expectedSQL == "REAL" || expectedSQL == "DOUBLE PRECISION" || expectedSQL == "DECIMAL"
	case "NUMERIC":
		return expectedSQL == "NUMERIC" || expectedSQL == "REAL" || expectedSQL == "DOUBLE PRECISION" || expectedSQL == "DECIMAL" || expectedSQL == "FLOAT"
	case "BOOLEAN":
		return expectedSQL == "BOOLEAN" || expectedSQL == "BOOL"
	default:
		return strings.EqualFold(argType, expectedSQL)
	}
}

func sqlToAQL(sqlType string) string {
	if strings.HasSuffix(sqlType, "[]") {
		return sqlToAQL(strings.TrimSuffix(sqlType, "[]")) + "[]"
	}
	switch sqlType {
	case "TEXT":
		return "str"
	case "SMALLINT":
		return "int16"
	case "INTEGER":
		return "int32"
	case "BIGINT":
		return "int64"
	case "REAL":
		return "float32"
	case "DOUBLE PRECISION":
		return "float64"
	case "BOOLEAN":
		return "bool"
	case "UUID":
		return "uuid"
	case "TIMESTAMPTZ":
		return "datetime"
	case "DATE":
		return "date"
	case "TIME":
		return "time"
	case "JSON":
		return "json"
	case "JSONB":
		return "jsonb"
	case "BYTEA":
		return "bytes"
	case "NUMERIC":
		return "decimal"
	default:
		return strings.ToLower(sqlType)
	}
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

// parseStructReturn extracts field mappings from `Type{field1: expr1, field2: expr2}`.
func parseStructReturn(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	open := strings.Index(raw, "{")
	close := strings.LastIndex(raw, "}")
	if open == -1 || close == -1 || close <= open {
		return nil
	}
	body := strings.TrimSpace(raw[open+1 : close])
	fields := make(map[string]string)
	depth := 0
	start := 0
	for i := 0; i < len(body); i++ {
		ch := body[i]
		switch ch {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				part := strings.TrimSpace(body[start:i])
				if k, v, ok := parseFieldKeyValue(part); ok {
					fields[k] = v
				}
				start = i + 1
			}
		}
	}
	if start < len(body) {
		part := strings.TrimSpace(body[start:])
		if k, v, ok := parseFieldKeyValue(part); ok {
			fields[k] = v
		}
	}
	return fields
}

func parseFieldKeyValue(part string) (string, string, bool) {
	colon := strings.Index(part, ":")
	if colon == -1 {
		return "", "", false
	}
	key := strings.TrimSpace(part[:colon])
	val := strings.TrimSpace(part[colon+1:])
	return key, val, key != "" && val != ""
}

func replaceParamWithSelf(expr, param string) string {
	if param == "" || param == "__self__" {
		return expr
	}
	var sb strings.Builder
	pLen := len(param)
	for i := 0; i < len(expr); {
		if i+pLen <= len(expr) && expr[i:i+pLen] == param {
			leftOk := (i == 0) || !isIdentChar(rune(expr[i-1]))
			rightOk := (i+pLen == len(expr)) || !isIdentChar(rune(expr[i+pLen]))
			if leftOk && rightOk {
				sb.WriteString("__self__")
				i += pLen
				continue
			}
		}
		sb.WriteByte(expr[i])
		i++
	}
	return sb.String()
}

func isIdentChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}
