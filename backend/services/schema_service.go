package services

import (
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

type SchemaService struct {
	db *gorm.DB
}

func NewSchemaService(db *gorm.DB) *SchemaService {
	return &SchemaService{db: db}
}

func (s *SchemaService) GetSchemasByModuleID(moduleID uint) ([]models.Schema, error) {
	var schemas []models.Schema
	err := s.db.Where("module_id = ?", moduleID).Find(&schemas).Error
	return schemas, err
}

// GetSchemasByModuleIDAndVersion 获取模块指定版本的 Schema 列表
// 如果 versionID 为空，则返回默认版本的 Schema
func (s *SchemaService) GetSchemasByModuleIDAndVersion(moduleID uint, versionID string) ([]models.Schema, error) {
	var schemas []models.Schema

	if versionID != "" {
		// 按指定版本过滤
		err := s.db.Where("module_id = ? AND module_version_id = ?", moduleID, versionID).
			Order("created_at DESC").Find(&schemas).Error
		return schemas, err
	}

	// 不传 versionID：获取默认版本的 Schema
	var module models.Module
	if err := s.db.First(&module, moduleID).Error; err != nil {
		return nil, err
	}

	if module.DefaultVersionID != nil && *module.DefaultVersionID != "" {
		// 获取默认版本的 Schema
		err := s.db.Where("module_id = ? AND module_version_id = ?", moduleID, *module.DefaultVersionID).
			Order("created_at DESC").Find(&schemas).Error
		return schemas, err
	}

	// 兼容旧数据：如果没有默认版本，返回所有 Schema
	err := s.db.Where("module_id = ?", moduleID).Order("created_at DESC").Find(&schemas).Error
	return schemas, err
}

func (s *SchemaService) CreateSchema(moduleID uint, req *models.CreateSchemaRequest) (*models.Schema, error) {
	schemaData, err := json.Marshal(req.SchemaData)
	if err != nil {
		return nil, err
	}

	// 设置默认status
	status := req.Status
	if status == "" {
		status = "draft"
	}

	// 设置默认source_type
	sourceType := req.SourceType
	if sourceType == "" {
		sourceType = "json_import"
	}

	schema := &models.Schema{
		ModuleID:    moduleID,
		Version:     req.Version,
		Status:      status,
		AIGenerated: false,
		SourceType:  sourceType,
		SchemaData:  string(schemaData),
	}

	err = s.db.Create(schema).Error
	if err != nil {
		return nil, err
	}

	return schema, nil
}

func (s *SchemaService) GetSchemaByID(id uint) (*models.Schema, error) {
	var schema models.Schema
	err := s.db.First(&schema, id).Error
	return &schema, err
}

func (s *SchemaService) UpdateSchema(id uint, req *models.UpdateSchemaRequest) (*models.Schema, error) {
	var schema models.Schema
	err := s.db.First(&schema, id).Error
	if err != nil {
		return nil, err
	}

	if req.SchemaData != nil {
		schemaData, err := json.Marshal(req.SchemaData)
		if err != nil {
			return nil, err
		}
		schema.SchemaData = string(schemaData)
	}

	if req.Status != "" {
		schema.Status = req.Status
	}

	err = s.db.Save(&schema).Error
	if err != nil {
		return nil, err
	}

	return &schema, nil
}

// GenerateSchemaFromModule 基于Module文件动态生成Schema
func (s *SchemaService) GenerateSchemaFromModule(moduleID uint) (*models.Schema, error) {
	// 1. 获取Module信息和文件内容
	var module models.Module
	err := s.db.First(&module, moduleID).Error
	if err != nil {
		return nil, err
	}

	// 2. 检查Module文件是否已同步
	if module.ModuleFiles == nil {
		return nil, fmt.Errorf("module files not synced, please sync module first")
	}

	// 3. 解析Module文件生成Schema
	schemaData, err := s.parseModuleFilesToSchema(module)
	if err != nil {
		return nil, fmt.Errorf("failed to parse module files: %v", err)
	}

	// 4. 将Schema存储到数据库
	schemaDataBytes, err := json.Marshal(schemaData)
	if err != nil {
		return nil, err
	}

	// 5. 检查是否已存在active状态的Schema，如果有则设为deprecated
	err = s.db.Model(&models.Schema{}).Where("module_id = ? AND status = ?", moduleID, "active").Update("status", "deprecated").Error
	if err != nil {
		return nil, err
	}

	schema := &models.Schema{
		ModuleID:    moduleID,
		Version:     s.generateNextVersion(moduleID),
		Status:      "active",
		AIGenerated: true,
		SourceType:  "ai_generate",
		SchemaData:  string(schemaDataBytes),
	}

	err = s.db.Create(schema).Error
	if err != nil {
		return nil, err
	}

	return schema, nil
}

// parseModuleFilesToSchema 解析Module文件生成Schema
func (s *SchemaService) parseModuleFilesToSchema(module models.Module) (map[string]interface{}, error) {
	// 解析module_files JSONB字段
	var moduleFiles map[string]string

	// 解析module_files
	if module.ModuleFiles == nil {
		return nil, fmt.Errorf("module_files is null, please sync module first")
	}

	// 尝试解析module_files
	jsonBytes, err := json.Marshal(module.ModuleFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal module_files: %v", err)
	}

	err = json.Unmarshal(jsonBytes, &moduleFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal module_files: %v", err)
	}

	// 提取variables.tf文件内容
	variablesTf, exists := moduleFiles["variables.tf"]
	if !exists {
		return nil, fmt.Errorf("variables.tf not found in module files")
	}

	// 解析variables.tf生成Schema
	schemaData := s.parseVariablesFile(variablesTf)

	return schemaData, nil
}

// parseVariablesFile 解析variables.tf文件
func (s *SchemaService) parseVariablesFile(variablesTf string) map[string]interface{} {
	schemaData := make(map[string]interface{})

	// 简单的正则解析（实际项目中应使用HCL解析器）
	lines := strings.Split(variablesTf, "\n")
	var currentVar string
	var currentVarData map[string]interface{}

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// 匹配variable定义
		if strings.HasPrefix(line, "variable ") {
			// 保存上一个变量
			if currentVar != "" && currentVarData != nil {
				schemaData[currentVar] = currentVarData
			}

			// 提取变量名
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				currentVar = strings.Trim(parts[1], `"`)
				currentVarData = map[string]interface{}{
					"type":        "string", // 默认类型
					"required":    false,
					"description": "",
				}
			}
		}

		// 解析变量属性
		if currentVarData != nil {
			if strings.Contains(line, "type") && strings.Contains(line, "=") {
				// 正确区分TypeMap和TypeObject
				if strings.Contains(line, "map(string)") || strings.Contains(line, "map(any)") {
					// TypeMap: 用户可自由添加key-value对
					currentVarData["type"] = "map"
				} else if strings.Contains(line, "object(") {
					// TypeObject: 固定结构，用户无法自由添加key
					currentVarData["type"] = "object"
					currentVarData["properties"] = map[string]interface{}{}
				} else if strings.Contains(line, "string") {
					currentVarData["type"] = "string"
				} else if strings.Contains(line, "number") {
					currentVarData["type"] = "number"
				} else if strings.Contains(line, "bool") {
					currentVarData["type"] = "boolean"
				} else if strings.Contains(line, "list(") {
					currentVarData["type"] = "array"
					if strings.Contains(line, "list(object(") {
						currentVarData["items"] = map[string]interface{}{
							"type":       "object",
							"properties": map[string]interface{}{},
						}
					}
				}
			}

			if strings.Contains(line, "description") && strings.Contains(line, "=") {
				// 提取描述信息
				start := strings.Index(line, `"`)
				end := strings.LastIndex(line, `"`)
				if start != -1 && end != -1 && start < end {
					currentVarData["description"] = line[start+1 : end]
				}
			}

			if strings.Contains(line, "default") && strings.Contains(line, "=") {
				// 提取默认值
				if strings.Contains(line, "true") {
					currentVarData["default"] = true
				} else if strings.Contains(line, "false") {
					currentVarData["default"] = false
				} else if strings.Contains(line, `"`) {
					start := strings.Index(line, `"`)
					end := strings.LastIndex(line, `"`)
					if start != -1 && end != -1 && start < end {
						currentVarData["default"] = line[start+1 : end]
					}
				}
			}
		}
	}

	// 保存最后一个变量
	if currentVar != "" && currentVarData != nil {
		schemaData[currentVar] = currentVarData
	}

	return schemaData
}

// generateNextVersion 生成下一个版本号
func (s *SchemaService) generateNextVersion(moduleID uint) string {
	var latestSchema models.Schema
	err := s.db.Where("module_id = ?", moduleID).Order("created_at DESC").First(&latestSchema).Error
	if err != nil {
		return "1.0.0" // 首个版本
	}

	// 简单的版本递增逻辑
	parts := strings.Split(latestSchema.Version, ".")
	if len(parts) == 3 {
		if patch, err := strconv.Atoi(parts[2]); err == nil {
			return fmt.Sprintf("%s.%s.%d", parts[0], parts[1], patch+1)
		}
	}

	return "1.0.0"
}

// ProcessSchemasForResponse 处理Schema列表数据供前端使用
func (s *SchemaService) ProcessSchemasForResponse(schemas []models.Schema) ([]map[string]interface{}, error) {
	var result []map[string]interface{}

	for _, schema := range schemas {
		processedSchema, err := s.ProcessSchemaForResponse(&schema)
		if err != nil {
			return nil, err
		}
		result = append(result, processedSchema)
	}

	return result, nil
}

// ProcessSchemaForResponse 处理单个Schema数据供前端使用
func (s *SchemaService) ProcessSchemaForResponse(schema *models.Schema) (map[string]interface{}, error) {
	// 解析schema_data JSON
	var schemaData map[string]interface{}
	err := json.Unmarshal([]byte(schema.SchemaData), &schemaData)
	if err != nil {
		// 如果解析失败，使用空对象
		schemaData = make(map[string]interface{})
	}

	// 构建响应数据
	result := map[string]interface{}{
		"id":             schema.ID,
		"module_id":      schema.ModuleID,
		"version":        schema.Version,
		"status":         schema.Status,
		"ai_generated":   schema.AIGenerated,
		"source_type":    schema.SourceType,
		"schema_data":    schemaData, // 解析后的JSON对象
		"schema_version": schema.SchemaVersion,
		"created_at":     schema.CreatedAt,
		"updated_at":     schema.UpdatedAt,
	}

	// 如果是 V2 Schema，添加 openapi_schema 和 ui_config
	if schema.SchemaVersion == "v2" {
		result["openapi_schema"] = schema.OpenAPISchema
		result["ui_config"] = schema.UIConfig
		result["variables_tf"] = schema.VariablesTF
	}

	return result, nil
}
