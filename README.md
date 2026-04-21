<p align="center">
  <pre>
   ███████╗ ██████╗ ██████╗  ██████╗ ███████╗
   ██╔════╝██╔═══██╗██╔══██╗██╔════╝ ██╔════╝
   █████╗  ██║   ██║██████╔╝██║  ███╗█████╗  
   ██╔══╝  ██║   ██║██╔══██╗██║   ██║██╔══╝  
   ██║     ╚██████╔╝██║  ██║╚██████╔╝███████╗
   ╚═╝      ╚═════╝ ╚═╝  ╚═╝ ╚═════╝ ╚══════╝
  </pre>
</p>

<h3 align="center">A Firebase replacement built from scratch. Zero dependencies. Every byte understood.</h3>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.22-00ADD8?style=flat-square&logo=go" alt="Go 1.22" />
  <img src="https://img.shields.io/badge/Dependencies-0-brightgreen?style=flat-square" alt="Zero Dependencies" />
  <img src="https://img.shields.io/badge/License-MIT-blue?style=flat-square" alt="License" />
  <img src="https://img.shields.io/badge/Status-Complete-success?style=flat-square" alt="Status" />
</p>

---

**Forge** is a self-hosted, production-grade Backend-as-a-Service (BaaS) platform written entirely in Go with **zero external dependencies**. It provides everything you need to power a modern application — authentication, database, storage, realtime, serverless functions, hosting, and analytics — all compiled into a single binary.

## ✨ Features

| Service | Highlights |
|---------|------------|
| 🔐 **Authentication** | Email/password, JWT (RS256), refresh tokens, OAuth (Google & GitHub), admin API |
| 🗄️ **Database** | Document collections, structured queries, secondary indexes, transactions, batch writes, WAL, snapshots |
| 📁 **Storage** | File uploads, content-addressable blobs, MIME detection, signed URLs, chunked uploads, byte-range streaming |
| ⚡ **Real-Time** | Hand-rolled RFC 6455 WebSocket engine, pub/sub channels, document change streams, auto-reconnect SDK |
| ⚙️ **Functions** | Deploy JS/shell scripts, HTTP invocation, cron scheduling, event triggers, persistent execution logs |
| 🌐 **Hosting** | Static site deployment, LRU cache, gzip compression, SPA fallback, redirect rules |
| 📊 **Analytics** | Buffered event ingestion, time-series aggregation, session tracking, batch API |
| 🛡️ **Security Rules** | Custom DSL with lexer/parser/evaluator — declarative rules like Firebase |
| 🖥️ **Dashboard** | Embedded admin UI with glassmorphism design |

## 🏗️ Built From Scratch

This isn't a wrapper around existing libraries. Every layer is hand-crafted:

- **HTTP Server** — Raw TCP sockets, hand-parsed HTTP/1.1
- **WebSocket** — RFC 6455 frame parsing from raw bytes
- **Bcrypt** — From-scratch Blowfish cipher implementation
- **Router** — Trie-based URL matching with path params and wildcards
- **Database** — In-memory B-Tree with Write-Ahead Logging
- **Rules Engine** — Custom lexer → parser → AST → evaluator pipeline

```
go.mod → zero third-party modules
```

## 🚀 Quick Start

```bash
# Clone the repo
git clone https://github.com/ayushkunwarsingh/Forge.git
cd Forge

# Build the binary
go build -o forge main.go

# Run with defaults (port 8080, all services enabled)
./forge

# Or with custom config
./forge --config forge.json

# Or with environment overrides
FORGE_PORT=3000 FORGE_LOG_LEVEL=debug ./forge
```

Forge starts and prints:

```
╔═══════════════════════════════════════════════╗
║   ███████╗ ██████╗ ██████╗  ██████╗ ███████╗  ║
║   █████╗  ██║   ██║██████╔╝██║  ███╗█████╗    ║
║   ...                                         ║
╚═══════════════════════════════════════════════╝

INFO  Forge is starting up  version=0.1.0  go=go1.22  cpus=10
INFO  Auth service registered
INFO  Database service registered
INFO  Real-time service registered
INFO  Storage service registered
INFO  Functions service registered
INFO  Hosting service registered
INFO  Analytics service registered
INFO  Admin dashboard registered  path=/dashboard/
INFO  Starting TCP listener  address=0.0.0.0:8080
```

**Verify it's running:**

```bash
curl http://localhost:8080/health
```

```json
{
  "status": "ok",
  "service": "forge",
  "version": "0.1.0",
  "services": {
    "auth": "ok",
    "database": "ok",
    "realtime": "ok",
    "storage": "ok",
    "functions": "ok",
    "hosting": "ok",
    "analytics": "ok"
  }
}
```

## 📦 TypeScript SDK

Forge ships with a complete TypeScript SDK for frontend and Node.js integrations.

```bash
cd sdk
npm install
npm run build
```

```typescript
import { initializeApp } from "@forge/client";

const forge = initializeApp({ endpoint: "http://localhost:8080" });

// Auth
await forge.auth.signup("alice@example.com", "supersecret");
await forge.auth.login("alice@example.com", "supersecret");

// Database
await forge.db.collection("posts").set("post-1", { title: "Hello World", body: "..." });
const post = await forge.db.collection("posts").get("post-1");

// Realtime subscriptions
const unsub = await forge.db.collection("posts").onSnapshot((change) => {
  console.log("Live update:", change);
});

// Storage
await forge.storage.upload("photos", imageFile, "vacation.jpg");

// Serverless functions
const result = await forge.functions.invoke("processImage", { imageUrl: "..." });

// Analytics
await forge.analytics.track("purchase", { amount: 29.99, currency: "USD" });
```

## 🔌 REST API Overview

<details>
<summary><strong>Auth Endpoints</strong></summary>

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/auth/signup` | Create account |
| `POST` | `/auth/signin` | Login |
| `POST` | `/auth/refresh` | Refresh tokens |
| `GET` | `/auth/me` | Current user |
| `PUT` | `/auth/me` | Update profile |
| `POST` | `/auth/signout` | Sign out |
| `POST` | `/auth/change-password` | Change password |
| `GET` | `/auth/admin/users` | List users (admin) |

</details>

<details>
<summary><strong>Database Endpoints</strong></summary>

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/db/:collection` | Create document |
| `GET` | `/db/:collection` | List documents |
| `GET` | `/db/:collection/:id` | Get document |
| `PUT` | `/db/:collection/:id` | Set document |
| `PATCH` | `/db/:collection/:id` | Update document |
| `DELETE` | `/db/:collection/:id` | Delete document |
| `POST` | `/db/_query` | Structured query |
| `POST` | `/db/_batch` | Batch write |
| `POST` | `/db/_transaction` | Transaction |

</details>

<details>
<summary><strong>Storage Endpoints</strong></summary>

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/storage/upload/*path` | Upload file |
| `GET` | `/storage/object/*path` | Download file |
| `DELETE` | `/storage/object/*path` | Delete file |
| `GET` | `/storage/list/*path` | List files |
| `POST` | `/storage/signed-url` | Generate signed URL |

</details>

<details>
<summary><strong>Functions Endpoints</strong></summary>

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/functions/deploy` | Deploy function |
| `POST` | `/functions/invoke/:name` | Invoke function |
| `GET` | `/functions/logs/:name` | View logs |
| `GET` | `/functions/list` | List functions |

</details>

<details>
<summary><strong>Realtime & Analytics</strong></summary>

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/realtime/ws` | WebSocket connection |
| `POST` | `/realtime/publish` | Server-side publish |
| `POST` | `/analytics/track` | Track event |
| `POST` | `/analytics/batch` | Batch track |

</details>

## ⚙️ Configuration

Create a `forge.json` in the project root or use environment variables:

```json
{
  "server": { "host": "0.0.0.0", "port": 8080, "enable_cors": true },
  "auth": { "enabled": true, "token_expiry": "1h" },
  "database": { "enabled": true },
  "storage": { "enabled": true, "max_file_size": 104857600 },
  "functions": { "enabled": true, "timeout": 60 },
  "hosting": { "enabled": true, "spa_mode": true },
  "analytics": { "enabled": true },
  "data_dir": "forge-data"
}
```

| Env Variable | Default | Description |
|-------------|---------|-------------|
| `FORGE_PORT` | `8080` | Server port |
| `FORGE_HOST` | `0.0.0.0` | Bind address |
| `FORGE_DATA_DIR` | `forge-data` | Data directory |
| `FORGE_LOG_LEVEL` | `info` | debug/info/warn/error |

## 🚢 Deployment

<details>
<summary><strong>Binary on any Linux/macOS server</strong></summary>

```bash
go build -o forge main.go
scp forge your-server:/opt/forge/
ssh your-server "FORGE_PORT=8080 /opt/forge/forge"
```

</details>

<details>
<summary><strong>Docker</strong></summary>

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o forge main.go

FROM alpine:latest
RUN apk add --no-cache nodejs
WORKDIR /opt/forge
COPY --from=builder /app/forge .
COPY forge.json .
EXPOSE 8080
VOLUME /var/lib/forge-data
ENV FORGE_DATA_DIR=/var/lib/forge-data
CMD ["./forge"]
```

```bash
docker build -t forge .
docker run -d -p 8080:8080 -v forge-data:/var/lib/forge-data forge
```

</details>

<details>
<summary><strong>Systemd</strong></summary>

```ini
[Unit]
Description=Forge BaaS
After=network.target

[Service]
Type=simple
User=forge
WorkingDirectory=/opt/forge
ExecStart=/opt/forge/forge
Environment=FORGE_PORT=8080
Environment=FORGE_DATA_DIR=/var/lib/forge-data
Restart=always

[Install]
WantedBy=multi-user.target
```

</details>

## 📂 Project Structure

```
Forge/
├── main.go               # Entry point — boots all services
├── go.mod                 # Zero external dependencies
├── forge.rules            # Default security rules
├── server/                # Raw TCP → HTTP engine
├── auth/                  # Authentication & JWT
├── database/              # Document database
├── realtime/              # WebSocket pub/sub
├── storage/               # File storage
├── functions/             # Serverless functions
├── hosting/               # Static site hosting
├── analytics/             # Event analytics
├── rules/                 # Security rules DSL
├── dashboard/             # Embedded admin UI
├── config/                # Configuration
├── logger/                # Structured logging
├── utils/                 # Shared utilities
└── sdk/                   # TypeScript client SDK
```

## 📖 Documentation

See [`DOCUMENTATION.md`](./DOCUMENTATION.md) for a comprehensive deep-dive into every service, API endpoint, and internal design decision.

## 🤝 Integration Guide

See [`INTEGRATION.md`](./INTEGRATION.md) for step-by-step instructions on how to use Forge as the backend for your projects — including SDK setup, REST API examples, and deployment.

## 📄 License

MIT
