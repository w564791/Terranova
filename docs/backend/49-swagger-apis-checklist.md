# Swagger API文档完成情况

##  已完成的API

### Auth (刚添加)
- POST /api/v1/auth/login
- POST /api/v1/auth/register
- POST /api/v1/auth/logout
- POST /api/v1/auth/refresh
- GET /api/v1/auth/me
- POST /api/v1/user/reset-password

### Admin - Terraform版本管理
- GET /api/v1/admin/terraform-versions
- POST /api/v1/admin/terraform-versions
- GET /api/v1/admin/terraform-versions/:id
- PUT /api/v1/admin/terraform-versions/:id
- DELETE /api/v1/admin/terraform-versions/:id
- GET /api/v1/admin/terraform-versions/default
- POST /api/v1/admin/terraform-versions/:id/set-default

### Workspace - 基本操作
- GET /api/v1/workspaces
- POST /api/v1/workspaces
- GET /api/v1/workspaces/:id
- PUT /api/v1/workspaces/:id
- DELETE /api/v1/workspaces/:id
- GET /api/v1/workspaces/:id/overview
- GET /api/v1/workspaces/form-data

## ❌ 需要添加的API

### Dashboard
- GET /api/v1/dashboard/overview
- GET /api/v1/dashboard/compliance

### Modules
- GET /api/v1/modules
- POST /api/v1/modules
- GET /api/v1/modules/:id
- PUT /api/v1/modules/:id
- DELETE /api/v1/modules/:id
- POST /api/v1/modules/:id/sync
- GET /api/v1/modules/:id/files
- POST /api/v1/modules/parse-tf
- GET /api/v1/modules/:id/schemas
- POST /api/v1/modules/:id/schemas
- POST /api/v1/modules/:id/schemas/generate
- GET /api/v1/modules/:id/demos
- POST /api/v1/modules/:id/demos

### Demos
- GET /api/v1/demos/:id
- PUT /api/v1/demos/:id
- DELETE /api/v1/demos/:id
- GET /api/v1/demos/:id/versions
- GET /api/v1/demos/:id/compare
- POST /api/v1/demos/:id/rollback
- GET /api/v1/demo-versions/:versionId

### Schemas
- GET /api/v1/schemas/:id
- PUT /api/v1/schemas/:id

### Workspace - 锁定管理
- POST /api/v1/workspaces/:id/lock
- POST /api/v1/workspaces/:id/unlock

### Workspace - 任务管理
- POST /api/v1/workspaces/:id/tasks/plan
- POST /api/v1/workspaces/:id/tasks/apply
- GET /api/v1/workspaces/:id/tasks
- GET /api/v1/workspaces/:id/tasks/:task_id
- GET /api/v1/workspaces/:id/tasks/:task_id/logs
- POST /api/v1/workspaces/:id/tasks/:task_id/cancel
- POST /api/v1/workspaces/:id/tasks/:task_id/cancel-previous
- POST /api/v1/workspaces/:id/tasks/:task_id/confirm-apply
- POST /api/v1/workspaces/:id/tasks/:task_id/retry-state-save
- GET /api/v1/workspaces/:id/tasks/:task_id/state-backup
- POST /api/v1/workspaces/:id/tasks/:task_id/comments
- GET /api/v1/workspaces/:id/tasks/:task_id/comments
- GET /api/v1/workspaces/:id/tasks/:task_id/resource-changes
- PATCH /api/v1/workspaces/:id/tasks/:task_id/resource-changes/:resource_id
- POST /api/v1/workspaces/:id/tasks/:task_id/parse-plan

### Task Logs
- GET /api/v1/tasks/:task_id/output/stream (WebSocket)
- GET /api/v1/tasks/:task_id/logs
- GET /api/v1/tasks/:task_id/logs/download
- GET /api/v1/terraform/streams/stats

### Workspace - State版本控制
- GET /api/v1/workspaces/:id/current-state
- GET /api/v1/workspaces/:id/state-versions
- GET /api/v1/workspaces/:id/state-versions/compare
- GET /api/v1/workspaces/:id/state-versions/:version/metadata
- GET /api/v1/workspaces/:id/state-versions/:version
- POST /api/v1/workspaces/:id/state-versions/:version/rollback
- DELETE /api/v1/workspaces/:id/state-versions/:version

### Workspace - 变量管理
- GET /api/v1/workspaces/:id/variables
- POST /api/v1/workspaces/:id/variables
- GET /api/v1/workspaces/:id/variables/:var_id
- PUT /api/v1/workspaces/:id/variables/:var_id
- DELETE /api/v1/workspaces/:id/variables/:var_id

### Workspace - 资源管理
- GET /api/v1/workspaces/:id/resources
- POST /api/v1/workspaces/:id/resources
- POST /api/v1/workspaces/:id/resources/import
- POST /api/v1/workspaces/:id/resources/deploy
- GET /api/v1/workspaces/:id/resources/:resource_id
- PUT /api/v1/workspaces/:id/resources/:resource_id
- DELETE /api/v1/workspaces/:id/resources/:resource_id
- POST /api/v1/workspaces/:id/resources/:resource_id/restore

### Workspace - 资源版本管理
- GET /api/v1/workspaces/:id/resources/:resource_id/versions
- GET /api/v1/workspaces/:id/resources/:resource_id/versions/compare
- GET /api/v1/workspaces/:id/resources/:resource_id/versions/:version
- POST /api/v1/workspaces/:id/resources/:resource_id/versions/:version/rollback

### Workspace - 资源依赖关系
- GET /api/v1/workspaces/:id/resources/:resource_id/dependencies
- PUT /api/v1/workspaces/:id/resources/:resource_id/dependencies

### Workspace - 快照管理
- POST /api/v1/workspaces/:id/snapshots
- GET /api/v1/workspaces/:id/snapshots
- GET /api/v1/workspaces/:id/snapshots/:snapshot_id
- POST /api/v1/workspaces/:id/snapshots/:snapshot_id/restore
- DELETE /api/v1/workspaces/:id/snapshots/:snapshot_id

### Workspace - 资源编辑协作
- POST /api/v1/workspaces/:id/resources/:resource_id/editing/start
- POST /api/v1/workspaces/:id/resources/:resource_id/editing/heartbeat
- POST /api/v1/workspaces/:id/resources/:resource_id/editing/end
- GET /api/v1/workspaces/:id/resources/:resource_id/editing/status
- POST /api/v1/workspaces/:id/resources/:resource_id/drift/save
- GET /api/v1/workspaces/:id/resources/:resource_id/drift
- DELETE /api/v1/workspaces/:id/resources/:resource_id/drift
- POST /api/v1/workspaces/:id/resources/:resource_id/drift/takeover

### Agents
- POST /api/v1/agents/register
- POST /api/v1/agents/heartbeat
- GET /api/v1/agents
- GET /api/v1/agents/:id
- PUT /api/v1/agents/:id
- DELETE /api/v1/agents/:id
- POST /api/v1/agents/:id/revoke-token
- POST /api/v1/agents/:id/regenerate-token

### Agent Pools
- POST /api/v1/agent-pools
- GET /api/v1/agent-pools
- GET /api/v1/agent-pools/:id
- PUT /api/v1/agent-pools/:id
- DELETE /api/v1/agent-pools/:id
- POST /api/v1/agent-pools/:id/agents
- DELETE /api/v1/agent-pools/:id/agents/:agent_id

### Admin - AI配置管理
- GET /api/v1/admin/ai-configs
- POST /api/v1/admin/ai-configs
- GET /api/v1/admin/ai-configs/:id
- PUT /api/v1/admin/ai-configs/:id
- DELETE /api/v1/admin/ai-configs/:id
- GET /api/v1/admin/ai-config/regions
- GET /api/v1/admin/ai-config/models

### AI分析
- POST /api/v1/ai/analyze-error
- GET /api/v1/workspaces/:id/tasks/:task_id/error-analysis

## 统计
- 已完成: 15个API
- 待添加: 约120个API
- 完成度: 约11%
