package api

import "github.com/dheerajb/routebite/internal/scoring"

// LatLng is a geographic coordinate.
type LatLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// SearchRequest is what a driving client POSTs to /v1/search.
type SearchRequest struct {
	Origin           LatLng `json:"origin" binding:"required"`
	Destination      LatLng `json:"destination" binding:"required"`
	VoiceText        string `json:"voice_text"`         // raw transcribed query, optional
	Cuisine          string `json:"cuisine,omitempty"`  // override if known (e.g. "soup")
	MaxDetourMinutes int    `json:"max_detour_minutes"` // hard filter, default 10
	MaxResults       int    `json:"max_results"`        // default 5
	OpenNowOnly      bool   `json:"open_now_only"`      // default true
}

// RouteSummary describes the base trip (no detour).
type RouteSummary struct {
	BaseDurationMin float64 `json:"base_duration_min"`
	DistanceKm      float64 `json:"distance_km"`
}

// SearchResponse is what we send back.
type SearchResponse struct {
	RouteSummary RouteSummary         `json:"route_summary"`
	Results      []scoring.Restaurant `json:"results"`
	VoiceSummary string               `json:"voice_summary"` // for clients to read aloud
}
