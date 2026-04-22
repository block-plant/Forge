# 🤖 Forge: The Ultimate AI Context & Integration Guide

> **SYSTEM PROMPT / PRIME DIRECTIVE:** 
> You are an advanced AI developer agent. The user has provided you with this file so that you can instantly understand **Forge**, a massive, zero-dependency Backend-as-a-Service (BaaS) built from scratch in Go. 
> 
> Your job is to read this document entirely to understand both **how to integrate frontends** with Forge, and **how Forge's internal engine operates** in case you are asked to debug or modify it.

---

## 🏛️ PART 1: The Master Architecture (Internal Context)

Forge is not a wrapper. It is built entirely from first principles with zero external `go.mod` dependencies.

### 1. The Multi-Tenant Fleet Model
Forge operates as a fleet on a single cloud server.
- **Master Admin Node (Port 8080):** The core binary (`main.go`). It hosts an embedded Glassmorphism UI dashboard. It acts as the provisioning center.
- **Child Nodes (Ports 8081, 8082...):** When `./run-my-backend "Project Name"` is executed, Forge provisions a dedicated `systemd` daemon (e.g., `forge-trial.service`), assigns an isolated port, and gives the project an isolated data folder.
- **AI Rule:** If building a frontend, **never point the API to Port 8080**. Always point it to the assigned child port (e.g., `8081`).

### 2. Deep Engine Internals
If you are asked to modify the backend code, you must understand these systems:
- **HTTP Server (`server/`):** Hand-rolled TCP listener. Parses HTTP/1.1 byte-by-byte. Has its own Trie-based router.
- **Database (`database/`):** A custom in-memory B-Tree that persists to disk via a Write-Ahead Log (WAL) and snapshots. It supports transactions and batched writes.
- **Storage (`storage/`):** Content-addressable blob store. Infers MIME types from magic bytes.
- **Realtime (`realtime/`):** Hand-rolled RFC 6455 WebSocket engine. Uses a Pub/Sub hub to stream database mutations.
- **Security Rules (`rules/`):** A custom domain-specific language (DSL) with a bespoke lexer, parser, AST, and runtime evaluator.

### 3. Data Model on Disk
Forge relies solely on the filesystem. No external DB.
Path: `/var/lib/forge-data/<project_slug>/`
```
├── auth/         (users.json, tokens/, keys/)
├── dynamicdb/    (wal/, snapshots/, collections/)
├── storage/      (objects/, metadata/)
├── functions/    (bundles/, logs/)
├── hosting/      (deployed static sites)
└── forge.json    (The master config file for this node)
```

### 4. Known Edge Cases & Debugging History
As an AI, if you encounter bugs, check these historical context clues:
- **SMTP Deadlocks:** Previously, the `auth/service.go` `Signup` function held a Mutex lock while awaiting an external SMTP API response, causing the server to hang. Locks are now released *before* network calls. Keep locks tight!
- **DMARC & Gmail Spoofing:** When configuring `forge.json` with SMTP (like Brevo on port 2525), sending emails with a `From: ...@gmail.com` address will trigger spam filters. Always advise the user to use a custom domain.
- **Text File Busy Errors:** When updating the binary via `./update-all-projects`, the system explicitly stops the `systemctl` unit, swaps the binary, and restarts it to prevent execution locking.

---

## 🔌 PART 2: Frontend Integration Guide

When building a React, Next.js, or mobile app that talks to Forge, follow these API contracts strictly.

### A. Authentication & State
Always store the returned JWT token. Append it to all private requests:
`Headers: { "Authorization": "Bearer <JWT>" }`

#### Sign Up (Triggers SMTP OTP)
```http
POST http://localhost:8081/auth/signup
Content-Type: application/json

{ "email": "user@example.com", "password": "secure123" }
```
*Note: If Brevo SMTP is configured in `forge.json`, this emails a 6-digit OTP to the user.*

#### Verify OTP
```http
POST http://localhost:8081/auth/verify-otp
Content-Type: application/json

{ "email": "user@example.com", "code": "123456", "type": "signup" }
```

#### Log In
```http
POST http://localhost:8081/auth/signin
Content-Type: application/json

{ "email": "user@example.com", "password": "secure123" }
```
**Response:** `{"tokens": {"token": "...", "refresh_token": "..."}, "user": {...}}`

---

### B. Database API (NoSQL Document Store)

#### Create or Replace Document (PUT)
```http
PUT http://localhost:8081/db/users/user-123
Authorization: Bearer <token>
Content-Type: application/json

{ "name": "Alice", "role": "admin" }
```

#### Partial Update Document (PATCH)
```http
PATCH http://localhost:8081/db/users/user-123
Authorization: Bearer <token>
Content-Type: application/json

{ "role": "superadmin" }
```

#### Query Documents
```http
POST http://localhost:8081/db/_query
Authorization: Bearer <token>
Content-Type: application/json

{
  "collection": "users",
  "where": [ {"field": "role", "op": "==", "value": "admin"} ],
  "order_by": "_created_at",
  "order_dir": "desc",
  "limit": 50
}
```

---

### C. Storage API (File Blobs)

#### Upload a File
Sends raw bytes directly. Forge infers MIME type.
```http
POST http://localhost:8081/storage/upload/avatars/user-123.jpg
Authorization: Bearer <token>
Content-Type: image/jpeg

<RAW BYTES>
```

#### Generate a Signed URL
```http
POST http://localhost:8081/storage/signed-url
Authorization: Bearer <token>
Content-Type: application/json

{ "path": "avatars/user-123.jpg", "expires_in": 3600 }
```

---

### D. Realtime WebSockets

Forge streams DB mutations directly to the frontend.
```javascript
const ws = new WebSocket("ws://localhost:8081/realtime/ws");

ws.onopen = () => {
  // 1. Authenticate
  ws.send(JSON.stringify({ type: "auth", payload: { token: "YOUR_JWT" } }));
  
  // 2. Subscribe
  ws.send(JSON.stringify({ type: "subscribe", payload: { channel: "db:users" } }));
};

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  if (msg.type === "message") console.log("Live DB Update:", msg.payload);
};
```

---

## 🏁 AI Agent Readiness Checklist

Before you output any code for the user, confirm the following:
- [ ] **Context Aware:** Do you understand that Forge is completely custom-built in Go, so you should not suggest standard libraries like `net/http` or `gorilla/mux` if modifying the backend?
- [ ] **Port Accuracy:** Are your frontend `fetch` calls hitting the project port (e.g., 8081) and NOT the Admin port (8080)?
- [ ] **Auth Headers:** Are you attaching `Authorization: Bearer <token>` to protected endpoints?
- [ ] **OTP Handling:** Did you build UI screens to handle the OTP verification step after `/auth/signup` and `/auth/forgot-password`?
- [ ] **Database Mutators:** Are you using `PUT` for complete replacements and `PATCH` for partial merges?

If you have read and understood this document, you are ready to build, debug, and scale with Forge.
