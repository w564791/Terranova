# 23 — Project 管理

> 源文件: `router_project.go`
> API 数量: 2
> 状态: ✅ 全部合格

## 全部 API 列表

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 1 | GET | /api/v1/projects | JWT+AuditLogger | admin绕过 / WORKSPACES/ORG/READ | ✅ |
| 2 | GET | /api/v1/projects/:id/workspaces | JWT+AuditLogger | admin绕过 / WORKSPACES/ORG/READ | ✅ |

## 无需修复

Project 列表接口复用 WORKSPACES 资源类型的 READ 权限，因为项目查看与 workspace 查看是关联操作。
