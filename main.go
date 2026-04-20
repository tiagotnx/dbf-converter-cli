package main

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

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

	var stats converter.Stats
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
		ProgressIsTTY: isTerminal(os.Stderr),
		InputPath:     opts.Input,
		Stats:         &stats,
	}

	if opts.Schema {
		schemaPath := opts.SchemaOut
		if schemaPath == "" {
			schemaPath = cli.SchemaPath(opts.Input)
		}
		schemaFile, err := os.Create(schemaPath)
		if err != nil {
			return fmt.Errorf("creating schema file: %w", err)
		}
		defer schemaFile.Close()
		cfg.SchemaOut = schemaFile
	}

	if err := converter.Convert(cfg); err != nil {
		return err
	}

	if shouldShowSummary(opts, os.Stderr) {
		printSummary(os.Stderr, opts.Output, stats)
	}
	return nil
}

// shouldShowSummary gates the completion line so it only appears when the user
// is likely to want it: interactive stderr, or explicit --progress / --verbose.
// Silent pipes stay silent (2>/dev/null users and CI parents).
func shouldShowSummary(opts *cli.Options, stderr *os.File) bool {
	if opts.Progress || opts.Verbose {
		return true
	}
	return isTerminal(stderr)
}

// printSummary writes a single "✓ N/M records → out.csv (2.1 MB) in 3.4s @ 1.7k rec/s"
// line to stderr. File size is stat()'d when the output is a regular path; it's
// omitted for stdout to avoid guessing.
func printSummary(w io.Writer, outPath string, s converter.Stats) {
	dst := outPath
	size := ""
	if outPath == "-" {
		dst = "<stdout>"
	} else if fi, err := os.Stat(outPath); err == nil {
		size = fmt.Sprintf(" (%s)", humanBytes(fi.Size()))
	}

	rate := ""
	if s.Elapsed > 0 {
		r := float64(s.Emitted) / s.Elapsed.Seconds()
		if r >= 1000 {
			rate = fmt.Sprintf(" @ %.1fk rec/s", r/1000)
		} else {
			rate = fmt.Sprintf(" @ %.0f rec/s", r)
		}
	}

	count := fmt.Sprintf("%d", s.Emitted)
	if s.Total > 0 && uint32(s.Emitted) != s.Total {
		count = fmt.Sprintf("%d/%d", s.Emitted, s.Total)
	}

	fmt.Fprintf(w, "✓ %s records → %s%s in %s%s\n", count, dst, size, formatElapsed(s.Elapsed), rate)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func formatElapsed(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	total := int(d.Seconds())
	return fmt.Sprintf("%dm%02ds", total/60, total%60)
}

// isTerminal reports whether w is a terminal (not a pipe/file redirect).
// Uses os.ModeCharDevice so we don't need golang.org/x/term as a direct dep.
func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
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
