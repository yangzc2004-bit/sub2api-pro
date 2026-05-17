package admin

import (
	"errors"
	"log"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type ModelCatalogHandler struct {
	modelCatalogService *service.ModelCatalogService
}

type updateModelCatalogPricingRequest struct {
	Input      *float64 `json:"input" binding:"omitempty,min=0"`
	Output     *float64 `json:"output" binding:"omitempty,min=0"`
	CacheWrite *float64 `json:"cache_write" binding:"omitempty,min=0"`
	CacheRead  *float64 `json:"cache_read" binding:"omitempty,min=0"`
}

func NewModelCatalogHandler(modelCatalogService *service.ModelCatalogService) *ModelCatalogHandler {
	return &ModelCatalogHandler{modelCatalogService: modelCatalogService}
}

func (h *ModelCatalogHandler) List(c *gin.Context) {
	providers, err := h.modelCatalogService.ListCatalog(c.Request.Context())
	if err != nil {
		log.Printf("[ERROR] GET /api/v1/admin/models/catalog failed: %v", err)
		response.InternalError(c, "Failed to load model catalog")
		return
	}
	response.Success(c, providers)
}

func (h *ModelCatalogHandler) UpdatePricing(c *gin.Context) {
	var req updateModelCatalogPricingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid pricing payload")
		return
	}

	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	updatedBy := subject.UserID
	model, err := h.modelCatalogService.UpdatePricing(c.Request.Context(), c.Param("model_id"), service.ModelCatalogPricingUpdate{
		Input:      req.Input,
		Output:     req.Output,
		CacheWrite: req.CacheWrite,
		CacheRead:  req.CacheRead,
		UpdatedBy:  &updatedBy,
	})
	if err != nil {
		if errors.Is(err, service.ErrModelCatalogModelNotFound) {
			response.NotFound(c, "Model not found")
			return
		}
		log.Printf("[ERROR] PUT /api/v1/admin/models/catalog/%s/pricing failed: %v", c.Param("model_id"), err)
		response.InternalError(c, "Failed to update model pricing")
		return
	}

	response.Success(c, model)
}

func (h *ModelCatalogHandler) ResetPricing(c *gin.Context) {
	if err := h.modelCatalogService.ResetPricingOverrides(c.Request.Context()); err != nil {
		log.Printf("[ERROR] POST /api/v1/admin/models/catalog/reset failed: %v", err)
		response.InternalError(c, "Failed to reset model pricing")
		return
	}
	response.Success(c, gin.H{"message": "Model catalog pricing reset successfully"})
}
