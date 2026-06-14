package yelp

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func testClient(t *testing.T, status int, body string, inspect func(*http.Request)) *httpClient {
	t.Helper()
	c := New("test-key").(*httpClient)
	c.base = "https://example.test/businesses/search"
	c.hc = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if inspect != nil {
				inspect(r)
			}
			return &http.Response{
				StatusCode: status,
				Status:     http.StatusText(status),
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    r,
			}, nil
		}),
	}
	return c
}

func TestHTTPClientSearchSuccess(t *testing.T) {
	c := testClient(t, http.StatusOK, `{"businesses":[{"id":"1","name":"Real Soup","image_url":"https://img.example/soup.jpg","rating":4.7,"review_count":321}]}`, func(r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization header = %q, want Bearer test-key", got)
		}
		if got := r.URL.Query().Get("term"); got != "soup" {
			t.Fatalf("term = %q, want soup", got)
		}
	})

	got, err := c.Search(context.Background(), SearchParams{
		Term:    "soup",
		Lat:     25.9,
		Lng:     -80.1,
		RadiusM: 5000,
		Limit:   3,
		OpenNow: true,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "Real Soup" || got[0].ImageURL == "" {
		t.Fatalf("Search() = %+v, want Real Soup", got)
	}
}

func TestHTTPClientSearchYelpErrorBody(t *testing.T) {
	c := testClient(t, http.StatusUnauthorized, `{"error":{"code":"TOKEN_INVALID","description":"Invalid API key"}}`, nil)

	_, err := c.Search(context.Background(), SearchParams{Term: "pizza", Lat: 25, Lng: -80})
	if err == nil || !strings.Contains(err.Error(), "TOKEN_INVALID") {
		t.Fatalf("Search() error = %v, want TOKEN_INVALID", err)
	}
}

func TestHTTPClientSearchNonJSONError(t *testing.T) {
	c := testClient(t, http.StatusBadGateway, "bad gateway", nil)

	_, err := c.Search(context.Background(), SearchParams{Term: "coffee", Lat: 25, Lng: -80})
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("Search() error = %v, want HTTP 502", err)
	}
}

func TestHTTPClientSearchNon2xxWithoutYelpError(t *testing.T) {
	c := testClient(t, http.StatusTooManyRequests, `{"businesses":[]}`, nil)

	_, err := c.Search(context.Background(), SearchParams{Term: "ramen", Lat: 25, Lng: -80})
	if err == nil || !strings.Contains(err.Error(), "HTTP 429") {
		t.Fatalf("Search() error = %v, want HTTP 429", err)
	}
}
