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

- ✅ Formatos de saída: **CSV**, **JSONL** (NDJSON), **SQL** (`CREATE TABLE` + `INSERT`)
- ✅ Codificações de entrada: **CP850**, **Windows-1252**, **ISO-8859-1**
- ✅ Trim automático de padding DBF em campos de texto
- ✅ Numéricos parseados para `float64` nativamente
- ✅ Datas normalizadas (válidas → `YYYY-MM-DD`, inválidas/vazias → `null`)
- ✅ Campos lógicos `L` → `true` / `false` / `null`
- ✅ Registros deletados transparentemente ignorados (padrão; configurável)
- ✅ Motor de filtragem com expressões pré-compiladas ([expr-lang/expr](https://github.com/expr-lang/expr))
- ✅ `--head N` para amostragem rápida
- ✅ `--schema` gera dicionário de dados JSON (`[nome]_schema.json`)
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
| `--input`                   | `-i`  | *(obrig.)*| Caminho do arquivo `.dbf` de entrada                                      |
| `--output`                  | `-o`  | *(obrig.)*| Caminho do arquivo de saída                                               |
| `--format`                  | `-f`  | `csv`     | Formato: `csv`, `jsonl`, `sql`                                            |
| `--encoding`                | `-e`  | `cp850`   | Codificação de origem: `cp850`, `windows-1252`, `iso-8859-1`              |
| `--where`                   |       | *(vazio)* | Expressão de filtro lógica (ver abaixo)                                   |
| `--head`                    |       | `0`       | Limita processamento a N registros (`0` = sem limite)                     |
| `--schema`                  |       | `false`   | Gera dicionário de dados em `[nome]_schema.json`                          |
| `--ignore-deleted`          |       | `true`    | Pula registros marcados como deletados no DBF                             |
| `--table`                   |       | `data`    | Nome da tabela usado por `--format sql`                                   |

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

### Geração de schema (`--schema`)

Quando presente, grava ao lado do DBF de entrada um arquivo `[nome]_schema.json`:

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

Útil para:
- Alimentar LLMs com o contexto das colunas
- Gerar DDL automaticamente em outro dialeto
- Auditar bases legadas sem documentação

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

## Arquitetura

```
main.go                       # abre arquivos e delega ao converter
internal/cli/                 # ParseFlags — validação de argv
internal/converter/           # pipeline streaming: read → filter → export
internal/dbf/                 # parser DBF próprio (header + tipos C/N/F/D/L/I/M)
internal/filter/              # wrapper expr-lang (compile once, run per row)
internal/exporter/            # CSVExporter, JSONLExporter, SQLExporter
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
- Formato de saída **Parquet**
- Benchmark suite (`testing.B`) para validar regressões de performance

---

## Roadmap / limitações conhecidas

- ❌ Campos Memo (`M`, `B`, `G`) são decodificados como texto (sem leitura do `.dbt`).
- ❌ Timestamps Visual FoxPro (`T`) não suportados.
- ❌ Sem suporte a arquivos criptografados ou compactados.
- ❌ CSV não gera BOM UTF-8 (alguns Excel antigos reclamam — use JSONL ou abra via "Import Text").

---

## Licença

MIT — veja `LICENSE`.
