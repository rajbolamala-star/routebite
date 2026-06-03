package middleware

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

// StructuredLogger emits one JSON log per request. Easy to ship to any
// log pipeline (Datadog, CloudWatch, Loki, Stackdriver) without parsing.
func StructuredLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		entry := map[string]interface{}{
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
			"method":     c.Request.Method,
			"path":       c.Request.URL.Path,
			"status":     c.Writer.Status(),
			"latency_ms": time.Since(start).Milliseconds(),
			"client_ip":  c.ClientIP(),
		}
		raw, _ := json.Marshal(entry)
		log.Println(string(raw))
	}
}
