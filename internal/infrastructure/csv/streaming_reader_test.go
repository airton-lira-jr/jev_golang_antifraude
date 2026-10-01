package csv

import (
	"context"
	"errors"
	"strings"
	"testing"

	"fraud-detector-jev/internal/domain"
)

const cabecalho = "step,type,amount,nameOrig,oldbalanceOrg,newbalanceOrig,nameDest,oldbalanceDest,newbalanceDest,isFraud,isFlaggedFraud\n"

func coletar(t *testing.T, conteudo string) ([]domain.Transaction, error) {
	t.Helper()
	var txs []domain.Transaction
	err := StreamReader(context.Background(), strings.NewReader(conteudo), func(tx domain.Transaction) error {
		txs = append(txs, tx)
		return nil
	})
	return txs, err
}

func TestStreamLeTransacoesEmOrdem(t *testing.T) {
	txs, err := coletar(t, cabecalho+
		"1,TRANSFER,100.5,C1,500.0,399.5,C2,0.0,100.5,0,0\n"+
		"1,CASH_OUT,20,C3,20,0,M9,5,25,1,0\n"+
		"2,PAYMENT,7.25,C4,10,2.75,M8,0,0,0,0\n")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(txs) != 3 {
		t.Fatalf("esperava 3 transações, vieram %d", len(txs))
	}
	primeira := txs[0]
	if primeira.Seq != 0 || primeira.Step != 1 || primeira.Type != "TRANSFER" || primeira.Amount != 100.5 ||
		primeira.Orig != "C1" || primeira.OldBalanceOrig != 500 || primeira.NewBalanceOrig != 399.5 ||
		primeira.Dest != "C2" || primeira.OldBalanceDest != 0 || primeira.NewBalanceDest != 100.5 || primeira.IsFraud {
		t.Errorf("primeira transação errada: %+v", primeira)
	}
	if txs[1].Seq != 1 || !txs[1].IsFraud || txs[2].Seq != 2 || txs[2].Step != 2 {
		t.Errorf("seq ou fraude errados: %+v %+v", txs[1], txs[2])
	}
}

func TestStreamLeAsColunasOpcionaisPeloNome(t *testing.T) {
	const comExtras = "step,type,amount,nameOrig,oldbalanceOrg,newbalanceOrig,nameDest,oldbalanceDest,newbalanceDest,isFraud,isFlaggedFraud,paymentMethod,channel,category\n"
	txs, err := coletar(t, comExtras+
		"1,TRANSFER,100.5,C1,500.0,399.5,C2,0.0,100.5,0,0,PIX,MOBILE_APP,FOOD\n"+
		"2,CASH_OUT,20,C3,20,0,M9,5,25,1,0,BOLETO,ATM,BILLS\n")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if txs[0].PaymentMethod != "PIX" || txs[0].Channel != "MOBILE_APP" || txs[0].Category != "FOOD" {
		t.Errorf("colunas opcionais da primeira linha erradas: %+v", txs[0])
	}
	if txs[1].PaymentMethod != "BOLETO" || txs[1].Channel != "ATM" || txs[1].Category != "BILLS" {
		t.Errorf("colunas opcionais da segunda linha erradas: %+v", txs[1])
	}

	// A ordem das colunas novas e a presença do isFlaggedFraud não importam.
	const embaralhado = "step,type,amount,nameOrig,oldbalanceOrg,newbalanceOrig,nameDest,oldbalanceDest,newbalanceDest,isFraud,category,paymentMethod\n"
	txs, err = coletar(t, embaralhado+"1,TRANSFER,10,C1,10,0,C2,0,10,0,HEALTH,DEBIT_CARD\n")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if txs[0].PaymentMethod != "DEBIT_CARD" || txs[0].Category != "HEALTH" || txs[0].Channel != "" {
		t.Errorf("colunas fora de ordem erradas: %+v", txs[0])
	}
}

func TestStreamSemAsColunasOpcionaisDeixaOsCamposVazios(t *testing.T) {
	txs, err := coletar(t, cabecalho+"1,TRANSFER,10,C1,10,0,C2,0,10,0,0\n")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if txs[0].PaymentMethod != "" || txs[0].Channel != "" || txs[0].Category != "" {
		t.Errorf("sem as colunas os campos deveriam ficar vazios: %+v", txs[0])
	}
}

func TestStreamToleraCRLFELinhaEmBrancoNoFim(t *testing.T) {
	conteudo := strings.ReplaceAll(cabecalho+
		"1,TRANSFER,10,C1,10,0,C2,0,10,0,0\n"+
		"1,TRANSFER,20,C1,10,0,C2,0,10,1,0\n"+
		"\n", "\n", "\r\n")
	txs, err := coletar(t, conteudo)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(txs) != 2 {
		t.Fatalf("esperava 2 transações e nenhuma fantasma, vieram %d", len(txs))
	}
}

func TestStreamRejeitaCabecalhoErrado(t *testing.T) {
	_, err := coletar(t, "a,b,c\n1,2,3\n")
	if err == nil || !strings.Contains(err.Error(), "cabeçalho") {
		t.Fatalf("esperava erro de cabeçalho, veio %v", err)
	}
}

func TestStreamRejeitaLinhaComCampoFaltando(t *testing.T) {
	_, err := coletar(t, cabecalho+"1,TRANSFER,10,C1,10,0,C2,0,10\n")
	if err == nil || !strings.Contains(err.Error(), "linha 2") {
		t.Fatalf("esperava erro citando a linha 2, veio %v", err)
	}
}

func TestStreamRejeitaNumeroInvalido(t *testing.T) {
	_, err := coletar(t, cabecalho+"1,TRANSFER,dez,C1,10,0,C2,0,10,0,0\n")
	if err == nil || !strings.Contains(err.Error(), "amount") || !strings.Contains(err.Error(), "linha 2") {
		t.Fatalf("esperava erro citando amount e a linha 2, veio %v", err)
	}
	_, err = coletar(t, cabecalho+"1,TRANSFER,10,C1,10,0,C2,0,10,talvez,0\n")
	if err == nil || !strings.Contains(err.Error(), "isFraud") {
		t.Fatalf("esperava erro citando isFraud, veio %v", err)
	}
}

func TestStreamDetectaArquivoForaDeOrdem(t *testing.T) {
	_, err := coletar(t, cabecalho+
		"5,TRANSFER,10,C1,10,0,C2,0,10,0,0\n"+
		"4,TRANSFER,10,C1,10,0,C2,0,10,0,0\n")
	if !errors.Is(err, ErrUnsortedSteps) {
		t.Fatalf("esperava ErrUnsortedSteps, veio %v", err)
	}
	if !strings.Contains(err.Error(), "linha 3") {
		t.Errorf("erro deveria citar a linha 3: %v", err)
	}
}

func TestStreamPropagaErroDoCallbackEPara(t *testing.T) {
	falha := errors.New("parei")
	chamadas := 0
	err := StreamReader(context.Background(), strings.NewReader(cabecalho+
		"1,TRANSFER,10,C1,10,0,C2,0,10,0,0\n"+
		"1,TRANSFER,10,C1,10,0,C2,0,10,0,0\n"), func(domain.Transaction) error {
		chamadas++
		return falha
	})
	if !errors.Is(err, falha) || chamadas != 1 {
		t.Fatalf("err=%v chamadas=%d", err, chamadas)
	}
}

func TestStreamRespeitaContextoCancelado(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := StreamReader(ctx, strings.NewReader(cabecalho+"1,TRANSFER,10,C1,10,0,C2,0,10,0,0\n"), func(domain.Transaction) error {
		t.Fatal("não deveria chamar o callback")
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("esperava context.Canceled, veio %v", err)
	}
}

func TestStreamAbreArquivoInexistente(t *testing.T) {
	err := Stream(context.Background(), "/nao/existe.csv", func(domain.Transaction) error { return nil })
	if err == nil {
		t.Fatal("esperava erro")
	}
}
