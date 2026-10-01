package csv

import (
	"bufio"
	"context"
	stdcsv "encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"fraud-detector-jev/internal/domain"
)

// ErrUnsortedSteps aparece quando o step diminui no meio do arquivo. O seq só
// serve como régua do tempo se o CSV estiver ordenado por step.
var ErrUnsortedSteps = errors.New("csv fora de ordem, o step diminuiu")

// As dez primeiras colunas são obrigatórias e precisam estar nesta ordem. A
// coluna isFlaggedFraud, se existir, é ignorada.
var colunasObrigatorias = []string{
	"step", "type", "amount", "nameOrig", "oldbalanceOrg", "newbalanceOrig",
	"nameDest", "oldbalanceDest", "newbalanceDest", "isFraud",
}

// Stream abre o arquivo e chama fn para cada transação, na ordem do arquivo.
func Stream(ctx context.Context, path string, fn func(domain.Transaction) error) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("abrindo o CSV: %w", err)
	}
	defer f.Close()
	return StreamReader(ctx, f, fn)
}

// StreamReader é o Stream sobre qualquer io.Reader. Lê uma linha por vez, então
// a memória não cresce com o tamanho do arquivo.
func StreamReader(ctx context.Context, r io.Reader, fn func(domain.Transaction) error) error {
	cr := stdcsv.NewReader(bufio.NewReaderSize(r, 1<<20))
	cr.ReuseRecord = true

	header, err := cr.Read()
	if err != nil {
		return fmt.Errorf("lendo cabeçalho: %w", err)
	}
	if err := conferirCabecalho(header); err != nil {
		return err
	}
	cr.FieldsPerRecord = len(header)
	extras := acharOpcionais(header)

	var seq int64
	lastStep := -1
	for {
		if seq%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			var pe *stdcsv.ParseError
			if errors.As(err, &pe) {
				return fmt.Errorf("linha %d: %w", pe.Line, err)
			}
			return fmt.Errorf("lendo o CSV: %w", err)
		}
		line, _ := cr.FieldPos(0)

		tx, err := parseRecord(seq, rec, extras)
		if err != nil {
			return fmt.Errorf("linha %d: %w", line, err)
		}
		if tx.Step < lastStep {
			return fmt.Errorf("linha %d: %w (step %d depois de %d)", line, ErrUnsortedSteps, tx.Step, lastStep)
		}
		lastStep = tx.Step

		if err := fn(tx); err != nil {
			return err
		}
		seq++
	}
}

func conferirCabecalho(header []string) error {
	if len(header) < len(colunasObrigatorias) {
		return fmt.Errorf("cabeçalho com %d colunas, esperava pelo menos %d", len(header), len(colunasObrigatorias))
	}
	for i, nome := range colunasObrigatorias {
		if header[i] != nome {
			return fmt.Errorf("cabeçalho errado na coluna %d: esperava %q e veio %q", i+1, nome, header[i])
		}
	}
	return nil
}

// colunasOpcionais guarda a posição de cada coluna opcional no cabeçalho, ou -1
// quando o arquivo não tem a coluna.
type colunasOpcionais struct {
	paymentMethod, channel, category int
}

// acharOpcionais procura paymentMethod, channel e category pelo nome, depois das dez
// obrigatórias. A ordem delas no arquivo não importa.
func acharOpcionais(header []string) colunasOpcionais {
	posicao := func(nome string) int {
		for i := len(colunasObrigatorias); i < len(header); i++ {
			if header[i] == nome {
				return i
			}
		}
		return -1
	}
	return colunasOpcionais{
		paymentMethod: posicao("paymentMethod"),
		channel:       posicao("channel"),
		category:      posicao("category"),
	}
}

// campo devolve o valor da coluna, ou vazio quando o arquivo não tem a coluna.
func campo(rec []string, posicao int) string {
	if posicao < 0 {
		return ""
	}
	return rec[posicao]
}

func parseRecord(seq int64, rec []string, extras colunasOpcionais) (domain.Transaction, error) {
	tx := domain.Transaction{
		Seq: seq, Type: rec[1], Orig: rec[3], Dest: rec[6],
		PaymentMethod: campo(rec, extras.paymentMethod),
		Channel:       campo(rec, extras.channel),
		Category:      campo(rec, extras.category),
	}

	step, err := strconv.Atoi(rec[0])
	if err != nil {
		return tx, fmt.Errorf("campo step inválido %q", rec[0])
	}
	tx.Step = step

	if tx.Amount, err = parseFloat("amount", rec[2]); err != nil {
		return tx, err
	}
	if tx.OldBalanceOrig, err = parseFloat("oldbalanceOrg", rec[4]); err != nil {
		return tx, err
	}
	if tx.NewBalanceOrig, err = parseFloat("newbalanceOrig", rec[5]); err != nil {
		return tx, err
	}
	if tx.OldBalanceDest, err = parseFloat("oldbalanceDest", rec[7]); err != nil {
		return tx, err
	}
	if tx.NewBalanceDest, err = parseFloat("newbalanceDest", rec[8]); err != nil {
		return tx, err
	}

	switch rec[9] {
	case "0":
		tx.IsFraud = false
	case "1":
		tx.IsFraud = true
	default:
		return tx, fmt.Errorf("campo isFraud inválido %q", rec[9])
	}
	return tx, nil
}

func parseFloat(nome, bruto string) (float64, error) {
	v, err := strconv.ParseFloat(bruto, 64)
	if err != nil {
		return 0, fmt.Errorf("campo %s inválido %q", nome, bruto)
	}
	return v, nil
}
