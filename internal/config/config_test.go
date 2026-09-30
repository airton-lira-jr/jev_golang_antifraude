package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadUsaPadroes(t *testing.T) {
	cfg, err := Load(env(map[string]string{"NEO4J_PASSWORD": "segredo123"}))
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.Neo4jURI != "bolt://localhost:7687" || cfg.Neo4jUser != "neo4j" {
		t.Errorf("neo4j = %q %q", cfg.Neo4jURI, cfg.Neo4jUser)
	}
	if cfg.Neo4jQueryTimeout != 2*time.Second {
		t.Errorf("Neo4jQueryTimeout = %v", cfg.Neo4jQueryTimeout)
	}
	if cfg.JevBaseURL != "http://host.docker.internal:11434" || cfg.JevModel != "tev1:4b" {
		t.Errorf("jev = %q %q", cfg.JevBaseURL, cfg.JevModel)
	}
	if cfg.JevMaxInflight != 4 || cfg.JevTimeout != 30*time.Second {
		t.Errorf("limites do modelo = %d %v", cfg.JevMaxInflight, cfg.JevTimeout)
	}
	if cfg.MaxSampleSize != 500000 || cfg.MaxWorkers != 256 {
		t.Errorf("limites da API = %d %d", cfg.MaxSampleSize, cfg.MaxWorkers)
	}
	if cfg.Instructions != DefaultInstructions {
		t.Errorf("instruções não são as padrão")
	}
}

func TestLoadExigeSenhaDoNeo4j(t *testing.T) {
	_, err := Load(env(nil))
	if err == nil || !strings.Contains(err.Error(), "NEO4J_PASSWORD") {
		t.Fatalf("esperava erro citando NEO4J_PASSWORD, veio %v", err)
	}
}

func TestLoadRejeitaValoresInvalidos(t *testing.T) {
	casos := map[string]map[string]string{
		"inflight não numérico":  {"JEV_MAX_INFLIGHT": "muitos"},
		"inflight fora da faixa": {"JEV_MAX_INFLIGHT": "0"},
		"timeout inválido":       {"JEV_TIMEOUT": "rapido"},
		"timeout negativo":       {"NEO4J_QUERY_TIMEOUT": "-1s"},
		"workers demais":         {"MAX_WORKERS": "99999"},
	}
	for nome, vars := range casos {
		t.Run(nome, func(t *testing.T) {
			vars["NEO4J_PASSWORD"] = "segredo123"
			if _, err := Load(env(vars)); err == nil {
				t.Fatal("esperava erro")
			}
		})
	}
}

func TestLoadJuntaTodosOsErros(t *testing.T) {
	_, err := Load(env(map[string]string{"JEV_MAX_INFLIGHT": "x", "JEV_TIMEOUT": "y"}))
	if err == nil {
		t.Fatal("esperava erro")
	}
	for _, trecho := range []string{"NEO4J_PASSWORD", "JEV_MAX_INFLIGHT", "JEV_TIMEOUT"} {
		if !strings.Contains(err.Error(), trecho) {
			t.Errorf("erro não cita %s: %v", trecho, err)
		}
	}
}

func TestLoadLeInstrucoesDeArquivo(t *testing.T) {
	arquivo := filepath.Join(t.TempDir(), "instrucoes.txt")
	if err := os.WriteFile(arquivo, []byte("  Essa transação é suspeita?\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(env(map[string]string{
		"NEO4J_PASSWORD":        "segredo123",
		"JEV_INSTRUCTIONS_FILE": arquivo,
	}))
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if cfg.Instructions != "Essa transação é suspeita?" {
		t.Errorf("Instructions = %q", cfg.Instructions)
	}
}

func TestLoadRejeitaArquivoDeInstrucoesVazioOuAusente(t *testing.T) {
	vazio := filepath.Join(t.TempDir(), "vazio.txt")
	if err := os.WriteFile(vazio, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, caminho := range []string{vazio, filepath.Join(t.TempDir(), "nao-existe.txt")} {
		_, err := Load(env(map[string]string{
			"NEO4J_PASSWORD":        "segredo123",
			"JEV_INSTRUCTIONS_FILE": caminho,
		}))
		if err == nil {
			t.Errorf("esperava erro para %s", caminho)
		}
	}
}
