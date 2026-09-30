package service

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fraud-detector-jev/internal/domain"
)

func paramsDeTeste() domain.RunParams {
	return domain.RunParams{
		N: 30, FraudPct: 50, Seed: 1, Workers: 4, Model: "tev1:4b", Graph: true,
		Types: []string{"TRANSFER"}, WindowHours: 24, CycleMaxHops: 3, MaxInflight: 2, Threshold: 0.5,
	}
}

func novoGerenciador(t *testing.T, txs []domain.Transaction, cl *fakeClassifier) (*RunManager, *memStore) {
	t.Helper()
	store := &memStore{}
	factory := func(domain.RunParams) (Pipeline, error) {
		return Pipeline{Features: &fakeFeatures{}, Classifier: cl}, nil
	}
	m := NewRunManager(store, fonte(txs), factory, nil)
	t.Cleanup(m.Shutdown)
	return m, store
}

// esperar espera a condição ficar verdadeira por até 3 segundos.
func esperar(t *testing.T, descricao string, cond func() bool) {
	t.Helper()
	limite := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(limite) {
			t.Fatalf("passou o tempo esperando: %s", descricao)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRunManagerExecutaEGravaOResumoFinal(t *testing.T) {
	m, store := novoGerenciador(t, txsDeTeste(300), &fakeClassifier{})

	inicial, err := m.Start(paramsDeTeste())
	if err != nil {
		t.Fatal(err)
	}
	if inicial.Status != domain.StatusRunning || inicial.RunID == "" {
		t.Errorf("resumo inicial = %+v", inicial)
	}
	m.Wait()

	sum := store.ultimoResumo()
	if sum.RunID != inicial.RunID || sum.Status != domain.StatusDone || sum.Error != "" || sum.FinishedAt == nil {
		t.Fatalf("resumo final = %+v", sum)
	}
	// 300 transações, uma fraude a cada 3, então 100 fraudes em 300 (prevalência 1/3).
	if sum.Sample == nil || sum.Sample.N != 30 || sum.Sample.NFraud != 15 || sum.Sample.NLegit != 15 ||
		sum.Sample.EligibleRows != 300 || sum.Sample.EligibleFraud != 100 || sum.Sample.Prevalence != 1.0/3 {
		t.Errorf("amostra = %+v", sum.Sample)
	}
	if sum.Counts == nil || sum.Counts.TP != 15 || sum.Counts.TN != 15 || sum.Counts.Errors != 0 {
		t.Errorf("contagens = %+v", sum.Counts)
	}
	want := domain.ComputeMetrics(sum.Counts.Confusion, 1.0/3)
	if sum.Metrics == nil || *sum.Metrics != want {
		t.Errorf("métricas = %+v, esperadas %+v", sum.Metrics, want)
	}
	if sum.Latency == nil || sum.TPS <= 0 || sum.TokensTotal != 300 {
		t.Errorf("latência=%+v tps=%v tokens=%d", sum.Latency, sum.TPS, sum.TokensTotal)
	}
	if store.qtdResultados() != 30 || !store.closed {
		t.Errorf("resultados gravados=%d fechado=%v", store.qtdResultados(), store.closed)
	}
	if m.ActiveID() != "" {
		t.Error("não deveria haver execução ativa depois de terminar")
	}
	if len(store.summaries) < 2 || store.summaries[0].Status != domain.StatusRunning {
		t.Errorf("o primeiro resumo gravado deveria estar running: %+v", store.summaries[0])
	}
}

func TestRunManagerSoAceitaUmaExecucaoPorVez(t *testing.T) {
	liberar := make(chan struct{})
	cl := &fakeClassifier{fn: func(ctx context.Context, state string) (domain.Decision, error) {
		if seqDoState(state) >= 0 {
			select {
			case <-liberar:
			case <-ctx.Done():
				return domain.Decision{}, ctx.Err()
			}
		}
		return domain.Decision{Noul: 0.1, Attempts: 1, Latency: time.Millisecond}, nil
	}}
	m, _ := novoGerenciador(t, txsDeTeste(300), cl)

	if _, err := m.Start(paramsDeTeste()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(paramsDeTeste()); !errors.Is(err, ErrBusy) {
		t.Fatalf("esperava ErrBusy, veio %v", err)
	}
	close(liberar)
	m.Wait()
	if _, err := m.Start(paramsDeTeste()); err != nil {
		t.Fatalf("depois de terminar deveria aceitar outra: %v", err)
	}
}

func TestRunManagerCancelaEPreservaOQueJaFoiGravado(t *testing.T) {
	var chamadas atomic.Int64
	cl := &fakeClassifier{fn: func(ctx context.Context, state string) (domain.Decision, error) {
		if seqDoState(state) >= 0 && chamadas.Add(1) > 5 {
			<-ctx.Done()
			return domain.Decision{}, ctx.Err()
		}
		return domain.Decision{Noul: 0.1, Attempts: 1, Latency: time.Millisecond}, nil
	}}
	m, store := novoGerenciador(t, txsDeTeste(300), cl)

	sum, err := m.Start(paramsDeTeste())
	if err != nil {
		t.Fatal(err)
	}
	// Com 2 workers, ver a 7a chamada começar garante que as duas estão presas
	// e que os 5 resultados prontos já foram entregues ao coletor.
	esperar(t, "as duas workers presas no modelo", func() bool { return chamadas.Load() > 6 })

	if m.Cancel("run_de_outra") {
		t.Error("cancelar um id que não é o ativo deveria devolver false")
	}
	if !m.Cancel(sum.RunID) {
		t.Fatal("Cancel do id ativo deveria devolver true")
	}
	m.Wait()

	final := store.ultimoResumo()
	if final.Status != domain.StatusCanceled || final.FinishedAt == nil || final.Error != "" {
		t.Errorf("resumo = %+v", final)
	}
	if n := store.qtdResultados(); n != 5 {
		t.Errorf("deveriam sobrar os 5 resultados prontos, sobraram %d", n)
	}
	if final.Counts == nil || final.Counts.Errors != 0 {
		t.Errorf("cancelamento não pode virar erro: %+v", final.Counts)
	}
	if m.Cancel(sum.RunID) {
		t.Error("depois de terminar, Cancel deveria devolver false")
	}
}

func TestRunManagerErroDaFabricaNaoGravaNada(t *testing.T) {
	store := &memStore{}
	m := NewRunManager(store, fonte(txsDeTeste(30)), func(domain.RunParams) (Pipeline, error) {
		return Pipeline{}, errors.New("modelo desconhecido")
	}, nil)
	t.Cleanup(m.Shutdown)

	if _, err := m.Start(paramsDeTeste()); err == nil || !strings.Contains(err.Error(), "modelo desconhecido") {
		t.Fatalf("esperava o erro da fábrica, veio %v", err)
	}
	if len(store.summaries) != 0 || m.ActiveID() != "" {
		t.Errorf("não deveria ter gravado nada nem ficado ocupado: %+v", store.summaries)
	}
}

func TestRunManagerFonteComErroViraFailed(t *testing.T) {
	store := &memStore{}
	falha := errors.New("o CSV sumiu")
	m := NewRunManager(store, func(context.Context, func(domain.Transaction) error) error { return falha },
		func(domain.RunParams) (Pipeline, error) {
			return Pipeline{Features: &fakeFeatures{}, Classifier: &fakeClassifier{}}, nil
		}, nil)
	t.Cleanup(m.Shutdown)

	if _, err := m.Start(paramsDeTeste()); err != nil {
		t.Fatal(err)
	}
	m.Wait()
	sum := store.ultimoResumo()
	if sum.Status != domain.StatusFailed || !strings.Contains(sum.Error, "o CSV sumiu") || sum.FinishedAt == nil {
		t.Errorf("resumo = %+v", sum)
	}
}

func TestRunManagerFalhaQuandoOModeloNaoResponde(t *testing.T) {
	cl := &fakeClassifier{fn: func(context.Context, string) (domain.Decision, error) {
		return domain.Decision{}, errors.New("conexão recusada")
	}}
	m, store := novoGerenciador(t, txsDeTeste(300), cl)
	if _, err := m.Start(paramsDeTeste()); err != nil {
		t.Fatal(err)
	}
	m.Wait()
	sum := store.ultimoResumo()
	if sum.Status != domain.StatusFailed || !strings.Contains(sum.Error, "aquecimento") {
		t.Errorf("resumo = %+v", sum)
	}
}

func TestRunManagerReportaProgresso(t *testing.T) {
	liberar := make(chan struct{})
	cl := &fakeClassifier{fn: func(ctx context.Context, state string) (domain.Decision, error) {
		if seqDoState(state) >= 0 {
			select {
			case <-liberar:
			case <-ctx.Done():
				return domain.Decision{}, ctx.Err()
			}
		}
		return domain.Decision{Noul: 0.1, Attempts: 1, Latency: time.Millisecond}, nil
	}}
	m, _ := novoGerenciador(t, txsDeTeste(300), cl)

	sum, err := m.Start(paramsDeTeste())
	if err != nil {
		t.Fatal(err)
	}
	esperar(t, "total do progresso", func() bool {
		p, ok := m.Progress(sum.RunID)
		return ok && p.Total == 30
	})
	if _, ok := m.Progress("run_de_outra"); ok {
		t.Error("Progress de outro id deveria devolver false")
	}
	close(liberar)
	m.Wait()
	if _, ok := m.Progress(sum.RunID); ok {
		t.Error("depois de terminar, não há progresso ativo")
	}
}

func TestRunManagerShutdownCancelaEEspera(t *testing.T) {
	cl := &fakeClassifier{fn: func(ctx context.Context, state string) (domain.Decision, error) {
		if seqDoState(state) >= 0 {
			<-ctx.Done()
			return domain.Decision{}, ctx.Err()
		}
		return domain.Decision{Noul: 0.1, Attempts: 1, Latency: time.Millisecond}, nil
	}}
	m, store := novoGerenciador(t, txsDeTeste(300), cl)
	if _, err := m.Start(paramsDeTeste()); err != nil {
		t.Fatal(err)
	}
	esperar(t, "a execução começar a chamar o modelo", func() bool { return cl.calls.Load() > 3 })

	acabou := make(chan struct{})
	go func() {
		m.Shutdown()
		close(acabou)
	}()
	select {
	case <-acabou:
	case <-time.After(3 * time.Second):
		t.Fatal("Shutdown não terminou")
	}
	if store.ultimoResumo().Status != domain.StatusCanceled {
		t.Errorf("status = %s", store.ultimoResumo().Status)
	}
}
