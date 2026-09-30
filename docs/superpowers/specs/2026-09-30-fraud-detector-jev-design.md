# Detector de fraude com Go, Neo4j e modelo de decisão local

Data: 2026-09-30
Status: rascunho para revisão
Pasta do projeto: `jev_golang_antifraude` (módulo Go `fraud-detector-jev`)

## 1. Objetivo

Projeto de estudo que mede, de forma honesta, se features de grafo do Neo4j melhoram a decisão de um modelo de classificação rápida (estilo "System One") sobre transações do dataset PaySim. O modelo roda local, no Ollama.

Sucesso significa que você consegue:

1. Subir tudo com Docker, carregar o PaySim no Neo4j e rodar um benchmark.
2. Ver no Gradio o comparativo com e sem grafo (qualidade e velocidade) sobre as mesmas transações.
3. Simular uma transação pontual e inspecionar o subgrafo dela.
4. Ver perfis transacionais que explicam as decisões do modelo.

Fora do objetivo: produção, tempo real, treinar modelo próprio, usar a Jev da TypeSafe na nuvem.

## 2. Aviso sobre o modelo

O modelo usado **não é a Jev da TypeSafe**. Segundo o post do Ollama, são modelos abertos que falam o mesmo protocolo dela:

- `nimble`: 9B, Bespoke Labs, Apache 2.0, 9,5 GB.
- `tev1:4b`: 4B, Together AI, MIT, 4,5 GB.
- `tev1:0.8b`: 0,8B, Together AI, 812 MB.

Os resultados medem esses modelos. O README e o dashboard rotulam como "modelo aberto estilo Jev rodando no Ollama". Toda execução grava qual modelo foi usado.

## 3. Fatos verificados e premissas a confirmar

**Verificado na documentação em 2026-09-30:**

- Rota `POST /v1/systemone`, no Ollama local em `http://localhost:11434`, igual à da API da TypeSafe.
- Pergunta `noul`: `{"type":"noul","instructions":"...","criteria":{"true":"...","false":"..."}}`, com `criteria` opcional.
- Resposta `noul`: `{"type":"noul","noul":0.0 a 1.0}`. Não existem os campos `value`, `probability`, `confidence` nem `elapsedMs`. O contrato colado no prompt original estava diferente disso.
- O `tev1` exige Ollama 0.35 ou mais novo.
- Neo4j 5 usa `server.memory.heap.initial_size`, `server.memory.heap.max_size` e `server.memory.pagecache.size`. Os nomes `dbms.memory.*` do compose original são do 4.x.

**Premissas a confirmar durante a implementação:**

- O PaySim tem cerca de 6,36 milhões de linhas e cerca de 8,2 mil fraudes (0,13%), e não 10 milhões.
- O CSV está ordenado por `step`. O seeder confere e para se não estiver.
- Só existe fraude nos tipos `TRANSFER` e `CASH_OUT`. O sampler confere na carga.
- O Ollama aceita `state` como string JSON no `/v1/systemone`. Os exemplos da doc usam string e objeto.
- Algo perto de 9 milhões de contas e base de 2 a 3 GB no disco.
- O tempo de carga (chute de 20 a 60 minutos) e o desempenho das queries de ciclo.
- A imagem `neo4j:5.26-community` funciona (foi trocada da 5.20 ao planejar: é a última LTS da linha 5.x e já estava baixada na máquina).
- A velocidade dos modelos na RTX 3050 de 6 GB. Hoje o comando `nvidia-smi` não existe na máquina, então o driver provavelmente não está instalado e o Ollama rodaria na CPU.

## 4. Decisões fechadas

| Tema | Decisão | Motivo |
| --- | --- | --- |
| Modelo | Ollama local no host, padrão `tev1:4b`, `nimble` para números finais, `tev1:0.8b` para teste rápido | Sem custo, sem limite de requisições, `tev1:4b` cabe nos 6 GB de VRAM |
| Contrato | Igual à API oficial da TypeSafe, com `noul` como número único | É o que o Ollama expõe |
| Dataset | PaySim original inteiro no Neo4j | Comparável com outros estudos, grafo completo dá histórico real |
| Amostra | Estratificada, % de fraude configurável (padrão 10%), seed fixa | Com 0,13% de fraude natural, uma amostra aleatória pequena traz poucos casos |
| Features | Cypher ao vivo por transação (abordagem A) | Mede a latência real do Neo4j e é o que o projeto propõe |
| Aba 4 | Árvore de decisão substituta | Explica a decisão de uma caixa preta com regras legíveis |
| Ollama | Fora do compose, no host | Usa driver e GPU do host sem configuração extra |
| Go | 1.27.1, instalado em `~/.local/go` | Testes rodam direto no host |
| Idioma | Comentários, logs, erros, telas, README e commits em português informal, sem emoji e sem travessão. Identificadores e campos JSON em inglês | Segue o esqueleto de pastas e o contrato da API |

## 5. Arquitetura e fluxo

Quatro peças: **Neo4j** (container), **Ollama** (host), **fraud-api** em Go (container) e **dashboard** Gradio (container).

- **Go:** ingestão, grafo, benchmark e métricas consolidadas.
- **Python:** análise e visualização.
- **Comunicação:** só por REST, então o dashboard funciona dentro ou fora do Docker.

Fluxo de um benchmark:

1. O dashboard chama `POST /benchmarks`.
2. O Go faz uma passada de streaming no CSV e monta a amostra estratificada com a seed. Mesma seed significa mesmas transações.
3. Cada transação passa pelo pool do Neo4j, que calcula as features usando só o passado. O Go monta o `state` e entrega ao cliente do modelo.
4. Cada resultado vai para `runs/<id>/results.jsonl` e o resumo para `summary.json`.
5. O dashboard busca os dois por REST. ROC, PR, varredura de limiar e a árvore da Aba 4 são calculados em Python sobre os scores, sem chamar o modelo de novo.

**Com grafo contra sem grafo** são duas execuções com a mesma seed. A comparação usa só as transações presentes nas duas.

### Estrutura de pastas

```text
jev_golang_antifraude/
├── cmd/
│   ├── api/main.go
│   └── seeder/seed_graph.go
├── internal/
│   ├── domain/            transaction.go, metrics.go
│   ├── infrastructure/
│   │   ├── csv/           streaming_reader.go
│   │   ├── neo4j/         client.go, queries.go, features_queries.go, features.go, loader.go
│   │   ├── jev/           client.go, models.go, jevtest/mock_server.go (só para testes)
│   │   ├── runstore/      runstore.go (grava e lê execuções)
│   │   └── web/           router.go
│   ├── service/           enricher.go, features.go, classifier.go, sampler.go, benchmark_pipeline.go,
│   │                      run_manager.go, params.go, evaluator.go
│   └── config/            config.go
├── web_dashboard/         app.py, client.py, analysis.py, surrogate.py, plots.py, handlers.py,
│                          requirements.txt, Dockerfile, tests/
├── data/                  transactions.csv (fora do git)
├── docs/superpowers/specs/
├── docker-compose.yml
├── Dockerfile.api
├── config/                instructions.txt (a pergunta ao modelo, editável)
├── scripts/               test-integration.sh, smoke_benchmark.sh
├── testdata/              metrics_golden.json, mini_paysim.csv
├── Makefile
├── .env.example
├── go.mod
└── README.md
```

Responsabilidades em `service`:

- `features.go` define as interfaces `GraphFeatureSource` e `SubgraphSource`. Os tipos das features (`GraphFeatures`, `FeatureQuery`, `Subgraph`) ficam em `internal/domain`, para evitar import circular.
- `enricher.go` monta o `state` a partir da transação e das features.
- `sampler.go` faz a amostra estratificada.
- `benchmark_pipeline.go` orquestra os estágios e as métricas.

A interface `Classifier` fica em `service`, no lado de quem consome, e o `jev.Client` implementa.

## 6. Grafo e features

### Modelo

- Nó `(:Account {id})` com constraint de unicidade.
- Aresta `(:Account)-[:TRANSFERRED {seq, step, type, amount, isFraud}]->(:Account)`. A transação é aresta, não nó.
- `seq` é o número da linha no CSV (base 0). É a régua do tempo. Toda query filtra `t.seq < $seq`, o que exclui a própria transação e o futuro. Isso assume que a ordem das linhas dentro de um mesmo `step` é a ordem de chegada.
- `isFraud` fica na aresta só para análise e para pintar o subgrafo na Aba 3. **Nenhuma query de feature lê esse campo.**
- O `isFlagged` do nó do prompt original foi removido, porque seria preenchido com o gabarito. Se a intenção era a coluna `isFlaggedFraud` do CSV, ela entra como campo tabular.
- Restrições e índices (Neo4j 5):

```text
CREATE CONSTRAINT account_id IF NOT EXISTS FOR (a:Account) REQUIRE a.id IS UNIQUE
CREATE INDEX transferred_seq IF NOT EXISTS FOR ()-[t:TRANSFERRED]-() ON (t.seq)
CREATE INDEX transferred_step IF NOT EXISTS FOR ()-[t:TRANSFERRED]-() ON (t.step)
```

### Features de grafo

Todas usam só arestas com `seq` menor que o da transação. A janela tem K horas (padrão 24, configurável). É uma query por transação, para medir a latência do Neo4j como um número único.

| Feature | Definição |
| --- | --- |
| `first_transfer_between_pair` | A origem nunca enviou para esse destino antes |
| `dest_in_degree` | Remetentes distintos que já enviaram ao destino, em todo o passado |
| `dest_fan_in_window` | Remetentes distintos ao destino nas últimas K horas |
| `dest_fan_out_window` | Destinatários distintos para quem o destino enviou nas últimas K horas |
| `orig_fan_out_window` | Destinatários distintos para quem a origem enviou nas últimas K horas |
| `dest_account_age_steps` | Horas desde a primeira aresta anterior que envolve o destino, como origem ou como destino. `null` se a conta nunca apareceu antes |
| `cycle_detected` | Existe caminho do destino de volta até a origem em até `cycle_max_hops` saltos (padrão 3), todas as arestas do caminho com `seq` menor que o da transação |

Cada query tem timeout (padrão 2 segundos). O pool do driver tem o tamanho do número de workers.

### Campos tabulares no `state`

`transaction_id` (formato `TX_<seq>`), `type`, `amount`, `time_step`, `orig_balance_before`, `orig_balance_after`, `dest_balance_before`, `dest_balance_after`, `balance_drop` (saldo anterior menos saldo posterior da origem) e `dest_is_merchant` (destino começa com "M"). Os nomes das contas não vão no `state`.

### Exemplo de `state`

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

Na execução sem grafo, o bloco `graph_features` não existe.

### O que saiu do payload original e por quê

- `layering_risk_score` e `dest_is_new_hub` saíram. Seriam fórmulas e limites inventados por nós, e escondem os sinais brutos. O modelo recebe os números crus.

### Proteções contra vazamento de dados

- Teste que lê o texto das queries Cypher e falha se achar `isFraud` ou `isFlagged`.
- Teste de "só o passado": num grafo pequeno, as features de uma transação no grafo completo precisam ser iguais às do grafo cortado em `seq - 1`.

## 7. Pipeline do benchmark

### Amostra

- Uma passada de streaming no CSV, sorteando por classe com a seed. Com N e a % de fraude, calcula quantas fraudes e quantas legítimas entram.
- Se pedirem mais fraudes do que existem no universo elegível, usa todas e avisa no resumo.
- Por padrão o universo elegível é `TRANSFER` e `CASH_OUT`, controlado pelo parâmetro `types`. Os outros tipos seriam legítimos fáceis e diluiriam a taxa de falso positivo.
- Na mesma passada conta linhas e fraudes por tipo. Disso sai a prevalência do universo elegível, usada na precisão reprojetada.

### Estágios

`amostra` para `pool Neo4j` para `chamador do modelo` para `coletor`, ligados por channels com buffer limitado.

- Com grafo desligado, o estágio do Neo4j é pulado.
- O parâmetro `workers` controla o pool do Neo4j. O chamador do modelo tem `max_inflight` próprio.
- Antes de medir, 3 chamadas de aquecimento ao modelo e, nas execuções com grafo, uma consulta falsa por worker ao Neo4j (em paralelo) não entram na conta. O aquecimento do Neo4j abre o pool de conexões e compila o plano da query, o que num teste ponta a ponta inflava o P50 para cerca de 600 ms. Se todo o aquecimento de um dos dois falhar, a execução nem começa.
- A instrução da pergunta `is_fraud` é **idêntica** nas duas variantes, para que a única diferença seja a presença de `graph_features`. Texto padrão: "Determine se esta transação tem perfil típico de fraude bancária, esvaziamento ilícito de conta ou lavagem de dinheiro, com base em todos os dados fornecidos." O texto fica em arquivo editável.

### Medição

- `neo4j_ms` é o tempo da query Cypher. `jev_ms` é o tempo da requisição HTTP bem-sucedida. Nenhum inclui espera em fila ou limitador.
- TPS é o total de transações concluídas dividido pelo tempo total da execução.
- Percentis P50, P95 e P99 calculados de forma exata, ordenando todos os tempos.

### Erros

- Erro de rede e 5xx no modelo: retry com backoff exponencial e jitter, até 3 tentativas.
- Timeout do Neo4j, 4xx do modelo ou tentativas esgotadas: a transação fica marcada com `error` e fora das métricas. O resumo mostra quantas foram.
- A comparação entre execuções usa só as transações presentes nas duas.

### Execução

- `POST /benchmarks` responde na hora com o `run_id` e roda em segundo plano.
- Só uma execução por vez. Uma segunda recebe 409, porque duas brigariam pelo mesmo modelo e pelo mesmo Neo4j e estragariam as medições.
- Cancelamento por `DELETE` ou por sinal de encerramento. O que já foi gravado é preservado e a execução fica com status `canceled`.

### Métricas

O Go calcula, no limiar da execução: TP, FP, TN, FN, acurácia, precisão, recall, F1, taxa de falso positivo, latências e TPS.

A **precisão reprojetada** para a prevalência real usa `TPR * pi / (TPR * pi + FPR * (1 - pi))`, com `pi` sendo a prevalência do universo elegível. TPR e FPR não mudam com a estratificação.

O Python recalcula as métricas de classificação quando o limiar do slider muda, com as mesmas fórmulas, cobertas por um teste com casos compartilhados.

### Saídas

`runs/<id>/results.jsonl`, uma linha por transação:

- Identificação: `transaction_id`, `seq`, `step`, `type`, `orig`, `dest`.
- Gabarito e decisão: `is_fraud`, `noul`.
- Medição: `neo4j_ms` (`null` sem grafo), `jev_ms`, `attempts`, `input_tokens`, `error`.
- Features tabulares e de grafo, achatadas. Os campos de grafo são `null` sem grafo.

`runs/<id>/summary.json`:

- `run_id`, `status`, `started_at`, `finished_at`, `duration_s`.
- `params`: n, fraud_pct, seed, workers, model, base_url, graph, types, window_hours, cycle_max_hops, max_inflight, threshold.
- `sample`: tamanho pedido e real da amostra, fraudes e legítimas, se faltou fraude, e o universo elegível (`eligible_rows`, `eligible_fraud`) com a prevalência usada. Não existe um bloco `dataset` separado.
- `counts`: tp, fp, tn, fn, errors.
- `metrics`: accuracy, precision, recall, f1, fpr, precision_at_prevalence.
- `latency`: p50, p95 e p99 de `neo4j_ms` e de `jev_ms`.
- `tps` e `tokens_total`.

## 8. Cliente do modelo e API REST

### Configuração (variáveis de ambiente)

| Variável | Padrão | Uso |
| --- | --- | --- |
| `JEV_BASE_URL` | `http://host.docker.internal:11434` | Onde está o Ollama |
| `JEV_MODEL` | `tev1:4b` | Modelo padrão |
| `JEV_MAX_INFLIGHT` | `4` | Chamadas simultâneas ao modelo |
| `JEV_TIMEOUT` | `30s` | Timeout por requisição |
| `JEV_INSTRUCTIONS_FILE` | vazio | Arquivo com o texto da pergunta. No compose aponta para `config/instructions.txt` |
| `NEO4J_QUERY_TIMEOUT` | `2s` | Timeout de cada query de feature |
| `NEO4J_URI`, `NEO4J_USER`, `NEO4J_PASSWORD` | ver `.env.example` | Conexão com o Neo4j |

A rota `/v1/systemone` é fixa.

### Contrato

- Request: `model`, `state` (string JSON) e `questions` com a pergunta `is_fraud` do tipo `noul`, com `instructions` e `criteria`.
- Resposta: lê `answers.is_fraud.noul` e valida que está entre 0 e 1. Guarda `usage.input_tokens` quando existir.
- Cria-se um cliente novo por execução, com os parâmetros dela. Retry e limite de simultaneidade ficam dentro do cliente.
- O mock é um servidor `httptest` que devolve score por regra simples e injeta 5xx e lentidão. Serve só para testes automáticos.

### Endpoints

| Método e rota | Função |
| --- | --- |
| `GET /healthz` | Confere Neo4j e alcance do Ollama. Responde 503 só se o Neo4j caiu. Com o Ollama fora responde 200 `degraded`, com uma dica que aponta o passo do README |
| `GET /info` | Modelo e destino em uso, modelos de decisão já baixados no Ollama (`nimble`, `tev1`), estatísticas do dataset e padrões da API |
| `POST /benchmarks` | Inicia execução. Corpo com n, fraud_pct, seed, workers, model, graph, types, window_hours, cycle_max_hops, max_inflight, threshold |
| `GET /benchmarks` | Lista execuções |
| `GET /benchmarks/{id}` | Status, progresso e resumo |
| `DELETE /benchmarks/{id}` | Cancela |
| `GET /benchmarks/{id}/results` | JSONL em streaming. Responde 409 enquanto a execução está rodando |
| `POST /transactions/evaluate` | Aba 3. Recebe origem, destino, tipo, valor, `step`, quatro saldos e `seq` opcional |

**Padrões iniciais do `POST /benchmarks`:** n=2000, fraud_pct=10, seed=42, workers=32, graph=true, types=[TRANSFER, CASH_OUT], window_hours=24, cycle_max_hops=3, max_inflight=4, threshold=0.5. Limites configuráveis para n (máximo 500 mil) e workers (máximo 256).

**`evaluate`:**

- Com `seq`, faz o replay: calcula as features só com o passado daquela transação.
- Sem `seq`, é uma transação hipotética "de agora". O cálculo considera todas as arestas com `step` menor ou igual ao `step` informado, que por padrão é o maior `step` do dataset. Nesse modo o `step` define tanto o "agora" quanto a janela de K horas.
- Devolve decisão, score, features, o `state` enviado, a resposta bruta, os tempos e um subgrafo pequeno (vizinhança de 1 salto da origem e do destino, máximo 50 arestas cada).

**Erros:** JSON com código e mensagem. 400 para pedido inválido, 404 para execução inexistente, 409 para execução em andamento e 502 quando o modelo ou o Neo4j falham no `evaluate`.

**Segurança:** sem autenticação, porque é projeto local. As portas ficam publicadas só em `127.0.0.1`.

## 9. Dashboard Gradio

**Arquivos:** `app.py` só monta a interface. `client.py` chama a API Go. `analysis.py` faz métricas, ROC/PR, varredura de limiar e tipos de fraude. `surrogate.py` faz a árvore da Aba 4. `plots.py` gera as figuras Plotly. `handlers.py` tem uma função pura por aba, testável sem abrir a tela.

**Topo:** modelo e destino em uso, números do dataset, seletores de **Execução A** (base) e **Execução B**, e um slider de **limiar de decisão** (0,5). As abas 1, 2 e 4 usam A e B. O limiar recalcula tudo sem chamar o modelo.

**Aba 1, benchmark e impacto do grafo**

- Controles: N, % de fraude, workers, K horas, modelo, seed, tipos e modo (comparar com e sem grafo, só sem, só com). No modo comparar, dispara as duas execuções em sequência com a mesma seed, com progresso e cancelar.
- Resultados lado a lado: TP, FP, TN, FN, acurácia, precisão, recall, F1 e taxa de falso positivo, com a diferença entre A e B, e a precisão reprojetada em destaque.
- Gráficos: matrizes de confusão, ROC e PR sobrepostas com AUC, F1 por limiar, barras de FP contra FN e distribuição por tipo de fraude. A curva PR é reprojetada para a prevalência real (a partir de TPR e FPR), porque a curva crua de uma amostra estratificada seria otimista.
- Avisos: erros nas execuções, interseção pequena entre A e B e modelos diferentes entre A e B.

**Aba 2, performance e latência**

- TPS, tempo total, erros e tokens de cada execução.
- Barras com P50, P95 e P99 do Neo4j e do modelo, lado a lado, e histogramas de latência com os percentis marcados.
- Vale observar se o `state` com features de grafo, por ser maior, deixa o modelo mais lento.

**Aba 3, simulador**

- Formulário: origem, destino, tipo, valor, `step` e os quatro saldos, com valores de exemplo editáveis.
- Botão "Sortear da Execução A" preenche o formulário com uma transação real da amostra e manda o `seq` dela para o replay.
- Painel de resposta: selo Legítima ou Fraude, score, cartões das features (por exemplo "Ciclo detectado: Sim" e "Conexões do destino: 42"), tempos, `state` enviado, JSON bruto e o subgrafo em Plotly com origem e destino destacados.

**Aba 4, perfil transacional**

- Alvo padrão: as decisões do modelo na Execução B. Um seletor troca para o gabarito `isFraud`.
- Árvore rasa do scikit-learn, profundidade de 2 a 5 (padrão 3).
- Fidelidade medida em 30% de dados separados. Abaixo de 80%, o dashboard avisa que a árvore explica mal o modelo e que os perfis não são confiáveis.
- Saídas: importância das features, tabela de perfis (uma folha por perfil, com regra legível, quantidade, % de "Fraude" pelo modelo e % de fraude real), gráfico com a quantidade por perfil e a árvore em texto.
- As quantidades são da amostra, e a tela deixa isso escrito, porque a amostra é estratificada.

**Geral:** Gradio 5, tema `Soft` forçado em claro, textos em português informal, sem emoji. As cores seguem o padrão visual da Asaas: a execução A é `#1e55ff` e a B é `#e86835` (o par dos slots 1 e 2 da paleta foi reprovado pelo validador de paleta, por serem dois azuis quase iguais), o cinza `#767461` marca contexto e o `#FF365E` só aparece em linha de referência. Cada gráfico tem uma vista em tabela com os mesmos dados. As abas 2 e 4 se redesenham ao abrir, porque o Plotly calcula a largura ao criar o gráfico e numa aba escondida ela é zero. Step e seq do simulador são caixas de texto que começam vazias.

## 10. Infraestrutura, carga e README

### docker-compose.yml

- **neo4j:** `neo4j:5.26-community`, memória com nomes do 5.x (heap 2G fixo, page cache 3G, ajustáveis), sem APOC (as queries são Cypher puro), senha vinda do `.env`, healthcheck com `cypher-shell`.
- **fraud-api:** imagem construída por `Dockerfile.api` (multi-stage), `extra_hosts: host.docker.internal:host-gateway`, CSV montado como somente leitura, volume `runs_data`, healthcheck em `/healthz`.
- **seeder:** mesmo binário e imagem da API, `profiles: [seed]`, executado com `docker compose run --rm seeder`.
- **Build:** os dois `build:` usam `network: host`, porque em alguns setups o DNS dentro do container de build é um IPv6 sem rota e o `go mod download` falha.
- **Instruções:** `config/instructions.txt` é montado somente leitura na API e lido por `JEV_INSTRUCTIONS_FILE`.
- **dashboard:** espera a API ficar saudável.
- Volumes: `neo4j_data`, `neo4j_logs`, `runs_data`. O `neo4j_import` e o campo `version:` saem.
- Todas as portas (7474, 7687, 8080, 7860) publicadas só em `127.0.0.1`.

### Ollama no host (Linux)

O Ollama escuta só em `127.0.0.1` por padrão, e o container não chega nele. É preciso `OLLAMA_HOST=0.0.0.0:11434` por override do systemd. O README explica e avisa para conferir o firewall em rede pública. O `/healthz` acusa quando não alcança o Ollama.

### Carga do grafo (seeder)

1. Cria a constraint de `Account.id`.
2. Passada de streaming criando os nós com MERGE, em lotes de 10 mil.
3. Segunda passada criando as arestas com CREATE, em lotes de 10 mil, em ordem de `seq`, um lote por transação, de forma sequencial (em paralelo, contas populares dariam deadlock).
4. Só no fim cria os índices de `seq` e `step` e espera ficarem prontos.
5. Valida que o total de arestas bate com as linhas do CSV e que o CSV está ordenado por `step`. Grava `dataset_stats.json` no volume `runs_data`, com totais e contagem de linhas e fraudes por tipo.

**Retomada:** se a carga cair, continua do maior `seq` já presente no banco, porque cada lote é atômico. Plano B para velocidade: `neo4j-admin import`.

### README (português informal, nesta ordem)

1. O que é, com o aviso sobre o modelo.
2. Pré-requisitos: Docker, Go, Python, Ollama 0.35 ou mais novo e driver da NVIDIA (opcional).
3. Como baixar o PaySim no Kaggle e onde colocar (`data/transactions.csv`).
4. Configurar o `.env`.
5. Subir o Neo4j e carregar os dados.
6. Instalar o Ollama, liberar o `OLLAMA_HOST` e fazer o `pull` do modelo.
7. Subir API e dashboard.
8. Rodar o benchmark, pela tela e por `curl`.
9. Como ler os resultados e as limitações.
10. Solução de problemas.

### Makefile

`make up`, `make down`, `make seed`, `make lint`, `make test-go`, `make test-integration`, `make test-py` e `make smoke`.

## 11. Testes e aceite

**Go** (sempre com `-race`):

- Unitários: sampler (estratificação, seed determinística, limite de fraudes), métricas com casos conhecidos, streaming reader (linha quebrada, cabeçalho, CSV fora de ordem) e validação da API.
- Cliente do modelo com `httptest`: sucesso, retry em 5xx, timeout, score fora de 0 a 1 e respeito ao `max_inflight`.
- Guardas contra vazamento (seção 6).
- Pipeline ponta a ponta com mock do modelo e uma fonte de features falsa: concorrência, cancelamento, tratamento de erro e arquivos de saída.
- Integração com Neo4j real, com a tag `integration`. O `make test-integration` sobe um Neo4j descartável em outra porta, com volume próprio, roda e derruba. Nunca encosta na base de verdade. Casos: par novo, ciclo A para B para C para A, fan-in dentro e fora da janela, idade da conta, e a própria transação e o futuro ficando de fora.

**Python** (`pytest`): `analysis.py` (métricas iguais às do Go com casos compartilhados, ROC e PR, precisão reprojetada, árvore com fidelidade calculada e folhas somando N) e `client.py` com respostas simuladas. A interface não tem teste automático.

**Aceite manual (definição de pronto):**

1. `make up` e `make seed` funcionam, e as contagens do Neo4j batem com o CSV.
2. Benchmark de N=200 com `tev1:0.8b` roda de ponta a ponta, com e sem grafo, sem erro.
3. Benchmark de N=2000 com `tev1:4b` gera as 4 abas coerentes, e o replay da Aba 3 reproduz o score do benchmark.
4. `make test-go`, `make test-integration` e `make test-py` passam.

## 12. Convenções

- Comentários, logs, mensagens de erro, textos de tela, README e commits em português informal, sem emoji e sem travessão.
- Identificadores (funções, tipos, variáveis, arquivos, campos JSON) em inglês.
- Go: `slog` para logs, configuração por variável de ambiente validada na subida, `context` em toda chamada de I/O, `gofmt` e `go vet` no `make lint`.
- Python: `ruff` e `pytest`.

## 13. Riscos

- **Grafo esparso no PaySim.** A maioria das contas de origem aparece poucas vezes, então ciclos e fan-in podem ser raros e o ganho do grafo pode ser pequeno. O dashboard mostra o resultado como ele for, sem prometer melhora.
- **Qualidade dos modelos locais em fraude.** Não há garantia. O benchmark existe para medir isso, e comparar `tev1:4b`, `tev1:0.8b` e `nimble` faz parte do estudo.
- **Velocidade no seu hardware.** Sem driver da NVIDIA, o Ollama roda na CPU. O `nimble` de 9,5 GB não cabe nos 6 GB de VRAM mesmo com driver. Medimos e ajustamos `max_inflight`.
- **Queries de ciclo.** Podem ser lentas em contas com muitos vizinhos. Mitigação: limite de saltos e timeout. Plano B: abordagem B (pré-calcular features em lote), o que a interface `GraphFeatureSource` permite sem refazer o resto.
- **Memória.** Neo4j (cerca de 5 GB entre heap e page cache) mais o modelo no Ollama, numa máquina com 31 GB e cerca de 14 GB livres hoje.
- **Ordem dentro do mesmo `step`.** Tratar a ordem das linhas como ordem de chegada é uma premissa de modelagem.
- **`OLLAMA_HOST=0.0.0.0`** expõe o Ollama na rede local. Documentado no README.

## 14. Fora de escopo

- Jev da TypeSafe na nuvem, chave de API e limite de requisições por segundo.
- Binário de mock separado, autenticação, execução de mais de um benchmark ao mesmo tempo.
- APOC, treino de modelo próprio, processamento em tempo real.
- Teste automático da interface Gradio.
