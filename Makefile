.PHONY: run build test fmt vet tidy db-up db-down migrate migrate-down sqlc templ help

BINARY := bin/server
MAIN := ./cmd/server

help:
	@echo "Targets:"
	@echo "  run         - go run ./cmd/server"
	@echo "  build       - build binary to bin/server"
	@echo "  test        - go test ./..."
	@echo "  fmt         - go fmt ./..."
	@echo "  vet         - go vet ./..."
	@echo "  tidy        - go mod tidy"
	@echo "  db-up       - docker compose up -d postgres"
	@echo "  db-down     - docker compose down"
	@echo "  migrate     - goose up (requires goose installed)"
	@echo "  migrate-down- goose down"
	@echo "  sqlc        - sqlc generate"
	@echo "  templ       - templ generate"

run:
	go run $(MAIN)

build:
	go build -o $(BINARY) $(MAIN)

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

tidy:
	go mod tidy

db-up:
	docker compose up -d postgres

db-down:
	docker compose down

migrate:
	goose -dir internal/db/migrations postgres "$$DATABASE_URL" up

migrate-down:
	goose -dir internal/db/migrations postgres "$$DATABASE_URL" down

sqlc:
	sqlc generate

templ:
	templ generate
