package environmental

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Telemetry captures microclimate conditions and USFS fire danger ratings for a backcountry campsite.
type Telemetry struct {
	TemperatureMin float64 `json:"temperature_min"`
	TemperatureMax float64 `json:"temperature_max"`
	WindSpeedMph float64 `json:"wind_speed_mph"`
	PrecipProbability int `json:"precip_probability"`
	ShortForecast string `json:"short_forecast"`
	FireRiskIndex string `json:"fire_risk_index"`
	FreezeWarning bool `json:"freeze_warning"`
	StationID string `json:"station_id"`
	ObservedAt time.Time `json:"observed_at"`
}

// Service manages live NOAA National Weather Service telemetry and Vertex AI Gemini 3.8 Flash synthesis.
type Service struct {
	HTTPClient *http.Client
	BaseNOAAURL string
	ProjectID string
	Location string
	Model string
	mu sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

func getDefaultService() *Service {
	return NewService()
}

// NewService instantiates an environmental telemetry service configured for live NOAA and Vertex AI.
func NewService() *Service {
	return &Service{
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		BaseNOAAURL: "https://api.weather.gov",
		ProjectID:   resolveGCPProject(),
		Location:    resolveGCPLocation(),
		Model:       "gemini-3.8-flash",
	}
}

// FetchNOAAMicroclimate queries the real NOAA National Weather Service API for the given coordinates.
// In accordance with the STRICT NEVER MOCK DIRECTIVE, real structured errors are returned upon failure.
func (s *Service) FetchNOAAMicroclimate(lat, lon float64) (*Telemetry, error) {
	if lat < -90.0 || lat > 90.0 || lon < -180.0 || lon > 180.0 {
		return nil, fmt.Errorf("environmental: invalid coordinates (lat=%.6f, lon=%.6f); must be within [-90,90] and [-180,180]", lat, lon)
	}

	client := s.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	baseURL := s.BaseNOAAURL
	if baseURL == "" {
		baseURL = "https://api.weather.gov"
	}

	// 1. Resolve NWS Grid Points endpoint
	pointsURL := fmt.Sprintf("%s/points/%.4f,%.4f", strings.TrimRight(baseURL, "/"), lat, lon)
	req, err := http.NewRequest("GET", pointsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("environmental: failed to construct NOAA points request: %w", err)
	}
	req.Header.Set("User-Agent", "(AlpineEscapes/2.0, contact@alpineescapes.org)")
	req.Header.Set("Accept", "application/geo+json, application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("environmental: NOAA points network request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("environmental: NOAA points API returned HTTP %d for coordinates (%.4f, %.4f): %s", resp.StatusCode, lat, lon, strings.TrimSpace(string(bodyBytes)))
	}

	var pointsPayload struct {
		Properties struct {
			CWA            string `json:"cwa"`
			GridID         string `json:"gridId"`
			RadarStation   string `json:"radarStation"`
			Forecast       string `json:"forecast"`
			ForecastHourly string `json:"forecastHourly"`
		} `json:"properties"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&pointsPayload); err != nil {
		return nil, fmt.Errorf("environmental: failed to parse NOAA points JSON payload: %w", err)
	}

	forecastURL := pointsPayload.Properties.Forecast
	if forecastURL == "" {
		forecastURL = pointsPayload.Properties.ForecastHourly
	}
	if forecastURL == "" {
		return nil, fmt.Errorf("environmental: NOAA points payload contains no forecast or forecastHourly endpoints")
	}

	stationID := pointsPayload.Properties.GridID
	if stationID == "" {
		stationID = pointsPayload.Properties.CWA
	}
	if stationID == "" {
		stationID = pointsPayload.Properties.RadarStation
	}

	// 2. Fetch Forecast Grid Periods
	fReq, err := http.NewRequest("GET", forecastURL, nil)
	if err != nil {
		return nil, fmt.Errorf("environmental: failed to construct NOAA forecast request: %w", err)
	}
	fReq.Header.Set("User-Agent", "(AlpineEscapes/2.0, contact@alpineescapes.org)")
	fReq.Header.Set("Accept", "application/geo+json, application/json")

	fResp, err := client.Do(fReq)
	if err != nil {
		return nil, fmt.Errorf("environmental: NOAA forecast network request failed: %w", err)
	}
	defer fResp.Body.Close()

	if fResp.StatusCode != http.StatusOK {
		fBody, _ := io.ReadAll(fResp.Body)
		return nil, fmt.Errorf("environmental: NOAA forecast API returned HTTP %d: %s", fResp.StatusCode, strings.TrimSpace(string(fBody)))
	}

	var forecastPayload struct {
		Properties struct {
			Updated string `json:"updated"`
			Periods []struct {
				Number                     int     `json:"number"`
				Name                       string  `json:"name"`
				IsDaytime                  bool    `json:"isDaytime"`
				Temperature                float64 `json:"temperature"`
				TemperatureUnit            string  `json:"temperatureUnit"`
				WindSpeed                  string  `json:"windSpeed"`
				ShortForecast              string  `json:"shortForecast"`
				ProbabilityOfPrecipitation *struct {
					Value *int `json:"value"`
				} `json:"probabilityOfPrecipitation"`
				RelativeHumidity *struct {
					Value *int `json:"value"`
				} `json:"relativeHumidity"`
			} `json:"periods"`
		} `json:"properties"`
	}

	if err := json.NewDecoder(fResp.Body).Decode(&forecastPayload); err != nil {
		return nil, fmt.Errorf("environmental: failed to parse NOAA forecast JSON: %w", err)
	}

	periods := forecastPayload.Properties.Periods
	if len(periods) == 0 {
		return nil, fmt.Errorf("environmental: NOAA forecast payload contains empty periods slice")
	}

	// Calculate temperature min and max across available frontier periods (up to 4 periods / ~48 hours)
	tempMin := periods[0].Temperature
	tempMax := periods[0].Temperature
	sampleLimit := 4
	if len(periods) < sampleLimit {
		sampleLimit = len(periods)
	}
	for i := 0; i < sampleLimit; i++ {
		if periods[i].Temperature < tempMin {
			tempMin = periods[i].Temperature
		}
		if periods[i].Temperature > tempMax {
			tempMax = periods[i].Temperature
		}
	}

	current := periods[0]
	windSpeed := parseWindSpeed(current.WindSpeed)

	precipProb := 0
	if current.ProbabilityOfPrecipitation != nil && current.ProbabilityOfPrecipitation.Value != nil {
		precipProb = *current.ProbabilityOfPrecipitation.Value
	}

	humidityPct := 40
	if current.RelativeHumidity != nil && current.RelativeHumidity.Value != nil {
		humidityPct = *current.RelativeHumidity.Value
	}

	fireRisk := CalculateFireRisk(tempMax, windSpeed, humidityPct)
	freezeWarning := tempMin <= 32.0

	observedAt := time.Now().UTC()
	if forecastPayload.Properties.Updated != "" {
		if t, err := time.Parse(time.RFC3339, forecastPayload.Properties.Updated); err == nil {
			observedAt = t
		}
	}

	return &Telemetry{
		TemperatureMin:    tempMin,
		TemperatureMax:    tempMax,
		WindSpeedMph:      windSpeed,
		PrecipProbability: precipProb,
		ShortForecast:     current.ShortForecast,
		FireRiskIndex:     fireRisk,
		FreezeWarning:     freezeWarning,
		StationID:         stationID,
		ObservedAt:        observedAt,
	}, nil
}

// FetchNOAAMicroclimate package-level wrapper using the default service.
func FetchNOAAMicroclimate(lat, lon float64) (*Telemetry, error) {
	return getDefaultService().FetchNOAAMicroclimate(lat, lon)
}

// CalculateFireRisk computes the algorithmic USFS fire danger rating based on temperature, wind, and humidity.
// Rating logic:
// - temp > 85 and humidity < 15 and wind > 20 -> "EXTREME"
// - temp > 75 and humidity < 25 and wind > 15 -> "HIGH"
// - temp > 65 and humidity < 35 -> "MODERATE"
// - else -> "LOW"
func CalculateFireRisk(tempF, windMph float64, humidityPct int) string {
	if tempF > 85.0 && humidityPct < 15 && windMph > 20.0 {
		return "EXTREME"
	}
	if tempF > 75.0 && humidityPct < 25 && windMph > 15.0 {
		return "HIGH"
	}
	if tempF > 65.0 && humidityPct < 35 {
		return "MODERATE"
	}
	return "LOW"
}

// CalculateFireRisk method on Service.
func (s *Service) CalculateFireRisk(tempF, windMph float64, humidityPct int) string {
	return CalculateFireRisk(tempF, windMph, humidityPct)
}

// GenerateCamperAdvisory formats an actionable markdown advisory with freeze, wind, and fire danger alerts.
func GenerateCamperAdvisory(telemetry *Telemetry) string {
	if telemetry == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("# Alpine Escapes - Environmental & Weather Advisory\n\n")

	if telemetry.StationID != "" {
		b.WriteString(fmt.Sprintf("**Station:** `%s` | **Observed:** %s\n\n",
			telemetry.StationID, telemetry.ObservedAt.Format(time.RFC1123)))
	}

	b.WriteString("### Microclimate Conditions\n")
	b.WriteString(fmt.Sprintf("- **Temperature Range:** Min %.1f°F / Max %.1f°F\n", telemetry.TemperatureMin, telemetry.TemperatureMax))
	b.WriteString(fmt.Sprintf("- **Sustained / Peak Winds:** %.1f mph\n", telemetry.WindSpeedMph))
	b.WriteString(fmt.Sprintf("- **Precipitation Probability:** %d%%\n", telemetry.PrecipProbability))
	b.WriteString(fmt.Sprintf("- **Forecast Outlook:** %s\n", telemetry.ShortForecast))
	b.WriteString(fmt.Sprintf("- **USFS Fire Danger Rating:** **%s**\n\n", telemetry.FireRiskIndex))

	b.WriteString("### Safety Directives & Gear Protocols\n")
	alertsTriggered := false

	if telemetry.FreezeWarning {
		alertsTriggered = true
		b.WriteString(fmt.Sprintf("> [!WARNING]\n> **FREEZE WARNING:** Minimum temperature will drop to %.1f°F (at or below 32°F freezing threshold). Sub-zero rated sleeping bags, insulated R-value 4.0+ sleeping pads, and 4-season shelter required. Invert and insulate water filtration elements to prevent ceramic micro-fractures.\n\n", telemetry.TemperatureMin))
	}

	if telemetry.WindSpeedMph > 25.0 {
		alertsTriggered = true
		b.WriteString(fmt.Sprintf("> [!WARNING]\n> **HIGH WIND ALERT:** Wind speeds reaching %.1f mph (>25 mph safety threshold). Double-stake tent corners and tighten storm guy lines. Never pitch shelters directly beneath dead snags or beetle-kill timber hazard trees.\n\n", telemetry.WindSpeedMph))
	}

	switch telemetry.FireRiskIndex {
	case "EXTREME":
		alertsTriggered = true
		b.WriteString("> [!CAUTION]\n> **STAGE 2 FIRE RESTRICTION - COMPLETE FIRE BAN:** Extreme wildfire risk under red-flag atmospheric conditions. Campfires, charcoal briquettes, open flames, and outdoor smoking are strictly prohibited. Pressurized liquid fuel stoves with emergency shutoff valves permitted only.\n\n")
	case "HIGH":
		alertsTriggered = true
		b.WriteString("> [!CAUTION]\n> **STAGE 1 FIRE RESTRICTION - ELEVATED HAZARD:** High wildfire hazard. Campfires permitted ONLY within designated USFS metal fire rings at active campsites. Maintain 5 gallons of water and an entrenching shovel on site at all times.\n\n")
	case "MODERATE":
		b.WriteString("> [!NOTE]\n> **MODERATE FIRE CAUTION:** Wildfire risk is elevated. Never leave burning embers unattended. Douse with water and stir until cold to the touch.\n\n")
	}

	if !alertsTriggered {
		b.WriteString("> [!NOTE]\n> **FAVORABLE CONDITIONS:** Atmospheric conditions are clear and stable. Follow standard Leave No Trace backcountry etiquette.\n\n")
	}

	return strings.TrimSpace(b.String())
}

// GenerateCamperAdvisory method on Service.
func (s *Service) GenerateCamperAdvisory(telemetry *Telemetry) string {
	return GenerateCamperAdvisory(telemetry)
}

// SynthesizeAdvisoryWithGemini integrates directly with Google Cloud Vertex AI Gemini 3.8 Flash via ADC.
// STRICT NEVER MOCK DIRECTIVE: Fails cleanly with real errors if ADC auth or Vertex AI execution fails.
func (s *Service) SynthesizeAdvisoryWithGemini(ctx context.Context, telemetry *Telemetry) (string, error) {
	if telemetry == nil {
		return "", fmt.Errorf("environmental: telemetry cannot be nil for Gemini synthesis")
	}

	token, err := s.getADCAuthToken(ctx)
	if err != nil {
		return "", fmt.Errorf("environmental: Google Cloud ADC authentication failed: %w", err)
	}

	endpoint := fmt.Sprintf(
		"https://aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:generateContent",
		s.ProjectID, s.Location, s.Model,
	)

	prompt := fmt.Sprintf(`You are Alpine-Shield, the real-time environmental risk telemetry specialist for Alpine Escapes.
Analyze the following backcountry microclimate telemetry:
- Station ID: %s
- Temperature Min / Max: %.1f°F / %.1f°F
- Peak Winds: %.1f mph
- Precipitation Probability: %d%%
- Short Forecast: %s
- USFS Fire Danger Rating: %s
- Freeze Warning Active: %t

Generate a concise, professional markdown advisory for backcountry campers detailing safety hazards, fire restrictions, and critical gear protocols.`,
		telemetry.StationID,
		telemetry.TemperatureMin,
		telemetry.TemperatureMax,
		telemetry.WindSpeedMph,
		telemetry.PrecipProbability,
		telemetry.ShortForecast,
		telemetry.FireRiskIndex,
		telemetry.FreezeWarning,
	)

	reqPayload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"role": "user",
				"parts": []map[string]interface{}{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.2,
			"maxOutputTokens": 1024,
		},
	}

	jsonBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("environmental: failed to serialize Vertex AI payload: %w", err)
	}

	client := s.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBytes))
	if err != nil {
		return "", fmt.Errorf("environmental: failed to build Vertex AI HTTP request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("environmental: Vertex AI request execution failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("environmental: failed reading Vertex AI response stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("environmental: Vertex AI returned HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(respBytes, &geminiResp); err != nil {
		return "", fmt.Errorf("environmental: failed to decode Vertex AI JSON response: %w", err)
	}

	if geminiResp.Error != nil {
		return "", fmt.Errorf("environmental: Vertex AI error (%d): %s", geminiResp.Error.Code, geminiResp.Error.Message)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("environmental: Vertex AI returned empty candidate parts")
	}

	return strings.TrimSpace(geminiResp.Candidates[0].Content.Parts[0].Text), nil
}

// SynthesizeAdvisoryWithGemini package-level helper.
func SynthesizeAdvisoryWithGemini(ctx context.Context, telemetry *Telemetry) (string, error) {
	return getDefaultService().SynthesizeAdvisoryWithGemini(ctx, telemetry)
}

// parseWindSpeed extracts the highest numeric value from a wind speed description (e.g. "10 to 25 mph" -> 25).
func parseWindSpeed(s string) float64 {
	pattern := regexp.MustCompile(`\d+`)
	matches := pattern.FindAllString(s, -1)
	if len(matches) == 0 {
		return 0.0
	}
	var maxSpeed float64
	for _, m := range matches {
		if val, err := strconv.ParseFloat(m, 64); err == nil {
			if val > maxSpeed {
				maxSpeed = val
			}
		}
	}
	return maxSpeed
}

func (s *Service) getADCAuthToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cachedToken != "" && time.Now().Before(s.tokenExpiry) {
		return s.cachedToken, nil
	}

	// 1. Env var GOOGLE_OAUTH_ACCESS_TOKEN
	if envTok := os.Getenv("GOOGLE_OAUTH_ACCESS_TOKEN"); envTok != "" {
		s.cachedToken = envTok
		s.tokenExpiry = time.Now().Add(5 * time.Minute)
		return envTok, nil
	}

	// 2. Cloud metadata server (Cloud Run / GCE)
	metaReq, _ := http.NewRequestWithContext(ctx, "GET", "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token", nil)
	metaReq.Header.Set("Metadata-Flavor", "Google")
	metaClient := &http.Client{Timeout: 800 * time.Millisecond}
	if resp, err := metaClient.Do(metaReq); err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		var metaToken struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int    `json:"expires_in"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&metaToken); err == nil && metaToken.AccessToken != "" {
			s.cachedToken = metaToken.AccessToken
			exp := metaToken.ExpiresIn - 60
			if exp < 60 {
				exp = 60
			}
			s.tokenExpiry = time.Now().Add(time.Duration(exp) * time.Second)
			return s.cachedToken, nil
		}
	}

	// 3. Application Default Credentials via gcloud CLI
	if out, err := exec.CommandContext(ctx, "gcloud", "auth", "application-default", "print-access-token").Output(); err == nil && len(out) > 0 {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if strings.HasPrefix(line, "ya29.") {
				s.cachedToken = line
				s.tokenExpiry = time.Now().Add(10 * time.Minute)
				return line, nil
			}
		}
	}

	// 4. Fallback gcloud access token
	if out, err := exec.CommandContext(ctx, "gcloud", "auth", "print-access-token").Output(); err == nil && len(out) > 0 {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if strings.HasPrefix(line, "ya29.") {
				s.cachedToken = line
				s.tokenExpiry = time.Now().Add(10 * time.Minute)
				return line, nil
			}
		}
	}

	return "", fmt.Errorf("environmental: could not retrieve Google Cloud ADC token from environment, metadata server, or gcloud CLI")
}

func resolveGCPProject() string {
	if p := os.Getenv("VERTEX_PROJECT"); p != "" {
		return p
	}
	if p := os.Getenv("GOOGLE_CLOUD_PROJECT"); p != "" {
		return p
	}
	if p := os.Getenv("GCP_PROJECT_ID"); p != "" {
		return p
	}
	if p := os.Getenv("PROJECT_ID"); p != "" {
		return p
	}
	if out, err := exec.Command("gcloud", "config", "get-value", "project").Output(); err == nil {
		if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
			return trimmed
		}
	}
	return "davenport-boutique"
}

func resolveGCPLocation() string {
	if loc := os.Getenv("VERTEX_LOCATION"); loc != "" {
		return loc
	}
	if loc := os.Getenv("GOOGLE_CLOUD_LOCATION"); loc != "" {
		return loc
	}
	return "us"
}

