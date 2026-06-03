package voice

import (
	"strings"
)

// Intent captures what the driver asked for in natural language.
type Intent struct {
	Cuisine     string // best-guess search term for Yelp ("soup", "coffee", "burger")
	OpenNowOnly bool
	MaxDetour   int // minutes, if user said something like "no more than 5 min detour"
}

// Parse extracts intent from raw text like "find me soup along the way".
//
// V1 is keyword-based by design — fast, free, deterministic. Pluggable for
// an LLM-backed parser later: same Intent contract.
func Parse(text string) Intent {
	lc := strings.ToLower(text)
	out := Intent{OpenNowOnly: true}

	// Cuisine keywords — ordered most-specific first.
	keywords := []string{
		"soup", "pho", "ramen",
		"coffee", "espresso", "latte",
		"sushi", "ramen", "thai", "vietnamese",
		"pizza", "burger", "burgers", "wings",
		"taco", "tacos", "mexican",
		"indian", "chinese", "japanese", "korean",
		"sandwich", "salad", "bbq", "barbecue",
		"breakfast", "brunch", "lunch", "dinner",
		"vegan", "vegetarian", "gluten free",
		"ice cream", "dessert", "smoothie", "juice",
	}
	for _, kw := range keywords {
		if strings.Contains(lc, kw) {
			out.Cuisine = kw
			break
		}
	}
	if out.Cuisine == "" {
		// Fall back to the first noun-like token if user said something like
		// "find me a bakery".
		out.Cuisine = fallbackTerm(lc)
	}

	// "open now" / "currently open"
	if strings.Contains(lc, "any time") || strings.Contains(lc, "doesn't matter") {
		out.OpenNowOnly = false
	}

	return out
}

// fallbackTerm pulls the last noun-ish word out of common patterns:
//   "find me X along the way"
//   "I want X"
//   "looking for X near me"
func fallbackTerm(lc string) string {
	patterns := []string{
		"find me ", "find ", "i want ", "looking for ", "get me ", "where's ",
	}
	for _, p := range patterns {
		if i := strings.Index(lc, p); i >= 0 {
			rest := lc[i+len(p):]
			// take everything until "along", "near", or end
			for _, stop := range []string{" along", " near", " nearby", " on the", " somewhere"} {
				if j := strings.Index(rest, stop); j > 0 {
					rest = rest[:j]
				}
			}
			rest = strings.TrimSpace(rest)
			// drop trailing punctuation
			rest = strings.TrimRight(rest, ".?!,")
			// take last word for the cuisine
			parts := strings.Fields(rest)
			if len(parts) > 0 {
				return parts[len(parts)-1]
			}
		}
	}
	return ""
}
