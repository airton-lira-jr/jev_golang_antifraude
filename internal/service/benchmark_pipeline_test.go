package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fraud-detector-jev/internal/domain"
)

func cfgPadrao() PipelineConfig {
	return PipelineConfig{Graph: true, Workers: 4, ClassifyWorkers: 2, WindowHours: 24, CycleMaxHops: 3, Threshold: 0.5, Warmup: 3}
}

// rodar executa o pipeline e junta o que o sink recebeu.
func rodar(t *testing.T, p Pipeline, cfg PipelineConfig, txs []domain.Transaction) (PipelineOutput, []domain.Result, error) {
	t.Helper()
	var mu sync.Mutex
	var recebidos []domain.Result
	out, err := p.Run(context.Background(), cfg, txs, func(r domain.Result) error {
		mu.Lock()
		defer mu.Unlock()
		recebidos = append(recebidos, r)
		return nil
	}, nil)
	return out, recebidos, err
}

func TestPipelineComGrafoCalculaMatrizELatencias(t *testing.T) {
	feats, cl := &fakeFeatures{}, &fakeClassifier{}
	out, resultados, err := rodar(t, Pipeline{Features: feats, Classifier: cl}, cfgPadrao(), txsDeTeste(9))
	if err != nil {
		t.Fatal(err)
	}
	if out.Counts.TP != 3 || out.Counts.TN != 6 || out.Counts.FP != 0 || out.Counts.FN != 0 || out.Counts.Errors != 0 {
		t.Errorf("matriz = %+v", out.Counts)
	}
	if out.Completed != 9 || len(resultados) != 9 {
		t.Errorf("completos=%d gravados=%d", out.Completed, len(resultados))
	}
	if out.TokensTotal != 90 {
		t.Errorf("tokens = %d", out.TokensTotal)
	}
	if out.Duration <= 0 || out.TPS <= 0 {
		t.Errorf("duração=%v tps=%v", out.Duration, out.TPS)
	}
	if out.Latency.Jev.P50 != 2 || out.Latency.Neo4j.P99 < 0 {
		t.Errorf("latências = %+v", out.Latency)
	}
	for _, r := range resultados {
		if r.Neo4jMs == nil || r.JevMs == nil || r.Noul == nil || r.GraphFeatures == nil {
			t.Fatalf("resultado incompleto: %+v", r)
		}
	}
	// 9 transações mais 4 consultas de aquecimento (uma por worker do pool).
	if feats.calls.Load() != 13 {
		t.Errorf("chamadas ao Neo4j = %d", feats.calls.Load())
	}
	// 9 transações mais 3 chamadas de aquecimento.
	if cl.calls.Load() != 12 {
		t.Errorf("chamadas ao modelo = %d", cl.calls.Load())
	}
	for _, s := range cl.statesVistos() {
		if seqDoState(s) >= 0 && !strings.Contains(s, "graph_features") {
			t.Errorf("state de execução com grafo sem graph_features: %s", s)
		}
	}
}

func TestPipelineSemGrafoPulaOEstagioDoNeo4j(t *testing.T) {
	feats, cl := &fakeFeatures{}, &fakeClassifier{}
	cfg := cfgPadrao()
	cfg.Graph = false
	out, resultados, err := rodar(t, Pipeline{Features: feats, Classifier: cl}, cfg, txsDeTeste(6))
	if err != nil {
		t.Fatal(err)
	}
	if feats.calls.Load() != 0 {
		t.Errorf("não deveria chamar o Neo4j, chamou %d vezes", feats.calls.Load())
	}
	for _, s := range cl.statesVistos() {
		if strings.Contains(s, "graph_features") {
			t.Errorf("state sem grafo não pode ter graph_features: %s", s)
		}
	}
	for _, r := range resultados {
		if r.Neo4jMs != nil || r.GraphFeatures != nil {
			t.Errorf("resultado sem grafo não pode ter campos de grafo: %+v", r)
		}
	}
	if out.Latency.Neo4j != (domain.LatencyStats{}) || out.Completed != 6 {
		t.Errorf("saída = %+v", out)
	}
}

func TestPipelineTrataErrosPorTransacao(t *testing.T) {
	feats := &fakeFeatures{fn: func(q domain.FeatureQuery) (domain.GraphFeatures, error) {
		if q.BeforeSeq == 2 {
			return domain.GraphFeatures{}, errors.New("timeout da query")
		}
		return domain.GraphFeatures{}, nil
	}}
	cl := &fakeClassifier{fn: func(_ context.Context, state string) (domain.Decision, error) {
		if seqDoState(state) == 3 {
			return domain.Decision{Attempts: 3}, errors.New("modelo caiu")
		}
		return domain.Decision{Noul: noulPorValor(state), InputTokens: 10, Attempts: 1, Latency: time.Millisecond}, nil
	}}
	out, resultados, err := rodar(t, Pipeline{Features: feats, Classifier: cl}, cfgPadrao(), txsDeTeste(9))
	if err != nil {
		t.Fatalf("erro de transação não pode derrubar a execução: %v", err)
	}
	if out.Counts.Errors != 2 || out.Completed != 7 || out.Counts.Total() != 7 || len(resultados) != 9 {
		t.Errorf("saída = %+v gravados=%d", out, len(resultados))
	}
	erros := map[int64]string{}
	for _, r := range resultados {
		if r.Error != "" {
			erros[r.Seq] = r.Error
			if r.Noul != nil {
				t.Errorf("transação com erro não pode ter noul: %+v", r)
			}
		}
	}
	if !strings.HasPrefix(erros[2], "neo4j:") || !strings.HasPrefix(erros[3], "modelo:") || len(erros) != 2 {
		t.Errorf("erros = %v", erros)
	}
}

func TestPipelineAplicaOLimiar(t *testing.T) {
	cl := &fakeClassifier{fn: func(context.Context, string) (domain.Decision, error) {
		return domain.Decision{Noul: 0.6, Attempts: 1, Latency: time.Millisecond}, nil
	}}
	p := Pipeline{Features: &fakeFeatures{}, Classifier: cl}

	cfg := cfgPadrao()
	cfg.Threshold = 0.5
	out, _, err := rodar(t, p, cfg, txsDeTeste(9))
	if err != nil || out.Counts.TP != 3 || out.Counts.FP != 6 {
		t.Errorf("limiar 0.5: %+v err=%v", out.Counts, err)
	}
	cfg.Threshold = 0.7
	out, _, err = rodar(t, p, cfg, txsDeTeste(9))
	if err != nil || out.Counts.FN != 3 || out.Counts.TN != 6 {
		t.Errorf("limiar 0.7: %+v err=%v", out.Counts, err)
	}
}

func TestPipelineCancelaSemContarEmVooComoErro(t *testing.T) {
	// As 3 primeiras chamadas de verdade respondem (a ordem entre elas não
	// importa). Da quarta em diante, tudo fica preso até cancelar. O aquecimento
	// não tem transaction_id e passa direto.
	var chamadas atomic.Int64
	cl := &fakeClassifier{fn: func(ctx context.Context, state string) (domain.Decision, error) {
		if seqDoState(state) >= 0 && chamadas.Add(1) > 3 {
			<-ctx.Done()
			return domain.Decision{}, ctx.Err()
		}
		return domain.Decision{Noul: 0.1, Attempts: 1, Latency: time.Millisecond}, nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var recebidos []domain.Result
	fim := make(chan error, 1)
	go func() {
		_, err := Pipeline{Features: &fakeFeatures{}, Classifier: cl}.Run(ctx, cfgPadrao(), txsDeTeste(20), func(r domain.Result) error {
			recebidos = append(recebidos, r)
			if len(recebidos) == 3 {
				cancel()
			}
			return nil
		}, nil)
		fim <- err
	}()

	select {
	case err := <-fim:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("esperava context.Canceled, veio %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("o pipeline não parou depois do cancelamento")
	}
	if len(recebidos) != 3 {
		t.Errorf("deveriam ter sobrado só os 3 resultados prontos, sobraram %d", len(recebidos))
	}
	for _, r := range recebidos {
		if r.Error != "" {
			t.Errorf("cancelamento virou erro de transação: %+v", r)
		}
	}
}

func TestPipelineFalhaRapidoQuandoOModeloNaoResponde(t *testing.T) {
	feats := &fakeFeatures{}
	cl := &fakeClassifier{fn: func(context.Context, string) (domain.Decision, error) {
		return domain.Decision{}, errors.New("conexão recusada")
	}}
	out, resultados, err := rodar(t, Pipeline{Features: feats, Classifier: cl}, cfgPadrao(), txsDeTeste(9))
	if err == nil || !strings.Contains(err.Error(), "aquecimento") {
		t.Fatalf("esperava erro de aquecimento, veio %v", err)
	}
	if len(resultados) != 0 || feats.calls.Load() != 0 || out.Completed != 0 {
		t.Errorf("não deveria ter processado nada: resultados=%d neo4j=%d", len(resultados), feats.calls.Load())
	}
}

func TestPipelineAqueceOPoolDoNeo4jAntesDeMedir(t *testing.T) {
	var aquecimento, reais atomic.Int64
	feats := &fakeFeatures{fn: func(q domain.FeatureQuery) (domain.GraphFeatures, error) {
		if q.Orig == warmupAccount {
			aquecimento.Add(1)
		} else {
			reais.Add(1)
		}
		return domain.GraphFeatures{}, nil
	}}
	out, resultados, err := rodar(t, Pipeline{Features: feats, Classifier: &fakeClassifier{}}, cfgPadrao(), txsDeTeste(6))
	if err != nil {
		t.Fatal(err)
	}
	if aquecimento.Load() != 4 || reais.Load() != 6 {
		t.Errorf("aquecimento=%d reais=%d, esperava 4 e 6 (4 workers)", aquecimento.Load(), reais.Load())
	}
	if out.Completed != 6 || len(resultados) != 6 {
		t.Errorf("o aquecimento não pode virar resultado: completos=%d gravados=%d", out.Completed, len(resultados))
	}

	// Sem grafo não há o que aquecer no Neo4j.
	semGrafo := cfgPadrao()
	semGrafo.Graph = false
	aquecimento.Store(0)
	reais.Store(0)
	if _, _, err := rodar(t, Pipeline{Features: feats, Classifier: &fakeClassifier{}}, semGrafo, txsDeTeste(3)); err != nil {
		t.Fatal(err)
	}
	if aquecimento.Load() != 0 || reais.Load() != 0 {
		t.Errorf("sem grafo não pode tocar no Neo4j: aquecimento=%d reais=%d", aquecimento.Load(), reais.Load())
	}
}

func TestPipelineFalhaRapidoQuandoONeo4jNaoResponde(t *testing.T) {
	feats := &fakeFeatures{fn: func(domain.FeatureQuery) (domain.GraphFeatures, error) {
		return domain.GraphFeatures{}, errors.New("conexão recusada")
	}}
	cl := &fakeClassifier{}
	out, resultados, err := rodar(t, Pipeline{Features: feats, Classifier: cl}, cfgPadrao(), txsDeTeste(9))
	if err == nil || !strings.Contains(err.Error(), "Neo4j") || !strings.Contains(err.Error(), "aquecimento") {
		t.Fatalf("esperava erro de aquecimento do Neo4j, veio %v", err)
	}
	if len(resultados) != 0 || out.Completed != 0 {
		t.Errorf("não deveria ter processado nada: %d resultados", len(resultados))
	}
}

func TestPipelinePropagaOErroDoSink(t *testing.T) {
	falha := errors.New("disco cheio")
	n := 0
	_, err := Pipeline{Features: &fakeFeatures{}, Classifier: &fakeClassifier{}}.Run(
		context.Background(), cfgPadrao(), txsDeTeste(30),
		func(domain.Result) error {
			n++
			if n == 2 {
				return falha
			}
			return nil
		}, nil)
	if !errors.Is(err, falha) {
		t.Fatalf("esperava o erro do sink, veio %v", err)
	}
}

func TestPipelineRespeitaOLimiteDeChamadasSimultaneas(t *testing.T) {
	cl := &fakeClassifier{delay: 20 * time.Millisecond}
	cfg := cfgPadrao()
	cfg.ClassifyWorkers = 2
	cfg.Warmup = 0
	if _, _, err := rodar(t, Pipeline{Features: &fakeFeatures{}, Classifier: cl}, cfg, txsDeTeste(12)); err != nil {
		t.Fatal(err)
	}
	if vistas := cl.maxInflight.Load(); vistas < 1 || vistas > 2 {
		t.Errorf("simultâneas = %d, o limite era 2", vistas)
	}
}

func TestPipelineReportaProgresso(t *testing.T) {
	var vistos []int
	_, err := Pipeline{Features: &fakeFeatures{}, Classifier: &fakeClassifier{}}.Run(
		context.Background(), cfgPadrao(), txsDeTeste(7),
		func(domain.Result) error { return nil },
		func(done int) { vistos = append(vistos, done) })
	if err != nil {
		t.Fatal(err)
	}
	if len(vistos) != 7 || vistos[0] != 1 || vistos[6] != 7 {
		t.Errorf("progresso = %v", vistos)
	}
}

func TestPipelineSemTransacoes(t *testing.T) {
	out, resultados, err := rodar(t, Pipeline{Features: &fakeFeatures{}, Classifier: &fakeClassifier{}}, cfgPadrao(), nil)
	if err != nil || out.Completed != 0 || len(resultados) != 0 || out.TPS != 0 {
		t.Errorf("out=%+v resultados=%d err=%v", out, len(resultados), err)
	}
}

func TestPipelineValidaConfiguracao(t *testing.T) {
	ok := Pipeline{Features: &fakeFeatures{}, Classifier: &fakeClassifier{}}
	semWorkers := cfgPadrao()
	semWorkers.Workers = 0
	if _, _, err := rodar(t, ok, semWorkers, txsDeTeste(3)); err == nil {
		t.Error("Workers 0 deveria dar erro")
	}
	semClassificadores := cfgPadrao()
	semClassificadores.ClassifyWorkers = 0
	if _, _, err := rodar(t, ok, semClassificadores, txsDeTeste(3)); err == nil {
		t.Error("ClassifyWorkers 0 deveria dar erro")
	}
	if _, _, err := rodar(t, Pipeline{Classifier: &fakeClassifier{}}, cfgPadrao(), txsDeTeste(3)); err == nil {
		t.Error("execução com grafo e sem fonte de features deveria dar erro")
	}
}
