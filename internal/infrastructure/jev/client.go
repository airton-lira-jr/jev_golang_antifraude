package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"

	"fraud-detector-jev/internal/domain"
)

const (
	questionKey   = "is_fraud"
	systemOnePath = "/v1/systemone"
	maxBodyBytes  = 1 << 20
)

// Textos do criteria da pergunta noul (o que conta como sim e como não).
const (
	criteriaTrue  = "A transação é fraude, esvaziamento ilícito ou lavagem de dinheiro."
	criteriaFalse = "A transação é legítima."
)

// ErrInvalidResponse aparece quando o corpo não tem answers.is_fraud.noul entre 0 e 1.
var ErrInvalidResponse = errors.New("resposta inválida do modelo")

// StatusError é um HTTP diferente de 200.
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("o modelo respondeu HTTP %d: %s", e.Code, e.Body)
}

// Config configura o cliente. MaxAttempts e BaseBackoff têm padrão (3 e 200ms).
type Config struct {
	BaseURL      string
	Model        string
	Instructions string
	MaxInflight  int
	Timeout      time.Duration
	MaxAttempts  int
	BaseBackoff  time.Duration
}

// Client fala com o /v1/systemone do Ollama. É seguro para uso concorrente.
type Client struct {
	cfg  Config
	http *http.Client
	sem  chan struct{}
	url  string
}

// New valida a configuração e cria o cliente.
func New(cfg Config) (*Client, error) {
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("JEV_BASE_URL inválida: %q", cfg.BaseURL)
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("o modelo não pode ser vazio")
	}
	if strings.TrimSpace(cfg.Instructions) == "" {
		return nil, errors.New("as instruções da pergunta não podem ser vazias")
	}
	if cfg.MaxInflight < 1 {
		return nil, errors.New("MaxInflight precisa ser pelo menos 1")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = 200 * time.Millisecond
	}

	c := &Client{
		cfg: cfg,
		http: &http.Client{Transport: &http.Transport{
			MaxIdleConnsPerHost: cfg.MaxInflight,
			IdleConnTimeout:     90 * time.Second,
		}},
		sem: make(chan struct{}, cfg.MaxInflight),
		url: strings.TrimRight(cfg.BaseURL, "/") + systemOnePath,
	}
	return c, nil
}

// Close solta as conexões ociosas. Chame quando terminar uma execução.
func (c *Client) Close() { c.http.CloseIdleConnections() }

type question struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

type request struct {
	Model     string              `json:"model"`
	State     string              `json:"state"`
	Questions map[string]question `json:"questions"`
}

type response struct {
	Answers map[string]struct {
		Type string   `json:"type"`
		Noul *float64 `json:"noul"`
	} `json:"answers"`
	Usage struct {
		InputTokens int `json:"input_tokens"`
	} `json:"usage"`
}

func (c *Client) buildBody(state string) ([]byte, error) {
	return json.Marshal(request{
		Model: c.cfg.Model,
		State: state,
		Questions: map[string]question{
			questionKey: {
				Type:         "noul",
				Instructions: c.cfg.Instructions,
				Criteria:     map[string]string{"true": criteriaTrue, "false": criteriaFalse},
			},
		},
	})
}

// Classify manda o state e devolve o noul. Faz retry só em erro de rede,
// timeout e 5xx.
func (c *Client) Classify(ctx context.Context, state string) (domain.Decision, error) {
	body, err := c.buildBody(state)
	if err != nil {
		return domain.Decision{}, fmt.Errorf("montando o pedido: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		dec, err := c.once(ctx, body)
		if err == nil {
			dec.Attempts = attempt
			return dec, nil
		}
		lastErr = err
		if ctx.Err() != nil || !retryable(err) {
			return domain.Decision{Attempts: attempt}, err
		}
		if attempt < c.cfg.MaxAttempts {
			if err := sleep(ctx, c.backoff(attempt)); err != nil {
				return domain.Decision{Attempts: attempt}, err
			}
		}
	}
	return domain.Decision{Attempts: c.cfg.MaxAttempts},
		fmt.Errorf("esgotou %d tentativas: %w", c.cfg.MaxAttempts, lastErr)
}

func (c *Client) once(ctx context.Context, body []byte) (domain.Decision, error) {
	select {
	case c.sem <- struct{}{}:
	case <-ctx.Done():
		return domain.Decision{}, ctx.Err()
	}
	defer func() { <-c.sem }()

	reqCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return domain.Decision{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return domain.Decision{}, fmt.Errorf("chamando o modelo: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	latency := time.Since(start)
	if err != nil {
		return domain.Decision{}, fmt.Errorf("lendo a resposta do modelo: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return domain.Decision{}, &StatusError{Code: resp.StatusCode, Body: truncate(string(raw), 200)}
	}

	noul, tokens, err := parseResponse(raw)
	if err != nil {
		return domain.Decision{}, err
	}
	return domain.Decision{Noul: noul, InputTokens: tokens, Latency: latency, Raw: raw}, nil
}

func parseResponse(raw []byte) (noul float64, tokens int, err error) {
	var r response
	if err := json.Unmarshal(raw, &r); err != nil {
		return 0, 0, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}
	ans, ok := r.Answers[questionKey]
	if !ok || ans.Noul == nil {
		return 0, 0, fmt.Errorf("%w: faltou answers.%s.noul", ErrInvalidResponse, questionKey)
	}
	if *ans.Noul < 0 || *ans.Noul > 1 {
		return 0, 0, fmt.Errorf("%w: noul %v fora do intervalo de 0 a 1", ErrInvalidResponse, *ans.Noul)
	}
	return *ans.Noul, r.Usage.InputTokens, nil
}

func retryable(err error) bool {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Code >= 500
	}
	return !errors.Is(err, ErrInvalidResponse)
}

func (c *Client) backoff(attempt int) time.Duration {
	d := c.cfg.BaseBackoff << (attempt - 1)
	return d + rand.N(d/2+1)
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
