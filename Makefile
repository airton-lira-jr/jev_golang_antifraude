GO ?= go

.PHONY: up down seed lint test-go test-integration test-py smoke

# Sobe o Neo4j e a API (constrói a imagem se preciso) e espera ficarem saudáveis.
up:
	docker compose up -d --build --wait

down:
	docker compose down

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

test-py:
	cd web_dashboard && .venv/bin/python -m pytest

# Roda um benchmark pequeno pela API. Exemplo: make smoke ARGS="200 tev1:0.8b true"
smoke:
	./scripts/smoke_benchmark.sh $(ARGS)
