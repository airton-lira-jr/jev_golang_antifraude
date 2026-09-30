GO ?= go

.PHONY: up down seed ollama lint lint-py setup-py test-go test-integration test-py smoke

# Sobe o Neo4j e a API (constrói a imagem se preciso) e espera ficarem saudáveis.
up:
	docker compose up -d --build --wait

down:
	docker compose down

# Sobe o Ollama num container avulso e baixa os modelos (alternativa a instalar no host).
# Usa a GPU da NVIDIA sozinho quando o Docker tem o runtime dela.
ollama:
	./scripts/ollama-docker.sh

# Carrega o CSV no Neo4j. Pode rodar de novo para retomar de onde parou.
seed:
	docker compose build seeder
	docker compose run --rm seeder

lint:
	@test -z "$$(gofmt -l cmd internal)" || { echo "gofmt encontrou arquivos fora do padrão:"; gofmt -l cmd internal; exit 1; }
	$(GO) vet ./...
	$(GO) vet -tags=integration ./...

test-go:
	$(GO) test -race ./...

# Sobe um Neo4j descartável e roda os testes de integração.
test-integration:
	./scripts/test-integration.sh

# Cria o ambiente Python do dashboard (para rodar os testes e o app fora do Docker).
setup-py:
	python3 -m venv web_dashboard/.venv
	web_dashboard/.venv/bin/pip install -r web_dashboard/requirements-dev.txt

lint-py:
	web_dashboard/.venv/bin/ruff check web_dashboard
	web_dashboard/.venv/bin/ruff format --check web_dashboard

test-py:
	cd web_dashboard && .venv/bin/python -m pytest

# Roda um benchmark pequeno pela API. Exemplo: make smoke ARGS="200 tev1:0.8b true"
smoke:
	./scripts/smoke_benchmark.sh $(ARGS)
