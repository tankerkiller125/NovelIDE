# NovelIDE Sync Server (optional)

NovelIDE is local-first: your workspaces are plain folders on disk and the app
works **fully offline with no account and no server**. If you want to sync a
workspace across several devices, you can run this small, self-hostable sync
server and point the app at it. It is entirely optional and opt-in.

- **Multi-account** — one server hosts many users; each account's workspaces are
  isolated on disk and unreachable by other accounts.
- **Plain-file friendly** — a workspace syncs as a set of files, each addressed
  by the SHA-256 of its contents. Only changed files move over the wire.
- **Small & dependency-light** — pure Go, no cgo, no external database; metadata
  is JSON and file contents are content-addressed blobs on disk.

## Running it

### Docker Compose (recommended)

```bash
cd build/docker
docker compose up -d
```

The server listens on `:8787` and stores data in the `novelide-sync-data`
volume. Point the desktop app at `http://<host>:8787` and register an account.

### Docker

```bash
docker build -f build/docker/Dockerfile.sync -t novelide-sync .
docker run -d -p 8787:8787 -v novelide-sync-data:/data novelide-sync
```

### From source

```bash
go run ./cmd/novelide-sync
# or a static binary:
CGO_ENABLED=0 go build -o novelide-sync ./cmd/novelide-sync && ./novelide-sync
```

## Configuration

All via environment variables; all optional.

| Variable | Default | Purpose |
| --- | --- | --- |
| `NOVELIDE_SYNC_ADDR` | `:8787` | Listen address |
| `NOVELIDE_SYNC_DATA` | `./sync-data` (`/data` in Docker) | Data directory |
| `NOVELIDE_SYNC_SECRET` | generated & persisted | HMAC secret for signing auth tokens |
| `NOVELIDE_SYNC_ALLOW_REGISTRATION` | `true` | Allow new sign-ups |
| `NOVELIDE_SYNC_MAX_BLOB_MB` | `100` | Max size of a single uploaded file |
| `NOVELIDE_SYNC_TOKEN_DAYS` | `30` | Auth-token lifetime |

### Authentication mode & SSO

By default the server uses username/password. You can disable that and/or add
single sign-on against any OpenID Connect provider (Zitadel, Authentik,
Keycloak, …).

| Variable | Default | Purpose |
| --- | --- | --- |
| `NOVELIDE_SYNC_AUTH_MODE` | `password` | `password`, `sso`, or `both` |
| `NOVELIDE_SYNC_PUBLIC_URL` | — | The server's public base URL (**required for SSO**) |
| `NOVELIDE_SYNC_OIDC_ISSUER` | — | Provider issuer URL |
| `NOVELIDE_SYNC_OIDC_CLIENT_ID` | — | OIDC client id |
| `NOVELIDE_SYNC_OIDC_CLIENT_SECRET` | — | OIDC client secret |
| `NOVELIDE_SYNC_OIDC_SCOPES` | `openid profile email` | Requested scopes |
| `NOVELIDE_SYNC_OIDC_NAME` | `SSO` | Button label shown in the app |

- Set `NOVELIDE_SYNC_AUTH_MODE=sso` to turn off password sign-up/login entirely,
  or `both` to offer both.
- Register **`<PUBLIC_URL>/api/sso/callback`** as an allowed redirect URI in your
  provider, and create a **confidential** client (client id + secret).
- Accounts are keyed by the provider's stable subject (`sub`) and provisioned
  automatically on first sign-in.
- How it works: the server runs the OIDC Authorization Code flow with PKCE,
  exchanges the code with its client secret, and reads identity from the
  provider's UserInfo endpoint over TLS. The desktop app never handles the OIDC
  exchange — it opens the system browser and receives the server's own session
  token on a loopback redirect. (This uses UserInfo rather than verifying the
  ID-token JWT; run the provider over HTTPS.)
- SSO keeps sign-in state in memory, so run a single instance (or sticky
  sessions) if you enable it.

## Security notes

- **Put it behind TLS.** The server speaks plain HTTP; terminate TLS at a
  reverse proxy (Caddy, nginx, Traefik) for anything reachable off localhost.
- **Set `NOVELIDE_SYNC_SECRET`** to a long random string in production so tokens
  survive restarts and can't be forged. Rotating it invalidates all tokens.
- **Disable registration** (`NOVELIDE_SYNC_ALLOW_REGISTRATION=false`) once your
  accounts exist, so the server isn't an open sign-up.
- Passwords are stored with bcrypt; auth is a stateless HMAC-signed bearer token.

## Data layout

```
<data>/
  accounts.json                          all accounts (bcrypt password hashes)
  secret.key                             generated token secret (if not set via env)
  accounts/<accountID>/
    workspaces.json                      that account's workspace list
    ws/<workspaceID>/
      manifest.json                      { revision, files: [{path, hash, size}] }
      blobs/<sha256>                     file contents, deduplicated
```

Because contents are content-addressed, you can back the whole thing up by
copying the data directory.

## HTTP API

All bodies are JSON except blob transfers (raw bytes). Authenticated routes take
`Authorization: Bearer <token>`.

| Method & path | Auth | Purpose |
| --- | --- | --- |
| `GET /healthz` | — | Liveness check |
| `GET /api/auth/config` | — | Which methods are enabled → `{passwordEnabled, ssoEnabled, ssoName}` |
| `POST /api/register` | — | Create a password account → `{token, accountId, username}` |
| `POST /api/login` | — | Log in → `{token, accountId, username}` |
| `GET /api/sso/start` | — | Begin OIDC sign-in (redirects to the provider) |
| `GET /api/sso/callback` | — | OIDC redirect target (hands the token back to the app) |
| `GET /api/me` | ✔ | The authenticated account → `{accountId, username}` |
| `GET /api/workspaces` | ✔ | List the account's workspaces |
| `GET /api/workspaces/{id}/manifest` | ✔ | Current `{revision, files}` |
| `POST /api/workspaces/{id}/commit` | ✔ | Set the file set (see below) |
| `HEAD /api/workspaces/{id}/blobs/{hash}` | ✔ | Does the server have this blob? |
| `PUT /api/workspaces/{id}/blobs/{hash}` | ✔ | Upload a blob (contents verified against the hash) |
| `GET /api/workspaces/{id}/blobs/{hash}` | ✔ | Download a blob |

### Sync model

A workspace has a monotonic **revision**. To push:

1. Compute the local manifest (path → SHA-256).
2. `PUT` any blobs the server lacks (check with `HEAD`, or let commit tell you).
3. `POST /commit` with `baseRevision` = the revision you last synced from.
   - **Success** → `{revision: N+1}`.
   - **`422` with `missing: [...]`** → upload those blobs and retry.
   - **`409` with `conflict: true, current: {...}`** → another device pushed
     first; merge against `current` and retry with its revision as `baseRevision`.

To pull, `GET /manifest`, then `GET` any blobs whose hash you don't already have
locally, and write them to their paths. Use `baseRevision: -1` to create a new
workspace or to force-overwrite regardless of revision.

## Using it from the desktop app

Open **Settings → Sync** (it's clearly marked *optional*):

1. Enter your server URL and **Connect**. The app then shows whatever the server
   allows: a password form, a **Sign in with …** SSO button, or both. (SSO opens
   your browser and returns automatically.)
1. Create an account or log in.
2. With a workspace open, click **Sync now** — the first sync links the
   workspace to a remote (named after its folder) and pushes it. After that, a
   **⟳** button appears in the sidebar footer for one-click sync.
3. On another device, open Settings → Sync, sign in with the same account, open
   (or create) the workspace, and either **Sync now** or pick the workspace
   under **Remote workspaces → Link & pull here** to download it.

Credentials are stored in the app settings; each workspace's link is stored in
its `.novelide/sync.json` (which, like snapshots, is never itself synced).

### Conflicts

Sync merges at the file level with a three-way merge. If the same file changed
on two devices, your local version is kept in place and the other device's
version is saved next to it as `name (conflict <timestamp>).ext` — nothing is
ever overwritten or lost. Resolve by comparing the two files and deleting the
conflict copy.

The engine that does this lives in `internal/syncclient`; the app bindings are
`Sync*`/`RemoteWorkspaces` in `app.go`.
