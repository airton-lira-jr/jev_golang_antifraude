package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"fraud-detector-jev/internal/domain"
)

type fakeSubgraphs struct {
	beforeSeq int64
	atStep    int
	limit     int
	err       error
}

func (f *fakeSubgraphs) Subgraph(_ context.Context, orig, dest string, beforeSeq int64, atStep, limit int) (domain.Subgraph, error) {
	f.beforeSeq, f.atStep, f.limit = beforeSeq, atStep, limit
	if f.err != nil {
		return domain.Subgraph{}, f.err
	}
	return domain.Subgraph{Nodes: []string{orig, dest}, Edges: []domain.SubEdge{}}, nil
}

type montagem struct {
	ev       *Evaluator
	feats    *fakeFeatures
	sub      *fakeSubgraphs
	cl       *fakeClassifier
	consulta domain.FeatureQuery
	modelo   string
}

func novoAvaliador(noul float64) *montagem {
	m := &montagem{sub: &fakeSubgraphs{}}
	m.feats = &fakeFeatures{fn: func(q domain.FeatureQuery) (domain.GraphFeatures, error) {
		m.consulta = q
		return domain.GraphFeatures{DestInDegree: 42, CycleDetected: true}, nil
	}}
	m.cl = &fakeClassifier{fn: func(context.Context, string) (domain.Decision, error) {
		return domain.Decision{
			Noul: noul, InputTokens: 50, Attempts: 1, Latency: 3 * time.Millisecond,
			Raw: json.RawMessage(`{"answers":{"is_fraud":{"type":"noul","noul":0.8}}}`),
		}, nil
	}}
	m.ev = &Evaluator{
		Features: m.feats, Subgraphs: m.sub,
		ClassifierFor: func(model string) (Classifier, error) {
			m.modelo = model
			return m.cl, nil
		},
		DefaultModel: "tev1:4b", WindowHours: 24, CycleMaxHops: 3, DefaultThreshold: 0.5, SubgraphLimit: 50,
	}
	return m
}

func pedidoBase() EvaluateRequest {
	step := 31
	return EvaluateRequest{
		Orig: "C1", Dest: "C2", Type: "TRANSFER", Amount: 350000, Step: &step,
		OrigBalanceBefore: 350000, OrigBalanceAfter: 0,
	}
}

func TestEvaluateReplayUsaOPassadoDaTransacao(t *testing.T) {
	m := novoAvaliador(0.8)
	req := pedidoBase()
	seq := int64(1029)
	req.Seq = &seq

	resp, err := m.ev.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if m.consulta.BeforeSeq != 1029 || m.consulta.AtStep != 31 || m.consulta.Orig != "C1" || m.consulta.Dest != "C2" ||
		m.consulta.WindowHours != 24 || m.consulta.CycleMaxHops != 3 {
		t.Errorf("consulta de features = %+v", m.consulta)
	}
	if m.sub.beforeSeq != 1029 || m.sub.atStep != 31 || m.sub.limit != 50 {
		t.Errorf("consulta do subgrafo = %+v", m.sub)
	}
	if !resp.Replay || !resp.IsFraud || resp.Decision != "fraud" || resp.Noul != 0.8 || resp.Threshold != 0.5 || resp.Model != "tev1:4b" {
		t.Errorf("resposta = %+v", resp)
	}
	if resp.GraphFeatures.DestInDegree != 42 || !resp.GraphFeatures.CycleDetected {
		t.Errorf("features = %+v", resp.GraphFeatures)
	}
	if resp.Neo4jMs < 0 || resp.JevMs != 3 || len(resp.Subgraph.Nodes) != 2 {
		t.Errorf("tempos ou subgrafo errados: %+v", resp)
	}

	var estado map[string]any
	if err := json.Unmarshal(resp.State, &estado); err != nil {
		t.Fatalf("state deveria ser um objeto JSON: %v", err)
	}
	if estado["transaction_id"] != "TX_1029" || estado["time_step"] != float64(31) || estado["graph_features"] == nil {
		t.Errorf("state = %v", estado)
	}
	if !strings.Contains(string(resp.RawResponse), `"noul":0.8`) {
		t.Errorf("resposta bruta = %s", resp.RawResponse)
	}
}

func TestEvaluateReplayLevaAsColunasOpcionaisParaOState(t *testing.T) {
	m := novoAvaliador(0.8)
	req := pedidoBase()
	seq := int64(1029)
	req.Seq = &seq
	req.PaymentMethod, req.Channel, req.Category = "CONTACTLESS", "POS", "TRAVEL"

	resp, err := m.ev.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var estado map[string]any
	if err := json.Unmarshal(resp.State, &estado); err != nil {
		t.Fatal(err)
	}
	if estado["payment_method"] != "CONTACTLESS" || estado["channel"] != "POS" || estado["category"] != "TRAVEL" {
		t.Errorf("o replay precisa mandar as colunas opcionais no state, veio %v", estado)
	}

	semColunas, err := novoAvaliador(0.8).ev.Evaluate(context.Background(), pedidoBase())
	if err != nil {
		t.Fatal(err)
	}
	estado = map[string]any{}
	_ = json.Unmarshal(semColunas.State, &estado)
	for _, chave := range []string{"payment_method", "channel", "category"} {
		if _, existe := estado[chave]; existe {
			t.Errorf("pedido sem as colunas não deveria ter %s no state", chave)
		}
	}
}

func TestEvaluateSemSeqEhTransacaoDeAgora(t *testing.T) {
	m := novoAvaliador(0.2)
	resp, err := m.ev.Evaluate(context.Background(), pedidoBase())
	if err != nil {
		t.Fatal(err)
	}
	if m.consulta.BeforeSeq != math.MaxInt64 || m.sub.beforeSeq != math.MaxInt64 {
		t.Errorf("sem seq deveria enxergar tudo: features=%d subgrafo=%d", m.consulta.BeforeSeq, m.sub.beforeSeq)
	}
	if resp.Replay || resp.IsFraud || resp.Decision != "legit" {
		t.Errorf("resposta = %+v", resp)
	}
	var estado map[string]any
	_ = json.Unmarshal(resp.State, &estado)
	if estado["transaction_id"] != "TX_manual" {
		t.Errorf("transaction_id = %v", estado["transaction_id"])
	}
}

func TestEvaluateAplicaLimiarEModeloDoPedido(t *testing.T) {
	m := novoAvaliador(0.6)
	req := pedidoBase()
	limiar := 0.7
	req.Threshold = &limiar
	req.Model = "nimble"

	resp, err := m.ev.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.IsFraud || resp.Threshold != 0.7 || resp.Model != "nimble" || m.modelo != "nimble" {
		t.Errorf("resposta = %+v modelo pedido = %q", resp, m.modelo)
	}
}

func TestEvaluateUsaOStepPadraoQuandoOPedidoNaoTraz(t *testing.T) {
	m := novoAvaliador(0.5)
	m.ev.DefaultStep = func() (int, bool) { return 743, true }
	req := pedidoBase()
	req.Step = nil
	if _, err := m.ev.Evaluate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if m.consulta.AtStep != 743 {
		t.Errorf("AtStep = %d", m.consulta.AtStep)
	}

	m.ev.DefaultStep = nil
	if _, err := m.ev.Evaluate(context.Background(), req); err == nil {
		t.Error("sem step e sem padrão deveria dar erro de validação")
	}
}

// O replay só reproduz o score do benchmark se usar a janela e os saltos da execução de
// origem, e não os padrões do servidor.
func TestEvaluateUsaOsParametrosDaExecucaoDeOrigem(t *testing.T) {
	m := novoAvaliador(0.8)
	req := pedidoBase()
	seq, janela, saltos := int64(1029), 48, 5
	req.Seq, req.WindowHours, req.CycleMaxHops = &seq, &janela, &saltos

	resp, err := m.ev.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if m.consulta.WindowHours != 48 || m.consulta.CycleMaxHops != 5 {
		t.Errorf("consulta de features = %+v", m.consulta)
	}
	if !resp.Graph {
		t.Error("sem o campo graph o padrão é consultar o grafo")
	}
}

// Numa execução sem grafo o state do benchmark não leva features. O replay precisa mandar o
// mesmo state, e não consultar o Neo4j para as features.
func TestEvaluateSemGrafoNaoConsultaFeaturesEOStateSaiSemGrafo(t *testing.T) {
	m := novoAvaliador(0.8)
	req := pedidoBase()
	seq, semGrafo := int64(1029), false
	req.Seq, req.Graph = &seq, &semGrafo

	resp, err := m.ev.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if m.feats.calls.Load() != 0 {
		t.Error("sem grafo não pode consultar as features")
	}
	if resp.Graph || resp.GraphFeatures != (domain.GraphFeatures{}) || resp.Neo4jMs != 0 {
		t.Errorf("resposta sem grafo = %+v", resp)
	}
	var estado map[string]any
	if err := json.Unmarshal(resp.State, &estado); err != nil {
		t.Fatalf("state deveria ser um objeto JSON: %v", err)
	}
	if estado["transaction_id"] != "TX_1029" || estado["graph_features"] != nil {
		t.Errorf("state sem grafo = %v", estado)
	}
	if m.sub.beforeSeq != 1029 {
		t.Errorf("o subgrafo segue disponível e respeita o seq, veio %+v", m.sub)
	}
}

func TestEvaluateValidaOPedido(t *testing.T) {
	m := novoAvaliador(0.5)
	negativo, limiarAlto := int64(-1), 1.5
	passoNegativo := -3
	janelaZero, janelaGrande, saltosZero, saltosDemais := 0, 745, 0, 6
	casos := map[string]func(*EvaluateRequest){
		"janela zero":          func(r *EvaluateRequest) { r.WindowHours = &janelaZero },
		"janela acima de 744":  func(r *EvaluateRequest) { r.WindowHours = &janelaGrande },
		"saltos zero":          func(r *EvaluateRequest) { r.CycleMaxHops = &saltosZero },
		"saltos acima de 5":    func(r *EvaluateRequest) { r.CycleMaxHops = &saltosDemais },
		"origem vazia":         func(r *EvaluateRequest) { r.Orig = "  " },
		"destino vazio":        func(r *EvaluateRequest) { r.Dest = "" },
		"tipo inválido":        func(r *EvaluateRequest) { r.Type = "REFUND" },
		"valor negativo":       func(r *EvaluateRequest) { r.Amount = -1 },
		"step negativo":        func(r *EvaluateRequest) { r.Step = &passoNegativo },
		"seq negativo":         func(r *EvaluateRequest) { r.Seq = &negativo },
		"limiar fora de 0 a 1": func(r *EvaluateRequest) { r.Threshold = &limiarAlto },
	}
	for nome, muda := range casos {
		t.Run(nome, func(t *testing.T) {
			req := pedidoBase()
			muda(&req)
			_, err := m.ev.Evaluate(context.Background(), req)
			var ve *ValidationError
			if !asValidation(err, &ve) {
				t.Fatalf("esperava ValidationError, veio %v", err)
			}
		})
	}
	if m.feats.calls.Load() != 0 {
		t.Error("pedido inválido não pode chegar no Neo4j")
	}
}

func TestEvaluateMarcaFalhasDeFornecedorComOrigem(t *testing.T) {
	falha := errors.New("caiu")

	m := novoAvaliador(0.5)
	m.feats.fn = func(domain.FeatureQuery) (domain.GraphFeatures, error) { return domain.GraphFeatures{}, falha }
	_, err := m.ev.Evaluate(context.Background(), pedidoBase())
	var up *UpstreamError
	if !errors.As(err, &up) || up.Source != "neo4j" || !errors.Is(err, falha) {
		t.Errorf("features: %v", err)
	}

	m = novoAvaliador(0.5)
	m.cl.fn = func(context.Context, string) (domain.Decision, error) { return domain.Decision{}, falha }
	_, err = m.ev.Evaluate(context.Background(), pedidoBase())
	if !errors.As(err, &up) || up.Source != "modelo" {
		t.Errorf("modelo: %v", err)
	}

	m = novoAvaliador(0.5)
	m.sub.err = falha
	_, err = m.ev.Evaluate(context.Background(), pedidoBase())
	if !errors.As(err, &up) || up.Source != "neo4j" {
		t.Errorf("subgrafo: %v", err)
	}

	m = novoAvaliador(0.5)
	m.ev.ClassifierFor = func(string) (Classifier, error) { return nil, falha }
	_, err = m.ev.Evaluate(context.Background(), pedidoBase())
	if !errors.As(err, &up) || up.Source != "modelo" {
		t.Errorf("fábrica do modelo: %v", err)
	}
}
