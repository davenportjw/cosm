-- Track 7: Wilderness Emergency SOS & SAR Dispatch Schema
-- Migration: 008_emergency_sos_beacon.sql
-- Description: Emergency distress beacons, SAR telemetry coordinates, and ranger dispatch records.

CREATE TABLE IF NOT EXISTS emergency_sos_beacons (
    id VARCHAR(64) PRIMARY KEY,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    user_name VARCHAR(255) NOT NULL,
    trailhead_or_site VARCHAR(255) NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    distress_type VARCHAR(32) NOT NULL CHECK (distress_type IN ('MEDICAL', 'LOST', 'WILDFIRE', 'INJURY', 'WILDLIFE')),
    description TEXT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE_SEARCHING' CHECK (status IN ('ACTIVE_SEARCHING', 'DISPATCHED', 'RESCUE_UNDERWAY', 'RESOLVED')),
    assigned_ranger VARCHAR(255),
    reported_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_emergency_sos_beacons_status
    ON emergency_sos_beacons (status, reported_at DESC);
