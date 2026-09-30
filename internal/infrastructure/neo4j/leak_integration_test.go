//go:build integration

package neo4j

import (
	"bufio"
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fraud-detector-jev/internal/domain"
	"fraud-detector-jev/internal/infrastructure/csv"
)

// Para cada transação da fixture, compara as features do grafo completo
// (cortadas por seq) com as de um grafo que só contém as linhas anteriores.
// Se alguma query espiar o futuro, os dois resultados divergem.
func TestFeaturesSoEnxergamOPassado(t *testing.T) {
	c := clienteDeTeste(t)

	var txs []domain.Transaction
	err := csv.Stream(context.Background(), caminhoFixture(), func(tx domain.Transaction) error {
		txs = append(txs, tx)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	linhas := linhasDoArquivo(t, caminhoFixture())

	carregarFixture(t, c)
	cheio := map[int64]domain.GraphFeatures{}
	for _, tx := range txs {
		f, err := c.Features(context.Background(), domain.FeatureQuery{
			Orig: tx.Orig, Dest: tx.Dest, BeforeSeq: tx.Seq, AtStep: tx.Step, WindowHours: 24, CycleMaxHops: 3,
		})
		if err != nil {
			t.Fatal(err)
		}
		cheio[tx.Seq] = f
	}

	for _, tx := range txs {
		limparGrafo(t, c)
		truncado := filepath.Join(t.TempDir(), "truncado.csv")
		// linhas[0] é o cabeçalho, então as tx.Seq linhas anteriores são linhas[1 : 1+Seq].
		conteudo := strings.Join(linhas[:1+int(tx.Seq)], "\n") + "\n"
		if err := os.WriteFile(truncado, []byte(conteudo), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Load(context.Background(), LoadOptions{CSVPath: truncado, BatchSize: 4}); err != nil {
			t.Fatalf("carga truncada no seq %d: %v", tx.Seq, err)
		}
		so, err := c.Features(context.Background(), domain.FeatureQuery{
			Orig: tx.Orig, Dest: tx.Dest, BeforeSeq: math.MaxInt64, AtStep: tx.Step, WindowHours: 24, CycleMaxHops: 3,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !igualIdade(so.DestAccountAgeSteps, cheio[tx.Seq].DestAccountAgeSteps) {
			t.Errorf("seq %d: idade diverge (completo %v, truncado %v)", tx.Seq, ponteiro(cheio[tx.Seq].DestAccountAgeSteps), ponteiro(so.DestAccountAgeSteps))
		}
		so.DestAccountAgeSteps = nil
		esperado := cheio[tx.Seq]
		esperado.DestAccountAgeSteps = nil
		if so != esperado {
			t.Errorf("seq %d espiou o futuro:\n  grafo completo  %+v\n  grafo truncado  %+v", tx.Seq, esperado, so)
		}
	}
}

func linhasDoArquivo(t *testing.T, caminho string) []string {
	t.Helper()
	f, err := os.Open(caminho)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var linhas []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) != "" {
			linhas = append(linhas, sc.Text())
		}
	}
	return linhas
}
