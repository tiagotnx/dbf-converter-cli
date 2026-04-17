// Package converter is the public entry point for driving the full
// DBF → CSV/JSONL/SQL streaming pipeline from other Go programs.
//
// Example:
//
//	in, _ := os.Open("clientes.dbf")
//	defer in.Close()
//	out, _ := os.Create("clientes.csv")
//	defer out.Close()
//	err := converter.Convert(converter.Config{
//	    Input: in, Output: out,
//	    Format: "csv", Encoding: "auto",
//	})
//
// The Config type and Convert function are aliases over `internal/converter`,
// so semantics match the CLI exactly.
package converter

import internalconv "dbf-converter-cli/internal/converter"

// Config is the input DTO for Convert. See the CLI flags for field semantics.
type Config = internalconv.Config

// Convert runs the full streaming pipeline: reader → filter → exporter.
func Convert(cfg Config) error { return internalconv.Convert(cfg) }
