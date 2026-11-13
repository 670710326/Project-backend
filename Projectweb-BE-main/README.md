# Projectweb Backend (Go)

## Requirements
- Go (version compatible with go.mod)
- PostgreSQL

## Setup
1. Create a PostgreSQL database named `mangahub`.
2. Execute `schema.sql` in that database.
3. Copy `.env.example` to `.env`.
4. Set `DATABASE_URL` to your own PostgreSQL connection string.
5. Set `JWT_SECRET` to a private random string of at least 32 characters.
6. Run:

```bash
go mod download
go run .
```

The API listens on port `8000` by default. The Vite frontend proxies `/api` to `http://127.0.0.1:8000`.

Do not commit or share `.env`. It contains private database credentials and signing secrets.
