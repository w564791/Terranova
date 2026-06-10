# query_resource_code_diff 支持 Manifest 资源变更

## 背景

AI Plan Summary 的 `query_resource_code_diff` 工具目前仅支持 workspace UI module 资源的代码变更分析。随着 manifest 编排的广泛使用，manifest 管理的 workspace 在 plan 时同样需要代码 diff 来分析 `after_unknown` 字段，但该工具无法处理。

本文档分析差异、提出扩展方案。

---

## 一、现状分析

### 1.1 当前工具工作流程

**入口：** `backend/services/ai_summary_tools.go` — `QueryResourceCodeDiffTool.Execute()`

**输入：** `(workspace_id, resource_id)`，其中 `resource_id` 为 CMDB 格式（如 `AWS_s3-bucket.xxx`）

**输出：** 字段级 JSON diff — `[{path, type, old, new}]`

**数据链路：**

```
workspace_resources (resource_id → ID)
  → resource_code_versions (resource_id, is_latest=true → 当前代码 TFCode JSONB)
  → workspace_task_resource_changes.applied_code_version (快速路径: 上次 apply 时的版本号)
    ↳ fallback: workspace_tasks.snapshot_resource_versions JSONB
  → resource_code_versions (resource_id, version=applied → 上次 apply 的代码)
  → computeJSONDiff(appliedTFCode, currentTFCode) → [{path, type, old, new}]
```

**调用方：** 仅 Plan 阶段注册（`ai_summary_service.go:169`），Apply 阶段不注册。

**下游消费：** `ai_summary_service.go:hasCMDBNotFound()` 读取 tool_calls 中 `query_resource_code_diff` 的返回结果，判断是否推断 `service_disruption` 风险因子。

### 1.2 三个硬约束导致不支持 manifest

| 约束 | 说明 |
|---|---|
| **代码存储不同** | module 资源代码在 `resource_code_versions.tf_code`（JSONB，按资源逐条存储）；manifest 资源代码在 `manifest_files.content`（bytea，按文件存储，一个文件可含多个 resource/module 块） |
| **无版本记录** | manifest 资源的 `WorkspaceResource.CurrentVersionID` 为 NULL，没有 `ResourceCodeVersion` 行。`workspace_output_controller.go:881` 注释："manifest 资源无 tf_code" |
| **backfill 路径不通** | `backfillAppliedCodeVersion()` 的 SQL JOIN 条件 `rc.module_address = 'module.' \|\| REPLACE(wr.resource_id, '.', '_')` 仅适用于 UI module 资源 |

### 1.3 Manifest 代码存储架构

```
manifests (mf-xxx)
  → manifest_versions (mfv-xxx, version=vX.Y.Z)
    → manifest_files (version_id=mfv-xxx, path="main.tf", content=bytea)
  → manifest_deployments (mfd-xxx, version_id=mfv-xxx, workspace_id=ws-xxx)
    → manifest_deployment_resources (node_id, resource_id)

workspace.manifest_deployment_id → mfd-xxx (软链接)
workspace.manifest_active_tag   → "v1.2.0"
```

**版本变更流程：**

- `install`：首次部署，写 `workspace_resources`（含 `ManifestDeploymentID`），更新 `workspace.manifest_active_tag`
- `upgrade`：切换版本，reconcile `workspace_resources`（增删），更新 `manifest_deployments.version_id` + `workspace.manifest_active_tag`
- `plan/apply`：`writeManifestFiles()` 拉 `manifest_files` 全量落盘

### 1.4 核心差异对比

| 维度 | Module 资源 | Manifest 资源 |
|---|---|---|
| 代码位置 | `resource_code_versions.tf_code` (JSONB/资源) | `manifest_files.content` (bytea/文件) |
| 版本粒度 | 每个资源独立版本号 (int) | 整个 manifest 共享版本号 (vX.Y.Z) |
| 资源→代码映射 | 1:1 (一个资源 = 一个 TFCode JSON) | N:M (一个 .tf 文件可含多个资源/模块) |
| 版本回溯 | `applied_code_version` 列 / snapshot JSONB | **无**（deployment 只存当前版本） |
| diff 方式 | JSON 字段级递归对比 | HCL 文本级对比 |

---

## 二、方案设计

### 2.1 自动感知：manifest vs module 资源

**已有现成判据，无需额外传参。**

`QueryResourceCodeDiffTool` 第一步查 `workspace_resources`，返回的 `WorkspaceResource` 已含 `ManifestDeploymentID` 字段：

```go
// workspace_resource.go L26
ManifestDeploymentID *string `gorm:"type:varchar(36);index:..."`
```

分流逻辑：

- `resource.ManifestDeploymentID != nil` → **走 manifest 分支**
- `resource.ManifestDeploymentID == nil` → **走现有 module 分支（不变）**

对 AI agent 完全透明，调用签名不变：`query_resource_code_diff(workspace_id, resource_id)`。

这与 `terraform_executor.go:212-216` 的 `workspaceUsesManifest()` 判据一致。

### 2.2 版本追踪层（新增）

**问题：** 当前无法回溯"上次 apply 时的 manifest 版本"。`manifest_deployments.version_id` 只保留当前版本，upgrade 后旧版本信息丢失。

**方案：** 在 `workspace_tasks` 表新增 `snapshot_manifest_version_id` 列。

```sql
ALTER TABLE workspace_tasks ADD COLUMN snapshot_manifest_version_id VARCHAR(36);
```

在 `CreateResourceVersionSnapshot()`（`terraform_executor.go:3848`）中，若 workspace 使用 manifest，则快照当前 `manifest_deployments.version_id`：

```go
// 在 CreateResourceVersionSnapshot 中追加
if workspace.ManifestDeploymentID != nil && *workspace.ManifestDeploymentID != "" {
    var dep models.ManifestDeployment
    s.db.Select("version_id").Where("id = ?", *workspace.ManifestDeploymentID).First(&dep)
    task.SnapshotManifestVersionID = &dep.VersionID
}
```

**回溯逻辑：** 找上次 apply 成功的 task → 取其 `snapshot_manifest_version_id` → 即为"上次 apply 时的版本"。

### 2.3 QueryResourceCodeDiffTool manifest 分支

在现有 `Execute()` 方法中，查到 `WorkspaceResource` 后根据 `ManifestDeploymentID` 分流：

```
1. 查 workspace_resources → 拿到 resource
2. 若 resource.ManifestDeploymentID != nil → 走 manifest 分支:
   a. 查 workspace → 拿到 manifest_deployment_id + manifest_subpath
   b. 查 manifest_deployments → 拿到 manifest_id + 当前 version_id
   c. 查上次 apply 成功的 task → 拿到 snapshot_manifest_version_id (旧版本)
   d. 若新旧版本相同 → return {has_changes: false}
   e. 拉两个版本的 manifest_files（.tf 文件）
   f. 用 ParseManifestResources 定位 resource 所在的文件
   g. 从两个版本的文件内容中提取该 resource/module 的 HCL block 文本
   h. 对比并返回 diff
3. 若 resource.ManifestDeploymentID == nil → 走现有 module 分支（不变）
```

### 2.4 HCL Block 提取与 Diff 计算

manifest 的 diff 不能复用 `computeJSONDiff`（输入是 HCL 文本，不是 JSONB）。

**提取策略：**

1. 用现有 `ParseManifestResources()` 确定 resource 在哪个 `.tf` 文件中
2. 解析该文件的 HCL，提取对应 resource/module block 的源码区间（`block.DefRange`）
3. 从原始文件内容中切出该 block 的 HCL 文本
4. 对比新旧版本的 HCL block 文本

**返回格式：**

```json
{
  "found": true,
  "has_changes": true,
  "source_type": "manifest",
  "manifest_version_applied": "v1.0.0",
  "manifest_version_current": "v1.1.0",
  "applied_task_id": 123,
  "file": "main.tf",
  "block_type": "resource",
  "block_address": "aws_s3_bucket.my_bucket",
  "old_hcl": "resource \"aws_s3_bucket\" \"my_bucket\" {\n  bucket = \"old-name\"\n}",
  "new_hcl": "resource \"aws_s3_bucket\" \"my_bucket\" {\n  bucket = \"new-name\"\n}",
  "changed": true
}
```

对于 AI agent 来说，HCL 文本对比比 JSON 字段 diff 更直观——它本身就是处理 Terraform 代码的。

**新增 HCL block 提取函数：** `extractResourceBlock(content []byte, kind, typeName, name string) string`

使用 `hclparse.Parser` + `PartialContent` 提取指定 resource/module block 的完整源码文本。

### 2.5 Skill Prompt 更新

`skill/execute_summary/task/execute_summary_workflow.md` 第十四节需补充：

1. manifest 资源的 `query_resource_code_diff` 返回 HCL 文本对比而非 JSON 字段 diff
2. 返回格式差异说明（`source_type: "manifest"` 标识来源）
3. manifest 版本是整体版本（vX.Y.Z），两个版本间同一 resource 的 HCL block 对比

推断规则无需修改——manifest 资源的 `resource_id` 格式已经是 `aws_s3_bucket.my_bucket` 或 `module.network`，与 `workspace_resources` 存储一致。

### 2.6 下游消费适配

`ai_summary_service.go:hasCMDBNotFound()` 读取 tool_calls 判断 `found=false`。manifest 分支的返回结构需保持 `found` 字段一致，下游消费无需改动。

`backfillAppliedCodeVersion()` 的 JOIN 条件天然不匹配 manifest 资源（无 `resource_code_versions` 行），不会产生错误数据，无需修改。

---

## 三、涉及文件清单

| 文件 | 改动类型 | 说明 |
|---|---|---|
| `backend/services/ai_summary_tools.go` | **修改** | `QueryResourceCodeDiffTool.Execute` 新增 manifest 分流 + HCL block 提取 + diff |
| `backend/services/terraform_executor.go` | **修改** | `CreateResourceVersionSnapshot` 追加 manifest version 快照 |
| `backend/internal/models/workspace.go` | **修改** | `WorkspaceTask` 新增 `SnapshotManifestVersionID` 字段 |
| `backend/migrations/` | **新增** | `add_snapshot_manifest_version_id.sql` |
| `skill/execute_summary/task/execute_summary_workflow.md` | **修改** | 第十四节补充 manifest 场景说明 |

**无需改动：**

| 文件 | 原因 |
|---|---|
| `ai_summary_service.go` | `hasCMDBNotFound` 等下游逻辑通过 `found` 字段判断，兼容 |
| `terraform_executor.go:backfillAppliedCodeVersion` | JOIN 条件天然跳过 manifest 资源 |
| 前端 | code diff 工具结果仅在 AI agent 内部消费，不直接展示 |

---

## 四、边界情况

| 场景 | 处理方式 |
|---|---|
| manifest 首次 plan，无 apply 历史 | 返回 `{found: true, message: "no apply history found", current_version: "vX.Y.Z"}` |
| upgrade 后 plan，新旧版本相同（无实质变更） | 返回 `{has_changes: false}` |
| resource 在新版本中被删除 | HCL block 提取不到 → 返回 `{block_removed: true}` |
| resource 在新版本中新增（旧版本无此 block） | HCL block 提取不到 → 返回 `{block_added: true}` |
| resource 跨文件移动 | 按文件分别提取，标记 `{file_changed: true}` |
| subpath 场景 | 仅提取 subpath 直接下层的 .tf 文件（复用 `shouldParseForResources` 规则） |
| ExternalFiles（Run 草稿预览） | task 携带 ExternalFiles 时，代码来自草稿而非已发布版本，返回 `{source_type: "external_files"}` 或跳过 |
