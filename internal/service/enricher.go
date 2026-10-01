package service

import (
	"encoding/json"

	"fraud-detector-jev/internal/domain"
)

// State é o que o modelo enxerga. Os nomes das contas, o gabarito e o seq não
// vão para o modelo de propósito.
type State struct {
	TransactionID     string  `json:"transaction_id"`
	Type              string  `json:"type"`
	Amount            float64 `json:"amount"`
	TimeStep          int     `json:"time_step"`
	OrigBalanceBefore float64 `json:"orig_balance_before"`
	OrigBalanceAfter  float64 `json:"orig_balance_after"`
	DestBalanceBefore float64 `json:"dest_balance_before"`
	DestBalanceAfter  float64 `json:"dest_balance_after"`
	BalanceDrop       float64 `json:"balance_drop"`
	DestIsMerchant    bool    `json:"dest_is_merchant"`
	// As três vêm das colunas opcionais do CSV e só existem quando o arquivo as tem.
	// Vão nas duas variantes (com e sem grafo), para o grafo seguir como a única diferença.
	PaymentMethod string                `json:"payment_method,omitempty"`
	Channel       string                `json:"channel,omitempty"`
	Category      string                `json:"category,omitempty"`
	GraphFeatures *domain.GraphFeatures `json:"graph_features,omitempty"`
}

// BuildState monta o state. Com gf nil (execução sem grafo) o bloco
// graph_features não existe.
func BuildState(tx domain.Transaction, gf *domain.GraphFeatures) State {
	return State{
		TransactionID:     tx.ID(),
		Type:              tx.Type,
		Amount:            tx.Amount,
		TimeStep:          tx.Step,
		OrigBalanceBefore: tx.OldBalanceOrig,
		OrigBalanceAfter:  tx.NewBalanceOrig,
		DestBalanceBefore: tx.OldBalanceDest,
		DestBalanceAfter:  tx.NewBalanceDest,
		BalanceDrop:       tx.BalanceDrop(),
		DestIsMerchant:    tx.DestIsMerchant(),
		PaymentMethod:     tx.PaymentMethod,
		Channel:           tx.Channel,
		Category:          tx.Category,
		GraphFeatures:     gf,
	}
}

// JSON devolve o state como string JSON, que é o formato do campo state.
func (s State) JSON() (string, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// FeatureQueryFor monta a consulta de replay: a transação só enxerga o que veio
// antes dela.
func FeatureQueryFor(tx domain.Transaction, windowHours, cycleMaxHops int) domain.FeatureQuery {
	return domain.FeatureQuery{
		Orig:         tx.Orig,
		Dest:         tx.Dest,
		BeforeSeq:    tx.Seq,
		AtStep:       tx.Step,
		WindowHours:  windowHours,
		CycleMaxHops: cycleMaxHops,
	}
}
