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
