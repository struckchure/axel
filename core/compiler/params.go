package compiler

import "fmt"

// paramCollector accumulates named AQL parameters ($name) in first-appearance
// order and maps them to positional SQL parameters ($1, $2, ...).
type paramCollector struct {
	params        []ParamInfo
	index         map[string]int // name → 1-based position
	explicitTypes map[string]bool
}

func newParamCollector() *paramCollector {
	return &paramCollector{
		index:         make(map[string]int),
		explicitTypes: make(map[string]bool),
	}
}

// setType updates the AQLType for an already-registered param.
// No-op if the param is unknown or already has a type.
func (p *paramCollector) setType(name, aqlType string) {
	if pos, ok := p.index[name]; ok && aqlType != "" && p.params[pos-1].AQLType == "" {
		p.params[pos-1].AQLType = aqlType
	}
}

// setExplicitType forces the AQLType from an inline annotation ($name<type>),
// overriding any previously inferred type. No-op if the param is unknown.
func (p *paramCollector) setExplicitType(name, aqlType string) {
	if pos, ok := p.index[name]; ok && aqlType != "" {
		p.params[pos-1].AQLType = aqlType
		p.explicitTypes[name] = true
	}
}

func (p *paramCollector) isExplicit(name string) bool {
	return p.explicitTypes[name]
}

// setEnumType records the enum type name for an already-registered param.
// No-op if the param is unknown or already has an enum type.
func (p *paramCollector) setEnumType(name, enumType string) {
	if pos, ok := p.index[name]; ok && enumType != "" && p.params[pos-1].EnumType == "" {
		p.params[pos-1].EnumType = enumType
	}
}

// markOptional flags an already-registered param as optional (nullable /
// skipped-when-null). No-op if the param is unknown.
func (p *paramCollector) markOptional(name string) {
	if pos, ok := p.index[name]; ok {
		p.params[pos-1].Optional = true
	}
}

// markMulti flags an already-registered param as multi (array).
func (p *paramCollector) markMulti(name string) {
	if pos, ok := p.index[name]; ok {
		p.params[pos-1].Multi = true
	}
}

// isMulti returns whether the param is multi.
func (p *paramCollector) isMulti(name string) bool {
	if pos, ok := p.index[name]; ok {
		return p.params[pos-1].Multi
	}
	return false
}

// setDefault records a default SQL expression for the parameter.
func (p *paramCollector) setDefault(name, defaultSQL string) {
	if pos, ok := p.index[name]; ok {
		p.params[pos-1].Default = defaultSQL
	}
}

// getDefault returns the default SQL expression for the parameter.
func (p *paramCollector) getDefault(name string) string {
	if pos, ok := p.index[name]; ok {
		return p.params[pos-1].Default
	}
	return ""
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
