package services

import (
	"context"
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"gorm.io/gorm"
)

// PlanParserService Plan数据解析服务
type PlanParserService struct {
	db *gorm.DB
}

// NewPlanParserService 创建Plan解析服务
func NewPlanParserService(db *gorm.DB) *PlanParserService {
	return &PlanParserService{
		db: db,
	}
}

// ParseAndStorePlanChanges 解析并存储Plan变更数据
func (s *PlanParserService) ParseAndStorePlanChanges(taskID uint) error {
	log.Printf("Starting to parse plan changes for task %d", taskID)

	// 1. 获取任务
	var task models.WorkspaceTask
	if err := s.db.First(&task, taskID).Error; err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	// 2. 检查是否已有plan_json（优先使用已保存的plan_json）
	if task.PlanJSON != nil && len(task.PlanJSON) > 0 {
		log.Printf("Using existing plan_json from database for task %d", taskID)

		// 直接使用数据库中的plan_json
		planJSON := map[string]interface{}(task.PlanJSON)

		// 解析 resource_changes
		resourceChanges, err := s.parseResourceChanges(planJSON)
		if err != nil {
			return fmt.Errorf("failed to parse resource changes: %w", err)
		}

		// 存储到数据库
		if err := s.storeResourceChanges(task.WorkspaceID, taskID, resourceChanges); err != nil {
			return fmt.Errorf("failed to store resource changes: %w", err)
		}

		log.Printf("Successfully parsed and stored %d resource changes for task %d", len(resourceChanges), taskID)
		return nil
	}

	// 3. 如果没有plan_json，尝试从plan_data生成（fallback）
	if len(task.PlanData) == 0 {
		return fmt.Errorf("task %d has no plan data", taskID)
	}

	log.Printf("No plan_json found, generating from plan_data for task %d", taskID)

	// 从数据库恢复plan文件
	planFile, err := s.restorePlanFile(&task)
	if err != nil {
		return fmt.Errorf("failed to restore plan file: %w", err)
	}
	defer os.Remove(planFile)
	defer os.RemoveAll(filepath.Dir(planFile)) // 清理临时目录

	// 执行 terraform show -json
	planJSON, err := s.executeTerraformShowJSON(planFile)
	if err != nil {
		return fmt.Errorf("failed to execute terraform show: %w", err)
	}

	// 解析 resource_changes
	resourceChanges, err := s.parseResourceChanges(planJSON)
	if err != nil {
		return fmt.Errorf("failed to parse resource changes: %w", err)
	}

	// 存储到数据库
	if err := s.storeResourceChanges(task.WorkspaceID, taskID, resourceChanges); err != nil {
		return fmt.Errorf("failed to store resource changes: %w", err)
	}

	log.Printf("Successfully parsed and stored %d resource changes for task %d", len(resourceChanges), taskID)
	return nil
}

// restorePlanFile 从数据库恢复plan文件到临时目录
func (s *PlanParserService) restorePlanFile(task *models.WorkspaceTask) (string, error) {
	// 创建临时目录
	tmpDir := fmt.Sprintf("/tmp/iac-platform/plan-parser/%s/%d", task.WorkspaceID, task.ID)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// 写入plan文件
	planFile := filepath.Join(tmpDir, "plan.out")
	if err := os.WriteFile(planFile, task.PlanData, 0644); err != nil {
		return "", fmt.Errorf("failed to write plan file: %w", err)
	}

	log.Printf("Restored plan file to %s (size: %d bytes)", planFile, len(task.PlanData))
	return planFile, nil
}

// executeTerraformShowJSON 执行 terraform show -json
func (s *PlanParserService) executeTerraformShowJSON(planFile string) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	workDir := filepath.Dir(planFile)

	// 先执行terraform init（plan文件需要provider）
	initCmd := exec.CommandContext(ctx, "terraform", "init", "-no-color")
	initCmd.Dir = workDir
	if err := initCmd.Run(); err != nil {
		log.Printf("Warning: terraform init failed: %v (continuing anyway)", err)
		// 不阻塞，继续尝试show
	}

	// 执行terraform show
	cmd := exec.CommandContext(ctx, "terraform", "show", "-json", planFile)
	cmd.Dir = workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("terraform show failed: %w\nOutput: %s", err, string(output))
	}

	var planJSON map[string]interface{}
	if err := json.Unmarshal(output, &planJSON); err != nil {
		return nil, fmt.Errorf("failed to parse plan JSON: %w", err)
	}

	return planJSON, nil
}

// parseResourceChanges 解析resource_changes数组
func (s *PlanParserService) parseResourceChanges(planJSON map[string]interface{}) ([]*models.WorkspaceTaskResourceChange, error) {
	resourceChanges := []*models.WorkspaceTaskResourceChange{}

	changes, ok := planJSON["resource_changes"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid plan JSON structure: resource_changes not found")
	}

	for _, item := range changes {
		rc, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		change, ok := rc["change"].(map[string]interface{})
		if !ok {
			continue
		}

		actions, ok := change["actions"].([]interface{})
		if !ok {
			continue
		}

		// 忽略 no-op
		if len(actions) == 1 {
			if actionStr, ok := actions[0].(string); ok && actionStr == "no-op" {
				continue
			}
		}

		// 判断操作类型
		action := s.determineAction(actions)

		// 提取资源信息
		resourceChange := &models.WorkspaceTaskResourceChange{
			ResourceAddress: getStringValue(rc, "address"),
			ResourceType:    getStringValue(rc, "type"),
			ResourceName:    getStringValue(rc, "name"),
			ModuleAddress:   getStringValue(rc, "module_address"),
			Action:          action,
			ChangesBefore:   convertToJSONB(change["before"]),
			ChangesAfter:    convertToJSONB(change["after"]),
			ApplyStatus:     "pending",
		}

		resourceChanges = append(resourceChanges, resourceChange)
	}

	return resourceChanges, nil
}

// determineAction 判断操作类型
func (s *PlanParserService) determineAction(actions []interface{}) string {
	if len(actions) == 1 {
		if action, ok := actions[0].(string); ok {
			return action
		}
	}

	// ["delete", "create"] = replace
	if len(actions) == 2 {
		action0, ok0 := actions[0].(string)
		action1, ok1 := actions[1].(string)
		if ok0 && ok1 && action0 == "delete" && action1 == "create" {
			return "replace"
		}
	}

	return "unknown"
}

// storeResourceChanges 存储资源变更到数据库
func (s *PlanParserService) storeResourceChanges(workspaceID string, taskID uint, changes []*models.WorkspaceTaskResourceChange) error {
	// 使用事务
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 删除该任务的旧数据（如果存在）
		if err := tx.Where("task_id = ?", taskID).Delete(&models.WorkspaceTaskResourceChange{}).Error; err != nil {
			return fmt.Errorf("failed to delete old changes: %w", err)
		}

		// 批量插入新数据
		for _, change := range changes {
			change.WorkspaceID = workspaceID // workspaceID 现在是 string
			change.TaskID = taskID
			if err := tx.Create(change).Error; err != nil {
				return fmt.Errorf("failed to create resource change: %w", err)
			}
		}

		return nil
	})
}

// getStringValue 安全获取字符串值
func getStringValue(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// convertToJSONB 转换为JSONB类型
func convertToJSONB(data interface{}) models.JSONB {
	if data == nil {
		return nil
	}

	if m, ok := data.(map[string]interface{}); ok {
		return models.JSONB(m)
	}

	return nil
}
