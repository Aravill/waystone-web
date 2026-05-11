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
│   ├── auth.go          # OAuth authentication handlers
│   ├── events.go        # Event management handlers
│   ├── campaigns.go     # Campaign management handlers
│   ├── signup.go        # Signup handlers
│   └── roles.go         # Role management endpoints
├── db/                  # Database layer
│   ├── store.go         # SQLite storage interface and implementation
│   ├── sqlite_store.go  # SQLite database store implementation
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
    ├── login.html       # OAuth login page
    ├── styles.css       # Styling (black minimalistic design)
    └── script.js        # Client-side JavaScript (Fetch API)
```

## Features

- **Go Backend**: Lightweight HTTP server with event management APIs
- **SQLite Storage**: Persistent data storage for events and signups using SQLite
- **Web Frontend**: Responsive sign-up form with black minimalistic design and Fira Code monospace font
- **Docker Support**: Containerized for consistent deployment with persistent data volumes
- **Easy Deployment**: Simple deploy script for local Docker deployment

## Prerequisites

- Docker (version 20.10+)
- Docker Compose (version 1.29+)

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
go run main.go
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
| `GOOGLE_CLIENT_ID` | (empty) | No* | Google OAuth 2.0 Client ID from Google Cloud Console |
| `GOOGLE_CLIENT_SECRET` | (empty) | No* | Google OAuth 2.0 Client Secret from Google Cloud Console |
| `OAUTH_CALLBACK_URL` | `http://localhost:8080/auth/callback` | No | OAuth callback URL (must match registered URI in Google Console) |
| `SESSION_SECRET` | (random) | No | Secret key for signing session cookies (set to random value in production) |

*OAuth variables are optional for development. The app runs without OAuth credentials but OAuth login will be unavailable until configured.

### Setting Up Google OAuth

To enable Google login functionality:

1. Go to [Google Cloud Console](https://console.cloud.google.com)
2. Create a new project or select an existing one
3. Enable the Google+ API
4. Create OAuth 2.0 credentials:
   - Application type: Web Application
   - Authorized JavaScript origins: `http://localhost:8080` (development) or your domain
   - Authorized redirect URIs: `http://localhost:8080/auth/callback` (development) or your domain
5. Copy the Client ID and Client Secret
6. Update your `.env` file:
   ```
   GOOGLE_CLIENT_ID=your-client-id
   GOOGLE_CLIENT_SECRET=your-client-secret
   ```
7. Run `./deploy.sh` to apply the changes

### Example .env File

See `.env.example` in the repository for a complete template with all available variables and descriptions.

## Data Persistence

The application uses **SQLite** (embedded SQL database) to store all event and signup data. Data is persisted in a Docker volume (`waystone-data`) mounted at `/root/data` in the container.

### Data Storage Location
- Docker volume: `waystone-data`
- Container path: `/root/data/waystone.db`

### Data Preservation
- Data persists across container restarts: `docker-compose restart`
- Data is removed only when volumes are explicitly deleted: `docker-compose down -v`
- Data survives container recreation during rebuilds

### Initial Data
On first run, the application automatically seeds two sample events:
1. "Tech Conference 2024" (2024-05-10)
2. "Web Summit" (2024-06-15)

These can be replaced by modifying the `seedInitialEvents()` function in `main.go`.

## Authentication & Whitelist

The application uses **Google OAuth 2.0** for user authentication with a **whitelist-based access model**. Only users who have been explicitly created/approved by an administrator can log in.

### How It Works

1. **Whitelist Model** - Users must be pre-created by an admin before they can log in
2. **OAuth Login** - Users click "Login with Google" and authenticate with their Google account
3. **Email Verification** - System checks if the user's email exists in the whitelist (user database)
4. **Access Granted/Denied** - Whitelisted users proceed to the application; non-whitelisted users see an access denied error
5. **Auto-Update** - On first successful login, the user's Google ID is stored; subsequent logins reuse the existing user record

### Creating Users (Whitelist)

To add a user to the whitelist, call the user creation endpoint:

```bash
curl -X POST http://localhost:8080/api/users/create \
  -H "Content-Type: application/json" \
  -H "Cookie: <session_cookie>" \
  -d {
    "email": "user@example.com",
    "name": "User Name (optional)"
  }
```

**Requirements:**
- Caller must be authenticated (have a valid session cookie)
- Caller must have the `admin` role

**Response (201 Created):**
```json
{
  "status": "success",
  "user": {
    "email": "user@example.com",
    "name": "User Name",
    "created_at": "2026-05-04T22:00:00Z",
    "roles": []
  }
}
```

**Error Responses:**
- `303 See Other` - Not authenticated; redirects to /login.html
- `400 Bad Request` - User already exists or invalid email
- `403 Forbidden` - Authenticated but user is not an admin

### Initial Admin Setup

On first deployment, at least one admin must be created to whitelist other users. This can be done by:

1. **Option A: Manual Database Edit** - Edit the SQLite database directly and add a user record with admin role
2. **Option B: Bootstrap Email** (Future) - Set `BOOTSTRAP_ADMIN_EMAIL` environment variable for automatic first-user admin creation

### Login Flow

**Whitelisted User:**
```
Google OAuth → Email verified → Email in whitelist? YES → Login successful
```

**Non-Whitelisted User:**
```
Google OAuth → Email verified → Email in whitelist? NO → Access Denied page
                                                         ↓
                                    "Contact an administrator to request access"
```

## User Roles & Permissions

The application implements a flexible, role-based access control (RBAC) system using Google OAuth authentication. Users can be assigned one or more roles to control access to specific features and endpoints.

### Role Model

Roles are stored as an array of strings on each User. This allows a single user to have multiple roles (e.g., both "user" and "admin").

**Built-in Roles:**
- **`user`** - Regular user; can view events and sign up
- **`admin`** - Full system access; can assign roles, view all users, and manage other users' roles
- **`dungeon-master`** - Custom role available for application-specific access control

Additional custom roles can be added to the `models/roles.go` file as needed.

### Permission Matrix

| Endpoint | Requires Auth | Requires Role | Description |
|----------|:-:|:-:|---|
| `GET /auth/login` | No | - | OAuth login redirect to Google |
| `GET /auth/callback` | No | - | OAuth callback handler (checks whitelist) |
| `GET /auth/current-user` | Yes | - | Get current user info with roles |
| `POST /auth/logout` | Yes | - | Clear session and logout |
| `GET /api/events` | Yes | - | List all events (any authenticated user) |
| `POST /api/events` | Yes | - | Create new event (any authenticated user) |
| `POST /api/signup` | Yes | - | Sign up for an event (any authenticated user) |
| `POST /api/roles` | Yes | admin | Assign roles to a user by email |
| `GET /api/user-roles` | Yes | admin or self | Get roles for a user (admin or self-view) |
| `GET /api/users` | Yes | admin | List all users with their roles |
| `POST /api/users/create` | Yes | admin | Create/whitelist a new user (admin only) |

### Role Assignment Workflow

1. **Admin Whitelists User** - Admin calls `/api/users/create` to create a user record with the user's email
2. **User Logs In** - User clicks "Login with Google" and authenticates via Google OAuth
3. **Whitelist Check** - System verifies user's email is in the whitelist (user exists in database)
4. **Account Updated** - User's Google ID is stored and roles are loaded from database
5. **Admin Assigns Roles** - Admin can call `/api/roles` endpoint to assign additional roles (e.g., "admin") to the user
6. **Authorization Checks** - Each protected endpoint checks the user's roles before processing the request

### API Endpoints for Role Management

#### Assign Roles to a User

```bash
curl -X POST http://localhost:8080/api/roles \
  -H "Content-Type: application/json" \
  -d {
    "email": "user@example.com",
    "roles": ["user", "admin"]
  }
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Roles updated for user@example.com"
}
```

**Error Responses:**
- `401 Unauthorized` - Not logged in
- `303 See Other` - Redirects to /login.html if not authenticated
- `403 Forbidden` - Logged in but user is not an admin
- `400 Bad Request` - Invalid request body

#### Get Current User's Roles

```bash
curl http://localhost:8080/api/user-roles
```

**Response (200 OK):**
```json
{
  "email": "user@example.com",
  "roles": ["user", "admin"]
}
```

**Error Responses:**
- `401 Unauthorized` - Not logged in
- `303 See Other` - Redirects to /login.html if not authenticated

#### List All Users and Their Roles

```bash
curl http://localhost:8080/api/users
```

**Response (200 OK):**
```json
[
  {
    "email": "user1@example.com",
    "name": "User One",
    "roles": ["user"]
  },
  {
    "email": "admin@example.com",
    "name": "Admin User",
    "roles": ["user", "admin"]
  }
]
```

**Requires:** Admin role
**Error Responses:**
- `401 Unauthorized` - Not logged in
- `303 See Other` - Redirects to /login.html if not authenticated
- `403 Forbidden` - Logged in but user is not an admin

### Getting Started with Roles

1. **First Login** - Use Google OAuth to log in. Your account will be created with no roles.
2. **Manual Admin Setup** - Add the first admin user by manually editing the SQLite database or asking an existing admin to assign the "admin" role.
3. **Role Assignment** - Once an admin exists, they can use the `/api/roles` endpoint to assign roles to other users.
4. **Verify Permissions** - Test role-based access control by attempting to access protected endpoints with different users and roles.

### Implementation Details

- **Storage**: Roles are stored as JSON strings in SQLite alongside user data. No separate roles table is needed.
- **Session Management**: Roles are included in the session cookie and persist across authenticated requests.
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
