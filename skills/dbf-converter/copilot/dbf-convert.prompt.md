---
mode: agent
description: "Convert, inspect or filter a .dbf file using dbf-converter with sensible defaults and sanity checks."
---

# /dbf-convert

Help the user convert a legacy `.dbf` file into an AI-ready format (CSV / JSONL / SQL) using the `dbf-converter` CLI.

## Arguments expected from the user

Ask only for what's missing:

1. **Input path** (`-i`) — required.
2. **Goal** — one of: "explore", "analysis", "LLM input", "DB ingestion", "data lake". Infer format from goal:
   - explore → `jsonl` with `--head 100 --schema`
   - analysis → `csv`
   - LLM input → `jsonl --head 500 --schema`
   - DB ingestion → `sql` (ask for table name and target DB to pick `--dialect postgres|mysql|sqlite`)
   - data lake / Spark / DuckDB ingestion → `parquet`
3. **Encoding** — leave as `auto` (the default reads the DBF language-driver byte). Only prompt for an explicit encoding if a first-pass run shows garbled accents.
4. **Filter** — ask if the user mentioned criteria ("only active", "value above X", "this date range").
5. **Schema path** — if the input lives in a read-only or versioned directory, use `--schema-out <explicit-path>` instead of `--schema` to avoid polluting the source tree.

## Steps

1. Verify `dbf-converter` is on PATH. If not, propose installation from GitHub Releases.
2. Run a 1-row preview with `--schema` to surface the field list:
   ```bash
   dbf-converter -i <input> -o /tmp/dbf_preview.jsonl -f jsonl --head 1 --schema
   ```
3. Show the schema field list and the preview row to the user. Confirm field names before writing the real command (DBF headers are case-sensitive, usually uppercase, often cryptic like `LAN_ES05`).
4. Build the final command using the goal-inferred format and any filter.
5. Execute and report:
   - Record count in output
   - First 3 rows (sanity check on encoding/trim)
   - Any warnings (garbled accents → suggest alternate encoding)

## Guardrails

- Never suggest `cat` / `head` on the raw `.dbf` — it's binary.
- Never invent field names. Always inspect schema first.
- For numeric filters, compare against numbers (`VALOR >= 150`), not strings.
- For null-safe filters, guard with `FIELD != nil && ...`.
- Warn if the input file is > 100 MB and offer `--head` for a sampling-first run.

## Example complete invocations

```bash
# Explore unknown file (auto-detect encoding)
dbf-converter -i clientes.dbf -o /tmp/sample.jsonl -f jsonl --head 100 --schema

# Full export with filter
dbf-converter -i vendas.dbf -o vendas.csv \
  --where "STATUS == 'A' && VALOR >= 1000"

# Pipe straight to PostgreSQL with native types
dbf-converter -i mov.dbf -o - -f sql --dialect postgres --table movimento \
  | psql "$DATABASE_URL"

# Land a large file into a data lake
dbf-converter -i vendas.dbf -o vendas.parquet -f parquet

# Inspect a DBF in a read-only data/ dir without polluting it
dbf-converter -i data/clientes.dbf -o /tmp/preview.jsonl -f jsonl --head 1 \
  --schema-out /tmp/clientes.schema.json

# Force a non-auto encoding
dbf-converter -i latin1_data.dbf -o data.jsonl -f jsonl -e iso-8859-1
```

Reference: https://github.com/tiagotnx/dbf-converter-cli
