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
