package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// RequestIDHeader is returned on every response and accepted from callers
	// that already have an upstream trace/correlation ID.
	RequestIDHeader = "X-Request-ID"
	requestIDKey    = "request_id"
)

type requestIDContextKey struct{}

// RequestID attaches a stable request ID to the Gin context and response.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(RequestIDHeader)
		if requestID == "" {
			requestID = newRequestID()
		}
		c.Set(requestIDKey, requestID)
		c.Request = c.Request.WithContext(ContextWithRequestID(c.Request.Context(), requestID))
		c.Writer.Header().Set(RequestIDHeader, requestID)
		c.Next()
	}
}

// GetRequestID returns the request ID stored by RequestID middleware.
func GetRequestID(c *gin.Context) string {
	if v, ok := c.Get(requestIDKey); ok {
		if requestID, ok := v.(string); ok {
			return requestID
		}
	}
	return c.GetHeader(RequestIDHeader)
}

func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}

func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDContextKey{}).(string); ok {
		return v
	}
	return ""
}

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
			"request_id": GetRequestID(c),
		}
		raw, _ := json.Marshal(entry)
		log.Println(string(raw))
	}
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b[:])
}
