package exporter

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fieldsFixture() []Field {
	return []Field{
		{Name: "ID", Type: 'N'},
		{Name: "NOME", Type: 'C'},
		{Name: "VALOR", Type: 'N'},
		{Name: "ATIVO", Type: 'L'},
		{Name: "DATA", Type: 'D'},
	}
}

func TestCSVExporter_HeaderAndRows(t *testing.T) {
	var buf bytes.Buffer
	exp, err := NewCSV(&buf, fieldsFixture())
	require.NoError(t, err)

	require.NoError(t, exp.Write(map[string]interface{}{
		"ID": 1.0, "NOME": "João", "VALOR": 150.75, "ATIVO": true, "DATA": "2025-01-15",
	}))
	require.NoError(t, exp.Write(map[string]interface{}{
		"ID": 2.0, "NOME": "Maria", "VALOR": nil, "ATIVO": false, "DATA": nil,
	}))
	require.NoError(t, exp.Close())

	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, "ID,NOME,VALOR,ATIVO,DATA", lines[0])
	assert.Equal(t, "1,João,150.75,true,2025-01-15", lines[1])
	// nil → empty string in CSV (AI-ready: no "null" / "NULL" literals leaking in).
	assert.Equal(t, "2,Maria,,false,", lines[2])
}

func TestCSVExporter_QuotesSpecialChars(t *testing.T) {
	var buf bytes.Buffer
	exp, _ := NewCSV(&buf, []Field{{Name: "TXT", Type: 'C'}})
	require.NoError(t, exp.Write(map[string]interface{}{"TXT": `linha com "aspas", vírgula`}))
	require.NoError(t, exp.Close())

	assert.Contains(t, buf.String(), `"linha com ""aspas"", vírgula"`)
}

func TestJSONLExporter_OneObjectPerLine(t *testing.T) {
	var buf bytes.Buffer
	exp, err := NewJSONL(&buf, fieldsFixture())
	require.NoError(t, err)

	require.NoError(t, exp.Write(map[string]interface{}{
		"ID": 1.0, "NOME": "João", "VALOR": 150.75, "ATIVO": true, "DATA": "2025-01-15",
	}))
	require.NoError(t, exp.Write(map[string]interface{}{
		"ID": 2.0, "NOME": "Maria", "VALOR": nil, "ATIVO": false, "DATA": nil,
	}))
	require.NoError(t, exp.Close())

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	require.Len(t, lines, 2)

	var first map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &first))
	assert.Equal(t, "João", first["NOME"])
	assert.Equal(t, 150.75, first["VALOR"])
	assert.Equal(t, true, first["ATIVO"])
	assert.Equal(t, "2025-01-15", first["DATA"])

	var second map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &second))
	assert.Nil(t, second["VALOR"], "empty numeric must serialize as JSON null")
	assert.Nil(t, second["DATA"], "empty date must serialize as JSON null")
}

func TestSQLExporter_GeneratesInserts(t *testing.T) {
	var buf bytes.Buffer
	exp, err := NewSQL(&buf, fieldsFixture(), "clientes")
	require.NoError(t, err)

	require.NoError(t, exp.Write(map[string]interface{}{
		"ID": 1.0, "NOME": "João", "VALOR": 150.75, "ATIVO": true, "DATA": "2025-01-15",
	}))
	require.NoError(t, exp.Write(map[string]interface{}{
		"ID": 2.0, "NOME": "O'Brien", "VALOR": nil, "ATIVO": false, "DATA": nil,
	}))
	require.NoError(t, exp.Close())

	out := buf.String()

	// CREATE TABLE preamble
	assert.Contains(t, out, "CREATE TABLE IF NOT EXISTS clientes")
	// Row 1: typed literals
	assert.Contains(t, out, "INSERT INTO clientes (ID, NOME, VALOR, ATIVO, DATA) VALUES (1, 'João', 150.75, TRUE, '2025-01-15');")
	// Row 2: NULL for nils and escaped apostrophe
	assert.Contains(t, out, "INSERT INTO clientes (ID, NOME, VALOR, ATIVO, DATA) VALUES (2, 'O''Brien', NULL, FALSE, NULL);")
}

func TestSQLExporter_RejectsInvalidTableName(t *testing.T) {
	var buf bytes.Buffer
	_, err := NewSQL(&buf, fieldsFixture(), "bad table; DROP TABLE x;--")
	assert.Error(t, err, "table names must be validated to prevent SQL injection via --table")
}
