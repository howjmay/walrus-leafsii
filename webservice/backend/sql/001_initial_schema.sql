-- +goose Up
-- +goose StatementBegin

-- Protocol state snapshots
CREATE TABLE protocol_snapshots (
    at timestamptz PRIMARY KEY,
    cr numeric(20,8) NOT NULL,
    cr_target numeric(20,8) NOT NULL,
    reserves_r numeric(30,8) NOT NULL,
    supply_f numeric(30,8) NOT NULL,
    supply_x numeric(30,8) NOT NULL,
    peg_deviation numeric(10,8) NOT NULL,
    mode text NOT NULL DEFAULT 'normal',
    oracle_age_sec bigint NOT NULL DEFAULT 0
);

-- Events from the blockchain
CREATE TABLE events (
    id bigserial PRIMARY KEY,
    checkpoint bigint NOT NULL,
    sequence_number bigint NOT NULL,
    ts timestamptz NOT NULL,
    type text NOT NULL, -- MINT|REDEEM|STAKE|UNSTAKE|CLAIM|REBALANCE
    tx_digest text NOT NULL,
    sender text,
    fields jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_events_checkpoint ON events(checkpoint);
CREATE INDEX idx_events_type ON events(type);
CREATE INDEX idx_events_sender ON events(sender);
CREATE INDEX idx_events_ts ON events(ts);
CREATE INDEX idx_events_fields_address ON events USING gin ((fields->>'address'));

-- User positions (current state)
CREATE TABLE user_positions (
    address text PRIMARY KEY,
    balance_f numeric(30,8) NOT NULL DEFAULT 0,
    balance_x numeric(30,8) NOT NULL DEFAULT 0,
    balance_r numeric(30,8) NOT NULL DEFAULT 0,
    stake_f numeric(30,8) NOT NULL DEFAULT 0,
    index_at_join numeric(20,8) NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Stability Pool index history
CREATE TABLE sp_index (
    at timestamptz PRIMARY KEY,
    index_value numeric(20,8) NOT NULL,
    tvl_f numeric(30,8) NOT NULL,
    total_rewards_r numeric(30,8) NOT NULL DEFAULT 0
);

-- Rebalance events (detailed view)
CREATE TABLE rebalance_events (
    id bigserial PRIMARY KEY,
    ts timestamptz NOT NULL,
    checkpoint bigint NOT NULL,
    f_burn numeric(30,8) NOT NULL,
    payout_r numeric(30,8) NOT NULL,
    index_delta numeric(20,8) NOT NULL,
    pre_cr numeric(20,8) NOT NULL,
    post_cr numeric(20,8) NOT NULL,
    tx_digest text NOT NULL
);

CREATE INDEX idx_rebalance_events_ts ON rebalance_events(ts);

-- Oracle price data
CREATE TABLE oracle_prices (
    symbol text NOT NULL,
    price numeric(20,8) NOT NULL,
    updated_at timestamptz NOT NULL,
    source text NOT NULL,
    PRIMARY KEY (symbol, source)
);

CREATE INDEX idx_oracle_prices_updated_at ON oracle_prices(updated_at);

-- Quote cache (for tracking quote IDs and preventing replay)
CREATE TABLE quote_cache (
    quote_id text PRIMARY KEY,
    quote_type text NOT NULL, -- mint|redeem|swap
    params jsonb NOT NULL,
    result jsonb NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_quote_cache_expires_at ON quote_cache(expires_at);

-- Indexer state (track last processed checkpoint)
CREATE TABLE indexer_state (
    name text PRIMARY KEY, -- 'events', 'protocol_snapshots', etc.
    last_checkpoint bigint NOT NULL DEFAULT 0,
    last_sequence_number bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Insert initial indexer state
INSERT INTO indexer_state (name, last_checkpoint, last_sequence_number) 
VALUES ('events', 0, 0), ('protocol_snapshots', 0, 0);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS indexer_state;
DROP TABLE IF EXISTS quote_cache;
DROP TABLE IF EXISTS oracle_prices;
DROP TABLE IF EXISTS rebalance_events;
DROP TABLE IF EXISTS sp_index;
DROP TABLE IF EXISTS user_positions;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS protocol_snapshots;

-- +goose StatementEnd