#!/usr/bin/env bash
# Sobe um Neo4j descartável em outra porta, roda os testes de integração e
# derruba tudo no final. Nunca encosta na base de verdade do compose.
set -euo pipefail

NOME=fraud-neo4j-it
PORTA=${NEO4J_TEST_PORT:-17687}
SENHA=testpassword123
IMAGEM=${NEO4J_IMAGE:-neo4j:5.26-community}

limpar() { docker rm -f -v "$NOME" >/dev/null 2>&1 || true; }
trap limpar EXIT
limpar

docker run -d --name "$NOME" -p "127.0.0.1:${PORTA}:7687" \
  -e NEO4J_AUTH="neo4j/${SENHA}" \
  -e NEO4J_server_memory_heap_initial__size=512m \
  -e NEO4J_server_memory_heap_max__size=512m \
  -e NEO4J_server_memory_pagecache_size=256m \
  "$IMAGEM" >/dev/null

echo "Esperando o Neo4j de teste ficar pronto..."
pronto=0
for _ in $(seq 1 60); do
  if docker exec "$NOME" cypher-shell -u neo4j -p "$SENHA" 'RETURN 1' >/dev/null 2>&1; then
    pronto=1
    break
  fi
  sleep 2
done
if [ "$pronto" != 1 ]; then
  echo "O Neo4j de teste não subiu a tempo. Últimas linhas do log:"
  docker logs "$NOME" 2>&1 | tail -20
  exit 1
fi

export NEO4J_TEST_URI="bolt://127.0.0.1:${PORTA}"
export NEO4J_TEST_PASSWORD="$SENHA"
go test -race -tags=integration -count=1 ./internal/infrastructure/neo4j/...
