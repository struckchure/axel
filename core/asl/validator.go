package asl

import "fmt"

// Validate performs semantic validation on a resolved SchemaIR.
// It checks that link targets resolve and that properties have SQL types.
// Note: the link (foreign-key reference) graph is allowed to contain cycles —
// self-referential and mutually-referential links are valid relational shapes.
// Inheritance cycles are detected earlier, at resolve time (see Resolver.Resolve),
// where the `extends` edges are still available.
func Validate(ir *SchemaIR) []error {
	var errs []error

	// Validate that all link target types exist.
	for typeName, t := range ir.ObjectTypes {
		for linkName, link := range t.Links {
			if _, ok := ir.ObjectTypes[link.TargetType]; !ok {
				errs = append(errs, fmt.Errorf(
					"type %q: link %q references unknown type %q",
					typeName, linkName, link.TargetType,
				))
			}
		}
	}

	// Validate that all property SQL types are non-empty.
	for typeName, t := range ir.ObjectTypes {
		for propName, prop := range t.Properties {
			if prop.SQLType == "" {
				errs = append(errs, fmt.Errorf(
					"type %q: property %q has no resolved SQL type",
					typeName, propName,
				))
			}
		}
	}

	return errs
}

// ValidateWarnings returns non-fatal deprecation warnings and lint hints.
func ValidateWarnings(ir *SchemaIR) []error {
	var warnings []error
	for name, s := range ir.ScalarTypes {
		if s.ExtendKeyword == "extending" {
			warnings = append(warnings, fmt.Errorf("scalar type %q: keyword 'extending' is deprecated; use 'extends' instead (run 'axel fmt' to fix)", name))
		}
	}
	for name, t := range ir.ObjectTypes {
		if t.ExtendKeyword == "extending" {
			warnings = append(warnings, fmt.Errorf("type %q: keyword 'extending' is deprecated; use 'extends' instead (run 'axel fmt' to fix)", name))
		}
	}
	return warnings
}
