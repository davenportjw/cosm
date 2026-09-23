package astronomy

import (
	"sync"
	"time"
)

// BortleClass represents celestial dark sky clarity.
type BortleClass int

const (
	BortleClass1 BortleClass = 1 // Excellent dark-sky site
	BortleClass2 BortleClass = 2 // Typical truly dark site
	BortleClass3 BortleClass = 3 // Rural sky
	BortleClass4 BortleClass = 4 // Rural/suburban transition
	BortleClass5 BortleClass = 5 // Suburban sky
)

// StargazingForecast represents night-sky observation clarity for a campsite.
type StargazingForecast struct {
	CampsiteID     string      `json:"campsite_id"`
	CampsiteName   string      `json:"campsite_name"`
	Bortle         BortleClass `json:"bortle_class"`
	CloudCoverPct  float64     `json:"cloud_cover_pct"`
	SeeingScore    int         `json:"seeing_score"`   // 0-100
	ViewingRating  string      `json:"viewing_rating"` // OPTIMAL, GOOD, FAIR, POOR
	VisibleObjects []string    `json:"visible_objects"`
	ForecastAt     time.Time   `json:"forecast_at"`
}

// AstronomyEngine calculates astronomical viewing clarity.
type AstronomyEngine struct {
	mu        sync.RWMutex
	forecasts map[string]*StargazingForecast
}

// NewAstronomyEngine creates a new astronomy engine.
func NewAstronomyEngine() *AstronomyEngine {
	return &AstronomyEngine{
		forecasts: make(map[string]*StargazingForecast),
	}
}

// CalculateObservationRating evaluates cloud cover, seeing score, and Bortle class.
func CalculateObservationRating(cloudCover float64, seeingScore int, bortle BortleClass) string {
	if cloudCover > 60.0 || seeingScore < 30 {
		return "POOR"
	}
	if cloudCover > 30.0 || bortle > BortleClass4 {
		return "FAIR"
	}
	if seeingScore >= 75 && bortle <= BortleClass3 {
		return "OPTIMAL"
	}
	return "GOOD"
}
