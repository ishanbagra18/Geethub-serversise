package main

import (
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	targetURL := flag.String("url", "http://localhost:9000/live", "Target endpoint to load test")
	concurrency := flag.Int("c", 50, "Number of concurrent workers")
	totalRequests := flag.Int("n", 2000, "Total number of HTTP requests to dispatch")
	flag.Parse()

	fmt.Printf("\n🚀 GeetHub Load Test Runner\n")
	fmt.Printf("Target:      %s\n", *targetURL)
	fmt.Printf("Workers:     %d\n", *concurrency)
	fmt.Printf("Requests:    %d\n", *totalRequests)
	fmt.Println("-------------------------------------------------------")

	var beforeMem runtime.MemStats
	runtime.ReadMemStats(&beforeMem)

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        1000,
			MaxIdleConnsPerHost: 500,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	reqChan := make(chan struct{}, *totalRequests)
	for i := 0; i < *totalRequests; i++ {
		reqChan <- struct{}{}
	}
	close(reqChan)

	var wg sync.WaitGroup
	var errCount int64
	var completedCount int64
	latencies := make([]float64, *totalRequests)

	startTime := time.Now()

	for w := 0; w < *concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range reqChan {
				reqStart := time.Now()
				resp, err := client.Get(*targetURL)
				reqDuration := float64(time.Since(reqStart).Microseconds()) / 1000.0

				if err != nil || resp.StatusCode >= 400 {
					atomic.AddInt64(&errCount, 1)
				}
				if resp != nil {
					_, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}

				idx := atomic.AddInt64(&completedCount, 1) - 1
				if idx < int64(len(latencies)) {
					latencies[idx] = reqDuration
				}
			}
		}()
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	var afterMem runtime.MemStats
	runtime.ReadMemStats(&afterMem)

	// Percentile calculation
	actualCount := int(completedCount)
	validLatencies := latencies[:actualCount]
	sort.Float64s(validLatencies)

	min := validLatencies[0]
	max := validLatencies[actualCount-1]
	sum := 0.0
	for _, l := range validLatencies {
		sum += l
	}
	mean := sum / float64(actualCount)

	p50 := validLatencies[int(float64(actualCount)*0.50)]
	p90 := validLatencies[int(float64(actualCount)*0.90)]
	p95 := validLatencies[int(float64(actualCount)*0.95)]
	p99 := validLatencies[int(float64(actualCount)*0.99)]

	rps := float64(actualCount) / totalDuration.Seconds()

	fmt.Printf("\n📊 Benchmark Results Summary:\n")
	fmt.Printf("Completed:    %d / %d requests in %v\n", actualCount, *totalRequests, totalDuration)
	fmt.Printf("Throughput:   %.2f req/sec (RPS)\n", rps)
	fmt.Printf("Success Rate: %.2f%%\n", (1.0-float64(errCount)/float64(actualCount))*100.0)
	fmt.Printf("Latency (ms):\n")
	fmt.Printf("  • p50:      %.3f ms\n", p50)
	fmt.Printf("  • p90:      %.3f ms\n", p90)
	fmt.Printf("  • p95:      %.3f ms\n", p95)
	fmt.Printf("  • p99:      %.3f ms\n", p99)
	fmt.Printf("  • Min/Max:  %.3f ms / %.3f ms (Mean: %.3f ms)\n", min, max, mean)
	fmt.Printf("Memory:\n")
	fmt.Printf("  • Alloc:    %.2f MB (Sys: %.2f MB)\n", float64(afterMem.Alloc)/1024/1024, float64(afterMem.Sys)/1024/1024)
	fmt.Printf("  • NumGC:    %d cycles\n", afterMem.NumGC-beforeMem.NumGC)
	fmt.Printf("-------------------------------------------------------\n\n")
}
