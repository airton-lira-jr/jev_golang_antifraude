"""Dados sintéticos com o mesmo formato do results.jsonl e do summary.json do backend."""

from __future__ import annotations

import numpy as np
import pandas as pd
import pytest

GRAPH_COLUMNS = [
    "first_transfer_between_pair",
    "dest_in_degree",
    "dest_fan_in_window",
    "dest_fan_out_window",
    "orig_fan_out_window",
    "dest_account_age_steps",
    "cycle_detected",
]


def _sigmoid(x):
    return 1 / (1 + np.exp(-x))


def make_results(
    n: int = 400, seed: int = 0, graph: bool = True, n_errors: int = 0
) -> pd.DataFrame:
    """Gera n transações. Com a mesma seed, graph=True e graph=False trazem as mesmas
    transações e só mudam a nota do modelo e as colunas de grafo."""
    base = np.random.default_rng(seed)
    is_fraud = base.random(n) < 0.25
    tipo = np.where(base.random(n) < 0.5, "TRANSFER", "CASH_OUT")
    amount = np.where(is_fraud, base.lognormal(11.0, 0.6, n), base.lognormal(8.5, 1.0, n))
    cycle = base.random(n) < np.where(is_fraud, 0.5, 0.03)
    fan_in = base.poisson(np.where(is_fraud, 6, 1))
    in_degree = fan_in + base.poisson(2, n)
    first_pair = base.random(n) < np.where(is_fraud, 0.9, 0.6)
    age = base.integers(0, 200, n).astype(float)
    age[base.random(n) < 0.3] = np.nan
    fan_out_dest = base.poisson(1, n)
    fan_out_orig = base.poisson(1, n)
    before = amount * (1 + base.random(n))
    after = before - amount

    noise = np.random.default_rng(seed + 1000 + int(graph))
    if graph:
        score = _sigmoid(-3.5 + 3.0 * cycle + 0.35 * fan_in + 1.2 * (amount > 50000))
    else:
        score = _sigmoid(-2.0 + 3.0 * (amount > 50000))
    noul = np.clip(score + noise.normal(0, 0.05, n), 0, 1)

    df = pd.DataFrame(
        {
            "transaction_id": [f"TX_{i}" for i in range(n)],
            "seq": np.arange(n),
            "step": np.arange(n) // 10,
            "type": tipo,
            "orig": [f"C{i}" for i in range(n)],
            "dest": [f"D{i % 50}" for i in range(n)],
            "is_fraud": is_fraud,
            "noul": noul,
            "neo4j_ms": noise.lognormal(2.5, 0.4, n) if graph else np.full(n, np.nan),
            "jev_ms": noise.lognormal(1.5, 0.5, n),
            "attempts": 1,
            "input_tokens": 100 + (150 if graph else 0),
            "amount": amount,
            "orig_balance_before": before,
            "orig_balance_after": after,
            "dest_balance_before": 0.0,
            "dest_balance_after": amount,
            "balance_drop": amount,
            "dest_is_merchant": False,
        }
    )
    if graph:
        df["first_transfer_between_pair"] = first_pair
        df["dest_in_degree"] = in_degree
        df["dest_fan_in_window"] = fan_in
        df["dest_fan_out_window"] = fan_out_dest
        df["orig_fan_out_window"] = fan_out_orig
        df["dest_account_age_steps"] = age
        df["cycle_detected"] = cycle
    if n_errors:
        df.loc[: n_errors - 1, "noul"] = np.nan
        df["error"] = None
        df.loc[: n_errors - 1, "error"] = "modelo: conexão recusada"
    return df


def make_summary(
    run_id: str = "run_20260930_160931_2756",
    graph: bool = True,
    model: str = "tev1:4b",
    n: int = 400,
    prevalence: float = 0.003,
    status: str = "done",
    **extra,
) -> dict:
    """Um summary.json no formato da API."""
    neo4j = {"p50": 12.0, "p95": 30.0, "p99": 50.0} if graph else {"p50": 0, "p95": 0, "p99": 0}
    summary = {
        "run_id": run_id,
        "status": status,
        "started_at": "2026-09-30T16:09:31Z",
        "finished_at": "2026-09-30T16:10:31Z",
        "duration_s": 60.0,
        "params": {
            "n": n,
            "fraud_pct": 25,
            "seed": 42,
            "workers": 32,
            "model": model,
            "base_url": "http://host.docker.internal:11434",
            "graph": graph,
            "types": ["TRANSFER", "CASH_OUT"],
            "window_hours": 24,
            "cycle_max_hops": 3,
            "max_inflight": 4,
            "threshold": 0.5,
        },
        "sample": {
            "requested": n,
            "n": n,
            "n_fraud": n // 4,
            "n_legit": n - n // 4,
            "fraud_shortfall": False,
            "eligible_rows": 100000,
            "eligible_fraud": int(100000 * prevalence),
            "prevalence": prevalence,
        },
        "counts": {"tp": 80, "fp": 20, "tn": 280, "fn": 20, "errors": 0},
        "metrics": {
            "accuracy": 0.9,
            "precision": 0.8,
            "recall": 0.8,
            "f1": 0.8,
            "fpr": 0.07,
            "precision_at_prevalence": 0.03,
        },
        "latency": {"neo4j_ms": neo4j, "jev_ms": {"p50": 4.0, "p95": 9.0, "p99": 14.0}},
        "tps": 25.0,
        "tokens_total": 40000,
    }
    summary.update(extra)
    return summary


@pytest.fixture
def results_graph() -> pd.DataFrame:
    return make_results(seed=7, graph=True)


@pytest.fixture
def results_nograph() -> pd.DataFrame:
    return make_results(seed=7, graph=False)
