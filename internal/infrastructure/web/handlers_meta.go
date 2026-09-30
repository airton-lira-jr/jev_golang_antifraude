package web

import (
	"context"
	"net/http"
	"time"

	"fraud-detector-jev/internal/domain"
	"fraud-detector-jev/internal/infrastructure/jev"
	"fraud-detector-jev/internal/service"
)

const ollamaHint = "Confira se o Ollama está rodando no host e escutando em 0.0.0.0:11434 (veja o passo do OLLAMA_HOST no README)."

type componentStatus struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Hint  string `json:"hint,omitempty"`
}

type healthResponse struct {
	Status string          `json:"status"`
	Neo4j  componentStatus `json:"neo4j"`
	Ollama componentStatus `json:"ollama"`
}

// handleHealthz responde 503 só se o Neo4j caiu. O Ollama fora deixa o serviço
// "degraded" mas de pé, porque dá para carregar dados e olhar resultados antigos.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var resp healthResponse
	if err := s.PingNeo4j(ctx); err != nil {
		resp.Neo4j = componentStatus{Error: err.Error()}
	} else {
		resp.Neo4j.OK = true
	}
	if _, err := s.ListModels(ctx); err != nil {
		resp.Ollama = componentStatus{Error: err.Error(), Hint: ollamaHint}
	} else {
		resp.Ollama.OK = true
	}

	status := http.StatusOK
	switch {
	case !resp.Neo4j.OK:
		resp.Status, status = "unavailable", http.StatusServiceUnavailable
	case !resp.Ollama.OK:
		resp.Status = "degraded"
	default:
		resp.Status = "ok"
	}
	writeJSON(w, status, resp)
}

type infoDefaults struct {
	N            int      `json:"n"`
	FraudPct     float64  `json:"fraud_pct"`
	Seed         uint64   `json:"seed"`
	Workers      int      `json:"workers"`
	Graph        bool     `json:"graph"`
	Types        []string `json:"types"`
	WindowHours  int      `json:"window_hours"`
	CycleMaxHops int      `json:"cycle_max_hops"`
	MaxInflight  int      `json:"max_inflight"`
	Threshold    float64  `json:"threshold"`
}

type infoLimits struct {
	MaxSampleSize int `json:"max_sample_size"`
	MaxWorkers    int `json:"max_workers"`
}

type infoResponse struct {
	Model           string               `json:"model"`
	BaseURL         string               `json:"base_url"`
	OllamaOK        bool                 `json:"ollama_ok"`
	AvailableModels []string             `json:"available_models"`
	Dataset         *domain.DatasetStats `json:"dataset"`
	DatasetHint     string               `json:"dataset_hint,omitempty"`
	Defaults        infoDefaults         `json:"defaults"`
	Limits          infoLimits           `json:"limits"`
	ActiveRun       *string              `json:"active_run"`
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	resp := infoResponse{
		Model:           s.Limits.DefaultModel,
		BaseURL:         s.Limits.BaseURL,
		AvailableModels: []string{},
		Defaults: infoDefaults{
			N: service.DefaultN, FraudPct: service.DefaultFraudPct, Seed: service.DefaultSeed,
			Workers: service.DefaultWorkers, Graph: true, Types: service.DefaultTypes,
			WindowHours: service.DefaultWindowHours, CycleMaxHops: service.DefaultCycleMaxHops,
			MaxInflight: service.DefaultMaxInflight, Threshold: service.DefaultThreshold,
		},
		Limits: infoLimits{MaxSampleSize: s.Limits.MaxSampleSize, MaxWorkers: s.Limits.MaxWorkers},
	}

	if models, err := s.ListModels(ctx); err == nil {
		resp.OllamaOK = true
		if decision := jev.DecisionModels(models); decision != nil {
			resp.AvailableModels = decision
		}
	}
	if stats, err := s.Store.ReadDatasetStats(); err == nil {
		resp.Dataset = &stats
	} else {
		resp.DatasetHint = "O dataset ainda não foi carregado. Rode o seeder (make seed) e veja o README."
	}
	if id := s.Runs.ActiveID(); id != "" {
		resp.ActiveRun = &id
	}
	writeJSON(w, http.StatusOK, resp)
}
