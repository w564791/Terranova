# Task 604: Slot Detection Bug Analysis

## Problem Summary

Task 604 is stuck in pending state despite the pool reporting 2 idle slots available. The logs show:

```
2025/11/10 10:34:08 [TaskQueue] No free slot available for task 604: no free slot available in pool pool-z73eh8ihywlmgx0x
2025/11/10 10:34:11 [K8sPodService] Pool pool-z73eh8ihywlmgx0x status: pods=1, slots(total=3, used=1, reserved=0, idle=2)
```

## Root Cause Analysis

### Issue 1: Agent Connectivity Check Mismatch

In `task_queue_manager.go`, the `pushTaskToAgent` function checks for connected agents:

```go
// 3. Get actually connected agents from AgentCCHandler
connectedAgentIDs := m.agentCCHandler.GetConnectedAgents()
if len(connectedAgentIDs) == 0 {
    // Release slot and retry
    log.Printf("[TaskQueue] No connected agents found, task %d will retry", task.ID)
    m.scheduleRetry(task.WorkspaceID, 15*time.Second)
    return nil
}
```

However, in `k8s_pod_manager.go`, the `FindPodWithFreeSlot` function has a grace period for new pods:

```go
// Grace period: 5 minutes for new pods to register and start sending heartbeats
if timeSinceCreation < 5*time.Minute {
    // New pod, allow it even without recent heartbeat
    log.Printf("[PodManager] Pod %s is new (created %v ago), allowing without strict heartbeat check",
        pod.PodName, timeSinceCreation)
} else if timeSinceHeartbeat > 2*time.Minute {
    // Old pod without recent heartbeat, skip it
    log.Printf("[PodManager] Pod %s is offline (last heartbeat: %v ago), skipping",
        pod.PodName, timeSinceHeartbeat)
    continue
}
```

**The Problem**: 
1. `FindPodWithFreeSlot` finds a slot on a new Pod (within 5-minute grace period)
2. The slot is allocated successfully
3. But then `pushTaskToAgent` checks for connected agents via C&C handler
4. The agent hasn't connected yet (still starting up), so `GetConnectedAgents()` returns empty
5. The slot is released and the task retries

This creates a race condition where:
- The Pod exists and has idle slots
- But the Agent hasn't connected to the C&C channel yet
- So tasks can't be assigned even though slots are available

### Issue 2: Cold Start Detection Logic

In `k8s_deployment_service.go`, the `AutoScalePods` function has cold start logic:

```go
if totalSlots == 0 || currentPodCount == 0 {
    // Cold start scenario - simplified logic
    var pendingTaskCount int64
    // ... query pending tasks ...
    
    if pendingTaskCount > 0 {
        desiredPodCount = k8sConfig.MinReplicas
        if desiredPodCount < 1 {
            desiredPodCount = 1
        }
    }
}
```

**The Problem**:
- This only triggers when `totalSlots == 0` or `currentPodCount == 0`
- But in our case, we have 1 Pod with 3 slots (total=3, idle=2)
- So the cold start logic doesn't trigger
- The autoscaler doesn't create new Pods because slot utilization is low (33%)
- But the existing Pod's agent isn't connected yet, so tasks can't be assigned

## Current State

From the logs:
- Pool: `pool-z73eh8ihywlmgx0x`
- Pods: 1 (Pod name: `iac-agent-pool-z73eh8ihywlmgx0x-1762741970`)
- Pod age: ~1m26s (created at 10:32:50, current time 10:34:16)
- Slots: total=3, used=1 (task 603), reserved=0, idle=2
- Task 603: Running on workspace ws-mb7m9ii5ey
- Task 604: Pending on workspace ws-0yrm628p8h3f9mw0
- Agent: Connected (agent-pool-z73eh8ihywlmgx0x-1762741971140016000)

The Pod is within the 5-minute grace period, so `FindPodWithFreeSlot` allows it. But the agent might not be fully registered in the C&C handler yet, causing the assignment to fail.

## Solution Options

### Option 1: Align Grace Period Logic (Recommended)

Make the grace period check consistent between `FindPodWithFreeSlot` and `pushTaskToAgent`:

1. In `FindPodWithFreeSlot`: Check if the Pod's agent is actually connected before returning it
2. Or in `pushTaskToAgent`: Apply the same grace period logic when checking for connected agents

### Option 2: Improve Cold Start Detection

Enhance the cold start detection to handle the "agent not connected yet" scenario:

1. Check if any Pods exist but have no connected agents
2. If so, wait for agents to connect before creating new Pods
3. Only create new Pods if no Pods exist OR all Pods have connected agents but are at capacity

### Option 3: Separate Slot Allocation from Agent Assignment

1. Keep slot allocation as-is (reserves the slot)
2. But delay agent assignment until the agent is confirmed connected
3. Add a "slot_reserved_but_agent_not_ready" state

## Recommended Fix

**Option 1** is the simplest and most effective:

In `k8s_pod_manager.go`, modify `FindPodWithFreeSlot` to check agent connectivity:

```go
// Check Agent heartbeat with grace period for new pods
timeSinceCreation := time.Since(pod.CreatedAt)
timeSinceHeartbeat := time.Since(pod.LastHeartbeat)

// Grace period: 5 minutes for new pods to register
if timeSinceCreation < 5*time.Minute {
    // New pod - check if agent has registered
    if pod.AgentID == "" {
        log.Printf("[PodManager] Pod %s is new but agent not registered yet, skipping",
            pod.PodName)
        continue
    }
    log.Printf("[PodManager] Pod %s is new (created %v ago) with registered agent %s, allowing",
        pod.PodName, timeSinceCreation, pod.AgentID)
} else if timeSinceHeartbeat > 2*time.Minute {
    // Old pod without recent heartbeat, skip it
    log.Printf("[PodManager] Pod %s is offline (last heartbeat: %v ago), skipping",
        pod.PodName, timeSinceHeartbeat)
    continue
}
```

This ensures that:
1. New Pods are only considered if their agent has registered (AgentID is set)
2. The slot allocation and agent assignment are aligned
3. Tasks won't be assigned to Pods whose agents haven't connected yet
4. The autoscaler will create new Pods if needed (when no Pods have connected agents)

## Impact

- **Low Risk**: Only affects new Pod detection logic
- **High Benefit**: Fixes the race condition causing tasks to get stuck
- **No Breaking Changes**: Existing behavior for established Pods remains unchanged

## Testing Plan

1. Create a new workspace with pending tasks
2. Verify that tasks wait for agent to connect before being assigned
3. Verify that autoscaler creates new Pods if no connected agents are available
4. Verify that tasks are assigned once agent connects
5. Test with multiple concurrent tasks to ensure proper slot allocation
