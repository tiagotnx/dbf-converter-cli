package converter

import (
	"bytes"
	"io"
	"testing"
)

// benchDBF assembles a synthetic DBF with `rows` records and a schema
// representative of real workloads (mixed C/N/D/L columns).
func benchDBF(b *testing.B, rows int) []byte {
	b.Helper()
	fields := []fieldDef{
		{name: "ID", typ: 'N', length: 8},
		{name: "NOME", typ: 'C', length: 40},
		{name: "VALOR", typ: 'N', length: 12, decimal: 2},
		{name: "STATUS", typ: 'C', length: 1},
		{name: "ATIVO", typ: 'L', length: 1},
		{name: "DATA", typ: 'D', length: 8},
	}
	records := make([][]string, rows)
	for i := range records {
		records[i] = []string{
			"       1",
			"Cliente de exemplo                      ",
			"      123.45",
			"A",
			"T",
			"20250116",
		}
	}
	// Reuse the test helper defined in converter_test.go (same package).
	return buildDBFB(fields, records)
}

// buildDBFB is the benchmark-facing build helper; it mirrors buildDBF from
// converter_test.go but doesn't need *testing.T.
func buildDBFB(fields []fieldDef, records [][]string) []byte {
	recordLength := 1
	for _, f := range fields {
		recordLength += int(f.length)
	}
	headerLength := 32 + 32*len(fields) + 1

	var buf bytes.Buffer
	header := make([]byte, 32)
	header[0] = 0x03
	header[4] = byte(len(records))
	header[5] = byte(len(records) >> 8)
	header[6] = byte(len(records) >> 16)
	header[7] = byte(len(records) >> 24)
	header[8] = byte(headerLength)
	header[9] = byte(headerLength >> 8)
	header[10] = byte(recordLength)
	header[11] = byte(recordLength >> 8)
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

	for _, rec := range records {
		buf.WriteByte(' ')
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

func BenchmarkConvert_CSV_10k(b *testing.B) {
	data := benchDBF(b, 10_000)
	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		if err := Convert(Config{
			Input:    bytes.NewReader(data),
			Output:   io.Discard,
			Format:   "csv",
			Encoding: "cp850",
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConvert_JSONL_10k(b *testing.B) {
	data := benchDBF(b, 10_000)
	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		if err := Convert(Config{
			Input:    bytes.NewReader(data),
			Output:   io.Discard,
			Format:   "jsonl",
			Encoding: "cp850",
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConvert_CSV_Filtered_10k(b *testing.B) {
	data := benchDBF(b, 10_000)
	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		if err := Convert(Config{
			Input:    bytes.NewReader(data),
			Output:   io.Discard,
			Format:   "csv",
			Encoding: "cp850",
			Where:    "STATUS == 'A' && VALOR > 100",
		}); err != nil {
			b.Fatal(err)
		}
	}
}
