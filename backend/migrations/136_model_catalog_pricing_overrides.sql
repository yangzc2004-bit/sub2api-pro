CREATE TABLE IF NOT EXISTS model_catalog_pricing_overrides (
    model_id VARCHAR(255) PRIMARY KEY,
    provider_id VARCHAR(64) NOT NULL,
    input_price_per_1m DOUBLE PRECISION,
    output_price_per_1m DOUBLE PRECISION,
    cache_write_price_per_1m DOUBLE PRECISION,
    cache_read_price_per_1m DOUBLE PRECISION,
    updated_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_model_catalog_pricing_overrides_provider
    ON model_catalog_pricing_overrides(provider_id);

