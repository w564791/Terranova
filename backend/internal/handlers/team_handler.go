package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/entity"
)

// TeamHandler 团队管理Handler
type TeamHandler struct {
	teamService service.TeamService
}

// NewTeamHandler 创建团队管理Handler实例
func NewTeamHandler(teamService service.TeamService) *TeamHandler {
	return &TeamHandler{
		teamService: teamService,
	}
}

// CreateTeamRequest 创建团队请求
type CreateTeamRequest struct {
	OrgID       uint   `json:"org_id" binding:"required"`
	Name        string `json:"name" binding:"required"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
}

// CreateTeam 创建团队
// @Summary 创建团队
// @Tags Team
// @Accept json
// @Produce json
// @Param request body CreateTeamRequest true "创建团队请求"
// @Success 200 {object} entity.Team
// @Router /api/v1/teams [post]
func (h *TeamHandler) CreateTeam(c *gin.Context) {
	var req CreateTeamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取当前用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 创建团队
	createReq := &service.CreateTeamRequest{
		OrgID:       req.OrgID,
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		CreatedBy:   userID.(string),
	}

	team, err := h.teamService.CreateTeam(c.Request.Context(), createReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, team)
}

// GetTeam 获取团队详情
// @Summary 获取团队详情
// @Tags Team
// @Param id path string true "团队ID"
// @Success 200 {object} entity.Team
// @Router /api/v1/teams/{id} [get]
func (h *TeamHandler) GetTeam(c *gin.Context) {
	teamID := c.Param("id")

	team, err := h.teamService.GetTeam(c.Request.Context(), teamID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, team)
}

// ListTeamsByOrg 列出组织的所有团队
// @Summary 列出组织的所有团队
// @Tags Team
// @Param org_id query int true "组织ID"
// @Success 200 {array} entity.Team
// @Router /api/v1/teams [get]
func (h *TeamHandler) ListTeamsByOrg(c *gin.Context) {
	orgIDStr := c.Query("org_id")
	if orgIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "org_id is required"})
		return
	}

	orgID, err := strconv.ParseUint(orgIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid org_id"})
		return
	}

	teams, err := h.teamService.ListTeamsByOrg(c.Request.Context(), uint(orgID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"teams": teams,
		"total": len(teams),
	})
}

// DeleteTeam 删除团队
// @Summary 删除团队
// @Tags Team
// @Param id path string true "团队ID"
// @Success 200 {object} map[string]string
// @Router /api/v1/teams/{id} [delete]
func (h *TeamHandler) DeleteTeam(c *gin.Context) {
	teamID := c.Param("id")

	// 获取当前用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	if err := h.teamService.DeleteTeam(c.Request.Context(), teamID, userID.(string)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Team deleted successfully"})
}

// AddTeamMemberRequest 添加团队成员请求
type AddTeamMemberRequest struct {
	UserID string `json:"user_id" binding:"required"`
	Role   string `json:"role" binding:"required"` // MEMBER/MAINTAINER
}

// AddTeamMember 添加团队成员
// @Summary 添加团队成员
// @Tags Team
// @Param id path int true "团队ID"
// @Param request body AddTeamMemberRequest true "添加成员请求"
// @Success 200 {object} map[string]string
// @Router /api/v1/teams/{id}/members [post]
func (h *TeamHandler) AddTeamMember(c *gin.Context) {
	teamID := c.Param("id")

	var req AddTeamMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取当前用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// 解析角色
	role := entity.TeamRole(req.Role)
	if !role.IsValid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}

	// 添加成员
	addReq := &service.AddTeamMemberRequest{
		TeamID:  teamID,
		UserID:  req.UserID,
		Role:    role,
		AddedBy: userID.(string),
	}

	if err := h.teamService.AddTeamMember(c.Request.Context(), addReq); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Team member added successfully"})
}

// RemoveTeamMember 移除团队成员
// @Summary 移除团队成员
// @Tags Team
// @Param id path int true "团队ID"
// @Param user_id path int true "用户ID"
// @Success 200 {object} map[string]string
// @Router /api/v1/teams/{id}/members/{user_id} [delete]
func (h *TeamHandler) RemoveTeamMember(c *gin.Context) {
	teamID := c.Param("id")
	memberUserID := c.Param("user_id")

	// 获取当前用户ID
	currentUserID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	if err := h.teamService.RemoveTeamMember(c.Request.Context(), teamID, memberUserID, currentUserID.(string)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Team member removed successfully"})
}

// ListTeamMembers 列出团队成员
// @Summary 列出团队成员
// @Tags Team
// @Param id path string true "团队ID"
// @Success 200 {array} entity.TeamMember
// @Router /api/v1/teams/{id}/members [get]
func (h *TeamHandler) ListTeamMembers(c *gin.Context) {
	teamID := c.Param("id")

	members, err := h.teamService.ListTeamMembers(c.Request.Context(), teamID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"members": members,
		"total":   len(members),
	})
}
