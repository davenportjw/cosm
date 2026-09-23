-- Track 4: Wilderness Outfitter & Smart Contactless Lockers Schema
-- Migration: 007_outfitter_and_lockers.sql
-- Description: Gear rental catalog, smart contactless locker bays (1..16), and rental agreements.

-- Table: gear_inventory
-- Catalog of high-end backcountry expedition equipment available for rental
CREATE TABLE IF NOT EXISTS gear_inventory (
    serial_number VARCHAR(64) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    category VARCHAR(64) NOT NULL CHECK (category IN ('BEAR_CANISTER', 'SATELLITE_BEACON', 'FOUR_SEASON_TENT', 'SNOWSHOES', 'WATER_FILTER')),
    daily_rate_cents INT NOT NULL CHECK (daily_rate_cents >= 0),
    deposit_cents INT NOT NULL CHECK (deposit_cents >= 0),
    condition VARCHAR(32) NOT NULL DEFAULT 'MINT' CHECK (condition IN ('MINT', 'GOOD', 'INSPECTION_REQUIRED')),
    status VARCHAR(32) NOT NULL DEFAULT 'AVAILABLE' CHECK (status IN ('AVAILABLE', 'RESERVED', 'CHECKED_OUT', 'MAINTENANCE')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_gear_inventory_category_status
    ON gear_inventory (category, status);

-- Table: smart_lockers
-- Contactless electronic locker compartments (bays 1..16) with OTP PIN authentication
CREATE TABLE IF NOT EXISTS smart_lockers (
    bay_number INT PRIMARY KEY CHECK (bay_number BETWEEN 1 AND 16),
    item_serial_number VARCHAR(64) REFERENCES gear_inventory(serial_number) ON DELETE SET NULL,
    assigned_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    reservation_id VARCHAR(64),
    passcode_pin VARCHAR(6),
    status VARCHAR(32) NOT NULL DEFAULT 'IDLE' CHECK (status IN ('IDLE', 'LOADED', 'CLAIMED', 'RETURNED')),
    expires_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_smart_lockers_status
    ON smart_lockers (status);

-- Table: gear_rentals
-- Rental contract agreements and audit lifecycle for outfitter gear
CREATE TABLE IF NOT EXISTS gear_rentals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reservation_id VARCHAR(64) NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    item_serial_number VARCHAR(64) NOT NULL REFERENCES gear_inventory(serial_number) ON DELETE RESTRICT,
    bay_number INT REFERENCES smart_lockers(bay_number) ON DELETE SET NULL,
    rental_days INT NOT NULL CHECK (rental_days > 0),
    total_cents INT NOT NULL CHECK (total_cents >= 0),
    deposit_held_cents INT NOT NULL CHECK (deposit_held_cents >= 0),
    status VARCHAR(32) NOT NULL DEFAULT 'RESERVED' CHECK (status IN ('RESERVED', 'CHECKED_OUT', 'RETURNED', 'OVERDUE')),
    rented_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    due_at TIMESTAMPTZ NOT NULL,
    returned_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_gear_rentals_user
    ON gear_rentals (user_id, status);

CREATE INDEX IF NOT EXISTS idx_gear_rentals_item
    ON gear_rentals (item_serial_number, status);
