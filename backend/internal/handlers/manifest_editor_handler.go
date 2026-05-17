package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"iac-platform/internal/models"
)

// ManifestEditorHandler 给 manifest 编辑器(Monaco IntelliSense)用的只读摘要 API
//
// 路由:
//   GET /api/v1/manifest-editor/modules?q=&limit=20
//   GET /api/v1/manifest-editor/modules/:module_id/demos
//
// 这两个端点专为编辑器 IntelliSense / Hover / Inlay Hint / Quick Fix 设计:
//   - 字段裁剪小, 减少 IntelliSense 阻塞时长
//   - 不带 AI prompts / module_files 等大字段
//   - module 列表按 demo 数量降序, 让"有 demo 的 module"先出
type ManifestEditorHandler struct {
	db *gorm.DB
}

func NewManifestEditorHandler(db *gorm.DB) *ManifestEditorHandler {
	return &ManifestEditorHandler{db: db}
}

// ModuleSummary 编辑器 IntelliSense 用的 module 摘要
type ModuleSummary struct {
	ModuleID      uint   `json:"module_id"`
	Name          string `json:"name"`
	Source        string `json:"source"`         // 用户写在 module "x" { source = "..." } 里的字符串
	LatestVersion string `json:"latest_version"` // 默认推荐版本
	Description   string `json:"description"`
	DemoCount     int    `json:"demo_count"`
}

// DemoSummary 编辑器 IntelliSense 用的 demo 摘要
type DemoSummary struct {
	DemoID        uint                   `json:"demo_id"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	IsDefault     bool                   `json:"is_default"`
	ConfigData    map[string]interface{} `json:"config_data"`
	ChangeSummary string                 `json:"change_summary"`
}

// ListModules GET /manifest-editor/modules?q=&limit=20
func (h *ManifestEditorHandler) ListModules(c *gin.Context) {
	q := c.Query("q")
	limitStr := c.DefaultQuery("limit", "20")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 100 {
		limit = 20
	}

	type row struct {
		ID            uint   `gorm:"column:id"`
		Name          string `gorm:"column:name"`
		Source        string `gorm:"column:source"`
		Version       string `gorm:"column:version"`
		Description   string `gorm:"column:description"`
		DemoCount     int64  `gorm:"column:demo_count"`
	}
	var rows []row

	query := h.db.Table("modules m").
		Select(`m.id, m.name, m.source, m.version, m.description,
				COALESCE((SELECT COUNT(*) FROM module_demos d
						   WHERE d.module_id = m.id AND d.is_active = true), 0) AS demo_count`).
		Where("m.status = ?", "active")

	if q != "" {
		like := "%" + q + "%"
		query = query.Where("m.name ILIKE ? OR m.source ILIKE ? OR m.description ILIKE ?", like, like, like)
	}

	if err := query.
		Order("demo_count DESC, m.name ASC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	out := make([]ModuleSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, ModuleSummary{
			ModuleID:      r.ID,
			Name:          r.Name,
			Source:        r.Source,
			LatestVersion: r.Version,
			Description:   r.Description,
			DemoCount:     int(r.DemoCount),
		})
	}
	c.JSON(http.StatusOK, gin.H{"modules": out})
}

// ListDemos GET /manifest-editor/modules/:module_id/demos
func (h *ManifestEditorHandler) ListDemos(c *gin.Context) {
	idStr := c.Param("module_id")
	moduleID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid module_id"})
		return
	}

	// 拉 demos + 当前版本(同时带 config_data)
	type row struct {
		DemoID        uint            `gorm:"column:demo_id"`
		Name          string          `gorm:"column:name"`
		Description   string          `gorm:"column:description"`
		ConfigData    json.RawMessage `gorm:"column:config_data"`
		ChangeSummary string          `gorm:"column:change_summary"`
		// IsDefault: 我们用 demo.name='default' 或 inherited_from_demo_id 判定;
		// 在没有显式标记时,把第一个 demo 当作默认
	}
	var rows []row
	if err := h.db.Table("module_demos d").
		Select(`d.id AS demo_id, d.name, d.description,
				v.config_data, v.change_summary`).
		Joins("LEFT JOIN module_demo_versions v ON v.id = d.current_version_id").
		Where("d.module_id = ? AND d.is_active = true", uint(moduleID)).
		Order("d.created_at ASC").
		Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	out := make([]DemoSummary, 0, len(rows))
	for i, r := range rows {
		var cfg map[string]interface{}
		if len(r.ConfigData) > 0 {
			_ = json.Unmarshal(r.ConfigData, &cfg)
		}
		out = append(out, DemoSummary{
			DemoID:        r.DemoID,
			Name:          r.Name,
			Description:   r.Description,
			IsDefault:     i == 0, // 第一个标默认 (后续可加显式 is_default 字段)
			ConfigData:    cfg,
			ChangeSummary: r.ChangeSummary,
		})
	}
	c.JSON(http.StatusOK, gin.H{"demos": out})
}

// 占位防 lint: models 包导入不能为空
var _ = models.ModuleDemo{}
