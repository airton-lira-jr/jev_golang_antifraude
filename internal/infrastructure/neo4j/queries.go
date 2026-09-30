package neo4j

// Schema. Os índices de seq e step só são criados no fim da carga, porque
// criar antes deixaria a inserção bem mais lenta.
const (
	createConstraint = `CREATE CONSTRAINT account_id IF NOT EXISTS FOR (a:Account) REQUIRE a.id IS UNIQUE`
	createIndexSeq   = `CREATE INDEX transferred_seq IF NOT EXISTS FOR ()-[t:TRANSFERRED]-() ON (t.seq)`
	createIndexStep  = `CREATE INDEX transferred_step IF NOT EXISTS FOR ()-[t:TRANSFERRED]-() ON (t.step)`
	awaitIndexes     = `CALL db.awaitIndexes(600)`
)

// Carga.
const (
	mergeAccounts = `UNWIND $ids AS id MERGE (:Account {id: id})`

	createEdges = `UNWIND $rows AS r
MATCH (a:Account {id: r.orig})
MATCH (b:Account {id: r.dest})
CREATE (a)-[:TRANSFERRED {seq: r.seq, step: r.step, type: r.type, amount: r.amount, isFraud: r.isFraud}]->(b)`

	maxSeqQuery = `MATCH ()-[t:TRANSFERRED]->() RETURN max(t.seq) AS maxSeq`
	countNodes  = `MATCH (n:Account) RETURN count(n) AS n`
	countRels   = `MATCH ()-[t:TRANSFERRED]->() RETURN count(t) AS n`
)
