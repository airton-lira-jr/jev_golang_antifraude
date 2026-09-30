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
