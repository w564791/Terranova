# Agent Mode Complete Refactoring Plan

## Executive Summary

This document outlines the complete refactoring plan to fix Agent Mode in the IAC Platform. The current implementation has 20+ locations in `terraform_executor.go` that directly access the database (`s.db`), which causes panics in Agent Mode where `s.db == nil`.

**Status**: 🔴 Agent Mode is severely broken
-  Plan tasks work (basic)
- ❌ Apply tasks will panic
- ❌ Resource changes parsing skipped
- ❌ 20+ database access points need refactoring

## Problem Analysis

### Root Cause
The `TerraformExecutor` was originally designed for Local Mode only, with direct database access throughout. The DataAccessor interface was added later but not fully implemented across all functions.

### Impact
- **ExecuteApply**: Completely unusable (will panic on multiple `s.db` calls)
- **Resource Changes Parsing**: Skipped in Agent mode (empty API response)
- **State Management**: Partially broken
- **Workspace Locking**: Not working
- **Task Logs**: Cannot be retrieved

## Implementation Strategy

### Phase 1: Extend DataAccessor Interface 
Add missing methods to support all database operations needed by TerraformExecutor.

**New Methods Needed**:
```go
// Plan-specific
GetPlanTask(taskID uint) (*models.WorkspaceTask, error)
ParsePlanChanges(taskID uint, planOutput string) error

// Workspace management  
LockWorkspace(workspaceID, userID, reason string) error
UnlockWorkspace(workspaceID string) error

// Task logs
GetTaskLogs(taskID uint) ([]models.TaskLog, error)

// State management (enhanced)
GetMaxStateVersion(workspaceID string) (int, error)
SaveStateVersionInTransaction(version *models.WorkspaceStateVersion) error

// Resource snapshots
GetWorkspaceResourcesWithVersions(workspaceID string) ([]models.WorkspaceResource, error)
```

### Phase 2: Implement in LocalDataAccessor 
Wrap existing database operations with new interface methods.

**Implementation Notes**:
- Simple wrappers around existing `s.db` calls
- Maintain transaction support
- No behavior changes

### Phase 3: Implement in RemoteDataAccessor 
Add API client calls for each new method.

**Implementation Notes**:
- Call corresponding API endpoints
- Handle errors appropriately
- Support retries for transient failures

### Phase 4: Extend AgentAPIClient 
Add new API client methods.

**New Client Methods**:
```go
GetPlanTask(taskID uint) (*models.WorkspaceTask, error)
LockWorkspace(workspaceID, userID, reason string) error
ParsePlanChanges(taskID uint, planOutput string) error
GetTaskLogs(taskID uint) ([]models.TaskLog, error)
GetMaxStateVersion(workspaceID string) (int, error)
// ... etc
```

### Phase 5: Add API Endpoints 
Add handlers and routes for new operations.

**New Endpoints**:
```
GET    /api/v1/agents/tasks/{id}/plan-task
POST   /api/v1/agents/workspaces/{id}/lock
POST   /api/v1/agents/workspaces/{id}/unlock
POST   /api/v1/agents/tasks/{id}/parse-plan-changes
GET    /api/v1/agents/tasks/{id}/logs
GET    /api/v1/agents/workspaces/{id}/state/max-version
POST   /api/v1/agents/workspaces/{id}/state/save-transaction
GET    /api/v1/agents/workspaces/{id}/resources-with-versions
```

### Phase 6: Refactor TerraformExecutor 
Replace all `s.db` usage with `s.dataAccessor` calls.

**Functions to Refactor** (20 locations):

1. **ExecutePlan** (3 locations)
   - Line ~370: Get TF_LOG variable → `GetWorkspaceVariables`
   - Line ~470: Save snapshot_id → `UpdateTask`
   - Line ~476: Parse plan changes → `ParsePlanChanges`

2. **ExecuteApply** (10+ locations) - CRITICAL
   - Line ~550: Get workspace → Already using dataAccessor 
   - Line ~650: Get plan task → `GetPlanTask`
   - Line ~730: Save task (multiple) → `UpdateTask`
   - Line ~780: Apply parser needs refactoring
   - Line ~850: Apply parser service needs refactoring

3. **SaveStateToDatabase** (2 locations)
   - Line ~730: Get max version → `GetMaxStateVersion`
   - Line ~740: Transaction → `SaveStateVersionInTransaction`

4. **lockWorkspace** (1 location)
   - Line ~760: Update workspace → `LockWorkspace`

5. **GetTaskLogs** (1 location)
   - Line ~775: Query logs → `GetTaskLogs`

6. **CreateResourceSnapshot** (2 locations)
   - Line ~800: Get resources → `GetWorkspaceResourcesWithVersions`
   - Line ~810: Get version → Already handled in above

7. **maskSensitiveVariables** (1 location)
   - Line ~920: Get sensitive vars → `GetWorkspaceVariables`

8. **SaveNewStateVersionWithLogging** (1 location)
   - Line ~1050: Get max version → `GetMaxStateVersion`

### Phase 7: Handle Special Cases 

#### Apply Output Parser
**Problem**: `NewApplyOutputParser` requires `*gorm.DB`

**Solution**: Refactor to use DataAccessor
```go
// Old
parser := NewApplyOutputParser(task.ID, s.db, logger)

// New
parser := NewApplyOutputParser(task.ID, s.dataAccessor, logger)
```

#### Apply Parser Service
**Problem**: `NewApplyParserService` requires `*gorm.DB`

**Solution**: Refactor to use DataAccessor
```go
// Old
parserService := NewApplyParserService(s.db, logger)

// New
parserService := NewApplyParserService(s.dataAccessor, logger)
```

#### Transaction Handling
**Problem**: State saving uses `s.db.Transaction()`

**Solution**: Use DataAccessor transaction support
```go
// Old
return s.db.Transaction(func(tx *gorm.DB) error {
    // ...
})

// New
txAccessor, err := s.dataAccessor.BeginTransaction()
if err != nil {
    return err
}
defer txAccessor.Rollback()

// ... operations ...

return txAccessor.Commit()
```

## Detailed Implementation Steps

### Step 1: Extend DataAccessor Interface

File: `backend/services/data_accessor.go`

```go
type DataAccessor interface {
    // ... existing methods ...
    
    // Plan-specific operations
    GetPlanTask(taskID uint) (*models.WorkspaceTask, error)
    ParsePlanChanges(taskID uint, planOutput string) error
    
    // Workspace locking
    LockWorkspace(workspaceID, userID, reason string) error
    UnlockWorkspace(workspaceID string) error
    
    // Task logs
    GetTaskLogs(taskID uint) ([]models.TaskLog, error)
    
    // State management (enhanced)
    GetMaxStateVersion(workspaceID string) (int, error)
    SaveStateVersionInTransaction(version *models.WorkspaceStateVersion) error
    
    // Resource operations (enhanced)
    GetWorkspaceResourcesWithVersions(workspaceID string) ([]models.WorkspaceResource, error)
}
```

### Step 2: Implement in LocalDataAccessor

File: `backend/services/local_data_accessor.go`

Add implementations for each new method, wrapping existing database operations.

### Step 3: Implement in RemoteDataAccessor

File: `backend/services/remote_data_accessor.go`

Add API client calls for each new method.

### Step 4: Extend AgentAPIClient

File: `backend/services/agent_api_client.go`

Add HTTP client methods for new endpoints.

### Step 5: Add API Endpoints

Files:
- `backend/internal/handlers/agent_handler.go` - Add handlers
- `backend/internal/router/router_agent.go` - Add routes

### Step 6: Refactor TerraformExecutor

File: `backend/services/terraform_executor.go`

Systematically replace each `s.db` usage with appropriate `s.dataAccessor` call.

### Step 7: Refactor Parser Services

Files:
- `backend/services/apply_parser_service.go`
- `backend/services/plan_parser_service.go`

Update to use DataAccessor instead of *gorm.DB.

## Testing Strategy

### Unit Tests
- Test each new DataAccessor method in isolation
- Mock API responses for RemoteDataAccessor
- Verify LocalDataAccessor wraps DB correctly

### Integration Tests
1. **Local Mode Tests**
   - Verify no regressions
   - All existing functionality works
   - Plan and Apply tasks complete successfully

2. **Agent Mode Tests**
   - Agent can execute Plan tasks
   - Agent can execute Apply tasks
   - Resource changes are parsed correctly
   - State is saved properly
   - Workspace locking works

### Test Scenarios
```
Scenario 1: Plan Task in Agent Mode
- Create workspace
- Queue plan task
- Agent picks up task
- Plan executes successfully
- Resource changes are parsed
- API returns correct data

Scenario 2: Apply Task in Agent Mode
- Create workspace with plan
- Queue apply task
- Agent picks up task
- Apply executes successfully
- State is saved
- Resources are updated

Scenario 3: Error Handling
- Network failures
- API errors
- Database errors
- Graceful degradation
```

## Risk Mitigation

### Risks
1. **Breaking Local Mode**: Changes might break existing functionality
2. **API Performance**: Too many API calls might slow down execution
3. **Transaction Handling**: Complex transaction logic might fail
4. **Data Consistency**: Race conditions in distributed system

### Mitigation Strategies
1. **Comprehensive Testing**: Test both modes thoroughly
2. **Gradual Rollout**: Deploy to staging first
3. **Feature Flags**: Allow rollback if issues found
4. **Monitoring**: Add metrics for Agent mode operations
5. **Rollback Plan**: Keep old code path available

## Success Criteria

### Must Have
-  Agent can execute Plan tasks without errors
-  Agent can execute Apply tasks without errors
-  Resource changes are parsed in Agent mode
-  State is saved correctly in Agent mode
-  No regressions in Local mode
-  All 20+ `s.db` usages replaced

### Nice to Have
- Performance metrics for Agent mode
- Detailed error logging
- Retry logic for transient failures
- Circuit breaker for API calls

## Timeline

### Week 1: Foundation (Days 1-2)
- [x] Day 1: Extend DataAccessor interface
- [x] Day 1: Implement in LocalDataAccessor
- [ ] Day 2: Implement in RemoteDataAccessor
- [ ] Day 2: Test Local mode (no regressions)

### Week 2: API Layer (Days 3-4)
- [ ] Day 3: Extend AgentAPIClient
- [ ] Day 3: Add API endpoints
- [ ] Day 4: Add API handlers
- [ ] Day 4: Test API endpoints

### Week 3: Refactoring (Days 5-7)
- [ ] Day 5: Refactor ExecutePlan
- [ ] Day 6: Refactor ExecuteApply (critical)
- [ ] Day 7: Refactor helper functions
- [ ] Day 7: Refactor parser services

### Week 4: Testing & Deployment (Days 8-10)
- [ ] Day 8: Integration testing
- [ ] Day 9: Performance testing
- [ ] Day 10: Documentation & deployment

## Rollout Plan

### Phase 1: Staging Deployment
1. Deploy to staging environment
2. Run automated tests
3. Manual testing of both modes
4. Performance benchmarking

### Phase 2: Canary Deployment
1. Deploy to 10% of production agents
2. Monitor for 24 hours
3. Check error rates and performance
4. Rollback if issues found

### Phase 3: Full Deployment
1. Deploy to all production agents
2. Monitor closely for 48 hours
3. Document any issues
4. Prepare hotfix if needed

## Monitoring & Alerts

### Metrics to Track
- Agent task success rate
- API call latency
- Database query performance
- Error rates by type
- Resource changes parsing success

### Alerts
- Agent task failure rate > 5%
- API latency > 1s (P95)
- Database errors > 1%
- Resource parsing failures > 1%

## Documentation Updates

### Developer Documentation
- Update architecture diagrams
- Document new API endpoints
- Add troubleshooting guide
- Update deployment guide

### Operations Documentation
- Update monitoring guide
- Add runbook for common issues
- Document rollback procedures
- Update capacity planning

## Conclusion

This refactoring is a significant undertaking that will take approximately 2-3 weeks of focused development. However, it's essential for making Agent Mode fully functional and production-ready.

The systematic approach outlined here minimizes risk while ensuring comprehensive coverage of all affected areas. By following this plan, we can deliver a robust, well-tested solution that works reliably in both Local and Agent modes.

---

**Document Version**: 1.0  
**Last Updated**: 2025-11-01  
**Status**: 🟡 In Progress
