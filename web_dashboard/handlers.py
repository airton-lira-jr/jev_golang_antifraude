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


def run_selector_hint(choices: list) -> str:
    """Texto de apoio das caixas A e B. Uma caixa vazia parece quebrada, então ela explica."""
    if choices:
        return ""
    return (
        "Nenhuma execução concluída ainda. Rode um benchmark abaixo e a lista se atualiza sozinha."
    )


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
    replay: dict | None = None,
) -> dict:
    """Monta o corpo do POST /transactions/evaluate a partir dos campos da tela. `replay` são
    as configurações da execução de origem (veja `replay_settings`). Só valem com `seq`, porque
    uma transação de agora não vem de execução nenhuma."""
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
    numero_seq = _optional_int(seq, "seq")
    if numero_seq is not None:
        payload["seq"] = numero_seq
        if replay:
            for chave in ("graph", "window_hours", "cycle_max_hops"):
                if chave in replay:
                    payload[chave] = replay[chave]
    if model:
        payload["model"] = model
    return payload


def replay_settings(client: BackendClient, run_id: str) -> dict:
    """As configurações da execução que o replay precisa repetir para reproduzir o score:
    se usou grafo, a janela, os saltos do ciclo e o modelo."""
    params = client.get_run(run_id).get("params") or {}
    return {
        "graph": bool(params.get("graph", True)),
        "window_hours": params.get("window_hours"),
        "cycle_max_hops": params.get("cycle_max_hops"),
        "model": params.get("model"),
    }


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
    com_grafo = resp.get("graph", True)
    return Tab3View(
        notice=""
        if com_grafo
        else "Consulta **sem features de grafo**, igual à execução de origem. O state enviado "
        "não tem a parte de grafo.",
        verdict=verdict_html(resp),
        features=features_html(resp["graph_features"]) if com_grafo else "",
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
