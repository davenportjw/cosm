package weather

import (
	"fmt"
	"sync"
	"time"
)

// FireDangerLevel represents NFDRS wilderness danger classifications.
type FireDangerLevel string

const (
	FireDangerLow      FireDangerLevel = "LOW"
	FireDangerModerate FireDangerLevel = "MODERATE"
	FireDangerHigh     FireDangerLevel = "HIGH"
	FireDangerExtreme  FireDangerLevel = "EXTREME"
)

// CampfireAdvisory indicates permissible flame sources at campsites.
type CampfireAdvisory string

const (
	AdvisoryAllowed           CampfireAdvisory = "ALLOWED"
	AdvisoryContainedRingOnly CampfireAdvisory = "CONTAINED_RING_ONLY"
	AdvisoryPropaneOnly       CampfireAdvisory = "PROPANE_ONLY"
	AdvisoryTotalBan          CampfireAdvisory = "TOTAL_FIRE_BAN"
)

// CampsiteWeather holds real-time telemetry and calculated safety ratings.
type CampsiteWeather struct {
	CampsiteID       string           `json:"campsite_id"`
	CampsiteName     string           `json:"campsite_name"`
	ElevationFt      int              `json:"elevation_ft"`
	TemperatureF     float64          `json:"temperature_f"`
	HumidityPct      float64          `json:"humidity_pct"`
	WindSpeedMph     float64          `json:"wind_speed_mph"`
	WindGustMph      float64          `json:"wind_gust_mph"`
	Conditions       string           `json:"conditions"`
	FireDanger       FireDangerLevel  `json:"fire_danger"`
	CampfireAdvisory CampfireAdvisory `json:"campfire_advisory"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

// WeatherEngine manages microclimate telemetry for park campsites.
type WeatherEngine struct {
	mu        sync.RWMutex
	telemetry map[string]*CampsiteWeather
}

// NewWeatherEngine initializes an active telemetry engine with default campsite readings.
func NewWeatherEngine() *WeatherEngine {
	engine := &WeatherEngine{
		telemetry: make(map[string]*CampsiteWeather),
	}

	// Baseline seed data for Alpine Escapes campgrounds
	engine.telemetry["c1"] = engine.BuildTelemetry("c1", "Pine Ridge Site 4", 8200, 68.0, 32.0, 8.5, 12.0, "Sunny / Clear Skies")
	engine.telemetry["c2"] = engine.BuildTelemetry("c2", "Eagle Crest Yurt A", 9400, 61.0, 22.0, 16.5, 24.0, "Breezy / High Wind")
	engine.telemetry["c3"] = engine.BuildTelemetry("c3", "Timberline Cabin", 7800, 74.0, 14.0, 21.0, 28.0, "Dry / Red Flag Advisory")

	return engine
}

// BuildTelemetry constructs a new CampsiteWeather record with deterministic safety ratings.
func (e *WeatherEngine) BuildTelemetry(id, name string, elevation int, tempF, humidity, windMph, gustMph float64, conditions string) *CampsiteWeather {
	danger, advisory := EvaluateFireSafety(tempF, humidity, windMph)
	return &CampsiteWeather{
		CampsiteID:       id,
		CampsiteName:     name,
		ElevationFt:      elevation,
		TemperatureF:     tempF,
		HumidityPct:      humidity,
		WindSpeedMph:     windMph,
		WindGustMph:      gustMph,
		Conditions:       conditions,
		FireDanger:       danger,
		CampfireAdvisory: advisory,
		UpdatedAt:        time.Now().UTC(),
	}
}

// EvaluateFireSafety applies NFDRS environmental safety thresholds.
func EvaluateFireSafety(tempF, humidityPct, windSpeedMph float64) (FireDangerLevel, CampfireAdvisory) {
	if humidityPct < 15.0 && windSpeedMph >= 18.0 {
		return FireDangerExtreme, AdvisoryTotalBan
	}
	if humidityPct < 25.0 || windSpeedMph >= 15.0 {
		return FireDangerHigh, AdvisoryPropaneOnly
	}
	if humidityPct < 40.0 || windSpeedMph >= 10.0 {
		return FireDangerModerate, AdvisoryContainedRingOnly
	}
	return FireDangerLow, AdvisoryAllowed
}

// GetWeather retrieves the latest microclimate record for a campsite.
func (e *WeatherEngine) GetWeather(campsiteID string) (*CampsiteWeather, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	w, exists := e.telemetry[campsiteID]
	if !exists {
		return nil, fmt.Errorf("weather telemetry not available for campsite %q", campsiteID)
	}
	return w, nil
}

// UpdateWeather updates or inserts telemetry readings for a campsite.
func (e *WeatherEngine) UpdateWeather(w *CampsiteWeather) {
	e.mu.Lock()
	defer e.mu.Unlock()
	w.FireDanger, w.CampfireAdvisory = EvaluateFireSafety(w.TemperatureF, w.HumidityPct, w.WindSpeedMph)
	w.UpdatedAt = time.Now().UTC()
	e.telemetry[w.CampsiteID] = w
}

// ListAll returns all current campsite weather records.
func (e *WeatherEngine) ListAll() []*CampsiteWeather {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*CampsiteWeather, 0, len(e.telemetry))
	for _, w := range e.telemetry {
		result = append(result, w)
	}
	return result
}
