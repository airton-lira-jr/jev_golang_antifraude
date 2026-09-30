package neo4j

import (
	"context"
	"strings"
	"testing"

	"fraud-detector-jev/internal/domain"
)

// A query de feature nunca pode ler o gabarito. Se alguém colocar isFraud ou
// isFlagged aqui, o benchmark passa a trapacear e este teste avisa.
func TestFeaturesQueryNaoLeOGabarito(t *testing.T) {
	q, err := FeaturesQuery(3)
	if err != nil {
		t.Fatal(err)
	}
	baixo := strings.ToLower(q)
	for _, proibida := range []string{"isfraud", "isflagged"} {
		if strings.Contains(baixo, proibida) {
			t.Errorf("a query de features não pode citar %s", proibida)
		}
	}
	for _, exigida := range []string{"$beforeSeq", "$atStep", "$window", "*1..3"} {
		if !strings.Contains(q, exigida) {
			t.Errorf("a query de features deveria conter %s", exigida)
		}
	}
}

func TestFeaturesQueryValidaSaltos(t *testing.T) {
	for _, ruim := range []int{0, -1, 6, 100} {
		if _, err := FeaturesQuery(ruim); err == nil {
			t.Errorf("%d saltos deveria dar erro", ruim)
		}
	}
	for _, bom := range []int{1, 2, 3, 4, 5} {
		if _, err := FeaturesQuery(bom); err != nil {
			t.Errorf("%d saltos deveria valer: %v", bom, err)
		}
	}
}

func TestFeaturesRecusaSaltosInvalidosSemTocarNoBanco(t *testing.T) {
	// O cliente vazio nunca é usado, porque a validação vem antes da consulta.
	c := &Client{}
	_, err := c.Features(context.Background(), domain.FeatureQuery{Orig: "A", Dest: "B", CycleMaxHops: 9})
	if err == nil {
		t.Fatal("esperava erro")
	}
}
