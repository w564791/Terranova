# Phase 6: TerraformExecutor Refactoring Implementation Plan

## Overview

This document outlines the detailed plan for refactoring `backend/services/terraform_executor.go` to replace all direct database access (`s.db`) with DataAccessor interface calls (`s.dataAccessor`).

## Current Status

**Total `s.db` usages found**: 20 locations
**Priority**: CRITICAL - Agent Mode is completely broken without this refactoring

## Refactoring Strategy

### Approach
1. Work function by function, starting with highest priority
2. Replace each `s.db` call with appropriate `s.dataAccessor` method
3. Test after each major function refactoring
4. Ensure no regressions in Local Mode

### Priority Order

**P0 - Critical (Must Fix)**:
1. ExecuteApply - 10+ locations
2. SaveStateToDatabase - 2 locations  
3. lockWorkspace - 1 location

**P1 - Important (Should Fix)**:
4. ExecutePlan - 3 locations
5. CreateResourceSnapshot - 2 locations
6. maskSensitiveVariables - 1 location

**P2 - Nice to Have (Can Defer)**:
7. GetTaskLogs - 1 location
8. Other helper functions

## Detailed Refactoring Plan

### 1. ExecutePlan Function

**Current Issues** (3 locations):

#### Location 1: Line ~370 - Get TF_LOG variable
```go
// OLD
var tfLogVar models.WorkspaceVariable
if err := s.db.Where("workspace_id = ? AND key = ? AND variable_type = ?",
    workspace.WorkspaceID, "TF_LOG", models.VariableTypeEnvironment).First(&tfLogVar).Error; err == nil {
    // use tfLogVar
}

// NEW
envVars, err := s.dataAccessor.GetWorkspaceVariables(workspace.WorkspaceID, models.VariableTypeEnvironment)
if err == nil {
    for _, v := range envVars {
        if v.Key == "TF_LOG" {
            // use v
            break
        }
    }
}
```

#### Location 2: Line ~470 - Save snapshot_id
```go
// OLD
task.SnapshotID = snapshotID
s.db.Save(task)

// NEW
task.SnapshotID = snapshotID
s.dataAccessor.UpdateTask(task)
```

#### Location 3: Line ~476 - Parse plan changes (currently skipped in Agent mode)
```go
// OLD
if s.db != nil {
    // Parse plan changes
    parserService := NewPlanParserService(s.db, logger)
    // ...
}

// NEW
// Always parse, regardless of mode
err := s.dataAccessor.ParsePlanChanges(task.ID, task.PlanOutput)
if err != nil {
    logger.Warn("Failed to parse plan changes: %v", err)
}
```

### 2. ExecuteApply Function (CRITICAL)

**Current Issues** (10+ locations):

#### Location 1: Line ~650 - Get plan task
```go
// OLD
var planTask models.WorkspaceTask
if err := s.db.First(&planTask, *task.PlanTaskID).Error; err != nil {
    // handle error
}

// NEW
planTask, err := s.dataAccessor.GetPlanTask(*task.PlanTaskID)
if err != nil {
    // handle error
}
```

#### Locations 2-5: Multiple s.db.Save(task) calls
```go
// OLD
s.db.Save(task)

// NEW
s.dataAccessor.UpdateTask(task)
```

#### Location 6: Apply output parser
```go
// OLD
parser := NewApplyOutputParser(task.ID, s.db, logger)

// NEW
// Need to refactor ApplyOutputParser to accept DataAccessor
parser := NewApplyOutputParser(task.ID, s.dataAccessor, logger)
```

#### Location 7: Apply parser service
```go
// OLD
parserService := NewApplyParserService(s.db, logger)

// NEW
// Need to refactor ApplyParserService to accept DataAccessor
parserService := NewApplyParserService(s.dataAccessor, logger)
```

### 3. SaveStateToDatabase Function

**Current Issues** (2 locations):

#### Location 1: Line ~730 - Get max version
```go
// OLD
var maxVersion int
s.db.Model(&models.WorkspaceStateVersion{}).
    Where("workspace_id = ?", workspace.WorkspaceID).
    Select("COALESCE(MAX(version), 0)").
    Scan(&maxVersion)

// NEW
maxVersion, err := s.dataAccessor.GetMaxStateVersion(workspace.WorkspaceID)
if err != nil {
    return fmt.Errorf("failed to get max version: %w", err)
}
```

#### Location 2: Line ~740 - Transaction
```go
// OLD
return s.db.Transaction(func(tx *gorm.DB) error {
    stateVersion := &models.WorkspaceStateVersion{...}
    if err := tx.Create(stateVersion).Error; err != nil {
        return err
    }
    return tx.Model(&workspace).Update("tf_state", stateContent).Error
})

// NEW
// For Agent mode, transactions are not supported
// We'll save state version and update workspace separately
stateVersion := &models.WorkspaceStateVersion{
    WorkspaceID: workspace.WorkspaceID,
    Version:     maxVersion + 1,
    Content:     stateContent,
    // ... other fields
}

if err := s.dataAccessor.SaveStateVersion(stateVersion); err != nil {
    return fmt.Errorf("failed to save state version: %w", err)
}

if err := s.dataAccessor.UpdateWorkspaceState(workspace.WorkspaceID, stateContent); err != nil {
    return fmt.Errorf("failed to update workspace state: %w", err)
}
```

### 4. lockWorkspace Function

**Current Issues** (1 location):

```go
// OLD
func (s *TerraformExecutor) lockWorkspace(workspaceID, userID, reason string) error {
    updates := map[string]interface{}{
        "locked":    true,
        "locked_by": userID,
        "locked_at": "NOW()",
    }
    if reason != "" {
        updates["lock_reason"] = reason
    }
    return s.db.Model(&models.Workspace{}).
        Where("workspace_id = ?", workspaceID).
        Updates(updates).Error
}

// NEW
func (s *TerraformExecutor) lockWorkspace(workspaceID, userID, reason string) error {
    return s.dataAccessor.LockWorkspace(workspaceID, userID, reason)
}
```

### 5. GetTaskLogs Function

**Current Issues** (1 location):

```go
// OLD
func (s *TerraformExecutor) GetTaskLogs(taskID uint) ([]models.TaskLog, error) {
    var logs []models.TaskLog
    err := s.db.Where("task_id = ?", taskID).
        Order("created_at ASC").
        Find(&logs).Error
    return logs, err
}

// NEW
func (s *TerraformExecutor) GetTaskLogs(taskID uint) ([]models.TaskLog, error) {
    return s.dataAccessor.GetTaskLogs(taskID)
}
```

### 6. CreateResourceSnapshot Function

**Current Issues** (2 locations):

```go
// OLD
var resources []models.WorkspaceResource
if err := s.db.Where("workspace_id = ? AND is_active = true", workspaceID).
    Find(&resources).Error; err != nil {
    return "", err
}

for i := range resources {
    if resources[i].CurrentVersionID != nil {
        var version models.ResourceCodeVersion
        if err := s.db.First(&version, *resources[i].CurrentVersionID).Error; err == nil {
            resources[i].CurrentVersion = &version
        }
    }
}

// NEW
resources, err := s.dataAccessor.GetWorkspaceResourcesWithVersions(workspaceID)
if err != nil {
    return "", fmt.Errorf("failed to get resources: %w", err)
}
```

### 7. maskSensitiveVariables Function

**Current Issues** (1 location):

```go
// OLD
var sensitiveVars []models.WorkspaceVariable
s.db.Where("workspace_id = ? AND variable_type = ? AND sensitive = true",
    workspaceID, models.VariableTypeTerraform).
    Find(&sensitiveVars)

// NEW
allVars, err := s.dataAccessor.GetWorkspaceVariables(workspaceID, models.VariableTypeTerraform)
if err != nil {
    logger.Warn("Failed to get variables for masking: %v", err)
    return output
}

var sensitiveVars []models.WorkspaceVariable
for _, v := range allVars {
    if v.Sensitive {
        sensitiveVars = append(sensitiveVars, v)
    }
}
```

### 8. SaveNewStateVersionWithLogging Function

**Current Issues** (1 location):

```go
// OLD
var maxVersion int
s.db.Model(&models.WorkspaceStateVersion{}).
    Where("workspace_id = ?", workspace.WorkspaceID).
    Select("COALESCE(MAX(version), 0)").
    Scan(&maxVersion)

// NEW
maxVersion, err := s.dataAccessor.GetMaxStateVersion(workspace.WorkspaceID)
if err != nil {
    return fmt.Errorf("failed to get max version: %w", err)
}
```

## Special Considerations

### 1. Apply Output Parser Refactoring

The `ApplyOutputParser` currently requires `*gorm.DB`. We need to:

1. Change constructor to accept `DataAccessor` instead of `*gorm.DB`
2. Update all database operations inside the parser to use DataAccessor
3. This is in `backend/services/apply_parser_service.go`

### 2. Apply Parser Service Refactoring

Similar to ApplyOutputParser, `ApplyParserService` needs refactoring:

1. Change constructor to accept `DataAccessor`
2. Update internal database operations
3. This is also in `backend/services/apply_parser_service.go`

### 3. Transaction Handling

Agent mode doesn't support transactions. For operations that currently use transactions:

1. **Option A**: Execute operations sequentially and accept potential inconsistency
2. **Option B**: Add transaction support to RemoteDataAccessor (complex)
3. **Recommendation**: Use Option A for now, document the limitation

### 4. Error Handling

Ensure proper error handling for all DataAccessor calls:
- Log errors appropriately
- Return meaningful error messages
- Don't panic on errors

## Testing Strategy

### Unit Tests
- Mock DataAccessor for testing TerraformExecutor
- Test both Local and Remote implementations
- Verify error handling

### Integration Tests
1. **Local Mode**:
   - Run existing test suite
   - Verify no regressions
   - All Plan/Apply operations work

2. **Agent Mode**:
   - Set up test agent
   - Execute Plan task
   - Execute Apply task
   - Verify resource changes parsing
   - Verify state saving

### Manual Testing Checklist
- [ ] Local Mode: Plan task
- [ ] Local Mode: Apply task
- [ ] Local Mode: Resource changes visible
- [ ] Agent Mode: Plan task
- [ ] Agent Mode: Apply task
- [ ] Agent Mode: Resource changes visible
- [ ] Agent Mode: State saved correctly
- [ ] Agent Mode: Workspace locking works

## Implementation Order

1.  Phase 1-5: Foundation (Complete)
2. 🔄 Phase 6: TerraformExecutor refactoring
   - Step 1: Refactor helper functions (lockWorkspace, GetTaskLogs, etc.)
   - Step 2: Refactor CreateResourceSnapshot
   - Step 3: Refactor maskSensitiveVariables
   - Step 4: Refactor SaveStateToDatabase
   - Step 5: Refactor ExecutePlan
   - Step 6: Refactor ExecuteApply (most complex)
3. ⏳ Phase 7: Parser services refactoring

## Rollback Plan

If issues are found:
1. Keep old code commented out for quick rollback
2. Use feature flag to toggle between old and new implementation
3. Monitor error rates closely
4. Have database backup ready

## Success Criteria

-  All 20+ `s.db` usages replaced
-  Code compiles without errors
-  Local Mode tests pass
-  Agent Mode can execute Plan tasks
-  Agent Mode can execute Apply tasks
-  Resource changes parsed in Agent Mode
-  No regressions in Local Mode

---

**Status**: 🔄 In Progress  
**Last Updated**: 2025-11-01  
**Next Action**: Start with helper functions refactoring
