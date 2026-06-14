package yelp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Business is the subset of Yelp Fusion business fields we use.
type Business struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Phone        string  `json:"phone"`
	DisplayPhone string  `json:"display_phone"`
	URL          string  `json:"url"`
	Rating       float64 `json:"rating"`
	ReviewCount  int     `json:"review_count"`
	Price        string  `json:"price"`
	IsClosed     bool    `json:"is_closed"`
	Categories   []struct {
		Title string `json:"title"`
		Alias string `json:"alias"`
	} `json:"categories"`
	Coordinates struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"coordinates"`
	Location struct {
		Address1 string `json:"address1"`
		City     string `json:"city"`
		State    string `json:"state"`
		ZipCode  string `json:"zip_code"`
	} `json:"location"`
}

// Client wraps the Yelp Fusion search endpoint.
type Client interface {
	Search(ctx context.Context, q SearchParams) ([]Business, error)
}

// SearchParams maps to the Yelp Fusion /businesses/search query.
type SearchParams struct {
	Term    string
	Lat     float64
	Lng     float64
	RadiusM int // capped at 40000 by Yelp
	Limit   int // max 50
	OpenNow bool
}

// --- HTTP implementation ---

type httpClient struct {
	apiKey string
	hc     *http.Client
	base   string
}

// New returns a Yelp client. apiKey can be empty in tests; pair with Mock.
func New(apiKey string) Client {
	return &httpClient{
		apiKey: apiKey,
		hc:     &http.Client{Timeout: 8 * time.Second},
		base:   "https://api.yelp.com/v3/businesses/search",
	}
}

type yelpResponse struct {
	Businesses []Business `json:"businesses"`
	Error      *struct {
		Code        string `json:"code"`
		Description string `json:"description"`
	} `json:"error"`
}

func (c *httpClient) Search(ctx context.Context, p SearchParams) ([]Business, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("yelp: missing API key")
	}

	u, _ := url.Parse(c.base)
	q := u.Query()
	if p.Term != "" {
		q.Set("term", p.Term)
	}
	q.Set("latitude", strconv.FormatFloat(p.Lat, 'f', 6, 64))
	q.Set("longitude", strconv.FormatFloat(p.Lng, 'f', 6, 64))
	if p.RadiusM > 0 {
		if p.RadiusM > 40000 {
			p.RadiusM = 40000
		}
		q.Set("radius", strconv.Itoa(p.RadiusM))
	}
	if p.Limit > 0 {
		if p.Limit > 50 {
			p.Limit = 50
		}
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.OpenNow {
		q.Set("open_now", "true")
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var out yelpResponse
	if err := json.Unmarshal(body, &out); err != nil {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("yelp: HTTP %d - %s", resp.StatusCode, http.StatusText(resp.StatusCode))
		}
		return nil, fmt.Errorf("yelp: could not decode response: %w", err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("yelp: %s - %s", out.Error.Code, out.Error.Description)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("yelp: HTTP %d - %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	return out.Businesses, nil
}

// --- Mock for offline dev / tests ---

type Mock struct{}

// NewMock returns a Yelp client that fabricates deterministic results based on
// the search center, so the rest of the pipeline can run without an API key.
func NewMock() Client { return &Mock{} }

func (Mock) Search(_ context.Context, p SearchParams) ([]Business, error) {
	// Generate a few synthetic restaurants near the center.
	// Spread them in a small grid so the scoring + detour math has signal.
	offsets := []struct {
		dLat, dLng float64
		rating     float64
		reviews    int
		name       string
		categories []string
		price      string
	}{
		{0.01, 0.01, 4.6, 412, "Pho 78", []string{"Vietnamese", "Soup"}, "$$"},
		{-0.012, 0.008, 4.4, 280, "Soup Spot", []string{"Soup", "American"}, "$"},
		{0.018, -0.006, 4.1, 88, "Pho Real", []string{"Vietnamese"}, "$$"},
		{-0.006, -0.014, 4.8, 1530, "Hot Pot House", []string{"Hot Pot", "Soup"}, "$$$"},
		{0.022, 0.020, 3.9, 64, "Lucky Bowl", []string{"Asian", "Soup"}, "$"},
	}

	out := []Business{}
	for i, o := range offsets {
		if p.Limit > 0 && i >= p.Limit {
			break
		}
		b := Business{
			ID:           fmt.Sprintf("mock-%d", i),
			Name:         o.name,
			Phone:        "+13055551200",
			DisplayPhone: "(305) 555-1200",
			URL:          "https://www.yelp.com/biz/mock-" + strconv.Itoa(i),
			Rating:       o.rating,
			ReviewCount:  o.reviews,
			Price:        o.price,
			IsClosed:     false,
		}
		for _, c := range o.categories {
			b.Categories = append(b.Categories, struct {
				Title string `json:"title"`
				Alias string `json:"alias"`
			}{Title: c, Alias: c})
		}
		b.Coordinates.Latitude = p.Lat + o.dLat
		b.Coordinates.Longitude = p.Lng + o.dLng
		b.Location.Address1 = fmt.Sprintf("%d Main St", 100+i*10)
		b.Location.City = "Hollywood"
		b.Location.State = "FL"
		b.Location.ZipCode = "33020"
		out = append(out, b)
	}
	return out, nil
}
