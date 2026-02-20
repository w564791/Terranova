# K8s Agent Pool Deployment Implementation Summary

## Overview
Successfully migrated K8s agent pools from Job-based to Deployment-based architecture with auto-scaling capabilities.

## Implementation Date
2025-01-03

## Changes Made

### 1. New Service: K8s Deployment Service
**File**: `backend/services/k8s_deployment_service.go`

#### Key Features:
- **Deployment Management**: Creates and manages K8s Deployments for agent pools
- **Auto-Scaling**: Automatically scales replicas based on pending tasks
- **Freeze Schedule Support**: Respects configured freeze time windows
- **Environment Variable Injection**: Properly injects required variables

#### Core Functions:
- `NewK8sDeploymentService()`: Initializes the service with K8s client
- `EnsureDeploymentForPool()`: Creates deployment if it doesn't exist
- `ScaleDeployment()`: Scales deployment to specified replica count
- `GetDeploymentReplicas()`: Returns current and desired replica counts
- `AutoScaleDeployment()`: Performs auto-scaling logic for a pool
- `StartAutoScaler()`: Starts background goroutine for auto-scaling
- `CountPendingTasksForPool()`: Counts pending tasks for scaling decisions

### 2. Environment Variables Injected

The following environment variables are automatically injected into agent pods:

1. **POOL_ID**: The pool identifier
2. **POOL_NAME**: The pool name
3. **POOL_TYPE**: Set to "k8s"
4. **API_ENDPOINT**: Platform API endpoint for agents to connect
5. **HOST_IP**: Platform server IP (from HOST_IP env var)
6. **IAC_AGENT_NAME**: Pod hostname (auto-injected via K8s fieldRef)
7. **CC_SERVER_PORT**: User-configured (from pool config)
8. **SERVER_PORT**: User-configured (from pool config)
9. **Custom variables**: Any additional env vars from pool configuration

### 3. Auto-Scaling Logic

#### Scaling Rules:
- **Scale Up**: When pending tasks exist, increase replicas by 1 (up to max_replicas)
- **Scale Down**: When no pending tasks, scale down to min_replicas (can be 0)
- **Constraints**:
  - Respects `min_replicas` configuration
  - Respects `max_replicas` configuration
  - Respects freeze schedule windows (no scaling during freeze periods)

#### Scaling Frequency:
- Auto-scaler runs every **30 seconds**
- Checks all active K8s pools in each cycle

#### Pending Task Detection:
Counts tasks that are:
- In `pending` status
- Associated with workspaces using the pool
- Using `k8s` execution mode

### 4. Integration with Main Application
**File**: `backend/main.go`

#### Startup Sequence:
1. Initialize K8s Deployment Service
2. Create deployments for all active K8s pools (initial replicas = 0)
3. Start auto-scaler goroutine
4. Auto-scaler will scale up as needed when tasks arrive

#### Graceful Shutdown:
- Auto-scaler stops automatically via context cancellation
- Deployments remain running in K8s cluster

### 5. Deployment Specification

#### Deployment Naming:
- Format: `iac-agent-{pool-id}`
- Namespace: `terraform` (default)

#### Pod Specification:
- **Restart Policy**: Always (for long-running agents)
- **Image Pull Policy**: Configurable (default: IfNotPresent)
- **Resource Limits**: Configurable CPU and memory limits
- **Labels**: 
  - `app: iac-platform`
  - `component: agent`
  - `pool-id: {pool-id}`
  - `pool-name: {pool-name}`

### 6. Differences from Job-Based Approach

| Aspect | Job-Based (Old) | Deployment-Based (New) |
|--------|----------------|------------------------|
| **Lifecycle** | One-time execution | Long-running |
| **Creation** | Per task | Per pool |
| **Scaling** | Manual (create new job) | Automatic |
| **Resource Usage** | High (new pod per task) | Efficient (reuse pods) |
| **Startup Time** | Slow (pod creation) | Fast (pods already running) |
| **Cost** | Higher | Lower |

### 7. Configuration Requirements

#### Required Environment Variables (Platform):
- `HOST_IP`: Platform server IP for agents to connect back
- `API_ENDPOINT`: Platform API endpoint (optional, defaults to localhost:8080)
- `KUBECONFIG`: Path to kubeconfig file (if not running in-cluster)

#### Pool Configuration (K8s Config):
```json
{
  "image": "your-agent-image:tag",
  "namespace": "terraform",
  "service_account": "terraform-agent",
  "min_replicas": 0,
  "max_replicas": 10,
  "cpu_limit": "500m",
  "memory_limit": "512Mi",
  "env": {
    "CC_SERVER_PORT": "8090",
    "SERVER_PORT": "8080",
    "CUSTOM_VAR": "value"
  },
  "freeze_schedules": [
    {
      "from_time": "23:00",
      "to_time": "02:00",
      "weekdays": [1, 2, 3, 4, 5]
    }
  ]
}
```

## Testing Checklist

- [ ] Verify deployment creation on service startup
- [ ] Test auto-scaling up when tasks are pending
- [ ] Test auto-scaling down when no tasks
- [ ] Verify freeze schedule prevents scaling
- [ ] Confirm environment variables are injected correctly
- [ ] Test with min_replicas = 0 (scale to zero)
- [ ] Test with min_replicas > 0 (always-on agents)
- [ ] Verify max_replicas constraint is respected
- [ ] Test graceful shutdown
- [ ] Monitor resource usage vs Job-based approach

## Monitoring

### Logs to Watch:
```
[K8sDeployment] Service initialized
[K8sDeployment] Found X active K8s pools
[K8sDeployment] Deployment ensured for pool {pool-id}
[K8sDeployment] Auto-scaler started (30 second interval)
[K8sDeployment] Auto-scaled pool {pool-id} from X to Y replicas (pending tasks: Z)
[K8sDeployment] Pool {pool-id} is in freeze window: {reason}, skipping auto-scale
```

### K8s Commands:
```bash
# List deployments
kubectl get deployments -n terraform

# Check deployment status
kubectl get deployment iac-agent-{pool-id} -n terraform

# View pods
kubectl get pods -n terraform -l pool-id={pool-id}

# Check pod logs
kubectl logs -n terraform -l pool-id={pool-id}

# Scale manually (for testing)
kubectl scale deployment iac-agent-{pool-id} --replicas=3 -n terraform
```

## Benefits

1. **Cost Efficiency**: Reuse pods instead of creating new ones per task
2. **Faster Task Execution**: Agents are already running and ready
3. **Better Resource Utilization**: Scale to zero when idle
4. **Automatic Scaling**: No manual intervention needed
5. **Freeze Window Support**: Prevent scaling during maintenance windows
6. **Flexible Configuration**: Min/max replicas per pool

## Future Enhancements

1. **Metrics-Based Scaling**: Scale based on CPU/memory usage
2. **Predictive Scaling**: Scale up before tasks arrive based on patterns
3. **Multi-Zone Support**: Distribute agents across availability zones
4. **Health Checks**: Automatic pod restart on failure
5. **Custom Scaling Policies**: Per-pool scaling strategies
6. **Integration with HPA**: Use Horizontal Pod Autoscaler

## Rollback Plan

If issues arise, you can:
1. Disable auto-scaler by setting environment variable
2. Manually scale deployments to desired count
3. Revert to Job-based approach by using k8s_job_service.go
4. Delete deployments: `kubectl delete deployment iac-agent-{pool-id} -n terraform`

## Related Files

- `backend/services/k8s_deployment_service.go` - Main implementation
- `backend/services/k8s_job_service.go` - Old Job-based implementation (kept for reference)
- `backend/main.go` - Service initialization and startup
- `backend/internal/models/pool_token.go` - K8s configuration models
- `frontend/src/pages/admin/AgentPoolDetail.tsx` - Frontend configuration UI

## Notes

- The old Job-based service (`k8s_job_service.go`) is still available but not used
- Deployments are created with 0 replicas initially and scaled up by auto-scaler
- Freeze schedules are checked before every scaling operation
- The system uses the same pool token authentication as before
- Agent pods use the pod name as IAC_AGENT_NAME for identification

## Contact

For questions or issues, please refer to the main project documentation or contact the development team.
