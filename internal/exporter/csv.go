package exporter

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
)

// CSVExporter writes rows as comma-separated values with an AI-ready shape:
// nils become empty cells (no "null"/"NULL" literals) and numerics use %g to
// avoid trailing zeros while preserving precision.
type CSVExporter struct {
	w      *csv.Writer
	fields []Field
	record []string
}

func NewCSV(w io.Writer, fields []Field) (*CSVExporter, error) {
	cw := csv.NewWriter(w)
	header := make([]string, len(fields))
	for i, f := range fields {
		header[i] = f.Name
	}
	if err := cw.Write(header); err != nil {
		return nil, fmt.Errorf("writing csv header: %w", err)
	}
	return &CSVExporter{w: cw, fields: fields, record: make([]string, len(fields))}, nil
}

func (e *CSVExporter) Write(row map[string]interface{}) error {
	for i, f := range e.fields {
		e.record[i] = csvCell(row[f.Name])
	}
	if err := e.w.Write(e.record); err != nil {
		return fmt.Errorf("writing csv row: %w", err)
	}
	return nil
}

func (e *CSVExporter) Close() error {
	e.w.Flush()
	return e.w.Error()
}

func csvCell(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", x)
	}
}
