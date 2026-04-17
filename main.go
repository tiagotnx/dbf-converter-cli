package main

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"

	"dbf-converter-cli/internal/cli"
	"dbf-converter-cli/internal/converter"
)

// ldflags-injected at release time by goreleaser.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	cmd := cli.NewRootCommand(
		cli.BuildInfo{Version: version, Commit: commit, Date: date},
		runConvert,
	)
	if err := cmd.Execute(); err != nil {
		// Cobra already printed the error via SilenceErrors=false.
		os.Exit(1)
	}
}

func runConvert(opts *cli.Options) error {
	configureLogger(opts.Verbose)

	in, closeIn, err := openInput(opts.Input)
	if err != nil {
		return fmt.Errorf("opening input: %w", err)
	}
	defer closeIn()

	out, closeOut, err := openOutput(opts.Output)
	if err != nil {
		return fmt.Errorf("creating output: %w", err)
	}
	defer closeOut()

	bw := bufio.NewWriter(out)
	defer bw.Flush()

	cfg := converter.Config{
		Input:         in,
		Output:        bw,
		Format:        opts.Format,
		Encoding:      opts.Encoding,
		Where:         opts.Where,
		Head:          opts.Head,
		IgnoreDeleted: opts.IgnoreDeleted,
		TableName:     opts.TableName,
		Dialect:       opts.Dialect,
		Fields:        opts.Fields,
		Progress:      opts.Progress,
		ProgressOut:   os.Stderr,
		InputPath:     opts.Input,
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

// openInput resolves the path to an io.Reader. A bare "-" means stdin.
// Regular paths are wrapped in a bufio.Reader so DBF record reads don't
// trigger one syscall per field slice.
func openInput(path string) (io.Reader, func(), error) {
	if path == "-" {
		return bufio.NewReader(os.Stdin), func() {}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return bufio.NewReader(f), func() { f.Close() }, nil
}

// openOutput mirrors openInput for writes. Stdout stays unbuffered here
// because the caller wraps the returned writer in bufio.NewWriter anyway.
func openOutput(path string) (io.Writer, func(), error) {
	if path == "-" {
		return os.Stdout, func() {}, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { f.Close() }, nil
}

// configureLogger installs a slog handler on stderr. Debug level is only
// enabled with --verbose so that the default run stays quiet.
func configureLogger(verbose bool) {
	level := slog.LevelWarn
	if verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})))
}
