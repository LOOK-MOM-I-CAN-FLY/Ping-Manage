# PingParallel — lightweight concurrent HTTP pinger

A small, fast, production-minded Go tool to **ping (HTTP HEAD/GET) many sites concurrently** with rate-limiting, retries, timeouts and graceful shutdown. Designed as an idiomatic goroutine-based concurrency example and an easy-to-adapt pet project.

---

## Features
- Concurrent HTTP pings using goroutines + semaphore (concurrency control)  
- Rate limiting (tokens per second)  
- Timeout and retries with exponential backoff + jitter  
- HEAD-first with GET fallback (reduces payload)  
- Reusable `http.Client`/Transport (connection pooling)  
- Graceful shutdown on SIGINT/SIGTERM  
- Compact output and final summary statistics

---

## Quickstart
1. Initialize module:
```bash
go mod init github.com/you/pingparallel

2. Create urls.txt (one URL per line, # for comments). Schemes auto-add https:// if missing.
 
3. Run:
```bash
go run main.go \
  -urls=urls.txt \
  -concurrency=50 \
  -rate=20 \
  -count=5 \
  -interval=3s \
  -timeout=4s \
  -retries=2
```


### Example output
[21:10:01] https://google.com 200 in 45.123ms
[21:10:01] https://github.com 200 in 120.456ms
[21:10:03] https://example.com ERROR: context deadline exceeded
---- summary ----
requests: 12, success: 11, failed: 1
avg latency: 90ms, min: 10ms, max: 300ms
total runtime: 15.234s
