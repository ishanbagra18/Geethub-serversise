package controllers

import (
	"context"
	"math"
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ishanbagra18/ecommerce-using-go/database"
)

var ServerStartTime = time.Now()

// HealthResponse represents deep system health state
type HealthResponse struct {
	Status        string          `json:"status"`
	Timestamp     string          `json:"timestamp"`
	UptimeSeconds float64         `json:"uptime_seconds"`
	Database      DatabaseHealth  `json:"database"`
	System        SystemResources `json:"system"`
}

type DatabaseHealth struct {
	Status    string  `json:"status"`
	LatencyMs float64 `json:"latency_ms"`
	Error     string  `json:"error,omitempty"`
}

type SystemResources struct {
	GoVersion    string  `json:"go_version"`
	NumCPU       int     `json:"num_cpu"`
	NumGoroutine int     `json:"num_goroutine"`
	AllocMB      float64 `json:"alloc_mb"`
	TotalAllocMB float64 `json:"total_alloc_mb"`
	SysMB        float64 `json:"sys_mb"`
	NumGC        uint32  `json:"num_gc"`
}

// RuntimeMetricsResponse provides deep runtime telemetries for observability monitoring
type RuntimeMetricsResponse struct {
	Timestamp       string             `json:"timestamp"`
	UptimeSeconds   float64            `json:"uptime_seconds"`
	Goroutines      int                `json:"goroutines"`
	CPUCount        int                `json:"cpu_count"`
	Memory          MemoryMetrics      `json:"memory"`
	GarbageCollect  GCMetrics          `json:"garbage_collector"`
}

type MemoryMetrics struct {
	AllocBytes      uint64  `json:"alloc_bytes"`
	AllocMB         float64 `json:"alloc_mb"`
	TotalAllocBytes uint64  `json:"total_alloc_bytes"`
	SysBytes        uint64  `json:"sys_bytes"`
	SysMB           float64 `json:"sys_mb"`
	HeapAllocBytes  uint64  `json:"heap_alloc_bytes"`
	HeapSysBytes    uint64  `json:"heap_sys_bytes"`
	HeapIdleBytes   uint64  `json:"heap_idle_bytes"`
	HeapInuseBytes  uint64  `json:"heap_inuse_bytes"`
	HeapReleased    uint64  `json:"heap_released_bytes"`
	StackInuseBytes uint64  `json:"stack_inuse_bytes"`
}

type GCMetrics struct {
	NumGC         uint32   `json:"num_gc"`
	PauseTotalNs  uint64   `json:"pause_total_ns"`
	PauseTotalMs  float64  `json:"pause_total_ms"`
	RecentPauseNs []uint64 `json:"recent_pause_ns,omitempty"`
	NextGCBytes   uint64   `json:"next_gc_bytes"`
}

// HealthCheck returns overall deep health status including database connectivity
func HealthCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		uptime := time.Since(ServerStartTime).Seconds()

		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		dbHealth := DatabaseHealth{
			Status: "disconnected",
		}

		overallStatus := "ok"

		// Ping database
		if database.Client != nil {
			dbStart := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			err := database.Client.Ping(ctx, nil)
			dbLatency := float64(time.Since(dbStart).Microseconds()) / 1000.0
			dbHealth.LatencyMs = dbLatency

			if err == nil {
				dbHealth.Status = "connected"
			} else {
				dbHealth.Status = "error"
				dbHealth.Error = err.Error()
				overallStatus = "degraded"
			}
		} else {
			dbHealth.Status = "uninitialized"
			overallStatus = "degraded"
		}

		statusCode := http.StatusOK
		if overallStatus != "ok" {
			statusCode = http.StatusServiceUnavailable
		}

		c.JSON(statusCode, HealthResponse{
			Status:        overallStatus,
			Timestamp:     time.Now().UTC().Format(time.RFC3339),
			UptimeSeconds: math.Round(uptime*100) / 100,
			Database:      dbHealth,
			System: SystemResources{
				GoVersion:    runtime.Version(),
				NumCPU:       runtime.NumCPU(),
				NumGoroutine: runtime.NumGoroutine(),
				AllocMB:      math.Round(float64(memStats.Alloc)/1024/1024*100) / 100,
				TotalAllocMB: math.Round(float64(memStats.TotalAlloc)/1024/1024*100) / 100,
				SysMB:        math.Round(float64(memStats.Sys)/1024/1024*100) / 100,
				NumGC:        memStats.NumGC,
			},
		})
	}
}

// LivenessCheck returns fast HTTP 200 for container orchestrators (Kubernetes / Docker)
func LivenessCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "alive",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// ReadinessCheck confirms the service is ready to accept user traffic
func ReadinessCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		if database.Client == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not_ready",
				"reason": "database client is uninitialized",
			})
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := database.Client.Ping(ctx, nil); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not_ready",
				"reason": "database ping failed: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":    "ready",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// RuntimeMetrics returns real-time Go runtime memory, GC, and goroutine performance metrics
func RuntimeMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		uptime := time.Since(ServerStartTime).Seconds()

		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		// Get recent pause durations (up to last 5)
		recentPauses := make([]uint64, 0, 5)
		if m.NumGC > 0 {
			count := int(m.NumGC)
			if count > 5 {
				count = 5
			}
			for i := 0; i < count; i++ {
				idx := (int(m.NumGC) - 1 - i) % 256
				recentPauses = append(recentPauses, m.PauseNs[idx])
			}
		}

		c.JSON(http.StatusOK, RuntimeMetricsResponse{
			Timestamp:     time.Now().UTC().Format(time.RFC3339),
			UptimeSeconds: math.Round(uptime*100) / 100,
			Goroutines:    runtime.NumGoroutine(),
			CPUCount:      runtime.NumCPU(),
			Memory: MemoryMetrics{
				AllocBytes:      m.Alloc,
				AllocMB:         math.Round(float64(m.Alloc)/1024/1024*100) / 100,
				TotalAllocBytes: m.TotalAlloc,
				SysBytes:        m.Sys,
				SysMB:           math.Round(float64(m.Sys)/1024/1024*100) / 100,
				HeapAllocBytes:  m.HeapAlloc,
				HeapSysBytes:    m.HeapSys,
				HeapIdleBytes:   m.HeapIdle,
				HeapInuseBytes:  m.HeapInuse,
				HeapReleased:    m.HeapReleased,
				StackInuseBytes: m.StackInuse,
			},
			GarbageCollect: GCMetrics{
				NumGC:         m.NumGC,
				PauseTotalNs:  m.PauseTotalNs,
				PauseTotalMs:  float64(m.PauseTotalNs) / 1e6,
				RecentPauseNs: recentPauses,
				NextGCBytes:   m.NextGC,
			},
		})
	}
}
