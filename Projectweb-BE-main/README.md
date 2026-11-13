# Projectweb Backend (Go)

## Requirements
- Go (version compatible with go.mod)
- PostgreSQL

## Setup
1. Create a PostgreSQL database (e.g. `mangahub`).
2. Copy `.env.example` to `.env`.
3. Set `DATABASE_URL` to your own PostgreSQL connection string.
4. Set `JWT_SECRET` to a private random string of at least 32 characters (`openssl rand -hex 64`).
5. (Optional) Set `ADMIN_EMAIL` and `ADMIN_PASSWORD` (8-72 chars) to create the first admin on startup.
6. Run:

```bash
go mod download
go run .
```

Tables are created automatically on startup (`schema.sql` is idempotent). You can still run it by hand.

The API listens on port `8000` by default. The Vite frontend proxies `/api` to `http://127.0.0.1:8000`.

Do not commit or share `.env`. It contains private database credentials and signing secrets.

## Making an admin
- Easiest: set `ADMIN_EMAIL` / `ADMIN_PASSWORD` in `.env` and restart. If that email is already registered, it is promoted to admin and its password is left unchanged.
- Or register normally on the website, then run in PostgreSQL:
  `UPDATE members SET role='admin' WHERE email='you@example.com';`
- Afterwards, admins can promote/demote other members from the admin panel.

## Admin panel
Open `http://localhost:5173/api/admin-panel` (through the Vite proxy, so it shares the website's login)
or `http://127.0.0.1:8000/api/admin-panel` directly. Only admin accounts see the menu; everyone else gets "no permission".

It can: view stats, add manga (manually or import from MangaDex by id/link), delete manga, list/search members,
change a member's role, delete a member, and sync popular manga from MangaDex.

## API
Public: `POST /api/auth/register`, `POST /api/auth/login`, `GET /api/manga?q=&limit=&offset=`, `GET /api/manga/{id}`,
`GET /api/manga/{id}/chapters`, `GET /api/chapters/{id}/pages`

Logged in: `GET /api/me`, `GET /api/me/bookmarks`, `PUT /api/me/bookmarks/{mangaId}`

Admin only (role is re-checked against the database on every request):

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/admin/stats` | counts of manga / members / chapters |
| GET | `/api/admin/members?q=` | list / search members |
| PUT | `/api/admin/members/{id}/role` | `{ "role": "member" \| "admin" }` |
| DELETE | `/api/admin/members/{id}` | delete a member |
| POST | `/api/admin/manga` | add manga manually (`title` required) |
| POST | `/api/admin/manga/import` | `{ "mangadex_id": "<id or mangadex.org link>" }` |
| DELETE | `/api/admin/manga/{id}` | delete manga (chapters and bookmarks cascade) |
| POST | `/api/admin/sync` | import top 100 popular manga from MangaDex |

Safety rules: an admin cannot change or delete their own account, and the last admin can never be demoted or deleted.
A deleted manga is remembered in `blocked_mangas` so a later sync does not bring it back (importing it again by hand removes it from that list).
