package web

import (
	"errors"
	"net/http"

	"fraud-detector-jev/internal/service"
)

func (s *Server) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	var req service.EvaluateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corpo inválido: "+err.Error())
		return
	}

	resp, err := s.Evaluator.Evaluate(r.Context(), req)
	var validation *service.ValidationError
	var upstream *service.UpstreamError
	switch {
	case errors.As(err, &validation):
		writeError(w, http.StatusBadRequest, "validation_error", "pedido inválido", validation.Problems...)
	case errors.As(err, &upstream):
		writeError(w, http.StatusBadGateway, "upstream_error", upstream.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
	default:
		writeJSON(w, http.StatusOK, resp)
	}
}
