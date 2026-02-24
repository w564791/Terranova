package services

import (
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ProviderTemplateService Provider模板服务
type ProviderTemplateService struct {
	db *gorm.DB
}

// NewProviderTemplateService 创建Provider模板服务
func NewProviderTemplateService(db *gorm.DB) *ProviderTemplateService {
	return &ProviderTemplateService{
		db: db,
	}
}

// List 获取Provider模板列表
func (s *ProviderTemplateService) List(enabled *bool, providerType string) ([]models.ProviderTemplate, error) {
	var templates []models.ProviderTemplate
	query := s.db.Model(&models.ProviderTemplate{})

	if enabled != nil {
		query = query.Where("enabled = ?", *enabled)
	}

	if providerType != "" {
		query = query.Where("type = ?", providerType)
	}

	// 默认模板排在最前面
	err := query.Order("is_default DESC, created_at DESC").Find(&templates).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query provider templates: %w", err)
	}

	if templates == nil {
		templates = []models.ProviderTemplate{}
	}

	return templates, nil
}

// GetByID 根据ID获取Provider模板
func (s *ProviderTemplateService) GetByID(id uint) (*models.ProviderTemplate, error) {
	var template models.ProviderTemplate
	err := s.db.First(&template, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("provider template not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get provider template: %w", err)
	}

	return &template, nil
}

// GetByIDs 根据ID列表获取Provider模板
func (s *ProviderTemplateService) GetByIDs(ids []uint) ([]models.ProviderTemplate, error) {
	var templates []models.ProviderTemplate
	if len(ids) == 0 {
		return templates, nil
	}

	err := s.db.Where("id IN ?", ids).Find(&templates).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get provider templates by ids: %w", err)
	}

	return templates, nil
}

// Create 创建Provider模板
func (s *ProviderTemplateService) Create(req *models.CreateProviderTemplateRequest) (*models.ProviderTemplate, error) {
	// 检查名称是否已存在
	var count int64
	s.db.Model(&models.ProviderTemplate{}).Where("name = ?", req.Name).Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("provider template name '%s' already exists", req.Name)
	}

	template := &models.ProviderTemplate{
		Name:         req.Name,
		Type:         req.Type,
		Source:       req.Source,
		Alias:        req.Alias,
		Config:       models.JSONB(req.Config),
		Version:      req.Version,
		ConstraintOp: req.ConstraintOp,
		Enabled:      req.Enabled,
		Description:  req.Description,
	}

	err := s.db.Create(template).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create provider template: %w", err)
	}

	return template, nil
}

// Update 更新Provider模板
func (s *ProviderTemplateService) Update(id uint, req *models.UpdateProviderTemplateRequest) (*models.ProviderTemplate, error) {
	// 检查模板是否存在
	_, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	// 检查名称唯一性
	if req.Name != nil {
		var count int64
		s.db.Model(&models.ProviderTemplate{}).Where("name = ? AND id != ?", *req.Name, id).Count(&count)
		if count > 0 {
			return nil, fmt.Errorf("provider template name '%s' already exists", *req.Name)
		}
	}

	// 构建更新数据
	updates := make(map[string]interface{})

	if req.Name != nil {
		updates["name"] = *req.Name
	}

	if req.Type != nil {
		updates["type"] = *req.Type
	}

	if req.Source != nil {
		updates["source"] = *req.Source
	}

	if req.Alias != nil {
		updates["alias"] = *req.Alias
	}

	if req.Config != nil {
		updates["config"] = models.JSONB(req.Config)
	}

	if req.Version != nil {
		updates["version"] = *req.Version
	}

	if req.ConstraintOp != nil {
		updates["constraint_op"] = *req.ConstraintOp
	}

	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}

	if req.Description != nil {
		updates["description"] = *req.Description
	}

	if len(updates) > 0 {
		updates["updated_at"] = time.Now()
		err = s.db.Model(&models.ProviderTemplate{}).Where("id = ?", id).Updates(updates).Error
		if err != nil {
			return nil, fmt.Errorf("failed to update provider template: %w", err)
		}
	}

	// 返回更新后的模板
	return s.GetByID(id)
}

// SetDefault 设置默认Provider模板
func (s *ProviderTemplateService) SetDefault(id uint) (*models.ProviderTemplate, error) {
	// 检查模板是否存在
	template, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	// 检查模板是否启用
	if !template.Enabled {
		return nil, fmt.Errorf("cannot set disabled template as default")
	}

	// 使用事务确保原子性
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// 1. 取消同类型所有模板的默认状态
		if err := tx.Model(&models.ProviderTemplate{}).
			Where("type = ? AND is_default = ?", template.Type, true).
			Update("is_default", false).Error; err != nil {
			return fmt.Errorf("failed to clear default flags: %w", err)
		}

		// 2. 设置新的默认模板
		if err := tx.Model(&models.ProviderTemplate{}).
			Where("id = ?", id).
			Update("is_default", true).Error; err != nil {
			return fmt.Errorf("failed to set default template: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return s.GetByID(id)
}

// Delete 删除Provider模板
func (s *ProviderTemplateService) Delete(id uint) error {
	// 检查模板是否存在
	template, err := s.GetByID(id)
	if err != nil {
		return err
	}

	// 不允许删除默认模板
	if template.IsDefault {
		return fmt.Errorf("cannot delete default template, please set another template as default first")
	}

	// 检查是否有workspace在使用该模板
	if s.CheckTemplateInUse(id) {
		return fmt.Errorf("template is in use by workspaces")
	}

	result := s.db.Delete(&models.ProviderTemplate{}, id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete provider template: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("provider template not found")
	}

	return nil
}

// CheckTemplateInUse 检查模板是否被workspace使用
func (s *ProviderTemplateService) CheckTemplateInUse(id uint) bool {
	var count int64
	s.db.Model(&models.Workspace{}).
		Where("provider_template_ids @> ?::jsonb", fmt.Sprintf("[%d]", id)).
		Count(&count)
	return count > 0
}

// ResolveProviderConfig 解析Provider配置
func (s *ProviderTemplateService) ResolveProviderConfig(templateIDs []uint, overrides map[string]interface{}) (map[string]interface{}, error) {
	if len(templateIDs) == 0 {
		return nil, nil
	}

	templates, err := s.GetByIDs(templateIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to load provider templates: %w", err)
	}

	if len(templates) == 0 {
		return nil, nil
	}

	providerBlock := make(map[string][]interface{})
	requiredProviders := make(map[string]interface{})

	for _, tmpl := range templates {
		// 深拷贝Config
		config := deepCopyJSONB(tmpl.Config)

		// 应用覆盖（按模板ID查找，与前端发送的 provider_overrides 结构一致）
		tidStr := fmt.Sprintf("%d", tmpl.ID)
		if overrides != nil {
			if tmplOverrides, ok := overrides[tidStr]; ok {
				if overrideMap, ok := tmplOverrides.(map[string]interface{}); ok {
					for k, v := range overrideMap {
						config[k] = v
					}
				}
			}
		}

		// 如果模板设置了 alias，注入到 config 中（Terraform provider alias）
		if tmpl.Alias != "" {
			config["alias"] = tmpl.Alias
		}

		// 追加到同类型 provider 数组（支持多个同类型 provider，如多个 aws 用 alias 区分）
		providerBlock[tmpl.Type] = append(providerBlock[tmpl.Type], config)

		// required_providers 每种类型只需一条（source + version 约束相同）
		if _, exists := requiredProviders[tmpl.Type]; !exists {
			rpEntry := make(map[string]interface{})
			rpEntry["source"] = tmpl.Source
			if tmpl.Version != "" {
				if tmpl.ConstraintOp == "=" {
					rpEntry["version"] = tmpl.Version
				} else if tmpl.ConstraintOp != "" {
					rpEntry["version"] = tmpl.ConstraintOp + " " + tmpl.Version
				} else {
					rpEntry["version"] = tmpl.Version
				}
			}
			requiredProviders[tmpl.Type] = rpEntry
		}
	}

	// 转换 providerBlock 为 map[string]interface{} 以匹配 JSON 输出格式
	providerOut := make(map[string]interface{})
	for k, v := range providerBlock {
		providerOut[k] = v
	}

	result := map[string]interface{}{
		"provider": providerOut,
		"terraform": []interface{}{
			map[string]interface{}{
				"required_providers": []interface{}{requiredProviders},
			},
		},
	}

	return result, nil
}

// FilterTemplateSensitiveInfo 递归过滤敏感信息，支持嵌套map和数组
func FilterTemplateSensitiveInfo(config map[string]interface{}) map[string]interface{} {
	if config == nil {
		return nil
	}

	result := make(map[string]interface{})
	for k, v := range config {
		result[k] = filterValue(k, v)
	}

	return result
}

// filterValue 根据字段名和值类型递归过滤
func filterValue(key string, value interface{}) interface{} {
	if isSensitiveKey(key) {
		if str, ok := value.(string); ok && len(str) > 0 {
			return "******"
		}
		return value
	}

	// 递归处理嵌套map
	if m, ok := value.(map[string]interface{}); ok {
		filtered := make(map[string]interface{})
		for k, v := range m {
			filtered[k] = filterValue(k, v)
		}
		return filtered
	}

	// 递归处理数组
	if arr, ok := value.([]interface{}); ok {
		filtered := make([]interface{}, len(arr))
		for i, item := range arr {
			if m, ok := item.(map[string]interface{}); ok {
				fm := make(map[string]interface{})
				for k, v := range m {
					fm[k] = filterValue(k, v)
				}
				filtered[i] = fm
			} else {
				filtered[i] = item
			}
		}
		return filtered
	}

	return value
}

// isSensitiveKey 判断字段名是否为敏感字段
func isSensitiveKey(fieldName string) bool {
	lower := strings.ToLower(fieldName)

	// 精确匹配
	sensitiveKeys := map[string]bool{
		"access_key":    true,
		"secret_key":    true,
		"secret_id":     true,
		"password":      true,
		"token":         true,
		"client_key":    true,
		"client_secret": true,
	}
	if sensitiveKeys[lower] {
		return true
	}

	// 模糊匹配：字段名包含关键词且长度大于关键词（避免误判）
	sensitiveKeywords := []string{"password", "secret", "token", "key"}
	for _, keyword := range sensitiveKeywords {
		if len(lower) > len(keyword) && strings.Contains(lower, keyword) {
			return true
		}
	}

	return false
}

// deepCopyJSONB 深拷贝JSONB
func deepCopyJSONB(src models.JSONB) map[string]interface{} {
	if src == nil {
		return make(map[string]interface{})
	}

	data, err := json.Marshal(src)
	if err != nil {
		return make(map[string]interface{})
	}

	var dst map[string]interface{}
	if err := json.Unmarshal(data, &dst); err != nil {
		return make(map[string]interface{})
	}

	return dst
}
