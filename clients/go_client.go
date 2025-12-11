package clients

type TemplateFile struct {
	Source      string
	Destination string
	Values      map[string]string
}

var defaultPackageName = "ax"

var opsFiles = []TemplateFile{
	{
		Source:      "./clients/go_client_op_bool_tpl.go",
		Destination: "op_bool.go",
	},
	{
		Source:      "./clients/go_client_op_datetime_tpl.go",
		Destination: "op_datetime.go",
	},
	{
		Source:      "./clients/go_client_op_number_tpl.go",
		Destination: "op_number.go",
	},
	{
		Source:      "./clients/go_client_op_string_tpl.go",
		Destination: "op_string.go",
	},
	{
		Source:      "./clients/go_client_op_tpl.go",
		Destination: "op.go",
	},
}

var modelFiles = []TemplateFile{
	{
		Source:      "./clients/go_client_model_tpl.go",
		Destination: "{{ .TableName }}_model.go",
	},
	{
		Source:      "./clients/go_client_query_tpl.go",
		Destination: "{{ .TableName }}_query.go",
	},
	{
		Source:      "./clients/go_client_mutation_tpl.go",
		Destination: "{{ .TableName }}_mutation.go",
	},
}
