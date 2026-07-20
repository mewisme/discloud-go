# Contributing

Thanks for helping with DisCloud.

## Development

```bash
cp .env.example .env
# set DISCORD_BOT_TOKEN and DISCORD_CHANNEL_ID

docker compose up --build -d   # full stack
# or locally:
go run ./cmd/discloud
cd web && pnpm install && pnpm run dev
```

## Checks before a PR

```bash
gofmt -w .
go vet ./...
go test ./...

cd web
pnpm run lint
pnpm exec tsc --noEmit
pnpm run build
```

## Pull requests

- Keep changes focused; prefer small PRs.
- Match existing style (Go stdlib HTTP, few dependencies).
- Add or update tests when changing upload/download/chunk behavior.
- Do not commit `.env`, tokens, or personal Discord channel IDs.
- Update `README.md` / `.env.example` when you change configuration.

## Issues

Use the bug / feature templates under **New issue**. For security reports, see [SECURITY.md](SECURITY.md).
