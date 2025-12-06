package axel

type Model struct {
	Name       string
	IsAbstract bool
	Extends    string
	Fields     []Field
}

type OnTarget struct {
	Name string
	Type string
}

type Field struct {
	Name        string
	Type        string
	IsRequired  bool
	IsMulti     bool
	Constraints []Constraint
	Default     string
	OnTarget    OnTarget // For links
}

type Constraint struct {
	Name string
	Args []string
}
