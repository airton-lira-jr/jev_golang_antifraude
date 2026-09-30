//go:build integration

package neo4j

import (
	"context"
	"math"
	"testing"

	"fraud-detector-jev/internal/domain"
)

func idade(n int) *int { return &n }

func igualIdade(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func TestFeaturesNosCenariosDaFixture(t *testing.T) {
	c := clienteDeTeste(t)
	carregarFixture(t, c)

	casos := []struct {
		nome string
		q    domain.FeatureQuery
		want domain.GraphFeatures
	}{
		{
			"seq0 A para B, primeira transação de todas",
			domain.FeatureQuery{Orig: "A", Dest: "B", BeforeSeq: 0, AtStep: 1, WindowHours: 24, CycleMaxHops: 3},
			domain.GraphFeatures{FirstTransferBetweenPair: true},
		},
		{
			"seq1 B para C, o futuro (C para A no seq2) não pode fechar ciclo",
			domain.FeatureQuery{Orig: "B", Dest: "C", BeforeSeq: 1, AtStep: 1, WindowHours: 24, CycleMaxHops: 3},
			domain.GraphFeatures{FirstTransferBetweenPair: true},
		},
		{
			"seq2 C para A fecha o ciclo A para B para C para A",
			domain.FeatureQuery{Orig: "C", Dest: "A", BeforeSeq: 2, AtStep: 2, WindowHours: 24, CycleMaxHops: 3},
			domain.GraphFeatures{FirstTransferBetweenPair: true, DestFanOutWindow: 1, DestAccountAgeSteps: idade(1), CycleDetected: true},
		},
		{
			"seq2 com limite de 1 salto não enxerga o ciclo de 2 saltos",
			domain.FeatureQuery{Orig: "C", Dest: "A", BeforeSeq: 2, AtStep: 2, WindowHours: 24, CycleMaxHops: 1},
			domain.GraphFeatures{FirstTransferBetweenPair: true, DestFanOutWindow: 1, DestAccountAgeSteps: idade(1)},
		},
		{
			"seq6 G para X, os 3 remetentes anteriores estão fora da janela de 24h",
			domain.FeatureQuery{Orig: "G", Dest: "X", BeforeSeq: 6, AtStep: 30, WindowHours: 24, CycleMaxHops: 3},
			domain.GraphFeatures{FirstTransferBetweenPair: true, DestInDegree: 3, DestAccountAgeSteps: idade(28)},
		},
		{
			"seq6 com janela de 48h passa a enxergar os 3 remetentes",
			domain.FeatureQuery{Orig: "G", Dest: "X", BeforeSeq: 6, AtStep: 30, WindowHours: 48, CycleMaxHops: 3},
			domain.GraphFeatures{FirstTransferBetweenPair: true, DestInDegree: 3, DestFanInWindow: 3, DestAccountAgeSteps: idade(28)},
		},
		{
			"seq9 H para R, H já mandou para P e Q na mesma hora",
			domain.FeatureQuery{Orig: "H", Dest: "R", BeforeSeq: 9, AtStep: 31, WindowHours: 24, CycleMaxHops: 3},
			domain.GraphFeatures{FirstTransferBetweenPair: true, OrigFanOutWindow: 2},
		},
		{
			"seq10 A para B repete o par, e B para C para A é um ciclo",
			domain.FeatureQuery{Orig: "A", Dest: "B", BeforeSeq: 10, AtStep: 32, WindowHours: 24, CycleMaxHops: 3},
			domain.GraphFeatures{FirstTransferBetweenPair: false, DestInDegree: 1, DestAccountAgeSteps: idade(31), CycleDetected: true},
		},
		{
			"modo agora: origem que nunca apareceu e destino X, com o grafo inteiro",
			domain.FeatureQuery{Orig: "ZZ", Dest: "X", BeforeSeq: math.MaxInt64, AtStep: 32, WindowHours: 24, CycleMaxHops: 3},
			domain.GraphFeatures{FirstTransferBetweenPair: true, DestInDegree: 4, DestFanInWindow: 1, DestAccountAgeSteps: idade(30)},
		},
		{
			"contas que nunca apareceram: tudo zerado, idade nula e sem erro",
			domain.FeatureQuery{Orig: "ZZ", Dest: "YY", BeforeSeq: math.MaxInt64, AtStep: 32, WindowHours: 24, CycleMaxHops: 3},
			domain.GraphFeatures{FirstTransferBetweenPair: true},
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			got, err := c.Features(context.Background(), caso.q)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if !igualIdade(got.DestAccountAgeSteps, caso.want.DestAccountAgeSteps) {
				t.Errorf("idade = %v, esperada %v", ponteiro(got.DestAccountAgeSteps), ponteiro(caso.want.DestAccountAgeSteps))
			}
			got.DestAccountAgeSteps, caso.want.DestAccountAgeSteps = nil, nil
			if got != caso.want {
				t.Errorf("features = %+v\nesperadas  %+v", got, caso.want)
			}
		})
	}
}

func ponteiro(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestSubgraphDaVizinhanca(t *testing.T) {
	c := clienteDeTeste(t)
	carregarFixture(t, c)

	sg, err := c.Subgraph(context.Background(), "A", "X", math.MaxInt64, 32, 50)
	if err != nil {
		t.Fatal(err)
	}
	// A tem 3 arestas (A para B, C para A, A para B) e X tem 4 (D, E, F, G).
	if len(sg.Edges) != 7 {
		t.Errorf("esperava 7 arestas, vieram %d: %+v", len(sg.Edges), sg.Edges)
	}
	ids := map[string]bool{}
	for _, e := range sg.Edges {
		if ids[e.ID] {
			t.Errorf("aresta repetida: %s", e.ID)
		}
		ids[e.ID] = true
	}
	tem := map[string]bool{}
	for _, n := range sg.Nodes {
		tem[n] = true
	}
	for _, esperado := range []string{"A", "X", "B", "C", "D", "E", "F", "G"} {
		if !tem[esperado] {
			t.Errorf("faltou o nó %s em %v", esperado, sg.Nodes)
		}
	}

	limitado, err := c.Subgraph(context.Background(), "A", "X", math.MaxInt64, 32, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(limitado.Edges) != 4 {
		t.Errorf("com limite de 2 por conta esperava 4 arestas, vieram %d", len(limitado.Edges))
	}
}

func TestSubgraphDeContasQueNaoExistem(t *testing.T) {
	c := clienteDeTeste(t)
	carregarFixture(t, c)
	sg, err := c.Subgraph(context.Background(), "ZZ", "YY", math.MaxInt64, 32, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(sg.Edges) != 0 || len(sg.Nodes) != 2 {
		t.Errorf("subgrafo = %+v", sg)
	}
}
