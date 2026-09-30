"""Análise dos resultados de um benchmark. Só funções puras sobre DataFrames.

As fórmulas de classificação são as mesmas do backend em Go (veja
testdata/metrics_golden.json): fraude é a classe positiva, noul >= limiar é
fraude e toda divisão por zero dá 0.
"""

from __future__ import annotations

import numpy as np
import pandas as pd
from sklearn.metrics import roc_curve


def scored(df: pd.DataFrame) -> pd.DataFrame:
    """Só as transações em que o modelo respondeu."""
    if df.empty or "noul" not in df.columns:
        return df.iloc[0:0]
    return df[df["noul"].notna()]


def align(df_a: pd.DataFrame, df_b: pd.DataFrame) -> tuple[pd.DataFrame, pd.DataFrame]:
    """Só as transações presentes (e respondidas) nas duas execuções, na mesma ordem."""
    a, b = scored(df_a), scored(df_b)
    comuns = set(a["transaction_id"]) & set(b["transaction_id"])
    a = a[a["transaction_id"].isin(comuns)].sort_values("seq").reset_index(drop=True)
    b = b[b["transaction_id"].isin(comuns)].sort_values("seq").reset_index(drop=True)
    return a, b


def predicted(df: pd.DataFrame, threshold: float) -> pd.Series:
    return df["noul"] >= threshold


def confusion(df: pd.DataFrame, threshold: float) -> dict[str, int]:
    pred = predicted(df, threshold).to_numpy()
    real = df["is_fraud"].astype(bool).to_numpy()
    return {
        "tp": int((pred & real).sum()),
        "fp": int((pred & ~real).sum()),
        "tn": int((~pred & ~real).sum()),
        "fn": int((~pred & real).sum()),
    }


def _ratio(num: float, den: float) -> float:
    return float(num) / float(den) if den else 0.0


def precision_at_prevalence(tpr, fpr, prevalence):
    """Precisão reprojetada para outra proporção de fraude. Aceita números ou arrays.
    Onde o denominador é zero devolve 0."""
    tpr, fpr = np.asarray(tpr, dtype=float), np.asarray(fpr, dtype=float)
    den = tpr * prevalence + fpr * (1 - prevalence)
    with np.errstate(divide="ignore", invalid="ignore"):
        return np.where(den > 0, tpr * prevalence / den, 0.0)


def metrics(conf: dict[str, int], prevalence: float) -> dict[str, float]:
    tp, fp, tn, fn = conf["tp"], conf["fp"], conf["tn"], conf["fn"]
    precision = _ratio(tp, tp + fp)
    recall = _ratio(tp, tp + fn)
    fpr = _ratio(fp, fp + tn)
    return {
        "accuracy": _ratio(tp + tn, tp + fp + tn + fn),
        "precision": precision,
        "recall": recall,
        "f1": 2 * precision * recall / (precision + recall) if precision + recall else 0.0,
        "fpr": fpr,
        "precision_at_prevalence": float(precision_at_prevalence(recall, fpr, prevalence)),
    }


def roc_pr(df: pd.DataFrame, prevalence: float) -> dict | None:
    """Curvas ROC e PR. A PR é reprojetada para a prevalência real (TPR e FPR não
    mudam com a estratificação da amostra). Devolve None se só existir uma classe."""
    if df.empty:
        return None
    y = df["is_fraud"].astype(int).to_numpy()
    if y.min() == y.max():
        return None
    fpr, tpr, _ = roc_curve(y, df["noul"].to_numpy(dtype=float))
    den = tpr * prevalence + fpr * (1 - prevalence)
    with np.errstate(divide="ignore", invalid="ignore"):
        precision = np.where(den > 0, tpr * prevalence / den, 1.0)
    return {
        "fpr": fpr,
        "tpr": tpr,
        "auc_roc": float(np.trapezoid(tpr, fpr)),
        "recall": tpr,
        "precision": precision,
        "auc_pr": float(np.trapezoid(precision, tpr)),
    }


def threshold_sweep(df: pd.DataFrame, prevalence: float, steps: int = 101) -> pd.DataFrame:
    """Métricas para cada limiar de 0 a 1, para o gráfico de F1 por limiar."""
    real = df["is_fraud"].astype(bool).to_numpy()
    score = df["noul"].to_numpy(dtype=float)
    linhas = []
    for limiar in np.linspace(0, 1, steps):
        pred = score >= limiar
        conf = {
            "tp": int((pred & real).sum()),
            "fp": int((pred & ~real).sum()),
            "tn": int((~pred & ~real).sum()),
            "fn": int((~pred & real).sum()),
        }
        m = metrics(conf, prevalence)
        linhas.append({"limiar": float(limiar), **m, "fp": conf["fp"], "fn": conf["fn"]})
    return pd.DataFrame(linhas)


def fraud_types(df: pd.DataFrame, threshold: float) -> pd.DataFrame:
    """Quantas fraudes reais existem em cada tipo e quantas o modelo detectou."""
    colunas = ["tipo", "fraudes", "detectadas"]
    if df.empty:
        return pd.DataFrame(columns=colunas)
    d = df.assign(pred=predicted(df, threshold), real=df["is_fraud"].astype(bool))
    fraudes = d[d["real"]]
    if fraudes.empty:
        return pd.DataFrame(columns=colunas)
    g = (
        fraudes.groupby("type")
        .agg(fraudes=("real", "size"), detectadas=("pred", "sum"))
        .reset_index()
    )
    g = g.rename(columns={"type": "tipo"})
    g["fraudes"] = g["fraudes"].astype(int)
    g["detectadas"] = g["detectadas"].astype(int)
    return g[colunas]


def prevalence_of(summary: dict, df: pd.DataFrame | None = None) -> float:
    """Prevalência real de fraude do universo de onde saiu a amostra. Se o resumo não
    trouxer, usa a proporção da própria amostra (sem reprojeção)."""
    prev = (summary.get("sample") or {}).get("prevalence")
    if prev is not None:
        return float(prev)
    if df is not None and not df.empty:
        return float(df["is_fraud"].astype(bool).mean())
    return 0.0
