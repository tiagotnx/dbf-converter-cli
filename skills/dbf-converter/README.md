# `dbf-converter` agent skill

Drop-in skill package that teaches **Claude Code** and **GitHub Copilot** how to use the [`dbf-converter` CLI](https://github.com/tiagotnx/dbf-converter-cli) to work with legacy dBase (`.dbf`) files in any project.

Install once into your project and the agent will:

- Detect `.dbf` files and know what to do with them
- Inspect the schema before making assumptions about field names
- Pick the right output format (CSV / JSONL / SQL) based on your goal
- Default to the correct encoding for Brazilian Clipper/FoxBase data (CP850)
- Build `--where` filters using the correct expression syntax and null handling
- Diagnose common problems (garbled accents, empty numerics, deleted records)

---

## Which files to copy

This package contains artifacts for two agent ecosystems. Copy the ones that match your tooling — they don't conflict with each other, so you can install both.

```
skills/dbf-converter/
├── README.md                                  (this file, not copied)
├── claude/
│   └── dbf-converter/
│       └── SKILL.md                           → .claude/skills/dbf-converter/SKILL.md
└── copilot/
    ├── dbf-converter.instructions.md          → .github/instructions/dbf-converter.instructions.md
    └── dbf-convert.prompt.md                  → .github/prompts/dbf-convert.prompt.md
```

---

## Install for Claude Code

From your target project's root:

```bash
mkdir -p .claude/skills/dbf-converter
curl -sSL -o .claude/skills/dbf-converter/SKILL.md \
  https://raw.githubusercontent.com/tiagotnx/dbf-converter-cli/main/skills/dbf-converter/claude/dbf-converter/SKILL.md
```

The skill activates automatically when Claude detects the user is working with `.dbf` files (the `description` field in the SKILL.md frontmatter is what Claude matches against user intent).

**Verify it's picked up:**
```bash
# Inside Claude Code in that project:
> What skills are available?
```
You should see `dbf-converter` listed.

---

## Install for GitHub Copilot

From your target project's root:

```bash
mkdir -p .github/instructions .github/prompts
curl -sSL -o .github/instructions/dbf-converter.instructions.md \
  https://raw.githubusercontent.com/tiagotnx/dbf-converter-cli/main/skills/dbf-converter/copilot/dbf-converter.instructions.md
curl -sSL -o .github/prompts/dbf-convert.prompt.md \
  https://raw.githubusercontent.com/tiagotnx/dbf-converter-cli/main/skills/dbf-converter/copilot/dbf-convert.prompt.md
```

**How each file activates:**

- `.github/instructions/dbf-converter.instructions.md` has `applyTo: "**/*.dbf,**/dbf/**,..."` — Copilot includes it automatically in the context when editing matching files.
- `.github/prompts/dbf-convert.prompt.md` is an invocable prompt. Run it from Copilot Chat with `/dbf-convert` and the prompt's step-by-step workflow kicks in.

> Requires **VS Code 1.91+** with Copilot Chat. The `.instructions.md` / `.prompt.md` format is a 2024 feature — older Copilot versions won't read it.

---

## Install the CLI itself

The skill assumes `dbf-converter` is reachable on `PATH`. The agent will prompt the user to install it if missing, but you can pre-install to avoid that step.

### Pre-built binary (recommended)
```bash
# Linux / macOS
curl -sSL -o /usr/local/bin/dbf-converter \
  https://github.com/tiagotnx/dbf-converter-cli/releases/latest/download/dbf-converter-linux-amd64
chmod +x /usr/local/bin/dbf-converter
```

```powershell
# Windows — save to a folder on PATH
Invoke-WebRequest -Uri https://github.com/tiagotnx/dbf-converter-cli/releases/latest/download/dbf-converter-windows-amd64.exe `
  -OutFile $env:USERPROFILE\bin\dbf-converter.exe
```

### Via Go toolchain
```bash
go install github.com/tiagotnx/dbf-converter-cli@latest
```

---

## What the skill covers

Both variants agree on the same core knowledge:

- **Flag surface** — `-i`, `-o`, `-f`, `-e`, `--where`, `--head`, `--schema`, `--schema-out`, `--ignore-deleted`, `--table`, `--dialect`, `--fields`, `--progress` (TTY bar + ETA, plain in CI/pipes), `--verbose`, `--version`; plus `preview` / `version` / `completion` subcommands
- **First-contact command** — `dbf-converter preview <file>` as the canonical "show me what's in here" shortcut (JSONL to stdout + schema next to input)
- **Completion summary** — `✓ N/M records → out.csv (size) in Xs @ rate` line printed to stderr at end of run (interactive or `--progress`/`--verbose`)
- **Format decision tree** — CSV for spreadsheets/DuckDB, JSONL for LLMs/pipelines, SQL (with a dialect) for DB ingestion, Parquet for data lakes
- **Encoding defaults** — `auto` (reads the DBF language-driver byte) with CP850 / Windows-1252 / ISO-8859-1 / UTF-8 as explicit overrides
- **Filter language** — expr-lang syntax, null guards (`FIELD != nil && ...`), string helpers
- **Output semantics** — trimmed text, float64 numerics, ISO-8601 dates, explicit nulls, skipped deletes, lossless hex fallback for binary payloads in `C` fields
- **Canonical workflows** — "explore this DBF", "clean export", "load into Postgres", "sample for an LLM", "land in a data lake"
- **Troubleshooting** — garbled accents (encoding), filter-matches-nothing (case), corrupt headers (padding), reserved words (SQL), binary-looking columns (hex auto-fallback)

The Claude SKILL.md is the deeper reference; the Copilot `.instructions.md` is a compressed version tuned for inline completions, and the `.prompt.md` is the step-by-step interactive variant.

---

## Keeping the skill up to date

Pin to a release tag instead of `main` if you want reproducible installs:

```bash
curl -sSL -o .claude/skills/dbf-converter/SKILL.md \
  https://raw.githubusercontent.com/tiagotnx/dbf-converter-cli/v0.1.0/skills/dbf-converter/claude/dbf-converter/SKILL.md
```

Updates ship with new `dbf-converter` releases; watch the repo for changelog entries tagged `skill`.

---

## Contributing

Improvements to the skill text are welcome. Open a PR on [tiagotnx/dbf-converter-cli](https://github.com/tiagotnx/dbf-converter-cli) modifying `skills/dbf-converter/**`. Keep the two variants in sync — the Copilot files are intentionally shorter, but the underlying claims (flag defaults, data semantics, filter rules) must not drift between them.
