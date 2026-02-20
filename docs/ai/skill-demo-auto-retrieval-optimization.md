# Skill系统优化方案：AI自主决策Demo检索

## 文档信息

| 项目 | 内容 |
|------|------|
| 创建日期 | 2026-01-30 |
| 状态 | 待实施 |
| 优先级 | 中 |
| 预估工作量 | 3-5天 |

---

## 1. 背景与问题

### 1.1 当前架构

```
用户描述 → Task Skill加载 → Module Skill加载 → LLM生成候选参数
```

**Module Skill组成**：
- `schema_generated_content`：根据OpenAPI Schema自动生成的配置知识
- `custom_content`：用户自定义补充内容（当前为空）

**Demo库**：
- 独立的`module_demos`和`module_demo_versions`表
- 存储完整的配置示例（如minimal、complete等）
- 包含复杂字段的真实配置（lifecycle_rule、cors_rule、intelligent_tiering等）

### 1.2 问题描述

1. **Demo与Skill分离**：Demo库中有丰富的配置示例，但资源生成流程没有利用
2. **Tag强关联**：当前Skill系统通过tag进行关联，缺乏灵活性
3. **复杂配置生成困难**：对于复杂嵌套结构（如S3的intelligent_tiering），AI可能无法正确生成

### 1.3 期望目标

- AI能够**自主决策**是否需要参考Demo
- 根据用户需求**动态检索**相关Demo片段
- 提高复杂配置的生成准确性

---

## 2. 现状分析

### 2.1 S3模块Demo数据示例

| Demo名称 | 描述 | 包含的复杂字段 |
|----------|------|----------------|
| `minimal` | 最小配置 | bucket, tags |
| `complete` | 完整配置 | lifecycle_rule, cors_rule, intelligent_tiering, website, server_side_encryption_configuration, object_lock_configuration, metric_configuration |

### 2.2 当前Module Skill内容

S3的`schema_generated_content`包含：
- 模块概述
- 参数约束（必填字段、可选字段、关联关系）
- 复杂字段结构（lifecycle_rule的展开）
- 配置分组
- 最佳实践
- 输出属性

**缺失**：配置示例（需要从Demo库获取）

### 2.3 资源生成效果

| 用户需求 | 生成效果 |
|----------|----------|
| "创建一个S3桶，30天后转智能分层，90天后深层归档" | ✅ 正确生成lifecycle_rule |
| "创建一个支持CORS的S3桶" | ❓ 可能需要Demo参考 |
| "创建一个完整配置的S3桶" | ❓ 需要Demo参考 |

---

## 3. 优化方案

### 3.1 方案概述

```
用户描述 
    ↓
AI分析需求复杂度（第一阶段）
    ↓
决定是否需要Demo参考
    ↓
[如需要] 检索相关Demo片段
    ↓
生成配置（第二阶段）
```

### 3.2 方案对比

| 方案 | 描述 | 优点 | 缺点 | 推荐度 |
|------|------|------|------|--------|
| 方案1：两阶段生成 | 先分析需求，再生成配置 | 实现简单，决策明确 | 增加一次LLM调用 | ⭐⭐⭐⭐⭐ |
| 方案2：工具调用 | AI通过@request-demo请求示例 | 灵活，AI自主决策 | 需要修改prompt解析 | ⭐⭐⭐⭐ |
| 方案3：语义检索 | 向量化Demo，语义匹配 | 最智能 | 实现复杂，需要向量数据库 | ⭐⭐⭐ |

### 3.3 推荐方案：两阶段生成

#### 第一阶段：需求分析

**输入**：
- 用户描述
- Module Skill（字段列表）

**输出**：
```json
{
  "complexity": "simple | standard | complex",
  "required_features": ["lifecycle_rule", "cors_rule"],
  "need_demo_reference": true,
  "demo_search_keywords": ["生命周期", "CORS"]
}
```

**判断规则**：
| 复杂度 | 条件 | 是否需要Demo |
|--------|------|--------------|
| simple | 只涉及基础字段（bucket, tags等） | 否 |
| standard | 涉及1-2个复杂字段 | 可选 |
| complex | 涉及3个以上复杂字段或用户明确要求"完整配置" | 是 |

#### 第二阶段：配置生成

**输入**：
- 用户描述
- Module Skill
- CMDB数据
- [可选] Demo片段

**输出**：
- 生成的配置JSON

---

## 4. 详细设计

### 4.1 数据库设计

无需新增表，利用现有的`module_demos`和`module_demo_versions`表。

可选优化：为Demo添加特性标签字段
```sql
ALTER TABLE module_demos ADD COLUMN feature_tags TEXT[];
-- 示例：['lifecycle_rule', 'cors_rule', 'intelligent_tiering']
```

### 4.2 后端服务设计

#### 4.2.1 DemoRetriever服务

```go
// backend/services/demo_retriever.go

package services

type DemoRetriever struct {
    db *gorm.DB
}

// GetDemosByFeatures 根据特性检索相关Demo片段
func (r *DemoRetriever) GetDemosByFeatures(moduleID uint, features []string) (map[string]interface{}, error) {
    // 1. 查询该Module的所有活跃Demo
    var demos []models.ModuleDemo
    r.db.Where("module_id = ? AND is_active = ?", moduleID, true).Find(&demos)
    
    // 2. 获取每个Demo的最新版本config_data
    result := make(map[string]interface{})
    for _, demo := range demos {
        var version models.ModuleDemoVersion
        r.db.Where("demo_id = ? AND is_latest = ?", demo.ID, true).First(&version)
        
        // 3. 从config_data中提取包含指定features的片段
        for _, feature := range features {
            if snippet, ok := version.ConfigData[feature]; ok {
                result[feature] = snippet
            }
        }
    }
    
    return result, nil
}

// GetDemoSnippetsAsMarkdown 将Demo片段格式化为Markdown
func (r *DemoRetriever) GetDemoSnippetsAsMarkdown(snippets map[string]interface{}) string {
    var sb strings.Builder
    sb.WriteString("## 配置示例参考\n\n")
    
    for feature, config := range snippets {
        sb.WriteString(fmt.Sprintf("### %s 配置示例\n", feature))
        sb.WriteString("```json\n")
        jsonBytes, _ := json.MarshalIndent(map[string]interface{}{feature: config}, "", "  ")
        sb.WriteString(string(jsonBytes))
        sb.WriteString("\n```\n\n")
    }
    
    return sb.String()
}
```

#### 4.2.2 修改SkillAssembler

```go
// backend/services/skill_assembler.go

// 在AssemblePrompt方法中添加Demo注入逻辑
func (a *SkillAssembler) AssemblePrompt(...) (*AssemblePromptResult, error) {
    // ... 现有代码 ...
    
    // 新增：检查是否需要注入Demo
    if dynamicContext != nil && dynamicContext.ExtraContext != nil {
        if features, ok := dynamicContext.ExtraContext["required_features"].([]string); ok && len(features) > 0 {
            log.Printf("[SkillAssembler] 检测到需要Demo参考，features: %v", features)
            
            demoRetriever := NewDemoRetriever(a.db)
            snippets, err := demoRetriever.GetDemosByFeatures(moduleID, features)
            if err == nil && len(snippets) > 0 {
                demoMarkdown := demoRetriever.GetDemoSnippetsAsMarkdown(snippets)
                // 将Demo内容添加到prompt中
                promptParts = append(promptParts, demoMarkdown)
                log.Printf("[SkillAssembler] 注入了 %d 个Demo片段", len(snippets))
            }
        }
    }
    
    // ... 现有代码 ...
}
```

### 4.3 Skill Workflow修改

#### 4.3.1 修改 resource_generation_workflow.md

在现有workflow中添加"步骤0：需求复杂度分析"：

```markdown
### 步骤 0: 需求复杂度分析（新增）

在开始生成配置之前，先分析用户需求的复杂度。

#### 0.1 识别用户提到的功能特性

从用户描述中识别以下关键词，映射到对应的复杂字段：

| 关键词 | 对应字段 |
|--------|----------|
| 生命周期、过期、转换存储类型、归档 | lifecycle_rule |
| CORS、跨域 | cors_rule |
| 静态网站、网站托管 | website |
| 智能分层 | intelligent_tiering |
| 加密、KMS | server_side_encryption_configuration |
| 对象锁定、合规 | object_lock_configuration |
| 复制、跨区域复制 | replication_configuration |
| 日志、访问日志 | logging |
| 指标、监控 | metric_configuration |

#### 0.2 判断是否需要参考示例

| 条件 | 是否需要Demo |
|------|--------------|
| 只涉及基础字段（bucket, tags, versioning等） | 否 |
| 涉及1-2个复杂字段 | 可选（建议参考） |
| 涉及3个以上复杂字段 | 是 |
| 用户明确要求"完整配置"或"高级配置" | 是 |

#### 0.3 输出分析结果

如果判断需要参考示例，在内部标记：
- `need_demo: true`
- `required_features: ["lifecycle_rule", ...]`

系统将自动注入相关配置示例到上下文中。
```

### 4.4 API设计

#### 4.4.1 新增Demo检索API（可选）

```
GET /api/v1/modules/:id/demos/snippets?features=lifecycle_rule,cors_rule
```

**响应**：
```json
{
  "snippets": {
    "lifecycle_rule": [...],
    "cors_rule": [...]
  },
  "source_demos": ["complete"]
}
```

---

## 5. 实现步骤

### 5.1 Phase 1：基础实现（2天）

- [ ] 创建`DemoRetriever`服务
- [ ] 修改`SkillAssembler`支持Demo注入
- [ ] 添加Demo检索API

### 5.2 Phase 2：Workflow优化（1天）

- [ ] 修改`resource_generation_workflow.md`添加需求分析步骤
- [ ] 定义复杂字段关键词映射表

### 5.3 Phase 3：测试与调优（1-2天）

- [ ] 测试不同复杂度的用户需求
- [ ] 调优Demo检索逻辑
- [ ] 验证生成效果

---

## 6. 风险与注意事项

### 6.1 风险

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| Demo数据不完整 | 无法提供有效参考 | 确保每个Module有minimal和complete两个Demo |
| Demo片段过大 | 超出上下文限制 | 只提取需要的字段，限制片段大小 |
| 误判需求复杂度 | 不必要的Demo注入或缺失 | 提供手动覆盖选项 |

### 6.2 注意事项

1. **Demo质量**：Demo数据需要保持更新，与Schema同步
2. **性能考虑**：Demo检索应该有缓存机制
3. **可配置性**：是否启用Demo自动检索应该可配置

---

## 7. 未来扩展

### 7.1 语义检索（Phase 2）

- 对Demo的description和config_data建立向量索引
- 根据用户描述进行语义检索
- 返回最相关的Demo片段

### 7.2 Demo推荐（Phase 3）

- 基于用户历史使用记录推荐Demo
- 基于组织/团队的常用配置推荐

### 7.3 Demo自动生成（Phase 4）

- 从生产环境的成功配置中自动提取Demo
- 基于最佳实践自动生成推荐配置

---

## 8. 相关文件

| 文件 | 说明 |
|------|------|
| `backend/services/skill_assembler.go` | Skill组装器，需要修改 |
| `backend/services/demo_retriever.go` | 新增，Demo检索服务 |
| `skill/resource_generation/task/resource_generation_workflow.md` | 资源生成workflow，需要修改 |
| `backend/internal/models/module_demo.go` | Demo模型 |
| `backend/internal/models/module_demo_version.go` | Demo版本模型 |

---

## 9. 参考

- [Module Skill生成Workflow](../../skill/module_parsing/task/module_skill_generation_workflow.md)
- [资源生成Workflow](../../skill/resource_generation/task/resource_generation_workflow.md)
- [Skill系统设计文档](./skill-system-design.md)