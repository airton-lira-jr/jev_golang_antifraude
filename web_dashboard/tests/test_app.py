import gradio as gr
import pytest

import app
import handlers
from client import BackendError
from tests.conftest import make_results, make_summary
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
    overview, a, b, modelo1, modelo3, conhecido = ev.fn(*padrao(ev))
    assert "estilo Jev" in overview
    assert a["value"] == "run_20260930_160000_aaaa" and b["value"] == "run_20260930_161000_bbbb"
    assert len(a["choices"]) == 2
    assert modelo1["value"] == "tev1:4b" and modelo3["value"] == "tev1:4b"
    assert modelo3["choices"] == ["tev1:4b", "tev1:0.8b", "nimble:latest"]
    assert modelo1 is not modelo3  # o Gradio consome o dict, então cada campo precisa do seu


def test_atualizar_preserva_a_escolha_que_ainda_existe():
    demo = novo_app()
    ev = eventos(demo)["refresh"]
    _, a, b, _, _, _ = ev.fn("run_20260930_161000_bbbb", "run_20260930_160000_aaaa")
    assert a["value"] == "run_20260930_161000_bbbb" and b["value"] == "run_20260930_160000_aaaa"
    _, a, b, _, _, _ = ev.fn("run_que_sumiu", None)
    assert a["value"] == "run_20260930_160000_aaaa" and b["value"] == "run_20260930_161000_bbbb"


def test_abrir_a_tela_com_a_api_fora_do_ar_mostra_o_aviso():
    client = FakeClient()
    client.errors["info"] = BackendError("Não consegui falar com a API", code="unreachable")
    ev = eventos(novo_app(client))["load"]
    overview, a, b, _, _, _ = ev.fn(*padrao(ev))
    assert "Não consegui falar com a API" in overview and a["value"] is None and a["choices"] == []


def test_primeiro_uso_sem_nenhuma_execucao_nao_quebra_as_abas():
    demo = novo_app(FakeClient())
    for nome in ("tab1", "tab2", "tab4"):
        ev = eventos(demo)[nome]
        resultado = ev.fn(*padrao(ev))
        assert len(resultado) == len(ev.outputs)
    aviso = eventos(demo)["tab1"].fn(None, None, 0.5)[0]
    assert "rode um benchmark" in aviso.lower()


def test_caixa_vazia_explica_o_motivo_e_com_execucoes_a_dica_some():
    vazia = eventos(novo_app(FakeClient()))["load"]
    _, a, b, _, _, _ = vazia.fn(*padrao(vazia))
    assert "Nenhuma execução concluída" in a["info"] and "Nenhuma execução concluída" in b["info"]

    cheia = eventos(novo_app())["load"]
    _, a, b, _, _, _ = cheia.fn(*padrao(cheia))
    assert a["info"] == "" and b["info"] == ""


def test_a_lista_de_execucoes_se_atualiza_sozinha_quando_uma_termina():
    """Uma execução que termina em segundo plano (ou por fora da tela) não muda o valor dos
    seletores. Sem isso a lista só se atualizava ao clicar em Atualizar, e a caixa parecia
    quebrada."""
    client = FakeClient()
    evs = eventos(novo_app(client))
    assert "tick" in {evento for _, evento in evs["poll"].targets}
    *_, conhecido = evs["load"].fn(*padrao(evs["load"]))

    # nada mudou: a tela fica quieta, sem fechar uma caixa que o usuário esteja usando
    quieto = evs["poll"].fn(None, None, conhecido)
    assert len(quieto) == len(evs["poll"].outputs) and all(item == gr.skip() for item in quieto)

    # uma execução termina
    resumo, tabela = two_runs()[1]
    client.add_run(resumo, tabela)
    overview, a, b, novo = evs["poll"].fn(None, None, conhecido)
    assert [valor for _, valor in a["choices"]] == [resumo["run_id"]]
    assert a["info"] == "" and novo != conhecido
    assert b["value"] == resumo["run_id"]  # um único resultado vai para o B, como no default_pair


def test_atualizar_sozinho_preserva_a_escolha_do_usuario():
    client = FakeClient(runs=two_runs())
    evs = eventos(novo_app(client))
    *_, conhecido = evs["load"].fn(*padrao(evs["load"]))
    nova = make_summary(run_id="run_20260930_170000_cccc", graph=True)
    client.add_run(nova, make_results(seed=7, graph=True))

    _, a, b, _ = evs["poll"].fn("run_20260930_160000_aaaa", "run_20260930_161000_bbbb", conhecido)
    assert len(a["choices"]) == 3
    assert a["value"] == "run_20260930_160000_aaaa" and b["value"] == "run_20260930_161000_bbbb"


def test_primeiro_uso_desenha_a_aba_1_ao_abrir_e_ao_atualizar():
    """Sem nenhuma execução os seletores continuam None, então o change deles não dispara e a
    Aba 1 ficaria com o vazio cru do Gradio. Por isso ela também roda no fim de load e refresh."""
    demo = novo_app(FakeClient())
    evs = eventos(demo)
    for nome, origem in (("tab1_load", "load"), ("tab1_refresh", "refresh")):
        assert evs[nome].trigger_after == evs[origem]._id, nome
        assert len(evs[nome].outputs) == len(evs["tab1"].outputs), nome
        aviso = evs[nome].fn(None, None, 0.5)[0]
        assert "rode um benchmark" in aviso.lower(), nome


def test_com_execucao_escolhida_o_evento_encadeado_nao_redesenha_a_aba_1():
    """O change dos seletores já desenha a Aba 1. O encadeado só cobre o primeiro uso."""
    evs = eventos(novo_app())
    saida = evs["tab1_load"].fn("run_20260930_160000_aaaa", "run_20260930_161000_bbbb", 0.5)
    assert len(saida) == len(evs["tab1_load"].outputs)
    assert all(item == gr.skip() for item in saida)


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
    assert len(valores) == len(ev.outputs) == 15
    assert valores[12:] == ("", "", "")  # essa execução não tem as colunas opcionais do CSV
    assert isinstance(valores[4], str) and isinstance(
        valores[9], str
    )  # step e seq são caixas de texto
    assert valores[4].isdigit() and valores[9].isdigit()
    # o modelo e as configurações da execução de origem vão junto, para o replay reproduzir o score
    assert valores[10]["value"] == "tev1:4b"
    assert valores[11] == {
        "graph": False,
        "window_hours": 24,
        "cycle_max_hops": 3,
        "model": "tev1:4b",
    }

    sem = ev.fn(None)
    assert len(sem) == 15 and avisos and "Execução A" in avisos[0]


def test_sortear_preenche_as_colunas_opcionais_e_consultar_manda_no_pedido():
    resumo = make_summary(run_id="run_20260930_160000_aaaa", graph=False)
    df = make_results(graph=False, categorical=True)
    client = FakeClient(runs=[(resumo, df)], evaluate_response=evaluate_response())
    demo = novo_app(client)
    valores = eventos(demo)["tab3_sample"].fn("run_20260930_160000_aaaa")
    metodo, canal, categoria = valores[12:]
    assert metodo and canal and categoria  # vêm da linha sorteada

    ev = eventos(demo)["tab3_evaluate"]
    sem = padrao(ev)  # a tela abre com as caixas vazias
    ev.fn(*sem)
    assert not {"payment_method", "channel", "category"} & set(client.evaluated[0])
    com = padrao(ev)
    com[10:13] = [metodo, canal, categoria]
    ev.fn(*com)
    enviado = client.evaluated[1]
    assert (enviado["payment_method"], enviado["channel"], enviado["category"]) == (
        metodo,
        canal,
        categoria,
    )


def test_sortear_com_a_api_fora_do_ar_avisa_em_vez_de_estourar(monkeypatch):
    avisos = []
    monkeypatch.setattr(app.gr, "Warning", lambda msg, *a, **k: avisos.append(msg))
    client = FakeClient(runs=two_runs())
    client.errors["results"] = BackendError("Não consegui falar com a API", code="unreachable")
    ev = eventos(novo_app(client))["tab3_sample"]
    saida = ev.fn("run_20260930_160000_aaaa")
    assert len(saida) == len(ev.outputs) and all(item == gr.skip() for item in saida)
    assert avisos and "Não consegui falar com a API" in avisos[0]


def test_consultar_um_replay_leva_os_parametros_da_execucao_de_origem():
    client = FakeClient(evaluate_response=evaluate_response())
    ev = eventos(novo_app(client))["tab3_evaluate"]
    entradas = padrao(ev)
    entradas[9] = "1029"  # seq
    entradas[-1] = {"graph": False, "window_hours": 48, "cycle_max_hops": 5, "model": "tev1:4b"}
    ev.fn(*entradas)
    enviado = client.evaluated[0]
    assert enviado["seq"] == 1029 and enviado["graph"] is False
    assert enviado["window_hours"] == 48 and enviado["cycle_max_hops"] == 5


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
