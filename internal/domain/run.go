package domain

import (
	"errors"
	"time"
)

// ErrNotFound é devolvido quando uma execução ou arquivo não existe.
var ErrNotFound = errors.New("não encontrado")

// RunStatus é o estado de uma execução de benchmark.
type RunStatus string

const (
	StatusRunning  RunStatus = "running"
	StatusDone     RunStatus = "done"
	StatusFailed   RunStatus = "failed"
	StatusCanceled RunStatus = "canceled"
)

// RunParams são os parâmetros de uma execução. BaseURL é preenchida pelo
// servidor e nunca vem do pedido.
type RunParams struct {
	N            int      `json:"n"`
	FraudPct     float64  `json:"fraud_pct"`
	Seed         uint64   `json:"seed"`
	Workers      int      `json:"workers"`
	Model        string   `json:"model"`
	BaseURL      string   `json:"base_url"`
	Graph        bool     `json:"graph"`
	Types        []string `json:"types"`
	WindowHours  int      `json:"window_hours"`
	CycleMaxHops int      `json:"cycle_max_hops"`
	MaxInflight  int      `json:"max_inflight"`
	Threshold    float64  `json:"threshold"`
}

// SampleInfo descreve a amostra e o universo de onde ela saiu.
type SampleInfo struct {
	Requested      int     `json:"requested"`
	N              int     `json:"n"`
	NFraud         int     `json:"n_fraud"`
	NLegit         int     `json:"n_legit"`
	FraudShortfall bool    `json:"fraud_shortfall"`
	EligibleRows   int64   `json:"eligible_rows"`
	EligibleFraud  int64   `json:"eligible_fraud"`
	Prevalence     float64 `json:"prevalence"`
}

// LatencyBlock junta os percentis do Neo4j e do modelo.
type LatencyBlock struct {
	Neo4j LatencyStats `json:"neo4j_ms"`
	Jev   LatencyStats `json:"jev_ms"`
}

// Progress é o andamento de uma execução ativa.
type Progress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// RunSummary é o conteúdo do summary.json.
type RunSummary struct {
	RunID       string        `json:"run_id"`
	Status      RunStatus     `json:"status"`
	Error       string        `json:"error,omitempty"`
	StartedAt   time.Time     `json:"started_at"`
	FinishedAt  *time.Time    `json:"finished_at,omitempty"`
	DurationS   float64       `json:"duration_s"`
	Params      RunParams     `json:"params"`
	Sample      *SampleInfo   `json:"sample,omitempty"`
	Counts      *Counts       `json:"counts,omitempty"`
	Metrics     *Metrics      `json:"metrics,omitempty"`
	Latency     *LatencyBlock `json:"latency,omitempty"`
	TPS         float64       `json:"tps"`
	TokensTotal int64         `json:"tokens_total"`
	Progress    *Progress     `json:"progress,omitempty"`
}
