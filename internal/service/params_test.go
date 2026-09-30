package service

import (
	"encoding/json"
	"slices"
	"testing"

	"fraud-detector-jev/internal/domain"
)

var limitesDeTeste = Limits{MaxSampleSize: 500000, MaxWorkers: 256, DefaultModel: "tev1:4b", BaseURL: "http://ollama:11434"}

func TestToParamsAplicaOsPadroes(t *testing.T) {
	p, err := RunRequest{}.ToParams(limitesDeTeste)
	if err != nil {
		t.Fatal(err)
	}
	want := domain.RunParams{
		N: 2000, FraudPct: 10, Seed: 42, Workers: 32, Model: "tev1:4b", BaseURL: "http://ollama:11434", Graph: true,
		Types: []string{"TRANSFER", "CASH_OUT"}, WindowHours: 24, CycleMaxHops: 3, MaxInflight: 4, Threshold: 0.5,
	}
	if p.N != want.N || p.FraudPct != want.FraudPct || p.Seed != want.Seed || p.Workers != want.Workers ||
		p.Model != want.Model || p.BaseURL != want.BaseURL || p.Graph != want.Graph || !slices.Equal(p.Types, want.Types) ||
		p.WindowHours != want.WindowHours || p.CycleMaxHops != want.CycleMaxHops || p.MaxInflight != want.MaxInflight ||
		p.Threshold != want.Threshold {
		t.Errorf("params = %+v\nesperado  %+v", p, want)
	}
}

func TestToParamsAceitaValoresDoPedidoInclusiveZeroEFalse(t *testing.T) {
	var req RunRequest
	corpo := `{"n":500,"fraud_pct":0,"seed":0,"workers":8,"model":"nimble","graph":false,
	           "types":["CASH_OUT","TRANSFER","CASH_OUT"],"window_hours":48,"cycle_max_hops":2,"max_inflight":1,"threshold":0}`
	if err := json.Unmarshal([]byte(corpo), &req); err != nil {
		t.Fatal(err)
	}
	p, err := req.ToParams(limitesDeTeste)
	if err != nil {
		t.Fatal(err)
	}
	if p.N != 500 || p.FraudPct != 0 || p.Seed != 0 || p.Workers != 8 || p.Model != "nimble" || p.Graph ||
		p.WindowHours != 48 || p.CycleMaxHops != 2 || p.MaxInflight != 1 || p.Threshold != 0 {
		t.Errorf("params = %+v", p)
	}
	if !slices.Equal(p.Types, []string{"CASH_OUT", "TRANSFER"}) {
		t.Errorf("types deveria vir sem repetição e na ordem do pedido: %v", p.Types)
	}
}

func TestToParamsRejeitaValoresInvalidosEJuntaOsProblemas(t *testing.T) {
	inteiro := func(n int) *int { return &n }
	decimal := func(f float64) *float64 { return &f }
	vazio := ""

	casos := map[string]RunRequest{
		"n zero":             {N: inteiro(0)},
		"n acima do limite":  {N: inteiro(500001)},
		"fraud_pct negativo": {FraudPct: decimal(-1)},
		"fraud_pct acima":    {FraudPct: decimal(100.5)},
		"workers zero":       {Workers: inteiro(0)},
		"workers acima":      {Workers: inteiro(257)},
		"modelo vazio":       {Model: &vazio},
		"tipo desconhecido":  {Types: []string{"REFUND"}},
		"tipos vazios":       {Types: []string{}},
		"janela zero":        {WindowHours: inteiro(0)},
		"janela acima":       {WindowHours: inteiro(745)},
		"saltos zero":        {CycleMaxHops: inteiro(0)},
		"saltos acima":       {CycleMaxHops: inteiro(6)},
		"inflight zero":      {MaxInflight: inteiro(0)},
		"inflight acima":     {MaxInflight: inteiro(65)},
		"limiar acima":       {Threshold: decimal(1.1)},
		"limiar negativo":    {Threshold: decimal(-0.1)},
	}
	for nome, req := range casos {
		t.Run(nome, func(t *testing.T) {
			_, err := req.ToParams(limitesDeTeste)
			var ve *ValidationError
			if !asValidation(err, &ve) || len(ve.Problems) != 1 {
				t.Fatalf("esperava 1 problema de validação, veio %v", err)
			}
		})
	}

	_, err := RunRequest{N: inteiro(0), Workers: inteiro(0), Types: []string{"X"}}.ToParams(limitesDeTeste)
	var ve *ValidationError
	if !asValidation(err, &ve) || len(ve.Problems) != 3 {
		t.Errorf("esperava 3 problemas juntos, veio %v", err)
	}
}

func asValidation(err error, alvo **ValidationError) bool {
	ve, ok := err.(*ValidationError)
	if ok {
		*alvo = ve
	}
	return ok
}
