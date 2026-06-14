package routing

import (
	"context"
	"testing"
)

func TestRouteViaComposesFallbackEngine(t *testing.T) {
	engine := NewMockEngine()
	origin := Point{Lat: 25.0, Lng: -80.0}
	stop := Point{Lat: 25.1, Lng: -80.0}
	dest := Point{Lat: 25.2, Lng: -80.0}

	got, err := RouteVia(context.Background(), engine, origin, stop, dest)
	if err != nil {
		t.Fatalf("RouteVia() error = %v", err)
	}
	if got.DurationSec <= 0 {
		t.Fatalf("RouteVia().DurationSec = %f, want positive", got.DurationSec)
	}
	if len(got.Polyline) == 0 {
		t.Fatalf("RouteVia().Polyline is empty")
	}
}
