package domain

// Result é uma linha do results.jsonl. Os campos de grafo vêm achatados pelo
// GraphFeatures embutido e somem do JSON quando a execução foi sem grafo.
type Result struct {
	TransactionID string   `json:"transaction_id"`
	Seq           int64    `json:"seq"`
	Step          int      `json:"step"`
	Type          string   `json:"type"`
	Orig          string   `json:"orig"`
	Dest          string   `json:"dest"`
	IsFraud       bool     `json:"is_fraud"`
	Noul          *float64 `json:"noul"`
	Neo4jMs       *float64 `json:"neo4j_ms"`
	JevMs         *float64 `json:"jev_ms"`
	Attempts      int      `json:"attempts"`
	InputTokens   int      `json:"input_tokens"`
	Error         string   `json:"error,omitempty"`

	Amount            float64 `json:"amount"`
	OrigBalanceBefore float64 `json:"orig_balance_before"`
	OrigBalanceAfter  float64 `json:"orig_balance_after"`
	DestBalanceBefore float64 `json:"dest_balance_before"`
	DestBalanceAfter  float64 `json:"dest_balance_after"`
	BalanceDrop       float64 `json:"balance_drop"`
	DestIsMerchant    bool    `json:"dest_is_merchant"`

	*GraphFeatures
}

// NewResult monta a parte fixa do resultado a partir da transação.
func NewResult(tx Transaction, gf *GraphFeatures) Result {
	return Result{
		TransactionID:     tx.ID(),
		Seq:               tx.Seq,
		Step:              tx.Step,
		Type:              tx.Type,
		Orig:              tx.Orig,
		Dest:              tx.Dest,
		IsFraud:           tx.IsFraud,
		Amount:            tx.Amount,
		OrigBalanceBefore: tx.OldBalanceOrig,
		OrigBalanceAfter:  tx.NewBalanceOrig,
		DestBalanceBefore: tx.OldBalanceDest,
		DestBalanceAfter:  tx.NewBalanceDest,
		BalanceDrop:       tx.BalanceDrop(),
		DestIsMerchant:    tx.DestIsMerchant(),
		GraphFeatures:     gf,
	}
}
