package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type modelCatalogRepository struct {
	db *sql.DB
}

func NewModelCatalogRepository(db *sql.DB) service.ModelCatalogRepository {
	return &modelCatalogRepository{db: db}
}

func (r *modelCatalogRepository) ListModelCatalogPricingOverrides(ctx context.Context) ([]service.ModelCatalogPricingOverride, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT provider_id, model_id, input_price_per_1m, output_price_per_1m,
		       cache_write_price_per_1m, cache_read_price_per_1m, updated_by, updated_at
		FROM model_catalog_pricing_overrides
		ORDER BY provider_id, model_id`)
	if err != nil {
		return nil, fmt.Errorf("list model catalog pricing overrides: %w", err)
	}
	defer func() { _ = rows.Close() }()

	overrides := make([]service.ModelCatalogPricingOverride, 0)
	for rows.Next() {
		override, err := scanModelCatalogPricingOverride(rows)
		if err != nil {
			return nil, err
		}
		overrides = append(overrides, override)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate model catalog pricing overrides: %w", err)
	}
	return overrides, nil
}

func (r *modelCatalogRepository) GetModelCatalogPricingOverride(ctx context.Context, modelID string) (*service.ModelCatalogPricingOverride, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT provider_id, model_id, input_price_per_1m, output_price_per_1m,
		       cache_write_price_per_1m, cache_read_price_per_1m, updated_by, updated_at
		FROM model_catalog_pricing_overrides
		WHERE lower(model_id) = lower($1)`, modelID)

	override, err := scanModelCatalogPricingOverride(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &override, nil
}

func (r *modelCatalogRepository) UpsertModelCatalogPricingOverride(ctx context.Context, override service.ModelCatalogPricingOverride) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO model_catalog_pricing_overrides (
			provider_id, model_id, input_price_per_1m, output_price_per_1m,
			cache_write_price_per_1m, cache_read_price_per_1m, updated_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (model_id) DO UPDATE SET
			provider_id = EXCLUDED.provider_id,
			input_price_per_1m = EXCLUDED.input_price_per_1m,
			output_price_per_1m = EXCLUDED.output_price_per_1m,
			cache_write_price_per_1m = EXCLUDED.cache_write_price_per_1m,
			cache_read_price_per_1m = EXCLUDED.cache_read_price_per_1m,
			updated_by = EXCLUDED.updated_by,
			updated_at = NOW()`,
		override.ProviderID,
		override.ModelID,
		nullableFloat64(override.Input),
		nullableFloat64(override.Output),
		nullableFloat64(override.CacheWrite),
		nullableFloat64(override.CacheRead),
		nullableInt64(override.UpdatedBy),
	)
	if err != nil {
		return fmt.Errorf("upsert model catalog pricing override: %w", err)
	}
	return nil
}

func (r *modelCatalogRepository) DeleteModelCatalogPricingOverrides(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM model_catalog_pricing_overrides`); err != nil {
		return fmt.Errorf("delete model catalog pricing overrides: %w", err)
	}
	return nil
}

type modelCatalogOverrideScanner interface {
	Scan(dest ...any) error
}

func scanModelCatalogPricingOverride(scanner modelCatalogOverrideScanner) (service.ModelCatalogPricingOverride, error) {
	var override service.ModelCatalogPricingOverride
	var input, output, cacheWrite, cacheRead sql.NullFloat64
	var updatedBy sql.NullInt64
	if err := scanner.Scan(
		&override.ProviderID,
		&override.ModelID,
		&input,
		&output,
		&cacheWrite,
		&cacheRead,
		&updatedBy,
		&override.UpdatedAt,
	); err != nil {
		return override, fmt.Errorf("scan model catalog pricing override: %w", err)
	}
	override.Input = modelCatalogNullFloat64Ptr(input)
	override.Output = modelCatalogNullFloat64Ptr(output)
	override.CacheWrite = modelCatalogNullFloat64Ptr(cacheWrite)
	override.CacheRead = modelCatalogNullFloat64Ptr(cacheRead)
	if updatedBy.Valid {
		value := updatedBy.Int64
		override.UpdatedBy = &value
	}
	return override, nil
}

func nullableFloat64(value *float64) sql.NullFloat64 {
	if value == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *value, Valid: true}
}

func nullableInt64(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

func modelCatalogNullFloat64Ptr(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	out := value.Float64
	return &out
}
