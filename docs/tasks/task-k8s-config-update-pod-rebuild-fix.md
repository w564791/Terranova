# K8s Config Update Pod Rebuild Fix

## Issue Description

When updating the K8s configuration for an agent pool (e.g., changing image, resources, environment variables), the idle pods were not being rebuilt with the new configuration. This meant that configuration changes would only take effect when:
1. Pods were manually deleted
2. Pods were scaled down and back up
3. New pods were created during scale-up

According to the design requirements, **any K8s configuration change should trigger a rebuild of all idle pods** (pods where all slots are idle).

## Root Cause

The `UpdateK8sConfig` handler in `backend/internal/handlers/agent_pool_handler.go` only updated the database with the new configuration but did not trigger any pod rebuild logic. The configuration was stored but not applied to existing pods.

## Solution

### 1. Updated `UpdateK8sConfig` Handler

Modified `backend/internal/handlers/agent_pool_handler.go` to call a new `RebuildIdlePods` method after updating the configuration:

```go
// Update K8s config in database
err := h.poolTokenService.UpdateK8sConfig(...)

// Get pool to pass to K8s deployment service
var pool models.AgentPool
if err := h.db.Where("pool_id = ?", poolID).First(&pool).Error; err != nil {
    // handle error
}

// Rebuild idle pods with new configuration
k8sDeploymentService, err := services.NewK8sDeploymentService(h.db)
if err := k8sDeploymentService.RebuildIdlePods(c.Request.Context(), &pool); err != nil {
    // handle error
}
```

### 2. Implemented `RebuildIdlePods` Method

Added a new method in `backend/services/k8s_deployment_service.go`:

```go
func (s *K8sDeploymentService) RebuildIdlePods(ctx context.Context, pool *models.AgentPool) error
```

This method:
1. Syncs pods from K8s to ensure latest state
2. Reconciles pods to sync task status to slots
3. Finds all idle pods (where ALL slots are idle)
4. Deletes idle pods
5. Recreates them with the new configuration

## Key Features

### Safe Pod Deletion

- Only deletes pods where **ALL slots are idle**
- Pods with any busy or reserved slots are preserved
- This ensures no running tasks are interrupted

### Automatic Recreation

- After deleting idle pods, immediately recreates them with the new configuration
- Maintains the same number of pods (doesn't change scale)
- Uses the latest K8s config from the database

### Error Handling

- Logs warnings if pod deletion fails but continues with other pods
- Skips recreation if no pods were successfully deleted
- Returns detailed error messages to the API caller

## API Response

The API now returns a more informative message:

```json
{
  "message": "K8s configuration updated successfully and idle pods are being rebuilt"
}
```

If pod rebuild fails, it returns:

```json
{
  "error": "K8s config updated but failed to rebuild idle pods: <error details>"
}
```

## Testing

To test the fix:

1. Create a K8s agent pool with some idle pods
2. Update the K8s configuration (e.g., change image or environment variables)
3. Verify that idle pods are deleted and recreated with new configuration
4. Verify that busy pods (with running tasks) are NOT deleted

## Logs

The fix adds detailed logging:

```
[K8sPodService] Rebuilding idle pods for pool <pool-id> after config update
[K8sPodService] Found <n> idle pods to rebuild for pool <pool-id>
[K8sPodService] Deleted idle pod <pod-name> for rebuild (1/n)
[K8sPodService] Created replacement pod with new config (1/n)
[K8sPodService] Successfully rebuilt <n> idle pods for pool <pool-id> with updated configuration
```

## Related Code

- Handler: `backend/internal/handlers/agent_pool_handler.go` - `UpdateK8sConfig()`
- Service: `backend/services/k8s_deployment_service.go` - `RebuildIdlePods()`
- Pod Manager: `backend/services/k8s_pod_manager.go` - `FindIdlePods()`, `DeletePod()`, `CreatePod()`

## Design Compliance

This fix ensures compliance with the design requirement:

> "只要pod是空闲的 更改k8s的任何配置都要重建pod"
> (As long as the pod is idle, any K8s configuration change should rebuild the pod)

The implementation correctly identifies idle pods (all slots idle) and rebuilds them with the new configuration while preserving busy pods.
