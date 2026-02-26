package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"iac-platform/internal/models"
	"iac-platform/services"
)

// ModuleSchemaV2Handler 处理 V2 Schema 相关的 API 请求
type ModuleSchemaV2Handler struct {
	db            *gorm.DB
	parserService *services.SchemaParserService
}

// NewModuleSchemaV2Handler 创建新的 V2 Schema Handler
func NewModuleSchemaV2Handler(db *gorm.DB) *ModuleSchemaV2Handler {
	return &ModuleSchemaV2Handler{
		db:            db,
		parserService: services.NewSchemaParserService(),
	}
}

// ParseTF 解析 Terraform variables.tf 并返回 OpenAPI Schema
// @Summary 解析 Terraform variables.tf
// @Description 解析 Terraform variables.tf 文件内容，生成 OpenAPI v3 格式的 Schema
// @Tags Module Schema V2
// @Accept json
// @Produce json
// @Param request body models.ParseTFRequest true "解析请求"
// @Success 200 {object} models.ParseTFResponse
// @Failure 400 {object} map[string]string
// @Router /api/v1/modules/parse-tf-v2 [post]
func (h *ModuleSchemaV2Handler) ParseTF(c *gin.Context) {
	var req models.ParseTFRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
		return
	}

	// 验证至少有一个输入
	if req.VariablesTF == "" && req.OutputsTF == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供 variables_tf 或 outputs_tf"})
		return
	}

	opts := services.ParseOptions{
		ModuleName: req.ModuleName,
		Provider:   req.Provider,
		Version:    req.Version,
		Layout:     req.Layout,
	}

	// 使用新的 ParseTFWithOutputs 方法，支持只解析 outputs
	result, err := h.parserService.ParseTFWithOutputs(req.VariablesTF, req.OutputsTF, opts)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, models.ParseTFResponse{
		OpenAPISchema:  result.OpenAPISchema,
		FieldCount:     result.FieldCount,
		BasicFields:    result.BasicFields,
		AdvancedFields: result.AdvancedFields,
		Warnings:       result.Warnings,
	})
}

// GetSchemaV2 获取模块的 V2 Schema
// @Summary 获取 V2 Schema
// @Description 获取指定模块的 OpenAPI v3 格式 Schema。如果不传 version_id，则获取默认版本的 active Schema
// @Tags Module Schema V2
// @Produce json
// @Param id path int true "模块ID"
// @Param version_id query string false "Module Version ID (modv-xxx)，不传则获取默认版本的 Schema"
// @Success 200 {object} models.Schema
// @Failure 404 {object} map[string]string
// @Router /api/v1/modules/{id}/schemas/v2 [get]
func (h *ModuleSchemaV2Handler) GetSchemaV2(c *gin.Context) {
	moduleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的模块ID"})
		return
	}

	versionID := c.Query("version_id")

	// 如果没指定 version_id，使用默认版本
	if versionID == "" {
		var module models.Module
		if err := h.db.First(&module, moduleID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "模块不存在"})
			return
		}
		if module.DefaultVersionID != nil {
			versionID = *module.DefaultVersionID
		}
	}

	if versionID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到默认版本"})
		return
	}

	// 验证版本属于该模块
	var version models.ModuleVersion
	if err := h.db.Where("id = ? AND module_id = ?", versionID, moduleID).First(&version).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "版本不存在"})
		return
	}

	schema, err := services.GetLatestSchemaV2(h.db, versionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询 Schema 失败"})
		return
	}
	if schema == nil {
		// 兼容回退：如果该版本没有 v2 schema，尝试获取任意最新 schema
		schema, err = services.GetLatestSchema(h.db, versionID)
		if err != nil || schema == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "未找到 V2 Schema"})
			return
		}
	}

	c.JSON(http.StatusOK, schema)
}

// CreateSchemaV2 创建 V2 Schema
// @Summary 创建 V2 Schema
// @Description 为指定模块创建 OpenAPI v3 格式的 Schema
// @Tags Module Schema V2
// @Accept json
// @Produce json
// @Param id path int true "模块ID"
// @Param version_id query string false "Module Version ID (modv-xxx)，不传则关联到默认版本"
// @Param request body models.CreateSchemaV2Request true "创建请求"
// @Success 201 {object} models.Schema
// @Failure 400 {object} map[string]string
// @Router /api/v1/modules/{id}/schemas/v2 [post]
func (h *ModuleSchemaV2Handler) CreateSchemaV2(c *gin.Context) {
	moduleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的模块ID"})
		return
	}

	// 支持按 version_id 创建
	versionID := c.Query("version_id")

	var req models.CreateSchemaV2Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
		return
	}

	// 验证模块是否存在
	var module models.Module
	if err := h.db.First(&module, moduleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "模块不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询模块失败"})
		return
	}

	// 如果没有指定 version_id，使用模块的默认版本
	if versionID == "" && module.DefaultVersionID != nil {
		versionID = *module.DefaultVersionID
	}

	// 验证版本是否存在（如果指定了）
	if versionID != "" {
		var version models.ModuleVersion
		if err := h.db.Where("id = ? AND module_id = ?", versionID, moduleID).First(&version).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "指定的模块版本不存在"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询模块版本失败"})
			return
		}
	}

	// 转换 OpenAPI Schema 为 JSONB
	openAPISchemaJSON, err := json.Marshal(req.OpenAPISchema)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 OpenAPI Schema"})
		return
	}

	var openAPISchema models.JSONB
	if err := json.Unmarshal(openAPISchemaJSON, &openAPISchema); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "解析 OpenAPI Schema 失败"})
		return
	}

	// 提取 UI 配置
	var uiConfig models.JSONB
	if iacPlatform, ok := openAPISchema["x-iac-platform"].(map[string]interface{}); ok {
		if ui, ok := iacPlatform["ui"].(map[string]interface{}); ok {
			uiConfig = ui
		}
	}

	// 获取用户ID
	userID := c.GetString("user_id")

	// 设置状态
	status := req.Status
	if status == "" {
		status = "active"
	}

	// 设置来源类型
	sourceType := req.SourceType
	if sourceType == "" {
		sourceType = "tf_parse"
	}

	// 如果新 Schema 状态为 active，先将该版本的所有现有 Schema 设为 inactive
	if status == "active" && versionID != "" {
		if err := h.db.Model(&models.Schema{}).
			Where("module_id = ? AND module_version_id = ?", moduleID, versionID).
			Update("status", "inactive").Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "更新旧版本状态失败: " + err.Error()})
			return
		}
	}

	schema := models.Schema{
		ModuleID:      uint(moduleID),
		Version:       req.Version,
		Status:        status,
		SchemaVersion: "v2",
		SchemaData:    "{}", // V2 Schema 使用 OpenAPISchema，但 schema_data 是 NOT NULL
		OpenAPISchema: openAPISchema,
		VariablesTF:   req.VariablesTF,
		UIConfig:      uiConfig,
		SourceType:    sourceType,
		CreatedBy:     &userID,
	}

	// 关联到模块版本
	if versionID != "" {
		schema.ModuleVersionID = &versionID
	}

	if err := h.db.Create(&schema).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建 Schema 失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, schema)
}

// UpdateSchemaV2 更新 V2 Schema
// @Summary 更新 V2 Schema
// @Description 更新指定模块的 OpenAPI v3 格式 Schema
// @Tags Module Schema V2
// @Accept json
// @Produce json
// @Param id path int true "模块ID"
// @Param schemaId path int true "Schema ID"
// @Param request body models.UpdateSchemaV2Request true "更新请求"
// @Success 200 {object} models.Schema
// @Failure 400 {object} map[string]string
// @Router /api/v1/modules/{id}/schemas/v2/{schemaId} [put]
func (h *ModuleSchemaV2Handler) UpdateSchemaV2(c *gin.Context) {
	moduleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的模块ID"})
		return
	}

	schemaID, err := strconv.ParseUint(c.Param("schemaId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 Schema ID"})
		return
	}

	var req models.UpdateSchemaV2Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
		return
	}

	// 查找现有 Schema
	var schema models.Schema
	if err := h.db.Where("id = ? AND module_id = ? AND schema_version = ?", schemaID, moduleID, "v2").First(&schema).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Schema 不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	// 更新字段
	updates := make(map[string]interface{})

	if req.OpenAPISchema != nil {
		openAPISchemaJSON, err := json.Marshal(req.OpenAPISchema)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 OpenAPI Schema"})
			return
		}
		var openAPISchema models.JSONB
		if err := json.Unmarshal(openAPISchemaJSON, &openAPISchema); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "解析 OpenAPI Schema 失败"})
			return
		}
		updates["openapi_schema"] = openAPISchema

		// 更新 UI 配置
		if iacPlatform, ok := openAPISchema["x-iac-platform"].(map[string]interface{}); ok {
			if ui, ok := iacPlatform["ui"].(map[string]interface{}); ok {
				updates["ui_config"] = ui
			}
		}
	}

	if req.UIConfig != nil {
		uiConfigJSON, err := json.Marshal(req.UIConfig)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 UI 配置"})
			return
		}
		var uiConfig models.JSONB
		if err := json.Unmarshal(uiConfigJSON, &uiConfig); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "解析 UI 配置失败"})
			return
		}
		updates["ui_config"] = uiConfig
	}

	if req.VariablesTF != "" {
		updates["variables_tf"] = req.VariablesTF
	}

	if req.Status != "" {
		updates["status"] = req.Status
	}

	if len(updates) > 0 {
		if err := h.db.Model(&schema).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败: " + err.Error()})
			return
		}
	}

	// 重新加载
	h.db.First(&schema, schema.ID)

	c.JSON(http.StatusOK, schema)
}

// UpdateSchemaField 更新单个字段的 UI 配置
// @Summary 更新单个字段配置
// @Description 更新 Schema 中单个字段的 UI 配置
// @Tags Module Schema V2
// @Accept json
// @Produce json
// @Param id path int true "模块ID"
// @Param schemaId path int true "Schema ID"
// @Param request body models.SchemaFieldUpdate true "字段更新请求"
// @Success 200 {object} models.Schema
// @Failure 400 {object} map[string]string
// @Router /api/v1/modules/{id}/schemas/v2/{schemaId}/fields [patch]
func (h *ModuleSchemaV2Handler) UpdateSchemaField(c *gin.Context) {
	moduleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的模块ID"})
		return
	}

	schemaID, err := strconv.ParseUint(c.Param("schemaId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 Schema ID"})
		return
	}

	var req models.SchemaFieldUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
		return
	}

	// 查找现有 Schema
	var schema models.Schema
	if err := h.db.Where("id = ? AND module_id = ? AND schema_version = ?", schemaID, moduleID, "v2").First(&schema).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Schema 不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	// 更新 UI 配置中的字段
	if schema.UIConfig == nil {
		schema.UIConfig = make(models.JSONB)
	}

	fields, ok := schema.UIConfig["fields"].(map[string]interface{})
	if !ok {
		fields = make(map[string]interface{})
	}

	fieldConfig, ok := fields[req.FieldName].(map[string]interface{})
	if !ok {
		fieldConfig = make(map[string]interface{})
	}

	fieldConfig[req.Property] = req.Value
	fields[req.FieldName] = fieldConfig
	schema.UIConfig["fields"] = fields

	// 同时更新 openapi_schema 中的 x-iac-platform.ui.fields
	if schema.OpenAPISchema != nil {
		if iacPlatform, ok := schema.OpenAPISchema["x-iac-platform"].(map[string]interface{}); ok {
			if ui, ok := iacPlatform["ui"].(map[string]interface{}); ok {
				ui["fields"] = fields
			}
		}
	}

	if err := h.db.Save(&schema).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, schema)
}

// GetAllSchemas 获取模块的所有 Schema（包括 v1 和 v2）
// @Summary 获取所有 Schema
// @Description 获取指定模块的所有 Schema，包括 v1 和 v2 版本
// @Tags Module Schema V2
// @Produce json
// @Param id path int true "模块ID"
// @Success 200 {array} models.Schema
// @Router /api/v1/modules/{id}/schemas/all [get]
func (h *ModuleSchemaV2Handler) GetAllSchemas(c *gin.Context) {
	moduleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的模块ID"})
		return
	}

	var schemas []models.Schema
	if err := h.db.Where("module_id = ?", moduleID).Order("created_at DESC").Find(&schemas).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	c.JSON(http.StatusOK, schemas)
}

// MigrateToV2 将 v1 Schema 迁移到 v2
// @Summary 迁移到 V2
// @Description 将 v1 Schema 迁移到 v2 格式
// @Tags Module Schema V2
// @Produce json
// @Param id path int true "模块ID"
// @Param schemaId path int true "Schema ID"
// @Success 200 {object} models.Schema
// @Failure 400 {object} map[string]string
// @Router /api/v1/modules/{id}/schemas/{schemaId}/migrate-v2 [post]
func (h *ModuleSchemaV2Handler) MigrateToV2(c *gin.Context) {
	moduleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的模块ID"})
		return
	}

	schemaID, err := strconv.ParseUint(c.Param("schemaId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 Schema ID"})
		return
	}

	// 查找 v1 Schema
	var v1Schema models.Schema
	if err := h.db.Where("id = ? AND module_id = ?", schemaID, moduleID).First(&v1Schema).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Schema 不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	if v1Schema.SchemaVersion == "v2" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该 Schema 已经是 v2 版本"})
		return
	}

	// 解析 v1 SchemaData
	var v1Data map[string]interface{}
	if err := json.Unmarshal([]byte(v1Schema.SchemaData), &v1Data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解析 v1 Schema 失败"})
		return
	}

	// 转换为 v2 格式
	v2Schema := h.convertV1ToV2(v1Data)

	// 获取模块信息
	var module models.Module
	h.db.First(&module, moduleID)

	// 添加模块信息
	if info, ok := v2Schema["info"].(map[string]interface{}); ok {
		info["title"] = module.Name
		info["x-provider"] = module.Provider
		info["x-module-source"] = module.Source
	}

	// 获取用户ID
	userID := c.GetString("user_id")

	// 创建新的 v2 Schema
	newSchema := models.Schema{
		ModuleID:      uint(moduleID),
		Version:       v1Schema.Version,
		Status:        "active",
		SchemaVersion: "v2",
		OpenAPISchema: v2Schema,
		SourceType:    "migration",
		CreatedBy:     &userID,
	}

	// 提取 UI 配置
	if iacPlatform, ok := v2Schema["x-iac-platform"].(map[string]interface{}); ok {
		if ui, ok := iacPlatform["ui"].(map[string]interface{}); ok {
			newSchema.UIConfig = ui
		}
	}

	if err := h.db.Create(&newSchema).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建 v2 Schema 失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, newSchema)
}

// convertV1ToV2 将 v1 Schema 转换为 v2 格式
func (h *ModuleSchemaV2Handler) convertV1ToV2(v1Data map[string]interface{}) models.JSONB {
	properties := make(map[string]interface{})
	required := []string{}
	uiFields := make(map[string]interface{})

	for fieldName, fieldData := range v1Data {
		field, ok := fieldData.(map[string]interface{})
		if !ok {
			continue
		}

		// 转换属性
		prop := make(map[string]interface{})

		// 类型转换
		if typeVal, ok := field["type"]; ok {
			prop["type"] = h.convertV1Type(typeVal)
		}

		// 描述
		if desc, ok := field["description"]; ok {
			prop["description"] = desc
		}

		// 默认值
		if def, ok := field["default"]; ok {
			prop["default"] = def
		}

		// 必填
		if req, ok := field["required"].(bool); ok && req {
			required = append(required, fieldName)
		}

		properties[fieldName] = prop

		// UI 配置
		uiField := map[string]interface{}{
			"label": h.formatLabel(fieldName),
			"group": "advanced",
		}

		if alias, ok := field["alias"]; ok {
			uiField["label"] = alias
		}

		if desc, ok := field["description"]; ok {
			uiField["help"] = desc
		}

		// 推断 widget
		uiField["widget"] = h.inferWidgetFromV1(field)

		uiFields[fieldName] = uiField
	}

	return models.JSONB{
		"openapi": "3.1.0",
		"info": map[string]interface{}{
			"title":   "Module",
			"version": "1.0.0",
		},
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{
				"ModuleInput": map[string]interface{}{
					"type":       "object",
					"properties": properties,
					"required":   required,
				},
			},
		},
		"x-iac-platform": map[string]interface{}{
			"ui": map[string]interface{}{
				"fields": uiFields,
				"groups": []map[string]interface{}{
					{"id": "basic", "title": "基础配置", "order": 1, "defaultExpanded": true},
					{"id": "advanced", "title": "高级配置", "order": 2, "defaultExpanded": false},
				},
			},
		},
	}
}

// convertV1Type 转换 v1 类型到 JSON Schema 类型
func (h *ModuleSchemaV2Handler) convertV1Type(typeVal interface{}) string {
	switch v := typeVal.(type) {
	case float64:
		// v1 使用数字表示类型
		switch int(v) {
		case 1: // TypeBool
			return "boolean"
		case 2, 3: // TypeInt, TypeFloat
			return "number"
		case 4, 9, 10: // TypeString, TypeJsonString, TypeText
			return "string"
		case 5, 7: // TypeList, TypeSet
			return "array"
		case 6, 8, 11, 12: // TypeMap, TypeObject, TypeListObject, CustomObject
			return "object"
		default:
			return "string"
		}
	case string:
		return v
	default:
		return "string"
	}
}

// inferWidgetFromV1 从 v1 字段推断 widget
func (h *ModuleSchemaV2Handler) inferWidgetFromV1(field map[string]interface{}) string {
	if typeVal, ok := field["type"].(float64); ok {
		switch int(typeVal) {
		case 1: // TypeBool
			return "switch"
		case 5, 7: // TypeList, TypeSet
			return "tags"
		case 6: // TypeMap
			return "key-value"
		case 8: // TypeObject
			return "object"
		case 9: // TypeJsonString
			return "json-editor"
		case 10: // TypeText
			return "textarea"
		case 11: // TypeListObject
			return "object-list"
		}
	}

	if _, ok := field["dynamic"]; ok {
		return "select"
	}

	return "text"
}

// formatLabel 格式化标签
func (h *ModuleSchemaV2Handler) formatLabel(name string) string {
	// 简单实现：将下划线替换为空格，首字母大写
	result := ""
	words := []rune(name)
	capitalize := true
	for _, r := range words {
		if r == '_' {
			result += " "
			capitalize = true
		} else if capitalize {
			result += string([]rune{r - 32}) // 简单的大写转换
			capitalize = false
		} else {
			result += string(r)
		}
	}
	return result
}

// CompareSchemas 对比两个 Schema 版本
// @Summary 对比 Schema 版本
// @Description 对比两个 Schema 版本的差异
// @Tags Module Schema V2
// @Produce json
// @Param id path int true "模块ID"
// @Param oldId query int true "旧版本 Schema ID"
// @Param newId query int true "新版本 Schema ID"
// @Success 200 {object} models.SchemaDiffResponse
// @Failure 400 {object} map[string]string
// @Router /api/v1/modules/{id}/schemas/compare [get]
func (h *ModuleSchemaV2Handler) CompareSchemas(c *gin.Context) {
	moduleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的模块ID"})
		return
	}

	oldID, err := strconv.ParseUint(c.Query("oldId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的旧版本 Schema ID"})
		return
	}

	newID, err := strconv.ParseUint(c.Query("newId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的新版本 Schema ID"})
		return
	}

	// 获取旧版本 Schema
	var oldSchema models.Schema
	if err := h.db.Where("id = ? AND module_id = ?", oldID, moduleID).First(&oldSchema).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "旧版本 Schema 不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询旧版本失败"})
		return
	}

	// 获取新版本 Schema
	var newSchema models.Schema
	if err := h.db.Where("id = ? AND module_id = ?", newID, moduleID).First(&newSchema).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "新版本 Schema 不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询新版本失败"})
		return
	}

	// 获取 Schema 数据
	var oldData, newData interface{}
	if oldSchema.SchemaVersion == "v2" && oldSchema.OpenAPISchema != nil {
		oldData = oldSchema.OpenAPISchema
	} else {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(oldSchema.SchemaData), &parsed); err == nil {
			oldData = parsed
		}
	}

	if newSchema.SchemaVersion == "v2" && newSchema.OpenAPISchema != nil {
		newData = newSchema.OpenAPISchema
	} else {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(newSchema.SchemaData), &parsed); err == nil {
			newData = parsed
		}
	}

	// 计算差异
	diffs := h.computeDiff(oldData, newData, "")

	// 统计
	var added, removed, modified int
	for _, diff := range diffs {
		switch diff["type"] {
		case "added":
			added++
		case "removed":
			removed++
		case "modified":
			modified++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"old_version": oldSchema.Version,
		"new_version": newSchema.Version,
		"old_id":      oldSchema.ID,
		"new_id":      newSchema.ID,
		"old_data":    oldData,
		"new_data":    newData,
		"diffs":       diffs,
		"stats": gin.H{
			"total":    len(diffs),
			"added":    added,
			"removed":  removed,
			"modified": modified,
		},
	})
}

// computeDiff 计算两个对象的差异
func (h *ModuleSchemaV2Handler) computeDiff(oldObj, newObj interface{}, path string) []map[string]interface{} {
	var diffs []map[string]interface{}

	// 处理 nil
	if oldObj == nil && newObj == nil {
		return diffs
	}

	if oldObj == nil {
		diffs = append(diffs, map[string]interface{}{
			"type":      "added",
			"path":      path,
			"new_value": newObj,
		})
		return diffs
	}

	if newObj == nil {
		diffs = append(diffs, map[string]interface{}{
			"type":      "removed",
			"path":      path,
			"old_value": oldObj,
		})
		return diffs
	}

	// 类型不同
	oldMap, oldIsMap := oldObj.(map[string]interface{})
	newMap, newIsMap := newObj.(map[string]interface{})

	if oldIsMap && newIsMap {
		// 比较 map
		allKeys := make(map[string]bool)
		for k := range oldMap {
			allKeys[k] = true
		}
		for k := range newMap {
			allKeys[k] = true
		}

		for key := range allKeys {
			keyPath := key
			if path != "" {
				keyPath = path + "." + key
			}

			oldVal, oldHas := oldMap[key]
			newVal, newHas := newMap[key]

			if !oldHas {
				diffs = append(diffs, map[string]interface{}{
					"type":      "added",
					"path":      keyPath,
					"new_value": newVal,
				})
			} else if !newHas {
				diffs = append(diffs, map[string]interface{}{
					"type":      "removed",
					"path":      keyPath,
					"old_value": oldVal,
				})
			} else {
				diffs = append(diffs, h.computeDiff(oldVal, newVal, keyPath)...)
			}
		}
	} else {
		// 比较基本类型或数组
		oldJSON, _ := json.Marshal(oldObj)
		newJSON, _ := json.Marshal(newObj)

		if string(oldJSON) != string(newJSON) {
			diffs = append(diffs, map[string]interface{}{
				"type":      "modified",
				"path":      path,
				"old_value": oldObj,
				"new_value": newObj,
			})
		}
	}

	return diffs
}

// ValidateModuleInput 验证模块输入
// @Summary 验证模块输入
// @Description 根据 Schema 验证模块输入参数
// @Tags Module Schema V2
// @Accept json
// @Produce json
// @Param id path int true "模块ID"
// @Param request body models.ValidateModuleInputRequest true "验证请求"
// @Success 200 {object} models.ValidateModuleInputResponse
// @Failure 400 {object} map[string]string
// @Router /api/v1/modules/{id}/validate [post]
func (h *ModuleSchemaV2Handler) ValidateModuleInput(c *gin.Context) {
	moduleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的模块ID"})
		return
	}

	var req models.ValidateModuleInputRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
		return
	}

	// 获取活跃的 v2 Schema（按版本解析）
	versionID := c.Query("version_id")
	if versionID == "" {
		var module models.Module
		if err := h.db.First(&module, moduleID).Error; err == nil && module.DefaultVersionID != nil {
			versionID = *module.DefaultVersionID
		}
	}

	var schema *models.Schema
	if versionID != "" {
		schema, _ = services.GetLatestSchemaV2(h.db, versionID)
	}
	if schema == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到活跃的 V2 Schema"})
		return
	}

	// 验证输入
	errors := h.validateInput(schema.OpenAPISchema, req.Values, req.Mode)

	c.JSON(http.StatusOK, models.ValidateModuleInputResponse{
		Valid:  len(errors) == 0,
		Errors: errors,
	})
}

// validateInput 验证输入值
func (h *ModuleSchemaV2Handler) validateInput(openAPISchema models.JSONB, values map[string]interface{}, mode string) []models.ValidationError {
	var errors []models.ValidationError

	if openAPISchema == nil {
		return errors
	}

	// 获取 Schema 定义
	components, ok := openAPISchema["components"].(map[string]interface{})
	if !ok {
		return errors
	}

	schemas, ok := components["schemas"].(map[string]interface{})
	if !ok {
		return errors
	}

	moduleInput, ok := schemas["ModuleInput"].(map[string]interface{})
	if !ok {
		return errors
	}

	properties, ok := moduleInput["properties"].(map[string]interface{})
	if !ok {
		return errors
	}

	// 获取必填字段
	requiredFields := make(map[string]bool)
	if required, ok := moduleInput["required"].([]interface{}); ok {
		for _, r := range required {
			if fieldName, ok := r.(string); ok {
				requiredFields[fieldName] = true
			}
		}
	}

	// 验证每个字段
	for fieldName, propData := range properties {
		prop, ok := propData.(map[string]interface{})
		if !ok {
			continue
		}

		value, hasValue := values[fieldName]

		// 检查必填
		if requiredFields[fieldName] && (!hasValue || h.isEmpty(value)) {
			errors = append(errors, models.ValidationError{
				Field:   fieldName,
				Type:    "required",
				Message: h.getFieldTitle(prop, fieldName) + " 是必填项",
			})
			continue
		}

		// 如果没有值，跳过其他验证
		if !hasValue || h.isEmpty(value) {
			continue
		}

		// 类型验证
		if propType, ok := prop["type"].(string); ok {
			if err := h.validateType(fieldName, value, propType, prop); err != nil {
				errors = append(errors, *err)
			}
		}

		// Pattern 验证
		if pattern, ok := prop["pattern"].(string); ok {
			if strVal, ok := value.(string); ok {
				if !h.matchPattern(strVal, pattern) {
					errorMsg := "格式不正确"
					if validation, ok := prop["x-validation"].([]interface{}); ok && len(validation) > 0 {
						if v, ok := validation[0].(map[string]interface{}); ok {
							if msg, ok := v["errorMessage"].(string); ok {
								errorMsg = msg
							}
						}
					}
					errors = append(errors, models.ValidationError{
						Field:   fieldName,
						Type:    "pattern",
						Message: errorMsg,
					})
				}
			}
		}

		// 最小值验证
		if minimum, ok := prop["minimum"].(float64); ok {
			if numVal, ok := h.toFloat64(value); ok {
				if numVal < minimum {
					errors = append(errors, models.ValidationError{
						Field:   fieldName,
						Type:    "minimum",
						Message: h.getFieldTitle(prop, fieldName) + " 不能小于 " + strconv.FormatFloat(minimum, 'f', -1, 64),
					})
				}
			}
		}

		// 最大值验证
		if maximum, ok := prop["maximum"].(float64); ok {
			if numVal, ok := h.toFloat64(value); ok {
				if numVal > maximum {
					errors = append(errors, models.ValidationError{
						Field:   fieldName,
						Type:    "maximum",
						Message: h.getFieldTitle(prop, fieldName) + " 不能大于 " + strconv.FormatFloat(maximum, 'f', -1, 64),
					})
				}
			}
		}

		// 最小长度验证
		if minLength, ok := prop["minLength"].(float64); ok {
			if strVal, ok := value.(string); ok {
				if len(strVal) < int(minLength) {
					errors = append(errors, models.ValidationError{
						Field:   fieldName,
						Type:    "minLength",
						Message: h.getFieldTitle(prop, fieldName) + " 最少需要 " + strconv.Itoa(int(minLength)) + " 个字符",
					})
				}
			}
		}

		// 最大长度验证
		if maxLength, ok := prop["maxLength"].(float64); ok {
			if strVal, ok := value.(string); ok {
				if len(strVal) > int(maxLength) {
					errors = append(errors, models.ValidationError{
						Field:   fieldName,
						Type:    "maxLength",
						Message: h.getFieldTitle(prop, fieldName) + " 最多 " + strconv.Itoa(int(maxLength)) + " 个字符",
					})
				}
			}
		}

		// Enum 验证
		if enum, ok := prop["enum"].([]interface{}); ok {
			if strVal, ok := value.(string); ok {
				found := false
				for _, e := range enum {
					if eStr, ok := e.(string); ok && eStr == strVal {
						found = true
						break
					}
				}
				if !found {
					errors = append(errors, models.ValidationError{
						Field:   fieldName,
						Type:    "enum",
						Message: h.getFieldTitle(prop, fieldName) + " 的值不在允许的选项中",
					})
				}
			}
		}
	}

	// 跨字段验证
	if iacPlatform, ok := openAPISchema["x-iac-platform"].(map[string]interface{}); ok {
		if validation, ok := iacPlatform["validation"].(map[string]interface{}); ok {
			if rules, ok := validation["rules"].([]interface{}); ok {
				for _, ruleData := range rules {
					rule, ok := ruleData.(map[string]interface{})
					if !ok {
						continue
					}

					if err := h.validateCrossFieldRule(rule, values); err != nil {
						errors = append(errors, *err)
					}
				}
			}
		}
	}

	return errors
}

// validateType 验证值类型
func (h *ModuleSchemaV2Handler) validateType(fieldName string, value interface{}, expectedType string, prop map[string]interface{}) *models.ValidationError {
	switch expectedType {
	case "string":
		if _, ok := value.(string); !ok {
			return &models.ValidationError{
				Field:   fieldName,
				Type:    "type",
				Message: h.getFieldTitle(prop, fieldName) + " 必须是字符串",
			}
		}
	case "integer", "number":
		if _, ok := h.toFloat64(value); !ok {
			return &models.ValidationError{
				Field:   fieldName,
				Type:    "type",
				Message: h.getFieldTitle(prop, fieldName) + " 必须是数字",
			}
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return &models.ValidationError{
				Field:   fieldName,
				Type:    "type",
				Message: h.getFieldTitle(prop, fieldName) + " 必须是布尔值",
			}
		}
	case "array":
		if _, ok := value.([]interface{}); !ok {
			return &models.ValidationError{
				Field:   fieldName,
				Type:    "type",
				Message: h.getFieldTitle(prop, fieldName) + " 必须是数组",
			}
		}
	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return &models.ValidationError{
				Field:   fieldName,
				Type:    "type",
				Message: h.getFieldTitle(prop, fieldName) + " 必须是对象",
			}
		}
	}
	return nil
}

// validateCrossFieldRule 验证跨字段规则
func (h *ModuleSchemaV2Handler) validateCrossFieldRule(rule map[string]interface{}, values map[string]interface{}) *models.ValidationError {
	ruleType, _ := rule["type"].(string)
	message, _ := rule["message"].(string)

	switch ruleType {
	case "conflicts":
		// 冲突验证：多个字段不能同时有值
		if fields, ok := rule["fields"].([]interface{}); ok {
			hasValueCount := 0
			for _, f := range fields {
				if fieldName, ok := f.(string); ok {
					if val, exists := values[fieldName]; exists && !h.isEmpty(val) {
						hasValueCount++
					}
				}
			}
			if hasValueCount > 1 {
				return &models.ValidationError{
					Field:   "",
					Type:    "conflicts",
					Message: message,
				}
			}
		}

	case "requiredWith":
		// 条件必填：当触发条件满足时，某些字段必填
		if trigger, ok := rule["trigger"].(map[string]interface{}); ok {
			triggerField, _ := trigger["field"].(string)
			triggerValue := trigger["value"]

			if val, exists := values[triggerField]; exists {
				// 检查触发条件是否满足
				if h.valuesEqual(val, triggerValue) {
					// 检查必填字段
					if requires, ok := rule["requires"].([]interface{}); ok {
						for _, r := range requires {
							if reqField, ok := r.(string); ok {
								if reqVal, exists := values[reqField]; !exists || h.isEmpty(reqVal) {
									return &models.ValidationError{
										Field:   reqField,
										Type:    "requiredWith",
										Message: message,
									}
								}
							}
						}
					}
				}
			}
		}

	case "exactlyOneOf":
		// 必须且只能有一个字段有值
		if fields, ok := rule["fields"].([]interface{}); ok {
			hasValueCount := 0
			for _, f := range fields {
				if fieldName, ok := f.(string); ok {
					if val, exists := values[fieldName]; exists && !h.isEmpty(val) {
						hasValueCount++
					}
				}
			}
			if hasValueCount != 1 {
				return &models.ValidationError{
					Field:   "",
					Type:    "exactlyOneOf",
					Message: message,
				}
			}
		}

	case "atLeastOneOf":
		// 至少有一个字段有值
		if fields, ok := rule["fields"].([]interface{}); ok {
			hasValue := false
			for _, f := range fields {
				if fieldName, ok := f.(string); ok {
					if val, exists := values[fieldName]; exists && !h.isEmpty(val) {
						hasValue = true
						break
					}
				}
			}
			if !hasValue {
				return &models.ValidationError{
					Field:   "",
					Type:    "atLeastOneOf",
					Message: message,
				}
			}
		}
	}

	return nil
}

// isEmpty 检查值是否为空
func (h *ModuleSchemaV2Handler) isEmpty(value interface{}) bool {
	if value == nil {
		return true
	}

	switch v := value.(type) {
	case string:
		return v == ""
	case []interface{}:
		return len(v) == 0
	case map[string]interface{}:
		return len(v) == 0
	}

	return false
}

// getFieldTitle 获取字段标题
func (h *ModuleSchemaV2Handler) getFieldTitle(prop map[string]interface{}, fieldName string) string {
	if title, ok := prop["title"].(string); ok && title != "" {
		return title
	}
	return fieldName
}

// toFloat64 转换为 float64
func (h *ModuleSchemaV2Handler) toFloat64(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// matchPattern 正则匹配
func (h *ModuleSchemaV2Handler) matchPattern(value, pattern string) bool {
	// 简单实现，实际应使用 regexp
	// 这里只做基本的前缀匹配
	if len(pattern) > 0 && pattern[0] == '^' {
		prefix := pattern[1:]
		if len(prefix) > 0 && prefix[len(prefix)-1] == '$' {
			// 完全匹配
			return value == prefix[:len(prefix)-1]
		}
		// 前缀匹配
		return len(value) >= len(prefix) && value[:len(prefix)] == prefix
	}
	return true
}

// valuesEqual 比较两个值是否相等
func (h *ModuleSchemaV2Handler) valuesEqual(a, b interface{}) bool {
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return string(aJSON) == string(bJSON)
}

// SetActiveSchema 设置活跃版本
// @Summary 设置活跃版本
// @Description 将指定 Schema 设置为活跃版本
// @Tags Module Schema V2
// @Produce json
// @Param id path int true "模块ID"
// @Param schemaId path int true "Schema ID"
// @Success 200 {object} models.Schema
// @Failure 400 {object} map[string]string
// @Router /api/v1/modules/{id}/schemas/{schemaId}/activate [post]
func (h *ModuleSchemaV2Handler) SetActiveSchema(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{
		"error": "此接口已废弃。系统现在自动使用最新的 Schema 版本。",
	})
}
