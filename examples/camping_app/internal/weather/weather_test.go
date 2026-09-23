package weather

import (
	"testing"
)

func TestEvaluateFireSafety(t *testing.T) {
	tests := []struct {
		name         string
		tempF        float64
		humidity     float64
		wind         float64
		wantDanger   FireDangerLevel
		wantAdvisory CampfireAdvisory
	}{
		{
			name:         "Extreme Danger High Wind Dry",
			tempF:        85.0,
			humidity:     12.0,
			wind:         22.0,
			wantDanger:   FireDangerExtreme,
			wantAdvisory: AdvisoryTotalBan,
		},
		{
			name:         "High Danger Moderate Wind",
			tempF:        78.0,
			humidity:     20.0,
			wind:         16.0,
			wantDanger:   FireDangerHigh,
			wantAdvisory: AdvisoryPropaneOnly,
		},
		{
			name:         "Moderate Danger Low Wind",
			tempF:        70.0,
			humidity:     35.0,
			wind:         11.0,
			wantDanger:   FireDangerModerate,
			wantAdvisory: AdvisoryContainedRingOnly,
		},
		{
			name:         "Low Danger Cool Moist",
			tempF:        62.0,
			humidity:     55.0,
			wind:         5.0,
			wantDanger:   FireDangerLow,
			wantAdvisory: AdvisoryAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDanger, gotAdvisory := EvaluateFireSafety(tt.tempF, tt.humidity, tt.wind)
			if gotDanger != tt.wantDanger {
				t.Errorf("EvaluateFireSafety() gotDanger = %v, want %v", gotDanger, tt.wantDanger)
			}
			if gotAdvisory != tt.wantAdvisory {
				t.Errorf("EvaluateFireSafety() gotAdvisory = %v, want %v", gotAdvisory, tt.wantAdvisory)
			}
		})
	}
}

func TestWeatherEngineLifecycle(t *testing.T) {
	engine := NewWeatherEngine()

	// 1. Check seed data
	c1, err := engine.GetWeather("c1")
	if err != nil {
		t.Fatalf("expected c1 weather, got error: %v", err)
	}
	if c1.CampsiteID != "c1" || c1.ElevationFt != 8200 {
		t.Errorf("unexpected telemetry fields: %+v", c1)
	}

	// 2. Query unknown campsite
	_, err = engine.GetWeather("c999")
	if err == nil {
		t.Fatalf("expected error for non-existent campsite c999")
	}

	// 3. Update weather telemetry
	updated := engine.BuildTelemetry("c1", "Pine Ridge Site 4", 8200, 92.0, 10.0, 25.0, 35.0, "Severe Heat & Gale")
	engine.UpdateWeather(updated)

	fresh, _ := engine.GetWeather("c1")
	if fresh.FireDanger != FireDangerExtreme || fresh.CampfireAdvisory != AdvisoryTotalBan {
		t.Errorf("expected extreme fire ban, got %v / %v", fresh.FireDanger, fresh.CampfireAdvisory)
	}

	// 4. List all
	all := engine.ListAll()
	if len(all) < 3 {
		t.Errorf("expected at least 3 campsites, got %d", len(all))
	}
}
