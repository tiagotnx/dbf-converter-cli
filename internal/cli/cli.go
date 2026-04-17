// Package cli turns argv into a validated Options struct. Validation happens
// here (not in converter) so that bad flags are rejected before any I/O
// begins and the user sees a friendly error with the full flag surface.
package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
)

// Options is the validated CLI surface. It's a dumb DTO — the main entrypoint
// is responsible for opening files and handing the derived io.Reader/io.Writer
// to the converter package.
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
}

var (
	supportedFormats   = map[string]struct{}{"csv": {}, "jsonl": {}, "sql": {}}
	supportedEncodings = map[string]struct{}{"cp850": {}, "windows-1252": {}, "iso-8859-1": {}}
)

// ParseFlags parses argv-style args (without the program name) and returns
// validated options.
func ParseFlags(args []string) (*Options, error) {
	fs := pflag.NewFlagSet("dbf-converter-cli", pflag.ContinueOnError)
	fs.SetOutput(io.Discard) // silence on error — we return the error to the caller

	opts := &Options{}
	fs.StringVarP(&opts.Input, "input", "i", "", "Path to the input .dbf file (required)")
	fs.StringVarP(&opts.Output, "output", "o", "", "Path to the output file (required)")
	fs.StringVarP(&opts.Format, "format", "f", "csv", "Output format: csv | jsonl | sql")
	fs.StringVarP(&opts.Encoding, "encoding", "e", "cp850", "Source encoding: cp850 | windows-1252 | iso-8859-1")
	fs.StringVar(&opts.Where, "where", "", "Optional expression filter, e.g. \"STATUS == 'A' && VALOR >= 150\"")
	fs.IntVar(&opts.Head, "head", 0, "Process at most N records (0 = unlimited)")
	fs.BoolVar(&opts.Schema, "schema", false, "Also emit a [name]_schema.json data dictionary")
	fs.BoolVar(&opts.IgnoreDeleted, "ignore-deleted", true, "Skip records marked as deleted in the DBF")
	fs.StringVar(&opts.TableName, "table", "data", "Table name used by --format sql")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if opts.Input == "" {
		return nil, fmt.Errorf("--input/-i is required")
	}
	if opts.Output == "" {
		return nil, fmt.Errorf("--output/-o is required")
	}
	opts.Format = strings.ToLower(opts.Format)
	if _, ok := supportedFormats[opts.Format]; !ok {
		return nil, fmt.Errorf("unsupported --format %q (supported: csv, jsonl, sql)", opts.Format)
	}
	opts.Encoding = strings.ToLower(opts.Encoding)
	if _, ok := supportedEncodings[opts.Encoding]; !ok {
		return nil, fmt.Errorf("unsupported --encoding %q (supported: cp850, windows-1252, iso-8859-1)", opts.Encoding)
	}
	if opts.Head < 0 {
		return nil, fmt.Errorf("--head must be >= 0, got %d", opts.Head)
	}

	return opts, nil
}

// SchemaPath derives the companion schema filename from the input path,
// e.g. "data/clientes.dbf" → "data/clientes_schema.json".
func SchemaPath(inputPath string) string {
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

