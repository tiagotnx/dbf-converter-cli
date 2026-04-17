# CLAUDE.md

Guia de contexto para agentes de IA (Claude Code, Cursor, Codeium, Copilot via `.github/copilot-instructions.md`) que vierem trabalhar neste repositório. **Leia este arquivo antes de propor mudanças.** Ele não descreve o código — descreve *como mudá-lo*.

---

## O que é este projeto

`dbf-converter-cli` é um conversor CLI em Go para arquivos DBF (dBase III / IV / FoxBase+) legados. Transforma bases brasileiras em CP850 / Windows-1252 em **saída AI-Ready** (CSV / JSONL / SQL) via **streaming linha a linha** — nenhum registro é acumulado em RAM.

Audiência primária: desenvolvedores Brasil migrando dados ERP/Clipper para pipelines modernos (data warehouses, LLMs, bancos relacionais).

---

## Princípios invioláveis

Esses princípios nasceram do design inicial. **Qualquer mudança que os viole precisa ser discutida em issue antes do PR.**

### 1. Streaming puro
O pipeline é `dbf.Reader.Next() → filter.Match() → exporter.Write()`, um registro por vez. Ciclo fechado, sem materialização em slice.

- ❌ Nunca acumule `[]Record` nem faça `io.ReadAll` em um DBF.
- ❌ Nunca leia "todos os registros que batem o filtro" antes de escrever.
- ✅ Se precisar ordenar/agregar, é responsabilidade do usuário fazer isso a jusante (DuckDB, `sort`, SQL).

### 2. TDD estrito (Red → Green → Refactor)
Toda mudança de comportamento começa com um teste que falha.

1. **RED:** escreva o teste no `_test.go` apropriado. Rode e confirme que falha com a mensagem esperada.
2. **GREEN:** escreva o mínimo de código para o teste passar. Não adicione nada além.
3. **REFACTOR:** limpe mantendo a suíte verde. Commit separado se a mudança for grande.

O bug real do `movpro01.dbf` (header com byte de padding) foi encontrado e corrigido nesse ciclo — veja `TestReader_HeaderWithTrailingPadding` como exemplo de "teste que surgiu do mundo real".

### 3. Output AI-Ready
Toda decodificação que pudesse ficar à meia-boca é responsabilidade do parser, não do consumidor:

- Texto: `TrimSpace` + decode UTF-8, sempre.
- Numérico vazio / inválido → `nil` (não `0`, não `""`).
- Data vazia / malformada (`"  /  /    "`) → `nil`.
- Data válida → `"YYYY-MM-DD"` (ISO-8601), nunca `20250115` nem `15/01/2025`.
- Lógico indeterminado (`?`) → `nil`.

### 4. Zero dependências tóxicas
Stack aprovado: `expr-lang/expr`, `spf13/cobra` (+ `pflag` transitivo), `stretchr/testify`, `golang.org/x/text`, `parquet-go/parquet-go` (para o exportador Parquet). Qualquer dependência nova precisa de justificativa no PR.

- ❌ Sem `panic` em código de produção. Sempre retorne `error`.
- ❌ Sem CGO. Build precisa continuar produzindo binário estático.
- ❌ Sem `init()` com side-effects.

---

## Arquitetura

```
main.go                    # abre arquivos + slog + progresso → converter.Convert
internal/cli/              # Cobra root command: argv → Options (valida allow-lists)
internal/converter/        # Convert: orquestra read→filter→export; projeta --fields; emite progresso
internal/dbf/              # parser DBF (header + tipos C/N/F/D/L/I/M) + auto-detect encoding
internal/filter/           # wrapper expr-lang (compile-once, run-per-row)
internal/exporter/         # CSVExporter, JSONLExporter, SQLExporter (interface Exporter) + dialetos SQL
pkg/dbf/                   # API pública (type alias para internal/dbf)
pkg/converter/             # API pública (type alias para internal/converter)
testdata/gen_fixture.go    # gerador de fixture sintético (//go:build ignore)
```

**Sinalizadores do CLI** (atualizados): `-i`/`-o` (obrigatórios, `-` = stdin/stdout), `-f` (csv|jsonl|sql|parquet), `-e` (auto padrão | cp850 | windows-1252 | iso-8859-1 | utf-8), `--where`, `--head`, `--schema`, `--ignore-deleted`, `--table`, `--dialect` (generic|postgres|mysql|sqlite), `--fields`, `--progress`, `--verbose`, `--version`. Subcomandos: `version` (detalhado) e `completion` (shell scripts).

**Regras de camadas** (estritas, checadas em PR):

- `dbf` não importa nada interno. É a base.
- `filter` não importa nada interno.
- `exporter` não importa `dbf` — usa `exporter.Field` (DTO próprio) para evitar ciclo.
- `converter` importa os três e faz o mapeamento `dbf.Field → exporter.Field` no seam.
- `cli` só produz uma struct DTO (`Options`). Não abre arquivo, não chama converter.
- `main.go` é a única camada que toca o filesystem (`os.Open`, `os.Create`).

---

## Convenções de código

### Go style
- `gofmt` obrigatório (sem exceções). `go vet ./...` limpo.
- Nomes exportados documentados com `//`, sempre.
- Erros: `fmt.Errorf("contexto: %w", err)`. Nunca `fmt.Errorf(err.Error())`.
- Testes: **table-driven quando há > 2 casos**. Use `require.NoError` para setup e `assert.*` para assertivas (testify).
- Nomes de teste: `TestTipo_Comportamento` (ex: `TestReader_DateNormalization`).
- Fixtures de DBF: construídas em memória via helper `buildDBF(t, fields, records, deleted)`. Não versione `.dbf` binários reais em `testdata/`.

### Comentários
Default: **não escreva comentários**. Nomes bons eliminam a maioria. Só comente quando:
- Há um invariante não óbvio (ex: "ordem de campos segue header do DBF, não alfabética").
- Há um workaround para um bug externo/variante de formato (ex: "headerLen pode incluir padding bytes após 0x0D").
- Há uma decisão que o leitor questionaria ("por que um parser próprio?").

Nunca comente o *que* o código faz. Nunca use comentário para marcar autoria, data, ticket — isso é responsabilidade do git.

### Mensagens de erro
- Começam com lowercase (padrão Go).
- Incluem o valor ofensivo quando útil: `fmt.Errorf("unsupported --format %q (supported: csv, jsonl, sql)", cfg.Format)`.
- Falham o mais cedo possível (validação na borda, em `cli.ParseFlags` ou no construtor do exporter).

---

## Testes

### Rodando
```bash
go test ./...                       # suíte completa (~40 testes)
go test ./internal/dbf -v -count=1  # módulo específico, sem cache
go test ./... -cover                # cobertura sumarizada
go test ./... -race                 # detecta data races (execute antes de qualquer PR que mexa em concorrência)
```

### Smoke test manual
```bash
go run testdata/gen_fixture.go testdata/sample.dbf
go build -o dbf-converter .
./dbf-converter -i testdata/sample.dbf -o /tmp/out.csv --schema
```

### O que cobrir em um novo teste
Qualquer módulo novo deve ter cobertura para:
- **Caminho feliz** (input válido, output correto).
- **Input vazio** (arquivo sem registros, filtro sem expressão, etc).
- **Input malformado** (data inválida, número vazio, campo deletado, header truncado).
- **Limite de tipo** (float overflow, string com bytes não ASCII, nome com acento).

---

## Commits e PRs

### Conventional Commits obrigatório
Formato: `<tipo>(<escopo>): <descrição imperativa em inglês>`

Tipos em uso: `feat`, `fix`, `test`, `docs`, `chore`, `refactor`, `perf`.

Escopos em uso: `dbf`, `filter`, `exporter`, `converter`, `cli`. Use `chore` sem escopo para mudanças de tooling/meta.

Exemplos bons:
```
feat(dbf): support memo fields by reading associated .dbt
fix(exporter): escape backslashes in SQL literals
test(filter): add table-driven cases for null-coalescing
refactor(converter): extract schema writer into its own function
```

### Co-autoria em commits gerados por IA
Commits criados por agente **obrigatoriamente** incluem:
```
Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```
(ou a identificação do agente usado). Isso deixa explícito no histórico o que foi mão humana e o que foi assistido.

### Pull Requests
- Um PR = uma preocupação. Não misture `feat` + `refactor` + `fmt`.
- Descreva o **porquê**, não o **o quê** (o diff já mostra o quê).
- Anexe passos para reproduzir se for fix.
- Sem PR "WIP" pedindo merge — use draft.

---

## Fluxos comuns

### Adicionar um novo formato de saída (ex: Parquet)
1. Adicione `internal/exporter/parquet_test.go` com casos de header, nulos, tipos.
2. Rode `go test ./internal/exporter/` — RED.
3. Crie `internal/exporter/parquet.go` implementando `Exporter` (`Write` / `Close`).
4. Em `internal/converter/converter.go`, adicione o case `"parquet"` em `buildExporter`.
5. Em `internal/cli/cli.go`, adicione `"parquet"` ao `supportedFormats` e às mensagens de erro.
6. Atualize README: tabela de formatos e exemplo de uso.
7. Commit único: `feat(exporter): add Parquet streaming exporter`.

### Adicionar um novo encoding
1. Mapeie em `dbf.resolveDecoder` (case-insensitive, sem hífen).
2. Adicione ao allow-list `supportedEncodings` em `cli.go`.
3. Teste em `dbf_test.go` com bytes característicos do encoding.
4. README: tabela de encodings.

### Adicionar um novo tipo de campo DBF (ex: `T` timestamp)
1. Teste em `dbf_test.go` com fixture que inclua o tipo.
2. Adicione o case em `(*Reader).decodeField`.
3. Se for de tamanho variável (memo), documente claramente por quê — hoje não suportamos blobs externos.

---

## Anti-patterns (coisas que foram recusadas/evitadas de propósito)

- **Carregar tudo em memória**: simplifica código mas inviabiliza arquivos > RAM. Rejeitado.
- **Usar `any` no lugar de tipos fortes**: só no mapa `map[string]interface{}` do contrato com `expr` (requisito externo). Todo o resto é tipado.
- **Bibliotecas gigantes de DBF**: avaliamos `go-dbase` e similares; o parser próprio é ~200 linhas, total controle sobre trim/encoding/normalização no ponto de decodificação. Não substitua sem discussão.
- **Colocar lógica de formato no converter**: o converter é um orquestrador burro. Toda decisão de serialização mora em `exporter/`.
- **Panic em vez de error**: código da lib nunca entra em pânico. Só em `main` se quiser (mas hoje não tem).
- **Versionar binários no git**: use GitHub Releases (já temos `v0.1.0`).

---

## Arquivos "importantes para saber que existem"

- `testdata/gen_fixture.go` — `//go:build ignore`. Rode com `go run testdata/gen_fixture.go <saida>.dbf` para gerar fixtures sintéticos. Nunca ship.
- `README.md` — documentação pública. Atualize junto com mudanças de UX/CLI.
- `CLAUDE.md` (este arquivo) — contexto interno. Atualize quando princípios mudarem.
- `.github/copilot-instructions.md` — variante resumida para Copilot inline.
- `.gitignore` — bloqueia `testdata/*.dbf` para evitar vazar dados reais de cliente. **Não remova.**

---

## Perguntas frequentes de agentes

**"Posso refatorar o parser DBF para usar uma biblioteca?"**
Não sem abrir issue. O parser próprio é deliberado (ver Anti-patterns).

**"Posso adicionar um comando de 'sort' no converter?"**
Não. Sort quebra o streaming. Documente no README que o usuário faça isso downstream (DuckDB/sort/SQL).

**"O teste tal está lento, posso marcar como `t.Parallel()`?"**
Sim, desde que o teste não compartilhe estado global (hoje nenhum compartilha). Rode com `-race` antes de commitar.

**"O expr-lang mudou de path (antonmedv/expr → expr-lang/expr). Posso migrar de volta?"**
Não. O nome canônico hoje é `expr-lang/expr`; `antonmedv/expr` é alias histórico.

**"Posso adicionar logging?"**
Use `log/slog` da stdlib, level `Debug` por padrão desligado. Nunca `fmt.Println` no caminho quente. Qualquer log em `Convert` precisa passar por `io.Writer` injetado, não em `os.Stderr` direto.

**"Posso adicionar progresso (`--progress` bar)?"**
Sim, em `main.go` (não em `converter/`). Use `io.TeeReader` para contar bytes sem quebrar o streaming.

---

## Referências rápidas

- Go version: 1.21+ (testado com 1.25)
- Build reprodutível: `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"`
- Cross-compile: `GOOS=linux|windows|darwin GOARCH=amd64|arm64 go build ...`
- Release atual: [v0.1.0](https://github.com/tiagotnx/dbf-converter-cli/releases/tag/v0.1.0)
