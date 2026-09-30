# Detector de fraude, backend em Go: plano de implementação

> **Para quem for executar:** SUB-SKILL OBRIGATÓRIA: use superpowers:subagent-driven-development (recomendado) ou superpowers:executing-plans para implementar este plano tarefa por tarefa. Os passos usam checkbox (`- [ ]`) para acompanhamento. Ao commitar, acrescente a linha de atribuição `Co-Authored-By` exigida pela sessão.

**Objetivo:** entregar a API em Go, o carregador do grafo (seeder), a infra Docker e o README, de forma que dê para subir o Neo4j, carregar o PaySim, rodar benchmarks por `curl` e simular uma transação, tudo contra um modelo local no Ollama.

**Arquitetura:** o Go lê o CSV em streaming, monta uma amostra estratificada, enriquece cada transação com features de grafo via Cypher ao vivo (só olhando o passado por `seq`) e chama o modelo `noul` num pool separado com limite de chamadas simultâneas. Cada execução grava `results.jsonl` e `summary.json`. O dashboard (plano separado) só consome a API REST.

**Tech Stack:** Go 1.27.1 (biblioteca padrão mais `github.com/neo4j/neo4j-go-driver/v5` v5.28.5), `net/http`, `log/slog`, Neo4j 5.26 Community, Docker Compose, Ollama no host.

**Spec:** `docs/superpowers/specs/2026-09-30-fraud-detector-jev-design.md`

**Este é o plano 1 de 2.** O plano 2 (`2026-09-30-fraud-detector-dashboard.md`) implementa o dashboard Gradio e depende só do contrato REST descrito aqui.

## Desvios da spec (decididos ao planejar, a spec já foi atualizada)

- Imagem do Neo4j: `neo4j:5.26-community` (última LTS da linha 5.x, e já está baixada na sua máquina) no lugar de `5.20`.
- Os tipos das features (`GraphFeatures`, `FeatureQuery`, `Subgraph`) ficam em `internal/domain`, para evitar import circular. A interface `GraphFeatureSource` continua em `internal/service/features.go`.
- O mock do modelo fica no subpacote `internal/infrastructure/jev/jevtest` (`mock_server.go`), para não levar `httptest` para dentro do binário de produção.
- A carga do grafo fica em `internal/infrastructure/neo4j/loader.go`, e `cmd/seeder/seed_graph.go` é só a linha de comando. Assim a carga é testável.
- O bloco `sample` do `summary.json` traz também o universo elegível (`eligible_rows`, `eligible_fraud`, `prevalence`). Não existe um bloco `dataset` separado.
- `GET /healthz` responde 200 se o Neo4j estiver ok, mesmo com o Ollama fora. O estado do Ollama vai no corpo, com a dica do README.
- `GET /benchmarks/{id}/results` responde 409 enquanto a execução está rodando.
- Nova variável `NEO4J_QUERY_TIMEOUT` (padrão `2s`), que a spec já pedia como "timeout por query" sem dar nome.

## Restrições globais

Todas as tarefas herdam estas regras, com os valores copiados da spec.

- Go 1.27.1, módulo `fraud-detector-jev`. Única dependência externa: `github.com/neo4j/neo4j-go-driver/v5`.
- Comentários, logs, mensagens de erro, README e commits em português informal, sem emoji e sem travessão. Sem ponto e vírgula em texto (o ponto e vírgula da sintaxe do Go é normal). Identificadores e campos JSON em inglês.
- Contrato do modelo: rota fixa `/v1/systemone`, resposta lida em `answers.is_fraud.noul`, número entre 0 e 1. Não existem `value`, `probability`, `confidence` nem `elapsedMs`.
- `state` enviado como string JSON. A instrução da pergunta `is_fraud` é idêntica com e sem grafo.
- Nenhuma query de feature lê `isFraud` nem `isFlagged`. Toda query de feature filtra `t.seq < $beforeSeq AND t.step <= $atStep`.
- Padrões do `POST /benchmarks`: `n=2000`, `fraud_pct=10`, `seed=42`, `workers=32`, `graph=true`, `types=[TRANSFER, CASH_OUT]`, `window_hours=24`, `cycle_max_hops=3`, `max_inflight=4`, `threshold=0.5`. Limites: `n` no máximo 500000 e `workers` no máximo 256.
- Variáveis do modelo: `JEV_BASE_URL` (padrão `http://host.docker.internal:11434`), `JEV_MODEL` (padrão `tev1:4b`), `JEV_MAX_INFLIGHT` (padrão `4`), `JEV_TIMEOUT` (padrão `30s`).
- Só uma execução de benchmark por vez (a segunda recebe 409).
- Portas publicadas só em `127.0.0.1`. Sem autenticação.
- `slog` para logs, `context` em toda chamada de I/O, testes com `-race`.

## Foco da revisão

Cinco entradas que a spec deixa implícitas e que mais provavelmente vão morder quem usar. Cada uma tem um teste na tarefa dona do código.

1. **Conta que nunca apareceu no grafo** (o `evaluate` com IDs inventados): as features vêm zeradas, `dest_account_age_steps` vem `null` e não há erro. Teste na Tarefa 8.
2. **CSV com final de linha CRLF e linha em branco no fim**: o leitor tolera e não conta linha fantasma. Teste na Tarefa 3.
3. **`fraud_pct` em 0 ou 100 e `n` maior que o universo elegível**: o sampler devolve o que existe, sem quebrar nem repetir transação. Teste na Tarefa 4.
4. **Resposta inválida do modelo** (corpo vazio, não JSON, `noul` ausente ou fora de 0 a 1): vira erro daquela transação e não derruba a execução. Testes nas Tarefas 5 e 10.
5. **Cancelar no meio de um benchmark**: os resultados já gravados ficam, o status vira `canceled` e nenhuma transação em voo é contada como erro. Testes nas Tarefas 10 e 11.

## Como este plano foi verificado

Antes de entregar, o código de todos os blocos foi extraído para um módulo temporário fora do projeto e executado:

- `gofmt`, `go vet` (com e sem a tag `integration`) e `go build` dos dois binários, sem avisos.
- Todos os testes unitários com `-race`, repetidos várias vezes para pegar instabilidade de concorrência.
- Os testes de integração contra um Neo4j 5.26 de verdade: as queries de features nos 10 cenários, o subgrafo, a carga com retomada e a guarda contra vazamento. Também foi feita uma **mutação de propósito** (tirar o filtro de `seq` de uma query) para confirmar que os testes falham quando deveriam.
- A imagem Docker construída, o compose de pé (Neo4j e API saudáveis), o seeder carregando o CSV pequeno e benchmarks completos com e sem grafo. O Ollama ainda não estava instalado, então um servidor falso do `/v1/systemone` fez o papel dele.
- Cancelamento, os erros 409 e 404, o replay do simulador (as features bateram exatamente com as do `results.jsonl`), o encerramento limpo e a queda brusca da API.

Isso encontrou três problemas que já estão corrigidos neste plano: o `go mod download` falhava dentro do build do Docker por causa do DNS IPv6 do host (o compose agora usa `network: host` no build), o P50 do Neo4j saía inflado por partida fria (o pipeline agora aquece o Neo4j antes de medir), e o roteiro de aceite descrevia errado o reinício da API.

**O que ainda não foi verificado** (só dá para conferir na Tarefa 17, com o hardware e os dados reais):

- O Ollama de verdade e os modelos `tev1` e `nimble`: formato exato da resposta, velocidade na sua GPU ou CPU e qualidade em fraude.
- O PaySim inteiro: tempo da carga, tamanho do grafo em disco e latência das queries de ciclo em escala.
- A premissa de que o CSV do Kaggle está ordenado por `step` (o seeder confere e para se não estiver).

---

## Mapa de arquivos

Cada arquivo tem uma responsabilidade só.

| Arquivo | Responsabilidade |
| --- | --- |
| `internal/config/config.go` | Lê e valida as variáveis de ambiente |
| `internal/domain/transaction.go` | `Transaction` e tipos de transação válidos |
| `internal/domain/features.go` | `GraphFeatures`, `FeatureQuery`, `Subgraph` |
| `internal/domain/decision.go` | `Decision` do modelo e `ResultsSink` |
| `internal/domain/result.go` | `Result` (uma linha do `results.jsonl`) e `NewResult` |
| `internal/domain/metrics.go` | Matriz de confusão, métricas, percentis, precisão reprojetada |
| `internal/domain/run.go` | `RunParams`, `RunSummary`, status, `ErrNotFound` |
| `internal/domain/stats.go` | `DatasetStats` |
| `internal/infrastructure/csv/streaming_reader.go` | Leitura em streaming do CSV, com conferência de ordem |
| `internal/service/sampler.go` | Amostra estratificada com reservoir sampling |
| `internal/infrastructure/jev/client.go` | Cliente HTTP do modelo, retry e limite de simultaneidade |
| `internal/infrastructure/jev/models.go` | Lista os modelos baixados no Ollama |
| `internal/infrastructure/jev/jevtest/mock_server.go` | Servidor falso do modelo, só para testes |
| `internal/service/features.go`, `classifier.go` | Interfaces (portas) |
| `internal/service/enricher.go` | Monta o `state` e a `FeatureQuery` |
| `internal/infrastructure/neo4j/client.go` | Conexão, features, subgrafo |
| `internal/infrastructure/neo4j/queries.go` | Todo o texto Cypher |
| `internal/infrastructure/neo4j/loader.go` | Carga do CSV no grafo, com retomada |
| `internal/infrastructure/runstore/runstore.go` | Grava e lê execuções e estatísticas do dataset |
| `internal/service/benchmark_pipeline.go` | Estágios, medição e métricas de uma execução |
| `internal/service/run_manager.go` | Uma execução por vez, cancelamento e resumo final |
| `internal/service/params.go` | Padrões e validação do pedido de benchmark |
| `internal/service/evaluator.go` | Caso de uso do simulador (`evaluate`) |
| `internal/infrastructure/web/*.go` | Roteador, handlers e erros JSON |
| `cmd/api/main.go`, `cmd/seeder/seed_graph.go` | Binários |
| `Dockerfile.api`, `docker-compose.yml`, `.env.example`, `.dockerignore` | Infra |
| `Makefile`, `scripts/test-integration.sh`, `scripts/smoke_benchmark.sh` | Automação |
| `testdata/metrics_golden.json`, `testdata/mini_paysim.csv` | Dados de teste compartilhados |
| `README.md` | Guia de uso |

Convenção dos blocos: um bloco de código com `file=caminho` no cabeçalho é o **conteúdo completo** daquele arquivo.

---
## Tarefa 1: módulo Go e configuração

**Arquivos:**
- Criar: `go.mod` (pelo comando), `internal/config/config.go`
- Testar: `internal/config/config_test.go`

**Interfaces:**
- Produz: `config.Load(getenv func(string) string) (config.Config, error)` e `config.DefaultInstructions`. Os campos de `Config` são usados pelas Tarefas 14 e 15.

- [ ] **Passo 1: criar o módulo e baixar o driver**

Rode na raiz do projeto (`jev_golang_antifraude`):

```bash
export PATH="$HOME/.local/go/bin:$HOME/go/bin:$PATH"
go mod init fraud-detector-jev
go get github.com/neo4j/neo4j-go-driver/v5@v5.28.5
```

Esperado: `go.mod` criado com `module fraud-detector-jev`. O `go.sum` aparece depois do primeiro `go mod tidy` (Passo 4 da Tarefa 7).

- [ ] **Passo 2: escrever o teste que falha**

```go file=internal/config/config_test.go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadUsaPadroes(t *testing.T) {
	cfg, err := Load(env(map[string]string{"NEO4J_PASSWORD": "segredo123"}))
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.Neo4jURI != "bolt://localhost:7687" || cfg.Neo4jUser != "neo4j" {
		t.Errorf("neo4j = %q %q", cfg.Neo4jURI, cfg.Neo4jUser)
	}
	if cfg.Neo4jQueryTimeout != 2*time.Second {
		t.Errorf("Neo4jQueryTimeout = %v", cfg.Neo4jQueryTimeout)
	}
	if cfg.JevBaseURL != "http://host.docker.internal:11434" || cfg.JevModel != "tev1:4b" {
		t.Errorf("jev = %q %q", cfg.JevBaseURL, cfg.JevModel)
	}
	if cfg.JevMaxInflight != 4 || cfg.JevTimeout != 30*time.Second {
		t.Errorf("limites do modelo = %d %v", cfg.JevMaxInflight, cfg.JevTimeout)
	}
	if cfg.MaxSampleSize != 500000 || cfg.MaxWorkers != 256 {
		t.Errorf("limites da API = %d %d", cfg.MaxSampleSize, cfg.MaxWorkers)
	}
	if cfg.Instructions != DefaultInstructions {
		t.Errorf("instruções não são as padrão")
	}
}

func TestLoadExigeSenhaDoNeo4j(t *testing.T) {
	_, err := Load(env(nil))
	if err == nil || !strings.Contains(err.Error(), "NEO4J_PASSWORD") {
		t.Fatalf("esperava erro citando NEO4J_PASSWORD, veio %v", err)
	}
}

func TestLoadRejeitaValoresInvalidos(t *testing.T) {
	casos := map[string]map[string]string{
		"inflight não numérico":  {"JEV_MAX_INFLIGHT": "muitos"},
		"inflight fora da faixa": {"JEV_MAX_INFLIGHT": "0"},
		"timeout inválido":       {"JEV_TIMEOUT": "rapido"},
		"timeout negativo":       {"NEO4J_QUERY_TIMEOUT": "-1s"},
		"workers demais":         {"MAX_WORKERS": "99999"},
	}
	for nome, vars := range casos {
		t.Run(nome, func(t *testing.T) {
			vars["NEO4J_PASSWORD"] = "segredo123"
			if _, err := Load(env(vars)); err == nil {
				t.Fatal("esperava erro")
			}
		})
	}
}

func TestLoadJuntaTodosOsErros(t *testing.T) {
	_, err := Load(env(map[string]string{"JEV_MAX_INFLIGHT": "x", "JEV_TIMEOUT": "y"}))
	if err == nil {
		t.Fatal("esperava erro")
	}
	for _, trecho := range []string{"NEO4J_PASSWORD", "JEV_MAX_INFLIGHT", "JEV_TIMEOUT"} {
		if !strings.Contains(err.Error(), trecho) {
			t.Errorf("erro não cita %s: %v", trecho, err)
		}
	}
}

func TestLoadLeInstrucoesDeArquivo(t *testing.T) {
	arquivo := filepath.Join(t.TempDir(), "instrucoes.txt")
	if err := os.WriteFile(arquivo, []byte("  Essa transação é suspeita?\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(env(map[string]string{
		"NEO4J_PASSWORD":        "segredo123",
		"JEV_INSTRUCTIONS_FILE": arquivo,
	}))
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if cfg.Instructions != "Essa transação é suspeita?" {
		t.Errorf("Instructions = %q", cfg.Instructions)
	}
}

func TestLoadRejeitaArquivoDeInstrucoesVazioOuAusente(t *testing.T) {
	vazio := filepath.Join(t.TempDir(), "vazio.txt")
	if err := os.WriteFile(vazio, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, caminho := range []string{vazio, filepath.Join(t.TempDir(), "nao-existe.txt")} {
		_, err := Load(env(map[string]string{
			"NEO4J_PASSWORD":        "segredo123",
			"JEV_INSTRUCTIONS_FILE": caminho,
		}))
		if err == nil {
			t.Errorf("esperava erro para %s", caminho)
		}
	}
}
```

- [ ] **Passo 3: rodar e ver falhar**

Rode: `go test ./internal/config/`
Esperado: FAIL com `undefined: Load` (o pacote ainda não tem implementação).

- [ ] **Passo 4: implementar**

```go file=internal/config/config.go
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultInstructions é a pergunta padrão feita ao modelo. Ela é a mesma nas
// duas variantes do benchmark (com e sem grafo), para a única diferença entre
// elas ser o conteúdo do state.
const DefaultInstructions = "Determine se esta transação tem perfil típico de fraude bancária, esvaziamento ilícito de conta ou lavagem de dinheiro, com base em todos os dados fornecidos."

// Config reúne tudo que vem de variável de ambiente.
type Config struct {
	HTTPAddr          string
	Neo4jURI          string
	Neo4jUser         string
	Neo4jPassword     string
	Neo4jQueryTimeout time.Duration
	JevBaseURL        string
	JevModel          string
	JevMaxInflight    int
	JevTimeout        time.Duration
	Instructions      string
	DataPath          string
	RunsDir           string
	MaxSampleSize     int
	MaxWorkers        int
}

// Load lê as variáveis usando getenv (assim o teste não mexe no ambiente real)
// e devolve todos os problemas encontrados de uma vez.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		HTTPAddr:      orDefault(getenv("HTTP_ADDR"), ":8080"),
		Neo4jURI:      orDefault(getenv("NEO4J_URI"), "bolt://localhost:7687"),
		Neo4jUser:     orDefault(getenv("NEO4J_USER"), "neo4j"),
		Neo4jPassword: getenv("NEO4J_PASSWORD"),
		JevBaseURL:    orDefault(getenv("JEV_BASE_URL"), "http://host.docker.internal:11434"),
		JevModel:      orDefault(getenv("JEV_MODEL"), "tev1:4b"),
		DataPath:      orDefault(getenv("DATA_PATH"), "data/transactions.csv"),
		RunsDir:       orDefault(getenv("RUNS_DIR"), "runs"),
		Instructions:  DefaultInstructions,
	}

	var errs []error
	if cfg.Neo4jPassword == "" {
		errs = append(errs, errors.New("NEO4J_PASSWORD é obrigatória"))
	}

	var err error
	if cfg.JevMaxInflight, err = intVar(getenv, "JEV_MAX_INFLIGHT", 4, 1, 64); err != nil {
		errs = append(errs, err)
	}
	if cfg.MaxSampleSize, err = intVar(getenv, "MAX_SAMPLE_SIZE", 500000, 1, 5000000); err != nil {
		errs = append(errs, err)
	}
	if cfg.MaxWorkers, err = intVar(getenv, "MAX_WORKERS", 256, 1, 4096); err != nil {
		errs = append(errs, err)
	}
	if cfg.JevTimeout, err = durationVar(getenv, "JEV_TIMEOUT", 30*time.Second); err != nil {
		errs = append(errs, err)
	}
	if cfg.Neo4jQueryTimeout, err = durationVar(getenv, "NEO4J_QUERY_TIMEOUT", 2*time.Second); err != nil {
		errs = append(errs, err)
	}

	if path := getenv("JEV_INSTRUCTIONS_FILE"); path != "" {
		text, err := readInstructions(path)
		if err != nil {
			errs = append(errs, err)
		} else {
			cfg.Instructions = text
		}
	}

	return cfg, errors.Join(errs...)
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func intVar(getenv func(string) string, name string, def, lo, hi int) (int, error) {
	raw := getenv(name)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def, fmt.Errorf("%s precisa ser um número inteiro, veio %q", name, raw)
	}
	if n < lo || n > hi {
		return def, fmt.Errorf("%s precisa estar entre %d e %d, veio %d", name, lo, hi, n)
	}
	return n, nil
}

func durationVar(getenv func(string) string, name string, def time.Duration) (time.Duration, error) {
	raw := getenv(name)
	if raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return def, fmt.Errorf("%s precisa ser uma duração como 30s, veio %q", name, raw)
	}
	if d <= 0 {
		return def, fmt.Errorf("%s precisa ser maior que zero, veio %s", name, raw)
	}
	return d, nil
}

func readInstructions(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("lendo JEV_INSTRUCTIONS_FILE: %w", err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", fmt.Errorf("JEV_INSTRUCTIONS_FILE (%s) está vazio", path)
	}
	return text, nil
}
```

- [ ] **Passo 5: rodar e ver passar**

Rode: `go test -race ./internal/config/`
Esperado: `ok  fraud-detector-jev/internal/config`.

- [ ] **Passo 6: commitar**

```bash
git add go.mod internal/config
git commit -m "feat: adiciona módulo Go e leitura das variáveis de ambiente"
```

---

## Tarefa 2: domínio (tipos e métricas)

**Arquivos:**
- Criar: `internal/domain/transaction.go`, `features.go`, `decision.go`, `result.go`, `metrics.go`, `run.go`, `stats.go`
- Criar: `testdata/metrics_golden.json` (compartilhado com o Python)
- Testar: `internal/domain/transaction_test.go`, `metrics_test.go`, `result_test.go`

**Interfaces:**
- Produz (usado por todas as tarefas seguintes):
  - `domain.Transaction{Seq int64, Step int, Type string, Amount float64, Orig string, OldBalanceOrig, NewBalanceOrig float64, Dest string, OldBalanceDest, NewBalanceDest float64, IsFraud bool}` com `ID() string`, `BalanceDrop() float64`, `DestIsMerchant() bool`.
  - `domain.ValidType(string) bool`, `domain.TransactionTypes []string`.
  - `domain.GraphFeatures`, `domain.FeatureQuery{Orig, Dest string, BeforeSeq int64, AtStep, WindowHours, CycleMaxHops int}`, `domain.Subgraph`, `domain.SubEdge`.
  - `domain.Decision{Noul float64, InputTokens, Attempts int, Latency time.Duration, Raw json.RawMessage}`, `domain.ResultsSink`.
  - `domain.Result`, `domain.NewResult(tx, gf *GraphFeatures) Result`.
  - `domain.Confusion` (`Add(isFraud, predictedFraud bool)`, `Accuracy/Precision/Recall/F1/FPR`), `domain.Counts`, `domain.Metrics`, `domain.ComputeMetrics(c, prevalence)`, `domain.PrecisionAtPrevalence`, `domain.Decide(noul, threshold) bool`, `domain.LatencyStats`, `domain.ComputeLatency([]float64)`.
  - `domain.RunParams`, `domain.RunSummary`, `domain.RunStatus` (`StatusRunning`, `StatusDone`, `StatusFailed`, `StatusCanceled`), `domain.SampleInfo`, `domain.LatencyBlock`, `domain.Progress`, `domain.ErrNotFound`.
  - `domain.DatasetStats`, `domain.TypeStats`.

- [ ] **Passo 1: criar o arquivo de casos compartilhado com o Python**

Estes casos são a fonte da verdade das fórmulas. O Go testa contra eles agora e o Python testa contra eles no plano 2.

```json file=testdata/metrics_golden.json
{
  "cases": [
    {
      "name": "caso_tipico",
      "tp": 8, "fp": 2, "tn": 88, "fn": 2,
      "prevalence": 0.01,
      "expected": {
        "accuracy": 0.96,
        "precision": 0.8,
        "recall": 0.8,
        "f1": 0.8,
        "fpr": 0.022222222222222223,
        "precision_at_prevalence": 0.26666666666666666
      }
    },
    {
      "name": "divisao_por_zero_vira_zero",
      "tp": 0, "fp": 0, "tn": 10, "fn": 0,
      "prevalence": 0.01,
      "expected": {
        "accuracy": 1.0,
        "precision": 0.0,
        "recall": 0.0,
        "f1": 0.0,
        "fpr": 0.0,
        "precision_at_prevalence": 0.0
      }
    },
    {
      "name": "tudo_previsto_como_fraude",
      "tp": 5, "fp": 5, "tn": 0, "fn": 0,
      "prevalence": 0.5,
      "expected": {
        "accuracy": 0.5,
        "precision": 0.5,
        "recall": 1.0,
        "f1": 0.6666666666666666,
        "fpr": 1.0,
        "precision_at_prevalence": 0.5
      }
    }
  ]
}
```

- [ ] **Passo 2: escrever os testes que falham**

```go file=internal/domain/transaction_test.go
package domain

import "testing"

func TestTransactionDerivados(t *testing.T) {
	tx := Transaction{Seq: 1029, OldBalanceOrig: 350000, NewBalanceOrig: 0, Dest: "C123"}
	if tx.ID() != "TX_1029" {
		t.Errorf("ID = %q", tx.ID())
	}
	if tx.BalanceDrop() != 350000 {
		t.Errorf("BalanceDrop = %v", tx.BalanceDrop())
	}
	if tx.DestIsMerchant() {
		t.Error("C123 não é comerciante")
	}
	tx.Dest = "M999"
	if !tx.DestIsMerchant() {
		t.Error("M999 é comerciante")
	}
}

func TestValidType(t *testing.T) {
	for _, ok := range []string{"PAYMENT", "TRANSFER", "CASH_OUT", "DEBIT", "CASH_IN"} {
		if !ValidType(ok) {
			t.Errorf("%s deveria ser válido", ok)
		}
	}
	for _, ruim := range []string{"", "transfer", "REFUND"} {
		if ValidType(ruim) {
			t.Errorf("%q não deveria ser válido", ruim)
		}
	}
}
```

```go file=internal/domain/metrics_test.go
package domain

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func perto(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

type casoGolden struct {
	Name       string  `json:"name"`
	TP         int     `json:"tp"`
	FP         int     `json:"fp"`
	TN         int     `json:"tn"`
	FN         int     `json:"fn"`
	Prevalence float64 `json:"prevalence"`
	Expected   Metrics `json:"expected"`
}

func TestMetricasBatemComArquivoCompartilhado(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "metrics_golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var arquivo struct {
		Cases []casoGolden `json:"cases"`
	}
	if err := json.Unmarshal(raw, &arquivo); err != nil {
		t.Fatal(err)
	}
	if len(arquivo.Cases) == 0 {
		t.Fatal("arquivo sem casos")
	}
	for _, c := range arquivo.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got := ComputeMetrics(Confusion{TP: c.TP, FP: c.FP, TN: c.TN, FN: c.FN}, c.Prevalence)
			e := c.Expected
			if !perto(got.Accuracy, e.Accuracy) || !perto(got.Precision, e.Precision) ||
				!perto(got.Recall, e.Recall) || !perto(got.F1, e.F1) ||
				!perto(got.FPR, e.FPR) || !perto(got.PrecisionAtPrevalence, e.PrecisionAtPrevalence) {
				t.Errorf("got %+v, esperado %+v", got, e)
			}
		})
	}
}

func TestConfusionAdd(t *testing.T) {
	var c Confusion
	c.Add(true, true)
	c.Add(true, false)
	c.Add(false, true)
	c.Add(false, false)
	c.Add(false, false)
	if c != (Confusion{TP: 1, FN: 1, FP: 1, TN: 2}) {
		t.Errorf("Confusion = %+v", c)
	}
	if c.Total() != 5 {
		t.Errorf("Total = %d", c.Total())
	}
}

func TestDecide(t *testing.T) {
	if !Decide(0.5, 0.5) {
		t.Error("no limiar exato deve ser fraude")
	}
	if Decide(0.49, 0.5) {
		t.Error("abaixo do limiar não é fraude")
	}
}

func TestPercentilEhExato(t *testing.T) {
	var de1a100 []float64
	for i := 1; i <= 100; i++ {
		de1a100 = append(de1a100, float64(i))
	}
	st := ComputeLatency(de1a100)
	if st.P50 != 50 || st.P95 != 95 || st.P99 != 99 {
		t.Errorf("1..100 = %+v", st)
	}
	quatro := ComputeLatency([]float64{40, 10, 30, 20})
	if quatro.P50 != 20 || quatro.P95 != 40 || quatro.P99 != 40 {
		t.Errorf("4 valores desordenados = %+v", quatro)
	}
	if vazio := ComputeLatency(nil); vazio != (LatencyStats{}) {
		t.Errorf("vazio = %+v", vazio)
	}
}

func TestComputeLatencyNaoAlteraEntrada(t *testing.T) {
	entrada := []float64{3, 1, 2}
	ComputeLatency(entrada)
	if entrada[0] != 3 || entrada[1] != 1 || entrada[2] != 2 {
		t.Errorf("entrada foi reordenada: %v", entrada)
	}
}
```

```go file=internal/domain/result_test.go
package domain

import (
	"encoding/json"
	"testing"
)

func TestResultSemGrafoNaoTemCamposDeGrafo(t *testing.T) {
	tx := Transaction{Seq: 7, Step: 3, Type: "TRANSFER", Orig: "C1", Dest: "C2", Amount: 10, IsFraud: true}
	raw, err := json.Marshal(NewResult(tx, nil))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["transaction_id"] != "TX_7" || m["is_fraud"] != true || m["orig"] != "C1" {
		t.Errorf("campos básicos errados: %v", m)
	}
	if _, existe := m["dest_in_degree"]; existe {
		t.Error("sem grafo não deveria ter dest_in_degree")
	}
	if m["noul"] != nil {
		t.Errorf("noul deveria ser null, veio %v", m["noul"])
	}
}

func TestResultComGrafoAchataOsCampos(t *testing.T) {
	idade := 29
	gf := &GraphFeatures{DestInDegree: 4, CycleDetected: true, DestAccountAgeSteps: &idade}
	raw, err := json.Marshal(NewResult(Transaction{Seq: 1}, gf))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["dest_in_degree"] != float64(4) || m["cycle_detected"] != true || m["dest_account_age_steps"] != float64(29) {
		t.Errorf("features achatadas erradas: %v", m)
	}

	semIdade := &GraphFeatures{}
	raw, _ = json.Marshal(NewResult(Transaction{Seq: 2}, semIdade))
	m = map[string]any{}
	_ = json.Unmarshal(raw, &m)
	if v, existe := m["dest_account_age_steps"]; !existe || v != nil {
		t.Errorf("dest_account_age_steps deveria existir e ser null, veio %v (existe=%v)", v, existe)
	}
}

func TestResultVoltaDoJSON(t *testing.T) {
	idade := 5
	original := NewResult(Transaction{Seq: 9, Type: "CASH_OUT"}, &GraphFeatures{DestFanInWindow: 3, DestAccountAgeSteps: &idade})
	noul := 0.75
	original.Noul = &noul
	raw, _ := json.Marshal(original)

	var volta Result
	if err := json.Unmarshal(raw, &volta); err != nil {
		t.Fatal(err)
	}
	if volta.GraphFeatures == nil || volta.DestFanInWindow != 3 || *volta.DestAccountAgeSteps != 5 || *volta.Noul != 0.75 {
		t.Errorf("volta = %+v", volta)
	}
}
```

- [ ] **Passo 3: rodar e ver falhar**

Rode: `go test ./internal/domain/`
Esperado: FAIL com erros de compilação como `undefined: Transaction`.

- [ ] **Passo 4: implementar os tipos**

```go file=internal/domain/transaction.go
package domain

import (
	"fmt"
	"slices"
	"strings"
)

// TransactionTypes são os cinco tipos do PaySim.
var TransactionTypes = []string{"PAYMENT", "TRANSFER", "CASH_OUT", "DEBIT", "CASH_IN"}

// ValidType diz se o tipo existe no PaySim (a comparação é exata).
func ValidType(t string) bool {
	return slices.Contains(TransactionTypes, t)
}

// Transaction é uma linha do CSV. Seq é o número da linha de dados, começando
// em 0, e funciona como a régua do tempo: menor Seq significa que chegou antes.
type Transaction struct {
	Seq            int64
	Step           int
	Type           string
	Amount         float64
	Orig           string
	OldBalanceOrig float64
	NewBalanceOrig float64
	Dest           string
	OldBalanceDest float64
	NewBalanceDest float64
	IsFraud        bool
}

// ID é o identificador que o PaySim não tem, derivado do Seq.
func (t Transaction) ID() string { return fmt.Sprintf("TX_%d", t.Seq) }

// BalanceDrop é quanto o saldo da origem caiu.
func (t Transaction) BalanceDrop() float64 { return t.OldBalanceOrig - t.NewBalanceOrig }

// DestIsMerchant diz se o destino é um comerciante (o nome começa com "M").
func (t Transaction) DestIsMerchant() bool { return strings.HasPrefix(t.Dest, "M") }
```

```go file=internal/domain/features.go
package domain

// GraphFeatures são as features de rede calculadas no Neo4j, sempre usando só
// o passado da transação. DestAccountAgeSteps é nil quando a conta nunca
// apareceu antes.
type GraphFeatures struct {
	FirstTransferBetweenPair bool `json:"first_transfer_between_pair"`
	DestInDegree             int  `json:"dest_in_degree"`
	DestFanInWindow          int  `json:"dest_fan_in_window"`
	DestFanOutWindow         int  `json:"dest_fan_out_window"`
	OrigFanOutWindow         int  `json:"orig_fan_out_window"`
	DestAccountAgeSteps      *int `json:"dest_account_age_steps"`
	CycleDetected            bool `json:"cycle_detected"`
}

// FeatureQuery diz para o Neo4j o que ele pode enxergar. Só entram arestas com
// seq menor que BeforeSeq e step menor ou igual a AtStep. Para uma transação
// hipotética "de agora", use BeforeSeq = math.MaxInt64.
type FeatureQuery struct {
	Orig         string
	Dest         string
	BeforeSeq    int64
	AtStep       int
	WindowHours  int
	CycleMaxHops int
}

// SubEdge é uma aresta do subgrafo mostrado na Aba 3. IsFraud serve só para
// pintar a aresta, nunca para calcular feature.
type SubEdge struct {
	ID      string  `json:"id"`
	Src     string  `json:"src"`
	Dst     string  `json:"dst"`
	Type    string  `json:"type"`
	Amount  float64 `json:"amount"`
	Step    int     `json:"step"`
	IsFraud bool    `json:"is_fraud"`
}

// Subgraph é a vizinhança de 1 salto da origem e do destino.
type Subgraph struct {
	Nodes []string  `json:"nodes"`
	Edges []SubEdge `json:"edges"`
}
```

```go file=internal/domain/decision.go
package domain

import (
	"encoding/json"
	"time"
)

// Decision é a resposta do modelo para a pergunta noul. Latency é o tempo só
// da tentativa que deu certo. Raw guarda o corpo bruto da resposta.
type Decision struct {
	Noul        float64
	InputTokens int
	Attempts    int
	Latency     time.Duration
	Raw         json.RawMessage
}

// ResultsSink recebe um Result por transação processada.
type ResultsSink interface {
	Write(Result) error
	Close() error
}
```

```go file=internal/domain/result.go
package domain

// Result é uma linha do results.jsonl. Os campos de grafo vêm achatados pelo
// GraphFeatures embutido e somem do JSON quando a execução foi sem grafo.
type Result struct {
	TransactionID string   `json:"transaction_id"`
	Seq           int64    `json:"seq"`
	Step          int      `json:"step"`
	Type          string   `json:"type"`
	Orig          string   `json:"orig"`
	Dest          string   `json:"dest"`
	IsFraud       bool     `json:"is_fraud"`
	Noul          *float64 `json:"noul"`
	Neo4jMs       *float64 `json:"neo4j_ms"`
	JevMs         *float64 `json:"jev_ms"`
	Attempts      int      `json:"attempts"`
	InputTokens   int      `json:"input_tokens"`
	Error         string   `json:"error,omitempty"`

	Amount            float64 `json:"amount"`
	OrigBalanceBefore float64 `json:"orig_balance_before"`
	OrigBalanceAfter  float64 `json:"orig_balance_after"`
	DestBalanceBefore float64 `json:"dest_balance_before"`
	DestBalanceAfter  float64 `json:"dest_balance_after"`
	BalanceDrop       float64 `json:"balance_drop"`
	DestIsMerchant    bool    `json:"dest_is_merchant"`

	*GraphFeatures
}

// NewResult monta a parte fixa do resultado a partir da transação.
func NewResult(tx Transaction, gf *GraphFeatures) Result {
	return Result{
		TransactionID:     tx.ID(),
		Seq:               tx.Seq,
		Step:              tx.Step,
		Type:              tx.Type,
		Orig:              tx.Orig,
		Dest:              tx.Dest,
		IsFraud:           tx.IsFraud,
		Amount:            tx.Amount,
		OrigBalanceBefore: tx.OldBalanceOrig,
		OrigBalanceAfter:  tx.NewBalanceOrig,
		DestBalanceBefore: tx.OldBalanceDest,
		DestBalanceAfter:  tx.NewBalanceDest,
		BalanceDrop:       tx.BalanceDrop(),
		DestIsMerchant:    tx.DestIsMerchant(),
		GraphFeatures:     gf,
	}
}
```

```go file=internal/domain/metrics.go
package domain

import (
	"math"
	"slices"
)

// Confusion é a matriz de confusão. Fraude é a classe positiva.
type Confusion struct {
	TP int `json:"tp"`
	FP int `json:"fp"`
	TN int `json:"tn"`
	FN int `json:"fn"`
}

// Counts é a matriz mais a quantidade de transações que deram erro.
type Counts struct {
	Confusion
	Errors int `json:"errors"`
}

// Add registra uma transação na matriz.
func (c *Confusion) Add(isFraud, predictedFraud bool) {
	switch {
	case isFraud && predictedFraud:
		c.TP++
	case isFraud && !predictedFraud:
		c.FN++
	case !isFraud && predictedFraud:
		c.FP++
	default:
		c.TN++
	}
}

func (c Confusion) Total() int { return c.TP + c.FP + c.TN + c.FN }

// Todas as razões devolvem 0 quando o denominador é 0. O Python usa a mesma
// convenção (veja testdata/metrics_golden.json).
func ratio(num, den int) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func (c Confusion) Accuracy() float64  { return ratio(c.TP+c.TN, c.Total()) }
func (c Confusion) Precision() float64 { return ratio(c.TP, c.TP+c.FP) }
func (c Confusion) Recall() float64    { return ratio(c.TP, c.TP+c.FN) }
func (c Confusion) FPR() float64       { return ratio(c.FP, c.FP+c.TN) }

func (c Confusion) F1() float64 {
	p, r := c.Precision(), c.Recall()
	if p+r == 0 {
		return 0
	}
	return 2 * p * r / (p + r)
}

// PrecisionAtPrevalence reprojeta a precisão para outra proporção de fraude.
// TPR e FPR não mudam com a estratificação, então dá para trocar só o pi.
func PrecisionAtPrevalence(tpr, fpr, prevalence float64) float64 {
	den := tpr*prevalence + fpr*(1-prevalence)
	if den == 0 {
		return 0
	}
	return tpr * prevalence / den
}

// Metrics são as métricas de classificação de uma execução.
type Metrics struct {
	Accuracy              float64 `json:"accuracy"`
	Precision             float64 `json:"precision"`
	Recall                float64 `json:"recall"`
	F1                    float64 `json:"f1"`
	FPR                   float64 `json:"fpr"`
	PrecisionAtPrevalence float64 `json:"precision_at_prevalence"`
}

// ComputeMetrics calcula tudo a partir da matriz. prevalence é a proporção
// real de fraude no universo elegível.
func ComputeMetrics(c Confusion, prevalence float64) Metrics {
	return Metrics{
		Accuracy:              c.Accuracy(),
		Precision:             c.Precision(),
		Recall:                c.Recall(),
		F1:                    c.F1(),
		FPR:                   c.FPR(),
		PrecisionAtPrevalence: PrecisionAtPrevalence(c.Recall(), c.FPR(), prevalence),
	}
}

// Decide aplica o limiar. No limiar exato conta como fraude.
func Decide(noul, threshold float64) bool { return noul >= threshold }

// LatencyStats são os percentis de latência em milissegundos.
type LatencyStats struct {
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
}

// percentile usa o método do posto mais próximo sobre valores já ordenados.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	idx := min(max(rank-1, 0), len(sorted)-1)
	return sorted[idx]
}

// ComputeLatency calcula P50, P95 e P99 de forma exata, sem mexer na entrada.
func ComputeLatency(values []float64) LatencyStats {
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	return LatencyStats{
		P50: percentile(sorted, 50),
		P95: percentile(sorted, 95),
		P99: percentile(sorted, 99),
	}
}
```

```go file=internal/domain/run.go
package domain

import (
	"errors"
	"time"
)

// ErrNotFound é devolvido quando uma execução ou arquivo não existe.
var ErrNotFound = errors.New("não encontrado")

// RunStatus é o estado de uma execução de benchmark.
type RunStatus string

const (
	StatusRunning  RunStatus = "running"
	StatusDone     RunStatus = "done"
	StatusFailed   RunStatus = "failed"
	StatusCanceled RunStatus = "canceled"
)

// RunParams são os parâmetros de uma execução. BaseURL é preenchida pelo
// servidor e nunca vem do pedido.
type RunParams struct {
	N            int      `json:"n"`
	FraudPct     float64  `json:"fraud_pct"`
	Seed         uint64   `json:"seed"`
	Workers      int      `json:"workers"`
	Model        string   `json:"model"`
	BaseURL      string   `json:"base_url"`
	Graph        bool     `json:"graph"`
	Types        []string `json:"types"`
	WindowHours  int      `json:"window_hours"`
	CycleMaxHops int      `json:"cycle_max_hops"`
	MaxInflight  int      `json:"max_inflight"`
	Threshold    float64  `json:"threshold"`
}

// SampleInfo descreve a amostra e o universo de onde ela saiu.
type SampleInfo struct {
	Requested      int     `json:"requested"`
	N              int     `json:"n"`
	NFraud         int     `json:"n_fraud"`
	NLegit         int     `json:"n_legit"`
	FraudShortfall bool    `json:"fraud_shortfall"`
	EligibleRows   int64   `json:"eligible_rows"`
	EligibleFraud  int64   `json:"eligible_fraud"`
	Prevalence     float64 `json:"prevalence"`
}

// LatencyBlock junta os percentis do Neo4j e do modelo.
type LatencyBlock struct {
	Neo4j LatencyStats `json:"neo4j_ms"`
	Jev   LatencyStats `json:"jev_ms"`
}

// Progress é o andamento de uma execução ativa.
type Progress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// RunSummary é o conteúdo do summary.json.
type RunSummary struct {
	RunID       string        `json:"run_id"`
	Status      RunStatus     `json:"status"`
	Error       string        `json:"error,omitempty"`
	StartedAt   time.Time     `json:"started_at"`
	FinishedAt  *time.Time    `json:"finished_at,omitempty"`
	DurationS   float64       `json:"duration_s"`
	Params      RunParams     `json:"params"`
	Sample      *SampleInfo   `json:"sample,omitempty"`
	Counts      *Counts       `json:"counts,omitempty"`
	Metrics     *Metrics      `json:"metrics,omitempty"`
	Latency     *LatencyBlock `json:"latency,omitempty"`
	TPS         float64       `json:"tps"`
	TokensTotal int64         `json:"tokens_total"`
	Progress    *Progress     `json:"progress,omitempty"`
}
```

```go file=internal/domain/stats.go
package domain

// TypeStats são as contagens de um tipo de transação.
type TypeStats struct {
	Rows   int64 `json:"rows"`
	Frauds int64 `json:"frauds"`
}

// DatasetStats é o conteúdo do dataset_stats.json, gravado pelo seeder.
type DatasetStats struct {
	TotalRows int64                `json:"total_rows"`
	FraudRows int64                `json:"fraud_rows"`
	MaxStep   int                  `json:"max_step"`
	ByType    map[string]TypeStats `json:"by_type"`
}
```

- [ ] **Passo 5: rodar e ver passar**

Rode: `go test -race ./internal/domain/`
Esperado: `ok  fraud-detector-jev/internal/domain`.

- [ ] **Passo 6: commitar**

```bash
git add internal/domain testdata/metrics_golden.json
git commit -m "feat: adiciona tipos do domínio, métricas e casos compartilhados"
```

---
## Tarefa 3: leitor CSV em streaming

**Arquivos:**
- Criar: `internal/infrastructure/csv/streaming_reader.go`
- Testar: `internal/infrastructure/csv/streaming_reader_test.go`

**Interfaces:**
- Consumes: `domain.Transaction`.
- Produz:
  - `csv.Stream(ctx context.Context, path string, fn func(domain.Transaction) error) error`
  - `csv.StreamReader(ctx context.Context, r io.Reader, fn func(domain.Transaction) error) error`
  - `csv.ErrUnsortedSteps` (erro sentinela, use `errors.Is`)

O leitor nunca carrega o arquivo inteiro: lê uma linha, monta a `Transaction`, chama `fn` e segue. `Seq` começa em 0 na primeira linha de dados. Se o `step` diminuir, devolve `ErrUnsortedSteps`, porque o `seq` só serve como régua do tempo se o arquivo estiver ordenado por `step`.

- [ ] **Passo 1: escrever o teste que falha**

```go file=internal/infrastructure/csv/streaming_reader_test.go
package csv

import (
	"context"
	"errors"
	"strings"
	"testing"

	"fraud-detector-jev/internal/domain"
)

const cabecalho = "step,type,amount,nameOrig,oldbalanceOrg,newbalanceOrig,nameDest,oldbalanceDest,newbalanceDest,isFraud,isFlaggedFraud\n"

func coletar(t *testing.T, conteudo string) ([]domain.Transaction, error) {
	t.Helper()
	var txs []domain.Transaction
	err := StreamReader(context.Background(), strings.NewReader(conteudo), func(tx domain.Transaction) error {
		txs = append(txs, tx)
		return nil
	})
	return txs, err
}

func TestStreamLeTransacoesEmOrdem(t *testing.T) {
	txs, err := coletar(t, cabecalho+
		"1,TRANSFER,100.5,C1,500.0,399.5,C2,0.0,100.5,0,0\n"+
		"1,CASH_OUT,20,C3,20,0,M9,5,25,1,0\n"+
		"2,PAYMENT,7.25,C4,10,2.75,M8,0,0,0,0\n")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(txs) != 3 {
		t.Fatalf("esperava 3 transações, vieram %d", len(txs))
	}
	primeira := txs[0]
	if primeira.Seq != 0 || primeira.Step != 1 || primeira.Type != "TRANSFER" || primeira.Amount != 100.5 ||
		primeira.Orig != "C1" || primeira.OldBalanceOrig != 500 || primeira.NewBalanceOrig != 399.5 ||
		primeira.Dest != "C2" || primeira.OldBalanceDest != 0 || primeira.NewBalanceDest != 100.5 || primeira.IsFraud {
		t.Errorf("primeira transação errada: %+v", primeira)
	}
	if txs[1].Seq != 1 || !txs[1].IsFraud || txs[2].Seq != 2 || txs[2].Step != 2 {
		t.Errorf("seq ou fraude errados: %+v %+v", txs[1], txs[2])
	}
}

func TestStreamToleraCRLFELinhaEmBrancoNoFim(t *testing.T) {
	conteudo := strings.ReplaceAll(cabecalho+
		"1,TRANSFER,10,C1,10,0,C2,0,10,0,0\n"+
		"1,TRANSFER,20,C1,10,0,C2,0,10,1,0\n"+
		"\n", "\n", "\r\n")
	txs, err := coletar(t, conteudo)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(txs) != 2 {
		t.Fatalf("esperava 2 transações e nenhuma fantasma, vieram %d", len(txs))
	}
}

func TestStreamRejeitaCabecalhoErrado(t *testing.T) {
	_, err := coletar(t, "a,b,c\n1,2,3\n")
	if err == nil || !strings.Contains(err.Error(), "cabeçalho") {
		t.Fatalf("esperava erro de cabeçalho, veio %v", err)
	}
}

func TestStreamRejeitaLinhaComCampoFaltando(t *testing.T) {
	_, err := coletar(t, cabecalho+"1,TRANSFER,10,C1,10,0,C2,0,10\n")
	if err == nil || !strings.Contains(err.Error(), "linha 2") {
		t.Fatalf("esperava erro citando a linha 2, veio %v", err)
	}
}

func TestStreamRejeitaNumeroInvalido(t *testing.T) {
	_, err := coletar(t, cabecalho+"1,TRANSFER,dez,C1,10,0,C2,0,10,0,0\n")
	if err == nil || !strings.Contains(err.Error(), "amount") || !strings.Contains(err.Error(), "linha 2") {
		t.Fatalf("esperava erro citando amount e a linha 2, veio %v", err)
	}
	_, err = coletar(t, cabecalho+"1,TRANSFER,10,C1,10,0,C2,0,10,talvez,0\n")
	if err == nil || !strings.Contains(err.Error(), "isFraud") {
		t.Fatalf("esperava erro citando isFraud, veio %v", err)
	}
}

func TestStreamDetectaArquivoForaDeOrdem(t *testing.T) {
	_, err := coletar(t, cabecalho+
		"5,TRANSFER,10,C1,10,0,C2,0,10,0,0\n"+
		"4,TRANSFER,10,C1,10,0,C2,0,10,0,0\n")
	if !errors.Is(err, ErrUnsortedSteps) {
		t.Fatalf("esperava ErrUnsortedSteps, veio %v", err)
	}
	if !strings.Contains(err.Error(), "linha 3") {
		t.Errorf("erro deveria citar a linha 3: %v", err)
	}
}

func TestStreamPropagaErroDoCallbackEPara(t *testing.T) {
	falha := errors.New("parei")
	chamadas := 0
	err := StreamReader(context.Background(), strings.NewReader(cabecalho+
		"1,TRANSFER,10,C1,10,0,C2,0,10,0,0\n"+
		"1,TRANSFER,10,C1,10,0,C2,0,10,0,0\n"), func(domain.Transaction) error {
		chamadas++
		return falha
	})
	if !errors.Is(err, falha) || chamadas != 1 {
		t.Fatalf("err=%v chamadas=%d", err, chamadas)
	}
}

func TestStreamRespeitaContextoCancelado(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := StreamReader(ctx, strings.NewReader(cabecalho+"1,TRANSFER,10,C1,10,0,C2,0,10,0,0\n"), func(domain.Transaction) error {
		t.Fatal("não deveria chamar o callback")
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("esperava context.Canceled, veio %v", err)
	}
}

func TestStreamAbreArquivoInexistente(t *testing.T) {
	err := Stream(context.Background(), "/nao/existe.csv", func(domain.Transaction) error { return nil })
	if err == nil {
		t.Fatal("esperava erro")
	}
}
```

- [ ] **Passo 2: rodar e ver falhar**

Rode: `go test ./internal/infrastructure/csv/`
Esperado: FAIL com `undefined: StreamReader`.

- [ ] **Passo 3: implementar**

```go file=internal/infrastructure/csv/streaming_reader.go
package csv

import (
	"bufio"
	"context"
	stdcsv "encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"fraud-detector-jev/internal/domain"
)

// ErrUnsortedSteps aparece quando o step diminui no meio do arquivo. O seq só
// serve como régua do tempo se o CSV estiver ordenado por step.
var ErrUnsortedSteps = errors.New("csv fora de ordem, o step diminuiu")

// As dez primeiras colunas são obrigatórias e precisam estar nesta ordem. A
// coluna isFlaggedFraud, se existir, é ignorada.
var colunasObrigatorias = []string{
	"step", "type", "amount", "nameOrig", "oldbalanceOrg", "newbalanceOrig",
	"nameDest", "oldbalanceDest", "newbalanceDest", "isFraud",
}

// Stream abre o arquivo e chama fn para cada transação, na ordem do arquivo.
func Stream(ctx context.Context, path string, fn func(domain.Transaction) error) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("abrindo o CSV: %w", err)
	}
	defer f.Close()
	return StreamReader(ctx, f, fn)
}

// StreamReader é o Stream sobre qualquer io.Reader. Lê uma linha por vez, então
// a memória não cresce com o tamanho do arquivo.
func StreamReader(ctx context.Context, r io.Reader, fn func(domain.Transaction) error) error {
	cr := stdcsv.NewReader(bufio.NewReaderSize(r, 1<<20))
	cr.ReuseRecord = true

	header, err := cr.Read()
	if err != nil {
		return fmt.Errorf("lendo cabeçalho: %w", err)
	}
	if err := conferirCabecalho(header); err != nil {
		return err
	}
	cr.FieldsPerRecord = len(header)

	var seq int64
	lastStep := -1
	for {
		if seq%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			var pe *stdcsv.ParseError
			if errors.As(err, &pe) {
				return fmt.Errorf("linha %d: %w", pe.Line, err)
			}
			return fmt.Errorf("lendo o CSV: %w", err)
		}
		line, _ := cr.FieldPos(0)

		tx, err := parseRecord(seq, rec)
		if err != nil {
			return fmt.Errorf("linha %d: %w", line, err)
		}
		if tx.Step < lastStep {
			return fmt.Errorf("linha %d: %w (step %d depois de %d)", line, ErrUnsortedSteps, tx.Step, lastStep)
		}
		lastStep = tx.Step

		if err := fn(tx); err != nil {
			return err
		}
		seq++
	}
}

func conferirCabecalho(header []string) error {
	if len(header) < len(colunasObrigatorias) {
		return fmt.Errorf("cabeçalho com %d colunas, esperava pelo menos %d", len(header), len(colunasObrigatorias))
	}
	for i, nome := range colunasObrigatorias {
		if header[i] != nome {
			return fmt.Errorf("cabeçalho errado na coluna %d: esperava %q e veio %q", i+1, nome, header[i])
		}
	}
	return nil
}

func parseRecord(seq int64, rec []string) (domain.Transaction, error) {
	tx := domain.Transaction{Seq: seq, Type: rec[1], Orig: rec[3], Dest: rec[6]}

	step, err := strconv.Atoi(rec[0])
	if err != nil {
		return tx, fmt.Errorf("campo step inválido %q", rec[0])
	}
	tx.Step = step

	if tx.Amount, err = parseFloat("amount", rec[2]); err != nil {
		return tx, err
	}
	if tx.OldBalanceOrig, err = parseFloat("oldbalanceOrg", rec[4]); err != nil {
		return tx, err
	}
	if tx.NewBalanceOrig, err = parseFloat("newbalanceOrig", rec[5]); err != nil {
		return tx, err
	}
	if tx.OldBalanceDest, err = parseFloat("oldbalanceDest", rec[7]); err != nil {
		return tx, err
	}
	if tx.NewBalanceDest, err = parseFloat("newbalanceDest", rec[8]); err != nil {
		return tx, err
	}

	switch rec[9] {
	case "0":
		tx.IsFraud = false
	case "1":
		tx.IsFraud = true
	default:
		return tx, fmt.Errorf("campo isFraud inválido %q", rec[9])
	}
	return tx, nil
}

func parseFloat(nome, bruto string) (float64, error) {
	v, err := strconv.ParseFloat(bruto, 64)
	if err != nil {
		return 0, fmt.Errorf("campo %s inválido %q", nome, bruto)
	}
	return v, nil
}
```

- [ ] **Passo 4: rodar e ver passar**

Rode: `go test -race ./internal/infrastructure/csv/`
Esperado: `ok`.

- [ ] **Passo 5: commitar**

```bash
git add internal/infrastructure/csv
git commit -m "feat: adiciona leitor de CSV em streaming com conferência de ordem"
```

---

## Tarefa 4: amostragem estratificada

**Arquivos:**
- Criar: `internal/service/sampler.go`
- Testar: `internal/service/sampler_test.go`

**Interfaces:**
- Consumes: `domain.Transaction`.
- Produz:
  - `service.TxSource` = `func(ctx context.Context, fn func(domain.Transaction) error) error`. O `main` liga isso ao `csv.Stream`.
  - `service.SampleParams{N int, FraudPct float64, Seed uint64, Types []string}`
  - `service.SampleResult{Transactions []domain.Transaction (ordenadas por Seq), NFraud, NLegit int, FraudShortfall bool, EligibleRows, EligibleFraud int64, Prevalence float64}`
  - `service.Sample(ctx, src TxSource, p SampleParams) (SampleResult, error)`
  - `service.ErrEmptySample`

Regras: uma passada só na fonte. Só entram tipos de `Types`. Pedem `round(N * FraudPct / 100)` fraudes e o resto de legítimas. Se faltar fraude, usa todas as que existem e completa com legítimas até `N`, se houver. Mesma seed e mesmos dados dão a mesma amostra.

- [ ] **Passo 1: escrever o teste que falha**

```go file=internal/service/sampler_test.go
package service

import (
	"context"
	"errors"
	"slices"
	"testing"

	"fraud-detector-jev/internal/domain"
)

// geraTransacoes cria linhas sintéticas. A cada fraudeACada linhas elegíveis
// uma é fraude. Os tipos se alternam entre TRANSFER, CASH_OUT e PAYMENT.
func geraTransacoes(total, fraudeACada int) []domain.Transaction {
	tipos := []string{"TRANSFER", "CASH_OUT", "PAYMENT"}
	var txs []domain.Transaction
	for i := range total {
		tx := domain.Transaction{Seq: int64(i), Step: i / 10, Type: tipos[i%3]}
		if fraudeACada > 0 && tx.Type != "PAYMENT" && i%fraudeACada == 0 {
			tx.IsFraud = true
		}
		txs = append(txs, tx)
	}
	return txs
}

func fonte(txs []domain.Transaction) TxSource {
	return func(ctx context.Context, fn func(domain.Transaction) error) error {
		for _, tx := range txs {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := fn(tx); err != nil {
				return err
			}
		}
		return nil
	}
}

var tiposPadrao = []string{"TRANSFER", "CASH_OUT"}

func contar(txs []domain.Transaction) (fraudes, legitimas int) {
	for _, tx := range txs {
		if tx.IsFraud {
			fraudes++
		} else {
			legitimas++
		}
	}
	return
}

func TestSampleRespeitaProporcaoETipos(t *testing.T) {
	txs := geraTransacoes(3000, 20)
	res, err := Sample(context.Background(), fonte(txs), SampleParams{N: 100, FraudPct: 10, Seed: 42, Types: tiposPadrao})
	if err != nil {
		t.Fatal(err)
	}
	fraudes, legitimas := contar(res.Transactions)
	if fraudes != 10 || legitimas != 90 || res.NFraud != 10 || res.NLegit != 90 {
		t.Errorf("fraudes=%d legítimas=%d (res %d/%d)", fraudes, legitimas, res.NFraud, res.NLegit)
	}
	for _, tx := range res.Transactions {
		if tx.Type == "PAYMENT" {
			t.Fatalf("tipo não elegível entrou: %+v", tx)
		}
	}
	if !slices.IsSortedFunc(res.Transactions, func(a, b domain.Transaction) int { return int(a.Seq - b.Seq) }) {
		t.Error("amostra deveria vir ordenada por Seq")
	}
	if res.FraudShortfall {
		t.Error("não faltou fraude")
	}
}

func TestSampleContaUniversoEPrevalenciaSoDosTiposElegiveis(t *testing.T) {
	txs := geraTransacoes(3000, 20)
	res, err := Sample(context.Background(), fonte(txs), SampleParams{N: 50, FraudPct: 10, Seed: 1, Types: tiposPadrao})
	if err != nil {
		t.Fatal(err)
	}
	// 2000 linhas são TRANSFER ou CASH_OUT, e as fraudes são as posições
	// multiplas de 20 que não caem em PAYMENT.
	var esperadoLinhas, esperadoFraudes int64
	for _, tx := range txs {
		if tx.Type != "PAYMENT" {
			esperadoLinhas++
			if tx.IsFraud {
				esperadoFraudes++
			}
		}
	}
	if res.EligibleRows != esperadoLinhas || res.EligibleFraud != esperadoFraudes {
		t.Errorf("universo %d/%d, esperado %d/%d", res.EligibleRows, res.EligibleFraud, esperadoLinhas, esperadoFraudes)
	}
	want := float64(esperadoFraudes) / float64(esperadoLinhas)
	if res.Prevalence != want {
		t.Errorf("prevalência %v, esperada %v", res.Prevalence, want)
	}
}

func TestSampleEhDeterministicaPorSeed(t *testing.T) {
	txs := geraTransacoes(5000, 15)
	pegar := func(seed uint64) []int64 {
		res, err := Sample(context.Background(), fonte(txs), SampleParams{N: 200, FraudPct: 20, Seed: seed, Types: tiposPadrao})
		if err != nil {
			t.Fatal(err)
		}
		var seqs []int64
		for _, tx := range res.Transactions {
			seqs = append(seqs, tx.Seq)
		}
		return seqs
	}
	if !slices.Equal(pegar(7), pegar(7)) {
		t.Error("mesma seed deveria dar a mesma amostra")
	}
	if slices.Equal(pegar(7), pegar(8)) {
		t.Error("seeds diferentes deveriam dar amostras diferentes")
	}
}

func TestSampleUsaTodasAsFraudesQuandoFaltaEComplementaComLegitimas(t *testing.T) {
	txs := geraTransacoes(3000, 400) // poucas fraudes
	fraudesExistentes, _ := contar(slices.DeleteFunc(slices.Clone(txs), func(tx domain.Transaction) bool { return tx.Type == "PAYMENT" }))
	res, err := Sample(context.Background(), fonte(txs), SampleParams{N: 100, FraudPct: 50, Seed: 3, Types: tiposPadrao})
	if err != nil {
		t.Fatal(err)
	}
	fraudes, legitimas := contar(res.Transactions)
	if fraudes != fraudesExistentes || !res.FraudShortfall {
		t.Errorf("fraudes=%d (existem %d) shortfall=%v", fraudes, fraudesExistentes, res.FraudShortfall)
	}
	if fraudes+legitimas != 100 {
		t.Errorf("total = %d, deveria completar 100 com legítimas", fraudes+legitimas)
	}
}

func TestSampleFraudPctZeroECemENMaiorQueOUniverso(t *testing.T) {
	txs := geraTransacoes(300, 10)
	casos := []struct {
		nome     string
		p        SampleParams
		fraudes  func(int) bool
		total    func(int) bool
		shortage bool
	}{
		{"0 por cento", SampleParams{N: 50, FraudPct: 0, Seed: 1, Types: tiposPadrao}, func(f int) bool { return f == 0 }, func(n int) bool { return n == 50 }, false},
		{"100 por cento", SampleParams{N: 50, FraudPct: 100, Seed: 1, Types: tiposPadrao}, func(f int) bool { return f > 0 && f <= 50 }, func(n int) bool { return n == 50 }, false},
		{"N maior que o universo", SampleParams{N: 100000, FraudPct: 10, Seed: 1, Types: tiposPadrao}, func(f int) bool { return f > 0 }, func(n int) bool { return n == 200 }, true},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			res, err := Sample(context.Background(), fonte(txs), c.p)
			if err != nil {
				t.Fatal(err)
			}
			fraudes, _ := contar(res.Transactions)
			if !c.fraudes(fraudes) || !c.total(len(res.Transactions)) {
				t.Errorf("fraudes=%d total=%d", fraudes, len(res.Transactions))
			}
			vistas := map[int64]bool{}
			for _, tx := range res.Transactions {
				if vistas[tx.Seq] {
					t.Fatalf("transação repetida: %d", tx.Seq)
				}
				vistas[tx.Seq] = true
			}
		})
	}
}

func TestSampleUniversoVazioDaErro(t *testing.T) {
	txs := geraTransacoes(30, 5)
	_, err := Sample(context.Background(), fonte(txs), SampleParams{N: 10, FraudPct: 10, Seed: 1, Types: []string{"DEBIT"}})
	if !errors.Is(err, ErrEmptySample) {
		t.Fatalf("esperava ErrEmptySample, veio %v", err)
	}
}

func TestSampleValidaParametros(t *testing.T) {
	ruins := []SampleParams{
		{N: 0, FraudPct: 10, Types: tiposPadrao},
		{N: 10, FraudPct: -1, Types: tiposPadrao},
		{N: 10, FraudPct: 101, Types: tiposPadrao},
		{N: 10, FraudPct: 10},
	}
	for _, p := range ruins {
		if _, err := Sample(context.Background(), fonte(nil), p); err == nil {
			t.Errorf("esperava erro para %+v", p)
		}
	}
}

func TestSamplePropagaErroDaFonteECancelamento(t *testing.T) {
	falha := errors.New("disco pegou fogo")
	src := TxSource(func(ctx context.Context, fn func(domain.Transaction) error) error { return falha })
	if _, err := Sample(context.Background(), src, SampleParams{N: 10, FraudPct: 10, Types: tiposPadrao}); !errors.Is(err, falha) {
		t.Errorf("esperava o erro da fonte, veio %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Sample(ctx, fonte(geraTransacoes(10, 2)), SampleParams{N: 10, FraudPct: 10, Types: tiposPadrao}); !errors.Is(err, context.Canceled) {
		t.Errorf("esperava context.Canceled, veio %v", err)
	}
}
```

- [ ] **Passo 2: rodar e ver falhar**

Rode: `go test ./internal/service/ -run Sample`
Esperado: FAIL com `undefined: Sample`.

- [ ] **Passo 3: implementar**

```go file=internal/service/sampler.go
package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"

	"fraud-detector-jev/internal/domain"
)

// ErrEmptySample aparece quando nenhuma transação dos tipos pedidos existe.
var ErrEmptySample = errors.New("nenhuma transação elegível para os tipos pedidos")

// TxSource entrega as transações uma a uma, em ordem. O main liga isso ao
// leitor de CSV, e os testes usam uma fatia em memória.
type TxSource func(ctx context.Context, fn func(domain.Transaction) error) error

// SampleParams define a amostra. FraudPct vai de 0 a 100.
type SampleParams struct {
	N        int
	FraudPct float64
	Seed     uint64
	Types    []string
}

// SampleResult traz a amostra e as contagens do universo elegível de onde ela
// saiu. Prevalence é EligibleFraud dividido por EligibleRows.
type SampleResult struct {
	Transactions   []domain.Transaction
	NFraud         int
	NLegit         int
	FraudShortfall bool
	EligibleRows   int64
	EligibleFraud  int64
	Prevalence     float64
}

func (p SampleParams) validate() error {
	switch {
	case p.N < 1:
		return fmt.Errorf("N precisa ser pelo menos 1, veio %d", p.N)
	case p.FraudPct < 0 || p.FraudPct > 100:
		return fmt.Errorf("FraudPct precisa estar entre 0 e 100, veio %v", p.FraudPct)
	case len(p.Types) == 0:
		return errors.New("Types não pode ser vazio")
	}
	return nil
}

// reservoir guarda uma amostra uniforme de tamanho k (algoritmo R).
type reservoir struct {
	k     int
	seen  int64
	items []domain.Transaction
	rng   *rand.Rand
}

func (r *reservoir) offer(tx domain.Transaction) {
	r.seen++
	if r.k == 0 {
		return
	}
	if len(r.items) < r.k {
		r.items = append(r.items, tx)
		return
	}
	if j := r.rng.Int64N(r.seen); j < int64(r.k) {
		r.items[j] = tx
	}
}

// Sample faz uma única passada na fonte e devolve a amostra estratificada.
func Sample(ctx context.Context, src TxSource, p SampleParams) (SampleResult, error) {
	if err := p.validate(); err != nil {
		return SampleResult{}, err
	}
	rng := rand.New(rand.NewPCG(p.Seed, p.Seed^0x9e3779b97f4a7c15))
	elegivel := make(map[string]bool, len(p.Types))
	for _, t := range p.Types {
		elegivel[t] = true
	}

	quero := int(math.Round(float64(p.N) * p.FraudPct / 100))
	fraudes := &reservoir{k: quero, rng: rng}
	// O reservatório de legítimas guarda até N, para poder completar a amostra
	// quando faltar fraude.
	legitimas := &reservoir{k: p.N, rng: rng}

	var res SampleResult
	err := src(ctx, func(tx domain.Transaction) error {
		if !elegivel[tx.Type] {
			return nil
		}
		res.EligibleRows++
		if tx.IsFraud {
			res.EligibleFraud++
			fraudes.offer(tx)
		} else {
			legitimas.offer(tx)
		}
		return nil
	})
	if err != nil {
		return SampleResult{}, err
	}
	if res.EligibleRows == 0 {
		return SampleResult{}, ErrEmptySample
	}

	res.NFraud = len(fraudes.items)
	res.FraudShortfall = res.NFraud < quero

	// O reservatório não é uma permutação uniforme, então embaralha antes de
	// cortar para completar só o que falta.
	rng.Shuffle(len(legitimas.items), func(i, j int) {
		legitimas.items[i], legitimas.items[j] = legitimas.items[j], legitimas.items[i]
	})
	res.NLegit = min(p.N-res.NFraud, len(legitimas.items))

	all := make([]domain.Transaction, 0, res.NFraud+res.NLegit)
	all = append(all, fraudes.items...)
	all = append(all, legitimas.items[:res.NLegit]...)
	slices.SortFunc(all, func(a, b domain.Transaction) int { return cmp.Compare(a.Seq, b.Seq) })

	res.Transactions = all
	res.Prevalence = float64(res.EligibleFraud) / float64(res.EligibleRows)
	return res, nil
}
```

- [ ] **Passo 4: rodar e ver passar**

Rode: `go test -race ./internal/service/ -run Sample -v`
Esperado: todos os `TestSample*` com PASS.

- [ ] **Passo 5: commitar**

```bash
git add internal/service/sampler.go internal/service/sampler_test.go
git commit -m "feat: adiciona amostragem estratificada com seed"
```

---
## Tarefa 5: cliente do modelo, lista de modelos e mock

**Arquivos:**
- Criar: `internal/infrastructure/jev/client.go`, `models.go`
- Criar: `internal/infrastructure/jev/jevtest/mock_server.go`
- Testar: `internal/infrastructure/jev/client_test.go`, `models_test.go`

**Interfaces:**
- Consumes: `domain.Decision`.
- Produz:
  - `jev.Config{BaseURL, Model, Instructions string, MaxInflight int, Timeout time.Duration, MaxAttempts int, BaseBackoff time.Duration}`
  - `jev.New(cfg Config) (*jev.Client, error)`
  - `(*jev.Client).Classify(ctx context.Context, state string) (domain.Decision, error)` e `(*jev.Client).Close()`
  - `jev.ErrInvalidResponse`, `*jev.StatusError{Code int, Body string}`
  - `jev.ListModels(ctx, baseURL string) ([]string, error)` e `jev.DecisionModels(all []string) []string`
  - `jevtest.New(jevtest.Options) *jevtest.Server` com `URL`, `Close()`, `Calls() int`, `MaxInflightSeen() int`, `LastBody() []byte`, e `jevtest.Raw(string) *string`.

Comportamento do cliente: uma requisição `POST {BaseURL}/v1/systemone` com `state` como string JSON e a pergunta `is_fraud` do tipo `noul`. `MaxInflight` limita as chamadas simultâneas (cada tentativa conta). Erro de rede, timeout e 5xx são tentados de novo com backoff exponencial e jitter, até `MaxAttempts`. 4xx e resposta inválida não são tentados de novo. `Decision.Latency` é só o tempo da tentativa que deu certo.

- [ ] **Passo 1: escrever o mock (é ferramenta de teste, usada por este e por outros pacotes)**

```go file=internal/infrastructure/jev/jevtest/mock_server.go
// Package jevtest tem um servidor falso do modelo, só para testes. Ele fica em
// um subpacote para o httptest não entrar no binário de produção.
package jevtest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"time"
)

// Options controla o comportamento do servidor falso.
type Options struct {
	// Score decide o noul a partir do state. Se for nil, usa a regra padrão:
	// amount acima de 200000 dá 0.9, senão 0.1.
	Score func(state string) float64
	// FailFirst faz as N primeiras requisições falharem com FailStatus.
	FailFirst int
	// FailStatus é o status das falhas simuladas. Padrão 503.
	FailStatus int
	// Delay espera antes de responder.
	Delay time.Duration
	// RawResponse, se não for nil, é devolvido como corpo em vez da resposta normal.
	RawResponse *string
}

// Raw devolve um ponteiro para s, para preencher Options.RawResponse.
func Raw(s string) *string { return &s }

// Server é o servidor falso.
type Server struct {
	*httptest.Server
	opts        Options
	calls       atomic.Int64
	inflight    atomic.Int64
	maxInflight atomic.Int64
	mu          sync.Mutex
	lastBody    []byte
}

// New sobe o servidor. Lembre de chamar Close.
func New(opts Options) *Server {
	s := &Server{opts: opts}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/systemone", s.handleSystemOne)
	mux.HandleFunc("GET /api/tags", s.handleTags)
	s.Server = httptest.NewServer(mux)
	return s
}

// Calls é o total de requisições ao /v1/systemone.
func (s *Server) Calls() int { return int(s.calls.Load()) }

// MaxInflightSeen é o maior número de requisições simultâneas já visto.
func (s *Server) MaxInflightSeen() int { return int(s.maxInflight.Load()) }

// LastBody é o corpo da última requisição ao /v1/systemone.
func (s *Server) LastBody() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.lastBody...)
}

func (s *Server) handleSystemOne(w http.ResponseWriter, r *http.Request) {
	n := s.calls.Add(1)
	cur := s.inflight.Add(1)
	defer s.inflight.Add(-1)
	for {
		seen := s.maxInflight.Load()
		if cur <= seen || s.maxInflight.CompareAndSwap(seen, cur) {
			break
		}
	}

	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.lastBody = body
	s.mu.Unlock()

	if s.opts.Delay > 0 {
		time.Sleep(s.opts.Delay)
	}
	if int(n) <= s.opts.FailFirst {
		status := s.opts.FailStatus
		if status == 0 {
			status = http.StatusServiceUnavailable
		}
		http.Error(w, "falha simulada", status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if s.opts.RawResponse != nil {
		_, _ = io.WriteString(w, *s.opts.RawResponse)
		return
	}

	var req struct {
		State string `json:"state"`
	}
	_ = json.Unmarshal(body, &req)
	score := defaultScore(req.State)
	if s.opts.Score != nil {
		score = s.opts.Score(req.State)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"model": "mock",
		"answers": map[string]any{
			"is_fraud": map[string]any{"type": "noul", "noul": score},
		},
		"usage": map[string]any{"input_tokens": len(req.State)/4 + 1},
	})
}

func (s *Server) handleTags(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"models":[{"name":"tev1:4b"},{"name":"tev1:0.8b"},{"name":"nimble:latest"},{"name":"llama3:8b"}]}`)
}

func defaultScore(state string) float64 {
	var st struct {
		Amount float64 `json:"amount"`
	}
	if err := json.Unmarshal([]byte(state), &st); err != nil {
		return 0.5
	}
	if st.Amount > 200000 {
		return 0.9
	}
	return 0.1
}
```

- [ ] **Passo 2: escrever os testes que falham**

```go file=internal/infrastructure/jev/client_test.go
package jev

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"fraud-detector-jev/internal/infrastructure/jev/jevtest"
)

func novoCliente(t *testing.T, srv *jevtest.Server, ajuste func(*Config)) *Client {
	t.Helper()
	cfg := Config{
		BaseURL:      srv.URL,
		Model:        "tev1:4b",
		Instructions: "Isso é fraude?",
		MaxInflight:  4,
		Timeout:      2 * time.Second,
		MaxAttempts:  3,
		BaseBackoff:  time.Millisecond,
	}
	if ajuste != nil {
		ajuste(&cfg)
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestClassifyMandaOPedidoNoFormatoDoContrato(t *testing.T) {
	srv := jevtest.New(jevtest.Options{})
	defer srv.Close()
	c := novoCliente(t, srv, nil)

	dec, err := c.Classify(context.Background(), `{"amount":350000}`)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Noul != 0.9 || dec.Attempts != 1 || dec.Latency <= 0 || dec.InputTokens < 1 || len(dec.Raw) == 0 {
		t.Errorf("decisão = %+v", dec)
	}

	var req struct {
		Model     string `json:"model"`
		State     string `json:"state"`
		Questions map[string]struct {
			Type         string            `json:"type"`
			Instructions string            `json:"instructions"`
			Criteria     map[string]string `json:"criteria"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(srv.LastBody(), &req); err != nil {
		t.Fatal(err)
	}
	q := req.Questions["is_fraud"]
	if req.Model != "tev1:4b" || req.State != `{"amount":350000}` || q.Type != "noul" || q.Instructions != "Isso é fraude?" {
		t.Errorf("pedido = %+v", req)
	}
	if q.Criteria["true"] == "" || q.Criteria["false"] == "" {
		t.Errorf("criteria faltando: %+v", q.Criteria)
	}
}

func TestClassifyTentaDeNovoEmErro5xx(t *testing.T) {
	srv := jevtest.New(jevtest.Options{FailFirst: 2})
	defer srv.Close()
	c := novoCliente(t, srv, nil)

	dec, err := c.Classify(context.Background(), `{"amount":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Attempts != 3 || srv.Calls() != 3 {
		t.Errorf("tentativas=%d chamadas=%d", dec.Attempts, srv.Calls())
	}
}

func TestClassifyDesisteDepoisDeMaxAttempts(t *testing.T) {
	srv := jevtest.New(jevtest.Options{FailFirst: 10})
	defer srv.Close()
	c := novoCliente(t, srv, nil)

	_, err := c.Classify(context.Background(), `{}`)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != 503 {
		t.Fatalf("esperava StatusError 503, veio %v", err)
	}
	if srv.Calls() != 3 {
		t.Errorf("chamadas = %d, esperava 3", srv.Calls())
	}
}

func TestClassifyNaoTentaDeNovoEm4xx(t *testing.T) {
	srv := jevtest.New(jevtest.Options{FailFirst: 10, FailStatus: 422})
	defer srv.Close()
	c := novoCliente(t, srv, nil)

	_, err := c.Classify(context.Background(), `{}`)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != 422 {
		t.Fatalf("esperava StatusError 422, veio %v", err)
	}
	if srv.Calls() != 1 {
		t.Errorf("chamadas = %d, esperava 1", srv.Calls())
	}
}

func TestClassifyRejeitaRespostasInvalidas(t *testing.T) {
	casos := map[string]string{
		"corpo vazio":      "",
		"não é JSON":       "isso não é json",
		"sem answers":      `{"model":"x"}`,
		"noul ausente":     `{"answers":{"is_fraud":{"type":"noul"}}}`,
		"noul acima de 1":  `{"answers":{"is_fraud":{"type":"noul","noul":1.5}}}`,
		"noul negativo":    `{"answers":{"is_fraud":{"type":"noul","noul":-0.1}}}`,
		"pergunta trocada": `{"answers":{"outra":{"type":"noul","noul":0.5}}}`,
		"noul como texto":  `{"answers":{"is_fraud":{"type":"noul","noul":"alto"}}}`,
		"formato antigo":   `{"answers":{"is_fraud":{"value":true,"probability":0.9,"confidence":0.9}}}`,
	}
	for nome, corpo := range casos {
		t.Run(nome, func(t *testing.T) {
			srv := jevtest.New(jevtest.Options{RawResponse: jevtest.Raw(corpo)})
			defer srv.Close()
			c := novoCliente(t, srv, nil)
			_, err := c.Classify(context.Background(), `{}`)
			if !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("esperava ErrInvalidResponse, veio %v", err)
			}
			if srv.Calls() != 1 {
				t.Errorf("resposta inválida não deveria ser tentada de novo, chamadas = %d", srv.Calls())
			}
		})
	}
}

func TestClassifyAplicaTimeoutPorTentativa(t *testing.T) {
	srv := jevtest.New(jevtest.Options{Delay: 300 * time.Millisecond})
	defer srv.Close()
	c := novoCliente(t, srv, func(cfg *Config) {
		cfg.Timeout = 50 * time.Millisecond
		cfg.MaxAttempts = 1
	})
	if _, err := c.Classify(context.Background(), `{}`); err == nil {
		t.Fatal("esperava erro de timeout")
	}
}

func TestClassifyLimitaChamadasSimultaneas(t *testing.T) {
	srv := jevtest.New(jevtest.Options{Delay: 40 * time.Millisecond})
	defer srv.Close()
	c := novoCliente(t, srv, func(cfg *Config) { cfg.MaxInflight = 2 })

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Classify(context.Background(), `{}`); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if vistas := srv.MaxInflightSeen(); vistas < 1 || vistas > 2 {
		t.Errorf("simultâneas vistas = %d, o limite era 2", vistas)
	}
}

func TestClassifyRespeitaCancelamentoDuranteOBackoff(t *testing.T) {
	srv := jevtest.New(jevtest.Options{FailFirst: 10})
	defer srv.Close()
	c := novoCliente(t, srv, func(cfg *Config) { cfg.BaseBackoff = 5 * time.Second })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	inicio := time.Now()
	_, err := c.Classify(ctx, `{}`)
	if err == nil {
		t.Fatal("esperava erro")
	}
	if time.Since(inicio) > 2*time.Second {
		t.Errorf("deveria ter parado logo, levou %v", time.Since(inicio))
	}
}

func TestNewValidaConfig(t *testing.T) {
	base := Config{BaseURL: "http://localhost:11434", Model: "tev1:4b", Instructions: "x", MaxInflight: 1}
	if _, err := New(base); err != nil {
		t.Fatalf("config válida deu erro: %v", err)
	}
	ruins := map[string]func(*Config){
		"url inválida":  func(c *Config) { c.BaseURL = "não é url" },
		"sem esquema":   func(c *Config) { c.BaseURL = "localhost:11434" },
		"sem modelo":    func(c *Config) { c.Model = " " },
		"sem instrução": func(c *Config) { c.Instructions = "" },
		"sem inflight":  func(c *Config) { c.MaxInflight = 0 },
	}
	for nome, muda := range ruins {
		t.Run(nome, func(t *testing.T) {
			cfg := base
			muda(&cfg)
			if _, err := New(cfg); err == nil {
				t.Error("esperava erro")
			}
		})
	}
}
```

```go file=internal/infrastructure/jev/models_test.go
package jev

import (
	"context"
	"slices"
	"testing"

	"fraud-detector-jev/internal/infrastructure/jev/jevtest"
)

func TestListModelsEFiltro(t *testing.T) {
	srv := jevtest.New(jevtest.Options{})
	defer srv.Close()

	todos, err := ListModels(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(todos, "llama3:8b") || len(todos) != 4 {
		t.Errorf("todos = %v", todos)
	}
	decisao := DecisionModels(todos)
	if !slices.Equal(decisao, []string{"tev1:4b", "tev1:0.8b", "nimble:latest"}) {
		t.Errorf("modelos de decisão = %v", decisao)
	}
}

func TestListModelsFalhaQuandoOllamaEstaFora(t *testing.T) {
	srv := jevtest.New(jevtest.Options{})
	url := srv.URL
	srv.Close()
	if _, err := ListModels(context.Background(), url); err == nil {
		t.Fatal("esperava erro com o servidor fora do ar")
	}
}
```

- [ ] **Passo 3: rodar e ver falhar**

Rode: `go test ./internal/infrastructure/jev/...`
Esperado: FAIL com `undefined: New` e `undefined: ListModels`.

- [ ] **Passo 4: implementar o cliente e a lista de modelos**

```go file=internal/infrastructure/jev/client.go
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"

	"fraud-detector-jev/internal/domain"
)

const (
	questionKey   = "is_fraud"
	systemOnePath = "/v1/systemone"
	maxBodyBytes  = 1 << 20
)

// Textos do criteria da pergunta noul (o que conta como sim e como não).
const (
	criteriaTrue  = "A transação é fraude, esvaziamento ilícito ou lavagem de dinheiro."
	criteriaFalse = "A transação é legítima."
)

// ErrInvalidResponse aparece quando o corpo não tem answers.is_fraud.noul entre 0 e 1.
var ErrInvalidResponse = errors.New("resposta inválida do modelo")

// StatusError é um HTTP diferente de 200.
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("o modelo respondeu HTTP %d: %s", e.Code, e.Body)
}

// Config configura o cliente. MaxAttempts e BaseBackoff têm padrão (3 e 200ms).
type Config struct {
	BaseURL      string
	Model        string
	Instructions string
	MaxInflight  int
	Timeout      time.Duration
	MaxAttempts  int
	BaseBackoff  time.Duration
}

// Client fala com o /v1/systemone do Ollama. É seguro para uso concorrente.
type Client struct {
	cfg  Config
	http *http.Client
	sem  chan struct{}
	url  string
}

// New valida a configuração e cria o cliente.
func New(cfg Config) (*Client, error) {
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("JEV_BASE_URL inválida: %q", cfg.BaseURL)
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("o modelo não pode ser vazio")
	}
	if strings.TrimSpace(cfg.Instructions) == "" {
		return nil, errors.New("as instruções da pergunta não podem ser vazias")
	}
	if cfg.MaxInflight < 1 {
		return nil, errors.New("MaxInflight precisa ser pelo menos 1")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = 200 * time.Millisecond
	}

	c := &Client{
		cfg: cfg,
		http: &http.Client{Transport: &http.Transport{
			MaxIdleConnsPerHost: cfg.MaxInflight,
			IdleConnTimeout:     90 * time.Second,
		}},
		sem: make(chan struct{}, cfg.MaxInflight),
		url: strings.TrimRight(cfg.BaseURL, "/") + systemOnePath,
	}
	return c, nil
}

// Close solta as conexões ociosas. Chame quando terminar uma execução.
func (c *Client) Close() { c.http.CloseIdleConnections() }

type question struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

type request struct {
	Model     string              `json:"model"`
	State     string              `json:"state"`
	Questions map[string]question `json:"questions"`
}

type response struct {
	Answers map[string]struct {
		Type string   `json:"type"`
		Noul *float64 `json:"noul"`
	} `json:"answers"`
	Usage struct {
		InputTokens int `json:"input_tokens"`
	} `json:"usage"`
}

func (c *Client) buildBody(state string) ([]byte, error) {
	return json.Marshal(request{
		Model: c.cfg.Model,
		State: state,
		Questions: map[string]question{
			questionKey: {
				Type:         "noul",
				Instructions: c.cfg.Instructions,
				Criteria:     map[string]string{"true": criteriaTrue, "false": criteriaFalse},
			},
		},
	})
}

// Classify manda o state e devolve o noul. Faz retry só em erro de rede,
// timeout e 5xx.
func (c *Client) Classify(ctx context.Context, state string) (domain.Decision, error) {
	body, err := c.buildBody(state)
	if err != nil {
		return domain.Decision{}, fmt.Errorf("montando o pedido: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		dec, err := c.once(ctx, body)
		if err == nil {
			dec.Attempts = attempt
			return dec, nil
		}
		lastErr = err
		if ctx.Err() != nil || !retryable(err) {
			return domain.Decision{Attempts: attempt}, err
		}
		if attempt < c.cfg.MaxAttempts {
			if err := sleep(ctx, c.backoff(attempt)); err != nil {
				return domain.Decision{Attempts: attempt}, err
			}
		}
	}
	return domain.Decision{Attempts: c.cfg.MaxAttempts},
		fmt.Errorf("esgotou %d tentativas: %w", c.cfg.MaxAttempts, lastErr)
}

func (c *Client) once(ctx context.Context, body []byte) (domain.Decision, error) {
	select {
	case c.sem <- struct{}{}:
	case <-ctx.Done():
		return domain.Decision{}, ctx.Err()
	}
	defer func() { <-c.sem }()

	reqCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return domain.Decision{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return domain.Decision{}, fmt.Errorf("chamando o modelo: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	latency := time.Since(start)
	if err != nil {
		return domain.Decision{}, fmt.Errorf("lendo a resposta do modelo: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return domain.Decision{}, &StatusError{Code: resp.StatusCode, Body: truncate(string(raw), 200)}
	}

	noul, tokens, err := parseResponse(raw)
	if err != nil {
		return domain.Decision{}, err
	}
	return domain.Decision{Noul: noul, InputTokens: tokens, Latency: latency, Raw: raw}, nil
}

func parseResponse(raw []byte) (noul float64, tokens int, err error) {
	var r response
	if err := json.Unmarshal(raw, &r); err != nil {
		return 0, 0, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}
	ans, ok := r.Answers[questionKey]
	if !ok || ans.Noul == nil {
		return 0, 0, fmt.Errorf("%w: faltou answers.%s.noul", ErrInvalidResponse, questionKey)
	}
	if *ans.Noul < 0 || *ans.Noul > 1 {
		return 0, 0, fmt.Errorf("%w: noul %v fora do intervalo de 0 a 1", ErrInvalidResponse, *ans.Noul)
	}
	return *ans.Noul, r.Usage.InputTokens, nil
}

func retryable(err error) bool {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Code >= 500
	}
	return !errors.Is(err, ErrInvalidResponse)
}

func (c *Client) backoff(attempt int) time.Duration {
	d := c.cfg.BaseBackoff << (attempt - 1)
	return d + rand.N(d/2+1)
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
```

```go file=internal/infrastructure/jev/models.go
package jev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ListModels pergunta ao Ollama quais modelos estão baixados (GET /api/tags).
// Serve também para saber se o Ollama está alcançável.
func ListModels(ctx context.Context, baseURL string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("falando com o Ollama em %s: %w", baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("o Ollama respondeu HTTP %d em /api/tags", resp.StatusCode)
	}
	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("lendo a lista de modelos: %w", err)
	}
	names := make([]string, 0, len(payload.Models))
	for _, m := range payload.Models {
		names = append(names, m.Name)
	}
	return names, nil
}

// DecisionModels deixa só os modelos de decisão (nimble e tev1), porque um
// modelo de chat comum não responde direito ao /v1/systemone.
func DecisionModels(all []string) []string {
	var out []string
	for _, name := range all {
		if strings.HasPrefix(name, "nimble") || strings.HasPrefix(name, "tev1") {
			out = append(out, name)
		}
	}
	return out
}
```

- [ ] **Passo 5: rodar e ver passar**

Rode: `go test -race ./internal/infrastructure/jev/...`
Esperado: `ok` nos pacotes `jev` e `jev/jevtest`.

- [ ] **Passo 6: commitar**

```bash
git add internal/infrastructure/jev
git commit -m "feat: adiciona cliente do modelo com retry, limite de simultaneidade e mock"
```

---

## Tarefa 6: portas de serviço e montagem do state

**Arquivos:**
- Criar: `internal/service/features.go`, `classifier.go`, `enricher.go`
- Testar: `internal/service/enricher_test.go`

**Interfaces:**
- Consumes: `domain.Transaction`, `domain.GraphFeatures`, `domain.FeatureQuery`, `domain.Subgraph`, `domain.Decision`.
- Produz:
  - `service.GraphFeatureSource` com `Features(ctx, domain.FeatureQuery) (domain.GraphFeatures, error)`
  - `service.SubgraphSource` com `Subgraph(ctx, orig, dest string, beforeSeq int64, atStep, limitPerAccount int) (domain.Subgraph, error)`
  - `service.Classifier` com `Classify(ctx, state string) (domain.Decision, error)`
  - `service.State`, `service.BuildState(tx, gf *domain.GraphFeatures) State`, `(State).JSON() (string, error)`
  - `service.FeatureQueryFor(tx domain.Transaction, windowHours, cycleMaxHops int) domain.FeatureQuery`

- [ ] **Passo 1: escrever o teste que falha**

```go file=internal/service/enricher_test.go
package service

import (
	"encoding/json"
	"testing"

	"fraud-detector-jev/internal/domain"
)

func estadoComoMapa(t *testing.T, tx domain.Transaction, gf *domain.GraphFeatures) map[string]any {
	t.Helper()
	js, err := BuildState(tx, gf).JSON()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(js), &m); err != nil {
		t.Fatalf("o state não é JSON válido: %v", err)
	}
	return m
}

func txExemplo() domain.Transaction {
	return domain.Transaction{
		Seq: 1029, Step: 4, Type: "TRANSFER", Amount: 350000,
		Orig: "C111", OldBalanceOrig: 350000, NewBalanceOrig: 0,
		Dest: "C222", OldBalanceDest: 0, NewBalanceDest: 0,
	}
}

func TestBuildStateSemGrafo(t *testing.T) {
	m := estadoComoMapa(t, txExemplo(), nil)
	if m["transaction_id"] != "TX_1029" || m["type"] != "TRANSFER" || m["time_step"] != float64(4) ||
		m["amount"] != float64(350000) || m["balance_drop"] != float64(350000) || m["dest_is_merchant"] != false {
		t.Errorf("campos tabulares errados: %v", m)
	}
	for _, chave := range []string{"orig_balance_before", "orig_balance_after", "dest_balance_before", "dest_balance_after"} {
		if _, ok := m[chave]; !ok {
			t.Errorf("faltou %s", chave)
		}
	}
	if _, ok := m["graph_features"]; ok {
		t.Error("sem grafo não pode ter graph_features")
	}
	for _, proibida := range []string{"orig", "dest", "nameOrig", "nameDest", "is_fraud", "isFraud", "seq"} {
		if _, ok := m[proibida]; ok {
			t.Errorf("o campo %s não pode ir para o modelo", proibida)
		}
	}
}

func TestBuildStateComGrafo(t *testing.T) {
	idade := 3
	gf := &domain.GraphFeatures{
		FirstTransferBetweenPair: true, DestInDegree: 42, DestFanInWindow: 17,
		DestAccountAgeSteps: &idade, CycleDetected: true,
	}
	m := estadoComoMapa(t, txExemplo(), gf)
	g, ok := m["graph_features"].(map[string]any)
	if !ok {
		t.Fatalf("faltou graph_features: %v", m)
	}
	if g["dest_in_degree"] != float64(42) || g["cycle_detected"] != true || g["dest_account_age_steps"] != float64(3) {
		t.Errorf("graph_features = %v", g)
	}

	gf.DestAccountAgeSteps = nil
	g = estadoComoMapa(t, txExemplo(), gf)["graph_features"].(map[string]any)
	if v, existe := g["dest_account_age_steps"]; !existe || v != nil {
		t.Errorf("idade ausente deveria ser null, veio %v (existe=%v)", v, existe)
	}
}

func TestFeatureQueryFor(t *testing.T) {
	q := FeatureQueryFor(txExemplo(), 24, 3)
	want := domain.FeatureQuery{Orig: "C111", Dest: "C222", BeforeSeq: 1029, AtStep: 4, WindowHours: 24, CycleMaxHops: 3}
	if q != want {
		t.Errorf("FeatureQueryFor = %+v, esperado %+v", q, want)
	}
}
```

- [ ] **Passo 2: rodar e ver falhar**

Rode: `go test ./internal/service/ -run 'BuildState|FeatureQueryFor'`
Esperado: FAIL com `undefined: BuildState`.

- [ ] **Passo 3: implementar**

```go file=internal/service/features.go
package service

import (
	"context"

	"fraud-detector-jev/internal/domain"
)

// GraphFeatureSource calcula as features de grafo de uma transação. Quem
// implementa (o Neo4j) precisa respeitar q.BeforeSeq e q.AtStep. Trocar por
// features pré-calculadas em lote é só implementar esta interface de outro jeito.
type GraphFeatureSource interface {
	Features(ctx context.Context, q domain.FeatureQuery) (domain.GraphFeatures, error)
}

// SubgraphSource devolve a vizinhança de 1 salto da origem e do destino, com
// no máximo limitPerAccount arestas por conta.
type SubgraphSource interface {
	Subgraph(ctx context.Context, orig, dest string, beforeSeq int64, atStep, limitPerAccount int) (domain.Subgraph, error)
}
```

```go file=internal/service/classifier.go
package service

import (
	"context"

	"fraud-detector-jev/internal/domain"
)

// Classifier manda um state ao modelo e devolve a decisão. O jev.Client
// implementa esta interface.
type Classifier interface {
	Classify(ctx context.Context, state string) (domain.Decision, error)
}
```

```go file=internal/service/enricher.go
package service

import (
	"encoding/json"

	"fraud-detector-jev/internal/domain"
)

// State é o que o modelo enxerga. Os nomes das contas, o gabarito e o seq não
// vão para o modelo de propósito.
type State struct {
	TransactionID     string                `json:"transaction_id"`
	Type              string                `json:"type"`
	Amount            float64               `json:"amount"`
	TimeStep          int                   `json:"time_step"`
	OrigBalanceBefore float64               `json:"orig_balance_before"`
	OrigBalanceAfter  float64               `json:"orig_balance_after"`
	DestBalanceBefore float64               `json:"dest_balance_before"`
	DestBalanceAfter  float64               `json:"dest_balance_after"`
	BalanceDrop       float64               `json:"balance_drop"`
	DestIsMerchant    bool                  `json:"dest_is_merchant"`
	GraphFeatures     *domain.GraphFeatures `json:"graph_features,omitempty"`
}

// BuildState monta o state. Com gf nil (execução sem grafo) o bloco
// graph_features não existe.
func BuildState(tx domain.Transaction, gf *domain.GraphFeatures) State {
	return State{
		TransactionID:     tx.ID(),
		Type:              tx.Type,
		Amount:            tx.Amount,
		TimeStep:          tx.Step,
		OrigBalanceBefore: tx.OldBalanceOrig,
		OrigBalanceAfter:  tx.NewBalanceOrig,
		DestBalanceBefore: tx.OldBalanceDest,
		DestBalanceAfter:  tx.NewBalanceDest,
		BalanceDrop:       tx.BalanceDrop(),
		DestIsMerchant:    tx.DestIsMerchant(),
		GraphFeatures:     gf,
	}
}

// JSON devolve o state como string JSON, que é o formato do campo state.
func (s State) JSON() (string, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// FeatureQueryFor monta a consulta de replay: a transação só enxerga o que veio
// antes dela.
func FeatureQueryFor(tx domain.Transaction, windowHours, cycleMaxHops int) domain.FeatureQuery {
	return domain.FeatureQuery{
		Orig:         tx.Orig,
		Dest:         tx.Dest,
		BeforeSeq:    tx.Seq,
		AtStep:       tx.Step,
		WindowHours:  windowHours,
		CycleMaxHops: cycleMaxHops,
	}
}
```

- [ ] **Passo 4: rodar e ver passar**

Rode: `go test -race ./internal/service/`
Esperado: `ok`.

- [ ] **Passo 5: commitar**

```bash
git add internal/service/features.go internal/service/classifier.go internal/service/enricher.go internal/service/enricher_test.go
git commit -m "feat: adiciona interfaces de serviço e montagem do state"
```

---
## Tarefa 7: Neo4j, cliente e carga do grafo

**Arquivos:**
- Criar: `internal/infrastructure/neo4j/client.go`, `queries.go`, `loader.go`
- Criar: `testdata/mini_paysim.csv` (11 linhas com os cenários de teste)
- Criar: `scripts/test-integration.sh`
- Testar: `internal/infrastructure/neo4j/helpers_integration_test.go`, `loader_integration_test.go` (tag `integration`)

**Interfaces:**
- Consumes: `domain.Transaction`, `domain.DatasetStats`, `csv.Stream`.
- Produz:
  - `neo4j.Options{URI, User, Password string, MaxPool int, QueryTimeout time.Duration}`
  - `neo4j.New(ctx, Options) (*neo4j.Client, error)`, `(*Client).Close(ctx) error`, `(*Client).Ping(ctx) error`
  - `neo4j.LoadOptions{CSVPath string, BatchSize int, Progress func(phase string, done int64)}`
  - `neo4j.LoadReport{Nodes, Rels int64, Stats domain.DatasetStats, Resumed bool}`
  - `(*Client).Load(ctx, LoadOptions) (LoadReport, error)` e `(*Client).Counts(ctx) (nodes, rels int64, err error)`

Como a carga funciona: cria a constraint de `Account.id`, cria as contas com `MERGE` em lotes, cria as arestas com `CREATE` em lotes ordenados por `seq` (cada lote é uma transação) e só no fim cria os índices de `seq` e `step`. Se já existirem arestas, assume que a fase das contas terminou e continua do maior `seq` gravado. No fim confere se o total de arestas bate com as linhas do CSV.

Os testes de integração precisam de um Neo4j de verdade. Eles usam um container descartável que o script sobe e derruba.

- [ ] **Passo 1: criar o CSV de teste com os cenários**

Cada linha vira um `seq` (0 a 10). Os cenários que ele cobre estão descritos na Tarefa 8.

```csv file=testdata/mini_paysim.csv
step,type,amount,nameOrig,oldbalanceOrg,newbalanceOrig,nameDest,oldbalanceDest,newbalanceDest,isFraud,isFlaggedFraud
1,TRANSFER,100.0,A,500.0,400.0,B,0.0,100.0,0,0
1,TRANSFER,90.0,B,300.0,210.0,C,0.0,90.0,0,0
2,TRANSFER,80.0,C,200.0,120.0,A,400.0,480.0,1,0
2,TRANSFER,10.0,D,50.0,40.0,X,0.0,10.0,0,0
3,TRANSFER,10.0,E,50.0,40.0,X,10.0,20.0,0,0
3,CASH_OUT,10.0,F,50.0,40.0,X,20.0,30.0,0,0
30,TRANSFER,10.0,G,50.0,40.0,X,30.0,40.0,0,0
31,TRANSFER,5.0,H,50.0,45.0,P,0.0,5.0,0,0
31,TRANSFER,5.0,H,45.0,40.0,Q,0.0,5.0,0,0
31,TRANSFER,5.0,H,40.0,35.0,R,0.0,5.0,0,0
32,TRANSFER,100.0,A,400.0,300.0,B,100.0,200.0,0,0
```

- [ ] **Passo 2: criar o script que sobe um Neo4j descartável e roda os testes de integração**

```bash file=scripts/test-integration.sh
#!/usr/bin/env bash
# Sobe um Neo4j descartável em outra porta, roda os testes de integração e
# derruba tudo no final. Nunca encosta na base de verdade do compose.
set -euo pipefail

NOME=fraud-neo4j-it
PORTA=${NEO4J_TEST_PORT:-17687}
SENHA=testpassword123
IMAGEM=${NEO4J_IMAGE:-neo4j:5.26-community}

limpar() { docker rm -f -v "$NOME" >/dev/null 2>&1 || true; }
trap limpar EXIT
limpar

docker run -d --name "$NOME" -p "127.0.0.1:${PORTA}:7687" \
  -e NEO4J_AUTH="neo4j/${SENHA}" \
  -e NEO4J_server_memory_heap_initial__size=512m \
  -e NEO4J_server_memory_heap_max__size=512m \
  -e NEO4J_server_memory_pagecache_size=256m \
  "$IMAGEM" >/dev/null

echo "Esperando o Neo4j de teste ficar pronto..."
pronto=0
for _ in $(seq 1 60); do
  if docker exec "$NOME" cypher-shell -u neo4j -p "$SENHA" 'RETURN 1' >/dev/null 2>&1; then
    pronto=1
    break
  fi
  sleep 2
done
if [ "$pronto" != 1 ]; then
  echo "O Neo4j de teste não subiu a tempo. Últimas linhas do log:"
  docker logs "$NOME" 2>&1 | tail -20
  exit 1
fi

export NEO4J_TEST_URI="bolt://127.0.0.1:${PORTA}"
export NEO4J_TEST_PASSWORD="$SENHA"
go test -race -tags=integration -count=1 ./internal/infrastructure/neo4j/...
```

Depois de criar o arquivo, dê permissão de execução: `chmod +x scripts/test-integration.sh`.

- [ ] **Passo 3: escrever os testes de integração que falham**

```go file=internal/infrastructure/neo4j/helpers_integration_test.go
//go:build integration

package neo4j

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func envOu(nome, padrao string) string {
	if v := os.Getenv(nome); v != "" {
		return v
	}
	return padrao
}

// clienteDeTeste conecta no Neo4j descartável e devolve o grafo vazio.
func clienteDeTeste(t *testing.T) *Client {
	t.Helper()
	c, err := New(context.Background(), Options{
		URI:          envOu("NEO4J_TEST_URI", "bolt://127.0.0.1:17687"),
		User:         "neo4j",
		Password:     envOu("NEO4J_TEST_PASSWORD", "testpassword123"),
		MaxPool:      10,
		QueryTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("não consegui conectar no Neo4j de teste (rode pelo scripts/test-integration.sh): %v", err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	limparGrafo(t, c)
	return c
}

func limparGrafo(t *testing.T, c *Client) {
	t.Helper()
	if _, err := c.write(context.Background(), `MATCH (n) DETACH DELETE n`, nil); err != nil {
		t.Fatal(err)
	}
}

func caminhoFixture() string {
	return filepath.Join("..", "..", "..", "testdata", "mini_paysim.csv")
}

// carregarFixture carrega o mini_paysim.csv (11 linhas, lotes de 4 para
// exercitar mais de um lote).
func carregarFixture(t *testing.T, c *Client) LoadReport {
	t.Helper()
	rep, err := c.Load(context.Background(), LoadOptions{CSVPath: caminhoFixture(), BatchSize: 4})
	if err != nil {
		t.Fatalf("carga da fixture falhou: %v", err)
	}
	return rep
}
```

```go file=internal/infrastructure/neo4j/loader_integration_test.go
//go:build integration

package neo4j

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"fraud-detector-jev/internal/domain"
	"fraud-detector-jev/internal/infrastructure/csv"
)

func TestLoadCarregaGrafoEEstatisticas(t *testing.T) {
	c := clienteDeTeste(t)
	rep := carregarFixture(t, c)

	if rep.Nodes != 12 || rep.Rels != 11 || rep.Resumed {
		t.Errorf("relatório = %+v", rep)
	}
	want := domain.DatasetStats{
		TotalRows: 11, FraudRows: 1, MaxStep: 32,
		ByType: map[string]domain.TypeStats{
			"TRANSFER": {Rows: 10, Frauds: 1},
			"CASH_OUT": {Rows: 1, Frauds: 0},
		},
	}
	if rep.Stats.TotalRows != want.TotalRows || rep.Stats.FraudRows != want.FraudRows || rep.Stats.MaxStep != want.MaxStep ||
		rep.Stats.ByType["TRANSFER"] != want.ByType["TRANSFER"] || rep.Stats.ByType["CASH_OUT"] != want.ByType["CASH_OUT"] {
		t.Errorf("estatísticas = %+v", rep.Stats)
	}

	nodes, rels, err := c.Counts(context.Background())
	if err != nil || nodes != 12 || rels != 11 {
		t.Errorf("Counts = %d %d %v", nodes, rels, err)
	}
}

func TestLoadRetomaDeOndeParou(t *testing.T) {
	c := clienteDeTeste(t)
	carregarFixture(t, c)

	// Simula uma carga que caiu depois do seq 7: apaga as arestas 8 a 10.
	if _, err := c.write(context.Background(), `MATCH ()-[t:TRANSFERRED]->() WHERE t.seq >= 8 DELETE t`, nil); err != nil {
		t.Fatal(err)
	}
	_, rels, _ := c.Counts(context.Background())
	if rels != 8 {
		t.Fatalf("preparação errada, arestas = %d", rels)
	}

	rep := carregarFixture(t, c)
	if !rep.Resumed || rep.Rels != 11 || rep.Nodes != 12 {
		t.Errorf("relatório da retomada = %+v", rep)
	}

	// Rodar de novo, com tudo já carregado, não pode duplicar nada.
	rep = carregarFixture(t, c)
	if rep.Rels != 11 {
		t.Errorf("segunda retomada duplicou arestas: %+v", rep)
	}
}

func TestLoadRecusaCSVForaDeOrdem(t *testing.T) {
	c := clienteDeTeste(t)
	arquivo := filepath.Join(t.TempDir(), "fora_de_ordem.csv")
	conteudo := "step,type,amount,nameOrig,oldbalanceOrg,newbalanceOrig,nameDest,oldbalanceDest,newbalanceDest,isFraud,isFlaggedFraud\n" +
		"5,TRANSFER,1,A,1,0,B,0,1,0,0\n" +
		"4,TRANSFER,1,A,1,0,B,0,1,0,0\n"
	if err := os.WriteFile(arquivo, []byte(conteudo), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := c.Load(context.Background(), LoadOptions{CSVPath: arquivo})
	if !errors.Is(err, csv.ErrUnsortedSteps) {
		t.Fatalf("esperava ErrUnsortedSteps, veio %v", err)
	}
	if _, rels, _ := c.Counts(context.Background()); rels != 0 {
		t.Errorf("não deveria ter gravado arestas, tem %d", rels)
	}
}
```

- [ ] **Passo 4: rodar e ver falhar**

Rode: `go vet -tags=integration ./internal/infrastructure/neo4j/`
Esperado: erros de compilação como `undefined: New` e `undefined: LoadOptions`. Isso confirma que os testes existem e dependem do código que ainda não foi escrito.

- [ ] **Passo 5: implementar as queries de carga**

O texto Cypher fica todo em `queries.go`. A parte de features entra na Tarefa 8, neste mesmo arquivo não: ela ganha um arquivo próprio para cada tarefa ser independente.

```go file=internal/infrastructure/neo4j/queries.go
package neo4j

// Schema. Os índices de seq e step só são criados no fim da carga, porque
// criar antes deixaria a inserção bem mais lenta.
const (
	createConstraint = `CREATE CONSTRAINT account_id IF NOT EXISTS FOR (a:Account) REQUIRE a.id IS UNIQUE`
	createIndexSeq   = `CREATE INDEX transferred_seq IF NOT EXISTS FOR ()-[t:TRANSFERRED]-() ON (t.seq)`
	createIndexStep  = `CREATE INDEX transferred_step IF NOT EXISTS FOR ()-[t:TRANSFERRED]-() ON (t.step)`
	awaitIndexes     = `CALL db.awaitIndexes(600)`
)

// Carga.
const (
	mergeAccounts = `UNWIND $ids AS id MERGE (:Account {id: id})`

	createEdges = `UNWIND $rows AS r
MATCH (a:Account {id: r.orig})
MATCH (b:Account {id: r.dest})
CREATE (a)-[:TRANSFERRED {seq: r.seq, step: r.step, type: r.type, amount: r.amount, isFraud: r.isFraud}]->(b)`

	maxSeqQuery = `MATCH ()-[t:TRANSFERRED]->() RETURN max(t.seq) AS maxSeq`
	countNodes  = `MATCH (n:Account) RETURN count(n) AS n`
	countRels   = `MATCH ()-[t:TRANSFERRED]->() RETURN count(t) AS n`
)
```

- [ ] **Passo 6: implementar o cliente**

```go file=internal/infrastructure/neo4j/client.go
package neo4j

import (
	"context"
	"fmt"
	"time"

	neo "github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// database é o banco padrão do Neo4j Community.
const database = "neo4j"

// Options configura a conexão. QueryTimeout vale para as queries de leitura
// de feature (padrão 2s).
type Options struct {
	URI          string
	User         string
	Password     string
	MaxPool      int
	QueryTimeout time.Duration
}

// Client é o acesso ao Neo4j. É seguro para uso concorrente.
type Client struct {
	driver       neo.DriverWithContext
	queryTimeout time.Duration
}

// New conecta e confere a conexão.
func New(ctx context.Context, o Options) (*Client, error) {
	driver, err := neo.NewDriverWithContext(o.URI, neo.BasicAuth(o.User, o.Password, ""), func(c *neo.Config) {
		if o.MaxPool > 0 {
			c.MaxConnectionPoolSize = o.MaxPool
		}
	})
	if err != nil {
		return nil, fmt.Errorf("criando o driver do Neo4j: %w", err)
	}
	if err := driver.VerifyConnectivity(ctx); err != nil {
		_ = driver.Close(ctx)
		return nil, fmt.Errorf("conectando no Neo4j em %s: %w", o.URI, err)
	}
	timeout := o.QueryTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &Client{driver: driver, queryTimeout: timeout}, nil
}

// Close fecha o driver.
func (c *Client) Close(ctx context.Context) error { return c.driver.Close(ctx) }

// Ping confere se o Neo4j está respondendo.
func (c *Client) Ping(ctx context.Context) error { return c.driver.VerifyConnectivity(ctx) }

// read roda uma leitura curta, com timeout no servidor e no cliente.
func (c *Client) read(ctx context.Context, query string, params map[string]any) (*neo.EagerResult, error) {
	ctx, cancel := context.WithTimeout(ctx, c.queryTimeout+time.Second)
	defer cancel()
	return neo.ExecuteQuery(ctx, c.driver, query, params, neo.EagerResultTransformer,
		neo.ExecuteQueryWithDatabase(database),
		neo.ExecuteQueryWithReadersRouting(),
		neo.ExecuteQueryWithTransactionConfig(neo.WithTxTimeout(c.queryTimeout)))
}

// readLong roda uma leitura sem timeout, para contagens e máximos na carga.
func (c *Client) readLong(ctx context.Context, query string) (*neo.EagerResult, error) {
	return neo.ExecuteQuery(ctx, c.driver, query, nil, neo.EagerResultTransformer,
		neo.ExecuteQueryWithDatabase(database),
		neo.ExecuteQueryWithReadersRouting())
}

// write roda uma escrita sem timeout.
func (c *Client) write(ctx context.Context, query string, params map[string]any) (*neo.EagerResult, error) {
	return neo.ExecuteQuery(ctx, c.driver, query, params, neo.EagerResultTransformer,
		neo.ExecuteQueryWithDatabase(database),
		neo.ExecuteQueryWithWritersRouting())
}

// recordInt lê um inteiro do registro e falha se a coluna não existir.
func recordInt(rec *neo.Record, key string) (int64, error) {
	v, ok := rec.Get(key)
	n, isInt := v.(int64)
	if !ok || !isInt {
		return 0, fmt.Errorf("a coluna %s veio ausente ou não é inteiro (%T)", key, v)
	}
	return n, nil
}
```

- [ ] **Passo 7: implementar a carga**

```go file=internal/infrastructure/neo4j/loader.go
package neo4j

import (
	"context"
	"fmt"

	"fraud-detector-jev/internal/domain"
	"fraud-detector-jev/internal/infrastructure/csv"
)

// LoadOptions configura a carga. Progress é opcional e recebe a fase
// ("nodes" ou "edges") e quantas linhas do CSV já foram tratadas.
type LoadOptions struct {
	CSVPath   string
	BatchSize int
	Progress  func(phase string, done int64)
}

// LoadReport resume o que ficou no banco depois da carga.
type LoadReport struct {
	Nodes   int64
	Rels    int64
	Stats   domain.DatasetStats
	Resumed bool
}

// Load carrega o CSV no grafo. Pode ser executada de novo depois de uma queda:
// se já existirem arestas, pula a criação das contas (que só começa a criar
// arestas depois de terminar) e continua do maior seq gravado. Cada lote é uma
// transação, então nunca sobra meia aresta.
func (c *Client) Load(ctx context.Context, o LoadOptions) (LoadReport, error) {
	if o.BatchSize <= 0 {
		o.BatchSize = 10000
	}
	progress := o.Progress
	if progress == nil {
		progress = func(string, int64) {}
	}

	if _, err := c.write(ctx, createConstraint, nil); err != nil {
		return LoadReport{}, fmt.Errorf("criando a constraint: %w", err)
	}
	maxSeq, err := c.maxLoadedSeq(ctx)
	if err != nil {
		return LoadReport{}, err
	}
	report := LoadReport{Resumed: maxSeq >= 0}

	if maxSeq < 0 {
		if err := c.loadNodes(ctx, o, progress); err != nil {
			return report, err
		}
	}
	stats, err := c.loadEdges(ctx, o, maxSeq, progress)
	if err != nil {
		return report, err
	}
	report.Stats = stats

	for _, q := range []string{createIndexSeq, createIndexStep, awaitIndexes} {
		if _, err := c.write(ctx, q, nil); err != nil {
			return report, fmt.Errorf("criando os índices: %w", err)
		}
	}

	report.Nodes, report.Rels, err = c.Counts(ctx)
	if err != nil {
		return report, err
	}
	if report.Rels != stats.TotalRows {
		return report, fmt.Errorf("o grafo ficou com %d arestas mas o CSV tem %d linhas", report.Rels, stats.TotalRows)
	}
	return report, nil
}

// Counts devolve quantas contas e quantas arestas existem no grafo.
func (c *Client) Counts(ctx context.Context) (nodes, rels int64, err error) {
	for query, dest := range map[string]*int64{countNodes: &nodes, countRels: &rels} {
		res, qerr := c.readLong(ctx, query)
		if qerr != nil {
			return 0, 0, fmt.Errorf("contando o grafo: %w", qerr)
		}
		n, ierr := recordInt(res.Records[0], "n")
		if ierr != nil {
			return 0, 0, ierr
		}
		*dest = n
	}
	return nodes, rels, nil
}

// maxLoadedSeq devolve o maior seq já gravado, ou -1 se não há arestas.
func (c *Client) maxLoadedSeq(ctx context.Context) (int64, error) {
	res, err := c.readLong(ctx, maxSeqQuery)
	if err != nil {
		return 0, fmt.Errorf("procurando o ponto de retomada: %w", err)
	}
	v, _ := res.Records[0].Get("maxSeq")
	if v == nil {
		return -1, nil
	}
	n, ok := v.(int64)
	if !ok {
		return 0, fmt.Errorf("maxSeq com tipo inesperado %T", v)
	}
	return n, nil
}

func (c *Client) loadNodes(ctx context.Context, o LoadOptions, progress func(string, int64)) error {
	batch := make(map[string]struct{}, o.BatchSize*2)
	var rows int64

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		ids := make([]string, 0, len(batch))
		for id := range batch {
			ids = append(ids, id)
		}
		if _, err := c.write(ctx, mergeAccounts, map[string]any{"ids": ids}); err != nil {
			return fmt.Errorf("criando contas: %w", err)
		}
		clear(batch)
		return nil
	}

	err := csv.Stream(ctx, o.CSVPath, func(tx domain.Transaction) error {
		batch[tx.Orig] = struct{}{}
		batch[tx.Dest] = struct{}{}
		rows++
		if len(batch) >= o.BatchSize {
			if err := flush(); err != nil {
				return err
			}
			progress("nodes", rows)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := flush(); err != nil {
		return err
	}
	progress("nodes", rows)
	return nil
}

// loadEdges lê o CSV inteiro para calcular as estatísticas (mesmo as linhas
// que já estavam gravadas) e grava só as arestas com seq maior que maxSeq.
func (c *Client) loadEdges(ctx context.Context, o LoadOptions, maxSeq int64, progress func(string, int64)) (domain.DatasetStats, error) {
	stats := domain.DatasetStats{ByType: map[string]domain.TypeStats{}}
	rows := make([]map[string]any, 0, o.BatchSize)

	flush := func() error {
		if len(rows) == 0 {
			return nil
		}
		if _, err := c.write(ctx, createEdges, map[string]any{"rows": rows}); err != nil {
			return fmt.Errorf("criando arestas: %w", err)
		}
		rows = rows[:0]
		return nil
	}

	err := csv.Stream(ctx, o.CSVPath, func(tx domain.Transaction) error {
		stats.TotalRows++
		ts := stats.ByType[tx.Type]
		ts.Rows++
		if tx.IsFraud {
			stats.FraudRows++
			ts.Frauds++
		}
		stats.ByType[tx.Type] = ts
		if tx.Step > stats.MaxStep {
			stats.MaxStep = tx.Step
		}

		if tx.Seq <= maxSeq {
			return nil
		}
		rows = append(rows, map[string]any{
			"seq": tx.Seq, "step": tx.Step, "type": tx.Type, "amount": tx.Amount,
			"isFraud": tx.IsFraud, "orig": tx.Orig, "dest": tx.Dest,
		})
		if len(rows) >= o.BatchSize {
			if err := flush(); err != nil {
				return err
			}
			progress("edges", tx.Seq+1)
		}
		return nil
	})
	if err != nil {
		return stats, err
	}
	if err := flush(); err != nil {
		return stats, err
	}
	progress("edges", stats.TotalRows)
	return stats, nil
}
```

- [ ] **Passo 8: rodar os testes de integração e ver passar**

Rode: `chmod +x scripts/test-integration.sh && ./scripts/test-integration.sh`
Esperado: o script sobe o container, espera ficar pronto, e o Go imprime `ok  fraud-detector-jev/internal/infrastructure/neo4j`. Na primeira vez a imagem pode demorar para baixar.

- [ ] **Passo 9: commitar**

```bash
git add internal/infrastructure/neo4j testdata/mini_paysim.csv scripts/test-integration.sh
git commit -m "feat: adiciona cliente do Neo4j e carga do grafo com retomada"
```

---

## Tarefa 8: Neo4j, queries de features e subgrafo

**Arquivos:**
- Criar: `internal/infrastructure/neo4j/features_queries.go`, `features.go`
- Testar: `internal/infrastructure/neo4j/features_queries_test.go` (unitário), `features_integration_test.go`, `leak_integration_test.go` (tag `integration`)

**Interfaces:**
- Consumes: `neo4j.Client` (Tarefa 7), `domain.FeatureQuery`, `domain.GraphFeatures`, `domain.Subgraph`.
- Produz:
  - `neo4j.FeaturesQuery(cycleMaxHops int) (string, error)` (texto Cypher, valida de 1 a 5 saltos)
  - `(*neo4j.Client).Features(ctx, domain.FeatureQuery) (domain.GraphFeatures, error)`, que implementa `service.GraphFeatureSource`
  - `(*neo4j.Client).Subgraph(ctx, orig, dest string, beforeSeq int64, atStep, limitPerAccount int) (domain.Subgraph, error)`, que implementa `service.SubgraphSource`

Regras de tempo: toda query de feature filtra `t.seq < $beforeSeq AND t.step <= $atStep`. A janela de K horas é `t.step > $atStep - $window`. A idade da conta é `atStep` menos o menor `step` entre as arestas anteriores que tocam o destino (como origem ou destino). Nenhuma query de feature pode ler `isFraud` nem `isFlagged`.

- [ ] **Passo 1: escrever os testes que falham**

Teste unitário (roda sem Neo4j):

```go file=internal/infrastructure/neo4j/features_queries_test.go
package neo4j

import (
	"context"
	"strings"
	"testing"

	"fraud-detector-jev/internal/domain"
)

// A query de feature nunca pode ler o gabarito. Se alguém colocar isFraud ou
// isFlagged aqui, o benchmark passa a trapacear e este teste avisa.
func TestFeaturesQueryNaoLeOGabarito(t *testing.T) {
	q, err := FeaturesQuery(3)
	if err != nil {
		t.Fatal(err)
	}
	baixo := strings.ToLower(q)
	for _, proibida := range []string{"isfraud", "isflagged"} {
		if strings.Contains(baixo, proibida) {
			t.Errorf("a query de features não pode citar %s", proibida)
		}
	}
	for _, exigida := range []string{"$beforeSeq", "$atStep", "$window", "*1..3"} {
		if !strings.Contains(q, exigida) {
			t.Errorf("a query de features deveria conter %s", exigida)
		}
	}
}

func TestFeaturesQueryValidaSaltos(t *testing.T) {
	for _, ruim := range []int{0, -1, 6, 100} {
		if _, err := FeaturesQuery(ruim); err == nil {
			t.Errorf("%d saltos deveria dar erro", ruim)
		}
	}
	for _, bom := range []int{1, 2, 3, 4, 5} {
		if _, err := FeaturesQuery(bom); err != nil {
			t.Errorf("%d saltos deveria valer: %v", bom, err)
		}
	}
}

func TestFeaturesRecusaSaltosInvalidosSemTocarNoBanco(t *testing.T) {
	// O cliente vazio nunca é usado, porque a validação vem antes da consulta.
	c := &Client{}
	_, err := c.Features(context.Background(), domain.FeatureQuery{Orig: "A", Dest: "B", CycleMaxHops: 9})
	if err == nil {
		t.Fatal("esperava erro")
	}
}
```

Teste de integração das features. Os números esperados vêm do `mini_paysim.csv` e cada caso comenta o cenário:

```go file=internal/infrastructure/neo4j/features_integration_test.go
//go:build integration

package neo4j

import (
	"context"
	"math"
	"testing"

	"fraud-detector-jev/internal/domain"
)

func idade(n int) *int { return &n }

func igualIdade(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func TestFeaturesNosCenariosDaFixture(t *testing.T) {
	c := clienteDeTeste(t)
	carregarFixture(t, c)

	casos := []struct {
		nome string
		q    domain.FeatureQuery
		want domain.GraphFeatures
	}{
		{
			"seq0 A para B, primeira transação de todas",
			domain.FeatureQuery{Orig: "A", Dest: "B", BeforeSeq: 0, AtStep: 1, WindowHours: 24, CycleMaxHops: 3},
			domain.GraphFeatures{FirstTransferBetweenPair: true},
		},
		{
			"seq1 B para C, o futuro (C para A no seq2) não pode fechar ciclo",
			domain.FeatureQuery{Orig: "B", Dest: "C", BeforeSeq: 1, AtStep: 1, WindowHours: 24, CycleMaxHops: 3},
			domain.GraphFeatures{FirstTransferBetweenPair: true},
		},
		{
			"seq2 C para A fecha o ciclo A para B para C para A",
			domain.FeatureQuery{Orig: "C", Dest: "A", BeforeSeq: 2, AtStep: 2, WindowHours: 24, CycleMaxHops: 3},
			domain.GraphFeatures{FirstTransferBetweenPair: true, DestFanOutWindow: 1, DestAccountAgeSteps: idade(1), CycleDetected: true},
		},
		{
			"seq2 com limite de 1 salto não enxerga o ciclo de 2 saltos",
			domain.FeatureQuery{Orig: "C", Dest: "A", BeforeSeq: 2, AtStep: 2, WindowHours: 24, CycleMaxHops: 1},
			domain.GraphFeatures{FirstTransferBetweenPair: true, DestFanOutWindow: 1, DestAccountAgeSteps: idade(1)},
		},
		{
			"seq6 G para X, os 3 remetentes anteriores estão fora da janela de 24h",
			domain.FeatureQuery{Orig: "G", Dest: "X", BeforeSeq: 6, AtStep: 30, WindowHours: 24, CycleMaxHops: 3},
			domain.GraphFeatures{FirstTransferBetweenPair: true, DestInDegree: 3, DestAccountAgeSteps: idade(28)},
		},
		{
			"seq6 com janela de 48h passa a enxergar os 3 remetentes",
			domain.FeatureQuery{Orig: "G", Dest: "X", BeforeSeq: 6, AtStep: 30, WindowHours: 48, CycleMaxHops: 3},
			domain.GraphFeatures{FirstTransferBetweenPair: true, DestInDegree: 3, DestFanInWindow: 3, DestAccountAgeSteps: idade(28)},
		},
		{
			"seq9 H para R, H já mandou para P e Q na mesma hora",
			domain.FeatureQuery{Orig: "H", Dest: "R", BeforeSeq: 9, AtStep: 31, WindowHours: 24, CycleMaxHops: 3},
			domain.GraphFeatures{FirstTransferBetweenPair: true, OrigFanOutWindow: 2},
		},
		{
			"seq10 A para B repete o par, e B para C para A é um ciclo",
			domain.FeatureQuery{Orig: "A", Dest: "B", BeforeSeq: 10, AtStep: 32, WindowHours: 24, CycleMaxHops: 3},
			domain.GraphFeatures{FirstTransferBetweenPair: false, DestInDegree: 1, DestAccountAgeSteps: idade(31), CycleDetected: true},
		},
		{
			"modo agora: origem que nunca apareceu e destino X, com o grafo inteiro",
			domain.FeatureQuery{Orig: "ZZ", Dest: "X", BeforeSeq: math.MaxInt64, AtStep: 32, WindowHours: 24, CycleMaxHops: 3},
			domain.GraphFeatures{FirstTransferBetweenPair: true, DestInDegree: 4, DestFanInWindow: 1, DestAccountAgeSteps: idade(30)},
		},
		{
			"contas que nunca apareceram: tudo zerado, idade nula e sem erro",
			domain.FeatureQuery{Orig: "ZZ", Dest: "YY", BeforeSeq: math.MaxInt64, AtStep: 32, WindowHours: 24, CycleMaxHops: 3},
			domain.GraphFeatures{FirstTransferBetweenPair: true},
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			got, err := c.Features(context.Background(), caso.q)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if !igualIdade(got.DestAccountAgeSteps, caso.want.DestAccountAgeSteps) {
				t.Errorf("idade = %v, esperada %v", ponteiro(got.DestAccountAgeSteps), ponteiro(caso.want.DestAccountAgeSteps))
			}
			got.DestAccountAgeSteps, caso.want.DestAccountAgeSteps = nil, nil
			if got != caso.want {
				t.Errorf("features = %+v\nesperadas  %+v", got, caso.want)
			}
		})
	}
}

func ponteiro(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestSubgraphDaVizinhanca(t *testing.T) {
	c := clienteDeTeste(t)
	carregarFixture(t, c)

	sg, err := c.Subgraph(context.Background(), "A", "X", math.MaxInt64, 32, 50)
	if err != nil {
		t.Fatal(err)
	}
	// A tem 3 arestas (A para B, C para A, A para B) e X tem 4 (D, E, F, G).
	if len(sg.Edges) != 7 {
		t.Errorf("esperava 7 arestas, vieram %d: %+v", len(sg.Edges), sg.Edges)
	}
	ids := map[string]bool{}
	for _, e := range sg.Edges {
		if ids[e.ID] {
			t.Errorf("aresta repetida: %s", e.ID)
		}
		ids[e.ID] = true
	}
	tem := map[string]bool{}
	for _, n := range sg.Nodes {
		tem[n] = true
	}
	for _, esperado := range []string{"A", "X", "B", "C", "D", "E", "F", "G"} {
		if !tem[esperado] {
			t.Errorf("faltou o nó %s em %v", esperado, sg.Nodes)
		}
	}

	limitado, err := c.Subgraph(context.Background(), "A", "X", math.MaxInt64, 32, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(limitado.Edges) != 4 {
		t.Errorf("com limite de 2 por conta esperava 4 arestas, vieram %d", len(limitado.Edges))
	}
}

func TestSubgraphDeContasQueNaoExistem(t *testing.T) {
	c := clienteDeTeste(t)
	carregarFixture(t, c)
	sg, err := c.Subgraph(context.Background(), "ZZ", "YY", math.MaxInt64, 32, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(sg.Edges) != 0 || len(sg.Nodes) != 2 {
		t.Errorf("subgrafo = %+v", sg)
	}
}
```

Guarda contra vazamento: as features de uma transação no grafo completo precisam ser idênticas às calculadas num grafo que só tem as linhas anteriores a ela.

```go file=internal/infrastructure/neo4j/leak_integration_test.go
//go:build integration

package neo4j

import (
	"bufio"
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fraud-detector-jev/internal/domain"
	"fraud-detector-jev/internal/infrastructure/csv"
)

// Para cada transação da fixture, compara as features do grafo completo
// (cortadas por seq) com as de um grafo que só contém as linhas anteriores.
// Se alguma query espiar o futuro, os dois resultados divergem.
func TestFeaturesSoEnxergamOPassado(t *testing.T) {
	c := clienteDeTeste(t)

	var txs []domain.Transaction
	err := csv.Stream(context.Background(), caminhoFixture(), func(tx domain.Transaction) error {
		txs = append(txs, tx)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	linhas := linhasDoArquivo(t, caminhoFixture())

	carregarFixture(t, c)
	cheio := map[int64]domain.GraphFeatures{}
	for _, tx := range txs {
		f, err := c.Features(context.Background(), domain.FeatureQuery{
			Orig: tx.Orig, Dest: tx.Dest, BeforeSeq: tx.Seq, AtStep: tx.Step, WindowHours: 24, CycleMaxHops: 3,
		})
		if err != nil {
			t.Fatal(err)
		}
		cheio[tx.Seq] = f
	}

	for _, tx := range txs {
		limparGrafo(t, c)
		truncado := filepath.Join(t.TempDir(), "truncado.csv")
		// linhas[0] é o cabeçalho, então as tx.Seq linhas anteriores são linhas[1 : 1+Seq].
		conteudo := strings.Join(linhas[:1+int(tx.Seq)], "\n") + "\n"
		if err := os.WriteFile(truncado, []byte(conteudo), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Load(context.Background(), LoadOptions{CSVPath: truncado, BatchSize: 4}); err != nil {
			t.Fatalf("carga truncada no seq %d: %v", tx.Seq, err)
		}
		so, err := c.Features(context.Background(), domain.FeatureQuery{
			Orig: tx.Orig, Dest: tx.Dest, BeforeSeq: math.MaxInt64, AtStep: tx.Step, WindowHours: 24, CycleMaxHops: 3,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !igualIdade(so.DestAccountAgeSteps, cheio[tx.Seq].DestAccountAgeSteps) {
			t.Errorf("seq %d: idade diverge (completo %v, truncado %v)", tx.Seq, ponteiro(cheio[tx.Seq].DestAccountAgeSteps), ponteiro(so.DestAccountAgeSteps))
		}
		so.DestAccountAgeSteps = nil
		esperado := cheio[tx.Seq]
		esperado.DestAccountAgeSteps = nil
		if so != esperado {
			t.Errorf("seq %d espiou o futuro:\n  grafo completo  %+v\n  grafo truncado  %+v", tx.Seq, esperado, so)
		}
	}
}

func linhasDoArquivo(t *testing.T, caminho string) []string {
	t.Helper()
	f, err := os.Open(caminho)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var linhas []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) != "" {
			linhas = append(linhas, sc.Text())
		}
	}
	return linhas
}
```

- [ ] **Passo 2: rodar e ver falhar**

Rode: `go test ./internal/infrastructure/neo4j/ -run 'FeaturesQuery|FeaturesRecusa'`
Esperado: FAIL com `undefined: FeaturesQuery`.

- [ ] **Passo 3: implementar as queries**

```go file=internal/infrastructure/neo4j/features_queries.go
package neo4j

import "fmt"

// featuresQueryTemplate calcula todas as features de uma transação numa ida só
// ao banco. O %d é o limite de saltos do ciclo, porque o Cypher não aceita
// parâmetro no tamanho de caminho variável. Cada CALL devolve exatamente uma
// linha, mesmo quando a conta nunca apareceu (count dá 0 e min dá null).
//
// Regra de tempo em todos os blocos: só arestas com seq menor que $beforeSeq e
// step menor ou igual a $atStep. A janela de K horas é step > atStep - window.
// Esta query não pode ler o gabarito (nada de isFraud nem isFlagged).
const featuresQueryTemplate = `
CALL {
  MATCH (:Account {id: $orig})-[t:TRANSFERRED]->(:Account {id: $dest})
  WHERE t.seq < $beforeSeq AND t.step <= $atStep
  RETURN count(t) AS pairCount
}
CALL {
  MATCH (s:Account)-[t:TRANSFERRED]->(:Account {id: $dest})
  WHERE t.seq < $beforeSeq AND t.step <= $atStep
  RETURN count(DISTINCT s) AS destInDegree
}
CALL {
  MATCH (s:Account)-[t:TRANSFERRED]->(:Account {id: $dest})
  WHERE t.seq < $beforeSeq AND t.step <= $atStep AND t.step > $atStep - $window
  RETURN count(DISTINCT s) AS destFanIn
}
CALL {
  MATCH (:Account {id: $dest})-[t:TRANSFERRED]->(r:Account)
  WHERE t.seq < $beforeSeq AND t.step <= $atStep AND t.step > $atStep - $window
  RETURN count(DISTINCT r) AS destFanOut
}
CALL {
  MATCH (:Account {id: $orig})-[t:TRANSFERRED]->(r:Account)
  WHERE t.seq < $beforeSeq AND t.step <= $atStep AND t.step > $atStep - $window
  RETURN count(DISTINCT r) AS origFanOut
}
CALL {
  MATCH (:Account {id: $dest})-[t:TRANSFERRED]-(:Account)
  WHERE t.seq < $beforeSeq AND t.step <= $atStep
  RETURN min(t.step) AS firstStep
}
CALL {
  OPTIONAL MATCH p = (:Account {id: $dest})-[:TRANSFERRED*1..%d]->(:Account {id: $orig})
  WHERE all(e IN relationships(p) WHERE e.seq < $beforeSeq AND e.step <= $atStep)
  WITH p LIMIT 1
  RETURN p IS NOT NULL AS cycle
}
RETURN pairCount, destInDegree, destFanIn, destFanOut, origFanOut, firstStep, cycle
`

// subgraphQuery devolve as arestas mais recentes que tocam uma conta. Aqui o
// isFraud pode aparecer, porque é só para pintar o desenho e não entra em
// nenhuma feature.
const subgraphQuery = `
MATCH (:Account {id: $id})-[t:TRANSFERRED]-(:Account)
WHERE t.seq < $beforeSeq AND t.step <= $atStep
RETURN elementId(t) AS id, startNode(t).id AS src, endNode(t).id AS dst,
       t.type AS type, t.amount AS amount, t.step AS step, t.isFraud AS isFraud
ORDER BY t.seq DESC
LIMIT $limit
`

// FeaturesQuery devolve o texto Cypher das features com o limite de saltos do
// ciclo já colocado. Aceita de 1 a 5 saltos.
func FeaturesQuery(cycleMaxHops int) (string, error) {
	if cycleMaxHops < 1 || cycleMaxHops > 5 {
		return "", fmt.Errorf("cycle_max_hops precisa estar entre 1 e 5, veio %d", cycleMaxHops)
	}
	return fmt.Sprintf(featuresQueryTemplate, cycleMaxHops), nil
}
```

- [ ] **Passo 4: implementar o cliente de features e subgrafo**

```go file=internal/infrastructure/neo4j/features.go
package neo4j

import (
	"context"
	"fmt"

	neo "github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"fraud-detector-jev/internal/domain"
)

// Features calcula as features de grafo de uma transação, olhando só o passado
// dela (veja domain.FeatureQuery). Implementa service.GraphFeatureSource.
func (c *Client) Features(ctx context.Context, q domain.FeatureQuery) (domain.GraphFeatures, error) {
	query, err := FeaturesQuery(q.CycleMaxHops)
	if err != nil {
		return domain.GraphFeatures{}, err
	}
	res, err := c.read(ctx, query, map[string]any{
		"orig":      q.Orig,
		"dest":      q.Dest,
		"beforeSeq": q.BeforeSeq,
		"atStep":    q.AtStep,
		"window":    q.WindowHours,
	})
	if err != nil {
		return domain.GraphFeatures{}, fmt.Errorf("consultando features no Neo4j: %w", err)
	}
	if len(res.Records) != 1 {
		return domain.GraphFeatures{}, fmt.Errorf("a query de features devolveu %d linhas, esperava 1", len(res.Records))
	}
	return decodeFeatures(res.Records[0], q.AtStep)
}

func decodeFeatures(rec *neo.Record, atStep int) (domain.GraphFeatures, error) {
	var gf domain.GraphFeatures
	ints := []struct {
		key  string
		dest *int
	}{
		{"destInDegree", &gf.DestInDegree},
		{"destFanIn", &gf.DestFanInWindow},
		{"destFanOut", &gf.DestFanOutWindow},
		{"origFanOut", &gf.OrigFanOutWindow},
	}
	for _, f := range ints {
		n, err := recordInt(rec, f.key)
		if err != nil {
			return gf, err
		}
		*f.dest = int(n)
	}

	pair, err := recordInt(rec, "pairCount")
	if err != nil {
		return gf, err
	}
	gf.FirstTransferBetweenPair = pair == 0

	first, ok := rec.Get("firstStep")
	if !ok {
		return gf, fmt.Errorf("a coluna firstStep não veio no resultado")
	}
	if first != nil {
		n, isInt := first.(int64)
		if !isInt {
			return gf, fmt.Errorf("firstStep com tipo inesperado %T", first)
		}
		age := atStep - int(n)
		gf.DestAccountAgeSteps = &age
	}

	cycle, ok := rec.Get("cycle")
	cycleBool, isBool := cycle.(bool)
	if !ok || !isBool {
		return gf, fmt.Errorf("a coluna cycle veio ausente ou não é booleana (%T)", cycle)
	}
	gf.CycleDetected = cycleBool
	return gf, nil
}

// Subgraph devolve a vizinhança de 1 salto da origem e do destino, com no
// máximo limitPerAccount arestas mais recentes por conta. Implementa
// service.SubgraphSource.
func (c *Client) Subgraph(ctx context.Context, orig, dest string, beforeSeq int64, atStep, limitPerAccount int) (domain.Subgraph, error) {
	sg := domain.Subgraph{Nodes: []string{}, Edges: []domain.SubEdge{}}
	nodes := map[string]bool{}
	edges := map[string]bool{}
	addNode := func(id string) {
		if !nodes[id] {
			nodes[id] = true
			sg.Nodes = append(sg.Nodes, id)
		}
	}
	addNode(orig)
	addNode(dest)

	consultadas := map[string]bool{}
	for _, id := range []string{orig, dest} {
		if consultadas[id] {
			continue
		}
		consultadas[id] = true
		res, err := c.read(ctx, subgraphQuery, map[string]any{
			"id": id, "beforeSeq": beforeSeq, "atStep": atStep, "limit": limitPerAccount,
		})
		if err != nil {
			return domain.Subgraph{}, fmt.Errorf("consultando o subgrafo de %s: %w", id, err)
		}
		for _, rec := range res.Records {
			edge, err := decodeEdge(rec)
			if err != nil {
				return domain.Subgraph{}, err
			}
			if edges[edge.ID] {
				continue
			}
			edges[edge.ID] = true
			sg.Edges = append(sg.Edges, edge)
			addNode(edge.Src)
			addNode(edge.Dst)
		}
	}
	return sg, nil
}

func decodeEdge(rec *neo.Record) (domain.SubEdge, error) {
	get := func(key string) any {
		v, _ := rec.Get(key)
		return v
	}
	id, _ := get("id").(string)
	src, _ := get("src").(string)
	dst, _ := get("dst").(string)
	if id == "" || src == "" || dst == "" {
		return domain.SubEdge{}, fmt.Errorf("aresta do subgrafo incompleta: %v", rec.Values)
	}
	typ, _ := get("type").(string)
	amount, _ := get("amount").(float64)
	step, _ := get("step").(int64)
	isFraud, _ := get("isFraud").(bool)
	return domain.SubEdge{ID: id, Src: src, Dst: dst, Type: typ, Amount: amount, Step: int(step), IsFraud: isFraud}, nil
}
```

- [ ] **Passo 5: rodar tudo e ver passar**

Rode: `go test -race ./internal/infrastructure/neo4j/ && ./scripts/test-integration.sh`
Esperado: o teste unitário passa e o script termina com `ok` para os testes de integração (features nos 10 cenários, subgrafo e a guarda de vazamento).

- [ ] **Passo 6: commitar**

```bash
git add internal/infrastructure/neo4j
git commit -m "feat: adiciona queries de features, subgrafo e guardas contra vazamento"
```

---
## Tarefa 9: armazenamento das execuções (runstore)

**Arquivos:**
- Criar: `internal/infrastructure/runstore/runstore.go`
- Testar: `internal/infrastructure/runstore/runstore_test.go`

**Interfaces:**
- Consumes: `domain.RunSummary`, `domain.Result`, `domain.ResultsSink`, `domain.DatasetStats`, `domain.ErrNotFound`.
- Produz (`*runstore.Store`):
  - `runstore.New(dir string) (*Store, error)`
  - `NewRunID(now time.Time) string`, no formato `run_YYYYMMDD_HHMMSS_xxxx`
  - `SaveSummary(domain.RunSummary) error` (gravação atômica)
  - `ReadSummary(id string) (domain.RunSummary, error)`
  - `List() ([]domain.RunSummary, error)`, da mais nova para a mais antiga
  - `OpenResultsWriter(id string) (domain.ResultsSink, error)` e `OpenResults(id string) (io.ReadCloser, error)`
  - `MarkStaleRunsFailed(now time.Time) (int, error)`
  - `WriteDatasetStats(domain.DatasetStats) error` e `ReadDatasetStats() (domain.DatasetStats, error)`
  - Todos devolvem um erro que casa com `errors.Is(err, domain.ErrNotFound)` quando o id é inválido ou não existe.

Layout em disco: `<dir>/<id>/summary.json`, `<dir>/<id>/results.jsonl` e `<dir>/dataset_stats.json`. O id é validado por regex antes de virar caminho, para `../` nunca escapar da pasta.

- [ ] **Passo 1: escrever o teste que falha**

```go file=internal/infrastructure/runstore/runstore_test.go
package runstore

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fraud-detector-jev/internal/domain"
)

func novaLoja(t *testing.T) *Store {
	t.Helper()
	s, err := New(filepath.Join(t.TempDir(), "runs"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func resumo(id string, status domain.RunStatus, inicio time.Time) domain.RunSummary {
	return domain.RunSummary{
		RunID: id, Status: status, StartedAt: inicio,
		Params: domain.RunParams{N: 100, FraudPct: 10, Seed: 42, Model: "tev1:4b", Graph: true, Types: []string{"TRANSFER"}},
	}
}

func TestNewRunIDTemFormatoValido(t *testing.T) {
	s := novaLoja(t)
	id := s.NewRunID(time.Date(2026, 9, 30, 14, 30, 5, 0, time.UTC))
	if !idPattern.MatchString(id) || id[:20] != "run_20260930_143005_" {
		t.Errorf("id = %q", id)
	}
	if s.NewRunID(time.Now()) == s.NewRunID(time.Now()) {
		t.Error("dois ids seguidos não deveriam ser iguais")
	}
}

func TestSaveELerResumo(t *testing.T) {
	s := novaLoja(t)
	inicio := time.Date(2026, 9, 30, 14, 30, 5, 0, time.UTC)
	fim := inicio.Add(90 * time.Second)
	original := resumo("run_20260930_143005_ab12", domain.StatusDone, inicio)
	original.FinishedAt = &fim
	original.DurationS = 90
	original.Counts = &domain.Counts{Confusion: domain.Confusion{TP: 5, TN: 90}, Errors: 1}
	original.Sample = &domain.SampleInfo{N: 100, Prevalence: 0.003}

	if err := s.SaveSummary(original); err != nil {
		t.Fatal(err)
	}
	volta, err := s.ReadSummary(original.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if volta.RunID != original.RunID || volta.Status != domain.StatusDone || !volta.StartedAt.Equal(inicio) ||
		volta.FinishedAt == nil || !volta.FinishedAt.Equal(fim) || volta.Counts == nil || volta.Counts.TP != 5 ||
		volta.Counts.Errors != 1 || volta.Sample.Prevalence != 0.003 || volta.Params.Model != "tev1:4b" {
		t.Errorf("resumo voltou diferente: %+v", volta)
	}

	// Regravar substitui.
	original.Status = domain.StatusFailed
	if err := s.SaveSummary(original); err != nil {
		t.Fatal(err)
	}
	volta, _ = s.ReadSummary(original.RunID)
	if volta.Status != domain.StatusFailed {
		t.Errorf("status = %s", volta.Status)
	}
}

func TestIdsInvalidosOuInexistentesDaoNotFound(t *testing.T) {
	s := novaLoja(t)
	for _, id := range []string{"", "../etc/passwd", "run_x", "run_20260930_143005_ab12/../..", "RUN_20260930_143005_ab12", "run_20260930_143005_zzzz"} {
		if _, err := s.ReadSummary(id); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("ReadSummary(%q) = %v, esperava ErrNotFound", id, err)
		}
		if _, err := s.OpenResults(id); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("OpenResults(%q) = %v, esperava ErrNotFound", id, err)
		}
	}
	// Id com formato certo, mas que nunca foi gravado.
	if _, err := s.ReadSummary("run_20260930_143005_ab12"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("id inexistente = %v", err)
	}
	if _, err := s.OpenResults("run_20260930_143005_ab12"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("results inexistente = %v", err)
	}
	if err := s.SaveSummary(resumo("../fora", domain.StatusRunning, time.Now())); err == nil {
		t.Error("SaveSummary com id malicioso deveria falhar")
	}
}

func TestListOrdenaDaMaisNovaParaAMaisAntigaEIgnoraLixo(t *testing.T) {
	s := novaLoja(t)
	base := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	for i, id := range []string{"run_20260930_100000_aaaa", "run_20260930_120000_bbbb", "run_20260930_110000_cccc"} {
		if err := s.SaveSummary(resumo(id, domain.StatusDone, base.Add(time.Duration([]int{0, 2, 1}[i])*time.Hour))); err != nil {
			t.Fatal(err)
		}
	}
	// Lixo que não pode quebrar a listagem.
	if err := os.WriteFile(filepath.Join(s.dir, "solto.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(s.dir, "pasta-qualquer"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(s.dir, "run_20260930_130000_dddd"), 0o755); err != nil { // sem summary.json
		t.Fatal(err)
	}

	lista, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(lista) != 3 || lista[0].RunID != "run_20260930_120000_bbbb" || lista[1].RunID != "run_20260930_110000_cccc" || lista[2].RunID != "run_20260930_100000_aaaa" {
		t.Errorf("lista = %+v", lista)
	}
}

func TestResultadosSaoGravadosELidosLinhaALinha(t *testing.T) {
	s := novaLoja(t)
	id := "run_20260930_143005_ab12"
	if err := s.SaveSummary(resumo(id, domain.StatusRunning, time.Now())); err != nil {
		t.Fatal(err)
	}
	w, err := s.OpenResultsWriter(id)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		noul := 0.5 + float64(i)/10
		if err := w.Write(domain.Result{TransactionID: "TX_" + string(rune('0'+i)), Seq: int64(i), Noul: &noul}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	r, err := s.OpenResults(id)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	sc := bufio.NewScanner(r)
	linhas := 0
	for sc.Scan() {
		var res domain.Result
		if err := json.Unmarshal(sc.Bytes(), &res); err != nil {
			t.Fatalf("linha %d não é JSON: %v", linhas, err)
		}
		if res.Seq != int64(linhas) || res.Noul == nil {
			t.Errorf("linha %d = %+v", linhas, res)
		}
		linhas++
	}
	if linhas != 3 {
		t.Errorf("linhas = %d", linhas)
	}
}

func TestMarkStaleRunsFailed(t *testing.T) {
	s := novaLoja(t)
	inicio := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	_ = s.SaveSummary(resumo("run_20260930_100000_aaaa", domain.StatusRunning, inicio))
	_ = s.SaveSummary(resumo("run_20260930_110000_bbbb", domain.StatusDone, inicio))

	agora := inicio.Add(time.Minute)
	n, err := s.MarkStaleRunsFailed(agora)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	a, _ := s.ReadSummary("run_20260930_100000_aaaa")
	b, _ := s.ReadSummary("run_20260930_110000_bbbb")
	if a.Status != domain.StatusFailed || a.Error == "" || a.FinishedAt == nil || a.DurationS != 60 {
		t.Errorf("a = %+v", a)
	}
	if b.Status != domain.StatusDone {
		t.Errorf("b não deveria mudar: %+v", b)
	}
}

func TestEstatisticasDoDataset(t *testing.T) {
	s := novaLoja(t)
	if _, err := s.ReadDatasetStats(); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("sem arquivo deveria ser ErrNotFound, veio %v", err)
	}
	want := domain.DatasetStats{TotalRows: 10, FraudRows: 1, MaxStep: 5, ByType: map[string]domain.TypeStats{"TRANSFER": {Rows: 10, Frauds: 1}}}
	if err := s.WriteDatasetStats(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReadDatasetStats()
	if err != nil || got.TotalRows != 10 || got.ByType["TRANSFER"].Frauds != 1 || got.MaxStep != 5 {
		t.Errorf("got=%+v err=%v", got, err)
	}
}
```

- [ ] **Passo 2: rodar e ver falhar**

Rode: `go test ./internal/infrastructure/runstore/`
Esperado: FAIL com `undefined: New`.

- [ ] **Passo 3: implementar**

```go file=internal/infrastructure/runstore/runstore.go
package runstore

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"fraud-detector-jev/internal/domain"
)

// idPattern é a única forma aceita de id. Ele vira nome de pasta, então nada
// fora desse formato (como ../) pode passar.
var idPattern = regexp.MustCompile(`^run_\d{8}_\d{6}_[0-9a-f]{4}$`)

// Store guarda as execuções em disco.
type Store struct{ dir string }

// New cria a pasta se preciso.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("criando a pasta das execuções: %w", err)
	}
	return &Store{dir: dir}, nil
}

// NewRunID gera um id como run_20260930_143005_ab12.
func (s *Store) NewRunID(now time.Time) string {
	var suffix [2]byte
	_, _ = rand.Read(suffix[:])
	return fmt.Sprintf("run_%s_%s", now.UTC().Format("20060102_150405"), hex.EncodeToString(suffix[:]))
}

func (s *Store) runDir(id string) (string, error) {
	if !idPattern.MatchString(id) {
		return "", fmt.Errorf("id de execução inválido %q: %w", id, domain.ErrNotFound)
	}
	return filepath.Join(s.dir, id), nil
}

// SaveSummary grava o summary.json de forma atômica (arquivo temporário e rename).
func (s *Store) SaveSummary(sum domain.RunSummary) error {
	dir, err := s.runDir(sum.RunID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("criando a pasta da execução: %w", err)
	}
	raw, err := json.MarshalIndent(sum, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, "summary.json"), raw)
}

// ReadSummary lê o summary.json de uma execução.
func (s *Store) ReadSummary(id string) (domain.RunSummary, error) {
	dir, err := s.runDir(id)
	if err != nil {
		return domain.RunSummary{}, err
	}
	var sum domain.RunSummary
	if err := readJSON(filepath.Join(dir, "summary.json"), &sum); err != nil {
		return domain.RunSummary{}, err
	}
	return sum, nil
}

// List devolve todas as execuções, da mais nova para a mais antiga. Pastas sem
// summary.json ou com arquivo ilegível são ignoradas.
func (s *Store) List() ([]domain.RunSummary, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("listando as execuções: %w", err)
	}
	var out []domain.RunSummary
	for _, e := range entries {
		if !e.IsDir() || !idPattern.MatchString(e.Name()) {
			continue
		}
		sum, err := s.ReadSummary(e.Name())
		if err != nil {
			continue
		}
		out = append(out, sum)
	}
	slices.SortFunc(out, func(a, b domain.RunSummary) int { return b.StartedAt.Compare(a.StartedAt) })
	return out, nil
}

type resultsWriter struct {
	f   *os.File
	w   *bufio.Writer
	enc *json.Encoder
}

// OpenResultsWriter cria o results.jsonl da execução (a pasta precisa existir,
// ou seja, o SaveSummary já rodou).
func (s *Store) OpenResultsWriter(id string) (domain.ResultsSink, error) {
	dir, err := s.runDir(id)
	if err != nil {
		return nil, err
	}
	f, err := os.Create(filepath.Join(dir, "results.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("criando o results.jsonl: %w", err)
	}
	w := bufio.NewWriterSize(f, 1<<20)
	return &resultsWriter{f: f, w: w, enc: json.NewEncoder(w)}, nil
}

func (r *resultsWriter) Write(res domain.Result) error { return r.enc.Encode(res) }

func (r *resultsWriter) Close() error {
	return errors.Join(r.w.Flush(), r.f.Close())
}

// OpenResults abre o results.jsonl para leitura. Quem chama precisa fechar.
func (s *Store) OpenResults(id string) (io.ReadCloser, error) {
	dir, err := s.runDir(id)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(dir, "results.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("results da execução %s: %w", id, domain.ErrNotFound)
	}
	return f, err
}

// MarkStaleRunsFailed marca como failed as execuções que ficaram "running" de
// uma vida anterior do servidor. Devolve quantas mudou.
func (s *Store) MarkStaleRunsFailed(now time.Time) (int, error) {
	lista, err := s.List()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, sum := range lista {
		if sum.Status != domain.StatusRunning {
			continue
		}
		fim := now
		sum.Status = domain.StatusFailed
		sum.Error = "execução interrompida porque o servidor foi reiniciado"
		sum.FinishedAt = &fim
		sum.DurationS = now.Sub(sum.StartedAt).Seconds()
		if err := s.SaveSummary(sum); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// WriteDatasetStats grava o dataset_stats.json (o seeder chama isso).
func (s *Store) WriteDatasetStats(stats domain.DatasetStats) error {
	raw, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(s.dir, "dataset_stats.json"), raw)
}

// ReadDatasetStats lê o dataset_stats.json. Devolve ErrNotFound se o seeder
// ainda não rodou.
func (s *Store) ReadDatasetStats() (domain.DatasetStats, error) {
	var stats domain.DatasetStats
	if err := readJSON(filepath.Join(s.dir, "dataset_stats.json"), &stats); err != nil {
		return domain.DatasetStats{}, err
	}
	return stats, nil
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSON(path string, into any) error {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s: %w", filepath.Base(path), domain.ErrNotFound)
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, into)
}
```

- [ ] **Passo 4: rodar e ver passar**

Rode: `go test -race ./internal/infrastructure/runstore/`
Esperado: `ok`.

- [ ] **Passo 5: commitar**

```bash
git add internal/infrastructure/runstore
git commit -m "feat: adiciona armazenamento das execuções em disco"
```

---

## Tarefa 10: pipeline do benchmark

**Arquivos:**
- Criar: `internal/service/benchmark_pipeline.go`
- Testar: `internal/service/fakes_test.go` (dublês compartilhados com a Tarefa 11), `internal/service/benchmark_pipeline_test.go`

**Interfaces:**
- Consumes: `service.GraphFeatureSource`, `service.Classifier`, `service.BuildState`, `service.FeatureQueryFor`, `domain.*`.
- Produz:
  - `service.PipelineConfig{Graph bool, Workers, ClassifyWorkers, WindowHours, CycleMaxHops int, Threshold float64, Warmup int}`
  - `service.Pipeline{Features GraphFeatureSource, Classifier Classifier}`
  - `service.PipelineOutput{Counts domain.Counts, Completed int, Latency domain.LatencyBlock, Duration time.Duration, TPS float64, TokensTotal int64}`
  - `(Pipeline).Run(ctx, cfg PipelineConfig, txs []domain.Transaction, sink func(domain.Result) error, onProgress func(done int)) (PipelineOutput, error)`

Estágios: um alimentador coloca as transações num channel, `Workers` goroutines calculam as features no Neo4j (pulado se `Graph` for falso) e montam o `state`, `ClassifyWorkers` goroutines chamam o modelo e um único coletor grava cada resultado, atualiza a matriz e junta as latências.

Aquecimento (não entra na conta, e se falhar a execução nem começa): `Warmup` chamadas ao modelo e, se `Graph` for verdadeiro, uma consulta de features falsa **em paralelo por worker**. O aquecimento do Neo4j existe porque, sem ele, as primeiras consultas pagam a abertura das conexões e a compilação do plano da query ao mesmo tempo, e isso apareceu como um P50 de uns 600 ms num grafo de 11 arestas quando o plano foi verificado ponta a ponta.

Erro de uma transação (Neo4j ou modelo) vira `Result.Error` e ela fica fora das métricas. Cancelamento não vira erro de transação: quem estava em voo é descartado. Se o `sink` falhar, a execução para e devolve esse erro.

- [ ] **Passo 1: escrever os dublês compartilhados**

```go file=internal/service/fakes_test.go
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fraud-detector-jev/internal/domain"
)

// txsDeTeste cria n transações TRANSFER. A cada 3 uma é fraude, com amount 1000
// (as outras têm amount 10). O classificador falso usa o amount para decidir.
func txsDeTeste(n int) []domain.Transaction {
	txs := make([]domain.Transaction, 0, n)
	for i := range n {
		fraude := i%3 == 0
		amount := 10.0
		if fraude {
			amount = 1000
		}
		txs = append(txs, domain.Transaction{
			Seq: int64(i), Step: i, Type: "TRANSFER", Amount: amount,
			Orig: "O" + strconv.Itoa(i), Dest: "D" + strconv.Itoa(i), IsFraud: fraude,
		})
	}
	return txs
}

// fakeFeatures é a fonte de features falsa.
type fakeFeatures struct {
	calls atomic.Int64
	fn    func(domain.FeatureQuery) (domain.GraphFeatures, error)
}

func (f *fakeFeatures) Features(_ context.Context, q domain.FeatureQuery) (domain.GraphFeatures, error) {
	f.calls.Add(1)
	if f.fn != nil {
		return f.fn(q)
	}
	return domain.GraphFeatures{DestInDegree: int(q.BeforeSeq)}, nil
}

// fakeClassifier é o modelo falso. Por padrão devolve 0.9 quando o amount do
// state é 1000 ou mais e 0.1 no resto.
type fakeClassifier struct {
	calls       atomic.Int64
	inflight    atomic.Int64
	maxInflight atomic.Int64
	delay       time.Duration
	fn          func(ctx context.Context, state string) (domain.Decision, error)

	mu     sync.Mutex
	states []string
}

func (f *fakeClassifier) Classify(ctx context.Context, state string) (domain.Decision, error) {
	f.calls.Add(1)
	cur := f.inflight.Add(1)
	defer f.inflight.Add(-1)
	for {
		seen := f.maxInflight.Load()
		if cur <= seen || f.maxInflight.CompareAndSwap(seen, cur) {
			break
		}
	}
	f.mu.Lock()
	f.states = append(f.states, state)
	f.mu.Unlock()

	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.fn != nil {
		return f.fn(ctx, state)
	}
	return domain.Decision{Noul: noulPorValor(state), InputTokens: 10, Attempts: 1, Latency: 2 * time.Millisecond}, nil
}

func (f *fakeClassifier) statesVistos() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.states...)
}

func noulPorValor(state string) float64 {
	var st struct {
		Amount float64 `json:"amount"`
	}
	_ = json.Unmarshal([]byte(state), &st)
	if st.Amount >= 1000 {
		return 0.9
	}
	return 0.1
}

// seqDoState devolve o seq do transaction_id do state, ou -1 (o state de
// aquecimento não tem transaction_id).
func seqDoState(state string) int64 {
	var st struct {
		ID string `json:"transaction_id"`
	}
	if json.Unmarshal([]byte(state), &st) != nil || !strings.HasPrefix(st.ID, "TX_") {
		return -1
	}
	n, err := strconv.ParseInt(strings.TrimPrefix(st.ID, "TX_"), 10, 64)
	if err != nil {
		return -1
	}
	return n
}

// memStore é o armazenamento em memória usado nos testes do RunManager.
type memStore struct {
	mu        sync.Mutex
	nextID    int
	summaries []domain.RunSummary
	results   []domain.Result
	closed    bool
	saveErr   error
}

func (m *memStore) NewRunID(time.Time) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	return fmt.Sprintf("run_teste_%d", m.nextID)
}

func (m *memStore) SaveSummary(s domain.RunSummary) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveErr != nil {
		return m.saveErr
	}
	m.summaries = append(m.summaries, s)
	return nil
}

func (m *memStore) OpenResultsWriter(string) (domain.ResultsSink, error) { return &memSink{m}, nil }

func (m *memStore) ultimoResumo() domain.RunSummary {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.summaries[len(m.summaries)-1]
}

func (m *memStore) qtdResultados() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.results)
}

type memSink struct{ m *memStore }

func (s *memSink) Write(r domain.Result) error {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	s.m.results = append(s.m.results, r)
	return nil
}

func (s *memSink) Close() error {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	s.m.closed = true
	return nil
}
```

- [ ] **Passo 2: escrever os testes do pipeline (que falham)**

```go file=internal/service/benchmark_pipeline_test.go
package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fraud-detector-jev/internal/domain"
)

func cfgPadrao() PipelineConfig {
	return PipelineConfig{Graph: true, Workers: 4, ClassifyWorkers: 2, WindowHours: 24, CycleMaxHops: 3, Threshold: 0.5, Warmup: 3}
}

// rodar executa o pipeline e junta o que o sink recebeu.
func rodar(t *testing.T, p Pipeline, cfg PipelineConfig, txs []domain.Transaction) (PipelineOutput, []domain.Result, error) {
	t.Helper()
	var mu sync.Mutex
	var recebidos []domain.Result
	out, err := p.Run(context.Background(), cfg, txs, func(r domain.Result) error {
		mu.Lock()
		defer mu.Unlock()
		recebidos = append(recebidos, r)
		return nil
	}, nil)
	return out, recebidos, err
}

func TestPipelineComGrafoCalculaMatrizELatencias(t *testing.T) {
	feats, cl := &fakeFeatures{}, &fakeClassifier{}
	out, resultados, err := rodar(t, Pipeline{Features: feats, Classifier: cl}, cfgPadrao(), txsDeTeste(9))
	if err != nil {
		t.Fatal(err)
	}
	if out.Counts.TP != 3 || out.Counts.TN != 6 || out.Counts.FP != 0 || out.Counts.FN != 0 || out.Counts.Errors != 0 {
		t.Errorf("matriz = %+v", out.Counts)
	}
	if out.Completed != 9 || len(resultados) != 9 {
		t.Errorf("completos=%d gravados=%d", out.Completed, len(resultados))
	}
	if out.TokensTotal != 90 {
		t.Errorf("tokens = %d", out.TokensTotal)
	}
	if out.Duration <= 0 || out.TPS <= 0 {
		t.Errorf("duração=%v tps=%v", out.Duration, out.TPS)
	}
	if out.Latency.Jev.P50 != 2 || out.Latency.Neo4j.P99 < 0 {
		t.Errorf("latências = %+v", out.Latency)
	}
	for _, r := range resultados {
		if r.Neo4jMs == nil || r.JevMs == nil || r.Noul == nil || r.GraphFeatures == nil {
			t.Fatalf("resultado incompleto: %+v", r)
		}
	}
	// 9 transações mais 4 consultas de aquecimento (uma por worker do pool).
	if feats.calls.Load() != 13 {
		t.Errorf("chamadas ao Neo4j = %d", feats.calls.Load())
	}
	// 9 transações mais 3 chamadas de aquecimento.
	if cl.calls.Load() != 12 {
		t.Errorf("chamadas ao modelo = %d", cl.calls.Load())
	}
	for _, s := range cl.statesVistos() {
		if seqDoState(s) >= 0 && !strings.Contains(s, "graph_features") {
			t.Errorf("state de execução com grafo sem graph_features: %s", s)
		}
	}
}

func TestPipelineSemGrafoPulaOEstagioDoNeo4j(t *testing.T) {
	feats, cl := &fakeFeatures{}, &fakeClassifier{}
	cfg := cfgPadrao()
	cfg.Graph = false
	out, resultados, err := rodar(t, Pipeline{Features: feats, Classifier: cl}, cfg, txsDeTeste(6))
	if err != nil {
		t.Fatal(err)
	}
	if feats.calls.Load() != 0 {
		t.Errorf("não deveria chamar o Neo4j, chamou %d vezes", feats.calls.Load())
	}
	for _, s := range cl.statesVistos() {
		if strings.Contains(s, "graph_features") {
			t.Errorf("state sem grafo não pode ter graph_features: %s", s)
		}
	}
	for _, r := range resultados {
		if r.Neo4jMs != nil || r.GraphFeatures != nil {
			t.Errorf("resultado sem grafo não pode ter campos de grafo: %+v", r)
		}
	}
	if out.Latency.Neo4j != (domain.LatencyStats{}) || out.Completed != 6 {
		t.Errorf("saída = %+v", out)
	}
}

func TestPipelineTrataErrosPorTransacao(t *testing.T) {
	feats := &fakeFeatures{fn: func(q domain.FeatureQuery) (domain.GraphFeatures, error) {
		if q.BeforeSeq == 2 {
			return domain.GraphFeatures{}, errors.New("timeout da query")
		}
		return domain.GraphFeatures{}, nil
	}}
	cl := &fakeClassifier{fn: func(_ context.Context, state string) (domain.Decision, error) {
		if seqDoState(state) == 3 {
			return domain.Decision{Attempts: 3}, errors.New("modelo caiu")
		}
		return domain.Decision{Noul: noulPorValor(state), InputTokens: 10, Attempts: 1, Latency: time.Millisecond}, nil
	}}
	out, resultados, err := rodar(t, Pipeline{Features: feats, Classifier: cl}, cfgPadrao(), txsDeTeste(9))
	if err != nil {
		t.Fatalf("erro de transação não pode derrubar a execução: %v", err)
	}
	if out.Counts.Errors != 2 || out.Completed != 7 || out.Counts.Total() != 7 || len(resultados) != 9 {
		t.Errorf("saída = %+v gravados=%d", out, len(resultados))
	}
	erros := map[int64]string{}
	for _, r := range resultados {
		if r.Error != "" {
			erros[r.Seq] = r.Error
			if r.Noul != nil {
				t.Errorf("transação com erro não pode ter noul: %+v", r)
			}
		}
	}
	if !strings.HasPrefix(erros[2], "neo4j:") || !strings.HasPrefix(erros[3], "modelo:") || len(erros) != 2 {
		t.Errorf("erros = %v", erros)
	}
}

func TestPipelineAplicaOLimiar(t *testing.T) {
	cl := &fakeClassifier{fn: func(context.Context, string) (domain.Decision, error) {
		return domain.Decision{Noul: 0.6, Attempts: 1, Latency: time.Millisecond}, nil
	}}
	p := Pipeline{Features: &fakeFeatures{}, Classifier: cl}

	cfg := cfgPadrao()
	cfg.Threshold = 0.5
	out, _, err := rodar(t, p, cfg, txsDeTeste(9))
	if err != nil || out.Counts.TP != 3 || out.Counts.FP != 6 {
		t.Errorf("limiar 0.5: %+v err=%v", out.Counts, err)
	}
	cfg.Threshold = 0.7
	out, _, err = rodar(t, p, cfg, txsDeTeste(9))
	if err != nil || out.Counts.FN != 3 || out.Counts.TN != 6 {
		t.Errorf("limiar 0.7: %+v err=%v", out.Counts, err)
	}
}

func TestPipelineCancelaSemContarEmVooComoErro(t *testing.T) {
	// As 3 primeiras chamadas de verdade respondem (a ordem entre elas não
	// importa). Da quarta em diante, tudo fica preso até cancelar. O aquecimento
	// não tem transaction_id e passa direto.
	var chamadas atomic.Int64
	cl := &fakeClassifier{fn: func(ctx context.Context, state string) (domain.Decision, error) {
		if seqDoState(state) >= 0 && chamadas.Add(1) > 3 {
			<-ctx.Done()
			return domain.Decision{}, ctx.Err()
		}
		return domain.Decision{Noul: 0.1, Attempts: 1, Latency: time.Millisecond}, nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var recebidos []domain.Result
	fim := make(chan error, 1)
	go func() {
		_, err := Pipeline{Features: &fakeFeatures{}, Classifier: cl}.Run(ctx, cfgPadrao(), txsDeTeste(20), func(r domain.Result) error {
			recebidos = append(recebidos, r)
			if len(recebidos) == 3 {
				cancel()
			}
			return nil
		}, nil)
		fim <- err
	}()

	select {
	case err := <-fim:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("esperava context.Canceled, veio %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("o pipeline não parou depois do cancelamento")
	}
	if len(recebidos) != 3 {
		t.Errorf("deveriam ter sobrado só os 3 resultados prontos, sobraram %d", len(recebidos))
	}
	for _, r := range recebidos {
		if r.Error != "" {
			t.Errorf("cancelamento virou erro de transação: %+v", r)
		}
	}
}

func TestPipelineFalhaRapidoQuandoOModeloNaoResponde(t *testing.T) {
	feats := &fakeFeatures{}
	cl := &fakeClassifier{fn: func(context.Context, string) (domain.Decision, error) {
		return domain.Decision{}, errors.New("conexão recusada")
	}}
	out, resultados, err := rodar(t, Pipeline{Features: feats, Classifier: cl}, cfgPadrao(), txsDeTeste(9))
	if err == nil || !strings.Contains(err.Error(), "aquecimento") {
		t.Fatalf("esperava erro de aquecimento, veio %v", err)
	}
	if len(resultados) != 0 || feats.calls.Load() != 0 || out.Completed != 0 {
		t.Errorf("não deveria ter processado nada: resultados=%d neo4j=%d", len(resultados), feats.calls.Load())
	}
}

func TestPipelineAqueceOPoolDoNeo4jAntesDeMedir(t *testing.T) {
	var aquecimento, reais atomic.Int64
	feats := &fakeFeatures{fn: func(q domain.FeatureQuery) (domain.GraphFeatures, error) {
		if q.Orig == warmupAccount {
			aquecimento.Add(1)
		} else {
			reais.Add(1)
		}
		return domain.GraphFeatures{}, nil
	}}
	out, resultados, err := rodar(t, Pipeline{Features: feats, Classifier: &fakeClassifier{}}, cfgPadrao(), txsDeTeste(6))
	if err != nil {
		t.Fatal(err)
	}
	if aquecimento.Load() != 4 || reais.Load() != 6 {
		t.Errorf("aquecimento=%d reais=%d, esperava 4 e 6 (4 workers)", aquecimento.Load(), reais.Load())
	}
	if out.Completed != 6 || len(resultados) != 6 {
		t.Errorf("o aquecimento não pode virar resultado: completos=%d gravados=%d", out.Completed, len(resultados))
	}

	// Sem grafo não há o que aquecer no Neo4j.
	semGrafo := cfgPadrao()
	semGrafo.Graph = false
	aquecimento.Store(0)
	reais.Store(0)
	if _, _, err := rodar(t, Pipeline{Features: feats, Classifier: &fakeClassifier{}}, semGrafo, txsDeTeste(3)); err != nil {
		t.Fatal(err)
	}
	if aquecimento.Load() != 0 || reais.Load() != 0 {
		t.Errorf("sem grafo não pode tocar no Neo4j: aquecimento=%d reais=%d", aquecimento.Load(), reais.Load())
	}
}

func TestPipelineFalhaRapidoQuandoONeo4jNaoResponde(t *testing.T) {
	feats := &fakeFeatures{fn: func(domain.FeatureQuery) (domain.GraphFeatures, error) {
		return domain.GraphFeatures{}, errors.New("conexão recusada")
	}}
	cl := &fakeClassifier{}
	out, resultados, err := rodar(t, Pipeline{Features: feats, Classifier: cl}, cfgPadrao(), txsDeTeste(9))
	if err == nil || !strings.Contains(err.Error(), "Neo4j") || !strings.Contains(err.Error(), "aquecimento") {
		t.Fatalf("esperava erro de aquecimento do Neo4j, veio %v", err)
	}
	if len(resultados) != 0 || out.Completed != 0 {
		t.Errorf("não deveria ter processado nada: %d resultados", len(resultados))
	}
}

func TestPipelinePropagaOErroDoSink(t *testing.T) {
	falha := errors.New("disco cheio")
	n := 0
	_, err := Pipeline{Features: &fakeFeatures{}, Classifier: &fakeClassifier{}}.Run(
		context.Background(), cfgPadrao(), txsDeTeste(30),
		func(domain.Result) error {
			n++
			if n == 2 {
				return falha
			}
			return nil
		}, nil)
	if !errors.Is(err, falha) {
		t.Fatalf("esperava o erro do sink, veio %v", err)
	}
}

func TestPipelineRespeitaOLimiteDeChamadasSimultaneas(t *testing.T) {
	cl := &fakeClassifier{delay: 20 * time.Millisecond}
	cfg := cfgPadrao()
	cfg.ClassifyWorkers = 2
	cfg.Warmup = 0
	if _, _, err := rodar(t, Pipeline{Features: &fakeFeatures{}, Classifier: cl}, cfg, txsDeTeste(12)); err != nil {
		t.Fatal(err)
	}
	if vistas := cl.maxInflight.Load(); vistas < 1 || vistas > 2 {
		t.Errorf("simultâneas = %d, o limite era 2", vistas)
	}
}

func TestPipelineReportaProgresso(t *testing.T) {
	var vistos []int
	_, err := Pipeline{Features: &fakeFeatures{}, Classifier: &fakeClassifier{}}.Run(
		context.Background(), cfgPadrao(), txsDeTeste(7),
		func(domain.Result) error { return nil },
		func(done int) { vistos = append(vistos, done) })
	if err != nil {
		t.Fatal(err)
	}
	if len(vistos) != 7 || vistos[0] != 1 || vistos[6] != 7 {
		t.Errorf("progresso = %v", vistos)
	}
}

func TestPipelineSemTransacoes(t *testing.T) {
	out, resultados, err := rodar(t, Pipeline{Features: &fakeFeatures{}, Classifier: &fakeClassifier{}}, cfgPadrao(), nil)
	if err != nil || out.Completed != 0 || len(resultados) != 0 || out.TPS != 0 {
		t.Errorf("out=%+v resultados=%d err=%v", out, len(resultados), err)
	}
}

func TestPipelineValidaConfiguracao(t *testing.T) {
	ok := Pipeline{Features: &fakeFeatures{}, Classifier: &fakeClassifier{}}
	semWorkers := cfgPadrao()
	semWorkers.Workers = 0
	if _, _, err := rodar(t, ok, semWorkers, txsDeTeste(3)); err == nil {
		t.Error("Workers 0 deveria dar erro")
	}
	semClassificadores := cfgPadrao()
	semClassificadores.ClassifyWorkers = 0
	if _, _, err := rodar(t, ok, semClassificadores, txsDeTeste(3)); err == nil {
		t.Error("ClassifyWorkers 0 deveria dar erro")
	}
	if _, _, err := rodar(t, Pipeline{Classifier: &fakeClassifier{}}, cfgPadrao(), txsDeTeste(3)); err == nil {
		t.Error("execução com grafo e sem fonte de features deveria dar erro")
	}
}
```

- [ ] **Passo 3: rodar e ver falhar**

Rode: `go test ./internal/service/ -run Pipeline`
Esperado: FAIL com `undefined: Pipeline`.

- [ ] **Passo 4: implementar**

```go file=internal/service/benchmark_pipeline.go
package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"fraud-detector-jev/internal/domain"
)

// warmupState é o state das chamadas de aquecimento. Elas só servem para o
// modelo carregar na memória antes de a medição começar.
const warmupState = `{"warmup":true}`

// warmupAccount é a conta falsa das consultas de aquecimento do Neo4j. Ela
// nunca existe no grafo, então a consulta devolve tudo zerado, mas o Neo4j
// abre a conexão e compila o plano da query do mesmo jeito.
const warmupAccount = "__warmup__"

// PipelineConfig define como uma execução roda. Workers é o pool do Neo4j e
// ClassifyWorkers é quantas chamadas simultâneas ao modelo.
type PipelineConfig struct {
	Graph           bool
	Workers         int
	ClassifyWorkers int
	WindowHours     int
	CycleMaxHops    int
	Threshold       float64
	Warmup          int
}

// PipelineOutput são as métricas consolidadas de uma execução. Completed conta
// só as transações sem erro.
type PipelineOutput struct {
	Counts      domain.Counts
	Completed   int
	Latency     domain.LatencyBlock
	Duration    time.Duration
	TPS         float64
	TokensTotal int64
}

// Pipeline junta a fonte de features e o modelo. Features pode ser nil quando a
// execução é sem grafo.
type Pipeline struct {
	Features   GraphFeatureSource
	Classifier Classifier
}

type job struct {
	res   domain.Result
	state string
}

// Run processa as transações e entrega cada Result ao sink. onProgress, se não
// for nil, recebe quantas transações já foram tratadas. Se o contexto for
// cancelado, devolve o que já foi gravado e context.Canceled.
func (p Pipeline) Run(ctx context.Context, cfg PipelineConfig, txs []domain.Transaction, sink func(domain.Result) error, onProgress func(done int)) (PipelineOutput, error) {
	if cfg.Workers < 1 || cfg.ClassifyWorkers < 1 {
		return PipelineOutput{}, errors.New("Workers e ClassifyWorkers precisam ser pelo menos 1")
	}
	if cfg.Graph && p.Features == nil {
		return PipelineOutput{}, errors.New("execução com grafo precisa de uma fonte de features")
	}
	if err := p.warmup(ctx, cfg.Warmup); err != nil {
		return PipelineOutput{}, err
	}
	if cfg.Graph && cfg.Warmup > 0 {
		if err := p.warmupNeo4j(ctx, cfg); err != nil {
			return PipelineOutput{}, err
		}
	}

	parent := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	in := make(chan domain.Transaction, cfg.Workers*2)
	enriched := make(chan job, cfg.ClassifyWorkers*2)
	results := make(chan domain.Result, cfg.ClassifyWorkers*2)
	start := time.Now()

	go func() {
		defer close(in)
		for _, tx := range txs {
			if !send(ctx, in, tx) {
				return
			}
		}
	}()

	var enrichWG, classifyWG sync.WaitGroup
	for range cfg.Workers {
		enrichWG.Add(1)
		go func() {
			defer enrichWG.Done()
			p.enrichLoop(ctx, cfg, in, enriched, results)
		}()
	}
	for range cfg.ClassifyWorkers {
		classifyWG.Add(1)
		go func() {
			defer classifyWG.Done()
			p.classifyLoop(ctx, enriched, results)
		}()
	}
	go func() {
		enrichWG.Wait()
		close(enriched)
	}()
	go func() {
		enrichWG.Wait()
		classifyWG.Wait()
		close(results)
	}()

	out, sinkErr := collect(cfg, results, sink, onProgress, cancel)
	out.Duration = time.Since(start)
	if out.Duration > 0 {
		out.TPS = float64(out.Completed) / out.Duration.Seconds()
	}
	if sinkErr != nil {
		return out, sinkErr
	}
	return out, parent.Err()
}

// warmup faz algumas chamadas ao modelo antes da medição. Se todas falharem, o
// modelo está fora do ar e a execução nem começa.
func (p Pipeline) warmup(ctx context.Context, n int) error {
	if n <= 0 {
		return nil
	}
	failures := 0
	var last error
	for range n {
		if _, err := p.Classifier.Classify(ctx, warmupState); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			failures++
			last = err
		}
	}
	if failures == n {
		return fmt.Errorf("o modelo não respondeu no aquecimento: %w", last)
	}
	return nil
}

// warmupNeo4j faz uma consulta falsa por worker, todas ao mesmo tempo. Assim o
// pool de conexões já está aberto e o plano da query já está no cache quando a
// medição começa. Se todas falharem, o Neo4j está fora do ar.
func (p Pipeline) warmupNeo4j(ctx context.Context, cfg PipelineConfig) error {
	q := domain.FeatureQuery{
		Orig: warmupAccount, Dest: warmupAccount, BeforeSeq: 0, AtStep: 0,
		WindowHours: cfg.WindowHours, CycleMaxHops: cfg.CycleMaxHops,
	}
	errs := make([]error, cfg.Workers)
	var wg sync.WaitGroup
	for i := range cfg.Workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = p.Features.Features(ctx, q)
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	var failed int
	for _, err := range errs {
		if err != nil {
			failed++
		}
	}
	if failed == len(errs) {
		return fmt.Errorf("o Neo4j não respondeu no aquecimento: %w", errors.Join(errs...))
	}
	return nil
}

func (p Pipeline) enrichLoop(ctx context.Context, cfg PipelineConfig, in <-chan domain.Transaction, enriched chan<- job, results chan<- domain.Result) {
	for tx := range in {
		if ctx.Err() != nil {
			return
		}
		res := domain.NewResult(tx, nil)
		var gf *domain.GraphFeatures

		if cfg.Graph {
			t0 := time.Now()
			f, err := p.Features.Features(ctx, FeatureQueryFor(tx, cfg.WindowHours, cfg.CycleMaxHops))
			ms := millis(time.Since(t0))
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				res.Error = "neo4j: " + err.Error()
				if !send(ctx, results, res) {
					return
				}
				continue
			}
			gf = &f
			res = domain.NewResult(tx, gf)
			res.Neo4jMs = &ms
		}

		state, err := BuildState(tx, gf).JSON()
		if err != nil {
			res.Error = "montando o state: " + err.Error()
			if !send(ctx, results, res) {
				return
			}
			continue
		}
		if !send(ctx, enriched, job{res: res, state: state}) {
			return
		}
	}
}

func (p Pipeline) classifyLoop(ctx context.Context, enriched <-chan job, results chan<- domain.Result) {
	for j := range enriched {
		if ctx.Err() != nil {
			return
		}
		dec, err := p.Classifier.Classify(ctx, j.state)
		res := j.res
		res.Attempts = dec.Attempts
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			res.Error = "modelo: " + err.Error()
		} else {
			noul, ms := dec.Noul, millis(dec.Latency)
			res.Noul, res.JevMs, res.InputTokens = &noul, &ms, dec.InputTokens
		}
		if !send(ctx, results, res) {
			return
		}
	}
}

// collect é o único que grava resultados e soma métricas. Se o sink falhar,
// cancela o resto e continua drenando o channel até ele fechar.
func collect(cfg PipelineConfig, results <-chan domain.Result, sink func(domain.Result) error, onProgress func(int), cancel context.CancelFunc) (PipelineOutput, error) {
	var out PipelineOutput
	var jevMs, neoMs []float64
	var sinkErr error
	done := 0

	for res := range results {
		if sinkErr != nil {
			continue
		}
		if err := sink(res); err != nil {
			sinkErr = fmt.Errorf("gravando o resultado: %w", err)
			cancel()
			continue
		}
		done++
		if onProgress != nil {
			onProgress(done)
		}
		if res.Error != "" {
			out.Counts.Errors++
			continue
		}
		out.Completed++
		out.Counts.Add(res.IsFraud, domain.Decide(*res.Noul, cfg.Threshold))
		jevMs = append(jevMs, *res.JevMs)
		if res.Neo4jMs != nil {
			neoMs = append(neoMs, *res.Neo4jMs)
		}
		out.TokensTotal += int64(res.InputTokens)
	}
	out.Latency = domain.LatencyBlock{Neo4j: domain.ComputeLatency(neoMs), Jev: domain.ComputeLatency(jevMs)}
	return out, sinkErr
}

func send[T any](ctx context.Context, ch chan<- T, v T) bool {
	select {
	case ch <- v:
		return true
	case <-ctx.Done():
		return false
	}
}

func millis(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
```

- [ ] **Passo 5: rodar e ver passar**

Rode: `go test -race -count=3 ./internal/service/`
Esperado: `ok`. O `-count=3` roda três vezes para pegar qualquer teste instável de concorrência.

- [ ] **Passo 6: commitar**

```bash
git add internal/service/benchmark_pipeline.go internal/service/benchmark_pipeline_test.go internal/service/fakes_test.go
git commit -m "feat: adiciona pipeline concorrente do benchmark com métricas"
```

---
## Tarefa 11: gerenciador de execuções

**Arquivos:**
- Criar: `internal/service/run_manager.go`
- Testar: `internal/service/run_manager_test.go`

**Interfaces:**
- Consumes: `service.Sample`, `service.TxSource`, `service.Pipeline`, `service.PipelineConfig`, `domain.RunParams`, `domain.RunSummary`, `domain.ResultsSink` e os dublês da Tarefa 10 (`fakes_test.go`, `fonte` do `sampler_test.go`).
- Produz:
  - `service.RunStore` (interface): `NewRunID(now time.Time) string`, `SaveSummary(domain.RunSummary) error`, `OpenResultsWriter(id string) (domain.ResultsSink, error)`. O `runstore.Store` implementa.
  - `service.RunFactory` = `func(params domain.RunParams) (Pipeline, error)`
  - `service.NewRunManager(store RunStore, source TxSource, factory RunFactory, logger *slog.Logger) *RunManager` (logger nil usa `slog.Default()`)
  - `(*RunManager).Start(params domain.RunParams) (domain.RunSummary, error)`, que devolve `service.ErrBusy` se já houver execução ativa
  - `(*RunManager).Cancel(id string) bool`, `Progress(id string) (domain.Progress, bool)`, `ActiveID() string`, `Wait()`, `Shutdown()`

Comportamento: `Start` cria o pipeline pela fábrica (erro aqui volta na hora e nada é gravado), grava o resumo com status `running` e roda em segundo plano: amostra, pipeline, resumo final. O status final é `done`, `canceled` (contexto cancelado) ou `failed` (qualquer outro erro, com a mensagem em `error`). Só uma execução por vez.

- [ ] **Passo 1: escrever os testes que falham**

```go file=internal/service/run_manager_test.go
package service

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fraud-detector-jev/internal/domain"
)

func paramsDeTeste() domain.RunParams {
	return domain.RunParams{
		N: 30, FraudPct: 50, Seed: 1, Workers: 4, Model: "tev1:4b", Graph: true,
		Types: []string{"TRANSFER"}, WindowHours: 24, CycleMaxHops: 3, MaxInflight: 2, Threshold: 0.5,
	}
}

func novoGerenciador(t *testing.T, txs []domain.Transaction, cl *fakeClassifier) (*RunManager, *memStore) {
	t.Helper()
	store := &memStore{}
	factory := func(domain.RunParams) (Pipeline, error) {
		return Pipeline{Features: &fakeFeatures{}, Classifier: cl}, nil
	}
	m := NewRunManager(store, fonte(txs), factory, nil)
	t.Cleanup(m.Shutdown)
	return m, store
}

// esperar espera a condição ficar verdadeira por até 3 segundos.
func esperar(t *testing.T, descricao string, cond func() bool) {
	t.Helper()
	limite := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(limite) {
			t.Fatalf("passou o tempo esperando: %s", descricao)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRunManagerExecutaEGravaOResumoFinal(t *testing.T) {
	m, store := novoGerenciador(t, txsDeTeste(300), &fakeClassifier{})

	inicial, err := m.Start(paramsDeTeste())
	if err != nil {
		t.Fatal(err)
	}
	if inicial.Status != domain.StatusRunning || inicial.RunID == "" {
		t.Errorf("resumo inicial = %+v", inicial)
	}
	m.Wait()

	sum := store.ultimoResumo()
	if sum.RunID != inicial.RunID || sum.Status != domain.StatusDone || sum.Error != "" || sum.FinishedAt == nil {
		t.Fatalf("resumo final = %+v", sum)
	}
	// 300 transações, uma fraude a cada 3, então 100 fraudes em 300 (prevalência 1/3).
	if sum.Sample == nil || sum.Sample.N != 30 || sum.Sample.NFraud != 15 || sum.Sample.NLegit != 15 ||
		sum.Sample.EligibleRows != 300 || sum.Sample.EligibleFraud != 100 || sum.Sample.Prevalence != 1.0/3 {
		t.Errorf("amostra = %+v", sum.Sample)
	}
	if sum.Counts == nil || sum.Counts.TP != 15 || sum.Counts.TN != 15 || sum.Counts.Errors != 0 {
		t.Errorf("contagens = %+v", sum.Counts)
	}
	want := domain.ComputeMetrics(sum.Counts.Confusion, 1.0/3)
	if sum.Metrics == nil || *sum.Metrics != want {
		t.Errorf("métricas = %+v, esperadas %+v", sum.Metrics, want)
	}
	if sum.Latency == nil || sum.TPS <= 0 || sum.TokensTotal != 300 {
		t.Errorf("latência=%+v tps=%v tokens=%d", sum.Latency, sum.TPS, sum.TokensTotal)
	}
	if store.qtdResultados() != 30 || !store.closed {
		t.Errorf("resultados gravados=%d fechado=%v", store.qtdResultados(), store.closed)
	}
	if m.ActiveID() != "" {
		t.Error("não deveria haver execução ativa depois de terminar")
	}
	if len(store.summaries) < 2 || store.summaries[0].Status != domain.StatusRunning {
		t.Errorf("o primeiro resumo gravado deveria estar running: %+v", store.summaries[0])
	}
}

func TestRunManagerSoAceitaUmaExecucaoPorVez(t *testing.T) {
	liberar := make(chan struct{})
	cl := &fakeClassifier{fn: func(ctx context.Context, state string) (domain.Decision, error) {
		if seqDoState(state) >= 0 {
			select {
			case <-liberar:
			case <-ctx.Done():
				return domain.Decision{}, ctx.Err()
			}
		}
		return domain.Decision{Noul: 0.1, Attempts: 1, Latency: time.Millisecond}, nil
	}}
	m, _ := novoGerenciador(t, txsDeTeste(300), cl)

	if _, err := m.Start(paramsDeTeste()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(paramsDeTeste()); !errors.Is(err, ErrBusy) {
		t.Fatalf("esperava ErrBusy, veio %v", err)
	}
	close(liberar)
	m.Wait()
	if _, err := m.Start(paramsDeTeste()); err != nil {
		t.Fatalf("depois de terminar deveria aceitar outra: %v", err)
	}
}

func TestRunManagerCancelaEPreservaOQueJaFoiGravado(t *testing.T) {
	var chamadas atomic.Int64
	cl := &fakeClassifier{fn: func(ctx context.Context, state string) (domain.Decision, error) {
		if seqDoState(state) >= 0 && chamadas.Add(1) > 5 {
			<-ctx.Done()
			return domain.Decision{}, ctx.Err()
		}
		return domain.Decision{Noul: 0.1, Attempts: 1, Latency: time.Millisecond}, nil
	}}
	m, store := novoGerenciador(t, txsDeTeste(300), cl)

	sum, err := m.Start(paramsDeTeste())
	if err != nil {
		t.Fatal(err)
	}
	// Com 2 workers, ver a 7a chamada começar garante que as duas estão presas
	// e que os 5 resultados prontos já foram entregues ao coletor.
	esperar(t, "as duas workers presas no modelo", func() bool { return chamadas.Load() > 6 })

	if m.Cancel("run_de_outra") {
		t.Error("cancelar um id que não é o ativo deveria devolver false")
	}
	if !m.Cancel(sum.RunID) {
		t.Fatal("Cancel do id ativo deveria devolver true")
	}
	m.Wait()

	final := store.ultimoResumo()
	if final.Status != domain.StatusCanceled || final.FinishedAt == nil || final.Error != "" {
		t.Errorf("resumo = %+v", final)
	}
	if n := store.qtdResultados(); n != 5 {
		t.Errorf("deveriam sobrar os 5 resultados prontos, sobraram %d", n)
	}
	if final.Counts == nil || final.Counts.Errors != 0 {
		t.Errorf("cancelamento não pode virar erro: %+v", final.Counts)
	}
	if m.Cancel(sum.RunID) {
		t.Error("depois de terminar, Cancel deveria devolver false")
	}
}

func TestRunManagerErroDaFabricaNaoGravaNada(t *testing.T) {
	store := &memStore{}
	m := NewRunManager(store, fonte(txsDeTeste(30)), func(domain.RunParams) (Pipeline, error) {
		return Pipeline{}, errors.New("modelo desconhecido")
	}, nil)
	t.Cleanup(m.Shutdown)

	if _, err := m.Start(paramsDeTeste()); err == nil || !strings.Contains(err.Error(), "modelo desconhecido") {
		t.Fatalf("esperava o erro da fábrica, veio %v", err)
	}
	if len(store.summaries) != 0 || m.ActiveID() != "" {
		t.Errorf("não deveria ter gravado nada nem ficado ocupado: %+v", store.summaries)
	}
}

func TestRunManagerFonteComErroViraFailed(t *testing.T) {
	store := &memStore{}
	falha := errors.New("o CSV sumiu")
	m := NewRunManager(store, func(context.Context, func(domain.Transaction) error) error { return falha },
		func(domain.RunParams) (Pipeline, error) {
			return Pipeline{Features: &fakeFeatures{}, Classifier: &fakeClassifier{}}, nil
		}, nil)
	t.Cleanup(m.Shutdown)

	if _, err := m.Start(paramsDeTeste()); err != nil {
		t.Fatal(err)
	}
	m.Wait()
	sum := store.ultimoResumo()
	if sum.Status != domain.StatusFailed || !strings.Contains(sum.Error, "o CSV sumiu") || sum.FinishedAt == nil {
		t.Errorf("resumo = %+v", sum)
	}
}

func TestRunManagerFalhaQuandoOModeloNaoResponde(t *testing.T) {
	cl := &fakeClassifier{fn: func(context.Context, string) (domain.Decision, error) {
		return domain.Decision{}, errors.New("conexão recusada")
	}}
	m, store := novoGerenciador(t, txsDeTeste(300), cl)
	if _, err := m.Start(paramsDeTeste()); err != nil {
		t.Fatal(err)
	}
	m.Wait()
	sum := store.ultimoResumo()
	if sum.Status != domain.StatusFailed || !strings.Contains(sum.Error, "aquecimento") {
		t.Errorf("resumo = %+v", sum)
	}
}

func TestRunManagerReportaProgresso(t *testing.T) {
	liberar := make(chan struct{})
	cl := &fakeClassifier{fn: func(ctx context.Context, state string) (domain.Decision, error) {
		if seqDoState(state) >= 0 {
			select {
			case <-liberar:
			case <-ctx.Done():
				return domain.Decision{}, ctx.Err()
			}
		}
		return domain.Decision{Noul: 0.1, Attempts: 1, Latency: time.Millisecond}, nil
	}}
	m, _ := novoGerenciador(t, txsDeTeste(300), cl)

	sum, err := m.Start(paramsDeTeste())
	if err != nil {
		t.Fatal(err)
	}
	esperar(t, "total do progresso", func() bool {
		p, ok := m.Progress(sum.RunID)
		return ok && p.Total == 30
	})
	if _, ok := m.Progress("run_de_outra"); ok {
		t.Error("Progress de outro id deveria devolver false")
	}
	close(liberar)
	m.Wait()
	if _, ok := m.Progress(sum.RunID); ok {
		t.Error("depois de terminar, não há progresso ativo")
	}
}

func TestRunManagerShutdownCancelaEEspera(t *testing.T) {
	cl := &fakeClassifier{fn: func(ctx context.Context, state string) (domain.Decision, error) {
		if seqDoState(state) >= 0 {
			<-ctx.Done()
			return domain.Decision{}, ctx.Err()
		}
		return domain.Decision{Noul: 0.1, Attempts: 1, Latency: time.Millisecond}, nil
	}}
	m, store := novoGerenciador(t, txsDeTeste(300), cl)
	if _, err := m.Start(paramsDeTeste()); err != nil {
		t.Fatal(err)
	}
	esperar(t, "a execução começar a chamar o modelo", func() bool { return cl.calls.Load() > 3 })

	acabou := make(chan struct{})
	go func() {
		m.Shutdown()
		close(acabou)
	}()
	select {
	case <-acabou:
	case <-time.After(3 * time.Second):
		t.Fatal("Shutdown não terminou")
	}
	if store.ultimoResumo().Status != domain.StatusCanceled {
		t.Errorf("status = %s", store.ultimoResumo().Status)
	}
}
```

- [ ] **Passo 2: rodar e ver falhar**

Rode: `go test ./internal/service/ -run RunManager`
Esperado: FAIL com `undefined: NewRunManager`.

- [ ] **Passo 3: implementar**

```go file=internal/service/run_manager.go
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"fraud-detector-jev/internal/domain"
)

// warmupCalls é quantas chamadas de aquecimento cada execução faz.
const warmupCalls = 3

// ErrBusy aparece quando já existe uma execução em andamento.
var ErrBusy = errors.New("já existe uma execução em andamento")

// RunStore é o que o gerenciador precisa do armazenamento. O runstore.Store
// implementa.
type RunStore interface {
	NewRunID(now time.Time) string
	SaveSummary(sum domain.RunSummary) error
	OpenResultsWriter(id string) (domain.ResultsSink, error)
}

// RunFactory monta o pipeline de uma execução. É aqui que o cliente do modelo
// nasce com o modelo escolhido nos parâmetros.
type RunFactory func(params domain.RunParams) (Pipeline, error)

// RunManager roda um benchmark por vez em segundo plano.
type RunManager struct {
	store   RunStore
	source  TxSource
	factory RunFactory
	log     *slog.Logger
	now     func() time.Time

	mu     sync.Mutex
	active *activeRun
	wg     sync.WaitGroup
}

type activeRun struct {
	id     string
	cancel context.CancelFunc
	done   atomic.Int64
	total  atomic.Int64
}

// NewRunManager cria o gerenciador. logger nil usa o slog padrão.
func NewRunManager(store RunStore, source TxSource, factory RunFactory, logger *slog.Logger) *RunManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &RunManager{store: store, source: source, factory: factory, log: logger, now: time.Now}
}

// Start começa uma execução e devolve o resumo inicial (status running).
func (m *RunManager) Start(params domain.RunParams) (domain.RunSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != nil {
		return domain.RunSummary{}, ErrBusy
	}

	pipe, err := m.factory(params)
	if err != nil {
		return domain.RunSummary{}, fmt.Errorf("preparando a execução: %w", err)
	}
	sum := domain.RunSummary{
		RunID:     m.store.NewRunID(m.now()),
		Status:    domain.StatusRunning,
		StartedAt: m.now(),
		Params:    params,
	}
	if err := m.store.SaveSummary(sum); err != nil {
		closeIfCloser(pipe.Classifier)
		return domain.RunSummary{}, fmt.Errorf("gravando o resumo inicial: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	run := &activeRun{id: sum.RunID, cancel: cancel}
	m.active = run
	m.wg.Add(1)
	go m.execute(ctx, run, pipe, sum)
	return sum, nil
}

func (m *RunManager) execute(ctx context.Context, run *activeRun, pipe Pipeline, sum domain.RunSummary) {
	defer m.wg.Done()
	defer func() {
		m.mu.Lock()
		m.active = nil
		m.mu.Unlock()
	}()
	defer closeIfCloser(pipe.Classifier)
	defer run.cancel()

	p := sum.Params
	m.log.Info("execução começou", "run_id", sum.RunID, "n", p.N, "modelo", p.Model, "grafo", p.Graph)

	sample, err := Sample(ctx, m.source, SampleParams{N: p.N, FraudPct: p.FraudPct, Seed: p.Seed, Types: p.Types})
	if err != nil {
		m.finish(&sum, err)
		return
	}
	run.total.Store(int64(len(sample.Transactions)))
	sum.Sample = &domain.SampleInfo{
		Requested: p.N, N: len(sample.Transactions), NFraud: sample.NFraud, NLegit: sample.NLegit,
		FraudShortfall: sample.FraudShortfall, EligibleRows: sample.EligibleRows,
		EligibleFraud: sample.EligibleFraud, Prevalence: sample.Prevalence,
	}

	sink, err := m.store.OpenResultsWriter(sum.RunID)
	if err != nil {
		m.finish(&sum, fmt.Errorf("abrindo os resultados: %w", err))
		return
	}
	cfg := PipelineConfig{
		Graph: p.Graph, Workers: p.Workers, ClassifyWorkers: p.MaxInflight,
		WindowHours: p.WindowHours, CycleMaxHops: p.CycleMaxHops, Threshold: p.Threshold, Warmup: warmupCalls,
	}
	out, runErr := pipe.Run(ctx, cfg, sample.Transactions, sink.Write, func(done int) { run.done.Store(int64(done)) })
	if closeErr := sink.Close(); closeErr != nil && runErr == nil {
		runErr = fmt.Errorf("fechando os resultados: %w", closeErr)
	}

	counts := out.Counts
	metrics := domain.ComputeMetrics(counts.Confusion, sample.Prevalence)
	latency := out.Latency
	sum.Counts, sum.Metrics, sum.Latency = &counts, &metrics, &latency
	sum.TPS, sum.TokensTotal = out.TPS, out.TokensTotal
	m.finish(&sum, runErr)
}

// finish define o status final e grava o resumo.
func (m *RunManager) finish(sum *domain.RunSummary, err error) {
	now := m.now()
	sum.FinishedAt = &now
	sum.DurationS = now.Sub(sum.StartedAt).Seconds()
	switch {
	case err == nil:
		sum.Status = domain.StatusDone
	case errors.Is(err, context.Canceled):
		sum.Status = domain.StatusCanceled
	default:
		sum.Status = domain.StatusFailed
		sum.Error = err.Error()
	}
	m.log.Info("execução terminou", "run_id", sum.RunID, "status", sum.Status, "erro", sum.Error)
	if saveErr := m.store.SaveSummary(*sum); saveErr != nil {
		m.log.Error("não consegui gravar o resumo final", "run_id", sum.RunID, "erro", saveErr)
	}
}

// Cancel cancela a execução ativa se o id bater. Devolve false caso contrário.
func (m *RunManager) Cancel(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil || m.active.id != id {
		return false
	}
	m.active.cancel()
	return true
}

// Progress devolve o andamento da execução ativa. Total é 0 enquanto a amostra
// ainda está sendo montada.
func (m *RunManager) Progress(id string) (domain.Progress, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil || m.active.id != id {
		return domain.Progress{}, false
	}
	return domain.Progress{Done: int(m.active.done.Load()), Total: int(m.active.total.Load())}, true
}

// ActiveID devolve o id da execução ativa, ou "" se não houver.
func (m *RunManager) ActiveID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil {
		return ""
	}
	return m.active.id
}

// Wait espera a execução ativa terminar.
func (m *RunManager) Wait() { m.wg.Wait() }

// Shutdown cancela a execução ativa e espera ela terminar de gravar.
func (m *RunManager) Shutdown() {
	m.mu.Lock()
	if m.active != nil {
		m.active.cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
}

// closeIfCloser fecha o classificador se ele souber se fechar (o jev.Client
// solta as conexões ociosas).
func closeIfCloser(c Classifier) {
	if closer, ok := c.(interface{ Close() }); ok {
		closer.Close()
	}
}
```

- [ ] **Passo 4: rodar e ver passar**

Rode: `go test -race -count=3 ./internal/service/`
Esperado: `ok`.

- [ ] **Passo 5: commitar**

```bash
git add internal/service/run_manager.go internal/service/run_manager_test.go
git commit -m "feat: adiciona gerenciador de execuções com cancelamento e resumo final"
```

---

## Tarefa 12: parâmetros do benchmark e caso de uso do simulador

**Arquivos:**
- Criar: `internal/service/params.go`, `internal/service/evaluator.go`
- Testar: `internal/service/params_test.go`, `internal/service/evaluator_test.go`

**Interfaces:**
- Consumes: `service.GraphFeatureSource`, `service.SubgraphSource`, `service.Classifier`, `service.BuildState`, `domain.*`, `fakeFeatures` e `fakeClassifier` da Tarefa 10.
- Produz:
  - Constantes `service.DefaultN`, `DefaultFraudPct`, `DefaultSeed`, `DefaultWorkers`, `DefaultWindowHours`, `DefaultCycleMaxHops`, `DefaultMaxInflight`, `DefaultThreshold` e `service.DefaultTypes`.
  - `service.Limits{MaxSampleSize, MaxWorkers int, DefaultModel, BaseURL string}`
  - `service.RunRequest` (corpo do `POST /benchmarks`, campos opcionais) e `(RunRequest).ToParams(Limits) (domain.RunParams, error)`
  - `*service.ValidationError{Problems []string}` (o handler HTTP transforma em 400)
  - `service.EvaluateRequest`, `service.EvaluateResponse`, `service.Evaluator`, `(*Evaluator).Evaluate(ctx, EvaluateRequest) (EvaluateResponse, error)`
  - `*service.UpstreamError{Source string, Err error}` (o handler transforma em 502)

Regras do `evaluate`: com `seq`, é replay (features só do passado daquela transação, e o `state` leva o id `TX_<seq>`). Sem `seq`, é uma transação hipotética "de agora": considera todas as arestas com `step` menor ou igual ao informado, e o id é `TX_manual`. O `step` é opcional só quando `Evaluator.DefaultStep` existir (o maior step do dataset).

- [ ] **Passo 1: escrever os testes que falham**

```go file=internal/service/params_test.go
package service

import (
	"encoding/json"
	"slices"
	"testing"

	"fraud-detector-jev/internal/domain"
)

var limitesDeTeste = Limits{MaxSampleSize: 500000, MaxWorkers: 256, DefaultModel: "tev1:4b", BaseURL: "http://ollama:11434"}

func TestToParamsAplicaOsPadroes(t *testing.T) {
	p, err := RunRequest{}.ToParams(limitesDeTeste)
	if err != nil {
		t.Fatal(err)
	}
	want := domain.RunParams{
		N: 2000, FraudPct: 10, Seed: 42, Workers: 32, Model: "tev1:4b", BaseURL: "http://ollama:11434", Graph: true,
		Types: []string{"TRANSFER", "CASH_OUT"}, WindowHours: 24, CycleMaxHops: 3, MaxInflight: 4, Threshold: 0.5,
	}
	if p.N != want.N || p.FraudPct != want.FraudPct || p.Seed != want.Seed || p.Workers != want.Workers ||
		p.Model != want.Model || p.BaseURL != want.BaseURL || p.Graph != want.Graph || !slices.Equal(p.Types, want.Types) ||
		p.WindowHours != want.WindowHours || p.CycleMaxHops != want.CycleMaxHops || p.MaxInflight != want.MaxInflight ||
		p.Threshold != want.Threshold {
		t.Errorf("params = %+v\nesperado  %+v", p, want)
	}
}

func TestToParamsAceitaValoresDoPedidoInclusiveZeroEFalse(t *testing.T) {
	var req RunRequest
	corpo := `{"n":500,"fraud_pct":0,"seed":0,"workers":8,"model":"nimble","graph":false,
	           "types":["CASH_OUT","TRANSFER","CASH_OUT"],"window_hours":48,"cycle_max_hops":2,"max_inflight":1,"threshold":0}`
	if err := json.Unmarshal([]byte(corpo), &req); err != nil {
		t.Fatal(err)
	}
	p, err := req.ToParams(limitesDeTeste)
	if err != nil {
		t.Fatal(err)
	}
	if p.N != 500 || p.FraudPct != 0 || p.Seed != 0 || p.Workers != 8 || p.Model != "nimble" || p.Graph ||
		p.WindowHours != 48 || p.CycleMaxHops != 2 || p.MaxInflight != 1 || p.Threshold != 0 {
		t.Errorf("params = %+v", p)
	}
	if !slices.Equal(p.Types, []string{"CASH_OUT", "TRANSFER"}) {
		t.Errorf("types deveria vir sem repetição e na ordem do pedido: %v", p.Types)
	}
}

func TestToParamsRejeitaValoresInvalidosEJuntaOsProblemas(t *testing.T) {
	inteiro := func(n int) *int { return &n }
	decimal := func(f float64) *float64 { return &f }
	vazio := ""

	casos := map[string]RunRequest{
		"n zero":             {N: inteiro(0)},
		"n acima do limite":  {N: inteiro(500001)},
		"fraud_pct negativo": {FraudPct: decimal(-1)},
		"fraud_pct acima":    {FraudPct: decimal(100.5)},
		"workers zero":       {Workers: inteiro(0)},
		"workers acima":      {Workers: inteiro(257)},
		"modelo vazio":       {Model: &vazio},
		"tipo desconhecido":  {Types: []string{"REFUND"}},
		"tipos vazios":       {Types: []string{}},
		"janela zero":        {WindowHours: inteiro(0)},
		"janela acima":       {WindowHours: inteiro(745)},
		"saltos zero":        {CycleMaxHops: inteiro(0)},
		"saltos acima":       {CycleMaxHops: inteiro(6)},
		"inflight zero":      {MaxInflight: inteiro(0)},
		"inflight acima":     {MaxInflight: inteiro(65)},
		"limiar acima":       {Threshold: decimal(1.1)},
		"limiar negativo":    {Threshold: decimal(-0.1)},
	}
	for nome, req := range casos {
		t.Run(nome, func(t *testing.T) {
			_, err := req.ToParams(limitesDeTeste)
			var ve *ValidationError
			if !asValidation(err, &ve) || len(ve.Problems) != 1 {
				t.Fatalf("esperava 1 problema de validação, veio %v", err)
			}
		})
	}

	_, err := RunRequest{N: inteiro(0), Workers: inteiro(0), Types: []string{"X"}}.ToParams(limitesDeTeste)
	var ve *ValidationError
	if !asValidation(err, &ve) || len(ve.Problems) != 3 {
		t.Errorf("esperava 3 problemas juntos, veio %v", err)
	}
}

func asValidation(err error, alvo **ValidationError) bool {
	ve, ok := err.(*ValidationError)
	if ok {
		*alvo = ve
	}
	return ok
}
```

```go file=internal/service/evaluator_test.go
package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"fraud-detector-jev/internal/domain"
)

type fakeSubgraphs struct {
	beforeSeq int64
	atStep    int
	limit     int
	err       error
}

func (f *fakeSubgraphs) Subgraph(_ context.Context, orig, dest string, beforeSeq int64, atStep, limit int) (domain.Subgraph, error) {
	f.beforeSeq, f.atStep, f.limit = beforeSeq, atStep, limit
	if f.err != nil {
		return domain.Subgraph{}, f.err
	}
	return domain.Subgraph{Nodes: []string{orig, dest}, Edges: []domain.SubEdge{}}, nil
}

type montagem struct {
	ev       *Evaluator
	feats    *fakeFeatures
	sub      *fakeSubgraphs
	cl       *fakeClassifier
	consulta domain.FeatureQuery
	modelo   string
}

func novoAvaliador(noul float64) *montagem {
	m := &montagem{sub: &fakeSubgraphs{}}
	m.feats = &fakeFeatures{fn: func(q domain.FeatureQuery) (domain.GraphFeatures, error) {
		m.consulta = q
		return domain.GraphFeatures{DestInDegree: 42, CycleDetected: true}, nil
	}}
	m.cl = &fakeClassifier{fn: func(context.Context, string) (domain.Decision, error) {
		return domain.Decision{
			Noul: noul, InputTokens: 50, Attempts: 1, Latency: 3 * time.Millisecond,
			Raw: json.RawMessage(`{"answers":{"is_fraud":{"type":"noul","noul":0.8}}}`),
		}, nil
	}}
	m.ev = &Evaluator{
		Features: m.feats, Subgraphs: m.sub,
		ClassifierFor: func(model string) (Classifier, error) {
			m.modelo = model
			return m.cl, nil
		},
		DefaultModel: "tev1:4b", WindowHours: 24, CycleMaxHops: 3, DefaultThreshold: 0.5, SubgraphLimit: 50,
	}
	return m
}

func pedidoBase() EvaluateRequest {
	step := 31
	return EvaluateRequest{
		Orig: "C1", Dest: "C2", Type: "TRANSFER", Amount: 350000, Step: &step,
		OrigBalanceBefore: 350000, OrigBalanceAfter: 0,
	}
}

func TestEvaluateReplayUsaOPassadoDaTransacao(t *testing.T) {
	m := novoAvaliador(0.8)
	req := pedidoBase()
	seq := int64(1029)
	req.Seq = &seq

	resp, err := m.ev.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if m.consulta.BeforeSeq != 1029 || m.consulta.AtStep != 31 || m.consulta.Orig != "C1" || m.consulta.Dest != "C2" ||
		m.consulta.WindowHours != 24 || m.consulta.CycleMaxHops != 3 {
		t.Errorf("consulta de features = %+v", m.consulta)
	}
	if m.sub.beforeSeq != 1029 || m.sub.atStep != 31 || m.sub.limit != 50 {
		t.Errorf("consulta do subgrafo = %+v", m.sub)
	}
	if !resp.Replay || !resp.IsFraud || resp.Decision != "fraud" || resp.Noul != 0.8 || resp.Threshold != 0.5 || resp.Model != "tev1:4b" {
		t.Errorf("resposta = %+v", resp)
	}
	if resp.GraphFeatures.DestInDegree != 42 || !resp.GraphFeatures.CycleDetected {
		t.Errorf("features = %+v", resp.GraphFeatures)
	}
	if resp.Neo4jMs < 0 || resp.JevMs != 3 || len(resp.Subgraph.Nodes) != 2 {
		t.Errorf("tempos ou subgrafo errados: %+v", resp)
	}

	var estado map[string]any
	if err := json.Unmarshal(resp.State, &estado); err != nil {
		t.Fatalf("state deveria ser um objeto JSON: %v", err)
	}
	if estado["transaction_id"] != "TX_1029" || estado["time_step"] != float64(31) || estado["graph_features"] == nil {
		t.Errorf("state = %v", estado)
	}
	if !strings.Contains(string(resp.RawResponse), `"noul":0.8`) {
		t.Errorf("resposta bruta = %s", resp.RawResponse)
	}
}

func TestEvaluateSemSeqEhTransacaoDeAgora(t *testing.T) {
	m := novoAvaliador(0.2)
	resp, err := m.ev.Evaluate(context.Background(), pedidoBase())
	if err != nil {
		t.Fatal(err)
	}
	if m.consulta.BeforeSeq != math.MaxInt64 || m.sub.beforeSeq != math.MaxInt64 {
		t.Errorf("sem seq deveria enxergar tudo: features=%d subgrafo=%d", m.consulta.BeforeSeq, m.sub.beforeSeq)
	}
	if resp.Replay || resp.IsFraud || resp.Decision != "legit" {
		t.Errorf("resposta = %+v", resp)
	}
	var estado map[string]any
	_ = json.Unmarshal(resp.State, &estado)
	if estado["transaction_id"] != "TX_manual" {
		t.Errorf("transaction_id = %v", estado["transaction_id"])
	}
}

func TestEvaluateAplicaLimiarEModeloDoPedido(t *testing.T) {
	m := novoAvaliador(0.6)
	req := pedidoBase()
	limiar := 0.7
	req.Threshold = &limiar
	req.Model = "nimble"

	resp, err := m.ev.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.IsFraud || resp.Threshold != 0.7 || resp.Model != "nimble" || m.modelo != "nimble" {
		t.Errorf("resposta = %+v modelo pedido = %q", resp, m.modelo)
	}
}

func TestEvaluateUsaOStepPadraoQuandoOPedidoNaoTraz(t *testing.T) {
	m := novoAvaliador(0.5)
	m.ev.DefaultStep = func() (int, bool) { return 743, true }
	req := pedidoBase()
	req.Step = nil
	if _, err := m.ev.Evaluate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if m.consulta.AtStep != 743 {
		t.Errorf("AtStep = %d", m.consulta.AtStep)
	}

	m.ev.DefaultStep = nil
	if _, err := m.ev.Evaluate(context.Background(), req); err == nil {
		t.Error("sem step e sem padrão deveria dar erro de validação")
	}
}

func TestEvaluateValidaOPedido(t *testing.T) {
	m := novoAvaliador(0.5)
	negativo, limiarAlto := int64(-1), 1.5
	passoNegativo := -3
	casos := map[string]func(*EvaluateRequest){
		"origem vazia":         func(r *EvaluateRequest) { r.Orig = "  " },
		"destino vazio":        func(r *EvaluateRequest) { r.Dest = "" },
		"tipo inválido":        func(r *EvaluateRequest) { r.Type = "REFUND" },
		"valor negativo":       func(r *EvaluateRequest) { r.Amount = -1 },
		"step negativo":        func(r *EvaluateRequest) { r.Step = &passoNegativo },
		"seq negativo":         func(r *EvaluateRequest) { r.Seq = &negativo },
		"limiar fora de 0 a 1": func(r *EvaluateRequest) { r.Threshold = &limiarAlto },
	}
	for nome, muda := range casos {
		t.Run(nome, func(t *testing.T) {
			req := pedidoBase()
			muda(&req)
			_, err := m.ev.Evaluate(context.Background(), req)
			var ve *ValidationError
			if !asValidation(err, &ve) {
				t.Fatalf("esperava ValidationError, veio %v", err)
			}
		})
	}
	if m.feats.calls.Load() != 0 {
		t.Error("pedido inválido não pode chegar no Neo4j")
	}
}

func TestEvaluateMarcaFalhasDeFornecedorComOrigem(t *testing.T) {
	falha := errors.New("caiu")

	m := novoAvaliador(0.5)
	m.feats.fn = func(domain.FeatureQuery) (domain.GraphFeatures, error) { return domain.GraphFeatures{}, falha }
	_, err := m.ev.Evaluate(context.Background(), pedidoBase())
	var up *UpstreamError
	if !errors.As(err, &up) || up.Source != "neo4j" || !errors.Is(err, falha) {
		t.Errorf("features: %v", err)
	}

	m = novoAvaliador(0.5)
	m.cl.fn = func(context.Context, string) (domain.Decision, error) { return domain.Decision{}, falha }
	_, err = m.ev.Evaluate(context.Background(), pedidoBase())
	if !errors.As(err, &up) || up.Source != "modelo" {
		t.Errorf("modelo: %v", err)
	}

	m = novoAvaliador(0.5)
	m.sub.err = falha
	_, err = m.ev.Evaluate(context.Background(), pedidoBase())
	if !errors.As(err, &up) || up.Source != "neo4j" {
		t.Errorf("subgrafo: %v", err)
	}

	m = novoAvaliador(0.5)
	m.ev.ClassifierFor = func(string) (Classifier, error) { return nil, falha }
	_, err = m.ev.Evaluate(context.Background(), pedidoBase())
	if !errors.As(err, &up) || up.Source != "modelo" {
		t.Errorf("fábrica do modelo: %v", err)
	}
}
```

- [ ] **Passo 2: rodar e ver falhar**

Rode: `go test ./internal/service/ -run 'ToParams|Evaluate'`
Esperado: FAIL com `undefined: RunRequest` e `undefined: Evaluator`.

- [ ] **Passo 3: implementar os parâmetros**

```go file=internal/service/params.go
package service

import (
	"fmt"
	"strings"

	"fraud-detector-jev/internal/domain"
)

// Padrões do POST /benchmarks (os mesmos da spec).
const (
	DefaultN            = 2000
	DefaultFraudPct     = 10.0
	DefaultSeed         = uint64(42)
	DefaultWorkers      = 32
	DefaultWindowHours  = 24
	DefaultCycleMaxHops = 3
	DefaultMaxInflight  = 4
	DefaultThreshold    = 0.5
)

// DefaultTypes são os tipos onde existe fraude no PaySim.
var DefaultTypes = []string{"TRANSFER", "CASH_OUT"}

// Limits são os limites e valores que vêm da configuração do servidor.
type Limits struct {
	MaxSampleSize int
	MaxWorkers    int
	DefaultModel  string
	BaseURL       string
}

// RunRequest é o corpo do POST /benchmarks. Os campos são ponteiros para dar
// para diferenciar "não veio" de "veio zero ou false".
type RunRequest struct {
	N            *int     `json:"n"`
	FraudPct     *float64 `json:"fraud_pct"`
	Seed         *uint64  `json:"seed"`
	Workers      *int     `json:"workers"`
	Model        *string  `json:"model"`
	Graph        *bool    `json:"graph"`
	Types        []string `json:"types"`
	WindowHours  *int     `json:"window_hours"`
	CycleMaxHops *int     `json:"cycle_max_hops"`
	MaxInflight  *int     `json:"max_inflight"`
	Threshold    *float64 `json:"threshold"`
}

// ValidationError junta todos os problemas de um pedido, para o usuário
// corrigir tudo de uma vez.
type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string {
	return "pedido inválido: " + strings.Join(e.Problems, ", ")
}

func valueOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

// ToParams aplica os padrões e valida. O BaseURL vem sempre do servidor.
func (r RunRequest) ToParams(l Limits) (domain.RunParams, error) {
	p := domain.RunParams{
		N:            valueOr(r.N, DefaultN),
		FraudPct:     valueOr(r.FraudPct, DefaultFraudPct),
		Seed:         valueOr(r.Seed, DefaultSeed),
		Workers:      valueOr(r.Workers, DefaultWorkers),
		Model:        strings.TrimSpace(valueOr(r.Model, l.DefaultModel)),
		BaseURL:      l.BaseURL,
		Graph:        valueOr(r.Graph, true),
		WindowHours:  valueOr(r.WindowHours, DefaultWindowHours),
		CycleMaxHops: valueOr(r.CycleMaxHops, DefaultCycleMaxHops),
		MaxInflight:  valueOr(r.MaxInflight, DefaultMaxInflight),
		Threshold:    valueOr(r.Threshold, DefaultThreshold),
	}

	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	if p.N < 1 || p.N > l.MaxSampleSize {
		add("n precisa estar entre 1 e %d, veio %d", l.MaxSampleSize, p.N)
	}
	if p.FraudPct < 0 || p.FraudPct > 100 {
		add("fraud_pct precisa estar entre 0 e 100, veio %v", p.FraudPct)
	}
	if p.Workers < 1 || p.Workers > l.MaxWorkers {
		add("workers precisa estar entre 1 e %d, veio %d", l.MaxWorkers, p.Workers)
	}
	if p.Model == "" {
		add("model não pode ser vazio")
	}
	if p.WindowHours < 1 || p.WindowHours > 744 {
		add("window_hours precisa estar entre 1 e 744, veio %d", p.WindowHours)
	}
	if p.CycleMaxHops < 1 || p.CycleMaxHops > 5 {
		add("cycle_max_hops precisa estar entre 1 e 5, veio %d", p.CycleMaxHops)
	}
	if p.MaxInflight < 1 || p.MaxInflight > 64 {
		add("max_inflight precisa estar entre 1 e 64, veio %d", p.MaxInflight)
	}
	if p.Threshold < 0 || p.Threshold > 1 {
		add("threshold precisa estar entre 0 e 1, veio %v", p.Threshold)
	}

	types := r.Types
	if types == nil {
		types = DefaultTypes
	}
	if len(types) == 0 {
		add("types não pode ser vazio")
	}
	for _, t := range types {
		switch {
		case !domain.ValidType(t):
			add("tipo desconhecido %q (use PAYMENT, TRANSFER, CASH_OUT, DEBIT ou CASH_IN)", t)
		case !contains(p.Types, t):
			p.Types = append(p.Types, t)
		}
	}

	if len(problems) > 0 {
		return domain.RunParams{}, &ValidationError{Problems: problems}
	}
	return p, nil
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
```

- [ ] **Passo 4: implementar o simulador (evaluate)**

```go file=internal/service/evaluator.go
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"fraud-detector-jev/internal/domain"
)

// EvaluateRequest é o corpo do POST /transactions/evaluate. Seq presente
// significa replay de uma transação real. Step ausente usa DefaultStep.
type EvaluateRequest struct {
	Orig              string   `json:"orig"`
	Dest              string   `json:"dest"`
	Type              string   `json:"type"`
	Amount            float64  `json:"amount"`
	Step              *int     `json:"step"`
	OrigBalanceBefore float64  `json:"orig_balance_before"`
	OrigBalanceAfter  float64  `json:"orig_balance_after"`
	DestBalanceBefore float64  `json:"dest_balance_before"`
	DestBalanceAfter  float64  `json:"dest_balance_after"`
	Seq               *int64   `json:"seq"`
	Model             string   `json:"model"`
	Threshold         *float64 `json:"threshold"`
}

// EvaluateResponse é o que o simulador da Aba 3 mostra.
type EvaluateResponse struct {
	Decision      string               `json:"decision"`
	IsFraud       bool                 `json:"is_fraud"`
	Noul          float64              `json:"noul"`
	Threshold     float64              `json:"threshold"`
	Model         string               `json:"model"`
	Replay        bool                 `json:"replay"`
	GraphFeatures domain.GraphFeatures `json:"graph_features"`
	State         json.RawMessage      `json:"state"`
	RawResponse   json.RawMessage      `json:"raw_response"`
	Neo4jMs       float64              `json:"neo4j_ms"`
	JevMs         float64              `json:"jev_ms"`
	Subgraph      domain.Subgraph      `json:"subgraph"`
}

// UpstreamError marca falha de um fornecedor (Neo4j ou modelo). O handler HTTP
// responde 502 nesse caso.
type UpstreamError struct {
	Source string
	Err    error
}

func (e *UpstreamError) Error() string { return e.Source + ": " + e.Err.Error() }
func (e *UpstreamError) Unwrap() error { return e.Err }

// Evaluator é o caso de uso do simulador.
type Evaluator struct {
	Features         GraphFeatureSource
	Subgraphs        SubgraphSource
	ClassifierFor    func(model string) (Classifier, error)
	DefaultModel     string
	DefaultStep      func() (step int, ok bool)
	WindowHours      int
	CycleMaxHops     int
	DefaultThreshold float64
	SubgraphLimit    int
}

func (e *Evaluator) validate(req EvaluateRequest) (step int, threshold float64, err error) {
	var problems []string
	if strings.TrimSpace(req.Orig) == "" {
		problems = append(problems, "orig não pode ser vazio")
	}
	if strings.TrimSpace(req.Dest) == "" {
		problems = append(problems, "dest não pode ser vazio")
	}
	if !domain.ValidType(req.Type) {
		problems = append(problems, fmt.Sprintf("type desconhecido %q", req.Type))
	}
	if req.Amount < 0 {
		problems = append(problems, "amount não pode ser negativo")
	}
	if req.Seq != nil && *req.Seq < 0 {
		problems = append(problems, "seq não pode ser negativo")
	}

	switch {
	case req.Step != nil:
		step = *req.Step
	case e.DefaultStep != nil:
		if s, ok := e.DefaultStep(); ok {
			step = s
			break
		}
		problems = append(problems, "step é obrigatório (o dataset ainda não foi carregado)")
	default:
		problems = append(problems, "step é obrigatório")
	}
	if req.Step != nil && step < 0 {
		problems = append(problems, "step não pode ser negativo")
	}

	threshold = e.DefaultThreshold
	if req.Threshold != nil {
		threshold = *req.Threshold
		if threshold < 0 || threshold > 1 {
			problems = append(problems, "threshold precisa estar entre 0 e 1")
		}
	}
	if len(problems) > 0 {
		return 0, 0, &ValidationError{Problems: problems}
	}
	return step, threshold, nil
}

// Evaluate calcula as features, chama o modelo e monta o subgrafo.
func (e *Evaluator) Evaluate(ctx context.Context, req EvaluateRequest) (EvaluateResponse, error) {
	step, threshold, err := e.validate(req)
	if err != nil {
		return EvaluateResponse{}, err
	}

	replay := req.Seq != nil
	beforeSeq := int64(math.MaxInt64)
	tx := domain.Transaction{
		Step: step, Type: req.Type, Amount: req.Amount,
		Orig: req.Orig, OldBalanceOrig: req.OrigBalanceBefore, NewBalanceOrig: req.OrigBalanceAfter,
		Dest: req.Dest, OldBalanceDest: req.DestBalanceBefore, NewBalanceDest: req.DestBalanceAfter,
	}
	if replay {
		beforeSeq = *req.Seq
		tx.Seq = *req.Seq
	}

	t0 := time.Now()
	gf, err := e.Features.Features(ctx, domain.FeatureQuery{
		Orig: req.Orig, Dest: req.Dest, BeforeSeq: beforeSeq, AtStep: step,
		WindowHours: e.WindowHours, CycleMaxHops: e.CycleMaxHops,
	})
	neo4jMs := millis(time.Since(t0))
	if err != nil {
		return EvaluateResponse{}, &UpstreamError{Source: "neo4j", Err: err}
	}

	state := BuildState(tx, &gf)
	if !replay {
		state.TransactionID = "TX_manual"
	}
	stateJSON, err := state.JSON()
	if err != nil {
		return EvaluateResponse{}, fmt.Errorf("montando o state: %w", err)
	}

	model := req.Model
	if model == "" {
		model = e.DefaultModel
	}
	classifier, err := e.ClassifierFor(model)
	if err != nil {
		return EvaluateResponse{}, &UpstreamError{Source: "modelo", Err: err}
	}
	defer closeIfCloser(classifier)
	dec, err := classifier.Classify(ctx, stateJSON)
	if err != nil {
		return EvaluateResponse{}, &UpstreamError{Source: "modelo", Err: err}
	}

	sub, err := e.Subgraphs.Subgraph(ctx, req.Orig, req.Dest, beforeSeq, step, e.SubgraphLimit)
	if err != nil {
		return EvaluateResponse{}, &UpstreamError{Source: "neo4j", Err: err}
	}

	isFraud := domain.Decide(dec.Noul, threshold)
	decision := "legit"
	if isFraud {
		decision = "fraud"
	}
	return EvaluateResponse{
		Decision: decision, IsFraud: isFraud, Noul: dec.Noul, Threshold: threshold, Model: model, Replay: replay,
		GraphFeatures: gf, State: json.RawMessage(stateJSON), RawResponse: dec.Raw,
		Neo4jMs: neo4jMs, JevMs: millis(dec.Latency), Subgraph: sub,
	}, nil
}
```

- [ ] **Passo 5: rodar e ver passar**

Rode: `go test -race -count=3 ./internal/service/`
Esperado: `ok`.

- [ ] **Passo 6: commitar**

```bash
git add internal/service/params.go internal/service/params_test.go internal/service/evaluator.go internal/service/evaluator_test.go
git commit -m "feat: adiciona validação dos parâmetros do benchmark e caso de uso do simulador"
```

---
## Tarefa 13: API HTTP

**Arquivos:**
- Criar: `internal/infrastructure/web/router.go`, `handlers_meta.go`, `handlers_bench.go`, `handlers_eval.go`
- Testar: `internal/infrastructure/web/router_test.go`

**Interfaces:**
- Consumes: `service.RunRequest`, `service.Limits`, `service.ValidationError`, `service.UpstreamError`, `service.ErrBusy`, `service.EvaluateRequest/Response`, `domain.*`, `jev.DecisionModels`.
- Produz:
  - `web.RunController` (interface): `Start`, `Cancel`, `Progress`, `ActiveID`. O `*service.RunManager` implementa.
  - `web.RunReader` (interface): `ReadSummary`, `List`, `OpenResults`, `ReadDatasetStats`. O `*runstore.Store` implementa.
  - `web.TransactionEvaluator` (interface): `Evaluate(ctx, service.EvaluateRequest) (service.EvaluateResponse, error)`. O `*service.Evaluator` implementa.
  - `web.Server{Runs, Store, Evaluator, Limits, PingNeo4j, ListModels, Log}` e `(*Server).Handler() http.Handler`.

Contrato REST (é o que o dashboard do plano 2 consome):

| Rota | Sucesso | Erros |
| --- | --- | --- |
| `GET /healthz` | 200 `{"status":"ok"\|"degraded","neo4j":{"ok":true},"ollama":{"ok":true}}` | 503 `status:"unavailable"` se o Neo4j caiu. Ollama fora dá 200 `degraded` com `hint` |
| `GET /info` | 200 com `model`, `base_url`, `ollama_ok`, `available_models`, `dataset` (ou `null` com `dataset_hint`), `defaults`, `limits`, `active_run` | |
| `POST /benchmarks` | 202 com o resumo inicial (`status:"running"`) | 400 `validation_error` ou `invalid_json`, 409 `run_in_progress` |
| `GET /benchmarks` | 200 `{"runs":[...]}`, da mais nova para a mais antiga | |
| `GET /benchmarks/{id}` | 200 com o resumo, e `progress` se estiver ativa | 404 `not_found` |
| `DELETE /benchmarks/{id}` | 202 `{"canceled":true,"run_id":"..."}` | 404, 409 `not_running` |
| `GET /benchmarks/{id}/results` | 200 `application/x-ndjson`, em streaming | 404, 409 `run_in_progress` |
| `POST /transactions/evaluate` | 200 com o `EvaluateResponse` | 400 `validation_error` ou `invalid_json`, 502 `upstream_error` |

Erro sempre no formato `{"error":{"code":"...","message":"...","details":["..."]}}`. Qualquer rota desconhecida (ou método errado) devolve 404 `not_found` em JSON.

- [ ] **Passo 1: escrever os testes que falham**

```go file=internal/infrastructure/web/router_test.go
package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"fraud-detector-jev/internal/domain"
	"fraud-detector-jev/internal/service"
)

const idExemplo = "run_20260930_120000_abcd"

type fakeRuns struct {
	mu         sync.Mutex
	started    []domain.RunParams
	startErr   error
	cancelavel string
	active     string
	progress   domain.Progress
}

func (f *fakeRuns) Start(p domain.RunParams) (domain.RunSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = append(f.started, p)
	if f.startErr != nil {
		return domain.RunSummary{}, f.startErr
	}
	return domain.RunSummary{RunID: idExemplo, Status: domain.StatusRunning, Params: p}, nil
}
func (f *fakeRuns) Cancel(id string) bool { return id == f.cancelavel }
func (f *fakeRuns) Progress(id string) (domain.Progress, bool) {
	if id == f.active {
		return f.progress, true
	}
	return domain.Progress{}, false
}
func (f *fakeRuns) ActiveID() string { return f.active }

type fakeStore struct {
	summaries   map[string]domain.RunSummary
	results     map[string]string
	stats       *domain.DatasetStats
	panicOnList bool
}

func (f *fakeStore) ReadSummary(id string) (domain.RunSummary, error) {
	if s, ok := f.summaries[id]; ok {
		return s, nil
	}
	return domain.RunSummary{}, fmt.Errorf("%s: %w", id, domain.ErrNotFound)
}
func (f *fakeStore) List() ([]domain.RunSummary, error) {
	if f.panicOnList {
		panic("bug de propósito")
	}
	var out []domain.RunSummary
	for _, s := range f.summaries {
		out = append(out, s)
	}
	return out, nil
}
func (f *fakeStore) OpenResults(id string) (io.ReadCloser, error) {
	if r, ok := f.results[id]; ok {
		return io.NopCloser(strings.NewReader(r)), nil
	}
	return nil, fmt.Errorf("%s: %w", id, domain.ErrNotFound)
}
func (f *fakeStore) ReadDatasetStats() (domain.DatasetStats, error) {
	if f.stats == nil {
		return domain.DatasetStats{}, domain.ErrNotFound
	}
	return *f.stats, nil
}

type fakeEval struct {
	last service.EvaluateRequest
	resp service.EvaluateResponse
	err  error
}

func (f *fakeEval) Evaluate(_ context.Context, req service.EvaluateRequest) (service.EvaluateResponse, error) {
	f.last = req
	return f.resp, f.err
}

type ambiente struct {
	srv   *Server
	runs  *fakeRuns
	store *fakeStore
	eval  *fakeEval
}

func novoAmbiente() *ambiente {
	a := &ambiente{runs: &fakeRuns{}, eval: &fakeEval{}}
	a.store = &fakeStore{summaries: map[string]domain.RunSummary{}, results: map[string]string{}}
	a.srv = &Server{
		Runs: a.runs, Store: a.store, Evaluator: a.eval,
		Limits:     service.Limits{MaxSampleSize: 500000, MaxWorkers: 256, DefaultModel: "tev1:4b", BaseURL: "http://ollama:11434"},
		PingNeo4j:  func(context.Context) error { return nil },
		ListModels: func(context.Context) ([]string, error) { return []string{"tev1:4b", "llama3:8b", "nimble:latest"}, nil },
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return a
}

func (a *ambiente) chamar(metodo, caminho, corpo string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(metodo, caminho, strings.NewReader(corpo))
	rec := httptest.NewRecorder()
	a.srv.Handler().ServeHTTP(rec, req)
	return rec
}

func jsonDe(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("a resposta não é JSON (%s): %v", rec.Body.String(), err)
	}
	return m
}

func codigoDoErro(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	erro, ok := jsonDe(t, rec)["error"].(map[string]any)
	if !ok {
		t.Fatalf("resposta sem o campo error: %s", rec.Body.String())
	}
	return erro["code"].(string)
}

func TestHealthz(t *testing.T) {
	a := novoAmbiente()
	rec := a.chamar("GET", "/healthz", "")
	if rec.Code != 200 || jsonDe(t, rec)["status"] != "ok" {
		t.Errorf("tudo ok: %d %s", rec.Code, rec.Body)
	}

	a.srv.ListModels = func(context.Context) ([]string, error) { return nil, errors.New("conexão recusada") }
	rec = a.chamar("GET", "/healthz", "")
	corpo := jsonDe(t, rec)
	ollama := corpo["ollama"].(map[string]any)
	if rec.Code != 200 || corpo["status"] != "degraded" || ollama["ok"] != false || !strings.Contains(ollama["hint"].(string), "OLLAMA_HOST") {
		t.Errorf("Ollama fora deveria dar 200 degraded com a dica: %d %s", rec.Code, rec.Body)
	}

	a.srv.PingNeo4j = func(context.Context) error { return errors.New("neo4j fora") }
	rec = a.chamar("GET", "/healthz", "")
	if rec.Code != 503 || jsonDe(t, rec)["status"] != "unavailable" {
		t.Errorf("Neo4j fora deveria dar 503: %d %s", rec.Code, rec.Body)
	}
}

func TestInfo(t *testing.T) {
	a := novoAmbiente()
	a.store.stats = &domain.DatasetStats{TotalRows: 100, FraudRows: 2, MaxStep: 743, ByType: map[string]domain.TypeStats{"TRANSFER": {Rows: 60, Frauds: 2}}}
	a.runs.active = idExemplo

	corpo := jsonDe(t, a.chamar("GET", "/info", ""))
	if corpo["model"] != "tev1:4b" || corpo["base_url"] != "http://ollama:11434" || corpo["ollama_ok"] != true || corpo["active_run"] != idExemplo {
		t.Errorf("info = %v", corpo)
	}
	modelos := corpo["available_models"].([]any)
	if len(modelos) != 2 || modelos[0] != "tev1:4b" || modelos[1] != "nimble:latest" {
		t.Errorf("só modelos de decisão deveriam aparecer: %v", modelos)
	}
	dataset := corpo["dataset"].(map[string]any)
	if dataset["total_rows"] != float64(100) || dataset["max_step"] != float64(743) {
		t.Errorf("dataset = %v", dataset)
	}
	padroes := corpo["defaults"].(map[string]any)
	if padroes["n"] != float64(2000) || padroes["fraud_pct"] != float64(10) || padroes["workers"] != float64(32) ||
		padroes["graph"] != true || padroes["threshold"] != 0.5 || padroes["window_hours"] != float64(24) ||
		padroes["cycle_max_hops"] != float64(3) || padroes["max_inflight"] != float64(4) || padroes["seed"] != float64(42) {
		t.Errorf("padrões = %v", padroes)
	}
	if tipos := padroes["types"].([]any); len(tipos) != 2 || tipos[0] != "TRANSFER" {
		t.Errorf("tipos = %v", tipos)
	}
	limites := corpo["limits"].(map[string]any)
	if limites["max_sample_size"] != float64(500000) || limites["max_workers"] != float64(256) {
		t.Errorf("limites = %v", limites)
	}
}

func TestInfoSemDatasetEComOllamaFora(t *testing.T) {
	a := novoAmbiente()
	a.srv.ListModels = func(context.Context) ([]string, error) { return nil, errors.New("fora") }
	corpo := jsonDe(t, a.chamar("GET", "/info", ""))
	if corpo["dataset"] != nil || corpo["dataset_hint"] == "" || corpo["ollama_ok"] != false || corpo["active_run"] != nil {
		t.Errorf("info = %v", corpo)
	}
	if modelos, ok := corpo["available_models"].([]any); !ok || len(modelos) != 0 {
		t.Errorf("available_models deveria ser lista vazia: %v", corpo["available_models"])
	}
}

func TestIniciarBenchmarkComCorpoVazioUsaPadroes(t *testing.T) {
	a := novoAmbiente()
	rec := a.chamar("POST", "/benchmarks", "")
	if rec.Code != 202 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if len(a.runs.started) != 1 {
		t.Fatalf("Start chamado %d vezes", len(a.runs.started))
	}
	p := a.runs.started[0]
	if p.N != 2000 || p.Model != "tev1:4b" || p.BaseURL != "http://ollama:11434" || !p.Graph {
		t.Errorf("params = %+v", p)
	}
	corpo := jsonDe(t, rec)
	if corpo["run_id"] != idExemplo || corpo["status"] != "running" {
		t.Errorf("resposta = %v", corpo)
	}
}

func TestIniciarBenchmarkUsaOsValoresDoPedido(t *testing.T) {
	a := novoAmbiente()
	rec := a.chamar("POST", "/benchmarks", `{"n":100,"graph":false,"model":"nimble","fraud_pct":5,"types":["CASH_OUT"]}`)
	if rec.Code != 202 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	p := a.runs.started[0]
	if p.N != 100 || p.Graph || p.Model != "nimble" || p.FraudPct != 5 || len(p.Types) != 1 || p.Types[0] != "CASH_OUT" {
		t.Errorf("params = %+v", p)
	}
}

func TestIniciarBenchmarkRejeitaPedidosRuins(t *testing.T) {
	a := novoAmbiente()

	rec := a.chamar("POST", "/benchmarks", `{"n":0,"workers":9999}`)
	if rec.Code != 400 || codigoDoErro(t, rec) != "validation_error" {
		t.Errorf("validação: %d %s", rec.Code, rec.Body)
	}
	detalhes := jsonDe(t, rec)["error"].(map[string]any)["details"].([]any)
	if len(detalhes) != 2 {
		t.Errorf("deveria listar os 2 problemas: %v", detalhes)
	}

	for nome, corpo := range map[string]string{
		"JSON quebrado":      `{"n":`,
		"campo desconhecido": `{"quantidade":10}`,
		"tipo errado":        `{"n":"muitos"}`,
		"conteúdo extra":     `{"n":10} {"n":20}`,
	} {
		rec := a.chamar("POST", "/benchmarks", corpo)
		if rec.Code != 400 || codigoDoErro(t, rec) != "invalid_json" {
			t.Errorf("%s: %d %s", nome, rec.Code, rec.Body)
		}
	}
	if len(a.runs.started) != 0 {
		t.Error("pedido inválido não pode iniciar execução")
	}
}

func TestIniciarBenchmarkOcupadoDa409(t *testing.T) {
	a := novoAmbiente()
	a.runs.startErr = service.ErrBusy
	a.runs.active = idExemplo
	rec := a.chamar("POST", "/benchmarks", "{}")
	if rec.Code != 409 || codigoDoErro(t, rec) != "run_in_progress" {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

func TestIniciarBenchmarkComFalhaInternaDa500(t *testing.T) {
	a := novoAmbiente()
	a.runs.startErr = errors.New("preparando a execução: modelo inválido")
	rec := a.chamar("POST", "/benchmarks", "{}")
	if rec.Code != 500 || codigoDoErro(t, rec) != "start_failed" {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

func TestListarEObterBenchmarks(t *testing.T) {
	a := novoAmbiente()
	a.store.summaries[idExemplo] = domain.RunSummary{RunID: idExemplo, Status: domain.StatusRunning}
	a.store.summaries["run_20260930_110000_beef"] = domain.RunSummary{RunID: "run_20260930_110000_beef", Status: domain.StatusDone}
	a.runs.active = idExemplo
	a.runs.progress = domain.Progress{Done: 10, Total: 40}

	corpo := jsonDe(t, a.chamar("GET", "/benchmarks", ""))
	runs := corpo["runs"].([]any)
	if len(runs) != 2 {
		t.Fatalf("runs = %v", runs)
	}
	comProgresso := 0
	for _, r := range runs {
		run := r.(map[string]any)
		if run["run_id"] == idExemplo {
			p := run["progress"].(map[string]any)
			if p["done"] != float64(10) || p["total"] != float64(40) {
				t.Errorf("progresso = %v", p)
			}
			comProgresso++
		} else if run["progress"] != nil {
			t.Errorf("execução terminada não tem progresso: %v", run)
		}
	}
	if comProgresso != 1 {
		t.Error("a execução ativa deveria trazer o progresso")
	}

	unico := jsonDe(t, a.chamar("GET", "/benchmarks/"+idExemplo, ""))
	if unico["run_id"] != idExemplo || unico["progress"] == nil {
		t.Errorf("get = %v", unico)
	}
}

func TestListarSemExecucoesDevolveListaVazia(t *testing.T) {
	a := novoAmbiente()
	corpo := jsonDe(t, a.chamar("GET", "/benchmarks", ""))
	if runs, ok := corpo["runs"].([]any); !ok || len(runs) != 0 {
		t.Errorf("deveria ser lista vazia e não null: %v", corpo)
	}
}

func TestObterBenchmarkInexistenteOuComIdMalicioso(t *testing.T) {
	a := novoAmbiente()
	for _, id := range []string{idExemplo, "run_x", "..%2F..%2Fetc%2Fpasswd"} {
		rec := a.chamar("GET", "/benchmarks/"+id, "")
		if rec.Code != 404 || codigoDoErro(t, rec) != "not_found" {
			t.Errorf("%s: %d %s", id, rec.Code, rec.Body)
		}
	}
}

func TestCancelarBenchmark(t *testing.T) {
	a := novoAmbiente()
	a.store.summaries[idExemplo] = domain.RunSummary{RunID: idExemplo, Status: domain.StatusRunning}
	a.store.summaries["run_20260930_110000_beef"] = domain.RunSummary{RunID: "run_20260930_110000_beef", Status: domain.StatusDone}
	a.runs.cancelavel = idExemplo

	rec := a.chamar("DELETE", "/benchmarks/"+idExemplo, "")
	if rec.Code != 202 || jsonDe(t, rec)["canceled"] != true {
		t.Errorf("cancelar ativa: %d %s", rec.Code, rec.Body)
	}
	rec = a.chamar("DELETE", "/benchmarks/run_20260930_110000_beef", "")
	if rec.Code != 409 || codigoDoErro(t, rec) != "not_running" {
		t.Errorf("cancelar terminada: %d %s", rec.Code, rec.Body)
	}
	rec = a.chamar("DELETE", "/benchmarks/run_20260930_100000_cafe", "")
	if rec.Code != 404 {
		t.Errorf("cancelar inexistente: %d %s", rec.Code, rec.Body)
	}
}

func TestResultadosEmStreaming(t *testing.T) {
	a := novoAmbiente()
	a.store.summaries[idExemplo] = domain.RunSummary{RunID: idExemplo, Status: domain.StatusDone}
	a.store.results[idExemplo] = "{\"seq\":0}\n{\"seq\":1}\n"
	a.store.summaries["run_20260930_110000_beef"] = domain.RunSummary{RunID: "run_20260930_110000_beef", Status: domain.StatusRunning}
	a.store.summaries["run_20260930_100000_cafe"] = domain.RunSummary{RunID: "run_20260930_100000_cafe", Status: domain.StatusFailed}

	rec := a.chamar("GET", "/benchmarks/"+idExemplo+"/results", "")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/x-ndjson" || rec.Body.String() != "{\"seq\":0}\n{\"seq\":1}\n" {
		t.Errorf("results: %d %q %q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	rec = a.chamar("GET", "/benchmarks/run_20260930_110000_beef/results", "")
	if rec.Code != 409 || codigoDoErro(t, rec) != "run_in_progress" {
		t.Errorf("em andamento: %d %s", rec.Code, rec.Body)
	}
	rec = a.chamar("GET", "/benchmarks/run_20260930_100000_cafe/results", "")
	if rec.Code != 404 {
		t.Errorf("falhou antes de gravar resultados: %d %s", rec.Code, rec.Body)
	}
	rec = a.chamar("GET", "/benchmarks/run_20260930_090000_0000/results", "")
	if rec.Code != 404 {
		t.Errorf("inexistente: %d %s", rec.Code, rec.Body)
	}
}

func TestEvaluate(t *testing.T) {
	a := novoAmbiente()
	a.eval.resp = service.EvaluateResponse{Decision: "fraud", IsFraud: true, Noul: 0.97, Model: "tev1:4b", State: json.RawMessage(`{"transaction_id":"TX_manual"}`)}

	rec := a.chamar("POST", "/transactions/evaluate", `{"orig":"C1","dest":"C2","type":"TRANSFER","amount":350000,"step":31,"seq":1029}`)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	corpo := jsonDe(t, rec)
	if corpo["decision"] != "fraud" || corpo["noul"] != 0.97 || corpo["state"].(map[string]any)["transaction_id"] != "TX_manual" {
		t.Errorf("resposta = %v", corpo)
	}
	if a.eval.last.Orig != "C1" || a.eval.last.Amount != 350000 || a.eval.last.Seq == nil || *a.eval.last.Seq != 1029 || *a.eval.last.Step != 31 {
		t.Errorf("pedido repassado = %+v", a.eval.last)
	}
}

func TestEvaluateErros(t *testing.T) {
	a := novoAmbiente()

	a.eval.err = &service.ValidationError{Problems: []string{"orig não pode ser vazio"}}
	rec := a.chamar("POST", "/transactions/evaluate", `{}`)
	if rec.Code != 400 || codigoDoErro(t, rec) != "validation_error" {
		t.Errorf("validação: %d %s", rec.Code, rec.Body)
	}

	a.eval.err = &service.UpstreamError{Source: "modelo", Err: errors.New("conexão recusada")}
	rec = a.chamar("POST", "/transactions/evaluate", `{"orig":"a"}`)
	if rec.Code != 502 || codigoDoErro(t, rec) != "upstream_error" || !strings.Contains(rec.Body.String(), "modelo") {
		t.Errorf("fornecedor: %d %s", rec.Code, rec.Body)
	}

	a.eval.err = errors.New("bug")
	rec = a.chamar("POST", "/transactions/evaluate", `{"orig":"a"}`)
	if rec.Code != 500 || codigoDoErro(t, rec) != "internal_error" {
		t.Errorf("interno: %d %s", rec.Code, rec.Body)
	}

	for _, corpo := range []string{"", "{", `{"campo_novo":1}`} {
		rec = a.chamar("POST", "/transactions/evaluate", corpo)
		if rec.Code != 400 || codigoDoErro(t, rec) != "invalid_json" {
			t.Errorf("corpo %q: %d %s", corpo, rec.Code, rec.Body)
		}
	}
}

func TestRotaDesconhecidaEMetodoErradoDao404EmJSON(t *testing.T) {
	a := novoAmbiente()
	for _, c := range []struct{ metodo, caminho string }{
		{"GET", "/nao-existe"},
		{"PUT", "/benchmarks"},
		{"POST", "/healthz"},
	} {
		rec := a.chamar(c.metodo, c.caminho, "")
		if rec.Code != 404 || codigoDoErro(t, rec) != "not_found" {
			t.Errorf("%s %s: %d %s", c.metodo, c.caminho, rec.Code, rec.Body)
		}
	}
}

func TestPanicNoHandlerViraErro500EmJSON(t *testing.T) {
	a := novoAmbiente()
	a.store.panicOnList = true
	rec := a.chamar("GET", "/benchmarks", "")
	if rec.Code != 500 || codigoDoErro(t, rec) != "internal_error" {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	// O servidor continua de pé depois do panic.
	if rec := a.chamar("GET", "/healthz", ""); rec.Code != 200 {
		t.Errorf("healthz depois do panic: %d", rec.Code)
	}
}
```

- [ ] **Passo 2: rodar e ver falhar**

Rode: `go test ./internal/infrastructure/web/`
Esperado: FAIL com `undefined: Server`.

- [ ] **Passo 3: implementar o roteador e os helpers**

```go file=internal/infrastructure/web/router.go
package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"fraud-detector-jev/internal/domain"
	"fraud-detector-jev/internal/service"
)

// RunController é o que o servidor precisa do gerenciador de execuções.
type RunController interface {
	Start(params domain.RunParams) (domain.RunSummary, error)
	Cancel(id string) bool
	Progress(id string) (domain.Progress, bool)
	ActiveID() string
}

// RunReader é a leitura das execuções gravadas em disco.
type RunReader interface {
	ReadSummary(id string) (domain.RunSummary, error)
	List() ([]domain.RunSummary, error)
	OpenResults(id string) (io.ReadCloser, error)
	ReadDatasetStats() (domain.DatasetStats, error)
}

// TransactionEvaluator é o caso de uso do simulador.
type TransactionEvaluator interface {
	Evaluate(ctx context.Context, req service.EvaluateRequest) (service.EvaluateResponse, error)
}

// Server junta tudo que os handlers precisam. PingNeo4j e ListModels são
// funções para o teste não precisar de Neo4j nem de Ollama.
type Server struct {
	Runs       RunController
	Store      RunReader
	Evaluator  TransactionEvaluator
	Limits     service.Limits
	PingNeo4j  func(ctx context.Context) error
	ListModels func(ctx context.Context) ([]string, error)
	Log        *slog.Logger
}

// Handler devolve o http.Handler com todas as rotas e o middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /info", s.handleInfo)
	mux.HandleFunc("POST /benchmarks", s.handleStartBenchmark)
	mux.HandleFunc("GET /benchmarks", s.handleListBenchmarks)
	mux.HandleFunc("GET /benchmarks/{id}", s.handleGetBenchmark)
	mux.HandleFunc("DELETE /benchmarks/{id}", s.handleCancelBenchmark)
	mux.HandleFunc("GET /benchmarks/{id}/results", s.handleResults)
	mux.HandleFunc("POST /transactions/evaluate", s.handleEvaluate)
	// Qualquer coisa que não casou (rota ou método errado) vira 404 em JSON.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "rota não encontrada: "+r.Method+" "+r.URL.Path)
	})
	return s.middleware(mux)
}

func (s *Server) logger() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

// statusRecorder guarda o status para o log e sabe se já escreveu algo.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.wrote = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wrote = true
	return r.ResponseWriter.Write(b)
}

// Unwrap deixa o http.ResponseController chegar no writer de verdade.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// middleware segura panics (devolvendo 500 em JSON) e registra cada requisição.
func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if p := recover(); p != nil {
				s.logger().Error("panic no handler", "erro", p, "pilha", string(debug.Stack()))
				if !rec.wrote {
					writeError(rec, http.StatusInternalServerError, "internal_error", "erro interno do servidor")
				}
			}
			level := slog.LevelInfo
			if r.URL.Path == "/healthz" {
				level = slog.LevelDebug // o healthcheck do Docker bate a cada poucos segundos
			}
			s.logger().Log(r.Context(), level, "requisição",
				"método", r.Method, "caminho", r.URL.Path, "status", rec.status,
				"duração_ms", time.Since(start).Milliseconds())
		}()
		next.ServeHTTP(rec, r)
	})
}

type errorBody struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string, details ...string) {
	writeJSON(w, status, map[string]errorBody{"error": {Code: code, Message: message, Details: details}})
}

// decodeJSON lê o corpo com limite de 1 MB, recusa campo desconhecido e recusa
// conteúdo depois do objeto. Corpo vazio devolve io.EOF.
func decodeJSON(w http.ResponseWriter, r *http.Request, into any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("o corpo tem conteúdo depois do objeto JSON")
	}
	return nil
}
```

- [ ] **Passo 4: implementar os handlers de saúde e informação**

```go file=internal/infrastructure/web/handlers_meta.go
package web

import (
	"context"
	"net/http"
	"time"

	"fraud-detector-jev/internal/domain"
	"fraud-detector-jev/internal/infrastructure/jev"
	"fraud-detector-jev/internal/service"
)

const ollamaHint = "Confira se o Ollama está rodando no host e escutando em 0.0.0.0:11434 (veja o passo do OLLAMA_HOST no README)."

type componentStatus struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Hint  string `json:"hint,omitempty"`
}

type healthResponse struct {
	Status string          `json:"status"`
	Neo4j  componentStatus `json:"neo4j"`
	Ollama componentStatus `json:"ollama"`
}

// handleHealthz responde 503 só se o Neo4j caiu. O Ollama fora deixa o serviço
// "degraded" mas de pé, porque dá para carregar dados e olhar resultados antigos.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var resp healthResponse
	if err := s.PingNeo4j(ctx); err != nil {
		resp.Neo4j = componentStatus{Error: err.Error()}
	} else {
		resp.Neo4j.OK = true
	}
	if _, err := s.ListModels(ctx); err != nil {
		resp.Ollama = componentStatus{Error: err.Error(), Hint: ollamaHint}
	} else {
		resp.Ollama.OK = true
	}

	status := http.StatusOK
	switch {
	case !resp.Neo4j.OK:
		resp.Status, status = "unavailable", http.StatusServiceUnavailable
	case !resp.Ollama.OK:
		resp.Status = "degraded"
	default:
		resp.Status = "ok"
	}
	writeJSON(w, status, resp)
}

type infoDefaults struct {
	N            int      `json:"n"`
	FraudPct     float64  `json:"fraud_pct"`
	Seed         uint64   `json:"seed"`
	Workers      int      `json:"workers"`
	Graph        bool     `json:"graph"`
	Types        []string `json:"types"`
	WindowHours  int      `json:"window_hours"`
	CycleMaxHops int      `json:"cycle_max_hops"`
	MaxInflight  int      `json:"max_inflight"`
	Threshold    float64  `json:"threshold"`
}

type infoLimits struct {
	MaxSampleSize int `json:"max_sample_size"`
	MaxWorkers    int `json:"max_workers"`
}

type infoResponse struct {
	Model           string               `json:"model"`
	BaseURL         string               `json:"base_url"`
	OllamaOK        bool                 `json:"ollama_ok"`
	AvailableModels []string             `json:"available_models"`
	Dataset         *domain.DatasetStats `json:"dataset"`
	DatasetHint     string               `json:"dataset_hint,omitempty"`
	Defaults        infoDefaults         `json:"defaults"`
	Limits          infoLimits           `json:"limits"`
	ActiveRun       *string              `json:"active_run"`
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	resp := infoResponse{
		Model:           s.Limits.DefaultModel,
		BaseURL:         s.Limits.BaseURL,
		AvailableModels: []string{},
		Defaults: infoDefaults{
			N: service.DefaultN, FraudPct: service.DefaultFraudPct, Seed: service.DefaultSeed,
			Workers: service.DefaultWorkers, Graph: true, Types: service.DefaultTypes,
			WindowHours: service.DefaultWindowHours, CycleMaxHops: service.DefaultCycleMaxHops,
			MaxInflight: service.DefaultMaxInflight, Threshold: service.DefaultThreshold,
		},
		Limits: infoLimits{MaxSampleSize: s.Limits.MaxSampleSize, MaxWorkers: s.Limits.MaxWorkers},
	}

	if models, err := s.ListModels(ctx); err == nil {
		resp.OllamaOK = true
		if decision := jev.DecisionModels(models); decision != nil {
			resp.AvailableModels = decision
		}
	}
	if stats, err := s.Store.ReadDatasetStats(); err == nil {
		resp.Dataset = &stats
	} else {
		resp.DatasetHint = "O dataset ainda não foi carregado. Rode o seeder (make seed) e veja o README."
	}
	if id := s.Runs.ActiveID(); id != "" {
		resp.ActiveRun = &id
	}
	writeJSON(w, http.StatusOK, resp)
}
```

- [ ] **Passo 5: implementar os handlers de benchmark**

```go file=internal/infrastructure/web/handlers_bench.go
package web

import (
	"errors"
	"io"
	"net/http"

	"fraud-detector-jev/internal/domain"
	"fraud-detector-jev/internal/service"
)

func (s *Server) handleStartBenchmark(w http.ResponseWriter, r *http.Request) {
	var req service.RunRequest
	// Corpo vazio vale como "tudo padrão".
	if err := decodeJSON(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_json", "corpo inválido: "+err.Error())
		return
	}

	params, err := req.ToParams(s.Limits)
	var validation *service.ValidationError
	if errors.As(err, &validation) {
		writeError(w, http.StatusBadRequest, "validation_error", "pedido inválido", validation.Problems...)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	sum, err := s.Runs.Start(params)
	switch {
	case errors.Is(err, service.ErrBusy):
		writeError(w, http.StatusConflict, "run_in_progress", "já existe uma execução em andamento", "execução ativa: "+s.Runs.ActiveID())
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "start_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, sum)
}

func (s *Server) handleListBenchmarks(w http.ResponseWriter, _ *http.Request) {
	runs, err := s.Store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if runs == nil {
		runs = []domain.RunSummary{}
	}
	for i := range runs {
		if p, ok := s.Runs.Progress(runs[i].RunID); ok {
			runs[i].Progress = &p
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (s *Server) handleGetBenchmark(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sum, err := s.Store.ReadSummary(id)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "execução não encontrada: "+id)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if p, ok := s.Runs.Progress(id); ok {
		sum.Progress = &p
	}
	writeJSON(w, http.StatusOK, sum)
}

func (s *Server) handleCancelBenchmark(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.Runs.Cancel(id) {
		writeJSON(w, http.StatusAccepted, map[string]any{"canceled": true, "run_id": id})
		return
	}
	if _, err := s.Store.ReadSummary(id); errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "execução não encontrada: "+id)
		return
	}
	writeError(w, http.StatusConflict, "not_running", "a execução não está em andamento")
}

func (s *Server) handleResults(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sum, err := s.Store.ReadSummary(id)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "execução não encontrada: "+id)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if sum.Status == domain.StatusRunning {
		writeError(w, http.StatusConflict, "run_in_progress", "os resultados só ficam disponíveis quando a execução termina")
		return
	}

	results, err := s.Store.OpenResults(id)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "essa execução não gravou resultados")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	defer results.Close()

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, results)
}
```

- [ ] **Passo 6: implementar o handler do simulador**

```go file=internal/infrastructure/web/handlers_eval.go
package web

import (
	"errors"
	"net/http"

	"fraud-detector-jev/internal/service"
)

func (s *Server) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	var req service.EvaluateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corpo inválido: "+err.Error())
		return
	}

	resp, err := s.Evaluator.Evaluate(r.Context(), req)
	var validation *service.ValidationError
	var upstream *service.UpstreamError
	switch {
	case errors.As(err, &validation):
		writeError(w, http.StatusBadRequest, "validation_error", "pedido inválido", validation.Problems...)
	case errors.As(err, &upstream):
		writeError(w, http.StatusBadGateway, "upstream_error", upstream.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
	default:
		writeJSON(w, http.StatusOK, resp)
	}
}
```

- [ ] **Passo 7: rodar e ver passar**

Rode: `go test -race -count=3 ./internal/infrastructure/web/`
Esperado: `ok`.

- [ ] **Passo 8: commitar**

```bash
git add internal/infrastructure/web
git commit -m "feat: adiciona API HTTP com benchmarks, simulador e healthcheck"
```

---

## Tarefa 14: binários da API e do seeder

**Arquivos:**
- Criar: `cmd/api/main.go`, `cmd/seeder/seed_graph.go`
- Testar: `cmd/seeder/seed_graph_test.go`

**Interfaces:**
- Consumes: tudo das tarefas anteriores. `*neo4j.Client` implementa `service.GraphFeatureSource` e `service.SubgraphSource`. `*jev.Client` implementa `service.Classifier`. `*runstore.Store` implementa `service.RunStore` e `web.RunReader`. `*service.RunManager` implementa `web.RunController`. `*service.Evaluator` implementa `web.TransactionEvaluator`.
- Produz: os dois binários. O seeder aceita `-batch` (padrão 10000) e lê o resto das variáveis de ambiente.

O `main` da API só liga as peças e cuida do encerramento: ao receber SIGINT ou SIGTERM, cancela a execução ativa, espera ela gravar e fecha o servidor HTTP.

- [ ] **Passo 1: escrever o teste do contador de linhas do seeder (que falha)**

O contador serve só para estimar o tempo restante da carga, então é aproximado. Ele conta as quebras de linha, desconta o cabeçalho e considera a última linha mesmo sem quebra no fim.

```go file=cmd/seeder/seed_graph_test.go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCountDataRows(t *testing.T) {
	casos := map[string]struct {
		conteudo string
		want     int64
	}{
		"com quebra no fim":    {"cabecalho\na\nb\nc\n", 3},
		"sem quebra no fim":    {"cabecalho\na\nb\nc", 3},
		"só o cabeçalho":       {"cabecalho\n", 0},
		"arquivo vazio":        {"", 0},
		"quebras CRLF":         {"cabecalho\r\na\r\nb\r\n", 2},
		"cabeçalho sem quebra": {"cabecalho", 0},
	}
	for nome, c := range casos {
		t.Run(nome, func(t *testing.T) {
			arquivo := filepath.Join(t.TempDir(), "x.csv")
			if err := os.WriteFile(arquivo, []byte(c.conteudo), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := countDataRows(arquivo)
			if err != nil || got != c.want {
				t.Errorf("countDataRows = %d, %v (esperado %d)", got, err, c.want)
			}
		})
	}
	if _, err := countDataRows("/nao/existe.csv"); err == nil {
		t.Error("arquivo inexistente deveria dar erro")
	}
}
```

- [ ] **Passo 2: rodar e ver falhar**

Rode: `go test ./cmd/seeder/`
Esperado: FAIL com `undefined: countDataRows`.

- [ ] **Passo 3: implementar o seeder**

```go file=cmd/seeder/seed_graph.go
// Command seeder carrega o CSV do PaySim no Neo4j e grava as estatísticas do
// dataset. Pode ser executado de novo depois de uma queda, ele continua de onde
// parou.
package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fraud-detector-jev/internal/config"
	"fraud-detector-jev/internal/infrastructure/neo4j"
	"fraud-detector-jev/internal/infrastructure/runstore"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("a carga falhou", "erro", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	batch := flag.Int("batch", 10000, "quantas linhas por lote de escrita no Neo4j")
	flag.Parse()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	total, err := countDataRows(cfg.DataPath)
	if err != nil {
		return err
	}
	logger.Info("começando a carga", "arquivo", cfg.DataPath, "linhas_aproximadas", total, "lote", *batch)

	client, err := neo4j.New(ctx, neo4j.Options{
		URI: cfg.Neo4jURI, User: cfg.Neo4jUser, Password: cfg.Neo4jPassword, MaxPool: 4,
	})
	if err != nil {
		return err
	}
	defer client.Close(context.Background())

	store, err := runstore.New(cfg.RunsDir)
	if err != nil {
		return err
	}

	start := time.Now()
	report, err := client.Load(ctx, neo4j.LoadOptions{
		CSVPath: cfg.DataPath, BatchSize: *batch, Progress: progressLogger(logger, total),
	})
	if err != nil {
		return err
	}
	if err := store.WriteDatasetStats(report.Stats); err != nil {
		return err
	}
	logger.Info("carga concluída",
		"contas", report.Nodes, "arestas", report.Rels, "retomada", report.Resumed,
		"fraudes", report.Stats.FraudRows, "maior_step", report.Stats.MaxStep,
		"duração", time.Since(start).Round(time.Second))
	return nil
}

// countDataRows conta as linhas de dados só olhando as quebras de linha (é
// rápido e serve para estimar o tempo restante, então é aproximado).
func countDataRows(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var lines int64
	last := byte('\n')
	buf := make([]byte, 1<<20)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			lines += int64(bytes.Count(buf[:n], []byte{'\n'}))
			last = buf[n-1]
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
	}
	if last != '\n' {
		lines++ // a última linha não tinha quebra
	}
	if lines > 0 {
		lines-- // o cabeçalho
	}
	return lines, nil
}

// progressLogger escreve o andamento no máximo a cada 5 segundos. Numa
// retomada a estimativa de tempo fica otimista, porque o contador de linhas já
// começa adiantado.
func progressLogger(logger *slog.Logger, total int64) func(phase string, done int64) {
	var phaseStart, lastLog time.Time
	var lastPhase string
	return func(phase string, done int64) {
		now := time.Now()
		if phase != lastPhase {
			lastPhase, phaseStart, lastLog = phase, now, time.Time{}
		}
		if done < total && now.Sub(lastLog) < 5*time.Second {
			return
		}
		lastLog = now

		rate := 0.0
		if elapsed := now.Sub(phaseStart).Seconds(); elapsed > 0 {
			rate = float64(done) / elapsed
		}
		var eta time.Duration
		if rate > 0 && total > done {
			eta = time.Duration(float64(total-done)/rate) * time.Second
		}
		logger.Info("carregando", "fase", phase, "linhas", done, "total", total,
			"linhas_por_segundo", int(rate), "falta", eta.Round(time.Second))
	}
}
```

- [ ] **Passo 4: rodar e ver passar**

Rode: `go test -race ./cmd/seeder/`
Esperado: `ok`.

- [ ] **Passo 5: implementar o binário da API**

```go file=cmd/api/main.go
// Command api sobe a API REST do detector de fraude.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fraud-detector-jev/internal/config"
	"fraud-detector-jev/internal/domain"
	"fraud-detector-jev/internal/infrastructure/csv"
	"fraud-detector-jev/internal/infrastructure/jev"
	"fraud-detector-jev/internal/infrastructure/neo4j"
	"fraud-detector-jev/internal/infrastructure/runstore"
	"fraud-detector-jev/internal/infrastructure/web"
	"fraud-detector-jev/internal/service"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("o servidor parou com erro", "erro", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	neoClient, err := neo4j.New(connectCtx, neo4j.Options{
		URI: cfg.Neo4jURI, User: cfg.Neo4jUser, Password: cfg.Neo4jPassword,
		MaxPool: cfg.MaxWorkers, QueryTimeout: cfg.Neo4jQueryTimeout,
	})
	cancel()
	if err != nil {
		return err
	}
	defer neoClient.Close(context.Background())

	store, err := runstore.New(cfg.RunsDir)
	if err != nil {
		return err
	}
	if n, err := store.MarkStaleRunsFailed(time.Now()); err != nil {
		return err
	} else if n > 0 {
		logger.Warn("execuções que ficaram penduradas foram marcadas como failed", "quantas", n)
	}

	newClassifier := func(model string, maxInflight int) (service.Classifier, error) {
		client, err := jev.New(jev.Config{
			BaseURL: cfg.JevBaseURL, Model: model, Instructions: cfg.Instructions,
			MaxInflight: maxInflight, Timeout: cfg.JevTimeout,
		})
		if err != nil {
			return nil, err
		}
		return client, nil
	}
	factory := func(p domain.RunParams) (service.Pipeline, error) {
		classifier, err := newClassifier(p.Model, p.MaxInflight)
		if err != nil {
			return service.Pipeline{}, err
		}
		return service.Pipeline{Features: neoClient, Classifier: classifier}, nil
	}
	source := func(ctx context.Context, fn func(domain.Transaction) error) error {
		return csv.Stream(ctx, cfg.DataPath, fn)
	}
	runs := service.NewRunManager(store, source, factory, logger)

	evaluator := &service.Evaluator{
		Features:  neoClient,
		Subgraphs: neoClient,
		ClassifierFor: func(model string) (service.Classifier, error) {
			return newClassifier(model, cfg.JevMaxInflight)
		},
		DefaultModel: cfg.JevModel,
		DefaultStep: func() (int, bool) {
			stats, err := store.ReadDatasetStats()
			if err != nil {
				return 0, false
			}
			return stats.MaxStep, true
		},
		WindowHours:      service.DefaultWindowHours,
		CycleMaxHops:     service.DefaultCycleMaxHops,
		DefaultThreshold: service.DefaultThreshold,
		SubgraphLimit:    50,
	}

	server := &web.Server{
		Runs:      runs,
		Store:     store,
		Evaluator: evaluator,
		Limits: service.Limits{
			MaxSampleSize: cfg.MaxSampleSize, MaxWorkers: cfg.MaxWorkers,
			DefaultModel: cfg.JevModel, BaseURL: cfg.JevBaseURL,
		},
		PingNeo4j: neoClient.Ping,
		ListModels: func(ctx context.Context) ([]string, error) {
			return jev.ListModels(ctx, cfg.JevBaseURL)
		},
		Log: logger,
	}
	httpServer := &http.Server{Addr: cfg.HTTPAddr, Handler: server.Handler(), ReadHeaderTimeout: 10 * time.Second}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("API no ar", "endereço", cfg.HTTPAddr, "modelo", cfg.JevModel, "ollama", cfg.JevBaseURL)
		serveErr <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		logger.Info("recebi o sinal de parada, encerrando")
	}

	runs.Shutdown()
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	return httpServer.Shutdown(shutdownCtx)
}
```

- [ ] **Passo 6: compilar os dois binários e rodar tudo**

Rode: `go build ./... && go vet ./... && go test -race ./...`
Esperado: sem saída de erro do build e do vet, e `ok` em todos os pacotes.

- [ ] **Passo 7: commitar**

```bash
git add cmd
git commit -m "feat: adiciona binários da API e do seeder com encerramento limpo"
```

---
## Tarefa 15: Docker, compose, Makefile e script de smoke

**Arquivos:**
- Criar: `Dockerfile.api`, `.dockerignore`, `docker-compose.yml`, `.env.example`
- Criar: `Makefile`, `scripts/smoke_benchmark.sh`
- Criar: `config/instructions.txt` (a pergunta feita ao modelo, editável)

**Interfaces:**
- Consumes: os binários `cmd/api` e `cmd/seeder`, e `scripts/test-integration.sh` (Tarefa 7).
- Produz:
  - Serviços do compose: `neo4j`, `fraud-api` e `seeder` (este só com `--profile seed`, ou via `docker compose run`).
  - Variáveis do `.env`: `NEO4J_PASSWORD` (obrigatória), `JEV_MODEL`, `JEV_BASE_URL`, `JEV_MAX_INFLIGHT`, `DATA_DIR`, `DATA_FILE`.
  - `config/instructions.txt`, montado na API em `/config/instructions.txt` e lido por `JEV_INSTRUCTIONS_FILE`. É o texto da pergunta `is_fraud`, igual nas duas variantes do benchmark. Para mudar, edite o arquivo e rode `docker compose restart fraud-api`.
  - Alvos do Makefile: `up`, `down`, `seed`, `lint`, `test-go`, `test-integration`, `test-py`, `smoke`.
  - `scripts/smoke_benchmark.sh [n] [modelo] [grafo]`, que roda um benchmark pela API e mostra o resumo. O plano 2 usa o mesmo contrato.

Um cuidado descoberto ao verificar este plano: na máquina de desenvolvimento o build sem `network: host` falhou em `go mod download` com `network is unreachable`, porque o DNS do host é um endereço IPv6 que a rede do container de build não alcança. Por isso o compose e o comando de build usam a rede do host durante o build.

Decisões do compose (todas da spec): imagem `neo4j:5.26-community` com heap de 2G fixo e page cache de 3G (nomes de variável do Neo4j 5), sem APOC, senha vinda do `.env`, portas só em `127.0.0.1`, o CSV montado como somente leitura, volume `runs_data` para as execuções, e `extra_hosts` para a API enxergar o Ollama no host.

Não há teste unitário aqui, a verificação é construir a imagem, validar o compose e rodar o ponta a ponta da Tarefa 17.

- [ ] **Passo 1: criar o Dockerfile da API (também leva o seeder)**

```dockerfile file=Dockerfile.api
# Build em duas etapas. A primeira compila os dois binários e a segunda leva só
# eles para uma imagem pequena, rodando sem root.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/seeder ./cmd/seeder

FROM alpine:3.20
RUN addgroup -S app && adduser -S -G app -u 10001 app \
 && mkdir -p /runs && chown app:app /runs
COPY --from=build /out/api /app/api
COPY --from=build /out/seeder /app/seeder
USER app
EXPOSE 8080
ENTRYPOINT ["/app/api"]
```

```text file=.dockerignore
.git
.env
.claude
data
runs
docs
testdata
scripts
web_dashboard
*.md
```

- [ ] **Passo 2: criar o compose**

```yaml file=docker-compose.yml
name: fraud-detector

# Variáveis compartilhadas entre a API e o seeder.
x-api-env: &api-env
  NEO4J_URI: bolt://neo4j:7687
  NEO4J_USER: neo4j
  NEO4J_PASSWORD: ${NEO4J_PASSWORD:?defina NEO4J_PASSWORD no arquivo .env}
  JEV_BASE_URL: ${JEV_BASE_URL:-http://host.docker.internal:11434}
  JEV_MODEL: ${JEV_MODEL:-tev1:4b}
  JEV_MAX_INFLIGHT: ${JEV_MAX_INFLIGHT:-4}
  DATA_PATH: /data/${DATA_FILE:-transactions.csv}
  RUNS_DIR: /runs

services:
  neo4j:
    image: neo4j:5.26-community
    container_name: fraud_neo4j
    restart: unless-stopped
    ports:
      - "127.0.0.1:7474:7474"
      - "127.0.0.1:7687:7687"
    environment:
      NEO4J_AUTH: neo4j/${NEO4J_PASSWORD:?defina NEO4J_PASSWORD no arquivo .env}
      NEO4J_server_memory_heap_initial__size: 2G
      NEO4J_server_memory_heap_max__size: 2G
      NEO4J_server_memory_pagecache_size: 3G
    volumes:
      - neo4j_data:/data
      - neo4j_logs:/logs
    healthcheck:
      test: ["CMD-SHELL", "cypher-shell -u neo4j -p \"$${NEO4J_AUTH#neo4j/}\" 'RETURN 1' || exit 1"]
      interval: 10s
      timeout: 10s
      retries: 12
      start_period: 40s

  fraud-api:
    build:
      context: .
      dockerfile: Dockerfile.api
      # O build usa a rede do host para baixar os módulos do Go. Em alguns setups
      # o DNS dentro do container de build é um IPv6 sem rota e o download falha.
      network: host
    image: fraud-detector-api
    container_name: fraud_api
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:8080"
    environment:
      <<: *api-env
      JEV_INSTRUCTIONS_FILE: /config/instructions.txt
    extra_hosts:
      - "host.docker.internal:host-gateway"
    volumes:
      - ${DATA_DIR:-./data}:/data:ro
      - ./config/instructions.txt:/config/instructions.txt:ro
      - runs_data:/runs
    depends_on:
      neo4j:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8080/healthz"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 10s

  # Roda só quando pedido: docker compose run --rm seeder (ou make seed).
  seeder:
    build:
      context: .
      dockerfile: Dockerfile.api
      network: host
    image: fraud-detector-api
    profiles: ["seed"]
    entrypoint: ["/app/seeder"]
    environment: *api-env
    volumes:
      - ${DATA_DIR:-./data}:/data:ro
      - runs_data:/runs
    depends_on:
      neo4j:
        condition: service_healthy

volumes:
  neo4j_data:
  neo4j_logs:
  runs_data:
```

```text file=config/instructions.txt
Determine se esta transação tem perfil típico de fraude bancária, esvaziamento ilícito de conta ou lavagem de dinheiro, com base em todos os dados fornecidos.
```

```bash file=.env.example
# Copie este arquivo para .env e ajuste. O .env fica fora do git.

# Senha do Neo4j. Precisa ter pelo menos 8 caracteres. Use só letras, números e
# hífen, porque ela passa por um comando de shell no healthcheck.
NEO4J_PASSWORD=troque-esta-senha-123

# Modelo de decisão do Ollama. Rode "ollama pull tev1:4b" antes.
JEV_MODEL=tev1:4b

# Onde o Ollama está, visto de dentro do container. No Linux o Ollama precisa
# escutar em 0.0.0.0 (veja o passo do OLLAMA_HOST no README).
JEV_BASE_URL=http://host.docker.internal:11434

# Chamadas simultâneas ao modelo. Comece com 4 e ajuste olhando a latência.
JEV_MAX_INFLIGHT=4

# Pasta e nome do CSV do PaySim.
DATA_DIR=./data
DATA_FILE=transactions.csv
```

- [ ] **Passo 3: criar o Makefile**

O Makefile usa TAB no começo das linhas de receita (não espaços).

```make file=Makefile
GO ?= go

.PHONY: up down seed lint test-go test-integration test-py smoke

# Sobe o Neo4j e a API (constrói a imagem se preciso) e espera ficarem saudáveis.
up:
	docker compose up -d --build --wait

down:
	docker compose down

# Carrega o CSV no Neo4j. Pode rodar de novo para retomar de onde parou.
seed:
	docker compose build seeder
	docker compose run --rm seeder

lint:
	@test -z "$$(gofmt -l cmd internal)" || { echo "gofmt encontrou arquivos fora do padrão:"; gofmt -l cmd internal; exit 1; }
	$(GO) vet ./...
	$(GO) vet -tags=integration ./...

test-go:
	$(GO) test -race ./...

# Sobe um Neo4j descartável e roda os testes de integração.
test-integration:
	./scripts/test-integration.sh

test-py:
	cd web_dashboard && .venv/bin/python -m pytest

# Roda um benchmark pequeno pela API. Exemplo: make smoke ARGS="200 tev1:0.8b true"
smoke:
	./scripts/smoke_benchmark.sh $(ARGS)
```

- [ ] **Passo 4: criar o script de smoke**

```bash file=scripts/smoke_benchmark.sh
#!/usr/bin/env bash
# Roda um benchmark pela API, acompanha até terminar e mostra o resumo.
# Uso: ./scripts/smoke_benchmark.sh [n] [modelo] [grafo]
#   n       quantidade de transações (padrão 200)
#   modelo  modelo do Ollama (padrão tev1:0.8b)
#   grafo   true ou false (padrão true)
# A API é lida de API_URL (padrão http://127.0.0.1:8080).
set -euo pipefail

API=${API_URL:-http://127.0.0.1:8080}
N=${1:-200}
MODELO=${2:-tev1:0.8b}
GRAFO=${3:-true}

echo "Iniciando benchmark: n=$N modelo=$MODELO grafo=$GRAFO"
resposta=$(curl -sS -X POST "$API/benchmarks" -H 'Content-Type: application/json' \
  -d "{\"n\": $N, \"model\": \"$MODELO\", \"graph\": $GRAFO}")
if ! id=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["run_id"])' <<<"$resposta" 2>/dev/null); then
  echo "Não consegui iniciar. Resposta da API:"
  echo "$resposta"
  exit 1
fi
echo "run_id: $id"

while true; do
  estado=$(curl -sS "$API/benchmarks/$id")
  linha=$(python3 -c 'import json,sys
d = json.load(sys.stdin)
p = d.get("progress")
print(d["status"], "%d/%d" % (p["done"], p["total"]) if p else "-")' <<<"$estado")
  status=${linha%% *}
  echo "status=$status progresso=${linha#* }"
  [ "$status" != "running" ] && break
  sleep 2
done

python3 -m json.tool <<<"$estado"
[ "$status" = "done" ]
```

Depois de criar o script: `chmod +x scripts/smoke_benchmark.sh`.

- [ ] **Passo 5: validar o Makefile, o compose e a imagem**

```bash
chmod +x scripts/smoke_benchmark.sh
make -n lint test-go
NEO4J_PASSWORD=senha-de-teste-123 docker compose config -q && echo "compose ok"
docker build --network=host -f Dockerfile.api -t fraud-detector-api .
```

Esperado:
- `make -n` imprime os comandos sem erro de "missing separator" (se aparecer, as linhas de receita estão com espaços em vez de TAB).
- `compose ok`.
- O build termina com `naming to docker.io/library/fraud-detector-api` e a imagem final fica pequena (dezenas de MB).

- [ ] **Passo 6: rodar o lint e commitar**

```bash
make lint
git add Dockerfile.api .dockerignore docker-compose.yml .env.example Makefile scripts/smoke_benchmark.sh
git commit -m "feat: adiciona Dockerfile, compose, Makefile e script de smoke"
```

---
## Tarefa 16: README

**Arquivos:**
- Criar: `README.md`

**Interfaces:**
- Consumes: tudo que as tarefas anteriores expõem (comandos do Makefile, variáveis do `.env`, rotas da API).
- Produz: o guia de uso. O plano 2 acrescenta a seção do dashboard no fim.

O README segue a ordem da spec: o que é (com o aviso sobre o modelo), pré-requisitos, PaySim, `.env`, Ollama, Neo4j e carga, API, benchmark, como ler os resultados e limitações, e solução de problemas. Texto em português informal, sem emoji e sem travessão.

- [ ] **Passo 1: criar o README**

````markdown file=README.md
# Detector de fraude com Go, Neo4j e modelo de decisão local

Projeto de estudo que responde uma pergunta simples: **as features de grafo do Neo4j ajudam um modelo de decisão rápido a errar menos ao classificar fraude?**

O Go lê o dataset PaySim, monta uma amostra, enriquece cada transação com métricas de rede (quem já mandou dinheiro para quem, ciclos, fan-in e fan-out) e pergunta a um modelo local, rodando no Ollama, se a transação parece fraude. No fim você compara os resultados com e sem grafo, sobre as mesmas transações.

> **Aviso importante:** o modelo usado aqui **não é a Jev da TypeSafe**. São modelos abertos que falam o mesmo protocolo dela (`/v1/systemone`, pergunta do tipo `noul`): `tev1:4b` e `tev1:0.8b` da Together AI, e `nimble` da Bespoke Labs. Os números medem esses modelos. Cada execução grava qual modelo foi usado.

## Como funciona

1. O Go faz uma passada no CSV e monta uma amostra **estratificada** (você escolhe a % de fraude).
2. Cada transação passa pelo Neo4j, que calcula as features usando **só o passado dela** (nada de espiar o futuro).
3. O Go monta o `state` e pergunta ao modelo `is_fraud` (a resposta é um número de 0 a 1, o `noul`).
4. Cada resultado vai para `runs/<id>/results.jsonl` e o resumo para `summary.json`.
5. Você roda duas vezes com a mesma seed, uma sem grafo e outra com, e compara.

## Pré-requisitos

- Docker com Docker Compose.
- Go 1.27 ou mais novo (só para rodar os testes e o lint fora do Docker).
- Ollama 0.35 ou mais novo, instalado no host (fora do compose).
- O CSV do PaySim (veja o passo 1).
- Opcional: driver da NVIDIA. Sem ele o Ollama roda na CPU, que funciona mas é mais lento.

## 1. Baixar o PaySim

No Kaggle, procure o dataset **Synthetic Financial Datasets For Fraud Detection** (PaySim). É preciso ter uma conta. Baixe e coloque o CSV em `data/transactions.csv`:

```bash
mkdir -p data
mv ~/Downloads/PS_*.csv data/transactions.csv
```

Crie a pasta `data` **antes** de subir o compose. Se ela não existir, o Docker cria uma pasta `data` como root e você vai precisar apagar com `sudo`.

## 2. Configurar o `.env`

```bash
cp .env.example .env
```

Abra o `.env` e troque a senha do Neo4j (`NEO4J_PASSWORD`, pelo menos 8 caracteres, só letras, números e hífen). O resto já vem com um padrão que funciona.

## 3. Instalar o Ollama e baixar o modelo

Instale o Ollama no host:

```bash
curl -fsSL https://ollama.com/install.sh | sh
ollama --version
```

A versão precisa ser 0.35 ou mais nova (o `tev1` exige isso). Depois baixe o modelo padrão:

```bash
ollama pull tev1:4b
```

Outros modelos que você pode comparar: `ollama pull tev1:0.8b` (pequeno e rápido, bom para testar) e `ollama pull nimble` (9B, mais pesado).

**Liberar o Ollama para o container (Linux).** Por padrão o Ollama escuta só em `127.0.0.1`, e o container da API não consegue chegar nele. Faça o Ollama escutar em todas as interfaces:

```bash
sudo systemctl edit ollama
```

Cole no editor:

```ini
[Service]
Environment="OLLAMA_HOST=0.0.0.0:11434"
```

Salve e reinicie:

```bash
sudo systemctl restart ollama
```

Isso faz o Ollama aceitar conexões da rede local. Se você usa uma rede pública, confira o firewall (por exemplo `sudo ufw status`).

Teste direto no host:

```bash
curl http://localhost:11434/api/tags
```

## 4. Subir o Neo4j e a API

```bash
make up
```

Isso constrói a imagem, sobe o Neo4j e a API e espera os dois ficarem saudáveis. Confira:

```bash
curl -s http://127.0.0.1:8080/healthz
```

Esperado: `"status":"ok"`. Se o Ollama estiver fora do ar aparece `"degraded"` com uma dica.

O navegador do Neo4j fica em http://127.0.0.1:7474 (usuário `neo4j`, senha do `.env`).

## 5. Carregar o grafo

```bash
make seed
```

O seeder cria as contas e as arestas em lotes, cria os índices no fim e confere se o total de arestas bate com as linhas do CSV. Ele mostra o andamento e uma estimativa de tempo. Com o PaySim inteiro a carga costuma levar dezenas de minutos.

Se a carga cair no meio (ou você der Ctrl+C), rode `make seed` de novo. Ele continua de onde parou.

Para trocar de dataset, apague os volumes antes (`docker compose down -v`), senão o seeder acha que é a mesma carga e tenta retomar.

## 6. Rodar um benchmark

O jeito mais rápido é o script de smoke. Argumentos: quantidade de transações, modelo e se usa grafo.

```bash
make smoke ARGS="2000 tev1:4b false"
make smoke ARGS="2000 tev1:4b true"
```

Os dois usam a mesma seed (42), então pegam **as mesmas transações**. Só pode haver uma execução por vez, então rode uma depois da outra.

Direto pela API:

```bash
curl -s -X POST http://127.0.0.1:8080/benchmarks \
  -H 'Content-Type: application/json' \
  -d '{"n": 2000, "fraud_pct": 10, "seed": 42, "graph": true, "model": "tev1:4b"}'
```

A resposta traz o `run_id`. Acompanhe com `curl -s http://127.0.0.1:8080/benchmarks/<run_id>`.

Parâmetros (todos opcionais):

| Campo | Padrão | O que faz |
| --- | --- | --- |
| `n` | 2000 | Tamanho da amostra (máximo 500000) |
| `fraud_pct` | 10 | % de fraude na amostra |
| `seed` | 42 | Mesma seed dá a mesma amostra |
| `graph` | true | Usa as features do Neo4j |
| `model` | do `.env` | Modelo do Ollama |
| `workers` | 32 | Consultas simultâneas ao Neo4j (máximo 256) |
| `max_inflight` | 4 | Chamadas simultâneas ao modelo |
| `types` | TRANSFER, CASH_OUT | Tipos de transação da amostra |
| `window_hours` | 24 | Janela do fan-in e do fan-out |
| `cycle_max_hops` | 3 | Tamanho máximo do ciclo procurado |
| `threshold` | 0.5 | Acima disso o modelo decidiu "fraude" |

## Como ler os resultados

O `summary.json` traz:

- `sample`: quantas transações entraram, quantas são fraude, e a prevalência real de fraude no universo de onde saiu a amostra.
- `counts`: TP, FP, TN, FN e `errors` (transações que deram erro no Neo4j ou no modelo e ficaram fora das métricas).
- `metrics`: acurácia, precisão, recall, F1, taxa de falso positivo (`fpr`) e a **precisão reprojetada** (`precision_at_prevalence`).
- `latency`: P50, P95 e P99 em milissegundos, do Neo4j e do modelo, separados.
- `tps`: transações por segundo, e `tokens_total`.

**Por que a precisão reprojetada?** A amostra tem bem mais fraude que a vida real (10% contra algo perto de 0,3%). Recall e taxa de falso positivo não mudam com isso, mas a precisão muda muito. A precisão reprojetada recalcula a precisão como ela seria na proporção real de fraude.

O `results.jsonl` tem uma linha por transação, com o score `noul`, os tempos, o gabarito `is_fraud` e todas as features (as de grafo só existem nas execuções com grafo).

```bash
curl -s http://127.0.0.1:8080/benchmarks/<run_id>/results | head -3
```

## Trocar a pergunta feita ao modelo

O texto da pergunta `is_fraud` fica em `config/instructions.txt`. Edite o arquivo e reinicie a API:

```bash
docker compose restart fraud-api
```

O mesmo texto vai nas duas variantes do benchmark (com e sem grafo), para a única diferença entre elas ser o `state`. Não se sabe de antemão se os modelos pequenos rendem melhor em português ou em inglês, então vale testar as duas versões.

## Limitações (leia antes de tirar conclusão)

- **O modelo não é a Jev.** Modelos pequenos podem ir mal em fraude. Comparar `tev1:4b`, `tev1:0.8b` e `nimble` faz parte do estudo.
- **O grafo do PaySim é esparso.** A maioria das contas de origem aparece poucas vezes, então ciclos e fan-in podem ser raros e o ganho do grafo pode ser pequeno. O projeto mede, não promete melhora.
- **Amostra estratificada.** Olhe recall, taxa de falso positivo e a precisão reprojetada. A precisão "crua" da amostra engana.
- **Ordem dentro de um mesmo `step`.** Tratamos a ordem das linhas do arquivo como ordem de chegada. É uma premissa.
- **Velocidade.** A latência do modelo depende do seu hardware. Ajuste `max_inflight` olhando o TPS e o P95.

## API em resumo

| Rota | Para que serve |
| --- | --- |
| `GET /healthz` | Saúde do Neo4j e do Ollama |
| `GET /info` | Modelo em uso, modelos disponíveis, estatísticas do dataset e padrões |
| `POST /benchmarks` | Inicia uma execução (uma por vez, senão 409) |
| `GET /benchmarks` | Lista as execuções |
| `GET /benchmarks/{id}` | Resumo e progresso |
| `DELETE /benchmarks/{id}` | Cancela uma execução em andamento |
| `GET /benchmarks/{id}/results` | Resultados em JSON Lines (só depois de terminar) |
| `POST /transactions/evaluate` | Simulador de uma transação (com `seq` faz o replay de uma transação real) |

Os erros vêm sempre como `{"error": {"code": "...", "message": "..."}}`.

## Desenvolvimento

```bash
make lint              # gofmt e go vet
make test-go           # testes unitários, com -race
make test-integration  # sobe um Neo4j descartável e testa as queries de verdade
```

Estrutura:

```text
config/                 texto da pergunta ao modelo (editável)
cmd/api, cmd/seeder     binários
internal/domain         tipos e métricas
internal/service        amostragem, pipeline, gerenciador de execuções, simulador
internal/infrastructure csv, neo4j, jev (cliente do modelo), runstore, web
testdata/               casos de teste compartilhados
docs/superpowers/       spec e planos
```

## Solução de problemas

- **`defina NEO4J_PASSWORD no arquivo .env`**: você ainda não criou o `.env`. Rode `cp .env.example .env`.
- **`/healthz` mostra `ollama` fora do ar**: o Ollama não está rodando, ou está escutando só em `127.0.0.1`. Refaça o passo do `OLLAMA_HOST` e teste `curl http://localhost:11434/api/tags`.
- **A execução falha com "o modelo não respondeu no aquecimento"**: o modelo não foi baixado (`ollama pull ...`), o nome está errado, ou o Ollama está inacessível.
- **`409 run_in_progress` ao iniciar**: já tem uma execução rodando. Espere terminar ou cancele com `DELETE /benchmarks/<id>`.
- **O Neo4j reinicia ou morre por falta de memória**: baixe o `heap` e o `pagecache` no `docker-compose.yml` (padrão 2G e 3G).
- **O build da imagem falha em `go mod download` com `network is unreachable`**: o DNS dentro do container de build não funciona (comum quando o host usa DNS por IPv6). O compose já faz o build na rede do host. Se ainda assim falhar, configure o DNS do Docker em `/etc/docker/daemon.json` (por exemplo `{"dns": ["1.1.1.1", "8.8.8.8"]}`) e reinicie o Docker.
- **A carga caiu no meio**: rode `make seed` de novo.
- **A pasta `data` ficou dona do root**: o Docker a criou porque ela não existia. Apague com `sudo rm -r data`, crie de novo e ponha o CSV.
- **Porta ocupada (7474, 7687 ou 8080)**: outro serviço está usando. Pare o outro ou mude a porta no `docker-compose.yml`.
````

- [ ] **Passo 2: conferir o estilo do texto**

Rode, na raiz do projeto:

```bash
grep -nP '[—–]' README.md || echo "sem travessão"
grep -n ';' README.md || echo "sem ponto e vírgula"
```

Esperado: `sem travessão` e `sem ponto e vírgula`.

- [ ] **Passo 3: commitar**

```bash
git add README.md
git commit -m "docs: adiciona README com o passo a passo de uso"
```

---

## Tarefa 17: aceite ponta a ponta do backend

**Arquivos:** nenhum novo. É a definição de pronto da spec para o backend.

**Interfaces:**
- Consumes: tudo. Precisa do Docker, do Ollama instalado no host e (nos passos 6 em diante) do CSV real do PaySim.
- Produz: a confirmação de que o backend funciona de ponta a ponta. O plano 2 parte daqui.

Os passos 3 e 5 pedem comandos com `sudo` (instalar o Ollama, mexer no systemd) e download no Kaggle. Quem executa não consegue fazer isso sozinho: peça para a pessoa rodar com o prefixo `!` no prompt, ou faça por ela e confirme.

- [ ] **Passo 1: testes automáticos**

```bash
make lint
make test-go
make test-integration
```

Esperado: os três terminam sem falha.

- [ ] **Passo 2: subir com o CSV pequeno e carregar o grafo**

Crie o `.env` e aponte para o CSV de 11 linhas, que já está no repositório:

```bash
cp .env.example .env
```

No `.env`, defina uma senha válida e troque as duas últimas linhas:

```text
DATA_DIR=./testdata
DATA_FILE=mini_paysim.csv
```

Depois:

```bash
make up
make seed
curl -s http://127.0.0.1:8080/info | python3 -m json.tool
```

Esperado:
- `make up` termina com `neo4j` e `fraud-api` saudáveis (`docker compose ps`).
- O seeder loga `carga concluída` com `contas=12` e `arestas=11`.
- No `/info`, `dataset.total_rows` é `11`, `dataset.fraud_rows` é `1` e `dataset.max_step` é `32`.

- [ ] **Passo 3: instalar o Ollama e liberar o acesso do container**

Siga a seção 3 do README: instalar, conferir a versão (0.35 ou mais nova), `ollama pull tev1:0.8b`, criar o override do systemd com `OLLAMA_HOST=0.0.0.0:11434` e reiniciar. Depois:

```bash
curl -s http://127.0.0.1:8080/healthz
```

Esperado: `"status":"ok"` com `ollama.ok` verdadeiro. Se vier `degraded`, o container não está alcançando o Ollama e a dica do corpo aponta o passo do `OLLAMA_HOST`.

- [ ] **Passo 4: smoke com o CSV pequeno, com e sem grafo**

```bash
make smoke ARGS="50 tev1:0.8b true"
make smoke ARGS="50 tev1:0.8b false"
```

Esperado nas duas execuções:
- `status` `done`.
- `sample.n` igual a `11` (o pedido de 50 é maior que o universo), `sample.n_fraud` igual a `1` e `sample.fraud_shortfall` igual a `true` (foram pedidas 5 fraudes e só existe 1).
- `counts.tp + fp + tn + fn + errors` igual a `11`.
- Na execução **com** grafo, `latency.neo4j_ms.p50` maior que zero. Na **sem** grafo, todos os percentis do Neo4j são zero.

- [ ] **Passo 5: trocar para o PaySim inteiro**

Baixe o CSV (seção 1 do README) para `data/transactions.csv`, volte o `.env` para `DATA_DIR=./data` e `DATA_FILE=transactions.csv`, apague os volumes (o seeder não sabe que o dataset mudou) e recarregue:

```bash
docker compose down -v
make up
make seed
```

Esperado ao fim da carga: o log `carga concluída` traz `arestas` igual ao número de linhas do CSV (`wc -l data/transactions.csv` menos 1). Anote o tempo da carga.

- [ ] **Passo 6: benchmark de 200 com e sem grafo**

```bash
make smoke ARGS="200 tev1:0.8b false"
make smoke ARGS="200 tev1:0.8b true"
```

Esperado: as duas terminam `done`, com o mesmo `sample.n_fraud` e o mesmo `sample.n_legit` (mesma seed, mesma amostra), e `sample.prevalence` na casa de 0,3% (o valor exato vem do sampler). `curl -s http://127.0.0.1:8080/benchmarks/<run_id>/results | wc -l` deve dar `200` menos as linhas com erro.

- [ ] **Passo 7: benchmark de 2000 com o modelo padrão**

```bash
make smoke ARGS="2000 tev1:4b false"
make smoke ARGS="2000 tev1:4b true"
```

Esperado: as duas terminam `done`. Anote `tps` e os percentis de latência do Neo4j e do modelo de cada uma. Se o modelo estiver lento demais, ajuste `max_inflight` e rode de novo. Essas anotações alimentam a Aba 2 do dashboard.

- [ ] **Passo 8: cancelamento e limite de uma execução por vez**

```bash
curl -s -X POST http://127.0.0.1:8080/benchmarks -H 'Content-Type: application/json' -d '{"n": 20000}'
curl -s -X POST http://127.0.0.1:8080/benchmarks -H 'Content-Type: application/json' -d '{"n": 10}'
```

Esperado: o primeiro devolve 202 com `run_id`. O segundo devolve 409 `run_in_progress`. Depois:

```bash
curl -s -X DELETE http://127.0.0.1:8080/benchmarks/<run_id>
```

Esperado: 202 `canceled: true`. Em poucos segundos `GET /benchmarks/<run_id>` mostra `status` `canceled`, e `/results` traz as linhas que já tinham sido gravadas (sem nenhuma com `error` de cancelamento). Uma nova execução volta a ser aceita.

- [ ] **Passo 9: replay do simulador**

Pegue uma linha de uma execução **com grafo** e refaça a consulta com o `seq` dela:

```bash
curl -s http://127.0.0.1:8080/benchmarks/<run_id_com_grafo>/results | head -1 | python3 -m json.tool
```

Monte o pedido com `orig`, `dest`, `type`, `amount`, `step`, os quatro saldos e o `seq` dessa linha:

```bash
curl -s -X POST http://127.0.0.1:8080/transactions/evaluate \
  -H 'Content-Type: application/json' \
  -d '{"orig":"...","dest":"...","type":"TRANSFER","amount":0,"step":0,
       "orig_balance_before":0,"orig_balance_after":0,"dest_balance_before":0,"dest_balance_after":0,
       "seq":0}' | python3 -m json.tool
```

Esperado: `replay` verdadeiro e `graph_features` **idêntico** aos campos de grafo da linha do `results.jsonl` (o score do modelo pode variar um pouco entre chamadas, as features não). Sem o campo `seq`, a mesma consulta trata a transação como "de agora" e enxerga o grafo inteiro até o `step` informado, então os números podem ser maiores.

- [ ] **Passo 10: reinício durante uma execução**

Há duas situações diferentes, e as duas precisam funcionar.

**a) Encerramento limpo.** O `docker compose restart` manda SIGTERM, e a API cancela a execução ativa, espera ela gravar e só então sai:

```bash
curl -s -X POST http://127.0.0.1:8080/benchmarks -H 'Content-Type: application/json' -d '{"n": 20000}'
docker compose restart fraud-api
curl -s http://127.0.0.1:8080/benchmarks/<run_id> | python3 -m json.tool
```

Esperado: `status` `canceled`, sem `error`, e o `/results` com as linhas que já estavam gravadas.

**b) Queda brusca.** Um SIGKILL (ou falta de memória) não deixa a API se despedir, então o resumo fica preso em `running`. Ao subir de novo, a API marca essas execuções como `failed`:

```bash
curl -s -X POST http://127.0.0.1:8080/benchmarks -H 'Content-Type: application/json' -d '{"n": 20000}'
docker compose kill -s SIGKILL fraud-api
docker compose up -d --wait fraud-api
curl -s http://127.0.0.1:8080/benchmarks/<run_id> | python3 -m json.tool
```

Esperado: `status` `failed` e `error` igual a `execução interrompida porque o servidor foi reiniciado`. O log da API traz o aviso `execuções que ficaram penduradas foram marcadas como failed`.

- [ ] **Passo 11: registrar o resultado**

Se tudo passou, anote no fim do `docs/superpowers/plans/2026-09-30-fraud-detector-backend.md` a data, o tempo da carga, o `tps` e as latências do passo 7. Se algo falhou, corrija na tarefa dona do código antes de seguir para o plano 2.

```bash
git add docs/superpowers/plans
git commit -m "docs: registra o resultado do aceite do backend"
```

---
