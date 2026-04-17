// Package exporter defines streaming writers for the supported output formats
// (CSV, JSONL, SQL). All exporters share the Exporter interface so that the
// converter orchestrator can treat them interchangeably.
package exporter

// Field is a lightweight shape of dbf.Field used to avoid a circular import.
// The converter layer maps dbf.Field → exporter.Field at the seam.
type Field struct {
	Name string
	Type byte
}

// Exporter is the streaming contract: write one row at a time, then Close.
// Implementations MUST flush buffered writers in Close so callers see a fully
// serialized payload.
type Exporter interface {
	Write(row map[string]interface{}) error
	Close() error
}
