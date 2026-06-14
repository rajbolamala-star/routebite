"use client";

import {
  ArrowRight,
  Car,
  CheckCircle2,
  Clock3,
  Compass,
  ExternalLink,
  MapPin,
  Mic,
  Navigation,
  Phone,
  Play,
  RefreshCcw,
  Search,
  Star,
  Utensils,
  Volume2,
} from "lucide-react";
import { FormEvent, useEffect, useState } from "react";

type Point = {
  lat: number;
  lng: number;
};

type RoutePreset = {
  originLabel: string;
  destinationLabel: string;
  origin: Point;
  destination: Point;
};

type Restaurant = {
  name: string;
  image_url: string;
  rating: number;
  review_count: number;
  phone: string;
  call_link: string;
  yelp_url: string;
  address: string;
  location: Point;
  distance_from_route_m: number;
  extra_minutes: number;
  cuisine: string[];
  price: string;
  is_open_now: boolean;
  score: number;
};

type SearchResponse = {
  route_summary: {
    base_duration_min: number;
    distance_km: number;
  };
  results: Restaurant[];
  voice_summary: string;
};

type GeocodeSuggestion = {
  label: string;
  point: Point;
};

type GeocodeResponse = {
  results: GeocodeSuggestion[];
};

type Providers = {
  restaurants: "mock" | "yelp" | string;
  routing: "mock" | "osrm" | string;
  geocoding: "mock" | "nominatim" | string;
};

type SpeechRecognitionConstructor = new () => SpeechRecognition;

type SpeechRecognition = {
  lang: string;
  interimResults: boolean;
  maxAlternatives: number;
  start: () => void;
  onresult: ((event: SpeechRecognitionEvent) => void) | null;
  onerror: (() => void) | null;
};

type SpeechRecognitionEvent = {
  results: ArrayLike<ArrayLike<{ transcript: string }>>;
};

const routePresets: Record<string, RoutePreset> = {
  "miami-sunrise": {
    originLabel: "North Miami Beach",
    destinationLabel: "Sunrise",
    origin: { lat: 25.929, lng: -80.169 },
    destination: { lat: 26.133, lng: -80.29 },
  },
  "la-santa-monica": {
    originLabel: "Downtown Los Angeles",
    destinationLabel: "Santa Monica",
    origin: { lat: 34.0522, lng: -118.2437 },
    destination: { lat: 34.0195, lng: -118.4912 },
  },
  "brooklyn-newark": {
    originLabel: "Brooklyn",
    destinationLabel: "Newark Airport",
    origin: { lat: 40.6782, lng: -73.9442 },
    destination: { lat: 40.6895, lng: -74.1745 },
  },
};

const routeLabels = [
  ["miami-sunrise", "Miami to Sunrise"],
  ["la-santa-monica", "LA to Santa Monica"],
  ["brooklyn-newark", "Brooklyn to Newark"],
] as const;

function formatMinutes(value?: number) {
  if (!Number.isFinite(value)) return "--";
  return `${Math.round(value ?? 0)} min`;
}

function formatKm(value?: number) {
  if (!Number.isFinite(value)) return "--";
  return `${(value ?? 0).toFixed(1)} km`;
}

function cuisineText(item: Restaurant) {
  if (!item.cuisine?.length) return "Restaurant";
  return item.cuisine.slice(0, 3).join(" · ");
}

function mapsUrl(place: Restaurant) {
  const query = `${place.name} ${place.address || ""} ${place.location.lat},${place.location.lng}`.trim();
  return `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(query)}`;
}

function reviewText(count: number) {
  if (!Number.isFinite(count) || count <= 0) return "No reviews yet";
  if (count === 1) return "1 review";
  return `${count.toLocaleString()} reviews`;
}

function resultImageStyle(item: Restaurant) {
  if (!item.image_url) return undefined;
  return { backgroundImage: `url("${item.image_url}")` };
}

function getSpeechRecognition(): SpeechRecognitionConstructor | null {
  if (typeof window === "undefined") return null;
  const speechWindow = window as Window &
    typeof globalThis & {
      SpeechRecognition?: SpeechRecognitionConstructor;
      webkitSpeechRecognition?: SpeechRecognitionConstructor;
    };
  return speechWindow.SpeechRecognition || speechWindow.webkitSpeechRecognition || null;
}

async function fetchGeocodeSuggestions(query: string, limit = 4): Promise<GeocodeSuggestion[]> {
  const res = await fetch(`/v1/geocode?q=${encodeURIComponent(query)}&limit=${limit}`);
  const data = (await res.json()) as GeocodeResponse & { error?: string };
  if (!res.ok) {
    throw new Error(data.error || "Could not load route suggestions.");
  }
  return data.results || [];
}

export default function Home() {
  const [activeRouteKey, setActiveRouteKey] = useState("miami-sunrise");
  const [originLabel, setOriginLabel] = useState(routePresets["miami-sunrise"].originLabel);
  const [destinationLabel, setDestinationLabel] = useState(routePresets["miami-sunrise"].destinationLabel);
  const [originPoint, setOriginPoint] = useState(routePresets["miami-sunrise"].origin);
  const [destinationPoint, setDestinationPoint] = useState(routePresets["miami-sunrise"].destination);
  const [originResolvedLabel, setOriginResolvedLabel] = useState(routePresets["miami-sunrise"].originLabel);
  const [destinationResolvedLabel, setDestinationResolvedLabel] = useState(routePresets["miami-sunrise"].destinationLabel);
  const [voiceText, setVoiceText] = useState("find me soup along the way");
  const [maxDetour, setMaxDetour] = useState(10);
  const [maxResults, setMaxResults] = useState(5);
  const [openNow, setOpenNow] = useState(true);
  const [status, setStatus] = useState("API ready");
  const [response, setResponse] = useState<SearchResponse | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [isResolvingRoute, setIsResolvingRoute] = useState(false);
  const [originSuggestions, setOriginSuggestions] = useState<GeocodeSuggestion[]>([]);
  const [destinationSuggestions, setDestinationSuggestions] = useState<GeocodeSuggestion[]>([]);
  const [activeSuggestionField, setActiveSuggestionField] = useState<"origin" | "destination" | null>(null);
  const [providers, setProviders] = useState<Providers | null>(null);
  const [searchError, setSearchError] = useState("");

  const summary = response?.voice_summary || "Ready for your first search";
  const restaurantProviderLabel = providers?.restaurants === "yelp" ? "Live Yelp" : "Mock restaurants";

  useEffect(() => {
    async function loadProviders() {
      try {
        const res = await fetch("/v1/providers");
        const data = (await res.json()) as Providers & { error?: string };
        if (!res.ok) {
          throw new Error(data.error || "Provider status unavailable.");
        }
        setProviders(data);
      } catch (error) {
        const message = error instanceof Error ? error.message : "Provider status unavailable.";
        setSearchError(message);
      }
    }
    void loadProviders();
  }, []);

  useEffect(() => {
    const timeout = window.setTimeout(() => {
      const query = originLabel.trim();
      if (query.length < 2 || activeSuggestionField !== "origin") {
        setOriginSuggestions([]);
        return;
      }
      void fetchGeocodeSuggestions(query).then(setOriginSuggestions).catch(() => setOriginSuggestions([]));
    }, 260);
    return () => window.clearTimeout(timeout);
  }, [originLabel, activeSuggestionField]);

  useEffect(() => {
    const timeout = window.setTimeout(() => {
      const query = destinationLabel.trim();
      if (query.length < 2 || activeSuggestionField !== "destination") {
        setDestinationSuggestions([]);
        return;
      }
      void fetchGeocodeSuggestions(query).then(setDestinationSuggestions).catch(() => setDestinationSuggestions([]));
    }, 260);
    return () => window.clearTimeout(timeout);
  }, [destinationLabel, activeSuggestionField]);

  function selectRoute(key: string) {
    const nextRoute = routePresets[key];
    setActiveRouteKey(key);
    setOriginLabel(nextRoute.originLabel);
    setDestinationLabel(nextRoute.destinationLabel);
    setOriginPoint(nextRoute.origin);
    setDestinationPoint(nextRoute.destination);
    setOriginResolvedLabel(nextRoute.originLabel);
    setDestinationResolvedLabel(nextRoute.destinationLabel);
  }

  function swapRoute() {
    setActiveRouteKey("custom");
    setOriginLabel(destinationLabel);
    setDestinationLabel(originLabel);
    setOriginPoint(destinationPoint);
    setDestinationPoint(originPoint);
    setOriginResolvedLabel(destinationResolvedLabel);
    setDestinationResolvedLabel(originResolvedLabel);
  }

  function chooseSuggestion(field: "origin" | "destination", suggestion: GeocodeSuggestion) {
    setActiveRouteKey("custom");
    if (field === "origin") {
      setOriginLabel(suggestion.label);
      setOriginPoint(suggestion.point);
      setOriginResolvedLabel(suggestion.label);
      setOriginSuggestions([]);
    } else {
      setDestinationLabel(suggestion.label);
      setDestinationPoint(suggestion.point);
      setDestinationResolvedLabel(suggestion.label);
      setDestinationSuggestions([]);
    }
    setActiveSuggestionField(null);
  }

  async function geocodePlace(label: string, fallback: Point): Promise<{ label: string; point: Point }> {
    const query = label.trim();
    if (!query) {
      throw new Error("Enter both origin and destination.");
    }

    const res = await fetch(`/v1/geocode?q=${encodeURIComponent(query)}&limit=1`);
    const data = (await res.json()) as GeocodeResponse & { error?: string };
    if (!res.ok) {
      throw new Error(data.error || `Could not resolve ${query}.`);
    }

    const [top] = data.results;
    if (!top) {
      throw new Error(`No location found for "${query}".`);
    }

    return {
      label: top.label || query,
      point: top.point || fallback,
    };
  }

  async function search(event?: FormEvent) {
    event?.preventDefault();
    setIsLoading(true);
    setIsResolvingRoute(true);
    setStatus("Resolving route");
    setSearchError("");

    try {
      const [origin, destination] = await Promise.all([
        geocodePlace(originLabel, originPoint),
        geocodePlace(destinationLabel, destinationPoint),
      ]);

      setOriginPoint(origin.point);
      setDestinationPoint(destination.point);
      setOriginResolvedLabel(origin.label);
      setDestinationResolvedLabel(destination.label);
      setStatus("Searching");

      const res = await fetch("/v1/search", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          origin: origin.point,
          destination: destination.point,
          voice_text: voiceText.trim(),
          max_detour_minutes: maxDetour,
          max_results: maxResults,
          open_now_only: openNow,
        }),
      });
      const data = await res.json();
      if (!res.ok) {
        throw new Error(data.error || "Search failed");
      }
      setResponse(data);
      setStatus("Results ready");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Search failed";
      const friendlyMessage = message.includes("Yelp") || message.includes("restaurant search failed")
        ? "Restaurant search is unavailable right now. Check the Yelp API key or try mock mode."
        : message;
      setResponse(null);
      setStatus("Search needs attention");
      setSearchError(friendlyMessage);
      console.error(message);
    } finally {
      setIsLoading(false);
      setIsResolvingRoute(false);
    }
  }

  function speak() {
    if (!summary || typeof window === "undefined" || !("speechSynthesis" in window)) return;
    window.speechSynthesis.cancel();
    const utterance = new SpeechSynthesisUtterance(summary);
    utterance.rate = 0.96;
    window.speechSynthesis.speak(utterance);
  }

  function listen() {
    const Recognition = getSpeechRecognition();
    if (!Recognition) {
      setStatus("Voice unavailable");
      return;
    }
    const recognition = new Recognition();
    recognition.lang = "en-US";
    recognition.interimResults = false;
    recognition.maxAlternatives = 1;
    setStatus("Listening");
    recognition.start();
    recognition.onresult = (event) => {
      setVoiceText(event.results[0][0].transcript);
      setStatus("Heard request");
    };
    recognition.onerror = () => setStatus("Voice stopped");
  }

  return (
    <main className="app-shell">
      <section className="route-workspace" aria-label="Route search">
        <header className="topbar">
          <div>
            <p className="eyebrow">RouteBite</p>
            <h1>Food on your way, without losing the route.</h1>
          </div>
          <span className="status-pill">
            <CheckCircle2 size={16} />
            {status}
          </span>
        </header>

        <div className="route-visual" aria-hidden="true">
          <div className="map-grid" />
          <div className="route-line" />
          <span className="map-pin start">
            <Car size={15} />
          </span>
          <span className="map-pin food">
            <Utensils size={17} />
          </span>
          <span className="map-pin end">
            <MapPin size={16} />
          </span>
        </div>

        <form className="search-panel" onSubmit={search}>
          <div className="provider-row" aria-live="polite">
            <span className={providers?.restaurants === "yelp" ? "provider-badge live" : "provider-badge"}>
              <Utensils size={14} />
              {restaurantProviderLabel}
            </span>
            <span>{providers ? `Routing: ${providers.routing} · Geocoding: ${providers.geocoding}` : "Checking providers"}</span>
          </div>

          {searchError ? (
            <div className="inline-alert" role="alert">
              {searchError}
            </div>
          ) : null}

          <div className="field-row">
            <label>
              <span>Origin</span>
              <div className="place-field">
                <input
                  value={originLabel}
                  onFocus={() => setActiveSuggestionField("origin")}
                  onBlur={() => window.setTimeout(() => setActiveSuggestionField(null), 140)}
                  onChange={(event) => {
                    setActiveRouteKey("custom");
                    setActiveSuggestionField("origin");
                    setOriginLabel(event.target.value);
                    setOriginResolvedLabel("");
                  }}
                  placeholder="Start address or city"
                />
                {activeSuggestionField === "origin" && originSuggestions.length > 0 ? (
                  <div className="suggestion-list">
                    {originSuggestions.map((suggestion) => (
                      <button
                        key={`${suggestion.label}-${suggestion.point.lat}-${suggestion.point.lng}`}
                        type="button"
                        onMouseDown={(event) => event.preventDefault()}
                        onClick={() => chooseSuggestion("origin", suggestion)}
                      >
                        <MapPin size={14} />
                        <span>{suggestion.label}</span>
                      </button>
                    ))}
                  </div>
                ) : null}
              </div>
              <small className="field-hint">
                {originResolvedLabel ? `Using ${originResolvedLabel}` : "Will resolve when you search"}
              </small>
            </label>
            <button className="icon-button" type="button" onClick={swapRoute} aria-label="Swap route" title="Swap route">
              <RefreshCcw size={18} />
            </button>
            <label>
              <span>Destination</span>
              <div className="place-field">
                <input
                  value={destinationLabel}
                  onFocus={() => setActiveSuggestionField("destination")}
                  onBlur={() => window.setTimeout(() => setActiveSuggestionField(null), 140)}
                  onChange={(event) => {
                    setActiveRouteKey("custom");
                    setActiveSuggestionField("destination");
                    setDestinationLabel(event.target.value);
                    setDestinationResolvedLabel("");
                  }}
                  placeholder="Destination address or city"
                />
                {activeSuggestionField === "destination" && destinationSuggestions.length > 0 ? (
                  <div className="suggestion-list">
                    {destinationSuggestions.map((suggestion) => (
                      <button
                        key={`${suggestion.label}-${suggestion.point.lat}-${suggestion.point.lng}`}
                        type="button"
                        onMouseDown={(event) => event.preventDefault()}
                        onClick={() => chooseSuggestion("destination", suggestion)}
                      >
                        <MapPin size={14} />
                        <span>{suggestion.label}</span>
                      </button>
                    ))}
                  </div>
                ) : null}
              </div>
              <small className="field-hint">
                {destinationResolvedLabel ? `Using ${destinationResolvedLabel}` : "Will resolve when you search"}
              </small>
            </label>
          </div>

          <div className="preset-row" aria-label="Route presets">
            {routeLabels.map(([key, label]) => (
              <button
                className={key === activeRouteKey ? "chip active" : "chip"}
                key={key}
                type="button"
                onClick={() => selectRoute(key)}
              >
                {label}
              </button>
            ))}
          </div>

          <label className="voice-field">
            <span>What sounds good?</span>
            <div className="voice-input">
              <Search className="input-icon" size={18} />
              <input value={voiceText} onChange={(event) => setVoiceText(event.target.value)} />
              <button className="icon-button" type="button" onClick={listen} aria-label="Speak food request" title="Speak food request">
                <Mic size={18} />
              </button>
            </div>
          </label>

          <div className="control-grid">
            <label>
              <span>Max detour</span>
              <select value={maxDetour} onChange={(event) => setMaxDetour(Number(event.target.value))}>
                <option value={5}>5 minutes</option>
                <option value={10}>10 minutes</option>
                <option value={15}>15 minutes</option>
              </select>
            </label>
            <label>
              <span>Results</span>
              <select value={maxResults} onChange={(event) => setMaxResults(Number(event.target.value))}>
                <option value={3}>Top 3</option>
                <option value={5}>Top 5</option>
                <option value={8}>Top 8</option>
              </select>
            </label>
            <label className="toggle-row">
              <input type="checkbox" checked={openNow} onChange={(event) => setOpenNow(event.target.checked)} />
              <span>Open now</span>
            </label>
          </div>

          <button className="primary-button" type="submit" disabled={isLoading}>
            <span>{isResolvingRoute ? "Resolving route" : isLoading ? "Finding food" : "Find food"}</span>
            <ArrowRight size={19} />
          </button>
        </form>
      </section>

      <section className="results-panel" aria-live="polite">
        <div className="summary-strip">
          <div>
            <p className="eyebrow">Drive summary</p>
            <h2>{summary}</h2>
          </div>
          <button className="icon-button" type="button" onClick={speak} aria-label="Read summary aloud" title="Read summary aloud">
            <Volume2 size={18} />
          </button>
        </div>

        <div className="metrics">
          <div>
            <Clock3 size={17} />
            <span>{formatMinutes(response?.route_summary.base_duration_min)}</span>
            <small>base drive</small>
          </div>
          <div>
            <Compass size={17} />
            <span>{formatKm(response?.route_summary.distance_km)}</span>
            <small>distance</small>
          </div>
          <div>
            <Utensils size={17} />
            <span>{response?.results.length ?? "--"}</span>
            <small>spots</small>
          </div>
        </div>

        {!response?.results.length ? (
          <div className="empty-state">
            <Play size={25} />
            {response ? (
              <>
                <strong>No matching stops on this route.</strong>
                <p>Try a broader food request, turn off open-now, or allow a longer detour.</p>
              </>
            ) : (
              <>
                <strong>Ask for what you want.</strong>
                <p>Try “pizza for the family,” “coffee near the route,” or “healthy lunch under 5 minutes.”</p>
              </>
            )}
          </div>
        ) : (
          <div className="results-list">
            {response.results.map((item, index) => (
              <article className="result-card" key={`${item.name}-${item.address}`}>
                <div
                  className={item.image_url ? "result-photo" : "result-photo empty"}
                  style={resultImageStyle(item)}
                  aria-hidden="true"
                >
                  <div className="result-rank">{index + 1}</div>
                </div>
                <div className="result-body">
                  <div className="result-head">
                    <div>
                      <h3>{item.name}</h3>
                      <p className="cuisine">{cuisineText(item)}</p>
                    </div>
                    <div className="score">{Math.round(item.score * 100)}%</div>
                  </div>
                  <div className="fact-row">
                    <span>
                      <Star size={14} />
                      {item.rating.toFixed(1)} · {reviewText(item.review_count)}
                    </span>
                    <span>
                      <Clock3 size={14} />
                      {item.extra_minutes} min detour
                    </span>
                    <span>{item.price || "Price n/a"}</span>
                  </div>
                  <p className="address">{item.address || "Address unavailable"}</p>
                  <div className="actions">
                    <a
                      className={item.call_link ? "call-action" : "call-action disabled"}
                      href={item.call_link || undefined}
                      aria-disabled={!item.call_link}
                    >
                      <Phone size={16} />
                      Call
                    </a>
                    <a className="nav-action" href={mapsUrl(item)} target="_blank" rel="noreferrer">
                      <Navigation size={16} />
                      Navigate
                    </a>
                    {item.yelp_url ? (
                      <a className="yelp-action" href={item.yelp_url} target="_blank" rel="noreferrer">
                        <ExternalLink size={16} />
                        Yelp
                      </a>
                    ) : null}
                  </div>
                </div>
              </article>
            ))}
          </div>
        )}
      </section>
    </main>
  );
}
