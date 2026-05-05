# Copilot Instructions for Waystone Web

## Architecture Overview

**Waystone Web** is a full-stack event sign-up application with a clear separation between backend and frontend:

- **Backend**: Single-file Go HTTP server (`main.go`) using only the standard library
  - Serves static files from `./static` directory at the root path
  - Exposes two REST API endpoints for events and sign-ups
  - No external dependencies (yet) - uses Go 1.21 stdlib

- **Frontend**: Three-file SPA in `./static` directory
  - `index.html` - Form structure with event listing section
  - `styles.css` - Responsive design (mobile-first, gradient background)
  - `script.js` - Fetch-based API calls with basic form validation

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

- **Handler naming**: Functions that handle HTTP endpoints are named `handle<Action>` (e.g., `handleGetEvents`, `handleSignup`)
- **Response format**: All API responses are JSON; handlers always set `Content-Type: application/json` and appropriate status codes
- **Method validation**: Use `if r.Method != http.MethodPost` pattern to guard endpoints (example in `handleSignup`)
- **No request body parsing yet**: Handlers currently only read headers and URL. If parsing JSON bodies, add `json.NewDecoder(r.Body).Decode()` pattern

### Frontend (JavaScript)

- **Fetch-based**: All API calls use modern `fetch()` API (not jQuery/Axios)
- **Error handling**: Try/catch wraps API calls; `catch` shows user-friendly error message via `showMessage(text, 'error')`
- **Message display**: Use `showMessage(text, type)` helper to show success/error messages (auto-hides after 5 seconds)
- **Event lifecycle**: `DOMContentLoaded` event triggers initial load; event listeners attached once on page load

### Styling

- **Mobile-first approach**: Base styles target mobile; media queries can be added for larger screens
- **Utility color palette**: Primary purple (`#667eea` / `#764ba2`), success (`#d4edda`), error (`#f8d7da`)
- **Form styling**: Consistent padding (12px), rounded corners (5px), focus states with shadow

### Routing Structure

- `/` - Serves `index.html` and static assets (catch-all file server)
- `/api/events` - GET only, returns JSON array of event objects
- `/api/signup` - POST only, accepts JSON signup data, returns `{"status": "success", "message": "..."}`

### Environment Variables

- `PORT` - Server port (defaults to 8080 if not set)

### Static Assets

- All frontend files must live in `./static/` directory
- The Go server mounts this directory at the root (`/`)
- Adding new assets: just add files to `./static/` (no build step needed)

## Future Extension Points

When adding features, consider these patterns established in the codebase:

- **New API endpoints**: Add `http.HandleFunc("/api/path", handleAction)` in `main.go`, create handler function
- **Database integration**: Will need `go.sum`, external packages, and potential schema migrations
- **Request body parsing**: Add `err := json.NewDecoder(r.Body).Decode(&data)` in handlers needing it
- **Authentication**: Consider middleware pattern for shared auth logic across handlers
- **Frontend state management**: Currently minimal; consider context API or state object if complexity grows

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
