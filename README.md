# Detector de Fraude Bancária com Go, Neo4j e Modelo de Decisão Local

[![GitHub](https://img.shields.io/badge/GitHub-airton--lima-blue)](https://github.com/airton-lima)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go 1.27+](https://img.shields.io/badge/Go-1.27+-blue)](https://golang.org/)
[![Python 3.12+](https://img.shields.io/badge/Python-3.12+-blue)](https://www.python.org/)

Projeto de estudo que responde uma pergunta simples: **as features de grafo do Neo4j ajudam um modelo de decisão rápido a errar menos ao classificar fraude?**

O Go lê o dataset PaySim, monta uma amostra, enriquece cada transação com métricas de rede (quem já mandou dinheiro para quem, ciclos, fan-in/fan-out) e pergunta a um modelo local rodando no Ollama se a transação parece fraude. No fim você compara os resultados com e sem grafo, sobre as mesmas transações.

> **⚠️ Aviso:** o modelo usado aqui **não é a Jev da TypeSafe**. São modelos abertos que falam o mesmo protocolo (`/v1/systemone`, pergunta do tipo `noul`): `tev1:4b` e `tev1:0.8b` da Together AI, e `nimble` da Bespoke Labs. Cada execução grava qual modelo foi usado.

## Sumário

- [Início rápido](#início-rápido)
- [O que mudou recentemente](#o-que-mudou-recentemente)
- [Como funciona](#como-funciona)
- [Arquitetura](#arquitetura)
- [Pré-requisitos](#pré-requisitos)
- [Instalação passo a passo](#instalação-passo-a-passo)
- [Rodar um benchmark](#rodar-um-benchmark)
- [Entendendo o modelo Jev](#entendendo-o-modelo-jev)
- [Dashboard (4 abas)](#dashboard-4-abas)
- [Desenvolvimento e testes](#desenvolvimento-e-testes)
- [Variáveis de ambiente](#variáveis-de-ambiente)
- [Licença](#licença)

## Início rápido

Com Docker, Ollama no host e o CSV do PaySim:

```bash
# 1. Preparar dados
mkdir -p data && mv ~/Downloads/PS_*.csv data/transactions.csv
cp .env.example .env  # edite a NEO4J_PASSWORD

# 2. Baixar o modelo
ollama pull tev1:4b

# 3. Subir serviços (Neo4j, API, dashboard)
make up

# 4. Carregar o grafo (6.3M transações ~5min)
make seed

# 5. Rodar benchmarks (compare com e sem grafo)
make smoke ARGS="2000 tev1:4b false"  # sem grafo
make smoke ARGS="2000 tev1:4b true"   # com grafo

# 6. Abrir dashboard
# http://127.0.0.1:7860
```

## O que mudou recentemente

### ✨ Adição de 3 Colunas Opcionais (Agora)

O leitor do CSV agora aceita **`paymentMethod`, `channel` e `category`** depois das 10 colunas obrigatórias. Quando existem, entram no `state` do modelo e na aba de perfil transacional:

- **Forma de pagamento:** `CREDIT_CARD`, `DEBIT_CARD`, `CONTACTLESS`, `PIX`, `BOLETO`, `DIGITAL_WALLET`
- **Canal:** `MOBILE_APP`, `WEB`, `ATM`, `BRANCH`, `POS`, `PHONE`
- **Categoria:** `SHOPPING`, `FOOD`, `BILLS`, `TRAVEL`, `SERVICES`, `EDUCATION`, `HEALTH`

Adicione as colunas ao CSV com:

```bash
python3 scripts/add_transaction_columns.py data/transactions.csv data/transactions.enriched.csv
mv data/transactions.enriched.csv data/transactions.csv
```

**Por que?** Para ver como o modelo e a árvore de decisão reagem a informação sem sinal. As colunas são sorteadas ao acaso, não têm relação com fraude, e servem para testar o pipeline.

### 📊 Testes Novos (TDD)

Adicionadas 23 testes de integração:
- Leitor do CSV com colunas opcionais
- State com e sem as colunas
- Dashboard: sortear transação com as colunas
- Simulador: enviar as colunas no pedido
- Árvore substituta: tratar categorias como features

Todos passam com `make test-py` (137 testes) e `make test-go`.

## Como funciona

```
1. Sampler estratificado  --> amostra N transações (mantém % de fraude)
2. Cada transação vira:
   - Query Neo4j          --> features de grafo (ciclo, fan-in, idade da conta)
   - State JSON           --> transaction_id, type, amount, saldos, features de grafo
   - Chamada Ollama       --> POST /v1/systemone com a pergunta "is_fraud"
3. Resposta do modelo    --> noul entre 0 e 1 (0=legítimo, 1=fraude)
4. Decisão              --> se noul >= threshold (0.5): fraude
5. Resultado            --> grava em runs/<id>/results.jsonl
6. Métricas             --> TP, FP, TN, FN, ROC, PR, precisão reprojetada
7. Comparação           --> execução A (sem grafo) vs B (com grafo)
```

## Arquitetura

```
PaySim CSV
    ├── seeder --> Neo4j (6.3M contas + transferências)
    └── API em Go (fraud-api, :8080)
            ├── pipeline (paralelo: Neo4j + modelo + coletor)
            ├── --> Neo4j: calcula features, só com o passado da tx
            ├── --> Ollama: pergunta /v1/systemone (no host)
            └── dashboard Gradio (Python, :7860)
                    ├── Aba 1: Benchmark e impacto do grafo
                    ├── Aba 2: Performance e latência
                    ├── Aba 3: Simulador (teste uma tx manual)
                    └── Aba 4: Perfil transacional (árvore explicável)
```

**Componentes:**
- **Seeder (Go):** lê CSV streaming, carrega contas e transferências
- **API (Go):** monta amostra, roda pipeline paralelo, persiste resultados
- **Neo4j:** 7 queries de grafo, sempre com `seq < beforeSeq` (sem futuros)
- **Ollama (host):** modelo local, protocolo `/v1/systemone`
- **Dashboard (Python + Gradio):** cliente fino da API, nunca lê arquivo

## Pré-requisitos

- **Docker** com Docker Compose
- **Ollama 0.35+** no host (fora do compose), escutando em `0.0.0.0:11434`
- **Go 1.27+** (opcional, para testes e lint fora do Docker)
- **Python 3.12+** (opcional, para rodar testes do dashboard)
- **CSV do PaySim** (~6.3M linhas, Kaggle)
- **GPU NVIDIA** (opcional, acelera o Ollama 100x)

## Instalação passo a passo

### 1. Baixar o PaySim

No [Kaggle](https://www.kaggle.com), procure **"Synthetic Financial Datasets For Fraud Detection"** (PaySim):

```bash
mkdir -p data
mv ~/Downloads/PS_*.csv data/transactions.csv
```

**Importante:** crie a pasta antes de subir o compose, senão o Docker a cria como root.

### 2. Configurar `.env`

```bash
cp .env.example .env
# Edite: NEO4J_PASSWORD (qualquer senha, descartável), JEV_MODEL (tev1:4b ou tev1:0.8b)
```

### 3. Instalar Ollama (Sem sudo)

```bash
# Opção 1: instalar no host
curl -fsSL https://ollama.ai/install.sh | sh
ollama serve  # em outro terminal

# Opção 2: container avulso (recomendado se não tiver privilégio)
make ollama
```

Baixe o modelo:
```bash
ollama pull tev1:4b       # 4B params, ~2.5GB
ollama pull tev1:0.8b     # 0.8B params, ~500MB (mais rápido)
```

Confirme que está alcançável:
```bash
curl http://0.0.0.0:11434/api/tags
```

### 4. Subir Neo4j, API e Dashboard

```bash
make up
# Neo4j: http://127.0.0.1:7687
# API: http://127.0.0.1:8080/info
# Dashboard: http://127.0.0.1:7860
```

### 5. Carregar o grafo

```bash
make seed
# Lê transactions.csv, carrega 6.3M linhas em Neo4j (~5 minutos)
```

### 6. Rodar um benchmark

```bash
make smoke ARGS="200 tev1:4b false"   # 200 tx, modelo tev1:4b, SEM grafo
make smoke ARGS="200 tev1:4b true"    # 200 tx, modelo tev1:4b, COM grafo
```

**Resultado:** `runs/<id>/summary.json` e `runs/<id>/results.jsonl`

### 7. Abrir o dashboard

Acesse http://127.0.0.1:7860 no navegador. As duas execuções aparecem na Aba 1.

## Rodar um benchmark

### Benchmark rápido (teste)

```bash
make smoke ARGS="200 tev1:0.8b true"
```

### Benchmark pequeno

```bash
make smoke ARGS="2000 tev1:4b true"
```

### Benchmark completo

```bash
make smoke ARGS="20000 tev1:4b true"
```

Também pode usar a API REST:

```bash
curl -X POST http://127.0.0.1:8080/benchmarks \
  -H "Content-Type: application/json" \
  -d '{
    "n": 2000,
    "fraud_pct": 10,
    "seed": 42,
    "model": "tev1:4b",
    "graph": true,
    "types": ["TRANSFER", "CASH_OUT"],
    "window_hours": 24,
    "cycle_max_hops": 3,
    "workers": 32,
    "max_inflight": 4,
    "threshold": 0.5
  }'
```

## Entendendo o modelo Jev

### A Pergunta

```
"Determine se esta transação tem perfil típico de fraude bancária, 
esvaziamento ilícito de conta ou lavagem de dinheiro, com base em 
todos os dados fornecidos."
```

### O State (JSON)

```json
{
  "transaction_id": "TX_1029",
  "type": "TRANSFER",
  "amount": 350000.0,
  "time_step": 4,
  "orig_balance_before": 350000.0,
  "orig_balance_after": 0.0,
  "dest_balance_before": 0.0,
  "dest_balance_after": 0.0,
  "balance_drop": 350000.0,
  "dest_is_merchant": false,
  "payment_method": "PIX",
  "channel": "MOBILE_APP",
  "category": "SHOPPING",
  "graph_features": {
    "first_transfer_between_pair": true,
    "dest_in_degree": 42,
    "dest_fan_in_window": 17,
    "dest_fan_out_window": 0,
    "orig_fan_out_window": 0,
    "dest_account_age_steps": 3,
    "cycle_detected": true
  }
}
```

**Sem grafo:** o bloco `graph_features` não existe.

### A Resposta

```json
{
  "answers": {
    "is_fraud": {
      "type": "noul",
      "noul": 0.75
    }
  },
  "usage": {
    "input_tokens": 245
  }
}
```

- `noul` é um número entre 0 (legítimo) e 1 (fraude)
- Comparado com o `threshold` (padrão 0.5) para tomar a decisão

### Com e Sem Grafo

| | Sem Grafo | Com Grafo |
|---|---|---|
| **State** | Só dados tabulares | + 7 features de rede |
| **Neo4j** | Não chama | Query por transação |
| **Tokens** | ~200 | ~400 |
| **Latência** | ~50ms | ~150ms |
| **Pergunta** | Mesma | Mesma |

## Dashboard (4 abas)

### Aba 1: Benchmark e Impacto do Grafo

Compara **Execução A** (sem grafo) vs **Execução B** (com grafo):
- Acurácia, precisão, recall, F1, ROC/PR
- Diferença em pontos percentuais
- Melhora da métrica de precisão no cenário real

### Aba 2: Performance e Latência

- Throughput (tx/s)
- P50, P95, P99 do Neo4j e do modelo
- Histogramas de latência
- Tokens gastos

### Aba 3: Simulador

Digite uma transação ou clique "Sortear da Execução A" para testar o modelo:
- Veredito (legítimo/fraude)
- Score do modelo
- Features de grafo (se houver)
- Tempos (Neo4j, modelo)
- State e resposta bruta (JSON)
- Subgrafo (vizinhança no Neo4j)

### Aba 4: Perfil Transacional

Árvore substituta (decision tree) que explica as decisões do modelo:
- Top features (o que mais pesa)
- Regras legíveis em português (ex: "ciclo detectado: sim")
- Perfis (grupos de transações com mesmas regras)
- Fidelidade da árvore (% de decisões reproduzidas)
- Aviso se a árvore explica mal o modelo

## Desenvolvimento e testes

### Estrutura

```
fraud-detector-jev/
├── cmd/
│   ├── api/              # API REST, main
│   └── seeder/           # Carrega o Neo4j
├── internal/
│   ├── domain/           # Tipos e métricas
│   ├── service/          # Regras de negócio
│   ├── infrastructure/   # Adaptadores (Neo4j, Jev, Web, etc)
│   └── config/
├── web_dashboard/        # Python + Gradio
│   ├── tests/
│   ├── app.py            # Layout (Gradio)
│   ├── handlers.py       # Lógica das abas (puro, testável)
│   ├── analysis.py       # Métricas
│   ├── surrogate.py      # Árvore substituta
│   └── client.py         # HTTP para a API
├── scripts/              # Utilidades
│   └── add_transaction_columns.py
├── testdata/             # Fixtures (mini_paysim.csv, golden.json)
└── Makefile              # Receitas (make up, make seed, make test-go, etc)
```

### Comandos

```bash
# Lint
make lint                  # Go (gofmt + vet + tags=integration)
make lint-py              # Python (ruff check + ruff format)

# Testes
make test-go              # Go (go test -race ./...)
make test-py              # Python (pytest web_dashboard/)
make test-integration     # Go com tag integration (Neo4j descartável)

# Um teste só
go test ./internal/service -run TestEvaluate
cd web_dashboard && .venv/bin/python -m pytest tests/test_app.py::test_nome

# Verificar formato Python
web_dashboard/.venv/bin/ruff format --check web_dashboard
web_dashboard/.venv/bin/ruff format web_dashboard  # corrige
```

### Teste-Primeiro (TDD)

As mudanças recentes foram desenvolvidas com **teste-primeiro**:

1. Escrever o teste (vê falhar)
2. Escrever o código mínimo (vê passar)
3. Refatorar (mantém verde)

Exemplos:
- `internal/domain/result_test.go`: testa se as colunas aparecem no JSON
- `internal/infrastructure/csv/streaming_reader_test.go`: testa leitor com colunas
- `web_dashboard/tests/test_surrogate.py`: testa árvore com as categorias

## Variáveis de ambiente

```bash
# .env (descartável, copiado de .env.example)
NEO4J_PASSWORD=senha-local   # qualquer coisa, banco local
JEV_MODEL=tev1:4b            # ou tev1:0.8b, nimble, etc
JEV_BASE_URL=http://host.docker.internal:11434
JEV_MAX_INFLIGHT=4
DATA_DIR=./data
DATA_FILE=transactions.csv
```

## Licença

MIT. Veja [LICENSE](LICENSE).

O dataset PaySim tem sua própria licença (Kaggle). Não redistribua sem verificar.

---

**Próximos passos (ideias):**

- Rodar com CSV real, Ollama real e GPU NVIDIA
- Testar com modelos maiores (`tev1:7b`, `tev1:70b`)
- Integrar mais tipos de features (temporal, behavioral)
- Validação cruzada e bootstrap para erro de estimativa
- Exportar a árvore para Cypher (usar diretamente no Neo4j)

**Contato/Autor:**

Airton Lira - [GitHub](https://github.com/airton-lima)
