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
