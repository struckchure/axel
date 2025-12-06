package axel

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func ParseModel(node *tree_sitter.Node, code []byte) Model {
	model := Model{
		IsAbstract: node.Kind() == "abstract_model",
	}

	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(uint(i))

		switch child.Kind() {
		case "identifier":
			if model.Name == "" {
				model.Name = getNodeText(child, code)
			}
		case "extends_clause":
			// Get first identifier in extends clause
			for j := 0; j < int(child.NamedChildCount()); j++ {
				extChild := child.NamedChild(uint(j))
				if extChild.Kind() == "identifier" {
					model.Extends = getNodeText(extChild, code)
					break
				}
			}
		case "model_body":
			model.Fields = parseModelBody(child, code)
		}
	}

	return model
}

func parseModelBody(node *tree_sitter.Node, code []byte) []Field {
	var fields []Field

	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(uint(i))

		if child.Kind() == "field_declaration" {
			field := parseField(child, code)
			fields = append(fields, field)
		}
	}

	return fields
}

func parseField(node *tree_sitter.Node, code []byte) Field {
	field := Field{}

	// Check for cardinality
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "cardinality" {
			cardText := getNodeText(child, code)
			field.IsRequired = strings.Contains(cardText, "required")
			field.IsMulti = strings.Contains(cardText, "multi")
		}
	}

	// Get field name and type
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(uint(i))

		switch child.Kind() {
		case "identifier":
			if field.Name == "" {
				field.Name = getNodeText(child, code)
			}
		case "type_expr":
			field.Type = getNodeText(child, code)
		case "field_body":
			parseFieldBody(child, code, &field)
		}
	}

	return field
}

func parseFieldBody(node *tree_sitter.Node, code []byte, field *Field) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(uint(i))

		switch child.Kind() {
		case "constraint":
			constraint := parseConstraint(child, code)
			field.Constraints = append(field.Constraints, constraint)
		case "default_clause":
			// Extract default value
			for j := 0; j < int(child.NamedChildCount()); j++ {
				expr := child.NamedChild(uint(j))
				if expr.Kind() == "expression" || expr.Kind() == "function_call" {
					field.Default = getNodeText(expr, code)
				}
			}
		case "on_clause":
			// Extract on target
			for j := 0; j < int(child.NamedChildCount()); j++ {
				ident := child.NamedChild(uint(j))
				if ident.Kind() == "identifier" {
					field.OnTarget = OnTarget{
						Name: getNodeText(ident, code),
						Type: "", // Will be resolved later when we have all models
					}
				}
			}
		}
	}
}

func parseConstraint(node *tree_sitter.Node, code []byte) Constraint {
	constraint := Constraint{}

	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(uint(i))

		switch child.Kind() {
		case "identifier":
			constraint.Name = getNodeText(child, code)
		case "argument_list":
			// Parse arguments
			for j := 0; j < int(child.NamedChildCount()); j++ {
				arg := child.NamedChild(uint(j))
				argText := getNodeText(arg, code)
				constraint.Args = append(constraint.Args, argText)
			}
		}
	}

	return constraint
}

func ResolveOnTargetTypes(models []Model) {
	// Build a map of all models (including abstract ones)
	modelMap := make(map[string]Model)
	for _, model := range models {
		modelMap[model.Name] = model
	}

	// Resolve OnTarget types
	for i := range models {
		for j := range models[i].Fields {
			field := &models[i].Fields[j]

			if field.OnTarget.Name != "" {
				// Look up the referenced model
				if refModel, ok := modelMap[field.Type]; ok {
					// Find the field in the referenced model
					targetField := findFieldInModel(refModel, field.OnTarget.Name, modelMap)
					if targetField != nil {
						field.OnTarget.Type = targetField.Type
					}
				}
			}
		}
	}
}

func findFieldInModel(model Model, fieldName string, modelMap map[string]Model) *Field {
	// Check in current model's fields
	for i := range model.Fields {
		if model.Fields[i].Name == fieldName {
			return &model.Fields[i]
		}
	}

	// Check in parent model if it extends one
	if model.Extends != "" {
		if parent, ok := modelMap[model.Extends]; ok {
			return findFieldInModel(parent, fieldName, modelMap)
		}
	}

	return nil
}
