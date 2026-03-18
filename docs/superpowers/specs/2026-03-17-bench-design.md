# Parley Stress-Testing Suite Design

## Goal

Build a custom Go CLI (`bench/`) that hammers Parley's HTTP and WebSocket APIs under sustained load to identify bottlenecks, measure broadcast latency, and inform where rate limits and optimizations are needed.

## Architecture

### Repository Layout

```
bench/
  cmd/
    parley-bench/
      main.go            ← CLI entry point, subcommands via cobra
  internal/
    client/
      http.go            ← REST client (auth, messages, channels, servers)
      websocket.go       ← WS client (connect via ticket, subscribe, receive typed events)
      user.go            ← Virtual user owning an HTTP+WS client pair
      provisioner.go     ← Bulk account creation + environment setup; falls back to
                            normal registration if /internal/bench/provision is absent
    scenarios/
      auth_flood.go
      ws_scale.go
      message_storm.go
      broadcast_amp.go
      read_heavy.go
      mixed.go
    metrics/
      collector.go       ← Thread-safe stats accumulator
      histogram.go       ← HDR histogram for latency percentiles
    reporter/
      terminal.go        ← Live updating progress lines + final summary table
  go.mod                 ← Separate module: parley/bench
```

`bench` is a separate Go module so it is never pulled into production builds and can have independent dependency versions.

### Server-Side: Stresstest Build Tag

A `//go:build stresstest` file adds provisioning routes to the server binary **only when explicitly built with `-tags stresstest`**. These routes are **completely absent** from the production binary — not behind a feature flag, physically not compiled in.

```
cmd/api/bench_handlers_stresstest.go   // //go:build stresstest
```

This file also registers pprof handlers (`net/http/pprof`) so goroutine counts and mutex contention are observable during a run. In a normal build, pprof is not registered anywhere.

Routes registered under `/internal/bench/`:

```
POST /internal/bench/provision
  Body:     {count: int, prefix: string}
  Response: {
    users: [{id, username, token}, ...],
    server_id: int,
    channel_id: int
  }
  Creates N users instantly:
    - No email verification
    - No rate limiting
    - Fast password hash (bypass bcrypt cost)
  Also creates one shared test server and one text channel, adds all bench
  users as members. Returns the server_id and channel_id so scenarios that
  require channel subscriptions or message posting have a ready-made target.
  Protected by: X-Bench-Secret header matching BENCH_SECRET env var

DELETE /internal/bench/cleanup
  Body:     {prefix: string}
  Response: {deleted: int}
  Deletes all users matching the prefix and all cascaded data (messages,
  server memberships, the test server). Synchronous — waits for full deletion
  before responding. Expected response time: a few seconds for 500+ users;
  client should use a 30s timeout.
  Protected by: X-Bench-Secret header
```

Build the stress-testable server:

```bash
go build -tags stresstest -o parley-api-bench ./cmd/api
BENCH_SECRET=localonly ./parley-api-bench
```

The bench tool's `provisioner.go` tries the provision endpoint first. On 404 (normal build), it falls back to regular registration with rate-limit-aware spacing (10 req/min per IP), creates a server/channel via the normal API, and adds users as members. The bench CLI works against both — fast setup against a stresstest build, slower against a normal one.

**The production DigitalOcean binary is always built without `-tags stresstest`. The Proxmox dev environment is the intended target.**

---

## Scenarios

All scenarios follow a **ramp → sustain → drain** lifecycle:
- **Ramp**: Users join gradually to avoid a thundering herd masking the real bottleneck
- **Sustain**: Full load held long enough to expose memory leaks, goroutine accumulation, and buffer saturation
- **Drain**: Users disconnect; measures recovery time

All scenarios accept:
- `--host`         (default `http://localhost:8080`) — refuses to run if host matches `parley.x86-64.com`
- `--duration`     total sustain duration
- `--cleanup`      delete test data after run (default `true`)
- `--bench-secret` for provisioner auth

---

### `auth-flood`

Tests bcrypt latency and auth endpoint throughput under maximum concurrency from a single IP. Note: the rate limiter (10 req/min per IP) caps throughput from one machine to 10 req/min. This scenario deliberately hits that cap to measure: (a) how quickly the rate limiter responds with 429, (b) bcrypt verification latency for the requests that do pass, and (c) whether the in-memory rate limiter leaks under sustained access.

To test throughput beyond the single-IP limit, the operator must run multiple bench processes from different source IPs (e.g., multiple Proxmox VMs), or configure the server with IP aliasing on the loopback (`sudo ip addr add 127.0.0.2/8 dev lo`). The bench tool itself does not forge source IPs.

- **Ramp**: `--workers` concurrent goroutines (each from the same source IP)
- **Sustain**: `--duration 5m` of continuous login attempts, mix of valid + invalid credentials
- **Measures**: Throughput, 429 rate, bcrypt latency p50/p95/p99
- **Flags**: `--workers 20 --duration 5m`

---

### `ws-scale`

Finds the WebSocket connection cliff — how many simultaneous clients before the hub degrades.

- **Ramp**: +10 connections/second up to `--max`
- **Sustain**: Hold at max for 3 minutes; clients send periodic pings and receive pongs
- **Measures**: Successful connections, forcible disconnections, time-to-first-ping. If the server was built with `-tags stresstest`, goroutine counts are readable via `GET /debug/pprof/goroutine?debug=1`.
- **Flags**: `--max 1000 --ramp-rate 10/s --sustain 3m`

---

### `message-storm`

N writers hammering one channel as fast as the server accepts. All writers are members of the provisioned test server. Before the sustain phase begins, one co-located WS listener connects (via ticket), subscribes to the test channel with `CHANNEL_SUBSCRIBE`, and holds the connection open to measure broadcast delivery lag.

The real bottleneck here is `hub.BroadcastToChannel()`, which holds `h.mu` (a `sync.RWMutex`) for the entire fan-out loop over all subscribed clients, writing to each client's 256-slot `send chan []byte`. Under heavy write load, the mutex becomes the serialization point and clients with full send buffers are synchronously evicted while the lock is held.

- **Ramp**: 50 users
- **Sustain**: 10 minutes, each writer sending 1 msg/s = 30,000 messages minimum
- **Measures**: POST latency p50/p95/p99, DB write throughput, broadcast delivery lag (measured via the pre-connected WS listener), client eviction count
- **Flags**: `--writers 50 --rate 1/s --duration 10m`

---

### `broadcast-amp`

The highest-risk scenario. One sender, N listeners subscribed to the same channel. Exploits fan-out: each message POST triggers `BroadcastToChannel`, which holds `h.mu` and iterates over all N subscriber `send` channels. If any subscriber's 256-slot buffer is full, it is evicted synchronously under the lock, stalling delivery to all other subscribers for that iteration.

All listeners are members of the provisioned test server and have sent `CHANNEL_SUBSCRIBE` before the writer starts.

**Broadcast latency** is measured by embedding a monotonic send timestamp in the message content (a JSON field ignored by the UI). Each WS listener records its receive time and reports `receive_ns - send_ns`. This requires the bench tool to run on the **same host** as the server to avoid clock skew corrupting the measurement. The CLI prints a warning **at startup** when `--host` is not localhost.

- **Ramp**: 500 listeners subscribe, then 1 writer starts sending 1 msg/s
- **Sustain**: 10 minutes. 500 listeners × 1 msg/s = 500 WS sends/sec sustained
- **Measures**: Broadcast latency p50/p95/p99 per listener, listener disconnect rate, hub mutex hold time (observable via pprof mutex profile on stresstest build)
- **Flags**: `--listeners 500 --rate 1/s --duration 10m`

---

### `read-heavy`

Concurrent message history fetches. Targets the 120/min rate limit and DB read path under load. Readers fetch from the provisioned test channel; they must be server members (ensured by provisioner).

- **Ramp**: 20 readers
- **Sustain**: 5 minutes of tight-loop `GET /api/channels/{channel_id}/messages` against the provisioned channel
- **Measures**: Throughput, 429 rate, DB read latency, query time distribution
- **Flags**: `--readers 20 --duration 5m`

---

### `mixed`

Realistic combined load for capacity planning. All virtual users are members of the provisioned test server and subscribed to the test channel before the sustain phase begins, so typing events and reads produce real broadcast fan-out.

- **Composition**: 20% writers (1 msg/s each), 60% readers (polling message history), 20% typing-event spammers (sending `TYPING` WS events at 2/s)
- **Sustain**: 15 minutes — long enough that memory leaks and goroutine accumulation become visible
- **Flags**: `--users 200 --duration 15m`

---

## Metrics

All scenarios collect and report:

| Metric | Description |
|--------|-------------|
| RPS | Requests per second (actual throughput) |
| p50 / p95 / p99 latency | Per operation type |
| Error rate | Broken down by HTTP status code |
| WS disconnect rate | Forcible closes by the server |
| Broadcast latency | HTTP POST → WS receive delta (broadcast-amp, same-host only) |
| 429 rate | Rate limit hit frequency |
| Client evictions | WS clients dropped due to full send buffer |

**Output**: Live updating terminal lines during run, final summary table at completion. Machine-readable JSON output via `--json` flag for CI/scripting.

---

## Known Bottlenecks Being Tested

From code inspection, these are the specific limits under test:

| Bottleneck | Location | Targeted by |
|------------|----------|-------------|
| `BroadcastToChannel` mutex held during full fan-out | `internal/websocket/hub.go` | `broadcast-amp`, `message-storm` |
| Client send buffer: 256 slots — full buffer = synchronous eviction under lock | `internal/websocket/hub.go` | `broadcast-amp` |
| IP rate limiter in-memory growth (cleanup every 5 min) | `cmd/api/middleware.go` | `auth-flood`, `read-heavy` |
| No per-user rate limits on authenticated endpoints | `cmd/api/middleware.go` | `message-storm` |
| Synchronous DB access check per channel subscription | `internal/websocket/client.go` | `ws-scale` |
| bcrypt latency under parallel logins | `internal/auth/` | `auth-flood` |
| No pagination limit enforcement on message history | `internal/message/` | `read-heavy` |

---

## Security Constraints

- `bench_handlers_stresstest.go` uses `//go:build stresstest` — absent from all production binaries
- Provision endpoint protected by `X-Bench-Secret` header (BENCH_SECRET env var)
- All test accounts created with a common prefix (`bench_` by default) for reliable cleanup
- `--cleanup true` by default — test data deleted after every run
- CLI refuses to run if `--host` contains `parley.x86-64.com` (production domain)
- Broadcast latency measurement (`broadcast-amp`) is only valid when bench tool runs on same host as server; CLI prints a warning if host is not localhost
