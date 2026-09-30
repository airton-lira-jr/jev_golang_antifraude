package web

import (
	"errors"
	"io"
	"net/http"

	"fraud-detector-jev/internal/domain"
	"fraud-detector-jev/internal/service"
)

func (s *Server) handleStartBenchmark(w http.ResponseWriter, r *http.Request) {
	var req service.RunRequest
	// Corpo vazio vale como "tudo padrão".
	if err := decodeJSON(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_json", "corpo inválido: "+err.Error())
		return
	}

	params, err := req.ToParams(s.Limits)
	var validation *service.ValidationError
	if errors.As(err, &validation) {
		writeError(w, http.StatusBadRequest, "validation_error", "pedido inválido", validation.Problems...)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	sum, err := s.Runs.Start(params)
	switch {
	case errors.Is(err, service.ErrBusy):
		writeError(w, http.StatusConflict, "run_in_progress", "já existe uma execução em andamento", "execução ativa: "+s.Runs.ActiveID())
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "start_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, sum)
}

func (s *Server) handleListBenchmarks(w http.ResponseWriter, _ *http.Request) {
	runs, err := s.Store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if runs == nil {
		runs = []domain.RunSummary{}
	}
	for i := range runs {
		if p, ok := s.Runs.Progress(runs[i].RunID); ok {
			runs[i].Progress = &p
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (s *Server) handleGetBenchmark(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sum, err := s.Store.ReadSummary(id)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "execução não encontrada: "+id)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if p, ok := s.Runs.Progress(id); ok {
		sum.Progress = &p
	}
	writeJSON(w, http.StatusOK, sum)
}

func (s *Server) handleCancelBenchmark(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.Runs.Cancel(id) {
		writeJSON(w, http.StatusAccepted, map[string]any{"canceled": true, "run_id": id})
		return
	}
	if _, err := s.Store.ReadSummary(id); errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "execução não encontrada: "+id)
		return
	}
	writeError(w, http.StatusConflict, "not_running", "a execução não está em andamento")
}

func (s *Server) handleResults(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sum, err := s.Store.ReadSummary(id)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "execução não encontrada: "+id)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if sum.Status == domain.StatusRunning {
		writeError(w, http.StatusConflict, "run_in_progress", "os resultados só ficam disponíveis quando a execução termina")
		return
	}

	results, err := s.Store.OpenResults(id)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "essa execução não gravou resultados")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	defer results.Close()

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, results)
}
