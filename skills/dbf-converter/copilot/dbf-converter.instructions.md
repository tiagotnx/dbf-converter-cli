---
applyTo: "**/*.dbf,**/dbf/**,**/*.prg,**/*.dbc"
description: "Guidance for working with legacy dBase .dbf files via the dbf-converter CLI."
---

# Working with DBF files — Copilot instructions

This project contains legacy **dBase (.dbf)** files. Use the `dbf-converter` CLI ([github.com/tiagotnx/dbf-converter-cli](https://github.com/tiagotnx/dbf-converter-cli)) for any read, convert, filter, or migration task.

## The binary

Check availability before suggesting a command:

```bash
command -v dbf-converter || echo "install from https://github.com/tiagotnx/dbf-converter-cli/releases/latest"
```

## Flags at a glance

- `-i` / `--input` — **required**, path to `.dbf` (use `-` for stdin)
- `-o` / `--output` — **required**, path to output (use `-` for stdout)
- `-f` / `--format` — `csv` (default) / `jsonl` / `sql` / `parquet`
- `-e` / `--encoding` — `auto` (default, reads header[29]) / `cp850` / `windows-1252` / `iso-8859-1` / `utf-8`
- `--where` — [expr-lang](https://expr-lang.org) filter, compiled once and run per record
- `--head N` — limit emitted records to N (counts **after** filter)
- `--schema` — also emit `[input_basename]_schema.json` alongside input
- `--schema-out <path>` — explicit schema path (implies `--schema`; keeps the input directory clean)
- `--ignore-deleted` — default `true`; set `=false` for forensic/audit use
- `--table` — table name used by `--format sql` (default `data`)
- `--dialect` — `generic` (default) / `postgres` / `mysql` / `sqlite`; only meaningful with `-f sql`
- `--fields A,B,C` — project a subset of columns in the order given
- `--progress` — one-line stderr progress, at most one tick per second
- `--verbose` — enable `log/slog` debug output on stderr
- `--version` / `-v` — print version and exit
- Subcommands: `version`, `completion bash|zsh|fish|powershell`

## Decision hints when auto-completing commands

- If the user didn't specify `-e`, leave the default `auto` — it reads the DBF language-driver byte and picks CP850 for most Brazilian Clipper data. Only suggest an explicit `-e` if accents look garbled after a first run.
- For LLM-bound or `jq`-piped data, prefer `-f jsonl`.
- For Excel / DuckDB / pandas, prefer `-f csv`.
- For DB ingestion, prefer `-f sql` with an explicit `--table` and `--dialect` matching the target database.
- For data lakes / long-term storage / Spark pipelines, prefer `-f parquet`.
- On first contact with an unknown DBF, suggest `--head 1 --schema` to inspect before full conversion.
- When writing to a shared, read-only, or versioned directory (`testdata/`, `data/`), suggest `--schema-out <path>` instead of `--schema`.

## Data semantics after conversion

Output is **AI-ready**:

- Text: trimmed, UTF-8.
- Text with raw control bytes in a `C` field (hashes, packed blobs) → **lowercase hex**, padding stripped — never mojibake.
- Numeric: `float64` in JSONL/SQL/Parquet; empty → `null`.
- Date: `"YYYY-MM-DD"`; empty/malformed → `null`. (Parquet keeps dates as ISO strings, not INT32(DATE).)
- Logical: `true` / `false`; indeterminate → `null`.
- Deleted records skipped by default.

## Filter language — common patterns

```
STATUS == 'A'
VALOR >= 150 && VALOR <= 500
STATUS in ['A','B','C']
DATA >= '2024-01-01'
EMAIL != nil && startsWith(EMAIL, 'admin@')
NOME != nil && contains(NOME, 'LTDA')
```

Numeric fields are `float64` — compare with numbers, not strings. Empty cells are `nil`.

## When suggesting code that processes the CSV/JSONL/SQL/Parquet output

- CSV has an empty cell for nulls (no literal `null`/`NULL`).
- JSONL keys preserve DBF header order (not alphabetical).
- SQL output includes `CREATE TABLE IF NOT EXISTS` — safe to run repeatedly. Pick `--dialect` to match the target (`postgres` → `DOUBLE PRECISION/BOOLEAN/DATE`; `mysql` → `DOUBLE/TINYINT(1)/DATE`; `sqlite` → `REAL/INTEGER/TEXT`; default `generic` → `NUMERIC/BOOLEAN/DATE`).
- Parquet is columnar — consumers address columns by name; the physical column order in the file is not guaranteed to match the DBF order.

## Never suggest

- `cat file.dbf` / `head file.dbf` (binary noise).
- UTF-8 parsing of raw `.dbf` bytes. The file is CP850/CP1252, and record layout is fixed-width binary — must go through the CLI.
- Per-row `dbf-converter` invocations inside shell loops. One invocation streams the whole file.
- `--where` expressions that return non-boolean (e.g. `--where "VALOR"` will fail at runtime).

## Project repo layout conventions to respect

- Keep source `.dbf` files in `data/` or `dbf/` and git-ignore them if they contain customer data.
- Put converted artifacts in `data/processed/` or similar — never commit alongside the source.
- Schema files (`*_schema.json`) are safe to commit if the field names aren't sensitive; useful for code review.
