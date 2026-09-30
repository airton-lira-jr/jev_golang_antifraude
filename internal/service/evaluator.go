package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"fraud-detector-jev/internal/domain"
)

// EvaluateRequest é o corpo do POST /transactions/evaluate. Seq presente
// significa replay de uma transação real. Step ausente usa DefaultStep.
type EvaluateRequest struct {
	Orig              string   `json:"orig"`
	Dest              string   `json:"dest"`
	Type              string   `json:"type"`
	Amount            float64  `json:"amount"`
	Step              *int     `json:"step"`
	OrigBalanceBefore float64  `json:"orig_balance_before"`
	OrigBalanceAfter  float64  `json:"orig_balance_after"`
	DestBalanceBefore float64  `json:"dest_balance_before"`
	DestBalanceAfter  float64  `json:"dest_balance_after"`
	Seq               *int64   `json:"seq"`
	Model             string   `json:"model"`
	Threshold         *float64 `json:"threshold"`
}

// EvaluateResponse é o que o simulador da Aba 3 mostra.
type EvaluateResponse struct {
	Decision      string               `json:"decision"`
	IsFraud       bool                 `json:"is_fraud"`
	Noul          float64              `json:"noul"`
	Threshold     float64              `json:"threshold"`
	Model         string               `json:"model"`
	Replay        bool                 `json:"replay"`
	GraphFeatures domain.GraphFeatures `json:"graph_features"`
	State         json.RawMessage      `json:"state"`
	RawResponse   json.RawMessage      `json:"raw_response"`
	Neo4jMs       float64              `json:"neo4j_ms"`
	JevMs         float64              `json:"jev_ms"`
	Subgraph      domain.Subgraph      `json:"subgraph"`
}

// UpstreamError marca falha de um fornecedor (Neo4j ou modelo). O handler HTTP
// responde 502 nesse caso.
type UpstreamError struct {
	Source string
	Err    error
}

func (e *UpstreamError) Error() string { return e.Source + ": " + e.Err.Error() }
func (e *UpstreamError) Unwrap() error { return e.Err }

// Evaluator é o caso de uso do simulador.
type Evaluator struct {
	Features         GraphFeatureSource
	Subgraphs        SubgraphSource
	ClassifierFor    func(model string) (Classifier, error)
	DefaultModel     string
	DefaultStep      func() (step int, ok bool)
	WindowHours      int
	CycleMaxHops     int
	DefaultThreshold float64
	SubgraphLimit    int
}

func (e *Evaluator) validate(req EvaluateRequest) (step int, threshold float64, err error) {
	var problems []string
	if strings.TrimSpace(req.Orig) == "" {
		problems = append(problems, "orig não pode ser vazio")
	}
	if strings.TrimSpace(req.Dest) == "" {
		problems = append(problems, "dest não pode ser vazio")
	}
	if !domain.ValidType(req.Type) {
		problems = append(problems, fmt.Sprintf("type desconhecido %q", req.Type))
	}
	if req.Amount < 0 {
		problems = append(problems, "amount não pode ser negativo")
	}
	if req.Seq != nil && *req.Seq < 0 {
		problems = append(problems, "seq não pode ser negativo")
	}

	switch {
	case req.Step != nil:
		step = *req.Step
	case e.DefaultStep != nil:
		if s, ok := e.DefaultStep(); ok {
			step = s
			break
		}
		problems = append(problems, "step é obrigatório (o dataset ainda não foi carregado)")
	default:
		problems = append(problems, "step é obrigatório")
	}
	if req.Step != nil && step < 0 {
		problems = append(problems, "step não pode ser negativo")
	}

	threshold = e.DefaultThreshold
	if req.Threshold != nil {
		threshold = *req.Threshold
		if threshold < 0 || threshold > 1 {
			problems = append(problems, "threshold precisa estar entre 0 e 1")
		}
	}
	if len(problems) > 0 {
		return 0, 0, &ValidationError{Problems: problems}
	}
	return step, threshold, nil
}

// Evaluate calcula as features, chama o modelo e monta o subgrafo.
func (e *Evaluator) Evaluate(ctx context.Context, req EvaluateRequest) (EvaluateResponse, error) {
	step, threshold, err := e.validate(req)
	if err != nil {
		return EvaluateResponse{}, err
	}

	replay := req.Seq != nil
	beforeSeq := int64(math.MaxInt64)
	tx := domain.Transaction{
		Step: step, Type: req.Type, Amount: req.Amount,
		Orig: req.Orig, OldBalanceOrig: req.OrigBalanceBefore, NewBalanceOrig: req.OrigBalanceAfter,
		Dest: req.Dest, OldBalanceDest: req.DestBalanceBefore, NewBalanceDest: req.DestBalanceAfter,
	}
	if replay {
		beforeSeq = *req.Seq
		tx.Seq = *req.Seq
	}

	t0 := time.Now()
	gf, err := e.Features.Features(ctx, domain.FeatureQuery{
		Orig: req.Orig, Dest: req.Dest, BeforeSeq: beforeSeq, AtStep: step,
		WindowHours: e.WindowHours, CycleMaxHops: e.CycleMaxHops,
	})
	neo4jMs := millis(time.Since(t0))
	if err != nil {
		return EvaluateResponse{}, &UpstreamError{Source: "neo4j", Err: err}
	}

	state := BuildState(tx, &gf)
	if !replay {
		state.TransactionID = "TX_manual"
	}
	stateJSON, err := state.JSON()
	if err != nil {
		return EvaluateResponse{}, fmt.Errorf("montando o state: %w", err)
	}

	model := req.Model
	if model == "" {
		model = e.DefaultModel
	}
	classifier, err := e.ClassifierFor(model)
	if err != nil {
		return EvaluateResponse{}, &UpstreamError{Source: "modelo", Err: err}
	}
	defer closeIfCloser(classifier)
	dec, err := classifier.Classify(ctx, stateJSON)
	if err != nil {
		return EvaluateResponse{}, &UpstreamError{Source: "modelo", Err: err}
	}

	sub, err := e.Subgraphs.Subgraph(ctx, req.Orig, req.Dest, beforeSeq, step, e.SubgraphLimit)
	if err != nil {
		return EvaluateResponse{}, &UpstreamError{Source: "neo4j", Err: err}
	}

	isFraud := domain.Decide(dec.Noul, threshold)
	decision := "legit"
	if isFraud {
		decision = "fraud"
	}
	return EvaluateResponse{
		Decision: decision, IsFraud: isFraud, Noul: dec.Noul, Threshold: threshold, Model: model, Replay: replay,
		GraphFeatures: gf, State: json.RawMessage(stateJSON), RawResponse: dec.Raw,
		Neo4jMs: neo4jMs, JevMs: millis(dec.Latency), Subgraph: sub,
	}, nil
}
