package repl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/peterh/liner"
	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/asl"
	"github.com/struckchure/axel/core/compiler"
	"github.com/struckchure/axel/core/lsp"
	"github.com/struckchure/axel/core/runner"
)

// Config holds settings for a REPL session.
type Config struct {
	DatabaseURL     string
	SchemaPath      string
	SchemaIR        *asl.SchemaIR
	Format          OutputFormat
	Params          map[string]any
	HistoryPath     string
	RelLoadStrategy string
	Out             io.Writer
	Err             io.Writer
}

// REPL manages an interactive Axel shell session.
type REPL struct {
	cfg      Config
	dbPool   *pgxpool.Pool
	schema   *asl.SchemaIR
	runner   *runner.Runner
	params   map[string]any
	format   OutputFormat
	out      io.Writer
	errOut   io.Writer
	line     *liner.State
	histFile string
}

// New creates and initializes a new REPL instance.
func New(cfg Config) (*REPL, error) {
	out := cfg.Out
	if out == nil {
		out = os.Stdout
	}
	errOut := cfg.Err
	if errOut == nil {
		errOut = os.Stderr
	}

	format := cfg.Format
	if format == "" {
		format = FormatPretty
	}

	params := make(map[string]any)
	for k, v := range cfg.Params {
		params[k] = v
	}

	histFile := cfg.HistoryPath
	if histFile == "" {
		if home, err := os.UserHomeDir(); err == nil {
			histFile = filepath.Join(home, ".axel_history")
		}
	}

	r := &REPL{
		cfg:      cfg,
		schema:   cfg.SchemaIR,
		params:   params,
		format:   format,
		out:      out,
		errOut:   errOut,
		histFile: histFile,
	}

	return r, nil
}

// Init connects to the database (if configured) and initializes schema/runner.
func (r *REPL) Init(ctx context.Context) error {
	if r.cfg.DatabaseURL != "" {
		pool, err := pgxpool.New(ctx, r.cfg.DatabaseURL)
		if err != nil {
			fmt.Fprintf(r.errOut, "Warning: failed to connect to database (%v). Queries will compile only.\n", err)
		} else {
			r.dbPool = pool
		}
	}

	if r.schema == nil && r.cfg.SchemaPath != "" {
		if err := r.reloadSchema(r.cfg.SchemaPath); err != nil {
			fmt.Fprintf(r.errOut, "Warning: could not load schema from %s: %v\n", r.cfg.SchemaPath, err)
		}
	}

	if r.dbPool != nil && r.schema != nil {
		r.runner = runner.New(r.dbPool, r.schema)
	}

	return nil
}

// Close releases any database pool resources and cleans up liner.
func (r *REPL) Close() error {
	if r.dbPool != nil {
		r.dbPool.Close()
		r.dbPool = nil
	}
	if r.line != nil {
		r.saveHistory()
		r.line.Close()
		r.line = nil
	}
	return nil
}

func (r *REPL) reloadSchema(path string) error {
	if path == "" {
		path = r.cfg.SchemaPath
	}
	if path == "" {
		return fmt.Errorf("no schema path specified")
	}
	ir, _, err := asl.LoadIR(path)
	if err != nil {
		return err
	}
	r.cfg.SchemaPath = path
	r.schema = ir
	if r.dbPool != nil {
		r.runner = runner.New(r.dbPool, r.schema)
	}
	return nil
}

// Run starts the interactive REPL loop.
func (r *REPL) Run(ctx context.Context) error {
	if err := r.Init(ctx); err != nil {
		return err
	}
	defer r.Close()

	line := liner.NewLiner()
	r.line = line
	line.SetCtrlCAborts(true)
	line.SetMultiLineMode(true)

	// Set autocompleter
	line.SetCompleter(func(lineText string) []string {
		return r.complete(lineText)
	})

	r.loadHistory()

	r.printBanner()

	var buf strings.Builder

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		prompt := "axel> "
		if buf.Len() > 0 {
			prompt = "...   "
		}

		input, err := line.Prompt(prompt)
		if err != nil {
			if err == liner.ErrPromptAborted {
				// Ctrl+C pressed
				if buf.Len() > 0 {
					buf.Reset()
					fmt.Fprintln(r.out, "^C")
					continue
				}
				fmt.Fprintln(r.out, "^C (use .exit or Ctrl+D to quit)")
				continue
			}
			if err == io.EOF {
				// Ctrl+D pressed
				fmt.Fprintln(r.out, "")
				break
			}
			return err
		}

		trimmed := strings.TrimSpace(input)

		if buf.Len() > 0 {
			buf.WriteString("\n")
			buf.WriteString(input)
		} else {
			if trimmed == "" {
				continue
			}
			buf.WriteString(input)
		}

		fullInput := buf.String()

		if !IsCompleteStatement(fullInput) {
			continue
		}

		// Statement is complete; record in history and execute
		line.AppendHistory(fullInput)
		buf.Reset()

		fullTrimmed := strings.TrimSpace(fullInput)
		if strings.HasSuffix(fullTrimmed, ";") {
			fullTrimmed = strings.TrimSuffix(fullTrimmed, ";")
			fullTrimmed = strings.TrimSpace(fullTrimmed)
		}

		if fullTrimmed == "" {
			continue
		}

		if strings.HasPrefix(fullTrimmed, ".") || strings.HasPrefix(fullTrimmed, "\\") {
			if shouldExit := r.handleMetaCommand(ctx, fullTrimmed); shouldExit {
				break
			}
			continue
		}

		// Execute AQL query
		r.executeQuery(ctx, fullTrimmed)
	}

	return nil
}

func (r *REPL) printBanner() {
	fmt.Fprintln(r.out, "Axel Interactive Query REPL")
	if r.schema != nil {
		modelCount := len(r.schema.ObjectTypes)
		modelsWord := "models"
		if modelCount == 1 {
			modelsWord = "model"
		}
		fmt.Fprintf(r.out, "Schema:   %s (%d %s)\n", r.cfg.SchemaPath, modelCount, modelsWord)
	} else {
		fmt.Fprintln(r.out, "Schema:   (no schema loaded)")
	}

	if r.dbPool != nil {
		fmt.Fprintln(r.out, "Database: connected")
	} else {
		fmt.Fprintln(r.out, "Database: not connected (queries will compile only)")
	}

	fmt.Fprintln(r.out, "Type .help for commands, .exit or Ctrl+D to quit.")
	fmt.Fprintln(r.out, "")
}

func (r *REPL) executeQuery(ctx context.Context, src string) {
	if r.schema == nil {
		fmt.Fprintln(r.errOut, "Error: no schema loaded. Load a schema with .reload <path> or restart with --schema-path.")
		fmt.Fprintln(r.out, "")
		return
	}

	if r.dbPool != nil && r.runner != nil {
		start := time.Now()
		res, err := r.runner.Run(ctx, src, r.params)
		duration := time.Since(start)
		if err != nil {
			fmt.Fprintf(r.errOut, "Error: %v\n\n", err)
			return
		}

		formatted := FormatResult(res, r.format)
		fmt.Fprintln(r.out, formatted)
		fmt.Fprintln(r.out, FormatSummary(res, duration))
		fmt.Fprintln(r.out, "")
		return
	}

	// No database pool connected: compile query and print SQL
	r.compileOnly(src)
}

// handleMetaCommand executes dot commands. Returns true if REPL should exit.
func (r *REPL) handleMetaCommand(ctx context.Context, cmdStr string) bool {
	fields := strings.Fields(cmdStr)
	if len(fields) == 0 {
		return false
	}

	cmd := strings.ToLower(fields[0])
	args := fields[1:]

	switch cmd {
	case ".exit", ".quit", "\\q":
		return true

	case ".help", "\\?", "\\h":
		r.printHelp()

	case ".clear", "\\l":
		fmt.Fprint(r.out, "\033[H\033[2J")

	case ".reload", "\\r":
		targetPath := r.cfg.SchemaPath
		if len(args) > 0 {
			targetPath = args[0]
		}
		if err := r.reloadSchema(targetPath); err != nil {
			fmt.Fprintf(r.errOut, "Error reloading schema: %v\n\n", err)
		} else {
			modelCount := 0
			if r.schema != nil {
				modelCount = len(r.schema.ObjectTypes)
			}
			fmt.Fprintf(r.out, "Schema loaded from %s (%d models).\n\n", r.cfg.SchemaPath, modelCount)
		}

	case ".format":
		if len(args) == 0 {
			fmt.Fprintf(r.out, "Current output format: %s\n\n", r.format)
		} else {
			fmtChoice := OutputFormat(strings.ToLower(args[0]))
			switch fmtChoice {
			case FormatPretty, FormatTable, FormatCompact:
				r.format = fmtChoice
				fmt.Fprintf(r.out, "Output format set to %s.\n\n", r.format)
			default:
				fmt.Fprintf(r.errOut, "Unknown format %q. Choices: pretty, table, compact\n\n", args[0])
			}
		}

	case ".models", ".tables", "\\dt":
		r.printModels()

	case ".schema", "\\d":
		target := ""
		if len(args) > 0 {
			target = args[0]
		}
		r.printSchema(target)

	case ".compile", "\\c":
		if len(args) == 0 {
			fmt.Fprintln(r.errOut, "Usage: .compile <aql query>")
			return false
		}
		querySrc := strings.TrimSpace(cmdStr[len(fields[0]):])
		r.compileOnly(querySrc)

	case ".param", ".params", "\\p":
		r.handleParamCommand(args)

	case ".history":
		r.printHistory()

	default:
		fmt.Fprintf(r.errOut, "Unknown command %q. Type .help for available commands.\n\n", cmd)
	}

	return false
}

func (r *REPL) printHelp() {
	helpText := `Axel REPL Commands:
  .help, \?          Show this help message
  .models, \dt       List all models/types in schema
  .schema [model]    View schema overview or details of a specific model
  .compile <query>   Compile AQL query to SQL without running against DB
  .format [fmt]      Get or set output format (pretty, table, compact)
  .param             List active session parameters
  .param <k> <v>     Set a session parameter (e.g. .param limit 10)
  .param json <obj>  Set parameters from JSON (e.g. .param json {"limit": 10})
  .param clear       Clear all session parameters
  .reload [path], \r Reload schema from disk (or load another .asl file)
  .clear, \l         Clear terminal screen
  .history           Show command history
  .exit, .quit, \q   Exit REPL (or Ctrl+D)

Tips:
  - Multi-line queries are automatically buffered until braces/parentheses close or end with ';'
  - Press Tab for autocompletion of keywords, models, fields, and commands
  - Use Ctrl+C to discard the current line or multi-line buffer
`
	fmt.Fprintln(r.out, helpText)
}

func (r *REPL) printModels() {
	if r.schema == nil || len(r.schema.ObjectTypes) == 0 {
		fmt.Fprintln(r.out, "No models loaded in schema.")
		fmt.Fprintln(r.out, "")
		return
	}

	var names []string
	for name := range r.schema.ObjectTypes {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Fprintln(r.out, "Models:")
	for _, name := range names {
		obj := r.schema.ObjectTypes[name]
		abstractStr := ""
		if obj.IsAbstract {
			abstractStr = " (abstract)"
		}
		propCount := len(obj.Properties)
		linkCount := len(obj.Links)
		fmt.Fprintf(r.out, "  %-20s %s (%d properties, %d links)\n", name, abstractStr, propCount, linkCount)
	}
	fmt.Fprintln(r.out, "")
}

func (r *REPL) printSchema(target string) {
	if r.schema == nil {
		fmt.Fprintln(r.out, "No schema loaded.")
		fmt.Fprintln(r.out, "")
		return
	}

	if target == "" {
		// Overview of entire schema
		r.printModels()

		if len(r.schema.EnumTypes) > 0 {
			var enums []string
			for name := range r.schema.EnumTypes {
				enums = append(enums, name)
			}
			sort.Strings(enums)
			fmt.Fprintln(r.out, "Enums:")
			for _, e := range enums {
				enumObj := r.schema.EnumTypes[e]
				fmt.Fprintf(r.out, "  %-20s [%s]\n", e, strings.Join(enumObj.Values, ", "))
			}
			fmt.Fprintln(r.out, "")
		}

		if len(r.schema.ScalarTypes) > 0 {
			var scalars []string
			for name := range r.schema.ScalarTypes {
				scalars = append(scalars, name)
			}
			sort.Strings(scalars)
			fmt.Fprintln(r.out, "Scalars:")
			for _, s := range scalars {
				sc := r.schema.ScalarTypes[s]
				fmt.Fprintf(r.out, "  %-20s extends %s\n", s, sc.Base)
			}
			fmt.Fprintln(r.out, "")
		}
		return
	}

	// Specific target model or enum
	if obj, ok := r.schema.ObjectTypes[target]; ok {
		fmt.Fprintf(r.out, "Model %s:\n", target)
		if obj.Table != "" {
			fmt.Fprintf(r.out, "  Table: %s\n", obj.Table)
		}
		if obj.IsAbstract {
			fmt.Fprintln(r.out, "  Abstract: true")
		}

		if len(obj.Properties) > 0 {
			fmt.Fprintln(r.out, "  Properties:")
			var pNames []string
			for pn := range obj.Properties {
				pNames = append(pNames, pn)
			}
			sort.Strings(pNames)
			for _, pn := range pNames {
				p := obj.Properties[pn]
				req := ""
				if p.IsRequired {
					req = " (required)"
				}
				typeStr := p.AQLType
				if typeStr == "" {
					typeStr = p.SQLType
				}
				if p.EnumType != "" {
					typeStr = "enum " + p.EnumType
				}
				fmt.Fprintf(r.out, "    %-18s : %s%s\n", pn, typeStr, req)
			}
		}

		if len(obj.Links) > 0 {
			fmt.Fprintln(r.out, "  Links:")
			var lNames []string
			for ln := range obj.Links {
				lNames = append(lNames, ln)
			}
			sort.Strings(lNames)
			for _, ln := range lNames {
				l := obj.Links[ln]
				card := "single"
				if l.IsMulti {
					card = "multi"
				}
				fmt.Fprintf(r.out, "    %-18s -> %s (%s)\n", ln, l.TargetType, card)
			}
		}

		if len(obj.Computed) > 0 {
			fmt.Fprintln(r.out, "  Computed:")
			for cn, c := range obj.Computed {
				fmt.Fprintf(r.out, "    %-18s := %s\n", cn, c.Expr)
			}
		}
		fmt.Fprintln(r.out, "")
		return
	}

	if enumObj, ok := r.schema.EnumTypes[target]; ok {
		fmt.Fprintf(r.out, "Enum %s: [%s]\n\n", target, strings.Join(enumObj.Values, ", "))
		return
	}

	if sc, ok := r.schema.ScalarTypes[target]; ok {
		fmt.Fprintf(r.out, "Scalar %s: extends %s\n\n", target, sc.Base)
		return
	}

	fmt.Fprintf(r.errOut, "Unknown type or model %q in schema.\n\n", target)
}

func (r *REPL) compileOnly(querySrc string) {
	if r.schema == nil {
		fmt.Fprintln(r.errOut, "Error: no schema loaded. Load a schema with .reload <path> or restart with --schema-path.")
		return
	}

	stmt, err := aql.ParseString(querySrc)
	if err != nil {
		fmt.Fprintf(r.errOut, "Parse error: %v\n\n", err)
		return
	}

	var opts compiler.CompileOptions
	opts.RelLoadStrategy = r.cfg.RelLoadStrategy
	compiled, err := compiler.CompileWithOptions(stmt, r.schema, opts)
	if err != nil {
		fmt.Fprintf(r.errOut, "Compile error: %v\n\n", err)
		return
	}

	fmt.Fprintln(r.out, compiled.Full())
	if len(compiled.Params) > 0 {
		var paramNames []string
		for _, p := range compiled.Params {
			paramNames = append(paramNames, fmt.Sprintf("$%s (%s)", p.Name, p.AQLType))
		}
		fmt.Fprintf(r.out, "-- Parameters: %s\n", strings.Join(paramNames, ", "))
	}
	fmt.Fprintln(r.out, "")
}

func (r *REPL) handleParamCommand(args []string) {
	if len(args) == 0 || (len(args) == 1 && args[0] == "list") {
		if len(r.params) == 0 {
			fmt.Fprintln(r.out, "No active session parameters.")
			fmt.Fprintln(r.out, "")
			return
		}
		fmt.Fprintln(r.out, "Active Parameters:")
		var keys []string
		for k := range r.params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b, _ := json.Marshal(r.params[k])
			fmt.Fprintf(r.out, "  %-16s = %s\n", k, string(b))
		}
		fmt.Fprintln(r.out, "")
		return
	}

	if args[0] == "clear" {
		r.params = make(map[string]any)
		fmt.Fprintln(r.out, "Session parameters cleared.")
		fmt.Fprintln(r.out, "")
		return
	}

	if args[0] == "json" {
		if len(args) < 2 {
			fmt.Fprintln(r.errOut, "Usage: .param json <json object>")
			return
		}
		jsonStr := strings.Join(args[1:], " ")
		var parsed map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
			fmt.Fprintf(r.errOut, "Error parsing JSON: %v\n", err)
			return
		}
		for k, v := range parsed {
			r.params[k] = v
		}
		fmt.Fprintf(r.out, "Updated %d parameters.\n\n", len(parsed))
		return
	}

	if len(args) == 1 {
		// Key=Value format or inspect single key
		part := args[0]
		if idx := strings.Index(part, "="); idx >= 0 {
			key := strings.TrimSpace(part[:idx])
			valStr := strings.TrimSpace(part[idx+1:])
			r.setParam(key, valStr)
			return
		}

		key := part
		if val, ok := r.params[key]; ok {
			b, _ := json.Marshal(val)
			fmt.Fprintf(r.out, "%s = %s\n\n", key, string(b))
		} else {
			fmt.Fprintf(r.errOut, "Parameter %q is not set.\n\n", key)
		}
		return
	}

	// Key Value format
	key := args[0]
	valStr := strings.Join(args[1:], " ")
	r.setParam(key, valStr)
}

func (r *REPL) setParam(key, valStr string) {
	var val any
	if valStr == "null" {
		val = nil
	} else if valStr == "true" {
		val = true
	} else if valStr == "false" {
		val = false
	} else if i, err := strconv.ParseInt(valStr, 10, 64); err == nil {
		val = i
	} else if f, err := strconv.ParseFloat(valStr, 64); err == nil {
		val = f
	} else if strings.HasPrefix(valStr, "{") || strings.HasPrefix(valStr, "[") {
		var jsonVal any
		if err := json.Unmarshal([]byte(valStr), &jsonVal); err == nil {
			val = jsonVal
		} else {
			val = valStr
		}
	} else {
		val = strings.Trim(valStr, `"'`)
	}

	r.params[key] = val
	b, _ := json.Marshal(val)
	fmt.Fprintf(r.out, "Set parameter %s = %s\n\n", key, string(b))
}

func (r *REPL) printHistory() {
	if r.histFile == "" {
		fmt.Fprintln(r.out, "No history file configured.")
		return
	}
	f, err := os.Open(r.histFile)
	if err != nil {
		fmt.Fprintf(r.out, "History is empty (%v).\n\n", err)
		return
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		fmt.Fprintf(r.errOut, "Error reading history: %v\n\n", err)
		return
	}
	fmt.Fprintln(r.out, string(data))
}

func (r *REPL) loadHistory() {
	if r.histFile != "" && r.line != nil {
		if f, err := os.Open(r.histFile); err == nil {
			_, _ = r.line.ReadHistory(f)
			_ = f.Close()
		}
	}
}

func (r *REPL) saveHistory() {
	if r.histFile != "" && r.line != nil {
		_ = os.MkdirAll(filepath.Dir(r.histFile), 0755)
		if f, err := os.Create(r.histFile); err == nil {
			_, _ = r.line.WriteHistory(f)
			_ = f.Close()
		}
	}
}

var dotCommands = []string{
	".help", ".schema", ".models", ".tables", ".compile", ".format",
	".param", ".params", ".reload", ".clear", ".history", ".exit", ".quit",
}

func (r *REPL) complete(line string) []string {
	trimmed := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(trimmed, ".") {
		var matches []string
		for _, cmd := range dotCommands {
			if strings.HasPrefix(cmd, trimmed) {
				matches = append(matches, cmd)
			}
		}
		return matches
	}

	// Use lsp QueryCompletion for context-aware completion
	items := lsp.QueryCompletion(line, len(line), r.schema)
	if len(items) == 0 {
		return nil
	}

	// Find the prefix of the last word
	lastWordStart := len(line)
	for lastWordStart > 0 && isWordChar(rune(line[lastWordStart-1])) {
		lastWordStart--
	}
	prefix := line[:lastWordStart]
	typedWord := line[lastWordStart:]

	var completions []string
	for _, item := range items {
		if strings.HasPrefix(item.Label, typedWord) {
			completions = append(completions, prefix+item.Label)
		}
	}

	return completions
}

func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '.' || r == '@' || r == '$'
}
