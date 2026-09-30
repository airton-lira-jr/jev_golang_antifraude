package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultInstructions é a pergunta padrão feita ao modelo. Ela é a mesma nas
// duas variantes do benchmark (com e sem grafo), para a única diferença entre
// elas ser o conteúdo do state.
const DefaultInstructions = "Determine se esta transação tem perfil típico de fraude bancária, esvaziamento ilícito de conta ou lavagem de dinheiro, com base em todos os dados fornecidos."

// Config reúne tudo que vem de variável de ambiente.
type Config struct {
	HTTPAddr          string
	Neo4jURI          string
	Neo4jUser         string
	Neo4jPassword     string
	Neo4jQueryTimeout time.Duration
	JevBaseURL        string
	JevModel          string
	JevMaxInflight    int
	JevTimeout        time.Duration
	Instructions      string
	DataPath          string
	RunsDir           string
	MaxSampleSize     int
	MaxWorkers        int
}

// Load lê as variáveis usando getenv (assim o teste não mexe no ambiente real)
// e devolve todos os problemas encontrados de uma vez.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		HTTPAddr:      orDefault(getenv("HTTP_ADDR"), ":8080"),
		Neo4jURI:      orDefault(getenv("NEO4J_URI"), "bolt://localhost:7687"),
		Neo4jUser:     orDefault(getenv("NEO4J_USER"), "neo4j"),
		Neo4jPassword: getenv("NEO4J_PASSWORD"),
		JevBaseURL:    orDefault(getenv("JEV_BASE_URL"), "http://host.docker.internal:11434"),
		JevModel:      orDefault(getenv("JEV_MODEL"), "tev1:4b"),
		DataPath:      orDefault(getenv("DATA_PATH"), "data/transactions.csv"),
		RunsDir:       orDefault(getenv("RUNS_DIR"), "runs"),
		Instructions:  DefaultInstructions,
	}

	var errs []error
	if cfg.Neo4jPassword == "" {
		errs = append(errs, errors.New("NEO4J_PASSWORD é obrigatória"))
	}

	var err error
	if cfg.JevMaxInflight, err = intVar(getenv, "JEV_MAX_INFLIGHT", 4, 1, 64); err != nil {
		errs = append(errs, err)
	}
	if cfg.MaxSampleSize, err = intVar(getenv, "MAX_SAMPLE_SIZE", 500000, 1, 5000000); err != nil {
		errs = append(errs, err)
	}
	if cfg.MaxWorkers, err = intVar(getenv, "MAX_WORKERS", 256, 1, 4096); err != nil {
		errs = append(errs, err)
	}
	if cfg.JevTimeout, err = durationVar(getenv, "JEV_TIMEOUT", 30*time.Second); err != nil {
		errs = append(errs, err)
	}
	if cfg.Neo4jQueryTimeout, err = durationVar(getenv, "NEO4J_QUERY_TIMEOUT", 2*time.Second); err != nil {
		errs = append(errs, err)
	}

	if path := getenv("JEV_INSTRUCTIONS_FILE"); path != "" {
		text, err := readInstructions(path)
		if err != nil {
			errs = append(errs, err)
		} else {
			cfg.Instructions = text
		}
	}

	return cfg, errors.Join(errs...)
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func intVar(getenv func(string) string, name string, def, lo, hi int) (int, error) {
	raw := getenv(name)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def, fmt.Errorf("%s precisa ser um número inteiro, veio %q", name, raw)
	}
	if n < lo || n > hi {
		return def, fmt.Errorf("%s precisa estar entre %d e %d, veio %d", name, lo, hi, n)
	}
	return n, nil
}

func durationVar(getenv func(string) string, name string, def time.Duration) (time.Duration, error) {
	raw := getenv(name)
	if raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return def, fmt.Errorf("%s precisa ser uma duração como 30s, veio %q", name, raw)
	}
	if d <= 0 {
		return def, fmt.Errorf("%s precisa ser maior que zero, veio %s", name, raw)
	}
	return d, nil
}

func readInstructions(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("lendo JEV_INSTRUCTIONS_FILE: %w", err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", fmt.Errorf("JEV_INSTRUCTIONS_FILE (%s) está vazio", path)
	}
	return text, nil
}
