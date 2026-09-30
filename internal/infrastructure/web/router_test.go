package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"fraud-detector-jev/internal/domain"
	"fraud-detector-jev/internal/service"
)

const idExemplo = "run_20260930_120000_abcd"

type fakeRuns struct {
	mu         sync.Mutex
	started    []domain.RunParams
	startErr   error
	cancelavel string
	active     string
	progress   domain.Progress
}

func (f *fakeRuns) Start(p domain.RunParams) (domain.RunSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = append(f.started, p)
	if f.startErr != nil {
		return domain.RunSummary{}, f.startErr
	}
	return domain.RunSummary{RunID: idExemplo, Status: domain.StatusRunning, Params: p}, nil
}
func (f *fakeRuns) Cancel(id string) bool { return id == f.cancelavel }
func (f *fakeRuns) Progress(id string) (domain.Progress, bool) {
	if id == f.active {
		return f.progress, true
	}
	return domain.Progress{}, false
}
func (f *fakeRuns) ActiveID() string { return f.active }

type fakeStore struct {
	summaries   map[string]domain.RunSummary
	results     map[string]string
	stats       *domain.DatasetStats
	panicOnList bool
}

func (f *fakeStore) ReadSummary(id string) (domain.RunSummary, error) {
	if s, ok := f.summaries[id]; ok {
		return s, nil
	}
	return domain.RunSummary{}, fmt.Errorf("%s: %w", id, domain.ErrNotFound)
}
func (f *fakeStore) List() ([]domain.RunSummary, error) {
	if f.panicOnList {
		panic("bug de propósito")
	}
	var out []domain.RunSummary
	for _, s := range f.summaries {
		out = append(out, s)
	}
	return out, nil
}
func (f *fakeStore) OpenResults(id string) (io.ReadCloser, error) {
	if r, ok := f.results[id]; ok {
		return io.NopCloser(strings.NewReader(r)), nil
	}
	return nil, fmt.Errorf("%s: %w", id, domain.ErrNotFound)
}
func (f *fakeStore) ReadDatasetStats() (domain.DatasetStats, error) {
	if f.stats == nil {
		return domain.DatasetStats{}, domain.ErrNotFound
	}
	return *f.stats, nil
}

type fakeEval struct {
	last service.EvaluateRequest
	resp service.EvaluateResponse
	err  error
}

func (f *fakeEval) Evaluate(_ context.Context, req service.EvaluateRequest) (service.EvaluateResponse, error) {
	f.last = req
	return f.resp, f.err
}

type ambiente struct {
	srv   *Server
	runs  *fakeRuns
	store *fakeStore
	eval  *fakeEval
}

func novoAmbiente() *ambiente {
	a := &ambiente{runs: &fakeRuns{}, eval: &fakeEval{}}
	a.store = &fakeStore{summaries: map[string]domain.RunSummary{}, results: map[string]string{}}
	a.srv = &Server{
		Runs: a.runs, Store: a.store, Evaluator: a.eval,
		Limits:     service.Limits{MaxSampleSize: 500000, MaxWorkers: 256, DefaultModel: "tev1:4b", BaseURL: "http://ollama:11434"},
		PingNeo4j:  func(context.Context) error { return nil },
		ListModels: func(context.Context) ([]string, error) { return []string{"tev1:4b", "llama3:8b", "nimble:latest"}, nil },
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return a
}

func (a *ambiente) chamar(metodo, caminho, corpo string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(metodo, caminho, strings.NewReader(corpo))
	rec := httptest.NewRecorder()
	a.srv.Handler().ServeHTTP(rec, req)
	return rec
}

func jsonDe(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("a resposta não é JSON (%s): %v", rec.Body.String(), err)
	}
	return m
}

func codigoDoErro(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	erro, ok := jsonDe(t, rec)["error"].(map[string]any)
	if !ok {
		t.Fatalf("resposta sem o campo error: %s", rec.Body.String())
	}
	return erro["code"].(string)
}

func TestHealthz(t *testing.T) {
	a := novoAmbiente()
	rec := a.chamar("GET", "/healthz", "")
	if rec.Code != 200 || jsonDe(t, rec)["status"] != "ok" {
		t.Errorf("tudo ok: %d %s", rec.Code, rec.Body)
	}

	a.srv.ListModels = func(context.Context) ([]string, error) { return nil, errors.New("conexão recusada") }
	rec = a.chamar("GET", "/healthz", "")
	corpo := jsonDe(t, rec)
	ollama := corpo["ollama"].(map[string]any)
	if rec.Code != 200 || corpo["status"] != "degraded" || ollama["ok"] != false || !strings.Contains(ollama["hint"].(string), "OLLAMA_HOST") {
		t.Errorf("Ollama fora deveria dar 200 degraded com a dica: %d %s", rec.Code, rec.Body)
	}

	a.srv.PingNeo4j = func(context.Context) error { return errors.New("neo4j fora") }
	rec = a.chamar("GET", "/healthz", "")
	if rec.Code != 503 || jsonDe(t, rec)["status"] != "unavailable" {
		t.Errorf("Neo4j fora deveria dar 503: %d %s", rec.Code, rec.Body)
	}
}

func TestInfo(t *testing.T) {
	a := novoAmbiente()
	a.store.stats = &domain.DatasetStats{TotalRows: 100, FraudRows: 2, MaxStep: 743, ByType: map[string]domain.TypeStats{"TRANSFER": {Rows: 60, Frauds: 2}}}
	a.runs.active = idExemplo

	corpo := jsonDe(t, a.chamar("GET", "/info", ""))
	if corpo["model"] != "tev1:4b" || corpo["base_url"] != "http://ollama:11434" || corpo["ollama_ok"] != true || corpo["active_run"] != idExemplo {
		t.Errorf("info = %v", corpo)
	}
	modelos := corpo["available_models"].([]any)
	if len(modelos) != 2 || modelos[0] != "tev1:4b" || modelos[1] != "nimble:latest" {
		t.Errorf("só modelos de decisão deveriam aparecer: %v", modelos)
	}
	dataset := corpo["dataset"].(map[string]any)
	if dataset["total_rows"] != float64(100) || dataset["max_step"] != float64(743) {
		t.Errorf("dataset = %v", dataset)
	}
	padroes := corpo["defaults"].(map[string]any)
	if padroes["n"] != float64(2000) || padroes["fraud_pct"] != float64(10) || padroes["workers"] != float64(32) ||
		padroes["graph"] != true || padroes["threshold"] != 0.5 || padroes["window_hours"] != float64(24) ||
		padroes["cycle_max_hops"] != float64(3) || padroes["max_inflight"] != float64(4) || padroes["seed"] != float64(42) {
		t.Errorf("padrões = %v", padroes)
	}
	if tipos := padroes["types"].([]any); len(tipos) != 2 || tipos[0] != "TRANSFER" {
		t.Errorf("tipos = %v", tipos)
	}
	limites := corpo["limits"].(map[string]any)
	if limites["max_sample_size"] != float64(500000) || limites["max_workers"] != float64(256) {
		t.Errorf("limites = %v", limites)
	}
}

func TestInfoSemDatasetEComOllamaFora(t *testing.T) {
	a := novoAmbiente()
	a.srv.ListModels = func(context.Context) ([]string, error) { return nil, errors.New("fora") }
	corpo := jsonDe(t, a.chamar("GET", "/info", ""))
	if corpo["dataset"] != nil || corpo["dataset_hint"] == "" || corpo["ollama_ok"] != false || corpo["active_run"] != nil {
		t.Errorf("info = %v", corpo)
	}
	if modelos, ok := corpo["available_models"].([]any); !ok || len(modelos) != 0 {
		t.Errorf("available_models deveria ser lista vazia: %v", corpo["available_models"])
	}
}

func TestIniciarBenchmarkComCorpoVazioUsaPadroes(t *testing.T) {
	a := novoAmbiente()
	rec := a.chamar("POST", "/benchmarks", "")
	if rec.Code != 202 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if len(a.runs.started) != 1 {
		t.Fatalf("Start chamado %d vezes", len(a.runs.started))
	}
	p := a.runs.started[0]
	if p.N != 2000 || p.Model != "tev1:4b" || p.BaseURL != "http://ollama:11434" || !p.Graph {
		t.Errorf("params = %+v", p)
	}
	corpo := jsonDe(t, rec)
	if corpo["run_id"] != idExemplo || corpo["status"] != "running" {
		t.Errorf("resposta = %v", corpo)
	}
}

func TestIniciarBenchmarkUsaOsValoresDoPedido(t *testing.T) {
	a := novoAmbiente()
	rec := a.chamar("POST", "/benchmarks", `{"n":100,"graph":false,"model":"nimble","fraud_pct":5,"types":["CASH_OUT"]}`)
	if rec.Code != 202 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	p := a.runs.started[0]
	if p.N != 100 || p.Graph || p.Model != "nimble" || p.FraudPct != 5 || len(p.Types) != 1 || p.Types[0] != "CASH_OUT" {
		t.Errorf("params = %+v", p)
	}
}

func TestIniciarBenchmarkRejeitaPedidosRuins(t *testing.T) {
	a := novoAmbiente()

	rec := a.chamar("POST", "/benchmarks", `{"n":0,"workers":9999}`)
	if rec.Code != 400 || codigoDoErro(t, rec) != "validation_error" {
		t.Errorf("validação: %d %s", rec.Code, rec.Body)
	}
	detalhes := jsonDe(t, rec)["error"].(map[string]any)["details"].([]any)
	if len(detalhes) != 2 {
		t.Errorf("deveria listar os 2 problemas: %v", detalhes)
	}

	for nome, corpo := range map[string]string{
		"JSON quebrado":      `{"n":`,
		"campo desconhecido": `{"quantidade":10}`,
		"tipo errado":        `{"n":"muitos"}`,
		"conteúdo extra":     `{"n":10} {"n":20}`,
	} {
		rec := a.chamar("POST", "/benchmarks", corpo)
		if rec.Code != 400 || codigoDoErro(t, rec) != "invalid_json" {
			t.Errorf("%s: %d %s", nome, rec.Code, rec.Body)
		}
	}
	if len(a.runs.started) != 0 {
		t.Error("pedido inválido não pode iniciar execução")
	}
}

func TestIniciarBenchmarkOcupadoDa409(t *testing.T) {
	a := novoAmbiente()
	a.runs.startErr = service.ErrBusy
	a.runs.active = idExemplo
	rec := a.chamar("POST", "/benchmarks", "{}")
	if rec.Code != 409 || codigoDoErro(t, rec) != "run_in_progress" {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

func TestIniciarBenchmarkComFalhaInternaDa500(t *testing.T) {
	a := novoAmbiente()
	a.runs.startErr = errors.New("preparando a execução: modelo inválido")
	rec := a.chamar("POST", "/benchmarks", "{}")
	if rec.Code != 500 || codigoDoErro(t, rec) != "start_failed" {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

func TestListarEObterBenchmarks(t *testing.T) {
	a := novoAmbiente()
	a.store.summaries[idExemplo] = domain.RunSummary{RunID: idExemplo, Status: domain.StatusRunning}
	a.store.summaries["run_20260930_110000_beef"] = domain.RunSummary{RunID: "run_20260930_110000_beef", Status: domain.StatusDone}
	a.runs.active = idExemplo
	a.runs.progress = domain.Progress{Done: 10, Total: 40}

	corpo := jsonDe(t, a.chamar("GET", "/benchmarks", ""))
	runs := corpo["runs"].([]any)
	if len(runs) != 2 {
		t.Fatalf("runs = %v", runs)
	}
	comProgresso := 0
	for _, r := range runs {
		run := r.(map[string]any)
		if run["run_id"] == idExemplo {
			p := run["progress"].(map[string]any)
			if p["done"] != float64(10) || p["total"] != float64(40) {
				t.Errorf("progresso = %v", p)
			}
			comProgresso++
		} else if run["progress"] != nil {
			t.Errorf("execução terminada não tem progresso: %v", run)
		}
	}
	if comProgresso != 1 {
		t.Error("a execução ativa deveria trazer o progresso")
	}

	unico := jsonDe(t, a.chamar("GET", "/benchmarks/"+idExemplo, ""))
	if unico["run_id"] != idExemplo || unico["progress"] == nil {
		t.Errorf("get = %v", unico)
	}
}

func TestListarSemExecucoesDevolveListaVazia(t *testing.T) {
	a := novoAmbiente()
	corpo := jsonDe(t, a.chamar("GET", "/benchmarks", ""))
	if runs, ok := corpo["runs"].([]any); !ok || len(runs) != 0 {
		t.Errorf("deveria ser lista vazia e não null: %v", corpo)
	}
}

func TestObterBenchmarkInexistenteOuComIdMalicioso(t *testing.T) {
	a := novoAmbiente()
	for _, id := range []string{idExemplo, "run_x", "..%2F..%2Fetc%2Fpasswd"} {
		rec := a.chamar("GET", "/benchmarks/"+id, "")
		if rec.Code != 404 || codigoDoErro(t, rec) != "not_found" {
			t.Errorf("%s: %d %s", id, rec.Code, rec.Body)
		}
	}
}

func TestCancelarBenchmark(t *testing.T) {
	a := novoAmbiente()
	a.store.summaries[idExemplo] = domain.RunSummary{RunID: idExemplo, Status: domain.StatusRunning}
	a.store.summaries["run_20260930_110000_beef"] = domain.RunSummary{RunID: "run_20260930_110000_beef", Status: domain.StatusDone}
	a.runs.cancelavel = idExemplo

	rec := a.chamar("DELETE", "/benchmarks/"+idExemplo, "")
	if rec.Code != 202 || jsonDe(t, rec)["canceled"] != true {
		t.Errorf("cancelar ativa: %d %s", rec.Code, rec.Body)
	}
	rec = a.chamar("DELETE", "/benchmarks/run_20260930_110000_beef", "")
	if rec.Code != 409 || codigoDoErro(t, rec) != "not_running" {
		t.Errorf("cancelar terminada: %d %s", rec.Code, rec.Body)
	}
	rec = a.chamar("DELETE", "/benchmarks/run_20260930_100000_cafe", "")
	if rec.Code != 404 {
		t.Errorf("cancelar inexistente: %d %s", rec.Code, rec.Body)
	}
}

func TestResultadosEmStreaming(t *testing.T) {
	a := novoAmbiente()
	a.store.summaries[idExemplo] = domain.RunSummary{RunID: idExemplo, Status: domain.StatusDone}
	a.store.results[idExemplo] = "{\"seq\":0}\n{\"seq\":1}\n"
	a.store.summaries["run_20260930_110000_beef"] = domain.RunSummary{RunID: "run_20260930_110000_beef", Status: domain.StatusRunning}
	a.store.summaries["run_20260930_100000_cafe"] = domain.RunSummary{RunID: "run_20260930_100000_cafe", Status: domain.StatusFailed}

	rec := a.chamar("GET", "/benchmarks/"+idExemplo+"/results", "")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/x-ndjson" || rec.Body.String() != "{\"seq\":0}\n{\"seq\":1}\n" {
		t.Errorf("results: %d %q %q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	rec = a.chamar("GET", "/benchmarks/run_20260930_110000_beef/results", "")
	if rec.Code != 409 || codigoDoErro(t, rec) != "run_in_progress" {
		t.Errorf("em andamento: %d %s", rec.Code, rec.Body)
	}
	rec = a.chamar("GET", "/benchmarks/run_20260930_100000_cafe/results", "")
	if rec.Code != 404 {
		t.Errorf("falhou antes de gravar resultados: %d %s", rec.Code, rec.Body)
	}
	rec = a.chamar("GET", "/benchmarks/run_20260930_090000_0000/results", "")
	if rec.Code != 404 {
		t.Errorf("inexistente: %d %s", rec.Code, rec.Body)
	}
}

func TestEvaluate(t *testing.T) {
	a := novoAmbiente()
	a.eval.resp = service.EvaluateResponse{Decision: "fraud", IsFraud: true, Noul: 0.97, Model: "tev1:4b", State: json.RawMessage(`{"transaction_id":"TX_manual"}`)}

	rec := a.chamar("POST", "/transactions/evaluate", `{"orig":"C1","dest":"C2","type":"TRANSFER","amount":350000,"step":31,"seq":1029}`)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	corpo := jsonDe(t, rec)
	if corpo["decision"] != "fraud" || corpo["noul"] != 0.97 || corpo["state"].(map[string]any)["transaction_id"] != "TX_manual" {
		t.Errorf("resposta = %v", corpo)
	}
	if a.eval.last.Orig != "C1" || a.eval.last.Amount != 350000 || a.eval.last.Seq == nil || *a.eval.last.Seq != 1029 || *a.eval.last.Step != 31 {
		t.Errorf("pedido repassado = %+v", a.eval.last)
	}
}

func TestEvaluateErros(t *testing.T) {
	a := novoAmbiente()

	a.eval.err = &service.ValidationError{Problems: []string{"orig não pode ser vazio"}}
	rec := a.chamar("POST", "/transactions/evaluate", `{}`)
	if rec.Code != 400 || codigoDoErro(t, rec) != "validation_error" {
		t.Errorf("validação: %d %s", rec.Code, rec.Body)
	}

	a.eval.err = &service.UpstreamError{Source: "modelo", Err: errors.New("conexão recusada")}
	rec = a.chamar("POST", "/transactions/evaluate", `{"orig":"a"}`)
	if rec.Code != 502 || codigoDoErro(t, rec) != "upstream_error" || !strings.Contains(rec.Body.String(), "modelo") {
		t.Errorf("fornecedor: %d %s", rec.Code, rec.Body)
	}

	a.eval.err = errors.New("bug")
	rec = a.chamar("POST", "/transactions/evaluate", `{"orig":"a"}`)
	if rec.Code != 500 || codigoDoErro(t, rec) != "internal_error" {
		t.Errorf("interno: %d %s", rec.Code, rec.Body)
	}

	for _, corpo := range []string{"", "{", `{"campo_novo":1}`} {
		rec = a.chamar("POST", "/transactions/evaluate", corpo)
		if rec.Code != 400 || codigoDoErro(t, rec) != "invalid_json" {
			t.Errorf("corpo %q: %d %s", corpo, rec.Code, rec.Body)
		}
	}
}

func TestRotaDesconhecidaEMetodoErradoDao404EmJSON(t *testing.T) {
	a := novoAmbiente()
	for _, c := range []struct{ metodo, caminho string }{
		{"GET", "/nao-existe"},
		{"PUT", "/benchmarks"},
		{"POST", "/healthz"},
	} {
		rec := a.chamar(c.metodo, c.caminho, "")
		if rec.Code != 404 || codigoDoErro(t, rec) != "not_found" {
			t.Errorf("%s %s: %d %s", c.metodo, c.caminho, rec.Code, rec.Body)
		}
	}
}

func TestPanicNoHandlerViraErro500EmJSON(t *testing.T) {
	a := novoAmbiente()
	a.store.panicOnList = true
	rec := a.chamar("GET", "/benchmarks", "")
	if rec.Code != 500 || codigoDoErro(t, rec) != "internal_error" {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	// O servidor continua de pé depois do panic.
	if rec := a.chamar("GET", "/healthz", ""); rec.Code != 200 {
		t.Errorf("healthz depois do panic: %d", rec.Code)
	}
}
