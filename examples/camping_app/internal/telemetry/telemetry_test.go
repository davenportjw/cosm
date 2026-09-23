package telemetry

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestRothermelRateOfSpread_ZeroWindZeroSlopeBaseline(t *testing.T) {
	// Standard Fuel Model 1 (Short Grass)
	fuel := FuelModel1
	moisturePct := 8.0 // 8% fuel moisture (below 12% extinction)
	windMph := 0.0
	slopeDeg := 0.0

	ros, fl, intens := CalculateRothermelRateOfSpread(fuel, moisturePct, windMph, slopeDeg)

	if ros <= 0 {
		t.Fatalf("expected positive baseline rate of spread for Model 1, got %.4f ft/min", ros)
	}
	if fl <= 0 {
		t.Fatalf("expected positive baseline flame length for Model 1, got %.4f ft", fl)
	}
	if intens <= 0 {
		t.Fatalf("expected positive baseline fireline intensity for Model 1, got %.4f Btu/ft-s", intens)
	}

	t.Logf("Model 1 Baseline (Wind=0, Slope=0, Moisture=8%%): ROS=%.2f ft/min, FlameLength=%.2f ft, Intensity=%.2f Btu/ft-s",
		ros, fl, intens)

	// Test moisture of extinction: when moisture >= 12%, fire does not spread
	rosExt, flExt, intensExt := CalculateRothermelRateOfSpread(fuel, 12.0, windMph, slopeDeg)
	if rosExt != 0 || flExt != 0 || intensExt != 0 {
		t.Fatalf("expected fire extinction (all 0) at moisture >= 12%%, got ROS=%.4f, FL=%.4f, I=%.4f",
			rosExt, flExt, intensExt)
	}

	// Model 10 (Timber Litter & Understory)
	fuel10 := FuelModel10
	ros10, fl10, intens10 := CalculateRothermelRateOfSpread(fuel10, 10.0, 0.0, 0.0)
	if ros10 <= 0 || fl10 <= 0 || intens10 <= 0 {
		t.Fatalf("expected positive baseline for Model 10, got ROS=%.4f, FL=%.4f, I=%.4f", ros10, fl10, intens10)
	}
	t.Logf("Model 10 Baseline: ROS=%.2f ft/min, FlameLength=%.2f ft, Intensity=%.2f Btu/ft-s", ros10, fl10, intens10)
}

func TestRothermelRateOfSpread_MonotonicWind(t *testing.T) {
	fuel := FuelModel1
	moisturePct := 6.0
	slopeDeg := 0.0

	winds := []float64{0.0, 3.0, 7.0, 12.0, 20.0, 30.0}
	prevROS := -1.0
	prevFL := -1.0

	for _, w := range winds {
		ros, fl, _ := CalculateRothermelRateOfSpread(fuel, moisturePct, w, slopeDeg)
		if ros <= prevROS {
			t.Fatalf("rate of spread failed monotonicity at wind %.1f mph: ros=%.4f <= prevROS=%.4f", w, ros, prevROS)
		}
		if fl < prevFL {
			t.Fatalf("flame length failed monotonicity at wind %.1f mph: fl=%.4f < prevFL=%.4f", w, fl, prevFL)
		}
		t.Logf("Wind %.1f mph -> ROS = %.2f ft/min, Flame Length = %.2f ft", w, ros, fl)
		prevROS = ros
		prevFL = fl
	}
}

func TestRothermelRateOfSpread_MonotonicSlope(t *testing.T) {
	fuel := FuelModel4 // Chaparral
	moisturePct := 8.0
	windMph := 0.0

	slopes := []float64{0.0, 5.0, 15.0, 25.0, 35.0, 45.0}
	prevROS := -1.0

	for _, s := range slopes {
		ros, _, _ := CalculateRothermelRateOfSpread(fuel, moisturePct, windMph, s)
		if ros <= prevROS {
			t.Fatalf("rate of spread failed monotonicity at slope %.1f deg: ros=%.4f <= prevROS=%.4f", s, ros, prevROS)
		}
		t.Logf("Slope %.1f deg -> ROS = %.2f ft/min", s, ros)
		prevROS = ros
	}
}

func TestRothermelFlameLengthCalculation(t *testing.T) {
	fuel := FuelModel10
	ros, fl, intens := CalculateRothermelRateOfSpread(fuel, 8.0, 15.0, 10.0)

	if intens <= 0 {
		t.Fatalf("expected positive intensity, got %.4f", intens)
	}

	// Byram's equation: F_L = 0.45 * I^0.46
	expectedFL := 0.45 * math.Pow(intens, 0.46)
	if math.Abs(fl-expectedFL) > 1e-6 {
		t.Fatalf("flame length mismatch: expected %.6f, got %.6f (ROS=%.2f, I=%.2f)", expectedFL, fl, ros, intens)
	}

	// Zero intensity produces zero flame length
	_, flZero, _ := CalculateRothermelRateOfSpread(fuel, 50.0, 0, 0)
	if flZero != 0 {
		t.Fatalf("expected 0 flame length for extinguished fire, got %.4f", flZero)
	}
}

func TestStandardFuelModelsRegistry(t *testing.T) {
	expectedModels := []int{1, 4, 8, 10}
	for _, m := range expectedModels {
		model, err := GetStandardFuelModel(m)
		if err != nil {
			t.Fatalf("failed retrieving fuel model %d: %v", m, err)
		}
		if model.FuelModelNumber != m {
			t.Fatalf("fuel model number mismatch: got %d, want %d", model.FuelModelNumber, m)
		}
		if model.FuelBedDepthFt <= 0 || model.SAVR <= 0 || model.HeatContentBtuPerLb <= 0 {
			t.Fatalf("fuel model %d has invalid physical constants: %+v", m, model)
		}
	}

	// Unsupported model returns error
	if _, err := GetStandardFuelModel(999); err == nil {
		t.Fatalf("expected error for unsupported fuel model 999")
	}
}

func TestSensorMesh_IngestionAndPrediction(t *testing.T) {
	mesh := NewSensorMesh()

	now := time.Now()
	r1 := SensorReading{
		SensorID:        "SN-ELEV-5200",
		ElevationFt:     5200,
		TemperatureF:    85.0,
		HumidityPct:     18.0,
		WindSpeedMph:    8.0,
		FuelMoisturePct: 7.0,
		Timestamp:       now,
	}
	r2 := SensorReading{
		SensorID:        "SN-ELEV-6800",
		ElevationFt:     6800,
		TemperatureF:    78.0,
		HumidityPct:     24.0,
		WindSpeedMph:    14.0,
		FuelMoisturePct: 9.0,
		Timestamp:       now,
	}
	r3 := SensorReading{
		SensorID:        "SN-ELEV-8500",
		ElevationFt:     8500,
		TemperatureF:    65.0,
		HumidityPct:     35.0,
		WindSpeedMph:    22.0,
		FuelMoisturePct: 12.0,
		Timestamp:       now,
	}

	mesh.AddReading(r1)
	mesh.AddReading(r2)
	mesh.AddReading(r3)

	latest := mesh.GetLatestReadings()
	if len(latest) != 3 {
		t.Fatalf("expected 3 latest readings, got %d", len(latest))
	}

	// Test elevation band filter [6000, 8000]
	bandReadings := mesh.GetReadingsInElevationBand(6000, 8000)
	if len(bandReadings) != 1 || bandReadings[0].SensorID != "SN-ELEV-6800" {
		t.Fatalf("expected 1 reading in band [6000, 8000], got %+v", bandReadings)
	}

	// Predict spread across mesh
	preds := mesh.PredictSpread(FuelModel10, 10.0)
	if len(preds) != 3 {
		t.Fatalf("expected 3 predictions, got %d", len(preds))
	}
	for id, ros := range preds {
		if ros <= 0 {
			t.Fatalf("expected positive rate of spread for sensor %s, got %.2f", id, ros)
		}
		t.Logf("Mesh Sensor %s: Rate of Spread = %.2f ft/min", id, ros)
	}
}

func TestGenerateRangerBriefing_Deterministic(t *testing.T) {
	now := time.Now()
	readings := []SensorReading{
		{
			SensorID:        "S-LOWER-CANYON",
			ElevationFt:     5400,
			TemperatureF:    88.0,
			HumidityPct:     12.0,
			WindSpeedMph:    18.0,
			FuelMoisturePct: 5.5,
			Timestamp:       now,
		},
		{
			SensorID:        "S-UPPER-RIDGE",
			ElevationFt:     7600,
			TemperatureF:    76.0,
			HumidityPct:     15.0,
			WindSpeedMph:    28.0,
			FuelMoisturePct: 6.0,
			Timestamp:       now,
		},
	}

	briefing, err := GenerateRangerBriefing(readings, 2, 7)
	if err != nil {
		t.Fatalf("unexpected error generating briefing: %v", err)
	}
	if briefing == nil {
		t.Fatalf("expected non-nil briefing")
	}

	if len(briefing.FireSpreadPredictions) != 2 {
		t.Fatalf("expected 2 predictions, got %d", len(briefing.FireSpreadPredictions))
	}

	if !strings.Contains(briefing.SummaryMarkdown, "Automated Backcountry Tactical Fire Briefing") {
		t.Fatalf("summary markdown missing title header: %s", briefing.SummaryMarkdown)
	}
	if !strings.Contains(briefing.SummaryMarkdown, "S-UPPER-RIDGE") {
		t.Fatalf("summary markdown missing sensor ID: %s", briefing.SummaryMarkdown)
	}
	if briefing.EvacuationAdvice == "" {
		t.Fatalf("evacuation advice should not be empty")
	}

	t.Logf("Generated Briefing:\nModel: %s\nAdvice: %s\nMarkdown Snippet:\n%s",
		briefing.ModelUsed, briefing.EvacuationAdvice, briefing.SummaryMarkdown[:250])

	// Test empty readings handling
	emptyBriefing, err := GenerateRangerBriefing(nil, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error on empty readings: %v", err)
	}
	if !strings.Contains(emptyBriefing.SummaryMarkdown, "No active sensor telemetry") {
		t.Fatalf("expected empty telemetry notice, got: %s", emptyBriefing.SummaryMarkdown)
	}
}

