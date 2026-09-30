# Detector de fraude, dashboard Gradio: plano de implementação

> **Para quem for executar:** SUB-SKILL OBRIGATÓRIA: use superpowers:subagent-driven-development (recomendado) ou superpowers:executing-plans para implementar este plano tarefa por tarefa. Os passos usam checkbox (`- [ ]`) para acompanhamento. Ao commitar, acrescente a linha de atribuição `Co-Authored-By` exigida pela sessão.

**Objetivo:** entregar o dashboard em Gradio com as 4 abas da spec (benchmark e impacto do grafo, performance e latência, simulador com subgrafo, perfil transacional), rodando no compose e falando só com a API REST do backend.

**Arquitetura:** o dashboard é um cliente fino da API. `client.py` fala HTTP, `analysis.py` faz a análise sobre os resultados (métricas, ROC e PR, varredura de limiar, árvore substituta), `plots.py` desenha os gráficos, `handlers.py` junta tudo em funções puras por aba (testáveis sem Gradio) e `app.py` só monta a tela e liga os eventos.

**Tech Stack:** Python 3.12, Gradio 5.50, Plotly 7.1, pandas 2.3, scikit-learn 1.9, requests 2.34. Testes com pytest e responses, lint com ruff.

**Spec:** `docs/superpowers/specs/2026-09-30-fraud-detector-jev-design.md`

**Este é o plano 2 de 2.** Ele depende do plano 1 (`2026-09-30-fraud-detector-backend.md`), que entrega a API REST, o compose e o Makefile. O contrato REST usado aqui está descrito na Tarefa 13 do plano 1.

## Desvios da spec (decididos ao planejar)

- Além de `app.py`, `client.py`, `analysis.py` e `plots.py`, entram `handlers.py` (a spec já pedia a análise testável sem a tela) e `surrogate.py` (a árvore da Aba 4, separada de `analysis.py` para cada arquivo ter uma responsabilidade só).
- O tema é `Soft` como a spec pede, mas **forçado em claro** e com a paleta e as fontes do padrão visual da Asaas. Os gráficos usam a variante clara do template Asaas (a spec fala em texto sem emoji e em dashboard elegante, e o padrão visual pede que só texto, grid e fundo mudem entre claro e escuro).
- A curva PR é **reprojetada para a prevalência real** (a partir de TPR e FPR), e não a curva crua da amostra estratificada. A curva crua seria otimista e contradiz a decisão de mostrar a precisão reprojetada em destaque.
- Cada gráfico tem uma "vista em tabela" com os mesmos dados, dentro de um acordeão. É uma regra de acessibilidade do guia de visualização de dados do ambiente.

## Restrições globais

Todas as tarefas herdam estas regras, com os valores copiados da spec.

- Textos de tela, comentários, mensagens e commits em português informal, sem emoji e sem travessão. Sem ponto e vírgula em texto. Identificadores em inglês, exceto os rótulos mostrados ao usuário.
- Só a API REST: nenhuma leitura de arquivo do backend. Toda URL vem de `BACKEND_API_URL` (padrão `http://127.0.0.1:8080`).
- O dashboard **recalcula** as métricas de classificação com o limiar do slider, sem chamar o modelo de novo. As fórmulas são as mesmas do Go e são conferidas contra `testdata/metrics_golden.json`. Divisão por zero dá 0.
- A comparação entre A e B usa só as transações presentes nas duas (`transaction_id`).
- Aba 4: árvore rasa do scikit-learn, profundidade de 2 a 5 (padrão 3), fidelidade medida em 30% de dados separados, aviso abaixo de 80%. As quantidades são da amostra e a tela diz isso.
- Aba 1: matriz de confusão, ROC e PR com AUC, F1 por limiar, barras de FP contra FN e a distribuição por tipo de fraude. Aviso quando há erros, quando a interseção entre A e B é pequena e quando os modelos são diferentes.
- Gradio 5, tema `Soft`, sem emoji. Porta 7860, publicada só em `127.0.0.1`.
- Visualização: paleta `ASAAS_COLORS` sem gerar cores novas, sem eixo duplo, legenda quando há 2 ou mais séries e nenhuma quando há uma só, `#FF365E` só para linha de referência, hover em todo gráfico e texto sempre em tons de texto (nunca na cor da série).
- Cores das execuções: **A é `#1e55ff` e B é `#e86835`**. O validador de paleta reprovou o par dos slots 1 e 2 da paleta Asaas (`#1e55ff` com `#488af5`, diferença de 13,6, abaixo do piso de 15 para visão normal). O par escolhido passa em todos os testes do validador em modo claro. O cinza neutro `#767461` marca "fraudes reais" e outros contextos.

## Foco da revisão

Cinco entradas que a spec deixa implícitas e que mais provavelmente vão morder quem usar. Cada uma tem um teste na tarefa dona do código.

1. **Execução só com uma classe** (sem nenhuma fraude ou só fraude na amostra): ROC, PR e a árvore não podem quebrar. Mostram um aviso e seguem. Testes nas Tarefas 3 e 4.
2. **Execução sem grafo escolhida em A e B**, ou a mesma execução nos dois: as colunas de grafo não existem, o gráfico de Neo4j fica vazio com um aviso e a árvore usa só o que existe. Testes nas Tarefas 4 e 6.
3. **API fora do ar ou devolvendo erro** (Neo4j caído, Ollama fora, 409 por execução em andamento): a tela mostra a mensagem da API, sem estourar o Gradio. Testes nas Tarefas 2, 6 e 7.
4. **Nenhuma execução terminada ainda** (primeiro uso): a tela abre, os seletores ficam vazios e as abas mostram "rode um benchmark primeiro". Teste na Tarefa 6.
5. **Limiar no extremo** (0 ou 1): tudo é fraude ou tudo é legítimo, e as métricas usam a convenção de divisão por zero em vez de quebrar. Teste na Tarefa 3.

## Como este plano foi verificado

Antes de entregar, o código de todos os blocos foi extraído para um módulo temporário fora do projeto e executado:

- `ruff check` e `ruff format` sem nada a corrigir, e a suíte inteira do pytest (120 testes) passando. A análise é conferida contra o mesmo `testdata/metrics_golden.json` do Go, então as fórmulas são idênticas nas duas linguagens.
- O validador de paleta do ambiente foi executado nos pares de cores (veja as restrições globais).
- A tela foi aberta **num navegador real** (Chrome), ligada ao backend do plano 1 rodando no compose, com um CSV sintético de 30 mil transações no formato do PaySim e um modelo falso do Ollama. Foram percorridas as 4 abas, o botão "Rodar" (comparação com e sem grafo), o sorteio e a consulta do simulador, a mudança de limiar e a API fora do ar. O dashboard também foi construído e rodado em container pelo compose.

Olhar a tela achou problemas que nenhum teste automático acharia, todos corrigidos neste plano:

- A coluna de diferença mostrava `p,p,` no lugar de `p.p.`.
- **Step e seq do simulador vinham com `0`**, e o pedido saía com `seq: 0`, fazendo um replay da primeira transação sem ninguém pedir. Agora são caixas de texto que começam vazias.
- O campo de modelo do simulador vinha vazio (o Gradio consome o dict de atualização, e eu reaproveitava o mesmo nos dois campos).
- Os gráficos das abas 2 e 4 saíam estreitos, porque o Plotly calcula a largura com o container escondido. Essas abas agora se redesenham ao abrir.
- Legendas cobrindo títulos, barras bem mais grossas que os 24 px da regra de marca, regras da árvore repetindo o mesmo corte da feature, coluna de regra estreita, ponto decimal no lugar de vírgula, rodapé do Gradio aparecendo e mensagem crua de rede quando a API cai.

Os testes também pegaram erros de dados e de gráficos que já estão corrigidos: `showlegend` não declarado vale `None` no Plotly e o modelo "sem grafo" sintético nunca decidia fraude.

**O que ainda não foi verificado** (só dá para conferir na Tarefa 9, com os dados e o modelo reais):

- O Ollama de verdade: com latências reais os histogramas e os percentis vão parecer bem diferentes dos da verificação, em que o modelo falso respondia em menos de 1 ms.
- O PaySim inteiro: o tempo de montar as tabelas e os gráficos com 500 mil linhas de resultado (o backend limita o `n` a esse valor).
- Leitores de tela e navegadores além do Chrome.
- A imagem do dashboard tem 1,18 GB (Gradio, scikit-learn e dependências), então o primeiro `make up` demora para baixar e construir.

---

## Mapa de arquivos

| Arquivo | Responsabilidade |
| --- | --- |
| `web_dashboard/client.py` | Cliente HTTP da API, erros tipados e cache dos resultados |
| `web_dashboard/analysis.py` | Métricas, ROC e PR, varredura de limiar e tipos de fraude |
| `web_dashboard/surrogate.py` | Árvore substituta da Aba 4: perfis, regras em texto, importância e fidelidade |
| `web_dashboard/plots.py` | Tema Asaas e todos os gráficos Plotly |
| `web_dashboard/handlers.py` | Uma função pura por aba, mais a comparação em segundo plano |
| `web_dashboard/app.py` | Layout, eventos e `main()` |
| `web_dashboard/tests/` | `conftest.py` (dados sintéticos) e os testes de cada módulo |
| `web_dashboard/requirements.txt`, `requirements-dev.txt`, `pyproject.toml` | Dependências e configuração de pytest e ruff |
| `web_dashboard/Dockerfile` | Imagem do dashboard |

Modificações em arquivos do plano 1 (Tarefa 8): `docker-compose.yml` (serviço `dashboard`), `Makefile` (`setup-py` e `lint-py`) e `README.md` (seção do dashboard).

Convenção dos blocos: um bloco de código com `file=caminho` no cabeçalho é o **conteúdo completo** daquele arquivo.

---
## Tarefa 1: base do projeto Python e dados sintéticos de teste

**Arquivos:**
- Criar: `web_dashboard/requirements.txt`, `requirements-dev.txt`, `pyproject.toml`
- Criar: `web_dashboard/tests/conftest.py`
- Testar: `web_dashboard/tests/test_fixtures.py`

**Interfaces:**
- Produz (usado por todos os testes das tarefas seguintes):
  - `make_results(n=400, seed=0, graph=True, n_errors=0) -> pandas.DataFrame`, com as mesmas colunas do `results.jsonl` do backend. Com a mesma `seed`, `graph=True` e `graph=False` trazem **as mesmas transações** (mesmos ids, tipos, valores e gabarito) e só mudam a nota do modelo e as colunas de grafo.
  - `make_summary(run_id=..., graph=True, model="tev1:4b", n=400, prevalence=0.003, status="done", **extra) -> dict`, com o formato do `summary.json`.
  - As fixtures `results_graph` e `results_nograph` (mesma seed, 400 linhas).
  - `GRAPH_COLUMNS`, a lista das 7 colunas de grafo.

Os dados sintéticos têm sinal de propósito: fraudes têm mais ciclos, fan-in e valores altos. Na variante com grafo a nota do modelo usa isso, e na variante sem grafo usa só o valor. Assim os testes da árvore e das curvas têm o que achar.

- [ ] **Passo 1: criar os arquivos de dependências e a configuração**

```text file=web_dashboard/requirements.txt
gradio==5.50.0
pandas==2.3.3
numpy==2.5.3
plotly==7.1.0
scikit-learn==1.9.1
requests==2.34.2
```

```text file=web_dashboard/requirements-dev.txt
-r requirements.txt
pytest==9.1.1
responses==0.26.3
ruff==0.16.9
```

```toml file=web_dashboard/pyproject.toml
[tool.pytest.ini_options]
testpaths = ["tests"]
pythonpath = ["."]
filterwarnings = [
    # Avisos de depreciação que o próprio Gradio 5 emite ao montar componentes. O
    # requirements.txt fixa o Gradio 5.50, então eles não afetam o projeto.
    "ignore:The '.*' parameter in the Blocks constructor:DeprecationWarning",
    "ignore:Passing a tuple to 'row_count':DeprecationWarning",
]

[tool.ruff]
line-length = 100
target-version = "py312"

[tool.ruff.lint]
select = ["E", "F", "I", "B", "UP"]
# Linhas longas de texto (mensagens, strings de teste) ficam a cargo do formatador.
ignore = ["E501"]

[tool.ruff.lint.isort]
# Módulos do próprio dashboard, para o ruff ordenar os imports igual em qualquer máquina.
known-first-party = ["client", "analysis", "surrogate", "plots", "handlers", "app", "tests"]
```

- [ ] **Passo 2: criar o ambiente virtual**

Na raiz do projeto:

```bash
python3 -m venv web_dashboard/.venv
web_dashboard/.venv/bin/pip install -r web_dashboard/requirements-dev.txt
```

Esperado: instalação sem erro. (O alvo `make setup-py` da Tarefa 8 faz o mesmo.)

- [ ] **Passo 3: escrever o teste das fixtures (que falha)**

```python file=web_dashboard/tests/test_fixtures.py
import numpy as np

from tests.conftest import GRAPH_COLUMNS, make_results, make_summary


def test_mesma_seed_traz_as_mesmas_transacoes_com_e_sem_grafo():
    com = make_results(seed=3, graph=True)
    sem = make_results(seed=3, graph=False)
    assert list(com["transaction_id"]) == list(sem["transaction_id"])
    assert (com["is_fraud"] == sem["is_fraud"]).all()
    assert (com["amount"] == sem["amount"]).all()
    assert not (com["noul"] == sem["noul"]).all()


def test_colunas_de_grafo_so_existem_na_variante_com_grafo():
    com = make_results(graph=True)
    sem = make_results(graph=False)
    assert all(c in com.columns for c in GRAPH_COLUMNS)
    assert not any(c in sem.columns for c in GRAPH_COLUMNS)
    assert com["neo4j_ms"].notna().all()
    assert sem["neo4j_ms"].isna().all()


def test_erros_ficam_sem_nota_e_com_a_coluna_error():
    df = make_results(n_errors=5)
    assert df["noul"].isna().sum() == 5
    assert df["error"].notna().sum() == 5
    assert "error" not in make_results(n_errors=0).columns


def test_notas_e_fraudes_tem_sinal():
    df = make_results(n=2000, graph=True)
    assert df["noul"].between(0, 1).all()
    assert df.loc[df["is_fraud"], "noul"].mean() > df.loc[~df["is_fraud"], "noul"].mean() + 0.1
    assert 0.15 < df["is_fraud"].mean() < 0.35
    assert np.isfinite(df["jev_ms"]).all()


def test_make_summary_tem_o_formato_da_api():
    s = make_summary(graph=False, model="nimble", n=50, prevalence=0.01)
    assert (
        s["status"] == "done" and s["params"]["graph"] is False and s["params"]["model"] == "nimble"
    )
    assert s["sample"]["prevalence"] == 0.01 and s["sample"]["n"] == 50
    assert set(s["latency"]) == {"neo4j_ms", "jev_ms"}
    assert s["latency"]["neo4j_ms"]["p50"] == 0
    assert make_summary(graph=True)["latency"]["neo4j_ms"]["p50"] > 0
```

- [ ] **Passo 4: rodar e ver falhar**

Rode: `web_dashboard/.venv/bin/python -m pytest web_dashboard/tests/test_fixtures.py`
Esperado: FAIL com `ModuleNotFoundError` ou `ImportError` (o `conftest.py` ainda não existe).

- [ ] **Passo 5: implementar as fixtures**

```python file=web_dashboard/tests/__init__.py
```

```python file=web_dashboard/tests/conftest.py
"""Dados sintéticos com o mesmo formato do results.jsonl e do summary.json do backend."""

from __future__ import annotations

import numpy as np
import pandas as pd
import pytest

GRAPH_COLUMNS = [
    "first_transfer_between_pair",
    "dest_in_degree",
    "dest_fan_in_window",
    "dest_fan_out_window",
    "orig_fan_out_window",
    "dest_account_age_steps",
    "cycle_detected",
]


def _sigmoid(x):
    return 1 / (1 + np.exp(-x))


def make_results(
    n: int = 400, seed: int = 0, graph: bool = True, n_errors: int = 0
) -> pd.DataFrame:
    """Gera n transações. Com a mesma seed, graph=True e graph=False trazem as mesmas
    transações e só mudam a nota do modelo e as colunas de grafo."""
    base = np.random.default_rng(seed)
    is_fraud = base.random(n) < 0.25
    tipo = np.where(base.random(n) < 0.5, "TRANSFER", "CASH_OUT")
    amount = np.where(is_fraud, base.lognormal(11.0, 0.6, n), base.lognormal(8.5, 1.0, n))
    cycle = base.random(n) < np.where(is_fraud, 0.5, 0.03)
    fan_in = base.poisson(np.where(is_fraud, 6, 1))
    in_degree = fan_in + base.poisson(2, n)
    first_pair = base.random(n) < np.where(is_fraud, 0.9, 0.6)
    age = base.integers(0, 200, n).astype(float)
    age[base.random(n) < 0.3] = np.nan
    fan_out_dest = base.poisson(1, n)
    fan_out_orig = base.poisson(1, n)
    before = amount * (1 + base.random(n))
    after = before - amount

    noise = np.random.default_rng(seed + 1000 + int(graph))
    if graph:
        score = _sigmoid(-3.5 + 3.0 * cycle + 0.35 * fan_in + 1.2 * (amount > 50000))
    else:
        score = _sigmoid(-2.0 + 3.0 * (amount > 50000))
    noul = np.clip(score + noise.normal(0, 0.05, n), 0, 1)

    df = pd.DataFrame(
        {
            "transaction_id": [f"TX_{i}" for i in range(n)],
            "seq": np.arange(n),
            "step": np.arange(n) // 10,
            "type": tipo,
            "orig": [f"C{i}" for i in range(n)],
            "dest": [f"D{i % 50}" for i in range(n)],
            "is_fraud": is_fraud,
            "noul": noul,
            "neo4j_ms": noise.lognormal(2.5, 0.4, n) if graph else np.full(n, np.nan),
            "jev_ms": noise.lognormal(1.5, 0.5, n),
            "attempts": 1,
            "input_tokens": 100 + (150 if graph else 0),
            "amount": amount,
            "orig_balance_before": before,
            "orig_balance_after": after,
            "dest_balance_before": 0.0,
            "dest_balance_after": amount,
            "balance_drop": amount,
            "dest_is_merchant": False,
        }
    )
    if graph:
        df["first_transfer_between_pair"] = first_pair
        df["dest_in_degree"] = in_degree
        df["dest_fan_in_window"] = fan_in
        df["dest_fan_out_window"] = fan_out_dest
        df["orig_fan_out_window"] = fan_out_orig
        df["dest_account_age_steps"] = age
        df["cycle_detected"] = cycle
    if n_errors:
        df.loc[: n_errors - 1, "noul"] = np.nan
        df["error"] = None
        df.loc[: n_errors - 1, "error"] = "modelo: conexão recusada"
    return df


def make_summary(
    run_id: str = "run_20260930_160931_2756",
    graph: bool = True,
    model: str = "tev1:4b",
    n: int = 400,
    prevalence: float = 0.003,
    status: str = "done",
    **extra,
) -> dict:
    """Um summary.json no formato da API."""
    neo4j = {"p50": 12.0, "p95": 30.0, "p99": 50.0} if graph else {"p50": 0, "p95": 0, "p99": 0}
    summary = {
        "run_id": run_id,
        "status": status,
        "started_at": "2026-09-30T16:09:31Z",
        "finished_at": "2026-09-30T16:10:31Z",
        "duration_s": 60.0,
        "params": {
            "n": n,
            "fraud_pct": 25,
            "seed": 42,
            "workers": 32,
            "model": model,
            "base_url": "http://host.docker.internal:11434",
            "graph": graph,
            "types": ["TRANSFER", "CASH_OUT"],
            "window_hours": 24,
            "cycle_max_hops": 3,
            "max_inflight": 4,
            "threshold": 0.5,
        },
        "sample": {
            "requested": n,
            "n": n,
            "n_fraud": n // 4,
            "n_legit": n - n // 4,
            "fraud_shortfall": False,
            "eligible_rows": 100000,
            "eligible_fraud": int(100000 * prevalence),
            "prevalence": prevalence,
        },
        "counts": {"tp": 80, "fp": 20, "tn": 280, "fn": 20, "errors": 0},
        "metrics": {
            "accuracy": 0.9,
            "precision": 0.8,
            "recall": 0.8,
            "f1": 0.8,
            "fpr": 0.07,
            "precision_at_prevalence": 0.03,
        },
        "latency": {"neo4j_ms": neo4j, "jev_ms": {"p50": 4.0, "p95": 9.0, "p99": 14.0}},
        "tps": 25.0,
        "tokens_total": 40000,
    }
    summary.update(extra)
    return summary


@pytest.fixture
def results_graph() -> pd.DataFrame:
    return make_results(seed=7, graph=True)


@pytest.fixture
def results_nograph() -> pd.DataFrame:
    return make_results(seed=7, graph=False)
```

- [ ] **Passo 6: rodar e ver passar**

Rode: `web_dashboard/.venv/bin/python -m pytest web_dashboard/tests/test_fixtures.py -q`
Esperado: `5 passed`.

- [ ] **Passo 7: commitar**

```bash
git add web_dashboard/requirements.txt web_dashboard/requirements-dev.txt web_dashboard/pyproject.toml web_dashboard/tests
git commit -m "feat: adiciona base do projeto Python do dashboard e dados sintéticos de teste"
```

---

## Tarefa 2: cliente da API

**Arquivos:**
- Criar: `web_dashboard/client.py`
- Testar: `web_dashboard/tests/test_client.py`

**Interfaces:**
- Consumes: o contrato REST do plano 1 (Tarefa 13).
- Produz:
  - `client.BackendError(message, code="backend_error", status=None, details=None)`, com os atributos `code`, `status` e `details`. Erro de rede vira `code="unreachable"`. Corpo de erro que não é JSON vira `code="http_error"`.
  - `client.BackendClient(base_url, timeout=15.0, session=None, cache_size=6)` com os métodos `health()`, `info()`, `list_runs() -> list[dict]`, `get_run(run_id) -> dict`, `start_run(params: dict) -> dict`, `cancel_run(run_id) -> dict`, `results(run_id) -> pandas.DataFrame` e `evaluate(payload: dict) -> dict`.
  - `results` guarda em cache (por `run_id`) as execuções já terminadas, porque elas não mudam mais. A API responde 409 enquanto a execução roda, então uma resposta bem-sucedida já é definitiva. O DataFrame devolvido é compartilhado, então **não altere**.

- [ ] **Passo 1: escrever os testes que falham**

```python file=web_dashboard/tests/test_client.py
import json

import pandas as pd
import pytest
import requests
import responses

from client import BackendClient, BackendError

BASE = "http://api.teste:8080"


@pytest.fixture
def api():
    with responses.RequestsMock(assert_all_requests_are_fired=False) as rsps:
        yield rsps


def test_url_base_com_barra_final_e_normalizada(api):
    api.add(responses.GET, f"{BASE}/info", json={"model": "tev1:4b"})
    assert BackendClient(BASE + "/").info() == {"model": "tev1:4b"}


def test_health_e_info(api):
    api.add(responses.GET, f"{BASE}/healthz", json={"status": "ok"})
    api.add(responses.GET, f"{BASE}/info", json={"model": "tev1:4b", "ollama_ok": True})
    c = BackendClient(BASE)
    assert c.health()["status"] == "ok"
    assert c.info()["ollama_ok"] is True


def test_list_get_start_cancel(api):
    api.add(
        responses.GET,
        f"{BASE}/benchmarks",
        json={"runs": [{"run_id": "run_a"}, {"run_id": "run_b"}]},
    )
    api.add(responses.GET, f"{BASE}/benchmarks/run_a", json={"run_id": "run_a", "status": "done"})
    api.add(
        responses.POST,
        f"{BASE}/benchmarks",
        json={"run_id": "run_c", "status": "running"},
        status=202,
    )
    api.add(responses.DELETE, f"{BASE}/benchmarks/run_c", json={"canceled": True}, status=202)
    c = BackendClient(BASE)

    assert [r["run_id"] for r in c.list_runs()] == ["run_a", "run_b"]
    assert c.get_run("run_a")["status"] == "done"
    assert c.start_run({"n": 100, "graph": False})["run_id"] == "run_c"
    assert json.loads(api.calls[-1].request.body) == {"n": 100, "graph": False}
    assert c.cancel_run("run_c")["canceled"] is True


def test_evaluate_manda_o_payload(api):
    api.add(
        responses.POST, f"{BASE}/transactions/evaluate", json={"decision": "fraud", "noul": 0.9}
    )
    resp = BackendClient(BASE).evaluate({"orig": "C1", "dest": "C2", "seq": 5})
    assert resp["decision"] == "fraud"
    assert json.loads(api.calls[-1].request.body)["seq"] == 5


def test_erro_da_api_vira_backend_error_com_codigo_e_detalhes(api):
    api.add(
        responses.POST,
        f"{BASE}/benchmarks",
        status=400,
        json={
            "error": {
                "code": "validation_error",
                "message": "pedido inválido",
                "details": ["n precisa ser 1 ou mais"],
            }
        },
    )
    with pytest.raises(BackendError) as info:
        BackendClient(BASE).start_run({"n": 0})
    assert info.value.code == "validation_error"
    assert info.value.status == 400
    assert info.value.details == ["n precisa ser 1 ou mais"]
    assert "pedido inválido" in str(info.value)


def test_conflito_409_mantem_o_codigo(api):
    api.add(
        responses.POST,
        f"{BASE}/benchmarks",
        status=409,
        json={
            "error": {"code": "run_in_progress", "message": "já existe uma execução em andamento"}
        },
    )
    with pytest.raises(BackendError) as info:
        BackendClient(BASE).start_run({})
    assert info.value.code == "run_in_progress" and info.value.status == 409


def test_corpo_de_erro_que_nao_e_json(api):
    api.add(responses.GET, f"{BASE}/info", status=502, body="<html>bad gateway</html>")
    with pytest.raises(BackendError) as info:
        BackendClient(BASE).info()
    assert info.value.code == "http_error" and info.value.status == 502


def test_api_fora_do_ar_vira_unreachable(api):
    api.add(responses.GET, f"{BASE}/info", body=requests.ConnectionError("recusada"))
    with pytest.raises(BackendError) as info:
        BackendClient(BASE).info()
    assert info.value.code == "unreachable"
    assert BASE in str(info.value) and "no ar" in str(info.value)
    assert "recusada" not in str(info.value)  # o erro cru não vai para a tela
    assert isinstance(info.value.__cause__, requests.ConnectionError)  # mas fica encadeado


JSONL = (
    '{"transaction_id":"TX_0","seq":0,"type":"TRANSFER","is_fraud":true,"noul":0.9,"amount":10.5,'
    '"neo4j_ms":3.2,"jev_ms":1.1,"dest_account_age_steps":null,"cycle_detected":true}\n'
    '{"transaction_id":"TX_1","seq":1,"type":"CASH_OUT","is_fraud":false,"noul":null,"amount":2,'
    '"neo4j_ms":null,"jev_ms":null,"dest_account_age_steps":4,"cycle_detected":false,"error":"modelo: caiu"}\n'
)


def test_results_vira_dataframe_com_tipos_certos(api):
    api.add(responses.GET, f"{BASE}/benchmarks/run_a/results", body=JSONL)
    df = BackendClient(BASE).results("run_a")
    assert isinstance(df, pd.DataFrame) and len(df) == 2
    assert df["is_fraud"].dtype == bool
    assert pd.api.types.is_float_dtype(df["noul"]) and df["noul"].isna().tolist() == [False, True]
    assert pd.api.types.is_float_dtype(df["dest_account_age_steps"])
    assert df["dest_account_age_steps"].isna().tolist() == [True, False]
    assert df["error"].isna().tolist() == [True, False]


def test_results_vazio_devolve_dataframe_com_as_colunas_basicas(api):
    api.add(responses.GET, f"{BASE}/benchmarks/run_a/results", body="")
    df = BackendClient(BASE).results("run_a")
    assert df.empty and {"transaction_id", "is_fraud", "noul"} <= set(df.columns)


def test_results_usa_cache_para_execucao_terminada(api):
    api.add(responses.GET, f"{BASE}/benchmarks/run_a/results", body=JSONL)
    c = BackendClient(BASE)
    primeira = c.results("run_a")
    segunda = c.results("run_a")
    assert len(api.calls) == 1
    assert primeira is segunda


def test_cache_descarta_a_mais_antiga_quando_enche(api):
    for rid in ("run_a", "run_b", "run_c"):
        api.add(responses.GET, f"{BASE}/benchmarks/{rid}/results", body=JSONL)
    c = BackendClient(BASE, cache_size=2)
    c.results("run_a")
    c.results("run_b")
    c.results("run_c")  # empurra a run_a para fora
    c.results("run_b")  # continua em cache
    assert len(api.calls) == 3
    c.results("run_a")  # foi descartada, busca de novo
    assert len(api.calls) == 4


def test_results_de_execucao_em_andamento_da_409_e_nao_entra_no_cache(api):
    api.add(
        responses.GET,
        f"{BASE}/benchmarks/run_a/results",
        status=409,
        json={"error": {"code": "run_in_progress", "message": "só depois de terminar"}},
    )
    c = BackendClient(BASE)
    with pytest.raises(BackendError) as info:
        c.results("run_a")
    assert info.value.code == "run_in_progress"
    with pytest.raises(BackendError):
        c.results("run_a")
    assert len(api.calls) == 2
```

- [ ] **Passo 2: rodar e ver falhar**

Rode: `web_dashboard/.venv/bin/python -m pytest web_dashboard/tests/test_client.py -q`
Esperado: FAIL com `ModuleNotFoundError: No module named 'client'`.

- [ ] **Passo 3: implementar**

```python file=web_dashboard/client.py
"""Cliente HTTP da API do backend. É a única parte do dashboard que faz rede."""

from __future__ import annotations

import json
from collections import OrderedDict

import pandas as pd
import requests

NUMERIC_COLUMNS = [
    "seq",
    "step",
    "noul",
    "neo4j_ms",
    "jev_ms",
    "attempts",
    "input_tokens",
    "amount",
    "orig_balance_before",
    "orig_balance_after",
    "dest_balance_before",
    "dest_balance_after",
    "balance_drop",
    "dest_in_degree",
    "dest_fan_in_window",
    "dest_fan_out_window",
    "orig_fan_out_window",
    "dest_account_age_steps",
]
BASIC_COLUMNS = ["transaction_id", "seq", "type", "is_fraud", "noul"]


class BackendError(Exception):
    """Erro devolvido pela API (ou falha de rede). O texto da exceção é a mensagem
    pronta para mostrar ao usuário."""

    def __init__(
        self,
        message: str,
        code: str = "backend_error",
        status: int | None = None,
        details: list[str] | None = None,
    ):
        super().__init__(message)
        self.code = code
        self.status = status
        self.details = details or []


class BackendClient:
    def __init__(
        self,
        base_url: str,
        timeout: float = 15.0,
        session: requests.Session | None = None,
        cache_size: int = 6,
    ):
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout
        self._session = session or requests.Session()
        self._cache: OrderedDict[str, pd.DataFrame] = OrderedDict()
        self._cache_size = cache_size

    def _request(
        self,
        method: str,
        path: str,
        *,
        body: dict | None = None,
        timeout: float | None = None,
        stream: bool = False,
    ) -> requests.Response:
        try:
            resp = self._session.request(
                method,
                f"{self.base_url}{path}",
                json=body,
                timeout=timeout or self.timeout,
                stream=stream,
            )
        except requests.RequestException as exc:
            # O erro original fica encadeado (__cause__), mas não vai para a tela: o texto cru
            # do urllib3 só confunde quem está usando o dashboard.
            raise BackendError(
                f"Não consegui falar com a API em {self.base_url}. Ela está no ar? "
                "Confira com `docker compose ps`.",
                code="unreachable",
            ) from exc
        if resp.status_code >= 400:
            raise self._to_error(resp)
        return resp

    @staticmethod
    def _to_error(resp: requests.Response) -> BackendError:
        try:
            err = resp.json()["error"]
            return BackendError(
                err.get("message", "erro sem mensagem"),
                err.get("code", "backend_error"),
                resp.status_code,
                err.get("details") or [],
            )
        except (ValueError, KeyError, TypeError, AttributeError):
            return BackendError(
                f"A API respondeu HTTP {resp.status_code}", "http_error", resp.status_code
            )

    def health(self) -> dict:
        return self._request("GET", "/healthz").json()

    def info(self) -> dict:
        return self._request("GET", "/info").json()

    def list_runs(self) -> list[dict]:
        return self._request("GET", "/benchmarks").json()["runs"]

    def get_run(self, run_id: str) -> dict:
        return self._request("GET", f"/benchmarks/{run_id}").json()

    def start_run(self, params: dict) -> dict:
        return self._request("POST", "/benchmarks", body=params).json()

    def cancel_run(self, run_id: str) -> dict:
        return self._request("DELETE", f"/benchmarks/{run_id}").json()

    def evaluate(self, payload: dict) -> dict:
        return self._request("POST", "/transactions/evaluate", body=payload, timeout=60).json()

    def results(self, run_id: str) -> pd.DataFrame:
        """Resultados de uma execução terminada. O DataFrame é compartilhado com o
        cache, então quem chama não deve alterá-lo."""
        if run_id in self._cache:
            self._cache.move_to_end(run_id)
            return self._cache[run_id]

        resp = self._request("GET", f"/benchmarks/{run_id}/results", timeout=120, stream=True)
        rows = [json.loads(line) for line in resp.iter_lines() if line]
        df = pd.DataFrame(rows) if rows else pd.DataFrame(columns=BASIC_COLUMNS)
        df = _coerce(df)

        self._cache[run_id] = df
        while len(self._cache) > self._cache_size:
            self._cache.popitem(last=False)
        return df


def _coerce(df: pd.DataFrame) -> pd.DataFrame:
    for col in NUMERIC_COLUMNS:
        if col in df.columns:
            df[col] = pd.to_numeric(df[col], errors="coerce")
    if "is_fraud" in df.columns:
        df["is_fraud"] = df["is_fraud"].astype(bool)
    return df
```

- [ ] **Passo 4: rodar e ver passar**

Rode: `web_dashboard/.venv/bin/python -m pytest web_dashboard/tests/test_client.py -q`
Esperado: todos passam.

- [ ] **Passo 5: commitar**

```bash
git add web_dashboard/client.py web_dashboard/tests/test_client.py
git commit -m "feat: adiciona cliente da API do backend com erros tipados e cache"
```

---

## Tarefa 3: análise das métricas

**Arquivos:**
- Criar: `web_dashboard/analysis.py`
- Testar: `web_dashboard/tests/test_analysis.py`

**Interfaces:**
- Consumes: DataFrames no formato do `results.jsonl` (`make_results`) e o `testdata/metrics_golden.json`.
- Produz (todas as funções são puras e não alteram a entrada):
  - `analysis.scored(df) -> DataFrame`: só as linhas em que o modelo respondeu (`noul` não nulo).
  - `analysis.align(df_a, df_b) -> (DataFrame, DataFrame)`: só as transações presentes nas duas, ordenadas por `seq`, com o mesmo índice.
  - `analysis.confusion(df, threshold) -> {"tp", "fp", "tn", "fn"}` (fraude é a classe positiva, `noul >= threshold` é fraude).
  - `analysis.metrics(conf, prevalence) -> {"accuracy", "precision", "recall", "f1", "fpr", "precision_at_prevalence"}`. As mesmas fórmulas do Go, divisão por zero dá 0.
  - `analysis.roc_pr(df, prevalence) -> dict | None`: `fpr`, `tpr`, `auc_roc`, `recall`, `precision` (reprojetada), `auc_pr`. Devolve `None` se só existir uma classe.
  - `analysis.threshold_sweep(df, prevalence, steps=101) -> DataFrame` com `limiar`, `precision`, `recall`, `f1`, `fpr`, `precision_at_prevalence`, `fp`, `fn`.
  - `analysis.fraud_types(df, threshold) -> DataFrame` com `tipo`, `fraudes`, `detectadas`.
  - `analysis.prevalence_of(summary, df=None) -> float`: a prevalência do resumo, ou a da própria amostra se o resumo não tiver.

- [ ] **Passo 1: escrever os testes que falham**

```python file=web_dashboard/tests/test_analysis.py
import json
import math
from pathlib import Path

import numpy as np
import pandas as pd
import pytest

import analysis
from tests.conftest import make_results, make_summary

GOLDEN = Path(__file__).resolve().parents[2] / "testdata" / "metrics_golden.json"


def test_metricas_batem_com_o_arquivo_compartilhado_com_o_go():
    cases = json.loads(GOLDEN.read_text(encoding="utf-8"))["cases"]
    assert cases, "arquivo sem casos"
    for c in cases:
        got = analysis.metrics({k: c[k] for k in ("tp", "fp", "tn", "fn")}, c["prevalence"])
        for nome, esperado in c["expected"].items():
            assert math.isclose(got[nome], esperado, rel_tol=1e-9, abs_tol=1e-12), (c["name"], nome)


def _pequeno():
    # previsto (limiar 0.5): fraude, legítima, fraude, legítima
    return pd.DataFrame(
        {
            "transaction_id": ["a", "b", "c", "d"],
            "seq": [0, 1, 2, 3],
            "noul": [0.9, 0.2, 0.6, 0.4],
            "is_fraud": [True, False, False, True],
        }
    )


def test_confusion_no_limiar():
    assert analysis.confusion(_pequeno(), 0.5) == {"tp": 1, "fp": 1, "tn": 1, "fn": 1}


def test_limiar_no_valor_exato_conta_como_fraude():
    assert analysis.confusion(_pequeno(), 0.6)["fp"] == 1
    assert analysis.confusion(_pequeno(), 0.61)["fp"] == 0


def test_limiares_extremos_nao_quebram():
    tudo_fraude = analysis.confusion(_pequeno(), 0.0)
    assert tudo_fraude == {"tp": 2, "fp": 2, "tn": 0, "fn": 0}
    m = analysis.metrics(tudo_fraude, 0.003)
    assert m["recall"] == 1.0 and m["fpr"] == 1.0
    nada = analysis.confusion(_pequeno(), 1.0)
    assert nada == {"tp": 0, "fp": 0, "tn": 2, "fn": 2}
    m = analysis.metrics(nada, 0.003)
    assert m["precision"] == 0.0 and m["recall"] == 0.0 and m["f1"] == 0.0
    assert m["precision_at_prevalence"] == 0.0


def test_scored_tira_quem_deu_erro():
    df = make_results(n_errors=7)
    assert len(analysis.scored(df)) == len(df) - 7
    assert analysis.scored(pd.DataFrame()).empty


def test_align_fica_so_com_as_transacoes_das_duas():
    a = make_results(seed=1, graph=False, n_errors=5)  # TX_0 a TX_4 deram erro
    b = make_results(seed=1, graph=True).iloc[:350]  # só TX_0 a TX_349
    aa, bb = analysis.align(a, b)
    assert len(aa) == len(bb) == 345  # TX_5 a TX_349
    assert (aa["transaction_id"].to_numpy() == bb["transaction_id"].to_numpy()).all()
    assert aa["seq"].is_monotonic_increasing
    assert list(aa.index) == list(range(len(aa)))


def test_roc_pr_classificador_perfeito():
    df = pd.DataFrame({"noul": [0.9, 0.8, 0.2, 0.1], "is_fraud": [True, True, False, False]})
    r = analysis.roc_pr(df, 0.003)
    assert r["auc_roc"] == pytest.approx(1.0)
    assert r["auc_pr"] == pytest.approx(1.0)


def test_roc_pr_de_uma_classe_so_devolve_none():
    so_legitimas = pd.DataFrame({"noul": [0.1, 0.2], "is_fraud": [False, False]})
    so_fraudes = pd.DataFrame({"noul": [0.9, 0.8], "is_fraud": [True, True]})
    assert analysis.roc_pr(so_legitimas, 0.003) is None
    assert analysis.roc_pr(so_fraudes, 0.003) is None
    assert analysis.roc_pr(pd.DataFrame(columns=["noul", "is_fraud"]), 0.003) is None


def test_roc_pr_tem_sinal_e_a_pr_cai_com_a_prevalencia(results_graph):
    r = analysis.roc_pr(results_graph, 0.25)
    assert 0.8 < r["auc_roc"] < 1.0
    assert np.all(np.diff(r["fpr"]) >= 0) and np.all(np.diff(r["tpr"]) >= 0)
    real = analysis.roc_pr(results_graph, 0.003)
    assert real["auc_roc"] == pytest.approx(r["auc_roc"])
    assert real["auc_pr"] < r["auc_pr"]
    assert r["precision"][0] == 1.0


def test_com_grafo_separa_melhor_que_sem_grafo(results_graph, results_nograph):
    com = analysis.roc_pr(results_graph, 0.25)["auc_roc"]
    sem = analysis.roc_pr(results_nograph, 0.25)["auc_roc"]
    assert com > sem


def test_threshold_sweep(results_graph):
    sweep = analysis.threshold_sweep(results_graph, 0.003)
    assert len(sweep) == 101
    assert sweep["limiar"].iloc[0] == 0 and sweep["limiar"].iloc[-1] == 1
    assert sweep["recall"].iloc[0] == 1.0
    assert sweep["recall"].is_monotonic_decreasing and sweep["fp"].is_monotonic_decreasing
    assert {
        "limiar",
        "precision",
        "recall",
        "f1",
        "fpr",
        "precision_at_prevalence",
        "fp",
        "fn",
    } <= set(sweep.columns)
    assert len(analysis.threshold_sweep(results_graph, 0.003, steps=11)) == 11


def test_fraud_types(results_graph):
    tabela = analysis.fraud_types(results_graph, 0.5)
    assert list(tabela.columns) == ["tipo", "fraudes", "detectadas"]
    assert set(tabela["tipo"]) <= {"TRANSFER", "CASH_OUT"}
    assert tabela["fraudes"].sum() == results_graph["is_fraud"].sum()
    assert (tabela["detectadas"] <= tabela["fraudes"]).all()
    assert tabela["detectadas"].sum() == analysis.confusion(results_graph, 0.5)["tp"]

    sem_fraude = results_graph[~results_graph["is_fraud"]]
    vazia = analysis.fraud_types(sem_fraude, 0.5)
    assert vazia.empty and list(vazia.columns) == ["tipo", "fraudes", "detectadas"]


def test_prevalence_of():
    assert analysis.prevalence_of(make_summary(prevalence=0.004)) == 0.004
    df = pd.DataFrame({"is_fraud": [True, False, False, False], "noul": [1, 0, 0, 0]})
    sem_prev = make_summary()
    sem_prev["sample"] = {}
    assert analysis.prevalence_of(sem_prev, df) == 0.25
    assert analysis.prevalence_of({}, df) == 0.25
    assert analysis.prevalence_of({}, None) == 0.0


def test_funcoes_nao_alteram_a_entrada(results_graph):
    antes = results_graph.copy(deep=True)
    analysis.confusion(results_graph, 0.5)
    analysis.roc_pr(results_graph, 0.003)
    analysis.threshold_sweep(results_graph, 0.003)
    analysis.fraud_types(results_graph, 0.5)
    analysis.align(results_graph, results_graph)
    pd.testing.assert_frame_equal(results_graph, antes)
```

- [ ] **Passo 2: rodar e ver falhar**

Rode: `web_dashboard/.venv/bin/python -m pytest web_dashboard/tests/test_analysis.py -q`
Esperado: FAIL com `ModuleNotFoundError: No module named 'analysis'`.

- [ ] **Passo 3: implementar**

```python file=web_dashboard/analysis.py
"""Análise dos resultados de um benchmark. Só funções puras sobre DataFrames.

As fórmulas de classificação são as mesmas do backend em Go (veja
testdata/metrics_golden.json): fraude é a classe positiva, noul >= limiar é
fraude e toda divisão por zero dá 0.
"""

from __future__ import annotations

import numpy as np
import pandas as pd
from sklearn.metrics import roc_curve


def scored(df: pd.DataFrame) -> pd.DataFrame:
    """Só as transações em que o modelo respondeu."""
    if df.empty or "noul" not in df.columns:
        return df.iloc[0:0]
    return df[df["noul"].notna()]


def align(df_a: pd.DataFrame, df_b: pd.DataFrame) -> tuple[pd.DataFrame, pd.DataFrame]:
    """Só as transações presentes (e respondidas) nas duas execuções, na mesma ordem."""
    a, b = scored(df_a), scored(df_b)
    comuns = set(a["transaction_id"]) & set(b["transaction_id"])
    a = a[a["transaction_id"].isin(comuns)].sort_values("seq").reset_index(drop=True)
    b = b[b["transaction_id"].isin(comuns)].sort_values("seq").reset_index(drop=True)
    return a, b


def predicted(df: pd.DataFrame, threshold: float) -> pd.Series:
    return df["noul"] >= threshold


def confusion(df: pd.DataFrame, threshold: float) -> dict[str, int]:
    pred = predicted(df, threshold).to_numpy()
    real = df["is_fraud"].astype(bool).to_numpy()
    return {
        "tp": int((pred & real).sum()),
        "fp": int((pred & ~real).sum()),
        "tn": int((~pred & ~real).sum()),
        "fn": int((~pred & real).sum()),
    }


def _ratio(num: float, den: float) -> float:
    return float(num) / float(den) if den else 0.0


def precision_at_prevalence(tpr, fpr, prevalence):
    """Precisão reprojetada para outra proporção de fraude. Aceita números ou arrays.
    Onde o denominador é zero devolve 0."""
    tpr, fpr = np.asarray(tpr, dtype=float), np.asarray(fpr, dtype=float)
    den = tpr * prevalence + fpr * (1 - prevalence)
    with np.errstate(divide="ignore", invalid="ignore"):
        return np.where(den > 0, tpr * prevalence / den, 0.0)


def metrics(conf: dict[str, int], prevalence: float) -> dict[str, float]:
    tp, fp, tn, fn = conf["tp"], conf["fp"], conf["tn"], conf["fn"]
    precision = _ratio(tp, tp + fp)
    recall = _ratio(tp, tp + fn)
    fpr = _ratio(fp, fp + tn)
    return {
        "accuracy": _ratio(tp + tn, tp + fp + tn + fn),
        "precision": precision,
        "recall": recall,
        "f1": 2 * precision * recall / (precision + recall) if precision + recall else 0.0,
        "fpr": fpr,
        "precision_at_prevalence": float(precision_at_prevalence(recall, fpr, prevalence)),
    }


def roc_pr(df: pd.DataFrame, prevalence: float) -> dict | None:
    """Curvas ROC e PR. A PR é reprojetada para a prevalência real (TPR e FPR não
    mudam com a estratificação da amostra). Devolve None se só existir uma classe."""
    if df.empty:
        return None
    y = df["is_fraud"].astype(int).to_numpy()
    if y.min() == y.max():
        return None
    fpr, tpr, _ = roc_curve(y, df["noul"].to_numpy(dtype=float))
    den = tpr * prevalence + fpr * (1 - prevalence)
    with np.errstate(divide="ignore", invalid="ignore"):
        precision = np.where(den > 0, tpr * prevalence / den, 1.0)
    return {
        "fpr": fpr,
        "tpr": tpr,
        "auc_roc": float(np.trapezoid(tpr, fpr)),
        "recall": tpr,
        "precision": precision,
        "auc_pr": float(np.trapezoid(precision, tpr)),
    }


def threshold_sweep(df: pd.DataFrame, prevalence: float, steps: int = 101) -> pd.DataFrame:
    """Métricas para cada limiar de 0 a 1, para o gráfico de F1 por limiar."""
    real = df["is_fraud"].astype(bool).to_numpy()
    score = df["noul"].to_numpy(dtype=float)
    linhas = []
    for limiar in np.linspace(0, 1, steps):
        pred = score >= limiar
        conf = {
            "tp": int((pred & real).sum()),
            "fp": int((pred & ~real).sum()),
            "tn": int((~pred & ~real).sum()),
            "fn": int((~pred & real).sum()),
        }
        m = metrics(conf, prevalence)
        linhas.append({"limiar": float(limiar), **m, "fp": conf["fp"], "fn": conf["fn"]})
    return pd.DataFrame(linhas)


def fraud_types(df: pd.DataFrame, threshold: float) -> pd.DataFrame:
    """Quantas fraudes reais existem em cada tipo e quantas o modelo detectou."""
    colunas = ["tipo", "fraudes", "detectadas"]
    if df.empty:
        return pd.DataFrame(columns=colunas)
    d = df.assign(pred=predicted(df, threshold), real=df["is_fraud"].astype(bool))
    fraudes = d[d["real"]]
    if fraudes.empty:
        return pd.DataFrame(columns=colunas)
    g = (
        fraudes.groupby("type")
        .agg(fraudes=("real", "size"), detectadas=("pred", "sum"))
        .reset_index()
    )
    g = g.rename(columns={"type": "tipo"})
    g["fraudes"] = g["fraudes"].astype(int)
    g["detectadas"] = g["detectadas"].astype(int)
    return g[colunas]


def prevalence_of(summary: dict, df: pd.DataFrame | None = None) -> float:
    """Prevalência real de fraude do universo de onde saiu a amostra. Se o resumo não
    trouxer, usa a proporção da própria amostra (sem reprojeção)."""
    prev = (summary.get("sample") or {}).get("prevalence")
    if prev is not None:
        return float(prev)
    if df is not None and not df.empty:
        return float(df["is_fraud"].astype(bool).mean())
    return 0.0
```

- [ ] **Passo 4: rodar e ver passar**

Rode: `web_dashboard/.venv/bin/python -m pytest web_dashboard/tests/test_analysis.py -q`
Esperado: todos passam.

- [ ] **Passo 5: commitar**

```bash
git add web_dashboard/analysis.py web_dashboard/tests/test_analysis.py
git commit -m "feat: adiciona análise de métricas, curvas e varredura de limiar"
```

---
## Tarefa 4: árvore substituta

**Arquivos:**
- Criar: `web_dashboard/surrogate.py`
- Testar: `web_dashboard/tests/test_surrogate.py`

**Interfaces:**
- Consumes: `analysis.scored`, `analysis.predicted` (Tarefa 3) e DataFrames no formato do `results.jsonl`.
- Produz:
  - `surrogate.label(feature) -> str`, o nome legível de uma feature (exemplo: `cycle_detected` vira `ciclo detectado`). Também vale para as colunas `type_TRANSFER` e afins.
  - `surrogate.FIDELITY_MIN = 0.8` e `surrogate.MIN_ROWS = 20`.
  - `surrogate.feature_matrix(df) -> DataFrame` (só números, sem nulos: o que faltava vira `-1`).
  - `surrogate.fit(df, target, threshold, max_depth=3, seed=0) -> Surrogate`, onde `target` é `"model"` (imita as decisões do modelo no limiar) ou `"truth"` (imita o gabarito `is_fraud`). `max_depth` vai de 2 a 5, senão `ValueError`.
  - `surrogate.Surrogate` com os campos `target`, `n`, `fidelity` (acerto em 30% de dados separados), `fidelity_train`, `reliable` (`fidelity >= 0.8`), `warning` (texto ou `None`), `tree_text`, e três DataFrames:
    - `importances`: `feature`, `rotulo`, `importancia` (só maiores que zero, da maior para a menor).
    - `profiles`: `perfil`, `regra`, `transacoes`, `pct_amostra`, `pct_fraude_modelo`, `pct_fraude_real`, `arvore_decide` (uma linha por folha, da maior para a menor, chamadas `Perfil 1`, `Perfil 2` e assim por diante).
    - `counts`: `perfil`, `legitimas`, `fraudes` (fraudes reais por perfil).

Como funciona: treina uma `DecisionTreeClassifier` rasa em 70% das transações e mede a fidelidade nos 30% restantes (`train_test_split` com seed fixa e estratificado quando dá). Cada folha da árvore vira um perfil, e as contagens dos perfis são sobre **todas** as transações da amostra. Quando a fidelidade fica abaixo de 80%, o aviso diz que a árvore explica mal o alvo. Quando só existe uma classe, ou há poucas transações, ou a árvore não achou nenhuma regra, o `Surrogate` volta com o aviso e as tabelas vazias (ou com um perfil só), sem estourar exceção.

- [ ] **Passo 1: escrever os testes que falham**

```python file=web_dashboard/tests/test_surrogate.py
import numpy as np
import pandas as pd
import pytest

import surrogate
from tests.conftest import GRAPH_COLUMNS, make_results


def test_label_traduz_features_e_tipos():
    assert surrogate.label("cycle_detected") == "ciclo detectado"
    assert surrogate.label("type_TRANSFER") == "tipo TRANSFER"
    assert surrogate.label("coluna_desconhecida") == "coluna_desconhecida"


def test_feature_matrix_com_grafo(results_graph):
    x = surrogate.feature_matrix(results_graph)
    assert set(GRAPH_COLUMNS) <= set(x.columns)
    assert {"type_TRANSFER", "type_CASH_OUT", "amount", "step"} <= set(x.columns)
    assert not x.isna().any().any()
    assert set(np.unique(x["cycle_detected"])) <= {0.0, 1.0}
    nunca_apareceu = results_graph["dest_account_age_steps"].isna()
    assert nunca_apareceu.any()
    assert (x.loc[nunca_apareceu, "dest_account_age_steps"] == -1).all()
    assert all(x[c].dtype == float for c in x.columns)


def test_feature_matrix_sem_grafo_so_tem_o_tabular(results_nograph):
    x = surrogate.feature_matrix(results_nograph)
    assert not (set(GRAPH_COLUMNS) & set(x.columns))
    assert "amount" in x.columns


def test_fit_do_modelo_com_grafo_acha_o_que_pesa(results_graph):
    s = surrogate.fit(results_graph, "model", 0.5, max_depth=3)
    assert s.target == "model" and s.n == len(results_graph)
    assert s.fidelity > 0.85 and s.reliable and s.warning is None
    top = set(s.importances["feature"].head(3))
    assert top & {"cycle_detected", "dest_fan_in_window", "amount", "balance_drop"}
    assert list(s.importances.columns) == ["feature", "rotulo", "importancia"]
    assert (s.importances["importancia"] > 0).all()
    assert s.importances["importancia"].is_monotonic_decreasing
    assert s.importances["importancia"].sum() == pytest.approx(1.0)
    assert "|---" in s.tree_text


def test_perfis_somam_a_amostra_toda(results_graph):
    s = surrogate.fit(results_graph, "model", 0.5, max_depth=3)
    p = s.profiles
    assert list(p.columns) == [
        "perfil",
        "regra",
        "transacoes",
        "pct_amostra",
        "pct_fraude_modelo",
        "pct_fraude_real",
        "arvore_decide",
    ]
    assert 2 <= len(p) <= 2**3
    assert p["transacoes"].sum() == len(results_graph)
    assert p["pct_amostra"].sum() == pytest.approx(100.0)
    assert p["transacoes"].is_monotonic_decreasing
    assert list(p["perfil"]) == [f"Perfil {i + 1}" for i in range(len(p))]
    assert (p["regra"].str.len() > 0).all()
    assert set(p["arvore_decide"]) <= {"fraude", "legítima"}
    assert (
        p["pct_fraude_modelo"].between(0, 100).all() and p["pct_fraude_real"].between(0, 100).all()
    )
    assert s.counts["fraudes"].sum() == results_graph["is_fraud"].sum()
    assert (s.counts["legitimas"] + s.counts["fraudes"]).sum() == len(results_graph)
    assert list(s.counts["perfil"]) == list(p["perfil"])


def test_profundidade_limita_o_numero_de_perfis(results_graph):
    raso = surrogate.fit(results_graph, "model", 0.5, max_depth=2)
    fundo = surrogate.fit(results_graph, "model", 0.5, max_depth=5)
    assert len(raso.profiles) <= 4
    assert len(fundo.profiles) >= len(raso.profiles)


def test_profundidade_fora_de_2_a_5_da_erro(results_graph):
    for ruim in (1, 6, 0):
        with pytest.raises(ValueError):
            surrogate.fit(results_graph, "model", 0.5, max_depth=ruim)
    with pytest.raises(ValueError):
        surrogate.fit(results_graph, "outro", 0.5)


def test_e_deterministica(results_graph):
    a = surrogate.fit(results_graph, "model", 0.5)
    b = surrogate.fit(results_graph, "model", 0.5)
    pd.testing.assert_frame_equal(a.profiles, b.profiles)
    pd.testing.assert_frame_equal(a.importances, b.importances)
    assert a.fidelity == b.fidelity and a.tree_text == b.tree_text


def test_alvo_gabarito(results_graph):
    s = surrogate.fit(results_graph, "truth", 0.5)
    assert s.target == "truth" and s.fidelity > 0.7
    assert not s.importances.empty


def test_sem_grafo_funciona_so_com_o_tabular(results_nograph):
    s = surrogate.fit(results_nograph, "model", 0.5)
    assert not s.importances.empty
    assert not (set(s.importances["feature"]) & set(GRAPH_COLUMNS))


def test_avisa_quando_a_arvore_explica_mal():
    rng = np.random.default_rng(0)
    df = make_results(n=600, seed=2)
    df["noul"] = rng.random(len(df))  # decisão sem relação nenhuma com as features
    s = surrogate.fit(df, "model", 0.5, max_depth=2)
    assert s.fidelity < surrogate.FIDELITY_MIN and not s.reliable
    assert "explica mal" in s.warning and "80%" in s.warning
    assert not s.profiles.empty  # ainda mostra os perfis, só que avisa


def test_uma_classe_so_devolve_aviso_e_tabelas_vazias():
    df = make_results(n=200)
    df["noul"] = 0.1  # o modelo nunca decide fraude
    s = surrogate.fit(df, "model", 0.5)
    assert "mesma" in s.warning
    assert s.profiles.empty and s.importances.empty and s.counts.empty
    assert s.fidelity is None and not s.reliable

    so_legitimas = make_results(n=200)
    so_legitimas["is_fraud"] = False
    assert "mesma" in surrogate.fit(so_legitimas, "truth", 0.5).warning


def test_poucas_transacoes_devolve_aviso():
    s = surrogate.fit(make_results(n=10), "model", 0.5)
    assert str(surrogate.MIN_ROWS) in s.warning and s.profiles.empty
    assert surrogate.fit(pd.DataFrame(), "model", 0.5).profiles.empty


def test_ignora_quem_deu_erro():
    df = make_results(n=300, n_errors=50)
    s = surrogate.fit(df, "model", 0.5)
    assert s.n == 250 and s.profiles["transacoes"].sum() == 250


def test_arvore_sem_nenhuma_regra_vira_um_perfil_so():
    rng = np.random.default_rng(1)
    df = make_results(n=300, seed=4, graph=False)
    for coluna in (
        "amount",
        "orig_balance_before",
        "orig_balance_after",
        "balance_drop",
        "dest_balance_after",
    ):
        df[coluna] = 1.0
    df["step"] = 0
    df["type"] = "TRANSFER"
    df["noul"] = rng.random(len(df))
    s = surrogate.fit(df, "model", 0.5)
    assert len(s.profiles) == 1 and s.profiles.loc[0, "regra"] == "todas as transações"
    assert "nenhuma regra" in s.warning
    assert s.importances.empty


def test_regras_em_texto_legivel():
    d = surrogate._describe
    assert d("cycle_detected", ">", 0.5) == "ciclo detectado: sim"
    assert d("cycle_detected", "<=", 0.5) == "ciclo detectado: não"
    assert d("type_TRANSFER", ">", 0.5) == "tipo é TRANSFER"
    assert d("type_TRANSFER", "<=", 0.5) == "tipo não é TRANSFER"
    assert d("amount", ">", 123456.7) == "valor > 123.457"
    assert d("dest_fan_in_window", "<=", 2.5) == "remetentes do destino na janela <= 2,50"
    assert d("dest_account_age_steps", "<=", -0.5) == "conta do destino nunca apareceu antes"
    assert d("dest_account_age_steps", ">", -0.5) == "conta do destino já apareceu antes"
    assert d("dest_account_age_steps", ">", 24.5) == "idade do destino (horas) > 24,50"


def test_cortes_repetidos_da_mesma_feature_viram_um_so():
    j = surrogate._join_conditions
    assert j([("amount", "<=", 40885), ("amount", "<=", 22603), ("cycle_detected", "<=", 0.5)]) == (
        "valor <= 22.603 e ciclo detectado: não"
    )
    assert j([("amount", "<=", 40885), ("amount", ">", 22603)]) == "valor entre 22.603 e 40.885"
    assert j([("amount", ">", 100), ("amount", ">", 500)]) == "valor > 500"
    assert j([("dest_account_age_steps", ">", -0.5), ("dest_account_age_steps", "<=", 24.5)]) == (
        "conta do destino já apareceu antes e idade do destino (horas) <= 24,50"
    )
    assert j([]) == "todas as transações"


def test_nenhuma_regra_de_perfil_repete_a_mesma_feature_duas_vezes(results_graph):
    s = surrogate.fit(results_graph, "model", 0.5, max_depth=5)
    for regra in s.profiles["regra"]:
        assert regra.count("valor <=") <= 1 and regra.count("valor >") <= 1, regra


def test_nao_altera_a_entrada(results_graph):
    antes = results_graph.copy(deep=True)
    surrogate.fit(results_graph, "model", 0.5)
    pd.testing.assert_frame_equal(results_graph, antes)
```

- [ ] **Passo 2: rodar e ver falhar**

Rode: `web_dashboard/.venv/bin/python -m pytest web_dashboard/tests/test_surrogate.py -q`
Esperado: FAIL com `ModuleNotFoundError: No module named 'surrogate'`.

- [ ] **Passo 3: implementar**

```python file=web_dashboard/surrogate.py
"""Árvore substituta: uma árvore rasa que imita as decisões do modelo (ou o gabarito)
para explicar, com regras legíveis, o que pesa na decisão. Ela mostra o que a árvore
aprendeu sobre o alvo, não o que o modelo pensa por dentro. Por isso a fidelidade
sempre aparece junto."""

from __future__ import annotations

from dataclasses import dataclass, field

import numpy as np
import pandas as pd
from sklearn.model_selection import train_test_split
from sklearn.tree import DecisionTreeClassifier, export_text

import analysis

FIDELITY_MIN = 0.8
MIN_ROWS = 20

TABULAR_FEATURES = [
    "amount",
    "orig_balance_before",
    "orig_balance_after",
    "dest_balance_before",
    "dest_balance_after",
    "balance_drop",
    "dest_is_merchant",
    "step",
]
GRAPH_FEATURES = [
    "first_transfer_between_pair",
    "dest_in_degree",
    "dest_fan_in_window",
    "dest_fan_out_window",
    "orig_fan_out_window",
    "dest_account_age_steps",
    "cycle_detected",
]
BINARY_FEATURES = {"dest_is_merchant", "first_transfer_between_pair", "cycle_detected"}

LABELS = {
    "amount": "valor",
    "orig_balance_before": "saldo da origem antes",
    "orig_balance_after": "saldo da origem depois",
    "dest_balance_before": "saldo do destino antes",
    "dest_balance_after": "saldo do destino depois",
    "balance_drop": "queda do saldo da origem",
    "dest_is_merchant": "destino é comerciante",
    "step": "hora (step)",
    "first_transfer_between_pair": "primeira transferência do par",
    "dest_in_degree": "remetentes do destino (histórico)",
    "dest_fan_in_window": "remetentes do destino na janela",
    "dest_fan_out_window": "destinatários do destino na janela",
    "orig_fan_out_window": "destinatários da origem na janela",
    "dest_account_age_steps": "idade do destino (horas)",
    "cycle_detected": "ciclo detectado",
}


def label(feature: str) -> str:
    """Nome legível de uma feature."""
    if feature.startswith("type_"):
        return f"tipo {feature[5:]}"
    return LABELS.get(feature, feature)


@dataclass
class Surrogate:
    target: str
    n: int
    fidelity: float | None = None
    fidelity_train: float | None = None
    reliable: bool = False
    warning: str | None = None
    tree_text: str = ""
    importances: pd.DataFrame = field(
        default_factory=lambda: pd.DataFrame(columns=["feature", "rotulo", "importancia"])
    )
    profiles: pd.DataFrame = field(
        default_factory=lambda: pd.DataFrame(
            columns=[
                "perfil",
                "regra",
                "transacoes",
                "pct_amostra",
                "pct_fraude_modelo",
                "pct_fraude_real",
                "arvore_decide",
            ]
        )
    )
    counts: pd.DataFrame = field(
        default_factory=lambda: pd.DataFrame(columns=["perfil", "legitimas", "fraudes"])
    )


def feature_matrix(df: pd.DataFrame) -> pd.DataFrame:
    """Só números, sem nulos. O que faltava (por exemplo a idade de uma conta que nunca
    apareceu) vira -1. O tipo da transação vira colunas 0 ou 1."""
    cols = {}
    for name in TABULAR_FEATURES + GRAPH_FEATURES:
        if name in df.columns:
            cols[name] = pd.to_numeric(df[name], errors="coerce").astype(float)
    x = pd.DataFrame(cols, index=df.index)
    if "type" in df.columns:
        for tipo in sorted(df["type"].dropna().unique()):
            x[f"type_{tipo}"] = (df["type"] == tipo).astype(float)
    return x.fillna(-1.0)


def _num(valor: float) -> str:
    if abs(valor) >= 100:
        return f"{valor:,.0f}".replace(",", ".")
    return f"{valor:.2f}".replace(".", ",")


def _condition(name: str, lower: float | None, upper: float | None) -> str:
    """Uma condição sobre uma feature. lower (exclusivo) e upper (inclusivo) são None
    quando não há limite naquele lado."""
    if name.startswith("type_"):
        tipo = name[5:]
        return f"tipo é {tipo}" if lower is not None else f"tipo não é {tipo}"
    if name in BINARY_FEATURES:
        return f"{LABELS[name]}: {'sim' if lower is not None else 'não'}"
    partes = []
    if name == "dest_account_age_steps":
        if upper is not None and upper < 0:
            return "conta do destino nunca apareceu antes"
        if lower is not None and lower < 0:
            partes.append("conta do destino já apareceu antes")
            lower = None
    nome = label(name)
    if lower is not None and upper is not None:
        partes.append(f"{nome} entre {_num(lower)} e {_num(upper)}")
    elif upper is not None:
        partes.append(f"{nome} <= {_num(upper)}")
    elif lower is not None:
        partes.append(f"{nome} > {_num(lower)}")
    return " e ".join(partes)


def _describe(name: str, op: str, threshold: float) -> str:
    """Uma condição simples da árvore em português. op é '<=' ou '>'."""
    if op == "<=":
        return _condition(name, None, threshold)
    return _condition(name, threshold, None)


def _join_conditions(conds: list[tuple[str, str, float]]) -> str:
    """Junta as condições do caminho de uma folha. Cortes repetidos da mesma feature viram
    um só (vale o mais restrito), então a regra não repete `valor <= 40.885 e valor <= 22.603`."""
    limites: dict[str, list[float | None]] = {}
    for name, op, thr in conds:
        par = limites.setdefault(name, [None, None])
        if op == "<=":
            par[1] = thr if par[1] is None else min(par[1], thr)
        else:
            par[0] = thr if par[0] is None else max(par[0], thr)
    textos = [_condition(nome, lo, up) for nome, (lo, up) in limites.items()]
    return " e ".join(textos) if textos else "todas as transações"


def _leaf_rules(tree: DecisionTreeClassifier, names: list[str]) -> dict[int, str]:
    """A regra de cada folha: as condições do caminho da raiz até ela."""
    t = tree.tree_
    rules: dict[int, str] = {}

    def walk(node: int, conds: list[tuple[str, str, float]]) -> None:
        if t.children_left[node] == -1:
            rules[node] = _join_conditions(conds)
            return
        name, thr = names[t.feature[node]], float(t.threshold[node])
        walk(t.children_left[node], [*conds, (name, "<=", thr)])
        walk(t.children_right[node], [*conds, (name, ">", thr)])

    walk(0, [])
    return rules


def fit(
    df: pd.DataFrame, target: str, threshold: float, max_depth: int = 3, seed: int = 0
) -> Surrogate:
    """Treina a árvore substituta. target é 'model' (decisões do modelo no limiar) ou
    'truth' (gabarito is_fraud)."""
    if target not in ("model", "truth"):
        raise ValueError("target precisa ser 'model' ou 'truth'")
    if not 2 <= max_depth <= 5:
        raise ValueError("a profundidade precisa estar entre 2 e 5")

    d = analysis.scored(df).reset_index(drop=True)
    out = Surrogate(target=target, n=len(d))
    if len(d) < MIN_ROWS:
        out.warning = f"Só {len(d)} transações respondidas. Preciso de pelo menos {MIN_ROWS} para montar perfis."
        return out

    model_pred = analysis.predicted(d, threshold).to_numpy()
    real = d["is_fraud"].astype(bool).to_numpy()
    y = (model_pred if target == "model" else real).astype(int)
    if len(np.unique(y)) < 2:
        alvo = "as decisões do modelo" if target == "model" else "o gabarito"
        out.warning = (
            f"Todas as transações têm a mesma classe em {alvo}. "
            "Não dá para montar perfis. Mude o limiar ou o alvo."
        )
        return out

    x = feature_matrix(d)
    contagem = np.bincount(y)
    treino, teste = train_test_split(
        np.arange(len(d)),
        test_size=0.3,
        random_state=seed,
        stratify=y if contagem.min() >= 2 else None,
    )
    tree = DecisionTreeClassifier(
        max_depth=max_depth, min_samples_leaf=max(1, round(0.01 * len(treino))), random_state=seed
    )
    tree.fit(x.iloc[treino], y[treino])
    out.fidelity = float(tree.score(x.iloc[teste], y[teste]))
    out.fidelity_train = float(tree.score(x.iloc[treino], y[treino]))
    out.reliable = out.fidelity >= FIDELITY_MIN

    nomes = list(x.columns)
    imp = pd.DataFrame({"feature": nomes, "importancia": tree.feature_importances_})
    imp = (
        imp[imp["importancia"] > 0]
        .sort_values("importancia", ascending=False)
        .reset_index(drop=True)
    )
    imp.insert(1, "rotulo", imp["feature"].map(label))
    out.importances = imp
    out.tree_text = export_text(
        tree, feature_names=[label(n) for n in nomes], decimals=1, show_weights=True
    )

    # Um perfil por folha, contando todas as transações da amostra.
    folhas = tree.apply(x)
    regras = _leaf_rules(tree, nomes)
    quadro = pd.DataFrame({"folha": folhas, "modelo": model_pred, "real": real})
    grupos = quadro.groupby("folha").agg(
        transacoes=("folha", "size"),
        fraude_modelo=("modelo", "mean"),
        fraudes=("real", "sum"),
    )
    grupos = grupos.sort_values("transacoes", ascending=False, kind="stable")
    perfis = [f"Perfil {i + 1}" for i in range(len(grupos))]
    out.profiles = pd.DataFrame(
        {
            "perfil": perfis,
            "regra": [regras[f] for f in grupos.index],
            "transacoes": grupos["transacoes"].to_numpy(),
            "pct_amostra": grupos["transacoes"].to_numpy() / len(d) * 100,
            "pct_fraude_modelo": grupos["fraude_modelo"].to_numpy() * 100,
            "pct_fraude_real": grupos["fraudes"].to_numpy() / grupos["transacoes"].to_numpy() * 100,
            "arvore_decide": [
                "fraude" if np.argmax(tree.tree_.value[f][0]) == 1 else "legítima"
                for f in grupos.index
            ],
        }
    )
    out.counts = pd.DataFrame(
        {
            "perfil": perfis,
            "legitimas": (grupos["transacoes"] - grupos["fraudes"]).to_numpy(),
            "fraudes": grupos["fraudes"].to_numpy(),
        }
    )

    avisos = []
    if len(out.profiles) == 1:
        avisos.append(
            "A árvore não achou nenhuma regra que separe as decisões, então só há um perfil."
        )
    if not out.reliable:
        alvo = "das decisões do modelo" if target == "model" else "do gabarito"
        avisos.append(
            f"Atenção: a árvore reproduz só {out.fidelity * 100:.1f}% {alvo} em dados separados "
            f"(o mínimo é {FIDELITY_MIN * 100:.0f}%). Ela explica mal o alvo, então não confie nos perfis."
        )
    out.warning = " ".join(avisos) or None
    return out
```

- [ ] **Passo 4: rodar e ver passar**

Rode: `web_dashboard/.venv/bin/python -m pytest web_dashboard/tests/test_surrogate.py -q`
Esperado: todos passam.

- [ ] **Passo 5: commitar**

```bash
git add web_dashboard/surrogate.py web_dashboard/tests/test_surrogate.py
git commit -m "feat: adiciona árvore substituta para explicar as decisões"
```

---

## Tarefa 5: tema Asaas e gráficos

**Arquivos:**
- Criar: `web_dashboard/plots.py`
- Testar: `web_dashboard/tests/test_plots.py`

**Interfaces:**
- Consumes: `analysis.roc_pr`, `analysis.threshold_sweep`, `analysis.fraud_types` (os dicts e DataFrames que elas devolvem) e `surrogate.Surrogate` (os DataFrames `importances` e `counts`).
- Produz (todas devolvem `plotly.graph_objects.Figure` já com o tema aplicado, e nunca estouram com dado vazio, devolvem uma figura com um aviso):
  - Constantes: `ASAAS_COLORS`, `COLOR_A` (`#1e55ff`), `COLOR_B` (`#e86835`), `COLOR_NEUTRAL`, `COLOR_REFERENCE` (`#FF365E`, só para linha de referência), `COLOR_POSITIVE`, `COLOR_NEGATIVE`, `CLASS_LEGIT`, `CLASS_FRAUD`, `ASAAS_LIGHT` (o template) e `run_color(index) -> str` (A é o índice 0, B é o 1).
  - `empty_figure(message, height=300)`
  - `confusion_figure(confs: dict[str, dict])`: uma matriz por execução, com os 4 números escritos nas células.
  - `roc_pr_figure(curves: dict[str, dict | None], colors: dict[str, str], prevalence: float)`
  - `threshold_sweep_figure(sweeps: dict[str, DataFrame], colors, threshold)`: F1 por limiar, com uma linha de referência no limiar atual.
  - `fp_fn_figure(confs, colors)`
  - `fraud_types_figure(tables: dict[str, DataFrame], colors)`
  - `latency_percentiles_figure(latency: dict[str, dict], colors)`: dois gráficos lado a lado, Neo4j e modelo.
  - `latency_histogram_figure(dfs: dict[str, DataFrame], colors, column, title, percentiles: dict[str, dict])`
  - `layout_subgraph(nodes, edges, orig, dest) -> dict[str, tuple[float, float]]` e `subgraph_figure(subgraph: dict, orig, dest)`
  - `importance_figure(importances: DataFrame)` e `profile_counts_figure(counts: DataFrame)`

Regras de visualização aplicadas (do guia de dados do ambiente e do padrão Asaas): paleta fixa, sem eixo duplo, série única sem legenda e 2 ou mais com legenda, barras com o fim arredondado de 4 px e 2 px de "respiro" da cor do fundo entre segmentos, linhas de 2 px, marcadores de pelo menos 8 px com anel de 2 px, hover em todo gráfico, texto sempre em tom de texto e nunca na cor da série. Foi o validador de paleta que escolheu o par A e B (veja o cabeçalho do plano).

- [ ] **Passo 1: escrever os testes que falham**

```python file=web_dashboard/tests/test_plots.py
import numpy as np
import pandas as pd
import plotly.graph_objects as go

import analysis
import plots
import surrogate
from tests.conftest import make_results

CORES = {"A": plots.COLOR_A, "B": plots.COLOR_B}


def sem_eixo_duplo(fig: go.Figure) -> bool:
    for nome in fig.layout:
        if nome.startswith("yaxis") and fig.layout[nome].overlaying:
            return False
    return True


def textos_das_anotacoes(fig: go.Figure) -> list[str]:
    return [a.text for a in fig.layout.annotations]


def confusao(tp=8, fp=2, tn=88, fn=2):
    return {"tp": tp, "fp": fp, "tn": tn, "fn": fn}


def test_paleta_e_reservas():
    assert plots.COLOR_A == plots.ASAAS_COLORS[0]
    assert plots.COLOR_B in plots.ASAAS_COLORS
    assert plots.COLOR_A != plots.COLOR_B
    assert plots.COLOR_REFERENCE not in plots.ASAAS_COLORS  # reservada para linha de referência
    assert plots.run_color(0) == plots.COLOR_A and plots.run_color(1) == plots.COLOR_B
    assert list(plots.ASAAS_LIGHT.layout.colorway) == plots.ASAAS_COLORS


def test_empty_figure_mostra_o_aviso():
    fig = plots.empty_figure("Nada para mostrar")
    assert "Nada para mostrar" in textos_das_anotacoes(fig)
    assert not fig.layout.xaxis.visible and not fig.layout.yaxis.visible


def test_confusion_figure_uma_matriz_por_execucao_com_os_numeros():
    fig = plots.confusion_figure({"A": confusao(), "B": confusao(9, 1, 89, 1)})
    heatmaps = [t for t in fig.data if isinstance(t, go.Heatmap)]
    assert len(heatmaps) == 2
    assert [list(r) for r in heatmaps[0].z] == [[88, 2], [2, 8]]  # [[TN, FP], [FN, TP]]
    assert [list(r) for r in heatmaps[1].z] == [[89, 1], [1, 9]]
    numeros = [t for t in textos_das_anotacoes(fig) if t.isdigit()]
    assert sorted(numeros) == sorted(["88", "2", "2", "8", "89", "1", "1", "9"])
    assert all(h.showscale is False for h in heatmaps)
    assert sem_eixo_duplo(fig)


def test_confusion_figure_com_uma_so_execucao_e_com_zeros():
    fig = plots.confusion_figure({"A": confusao(0, 0, 0, 0)})
    assert len([t for t in fig.data if isinstance(t, go.Heatmap)]) == 1
    assert plots.confusion_figure({}).layout.annotations[0].text  # vazio vira aviso


def test_roc_pr_figure(results_graph, results_nograph):
    curvas = {
        "A": analysis.roc_pr(results_nograph, 0.003),
        "B": analysis.roc_pr(results_graph, 0.003),
    }
    fig = plots.roc_pr_figure(curvas, CORES, 0.003)
    linhas = [t for t in fig.data if isinstance(t, go.Scatter)]
    assert len(linhas) == 2 + 2 + 1  # ROC de A e B, PR de A e B e a linha do acaso
    com_legenda = [t.name for t in linhas if t.showlegend]
    assert len(com_legenda) == 2 and all("AUC" in n for n in com_legenda)
    assert {t.line.color for t in linhas} >= {plots.COLOR_A, plots.COLOR_B, plots.COLOR_REFERENCE}
    assert all(t.line.width in (1, 2) for t in linhas)
    assert sem_eixo_duplo(fig)
    assert any("0,30%" in t for t in textos_das_anotacoes(fig))
    assert fig.layout.legend.y > 1.1 and fig.layout.legend.x == 1  # acima dos títulos, à direita


def test_roc_pr_figure_com_curva_ausente():
    so_uma_classe = analysis.roc_pr(
        pd.DataFrame({"noul": [0.1, 0.2], "is_fraud": [False, False]}), 0.003
    )
    assert so_uma_classe is None
    fig = plots.roc_pr_figure({"A": None, "B": None}, CORES, 0.003)
    assert any("classes" in t for t in textos_das_anotacoes(fig))
    ok = analysis.roc_pr(make_results(), 0.003)
    fig = plots.roc_pr_figure({"A": None, "B": ok}, CORES, 0.003)
    assert len([t for t in fig.data if t.showlegend]) == 1


def test_threshold_sweep_figure(results_graph, results_nograph):
    varreduras = {
        "A": analysis.threshold_sweep(results_nograph, 0.003),
        "B": analysis.threshold_sweep(results_graph, 0.003),
    }
    fig = plots.threshold_sweep_figure(varreduras, CORES, 0.35)
    linhas = [t for t in fig.data if isinstance(t, go.Scatter)]
    assert [t.name for t in linhas] == ["A", "B"]
    referencias = [s for s in fig.layout.shapes if s.line.color == plots.COLOR_REFERENCE]
    assert len(referencias) == 1 and referencias[0].x0 == 0.35
    assert list(fig.layout.yaxis.range) == [0, 1.05]
    assert sem_eixo_duplo(fig)


def test_fp_fn_figure():
    fig = plots.fp_fn_figure({"A": confusao(fp=30, fn=5), "B": confusao(fp=10, fn=6)}, CORES)
    barras = [t for t in fig.data if isinstance(t, go.Bar)]
    assert [t.name for t in barras] == ["A", "B"]
    assert list(barras[0].x) == ["Falsos positivos", "Falsos negativos"]
    assert list(barras[0].y) == [30, 5] and list(barras[1].y) == [10, 6]
    assert barras[0].marker.color == plots.COLOR_A and barras[0].marker.cornerradius == 4
    assert fig.layout.barmode == "group" and sem_eixo_duplo(fig)
    assert fig.layout.bargap >= 0.7  # barras finas (até 24 px na largura de meia coluna)


def test_fraud_types_figure(results_graph, results_nograph):
    tabelas = {
        "A": analysis.fraud_types(results_nograph, 0.5),
        "B": analysis.fraud_types(results_graph, 0.5),
    }
    fig = plots.fraud_types_figure(tabelas, CORES)
    nomes = [t.name for t in fig.data]
    assert nomes[0] == "Fraudes na amostra" and fig.data[0].marker.color == plots.COLOR_NEUTRAL
    assert nomes[1:] == ["Detectadas por A", "Detectadas por B"]
    assert list(fig.data[0].x) == list(fig.data[1].x)
    vazia = plots.fraud_types_figure(
        {"A": pd.DataFrame(columns=["tipo", "fraudes", "detectadas"])}, CORES
    )
    assert any("fraude" in t for t in textos_das_anotacoes(vazia))


def test_latency_percentiles_figure_legenda_so_uma_vez():
    latencias = {
        "A": {
            "neo4j_ms": {"p50": 0, "p95": 0, "p99": 0},
            "jev_ms": {"p50": 4, "p95": 9, "p99": 14},
        },
        "B": {
            "neo4j_ms": {"p50": 12, "p95": 30, "p99": 50},
            "jev_ms": {"p50": 5, "p95": 11, "p99": 16},
        },
    }
    fig = plots.latency_percentiles_figure(latencias, CORES)
    barras = [t for t in fig.data if isinstance(t, go.Bar)]
    assert len(barras) == 4
    assert sum(1 for t in barras if t.showlegend) == 2
    assert list(barras[1].y) == [12, 30, 50]  # Neo4j de B
    assert list(barras[2].y) == [4, 9, 14]  # modelo de A
    assert fig.layout.legend.y > 1.1 and fig.layout.legend.x == 1
    assert sorted(textos_das_anotacoes(fig))[0].startswith(("Modelo", "Neo4j"))
    assert sem_eixo_duplo(fig)


def test_latency_histogram_figure(results_graph, results_nograph):
    pct = {"A": {"p50": 4, "p95": 9, "p99": 14}, "B": {"p50": 12, "p95": 30, "p99": 50}}
    fig = plots.latency_histogram_figure(
        {"A": results_nograph, "B": results_graph}, CORES, "neo4j_ms", "Latência do Neo4j", pct
    )
    assert [t.name for t in fig.data] == ["B"]  # A não usou grafo, então não tem tempo do Neo4j
    assert fig.layout.legend.y < 0  # a legenda fica embaixo, longe dos rótulos P50 a P99
    assert len([s for s in fig.layout.shapes if s.type == "line"]) == 3  # P50, P95 e P99 de B
    fig = plots.latency_histogram_figure(
        {"A": results_nograph, "B": results_graph}, CORES, "jev_ms", "Latência do modelo", pct
    )
    assert [t.name for t in fig.data] == ["A", "B"]
    vazia = plots.latency_histogram_figure({"A": results_nograph}, CORES, "neo4j_ms", "x", pct)
    assert any("grafo" in t for t in textos_das_anotacoes(vazia))


def subgrafo():
    return {
        "nodes": ["C1", "C2", "X", "Y", "Z"],
        "edges": [
            {
                "id": "e1",
                "src": "C1",
                "dst": "C2",
                "type": "TRANSFER",
                "amount": 100.0,
                "step": 1,
                "is_fraud": False,
            },
            {
                "id": "e2",
                "src": "X",
                "dst": "C1",
                "type": "TRANSFER",
                "amount": 5.0,
                "step": 1,
                "is_fraud": False,
            },
            {
                "id": "e3",
                "src": "Y",
                "dst": "C2",
                "type": "CASH_OUT",
                "amount": 7.0,
                "step": 2,
                "is_fraud": True,
            },
            {
                "id": "e4",
                "src": "Z",
                "dst": "C1",
                "type": "TRANSFER",
                "amount": 8.0,
                "step": 2,
                "is_fraud": False,
            },
            {
                "id": "e5",
                "src": "Z",
                "dst": "C2",
                "type": "TRANSFER",
                "amount": 9.0,
                "step": 2,
                "is_fraud": False,
            },
        ],
    }


def test_layout_do_subgrafo_separa_origem_destino_e_vizinhas():
    g = subgrafo()
    pos = plots.layout_subgraph(g["nodes"], g["edges"], "C1", "C2")
    assert pos["C1"][0] < 0 < pos["C2"][0]
    assert pos["X"][0] < pos["C1"][0]  # vizinha só da origem, à esquerda
    assert pos["Y"][0] > pos["C2"][0]  # vizinha só do destino, à direita
    assert pos["C1"][0] < pos["Z"][0] < pos["C2"][0]  # vizinha das duas, no meio
    assert set(pos) == set(g["nodes"])
    isolado = plots.layout_subgraph(["C1", "C2", "W"], [], "C1", "C2")
    assert "W" in isolado


def test_subgraph_figure():
    g = subgrafo()
    fig = plots.subgraph_figure(g, "C1", "C2")
    nomes = [t.name for t in fig.data if t.showlegend]
    assert {
        "Origem",
        "Destino",
        "Outras contas",
        "Transferência",
        "Marcada como fraude no dataset",
    } <= set(nomes)
    setas = [a for a in fig.layout.annotations if a.showarrow]
    assert len(setas) == 5
    assert sum(1 for a in setas if a.arrowcolor == plots.COLOR_NEGATIVE) == 1
    nos = {
        t.name: t for t in fig.data if t.mode == "markers+text" and t.name in ("Origem", "Destino")
    }
    assert len(nos) == 2 and all(t.marker.size >= 8 for t in nos.values())
    assert not fig.layout.xaxis.visible and sem_eixo_duplo(fig)


def test_subgraph_figure_vazio():
    fig = plots.subgraph_figure({"nodes": ["C1", "C2"], "edges": []}, "C1", "C2")
    assert len([a for a in fig.layout.annotations if a.showarrow]) == 0
    assert any("nenhuma" in t.lower() for t in textos_das_anotacoes(fig))


def test_importance_figure_serie_unica_sem_legenda(results_graph):
    s = surrogate.fit(results_graph, "model", 0.5)
    fig = plots.importance_figure(s.importances)
    assert len(fig.data) == 1 and fig.data[0].orientation == "h"
    assert fig.layout.showlegend is False
    assert fig.data[0].marker.color == plots.COLOR_A
    assert list(fig.data[0].y)[0] == s.importances["rotulo"].iloc[0]
    assert any(
        "regra" in t.lower() for t in textos_das_anotacoes(plots.importance_figure(pd.DataFrame()))
    )


def test_profile_counts_figure(results_graph):
    s = surrogate.fit(results_graph, "model", 0.5)
    fig = plots.profile_counts_figure(s.counts)
    assert [t.name for t in fig.data] == ["Legítimas", "Fraudes"]
    assert (
        fig.data[0].marker.color == plots.CLASS_LEGIT
        and fig.data[1].marker.color == plots.CLASS_FRAUD
    )
    assert fig.layout.barmode == "stack" and fig.data[0].marker.line.width == 2
    assert list(fig.data[0].y) == list(s.counts["perfil"])
    assert plots.profile_counts_figure(pd.DataFrame()).layout.annotations


def test_todas_as_figuras_serializam_e_usam_o_tema(results_graph):
    figuras = [
        plots.empty_figure("x"),
        plots.confusion_figure({"A": confusao()}),
        plots.fp_fn_figure({"A": confusao()}, CORES),
        plots.importance_figure(surrogate.fit(results_graph, "model", 0.5).importances),
    ]
    for fig in figuras:
        assert fig.to_json()
        assert fig.layout.template.layout.font.family == plots.FONT_BODY
        assert np.isfinite(fig.layout.height)
```

- [ ] **Passo 2: rodar e ver falhar**

Rode: `web_dashboard/.venv/bin/python -m pytest web_dashboard/tests/test_plots.py -q`
Esperado: FAIL com `ModuleNotFoundError: No module named 'plots'`.

- [ ] **Passo 3: implementar**

```python file=web_dashboard/plots.py
"""Gráficos do dashboard, no padrão visual da Asaas (variante clara).

Regras aplicadas em todos eles: paleta fixa (sem gerar cor nova), nada de eixo duplo,
legenda quando há 2 ou mais séries e nenhuma quando há uma só, barras com o fim
arredondado e um respiro da cor do fundo entre segmentos, linhas de 2 px, hover em todo
gráfico e texto sempre em tom de texto (nunca na cor da série). O #FF365E é só para linha
de referência.
"""

from __future__ import annotations

import plotly.graph_objects as go
from plotly.subplots import make_subplots

FONT_TITLE = "Space Grotesk, Inter, system-ui, sans-serif"
FONT_BODY = "Geist, Inter, system-ui, sans-serif"

ASAAS_COLORS = [
    "#1e55ff",
    "#488af5",
    "#499c9a",
    "#3b7736",
    "#ab8a02",
    "#d13700",
    "#e86835",
    "#cf5fec",
    "#9d6100",
    "#737f29",
    "#5297bc",
    "#b35a00",
]
# A execução A é o slot 1 da paleta. A B é o slot 7: o par dos slots 1 e 2 (dois azuis)
# foi reprovado pelo validador de paleta, porque não dá para distinguir as duas linhas.
COLOR_A = "#1e55ff"
COLOR_B = "#e86835"
COLOR_NEUTRAL = "#767461"  # contexto (por exemplo "fraudes na amostra")
COLOR_REFERENCE = "#FF365E"  # só para linha de referência
COLOR_POSITIVE = "#3b7736"
COLOR_NEGATIVE = "#d13700"
CLASS_LEGIT = "#b4d2e2"  # legítimas
CLASS_FRAUD = "#020c44"  # fraudes
NODE_ORIGIN = "#020c44"
NODE_DEST = "#0038e5"

INK = "#162f36"
INK_TITLE = "#020c44"
GRID = "#e0e0d8"
ZERO_LINE = "#c0c0b8"
PAPER = "#ffffff"
PLOT_BG = "#f8f8f1"
SURFACE = "#ffffff"
HEAT_SCALE = [[0.0, "#eef3ff"], [0.5, "#488af5"], [1.0, "#0038e5"]]

_AXIS = dict(
    gridcolor=GRID,
    zerolinecolor=ZERO_LINE,
    linecolor=GRID,
    tickfont=dict(color=INK, family=FONT_BODY, size=11),
    title=dict(font=dict(color=INK, family=FONT_BODY, size=12)),
    showspikes=True,
    spikecolor="rgba(22,47,54,0.4)",
    spikethickness=1,
    spikemode="across",
    spikedash="dot",
)

ASAAS_LIGHT = go.layout.Template(
    layout=go.Layout(
        paper_bgcolor=PAPER,
        plot_bgcolor=PLOT_BG,
        font=dict(color=INK, family=FONT_BODY, size=12),
        title=dict(font=dict(size=16, color=INK_TITLE, family=FONT_TITLE), x=0),
        xaxis=_AXIS,
        yaxis=_AXIS,
        colorway=ASAAS_COLORS,
        legend=dict(
            font=dict(color=INK, family=FONT_BODY, size=11),
            bgcolor="rgba(255,255,255,0.85)",
            bordercolor=GRID,
            borderwidth=1,
            orientation="h",
            yanchor="bottom",
            y=1.02,
            xanchor="left",
            x=0,
        ),
        hoverlabel=dict(
            bgcolor="#ffffff", bordercolor=GRID, font=dict(color=INK, family=FONT_BODY, size=12)
        ),
        hovermode="x unified",
        margin=dict(l=60, r=30, t=80, b=60),
    )
)


def run_color(index: int) -> str:
    """Cor da execução: 0 é a A e 1 é a B."""
    return (COLOR_A, COLOR_B)[index]


def _themed(fig: go.Figure, title: str | None = None, height: int = 380) -> go.Figure:
    fig.update_layout(template=ASAAS_LIGHT, height=height)
    if title:
        fig.update_layout(title=dict(text=title))
    return fig


def empty_figure(message: str, height: int = 300) -> go.Figure:
    fig = go.Figure()
    fig.add_annotation(
        text=message,
        x=0.5,
        y=0.5,
        xref="paper",
        yref="paper",
        showarrow=False,
        font=dict(color=INK, family=FONT_BODY, size=14),
    )
    fig.update_xaxes(visible=False)
    fig.update_yaxes(visible=False)
    return _themed(fig, height=height)


def _text_on(fill_ratio: float) -> str:
    """Texto dentro de uma célula: branco em fundo escuro e tinta em fundo claro."""
    return "#ffffff" if fill_ratio > 0.5 else INK


def confusion_figure(confs: dict[str, dict]) -> go.Figure:
    """Uma matriz de confusão por execução (linhas: real, colunas: previsto)."""
    if not confs:
        return empty_figure("Escolha uma execução para ver a matriz de confusão.")
    fig = make_subplots(
        rows=1, cols=len(confs), subplot_titles=list(confs), horizontal_spacing=0.14
    )
    classes = ["Legítima", "Fraude"]
    for i, (nome, c) in enumerate(confs.items(), start=1):
        z = [[c["tn"], c["fp"]], [c["fn"], c["tp"]]]
        zmax = max(max(linha) for linha in z) or 1
        fig.add_trace(
            go.Heatmap(
                z=z,
                x=classes,
                y=classes,
                colorscale=HEAT_SCALE,
                zmin=0,
                zmax=zmax,
                showscale=False,
                xgap=2,
                ygap=2,
                name=nome,
                hovertemplate="Real: %{y}<br>Previsto: %{x}<br>Transações: %{z}<extra></extra>",
            ),
            row=1,
            col=i,
        )
        eixo = "" if i == 1 else str(i)
        for linha, real in enumerate(classes):
            for coluna, previsto in enumerate(classes):
                valor = z[linha][coluna]
                fig.add_annotation(
                    text=str(valor),
                    x=previsto,
                    y=real,
                    xref=f"x{eixo}",
                    yref=f"y{eixo}",
                    showarrow=False,
                    font=dict(color=_text_on(valor / zmax), family=FONT_BODY, size=16),
                )
    fig.update_xaxes(title_text="Previsto", showspikes=False)
    fig.update_yaxes(title_text="Real", autorange="reversed", showspikes=False)
    fig.update_layout(hovermode="closest")
    return _themed(fig, "Matriz de confusão", height=340)


def roc_pr_figure(
    curves: dict[str, dict | None], colors: dict[str, str], prevalence: float
) -> go.Figure:
    """ROC e PR lado a lado. A PR é reprojetada para a prevalência real."""
    validas = {n: c for n, c in curves.items() if c is not None}
    if not validas:
        return empty_figure(
            "As curvas precisam de fraudes e de legítimas na amostra. Só existe uma das duas classes."
        )
    fig = make_subplots(
        rows=1,
        cols=2,
        horizontal_spacing=0.1,
        subplot_titles=(
            "ROC",
            f"Precisão x recall (prevalência real de {prevalence * 100:.2f}%)".replace(".", ","),
        ),
    )
    fig.add_trace(
        go.Scatter(
            x=[0, 1],
            y=[0, 1],
            mode="lines",
            name="Acaso",
            showlegend=False,
            hoverinfo="skip",
            line=dict(color=COLOR_REFERENCE, width=1, dash="dash"),
        ),
        row=1,
        col=1,
    )
    for nome, c in validas.items():
        cor = colors[nome]
        fig.add_trace(
            go.Scatter(
                x=c["fpr"],
                y=c["tpr"],
                mode="lines",
                name=f"{nome} (AUC {c['auc_roc']:.3f})",
                legendgroup=nome,
                showlegend=True,
                line=dict(color=cor, width=2),
                hovertemplate="Falsos positivos: %{x:.3f}<br>Recall: %{y:.3f}<extra></extra>",
            ),
            row=1,
            col=1,
        )
        fig.add_trace(
            go.Scatter(
                x=c["recall"],
                y=c["precision"],
                mode="lines",
                name=f"{nome} (AUC PR {c['auc_pr']:.3f})",
                legendgroup=nome,
                showlegend=False,
                line=dict(color=cor, width=2),
                hovertemplate="Recall: %{x:.3f}<br>Precisão: %{y:.3f}<extra></extra>",
            ),
            row=1,
            col=2,
        )
    fig.update_xaxes(title_text="Taxa de falso positivo", range=[0, 1], row=1, col=1)
    fig.update_yaxes(title_text="Recall", range=[0, 1.02], row=1, col=1)
    fig.update_xaxes(title_text="Recall", range=[0, 1], row=1, col=2)
    fig.update_yaxes(title_text="Precisão", range=[0, 1.02], row=1, col=2)
    # A legenda vai para a direita e acima dos títulos dos subgráficos, para não encostar no
    # título do gráfico (à esquerda) nem nos títulos de cada painel.
    fig.update_layout(
        hovermode="closest", legend=dict(y=1.2, x=1, xanchor="right"), margin=dict(t=120)
    )
    return _themed(fig, "ROC e PR", height=420)


def threshold_sweep_figure(sweeps: dict, colors: dict[str, str], threshold: float) -> go.Figure:
    if not sweeps:
        return empty_figure("Escolha uma execução para ver o F1 por limiar.")
    fig = go.Figure()
    for nome, sweep in sweeps.items():
        fig.add_trace(
            go.Scatter(
                x=sweep["limiar"],
                y=sweep["f1"],
                mode="lines",
                name=nome,
                showlegend=True,
                line=dict(color=colors[nome], width=2),
                hovertemplate="F1: %{y:.3f}<extra>" + nome + "</extra>",
            )
        )
    fig.add_vline(
        x=threshold,
        line=dict(color=COLOR_REFERENCE, width=1),
        annotation_text=f"limiar {threshold:.2f}".replace(".", ","),
        annotation_font=dict(color=INK, size=11),
    )
    fig.update_xaxes(title_text="Limiar de decisão", range=[0, 1])
    fig.update_yaxes(title_text="F1", range=[0, 1.05])
    return _themed(fig, "F1 por limiar", height=360)


def fp_fn_figure(confs: dict[str, dict], colors: dict[str, str]) -> go.Figure:
    if not confs:
        return empty_figure("Escolha uma execução para comparar os erros.")
    fig = go.Figure()
    for nome, c in confs.items():
        fig.add_trace(
            go.Bar(
                x=["Falsos positivos", "Falsos negativos"],
                y=[c["fp"], c["fn"]],
                name=nome,
                showlegend=True,
                marker=dict(color=colors[nome], cornerradius=4),
                text=[c["fp"], c["fn"]],
                textposition="outside",
                textfont=dict(color=INK, family=FONT_BODY, size=12),
                cliponaxis=False,
            )
        )
    fig.update_layout(barmode="group", bargap=0.78, bargroupgap=0.08)
    fig.update_yaxes(title_text="Transações")
    return _themed(fig, "Falsos positivos contra falsos negativos", height=360)


def fraud_types_figure(tables: dict, colors: dict[str, str]) -> go.Figure:
    """Fraudes reais por tipo (cinza de contexto) e quantas cada execução detectou."""
    usadas = {n: t for n, t in tables.items() if not t.empty}
    if not usadas:
        return empty_figure("Nenhuma fraude na amostra para mostrar por tipo.")
    tipos = sorted(set().union(*(set(t["tipo"]) for t in usadas.values())))
    referencia = next(iter(usadas.values())).set_index("tipo")["fraudes"]
    fig = go.Figure()
    fig.add_trace(
        go.Bar(
            x=tipos,
            y=[int(referencia.get(t, 0)) for t in tipos],
            name="Fraudes na amostra",
            showlegend=True,
            marker=dict(color=COLOR_NEUTRAL, cornerradius=4),
        )
    )
    for nome, t in usadas.items():
        detectadas = t.set_index("tipo")["detectadas"]
        fig.add_trace(
            go.Bar(
                x=tipos,
                y=[int(detectadas.get(tp, 0)) for tp in tipos],
                name=f"Detectadas por {nome}",
                showlegend=True,
                marker=dict(color=colors[nome], cornerradius=4),
            )
        )
    fig.update_layout(barmode="group", bargap=0.72, bargroupgap=0.08)
    fig.update_yaxes(title_text="Transações")
    fig.update_xaxes(title_text="Tipo de transação")
    return _themed(fig, "Fraudes por tipo", height=360)


def latency_percentiles_figure(latency: dict[str, dict], colors: dict[str, str]) -> go.Figure:
    """P50, P95 e P99 do Neo4j e do modelo. São dois gráficos, cada um com o seu eixo."""
    if not latency:
        return empty_figure("Escolha uma execução para ver as latências.")
    fig = make_subplots(
        rows=1,
        cols=2,
        horizontal_spacing=0.12,
        subplot_titles=("Neo4j (consulta Cypher)", "Modelo (inferência)"),
    )
    for col, componente in enumerate(("neo4j_ms", "jev_ms"), start=1):
        for nome, lat in latency.items():
            p = lat[componente]
            fig.add_trace(
                go.Bar(
                    x=["P50", "P95", "P99"],
                    y=[p["p50"], p["p95"], p["p99"]],
                    name=nome,
                    legendgroup=nome,
                    showlegend=col == 1,
                    marker=dict(color=colors[nome], cornerradius=4),
                ),
                row=1,
                col=col,
            )
    fig.update_layout(
        barmode="group",
        bargap=0.5,
        bargroupgap=0.08,
        legend=dict(y=1.2, x=1, xanchor="right"),
        margin=dict(t=120),
    )
    fig.update_yaxes(title_text="ms")
    return _themed(fig, "Latência por percentil", height=400)


def latency_histogram_figure(
    dfs: dict, colors: dict[str, str], column: str, title: str, percentiles: dict
) -> go.Figure:
    """Histograma de latência de cada execução, com P50, P95 e P99 marcados."""
    fig = go.Figure()
    for nome, df in dfs.items():
        serie = df[column].dropna() if column in df.columns else []
        if len(serie) == 0:
            continue
        fig.add_trace(
            go.Histogram(
                x=serie,
                name=nome,
                nbinsx=40,
                opacity=0.6,
                histnorm="percent",
                showlegend=True,
                marker=dict(color=colors[nome], line=dict(color=SURFACE, width=1)),
            )
        )
        p = percentiles.get(nome, {})
        for deslocamento, chave in enumerate(("p50", "p95", "p99")):
            if chave in p:
                fig.add_vline(
                    x=p[chave],
                    line=dict(color=colors[nome], width=1),
                    annotation_text=chave.upper(),
                    annotation_position="top",
                    annotation_font=dict(color=INK, size=10),
                    annotation_yshift=10 * deslocamento,
                )
    if not fig.data:
        return empty_figure(
            "Nenhuma das execuções escolhidas usou grafo, então não há tempo do Neo4j para mostrar."
        )
    # Legenda embaixo: em cima ficam os rótulos P50, P95 e P99.
    fig.update_layout(barmode="overlay", legend=dict(y=-0.32, yanchor="top"), margin=dict(b=110))
    fig.update_xaxes(title_text="ms")
    fig.update_yaxes(title_text="% das transações")
    return _themed(fig, title, height=360)


def layout_subgraph(
    nodes: list[str], edges: list[dict], orig: str, dest: str
) -> dict[str, tuple[float, float]]:
    """Posições fixas: origem à esquerda, destino à direita, vizinhas só da origem mais à
    esquerda, só do destino mais à direita e as das duas no meio."""
    pos = {orig: (-1.0, 0.0), dest: (1.0, 0.0)}
    perto_origem, perto_destino = set(), set()
    for e in edges:
        for alvo, conjunto in ((orig, perto_origem), (dest, perto_destino)):
            if alvo in (e["src"], e["dst"]):
                conjunto.update({e["src"], e["dst"]} - {orig, dest})
    ambas = perto_origem & perto_destino

    def espalhar(contas: set[str], x: float) -> None:
        ordenadas = sorted(contas)
        for i, conta in enumerate(ordenadas):
            y = 0.0 if len(ordenadas) == 1 else (i / (len(ordenadas) - 1) - 0.5) * 3.0
            pos[conta] = (x, y)

    espalhar(perto_origem - ambas, -2.4)
    espalhar(ambas, 0.0)
    espalhar(perto_destino - ambas, 2.4)
    for conta in nodes:
        pos.setdefault(conta, (0.0, -2.2))
    return pos


def subgraph_figure(subgraph: dict, orig: str, dest: str) -> go.Figure:
    """Vizinhança de 1 salto da origem e do destino. As setas vão do remetente ao
    destinatário. Arestas marcadas como fraude no dataset aparecem em outra cor (o gabarito
    é só para pintar, nunca entra numa feature)."""
    nodes, edges = subgraph.get("nodes", []), subgraph.get("edges", [])
    pos = layout_subgraph(nodes, edges, orig, dest)
    fig = go.Figure()

    for nome, cor in (
        ("Transferência", COLOR_NEUTRAL),
        ("Marcada como fraude no dataset", COLOR_NEGATIVE),
    ):
        fig.add_trace(
            go.Scatter(
                x=[None],
                y=[None],
                mode="lines",
                name=nome,
                showlegend=True,
                line=dict(color=cor, width=2),
                hoverinfo="skip",
            )
        )
    for e in edges:
        (x0, y0), (x1, y1) = pos[e["src"]], pos[e["dst"]]
        cor = COLOR_NEGATIVE if e.get("is_fraud") else COLOR_NEUTRAL
        # a seta termina um pouco antes do nó, para não ficar escondida atrás dele
        fx, fy = x0 + (x1 - x0) * 0.86, y0 + (y1 - y0) * 0.86
        fig.add_annotation(
            x=fx,
            y=fy,
            ax=x0,
            ay=y0,
            xref="x",
            yref="y",
            axref="x",
            ayref="y",
            showarrow=True,
            arrowhead=2,
            arrowsize=1,
            arrowwidth=2 if e.get("is_fraud") else 1.5,
            arrowcolor=cor,
            text="",
        )
    if edges:
        fig.add_trace(
            go.Scatter(
                x=[(pos[e["src"]][0] + pos[e["dst"]][0]) / 2 for e in edges],
                y=[(pos[e["src"]][1] + pos[e["dst"]][1]) / 2 for e in edges],
                mode="markers",
                marker=dict(size=16, opacity=0),
                showlegend=False,
                hovertext=[
                    f"{e['src']} para {e['dst']}<br>{e['type']}, valor {e['amount']:,.2f}, hora {e['step']}"
                    for e in edges
                ],
                hoverinfo="text",
            )
        )
    else:
        fig.add_annotation(
            text="Nenhuma transação anterior envolve essas contas.",
            x=0.5,
            y=0.08,
            xref="paper",
            yref="paper",
            showarrow=False,
            font=dict(color=INK, family=FONT_BODY, size=13),
        )

    grupos = [
        ("Origem", [orig], NODE_ORIGIN, "circle", 18),
        ("Destino", [dest], NODE_DEST, "diamond", 18),
        ("Outras contas", [n for n in nodes if n not in (orig, dest)], COLOR_NEUTRAL, "circle", 11),
    ]
    for nome, contas, cor, simbolo, tamanho in grupos:
        if not contas:
            continue
        fig.add_trace(
            go.Scatter(
                x=[pos[c][0] for c in contas],
                y=[pos[c][1] for c in contas],
                mode="markers+text",
                name=nome,
                showlegend=True,
                marker=dict(
                    color=cor, size=tamanho, symbol=simbolo, line=dict(color=SURFACE, width=2)
                ),
                text=[c if nome != "Outras contas" else "" for c in contas],
                textposition="top center",
                textfont=dict(color=INK, family=FONT_BODY, size=11),
                hovertext=[f"{nome}: {c}" for c in contas],
                hoverinfo="text",
            )
        )
    fig.update_xaxes(visible=False, range=[-3.4, 3.4])
    fig.update_yaxes(visible=False, range=[-2.6, 2.6])
    fig.update_layout(hovermode="closest")
    return _themed(fig, "Subgrafo da transação", height=440)


def importance_figure(importances) -> go.Figure:
    if importances is None or len(importances) == 0:
        return empty_figure(
            "A árvore não achou nenhuma regra, então não há importância para mostrar."
        )
    top = importances.head(10)
    fig = go.Figure(
        go.Bar(
            x=top["importancia"],
            y=top["rotulo"],
            orientation="h",
            marker=dict(color=COLOR_A, cornerradius=4),
            text=[f"{v * 100:.0f}%" for v in top["importancia"]],
            textposition="outside",
            textfont=dict(color=INK, family=FONT_BODY, size=12),
            cliponaxis=False,
            hovertemplate="%{y}: %{x:.1%}<extra></extra>",
        )
    )
    fig.update_yaxes(autorange="reversed")
    fig.update_xaxes(title_text="Importância na árvore", tickformat=".0%")
    fig.update_layout(showlegend=False, hovermode="closest", margin=dict(l=240), bargap=0.45)
    return _themed(fig, "Importância das features", height=max(280, 60 + 38 * len(top)))


def profile_counts_figure(counts) -> go.Figure:
    if counts is None or len(counts) == 0:
        return empty_figure("Sem perfis para mostrar.")
    fig = go.Figure()
    for nome, coluna, cor in (
        ("Legítimas", "legitimas", CLASS_LEGIT),
        ("Fraudes", "fraudes", CLASS_FRAUD),
    ):
        fig.add_trace(
            go.Bar(
                x=counts[coluna],
                y=counts["perfil"],
                orientation="h",
                name=nome,
                showlegend=True,
                marker=dict(color=cor, line=dict(color=SURFACE, width=2)),
                hovertemplate="%{y}: %{x} transações<extra>" + nome + "</extra>",
            )
        )
    fig.update_layout(barmode="stack", hovermode="y unified", bargap=0.5)
    fig.update_yaxes(autorange="reversed")
    fig.update_xaxes(title_text="Transações na amostra")
    return _themed(fig, "Transações por perfil (reais)", height=max(280, 90 + 46 * len(counts)))
```

- [ ] **Passo 4: rodar e ver passar**

Rode: `web_dashboard/.venv/bin/python -m pytest web_dashboard/tests/test_plots.py -q`
Esperado: todos passam.

- [ ] **Passo 5: commitar**

```bash
git add web_dashboard/plots.py web_dashboard/tests/test_plots.py
git commit -m "feat: adiciona tema Asaas e os gráficos do dashboard"
```

---
## Tarefa 6: handlers das abas

**Arquivos:**
- Criar: `web_dashboard/handlers.py`
- Criar: `web_dashboard/tests/fakes.py` (cliente falso para os testes, usado também na Tarefa 7)
- Testar: `web_dashboard/tests/test_handlers.py`

**Interfaces:**
- Consumes: `client.BackendClient` e `client.BackendError` (Tarefa 2), `analysis` (Tarefa 3), `surrogate` (Tarefa 4) e `plots` (Tarefa 5).
- Produz (nada aqui importa Gradio, então tudo se testa sem abrir a tela):
  - Constantes: `MODE_COMPARE`, `MODE_NO_GRAPH`, `MODE_GRAPH` (os três modos da Aba 1), `TYPES` (os 5 tipos do PaySim), `TARGETS` (`{"Decisões do modelo": "model", "Gabarito (isFraud)": "truth"}`).
  - `handlers.UserMessage`: aviso para o usuário (a tela mostra como aviso, não como erro).
  - Cabeçalho: `run_label(summary)`, `finished_runs(client)`, `run_choices(runs)`, `default_pair(runs) -> (a_id, b_id)`, `overview_markdown(info)` e `initial_state(client) -> InitialState` (`overview`, `choices`, `a`, `b`, `models`, `default_model`).
  - Aba 1: `build_tab1(client, a_id, b_id, threshold) -> Tab1View` (`.as_tuple()` na ordem `notices, metrics, confusion, roc_pr, sweep, fp_fn, fraud_types, sweep_table, types_table`).
  - Aba 2: `build_tab2(client, a_id, b_id) -> Tab2View` (`.as_tuple()`: `notices, kpis, percentiles, hist_neo4j, hist_jev, table`).
  - Aba 3: `build_evaluate_payload(...)`, `sample_transaction(client, run_id, rng=None)` e `evaluate_view(client, payload) -> Tab3View` (`.as_tuple()`: `notice, verdict, features, timings, state, raw, subgraph`).
  - Aba 4: `build_tab4(client, run_id, target, depth, threshold) -> Tab4View` (`.as_tuple()`: `summary, importance, profiles, counts, tree_text, importance_table`).
  - Execução: `run_benchmarks(client, mode, params, poll_seconds=1.0, sleep=time.sleep) -> Iterator[RunUpdate]` (`RunUpdate(status, a, b, finished)`) e `cancel_active(client) -> str`.
  - `error_notice(exc: BackendError) -> str`.

Comportamentos que os testes pinam:
- Qualquer `BackendError` vira um aviso em texto na aba, com as figuras vazias. Nada estoura.
- Sem execução escolhida (primeiro uso), as abas dizem "rode um benchmark primeiro".
- A comparação da Aba 1 usa só as transações presentes nas duas execuções e avisa quando há poucas em comum, quando os modelos são diferentes, quando há erros e quando a amostra tem uma classe só.
- A precisão reprojetada usa a prevalência da Execução A.
- O modo "Comparar" roda primeiro **sem** grafo (vira a Execução A) e depois **com** grafo (vira a B), com a mesma seed.

- [ ] **Passo 1: escrever o cliente falso**

```python file=web_dashboard/tests/fakes.py
"""Cliente falso da API, para testar os handlers e a tela sem rede."""

from __future__ import annotations

import pandas as pd

from client import BackendError
from tests.conftest import make_results, make_summary


class FakeClient:
    """Guarda execuções em memória. `runs` é uma lista de (summary, DataFrame), da mais
    nova para a mais antiga. Para simular uma execução em andamento, `start_run` cria uma
    nova que responde `running` algumas vezes e depois `done`."""

    def __init__(self, runs=None, info=None, evaluate_response=None, running_polls=2):
        self._runs = {s["run_id"]: (s, df) for s, df in (runs or [])}
        self._order = [s["run_id"] for s, _ in (runs or [])]
        self._info = info if info is not None else default_info()
        self._evaluate_response = evaluate_response
        self.running_polls = running_polls
        self.started: list[dict] = []
        self.canceled: list[str] = []
        self.evaluated: list[dict] = []
        self.errors: dict[str, BackendError] = {}
        self._polls: dict[str, int] = {}
        self._new_runs: dict[str, tuple[dict, pd.DataFrame]] = {}
        self._next = 0
        self.final_status = "done"

    # ---- leitura
    def info(self):
        self._raise("info")
        return self._info

    def health(self):
        self._raise("health")
        return {"status": "ok"}

    def list_runs(self):
        self._raise("list_runs")
        return [self._runs[i][0] for i in self._order]

    def get_run(self, run_id):
        self._raise("get_run")
        if run_id in self._new_runs:
            summary, _ = self._new_runs[run_id]
            polls = self._polls.get(run_id, 0)
            self._polls[run_id] = polls + 1
            if polls < self.running_polls:
                return {
                    **summary,
                    "status": "running",
                    "progress": {"done": polls * 10, "total": 100},
                }
            final = {**summary, "status": self.final_status}
            if self.final_status == "failed":
                final["error"] = "o modelo não respondeu no aquecimento"
            return final
        return self._runs[run_id][0]

    def results(self, run_id):
        self._raise("results")
        if run_id in self._runs:
            return self._runs[run_id][1]
        return self._new_runs[run_id][1]

    # ---- ações
    def start_run(self, params):
        self._raise("start_run")
        self.started.append(params)
        self._next += 1
        run_id = f"run_20260930_17000{self._next}_00{self._next:02d}"
        graph = params.get("graph", True)
        summary = make_summary(
            run_id=run_id, graph=graph, model=params.get("model", "tev1:4b"), n=params.get("n", 400)
        )
        self._new_runs[run_id] = (summary, make_results(seed=7, graph=graph))
        self._runs[run_id] = self._new_runs[run_id]
        self._order.insert(0, run_id)
        return {**summary, "status": "running"}

    def cancel_run(self, run_id):
        self._raise("cancel_run")
        self.canceled.append(run_id)
        return {"canceled": True, "run_id": run_id}

    def evaluate(self, payload):
        self._raise("evaluate")
        self.evaluated.append(payload)
        return self._evaluate_response

    def _raise(self, method):
        if method in self.errors:
            raise self.errors[method]


def default_info(**extra):
    info = {
        "model": "tev1:4b",
        "base_url": "http://host.docker.internal:11434",
        "ollama_ok": True,
        "available_models": ["tev1:4b", "tev1:0.8b", "nimble:latest"],
        "dataset": {"total_rows": 6362620, "fraud_rows": 8213, "max_step": 743, "by_type": {}},
        "defaults": {
            "n": 2000,
            "fraud_pct": 10,
            "seed": 42,
            "workers": 32,
            "graph": True,
            "types": ["TRANSFER", "CASH_OUT"],
            "window_hours": 24,
            "cycle_max_hops": 3,
            "max_inflight": 4,
            "threshold": 0.5,
        },
        "limits": {"max_sample_size": 500000, "max_workers": 256},
        "active_run": None,
    }
    info.update(extra)
    return info


def two_runs():
    """Uma execução sem grafo (A) e uma com grafo (B), com as mesmas transações."""
    sem = make_summary(run_id="run_20260930_160000_aaaa", graph=False)
    com = make_summary(run_id="run_20260930_161000_bbbb", graph=True)
    return [(com, make_results(seed=7, graph=True)), (sem, make_results(seed=7, graph=False))]


def evaluate_response(**extra):
    resp = {
        "decision": "fraud",
        "is_fraud": True,
        "noul": 0.97,
        "threshold": 0.5,
        "model": "tev1:4b",
        "replay": True,
        "graph_features": {
            "first_transfer_between_pair": True,
            "dest_in_degree": 42,
            "dest_fan_in_window": 17,
            "dest_fan_out_window": 0,
            "orig_fan_out_window": 0,
            "dest_account_age_steps": 3,
            "cycle_detected": True,
        },
        "state": {"transaction_id": "TX_1029", "amount": 350000.0},
        "raw_response": {"answers": {"is_fraud": {"type": "noul", "noul": 0.97}}},
        "neo4j_ms": 7.3,
        "jev_ms": 71.5,
        "subgraph": {
            "nodes": ["C1", "C2", "X"],
            "edges": [
                {
                    "id": "e1",
                    "src": "C1",
                    "dst": "C2",
                    "type": "TRANSFER",
                    "amount": 10.0,
                    "step": 1,
                    "is_fraud": False,
                },
                {
                    "id": "e2",
                    "src": "X",
                    "dst": "C2",
                    "type": "TRANSFER",
                    "amount": 5.0,
                    "step": 2,
                    "is_fraud": True,
                },
            ],
        },
    }
    resp.update(extra)
    return resp
```

- [ ] **Passo 2: escrever os testes dos handlers (que falham)**

```python file=web_dashboard/tests/test_handlers.py
import random
import re

import numpy as np
import pandas as pd
import plotly.graph_objects as go
import pytest

import analysis
import handlers
import plots
from client import BackendError
from tests.conftest import make_results, make_summary
from tests.fakes import FakeClient, default_info, evaluate_response, two_runs


def textos(fig: go.Figure) -> list[str]:
    return [a.text for a in fig.layout.annotations]


# ---------------------------------------------------------------- cabeçalho


def test_run_label_e_run_choices_so_com_execucoes_terminadas():
    client = FakeClient(
        runs=two_runs()
        + [(make_summary(run_id="run_20260930_162000_cccc", status="failed"), pd.DataFrame())]
    )
    runs = handlers.finished_runs(client)
    assert [r["run_id"] for r in runs] == ["run_20260930_161000_bbbb", "run_20260930_160000_aaaa"]
    rotulo, valor = handlers.run_choices(runs)[0]
    assert valor == "run_20260930_161000_bbbb"
    assert (
        "com grafo" in rotulo
        and "tev1:4b" in rotulo
        and "n=400" in rotulo
        and rotulo.endswith("bbbb"[-4:])
    )
    assert "sem grafo" in handlers.run_label(runs[1])


def test_default_pair():
    com = make_summary(run_id="run_b", graph=True)
    sem = make_summary(run_id="run_a", graph=False)
    outra_com = make_summary(run_id="run_c", graph=True)
    assert handlers.default_pair([]) == (None, None)
    assert handlers.default_pair([com]) == (None, "run_b")
    assert handlers.default_pair([com, sem]) == ("run_a", "run_b")
    # A é a execução mais nova com grafo diferente da B, mesmo que haja outra no meio
    assert handlers.default_pair([com, outra_com, sem]) == ("run_a", "run_b")
    assert handlers.default_pair([com, outra_com]) == ("run_c", "run_b")


def test_overview_rotula_o_modelo_e_o_dataset():
    texto = handlers.overview_markdown(default_info())
    assert "estilo Jev" in texto and "Não é a Jev da TypeSafe" in texto
    assert "tev1:4b" in texto and "6.362.620" in texto and "8.213" in texto and "no ar" in texto

    fora = handlers.overview_markdown(
        default_info(ollama_ok=False, dataset=None, dataset_hint="Rode o seeder.")
    )
    assert "fora do ar" in fora and "Rode o seeder." in fora


def test_initial_state_sem_execucoes_e_com_api_fora():
    vazio = handlers.initial_state(FakeClient())
    assert vazio.choices == [] and vazio.a is None and vazio.b is None
    assert (
        vazio.models == ["tev1:4b", "tev1:0.8b", "nimble:latest"]
        and vazio.default_model == "tev1:4b"
    )

    client = FakeClient()
    client.errors["info"] = BackendError("Não consegui falar com a API", code="unreachable")
    fora = handlers.initial_state(client)
    assert "Não consegui falar com a API" in fora.overview and fora.choices == []
    assert fora.models == [] and fora.default_model is None

    com_execucoes = handlers.initial_state(FakeClient(runs=two_runs()))
    assert (
        com_execucoes.a == "run_20260930_160000_aaaa"
        and com_execucoes.b == "run_20260930_161000_bbbb"
    )
    assert len(com_execucoes.choices) == 2


# ---------------------------------------------------------------- aba 1


def test_aba1_comparacao_completa():
    client = FakeClient(runs=two_runs())
    v = handlers.build_tab1(client, "run_20260930_160000_aaaa", "run_20260930_161000_bbbb", 0.5)
    tupla = v.as_tuple()
    assert len(tupla) == 9
    tabela = v.metrics
    assert list(tabela.columns)[0] == "Métrica" and len(tabela.columns) == 4
    assert list(tabela.columns)[1].startswith("A (sem grafo") and list(tabela.columns)[
        2
    ].startswith("B (com grafo")
    assert list(tabela.columns)[3] == "Diferença (B menos A)"
    assert len(tabela) == 10
    assert any("reprojetada" in m for m in tabela["Métrica"])
    assert "Falsos positivos" in list(tabela["Métrica"])

    # os números da tabela são os de analysis, sobre as transações em comum
    a, b = analysis.align(two_runs()[1][1], two_runs()[0][1])
    conf_b = analysis.confusion(b, 0.5)
    linha = tabela[tabela["Métrica"] == "Falsos positivos"].iloc[0]
    assert linha.iloc[2] == str(conf_b["fp"])
    assert linha.iloc[3] == f"{conf_b['fp'] - analysis.confusion(a, 0.5)['fp']:+d}"

    assert (
        isinstance(v.confusion, go.Figure)
        and len([t for t in v.confusion.data if isinstance(t, go.Heatmap)]) == 2
    )
    assert len(v.sweep_table) == 2 * 101 and {"tipo", "fraudes"} <= set(
        v.types_table.columns
    )  # 101 limiares por execução
    assert "0,30%" in v.notices and "estratificada" in v.notices


def test_aba1_limiares_extremos_nao_quebram():
    client = FakeClient(runs=two_runs())

    def celula(v, metrica):
        return v.metrics[v.metrics["Métrica"] == metrica].iloc[0].iloc[1]

    tudo_fraude = handlers.build_tab1(
        client, "run_20260930_160000_aaaa", "run_20260930_161000_bbbb", 0.0
    )
    assert (
        celula(tudo_fraude, "Recall") == "100,00%"
        and celula(tudo_fraude, "Taxa de falso positivo") == "100,00%"
    )
    nada = handlers.build_tab1(client, "run_20260930_160000_aaaa", "run_20260930_161000_bbbb", 1.0)
    assert (
        celula(nada, "Recall") == "0,00%"
        and celula(nada, "Precisão") == "0,00%"
        and celula(nada, "F1") == "0,00%"
    )


def test_aba1_avisos_de_modelo_diferente_erros_e_intersecao_pequena():
    sem = make_summary(run_id="run_20260930_160000_aaaa", graph=False, model="tev1:0.8b")
    com = make_summary(run_id="run_20260930_161000_bbbb", graph=True, model="nimble")
    com["counts"]["errors"] = 5
    df_com = make_results(seed=7, graph=True).iloc[:100]
    client = FakeClient(runs=[(com, df_com), (sem, make_results(seed=7, graph=False))])
    v = handlers.build_tab1(client, sem["run_id"], com["run_id"], 0.5)
    assert "modelos diferentes" in v.notices and "tev1:0.8b" in v.notices and "nimble" in v.notices
    assert "5 transações com erro" in v.notices
    assert "100 transações" in v.notices  # só 100 em comum


def test_aba1_mesma_execucao_nos_dois_lados_e_so_uma_escolhida():
    client = FakeClient(runs=two_runs())
    v = handlers.build_tab1(client, "run_20260930_161000_bbbb", "run_20260930_161000_bbbb", 0.5)
    assert "mesma execução" in v.notices
    so_b = handlers.build_tab1(client, None, "run_20260930_161000_bbbb", 0.5)
    assert len(so_b.metrics.columns) == 2 and so_b.metrics.columns[1].startswith("B (com grafo")
    so_a = handlers.build_tab1(client, "run_20260930_160000_aaaa", None, 0.5)
    assert so_a.metrics.columns[1].startswith("A (sem grafo")


def test_aba1_amostra_de_uma_classe_so():
    df = make_results(seed=7, graph=True)
    df["is_fraud"] = False
    client = FakeClient(runs=[(make_summary(run_id="run_20260930_161000_bbbb"), df)])
    v = handlers.build_tab1(client, None, "run_20260930_161000_bbbb", 0.5)
    assert "uma classe" in v.notices
    assert any("classes" in t for t in textos(v.roc_pr))
    assert any("fraude" in t for t in textos(v.fraud_types))


def test_aba1_primeiro_uso_sem_execucao():
    v = handlers.build_tab1(FakeClient(), None, None, 0.5)
    assert "rode um benchmark" in v.notices.lower()
    assert v.metrics.empty and len(v.as_tuple()) == 9
    assert all(
        isinstance(f, go.Figure) for f in (v.confusion, v.roc_pr, v.sweep, v.fp_fn, v.fraud_types)
    )


def test_aba1_api_com_erro_vira_aviso():
    client = FakeClient(runs=two_runs())
    client.errors["results"] = BackendError(
        "só depois de terminar",
        code="run_in_progress",
        status=409,
        details=["execução ativa: run_x"],
    )
    v = handlers.build_tab1(client, "run_20260930_160000_aaaa", "run_20260930_161000_bbbb", 0.5)
    assert "só depois de terminar" in v.notices and "execução ativa: run_x" in v.notices
    assert v.metrics.empty


# ---------------------------------------------------------------- aba 2


def test_aba2_kpis_latencias_e_histogramas():
    client = FakeClient(runs=two_runs())
    v = handlers.build_tab2(client, "run_20260930_160000_aaaa", "run_20260930_161000_bbbb")
    assert len(v.as_tuple()) == 6
    assert v.kpis.count('class="kpi"') == 8  # 4 números para cada execução
    assert "25,0" in v.kpis and "1 min 00 s" in v.kpis and "40.000" in v.kpis
    assert "A (sem grafo" in v.kpis and "B (com grafo" in v.kpis
    assert list(v.table.columns)[:2] == ["Componente", "Percentil"] and len(v.table) == 6
    assert [t.name for t in v.hist_neo4j.data][0].startswith(
        "B (com grafo"
    )  # só B tem tempo do Neo4j
    assert len(v.hist_jev.data) == 2
    assert "tokens" in v.notices.lower()


def test_aba2_execucao_sem_grafo_deixa_o_grafico_do_neo4j_vazio():
    sem = make_summary(run_id="run_20260930_160000_aaaa", graph=False)
    client = FakeClient(runs=[(sem, make_results(seed=7, graph=False))])
    v = handlers.build_tab2(client, "run_20260930_160000_aaaa", None)
    assert any("grafo" in t for t in textos(v.hist_neo4j))
    assert len(v.hist_jev.data) == 1


def test_aba2_html_escapa_nomes_e_lida_com_ausencia():
    perigoso = make_summary(run_id="run_20260930_160000_aaaa", model="<script>x</script>")
    client = FakeClient(runs=[(perigoso, make_results(seed=7))])
    v = handlers.build_tab2(client, "run_20260930_160000_aaaa", None)
    assert "<script>" not in v.kpis and "&lt;script&gt;" in v.kpis
    vazio = handlers.build_tab2(FakeClient(), None, None)
    assert "rode um benchmark" in vazio.notices.lower() and vazio.table.empty


def test_fmt_duration():
    assert handlers.fmt_duration(0.61) == "0,6 s"
    assert handlers.fmt_duration(59.94) == "59,9 s"
    assert handlers.fmt_duration(60) == "1 min 00 s"
    assert handlers.fmt_duration(3725) == "62 min 05 s"


# ---------------------------------------------------------------- aba 3


def test_build_evaluate_payload():
    p = handlers.build_evaluate_payload(
        " C1 ", "C2", "TRANSFER", 350000, 31, 350000, 0, 0, 0, None, "nimble", 0.6
    )
    assert p == {
        "orig": "C1",
        "dest": "C2",
        "type": "TRANSFER",
        "amount": 350000.0,
        "step": 31,
        "orig_balance_before": 350000.0,
        "orig_balance_after": 0.0,
        "dest_balance_before": 0.0,
        "dest_balance_after": 0.0,
        "model": "nimble",
        "threshold": 0.6,
    }
    com_seq = handlers.build_evaluate_payload(
        "a", "b", "CASH_OUT", None, 5.0, None, None, None, None, 1029.0, "", 0.5
    )
    assert com_seq["seq"] == 1029 and isinstance(com_seq["seq"], int) and com_seq["step"] == 5
    assert com_seq["amount"] == 0.0 and "model" not in com_seq
    sem_step = handlers.build_evaluate_payload(
        "a", "b", "CASH_OUT", 1, None, 0, 0, 0, 0, None, None, 0.5
    )
    assert "step" not in sem_step and "seq" not in sem_step


def test_campos_step_e_seq_aceitam_texto_e_vazio():
    base = ("a", "b", "TRANSFER", 1, None, 0, 0, 0, 0, None, None, 0.5)
    vazio = handlers.build_evaluate_payload(*base[:4], "", *base[5:9], "  ", *base[10:])
    assert "step" not in vazio and "seq" not in vazio
    texto = handlers.build_evaluate_payload(*base[:4], "31", *base[5:9], "1029", *base[10:])
    assert texto["step"] == 31 and texto["seq"] == 1029
    virgula = handlers.build_evaluate_payload(*base[:4], "31,0", *base[5:9], None, *base[10:])
    assert virgula["step"] == 31
    for ruim in ("abc", "3.5", "1,5"):
        with pytest.raises(handlers.UserMessage) as info:
            handlers.build_evaluate_payload(*base[:4], ruim, *base[5:])
        assert "número inteiro" in str(info.value)


def test_diferenca_em_pontos_percentuais_usa_virgula_so_no_numero():
    assert handlers.fmt_pp(-0.0213) == "-2,13 p.p."
    assert handlers.fmt_pp(0.0567) == "+5,67 p.p."
    client = FakeClient(runs=two_runs())
    v = handlers.build_tab1(client, "run_20260930_160000_aaaa", "run_20260930_161000_bbbb", 0.5)
    pp = v.metrics["Diferença (B menos A)"].iloc[:6]
    assert all(t.endswith(" p.p.") and "p,p" not in t for t in pp)


def test_sample_transaction_devolve_uma_linha_real_da_execucao():
    client = FakeClient(runs=two_runs())
    df = make_results(seed=7, graph=False)
    valores = handlers.sample_transaction(client, "run_20260930_160000_aaaa", rng=random.Random(1))
    orig, dest, tipo, amount, step, ob, oa, db, da, seq = valores
    linha = df[df["seq"] == seq].iloc[0]
    assert (orig, dest, tipo) == (linha["orig"], linha["dest"], linha["type"])
    assert (
        amount == pytest.approx(linha["amount"]) and step == linha["step"] and isinstance(seq, int)
    )
    assert ob == pytest.approx(linha["orig_balance_before"]) and da == pytest.approx(
        linha["dest_balance_after"]
    )
    assert (
        handlers.sample_transaction(client, "run_20260930_160000_aaaa", rng=random.Random(1))
        == valores
    )


def test_sample_transaction_sem_execucao_ou_vazia_vira_aviso():
    with pytest.raises(handlers.UserMessage):
        handlers.sample_transaction(FakeClient(runs=two_runs()), None)
    vazia = make_summary(run_id="run_20260930_160000_aaaa")
    client = FakeClient(runs=[(vazia, make_results(n_errors=400, n=400))])
    with pytest.raises(handlers.UserMessage):
        handlers.sample_transaction(client, "run_20260930_160000_aaaa")


def test_evaluate_view_replay_com_fraude():
    client = FakeClient(evaluate_response=evaluate_response())
    payload = handlers.build_evaluate_payload(
        "C1", "C2", "TRANSFER", 10, 1, 10, 0, 0, 0, 1029, None, 0.5
    )
    v = handlers.evaluate_view(client, payload)
    assert len(v.as_tuple()) == 7 and v.notice == ""
    assert "verdict-fraud" in v.verdict and "Fraude" in v.verdict and "0,970" in v.verdict
    assert "replay" in v.verdict
    assert "Ciclo detectado" in v.features and "Sim" in v.features and "42" in v.features
    assert "3 horas" in v.features
    assert "7,3 ms" in v.timings and "71,5 ms" in v.timings
    assert v.state == {"transaction_id": "TX_1029", "amount": 350000.0}
    assert v.raw["answers"]["is_fraud"]["noul"] == 0.97
    assert isinstance(v.subgraph, go.Figure) and len(v.subgraph.data) >= 4
    assert client.evaluated == [payload]


def test_evaluate_view_legitima_conta_nova_e_modo_agora():
    resp = evaluate_response(
        decision="legit",
        is_fraud=False,
        noul=0.12,
        replay=False,
        raw_response=None,
        graph_features={
            "first_transfer_between_pair": True,
            "dest_in_degree": 0,
            "dest_fan_in_window": 0,
            "dest_fan_out_window": 0,
            "orig_fan_out_window": 0,
            "dest_account_age_steps": None,
            "cycle_detected": False,
        },
    )
    v = handlers.evaluate_view(FakeClient(evaluate_response=resp), {"orig": "C1", "dest": "C2"})
    assert (
        "verdict-legit" in v.verdict
        and "Legítima" in v.verdict
        and "transação de agora" in v.verdict
    )
    assert "nunca apareceu" in v.features and "Não" in v.features
    assert v.raw == {}


def test_evaluate_view_erro_da_api():
    client = FakeClient()
    client.errors["evaluate"] = BackendError(
        "pedido inválido", code="validation_error", status=400, details=["orig não pode ser vazio"]
    )
    v = handlers.evaluate_view(client, {})
    assert "pedido inválido" in v.notice and "orig não pode ser vazio" in v.notice
    assert v.verdict == "" and v.state == {} and isinstance(v.subgraph, go.Figure)

    client.errors["evaluate"] = BackendError("Não consegui falar com a API", code="unreachable")
    assert "Não consegui falar com a API" in handlers.evaluate_view(client, {}).notice


# ---------------------------------------------------------------- aba 4


def test_aba4_perfis_do_modelo():
    client = FakeClient(runs=two_runs())
    v = handlers.build_tab4(client, "run_20260930_161000_bbbb", "model", 3, 0.5)
    assert len(v.as_tuple()) == 6
    assert list(v.profiles.columns) == [
        "Perfil",
        "Regra",
        "Transações",
        "% da amostra",
        "% fraude (modelo)",
        "% fraude (real)",
        "Árvore decide",
    ]
    assert 2 <= len(v.profiles) <= 8 and v.profiles["Transações"].sum() == 400
    assert (
        "Fidelidade da árvore" in v.summary and "estratificada" in v.summary and "25%" in v.summary
    )
    assert re.search(r"\*\*\d+,\d%\*\*", v.summary)  # fidelidade com vírgula decimal
    assert "|---" in v.tree_text
    assert list(v.importance_table.columns) == ["Feature", "Importância (%)"]
    assert isinstance(v.importance, go.Figure) and isinstance(v.counts, go.Figure)


def test_aba4_alvo_gabarito_e_profundidade_invalida():
    client = FakeClient(runs=two_runs())
    v = handlers.build_tab4(client, "run_20260930_161000_bbbb", "truth", 3, 0.5)
    assert "gabarito" in v.summary
    with pytest.raises(ValueError):
        handlers.build_tab4(client, "run_20260930_161000_bbbb", "model", 9, 0.5)


def test_aba4_avisa_quando_explica_mal():
    df = make_results(n=600, seed=2)
    df["noul"] = np.random.default_rng(0).random(len(df))
    client = FakeClient(runs=[(make_summary(run_id="run_20260930_161000_bbbb", n=600), df)])
    v = handlers.build_tab4(client, "run_20260930_161000_bbbb", "model", 2, 0.5)
    assert "explica mal" in v.summary and "não confie" in v.summary


def test_aba4_sem_execucao_uma_classe_e_erro_da_api():
    v = handlers.build_tab4(FakeClient(), None, "model", 3, 0.5)
    assert "rode um benchmark" in v.summary.lower() and v.profiles.empty

    df = make_results(n=200)
    df["noul"] = 0.1
    client = FakeClient(runs=[(make_summary(run_id="run_20260930_161000_bbbb", n=200), df)])
    v = handlers.build_tab4(client, "run_20260930_161000_bbbb", "model", 3, 0.5)
    assert "mesma classe" in v.summary and v.profiles.empty

    client.errors["results"] = BackendError("caiu", code="unreachable")
    assert (
        "caiu" in handlers.build_tab4(client, "run_20260930_161000_bbbb", "model", 3, 0.5).summary
    )


# ---------------------------------------------------------------- execução em segundo plano


PARAMS = {"n": 100, "fraud_pct": 10, "seed": 7, "model": "tev1:0.8b", "types": ["TRANSFER"]}


def ultimo(updates):
    return updates[-1]


def test_comparar_roda_sem_grafo_depois_com_grafo_e_devolve_os_ids():
    client = FakeClient(running_polls=2)
    pausas = []
    updates = list(
        handlers.run_benchmarks(
            client, handlers.MODE_COMPARE, PARAMS, poll_seconds=0.5, sleep=pausas.append
        )
    )
    assert [p["graph"] for p in client.started] == [False, True]
    assert all(p["seed"] == 7 and p["model"] == "tev1:0.8b" for p in client.started)
    fim = ultimo(updates)
    assert (
        fim.finished and fim.a == "run_20260930_170001_0001" and fim.b == "run_20260930_170002_0002"
    )
    andamento = [u for u in updates if not u.finished]
    assert len(andamento) == 4 and all(not u.a and not u.b for u in andamento)
    assert "sem grafo" in andamento[0].status and "com grafo" in andamento[-1].status
    assert "Execução 1 de 2" in andamento[0].status and "%" in andamento[1].status
    assert pausas == [0.5] * 4


def test_modos_de_uma_execucao_so():
    client = FakeClient(running_polls=0)
    fim = ultimo(
        list(handlers.run_benchmarks(client, handlers.MODE_GRAPH, PARAMS, sleep=lambda _: None))
    )
    assert fim.a is None and fim.b and client.started[0]["graph"] is True
    client = FakeClient(running_polls=0)
    fim = ultimo(
        list(handlers.run_benchmarks(client, handlers.MODE_NO_GRAPH, PARAMS, sleep=lambda _: None))
    )
    assert fim.b is None and fim.a and client.started[0]["graph"] is False


def test_execucao_em_andamento_da_409_e_mostra_a_mensagem():
    client = FakeClient()
    client.errors["start_run"] = BackendError(
        "já existe uma execução em andamento", code="run_in_progress", status=409
    )
    updates = list(
        handlers.run_benchmarks(client, handlers.MODE_COMPARE, PARAMS, sleep=lambda _: None)
    )
    assert len(updates) == 1 and updates[0].finished and not updates[0].a and not updates[0].b
    assert "já existe uma execução em andamento" in updates[0].status


def test_execucao_que_falha_para_e_guarda_o_que_ja_terminou():
    client = FakeClient(running_polls=0)
    client.final_status = "failed"
    fim = ultimo(
        list(handlers.run_benchmarks(client, handlers.MODE_COMPARE, PARAMS, sleep=lambda _: None))
    )
    assert fim.finished and fim.a is None and fim.b is None
    assert "failed" in fim.status and "aquecimento" in fim.status
    assert len(client.started) == 1  # não tentou a segunda


def test_erro_de_rede_durante_o_acompanhamento():
    client = FakeClient(running_polls=5)
    original = client.get_run

    def get_run(run_id):
        if client._polls.get(run_id, 0) >= 1:
            raise BackendError("Não consegui falar com a API", code="unreachable")
        return original(run_id)

    client.get_run = get_run
    fim = ultimo(
        list(handlers.run_benchmarks(client, handlers.MODE_GRAPH, PARAMS, sleep=lambda _: None))
    )
    assert fim.finished and "Não consegui falar com a API" in fim.status


def test_cancel_active():
    client = FakeClient(info=default_info(active_run="run_20260930_170001_0001"))
    assert "run_20260930_170001_0001" in handlers.cancel_active(client)
    assert client.canceled == ["run_20260930_170001_0001"]
    assert "Nenhuma execução" in handlers.cancel_active(FakeClient())
    erro = FakeClient(info=default_info(active_run="run_x"))
    erro.errors["cancel_run"] = BackendError(
        "execução não encontrada", code="not_found", status=404
    )
    assert "execução não encontrada" in handlers.cancel_active(erro)


def test_error_notice_inclui_detalhes():
    texto = handlers.error_notice(
        BackendError("pedido inválido", details=["n precisa ser 1 ou mais", "workers alto"])
    )
    assert (
        "pedido inválido" in texto
        and "n precisa ser 1 ou mais" in texto
        and "workers alto" in texto
    )
    assert handlers.error_notice(BackendError("só isso")).count(
        "\n"
    ) == 0 or "só isso" in handlers.error_notice(BackendError("só isso"))


def test_cores_das_execucoes_ficam_fixas_por_letra():
    client = FakeClient(runs=two_runs())
    so_b = handlers.build_tab1(client, None, "run_20260930_161000_bbbb", 0.5)
    barras_b = [t for t in so_b.fp_fn.data]
    assert barras_b[0].marker.color == plots.COLOR_B  # B continua salmão mesmo sozinha
    ambas = handlers.build_tab1(client, "run_20260930_160000_aaaa", "run_20260930_161000_bbbb", 0.5)
    assert [t.marker.color for t in ambas.fp_fn.data] == [plots.COLOR_A, plots.COLOR_B]
```

- [ ] **Passo 3: rodar e ver falhar**

Rode: `web_dashboard/.venv/bin/python -m pytest web_dashboard/tests/test_handlers.py -q`
Esperado: FAIL com `ModuleNotFoundError: No module named 'handlers'`.

- [ ] **Passo 4: implementar**

```python file=web_dashboard/handlers.py
"""Uma função pura por aba. Recebem o cliente da API e devolvem o que a tela mostra.
Nada aqui importa Gradio, então dá para testar tudo sem abrir a tela."""

from __future__ import annotations

import html
import random
import time
from collections.abc import Iterator
from dataclasses import dataclass, field

import pandas as pd
import plotly.graph_objects as go

import analysis
import plots
import surrogate
from client import BackendClient, BackendError

MODE_COMPARE = "Comparar com e sem grafo"
MODE_NO_GRAPH = "Só sem grafo"
MODE_GRAPH = "Só com grafo"
TYPES = ["TRANSFER", "CASH_OUT", "PAYMENT", "DEBIT", "CASH_IN"]
TARGETS = {"Decisões do modelo": "model", "Gabarito (isFraud)": "truth"}
LETTERS = ("A", "B")


class UserMessage(Exception):
    """Aviso para o usuário. A tela mostra como aviso e não como erro."""


# ------------------------------------------------------------------ formatação


def fmt_pct(valor: float) -> str:
    return f"{valor * 100:.2f}%".replace(".", ",")


def fmt_pp(diferenca: float) -> str:
    """Diferença em pontos percentuais, com sinal e vírgula decimal (só no número)."""
    return f"{diferenca * 100:+.2f}".replace(".", ",") + " p.p."


def fmt_num(valor: float, casas: int = 1) -> str:
    return f"{valor:,.{casas}f}".replace(",", "X").replace(".", ",").replace("X", ".")


def fmt_int(valor: float) -> str:
    return f"{int(round(valor)):,}".replace(",", ".")


def fmt_duration(segundos: float) -> str:
    if segundos < 60:
        return f"{segundos:.1f} s".replace(".", ",")
    minutos, resto = divmod(int(round(segundos)), 60)
    return f"{minutos} min {resto:02d} s"


def error_notice(exc: BackendError) -> str:
    texto = f"**Erro:** {exc}"
    if exc.details:
        texto += "\n\n" + "\n".join(f"- {d}" for d in exc.details)
    return texto


# ------------------------------------------------------------------ cabeçalho


def graph_label(summary: dict) -> str:
    return "com grafo" if summary["params"]["graph"] else "sem grafo"


def series_name(letter: str, summary: dict) -> str:
    return f"{letter} ({graph_label(summary)}, {summary['params']['model']})"


def run_label(summary: dict) -> str:
    inicio = summary["started_at"][:16].replace("T", " ")
    n = (summary.get("sample") or {}).get("n", "?")
    return f"{inicio} | {summary['params']['model']} | {graph_label(summary)} | n={n} | {summary['run_id'][-4:]}"


def finished_runs(client: BackendClient) -> list[dict]:
    return [r for r in client.list_runs() if r.get("status") == "done"]


def run_choices(runs: list[dict]) -> list[tuple[str, str]]:
    return [(run_label(r), r["run_id"]) for r in runs]


def default_pair(runs: list[dict]) -> tuple[str | None, str | None]:
    """B é a execução mais nova. A é a mais nova das outras com o grafo diferente (e, na
    falta dela, a segunda mais nova)."""
    if not runs:
        return None, None
    b = runs[0]
    if len(runs) == 1:
        return None, b["run_id"]
    for r in runs[1:]:
        if r["params"]["graph"] != b["params"]["graph"]:
            return r["run_id"], b["run_id"]
    return runs[1]["run_id"], b["run_id"]


def overview_markdown(info: dict) -> str:
    estado = "no ar" if info.get("ollama_ok") else "fora do ar"
    linhas = [
        f"**Modelo:** {info['model']}, um modelo aberto estilo Jev rodando no Ollama "
        f"({info['base_url']}, {estado}). Não é a Jev da TypeSafe.",
    ]
    dataset = info.get("dataset")
    if dataset:
        linhas.append(
            f"**Dataset:** {fmt_int(dataset['total_rows'])} transações, {fmt_int(dataset['fraud_rows'])} fraudes, "
            f"maior step {dataset['max_step']}."
        )
    else:
        linhas.append(f"**Dataset:** {info.get('dataset_hint', 'ainda não foi carregado.')}")
    if info.get("active_run"):
        linhas.append(f"**Execução em andamento:** {info['active_run']}")
    return "\n\n".join(linhas)


@dataclass
class InitialState:
    overview: str
    choices: list[tuple[str, str]] = field(default_factory=list)
    a: str | None = None
    b: str | None = None
    models: list[str] = field(default_factory=list)
    default_model: str | None = None


def initial_state(client: BackendClient) -> InitialState:
    """O que a tela precisa ao abrir (ou ao atualizar). Se a API estiver fora, devolve o
    aviso no lugar do resumo, com os seletores vazios."""
    try:
        info = client.info()
        runs = finished_runs(client)
    except BackendError as exc:
        return InitialState(overview=error_notice(exc))
    a, b = default_pair(runs)
    return InitialState(
        overview=overview_markdown(info),
        choices=run_choices(runs),
        a=a,
        b=b,
        models=list(info.get("available_models") or []),
        default_model=info.get("model"),
    )


# ------------------------------------------------------------------ carregamento


@dataclass
class Loaded:
    summary: dict
    df: pd.DataFrame


def _load(client: BackendClient, run_id: str | None) -> Loaded | None:
    if not run_id:
        return None
    return Loaded(client.get_run(run_id), client.results(run_id))


def _load_selected(client: BackendClient, a_id: str | None, b_id: str | None) -> dict[str, Loaded]:
    """As execuções escolhidas, indexadas pela letra (A ou B)."""
    escolhidas = {}
    for letra, run_id in zip(LETTERS, (a_id, b_id), strict=True):
        carregada = _load(client, run_id)
        if carregada is not None:
            escolhidas[letra] = carregada
    return escolhidas


NO_RUN = "Escolha uma execução nos seletores acima ou rode um benchmark primeiro."


# ------------------------------------------------------------------ aba 1


def _empty_figs(mensagem: str) -> list[go.Figure]:
    return [plots.empty_figure(mensagem) for _ in range(5)]


@dataclass
class Tab1View:
    notices: str
    metrics: pd.DataFrame
    confusion: go.Figure
    roc_pr: go.Figure
    sweep: go.Figure
    fp_fn: go.Figure
    fraud_types: go.Figure
    sweep_table: pd.DataFrame
    types_table: pd.DataFrame

    def as_tuple(self) -> tuple:
        return (
            self.notices,
            self.metrics,
            self.confusion,
            self.roc_pr,
            self.sweep,
            self.fp_fn,
            self.fraud_types,
            self.sweep_table,
            self.types_table,
        )

    @classmethod
    def empty(cls, aviso: str) -> Tab1View:
        return cls(
            aviso,
            pd.DataFrame(),
            *_empty_figs("Sem dados para mostrar."),
            pd.DataFrame(),
            pd.DataFrame(),
        )


def metrics_table(confs: dict[str, dict], prevalence: float) -> pd.DataFrame:
    nomes = list(confs)
    ms = {n: analysis.metrics(c, prevalence) for n, c in confs.items()}
    porcentagens = [
        ("Acurácia", "accuracy"),
        ("Precisão", "precision"),
        ("Recall", "recall"),
        ("F1", "f1"),
        ("Taxa de falso positivo", "fpr"),
        (
            f"Precisão reprojetada (prevalência real de {fmt_pct(prevalence)})",
            "precision_at_prevalence",
        ),
    ]
    contagens = [
        ("Verdadeiros positivos", "tp"),
        ("Falsos positivos", "fp"),
        ("Verdadeiros negativos", "tn"),
        ("Falsos negativos", "fn"),
    ]
    linhas = []
    for rotulo, chave in porcentagens:
        linha = {"Métrica": rotulo, **{n: fmt_pct(ms[n][chave]) for n in nomes}}
        if len(nomes) == 2:
            linha["Diferença (B menos A)"] = fmt_pp(ms[nomes[1]][chave] - ms[nomes[0]][chave])
        linhas.append(linha)
    for rotulo, chave in contagens:
        linha = {"Métrica": rotulo, **{n: str(confs[n][chave]) for n in nomes}}
        if len(nomes) == 2:
            linha["Diferença (B menos A)"] = f"{confs[nomes[1]][chave] - confs[nomes[0]][chave]:+d}"
        linhas.append(linha)
    return pd.DataFrame(linhas)


def build_tab1(
    client: BackendClient, a_id: str | None, b_id: str | None, threshold: float
) -> Tab1View:
    try:
        escolhidas = _load_selected(client, a_id, b_id)
    except BackendError as exc:
        return Tab1View.empty(error_notice(exc))
    if not escolhidas:
        return Tab1View.empty(NO_RUN)

    if len(escolhidas) == 2:
        a_df, b_df = analysis.align(escolhidas["A"].df, escolhidas["B"].df)
        quadros = {"A": a_df, "B": b_df}
    else:
        quadros = {letra: analysis.scored(carregada.df) for letra, carregada in escolhidas.items()}

    nomes = {letra: series_name(letra, c.summary) for letra, c in escolhidas.items()}
    cores = {nomes[letra]: plots.run_color(LETTERS.index(letra)) for letra in escolhidas}
    primeira = next(iter(escolhidas))
    prevalencia = analysis.prevalence_of(escolhidas[primeira].summary, quadros[primeira])

    confs = {nomes[letra]: analysis.confusion(q, threshold) for letra, q in quadros.items()}
    curvas = {nomes[letra]: analysis.roc_pr(q, prevalencia) for letra, q in quadros.items()}
    varreduras = {
        nomes[letra]: analysis.threshold_sweep(q, prevalencia) for letra, q in quadros.items()
    }
    tipos = {nomes[letra]: analysis.fraud_types(q, threshold) for letra, q in quadros.items()}

    tabela_varredura = pd.concat(
        [
            v.assign(execucao=n)[
                ["execucao", "limiar", "precision", "recall", "f1", "fpr", "fp", "fn"]
            ]
            for n, v in varreduras.items()
        ],
        ignore_index=True,
    )
    tabela_tipos = pd.concat([t.assign(execucao=n) for n, t in tipos.items()], ignore_index=True)

    return Tab1View(
        notices=_tab1_notices(escolhidas, quadros, nomes, a_id, b_id, prevalencia),
        metrics=metrics_table(confs, prevalencia),
        confusion=plots.confusion_figure(confs),
        roc_pr=plots.roc_pr_figure(curvas, cores, prevalencia),
        sweep=plots.threshold_sweep_figure(varreduras, cores, threshold),
        fp_fn=plots.fp_fn_figure(confs, cores),
        fraud_types=plots.fraud_types_figure(tipos, cores),
        sweep_table=tabela_varredura,
        types_table=tabela_tipos,
    )


def _tab1_notices(escolhidas, quadros, nomes, a_id, b_id, prevalencia) -> str:
    avisos = []
    if a_id and a_id == b_id:
        avisos.append("A e B são a mesma execução, então as duas colunas são iguais.")
    if len(escolhidas) == 2:
        modelos = {c.summary["params"]["model"] for c in escolhidas.values()}
        if len(modelos) > 1:
            ma, mb = (escolhidas[x].summary["params"]["model"] for x in LETTERS)
            avisos.append(
                f"As execuções usam modelos diferentes ({ma} e {mb}). A diferença mistura o efeito do modelo "
                "e o do grafo."
            )
        comuns = len(quadros["A"])
        tamanhos = [len(analysis.scored(escolhidas[x].df)) for x in LETTERS]
        if comuns == 0:
            avisos.append(
                "Não há nenhuma transação em comum. Rode as duas execuções com a mesma seed."
            )
        elif comuns < max(tamanhos):
            avisos.append(
                f"Só {comuns} transações aparecem nas duas execuções (A tem {tamanhos[0]} e B tem {tamanhos[1]}). "
                "A comparação usa só essas. Rode as duas com a mesma seed para comparar as mesmas transações."
            )
        prevs = [analysis.prevalence_of(escolhidas[x].summary) for x in LETTERS]
        if abs(prevs[0] - prevs[1]) > 1e-12:
            avisos.append(
                "As execuções têm prevalência real diferente. A precisão reprojetada usa a da execução A."
            )
    for letra, carregada in escolhidas.items():
        erros = (carregada.summary.get("counts") or {}).get("errors", 0)
        if erros:
            avisos.append(
                f"A execução {letra} teve {erros} transações com erro. Elas ficaram fora das métricas."
            )
    for letra, quadro in quadros.items():
        if len(quadro) and quadro["is_fraud"].nunique() < 2:
            avisos.append(
                f"A amostra da execução {letra} tem uma classe só (sem fraudes ou só fraudes). "
                "ROC e PR ficam de fora."
            )
    avisos.append(
        "A amostra é estratificada (tem bem mais fraude que a vida real). Recall e taxa de falso positivo não "
        f"mudam com isso. A precisão reprojetada usa a prevalência real de {fmt_pct(prevalencia)}."
    )
    return "\n\n".join(f"- {a}" for a in avisos)


# ------------------------------------------------------------------ aba 2

KPI_CSS_ROW = "kpi-row"


def _kpi(rotulo: str, valor: str) -> str:
    return (
        f'<div class="kpi"><div class="kpi-label">{html.escape(rotulo)}</div>'
        f'<div class="kpi-value">{html.escape(valor)}</div></div>'
    )


def kpi_html(itens: list[tuple[str, str, dict]]) -> str:
    """Um bloco de números por execução: (nome, cor, resumo)."""
    blocos = []
    for nome, cor, resumo in itens:
        erros = (resumo.get("counts") or {}).get("errors", 0)
        tiles = "".join(
            [
                _kpi("Transações por segundo", fmt_num(resumo["tps"])),
                _kpi("Tempo total", fmt_duration(resumo["duration_s"])),
                _kpi("Erros", str(erros)),
                _kpi("Tokens de entrada", fmt_int(resumo["tokens_total"])),
            ]
        )
        blocos.append(
            f'<div class="kpi-title"><span class="kpi-dot" style="background:{cor}"></span>'
            f'{html.escape(nome)}</div><div class="{KPI_CSS_ROW}">{tiles}</div>'
        )
    return "".join(blocos)


@dataclass
class Tab2View:
    notices: str
    kpis: str
    percentiles: go.Figure
    hist_neo4j: go.Figure
    hist_jev: go.Figure
    table: pd.DataFrame

    def as_tuple(self) -> tuple:
        return (
            self.notices,
            self.kpis,
            self.percentiles,
            self.hist_neo4j,
            self.hist_jev,
            self.table,
        )

    @classmethod
    def empty(cls, aviso: str) -> Tab2View:
        figs = _empty_figs("Sem dados para mostrar.")
        return cls(aviso, "", figs[0], figs[1], figs[2], pd.DataFrame())


def build_tab2(client: BackendClient, a_id: str | None, b_id: str | None) -> Tab2View:
    try:
        escolhidas = _load_selected(client, a_id, b_id)
    except BackendError as exc:
        return Tab2View.empty(error_notice(exc))
    if not escolhidas:
        return Tab2View.empty(NO_RUN)

    nomes = {letra: series_name(letra, c.summary) for letra, c in escolhidas.items()}
    cores = {nomes[letra]: plots.run_color(LETTERS.index(letra)) for letra in escolhidas}
    latencias = {nomes[letra]: c.summary["latency"] for letra, c in escolhidas.items()}
    quadros = {nomes[letra]: analysis.scored(c.df) for letra, c in escolhidas.items()}

    linhas = []
    for rotulo, chave in (
        ("Neo4j (consulta Cypher)", "neo4j_ms"),
        ("Modelo (inferência)", "jev_ms"),
    ):
        for p in ("p50", "p95", "p99"):
            linhas.append(
                {
                    "Componente": rotulo,
                    "Percentil": p.upper(),
                    **{n: f"{fmt_num(lat[chave][p])} ms" for n, lat in latencias.items()},
                }
            )

    kpis = kpi_html(
        [
            (nomes[letra], plots.run_color(LETTERS.index(letra)), c.summary)
            for letra, c in escolhidas.items()
        ]
    )
    return Tab2View(
        notices=_tab2_notices(escolhidas, nomes),
        kpis=kpis,
        percentiles=plots.latency_percentiles_figure(latencias, cores),
        hist_neo4j=plots.latency_histogram_figure(
            quadros,
            cores,
            "neo4j_ms",
            "Distribuição da latência do Neo4j",
            latencias_por_chave(latencias, "neo4j_ms"),
        ),
        hist_jev=plots.latency_histogram_figure(
            quadros,
            cores,
            "jev_ms",
            "Distribuição da latência do modelo",
            latencias_por_chave(latencias, "jev_ms"),
        ),
        table=pd.DataFrame(linhas),
    )


def latencias_por_chave(latencias: dict, chave: str) -> dict:
    return {nome: lat[chave] for nome, lat in latencias.items()}


def _tab2_notices(escolhidas: dict[str, Loaded], nomes: dict[str, str]) -> str:
    if len(escolhidas) < 2:
        return ""
    por_transacao = {}
    for letra, c in escolhidas.items():
        contagens = c.summary.get("counts") or {}
        feitas = sum(contagens.get(k, 0) for k in ("tp", "fp", "tn", "fn")) or 1
        por_transacao[letra] = c.summary["tokens_total"] / feitas
    p50 = {letra: c.summary["latency"]["jev_ms"]["p50"] for letra, c in escolhidas.items()}
    return (
        f"- Tokens de entrada por transação: A {fmt_num(por_transacao['A'], 0)} e B {fmt_num(por_transacao['B'], 0)}. "
        "Um state maior (com as features de grafo) pode deixar o modelo mais lento. "
        f"P50 do modelo: A {fmt_num(p50['A'])} ms e B {fmt_num(p50['B'])} ms."
    )


# ------------------------------------------------------------------ aba 3

FEATURE_TILES = [
    ("cycle_detected", "Ciclo detectado", "yes_no"),
    ("first_transfer_between_pair", "Primeira transferência entre o par", "yes_no"),
    ("dest_in_degree", "Conexões do destino (remetentes distintos no histórico)", "int"),
    ("dest_fan_in_window", "Remetentes ao destino na janela", "int"),
    ("dest_fan_out_window", "Destinatários do destino na janela", "int"),
    ("orig_fan_out_window", "Destinatários da origem na janela", "int"),
    ("dest_account_age_steps", "Idade da conta destino", "age"),
]


def _format_feature(valor, tipo: str) -> str:
    if tipo == "yes_no":
        return "Sim" if valor else "Não"
    if tipo == "age":
        return "conta nova (nunca apareceu antes)" if valor is None else f"{valor} horas"
    return str(valor)


def features_html(features: dict) -> str:
    tiles = "".join(
        _kpi(rotulo, _format_feature(features.get(chave), tipo))
        for chave, rotulo, tipo in FEATURE_TILES
    )
    return f'<div class="{KPI_CSS_ROW}">{tiles}</div>'


def verdict_html(resp: dict) -> str:
    tipo = "fraud" if resp["is_fraud"] else "legit"
    palavra = "Fraude" if resp["is_fraud"] else "Legítima"
    origem = "replay de uma transação real" if resp["replay"] else "transação de agora"
    meta = f"noul {fmt_num(resp['noul'], 3)} | limiar {fmt_num(resp['threshold'], 2)} | modelo {resp['model']} | {origem}"
    return (
        f'<div class="verdict verdict-{tipo}"><span class="verdict-word">{palavra}</span>'
        f'<span class="verdict-meta">{html.escape(meta)}</span></div>'
    )


def _optional_int(valor, nome: str) -> int | None:
    """Campo inteiro opcional. Vazio vira None. Aceita número ou texto (o formulário usa
    caixas de texto para step e seq, para o vazio ficar realmente vazio)."""
    if valor is None or (isinstance(valor, str) and not valor.strip()):
        return None
    try:
        numero = float(str(valor).strip().replace(",", "."))
    except ValueError:
        raise UserMessage(f"{nome} precisa ser um número inteiro, veio {valor!r}.") from None
    if not numero.is_integer():
        raise UserMessage(f"{nome} precisa ser um número inteiro, veio {valor!r}.")
    return int(numero)


def build_evaluate_payload(
    orig,
    dest,
    tipo,
    amount,
    step,
    orig_before,
    orig_after,
    dest_before,
    dest_after,
    seq,
    model,
    threshold,
) -> dict:
    """Monta o corpo do POST /transactions/evaluate a partir dos campos da tela."""
    payload = {
        "orig": (orig or "").strip(),
        "dest": (dest or "").strip(),
        "type": tipo,
        "amount": float(amount or 0),
        "orig_balance_before": float(orig_before or 0),
        "orig_balance_after": float(orig_after or 0),
        "dest_balance_before": float(dest_before or 0),
        "dest_balance_after": float(dest_after or 0),
        "threshold": float(threshold),
    }
    passo = _optional_int(step, "step")
    if passo is not None:
        payload["step"] = passo
    replay = _optional_int(seq, "seq")
    if replay is not None:
        payload["seq"] = replay
    if model:
        payload["model"] = model
    return payload


def sample_transaction(
    client: BackendClient, run_id: str | None, rng: random.Random | None = None
) -> tuple:
    """Sorteia uma transação real da execução para preencher o formulário. Devolve
    (orig, dest, tipo, valor, step, saldos..., seq)."""
    if not run_id:
        raise UserMessage("Escolha a Execução A para sortear uma transação dela.")
    df = analysis.scored(client.results(run_id))
    if df.empty:
        raise UserMessage("Essa execução não tem nenhuma transação respondida para sortear.")
    linha = df.iloc[(rng or random).randrange(len(df))]
    return (
        str(linha["orig"]),
        str(linha["dest"]),
        str(linha["type"]),
        float(linha["amount"]),
        int(linha["step"]),
        float(linha["orig_balance_before"]),
        float(linha["orig_balance_after"]),
        float(linha["dest_balance_before"]),
        float(linha["dest_balance_after"]),
        int(linha["seq"]),
    )


@dataclass
class Tab3View:
    notice: str
    verdict: str
    features: str
    timings: str
    state: dict
    raw: dict
    subgraph: go.Figure

    def as_tuple(self) -> tuple:
        return (
            self.notice,
            self.verdict,
            self.features,
            self.timings,
            self.state,
            self.raw,
            self.subgraph,
        )

    @classmethod
    def empty(cls, aviso: str) -> Tab3View:
        return cls(aviso, "", "", "", {}, {}, plots.empty_figure("Sem subgrafo para mostrar."))


def evaluate_view(client: BackendClient, payload: dict) -> Tab3View:
    try:
        resp = client.evaluate(payload)
    except BackendError as exc:
        return Tab3View.empty(error_notice(exc))
    tempos = f"Neo4j: {fmt_num(resp['neo4j_ms'])} ms | Modelo: {fmt_num(resp['jev_ms'])} ms"
    return Tab3View(
        notice="",
        verdict=verdict_html(resp),
        features=features_html(resp["graph_features"]),
        timings=tempos,
        state=resp["state"],
        raw=resp.get("raw_response") or {},
        subgraph=plots.subgraph_figure(resp["subgraph"], payload["orig"], payload["dest"]),
    )


# ------------------------------------------------------------------ aba 4


@dataclass
class Tab4View:
    summary: str
    importance: go.Figure
    profiles: pd.DataFrame
    counts: go.Figure
    tree_text: str
    importance_table: pd.DataFrame

    def as_tuple(self) -> tuple:
        return (
            self.summary,
            self.importance,
            self.profiles,
            self.counts,
            self.tree_text,
            self.importance_table,
        )

    @classmethod
    def empty(cls, aviso: str) -> Tab4View:
        figs = _empty_figs("Sem dados para mostrar.")
        return cls(aviso, figs[0], pd.DataFrame(), figs[1], "", pd.DataFrame())


PROFILE_COLUMNS = {
    "perfil": "Perfil",
    "regra": "Regra",
    "transacoes": "Transações",
    "pct_amostra": "% da amostra",
    "pct_fraude_modelo": "% fraude (modelo)",
    "pct_fraude_real": "% fraude (real)",
    "arvore_decide": "Árvore decide",
}


def build_tab4(
    client: BackendClient, run_id: str | None, target: str, depth: int, threshold: float
) -> Tab4View:
    if not run_id:
        return Tab4View.empty(
            "Escolha a Execução B nos seletores acima ou rode um benchmark primeiro."
        )
    try:
        carregada = _load(client, run_id)
    except BackendError as exc:
        return Tab4View.empty(error_notice(exc))

    s = surrogate.fit(carregada.df, target, threshold, max_depth=int(depth))
    amostra = carregada.summary.get("sample") or {}
    fracao = (amostra.get("n_fraud", 0) / amostra["n"]) if amostra.get("n") else 0
    linhas = [f"**{series_name('B', carregada.summary)}**: {s.n} transações analisadas."]
    if s.fidelity is not None:
        alvo = (
            "das decisões do modelo reproduzidas" if target == "model" else "do gabarito acertado"
        )
        nome = "Fidelidade da árvore" if target == "model" else "Acerto da árvore contra o gabarito"
        linhas.append(
            f"{nome}: **{fmt_num(s.fidelity * 100)}%** {alvo} em 30% de dados separados "
            f"(no treino: {fmt_num(s.fidelity_train * 100)}%)."
        )
    if s.warning:
        linhas.append(f"**{s.warning}**")
    linhas.append(
        f"As quantidades são da amostra, que é estratificada ({fracao * 100:.0f}% de fraude), e não da base inteira."
    )

    perfis = s.profiles.rename(columns=PROFILE_COLUMNS)
    for coluna in ("% da amostra", "% fraude (modelo)", "% fraude (real)"):
        perfis[coluna] = pd.to_numeric(perfis[coluna], errors="coerce").round(1)
    percentuais = pd.to_numeric(s.importances["importancia"], errors="coerce") * 100
    importancias = s.importances.assign(**{"Importância (%)": percentuais.round(1)})
    importancias = importancias.rename(columns={"rotulo": "Feature"})[
        ["Feature", "Importância (%)"]
    ]
    return Tab4View(
        summary="\n\n".join(linhas),
        importance=plots.importance_figure(s.importances),
        profiles=perfis,
        counts=plots.profile_counts_figure(s.counts),
        tree_text=s.tree_text,
        importance_table=importancias,
    )


# ------------------------------------------------------------------ execução em segundo plano


@dataclass
class RunUpdate:
    status: str
    a: str | None
    b: str | None
    finished: bool


def _progress_line(titulo: str, resumo: dict) -> str:
    p = resumo.get("progress") or {}
    if not p.get("total"):
        return f"{titulo}: montando a amostra e aquecendo o modelo."
    return (
        f"{titulo}: {p['done']} de {p['total']} transações ({p['done'] / p['total'] * 100:.0f}%)."
    )


def run_benchmarks(
    client: BackendClient, mode: str, params: dict, poll_seconds: float = 1.0, sleep=time.sleep
) -> Iterator[RunUpdate]:
    """Roda uma ou duas execuções em sequência e vai avisando o andamento. No modo de
    comparação, a primeira é sem grafo (vira a Execução A) e a segunda com grafo (a B),
    com a mesma seed."""
    graficos = {MODE_COMPARE: [False, True], MODE_NO_GRAPH: [False], MODE_GRAPH: [True]}[mode]
    a_id = b_id = None
    for i, com_grafo in enumerate(graficos, start=1):
        titulo = f"Execução {i} de {len(graficos)} ({'com' if com_grafo else 'sem'} grafo)"
        try:
            run = client.start_run({**params, "graph": com_grafo})
            run_id = run["run_id"]
            while True:
                resumo = client.get_run(run_id)
                if resumo["status"] != "running":
                    break
                yield RunUpdate(_progress_line(titulo, resumo), None, None, False)
                sleep(poll_seconds)
        except BackendError as exc:
            yield RunUpdate(
                f"**Não consegui seguir com a {titulo.lower()}:** {exc}", a_id, b_id, True
            )
            return
        if resumo["status"] != "done":
            detalhe = f" {resumo['error']}" if resumo.get("error") else ""
            yield RunUpdate(
                f"**{titulo} terminou como {resumo['status']}.**{detalhe}", a_id, b_id, True
            )
            return
        if com_grafo:
            b_id = run_id
        else:
            a_id = run_id
    yield RunUpdate("Terminou. Os resultados aparecem nas abas.", a_id, b_id, True)


def cancel_active(client: BackendClient) -> str:
    try:
        ativa = client.info().get("active_run")
        if not ativa:
            return "Nenhuma execução em andamento."
        client.cancel_run(ativa)
    except BackendError as exc:
        return f"**Não consegui cancelar:** {exc}"
    return f"Cancelando a execução {ativa}."
```

- [ ] **Passo 5: rodar e ver passar**

Rode: `web_dashboard/.venv/bin/python -m pytest web_dashboard/tests/test_handlers.py -q`
Esperado: todos passam.

- [ ] **Passo 6: commitar**

```bash
git add web_dashboard/handlers.py web_dashboard/tests/fakes.py web_dashboard/tests/test_handlers.py
git commit -m "feat: adiciona handlers das quatro abas e a execução em segundo plano"
```

---
## Tarefa 7: a tela em Gradio

**Arquivos:**
- Criar: `web_dashboard/app.py`
- Testar: `web_dashboard/tests/test_app.py`

**Interfaces:**
- Consumes: `handlers` (Tarefa 6), `plots` (Tarefa 5), `client.BackendClient` (Tarefa 2) e o `FakeClient` de `tests/fakes.py`.
- Produz:
  - `app.build_app(client) -> gradio.Blocks`, que monta o cabeçalho e as quatro abas.
  - `app.main()`, que lê `BACKEND_API_URL` (padrão `http://127.0.0.1:8080`), `GRADIO_SERVER_NAME` (padrão `127.0.0.1`) e `GRADIO_SERVER_PORT` (padrão `7860`) e sobe a tela.
  - Eventos com `api_name` fixo (o teste e a verificação ponta a ponta chamam por esses nomes): `load`, `refresh`, `tab1`, `run`, `cancel`, `tab2`, `tab3_sample`, `tab3_evaluate` e `tab4`.
  - `app.POLL_SECONDS` (intervalo de consulta do andamento, o teste troca por 0).

Como a tela se comporta:
- Cabeçalho: resumo do modelo e do dataset, os seletores **Execução A** e **Execução B**, o slider de **limiar** (0,5) e o botão de atualizar. As abas 1, 2 e 4 reagem a esses controles, e o limiar recalcula tudo sem chamar o modelo.
- Ao abrir ou atualizar, a A e a B ficam com as escolhas atuais, se elas ainda existirem, ou com o par padrão (`default_pair`).
- Aba 1: o botão "Rodar" chama `run_benchmarks` e vai atualizando o texto de andamento. Ao terminar, troca A e B pelas novas execuções, e isso dispara a renderização das abas.
- **As abas 2 e 4 redesenham ao serem abertas** (evento `select`). O Plotly calcula a largura quando o gráfico é criado, e numa aba escondida o container tem largura zero, então sem isso os gráficos saem estreitos. Isso apareceu na verificação no navegador.
- **Step e seq do simulador são caixas de texto** que começam realmente vazias. Com `gr.Number` o Gradio mostrava `0`, e o pedido saía com `seq: 0`, fazendo um replay da primeira transação sem o usuário pedir.
- O tema é o `Soft`, forçado em claro (`?__theme=light`), com azul da marca, as fontes Geist e Space Grotesk e o CSS dos cartões de número (KPI) e do veredito. O rodapé do Gradio fica escondido e a página de API desligada.
- O veredito usa palavra e cor (`Fraude` ou `Legítima`), nunca só a cor.
- Cada gráfico tem sua "vista em tabela" num acordeão.

- [ ] **Passo 1: escrever os testes que falham**

```python file=web_dashboard/tests/test_app.py
import gradio as gr
import pytest

import app
import handlers
from client import BackendError
from tests.fakes import FakeClient, default_info, evaluate_response, two_runs


@pytest.fixture(autouse=True)
def sem_espera(monkeypatch):
    monkeypatch.setattr(app, "POLL_SECONDS", 0.0)


def novo_app(client=None):
    client = client or FakeClient(runs=two_runs(), evaluate_response=evaluate_response())
    return app.build_app(client)


def eventos(demo):
    return {f.api_name: f for f in demo.fns.values() if f.api_name}


def padrao(bloco_fn):
    """Os valores iniciais dos componentes de entrada, como a tela os mandaria ao abrir."""
    return [c.value for c in bloco_fn.inputs]


def test_monta_a_tela_com_as_quatro_abas():
    demo = novo_app()
    assert isinstance(demo, gr.Blocks)
    abas = [b.label for b in demo.blocks.values() if isinstance(b, gr.Tab)]
    assert abas == [
        "Benchmark e impacto do grafo",
        "Performance e latência",
        "Simulador",
        "Perfil transacional",
    ]


def test_todos_os_eventos_existem_com_o_nome_combinado():
    nomes = set(eventos(novo_app()))
    assert {
        "load",
        "refresh",
        "tab1",
        "run",
        "cancel",
        "tab2",
        "tab3_sample",
        "tab3_evaluate",
        "tab4",
    } <= nomes


def test_cada_evento_devolve_o_numero_certo_de_saidas():
    """Em Gradio, saída a mais ou a menos só estoura no navegador. Aqui chamamos cada função
    com os valores iniciais da tela e conferimos a contagem."""
    demo = novo_app()
    for nome, bloco_fn in eventos(demo).items():
        if nome in ("run", "tab3_sample", "cancel"):
            continue  # têm testes próprios abaixo
        resultado = bloco_fn.fn(*padrao(bloco_fn))
        if not isinstance(resultado, tuple):
            resultado = (resultado,)
        assert len(resultado) == len(bloco_fn.outputs), nome


def test_abas_2_e_4_redesenham_ao_serem_abertas():
    """Plotly calcula a largura ao criar o gráfico. Numa aba escondida ela é zero, e o
    gráfico sairia estreito. Por isso essas abas se redesenham no evento select."""
    demo = novo_app()
    for nome in ("tab2", "tab4"):
        gatilhos = {evento for _, evento in eventos(demo)[nome].targets}
        assert "select" in gatilhos, nome
    assert "change" in {evento for _, evento in eventos(demo)["tab1"].targets}


def test_abrir_a_tela_escolhe_o_par_padrao_e_o_modelo():
    demo = novo_app()
    ev = eventos(demo)["load"]
    overview, a, b, modelo1, modelo3 = ev.fn(*padrao(ev))
    assert "estilo Jev" in overview
    assert a["value"] == "run_20260930_160000_aaaa" and b["value"] == "run_20260930_161000_bbbb"
    assert len(a["choices"]) == 2
    assert modelo1["value"] == "tev1:4b" and modelo3["value"] == "tev1:4b"
    assert modelo3["choices"] == ["tev1:4b", "tev1:0.8b", "nimble:latest"]
    assert modelo1 is not modelo3  # o Gradio consome o dict, então cada campo precisa do seu


def test_atualizar_preserva_a_escolha_que_ainda_existe():
    demo = novo_app()
    ev = eventos(demo)["refresh"]
    _, a, b, _, _ = ev.fn("run_20260930_161000_bbbb", "run_20260930_160000_aaaa")
    assert a["value"] == "run_20260930_161000_bbbb" and b["value"] == "run_20260930_160000_aaaa"
    _, a, b, _, _ = ev.fn("run_que_sumiu", None)
    assert a["value"] == "run_20260930_160000_aaaa" and b["value"] == "run_20260930_161000_bbbb"


def test_abrir_a_tela_com_a_api_fora_do_ar_mostra_o_aviso():
    client = FakeClient()
    client.errors["info"] = BackendError("Não consegui falar com a API", code="unreachable")
    ev = eventos(novo_app(client))["load"]
    overview, a, b, _, _ = ev.fn(*padrao(ev))
    assert "Não consegui falar com a API" in overview and a["value"] is None and a["choices"] == []


def test_primeiro_uso_sem_nenhuma_execucao_nao_quebra_as_abas():
    demo = novo_app(FakeClient())
    for nome in ("tab1", "tab2", "tab4"):
        ev = eventos(demo)[nome]
        resultado = ev.fn(*padrao(ev))
        assert len(resultado) == len(ev.outputs)
    aviso = eventos(demo)["tab1"].fn(None, None, 0.5)[0]
    assert "rode um benchmark" in aviso.lower()


def test_rodar_atualiza_o_andamento_e_no_fim_troca_a_e_b():
    client = FakeClient(running_polls=1)
    ev = eventos(novo_app(client))["run"]
    entradas = padrao(ev)
    atualizacoes = list(ev.fn(*entradas))
    assert all(len(u) == len(ev.outputs) == 3 for u in atualizacoes)
    final = atualizacoes[-1]
    assert "Terminou" in final[0]
    assert final[1]["value"] == "run_20260930_170001_0001"
    assert final[2]["value"] == "run_20260930_170002_0002"
    assert [p["graph"] for p in client.started] == [False, True]
    assert client.started[0]["types"] == ["TRANSFER", "CASH_OUT"] and client.started[0]["n"] == 2000


def test_rodar_so_com_grafo_mantem_a_escolha_de_a():
    client = FakeClient(runs=two_runs(), running_polls=0)
    ev = eventos(novo_app(client))["run"]
    entradas = padrao(ev)
    entradas[0] = handlers.MODE_GRAPH
    entradas[-2] = "run_20260930_160000_aaaa"  # A atual
    entradas[-1] = "run_20260930_161000_bbbb"  # B atual
    final = list(ev.fn(*entradas))[-1]
    assert final[1]["value"] == "run_20260930_160000_aaaa"  # A continua
    assert final[2]["value"].startswith("run_20260930_17")  # B é a nova


def test_rodar_com_erro_da_api_mostra_a_mensagem():
    client = FakeClient()
    client.errors["start_run"] = BackendError(
        "já existe uma execução em andamento", code="run_in_progress"
    )
    ev = eventos(novo_app(client))["run"]
    final = list(ev.fn(*padrao(ev)))[-1]
    assert "já existe uma execução em andamento" in final[0]


def test_cancelar():
    client = FakeClient(info=default_info(active_run="run_20260930_170001_0001"))
    ev = eventos(novo_app(client))["cancel"]
    assert "Cancelando" in ev.fn()
    assert client.canceled == ["run_20260930_170001_0001"]


def test_sortear_preenche_o_formulario_e_sem_execucao_avisa(monkeypatch):
    avisos = []
    monkeypatch.setattr(app.gr, "Warning", lambda msg, *a, **k: avisos.append(msg))
    demo = novo_app()
    ev = eventos(demo)["tab3_sample"]
    valores = ev.fn("run_20260930_160000_aaaa")
    assert len(valores) == len(ev.outputs) == 10
    assert isinstance(valores[4], str) and isinstance(
        valores[9], str
    )  # step e seq são caixas de texto
    assert valores[4].isdigit() and valores[9].isdigit()

    sem = ev.fn(None)
    assert len(sem) == 10 and avisos and "Execução A" in avisos[0]


def test_step_e_seq_comecam_vazios_e_nao_vao_no_pedido():
    client = FakeClient(evaluate_response=evaluate_response())
    ev = eventos(novo_app(client))["tab3_evaluate"]
    entradas = padrao(ev)
    assert entradas[4] == "" and entradas[9] == ""  # step e seq
    resultado = ev.fn(*entradas)
    assert len(resultado) == len(ev.outputs) == 7 and "verdict-fraud" in resultado[1]
    assert "step" not in client.evaluated[0] and "seq" not in client.evaluated[0]


def test_consultar_com_step_invalido_e_erro_da_api():
    client = FakeClient(evaluate_response=evaluate_response())
    ev = eventos(novo_app(client))["tab3_evaluate"]
    entradas = padrao(ev)
    entradas[4] = "abc"
    saida = ev.fn(*entradas)
    assert "número inteiro" in saida[0] and saida[1] == "" and not client.evaluated

    client = FakeClient()
    client.errors["evaluate"] = BackendError(
        "pedido inválido", code="validation_error", details=["orig vazio"]
    )
    ev = eventos(novo_app(client))["tab3_evaluate"]
    saida = ev.fn(*padrao(ev))
    assert "pedido inválido" in saida[0] and "orig vazio" in saida[0]


def test_visual_padrao_da_marca():
    assert "Space+Grotesk" in app.HEAD and "Geist" in app.HEAD
    assert "__theme" in app.FORCE_LIGHT_JS and "light" in app.FORCE_LIGHT_JS
    for classe in (".kpi", ".kpi-row", ".verdict-fraud", ".verdict-legit", ".kpi-dot"):
        assert classe in app.CSS
    assert "#d13700" in app.CSS and "#3b7736" in app.CSS  # status: negativo e positivo
    assert "footer" in app.CSS


def test_main_le_o_ambiente(monkeypatch):
    lancado = {}

    class FalsoDemo:
        def queue(self):
            return self

        def launch(self, **kwargs):
            lancado.update(kwargs)

    criados = {}
    monkeypatch.setattr(
        app, "build_app", lambda client: criados.setdefault("client", client) and FalsoDemo()
    )
    monkeypatch.setenv("BACKEND_API_URL", "http://api.teste:8080/")
    monkeypatch.setenv("GRADIO_SERVER_NAME", "0.0.0.0")
    monkeypatch.setenv("GRADIO_SERVER_PORT", "7999")
    app.main()
    assert criados["client"].base_url == "http://api.teste:8080"
    assert lancado == {"server_name": "0.0.0.0", "server_port": 7999, "show_api": False}
```

- [ ] **Passo 2: rodar e ver falhar**

Rode: `web_dashboard/.venv/bin/python -m pytest web_dashboard/tests/test_app.py -q`
Esperado: FAIL com `ModuleNotFoundError: No module named 'app'`.

- [ ] **Passo 3: implementar**

```python file=web_dashboard/app.py
"""Tela do dashboard em Gradio. Só monta o layout e liga os eventos aos handlers."""

from __future__ import annotations

import os

import gradio as gr

import handlers
import plots
from client import BackendClient

POLL_SECONDS = 1.0

# No Gradio 6 os parâmetros head, css, js e theme passam do Blocks() para o launch().
HEAD = """
<link rel="preconnect" href="https://fonts.googleapis.com">
<link href="https://fonts.googleapis.com/css2?family=Geist:wght@400;500;600&family=Space+Grotesk:wght@500;600;700&display=swap" rel="stylesheet">
"""

# O padrão visual pede o tema claro para exportações e relatórios. Sem isso o Gradio segue o
# tema escuro do sistema, e os gráficos (feitos para fundo claro) ficam ilegíveis.
FORCE_LIGHT_JS = """
() => {
  const url = new URL(window.location);
  if (url.searchParams.get('__theme') !== 'light') {
    url.searchParams.set('__theme', 'light');
    window.location.href = url.href;
  }
}
"""

CSS = """
footer { display: none !important; }
h1, h2, h3, .kpi-title, .verdict-word { font-family: 'Space Grotesk', system-ui, sans-serif; }
h1, h2, h3 { color: #020c44; }
.kpi-title { display: flex; align-items: center; gap: 8px; font-weight: 600; color: #020c44; margin: 14px 0 8px; }
.kpi-dot { width: 10px; height: 10px; border-radius: 50%; display: inline-block; }
.kpi-row { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 12px; }
.kpi { background: #ffffff; border: 1px solid #e0e0d8; border-radius: 8px; padding: 12px 14px; }
.kpi-label { color: #162f36; font-size: 13px; }
.kpi-value { color: #020c44; font-family: 'Space Grotesk', system-ui, sans-serif; font-weight: 600;
  font-size: 26px; line-height: 1.25; }
.verdict { display: flex; flex-direction: column; gap: 4px; padding: 14px 18px; background: #ffffff;
  border: 1px solid #e0e0d8; border-left-width: 8px; border-radius: 8px; }
.verdict-fraud { border-left-color: #d13700; }
.verdict-legit { border-left-color: #3b7736; }
.verdict-word { font-weight: 700; font-size: 28px; color: #020c44; }
.verdict-meta { color: #162f36; font-size: 13px; }
"""

BRAND = gr.themes.Color(
    c50="#eef3ff",
    c100="#dbe6ff",
    c200="#b9ceff",
    c300="#8daeff",
    c400="#5a86ff",
    c500="#2f61ff",
    c600="#0038e5",
    c700="#002fc0",
    c800="#00279b",
    c900="#001f7a",
    c950="#020c44",
)
THEME = gr.themes.Soft(
    primary_hue=BRAND,
    neutral_hue=gr.themes.colors.slate,
    font=[gr.themes.GoogleFont("Geist"), "system-ui", "sans-serif"],
)


def build_app(client: BackendClient) -> gr.Blocks:
    with gr.Blocks(
        title="Detector de fraude com grafo",
        theme=THEME,
        css=CSS,
        head=HEAD,
        js=FORCE_LIGHT_JS,
        analytics_enabled=False,
    ) as demo:
        gr.Markdown("# Detector de fraude com Go, Neo4j e modelo local")
        overview = gr.Markdown()
        with gr.Row():
            run_a = gr.Dropdown(label="Execução A (base)", choices=[], value=None, scale=3)
            run_b = gr.Dropdown(label="Execução B", choices=[], value=None, scale=3)
            threshold = gr.Slider(
                0.0, 1.0, value=0.5, step=0.01, label="Limiar de decisão", scale=2
            )
            refresh = gr.Button("Atualizar", scale=1)

        with gr.Tabs():
            with gr.Tab("Benchmark e impacto do grafo"):
                with gr.Accordion("Rodar um benchmark", open=True):
                    mode = gr.Radio(
                        [handlers.MODE_COMPARE, handlers.MODE_NO_GRAPH, handlers.MODE_GRAPH],
                        value=handlers.MODE_COMPARE,
                        label="O que rodar",
                    )
                    with gr.Row():
                        n = gr.Number(
                            value=2000, precision=0, minimum=1, label="Quantidade de transações"
                        )
                        fraud_pct = gr.Slider(
                            0, 100, value=10, step=1, label="% de fraude na amostra"
                        )
                        model1 = gr.Dropdown(
                            label="Modelo", choices=[], value=None, allow_custom_value=True
                        )
                        seed = gr.Number(value=42, precision=0, minimum=0, label="Seed")
                    types = gr.CheckboxGroup(
                        handlers.TYPES,
                        value=["TRANSFER", "CASH_OUT"],
                        label="Tipos de transação da amostra",
                    )
                    with gr.Accordion("Parâmetros avançados", open=False):
                        with gr.Row():
                            workers = gr.Number(
                                value=32,
                                precision=0,
                                minimum=1,
                                label="Consultas simultâneas ao Neo4j",
                            )
                            window = gr.Number(
                                value=24,
                                precision=0,
                                minimum=1,
                                label="Janela em horas (fan-in e fan-out)",
                            )
                            hops = gr.Slider(1, 5, value=3, step=1, label="Saltos máximos do ciclo")
                            inflight = gr.Number(
                                value=4,
                                precision=0,
                                minimum=1,
                                label="Chamadas simultâneas ao modelo",
                            )
                    with gr.Row():
                        run_btn = gr.Button("Rodar", variant="primary")
                        cancel_btn = gr.Button("Cancelar a execução em andamento")
                    run_status = gr.Markdown()

                notices1 = gr.Markdown()
                metrics1 = gr.Dataframe(label="Métricas", interactive=False, wrap=True)
                with gr.Row():
                    plot_conf = gr.Plot(show_label=False)
                    plot_fpfn = gr.Plot(show_label=False)
                plot_roc = gr.Plot(show_label=False)
                with gr.Row():
                    plot_sweep = gr.Plot(show_label=False)
                    plot_types = gr.Plot(show_label=False)
                with gr.Accordion("Ver os dados dos gráficos como tabela", open=False):
                    sweep_table = gr.Dataframe(
                        label="F1 e demais métricas por limiar", interactive=False
                    )
                    types_table = gr.Dataframe(label="Fraudes por tipo", interactive=False)

            with gr.Tab("Performance e latência") as tab2:
                notices2 = gr.Markdown()
                kpis = gr.HTML(padding=False)
                plot_pct = gr.Plot(show_label=False)
                with gr.Row():
                    plot_hist_neo4j = gr.Plot(show_label=False)
                    plot_hist_jev = gr.Plot(show_label=False)
                with gr.Accordion("Ver as latências como tabela", open=False):
                    table2 = gr.Dataframe(interactive=False)

            with gr.Tab("Simulador"):
                with gr.Row():
                    with gr.Column(scale=2):
                        gr.Markdown("### Transação")
                        orig = gr.Textbox(label="Origem", max_lines=1)
                        dest = gr.Textbox(label="Destino", max_lines=1)
                        tipo = gr.Dropdown(handlers.TYPES, value="TRANSFER", label="Tipo")
                        amount = gr.Number(value=350000, label="Valor")
                        step = gr.Textbox(
                            value="",
                            max_lines=1,
                            placeholder="vazio",
                            label="Step (vazio usa o maior do dataset)",
                        )
                        with gr.Accordion("Saldos", open=False):
                            ob = gr.Number(value=350000, label="Saldo da origem antes")
                            oa = gr.Number(value=0, label="Saldo da origem depois")
                            db = gr.Number(value=0, label="Saldo do destino antes")
                            da = gr.Number(value=0, label="Saldo do destino depois")
                        seq = gr.Textbox(
                            value="",
                            max_lines=1,
                            placeholder="vazio",
                            label="seq (replay de uma transação real. Vazio significa transação de agora)",
                        )
                        model3 = gr.Dropdown(
                            label="Modelo", choices=[], value=None, allow_custom_value=True
                        )
                        with gr.Row():
                            sample_btn = gr.Button("Sortear da Execução A")
                            eval_btn = gr.Button("Consultar modelo e Neo4j", variant="primary")
                    with gr.Column(scale=3):
                        notice3 = gr.Markdown()
                        verdict = gr.HTML(padding=False)
                        features = gr.HTML(padding=False)
                        timings = gr.Markdown()
                        plot_subgraph = gr.Plot(
                            value=plots.empty_figure("Consulte uma transação para ver o subgrafo."),
                            show_label=False,
                        )
                        with gr.Accordion(
                            "Ver o state enviado e a resposta bruta do modelo", open=False
                        ):
                            state_json = gr.JSON(label="state enviado")
                            raw_json = gr.JSON(label="resposta bruta")

            with gr.Tab("Perfil transacional") as tab4:
                with gr.Row():
                    target = gr.Radio(
                        list(handlers.TARGETS),
                        value="Decisões do modelo",
                        label="O que a árvore imita",
                    )
                    depth = gr.Slider(2, 5, value=3, step=1, label="Profundidade da árvore")
                    profiles_btn = gr.Button("Gerar perfis", variant="primary")
                summary4 = gr.Markdown()
                profiles = gr.Dataframe(
                    label="Perfis",
                    interactive=False,
                    wrap=True,
                    column_widths=["8%", "40%", "10%", "10%", "11%", "11%", "10%"],
                )
                with gr.Row():
                    plot_importance = gr.Plot(show_label=False)
                    plot_counts = gr.Plot(show_label=False)
                with gr.Accordion("Ver a árvore e a importância como tabela", open=False):
                    tree_text = gr.Code(label="Árvore em texto", interactive=False, lines=14)
                    importance_table = gr.Dataframe(
                        label="Importância das features", interactive=False
                    )

        # ---------------------------------------------------------------- eventos

        def on_load(a, b):
            st = handlers.initial_state(client)
            validas = {valor for _, valor in st.choices}
            novo_a = a if a in validas else st.a
            novo_b = b if b in validas else st.b
            return (
                st.overview,
                gr.update(choices=st.choices, value=novo_a),
                gr.update(choices=st.choices, value=novo_b),
                # um dict para cada campo: o Gradio consome o dict ao aplicar a atualização
                gr.update(choices=st.models, value=st.default_model),
                gr.update(choices=st.models, value=st.default_model),
            )

        cabecalho = [overview, run_a, run_b, model1, model3]
        demo.load(on_load, [run_a, run_b], cabecalho, api_name="load")
        refresh.click(on_load, [run_a, run_b], cabecalho, api_name="refresh")

        # Aba 1
        saidas1 = [
            notices1,
            metrics1,
            plot_conf,
            plot_roc,
            plot_sweep,
            plot_fpfn,
            plot_types,
            sweep_table,
            types_table,
        ]

        def on_tab1(a, b, limiar):
            return handlers.build_tab1(client, a, b, limiar).as_tuple()

        gr.on(
            [run_a.change, run_b.change, threshold.release],
            on_tab1,
            [run_a, run_b, threshold],
            saidas1,
            api_name="tab1",
            show_progress="minimal",
        )

        def on_run(
            modo,
            qtd,
            pct,
            modelo,
            semente,
            tipos,
            trabalhadores,
            janela,
            saltos,
            simultaneas,
            atual_a,
            atual_b,
        ):
            params = {
                "n": int(qtd),
                "fraud_pct": float(pct),
                "seed": int(semente),
                "types": list(tipos),
                "workers": int(trabalhadores),
                "window_hours": int(janela),
                "cycle_max_hops": int(saltos),
                "max_inflight": int(simultaneas),
            }
            if modelo:
                params["model"] = modelo
            for atualizacao in handlers.run_benchmarks(
                client, modo, params, poll_seconds=POLL_SECONDS
            ):
                if not atualizacao.finished:
                    yield atualizacao.status, gr.skip(), gr.skip()
                    continue
                st = handlers.initial_state(client)
                yield (
                    atualizacao.status,
                    gr.update(choices=st.choices, value=atualizacao.a or atual_a),
                    gr.update(choices=st.choices, value=atualizacao.b or atual_b),
                )

        run_btn.click(
            on_run,
            [
                mode,
                n,
                fraud_pct,
                model1,
                seed,
                types,
                workers,
                window,
                hops,
                inflight,
                run_a,
                run_b,
            ],
            [run_status, run_a, run_b],
            api_name="run",
        )
        cancel_btn.click(
            lambda: handlers.cancel_active(client), None, run_status, api_name="cancel"
        )

        # Aba 2 (redesenha ao abrir, porque numa aba escondida o Plotly calcula largura zero)
        saidas2 = [notices2, kpis, plot_pct, plot_hist_neo4j, plot_hist_jev, table2]

        def on_tab2(a, b):
            return handlers.build_tab2(client, a, b).as_tuple()

        gr.on(
            [run_a.change, run_b.change, tab2.select],
            on_tab2,
            [run_a, run_b],
            saidas2,
            api_name="tab2",
            show_progress="minimal",
        )

        # Aba 3
        campos = [orig, dest, tipo, amount, step, ob, oa, db, da, seq]

        def on_sample(run_id):
            try:
                valores = list(handlers.sample_transaction(client, run_id))
            except handlers.UserMessage as aviso:
                gr.Warning(str(aviso))
                return tuple(gr.skip() for _ in campos)
            valores[4], valores[9] = (
                str(valores[4]),
                str(valores[9]),
            )  # step e seq são caixas de texto
            return tuple(valores)

        sample_btn.click(on_sample, [run_a], campos, api_name="tab3_sample")

        saidas3 = [notice3, verdict, features, timings, state_json, raw_json, plot_subgraph]

        def on_evaluate(*valores):
            *formulario, modelo, limiar = valores
            try:
                payload = handlers.build_evaluate_payload(*formulario, modelo, limiar)
            except handlers.UserMessage as aviso:
                return handlers.Tab3View.empty(f"**Erro:** {aviso}").as_tuple()
            return handlers.evaluate_view(client, payload).as_tuple()

        eval_btn.click(on_evaluate, [*campos, model3, threshold], saidas3, api_name="tab3_evaluate")

        # Aba 4 (também redesenha ao abrir)
        saidas4 = [summary4, plot_importance, profiles, plot_counts, tree_text, importance_table]

        def on_tab4(run_id, alvo, profundidade, limiar):
            return handlers.build_tab4(
                client, run_id, handlers.TARGETS[alvo], int(profundidade), limiar
            ).as_tuple()

        gr.on(
            [
                profiles_btn.click,
                run_b.change,
                threshold.release,
                target.change,
                depth.release,
                tab4.select,
            ],
            on_tab4,
            [run_b, target, depth, threshold],
            saidas4,
            api_name="tab4",
            show_progress="minimal",
        )

    return demo


def main() -> None:
    client = BackendClient(os.environ.get("BACKEND_API_URL", "http://127.0.0.1:8080"))
    demo = build_app(client)
    demo.queue().launch(
        server_name=os.environ.get("GRADIO_SERVER_NAME", "127.0.0.1"),
        server_port=int(os.environ.get("GRADIO_SERVER_PORT", "7860")),
        show_api=False,
    )


if __name__ == "__main__":
    main()
```

- [ ] **Passo 4: rodar e ver passar**

Rode: `web_dashboard/.venv/bin/python -m pytest web_dashboard/tests/test_app.py -q`
Esperado: todos passam.

- [ ] **Passo 5: rodar a suíte inteira e o lint**

Rode:

```bash
web_dashboard/.venv/bin/ruff check web_dashboard
web_dashboard/.venv/bin/ruff format --check web_dashboard
web_dashboard/.venv/bin/python -m pytest web_dashboard -q
```

Esperado: `All checks passed!`, tudo formatado e todos os testes passando.

- [ ] **Passo 6: commitar**

```bash
git add web_dashboard/app.py web_dashboard/tests/test_app.py
git commit -m "feat: adiciona a tela em Gradio com as quatro abas"
```

---
## Tarefa 8: Docker, compose, Makefile e README do dashboard

**Arquivos:**
- Criar: `web_dashboard/Dockerfile`, `web_dashboard/.dockerignore`
- Modificar (arquivos do plano 1): `docker-compose.yml`, `Makefile`, `README.md`

**Interfaces:**
- Consumes: os arquivos do dashboard das tarefas anteriores e o `fraud-api` do plano 1.
- Produz:
  - O serviço `dashboard` do compose, em `http://127.0.0.1:7860`, que fala com `http://fraud-api:8080`.
  - Os alvos `make setup-py` (cria o ambiente virtual) e `make lint-py` (ruff). O `make test-py` já existia no plano 1.
  - A seção do dashboard no README.

Decisões: a imagem é `python:3.12-slim`, roda sem root, tem `HEALTHCHECK` (então o `docker compose up --wait` espera a tela responder) e só copia os cinco módulos do app (nada de testes). Como no plano 1, o build usa `network: host`, porque em alguns setups o DNS dentro do container de build é um IPv6 sem rota e o `pip install` falha.

- [ ] **Passo 1: criar o Dockerfile e o `.dockerignore`**

```dockerfile file=web_dashboard/Dockerfile
FROM python:3.12-slim

WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY client.py analysis.py surrogate.py plots.py handlers.py app.py ./

# Roda sem root. O Gradio precisa de uma pasta temporária com escrita.
RUN useradd --system --uid 10001 app && mkdir -p /tmp/gradio && chown app /tmp/gradio
USER app

ENV GRADIO_SERVER_NAME=0.0.0.0 \
    GRADIO_SERVER_PORT=7860 \
    GRADIO_ANALYTICS_ENABLED=False \
    GRADIO_TEMP_DIR=/tmp/gradio \
    BACKEND_API_URL=http://fraud-api:8080

EXPOSE 7860
HEALTHCHECK --interval=15s --timeout=5s --retries=5 --start-period=20s \
  CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:7860/', timeout=3)"

CMD ["python", "app.py"]
```

```text file=web_dashboard/.dockerignore
.venv
tests
__pycache__
.pytest_cache
.ruff_cache
*.md
pyproject.toml
requirements-dev.txt
```

- [ ] **Passo 2: acrescentar o serviço no `docker-compose.yml`**

Abra o `docker-compose.yml` do plano 1 e, **logo antes** do comentário `# Roda só quando pedido: docker compose run --rm seeder (ou make seed).`, acrescente o serviço (com a linha em branco no fim, para separar do comentário):

```yaml
  dashboard:
    build:
      context: ./web_dashboard
      dockerfile: Dockerfile
      # Mesmo motivo do fraud-api: o build usa a rede do host para o pip.
      network: host
    image: fraud-detector-dashboard
    container_name: fraud_dashboard
    restart: unless-stopped
    ports:
      - "127.0.0.1:7860:7860"
    environment:
      BACKEND_API_URL: http://fraud-api:8080
    depends_on:
      fraud-api:
        condition: service_healthy

```

- [ ] **Passo 3: acrescentar os alvos no `Makefile`**

No `Makefile`, troque a linha do `.PHONY` por:

```text
.PHONY: up down seed lint lint-py setup-py test-go test-integration test-py smoke
```

E, **logo antes** da linha `test-py:`, acrescente (as linhas de receita começam com TAB, não com espaços):

```make
# Cria o ambiente Python do dashboard (para rodar os testes e o app fora do Docker).
setup-py:
	python3 -m venv web_dashboard/.venv
	web_dashboard/.venv/bin/pip install -r web_dashboard/requirements-dev.txt

lint-py:
	web_dashboard/.venv/bin/ruff check web_dashboard
	web_dashboard/.venv/bin/ruff format --check web_dashboard

```

- [ ] **Passo 4: acrescentar a seção do dashboard no `README.md`**

No `README.md` faça quatro edições.

**4.1.** Na seção "4. Subir o Neo4j e a API", troque a frase `Isso constrói a imagem, sobe o Neo4j e a API e espera os dois ficarem saudáveis.` por:

```text
Isso constrói as imagens, sobe o Neo4j, a API e o dashboard e espera todos ficarem saudáveis.
```

**4.2.** **Antes** do título `## Como ler os resultados`, acrescente:

````markdown
## 7. Abrir o dashboard

O `make up` já sobe o dashboard. Abra http://127.0.0.1:7860 no navegador. O tema é claro de propósito, e a página recarrega uma vez ao abrir para fixar esse tema (um clique dado nessa primeira fração de segundo se perde).

No topo você escolhe a **Execução A** (a base, normalmente sem grafo) e a **Execução B** (normalmente com grafo), e ajusta o **limiar de decisão**. Mexer no limiar recalcula tudo sem chamar o modelo de novo.

- **Benchmark e impacto do grafo:** o botão "Rodar" no modo "Comparar com e sem grafo" faz duas execuções com a mesma seed, primeiro sem grafo (vira a A) e depois com grafo (vira a B), e mostra o andamento. Abaixo ficam as métricas lado a lado, a matriz de confusão, falsos positivos contra falsos negativos, ROC e PR, o F1 por limiar e as fraudes por tipo. A comparação usa só as transações que aparecem nas duas execuções.
- **Performance e latência:** transações por segundo, tempo total, erros e tokens, e os percentis P50, P95 e P99 do Neo4j e do modelo em separado.
- **Simulador:** digite uma transação, ou use "Sortear da Execução A" para preencher com uma real (aí o `seq` vai junto e a consulta vira um replay, só com o passado daquela transação). Com `seq` e `step` vazios, é uma transação hipotética "de agora". O resultado mostra o veredito, as features de grafo, os tempos, o `state` enviado, a resposta bruta e o subgrafo.
- **Perfil transacional:** uma árvore rasa imita as decisões do modelo (ou o gabarito) e cada folha vira um perfil com regra em texto. A tela mostra a **fidelidade** da árvore e avisa quando ela explica mal o modelo. As quantidades são da amostra, não da base inteira.

Para rodar o dashboard fora do Docker (por exemplo, para desenvolver):

```bash
make setup-py
BACKEND_API_URL=http://127.0.0.1:8080 web_dashboard/.venv/bin/python web_dashboard/app.py
```

````

**4.3.** Na seção "Desenvolvimento", troque a linha `make test-integration  # sobe um Neo4j descartável e testa as queries de verdade` por:

```text
make test-integration  # sobe um Neo4j descartável e testa as queries de verdade
make setup-py          # cria o ambiente Python do dashboard
make test-py           # testes do dashboard
make lint-py           # ruff no dashboard
```

E, na árvore de pastas dessa mesma seção, **depois** da linha `testdata/               casos de teste compartilhados`, acrescente:

```text
web_dashboard/          dashboard Gradio (client, analysis, surrogate, plots, handlers, app)
```

**4.4.** Na seção "Solução de problemas", acrescente no fim:

```text
- **O dashboard abre e mostra "Não consegui falar com a API"**: a API não está no ar. Confira `docker compose ps` e `curl http://127.0.0.1:8080/healthz`. Depois de subir a API, clique em "Atualizar".
- **As abas de gráficos aparecem com a largura errada**: abra a aba de novo (elas se redesenham ao abrir). Se persistir, recarregue a página.
- **O botão "Rodar" não faz nada logo depois de abrir a página**: a página recarrega uma vez para fixar o tema claro. Clique de novo.
```

- [ ] **Passo 5: validar**

```bash
NEO4J_PASSWORD=senha-de-teste-123 docker compose config -q && echo "compose ok"
make -n lint-py setup-py
grep -nP '[—–]' README.md || echo "README sem travessão"
```

Esperado: `compose ok`, o `make -n` mostra os comandos sem erro de "missing separator", e o README sem travessão.

- [ ] **Passo 6: construir a imagem e subir**

```bash
docker compose up -d --build --wait
docker compose ps
curl -s -o /dev/null -w "dashboard HTTP %{http_code}\n" http://127.0.0.1:7860/
```

Esperado: `neo4j`, `fraud-api` e `dashboard` saudáveis e `dashboard HTTP 200`. Se o build da imagem falhar em `pip install` com `network is unreachable`, confira se o `network: host` está no serviço.

- [ ] **Passo 7: commitar**

```bash
git add web_dashboard/Dockerfile web_dashboard/.dockerignore docker-compose.yml Makefile README.md
git commit -m "feat: adiciona o dashboard ao compose, ao Makefile e ao README"
```

---

## Tarefa 9: aceite ponta a ponta do dashboard

**Arquivos:** nenhum novo. É a definição de pronto da spec para o dashboard. Precisa do backend do plano 1 já aceito (Tarefa 17 dele) e de pelo menos uma execução com grafo e uma sem grafo.

Os passos pedem para olhar a tela no navegador. Quem executa pode fazer isso direto, ou pedir para a pessoa abrir o endereço. **Olhe os gráficos de verdade**: foi olhando a tela que a verificação deste plano achou os problemas de largura, de legenda e de barra grossa. Nenhum teste automático pega isso.

- [ ] **Passo 1: testes automáticos**

```bash
make setup-py
make lint-py
make test-py
```

Esperado: `All checks passed!`, tudo formatado e todos os testes passando (a contagem vai aparecer na saída do pytest).

- [ ] **Passo 2: subir tudo e abrir a tela**

```bash
make up
```

Abra http://127.0.0.1:7860. Esperado: o topo mostra o modelo (com o aviso de que não é a Jev da TypeSafe) e o resumo do dataset, e os seletores A e B já vêm com as execuções mais recentes (A sem grafo, B com grafo).

- [ ] **Passo 3: Aba 1, comparar com e sem grafo**

Clique em "Rodar" no modo "Comparar com e sem grafo", com `n` de 200 e o modelo `tev1:0.8b`. Esperado:
- O andamento aparece (`Execução 1 de 2 (sem grafo)...`) e termina com "Terminou".
- A e B passam para as duas execuções novas.
- A tabela de métricas mostra as colunas A, B e "Diferença (B menos A)" com `p.p.` (e não `p,p`). A linha "Precisão reprojetada" traz a prevalência real.
- Mexa no slider de limiar: tudo se recalcula e a linha vermelha do gráfico de F1 acompanha. Em 0 e em 1 nada quebra.
- Barras finas (até uns 24 px), legenda sem encostar em nenhum título, e as duas cores (A azul, B laranja) distinguíveis.

- [ ] **Passo 4: Aba 2, performance e latência**

Abra a aba. Esperado: os cartões de número de A e B, o gráfico de percentis ocupando **a largura toda** (não só um pedaço) e os dois histogramas. Uma execução sem grafo deixa o histograma do Neo4j vazio com um aviso.

- [ ] **Passo 5: Aba 3, simulador e replay**

Clique em "Sortear da Execução A" e depois em "Consultar modelo e Neo4j" (espere o formulário preencher antes de consultar). Esperado:
- Step e seq vêm preenchidos pelo sorteio. Vazios, o pedido não leva `step` nem `seq`.
- O veredito aparece com palavra e cor, os cartões de grafo, os tempos e o subgrafo.
- **Replay:** as features mostradas são idênticas às da mesma linha no `results.jsonl` da Execução A. Confira com `curl -s http://127.0.0.1:8080/benchmarks/<run_id_de_A>/results | grep '"seq":<seq>'`. Se a A era sem grafo, compare com uma execução com grafo da mesma seed.
- Consultar com origem e destino vazios mostra a mensagem da API, sem quebrar a tela.

- [ ] **Passo 6: Aba 4, perfil transacional**

Abra a aba. Esperado: a fidelidade da árvore com vírgula decimal, uma tabela de perfis com regras legíveis (sem repetir a mesma feature duas vezes), gráficos de importância e de transações por perfil ocupando a largura, e o aviso de que as quantidades são da amostra. Troque o alvo para "Gabarito (isFraud)" e a profundidade para 2 e 5, e confira que a tabela muda e que o número de perfis respeita o limite.

- [ ] **Passo 7: API fora do ar e primeiro uso**

```bash
docker compose stop fraud-api
```

Clique em "Atualizar" no topo. Esperado: o resumo vira "Não consegui falar com a API em ..." e as abas avisam, sem erro do Gradio. Depois:

```bash
docker compose start fraud-api
```

Espere ficar saudável e clique em "Atualizar": tudo volta. Para o primeiro uso, rode `docker compose down -v`, `make up` e abra a tela sem nenhuma execução: os seletores ficam vazios e as abas mostram "rode um benchmark primeiro". (Isso apaga os dados, então rode `make seed` de novo depois.)

- [ ] **Passo 8: conferir a definição de pronto da spec**

Com o backend e o dashboard aceitos, confirme os quatro itens da seção de aceite da spec:
1. `make up` e `make seed` funcionam e as contagens do Neo4j batem com o CSV (plano 1, Tarefa 17).
2. Benchmark de N=200 com `tev1:0.8b` roda de ponta a ponta, com e sem grafo, sem erro (Passo 3 acima).
3. Benchmark de N=2000 com `tev1:4b` gera as 4 abas coerentes, e o replay da Aba 3 reproduz as features do benchmark (Passos 3 a 6 acima, com N=2000).
4. `make test-go`, `make test-integration` e `make test-py` passam.

- [ ] **Passo 9: registrar e commitar**

Anote no fim do `docs/superpowers/plans/2026-09-30-fraud-detector-dashboard.md` a data e qualquer observação da tela (por exemplo, o tempo de uma execução de 2000 com o `tev1:4b` na sua máquina).

```bash
git add docs/superpowers/plans
git commit -m "docs: registra o resultado do aceite do dashboard"
```

---
