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

## ⚠️ 重要设计决策

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
grep -E "(SchemaSolver|加载了|应用了)" /tmp/platform.log | tail -20

# 预期输出
[SchemaSolver] 检测到 OpenAPI 3.x 格式
[SchemaSolver] 加载了 71 个字段定义
[AICMDBSkillService] SchemaSolver 验证成功，应用了 0 条规则
```

## 🎯 关键点总结

1. ✅ **一定要加 Schema 组装器** - 这是质量保证的最后一道防线
2. ✅ **AI 只做建议，不做决策**
3. ✅ **Schema 是唯一真相来源**
4. ✅ **用户手填表单和 AI 生成共用同一个组装器**
5. ✅ **组装过程可观测、可审计**