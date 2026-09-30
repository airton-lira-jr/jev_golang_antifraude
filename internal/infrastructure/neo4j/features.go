package neo4j

import (
	"context"
	"fmt"

	neo "github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"fraud-detector-jev/internal/domain"
)

// Features calcula as features de grafo de uma transação, olhando só o passado
// dela (veja domain.FeatureQuery). Implementa service.GraphFeatureSource.
func (c *Client) Features(ctx context.Context, q domain.FeatureQuery) (domain.GraphFeatures, error) {
	query, err := FeaturesQuery(q.CycleMaxHops)
	if err != nil {
		return domain.GraphFeatures{}, err
	}
	res, err := c.read(ctx, query, map[string]any{
		"orig":      q.Orig,
		"dest":      q.Dest,
		"beforeSeq": q.BeforeSeq,
		"atStep":    q.AtStep,
		"window":    q.WindowHours,
	})
	if err != nil {
		return domain.GraphFeatures{}, fmt.Errorf("consultando features no Neo4j: %w", err)
	}
	if len(res.Records) != 1 {
		return domain.GraphFeatures{}, fmt.Errorf("a query de features devolveu %d linhas, esperava 1", len(res.Records))
	}
	return decodeFeatures(res.Records[0], q.AtStep)
}

func decodeFeatures(rec *neo.Record, atStep int) (domain.GraphFeatures, error) {
	var gf domain.GraphFeatures
	ints := []struct {
		key  string
		dest *int
	}{
		{"destInDegree", &gf.DestInDegree},
		{"destFanIn", &gf.DestFanInWindow},
		{"destFanOut", &gf.DestFanOutWindow},
		{"origFanOut", &gf.OrigFanOutWindow},
	}
	for _, f := range ints {
		n, err := recordInt(rec, f.key)
		if err != nil {
			return gf, err
		}
		*f.dest = int(n)
	}

	pair, err := recordInt(rec, "pairCount")
	if err != nil {
		return gf, err
	}
	gf.FirstTransferBetweenPair = pair == 0

	first, ok := rec.Get("firstStep")
	if !ok {
		return gf, fmt.Errorf("a coluna firstStep não veio no resultado")
	}
	if first != nil {
		n, isInt := first.(int64)
		if !isInt {
			return gf, fmt.Errorf("firstStep com tipo inesperado %T", first)
		}
		age := atStep - int(n)
		gf.DestAccountAgeSteps = &age
	}

	cycle, ok := rec.Get("cycle")
	cycleBool, isBool := cycle.(bool)
	if !ok || !isBool {
		return gf, fmt.Errorf("a coluna cycle veio ausente ou não é booleana (%T)", cycle)
	}
	gf.CycleDetected = cycleBool
	return gf, nil
}

// Subgraph devolve a vizinhança de 1 salto da origem e do destino, com no
// máximo limitPerAccount arestas mais recentes por conta. Implementa
// service.SubgraphSource.
func (c *Client) Subgraph(ctx context.Context, orig, dest string, beforeSeq int64, atStep, limitPerAccount int) (domain.Subgraph, error) {
	sg := domain.Subgraph{Nodes: []string{}, Edges: []domain.SubEdge{}}
	nodes := map[string]bool{}
	edges := map[string]bool{}
	addNode := func(id string) {
		if !nodes[id] {
			nodes[id] = true
			sg.Nodes = append(sg.Nodes, id)
		}
	}
	addNode(orig)
	addNode(dest)

	consultadas := map[string]bool{}
	for _, id := range []string{orig, dest} {
		if consultadas[id] {
			continue
		}
		consultadas[id] = true
		res, err := c.read(ctx, subgraphQuery, map[string]any{
			"id": id, "beforeSeq": beforeSeq, "atStep": atStep, "limit": limitPerAccount,
		})
		if err != nil {
			return domain.Subgraph{}, fmt.Errorf("consultando o subgrafo de %s: %w", id, err)
		}
		for _, rec := range res.Records {
			edge, err := decodeEdge(rec)
			if err != nil {
				return domain.Subgraph{}, err
			}
			if edges[edge.ID] {
				continue
			}
			edges[edge.ID] = true
			sg.Edges = append(sg.Edges, edge)
			addNode(edge.Src)
			addNode(edge.Dst)
		}
	}
	return sg, nil
}

func decodeEdge(rec *neo.Record) (domain.SubEdge, error) {
	get := func(key string) any {
		v, _ := rec.Get(key)
		return v
	}
	id, _ := get("id").(string)
	src, _ := get("src").(string)
	dst, _ := get("dst").(string)
	if id == "" || src == "" || dst == "" {
		return domain.SubEdge{}, fmt.Errorf("aresta do subgrafo incompleta: %v", rec.Values)
	}
	typ, _ := get("type").(string)
	amount, _ := get("amount").(float64)
	step, _ := get("step").(int64)
	isFraud, _ := get("isFraud").(bool)
	return domain.SubEdge{ID: id, Src: src, Dst: dst, Type: typ, Amount: amount, Step: int(step), IsFraud: isFraud}, nil
}
