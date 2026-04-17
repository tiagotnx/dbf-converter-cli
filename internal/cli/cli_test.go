package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFlags_Required(t *testing.T) {
	_, err := ParseFlags([]string{})
	assert.Error(t, err, "missing -i/-o should fail")
}

func TestParseFlags_Defaults(t *testing.T) {
	opts, err := ParseFlags([]string{"-i", "in.dbf", "-o", "out.csv"})
	require.NoError(t, err)
	assert.Equal(t, "in.dbf", opts.Input)
	assert.Equal(t, "out.csv", opts.Output)
	assert.Equal(t, "csv", opts.Format)
	assert.Equal(t, "cp850", opts.Encoding)
	assert.True(t, opts.IgnoreDeleted, "--ignore-deleted must default to true per spec")
	assert.False(t, opts.Schema)
	assert.Equal(t, 0, opts.Head)
	assert.Equal(t, "", opts.Where)
}

func TestParseFlags_AllOptions(t *testing.T) {
	opts, err := ParseFlags([]string{
		"-i", "in.dbf",
		"-o", "out.jsonl",
		"-f", "jsonl",
		"-e", "windows-1252",
		"--where", "STATUS == 'A'",
		"--head", "100",
		"--schema",
		"--ignore-deleted=false",
		"--table", "clientes",
	})
	require.NoError(t, err)
	assert.Equal(t, "jsonl", opts.Format)
	assert.Equal(t, "windows-1252", opts.Encoding)
	assert.Equal(t, "STATUS == 'A'", opts.Where)
	assert.Equal(t, 100, opts.Head)
	assert.True(t, opts.Schema)
	assert.False(t, opts.IgnoreDeleted)
	assert.Equal(t, "clientes", opts.TableName)
}

func TestParseFlags_InvalidFormat(t *testing.T) {
	_, err := ParseFlags([]string{"-i", "in.dbf", "-o", "out.xml", "-f", "xml"})
	assert.Error(t, err, "unknown format must be rejected")
}

func TestParseFlags_InvalidEncoding(t *testing.T) {
	_, err := ParseFlags([]string{"-i", "in.dbf", "-o", "out.csv", "-e", "utf-16"})
	assert.Error(t, err, "unsupported encoding must be rejected")
}

func TestSchemaPath(t *testing.T) {
	assert.Equal(t, "clientes_schema.json", SchemaPath("clientes.dbf"))
	assert.Equal(t, "noext_schema.json", SchemaPath("noext"))
	assert.Equal(t, "/path/to/clientes_schema.json", SchemaPath("/path/to/clientes.dbf"))
}
