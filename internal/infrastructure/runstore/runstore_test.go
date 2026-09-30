package runstore

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fraud-detector-jev/internal/domain"
)

func novaLoja(t *testing.T) *Store {
	t.Helper()
	s, err := New(filepath.Join(t.TempDir(), "runs"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func resumo(id string, status domain.RunStatus, inicio time.Time) domain.RunSummary {
	return domain.RunSummary{
		RunID: id, Status: status, StartedAt: inicio,
		Params: domain.RunParams{N: 100, FraudPct: 10, Seed: 42, Model: "tev1:4b", Graph: true, Types: []string{"TRANSFER"}},
	}
}

func TestNewRunIDTemFormatoValido(t *testing.T) {
	s := novaLoja(t)
	id := s.NewRunID(time.Date(2026, 9, 30, 14, 30, 5, 0, time.UTC))
	if !idPattern.MatchString(id) || id[:20] != "run_20260930_143005_" {
		t.Errorf("id = %q", id)
	}
	if s.NewRunID(time.Now()) == s.NewRunID(time.Now()) {
		t.Error("dois ids seguidos não deveriam ser iguais")
	}
}

func TestSaveELerResumo(t *testing.T) {
	s := novaLoja(t)
	inicio := time.Date(2026, 9, 30, 14, 30, 5, 0, time.UTC)
	fim := inicio.Add(90 * time.Second)
	original := resumo("run_20260930_143005_ab12", domain.StatusDone, inicio)
	original.FinishedAt = &fim
	original.DurationS = 90
	original.Counts = &domain.Counts{Confusion: domain.Confusion{TP: 5, TN: 90}, Errors: 1}
	original.Sample = &domain.SampleInfo{N: 100, Prevalence: 0.003}

	if err := s.SaveSummary(original); err != nil {
		t.Fatal(err)
	}
	volta, err := s.ReadSummary(original.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if volta.RunID != original.RunID || volta.Status != domain.StatusDone || !volta.StartedAt.Equal(inicio) ||
		volta.FinishedAt == nil || !volta.FinishedAt.Equal(fim) || volta.Counts == nil || volta.Counts.TP != 5 ||
		volta.Counts.Errors != 1 || volta.Sample.Prevalence != 0.003 || volta.Params.Model != "tev1:4b" {
		t.Errorf("resumo voltou diferente: %+v", volta)
	}

	// Regravar substitui.
	original.Status = domain.StatusFailed
	if err := s.SaveSummary(original); err != nil {
		t.Fatal(err)
	}
	volta, _ = s.ReadSummary(original.RunID)
	if volta.Status != domain.StatusFailed {
		t.Errorf("status = %s", volta.Status)
	}
}

func TestIdsInvalidosOuInexistentesDaoNotFound(t *testing.T) {
	s := novaLoja(t)
	for _, id := range []string{"", "../etc/passwd", "run_x", "run_20260930_143005_ab12/../..", "RUN_20260930_143005_ab12", "run_20260930_143005_zzzz"} {
		if _, err := s.ReadSummary(id); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("ReadSummary(%q) = %v, esperava ErrNotFound", id, err)
		}
		if _, err := s.OpenResults(id); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("OpenResults(%q) = %v, esperava ErrNotFound", id, err)
		}
	}
	// Id com formato certo, mas que nunca foi gravado.
	if _, err := s.ReadSummary("run_20260930_143005_ab12"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("id inexistente = %v", err)
	}
	if _, err := s.OpenResults("run_20260930_143005_ab12"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("results inexistente = %v", err)
	}
	if err := s.SaveSummary(resumo("../fora", domain.StatusRunning, time.Now())); err == nil {
		t.Error("SaveSummary com id malicioso deveria falhar")
	}
}

func TestListOrdenaDaMaisNovaParaAMaisAntigaEIgnoraLixo(t *testing.T) {
	s := novaLoja(t)
	base := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	for i, id := range []string{"run_20260930_100000_aaaa", "run_20260930_120000_bbbb", "run_20260930_110000_cccc"} {
		if err := s.SaveSummary(resumo(id, domain.StatusDone, base.Add(time.Duration([]int{0, 2, 1}[i])*time.Hour))); err != nil {
			t.Fatal(err)
		}
	}
	// Lixo que não pode quebrar a listagem.
	if err := os.WriteFile(filepath.Join(s.dir, "solto.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(s.dir, "pasta-qualquer"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(s.dir, "run_20260930_130000_dddd"), 0o755); err != nil { // sem summary.json
		t.Fatal(err)
	}

	lista, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(lista) != 3 || lista[0].RunID != "run_20260930_120000_bbbb" || lista[1].RunID != "run_20260930_110000_cccc" || lista[2].RunID != "run_20260930_100000_aaaa" {
		t.Errorf("lista = %+v", lista)
	}
}

func TestResultadosSaoGravadosELidosLinhaALinha(t *testing.T) {
	s := novaLoja(t)
	id := "run_20260930_143005_ab12"
	if err := s.SaveSummary(resumo(id, domain.StatusRunning, time.Now())); err != nil {
		t.Fatal(err)
	}
	w, err := s.OpenResultsWriter(id)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		noul := 0.5 + float64(i)/10
		if err := w.Write(domain.Result{TransactionID: "TX_" + string(rune('0'+i)), Seq: int64(i), Noul: &noul}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	r, err := s.OpenResults(id)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	sc := bufio.NewScanner(r)
	linhas := 0
	for sc.Scan() {
		var res domain.Result
		if err := json.Unmarshal(sc.Bytes(), &res); err != nil {
			t.Fatalf("linha %d não é JSON: %v", linhas, err)
		}
		if res.Seq != int64(linhas) || res.Noul == nil {
			t.Errorf("linha %d = %+v", linhas, res)
		}
		linhas++
	}
	if linhas != 3 {
		t.Errorf("linhas = %d", linhas)
	}
}

func TestMarkStaleRunsFailed(t *testing.T) {
	s := novaLoja(t)
	inicio := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	_ = s.SaveSummary(resumo("run_20260930_100000_aaaa", domain.StatusRunning, inicio))
	_ = s.SaveSummary(resumo("run_20260930_110000_bbbb", domain.StatusDone, inicio))

	agora := inicio.Add(time.Minute)
	n, err := s.MarkStaleRunsFailed(agora)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	a, _ := s.ReadSummary("run_20260930_100000_aaaa")
	b, _ := s.ReadSummary("run_20260930_110000_bbbb")
	if a.Status != domain.StatusFailed || a.Error == "" || a.FinishedAt == nil || a.DurationS != 60 {
		t.Errorf("a = %+v", a)
	}
	if b.Status != domain.StatusDone {
		t.Errorf("b não deveria mudar: %+v", b)
	}
}

func TestEstatisticasDoDataset(t *testing.T) {
	s := novaLoja(t)
	if _, err := s.ReadDatasetStats(); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("sem arquivo deveria ser ErrNotFound, veio %v", err)
	}
	want := domain.DatasetStats{TotalRows: 10, FraudRows: 1, MaxStep: 5, ByType: map[string]domain.TypeStats{"TRANSFER": {Rows: 10, Frauds: 1}}}
	if err := s.WriteDatasetStats(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReadDatasetStats()
	if err != nil || got.TotalRows != 10 || got.ByType["TRANSFER"].Frauds != 1 || got.MaxStep != 5 {
		t.Errorf("got=%+v err=%v", got, err)
	}
}
