package api

import (
	"context"
	"fmt"
	"regexp"
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

func (h *Handler) runAgentSearch(ctx context.Context, req AgentSearchRequest) (AgentSearchResponse, *apiSearchError) {
	plan := h.agent.Parse(req)
	if plan.Start == "" {
		return AgentSearchResponse{}, badRequest("agent needs a start location")
	}
	if plan.Destination == "" {
		return AgentSearchResponse{}, badRequest("agent needs a destination")
	}
	if plan.Preference == "" {
		return AgentSearchResponse{}, badRequest("agent needs a food preference")
	}

	origin, err := h.geocodeOne(ctx, plan.Start)
	if err != nil {
		return AgentSearchResponse{}, badGateway("start geocoding failed", err)
	}
	destination, err := h.geocodeOne(ctx, plan.Destination)
	if err != nil {
		return AgentSearchResponse{}, badGateway("destination geocoding failed", err)
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
		return AgentSearchResponse{}, searchErr
	}

	restaurants := make([]AgentRestaurant, 0, len(searchResp.Results))
	for _, result := range searchResp.Results {
		restaurants = append(restaurants, toAgentRestaurant(result))
	}

	return AgentSearchResponse{
		Summary:     agentSummary(restaurants, plan.Preference),
		Restaurants: restaurants,
	}, nil
}

func (h *Handler) geocodeOne(ctx context.Context, place string) (geocode.Suggestion, error) {
	results, err := h.geocode.Search(ctx, place, 1)
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

func toAgentRestaurant(result scoring.Restaurant) AgentRestaurant {
	return AgentRestaurant{
		Name:          result.Name,
		Rating:        result.Rating,
		DetourMinutes: result.ExtraMinutes,
		OpenNow:       result.IsOpenNow,
		Address:       result.Address,
		Phone:         result.Phone,
		Reason:        agentReason(result),
	}
}

func agentReason(result scoring.Restaurant) string {
	parts := []string{}
	if result.ExtraMinutes <= 5 {
		parts = append(parts, "low detour")
	} else {
		parts = append(parts, fmt.Sprintf("%d minute detour", result.ExtraMinutes))
	}
	if result.Rating >= 4.3 {
		parts = append(parts, "highly rated")
	}
	if result.IsOpenNow {
		parts = append(parts, "currently open")
	}
	if len(parts) == 0 {
		return "Balanced route convenience and restaurant quality"
	}
	return sentenceCase(strings.Join(parts, ", "))
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
