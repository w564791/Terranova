package services

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"iac-platform/internal/models"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"gorm.io/gorm"
)

// ============================================================================
// 数据结构定义
// ============================================================================

// PodSlot 槽位
type PodSlot struct {
	SlotID    int       `json:"slot_id"`   // 0, 1, 2
	TaskID    *uint     `json:"task_id"`   // 分配的任务ID
	TaskType  string    `json:"task_type"` // plan, plan_and_apply
	Status    string    `json:"status"`    // idle, reserved, running
	UpdatedAt time.Time `json:"updated_at"`
}

// ManagedPod 管理的Pod
type ManagedPod struct {
	PodName       string       `json:"pod_name"`
	AgentID       string       `json:"agent_id"`
	PoolID        string       `json:"pool_id"`
	Slots         [4]PodSlot   `json:"slots"` // 固定4个槽位
	CreatedAt     time.Time    `json:"created_at"`
	LastHeartbeat time.Time    `json:"last_heartbeat"`
	mu            sync.RWMutex // 保护槽位状态
}

// K8sPodManager Pod管理器
type K8sPodManager struct {
	db                    *gorm.DB
	clientset             *kubernetes.Clientset
	platformConfigService *PlatformConfigService // Platform configuration service
	pods                  map[string]*ManagedPod // podName -> ManagedPod
	mu                    sync.RWMutex           // 保护pods map
}

// ============================================================================
// 构造函数
// ============================================================================

// NewK8sPodManager 创建Pod管理器
func NewK8sPodManager(db *gorm.DB, clientset *kubernetes.Clientset) *K8sPodManager {
	return &K8sPodManager{
		db:        db,
		clientset: clientset,
		pods:      make(map[string]*ManagedPod),
	}
}

// NewK8sPodManagerWithConfig 创建带平台配置的Pod管理器
func NewK8sPodManagerWithConfig(db *gorm.DB, clientset *kubernetes.Clientset, platformConfigService *PlatformConfigService) *K8sPodManager {
	return &K8sPodManager{
		db:                    db,
		clientset:             clientset,
		platformConfigService: platformConfigService,
		pods:                  make(map[string]*ManagedPod),
	}
}

// ============================================================================
// Pod生命周期管理
// ============================================================================

// CreatePod 创建新的Agent Pod
func (m *K8sPodManager) CreatePod(ctx context.Context, poolID string, config *models.K8sJobTemplateConfig, secretName string) (*ManagedPod, error) {
	// 生成Pod名称: iac-agent-{pool_id}-{timestamp}
	podName := fmt.Sprintf("iac-agent-%s-%d", poolID, time.Now().Unix())
	namespace := "terraform"

	// 构建Pod spec
	pod := m.buildPodSpec(podName, namespace, poolID, config, secretName)

	// 创建Pod
	createdPod, err := m.clientset.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create pod: %w", err)
	}

	// 创建ManagedPod对象
	managedPod := &ManagedPod{
		PodName:       podName,
		AgentID:       "", // Agent启动后会注册并填充
		PoolID:        poolID,
		CreatedAt:     time.Now(),
		LastHeartbeat: time.Now(),
	}

	// 初始化4个空闲槽位
	for i := 0; i < 4; i++ {
		managedPod.Slots[i] = PodSlot{
			SlotID:    i,
			TaskID:    nil,
			TaskType:  "",
			Status:    "idle",
			UpdatedAt: time.Now(),
		}
	}

	// 添加到管理列表
	m.mu.Lock()
	m.pods[podName] = managedPod
	m.mu.Unlock()

	log.Printf("[PodManager] Created pod %s for pool %s", podName, poolID)
	log.Printf("[PodManager] Pod UID: %s", createdPod.UID)

	return managedPod, nil
}

// DeletePod 删除Pod
func (m *K8sPodManager) DeletePod(ctx context.Context, podName string) error {
	namespace := "terraform"

	// 检查Pod是否有运行中的任务
	m.mu.RLock()
	pod, exists := m.pods[podName]
	m.mu.RUnlock()

	if exists {
		pod.mu.RLock()
		hasRunningTasks := false
		for _, slot := range pod.Slots {
			if slot.Status == "running" || slot.Status == "reserved" {
				hasRunningTasks = true
				break
			}
		}
		pod.mu.RUnlock()

		if hasRunningTasks {
			return fmt.Errorf("cannot delete pod %s: has running or reserved tasks", podName)
		}
	}

	// 删除K8s Pod
	propagationPolicy := metav1.DeletePropagationBackground
	err := m.clientset.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	})

	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete pod: %w", err)
	}

	// 从管理列表中移除
	m.mu.Lock()
	delete(m.pods, podName)
	m.mu.Unlock()

	log.Printf("[PodManager] Deleted pod %s", podName)
	return nil
}

// ListPods 列出指定pool的所有Pod
func (m *K8sPodManager) ListPods(poolID string) []*ManagedPod {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var pods []*ManagedPod
	for _, pod := range m.pods {
		if pod.PoolID == poolID {
			pods = append(pods, pod)
		}
	}

	return pods
}

// FindIdlePods 查找完全空闲的Pod（所有槽位都是idle）
func (m *K8sPodManager) FindIdlePods(poolID string) []*ManagedPod {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var idlePods []*ManagedPod
	for _, pod := range m.pods {
		if pod.PoolID != poolID {
			continue
		}

		pod.mu.RLock()
		allIdle := true
		for _, slot := range pod.Slots {
			if slot.Status != "idle" {
				allIdle = false
				break
			}
		}
		pod.mu.RUnlock()

		if allIdle {
			idlePods = append(idlePods, pod)
		}
	}

	return idlePods
}

// ============================================================================
// 槽位管理
// ============================================================================

// FindPodWithFreeSlot 查找有空闲槽位的Pod
func (m *K8sPodManager) FindPodWithFreeSlot(poolID string, taskType string) (*ManagedPod, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, pod := range m.pods {
		if pod.PoolID != poolID {
			continue
		}

		// Check Agent heartbeat - only skip pods that are clearly offline
		// Don't check pod.AgentID registration as Agent may be connected via C&C but not yet registered in PodManager
		timeSinceHeartbeat := time.Since(pod.LastHeartbeat)

		// Only skip pods that haven't sent heartbeat in over 2 minutes (clearly offline)
		if timeSinceHeartbeat > 2*time.Minute {
			log.Printf("[PodManager] Pod %s is offline (last heartbeat: %v ago), skipping",
				pod.PodName, timeSinceHeartbeat)
			continue
		}

		pod.mu.RLock()

		// 如果是plan+apply任务，检查Pod上是否已有其他plan+apply任务（running或reserved）
		if taskType == string(models.TaskTypePlanAndApply) {
			hasOtherPlanAndApply := false
			for _, slot := range pod.Slots {
				if (slot.Status == "running" || slot.Status == "reserved") &&
					slot.TaskType == string(models.TaskTypePlanAndApply) {
					hasOtherPlanAndApply = true
					break
				}
			}

			if hasOtherPlanAndApply {
				pod.mu.RUnlock()
				log.Printf("[PodManager] Pod %s already has a plan+apply task, cannot accept another",
					pod.PodName)
				continue
			}
		}

		// 查找空闲槽位
		for i, slot := range pod.Slots {
			if slot.Status == "idle" {
				pod.mu.RUnlock()
				return pod, i, nil
			}
		}
		pod.mu.RUnlock()
	}

	return nil, -1, fmt.Errorf("no free slot available in pool %s", poolID)
}

// AssignTaskToSlot 分配任务到槽位
func (m *K8sPodManager) AssignTaskToSlot(podName string, slotID int, taskID uint, taskType string) error {
	m.mu.RLock()
	pod, exists := m.pods[podName]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("pod %s not found", podName)
	}

	if slotID < 0 || slotID > 3 {
		return fmt.Errorf("invalid slot ID: %d", slotID)
	}

	pod.mu.Lock()
	defer pod.mu.Unlock()

	// 检查槽位是否空闲
	if pod.Slots[slotID].Status != "idle" {
		return fmt.Errorf("slot %d is not idle (current status: %s)", slotID, pod.Slots[slotID].Status)
	}

	// 分配槽位
	pod.Slots[slotID] = PodSlot{
		SlotID:    slotID,
		TaskID:    &taskID,
		TaskType:  taskType,
		Status:    "running",
		UpdatedAt: time.Now(),
	}

	log.Printf("[PodManager] Assigned task %d (type: %s) to pod %s slot %d",
		taskID, taskType, podName, slotID)

	return nil
}

// ReleaseSlot 释放槽位
func (m *K8sPodManager) ReleaseSlot(podName string, slotID int) error {
	m.mu.RLock()
	pod, exists := m.pods[podName]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("pod %s not found", podName)
	}

	if slotID < 0 || slotID > 3 {
		return fmt.Errorf("invalid slot ID: %d", slotID)
	}

	pod.mu.Lock()
	defer pod.mu.Unlock()

	// 释放槽位
	oldTaskID := pod.Slots[slotID].TaskID
	pod.Slots[slotID] = PodSlot{
		SlotID:    slotID,
		TaskID:    nil,
		TaskType:  "",
		Status:    "idle",
		UpdatedAt: time.Now(),
	}

	if oldTaskID != nil {
		log.Printf("[PodManager] Released slot %d on pod %s (task %d completed)",
			slotID, podName, *oldTaskID)
	}

	return nil
}

// ReserveSlot 预留槽位（用于apply_pending任务）
func (m *K8sPodManager) ReserveSlot(podName string, slotID int, taskID uint) error {
	m.mu.RLock()
	pod, exists := m.pods[podName]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("pod %s not found", podName)
	}

	if slotID < 0 || slotID > 3 {
		return fmt.Errorf("invalid slot ID: %d", slotID)
	}

	pod.mu.Lock()
	defer pod.mu.Unlock()

	// 任何槽位都可以被预留（不再限制只有Slot 0）

	// 预留槽位
	pod.Slots[slotID] = PodSlot{
		SlotID:    slotID,
		TaskID:    &taskID,
		TaskType:  string(models.TaskTypePlanAndApply),
		Status:    "reserved",
		UpdatedAt: time.Now(),
	}

	log.Printf("[PodManager] Reserved slot %d on pod %s for task %d (apply_pending)",
		slotID, podName, taskID)

	return nil
}

// ============================================================================
// 槽位统计
// ============================================================================

// GetSlotStats 获取槽位统计信息
func (m *K8sPodManager) GetSlotStats(poolID string) (total, used, reserved, idle int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, pod := range m.pods {
		if pod.PoolID != poolID {
			continue
		}

		pod.mu.RLock()
		for _, slot := range pod.Slots {
			total++
			switch slot.Status {
			case "running":
				used++
			case "reserved":
				reserved++
			case "idle":
				idle++
			}
		}
		pod.mu.RUnlock()
	}

	return total, used, reserved, idle
}

// GetPodSlotStatus 获取Pod的槽位状态（用于监控和调试）
func (m *K8sPodManager) GetPodSlotStatus(podName string) ([]PodSlot, error) {
	m.mu.RLock()
	pod, exists := m.pods[podName]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("pod %s not found", podName)
	}

	pod.mu.RLock()
	defer pod.mu.RUnlock()

	slots := make([]PodSlot, 4)
	copy(slots, pod.Slots[:])

	return slots, nil
}

// ============================================================================
// Pod同步和协调
// ============================================================================

// SyncPodsFromK8s 从K8s同步Pod状态
// 同时检测并清理Failed状态的Pod
func (m *K8sPodManager) SyncPodsFromK8s(ctx context.Context, poolID string) error {
	namespace := "terraform"

	// 列出所有属于此pool的Pod
	labelSelector := fmt.Sprintf("app=iac-platform,component=agent,pool-id=%s", poolID)
	podList, err := m.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})

	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	log.Printf("[PodManager] Syncing %d pods from K8s for pool %s", len(podList.Items), poolID)

	// 收集需要清理的Failed Pod
	var failedPods []string

	// 更新或创建ManagedPod
	for _, k8sPod := range podList.Items {
		podName := k8sPod.Name

		// 检查Pod状态，如果是Failed或Succeeded，需要清理
		if k8sPod.Status.Phase == corev1.PodFailed || k8sPod.Status.Phase == corev1.PodSucceeded {
			log.Printf("[PodManager] Pod %s is in %s state, will be cleaned up", podName, k8sPod.Status.Phase)
			failedPods = append(failedPods, podName)
			continue
		}

		m.mu.Lock()
		managedPod, exists := m.pods[podName]
		if !exists {
			// 新发现的Pod，创建ManagedPod对象
			managedPod = &ManagedPod{
				PodName:       podName,
				AgentID:       "", // 等待Agent注册
				PoolID:        poolID,
				CreatedAt:     k8sPod.CreationTimestamp.Time,
				LastHeartbeat: time.Now(),
			}

			// 初始化槽位
			for i := 0; i < 4; i++ {
				managedPod.Slots[i] = PodSlot{
					SlotID:    i,
					TaskID:    nil,
					TaskType:  "",
					Status:    "idle",
					UpdatedAt: time.Now(),
				}
			}

			m.pods[podName] = managedPod
			log.Printf("[PodManager] Discovered new pod %s in K8s (phase: %s)", podName, k8sPod.Status.Phase)
		}
		m.mu.Unlock()

		// 更新Pod状态
		managedPod.LastHeartbeat = time.Now()
	}

	// 清理Failed/Succeeded状态的Pod
	for _, podName := range failedPods {
		log.Printf("[PodManager] Cleaning up failed/succeeded pod %s", podName)

		// 从管理列表中移除
		m.mu.Lock()
		delete(m.pods, podName)
		m.mu.Unlock()

		// 删除K8s中的Pod
		propagationPolicy := metav1.DeletePropagationBackground
		if err := m.clientset.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{
			PropagationPolicy: &propagationPolicy,
		}); err != nil && !errors.IsNotFound(err) {
			log.Printf("[PodManager] Warning: failed to delete failed pod %s: %v", podName, err)
		} else {
			log.Printf("[PodManager] Deleted failed/succeeded pod %s from K8s", podName)
		}
	}

	// 清理已删除的Pod
	m.mu.Lock()
	k8sPodNames := make(map[string]bool)
	for _, k8sPod := range podList.Items {
		// 只记录非Failed/Succeeded的Pod
		if k8sPod.Status.Phase != corev1.PodFailed && k8sPod.Status.Phase != corev1.PodSucceeded {
			k8sPodNames[k8sPod.Name] = true
		}
	}

	for podName := range m.pods {
		if m.pods[podName].PoolID == poolID && !k8sPodNames[podName] {
			log.Printf("[PodManager] Pod %s no longer exists in K8s, removing from management", podName)
			delete(m.pods, podName)
		}
	}
	m.mu.Unlock()

	return nil
}

// ReconcilePods 协调Pod状态（确保实际状态与期望状态一致）
func (m *K8sPodManager) ReconcilePods(ctx context.Context, poolID string) error {
	// 1. 从K8s同步Pod状态
	if err := m.SyncPodsFromK8s(ctx, poolID); err != nil {
		return fmt.Errorf("failed to sync pods: %w", err)
	}

	// 2. 从数据库同步任务状态到槽位
	if err := m.syncTaskStatusToSlots(ctx, poolID); err != nil {
		log.Printf("[PodManager] Warning: failed to sync task status: %v", err)
	}

	// 3. 清理过期的reserved槽位（超过24小时）
	m.cleanupExpiredReservations(poolID)

	return nil
}

// syncTaskStatusToSlots 从数据库同步任务状态到槽位
func (m *K8sPodManager) syncTaskStatusToSlots(ctx context.Context, poolID string) error {
	// 获取所有分配到此pool的running/apply_pending任务
	var tasks []models.WorkspaceTask
	err := m.db.WithContext(ctx).
		Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
		Where("workspaces.current_pool_id = ?", poolID).
		Where("workspace_tasks.status IN (?)", []models.TaskStatus{
			models.TaskStatusRunning,
			models.TaskStatusApplyPending,
		}).
		Where("workspace_tasks.agent_id IS NOT NULL AND workspace_tasks.agent_id != ''").
		Find(&tasks).Error

	if err != nil {
		return fmt.Errorf("failed to query tasks: %w", err)
	}

	// 构建任务ID到状态的映射
	taskStatusMap := make(map[uint]models.TaskStatus)
	for _, task := range tasks {
		taskStatusMap[task.ID] = task.Status
	}

	// 更新槽位状态
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, pod := range m.pods {
		if pod.PoolID != poolID {
			continue
		}

		pod.mu.Lock()
		for i := range pod.Slots {
			slot := &pod.Slots[i]
			if slot.TaskID == nil {
				continue
			}

			taskStatus, exists := taskStatusMap[*slot.TaskID]
			if !exists {
				// 任务已完成或不存在，释放槽位
				log.Printf("[PodManager] Task %d no longer active, releasing slot %d on pod %s",
					*slot.TaskID, i, pod.PodName)
				*slot = PodSlot{
					SlotID:    i,
					TaskID:    nil,
					TaskType:  "",
					Status:    "idle",
					UpdatedAt: time.Now(),
				}
				continue
			}

			// 更新槽位状态
			if taskStatus == models.TaskStatusApplyPending && slot.Status != "reserved" {
				slot.Status = "reserved"
				slot.UpdatedAt = time.Now()
				log.Printf("[PodManager] Updated slot %d on pod %s to reserved (task %d is apply_pending)",
					i, pod.PodName, *slot.TaskID)
			}
		}
		pod.mu.Unlock()
	}

	return nil
}

// cleanupExpiredReservations 清理过期的预留槽位
func (m *K8sPodManager) cleanupExpiredReservations(poolID string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	expirationTime := 24 * time.Hour

	for _, pod := range m.pods {
		if pod.PoolID != poolID {
			continue
		}

		pod.mu.Lock()
		for i := range pod.Slots {
			slot := &pod.Slots[i]
			if slot.Status == "reserved" && time.Since(slot.UpdatedAt) > expirationTime {
				log.Printf("[PodManager] Releasing expired reservation on pod %s slot %d (task %d, age: %v)",
					pod.PodName, i, *slot.TaskID, time.Since(slot.UpdatedAt))

				*slot = PodSlot{
					SlotID:    i,
					TaskID:    nil,
					TaskType:  "",
					Status:    "idle",
					UpdatedAt: now,
				}
			}
		}
		pod.mu.Unlock()
	}
}

// ============================================================================
// Agent注册和心跳
// ============================================================================

// RegisterAgent 注册Agent到Pod
func (m *K8sPodManager) RegisterAgent(podName string, agentID string) error {
	m.mu.RLock()
	pod, exists := m.pods[podName]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("pod %s not found", podName)
	}

	pod.mu.Lock()
	pod.AgentID = agentID
	pod.LastHeartbeat = time.Now()
	pod.mu.Unlock()

	log.Printf("[PodManager] Registered agent %s to pod %s", agentID, podName)
	return nil
}

// UpdateHeartbeat 更新Agent心跳
func (m *K8sPodManager) UpdateHeartbeat(podName string) error {
	m.mu.RLock()
	pod, exists := m.pods[podName]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("pod %s not found", podName)
	}

	pod.mu.Lock()
	pod.LastHeartbeat = time.Now()
	pod.mu.Unlock()

	return nil
}

// ============================================================================
// Pod Spec构建
// ============================================================================

// buildPodSpec 构建Pod规格
func (m *K8sPodManager) buildPodSpec(podName, namespace, poolID string, config *models.K8sJobTemplateConfig, secretName string) *corev1.Pod {
	// Image pull policy
	imagePullPolicy := corev1.PullIfNotPresent
	if config.ImagePullPolicy != "" {
		imagePullPolicy = corev1.PullPolicy(config.ImagePullPolicy)
	}

	// Build environment variables
	envVars := []corev1.EnvVar{
		{Name: "POOL_ID", Value: poolID},
		{Name: "POOL_TYPE", Value: "k8s"},
		{Name: "IAC_AGENT_NAME", Value: podName}, // 使用Pod名称作为Agent名称
		// IAC_AGENT_TOKEN from secret
		{
			Name: "IAC_AGENT_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
					Key: "token",
				},
			},
		},
	}

	// Auto-inject platform configuration environment variables
	// These are read from database system_configs table
	if m.platformConfigService != nil {
		platformConfig, err := m.platformConfigService.GetConfig()
		if err == nil && platformConfig != nil {
			// IAC_API_ENDPOINT - Platform host address only (without protocol or port)
			// Agent code will add protocol and port using SERVER_PORT
			// See backend/cmd/agent/main.go: fullAPIURL := fmt.Sprintf("%s://%s:%s", protocol, apiEndpoint, serverPort)
			envVars = append(envVars, corev1.EnvVar{
				Name:  "IAC_API_ENDPOINT",
				Value: platformConfig.Host,
			})

			// CC_SERVER_PORT - Agent control channel port
			envVars = append(envVars, corev1.EnvVar{
				Name:  "CC_SERVER_PORT",
				Value: platformConfig.CCPort,
			})

			// SERVER_PORT - Platform API port (used by Agent to construct full URL)
			envVars = append(envVars, corev1.EnvVar{
				Name:  "SERVER_PORT",
				Value: platformConfig.APIPort,
			})

			// PLATFORM_HOST - Platform host address (same as IAC_API_ENDPOINT for reference)
			envVars = append(envVars, corev1.EnvVar{
				Name:  "PLATFORM_HOST",
				Value: platformConfig.Host,
			})

			log.Printf("[PodManager] Auto-injected platform config: IAC_API_ENDPOINT=%s, SERVER_PORT=%s, CC_SERVER_PORT=%s",
				platformConfig.Host, platformConfig.APIPort, platformConfig.CCPort)
		} else if err != nil {
			log.Printf("[PodManager] Warning: failed to get platform config, skipping auto-injection: %v", err)
		}
	}

	// Add custom env vars from config (these can override auto-injected values)
	for key, value := range config.Env {
		envVars = append(envVars, corev1.EnvVar{Name: key, Value: value})
	}

	// Build resource requirements
	resources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}

	if config.Resources != nil {
		if config.Resources.Limits != nil {
			if cpu, ok := config.Resources.Limits["cpu"]; ok {
				if quantity, err := resource.ParseQuantity(cpu); err == nil {
					resources.Limits[corev1.ResourceCPU] = quantity
				}
			}
			if memory, ok := config.Resources.Limits["memory"]; ok {
				if quantity, err := resource.ParseQuantity(memory); err == nil {
					resources.Limits[corev1.ResourceMemory] = quantity
				}
			}
		}
		if config.Resources.Requests != nil {
			if cpu, ok := config.Resources.Requests["cpu"]; ok {
				if quantity, err := resource.ParseQuantity(cpu); err == nil {
					resources.Requests[corev1.ResourceCPU] = quantity
				}
			}
			if memory, ok := config.Resources.Requests["memory"]; ok {
				if quantity, err := resource.ParseQuantity(memory); err == nil {
					resources.Requests[corev1.ResourceMemory] = quantity
				}
			}
		}
	}

	// Build container
	container := corev1.Container{
		Name:            "agent",
		Image:           config.Image,
		ImagePullPolicy: imagePullPolicy,
		Env:             envVars,
		Resources:       resources,
	}

	if len(config.Command) > 0 {
		container.Command = config.Command
	}
	if len(config.Args) > 0 {
		container.Args = config.Args
	}

	// Build Pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":       "iac-platform",
				"component": "agent",
				"pool-id":   poolID,
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever, // Pod不自动重启
			Containers:    []corev1.Container{container},
		},
	}

	return pod
}

// ============================================================================
// 辅助方法
// ============================================================================

// GetPodCount 获取指定pool的Pod数量
func (m *K8sPodManager) GetPodCount(poolID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, pod := range m.pods {
		if pod.PoolID == poolID {
			count++
		}
	}

	return count
}

// FindPodByAgentID 根据AgentID查找Pod
func (m *K8sPodManager) FindPodByAgentID(agentID string) (*ManagedPod, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, pod := range m.pods {
		if pod.AgentID == agentID {
			return pod, nil
		}
	}

	return nil, fmt.Errorf("pod not found for agent %s", agentID)
}

// FindPodByTaskID 根据TaskID查找Pod和槽位
func (m *K8sPodManager) FindPodByTaskID(taskID uint) (*ManagedPod, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, pod := range m.pods {
		pod.mu.RLock()
		for i, slot := range pod.Slots {
			if slot.TaskID != nil && *slot.TaskID == taskID {
				pod.mu.RUnlock()
				return pod, i, nil
			}
		}
		pod.mu.RUnlock()
	}

	return nil, -1, fmt.Errorf("task %d not found in any pod", taskID)
}
