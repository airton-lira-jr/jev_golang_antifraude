package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"

	"fraud-detector-jev/internal/domain"
)

// ErrEmptySample aparece quando nenhuma transação dos tipos pedidos existe.
var ErrEmptySample = errors.New("nenhuma transação elegível para os tipos pedidos")

// TxSource entrega as transações uma a uma, em ordem. O main liga isso ao
// leitor de CSV, e os testes usam uma fatia em memória.
type TxSource func(ctx context.Context, fn func(domain.Transaction) error) error

// SampleParams define a amostra. FraudPct vai de 0 a 100.
type SampleParams struct {
	N        int
	FraudPct float64
	Seed     uint64
	Types    []string
}

// SampleResult traz a amostra e as contagens do universo elegível de onde ela
// saiu. Prevalence é EligibleFraud dividido por EligibleRows.
type SampleResult struct {
	Transactions   []domain.Transaction
	NFraud         int
	NLegit         int
	FraudShortfall bool
	EligibleRows   int64
	EligibleFraud  int64
	Prevalence     float64
}

func (p SampleParams) validate() error {
	switch {
	case p.N < 1:
		return fmt.Errorf("N precisa ser pelo menos 1, veio %d", p.N)
	case p.FraudPct < 0 || p.FraudPct > 100:
		return fmt.Errorf("FraudPct precisa estar entre 0 e 100, veio %v", p.FraudPct)
	case len(p.Types) == 0:
		return errors.New("Types não pode ser vazio")
	}
	return nil
}

// reservoir guarda uma amostra uniforme de tamanho k (algoritmo R).
type reservoir struct {
	k     int
	seen  int64
	items []domain.Transaction
	rng   *rand.Rand
}

func (r *reservoir) offer(tx domain.Transaction) {
	r.seen++
	if r.k == 0 {
		return
	}
	if len(r.items) < r.k {
		r.items = append(r.items, tx)
		return
	}
	if j := r.rng.Int64N(r.seen); j < int64(r.k) {
		r.items[j] = tx
	}
}

// Sample faz uma única passada na fonte e devolve a amostra estratificada.
func Sample(ctx context.Context, src TxSource, p SampleParams) (SampleResult, error) {
	if err := p.validate(); err != nil {
		return SampleResult{}, err
	}
	rng := rand.New(rand.NewPCG(p.Seed, p.Seed^0x9e3779b97f4a7c15))
	elegivel := make(map[string]bool, len(p.Types))
	for _, t := range p.Types {
		elegivel[t] = true
	}

	quero := int(math.Round(float64(p.N) * p.FraudPct / 100))
	fraudes := &reservoir{k: quero, rng: rng}
	// O reservatório de legítimas guarda até N, para poder completar a amostra
	// quando faltar fraude.
	legitimas := &reservoir{k: p.N, rng: rng}

	var res SampleResult
	err := src(ctx, func(tx domain.Transaction) error {
		if !elegivel[tx.Type] {
			return nil
		}
		res.EligibleRows++
		if tx.IsFraud {
			res.EligibleFraud++
			fraudes.offer(tx)
		} else {
			legitimas.offer(tx)
		}
		return nil
	})
	if err != nil {
		return SampleResult{}, err
	}
	if res.EligibleRows == 0 {
		return SampleResult{}, ErrEmptySample
	}

	res.NFraud = len(fraudes.items)
	res.FraudShortfall = res.NFraud < quero

	// O reservatório não é uma permutação uniforme, então embaralha antes de
	// cortar para completar só o que falta.
	rng.Shuffle(len(legitimas.items), func(i, j int) {
		legitimas.items[i], legitimas.items[j] = legitimas.items[j], legitimas.items[i]
	})
	res.NLegit = min(p.N-res.NFraud, len(legitimas.items))

	all := make([]domain.Transaction, 0, res.NFraud+res.NLegit)
	all = append(all, fraudes.items...)
	all = append(all, legitimas.items[:res.NLegit]...)
	slices.SortFunc(all, func(a, b domain.Transaction) int { return cmp.Compare(a.Seq, b.Seq) })

	res.Transactions = all
	res.Prevalence = float64(res.EligibleFraud) / float64(res.EligibleRows)
	return res, nil
}
