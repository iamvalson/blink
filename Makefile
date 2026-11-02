-include .env
export

.PHONY: up down logs ps run-api run-worker migrate-up migrate-down migrate-status migrate-baseline keygen images db-setup db-reset

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

# The runner embeds SQL files, tracks versions/dirty state, and locks migrations.
migrate-up:
	go run ./cmd/migrate up

# Reverts ONE migration. Never run against an active publishing deployment.
migrate-down:
	go run ./cmd/migrate down

migrate-status:
	go run ./cmd/migrate status

# Existing manually migrated databases only, after inspecting their schema.
migrate-baseline:
	go run ./cmd/migrate baseline -version "$(VERSION)"

keygen:
	go run ./cmd/keygen -out "$(if $(JWT_SIGNING_KEY_FILE),$(JWT_SIGNING_KEY_FILE),.secrets/jwt-signing-key.pem)"

images:
	docker build -f infra/Dockerfile --target api -t blink-api:local .
	docker build -f infra/Dockerfile --target worker -t blink-worker:local .
	docker build -f infra/Dockerfile --target migrate -t blink-migrate:local .
	docker build -f infra/Dockerfile --target keygen -t blink-keygen:local .

db-setup: up migrate-up
	@echo "Database setup complete"

db-reset:
	docker compose -f infra/docker-compose.yml down -v
	$(MAKE) db-setup