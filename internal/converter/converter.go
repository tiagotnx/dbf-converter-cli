// Package converter wires the DBF reader, filter engine, and format exporter
// into a single streaming pipeline: read → filter → export, one record at a time.
package converter

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"dbf-converter-cli/internal/dbf"
	"dbf-converter-cli/internal/exporter"
	"dbf-converter-cli/internal/filter"
)

// Config is the complete input to Convert. Only Input, Output, and Format
// are strictly required; everything else has a sensible default.
type Config struct {
	Input         io.Reader
	Output        io.Writer
	Format        string // "csv" | "jsonl" | "sql" | "parquet"
	Encoding      string // "auto" | "cp850" | "windows-1252" | "iso-8859-1" | "utf-8"
	Where         string // optional expr-lang filter
	Head          int    // 0 = unlimited
	IgnoreDeleted bool
	TableName     string    // --format sql (default "data")
	Dialect       string    // --dialect (generic|postgres|mysql|sqlite), default "generic"
	Fields        []string  // projection — empty = all fields, in DBF order
	Progress      bool      // emit progress lines to ProgressOut
	ProgressOut   io.Writer // where progress lines go (typically os.Stderr)
	ProgressIsTTY bool      // when true, progress renders a visual bar
	InputPath     string    // label used in progress output; optional
	SchemaOut     io.Writer // optional — non-nil = emit data dictionary JSON
	Stats         *Stats    // optional — populated on successful return
}

// Stats reports how much work Convert actually did. The caller supplies a
// non-nil pointer via Config.Stats; Convert writes the final tallies before
// returning. Useful for building a completion summary line outside the
// converter without giving it a UX responsibility.
type Stats struct {
	Emitted int           // rows written to the exporter (post-filter, post-head)
	Total   uint32        // records declared in the DBF header (includes deleted)
	Elapsed time.Duration // wall-clock time spent inside Convert
}

// Convert runs the full streaming pipeline. Records are never materialized as
// a slice — each row moves reader → filter → exporter → output in order.
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
	if cfg.Dialect == "" {
		cfg.Dialect = "generic"
	}

	start := time.Now()

	reader, err := dbf.NewReader(cfg.Input, cfg.Encoding)
	if err != nil {
		return fmt.Errorf("opening dbf: %w", err)
	}
	reader.IgnoreDeleted = cfg.IgnoreDeleted

	emittedFields, err := resolveFields(reader.Fields(), cfg.Fields)
	if err != nil {
		return err
	}

	exp, err := buildExporter(cfg, emittedFields)
	if err != nil {
		return err
	}

	if cfg.SchemaOut != nil {
		if err := writeSchema(cfg.SchemaOut, emittedFields, reader.NumRecords()); err != nil {
			return fmt.Errorf("writing schema: %w", err)
		}
	}

	flt, err := filter.New(cfg.Where)
	if err != nil {
		return err
	}

	prog := newProgress(cfg, reader.NumRecords())

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
				prog.tick(emitted)
				continue
			}
		}

		row := rec.Values
		if len(cfg.Fields) > 0 {
			row = projectRow(rec.Values, cfg.Fields)
		}

		if err := exp.Write(row); err != nil {
			return fmt.Errorf("writing record: %w", err)
		}
		emitted++
		prog.tick(emitted)
		if cfg.Head > 0 && emitted >= cfg.Head {
			break
		}
	}
	prog.finish(emitted)

	if err := exp.Close(); err != nil {
		return err
	}

	if cfg.Stats != nil {
		cfg.Stats.Emitted = emitted
		cfg.Stats.Total = reader.NumRecords()
		cfg.Stats.Elapsed = time.Since(start)
	}
	return nil
}

// resolveFields optionally narrows the field list to the --fields projection,
// preserving the user-supplied order and erroring on unknown names.
func resolveFields(all []dbf.Field, projection []string) ([]dbf.Field, error) {
	if len(projection) == 0 {
		return all, nil
	}
	byName := make(map[string]dbf.Field, len(all))
	for _, f := range all {
		byName[f.Name] = f
	}
	out := make([]dbf.Field, 0, len(projection))
	for _, name := range projection {
		f, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("--fields: unknown column %q (available: %s)", name, fieldNames(all))
		}
		out = append(out, f)
	}
	return out, nil
}

func fieldNames(fields []dbf.Field) string {
	names := make([]byte, 0, len(fields)*8)
	for i, f := range fields {
		if i > 0 {
			names = append(names, ',', ' ')
		}
		names = append(names, f.Name...)
	}
	return string(names)
}

// projectRow returns a copy of row restricted to the requested fields.
func projectRow(row map[string]interface{}, fields []string) map[string]interface{} {
	out := make(map[string]interface{}, len(fields))
	for _, f := range fields {
		out[f] = row[f]
	}
	return out
}

func buildExporter(cfg Config, dbfFields []dbf.Field) (exporter.Exporter, error) {
	fields := toExporterFields(dbfFields)
	switch cfg.Format {
	case "csv":
		return exporter.NewCSV(cfg.Output, fields)
	case "jsonl":
		return exporter.NewJSONL(cfg.Output, fields)
	case "sql":
		return exporter.NewSQLWithDialect(cfg.Output, fields, cfg.TableName, cfg.Dialect)
	case "parquet":
		return exporter.NewParquet(cfg.Output, fields)
	default:
		return nil, fmt.Errorf("unsupported format %q (supported: csv, jsonl, sql, parquet)", cfg.Format)
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

// progress emits a single-line stderr update at most once per second so that
// long conversions show liveness without spamming the terminal.
type progress struct {
	enabled  bool
	out      io.Writer
	label    string
	total    uint32
	isTTY    bool
	lastTick time.Time
	start    time.Time
}

func newProgress(cfg Config, total uint32) *progress {
	if !cfg.Progress || cfg.ProgressOut == nil {
		return &progress{}
	}
	label := cfg.InputPath
	if label == "" || label == "-" {
		label = "stdin"
	} else {
		label = filepath.Base(label)
	}
	start := time.Now()
	return &progress{
		enabled:  true,
		out:      cfg.ProgressOut,
		label:    label,
		total:    total,
		isTTY:    cfg.ProgressIsTTY,
		start:    start,
		lastTick: start, // avoid spurious sub-second first-tick output
	}
}

func (p *progress) tick(done int) {
	if !p.enabled {
		return
	}
	now := time.Now()
	if now.Sub(p.lastTick) < time.Second {
		return
	}
	p.lastTick = now
	p.write(done, now)
}

func (p *progress) finish(done int) {
	if !p.enabled {
		return
	}
	p.write(done, time.Now())
	fmt.Fprintln(p.out)
}

func (p *progress) write(done int, now time.Time) {
	fmt.Fprint(p.out, "\r", formatProgress(p.label, done, p.total, now.Sub(p.start), p.isTTY))
}

// formatProgress renders one progress line. Pure function — no clock, no
// writer — so tests can assert on the exact string for every branch.
//
// Fields shown:
//   - label: input basename (or "stdin")
//   - done/total when total is known, plus percentage and ETA
//   - rate in rec/s (compact "k rec/s" past 1000)
//   - TTY mode adds a fixed-width ASCII bar; plain mode stays CI-friendly
func formatProgress(label string, done int, total uint32, elapsed time.Duration, isTTY bool) string {
	rate := 0.0
	if elapsed > 0 {
		rate = float64(done) / elapsed.Seconds()
	}

	var b strings.Builder
	b.WriteString(label)

	if total > 0 {
		pct := 100 * float64(done) / float64(total)
		if isTTY {
			b.WriteString(" ")
			b.WriteString(renderBar(pct, 20))
		}
		fmt.Fprintf(&b, " %.1f%% %d/%d", pct, done, total)
	} else {
		fmt.Fprintf(&b, ": %d records", done)
	}

	b.WriteString(" @ ")
	b.WriteString(formatRate(rate))

	if total > 0 && rate > 0 {
		remaining := int64(total) - int64(done)
		if remaining > 0 {
			etaSeconds := float64(remaining) / rate
			fmt.Fprintf(&b, " ETA %s", formatDuration(time.Duration(etaSeconds*float64(time.Second))))
		}
	}
	return b.String()
}

func renderBar(pct float64, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(pct / 100 * float64(width))
	var b strings.Builder
	b.Grow(width + 2)
	b.WriteByte('[')
	for i := 0; i < width; i++ {
		switch {
		case i < filled:
			b.WriteByte('=')
		case i == filled:
			b.WriteByte('>')
		default:
			b.WriteByte(' ')
		}
	}
	b.WriteByte(']')
	return b.String()
}

func formatRate(rate float64) string {
	if rate >= 1000 {
		return fmt.Sprintf("%.1fk rec/s", rate/1000)
	}
	return fmt.Sprintf("%.0f rec/s", rate)
}

// formatDuration renders a compact m:ss / h:mm:ss string. Tests care about
// the exact shape ("00:12", "01:00:00"), so keep this deterministic.
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Seconds())
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}
