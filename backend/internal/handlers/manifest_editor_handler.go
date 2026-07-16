package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

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
// @Summary List modules for editor
// @Description Lightweight module list for manifest editor IntelliSense
// @Tags Manifest Editor
// @Accept json
// @Produce json
// @Param q query string false "Search query (name, source, description)"
// @Param limit query int false "Max results" default(20)
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/manifest-editor/modules [get]
// @Security BearerAuth
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
						   WHERE d.module_id = m.id AND d.is_active = true
						     AND d.module_version_id = m.default_version_id), 0) AS demo_count`).
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
// @Summary List module demos for editor
// @Description List demos of a module's default version for editor IntelliSense
// @Tags Manifest Editor
// @Accept json
// @Produce json
// @Param module_id path int true "Module ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/manifest-editor/modules/{module_id}/demos [get]
// @Security BearerAuth
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
	// 按 source+version 语义:只返回该 module "当前版本" 的 demo,而非跨所有版本的并集。
	// 当前版本 = modules.default_version_id(与模块库页面默认展示的版本一致);
	// 据此过滤 module_demos.module_version_id,自然排除未绑版本的孤儿 demo。
	if err := h.db.Table("module_demos d").
		Select(`d.id AS demo_id, d.name, d.description,
				v.config_data, v.change_summary`).
		Joins("LEFT JOIN module_demo_versions v ON v.id = d.current_version_id").
		Joins("JOIN modules m ON m.id = d.module_id").
		Where("d.module_id = ? AND d.is_active = true", uint(moduleID)).
		Where("d.module_version_id = m.default_version_id").
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

// ModuleInputField 编辑器补全用的 module 输入变量定义（扁平参数+类型，不做条件分支）。
type ModuleInputField struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`                  // OpenAPI base: string/number/boolean/object/array
	TypeLabel   string   `json:"type_label"`            // 展示/snippet 用: string, number, bool, list(string), map(string), object...
	Required    bool     `json:"required"`
	Description string   `json:"description"`
	Default     string   `json:"default,omitempty"`     // 默认值 JSON 串（仅提示）
	Enum        []string `json:"enum,omitempty"`        // 枚举可选值（有则补全可提示）
	Title       string   `json:"title,omitempty"`       // OpenAPI title
}

// ListModuleInputs GET /manifest-editor/modules/:module_id/inputs
//
// 从 module 活跃 schema 的 OpenAPI 定义提取输入变量（name / type / type_label / required / …），
// 供编辑器在 module 块内做属性补全（Tier3）。不做 x-iac-platform 条件过滤。
// 数据源: components.schemas.ModuleInput.properties + required。
// @Summary List module inputs for editor
// @Description Extract module input variables from active OpenAPI schema for editor completion
// @Tags Manifest Editor
// @Accept json
// @Produce json
// @Param module_id path int true "Module ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/manifest-editor/modules/{module_id}/inputs [get]
// @Security BearerAuth
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
// 仅扁平 properties；解析 array/items、map(additionalProperties)、enum；$ref 简化为 ref 名。
func extractModuleInputs(schema models.JSONB) []ModuleInputField {
	out := []ModuleInputField{}
	if schema == nil {
		return out
	}
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
		base, label := openAPITypeInfo(p)
		field := ModuleInputField{
			Name:      name,
			Type:      base,
			TypeLabel: label,
			Required:  requiredSet[name],
		}
		if d, ok := p["description"].(string); ok {
			field.Description = d
		}
		if t, ok := p["title"].(string); ok {
			field.Title = t
		}
		if dv, ok := p["default"]; ok && dv != nil {
			if b, mErr := json.Marshal(dv); mErr == nil {
				field.Default = string(b)
			}
		}
		if enumVals := openAPIEnumStrings(p); len(enumVals) > 0 {
			field.Enum = enumVals
		}
		out = append(out, field)
	}

	// 稳定顺序: required 优先，再按 name
	sort.Slice(out, func(i, j int) bool {
		if out[i].Required != out[j].Required {
			return out[i].Required
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// openAPITypeInfo 返回 (baseType, typeLabel)。typeLabel 贴近 HCL/Terraform 习惯展示。
func openAPITypeInfo(p map[string]interface{}) (base, label string) {
	// $ref → 用最后一段作类型名
	if ref, ok := p["$ref"].(string); ok && ref != "" {
		name := ref
		if i := strings.LastIndex(ref, "/"); i >= 0 {
			name = ref[i+1:]
		}
		return "object", name
	}

	t := openAPITypeString(p["type"])
	switch t {
	case "string":
		if enumVals := openAPIEnumStrings(p); len(enumVals) > 0 {
			return "string", "string" // enum 在字段 Enum 里
		}
		return "string", "string"
	case "integer", "number":
		return "number", "number"
	case "boolean":
		return "boolean", "bool"
	case "array":
		itemLabel := "any"
		if items, ok := p["items"].(map[string]interface{}); ok {
			_, itemLabel = openAPITypeInfo(items)
		}
		return "array", "list(" + itemLabel + ")"
	case "object":
		// additionalProperties → map(T)（如 tags）
		if ap, ok := p["additionalProperties"]; ok {
			switch v := ap.(type) {
			case bool:
				if v {
					return "object", "map(any)"
				}
			case map[string]interface{}:
				_, elem := openAPITypeInfo(v)
				return "object", "map(" + elem + ")"
			}
		}
		// 有/无 properties 的 object 块（list(object) 的 item 常无 properties）
		return "object", "object"
	default:
		if t != "" {
			return t, t
		}
		// 无 type 但有 enum
		if enumVals := openAPIEnumStrings(p); len(enumVals) > 0 {
			return "string", "string"
		}
		return "any", "any"
	}
}

func openAPITypeString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case []interface{}:
		// type: ["string","null"] → 取第一个非 null
		for _, x := range t {
			if s, ok := x.(string); ok && s != "" && s != "null" {
				return s
			}
		}
	}
	return ""
}

func openAPIEnumStrings(p map[string]interface{}) []string {
	raw, ok := p["enum"].([]interface{})
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, e := range raw {
		switch v := e.(type) {
		case string:
			out = append(out, v)
		default:
			if b, err := json.Marshal(v); err == nil {
				out = append(out, string(b))
			}
		}
	}
	return out
}

// 占位防 lint: models 包导入不能为空
var _ = models.ModuleDemo{}
