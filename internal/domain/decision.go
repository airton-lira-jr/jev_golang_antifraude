package domain

import (
	"encoding/json"
	"time"
)

// Decision é a resposta do modelo para a pergunta noul. Latency é o tempo só
// da tentativa que deu certo. Raw guarda o corpo bruto da resposta.
type Decision struct {
	Noul        float64
	InputTokens int
	Attempts    int
	Latency     time.Duration
	Raw         json.RawMessage
}

// ResultsSink recebe um Result por transação processada.
type ResultsSink interface {
	Write(Result) error
	Close() error
}
