package handlers

import (
	"net/http"

	"iac-platform/internal/application/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TeamTokenHandler 团队Token处理器
type TeamTokenHandler struct {
	service *service.TeamTokenService
	db      *gorm.DB
}

// NewTeamTokenHandler 创建团队Token处理器实例
func NewTeamTokenHandler(svc *service.TeamTokenService) *TeamTokenHandler {
	return &TeamTokenHandler{service: svc}
}

// NewTeamTokenHandlerWithDB 带 db，用于 team→org 绑定
func NewTeamTokenHandlerWithDB(svc *service.TeamTokenService, db *gorm.DB) *TeamTokenHandler {
	return &TeamTokenHandler{service: svc, db: db}
}

// requireTeamInAuthOrg 校验 path team 属于鉴权 org
func (h *TeamTokenHandler) requireTeamInAuthOrg(c *gin.Context, teamID string) bool {
	authOrg, ok := requireAuthOrg(c)
	if !ok {
		return false
	}
	if h.db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "team org binding not configured"})
		return false
	}
	orgID, err := loadTeamOrgID(c.Request.Context(), h.db, teamID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "team not found"})
		return false
	}
	if err := ensureTeamBelongsToAuthOrg(orgID, authOrg); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "team not found"})
		return false
	}
	return true
}

// CreateTeamTokenRequest 创建团队Token请求
type CreateTeamTokenRequest struct {
	TokenName     string `json:"token_name" binding:"required"`
	ExpiresInDays int    `json:"expires_in_days"` // 默认/最大 1 天（24h），0 或 >1 按 1 天处理
}

// CreateTeamToken creates a team token
// @Summary Create team token
// @Description Create a new access token for a team
// @Tags IAM-Team
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Team ID"
// @Param request body CreateTeamTokenRequest true "Create token request"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
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

	if !h.requireTeamInAuthOrg(c, teamID) {
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

// ListTeamTokens lists all tokens for a team
// @Summary List team tokens
// @Description Get all tokens for a specific team
// @Tags IAM-Team
// @Produce json
// @Security BearerAuth
// @Param id path string true "Team ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/teams/{id}/tokens [get]
func (h *TeamTokenHandler) ListTeamTokens(c *gin.Context) {
	// 获取团队ID
	teamID := c.Param("id")

	if !h.requireTeamInAuthOrg(c, teamID) {
		return
	}

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

// RevokeTeamToken revokes a team token
// @Summary Revoke team token
// @Description Revoke a specific token for a team
// @Tags IAM-Team
// @Produce json
// @Security BearerAuth
// @Param id path string true "Team ID"
// @Param token_id path int true "Token ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /api/v1/iam/teams/{id}/tokens/{token_id} [delete]
func (h *TeamTokenHandler) RevokeTeamToken(c *gin.Context) {
	// 获取团队ID
	teamID := c.Param("id")

	// 获取当前用户ID（须先于 org 绑定，便于未登录返回 401）
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	if !h.requireTeamInAuthOrg(c, teamID) {
		return
	}

	// 路径参数 :token_id 实际为 token_name（列表接口标识，非数字主键）
	tokenName := c.Param("token_id")
	if tokenName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token_name is required"})
		return
	}

	// 吊销token（按 name）
	if err := h.service.RevokeTokenByName(c.Request.Context(), teamID, tokenName, userID.(string)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Token revoked successfully",
	})
}
