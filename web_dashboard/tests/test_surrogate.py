import numpy as np
import pandas as pd
import pytest

import surrogate
from tests.conftest import CATEGORICAL_COLUMNS, GRAPH_COLUMNS, make_results


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


def test_feature_matrix_com_as_colunas_opcionais_vira_0_ou_1():
    df = make_results(categorical=True)
    x = surrogate.feature_matrix(df)
    for coluna in CATEGORICAL_COLUMNS:
        for valor in df[coluna].unique():
            dummy = x[f"{coluna}_{valor}"]
            assert set(np.unique(dummy)) <= {0.0, 1.0} and dummy.dtype == float
            assert dummy.sum() == (df[coluna] == valor).sum()
    assert not x.isna().any().any()

    sem = surrogate.feature_matrix(make_results())
    prefixos = tuple(f"{coluna}_" for coluna in CATEGORICAL_COLUMNS)
    assert not [c for c in sem.columns if c.startswith(prefixos)]


def test_rotulos_e_regras_das_colunas_opcionais():
    assert surrogate.label("payment_method_PIX") == "pagamento PIX"
    assert surrogate.label("channel_WEB") == "canal WEB"
    assert surrogate.label("category_FOOD") == "categoria FOOD"
    d = surrogate._describe
    assert d("payment_method_PIX", ">", 0.5) == "forma de pagamento é PIX"
    assert d("payment_method_PIX", "<=", 0.5) == "forma de pagamento não é PIX"
    assert d("channel_ATM", ">", 0.5) == "canal é ATM"
    assert d("category_FOOD", "<=", 0.5) == "categoria não é FOOD"


def test_arvore_acha_a_coluna_opcional_que_separa_a_fraude():
    df = make_results(n=600, seed=3, categorical=True)
    df["channel"] = np.where(df["is_fraud"], "ATM", "WEB")  # só a fraude passa pelo ATM
    s = surrogate.fit(df, "truth", 0.5, max_depth=2)
    assert s.importances.loc[0, "feature"] in ("channel_ATM", "channel_WEB")
    assert s.importances.loc[0, "importancia"] > 0.9
    assert s.profiles["regra"].str.contains("canal").any()


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
