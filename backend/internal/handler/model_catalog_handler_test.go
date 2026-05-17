//go:build unit

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelCatalog_Unauthenticated401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewModelCatalogHandler(service.NewModelCatalogService(nil))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/models/catalog", nil)

	h.List(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestModelCatalog_FieldShapeAndProviders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewModelCatalogHandler(service.NewModelCatalogService(nil))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1, Concurrency: 1})
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/models/catalog", nil)

	h.List(c)

	require.Equal(t, http.StatusOK, w.Code)

	var envelope response.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	raw, err := json.Marshal(envelope.Data)
	require.NoError(t, err)

	var providers []service.ModelCatalogProvider
	require.NoError(t, json.Unmarshal(raw, &providers))
	require.Len(t, providers, 6)

	ids := make([]string, 0, len(providers))
	for _, provider := range providers {
		ids = append(ids, provider.ID)
		require.NotEmpty(t, provider.Models)
		for _, model := range provider.Models {
			require.Equal(t, service.ModelCatalogPricingUnit, model.Pricing.Unit)
			require.Equal(t, "official", model.PricingSource)
		}
	}

	require.ElementsMatch(t, []string{"anthropic", "openai", "google", "deepseek", "moonshot", "mimo"}, ids)
	require.NotContains(t, ids, "qwen")
}
