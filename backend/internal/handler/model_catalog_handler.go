package handler

import (
	"log"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type ModelCatalogHandler struct {
	modelCatalogService *service.ModelCatalogService
}

func NewModelCatalogHandler(modelCatalogService *service.ModelCatalogService) *ModelCatalogHandler {
	return &ModelCatalogHandler{modelCatalogService: modelCatalogService}
}

// GET /api/v1/models/catalog
func (h *ModelCatalogHandler) List(c *gin.Context) {
	if _, ok := middleware.GetAuthSubjectFromContext(c); !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	providers, err := h.modelCatalogService.ListCatalog(c.Request.Context())
	if err != nil {
		log.Printf("[ERROR] GET /api/v1/models/catalog failed: %v", err)
		response.InternalError(c, "Failed to load model catalog")
		return
	}

	response.Success(c, providers)
}
