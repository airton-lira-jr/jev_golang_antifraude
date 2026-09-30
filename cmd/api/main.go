// Command api sobe a API REST do detector de fraude.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fraud-detector-jev/internal/config"
	"fraud-detector-jev/internal/domain"
	"fraud-detector-jev/internal/infrastructure/csv"
	"fraud-detector-jev/internal/infrastructure/jev"
	"fraud-detector-jev/internal/infrastructure/neo4j"
	"fraud-detector-jev/internal/infrastructure/runstore"
	"fraud-detector-jev/internal/infrastructure/web"
	"fraud-detector-jev/internal/service"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("o servidor parou com erro", "erro", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	neoClient, err := neo4j.New(connectCtx, neo4j.Options{
		URI: cfg.Neo4jURI, User: cfg.Neo4jUser, Password: cfg.Neo4jPassword,
		MaxPool: cfg.MaxWorkers, QueryTimeout: cfg.Neo4jQueryTimeout,
	})
	cancel()
	if err != nil {
		return err
	}
	defer neoClient.Close(context.Background())

	store, err := runstore.New(cfg.RunsDir)
	if err != nil {
		return err
	}
	if n, err := store.MarkStaleRunsFailed(time.Now()); err != nil {
		return err
	} else if n > 0 {
		logger.Warn("execuções que ficaram penduradas foram marcadas como failed", "quantas", n)
	}

	newClassifier := func(model string, maxInflight int) (service.Classifier, error) {
		client, err := jev.New(jev.Config{
			BaseURL: cfg.JevBaseURL, Model: model, Instructions: cfg.Instructions,
			MaxInflight: maxInflight, Timeout: cfg.JevTimeout,
		})
		if err != nil {
			return nil, err
		}
		return client, nil
	}
	factory := func(p domain.RunParams) (service.Pipeline, error) {
		classifier, err := newClassifier(p.Model, p.MaxInflight)
		if err != nil {
			return service.Pipeline{}, err
		}
		return service.Pipeline{Features: neoClient, Classifier: classifier}, nil
	}
	source := func(ctx context.Context, fn func(domain.Transaction) error) error {
		return csv.Stream(ctx, cfg.DataPath, fn)
	}
	runs := service.NewRunManager(store, source, factory, logger)

	evaluator := &service.Evaluator{
		Features:  neoClient,
		Subgraphs: neoClient,
		ClassifierFor: func(model string) (service.Classifier, error) {
			return newClassifier(model, cfg.JevMaxInflight)
		},
		DefaultModel: cfg.JevModel,
		DefaultStep: func() (int, bool) {
			stats, err := store.ReadDatasetStats()
			if err != nil {
				return 0, false
			}
			return stats.MaxStep, true
		},
		WindowHours:      service.DefaultWindowHours,
		CycleMaxHops:     service.DefaultCycleMaxHops,
		DefaultThreshold: service.DefaultThreshold,
		SubgraphLimit:    50,
	}

	server := &web.Server{
		Runs:      runs,
		Store:     store,
		Evaluator: evaluator,
		Limits: service.Limits{
			MaxSampleSize: cfg.MaxSampleSize, MaxWorkers: cfg.MaxWorkers,
			DefaultModel: cfg.JevModel, BaseURL: cfg.JevBaseURL,
		},
		PingNeo4j: neoClient.Ping,
		ListModels: func(ctx context.Context) ([]string, error) {
			return jev.ListModels(ctx, cfg.JevBaseURL)
		},
		Log: logger,
	}
	httpServer := &http.Server{Addr: cfg.HTTPAddr, Handler: server.Handler(), ReadHeaderTimeout: 10 * time.Second}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("API no ar", "endereço", cfg.HTTPAddr, "modelo", cfg.JevModel, "ollama", cfg.JevBaseURL)
		serveErr <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		logger.Info("recebi o sinal de parada, encerrando")
	}

	runs.Shutdown()
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	return httpServer.Shutdown(shutdownCtx)
}
