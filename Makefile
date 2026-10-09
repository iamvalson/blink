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
	@for f in $(shell ls migrations/*.up.sql | sort); do \
		echo "  Applying $$f..."; \
		psql -v ON_ERROR_STOP=1 "$(DATABASE_URL)?sslmode=disable" -f $$f; \
	done
	@echo "Migrations complete"

migrate-down:
	@echo "Rolling back migrations..."
	@for f in $(shell ls migrations/*.down.sql | sort -r); do \
		echo "  Reverting $$f..."; \
		psql -v ON_ERROR_STOP=1 "$(DATABASE_URL)?sslmode=disable" -f $$f; \
	done
	@echo "Rollback complete"

db-setup: up migrate-up
	@echo "Database setup complete"

db-reset:
	docker compose -f infra/docker-compose.yml down -v
	$(MAKE) db-setup