package controllers_test

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/ishanbagra18/ecommerce-using-go/controllers"
	"github.com/ishanbagra18/ecommerce-using-go/middleware"
)

// LoadTestStats holds computed throughput and percentile latencies
type LoadTestStats struct {
	Name            string        `json:"name"`
	TotalRequests   int           `json:"total_requests"`
	Concurrency     int           `json:"concurrency"`
	Duration        time.Duration `json:"duration"`
	RPS             float64       `json:"requests_per_sec"`
	P50LatencyMs    float64       `json:"p50_latency_ms"`
	P90LatencyMs    float64       `json:"p90_latency_ms"`
	P95LatencyMs    float64       `json:"p95_latency_ms"`
	P99LatencyMs    float64       `json:"p99_latency_ms"`
	MinLatencyMs    float64       `json:"min_latency_ms"`
	MaxLatencyMs    float64       `json:"max_latency_ms"`
	MeanLatencyMs   float64       `json:"mean_latency_ms"`
	ErrorCount      int           `json:"error_count"`
	ErrorRatePct    float64       `json:"error_rate_pct"`
	MemoryAllocMB   float64       `json:"memory_alloc_mb"`
	GoroutineDelta  int           `json:"goroutine_delta"`
}

// computePercentiles calculates p50, p90, p95, p99, min, max, and mean latencies
func computePercentiles(latencies []float64) (p50, p90, p95, p99, min, max, mean float64) {
	if len(latencies) == 0 {
		return 0, 0, 0, 0, 0, 0, 0
	}

	sort.Float64s(latencies)
	n := len(latencies)

	min = latencies[0]
	max = latencies[n-1]

	sum := 0.0
	for _, l := range latencies {
		sum += l
	}
	mean = sum / float64(n)

	p50 = latencies[int(math.Min(float64(n-1), float64(n)*0.50))]
	p90 = latencies[int(math.Min(float64(n-1), float64(n)*0.90))]
	p95 = latencies[int(math.Min(float64(n-1), float64(n)*0.95))]
	p99 = latencies[int(math.Min(float64(n-1), float64(n)*0.99))]

	return
}

// TestLoadTest_RESTApi simulates concurrent load on REST endpoints and measures p50/p95/p99 latencies
func TestLoadTest_RESTApi(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(controllers.LivenessCheck())

	const totalRequests = 2000
	const concurrency = 50

	var beforeMem runtime.MemStats
	runtime.ReadMemStats(&beforeMem)
	beforeGoroutines := runtime.NumGoroutine()

	reqChan := make(chan struct{}, totalRequests)
	for i := 0; i < totalRequests; i++ {
		reqChan <- struct{}{}
	}
	close(reqChan)

	var wg sync.WaitGroup
	var errCount int64
	latencies := make([]float64, totalRequests)
	var latencyIdx int64

	startTime := time.Now()

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/live", nil)
			for range reqChan {
				reqStart := time.Now()
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, req)
				reqDuration := float64(time.Since(reqStart).Microseconds()) / 1000.0

				if recorder.Code != http.StatusOK {
					atomic.AddInt64(&errCount, 1)
				}

				idx := atomic.AddInt64(&latencyIdx, 1) - 1
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
	afterGoroutines := runtime.NumGoroutine()

	p50, p90, p95, p99, min, max, mean := computePercentiles(latencies[:latencyIdx])
	rps := float64(latencyIdx) / totalDuration.Seconds()

	stats := LoadTestStats{
		Name:           "REST API (/live)",
		TotalRequests:  int(latencyIdx),
		Concurrency:    concurrency,
		Duration:       totalDuration,
		RPS:            math.Round(rps*10) / 10,
		P50LatencyMs:   math.Round(p50*1000) / 1000,
		P90LatencyMs:   math.Round(p90*1000) / 1000,
		P95LatencyMs:   math.Round(p95*1000) / 1000,
		P99LatencyMs:   math.Round(p99*1000) / 1000,
		MinLatencyMs:   math.Round(min*1000) / 1000,
		MaxLatencyMs:   math.Round(max*1000) / 1000,
		MeanLatencyMs:  math.Round(mean*1000) / 1000,
		ErrorCount:     int(errCount),
		ErrorRatePct:   (float64(errCount) / float64(latencyIdx)) * 100.0,
		MemoryAllocMB:  math.Round(float64(afterMem.Alloc)/1024/1024*100) / 100,
		GoroutineDelta: afterGoroutines - beforeGoroutines,
	}

	t.Logf("\n=======================================================")
	t.Logf("🚀 LOAD TEST RESULTS: %s", stats.Name)
	t.Logf("-------------------------------------------------------")
	t.Logf("Total Requests:  %d across %d concurrent workers", stats.TotalRequests, stats.Concurrency)
	t.Logf("Total Time:      %v", stats.Duration)
	t.Logf("Throughput:      %.1f req/sec", stats.RPS)
	t.Logf("Latency P50:     %.3f ms", stats.P50LatencyMs)
	t.Logf("Latency P90:     %.3f ms", stats.P90LatencyMs)
	t.Logf("Latency P95:     %.3f ms", stats.P95LatencyMs)
	t.Logf("Latency P99:     %.3f ms", stats.P99LatencyMs)
	t.Logf("Latency Min/Max: %.3f ms / %.3f ms (Mean: %.3f ms)", stats.MinLatencyMs, stats.MaxLatencyMs, stats.MeanLatencyMs)
	t.Logf("Errors:          %d (%.2f%%)", stats.ErrorCount, stats.ErrorRatePct)
	t.Logf("Memory Alloc:    %.2f MB", stats.MemoryAllocMB)
	t.Logf("Goroutine Delta: %+d", stats.GoroutineDelta)
	t.Logf("=======================================================\n")

	if stats.ErrorCount > 0 {
		t.Errorf("Expected 0 errors, got %d", stats.ErrorCount)
	}
	if stats.P99LatencyMs > 50.0 {
		t.Errorf("Expected P99 latency < 50ms, got %.3f ms", stats.P99LatencyMs)
	}
}

// TestLoadTest_WebSocketPartyBroadcast load-tests real-time party room message broadcasts to concurrent clients
func TestLoadTest_WebSocketPartyBroadcast(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.POST("/api/party/create", func(c *gin.Context) {
		c.Set("user_id", "host_loadtest")
		c.Set("first_name", "Host")
		controllers.CreatePartyRoom()(c)
	})
	router.GET("/ws/party/:room_id", controllers.ConnectPartyRoomWebSocket())

	server := httptest.NewServer(router)
	defer server.Close()

	// 1. Create Room
	resp, err := http.Post(server.URL+"/api/party/create", "application/json", nil)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	var createRes map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&createRes)
	resp.Body.Close()

	roomCode := createRes["room_code"].(string)

	// 2. Connect 30 concurrent listener clients
	const clientCount = 30
	const broadcastsCount = 30

	type clientConn struct {
		conn     *websocket.Conn
		messages chan map[string]interface{}
		closed   bool
		mu       sync.Mutex
	}

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/party/" + roomCode

	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	clients := make([]*clientConn, clientCount)

	for i := 0; i < clientCount; i++ {
		uID := fmt.Sprintf("listener_%d", i)
		conn, _, err := dialer.Dial(fmt.Sprintf("%s?user_id=%s&user_name=User_%d", wsURL, uID, i), nil)
		if err != nil {
			t.Fatalf("Client %d failed to connect: %v", i, err)
		}

		cc := &clientConn{
			conn:     conn,
			messages: make(chan map[string]interface{}, broadcastsCount*2),
		}
		clients[i] = cc

		go func(c *clientConn) {
			for {
				var msg map[string]interface{}
				if err := c.conn.ReadJSON(&msg); err != nil {
					return
				}
				c.mu.Lock()
				if !c.closed {
					select {
					case c.messages <- msg:
					default:
					}
				}
				c.mu.Unlock()
			}
		}(cc)
	}

	// Connect Host
	hostConn, _, err := dialer.Dial(fmt.Sprintf("%s?user_id=host_loadtest&user_name=Host", wsURL), nil)
	if err != nil {
		t.Fatalf("Host failed to connect: %v", err)
	}
	defer hostConn.Close()

	time.Sleep(100 * time.Millisecond) // Let all connections stabilize

	// 3. Broadcast playback state sync events and measure fanout delivery latency
	latencies := make([]float64, broadcastsCount)
	startBroadcast := time.Now()

	for b := 0; b < broadcastsCount; b++ {
		bStart := time.Now()
		expectedTrack := fmt.Sprintf("song_%d", b)
		syncMsg := map[string]interface{}{
			"type":                 "sync_playback",
			"track_id":             expectedTrack,
			"playback_position_ms": b * 1000,
			"is_playing":           true,
			"state_version":        float64(b + 1),
		}

		if err := hostConn.WriteJSON(syncMsg); err != nil {
			t.Fatalf("Failed to write broadcast: %v", err)
		}

		// Wait for all clients to receive this specific state update
		for i := 0; i < clientCount; i++ {
			matched := false
			for !matched {
				select {
				case msg := <-clients[i].messages:
					if msg["type"] == "playback_state" && msg["track_id"] == expectedTrack {
						matched = true
					}
				case <-time.After(3 * time.Second):
					t.Fatalf("Client %d timed out waiting for broadcast %d (%s)", i, b, expectedTrack)
				}
			}
		}

		latencies[b] = float64(time.Since(bStart).Microseconds()) / 1000.0
	}

	totalTime := time.Since(startBroadcast)

	// Clean up clients
	for _, c := range clients {
		c.mu.Lock()
		c.closed = true
		c.conn.Close()
		c.mu.Unlock()
	}

	totalMessagesFanout := broadcastsCount * clientCount
	fanoutThroughput := float64(totalMessagesFanout) / totalTime.Seconds()
	p50, p90, p95, p99, min, max, mean := computePercentiles(latencies)

	t.Logf("\n=======================================================")
	t.Logf("🎧 WEBSOCKET PARTY ROOM BROADCAST LOAD TEST")
	t.Logf("-------------------------------------------------------")
	t.Logf("Connected Clients:    %d concurrent WebSocket connections", clientCount)
	t.Logf("Broadcast Batches:    %d state update iterations", broadcastsCount)
	t.Logf("Total Messages Sent:  %d fanout deliveries (100%% received)", totalMessagesFanout)
	t.Logf("Total Time:           %v", totalTime)
	t.Logf("Fanout Throughput:    %.1f msgs/sec", fanoutThroughput)
	t.Logf("Broadcast Roundtrip:")
	t.Logf("  - Latency P50:      %.3f ms", p50)
	t.Logf("  - Latency P90:      %.3f ms", p90)
	t.Logf("  - Latency P95:      %.3f ms", p95)
	t.Logf("  - Latency P99:      %.3f ms", p99)
	t.Logf("  - Latency Min/Max:  %.3f ms / %.3f ms (Mean: %.3f ms)", min, max, mean)
	t.Logf("=======================================================\n")

	if p99 > 80.0 {
		t.Errorf("Expected P99 broadcast fanout latency < 80ms, got %.3f ms", p99)
	}
}
