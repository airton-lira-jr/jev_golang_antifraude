package service

import (
	"context"

	"fraud-detector-jev/internal/domain"
)

// GraphFeatureSource calcula as features de grafo de uma transação. Quem
// implementa (o Neo4j) precisa respeitar q.BeforeSeq e q.AtStep. Trocar por
// features pré-calculadas em lote é só implementar esta interface de outro jeito.
type GraphFeatureSource interface {
	Features(ctx context.Context, q domain.FeatureQuery) (domain.GraphFeatures, error)
}

// SubgraphSource devolve a vizinhança de 1 salto da origem e do destino, com
// no máximo limitPerAccount arestas por conta.
type SubgraphSource interface {
	Subgraph(ctx context.Context, orig, dest string, beforeSeq int64, atStep, limitPerAccount int) (domain.Subgraph, error)
}
