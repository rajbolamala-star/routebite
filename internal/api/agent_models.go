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
	Summary     string            `json:"summary"`
	Restaurants []AgentRestaurant `json:"restaurants"`
}

// AgentRestaurant is a simplified recommendation with a human-readable reason.
type AgentRestaurant struct {
	Name          string  `json:"name"`
	Rating        float64 `json:"rating"`
	DetourMinutes int     `json:"detour_minutes"`
	OpenNow       bool    `json:"open_now"`
	Address       string  `json:"address"`
	Phone         string  `json:"phone"`
	Reason        string  `json:"reason"`
}
