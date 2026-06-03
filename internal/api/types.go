package api

// LatLng is a geographic coordinate.
type LatLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// SearchRequest is what a driving client POSTs to /v1/search.
type SearchRequest struct {
	Origin             LatLng  `json:"origin" binding:"required"`
	Destination        LatLng  `json:"destination" binding:"required"`
	VoiceText          string  `json:"voice_text"`           // raw transcribed query, optional
	Cuisine            string  `json:"cuisine,omitempty"`    // override if known (e.g. "soup")
	MaxDetourMinutes   int     `json:"max_detour_minutes"`   // hard filter, default 10
	MaxResults         int     `json:"max_results"`          // default 5
	OpenNowOnly        bool    `json:"open_now_only"`        // default true
}

// RouteSummary describes the base trip (no detour).
type RouteSummary struct {
	BaseDurationMin float64 `json:"base_duration_min"`
	DistanceKm      float64 `json:"distance_km"`
}

// Restaurant is one ranked result.
type Restaurant struct {
	Name              string   `json:"name"`
	Rating            float64  `json:"rating"`
	ReviewCount       int      `json:"review_count"`
	Phone             string   `json:"phone"`
	CallLink          string   `json:"call_link"`           // tel:+1...
	YelpURL           string   `json:"yelp_url"`
	Address           string   `json:"address"`
	Location          LatLng   `json:"location"`
	DistanceFromRouteM int     `json:"distance_from_route_m"`
	ExtraMinutes      int      `json:"extra_minutes"`
	Cuisine           []string `json:"cuisine"`
	Price             string   `json:"price"`               // "$", "$$", "$$$"
	IsOpenNow         bool     `json:"is_open_now"`
	Score             float64  `json:"score"`               // 0.0 - 1.0
}

// SearchResponse is what we send back.
type SearchResponse struct {
	RouteSummary RouteSummary  `json:"route_summary"`
	Results      []Restaurant  `json:"results"`
	VoiceSummary string        `json:"voice_summary"`        // for clients to read aloud
}
