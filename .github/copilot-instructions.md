# GitHub Copilot Instructions

Contexto compacto para sugestões inline no **dbf-converter-cli**. Para contexto completo (arquitetura, princípios, fluxos), leia [../CLAUDE.md](../CLAUDE.md).

## Stack
- **Go 1.21+** (testado com 1.25). Sem CGO.
- Dependências: `github.com/expr-lang/expr`, `github.com/spf13/cobra` (+ `pflag` transitivo), `github.com/stretchr/testify`, `golang.org/x/text`, `github.com/parquet-go/parquet-go`.

## O que é
Conversor CLI de arquivos DBF (dBase) legados (CP850 / Windows-1252 / ISO-8859-1 / UTF-8) para CSV / JSONL / SQL / Parquet em UTF-8, com saída **AI-Ready** e processamento **streaming** (nunca carrega tudo em RAM).

## Princípios inegociáveis (não sugira código que os viole)

1. **Streaming.** Pipeline `reader → filter → exporter` processa um registro por vez. Sem `[]Record`. Sem `io.ReadAll` em DBF.
2. **TDD Red-Green-Refactor.** Toda nova funcionalidade começa pelo teste no `*_test.go` correspondente.
3. **AI-Ready output:**
   - Texto: `strings.TrimSpace` + decode para UTF-8.
   - Texto binário em campos `C` (bytes `< 0x20` fora de `\t\r\n`) → hex lossless via `encoding/hex`, sem o padding `0x00`/espaço.
   - Numérico vazio/inválido → `nil` (não `0`, não `""`).
   - Data válida → `"YYYY-MM-DD"`; vazia/inválida → `nil`.
   - Lógico indeterminado (`?`) → `nil`.
4. **Errors, not panics.** `fmt.Errorf("contexto: %w", err)`. Nunca `panic`.
5. **Sem dependências novas** sem justificativa no PR.

## Convenções

### Nomeação
- Testes: `TestTipo_Comportamento` (ex: `TestReader_DateNormalization`).
- Arquivos de teste: colocated, `foo_test.go` ao lado de `foo.go`.
- Pacotes: lowercase, 1 palavra (`dbf`, `filter`, `exporter`, `converter`, `cli`).

### Estilo
- `gofmt` obrigatório; `go vet` limpo.
- Exportado = documentado com `//`.
- Comentários só para *porquê*, nunca para *o quê*.
- Table-driven tests para > 2 casos. Use `testify/require` para setup, `testify/assert` para assertivas.

### Erros
- Mensagens lowercase, incluem valor ofensivo: `fmt.Errorf("unsupported --format %q", cfg.Format)`.
- Validação na borda (CLI / construtores de exporter), não no meio do loop hot.

## Arquitetura e regras de camadas

```
main.go              → abre arquivos, slog, progresso; chama converter
internal/cli/        → Cobra root + `version`/`completion` subcomandos; argv → Options DTO
internal/converter/  → orquestra read→filter→export; projeta --fields; emite progresso
internal/dbf/        → parser DBF (base, sem imports internos) + auto-detect de encoding
internal/filter/     → wrapper expr-lang (compile-once)
internal/exporter/   → CSV / JSONL / SQL / Parquet (interface Exporter)
pkg/dbf/             → API pública (type alias para internal/dbf)
pkg/converter/       → API pública (type alias para internal/converter)
```

- `dbf` e `filter` **não** importam nada interno.
- `exporter` **não** importa `dbf`; usa `exporter.Field` (DTO próprio).
- `converter` faz o mapeamento `dbf.Field → exporter.Field`.
- Só `main.go` toca o filesystem.

## Flags principais (referência rápida)

- `-i`/`-o` obrigatórios. Use `-` para stdin/stdout.
- `-f csv|jsonl|sql|parquet` (default `csv`).
- `-e auto|cp850|windows-1252|iso-8859-1|utf-8` (default `auto`: detecta do byte 29 do header).
- `--dialect generic|postgres|mysql|sqlite` (só com `-f sql`).
- `--fields A,B,C` projeta colunas (preserva a ordem do usuário).
- `--where "<expr>"` filtro expr-lang compilado uma vez.
- `--head N` (conta pós-filtro).
- `--schema` grava `[input]_schema.json` ao lado do input.
- `--schema-out <path>` caminho explícito (implica `--schema`; útil para não sujar `testdata/`).
- `--progress` barra em stderr (≤1 tick/s).
- `--verbose` ativa `slog.Debug`.
- Subcomandos: `version` (detalhado), `completion bash|zsh|fish|powershell`.

## Exporter: interface única

```go
type Exporter interface {
    Write(row map[string]interface{}) error
    Close() error
}
```

Para adicionar um novo formato, implemente essa interface e registre em `converter.buildExporter`.

## Parsing de DBF: valores retornados

Reader retorna `map[string]interface{}` onde valores são exclusivamente:
- `string` (campos C/M, já trimmed e em UTF-8; hex lowercase se o conteúdo útil tiver bytes de controle)
- `float64` (campos N/F/I; `nil` se vazio)
- `bool` (campos L; `nil` se indeterminado)
- `string` ISO-8601 `"YYYY-MM-DD"` (campos D; `nil` se vazio/inválido)
- `nil`

## Filtros (`--where`)

Expressão [expr-lang](https://expr-lang.org/docs/language-definition), compilada uma vez em `filter.New`, executada por registro. Resultado **deve** ser `bool` — qualquer outro tipo é erro.

## Commits

Conventional Commits em **inglês imperativo**:
- `feat(dbf): support memo fields`
- `fix(exporter): escape backslashes in SQL literals`
- `test(filter): add table-driven null cases`
- `docs: update encoding table`

Co-autoria obrigatória em commits gerados por IA:
```
Co-Authored-By: GitHub Copilot <noreply@github.com>
```

## Anti-patterns (não sugira)

- ❌ Carregar todos os registros antes de escrever.
- ❌ Usar `log.Fatal` ou `panic` no código de biblioteca.
- ❌ `fmt.Println` no caminho quente — use `slog` se precisar.
- ❌ Versionar `.dbf` binário em `testdata/`. Use `testdata/gen_fixture.go` para gerar sintéticos.
- ❌ Introduzir `init()` com efeitos colaterais.
- ❌ Concatenar SQL manualmente sem validar identificador (veja `validIdent` em `exporter/sql.go`).

## Testes em memória

Fixtures DBF são construídos em memória no helper `buildDBF(t, fields, records, deleted)` dentro dos `_test.go`. Nunca dependa de arquivos em disco nos testes automatizados — smoke tests manuais com arquivos reais vão separados.

## Comandos úteis

```bash
go test ./...                                          # suíte
go test ./... -v -count=1 -race                        # pré-PR
go build -o dbf-converter .                            # binário local
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" .    # release
go run testdata/gen_fixture.go testdata/sample.dbf     # fixture sintético
```
