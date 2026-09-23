package environmental

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestCalculateFireRisk(t *testing.T) {
	tests := []struct {
		name        string
		tempF       float64
		windMph     float64
		humidityPct int
		expected    string
	}{
		// EXTREME: temp > 85, humidity < 15, wind > 20
		{
			name:        "extreme danger standard",
			tempF:       92.0,
			windMph:     24.0,
			humidityPct: 10,
			expected:    "EXTREME",
		},
		{
			name:        "extreme danger boundary just above thresholds",
			tempF:       85.1,
			windMph:     20.1,
			humidityPct: 14,
			expected:    "EXTREME",
		},
		// HIGH: temp > 75, humidity < 25, wind > 15
		{
			name:        "high danger standard",
			tempF:       80.0,
			windMph:     18.0,
			humidityPct: 20,
			expected:    "HIGH",
		},
		{
			name:        "high danger just above thresholds",
			tempF:       75.1,
			windMph:     15.1,
			humidityPct: 24,
			expected:    "HIGH",
		},
		{
			name:        "fails extreme on wind, qualifies as high",
			tempF:       90.0,
			windMph:     18.0, // <= 20 so not extreme, but > 15 so high
			humidityPct: 12,
			expected:    "HIGH",
		},
		{
			name:        "fails extreme on humidity, qualifies as high",
			tempF:       90.0,
			windMph:     25.0,
			humidityPct: 18, // >= 15 so not extreme, but < 25 so high
			expected:    "HIGH",
		},
		// MODERATE: temp > 65, humidity < 35
		{
			name:        "moderate danger standard",
			tempF:       70.0,
			windMph:     8.0,
			humidityPct: 30,
			expected:    "MODERATE",
		},
		{
			name:        "moderate danger boundary just above thresholds",
			tempF:       65.1,
			windMph:     5.0,
			humidityPct: 34,
			expected:    "MODERATE",
		},
		{
			name:        "high temp low wind moderate humidity",
			tempF:       80.0,
			windMph:     10.0, // <= 15 so not high, but temp > 65 and hum < 35
			humidityPct: 30,
			expected:    "MODERATE",
		},
		// LOW: all else
		{
			name:        "low danger cool temp",
			tempF:       60.0,
			windMph:     10.0,
			humidityPct: 20,
			expected:    "LOW",
		},
		{
			name:        "low danger high humidity",
			tempF:       90.0,
			windMph:     25.0,
			humidityPct: 60,
			expected:    "LOW",
		},
		{
			name:        "boundary temp exactly 65",
			tempF:       65.0,
			windMph:     20.0,
			humidityPct: 20,
			expected:    "LOW",
		},
		{
			name:        "boundary humidity exactly 35",
			tempF:       70.0,
			windMph:     5.0,
			humidityPct: 35,
			expected:    "LOW",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := CalculateFireRisk(tc.tempF, tc.windMph, tc.humidityPct)
			if actual != tc.expected {
				t.Errorf("CalculateFireRisk(%.1f, %.1f, %d) = %q; expected %q",
					tc.tempF, tc.windMph, tc.humidityPct, actual, tc.expected)
			}
		})
	}
}

func TestGenerateCamperAdvisory(t *testing.T) {
	t.Run("nil telemetry returns empty", func(t *testing.T) {
		adv := GenerateCamperAdvisory(nil)
		if adv != "" {
			t.Errorf("expected empty string for nil telemetry, got %q", adv)
		}
	})

	t.Run("freeze warning active", func(t *testing.T) {
		tel := &Telemetry{
			StationID:         "BOU",
			TemperatureMin:    24.5,
			TemperatureMax:    48.0,
			WindSpeedMph:      10.0,
			PrecipProbability: 20,
			ShortForecast:     "Clear and Cold",
			FireRiskIndex:     "LOW",
			FreezeWarning:     true,
			ObservedAt:        time.Now().UTC(),
		}
		adv := GenerateCamperAdvisory(tel)
		if !strings.Contains(adv, "FREEZE WARNING") {
			t.Errorf("expected FREEZE WARNING in advisory, got:\n%s", adv)
		}
		if !strings.Contains(adv, "24.5°F") {
			t.Errorf("expected 24.5°F in advisory, got:\n%s", adv)
		}
		if !strings.Contains(adv, "sleeping bags") {
			t.Errorf("expected cold gear guidance in advisory, got:\n%s", adv)
		}
	})

	t.Run("high wind alert active", func(t *testing.T) {
		tel := &Telemetry{
			StationID:         "BOU",
			TemperatureMin:    55.0,
			TemperatureMax:    72.0,
			WindSpeedMph:      32.0,
			PrecipProbability: 10,
			ShortForecast:     "Windy",
			FireRiskIndex:     "LOW",
			FreezeWarning:     false,
			ObservedAt:        time.Now().UTC(),
		}
		adv := GenerateCamperAdvisory(tel)
		if !strings.Contains(adv, "HIGH WIND ALERT") {
			t.Errorf("expected HIGH WIND ALERT in advisory, got:\n%s", adv)
		}
		if !strings.Contains(adv, "32.0 mph") {
			t.Errorf("expected 32.0 mph in advisory, got:\n%s", adv)
		}
		if !strings.Contains(adv, "guy lines") {
			t.Errorf("expected guy lines recommendation in advisory, got:\n%s", adv)
		}
	})

	t.Run("extreme fire danger active", func(t *testing.T) {
		tel := &Telemetry{
			StationID:         "BOU",
			TemperatureMin:    65.0,
			TemperatureMax:    95.0,
			WindSpeedMph:      22.0,
			PrecipProbability: 0,
			ShortForecast:     "Hot and Dry",
			FireRiskIndex:     "EXTREME",
			FreezeWarning:     false,
			ObservedAt:        time.Now().UTC(),
		}
		adv := GenerateCamperAdvisory(tel)
		if !strings.Contains(adv, "STAGE 2 FIRE RESTRICTION") {
			t.Errorf("expected STAGE 2 FIRE RESTRICTION in advisory, got:\n%s", adv)
		}
		if !strings.Contains(adv, "COMPLETE FIRE BAN") {
			t.Errorf("expected COMPLETE FIRE BAN in advisory, got:\n%s", adv)
		}
	})

	t.Run("high fire danger active", func(t *testing.T) {
		tel := &Telemetry{
			StationID:         "BOU",
			TemperatureMin:    50.0,
			TemperatureMax:    80.0,
			WindSpeedMph:      18.0,
			PrecipProbability: 5,
			ShortForecast:     "Sunny",
			FireRiskIndex:     "HIGH",
			FreezeWarning:     false,
			ObservedAt:        time.Now().UTC(),
		}
		adv := GenerateCamperAdvisory(tel)
		if !strings.Contains(adv, "STAGE 1 FIRE RESTRICTION") {
			t.Errorf("expected STAGE 1 FIRE RESTRICTION in advisory, got:\n%s", adv)
		}
	})

	t.Run("favorable conditions", func(t *testing.T) {
		tel := &Telemetry{
			StationID:         "BOU",
			TemperatureMin:    52.0,
			TemperatureMax:    74.0,
			WindSpeedMph:      8.0,
			PrecipProbability: 10,
			ShortForecast:     "Sunny",
			FireRiskIndex:     "LOW",
			FreezeWarning:     false,
			ObservedAt:        time.Now().UTC(),
		}
		adv := GenerateCamperAdvisory(tel)
		if !strings.Contains(adv, "FAVORABLE CONDITIONS") {
			t.Errorf("expected FAVORABLE CONDITIONS in advisory, got:\n%s", adv)
		}
	})
}

func TestFetchNOAAMicroclimate_ErrorHandling(t *testing.T) {
	svc := NewService()

	t.Run("invalid coordinates latitude out of bounds", func(t *testing.T) {
		_, err := svc.FetchNOAAMicroclimate(95.0, -105.0)
		if err == nil {
			t.Fatal("expected error for lat=95.0, got nil")
		}
		if !strings.Contains(err.Error(), "invalid coordinates") {
			t.Errorf("expected 'invalid coordinates' in error, got %v", err)
		}
	})

	t.Run("invalid coordinates longitude out of bounds", func(t *testing.T) {
		_, err := svc.FetchNOAAMicroclimate(40.0, -195.0)
		if err == nil {
			t.Fatal("expected error for lon=-195.0, got nil")
		}
		if !strings.Contains(err.Error(), "invalid coordinates") {
			t.Errorf("expected 'invalid coordinates' in error, got %v", err)
		}
	})

	t.Run("server returns 404 not found", func(t *testing.T) {
		client := &http.Client{
			Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Status:     "404 Not Found",
					Body:       io.NopCloser(strings.NewReader(`{"title": "Not Found", "detail": "Data not found for point"}`)),
					Header:     make(http.Header),
				}, nil
			}),
		}

		testSvc := &Service{
			HTTPClient:  client,
			BaseNOAAURL: "https://api.weather.gov",
		}

		_, err := testSvc.FetchNOAAMicroclimate(39.7392, -104.9903)
		if err == nil {
			t.Fatal("expected error for 404 response, got nil")
		}
		if !strings.Contains(err.Error(), "HTTP 404") {
			t.Errorf("expected 'HTTP 404' in error message, got %v", err)
		}
	})

	t.Run("server returns 500 internal server error", func(t *testing.T) {
		client := &http.Client{
			Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Status:     "500 Internal Server Error",
					Body:       io.NopCloser(strings.NewReader("internal server error")),
					Header:     make(http.Header),
				}, nil
			}),
		}

		testSvc := &Service{
			HTTPClient:  client,
			BaseNOAAURL: "https://api.weather.gov",
		}

		_, err := testSvc.FetchNOAAMicroclimate(39.7392, -104.9903)
		if err == nil {
			t.Fatal("expected error for 500 response, got nil")
		}
		if !strings.Contains(err.Error(), "HTTP 500") {
			t.Errorf("expected 'HTTP 500' in error message, got %v", err)
		}
	})

	t.Run("network timeout", func(t *testing.T) {
		client := &http.Client{
			Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				return nil, context.DeadlineExceeded
			}),
		}

		testSvc := &Service{
			HTTPClient:  client,
			BaseNOAAURL: "https://api.weather.gov",
		}

		_, err := testSvc.FetchNOAAMicroclimate(39.7392, -104.9903)
		if err == nil {
			t.Fatal("expected network timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "network request failed") && !strings.Contains(err.Error(), "Client.Timeout exceeded") && !strings.Contains(err.Error(), "context deadline exceeded") {
			t.Errorf("expected timeout/network error, got %v", err)
		}
	})
}

func TestFetchNOAAMicroclimate_EndToEndStructured(t *testing.T) {
	client := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if strings.HasPrefix(r.URL.Path, "/points/") {
				payload := map[string]interface{}{
					"properties": map[string]interface{}{
						"gridId":         "BOU",
						"radarStation":   "KFTG",
						"forecast":       "https://api.weather.gov/gridpoints/BOU/63,62/forecast",
						"forecastHourly": "https://api.weather.gov/gridpoints/BOU/63,62/forecast/hourly",
					},
				}
				body, _ := json.Marshal(payload)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/geo+json"}},
					Body:       io.NopCloser(bytes.NewReader(body)),
				}, nil
			}

			if strings.HasPrefix(r.URL.Path, "/gridpoints/BOU/63,62/forecast") {
				precipVal := 45
				humVal := 12
				payload := map[string]interface{}{
					"properties": map[string]interface{}{
						"updated": "2026-09-18T20:00:00Z",
						"periods": []map[string]interface{}{
							{
								"number":          1,
								"name":            "Tonight",
								"temperature":     29.0,
								"temperatureUnit": "F",
								"windSpeed":       "15 to 28 mph",
								"shortForecast":   "Freezing Breezy and Clear",
								"probabilityOfPrecipitation": map[string]interface{}{
									"value": &precipVal,
								},
								"relativeHumidity": map[string]interface{}{
									"value": &humVal,
								},
							},
							{
								"number":          2,
								"name":            "Saturday",
								"temperature":     88.0,
								"temperatureUnit": "F",
								"windSpeed":       "25 mph",
								"shortForecast":   "Sunny and Hot",
							},
						},
					},
				}
				body, _ := json.Marshal(payload)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/geo+json"}},
					Body:       io.NopCloser(bytes.NewReader(body)),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader("404 Not Found")),
			}, nil
		}),
	}

	svc := &Service{
		HTTPClient:  client,
		BaseNOAAURL: "https://api.weather.gov",
	}

	tel, err := svc.FetchNOAAMicroclimate(39.7392, -104.9903)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tel.StationID != "BOU" {
		t.Errorf("expected StationID BOU, got %q", tel.StationID)
	}
	if tel.TemperatureMin != 29.0 {
		t.Errorf("expected TemperatureMin 29.0, got %.1f", tel.TemperatureMin)
	}
	if tel.TemperatureMax != 88.0 {
		t.Errorf("expected TemperatureMax 88.0, got %.1f", tel.TemperatureMax)
	}
	if tel.WindSpeedMph != 28.0 {
		t.Errorf("expected WindSpeedMph 28.0, got %.1f", tel.WindSpeedMph)
	}
	if tel.PrecipProbability != 45 {
		t.Errorf("expected PrecipProbability 45, got %d", tel.PrecipProbability)
	}
	if tel.ShortForecast != "Freezing Breezy and Clear" {
		t.Errorf("expected ShortForecast 'Freezing Breezy and Clear', got %q", tel.ShortForecast)
	}
	if !tel.FreezeWarning {
		t.Errorf("expected FreezeWarning true for 29.0°F min temp")
	}
	// Max temp 88.0, wind 28.0, humidity 12 -> EXTREME fire danger
	if tel.FireRiskIndex != "EXTREME" {
		t.Errorf("expected FireRiskIndex EXTREME, got %q", tel.FireRiskIndex)
	}
}

func TestLiveNOAAService(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live NOAA network call in short mode")
	}

	svc := NewService()
	// Denver, Colorado backcountry gateway coordinates
	tel, err := svc.FetchNOAAMicroclimate(39.7392, -104.9903)
	if err != nil {
		t.Logf("Live NOAA query returned: %v (network or endpoint rate-limiting may apply in hermetic test)", err)
		return
	}

	if tel == nil {
		t.Fatal("expected non-nil telemetry from live NOAA query")
	}

	t.Logf("Live NOAA Telemetry: Station=%s TempMin=%.1f TempMax=%.1f Wind=%.1f Precip=%d%% FireRisk=%s Freeze=%t",
		tel.StationID, tel.TemperatureMin, tel.TemperatureMax, tel.WindSpeedMph, tel.PrecipProbability, tel.FireRiskIndex, tel.FreezeWarning)

	if tel.StationID == "" {
		t.Errorf("expected non-empty StationID from live NOAA")
	}

	adv := svc.GenerateCamperAdvisory(tel)
	if adv == "" {
		t.Errorf("expected non-empty camper advisory from live telemetry")
	}
}

func TestLiveVertexAIGeminiSynthesis(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live Vertex AI synthesis in short mode")
	}

	svc := NewService()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sampleTel := &Telemetry{
		StationID:         "BOU",
		TemperatureMin:    26.0,
		TemperatureMax:    58.0,
		WindSpeedMph:      30.0,
		PrecipProbability: 40,
		ShortForecast:     "Chance Snow Showers and Windy",
		FireRiskIndex:     "MODERATE",
		FreezeWarning:     true,
		ObservedAt:        time.Now().UTC(),
	}

	advisory, err := svc.SynthesizeAdvisoryWithGemini(ctx, sampleTel)
	if err != nil {
		t.Logf("Live Vertex AI call result: %v (ADC or network access dependent)", err)
		return
	}

	if advisory == "" {
		t.Errorf("expected non-empty Gemini advisory output")
	}
	t.Logf("Vertex AI Gemini 3.8 Flash Advisory Output:\n%s", advisory)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)
