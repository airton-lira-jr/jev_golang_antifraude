package domain

import (
	"fmt"
	"slices"
	"strings"
)

// TransactionTypes são os cinco tipos do PaySim.
var TransactionTypes = []string{"PAYMENT", "TRANSFER", "CASH_OUT", "DEBIT", "CASH_IN"}

// ValidType diz se o tipo existe no PaySim (a comparação é exata).
func ValidType(t string) bool {
	return slices.Contains(TransactionTypes, t)
}

// Transaction é uma linha do CSV. Seq é o número da linha de dados, começando
// em 0, e funciona como a régua do tempo: menor Seq significa que chegou antes.
type Transaction struct {
	Seq            int64
	Step           int
	Type           string
	Amount         float64
	Orig           string
	OldBalanceOrig float64
	NewBalanceOrig float64
	Dest           string
	OldBalanceDest float64
	NewBalanceDest float64
	IsFraud        bool

	// Colunas opcionais do CSV (paymentMethod, channel e category). Ficam vazias
	// quando o arquivo não tem a coluna.
	PaymentMethod string
	Channel       string
	Category      string
}

// ID é o identificador que o PaySim não tem, derivado do Seq.
func (t Transaction) ID() string { return fmt.Sprintf("TX_%d", t.Seq) }

// BalanceDrop é quanto o saldo da origem caiu.
func (t Transaction) BalanceDrop() float64 { return t.OldBalanceOrig - t.NewBalanceOrig }

// DestIsMerchant diz se o destino é um comerciante (o nome começa com "M").
func (t Transaction) DestIsMerchant() bool { return strings.HasPrefix(t.Dest, "M") }
