package jev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ListModels pergunta ao Ollama quais modelos estão baixados (GET /api/tags).
// Serve também para saber se o Ollama está alcançável.
func ListModels(ctx context.Context, baseURL string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("falando com o Ollama em %s: %w", baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("o Ollama respondeu HTTP %d em /api/tags", resp.StatusCode)
	}
	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("lendo a lista de modelos: %w", err)
	}
	names := make([]string, 0, len(payload.Models))
	for _, m := range payload.Models {
		names = append(names, m.Name)
	}
	return names, nil
}

// DecisionModels deixa só os modelos de decisão (nimble e tev1), porque um
// modelo de chat comum não responde direito ao /v1/systemone.
func DecisionModels(all []string) []string {
	var out []string
	for _, name := range all {
		if strings.HasPrefix(name, "nimble") || strings.HasPrefix(name, "tev1") {
			out = append(out, name)
		}
	}
	return out
}
