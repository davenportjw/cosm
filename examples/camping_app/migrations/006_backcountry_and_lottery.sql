-- Track 3: Backcountry Trailhead Quotas & Provably Fair Lottery Engine Schema
-- Migration: 006_backcountry_and_lottery.sql
-- Description: Trailhead zones registry, daily quota allocations, cryptographic commitments, and lottery submissions.

-- Table: trailhead_zones
-- Defines regulated backcountry zones with daily human quotas and gear requirements
CREATE TABLE IF NOT EXISTS trailhead_zones (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    elevation_ft INT NOT NULL,
    daily_quota INT NOT NULL CHECK (daily_quota > 0),
    bear_canister_required BOOLEAN NOT NULL DEFAULT FALSE,
    wag_bag_required BOOLEAN NOT NULL DEFAULT FALSE,
    difficulty_rating VARCHAR(32) NOT NULL CHECK (difficulty_rating IN ('MODERATE', 'STRENUOUS', 'ALPINE_TECHNICAL')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Table: backcountry_quotas
-- Tracks day-by-day permit capacity, allocated permits, and remaining capacity
CREATE TABLE IF NOT EXISTS backcountry_quotas (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trailhead_id VARCHAR(64) NOT NULL REFERENCES trailhead_zones(id) ON DELETE CASCADE,
    quota_date DATE NOT NULL,
    daily_quota INT NOT NULL CHECK (daily_quota >= 0),
    allocated_quota INT NOT NULL DEFAULT 0 CHECK (allocated_quota >= 0),
    remaining_quota INT NOT NULL CHECK (remaining_quota >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_backcountry_quota_trailhead_date UNIQUE (trailhead_id, quota_date)
);

CREATE INDEX IF NOT EXISTS idx_backcountry_quotas_lookup
    ON backcountry_quotas (trailhead_id, quota_date);

-- Table: lottery_draw_commitments
-- Cryptographic SHA-256 commitments published before lottery draw, revealed post-draw
CREATE TABLE IF NOT EXISTS lottery_draw_commitments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trailhead_id VARCHAR(64) NOT NULL REFERENCES trailhead_zones(id) ON DELETE CASCADE,
    draw_date DATE NOT NULL,
    commitment_hash VARCHAR(64) NOT NULL,
    revealed_secret VARCHAR(255),
    published_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    drawn_at TIMESTAMPTZ,
    CONSTRAINT uq_lottery_commitment_trailhead_date UNIQUE (trailhead_id, draw_date)
);

CREATE INDEX IF NOT EXISTS idx_lottery_commitments_date
    ON lottery_draw_commitments (draw_date, trailhead_id);

-- Table: lottery_applications
-- Backcountry permit requests subject to provably fair cryptographic draw
CREATE TABLE IF NOT EXISTS lottery_applications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    full_name VARCHAR(255) NOT NULL,
    trailhead_id VARCHAR(64) NOT NULL REFERENCES trailhead_zones(id) ON DELETE CASCADE,
    target_date DATE NOT NULL,
    party_size INT NOT NULL CHECK (party_size > 0),
    leave_no_trace_cert_id VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'WON', 'UNSUCCESSFUL', 'CONFIRMED')),
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lottery_applications_draw
    ON lottery_applications (trailhead_id, target_date, status);

CREATE INDEX IF NOT EXISTS idx_lottery_applications_user
    ON lottery_applications (user_id);
