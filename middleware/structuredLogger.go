package middleware

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// StructuredLogEntry represents a canonical access log record
type StructuredLogEntry struct {
	Timestamp string  `json:"timestamp"`
	RequestID string  `json:"request_id,omitempty"`
	ClientIP  string  `json:"client_ip"`
	Method    string  `json:"method"`
	Path      string  `json:"path"`
	Query     string  `json:"query,omitempty"`
	Status    int     `json:"status"`
	LatencyMs float64 `json:"latency_ms"`
	BytesOut  int     `json:"bytes_out"`
	UserID    string  `json:"user_id,omitempty"`
	UserAgent string  `json:"user_agent,omitempty"`
	Error     string  `json:"error,omitempty"`
}

// StructuredLogger returns a Gin middleware that records every request as structured JSON
func StructuredLogger(out io.Writer) gin.HandlerFunc {
	if out == nil {
		out = os.Stdout
	}

	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		rawQuery := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		latencyMs := float64(latency.Microseconds()) / 1000.0

		reqID := GetRequestID(c)
		userID := c.GetString("user_id")

		errStr := ""
		if len(c.Errors) > 0 {
			errStr = c.Errors.String()
		}

		entry := StructuredLogEntry{
			Timestamp: start.UTC().Format(time.RFC3339Nano),
			RequestID: reqID,
			ClientIP:  c.ClientIP(),
			Method:    c.Request.Method,
			Path:      path,
			Query:     rawQuery,
			Status:    c.Writer.Status(),
			LatencyMs: latencyMs,
			BytesOut:  c.Writer.Size(),
			UserID:    userID,
			UserAgent: c.Request.UserAgent(),
			Error:     errStr,
		}

		data, err := json.Marshal(entry)
		if err == nil {
			fmt.Fprintf(out, "%s\n", data)
		} else {
			// Fallback text log
			fmt.Fprintf(out, "[%s] %s %s | %d | %.2fms | req_id=%s\n",
				entry.Timestamp, entry.Method, entry.Path, entry.Status, entry.LatencyMs, entry.RequestID)
		}
	}
}
