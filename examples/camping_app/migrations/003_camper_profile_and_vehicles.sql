-- Migration: 003_camper_profile_and_vehicles.sql
-- Description: Schema for camper profiles, wilderness permits, and multi-vehicle fleet records.

CREATE TABLE IF NOT EXISTS camper_profiles (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    full_name VARCHAR(255) NOT NULL,
    phone VARCHAR(32) NOT NULL,
    emergency_contact_name VARCHAR(255),
    emergency_contact_phone VARCHAR(32),
    wilderness_pass_id VARCHAR(64),
    notifications_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS camper_vehicles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES camper_profiles(user_id) ON DELETE CASCADE,
    plate VARCHAR(32) NOT NULL,
    state VARCHAR(16) NOT NULL,
    make_model VARCHAR(128),
    color VARCHAR(64),
    is_ev BOOLEAN NOT NULL DEFAULT FALSE,
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_camper_vehicle_plate_state UNIQUE (user_id, plate, state)
);

CREATE INDEX IF NOT EXISTS idx_camper_vehicles_user_id
    ON camper_vehicles (user_id);

CREATE INDEX IF NOT EXISTS idx_camper_vehicles_plate_state
    ON camper_vehicles (plate, state);
