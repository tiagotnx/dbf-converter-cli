package converter

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildDBF mirrors the helper in the dbf package — duplicated here to keep the
// converter tests independent from internal test helpers.
type fieldDef struct {
	name    string
	typ     byte
	length  byte
	decimal byte
}

func buildDBF(t *testing.T, fields []fieldDef, records [][]string, deleted []bool) []byte {
	t.Helper()

	recordLength := 1
	for _, f := range fields {
		recordLength += int(f.length)
	}
	headerLength := 32 + 32*len(fields) + 1

	var buf bytes.Buffer
	header := make([]byte, 32)
	header[0] = 0x03
	binary.LittleEndian.PutUint32(header[4:8], uint32(len(records)))
	binary.LittleEndian.PutUint16(header[8:10], uint16(headerLength))
	binary.LittleEndian.PutUint16(header[10:12], uint16(recordLength))
	buf.Write(header)

	for _, f := range fields {
		fd := make([]byte, 32)
		copy(fd[0:11], f.name)
		fd[11] = f.typ
		fd[16] = f.length
		fd[17] = f.decimal
		buf.Write(fd)
	}
	buf.WriteByte(0x0D)

	for i, rec := range records {
		if len(deleted) > i && deleted[i] {
			buf.WriteByte('*')
		} else {
			buf.WriteByte(' ')
		}
		for j, val := range rec {
			flen := int(fields[j].length)
			padded := make([]byte, flen)
			for k := range padded {
				padded[k] = ' '
			}
			copy(padded, val)
			buf.Write(padded)
		}
	}
	buf.WriteByte(0x1A)
	return buf.Bytes()
}

func sampleDBF(t *testing.T) []byte {
	fields := []fieldDef{
		{name: "ID", typ: 'N', length: 5},
		{name: "STATUS", typ: 'C', length: 5},
		{name: "VALOR", typ: 'N', length: 10, decimal: 2},
	}
	records := [][]string{
		{"    1", "A    ", "    100.00"},
		{"    2", "A    ", "    250.50"},
		{"    3", "B    ", "    999.99"},
		{"    4", "A    ", "    180.00"},
	}
	return buildDBF(t, fields, records, []bool{false, false, false, false})
}

func TestConvert_CSVEndToEnd(t *testing.T) {
	dbfBytes := sampleDBF(t)
	var out bytes.Buffer

	err := Convert(Config{
		Input:    bytes.NewReader(dbfBytes),
		Output:   &out,
		Format:   "csv",
		Encoding: "cp850",
	})
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	require.Len(t, lines, 5, "header + 4 rows")
	assert.Equal(t, "ID,STATUS,VALOR", lines[0])
	assert.Equal(t, "1,A,100", lines[1])
}

func TestConvert_JSONLWithFilter(t *testing.T) {
	dbfBytes := sampleDBF(t)
	var out bytes.Buffer

	err := Convert(Config{
		Input:    bytes.NewReader(dbfBytes),
		Output:   &out,
		Format:   "jsonl",
		Encoding: "cp850",
		Where:    "STATUS == 'A' && VALOR >= 150",
	})
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	// IDs 2 (A,250.50) and 4 (A,180.00) match. ID 1 (100) below threshold, ID 3 (B) wrong status.
	require.Len(t, lines, 2)

	var r1 map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &r1))
	assert.Equal(t, 2.0, r1["ID"])

	var r2 map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &r2))
	assert.Equal(t, 4.0, r2["ID"])
}

func TestConvert_HeadLimit(t *testing.T) {
	dbfBytes := sampleDBF(t)
	var out bytes.Buffer

	err := Convert(Config{
		Input:    bytes.NewReader(dbfBytes),
		Output:   &out,
		Format:   "csv",
		Encoding: "cp850",
		Head:     2,
	})
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	require.Len(t, lines, 3, "header + 2 rows (head=2)")
}

func TestConvert_HeadCountsAfterFilter(t *testing.T) {
	dbfBytes := sampleDBF(t)
	var out bytes.Buffer

	err := Convert(Config{
		Input:    bytes.NewReader(dbfBytes),
		Output:   &out,
		Format:   "csv",
		Encoding: "cp850",
		Where:    "STATUS == 'A'",
		Head:     2,
	})
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	// STATUS=='A' matches IDs 1, 2, 4 — head=2 limits to first two matches.
	require.Len(t, lines, 3)
	assert.Equal(t, "1,A,100", lines[1])
	assert.Equal(t, "2,A,250.5", lines[2])
}

func TestConvert_IgnoreDeleted(t *testing.T) {
	fields := []fieldDef{
		{name: "ID", typ: 'N', length: 3},
	}
	records := [][]string{{"  1"}, {"  2"}, {"  3"}}
	dbfBytes := buildDBF(t, fields, records, []bool{false, true, false})

	var out bytes.Buffer
	err := Convert(Config{
		Input:          bytes.NewReader(dbfBytes),
		Output:         &out,
		Format:         "csv",
		Encoding:       "cp850",
		IgnoreDeleted:  true,
	})
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	require.Len(t, lines, 3, "header + 2 non-deleted rows")
}

func TestConvert_SchemaGeneration(t *testing.T) {
	dbfBytes := sampleDBF(t)
	var out, schemaBuf bytes.Buffer

	err := Convert(Config{
		Input:      bytes.NewReader(dbfBytes),
		Output:     &out,
		Format:     "csv",
		Encoding:   "cp850",
		SchemaOut:  &schemaBuf,
	})
	require.NoError(t, err)

	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal(schemaBuf.Bytes(), &schema))
	fieldsAny, ok := schema["fields"].([]interface{})
	require.True(t, ok, "schema must expose a 'fields' array")
	require.Len(t, fieldsAny, 3)

	first := fieldsAny[0].(map[string]interface{})
	assert.Equal(t, "ID", first["name"])
	assert.Equal(t, "N", first["type"])
	assert.Equal(t, 5.0, first["length"])
}

func TestConvert_SQLWithTableName(t *testing.T) {
	dbfBytes := sampleDBF(t)
	var out bytes.Buffer

	err := Convert(Config{
		Input:     bytes.NewReader(dbfBytes),
		Output:    &out,
		Format:    "sql",
		Encoding:  "cp850",
		TableName: "clientes",
	})
	require.NoError(t, err)

	s := out.String()
	assert.Contains(t, s, "CREATE TABLE IF NOT EXISTS clientes")
	assert.Contains(t, s, "INSERT INTO clientes")
}
