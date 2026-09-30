package jev

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"fraud-detector-jev/internal/infrastructure/jev/jevtest"
)

func novoCliente(t *testing.T, srv *jevtest.Server, ajuste func(*Config)) *Client {
	t.Helper()
	cfg := Config{
		BaseURL:      srv.URL,
		Model:        "tev1:4b",
		Instructions: "Isso é fraude?",
		MaxInflight:  4,
		Timeout:      2 * time.Second,
		MaxAttempts:  3,
		BaseBackoff:  time.Millisecond,
	}
	if ajuste != nil {
		ajuste(&cfg)
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestClassifyMandaOPedidoNoFormatoDoContrato(t *testing.T) {
	srv := jevtest.New(jevtest.Options{})
	defer srv.Close()
	c := novoCliente(t, srv, nil)

	dec, err := c.Classify(context.Background(), `{"amount":350000}`)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Noul != 0.9 || dec.Attempts != 1 || dec.Latency <= 0 || dec.InputTokens < 1 || len(dec.Raw) == 0 {
		t.Errorf("decisão = %+v", dec)
	}

	var req struct {
		Model     string `json:"model"`
		State     string `json:"state"`
		Questions map[string]struct {
			Type         string            `json:"type"`
			Instructions string            `json:"instructions"`
			Criteria     map[string]string `json:"criteria"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(srv.LastBody(), &req); err != nil {
		t.Fatal(err)
	}
	q := req.Questions["is_fraud"]
	if req.Model != "tev1:4b" || req.State != `{"amount":350000}` || q.Type != "noul" || q.Instructions != "Isso é fraude?" {
		t.Errorf("pedido = %+v", req)
	}
	if q.Criteria["true"] == "" || q.Criteria["false"] == "" {
		t.Errorf("criteria faltando: %+v", q.Criteria)
	}
}

func TestClassifyTentaDeNovoEmErro5xx(t *testing.T) {
	srv := jevtest.New(jevtest.Options{FailFirst: 2})
	defer srv.Close()
	c := novoCliente(t, srv, nil)

	dec, err := c.Classify(context.Background(), `{"amount":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Attempts != 3 || srv.Calls() != 3 {
		t.Errorf("tentativas=%d chamadas=%d", dec.Attempts, srv.Calls())
	}
}

func TestClassifyDesisteDepoisDeMaxAttempts(t *testing.T) {
	srv := jevtest.New(jevtest.Options{FailFirst: 10})
	defer srv.Close()
	c := novoCliente(t, srv, nil)

	_, err := c.Classify(context.Background(), `{}`)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != 503 {
		t.Fatalf("esperava StatusError 503, veio %v", err)
	}
	if srv.Calls() != 3 {
		t.Errorf("chamadas = %d, esperava 3", srv.Calls())
	}
}

func TestClassifyNaoTentaDeNovoEm4xx(t *testing.T) {
	srv := jevtest.New(jevtest.Options{FailFirst: 10, FailStatus: 422})
	defer srv.Close()
	c := novoCliente(t, srv, nil)

	_, err := c.Classify(context.Background(), `{}`)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != 422 {
		t.Fatalf("esperava StatusError 422, veio %v", err)
	}
	if srv.Calls() != 1 {
		t.Errorf("chamadas = %d, esperava 1", srv.Calls())
	}
}

func TestClassifyRejeitaRespostasInvalidas(t *testing.T) {
	casos := map[string]string{
		"corpo vazio":      "",
		"não é JSON":       "isso não é json",
		"sem answers":      `{"model":"x"}`,
		"noul ausente":     `{"answers":{"is_fraud":{"type":"noul"}}}`,
		"noul acima de 1":  `{"answers":{"is_fraud":{"type":"noul","noul":1.5}}}`,
		"noul negativo":    `{"answers":{"is_fraud":{"type":"noul","noul":-0.1}}}`,
		"pergunta trocada": `{"answers":{"outra":{"type":"noul","noul":0.5}}}`,
		"noul como texto":  `{"answers":{"is_fraud":{"type":"noul","noul":"alto"}}}`,
		"formato antigo":   `{"answers":{"is_fraud":{"value":true,"probability":0.9,"confidence":0.9}}}`,
	}
	for nome, corpo := range casos {
		t.Run(nome, func(t *testing.T) {
			srv := jevtest.New(jevtest.Options{RawResponse: jevtest.Raw(corpo)})
			defer srv.Close()
			c := novoCliente(t, srv, nil)
			_, err := c.Classify(context.Background(), `{}`)
			if !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("esperava ErrInvalidResponse, veio %v", err)
			}
			if srv.Calls() != 1 {
				t.Errorf("resposta inválida não deveria ser tentada de novo, chamadas = %d", srv.Calls())
			}
		})
	}
}

func TestClassifyAplicaTimeoutPorTentativa(t *testing.T) {
	srv := jevtest.New(jevtest.Options{Delay: 300 * time.Millisecond})
	defer srv.Close()
	c := novoCliente(t, srv, func(cfg *Config) {
		cfg.Timeout = 50 * time.Millisecond
		cfg.MaxAttempts = 1
	})
	if _, err := c.Classify(context.Background(), `{}`); err == nil {
		t.Fatal("esperava erro de timeout")
	}
}

func TestClassifyLimitaChamadasSimultaneas(t *testing.T) {
	srv := jevtest.New(jevtest.Options{Delay: 40 * time.Millisecond})
	defer srv.Close()
	c := novoCliente(t, srv, func(cfg *Config) { cfg.MaxInflight = 2 })

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Classify(context.Background(), `{}`); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if vistas := srv.MaxInflightSeen(); vistas < 1 || vistas > 2 {
		t.Errorf("simultâneas vistas = %d, o limite era 2", vistas)
	}
}

func TestClassifyRespeitaCancelamentoDuranteOBackoff(t *testing.T) {
	srv := jevtest.New(jevtest.Options{FailFirst: 10})
	defer srv.Close()
	c := novoCliente(t, srv, func(cfg *Config) { cfg.BaseBackoff = 5 * time.Second })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	inicio := time.Now()
	_, err := c.Classify(ctx, `{}`)
	if err == nil {
		t.Fatal("esperava erro")
	}
	if time.Since(inicio) > 2*time.Second {
		t.Errorf("deveria ter parado logo, levou %v", time.Since(inicio))
	}
}

func TestNewValidaConfig(t *testing.T) {
	base := Config{BaseURL: "http://localhost:11434", Model: "tev1:4b", Instructions: "x", MaxInflight: 1}
	if _, err := New(base); err != nil {
		t.Fatalf("config válida deu erro: %v", err)
	}
	ruins := map[string]func(*Config){
		"url inválida":  func(c *Config) { c.BaseURL = "não é url" },
		"sem esquema":   func(c *Config) { c.BaseURL = "localhost:11434" },
		"sem modelo":    func(c *Config) { c.Model = " " },
		"sem instrução": func(c *Config) { c.Instructions = "" },
		"sem inflight":  func(c *Config) { c.MaxInflight = 0 },
	}
	for nome, muda := range ruins {
		t.Run(nome, func(t *testing.T) {
			cfg := base
			muda(&cfg)
			if _, err := New(cfg); err == nil {
				t.Error("esperava erro")
			}
		})
	}
}
