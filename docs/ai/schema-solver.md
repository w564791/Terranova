# Schema Solver 设计文档

## 🎯 为什么需要 Schema 组装器

### 当前架构问题

```
用户自然语言
    ↓
AI 生成代码
    ↓
直接输出 Terraform json代码
    ↓
问题:
- AI 可能生成不符合 Schema 约束的代码
- AI 可能漏掉必填参数
- AI 可能用错参数类型
- AI 可能不知道参数间的依赖关系
- 绕过了精心设计的 Schema 校验
```

### 加入组装器后

```
用户自然语言
    ↓
AI 理解意图 → 生成参数建议
    ↓
Schema 组装器 (SchemaSolver) ⭐
    ├─ 校验参数完整性
    ├─ 验证参数约束 (互斥/依赖)
    ├─ 类型转换和格式化
    └─ 生成 AI 反馈（如有错误）
    ↓
✅ 标准化的 Terraform 代码
```

## 💡 核心价值

### 1. AI 降级为意图理解，不负责生成最终代码

```
传统方式 (不推荐):
AI: "我觉得应该这样写..."
→ instance_type = "t2.micro"  # ❌ 可能不符合企业规范

使用组装器:
AI: {"instance_type": "t2.micro"}
    ↓
Schema 组装器验证:
→ 发现 t2.micro 不在 enum 列表中
→ 生成反馈让 AI 修正
```

### 2. Schema 成为唯一的真实来源 (Single Source of Truth)

- AI 只使用 Module Skill 生成参数
- SchemaSolver 负责验证参数是否符合 Schema 约束
- 如果验证失败，通过反馈机制让 AI 修正

### 3. 解耦 AI 能力和业务规则

```
✅ AI 模型升级不影响业务逻辑
   - 换 GPT-5 也不用改 Schema
   - AI 只负责"理解",不负责"决策"

✅ Schema 演进不影响 AI
   - 加新参数约束,AI 无感知
   - 改默认值,AI 无感知

✅ 可以不用 AI
   - 用户手动填表单 → Schema 组装器
   - AI 生成建议 → Schema 组装器
   - 两条路径共享同一套规则
```

## 🏗️ 架构设计

```
用户输入
    ↓
┌─────────────────┐
│ AI Layer        │  负责: 意图理解 + 参数建议
│ (可替换/可关闭)  │  输出: JSON 参数
└─────────────────┘
    ↓
┌─────────────────┐
│ Schema Solver ⭐ │  负责: 校验 + 反馈
│ (核心/必需)      │  输出: 验证结果 + AI 反馈
└─────────────────┘
    ↓
┌─────────────────┐
│ Code Generator  │  负责: 生成 Terraform 代码
│ (模板引擎)       │  输出: HCL 代码
└─────────────────┘
```

## 📝 完整组装流程示例

### 用户输入

```
"我需要创建一个名为ken-test-2026的s3桶，桶里的文件30天后转入智能分层，
180天后深层归档，查询cmdb，添加bucket policy允许ec2 role只读，允许lambda role只写"
```

### 第一步：AI 生成参数建议

AI 使用 Module Skill 理解用户意图，生成参数建议：

```json
{
  "bucket": "ken-test-2026",
  "lifecycle_rule": [
    {
      "id": "intelligent-tiering",
      "enabled": true,
      "transition": [
        { "days": 30, "storage_class": "INTELLIGENT_TIERING" },
        { "days": 180, "storage_class": "DEEP_ARCHIVE" }
      ]
    }
  ],
  "attach_policy": true,
  "policy": "{\"Version\":\"2012-10-17\",\"Statement\":[...]}"
}
```

### 第二步：SchemaSolver 验证

SchemaSolver 加载 Module Schema（71 个字段定义），执行验证：

```
[SchemaSolver] 检测到 OpenAPI 3.x 格式
[SchemaSolver] 加载了 71 个字段定义
```

**验证步骤：**

1. **枚举值验证** - 检查 storage_class 是否在允许列表中
2. **类型验证** - 检查 bucket 是 string，lifecycle_rule 是 array
3. **字符串约束** - 检查 bucket 名称长度和格式
4. **数组约束** - 检查 lifecycle_rule 元素数量
5. **互斥条件** - 检查是否有冲突的参数
6. **依赖条件** - 检查 attach_policy=true 时 policy 是否存在
7. **必填字段** - 检查所有 required 字段是否存在

### 第三步：验证结果

**场景 A：验证通过**

```
[AICMDBSkillService] SchemaSolver 验证成功，应用了 0 条规则
```

直接使用 AI 生成的参数，生成 Terraform 代码。

**场景 B：验证失败**

如果 AI 给出的 storage_class 不在 enum 列表中：

```json
{
  "success": false,
  "need_ai_fix": true,
  "feedbacks": [
    {
      "type": "error",
      "action": "choose_from",
      "field": "lifecycle_rule[0].transition[0].storage_class",
      "message": "字段的值 'SMART_TIERING' 不在允许的选项中",
      "ai_prompt": "字段的值 'SMART_TIERING' 不在允许的选项中。请从以下选项中选择: [GLACIER, STANDARD_IA, ONEZONE_IA, INTELLIGENT_TIERING, DEEP_ARCHIVE, GLACIER_IR]"
    }
  ]
}
```

### 第四步：AI 修正（如果需要）

SchemaSolver 生成 AI 指令，让 AI 修正参数：

```
Schema 验证发现以下问题需要你处理：

🚨 错误（必须修复）：

1. [lifecycle_rule[0].transition[0].storage_class] 
   字段的值 'SMART_TIERING' 不在允许的选项中。
   请从以下选项中选择: [GLACIER, STANDARD_IA, ONEZONE_IA, INTELLIGENT_TIERING, DEEP_ARCHIVE, GLACIER_IR]

请提供修正后的参数，解决所有错误。
```

AI 修正后重新提交，SchemaSolver 再次验证，直到通过或达到最大重试次数。

### 第五步：生成 Terraform 代码

验证通过后，使用最终参数生成 Terraform 代码：

```hcl
module "s3_bucket" {
  source = "terraform-aws-modules/s3-bucket/aws"
  
  bucket = "ken-test-2026"
  
  lifecycle_rule = [
    {
      id      = "intelligent-tiering"
      enabled = true
      transition = [
        { days = 30, storage_class = "INTELLIGENT_TIERING" },
        { days = 180, storage_class = "DEEP_ARCHIVE" }
      ]
    }
  ]
  
  attach_policy = true
  policy        = jsonencode({...})
}
```

## 📋 SchemaSolver 功能清单

### 验证功能

| 功能 | 说明 |
|------|------|
| 枚举值验证 | 检查值是否在 enum/options 列表中 |
| 类型验证 | 支持 string, number, integer, boolean, array, object |
| 字符串约束 | minLength, maxLength, pattern (正则) |
| 数值约束 | minimum, maximum |
| 数组约束 | minItems, maxItems |
| 必填字段检查 | 检查 required 字段是否存在 |

### 参数关联

| 功能 | 说明 |
|------|------|
| 互斥条件 | conflicts_with - 两个字段不能同时存在 |
| 依赖条件 | depends_on - 字段 A 存在时，字段 B 必须存在 |
| 隐含规则 | implies - 当 A=true 时，自动设置 B=true |
| 条件规则 | conditional if/else - 复杂的条件逻辑 |

### AI 反馈机制

| 反馈类型 | 说明 |
|----------|------|
| error | 必须修复的错误 |
| warning | 建议修复的警告 |
| suggestion | 可选的建议 |

##  重要设计决策

### SchemaSolver 不做的事情

1. **不自动填充默认值** - 这应该由 AI 来决定
2. **不自动填充空值** - AI 给什么就验证什么
3. **不从 CMDB/Output 自动获取数据** - AI 已经通过 CMDB Skill 获取了数据

### 为什么这样设计？

- AI 对参数有完全的控制权
- SchemaSolver 只负责验证和反馈
- 如果 AI 漏掉了必填参数，通过反馈机制让 AI 补充

## 📁 相关文件

- `backend/services/schema_solver.go` - SchemaSolver 核心实现
- `backend/services/schema_solver_loop.go` - AI 反馈循环实现
- `backend/services/ai_cmdb_skill_service_sse.go` - 集成 SchemaSolver 的 AI 服务

## 🔍 日志关键字

```bash
# 查看 SchemaSolver 工作状态
grep -E "(SchemaSolver|加载了|应用了|验证成功|验证失败)" /tmp/platform.log | tail -20
```

### 实际日志证据（2026-02-01 20:34:27）

```
2026/02/01 20:34:27 [SchemaSolver]  OpenAPI 3.x
2026/02/01 20:34:27 [SchemaSolver] 加载了 71 个字段定义
2026/02/01 20:34:27 [AICMDBSkillService] SchemaSolver 验证成功，应用了 0 条规则
```

**日志逐行解读：**

| 日志 | 含义 | 说明 |
|------|------|------|
| `OpenAPI 3.x` | Schema 格式检测 | 正确识别为 OpenAPI 3.x 格式，字段定义在 `components.schemas.ModuleInput.properties` |
| `加载了 71 个字段定义` | Schema 解析成功 | S3 bucket module 有 71 个可配置字段（bucket、lifecycle_rule、policy 等） |
| `应用了 0 条规则` | **关键指标** | SchemaSolver 只做验证，不自动填充默认值。之前是 39 条（自动填充），现在是 0 条 |

**"验证成功，应用了 0 条规则" 的意义：**

✅ **AI 提供的参数格式正确：**
- AI 提供的所有参数都通过了 Schema 验证
- 枚举值正确（在 enum 列表中）
- 类型正确（string/number/array 等）
- 没有互斥冲突
- 依赖关系满足

 **注意：这不代表 AI 提供了所有必要的参数！**
- SchemaSolver 只验证 AI 提供的参数
- 如果 Schema 没有 required 字段（如 S3 bucket），不会因为缺少字段而报错
- S3 bucket 的 71 个字段都是可选的（`required: []`）

**"应用了 0 条规则" 的含义：**
- 没有触发隐含规则（implies）
- 没有触发条件规则（conditional）
- 不再自动填充默认值

## 🧪 健壮性测试验证

### 测试场景

修改 Schema 中 `versioning` 字段的类型从 `object` 改为 `boolean`，但不更新 Module Skill。
AI 会根据旧的 Module Skill 生成 `versioning` 为 map 类型。

### 测试结果（2026-02-01 20:47:58）

```
2026/02/01 20:47:58 [SchemaSolver] 检测到 OpenAPI 3.x 格式
2026/02/01 20:47:58 [SchemaSolver] 加载了 71 个字段定义
2026/02/01 20:47:58 [AICMDBSkillService] SchemaSolver 验证失败，需要 AI 修正
1. [versioning] 字段 'versioning' 应该是 'boolean' 类型，但你提供的是 'object' 类型，值为 'map[enabled:true]'。请将此值转换为正确的类型。
2026/02/01 20:47:58 [SchemaSolver] 检测到 OpenAPI 3.x 格式
2026/02/01 20:47:58 [SchemaSolver] 加载了 71 个字段定义
2026/02/01 20:48:08 [AIFeedbackLoop] 迭代 2: 验证成功
```

### 验证结论

| 测试项 | 结果 | 说明 |
|--------|------|------|
| 类型不匹配检测 | ✅ 通过 | SchemaSolver 检测到 `versioning` 期望 `boolean`，但 AI 给的是 `object` |
| 错误反馈生成 | ✅ 通过 | 生成了清晰的错误信息，告诉 AI 需要转换类型 |
| AI 反馈循环 | ✅ 通过 | 第 2 次迭代验证成功，AI 自动修正了参数 |

**结论：SchemaSolver 健壮性足够好！**
- 能检测 Schema 和 AI 输出之间的类型不匹配
- AI 反馈循环能让 AI 自动修正错误
- 即使 Module Skill 过期，SchemaSolver 也能保证最终输出符合 Schema 约束

## 🎯 关键点总结

1. ✅ **一定要加 Schema 组装器** - 这是质量保证的最后一道防线
2. ✅ **AI 只做建议，不做决策**
3. ✅ **Schema 是唯一真相来源**
4. ✅ **用户手填表单和 AI 生成共用同一个组装器**
5. ✅ **组装过程可观测、可审计**
6. ✅ **健壮性验证通过** - 即使 Module Skill 过期，SchemaSolver 也能保证输出正确
