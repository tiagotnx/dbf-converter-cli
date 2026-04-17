// Package converter wires the DBF reader, filter engine, and format exporter
// into a single streaming pipeline: read → filter → export, one record at a time.
package converter

import (
	"encoding/json"
	"fmt"
	"io"

	"dbf-converter-cli/internal/dbf"
	"dbf-converter-cli/internal/exporter"
	"dbf-converter-cli/internal/filter"
)

// Config is the complete input to Convert. Only Input, Output, Format, and Encoding
// are required; the rest default to their zero-value semantics.
type Config struct {
	Input         io.Reader
	Output        io.Writer
	Format        string // "csv" | "jsonl" | "sql"
	Encoding      string // "cp850" | "windows-1252" | "iso-8859-1"
	Where         string // optional expr-lang filter
	Head          int    // 0 = unlimited
	IgnoreDeleted bool
	TableName     string    // used for --format sql (default "data")
	SchemaOut     io.Writer // optional: when non-nil, schema JSON is written here
}

// Convert runs the full streaming pipeline. Records are never materialized as a
// slice — each row moves reader → filter → exporter → output in order.
func Convert(cfg Config) error {
	if cfg.Input == nil {
		return fmt.Errorf("converter: Input is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("converter: Output is required")
	}
	if cfg.Format == "" {
		cfg.Format = "csv"
	}
	if cfg.Encoding == "" {
		cfg.Encoding = "cp850"
	}
	if cfg.TableName == "" {
		cfg.TableName = "data"
	}

	reader, err := dbf.NewReader(cfg.Input, cfg.Encoding)
	if err != nil {
		return fmt.Errorf("opening dbf: %w", err)
	}
	reader.IgnoreDeleted = cfg.IgnoreDeleted

	exp, err := buildExporter(cfg, reader.Fields())
	if err != nil {
		return err
	}

	if cfg.SchemaOut != nil {
		if err := writeSchema(cfg.SchemaOut, reader.Fields(), reader.NumRecords()); err != nil {
			return fmt.Errorf("writing schema: %w", err)
		}
	}

	flt, err := filter.New(cfg.Where)
	if err != nil {
		return err
	}

	emitted := 0
	for {
		rec, err := reader.Next()
		if err != nil {
			return fmt.Errorf("reading record: %w", err)
		}
		if rec == nil {
			break
		}

		if flt != nil {
			match, err := flt.Match(rec.Values)
			if err != nil {
				return err
			}
			if !match {
				continue
			}
		}

		if err := exp.Write(rec.Values); err != nil {
			return fmt.Errorf("writing record: %w", err)
		}
		emitted++
		if cfg.Head > 0 && emitted >= cfg.Head {
			break
		}
	}

	return exp.Close()
}

func buildExporter(cfg Config, dbfFields []dbf.Field) (exporter.Exporter, error) {
	fields := toExporterFields(dbfFields)
	switch cfg.Format {
	case "csv":
		return exporter.NewCSV(cfg.Output, fields)
	case "jsonl":
		return exporter.NewJSONL(cfg.Output, fields)
	case "sql":
		return exporter.NewSQL(cfg.Output, fields, cfg.TableName)
	default:
		return nil, fmt.Errorf("unsupported format %q (supported: csv, jsonl, sql)", cfg.Format)
	}
}

func toExporterFields(in []dbf.Field) []exporter.Field {
	out := make([]exporter.Field, len(in))
	for i, f := range in {
		out[i] = exporter.Field{Name: f.Name, Type: f.Type}
	}
	return out
}

// writeSchema emits a data dictionary describing every field in the DBF header.
// Useful as an AI-ready companion file for downstream pipelines.
func writeSchema(w io.Writer, fields []dbf.Field, total uint32) error {
	type fieldEntry struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Length  int    `json:"length"`
		Decimal int    `json:"decimal"`
	}
	entries := make([]fieldEntry, len(fields))
	for i, f := range fields {
		entries[i] = fieldEntry{
			Name:    f.Name,
			Type:    string(f.Type),
			Length:  f.Length,
			Decimal: f.Decimal,
		}
	}
	payload := map[string]interface{}{
		"totalRecords": total,
		"fieldCount":   len(fields),
		"fields":       entries,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}
