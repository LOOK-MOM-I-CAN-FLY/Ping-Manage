package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type Result struct {
	URL      string
	Status   int
	Duration time.Duration
	Error    error
	When     time.Time
}

func newHTTPClient(timeout time.Duration) *http.Client {
	tr := &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		// other tuning if needed
	}
	return &http.Client{
		Transport: tr,
		Timeout:   timeout,
	}
}

func pingOnce(ctx context.Context, client *http.Client, url string) Result {
	start := time.Now()

	// prefer HEAD to reduce payload; fall back to GET if server doesn't like HEAD
	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	resp, err := client.Do(req)
	if err != nil {
		// try GET as fallback
		req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err = client.Do(req2)
	}
	if resp != nil {
		// fully close body to allow connection reuse
		// read at most a bit to avoid leaking connections (not required for HEAD)
		_ = resp.Body.Close()
	}

	elapsed := time.Since(start)
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}

	return Result{
		URL:      url,
		Status:   status,
		Duration: elapsed,
		Error:    err,
		When:     start,
	}
}

func pingWithRetries(ctx context.Context, client *http.Client, url string, maxRetries int) Result {
	var last Result
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// respect context cancellation
		if ctx.Err() != nil {
			return Result{URL: url, Error: ctx.Err(), When: time.Now()}
		}

		last = pingOnce(ctx, client, url)
		// consider success if no error and status < 500
		if last.Error == nil && (last.Status == 0 || last.Status < 500) {
			return last
		}

		// exponential backoff with jitter
		if attempt < maxRetries {
			base := float64(100 * (1 << attempt)) // ms
			jitter := rand.Float64() * base
			sleep := time.Duration(base+jitter) * time.Millisecond
			select {
			case <-time.After(sleep):
			case <-ctx.Done():
				return Result{URL: url, Error: ctx.Err(), When: time.Now()}
			}
		}
	}
	return last
}

func loadURLsFromFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var urls []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// ensure scheme
		if !strings.HasPrefix(line, "http://") && !strings.HasPrefix(line, "https://") {
			line = "https://" + line
		}
		urls = append(urls, line)
	}
	return urls, sc.Err()
}

func main() {
	var (
		urlsFile    = flag.String("urls", "urls.txt", "file with URLs (one per line). Lines starting with # ignored")
		concurrency = flag.Int("concurrency", 50, "max concurrent requests")
		rate        = flag.Int("rate", 0, "rate limit requests per second (0 = unlimited)")
		timeout     = flag.Duration("timeout", 5*time.Second, "HTTP request timeout")
		count       = flag.Int("count", 1, "how many pings per URL")
		interval    = flag.Duration("interval", 2*time.Second, "interval between ping rounds")
		retries     = flag.Int("retries", 2, "retries on failure (per request)")
	)
	flag.Parse()

	urls, err := loadURLsFromFile(*urlsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load urls: %v\n", err)
		os.Exit(1)
	}
	if len(urls) == 0 {
		fmt.Fprintln(os.Stderr, "no urls provided")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	// graceful shutdown on Ctrl+C
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Println("\nreceived interrupt, shutting down...")
		cancel()
	}()

	client := newHTTPClient(*timeout)

	// rate limiter token channel (if rate>0)
	var tokenCh chan struct{}
	if *rate > 0 {
		tokenCh = make(chan struct{}, *rate*2)
		intervalDur := time.Duration(float64(time.Second) / float64(*rate))
		ticker := time.NewTicker(intervalDur)
		go func() {
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// non-blocking add token
					select {
					case tokenCh <- struct{}{}:
					default:
					}
				}
			}
		}()
	}

	sem := make(chan struct{}, *concurrency)
	results := make(chan Result, 1000)

	var wg sync.WaitGroup

	var total int64
	var success int64
	var failed int64
	var sumNanos int64
	var minLatency int64 = math.MaxInt64
	var maxLatency int64

	var statsMu sync.Mutex

	// collector
	go func() {
		for r := range results {
			atomic.AddInt64(&total, 1)
			ok := (r.Error == nil) && (r.Status == 0 || r.Status < 400)
			if ok {
				atomic.AddInt64(&success, 1)
			} else {
				atomic.AddInt64(&failed, 1)
			}
			if r.Error == nil {
				n := r.Duration.Nanoseconds()
				atomic.AddInt64(&sumNanos, n)
				statsMu.Lock()
				if n < minLatency {
					minLatency = n
				}
				if n > maxLatency {
					maxLatency = n
				}
				statsMu.Unlock()
				fmt.Printf("[%s] %s %d in %v\n", r.When.Format("15:04:05"), r.URL, r.Status, r.Duration)
			} else {
				fmt.Printf("[%s] %s ERROR: %v\n", r.When.Format("15:04:05"), r.URL, r.Error)
			}
		}
	}()

	startAll := time.Now()
outer:
	for i := 0; i < *count; i++ {
		for _, u := range urls {
			// check cancellation
			select {
			case <-ctx.Done():
				break outer
			default:
			}

			wg.Add(1)
			// workers limited by sem
			go func(url string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				// rate limit token
				if tokenCh != nil {
					select {
					case <-ctx.Done():
						return
					case <-tokenCh:
					}
				}

				res := pingWithRetries(ctx, client, url, *retries)
				select {
				case <-ctx.Done():
					return
				case results <- res:
				}
			}(u)
		}

		// wait for this round if we want (optional). We'll wait for all goroutines spawned so far OR sleep interval
		// Simpler: sleep interval, but still allow CTRL+C to break
		if i < *count-1 {
			select {
			case <-ctx.Done():
				break outer
			case <-time.After(*interval):
			}
		}
	}

	wg.Wait()
	// close results and wait collector to finish
	close(results)

	totalDur := time.Since(startAll)
	// compute averages
	tot := atomic.LoadInt64(&total)
	succ := atomic.LoadInt64(&success)
	fail := atomic.LoadInt64(&failed)
	var avg time.Duration
	if tot > 0 && sumNanos > 0 {
		avg = time.Duration(sumNanos / tot)
	}

	fmt.Println("---- summary ----")
	fmt.Printf("requests: %d, success: %d, failed: %d\n", tot, succ, fail)
	if tot > 0 {
		fmt.Printf("avg latency: %v, min: %v, max: %v\n", avg, time.Duration(minLatency), time.Duration(maxLatency))
	}
	fmt.Printf("total runtime: %v\n", totalDur)
}
