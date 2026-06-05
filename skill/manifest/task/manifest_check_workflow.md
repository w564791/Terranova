---
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
```
