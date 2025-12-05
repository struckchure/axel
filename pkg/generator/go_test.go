package generator

import (
	"testing"

	"github.com/struckchure/axel/pkg/db"
)

func TestGoGenerator_MapTypeToGo(t *testing.T) {
	gen := &GoGenerator{}

	tests := []struct {
		dbType   string
		nullable bool
		expected string
	}{
		{"integer", false, "int"},
		{"bigint", false, "int64"},
		{"varchar", false, "string"},
		{"text", false, "string"},
		{"boolean", false, "bool"},
		{"timestamp", false, "time.Time"},
		{"integer", true, "*int"},
		{"varchar", true, "*string"},
	}

	for _, tt := range tests {
		result := gen.mapTypeToGo(tt.dbType, tt.nullable)
		if result != tt.expected {
			t.Errorf("mapTypeToGo(%s, %v) = %s; expected %s", 
				tt.dbType, tt.nullable, result, tt.expected)
		}
	}
}

func TestGoGenerator_ToPascalCase(t *testing.T) {
	gen := &GoGenerator{}

	tests := []struct {
		input    string
		expected string
	}{
		{"user_name", "UserName"},
		{"first_name", "FirstName"},
		{"id", "Id"},
		{"created_at", "CreatedAt"},
	}

	for _, tt := range tests {
		result := gen.toPascalCase(tt.input)
		if result != tt.expected {
			t.Errorf("toPascalCase(%s) = %s; expected %s", 
				tt.input, result, tt.expected)
		}
	}
}

func TestGoGenerator_GenerateStruct(t *testing.T) {
	gen := &GoGenerator{
		pkg: "models",
	}

	table := db.Table{
		Name: "users",
		Columns: []db.Column{
			{Name: "id", Type: "integer", Nullable: false, IsPrimaryKey: true},
			{Name: "username", Type: "varchar", Nullable: false},
			{Name: "email", Type: "varchar", Nullable: true},
		},
	}

	result := gen.generateStruct(table)

	// Check that the struct contains expected elements
	expectedStrings := []string{
		"package models",
		"type Users struct",
		"Id int",
		"Username string",
		"Email *string",
	}

	for _, expected := range expectedStrings {
		if !contains(result, expected) {
			t.Errorf("Generated struct missing expected string: %s", expected)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && 
		(findSubstring(s, substr) != -1))
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
