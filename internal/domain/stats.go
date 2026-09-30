package domain

// TypeStats são as contagens de um tipo de transação.
type TypeStats struct {
	Rows   int64 `json:"rows"`
	Frauds int64 `json:"frauds"`
}

// DatasetStats é o conteúdo do dataset_stats.json, gravado pelo seeder.
type DatasetStats struct {
	TotalRows int64                `json:"total_rows"`
	FraudRows int64                `json:"fraud_rows"`
	MaxStep   int                  `json:"max_step"`
	ByType    map[string]TypeStats `json:"by_type"`
}
