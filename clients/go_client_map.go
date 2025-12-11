package clients

var goTypeMap = map[string]string{
	"str":      "string",
	"int16":    "int16",
	"int32":    "int32",
	"int64":    "int64",
	"float32":  "float32",
	"float64":  "float64",
	"bool":     "bool",
	"uuid":     "uuid.UUID",
	"datetime": "time.Time",
	"json":     "map[string]any",
	"bytes":    "[]byte",
}
