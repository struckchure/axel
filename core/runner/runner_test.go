package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/struckchure/axel/core/asl"
	"github.com/struckchure/axel/core/compiler"
)

func TestBuildArgsFollowsCompiledParameterOrder(t *testing.T) {
	params := []compiler.ParamInfo{
		{Name: "tenant_id", AQLType: "uuid"},
		{Name: "limit", AQLType: "int32"},
		{Name: "optional", AQLType: "str", Optional: true},
	}
	args := buildArgs(params, map[string]any{"limit": int32(10), "tenant_id": "tenant-1"})
	if len(args) != 3 || args[0] != "tenant-1" || args[1] != int32(10) || args[2] != nil {
		t.Errorf("args = %#v", args)
	}
}

func TestRunReturnsContextualParseAndCompileErrors(t *testing.T) {
	r := New(nil, &asl.SchemaIR{ObjectTypes: map[string]*asl.ResolvedType{}})

	if _, err := r.Run(context.Background(), `select User {`, nil); err == nil || !strings.Contains(err.Error(), "axel/runner: parse:") {
		t.Errorf("parse error = %v", err)
	}
	if _, err := r.Run(context.Background(), `select User { id };`, nil); err == nil || !strings.Contains(err.Error(), "axel/runner: compile:") {
		t.Errorf("compile error = %v", err)
	}
}
