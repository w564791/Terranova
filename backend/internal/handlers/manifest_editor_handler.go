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
		ID          uint   `gorm:"column:id"`
		Name        string `gorm:"column:name"`
		Source      string `gorm:"column:source"`
		Version     string `gorm:"column:version"`
		Description string `gorm:"column:description"`
		DemoCount   int64  `gorm:"column:demo_count"`
	}
	var rows []row

	// source 用 module_source(真实 terraform source,如 terraform-aws-modules/ec2-instance/aws),
	// 不是 modules.source(那是导入方式标识 tf-file-import)。module_source 为空时回退 source。
	query := h.db.Table("modules m").
		Select(`m.id, m.name,
				COALESCE(NULLIF(m.module_source, ''), m.source) AS source,
				m.version, m.description,
				COALESCE((SELECT COUNT(*) FROM module_demos d
						   WHERE d.module_id = m.id AND d.is_active = true), 0) AS demo_count`).
		Where("m.status = ?", "active")

	if q != "" {
		like := "%" + q + "%"
		query = query.Where("m.name ILIKE ? OR m.module_source ILIKE ? OR m.source ILIKE ? OR m.description ILIKE ?", like, like, like, like)
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

// ModuleInputField 编辑器补全用的 module 输入变量定义
type ModuleInputField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`        // string / number / boolean / object / array
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Default     string `json:"default,omitempty"` // 有默认值时的展示字符串(原样,仅提示)
}

// ListModuleInputs GET /manifest-editor/modules/:module_id/inputs
//
// 从 module 活跃 schema 的 OpenAPI 定义提取输入变量(name/type/required/description),
// 供编辑器在 module 块内做属性补全(Tier3)。数据源与输出提示同款:
// components.schemas.ModuleInput.properties + required 数组。
func (h *ManifestEditorHandler) ListModuleInputs(c *gin.Context) {
	idStr := c.Param("module_id")
	moduleID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid module_id"})
		return
	}

	// 活跃 schema, 优先 v2
	var schema models.Schema
	if err := h.db.Where("module_id = ? AND status = ?", uint(moduleID), "active").
		Order("CASE WHEN schema_version = 'v2' THEN 0 ELSE 1 END, created_at DESC").
		First(&schema).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusOK, gin.H{"inputs": []ModuleInputField{}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"inputs": extractModuleInputs(schema.OpenAPISchema)})
}

// extractModuleInputs 从 OpenAPI schema 的 components.schemas.ModuleInput 提取输入变量。
func extractModuleInputs(schema models.JSONB) []ModuleInputField {
	out := []ModuleInputField{}
	components, ok := schema["components"].(map[string]interface{})
	if !ok {
		return out
	}
	schemas, ok := components["schemas"].(map[string]interface{})
	if !ok {
		return out
	}
	moduleInput, ok := schemas["ModuleInput"].(map[string]interface{})
	if !ok {
		return out
	}

	// required 名单
	requiredSet := map[string]bool{}
	if reqArr, ok := moduleInput["required"].([]interface{}); ok {
		for _, r := range reqArr {
			if s, ok := r.(string); ok {
				requiredSet[s] = true
			}
		}
	}

	props, ok := moduleInput["properties"].(map[string]interface{})
	if !ok {
		return out
	}
	for name, raw := range props {
		p, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		field := ModuleInputField{Name: name, Required: requiredSet[name]}
		if t, ok := p["type"].(string); ok {
			field.Type = t
		}
		if d, ok := p["description"].(string); ok {
			field.Description = d
		}
		if dv, ok := p["default"]; ok {
			if b, mErr := json.Marshal(dv); mErr == nil {
				field.Default = string(b)
			}
		}
		out = append(out, field)
	}
	return out
}

// 占位防 lint: models 包导入不能为空
var _ = models.ModuleDemo{}
