package dbf

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildDBF builds an in-memory DBF byte stream for testing purposes.
// Supports C (character), N (numeric), D (date), L (logical) fields.
type fieldDef struct {
	name    string
	typ     byte
	length  byte
	decimal byte
}

func buildDBF(t *testing.T, fields []fieldDef, records [][]string, deleted []bool) []byte {
	t.Helper()

	recordLength := 1 // deletion marker
	for _, f := range fields {
		recordLength += int(f.length)
	}
	headerLength := 32 + 32*len(fields) + 1

	var buf bytes.Buffer

	// Header
	header := make([]byte, 32)
	header[0] = 0x03 // dBase III
	header[1] = 125  // YY (2025)
	header[2] = 1
	header[3] = 1
	binary.LittleEndian.PutUint32(header[4:8], uint32(len(records)))
	binary.LittleEndian.PutUint16(header[8:10], uint16(headerLength))
	binary.LittleEndian.PutUint16(header[10:12], uint16(recordLength))
	buf.Write(header)

	// Field descriptors
	for _, f := range fields {
		fd := make([]byte, 32)
		copy(fd[0:11], f.name)
		fd[11] = f.typ
		fd[16] = f.length
		fd[17] = f.decimal
		buf.Write(fd)
	}

	// Header terminator
	buf.WriteByte(0x0D)

	// Records
	for i, rec := range records {
		if len(deleted) > i && deleted[i] {
			buf.WriteByte('*')
		} else {
			buf.WriteByte(' ')
		}
		require.Equal(t, len(fields), len(rec), "record %d has wrong field count", i)
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

	// EOF marker
	buf.WriteByte(0x1A)

	return buf.Bytes()
}

func TestReader_HeaderAndFields(t *testing.T) {
	fields := []fieldDef{
		{name: "NOME", typ: 'C', length: 20},
		{name: "IDADE", typ: 'N', length: 3},
	}
	data := buildDBF(t, fields, [][]string{{"Joao", "30"}}, nil)

	r, err := NewReader(bytes.NewReader(data), "cp850")
	require.NoError(t, err)

	assert.Equal(t, uint32(1), r.NumRecords())
	fs := r.Fields()
	require.Len(t, fs, 2)
	assert.Equal(t, "NOME", fs[0].Name)
	assert.Equal(t, byte('C'), fs[0].Type)
	assert.Equal(t, "IDADE", fs[1].Name)
	assert.Equal(t, byte('N'), fs[1].Type)
}

func TestReader_TrimAndEncoding(t *testing.T) {
	// "Jo\xe3o" in CP850 = J o 0xE3 o (actually 0xE3 in CP850 is not 'ã'),
	// In CP850, 'ã' = 0xC6; in windows-1252/iso-8859-1, 'ã' = 0xE3.
	// Test with iso-8859-1 for clarity.
	fields := []fieldDef{
		{name: "NOME", typ: 'C', length: 10},
	}
	// "Jo\xe3o      " (4 chars + 6 trailing spaces padding)
	records := [][]string{{string([]byte{'J', 'o', 0xE3, 'o'})}}
	data := buildDBF(t, fields, records, nil)

	r, err := NewReader(bytes.NewReader(data), "iso-8859-1")
	require.NoError(t, err)

	rec, err := r.Next()
	require.NoError(t, err)
	require.NotNil(t, rec)

	// Text fields should be trimmed and decoded to UTF-8.
	assert.Equal(t, "João", rec.Values["NOME"], "must trim padding AND decode ISO-8859-1 to UTF-8")
}

func TestReader_NumericConversion(t *testing.T) {
	fields := []fieldDef{
		{name: "VALOR", typ: 'N', length: 10, decimal: 2},
		{name: "QTD", typ: 'N', length: 5},
	}
	records := [][]string{
		{"   150.75", "    3"},
		{"  -999.99", "   -1"},
	}
	data := buildDBF(t, fields, records, nil)

	r, err := NewReader(bytes.NewReader(data), "cp850")
	require.NoError(t, err)

	rec, err := r.Next()
	require.NoError(t, err)
	// Numeric must be a float64 for expression engine compatibility.
	assert.InDelta(t, 150.75, rec.Values["VALOR"], 0.0001)
	assert.InDelta(t, 3.0, rec.Values["QTD"], 0.0001)

	rec2, err := r.Next()
	require.NoError(t, err)
	assert.InDelta(t, -999.99, rec2.Values["VALOR"], 0.0001)
	assert.InDelta(t, -1.0, rec2.Values["QTD"], 0.0001)
}

func TestReader_EmptyNumericBecomesNil(t *testing.T) {
	fields := []fieldDef{
		{name: "VALOR", typ: 'N', length: 10, decimal: 2},
	}
	records := [][]string{{"          "}}
	data := buildDBF(t, fields, records, nil)

	r, err := NewReader(bytes.NewReader(data), "cp850")
	require.NoError(t, err)

	rec, err := r.Next()
	require.NoError(t, err)
	assert.Nil(t, rec.Values["VALOR"], "empty numeric must be nil")
}

func TestReader_DateNormalization(t *testing.T) {
	fields := []fieldDef{
		{name: "DATA", typ: 'D', length: 8},
		{name: "NASC", typ: 'D', length: 8},
		{name: "INVA", typ: 'D', length: 8},
	}
	records := [][]string{
		{"20250115", "        ", "  /  /  "}, // valid, empty, garbage
	}
	data := buildDBF(t, fields, records, nil)

	r, err := NewReader(bytes.NewReader(data), "cp850")
	require.NoError(t, err)

	rec, err := r.Next()
	require.NoError(t, err)
	assert.Equal(t, "2025-01-15", rec.Values["DATA"], "valid date should be ISO formatted")
	assert.Nil(t, rec.Values["NASC"], "empty date must be nil")
	assert.Nil(t, rec.Values["INVA"], "invalid date '  /  /  ' must be nil")
}

func TestReader_LogicalField(t *testing.T) {
	fields := []fieldDef{
		{name: "ATIVO", typ: 'L', length: 1},
		{name: "PAGO", typ: 'L', length: 1},
		{name: "VAZIO", typ: 'L', length: 1},
	}
	records := [][]string{{"T", "N", "?"}}
	data := buildDBF(t, fields, records, nil)

	r, err := NewReader(bytes.NewReader(data), "cp850")
	require.NoError(t, err)

	rec, err := r.Next()
	require.NoError(t, err)
	assert.Equal(t, true, rec.Values["ATIVO"])
	assert.Equal(t, false, rec.Values["PAGO"])
	assert.Nil(t, rec.Values["VAZIO"], "undefined logical '?' must be nil")
}

func TestReader_SkipDeleted(t *testing.T) {
	fields := []fieldDef{
		{name: "NOME", typ: 'C', length: 10},
	}
	records := [][]string{{"Alice"}, {"Bob"}, {"Carol"}}
	deleted := []bool{false, true, false}
	data := buildDBF(t, fields, records, deleted)

	// With ignoreDeleted=true, only 2 records should be returned.
	r, err := NewReader(bytes.NewReader(data), "cp850")
	require.NoError(t, err)
	r.IgnoreDeleted = true

	names := []string{}
	for {
		rec, err := r.Next()
		require.NoError(t, err)
		if rec == nil {
			break
		}
		names = append(names, rec.Values["NOME"].(string))
	}
	assert.Equal(t, []string{"Alice", "Carol"}, names)

	// With ignoreDeleted=false, all 3 records should be returned.
	r2, _ := NewReader(bytes.NewReader(data), "cp850")
	r2.IgnoreDeleted = false
	count := 0
	for {
		rec, err := r2.Next()
		require.NoError(t, err)
		if rec == nil {
			break
		}
		count++
	}
	assert.Equal(t, 3, count)
}

func TestReader_CP850Encoding(t *testing.T) {
	fields := []fieldDef{
		{name: "NOME", typ: 'C', length: 10},
	}
	// In CP850: 'ç' = 0x87, 'ã' = 0xC6
	records := [][]string{{string([]byte{'a', 'c', 0x87, 0xC6, 'o'})}}
	data := buildDBF(t, fields, records, nil)

	r, err := NewReader(bytes.NewReader(data), "cp850")
	require.NoError(t, err)

	rec, err := r.Next()
	require.NoError(t, err)
	assert.Equal(t, "acção", rec.Values["NOME"])
}

// Some dBase variants leave one or more padding bytes between the 0x0D header
// terminator and the first record (headerLen reflects the padded offset). The
// reader MUST read fields until 0x0D and seek to headerLen, not assume the two
// coincide.
func TestReader_HeaderWithTrailingPadding(t *testing.T) {
	fields := []fieldDef{{name: "ID", typ: 'N', length: 3}}
	base := buildDBF(t, fields, [][]string{{"  7"}}, nil)

	// Inject a single padding byte between 0x0D and the first record, and
	// bump headerLen by 1 to match. Original header layout: 32 header + 32 field + 1 term.
	headerLen := 32 + 32 + 1
	padded := make([]byte, 0, len(base)+1)
	padded = append(padded, base[:headerLen]...) // up to and including 0x0D
	padded = append(padded, 0x00)                // extra pad
	padded = append(padded, base[headerLen:]...) // records + EOF

	// Fix the declared header length in bytes 8-9 to include the padding byte.
	padded[8] = byte((headerLen + 1) & 0xFF)
	padded[9] = byte((headerLen + 1) >> 8)

	r, err := NewReader(bytes.NewReader(padded), "cp850")
	require.NoError(t, err)

	rec, err := r.Next()
	require.NoError(t, err)
	require.NotNil(t, rec)
	assert.InDelta(t, 7.0, rec.Values["ID"], 0.0001)
}

func TestReader_TrimsTextFields(t *testing.T) {
	fields := []fieldDef{
		{name: "COD", typ: 'C', length: 10},
	}
	records := [][]string{{"ABC"}} // will be padded with 7 spaces
	data := buildDBF(t, fields, records, nil)

	r, err := NewReader(bytes.NewReader(data), "cp850")
	require.NoError(t, err)

	rec, err := r.Next()
	require.NoError(t, err)
	assert.Equal(t, "ABC", rec.Values["COD"], "text field must be trimmed")
}

func TestReader_EncodingAuto(t *testing.T) {
	fields := []fieldDef{{name: "TXT", typ: 'C', length: 5}}
	// With language byte 0x02, auto must pick cp850. In CP850 the byte 0x87 maps
	// to "ç" — a glyph that differs from windows-1252's "‡" for the same byte,
	// so the decoded output uniquely confirms the codepage selection.
	records := [][]string{{string([]byte{0x87, ' ', ' ', ' ', ' '})}}
	data := buildDBF(t, fields, records, nil)
	data[29] = 0x02 // CP850 language driver

	r, err := NewReader(bytes.NewReader(data), "auto")
	require.NoError(t, err)
	rec, err := r.Next()
	require.NoError(t, err)
	assert.Equal(t, "ç", rec.Values["TXT"], "cp850 decoding: 0x87 → ç")
}

func TestDetectEncoding(t *testing.T) {
	assert.Equal(t, "cp850", detectEncoding(0x02))
	assert.Equal(t, "windows-1252", detectEncoding(0x03))
	assert.Equal(t, "windows-1252", detectEncoding(0x57))
	assert.Equal(t, "cp850", detectEncoding(0x00), "absent language driver → cp850 (legacy Clipper/dBase default)")
}

func TestReader_EncodingUTF8Passthrough(t *testing.T) {
	fields := []fieldDef{{name: "TXT", typ: 'C', length: 6}}
	records := [][]string{{"João"}}
	data := buildDBF(t, fields, records, nil)

	r, err := NewReader(bytes.NewReader(data), "utf-8")
	require.NoError(t, err)
	rec, err := r.Next()
	require.NoError(t, err)
	assert.Equal(t, "João", rec.Values["TXT"])
}
