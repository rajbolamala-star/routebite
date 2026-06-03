package voice

import "testing"

func TestParse_DirectCuisineKeyword(t *testing.T) {
	cases := map[string]string{
		"find me soup along the way":     "soup",
		"I want pho near the route":      "pho",
		"looking for tacos":              "taco",
		"any pizza place open now":       "pizza",
		"get me coffee":                  "coffee",
	}
	for in, want := range cases {
		got := Parse(in)
		if got.Cuisine != want {
			t.Errorf("Parse(%q).Cuisine = %q, want %q", in, got.Cuisine, want)
		}
	}
}

func TestParse_OpenNowDefaultTrue(t *testing.T) {
	if !Parse("find me soup").OpenNowOnly {
		t.Error("expected OpenNowOnly true by default")
	}
}

func TestParse_AnyTimeOverrides(t *testing.T) {
	if Parse("find me soup, any time").OpenNowOnly {
		t.Error("expected OpenNowOnly false when 'any time' is mentioned")
	}
}

func TestParse_FallbackPattern(t *testing.T) {
	got := Parse("find me a bakery along the way")
	if got.Cuisine != "bakery" {
		t.Errorf("expected fallback to extract 'bakery', got %q", got.Cuisine)
	}
}
