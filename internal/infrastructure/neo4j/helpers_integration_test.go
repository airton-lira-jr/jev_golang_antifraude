//go:build integration

package neo4j

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func envOu(nome, padrao string) string {
	if v := os.Getenv(nome); v != "" {
		return v
	}
	return padrao
}

// clienteDeTeste conecta no Neo4j descartável e devolve o grafo vazio.
func clienteDeTeste(t *testing.T) *Client {
	t.Helper()
	c, err := New(context.Background(), Options{
		URI:          envOu("NEO4J_TEST_URI", "bolt://127.0.0.1:17687"),
		User:         "neo4j",
		Password:     envOu("NEO4J_TEST_PASSWORD", "testpassword123"),
		MaxPool:      10,
		QueryTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("não consegui conectar no Neo4j de teste (rode pelo scripts/test-integration.sh): %v", err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	limparGrafo(t, c)
	return c
}

func limparGrafo(t *testing.T, c *Client) {
	t.Helper()
	if _, err := c.write(context.Background(), `MATCH (n) DETACH DELETE n`, nil); err != nil {
		t.Fatal(err)
	}
}

func caminhoFixture() string {
	return filepath.Join("..", "..", "..", "testdata", "mini_paysim.csv")
}

// carregarFixture carrega o mini_paysim.csv (11 linhas, lotes de 4 para
// exercitar mais de um lote).
func carregarFixture(t *testing.T, c *Client) LoadReport {
	t.Helper()
	rep, err := c.Load(context.Background(), LoadOptions{CSVPath: caminhoFixture(), BatchSize: 4})
	if err != nil {
		t.Fatalf("carga da fixture falhou: %v", err)
	}
	return rep
}
