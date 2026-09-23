-- Topocosm Distributed Real-Time Booking and Lease Contention Engine Schema
-- Migration: 002_topo_lease_schema.sql
-- Description: Inventory holds table and audit event logs for high-concurrency cart leases.

-- Enable btree_gist extension if available for exclusion constraints
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- Table: inventory_holds
-- Manages short-lived cryptographic reservation holds with automatic expiration TTL
CREATE TABLE IF NOT EXISTS inventory_holds (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_id VARCHAR(64) UNIQUE NOT NULL,
    campsite_id UUID NOT NULL REFERENCES campsites(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'HELD', -- HELD, COMMITTED, EXPIRED, CANCELLED
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT valid_hold_dates CHECK (end_date > start_date),
    CONSTRAINT valid_hold_status CHECK (status IN ('HELD', 'COMMITTED', 'EXPIRED', 'CANCELLED'))
);

-- Fast lookup index for active campsite holds and availability queries
CREATE INDEX IF NOT EXISTS idx_inventory_holds_campsite_status
    ON inventory_holds (campsite_id, status, expires_at);

-- Fast lookup index for cryptographic token verification
CREATE INDEX IF NOT EXISTS idx_inventory_holds_token
    ON inventory_holds (token_id);

-- Sweeper partial index for finding expired holds
CREATE INDEX IF NOT EXISTS idx_inventory_holds_sweeper
    ON inventory_holds (status, expires_at)
    WHERE status = 'HELD';

-- Table: hold_audit_events
-- Immutable causal ledger tracking all lease lifecycle events (ACQUIRED, COMMITTED, RELEASED, EXPIRED)
CREATE TABLE IF NOT EXISTS hold_audit_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hold_id UUID REFERENCES inventory_holds(id) ON DELETE SET NULL,
    token_id VARCHAR(64) NOT NULL,
    campsite_id UUID NOT NULL,
    user_id UUID NOT NULL,
    action VARCHAR(32) NOT NULL, -- ACQUIRED, COMMITTED, RELEASED, EXPIRED
    details JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indices for audit trail analysis and user/campsite history queries
CREATE INDEX IF NOT EXISTS idx_hold_audit_events_token
    ON hold_audit_events (token_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_hold_audit_events_campsite
    ON hold_audit_events (campsite_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_hold_audit_events_user
    ON hold_audit_events (user_id, created_at DESC);
