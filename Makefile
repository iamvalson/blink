include .env
export

.PHONY: up down logs run-api run-worker migrate-up migrate-down db-setup db-reset

up:
	docker compose -f infra/docker-compose.yml up -d --wait

down:
	docker compose -f infra/docker-compose.yml down

logs:
	docker compose -f infra/docker-compose.yml logs -f

ps:
	docker compose -f infra/docker-compose.yml ps

run-api:
	go run cmd/api/main.go

run-worker:
	go run cmd/worker/main.go

migrate-up:
	@echo "Running migrations..."
	psql -v ON_ERROR_STOP=1 "$(DATABASE_URL)?sslmode=disable" -f migrations/000001_init_schema.up.sql
	@echo "Migrations complete"

migrate-down:
	@echo "Rolling back migrations..."
	psql -v ON_ERROR_STOP=1 "$(DATABASE_URL)?sslmode=disable" -f migrations/000001_init_schema.down.sql
	@echo "Rollback complete"

db-setup: up migrate-up
	@echo "Database setup complete"

db-reset:
	docker compose -f infra/docker-compose.yml down -v
	$(MAKE) db-setup