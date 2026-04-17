package main

import (
	"bufio"
	"fmt"
	"os"

	"dbf-converter-cli/internal/cli"
	"dbf-converter-cli/internal/converter"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	opts, err := cli.ParseFlags(args)
	if err != nil {
		return err
	}

	in, err := os.Open(opts.Input)
	if err != nil {
		return fmt.Errorf("opening input: %w", err)
	}
	defer in.Close()

	out, err := os.Create(opts.Output)
	if err != nil {
		return fmt.Errorf("creating output: %w", err)
	}
	defer out.Close()

	// bufio.Writer keeps the streaming story honest: records move in, bytes flush out,
	// nothing is accumulated in RAM beyond a 4 KiB buffer.
	bw := bufio.NewWriter(out)
	defer bw.Flush()

	cfg := converter.Config{
		Input:         bufio.NewReader(in),
		Output:        bw,
		Format:        opts.Format,
		Encoding:      opts.Encoding,
		Where:         opts.Where,
		Head:          opts.Head,
		IgnoreDeleted: opts.IgnoreDeleted,
		TableName:     opts.TableName,
	}

	if opts.Schema {
		schemaFile, err := os.Create(cli.SchemaPath(opts.Input))
		if err != nil {
			return fmt.Errorf("creating schema file: %w", err)
		}
		defer schemaFile.Close()
		cfg.SchemaOut = schemaFile
	}

	return converter.Convert(cfg)
}
