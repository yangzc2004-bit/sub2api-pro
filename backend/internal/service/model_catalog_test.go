package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type memoryModelCatalogRepository struct {
	overrides map[string]ModelCatalogPricingOverride
}

func newMemoryModelCatalogRepository(overrides ...ModelCatalogPricingOverride) *memoryModelCatalogRepository {
	repo := &memoryModelCatalogRepository{overrides: map[string]ModelCatalogPricingOverride{}}
	for _, override := range overrides {
		repo.overrides[normalizeCatalogModelID(override.ModelID)] = override
	}
	return repo
}

func (r *memoryModelCatalogRepository) ListModelCatalogPricingOverrides(_ context.Context) ([]ModelCatalogPricingOverride, error) {
	out := make([]ModelCatalogPricingOverride, 0, len(r.overrides))
	for _, override := range r.overrides {
		out = append(out, override)
	}
	return out, nil
}

func (r *memoryModelCatalogRepository) GetModelCatalogPricingOverride(_ context.Context, modelID string) (*ModelCatalogPricingOverride, error) {
	override, ok := r.overrides[normalizeCatalogModelID(modelID)]
	if !ok {
		return nil, nil
	}
	cp := override
	return &cp, nil
}

func (r *memoryModelCatalogRepository) UpsertModelCatalogPricingOverride(_ context.Context, override ModelCatalogPricingOverride) error {
	r.overrides[normalizeCatalogModelID(override.ModelID)] = override
	return nil
}

func (r *memoryModelCatalogRepository) DeleteModelCatalogPricingOverrides(_ context.Context) error {
	r.overrides = map[string]ModelCatalogPricingOverride{}
	return nil
}

func TestModelCatalogService_ListCatalogAppliesOverrides(t *testing.T) {
	input := 7.5
	output := 31.25
	svc := NewModelCatalogService(newMemoryModelCatalogRepository(ModelCatalogPricingOverride{
		ProviderID: "anthropic",
		ModelID:    "claude-opus-4-7",
		Input:      &input,
		Output:     &output,
	}))

	providers, err := svc.ListCatalog(context.Background())
	require.NoError(t, err)

	var found *ModelCatalogModel
	for pIdx := range providers {
		for mIdx := range providers[pIdx].Models {
			if providers[pIdx].Models[mIdx].ID == "claude-opus-4-7" {
				found = &providers[pIdx].Models[mIdx]
			}
		}
	}
	require.NotNil(t, found)
	require.Equal(t, "override", found.PricingSource)
	require.Equal(t, input, *found.Pricing.Input)
	require.Equal(t, output, *found.Pricing.Output)
}

func TestModelCatalogDefaults_DeepSeekAndCachePrices(t *testing.T) {
	providers := defaultModelCatalogProviders()

	byModel := make(map[string]ModelCatalogModel)
	for _, provider := range providers {
		for _, model := range provider.Models {
			byModel[model.ID] = model
		}
	}

	require.NotContains(t, byModel, "deepseek-chat")
	require.NotContains(t, byModel, "deepseek-reasoner")

	requireCatalogPrice(t, byModel["deepseek-v4-flash"], 1.15, 2.5, 1.15, 0.03)
	requireCatalogPrice(t, byModel["deepseek-v4-pro"], 4, 7.5, 4, 0.035)
	require.Equal(t, 1000000, byModel["deepseek-v4-flash"].ContextTokens)
	require.Equal(t, 1000000, byModel["deepseek-v4-pro"].ContextTokens)
	requireCatalogPrice(t, byModel["gpt-5.5"], 5, 30, 5, 0.5)
	requireCatalogPrice(t, byModel["gemini-3.1-pro-high"], 2, 12, 2, 0.2)
	requireCatalogPrice(t, byModel["kimi-k2.6-full"], 4, 16, 4, 0.8)
	requireCatalogPrice(t, byModel["kimi-for-coding"], 2, 8, 2, 0.4)
	requireCatalogPrice(t, byModel["mimo-v2.5-pro"], 4, 16, 4, 0.8)
	requireCatalogPrice(t, byModel["mimo-v2-flash"], 0.8, 2.4, 0.8, 0.16)
	require.Equal(t, 1000000, byModel["mimo-v2.5-pro"].ContextTokens)
	require.Equal(t, 1000000, byModel["mimo-v2.5"].ContextTokens)
	require.Equal(t, 256000, byModel["mimo-v2-omni"].ContextTokens)
	require.Equal(t, 256000, byModel["mimo-v2-flash"].ContextTokens)
}

func TestModelPricingResolver_UsesCatalogOverrideBeforeLiteLLM(t *testing.T) {
	input := 8.0
	output := 40.0
	catalogSvc := NewModelCatalogService(newMemoryModelCatalogRepository(ModelCatalogPricingOverride{
		ProviderID: "anthropic",
		ModelID:    "claude-opus-4-7",
		Input:      &input,
		Output:     &output,
	}))
	billingSvc := NewBillingService(&config.Config{}, nil)
	resolver := NewModelPricingResolver(nil, billingSvc, catalogSvc)

	resolved := resolver.Resolve(context.Background(), PricingInput{Model: "claude-opus-4-7"})

	require.NotNil(t, resolved)
	require.Equal(t, PricingSourceModelCatalog, resolved.Source)
	require.NotNil(t, resolved.BasePricing)
	require.Equal(t, input/1_000_000, resolved.BasePricing.InputPricePerToken)
	require.Equal(t, output/1_000_000, resolved.BasePricing.OutputPricePerToken)
}

func requireCatalogPrice(t *testing.T, model ModelCatalogModel, input, output, cacheWrite, cacheRead float64) {
	t.Helper()
	require.NotEmpty(t, model.ID)
	require.NotNil(t, model.Pricing.Input)
	require.NotNil(t, model.Pricing.Output)
	require.NotNil(t, model.Pricing.CacheWrite)
	require.NotNil(t, model.Pricing.CacheRead)
	require.Equal(t, input, *model.Pricing.Input)
	require.Equal(t, output, *model.Pricing.Output)
	require.Equal(t, cacheWrite, *model.Pricing.CacheWrite)
	require.Equal(t, cacheRead, *model.Pricing.CacheRead)
}
