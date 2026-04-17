// Package dbf is the public, stable API surface for consuming the DBF
// streaming reader as a library from outside this repository.
//
// Example:
//
//	f, _ := os.Open("clientes.dbf")
//	defer f.Close()
//	r, err := dbf.NewReader(f, "auto")
//	if err != nil { log.Fatal(err) }
//	for {
//	    rec, err := r.Next()
//	    if err != nil { log.Fatal(err) }
//	    if rec == nil { break }
//	    fmt.Println(rec.Values)
//	}
//
// Everything exported here is a thin type alias over the internal
// implementation so library consumers are insulated from refactors inside
// `internal/dbf`.
package dbf

import (
	"io"

	internaldbf "dbf-converter-cli/internal/dbf"
)

// Reader streams decoded records out of a DBF file.
type Reader = internaldbf.Reader

// Field describes a single column declared in the DBF header.
type Field = internaldbf.Field

// Record is one decoded row. Values are keyed by field name and may be
// string, float64, bool, or nil (per the AI-ready normalization rules).
type Record = internaldbf.Record

// NewReader parses the header and prepares the stream for iteration. Pass
// "auto" as encodingName to derive the codepage from header byte 29.
func NewReader(src io.Reader, encodingName string) (*Reader, error) {
	return internaldbf.NewReader(src, encodingName)
}
