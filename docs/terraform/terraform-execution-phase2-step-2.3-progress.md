# Phase 2 Step 2.3 Progress - Pod Management Refactoring

## Date: 2025-11-08 14:25

## Current Status: IN PROGRESS

### Completed Work 

1. **Added podManager field to K8sDeploymentService** 
   - Updated struct definition with comment explaining the architecture change
   - Added `podManager *K8sPodManager` field

2. **Updated NewK8sDeploymentService constructor** 
   - Initialize PodManager with `NewK8sPodManager(db, clientset)`
   - PodManager is now available for use in all service methods

### Next Steps (Remaining Work)

#### 3. Refactor Core Methods (High Priority)

The following methods need to be refactored to use Pod management instead of Deployment management:

**A. EnsureDeploymentForPool → EnsurePodsForPool**
- Current: Creates/updates K8s Deployment
- Target: Sync Pods from K8s and ensure minimum pods exist
- Implementation:
  ```go
  func (s *K8sDeploymentService) EnsurePodsForPool(ctx context.Context, pool *models.AgentPool) error {
      // 1. Ensure secret exists
      // 2. Sync pods from K8s: s.podManager.SyncPodsFromK8s(ctx, pool.PoolID)
      // 3. Check current pod count
      // 4. If needed, create initial pods (respecting min_replicas)
  }
  ```

**B. ScaleDeployment → ScalePods**
- Current: Updates Deployment replicas
- Target: Create/delete individual Pods
- Implementation:
  ```go
  func (s *K8sDeploymentService) ScalePods(ctx context.Context, poolID string, desiredCount int) error {
      // 1. Get current pod count: s.podManager.GetPodCount(poolID)
      // 2. If scale up: create new pods
      // 3. If scale down: delete ONLY idle pods (all slots idle)
      //    idlePods := s.podManager.FindIdlePods(poolID)
  }
  ```

**C. GetDeploymentReplicas → GetPodCount**
- Current: Returns Deployment replica counts
- Target: Returns Pod counts
- Implementation:
  ```go
  func (s *K8sDeploymentService) GetPodCount(ctx context.Context, poolID string) (current, desired int, err error) {
      // 1. Sync pods: s.podManager.SyncPodsFromK8s(ctx, poolID)
      // 2. Return: s.podManager.GetPodCount(poolID), desired (from config), nil
  }
  ```

**D. AutoScaleDeployment → AutoScalePods**
- Current: Scales Deployment based on task count
- Target: Scales Pods based on slot utilization
- Key Changes:
  - Use slot statistics instead of task count
  - Scale up: Create pods when slots are full
  - Scale down: Only delete pods with all slots idle
  - Respect reserved slots (apply_pending tasks)

#### 4. Update Auto-Scaler Logic

**runAutoScalerCycle**
- Update to call `AutoScalePods` instead of `AutoScaleDeployment`
- Add Pod reconciliation: `s.podManager.ReconcilePods(ctx, pool.PoolID)`

#### 5. Remove Deprecated Code

Once Pod management is working:
- Remove `buildDeployment` method
- Remove `UpdateDeploymentConfig` method
- Keep `DeleteDeployment` for cleanup of old deployments

#### 6. Update Dependent Services

**TaskQueueManager** (`backend/services/task_queue_manager.go`):
- Update `pushTaskToAgent` to use slot allocation
- After task assignment: `podManager.AssignTaskToSlot(podName, slotID, taskID, taskType)`
- After task completion: `podManager.ReleaseSlot(podName, slotID)`
- After plan completion (plan_and_apply): `podManager.ReserveSlot(podName, 0, taskID)`

#### 7. Agent-Side Changes

Create `backend/agent/worker/slot_manager.go`:
- Report slot status to platform
- Handle slot allocation requests
- Manage concurrent task execution

#### 8. Update main.go

- Start Pod reconciliation goroutine
- Initialize services with Pod management support

### Testing Strategy

1. **Compilation Test** (Next Step)
   - Verify current changes compile successfully
   - Fix any import or type errors

2. **Unit Tests**
   - Test slot allocation logic
   - Test scale-down protection (only delete idle pods)
   - Test reserved slot protection (apply_pending)

3. **Integration Tests**
   - Test full task execution flow with slots
   - Test concurrent plan tasks (3 per pod)
   - Test plan_and_apply exclusive execution

### Key Design Principles

1. **Slot-Based Capacity**
   - Each Pod has 3 slots (0, 1, 2)
   - Slot 0: Can run any task (plan or plan_and_apply)
   - Slots 1-2: Can only run plan tasks
   - Slot states: idle, running, reserved

2. **Safe Scale-Down**
   - Only delete pods where ALL slots are idle
   - Never delete pods with running or reserved slots
   - Prevents task interruption during scale-down

3. **Apply-Pending Protection**
   - When plan completes in plan_and_apply task, reserve Slot 0
   - Reserved slot prevents pod deletion
   - Ensures agent stays alive for apply execution

### Risk Mitigation

1. **Backward Compatibility**
   - Keep existing Deployment methods temporarily
   - Gradual migration path
   - Can rollback if issues arise

2. **Monitoring**
   - Log all slot state changes
   - Track pod creation/deletion events
   - Monitor task assignment failures

3. **Graceful Degradation**
   - If slot allocation fails, fall back to creating new pod
   - If pod manager unavailable, use existing logic

### Timeline

- **Step 2.3**: 2 days (refactor core methods)
- **Step 2.4**: 2 days (update auto-scaler)
- **Step 2.5**: 1 day (TaskQueueManager updates)
- **Step 2.6**: 1 day (Agent slot manager)
- **Step 2.7**: 0.5 days (main.go updates)
- **Step 2.8**: 1.5 days (testing)

**Total Remaining**: ~8 days

### Current Blockers

None - ready to proceed with method refactoring.

### Next Immediate Action

1. Test compilation of current changes
2. Begin refactoring `EnsureDeploymentForPool` → `EnsurePodsForPool`
3. Update method signatures and implementations one by one
