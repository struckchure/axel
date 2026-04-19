package codegen

import (
	"fmt"
	"os"
	"path/filepath"
)

// Context is passed to every generator hook. Use WriteFile to emit output.
type Context struct {
	OutDir  string
	Options map[string]string
}

// WriteFile writes content to OutDir/rel, creating parent directories as needed.
func (c *Context) WriteFile(rel string, content []byte) error {
	path := filepath.Join(c.OutDir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	return os.WriteFile(path, content, 0o644)
}

// Generator is implemented by any codegen plugin (native Go or via SubprocessGenerator).
// Axel calls these hooks in the order documented on Walk.
type Generator interface {
	Name() string

	// Schema hooks — fired during ASL schema traversal.
	BeginSchema(ctx *Context, schema SchemaDescriptor) error
	EndSchema(ctx *Context) error
	BeginType(ctx *Context, typ TypeDescriptor) error
	EndType(ctx *Context) error
	OnProperty(ctx *Context, prop PropertyDescriptor) error
	OnLink(ctx *Context, link LinkDescriptor) error
	OnComputed(ctx *Context, comp ComputedDescriptor) error
	OnIndex(ctx *Context, idx IndexDescriptor) error
	OnEnum(ctx *Context, enum EnumDescriptor) error
	OnScalar(ctx *Context, scalar ScalarDescriptor) error

	// Query hook — fired once per compiled AQL query.
	OnQuery(ctx *Context, query QueryDescriptor) error
}

// Walk traverses schema and queries, firing generator hooks in this order:
//
//	BeginSchema
//	  OnScalar (each scalar)
//	  OnEnum   (each enum)
//	  BeginType
//	    OnProperty / OnLink / OnComputed / OnIndex  (each member)
//	  EndType
//	OnQuery (each AQL query)
//	EndSchema
func Walk(schema SchemaDescriptor, queries []QueryDescriptor, gen Generator, ctx *Context) error {
	if err := gen.BeginSchema(ctx, schema); err != nil {
		return fmt.Errorf("[%s] BeginSchema: %w", gen.Name(), err)
	}

	for _, s := range schema.Scalars {
		if err := gen.OnScalar(ctx, s); err != nil {
			return fmt.Errorf("[%s] OnScalar %s: %w", gen.Name(), s.Name, err)
		}
	}

	for _, e := range schema.Enums {
		if err := gen.OnEnum(ctx, e); err != nil {
			return fmt.Errorf("[%s] OnEnum %s: %w", gen.Name(), e.Name, err)
		}
	}

	for _, t := range schema.Types {
		if err := gen.BeginType(ctx, t); err != nil {
			return fmt.Errorf("[%s] BeginType %s: %w", gen.Name(), t.Name, err)
		}
		for _, p := range t.Properties {
			if err := gen.OnProperty(ctx, p); err != nil {
				return fmt.Errorf("[%s] OnProperty %s.%s: %w", gen.Name(), t.Name, p.Name, err)
			}
		}
		for _, l := range t.Links {
			if err := gen.OnLink(ctx, l); err != nil {
				return fmt.Errorf("[%s] OnLink %s.%s: %w", gen.Name(), t.Name, l.Name, err)
			}
		}
		for _, c := range t.Computed {
			if err := gen.OnComputed(ctx, c); err != nil {
				return fmt.Errorf("[%s] OnComputed %s.%s: %w", gen.Name(), t.Name, c.Name, err)
			}
		}
		for _, idx := range t.Indexes {
			if err := gen.OnIndex(ctx, idx); err != nil {
				return fmt.Errorf("[%s] OnIndex %s: %w", gen.Name(), t.Name, err)
			}
		}
		if err := gen.EndType(ctx); err != nil {
			return fmt.Errorf("[%s] EndType %s: %w", gen.Name(), t.Name, err)
		}
	}

	for _, q := range queries {
		if err := gen.OnQuery(ctx, q); err != nil {
			return fmt.Errorf("[%s] OnQuery %s: %w", gen.Name(), q.Name, err)
		}
	}

	if err := gen.EndSchema(ctx); err != nil {
		return fmt.Errorf("[%s] EndSchema: %w", gen.Name(), err)
	}

	return nil
}
