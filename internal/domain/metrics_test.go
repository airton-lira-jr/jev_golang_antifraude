package domain

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func perto(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

type casoGolden struct {
	Name       string  `json:"name"`
	TP         int     `json:"tp"`
	FP         int     `json:"fp"`
	TN         int     `json:"tn"`
	FN         int     `json:"fn"`
	Prevalence float64 `json:"prevalence"`
	Expected   Metrics `json:"expected"`
}

func TestMetricasBatemComArquivoCompartilhado(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "metrics_golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var arquivo struct {
		Cases []casoGolden `json:"cases"`
	}
	if err := json.Unmarshal(raw, &arquivo); err != nil {
		t.Fatal(err)
	}
	if len(arquivo.Cases) == 0 {
		t.Fatal("arquivo sem casos")
	}
	for _, c := range arquivo.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got := ComputeMetrics(Confusion{TP: c.TP, FP: c.FP, TN: c.TN, FN: c.FN}, c.Prevalence)
			e := c.Expected
			if !perto(got.Accuracy, e.Accuracy) || !perto(got.Precision, e.Precision) ||
				!perto(got.Recall, e.Recall) || !perto(got.F1, e.F1) ||
				!perto(got.FPR, e.FPR) || !perto(got.PrecisionAtPrevalence, e.PrecisionAtPrevalence) {
				t.Errorf("got %+v, esperado %+v", got, e)
			}
		})
	}
}

func TestConfusionAdd(t *testing.T) {
	var c Confusion
	c.Add(true, true)
	c.Add(true, false)
	c.Add(false, true)
	c.Add(false, false)
	c.Add(false, false)
	if c != (Confusion{TP: 1, FN: 1, FP: 1, TN: 2}) {
		t.Errorf("Confusion = %+v", c)
	}
	if c.Total() != 5 {
		t.Errorf("Total = %d", c.Total())
	}
}

func TestDecide(t *testing.T) {
	if !Decide(0.5, 0.5) {
		t.Error("no limiar exato deve ser fraude")
	}
	if Decide(0.49, 0.5) {
		t.Error("abaixo do limiar não é fraude")
	}
}

func TestPercentilEhExato(t *testing.T) {
	var de1a100 []float64
	for i := 1; i <= 100; i++ {
		de1a100 = append(de1a100, float64(i))
	}
	st := ComputeLatency(de1a100)
	if st.P50 != 50 || st.P95 != 95 || st.P99 != 99 {
		t.Errorf("1..100 = %+v", st)
	}
	quatro := ComputeLatency([]float64{40, 10, 30, 20})
	if quatro.P50 != 20 || quatro.P95 != 40 || quatro.P99 != 40 {
		t.Errorf("4 valores desordenados = %+v", quatro)
	}
	if vazio := ComputeLatency(nil); vazio != (LatencyStats{}) {
		t.Errorf("vazio = %+v", vazio)
	}
}

func TestComputeLatencyNaoAlteraEntrada(t *testing.T) {
	entrada := []float64{3, 1, 2}
	ComputeLatency(entrada)
	if entrada[0] != 3 || entrada[1] != 1 || entrada[2] != 2 {
		t.Errorf("entrada foi reordenada: %v", entrada)
	}
}
