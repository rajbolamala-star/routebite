package scoring

import (
	"testing"

	"github.com/dheerajb/routebite/internal/routing"
	"github.com/dheerajb/routebite/internal/yelp"
)

func TestRank_FiltersByDetour(t *testing.T) {
	bs := []yelp.Business{
		{Name: "Near", Rating: 4.0, ReviewCount: 100},
		{Name: "Far", Rating: 5.0, ReviewCount: 500},
	}
	bs[0].Coordinates.Latitude, bs[0].Coordinates.Longitude = 25.0, -80.0 // on route
	bs[1].Coordinates.Latitude, bs[1].Coordinates.Longitude = 25.5, -80.0 // far off

	poly := []routing.Point{{Lat: 25.0, Lng: -80.0}, {Lat: 25.0, Lng: -79.5}}

	got := Rank(bs, poly, 5, 5, Default)
	for _, r := range got {
		if r.Name == "Far" {
			t.Errorf("expected Far to be filtered out by detour, but it was returned")
		}
	}
}

func TestRank_SortsByScoreDesc(t *testing.T) {
	bs := []yelp.Business{
		{Name: "Low", Rating: 3.0, ReviewCount: 10},
		{Name: "High", Rating: 4.8, ReviewCount: 800},
	}
	for i := range bs {
		bs[i].Coordinates.Latitude, bs[i].Coordinates.Longitude = 25.0, -80.0
	}
	poly := []routing.Point{{Lat: 25.0, Lng: -80.0}, {Lat: 25.0, Lng: -79.5}}

	got := Rank(bs, poly, 10, 5, Default)
	if len(got) < 2 || got[0].Name != "High" {
		t.Errorf("expected High first, got %+v", got)
	}
}

func TestVoiceSummary_Empty(t *testing.T) {
	s := VoiceSummary(nil, "soup")
	if s == "" {
		t.Error("empty results should still produce a spoken response")
	}
}

func TestVoiceSummary_NonEmpty(t *testing.T) {
	r := []struct{}{}
	_ = r
	results := Rank(
		[]yelp.Business{
			{Name: "Pho 78", Rating: 4.6, ReviewCount: 412,
				Categories: []struct {
					Title string `json:"title"`
					Alias string `json:"alias"`
				}{{Title: "Vietnamese"}},
				Coordinates: struct {
					Latitude  float64 `json:"latitude"`
					Longitude float64 `json:"longitude"`
				}{Latitude: 25.0, Longitude: -80.0}},
		},
		[]routing.Point{{Lat: 25.0, Lng: -80.0}, {Lat: 25.0, Lng: -79.5}},
		10, 5, Default,
	)
	for i := range results {
		results[i].Cuisine = []string{"Vietnamese"}
	}
	s := VoiceSummary(results, "soup")
	if s == "" || s[0] != 'F' {
		t.Errorf("unexpected summary: %q", s)
	}
}
