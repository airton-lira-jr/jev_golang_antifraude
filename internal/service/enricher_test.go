package service

import (
	"encoding/json"
	"testing"

	"fraud-detector-jev/internal/domain"
)

func estadoComoMapa(t *testing.T, tx domain.Transaction, gf *domain.GraphFeatures) map[string]any {
	t.Helper()
	js, err := BuildState(tx, gf).JSON()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(js), &m); err != nil {
		t.Fatalf("o state não é JSON válido: %v", err)
	}
	return m
}

func txExemplo() domain.Transaction {
	return domain.Transaction{
		Seq: 1029, Step: 4, Type: "TRANSFER", Amount: 350000,
		Orig: "C111", OldBalanceOrig: 350000, NewBalanceOrig: 0,
		Dest: "C222", OldBalanceDest: 0, NewBalanceDest: 0,
	}
}

func TestBuildStateSemGrafo(t *testing.T) {
	m := estadoComoMapa(t, txExemplo(), nil)
	if m["transaction_id"] != "TX_1029" || m["type"] != "TRANSFER" || m["time_step"] != float64(4) ||
		m["amount"] != float64(350000) || m["balance_drop"] != float64(350000) || m["dest_is_merchant"] != false {
		t.Errorf("campos tabulares errados: %v", m)
	}
	for _, chave := range []string{"orig_balance_before", "orig_balance_after", "dest_balance_before", "dest_balance_after"} {
		if _, ok := m[chave]; !ok {
			t.Errorf("faltou %s", chave)
		}
	}
	if _, ok := m["graph_features"]; ok {
		t.Error("sem grafo não pode ter graph_features")
	}
	for _, proibida := range []string{"orig", "dest", "nameOrig", "nameDest", "is_fraud", "isFraud", "seq"} {
		if _, ok := m[proibida]; ok {
			t.Errorf("o campo %s não pode ir para o modelo", proibida)
		}
	}
}

func TestBuildStateComGrafo(t *testing.T) {
	idade := 3
	gf := &domain.GraphFeatures{
		FirstTransferBetweenPair: true, DestInDegree: 42, DestFanInWindow: 17,
		DestAccountAgeSteps: &idade, CycleDetected: true,
	}
	m := estadoComoMapa(t, txExemplo(), gf)
	g, ok := m["graph_features"].(map[string]any)
	if !ok {
		t.Fatalf("faltou graph_features: %v", m)
	}
	if g["dest_in_degree"] != float64(42) || g["cycle_detected"] != true || g["dest_account_age_steps"] != float64(3) {
		t.Errorf("graph_features = %v", g)
	}

	gf.DestAccountAgeSteps = nil
	g = estadoComoMapa(t, txExemplo(), gf)["graph_features"].(map[string]any)
	if v, existe := g["dest_account_age_steps"]; !existe || v != nil {
		t.Errorf("idade ausente deveria ser null, veio %v (existe=%v)", v, existe)
	}
}

func TestFeatureQueryFor(t *testing.T) {
	q := FeatureQueryFor(txExemplo(), 24, 3)
	want := domain.FeatureQuery{Orig: "C111", Dest: "C222", BeforeSeq: 1029, AtStep: 4, WindowHours: 24, CycleMaxHops: 3}
	if q != want {
		t.Errorf("FeatureQueryFor = %+v, esperado %+v", q, want)
	}
}
