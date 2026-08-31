## v0.9.0-beta1

IAM 切到 Role-primary 并隔离租户:用户/团队/应用的 Direct Grant HTTP 写入返回 410,授权改走 Role API;应用密钥成为一等主体(组织级角色 + workspace tag 过滤);Workspace 列表、任务、AI/CMDB 及相关 API 绑定 `auth_org_id`;前端通过共享 `apiRequestPolicy` 发送 `org_id`,授权 UI 仅支持 Role。部署必须跑 `iac-migrate`(Compose migrate 服务 + Kubernetes backend initContainer),应用启动不再 AutoMigrate。`patch_*.sql` 仅作应急恢复,禁止与 `iac-migrate` 混用。本 tag 相对 v0.8.0 同时带上尚未正式发过的编辑器/AI/CMDB/设计系统/Workspaces 改版,细节见 `changelog-v0.8.1.md`。这是 beta,不是正式版 v0.9.0。

### 新增功能

#### Role-primary 授权

- **重构** Direct Grant 写入退役:用户/团队/应用主体的 Direct Grant HTTP 写入返回 **410**,授权统一走 Role API(`docs/iam/39-direct-grant-retirement.md`)
- **新增** Role 分配与反提权:`role_anti_escalation.go` 约束角色策略范围,禁止把超出调用者自身能力的权限写入自定义角色
- **新增** 角色组织归属:`iam_roles.org_id` + 唯一约束 `(org_id, name)`,系统角色与租户自定义角色分开
- **优化** Permission Checker 仍 dual-read 遗留 `*_permissions` 行,旧 Direct Grant 数据在迁移完成前继续生效,读路径不立刻切断

#### 应用密钥作为一等主体

- **新增** 应用密钥 principal:org-scoped Role + `applications.workspace_tag_filter`,按 workspace tag 收窄可见工作区
- **新增** `iam_application_roles` 表 + `uq_app_roles_identity`,应用与角色的绑定独立于用户/团队授权
- **新增** `application_principal_handler.go` / `application_principal_workspace.go`:应用身份解析、workspace 可见性与 tag 匹配(`workspace_tag_match.go`)
- **新增** `jwt_or_app_auth.go`:JWT 与应用密钥共用鉴权入口
- **重构** `principal_id` 改为 `varchar(64)`,应用密钥改写为 `app_key` 形态,不再占用过短的 numeric id

#### 租户隔离

- **新增** API 绑定 `auth_org_id`:Workspace 列表、任务、AI/CMDB、资源、变量、远端数据、Run Trigger 等按当前组织过滤,禁止跨租户读到邻居数据
- **新增** `workspace_list_access.go`:列表可见性按组织 + Role + tag filter 计算
- **新增** `org_binding.go`:handler 层统一解析并校验请求组织
- **新增** 租户约束 CHECK:自定义角色必须属于某个 org;`iac-migrate` v1/v5 会把种子里较松的 CHECK 收成 `(is_system = true OR is_active = false OR org_id > 0)`
- **优化** 技能、摘要、CMDB 查询等 AI 路径同样带租户范围(`ai_cmdb_scope.go`)

#### iac-migrate 必跑迁移

- **新增** `backend/cmd/migrate/main.go` + `backend/internal/migration/runner.go`:独立 `iac-migrate` 二进制,版本化执行 IAM schema/data
- **新增** Compose:`docker-compose.example.yml` 增加 migrate 服务,`entrypoint: /app/iac-migrate`,backend 依赖其完成后再启动
- **新增** Kubernetes:`manifests/base/deployment-backend.yaml` backend initContainer 执行 `/app/iac-migrate`
- **新增** Release 流水线构建并随镜像分发 `iac-migrate`(`.github/workflows/release.yml` / `Dockerfile.server`)
- **优化** 应用进程启动不做 AutoMigrate,schema 变更只走 `iac-migrate`

#### 前端 org_id 与 Role-only UI

- **新增** `apiRequestPolicy.ts`:共享请求策略自动附带 `org_id`,各 service 不再各自拼组织参数
- **重构** `GrantPermission.tsx` / 团队与应用授权页改为 Role-only,去掉 Direct Grant 写入入口
- **优化** `AuthProvider` / `ProtectedRoute` / IAM 侧栏随当前组织切换上下文
- **新增** `frontend/tests/apiRequestPolicy.test.ts` 覆盖 org_id 注入

#### 种子 SQL schema 对齐

- **优化** `manifests/db/init_seed_data.sql` 跟上 IAM schema:`applications.workspace_tag_filter`、`iam_roles.org_id` + `(org_id, name)` 唯一、`principal_id varchar(64)`、`iam_application_roles`、`chk_iam_roles_custom_org`
- **说明** 种子对齐的是 **schema**,不是 patch **data**:系统 `admin` 的全量 `IAM_*` 策略、`workspace_admin`、部分索引由 migrate v2/v3 写入。Compose 路径 seed → migrate → app 可用;只灌种子不跑 migrate 时 IAM 管理接口会 403

### 本 tag 同时包含(相对 v0.8.0,详见 changelog-v0.8.1.md)

以下内容此前写在 `changelog-v0.8.1.md`,从未打过 `v0.8.1` tag,随 `v0.9.0-beta1` 一起发出。完整条目不要在本文件重复,这里只列主题:

- **新增** Manifest 编辑器 Provider 类型补全:`post_init` 落库 `manifest_provider_schemas`,HashiCorp TextMate grammar,补全/跳转/诊断/Problems 面板/Quick Open
- **新增** Grok (xAI) Provider + Provider 级 Fallback 容灾 + 自定义 Capability 场景
- **新增** CMDB 搜索结果 AI 解读 + 相关性筛查(`cmdb_search_summary`)
- **新增** 前端设计系统 v3:Design Token 统一 + UI 基元 + Ant Design 主题
- **新增** Workspaces 列表 Scheme A + `ContextShell`/`TopBar` 上下文导航壳(commit `b618c51`)
- **新增** CMDB/Manifest 深链、发布版本多 Workspace 升级、任务状态标签抽取、Swagger/OpenAPI 与线上路由同步

### 优化改进

- **优化** 中间件 `iam_permission.go` 按组织解析权限,JWT / 团队 token / 应用密钥 / pool token 走同一套 org 上下文
- **优化** 组织生命周期:禁用/隔离租户时角色与授权一并收敛(migrate v5 对游离租户数据 quarantine)
- **优化** Team token 有效名唯一:`uq_team_tokens_active_name`;临时权限增加 `idx_temp_perms_task_user_id`
- **重构** 遗留 USER/TEAM Direct Grant 在 migrate v2 转成 Role;APPLICATION Direct Grant 在 migrate v4 转入 `iam_application_roles`
- **重构** `system_admin` 映射为 `admin@ORGANIZATION`,不再靠散落的 Direct Grant 撑系统管理员

### 升级说明

从 v0.8.0 升到本 beta **必须先跑 `iac-migrate`**,再启 backend。不要把 `backend/migrations/patch_*.sql` 手工灌进已跑过 migrate 的库。

1. 备份数据库
2. 部署包含 `iac-migrate` 的镜像/二进制
3. Compose:先起 migrate 服务(entrypoint `/app/iac-migrate`);Kubernetes:backend initContainer `/app/iac-migrate`
4. migrate 成功后再起 backend / 前端
5. 验证:能按 Role 给用户/团队/应用授权;Direct Grant 写入接口返回 410;Workspace/任务/CMDB 只看见当前组织数据

`iac-migrate` 版本(按 `runner.go` 顺序,不可跳过):

- `20260717_01` schema:`principal_id varchar(64)`、`workspace_tag_filter`、`iam_roles.org_id`、唯一 `(org_id, name)`、`iam_application_roles` + `uq_app_roles_identity`、租户 CHECK、app_key 改写
- `20260717_02` data:admin 角色补齐组织级 `IAM_*` 策略;`system_admin` → `admin@ORGANIZATION`;USER/TEAM Direct Grant → Role
- `20260717_03`:`workspace_admin`、`uq_team_tokens_active_name`、`idx_temp_perms_task_user_id`
- `20260717_04`:APPLICATION Direct Grant → `iam_application_roles`
- `20260717_05`:租户对账 / 隔离(quarantine)

### 已知限制

- Checker **仍 dual-read** 遗留 `*_permissions`,Direct Grant 清理未完成,所以这是 beta 而不是正式版 v0.9.0
- `patch_*.sql` 是应急恢复材料,`iam_patch_contract_test.go` 只断言两份 patch 字符串,不是 seed ↔ patch 契约测试
- 种子里仍有 11 条指向 `USER 00000000000000000001` 的孤儿 `org_permissions`,同时 `users` COPY 为空;Checker dual-read 对这些行有效,但新安装 setup 出来的用户 id 对不上,不是登录口子
- Docker Release 流水线对 `v*` tag 仍会打镜像版本号 **和** `:latest`(与以往 beta 行为一致)

### 修改文件

#### 迁移与部署

- `backend/cmd/migrate/main.go` — **新增** iac-migrate 入口
- `backend/internal/migration/runner.go` — **新增** 版本化 schema/data 迁移
- `backend/internal/migration/runner_test.go` — **新增**
- `docker-compose.example.yml` — migrate 服务,entrypoint `/app/iac-migrate`
- `manifests/base/deployment-backend.yaml` — backend initContainer 跑 iac-migrate
- `manifests/db/init_seed_data.sql` — IAM schema 与种子对齐
- `.github/workflows/release.yml` / `.github/dockerfiles/Dockerfile.server` / `backend/Dockerfile` — 构建并分发 iac-migrate
- `backend/migrations/patch_*.sql` — 应急恢复 SQL(不要与 iac-migrate 混用)
- `backend/migrations/iam_patch_contract_test.go` — **新增**

#### 后端 IAM / 租户

- `backend/internal/application/service/permission_checker.go` — Role-primary + dual-read 遗留 grants
- `backend/internal/application/service/role_anti_escalation.go` — **新增** 角色反提权
- `backend/internal/application/service/workspace_list_access.go` — **新增** 列表可见性
- `backend/internal/application/service/workspace_tag_match.go` — **新增** 应用 tag 过滤
- `backend/internal/application/service/app_principal_aliases.go` — **新增** 应用主体别名
- `backend/internal/handlers/role_handler.go` / `role_application_handler.go` — Role API 与应用绑角色
- `backend/internal/handlers/permission_handler.go` — Direct Grant 写入 410
- `backend/internal/handlers/application_principal_handler.go` / `application_principal_workspace.go` / `application_principal_id.go` — **新增**
- `backend/internal/handlers/org_binding.go` — **新增** 请求组织绑定
- `backend/internal/middleware/iam_permission.go` / `jwt_or_app_auth.go` — 组织上下文 + 应用密钥鉴权
- `backend/internal/domain/entity/application_role.go` — **新增**
- `backend/internal/router/router_iam.go` / `router_application.go` / `router_workspace.go` / `router_ai.go` / `router_cmdb.go` — 路由与守卫
- `backend/controllers/workspace_controller.go` / `workspace_task_controller.go` / `resource_controller.go` / `task_workspace_access.go` — 租户绑定
- `backend/services/workspace_service.go` / `cmdb_service.go` / `ai_cmdb_service.go` — 列表与 CMDB 按 org 过滤

#### 前端

- `frontend/src/services/apiRequestPolicy.ts` — **新增** 共享 `org_id` 请求策略
- `frontend/src/services/api.ts` / `iam.ts` / `authIdentity.ts` — 走 apiRequestPolicy
- `frontend/src/pages/admin/GrantPermission.tsx` — Role-only 授权 UI
- `frontend/src/pages/admin/ApplicationManagement.tsx` / `TeamDetail.tsx` / `TeamManagement.tsx` / `PermissionManagement.tsx` / `RoleManagement.tsx` — 去掉 Direct Grant 写入
- `frontend/src/components/AuthProvider.tsx` / `ProtectedRoute.tsx` / `Layout.tsx` / `IAMSidebar.tsx` — 组织上下文
- `frontend/tests/apiRequestPolicy.test.ts` — **新增**

#### 文档

- `docs/iam/32-iam-remediation-report.md` ~ `docs/iam/40-iam-remediation-status-report.md` — IAM 修复过程与 Direct Grant 退役说明
- `docs/iam/README.md` — 索引更新

IAM 提交完整文件列表见 commit `54bd90d`。编辑器/AI/CMDB/设计系统/Workspaces 改版的文件列表见 `changelog-v0.8.1.md`。

### 技术细节

#### Direct Grant 退役与 dual-read

- **问题**:历史上用户/团队/应用权限写在 `*_permissions` Direct Grant 表,和 Role 并行,租户边界不清晰,HTTP 上仍能直接写 grant
- **方案**:写路径对 USER/TEAM/APPLICATION 返回 410,只保留 Role API;Checker 读路径 dual-read 遗留行,避免 migrate 未跑完的环境立刻 403。数据面由 `iac-migrate` v2/v4 把 Direct Grant 收进 Role / `iam_application_roles`
- **效果**:新授权只能走 Role;旧行在 dual-read 期间仍生效。彻底删 Direct Grant 表不在本 beta 范围

#### iac-migrate 与 patch SQL 分工

- **问题**:应用启动 AutoMigrate 扛不住这次 IAM 结构+数据迁移;`patch_*.sql` 是按故障现场写的恢复脚本,顺序和幂等与 runner 不一致
- **方案**:独立 `iac-migrate`,版本记录在 runner 内。Compose / K8s 把 migrate 放在 backend 之前。`patch_*.sql` 留在 `backend/migrations/` 仅供应急,禁止和 iac-migrate 对着同一库各跑一遍
- **效果**:新安装 seed → migrate → app 可完成 admin 角色与 IAM_* 策略;只灌种子会缺 v2 数据

#### 租户 CHECK 与种子差异

- **问题**:种子 CHECK 为 `(is_system OR org_id > 0)`,runner 目标为 `(is_system = true OR is_active = false OR org_id > 0)`,以允许停用角色保留 `org_id = 0`
- **方案**:migrate v1/v5 改写约束,不要求把种子 CHECK 先写成最终式
- **效果**:新鲜安装跑完 migrate 后约束与运行时一致

#### 应用主体与 tag filter

- **问题**:应用密钥之前不是完整 principal,跨 workspace 授权只能靠散 grant,无法按 tag 收窄
- **方案**:`iam_application_roles` + `workspace_tag_filter`,列表与资源访问用 `workspace_tag_match` 过滤
- **效果**:应用只能看见匹配 tag(或未设 filter)的本组织 workspace
