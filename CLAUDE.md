# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Waystone Web: Go (stdlib `net/http`, SQLite via `modernc.org/sqlite`, local username/password auth) backend + plain HTML/JS frontend in `./static` (no build step; Alpine.js vendored at `static/vendor/`). MVP scope and progress: `docs/mvp.md`.

## Commands

- `.env.example` must list every env var the code reads (`os.Getenv`), with required/optional and defaults. Update it whenever you add, rename, or remove one, and keep `docker-compose.yml`'s `environment:` in sync.
- Run locally: `go run main.go` → http://localhost:8080. Set `ADMIN_PASSWORD` (and `SESSION_SECRET`) per `.env.example`; without `ADMIN_PASSWORD` the server starts but the admin can't log in.
- Verify changes: `go build ./... && go vet ./...`. There are no tests or linters; do not add tests unless explicitly asked.
- `./deploy.sh` runs `docker-compose down -v`, which **wipes the SQLite data volume**. Never run it against data worth keeping.
- For local Docker testing use `/deploy-local` (`up`/`down`/`status`), not `docker compose up`: plain compose names the project after the folder, so every worktree leaves its own image and volume behind.
- Static files are copied into the Docker image, so frontend changes need `docker-compose build --no-cache`.
- Local DB lives at `./data/waystone.db` (gitignored).

## Architecture gotchas

- Routes are registered in `api/router.go` (not `main.go`, despite the README). `/` is auth-wrapped and serves `dashboard.html`, so every root-level JS/CSS file a page loads must get its own `http.HandleFunc` entry there, or be referenced under `/static/...`.
- DB access goes through the `Store` interface in `db/store.go`: add the method to the interface, implement it on the SQLite store, and expose a package-level wrapper in `db/` that calls `GetStore()`.
- Protect routes with `middleware.AuthMiddleware`; use `middleware.GetSession()` / `ClearSession()` for session state.
- User display name precedence: `Nickname` → `Name` → `Username` (`User.DisplayName()`). API responses that reference users keep raw IDs and add enriched `*_user` / `*_users` objects (mirror the campaigns API).
- Session responses are one per `(session_id, user_id)` (upsert); pending players are computed from the campaign's player list.
- When changing `models/user.go`, update seed data in `config/config.go` and keep `SaveUser` and the `SELECT`/`Scan` sites in `db/sqlite_store.go` in sync.
- The `ADMIN_USERNAME` user is env-managed: password reset from `.env` on every start; can't be blocked, deleted or demoted.

## Conventions

- Go: handlers named `HandleXxx`; always set `Content-Type: application/json` and an explicit status; decode bodies with `json.NewDecoder(r.Body).Decode(&v)`; guard methods with `r.Method` checks.
- JS: `fetch()` only; wrap calls in try/catch and report via `showMessage(text, 'error')`; use `textContent` (or `escapeHtml()`) for user-generated content; init on `DOMContentLoaded`.
- Styling follows `STYLE_GUIDE.md` (black, monospace, cyan accent). Read it before UI work. Bump the `?v=N` cache-busting query on CSS links when changing CSS.
- Keep the app lightweight and minimal: surgical changes, reuse existing helpers before adding abstractions, no broad refactors unless asked.

## Git

- Don't commit until asked. Commit subjects: lower case, ≤6 words, **no** `feat:`/`fix:` prefixes; put details in the body. Small logical commits.
- Branches use type prefixes: `feat/...`, `fix/...`, `chore/...`; worktrees go under `.claude/worktrees/` with matching names (`feat-...`).
- PRs are squash-merged with titles like `feat/admin users`.
