# Waystone Web - Event & Campaign Management

A lightweight Go application hosting event sign-up and campaign management. Built as a Docker container for easy deployment.

## Project Structure

```
.
├── main.go              # Go HTTP server entry point
├── go.mod               # Go module definition
├── go.sum               # Go module dependencies
├── Dockerfile           # Container image definition
├── docker-compose.yml   # Docker Compose configuration
├── deploy.sh            # Deployment script
├── .env.example         # Environment variables template
├── README.md            # This file
├── .gitignore
├── api/                 # API handlers and routing
│   ├── router.go        # Route registration
│   ├── auth.go          # Login, logout, change-password handlers
│   ├── events.go        # Event management handlers
│   ├── campaigns.go     # Campaign management handlers
│   ├── signup.go        # Signup handlers
│   └── admin.go         # Admin user management endpoints
├── db/                  # Database layer
│   ├── store.go         # SQLite storage interface and implementation
│   ├── sqlite_store.go  # SQLite database store implementation
│   ├── campaign.go      # Campaign CRUD operations
│   ├── user.go          # User CRUD operations
│   ├── uuid.go          # UUID generation utilities
├── middleware/          # HTTP middleware
│   └── auth.go          # Authentication middleware
├── models/              # Data models and types
│   ├── event.go         # Event struct
│   ├── campaign.go      # Campaign struct + status lifecycle
│   ├── signup.go        # Signup struct
│   ├── user.go          # User struct
│   └── roles.go         # Role constants and helper functions
└── static/              # Frontend files
    ├── index.html       # Main application page
    ├── login.html       # Username/password login page
    ├── styles.css       # Styling (black minimalistic design)
    └── script.js        # Client-side JavaScript (Fetch API)
```

## Features

- **Go Backend**: Lightweight HTTP server with event management APIs
- **SQLite Storage**: Persistent data storage for campaigns and user accounts using SQLite
- **Web Frontend**: Responsive sign-up form with black minimalistic design and Fira Code monospace font
- **Docker Support**: Containerized for consistent deployment with persistent data volumes
- **Easy Deployment**: Simple deploy script for local Docker deployment

## Prerequisites

- Docker (version 20.10+)
- Docker Compose (version 1.29+)
- Go 1.27+ (only for running without Docker)

## Quick Start

### 1. Deploy with the Deploy Script (Recommended)

```bash
chmod +x deploy.sh
./deploy.sh
```

The application will be available at: **http://localhost:8080**

### 2. Manual Deployment

```bash
docker-compose build
docker-compose up -d
```

### 3. Local Development (without Docker)

```bash
go mod download
SESSION_SECRET=$(openssl rand -base64 32) ADMIN_PASSWORD=... go run main.go
# with demo users, campaigns and sessions on a fresh DB:
SEED_DEMO_DATA=1 SESSION_SECRET=$(openssl rand -base64 32) ADMIN_PASSWORD=... go run main.go
```

Visit: **http://localhost:8080**

## API Endpoints

### Get Events
- **GET** `/api/events`
- Returns: List of available events

### Get Campaigns
- **GET** `/api/campaigns`
- Returns: List of campaigns with title, status, summary, description, DM, players, and sign-up openness.

### Sign Up
- **POST** `/api/signup`
- Body:
  ```json
  {
    "event": "1",
    "name": "John Doe",
    "email": "john@example.com",
    "phone": "555-1234"
  }
  ```
- Returns: `{"status": "success", "message": "Signup received"}`

## Docker Commands

```bash
# Build the image
docker-compose build

# Start the application
docker-compose up -d

# View logs
docker-compose logs -f

# Stop the application
docker-compose stop

# Remove containers and volumes
docker-compose down

# Restart the application
docker-compose restart
```

## Environment Variables

The application supports the following environment variables for configuration:

### Configuration File (.env)

A `.env` file is used to manage environment variables. The `deploy.sh` script will automatically:
1. Check if `.env` exists
2. If not, copy from `.env.example` as a template
3. Load all variables from `.env` before starting the container

**To set up your environment:**

1. The first time you run `./deploy.sh`, it will create `.env` from `.env.example`:
   ```bash
   chmod +x deploy.sh
   ./deploy.sh
   ```

2. Edit `.env` and fill in your configuration:
   ```bash
   nano .env
   ```

3. Run the deployment again with your configured variables:
   ```bash
   ./deploy.sh
   ```

### Available Variables

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `PORT` | `8080` | No | Server port number |
| `ADMIN_USERNAME` | `admin` | No | Username of the env-managed admin account |
| `ADMIN_PASSWORD` | (empty) | Yes | Password of the admin account; applied on every startup. Write `$` as `$$` in `.env` (compose interpolates it). If unset, the admin cannot log in |
| `SESSION_SECRET` | (none) | Yes | Secret key for signing session cookies, min 32 chars (`openssl rand -base64 32`). The server refuses to start if it is unset, too short, or an old placeholder value |
| `SESSION_COOKIE_SECURE` | `true` | No | Mark the session cookie `Secure` (HTTPS only). Browsers drop Secure cookies on plain-HTTP origins other than `localhost` (older Safari drops them there too), so set `false` only when serving over plain HTTP on a LAN IP/hostname (login otherwise "succeeds" but the cookie never sticks) |
| `TRUST_PROXY` | (off) | No | Use the last `X-Forwarded-For` entry as the client IP for login rate limits. Set only when the app is reachable solely through a reverse proxy (e.g. Caddy) |

### Example .env File

See `.env.example` in the repository for a complete template with all available variables and descriptions.

## Data Persistence

The application uses **SQLite** (embedded SQL database) to store campaigns, user accounts, and role information. Data is persisted in a Docker volume (`waystone-data`) mounted at `/root/data` in the container.

### Data Storage Location
- Docker volume: `waystone-data`
- Container path: `/root/data/waystone.db`

### Data Preservation
- Data persists across container restarts: `docker-compose restart`
- Data is removed only when volumes are explicitly deleted: `docker-compose down -v`
- Data survives container recreation during rebuilds

### Initial Data
On first run (empty database) the application seeds the game systems list (`InitialSystems` in `config/config.go`); admins can manage it afterwards.

Demo data is seeded only when `SEED_DEMO_DATA` is set to `1`/`true` (default off). It is meant for local development: it creates the users `testy`, `roy`, `adam` and `simon` with a password that is public in this repo (`InitialPasswords`), two sample campaigns ("Age of Sojourn" and "Embittered Arcanist") and Age of Sojourn's session history. The server logs a warning on every start while it is on. **Never enable it on a publicly reachable instance.**

## Authentication & Users

The application uses **local username/password accounts**. There is no self-registration: admins create users and hand out temporary passwords.

### How It Works

1. **Admin seeding** - On every startup the user named by `ADMIN_USERNAME` (default `admin`) is created if missing, unblocked, given the `admin` role and its password is reset to `ADMIN_PASSWORD`. This account is env-managed: its password can't be changed in the app, and it can't be blocked, deleted or demoted.
2. **Creating users** - On the Admin page an admin enters a username (2-32 characters: `a-z`, `0-9`, `_`, `-`; stored lowercase) and an optional name. The server generates a 12-character temporary password and shows it once.
3. **First login** - Users with a temporary password are redirected to `/change-password` and cannot use the rest of the app (API calls return `403 password change required`) until they pick a new password (at least 10 characters and at most 72 bytes, with an uppercase letter, a lowercase letter and a digit, not containing the username).
4. **Reset** - An admin can reset any other user's password from the Admin page. This generates a new temporary password and forces a change at the next login.
5. **Sessions** - The session cookie stores only the user ID (HttpOnly, SameSite=Lax, 30 days). Roles, blocked state and the forced-change flag are read from the database on every request.

### Auth Endpoints

| Endpoint | Description |
|----------|-------------|
| `POST /auth/login` | Body `{username, password}`; returns `{status, must_change_password}`. Every failure returns the same `401 invalid username or password` |
| `POST /auth/change-password` | Body `{current_password, new_password}`; requires a session; rejected for the env admin |
| `POST /auth/logout` | Clears the session |
| `GET /auth/current-user` | Current user info (`username`, `display_name`, `roles`, `must_change_password`, `is_env_admin`) |

### Admin API

| Endpoint | Description |
|----------|-------------|
| `GET /api/admin/users` | List users |
| `POST /api/admin/users` | Body `{username, name?}`; returns `201 {user, temp_password}`, `409` if the username is taken |
| `POST /api/admin/users/{id}/reset-password` | Returns `{temp_password}`; refused for the env admin and yourself |
| `POST /api/admin/users/{id}/block` / `unblock` | Block or unblock a user |
| `POST /api/admin/users/{id}/make-admin` / `remove-admin` | Change the admin role |
| `DELETE /api/admin/users/{id}` | Delete a user |

All admin endpoints require the `admin` role.

### Initial Setup

Set `ADMIN_PASSWORD` (and optionally `ADMIN_USERNAME`) in `.env`, start the app and log in as the admin. The database is wiped on every `./deploy.sh` (`down -v`), but the admin is recreated from `.env` on each start. Locally, delete `./data/waystone.db` after schema changes.

## User Roles & Permissions

The application implements a flexible, role-based access control (RBAC) system on top of local authentication. Users can be assigned one or more roles to control access to specific features and endpoints.

### Role Model

Roles are stored as an array of strings on each User. This allows a single user to have multiple roles (e.g., both "user" and "admin").

**Built-in Roles:**
- **`user`** - Regular user; can view events and sign up
- **`admin`** - Full system access; can manage users (create, reset passwords, block, delete, change admin role)
- **`dungeon-master`** - Custom role available for application-specific access control

Additional custom roles can be added to the `models/roles.go` file as needed.

### Permission Matrix

| Endpoint | Requires Auth | Requires Role | Description |
|----------|:-:|:-:|---|
| `POST /auth/login` | No | - | Log in with username and password |
| `POST /auth/change-password` | Yes | - | Change own password (not the env admin) |
| `GET /auth/current-user` | Yes | - | Get current user info with roles |
| `POST /auth/logout` | Yes | - | Clear session and logout |
| `GET /api/campaigns` | Yes | - | List campaigns |
| `/api/admin/users...` | Yes | admin | User management (see Admin API) |

### Role Assignment

Admins create users (role `user`) from the Admin page and can promote or demote other users with `make-admin` / `remove-admin`. Each protected endpoint checks the user's roles before processing the request.

### Implementation Details

- **Storage**: Roles are stored as JSON strings in SQLite alongside user data. No separate roles table is needed.
- **Session Management**: The session cookie holds only the user ID; roles are loaded from the database on each request.
- **Authorization Pattern**: Each handler explicitly checks roles using helper methods like `user.IsAdmin()` or `user.HasRole("admin")`.
- **Backward Compatibility**: Existing users with no roles are automatically handled; their role array defaults to empty.

## Troubleshooting

**Port already in use**: Change the port in `docker-compose.yml` or stop the conflicting service.

```yaml
ports:
  - "9090:8080"  # Access via http://localhost:9090
```

**Container won't start**: Check logs with:
```bash
docker-compose logs waystone-web
```

## Development

### Adding New API Endpoints

Edit `main.go` and add handlers:

```go
http.HandleFunc("/api/myendpoint", handleMyEndpoint)
```

### Rebuilding After Changes

```bash
docker-compose build --no-cache
docker-compose up -d
```

## Future Enhancements

- Event management dashboard (admin panel)
- Email confirmations for signups
- Admin panel for creating/editing events
- Signup list management and export (CSV/PDF)
- Data backup and recovery
- Role hierarchy (e.g., "super-admin" > "admin" > "moderator")
- Permission groups (granular permissions instead of just roles)
- Audit logs for role assignments and access attempts

## License

MIT
