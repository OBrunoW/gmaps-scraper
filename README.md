# gmaps-scraper

Microserviço HTTP assíncrono para scrape de leads do Google Maps (API + worker).

## Stack

| Item | Tecnologia |
|------|------------|
| Linguagem | Go 1.27 |
| HTTP | chi |
| Fila | River + Postgres |
| Browser | Playwright / Chromium |
| Auth | `X-Service-Key` · `X-Api-Key` |
| Testes | `go test` |

## Quick start

```bash
cp .env.example .env
docker compose up -d --build
```

Local (precisa Postgres):

```bash
go mod tidy
go run ./cmd/gmaps-scraper serve
go run ./cmd/gmaps-scraper worker
```

## Scripts

```bash
go vet ./...
go test ./...
go build -o bin/gmaps-scraper ./cmd/gmaps-scraper
docker compose up -d --build
```

## Env

| Arquivo | Uso |
|---------|-----|
| `.env` | laptop + teste |
| `.env.prod` | espelho de produção |
| `.env.example` | git |

| Variável | Papel |
|----------|-------|
| `PORT` / `HOST_PORT` | API (`8080`) |
| `DATABASE_URL` ou `DB_*` | Postgres |
| `SERVICE_KEYS` | S2S (`name:secret`, header `X-Service-Key`) |
| `API_KEY` | chave estática (`X-Api-Key`) |
| `PROXIES` | proxies do worker (CSV) |
| `FAST_MODE` | scrape HTTP stealth |

## Banco

Migrations em `migrations/*.sql`, embutidas no binário e aplicadas no start do `serve`.

## API

| Prefixo | Auth |
|---------|------|
| `GET /api/v1/health` | público |
| `POST /api/v1/scrape` | `X-Service-Key` ou `X-Api-Key` |
| `GET /api/v1/jobs` | idem |
| `GET /api/v1/jobs/{job_id}` | idem |
| `DELETE /api/v1/jobs/{job_id}` | idem |

```bash
curl -s -X POST http://127.0.0.1:8080/api/v1/scrape \
  -H 'Content-Type: application/json' \
  -H 'X-Service-Key: changeme' \
  -d '{"keyword":"cafes in athens","max_depth":1}'
```

## Git

| Item | Regra |
|------|-------|
| Commits | `tipo(escopo): resumo`, resumo em português, minúsculo, sem ponto final |
| Branches | `main` é a base; trabalho em `tipo/slug` com PR para `main` |

## Deploy

| Evento | Artefato | Porta |
|--------|----------|-------|
| Push ou PR em `main` | build e testes (`.github/workflows/build.yml`) | — |
| Release publicado | imagem em `ghcr.io/<owner>/<repo>` (`.github/workflows/publish.yml`) | `8080` |
