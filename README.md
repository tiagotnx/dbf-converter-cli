# dbf-converter-cli

Conversor de alta performance para arquivos **DBF (dBase III / IV / FoxBase+)** em Go. Lê bases de dados legadas via **streaming linha a linha** — sem nunca carregar o arquivo inteiro em RAM — e produz saída **AI-Ready**: texto já higienizado, numéricos como `float64`, datas em ISO-8601 (`YYYY-MM-DD`), nulos explícitos em vez de padding ou strings vazias.

Construído com metodologia **TDD** (Red-Green-Refactor). Todo o código principal nasceu de um teste que falhava primeiro.

---

## Sumário

- [Motivação](#motivação)
- [Funcionalidades](#funcionalidades)
- [Instalação](#instalação)
- [Uso](#uso)
  - [Referência das flags](#referência-das-flags)
  - [Formatos de saída](#formatos-de-saída)
  - [Codificações suportadas](#codificações-suportadas)
  - [Motor de filtragem (`--where`)](#motor-de-filtragem---where)
  - [Geração de schema (`--schema`)](#geração-de-schema---schema)
  - [Subcomando `preview`](#subcomando-preview)
  - [Progresso e resumo de conclusão](#progresso-e-resumo-de-conclusão)
- [Exemplos práticos](#exemplos-práticos)
- [Arquitetura](#arquitetura)
- [Testes](#testes)
- [Como contribuir](#como-contribuir)
- [Roadmap / limitações conhecidas](#roadmap--limitações-conhecidas)
- [Licença](#licença)

---

## Motivação

Bases de dados legadas do mundo ERP/contábil brasileiro ainda vivem em `.dbf` (Clipper, dBase, FoxPro). Quando esses dados precisam alimentar pipelines modernos — BI, LLMs, data warehouses — surgem três problemas clássicos:

1. **Padding à direita** nos campos `C` (character): `"ABC       "` em vez de `"ABC"`.
2. **Codificação** em CP850 / Windows-1252 em vez de UTF-8, quebrando acentuação.
3. **Campos numéricos e de data** vindos como strings cruas: `"  150.75"`, `"  /  /    "`.

`dbf-converter-cli` resolve os três de uma vez, em uma única passada streaming.

---

## Funcionalidades

- ✅ Formatos de saída: **CSV**, **JSONL** (NDJSON), **SQL** (`CREATE TABLE` + `INSERT`), **Parquet** (colunar, comprimido)
- ✅ Dialetos SQL: `generic` (padrão), `postgres`, `mysql`, `sqlite` via `--dialect`
- ✅ Codificações de entrada: **auto** (padrão), **CP850**, **Windows-1252**, **ISO-8859-1**, **UTF-8**
- ✅ Auto-detecção da codificação a partir do byte 29 do header (language driver)
- ✅ Trim automático de padding DBF em campos de texto
- ✅ Numéricos parseados para `float64` nativamente
- ✅ Datas normalizadas (válidas → `YYYY-MM-DD`, inválidas/vazias → `null`)
- ✅ Campos lógicos `L` → `true` / `false` / `null`
- ✅ Campos `C` que contêm payloads binários (hashes, registros serializados) são emitidos como **hex lossless** em vez de mojibake
- ✅ Registros deletados transparentemente ignorados (padrão; configurável)
- ✅ Motor de filtragem com expressões pré-compiladas ([expr-lang/expr](https://github.com/expr-lang/expr))
- ✅ `--head N` para amostragem rápida
- ✅ `--fields` para projetar apenas um subconjunto das colunas
- ✅ `--schema` gera dicionário de dados JSON (`[nome]_schema.json`)
- ✅ `--progress` emite progresso em stderr com **barra visual, percentual, ETA e rate** em terminal interativo (degrada automaticamente para texto em CI/pipes)
- ✅ **Resumo de conclusão** automático em stderr após a conversão (`✓ N/M records → out.csv (2.1 KB) in 3.4s @ 1.7k rec/s`)
- ✅ `--verbose` ativa logging estruturado em debug via `log/slog`
- ✅ Stdin/stdout sentinel: use `-` como entrada ou saída para compor pipelines
- ✅ Subcomando `preview <file>` para inspecionar um DBF no terminal em um único comando
- ✅ `version` subcomando e `--version` exibem versão/commit/data de build
- ✅ `completion` subcomando gera scripts de autocomplete para bash/zsh/fish/powershell
- ✅ API pública em `pkg/dbf` e `pkg/converter` para consumo como biblioteca
- ✅ Proteção contra SQL injection no nome da tabela (validação de identificador)
- ✅ Tolerante a variantes de header com bytes de padding entre `0x0D` e registros

---

## Instalação

### Pré-requisitos

- Go **≥ 1.21** (testado com Go 1.25)

### Via `go install`

```bash
go install github.com/SEU_USUARIO/dbf-converter-cli@latest
```

> Substitua `SEU_USUARIO` pelo namespace do seu fork. O binário será instalado em `$GOBIN` (ou `$GOPATH/bin`).

### A partir do código-fonte

```bash
git clone https://github.com/SEU_USUARIO/dbf-converter-cli.git
cd dbf-converter-cli
go build -o dbf-converter .
./dbf-converter --help
```

### Binário estático (para distribuir)

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o dbf-converter .
```

O binário resultante tem cerca de 6 MB e não depende de libs externas — pode ser copiado diretamente para servidores Linux.

### Cross-compilation

```bash
# Windows 64-bit
GOOS=windows GOARCH=amd64 go build -o dbf-converter.exe .

# macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -o dbf-converter .
```

---

## Uso

### Referência das flags

| Flag                        | Curta | Padrão    | Descrição                                                                 |
|-----------------------------|:-----:|-----------|---------------------------------------------------------------------------|
| `--input`                   | `-i`  | *(obrig.)*| Caminho do arquivo `.dbf` de entrada (use `-` para stdin)                 |
| `--output`                  | `-o`  | *(obrig.)*| Caminho do arquivo de saída (use `-` para stdout)                         |
| `--format`                  | `-f`  | `csv`     | Formato: `csv`, `jsonl`, `sql`, `parquet`                                 |
| `--encoding`                | `-e`  | `auto`    | Codificação: `auto`, `cp850`, `windows-1252`, `iso-8859-1`, `utf-8`       |
| `--where`                   |       | *(vazio)* | Expressão de filtro lógica (ver abaixo)                                   |
| `--head`                    |       | `0`       | Limita processamento a N registros (`0` = sem limite)                     |
| `--schema`                  |       | `false`   | Gera dicionário de dados em `[nome]_schema.json` ao lado do input         |
| `--schema-out`              |       | *(auto)*  | Caminho explícito para o schema (implica `--schema`; evita sujar a pasta do input) |
| `--ignore-deleted`          |       | `true`    | Pula registros marcados como deletados no DBF                             |
| `--table`                   |       | `data`    | Nome da tabela usado por `--format sql`                                   |
| `--dialect`                 |       | `generic` | Dialeto SQL: `generic`, `postgres`, `mysql`, `sqlite`                     |
| `--fields`                  |       | *(todas)* | Lista separada por vírgula das colunas a exportar                         |
| `--progress`                |       | `false`   | Emite progresso em stderr (uma linha por segundo)                         |
| `--verbose`                 |       | `false`   | Ativa logs de debug via `log/slog` em stderr                              |
| `--version`                 |       |           | Imprime a versão e encerra                                                 |

### Formatos de saída

#### CSV
```csv
ID,NOME,VALOR,DATA,ATIVO
1,João Silva,150.75,2025-01-15,true
2,Maria,,,,false
```
- Nulos viram células vazias (sem literal `null` / `NULL`).
- Aspas e vírgulas são escapadas pelo `encoding/csv` padrão.

#### JSONL (NDJSON)
```json
{"ID":1,"NOME":"João Silva","VALOR":150.75,"DATA":"2025-01-15","ATIVO":true}
{"ID":2,"NOME":"Maria","VALOR":null,"DATA":null,"ATIVO":false}
```
- Um objeto JSON por linha; ideal para ingestão em Elasticsearch, BigQuery, DuckDB.
- Ordem das chaves preservada conforme header do DBF (não alfabética).

#### SQL
```sql
CREATE TABLE IF NOT EXISTS clientes (
  ID NUMERIC,
  NOME TEXT,
  VALOR NUMERIC,
  DATA DATE,
  ATIVO BOOLEAN
);
INSERT INTO clientes (ID, NOME, VALOR, DATA, ATIVO) VALUES (1, 'João Silva', 150.75, '2025-01-15', TRUE);
INSERT INTO clientes (ID, NOME, VALOR, DATA, ATIVO) VALUES (2, 'Maria', NULL, NULL, FALSE);
```
- Dialeto neutro (compatível com PostgreSQL, SQLite, MySQL 8+).
- Apóstrofos são escapados (`'O''Brien'`).
- Nome de tabela validado contra regex `^[A-Za-z_][A-Za-z0-9_]*$` — impede injeção.
- Use `--dialect postgres|mysql|sqlite` para tipos nativos da base de destino.

#### Parquet
Formato colunar comprimido, ideal para data lakes e análise. Todas as colunas são declaradas como `optional` para preservar nulos (numéricos, datas e lógicos indeterminados viram `null` em vez de zero/vazio).

```bash
dbf-converter -i clientes.dbf -o clientes.parquet -f parquet
```
Arquivos Parquet podem ser lidos direto por pandas, DuckDB, Apache Spark, PyArrow e Polars:
```python
import pandas as pd
df = pd.read_parquet('clientes.parquet')
```
- Datas permanecem como string ISO-8601 (consistente com CSV/JSONL).
- Numéricos são `DOUBLE`, lógicos são `BOOLEAN`, texto é `BYTE_ARRAY(UTF8)`.
- Colunas são endereçadas por **nome** em Parquet; a ordem física no arquivo pode diferir da do DBF.

### Codificações suportadas

| Valor              | Uso típico                                   |
|--------------------|----------------------------------------------|
| `cp850`            | dBase DOS, Clipper (padrão brasileiro legado)|
| `windows-1252`     | Visual FoxPro em Windows                     |
| `iso-8859-1`       | Latin-1 / ISO puro                           |

### Motor de filtragem (`--where`)

O filtro é uma expressão [expr-lang](https://expr-lang.org/docs/language-definition) compilada **uma única vez** na inicialização e executada por registro. Suporta:

- Comparações: `==`, `!=`, `<`, `<=`, `>`, `>=`
- Lógicos: `&&`, `||`, `!`
- Literais: `'string'`, `123.45`, `true`, `nil`
- Pertinência: `STATUS in ['A','B','C']`
- Strings: `startsWith(NOME, 'Jo')`, `contains(DESCR, 'urgente')`

**Importante:**
- Campos numéricos do DBF são `float64` — compare com números, não strings.
- Campos vazios/inválidos ficam `nil` — teste com `VALOR != nil && VALOR > 0`.
- Nomes de campo são case-sensitive e seguem o header do DBF (geralmente maiúsculos).

Exemplos:
```bash
--where "STATUS == 'A'"
--where "VALOR >= 150 && VALOR <= 500"
--where "STATUS == 'A' && DATA >= '2024-01-01'"
--where "NOME != nil && startsWith(NOME, 'João')"
```

### Campos `C` com conteúdo binário

Alguns ERPs legados reaproveitam colunas `C` (character) para guardar payloads binários opacos — MD5/SHA raw, registros serializados, assinaturas digitais. Tentar decodificar esses bytes como CP850 produz mojibake (`-┌à'ÞNm©º7Ã\╔1...`) que nenhum consumidor downstream aceita.

O reader detecta a presença de bytes de controle (< 0x20, exceto `\t\r\n`) e, nesse caso, emite o conteúdo como **hex lossless lowercase** (sem o padding `0x00` / espaços à direita):

```
MD5AUT98 (campo C, 49 bytes):
  antes do fix → "-┌à'ÞNm©º7Ã\╔1`m╦ÛÖ╦¼ ²-8k╦F..."   (mojibake)
  depois       → "2dda8527e84e6d19b8a737c75c11c931606d17cbea99cbacfffd2d386b16cb46"
```

Texto CP850 legítimo (incluindo acentuação como `João`, `ação`, `São Paulo`) **nunca** é tocado pela heurística — apenas o que contém bytes de controle crus.

### Geração de schema (`--schema`, `--schema-out`)

Quando `--schema` (ou `--schema-out <path>`) está presente, o conversor grava um arquivo JSON com o dicionário de dados:

```json
{
  "totalRecords": 9667,
  "fieldCount": 137,
  "fields": [
    { "name": "ID", "type": "N", "length": 5, "decimal": 0 },
    { "name": "NOME", "type": "C", "length": 40, "decimal": 0 },
    { "name": "VALOR", "type": "N", "length": 10, "decimal": 2 }
  ]
}
```

Por padrão, o arquivo é gravado ao lado do input (`clientes.dbf` → `clientes_schema.json`). Use `--schema-out /tmp/schema.json` para escolher o caminho explicitamente — útil quando o diretório do input é read-only, está versionado (`testdata/`), ou quando o schema vai para outro volume.

```bash
# Caminho derivado (default): grava ao lado do .dbf
dbf-converter -i data/clientes.dbf -o clientes.csv --schema
# → data/clientes_schema.json

# Caminho explícito: não polui data/
dbf-converter -i data/clientes.dbf -o clientes.csv --schema-out build/clientes.schema.json
```

Útil para:
- Alimentar LLMs com o contexto das colunas
- Gerar DDL automaticamente em outro dialeto
- Auditar bases legadas sem documentação

### Subcomando `preview`

`preview <file>` é um atalho para o primeiro comando que se roda em um DBF desconhecido:

```bash
dbf-converter preview clientes.dbf
# equivalente a:
dbf-converter -i clientes.dbf -o - -f jsonl --head 20 --schema
```

- Emite JSONL direto no **terminal** (stdout) — não cria arquivo de saída.
- Grava o schema em `clientes_schema.json` ao lado do input (igual ao `--schema`).
- Aceita `--head N` para mudar o tamanho da amostra.

```bash
dbf-converter preview movpro01.dbf --head 5   # só os 5 primeiros registros
dbf-converter preview movpro01.dbf | jq .     # inspecionar com jq
```

### Progresso e resumo de conclusão

Com `--progress`, o conversor emite em stderr uma linha atualizada **uma vez por segundo** indicando o andamento. Quando stderr é um terminal interativo, o formato inclui barra visual, percentual, total, rate e ETA:

```
vendas.dbf [=======>            ] 42.3% 4230/10000 @ 1.2k rec/s ETA 00:04:32
```

Em pipes ou CI (stderr não-TTY), o mesmo dado é emitido em texto plano (sem barra) para não poluir logs:

```
vendas.dbf 42.3% 4230/10000 @ 1.2k rec/s ETA 00:04:32
```

Ao final — independentemente de `--progress` — uma linha de **resumo de conclusão** é impressa em stderr sempre que o usuário está em terminal interativo ou usou `--progress`/`--verbose`:

```
✓ 9856/10000 records → vendas.csv (2.1 MB) in 8.3s @ 1.2k rec/s
```

- Mostra `N/M` quando `N != M` (ex.: filtro ativo ou deletados descartados).
- Omite o tamanho quando a saída é stdout (`-`).
- **Nunca** é impresso em pipes silenciosos (consumidores de stdout não são afetados; stderr permanece limpo quando redirecionado).

---

## Exemplos práticos

### Caso 1 — Modernizar uma base Clipper para análise

```bash
./dbf-converter -i clientes.dbf -o clientes.csv
```

Defaults aplicados: formato `csv`, encoding `cp850`, `--ignore-deleted=true`. Saída pronta para `pandas.read_csv()` ou `DuckDB`.

### Caso 2 — Amostragem JSONL + dicionário para um LLM

```bash
./dbf-converter \
  -i vendas.dbf \
  -o vendas_sample.jsonl \
  -f jsonl \
  -e windows-1252 \
  --head 500 \
  --schema
```

Produz `vendas_sample.jsonl` (500 registros) + `vendas_schema.json`. Ambos podem ser colados direto em um prompt.

### Caso 3 — Migração para PostgreSQL com filtro de qualidade

```bash
./dbf-converter \
  -i movpro01.dbf \
  -o movpro01.sql \
  -f sql \
  --table movimento_produto \
  --where "DATAMOV05 != nil && QTDEMOV05 > 0"

psql meudb < movpro01.sql
```

Descarta registros sem data ou com quantidade zero antes de chegar ao banco.

### Caso 4 — Pipeline encadeado com ferramentas Unix

```bash
./dbf-converter -i cadastros.dbf -o /dev/stdout -f jsonl \
  | jq -r 'select(.ATIVO == true) | [.ID, .EMAIL] | @csv' \
  > emails_ativos.csv
```

---

## Usando com agentes de IA (Claude Code / Copilot)

Este repo publica uma **skill plug-and-play** em [`skills/dbf-converter/`](skills/dbf-converter/) que ensina agentes de IA a usarem o CLI corretamente no seu projeto. Ela cobre detecção de `.dbf`, inspeção de schema antes de conversão, escolha de formato, construção de filtros `--where` com sintaxe correta e troubleshooting de acentuação/encoding.

### Claude Code
```bash
mkdir -p .claude/skills/dbf-converter
curl -sSL -o .claude/skills/dbf-converter/SKILL.md \
  https://raw.githubusercontent.com/tiagotnx/dbf-converter-cli/main/skills/dbf-converter/claude/dbf-converter/SKILL.md
```

### GitHub Copilot
```bash
mkdir -p .github/instructions .github/prompts
curl -sSL -o .github/instructions/dbf-converter.instructions.md \
  https://raw.githubusercontent.com/tiagotnx/dbf-converter-cli/main/skills/dbf-converter/copilot/dbf-converter.instructions.md
curl -sSL -o .github/prompts/dbf-convert.prompt.md \
  https://raw.githubusercontent.com/tiagotnx/dbf-converter-cli/main/skills/dbf-converter/copilot/dbf-convert.prompt.md
```

Veja [`skills/dbf-converter/README.md`](skills/dbf-converter/README.md) para detalhes.

## Uso como biblioteca

Além do CLI, o pacote expõe uma API pública estável em `pkg/`:

```go
import (
    "os"

    "dbf-converter-cli/pkg/converter"
    "dbf-converter-cli/pkg/dbf"
)

// 1) Pipeline completo em uma chamada
func fullPipeline() error {
    in, _ := os.Open("clientes.dbf")
    defer in.Close()
    out, _ := os.Create("clientes.jsonl")
    defer out.Close()
    return converter.Convert(converter.Config{
        Input: in, Output: out,
        Format: "jsonl", Encoding: "auto",
    })
}

// 2) Leitor de baixo nível para integrar ao seu próprio pipeline
func customLoop() error {
    f, _ := os.Open("clientes.dbf")
    defer f.Close()
    r, err := dbf.NewReader(f, "auto")
    if err != nil { return err }
    for {
        rec, err := r.Next()
        if err != nil { return err }
        if rec == nil { break }
        _ = rec.Values // map[string]interface{} — pronto para JSON/ML/ETL
    }
    return nil
}
```

## Arquitetura

```
main.go                       # abre arquivos e delega ao converter
internal/cli/                 # Cobra root command e validação de flags
internal/converter/           # pipeline streaming: read → filter → export
internal/dbf/                 # parser DBF próprio (header + tipos C/N/F/D/L/I/M)
internal/filter/              # wrapper expr-lang (compile once, run per row)
internal/exporter/            # CSVExporter, JSONLExporter, SQLExporter
pkg/dbf/                      # API pública estável — reader streaming
pkg/converter/                # API pública — Convert(Config)
testdata/gen_fixture.go       # gerador de DBF sintético para smoke tests
```

**Princípios de design:**

1. **Streaming estrito.** `converter.Convert` nunca acumula registros — lê um, filtra, exporta, libera. O pico de RAM independe do tamanho do DBF.
2. **Interface `Exporter` única.** Os três formatos implementam `Write(row) / Close()` — adicionar um novo formato é uma classe e uma entrada no switch.
3. **Parser DBF próprio.** Em vez de depender de uma biblioteca, o módulo `internal/dbf` implementa o formato (header + tipos comuns), o que dá controle total sobre trim, encoding e normalização no ponto de decodificação.
4. **Validação na borda.** CLI valida flags antes de qualquer I/O; o SQL exporter valida identificadores antes de emitir DDL.
5. **Teste primeiro.** Cada módulo tem um `_test.go` escrito antes da implementação, cobrindo o caminho feliz e os edge cases citados na spec (datas inválidas, numéricos vazios, registros deletados, expressões malformadas, headers com padding).

---

## Testes

Rodar toda a suíte:

```bash
go test ./...
```

Com verbose e sem cache:

```bash
go test ./... -v -count=1
```

Cobertura:

```bash
go test ./... -cover
go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out
```

**Fixture sintético** (útil para smoke tests manuais):

```bash
go run testdata/gen_fixture.go testdata/sample.dbf
./dbf-converter -i testdata/sample.dbf -o /tmp/out.csv
```

Os testes cobrem, entre outros:

- `TestReader_TrimAndEncoding` — padding DBF + decodificação ISO-8859-1
- `TestReader_DateNormalization` — `"  /  /    "` → `nil`
- `TestReader_SkipDeleted` — registros marcados com `*`
- `TestReader_HeaderWithTrailingPadding` — variantes com pad entre `0x0D` e records
- `TestFilter_LogicalAnd` — `STATUS == 'A' && VALOR >= 150`
- `TestSQLExporter_RejectsInvalidTableName` — guard contra SQL injection
- `TestConvert_HeadCountsAfterFilter` — `--head` aplica-se após o filtro

---

## Como contribuir

Contribuições são bem-vindas! O fluxo esperado:

### 1. Abra uma issue antes de um PR grande

Descreva o problema ou feature, anexe um `.dbf` mínimo reproduzindo o caso (se possível anonimize). Isso evita retrabalho.

### 2. Setup local

```bash
git clone https://github.com/SEU_USUARIO/dbf-converter-cli.git
cd dbf-converter-cli
go mod download
go test ./...
```

### 3. Siga o ciclo TDD

Este é um projeto construído com **Red-Green-Refactor**. Para cada alteração de comportamento:

1. **RED** — adicione um teste em `*_test.go` que falha expressando a nova expectativa.
2. **GREEN** — escreva o mínimo de código para o teste passar.
3. **REFACTOR** — limpe o código mantendo a suíte verde.

Use **table-driven tests** quando apropriado (veja `TestFilter_NumericComparison` como referência).

### 4. Padrões de código

- `go fmt ./...` obrigatório antes de commitar.
- `go vet ./...` deve passar sem warnings.
- Nomes de funções exportadas documentadas com comentário `//`.
- Erros embrulhados com `fmt.Errorf("contexto: %w", err)` — nunca `err.Error()` concatenado.
- Sem `panic` fora de código obviamente incorreto; sempre retorne `error`.

### 5. Commits

Use mensagens no imperativo em inglês (para manter consistência com o ecossistema Go):

```
feat(dbf): support memo fields (M type)
fix(filter): handle nil env values without panic
test(exporter): add case for empty date in SQL output
docs(readme): clarify --where examples
```

### 6. Pull request

- Branch a partir de `main`.
- Um PR = uma preocupação (não misture feature + refactor + formatação).
- Descreva o **porquê**, não só o **o quê**. Se fixa um bug, anexe passos para reproduzir.
- Aguarde a suíte de CI passar antes de solicitar revisão.

### Áreas onde contribuições são especialmente bem-vindas

- Suporte a campos **Memo** (`M`) com leitura do `.dbt` / `.fpt` associado
- Suporte a **Visual FoxPro** com campos `V`, `W`, `T` (timestamp)
- Mais codepages (CP437, CP852, Shift-JIS)
- Opções de compressão e tamanho de row group configuráveis no exportador Parquet
- Ampliar a suíte de benchmarks em `internal/converter/bench_test.go`

---

## Roadmap / limitações conhecidas

- ❌ Campos Memo (`M`, `B`, `G`) são decodificados como texto (sem leitura do `.dbt`).
- ❌ Timestamps Visual FoxPro (`T`) não suportados.
- ❌ Sem suporte a arquivos criptografados ou compactados.
- ❌ CSV não gera BOM UTF-8 (alguns Excel antigos reclamam — use JSONL ou abra via "Import Text").

---

## Licença

MIT — veja `LICENSE`.
