package services

import (
	"bufio"
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"log"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ApplyParserService Apply输出解析服务
type ApplyParserService struct {
	db            *gorm.DB
	streamManager *OutputStreamManager
}

// NewApplyParserService 创建Apply解析服务
func NewApplyParserService(db *gorm.DB, streamManager *OutputStreamManager) *ApplyParserService {
	return &ApplyParserService{
		db:            db,
		streamManager: streamManager,
	}
}

// ApplyOutputParser Apply输出解析器
type ApplyOutputParser struct {
	taskID        uint
	db            *gorm.DB
	dataAccessor  DataAccessor
	streamManager *OutputStreamManager

	// 正则表达式
	creatingRegex   *regexp.Regexp
	modifyingRegex  *regexp.Regexp
	destroyingRegex *regexp.Regexp
	createdRegex    *regexp.Regexp
	modifiedRegex   *regexp.Regexp
	destroyedRegex  *regexp.Regexp
}

// NewApplyOutputParser 创建Apply输出解析器
func NewApplyOutputParser(taskID uint, db *gorm.DB, streamManager *OutputStreamManager) *ApplyOutputParser {
	var dataAccessor DataAccessor
	if db != nil {
		dataAccessor = NewLocalDataAccessor(db)
	}

	return &ApplyOutputParser{
		taskID:        taskID,
		db:            db,
		dataAccessor:  dataAccessor,
		streamManager: streamManager,

		// 编译正则表达式
		creatingRegex:   regexp.MustCompile(`^([a-zA-Z0-9_\-\.\[\]"]+):\s+Creating\.\.\.`),
		modifyingRegex:  regexp.MustCompile(`^([a-zA-Z0-9_\-\.\[\]"]+):\s+Modifying\.\.\.`),
		destroyingRegex: regexp.MustCompile(`^([a-zA-Z0-9_\-\.\[\]"]+):\s+Destroying\.\.\.`),
		createdRegex:    regexp.MustCompile(`^([a-zA-Z0-9_\-\.\[\]"]+):\s+Creation complete after`),
		modifiedRegex:   regexp.MustCompile(`^([a-zA-Z0-9_\-\.\[\]"]+):\s+Modifications complete after`),
		destroyedRegex:  regexp.MustCompile(`^([a-zA-Z0-9_\-\.\[\]"]+):\s+Destruction complete after`),
	}
}

// NewApplyOutputParserWithAccessor 创建Apply输出解析器（使用 DataAccessor）
func NewApplyOutputParserWithAccessor(taskID uint, dataAccessor DataAccessor, streamManager *OutputStreamManager) *ApplyOutputParser {
	return &ApplyOutputParser{
		taskID:        taskID,
		db:            nil,
		dataAccessor:  dataAccessor,
		streamManager: streamManager,

		// 编译正则表达式
		creatingRegex:   regexp.MustCompile(`^([a-zA-Z0-9_\-\.\[\]"]+):\s+Creating\.\.\.`),
		modifyingRegex:  regexp.MustCompile(`^([a-zA-Z0-9_\-\.\[\]"]+):\s+Modifying\.\.\.`),
		destroyingRegex: regexp.MustCompile(`^([a-zA-Z0-9_\-\.\[\]"]+):\s+Destroying\.\.\.`),
		createdRegex:    regexp.MustCompile(`^([a-zA-Z0-9_\-\.\[\]"]+):\s+Creation complete after`),
		modifiedRegex:   regexp.MustCompile(`^([a-zA-Z0-9_\-\.\[\]"]+):\s+Modifications complete after`),
		destroyedRegex:  regexp.MustCompile(`^([a-zA-Z0-9_\-\.\[\]"]+):\s+Destruction complete after`),
	}
}

// ParseLine 解析单行Apply输出
func (p *ApplyOutputParser) ParseLine(line string) {
	line = strings.TrimSpace(line)

	// 检查是否是资源操作行
	if p.creatingRegex.MatchString(line) {
		matches := p.creatingRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			resourceAddress := matches[1]
			p.updateResourceStatus(resourceAddress, "applying", "create")
		}
	} else if p.modifyingRegex.MatchString(line) {
		matches := p.modifyingRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			resourceAddress := matches[1]
			p.updateResourceStatus(resourceAddress, "applying", "update")
		}
	} else if p.destroyingRegex.MatchString(line) {
		matches := p.destroyingRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			resourceAddress := matches[1]
			p.updateResourceStatus(resourceAddress, "applying", "delete")
		}
	} else if p.createdRegex.MatchString(line) {
		matches := p.createdRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			resourceAddress := matches[1]
			p.updateResourceStatus(resourceAddress, "completed", "create")
		}
	} else if p.modifiedRegex.MatchString(line) {
		matches := p.modifiedRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			resourceAddress := matches[1]
			p.updateResourceStatus(resourceAddress, "completed", "update")
		}
	} else if p.destroyedRegex.MatchString(line) {
		matches := p.destroyedRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			resourceAddress := matches[1]
			p.updateResourceStatus(resourceAddress, "completed", "delete")
		}
	}
}

// updateResourceStatus 更新资源状态
func (p *ApplyOutputParser) updateResourceStatus(resourceAddress, status, action string) {
	// 使用 DataAccessor 更新资源状态（支持 Local 和 Agent 模式）
	if p.dataAccessor == nil {
		log.Printf("[Warning] DataAccessor is nil, skipping resource status update for %s", resourceAddress)
		return
	}

	// 通过 DataAccessor 更新资源状态
	if err := p.dataAccessor.UpdateResourceStatus(p.taskID, resourceAddress, status, action); err != nil {
		log.Printf("Warning: Failed to update resource status for %s: %v", resourceAddress, err)
		return
	}

	// 如果是 Local 模式，还需要广播 WebSocket 更新
	if p.db != nil {
		// 重新加载资源以获取最新数据
		var resource models.WorkspaceTaskResourceChange
		if err := p.db.Where("task_id = ? AND resource_address = ?", p.taskID, resourceAddress).
			First(&resource).Error; err == nil {
			// 通过WebSocket推送状态更新
			p.broadcastResourceUpdate(&resource)
		}
	}

	log.Printf("Updated resource %s status to %s", resourceAddress, status)
}

// broadcastResourceUpdate 广播资源状态更新
func (p *ApplyOutputParser) broadcastResourceUpdate(resource *models.WorkspaceTaskResourceChange) {
	stream := p.streamManager.GetOrCreate(p.taskID)
	if stream == nil {
		return
	}

	// 构造更新消息 - 使用JSON格式的Line字段
	data := map[string]interface{}{
		"task_id":            p.taskID,
		"resource_id":        resource.ID,
		"resource_address":   resource.ResourceAddress,
		"apply_status":       resource.ApplyStatus,
		"action":             resource.Action,
		"apply_started_at":   resource.ApplyStartedAt,
		"apply_completed_at": resource.ApplyCompletedAt,
	}

	dataJSON, _ := json.Marshal(data)

	message := OutputMessage{
		Type:      "resource_status_update",
		Line:      string(dataJSON),
		Timestamp: time.Now(),
	}

	stream.Broadcast(message)
}

// ExtractResourceDetailsFromState 从State提取资源详情（只提取resource_id）
// 对于delete操作，从删除前的state提取ID；对于create/update，从删除后的state提取ID
func (s *ApplyParserService) ExtractResourceDetailsFromState(
	taskID uint,
	stateContent map[string]interface{},
	logger *TerraformLogger,
) error {
	logger.Debug("ExtractResourceDetailsFromState: Starting for task %d", taskID)

	// 获取所有待更新的资源
	var resources []models.WorkspaceTaskResourceChange
	if err := s.db.Where("task_id = ?", taskID).Find(&resources).Error; err != nil {
		return fmt.Errorf("failed to get resources: %w", err)
	}
	logger.Debug("ExtractResourceDetailsFromState: Found %d resources in database", len(resources))

	// 检查是否有delete操作
	hasDeleteAction := false
	for _, resource := range resources {
		if resource.Action == "delete" {
			hasDeleteAction = true
			break
		}
	}

	// 如果有delete操作，需要从旧的state（Apply之前）提取ID
	if hasDeleteAction {
		logger.Debug("ExtractResourceDetailsFromState: Task has delete actions, will extract IDs from previous state")

		// 获取当前任务
		var task models.WorkspaceTask
		if err := s.db.First(&task, taskID).Error; err != nil {
			logger.Error("ExtractResourceDetailsFromState: Failed to get task: %v", err)
		} else {
			// 获取Apply之前的最后一个state version（不包括当前任务创建的）
			var stateVersion models.WorkspaceStateVersion
			err := s.db.Where("workspace_id = ? AND task_id != ?", task.WorkspaceID, taskID).
				Order("version DESC").
				First(&stateVersion).Error

			if err == nil {
				logger.Debug("ExtractResourceDetailsFromState: Found previous state version %d (task_id=%v) for delete operations",
					stateVersion.Version, stateVersion.TaskID)
				// 使用旧的state提取delete资源的ID
				s.extractResourceIDsFromState(resources, stateVersion.Content, true, logger)
			} else {
				logger.Warn("ExtractResourceDetailsFromState: No previous state found for delete operations: %v", err)
			}
		}
	}

	// 从state中提取resources数组
	stateResources, ok := stateContent["resources"].([]interface{})
	if !ok {
		logger.Error("ExtractResourceDetailsFromState: No resources array in state")
		return nil
	}
	logger.Debug("ExtractResourceDetailsFromState: Found %d resources in state", len(stateResources))

	// 创建资源地址到state资源的映射
	stateResourceMap := make(map[string]map[string]interface{})
	for i, res := range stateResources {
		resMap, ok := res.(map[string]interface{})
		if !ok {
			logger.Debug("ExtractResourceDetailsFromState: Resource %d is not a map", i)
			continue
		}

		// 获取资源地址
		mode, _ := resMap["mode"].(string)
		resType, _ := resMap["type"].(string)
		name, _ := resMap["name"].(string)

		logger.Trace("ExtractResourceDetailsFromState: Processing state resource %d: mode=%s, type=%s, name=%s", i, mode, resType, name)

		// 跳过 data 资源
		if mode != "managed" {
			continue
		}

		// 获取 module 前缀
		modulePrefix := ""
		if module, ok := resMap["module"].(string); ok && module != "" {
			modulePrefix = module + "."
		}

		// 获取instances - 每个instance可能有不同的index_key
		if instances, ok := resMap["instances"].([]interface{}); ok && len(instances) > 0 {
			logger.Trace("ExtractResourceDetailsFromState: Found %d instances for %s.%s", len(instances), resType, name)
			for _, inst := range instances {
				instance, ok := inst.(map[string]interface{})
				if !ok {
					continue
				}

				// 构造完整地址，包括 index_key
				var address string
				if indexKey, ok := instance["index_key"]; ok {
					// 有索引的资源
					switch v := indexKey.(type) {
					case float64:
						address = fmt.Sprintf("%s%s.%s[%d]", modulePrefix, resType, name, int(v))
					case string:
						address = fmt.Sprintf("%s%s.%s[\"%s\"]", modulePrefix, resType, name, v)
					default:
						address = fmt.Sprintf("%s%s.%s", modulePrefix, resType, name)
					}
				} else {
					// 无索引的资源
					address = fmt.Sprintf("%s%s.%s", modulePrefix, resType, name)
				}

				stateResourceMap[address] = instance
				logger.Trace("ExtractResourceDetailsFromState: Mapped %s to state resource", address)
			}
		} else {
			logger.Trace("ExtractResourceDetailsFromState: No instances found for %s.%s", resType, name)
		}
	}

	logger.Debug("ExtractResourceDetailsFromState: Created state resource map with %d entries", len(stateResourceMap))

	// 更新每个资源的详情（只提取resource_id）
	for _, resource := range resources {
		logger.Trace("ExtractResourceDetailsFromState: Looking for resource address: %s", resource.ResourceAddress)

		// 尝试直接匹配
		stateResource, ok := stateResourceMap[resource.ResourceAddress]

		// 如果直接匹配失败，尝试去掉索引后匹配（例如 this[0] -> this）
		if !ok {
			// 移除末尾的 [数字] 索引
			addressWithoutIndex := regexp.MustCompile(`\[\d+\]$`).ReplaceAllString(resource.ResourceAddress, "")
			if addressWithoutIndex != resource.ResourceAddress {
				logger.Trace("ExtractResourceDetailsFromState: Trying without index: %s", addressWithoutIndex)
				stateResource, ok = stateResourceMap[addressWithoutIndex]
			}
		}

		if ok {
			logger.Trace("ExtractResourceDetailsFromState: Found match for %s", resource.ResourceAddress)

			// 提取attributes
			if attributes, ok := stateResource["attributes"].(map[string]interface{}); ok {
				logger.Trace("ExtractResourceDetailsFromState: Found attributes for %s", resource.ResourceAddress)

				// 只提取resource_id（通常是id字段）
				var resourceID *string
				if id, ok := attributes["id"].(string); ok {
					resourceID = &id
					resource.ResourceID = resourceID
					logger.Debug("ExtractResourceDetailsFromState: Extracted ID for %s: %s", resource.ResourceAddress, id)
				} else {
					logger.Debug("ExtractResourceDetailsFromState: No 'id' field in attributes for %s", resource.ResourceAddress)
				}

				// 【修改】不再存储resource_attributes，只保存resource_id
				// resource.ResourceAttributes = attributesJSONB

				// 更新数据库
				if err := s.db.Save(&resource).Error; err != nil {
					logger.Error("ExtractResourceDetailsFromState: Failed to save resource ID for %s: %v", resource.ResourceAddress, err)
				} else {
					if resourceID != nil {
						logger.Info("Updated resource ID for %s: %s", resource.ResourceAddress, *resourceID)
						// 【新增】通过 WebSocket 推送 Resource ID 更新
						s.broadcastResourceIDUpdate(taskID, &resource)
					} else {
						logger.Warn("No resource ID found for %s", resource.ResourceAddress)
					}
				}
			} else {
				logger.Debug("ExtractResourceDetailsFromState: No attributes found for %s", resource.ResourceAddress)
			}
		} else {
			logger.Warn("ExtractResourceDetailsFromState: No state resource found for address: %s", resource.ResourceAddress)
		}
	}

	return nil
}

// extractResourceIDsFromState 从指定的state中提取资源ID
// onlyDeleteActions: 如果为true，只处理delete操作的资源
func (s *ApplyParserService) extractResourceIDsFromState(
	resources []models.WorkspaceTaskResourceChange,
	stateContent map[string]interface{},
	onlyDeleteActions bool,
	logger *TerraformLogger,
) {
	// 从state中提取resources数组
	stateResources, ok := stateContent["resources"].([]interface{})
	if !ok {
		logger.Debug("extractResourceIDsFromState: No resources array in state")
		return
	}

	// 创建资源地址到state资源的映射
	stateResourceMap := make(map[string]map[string]interface{})
	for _, res := range stateResources {
		resMap, ok := res.(map[string]interface{})
		if !ok {
			continue
		}

		// 获取资源地址
		mode, _ := resMap["mode"].(string)
		resType, _ := resMap["type"].(string)
		name, _ := resMap["name"].(string)

		// 构造资源地址
		var address string
		if mode == "managed" {
			address = fmt.Sprintf("%s.%s", resType, name)
		}

		// 处理module
		if module, ok := resMap["module"].(string); ok && module != "" {
			address = fmt.Sprintf("%s.%s", module, address)
		}

		// 获取instances
		if instances, ok := resMap["instances"].([]interface{}); ok && len(instances) > 0 {
			if instance, ok := instances[0].(map[string]interface{}); ok {
				stateResourceMap[address] = instance
			}
		}
	}

	// 更新每个资源的详情
	for _, resource := range resources {
		// 如果只处理delete操作，跳过其他操作
		if onlyDeleteActions && resource.Action != "delete" {
			continue
		}

		// 尝试直接匹配
		stateResource, ok := stateResourceMap[resource.ResourceAddress]

		// 如果直接匹配失败，尝试去掉索引后匹配
		if !ok {
			addressWithoutIndex := regexp.MustCompile(`\[\d+\]$`).ReplaceAllString(resource.ResourceAddress, "")
			if addressWithoutIndex != resource.ResourceAddress {
				stateResource, ok = stateResourceMap[addressWithoutIndex]
			}
		}

		if ok {
			// 提取attributes
			if attributes, ok := stateResource["attributes"].(map[string]interface{}); ok {
				// 只提取resource_id
				if id, ok := attributes["id"].(string); ok {
					resource.ResourceID = &id

					// 更新数据库
					if err := s.db.Save(&resource).Error; err != nil {
						logger.Error("extractResourceIDsFromState: Failed to save resource ID for %s: %v", resource.ResourceAddress, err)
					} else {
						logger.Info("extractResourceIDsFromState: Updated resource ID for %s (action=%s): %s", resource.ResourceAddress, resource.Action, id)
					}
				}
			}
		}
	}
}

// WrapApplyOutputReader 包装Apply输出读取器，实时解析
func (s *ApplyParserService) WrapApplyOutputReader(
	taskID uint,
	reader *bufio.Scanner,
	originalHandler func(string),
) func(string) {
	parser := NewApplyOutputParser(taskID, s.db, s.streamManager)

	return func(line string) {
		// 调用原始处理器
		originalHandler(line)

		// 解析行以更新资源状态
		parser.ParseLine(line)
	}
}

// broadcastResourceIDUpdate 广播资源 ID 更新
func (s *ApplyParserService) broadcastResourceIDUpdate(taskID uint, resource *models.WorkspaceTaskResourceChange) {
	if s.streamManager == nil {
		return
	}

	stream := s.streamManager.GetOrCreate(taskID)
	if stream == nil {
		return
	}

	// 构造更新消息 - 使用 resource_id_update 类型
	data := map[string]interface{}{
		"task_id":          taskID,
		"id":               resource.ID,
		"resource_address": resource.ResourceAddress,
		"resource_id":      resource.ResourceID,
	}

	dataJSON, _ := json.Marshal(data)

	message := OutputMessage{
		Type:      "resource_id_update",
		Line:      string(dataJSON),
		Timestamp: time.Now(),
	}

	stream.Broadcast(message)
	log.Printf("[WebSocket] Broadcasted resource ID update for %s: %v", resource.ResourceAddress, resource.ResourceID)
}
