# Workspace Task and Variable IAM Permission Implementation

## Overview

This document describes the IAM permission controls implemented for workspace task operations and workspace variable management to ensure proper access control based on user permissions.

## Problem Statement

Previously, all workspace task operations lacked IAM permission checks, allowing any user with workspace access to perform all operations including administrative actions like canceling tasks or confirming applies. This created a security vulnerability where READ-only users could perform ADMIN-level operations.

## Solution

Implemented three-tier permission control for workspace task operations using the `WORKSPACE_EXECUTION` resource type:

### Permission Levels

#### READ Level
Users with READ permission can view task information but cannot modify anything:

- `GET /:id/tasks` - List all tasks
- `GET /:id/tasks/:task_id` - View task details
- `GET /:id/tasks/:task_id/logs` - View task logs
- `GET /:id/tasks/:task_id/comments` - View comments
- `GET /:id/tasks/:task_id/resource-changes` - View resource changes
- `GET /:id/tasks/:task_id/state-backup` - Download state backup

#### WRITE Level
Users with WRITE permission can create tasks and add comments:

- `POST /:id/tasks/plan` - Create a new Plan task
- `POST /:id/tasks/:task_id/comments` - Add comments to tasks

#### ADMIN Level
Users with ADMIN permission can perform critical management operations:

- `POST /:id/tasks/:task_id/cancel` - Cancel a running task
- `POST /:id/tasks/:task_id/cancel-previous` - Cancel previous tasks
- `POST /:id/tasks/:task_id/confirm-apply` - Confirm and execute Apply
- `PATCH /:id/tasks/:task_id/resource-changes/:resource_id` - Update resource apply status
- `POST /:id/tasks/:task_id/retry-state-save` - Retry state save on failure
- `POST /:id/tasks/:task_id/parse-plan` - Manually trigger plan parsing

## Implementation Details

### Permission Check Pattern

Each protected route follows this pattern:

```go
workspaces.GET("/:id/tasks", func(c *gin.Context) {
    role, _ := c.Get("role")
    if role == "admin" {
        taskController.GetTasks(c)
        return
    }
    iamMiddleware.RequirePermission("WORKSPACE_EXECUTION", "WORKSPACE", "READ")(c)
    if !c.IsAborted() {
        taskController.GetTasks(c)
    }
})
```

### Admin Bypass

System administrators (role="admin") bypass IAM checks and have full access to all operations. This ensures system maintenance capabilities while still enforcing permissions for regular users.

### Scope Type

All workspace task permissions use `WORKSPACE` scope type, meaning permissions are granted per workspace using the workspace ID from the URL path parameter `:id`.

## Permission Matrix

### Task Operations (WORKSPACE_EXECUTION)

| Operation | HTTP Method | Path | Required Permission | Level |
|-----------|-------------|------|---------------------|-------|
| List tasks | GET | `/:id/tasks` | WORKSPACE_EXECUTION | READ |
| View task | GET | `/:id/tasks/:task_id` | WORKSPACE_EXECUTION | READ |
| View logs | GET | `/:id/tasks/:task_id/logs` | WORKSPACE_EXECUTION | READ |
| View comments | GET | `/:id/tasks/:task_id/comments` | WORKSPACE_EXECUTION | READ |
| View resource changes | GET | `/:id/tasks/:task_id/resource-changes` | WORKSPACE_EXECUTION | READ |
| Download state backup | GET | `/:id/tasks/:task_id/state-backup` | WORKSPACE_EXECUTION | READ |
| Create plan task | POST | `/:id/tasks/plan` | WORKSPACE_EXECUTION | WRITE |
| Add comment | POST | `/:id/tasks/:task_id/comments` | WORKSPACE_EXECUTION | WRITE |
| Cancel task | POST | `/:id/tasks/:task_id/cancel` | WORKSPACE_EXECUTION | ADMIN |
| Cancel previous tasks | POST | `/:id/tasks/:task_id/cancel-previous` | WORKSPACE_EXECUTION | ADMIN |
| Confirm apply | POST | `/:id/tasks/:task_id/confirm-apply` | WORKSPACE_EXECUTION | ADMIN |
| Update resource status | PATCH | `/:id/tasks/:task_id/resource-changes/:resource_id` | WORKSPACE_EXECUTION | ADMIN |
| Retry state save | POST | `/:id/tasks/:task_id/retry-state-save` | WORKSPACE_EXECUTION | ADMIN |
| Parse plan manually | POST | `/:id/tasks/:task_id/parse-plan` | WORKSPACE_EXECUTION | ADMIN |

### Variable Operations (WORKSPACE_MANAGEMENT)

| Operation | HTTP Method | Path | Required Permission | Level |
|-----------|-------------|------|---------------------|-------|
| List variables | GET | `/:id/variables` | WORKSPACE_MANAGEMENT | READ |
| View variable | GET | `/:id/variables/:var_id` | WORKSPACE_MANAGEMENT | READ |
| Create variable | POST | `/:id/variables` | WORKSPACE_MANAGEMENT | WRITE |
| Update variable | PUT | `/:id/variables/:var_id` | WORKSPACE_MANAGEMENT | WRITE |
| Delete variable | DELETE | `/:id/variables/:var_id` | WORKSPACE_MANAGEMENT | ADMIN |

## Security Benefits

1. **Principle of Least Privilege**: Users only get the minimum permissions needed for their role
2. **Audit Trail**: All permission checks are logged through the audit middleware
3. **Granular Control**: Different permission levels for different operations
4. **Workspace Isolation**: Permissions are scoped per workspace
5. **Prevention of Unauthorized Actions**: READ users cannot cancel tasks or confirm applies

## Usage Examples

### Granting Task Execution Permissions

To grant a user READ access to workspace tasks:

```bash
curl -X POST http://localhost:8080/api/v1/iam/permissions/grant \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "principal_type": "USER",
    "principal_id": 2,
    "resource_type": "WORKSPACE_EXECUTION",
    "scope_type": "WORKSPACE",
    "scope_id": 1,
    "permission_level": "READ"
  }'
```

To grant ADMIN access for task operations:

```bash
curl -X POST http://localhost:8080/api/v1/iam/permissions/grant \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "principal_type": "USER",
    "principal_id": 2,
    "resource_type": "WORKSPACE_EXECUTION",
    "scope_type": "WORKSPACE",
    "scope_id": 1,
    "permission_level": "ADMIN"
  }'
```

### Granting Variable Management Permissions

To grant a user READ access to workspace variables:

```bash
curl -X POST http://localhost:8080/api/v1/iam/permissions/grant \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "principal_type": "USER",
    "principal_id": 2,
    "resource_type": "WORKSPACE_MANAGEMENT",
    "scope_type": "WORKSPACE",
    "scope_id": 1,
    "permission_level": "READ"
  }'
```

To grant WRITE access for variable management:

```bash
curl -X POST http://localhost:8080/api/v1/iam/permissions/grant \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "principal_type": "USER",
    "principal_id": 2,
    "resource_type": "WORKSPACE_MANAGEMENT",
    "scope_type": "WORKSPACE",
    "scope_id": 1,
    "permission_level": "WRITE"
  }'
```

## Testing

### Test Task Execution Permissions

#### Test READ Permission

1. Grant READ permission to a test user
2. Verify they can:
   - List tasks
   - View task details
   - View logs and comments
3. Verify they cannot:
   - Create plan tasks (should get 403)
   - Cancel tasks (should get 403)
   - Confirm applies (should get 403)

#### Test WRITE Permission

1. Grant WRITE permission to a test user
2. Verify they can:
   - All READ operations
   - Create plan tasks
   - Add comments
3. Verify they cannot:
   - Cancel tasks (should get 403)
   - Confirm applies (should get 403)

#### Test ADMIN Permission

1. Grant ADMIN permission to a test user
2. Verify they can:
   - All READ and WRITE operations
   - Cancel tasks
   - Confirm applies
   - Update resource statuses

### Test Variable Management Permissions

#### Test READ Permission

1. Grant READ permission for WORKSPACE_MANAGEMENT to a test user
2. Verify they can:
   - List variables
   - View variable details
3. Verify they cannot:
   - Create variables (should get 403)
   - Update variables (should get 403)
   - Delete variables (should get 403)

#### Test WRITE Permission

1. Grant WRITE permission for WORKSPACE_MANAGEMENT to a test user
2. Verify they can:
   - All READ operations
   - Create variables
   - Update variables
3. Verify they cannot:
   - Delete variables (should get 403)

#### Test ADMIN Permission

1. Grant ADMIN permission for WORKSPACE_MANAGEMENT to a test user
2. Verify they can:
   - All READ and WRITE operations
   - Delete variables

## Related Files

- `backend/internal/router/router.go` - Route definitions with permission checks
- `backend/internal/middleware/iam_permission.go` - IAM permission middleware
- `backend/internal/domain/valueobject/resource_type.go` - Resource type definitions
- `frontend/src/pages/admin/PermissionManagement.tsx` - Permission management UI

## Migration Notes

Existing users with workspace access will need to be granted appropriate permissions:

### WORKSPACE_EXECUTION Permissions
- Developers/Operators: WRITE level (can create and view tasks)
- Managers/Approvers: ADMIN level (can cancel and confirm applies)
- Auditors/Viewers: READ level (can only view task information)

### WORKSPACE_MANAGEMENT Permissions
- Developers/Operators: WRITE level (can create and update variables)
- Senior Engineers: ADMIN level (can delete variables)
- Auditors/Viewers: READ level (can only view variables)

System administrators (role="admin") continue to have full access without explicit permission grants.

## Future Enhancements

1. Consider adding a REVIEW permission level between WRITE and ADMIN for users who can approve but not cancel
2. Add permission checks for state version operations
3. Add permission checks for resource operations (WORKSPACE_RESOURCES permission)
4. Add permission checks for workspace lock/unlock operations
5. Implement team-based permission inheritance
6. Add bulk permission grant operations for common role patterns
