package repl

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/struckchure/axel/core/asl"
	"github.com/struckchure/axel/core/runner"
)

func TestIsCompleteStatement(t *testing.T) {
	tests := []struct {
		input    string
		complete bool
	}{
		{"select User { id }", true},
		{"select User {\n  id,\n  name\n}", true},
		{"select User {", false},
		{"select User {\n  id,", false},
		{"select User { id } filter", false},
		{"select User { id } filter (", false},
		{"select User { id } filter (.age > 10)", true},
		{`.help`, true},
		{`.schema User`, true},
		{`select User { id };`, true},
		{`insert User { name := "John {Doe}" }`, true},
		{`insert User { name := "John `, false},
		{`select User { id } # comment`, true},
		{"select User { id } with", false},
		{"select User { id } order by", false},
	}

	for _, tt := range tests {
		got := IsCompleteStatement(tt.input)
		if got != tt.complete {
			t.Errorf("IsCompleteStatement(%q) = %v; want %v", tt.input, got, tt.complete)
		}
	}
}

func TestFormatResult(t *testing.T) {
	rows := []runner.Row{
		{"id": "u1", "name": "Alice", "age": 30},
		{"id": "u2", "name": "Bob", "age": 25},
	}
	res := &runner.Result{Rows: rows}

	// Pretty
	pretty := FormatResult(res, FormatPretty)
	if !strings.Contains(pretty, "Alice") || !strings.Contains(pretty, "\n") {
		t.Errorf("FormatResult(pretty) = %s", pretty)
	}

	// Compact
	compact := FormatResult(res, FormatCompact)
	if !strings.Contains(compact, "Alice") || strings.Contains(compact, "\n") {
		t.Errorf("FormatResult(compact) = %s", compact)
	}

	// Table
	table := FormatResult(res, FormatTable)
	if !strings.Contains(table, "+") || !strings.Contains(table, "Alice") || !strings.Contains(table, "| id") {
		t.Errorf("FormatResult(table) =\n%s", table)
	}

	// Mutation result (no rows, rows affected)
	mutRes := &runner.Result{RowsAffected: 5}
	mutOut := FormatResult(mutRes, FormatPretty)
	if !strings.Contains(mutOut, `"rows_affected": 5`) {
		t.Errorf("FormatResult(mutation) = %s", mutOut)
	}
}

func TestFormatTableRowsEmpty(t *testing.T) {
	out := FormatTableRows(nil)
	if out != "(0 rows)" {
		t.Errorf("FormatTableRows(nil) = %s; want (0 rows)", out)
	}
}

func TestMetaCommands(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	schema := &asl.SchemaIR{
		ObjectTypes: map[string]*asl.ResolvedType{
			"User": {
				Name:  "User",
				Table: "users",
				Properties: map[string]*asl.ResolvedProp{
					"id":   {Name: "id", SQLType: "UUID", IsRequired: true},
					"name": {Name: "name", SQLType: "TEXT"},
				},
				Links: map[string]*asl.ResolvedLink{
					"posts": {Name: "posts", TargetType: "Post", IsMulti: true},
				},
			},
			"Post": {
				Name:  "Post",
				Table: "posts",
				Properties: map[string]*asl.ResolvedProp{
					"id":    {Name: "id", SQLType: "UUID", IsRequired: true},
					"title": {Name: "title", SQLType: "TEXT"},
				},
			},
		},
		EnumTypes: map[string]*asl.ResolvedEnum{
			"Role": {Name: "Role", Values: []string{"admin", "user"}},
		},
	}

	r, err := New(Config{
		SchemaIR: schema,
		Out:      &out,
		Err:      &errOut,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()

	// .help
	out.Reset()
	r.handleMetaCommand(ctx, ".help")
	if !strings.Contains(out.String(), "Axel REPL Commands:") {
		t.Errorf(".help output = %s", out.String())
	}

	// .models
	out.Reset()
	r.handleMetaCommand(ctx, ".models")
	if !strings.Contains(out.String(), "User") || !strings.Contains(out.String(), "Post") {
		t.Errorf(".models output = %s", out.String())
	}

	// .schema
	out.Reset()
	r.handleMetaCommand(ctx, ".schema")
	if !strings.Contains(out.String(), "Enums:") || !strings.Contains(out.String(), "Role") {
		t.Errorf(".schema output = %s", out.String())
	}

	// .schema User
	out.Reset()
	r.handleMetaCommand(ctx, ".schema User")
	if !strings.Contains(out.String(), "Model User:") || !strings.Contains(out.String(), "posts") {
		t.Errorf(".schema User output = %s", out.String())
	}

	// .format
	out.Reset()
	r.handleMetaCommand(ctx, ".format table")
	if r.format != FormatTable {
		t.Errorf("r.format = %s; want table", r.format)
	}

	// .param
	out.Reset()
	r.handleMetaCommand(ctx, ".param limit 10")
	if r.params["limit"] != int64(10) {
		t.Errorf("r.params[limit] = %#v; want 10", r.params["limit"])
	}

	// .param json
	r.handleMetaCommand(ctx, `.param json {"skip": 5, "active": true}`)
	if r.params["skip"] != float64(5) || r.params["active"] != true {
		t.Errorf("r.params after json = %#v", r.params)
	}

	// .compile
	out.Reset()
	r.handleMetaCommand(ctx, ".compile select User { id, name }")
	if !strings.Contains(out.String(), "SELECT") && !strings.Contains(out.String(), "json_build_object") {
		t.Errorf(".compile output = %s", out.String())
	}

	// .exit
	if !r.handleMetaCommand(ctx, ".exit") {
		t.Errorf(".exit should return true")
	}
}

func TestCompleter(t *testing.T) {
	schema := &asl.SchemaIR{
		ObjectTypes: map[string]*asl.ResolvedType{
			"User": {
				Name:  "User",
				Table: "users",
			},
			"Post": {
				Name:  "Post",
				Table: "posts",
			},
		},
	}

	r, _ := New(Config{SchemaIR: schema})

	// Dot command completion
	dotMatches := r.complete(".sche")
	if len(dotMatches) == 0 || dotMatches[0] != ".schema" {
		t.Errorf("complete(.sche) = %v; want [.schema]", dotMatches)
	}

	// AQL keyword / type completion
	queryMatches := r.complete("select U")
	foundUser := false
	for _, m := range queryMatches {
		if strings.HasSuffix(m, "User") {
			foundUser = true
			break
		}
	}
	if !foundUser {
		t.Errorf("complete(select U) = %v; expected User in completions", queryMatches)
	}
}
