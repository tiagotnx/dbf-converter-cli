package exporter

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// validIdent guards the --table name against SQL injection. We only accept
// ASCII identifiers because DBF field names themselves are constrained the same way.
var validIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// SQLExporter emits a CREATE TABLE IF NOT EXISTS preamble followed by one
// INSERT statement per row. Types are inferred from the DBF field type code.
type SQLExporter struct {
	w         *bufio.Writer
	fields    []Field
	tableName string
	dialect   string
	colsList  string // pre-joined "ID, NOME, VALOR" — built once
}

// NewSQL builds an SQL exporter using the "generic" dialect.
func NewSQL(w io.Writer, fields []Field, tableName string) (*SQLExporter, error) {
	return NewSQLWithDialect(w, fields, tableName, "generic")
}

// NewSQLWithDialect builds an SQL exporter whose CREATE TABLE column types
// follow the target dialect (generic, postgres, mysql, sqlite). INSERT syntax
// is portable across all four.
func NewSQLWithDialect(w io.Writer, fields []Field, tableName, dialect string) (*SQLExporter, error) {
	if !validIdent.MatchString(tableName) {
		return nil, fmt.Errorf("invalid SQL table name %q: must match [A-Za-z_][A-Za-z0-9_]*", tableName)
	}
	for _, f := range fields {
		if !validIdent.MatchString(f.Name) {
			return nil, fmt.Errorf("invalid SQL column name %q", f.Name)
		}
	}
	if dialect == "" {
		dialect = "generic"
	}

	bw := bufio.NewWriter(w)
	exp := &SQLExporter{w: bw, fields: fields, tableName: tableName, dialect: dialect}

	// CREATE TABLE preamble
	var sb strings.Builder
	sb.WriteString("CREATE TABLE IF NOT EXISTS ")
	sb.WriteString(tableName)
	sb.WriteString(" (\n")
	for i, f := range fields {
		sb.WriteString("  ")
		sb.WriteString(f.Name)
		sb.WriteByte(' ')
		sb.WriteString(sqlTypeFor(f.Type, dialect))
		if i < len(fields)-1 {
			sb.WriteByte(',')
		}
		sb.WriteByte('\n')
	}
	sb.WriteString(");\n")
	if _, err := bw.WriteString(sb.String()); err != nil {
		return nil, err
	}

	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = f.Name
	}
	exp.colsList = strings.Join(names, ", ")

	return exp, nil
}

func (e *SQLExporter) Write(row map[string]interface{}) error {
	var sb strings.Builder
	sb.WriteString("INSERT INTO ")
	sb.WriteString(e.tableName)
	sb.WriteString(" (")
	sb.WriteString(e.colsList)
	sb.WriteString(") VALUES (")
	for i, f := range e.fields {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(sqlLiteral(row[f.Name]))
	}
	sb.WriteString(");\n")
	_, err := e.w.WriteString(sb.String())
	return err
}

func (e *SQLExporter) Close() error { return e.w.Flush() }

func sqlTypeFor(t byte, dialect string) string {
	switch dialect {
	case "postgres":
		switch t {
		case 'N', 'F':
			return "DOUBLE PRECISION"
		case 'I':
			return "INTEGER"
		case 'L':
			return "BOOLEAN"
		case 'D':
			return "DATE"
		default:
			return "TEXT"
		}
	case "mysql":
		switch t {
		case 'N', 'F':
			return "DOUBLE"
		case 'I':
			return "INT"
		case 'L':
			return "TINYINT(1)"
		case 'D':
			return "DATE"
		default:
			return "TEXT"
		}
	case "sqlite":
		switch t {
		case 'N', 'F':
			return "REAL"
		case 'I':
			return "INTEGER"
		case 'L':
			return "INTEGER"
		case 'D':
			return "TEXT"
		default:
			return "TEXT"
		}
	default: // generic
		switch t {
		case 'N', 'F', 'I':
			return "NUMERIC"
		case 'L':
			return "BOOLEAN"
		case 'D':
			return "DATE"
		default:
			return "TEXT"
		}
	}
}

func sqlLiteral(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case bool:
		if x {
			return "TRUE"
		}
		return "FALSE"
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case string:
		return "'" + strings.ReplaceAll(x, "'", "''") + "'"
	default:
		return "'" + strings.ReplaceAll(fmt.Sprintf("%v", x), "'", "''") + "'"
	}
}
