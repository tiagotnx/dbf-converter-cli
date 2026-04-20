package converter

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"
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
		Input:         bytes.NewReader(dbfBytes),
		Output:        &out,
		Format:        "csv",
		Encoding:      "cp850",
		IgnoreDeleted: true,
	})
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	require.Len(t, lines, 3, "header + 2 non-deleted rows")
}

func TestConvert_SchemaGeneration(t *testing.T) {
	dbfBytes := sampleDBF(t)
	var out, schemaBuf bytes.Buffer

	err := Convert(Config{
		Input:     bytes.NewReader(dbfBytes),
		Output:    &out,
		Format:    "csv",
		Encoding:  "cp850",
		SchemaOut: &schemaBuf,
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

// Progress emits ticks at most once per second. The first record should NOT
// trigger a tick just because lastTick started at zero time — otherwise the
// very first line shows bogus numbers like "1/3900 @ 90k rec/s" before the
// rate has stabilized.
func TestConvert_ProgressSuppressesFirstTick(t *testing.T) {
	dbfBytes := sampleDBF(t)
	var out, progOut bytes.Buffer

	err := Convert(Config{
		Input:       bytes.NewReader(dbfBytes),
		Output:      &out,
		Format:      "csv",
		Encoding:    "cp850",
		Progress:    true,
		ProgressOut: &progOut,
		InputPath:   "sample.dbf",
	})
	require.NoError(t, err)

	// Only the finish() line should have written — no intermediate ticks on a
	// sub-second run with just 4 records.
	s := progOut.String()
	lines := strings.Count(s, "\r")
	assert.LessOrEqual(t, lines, 1, "expected at most one (final) tick, got %d: %q", lines, s)
}

// formatProgress is the pure formatter that turns (label, done, total, elapsed)
// into a single progress line. Extracting it from the tick path makes both the
// ETA math and the TTY/plain branches testable without mocking a clock.
func TestFormatProgress_WithTotalAddsETA(t *testing.T) {
	// 300 records done in 3s at total=1500 → rate=100 rec/s → ETA=12s.
	line := formatProgress("vendas.dbf", 300, 1500, 3*time.Second, false)
	assert.Contains(t, line, "300/1500")
	assert.Contains(t, line, "20.0%")
	assert.Contains(t, line, "100 rec/s")
	assert.Contains(t, line, "ETA 00:12")
}

func TestFormatProgress_UnknownTotalOmitsETA(t *testing.T) {
	// total=0 means the header didn't declare the record count; we still
	// report rate but cannot forecast completion.
	line := formatProgress("stdin", 1234, 0, 2*time.Second, false)
	assert.Contains(t, line, "1234 records")
	assert.Contains(t, line, "rec/s")
	assert.NotContains(t, line, "ETA")
}

func TestFormatProgress_TTYAddsBar(t *testing.T) {
	line := formatProgress("vendas.dbf", 300, 1500, 3*time.Second, true)
	// Bar characters should be present only in TTY mode — plain output keeps
	// it out to avoid noisy CI logs.
	assert.Contains(t, line, "[")
	assert.Contains(t, line, "]")
	assert.Contains(t, line, "ETA")
}

func TestFormatProgress_LargeRateUsesK(t *testing.T) {
	// >= 1000 rec/s switches to the compact "1.2k rec/s" form.
	line := formatProgress("big.dbf", 15000, 100000, 10*time.Second, false)
	assert.Contains(t, line, "k rec/s")
}

func TestFormatProgress_LongETAUsesHours(t *testing.T) {
	// Remaining 360000 records at 100 rec/s → 3600s → "01:00:00".
	line := formatProgress("giant.dbf", 100, 360100, 1*time.Second, false)
	assert.Contains(t, line, "ETA 01:00:00")
}

// Convert should populate Stats when the caller provides it, so main.go can
// build a completion summary without the converter owning any UX concern.
func TestConvert_PopulatesStatsWhenProvided(t *testing.T) {
	dbfBytes := sampleDBF(t)
	var out bytes.Buffer
	var stats Stats

	err := Convert(Config{
		Input:    bytes.NewReader(dbfBytes),
		Output:   &out,
		Format:   "csv",
		Encoding: "cp850",
		Stats:    &stats,
	})
	require.NoError(t, err)
	assert.Equal(t, 4, stats.Emitted)
	assert.Equal(t, uint32(4), stats.Total)
	// Windows timers have ~15ms resolution — a 4-record conversion can round
	// to zero. Asserting non-negative documents the invariant without being
	// flaky on slower-clock platforms.
	assert.GreaterOrEqual(t, stats.Elapsed, time.Duration(0))
}

func TestConvert_StatsCountsFilteredRowsOnly(t *testing.T) {
	dbfBytes := sampleDBF(t)
	var out bytes.Buffer
	var stats Stats

	err := Convert(Config{
		Input:    bytes.NewReader(dbfBytes),
		Output:   &out,
		Format:   "csv",
		Encoding: "cp850",
		Where:    "STATUS == 'A'",
		Stats:    &stats,
	})
	require.NoError(t, err)
	// 3 of 4 sample rows have STATUS='A' (IDs 1, 2, 4).
	assert.Equal(t, 3, stats.Emitted)
	assert.Equal(t, uint32(4), stats.Total, "Total always reflects header count, not post-filter")
}

func TestConvert_ParquetEndToEnd(t *testing.T) {
	dbfBytes := sampleDBF(t)
	var out bytes.Buffer

	err := Convert(Config{
		Input:    bytes.NewReader(dbfBytes),
		Output:   &out,
		Format:   "parquet",
		Encoding: "cp850",
	})
	require.NoError(t, err)

	// Verify the result is a structurally valid Parquet file containing the
	// 4 sample records with the expected schema.
	raw := bytes.NewReader(out.Bytes())
	file, err := parquet.OpenFile(raw, int64(out.Len()))
	require.NoError(t, err)
	assert.Equal(t, int64(4), file.NumRows())
	assert.Len(t, file.Schema().Columns(), 3, "ID, STATUS, VALOR")
}
