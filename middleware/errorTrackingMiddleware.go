package middleware

import (
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
)

// ErrorResponse represents a standardized API error response correlated with a Request ID
type ErrorResponse struct {
	Error     string `json:"error"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Timestamp string `json:"timestamp"`
}

// ErrorTrackingRecovery intercepts unhandled panics, logs the stack trace with Request ID,
// and returns a structured 500 error payload to the caller
func ErrorTrackingRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				reqID := GetRequestID(c)
				stack := string(debug.Stack())

				log.Printf("🚨 [PANIC RECOVERED] RequestID: %s | Error: %v\nStack Trace:\n%s\n", reqID, r, stack)

				c.AbortWithStatusJSON(http.StatusInternalServerError, ErrorResponse{
					Error:     "Internal Server Error",
					Code:      "INTERNAL_SERVER_ERROR",
					Message:   fmt.Sprintf("An unexpected server error occurred. Correlation ID: %s", reqID),
					RequestID: reqID,
					Timestamp: time.Now().UTC().Format(time.RFC3339),
				})
			}
		}()

		c.Next()
	}
}
