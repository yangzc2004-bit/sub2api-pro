//go:build unit

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type adminModelCatalogRepo struct {
	overrides map[string]service.ModelCatalogPricingOverride
}

func newAdminModelCatalogRepo() *adminModelCatalogRepo {
	return &adminModelCatalogRepo{overrides: map[string]service.ModelCatalogPricingOverride{}}
}

func (r *adminModelCatalogRepo) ListModelCatalogPricingOverrides(_ context.Context) ([]service.ModelCatalogPricingOverride, error) {
	out := make([]service.ModelCatalogPricingOverride, 0, len(r.overrides))
	for _, override := range r.overrides {
		out = append(out, override)
	}
	return out, nil
}

func (r *adminModelCatalogRepo) GetModelCatalogPricingOverride(_ context.Context, modelID string) (*service.ModelCatalogPricingOverride, error) {
	override, ok := r.overrides[modelID]
	if !ok {
		return nil, nil
	}
	cp := override
	return &cp, nil
}

func (r *adminModelCatalogRepo) UpsertModelCatalogPricingOverride(_ context.Context, override service.ModelCatalogPricingOverride) error {
	r.overrides[override.ModelID] = override
	return nil
}

func (r *adminModelCatalogRepo) DeleteModelCatalogPricingOverrides(_ context.Context) error {
	r.overrides = map[string]service.ModelCatalogPricingOverride{}
	return nil
}

func TestAdminModelCatalog_UpdatePricing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newAdminModelCatalogRepo()
	h := NewModelCatalogHandler(service.NewModelCatalogService(repo))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "model_id", Value: "claude-opus-4-7"}}
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42, Concurrency: 1})
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/models/catalog/claude-opus-4-7/pricing", bytes.NewBufferString(`{
		"input": 9.5,
		"output": 31,
		"cache_write": null,
		"cache_read": 0.75
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdatePricing(c)

	require.Equal(t, http.StatusOK, w.Code)
	override, ok := repo.overrides["claude-opus-4-7"]
	require.True(t, ok)
	require.Equal(t, 9.5, *override.Input)
	require.Equal(t, 31.0, *override.Output)
	require.Nil(t, override.CacheWrite)
	require.Equal(t, 0.75, *override.CacheRead)
	require.NotNil(t, override.UpdatedBy)
	require.Equal(t, int64(42), *override.UpdatedBy)

	var payload struct {
		Data service.ModelCatalogModel `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Equal(t, "override", payload.Data.PricingSource)
}

func TestAdminModelCatalog_UpdatePricingUnknownModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewModelCatalogHandler(service.NewModelCatalogService(newAdminModelCatalogRepo()))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "model_id", Value: "unknown-model"}}
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42, Concurrency: 1})
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/models/catalog/unknown-model/pricing", bytes.NewBufferString(`{"input": 1}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdatePricing(c)

	require.Equal(t, http.StatusNotFound, w.Code)
}
