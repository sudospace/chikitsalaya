.PHONY: dev up down build docker-up docker-down docker-logs seed-admin

up:
	docker compose up -d db

down:
	docker compose down

dev: up
	go run ./cmd/chikitsalaya

build:
	CGO_ENABLED=0 go build -o bin/chikitsalaya ./cmd/chikitsalaya

# Full stack (Postgres + app) via Docker only, no local Go toolchain needed.
docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f app

# Bootstrap the clinic + its first admin inside the running app container, e.g.:
#   make seed-admin EMAIL=admin@clinic.test PASSWORD=changeme
seed-admin:
	docker compose exec app ./chikitsalaya --seed-admin --admin-email=$(EMAIL) --admin-password=$(PASSWORD)
