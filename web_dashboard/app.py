"""Tela do dashboard em Gradio. Só monta o layout e liga os eventos aos handlers."""

from __future__ import annotations

import os

import gradio as gr

import handlers
import plots
from client import BackendClient

POLL_SECONDS = 1.0

# No Gradio 6 os parâmetros head, css, js e theme passam do Blocks() para o launch().
HEAD = """
<link rel="preconnect" href="https://fonts.googleapis.com">
<link href="https://fonts.googleapis.com/css2?family=Geist:wght@400;500;600&family=Space+Grotesk:wght@500;600;700&display=swap" rel="stylesheet">
"""

# O padrão visual pede o tema claro para exportações e relatórios. Sem isso o Gradio segue o
# tema escuro do sistema, e os gráficos (feitos para fundo claro) ficam ilegíveis.
FORCE_LIGHT_JS = """
() => {
  const url = new URL(window.location);
  if (url.searchParams.get('__theme') !== 'light') {
    url.searchParams.set('__theme', 'light');
    window.location.href = url.href;
  }
}
"""

CSS = """
footer { display: none !important; }
h1, h2, h3, .kpi-title, .verdict-word { font-family: 'Space Grotesk', system-ui, sans-serif; }
h1, h2, h3 { color: #020c44; }
.kpi-title { display: flex; align-items: center; gap: 8px; font-weight: 600; color: #020c44; margin: 14px 0 8px; }
.kpi-dot { width: 10px; height: 10px; border-radius: 50%; display: inline-block; }
.kpi-row { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 12px; }
.kpi { background: #ffffff; border: 1px solid #e0e0d8; border-radius: 8px; padding: 12px 14px; }
.kpi-label { color: #162f36; font-size: 13px; }
.kpi-value { color: #020c44; font-family: 'Space Grotesk', system-ui, sans-serif; font-weight: 600;
  font-size: 26px; line-height: 1.25; }
.verdict { display: flex; flex-direction: column; gap: 4px; padding: 14px 18px; background: #ffffff;
  border: 1px solid #e0e0d8; border-left-width: 8px; border-radius: 8px; }
.verdict-fraud { border-left-color: #d13700; }
.verdict-legit { border-left-color: #3b7736; }
.verdict-word { font-weight: 700; font-size: 28px; color: #020c44; }
.verdict-meta { color: #162f36; font-size: 13px; }
"""

BRAND = gr.themes.Color(
    c50="#eef3ff",
    c100="#dbe6ff",
    c200="#b9ceff",
    c300="#8daeff",
    c400="#5a86ff",
    c500="#2f61ff",
    c600="#0038e5",
    c700="#002fc0",
    c800="#00279b",
    c900="#001f7a",
    c950="#020c44",
)
THEME = gr.themes.Soft(
    primary_hue=BRAND,
    neutral_hue=gr.themes.colors.slate,
    font=[gr.themes.GoogleFont("Geist"), "system-ui", "sans-serif"],
)


def build_app(client: BackendClient) -> gr.Blocks:
    with gr.Blocks(
        title="Detector de fraude com grafo",
        theme=THEME,
        css=CSS,
        head=HEAD,
        js=FORCE_LIGHT_JS,
        analytics_enabled=False,
    ) as demo:
        gr.Markdown("# Detector de fraude com Go, Neo4j e modelo local")
        overview = gr.Markdown()
        with gr.Row():
            run_a = gr.Dropdown(label="Execução A (base)", choices=[], value=None, scale=3)
            run_b = gr.Dropdown(label="Execução B", choices=[], value=None, scale=3)
            threshold = gr.Slider(
                0.0, 1.0, value=0.5, step=0.01, label="Limiar de decisão", scale=2
            )
            refresh = gr.Button("Atualizar", scale=1)

        with gr.Tabs():
            with gr.Tab("Benchmark e impacto do grafo"):
                with gr.Accordion("Rodar um benchmark", open=True):
                    mode = gr.Radio(
                        [handlers.MODE_COMPARE, handlers.MODE_NO_GRAPH, handlers.MODE_GRAPH],
                        value=handlers.MODE_COMPARE,
                        label="O que rodar",
                    )
                    with gr.Row():
                        n = gr.Number(
                            value=2000, precision=0, minimum=1, label="Quantidade de transações"
                        )
                        fraud_pct = gr.Slider(
                            0, 100, value=10, step=1, label="% de fraude na amostra"
                        )
                        model1 = gr.Dropdown(
                            label="Modelo", choices=[], value=None, allow_custom_value=True
                        )
                        seed = gr.Number(value=42, precision=0, minimum=0, label="Seed")
                    types = gr.CheckboxGroup(
                        handlers.TYPES,
                        value=["TRANSFER", "CASH_OUT"],
                        label="Tipos de transação da amostra",
                    )
                    with gr.Accordion("Parâmetros avançados", open=False):
                        with gr.Row():
                            workers = gr.Number(
                                value=32,
                                precision=0,
                                minimum=1,
                                label="Consultas simultâneas ao Neo4j",
                            )
                            window = gr.Number(
                                value=24,
                                precision=0,
                                minimum=1,
                                label="Janela em horas (fan-in e fan-out)",
                            )
                            hops = gr.Slider(1, 5, value=3, step=1, label="Saltos máximos do ciclo")
                            inflight = gr.Number(
                                value=4,
                                precision=0,
                                minimum=1,
                                label="Chamadas simultâneas ao modelo",
                            )
                    with gr.Row():
                        run_btn = gr.Button("Rodar", variant="primary")
                        cancel_btn = gr.Button("Cancelar a execução em andamento")
                    run_status = gr.Markdown()

                notices1 = gr.Markdown()
                metrics1 = gr.Dataframe(label="Métricas", interactive=False, wrap=True)
                with gr.Row():
                    plot_conf = gr.Plot(show_label=False)
                    plot_fpfn = gr.Plot(show_label=False)
                plot_roc = gr.Plot(show_label=False)
                with gr.Row():
                    plot_sweep = gr.Plot(show_label=False)
                    plot_types = gr.Plot(show_label=False)
                with gr.Accordion("Ver os dados dos gráficos como tabela", open=False):
                    sweep_table = gr.Dataframe(
                        label="F1 e demais métricas por limiar", interactive=False
                    )
                    types_table = gr.Dataframe(label="Fraudes por tipo", interactive=False)

            with gr.Tab("Performance e latência") as tab2:
                notices2 = gr.Markdown()
                kpis = gr.HTML(padding=False)
                plot_pct = gr.Plot(show_label=False)
                with gr.Row():
                    plot_hist_neo4j = gr.Plot(show_label=False)
                    plot_hist_jev = gr.Plot(show_label=False)
                with gr.Accordion("Ver as latências como tabela", open=False):
                    table2 = gr.Dataframe(interactive=False)

            with gr.Tab("Simulador"):
                with gr.Row():
                    with gr.Column(scale=2):
                        gr.Markdown("### Transação")
                        orig = gr.Textbox(label="Origem", max_lines=1)
                        dest = gr.Textbox(label="Destino", max_lines=1)
                        tipo = gr.Dropdown(handlers.TYPES, value="TRANSFER", label="Tipo")
                        amount = gr.Number(value=350000, label="Valor")
                        step = gr.Textbox(
                            value="",
                            max_lines=1,
                            placeholder="vazio",
                            label="Step (vazio usa o maior do dataset)",
                        )
                        with gr.Accordion("Saldos", open=False):
                            ob = gr.Number(value=350000, label="Saldo da origem antes")
                            oa = gr.Number(value=0, label="Saldo da origem depois")
                            db = gr.Number(value=0, label="Saldo do destino antes")
                            da = gr.Number(value=0, label="Saldo do destino depois")
                        seq = gr.Textbox(
                            value="",
                            max_lines=1,
                            placeholder="vazio",
                            label="seq (replay de uma transação real. Vazio significa transação de agora)",
                        )
                        model3 = gr.Dropdown(
                            label="Modelo", choices=[], value=None, allow_custom_value=True
                        )
                        with gr.Row():
                            sample_btn = gr.Button("Sortear da Execução A")
                            eval_btn = gr.Button("Consultar modelo e Neo4j", variant="primary")
                    with gr.Column(scale=3):
                        notice3 = gr.Markdown()
                        verdict = gr.HTML(padding=False)
                        features = gr.HTML(padding=False)
                        timings = gr.Markdown()
                        plot_subgraph = gr.Plot(
                            value=plots.empty_figure("Consulte uma transação para ver o subgrafo."),
                            show_label=False,
                        )
                        with gr.Accordion(
                            "Ver o state enviado e a resposta bruta do modelo", open=False
                        ):
                            state_json = gr.JSON(label="state enviado")
                            raw_json = gr.JSON(label="resposta bruta")

            with gr.Tab("Perfil transacional") as tab4:
                with gr.Row():
                    target = gr.Radio(
                        list(handlers.TARGETS),
                        value="Decisões do modelo",
                        label="O que a árvore imita",
                    )
                    depth = gr.Slider(2, 5, value=3, step=1, label="Profundidade da árvore")
                    profiles_btn = gr.Button("Gerar perfis", variant="primary")
                summary4 = gr.Markdown()
                profiles = gr.Dataframe(
                    label="Perfis",
                    interactive=False,
                    wrap=True,
                    column_widths=["8%", "40%", "10%", "10%", "11%", "11%", "10%"],
                )
                with gr.Row():
                    plot_importance = gr.Plot(show_label=False)
                    plot_counts = gr.Plot(show_label=False)
                with gr.Accordion("Ver a árvore e a importância como tabela", open=False):
                    tree_text = gr.Code(label="Árvore em texto", interactive=False, lines=14)
                    importance_table = gr.Dataframe(
                        label="Importância das features", interactive=False
                    )

        # ---------------------------------------------------------------- eventos

        def on_load(a, b):
            st = handlers.initial_state(client)
            validas = {valor for _, valor in st.choices}
            novo_a = a if a in validas else st.a
            novo_b = b if b in validas else st.b
            return (
                st.overview,
                gr.update(choices=st.choices, value=novo_a),
                gr.update(choices=st.choices, value=novo_b),
                # um dict para cada campo: o Gradio consome o dict ao aplicar a atualização
                gr.update(choices=st.models, value=st.default_model),
                gr.update(choices=st.models, value=st.default_model),
            )

        cabecalho = [overview, run_a, run_b, model1, model3]
        demo.load(on_load, [run_a, run_b], cabecalho, api_name="load")
        refresh.click(on_load, [run_a, run_b], cabecalho, api_name="refresh")

        # Aba 1
        saidas1 = [
            notices1,
            metrics1,
            plot_conf,
            plot_roc,
            plot_sweep,
            plot_fpfn,
            plot_types,
            sweep_table,
            types_table,
        ]

        def on_tab1(a, b, limiar):
            return handlers.build_tab1(client, a, b, limiar).as_tuple()

        gr.on(
            [run_a.change, run_b.change, threshold.release],
            on_tab1,
            [run_a, run_b, threshold],
            saidas1,
            api_name="tab1",
            show_progress="minimal",
        )

        def on_run(
            modo,
            qtd,
            pct,
            modelo,
            semente,
            tipos,
            trabalhadores,
            janela,
            saltos,
            simultaneas,
            atual_a,
            atual_b,
        ):
            params = {
                "n": int(qtd),
                "fraud_pct": float(pct),
                "seed": int(semente),
                "types": list(tipos),
                "workers": int(trabalhadores),
                "window_hours": int(janela),
                "cycle_max_hops": int(saltos),
                "max_inflight": int(simultaneas),
            }
            if modelo:
                params["model"] = modelo
            for atualizacao in handlers.run_benchmarks(
                client, modo, params, poll_seconds=POLL_SECONDS
            ):
                if not atualizacao.finished:
                    yield atualizacao.status, gr.skip(), gr.skip()
                    continue
                st = handlers.initial_state(client)
                yield (
                    atualizacao.status,
                    gr.update(choices=st.choices, value=atualizacao.a or atual_a),
                    gr.update(choices=st.choices, value=atualizacao.b or atual_b),
                )

        run_btn.click(
            on_run,
            [
                mode,
                n,
                fraud_pct,
                model1,
                seed,
                types,
                workers,
                window,
                hops,
                inflight,
                run_a,
                run_b,
            ],
            [run_status, run_a, run_b],
            api_name="run",
        )
        cancel_btn.click(
            lambda: handlers.cancel_active(client), None, run_status, api_name="cancel"
        )

        # Aba 2 (redesenha ao abrir, porque numa aba escondida o Plotly calcula largura zero)
        saidas2 = [notices2, kpis, plot_pct, plot_hist_neo4j, plot_hist_jev, table2]

        def on_tab2(a, b):
            return handlers.build_tab2(client, a, b).as_tuple()

        gr.on(
            [run_a.change, run_b.change, tab2.select],
            on_tab2,
            [run_a, run_b],
            saidas2,
            api_name="tab2",
            show_progress="minimal",
        )

        # Aba 3
        campos = [orig, dest, tipo, amount, step, ob, oa, db, da, seq]

        def on_sample(run_id):
            try:
                valores = list(handlers.sample_transaction(client, run_id))
            except handlers.UserMessage as aviso:
                gr.Warning(str(aviso))
                return tuple(gr.skip() for _ in campos)
            valores[4], valores[9] = (
                str(valores[4]),
                str(valores[9]),
            )  # step e seq são caixas de texto
            return tuple(valores)

        sample_btn.click(on_sample, [run_a], campos, api_name="tab3_sample")

        saidas3 = [notice3, verdict, features, timings, state_json, raw_json, plot_subgraph]

        def on_evaluate(*valores):
            *formulario, modelo, limiar = valores
            try:
                payload = handlers.build_evaluate_payload(*formulario, modelo, limiar)
            except handlers.UserMessage as aviso:
                return handlers.Tab3View.empty(f"**Erro:** {aviso}").as_tuple()
            return handlers.evaluate_view(client, payload).as_tuple()

        eval_btn.click(on_evaluate, [*campos, model3, threshold], saidas3, api_name="tab3_evaluate")

        # Aba 4 (também redesenha ao abrir)
        saidas4 = [summary4, plot_importance, profiles, plot_counts, tree_text, importance_table]

        def on_tab4(run_id, alvo, profundidade, limiar):
            return handlers.build_tab4(
                client, run_id, handlers.TARGETS[alvo], int(profundidade), limiar
            ).as_tuple()

        gr.on(
            [
                profiles_btn.click,
                run_b.change,
                threshold.release,
                target.change,
                depth.release,
                tab4.select,
            ],
            on_tab4,
            [run_b, target, depth, threshold],
            saidas4,
            api_name="tab4",
            show_progress="minimal",
        )

    return demo


def main() -> None:
    client = BackendClient(os.environ.get("BACKEND_API_URL", "http://127.0.0.1:8080"))
    demo = build_app(client)
    demo.queue().launch(
        server_name=os.environ.get("GRADIO_SERVER_NAME", "127.0.0.1"),
        server_port=int(os.environ.get("GRADIO_SERVER_PORT", "7860")),
        show_api=False,
    )


if __name__ == "__main__":
    main()
