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
