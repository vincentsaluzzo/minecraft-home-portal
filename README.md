# Minecraft Home Portal

Minecraft Home Portal is a small Go web service for managing a homelab fleet of Minecraft servers running as Docker containers.

It is intentionally simple:

- Docker labels are the source of truth for server discovery
- SQLite stores users, sessions, and audit logs
- RCON is used for Minecraft administration
- One web app provides dashboard, detail pages, auth, and admin actions

## Current Scope

The current scaffold already includes:

- label-based discovery of Minecraft containers
- public vs private visibility
- login with `admin` and `viewer` roles
- bootstrap admin creation from environment variables
- dashboard and per-server detail page
- Docker actions:
  - start
  - stop
  - restart
- RCON actions:
  - `op`
  - `deop`
  - `say`
- audit logging for admin actions
- polling-based status refresh

The app also attempts to refresh player presence using the RCON `list` command when RCON is configured.

## Stack

- Go `1.26`
- `net/http`
- server-rendered `html/template`
- SQLite via `modernc.org/sqlite`
- Docker Engine Go client
- `github.com/gorcon/rcon`

## Project Layout

- [PLAN.md](/Users/vincent.saluzzo/PERSONNEL/minecraft-home-portal/PLAN.md)
- [cmd/mcportal/main.go](/Users/vincent.saluzzo/PERSONNEL/minecraft-home-portal/cmd/mcportal/main.go)
- [internal/app/app.go](/Users/vincent.saluzzo/PERSONNEL/minecraft-home-portal/internal/app/app.go)
- [internal/discovery/service.go](/Users/vincent.saluzzo/PERSONNEL/minecraft-home-portal/internal/discovery/service.go)
- [internal/minecraft/rcon.go](/Users/vincent.saluzzo/PERSONNEL/minecraft-home-portal/internal/minecraft/rcon.go)
- [internal/store/store.go](/Users/vincent.saluzzo/PERSONNEL/minecraft-home-portal/internal/store/store.go)

## Discovery Model

The portal discovers any container that has:

```yaml
mcportal.enabled: "true"
```

### Supported Labels

```yaml
mcportal.enabled: "true"
mcportal.name: "Survival"
mcportal.kind: "minecraft-java"
mcportal.visibility: "public"
mcportal.connect.host: "mc.example.com"
mcportal.connect.port: "25565"
mcportal.connect.version: "1.21.x"
mcportal.connect.notes: "Vanilla survival"
mcportal.rcon.host: "mc-survival"
mcportal.rcon.port: "25575"
mcportal.rcon.password-env: "SURVIVAL_RCON_PASSWORD"
mcportal.actions.start: "true"
mcportal.actions.stop: "true"
mcportal.actions.restart: "true"
mcportal.actions.op: "true"
mcportal.actions.deop: "true"
mcportal.actions.say: "true"
```

### Visibility Rules

- `public`: visible without login
- `private`: visible only to authenticated users

### Secrets

Do not put secrets in labels.

For RCON, the portal currently expects:

- a label like `mcportal.rcon.password-env=SURVIVAL_RCON_PASSWORD`
- and an environment variable with that name inside the portal container

## Example Minecraft Service

```yaml
services:
  mc-survival:
    image: itzg/minecraft-server:latest
    container_name: mc-survival
    ports:
      - "25565:25565"
      - "25575:25575"
    environment:
      EULA: "TRUE"
      ENABLE_RCON: "true"
      RCON_PASSWORD: "${SURVIVAL_RCON_PASSWORD}"
    labels:
      mcportal.enabled: "true"
      mcportal.name: "Survival"
      mcportal.kind: "minecraft-java"
      mcportal.visibility: "public"
      mcportal.connect.host: "mc.example.com"
      mcportal.connect.port: "25565"
      mcportal.connect.version: "1.21.x"
      mcportal.connect.notes: "Vanilla survival"
      mcportal.rcon.host: "mc-survival"
      mcportal.rcon.port: "25575"
      mcportal.rcon.password-env: "SURVIVAL_RCON_PASSWORD"
```

The portal must be able to reach:

- the Docker socket
- the RCON endpoint for each server

## Running Locally

### 1. Bootstrap an admin

```bash
export MCPORTAL_BOOTSTRAP_ADMIN_USERNAME=admin
export MCPORTAL_BOOTSTRAP_ADMIN_PASSWORD=change-me
```

### 2. Point the app at Docker

For a local engine:

```bash
export DOCKER_HOST=unix:///var/run/docker.sock
```

### 3. Optionally export RCON secrets used by label references

```bash
export SURVIVAL_RCON_PASSWORD=your-rcon-password
```

### 4. Run

```bash
go run ./cmd/mcportal
```

Open [http://localhost:8080](http://localhost:8080).

## Docker Deployment

The repository includes [docker-compose.yml](/Users/vincent.saluzzo/PERSONNEL/minecraft-home-portal/docker-compose.yml) for the portal itself.

Start it with:

```bash
docker compose up --build
```

Default bootstrap credentials in that file are placeholders only. Change them before real use.

### Example Compose Install

If you want to deploy from a published image instead of building locally, use a compose file like this:

```yaml
services:
  mcportal:
    image: ghcr.io/vincentsaluzzo/minecraft-home-portal:latest
    container_name: mcportal
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      MCPORTAL_ADDR: ":8080"
      MCPORTAL_DATA_DIR: "/app/data"
      MCPORTAL_SESSION_COOKIE_NAME: "mcportal_session"
      MCPORTAL_DISCOVERY_REFRESH: "30s"
      MCPORTAL_LABEL_NAMESPACE: "mcportal"
      MCPORTAL_BOOTSTRAP_ADMIN_USERNAME: "${MCPORTAL_BOOTSTRAP_ADMIN_USERNAME}"
      MCPORTAL_BOOTSTRAP_ADMIN_PASSWORD: "${MCPORTAL_BOOTSTRAP_ADMIN_PASSWORD}"
      DOCKER_HOST: "unix:///var/run/docker.sock"

      # Example RCON secret used by a discovered server label such as:
      # mcportal.rcon.password-env=SURVIVAL_RCON_PASSWORD
      SURVIVAL_RCON_PASSWORD: "${SURVIVAL_RCON_PASSWORD}"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./data:/app/data
```

Example `.env` file:

```dotenv
MCPORTAL_BOOTSTRAP_ADMIN_USERNAME=admin
MCPORTAL_BOOTSTRAP_ADMIN_PASSWORD=change-me-now
SURVIVAL_RCON_PASSWORD=replace-with-your-rcon-password
```

Then start it with:

```bash
docker compose up -d
```

The portal will be available at [http://localhost:8080](http://localhost:8080).

Important notes:

- the portal needs access to `/var/run/docker.sock` to discover and control containers
- any RCON password referenced by `mcportal.rcon.password-env` must also be present in the portal container environment
- for internet-facing deployments, put the portal behind a reverse proxy and consider a Docker socket proxy instead of mounting the raw socket directly

## GitHub Actions

The repository includes [`.github/workflows/ci.yml`](/Users/vincent.saluzzo/PERSONNEL/minecraft-home-portal/.github/workflows/ci.yml).

It currently does this:

- runs `go mod tidy`, `go build ./...`, and `go test ./...`
- builds a multi-arch Docker image for `linux/amd64` and `linux/arm64`
- pushes images to `ghcr.io/vincentsaluzzo/minecraft-home-portal` on pushes to `main` and version tags
- builds without pushing on pull requests

## Environment Variables

- `MCPORTAL_ADDR`
- `MCPORTAL_DATA_DIR`
- `MCPORTAL_DATABASE_PATH`
- `MCPORTAL_SESSION_COOKIE_NAME`
- `MCPORTAL_SESSION_TTL`
- `MCPORTAL_DISCOVERY_REFRESH`
- `MCPORTAL_LABEL_NAMESPACE`
- `MCPORTAL_BOOTSTRAP_ADMIN_USERNAME`
- `MCPORTAL_BOOTSTRAP_ADMIN_PASSWORD`
- `DOCKER_HOST`

## Verification

The current scaffold has been verified with:

```bash
env GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go build ./...
env GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
```

## Next Recommended Steps

- add CSRF protection for admin forms
- move from periodic polling to event-assisted Docker refresh
- add a dedicated audit log page
- add a bootstrap CLI for creating additional users
- add richer server status detection beyond RCON `list`
