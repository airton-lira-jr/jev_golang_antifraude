package service

import (
	"fmt"
	"strings"

	"fraud-detector-jev/internal/domain"
)

// Padrões do POST /benchmarks (os mesmos da spec).
const (
	DefaultN            = 2000
	DefaultFraudPct     = 10.0
	DefaultSeed         = uint64(42)
	DefaultWorkers      = 32
	DefaultWindowHours  = 24
	DefaultCycleMaxHops = 3
	DefaultMaxInflight  = 4
	DefaultThreshold    = 0.5
)

// DefaultTypes são os tipos onde existe fraude no PaySim.
var DefaultTypes = []string{"TRANSFER", "CASH_OUT"}

// Limits são os limites e valores que vêm da configuração do servidor.
type Limits struct {
	MaxSampleSize int
	MaxWorkers    int
	DefaultModel  string
	BaseURL       string
}

// RunRequest é o corpo do POST /benchmarks. Os campos são ponteiros para dar
// para diferenciar "não veio" de "veio zero ou false".
type RunRequest struct {
	N            *int     `json:"n"`
	FraudPct     *float64 `json:"fraud_pct"`
	Seed         *uint64  `json:"seed"`
	Workers      *int     `json:"workers"`
	Model        *string  `json:"model"`
	Graph        *bool    `json:"graph"`
	Types        []string `json:"types"`
	WindowHours  *int     `json:"window_hours"`
	CycleMaxHops *int     `json:"cycle_max_hops"`
	MaxInflight  *int     `json:"max_inflight"`
	Threshold    *float64 `json:"threshold"`
}

// ValidationError junta todos os problemas de um pedido, para o usuário
// corrigir tudo de uma vez.
type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string {
	return "pedido inválido: " + strings.Join(e.Problems, ", ")
}

func valueOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

// ToParams aplica os padrões e valida. O BaseURL vem sempre do servidor.
func (r RunRequest) ToParams(l Limits) (domain.RunParams, error) {
	p := domain.RunParams{
		N:            valueOr(r.N, DefaultN),
		FraudPct:     valueOr(r.FraudPct, DefaultFraudPct),
		Seed:         valueOr(r.Seed, DefaultSeed),
		Workers:      valueOr(r.Workers, DefaultWorkers),
		Model:        strings.TrimSpace(valueOr(r.Model, l.DefaultModel)),
		BaseURL:      l.BaseURL,
		Graph:        valueOr(r.Graph, true),
		WindowHours:  valueOr(r.WindowHours, DefaultWindowHours),
		CycleMaxHops: valueOr(r.CycleMaxHops, DefaultCycleMaxHops),
		MaxInflight:  valueOr(r.MaxInflight, DefaultMaxInflight),
		Threshold:    valueOr(r.Threshold, DefaultThreshold),
	}

	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	if p.N < 1 || p.N > l.MaxSampleSize {
		add("n precisa estar entre 1 e %d, veio %d", l.MaxSampleSize, p.N)
	}
	if p.FraudPct < 0 || p.FraudPct > 100 {
		add("fraud_pct precisa estar entre 0 e 100, veio %v", p.FraudPct)
	}
	if p.Workers < 1 || p.Workers > l.MaxWorkers {
		add("workers precisa estar entre 1 e %d, veio %d", l.MaxWorkers, p.Workers)
	}
	if p.Model == "" {
		add("model não pode ser vazio")
	}
	if p.WindowHours < 1 || p.WindowHours > 744 {
		add("window_hours precisa estar entre 1 e 744, veio %d", p.WindowHours)
	}
	if p.CycleMaxHops < 1 || p.CycleMaxHops > 5 {
		add("cycle_max_hops precisa estar entre 1 e 5, veio %d", p.CycleMaxHops)
	}
	if p.MaxInflight < 1 || p.MaxInflight > 64 {
		add("max_inflight precisa estar entre 1 e 64, veio %d", p.MaxInflight)
	}
	if p.Threshold < 0 || p.Threshold > 1 {
		add("threshold precisa estar entre 0 e 1, veio %v", p.Threshold)
	}

	types := r.Types
	if types == nil {
		types = DefaultTypes
	}
	if len(types) == 0 {
		add("types não pode ser vazio")
	}
	for _, t := range types {
		switch {
		case !domain.ValidType(t):
			add("tipo desconhecido %q (use PAYMENT, TRANSFER, CASH_OUT, DEBIT ou CASH_IN)", t)
		case !contains(p.Types, t):
			p.Types = append(p.Types, t)
		}
	}

	if len(problems) > 0 {
		return domain.RunParams{}, &ValidationError{Problems: problems}
	}
	return p, nil
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
