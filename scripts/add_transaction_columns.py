#!/usr/bin/env python3
"""Acrescenta ao CSV do PaySim três colunas sorteadas ao acaso, no fim de cada linha.

As colunas novas não têm relação com isFraud. O sorteio é uniforme e independente por
linha, com seed fixa, então rodar de novo com a mesma seed gera o mesmo arquivo.
O leitor do backend só exige as dez primeiras colunas, então o resto do projeto segue
funcionando. As linhas continuam na mesma ordem, e por isso o seq não muda.

Uso: scripts/add_transaction_columns.py ENTRADA SAIDA [--seed 42]
O arquivo de entrada nunca é alterado.
"""

from __future__ import annotations

import argparse
import random
import sys
from pathlib import Path

# Cada coluna tem no máximo 7 valores.
COLUMNS: dict[str, list[str]] = {
    "paymentMethod": [
        "CREDIT_CARD",
        "DEBIT_CARD",
        "CONTACTLESS",
        "PIX",
        "BOLETO",
        "DIGITAL_WALLET",
    ],
    "channel": ["MOBILE_APP", "WEB", "ATM", "BRANCH", "POS", "PHONE"],
    "category": ["SHOPPING", "FOOD", "BILLS", "TRAVEL", "SERVICES", "EDUCATION", "HEALTH"],
}
MAX_VALUES = 7
BASE_COLUMNS = 11  # as dez obrigatórias mais isFlaggedFraud


def enrich(source: Path, target: Path, seed: int) -> int:
    """Copia source para target com as colunas novas. Devolve quantas linhas de dados."""
    if source.resolve() == target.resolve():
        raise SystemExit("A saída precisa ser outro arquivo, a entrada não é alterada.")
    for nome, valores in COLUMNS.items():
        if len(valores) > MAX_VALUES:
            raise SystemExit(f"A coluna {nome} tem {len(valores)} valores, o máximo é {MAX_VALUES}.")

    rng = random.Random(seed)
    listas = list(COLUMNS.values())
    linhas = 0
    with source.open(newline="") as entrada, target.open("w", newline="") as saida:
        cabecalho = entrada.readline().rstrip("\r\n")
        campos = cabecalho.split(",")
        if len(campos) != BASE_COLUMNS:
            raise SystemExit(f"Esperava {BASE_COLUMNS} colunas e o cabeçalho tem {len(campos)}.")
        repetidas = set(campos) & set(COLUMNS)
        if repetidas:
            raise SystemExit(f"O arquivo já tem as colunas {sorted(repetidas)}.")
        saida.write(",".join([*campos, *COLUMNS]) + "\n")

        for numero, linha in enumerate(entrada, start=2):
            linha = linha.rstrip("\r\n")
            if linha.count(",") != BASE_COLUMNS - 1 or '"' in linha:
                raise SystemExit(f"Linha {numero} fora do formato esperado: {linha[:80]!r}")
            sorteio = ",".join(rng.choice(valores) for valores in listas)
            saida.write(f"{linha},{sorteio}\n")
            linhas += 1
    return linhas


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    parser.add_argument("entrada", type=Path)
    parser.add_argument("saida", type=Path)
    parser.add_argument("--seed", type=int, default=42)
    args = parser.parse_args()

    linhas = enrich(args.entrada, args.saida, args.seed)
    print(f"{linhas} linhas gravadas em {args.saida}", file=sys.stderr)


if __name__ == "__main__":
    main()
