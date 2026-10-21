OPENAPI_GENERATOR = docker compose run --rm openapi-generator
FLYWAY = docker compose run --rm flyway

.PHONY: openapi-version validate-openapi generate build db-up migrate migrate-info

openapi-version:
	$(OPENAPI_GENERATOR) version

validate-openapi:
	$(OPENAPI_GENERATOR) validate -i /app/api/openapi.yaml

generate:
	rm -rf -- internal/gen/openapi
	$(OPENAPI_GENERATOR) generate -c /app/api/codegen.yaml

build: generate
	mkdir -p bin
	go build -o bin/server ./cmd/server

db-up:
	docker compose up -d --wait postgres

migrate:
	$(FLYWAY) migrate

migrate-info:
	$(FLYWAY) info

