# Environment Variable Injection Issue Analysis

## Issue Description
Environment variable类型的变量没有注入到系统环境里，导致terraform命令没法读取到一些权限相关的环境变量。

## Code Analysis

### Current Implementation (terraform_executor.go, lines 318-343)

```go
func (s *TerraformExecutor) buildEnvironmentVariables(
	workspace *models.Workspace,
) []string {
	env := append(os.Environ(),
		"TF_IN_AUTOMATION=true",
		"TF_INPUT=false",
	)

	// 从workspace_variables表读取环境变量（使用 DataAccessor）
	if envVars, err := s.dataAccessor.GetWorkspaceVariables(workspace.WorkspaceID, models.VariableTypeEnvironment); err == nil {
		for _, v := range envVars {
			// 跳过TF_CLI_ARGS，它会被特殊处理添加到命令参数中
			if v.Key == "TF_CLI_ARGS" {
				continue
			}
			env = append(env, fmt.Sprintf("%s=%s", v.Key, v.Value))
		}
	}

	// AWS Provider - 使用IAM Role
	if workspace.ProviderConfig != nil {
		if awsConfig, ok := workspace.ProviderConfig["aws"].([]interface{}); ok && len(awsConfig) > 0 {
			aws := awsConfig[0].(map[string]interface{})

			// 设置region（必需）
			if region, ok := aws["region"].(string); ok {
				env = append(env, fmt.Sprintf("AWS_DEFAULT_REGION=%s", region))
				env = append(env, fmt.Sprintf("AWS_REGION=%s", region))
			}
		}
	}

	return env
}
```

## Potential Issues Identified

### Issue 1: Silent Error Handling
```go
if envVars, err := s.dataAccessor.GetWorkspaceVariables(workspace.WorkspaceID, models.VariableTypeEnvironment); err == nil {
```

**Problem**: If there's an error retrieving environment variables, it's silently ignored. No log, no warning. The function continues and returns an environment without the workspace-specific variables.

**Impact**: 
- If database query fails, environment variables won't be injected
- No visibility into why variables are missing
- Difficult to debug in production

### Issue 2: Potential Variable Override
The AWS region variables are added **after** workspace environment variables. If a user sets `AWS_REGION` or `AWS_DEFAULT_REGION` as environment variables, they will be **overridden** by the provider config values.

**Order of precedence (current)**:
1. System environment (`os.Environ()`)
2. TF_IN_AUTOMATION, TF_INPUT
3. Workspace environment variables
4. AWS region from provider config (overwrites if exists)

### Issue 3: No Logging for Debugging
There's no logging to show:
- How many environment variables were loaded
- Which variables were added
- If any errors occurred during retrieval

## Recommended Fixes

### Fix 1: Add Error Logging
```go
envVars, err := s.dataAccessor.GetWorkspaceVariables(workspace.WorkspaceID, models.VariableTypeEnvironment)
if err != nil {
	log.Printf("WARNING: Failed to get environment variables for workspace %s: %v", workspace.WorkspaceID, err)
} else {
	log.Printf("DEBUG: Loaded %d environment variables for workspace %s", len(envVars), workspace.WorkspaceID)
	for _, v := range envVars {
		if v.Key == "TF_CLI_ARGS" {
			continue
		}
		env = append(env, fmt.Sprintf("%s=%s", v.Key, v.Value))
		log.Printf("DEBUG: Added environment variable: %s", v.Key)
	}
}
```

### Fix 2: Respect User-Defined AWS Variables
Only set AWS region from provider config if not already set by user:
```go
// Check if user has already set AWS region variables
hasAWSRegion := false
for _, v := range envVars {
	if v.Key == "AWS_REGION" || v.Key == "AWS_DEFAULT_REGION" {
		hasAWSRegion = true
		break
	}
}

// Only set from provider config if user hasn't set it
if !hasAWSRegion && workspace.ProviderConfig != nil {
	// ... existing AWS region logic
}
```

### Fix 3: Add Sensitive Variable Masking in Logs
```go
for _, v := range envVars {
	if v.Key == "TF_CLI_ARGS" {
		continue
	}
	env = append(env, fmt.Sprintf("%s=%s", v.Key, v.Value))
	if v.Sensitive {
		log.Printf("DEBUG: Added environment variable: %s=***SENSITIVE***", v.Key)
	} else {
		log.Printf("DEBUG: Added environment variable: %s=%s", v.Key, v.Value)
	}
}
```

## Testing Checklist

After fix, verify:
- [ ] Environment variables are retrieved from database
- [ ] Variables are properly formatted as KEY=VALUE
- [ ] Variables are passed to terraform command
- [ ] Error cases are logged
- [ ] AWS credentials environment variables work
- [ ] Sensitive variables are masked in logs
- [ ] User-defined AWS region variables are not overridden

## Next Steps

1. **Confirm the issue**: Check logs to see if environment variables are being retrieved
2. **Apply fixes**: Implement error logging and debugging
3. **Test**: Verify environment variables are injected correctly
4. **Monitor**: Check logs after deployment
