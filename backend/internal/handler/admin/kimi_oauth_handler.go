package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type KimiOAuthHandler struct {
	kimiOAuthService *service.KimiOAuthService
	adminService     service.AdminService
}

func NewKimiOAuthHandler(kimiOAuthService *service.KimiOAuthService, adminService service.AdminService) *KimiOAuthHandler {
	return &KimiOAuthHandler{
		kimiOAuthService: kimiOAuthService,
		adminService:     adminService,
	}
}

type KimiGenerateAuthURLRequest struct {
	ProxyID *int64 `json:"proxy_id"`
}

func (h *KimiOAuthHandler) GenerateAuthURL(c *gin.Context) {
	var req KimiGenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = KimiGenerateAuthURLRequest{}
	}
	result, err := h.kimiOAuthService.GenerateAuthURL(c.Request.Context(), req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

type KimiExchangeCodeRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	ProxyID   *int64 `json:"proxy_id"`
}

func (h *KimiOAuthHandler) ExchangeCode(c *gin.Context) {
	var req KimiExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	tokenInfo, err := h.kimiOAuthService.ExchangeDeviceCode(c.Request.Context(), req.SessionID, req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}

type KimiRefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
	RT           string `json:"rt"`
	ProxyID      *int64 `json:"proxy_id"`
}

func (h *KimiOAuthHandler) RefreshToken(c *gin.Context) {
	var req KimiRefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	refreshToken := strings.TrimSpace(req.RefreshToken)
	if refreshToken == "" {
		refreshToken = strings.TrimSpace(req.RT)
	}
	if refreshToken == "" {
		response.BadRequest(c, "refresh_token is required")
		return
	}

	var proxyURL string
	if req.ProxyID != nil {
		proxy, err := h.adminService.GetProxy(c.Request.Context(), *req.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
		}
	}
	tokenInfo, err := h.kimiOAuthService.RefreshToken(c.Request.Context(), refreshToken, proxyURL)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}

func (h *KimiOAuthHandler) RefreshAccountToken(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if account.Platform != service.PlatformKimi || account.Type != service.AccountTypeOAuth {
		response.BadRequest(c, "Account is not a Kimi OAuth account")
		return
	}
	tokenInfo, err := h.kimiOAuthService.RefreshAccountToken(c.Request.Context(), account)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	newCredentials := h.kimiOAuthService.BuildAccountCredentials(tokenInfo)
	for k, v := range account.Credentials {
		if _, exists := newCredentials[k]; !exists {
			newCredentials[k] = v
		}
	}
	updatedAccount, err := h.adminService.UpdateAccount(c.Request.Context(), accountID, &service.UpdateAccountInput{
		Credentials: newCredentials,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(updatedAccount))
}
