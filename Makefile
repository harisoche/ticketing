DB_URL ?= postgres://ticketing_user:ticketing_password@localhost:5432/ticketing_db?sslmode=disable

.PHONY: postgres-up postgres-down migrate-up migrate-down migrate-status seed run fmt test vet tidy \
        verify-auth verify-tickets verify-comments verify-uploads verify-search verify-notifications \
        demo docker-up docker-down

# ---------- Local infrastructure ----------

postgres-up:
	docker compose up -d postgres

postgres-down:
	docker compose down

# ---------- Migrations / seeds ----------

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

migrate-status:
	go run ./cmd/migrate status

seed:
	go run ./cmd/seed

# ---------- Application ----------

run:
	go run ./cmd/api

# ---------- Quality ----------

fmt:
	gofmt -w .

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

# ---------- Docker ----------

docker-up:
	docker compose up --build

docker-down:
	docker compose down

# ---------- End-to-end verification ----------

verify-auth:
	bash scripts/verify_auth.sh

verify-tickets:
	DATABASE_URL="$(DB_URL)" PGPASSWORD=ticketing_password bash scripts/verify_tickets.sh

verify-comments:
	DATABASE_URL="$(DB_URL)" PGPASSWORD=ticketing_password bash scripts/verify_comments.sh

verify-uploads:
	DATABASE_URL="$(DB_URL)" PGPASSWORD=ticketing_password bash scripts/verify_uploads.sh

verify-search:
	DATABASE_URL="$(DB_URL)" PGPASSWORD=ticketing_password bash scripts/verify_search.sh

verify-notifications:
	DATABASE_URL="$(DB_URL)" PGPASSWORD=ticketing_password bash scripts/verify_notifications.sh

# Representative end-to-end demo. Requires API running and `make seed` already run.
demo:
	DATABASE_URL="$(DB_URL)" PGPASSWORD=ticketing_password bash scripts/demo_flow.sh
