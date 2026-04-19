package codegen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CodegenRequest is the JSON payload sent to a subprocess generator's stdin.
type CodegenRequest struct {
	Schema  SchemaDescriptor  `json:"schema"`
	Queries []QueryDescriptor `json:"queries"`
	Config  RequestConfig     `json:"config"`
}

// RequestConfig carries generator configuration from the CLI.
type RequestConfig struct {
	OutDir  string            `json:"out_dir"`
	Options map[string]string `json:"options"`
}

// CodegenResponse is the JSON payload read from a subprocess generator's stdout.
type CodegenResponse struct {
	Files []OutputFile `json:"files"`
}

// OutputFile is one file to be written by Axel after the generator exits.
type OutputFile struct {
	Path    string `json:"path"`    // relative to out_dir
	Content string `json:"content"`
}

// SubprocessGenerator implements Generator by sending a CodegenRequest to an
// external binary over stdin and reading a CodegenResponse from its stdout.
// The binary can be written in any language.
//
// Stdin → one JSON CodegenRequest, then EOF.
// Stdout ← one JSON CodegenResponse.
// Stderr → forwarded to Axel's stderr.
// Exit non-zero → error.
type SubprocessGenerator struct {
	BinaryPath string

	// accumulated across Walk calls
	schema  SchemaDescriptor
	queries []QueryDescriptor
}

func (g *SubprocessGenerator) Name() string { return filepath.Base(g.BinaryPath) }

// Schema hooks — accumulate data; actual invocation happens in EndSchema.
func (g *SubprocessGenerator) BeginSchema(_ *Context, schema SchemaDescriptor) error {
	g.schema = schema
	return nil
}
func (g *SubprocessGenerator) OnScalar(_ *Context, _ ScalarDescriptor) error   { return nil }
func (g *SubprocessGenerator) OnEnum(_ *Context, _ EnumDescriptor) error       { return nil }
func (g *SubprocessGenerator) BeginType(_ *Context, _ TypeDescriptor) error    { return nil }
func (g *SubprocessGenerator) OnProperty(_ *Context, _ PropertyDescriptor) error { return nil }
func (g *SubprocessGenerator) OnLink(_ *Context, _ LinkDescriptor) error       { return nil }
func (g *SubprocessGenerator) OnComputed(_ *Context, _ ComputedDescriptor) error { return nil }
func (g *SubprocessGenerator) OnIndex(_ *Context, _ IndexDescriptor) error     { return nil }
func (g *SubprocessGenerator) EndType(_ *Context) error                        { return nil }

// OnQuery accumulates each query descriptor; they are included in the request.
func (g *SubprocessGenerator) OnQuery(_ *Context, q QueryDescriptor) error {
	g.queries = append(g.queries, q)
	return nil
}

// EndSchema sends the full CodegenRequest to the subprocess and writes output files.
func (g *SubprocessGenerator) EndSchema(ctx *Context) error {
	req := CodegenRequest{
		Schema:  g.schema,
		Queries: g.queries,
		Config: RequestConfig{
			OutDir:  ctx.OutDir,
			Options: ctx.Options,
		},
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	cmd := exec.Command(g.BinaryPath)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("generator exited with error: %w", err)
	}

	var resp CodegenResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return fmt.Errorf("parsing generator response: %w", err)
	}

	for _, f := range resp.Files {
		if err := ctx.WriteFile(f.Path, []byte(f.Content)); err != nil {
			return fmt.Errorf("writing %s: %w", f.Path, err)
		}
	}
	return nil
}
