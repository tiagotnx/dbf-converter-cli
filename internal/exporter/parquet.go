package exporter

import (
	"fmt"
	"io"

	"github.com/parquet-go/parquet-go"
)

// ParquetExporter writes DBF records to Apache Parquet. Every column is
// declared as optional so that AI-ready nulls (empty numerics, empty dates,
// indeterminate logicals) round-trip faithfully instead of being coerced to
// zero values.
//
// Rows are buffered internally into Parquet row groups; callers MUST call
// Close to flush the footer — otherwise the resulting file is unreadable.
type ParquetExporter struct {
	w      *parquet.GenericWriter[map[string]any]
	fields []Field
	// buf is a single-element slice reused per Write call to avoid allocating
	// one wrapper slice per record.
	buf []map[string]any
}

// NewParquet builds a ParquetExporter whose schema mirrors the DBF field list
// in the same order, so column ordering in the output matches what CSV/JSONL
// produce for the same input.
func NewParquet(w io.Writer, fields []Field) (*ParquetExporter, error) {
	group := parquet.Group{}
	for _, f := range fields {
		node, err := parquetNodeFor(f.Type)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", f.Name, err)
		}
		group[f.Name] = parquet.Optional(node)
	}
	schema := parquet.NewSchema("record", group)

	return &ParquetExporter{
		w:      parquet.NewGenericWriter[map[string]any](w, schema),
		fields: fields,
		buf:    make([]map[string]any, 1),
	}, nil
}

func (e *ParquetExporter) Write(row map[string]interface{}) error {
	e.buf[0] = row
	_, err := e.w.Write(e.buf)
	return err
}

func (e *ParquetExporter) Close() error { return e.w.Close() }

// parquetNodeFor maps DBF type codes to Parquet logical leaf types. Numeric
// columns use DoubleType to match the float64 normalization performed by the
// DBF reader (which folds both N and F into float64 regardless of declared
// decimals). Dates stay as ISO-8601 strings — the reader has already
// normalized them, so keeping the string form avoids a second lossy
// conversion through Parquet's INT32(DATE) encoding.
func parquetNodeFor(t byte) (parquet.Node, error) {
	switch t {
	case 'C', 'M', 'D':
		return parquet.String(), nil
	case 'N', 'F', 'I':
		return parquet.Leaf(parquet.DoubleType), nil
	case 'L':
		return parquet.Leaf(parquet.BooleanType), nil
	default:
		// Unknown field types: fall back to string so the column is still usable
		// rather than failing the whole export.
		return parquet.String(), nil
	}
}
