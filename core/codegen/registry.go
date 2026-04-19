package codegen

import "fmt"

var registry = map[string]Generator{}

// Register adds a built-in generator. Called from init() in generator packages.
func Register(g Generator) {
	registry[g.Name()] = g
}

// Lookup returns a registered generator by name.
func Lookup(name string) (Generator, error) {
	g, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("no built-in generator named %q (available: use --plugin for external generators)", name)
	}
	return g, nil
}
