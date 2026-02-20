# Task: Variable Snapshot Empty Key Fix

## Problem Analysis

When the apply stage runs after pod deletion, it retrieves variable data from the task snapshot. However, the generated `variables.tf.json` and `variables.tfvars` files contain empty variable keys, resulting in:

```json
{
  "variable": {
    "": {
      "type": "string"
    }
  }
}
```

```
 = ""
```

## Root Cause

The issue is in the `ResolveVariableSnapshots` function in `terraform_executor.go`. When it detects the new snapshot format (which only contains `variable_id` and `version` references), it queries the database to get the actual variable data:

```go
err = s.db.Where("variable_id = ? AND version = ?", varID, version).
    First(&variable).Error
```

However, this query is not selecting all fields properly, resulting in empty `Key` values. The `Key` field is critical as it's used to generate the variable definitions in terraform files.

## Solution

The fix involves ensuring that when querying variables from the database, we explicitly select all necessary fields, especially the `key` field which is essential for terraform configuration generation.

### Changes Required

1. **Update ResolveVariableSnapshots function** to explicitly select all fields when querying variables
2. **Add validation** to ensure variables have non-empty keys before using them
3. **Add debug logging** to help diagnose similar issues in the future

## Implementation

### File: backend/services/terraform_executor.go

Update the `ResolveVariableSnapshots` function to:

1. Explicitly select all fields when querying
2. Validate that the key field is not empty
3. Add debug logging for troubleshooting

```go
// Query with explicit field selection
err = s.db.Select("*").
    Where("variable_id = ? AND version = ?", varID, version).
    First(&variable).Error

if err != nil {
    return nil, fmt.Errorf("variable %s version %d not found: %w", varID, version, err)
}

// Validate that key is not empty
if variable.Key == "" {
    return nil, fmt.Errorf("variable %s version %d has empty key field", varID, version)
}

log.Printf("[DEBUG] Loaded variable: id=%s, version=%d, key=%s", 
    variable.VariableID, variable.Version, variable.Key)
```

## Testing

1. Create a plan task that generates a snapshot
2. Delete the pod running the task
3. Trigger the apply stage
4. Verify that `variables.tf.json` and `variables.tfvars` contain proper variable keys and values

## Expected Result

After the fix:

```json
{
  "variable": {
    "my_variable": {
      "type": "string",
      "description": "My variable description"
    }
  }
}
```

```
my_variable = "my_value"
```

## Status

- [x] Root cause identified
- [x] Fix implemented
- [ ] Testing completed
- [ ] Deployed to production

## Implementation Details

The fix has been implemented in `backend/services/terraform_executor.go` in the `ResolveVariableSnapshots` function:

1. **Explicit Field Selection**: Changed the database query from:
   ```go
   err = s.db.Where("variable_id = ? AND version = ?", varID, version).
       First(&variable).Error
   ```
   
   To:
   ```go
   err = s.db.Select("*").
       Where("variable_id = ? AND version = ?", varID, version).
       First(&variable).Error
   ```

2. **Key Validation**: Added validation to ensure the `key` field is not empty:
   ```go
   if variable.Key == "" {
       return nil, fmt.Errorf("variable %s version %d has empty key field (workspace_id=%s)", 
           varID, version, variable.WorkspaceID)
   }
   ```

3. **Debug Logging**: Added detailed logging for troubleshooting:
   ```go
   log.Printf("[DEBUG] Loaded variable from DB: id=%s, version=%d, key=%s, type=%s", 
       variable.VariableID, variable.Version, variable.Key, variable.VariableType)
   ```

## Next Steps

1. Deploy the fix to the development environment
2. Test with a plan+apply workflow where the pod is deleted between plan and apply
3. Verify that `variables.tf.json` and `variables.tfvars` contain proper variable keys and values
4. Deploy to production after successful testing
