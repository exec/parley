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
      provisioner.go     ← Bulk account creation via /internal/bench/provision,
                            falls back to normal registration if endpoint absent
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
  go.mod                 ← Separate module: github.com/exec/parley/bench
```

`bench` is a separate Go module so it is never pulled into production builds and can have independent dependency versions.

### Server-Side: Stresstest Build Tag

A `//go:build stresstest` file adds provisioning routes to the server binary **only when explicitly built with `-tags stresstest`**. These routes are **completely absent** from the production binary — not behind a feature flag, physically not compiled in.

```
cmd/api/bench_handlers_stresstest.go   // //go:build stresstest
```

Routes registered under `/internal/bench/`:

```
POST /internal/bench/provision
  Body:     {count: int, prefix: string}
  Response: [{id, username, token}, ...]
  Creates N users instantly:
    - No email verification
    - No rate limiting
    - Fast password hash (bypass bcrypt cost)
  Protected by: X-Bench-Secret header matching BENCH_SECRET env var

DELETE /internal/bench/cleanup
  Body:     {prefix: string}
  Response: {deleted: int}
  Deletes all users (and cascaded data) matching the prefix
  Protected by: X-Bench-Secret header
```

Build the stress-testable server:

```bash
go build -tags stresstest -o parley-api-bench ./cmd/api
BENCH_SECRET=localonly ./parley-api-bench
```

The bench tool's `provisioner.go` tries the provision endpoint first. On 404 (normal build), it falls back to regular registration with rate-limit-aware spacing. The bench CLI works against both — fast setup against a stresstest build, slower against a normal one.

**The production DigitalOcean binary is always built without `-tags stresstest`. The Proxmox dev environment is the intended target.**

---

## Scenarios

All scenarios follow a **ramp → sustain → drain** lifecycle:
- **Ramp**: Users join gradually to avoid a thundering herd masking the real bottleneck
- **Sustain**: Full load held long enough to expose memory leaks, goroutine accumulation, and buffer saturation
- **Drain**: Users disconnect; measures recovery time

All scenarios accept:
- `--host`      (default `http://localhost:8080`)
- `--duration`  total sustain duration
- `--seed`      for reproducible test data
- `--cleanup`   delete test data after run (default `true`)
- `--bench-secret` for provisioner auth

---

### `auth-flood`

Tests IP-based rate limiter (10/min) and bcrypt latency under parallel login attempts.

- **Ramp**: 10 virtual IPs, each hitting the 10/min limit = 100 req/min combined
- **Sustain**: 5 minutes of continuous login attempts, mix of valid + invalid credentials
- **Measures**: Throughput, 429 rate, bcrypt latency p50/p95/p99
- **Flags**: `--ips 10 --duration 5m`

---

### `ws-scale`

Finds the WebSocket connection cliff — how many simultaneous clients before the hub degrades.

- **Ramp**: +10 connections/second up to `--max`
- **Sustain**: Hold at max for 3 minutes; clients send periodic pings and receive pongs
- **Measures**: Successful connections, forcible disconnections, time-to-first-ping, hub goroutine count (via `/debug/pprof` if enabled)
- **Flags**: `--max 1000 --ramp-rate 10/s --sustain 3m`

---

### `message-storm`

N writers hammering one channel as fast as the server accepts. Long enough to fill the hub's 1024-message broadcast buffer if delivery slows.

- **Ramp**: 50 users
- **Sustain**: 10 minutes, each writer sending 1 msg/s = 30,000 messages minimum
- **Measures**: POST latency p50/p95/p99, DB write throughput, broadcast delivery lag (measured via co-located WS listener)
- **Flags**: `--writers 50 --rate 1/s --duration 10m`

---

### `broadcast-amp`

The highest-risk scenario. One sender, N listeners on the same channel. Exploits fan-out: 1 message = N WebSocket sends through the hub. Directly targets the identified 1024-message broadcast buffer ceiling.

- **Ramp**: 500 listeners subscribe to a channel, 1 writer sends 1 msg/s
- **Sustain**: 10 minutes. 500 listeners × 1 msg/s = 500 WS sends/sec sustained
- **Measures**: **HTTP POST timestamp → WS receive timestamp per listener** (broadcast latency), listener disconnect rate as buffers fill, p50/p95/p99 broadcast latency
- **Flags**: `--listeners 500 --rate 1/s --duration 10m`

*Most likely to surface the hub broadcast buffer ceiling and slow-consumer disconnections.*

---

### `read-heavy`

Concurrent message history fetches. Targets the 120/min rate limit and DB read path under load.

- **Ramp**: 20 readers
- **Sustain**: 5 minutes of tight-loop `GET /api/channels/{id}/messages`
- **Measures**: Throughput, 429 rate, DB read latency, query time distribution
- **Flags**: `--readers 20 --duration 5m`

---

### `mixed`

Realistic combined load for capacity planning. Most useful for understanding total concurrent user capacity.

- **Composition**: 20% writers, 60% readers, 20% typing-event spammers (high-frequency WS → broadcast)
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
| Broadcast latency | HTTP POST → WS receive delta (broadcast-amp only) |
| 429 rate | Rate limit hit frequency |

**Output**: Live updating terminal lines during run, final summary table at completion. Machine-readable JSON output via `--json` flag for CI/scripting.

---

## Known Bottlenecks Being Tested

From code inspection, these are the specific limits under test:

| Bottleneck | Location | Targeted by |
|------------|----------|-------------|
| Hub broadcast buffer: 1024 messages | `internal/websocket/hub.go` | `broadcast-amp`, `message-storm` |
| Client send buffer: 256 messages | `internal/websocket/hub.go` | `broadcast-amp` |
| IP rate limiter memory growth | `cmd/api/middleware.go` | `auth-flood`, `read-heavy` |
| No per-user rate limits | `cmd/api/middleware.go` | `message-storm` |
| Synchronous DB query per channel subscription | `internal/websocket/` | `ws-scale` |
| bcrypt latency under parallel logins | `internal/auth/` | `auth-flood` |
| No pagination limit on message history | `internal/message/` | `read-heavy` |

---

## Security Constraints

- `bench_handlers_stresstest.go` uses `//go:build stresstest` — absent from all production binaries
- Provision endpoint protected by `X-Bench-Secret` header (BENCH_SECRET env var)
- All test accounts created with a common prefix (`bench_` by default) for reliable cleanup
- `--cleanup true` by default — test data deleted after every run
- Never point `--host` at production; the tool has no safeguard against this (it's a dev tool)
