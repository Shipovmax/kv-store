# Key-Value Store — In-Memory Storage with HTTP API and TTL

> Project #6 in the roadmap: in-memory KV store with HTTP API, TTL, and concurrent access via sync.RWMutex

---

## For Recruiters

### What this is and why it exists

This is the sixth project in a junior Go backend preparation roadmap. The previous project (#5, Worker Pool) covered goroutines and channels. This project transitions to a classic backend problem: building a Redis-like store from scratch.

The project models a real component — a simple cache or session store used in every high-load service. The key constraints match production requirements: concurrent requests, key expiration, and atomic operations.

The focus is on correct use of sync.RWMutex (not sync.Mutex), a TTL mechanism without goroutine leaks, and an HTTP API without third-party routers.

### What this project demonstrates

| Skill | Implementation |
|-------|----------------|
| Concurrent access | sync.RWMutex: parallel reads, exclusive writes |
| TTL | Background goroutine with ticker for sweep |
| HTTP API | net/http, clean routing via ServeMux |
| Data encoding | encoding/json: request/response via structs |
| Error handling | 404 for missing keys, 400 for bad requests |
| Graceful shutdown | context + os.Signal + http.Server.Shutdown |
| Code structure | store/, handler/, main.go — Clean Architecture |
| Structured logging | log/slog, JSON handler, error-level diagnostics |
| Method-based routing | Go 1.22+ ServeMux patterns (`POST /set`), no manual method checks |

### Stack

- Go 1.24
- Standard library only: net/http, encoding/json, sync, time, context
- No external dependencies

---

## For Developers

### Architectural decisions

#### sync.RWMutex instead of sync.Mutex

A KV store is a read-heavy workload. sync.Mutex blocks all readers on every operation. RWMutex allows N concurrent readers and one exclusive writer. With 100 concurrent GETs this produces a measurable difference. This is not premature optimization — it is the correct model from the start.

#### TTL via background sweep (not time.AfterFunc per key)

Using time.AfterFunc on every Set spawns a goroutine per key. With 10k entries that means 10k goroutines. Instead: one background ticker that sweeps all keys every second and deletes expired ones. This is the standard approach used in memcached-like systems. The sweep goroutine stops via context on shutdown.

#### Storing expiration time, not remaining TTL

The `entry` struct stores `expiresAt time.Time`, not a duration. On each Get: `time.Now().After(entry.expiresAt)`. This is correct under any scheduler delays and requires no recalculation.

#### HTTP handler as a thin layer

The handler contains no business logic, including method validation. Its job: parse the request → call the store → serialize the response. Method routing (`POST /set`, `GET /get`, `DELETE /delete`) is expressed in the `http.ServeMux` patterns themselves (Go 1.22+ enhanced routing) — a request with the wrong method never reaches the handler; the mux replies 405 with an `Allow` header directly. The store knows nothing about HTTP. This allows testing the store without HTTP.

#### Graceful shutdown

http.Server.Shutdown(ctx) waits for in-flight requests to complete. Without it, in-progress requests are cut off on SIGTERM. This is standard for any production HTTP service. The `ListenAndServe` goroutine reports its outcome over a buffered error channel so the main goroutine can both react to a startup failure (e.g. port already in use) and guarantee it never exits while that goroutine is still running — no goroutine leak on shutdown.

#### Structured logging

`log/slog` with a JSON handler writes to stdout. The delivery layer logs only delivery-level failures it cannot surface to the client (e.g. a broken connection during response encoding, discovered after `WriteHeader` has already been sent) — business errors (bad request, not found) are communicated via the HTTP response itself, not logged as errors.

#### Request body size limit

`SetKey` wraps the request body in `http.MaxBytesReader` (1 MiB cap) before decoding JSON, so a client cannot exhaust server memory with an oversized request body.

### Structure

```
kv-store/
├── main.go              # Initialization, Server.Shutdown, os.Signal
├── store/
│   └── store.go         # KVStore: Set, Get, Delete, sweep goroutine
├── handler/
│   └── handler.go       # HTTP handlers: POST /set, GET /get, DELETE /delete
├── go.mod
└── README.md
```

### Installation and run

```bash
git clone https://github.com/Shipovmax/kv-store
cd kv-store
go run ./...
```

The listen address defaults to `:8080` and can be overridden via `KV_STORE_ADDR`:

```bash
KV_STORE_ADDR=:9090 go run ./...
```

### Usage

```bash
# Set a key without TTL
curl -X POST http://localhost:8080/set \
  -H "Content-Type: application/json" \
  -d '{"key":"name","value":"max"}'

# Set a key with 10-second TTL
curl -X POST http://localhost:8080/set \
  -H "Content-Type: application/json" \
  -d '{"key":"session","value":"abc123","ttl_seconds":10}'

# Get a key
curl http://localhost:8080/get?key=name

# Delete a key
curl -X DELETE http://localhost:8080/delete?key=name
```

### Examples

```bash
# Successful Set
$ curl -X POST http://localhost:8080/set -d '{"key":"x","value":"1"}'
{"ok":true}

# Successful Get
$ curl "http://localhost:8080/get?key=x"
{"key":"x","value":"1"}

# Get a missing key
$ curl "http://localhost:8080/get?key=missing"
{"error":"key not found"}
# HTTP 404

# Get an expired key (TTL 1 second, request after 2 seconds)
$ curl "http://localhost:8080/get?key=expired"
{"error":"key not found"}
# HTTP 404

# Successful Delete
$ curl -X DELETE "http://localhost:8080/delete?key=x"
{"ok":true}
```

### Error handling

```bash
# Invalid JSON body
POST /set with body "not-json"
→ HTTP 400: {"error":"invalid request body"}

# Empty key
POST /set with {"key":"","value":"x"}
→ HTTP 400: {"error":"key cannot be empty"}

# Method not allowed (enforced by ServeMux, not the handler)
GET /set
→ HTTP 405, Allow: POST

# Negative TTL
POST /set with {"key":"x","value":"1","ttl_seconds":-1}
→ HTTP 400: {"error":"ttl_seconds must not be negative"}

# Key not found or expired
GET /get?key=missing
→ HTTP 404: {"error":"key not found"}
```

### Run without building

```bash
go run ./...
```
