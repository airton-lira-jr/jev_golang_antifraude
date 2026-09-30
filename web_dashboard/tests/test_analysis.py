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
