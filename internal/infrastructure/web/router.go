package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"fraud-detector-jev/internal/domain"
	"fraud-detector-jev/internal/service"
)

// RunController é o que o servidor precisa do gerenciador de execuções.
type RunController interface {
	Start(params domain.RunParams) (domain.RunSummary, error)
	Cancel(id string) bool
	Progress(id string) (domain.Progress, bool)
	ActiveID() string
}

// RunReader é a leitura das execuções gravadas em disco.
type RunReader interface {
	ReadSummary(id string) (domain.RunSummary, error)
	List() ([]domain.RunSummary, error)
	OpenResults(id string) (io.ReadCloser, error)
	ReadDatasetStats() (domain.DatasetStats, error)
}

// TransactionEvaluator é o caso de uso do simulador.
type TransactionEvaluator interface {
	Evaluate(ctx context.Context, req service.EvaluateRequest) (service.EvaluateResponse, error)
}

// Server junta tudo que os handlers precisam. PingNeo4j e ListModels são
// funções para o teste não precisar de Neo4j nem de Ollama.
type Server struct {
	Runs       RunController
	Store      RunReader
	Evaluator  TransactionEvaluator
	Limits     service.Limits
	PingNeo4j  func(ctx context.Context) error
	ListModels func(ctx context.Context) ([]string, error)
	Log        *slog.Logger
}

// Handler devolve o http.Handler com todas as rotas e o middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /info", s.handleInfo)
	mux.HandleFunc("POST /benchmarks", s.handleStartBenchmark)
	mux.HandleFunc("GET /benchmarks", s.handleListBenchmarks)
	mux.HandleFunc("GET /benchmarks/{id}", s.handleGetBenchmark)
	mux.HandleFunc("DELETE /benchmarks/{id}", s.handleCancelBenchmark)
	mux.HandleFunc("GET /benchmarks/{id}/results", s.handleResults)
	mux.HandleFunc("POST /transactions/evaluate", s.handleEvaluate)
	// Qualquer coisa que não casou (rota ou método errado) vira 404 em JSON.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "rota não encontrada: "+r.Method+" "+r.URL.Path)
	})
	return s.middleware(mux)
}

func (s *Server) logger() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

// statusRecorder guarda o status para o log e sabe se já escreveu algo.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.wrote = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wrote = true
	return r.ResponseWriter.Write(b)
}

// Unwrap deixa o http.ResponseController chegar no writer de verdade.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// middleware segura panics (devolvendo 500 em JSON) e registra cada requisição.
func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if p := recover(); p != nil {
				s.logger().Error("panic no handler", "erro", p, "pilha", string(debug.Stack()))
				if !rec.wrote {
					writeError(rec, http.StatusInternalServerError, "internal_error", "erro interno do servidor")
				}
			}
			level := slog.LevelInfo
			if r.URL.Path == "/healthz" {
				level = slog.LevelDebug // o healthcheck do Docker bate a cada poucos segundos
			}
			s.logger().Log(r.Context(), level, "requisição",
				"método", r.Method, "caminho", r.URL.Path, "status", rec.status,
				"duração_ms", time.Since(start).Milliseconds())
		}()
		next.ServeHTTP(rec, r)
	})
}

type errorBody struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string, details ...string) {
	writeJSON(w, status, map[string]errorBody{"error": {Code: code, Message: message, Details: details}})
}

// decodeJSON lê o corpo com limite de 1 MB, recusa campo desconhecido e recusa
// conteúdo depois do objeto. Corpo vazio devolve io.EOF.
func decodeJSON(w http.ResponseWriter, r *http.Request, into any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("o corpo tem conteúdo depois do objeto JSON")
	}
	return nil
}
