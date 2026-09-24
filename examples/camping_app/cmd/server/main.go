package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/cosmscm/cosm/examples/camping_app/internal/backcountry"
	"github.com/cosmscm/cosm/examples/camping_app/internal/environmental"
	"github.com/cosmscm/cosm/examples/camping_app/internal/incident"
	"github.com/cosmscm/cosm/examples/camping_app/internal/lease"
	"github.com/cosmscm/cosm/examples/camping_app/internal/outfitter"
	"github.com/cosmscm/cosm/examples/camping_app/internal/pms"
	"github.com/cosmscm/cosm/examples/camping_app/internal/profile"
	"github.com/cosmscm/cosm/examples/camping_app/internal/ranger"
	"github.com/cosmscm/cosm/examples/camping_app/internal/sos"
	"github.com/cosmscm/cosm/examples/camping_app/internal/telemetry"
	"github.com/cosmscm/cosm/examples/camping_app/internal/weather"
	"golang.org/x/crypto/bcrypt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "github.com/jackc/pgx/v5/stdlib"
	cSync "github.com/cosmscm/cosm/examples/camping_app/internal/sync"
)

// Data Models
type User struct {
	ID string
	Email string
	PasswordHash string
	FullName string
	CreatedAt time.Time
}

type Campsite struct {
	ID string
	Name string
	Slug string
	Description string
	Location string
	Latitude float64
	Longitude float64
	CampsiteType string
	DailyRateCents int
	Capacity int
	Amenities []string
	ImageURL string
}

type Reservation struct {
	ID string
	CampsiteID string
	UserID string
	StartDate time.Time
	EndDate time.Time
	GuestsCount int
	TotalCents int
	Status string
	CreatedAt time.Time
	CampsiteName string
	UserName string
}

// MemoryStore provides fallback in-memory concurrency protection when PostgreSQL is not configured.
type MemoryStore struct {
	mu sync.RWMutex
	users map[string]User
	usersByID map[string]User
	sessions map[string]string
	campsites map[string]Campsite
	reservations map[string]Reservation
}

// App Server
type Server struct {
	db *sql.DB
	mem *MemoryStore
	tmpl *template.Template
	isPgSQL bool
	leaseEngine *lease.Engine
	envSvc *environmental.Service
	rangerPMS *ranger.PMSManager
	rangerHub *ranger.EventHub
	profileMgr *profile.ProfileManager
	gateCtrl *ranger.GateController
	icsMgr *incident.ICSManager
	pmsAssetMgr *pms.PMSAssetManager
	backcountryEng *backcountry.BackcountryEngine
	lockerMgr *outfitter.LockerManager
	offlineCache *cSync.OfflineGateCache
	sensorMesh *telemetry.SensorMesh
	weatherEng *weather.WeatherEngine
	sosMgr *sos.SOSManager
}

func NewMemoryStore() *MemoryStore {
	m := &MemoryStore{
		users:        make(map[string]User),
		usersByID:    make(map[string]User),
		sessions:     make(map[string]string),
		campsites:    make(map[string]Campsite),
		reservations: make(map[string]Reservation),
	}
	m.seedCatalog()
	return m
}

func (m *MemoryStore) seedCatalog() {
	sites := []Campsite{
		{ID: "c1", Name: "Olympic Rainforest Glade", Slug: "olympic-rainforest-glade", Description: "Towering Sitka spruces and lush moss trails with direct river access.", Location: "Hoh Rainforest, WA", Latitude: 47.8600, Longitude: -123.9348, CampsiteType: "tent", DailyRateCents: 3500, Capacity: 4, Amenities: []string{"Fire Pit", "Drinking Water", "Vault Toilet", "Pet Friendly"}, ImageURL: "https://images.unsplash.com/photo-1504280390367-361c6d9f38f4?auto=format&fit=crop&w=800&q=80"},
		{ID: "c2", Name: "Mount Rainier Vista Cabin", Slug: "mount-rainier-vista-cabin", Description: "Rustic handcrafted cedar cabin with direct panoramic views of the glacier peaks.", Location: "Ashford, WA", Latitude: 46.8523, Longitude: -121.7603, CampsiteType: "cabin", DailyRateCents: 14500, Capacity: 6, Amenities: []string{"Wood Stove", "Electricity", "Kitchenette", "Hot Shower", "WiFi"}, ImageURL: "https://images.unsplash.com/photo-1510312305653-8ed496efae75?auto=format&fit=crop&w=800&q=80"},
		{ID: "c3", Name: "Crater Lake Meadow Ridge", Slug: "crater-lake-meadow-ridge", Description: "Alpine meadow tent pitch overlooking deep volcanic calderas and starry night skies.", Location: "Klamath County, OR", Latitude: 42.9446, Longitude: -122.1090, CampsiteType: "tent", DailyRateCents: 4000, Capacity: 4, Amenities: []string{"Picnic Table", "Bear Locker", "Drinking Water", "Stargazing Platform"}, ImageURL: "https://images.unsplash.com/photo-1478131143081-80f7f84ca84d?auto=format&fit=crop&w=800&q=80"},
		{ID: "c4", Name: "Cascades Alpine RV Haven", Slug: "cascades-alpine-rv-haven", Description: "Full electrical hookup pad nestled among fragrant ponderosa pines.", Location: "Leavenworth, WA", Latitude: 47.5962, Longitude: -120.6615, CampsiteType: "rv", DailyRateCents: 6500, Capacity: 8, Amenities: []string{"50-Amp Electric", "Water Hookup", "Sewer Dump", "Hot Showers", "WiFi"}, ImageURL: "https://images.unsplash.com/photo-1523987355523-c7b5b0dd90a7?auto=format&fit=crop&w=800&q=80"},
		{ID: "c5", Name: "Hood River Glamping Dome", Slug: "hood-river-glamping-dome", Description: "Geodesic luxury dome featuring queen bed, solar climate control, and vineyard sunsets.", Location: "Hood River, OR", Latitude: 45.7054, Longitude: -121.5215, CampsiteType: "glamping", DailyRateCents: 18500, Capacity: 2, Amenities: []string{"King Bed", "Heating & AC", "Ensuite Bathroom", "Private Deck", "Sauna"}, ImageURL: "https://images.unsplash.com/photo-1496545672447-f699b503d270?auto=format&fit=crop&w=800&q=80"},
		{ID: "c6", Name: "Sawtooth Wilderness Basin", Slug: "sawtooth-wilderness-basin", Description: "Remote backcountry campsite adjacent to crystal clear trout streams and granite peaks.", Location: "Stanley, ID", Latitude: 44.1800, Longitude: -114.9300, CampsiteType: "tent", DailyRateCents: 3000, Capacity: 4, Amenities: []string{"Fire Ring", "Bear Box", "Stream Water", "Pack-In Pack-Out"}, ImageURL: "https://images.unsplash.com/photo-1517824806704-9040b037703b?auto=format&fit=crop&w=800&q=80"},
		{ID: "c7", Name: "San Juan Island Waterfront", Slug: "san-juan-island-waterfront", Description: "Walk-in marine campsite where orca pods breach offshore at sunrise.", Location: "Friday Harbor, WA", Latitude: 48.5343, Longitude: -123.0189, CampsiteType: "tent", DailyRateCents: 4500, Capacity: 5, Amenities: []string{"Kayak Launch", "Campfire Ring", "Potable Water", "Compost Toilets"}, ImageURL: "https://images.unsplash.com/photo-1537905569824-f89f14cceb68?auto=format&fit=crop&w=800&q=80"},
		{ID: "c8", Name: "Glacier Peak Outlook Cabin", Slug: "glacier-peak-outlook-cabin", Description: "Restored historic fire lookout tower with 360-degree views of the Cascade mountain range.", Location: "Darrington, WA", Latitude: 48.1125, Longitude: -121.1138, CampsiteType: "cabin", DailyRateCents: 16000, Capacity: 4, Amenities: []string{"Propane Heater", "Solar Lights", "360 View Deck", "Bunk Beds"}, ImageURL: "https://images.unsplash.com/photo-1542601906990-b4d3fb778b09?auto=format&fit=crop&w=800&q=80"},
	}
	for _, s := range sites {
		m.campsites[s.ID] = s
	}

	// Create default persona Alice for zero-friction exploration
	pwd, _ := bcrypt.GenerateFromPassword([]byte("camping123"), bcrypt.DefaultCost)
	alice := User{
		ID:           "u-alice",
		Email:        "alice@camping.local",
		PasswordHash: string(pwd),
		FullName:     "Alice Explorer",
		CreatedAt:    time.Now().UTC(),
	}
	m.users[alice.Email] = alice
	m.usersByID[alice.ID] = alice
	m.sessions["session-alice-demo"] = alice.ID
}

func NewServer(databaseURL string) *Server {
	hub := ranger.NewEventHub()
	pmsMgr := ranger.NewPMSManager(hub)
	engine := lease.NewEngine()
	env := environmental.NewService()

	// Track 1: Profile & Multi-lane ALPR Gatehouse
	profMgr := profile.NewProfileManager()
	_ = profMgr.UpsertProfile(&profile.CamperProfile{
		UserID:                "u-alice",
		FullName:              "Alice Explorer",
		Phone:                 "+1-555-0199",
		EmergencyContactName:  "Bob Explorer",
		EmergencyContactPhone: "+1-555-0198",
		WildernessPassID:      "PASS-NORTH-2026",
		NotificationsEnabled:  true,
		CreatedAt:             time.Now().UTC(),
		Vehicles: []profile.VehicleRecord{
			{
				Plate:     "WA-ALPINE1",
				State:     "WA",
				MakeModel: "Subaru Outback",
				Color:     "Blue",
				IsPrimary: true,
				AddedAt:   time.Now().UTC(),
			},
			{
				Plate:     "WA-TRL-44",
				State:     "WA",
				MakeModel: "Airstream Bambi",
				Color:     "Silver",
				IsPrimary: false,
				AddedAt:   time.Now().UTC(),
			},
		},
	})

	gateController := pmsMgr.GateController
	pmsMgr.RegisterVehicleExtended(
		"WA-ALPINE1",
		"WA",
		"c1",
		"Alice Explorer",
		"Subaru Outback Blue",
		"WA-TRL-44",
		"b-seed-1",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(72*time.Hour),
	)
	pmsMgr.RegisterVehicleExtended(
		"WA-TRL-44",
		"WA",
		"c1",
		"Alice Explorer (Trailer)",
		"Airstream Bambi",
		"",
		"b-seed-1",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(72*time.Hour),
	)

	// Track 2: Wild-PMS & Incident Command System
	ics := incident.NewICSManager()
	_, _ = ics.ReportIncident(
		"BEAR_SIGHTING",
		"ADVISORY",
		"CAMP-PACIFIC-01",
		"Upper Loop Sector B",
		"46.8523,-121.7603",
		"Adult black bear observed investigating bear locker near site B-14.",
	)

	assetMgr := pms.NewPMSAssetManager()
	_ = assetMgr.RegisterAsset(&pms.CampsiteAsset{
		ID:              "ASSET-BB-014",
		CampsiteID:      "c1",
		CampgroundID:    "CAMP-PACIFIC-01",
		AssetClass:      "BEAR_BOX",
		SerialNumber:    "BB-2024-0014",
		ConditionRating: "FAIR",
		Notes:           "Hinge latch slightly loose, scheduled for tightening",
		LastInspected:   time.Now().Add(-48 * time.Hour),
	})
	_ = assetMgr.CheckInOccupant(&pms.OccupancyRecord{
		CampsiteID:        "c1",
		ReservationID:     "res-alice-01",
		GuestName:         "Alice Explorer",
		PartySize:         2,
		PrimaryPlate:      "WA-ALPINE1",
		TrailerPlate:      "WA-TRL-44",
		EmergencyPhone:    "+1-555-0198",
		CheckedInAt:       time.Now().Add(-2 * time.Hour),
		ScheduledCheckOut: time.Now().Add(48 * time.Hour),
	})

	// Track 3 & 4: Backcountry Quotas & Outfitter Lockers
	bcEngine := backcountry.NewBackcountryEngine()
	_ = bcEngine.RegisterZone(&backcountry.TrailheadZone{
		ID:                   "zone-enchantments",
		Name:                 "The Enchantments Core",
		ElevationFt:          7800,
		DailyQuota:           16,
		BearCanisterRequired: true,
		DifficultyRating:     "ALPINE_TECHNICAL",
	})
	_ = bcEngine.RegisterZone(&backcountry.TrailheadZone{
		ID:                   "zone-wonderland",
		Name:                 "Wonderland Trail High Pass",
		ElevationFt:          6400,
		DailyQuota:           24,
		BearCanisterRequired: true,
		DifficultyRating:     "STRENUOUS",
	})
	_ = bcEngine.RegisterZone(&backcountry.TrailheadZone{
		ID:                   "zone-sawtooth",
		Name:                 "Sawtooth Alice-Toxaway Loop",
		ElevationFt:          8400,
		DailyQuota:           30,
		BearCanisterRequired: false,
		DifficultyRating:     "STRENUOUS",
	})

	lockerMgr := outfitter.NewLockerManager()
	lockerMgr.SeedDefaultGear()

	// Track 5 & 6: Rothermel Wildfire Physics & Offline Gate Sync
	sMesh := telemetry.NewSensorMesh()
	sMesh.AddReading(telemetry.SensorReading{
		SensorID:        "SN-ELEV-5200",
		ElevationFt:     5200,
		TemperatureF:    74.5,
		HumidityPct:     22,
		WindSpeedMph:    11.0,
		FuelMoisturePct: 12.0,
		Timestamp:       time.Now(),
	})
	sMesh.AddReading(telemetry.SensorReading{
		SensorID:        "SN-ELEV-6800",
		ElevationFt:     6800,
		TemperatureF:    68.2,
		HumidityPct:     18,
		WindSpeedMph:    18.5,
		FuelMoisturePct: 9.0,
		Timestamp:       time.Now(),
	})
	sMesh.AddReading(telemetry.SensorReading{
		SensorID:        "SN-ELEV-8500",
		ElevationFt:     8500,
		TemperatureF:    58.0,
		HumidityPct:     14,
		WindSpeedMph:    24.0,
		FuelMoisturePct: 7.0,
		Timestamp:       time.Now(),
	})

	offCache := cSync.NewOfflineGateCache()
	offCache.LoadPermits(map[string]string{
		"WA-ALPINE1:WA": "Alice Explorer - Site C1 (Confirmed)",
		"WA-TRL-44:WA":  "Alice Explorer - Trailer Permit (Confirmed)",
	})

	s := &Server{
		mem:            NewMemoryStore(),
		leaseEngine:    engine,
		envSvc:         env,
		rangerHub:      hub,
		rangerPMS:      pmsMgr,
		profileMgr:     profMgr,
		gateCtrl:       gateController,
		icsMgr:         ics,
		pmsAssetMgr:    assetMgr,
		backcountryEng: bcEngine,
		lockerMgr:      lockerMgr,
		offlineCache:   offCache,
		sensorMesh:     sMesh,
		weatherEng:     weather.NewWeatherEngine(),
		sosMgr:         sos.NewSOSManager(),
	}

	if databaseURL != "" {
		db, err := sql.Open("pgx", databaseURL)
		if err == nil && db.Ping() == nil {
			s.db = db
			s.isPgSQL = true
			log.Println("Connected to PostgreSQL database with GiST exclusion constraints")
			s.initSchema()
		} else {
			log.Printf("PostgreSQL connection unavailable (%v), using resilient in-memory isolation engine\n", err)
		}
	} else {
		log.Println("DATABASE_URL not set; using resilient in-memory isolation engine")
	}

	s.parseTemplates()
	return s
}

func (s *Server) initSchema() {
	migrationPath := "migrations/001_campsite_schema.sql"
	if data, err := os.ReadFile(migrationPath); err == nil {
		_, execErr := s.db.Exec(string(data))
		if execErr != nil {
			log.Printf("Notice during migration execution: %v\n", execErr)
		} else {
			log.Println("PostgreSQL 001_campsite_schema.sql verified and applied")
		}
	}
}

// Auth Helpers
func (s *Server) authenticateUser(w http.ResponseWriter, r *http.Request) *User {
	cookie, err := r.Cookie("cosm_session")
	if err != nil || cookie.Value == "" {
		return nil
	}

	token := cookie.Value
	if s.isPgSQL {
		var u User
		var expires time.Time
		q := `SELECT u.id, u.email, u.full_name, s.expires_at 
		      FROM sessions s JOIN users u ON s.user_id = u.id 
		      WHERE s.token = $1`
		err := s.db.QueryRowContext(r.Context(), q, token).Scan(&u.ID, &u.Email, &u.FullName, &expires)
		if err == nil && expires.After(time.Now()) {
			return &u
		}
		return nil
	}

	s.mem.mu.RLock()
	defer s.mem.mu.RUnlock()
	if userID, ok := s.mem.sessions[token]; ok {
		if u, exists := s.mem.usersByID[userID]; exists {
			return &u
		}
	}
	return nil
}

func (s *Server) getAuthenticatedUser(r *http.Request) *User {
	return s.authenticateUser(nil, r)
}

func templateEscape(str string) string {
	return template.HTMLEscapeString(str)
}

func generateToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "cosm_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 7,
	})
}

// Handlers
func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	dbStatus := "memory"
	if s.isPgSQL {
		dbStatus = "postgresql-gist"
	}
	fmt.Fprintf(w, "{\"status\":\"HEALTHY\",\"service\":\"cosm-camping-app\",\"db\":\"%s\",\"ast_version\":\"0.3.0\",\"telemetry\":\"active\"}", dbStatus)
}

// J1: Discovery & Filtering Handler
func (s *Server) HandleCampsiteList(w http.ResponseWriter, r *http.Request) {
	user := s.authenticateUser(w, r)
	cType := r.URL.Query().Get("type")
	guests := r.URL.Query().Get("guests")
	query := strings.ToLower(r.URL.Query().Get("q"))

	var sites []Campsite
	if s.isPgSQL {
		rows, err := s.db.QueryContext(r.Context(), "SELECT id, name, slug, description, location, campsite_type, daily_rate_cents, capacity, amenities, image_url FROM campsites")
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var cs Campsite
				var am []string
				_ = rows.Scan(&cs.ID, &cs.Name, &cs.Slug, &cs.Description, &cs.Location, &cs.CampsiteType, &cs.DailyRateCents, &cs.Capacity, &am, &cs.ImageURL)
				cs.Amenities = am
				sites = append(sites, cs)
			}
		}
	}

	if len(sites) == 0 {
		s.mem.mu.RLock()
		for _, cs := range s.mem.campsites {
			sites = append(sites, cs)
		}
		s.mem.mu.RUnlock()
	}

	// Filter
	var filtered []Campsite
	for _, cs := range sites {
		if cType != "" && cType != "all" && cs.CampsiteType != cType {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(cs.Name), query) && !strings.Contains(strings.ToLower(cs.Location), query) {
			continue
		}
		filtered = append(filtered, cs)
	}

	data := map[string]interface{}{
		"User":      user,
		"Campsites": filtered,
		"Type":      cType,
		"Query":     query,
		"Guests":    guests,
	}

	if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Target") == "campsite-stream" {
		if err := s.tmpl.ExecuteTemplate(w, "campsite-cards", data); err != nil {
			http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
		}
		return
	}

	if err := s.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
	}
}

// J1: Availability Calendar Drawer
func (s *Server) HandleAvailability(w http.ResponseWriter, r *http.Request) {
	campsiteID := r.URL.Query().Get("id")
	if campsiteID == "" {
		http.Error(w, "Missing campsite ID", http.StatusBadRequest)
		return
	}

	var site Campsite
	var found bool
	if s.isPgSQL {
		row := s.db.QueryRowContext(r.Context(), "SELECT id, name, slug, description, location, campsite_type, daily_rate_cents, capacity, image_url FROM campsites WHERE id = $1", campsiteID)
		_ = row.Scan(&site.ID, &site.Name, &site.Slug, &site.Description, &site.Location, &site.CampsiteType, &site.DailyRateCents, &site.Capacity, &site.ImageURL)
		found = site.ID != ""
	}
	if !found {
		s.mem.mu.RLock()
		site, found = s.mem.campsites[campsiteID]
		s.mem.mu.RUnlock()
	}

	// Calculate booked days for current and next 30 days
	today := time.Now().Truncate(24 * time.Hour)
	bookedMap := make(map[string]bool)

	if s.isPgSQL {
		rows, err := s.db.QueryContext(r.Context(), "SELECT start_date, end_date FROM reservations WHERE campsite_id = $1 AND status = 'confirmed'", campsiteID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var start, end time.Time
				_ = rows.Scan(&start, &end)
				for d := start; d.Before(end); d = d.Add(24 * time.Hour) {
					bookedMap[d.Format("2006-01-02")] = true
				}
			}
		}
	} else {
		s.mem.mu.RLock()
		for _, res := range s.mem.reservations {
			if res.CampsiteID == campsiteID && res.Status == "confirmed" {
				for d := res.StartDate; d.Before(res.EndDate); d = d.Add(24 * time.Hour) {
					bookedMap[d.Format("2006-01-02")] = true
				}
			}
		}
		s.mem.mu.RUnlock()
	}

	type CalendarDay struct {
		DateStr   string
		DayNum    int
		IsBooked  bool
		IsPast    bool
		IsToday   bool
	}

	var days []CalendarDay
	for i := 0; i < 28; i++ {
		d := today.Add(time.Duration(i) * 24 * time.Hour)
		dStr := d.Format("2006-01-02")
		days = append(days, CalendarDay{
			DateStr:  dStr,
			DayNum:   d.Day(),
			IsBooked: bookedMap[dStr],
			IsPast:   false,
			IsToday:  i == 0,
		})
	}

	data := map[string]interface{}{
		"Campsite": site,
		"Days":     days,
		"Today":    today.Format("2006-01-02"),
		"Tomorrow": today.Add(24 * time.Hour).Format("2006-01-02"),
	}

	s.tmpl.ExecuteTemplate(w, "calendar-drawer", data)
}

// J2: User Authentication Handlers
func (s *Server) HandleSignup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	fullName := strings.TrimSpace(r.FormValue("full_name"))
	password := r.FormValue("password")

	if email == "" || password == "" || fullName == "" {
		http.Error(w, "All fields are required", http.StatusBadRequest)
		return
	}

	pwdHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Encryption failure", http.StatusInternalServerError)
		return
	}

	token := generateToken()
	userID := fmt.Sprintf("u-%s", generateToken()[:8])

	if s.isPgSQL {
		var id string
		insUser := `INSERT INTO users (email, password_hash, full_name) VALUES ($1, $2, $3) RETURNING id`
		err = s.db.QueryRowContext(r.Context(), insUser, email, string(pwdHash), fullName).Scan(&id)
		if err != nil {
			w.WriteHeader(http.StatusConflict)
			fmt.Fprintf(w, `<div class="p-3 bg-red-100 text-red-700 text-sm rounded">Email already registered. Please login.</div>`)
			return
		}
		userID = id
		insSess := `INSERT INTO sessions (token, user_id, expires_at) VALUES ($1, $2, $3)`
		_, _ = s.db.ExecContext(r.Context(), insSess, token, userID, time.Now().Add(7*24*time.Hour))
	} else {
		s.mem.mu.Lock()
		if _, exists := s.mem.users[email]; exists {
			s.mem.mu.Unlock()
			w.WriteHeader(http.StatusConflict)
			fmt.Fprintf(w, `<div class="p-3 bg-red-100 text-red-700 text-sm rounded">Email already registered. Please login.</div>`)
			return
		}
		u := User{ID: userID, Email: email, PasswordHash: string(pwdHash), FullName: fullName, CreatedAt: time.Now()}
		s.mem.users[email] = u
		s.mem.usersByID[userID] = u
		s.mem.sessions[token] = userID
		s.mem.mu.Unlock()
	}

	s.setSessionCookie(w, token)
	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	password := r.FormValue("password")

	var u User
	if s.isPgSQL {
		q := `SELECT id, email, password_hash, full_name FROM users WHERE email = $1`
		err := s.db.QueryRowContext(r.Context(), q, email).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.FullName)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `<div class="p-3 bg-red-100 text-red-700 text-sm rounded">Invalid email or password.</div>`)
			return
		}
	} else {
		s.mem.mu.RLock()
		user, exists := s.mem.users[email]
		s.mem.mu.RUnlock()
		if !exists {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `<div class="p-3 bg-red-100 text-red-700 text-sm rounded">Invalid email or password.</div>`)
			return
		}
		u = user
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `<div class="p-3 bg-red-100 text-red-700 text-sm rounded">Invalid email or password.</div>`)
		return
	}

	token := generateToken()
	if s.isPgSQL {
		insSess := `INSERT INTO sessions (token, user_id, expires_at) VALUES ($1, $2, $3)`
		_, _ = s.db.ExecContext(r.Context(), insSess, token, u.ID, time.Now().Add(7*24*time.Hour))
	} else {
		s.mem.mu.Lock()
		s.mem.sessions[token] = u.ID
		s.mem.mu.Unlock()
	}

	s.setSessionCookie(w, token)
	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) HandleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "cosm_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Persona Switcher for Live Demo & Testing
func (s *Server) HandleSwitchPersona(w http.ResponseWriter, r *http.Request) {
	persona := r.URL.Query().Get("user")
	var email, name string
	switch persona {
	case "bob":
		email = "bob@camping.local"
		name = "Bob Camper"
	case "charlie":
		email = "charlie@camping.local"
		name = "Charlie Hiker"
	default:
		email = "alice@camping.local"
		name = "Alice Explorer"
	}

	pwd, _ := bcrypt.GenerateFromPassword([]byte("camping123"), bcrypt.DefaultCost)
	token := "session-" + persona + "-demo"
	userID := "u-" + persona

	if s.isPgSQL {
		_, _ = s.db.ExecContext(r.Context(), `INSERT INTO users (id, email, password_hash, full_name) VALUES ($1, $2, $3, $4) ON CONFLICT (email) DO UPDATE SET full_name = EXCLUDED.full_name`, userID, email, string(pwd), name)
		_, _ = s.db.ExecContext(r.Context(), `INSERT INTO sessions (token, user_id, expires_at) VALUES ($1, $2, $3) ON CONFLICT (token) DO NOTHING`, token, userID, time.Now().Add(7*24*time.Hour))
	} else {
		s.mem.mu.Lock()
		u := User{ID: userID, Email: email, PasswordHash: string(pwd), FullName: name, CreatedAt: time.Now()}
		s.mem.users[email] = u
		s.mem.usersByID[userID] = u
		s.mem.sessions[token] = userID
		s.mem.mu.Unlock()
	}

	s.setSessionCookie(w, token)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// J3: Reservation Booking & Concurrency Engine
func (s *Server) HandleCreateBooking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := s.authenticateUser(w, r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `<div class="p-3 bg-amber-100 text-amber-800 text-sm rounded">Please log in or select a demo persona to complete this booking.</div>`)
		return
	}

	campsiteID := r.FormValue("campsite_id")
	startDateStr := r.FormValue("start_date")
	endDateStr := r.FormValue("end_date")

	start, err1 := time.Parse("2006-01-02", startDateStr)
	end, err2 := time.Parse("2006-01-02", endDateStr)

	if err1 != nil || err2 != nil || !end.After(start) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `<div class="p-3 bg-red-100 text-red-700 text-sm rounded">Check-out date must be strictly after check-in date.</div>`)
		return
	}

	// Calculate nights and pricing
	nights := int(end.Sub(start).Hours() / 24)
	if nights < 1 {
		nights = 1
	}

	dailyRate := 4000
	campsiteName := "Campsite"

	if s.isPgSQL {
		// Strict PostgreSQL GiST Transaction
		tx, txErr := s.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if txErr != nil {
			http.Error(w, "Transaction init error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		// Row-lock the campsite row
		row := tx.QueryRowContext(r.Context(), "SELECT name, daily_rate_cents FROM campsites WHERE id = $1 FOR UPDATE", campsiteID)
		_ = row.Scan(&campsiteName, &dailyRate)
		totalCents := dailyRate * nights

		// Attempt insertion guarded by btree_gist exclusion constraint
		insQ := `INSERT INTO reservations (campsite_id, user_id, start_date, end_date, guests_count, total_cents, status)
		         VALUES ($1, $2, $3, $4, $5, $6, 'confirmed') RETURNING id`
		var resID string
		insErr := tx.QueryRowContext(r.Context(), insQ, campsiteID, user.ID, start, end, 2, totalCents).Scan(&resID)

		if insErr != nil {
			// Exclusion constraint violation
			w.WriteHeader(http.StatusConflict)
			fmt.Fprintf(w, `<div class="p-4 bg-red-50 border-l-4 border-red-500 text-red-900 rounded shadow-sm">
				<div class="font-bold">⚠️ Concurrency Conflict (Double-Booking Prevented)</div>
				<div class="text-sm mt-1">Another camper secured dates [%s to %s] milliseconds ahead of your request. Please select alternate dates.</div>
			</div>`, startDateStr, endDateStr)
			return
		}

		if cErr := tx.Commit(); cErr != nil {
			w.WriteHeader(http.StatusConflict)
			fmt.Fprintf(w, `<div class="p-3 bg-red-100 text-red-700 text-sm rounded">Reservation commit conflict. Please retry.</div>`)
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<div class="p-4 bg-green-50 border-l-4 border-green-600 text-green-900 rounded shadow-sm">
			<div class="font-bold">🎉 Reservation Confirmed!</div>
			<div class="text-sm mt-1">Confirmed for <strong>%s</strong> from %s to %s (%d night(s) · $%.2f).</div>
			<div class="text-xs text-green-700 mt-2">Reservation ID: %s · Atomically locked via GiST</div>
		</div>`, campsiteName, startDateStr, endDateStr, nights, float64(totalCents)/100.0, resID)
		return
	}

	// Resilient In-Memory Isolation with Mutex Protection
	s.mem.mu.Lock()
	defer s.mem.mu.Unlock()

	if cs, exists := s.mem.campsites[campsiteID]; exists {
		dailyRate = cs.DailyRateCents
		campsiteName = cs.Name
	}

	// Check active holds from other campers
	holdToken := r.FormValue("hold_token")
	activeHolds := s.leaseEngine.GetActiveHolds(campsiteID)
	for _, h := range activeHolds {
		if holdToken != "" && h.TokenID == holdToken {
			continue // caller owns this hold
		}
		if start.Before(h.EndDate) && end.After(h.StartDate) {
			w.WriteHeader(http.StatusConflict)
			fmt.Fprintf(w, `<div class="p-4 bg-red-50 border-l-4 border-red-500 text-red-900 rounded shadow-sm">
				<div class="font-bold">⚠️ Concurrency Conflict (Slot Held in Cart)</div>
				<div class="text-sm mt-1">Another camper currently holds dates [%s to %s] in their 15-minute checkout cart. Please select alternate dates.</div>
			</div>`, startDateStr, endDateStr)
			return
		}
	}

	// Check date interval overlap: [start, end) overlaps [resStart, resEnd) iff start < resEnd AND end > resStart
	for _, res := range s.mem.reservations {
		if res.CampsiteID == campsiteID && res.Status == "confirmed" {
			if start.Before(res.EndDate) && end.After(res.StartDate) {
				w.WriteHeader(http.StatusConflict)
				fmt.Fprintf(w, `<div class="p-4 bg-red-50 border-l-4 border-red-500 text-red-900 rounded shadow-sm">
					<div class="font-bold">⚠️ Concurrency Conflict (Double-Booking Prevented)</div>
					<div class="text-sm mt-1">Another camper secured dates [%s to %s] milliseconds ahead of your request. Please select alternate dates.</div>
				</div>`, startDateStr, endDateStr)
				return
			}
		}
	}

	if holdToken != "" {
		_ = s.leaseEngine.CommitHold(holdToken)
	}

	resID := fmt.Sprintf("res-%s", generateToken()[:8])
	totalCents := dailyRate * nights
	s.mem.reservations[resID] = Reservation{
		ID:           resID,
		CampsiteID:   campsiteID,
		UserID:       user.ID,
		StartDate:    start,
		EndDate:      end,
		GuestsCount:  2,
		TotalCents:   totalCents,
		Status:       "confirmed",
		CreatedAt:    time.Now().UTC(),
		CampsiteName: campsiteName,
		UserName:     user.FullName,
	}

	// Notify Ranger operations hub over SSE
	s.rangerHub.Broadcast("RESERVATION_CREATED", fmt.Sprintf(`{"reservation_id":"%s","campsite":"%s","camper":"%s"}`, resID, campsiteName, user.FullName))

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<div class="p-4 bg-green-50 border-l-4 border-green-600 text-green-900 rounded shadow-sm">
		<div class="font-bold">🎉 Reservation Confirmed!</div>
		<div class="text-sm mt-1">Confirmed for <strong>%s</strong> from %s to %s (%d night(s) · $%.2f).</div>
		<div class="text-xs text-green-700 mt-2">Reservation ID: %s · Atomically locked via MemoryStore</div>
	</div>`, campsiteName, startDateStr, endDateStr, nights, float64(totalCents)/100.0, resID)
}

// J4: Self-Service Reservation Management & Cancellation
func (s *Server) HandleMyBookings(w http.ResponseWriter, r *http.Request) {
	user := s.authenticateUser(w, r)
	if user == nil {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<div class="p-6 text-center text-stone-500">Please sign in or select a demo persona to view bookings.</div>`)
		return
	}

	var bookings []Reservation
	if s.isPgSQL {
		q := `SELECT r.id, r.campsite_id, r.start_date, r.end_date, r.guests_count, r.total_cents, r.status, c.name 
		      FROM reservations r JOIN campsites c ON r.campsite_id = c.id 
		      WHERE r.user_id = $1 ORDER BY r.start_date DESC`
		rows, err := s.db.QueryContext(r.Context(), q, user.ID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var b Reservation
				_ = rows.Scan(&b.ID, &b.CampsiteID, &b.StartDate, &b.EndDate, &b.GuestsCount, &b.TotalCents, &b.Status, &b.CampsiteName)
				b.UserID = user.ID
				bookings = append(bookings, b)
			}
		}
	} else {
		s.mem.mu.RLock()
		for _, b := range s.mem.reservations {
			if b.UserID == user.ID {
				bookings = append(bookings, b)
			}
		}
		s.mem.mu.RUnlock()
	}

	data := map[string]interface{}{
		"User":     user,
		"Bookings": bookings,
	}
	s.tmpl.ExecuteTemplate(w, "bookings-stream", data)
}

func (s *Server) HandleCancelBooking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := s.authenticateUser(w, r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	bookingID := r.FormValue("booking_id")
	if s.isPgSQL {
		_, err := s.db.ExecContext(r.Context(), "UPDATE reservations SET status = 'cancelled' WHERE id = $1 AND user_id = $2", bookingID, user.ID)
		if err != nil {
			http.Error(w, "Failed to cancel", http.StatusInternalServerError)
			return
		}
	} else {
		s.mem.mu.Lock()
		if b, exists := s.mem.reservations[bookingID]; exists && b.UserID == user.ID {
			b.Status = "cancelled"
			s.mem.reservations[bookingID] = b
		}
		s.mem.mu.Unlock()
	}

	// Re-render bookings stream
	s.HandleMyBookings(w, r)
}

// J5: Multi-User Simulation & Swarm Contention Demonstrator
func (s *Server) HandleSimulateContention(w http.ResponseWriter, r *http.Request) {
	campsiteID := "c1"
	startDateStr := time.Now().Add(7 * 24 * time.Hour).Format("2006-01-02")
	endDateStr := time.Now().Add(10 * 24 * time.Hour).Format("2006-01-02")

	start, _ := time.Parse("2006-01-02", startDateStr)
	end, _ := time.Parse("2006-01-02", endDateStr)

	var wg sync.WaitGroup
	type SimResult struct {
		User    string
		Success bool
		Message string
		Latency time.Duration
	}
	results := make([]SimResult, 2)

	// Goroutine 1: Alice
	wg.Add(1)
	go func() {
		defer wg.Done()
		t0 := time.Now()
		var success bool
		var msg string

		if s.isPgSQL {
			tx, _ := s.db.Begin()
			defer tx.Rollback()
			var resID string
			err := tx.QueryRow(`INSERT INTO reservations (campsite_id, user_id, start_date, end_date, guests_count, total_cents, status)
			                    VALUES ($1, 'u-alice', $2, $3, 2, 10500, 'confirmed') RETURNING id`, campsiteID, start, end).Scan(&resID)
			if err == nil && tx.Commit() == nil {
				success = true
				msg = fmt.Sprintf("Alice acquired slot (ID: %s)", resID)
			} else {
				msg = "Alice blocked by GiST exclusion lock"
			}
		} else {
			s.mem.mu.Lock()
			overlap := false
			for _, res := range s.mem.reservations {
				if res.CampsiteID == campsiteID && res.Status == "confirmed" {
					if start.Before(res.EndDate) && end.After(res.StartDate) {
						overlap = true
						break
					}
				}
			}
			if !overlap {
				resID := "res-alice-sim"
				s.mem.reservations[resID] = Reservation{
					ID: resID, CampsiteID: campsiteID, UserID: "u-alice", StartDate: start, EndDate: end, Status: "confirmed", TotalCents: 10500,
				}
				success = true
				msg = "Alice acquired slot"
			} else {
				msg = "Alice blocked by in-memory mutex exclusion"
			}
			s.mem.mu.Unlock()
		}

		results[0] = SimResult{User: "Alice Explorer", Success: success, Message: msg, Latency: time.Since(t0)}
	}()

	// Goroutine 2: Bob (Simultaneous Contender)
	wg.Add(1)
	go func() {
		defer wg.Done()
		t0 := time.Now()
		var success bool
		var msg string

		if s.isPgSQL {
			tx, _ := s.db.Begin()
			defer tx.Rollback()
			var resID string
			err := tx.QueryRow(`INSERT INTO reservations (campsite_id, user_id, start_date, end_date, guests_count, total_cents, status)
			                    VALUES ($1, 'u-bob', $2, $3, 2, 10500, 'confirmed') RETURNING id`, campsiteID, start, end).Scan(&resID)
			if err == nil && tx.Commit() == nil {
				success = true
				msg = fmt.Sprintf("Bob acquired slot (ID: %s)", resID)
			} else {
				msg = "Bob blocked by GiST exclusion lock"
			}
		} else {
			s.mem.mu.Lock()
			overlap := false
			for _, res := range s.mem.reservations {
				if res.CampsiteID == campsiteID && res.Status == "confirmed" {
					if start.Before(res.EndDate) && end.After(res.StartDate) {
						overlap = true
						break
					}
				}
			}
			if !overlap {
				resID := "res-bob-sim"
				s.mem.reservations[resID] = Reservation{
					ID: resID, CampsiteID: campsiteID, UserID: "u-bob", StartDate: start, EndDate: end, Status: "confirmed", TotalCents: 10500,
				}
				success = true
				msg = "Bob acquired slot"
			} else {
				msg = "Bob blocked by in-memory mutex exclusion"
			}
			s.mem.mu.Unlock()
		}

		results[1] = SimResult{User: "Bob Camper", Success: success, Message: msg, Latency: time.Since(t0)}
	}()

	wg.Wait()

	data := map[string]interface{}{
		"DateRange": fmt.Sprintf("%s to %s", startDateStr, endDateStr),
		"Results":   results,
	}
	s.tmpl.ExecuteTemplate(w, "simulation-results", data)
}

func (s *Server) HandleHold(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		campsiteID := r.URL.Query().Get("campsite_id")
		holds := s.leaseEngine.GetActiveHolds(campsiteID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"campsite_id":  campsiteID,
			"active_holds": holds,
		})
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := s.authenticateUser(w, r)
	userID := "u-guest"
	if user != nil {
		userID = user.ID
	}

	campsiteID := r.FormValue("campsite_id")
	startDateStr := r.FormValue("start_date")
	endDateStr := r.FormValue("end_date")

	start, err1 := time.Parse("2006-01-02", startDateStr)
	end, err2 := time.Parse("2006-01-02", endDateStr)

	if err1 != nil || err2 != nil || !end.After(start) {
		w.WriteHeader(http.StatusBadRequest)
		if r.Header.Get("HX-Request") == "true" {
			fmt.Fprintf(w, `<div class="p-2 bg-rose-100 text-rose-800 text-xs rounded">Invalid date range</div>`)
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid dates"})
		}
		return
	}

	hold, err := s.leaseEngine.AcquireHold(campsiteID, userID, start, end)
	if err != nil {
		w.WriteHeader(http.StatusConflict)
		if r.Header.Get("HX-Request") == "true" {
			fmt.Fprintf(w, `<div class="p-2.5 bg-rose-50 border border-rose-300 text-rose-800 text-xs rounded">
				⚠️ <strong>Hold Conflict</strong>: %v
			</div>`, err)
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		}
		return
	}

	w.WriteHeader(http.StatusCreated)
	if r.Header.Get("HX-Request") == "true" {
		fmt.Fprintf(w, `<div class="p-2.5 bg-amber-50 border border-amber-300 text-amber-900 text-xs rounded space-y-1">
			<div class="font-bold flex items-center justify-between">
				<span>⏳ 15-Min Slot Hold Active</span>
				<span class="font-mono text-[10px]">%s</span>
			</div>
			<div>Held until %s. Complete checkout to finalize.</div>
			<div class="text-[10px] text-stone-500 font-mono">Token: %s</div>
		</div>`, hold.TokenID[:8], hold.ExpiresAt.Format("15:04:05"), hold.TokenID)
	} else {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(hold)
	}
}

func (s *Server) HandleHoldCommit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tokenID := r.FormValue("token_id")
	if tokenID == "" {
		tokenID = r.URL.Query().Get("token_id")
	}

	if err := s.leaseEngine.CommitHold(tokenID); err != nil {
		w.WriteHeader(http.StatusConflict)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "COMMITTED", "token_id": tokenID})
}

func (s *Server) HandleHoldRelease(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tokenID := r.FormValue("token_id")
	if tokenID == "" {
		tokenID = r.URL.Query().Get("token_id")
	}

	if err := s.leaseEngine.ReleaseHold(tokenID); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "RELEASED", "token_id": tokenID})
}

func (s *Server) HandleWeatherMicroclimate(w http.ResponseWriter, r *http.Request) {
	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")
	campsiteID := r.URL.Query().Get("campsite_id")

	var lat, lon float64
	var csName string

	if campsiteID != "" {
		s.mem.mu.RLock()
		if cs, exists := s.mem.campsites[campsiteID]; exists {
			lat = cs.Latitude
			lon = cs.Longitude
			csName = cs.Name
		}
		s.mem.mu.RUnlock()
	}

	if lat == 0 && lon == 0 && latStr != "" && lonStr != "" {
		lat, _ = strconv.ParseFloat(latStr, 64)
		lon, _ = strconv.ParseFloat(lonStr, 64)
	}

	if lat == 0 && lon == 0 {
		lat = 46.8523
		lon = -121.7603
		csName = "Cascade Wilderness"
	}

	telemetry, err := s.envSvc.FetchNOAAMicroclimate(lat, lon)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		if r.Header.Get("HX-Request") == "true" {
			fmt.Fprintf(w, `<div class="text-[10px] text-stone-400">NOAA Telemetry Unavailable</div>`)
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		}
		return
	}

	fireRisk := s.envSvc.CalculateFireRisk(telemetry.TemperatureMax, telemetry.WindSpeedMph, 20)
	telemetry.FireRiskIndex = fireRisk
	advisory := s.envSvc.GenerateCamperAdvisory(telemetry)

	if r.Header.Get("HX-Request") == "true" {
		riskColor := "bg-emerald-100 text-emerald-800"
		if fireRisk == "MODERATE" {
			riskColor = "bg-amber-100 text-amber-800"
		} else if fireRisk == "HIGH" || fireRisk == "EXTREME" {
			riskColor = "bg-rose-100 text-rose-800"
		}

		advSnippet := ""
		if advisory != "" {
			advSnippet = fmt.Sprintf(`<div class="text-[10px] text-amber-900 bg-amber-50 p-1 rounded mt-1">%s</div>`, advisory)
		}

		fmt.Fprintf(w, `<div class="p-2 bg-stone-100 border border-stone-200 rounded text-xs space-y-1">
			<div class="flex items-center justify-between">
				<span class="font-bold text-stone-800">🌤️ %s (%.0f° - %.0f°F)</span>
				<span class="text-[10px] px-1.5 py-0.5 rounded font-bold %s">Fire: %s</span>
			</div>
			<div class="text-[11px] text-stone-600">Wind: %.0f mph · Station: %s</div>
			%s
		</div>`, telemetry.ShortForecast, telemetry.TemperatureMin, telemetry.TemperatureMax, riskColor, fireRisk, telemetry.WindSpeedMph, telemetry.StationID, advSnippet)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"campsite":   csName,
		"lat":        lat,
		"lon":        lon,
		"telemetry":  telemetry,
		"fire_risk":  fireRisk,
		"advisory":   advisory,
	})
}

func (s *Server) HandleRangerWorkOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		cgID := r.FormValue("campground_id")
		csID := r.FormValue("campsite_id")
		cat := r.FormValue("category")
		sev := r.FormValue("severity")
		desc := r.FormValue("description")

		if cgID == "" {
			cgID = "cg-olympic"
		}
		if csID == "" {
			csID = "c1"
		}
		if cat == "" {
			cat = "BEAR_BOX"
		}
		if sev == "" {
			sev = "MEDIUM"
		}
		if desc == "" {
			desc = "Routine maintenance inspection"
		}

		order, err := s.rangerPMS.CreateWorkOrder(cgID, csID, cat, sev, desc)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(order)
		return
	}

	if r.Method == http.MethodPut || r.Method == http.MethodPatch {
		orderID := r.FormValue("order_id")
		status := r.FormValue("status")
		rangerID := r.FormValue("ranger_id")
		if rangerID == "" {
			rangerID = "ranger-lead"
		}

		order, err := s.rangerPMS.UpdateWorkOrderStatus(orderID, status, rangerID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(order)
		return
	}

	// GET: List work orders
	cgID := r.URL.Query().Get("campground_id")
	orders := s.rangerPMS.ListWorkOrders(cgID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orders)
}

func (s *Server) HandleRangerVehicles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	plate := r.FormValue("plate")
	state := r.FormValue("state")
	csID := r.FormValue("campsite_id")
	guest := r.FormValue("guest_name")

	checkIn := time.Now().Add(-1 * time.Hour)
	checkOut := time.Now().Add(48 * time.Hour)

	s.rangerPMS.RegisterVehicle(plate, state, csID, guest, checkIn, checkOut)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "REGISTERED",
		"plate":       plate,
		"state":       state,
		"campsite_id": csID,
		"guest_name":  guest,
	})
}

func (s *Server) HandleRangerGateAuthorize(w http.ResponseWriter, r *http.Request) {
	plate := r.URL.Query().Get("plate")
	state := r.URL.Query().Get("state")
	if plate == "" {
		plate = r.FormValue("plate")
		state = r.FormValue("state")
	}

	auth, reason := s.rangerPMS.AuthorizeGateEntry(plate, state)
	status := "DENIED"
	if auth {
		status = "AUTHORIZED"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     status,
		"authorized": auth,
		"reason":     reason,
		"plate":      plate,
		"state":      state,
	})
}

func (s *Server) HandleRangerEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	ch := s.rangerHub.Subscribe()
	defer s.rangerHub.Unsubscribe(ch)

	fmt.Fprintf(w, "event: connected\ndata: {\"timestamp\": \"%s\"}\n\n", time.Now().UTC().Format(time.RFC3339))
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "%s", msg)
			flusher.Flush()
		}
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	server := NewServer(dbURL)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", server.HandleHealth)
	mux.HandleFunc("/", server.HandleCampsiteList)
	mux.HandleFunc("/availability", server.HandleAvailability)
	mux.HandleFunc("/bookings", server.HandleCreateBooking)
	mux.HandleFunc("/my-bookings", server.HandleMyBookings)
	mux.HandleFunc("/cancel-booking", server.HandleCancelBooking)
	mux.HandleFunc("/signup", server.HandleSignup)
	mux.HandleFunc("/login", server.HandleLogin)
	mux.HandleFunc("/logout", server.HandleLogout)
	mux.HandleFunc("/switch-persona", server.HandleSwitchPersona)
	mux.HandleFunc("/simulate-contention", server.HandleSimulateContention)

	// Subagent domain routes: Topo-Lease, Alpine-Shield, Wild-PMS
	mux.HandleFunc("/api/v1/holds", server.HandleHold)
	mux.HandleFunc("/api/v1/holds/commit", server.HandleHoldCommit)
	mux.HandleFunc("/api/v1/holds/release", server.HandleHoldRelease)
	mux.HandleFunc("/api/v1/weather/microclimate", server.HandleWeatherMicroclimate)
	mux.HandleFunc("/api/v1/ranger/workorders", server.HandleRangerWorkOrders)
	mux.HandleFunc("/api/v1/ranger/vehicles", server.HandleRangerVehicles)
	mux.HandleFunc("/api/v1/ranger/gate/authorize", server.HandleRangerGateAuthorize)
	mux.HandleFunc("/api/v1/ranger/events", server.HandleRangerEvents)

	// Track 1: Camper Profile & Multi-lane ALPR Gatehouse
	mux.HandleFunc("/api/v1/profile", server.HandleCamperProfile)
	mux.HandleFunc("/api/v1/profile/vehicles", server.HandleCamperVehicles)
	mux.HandleFunc("/api/v1/gate/scan", server.HandleGateScan)
	mux.HandleFunc("/api/v1/gate/logs", server.HandleGateLogs)
	mux.HandleFunc("/api/v1/gate/reset", server.HandleGateReset)

	// Track 2: Wild-PMS & Incident Command System
	mux.HandleFunc("/api/v1/incidents", server.HandleIncidents)
	mux.HandleFunc("/api/v1/incidents/dispatch", server.HandleIncidentDispatch)
	mux.HandleFunc("/api/v1/incidents/resolve", server.HandleIncidentResolve)
	mux.HandleFunc("/api/v1/incidents/muster", server.HandleIncidentMuster)
	mux.HandleFunc("/api/v1/pms/assets", server.HandlePMSAssets)
	mux.HandleFunc("/api/v1/pms/occupancy", server.HandlePMSOccupancy)
	mux.HandleFunc("/api/v1/pms/checkin", server.HandlePMSCheckIn)
	mux.HandleFunc("/api/v1/pms/checkout", server.HandlePMSCheckOut)

	// Track 3 & 4: Backcountry Quotas & Outfitter Lockers
	mux.HandleFunc("/api/v1/backcountry/zones", server.HandleBackcountryZones)
	mux.HandleFunc("/api/v1/backcountry/applications", server.HandleBackcountryApply)
	mux.HandleFunc("/api/v1/backcountry/commitment", server.HandleBackcountryCommit)
	mux.HandleFunc("/api/v1/backcountry/draw", server.HandleBackcountryDraw)
	mux.HandleFunc("/api/v1/outfitter/gear", server.HandleOutfitterGear)
	mux.HandleFunc("/api/v1/outfitter/reserve", server.HandleOutfitterReserve)
	mux.HandleFunc("/api/v1/outfitter/unlock", server.HandleOutfitterUnlock)
	mux.HandleFunc("/api/v1/outfitter/return", server.HandleOutfitterReturn)
	mux.HandleFunc("/api/v1/outfitter/lockers", server.HandleOutfitterLockers)

	// Track 5 & 6: Rothermel Fire Spread & Offline Sync
	mux.HandleFunc("/api/v1/telemetry/mesh", server.HandleTelemetryMesh)
	mux.HandleFunc("/api/v1/telemetry/rothermel", server.HandleTelemetryRothermel)
	mux.HandleFunc("/api/v1/telemetry/briefing", server.HandleTelemetryBriefing)
	mux.HandleFunc("/api/v1/sync/wal", server.HandleSyncWAL)
	mux.HandleFunc("/api/v1/sync/authorize-offline", server.HandleSyncAuthorizeOffline)
	mux.HandleFunc("/api/v1/sync/reconcile", server.HandleSyncReconcile)

	// Track 7 & 8: Weather Telemetry & Emergency SOS SAR Dispatch
	mux.HandleFunc("/api/v1/weather/campsite", server.HandleWeatherCampsite)
	mux.HandleFunc("/api/v1/sos/beacon", server.HandleSOSBeaconTrigger)
	mux.HandleFunc("/api/v1/sos/active", server.HandleSOSBeaconList)
	mux.HandleFunc("/api/v1/sos/resolve", server.HandleSOSBeaconResolve)

	log.Printf("Alpine OS 2.0 server listening on :%s\n", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server shutdown: %v\n", err)
	}
}

// Route Binding: GET type -> 

// Route Binding: GET guests -> 

// Route Binding: GET q -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET HX-Target -> 

// Route Binding: GET id -> 

// Route Binding: GET user -> 

// Route Binding: GET campsite_id -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET token_id -> 

// Route Binding: GET token_id -> 

// Route Binding: GET lat -> 

// Route Binding: GET lon -> 

// Route Binding: GET campsite_id -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET campground_id -> 

// Route Binding: GET plate -> 

// Route Binding: GET state -> 

// Route Binding: ANY /health -> server.HandleHealth

// Route Binding: ANY / -> server.HandleCampsiteList

// Route Binding: ANY /availability -> server.HandleAvailability

// Route Binding: ANY /bookings -> server.HandleCreateBooking

// Route Binding: ANY /my-bookings -> server.HandleMyBookings

// Route Binding: ANY /cancel-booking -> server.HandleCancelBooking

// Route Binding: ANY /signup -> server.HandleSignup

// Route Binding: ANY /login -> server.HandleLogin

// Route Binding: ANY /logout -> server.HandleLogout

// Route Binding: ANY /switch-persona -> server.HandleSwitchPersona

// Route Binding: ANY /simulate-contention -> server.HandleSimulateContention

// Route Binding: ANY /api/v1/holds -> server.HandleHold

// Route Binding: ANY /api/v1/holds/commit -> server.HandleHoldCommit

// Route Binding: ANY /api/v1/holds/release -> server.HandleHoldRelease

// Route Binding: ANY /api/v1/weather/microclimate -> server.HandleWeatherMicroclimate

// Route Binding: ANY /api/v1/ranger/workorders -> server.HandleRangerWorkOrders

// Route Binding: ANY /api/v1/ranger/vehicles -> server.HandleRangerVehicles

// Route Binding: ANY /api/v1/ranger/gate/authorize -> server.HandleRangerGateAuthorize

// Route Binding: ANY /api/v1/ranger/events -> server.HandleRangerEvents

// Route Binding: ANY /api/v1/profile -> server.HandleCamperProfile

// Route Binding: ANY /api/v1/profile/vehicles -> server.HandleCamperVehicles

// Route Binding: ANY /api/v1/gate/scan -> server.HandleGateScan

// Route Binding: ANY /api/v1/gate/logs -> server.HandleGateLogs

// Route Binding: ANY /api/v1/gate/reset -> server.HandleGateReset

// Route Binding: ANY /api/v1/incidents -> server.HandleIncidents

// Route Binding: ANY /api/v1/incidents/dispatch -> server.HandleIncidentDispatch

// Route Binding: ANY /api/v1/incidents/resolve -> server.HandleIncidentResolve

// Route Binding: ANY /api/v1/incidents/muster -> server.HandleIncidentMuster

// Route Binding: ANY /api/v1/pms/assets -> server.HandlePMSAssets

// Route Binding: ANY /api/v1/pms/occupancy -> server.HandlePMSOccupancy

// Route Binding: ANY /api/v1/pms/checkin -> server.HandlePMSCheckIn

// Route Binding: ANY /api/v1/pms/checkout -> server.HandlePMSCheckOut

// Route Binding: ANY /api/v1/backcountry/zones -> server.HandleBackcountryZones

// Route Binding: ANY /api/v1/backcountry/applications -> server.HandleBackcountryApply

// Route Binding: ANY /api/v1/backcountry/commitment -> server.HandleBackcountryCommit

// Route Binding: ANY /api/v1/backcountry/draw -> server.HandleBackcountryDraw

// Route Binding: ANY /api/v1/outfitter/gear -> server.HandleOutfitterGear

// Route Binding: ANY /api/v1/outfitter/reserve -> server.HandleOutfitterReserve

// Route Binding: ANY /api/v1/outfitter/unlock -> server.HandleOutfitterUnlock

// Route Binding: ANY /api/v1/outfitter/return -> server.HandleOutfitterReturn

// Route Binding: ANY /api/v1/outfitter/lockers -> server.HandleOutfitterLockers

// Route Binding: ANY /api/v1/telemetry/mesh -> server.HandleTelemetryMesh

// Route Binding: ANY /api/v1/telemetry/rothermel -> server.HandleTelemetryRothermel

// Route Binding: ANY /api/v1/telemetry/briefing -> server.HandleTelemetryBriefing

// Route Binding: ANY /api/v1/sync/wal -> server.HandleSyncWAL

// Route Binding: ANY /api/v1/sync/authorize-offline -> server.HandleSyncAuthorizeOffline

// Route Binding: ANY /api/v1/sync/reconcile -> server.HandleSyncReconcile

