# Forge Integration Guide

> **This file is meant for AI agents and developers.**  
> It explains exactly how to integrate the Forge backend into any project — frontend, mobile, or server-side.

---

## Quick Context

**Forge** is a self-hosted Backend-as-a-Service (BaaS). It is a single Go binary that provides:
- Auth (JWT, OAuth)
- Document Database
- File Storage
- Real-Time WebSockets
- Serverless Functions
- Static Hosting
- Analytics

**Repository:** `https://github.com/ayushkunwarsingh/Forge`  
**Default port:** `8080`  
**API base:** `http://<host>:8080`  
**SDK package:** `@forge/client` (located in `sdk/` directory of this repo)

---

## Step 1: Start Forge

```bash
# Clone and build
git clone https://github.com/ayushkunwarsingh/Forge.git
cd Forge
go build -o forge main.go

# Run (all services on port 8080)
./forge

# Or with custom port
FORGE_PORT=4000 ./forge
```

Verify: `curl http://localhost:8080/health` → should return `{ "status": "ok" }`

---

## Step 2: Connect From Your Project

### Option A: Use the TypeScript SDK

```bash
# From your project directory, link the SDK
cd /path/to/your-project

# Option 1: npm link
cd /path/to/Forge/sdk && npm install && npm run build
npm link
cd /path/to/your-project && npm link @forge/client

# Option 2: Copy the SDK into your project
cp -r /path/to/Forge/sdk ./forge-sdk
# Then import from "./forge-sdk/dist"

# Option 3: Install from local path
npm install /path/to/Forge/sdk
```

**Initialize the client:**

```typescript
import { initializeApp } from "@forge/client";

// Point to wherever Forge is running
const forge = initializeApp({
  endpoint: "http://localhost:8080"
});
```

### Option B: Use REST API Directly (any language)

Forge exposes a standard REST API. No SDK required — use `fetch`, `axios`, Python `requests`, Go `http.Client`, or any HTTP client.

---

## Step 3: Authentication

### Sign Up

```typescript
// SDK
const result = await forge.auth.signup("user@example.com", "password123");
// result.tokens.token → JWT access token
// result.user → user object
```

```bash
# REST
curl -X POST http://localhost:8080/auth/signup \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"password123"}'
```

### Log In

```typescript
// SDK
const result = await forge.auth.login("user@example.com", "password123");
// Token is auto-stored in the SDK — all subsequent requests are authenticated
```

```bash
# REST
curl -X POST http://localhost:8080/auth/signin \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"password123"}'

# Response includes: { "tokens": { "token": "eyJ...", "refresh_token": "..." } }
# Use the token in subsequent requests:
# -H "Authorization: Bearer eyJ..."
```

### Get Current User

```typescript
const user = await forge.auth.me();
```

```bash
curl http://localhost:8080/auth/me \
  -H "Authorization: Bearer <token>"
```

### Refresh Token

```bash
curl -X POST http://localhost:8080/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"<refresh_token>"}'
```

---

## Step 4: Database Operations

### Create / Set a Document

```typescript
// SDK
await forge.db.collection("users").set("user-123", {
  name: "Alice",
  email: "alice@example.com",
  age: 28
});
```

```bash
# REST — PUT (set with explicit ID)
curl -X PUT http://localhost:8080/db/users/user-123 \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"name":"Alice","email":"alice@example.com","age":28}'

# REST — POST (auto-generate ID)
curl -X POST http://localhost:8080/db/users \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"name":"Bob","email":"bob@example.com"}'
```

### Read a Document

```typescript
const user = await forge.db.collection("users").get("user-123");
```

```bash
curl http://localhost:8080/db/users/user-123 \
  -H "Authorization: Bearer <token>"
```

### List Documents (Paginated)

```bash
curl "http://localhost:8080/db/users?limit=20&offset=0" \
  -H "Authorization: Bearer <token>"
```

### Update a Document (Partial Merge)

```bash
curl -X PATCH http://localhost:8080/db/users/user-123 \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"age":29}'
```

### Delete a Document

```bash
curl -X DELETE http://localhost:8080/db/users/user-123 \
  -H "Authorization: Bearer <token>"
```

### Query Documents

```bash
curl -X POST http://localhost:8080/db/_query \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{
    "collection": "users",
    "where": [
      {"field": "age", "op": ">=", "value": 21}
    ],
    "order_by": "name",
    "order_dir": "asc",
    "limit": 50
  }'
```

**Available query operators:** `==`, `!=`, `>`, `>=`, `<`, `<=`, `in`, `array-contains`

### Batch Write (up to 500 operations)

```bash
curl -X POST http://localhost:8080/db/_batch \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{
    "writes": [
      {"operation":"set","collection":"users","document_id":"u1","data":{"name":"Alice"}},
      {"operation":"set","collection":"users","document_id":"u2","data":{"name":"Bob"}},
      {"operation":"delete","collection":"old_users","document_id":"u99"}
    ]
  }'
```

### Transactions (Atomic Read-Then-Write)

```bash
curl -X POST http://localhost:8080/db/_transaction \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{
    "reads": [
      {"collection":"accounts","document_id":"acct-1"}
    ],
    "writes": [
      {"operation":"update","collection":"accounts","document_id":"acct-1","data":{"balance":950}}
    ]
  }'
```

---

## Step 5: File Storage

### Upload a File

```bash
curl -X POST http://localhost:8080/storage/upload/photos/vacation.jpg \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: image/jpeg" \
  --data-binary @vacation.jpg
```

### Download a File

```bash
curl http://localhost:8080/storage/object/photos/vacation.jpg -o vacation.jpg
```

### Generate a Signed URL (time-limited public access)

```bash
curl -X POST http://localhost:8080/storage/signed-url \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"path":"photos/vacation.jpg","expires_in":3600}'
```

### List Files

```bash
curl http://localhost:8080/storage/list/photos/ \
  -H "Authorization: Bearer <token>"
```

### Chunked Upload (for large files)

```bash
# 1. Init
curl -X POST http://localhost:8080/storage/upload-chunk/init \
  -H "Content-Type: application/json" \
  -d '{"path":"videos/big.mp4","total_size":104857600,"content_type":"video/mp4"}'
# Returns: { "upload_id": "...", "chunk_size": 262144, "total_chunks": 400 }

# 2. Upload each chunk
curl -X POST "http://localhost:8080/storage/upload-chunk/add?upload_id=<id>&index=0" \
  --data-binary @chunk_0

# 3. Finalize
curl -X POST http://localhost:8080/storage/upload-chunk/complete \
  -H "Content-Type: application/json" \
  -d '{"upload_id":"<id>"}'
```

---

## Step 6: Real-Time Subscriptions

### SDK (Recommended)

```typescript
// Subscribe to live document changes
const unsubscribe = await forge.db.collection("messages").onSnapshot((change) => {
  console.log("New message:", change);
});

// Advanced: use the RealtimeManager directly
forge.realtime.connect();
const unsub = forge.realtime.subscribe("my-channel", (data) => {
  console.log("Channel event:", data);
});

// Monitor connection state
forge.realtime.onStateChange((state) => {
  console.log("Connection:", state); // "connecting" | "connected" | "disconnected" | "reconnecting"
});
```

### Raw WebSocket

```javascript
const ws = new WebSocket("ws://localhost:8080/realtime/ws");

ws.onopen = () => {
  // Authenticate
  ws.send(JSON.stringify({ type: "auth", payload: { token: "<jwt-token>" } }));
  
  // Subscribe to a channel
  ws.send(JSON.stringify({ type: "subscribe", payload: { channel: "db:messages" } }));
};

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  if (msg.type === "message") {
    console.log(`[${msg.channel}]`, msg.payload);
  }
};
```

### Server-Side Publish

```bash
curl -X POST http://localhost:8080/realtime/publish \
  -H "Content-Type: application/json" \
  -d '{"channel":"notifications","event":"new","data":{"text":"Hello world"}}'
```

---

## Step 7: Serverless Functions

### Deploy a Function

```bash
curl -X POST http://localhost:8080/functions/deploy \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{
    "name": "greet",
    "source": "module.exports = function(payload) { return { message: \"Hello, \" + (payload.name || \"World\") + \"!\" }; }",
    "entry_point": "index.js",
    "runtime": "node"
  }'
```

### Invoke a Function

```typescript
// SDK
const result = await forge.functions.invoke("greet", { name: "Alice" });
// result.output → '{"message":"Hello, Alice!"}'
```

```bash
# REST
curl -X POST http://localhost:8080/functions/invoke/greet \
  -H "Content-Type: application/json" \
  -d '{"name":"Alice"}'
```

### View Function Logs

```bash
curl http://localhost:8080/functions/logs/greet?limit=20
```

### Deploy with Cron Schedule

```bash
curl -X POST http://localhost:8080/functions/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "name": "dailyCleanup",
    "source": "module.exports = () => { console.log(JSON.stringify({cleaned: true})); };",
    "triggers": [{"type":"schedule","schedule":"0 3 * * *"}]
  }'
```

---

## Step 8: Analytics

### Track Events

```typescript
// SDK
await forge.analytics.track("page_view", { page: "/home", referrer: "google" });
```

```bash
# REST — Single event
curl -X POST http://localhost:8080/analytics/track \
  -H "Content-Type: application/json" \
  -d '{"name":"page_view","properties":{"page":"/home"}}'

# REST — Batch (multiple events)
curl -X POST http://localhost:8080/analytics/batch \
  -H "Content-Type: application/json" \
  -d '{
    "events": [
      {"name":"page_view","properties":{"page":"/home"}},
      {"name":"click","properties":{"button":"signup"}}
    ]
  }'
```

---

## Step 9: React / Next.js Integration Example

```tsx
// lib/forge.ts
import { initializeApp } from "@forge/client";
export const forge = initializeApp({ endpoint: process.env.NEXT_PUBLIC_FORGE_URL || "http://localhost:8080" });

// app/page.tsx
"use client";
import { useEffect, useState } from "react";
import { forge } from "@/lib/forge";

export default function Home() {
  const [posts, setPosts] = useState([]);

  useEffect(() => {
    // Fetch initial data
    forge.db.collection("posts").get("all").then(setPosts);

    // Subscribe to live changes
    let unsub: (() => void) | null = null;
    forge.db.collection("posts").onSnapshot((change) => {
      console.log("Live update:", change);
    }).then(fn => { unsub = fn; });

    return () => { unsub?.(); };
  }, []);

  const handleCreate = async () => {
    await forge.db.collection("posts").set(`post-${Date.now()}`, {
      title: "New Post",
      body: "Created from React!",
      created_at: Date.now()
    });
  };

  return (
    <div>
      <button onClick={handleCreate}>Create Post</button>
      <pre>{JSON.stringify(posts, null, 2)}</pre>
    </div>
  );
}
```

---

## Common Patterns

### Protected API Calls

After login, the SDK automatically injects the JWT token into every request. For REST:

```
Authorization: Bearer eyJhbGciOiJSUzI1NiIs...
```

### Environment-Based Configuration

```env
# .env.local (for Next.js / Vite)
NEXT_PUBLIC_FORGE_URL=http://localhost:8080       # development
NEXT_PUBLIC_FORGE_URL=https://api.mysite.com      # production
```

### Error Handling

All Forge API errors return:

```json
{
  "error": "Not Found",
  "message": "Document 'users/xyz' not found",
  "status": 404
}
```

---

## Deployment Checklist

When deploying Forge to production:

1. **Build:** `go build -o forge main.go`
2. **Upload** the `forge` binary to your server
3. **Create** a `forge.json` config file (or use env vars)
4. **Set** `FORGE_DATA_DIR` to a persistent disk path
5. **Run** behind a reverse proxy (Nginx/Caddy) with HTTPS
6. **Update** your SDK endpoint to the production URL:
   ```typescript
   initializeApp({ endpoint: "https://api.yoursite.com" });
   ```
7. **Back up** the `FORGE_DATA_DIR` directory regularly — it contains everything

### Nginx Reverse Proxy

```nginx
server {
    listen 443 ssl;
    server_name api.yoursite.com;

    ssl_certificate     /etc/letsencrypt/live/api.yoursite.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/api.yoursite.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

---

## API Reference Quick Table

| Service | Method | Endpoint | Body |
|---------|--------|----------|------|
| Health | GET | `/health` | — |
| Signup | POST | `/auth/signup` | `{email, password}` |
| Login | POST | `/auth/signin` | `{email, password}` |
| Refresh | POST | `/auth/refresh` | `{refresh_token}` |
| Me | GET | `/auth/me` | — |
| Create Doc | POST | `/db/:collection` | `{...fields}` |
| Get Doc | GET | `/db/:collection/:id` | — |
| Set Doc | PUT | `/db/:collection/:id` | `{...fields}` |
| Update Doc | PATCH | `/db/:collection/:id` | `{...fields}` |
| Delete Doc | DELETE | `/db/:collection/:id` | — |
| Query | POST | `/db/_query` | `{collection, where, order_by, limit}` |
| Batch | POST | `/db/_batch` | `{writes: [{operation, collection, document_id, data}]}` |
| Upload | POST | `/storage/upload/*path` | raw file bytes |
| Download | GET | `/storage/object/*path` | — |
| Signed URL | POST | `/storage/signed-url` | `{path, expires_in}` |
| Deploy Fn | POST | `/functions/deploy` | `{name, source, runtime, triggers}` |
| Invoke Fn | POST | `/functions/invoke/:name` | `{...payload}` |
| Fn Logs | GET | `/functions/logs/:name` | — |
| WebSocket | GET | `/realtime/ws` | — (upgrade) |
| Publish | POST | `/realtime/publish` | `{channel, event, data}` |
| Track | POST | `/analytics/track` | `{name, properties}` |
| Batch Track | POST | `/analytics/batch` | `{events: [{name, properties}]}` |
| Dashboard | GET | `/dashboard/` | — (HTML) |
