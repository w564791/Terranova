# Workspace Variable Snapshot Implementation

## Overview
Implement variable snapshot functionality using version control references instead of storing complete variable data.

## Git Commit
**Previous**: 770d738 - feat: implement workspace variables version control with optimistic locking

## Implementation Status

###  Completed

1. **New Model Types** (`backend/internal/models/workspace.go`)
   - Added `VariableSnapshot` struct with `variable_id` and `version` fields
   - Added `VariableSnapshotArray` type with Value/Scan methods for JSONB storage

2. **Snapshot Creation** (`backend/controllers/workspace_task_controller.go`)
   - Modified `createTaskSnapshot()` to store only variable references
   - Changed from storing complete `WorkspaceVariable` objects to `VariableSnapshot` references
   - Query changed to `WHERE workspace_id = ? AND is_deleted = false`
   - Snapshot format: `[{"variable_id": "var-xxx", "version": 1}, ...]`

### 🔄 In Progress

3. **Snapshot Resolution** (Need to implement)
   - Add helper function to resolve variable snapshots to actual values
   - Update `GenerateConfigFilesFromSnapshot()` in `terraform_executor.go`
   - Update Agent API client to handle new snapshot format

## Technical Design

### Database Schema
No changes needed - `snapshot_variables` field already exists as JSONB type.

### Snapshot Format

**Old Format** (Complete data):
```json
[
  {
    "id": 123,
    "variable_id": "var-abc123",
    "workspace_id": "ws-xyz",
    "key": "AWS_REGION",
    "value": "us-east-1",  // Complete encrypted value
    "version": 1,
    "variable_type": "terraform",
    "sensitive": false
  }
]
```

**New Format** (References only):
```json
[
  {
    "variable_id": "var-abc123",
    "version": 1
  },
  {
    "variable_id": "var-def456",
    "version": 2
  }
]
```

### Resolution Logic

When Agent needs variables:
1. Read `snapshot_variables` from task
2. For each reference, query database:
   ```sql
   SELECT * FROM workspace_variables 
   WHERE variable_id = ? AND version = ?
   ```
3. Decrypt sensitive values
4. Apply to Terraform environment

## Implementation Steps

### Step 1: Add Variable Resolution Helper  (Partially)

Location: `backend/services/workspace_variable_service.go` or `terraform_executor.go`

```go
// ResolveVariableSnapshots resolves variable references to actual values
func (s *TerraformExecutor) ResolveVariableSnapshots(
    snapshotData interface{},
    workspaceID string,
) ([]models.WorkspaceVariable, error) {
    // Parse snapshot data
    var snapshots []models.VariableSnapshot
    
    // Handle both old format (WorkspaceVariableArray) and new format (VariableSnapshotArray)
    switch v := snapshotData.(type) {
    case models.WorkspaceVariableArray:
        // Old format - return as-is for backward compatibility
        return v, nil
    case []interface{}:
        // New format - need to resolve
        // Convert to VariableSnapshot array
        // Query database for each variable_id + version
    }
    
    // Query variables from database
    variables := make([]models.WorkspaceVariable, 0, len(snapshots))
    for _, snap := range snapshots {
        var variable models.WorkspaceVariable
        err := s.db.Where("variable_id = ? AND version = ?", 
            snap.VariableID, snap.Version).First(&variable).Error
        if err != nil {
            return nil, fmt.Errorf("variable %s version %d not found: %w", 
                snap.VariableID, snap.Version, err)
        }
        variables = append(variables, variable)
    }
    
    return variables, nil
}
```

### Step 2: Update GenerateConfigFilesFromSnapshot

Location: `backend/services/terraform_executor.go`

Current signature:
```go
func (s *TerraformExecutor) GenerateConfigFilesFromSnapshot(
    workspace *models.Workspace,
    resources []models.WorkspaceResource,
    variables models.WorkspaceVariableArray,  // OLD: Complete data
    workDir string,
    logger *TerraformLogger,
) error
```

New approach:
```go
func (s *TerraformExecutor) GenerateConfigFilesFromSnapshot(
    workspace *models.Workspace,
    resources []models.WorkspaceResource,
    variableSnapshots interface{},  // NEW: Can be old or new format
    workDir string,
    logger *TerraformLogger,
) error {
    // Resolve variable snapshots to actual values
    variables, err := s.ResolveVariableSnapshots(variableSnapshots, workspace.WorkspaceID)
    if err != nil {
        return fmt.Errorf("failed to resolve variable snapshots: %w", err)
    }
    
    // Rest of the function remains the same
    // ...
}
```

### Step 3: Update Agent API Client

Location: `backend/services/agent_api_client.go`

The Agent API client needs to handle the new snapshot format when sending task data to agents.

Current code converts `snapshot_variables` to `WorkspaceVariableArray`. Need to update to handle both formats.

### Step 4: Update Validation Logic

Location: `backend/services/terraform_executor.go`

The `ValidateResourceVersionSnapshot()` function validates resources. We may need a similar function for variables or update the existing one.

## Backward Compatibility

The implementation maintains backward compatibility:
1. Old snapshots (complete data) continue to work
2. New snapshots (references) are resolved on-the-fly
3. Type checking in `ResolveVariableSnapshots()` handles both formats

## Benefits

1. **Reduced Storage**: Snapshots only store 2 fields instead of 10+
2. **Security**: Encrypted values not duplicated in snapshots
3. **Consistency**: Always uses the actual variable record
4. **Version Control**: Leverages the new version control system
5. **Audit Trail**: Can track which version was used in each task

## Testing Plan

1. **Unit Tests**
   - Test `ResolveVariableSnapshots()` with both formats
   - Test snapshot creation with multiple variables
   - Test variable not found scenarios

2. **Integration Tests**
   - Create task with variables
   - Verify snapshot contains only references
   - Execute task and verify variables are resolved correctly
   - Update variable and verify old task still uses old version

3. **Migration Test**
   - Verify old tasks with complete variable data still work
   - Verify new tasks use reference format

## Files Modified

1.  `backend/internal/models/workspace.go` - New types
2.  `backend/controllers/workspace_task_controller.go` - Snapshot creation
3. ⏳ `backend/services/terraform_executor.go` - Resolution logic
4. ⏳ `backend/services/agent_api_client.go` - Agent communication
5. ⏳ `backend/services/workspace_variable_service.go` - Helper functions (optional)

## Next Steps

1. Implement `ResolveVariableSnapshots()` helper function
2. Update `GenerateConfigFilesFromSnapshot()` to use resolver
3. Update Agent API client to handle new format
4. Add comprehensive tests
5. Update documentation

## Related Documentation

- `docs/workspace-variable-version-control-complete.md` - Variable version control implementation
- `docs/plan-apply-race-condition-fix.md` - Resource snapshot implementation (reference)
- `scripts/add_plan_apply_snapshot_fields.sql` - Original snapshot SQL (reference)
