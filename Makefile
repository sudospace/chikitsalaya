.PHONY: dev up down build docker-up docker-down docker-logs seed-admin

# Created once on first run with a real generated encryption key (never
# overwritten if it already exists) -- both the local-dev and Docker paths
# need CHIKITSALAYA_LOCAL_STORAGE_KEY set, since the app refuses to boot
# with the (default) local document-storage backend otherwise.
.env:
	cp .env.example .env
	@KEY=$$(openssl rand -hex 32); \
	if [ "$$(uname)" = "Darwin" ]; then \
		sed -i '' "s/^CHIKITSALAYA_LOCAL_STORAGE_KEY=.*/CHIKITSALAYA_LOCAL_STORAGE_KEY=$$KEY/" .env; \
	else \
		sed -i "s/^CHIKITSALAYA_LOCAL_STORAGE_KEY=.*/CHIKITSALAYA_LOCAL_STORAGE_KEY=$$KEY/" .env; \
	fi
	@echo ".env created with a freshly generated CHIKITSALAYA_LOCAL_STORAGE_KEY"

up: .env
	docker compose up -d db

down:
	docker compose down

dev: up
	go run ./cmd/chikitsalaya

build:
	CGO_ENABLED=0 go build -o bin/chikitsalaya ./cmd/chikitsalaya

# Full stack (Postgres + app) via Docker only, no local Go toolchain needed.
docker-up: .env
	docker compose up -d --build

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f app

# Bootstrap the clinic + its first admin inside the running app container, e.g.:
#   make seed-admin EMAIL=admin@clinic.test PASSWORD=changeme
seed-admin:
	docker compose exec app ./chikitsalaya --seed-admin --admin-email=$(EMAIL) --admin-password=$(PASSWORD)
