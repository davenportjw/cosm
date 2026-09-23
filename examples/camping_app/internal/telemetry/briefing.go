package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// RangerBriefing encapsulates an operational telemetry and tactical fire behavior briefing.
type RangerBriefing struct {
	GeneratedAt time.Time `json:"generated_at"`
	ModelUsed string `json:"model_used"`
	SummaryMarkdown string `json:"summary_markdown"`
	EvacuationAdvice string `json:"evacuation_advice"`
	FireSpreadPredictions map[string]float64 `json:"fire_spread_predictions"`
}

// GenerateRangerBriefing computes Rothermel fire spread predictions across active sensor readings
// and synthesizes an operational briefing using Vertex AI Gemini 3.8 Flash (if ADC is accessible)
// or a deterministic local algorithmic synthesis engine.
func GenerateRangerBriefing(readings []SensorReading, activeIncidentsCount int, arrivingVehiclesCount int) (*RangerBriefing, error) {
	briefing := &RangerBriefing{
		GeneratedAt:           time.Now(),
		FireSpreadPredictions: make(map[string]float64),
	}

	if len(readings) == 0 {
		briefing.ModelUsed = "deterministic-algorithmic-v1"
		briefing.SummaryMarkdown = "### Backcountry Ranger Briefing\n\n*No active sensor telemetry available across the mesh.*"
		briefing.EvacuationAdvice = "Maintain routine situational awareness. No immediate hazard detected."
		return briefing, nil
	}

	// 1. Calculate Rothermel rate of spread for each reading using Timber Litter & Understory (Model 10)
	// assuming a moderate 15-degree canyon slope.
	defaultFuel := FuelModel10
	const defaultSlopeDeg = 15.0

	var maxROS float64
	var maxROSSensor string
	var maxFlameLength float64
	var maxIntensity float64
	var sumTemp, sumHumidity, sumWind, sumMoisture float64

	for _, r := range readings {
		ros, fl, intens := CalculateRothermelRateOfSpread(defaultFuel, r.FuelMoisturePct, r.WindSpeedMph, defaultSlopeDeg)
		briefing.FireSpreadPredictions[r.SensorID] = ros
		if ros > maxROS {
			maxROS = ros
			maxROSSensor = r.SensorID
			maxFlameLength = fl
			maxIntensity = intens
		}
		sumTemp += r.TemperatureF
		sumHumidity += r.HumidityPct
		sumWind += r.WindSpeedMph
		sumMoisture += r.FuelMoisturePct
	}

	n := float64(len(readings))
	avgTemp := sumTemp / n
	avgHumidity := sumHumidity / n
	avgWind := sumWind / n
	avgMoisture := sumMoisture / n

	// 2. Attempt Vertex AI Gemini 3.8 Flash synthesis via Application Default Credentials (ADC)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	adcToken, tokenErr := getADCAuthToken(ctx)
	if tokenErr == nil && adcToken != "" {
		geminiBriefing, err := callVertexAIGeminiBriefing(ctx, adcToken, readings, activeIncidentsCount, arrivingVehiclesCount, briefing.FireSpreadPredictions, maxROS, maxROSSensor)
		if err == nil && geminiBriefing != nil {
			geminiBriefing.GeneratedAt = briefing.GeneratedAt
			geminiBriefing.FireSpreadPredictions = briefing.FireSpreadPredictions
			return geminiBriefing, nil
		}
	}

	// 3. Deterministic Local Algorithmic Synthesis
	briefing.ModelUsed = "deterministic-algorithmic-v1"

	var advice string
	var dangerLevel string

	switch {
	case maxROS >= 30.0 || activeIncidentsCount >= 3:
		dangerLevel = "CRITICAL / RED FLAG"
		advice = fmt.Sprintf("STAGE 3 MANDATORY EVACUATION: Immediate evacuation of campground sectors near sensor %s. Halt all %d inbound vehicles at the gatehouse. Dispatch strike teams to active incidents (%d reported).",
			maxROSSensor, arrivingVehiclesCount, activeIncidentsCount)
	case maxROS >= 15.0 || activeIncidentsCount > 0:
		dangerLevel = "ELEVATED FIRE DANGER"
		advice = fmt.Sprintf("STAGE 2 EVACUATION WARNING: Issue pre-evacuation notices for canyon campsites. Enforce strict Stage 2 fire restrictions (no open flames). Brief %d arriving vehicle parties on emergency egress routes.",
			arrivingVehiclesCount)
	case maxROS >= 5.0 || avgMoisture < 10.0:
		dangerLevel = "MODERATE FIRE ADVISORY"
		advice = "STAGE 1 FIRE RESTRICTIONS: Campfires restricted to designated metal fire rings. Alert rangers to monitor high-elevation ridgelines."
	default:
		dangerLevel = "NORMAL BACKCOUNTRY CONDITIONS"
		advice = "ROUTINE PATROL: All conditions within seasonal norms. Distribute Leave-No-Trace and fire safety pamphlets to incoming campers."
	}

	briefing.EvacuationAdvice = advice

	// Format structured markdown summary
	var sb strings.Builder
	sb.WriteString("# Automated Backcountry Tactical Fire Briefing\n\n")
	sb.WriteString(fmt.Sprintf("**Operational Status:** `%s`  \n", dangerLevel))
	sb.WriteString(fmt.Sprintf("**Active Incidents:** %d | **Inbound Gate Queue:** %d vehicle(s)  \n", activeIncidentsCount, arrivingVehiclesCount))
	sb.WriteString(fmt.Sprintf("**Telemetry Coverage:** %d sensor node(s) across backcountry mesh  \n\n", len(readings)))

	sb.WriteString("## Environmental Aggregates\n")
	sb.WriteString(fmt.Sprintf("- **Mean Temperature:** %.1f°F\n", avgTemp))
	sb.WriteString(fmt.Sprintf("- **Mean Relative Humidity:** %.1f%%\n", avgHumidity))
	sb.WriteString(fmt.Sprintf("- **Mean Wind Velocity:** %.1f mph\n", avgWind))
	sb.WriteString(fmt.Sprintf("- **Mean 10-Hr Fuel Moisture:** %.1f%%\n\n", avgMoisture))

	sb.WriteString("## Rothermel Fire Spread Modeling (Model 10 - Timber Litter)\n")
	sb.WriteString(fmt.Sprintf("- **Peak Spread Rate:** **%.2f ft/min** (Sensor `%s`)\n", maxROS, maxROSSensor))
	sb.WriteString(fmt.Sprintf("- **Projected Flame Length:** %.2f ft\n", maxFlameLength))
	sb.WriteString(fmt.Sprintf("- **Fireline Intensity:** %.2f Btu/ft-s\n\n", maxIntensity))

	sb.WriteString("### Sensor Frontier Details\n")
	sortedSensors := make([]string, 0, len(briefing.FireSpreadPredictions))
	for sid := range briefing.FireSpreadPredictions {
		sortedSensors = append(sortedSensors, sid)
	}
	sort.Strings(sortedSensors)
	for _, sid := range sortedSensors {
		sb.WriteString(fmt.Sprintf("- `%s`: Rate of Spread = **%.2f ft/min**\n", sid, briefing.FireSpreadPredictions[sid]))
	}

	sb.WriteString("\n## Tactical Directives\n")
	sb.WriteString(fmt.Sprintf("> [!IMPORTANT]\n> %s\n", advice))

	briefing.SummaryMarkdown = sb.String()
	return briefing, nil
}

// callVertexAIGeminiBriefing dispatches a generation request to Vertex AI Gemini 3.8 Flash.
func callVertexAIGeminiBriefing(
	ctx context.Context,
	token string,
	readings []SensorReading,
	incidents int,
	inboundVehicles int,
	predictions map[string]float64,
	maxROS float64,
	maxSensor string,
) (*RangerBriefing, error) {
	projectID := resolveGCPProject()
	location := "us-central1"
	model := "gemini-3.8-flash"

	endpoint := fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:generateContent",
		location, projectID, location, model,
	)

	prompt := fmt.Sprintf(`You are Ranger-Ops Tactical AI for backcountry wildland fire and gatehouse operations.
Synthesize an executive operational briefing using this real-time telemetry:
- Mesh Sensor Count: %d
- Active Reported Fire/Medical Incidents: %d
- Inbound Gatehouse Vehicles Pending Check-In: %d
- Highest Rothermel Rate of Spread: %.2f ft/min at Sensor %s
- Spread Predictions (Sensor -> ft/min): %v

Provide a concise, high-impact tactical markdown report with:
1. Operational Risk Assessment
2. Backcountry Sensor Mesh Status
3. Exact Evacuation and Gatehouse Directives

At the very end of your response, output a single line with:
EVACUATION_ADVICE: <one-line definitive command for gatehouse and backcountry evacuation>`,
		len(readings), incidents, inboundVehicles, maxROS, maxSensor, predictions,
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
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 7 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vertex ai error HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, err
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty gemini candidate response")
	}

	fullText := geminiResp.Candidates[0].Content.Parts[0].Text
	evacAdvice := "Maintain situational awareness."
	summaryMarkdown := fullText

	if idx := strings.Index(fullText, "EVACUATION_ADVICE:"); idx != -1 {
		evacAdvice = strings.TrimSpace(fullText[idx+len("EVACUATION_ADVICE:"):])
		summaryMarkdown = strings.TrimSpace(fullText[:idx])
	}

	return &RangerBriefing{
		ModelUsed:        "gemini-3.8-flash",
		SummaryMarkdown:  summaryMarkdown,
		EvacuationAdvice: evacAdvice,
	}, nil
}

func getADCAuthToken(ctx context.Context) (string, error) {
	// 1. Env var GOOGLE_OAUTH_ACCESS_TOKEN
	if envTok := os.Getenv("GOOGLE_OAUTH_ACCESS_TOKEN"); envTok != "" {
		return envTok, nil
	}

	// 2. Cloud metadata server (Cloud Run / GCE)
	metaReq, _ := http.NewRequestWithContext(ctx, "GET", "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token", nil)
	metaReq.Header.Set("Metadata-Flavor", "Google")
	metaClient := &http.Client{Timeout: 600 * time.Millisecond}
	if resp, err := metaClient.Do(metaReq); err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		var metaToken struct {
			AccessToken string `json:"access_token"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&metaToken); err == nil && metaToken.AccessToken != "" {
			return metaToken.AccessToken, nil
		}
	}

	// 3. Application Default Credentials via gcloud CLI
	if out, err := exec.CommandContext(ctx, "gcloud", "auth", "application-default", "print-access-token").Output(); err == nil && len(out) > 0 {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if strings.HasPrefix(line, "ya29.") {
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
				return line, nil
			}
		}
	}

	return "", fmt.Errorf("no valid google ADC access token available")
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
	return "davenport-boutique"
}

