package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"fraud-detector-jev/internal/domain"
)

// warmupCalls é quantas chamadas de aquecimento cada execução faz.
const warmupCalls = 3

// ErrBusy aparece quando já existe uma execução em andamento.
var ErrBusy = errors.New("já existe uma execução em andamento")

// RunStore é o que o gerenciador precisa do armazenamento. O runstore.Store
// implementa.
type RunStore interface {
	NewRunID(now time.Time) string
	SaveSummary(sum domain.RunSummary) error
	OpenResultsWriter(id string) (domain.ResultsSink, error)
}

// RunFactory monta o pipeline de uma execução. É aqui que o cliente do modelo
// nasce com o modelo escolhido nos parâmetros.
type RunFactory func(params domain.RunParams) (Pipeline, error)

// RunManager roda um benchmark por vez em segundo plano.
type RunManager struct {
	store   RunStore
	source  TxSource
	factory RunFactory
	log     *slog.Logger
	now     func() time.Time

	mu     sync.Mutex
	active *activeRun
	wg     sync.WaitGroup
}

type activeRun struct {
	id     string
	cancel context.CancelFunc
	done   atomic.Int64
	total  atomic.Int64
}

// NewRunManager cria o gerenciador. logger nil usa o slog padrão.
func NewRunManager(store RunStore, source TxSource, factory RunFactory, logger *slog.Logger) *RunManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &RunManager{store: store, source: source, factory: factory, log: logger, now: time.Now}
}

// Start começa uma execução e devolve o resumo inicial (status running).
func (m *RunManager) Start(params domain.RunParams) (domain.RunSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != nil {
		return domain.RunSummary{}, ErrBusy
	}

	pipe, err := m.factory(params)
	if err != nil {
		return domain.RunSummary{}, fmt.Errorf("preparando a execução: %w", err)
	}
	sum := domain.RunSummary{
		RunID:     m.store.NewRunID(m.now()),
		Status:    domain.StatusRunning,
		StartedAt: m.now(),
		Params:    params,
	}
	if err := m.store.SaveSummary(sum); err != nil {
		closeIfCloser(pipe.Classifier)
		return domain.RunSummary{}, fmt.Errorf("gravando o resumo inicial: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	run := &activeRun{id: sum.RunID, cancel: cancel}
	m.active = run
	m.wg.Add(1)
	go m.execute(ctx, run, pipe, sum)
	return sum, nil
}

func (m *RunManager) execute(ctx context.Context, run *activeRun, pipe Pipeline, sum domain.RunSummary) {
	defer m.wg.Done()
	defer func() {
		m.mu.Lock()
		m.active = nil
		m.mu.Unlock()
	}()
	defer closeIfCloser(pipe.Classifier)
	defer run.cancel()

	p := sum.Params
	m.log.Info("execução começou", "run_id", sum.RunID, "n", p.N, "modelo", p.Model, "grafo", p.Graph)

	sample, err := Sample(ctx, m.source, SampleParams{N: p.N, FraudPct: p.FraudPct, Seed: p.Seed, Types: p.Types})
	if err != nil {
		m.finish(&sum, err)
		return
	}
	run.total.Store(int64(len(sample.Transactions)))
	sum.Sample = &domain.SampleInfo{
		Requested: p.N, N: len(sample.Transactions), NFraud: sample.NFraud, NLegit: sample.NLegit,
		FraudShortfall: sample.FraudShortfall, EligibleRows: sample.EligibleRows,
		EligibleFraud: sample.EligibleFraud, Prevalence: sample.Prevalence,
	}

	sink, err := m.store.OpenResultsWriter(sum.RunID)
	if err != nil {
		m.finish(&sum, fmt.Errorf("abrindo os resultados: %w", err))
		return
	}
	cfg := PipelineConfig{
		Graph: p.Graph, Workers: p.Workers, ClassifyWorkers: p.MaxInflight,
		WindowHours: p.WindowHours, CycleMaxHops: p.CycleMaxHops, Threshold: p.Threshold, Warmup: warmupCalls,
	}
	out, runErr := pipe.Run(ctx, cfg, sample.Transactions, sink.Write, func(done int) { run.done.Store(int64(done)) })
	if closeErr := sink.Close(); closeErr != nil && runErr == nil {
		runErr = fmt.Errorf("fechando os resultados: %w", closeErr)
	}

	counts := out.Counts
	metrics := domain.ComputeMetrics(counts.Confusion, sample.Prevalence)
	latency := out.Latency
	sum.Counts, sum.Metrics, sum.Latency = &counts, &metrics, &latency
	sum.TPS, sum.TokensTotal = out.TPS, out.TokensTotal
	m.finish(&sum, runErr)
}

// finish define o status final e grava o resumo.
func (m *RunManager) finish(sum *domain.RunSummary, err error) {
	now := m.now()
	sum.FinishedAt = &now
	sum.DurationS = now.Sub(sum.StartedAt).Seconds()
	switch {
	case err == nil:
		sum.Status = domain.StatusDone
	case errors.Is(err, context.Canceled):
		sum.Status = domain.StatusCanceled
	default:
		sum.Status = domain.StatusFailed
		sum.Error = err.Error()
	}
	m.log.Info("execução terminou", "run_id", sum.RunID, "status", sum.Status, "erro", sum.Error)
	if saveErr := m.store.SaveSummary(*sum); saveErr != nil {
		m.log.Error("não consegui gravar o resumo final", "run_id", sum.RunID, "erro", saveErr)
	}
}

// Cancel cancela a execução ativa se o id bater. Devolve false caso contrário.
func (m *RunManager) Cancel(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil || m.active.id != id {
		return false
	}
	m.active.cancel()
	return true
}

// Progress devolve o andamento da execução ativa. Total é 0 enquanto a amostra
// ainda está sendo montada.
func (m *RunManager) Progress(id string) (domain.Progress, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil || m.active.id != id {
		return domain.Progress{}, false
	}
	return domain.Progress{Done: int(m.active.done.Load()), Total: int(m.active.total.Load())}, true
}

// ActiveID devolve o id da execução ativa, ou "" se não houver.
func (m *RunManager) ActiveID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil {
		return ""
	}
	return m.active.id
}

// Wait espera a execução ativa terminar.
func (m *RunManager) Wait() { m.wg.Wait() }

// Shutdown cancela a execução ativa e espera ela terminar de gravar.
func (m *RunManager) Shutdown() {
	m.mu.Lock()
	if m.active != nil {
		m.active.cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
}

// closeIfCloser fecha o classificador se ele souber se fechar (o jev.Client
// solta as conexões ociosas).
func closeIfCloser(c Classifier) {
	if closer, ok := c.(interface{ Close() }); ok {
		closer.Close()
	}
}
