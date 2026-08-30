SHELL := /bin/sh

.PHONY: up down ps logs qa sat-bank gofmt backend-build backend-test frontend-check migrate vocab-help bootstrap-admin vocab-demo

up:
	docker compose up -d --build

down:
	docker compose down

ps:
	docker compose ps

logs:
	docker compose logs -f --tail=150 backend

qa:
	python3 tools/qa_ielts.py

sat-bank:
	python3 tools/generate_sat_math_bank.py

gofmt:
	cd backend && gofmt -w $$(find . -name '*.go' -type f)

backend-build:
	cd backend && go build -trimpath -o ../.runtime/bin/backend ./cmd/backend

backend-test:
	cd backend && go test ./...

frontend-check:
	@for app in admin-web center-web teacher-web student-web; do \
	  echo "== $$app =="; \
	  (cd apps/$$app && corepack enable && pnpm install --frozen-lockfile && pnpm typecheck && pnpm lint && pnpm build) || exit 1; \
	done

migrate:
	docker compose --profile tools run --rm compact-migrate

vocab-help:
	cd backend && go run ./cmd/vocab-import -h

bootstrap-admin:
	@test -n "$(EMAIL)" -a -n "$(PASSWORD)" || (echo "Usage: make bootstrap-admin EMAIL=admin@example.com PASSWORD='StrongPassword' NAME='Platform Admin'" && exit 2)
	docker compose --profile tools run --rm bootstrap-admin --email "$(EMAIL)" --password "$(PASSWORD)" --name "$(or $(NAME),Platform Admin)"

vocab-demo:
	docker compose --profile tools run --rm vocab-import -words /import/demo_seed.csv -synonyms /import/demo_synonyms.csv
