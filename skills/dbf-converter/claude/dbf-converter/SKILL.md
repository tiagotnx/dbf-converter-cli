---
name: dbf-converter
description: Convert, inspect, filter and sample legacy dBase .dbf files (Clipper, FoxBase, FoxPro, CP850/Windows-1252) into AI-ready CSV, JSONL or SQL using the `dbf-converter` CLI. Use when the user asks to read, convert, analyze, migrate or sample a .dbf file, or when the project contains .dbf files the user wants to work with.
---

# dbf-converter — skill for working with legacy DBF files

This skill teaches the agent to drive the `dbf-converter` CLI end-to-end on DBF files found in a project: inspect the schema first, choose the right format for the downstream task, apply streaming filters, and debug the common failure modes of real-world Brazilian ERP/Clipper data.

## When to use this skill

Invoke whenever the user's intent involves a `.dbf` file — read it, convert it, query it, migrate it, sample it, build a pipeline from it, or diagnose problems with it. Typical triggers:

- "Convert `clientes.dbf` to CSV"
- "What's in this DBF?"
- "Migrate the orders table into PostgreSQL"
- "Give me the first 500 rows as JSONL for a prompt"
- "Why is this DBF showing corrupted accents?"

## Prerequisites — verify the binary is available

Before any conversion, check for the CLI on `PATH`:

```bash
command -v dbf-converter || command -v ./dbf-converter
```

If missing, install from GitHub Releases (recommended, ~4 MB static binary):

**Linux / macOS:**
```bash
curl -sSL -o dbf-converter https://github.com/tiagotnx/dbf-converter-cli/releases/latest/download/dbf-converter-linux-amd64
chmod +x dbf-converter
```

**Windows (PowerShell):**
```powershell
Invoke-WebRequest -Uri https://github.com/tiagotnx/dbf-converter-cli/releases/latest/download/dbf-converter-windows-amd64.exe -OutFile dbf-converter.exe
```

Or `go install github.com/tiagotnx/dbf-converter-cli@latest` if the user has Go ≥ 1.21.

## Flag reference

| Flag | Short | Default | Notes |
|---|---|---|---|
| `--input` | `-i` | required | Path to `.dbf` |
| `--output` | `-o` | required | Path to output file; use `/dev/stdout` to pipe |
| `--format` | `-f` | `csv` | `csv` / `jsonl` / `sql` |
| `--encoding` | `-e` | `cp850` | `cp850` / `windows-1252` / `iso-8859-1` |
| `--where` | — | empty | expr-lang filter expression |
| `--head` | — | `0` | Max records to emit (0 = unlimited); counts **after** filter |
| `--schema` | — | `false` | Also emit `[name]_schema.json` next to the input |
| `--ignore-deleted` | — | `true` | Skip records marked as deleted |
| `--table` | — | `data` | Table name for `--format sql` |

## Decision tree — what to run

### Step 1: Always inspect the schema first
Before any real conversion, run with `--head 1 --schema` to learn the field list and a sample row:

```bash
dbf-converter -i clientes.dbf -o /tmp/preview.jsonl -f jsonl --head 1 --schema
cat clientes_schema.json
cat /tmp/preview.jsonl
```

The `_schema.json` is the single most useful artifact to reason about the file. Read it before writing filters — field names are usually uppercase and often cryptic (`LAN_ES05`, `FORNCLI05`).

### Step 2: Choose the format by downstream use

| User wants to... | Format | Why |
|---|---|---|
| Open in Excel / pandas / DuckDB | `csv` | Universal, smallest |
| Feed an LLM / load into Elasticsearch / process with `jq` | `jsonl` | One JSON per line, typed values (numeric, bool, null) |
| Import into PostgreSQL / MySQL / SQLite | `sql` | `CREATE TABLE` + `INSERT` ready to pipe to client |

### Step 3: Choose the encoding by origin

- **Brazilian Clipper / dBase DOS** → `cp850` (default, covers most cases)
- **Visual FoxPro on Windows** → `windows-1252`
- **International Latin-1 data** → `iso-8859-1`

If accents look wrong (`�`, `§`, `°` appearing in names), try another encoding — the choice is not auto-detectable.

### Step 4: Filter early

Filters are compiled once and run per record during streaming — filtering is free. Prefer a `--where` over post-processing:

```bash
# Only active, high-value records
dbf-converter -i vendas.dbf -o vendas.jsonl -f jsonl \
  --where "STATUS == 'A' && VALOR >= 1000"

# Specific date range
dbf-converter -i mov.dbf -o mov.csv \
  --where "DATAMOV >= '2024-01-01' && DATAMOV <= '2024-12-31'"

# Non-null fields only
dbf-converter -i cad.dbf -o cad.jsonl -f jsonl \
  --where "EMAIL != nil && ATIVO == true"
```

Filter language quick reference:

- Comparisons: `==`, `!=`, `<`, `<=`, `>`, `>=`
- Logic: `&&`, `||`, `!`
- Membership: `STATUS in ['A','B']`
- String helpers: `startsWith(NOME, 'Jo')`, `contains(DESCR, 'urgente')`
- `nil` for empty/invalid cells (empty numerics, empty dates)

**Important:** numeric fields are `float64`. Compare against numbers, not strings: `VALOR >= 150` ✅, `VALOR >= '150'` ❌.

## Canonical workflows

### Workflow A — "Quick explore this DBF"

```bash
dbf-converter -i <file>.dbf -o /tmp/sample.jsonl -f jsonl --head 100 --schema
# Read the schema JSON + the sample JSONL to brief the user.
```

### Workflow B — "Clean full export for analysis"

```bash
dbf-converter -i <file>.dbf -o <file>.csv --schema
# Open in DuckDB: duckdb -c "SELECT * FROM '<file>.csv' LIMIT 10"
```

### Workflow C — "Load into PostgreSQL"

```bash
dbf-converter -i clientes.dbf -o /tmp/clientes.sql -f sql --table clientes
psql "$DATABASE_URL" < /tmp/clientes.sql
```

Or stream directly without the temp file:

```bash
dbf-converter -i clientes.dbf -o /dev/stdout -f sql --table clientes | psql "$DATABASE_URL"
```

### Workflow D — "Feed an LLM"

```bash
dbf-converter -i base.dbf -o sample.jsonl -f jsonl \
  --where "<filter that narrows to representative rows>" \
  --head 200 --schema
# Attach base_schema.json + sample.jsonl to the prompt.
```

## Output semantics (know what the data looks like)

Every row is "AI-ready" after conversion:

- **Text fields**: trimmed of right-padding, decoded to UTF-8. `"ABC       "` becomes `"ABC"`.
- **Numeric fields**: `float64`. Empty or malformed → `null` in JSONL/SQL, empty cell in CSV.
- **Dates**: `"YYYY-MM-DD"` (ISO-8601) if valid; empty or garbage (`"  /  /    "`) → `null`.
- **Logical**: `true` / `false`; indeterminate (`?`) → `null`.
- **Deleted records**: skipped silently by default. Use `--ignore-deleted=false` if forensic inspection of deletions is needed.

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `invalid dbf header: declared length N too small` | Corrupt or non-DBF file | `file <path>`; verify it's actually a DBF |
| `unsupported encoding: "X"` | Typo or unsupported codepage | Use one of `cp850`, `windows-1252`, `iso-8859-1` |
| Accents look garbled (`ç` appears as `ž`) | Wrong `--encoding` | Try the other two; `cp850` and `windows-1252` differ for `ç`, `ã`, `õ` |
| Filter compiles but matches nothing | Field name case mismatch | DBF headers are usually uppercase; `status` ≠ `STATUS` |
| Filter errors with `cannot compare nil` | Empty numeric/date cells in rows | Guard with `FIELD != nil && FIELD > 0` |
| SQL output rejected at import | Table/column collision with reserved word | Use `--table "my_clientes"`; quote column names in your DDL |

## Things to avoid

- **Don't** write shell loops that run `dbf-converter` once per row. The tool already streams; one invocation handles millions of records.
- **Don't** gzip/bzip the `.dbf` before passing — the CLI reads raw dBase format.
- **Don't** assume UTF-8 input. Brazilian DBFs are overwhelmingly CP850.
- **Don't** skip `--schema` on first contact with an unknown DBF — you'll waste effort guessing field names.

## Check-in pattern

After running a conversion, always:

1. Report record counts: `wc -l <output>` (CSV/JSONL) or grep `INSERT` count (SQL).
2. Show the first few rows so the user can sanity-check encoding and trim.
3. If `--schema` was used, surface the field count and highlight any types the user should know about (dates, logicals, long text).

## Further reference

- Project repo: https://github.com/tiagotnx/dbf-converter-cli
- expr-lang filter language: https://expr-lang.org/docs/language-definition
- Latest release: https://github.com/tiagotnx/dbf-converter-cli/releases/latest
