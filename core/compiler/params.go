package compiler

import "fmt"

// paramCollector accumulates named AQL parameters ($name) in first-appearance
// order and maps them to positional SQL parameters ($1, $2, ...).
type paramCollector struct {
	params []ParamInfo
	index  map[string]int // name → 1-based position
}

func newParamCollector() *paramCollector {
	return &paramCollector{index: make(map[string]int)}
}

// setType updates the AQLType for an already-registered param.
// No-op if the param is unknown or already has a type.
func (p *paramCollector) setType(name, aqlType string) {
	if pos, ok := p.index[name]; ok && aqlType != "" && p.params[pos-1].AQLType == "" {
		p.params[pos-1].AQLType = aqlType
	}
}

// add registers a named param and returns its positional placeholder ($1, $2, ...).
// If the param was already registered, returns the same placeholder.
func (p *paramCollector) add(name, aqlType string) string {
	if pos, ok := p.index[name]; ok {
		return fmt.Sprintf("$%d", pos)
	}
	pos := len(p.params) + 1
	p.params = append(p.params, ParamInfo{Name: name, AQLType: aqlType})
	p.index[name] = pos
	return fmt.Sprintf("$%d", pos)
}
