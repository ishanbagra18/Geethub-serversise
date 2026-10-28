package controllers_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ishanbagra18/ecommerce-using-go/controllers"
	"github.com/ishanbagra18/ecommerce-using-go/middleware"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// BenchmarkHandler_Liveness measures raw routing, JSON serialization, and response overhead
func BenchmarkHandler_Liveness(b *testing.B) {
	router := gin.New()
	router.GET("/live", controllers.LivenessCheck())

	req := httptest.NewRequest("GET", "/live", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("Expected 200, got %d", w.Code)
		}
	}
}

// BenchmarkHandler_RuntimeMetrics measures memory introspection and JSON marshalling speed
func BenchmarkHandler_RuntimeMetrics(b *testing.B) {
	router := gin.New()
	router.GET("/metrics", controllers.RuntimeMetrics())

	req := httptest.NewRequest("GET", "/metrics", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("Expected 200, got %d", w.Code)
		}
	}
}

// BenchmarkMiddleware_RequestIDAndStructuredLogger measures overhead of request IDs + structured logging
func BenchmarkMiddleware_RequestIDAndStructuredLogger(b *testing.B) {
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(middleware.StructuredLogger(io.Discard))

	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	req := httptest.NewRequest("GET", "/ping", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("Expected 200, got %d", w.Code)
		}
	}
}
