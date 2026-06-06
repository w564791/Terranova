-- Patch: manifest AI 两个 task skill(资源生成 / 草稿检查工作流)
-- 幂等: ON CONFLICT (id) DO UPDATE,可重复执行
BEGIN;

INSERT INTO public.skills
  (id, name, display_name, layer, content, version, is_active, priority, source_type, source_module_id, metadata, created_by, created_at, updated_at, description)
VALUES (
  'skill-task-101', 'manifest_resource_generation_workflow', 'Manifest 资源生成工作流', 'task',
  '---
name: manifest_resource_generation_workflow
layer: task
description: manifest 编辑器的资源生成/修复工作流。根据用户描述生成 Terraform HCL，优先复用 Module 库中的候选 module。
tags: ["task", "manifest", "generation", "hcl", "terraform"]
---

# Manifest 资源生成工作流

## 任务目标
根据用户的自然语言描述，生成可直接写入 manifest 草稿的 Terraform HCL 代码。

## 最高执行原则（必须遵守）
1. **Foundation 层 = 最高优先级硬约束**。本 prompt 里 Foundation 层提供的所有规范（标签规范、输出格式、占位符规范等）是**强制规则**。生成的每个资源都**必须逐条满足**所有适用的 Foundation 规则——尤其标签:按资源类型把"必须标签清单"里的标签**全部**写进 `tags`,一个不能少(常被漏的:backup-enabled、env、managed-by-terraform)。
2. **冲突一律以 Foundation 为准**。
3. **优先复用 Module 库**:Module 候选里有与需求匹配的 module 时,**必须生成 module 引用块,不得改写成原生 resource**;只有候选为空或确无匹配,才生成原生 resource。

## 输入

### 用户请求
{user_description}

### Module 库候选（按描述召回，JSON 数组；可能为空提示）
{module_candidates}

### 当前编辑上下文（光标处选区或文件内容，可能为空）
{current_content}

### 候选 Module 的配置知识（可能为空；非空时优先据此填写 module 参数）
{module_skills}

## 执行步骤

### 步骤 1: 判断是否复用 Module（强制）
- 仔细阅读"Module 库候选"。**只要其中存在能满足用户需求的 module(资源类型一致即算匹配),就必须生成 module 引用块,禁止改写成原生 resource。**
- 仅当候选为空、或没有任何资源类型匹配的 module 时,才生成原生 Terraform 资源块。
- 不要编造不存在的 module source。module 的 source/version 必须来自候选数据。

### 步骤 2: 生成 HCL
- 复用 module 时：写 `module "<name>" { source = "<source>" version = "<version>" ... }`，并按候选的 inputs 填充必填变量。
- 生成原生资源时：使用正确的 provider 资源类型与必填参数。
- **标签(强制)**:按"最高执行原则"第 1 条,给每个资源的 `tags` 写全 Foundation 标签规范里该资源类型的所有必须标签。
- 若提供了"当前编辑上下文"且用户意图是修复/改写，则在该上下文基础上修改，保持风格一致。
- 无法确定的值用合理占位符并保持 HCL 语法合法。

### 步骤 3: 输出
- 仅输出一个 HCL 代码块，不要输出多余解释。

## 输出格式

```hcl
# 这里是生成的 HCL
```

## 输出前的强制自检（逐项确认后才输出）
1. 有匹配的 Module 候选时,我是否用了 module 引用块而非原生 resource?
2. 每个资源的 `tags` 是否按其资源类型写全了 Foundation 必须标签清单?(逐项核对,常漏:backup-enabled、env、managed-by-terraform)
3. HCL 语法合法、source/version 未编造、必填项有值或合法占位符。
4. 只输出一个 HCL 代码块,无多余文字。',
  '1.0.0', true, 0, 'manual', NULL, '{"tags": ["task", "manifest"]}'::jsonb, NULL, now(), now(), 'manifest 编辑器资源生成/修复工作流，输出 HCL'
)
ON CONFLICT (id) DO UPDATE SET
  name=EXCLUDED.name, display_name=EXCLUDED.display_name, layer=EXCLUDED.layer,
  content=EXCLUDED.content, is_active=EXCLUDED.is_active, priority=EXCLUDED.priority,
  metadata=EXCLUDED.metadata, description=EXCLUDED.description, updated_at=now();

INSERT INTO public.skills
  (id, name, display_name, layer, content, version, is_active, priority, source_type, source_module_id, metadata, created_by, created_at, updated_at, description)
VALUES (
  'skill-task-102', 'manifest_check_workflow', 'Manifest 草稿检查工作流', 'task',
  '---
name: manifest_check_workflow
layer: task
description: manifest 编辑器的草稿检查工作流。检查 Terraform HCL 的基本问题，输出结构化问题列表（含可选修复）。
tags: ["task", "manifest", "check", "lint", "terraform"]
---

# Manifest 草稿检查工作流

## 任务目标
对给定的 Terraform HCL 内容做检查，找出违反规范的问题、语法错误、配置问题、缺失必填参数、引用未定义变量等，并以结构化 JSON 输出问题列表；能确定性修复的问题额外给出修复。

## 最高执行原则（必须遵守）
1. **Foundation 层 = 最高优先级硬约束**。本 prompt 里 Foundation 层提供的所有规范（如标签规范、输出格式、占位符规范、风险基线等）是**强制规则**，不是"建议"。
2. **冲突一律以 Foundation 为准**。当某条最佳实践 / 用户写法 / 你的判断与 Foundation 规则冲突时，以 Foundation 为准。
3. **逐资源、逐规则核对,禁止抽样**。HCL 里有多少个 resource / module 块,就对**每一个块**独立完整核对所有适用的 Foundation 规则;不允许只检查第一个、或只挑明显的。
4. 若 Foundation 定义了"按资源类型的必须项清单"(如标签),对每个资源逐项比对清单,**确实缺失**的逐项报。

## 关于输出的铁律(防止把"思考过程"当问题)
- **issues 里只放真实存在的违规**。每条 issue 必须对应一个**确实存在**的问题。
- **核对过程必须保持静默**。不要把"我检查了什么"、"哪些已满足"、"经核对…"、"X 已提供"、"缺失:无"之类的**自检叙述**写进 issue。这些是你的内部思考,绝不出现在输出里。
- **某项合规 → 不产生任何 issue**。一个资源若完全满足某规范,就对该规范**什么都不输出**,而不是输出一条"已满足"的说明。
- 整个草稿若没有任何违规,返回 `issues: []`。
- 每条 issue 的 message 是**面向用户的、可执行的问题描述**(缺什么/错在哪/怎么不对),不是你的核对日志。

## 输入

### 主文件路径
{file_path}

### 待检查内容
{check_content}

## 检查维度（在遵守上面"最高执行原则"的前提下）
- **Foundation 规范符合性（最重要）**：逐资源对照 Foundation 层给出的所有规范，违反即报。例如标签规范——对每个 resource/module,按其资源类型核对"必须标签清单",缺哪个列哪个。
- 语法错误（括号/引号不匹配、非法 HCL 结构）。
- 块定义问题（缺少必填参数、resource/module 标签缺失）。
- 引用问题（引用未定义的 var./local./module./data. 输出，或引用了别处资源属性）。
- 安全问题（如硬编码密钥、0.0.0.0/0 全开放）。

## 输出要求
- 仅输出 JSON，不要输出多余文字。
- 待检查内容可能含多个文件，每个文件以 `### 文件: <路径>` 开头；每行都以「行号: 内容」形式给出。
- **line 必须直接填该行前缀里的行号，不要自己重新数行**。无法定位时填 0。
- **file 必须填该问题所属文件的 `### 文件:` 路径**。多文件检查时这是强制要求——省略 file 的问题将无法提供修复(fix 会被丢弃),因为行号无法安全定位到正确文件。
- level 取值仅限：error / warning / info。
- 若没有发现问题，返回空数组 issues: []。

## 修复（fix，可选）
- 仅对**能确定性修复**的问题给出 `fix`；不确定就省略该字段，不要瞎猜。
- `fix` 是按行范围替换：`start_line`/`end_line` 为要替换的行号范围（1-based，含，用前缀里的行号），`new_text` 为替换后的完整文本（含正确缩进，可多行）。
- `fix.file` 填该修复所属文件路径（默认与 issue.file 相同）。
- 替换范围要精确：只覆盖需要改动的行，不要把无关行卷进来。

## 输出格式

```json
{
  "issues": [
    {
      "file": "main.tf",
      "line": 12,
      "level": "error",
      "message": "resource 块缺少必填参数 ami",
      "fix": {
        "file": "main.tf",
        "start_line": 12,
        "end_line": 12,
        "new_text": "  ami = \"ami-xxxxxxxx\""
      }
    }
  ]
}
```

## 输出前自检(静默进行,不写进输出)
在给出最终 JSON 前,在**心里**完成下面的核对——这是内部步骤,**核对过程绝不能出现在 issues 里**:
- 是否对每一个 resource / module 块都按其资源类型,逐项比对了 Foundation 的必须项清单(如标签)?
- 是否覆盖了其余 Foundation 规则?
核对后,**只把"确实违规"的项写成 issue**;凡是已满足、不适用、或核对通过的,一律不输出。如果你发现自己想写"已提供/无缺失/经核对…"这类话,说明那不是问题,删掉它,不要放进 issues。',
  '1.0.0', true, 0, 'manual', NULL, '{"tags": ["task", "manifest"]}'::jsonb, NULL, now(), now(), 'manifest 编辑器草稿检查工作流，输出问题列表'
)
ON CONFLICT (id) DO UPDATE SET
  name=EXCLUDED.name, display_name=EXCLUDED.display_name, layer=EXCLUDED.layer,
  content=EXCLUDED.content, is_active=EXCLUDED.is_active, priority=EXCLUDED.priority,
  metadata=EXCLUDED.metadata, description=EXCLUDED.description, updated_at=now();

COMMIT;
