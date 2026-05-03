# Neith Config — Implementation Task List

## 1. Sidecar Agent (`neith`)

### 1.1 Core Process
- [ ] Implement agent as a minimal long-running daemon (single binary, no runtime deps)
- [ ] On startup, resolve Hub address from env/flag (`NEITH_HUB_URL`)
- [ ] On startup, resolve service token from env/flag (`NEITH_TOKEN`)
- [ ] Connect to Local Hub via SSE (Server-Sent Events) stream
- [ ] Parse incoming SSE events and extract config payload
- [ ] Atomically write received config snapshot to `/dev/shm/neith-<service>.json`
  - [ ] Write to temp file first, then `rename()` for atomicity
- [ ] Maintain an `mmap`-backed version counter (8-byte integer) in `/dev/shm/neith-<service>.version`
  - [ ] Increment counter atomically on every config update
- [ ] On SSE disconnect: reconnect with exponential backoff + random jitter
- [ ] Run background Jitter Polling loop as a fallback alongside SSE
  - [ ] Poll Hub on a randomized interval (base interval + random offset)
  - [ ] Apply config from poll response if version is newer than current
- [ ] Graceful shutdown: handle SIGTERM/SIGINT, flush state, exit cleanly

### 1.2 Wrapper Mode (`neith run -- <command>`)
- [x] Parse `--` separator and treat everything after it as the child command
- [x] Fetch config before starting the child process via `EnvFetcher` abstraction
- [ ] Implement real Hub HTTP client to resolve config snapshot for child process startup
- [x] Inject config key-value pairs as OS environment variables into the child process
- [x] Start child process with the injected environment
- [ ] Watch for config change events from Hub while child is running
- [ ] On config change: send SIGTERM to child process, wait for exit, restart with new env
- [x] Forward child process stdout/stderr to agent stdout/stderr
- [x] Propagate child exit code to the agent process exit code

### 1.3 Hot-Reload Mode (SDK integration)
- [ ] Expose `neith.Getenv(key string) string` function in Go SDK
  - [ ] On call: check version counter via `mmap`; if changed, re-read JSON from `/dev/shm`
  - [ ] Return value from in-memory parsed snapshot
- [ ] Thread-safe snapshot reads (use `sync/atomic` or `RWMutex` appropriately)
- [ ] Export SDK as a standalone Go module (`go.mod`, versioned releases)

### 1.4 Agent Configuration
- [x] Support CLI flags for run-mode behavior (`--no-env`, `--json-output`, `--verbose`, `--version`, `--help`)
- [ ] Support CLI flags for Hub connectivity (`--hub-url`, `--token`, `--poll-interval`)
- [ ] Support configuration via environment variables (`NEITH_HUB_URL`, `NEITH_TOKEN`, etc.)
- [ ] Document all configuration options

---

## 2. Local Hub (Proxy)

### 2.1 Cloud Connection
- [ ] Connect to Cloud Server via SSE using a single service token
- [ ] Receive config state updates from Cloud Server
- [ ] Store latest cloud state in memory

### 2.2 Override Merging
- [ ] Load `override.yaml` from a configurable path on startup
- [ ] Watch `override.yaml` for changes (inotify or polling)
- [ ] On each request from a local agent: deep-merge cloud state with `override.yaml`
- [ ] `override.yaml` values take precedence over cloud state

### 2.3 Agent API (no auth)
- [ ] Serve SSE endpoint for local agents (no authentication required)
- [ ] Push merged config to connected agents on every cloud or override change
- [ ] Serve HTTP endpoint for one-shot config polling (for Jitter Polling)
- [ ] Return current merged snapshot as JSON

### 2.4 Deployment
- [ ] Provide `docker-compose` snippet for running Hub locally
- [ ] Document token configuration for developers

---

## 3. Cloud Server (Master)

> See **Cloud Server** section below for full description.

---

## 4. Application SDK

### 4.1 Go SDK
- [ ] `neith.Getenv(key string) string` — hot-reload-aware env getter
- [ ] `neith.MustGetenv(key string) string` — panics if key is missing
- [ ] `neith.GetenvWithDefault(key, fallback string) string`
- [ ] Internal: open and `mmap` the version file on first call
- [ ] Internal: re-parse `/dev/shm` JSON snapshot when version changes
- [ ] Publish as versioned Go module

### 4.2 PHP SDK
- [ ] PSR-15 Middleware that overwrites `$_ENV` on every request
- [ ] Read latest snapshot from agent's `/dev/shm` file (shared via volume mount)
- [ ] Support Yii3 integration
- [ ] Publish as Composer package

---

## 5. Reliability & Delivery Guarantees

- [ ] SSE Reconnect: re-establish SSE connection on any network error
- [ ] Jitter Polling: periodic HTTP poll with randomized interval to survive SSE gaps
- [ ] Atomic file writes: always use `rename()` so readers never see partial writes
- [ ] Version counter: allows SDK to detect changes in O(1) without parsing JSON each time
- [ ] Startup bootstrap: agent fetches full snapshot via HTTP before subscribing to SSE

---

## 6. Licensing & Distribution

- [ ] Agent binary: **MIT** license
- [ ] Go SDK: **MIT** license
- [ ] PHP SDK: **MIT** license
- [ ] Base Cloud Server (self-hosted OSS): **Apache 2.0** license
- [ ] Enterprise/SaaS features (SSO, RBAC, deploy approvals, audit logs): proprietary / commercial license
- [ ] Set up GitHub repositories and CI/CD pipelines for each component
- [ ] Publish agent Docker image to Docker Hub / GHCR
- [ ] Publish Go module to pkg.go.dev
- [ ] Publish PHP package to Packagist

---

---

# Neth Server (Cloud Master) — System Description

> This section describes the Cloud Server component of the Neith architecture — the authoritative source of truth for all configuration state.

## Overview

The Cloud Server is a centralized, multi-tenant SaaS (or self-hosted) service that stores, versions, and distributes configuration to all Local Hubs and optionally directly to Sidecar Agents. It is the only component with write access to the configuration database.

## Storage

- Configuration state is stored in **PostgreSQL** using **JSONB** columns for flexible, schemaless config payloads.
- Each service's config is a versioned document. New writes append a new version; old versions are retained for audit and rollback.
- Full audit log of every change: who changed what, when, and from which IP/token.

## Distribution

- Pushes config updates to connected Local Hubs via **Server-Sent Events (SSE)**.
- Each Hub authenticates with a single long-lived token.
- On SSE connect: sends the current full snapshot immediately, then streams diffs as events.
- Supports direct agent connections for environments without a Local Hub.

## Access Control

### OSS / Base Tier
- Token-based authentication for Hubs and agents.
- Basic namespacing: configs scoped to `project` / `environment` / `service`.

### Enterprise Tier (closed source)
- **SSO / SAML** integration for team login.
- **RBAC** (Role-Based Access Control): viewer, editor, approver, admin roles per project.
- **Deploy Approvals**: changes to production configs require explicit approval from an authorized user before being pushed to Hubs.
- **Extended Audit Logs**: full change history with user identity, approval chain, and rollback capability.
- **IP Allowlisting** for Hub connections.

## API Surface

- `GET  /sse/v1/stream?token=...` — SSE stream for Hub/agent subscriptions
- `GET  /api/v1/snapshot?token=...` — one-shot full snapshot (for Jitter Polling and bootstrap)
- `POST /api/v1/config/:project/:env/:service` — write a new config version (authenticated, authorized)
- `GET  /api/v1/config/:project/:env/:service/history` — list config versions
- `POST /api/v1/config/:project/:env/:service/rollback/:version` — roll back to a previous version

## Reliability

- Horizontally scalable: multiple server instances behind a load balancer.
- SSE fan-out: use a pub/sub broker (Redis Streams or PostgreSQL `LISTEN/NOTIFY`) to propagate writes across server instances to all connected Hub SSE streams.
- Hub reconnects are handled gracefully: on reconnect, Hub receives the latest full snapshot.
