package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// StateService State 管理服务
type StateService struct {
	db *gorm.DB
}

// NewStateService 创建 State 服务
func NewStateService(db *gorm.DB) *StateService {
	return &StateService{
		db: db,
	}
}

// ValidateStateUpload 校验 State 上传
// 检查 lineage 和 serial 是否符合要求
func (s *StateService) ValidateStateUpload(newState map[string]interface{}, workspaceID string) error {
	// 1. 获取当前最新 state
	currentState, err := s.GetLatestStateVersion(workspaceID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to get current state: %w", err)
	}

	// 2. 提取新 state 的 lineage 和 serial
	newLineage, ok := newState["lineage"].(string)
	if !ok || newLineage == "" {
		return fmt.Errorf("state missing required field: lineage")
	}

	newSerialFloat, ok := newState["serial"].(float64)
	if !ok {
		return fmt.Errorf("state missing required field: serial")
	}
	newSerial := int(newSerialFloat)

	// 3. 如果是首次上传，不需要校验
	if currentState == nil {
		log.Printf("First state upload for workspace %s, skipping validation", workspaceID)
		return nil
	}

	// 4. Lineage 校验
	if currentState.Lineage != "" && newLineage != currentState.Lineage {
		return fmt.Errorf("lineage mismatch: expected %s, got %s. Use force=true to bypass validation",
			currentState.Lineage, newLineage)
	}

	// 5. Serial 校验（必须递增）
	if currentState.Serial > 0 && newSerial <= currentState.Serial {
		return fmt.Errorf("serial must be greater than current (%d), got %d. Use force=true to bypass validation",
			currentState.Serial, newSerial)
	}

	log.Printf("State validation passed for workspace %s (lineage: %s, serial: %d -> %d)",
		workspaceID, newLineage, currentState.Serial, newSerial)
	return nil
}

// UploadState 上传 State
// force: 是否强制上传（跳过校验）
func (s *StateService) UploadState(
	stateContent map[string]interface{},
	workspaceID string,
	userID string,
	force bool,
	description string,
) (*models.WorkspaceStateVersion, error) {
	// 1. 先锁定 workspace（防止并发修改）
	lockReason := "State upload in progress"
	if err := s.lockWorkspace(workspaceID, userID, lockReason); err != nil {
		return nil, fmt.Errorf("failed to lock workspace: %w", err)
	}

	// 2. 确保函数退出时释放锁（使用 defer）
	shouldAutoUnlock := !force // 正常上传自动释放，强制上传保持锁定
	defer func() {
		if shouldAutoUnlock {
			if unlockErr := s.unlockWorkspace(workspaceID); unlockErr != nil {
				log.Printf("Warning: failed to unlock workspace %s: %v", workspaceID, unlockErr)
			}
		} else {
			// 强制上传：更新锁定原因
			s.updateLockReason(workspaceID,
				"Locked after force upload. Please verify state before unlocking.")
		}
	}()

	// 3. 校验（除非 force=true）
	if !force {
		if err := s.ValidateStateUpload(stateContent, workspaceID); err != nil {
			// 校验失败，锁会被 defer 自动释放
			return nil, err
		}
	} else {
		log.Printf("WARNING: Force uploading state for workspace %s, bypassing validation", workspaceID)
	}

	// 4. 提取 lineage 和 serial
	lineage, _ := stateContent["lineage"].(string)
	serialFloat, _ := stateContent["serial"].(float64)
	serial := int(serialFloat)

	// 5. 计算 checksum 和大小
	stateBytes, err := json.Marshal(stateContent)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}
	checksum := s.calculateChecksum(stateBytes)
	sizeBytes := len(stateBytes)

	// 6. 获取下一个版本号
	maxVersion, err := s.getMaxStateVersion(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get max version: %w", err)
	}
	newVersion := maxVersion + 1

	// 7. 创建新版本
	stateVersion := &models.WorkspaceStateVersion{
		WorkspaceID:  workspaceID,
		Content:      models.JSONB(stateContent),
		Version:      newVersion,
		Checksum:     checksum,
		SizeBytes:    sizeBytes,
		Lineage:      lineage,
		Serial:       serial,
		IsImported:   true,          // 标记为导入
		ImportSource: "user_upload", // 来源：用户上传
		Description:  description,
		CreatedBy:    &userID,
	}

	// 8. 保存到数据库
	if err := s.db.Create(stateVersion).Error; err != nil {
		return nil, fmt.Errorf("failed to save state version: %w", err)
	}

	// 9. 更新 workspace 的 tf_state
	if err := s.db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Update("tf_state", models.JSONB(stateContent)).Error; err != nil {
		log.Printf("Warning: failed to update workspace tf_state: %v", err)
	}

	// 10. 记录审计日志
	s.logAudit("state_upload", workspaceID, userID,
		fmt.Sprintf("Uploaded state version %d (force=%v)", newVersion, force))

	log.Printf("State uploaded successfully: workspace=%s, version=%d, size=%d bytes, force=%v",
		workspaceID, newVersion, sizeBytes, force)

	return stateVersion, nil
}

// RollbackState 回滚 State 到指定版本
// force: 是否强制回滚（跳过 lineage/serial 校验）
// 回滚成功后自动解锁 workspace
func (s *StateService) RollbackState(
	workspaceID string,
	targetVersion int,
	userID string,
	reason string,
	force bool,
) (*models.WorkspaceStateVersion, error) {
	// 1. 先锁定 workspace
	lockReason := "State rollback in progress"
	if err := s.lockWorkspace(workspaceID, userID, lockReason); err != nil {
		return nil, fmt.Errorf("failed to lock workspace: %w", err)
	}

	// 2. 回滚成功后自动解锁 workspace
	rollbackSuccess := false
	defer func() {
		if rollbackSuccess {
			// 回滚成功，自动解锁
			if unlockErr := s.unlockWorkspace(workspaceID); unlockErr != nil {
				log.Printf("Warning: failed to unlock workspace %s after rollback: %v", workspaceID, unlockErr)
			}
		} else {
			// 回滚失败，也解锁（恢复原状）
			if unlockErr := s.unlockWorkspace(workspaceID); unlockErr != nil {
				log.Printf("Warning: failed to unlock workspace %s after failed rollback: %v", workspaceID, unlockErr)
			}
		}
	}()

	// 3. 获取目标版本
	var targetState models.WorkspaceStateVersion
	if err := s.db.Where("workspace_id = ? AND version = ?", workspaceID, targetVersion).
		First(&targetState).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("target version %d not found", targetVersion)
		}
		return nil, fmt.Errorf("failed to get target version: %w", err)
	}

	// 4. 获取下一个版本号
	maxVersion, err := s.getMaxStateVersion(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get max version: %w", err)
	}
	newVersion := maxVersion + 1

	// 5. 如果不是强制回滚，需要校验 lineage 和 serial
	// 回滚本质上是把旧版本的 state 重新导入，所以使用和 import 相同的校验逻辑：
	// - Serial 必须大于当前版本（但回滚的目标版本 serial 通常小于当前版本，所以会失败）
	// - Lineage 必须一致
	if !force {
		// 获取当前最新版本
		latestState, err := s.GetLatestStateVersion(workspaceID)
		if err != nil {
			return nil, fmt.Errorf("failed to get latest state: %w", err)
		}

		if latestState != nil {
			// Lineage 校验
			if latestState.Lineage != "" && targetState.Lineage != latestState.Lineage {
				return nil, fmt.Errorf("lineage mismatch: current is %s, target is %s. Use force=true to bypass validation",
					latestState.Lineage, targetState.Lineage)
			}

			// Serial 校验（与 import 相同：目标版本的 serial 必须大于当前版本）
			// 但回滚的目标版本 serial 通常小于当前版本，所以这个校验通常会失败
			// 这是预期行为：回滚是危险操作，需要用户明确使用 force
			if targetState.Serial <= latestState.Serial {
				return nil, fmt.Errorf("serial must be greater than current (%d), target is %d. Use force=true to bypass validation",
					latestState.Serial, targetState.Serial)
			}
		}
	} else {
		log.Printf("WARNING: Force rolling back state for workspace %s, bypassing validation", workspaceID)
	}

	// 6. 创建新版本（标记为 rollback）
	description := fmt.Sprintf("从版本 #%d 回滚", targetVersion)
	if reason != "" {
		description += ": " + reason
	}

	// 获取当前最新版本的 serial，确保新版本的 serial 递增
	latestState, _ := s.GetLatestStateVersion(workspaceID)
	newSerial := targetState.Serial
	if latestState != nil && latestState.Serial >= newSerial {
		newSerial = latestState.Serial + 1
	}

	// 注意：RollbackFromVersion 存储的是版本号，不是数据库 ID
	// 这样前端可以直接显示 "Rollback from #120"
	rollbackFromVersion := uint(targetVersion)
	newState := &models.WorkspaceStateVersion{
		WorkspaceID:         workspaceID,
		Content:             targetState.Content,
		Version:             newVersion,
		Checksum:            targetState.Checksum,
		SizeBytes:           targetState.SizeBytes,
		Lineage:             targetState.Lineage,
		Serial:              newSerial, // 使用递增后的 serial
		IsImported:          true,      // 回滚也标记为导入（与 import 逻辑一致）
		ImportSource:        "rollback",
		IsRollback:          true,
		RollbackFromVersion: &rollbackFromVersion, // 存储版本号，不是 ID
		Description:         description,
		CreatedBy:           &userID,
	}

	// 7. 保存到数据库
	if err := s.db.Create(newState).Error; err != nil {
		return nil, fmt.Errorf("failed to save rollback version: %w", err)
	}

	// 8. 更新 workspace 的 tf_state
	if err := s.db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Update("tf_state", targetState.Content).Error; err != nil {
		log.Printf("Warning: failed to update workspace tf_state: %v", err)
	}

	// 9. 记录审计日志
	s.logAudit("state_rollback", workspaceID, userID,
		fmt.Sprintf("Rolled back to version %d (force=%v). Reason: %s", targetVersion, force, reason))

	// 标记回滚成功，触发 defer 中的自动解锁
	rollbackSuccess = true

	log.Printf("State rolled back successfully: workspace=%s, from_version=%d, new_version=%d",
		workspaceID, targetVersion, newVersion)

	return newState, nil
}

// GetLatestStateVersion 获取最新的 State 版本
func (s *StateService) GetLatestStateVersion(workspaceID string) (*models.WorkspaceStateVersion, error) {
	var state models.WorkspaceStateVersion
	err := s.db.Where("workspace_id = ?", workspaceID).
		Order("version DESC").
		First(&state).Error

	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}

	return &state, err
}

// GetStateVersion 获取指定版本的 State
func (s *StateService) GetStateVersion(workspaceID string, version int) (*models.WorkspaceStateVersion, error) {
	var state models.WorkspaceStateVersion
	err := s.db.Where("workspace_id = ? AND version = ?", workspaceID, version).
		First(&state).Error
	return &state, err
}

// ListStateVersions 列出 State 版本历史
func (s *StateService) ListStateVersions(workspaceID string, limit, offset int) ([]models.WorkspaceStateVersion, int64, error) {
	var versions []models.WorkspaceStateVersion
	var total int64

	// 获取总数
	if err := s.db.Model(&models.WorkspaceStateVersion{}).
		Where("workspace_id = ?", workspaceID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 获取列表
	err := s.db.Where("workspace_id = ?", workspaceID).
		Order("version DESC").
		Limit(limit).
		Offset(offset).
		Find(&versions).Error

	return versions, total, err
}

// StateVersionWithUsername State 版本（包含用户名）
type StateVersionWithUsername struct {
	models.WorkspaceStateVersion
	CreatedByName string `json:"created_by_name"`
}

// ListStateVersionsWithUsernames 列出 State 版本历史（包含用户名）
func (s *StateService) ListStateVersionsWithUsernames(workspaceID string, limit, offset int) ([]StateVersionWithUsername, int64, error) {
	var versions []models.WorkspaceStateVersion
	var total int64

	// 获取总数
	if err := s.db.Model(&models.WorkspaceStateVersion{}).
		Where("workspace_id = ?", workspaceID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 获取列表
	err := s.db.Where("workspace_id = ?", workspaceID).
		Order("version DESC").
		Limit(limit).
		Offset(offset).
		Find(&versions).Error

	if err != nil {
		return nil, 0, err
	}

	// 收集所有用户 ID
	userIDs := make([]string, 0)
	for _, v := range versions {
		if v.CreatedBy != nil && *v.CreatedBy != "" {
			userIDs = append(userIDs, *v.CreatedBy)
		}
	}

	// 批量查询用户名
	userNameMap := make(map[string]string)
	if len(userIDs) > 0 {
		var users []models.User
		s.db.Where("user_id IN ?", userIDs).Select("user_id", "username").Find(&users)
		for _, u := range users {
			userNameMap[u.ID] = u.Username
		}
	}

	// 构建返回结果
	result := make([]StateVersionWithUsername, len(versions))
	for i, v := range versions {
		createdByName := "System"
		if v.CreatedBy != nil && *v.CreatedBy != "" {
			if name, ok := userNameMap[*v.CreatedBy]; ok && name != "" {
				createdByName = name
			} else {
				// 如果找不到用户名，显示用户 ID 的前 8 位
				if len(*v.CreatedBy) > 8 {
					createdByName = (*v.CreatedBy)[:8] + "..."
				} else {
					createdByName = *v.CreatedBy
				}
			}
		}
		result[i] = StateVersionWithUsername{
			WorkspaceStateVersion: v,
			CreatedByName:         createdByName,
		}
	}

	return result, total, nil
}

// ============================================================================
// 辅助方法
// ============================================================================

// getMaxStateVersion 获取最大版本号
func (s *StateService) getMaxStateVersion(workspaceID string) (int, error) {
	var maxVersion int
	err := s.db.Model(&models.WorkspaceStateVersion{}).
		Where("workspace_id = ?", workspaceID).
		Select("COALESCE(MAX(version), 0)").
		Scan(&maxVersion).Error
	return maxVersion, err
}

// calculateChecksum 计算 checksum
func (s *StateService) calculateChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// lockWorkspace 锁定 workspace
func (s *StateService) lockWorkspace(workspaceID, userID, reason string) error {
	// 检查是否已锁定
	var workspace models.Workspace
	if err := s.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		return fmt.Errorf("workspace not found: %w", err)
	}

	if workspace.IsLocked {
		return fmt.Errorf("workspace is already locked by %s: %s",
			*workspace.LockedBy, workspace.LockReason)
	}

	// 锁定
	now := time.Now()
	return s.db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Updates(map[string]interface{}{
			"is_locked":   true,
			"locked_by":   userID,
			"locked_at":   now,
			"lock_reason": reason,
		}).Error
}

// unlockWorkspace 解锁 workspace
func (s *StateService) unlockWorkspace(workspaceID string) error {
	return s.db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Updates(map[string]interface{}{
			"is_locked":   false,
			"locked_by":   nil,
			"locked_at":   nil,
			"lock_reason": "",
		}).Error
}

// updateLockReason 更新锁定原因
func (s *StateService) updateLockReason(workspaceID, reason string) error {
	return s.db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Update("lock_reason", reason).Error
}

// logAudit 记录审计日志
func (s *StateService) logAudit(action, workspaceID, userID, details string) {
	// TODO: 实现审计日志记录
	// 可以写入 audit_logs 表或使用专门的审计日志服务
	log.Printf("[AUDIT] action=%s, workspace=%s, user=%s, details=%s",
		action, workspaceID, userID, details)
}

// LogStateAccess 记录 State 访问审计日志
// 当用户通过 RetrieveStateVersion 接口获取 State 内容时调用
func (s *StateService) LogStateAccess(workspaceID string, userID string, version int) {
	s.logAudit("state_access", workspaceID, userID,
		fmt.Sprintf("Retrieved state version %d content (contains sensitive data)", version))
}
