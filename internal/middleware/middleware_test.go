package middleware

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestID_GeneratesHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/ping", func(c *gin.Context) {
		if GetRequestID(c) == "" {
			t.Fatal("request id missing from context")
		}
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))

	if got := w.Header().Get(RequestIDHeader); got == "" {
		t.Fatal("response request id header is empty")
	}
}

func TestRequestID_PreservesIncomingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(RequestIDHeader, "client-request-123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get(RequestIDHeader); got != "client-request-123" {
		t.Fatalf("request id header = %q, want client-request-123", got)
	}
}

func TestStructuredLogger_IncludesRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	original := log.Writer()
	originalFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(original)
		log.SetFlags(originalFlags)
	}()

	r := gin.New()
	r.Use(RequestID(), StructuredLogger())
	r.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(RequestIDHeader, "log-request-123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &got); err != nil {
		t.Fatalf("decode log: %v; raw=%s", err, logs.String())
	}
	if got["request_id"] != "log-request-123" {
		t.Fatalf("request_id = %v, want log-request-123", got["request_id"])
	}
	if got["method"] != http.MethodGet || got["path"] != "/ping" {
		t.Fatalf("unexpected log fields: %+v", got)
	}
}
