package compiler

// CompiledSQL is the output of the AQL compiler.
type CompiledSQL struct {
	SQL         string      // parameterized SQL ($1, $2, ...)
	Params      []ParamInfo // Params[i] corresponds to $i+1
	OriginalAQL string
	// Warnings are non-fatal notes about the compiled statement — the SQL is
	// still returned. Callers surface them (stderr for the CLI, LSP warnings in
	// the editor); ignoring them is safe.
	Warnings []string
}

// ParamInfo describes one named parameter in the query.
type ParamInfo struct {
	Name     string // "email", "min_age"
	AQLType  string // "str", "int32", "bool", "uuid" — inferred where possible
	EnumType string // enum type name when the param is enum-backed; "" otherwise
	Optional bool   // true when written as $name? — nullable, skipped-when-null in filters
	Multi    bool   // true when declared as multi $name
	Default  string // compiled default SQL expression if any (e.g. "ARRAY['Hot', 'Cold']::TEXT[]" or "'active'")
}

// Header returns the SQL comment block that maps param names to positions.
func (c *CompiledSQL) Header() string {
	if len(c.Params) == 0 {
		return ""
	}
	var s string
	for i, p := range c.Params {
		s += "-- $" + itoa(i+1) + ": " + p.Name
		if p.AQLType != "" {
			s += " (" + p.AQLType + ")"
		}
		s += "\n"
	}
	return s
}

// Full returns the header comment followed by the SQL.
func (c *CompiledSQL) Full() string {
	h := c.Header()
	if h == "" {
		return c.SQL
	}
	return h + c.SQL
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
