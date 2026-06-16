package api

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/dheerajb/routebite/internal/geocode"
	"github.com/dheerajb/routebite/internal/scoring"
	"github.com/dheerajb/routebite/internal/voice"
)

const agentResultLimit = 3
const (
	defaultAgentMaxDetour = 10
	maxAgentDetour        = 30
)

var (
	fromToPattern = regexp.MustCompile(`(?i)\bfrom\s+(.+?)\s+to\s+(.+?)(?:\s+(?:and|with|while|looking|want|need|craving|for)\b|$)`)
	detourPattern = regexp.MustCompile(`(?i)(?:less than|under|within|no more than|max(?:imum)?)\s+(\d+)\s*(?:minute|min)?`)
)

type agentParser interface {
	Parse(AgentSearchRequest) agentPlan
}

type ruleBasedAgentParser struct{}

func (h *Handler) runAgentSearch(ctx context.Context, req AgentSearchRequest) (AgentSearchResponse, agentPlan, *apiSearchError) {
	plan := h.agent.Parse(req)
	if plan.Start == "" {
		return AgentSearchResponse{}, plan, badRequest("agent needs a start location")
	}
	if plan.Destination == "" {
		return AgentSearchResponse{}, plan, badRequest("agent needs a destination")
	}
	if plan.Preference == "" {
		return AgentSearchResponse{}, plan, badRequest("agent needs a food preference")
	}

	origin, err := h.geocodeOne(ctx, plan.Start)
	if err != nil {
		return AgentSearchResponse{}, plan, badGateway("start geocoding failed", err)
	}
	destination, err := h.geocodeOne(ctx, plan.Destination)
	if err != nil {
		return AgentSearchResponse{}, plan, badGateway("destination geocoding failed", err)
	}

	searchResp, searchErr := h.runSearch(ctx, SearchRequest{
		Origin:           LatLng{Lat: origin.Point.Lat, Lng: origin.Point.Lng},
		Destination:      LatLng{Lat: destination.Point.Lat, Lng: destination.Point.Lng},
		VoiceText:        plan.Preference,
		Cuisine:          plan.Preference,
		MaxDetourMinutes: plan.MaxDetourMinutes,
		MaxResults:       agentResultLimit,
		OpenNowOnly:      plan.OpenNowOnly,
	})
	if searchErr != nil {
		return AgentSearchResponse{}, plan, searchErr
	}

	restaurants := make([]AgentRestaurant, 0, len(searchResp.Results))
	for _, result := range searchResp.Results {
		restaurants = append(restaurants, toAgentRestaurant(result, plan.Preference, plan.MaxDetourMinutes))
	}
	sortAgentRestaurants(restaurants)
	bestPick := bestAgentPick(restaurants)

	return AgentSearchResponse{
		Summary:     agentSummary(restaurants, plan.Preference),
		BestPick:    bestPick,
		Restaurants: restaurants,
	}, plan, nil
}

func (h *Handler) geocodeOne(ctx context.Context, place string) (geocode.Suggestion, error) {
	results, err := h.cachedGeocode(ctx, place, 1)
	if err != nil {
		return geocode.Suggestion{}, err
	}
	if len(results) == 0 {
		return geocode.Suggestion{}, fmt.Errorf("no location found for %q", place)
	}
	return results[0], nil
}

type agentPlan struct {
	Start            string
	Destination      string
	Preference       string
	MaxDetourMinutes int
	OpenNowOnly      bool
}

func parseAgentRequest(req AgentSearchRequest) agentPlan {
	return ruleBasedAgentParser{}.Parse(req)
}

func (ruleBasedAgentParser) Parse(req AgentSearchRequest) agentPlan {
	query := strings.TrimSpace(req.Query)
	plan := agentPlan{
		Start:            strings.TrimSpace(req.Start),
		Destination:      strings.TrimSpace(req.Destination),
		Preference:       strings.TrimSpace(req.Preference),
		MaxDetourMinutes: req.MaxDetourMinutes,
		OpenNowOnly:      true,
	}
	if req.OpenNowOnly {
		plan.OpenNowOnly = true
	}

	if query != "" {
		if plan.Start == "" || plan.Destination == "" {
			if match := fromToPattern.FindStringSubmatch(query); len(match) == 3 {
				if plan.Start == "" {
					plan.Start = cleanPlace(match[1])
				}
				if plan.Destination == "" {
					plan.Destination = cleanPlace(match[2])
				}
			}
		}
		if plan.Preference == "" {
			plan.Preference = parsePreference(query)
		}
		if plan.MaxDetourMinutes <= 0 {
			if match := detourPattern.FindStringSubmatch(query); len(match) == 2 {
				if mins, err := strconv.Atoi(match[1]); err == nil {
					plan.MaxDetourMinutes = mins
				}
			}
		}
		if strings.Contains(strings.ToLower(query), "any time") || strings.Contains(strings.ToLower(query), "doesn't matter") {
			plan.OpenNowOnly = false
		}
	}

	plan.MaxDetourMinutes = normalizeDetour(plan.MaxDetourMinutes)
	plan.Preference = normalizePreference(plan.Preference)
	return plan
}

func parsePreference(query string) string {
	lc := strings.ToLower(query)
	for _, marker := range []string{"want ", "looking for ", "find me ", "find ", "craving ", "need ", " for "} {
		if i := strings.Index(lc, marker); i >= 0 {
			rest := query[i+len(marker):]
			for _, stop := range []string{" with ", " under ", " less than ", " within ", " no more than ", " along ", " on the way"} {
				if j := strings.Index(strings.ToLower(rest), stop); j > 0 {
					rest = rest[:j]
				}
			}
			rest = strings.TrimSpace(strings.Trim(rest, ".?!,"))
			if rest != "" {
				return rest
			}
		}
	}

	intent := voice.Parse(query)
	return intent.Cuisine
}

func normalizeDetour(minutes int) int {
	if minutes <= 0 {
		return defaultAgentMaxDetour
	}
	if minutes > maxAgentDetour {
		return maxAgentDetour
	}
	return minutes
}

func normalizePreference(preference string) string {
	preference = strings.TrimSpace(strings.Trim(preference, ".?!,"))
	if strings.HasSuffix(strings.ToLower(preference), " food") {
		preference = strings.TrimSpace(preference[:len(preference)-len(" food")])
	}
	return preference
}

func cleanPlace(place string) string {
	place = strings.TrimSpace(strings.Trim(place, ".?!,"))
	if strings.HasPrefix(strings.ToLower(place), "driving ") {
		place = strings.TrimSpace(place[len("driving "):])
	}
	return strings.TrimSpace(place)
}

func toAgentRestaurant(result scoring.Restaurant, preference string, maxDetourMinutes int) AgentRestaurant {
	breakdown := routeBiteScoreBreakdown(result, preference, maxDetourMinutes)
	routeBiteScore := routeBiteScore(breakdown)
	restaurant := AgentRestaurant{
		Name:           result.Name,
		Rating:         result.Rating,
		DetourMinutes:  result.ExtraMinutes,
		OpenNow:        result.IsOpenNow,
		Address:        result.Address,
		Phone:          result.Phone,
		RouteBiteScore: routeBiteScore,
		ScoreBreakdown: breakdown,
	}
	restaurant.Reason = agentReason(restaurant)
	return restaurant
}

func routeBiteScoreBreakdown(result scoring.Restaurant, preference string, maxDetourMinutes int) ScoreBreakdown {
	return ScoreBreakdown{
		DetourScore:          detourScore(result.ExtraMinutes, maxDetourMinutes),
		RatingScore:          ratingScore(result.Rating),
		OpenNowScore:         openNowScore(result.IsOpenNow),
		PreferenceMatchScore: preferenceMatchScore(result, preference),
		ConvenienceScore:     convenienceScore(result),
	}
}

func routeBiteScore(b ScoreBreakdown) int {
	score := float64(b.DetourScore)*0.35 +
		float64(b.RatingScore)*0.25 +
		float64(b.OpenNowScore)*0.15 +
		float64(b.PreferenceMatchScore)*0.15 +
		float64(b.ConvenienceScore)*0.10
	return clampScore(int(math.Round(score)))
}

func detourScore(detourMinutes int, maxDetourMinutes int) int {
	if maxDetourMinutes <= 0 {
		maxDetourMinutes = defaultAgentMaxDetour
	}
	if detourMinutes <= 0 {
		return 100
	}
	score := 100 - int(math.Round((float64(detourMinutes)/float64(maxDetourMinutes))*100))
	return clampScore(score)
}

func ratingScore(rating float64) int {
	return clampScore(int(math.Round((rating / 5.0) * 100)))
}

func openNowScore(openNow bool) int {
	if openNow {
		return 100
	}
	return 0
}

func preferenceMatchScore(result scoring.Restaurant, preference string) int {
	preference = strings.ToLower(strings.TrimSpace(preference))
	if preference == "" {
		return 70
	}

	terms := strings.Fields(preference)
	if len(terms) == 0 {
		return 70
	}

	haystack := strings.ToLower(result.Name + " " + strings.Join(result.Cuisine, " "))
	matches := 0
	considered := 0
	for _, term := range terms {
		term = strings.Trim(term, ".,!?")
		if term == "" || term == "food" {
			continue
		}
		considered++
		if strings.Contains(haystack, term) {
			matches++
		}
	}
	if considered == 0 {
		return 70
	}
	if matches == 0 {
		return 60
	}
	if matches == considered {
		return 100
	}
	return 80
}

func convenienceScore(result scoring.Restaurant) int {
	score := 30
	if result.Address != "" {
		score += 35
	}
	if result.Phone != "" {
		score += 35
	}
	return clampScore(score)
}

func clampScore(score int) int {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func sortAgentRestaurants(restaurants []AgentRestaurant) {
	sort.SliceStable(restaurants, func(i, j int) bool {
		if restaurants[i].RouteBiteScore != restaurants[j].RouteBiteScore {
			return restaurants[i].RouteBiteScore > restaurants[j].RouteBiteScore
		}
		if restaurants[i].DetourMinutes != restaurants[j].DetourMinutes {
			return restaurants[i].DetourMinutes < restaurants[j].DetourMinutes
		}
		return restaurants[i].Rating > restaurants[j].Rating
	})
}

func bestAgentPick(restaurants []AgentRestaurant) *AgentRestaurant {
	if len(restaurants) == 0 {
		return nil
	}
	best := restaurants[0]
	best.Reason = bestPickReason(best)
	return &best
}

func agentReason(result AgentRestaurant) string {
	parts := []string{}
	if result.DetourMinutes <= 5 {
		parts = append(parts, "low detour")
	} else {
		parts = append(parts, fmt.Sprintf("%d minute detour", result.DetourMinutes))
	}
	if result.Rating >= 4.3 {
		parts = append(parts, "highly rated")
	}
	if result.OpenNow {
		parts = append(parts, "currently open")
	}
	if len(parts) == 0 {
		return "Balanced route convenience and restaurant quality"
	}
	return sentenceCase(strings.Join(parts, ", "))
}

func bestPickReason(result AgentRestaurant) string {
	return fmt.Sprintf("Highest RouteBite Score (%d/100): %s.", result.RouteBiteScore, strings.ToLower(agentReason(result)))
}

func agentSummary(restaurants []AgentRestaurant, preference string) string {
	if len(restaurants) == 0 {
		if preference == "" {
			return "I could not find a good food stop within your route constraints."
		}
		return fmt.Sprintf("I could not find a good %s stop within your route constraints.", preference)
	}
	top := restaurants[0]
	openText := "availability unknown"
	if top.OpenNow {
		openText = "currently open"
	}
	return fmt.Sprintf("Best option is %s, about %d minutes off your route, rated %.1f stars and %s.",
		top.Name, top.DetourMinutes, top.Rating, openText)
}

func sentenceCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
