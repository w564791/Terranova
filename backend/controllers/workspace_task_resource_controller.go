package controllers

import (
	"iac-platform/internal/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetTaskResourceChangesWithDB 获取任务的资源变更列表（带DB参数）
// @Summary 获取任务资源变更列表
// @Description 获取任务的资源变更详情和摘要
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param task_id path int true "任务ID"
// @Success 200 {object} map[string]interface{} "成功返回资源变更列表"
// @Failure 400 {object} map[string]interface{} "无效的参数"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/resource-changes [get]
// @Security Bearer
func GetTaskResourceChangesWithDB(c *gin.Context, db *gorm.DB) {

	workspaceIDParam := c.Param("id")
	if workspaceIDParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
		return
	}

	taskID, err := strconv.ParseUint(c.Param("task_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	// 获取workspace (支持语义化ID和数字ID)
	var workspace models.Workspace
	err = db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
			return
		}
	}

	// 获取资源变更列表 (使用语义化ID)
	var changes []models.WorkspaceTaskResourceChange
	if err := db.Where("workspace_id = ? AND task_id = ?", workspace.WorkspaceID, taskID).
		Order("id ASC").
		Find(&changes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 计算摘要
	summary := computeSummary(changes)

	// 获取任务的 plan_json 中的 output_changes、action_invocations 和 actions
	var task models.WorkspaceTask
	var outputChanges interface{}
	var actionInvocations interface{}
	var actions []interface{}
	if err := db.Where("id = ?", taskID).First(&task).Error; err == nil {
		if task.PlanJSON != nil {
			if oc, ok := task.PlanJSON["output_changes"]; ok {
				// 对 sensitive output 进行脱敏处理
				outputChanges = maskSensitiveOutputChanges(oc)
			}
			// 获取 action_invocations (Terraform 1.14+ 新特性)
			if ai, ok := task.PlanJSON["action_invocations"]; ok {
				// 如果任务已完成 Apply，尝试从 state 中获取实际值来填充 config_unknown 字段
				if task.Status == "applied" {
					actionInvocations = enrichActionInvocationsFromState(db, workspace.WorkspaceID, ai)
				} else {
					actionInvocations = ai
				}
			}
			// 获取 actions 资源定义 (从 configuration.root_module.module_calls.*.module.actions)
			actions = extractActionsFromConfiguration(task.PlanJSON)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"summary":            summary,
		"resources":          changes,
		"output_changes":     outputChanges,
		"action_invocations": actionInvocations,
		"actions":            actions,
	})
}

// UpdateResourceApplyStatusWithDB 更新资源的Apply状态（带DB参数）
// @Summary 更新资源Apply状态
// @Description 更新资源的Apply执行状态
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param task_id path int true "任务ID"
// @Param resource_id path int true "资源ID"
// @Param request body object true "状态更新信息"
// @Success 200 {object} models.WorkspaceTaskResourceChange "更新成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 404 {object} map[string]interface{} "资源不存在"
// @Failure 500 {object} map[string]interface{} "更新失败"
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/resource-changes/{resource_id} [patch]
// @Security Bearer
func UpdateResourceApplyStatusWithDB(c *gin.Context, db *gorm.DB) {

	resourceID, err := strconv.ParseUint(c.Param("resource_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	var req struct {
		ApplyStatus      string `json:"apply_status"`
		ApplyStartedAt   string `json:"apply_started_at"`
		ApplyCompletedAt string `json:"apply_completed_at"`
		ApplyError       string `json:"apply_error"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取资源变更记录
	var change models.WorkspaceTaskResourceChange
	if err := db.First(&change, resourceID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Resource change not found"})
		return
	}

	// 更新状态
	updates := make(map[string]interface{})
	if req.ApplyStatus != "" {
		updates["apply_status"] = req.ApplyStatus
	}
	if req.ApplyStartedAt != "" {
		updates["apply_started_at"] = req.ApplyStartedAt
	}
	if req.ApplyCompletedAt != "" {
		updates["apply_completed_at"] = req.ApplyCompletedAt
	}
	if req.ApplyError != "" {
		updates["apply_error"] = req.ApplyError
	}

	if err := db.Model(&change).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 重新获取更新后的数据
	db.First(&change, resourceID)

	c.JSON(http.StatusOK, change)
}

// maskSensitiveOutputChanges 对 sensitive output 进行脱敏处理
// Terraform plan_json 中的 output_changes 格式:
//
//	{
//	  "output_name": {
//	    "actions": ["create"|"update"|"delete"],
//	    "before": <value>,
//	    "after": <value>,
//	    "after_unknown": bool,
//	    "before_sensitive": bool,
//	    "after_sensitive": bool
//	  }
//	}
func maskSensitiveOutputChanges(outputChanges interface{}) interface{} {
	if outputChanges == nil {
		return nil
	}

	// 尝试转换为 map
	ocMap, ok := outputChanges.(map[string]interface{})
	if !ok {
		return outputChanges
	}

	// 创建新的 map 来存储脱敏后的数据
	maskedOC := make(map[string]interface{})

	for outputName, outputData := range ocMap {
		outputMap, ok := outputData.(map[string]interface{})
		if !ok {
			maskedOC[outputName] = outputData
			continue
		}

		// 检查是否为 sensitive
		beforeSensitive, _ := outputMap["before_sensitive"].(bool)
		afterSensitive, _ := outputMap["after_sensitive"].(bool)

		// 创建脱敏后的 output 数据
		maskedOutput := make(map[string]interface{})
		for k, v := range outputMap {
			maskedOutput[k] = v
		}

		// 如果 before 是 sensitive，将其值设为 nil
		if beforeSensitive {
			maskedOutput["before"] = nil
		}

		// 如果 after 是 sensitive，将其值设为 nil
		if afterSensitive {
			maskedOutput["after"] = nil
		}

		maskedOC[outputName] = maskedOutput
	}

	return maskedOC
}

// extractActionsFromConfiguration 从 plan_json 的 configuration 中提取 actions 资源定义
// 路径: configuration.root_module.module_calls.*.module.actions
func extractActionsFromConfiguration(planJSON map[string]interface{}) []interface{} {
	var allActions []interface{}

	configuration, ok := planJSON["configuration"].(map[string]interface{})
	if !ok {
		return allActions
	}

	rootModule, ok := configuration["root_module"].(map[string]interface{})
	if !ok {
		return allActions
	}

	moduleCalls, ok := rootModule["module_calls"].(map[string]interface{})
	if !ok {
		return allActions
	}

	// 遍历所有 module_calls
	for moduleName, moduleData := range moduleCalls {
		moduleMap, ok := moduleData.(map[string]interface{})
		if !ok {
			continue
		}

		module, ok := moduleMap["module"].(map[string]interface{})
		if !ok {
			continue
		}

		actions, ok := module["actions"].([]interface{})
		if !ok {
			continue
		}

		// 为每个 action 添加完整的 module 地址前缀
		for _, action := range actions {
			actionMap, ok := action.(map[string]interface{})
			if !ok {
				continue
			}

			// 构建完整地址: module.<module_name>.<action_address>
			if address, ok := actionMap["address"].(string); ok {
				actionMap["full_address"] = "module." + moduleName + "." + address
			}
			actionMap["module_address"] = "module." + moduleName

			allActions = append(allActions, actionMap)
		}
	}

	return allActions
}

// computeSummary 计算资源变更摘要
func computeSummary(changes []models.WorkspaceTaskResourceChange) map[string]int {
	summary := map[string]int{
		"add":     0,
		"change":  0,
		"destroy": 0,
	}

	for _, change := range changes {
		switch change.Action {
		case "create":
			summary["add"]++
		case "update":
			summary["change"]++
		case "delete":
			summary["destroy"]++
		case "replace":
			// replace = 1 delete + 1 create
			summary["add"]++
			summary["destroy"]++
		}
	}

	return summary
}

// enrichActionInvocationsFromState 从 state 中获取实际值来填充 action_invocations 的 config_unknown 字段
// 对于 aws_sns_publish action，尝试从 state 中获取:
// - topic_arn: 从 aws_sns_topic 资源获取
// - message: 从触发资源的 input/output 获取
func enrichActionInvocationsFromState(db *gorm.DB, workspaceID string, actionInvocations interface{}) interface{} {
	aiList, ok := actionInvocations.([]interface{})
	if !ok {
		return actionInvocations
	}

	// 获取最新的 state
	var stateVersion models.WorkspaceStateVersion
	if err := db.Where("workspace_id = ?", workspaceID).
		Order("id DESC").
		First(&stateVersion).Error; err != nil {
		return actionInvocations
	}

	// 解析 state 中的资源
	stateResources := make(map[string]map[string]interface{})
	if stateVersion.Content != nil {
		if resources, ok := stateVersion.Content["resources"].([]interface{}); ok {
			for _, res := range resources {
				resMap, ok := res.(map[string]interface{})
				if !ok {
					continue
				}

				// 构建资源地址
				mode, _ := resMap["mode"].(string)
				resType, _ := resMap["type"].(string)
				resName, _ := resMap["name"].(string)
				moduleAddr, _ := resMap["module"].(string)

				var address string
				if moduleAddr != "" {
					address = moduleAddr + "." + resType + "." + resName
				} else {
					address = resType + "." + resName
				}

				// 获取第一个实例的属性
				if instances, ok := resMap["instances"].([]interface{}); ok && len(instances) > 0 {
					if instance, ok := instances[0].(map[string]interface{}); ok {
						if attrs, ok := instance["attributes"].(map[string]interface{}); ok {
							stateResources[address] = attrs
							// 同时存储带 mode 前缀的地址（用于 data 资源）
							if mode == "data" {
								stateResources["data."+resType+"."+resName] = attrs
							}
						}
					}
				}
			}
		}
	}

	// 遍历 action_invocations，填充 config_unknown 中的值
	enrichedList := make([]interface{}, len(aiList))
	for i, ai := range aiList {
		aiMap, ok := ai.(map[string]interface{})
		if !ok {
			enrichedList[i] = ai
			continue
		}

		// 复制原始数据
		enrichedAI := make(map[string]interface{})
		for k, v := range aiMap {
			enrichedAI[k] = v
		}

		// 获取 config_values 和 config_unknown
		configValues, _ := aiMap["config_values"].(map[string]interface{})
		configUnknown, _ := aiMap["config_unknown"].(map[string]interface{})

		if configValues == nil {
			configValues = make(map[string]interface{})
		}

		// 创建新的 config_values，包含从 state 获取的实际值
		newConfigValues := make(map[string]interface{})
		for k, v := range configValues {
			newConfigValues[k] = v
		}

		// 获取 action 类型
		actionType, _ := aiMap["type"].(string)
		actionAddress, _ := aiMap["address"].(string)

		// 根据 action 类型，尝试从 state 获取相关值
		if actionType == "aws_sns_publish" {
			// 获取 topic_arn
			if configUnknown != nil {
				if _, isUnknown := configUnknown["topic_arn"]; isUnknown {
					// 尝试从 state 中找到对应的 SNS topic
					// 通常 action 地址格式: module.xxx.action.aws_sns_publish.yyy
					// 对应的 topic 地址格式: module.xxx.aws_sns_topic.zzz
					for addr, attrs := range stateResources {
						if arn, ok := attrs["arn"].(string); ok {
							// 检查是否是同一个 module 下的 SNS topic
							if isSameModule(actionAddress, addr) && isSnsTopic(addr) {
								newConfigValues["topic_arn"] = arn
								break
							}
						}
					}
				}

				// 获取 message
				if _, isUnknown := configUnknown["message"]; isUnknown {
					// 尝试从触发资源获取 message
					if trigger, ok := aiMap["lifecycle_action_trigger"].(map[string]interface{}); ok {
						if triggerAddr, ok := trigger["triggering_resource_address"].(string); ok {
							// 查找触发资源的 output
							if attrs, ok := stateResources[triggerAddr]; ok {
								if output, ok := attrs["output"]; ok {
									// output 可能是一个 JSON 字符串
									newConfigValues["message"] = output
								} else if input, ok := attrs["input"]; ok {
									newConfigValues["message"] = input
								}
							}
						}
					}
				}
			}
		}

		enrichedAI["config_values"] = newConfigValues
		// 标记哪些值已经从 state 获取
		enrichedAI["config_resolved"] = true

		enrichedList[i] = enrichedAI
	}

	return enrichedList
}

// isSameModule 检查两个地址是否在同一个 module 下
func isSameModule(addr1, addr2 string) bool {
	// 提取 module 前缀
	getModulePrefix := func(addr string) string {
		// 格式: module.xxx.resource_type.name 或 module.xxx.action.type.name
		parts := splitAddress(addr)
		if len(parts) >= 2 && parts[0] == "module" {
			return "module." + parts[1]
		}
		return ""
	}

	return getModulePrefix(addr1) == getModulePrefix(addr2)
}

// splitAddress 分割资源地址
func splitAddress(addr string) []string {
	var parts []string
	current := ""
	for _, c := range addr {
		if c == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// isSnsTopic 检查地址是否是 SNS topic
func isSnsTopic(addr string) bool {
	parts := splitAddress(addr)
	for _, part := range parts {
		if part == "aws_sns_topic" {
			return true
		}
	}
	return false
}
