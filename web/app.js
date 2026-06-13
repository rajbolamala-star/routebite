const routes = {
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

let activeRouteKey = "miami-sunrise";
let latestSummary = "";

const els = {
  form: document.querySelector("#searchForm"),
  originInput: document.querySelector("#originInput"),
  destinationInput: document.querySelector("#destinationInput"),
  voiceText: document.querySelector("#voiceText"),
  maxDetour: document.querySelector("#maxDetour"),
  maxResults: document.querySelector("#maxResults"),
  openNow: document.querySelector("#openNow"),
  apiStatus: document.querySelector("#apiStatus"),
  summaryTitle: document.querySelector("#summaryTitle"),
  baseTime: document.querySelector("#baseTime"),
  distance: document.querySelector("#distance"),
  resultCount: document.querySelector("#resultCount"),
  emptyState: document.querySelector("#emptyState"),
  resultsList: document.querySelector("#resultsList"),
  template: document.querySelector("#resultTemplate"),
  listenButton: document.querySelector("#listenButton"),
  speakButton: document.querySelector("#speakButton"),
  swapRoute: document.querySelector("#swapRoute"),
  chips: document.querySelectorAll("[data-route]"),
};

function setStatus(text) {
  els.apiStatus.textContent = text;
}

function setRoute(key) {
  activeRouteKey = key;
  const route = routes[key];
  els.originInput.value = route.originLabel;
  els.destinationInput.value = route.destinationLabel;
  els.chips.forEach((chip) => chip.classList.toggle("active", chip.dataset.route === key));
}

function currentRoute() {
  return routes[activeRouteKey] || routes["miami-sunrise"];
}

function payload() {
  const route = currentRoute();
  return {
    origin: route.origin,
    destination: route.destination,
    voice_text: els.voiceText.value.trim(),
    max_detour_minutes: Number(els.maxDetour.value),
    max_results: Number(els.maxResults.value),
    open_now_only: els.openNow.checked,
  };
}

function mapsUrl(place) {
  const { lat, lng } = place.location;
  return `https://www.google.com/maps/search/?api=1&query=${lat},${lng}`;
}

function formatMinutes(value) {
  if (!Number.isFinite(value)) return "--";
  return `${Math.round(value)} min`;
}

function formatKm(value) {
  if (!Number.isFinite(value)) return "--";
  return `${value.toFixed(1)} km`;
}

function cuisineText(item) {
  if (!item.cuisine || item.cuisine.length === 0) return "Restaurant";
  return item.cuisine.slice(0, 3).join(" · ");
}

function renderResults(data) {
  latestSummary = data.voice_summary || "";
  els.summaryTitle.textContent = latestSummary || "No summary returned";
  els.baseTime.textContent = formatMinutes(data.route_summary?.base_duration_min);
  els.distance.textContent = formatKm(data.route_summary?.distance_km);
  els.resultCount.textContent = String(data.results?.length ?? 0);
  els.resultsList.innerHTML = "";

  const results = data.results || [];
  els.emptyState.hidden = results.length > 0;

  results.forEach((item, index) => {
    const node = els.template.content.firstElementChild.cloneNode(true);
    node.querySelector(".result-rank").textContent = index + 1;
    node.querySelector("h3").textContent = item.name;
    node.querySelector(".cuisine").textContent = cuisineText(item);
    node.querySelector(".score").textContent = `${Math.round((item.score || 0) * 100)}%`;
    node.querySelector(".rating").textContent = `${item.rating.toFixed(1)} stars`;
    node.querySelector(".detour").textContent = `${item.extra_minutes} min detour`;
    node.querySelector(".price").textContent = item.price || "Price n/a";
    node.querySelector(".address").textContent = item.address || "Address unavailable";

    const call = node.querySelector(".call-action");
    call.href = item.call_link || "#";
    call.setAttribute("aria-disabled", item.call_link ? "false" : "true");
    if (!item.call_link) call.textContent = "No phone";

    node.querySelector(".nav-action").href = mapsUrl(item);
    els.resultsList.appendChild(node);
  });
}

async function search(event) {
  event?.preventDefault();
  els.form.classList.add("is-loading");
  setStatus("Searching");

  try {
    const response = await fetch("/v1/search", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload()),
    });

    const data = await response.json();
    if (!response.ok) {
      throw new Error(data.error || "Search failed");
    }

    renderResults(data);
    setStatus("Results ready");
  } catch (error) {
    els.summaryTitle.textContent = error.message;
    els.resultCount.textContent = "0";
    els.emptyState.hidden = false;
    els.resultsList.innerHTML = "";
    setStatus("Needs backend");
  } finally {
    els.form.classList.remove("is-loading");
  }
}

function speak(text) {
  if (!text || !("speechSynthesis" in window)) return;
  window.speechSynthesis.cancel();
  const utterance = new SpeechSynthesisUtterance(text);
  utterance.rate = 0.96;
  utterance.pitch = 1;
  window.speechSynthesis.speak(utterance);
}

function startListening() {
  const Recognition = window.SpeechRecognition || window.webkitSpeechRecognition;
  if (!Recognition) {
    els.voiceText.focus();
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
    els.voiceText.value = event.results[0][0].transcript;
    setStatus("Heard request");
  };
  recognition.onerror = () => setStatus("Voice stopped");
}

els.form.addEventListener("submit", search);
els.speakButton.addEventListener("click", () => speak(latestSummary || els.summaryTitle.textContent));
els.listenButton.addEventListener("click", startListening);
els.swapRoute.addEventListener("click", () => {
  const route = routes[activeRouteKey];
  [route.origin, route.destination] = [route.destination, route.origin];
  [route.originLabel, route.destinationLabel] = [route.destinationLabel, route.originLabel];
  setRoute(activeRouteKey);
});
els.chips.forEach((chip) => chip.addEventListener("click", () => setRoute(chip.dataset.route)));

setRoute(activeRouteKey);
