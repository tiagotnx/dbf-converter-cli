package cli

import (
	"bytes"
	"path/filepath"
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
	assert.Equal(t, "auto", opts.Encoding, "auto-detect should be the default")
	assert.True(t, opts.IgnoreDeleted, "--ignore-deleted must default to true per spec")
	assert.False(t, opts.Schema)
	assert.Equal(t, 0, opts.Head)
	assert.Equal(t, "", opts.Where)
	assert.Equal(t, "generic", opts.Dialect)
	assert.Empty(t, opts.Fields)
	assert.False(t, opts.Progress)
	assert.False(t, opts.Verbose)
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
		"--dialect", "postgres",
		"--fields", "ID,NOME,VALOR",
		"--progress",
		"--verbose",
	})
	require.NoError(t, err)
	assert.Equal(t, "jsonl", opts.Format)
	assert.Equal(t, "windows-1252", opts.Encoding)
	assert.Equal(t, "STATUS == 'A'", opts.Where)
	assert.Equal(t, 100, opts.Head)
	assert.True(t, opts.Schema)
	assert.False(t, opts.IgnoreDeleted)
	assert.Equal(t, "clientes", opts.TableName)
	assert.Equal(t, "postgres", opts.Dialect)
	assert.Equal(t, []string{"ID", "NOME", "VALOR"}, opts.Fields)
	assert.True(t, opts.Progress)
	assert.True(t, opts.Verbose)
}

func TestParseFlags_StdinStdoutSentinels(t *testing.T) {
	opts, err := ParseFlags([]string{"-i", "-", "-o", "-"})
	require.NoError(t, err)
	assert.Equal(t, "-", opts.Input)
	assert.Equal(t, "-", opts.Output)
}

func TestParseFlags_InvalidFormat(t *testing.T) {
	_, err := ParseFlags([]string{"-i", "in.dbf", "-o", "out.xml", "-f", "xml"})
	assert.Error(t, err, "unknown format must be rejected")
}

func TestParseFlags_InvalidEncoding(t *testing.T) {
	_, err := ParseFlags([]string{"-i", "in.dbf", "-o", "out.csv", "-e", "utf-16"})
	assert.Error(t, err, "unsupported encoding must be rejected")
}

func TestParseFlags_InvalidDialect(t *testing.T) {
	_, err := ParseFlags([]string{"-i", "in.dbf", "-o", "out.sql", "-f", "sql", "--dialect", "oracle"})
	assert.Error(t, err, "unsupported dialect must be rejected")
}

func TestSchemaPath(t *testing.T) {
	assert.Equal(t, "clientes_schema.json", SchemaPath("clientes.dbf"))
	assert.Equal(t, "noext_schema.json", SchemaPath("noext"))
	// SchemaPath uses filepath.Join, which emits OS-native separators — on
	// Windows this is "\" not "/". Build the expected value the same way so
	// the test is portable across GOOS.
	assert.Equal(t, filepath.Join("/path/to", "clientes_schema.json"), SchemaPath("/path/to/clientes.dbf"))
	assert.Equal(t, "stdin_schema.json", SchemaPath("-"))
}

func TestParseFlags_SchemaOutExplicitPath(t *testing.T) {
	opts, err := ParseFlags([]string{
		"-i", "in.dbf", "-o", "out.csv",
		"--schema-out", "/tmp/custom_schema.json",
	})
	require.NoError(t, err)
	assert.Equal(t, "/tmp/custom_schema.json", opts.SchemaOut)
	assert.True(t, opts.Schema, "--schema-out must implicitly enable schema emission")
}

func TestParseFlags_SchemaOutOverridesDerivedPath(t *testing.T) {
	// When both --schema (derived name) and --schema-out (explicit) are set,
	// the explicit path wins: avoids accidental clobber of the derived location.
	opts, err := ParseFlags([]string{
		"-i", "testdata/arqpar.dbf", "-o", "out.csv",
		"--schema", "--schema-out", "/var/tmp/explicit.json",
	})
	require.NoError(t, err)
	assert.Equal(t, "/var/tmp/explicit.json", opts.SchemaOut)
	assert.True(t, opts.Schema)
}

func TestVersionSubcommand(t *testing.T) {
	info := BuildInfo{Version: "1.2.3", Commit: "abc", Date: "2025-01-01"}
	cmd := NewRootCommand(info, func(*Options) error { return nil })
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"version"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, stdout.String(), "1.2.3")
	assert.Contains(t, stdout.String(), "commit: abc")
	assert.Contains(t, stdout.String(), "built:  2025-01-01")
}

func TestVersionFlag(t *testing.T) {
	info := BuildInfo{Version: "9.9.9"}
	cmd := NewRootCommand(info, func(*Options) error { return nil })
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--version"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, stdout.String(), "9.9.9")
}
