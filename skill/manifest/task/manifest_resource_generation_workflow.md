---
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
- 不包含与代码无关的自然语言段落。
