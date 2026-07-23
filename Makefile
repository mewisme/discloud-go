.PHONY: run test lint build web-dev web-build up up-build down snapshot

run: ## Run the API (needs env vars, see .env.example)
	go run ./cmd/discloud

test:
	go vet ./cmd/... ./internal/...
	go test ./cmd/... ./internal/...

lint:
	@fmt=$$(gofmt -l .); if [ -n "$$fmt" ]; then echo "gofmt needed on:"; echo "$$fmt"; exit 1; fi
	cd web && pnpm run lint && pnpm exec tsc --noEmit

build:
	CGO_ENABLED=0 go build -trimpath -o dist/discloud ./cmd/discloud
	CGO_ENABLED=0 go build -trimpath -o dist/discloud-cli ./cmd/discloud-cli

web-dev:
	cd web && pnpm run dev

web-build:
	cd web && pnpm run build

up:
	docker compose up -d

up-build:
	docker compose -f docker-compose.yml -f docker-compose.build.yml up --build -d

down:
	docker compose down

snapshot:
	goreleaser release --snapshot --clean
