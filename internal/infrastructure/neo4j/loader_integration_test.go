//go:build integration

package neo4j

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"fraud-detector-jev/internal/domain"
	"fraud-detector-jev/internal/infrastructure/csv"
)

func TestLoadCarregaGrafoEEstatisticas(t *testing.T) {
	c := clienteDeTeste(t)
	rep := carregarFixture(t, c)

	if rep.Nodes != 12 || rep.Rels != 11 || rep.Resumed {
		t.Errorf("relatório = %+v", rep)
	}
	want := domain.DatasetStats{
		TotalRows: 11, FraudRows: 1, MaxStep: 32,
		ByType: map[string]domain.TypeStats{
			"TRANSFER": {Rows: 10, Frauds: 1},
			"CASH_OUT": {Rows: 1, Frauds: 0},
		},
	}
	if rep.Stats.TotalRows != want.TotalRows || rep.Stats.FraudRows != want.FraudRows || rep.Stats.MaxStep != want.MaxStep ||
		rep.Stats.ByType["TRANSFER"] != want.ByType["TRANSFER"] || rep.Stats.ByType["CASH_OUT"] != want.ByType["CASH_OUT"] {
		t.Errorf("estatísticas = %+v", rep.Stats)
	}

	nodes, rels, err := c.Counts(context.Background())
	if err != nil || nodes != 12 || rels != 11 {
		t.Errorf("Counts = %d %d %v", nodes, rels, err)
	}
}

func TestLoadRetomaDeOndeParou(t *testing.T) {
	c := clienteDeTeste(t)
	carregarFixture(t, c)

	// Simula uma carga que caiu depois do seq 7: apaga as arestas 8 a 10.
	if _, err := c.write(context.Background(), `MATCH ()-[t:TRANSFERRED]->() WHERE t.seq >= 8 DELETE t`, nil); err != nil {
		t.Fatal(err)
	}
	_, rels, _ := c.Counts(context.Background())
	if rels != 8 {
		t.Fatalf("preparação errada, arestas = %d", rels)
	}

	rep := carregarFixture(t, c)
	if !rep.Resumed || rep.Rels != 11 || rep.Nodes != 12 {
		t.Errorf("relatório da retomada = %+v", rep)
	}

	// Rodar de novo, com tudo já carregado, não pode duplicar nada.
	rep = carregarFixture(t, c)
	if rep.Rels != 11 {
		t.Errorf("segunda retomada duplicou arestas: %+v", rep)
	}
}

func TestLoadRecusaCSVForaDeOrdem(t *testing.T) {
	c := clienteDeTeste(t)
	arquivo := filepath.Join(t.TempDir(), "fora_de_ordem.csv")
	conteudo := "step,type,amount,nameOrig,oldbalanceOrg,newbalanceOrig,nameDest,oldbalanceDest,newbalanceDest,isFraud,isFlaggedFraud\n" +
		"5,TRANSFER,1,A,1,0,B,0,1,0,0\n" +
		"4,TRANSFER,1,A,1,0,B,0,1,0,0\n"
	if err := os.WriteFile(arquivo, []byte(conteudo), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := c.Load(context.Background(), LoadOptions{CSVPath: arquivo})
	if !errors.Is(err, csv.ErrUnsortedSteps) {
		t.Fatalf("esperava ErrUnsortedSteps, veio %v", err)
	}
	if _, rels, _ := c.Counts(context.Background()); rels != 0 {
		t.Errorf("não deveria ter gravado arestas, tem %d", rels)
	}
}
