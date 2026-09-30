# Detector de fraude com Go, Neo4j e modelo de decisão local

Projeto de estudo que responde uma pergunta simples: **as features de grafo do Neo4j ajudam um modelo de decisão rápido a errar menos ao classificar fraude?**

O Go lê o dataset PaySim, monta uma amostra, enriquece cada transação com métricas de rede (quem já mandou dinheiro para quem, ciclos, fan-in e fan-out) e pergunta a um modelo local, rodando no Ollama, se a transação parece fraude. No fim você compara os resultados com e sem grafo, sobre as mesmas transações.

> **Aviso importante:** o modelo usado aqui **não é a Jev da TypeSafe**. São modelos abertos que falam o mesmo protocolo dela (`/v1/systemone`, pergunta do tipo `noul`): `tev1:4b` e `tev1:0.8b` da Together AI, e `nimble` da Bespoke Labs. Os números medem esses modelos. Cada execução grava qual modelo foi usado.

## Sumário

- [Início rápido](#início-rápido)
- [Como funciona](#como-funciona) e [Arquitetura](#arquitetura)
- [Pré-requisitos](#pré-requisitos)
- Passo a passo: [1. PaySim](#1-baixar-o-paysim), [2. `.env`](#2-configurar-o-env), [3. Ollama](#3-instalar-o-ollama-e-baixar-o-modelo), [4. Subir](#4-subir-o-neo4j-e-a-api), [5. Carregar o grafo](#5-carregar-o-grafo), [6. Benchmark](#6-rodar-um-benchmark), [7. Dashboard](#7-abrir-o-dashboard)
- [Como ler os resultados](#como-ler-os-resultados), [Trocar a pergunta](#trocar-a-pergunta-feita-ao-modelo) e [Limitações](#limitações-leia-antes-de-tirar-conclusão)
- Referência: [Variáveis de ambiente](#variáveis-de-ambiente) e [API em resumo](#api-em-resumo)
- [Desenvolvimento](#desenvolvimento), [Contribuindo](#contribuindo) e [Solução de problemas](#solução-de-problemas)
- [Documentação do projeto](#documentação-do-projeto) e [Licença](#licença)

## Início rápido

Com Docker, o Ollama instalado no host e o CSV do PaySim em mãos (cada passo está explicado nas seções abaixo):

```bash
mkdir -p data && mv ~/Downloads/PS_*.csv data/transactions.csv   # passo 1
cp .env.example .env                                              # passo 2: troque a NEO4J_PASSWORD
ollama pull tev1:4b                                               # passo 3: com o Ollama escutando em 0.0.0.0
make up                                                           # passo 4: Neo4j, API e dashboard
make seed                                                         # passo 5: carrega o grafo
make smoke ARGS="2000 tev1:4b false"                              # passo 6: sem grafo
make smoke ARGS="2000 tev1:4b true"                               # passo 6: com grafo
```

Depois abra o dashboard em http://127.0.0.1:7860.

## Como funciona

1. O Go faz uma passada no CSV e monta uma amostra **estratificada** (você escolhe a % de fraude).
2. Cada transação passa pelo Neo4j, que calcula as features usando **só o passado dela** (nada de espiar o futuro).
3. O Go monta o `state` e pergunta ao modelo `is_fraud` (a resposta é um número de 0 a 1, o `noul`).
4. Cada resultado vai para `runs/<id>/results.jsonl` e o resumo para `summary.json`.
5. Você roda duas vezes com a mesma seed, uma sem grafo e outra com, e compara.

## Arquitetura

```text
PaySim CSV --> seeder --> Neo4j  (contas e transferências)
                            ^
                            | features, só com o passado da transação
                            |
            +---------------+----------------+
            |   API em Go (fraud-api, :8080) |--> Ollama no host (/v1/systemone)
            +---------------+----------------+
                            |  REST                     |
                            v                           v
          dashboard Gradio (:7860)          runs/<id>/results.jsonl e summary.json
```

- **Seeder:** lê o CSV em streaming e carrega contas e transferências no Neo4j.
- **API em Go:** monta a amostra, roda o pipeline (Neo4j, modelo e coletor em paralelo) e serve a API REST. Uma execução por vez.
- **Neo4j:** calcula as features de grafo em Cypher ao vivo, sempre só com o passado da transação.
- **Ollama:** roda no host, fora do compose, e responde a pergunta `is_fraud`.
- **Dashboard:** cliente fino da API, em Python. Nunca lê arquivo do backend.

## Pré-requisitos

- Docker com Docker Compose.
- Go 1.27 ou mais novo (só para rodar os testes e o lint fora do Docker).
- Python 3.12 (só para rodar os testes e o dashboard fora do Docker).
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

Isso constrói as imagens, sobe o Neo4j, a API e o dashboard e espera todos ficarem saudáveis. Confira:

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

## 7. Abrir o dashboard

O `make up` já sobe o dashboard. Abra http://127.0.0.1:7860 no navegador. O tema é claro de propósito, e a página recarrega uma vez ao abrir para fixar esse tema (um clique dado nessa primeira fração de segundo se perde).

No topo você escolhe a **Execução A** (a base, normalmente sem grafo) e a **Execução B** (normalmente com grafo), e ajusta o **limiar de decisão**. Mexer no limiar recalcula tudo sem chamar o modelo de novo.

- **Benchmark e impacto do grafo:** o botão "Rodar" no modo "Comparar com e sem grafo" faz duas execuções com a mesma seed, primeiro sem grafo (vira a A) e depois com grafo (vira a B), e mostra o andamento. Abaixo ficam as métricas lado a lado, a matriz de confusão, falsos positivos contra falsos negativos, ROC e PR, o F1 por limiar e as fraudes por tipo. A comparação usa só as transações que aparecem nas duas execuções.
- **Performance e latência:** transações por segundo, tempo total, erros e tokens, e os percentis P50, P95 e P99 do Neo4j e do modelo em separado.
- **Simulador:** digite uma transação, ou use "Sortear da Execução A" para preencher com uma real (aí o `seq` vai junto e a consulta vira um replay, só com o passado daquela transação). O sorteio também leva o modelo e as configurações da execução de origem (com ou sem grafo, janela e saltos do ciclo), para o score bater com o do benchmark. Numa execução sem grafo o replay também sai sem features de grafo, e a tela avisa. Com `seq` e `step` vazios, é uma transação hipotética "de agora". O resultado mostra o veredito, as features de grafo, os tempos, o `state` enviado, a resposta bruta e o subgrafo.
- **Perfil transacional:** uma árvore rasa imita as decisões do modelo (ou o gabarito) e cada folha vira um perfil com regra em texto. A tela mostra a **fidelidade** da árvore e avisa quando ela explica mal o modelo. As quantidades são da amostra, não da base inteira.

Para rodar o dashboard fora do Docker (por exemplo, para desenvolver):

```bash
make setup-py
BACKEND_API_URL=http://127.0.0.1:8080 web_dashboard/.venv/bin/python web_dashboard/app.py
```

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

## Variáveis de ambiente

As do `.env` (leia o `.env.example`):

| Variável | Padrão | O que faz |
| --- | --- | --- |
| `NEO4J_PASSWORD` | sem padrão (obrigatória) | Senha do Neo4j, com pelo menos 8 caracteres |
| `JEV_MODEL` | `tev1:4b` | Modelo padrão do Ollama |
| `JEV_BASE_URL` | `http://host.docker.internal:11434` | Onde o Ollama está, visto de dentro do container |
| `JEV_MAX_INFLIGHT` | `4` | Chamadas simultâneas ao modelo (de 1 a 64) |
| `DATA_DIR` | `./data` | Pasta do CSV no host |
| `DATA_FILE` | `transactions.csv` | Nome do CSV dentro dessa pasta |

Avançadas da API (o compose não repassa estas. Para mudar, acrescente na seção `environment` do serviço `fraud-api`):

| Variável | Padrão | O que faz |
| --- | --- | --- |
| `HTTP_ADDR` | `:8080` | Endereço da API |
| `JEV_TIMEOUT` | `30s` | Tempo máximo de cada chamada ao modelo |
| `NEO4J_QUERY_TIMEOUT` | `2s` | Tempo máximo de cada consulta de features |
| `MAX_SAMPLE_SIZE` | `500000` | Maior `n` aceito em `POST /benchmarks` |
| `MAX_WORKERS` | `256` | Maior `workers` aceito em `POST /benchmarks` |

Do dashboard: `BACKEND_API_URL` (padrão `http://127.0.0.1:8080`, e no compose `http://fraud-api:8080`), `GRADIO_SERVER_NAME` (padrão `127.0.0.1`) e `GRADIO_SERVER_PORT` (padrão `7860`).

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
| `POST /transactions/evaluate` | Simulador de uma transação (com `seq` faz o replay de uma transação real, e `graph`, `window_hours` e `cycle_max_hops` opcionais repetem a configuração da execução de origem) |

Os erros vêm sempre como `{"error": {"code": "...", "message": "..."}}`.

## Desenvolvimento

```bash
make lint              # gofmt e go vet, com e sem a tag integration
make test-go           # testes unitários, com -race
make test-integration  # sobe um Neo4j descartável e testa as queries de verdade
make setup-py          # cria o ambiente Python do dashboard
make test-py           # testes do dashboard
make lint-py           # ruff no dashboard
```

Estrutura:

```text
config/                 texto da pergunta ao modelo (editável)
cmd/api, cmd/seeder     binários
internal/domain         tipos e métricas
internal/service        amostragem, pipeline, gerenciador de execuções, simulador
internal/infrastructure csv, neo4j, jev (cliente do modelo), runstore, web
testdata/               casos de teste compartilhados
web_dashboard/          dashboard Gradio (client, analysis, surrogate, plots, handlers, app)
docs/superpowers/       spec e planos
```

Os testes de integração têm a tag `integration` e limpam o grafo, então só rode com `make test-integration`, que usa um Neo4j descartável em outra porta. Nunca aponte esses testes para a base do compose.

## Contribuindo

1. Crie uma branch a partir da principal.
2. Escreva o teste primeiro, veja falhar e só então implemente.
3. Antes de abrir o pull request, rode `make lint lint-py test-go test-py test-integration`.
4. Siga o estilo do projeto: textos, comentários, mensagens e commits em português informal, sem emoji, sem travessão e sem ponto e vírgula em texto. Identificadores em inglês.
5. Commits com prefixo (`feat:`, `fix:`, `docs:`) e uma mensagem que diga o porquê.

Uma regra é inegociável: nenhuma query de feature pode ler o gabarito (`isFraud`, `isFlagged`) nem transações do futuro. Há testes que travam isso. Veja o `CLAUDE.md` para o resto das regras de arquitetura.

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
- **O dashboard abre e mostra "Não consegui falar com a API"**: a API não está no ar. Confira `docker compose ps` e `curl http://127.0.0.1:8080/healthz`. Depois de subir a API, clique em "Atualizar".
- **As abas de gráficos aparecem com a largura errada**: abra a aba de novo (elas se redesenham ao abrir). Se persistir, recarregue a página.
- **O botão "Rodar" não faz nada logo depois de abrir a página**: a página recarrega uma vez para fixar o tema claro. Clique de novo.

## Documentação do projeto

- [Spec de design](docs/superpowers/specs/2026-09-30-fraud-detector-jev-design.md): a fonte de verdade das decisões.
- [Plano do backend](docs/superpowers/plans/2026-09-30-fraud-detector-backend.md) e [plano do dashboard](docs/superpowers/plans/2026-09-30-fraud-detector-dashboard.md): o passo a passo de como foi construído, com o registro do aceite no fim de cada um.
- [CLAUDE.md](CLAUDE.md): guia para agentes de código que trabalham neste repositório.

## Licença

Ainda não definida. Antes de publicar ou aceitar contribuições externas, escolha uma licença e acrescente um arquivo `LICENSE`.
