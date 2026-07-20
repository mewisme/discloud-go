.PHONY: run test lint build web-dev web-build up down snapshot

run: ## Run the API (needs env vars, see .env.example)
	go run ./cmd/discloud

test:
	go vet ./...
	go test ./...

lint:
	gofmt -l .
	cd web && pnpm run lint && pnpm exec tsc --noEmit

build:
	CGO_ENABLED=0 go build -trimpath -o dist/discloud ./cmd/discloud

web-dev:
	cd web && pnpm run dev

web-build:
	cd web && pnpm run build

up:
	docker compose up --build -d

down:
	docker compose down

snapshot:
	goreleaser release --snapshot --clean
