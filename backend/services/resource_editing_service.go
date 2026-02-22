package services

import (
	"errors"
	"fmt"
	"log"
	"time"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// EditorInfo 编辑者信息
type EditorInfo struct {
	UserID             string    `json:"user_id"`
	UserName           string    `json:"user_name"`
	SessionID          string    `json:"session_id"`
	IsSameUser         bool      `json:"is_same_user"`
	IsCurrentSession   bool      `json:"is_current_session"`
	LastHeartbeat      time.Time `json:"last_heartbeat"`
	TimeSinceHeartbeat int       `json:"time_since_heartbeat"` // 秒
}

// EditingStatusResponse 编辑状态响应
type EditingStatusResponse struct {
	IsLocked       bool         `json:"is_locked"`
	CurrentVersion int          `json:"current_version"`
	Editors        []EditorInfo `json:"editors"`
}

// StartEditingResponse 开始编辑响应
type StartEditingResponse struct {
	Lock            *models.ResourceLock  `json:"lock"`
	Drift           *models.ResourceDrift `json:"drift,omitempty"`
	OtherEditors    []EditorInfo          `json:"other_editors"` // 确保返回空数组而不是null
	HasDrift        bool                  `json:"has_drift"`
	VersionConflict bool                  `json:"has_version_conflict"`
}

// ResourceEditingService 资源编辑服务
type ResourceEditingService struct {
	db *gorm.DB
}

// NewResourceEditingService 创建资源编辑服务
func NewResourceEditingService(db *gorm.DB) *ResourceEditingService {
	return &ResourceEditingService{db: db}
}

// StartEditing 开始编辑
func (s *ResourceEditingService) StartEditing(
	resourceID uint,
	userID string,
	sessionID string,
) (*StartEditingResponse, error) {
	// 初始化response,确保OtherEditors是空数组而不是nil
	response := StartEditingResponse{
		OtherEditors: []EditorInfo{}, // 初始化为空数组
	}

	// 1. 获取资源当前版本
	var resource models.WorkspaceResource
	if err := s.db.Preload("CurrentVersion").First(&resource, resourceID).Error; err != nil {
		return nil, fmt.Errorf("资源不存在: %w", err)
	}

	currentVersion := 1
	if resource.CurrentVersion != nil {
		currentVersion = resource.CurrentVersion.Version
	}

	// 2. 检查是否有该用户的drift(按user_id查找,不限session_id)
	var drift models.ResourceDrift
	err := s.db.Where("resource_id = ? AND user_id = ? AND status = ?",
		resourceID, userID, "active").
		Order("updated_at DESC").
		First(&drift).Error

	if err == nil {
		// 有drift，检查版本冲突
		response.HasDrift = true
		response.Drift = &drift
		response.VersionConflict = drift.BaseVersion < currentVersion
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("查询drift失败: %w", err)
	}

	now := time.Now() // 使用本地时间，因为数据库列是 timestamp without time zone

	// 3. 查找或创建当前session的锁（使用FirstOrCreate避免重复创建）
	var currentLock models.ResourceLock
	err = s.db.Where("resource_id = ? AND session_id = ?", resourceID, sessionID).
		Assign(models.ResourceLock{
			EditingUserID: userID,
			LockType:      "optimistic",
			Version:       currentVersion,
			LastHeartbeat: now,
		}).
		FirstOrCreate(&currentLock, models.ResourceLock{
			ResourceID: resourceID,
			SessionID:  sessionID,
		}).Error

	if err != nil {
		return nil, fmt.Errorf("创建或更新锁失败: %w", err)
	}

	// 确保心跳时间是最新的
	if currentLock.LastHeartbeat.Before(now.Add(-5 * time.Second)) {
		currentLock.LastHeartbeat = now
		currentLock.Version = currentVersion
		currentLock.EditingUserID = userID
		if err := s.db.Save(&currentLock).Error; err != nil {
			return nil, fmt.Errorf("更新锁失败: %w", err)
		}
	}

	response.Lock = &currentLock

	// 4. 获取所有编辑者信息(包括其他session)
	var allLocks []models.ResourceLock
	s.db.Preload("EditingUser").Where("resource_id = ?", resourceID).Find(&allLocks)

	for _, l := range allLocks {
		if l.SessionID == sessionID {
			continue // 跳过当前session
		}

		// 跳过过期的锁（2分钟无心跳）
		if l.IsExpired() {
			// 清理过期的锁
			s.db.Delete(&l)
			continue
		}

		editor := EditorInfo{
			UserID:             l.EditingUserID,
			UserName:           l.EditingUser.Username,
			SessionID:          l.SessionID,
			IsSameUser:         l.EditingUserID == userID,
			IsCurrentSession:   false,
			LastHeartbeat:      l.LastHeartbeat,
			TimeSinceHeartbeat: int(time.Now().Sub(l.LastHeartbeat).Seconds()),
		}
		response.OtherEditors = append(response.OtherEditors, editor)
	}

	return &response, nil
}

// Heartbeat 心跳更新
func (s *ResourceEditingService) Heartbeat(
	resourceID uint,
	userID string,
	sessionID string,
) error {
	now := time.Now() // 使用本地时间，因为数据库列是 timestamp without time zone

	// 更新锁的心跳
	result := s.db.Model(&models.ResourceLock{}).
		Where("resource_id = ? AND editing_user_id = ? AND session_id = ?",
			resourceID, userID, sessionID).
		Update("last_heartbeat", now)

	if result.Error != nil {
		return fmt.Errorf("更新锁心跳失败: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		// 返回特殊错误，让controller返回404
		return gorm.ErrRecordNotFound
	}

	// 更新drift的心跳
	s.db.Model(&models.ResourceDrift{}).
		Where("resource_id = ? AND user_id = ? AND session_id = ? AND status = ?",
			resourceID, userID, sessionID, "active").
		Update("last_heartbeat", now)

	return nil
}

// EndEditing 结束编辑
func (s *ResourceEditingService) EndEditing(
	resourceID uint,
	userID string,
	sessionID string,
) error {
	// 删除锁
	result := s.db.Where("resource_id = ? AND editing_user_id = ? AND session_id = ?",
		resourceID, userID, sessionID).
		Delete(&models.ResourceLock{})

	if result.Error != nil {
		return fmt.Errorf("删除锁失败: %w", result.Error)
	}

	// 保留drift，用户可能需要恢复
	return nil
}

// GetEditingStatus 获取编辑状态
func (s *ResourceEditingService) GetEditingStatus(
	resourceID uint,
	currentUserID string,
	sessionID string,
) (*EditingStatusResponse, error) {
	// 初始化response,确保Editors是空数组
	response := EditingStatusResponse{
		Editors: []EditorInfo{},
	}

	// 获取资源当前版本
	var resource models.WorkspaceResource
	if err := s.db.Preload("CurrentVersion").First(&resource, resourceID).Error; err != nil {
		return nil, fmt.Errorf("资源不存在: %w", err)
	}

	response.CurrentVersion = 1
	if resource.CurrentVersion != nil {
		response.CurrentVersion = resource.CurrentVersion.Version
	}

	// 获取所有锁
	var locks []models.ResourceLock
	s.db.Preload("EditingUser").Where("resource_id = ?", resourceID).Find(&locks)

	// 过滤并清理过期的锁
	var activeLocks []models.ResourceLock
	for _, lock := range locks {
		if lock.IsExpired() {
			// 清理过期的锁
			s.db.Delete(&lock)
			continue
		}
		activeLocks = append(activeLocks, lock)
	}

	response.IsLocked = len(activeLocks) > 0

	for _, lock := range activeLocks {
		editor := EditorInfo{
			UserID:             lock.EditingUserID,
			UserName:           lock.EditingUser.Username,
			SessionID:          lock.SessionID,
			IsSameUser:         lock.EditingUserID == currentUserID,
			IsCurrentSession:   lock.SessionID == sessionID,
			LastHeartbeat:      lock.LastHeartbeat,
			TimeSinceHeartbeat: int(time.Now().Sub(lock.LastHeartbeat).Seconds()),
		}
		response.Editors = append(response.Editors, editor)
	}

	return &response, nil
}

// SaveDrift 保存草稿
func (s *ResourceEditingService) SaveDrift(
	resourceID uint,
	userID string,
	sessionID string,
	content map[string]interface{},
) (*models.ResourceDrift, error) {
	// 获取资源当前版本
	var resource models.WorkspaceResource
	if err := s.db.Preload("CurrentVersion").First(&resource, resourceID).Error; err != nil {
		return nil, fmt.Errorf("资源不存在: %w", err)
	}

	baseVersion := 1
	if resource.CurrentVersion != nil {
		baseVersion = resource.CurrentVersion.Version
	}

	now := time.Now() // 使用本地时间，因为数据库列是 timestamp without time zone

	// 查找现有drift
	var drift models.ResourceDrift
	err := s.db.Where("resource_id = ? AND user_id = ? AND session_id = ?",
		resourceID, userID, sessionID).First(&drift).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 创建新drift
		drift = models.ResourceDrift{
			ResourceID:    resourceID,
			UserID:        userID,
			SessionID:     sessionID,
			DriftContent:  content,
			BaseVersion:   baseVersion,
			Status:        "active",
			LastHeartbeat: now,
		}
		if err := s.db.Create(&drift).Error; err != nil {
			return nil, fmt.Errorf("创建drift失败: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("查询drift失败: %w", err)
	} else {
		// 更新现有drift
		drift.DriftContent = content
		drift.LastHeartbeat = now
		drift.Status = "active"
		if err := s.db.Save(&drift).Error; err != nil {
			return nil, fmt.Errorf("更新drift失败: %w", err)
		}
	}

	return &drift, nil
}

// GetDrift 获取草稿
func (s *ResourceEditingService) GetDrift(
	resourceID uint,
	userID string,
	sessionID string,
) (*models.ResourceDrift, bool, error) {
	var drift models.ResourceDrift
	err := s.db.Where("resource_id = ? AND user_id = ? AND session_id = ? AND status = ?",
		resourceID, userID, sessionID, "active").First(&drift).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, fmt.Errorf("查询drift失败: %w", err)
	}

	// 检查版本冲突
	var resource models.WorkspaceResource
	if err := s.db.Preload("CurrentVersion").First(&resource, resourceID).Error; err != nil {
		return nil, false, fmt.Errorf("资源不存在: %w", err)
	}

	currentVersion := 1
	if resource.CurrentVersion != nil {
		currentVersion = resource.CurrentVersion.Version
	}

	hasVersionConflict := drift.BaseVersion < currentVersion

	return &drift, hasVersionConflict, nil
}

// DeleteDrift 删除草稿
func (s *ResourceEditingService) DeleteDrift(
	resourceID uint,
	userID string,
	sessionID string,
) error {
	result := s.db.Where("resource_id = ? AND user_id = ? AND session_id = ?",
		resourceID, userID, sessionID).
		Delete(&models.ResourceDrift{})

	if result.Error != nil {
		return fmt.Errorf("删除drift失败: %w", result.Error)
	}

	return nil
}

// TakeoverEditing 接管编辑
func (s *ResourceEditingService) TakeoverEditing(
	resourceID uint,
	userID string,
	newSessionID string,
	oldSessionID string,
) error {
	now := time.Now() // 使用本地时间，因为数据库列是 timestamp without time zone

	// 开始事务
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 1. 删除该资源的所有锁（包括旧session和新session的锁）
		// 这样可以确保接管后只有一个锁
		if err := tx.Where("resource_id = ?", resourceID).Delete(&models.ResourceLock{}).Error; err != nil {
			return fmt.Errorf("删除所有锁失败: %w", err)
		}

		// 2. 为新session创建唯一的锁
		newLock := models.ResourceLock{
			ResourceID:    resourceID,
			EditingUserID: userID,
			SessionID:     newSessionID,
			LockType:      "optimistic",
			Version:       1,
			LastHeartbeat: now,
		}
		if err := tx.Create(&newLock).Error; err != nil {
			return fmt.Errorf("创建新锁失败: %w", err)
		}

		// 3. 将旧session的drift标记为expired
		tx.Model(&models.ResourceDrift{}).
			Where("resource_id = ? AND user_id = ? AND session_id = ?",
				resourceID, userID, oldSessionID).
			Update("status", "expired")

		// 4. 复制drift到新session（如果旧drift存在且新session没有drift）
		var oldDrift models.ResourceDrift
		err := tx.Where("resource_id = ? AND user_id = ? AND session_id = ?",
			resourceID, userID, oldSessionID).First(&oldDrift).Error

		if err == nil {
			// 检查新session是否已有drift
			var existingDrift models.ResourceDrift
			err := tx.Where("resource_id = ? AND user_id = ? AND session_id = ?",
				resourceID, userID, newSessionID).First(&existingDrift).Error

			if errors.Is(err, gorm.ErrRecordNotFound) {
				// 新session没有drift,复制旧drift
				newDrift := models.ResourceDrift{
					ResourceID:    resourceID,
					UserID:        userID,
					SessionID:     newSessionID,
					DriftContent:  oldDrift.DriftContent,
					BaseVersion:   oldDrift.BaseVersion,
					Status:        "active",
					LastHeartbeat: now,
				}
				if err := tx.Create(&newDrift).Error; err != nil {
					return fmt.Errorf("创建新drift失败: %w", err)
				}
			}
		}

		return nil
	})
}

// CleanupExpiredLocks 清理过期锁（后台任务）
// 注意：使用本地时间进行比较，因为数据库列是 timestamp without time zone
func (s *ResourceEditingService) CleanupExpiredLocks() error {
	result := s.db.Where("last_heartbeat < ?", time.Now().Add(-1*time.Minute)).
		Delete(&models.ResourceLock{})

	if result.Error != nil {
		return fmt.Errorf("清理过期锁失败: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		log.Printf("[ResourceEdit] Cleaned up %d expired locks", result.RowsAffected)
	}

	return nil
}

// CleanupOldDrifts 清理旧草稿（后台任务）
func (s *ResourceEditingService) CleanupOldDrifts() error {
	result := s.db.Where("status = ? AND updated_at < ?",
		"expired", time.Now().Add(-7*24*time.Hour)).
		Delete(&models.ResourceDrift{})

	if result.Error != nil {
		return fmt.Errorf("清理旧草稿失败: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		log.Printf("[ResourceEdit] Cleaned up %d old drafts", result.RowsAffected)
	}

	return nil
}

// RequestTakeover 请求接管编辑
func (s *ResourceEditingService) RequestTakeover(
	resourceID uint,
	requesterUserID string,
	requesterName string,
	requesterSessionID string,
	targetSessionID string,
) (*models.TakeoverRequest, error) {
	// 查找目标session的锁
	var targetLock models.ResourceLock
	err := s.db.Where("resource_id = ? AND session_id = ?",
		resourceID, targetSessionID).First(&targetLock).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 目标session不存在，可能已过期被清理
			// 这种情况下，直接执行接管，不需要等待确认
			return nil, errors.New("目标session不存在或已过期，请直接刷新页面重新编辑")
		}
		return nil, fmt.Errorf("查询目标session失败: %w", err)
	}

	// 创建接管请求
	// 过期时间设置为35秒，比前端倒计时30秒多5秒缓冲
	request := models.TakeoverRequest{
		ResourceID:       resourceID,
		RequesterUserID:  requesterUserID,
		RequesterName:    requesterName,
		RequesterSession: requesterSessionID,
		TargetUserID:     targetLock.EditingUserID,
		TargetSession:    targetSessionID,
		Status:           "pending",
		IsSameUser:       requesterUserID == targetLock.EditingUserID,
		ExpiresAt:        time.Now().Add(35 * time.Second),
	}

	if err := s.db.Create(&request).Error; err != nil {
		return nil, fmt.Errorf("创建接管请求失败: %w", err)
	}

	return &request, nil
}

// RespondToTakeover 响应接管请求
func (s *ResourceEditingService) RespondToTakeover(
	requestID uint,
	approved bool,
) error {
	var request models.TakeoverRequest
	if err := s.db.First(&request, requestID).Error; err != nil {
		return fmt.Errorf("接管请求不存在: %w", err)
	}

	// 检查是否已过期
	if request.IsExpired() {
		request.Status = "expired"
		s.db.Save(&request)
		return errors.New("请求已过期")
	}

	// 检查状态
	if request.Status != "pending" {
		return fmt.Errorf("请求状态无效: %s", request.Status)
	}

	if approved {
		request.Status = "approved"
		if err := s.db.Save(&request).Error; err != nil {
			return fmt.Errorf("更新请求状态失败: %w", err)
		}

		// 执行接管
		if err := s.TakeoverEditing(
			request.ResourceID,
			request.RequesterUserID,
			request.RequesterSession,
			request.TargetSession,
		); err != nil {
			return fmt.Errorf("执行接管失败: %w", err)
		}
	} else {
		request.Status = "rejected"
		if err := s.db.Save(&request).Error; err != nil {
			return fmt.Errorf("更新请求状态失败: %w", err)
		}
	}

	return nil
}

// GetPendingRequests 获取待处理的接管请求
func (s *ResourceEditingService) GetPendingRequests(
	targetSessionID string,
) ([]models.TakeoverRequest, error) {
	var requests []models.TakeoverRequest
	err := s.db.Where("target_session = ? AND status = ? AND expires_at > ?",
		targetSessionID, "pending", time.Now()).
		Order("created_at DESC").
		Find(&requests).Error

	if err != nil {
		return nil, fmt.Errorf("查询待处理请求失败: %w", err)
	}

	return requests, nil
}

// GetRequestStatus 获取请求状态
func (s *ResourceEditingService) GetRequestStatus(
	requestID uint,
) (*models.TakeoverRequest, error) {
	var request models.TakeoverRequest
	if err := s.db.First(&request, requestID).Error; err != nil {
		return nil, fmt.Errorf("请求不存在: %w", err)
	}

	// 如果是pending状态但已过期，自动执行接管（超时默认同意）
	if request.Status == "pending" && request.IsExpired() {
		// 尝试执行接管
		if err := s.TakeoverEditing(
			request.ResourceID,
			request.RequesterUserID,
			request.RequesterSession,
			request.TargetSession,
		); err != nil {
			// 接管失败，标记为expired
			log.Printf("[ResourceEdit] Timeout takeover failed: %v", err)
			request.Status = "expired"
		} else {
			// 接管成功，标记为approved
			log.Printf("[ResourceEdit] Timeout takeover successful: request_id=%d", requestID)
			request.Status = "approved"
		}
		s.db.Save(&request)
	}

	return &request, nil
}

// CleanupExpiredRequests 清理过期请求（后台任务）
func (s *ResourceEditingService) CleanupExpiredRequests() error {
	// 1. 将过期的pending请求标记为expired
	result := s.db.Model(&models.TakeoverRequest{}).
		Where("status = ? AND expires_at < ?", "pending", time.Now()).
		Update("status", "expired")

	if result.Error != nil {
		return fmt.Errorf("清理过期pending请求失败: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		log.Printf("[ResourceEdit] Marked %d expired pending requests", result.RowsAffected)
	}

	// 2. 删除7天前的历史记录（approved, rejected, expired）
	deleteResult := s.db.Where("status IN (?, ?, ?) AND created_at < ?",
		"approved", "rejected", "expired", time.Now().Add(-7*24*time.Hour)).
		Delete(&models.TakeoverRequest{})

	if deleteResult.Error != nil {
		return fmt.Errorf("删除历史记录失败: %w", deleteResult.Error)
	}

	if deleteResult.RowsAffected > 0 {
		log.Printf("[ResourceEdit] Deleted %d historical takeover request records", deleteResult.RowsAffected)
	}

	return nil
}
