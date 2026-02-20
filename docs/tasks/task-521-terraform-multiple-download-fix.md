# Task 521: Terraform Multiple Download Issue Fix

## Problem Analysis

### Issue Description
During a single plan execution, Terraform binary verification (`terraform version`) is being executed multiple times, appearing 8 times in the logs:

```
2025/11/07 03:38:03 terraform_downloader.go:163: Terraform binary verified: Terraform v1.13.4
```

### Root Cause
In `terraform_executor.go`, the `EnsureTerraformBinary()` method is called **4 times** during a single plan execution:

1. **Fetching stage** (line ~600) - Downloads and verifies binary ✓ (correct)
2. **TerraformInit** (line ~750) - Calls `EnsureTerraformBinary` again (redundant)
3. **Planning stage** (line ~900) - Calls `EnsureTerraformBinary` again (redundant)
4. **GeneratePlanJSON** (line ~1200) - Calls `EnsureTerraformBinary` again (redundant)

Each call to `EnsureTerraformBinary()` executes `checkExecutable()` which runs `terraform version`, causing the verification message to appear multiple times.

### Why 8 Times Instead of 4?
The logs show 8 occurrences, which suggests the task might be running in both Local and Agent modes, or there are multiple concurrent operations.

## Solution

### Approach
Cache the binary path in the `TerraformExecutor` struct after the first successful download/verification, and reuse it for subsequent operations within the same task execution.

### Implementation Steps

1. **Add binary path cache to TerraformExecutor**
   - Add `cachedBinaryPath` field to store the verified binary path
   - Add `cachedBinaryVersion` field to track which version is cached

2. **Modify ExecutePlan to cache binary path**
   - After first successful `EnsureTerraformBinary` call in Fetching stage, cache the result
   - Use cached path for subsequent operations (Init, Planning, GeneratePlanJSON)

3. **Modify ExecuteApply similarly**
   - Cache binary path after first download in Fetching stage
   - Reuse for Init and Applying stages

4. **Keep EnsureTerraformBinary logic unchanged**
   - The caching is at the executor level, not in the downloader
   - This preserves the downloader's ability to verify and download when needed

## Benefits

1. **Performance**: Reduces redundant `terraform version` executions from 4 to 1 per task
2. **Cleaner logs**: Eliminates duplicate verification messages
3. **Consistency**: Ensures the same binary is used throughout the task execution
4. **No breaking changes**: The downloader logic remains unchanged

## Testing

1. Run a plan task and verify only 1 "Terraform binary verified" message appears
2. Run a plan_and_apply task and verify only 2 messages appear (1 for plan, 1 for apply)
3. Verify different terraform versions still work correctly
4. Test in both Local and Agent modes
