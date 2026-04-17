package exporter

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParquetExporter_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	exp, err := NewParquet(&buf, fieldsFixture())
	require.NoError(t, err)

	require.NoError(t, exp.Write(map[string]interface{}{
		"ID": 1.0, "NOME": "João", "VALOR": 150.75, "ATIVO": true, "DATA": "2025-01-15",
	}))
	require.NoError(t, exp.Write(map[string]interface{}{
		"ID": 2.0, "NOME": "Maria", "VALOR": nil, "ATIVO": false, "DATA": nil,
	}))
	require.NoError(t, exp.Close())

	// Read it back using parquet-go's generic reader with the schema parsed
	// from the file footer — confirms the file is structurally valid and that
	// nulls round-trip as Go nil (AI-ready, no zero-value coercion).
	raw := bytes.NewReader(buf.Bytes())
	file, err := parquet.OpenFile(raw, int64(buf.Len()))
	require.NoError(t, err)
	reader := parquet.NewGenericReader[map[string]any](raw, file.Schema())
	defer reader.Close()

	rows := make([]map[string]any, 2)
	for i := range rows {
		rows[i] = map[string]any{}
	}
	n, err := reader.Read(rows)
	// Read signals end-of-stream via io.EOF even on the successful final batch;
	// the returned n is still the number of rows populated.
	if err != nil && !errors.Is(err, io.EOF) {
		require.NoError(t, err)
	}
	require.Equal(t, 2, n, "expected 2 rows round-tripped")

	assert.Equal(t, 1.0, rows[0]["ID"])
	assert.Equal(t, "João", rows[0]["NOME"])
	assert.Equal(t, 150.75, rows[0]["VALOR"])
	assert.Equal(t, true, rows[0]["ATIVO"])
	assert.Equal(t, "2025-01-15", rows[0]["DATA"])

	assert.Equal(t, 2.0, rows[1]["ID"])
	assert.Equal(t, "Maria", rows[1]["NOME"])
	assert.Nil(t, rows[1]["VALOR"], "empty numeric must round-trip as Parquet null")
	assert.Equal(t, false, rows[1]["ATIVO"])
	assert.Nil(t, rows[1]["DATA"], "empty date must round-trip as Parquet null")
}

func TestParquetExporter_EmptyInput(t *testing.T) {
	var buf bytes.Buffer
	exp, err := NewParquet(&buf, fieldsFixture())
	require.NoError(t, err)
	require.NoError(t, exp.Close())

	// File must still be a valid Parquet with zero rows and the declared schema.
	raw := bytes.NewReader(buf.Bytes())
	file, err := parquet.OpenFile(raw, int64(buf.Len()))
	require.NoError(t, err)
	assert.Equal(t, int64(0), file.NumRows())
	assert.Len(t, file.Schema().Columns(), 5, "schema column count must match input fields")
}

func TestParquetExporter_AllFieldsPresent(t *testing.T) {
	// Unlike CSV/JSONL, Parquet is columnar: column order in the file is not
	// meaningful — consumers (Spark, DuckDB, pandas) address columns by name.
	// This test asserts that every input field is encoded as a column, without
	// constraining the order in which parquet-go serializes them.
	var buf bytes.Buffer
	fields := []Field{
		{Name: "DATA", Type: 'D'},
		{Name: "ATIVO", Type: 'L'},
		{Name: "VALOR", Type: 'N'},
		{Name: "NOME", Type: 'C'},
		{Name: "ID", Type: 'N'},
	}
	exp, err := NewParquet(&buf, fields)
	require.NoError(t, err)
	require.NoError(t, exp.Close())

	raw := bytes.NewReader(buf.Bytes())
	file, err := parquet.OpenFile(raw, int64(buf.Len()))
	require.NoError(t, err)

	cols := file.Schema().Columns()
	got := make(map[string]struct{}, len(cols))
	for _, path := range cols {
		got[path[0]] = struct{}{}
	}
	for _, f := range fields {
		_, ok := got[f.Name]
		assert.True(t, ok, "expected column %q in Parquet schema", f.Name)
	}
}
