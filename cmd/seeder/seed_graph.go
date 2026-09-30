// Command seeder carrega o CSV do PaySim no Neo4j e grava as estatísticas do
// dataset. Pode ser executado de novo depois de uma queda, ele continua de onde
// parou.
package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fraud-detector-jev/internal/config"
	"fraud-detector-jev/internal/infrastructure/neo4j"
	"fraud-detector-jev/internal/infrastructure/runstore"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("a carga falhou", "erro", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	batch := flag.Int("batch", 10000, "quantas linhas por lote de escrita no Neo4j")
	flag.Parse()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	total, err := countDataRows(cfg.DataPath)
	if err != nil {
		return err
	}
	logger.Info("começando a carga", "arquivo", cfg.DataPath, "linhas_aproximadas", total, "lote", *batch)

	client, err := neo4j.New(ctx, neo4j.Options{
		URI: cfg.Neo4jURI, User: cfg.Neo4jUser, Password: cfg.Neo4jPassword, MaxPool: 4,
	})
	if err != nil {
		return err
	}
	defer client.Close(context.Background())

	store, err := runstore.New(cfg.RunsDir)
	if err != nil {
		return err
	}

	start := time.Now()
	report, err := client.Load(ctx, neo4j.LoadOptions{
		CSVPath: cfg.DataPath, BatchSize: *batch, Progress: progressLogger(logger, total),
	})
	if err != nil {
		return err
	}
	if err := store.WriteDatasetStats(report.Stats); err != nil {
		return err
	}
	logger.Info("carga concluída",
		"contas", report.Nodes, "arestas", report.Rels, "retomada", report.Resumed,
		"fraudes", report.Stats.FraudRows, "maior_step", report.Stats.MaxStep,
		"duração", time.Since(start).Round(time.Second))
	return nil
}

// countDataRows conta as linhas de dados só olhando as quebras de linha (é
// rápido e serve para estimar o tempo restante, então é aproximado).
func countDataRows(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var lines int64
	last := byte('\n')
	buf := make([]byte, 1<<20)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			lines += int64(bytes.Count(buf[:n], []byte{'\n'}))
			last = buf[n-1]
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
	}
	if last != '\n' {
		lines++ // a última linha não tinha quebra
	}
	if lines > 0 {
		lines-- // o cabeçalho
	}
	return lines, nil
}

// progressLogger escreve o andamento no máximo a cada 5 segundos. Numa
// retomada a estimativa de tempo fica otimista, porque o contador de linhas já
// começa adiantado.
func progressLogger(logger *slog.Logger, total int64) func(phase string, done int64) {
	var phaseStart, lastLog time.Time
	var lastPhase string
	return func(phase string, done int64) {
		now := time.Now()
		if phase != lastPhase {
			lastPhase, phaseStart, lastLog = phase, now, time.Time{}
		}
		if done < total && now.Sub(lastLog) < 5*time.Second {
			return
		}
		lastLog = now

		rate := 0.0
		if elapsed := now.Sub(phaseStart).Seconds(); elapsed > 0 {
			rate = float64(done) / elapsed
		}
		var eta time.Duration
		if rate > 0 && total > done {
			eta = time.Duration(float64(total-done)/rate) * time.Second
		}
		logger.Info("carregando", "fase", phase, "linhas", done, "total", total,
			"linhas_por_segundo", int(rate), "falta", eta.Round(time.Second))
	}
}
