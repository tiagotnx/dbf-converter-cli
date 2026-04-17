// Package cli exposes a Cobra root command that turns argv into a validated
// Options DTO. Cobra was chosen over plain pflag for three reasons:
//   - free shell completion generation (bash / zsh / fish / powershell)
//   - built-in --help / usage rendering consistent with Go's ecosystem
//   - natural place to attach future subcommands (version, completion, etc.)
//
// The command layer owns validation; file I/O is the responsibility of the
// caller supplied RunFunc so that ParseFlags remains trivially testable.
package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// BuildInfo carries ldflags-injected build metadata surfaced by --version.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Options is the validated CLI surface. It's a dumb DTO — the RunFunc is
// responsible for opening files and calling the converter.
type Options struct {
	Input         string
	Output        string
	Format        string
	Encoding      string
	Where         string
	Head          int
	Schema        bool
	IgnoreDeleted bool
	TableName     string
	Dialect       string   // sql dialect: generic | postgres | mysql | sqlite
	Fields        []string // optional projection: empty = all fields, in DBF order
	Progress      bool     // emit a progress indicator to stderr
	Verbose       bool     // enable slog Debug output
}

// RunFunc receives the validated options and executes the conversion.
type RunFunc func(opts *Options) error

var (
	supportedFormats   = map[string]struct{}{"csv": {}, "jsonl": {}, "sql": {}, "parquet": {}}
	supportedEncodings = map[string]struct{}{"auto": {}, "cp850": {}, "windows-1252": {}, "iso-8859-1": {}, "utf-8": {}}
	supportedDialects  = map[string]struct{}{"generic": {}, "postgres": {}, "mysql": {}, "sqlite": {}}
)

// NewRootCommand wires the full CLI: flags on the root command, plus `version`
// and `completion` subcommands.
func NewRootCommand(info BuildInfo, run RunFunc) *cobra.Command {
	opts := &Options{}

	cmd := &cobra.Command{
		Use:   "dbf-converter",
		Short: "Streaming converter for legacy dBase .dbf files",
		Long: `dbf-converter converts legacy DBF files (dBase III/IV, FoxBase+, Clipper)
into AI-ready CSV / JSONL / SQL / Parquet via streaming, row-by-row processing.

Text is trimmed and decoded to UTF-8, numerics become float64, dates become
ISO-8601 strings, empty values become explicit nulls, and deleted records are
skipped by default. Use '-' as the input/output path to pipe via stdin/stdout.`,
		Example: `  dbf-converter -i clientes.dbf -o clientes.csv
  dbf-converter -i vendas.dbf -o vendas.jsonl -f jsonl --where "STATUS == 'A'"
  cat data.dbf | dbf-converter -i - -o - -f jsonl | jq .
  dbf-converter -i big.dbf -o /dev/null --head 1 --schema   # inspect only`,
		SilenceUsage:  true,
		SilenceErrors: false,
		Version:       formatVersion(info),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOptions(opts); err != nil {
				return err
			}
			return run(opts)
		},
	}

	cmd.SetVersionTemplate("{{.Version}}\n")

	flags := cmd.Flags()
	flags.StringVarP(&opts.Input, "input", "i", "", "Path to the input .dbf file, or '-' for stdin (required)")
	flags.StringVarP(&opts.Output, "output", "o", "", "Path to the output file, or '-' for stdout (required)")
	flags.StringVarP(&opts.Format, "format", "f", "csv", "Output format: csv | jsonl | sql | parquet")
	flags.StringVarP(&opts.Encoding, "encoding", "e", "auto", "Source encoding: auto | cp850 | windows-1252 | iso-8859-1 | utf-8")
	flags.StringVar(&opts.Where, "where", "", "Optional expression filter, e.g. \"STATUS == 'A' && VALOR >= 150\"")
	flags.IntVar(&opts.Head, "head", 0, "Process at most N records (0 = unlimited, counted post-filter)")
	flags.BoolVar(&opts.Schema, "schema", false, "Also emit a [name]_schema.json data dictionary")
	flags.BoolVar(&opts.IgnoreDeleted, "ignore-deleted", true, "Skip records marked as deleted in the DBF")
	flags.StringVar(&opts.TableName, "table", "data", "Table name used by --format sql")
	flags.StringVar(&opts.Dialect, "dialect", "generic", "SQL dialect when --format=sql: generic | postgres | mysql | sqlite")
	flags.StringSliceVar(&opts.Fields, "fields", nil, "Comma-separated subset of fields to emit (default: all)")
	flags.BoolVar(&opts.Progress, "progress", false, "Emit progress lines to stderr every second")
	flags.BoolVar(&opts.Verbose, "verbose", false, "Enable verbose (debug-level) logging")

	_ = cmd.MarkFlagRequired("input")
	_ = cmd.MarkFlagRequired("output")

	cmd.AddCommand(newVersionCmd(info))

	return cmd
}

// newVersionCmd yields a rich `version` subcommand that prints version, commit,
// and build date — complementing the terse `--version` flag.
func newVersionCmd(info BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print detailed version, commit, and build date",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(),
				"dbf-converter %s\n  commit: %s\n  built:  %s\n",
				info.Version, info.Commit, info.Date)
		},
	}
}

func formatVersion(info BuildInfo) string {
	v := info.Version
	if v == "" {
		v = "dev"
	}
	return v
}

func validateOptions(opts *Options) error {
	if opts.Input == "" {
		return fmt.Errorf("--input/-i is required")
	}
	if opts.Output == "" {
		return fmt.Errorf("--output/-o is required")
	}
	opts.Format = strings.ToLower(opts.Format)
	if _, ok := supportedFormats[opts.Format]; !ok {
		return fmt.Errorf("unsupported --format %q (supported: csv, jsonl, sql, parquet)", opts.Format)
	}
	opts.Encoding = strings.ToLower(opts.Encoding)
	if _, ok := supportedEncodings[opts.Encoding]; !ok {
		return fmt.Errorf("unsupported --encoding %q (supported: auto, cp850, windows-1252, iso-8859-1, utf-8)", opts.Encoding)
	}
	opts.Dialect = strings.ToLower(opts.Dialect)
	if _, ok := supportedDialects[opts.Dialect]; !ok {
		return fmt.Errorf("unsupported --dialect %q (supported: generic, postgres, mysql, sqlite)", opts.Dialect)
	}
	if opts.Head < 0 {
		return fmt.Errorf("--head must be >= 0, got %d", opts.Head)
	}
	return nil
}

// ParseFlags executes the root command in "parse-only" mode and returns the
// validated Options. Used by tests to keep contract checks simple.
func ParseFlags(args []string) (*Options, error) {
	var captured *Options
	cmd := NewRootCommand(BuildInfo{Version: "test"}, func(o *Options) error {
		captured = o
		return nil
	})
	cmd.SetArgs(args)
	cmd.SilenceUsage = true
	cmd.SetOut(discardWriter{})
	cmd.SetErr(discardWriter{})
	if err := cmd.Execute(); err != nil {
		return nil, err
	}
	return captured, nil
}

// SchemaPath derives the companion schema filename from an input path.
// E.g. "data/clientes.dbf" -> "data/clientes_schema.json".
// For stdin ("-"), it returns "stdin_schema.json" in the current directory.
func SchemaPath(inputPath string) string {
	if inputPath == "-" {
		return "stdin_schema.json"
	}
	dir := filepath.Dir(inputPath)
	base := filepath.Base(inputPath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	schemaName := name + "_schema.json"
	if dir == "." || dir == "" {
		return schemaName
	}
	return filepath.Join(dir, schemaName)
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
