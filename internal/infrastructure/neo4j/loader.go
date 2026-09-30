package neo4j

import (
	"context"
	"fmt"

	"fraud-detector-jev/internal/domain"
	"fraud-detector-jev/internal/infrastructure/csv"
)

// LoadOptions configura a carga. Progress é opcional e recebe a fase
// ("nodes" ou "edges") e quantas linhas do CSV já foram tratadas.
type LoadOptions struct {
	CSVPath   string
	BatchSize int
	Progress  func(phase string, done int64)
}

// LoadReport resume o que ficou no banco depois da carga.
type LoadReport struct {
	Nodes   int64
	Rels    int64
	Stats   domain.DatasetStats
	Resumed bool
}

// Load carrega o CSV no grafo. Pode ser executada de novo depois de uma queda:
// se já existirem arestas, pula a criação das contas (que só começa a criar
// arestas depois de terminar) e continua do maior seq gravado. Cada lote é uma
// transação, então nunca sobra meia aresta.
func (c *Client) Load(ctx context.Context, o LoadOptions) (LoadReport, error) {
	if o.BatchSize <= 0 {
		o.BatchSize = 10000
	}
	progress := o.Progress
	if progress == nil {
		progress = func(string, int64) {}
	}

	if _, err := c.write(ctx, createConstraint, nil); err != nil {
		return LoadReport{}, fmt.Errorf("criando a constraint: %w", err)
	}
	maxSeq, err := c.maxLoadedSeq(ctx)
	if err != nil {
		return LoadReport{}, err
	}
	report := LoadReport{Resumed: maxSeq >= 0}

	if maxSeq < 0 {
		if err := c.loadNodes(ctx, o, progress); err != nil {
			return report, err
		}
	}
	stats, err := c.loadEdges(ctx, o, maxSeq, progress)
	if err != nil {
		return report, err
	}
	report.Stats = stats

	for _, q := range []string{createIndexSeq, createIndexStep, awaitIndexes} {
		if _, err := c.write(ctx, q, nil); err != nil {
			return report, fmt.Errorf("criando os índices: %w", err)
		}
	}

	report.Nodes, report.Rels, err = c.Counts(ctx)
	if err != nil {
		return report, err
	}
	if report.Rels != stats.TotalRows {
		return report, fmt.Errorf("o grafo ficou com %d arestas mas o CSV tem %d linhas", report.Rels, stats.TotalRows)
	}
	return report, nil
}

// Counts devolve quantas contas e quantas arestas existem no grafo.
func (c *Client) Counts(ctx context.Context) (nodes, rels int64, err error) {
	for query, dest := range map[string]*int64{countNodes: &nodes, countRels: &rels} {
		res, qerr := c.readLong(ctx, query)
		if qerr != nil {
			return 0, 0, fmt.Errorf("contando o grafo: %w", qerr)
		}
		n, ierr := recordInt(res.Records[0], "n")
		if ierr != nil {
			return 0, 0, ierr
		}
		*dest = n
	}
	return nodes, rels, nil
}

// maxLoadedSeq devolve o maior seq já gravado, ou -1 se não há arestas.
func (c *Client) maxLoadedSeq(ctx context.Context) (int64, error) {
	res, err := c.readLong(ctx, maxSeqQuery)
	if err != nil {
		return 0, fmt.Errorf("procurando o ponto de retomada: %w", err)
	}
	v, _ := res.Records[0].Get("maxSeq")
	if v == nil {
		return -1, nil
	}
	n, ok := v.(int64)
	if !ok {
		return 0, fmt.Errorf("maxSeq com tipo inesperado %T", v)
	}
	return n, nil
}

func (c *Client) loadNodes(ctx context.Context, o LoadOptions, progress func(string, int64)) error {
	batch := make(map[string]struct{}, o.BatchSize*2)
	var rows int64

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		ids := make([]string, 0, len(batch))
		for id := range batch {
			ids = append(ids, id)
		}
		if _, err := c.write(ctx, mergeAccounts, map[string]any{"ids": ids}); err != nil {
			return fmt.Errorf("criando contas: %w", err)
		}
		clear(batch)
		return nil
	}

	err := csv.Stream(ctx, o.CSVPath, func(tx domain.Transaction) error {
		batch[tx.Orig] = struct{}{}
		batch[tx.Dest] = struct{}{}
		rows++
		if len(batch) >= o.BatchSize {
			if err := flush(); err != nil {
				return err
			}
			progress("nodes", rows)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := flush(); err != nil {
		return err
	}
	progress("nodes", rows)
	return nil
}

// loadEdges lê o CSV inteiro para calcular as estatísticas (mesmo as linhas
// que já estavam gravadas) e grava só as arestas com seq maior que maxSeq.
func (c *Client) loadEdges(ctx context.Context, o LoadOptions, maxSeq int64, progress func(string, int64)) (domain.DatasetStats, error) {
	stats := domain.DatasetStats{ByType: map[string]domain.TypeStats{}}
	rows := make([]map[string]any, 0, o.BatchSize)

	flush := func() error {
		if len(rows) == 0 {
			return nil
		}
		if _, err := c.write(ctx, createEdges, map[string]any{"rows": rows}); err != nil {
			return fmt.Errorf("criando arestas: %w", err)
		}
		rows = rows[:0]
		return nil
	}

	err := csv.Stream(ctx, o.CSVPath, func(tx domain.Transaction) error {
		stats.TotalRows++
		ts := stats.ByType[tx.Type]
		ts.Rows++
		if tx.IsFraud {
			stats.FraudRows++
			ts.Frauds++
		}
		stats.ByType[tx.Type] = ts
		if tx.Step > stats.MaxStep {
			stats.MaxStep = tx.Step
		}

		if tx.Seq <= maxSeq {
			return nil
		}
		rows = append(rows, map[string]any{
			"seq": tx.Seq, "step": tx.Step, "type": tx.Type, "amount": tx.Amount,
			"isFraud": tx.IsFraud, "orig": tx.Orig, "dest": tx.Dest,
		})
		if len(rows) >= o.BatchSize {
			if err := flush(); err != nil {
				return err
			}
			progress("edges", tx.Seq+1)
		}
		return nil
	})
	if err != nil {
		return stats, err
	}
	if err := flush(); err != nil {
		return stats, err
	}
	progress("edges", stats.TotalRows)
	return stats, nil
}
