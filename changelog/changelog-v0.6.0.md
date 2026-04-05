## v0.6.0

Terraform HTTP State Backend -- 将 Terraform state 管理从本地文件模式重构为 HTTP backend 协议，统一 Local/Agent/K8s 三种执行模式的 state 读写路径，消除手动文件操作，支持并发安全的锁机制。

### Features

#### HTTP State Backend 核心

- **新增** Terraform HTTP state backend handler（GET/POST/LOCK/UNLOCK），遵循官方 HTTP backend 协议
- **新增** 内部 HTTP 端口（SERVER_PORT+20, 127.0.0.1, plain HTTP）供 Local 模式使用，绕过 TLS 证书域名匹配
- **新增** `backend.tf.json` 自动生成，三处 `GenerateConfigFiles*` 方法统一注入
- **新增** `TF_HTTP_*` 环境变量注入（address, lock/unlock address, POST method, retry config）
- **新增** Checksum 去重：state 内容未变时跳过 version 创建
- **新增** Lock ID 验证：UpdateState 校验 `?ID=` 查询参数与当前持有的锁匹配
- **新增** DELETE state 返回 405 Method Not Allowed（不支持通过 HTTP backend 删除 state）
- **新增** `terraform plan` 自动添加 `-lock=false`（plan 不修改 state，避免阻塞并发 plan 或与 apply 锁冲突）
- **新增** Plan-only task 的 `terraform init` 也添加 `-lock=false`

#### JWT State Token 认证

- **新增** `StateTokenService`：为每个 task 生成 JWT token（7 天过期 + DB 活跃状态双重校验）
- **新增** `StateTokenAuth` 中间件：从 Basic Auth password 提取 JWT，校验 workspace 匹配
- **新增** Token hash 存储（SHA256），不存原文
- **新增** DB 不可用时降级为只验证 JWT 签名（避免 DB 短暂故障中断正在进行的 apply）
- **新增** Token 生命周期管理：Local 模式 defer revoke（仅终态），Agent 模式 `UpdateTaskStatus` 终态 revoke

#### 统一锁机制

- **重构** Workspace 锁字段：`is_locked/locked_by/locked_at/lock_reason` 合并为 `lock_id/lock_info`（JSONB）
- **重构** UI 手动锁和 Terraform 运行时锁共享同一字段，原子操作（PostgreSQL 行锁）
- **重构** `lock_info` 包含 `who/who_display/operation/info/created`，API 返回时自动查询用户名注入 `who_display`
- **适配** 19+ 文件的旧锁字段引用全部替换，涵盖 services/controllers/handlers/frontend

#### Executor 改造

- **重构** `PrepareStateFile`/`PrepareStateFileWithLogging`：HTTP backend 模式下跳过（Terraform 通过 HTTP GET 获取 state）
- **重构** `SaveNewStateVersion`/`SaveNewStateVersionWithLogging`：HTTP backend 模式下验证 DB state，支持 apply 无变更场景
- **重构** `ExtractResourceDetailsFromState`：从 DB 读取 state（不再从文件读取），适配两种模式
- **重构** `saveTaskCancellation`/`saveTaskFailure`：HTTP backend 模式下从 DB 验证 partial state
- **重构** `StateFileWatcher`：HTTP backend 模式下不启动（Terraform 每次资源变更后 HTTP POST 自动保存）
- **重构** `cleanProviderConfig`：过滤 `terraform.backend` 键，防止与 `backend.tf.json` 冲突
- **新增** `fallbackSaveFromErroredState`：从 `errored.tfstate` 降级恢复，5 次重试 + 自动锁定 workspace
- **新增** `ForTask()` 方法：创建 per-task executor 浅拷贝，避免并发 goroutine 覆盖 state backend 配置
- **修复** HTTP backend 模式下跳过 plan 完成后的 workspace auto-lock（防止与 Terraform HTTP LOCK 冲突）

#### Agent/K8s 模式

- **新增** `GetTaskData` 返回 `state_backend.url` 和 `state_backend.token`，URL 从 `PlatformConfigService.GetBaseURL()` 构造
- **新增** `RemoteDataAccessor.GetStateBackendConfig()` 解析 state backend 配置
- **新增** `NewTerraformExecutorWithAccessor` 自动从 task data 设置 HTTP state backend
- **新增** CA 证书注入（`_TERRANOVA_CA_CERT` → `TF_HTTP_CLIENT_CA_CERTIFICATE_PEM`，传 PEM 内容非文件路径）

#### 前端适配

- **重构** TypeScript 类型：`is_locked/locked_by/locked_at/lock_reason` → `lock_id/lock_info`
- **适配** `WorkspaceDetail.tsx`/`WorkspaceSettings.tsx`/`StateUpload.tsx` 锁状态 UI

#### 构建与部署

- **新增** Build version 注入：commit hash + build time 通过 `-ldflags -X` 注入二进制
- **适配** Makefile、Dockerfile（server + agent）、GitHub Actions CI/Release 统一注入

### Removed

- **删除** `SaveTaskState` endpoint（Agent API）-- 被 HTTP backend POST 替代
- **删除** `SaveStateVersion`/`SaveTaskStateWithRetry`（Agent API client）-- 不再手动 POST state
- **删除** `workspace_task_controller.go.bak` 备份文件
- **标记废弃** UpsertTempState/PromoteTempState/CleanupOrphanedTempStates（temp state 机制在 HTTP backend 下冗余）

### Bug Fixes

- **修复** Drift check 漂移检测无结果：`parsePlanChanges` 只解析 `resource_changes`，未解析 `-refresh-only` 模式下的 `resource_drift` 字段，导致外部变更（如手动修改 S3 tag）检测不到

### Database Migration

- `backend/migrations/add_http_state_backend.sql`
  - workspaces: 新增 `lock_id`/`lock_info`，删除 `is_locked`/`locked_by`/`locked_at`/`lock_reason`
  - workspace_tasks: 新增 `state_token_hash`（部分索引）
  - 迁移现有锁数据到新字段

### Branding

- **新增** Terranova 品牌 logo（SVG 纯文字版）替换 Vite 默认 logo
- **新增** Favicon（深蓝圆角方块 + 白色 T 字母）替换 Vite 默认 favicon
- **修改** 页面标题从 "IaC 平台" 改为 "Terranova"
- **清理** 删除 `vite.svg`、`react.svg` 默认资源

### Breaking Changes

- Workspace API 响应中 `is_locked`/`locked_by`/`locked_at`/`lock_reason` 字段替换为 `lock_id`/`lock_info`
- `DataAccessor.LockWorkspace` 接口签名变更：`(workspaceID, userID, reason string)` → `(workspaceID string, lockInfo map[string]interface{})`
