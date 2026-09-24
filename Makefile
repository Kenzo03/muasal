TEST_DATABASE_URL ?= postgres://owner:owner@localhost:55432/postgres?sslmode=disable
COMPOSE = docker compose -f deploy/compose.yaml --env-file deploy/.env

.PHONY: generate testdb test up down logs admin e2e

generate: ## regenerate Go stubs, sqlc code and TypeScript API types
	cd server && go generate ./...
	cd web && npm run gen:api

testdb: ## start a throwaway PostgreSQL for the Go tests
	docker run -d --rm --name muasal-testdb -e POSTGRES_USER=owner -e POSTGRES_PASSWORD=owner \
		-p 55432:5432 pgvector/pgvector:0.8.6-pg18-trixie

test:
	cd server && TEST_DATABASE_URL='$(TEST_DATABASE_URL)' go test ./...

up:
	$(COMPOSE) up -d --build --wait

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f

admin: ## make admin EMAIL=you@example.com NAME="Your Name"
	$(COMPOSE) exec app /app admin create-admin --email "$(EMAIL)" --name "$(NAME)"

e2e:
	cd web && npx playwright test
