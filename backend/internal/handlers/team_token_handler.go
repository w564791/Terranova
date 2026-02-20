package handlers

import (
	"net/http"
	"strconv"

	"iac-platform/internal/application/service"

	"github.com/gin-gonic/gin"
)

// TeamTokenHandler 团队Token处理器
type TeamTokenHandler struct {
	service *service.TeamTokenService
}

// NewTeamTokenHandler 创建团队Token处理器实例
func NewTeamTokenHandler(service *service.TeamTokenService) *TeamTokenHandler {
	return &TeamTokenHandler{
		service: service,
	}
}

// CreateTeamTokenRequest 创建团队Token请求
type CreateTeamTokenRequest struct {
	TokenName     string `json:"token_name" binding:"required"`
	ExpiresInDays int    `json:"expires_in_days"` // 0表示永不过期
}

// CreateTeamToken 创建团队Token
// @Summary 创建团队Token
// @Tags IAM-Team
// @Accept json
// @Produce json
// @Param id path int true "团队ID"
// @Param request body CreateTeamTokenRequest true "创建Token请求"
// @Success 201 {object} map[string]interface{}
// @Router /api/v1/iam/teams/{id}/tokens [post]
func (h *TeamTokenHandler) CreateTeamToken(c *gin.Context) {
	// 获取团队ID
	teamID := c.Param("id")

	// 获取当前用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 解析请求
	var req CreateTeamTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 创建token
	token, err := h.service.GenerateToken(c.Request.Context(), teamID, req.TokenName, userID.(string), req.ExpiresInDays)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Token created successfully. Please copy it now as it won't be shown again.",
		"token":   token,
	})
}

// ListTeamTokens 列出团队的所有Token
// @Summary 列出团队Token
// @Tags IAM-Team
// @Produce json
// @Param id path int true "团队ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/teams/{id}/tokens [get]
func (h *TeamTokenHandler) ListTeamTokens(c *gin.Context) {
	// 获取团队ID
	teamID := c.Param("id")

	// 获取token列表
	tokens, err := h.service.ListTeamTokens(c.Request.Context(), teamID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tokens": tokens,
	})
}

// RevokeTeamToken 吊销团队Token
// @Summary 吊销团队Token
// @Tags IAM-Team
// @Produce json
// @Param id path int true "团队ID"
// @Param token_id path int true "Token ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/teams/{id}/tokens/{token_id} [delete]
func (h *TeamTokenHandler) RevokeTeamToken(c *gin.Context) {
	// 获取团队ID
	teamID := c.Param("id")

	// 获取Token ID
	tokenIDStr := c.Param("token_id")
	tokenID, err := strconv.ParseUint(tokenIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid token id"})
		return
	}

	// 获取当前用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 吊销token
	if err := h.service.RevokeToken(c.Request.Context(), teamID, uint(tokenID), userID.(string)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Token revoked successfully",
	})
}
