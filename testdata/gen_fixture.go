//go:build ignore

// gen_fixture builds a small sample .dbf for manual smoke-testing the CLI.
// Run with: `go run testdata/gen_fixture.go testdata/sample.dbf`.
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run testdata/gen_fixture.go <out.dbf>")
		os.Exit(2)
	}

	type field struct {
		name    string
		typ     byte
		length  byte
		decimal byte
	}
	fields := []field{
		{name: "ID", typ: 'N', length: 5},
		{name: "NOME", typ: 'C', length: 20},
		{name: "STATUS", typ: 'C', length: 1},
		{name: "VALOR", typ: 'N', length: 10, decimal: 2},
		{name: "DATA", typ: 'D', length: 8},
		{name: "ATIVO", typ: 'L', length: 1},
	}

	// CP850 byte encodings: 'ç' = 0x87, 'ã' = 0xC6
	rows := [][]string{
		{"    1", "Jo" + string([]byte{0xC6}) + "o Silva         ", "A", "    150.75", "20250115", "T"},
		{"    2", "Maria " + string([]byte{0x87}) + "a Santos     ", "A", "    250.50", "20250220", "T"},
		{"    3", "Pedro Costa         ", "B", "     99.00", "        ", "F"},
		{"    4", "Ana Souza           ", "A", "    500.00", "20250310", "T"},
		{"    5", "Carlos Deleted      ", "X", "      0.00", "        ", "F"},
	}
	deleted := []bool{false, false, false, false, true}

	recLen := 1
	for _, f := range fields {
		recLen += int(f.length)
	}
	headerLen := 32 + 32*len(fields) + 1

	var buf []byte
	header := make([]byte, 32)
	header[0] = 0x03
	header[1] = 125
	header[2] = 1
	header[3] = 1
	binary.LittleEndian.PutUint32(header[4:8], uint32(len(rows)))
	binary.LittleEndian.PutUint16(header[8:10], uint16(headerLen))
	binary.LittleEndian.PutUint16(header[10:12], uint16(recLen))
	buf = append(buf, header...)

	for _, f := range fields {
		fd := make([]byte, 32)
		copy(fd[0:11], f.name)
		fd[11] = f.typ
		fd[16] = f.length
		fd[17] = f.decimal
		buf = append(buf, fd...)
	}
	buf = append(buf, 0x0D)

	for i, r := range rows {
		if deleted[i] {
			buf = append(buf, '*')
		} else {
			buf = append(buf, ' ')
		}
		for j, v := range r {
			flen := int(fields[j].length)
			pad := make([]byte, flen)
			for k := range pad {
				pad[k] = ' '
			}
			copy(pad, v)
			buf = append(buf, pad...)
		}
	}
	buf = append(buf, 0x1A)

	if err := os.WriteFile(os.Args[1], buf, 0644); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Println("wrote", os.Args[1], len(buf), "bytes")
}
