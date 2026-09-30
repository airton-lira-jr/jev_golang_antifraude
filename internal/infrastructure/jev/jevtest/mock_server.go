// Package jevtest tem um servidor falso do modelo, só para testes. Ele fica em
// um subpacote para o httptest não entrar no binário de produção.
package jevtest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"time"
)

// Options controla o comportamento do servidor falso.
type Options struct {
	// Score decide o noul a partir do state. Se for nil, usa a regra padrão:
	// amount acima de 200000 dá 0.9, senão 0.1.
	Score func(state string) float64
	// FailFirst faz as N primeiras requisições falharem com FailStatus.
	FailFirst int
	// FailStatus é o status das falhas simuladas. Padrão 503.
	FailStatus int
	// Delay espera antes de responder.
	Delay time.Duration
	// RawResponse, se não for nil, é devolvido como corpo em vez da resposta normal.
	RawResponse *string
}

// Raw devolve um ponteiro para s, para preencher Options.RawResponse.
func Raw(s string) *string { return &s }

// Server é o servidor falso.
type Server struct {
	*httptest.Server
	opts        Options
	calls       atomic.Int64
	inflight    atomic.Int64
	maxInflight atomic.Int64
	mu          sync.Mutex
	lastBody    []byte
}

// New sobe o servidor. Lembre de chamar Close.
func New(opts Options) *Server {
	s := &Server{opts: opts}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/systemone", s.handleSystemOne)
	mux.HandleFunc("GET /api/tags", s.handleTags)
	s.Server = httptest.NewServer(mux)
	return s
}

// Calls é o total de requisições ao /v1/systemone.
func (s *Server) Calls() int { return int(s.calls.Load()) }

// MaxInflightSeen é o maior número de requisições simultâneas já visto.
func (s *Server) MaxInflightSeen() int { return int(s.maxInflight.Load()) }

// LastBody é o corpo da última requisição ao /v1/systemone.
func (s *Server) LastBody() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.lastBody...)
}

func (s *Server) handleSystemOne(w http.ResponseWriter, r *http.Request) {
	n := s.calls.Add(1)
	cur := s.inflight.Add(1)
	defer s.inflight.Add(-1)
	for {
		seen := s.maxInflight.Load()
		if cur <= seen || s.maxInflight.CompareAndSwap(seen, cur) {
			break
		}
	}

	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.lastBody = body
	s.mu.Unlock()

	if s.opts.Delay > 0 {
		time.Sleep(s.opts.Delay)
	}
	if int(n) <= s.opts.FailFirst {
		status := s.opts.FailStatus
		if status == 0 {
			status = http.StatusServiceUnavailable
		}
		http.Error(w, "falha simulada", status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if s.opts.RawResponse != nil {
		_, _ = io.WriteString(w, *s.opts.RawResponse)
		return
	}

	var req struct {
		State string `json:"state"`
	}
	_ = json.Unmarshal(body, &req)
	score := defaultScore(req.State)
	if s.opts.Score != nil {
		score = s.opts.Score(req.State)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"model": "mock",
		"answers": map[string]any{
			"is_fraud": map[string]any{"type": "noul", "noul": score},
		},
		"usage": map[string]any{"input_tokens": len(req.State)/4 + 1},
	})
}

func (s *Server) handleTags(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"models":[{"name":"tev1:4b"},{"name":"tev1:0.8b"},{"name":"nimble:latest"},{"name":"llama3:8b"}]}`)
}

func defaultScore(state string) float64 {
	var st struct {
		Amount float64 `json:"amount"`
	}
	if err := json.Unmarshal([]byte(state), &st); err != nil {
		return 0.5
	}
	if st.Amount > 200000 {
		return 0.9
	}
	return 0.1
}
