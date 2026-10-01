"""Árvore substituta: uma árvore rasa que imita as decisões do modelo (ou o gabarito)
para explicar, com regras legíveis, o que pesa na decisão. Ela mostra o que a árvore
aprendeu sobre o alvo, não o que o modelo pensa por dentro. Por isso a fidelidade
sempre aparece junto."""

from __future__ import annotations

from dataclasses import dataclass, field

import numpy as np
import pandas as pd
from sklearn.model_selection import train_test_split
from sklearn.tree import DecisionTreeClassifier, export_text

import analysis

FIDELITY_MIN = 0.8
MIN_ROWS = 20

TABULAR_FEATURES = [
    "amount",
    "orig_balance_before",
    "orig_balance_after",
    "dest_balance_before",
    "dest_balance_after",
    "balance_drop",
    "dest_is_merchant",
    "step",
]
GRAPH_FEATURES = [
    "first_transfer_between_pair",
    "dest_in_degree",
    "dest_fan_in_window",
    "dest_fan_out_window",
    "orig_fan_out_window",
    "dest_account_age_steps",
    "cycle_detected",
]
BINARY_FEATURES = {"dest_is_merchant", "first_transfer_between_pair", "cycle_detected"}

# Colunas de texto do results.jsonl. Cada valor vira uma coluna 0 ou 1 chamada
# `<coluna>_<valor>`. O par é (rótulo curto, como aparece nas regras). As colunas que não
# existirem na execução (CSV sem elas) simplesmente não entram.
CATEGORICAL = {
    "type": ("tipo", "tipo"),
    "payment_method": ("pagamento", "forma de pagamento"),
    "channel": ("canal", "canal"),
    "category": ("categoria", "categoria"),
}

LABELS = {
    "amount": "valor",
    "orig_balance_before": "saldo da origem antes",
    "orig_balance_after": "saldo da origem depois",
    "dest_balance_before": "saldo do destino antes",
    "dest_balance_after": "saldo do destino depois",
    "balance_drop": "queda do saldo da origem",
    "dest_is_merchant": "destino é comerciante",
    "step": "hora (step)",
    "first_transfer_between_pair": "primeira transferência do par",
    "dest_in_degree": "remetentes do destino (histórico)",
    "dest_fan_in_window": "remetentes do destino na janela",
    "dest_fan_out_window": "destinatários do destino na janela",
    "orig_fan_out_window": "destinatários da origem na janela",
    "dest_account_age_steps": "idade do destino (horas)",
    "cycle_detected": "ciclo detectado",
}


def _split_categorical(feature: str) -> tuple[str, str] | None:
    """('channel', 'ATM') para 'channel_ATM'. None se a feature não for um valor de texto."""
    for coluna in CATEGORICAL:
        if feature.startswith(f"{coluna}_"):
            return coluna, feature[len(coluna) + 1 :]
    return None


def label(feature: str) -> str:
    """Nome legível de uma feature."""
    categorica = _split_categorical(feature)
    if categorica:
        coluna, valor = categorica
        return f"{CATEGORICAL[coluna][0]} {valor}"
    return LABELS.get(feature, feature)


@dataclass
class Surrogate:
    target: str
    n: int
    fidelity: float | None = None
    fidelity_train: float | None = None
    reliable: bool = False
    warning: str | None = None
    tree_text: str = ""
    importances: pd.DataFrame = field(
        default_factory=lambda: pd.DataFrame(columns=["feature", "rotulo", "importancia"])
    )
    profiles: pd.DataFrame = field(
        default_factory=lambda: pd.DataFrame(
            columns=[
                "perfil",
                "regra",
                "transacoes",
                "pct_amostra",
                "pct_fraude_modelo",
                "pct_fraude_real",
                "arvore_decide",
            ]
        )
    )
    counts: pd.DataFrame = field(
        default_factory=lambda: pd.DataFrame(columns=["perfil", "legitimas", "fraudes"])
    )


def feature_matrix(df: pd.DataFrame) -> pd.DataFrame:
    """Só números, sem nulos. O que faltava (por exemplo a idade de uma conta que nunca
    apareceu) vira -1. O tipo da transação e as colunas opcionais do CSV (forma de pagamento,
    canal e categoria) viram colunas 0 ou 1, uma por valor."""
    cols = {}
    for name in TABULAR_FEATURES + GRAPH_FEATURES:
        if name in df.columns:
            cols[name] = pd.to_numeric(df[name], errors="coerce").astype(float)
    x = pd.DataFrame(cols, index=df.index)
    for coluna in CATEGORICAL:
        if coluna in df.columns:
            for valor in sorted(df[coluna].dropna().unique()):
                x[f"{coluna}_{valor}"] = (df[coluna] == valor).astype(float)
    return x.fillna(-1.0)


def _num(valor: float) -> str:
    if abs(valor) >= 100:
        return f"{valor:,.0f}".replace(",", ".")
    return f"{valor:.2f}".replace(".", ",")


def _condition(name: str, lower: float | None, upper: float | None) -> str:
    """Uma condição sobre uma feature. lower (exclusivo) e upper (inclusivo) são None
    quando não há limite naquele lado."""
    categorica = _split_categorical(name)
    if categorica:
        coluna, valor = categorica
        nome = CATEGORICAL[coluna][1]
        return f"{nome} é {valor}" if lower is not None else f"{nome} não é {valor}"
    if name in BINARY_FEATURES:
        return f"{LABELS[name]}: {'sim' if lower is not None else 'não'}"
    partes = []
    if name == "dest_account_age_steps":
        if upper is not None and upper < 0:
            return "conta do destino nunca apareceu antes"
        if lower is not None and lower < 0:
            partes.append("conta do destino já apareceu antes")
            lower = None
    nome = label(name)
    if lower is not None and upper is not None:
        partes.append(f"{nome} entre {_num(lower)} e {_num(upper)}")
    elif upper is not None:
        partes.append(f"{nome} <= {_num(upper)}")
    elif lower is not None:
        partes.append(f"{nome} > {_num(lower)}")
    return " e ".join(partes)


def _describe(name: str, op: str, threshold: float) -> str:
    """Uma condição simples da árvore em português. op é '<=' ou '>'."""
    if op == "<=":
        return _condition(name, None, threshold)
    return _condition(name, threshold, None)


def _join_conditions(conds: list[tuple[str, str, float]]) -> str:
    """Junta as condições do caminho de uma folha. Cortes repetidos da mesma feature viram
    um só (vale o mais restrito), então a regra não repete `valor <= 40.885 e valor <= 22.603`."""
    limites: dict[str, list[float | None]] = {}
    for name, op, thr in conds:
        par = limites.setdefault(name, [None, None])
        if op == "<=":
            par[1] = thr if par[1] is None else min(par[1], thr)
        else:
            par[0] = thr if par[0] is None else max(par[0], thr)
    textos = [_condition(nome, lo, up) for nome, (lo, up) in limites.items()]
    return " e ".join(textos) if textos else "todas as transações"


def _leaf_rules(tree: DecisionTreeClassifier, names: list[str]) -> dict[int, str]:
    """A regra de cada folha: as condições do caminho da raiz até ela."""
    t = tree.tree_
    rules: dict[int, str] = {}

    def walk(node: int, conds: list[tuple[str, str, float]]) -> None:
        if t.children_left[node] == -1:
            rules[node] = _join_conditions(conds)
            return
        name, thr = names[t.feature[node]], float(t.threshold[node])
        walk(t.children_left[node], [*conds, (name, "<=", thr)])
        walk(t.children_right[node], [*conds, (name, ">", thr)])

    walk(0, [])
    return rules


def fit(
    df: pd.DataFrame, target: str, threshold: float, max_depth: int = 3, seed: int = 0
) -> Surrogate:
    """Treina a árvore substituta. target é 'model' (decisões do modelo no limiar) ou
    'truth' (gabarito is_fraud)."""
    if target not in ("model", "truth"):
        raise ValueError("target precisa ser 'model' ou 'truth'")
    if not 2 <= max_depth <= 5:
        raise ValueError("a profundidade precisa estar entre 2 e 5")

    d = analysis.scored(df).reset_index(drop=True)
    out = Surrogate(target=target, n=len(d))
    if len(d) < MIN_ROWS:
        out.warning = f"Só {len(d)} transações respondidas. Preciso de pelo menos {MIN_ROWS} para montar perfis."
        return out

    model_pred = analysis.predicted(d, threshold).to_numpy()
    real = d["is_fraud"].astype(bool).to_numpy()
    y = (model_pred if target == "model" else real).astype(int)
    if len(np.unique(y)) < 2:
        alvo = "as decisões do modelo" if target == "model" else "o gabarito"
        out.warning = (
            f"Todas as transações têm a mesma classe em {alvo}. "
            "Não dá para montar perfis. Mude o limiar ou o alvo."
        )
        return out

    x = feature_matrix(d)
    contagem = np.bincount(y)
    treino, teste = train_test_split(
        np.arange(len(d)),
        test_size=0.3,
        random_state=seed,
        stratify=y if contagem.min() >= 2 else None,
    )
    tree = DecisionTreeClassifier(
        max_depth=max_depth, min_samples_leaf=max(1, round(0.01 * len(treino))), random_state=seed
    )
    tree.fit(x.iloc[treino], y[treino])
    out.fidelity = float(tree.score(x.iloc[teste], y[teste]))
    out.fidelity_train = float(tree.score(x.iloc[treino], y[treino]))
    out.reliable = out.fidelity >= FIDELITY_MIN

    nomes = list(x.columns)
    imp = pd.DataFrame({"feature": nomes, "importancia": tree.feature_importances_})
    imp = (
        imp[imp["importancia"] > 0]
        .sort_values("importancia", ascending=False)
        .reset_index(drop=True)
    )
    imp.insert(1, "rotulo", imp["feature"].map(label))
    out.importances = imp
    out.tree_text = export_text(
        tree, feature_names=[label(n) for n in nomes], decimals=1, show_weights=True
    )

    # Um perfil por folha, contando todas as transações da amostra.
    folhas = tree.apply(x)
    regras = _leaf_rules(tree, nomes)
    quadro = pd.DataFrame({"folha": folhas, "modelo": model_pred, "real": real})
    grupos = quadro.groupby("folha").agg(
        transacoes=("folha", "size"),
        fraude_modelo=("modelo", "mean"),
        fraudes=("real", "sum"),
    )
    grupos = grupos.sort_values("transacoes", ascending=False, kind="stable")
    perfis = [f"Perfil {i + 1}" for i in range(len(grupos))]
    out.profiles = pd.DataFrame(
        {
            "perfil": perfis,
            "regra": [regras[f] for f in grupos.index],
            "transacoes": grupos["transacoes"].to_numpy(),
            "pct_amostra": grupos["transacoes"].to_numpy() / len(d) * 100,
            "pct_fraude_modelo": grupos["fraude_modelo"].to_numpy() * 100,
            "pct_fraude_real": grupos["fraudes"].to_numpy() / grupos["transacoes"].to_numpy() * 100,
            "arvore_decide": [
                "fraude" if np.argmax(tree.tree_.value[f][0]) == 1 else "legítima"
                for f in grupos.index
            ],
        }
    )
    out.counts = pd.DataFrame(
        {
            "perfil": perfis,
            "legitimas": (grupos["transacoes"] - grupos["fraudes"]).to_numpy(),
            "fraudes": grupos["fraudes"].to_numpy(),
        }
    )

    avisos = []
    if len(out.profiles) == 1:
        avisos.append(
            "A árvore não achou nenhuma regra que separe as decisões, então só há um perfil."
        )
    if not out.reliable:
        alvo = "das decisões do modelo" if target == "model" else "do gabarito"
        avisos.append(
            f"Atenção: a árvore reproduz só {out.fidelity * 100:.1f}% {alvo} em dados separados "
            f"(o mínimo é {FIDELITY_MIN * 100:.0f}%). Ela explica mal o alvo, então não confie nos perfis."
        )
    out.warning = " ".join(avisos) or None
    return out
