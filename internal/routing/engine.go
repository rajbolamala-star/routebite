package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

// Route is the minimal route data we need from a routing provider.
type Route struct {
	DurationSec float64 // total trip seconds
	DistanceM   float64 // total trip meters
	Polyline    []Point // ordered route geometry
}

// Point is one geographic vertex on the polyline.
type Point struct {
	Lat float64
	Lng float64
}

// Engine fetches a route between two points.
type Engine interface {
	Route(ctx context.Context, origin, dest Point) (Route, error)
}

// ViaEngine can fetch a route that includes one stop between origin and dest.
type ViaEngine interface {
	RouteVia(ctx context.Context, origin, stop, dest Point) (Route, error)
}

// --- OSRM (public demo server) ---

// OSRM uses the public demo at router.project-osrm.org. Good for prototyping
// only; production should use a self-hosted OSRM, Mapbox Directions, or
// Google Routes API.
type OSRM struct {
	hc   *http.Client
	base string
}

func NewOSRM() *OSRM {
	return &OSRM{
		hc:   &http.Client{Timeout: 8 * time.Second},
		base: "https://router.project-osrm.org",
	}
}

type osrmResp struct {
	Code   string `json:"code"`
	Routes []struct {
		Duration float64 `json:"duration"`
		Distance float64 `json:"distance"`
		Geometry struct {
			Coordinates [][]float64 `json:"coordinates"` // [lng, lat] pairs
		} `json:"geometry"`
	} `json:"routes"`
	Message string `json:"message"`
}

func (o *OSRM) Route(ctx context.Context, origin, dest Point) (Route, error) {
	url := fmt.Sprintf("%s/route/v1/driving/%f,%f;%f,%f?overview=full&geometries=geojson",
		o.base, origin.Lng, origin.Lat, dest.Lng, dest.Lat)

	return o.fetchRoute(ctx, url)
}

func (o *OSRM) RouteVia(ctx context.Context, origin, stop, dest Point) (Route, error) {
	url := fmt.Sprintf("%s/route/v1/driving/%f,%f;%f,%f;%f,%f?overview=full&geometries=geojson",
		o.base, origin.Lng, origin.Lat, stop.Lng, stop.Lat, dest.Lng, dest.Lat)

	return o.fetchRoute(ctx, url)
}

func (o *OSRM) fetchRoute(ctx context.Context, url string) (Route, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Route{}, err
	}
	resp, err := o.hc.Do(req)
	if err != nil {
		return Route{}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var out osrmResp
	if err := json.Unmarshal(body, &out); err != nil {
		return Route{}, fmt.Errorf("decode: %w", err)
	}
	if out.Code != "Ok" || len(out.Routes) == 0 {
		return Route{}, fmt.Errorf("osrm: %s %s", out.Code, out.Message)
	}

	r := out.Routes[0]
	route := Route{DurationSec: r.Duration, DistanceM: r.Distance}
	route.Polyline = make([]Point, 0, len(r.Geometry.Coordinates))
	for _, c := range r.Geometry.Coordinates {
		if len(c) >= 2 {
			route.Polyline = append(route.Polyline, Point{Lat: c[1], Lng: c[0]})
		}
	}
	return route, nil
}

// RouteVia fetches or composes a route with one stop. Engines that implement
// ViaEngine can do this in a single provider request; others fall back to two
// ordinary route calls.
func RouteVia(ctx context.Context, e Engine, origin, stop, dest Point) (Route, error) {
	if via, ok := e.(ViaEngine); ok {
		return via.RouteVia(ctx, origin, stop, dest)
	}

	first, err := e.Route(ctx, origin, stop)
	if err != nil {
		return Route{}, err
	}
	second, err := e.Route(ctx, stop, dest)
	if err != nil {
		return Route{}, err
	}

	polyline := append([]Point{}, first.Polyline...)
	if len(second.Polyline) > 1 {
		polyline = append(polyline, second.Polyline[1:]...)
	} else {
		polyline = append(polyline, second.Polyline...)
	}
	return Route{
		DurationSec: first.DurationSec + second.DurationSec,
		DistanceM:   first.DistanceM + second.DistanceM,
		Polyline:    polyline,
	}, nil
}

// --- Mock ---

// MockEngine returns a straight-line "route" between two points, with a
// crude time-from-distance estimate. Used when OSRM is unreachable or in tests.
type MockEngine struct{}

func NewMockEngine() *MockEngine { return &MockEngine{} }

func (MockEngine) Route(_ context.Context, origin, dest Point) (Route, error) {
	// Interpolate 25 points between origin and dest for the polyline.
	const steps = 25
	pts := make([]Point, 0, steps+1)
	for i := 0; i <= steps; i++ {
		f := float64(i) / steps
		pts = append(pts, Point{
			Lat: origin.Lat + (dest.Lat-origin.Lat)*f,
			Lng: origin.Lng + (dest.Lng-origin.Lng)*f,
		})
	}
	dist := Haversine(origin, dest)
	// Assume 50 km/h average city driving when we have no real router.
	const avgKmh = 50.0
	durationSec := (dist / 1000.0) / avgKmh * 3600.0
	return Route{DurationSec: durationSec, DistanceM: dist, Polyline: pts}, nil
}

// --- Geo helpers ---

// Haversine returns great-circle distance between two points in meters.
func Haversine(a, b Point) float64 {
	const R = 6371000.0 // earth radius m
	la1 := a.Lat * math.Pi / 180
	la2 := b.Lat * math.Pi / 180
	dLat := (b.Lat - a.Lat) * math.Pi / 180
	dLng := (b.Lng - a.Lng) * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(la1)*math.Cos(la2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * R * math.Asin(math.Sqrt(h))
}

// MinDistanceToPolyline returns the shortest distance in meters from point p
// to any segment of the polyline. Used to decide whether a candidate is "along
// the route" and how big the detour is.
func MinDistanceToPolyline(p Point, poly []Point) float64 {
	if len(poly) == 0 {
		return math.MaxFloat64
	}
	best := math.MaxFloat64
	for i := 0; i < len(poly)-1; i++ {
		d := distanceToSegment(p, poly[i], poly[i+1])
		if d < best {
			best = d
		}
	}
	return best
}

// distanceToSegment uses a local flat-earth projection — accurate enough for
// segments that span a few km.
func distanceToSegment(p, a, b Point) float64 {
	// project to meters using equirectangular at the midpoint latitude
	midLat := (a.Lat + b.Lat) / 2 * math.Pi / 180
	mPerDegLat := 111320.0
	mPerDegLng := 111320.0 * math.Cos(midLat)

	px := (p.Lng - a.Lng) * mPerDegLng
	py := (p.Lat - a.Lat) * mPerDegLat
	bx := (b.Lng - a.Lng) * mPerDegLng
	by := (b.Lat - a.Lat) * mPerDegLat

	segLenSq := bx*bx + by*by
	if segLenSq == 0 {
		return math.Sqrt(px*px + py*py)
	}
	t := (px*bx + py*by) / segLenSq
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	cx := bx * t
	cy := by * t
	return math.Sqrt((px-cx)*(px-cx) + (py-cy)*(py-cy))
}

// MidpointOf returns the geographic midpoint of a polyline by index.
// Useful as the Yelp "center" — we search around the middle of the route.
func MidpointOf(poly []Point) Point {
	if len(poly) == 0 {
		return Point{}
	}
	return poly[len(poly)/2]
}

// BoundingRadiusM returns half the great-circle distance from start to end
// of the polyline, plus a generous buffer — used as the Yelp search radius.
func BoundingRadiusM(poly []Point) int {
	if len(poly) < 2 {
		return 5000
	}
	d := Haversine(poly[0], poly[len(poly)-1])
	r := int(d/2.0) + 3000 // buffer
	if r < 2000 {
		r = 2000
	}
	if r > 40000 {
		r = 40000 // Yelp max
	}
	return r
}
