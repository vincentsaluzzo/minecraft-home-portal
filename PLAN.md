# Minecraft Home Portal Plan

## Goal

Build a single Dockerized web service that centralizes Minecraft server status and basic administration for a homelab setup. The service will discover servers declaratively from Docker labels, expose read-only/public views when enabled, and provide authenticated admin actions with audit logging.

## Product Scope

### In Scope for v1

- Docker-label-based discovery of Minecraft server containers
- Dashboard listing all discovered servers
- Per-server detail page with connection instructions
- Authenticated login
- Two roles: `admin` and `viewer`
- Anonymous access only to servers explicitly marked public
- Admin actions:
  - start
  - stop
  - restart
  - `op`
  - `deop`
  - `list`
  - `say`
- SQLite persistence for users, sessions, settings, and audit logs
- Basic polling-based status refresh

### Out of Scope for v1

- SSO / OIDC
- Fine-grained per-server permissions
- Live logs streaming
- Metrics / charts
- Full Docker auto-provisioning of Minecraft instances
- Bedrock-specific features beyond connection metadata

## Architecture

### Service Shape

- Single Go web application
- Server-rendered HTML with minimal JavaScript
- SQLite for application state
- Docker Engine API for discovery and lifecycle control
- RCON for Minecraft in-game administration

### Main Modules

- `cmd/mcportal`
  - process bootstrap
- `internal/app`
  - wiring, routes, middleware
- `internal/config`
  - environment/config parsing
- `internal/discovery`
  - Docker label parsing and refresh
- `internal/dockerctl`
  - container lifecycle actions
- `internal/minecraft`
  - RCON command execution
- `internal/auth`
  - login, sessions, RBAC checks
- `internal/store`
  - SQLite access and migrations
- `web/templates`
  - HTML templates
- `web/static`
  - CSS and tiny JS

## Declarative Discovery Model

### Required Labels

- `mcportal.enabled=true`
- `mcportal.name`
- `mcportal.visibility=private|public`

### Recommended Labels

- `mcportal.kind=minecraft-java`
- `mcportal.connect.host`
- `mcportal.connect.port`
- `mcportal.connect.version`
- `mcportal.connect.notes`
- `mcportal.rcon.host`
- `mcportal.rcon.port`
- `mcportal.actions.start=true|false`
- `mcportal.actions.stop=true|false`
- `mcportal.actions.restart=true|false`
- `mcportal.actions.op=true|false`
- `mcportal.actions.deop=true|false`
- `mcportal.actions.say=true|false`

### Secret Handling

- Never place RCON passwords in labels
- Read RCON passwords from:
  - environment variables
  - Docker secrets
  - mounted files

## Data Model

### Tables

- `users`
  - id
  - username
  - password_hash
  - role
  - created_at
- `sessions`
  - id
  - user_id
  - token_hash
  - expires_at
  - created_at
- `audit_logs`
  - id
  - actor_user_id nullable
  - action
  - target_type
  - target_id
  - metadata_json
  - created_at
- `settings`
  - key
  - value

## HTTP Surface

### Public/Anonymous

- `GET /`
- `GET /servers/:id`

### Auth

- `GET /login`
- `POST /login`
- `POST /logout`

### Admin

- `POST /servers/:id/start`
- `POST /servers/:id/stop`
- `POST /servers/:id/restart`
- `POST /servers/:id/rcon/op`
- `POST /servers/:id/rcon/deop`
- `POST /servers/:id/rcon/say`

### Internal

- `GET /healthz`

## Milestones

### 1. Foundation

- Create repository layout
- Add Go module and dependencies
- Add app config and startup flow
- Add Dockerfile and sample compose for the portal itself

### 2. Persistence and Auth

- Add SQLite store
- Add schema migration bootstrapping
- Add password hashing
- Add cookie-backed sessions
- Add admin/viewer authorization middleware

### 3. Discovery and Status

- Add Docker client bootstrap
- Discover containers by label
- Parse label metadata into server records
- Render dashboard and details pages

### 4. Actions

- Add container start/stop/restart actions
- Add RCON client integration
- Add audit logging for each action

### 5. Polish

- Add public/private filtering
- Add form validation and error rendering
- Add basic styling
- Add bootstrap path for initial admin user

### 6. Documentation

- Write `README.md` last, after the scaffold and first working slice are in place
- Document:
  - stack
  - architecture
  - labels
  - env vars
  - local run
  - Docker deployment
  - bootstrap admin flow

## Execution Order For This Iteration

1. Write this plan
2. Scaffold the Go project and Docker packaging
3. Implement config, server discovery model, and basic HTTP app shell
4. Implement SQLite store and auth/session primitives
5. Add initial UI for login, dashboard, and server detail
6. Add first admin lifecycle actions and audit log hooks
7. Write `README.md`
