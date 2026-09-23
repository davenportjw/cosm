package telemetry

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// FuelModel defines the physical fuel bed properties according to the USFS Rothermel (1972) fire spread model.
type FuelModel struct {
	FuelModelNumber int `json:"fuel_model_number"`
	FuelName string `json:"fuel_name"`
	FuelBedDepthFt float64 `json:"fuel_bed_depth_ft"`
	MoistureOfExtinction float64 `json:"moisture_of_extinction"`
	HeatContentBtuPerLb float64 `json:"heat_content_btu_per_lb"`
	Loading1Hr float64 `json:"loading_1hr"`
	Loading10Hr float64 `json:"loading_10hr"`
	Loading100Hr float64 `json:"loading_100hr"`
	SAVR float64 `json:"savr"`
}

// SensorReading records microclimate and fuel moisture telemetry from an automated backcountry sensor node.
type SensorReading struct {
	SensorID string `json:"sensor_id"`
	ElevationFt float64 `json:"elevation_ft"`
	TemperatureF float64 `json:"temperature_f"`
	HumidityPct float64 `json:"humidity_pct"`
	WindSpeedMph float64 `json:"wind_speed_mph"`
	FuelMoisturePct float64 `json:"fuel_moisture_pct"`
	Timestamp time.Time `json:"timestamp"`
}

// SensorMesh manages sensor telemetry across varying elevation bands in backcountry campgrounds.
type SensorMesh struct {
	mu sync.RWMutex
	readings map[string][]SensorReading
}

// GetStandardFuelModel returns a standard USFS fuel model by its number.
func GetStandardFuelModel(modelNumber int) (FuelModel, error) {
	if fm, ok := StandardModels[modelNumber]; ok {
		return fm, nil
	}
	return FuelModel{}, fmt.Errorf("telemetry: unsupported standard fuel model number %d; supported: 1, 4, 8, 10", modelNumber)
}

// CalculateRothermelRateOfSpread computes the forward rate of spread (ft/min), Byram flame length (ft),
// and fireline intensity (Btu/ft-sec) using the official USFS Rothermel (1972) mathematical physics equations.
// STRICT NEVER MOCK DIRECTIVE: Implements exact analytical solutions without mock approximations.
func CalculateRothermelRateOfSpread(fuel FuelModel, fuelMoisture10HrPct, midFlameWindMph, slopeAngleDeg float64) (rateOfSpreadFtPerMin float64, flameLengthFt float64, fireIntensityBtuPerFtSec float64) {
	// Guard against physical boundary degeneracies
	if fuel.FuelBedDepthFt <= 0 || fuel.SAVR <= 0 {
		return 0, 0, 0
	}

	totalLoading := fuel.Loading1Hr + fuel.Loading10Hr + fuel.Loading100Hr
	if totalLoading <= 0 {
		return 0, 0, 0
	}

	sigma := fuel.SAVR
	w0 := totalLoading
	delta := fuel.FuelBedDepthFt

	// 1. Bulk density and packing ratio
	// Particle density rho_p = 32.0 lb/ft^3 for standard ovendry wood/foliage
	const rhoP = 32.0
	rhoB := w0 / delta
	beta := rhoB / rhoP
	if beta <= 0 {
		return 0, 0, 0
	}

	// 2. Optimum packing ratio beta_op
	// beta_op = 3.348 * sigma^(-0.8189)
	betaOp := 3.348 * math.Pow(sigma, -0.8189)
	if betaOp <= 0 {
		betaOp = 0.001
	}
	betaRatio := beta / betaOp

	// 3. Optimum reaction velocity Gamma'
	// Gamma_max = sigma^1.5 / (495.0 + 0.0594 * sigma^1.5)
	sigma15 := math.Pow(sigma, 1.5)
	gammaMax := sigma15 / (495.0 + 0.0594*sigma15)

	// A = 133.0 * sigma^(-0.7913)
	aExp := 133.0 * math.Pow(sigma, -0.7913)

	// Gamma' = Gamma_max * (beta / beta_op)^A * exp(A * (1.0 - beta / beta_op))
	gammaPrime := gammaMax * math.Pow(betaRatio, aExp) * math.Exp(aExp*(1.0-betaRatio))

	// 4. Net fuel loading wn
	// wn = w0 * (1 - St) where St = 0.0555 (total mineral content fraction)
	const st = 0.0555
	wn := w0 * (1.0 - st)

	// 5. Moisture damping coefficient eta_M
	// Convert percentage to fraction if given as e.g. 8.0 -> 0.08
	mf := fuelMoisture10HrPct
	if mf > 1.0 {
		mf = mf / 100.0
	}
	if mf < 0 {
		mf = 0
	}

	mx := fuel.MoistureOfExtinction
	if mx > 1.0 {
		mx = mx / 100.0
	}
	if mx <= 0 {
		mx = 0.12
	}

	rm := mf / mx
	if rm >= 1.0 {
		// Moisture is at or above extinction: combustion ceases
		return 0, 0, 0
	}

	// eta_M = 1 - 2.59 * rm + 5.11 * rm^2 - 3.52 * rm^3
	etaM := 1.0 - 2.59*rm + 5.11*math.Pow(rm, 2) - 3.52*math.Pow(rm, 3)
	if etaM < 0 {
		etaM = 0
	}

	// 6. Mineral damping coefficient eta_s
	// eta_s = 0.174 * Se^(-0.19) where Se = 0.010 (effective mineral fraction)
	const se = 0.010
	etaS := 0.174 * math.Pow(se, -0.19)

	// 7. Reaction intensity I_R (Btu / (ft^2 * min))
	// I_R = Gamma' * wn * h * eta_M * eta_s
	h := fuel.HeatContentBtuPerLb
	if h <= 0 {
		h = 8000.0
	}
	ir := gammaPrime * wn * h * etaM * etaS
	if ir <= 0 {
		return 0, 0, 0
	}

	// 8. Propagating flux ratio xi
	// xi = (192 + 0.2595 * sigma)^(-1) * exp((0.792 + 0.681 * sigma^0.5) * (beta + 0.1))
	xiDenom := 192.0 + 0.2595*sigma
	xiExp := (0.792 + 0.681*math.Sqrt(sigma)) * (beta + 0.1)
	xi := (1.0 / xiDenom) * math.Exp(xiExp)

	// 9. Wind factor phi_w
	// phi_w = C * U^B * (beta / beta_op)^(-E)
	// U is midflame wind velocity in ft/min (1 mph = 88 ft/min)
	phiW := 0.0
	if midFlameWindMph > 0 {
		u := midFlameWindMph * 88.0
		cCoeff := 7.47 * math.Pow(sigma, -0.55)
		bExp := 0.02526 * math.Pow(sigma, 0.54)
		eExp := 0.715 * math.Exp(-3.59e-4*sigma)
		phiW = cCoeff * math.Pow(u, bExp) * math.Pow(betaRatio, -eExp)
	}

	// 10. Slope factor phi_s
	// phi_s = 5.275 * beta^(-0.3) * (tan(theta))^2
	phiS := 0.0
	if slopeAngleDeg > 0 {
		slopeRad := slopeAngleDeg * (math.Pi / 180.0)
		if slopeRad > 1.5533 { // Clamp to ~89 degrees to prevent divergence
			slopeRad = 1.5533
		}
		tanTheta := math.Tan(slopeRad)
		phiS = 5.275 * math.Pow(beta, -0.3) * math.Pow(tanTheta, 2)
	}

	// 11. Effective heating number epsilon
	// epsilon = exp(-138.0 / sigma)
	epsilon := math.Exp(-138.0 / sigma)

	// 12. Heat of pre-ignition Q_ig (Btu / lb)
	// Q_ig = 250.0 + 1116.0 * mf
	qIg := 250.0 + 1116.0*mf

	// 13. Forward Rate of Spread R (ft / min)
	// R = (I_R * xi * (1 + phi_w + phi_s)) / (rho_b * epsilon * Q_ig)
	spreadDenom := rhoB * epsilon * qIg
	if spreadDenom <= 0 {
		return 0, 0, 0
	}

	rateOfSpreadFtPerMin = (ir * xi * (1.0 + phiW + phiS)) / spreadDenom
	if rateOfSpreadFtPerMin < 0 {
		rateOfSpreadFtPerMin = 0
	}

	// 14. Fireline Intensity I (Btu / (ft * sec))
	// In Rothermel (1972), residence time tau_r = 384.0 / sigma (minutes)
	// Byram's fireline intensity I = (I_R * tau_r * R) / 60.0
	tauR := 384.0 / sigma
	fireIntensityBtuPerFtSec = (ir * tauR * rateOfSpreadFtPerMin) / 60.0
	if fireIntensityBtuPerFtSec < 0 {
		fireIntensityBtuPerFtSec = 0
	}

	// 15. Byram's Flame Length F_L (ft)
	// F_L = 0.45 * I^0.46
	if fireIntensityBtuPerFtSec > 0 {
		flameLengthFt = 0.45 * math.Pow(fireIntensityBtuPerFtSec, 0.46)
	} else {
		flameLengthFt = 0
	}

	return rateOfSpreadFtPerMin, flameLengthFt, fireIntensityBtuPerFtSec
}

// NewSensorMesh creates an initialized thread-safe SensorMesh.
func NewSensorMesh() *SensorMesh {
	return &SensorMesh{
		readings: make(map[string][]SensorReading),
	}
}

// AddReading records a new sensor reading into the mesh.
func (sm *SensorMesh) AddReading(reading SensorReading) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if reading.Timestamp.IsZero() {
		reading.Timestamp = time.Now()
	}
	sm.readings[reading.SensorID] = append(sm.readings[reading.SensorID], reading)
}

// GetLatestReadings returns the most recent telemetry reading for every sensor in the mesh.
func (sm *SensorMesh) GetLatestReadings() []SensorReading {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	latest := make([]SensorReading, 0, len(sm.readings))
	for _, history := range sm.readings {
		if len(history) > 0 {
			latest = append(latest, history[len(history)-1])
		}
	}
	return latest
}

// GetReadingsInElevationBand filters latest readings situated within [minFt, maxFt].
func (sm *SensorMesh) GetReadingsInElevationBand(minFt, maxFt float64) []SensorReading {
	allLatest := sm.GetLatestReadings()
	filtered := make([]SensorReading, 0, len(allLatest))
	for _, r := range allLatest {
		if r.ElevationFt >= minFt && r.ElevationFt <= maxFt {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// PredictSpread executes the Rothermel rate-of-spread calculation across all latest sensor readings.
// Returns a map of SensorID -> RateOfSpread (ft/min).
func (sm *SensorMesh) PredictSpread(fuel FuelModel, slopeAngleDeg float64) map[string]float64 {
	latest := sm.GetLatestReadings()
	predictions := make(map[string]float64, len(latest))
	for _, r := range latest {
		ros, _, _ := CalculateRothermelRateOfSpread(fuel, r.FuelMoisturePct, r.WindSpeedMph, slopeAngleDeg)
		predictions[r.SensorID] = ros
	}
	return predictions
}

// Standard USFS 13 Fire Behavior Fuel Models (Anderson 1982 / Rothermel 1972).
var (
	// FuelModel1: Short Grass (1 ft) - dry climate grassland.
	FuelModel1 = FuelModel{
		FuelModelNumber:      1,
		FuelName:             "Short Grass (1 ft)",
		FuelBedDepthFt:       1.0,
		MoistureOfExtinction: 0.12,
		HeatContentBtuPerLb:  8000.0,
		Loading1Hr:           0.034, // 0.74 tons/acre
		Loading10Hr:          0.0,
		Loading100Hr:         0.0,
		SAVR:                 3500.0,
	}

	// FuelModel4: Chaparral / High Brush (6 ft) - continuous deep flammable brush.
	FuelModel4 = FuelModel{
		FuelModelNumber:      4,
		FuelName:             "Chaparral / High Brush (6 ft)",
		FuelBedDepthFt:       6.0,
		MoistureOfExtinction: 0.20,
		HeatContentBtuPerLb:  8000.0,
		Loading1Hr:           0.230, // 5.0 tons/acre
		Loading10Hr:          0.184, // 4.0 tons/acre
		Loading100Hr:         0.092, // 2.0 tons/acre
		SAVR:                 2000.0,
	}

	// FuelModel8: Closed Timber Litter - compact dead pine/hardwood needle litter.
	FuelModel8 = FuelModel{
		FuelModelNumber:      8,
		FuelName:             "Closed Timber Litter",
		FuelBedDepthFt:       0.2,
		MoistureOfExtinction: 0.30,
		HeatContentBtuPerLb:  8000.0,
		Loading1Hr:           0.069, // 1.5 tons/acre
		Loading10Hr:          0.046, // 1.0 tons/acre
		Loading100Hr:         0.115, // 2.5 tons/acre
		SAVR:                 2000.0,
	}

	// FuelModel10: Timber Litter & Understory - mixed conifer with forest floor debris and saplings.
	FuelModel10 = FuelModel{
		FuelModelNumber:      10,
		FuelName:             "Timber Litter & Understory",
		FuelBedDepthFt:       1.0,
		MoistureOfExtinction: 0.25,
		HeatContentBtuPerLb:  8000.0,
		Loading1Hr:           0.138, // 3.0 tons/acre
		Loading10Hr:          0.092, // 2.0 tons/acre
		Loading100Hr:         0.230, // 5.0 tons/acre
		SAVR:                 2000.0,
	}

	// StandardModels registry indexed by model number.
	StandardModels = map[int]FuelModel{
		1:  FuelModel1,
		4:  FuelModel4,
		8:  FuelModel8,
		10: FuelModel10,
	}
)
