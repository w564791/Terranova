-- 插入 module_skill_generation Task Skill
-- 用于根据 Module 的 Schema 自动生成 Skill 内容

INSERT INTO skills (id, name, display_name, layer, content, version, is_active, priority, source_type, metadata, created_at, updated_at)
VALUES (
    'skill-task-004',
    'module_skill_generation_workflow',
    'Module Skill 生成工作流',
    'task',
    '# Module Skill 生成工作流

## 任务
根据 Module 的 OpenAPI Schema 定义，生成一份结构化的 Skill 文档，用于指导 AI 理解该 Module 的配置规则和最佳实践。

## 输入
- Module 名称: {module_name}
- Module 提供商: {provider}
- Module 描述: {description}
- OpenAPI Schema: {openapi_schema}

## 输出要求
生成 Markdown 格式的 Skill 文档，必须包含以下章节：

### 1. 模块概述
简要描述该模块的用途和适用场景（2-3句话）。

### 2. 参数约束

#### 必填字段
使用表格列出所有必填字段：
| 字段名 | 类型 | 验证规则 | 描述 |

#### 重要可选字段
使用表格列出有默认值或常用的可选字段：
| 字段名 | 类型 | 默认值 | 描述 |

#### 参数关联关系
如果存在条件必填、互斥或依赖关系，用列表说明。

### 3. 配置分组
根据 UI 配置说明字段分组：
- **基础配置**：用户必须关注的核心字段
- **高级配置**：可选的高级功能字段

### 4. 数据源映射
如果定义了外部数据源，使用表格说明：
| 字段 | 数据源 | 说明 |

### 5. 最佳实践
根据字段定义总结 3-5 条配置建议。

### 6. 输出属性
如果定义了 outputs，使用表格说明：
| 输出名 | 类型 | 描述 |

## 生成规则
1. 只输出 Markdown 格式的文档，不要有任何额外解释
2. 字段描述优先使用中文 label，其次使用 description
3. 对于枚举类型，在验证规则列列出所有可选值
4. 如果某个章节没有内容，可以省略
5. 保持简洁，每个字段描述不超过一行',
    '1.0.0',
    true,
    10,
    'manual',
    '{"tags": ["module", "skill", "generation"], "description": "用于根据 Module Schema 自动生成 Skill 内容的工作流"}',
    NOW(),
    NOW()
)
ON CONFLICT (name) DO UPDATE SET
    content = EXCLUDED.content,
    display_name = EXCLUDED.display_name,
    metadata = EXCLUDED.metadata,
    updated_at = NOW();

-- 验证插入结果
SELECT id, name, display_name, layer, is_active FROM skills WHERE name = 'module_skill_generation_workflow';