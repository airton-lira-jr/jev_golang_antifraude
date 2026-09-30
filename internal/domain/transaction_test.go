package domain

import "testing"

func TestTransactionDerivados(t *testing.T) {
	tx := Transaction{Seq: 1029, OldBalanceOrig: 350000, NewBalanceOrig: 0, Dest: "C123"}
	if tx.ID() != "TX_1029" {
		t.Errorf("ID = %q", tx.ID())
	}
	if tx.BalanceDrop() != 350000 {
		t.Errorf("BalanceDrop = %v", tx.BalanceDrop())
	}
	if tx.DestIsMerchant() {
		t.Error("C123 não é comerciante")
	}
	tx.Dest = "M999"
	if !tx.DestIsMerchant() {
		t.Error("M999 é comerciante")
	}
}

func TestValidType(t *testing.T) {
	for _, ok := range []string{"PAYMENT", "TRANSFER", "CASH_OUT", "DEBIT", "CASH_IN"} {
		if !ValidType(ok) {
			t.Errorf("%s deveria ser válido", ok)
		}
	}
	for _, ruim := range []string{"", "transfer", "REFUND"} {
		if ValidType(ruim) {
			t.Errorf("%q não deveria ser válido", ruim)
		}
	}
}
