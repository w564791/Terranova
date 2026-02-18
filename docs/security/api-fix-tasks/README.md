# API 权限修复任务

> 按 API 功能模块分类，每个文件包含该模块的**全部 API 列表**及权限状态。
> ✅ = 合格   = 可优化  ❌ = 需整改

## 文件清单

| 文件 | 模块 | API 总数 | 需修复 | 风险 |
|------|------|----------|--------|------|
| 01-public-and-auth.md | 公开端点 + 认证 + Setup | 18 | 1 | 🟡 |
| 02-sso.md | SSO 公开 + 身份管理 + 管理端点 | 17 | 12 | 🟡 |
| 03-websocket.md | WebSocket 端点 | 3 | 1 | 🟢 |
| 04-agent-api.md | Agent API (PoolToken) | 22 | 0 | - |
| 05-run-task.md | Run Task 回调 + 管理 | 10 | 3 | 🔴 |
| 06-secrets.md | 密文管理 | 5 | 5 | 🔴 |
| 07-user-self-service.md | 用户自服务 (密码/Token/MFA) | 10 | 4 | 🟢 |
| 08-workspace.md | Workspace CRUD + Pool关联 | 12 | 0 | - |
| 09-workspace-tasks.md | Workspace Task 操作 | 15 | 0 | - |
| 10-workspace-state.md | Workspace State 操作 | 14 | 0 | - |
| 11-workspace-variables.md | Workspace Variables | 7 | 0 | - |
| 12-workspace-resources.md | Workspace Resources | 22 | 0 | - |
| 13-workspace-misc.md | Workspace 其他 (snapshot/drift/embedding/output/remote-data/run-trigger/notification) | 42 | 0 | - |
| 14-manifest.md | Manifest 可视化编排 | 20 | 20 | 🔴 |
| 15-modules.md | Module 管理 (含 Schema V2/Version) | 30 | 0 | - |
| 16-demos-schemas.md | Demo + Schema 独立路由 | 9 | 0 | - |
| 17-ai.md | AI 分析 + Embedding | 12 | 1 | 🟡 |
| 18-admin-skills-embedding.md | Admin Skills/Embedding/Cache | 25 | 25 | 🟡 |
| 19-iam.md | IAM 权限/团队/组织/项目/应用/审计/用户/角色 | 64 | 0 | - |
| 20-agent-pools.md | Agent Pool 管理 (JWT) + Workspace-Pool关联 | 19 | 0 | - |
| 21-global-settings.md | 全局设置 (TF版本/AI配置/平台/MFA) + Admin MFA | 22 | 0 | - |
| 22-notifications.md | 通知管理 | 7 | 7 | 🟡 |
| 23-projects.md | Project 管理 | 2 | 0 | - |
| 24-tasks-global.md | 全局 Task 日志 | 4 | 0 | - |
| 25-dashboard.md | Dashboard | 2 | 2 | 🟢 |
| 26-cmdb.md | CMDB 资源索引 | 16 | 7 | 🟡 |

**合计: ~375 个 API，其中约 88 个需关注**

## 修复优先级

| 优先级 | 模块 | 说明 |
|--------|------|------|
| P0 | 05-run-task, 06-secrets, 14-manifest | 无认证或严重越权 |
| P1 | 22-notifications, 26-cmdb, 02-sso | 缺 IAM 或数据隔离 |
| P2 | 18-admin-skills-embedding, 07-user-self-service | 隐式拒绝 / 可选修复 |
| P3 | 25-dashboard, 01-public-and-auth, 03-websocket, 17-ai | 不一致 / 低风险 |
