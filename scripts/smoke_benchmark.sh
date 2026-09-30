#!/usr/bin/env bash
# Roda um benchmark pela API, acompanha até terminar e mostra o resumo.
# Uso: ./scripts/smoke_benchmark.sh [n] [modelo] [grafo]
#   n       quantidade de transações (padrão 200)
#   modelo  modelo do Ollama (padrão tev1:0.8b)
#   grafo   true ou false (padrão true)
# A API é lida de API_URL (padrão http://127.0.0.1:8080).
set -euo pipefail

API=${API_URL:-http://127.0.0.1:8080}
N=${1:-200}
MODELO=${2:-tev1:0.8b}
GRAFO=${3:-true}

echo "Iniciando benchmark: n=$N modelo=$MODELO grafo=$GRAFO"
resposta=$(curl -sS -X POST "$API/benchmarks" -H 'Content-Type: application/json' \
  -d "{\"n\": $N, \"model\": \"$MODELO\", \"graph\": $GRAFO}")
if ! id=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["run_id"])' <<<"$resposta" 2>/dev/null); then
  echo "Não consegui iniciar. Resposta da API:"
  echo "$resposta"
  exit 1
fi
echo "run_id: $id"

while true; do
  estado=$(curl -sS "$API/benchmarks/$id")
  linha=$(python3 -c 'import json,sys
d = json.load(sys.stdin)
p = d.get("progress")
print(d["status"], "%d/%d" % (p["done"], p["total"]) if p else "-")' <<<"$estado")
  status=${linha%% *}
  echo "status=$status progresso=${linha#* }"
  [ "$status" != "running" ] && break
  sleep 2
done

python3 -m json.tool <<<"$estado"
[ "$status" = "done" ]
