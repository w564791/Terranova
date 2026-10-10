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

	isDriftCheck := task.TaskType == models.TaskTypeDriftCheck

	// 2. 检查是否已有plan_json（优先使用已保存的plan_json）
	if task.PlanJSON != nil && len(task.PlanJSON) > 0 {
		log.Printf("Using existing plan_json from database for task %d", taskID)

		// 与 agent 上传路径同一推导(再次按平台敏感集合脱敏,幂等)
		n, err := s.StoreResourceChangesFromPlanJSON(&task)
		if err != nil {
			return fmt.Errorf("failed to store resource changes: %w", err)
		}
		log.Printf("Successfully parsed and stored %d resource changes for task %d", n, taskID)
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
	ps, psErr := PlanSensitivityForTask(s.db, &task)
	if psErr != nil {
		log.Printf("[WARN] plan redaction for task %d: platform sensitivity incomplete: %v", taskID, psErr)
	}
	planJSON = RedactPlanJSON(planJSON, ps)

	// 解析 resource_changes
	resourceChanges, err := s.parseResourceChanges(planJSON, isDriftCheck)
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
	planBytes, err := OpenTaskPlanData(task)
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("plan data unavailable: %w", err)
	}
	if err := os.WriteFile(planFile, planBytes, 0600); err != nil {
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
	// 与执行器同一 init 策略:不 -upgrade,只用本目录私有的插件缓存(不继承共享 TF_PLUGIN_CACHE_DIR)
	initCmd := exec.CommandContext(ctx, "terraform", terraformInitArgs(false)...)
	initCmd.Dir = workDir
	cacheDir, cacheErr := preparePerTaskPluginCache(workDir, true)
	if cacheErr != nil {
		cacheDir = ""
	}
	initCmd.Env = withPluginCache(os.Environ(), cacheDir)
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
// isDriftCheck: only drift_check tasks should fall back to resource_drift (TFE behavior).
func (s *PlanParserService) parseResourceChanges(planJSON map[string]interface{}, isDriftCheck bool) ([]*models.WorkspaceTaskResourceChange, error) {
	resourceChanges := []*models.WorkspaceTaskResourceChange{}

	changes, _ := planJSON["resource_changes"].([]interface{})

	// For refresh-only plans (drift check), resource_changes is all no-op.
	// Only fall back to resource_drift for drift_check tasks.
	// For plan/plan_and_apply, resource_drift is informational — not included (TFE behavior).
	if isDriftCheck {
		hasRealChanges := false
		for _, item := range changes {
			if rc, ok := item.(map[string]interface{}); ok {
				if ch, ok := rc["change"].(map[string]interface{}); ok {
					if actions, ok := ch["actions"].([]interface{}); ok {
						for _, a := range actions {
							if actionStr, ok := a.(string); ok && actionStr != "no-op" && actionStr != "read" {
								hasRealChanges = true
								break
							}
						}
					}
				}
			}
			if hasRealChanges {
				break
			}
		}
		if !hasRealChanges {
			if driftChanges, ok := planJSON["resource_drift"].([]interface{}); ok && len(driftChanges) > 0 {
				changes = driftChanges
			}
		}
	}

	if len(changes) == 0 {
		return resourceChanges, nil
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
			AfterUnknown:    convertToJSONB(change["after_unknown"]),
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

// ResourceChangesRedactionVersion the redaction rules of
// workspace_task_resource_changes values (column redaction_version). Rows are
// only ever derived by the platform from the redacted plan_json (or carry no
// values at all); bump this when RedactPlanJSON gains rules and the backfill
// (BackfillResourceChangeRedaction) re-derives every row.
const ResourceChangesRedactionVersion int16 = 1

// StoreResourceChangesFromPlanJSON replaces the task's resource changes with
// the ones derived from its stored plan_json. The stored plan is redacted on
// write; it is redacted again here (RedactPlanJSON is idempotent) with the
// task's platform-side sensitive set, so a legacy row written before the
// backfill never leaks either. Returns the number of rows stored.
func (s *PlanParserService) StoreResourceChangesFromPlanJSON(task *models.WorkspaceTask) (int, error) {
	if len(task.PlanJSON) == 0 {
		return 0, fmt.Errorf("task %d has no plan_json", task.ID)
	}
	ps, psErr := PlanSensitivityForTask(s.db, task)
	if psErr != nil {
		log.Printf("[WARN] resource changes for task %d: platform sensitivity incomplete: %v", task.ID, psErr)
	}
	changes, err := s.parseResourceChanges(RedactPlanJSON(task.PlanJSON, ps), task.TaskType == models.TaskTypeDriftCheck)
	if err != nil {
		return 0, err
	}
	return len(changes), s.storeResourceChanges(task.WorkspaceID, task.ID, changes)
}

// ResourceChangeMeta identifies a resource change without its values.
type ResourceChangeMeta struct {
	ResourceAddress, ResourceType, ResourceName, ModuleAddress, Action string
}

// StoreResourceChangeMetadata replaces the task's resource changes with
// address / type / name / module / action only: before, after and
// after_unknown stay NULL. Used when no plan_json exists to derive values
// from (an older agent whose plan upload failed): agent-sent values are never
// stored, since they carry no before_sensitive / after_sensitive and provider
// sensitive attributes could not be masked.
func (s *PlanParserService) StoreResourceChangeMetadata(task *models.WorkspaceTask, metas []ResourceChangeMeta) (int, error) {
	changes := make([]*models.WorkspaceTaskResourceChange, 0, len(metas))
	for _, m := range metas {
		if m.ResourceAddress == "" || m.Action == "" || m.Action == "no-op" {
			continue
		}
		changes = append(changes, &models.WorkspaceTaskResourceChange{
			ResourceAddress: clipRunes(m.ResourceAddress, 500),
			ResourceType:    clipRunes(m.ResourceType, 100),
			ResourceName:    clipRunes(m.ResourceName, 200),
			ModuleAddress:   clipRunes(m.ModuleAddress, 500),
			Action:          clipRunes(m.Action, 20),
			ApplyStatus:     "pending",
		})
	}
	return len(changes), s.storeResourceChanges(task.WorkspaceID, task.ID, changes)
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
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
			v := ResourceChangesRedactionVersion
			change.RedactionVersion = &v
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
