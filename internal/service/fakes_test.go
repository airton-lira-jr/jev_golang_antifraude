package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fraud-detector-jev/internal/domain"
)

// txsDeTeste cria n transações TRANSFER. A cada 3 uma é fraude, com amount 1000
// (as outras têm amount 10). O classificador falso usa o amount para decidir.
func txsDeTeste(n int) []domain.Transaction {
	txs := make([]domain.Transaction, 0, n)
	for i := range n {
		fraude := i%3 == 0
		amount := 10.0
		if fraude {
			amount = 1000
		}
		txs = append(txs, domain.Transaction{
			Seq: int64(i), Step: i, Type: "TRANSFER", Amount: amount,
			Orig: "O" + strconv.Itoa(i), Dest: "D" + strconv.Itoa(i), IsFraud: fraude,
		})
	}
	return txs
}

// fakeFeatures é a fonte de features falsa.
type fakeFeatures struct {
	calls atomic.Int64
	fn    func(domain.FeatureQuery) (domain.GraphFeatures, error)
}

func (f *fakeFeatures) Features(_ context.Context, q domain.FeatureQuery) (domain.GraphFeatures, error) {
	f.calls.Add(1)
	if f.fn != nil {
		return f.fn(q)
	}
	return domain.GraphFeatures{DestInDegree: int(q.BeforeSeq)}, nil
}

// fakeClassifier é o modelo falso. Por padrão devolve 0.9 quando o amount do
// state é 1000 ou mais e 0.1 no resto.
type fakeClassifier struct {
	calls       atomic.Int64
	inflight    atomic.Int64
	maxInflight atomic.Int64
	delay       time.Duration
	fn          func(ctx context.Context, state string) (domain.Decision, error)

	mu     sync.Mutex
	states []string
}

func (f *fakeClassifier) Classify(ctx context.Context, state string) (domain.Decision, error) {
	f.calls.Add(1)
	cur := f.inflight.Add(1)
	defer f.inflight.Add(-1)
	for {
		seen := f.maxInflight.Load()
		if cur <= seen || f.maxInflight.CompareAndSwap(seen, cur) {
			break
		}
	}
	f.mu.Lock()
	f.states = append(f.states, state)
	f.mu.Unlock()

	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.fn != nil {
		return f.fn(ctx, state)
	}
	return domain.Decision{Noul: noulPorValor(state), InputTokens: 10, Attempts: 1, Latency: 2 * time.Millisecond}, nil
}

func (f *fakeClassifier) statesVistos() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.states...)
}

func noulPorValor(state string) float64 {
	var st struct {
		Amount float64 `json:"amount"`
	}
	_ = json.Unmarshal([]byte(state), &st)
	if st.Amount >= 1000 {
		return 0.9
	}
	return 0.1
}

// seqDoState devolve o seq do transaction_id do state, ou -1 (o state de
// aquecimento não tem transaction_id).
func seqDoState(state string) int64 {
	var st struct {
		ID string `json:"transaction_id"`
	}
	if json.Unmarshal([]byte(state), &st) != nil || !strings.HasPrefix(st.ID, "TX_") {
		return -1
	}
	n, err := strconv.ParseInt(strings.TrimPrefix(st.ID, "TX_"), 10, 64)
	if err != nil {
		return -1
	}
	return n
}

// memStore é o armazenamento em memória usado nos testes do RunManager.
type memStore struct {
	mu        sync.Mutex
	nextID    int
	summaries []domain.RunSummary
	results   []domain.Result
	closed    bool
	saveErr   error
}

func (m *memStore) NewRunID(time.Time) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	return fmt.Sprintf("run_teste_%d", m.nextID)
}

func (m *memStore) SaveSummary(s domain.RunSummary) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveErr != nil {
		return m.saveErr
	}
	m.summaries = append(m.summaries, s)
	return nil
}

func (m *memStore) OpenResultsWriter(string) (domain.ResultsSink, error) { return &memSink{m}, nil }

func (m *memStore) ultimoResumo() domain.RunSummary {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.summaries[len(m.summaries)-1]
}

func (m *memStore) qtdResultados() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.results)
}

type memSink struct{ m *memStore }

func (s *memSink) Write(r domain.Result) error {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	s.m.results = append(s.m.results, r)
	return nil
}

func (s *memSink) Close() error {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	s.m.closed = true
	return nil
}
