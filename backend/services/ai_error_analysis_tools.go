package services

import (
	"context"
	"fmt"
	"iac-platform/internal/models"
	"log"

	"gorm.io/gorm"
)

// ========== Error Analysis Agent Tools ==========

// QueryModuleInputsTool 从 plan_json 中查询 module 的用户输入参数
// plan_json -> configuration -> root_module -> module_calls -> [module_name] -> expressions
// 这些是用户实际传入模块的参数值，包含 plan 阶段 (known after apply) 的字段的原始输入
type QueryModuleInputsTool struct {
	planJSON models.JSONB
}

func NewQueryModuleInputsTool(planJSON models.JSONB) *QueryModuleInputsTool {
	return &QueryModuleInputsTool{planJSON: planJSON}
}

func (t *QueryModuleInputsTool) Name() string { return "query_module_inputs" }
func (t *QueryModuleInputsTool) Description() string {
	return "查询指定 module 的用户输入参数（从 Terraform plan JSON 的 configuration 中提取）。" +
		"返回用户传入模块的实际变量值，包括 plan 阶段标记为 (known after apply) 的字段的原始输入，" +
		"例如 bucket policy JSON、自定义配置等。这些信息在 resource_changes 的 after 中可能是 null，但在此处可以看到原始值。"
}
func (t *QueryModuleInputsTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"module_name": map[string]interface{}{
				"type":        "string",
				"description": "Module 名称（不含 module. 前缀），如 AWS_s3-bucket_my-bucket。可从资源地址 module.XXX.resource 中提取 XXX 部分",
			},
		},
		"required": []string{"module_name"},
	}
}

func (t *QueryModuleInputsTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	moduleName, _ := params["module_name"].(string)
	if moduleName == "" {
		return nil, fmt.Errorf("module_name is required")
	}

	if len(t.planJSON) == 0 {
		return map[string]interface{}{"found": false, "message": "plan JSON not available for this task"}, nil
	}

	// plan_json 是 JSONB (map[string]interface{})，直接使用
	config, ok := t.planJSON["configuration"].(map[string]interface{})
	if !ok {
		return map[string]interface{}{"found": false, "message": "no configuration section in plan JSON"}, nil
	}

	rootModule, ok := config["root_module"].(map[string]interface{})
	if !ok {
		return map[string]interface{}{"found": false, "message": "no root_module in configuration"}, nil
	}

	moduleCalls, ok := rootModule["module_calls"].(map[string]interface{})
	if !ok {
		return map[string]interface{}{"found": false, "message": "no module_calls in root_module"}, nil
	}

	moduleCall, ok := moduleCalls[moduleName].(map[string]interface{})
	if !ok {
		// 列出可用的 module 名称帮助 AI 修正
		available := make([]string, 0, len(moduleCalls))
		for name := range moduleCalls {
			available = append(available, name)
		}
		return map[string]interface{}{
			"found":             false,
			"message":           fmt.Sprintf("module '%s' not found in module_calls", moduleName),
			"available_modules": available,
		}, nil
	}

	expressions, ok := moduleCall["expressions"].(map[string]interface{})
	if !ok {
		return map[string]interface{}{"found": true, "module_name": moduleName, "expressions": map[string]interface{}{}, "message": "no expressions found"}, nil
	}

	// 提取每个 expression 的值（constant_value 或 references）
	inputs := make(map[string]interface{})
	for key, val := range expressions {
		expr, ok := val.(map[string]interface{})
		if !ok {
			inputs[key] = val
			continue
		}
		if cv, exists := expr["constant_value"]; exists {
			inputs[key] = cv
		} else if refs, exists := expr["references"]; exists {
			inputs[key] = map[string]interface{}{"references": refs}
		} else {
			inputs[key] = expr
		}
	}

	log.Printf("[QueryModuleInputs] module=%s found %d inputs", moduleName, len(inputs))
	return map[string]interface{}{
		"found":       true,
		"module_name": moduleName,
		"inputs":      inputs,
	}, nil
}

// QueryTaskResourceChangesTool 查询任务的资源变更详情
type QueryTaskResourceChangesTool struct {
	db     *gorm.DB
	taskID uint
}

func NewQueryTaskResourceChangesTool(db *gorm.DB, taskID uint) *QueryTaskResourceChangesTool {
	return &QueryTaskResourceChangesTool{db: db, taskID: taskID}
}

func (t *QueryTaskResourceChangesTool) Name() string { return "query_task_resource_changes" }
func (t *QueryTaskResourceChangesTool) Description() string {
	return "查询当前任务的资源变更记录。返回每个资源的变更动作、apply 状态、配置数据（changes_after）和错误信息。" +
		"注意：某些字段在 plan 阶段是 unknown（如依赖其他资源的字段），这些字段的 changes_after 值为 null，" +
		"需要配合 query_module_inputs 工具查看用户的原始输入来获取完整信息。"
}
func (t *QueryTaskResourceChangesTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"status_filter": map[string]interface{}{
				"type":        "string",
				"description": "按 apply_status 过滤：all（全部）、failed（失败和 applying 中的）、completed（已完成的）。默认 all",
				"enum":        []string{"all", "failed", "completed"},
			},
		},
	}
}

func (t *QueryTaskResourceChangesTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	statusFilter, _ := params["status_filter"].(string)
	if statusFilter == "" {
		statusFilter = "all"
	}

	query := t.db.Where("task_id = ?", t.taskID)
	switch statusFilter {
	case "failed":
		query = query.Where("apply_status IN ?", []string{"applying", "failed"})
	case "completed":
		query = query.Where("apply_status = ?", "completed")
	}

	var changes []models.WorkspaceTaskResourceChange
	if err := query.Order("id ASC").Limit(50).Find(&changes).Error; err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	type changeInfo struct {
		ResourceAddress string      `json:"resource_address"`
		ResourceType    string      `json:"resource_type"`
		ModuleAddress   string      `json:"module_address"`
		Action          string      `json:"action"`
		ApplyStatus     string      `json:"apply_status"`
		ApplyError      string      `json:"apply_error,omitempty"`
		ChangesAfter    interface{} `json:"changes_after,omitempty"`
	}

	result := make([]changeInfo, 0, len(changes))
	for _, rc := range changes {
		info := changeInfo{
			ResourceAddress: rc.ResourceAddress,
			ResourceType:    rc.ResourceType,
			ModuleAddress:   rc.ModuleAddress,
			Action:          rc.Action,
			ApplyStatus:     rc.ApplyStatus,
			ApplyError:      rc.ApplyError,
		}
		if len(rc.ChangesAfter) > 0 {
			info.ChangesAfter = map[string]interface{}(rc.ChangesAfter)
		}
		result = append(result, info)
	}

	log.Printf("[QueryTaskResourceChanges] taskID=%d filter=%s found=%d", t.taskID, statusFilter, len(result))
	return map[string]interface{}{
		"task_id":       t.taskID,
		"status_filter": statusFilter,
		"total":         len(result),
		"changes":       result,
	}, nil
}
