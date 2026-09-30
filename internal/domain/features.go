package domain

// GraphFeatures são as features de rede calculadas no Neo4j, sempre usando só
// o passado da transação. DestAccountAgeSteps é nil quando a conta nunca
// apareceu antes.
type GraphFeatures struct {
	FirstTransferBetweenPair bool `json:"first_transfer_between_pair"`
	DestInDegree             int  `json:"dest_in_degree"`
	DestFanInWindow          int  `json:"dest_fan_in_window"`
	DestFanOutWindow         int  `json:"dest_fan_out_window"`
	OrigFanOutWindow         int  `json:"orig_fan_out_window"`
	DestAccountAgeSteps      *int `json:"dest_account_age_steps"`
	CycleDetected            bool `json:"cycle_detected"`
}

// FeatureQuery diz para o Neo4j o que ele pode enxergar. Só entram arestas com
// seq menor que BeforeSeq e step menor ou igual a AtStep. Para uma transação
// hipotética "de agora", use BeforeSeq = math.MaxInt64.
type FeatureQuery struct {
	Orig         string
	Dest         string
	BeforeSeq    int64
	AtStep       int
	WindowHours  int
	CycleMaxHops int
}

// SubEdge é uma aresta do subgrafo mostrado na Aba 3. IsFraud serve só para
// pintar a aresta, nunca para calcular feature.
type SubEdge struct {
	ID      string  `json:"id"`
	Src     string  `json:"src"`
	Dst     string  `json:"dst"`
	Type    string  `json:"type"`
	Amount  float64 `json:"amount"`
	Step    int     `json:"step"`
	IsFraud bool    `json:"is_fraud"`
}

// Subgraph é a vizinhança de 1 salto da origem e do destino.
type Subgraph struct {
	Nodes []string  `json:"nodes"`
	Edges []SubEdge `json:"edges"`
}
