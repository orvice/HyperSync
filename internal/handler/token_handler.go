package handler

import (
	"net/http"

	"butterfly.orx.me/core/log"
	"github.com/gin-gonic/gin"
	"go.orx.me/apps/hyper-sync/internal/service"
)

// TokenHandler handles token management endpoints
type TokenHandler struct {
	schedulerService *service.SchedulerService
}

// NewTokenHandler creates a new token handler
func NewTokenHandler(schedulerService *service.SchedulerService) *TokenHandler {
	return &TokenHandler{
		schedulerService: schedulerService,
	}
}

// TokenStatusResponse represents the response for token status
type TokenStatusResponse struct {
	Success bool                 `json:"success"`
	Data    *service.TokenStatus `json:"data,omitempty"`
	Error   string               `json:"error,omitempty"`
}

// RefreshTokenResponse represents the response for token refresh
type RefreshTokenResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

// GetTokenStatus returns the token status for a platform
// GET /api/token/status/:platform
func (h *TokenHandler) GetTokenStatus(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())
	platform := c.Param("platform")

	if platform == "" {
		c.JSON(http.StatusBadRequest, TokenStatusResponse{
			Success: false,
			Error:   "platform parameter is required",
		})
		return
	}

	logger.Info("Getting token status", "platform", platform)

	status, err := h.schedulerService.GetTokenStatus(c.Request.Context(), platform)
	if err != nil {
		logger.Error("Failed to get token status", "platform", platform, "error", err)
		c.JSON(http.StatusInternalServerError, TokenStatusResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, TokenStatusResponse{
		Success: true,
		Data:    status,
	})
}

// RefreshToken manually refreshes the token for a platform
// POST /api/token/refresh/:platform
func (h *TokenHandler) RefreshToken(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())
	platform := c.Param("platform")

	if platform == "" {
		c.JSON(http.StatusBadRequest, RefreshTokenResponse{
			Success: false,
			Error:   "platform parameter is required",
		})
		return
	}

	logger.Info("Manually refreshing token", "platform", platform)

	err := h.schedulerService.RefreshThreadsTokenManually(c.Request.Context(), platform)
	if err != nil {
		logger.Error("Failed to refresh token", "platform", platform, "error", err)
		c.JSON(http.StatusInternalServerError, RefreshTokenResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, RefreshTokenResponse{
		Success: true,
		Message: "Token refreshed successfully",
	})
}

// RefreshAllTokens manually refreshes tokens for all platforms
// POST /api/token/refresh-all
func (h *TokenHandler) RefreshAllTokens(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())

	logger.Info("Manually refreshing all tokens")

	// 直接调用 RefreshAllTokens，这会检查所有平台的 token
	h.schedulerService.RefreshAllTokens(c.Request.Context())

	c.JSON(http.StatusOK, RefreshTokenResponse{
		Success: true,
		Message: "All tokens refresh check completed",
	})
}
