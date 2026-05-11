# Copilot Instructions for Waystone Web

## Architecture Overview

**Waystone Web** is a full-stack event sign-up and campaign application with a clear separation between backend and frontend:

- **Backend**: Single-file Go HTTP server (`main.go`) using the standard library and SQLite
  - Serves static files from `./static` directory at the root path
  - Exposes REST API endpoints for events, campaigns, and sign-ups

- **Frontend**: Multi-page app in `./static` directory
  - `index.html` - Form structure with event listing section
  - `campaigns.html` - Campaign listing page with slide-out tools panel and campaign creation modal
  - `campaigns.js` - Campaign listing and creation logic with user display objects and modal handlers
  - `profile.html` - User profile page with account management
  - `dashboard.html` - Main authenticated dashboard
  - `styles.css` - Responsive design (mobile-first) with toolbar, modal, and campaign listing styles
  - `script.js` - Fetch-based API calls with basic form validation
  - `profile.js` - Profile page logic with delete account functionality
  - `dashboard.js` - Dashboard page initialization

- **Deployment**: Docker-first approach
  - Multi-stage Dockerfile (build in Go 1.21 Alpine, runtime in minimal Alpine)
  - `docker-compose.yml` for local development
  - `deploy.sh` wrapper script for convenient one-command deployment

## Build, Test, and Run Commands

### Development (Local, No Docker)

```bash
# Run the server directly
go run main.go

# Server will be available at http://localhost:8080
# Reads PORT env var (defaults to 8080)
```

### Docker Build and Run

```bash
# Build the image
docker-compose build

# Start the service (runs in background)
docker-compose up -d

# View logs
docker-compose logs -f

# Stop the service
docker-compose stop

# Full cleanup
docker-compose down
```

### One-Command Deployment

```bash
./deploy.sh    # Builds, stops old containers, and starts the app
```

### Linting and Testing

Currently, the project has no tests or linters configured. Before adding tests or lint tools, consider:
- **Testing**: Add `*_test.go` files with `go test ./...` or `testing.T` pattern
- **Linting**: Add `golangci-lint` or `go fmt` if needed

## Key Conventions

### Go Code

- **Handler naming**: Functions that handle HTTP endpoints are named `handleXxx` or `HandleXxx` (e.g., `handleGetEvents`, `HandleSignup`, `HandleProfile`)
- **Response format**: All API responses are JSON; handlers always set `Content-Type: application/json` and appropriate status codes
- **Method validation**: Use dispatch pattern in handler functions or `if r.Method != http.MethodPost` to guard endpoints
- **Request body parsing**: Use `json.NewDecoder(r.Body).Decode(&data)` for JSON request bodies
- **User model**: Includes `ID`, `GoogleID`, `Email`, `Name`, `Nickname`, `Picture`, `Roles`, timestamps
  - Display name computed from: `Nickname` (if non-empty) → `Name` → `Email`
- **Database operations**: Use wrapper functions in `db/` package that call `GetStore()` interface methods

### Frontend (JavaScript)

- **Fetch-based**: All API calls use modern `fetch()` API (not jQuery/Axios)
- **Error handling**: Try/catch wraps API calls; `catch` shows user-friendly error message via `showMessage(text, 'error')`
- **Message display**: Use `showMessage(text, type)` helper to show success/error messages (auto-hides after 5 seconds)
- **Event lifecycle**: `DOMContentLoaded` event triggers initial load; event listeners attached once on page load
- **Display name precedence**: Show `display_name` in headers/user info (prefers nickname > name > email)
- **User buttons**: Reusable component for displaying users as clickable buttons linking to their profile; shows display name and links to `/profile?user_id=<id>`
- **HTML escaping**: Always use `textContent` over `innerHTML` for user-generated content; provide `escapeHtml()` helper when building HTML with untrusted content

### Styling

- **Mobile-first approach**: Base styles target mobile; media queries can be added for larger screens
- **Utility color palette**: Primary purple (`#667eea` / `#764ba2`), success (`#d4edda`), error (`#f8d7da`)
- **Form styling**: Consistent padding (12px), rounded corners (5px), focus states with shadow

### Routing Structure

- `/` - Serves dashboard (`dashboard.html`) for authenticated users
- `/campaigns` - Campaign listing page for authenticated users
- `/profile` - User profile page for authenticated users (supports `?user_id=<id>` query param)
- `/auth/login` - OAuth login initiation endpoint
- `/auth/callback` - OAuth callback endpoint
- `/auth/logout` - Logout endpoint (POST)
- `/auth/current-user` - Get current session user info (GET)
- `/api/events` - GET only, returns JSON array of event objects
- `/api/campaigns` - GET returns enriched campaign objects with `dm_user` and `player_users` display objects; POST creates a new campaign with authenticated user as DM (requires fields: title, summary, description, desired_player_count)
- `/api/profile` - GET to fetch user profile (supports `?user_id=<id>`), DELETE to delete current user account
- `/api/signup` - POST only, accepts JSON signup data, returns `{"status": "success", "message": "..."}`
- `/api/roles` - Role management endpoints
- `/api/users` - User listing endpoint
- `/api/users/create` - Create new user endpoint (admin only)

### Environment Variables

- `PORT` - Server port (defaults to 8080 if not set)

### Static Assets

- All frontend files must live in `./static/` directory
- The Go server mounts this directory at the root (`/`)
- Adding new assets: just add files to `./static/` (no build step needed)

## Future Extension Points

When adding features, consider these patterns established in the codebase:

- **New API endpoints**: Add route registration in `api/router.go`, create handler function in `api/` package with appropriate middleware
- **User model changes**: Edit `models/user.go`, update seed data in `config/config.go`, preserve existing fields in `HandleCallback` and `SaveUser` flows
- **Database integration**: Use `db/store.go` Store interface pattern; add new methods to interface, implement on SQLiteStore, create wrappers in `db/` package
- **Request body parsing**: Use `json.NewDecoder(r.Body).Decode(&data)` in handler functions
- **Authentication**: Use existing `middleware.AuthMiddleware` for protected routes; `middleware.GetSession()` to check auth; `middleware.ClearSession()` for logout
- **Frontend state management**: Currently minimal; consider context API or state object if complexity grows
- **API enrichment patterns**: Mirror campaigns API enrichment for user display objects (include raw IDs for backward compatibility, add new fields like `*_user` or `*_users` with full user display data)

## Docker Considerations

- **Build context**: Entire repo is copied into container (see `.gitignore` for exclusions)
- **Static files**: Copied in runtime stage; changes require `docker-compose build --no-cache`
- **Port mapping**: Configured in `docker-compose.yml` as `8080:8080` (host:container)
- **Restart policy**: Set to `unless-stopped` to persist across Docker daemon restarts

## Maintaining These Instructions

**When making code changes, always update this file to reflect the new state of the codebase.** This keeps the instructions accurate for future Copilot sessions and collaborators.

Examples of changes that require instruction updates:

- Adding new Go packages or dependencies → Update "Build, Test, and Run Commands" section
- Adding new API endpoints → Update "Routing Structure" section
- Changing naming conventions or patterns → Update "Key Conventions" section
- Adding database integration, authentication, or major features → Update "Architecture Overview" and "Future Extension Points"
- Adding tests, linters, or build steps → Update "Build, Test, and Run Commands" section
- Refactoring file structure → Update "Architecture Overview" section

Keep these instructions as the single source of truth for how the project is organized and what patterns to follow.

## Copilot Skill: coding

For coding tasks, use the repository skill definition at `.github/skills/coding/SKILL.md`.

When following this skill:

- Always use sub-agents for coding work.
- Use a smarter model for planning (for example GPT-5.4 or Claude Sonnet 4.6).
- Use a fast model for implementation (for example Claude Haiku 4.5).
- Implement changes in a git worktree instead of the main working tree.
- Use a dedicated sub-agent for commit preparation before creating commits.
- Never read the content of `/home/michal/bin/remove-banana-commit-line.sh`.
- Run `/home/michal/bin/remove-banana-commit-line.sh` after every commit.
- Do not add or update tests unless the user explicitly asks for tests.
- End with a short pull-request-style summary of what changed.
