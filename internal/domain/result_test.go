package domain

import (
	"encoding/json"
	"testing"
)

func TestResultSemGrafoNaoTemCamposDeGrafo(t *testing.T) {
	tx := Transaction{Seq: 7, Step: 3, Type: "TRANSFER", Orig: "C1", Dest: "C2", Amount: 10, IsFraud: true}
	raw, err := json.Marshal(NewResult(tx, nil))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["transaction_id"] != "TX_7" || m["is_fraud"] != true || m["orig"] != "C1" {
		t.Errorf("campos básicos errados: %v", m)
	}
	if _, existe := m["dest_in_degree"]; existe {
		t.Error("sem grafo não deveria ter dest_in_degree")
	}
	if m["noul"] != nil {
		t.Errorf("noul deveria ser null, veio %v", m["noul"])
	}
}

func TestResultComGrafoAchataOsCampos(t *testing.T) {
	idade := 29
	gf := &GraphFeatures{DestInDegree: 4, CycleDetected: true, DestAccountAgeSteps: &idade}
	raw, err := json.Marshal(NewResult(Transaction{Seq: 1}, gf))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["dest_in_degree"] != float64(4) || m["cycle_detected"] != true || m["dest_account_age_steps"] != float64(29) {
		t.Errorf("features achatadas erradas: %v", m)
	}

	semIdade := &GraphFeatures{}
	raw, _ = json.Marshal(NewResult(Transaction{Seq: 2}, semIdade))
	m = map[string]any{}
	_ = json.Unmarshal(raw, &m)
	if v, existe := m["dest_account_age_steps"]; !existe || v != nil {
		t.Errorf("dest_account_age_steps deveria existir e ser null, veio %v (existe=%v)", v, existe)
	}
}

func TestResultTrazAsColunasOpcionaisDoCSV(t *testing.T) {
	tx := Transaction{Seq: 4, PaymentMethod: "PIX", Channel: "MOBILE_APP", Category: "FOOD"}
	raw, err := json.Marshal(NewResult(tx, nil))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["payment_method"] != "PIX" || m["channel"] != "MOBILE_APP" || m["category"] != "FOOD" {
		t.Errorf("colunas opcionais erradas: %v", m)
	}
}

func TestResultSemAsColunasOpcionaisNaoGravaOsCampos(t *testing.T) {
	raw, err := json.Marshal(NewResult(Transaction{Seq: 5}, nil))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, campo := range []string{"payment_method", "channel", "category"} {
		if _, existe := m[campo]; existe {
			t.Errorf("CSV sem a coluna não deveria gravar %s", campo)
		}
	}
}

func TestResultVoltaDoJSON(t *testing.T) {
	idade := 5
	original := NewResult(Transaction{Seq: 9, Type: "CASH_OUT"}, &GraphFeatures{DestFanInWindow: 3, DestAccountAgeSteps: &idade})
	noul := 0.75
	original.Noul = &noul
	raw, _ := json.Marshal(original)

	var volta Result
	if err := json.Unmarshal(raw, &volta); err != nil {
		t.Fatal(err)
	}
	if volta.GraphFeatures == nil || volta.DestFanInWindow != 3 || *volta.DestAccountAgeSteps != 5 || *volta.Noul != 0.75 {
		t.Errorf("volta = %+v", volta)
	}
}
