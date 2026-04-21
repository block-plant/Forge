# Forge — Project Documentation

> A complete Backend-as-a-Service (BaaS) platform built entirely from scratch in Go.  
> Zero external dependencies. Every byte understood.

---

## Table of Contents

1. [What is Forge?](#what-is-forge)
2. [Why Does It Exist?](#why-does-it-exist)
3. [Architecture Overview](#architecture-overview)
4. [Services Deep Dive](#services-deep-dive)
   - [HTTP Server](#1-http-server)
   - [Authentication](#2-authentication)
   - [Database](#3-database)
   - [Real-Time (WebSocket)](#4-real-time-websocket)
   - [Storage](#5-storage)
   - [Serverless Functions](#6-serverless-functions)
   - [Hosting](#7-hosting)
   - [Analytics](#8-analytics)
   - [Security Rules](#9-security-rules)
   - [Admin Dashboard](#10-admin-dashboard)
5. [TypeScript SDK](#typescript-sdk)
6. [Configuration](#configuration)
7. [Data Model](#data-model)
8. [Project Structure](#project-structure)

---

## What is Forge?

Forge is a self-hosted replacement for Firebase / Supabase. It gives you:

- **User authentication** with JWT tokens, refresh flows, OAuth (Google/GitHub), and admin controls
- **A document database** with collections, queries, indexes, transactions, and batch writes
- **File storage** with uploads, downloads, chunked uploads, signed URLs, and byte-range streaming
- **Real-time WebSockets** for live data sync, pub/sub channels, and presence
- **Serverless functions** you can deploy, invoke over HTTP, and schedule with cron
- **Static site hosting** with a built-in CDN cache, gzip compression, and SPA support
- **Analytics** for tracking events, sessions, and time-series aggregation
- **Declarative security rules** with a custom DSL, lexer, parser, and evaluator

All of this compiles into **a single binary** and runs with **zero external dependencies** — no Redis, no PostgreSQL, no Docker required.

---

## Why Does It Exist?

Forge was built as a deep engineering exercise to prove that an entire cloud backend can be constructed from first principles:

- The **HTTP server** is raw TCP sockets + hand-rolled HTTP parsing (no `net/http`).
- The **WebSocket engine** implements RFC 6455 from raw bytes.
- **Password hashing** uses a from-scratch Blowfish/bcrypt implementation.
- The **database** uses an in-memory B-Tree with Write-Ahead Logging (WAL).
- The **security rules** engine has its own lexer → parser → AST → evaluator pipeline.

The `go.mod` file declares **zero third-party modules**.

---

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────────┐
│                         forge (binary)                           │
│                                                                  │
│  ┌───────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────┐   │
│  │   Auth    │  │ Database │  │ Storage  │  │  Functions   │   │
│  │ JWT+OAuth │  │ B-Tree   │  │ Blob     │  │  Runtime     │   │
│  │ bcrypt    │  │ WAL+Snap │  │ Chunks   │  │  Scheduler   │   │
│  │ Sessions  │  │ Query    │  │ Signed   │  │  Triggers    │   │
│  │ Tokens    │  │ Indexes  │  │ Streams  │  │  Logs        │   │
│  └───────────┘  └──────────┘  └──────────┘  └──────────────┘   │
│                                                                  │
│  ┌───────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────┐   │
│  │ Real-Time │  │ Hosting  │  │Analytics │  │  Dashboard   │   │
│  │ WebSocket │  │ CDN+SSL  │  │Collector │  │  Embedded    │   │
│  │ Pub/Sub   │  │ Sites    │  │Time Ser. │  │  HTML/JS/CSS │   │
│  │ Streams   │  │ SPA Mode │  │Aggregator│  │              │   │
│  └───────────┘  └──────────┘  └──────────┘  └──────────────┘   │
│                                                                  │
│  ┌───────────────────────────────────────────────────────────┐   │
│  │  Server Core: TCP Listener → HTTP Parser → Router →      │   │
│  │  Middleware Chain → Context → Response Writer              │   │
│  └───────────────────────────────────────────────────────────┘   │
│                                                                  │
│  ┌───────────────────────────────────────────────────────────┐   │
│  │  Rules Engine: Lexer → Parser → AST → Evaluator           │   │
│  └───────────────────────────────────────────────────────────┘   │
│                                                                  │
│  ┌─────────────────────┐ ┌─────────────┐ ┌──────────────────┐   │
│  │ Config (JSON + ENV) │ │  Logger     │ │ Utils (crypto,   │   │
│  │                     │ │             │ │  json, time, etc)│   │
│  └─────────────────────┘ └─────────────┘ └──────────────────┘   │
└──────────────────────────────────────────────────────────────────┘
                              │
                    forge-data/ (on disk)
                    ├── auth/         (users, tokens, keys)
                    ├── dynamicdb/    (WAL, snapshots, collections)
                    ├── storage/      (objects, metadata)
                    ├── functions/    (bundles, logs)
                    ├── hosting/      (deployed sites)
                    └── analytics/    (event logs)
```

---

## Services Deep Dive

### 1. HTTP Server

**Location:** `server/`

Forge doesn't use Go's `net/http`. It opens a raw TCP listener and parses HTTP/1.1 requests byte-by-byte.

| File | Purpose |
|------|---------|
| `tcp.go` | TCP listener, connection accept loop, graceful shutdown |
| `http_parser.go` | Hand-rolled HTTP request parser (method, path, headers, body) |
| `router.go` | Trie-based URL router with path params (`:id`) and wildcards (`*path`) |
| `context.go` | Per-request context object with helpers (JSON parsing, query params, auth) |
| `response.go` | HTTP response builder with status codes, headers, and body |
| `middleware.go` | CORS, logging, panic recovery, request ID, rate limiting |

**How it works:**
1. TCP connection accepted
2. Raw bytes parsed into a `Request` struct
3. Router matches the path to a handler chain (middleware + handler)
4. A `Context` object wraps request + response for the handler
5. Response bytes written back to the TCP connection

---

### 2. Authentication

**Location:** `auth/`

Full user management with email/password, JWT access tokens, refresh tokens, and OAuth.

| File | Purpose |
|------|---------|
| `service.go` | User model, signup/signin/signout logic, user CRUD |
| `bcrypt.go` | From-scratch Blowfish cipher and bcrypt password hashing |
| `jwt.go` | RSA-4096 key generation, JWT signing (RS256), verification, JWKS |
| `tokens.go` | Refresh token generation, storage, and validation |
| `session.go` | Session lifecycle (create token pair, refresh, revoke) |
| `middleware.go` | JWT verification middleware, `RequireAuth()`, `RequireAdmin()` |
| `oauth.go` | Google and GitHub OAuth flows |
| `handlers.go` | HTTP endpoints |

**API Endpoints:**

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/auth/signup` | No | Create account |
| POST | `/auth/signin` | No | Login |
| POST | `/auth/refresh` | No | Refresh tokens |
| GET | `/auth/.well-known/jwks.json` | No | Public keys |
| GET | `/auth/me` | Yes | Current user profile |
| PUT | `/auth/me` | Yes | Update profile |
| POST | `/auth/signout` | Yes | Revoke session |
| POST | `/auth/change-password` | Yes | Change password |
| GET | `/auth/admin/users` | Admin | List all users |
| PUT | `/auth/admin/users/:uid` | Admin | Update any user |
| DELETE | `/auth/admin/users/:uid` | Admin | Delete a user |

---

### 3. Database

**Location:** `database/`

A document database inspired by Firestore. Data is organized in collections of JSON documents.

| File | Purpose |
|------|---------|
| `engine.go` | Top-level database engine, collection management, change listeners |
| `collection.go` | Collection with B-Tree backed storage |
| `document.go` | Document model with auto-generated metadata (`_id`, `_created_at`, `_version`) |
| `memory.go` | Custom in-memory B-Tree implementation |
| `query.go` | Query executor with where clauses, ordering, limit/offset |
| `index.go` | Secondary index management for query acceleration |
| `transaction.go` | ACID transactions and batch writes |
| `snapshot.go` | Point-in-time snapshots |
| `wal.go` | Write-Ahead Log for crash recovery |
| `handlers.go` | HTTP endpoints |

**API Endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/db/collections` | List all collections |
| DELETE | `/db/collections/:name` | Delete a collection |
| POST | `/db/:collection` | Create a document (auto-ID) |
| GET | `/db/:collection` | List documents (paginated) |
| GET | `/db/:collection/:id` | Get a single document |
| PUT | `/db/:collection/:id` | Set (full replace) a document |
| PATCH | `/db/:collection/:id` | Partial update a document |
| DELETE | `/db/:collection/:id` | Delete a document |
| POST | `/db/_query` | Run a structured query |
| POST | `/db/_batch` | Batch write (up to 500 ops) |
| POST | `/db/_transaction` | Atomic read-then-write |
| POST | `/db/_indexes` | Create a secondary index |
| GET | `/db/_indexes/:collection` | List indexes |
| POST | `/db/_snapshot` | Take a snapshot |
| GET | `/db/_stats` | Engine statistics |

**Query Example:**
```json
{
  "collection": "users",
  "where": [
    { "field": "age", "op": ">=", "value": 18 }
  ],
  "order_by": "name",
  "order_dir": "asc",
  "limit": 20
}
```

---

### 4. Real-Time (WebSocket)

**Location:** `realtime/`

A hand-rolled RFC 6455 WebSocket engine for live data sync.

| File | Purpose |
|------|---------|
| `websocket.go` | Raw RFC 6455 frame reading/writing over TCP |
| `hub.go` | Central pub/sub hub managing all clients and channels |
| `client.go` | Per-client read/write pumps and subscription tracking |
| `streams.go` | Document change stream bridge (DB mutations → WebSocket events) |
| `handlers.go` | HTTP upgrade endpoint + REST status endpoints |

**How it works:**
1. Client connects to `/realtime/ws` with a standard WebSocket handshake
2. Forge hijacks the TCP connection and performs the 101 Switching Protocols upgrade
3. Client subscribes to channels (e.g., `db:users` for the users collection)
4. When a document changes in the database, the `Streams` bridge pipes the change event into the hub
5. The hub fans out the event to all subscribed clients

**API Endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/realtime/ws` | WebSocket upgrade |
| GET | `/realtime/stats` | Connection statistics |
| GET | `/realtime/channels` | List active channels |
| POST | `/realtime/publish` | Server-side publish |

---

### 5. Storage

**Location:** `storage/`

File storage with content-addressable blobs, MIME detection, signed URLs, and chunked uploads.

| File | Purpose |
|------|---------|
| `engine.go` | Top-level engine, upload/download/delete, path normalization |
| `blob.go` | Content-addressable blob store (hash-based deduplication) |
| `metadata.go` | File metadata persistence (path → hash mapping) |
| `mime.go` | MIME type detection from file extension and magic bytes |
| `access.go` | Signed URL generation and verification (HMAC-SHA256) |
| `chunker.go` | Chunked upload session management |
| `stream.go` | HTTP Range request support and byte-range streaming |
| `handlers.go` | HTTP endpoints |

**API Endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| POST | `/storage/upload/*path` | Upload a file |
| GET | `/storage/object/*path` | Download a file |
| DELETE | `/storage/object/*path` | Delete a file |
| GET | `/storage/list/*path` | List files by prefix |
| GET | `/storage/metadata/*path` | Get file metadata |
| PUT | `/storage/metadata/*path` | Update custom metadata |
| POST | `/storage/signed-url` | Generate a signed URL |
| POST | `/storage/upload-chunk/init` | Start chunked upload |
| POST | `/storage/upload-chunk/add` | Upload a chunk |
| POST | `/storage/upload-chunk/complete` | Finalize chunked upload |
| DELETE | `/storage/upload-chunk/:uploadId` | Cancel chunked upload |
| GET | `/storage/stats` | Storage statistics |

---

### 6. Serverless Functions

**Location:** `functions/`

Deploy, invoke, and schedule JavaScript/shell functions.

| File | Purpose |
|------|---------|
| `deployer.go` | Function deployment, versioning, persistent metadata |
| `runtime.go` | Subprocess execution with timeout, concurrency limits, log persistence |
| `sandbox.go` | Sandboxed execution with environment isolation |
| `scheduler.go` | Cron-based scheduling with custom cron parser |
| `trigger.go` | Event-driven trigger management (HTTP, DB, auth, schedule) |
| `handlers.go` | HTTP endpoints |

**API Endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| POST | `/functions/deploy` | Deploy a function |
| GET | `/functions/list` | List all functions |
| GET | `/functions/get/:name` | Get function details |
| DELETE | `/functions/:name` | Delete a function |
| POST | `/functions/invoke/:name` | Invoke a function |
| GET | `/functions/logs/:name` | View execution logs |
| GET | `/functions/schedules` | List scheduled jobs |
| GET | `/functions/stats` | Service statistics |

---

### 7. Hosting

**Location:** `hosting/`

Deploy static websites with CDN caching, gzip compression, and SPA routing.

| File | Purpose |
|------|---------|
| `server.go` | Site management, file serving, redirect rules |
| `deployer.go` | Site deployment from ZIP/tar archives |
| `cache.go` | LRU in-memory file cache |
| `cdn.go` | Content Delivery Network abstraction |
| `ssl.go` | TLS/SSL certificate management |
| `handlers.go` | HTTP endpoints |

---

### 8. Analytics

**Location:** `analytics/`

Event tracking with buffered ingestion, time-series aggregation, and persistent storage.

| File | Purpose |
|------|---------|
| `engine.go` | Event model, buffered channel ingestion, background flusher |
| `collector.go` | Real-time counters and session tracking |
| `aggregator.go` | Time-series aggregation (hourly/daily/weekly buckets) |
| `store.go` | JSONL event persistence to disk |
| `handlers.go` | HTTP endpoints |

**API Endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| POST | `/analytics/track` | Track a single event |
| POST | `/analytics/batch` | Track multiple events |
| GET | `/analytics/stats` | Buffer and config stats |

---

### 9. Security Rules

**Location:** `rules/`

A custom Domain-Specific Language (DSL) for declarative security rules, modeled after Firebase Security Rules.

| File | Purpose |
|------|---------|
| `lexer.go` | Hand-written lexer (tokenizer) for the rules DSL |
| `parser.go` | Recursive descent parser producing an AST |
| `ast.go` | Abstract Syntax Tree node types |
| `evaluator.go` | Runtime expression evaluator |
| `builtin.go` | Built-in functions and methods |
| `validator.go` | Static analysis and validation of rule sets |

**Example Rule:**
```
rules_version = '2'

service forge.database {
  match /databases/{database}/documents {
    match /users/{userId} {
      allow read:  if request.auth != null;
      allow write: if request.auth.uid == userId;
    }
  }
}
```

---

### 10. Admin Dashboard

**Location:** `dashboard/`

An embedded single-page admin dashboard served at `/dashboard/`.

| File | Purpose |
|------|---------|
| `embed.go` | Go file that serves the HTML/CSS/JS assets via handlers |
| `index.html` | Dashboard HTML |
| `app.js` | Dashboard JavaScript (fetches from all service APIs) |
| `style.css` | Glassmorphism-themed styles |

---

## TypeScript SDK

**Location:** `sdk/`

A vanilla TypeScript SDK (`@forge/client`) for interacting with all Forge services from the browser or Node.js.

| File | Purpose |
|------|---------|
| `src/index.ts` | `ForgeClient` class and `initializeApp()` factory |
| `src/forge.ts` | `ForgeCore` — base HTTP client with auth token injection |
| `src/auth.ts` | `AuthModule` — signup, login, me |
| `src/db.ts` | `DatabaseModule` + `CollectionReference` — CRUD + realtime snapshots |
| `src/storage.ts` | `StorageModule` — upload, getUrl |
| `src/functions.ts` | `FunctionsModule` — invoke |
| `src/analytics.ts` | `AnalyticsModule` — track |
| `src/realtime.ts` | `RealtimeManager` — WebSocket with auto-reconnect, heartbeat, subscription persistence |
| `src/types.ts` | Full TypeScript type definitions |

**Usage:**
```typescript
import { initializeApp } from "@forge/client";

const forge = initializeApp({ endpoint: "http://localhost:8080" });

// Auth
await forge.auth.signup("user@example.com", "password123");
await forge.auth.login("user@example.com", "password123");

// Database
await forge.db.collection("posts").set("post-1", { title: "Hello" });
const post = await forge.db.collection("posts").get("post-1");

// Realtime
const unsubscribe = await forge.db.collection("posts").onSnapshot((change) => {
  console.log("Live change:", change);
});

// Storage
await forge.storage.upload("avatars", fileBlob, "avatar.png");

// Functions
const result = await forge.functions.invoke("sendEmail", { to: "recipient@example.com" });

// Analytics
await forge.analytics.track("page_view", { page: "/home" });
```

---

## Configuration

Forge loads configuration in this priority order: **Environment variables > Config file > Defaults**.

**Config file:** `forge.json` (or pass `--config path/to/config.json`)

```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 8080,
    "enable_cors": true,
    "cors_origins": ["*"],
    "max_body_size": 10485760
  },
  "auth": {
    "enabled": true,
    "token_expiry": "1h",
    "refresh_expiry": "720h",
    "bcrypt_cost": 12
  },
  "database": {
    "enabled": true,
    "snapshot_interval": "5m",
    "max_transaction_size": 500
  },
  "storage": {
    "enabled": true,
    "max_file_size": 104857600
  },
  "functions": {
    "enabled": true,
    "timeout": 60,
    "max_concurrent": 10,
    "runtime": "script"
  },
  "hosting": {
    "enabled": true,
    "spa_mode": true,
    "enable_compression": true
  },
  "analytics": {
    "enabled": true,
    "retention_days": 90
  },
  "log": {
    "level": "info",
    "pretty": true
  },
  "data_dir": "forge-data"
}
```

**Environment variable overrides:**

| Variable | Default | Description |
|----------|---------|-------------|
| `FORGE_PORT` | `8080` | Server port |
| `FORGE_HOST` | `0.0.0.0` | Server host |
| `FORGE_DATA_DIR` | `forge-data` | Root data directory |
| `FORGE_LOG_LEVEL` | `info` | Log level (debug/info/warn/error) |
| `FORGE_LOG_PRETTY` | `true` | Pretty print logs |

---

## Data Model

All persistent data lives under the `data_dir` (default: `forge-data/`):

```
forge-data/
├── auth/
│   ├── users.json          # User records
│   ├── tokens/             # Active refresh tokens
│   └── keys/               # RSA key pair
├── dynamicdb/
│   ├── wal/                # Write-ahead log entries
│   └── snapshots/          # Point-in-time snapshots
├── storage/
│   ├── objects/            # Content-addressable blobs
│   └── metadata/           # File path → hash mappings
├── functions/
│   └── bundles/
│       └── <function-name>/
│           ├── index.js             # Function source code
│           ├── forge-function.json  # Function metadata
│           └── logs.jsonl           # Execution logs
├── hosting/
│   └── projects/           # Deployed static sites
└── analytics/
    └── events/             # JSONL event logs
```

**Backups:** Copy the entire `forge-data/` directory. That's it. Your whole backend is a folder.

---

## Project Structure

```
Forge/
├── main.go               # Entry point — boots all services
├── go.mod                 # Go module (zero dependencies)
├── forge.rules            # Default security rules
│
├── server/                # Raw TCP → HTTP server
├── auth/                  # Authentication & authorization
├── database/              # Document database engine
├── realtime/              # WebSocket pub/sub
├── storage/               # File storage engine
├── functions/             # Serverless functions
├── hosting/               # Static site hosting
├── analytics/             # Event analytics
├── rules/                 # Security rules DSL
├── dashboard/             # Embedded admin UI
│
├── config/                # Configuration loading
├── logger/                # Structured logger
├── utils/                 # Shared utilities (crypto, JSON, time, validation)
│
├── sdk/                   # TypeScript client SDK
│   ├── src/               # SDK source files
│   ├── package.json       # npm package definition
│   └── tsconfig.json      # TypeScript config
│
├── forge-data/            # Runtime data (gitignored)
└── .gitignore
```
