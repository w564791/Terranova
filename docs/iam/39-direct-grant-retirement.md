# Direct Grant 下线说明（D5）

> **日期**：2026-07-17  
> **依据**：`32` §3.2 / D5 — Role 主模型；Direct Grant 不对业务管理员作平行入口  
> **状态**：**USER / TEAM / APPLICATION** HTTP 写路径均已退役（→ **410**）

---

## 1. 行为变更

| 接口 | USER / TEAM / APPLICATION |
|------|---------------------------|
| `POST /iam/permissions/grant` | **410 Gone** |
| `POST /iam/permissions/batch-grant` | **410** |
| `POST /iam/permissions/grant-preset` | **410** |
| `DELETE /iam/permissions/:scope/:id` | **仍可用**（清理遗留） |
| `GET` 列表 / 按主体查 | 仍可用（只读遗留数据） |

响应示例：

```json
{
  "code": 410,
  "error": "Direct Grant is retired; assign a Role instead (POST /iam/users|teams|applications/:id/roles).",
  "deprecated": true,
  "alternative": "role_assignment"
}
```

**应急恢复**（勿默认开启）：`IAM_ALLOW_DIRECT_GRANT=1`

**内部保留**：

- 创建 Workspace 时优先 `AssignBuiltinRoleToUser(..., "workspace_admin", ...)`
- 若角色未 seed，回退 `permissionService.GrantPresetPermissions`（service 层，不经 HTTP）

---

## 2. 业务运维主路径

1. 管理台 **角色** 配置 policies  
2. **分配角色**：
   - 用户：`POST /iam/users/:id/roles`
   - 团队：`POST /iam/teams/:id/roles`
   - 应用：`POST /iam/applications/:id/roles`（`:id` 可为数字主键或 **app_key**；库内存 **app_key**）
3. 前端「新增授权」统一 **分配角色**

Application 细粒度：组织 Role + `applications.workspace_tag_filter`（见 `38`）。

---

## 3. 发布迁移（唯一支持路径）

生产发布只运行镜像内的 `iac-migrate`（Compose 的 `migrate` service 或集群发布 Job）。它在一个事务中取得 PostgreSQL advisory lock，并将已执行版本写入 `schema_migrations`。当前迁移会完成：

- `workspace_admin` 与必要 IAM 索引；
- USER/TEAM 及 APPLICATION 的有效 Direct Grant → 合成 Role（**不删除**原 grant 行）；
- app principal 规范化、角色组织归属校验，以及无法安全归属的 legacy Role 隔离。

`backend/migrations/patch_*.sql` 是历史排障/人工恢复材料，**不得**与 `iac-migrate` 在同一环境或同一次发布中混用。它们没有版本记录、事务外发布屏障和现行的所有 tenant 校验；在已运行 runner 的库上再次执行可能产生与当前规则不同的 Role 数据。

Checker **双读**：遗留 `*_permissions` + Role 求值。确认 Role 无误后，再以单独、审计过的变更清理 legacy grant。

---

## 4. 遗留数据

- `org/project/workspace_permissions` 中行：Checker **只读仍生效**
- 建议：运行 `iac-migrate` → 验证 → 再撤销对应 direct grant
- APPLICATION 历史 grant 同样只读生效，直至改为 `iam_application_roles`

---

## 5. 前端入口

| 页面 | 变更 |
|------|------|
| GrantPermission | 全部主体 → 仅 Role；APPLICATION → `/applications/:id/roles` |
| PermissionManagement | 跳转 `type=role`；编辑 USER/TEAM grant 提示改 Role |
| TeamDetail | 跳转 `type=role`；禁止 re-grant 改 level |

---

## 6. 测试

```bash
cd backend && go test ./internal/handlers/ -count=1 -run 'GrantPermission|BatchGrant|GrantPreset|ApplicationRole'
cd backend && go test ./internal/application/service/ -count=1
```
