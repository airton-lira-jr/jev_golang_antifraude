#!/usr/bin/env bash
# Sobe o Ollama num container avulso (fora do compose) e baixa os modelos. Serve para
# quem não quer ou não consegue instalar o Ollama no host, e não precisa de sudo.
# Usa a GPU da NVIDIA sozinho quando o Docker tem o runtime dela (veja o README).
#
# Uso: scripts/ollama-docker.sh [modelo ...]   (padrão: tev1:0.8b e tev1:4b)
#
# Variáveis opcionais:
#   OLLAMA_CONTAINER  nome do container (padrão ollama)
#   OLLAMA_BIND       endereço onde a porta abre (padrão 172.17.0.1, a ponte do Docker)
#   OLLAMA_PORT       porta (padrão 11434)
#   OLLAMA_DNS        DNS usado pelo container, separado por espaço (padrão "1.1.1.1 8.8.8.8").
#                     Deixe vazio para usar o DNS do host.
set -euo pipefail

NOME=${OLLAMA_CONTAINER:-ollama}
# A porta só abre na ponte do Docker. O host.docker.internal aponta para ela, então os
# containers da API alcançam o Ollama e a rede local não.
ENDERECO=${OLLAMA_BIND:-172.17.0.1}
PORTA=${OLLAMA_PORT:-11434}
DNS=${OLLAMA_DNS-"1.1.1.1 8.8.8.8"}

MODELOS=("$@")
if [ ${#MODELOS[@]} -eq 0 ]
then
  MODELOS=(tev1:0.8b tev1:4b)
fi

args=(--restart unless-stopped -v ollama:/root/.ollama -p "${ENDERECO}:${PORTA}:11434")

# Em alguns setups o DNS do host não responde de dentro do container, e o pull dos modelos
# falha com "i/o timeout". Um DNS público resolve.
for servidor in $DNS
do
  args+=(--dns "$servidor")
done

if docker info --format '{{json .Runtimes}}' 2>/dev/null | grep -q nvidia
then
  echo "Runtime da NVIDIA encontrado no Docker, o Ollama vai usar a GPU."
  args+=(--gpus all)
else
  echo "Sem runtime da NVIDIA no Docker, o Ollama vai rodar na CPU (bem mais lento)."
  echo "Para usar a GPU, instale o driver e o nvidia-container-toolkit (veja o README)."
fi

# O tev1 exige o Ollama 0.35 ou mais novo, então sempre buscamos a imagem mais recente.
docker pull ollama/ollama:latest
docker rm -f "$NOME" >/dev/null 2>&1 || true
docker run -d --name "$NOME" "${args[@]}" ollama/ollama >/dev/null

echo "Esperando o Ollama responder..."
pronto=0
for _ in $(seq 1 30)
do
  if curl -fsS "http://${ENDERECO}:${PORTA}/api/version" >/dev/null 2>&1
  then
    pronto=1
    break
  fi
  sleep 1
done
if [ "$pronto" != 1 ]
then
  echo "O Ollama não respondeu a tempo. Últimas linhas do log:"
  docker logs "$NOME" 2>&1 | tail -20
  exit 1
fi

for modelo in "${MODELOS[@]}"
do
  docker exec "$NOME" ollama pull "$modelo"
done
docker exec "$NOME" ollama list
