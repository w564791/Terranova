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

## 输入

### 用户请求
{user_description}

### Module 库候选（按描述召回，JSON 数组；可能为空提示）
{module_candidates}

### 当前编辑上下文（光标处选区或文件内容，可能为空）
{current_content}

## 执行步骤

### 步骤 1: 判断是否复用 Module
- 仔细阅读"Module 库候选"。若其中存在与用户需求高度匹配的 module，**优先生成 module 引用块**。
- 若候选为空，或没有合适匹配，则**直接生成原生 Terraform 资源块**（resource 块）。
- 不要编造不存在的 module source。module 的 source/version 必须来自候选数据。

### 步骤 2: 生成 HCL
- 复用 module 时：写 `module "<name>" { source = "<source>" version = "<version>" ... }`，并按候选的 inputs 填充必填变量。
- 生成原生资源时：使用正确的 provider 资源类型与必填参数。
- 若提供了"当前编辑上下文"且用户意图是修复/改写，则在该上下文基础上修改，保持风格一致。
- 无法确定的值用合理占位符（如 "REPLACE_ME"）并保持 HCL 语法合法。

### 步骤 3: 输出
- 仅输出一个 HCL 代码块，不要输出多余解释。

## 输出格式

```hcl
# 这里是生成的 HCL
```

## 质量检查
- HCL 语法合法，可被 terraform 解析。
- module 的 source/version 未编造，来自候选数据。
- 必填变量/参数均已给出值或合法占位符。
- 不包含与代码无关的自然语言段落。',
  '1.0.0', true, 0, 'manual', NULL, '{"tags": ["task", "manifest"]}'::jsonb, NULL, now(), now(), 'manifest 编辑器资源生成/修复工作流，输出 HCL'
)
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name, display_name = EXCLUDED.display_name, layer = EXCLUDED.layer,
  content = EXCLUDED.content, is_active = EXCLUDED.is_active, priority = EXCLUDED.priority,
  metadata = EXCLUDED.metadata, description = EXCLUDED.description, updated_at = now();

INSERT INTO public.skills
  (id, name, display_name, layer, content, version, is_active, priority, source_type, source_module_id, metadata, created_by, created_at, updated_at, description)
VALUES (
  'skill-task-102', 'manifest_check_workflow', 'Manifest 草稿检查工作流', 'task',
  '---
name: manifest_check_workflow
layer: task
description: manifest 编辑器的草稿检查工作流。检查 Terraform HCL 的基本问题，输出结构化问题列表。
tags: ["task", "manifest", "check", "lint", "terraform"]
---

# Manifest 草稿检查工作流

## 任务目标
对给定的 Terraform HCL 内容做基础检查，找出语法错误、明显的配置问题、缺失必填参数、引用未定义变量等，并以结构化 JSON 输出问题列表。

## 输入

### 文件路径
{file_path}

### 待检查内容
{check_content}

## 检查维度
- 语法错误（括号/引号不匹配、非法 HCL 结构）。
- 块定义问题（缺少必填参数、resource/module 标签缺失）。
- 引用问题（引用未定义的 var./local./module. 输出）。
- 明显的安全/最佳实践问题（如硬编码密钥、0.0.0.0/0 全开放）。

## 输出要求
- 仅输出 JSON，不要输出多余文字。
- 待检查内容每行都以「行号: 内容」形式给出。**line 必须直接填该行前缀里的行号，不要自己重新数行**。无法定位时填 0。
- level 取值仅限：error / warning / info。
- file 填写传入的文件路径；若未知留空。
- 若没有发现问题，返回空数组 issues: []。

## 输出格式

```json
{
  "issues": [
    {
      "file": "main.tf",
      "line": 12,
      "level": "error",
      "message": "resource 块缺少必填参数 ami"
    }
  ]
}
```',
  '1.0.0', true, 0, 'manual', NULL, '{"tags": ["task", "manifest"]}'::jsonb, NULL, now(), now(), 'manifest 编辑器草稿检查工作流，输出问题列表'
)
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name, display_name = EXCLUDED.display_name, layer = EXCLUDED.layer,
  content = EXCLUDED.content, is_active = EXCLUDED.is_active, priority = EXCLUDED.priority,
  metadata = EXCLUDED.metadata, description = EXCLUDED.description, updated_at = now();

COMMIT;
