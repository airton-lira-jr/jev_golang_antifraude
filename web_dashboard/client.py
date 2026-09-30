"""Cliente HTTP da API do backend. É a única parte do dashboard que faz rede."""

from __future__ import annotations

import json
from collections import OrderedDict

import pandas as pd
import requests

NUMERIC_COLUMNS = [
    "seq",
    "step",
    "noul",
    "neo4j_ms",
    "jev_ms",
    "attempts",
    "input_tokens",
    "amount",
    "orig_balance_before",
    "orig_balance_after",
    "dest_balance_before",
    "dest_balance_after",
    "balance_drop",
    "dest_in_degree",
    "dest_fan_in_window",
    "dest_fan_out_window",
    "orig_fan_out_window",
    "dest_account_age_steps",
]
BASIC_COLUMNS = ["transaction_id", "seq", "type", "is_fraud", "noul"]


class BackendError(Exception):
    """Erro devolvido pela API (ou falha de rede). O texto da exceção é a mensagem
    pronta para mostrar ao usuário."""

    def __init__(
        self,
        message: str,
        code: str = "backend_error",
        status: int | None = None,
        details: list[str] | None = None,
    ):
        super().__init__(message)
        self.code = code
        self.status = status
        self.details = details or []


class BackendClient:
    def __init__(
        self,
        base_url: str,
        timeout: float = 15.0,
        session: requests.Session | None = None,
        cache_size: int = 6,
    ):
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout
        self._session = session or requests.Session()
        self._cache: OrderedDict[str, pd.DataFrame] = OrderedDict()
        self._cache_size = cache_size

    def _request(
        self,
        method: str,
        path: str,
        *,
        body: dict | None = None,
        timeout: float | None = None,
        stream: bool = False,
    ) -> requests.Response:
        try:
            resp = self._session.request(
                method,
                f"{self.base_url}{path}",
                json=body,
                timeout=timeout or self.timeout,
                stream=stream,
            )
        except requests.RequestException as exc:
            # O erro original fica encadeado (__cause__), mas não vai para a tela: o texto cru
            # do urllib3 só confunde quem está usando o dashboard.
            raise BackendError(
                f"Não consegui falar com a API em {self.base_url}. Ela está no ar? "
                "Confira com `docker compose ps`.",
                code="unreachable",
            ) from exc
        if resp.status_code >= 400:
            raise self._to_error(resp)
        return resp

    @staticmethod
    def _to_error(resp: requests.Response) -> BackendError:
        try:
            err = resp.json()["error"]
            return BackendError(
                err.get("message", "erro sem mensagem"),
                err.get("code", "backend_error"),
                resp.status_code,
                err.get("details") or [],
            )
        except (ValueError, KeyError, TypeError, AttributeError):
            return BackendError(
                f"A API respondeu HTTP {resp.status_code}", "http_error", resp.status_code
            )

    def health(self) -> dict:
        return self._request("GET", "/healthz").json()

    def info(self) -> dict:
        return self._request("GET", "/info").json()

    def list_runs(self) -> list[dict]:
        return self._request("GET", "/benchmarks").json()["runs"]

    def get_run(self, run_id: str) -> dict:
        return self._request("GET", f"/benchmarks/{run_id}").json()

    def start_run(self, params: dict) -> dict:
        return self._request("POST", "/benchmarks", body=params).json()

    def cancel_run(self, run_id: str) -> dict:
        return self._request("DELETE", f"/benchmarks/{run_id}").json()

    def evaluate(self, payload: dict) -> dict:
        return self._request("POST", "/transactions/evaluate", body=payload, timeout=60).json()

    def results(self, run_id: str) -> pd.DataFrame:
        """Resultados de uma execução terminada. O DataFrame é compartilhado com o
        cache, então quem chama não deve alterá-lo."""
        if run_id in self._cache:
            self._cache.move_to_end(run_id)
            return self._cache[run_id]

        resp = self._request("GET", f"/benchmarks/{run_id}/results", timeout=120, stream=True)
        rows = [json.loads(line) for line in resp.iter_lines() if line]
        df = pd.DataFrame(rows) if rows else pd.DataFrame(columns=BASIC_COLUMNS)
        df = _coerce(df)

        self._cache[run_id] = df
        while len(self._cache) > self._cache_size:
            self._cache.popitem(last=False)
        return df


def _coerce(df: pd.DataFrame) -> pd.DataFrame:
    for col in NUMERIC_COLUMNS:
        if col in df.columns:
            df[col] = pd.to_numeric(df[col], errors="coerce")
    if "is_fraud" in df.columns:
        df["is_fraud"] = df["is_fraud"].astype(bool)
    return df
