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
| `HASHID_SALT` | salt do `job_id` público. Lido depois do `.env`: em dev local, trocar o salt muda os `job_id` já emitidos |
| `PROSPECTOR_GEOCODER_URL` | geocodificador do prospector (padrão `https://nominatim.openstreetmap.org`) |
| `APIBRASIL_TOKEN` | segredo da API Brasil. Obrigatório para CEP e CNPJ; sem ele respondem `503` |
| `APIBRASIL_BASE_URL` | URL base da API Brasil (padrão `https://gateway.apibrasil.io/api/v2`) |
| `APIBRASIL_HOMOLOG` | `true` usa homologação e não cobra (padrão `false`) |

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

## Prospector

| Endpoint | Uso |
|----------|-----|
| `POST /api/v1/prospector/jobs` | cria o job de busca de leads (responde `202` com `job_id`) |
| `GET /api/v1/prospector/jobs/{job_id}` | status, `result_count`, `error` e `leads` (só quando `completed`) |
| `GET /api/v1/prospector/cep/{cep}` | logradouro, bairro, cidade, UF, latitude e longitude |
| `GET /api/v1/prospector/cnpj/{cnpj}` | cadastro do CNPJ |

Auth igual ao scrape (`X-Service-Key` ou `X-Api-Key`).

Localização do `POST`:

| Forma | Regra |
|-------|-------|
| `cep` | prioridade sobre as outras; centro vem da API Brasil |
| `address` + `uf` | UF obrigatória quando há endereço |
| `city` + `uf` | cidade e UF |

`radius_km`: padrão `5`, de `1` a `50`.

CEP e CNPJ precisam de `APIBRASIL_TOKEN`; sem ele respondem `503`.

| Consulta | Custo |
|----------|-------|
| CEP | R$ 0,04 |
| CNPJ | R$ 0,79 |

A API Brasil cobra a cada consulta. Com `APIBRASIL_HOMOLOG=true` não há cobrança.

## Páginas de teste

Servidas pelo próprio serviço em `/tools/` (mesma origem, sem CORS):

| Página | Uso |
|--------|-----|
| `/tools/scrape.html` | scrape |
| `/tools/prospector.html` | prospector, CEP e CNPJ |
| `/tools/lead-learning.html` | placeholder |

As páginas guardam a URL e a chave no `localStorage` do navegador. Não use a chave de produção nelas.

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
