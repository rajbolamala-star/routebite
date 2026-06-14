package scoring

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/dheerajb/routebite/internal/routing"
	"github.com/dheerajb/routebite/internal/yelp"
)

// Weights controls the scoring formula. Sensible defaults are baked in
// but exposed for tuning / A-B testing.
type Weights struct {
	Rating      float64 // 0..1
	ReviewCount float64
	Convenience float64
	OpenBonus   float64
}

var Default = Weights{
	Rating:      0.4,
	ReviewCount: 0.2,
	Convenience: 0.3,
	OpenBonus:   0.1,
}

// LatLng is a geographic coordinate in API-ready JSON form.
type LatLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// Restaurant is one ranked result returned by the scoring engine.
type Restaurant struct {
	Name               string   `json:"name"`
	Rating             float64  `json:"rating"`
	ReviewCount        int      `json:"review_count"`
	Phone              string   `json:"phone"`
	CallLink           string   `json:"call_link"` // tel:+1...
	YelpURL            string   `json:"yelp_url"`
	Address            string   `json:"address"`
	Location           LatLng   `json:"location"`
	DistanceFromRouteM int      `json:"distance_from_route_m"`
	ExtraMinutes       int      `json:"extra_minutes"`
	Cuisine            []string `json:"cuisine"`
	Price              string   `json:"price"` // "$", "$$", "$$$"
	IsOpenNow          bool     `json:"is_open_now"`
	Score              float64  `json:"score"` // 0.0 - 1.0
}

// Rank converts Yelp businesses into ranked API restaurants, applying detour
// math against the route polyline.
//
//   - businesses with detour > maxDetourMin are filtered out
//   - results are sorted high-to-low by score
//   - results are truncated to maxResults
func Rank(
	bs []yelp.Business,
	poly []routing.Point,
	maxDetourMin int,
	maxResults int,
	w Weights,
) []Restaurant {
	return RankWithDetours(bs, poly, nil, maxDetourMin, maxResults, w)
}

// DetourKey returns the stable key used by detour override maps.
func DetourKey(b yelp.Business) string {
	if b.ID != "" {
		return b.ID
	}
	return b.Name
}

// RankWithDetours ranks businesses using precise detour minute overrides when
// available, falling back to the distance-based estimate used by Rank.
func RankWithDetours(
	bs []yelp.Business,
	poly []routing.Point,
	detourMinutes map[string]int,
	maxDetourMin int,
	maxResults int,
	w Weights,
) []Restaurant {
	if maxResults <= 0 {
		maxResults = 5
	}
	if maxDetourMin <= 0 {
		maxDetourMin = 10
	}

	out := make([]Restaurant, 0, len(bs))
	for _, b := range bs {
		if b.IsClosed {
			continue
		}
		p := routing.Point{Lat: b.Coordinates.Latitude, Lng: b.Coordinates.Longitude}
		distM := routing.MinDistanceToPolyline(p, poly)

		extraMin := int(math.Round((distM / 1000.0) * 2.0))
		if extraMin == 0 && distM > 200 {
			extraMin = 1
		}
		if detourMinutes != nil {
			if detour, ok := detourMinutes[DetourKey(b)]; ok {
				extraMin = detour
			}
		}
		if extraMin > maxDetourMin {
			continue
		}

		score := compute(b, extraMin, w)
		out = append(out, toRestaurant(b, p, int(distM), extraMin, score))
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > maxResults {
		out = out[:maxResults]
	}
	return out
}

func compute(b yelp.Business, extraMin int, w Weights) float64 {
	// Normalize rating from [0,5] to [0,1].
	ratingN := b.Rating / 5.0
	// Soft cap reviews at 1000 so a 5000-review giant doesn't crush a great
	// 200-review neighborhood spot.
	reviewN := float64(b.ReviewCount) / 1000.0
	if reviewN > 1.0 {
		reviewN = 1.0
	}
	// Convenience falls off as detour grows. 0 min => 1.0, 10 min => ~0.09.
	convenience := 1.0 / (1.0 + float64(extraMin))
	openBonus := 0.0
	if !b.IsClosed {
		openBonus = 1.0
	}

	return ratingN*w.Rating +
		reviewN*w.ReviewCount +
		convenience*w.Convenience +
		openBonus*w.OpenBonus
}

func toRestaurant(b yelp.Business, p routing.Point, distM int, extraMin int, score float64) Restaurant {
	cuisines := make([]string, 0, len(b.Categories))
	for _, c := range b.Categories {
		cuisines = append(cuisines, c.Title)
	}
	addr := strings.TrimSpace(strings.Join([]string{
		b.Location.Address1, b.Location.City, b.Location.State, b.Location.ZipCode,
	}, ", "))
	addr = strings.ReplaceAll(addr, ", ,", ",")

	phone := b.Phone
	if phone == "" {
		phone = b.DisplayPhone
	}
	callLink := ""
	if phone != "" {
		callLink = "tel:" + phone
	}

	return Restaurant{
		Name:               b.Name,
		Rating:             b.Rating,
		ReviewCount:        b.ReviewCount,
		Phone:              phone,
		CallLink:           callLink,
		YelpURL:            b.URL,
		Address:            addr,
		Location:           LatLng{Lat: p.Lat, Lng: p.Lng},
		DistanceFromRouteM: distM,
		ExtraMinutes:       extraMin,
		Cuisine:            cuisines,
		Price:              b.Price,
		IsOpenNow:          !b.IsClosed,
		Score:              math.Round(score*100) / 100, // 2dp
	}
}

// VoiceSummary builds the one-line spoken response. Empty results get a
// helpful fallback rather than an awkward "0 results found."
func VoiceSummary(results []Restaurant, query string) string {
	if len(results) == 0 {
		if query != "" {
			return fmt.Sprintf("No %s spots within your detour limit. Want me to expand the search?", query)
		}
		return "No matching spots found along the route. Want me to broaden the search?"
	}
	top := results[0]
	cuisine := ""
	if len(top.Cuisine) > 0 {
		cuisine = " " + strings.ToLower(top.Cuisine[0])
	}
	return fmt.Sprintf(
		"Found %d%s spots. Top pick is %s — %.1f stars, %d extra minutes. Want to call?",
		len(results), cuisine, top.Name, top.Rating, top.ExtraMinutes,
	)
}
