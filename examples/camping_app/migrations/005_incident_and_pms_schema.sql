-- Migration 005: NPS Incident Command System (ICS) & Facilities Asset Management (Wild-PMS)
-- Supports Track 2: Incident lifecycle management, campsite asset maintenance registry, and live in-park occupancy roster.

-- 1. ICS Incidents Table
CREATE TABLE IF NOT EXISTS ics_incidents (
    id VARCHAR(64) PRIMARY KEY,
    incident_type VARCHAR(64) NOT NULL,
    severity VARCHAR(32) NOT NULL,
    campground_id VARCHAR(64) NOT NULL,
    sector VARCHAR(64) NOT NULL,
    gps_trailhead VARCHAR(128),
    description TEXT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'REPORTED',
    assigned_ranger VARCHAR(64),
    notes TEXT,
    reported_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    CONSTRAINT valid_incident_severity CHECK (severity IN ('ADVISORY', 'ALERT', 'EVACUATION_WARNING', 'IMMEDIATE_EVACUATION')),
    CONSTRAINT valid_incident_status CHECK (status IN ('REPORTED', 'DISPATCHED', 'CONTAINED', 'RESOLVED'))
);

CREATE INDEX IF NOT EXISTS idx_ics_incidents_campground ON ics_incidents(campground_id);
CREATE INDEX IF NOT EXISTS idx_ics_incidents_status ON ics_incidents(status);
CREATE INDEX IF NOT EXISTS idx_ics_incidents_severity ON ics_incidents(severity);
CREATE INDEX IF NOT EXISTS idx_ics_incidents_reported_at ON ics_incidents(reported_at);

-- 2. Campsite Assets Table
CREATE TABLE IF NOT EXISTS campsite_assets (
    id VARCHAR(64) PRIMARY KEY,
    campground_id VARCHAR(64),
    campsite_id VARCHAR(64) NOT NULL,
    asset_class VARCHAR(64) NOT NULL,
    serial_number VARCHAR(128) NOT NULL,
    installed_date TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    condition_rating VARCHAR(32) NOT NULL DEFAULT 'GOOD',
    last_inspected TIMESTAMPTZ,
    next_inspection_due TIMESTAMPTZ,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT valid_condition_rating CHECK (condition_rating IN ('EXCELLENT', 'GOOD', 'NEEDS_REPAIR', 'CONDEMNED')),
    CONSTRAINT valid_asset_class CHECK (asset_class IN ('BEAR_BOX', 'FIRE_RING', 'WATER_SPIGOT', 'GRAYWATER_DUMP', 'SOLAR_PEDESTAL'))
);

CREATE INDEX IF NOT EXISTS idx_campsite_assets_campground ON campsite_assets(campground_id);
CREATE INDEX IF NOT EXISTS idx_campsite_assets_campsite ON campsite_assets(campsite_id);
CREATE INDEX IF NOT EXISTS idx_campsite_assets_condition ON campsite_assets(condition_rating);
CREATE INDEX IF NOT EXISTS idx_campsite_assets_next_inspection ON campsite_assets(next_inspection_due);

-- 3. In-Park Live Occupancy Table
CREATE TABLE IF NOT EXISTS in_park_occupancy (
    campsite_id VARCHAR(64) PRIMARY KEY,
    reservation_id VARCHAR(64) NOT NULL,
    guest_name VARCHAR(255) NOT NULL,
    primary_plate VARCHAR(32),
    trailer_plate VARCHAR(32),
    checked_in_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    scheduled_check_out TIMESTAMPTZ NOT NULL,
    party_size INT NOT NULL DEFAULT 1,
    emergency_phone VARCHAR(32),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT valid_party_size CHECK (party_size > 0)
);

CREATE INDEX IF NOT EXISTS idx_in_park_occupancy_reservation ON in_park_occupancy(reservation_id);
CREATE INDEX IF NOT EXISTS idx_in_park_occupancy_primary_plate ON in_park_occupancy(primary_plate);
CREATE INDEX IF NOT EXISTS idx_in_park_occupancy_scheduled_out ON in_park_occupancy(scheduled_check_out);
