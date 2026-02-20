# Terraform执行流程连调进度

> **文档版本**: v1.0  
> **创建日期**: 2025-10-12  
> **状态**: 进行中  
> **相关文档**: [15-terraform-execution-detail.md](./15-terraform-execution-detail.md), [17-resource-level-version-control.md](./17-resource-level-version-control.md)

## 📋 概述

本文档记录Terraform执行流程的连调进度，包括发现的问题、修复方案和测试结果。

## 🎯 连调目标

验证完整的Terraform执行流程：
1. **Plan任务流程** - Fetching → Init → Planning → Saving Plan
2. **Apply任务流程** - Fetching → Init → Restoring Plan → Applying → Saving State
3. **资源级别版本控制** - 从workspace_resources表聚合生成main.tf.json

##  已解决的问题

### 问题1: 资源CurrentVersion加载失败  已修复

**发现时间**: 2025-10-12 09:28

**问题描述**:
```
[WARN] ✗ Resource: AWS_tesr-ccd.aa has no CurrentVersion!
```

**根本原因**:
- 资源记录存在（workspace_resources表）
- CurrentVersionID字段有值（1, 2, 3, 4）
- 但是GORM的`Preload("CurrentVersion")`无法加载关联数据
- 手动查询也失败，错误：`sql: Scan error on column index 4, name "tf_code": unsupported Scan, storing driver.Value type []uint8 into type *map[string]interface {}`

**根本原因分析**:
PostgreSQL的JSONB字段返回`[]uint8`（字节数组），但模型定义为`map[string]interface{}`，GORM无法自动转换。

**修复方案**:

1. **修改模型定义**（backend/internal/models/workspace_resource.go）:
```go
// 修改前
type ResourceCodeVersion struct {
    TFCode    map[string]interface{} `gorm:"type:jsonb;not null"`
    Variables map[string]interface{} `gorm:"type:jsonb"`
}

// 修改后
type ResourceCodeVersion struct {
    TFCode    JSONB `gorm:"type:jsonb;not null"`
    Variables JSONB `gorm:"type:jsonb"`
}
```

2. **改用手动加载**（backend/services/terraform_executor.go）:
```go
// 在ExecutePlan和generateMainTFFromResources中
var resources []models.WorkspaceResource
s.db.Where("workspace_id = ? AND is_active = true", workspaceID).
    Find(&resources)

// 手动加载每个资源的CurrentVersion
for i := range resources {
    if resources[i].CurrentVersionID != nil {
        var version models.ResourceCodeVersion
        if err := s.db.First(&version, *resources[i].CurrentVersionID).Error; err == nil {
            resources[i].CurrentVersion = &version
        }
    }
}
```

3. **添加外键关系**:
```go
CurrentVersion *ResourceCodeVersion `gorm:"foreignKey:CurrentVersionID;references:ID"`
```

**验证结果**:
```
[DEBUG] Manually loading version ID=1 for resource AWS_tesr-ccd.aa
[DEBUG] ✓ Successfully loaded version 1
[INFO] ✓ Resource: AWS_tesr-ccd.aa (version: 1)
[INFO] ✓ Generated main.tf.json (X.X KB)  ← 不再是0 KB
```

### 问题2: Plugin Cache目录错误  已修复

**发现时间**: 2025-10-12 09:37

**问题描述**:
```
Error: The specified plugin cache dir /var/cache/terraform/plugins cannot be opened: 
stat /var/cache/terraform/plugins: no such file or directory
```

**根本原因**:
- 使用全局目录`/var/cache/terraform/plugins`
- 目录不存在且可能没有权限创建
- 在设置环境变量后才尝试创建目录（顺序错误）

**修复方案**:

将plugin cache改为工作目录下的临时目录：

```go
// 修改前
pluginCacheDir := "/var/cache/terraform/plugins"
os.MkdirAll(pluginCacheDir, 0755)  // 在设置环境变量后
cmd.Env = append(cmd.Env, fmt.Sprintf("TF_PLUGIN_CACHE_DIR=%s", pluginCacheDir))

// 修改后
pluginCacheDir := filepath.Join(workDir, ".terraform-plugin-cache")
if err := os.MkdirAll(pluginCacheDir, 0755); err != nil {
    logger.Warn("Failed to create plugin cache dir: %v", err)
    pluginCacheDir = ""  // 失败则不使用缓存
}

// 只有创建成功才设置环境变量
if pluginCacheDir != "" {
    cmd.Env = append(cmd.Env, fmt.Sprintf("TF_PLUGIN_CACHE_DIR=%s", pluginCacheDir))
}
```

**优势**:
1.  每个任务有独立的plugin cache
2.  随工作目录一起清理
3.  不需要全局目录权限
4.  失败不阻塞执行

**验证结果**:
```
[INFO] Executing: terraform init -no-color -upgrade
← 不再有plugin cache错误
```

## 🔄 当前工作目录结构

```
/tmp/iac-platform/workspaces/{workspace_id}/{task_id}/
├── main.tf.json                    # 从资源聚合生成 
├── provider.tf.json                # Provider配置 
├── variables.tf.json               # 变量定义 
├── variables.tfvars                # 变量赋值 
├── terraform.tfstate               # State文件（从数据库拉取）
├── plan.out                        # Plan输出文件 ⏳
├── .terraform/                     # Terraform初始化目录 
│   └── providers/                  # Provider插件 
└── .terraform-plugin-cache/        # 插件缓存  新增
```

## 📊 连调进度

### Phase 1: Plan任务流程 (75% 完成)

#### Fetching阶段  已完成
- [x] 创建工作目录
- [x] 获取Workspace配置
- [x] 获取资源列表（workspace_resources）
- [x] 手动加载资源的CurrentVersion
- [x] 获取变量列表（workspace_variables）
- [x] 获取Provider配置
- [x] 获取State版本
- [x] 生成4个配置文件（main.tf.json, provider.tf.json, variables.tf.json, variables.tfvars）
- [x] 准备State文件

#### Init阶段  已完成
- [x] 创建plugin cache目录
- [x] 设置环境变量（AWS region等）
- [x] 执行terraform init -upgrade
- [x] 实时流式输出
- [x] 错误处理

#### Planning阶段 ⏳ 待测试
- [ ] 执行terraform plan
- [ ] 实时流式输出
- [ ] 解析Plan输出
- [ ] 生成Plan JSON
- [ ] 统计资源变更（add/change/destroy）

#### Saving Plan阶段 ⏳ 待测试
- [ ] 读取plan.out文件
- [ ] 保存Plan二进制数据到数据库
- [ ] 保存Plan JSON到数据库
- [ ] 重试机制验证

### Phase 2: Apply任务流程 (0% 完成)

#### Restoring Plan阶段 ⏳ 待测试
- [ ] 查找关联的Plan任务
- [ ] 从数据库读取Plan数据
- [ ] 恢复plan.out文件到工作目录
- [ ] 验证Plan文件有效性

#### Applying阶段 ⏳ 待测试
- [ ] 执行terraform apply
- [ ] 使用数据库中的Plan文件
- [ ] 实时流式输出
- [ ] 提取terraform outputs

#### Saving State阶段 ⏳ 待测试
- [ ] 读取terraform.tfstate文件
- [ ] 解析State内容
- [ ] 计算checksum
- [ ] 备份到文件系统
- [ ] 保存到数据库（带重试）
- [ ] 自动锁定机制（失败时）

### Phase 3: 端到端测试 (0% 完成)

- [ ] 创建测试Workspace
- [ ] 添加测试资源
- [ ] 执行Plan任务
- [ ] 验证Plan输出
- [ ] 执行Apply任务
- [ ] 验证State保存
- [ ] 验证资源创建成功

## 🐛 待解决的问题

暂无

## 📝 测试记录

### 测试1: 资源加载和main.tf.json生成

**时间**: 2025-10-12 09:40

**测试步骤**:
1. Workspace ID: 10
2. 资源数量: 4个
3. 触发Plan任务

**测试结果**:  成功
- 资源成功加载
- CurrentVersion成功加载
- main.tf.json成功生成（大于0 KB）
- terraform init成功执行

**日志片段**:
```
[INFO] Fetching workspace resources from workspace_resources table...
[DEBUG] Manually loading version ID=1 for resource AWS_tesr-ccd.aa
[DEBUG] ✓ Successfully loaded version 1
[INFO] ✓ Resource: AWS_tesr-ccd.aa (version: 1)
[INFO] Total: 4 resources loaded
[INFO] ✓ Generated main.tf.json (X.X KB)
[INFO] Executing: terraform init -no-color -upgrade
```

### 测试2: Terraform Init

**时间**: 2025-10-12 09:40

**测试步骤**:
1. 使用工作目录下的plugin cache
2. 执行terraform init -upgrade

**测试结果**:  成功
- Plugin cache目录创建成功
- 不再有目录不存在的错误
- Terraform init成功完成

### 测试3: Plan任务完整流程

**时间**: 待测试

**测试步骤**:
1. 触发Plan任务
2. 观察所有阶段日志
3. 验证Plan数据保存

**测试结果**: ⏳ 待测试

### 测试4: Apply任务完整流程

**时间**: 待测试

**测试步骤**:
1. 基于成功的Plan任务
2. 触发Apply任务
3. 验证State保存

**测试结果**: ⏳ 待测试

## 🔧 代码修改记录

### 修改1: ResourceCodeVersion模型

**文件**: `backend/internal/models/workspace_resource.go`

**修改内容**:
```go
// 将TFCode和Variables字段类型从map[string]interface{}改为JSONB
TFCode    JSONB `gorm:"type:jsonb;not null" json:"tf_code"`
Variables JSONB `gorm:"type:jsonb" json:"variables"`

// 添加外键关系
CurrentVersion *ResourceCodeVersion `gorm:"foreignKey:CurrentVersionID;references:ID"`
```

**原因**: 修复PostgreSQL JSONB字段扫描错误

### 修改2: 资源加载逻辑

**文件**: `backend/services/terraform_executor.go`

**修改位置**: 
- ExecutePlan方法（第330-350行）
- generateMainTFFromResources方法（第1030-1055行）

**修改内容**:
```go
// 改用手动加载CurrentVersion
for i := range resources {
    if resources[i].CurrentVersionID != nil {
        var version models.ResourceCodeVersion
        if err := s.db.First(&version, *resources[i].CurrentVersionID).Error; err == nil {
            resources[i].CurrentVersion = &version
        }
    }
}
```

**原因**: GORM Preload无法正确加载关联数据

### 修改3: Plugin Cache目录

**文件**: `backend/services/terraform_executor.go`

**修改位置**:
- TerraformInit方法（第238-250行）
- TerraformInitWithLogging方法（第1180-1192行）

**修改内容**:
```go
// 从全局目录改为工作目录
pluginCacheDir := filepath.Join(workDir, ".terraform-plugin-cache")
if err := os.MkdirAll(pluginCacheDir, 0755); err != nil {
    logger.Warn("Failed to create plugin cache dir: %v", err)
    pluginCacheDir = ""
}

// 只有创建成功才设置环境变量
if pluginCacheDir != "" {
    cmd.Env = append(cmd.Env, fmt.Sprintf("TF_PLUGIN_CACHE_DIR=%s", pluginCacheDir))
}
```

**原因**: 
- 全局目录可能没有权限
- 工作目录下的缓存随任务一起清理
- 每个任务有独立的plugin cache

## 📈 进度统计

| 阶段 | 状态 | 完成度 | 备注 |
|------|------|--------|------|
| Fetching |  完成 | 100% | 资源加载、配置生成 |
| Init |  完成 | 100% | Terraform初始化 |
| Planning | ⏳ 待测试 | 0% | 执行terraform plan |
| Saving Plan | ⏳ 待测试 | 0% | 保存Plan数据 |
| Restoring Plan | ⏳ 待测试 | 0% | Apply任务恢复Plan |
| Applying | ⏳ 待测试 | 0% | 执行terraform apply |
| Saving State | ⏳ 待测试 | 0% | 保存State到数据库 |

**总体进度**: 28% (2/7 阶段完成)

## 🎯 下一步计划

### 立即执行
1. **测试Plan任务完整流程**
   - 触发Plan任务
   - 观察Planning阶段日志
   - 验证Plan数据保存
   - 检查plan.out和plan.json文件

2. **测试Apply任务完整流程**
   - 基于成功的Plan任务
   - 触发Apply任务
   - 验证Plan恢复
   - 验证Apply执行
   - 验证State保存

### 后续优化
3. **错误场景测试**
   - Plan失败场景
   - Apply失败场景
   - State保存失败场景
   - 网络超时场景

4. **性能优化**
   - 并发执行测试
   - 大型配置测试
   - 资源清理验证

## 📝 调试技巧

### 查看详细日志

在Fetching阶段，日志会显示：
```
[DEBUG] Resource ID=1, ResourceID=AWS_tesr-ccd.aa, CurrentVersionID=1
[DEBUG] Manually loading version ID=1 for resource AWS_tesr-ccd.aa
[DEBUG] ✓ Successfully loaded version 1
[DEBUG]   - CurrentVersion.TFCode: map[module:map[...]]
```

### 查看生成的文件

```bash
# 查看工作目录
ls -la /tmp/iac-platform/workspaces/10/27/

# 查看main.tf.json内容
cat /tmp/iac-platform/workspaces/10/27/main.tf.json | jq '.'

# 查看plugin cache
ls -la /tmp/iac-platform/workspaces/10/27/.terraform-plugin-cache/
```

### 查看数据库数据

```sql
-- 查看资源
SELECT r.id, r.resource_id, r.current_version_id, v.version, v.tf_code 
FROM workspace_resources r 
LEFT JOIN resource_code_versions v ON r.current_version_id = v.id 
WHERE r.workspace_id = 10;

-- 查看任务
SELECT id, task_type, status, stage, error_message 
FROM workspace_tasks 
WHERE workspace_id = 10 
ORDER BY created_at DESC 
LIMIT 5;
```

## 🔗 相关文档

- [15-terraform-execution-detail.md](./15-terraform-execution-detail.md) - 执行流程详细设计
- [17-resource-level-version-control.md](./17-resource-level-version-control.md) - 资源级别版本控制
- [22-logging-specification.md](./22-logging-specification.md) - 日志规范
- [23-detailed-logging-implementation.md](./23-detailed-logging-implementation.md) - 详细日志实现

## 📅 更新日志

### 2025-10-12 上午
-  修复资源CurrentVersion加载失败（JSONB类型问题）
-  修复Plugin Cache目录错误
-  完成Fetching和Init阶段验证
-  创建连调进度文档
-  添加module_source字段支持（Module管理功能增强）
- ⏳ 准备测试Planning阶段

### 2025-10-12 11:22 - Module Source字段支持
**功能**: 为Module添加独立的module_source字段

**修改内容**:
- 后端模型添加`ModuleSource`字段（保留原有`Source`字段）
- Controller层支持创建和更新`module_source`
- Service层添加`UpdateModuleFields`方法
- 前端CreateModule/ImportModule/EditModule页面支持
- ModuleDetail页面显示`module_source`
- TypeScript类型定义更新
- 数据库迁移脚本创建并执行

**字段说明**:
- `source`: 原有含义（保留，用于其他用途）
- `module_source`: Terraform Module的source地址，用于在main.tf.json中引用

**Git提交**: commit 09b0b25, 8d88656

### 2025-10-12 12:07 - 变量处理和JSONB扫描问题修复
**问题1**: HCL格式string值没有引号
- 现象：`dde = ee` 导致Terraform报错
- 原因：HCL格式直接使用值，没有判断string类型
- 修复：智能判断值类型，string类型自动添加引号

**问题2**: 敏感变量在TRACE日志中泄露
- 现象：`aa = "ee"` 出现在日志中
- 原因：TRACE级别直接打印variables.tfvars内容
- 修复：添加maskSensitiveVariables函数，日志打印时自动脱敏

**问题3**: WorkspaceTask的JSONB字段扫描错误
- 现象：GET /api/v1/workspaces/:id/tasks 返回500错误
- 原因：PlanJSON和Outputs字段类型为map[string]interface{}
- 修复：改为JSONB类型，与ResourceCodeVersion保持一致

**Git提交**: commit 6cb299e

### 2025-10-13 10:13-11:47 - 任务状态和日志系统完善  已完成

#### 问题1: 任务失败时页面自动刷新导致日志丢失
**现象**: 用户取消任务后，WebSocket日志被清空
**根本原因**: TerraformOutputViewer中取消任务成功后调用`window.location.reload()`
**修复方案**:
```typescript
// 修改前
window.location.reload();

// 修改后
setTaskStatus('canceled');
setShowCancelDialog(false);
```
**效果**: 取消任务后日志保留在页面上

#### 问题2: 任意流程失败时日志可能不保存
**现象**: 某些阶段失败时日志没有保存到数据库
**根本原因**: 只在部分失败点调用了`saveTaskFailure()`
**修复方案**: 在所有可能失败的地方添加日志保存
-  ExecutePlan: 8个失败点全部添加`saveTaskFailure()`
-  ExecuteApply: 12个失败点全部已有`saveTaskFailure()`
**效果**: 任何阶段失败都会保存完整日志

#### 问题3: Apply成功后状态显示不正确
**现象**: Apply成功后显示"Planned"而不是"Applied"
**根本原因**: Apply成功后使用`TaskStatusSuccess`，与Plan成功状态相同
**修复方案**:
1. 添加新状态：`TaskStatusApplied = "applied"`
2. ExecuteApply成功时使用新状态
3. 前端TaskStateBadge支持新状态显示
**效果**: 
- Plan成功 → 显示"Planned"
- Apply成功 → 显示"Applied"
- 任何失败 → 显示"Errored"

#### 问题4: 取消任务时日志未保存
**现象**: 运行中取消任务，页面刷新后显示"任务已取消，未生成日志"
**根本原因**: CancelTask只更新状态，没有保存OutputStreamManager中的缓冲日志
**修复方案**:
1. 添加`OutputStream.GetBufferedLogs()`方法
2. CancelTask时从OutputStreamManager获取日志并保存
**效果**: 
- 运行中取消 → 保存已执行的日志
- Pending状态取消 → 显示"任务在执行前被取消"

#### 问题5: Apply成功后Cancel按钮仍显示
**现象**: 任务已完成但Cancel按钮还在
**根本原因**: 判断条件缺少`applied`状态
**修复方案**: 添加`task.status !== 'applied'`检查
**效果**: Applied状态不显示Cancel按钮

#### 问题6: 状态进度条全是灰色
**现象**: Apply成功后所有阶段显示灰色而不是绿色
**根本原因**: StageProgress组件没有处理`applied`状态
**修复方案**: 添加`if (taskStatus === 'applied') { return completed }`
**效果**: Applied状态所有阶段显示绿色✓

#### 问题7: plan_and_apply任务缺少Plan阶段日志
**现象**: 只能看到Apply日志，看不到Plan日志
**根本原因**: SmartLogViewer只显示一个阶段的日志
**修复方案**: 
1. StageProgress添加`onViewModeChange`回调
2. TaskDetail管理`logViewMode`状态
3. SmartLogViewer接收`viewMode` prop
4. 状态栏箭头控制日志显示
**效果**: 点击状态栏左右箭头可切换Plan/Apply日志

**修改文件**:
- 后端: 4个文件（models, executor, output_stream, controller）
- 前端: 7个文件（TaskDetail, StageProgress, SmartLogViewer, TaskStateBadge, StageLogViewer, TerraformOutputViewer, CSS）

**Git提交**: 待提交

### 2025-10-13 11:52-12:02 - 资源查看页面JSON显示修复  已完成

#### 问题8: 资源查看页面JSON显示异常
**现象**: 
1. JSON显示为一行没有格式化
2. JSON字段出现`${jsonencode()}`包装（接口和编辑页面都没有）
3. 修复后页面白屏，控制台报错：`Uncaught TypeError: value.split is not a function`

**根本原因**:
1. **格式化问题**：使用`dangerouslySetInnerHTML`插入HTML，不保留换行符
2. **包装问题**：`filterEmptyValues`函数错误地为json类型字段添加terraform包装
3. **白屏问题**：将JSON字符串解析为对象后，FormField组件期望字符串但收到对象

**修复方案**:

1. **移除HTML高亮，使用标准pre/code标签**:
```typescript
// 修改前
<div dangerouslySetInnerHTML={{ __html: highlightJson(jsonString) }} />

// 修改后
<pre className={styles.jsonContent}>
  <code className={styles.jsonCode}>{jsonString}</code>
</pre>
```

2. **分离表单视图和JSON视图的数据准备**:
```typescript
// 表单视图：保持原始字符串（FormField需要）
const filterEmptyValues = (obj: any): any => {
  // 不解析json类型字段，保持字符串
  result[key] = value;  // 字符串
};

// JSON视图：解析json字段为对象（格式化需要）
const prepareJsonViewData = (obj: any): any => {
  if (fieldSchema.type === 'json' && typeof value === 'string') {
    result[key] = JSON.parse(value);  // 对象
  }
};
```

3. **分别使用不同的数据源**:
```typescript
const filteredValues = filterEmptyValues(values);      // 表单视图用
const jsonViewData = prepareJsonViewData(filteredValues); // JSON视图用

// 表单视图
<FormField value={filteredValues[key]} />  //  字符串

// JSON视图
JSON.stringify(jsonViewData, null, 2)  //  对象会被正确格式化
```

**修复效果**:
-  表单视图正常显示（FormField接收字符串）
-  JSON视图正确格式化（对象被展开）
-  没有`${jsonencode()}`包装
-  页面不再白屏

**修改文件**:
- `frontend/src/components/DynamicForm/FormPreview.tsx`

**Git提交**: 待提交

---

**下一步**: 测试Plan任务的Planning和Saving Plan阶段
