package codegen

import (
	"fmt"
	"sort"
	"strings"
)

// validateDirectives enforces that @request / @response type names are consistent
// across all queries (and against schema type/enum names). Because a directive
// name like `@response User` produces a single shared type in one generated
// package, two sources claiming the same name must describe identical fields.
//
// If the fields match, the type is deduplicated (emitted once and reused). If
// they differ, generation aborts here — before any generator runs — with an
// error naming both sources and the differing fields.
func validateDirectives(schema SchemaDescriptor, queries []QueryDescriptor) error {
	type entry struct{ sig, source string }
	reg := make(map[string]entry)

	// Seed with names that already occupy the generated type namespace.
	for _, td := range schema.Types {
		if td.IsAbstract {
			continue
		}
		reg[td.Name] = entry{typeSig(td), fmt.Sprintf("schema type %q", td.Name)}
	}
	for _, e := range schema.Enums {
		reg[e.Name] = entry{"enum:" + e.Name, fmt.Sprintf("enum %q", e.Name)}
	}

	claim := func(name, sig, source string) error {
		if e, ok := reg[name]; ok {
			if e.sig != sig {
				return fmt.Errorf(
					"generated type name %q is claimed by %s and %s with mismatched fields (%s)",
					name, e.source, source, describeSigDiff(e.sig, sig),
				)
			}
			return nil // identical → reuse
		}
		reg[name] = entry{sig, source}
		return nil
	}

	for _, q := range queries {
		src := fmt.Sprintf("query %q", q.File)
		// @response — only meaningful when the query returns a row struct.
		if name, ok := q.Directive("response"); ok && name != "" &&
			!q.Result.IsScalar && q.Operation != "delete" {
			if err := claim(name, resultSig(q.Result), src); err != nil {
				return err
			}
		}
		// @request — only meaningful when the query has params.
		if name, ok := q.Directive("request"); ok && name != "" && len(q.Params) > 0 {
			if err := claim(name, "params:"+paramsSig(q.Params), src); err != nil {
				return err
			}
		}
	}
	return nil
}

// resultSig is a canonical, order-insensitive signature of a query's row shape.
func resultSig(r ResultDescriptor) string {
	return fieldsSig(r.Fields)
}

func fieldsSig(fields []ResultField) string {
	toks := make([]string, 0, len(fields))
	for _, f := range fields {
		t := f.AQLType
		if f.EnumType != "" {
			t = f.EnumType
		}
		if len(f.SubFields) > 0 {
			t = "{" + fieldsSig(f.SubFields) + "}"
		}
		toks = append(toks, fmt.Sprintf("%s:%s:%t:%t", f.Name, t, f.IsNullable, f.IsMultiple))
	}
	sort.Strings(toks)
	return strings.Join(toks, ",")
}

// typeSig mirrors the fields of a schema type's row: scalar properties plus
// single-link FK columns (keyed by the FK column, matching allPropsAsFields).
// Multi-links are omitted (they are not scalar FK columns).
func typeSig(td TypeDescriptor) string {
	toks := make([]string, 0, len(td.Properties)+len(td.Links))
	for _, p := range td.Properties {
		t := p.AQLType
		if p.EnumType != "" {
			t = p.EnumType
		}
		toks = append(toks, fmt.Sprintf("%s:%s:%t:%t", p.Name, t, !p.IsRequired, p.IsMulti))
	}
	for _, l := range td.Links {
		if l.IsMulti {
			continue
		}
		toks = append(toks, fmt.Sprintf("%s:uuid:%t:false", l.JoinColumn, !l.IsRequired))
	}
	sort.Strings(toks)
	return strings.Join(toks, ",")
}

// paramsSig is a canonical, order-insensitive signature of a params struct.
func paramsSig(params []ParamDescriptor) string {
	toks := make([]string, 0, len(params))
	for _, p := range params {
		t := p.AQLType
		if p.EnumType != "" {
			t = p.EnumType
		}
		toks = append(toks, fmt.Sprintf("%s:%s:%t", p.Name, t, p.IsOptional))
	}
	sort.Strings(toks)
	return strings.Join(toks, ",")
}

// describeSigDiff returns a short human-readable summary of the field names that
// differ between two signatures, to make conflict errors actionable.
func describeSigDiff(a, b string) string {
	fieldNames := func(sig string) map[string]bool {
		names := map[string]bool{}
		for _, tok := range strings.Split(sig, ",") {
			if tok == "" {
				continue
			}
			name, _, _ := strings.Cut(tok, ":")
			names[name] = true
		}
		return names
	}
	an, bn := fieldNames(a), fieldNames(b)
	var onlyA, onlyB []string
	for n := range an {
		if !bn[n] {
			onlyA = append(onlyA, n)
		}
	}
	for n := range bn {
		if !an[n] {
			onlyB = append(onlyB, n)
		}
	}
	sort.Strings(onlyA)
	sort.Strings(onlyB)
	switch {
	case len(onlyA) == 0 && len(onlyB) == 0:
		return "same field names but differing types/nullability"
	case len(onlyB) == 0:
		return fmt.Sprintf("fields only in the first: %s", strings.Join(onlyA, ", "))
	case len(onlyA) == 0:
		return fmt.Sprintf("fields only in the second: %s", strings.Join(onlyB, ", "))
	default:
		return fmt.Sprintf("first-only: %s; second-only: %s", strings.Join(onlyA, ", "), strings.Join(onlyB, ", "))
	}
}
