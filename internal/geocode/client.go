package geocode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Point is a geographic coordinate returned from geocoding.
type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// Suggestion is one address/place candidate.
type Suggestion struct {
	Label string `json:"label"`
	Point Point  `json:"point"`
}

// Client resolves human-readable places into coordinates.
type Client interface {
	Search(ctx context.Context, q string, limit int) ([]Suggestion, error)
}

// Nominatim uses OpenStreetMap's public geocoder. It is good for MVP demos;
// production should move to Mapbox, Google Places, or a paid Nominatim host.
type Nominatim struct {
	hc        *http.Client
	base      string
	userAgent string
}

func NewNominatim() Client {
	return &Nominatim{
		hc:        &http.Client{Timeout: 8 * time.Second},
		base:      "https://nominatim.openstreetmap.org/search",
		userAgent: "routebite-mvp/0.1",
	}
}

type nominatimItem struct {
	DisplayName string `json:"display_name"`
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
}

func (n *Nominatim) Search(ctx context.Context, q string, limit int) ([]Suggestion, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 8 {
		limit = 5
	}

	u, _ := url.Parse(n.base)
	params := u.Query()
	params.Set("q", q)
	params.Set("format", "jsonv2")
	params.Set("addressdetails", "0")
	params.Set("limit", strconv.Itoa(limit))
	params.Set("countrycodes", "us")
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", n.userAgent)

	resp, err := n.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("geocode: nominatim returned %s", resp.Status)
	}

	var raw []nominatimItem
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("geocode decode: %w", err)
	}

	out := make([]Suggestion, 0, len(raw))
	for _, item := range raw {
		lat, latErr := strconv.ParseFloat(item.Lat, 64)
		lng, lngErr := strconv.ParseFloat(item.Lon, 64)
		if latErr != nil || lngErr != nil || item.DisplayName == "" {
			continue
		}
		out = append(out, Suggestion{
			Label: item.DisplayName,
			Point: Point{Lat: lat, Lng: lng},
		})
	}
	return out, nil
}

// Mock is deterministic and keeps local development useful without network.
type Mock struct{}

func NewMock() Client { return &Mock{} }

func (Mock) Search(_ context.Context, q string, limit int) ([]Suggestion, error) {
	q = strings.ToLower(strings.TrimSpace(q))
	if limit <= 0 || limit > 8 {
		limit = 5
	}

	all := []Suggestion{
		{Label: "North Miami Beach, FL", Point: Point{Lat: 25.929, Lng: -80.169}},
		{Label: "Sunrise, FL", Point: Point{Lat: 26.133, Lng: -80.29}},
		{Label: "Downtown Los Angeles, CA", Point: Point{Lat: 34.0522, Lng: -118.2437}},
		{Label: "Santa Monica, CA", Point: Point{Lat: 34.0195, Lng: -118.4912}},
		{Label: "Brooklyn, NY", Point: Point{Lat: 40.6782, Lng: -73.9442}},
		{Label: "Newark Liberty International Airport, NJ", Point: Point{Lat: 40.6895, Lng: -74.1745}},
	}

	out := make([]Suggestion, 0, limit)
	for _, item := range all {
		if q == "" || strings.Contains(strings.ToLower(item.Label), q) {
			out = append(out, item)
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}
