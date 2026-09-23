-- Migration: 004_alpr_gate_and_lanes.sql
-- Description: Schema for multi-lane ALPR gatehouses, physical barrier states, and access audit logs.

CREATE TABLE IF NOT EXISTS gate_lanes (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    description TEXT,
    barrier_state VARCHAR(32) NOT NULL DEFAULT 'LOWERED',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS gate_access_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    log_id VARCHAR(64) UNIQUE NOT NULL,
    plate VARCHAR(32) NOT NULL,
    state VARCHAR(16) NOT NULL,
    lane VARCHAR(64) NOT NULL REFERENCES gate_lanes(id) ON DELETE RESTRICT,
    authorized BOOLEAN NOT NULL,
    reason TEXT NOT NULL,
    campsite_id VARCHAR(64),
    guest_name VARCHAR(255),
    barrier_action VARCHAR(64) NOT NULL,
    scanned_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_gate_access_logs_plate_state
    ON gate_access_logs (plate, state);

CREATE INDEX IF NOT EXISTS idx_gate_access_logs_lane
    ON gate_access_logs (lane);

CREATE INDEX IF NOT EXISTS idx_gate_access_logs_scanned_at
    ON gate_access_logs (scanned_at DESC);

-- Seed initial standard gate lanes
INSERT INTO gate_lanes (id, name, description, barrier_state, is_active)
VALUES
    ('STANDARD', 'Standard Vehicle Lane', 'Primary passenger cars, SUVs, and light trucks access lane', 'LOWERED', TRUE),
    ('OVERSIZE_TRAILER', 'Oversize & Trailer Lane', 'Dedicated clearance lane for RVs, motorhomes, and towed travel trailers', 'LOWERED', TRUE),
    ('EMERGENCY_RANGER', 'Emergency & Ranger Priority Lane', 'Restricted priority corridor for NPS emergency response and ranger patrol vehicles', 'LOWERED', TRUE)
ON CONFLICT (id) DO NOTHING;
