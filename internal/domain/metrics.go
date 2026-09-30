package domain

import (
	"math"
	"slices"
)

// Confusion é a matriz de confusão. Fraude é a classe positiva.
type Confusion struct {
	TP int `json:"tp"`
	FP int `json:"fp"`
	TN int `json:"tn"`
	FN int `json:"fn"`
}

// Counts é a matriz mais a quantidade de transações que deram erro.
type Counts struct {
	Confusion
	Errors int `json:"errors"`
}

// Add registra uma transação na matriz.
func (c *Confusion) Add(isFraud, predictedFraud bool) {
	switch {
	case isFraud && predictedFraud:
		c.TP++
	case isFraud && !predictedFraud:
		c.FN++
	case !isFraud && predictedFraud:
		c.FP++
	default:
		c.TN++
	}
}

func (c Confusion) Total() int { return c.TP + c.FP + c.TN + c.FN }

// Todas as razões devolvem 0 quando o denominador é 0. O Python usa a mesma
// convenção (veja testdata/metrics_golden.json).
func ratio(num, den int) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func (c Confusion) Accuracy() float64  { return ratio(c.TP+c.TN, c.Total()) }
func (c Confusion) Precision() float64 { return ratio(c.TP, c.TP+c.FP) }
func (c Confusion) Recall() float64    { return ratio(c.TP, c.TP+c.FN) }
func (c Confusion) FPR() float64       { return ratio(c.FP, c.FP+c.TN) }

func (c Confusion) F1() float64 {
	p, r := c.Precision(), c.Recall()
	if p+r == 0 {
		return 0
	}
	return 2 * p * r / (p + r)
}

// PrecisionAtPrevalence reprojeta a precisão para outra proporção de fraude.
// TPR e FPR não mudam com a estratificação, então dá para trocar só o pi.
func PrecisionAtPrevalence(tpr, fpr, prevalence float64) float64 {
	den := tpr*prevalence + fpr*(1-prevalence)
	if den == 0 {
		return 0
	}
	return tpr * prevalence / den
}

// Metrics são as métricas de classificação de uma execução.
type Metrics struct {
	Accuracy              float64 `json:"accuracy"`
	Precision             float64 `json:"precision"`
	Recall                float64 `json:"recall"`
	F1                    float64 `json:"f1"`
	FPR                   float64 `json:"fpr"`
	PrecisionAtPrevalence float64 `json:"precision_at_prevalence"`
}

// ComputeMetrics calcula tudo a partir da matriz. prevalence é a proporção
// real de fraude no universo elegível.
func ComputeMetrics(c Confusion, prevalence float64) Metrics {
	return Metrics{
		Accuracy:              c.Accuracy(),
		Precision:             c.Precision(),
		Recall:                c.Recall(),
		F1:                    c.F1(),
		FPR:                   c.FPR(),
		PrecisionAtPrevalence: PrecisionAtPrevalence(c.Recall(), c.FPR(), prevalence),
	}
}

// Decide aplica o limiar. No limiar exato conta como fraude.
func Decide(noul, threshold float64) bool { return noul >= threshold }

// LatencyStats são os percentis de latência em milissegundos.
type LatencyStats struct {
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
}

// percentile usa o método do posto mais próximo sobre valores já ordenados.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	idx := min(max(rank-1, 0), len(sorted)-1)
	return sorted[idx]
}

// ComputeLatency calcula P50, P95 e P99 de forma exata, sem mexer na entrada.
func ComputeLatency(values []float64) LatencyStats {
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	return LatencyStats{
		P50: percentile(sorted, 50),
		P95: percentile(sorted, 95),
		P99: percentile(sorted, 99),
	}
}
