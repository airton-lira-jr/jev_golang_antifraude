package neo4j

import "fmt"

// featuresQueryTemplate calcula todas as features de uma transação numa ida só
// ao banco. O %d é o limite de saltos do ciclo, porque o Cypher não aceita
// parâmetro no tamanho de caminho variável. Cada CALL devolve exatamente uma
// linha, mesmo quando a conta nunca apareceu (count dá 0 e min dá null).
//
// Regra de tempo em todos os blocos: só arestas com seq menor que $beforeSeq e
// step menor ou igual a $atStep. A janela de K horas é step > atStep - window.
// Esta query não pode ler o gabarito (nada de isFraud nem isFlagged).
const featuresQueryTemplate = `
CALL {
  MATCH (:Account {id: $orig})-[t:TRANSFERRED]->(:Account {id: $dest})
  WHERE t.seq < $beforeSeq AND t.step <= $atStep
  RETURN count(t) AS pairCount
}
CALL {
  MATCH (s:Account)-[t:TRANSFERRED]->(:Account {id: $dest})
  WHERE t.seq < $beforeSeq AND t.step <= $atStep
  RETURN count(DISTINCT s) AS destInDegree
}
CALL {
  MATCH (s:Account)-[t:TRANSFERRED]->(:Account {id: $dest})
  WHERE t.seq < $beforeSeq AND t.step <= $atStep AND t.step > $atStep - $window
  RETURN count(DISTINCT s) AS destFanIn
}
CALL {
  MATCH (:Account {id: $dest})-[t:TRANSFERRED]->(r:Account)
  WHERE t.seq < $beforeSeq AND t.step <= $atStep AND t.step > $atStep - $window
  RETURN count(DISTINCT r) AS destFanOut
}
CALL {
  MATCH (:Account {id: $orig})-[t:TRANSFERRED]->(r:Account)
  WHERE t.seq < $beforeSeq AND t.step <= $atStep AND t.step > $atStep - $window
  RETURN count(DISTINCT r) AS origFanOut
}
CALL {
  MATCH (:Account {id: $dest})-[t:TRANSFERRED]-(:Account)
  WHERE t.seq < $beforeSeq AND t.step <= $atStep
  RETURN min(t.step) AS firstStep
}
CALL {
  OPTIONAL MATCH p = (:Account {id: $dest})-[:TRANSFERRED*1..%d]->(:Account {id: $orig})
  WHERE all(e IN relationships(p) WHERE e.seq < $beforeSeq AND e.step <= $atStep)
  WITH p LIMIT 1
  RETURN p IS NOT NULL AS cycle
}
RETURN pairCount, destInDegree, destFanIn, destFanOut, origFanOut, firstStep, cycle
`

// subgraphQuery devolve as arestas mais recentes que tocam uma conta. Aqui o
// isFraud pode aparecer, porque é só para pintar o desenho e não entra em
// nenhuma feature.
const subgraphQuery = `
MATCH (:Account {id: $id})-[t:TRANSFERRED]-(:Account)
WHERE t.seq < $beforeSeq AND t.step <= $atStep
RETURN elementId(t) AS id, startNode(t).id AS src, endNode(t).id AS dst,
       t.type AS type, t.amount AS amount, t.step AS step, t.isFraud AS isFraud
ORDER BY t.seq DESC
LIMIT $limit
`

// FeaturesQuery devolve o texto Cypher das features com o limite de saltos do
// ciclo já colocado. Aceita de 1 a 5 saltos.
func FeaturesQuery(cycleMaxHops int) (string, error) {
	if cycleMaxHops < 1 || cycleMaxHops > 5 {
		return "", fmt.Errorf("cycle_max_hops precisa estar entre 1 e 5, veio %d", cycleMaxHops)
	}
	return fmt.Sprintf(featuresQueryTemplate, cycleMaxHops), nil
}
