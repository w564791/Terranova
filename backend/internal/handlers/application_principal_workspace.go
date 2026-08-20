package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/entity"
	"iac-platform/internal/middleware"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ApplicationWorkspaceHandler Application 主体的工作区只读 API
type ApplicationWorkspaceHandler struct {
	db    *gorm.DB
	wsSvc *services.WorkspaceService
	iam   *middleware.IAMPermissionMiddleware
}

// NewApplicationWorkspaceHandler 创建
func NewApplicationWorkspaceHandler(db *gorm.DB, iam *middleware.IAMPermissionMiddleware) *ApplicationWorkspaceHandler {
	return &ApplicationWorkspaceHandler{
		db:    db,
		wsSvc: services.NewWorkspaceService(db),
		iam:   iam,
	}
}

// appTagFilterFromContext 读取 AgentAuth 注入的 Application.workspace_tag_filter
func appTagFilterFromContext(c *gin.Context) map[string]interface{} {
	raw, ok := c.Get("application")
	if !ok || raw == nil {
		return nil
	}
	switch app := raw.(type) {
	case *entity.Application:
		return service.NormalizeWorkspaceTagFilter(app.WorkspaceTagFilter)
	case entity.Application:
		return service.NormalizeWorkspaceTagFilter(app.WorkspaceTagFilter)
	default:
		return nil
	}
}

// parseWSTags 从 JSON 文本解析 tags
func parseWSTags(raw string) map[string]interface{} {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil
	}
	return m
}

// ListWorkspaces 列出鉴权 org 下、且满足 Application tag 过滤的工作区
// 须 WORKSPACES READ @ ORGANIZATION；再按 org + workspace_tag_filter 收窄。
// @Summary List workspaces for Application principal (tag-filtered)
// @Tags Application-Principal
// @Produce json
// @Param page query int false "page" default(1)
// @Param size query int false "size" default(20)
// @Param search query string false "search"
// @Success 200 {object} map[string]interface{}
// @Security AppKeyAuth
// @Router /api/v1/app/workspaces [get]
func (h *ApplicationWorkspaceHandler) ListWorkspaces(c *gin.Context) {
	authOrg, ok := requireAuthOrg(c)
	if !ok {
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	search := c.Query("search")
	tagFilter := appTagFilterFromContext(c)

	// 拉取 org 内候选（含 tags），再内存 tag 过滤后分页
	// 说明：tags 在 JSONB，跨 PG/SQLite 统一在应用层匹配更稳妥
	type wsRow struct {
		ID               uint       `json:"id" gorm:"column:id"`
		WorkspaceID      string     `json:"workspace_id" gorm:"column:workspace_id"`
		Name             string     `json:"name" gorm:"column:name"`
		Description      string     `json:"description" gorm:"column:description"`
		ExecutionMode    string     `json:"execution_mode" gorm:"column:execution_mode"`
		TerraformVersion string     `json:"terraform_version" gorm:"column:terraform_version"`
		State            string     `json:"state" gorm:"column:state"`
		TagsRaw          string     `json:"-" gorm:"column:tags"` // JSON text (PG jsonb / SQLite text)
		CreatedAt        *time.Time `json:"created_at" gorm:"column:created_at"`
		UpdatedAt        *time.Time `json:"updated_at" gorm:"column:updated_at"`
	}

	q := h.db.Table("workspaces w").
		Select("w.id, w.workspace_id, w.name, w.description, w.execution_mode, w.terraform_version, w.state, w.tags, w.updated_at, w.created_at").
		Joins(`JOIN workspace_project_relations wpr ON wpr.workspace_id = w.workspace_id`).
		Joins(`JOIN projects p ON p.id = wpr.project_id`).
		Where("p.org_id = ?", authOrg)
	if search != "" {
		like := "%" + search + "%"
		q = q.Where("w.name LIKE ? OR w.description LIKE ?", like, like)
	}

	var rows []wsRow
	if err := q.Order("w.updated_at DESC").Find(&rows).Error; err != nil {
		// 降级：无 tags/updated_at
		if err2 := h.db.Table("workspaces w").
			Select("w.id, w.workspace_id, w.name, w.description").
			Joins(`JOIN workspace_project_relations wpr ON wpr.workspace_id = w.workspace_id`).
			Joins(`JOIN projects p ON p.id = wpr.project_id`).
			Where("p.org_id = ?", authOrg).
			Find(&rows).Error; err2 != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list workspaces", "details": err.Error()})
			return
		}
	}

	filtered := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		tags := parseWSTags(r.TagsRaw)
		if !service.WorkspaceTagsMatchFilter(tags, tagFilter) {
			continue
		}
		filtered = append(filtered, gin.H{
			"id":                r.ID,
			"workspace_id":      r.WorkspaceID,
			"name":              r.Name,
			"description":       r.Description,
			"execution_mode":    r.ExecutionMode,
			"terraform_version": r.TerraformVersion,
			"state":             r.State,
			"tags":              tags,
			"created_at":        r.CreatedAt,
			"updated_at":        r.UpdatedAt,
		})
	}

	total := int64(len(filtered))
	offset := (page - 1) * size
	if offset > len(filtered) {
		offset = len(filtered)
	}
	end := offset + size
	if end > len(filtered) {
		end = len(filtered)
	}
	pageItems := filtered[offset:end]

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"items": pageItems,
			"total": total,
			"page":  page,
			"size":  size,
		},
		"auth_org_id":          authOrg,
		"workspace_tag_filter": tagFilter,
		"timestamp":            time.Now(),
	})
}

// GetWorkspace 获取单个工作区详情（org 归属 + IAM + tag 匹配）
// @Summary Get workspace for Application principal
// @Tags Application-Principal
// @Produce json
// @Param id path string true "workspace id or semantic id"
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Security AppKeyAuth
// @Router /api/v1/app/workspaces/{id} [get]
func (h *ApplicationWorkspaceHandler) GetWorkspace(c *gin.Context) {
	authOrg, ok := requireAuthOrg(c)
	if !ok {
		return
	}
	wsID := c.Param("id")
	if wsID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace id required"})
		return
	}

	ws, err := h.wsSvc.GetWorkspaceByID(wsID)
	if err != nil || ws == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}

	// org 归属
	if err := ensureWorkspaceSemanticInAuthOrg(c.Request.Context(), h.db, ws.WorkspaceID, authOrg); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}

	// tag 匹配（不匹配 → 404，防探测）
	tagFilter := appTagFilterFromContext(c)
	wsTags := map[string]interface{}(ws.Tags)
	if !service.WorkspaceTagsMatchFilter(wsTags, tagFilter) {
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}

	// IAM：org WORKSPACES READ 或 ws MANAGEMENT READ
	if h.iam == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "IAM not configured"})
		return
	}
	if _, _, _, ok := principalFromAppContext(c); !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "application principal missing"})
		return
	}
	if !h.iam.CheckWorkspaceOrOrgWorkspacesRead(c, ws.WorkspaceID) {
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"id":                ws.ID,
			"workspace_id":      ws.WorkspaceID,
			"name":              ws.Name,
			"description":       ws.Description,
			"execution_mode":    ws.ExecutionMode,
			"terraform_version": ws.TerraformVersion,
			"state":             ws.State,
			"tags":              wsTags,
			"created_at":        ws.CreatedAt,
			"updated_at":        ws.UpdatedAt,
		},
		"auth_org_id":          authOrg,
		"workspace_tag_filter": tagFilter,
		"timestamp":            time.Now(),
	})
}
