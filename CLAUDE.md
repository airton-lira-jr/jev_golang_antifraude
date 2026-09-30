# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## O que é

Projeto de estudo que mede se features de grafo do Neo4j ajudam um modelo de decisão local (Ollama, protocolo `/v1/systemone`) a classificar fraude no dataset PaySim. São duas partes: um backend em Go (API REST e seeder) e um dashboard em Gradio (Python) que só fala com a API.

O modelo **não é a Jev da TypeSafe**. São modelos abertos que falam o mesmo protocolo. A tela e o README dizem isso, e cada execução grava o modelo usado. Não escreva nada que sugira o contrário.

A spec em `docs/superpowers/specs/2026-09-30-fraud-detector-jev-design.md` é a fonte de verdade. Os dois planos em `docs/superpowers/plans/` argumentam a partir dela e trazem no fim o registro do aceite. Se spec e plano discordarem, vale a spec.

## Comandos

```bash
make up                 # sobe Neo4j, API e dashboard (constrói as imagens) e espera ficarem saudáveis
make seed               # carrega o CSV no Neo4j (pode rodar de novo para retomar)
make down

make lint               # gofmt e go vet, com e sem a tag integration
make test-go            # go test -race ./...
make test-integration   # sobe um Neo4j descartável (scripts/test-integration.sh)
make setup-py           # cria web_dashboard/.venv
make lint-py            # ruff check e ruff format --check
make test-py            # pytest do dashboard
make smoke ARGS="200 tev1:0.8b true"   # benchmark pela API: n, modelo, com grafo
```

Um teste só:

```bash
go test -race ./internal/service -run TestEvaluate
cd web_dashboard && .venv/bin/python -m pytest tests/test_app.py::test_nome
```

O `make test-integration` roda o pacote do Neo4j inteiro. Para um teste de integração só, suba um Neo4j descartável, exporte `NEO4J_TEST_URI` e `NEO4J_TEST_PASSWORD` e rode `go test -tags=integration ./internal/infrastructure/neo4j -run NomeDoTeste`. Nunca aponte essas variáveis para a base do compose, porque os testes limpam o grafo.

Para desenvolver sem o PaySim inteiro, ponha `DATA_DIR=./testdata` e `DATA_FILE=mini_paysim.csv` no `.env` (11 linhas, 12 contas). O Ollama real, o PaySim inteiro e leitores de tela ainda não foram verificados: o aceite rodou com um Ollama falso e um CSV sintético.

## Arquitetura

### Backend (Go)

Módulo `fraud-detector-jev`. A única dependência externa é o driver do Neo4j.

- `internal/domain`: tipos e métricas, sem dependências.
- `internal/service`: as regras (sampler, montagem do `state`, pipeline, `RunManager`, `Evaluator`).
- `internal/infrastructure/{csv,neo4j,jev,runstore,web}`: adaptadores.
- `cmd/api` liga as peças e cuida do encerramento. `cmd/seeder` carrega o CSV.

As interfaces ficam do lado de quem consome: `service.GraphFeatureSource`, `SubgraphSource`, `Classifier`, `RunStore`, e no pacote `web` as `RunController`, `RunReader` e `TransactionEvaluator`. Os adaptadores só implementam. Para trocar um fornecedor, mexa no adaptador e não no serviço.

Fluxo de um benchmark: sampler estratificado (uma passada no CSV), alimentador, pool de workers do Neo4j, chamador do modelo (semáforo `max_inflight`, retry só em 5xx e erro de rede) e um coletor, que é o único que escreve `runs/<id>/results.jsonl` e depois `summary.json`. Antes de medir, o pipeline aquece o modelo (3 chamadas) e o Neo4j (1 consulta falsa por worker). O `RunManager` aceita uma execução por vez (409 se houver outra). Ao receber SIGTERM a API cancela a execução ativa e espera gravar. Execuções que ficaram `running` por queda brusca viram `failed` na subida.

### Regras que não podem quebrar

- **Nada de espiar o futuro.** A régua do tempo é o `seq` (número da linha do CSV). Toda query de feature filtra `t.seq < $beforeSeq AND t.step <= $atStep`, e nenhuma lê `isFraud` ou `isFlagged`. Só o subgrafo lê `isFraud`, para pintar arestas do passado. Toda query nova precisa manter isso. Os guardas são `features_queries_test.go` (texto das queries) e `TestFeaturesSoEnxergamOPassado` em `leak_integration_test.go`.
- **CSV ordenado por `step`.** O leitor devolve `csv.ErrUnsortedSteps` se o step diminuir. As dez primeiras colunas são obrigatórias, nessa ordem.
- **Contrato do modelo.** `POST /v1/systemone` com o `state` como string JSON e a pergunta `is_fraud` do tipo `noul`. A resposta vem em `answers.is_fraud.noul`, entre 0 e 1. Resposta inválida vira erro daquela transação (`ErrInvalidResponse`, sem retry) e não derruba a execução.
- **Mesma pergunta com e sem grafo** (`config/instructions.txt`), para a única diferença entre as duas variantes ser o `state`.
- **Métricas.** A precisão reprojetada é `TPR*pi / (TPR*pi + FPR*(1-pi))`, com `pi` a prevalência real do universo elegível. Divisão por zero dá 0. Go e Python usam as mesmas fórmulas e o mesmo `testdata/metrics_golden.json`. Mudou a fórmula de um lado, mude o golden e o outro lado.
- **Cypher.** Só `cycle_max_hops` entra por `Sprintf` (validado entre 1 e 5 antes). Todo o resto vai como parâmetro.
- **Replay do simulador.** `POST /transactions/evaluate` com `seq` usa só o passado daquela transação. Para reproduzir o score de um benchmark, o pedido também precisa levar `graph`, `window_hours` e `cycle_max_hops` da execução de origem.

### API REST

`/healthz` (200 com `degraded` se só o Ollama caiu, 503 se o Neo4j caiu), `/info`, `POST/GET /benchmarks`, `GET/DELETE /benchmarks/{id}`, `GET /benchmarks/{id}/results` (JSON Lines, 409 enquanto roda) e `POST /transactions/evaluate`. Erro sempre no formato `{"error":{"code","message","details"}}`. A tabela completa está no README.

### Dashboard (`web_dashboard/`, Python 3.12, Gradio 5.50)

Cliente fino da API: nunca lê arquivo do backend, e toda URL vem de `BACKEND_API_URL`.

- `client.py`: HTTP, `BackendError` tipado e cache dos resultados.
- `analysis.py`: métricas, ROC, PR e varredura de limiar. Recalcula tudo no limiar do slider sem chamar o modelo.
- `surrogate.py`: a árvore da Aba 4.
- `plots.py`: tema Asaas e os gráficos Plotly.
- `handlers.py`: uma função pura por aba. **Não importa Gradio**, para testar sem abrir a tela.
- `app.py`: só layout e eventos, com `api_name` fixos.

Os imports são planos (`import handlers`), e o `pythonpath` está no `pyproject.toml`. Os testes usam `tests/fakes.py` (`FakeClient`) e `tests/conftest.py` (`make_results`, `make_summary`).

Visual: execução A é `#1e55ff` e B é `#e86835` (par validado), paleta `ASAAS_COLORS` sem gerar cor nova, `#FF365E` só em linha de referência, sem eixo duplo e tema `Soft` forçado em claro.

## Convenções

- Textos, comentários, mensagens de erro, logs e commits em **português informal, sem emoji, sem travessão e sem ponto e vírgula em texto**. Identificadores em inglês, exceto os rótulos mostrados ao usuário.
- Teste primeiro: escreva o teste, veja falhar e só então implemente. Nomes de teste em português.
- Commits com prefixo (`feat:`, `fix:`, `docs:`) e a linha `Co-Authored-By` exigida pela sessão.
- Go: `gofmt` e `go vet` com e sem `-tags=integration`. Python: ruff com linha de 100 (`make lint-py`).

## Armadilhas

- O build das imagens usa `network: host`, porque o DNS IPv6 do host não tem rota dentro do container de build. Não remova.
- O Ollama roda no host e precisa escutar em `0.0.0.0:11434` para o container chegar nele (`host.docker.internal`).
- Ao trocar de dataset, rode `docker compose down -v` antes do `make seed`. Sem isso o seeder acha que é a mesma carga e tenta retomar.
- A pasta `data/` precisa existir antes do `docker compose up`, senão o Docker a cria como root.
- As receitas do `Makefile` usam TAB.
- O `.env` fica fora do git. No compose as portas só abrem em `127.0.0.1`.
- Os testes de integração têm a tag `integration` e ficam fora do `go test ./...` comum.
- O dashboard recarrega uma vez ao abrir para fixar o tema claro, então um clique nessa primeira fração de segundo se perde.
