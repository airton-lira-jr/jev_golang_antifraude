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
