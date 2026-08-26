.PHONY: up down logs run-api setup clean migrate-up migrate-down migrate-create

up:
	docker compose -f infra/docker-compose.yml up -d

down:
	docker compose -f infra/docker-compose.yml down

logs:
	docker compose -f infra/docker-compose.yml logs -f

ps:
	docker compose -f infra/docker-compose.yml ps

run-api:
	go run cmd/api/main.go

setup:
	go mod download
	cp .env.example .env

clean:
	docker compose -f infra/docker-compose.yml down -v

migrate-up:
	migrate -path migrations -database "$(DATABASE_URL)?sslmode=disable" up

migrate-down:
	migrate -path migrations -database "$(DATABASE_URL)?sslmode=disable" down

migrate-create:
	migrate create -ext sql -dir migrations -seq $(NAME)