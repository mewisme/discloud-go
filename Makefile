.PHONY: run test lint build web-dev web-build up down snapshot

run: ## Run the API (needs env vars, see .env.example)
	go run ./cmd/discloud

test:
	go vet ./...
	go test ./...

lint:
	gofmt -l .
	cd web && npm run lint && npx tsc --noEmit

build:
	CGO_ENABLED=0 go build -trimpath -o dist/discloud ./cmd/discloud

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build

up:
	docker compose up --build -d

down:
	docker compose down

snapshot:
	goreleaser release --snapshot --clean
