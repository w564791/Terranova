---
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
4. 只输出一个 HCL 代码块,无多余文字。
