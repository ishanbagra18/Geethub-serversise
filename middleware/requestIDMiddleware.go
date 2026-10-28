package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
)

const RequestIDHeader = "X-Request-ID"
const RequestIDContextKey = "request_id"

// generateRequestID produces a cryptographically sound, time-ordered unique request identifier
func generateRequestID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp if random read fails
		return fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x-%x", time.Now().UnixMilli(), hex.EncodeToString(bytes))
}

// RequestID injects or propagates X-Request-ID into the Gin context and response headers
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader(RequestIDHeader)
		if reqID == "" {
			reqID = generateRequestID()
		}

		c.Set(RequestIDContextKey, reqID)
		c.Writer.Header().Set(RequestIDHeader, reqID)

		c.Next()
	}
}

// GetRequestID extracts the request ID from a Gin context
func GetRequestID(c *gin.Context) string {
	if val, exists := c.Get(RequestIDContextKey); exists {
		if id, ok := val.(string); ok {
			return id
		}
	}
	return ""
}
