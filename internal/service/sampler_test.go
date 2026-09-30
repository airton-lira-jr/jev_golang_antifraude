package service

import (
	"context"
	"errors"
	"slices"
	"testing"

	"fraud-detector-jev/internal/domain"
)

// geraTransacoes cria linhas sintéticas. A cada fraudeACada linhas elegíveis
// uma é fraude. Os tipos se alternam entre TRANSFER, CASH_OUT e PAYMENT.
func geraTransacoes(total, fraudeACada int) []domain.Transaction {
	tipos := []string{"TRANSFER", "CASH_OUT", "PAYMENT"}
	var txs []domain.Transaction
	for i := range total {
		tx := domain.Transaction{Seq: int64(i), Step: i / 10, Type: tipos[i%3]}
		if fraudeACada > 0 && tx.Type != "PAYMENT" && i%fraudeACada == 0 {
			tx.IsFraud = true
		}
		txs = append(txs, tx)
	}
	return txs
}

func fonte(txs []domain.Transaction) TxSource {
	return func(ctx context.Context, fn func(domain.Transaction) error) error {
		for _, tx := range txs {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := fn(tx); err != nil {
				return err
			}
		}
		return nil
	}
}

var tiposPadrao = []string{"TRANSFER", "CASH_OUT"}

func contar(txs []domain.Transaction) (fraudes, legitimas int) {
	for _, tx := range txs {
		if tx.IsFraud {
			fraudes++
		} else {
			legitimas++
		}
	}
	return
}

func TestSampleRespeitaProporcaoETipos(t *testing.T) {
	txs := geraTransacoes(3000, 20)
	res, err := Sample(context.Background(), fonte(txs), SampleParams{N: 100, FraudPct: 10, Seed: 42, Types: tiposPadrao})
	if err != nil {
		t.Fatal(err)
	}
	fraudes, legitimas := contar(res.Transactions)
	if fraudes != 10 || legitimas != 90 || res.NFraud != 10 || res.NLegit != 90 {
		t.Errorf("fraudes=%d legítimas=%d (res %d/%d)", fraudes, legitimas, res.NFraud, res.NLegit)
	}
	for _, tx := range res.Transactions {
		if tx.Type == "PAYMENT" {
			t.Fatalf("tipo não elegível entrou: %+v", tx)
		}
	}
	if !slices.IsSortedFunc(res.Transactions, func(a, b domain.Transaction) int { return int(a.Seq - b.Seq) }) {
		t.Error("amostra deveria vir ordenada por Seq")
	}
	if res.FraudShortfall {
		t.Error("não faltou fraude")
	}
}

func TestSampleContaUniversoEPrevalenciaSoDosTiposElegiveis(t *testing.T) {
	txs := geraTransacoes(3000, 20)
	res, err := Sample(context.Background(), fonte(txs), SampleParams{N: 50, FraudPct: 10, Seed: 1, Types: tiposPadrao})
	if err != nil {
		t.Fatal(err)
	}
	// 2000 linhas são TRANSFER ou CASH_OUT, e as fraudes são as posições
	// multiplas de 20 que não caem em PAYMENT.
	var esperadoLinhas, esperadoFraudes int64
	for _, tx := range txs {
		if tx.Type != "PAYMENT" {
			esperadoLinhas++
			if tx.IsFraud {
				esperadoFraudes++
			}
		}
	}
	if res.EligibleRows != esperadoLinhas || res.EligibleFraud != esperadoFraudes {
		t.Errorf("universo %d/%d, esperado %d/%d", res.EligibleRows, res.EligibleFraud, esperadoLinhas, esperadoFraudes)
	}
	want := float64(esperadoFraudes) / float64(esperadoLinhas)
	if res.Prevalence != want {
		t.Errorf("prevalência %v, esperada %v", res.Prevalence, want)
	}
}

func TestSampleEhDeterministicaPorSeed(t *testing.T) {
	txs := geraTransacoes(5000, 15)
	pegar := func(seed uint64) []int64 {
		res, err := Sample(context.Background(), fonte(txs), SampleParams{N: 200, FraudPct: 20, Seed: seed, Types: tiposPadrao})
		if err != nil {
			t.Fatal(err)
		}
		var seqs []int64
		for _, tx := range res.Transactions {
			seqs = append(seqs, tx.Seq)
		}
		return seqs
	}
	if !slices.Equal(pegar(7), pegar(7)) {
		t.Error("mesma seed deveria dar a mesma amostra")
	}
	if slices.Equal(pegar(7), pegar(8)) {
		t.Error("seeds diferentes deveriam dar amostras diferentes")
	}
}

func TestSampleUsaTodasAsFraudesQuandoFaltaEComplementaComLegitimas(t *testing.T) {
	txs := geraTransacoes(3000, 400) // poucas fraudes
	fraudesExistentes, _ := contar(slices.DeleteFunc(slices.Clone(txs), func(tx domain.Transaction) bool { return tx.Type == "PAYMENT" }))
	res, err := Sample(context.Background(), fonte(txs), SampleParams{N: 100, FraudPct: 50, Seed: 3, Types: tiposPadrao})
	if err != nil {
		t.Fatal(err)
	}
	fraudes, legitimas := contar(res.Transactions)
	if fraudes != fraudesExistentes || !res.FraudShortfall {
		t.Errorf("fraudes=%d (existem %d) shortfall=%v", fraudes, fraudesExistentes, res.FraudShortfall)
	}
	if fraudes+legitimas != 100 {
		t.Errorf("total = %d, deveria completar 100 com legítimas", fraudes+legitimas)
	}
}

func TestSampleFraudPctZeroECemENMaiorQueOUniverso(t *testing.T) {
	txs := geraTransacoes(300, 10)
	casos := []struct {
		nome     string
		p        SampleParams
		fraudes  func(int) bool
		total    func(int) bool
		shortage bool
	}{
		{"0 por cento", SampleParams{N: 50, FraudPct: 0, Seed: 1, Types: tiposPadrao}, func(f int) bool { return f == 0 }, func(n int) bool { return n == 50 }, false},
		{"100 por cento", SampleParams{N: 50, FraudPct: 100, Seed: 1, Types: tiposPadrao}, func(f int) bool { return f > 0 && f <= 50 }, func(n int) bool { return n == 50 }, false},
		{"N maior que o universo", SampleParams{N: 100000, FraudPct: 10, Seed: 1, Types: tiposPadrao}, func(f int) bool { return f > 0 }, func(n int) bool { return n == 200 }, true},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			res, err := Sample(context.Background(), fonte(txs), c.p)
			if err != nil {
				t.Fatal(err)
			}
			fraudes, _ := contar(res.Transactions)
			if !c.fraudes(fraudes) || !c.total(len(res.Transactions)) {
				t.Errorf("fraudes=%d total=%d", fraudes, len(res.Transactions))
			}
			vistas := map[int64]bool{}
			for _, tx := range res.Transactions {
				if vistas[tx.Seq] {
					t.Fatalf("transação repetida: %d", tx.Seq)
				}
				vistas[tx.Seq] = true
			}
		})
	}
}

func TestSampleUniversoVazioDaErro(t *testing.T) {
	txs := geraTransacoes(30, 5)
	_, err := Sample(context.Background(), fonte(txs), SampleParams{N: 10, FraudPct: 10, Seed: 1, Types: []string{"DEBIT"}})
	if !errors.Is(err, ErrEmptySample) {
		t.Fatalf("esperava ErrEmptySample, veio %v", err)
	}
}

func TestSampleValidaParametros(t *testing.T) {
	ruins := []SampleParams{
		{N: 0, FraudPct: 10, Types: tiposPadrao},
		{N: 10, FraudPct: -1, Types: tiposPadrao},
		{N: 10, FraudPct: 101, Types: tiposPadrao},
		{N: 10, FraudPct: 10},
	}
	for _, p := range ruins {
		if _, err := Sample(context.Background(), fonte(nil), p); err == nil {
			t.Errorf("esperava erro para %+v", p)
		}
	}
}

func TestSamplePropagaErroDaFonteECancelamento(t *testing.T) {
	falha := errors.New("disco pegou fogo")
	src := TxSource(func(ctx context.Context, fn func(domain.Transaction) error) error { return falha })
	if _, err := Sample(context.Background(), src, SampleParams{N: 10, FraudPct: 10, Types: tiposPadrao}); !errors.Is(err, falha) {
		t.Errorf("esperava o erro da fonte, veio %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Sample(ctx, fonte(geraTransacoes(10, 2)), SampleParams{N: 10, FraudPct: 10, Types: tiposPadrao}); !errors.Is(err, context.Canceled) {
		t.Errorf("esperava context.Canceled, veio %v", err)
	}
}
