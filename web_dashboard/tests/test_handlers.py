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
    orig, dest, tipo, amount, step, ob, oa, db, da, seq, *opcionais = valores
    assert opcionais == ["", "", ""]  # a execução não tem as colunas opcionais do CSV
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


def test_sample_transaction_leva_as_colunas_opcionais_da_execucao():
    df = make_results(graph=False, categorical=True)
    resumo = make_summary(run_id="run_20260930_160000_aaaa", graph=False)
    client = FakeClient(runs=[(resumo, df)])
    *_, seq, metodo, canal, categoria = handlers.sample_transaction(
        client, "run_20260930_160000_aaaa", rng=random.Random(1)
    )
    linha = df[df["seq"] == seq].iloc[0]
    assert (metodo, canal, categoria) == (
        linha["payment_method"],
        linha["channel"],
        linha["category"],
    )


def test_payload_leva_as_colunas_opcionais_so_quando_preenchidas():
    base = ("a", "b", "TRANSFER", 1, 5, 0, 0, 0, 0, None, None, 0.5)
    p = handlers.build_evaluate_payload(*base, payment_method=" PIX ", channel="", category=None)
    assert p["payment_method"] == "PIX"
    assert "channel" not in p and "category" not in p
    completo = handlers.build_evaluate_payload(*base, channel="WEB", category="FOOD")
    assert completo["channel"] == "WEB" and completo["category"] == "FOOD"
    assert not {"payment_method", "channel", "category"} & set(
        handlers.build_evaluate_payload(*base)
    )


def test_dica_da_caixa_de_execucoes_so_aparece_quando_ela_esta_vazia():
    assert "Nenhuma execução concluída" in handlers.run_selector_hint([])
    assert handlers.run_selector_hint([("2026-09-30 | sem grafo", "run_x")]) == ""


def test_replay_settings_le_os_parametros_da_execucao_de_origem():
    sem = make_summary(run_id="run_20260930_160000_aaaa", graph=False, model="nimble:latest")
    sem["params"]["window_hours"], sem["params"]["cycle_max_hops"] = 48, 5
    client = FakeClient(runs=[(sem, make_results(graph=False))])
    assert handlers.replay_settings(client, "run_20260930_160000_aaaa") == {
        "graph": False,
        "window_hours": 48,
        "cycle_max_hops": 5,
        "model": "nimble:latest",
    }


def test_payload_do_replay_leva_os_parametros_da_execucao_so_quando_ha_seq():
    cfg = {"graph": False, "window_hours": 48, "cycle_max_hops": 5, "model": "nimble:latest"}
    com_seq = handlers.build_evaluate_payload(
        "a", "b", "TRANSFER", 1, 5, 0, 0, 0, 0, 1029, "nimble:latest", 0.5, replay=cfg
    )
    assert com_seq["graph"] is False
    assert com_seq["window_hours"] == 48 and com_seq["cycle_max_hops"] == 5
    assert com_seq["model"] == "nimble:latest"

    sem_seq = handlers.build_evaluate_payload(
        "a", "b", "TRANSFER", 1, 5, 0, 0, 0, 0, None, "nimble:latest", 0.5, replay=cfg
    )
    assert not {"graph", "window_hours", "cycle_max_hops"} & set(sem_seq)


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


def test_evaluate_view_sem_grafo_avisa_e_nao_mostra_features():
    """O replay de uma execução sem grafo não consulta as features. A tela diz isso em vez
    de mostrar zeros que parecem uma conta sem histórico."""
    resp = evaluate_response(
        graph=False,
        graph_features={
            "first_transfer_between_pair": False,
            "dest_in_degree": 0,
            "dest_fan_in_window": 0,
            "dest_fan_out_window": 0,
            "orig_fan_out_window": 0,
            "dest_account_age_steps": None,
            "cycle_detected": False,
        },
    )
    v = handlers.evaluate_view(FakeClient(evaluate_response=resp), {"orig": "C1", "dest": "C2"})
    assert "sem features de grafo" in v.notice
    assert v.features == "" and "verdict-fraud" in v.verdict


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
