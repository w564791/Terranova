# Application Principal 集成指南（选项 A）

> **日期**：2026-07-17  
> **产品决策**：启用 Application 作为 IAM 主体（见 `37` R0-3 → **A**）  
> **与 Agent 任务分离**：Terraform 执行 / 锁 / state 上传仍走 **Pool Token**（`/api/v1/agents/*`）

---

## 1. 概念

| 主体 | 鉴权 | 典型用途 |
|------|------|----------|
| USER / TEAM | JWT / Team Token | 控制台、人工运维 |
| **APPLICATION** | `X-App-Key` + `X-App-Secret` | CI、外部系统只读/集成 API |
| Agent（执行） | Pool Token | 任务执行通道，**不是** Application IAM |

**principal_id** 运行时与授权表统一为 **`app_key`**（不是数字主键）。  
管理台若误传数字 id，后端 grant 会解析成 `app_key`；Checker 也会展开 id↔key 兼容历史数据。

Application 授权主路径为 **Role 赋值**（`iam_application_roles`，principal=`app_key`）：

```http
POST /api/v1/iam/applications/<id|app_key>/roles?org_id=1
Authorization: Bearer <admin_jwt>
Content-Type: application/json

{
  "role_id": 12,
  "scope_type": "ORGANIZATION",
  "scope_id": 1,
  "reason": "CI list workspaces"
}
```

HTTP Direct Grant（`POST /iam/permissions/grant`）对 APPLICATION 已 **410**（见 `39`）。  
遗留 org grant 行 Checker 仍可读；细粒度 workspace 继续用 `workspace_tag_filter`。

---

## 2. 前置步骤

### 2.1 创建 Application

管理台 **IAM → 应用**，或：

```http
POST /api/v1/iam/applications?org_id=1
Authorization: Bearer <admin_jwt>
Content-Type: application/json

{
  "org_id": 1,
  "name": "ci-readonly",
  "description": "CI 只读",
  "workspace_tag_filter": { "env": "prod" }
}
```

响应中的 **`app_key` / `app_secret` 仅创建时展示一次**。

可选字段 **`workspace_tag_filter`**：

| 值 | 含义 |
|----|------|
| 省略 / `null` / `{}` | 不按 tag 限制（org 内全部 WS，仍需 grant） |
| `{"env":"prod"}` | `workspace.tags.env` 必须等于 `prod` |
| `{"env":["prod","staging"],"team":"platform"}` | **AND**：env 属于集合且 team=platform |

SQL 列：`applications.workspace_tag_filter`（见 `patch_applications_workspace_tag_filter.sql`）。

### 2.2 分配角色（推荐）

管理台 **授权** → 主体类型 **应用** → **分配角色**（含含 WORKSPACES 等 policy 的 Role）。

API：

```http
POST /api/v1/iam/applications/<app_key>/roles?org_id=1
Authorization: Bearer <admin_jwt>
Content-Type: application/json

{
  "role_id": <role_id>,
  "scope_type": "ORGANIZATION",
  "scope_id": 1,
  "reason": "CI list workspaces"
}
```

列表 / 撤销：

- `GET /api/v1/iam/applications/<id|app_key>/roles`
- `DELETE /api/v1/iam/applications/<id|app_key>/roles/:assignment_id`

应急 Direct Grant（勿默认）：`IAM_ALLOW_DIRECT_GRANT=1` + 原 grant API。

### 2.3 数据迁移（可选）

历史 Direct Grant 若把 `principal_id` 写成数字 app id：

```bash
# 见 backend/migrations/patch_application_principal_id_to_app_key.sql
```

不跑迁移也可：Checker 会同时匹配 id 与 app_key。

---

## 3. 调用面（`/api/v1/app/*`）

认证头（所有接口）：

```http
X-App-Key: <app_key>
X-App-Secret: <app_secret>
```

### 3.1 Whoami

```bash
curl -sS -H "X-App-Key: $APP_KEY" -H "X-App-Secret: $APP_SECRET" \
  "$API/api/v1/app/whoami"
```

期望：**200**，`principal_type=APPLICATION`，`principal_id=<app_key>`，`auth_org_id` 为应用所属 org。

### 3.2 权限检查

```bash
curl -sS -X POST -H "X-App-Key: $APP_KEY" -H "X-App-Secret: $APP_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"resource_type":"WORKSPACES","scope_type":"ORGANIZATION","scope_id":"1","required_level":"READ"}' \
  "$API/api/v1/app/permissions/check"
```

### 3.3 列出 Workspace

```bash
curl -sS -H "X-App-Key: $APP_KEY" -H "X-App-Secret: $APP_SECRET" \
  "$API/api/v1/app/workspaces"
```

叠层：

1. 有效 App 密钥  
2. 组织 `WORKSPACES` **READ** grant  
3. 仅返回 **auth_org** 下 workspace  
4. 若配置了 `workspace_tag_filter`，再按 **tags AND 匹配** 过滤  

### 3.4 单个 Workspace

```bash
curl -sS -H "X-App-Key: $APP_KEY" -H "X-App-Secret: $APP_SECRET" \
  "$API/api/v1/app/workspaces/<workspace_id>"
```

| 场景 | 期望 |
|------|------|
| 有 grant + tag 匹配 + 同 org | **200** |
| 有 grant + **tag 不匹配** | **404**（防探测） |
| **无 grant** | **403** |
| 跨 org workspace | **404** |
| 错误密钥 | **401** |

---

## 4. 冒烟清单（手工 / CI）

```text
[ ] whoami 200，principal_id = app_key
[ ] 未授权 WORKSPACES：GET /app/workspaces → 403
[ ] 已授权、无 tag 过滤：list 含 org 内 WS
[ ] 设置 workspace_tag_filter={"env":"prod"}：list 仅 env=prod
[ ] get 匹配 tag 的 ws → 200；不匹配 → 404
[ ] 错误 secret → 401
```

自动化：

```bash
cd backend && go test ./internal/handlers/ -count=1 -run 'Application' -timeout 60s
# 手工/环境冒烟：
# APP_KEY=... APP_SECRET=... API=http://localhost:8080 bash docs/iam/scripts/app-principal-smoke.sh
```

---

## 5. 相关补丁 SQL

| 文件 | 用途 |
|------|------|
| `patch_applications_workspace_tag_filter.sql` | 列 `workspace_tag_filter` |
| `patch_application_principal_id_to_app_key.sql` | 存量 APPLICATION grant id→app_key |
| `patch_system_admin_iam_roles.sql` + `patch_admin_role_iam_policies.sql` | 超管/admin IAM（发版必须） |

---

## 6. 不要做

- 不要用 Application 密钥调 `/api/v1/agents/*`（那是 Pool Token）。  
- 不要期望 Application 挂 workspace 级 Direct Grant（模型限制：仅 ORGANIZATION）。细粒度用 **tag 过滤** + org grant。  
- 不要把 `app_secret` 提交进 git / 前端长期存储。
