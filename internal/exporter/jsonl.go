package exporter

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// JSONLExporter serializes one JSON object per line (.jsonl / NDJSON).
// Field ordering follows the DBF header for deterministic output.
type JSONLExporter struct {
	w      *bufio.Writer
	fields []Field
}

func NewJSONL(w io.Writer, fields []Field) (*JSONLExporter, error) {
	return &JSONLExporter{w: bufio.NewWriter(w), fields: fields}, nil
}

func (e *JSONLExporter) Write(row map[string]interface{}) error {
	// encoding/json on a map would alphabetize keys; we want DBF declaration order,
	// so we stream the object ourselves using json.Marshal on each value.
	if err := e.w.WriteByte('{'); err != nil {
		return err
	}
	for i, f := range e.fields {
		if i > 0 {
			if err := e.w.WriteByte(','); err != nil {
				return err
			}
		}
		keyBytes, err := json.Marshal(f.Name)
		if err != nil {
			return fmt.Errorf("marshaling key %s: %w", f.Name, err)
		}
		if _, err := e.w.Write(keyBytes); err != nil {
			return err
		}
		if err := e.w.WriteByte(':'); err != nil {
			return err
		}
		valBytes, err := json.Marshal(row[f.Name])
		if err != nil {
			return fmt.Errorf("marshaling value for %s: %w", f.Name, err)
		}
		if _, err := e.w.Write(valBytes); err != nil {
			return err
		}
	}
	if err := e.w.WriteByte('}'); err != nil {
		return err
	}
	return e.w.WriteByte('\n')
}

func (e *JSONLExporter) Close() error { return e.w.Flush() }
