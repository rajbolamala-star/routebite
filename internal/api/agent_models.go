package api

// AgentSearchRequest accepts either structured route fields or a natural
// language query that the rule-based agent can partially parse.
type AgentSearchRequest struct {
	Query            string `json:"query"`
	Start            string `json:"start"`
	Destination      string `json:"destination"`
	Preference       string `json:"preference"`
	MaxDetourMinutes int    `json:"max_detour_minutes"`
	OpenNowOnly      bool   `json:"open_now_only"`
}

// AgentSearchResponse is intentionally compact and voice-friendly for driver
// assistant clients.
type AgentSearchResponse struct {
	Summary           string            `json:"summary"`
	DriverSafeSummary string            `json:"driver_safe_summary"`
	MatchQuality      string            `json:"match_quality"`
	TripIntent        string            `json:"trip_intent"`
	BestPick          *AgentRestaurant  `json:"best_pick,omitempty"`
	Restaurants       []AgentRestaurant `json:"restaurants"`
}

// ScoreBreakdown explains the 0-100 RouteBite Score in interview-friendly,
// deterministic pieces.
type ScoreBreakdown struct {
	DetourScore          int `json:"detour_score"`
	RatingScore          int `json:"rating_score"`
	OpenNowScore         int `json:"open_now_score"`
	PreferenceMatchScore int `json:"preference_match_score"`
	ConvenienceScore     int `json:"convenience_score"`
}

// AgentRestaurant is a simplified recommendation with a human-readable reason.
type AgentRestaurant struct {
	Name           string         `json:"name"`
	Rating         float64        `json:"rating"`
	DetourMinutes  int            `json:"detour_minutes"`
	OpenNow        bool           `json:"open_now"`
	Address        string         `json:"address"`
	Phone          string         `json:"phone"`
	TapToCall      string         `json:"tap_to_call,omitempty"`
	OpenInMapsURL  string         `json:"open_in_maps_url,omitempty"`
	Reason         string         `json:"reason"`
	RouteBiteScore int            `json:"routebite_score"`
	ScoreBreakdown ScoreBreakdown `json:"score_breakdown"`
}
