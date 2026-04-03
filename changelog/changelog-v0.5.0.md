## v0.5.0

Variable Set -- 组织级变量集管理，支持 Global/Specific 作用域，软连接 attach 到 Workspace，优先级解析，版本控制，Terraform 执行路径集成。

### Features

#### Variable Set 核心功能

- **新增** `variable_sets`、`varset_variables`、`varset_assignments` 三张表，支持变量集的完整生命周期管理
- **新增** `VARIABLE_SETS` IAM 资源类型（ORGANIZATION scope），READ/WRITE/ADMIN 三级权限分控
- **新增** Variable Set CRUD API（`/api/v1/variable-sets`），支持 Global/Specific 两种作用域模式
- **新增** Varset Variable CRUD API，支持 terraform/environment 变量类型、string/hcl 值格式、sensitive 加密
- **新增** Assignment API，支持将 Specific 变量集 attach 到 Project 和 Workspace

#### 变量版本控制

- **新增** varset_variables 版本控制：更新变量创建新版本行，旧版本保留供任务快照引用
- **约束** 变量 key、variable_type、value_format 创建后不可修改；sensitive 只能 false->true
- **约束** 同一变量集内 key 唯一（不区分 variable_type）

#### 变量优先级解析

- **新增** `VariableResolutionService`，支持 Display（前端展示）和 Resolve（执行器）双模式
- **优先级** Workspace 变量 > Workspace 级 VarSet > Project 级 VarSet > Global VarSet
- **同层级** 后 attach 的变量集覆盖先 attach 的；Global 按创建时间排序

#### Terraform 执行路径集成

- **改造** `LocalDataAccessor.GetWorkspaceVariables()` 使用 VariableResolutionService 合并变量集变量
- **改造** `createTaskSnapshot()` 通过 ResolutionService 获取合并变量，varset 变量也纳入快照（variable_id + version）
- **改造** Agent/K8s `GetTaskData` API 返回合并后的变量（含变量集）
- **改造** Agent `GetPlanTask` 快照重建时 fallback 查询 varset_variables 表
- **新增** 任务执行日志打印已加载的 terraform/environment 变量 key（INFO 级别，不打印 value）

#### 前端

- **新增** Variable Sets 列表页（`/variable-sets`），支持创建、编辑、删除变量集
- **新增** Variable Set 详情页（`/variable-sets/:varsetId`），Variables Tab + Assignments Tab
- **新增** Sidebar 导航 "Variable Sets" 项（Workspaces 下方），带 VARIABLE_SETS 权限检查
- **新增** Assignment 选择器：下拉选择 Project/Workspace（从 API 加载列表）
- **改进** Workspace 变量页面增加 "Variable Set Variables" 区域，展示来自变量集的变量
- **改进** 被覆盖的变量显示 "Overridden {key}" 超链接，点击跳转到覆盖方变量
- **改进** 创建变量集后自动跳转到详情页，方便立即添加变量和分配

### Bug Fixes

- **修复** Assignment 删除未校验 varset 归属，可跨变量集删除 assignment 的授权漏洞
- **修复** `VarsetVariableService.Update()` 使用 map-based Updates 绕过 GORM hooks 导致 sensitive 变量明文存储
- **修复** 错误信息直接暴露给客户端（包含 DB 细节），改为分类返回 400/404/500 + 日志记录
- **修复** List 接口 N+1 查询（每个 varset 2 次 count 查询），改为批量 GROUP BY
- **修复** Resolution Service N+1 查询（每个 varset 2 次查询），改为批量加载
- **修复** Toast 通知不显示：useToast 误导入 hooks/useToast（本地状态）而非 contexts/ToastContext（全局渲染）
- **修复** 变量操作按钮显示不完整，改为直接显示"编辑"/"删除"按钮
- **修复** Workspace 列表 API 响应数据解析错误（未正确处理 `{code, data: {items}}` 格式）
- **修复** Effective variables 默认折叠，改为页面加载时自动显示并在变量 CRUD 后刷新
- **修复** Overridden 超链接跳转失效（workspace 变量行缺少 anchor ID）
- **修复** 删除确认弹窗不符合 ConfirmDialog 规范，从 toast 风格改为 type="danger" 弹窗
- **修复** 数据库残留旧索引 idx_varset_key_type，migration 增加 DROP 保证幂等
- **修复** snapshot_variables 在 GetTask 查询中被 Omit，导致任务详情无法展示快照变量

### Tests

- **新增** 9 个集成测试，覆盖版本控制、键唯一性、sensitive 约束、全局解析、覆盖优先级、版本传播
- **新增** TestMain 自动管理测试库生命周期（创建 iac_platform_test -> 初始化 schema -> 执行测试 -> 清理）

### Database

- **新表** `variable_sets` -- 变量集元数据（varset_id, name, scope, is_deleted）
- **新表** `varset_variables` -- 变量集内变量（variable_id, key, value, version, sensitive, 加密存储）
- **新表** `varset_assignments` -- 软连接分配（scope_type, project_id/workspace_id, attached_at）
- **索引** partial unique `(varset_id, key, version) WHERE is_deleted=false`、partial unique `(name) WHERE is_deleted=false`
- **约束** CHECK scope_type 互斥、FK CASCADE 到 variable_sets/projects/workspaces
