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
