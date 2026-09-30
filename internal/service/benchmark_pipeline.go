package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"fraud-detector-jev/internal/domain"
)

// warmupState é o state das chamadas de aquecimento. Elas só servem para o
// modelo carregar na memória antes de a medição começar.
const warmupState = `{"warmup":true}`

// warmupAccount é a conta falsa das consultas de aquecimento do Neo4j. Ela
// nunca existe no grafo, então a consulta devolve tudo zerado, mas o Neo4j
// abre a conexão e compila o plano da query do mesmo jeito.
const warmupAccount = "__warmup__"

// PipelineConfig define como uma execução roda. Workers é o pool do Neo4j e
// ClassifyWorkers é quantas chamadas simultâneas ao modelo.
type PipelineConfig struct {
	Graph           bool
	Workers         int
	ClassifyWorkers int
	WindowHours     int
	CycleMaxHops    int
	Threshold       float64
	Warmup          int
}

// PipelineOutput são as métricas consolidadas de uma execução. Completed conta
// só as transações sem erro.
type PipelineOutput struct {
	Counts      domain.Counts
	Completed   int
	Latency     domain.LatencyBlock
	Duration    time.Duration
	TPS         float64
	TokensTotal int64
}

// Pipeline junta a fonte de features e o modelo. Features pode ser nil quando a
// execução é sem grafo.
type Pipeline struct {
	Features   GraphFeatureSource
	Classifier Classifier
}

type job struct {
	res   domain.Result
	state string
}

// Run processa as transações e entrega cada Result ao sink. onProgress, se não
// for nil, recebe quantas transações já foram tratadas. Se o contexto for
// cancelado, devolve o que já foi gravado e context.Canceled.
func (p Pipeline) Run(ctx context.Context, cfg PipelineConfig, txs []domain.Transaction, sink func(domain.Result) error, onProgress func(done int)) (PipelineOutput, error) {
	if cfg.Workers < 1 || cfg.ClassifyWorkers < 1 {
		return PipelineOutput{}, errors.New("Workers e ClassifyWorkers precisam ser pelo menos 1")
	}
	if cfg.Graph && p.Features == nil {
		return PipelineOutput{}, errors.New("execução com grafo precisa de uma fonte de features")
	}
	if err := p.warmup(ctx, cfg.Warmup); err != nil {
		return PipelineOutput{}, err
	}
	if cfg.Graph && cfg.Warmup > 0 {
		if err := p.warmupNeo4j(ctx, cfg); err != nil {
			return PipelineOutput{}, err
		}
	}

	parent := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	in := make(chan domain.Transaction, cfg.Workers*2)
	enriched := make(chan job, cfg.ClassifyWorkers*2)
	results := make(chan domain.Result, cfg.ClassifyWorkers*2)
	start := time.Now()

	go func() {
		defer close(in)
		for _, tx := range txs {
			if !send(ctx, in, tx) {
				return
			}
		}
	}()

	var enrichWG, classifyWG sync.WaitGroup
	for range cfg.Workers {
		enrichWG.Add(1)
		go func() {
			defer enrichWG.Done()
			p.enrichLoop(ctx, cfg, in, enriched, results)
		}()
	}
	for range cfg.ClassifyWorkers {
		classifyWG.Add(1)
		go func() {
			defer classifyWG.Done()
			p.classifyLoop(ctx, enriched, results)
		}()
	}
	go func() {
		enrichWG.Wait()
		close(enriched)
	}()
	go func() {
		enrichWG.Wait()
		classifyWG.Wait()
		close(results)
	}()

	out, sinkErr := collect(cfg, results, sink, onProgress, cancel)
	out.Duration = time.Since(start)
	if out.Duration > 0 {
		out.TPS = float64(out.Completed) / out.Duration.Seconds()
	}
	if sinkErr != nil {
		return out, sinkErr
	}
	return out, parent.Err()
}

// warmup faz algumas chamadas ao modelo antes da medição. Se todas falharem, o
// modelo está fora do ar e a execução nem começa.
func (p Pipeline) warmup(ctx context.Context, n int) error {
	if n <= 0 {
		return nil
	}
	failures := 0
	var last error
	for range n {
		if _, err := p.Classifier.Classify(ctx, warmupState); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			failures++
			last = err
		}
	}
	if failures == n {
		return fmt.Errorf("o modelo não respondeu no aquecimento: %w", last)
	}
	return nil
}

// warmupNeo4j faz uma consulta falsa por worker, todas ao mesmo tempo. Assim o
// pool de conexões já está aberto e o plano da query já está no cache quando a
// medição começa. Se todas falharem, o Neo4j está fora do ar.
func (p Pipeline) warmupNeo4j(ctx context.Context, cfg PipelineConfig) error {
	q := domain.FeatureQuery{
		Orig: warmupAccount, Dest: warmupAccount, BeforeSeq: 0, AtStep: 0,
		WindowHours: cfg.WindowHours, CycleMaxHops: cfg.CycleMaxHops,
	}
	errs := make([]error, cfg.Workers)
	var wg sync.WaitGroup
	for i := range cfg.Workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = p.Features.Features(ctx, q)
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	var failed int
	for _, err := range errs {
		if err != nil {
			failed++
		}
	}
	if failed == len(errs) {
		return fmt.Errorf("o Neo4j não respondeu no aquecimento: %w", errors.Join(errs...))
	}
	return nil
}

func (p Pipeline) enrichLoop(ctx context.Context, cfg PipelineConfig, in <-chan domain.Transaction, enriched chan<- job, results chan<- domain.Result) {
	for tx := range in {
		if ctx.Err() != nil {
			return
		}
		res := domain.NewResult(tx, nil)
		var gf *domain.GraphFeatures

		if cfg.Graph {
			t0 := time.Now()
			f, err := p.Features.Features(ctx, FeatureQueryFor(tx, cfg.WindowHours, cfg.CycleMaxHops))
			ms := millis(time.Since(t0))
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				res.Error = "neo4j: " + err.Error()
				if !send(ctx, results, res) {
					return
				}
				continue
			}
			gf = &f
			res = domain.NewResult(tx, gf)
			res.Neo4jMs = &ms
		}

		state, err := BuildState(tx, gf).JSON()
		if err != nil {
			res.Error = "montando o state: " + err.Error()
			if !send(ctx, results, res) {
				return
			}
			continue
		}
		if !send(ctx, enriched, job{res: res, state: state}) {
			return
		}
	}
}

func (p Pipeline) classifyLoop(ctx context.Context, enriched <-chan job, results chan<- domain.Result) {
	for j := range enriched {
		if ctx.Err() != nil {
			return
		}
		dec, err := p.Classifier.Classify(ctx, j.state)
		res := j.res
		res.Attempts = dec.Attempts
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			res.Error = "modelo: " + err.Error()
		} else {
			noul, ms := dec.Noul, millis(dec.Latency)
			res.Noul, res.JevMs, res.InputTokens = &noul, &ms, dec.InputTokens
		}
		if !send(ctx, results, res) {
			return
		}
	}
}

// collect é o único que grava resultados e soma métricas. Se o sink falhar,
// cancela o resto e continua drenando o channel até ele fechar.
func collect(cfg PipelineConfig, results <-chan domain.Result, sink func(domain.Result) error, onProgress func(int), cancel context.CancelFunc) (PipelineOutput, error) {
	var out PipelineOutput
	var jevMs, neoMs []float64
	var sinkErr error
	done := 0

	for res := range results {
		if sinkErr != nil {
			continue
		}
		if err := sink(res); err != nil {
			sinkErr = fmt.Errorf("gravando o resultado: %w", err)
			cancel()
			continue
		}
		done++
		if onProgress != nil {
			onProgress(done)
		}
		if res.Error != "" {
			out.Counts.Errors++
			continue
		}
		out.Completed++
		out.Counts.Add(res.IsFraud, domain.Decide(*res.Noul, cfg.Threshold))
		jevMs = append(jevMs, *res.JevMs)
		if res.Neo4jMs != nil {
			neoMs = append(neoMs, *res.Neo4jMs)
		}
		out.TokensTotal += int64(res.InputTokens)
	}
	out.Latency = domain.LatencyBlock{Neo4j: domain.ComputeLatency(neoMs), Jev: domain.ComputeLatency(jevMs)}
	return out, sinkErr
}

func send[T any](ctx context.Context, ch chan<- T, v T) bool {
	select {
	case ch <- v:
		return true
	case <-ctx.Done():
		return false
	}
}

func millis(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
