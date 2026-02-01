# Claude Skills 三层架构定义

## 架构概览

Claude Skills 采用三层架构设计：Foundation 层（强制规范）→ Domain 层（基线规范）→ Task 层（任务执行）

```
┌─────────────────────────────────────────────────────────────┐
│                    Task Layer (任务层)                       │
│                      "What to do"                           │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ 安全基线检查  │  │ Q3财报生成   │  │ 代码审计     │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
                           ↓ 组合引用
┌─────────────────────────────────────────────────────────────┐
│                   Domain Layer (领域层)                      │
│              "基线规范 - 可自由组合"                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ 信息安全规范  │  │ 财务建模规范 │  │ Python规范   │      │
│  │ - 密码强度   │  │ - 颜色编码   │  │ - PEP8      │      │
│  │ - 加密标准   │  │ - 公式规则   │  │ - 类型注解   │      │
│  │ - 访问控制   │  │ - 审计追踪   │  │ - 文档字符串 │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
                           ↓ 必须遵守
┌─────────────────────────────────────────────────────────────┐
│                Foundation Layer (基础层)                     │
│                  "强制规范 - How to do"                      │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ docx         │  │ xlsx         │  │ pdf          │      │
│  │ pptx         │  │ json         │  │ sql          │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
```

---

## Layer 1: Foundation Layer (基础层) - "强制规范"

### 定义
提供**不可妥协的技术实现规范**，定义如何正确、安全地操作特定格式或工具。这些是技术层面的"硬性要求"。

### 核心特征
- **强制性 (Mandatory)**: 必须遵守的技术规范，不可绕过
- **格式专精 (Format-Specific)**: 针对特定文件格式或技术栈
- **技术正确性 (Technical Correctness)**: 确保操作不会损坏数据或产生错误
- **工具属性 (Tool-like)**: 被动调用，不主动决策业务逻辑
- **零业务假设 (Business-Agnostic)**: 不包含任何行业或领域知识

### 典型内容

#### 官方 Foundation Skills
```yaml
docx:
  强制规范:
    - 必须保持文档结构完整性
    - 必须正确处理 tracked changes
    - 必须保留原有格式（除非明确要求修改）
    
xlsx:
  强制规范:
    - 必须零公式错误（#REF!, #DIV/0!, #VALUE!）
    - 公式必须使用单元格引用，不能硬编码值
    - 必须验证所有单元格依赖关系存在
    
pdf:
  强制规范:
    - 必须保持 PDF 完整性
    - 加密必须使用标准加密算法
    - 表单填充必须保留原有字段
```

#### 自定义 Foundation Skills 示例
```yaml
json-processor:
  强制规范:
    - 必须验证 JSON Schema 有效性
    - 必须处理 Unicode 字符转义
    - 必须保持数据类型精确性（number vs string）

sql-executor:
  强制规范:
    - 必须使用参数化查询防止 SQL 注入
    - 必须在事务内执行写操作
    - 必须验证连接池配置

python-runtime:
  强制规范:
    - 必须使用虚拟环境隔离依赖
    - 必须验证包签名（如果启用）
    - 必须捕获并记录所有异常
```

### 技能结构
```
foundation-skills/
├── xlsx/
│   ├── SKILL.md                    # 强制规范文档
│   │   ├── 零公式错误要求
│   │   ├── 公式构建规则（技术层面）
│   │   ├── LibreOffice 配置要求
│   ├── scripts/
│   │   ├── recalc.py              # 公式重算脚本
│   │   ├── validate.py            # 验证脚本
│   └── references/
│       └── openpyxl_api.md        # API 参考
```

### 何时创建 Foundation Skill
✅ **应该创建时**:
- 需要处理特定文件格式（.docx, .json, .yaml）
- 需要与特定技术栈交互（SQL, Git, Docker）
- 有明确的技术正确性要求
- 需要提供可复用的底层能力

❌ **不应该创建时**:
- 仅包含业务逻辑或规则
- 只是组合其他技能的工作流
- 只适用于特定公司或项目

---

## Layer 2: Domain Layer (领域层) - "基线规范 - 可自由组合"

### 定义
定义**特定领域或行业的标准和最佳实践**，可被多个任务技能灵活组合使用。这些是业务层面的"推荐做法"。

### 核心特征
- **领域专精 (Domain-Specific)**: 针对特定行业、部门或专业领域
- **可组合性 (Composable)**: 可被多个 Task 技能混合引用
- **规范性 (Normative)**: 定义"应该如何做"的标准
- **独立性 (Independent)**: 每个规范可独立存在，不强制捆绑
- **可覆盖性 (Overridable)**: Task 层可根据具体需求调整

### 典型示例

#### 信息安全领域
```yaml
security-baseline:
  描述: 企业级信息安全基线规范
  适用范围: 所有涉及数据安全、访问控制的任务
  
  包含规范:
    密码策略:
      - 最小长度: 12 字符
      - 复杂度要求: 大小写+数字+特殊字符
      - 过期周期: 90 天
      - 禁止重用: 最近 5 次
    
    数据加密:
      - 静态数据: AES-256
      - 传输数据: TLS 1.2+
      - 密钥管理: 使用 KMS
    
    访问控制:
      - 最小权限原则
      - 强制双因素认证（敏感操作）
      - 会话超时: 30 分钟
    
    审计日志:
      - 记录所有访问和修改
      - 保留期: 2 年
      - 不可篡改性验证

network-security:
  描述: 网络安全配置规范
  
  包含规范:
    防火墙规则:
      - 默认拒绝策略
      - 白名单 IP 范围
      - 端口开放最小化
    
    入侵检测:
      - 异常流量监控
      - 自动告警阈值
```

#### 财务建模领域
```yaml
financial-modeling-standards:
  描述: 财务建模行业标准（基于 FAST 标准）
  适用范围: 所有财务分析、预测、估值任务
  
  包含规范:
    颜色编码:
      - 蓝色: 用户输入的假设
      - 黑色: 公式计算结果
      - 绿色: 同工作簿内部链接
      - 红色: 外部文件链接
      - 黄色背景: 需要特别关注的单元格
    
    公式规则:
      - 所有假设必须独立成单元格
      - 禁止在公式中硬编码数值
      - 使用绝对引用锁定假设单元格
      - 每个公式必须可审计
    
    数字格式:
      - 货币: $#,##0（标注单位如 $mm）
      - 百分比: 0.0%
      - 负数: 括号形式 (123) 而非 -123
      - 零值显示为 "-"
    
    文档要求:
      - 每个假设必须注明来源
      - 格式: "Source: [Document], [Date], [Page/Section]"
      - 外部数据必须包含 URL

audit-trail-standards:
  描述: 审计追踪标准
  
  包含规范:
    变更记录:
      - 记录修改人、时间、原因
      - 保留历史版本
      - 关键单元格锁定保护
```

#### 软件开发领域
```yaml
python-coding-standards:
  描述: Python 代码规范（基于 PEP8 + 公司扩展）
  
  包含规范:
    代码风格:
      - 缩进: 4 空格
      - 行长: 88 字符（Black formatter）
      - 命名: snake_case (函数/变量), PascalCase (类)
    
    类型注解:
      - 所有公共函数必须有类型注解
      - 使用 typing 模块
      - 复杂类型使用 TypedDict/Protocol
    
    文档字符串:
      - 所有模块、类、函数必须有 docstring
      - 格式: Google Style
      - 包含参数说明、返回值、异常
    
    错误处理:
      - 优先使用具体异常类型
      - 避免裸 except
      - 记录异常上下文

api-design-standards:
  描述: RESTful API 设计规范
  
  包含规范:
    URL 设计:
      - 使用复数名词
      - 版本号在路径中 /v1/
      - 资源嵌套不超过 3 层
    
    状态码:
      - 2xx: 成功
      - 4xx: 客户端错误
      - 5xx: 服务端错误
    
    响应格式:
      - 统一 JSON 结构
      - 包含 metadata
      - 错误消息标准化
```

#### 数据治理领域
```yaml
data-privacy-compliance:
  描述: 数据隐私合规规范（GDPR/CCPA）
  
  包含规范:
    个人信息识别:
      - PII 数据类型清单
      - 敏感度分类
    
    处理原则:
      - 数据最小化
      - 明确用途说明
      - 用户同意记录
    
    存储要求:
      - 加密存储
      - 访问日志
      - 定期清理
    
    数据主体权利:
      - 访问权
      - 删除权（被遗忘权）
      - 可携带权
```

### 技能结构
```
domain-skills/
├── security-baseline/
│   ├── SKILL.md                    # 规范总览
│   ├── references/
│   │   ├── password-policy.md     # 密码策略详细说明
│   │   ├── encryption-standards.md
│   │   ├── access-control.md
│   │   └── audit-requirements.md
│   └── assets/
│       ├── security-checklist.xlsx # 检查清单模板
│       └── compliance-report-template.docx

├── financial-modeling-standards/
│   ├── SKILL.md
│   ├── references/
│   │   ├── fast-standards.md      # FAST 建模标准
│   │   ├── color-coding-guide.md
│   │   └── formula-best-practices.md
│   └── assets/
│       └── model-template.xlsx     # 标准模型模板

└── python-coding-standards/
    ├── SKILL.md
    ├── references/
    │   ├── pep8-summary.md
    │   ├── type-annotation-guide.md
    │   └── docstring-examples.md
    └── scripts/
        ├── linter-config.toml      # Ruff/Black 配置
        └── pre-commit-hook.sh      # Git pre-commit hook
```

### SKILL.md 示例结构
```markdown
---
name: security-baseline
description: 企业级信息安全基线规范。当任务涉及密码管理、数据加密、访问控制、审计日志等安全相关内容时使用。本规范定义了可组合的安全标准，可被多个任务技能引用。
---

# 信息安全基线规范

## 概述
本规范定义了企业级应用的最低安全要求。Task 技能可根据具体场景选择性应用这些规范。

## 规范模块

### 1. 密码策略
详见 [references/password-policy.md](references/password-policy.md)

**快速参考**:
- 最小长度: 12 字符
- 复杂度: 必须包含大小写字母、数字、特殊字符
- 过期周期: 90 天

### 2. 数据加密
详见 [references/encryption-standards.md](references/encryption-standards.md)

**快速参考**:
- 静态数据: AES-256-GCM
- 传输数据: TLS 1.3（最低 1.2）
- 密钥长度: 至少 2048 位 RSA 或 256 位 ECC

### 3. 访问控制
详见 [references/access-control.md](references/access-control.md)

**快速参考**:
- 最小权限原则（Principle of Least Privilege）
- 基于角色的访问控制（RBAC）
- 敏感操作强制 MFA

### 4. 审计日志
详见 [references/audit-requirements.md](references/audit-requirements.md)

**快速参考**:
- 必须记录: 谁、何时、做了什么、结果如何
- 保留期: 最少 2 年
- 完整性保护: 使用只追加日志或区块链

## 使用指南

### 如何在 Task 技能中引用

Task 技能应在 SKILL.md 中明确说明使用了哪些规范模块:

```markdown
# 用户账户管理任务

## 依赖的领域规范
- security-baseline: 密码策略、访问控制、审计日志
- data-privacy-compliance: 个人信息保护

## 实施步骤
1. 读取 security-baseline 的密码策略要求
2. 创建符合复杂度要求的密码验证函数
3. ...
```

### 覆盖规范（Override）

如果特定任务需要更严格的要求:

```markdown
# 高敏感度系统任务

## 规范覆盖
基于 security-baseline，但采用更严格标准:
- 密码最小长度: 16 字符（覆盖默认的 12）
- 会话超时: 15 分钟（覆盖默认的 30）
- MFA: 所有操作强制启用（而非仅敏感操作）
```

## 检查清单
见 assets/security-checklist.xlsx
```

### 何时创建 Domain Skill
✅ **应该创建时**:
- 有明确的行业标准或最佳实践
- 规范可被多个不同任务复用
- 需要在不同项目间保持一致性
- 规范会随时间演进（集中维护）

❌ **不应该创建时**:
- 只适用于单一具体任务
- 规范过于宽泛没有实际指导意义
- 与 Foundation 层的技术规范混淆

---

## Layer 3: Task Layer (任务层) - "What to do"

### 定义
定义**完整的端到端业务任务**，编排 Foundation 和 Domain 技能，回答"做什么"的问题。

### 核心特征
- **任务导向 (Task-Oriented)**: 完成具体的业务目标
- **编排能力 (Orchestration)**: 组合多个底层技能
- **上下文感知 (Context-Aware)**: 理解业务背景和约束
- **决策逻辑 (Decision Logic)**: 包含条件判断和工作流
- **用户面向 (User-Facing)**: 直接响应用户请求

### 典型示例

#### 示例 1: 安全基线检查
```yaml
security-baseline-audit:
  描述: 对系统配置进行安全基线合规性检查
  
  引用的技能:
    Foundation:
      - json-processor: 读取配置文件
      - yaml-processor: 解析 YAML 配置
      - xlsx: 生成检查报告
    Domain:
      - security-baseline: 密码策略、加密标准、访问控制
      - audit-trail-standards: 审计日志要求
  
  工作流:
    1. 读取系统配置文件（使用 json-processor）
    2. 对照 security-baseline 的各项规范
    3. 检测不合规项目
    4. 生成详细审计报告（使用 xlsx）
    5. 标注风险等级（高/中/低）
  
  输出:
    - security-audit-report.xlsx
    - 不合规项清单
    - 修复建议
```

#### 示例 2: Q3 财务报表生成
```yaml
quarterly-financial-report:
  描述: 生成季度财务分析报表
  
  引用的技能:
    Foundation:
      - xlsx: 表格操作
      - pptx: 演示文稿
      - pdf: 最终报告生成
    Domain:
      - financial-modeling-standards: 颜色编码、公式规则、数字格式
      - audit-trail-standards: 数据来源追踪
  
  工作流:
    1. 收集原始财务数据
    2. 创建符合 financial-modeling-standards 的分析模型
       - 应用颜色编码规则
       - 确保所有假设可追溯
       - 使用标准数字格式
    3. 生成三大财务报表
       - 资产负债表
       - 利润表
       - 现金流量表
    4. 计算关键财务指标
    5. 创建管理层报告（pptx）
    6. 生成最终 PDF 报告
  
  输出:
    - Q3-2024-Financial-Model.xlsx
    - Q3-2024-Executive-Summary.pptx
    - Q3-2024-Full-Report.pdf
```

#### 示例 3: Python 代码审计
```yaml
python-code-review:
  描述: 对 Python 代码库进行全面审计
  
  引用的技能:
    Foundation:
      - git-operator: 代码仓库操作
      - python-runtime: 执行静态分析工具
      - docx: 生成审计报告
    Domain:
      - python-coding-standards: PEP8、类型注解、文档规范
      - security-baseline: 代码安全检查
      - api-design-standards: API 设计审查
  
  工作流:
    1. 克隆代码仓库（git-operator）
    2. 运行静态分析工具
       - Ruff/Black: 代码风格检查（python-coding-standards）
       - Mypy: 类型检查
       - Bandit: 安全漏洞扫描（security-baseline）
    3. 人工审查关键模块
       - API 设计是否符合 api-design-standards
       - 文档完整性
       - 测试覆盖率
    4. 汇总问题并分级
       - Critical: 安全漏洞、数据泄露风险
       - High: 违反强制规范
       - Medium: 不符合推荐实践
       - Low: 代码风格问题
    5. 生成审计报告（docx）
  
  输出:
    - code-review-report.docx
    - issues.json（结构化问题清单）
    - 修复优先级建议
```

#### 示例 4: 数据库设计评审
```yaml
database-design-review:
  描述: 评审数据库设计方案的合规性和最佳实践
  
  引用的技能:
    Foundation:
      - sql-executor: 执行查询和分析
      - xlsx: 生成评审报告
    Domain:
      - data-privacy-compliance: GDPR/CCPA 合规性
      - security-baseline: 数据加密、访问控制
      - database-design-patterns: 范式化、索引策略
  
  工作流:
    1. 分析数据库 Schema
    2. 识别个人敏感信息（PII）
       - 对照 data-privacy-compliance 的 PII 清单
    3. 检查加密和访问控制
       - 验证敏感字段加密（security-baseline）
       - 检查行级权限配置
    4. 评估设计质量
       - 范式化程度（database-design-patterns）
       - 索引优化建议
       - 性能瓶颈预警
    5. 生成评审报告
  
  输出:
    - database-review.xlsx
    - 合规性检查清单
    - 优化建议
```

### 技能结构
```
task-skills/
├── security-baseline-audit/
│   ├── SKILL.md                    # 任务定义和工作流
│   ├── scripts/
│   │   ├── run_audit.py           # 主执行脚本
│   │   ├── config_parser.py       # 配置解析器
│   │   └── report_generator.py    # 报告生成器
│   ├── references/
│   │   ├── workflow.md            # 详细工作流说明
│   │   └── risk-matrix.md         # 风险评估矩阵
│   └── assets/
│       └── report-template.xlsx   # 报告模板

├── quarterly-financial-report/
│   ├── SKILL.md
│   ├── scripts/
│   │   ├── data_collector.py      # 数据收集
│   │   ├── model_builder.py       # 模型构建
│   │   └── report_generator.py    # 报告生成
│   └── assets/
│       ├── financial-model-template.xlsx
│       └── presentation-template.pptx

└── python-code-review/
    ├── SKILL.md
    ├── scripts/
    │   ├── analyze.py              # 静态分析
    │   ├── security_scan.py        # 安全扫描
    │   └── generate_report.py      # 报告生成
    └── references/
        ├── checklist.md            # 审查清单
        └── severity-guidelines.md  # 问题严重性指南
```

### SKILL.md 示例结构
```markdown
---
name: security-baseline-audit
description: 对系统进行安全基线合规性审计。当用户需要检查系统配置是否符合安全规范、生成安全审计报告、识别安全风险时触发。
---

# 安全基线审计任务

## 任务目标
对目标系统进行全面的安全基线合规性检查，识别不合规项，生成审计报告和修复建议。

## 依赖技能

### Foundation 层
1. **json-processor**: 读取和解析 JSON 配置文件
2. **yaml-processor**: 读取和解析 YAML 配置文件
3. **xlsx**: 生成 Excel 格式的审计报告

### Domain 层
1. **security-baseline**: 
   - 使用模块: 密码策略、数据加密、访问控制、审计日志
   - 参考文件: 全部 references/
   
2. **audit-trail-standards**:
   - 使用模块: 变更记录、审计日志格式

## 执行工作流

### 步骤 1: 准备阶段
```bash
# 确保依赖技能可用
view /mnt/skills/domain/security-baseline/SKILL.md
view /mnt/skills/foundation/json-processor/SKILL.md
```

### 步骤 2: 数据收集
1. 识别配置文件类型（JSON/YAML/其他）
2. 使用相应的 processor 读取配置
3. 提取关键安全配置项:
   - 用户认证设置
   - 密码策略
   - 加密配置
   - 访问控制列表
   - 日志配置

### 步骤 3: 合规性检查
对照 security-baseline 的各项规范逐一检查:

**密码策略检查**:
```python
# 读取 security-baseline 的密码要求
view /mnt/skills/domain/security-baseline/references/password-policy.md

# 执行检查
password_issues = check_password_policy(
    config=system_config,
    baseline=security_baseline.password_policy
)
```

**加密标准检查**:
```python
# 读取加密标准
view /mnt/skills/domain/security-baseline/references/encryption-standards.md

# 检查静态数据加密
encryption_issues = check_encryption(
    config=system_config,
    required_algorithm="AES-256-GCM"
)
```

**访问控制检查**:
```python
# 检查访问控制配置
access_issues = check_access_control(
    config=system_config,
    baseline=security_baseline.access_control
)
```

**审计日志检查**:
```python
# 检查审计日志配置
audit_issues = check_audit_logs(
    config=system_config,
    baseline=security_baseline.audit_requirements
)
```

### 步骤 4: 风险评级
根据 references/risk-matrix.md 对每个不合规项进行风险评级:

- **Critical (严重)**: 存在明显安全漏洞，可能导致数据泄露
  - 例: 未启用加密、弱密码策略、无审计日志
  
- **High (高)**: 不符合强制安全规范
  - 例: 加密算法不符合标准、访问控制不完整
  
- **Medium (中)**: 不符合推荐实践
  - 例: 会话超时时间过长、日志保留期不足
  
- **Low (低)**: 次要配置问题
  - 例: 日志格式不统一

### 步骤 5: 生成报告
使用 xlsx 技能生成审计报告:

```python
# 读取 xlsx 技能的最佳实践
view /mnt/skills/foundation/xlsx/SKILL.md

# 使用报告模板
template = "assets/report-template.xlsx"

# 生成报告
create_audit_report(
    template=template,
    findings=all_issues,
    summary=executive_summary,
    output="security-audit-report.xlsx"
)
```

报告包含:
1. **执行摘要**: 总体合规状态、关键发现
2. **详细发现**: 按风险等级分组的不合规项
3. **合规性矩阵**: 每项规范的符合情况
4. **修复建议**: 优先级排序的修复步骤
5. **附录**: 完整的配置清单

### 步骤 6: 输出交付
```markdown
[present_files tool]
- security-audit-report.xlsx
- issues.json (结构化数据，供自动化处理)
```

## 示例对话

**用户**: 请对我们的 production 环境进行安全基线审计

**Claude 执行**:
1. 读取 security-baseline 技能获取检查标准
2. 收集 production 配置文件
3. 逐项对照检查
4. 发现问题: 
   - Critical: 数据库连接未加密
   - High: 密码最小长度仅 8 字符（要求 12）
   - Medium: 会话超时 60 分钟（推荐 30）
5. 生成报告
6. 提供修复建议和优先级

## 注意事项

### 规范覆盖（Override）
如果用户指定了更严格的要求:
```markdown
用户: 我们需要符合 SOC 2 Type II，密码要求 16 位

Claude: 了解，将使用比 security-baseline 更严格的标准:
- 密码最小长度: 16 字符 (覆盖默认 12)
- 强制 MFA: 所有用户 (覆盖默认仅敏感操作)
```

### 自定义规范
如果 security-baseline 不包含某些行业特定要求:
```markdown
用户: 还需要检查是否符合 HIPAA 的 PHI 保护要求

Claude: security-baseline 不包含 HIPAA 规范。建议:
1. 创建 healthcare-compliance domain 技能包含 HIPAA 规范
2. 或在本次审计中手动补充检查项
3. 将补充检查结果记录在报告附录中
```

## 故障排除

### 配置文件格式不支持
如果遇到未知格式，先使用 bash 工具查看文件:
```bash
file config.unknown
head -20 config.unknown
```

### 规范冲突
如果 Domain 层的不同规范有冲突，优先级:
1. 用户明确指定的要求（最高）
2. 法律法规要求（如 GDPR）
3. Domain 技能中标注为 "MUST" 的规范
4. Domain 技能中标注为 "SHOULD" 的推荐实践
```

### 何时创建 Task Skill
✅ **应该创建时**:
- 有明确的端到端业务任务
- 需要组合多个 Foundation/Domain 技能
- 有复杂的决策逻辑或工作流
- 任务会被重复执行

❌ **不应该创建时**:
- 任务过于简单（仅调用一个 Foundation 技能）
- 任务过于通用（没有具体目标）
- 只是 Domain 规范的重复（应放在 Domain 层）

---

## 三层之间的交互模式

### 向下引用（Task → Domain → Foundation）

```python
# Task 层的任务执行
def execute_security_audit():
    # 1. 读取 Domain 层规范
    password_policy = read_domain_skill(
        "security-baseline",
        "references/password-policy.md"
    )
    
    # 2. 使用 Foundation 层工具读取配置
    config = json_processor.read("system-config.json")
    
    # 3. 执行业务逻辑（Task 层的核心）
    issues = []
    if config["password"]["min_length"] < password_policy["min_length"]:
        issues.append({
            "severity": "HIGH",
            "finding": f"密码长度 {config['password']['min_length']} 不符合要求 {password_policy['min_length']}",
            "remediation": "修改配置文件中的 password.min_length 参数"
        })
    
    # 4. 使用 Foundation 层工具生成输出
    xlsx_processor.create_report(
        template="report-template.xlsx",
        data=issues,
        output="audit-report.xlsx"
    )
```

### 规范组合（Task 组合多个 Domain）

```python
# 财务报表任务同时应用多个 Domain 规范
def generate_financial_report():
    # 应用财务建模规范
    apply_standard("financial-modeling-standards")
    # - 颜色编码
    # - 公式规则
    # - 数字格式
    
    # 应用审计追踪规范
    apply_standard("audit-trail-standards")
    # - 数据来源标注
    # - 变更记录
    
    # 应用数据隐私规范（如果包含个人数据）
    if contains_pii(data):
        apply_standard("data-privacy-compliance")
        # - 数据脱敏
        # - 访问控制
```

### 规范覆盖（Task 覆盖 Domain 默认值）

```python
# Task 可以根据具体需求调整 Domain 规范
def high_security_audit():
    # 获取默认规范
    baseline = read_domain_skill("security-baseline")
    
    # 覆盖部分参数（更严格）
    custom_policy = baseline.copy()
    custom_policy["password"]["min_length"] = 16  # 默认 12
    custom_policy["session_timeout"] = 15         # 默认 30
    custom_policy["mfa_required"] = "ALL"          # 默认 "SENSITIVE_OPS"
    
    # 使用定制规范执行审计
    execute_audit(custom_policy)
```

---

## 技能发现机制

### Claude 如何发现需要使用哪些技能？

#### 1. 基于 Description 的自动触发
每个技能的 `description` 字段会被加载到上下文中，Claude 根据用户请求自动匹配:

```yaml
# 用户: "请生成 Q3 财务报告"
# Claude 自动匹配到:

quarterly-financial-report:
  description: "生成季度财务分析报表。当用户需要创建季度或年度财务报告、分析财务数据、生成管理层报告时触发。"
```

#### 2. Task 技能中的显式引用
在 Task 技能的 SKILL.md 中明确列出依赖:

```markdown
## 依赖技能

### Foundation 层
- 首先读取 `/mnt/skills/foundation/xlsx/SKILL.md`
- 然后读取 `/mnt/skills/foundation/pptx/SKILL.md`

### Domain 层
- 读取 `/mnt/skills/domain/financial-modeling-standards/SKILL.md`
- 读取 `/mnt/skills/domain/audit-trail-standards/SKILL.md`
```

#### 3. 渐进式加载
```
用户请求 → Task 技能触发 (基于 description)
    ↓
Task SKILL.md 加载
    ↓
Task 指示读取 Domain 技能
    ↓
Domain SKILL.md 加载
    ↓
Domain 指示读取 Foundation 技能
    ↓
Foundation SKILL.md 加载
    ↓
执行任务
```

### 最佳实践

#### Task 技能应该这样写依赖:
```markdown
## 执行前准备

在开始任务前，按顺序读取以下技能文档:

1. **Foundation 层依赖**
   ```bash
   view /mnt/skills/foundation/xlsx/SKILL.md
   view /mnt/skills/foundation/json-processor/SKILL.md
   ```

2. **Domain 层依赖**
   ```bash
   view /mnt/skills/domain/security-baseline/SKILL.md
   view /mnt/skills/domain/security-baseline/references/password-policy.md
   ```

3. 理解完所有依赖后再开始执行任务
```

---

## 技能开发 Checklist

### Foundation 技能
- [ ] 定义了技术层面的强制规范
- [ ] 不包含任何业务逻辑或行业知识
- [ ] 提供了可复用的脚本或工具
- [ ] 文档清晰说明了技术约束和错误处理
- [ ] 可被多个上层技能安全调用

### Domain 技能
- [ ] 定义了特定领域的标准和最佳实践
- [ ] 规范模块化，可独立使用
- [ ] 文档说明了何时应用/何时可覆盖
- [ ] 提供了具体的检查清单或示例
- [ ] 可被多个不同任务组合使用

### Task 技能
- [ ] 有明确的任务目标和成功标准
- [ ] 列出了所有依赖的 Foundation/Domain 技能
- [ ] 定义了清晰的工作流和决策逻辑
- [ ] 说明了异常情况的处理方式
- [ ] 包含了示例对话或使用场景

---

## 示例：完整的三层协作

### 场景：对新上线的 API 进行安全审计

```
用户请求:
"我们的用户认证 API 即将上线，请进行安全审计"

┌─────────────────────────────────────────────────────────┐
│ Task Layer: api-security-audit                          │
│ - 编排整个审计流程                                       │
│ - 决定检查哪些方面                                       │
│ - 生成最终报告                                          │
└─────────────────────────────────────────────────────────┘
         ↓ 引用规范
┌─────────────────────────────────────────────────────────┐
│ Domain Layer                                            │
│ - security-baseline: 密码策略、加密标准、访问控制        │
│ - api-design-standards: RESTful 设计规范                │
│ - data-privacy-compliance: GDPR/CCPA 合规               │
└─────────────────────────────────────────────────────────┘
         ↓ 使用工具
┌─────────────────────────────────────────────────────────┐
│ Foundation Layer                                        │
│ - json-processor: 解析 API spec                         │
│ - python-runtime: 执行安全扫描工具                       │
│ - docx: 生成审计报告                                    │
└─────────────────────────────────────────────────────────┘

执行流程:
1. Task 读取 security-baseline, api-design-standards
2. Task 使用 json-processor 读取 OpenAPI spec
3. Task 对照 Domain 规范检查:
   - 身份认证机制（security-baseline）
   - Token 过期时间（security-baseline）
   - API 端点命名（api-design-standards）
   - PII 数据处理（data-privacy-compliance）
4. Task 使用 python-runtime 运行 OWASP ZAP 扫描
5. Task 汇总发现，使用 docx 生成报告
6. 输出: api-security-audit-report.docx
```

---

## 总结对比

| 维度 | Foundation 层 | Domain 层 | Task 层 |
|------|--------------|-----------|---------|
| **目的** | 如何操作（How） | 应该遵守什么规范（What Standard） | 做什么任务（What to Do） |
| **性质** | 强制技术规范 | 可组合业务规范 | 端到端任务编排 |
| **抽象级别** | 工具/格式级别 | 领域/行业级别 | 任务/流程级别 |
| **复用性** | 跨所有领域复用 | 跨同领域任务复用 | 特定任务使用 |
| **业务逻辑** | 无 | 规范和标准 | 包含决策和流程 |
| **可覆盖性** | 不可覆盖 | 可覆盖/可组合 | 可定制 |
| **示例** | xlsx, pdf, json | security-baseline, financial-modeling-standards | security-audit, financial-report |