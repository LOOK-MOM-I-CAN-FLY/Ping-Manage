PingParallel — lightweight concurrent HTTP pinger

A small, fast, production-minded Go tool to ping (HTTP HEAD/GET) many sites concurrently with rate-limiting, retries, timeouts and graceful shutdown. Designed as a clear example of idiomatic goroutine-based concurrency and an easy-to-adapt pet project.

Features

Concurrent HTTP pings using goroutines + semaphore (concurrency control)

Rate limiting (tokens per second)

Timeout and retries with exponential backoff + jitter

HEAD-first with GET fallback (reduces payload)

Reusable http.Client/Transport (connection pooling)

Graceful shutdown on SIGINT/SIGTERM

Compact output and final summary statistics

Quickstart

Create module:

go mod init github.com/you/pingparallel


Put URLs (one per line) into urls.txt (comments with # allowed).

Run:

go run main.go -urls=urls.txt -concurrency=50 -rate=20 -count=5 -interval=3s -timeout=4s -retries=2

Example output
[21:10:01] https://google.com 200 in 45.123ms
[21:10:01] https://github.com 200 in 120.456ms
[21:10:03] https://example.com ERROR: context deadline exceeded
---- summary ----
requests: 12, success: 11, failed: 1
avg latency: 90ms, min: 10ms, max: 300ms
total runtime: 15.234s

CLI flags (common)

-urls (string) file of URLs (default urls.txt)

-concurrency (int) max concurrent requests (default 50)

-rate (int) requests per second (0 = unlimited)

-count (int) how many rounds per URL

-interval (duration) wait between rounds

-timeout (duration) request timeout

-retries (int) retries per request

Architecture (diagram)

Mermaid (if your viewer supports it):

flowchart LR
  A[urls.txt] --> B[Dispatcher]
  B --> |tokens / semaphore| C[Worker goroutines]
  C --> D[HTTP Client (shared)]
  D --> E[Result Collector]
  E --> F[Console / Summary]
  subgraph Retry/Backoff
    C --> G[Exponential backoff + jitter]
  end


ASCII fallback:

urls.txt
   │
   ▼
Dispatcher (rate limiter + semaphore)
   │
   ▼
Worker goroutines ──▶ shared http.Client
   │                        │
   └─ retries/backoff ◀─────┘
   │
   ▼
Result collector -> Console + final stats

Why this design (quick)

Semaphore + workers prevent resource exhaustion.

Shared http.Client reuses connections (performance).

Rate limiting prevents accidental flood of targets.

Retries with jitter improve success under transient failures.

Graceful shutdown avoids lost logs and leaks.

Extensions / next steps

Add ICMP ping option (with go-ping)

Persist results to CSV/JSON or expose Prometheus metrics

Add a small web UI with history/graphs

Dockerfile for easy deployment
