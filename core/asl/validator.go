package asl

import "fmt"

// Validate performs semantic validation on a resolved SchemaIR.
// It checks for inheritance cycles and unresolved type references.
func Validate(ir *SchemaIR) []error {
	var errs []error

	// Check for inheritance cycles via DFS.
	visited := make(map[string]bool)
	visiting := make(map[string]bool)

	var checkCycles func(name string) error
	checkCycles = func(name string) error {
		if visited[name] {
			return nil
		}
		if visiting[name] {
			return fmt.Errorf("inheritance cycle detected involving type %q", name)
		}
		visiting[name] = true

		t, ok := ir.ObjectTypes[name]
		if !ok {
			return nil
		}

		for _, link := range t.Links {
			if err := checkCycles(link.TargetType); err != nil {
				return err
			}
		}

		visiting[name] = false
		visited[name] = true
		return nil
	}

	for name := range ir.ObjectTypes {
		if err := checkCycles(name); err != nil {
			errs = append(errs, err)
		}
	}

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
