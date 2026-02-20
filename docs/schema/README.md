# Schema 管理系统文档

本目录包含 IAC Platform Schema 管理系统的相关文档。

## 📋 目录

### 核心功能文档

- [JSON 编辑器功能设计](json-editor-feature-design.md) - JSON 编辑器功能设计与实现

## 🔗 相关文档

### 根目录相关文档

#### Schema 核心功能
- [动态 Schema 测试指南](../90-dynamic-schema-testing-guide.md) - 动态 Schema 功能测试指南
- [嵌套 Schema 渲染指南](../91-nested-schema-rendering-guide.md) - 嵌套 Schema 渲染实现
- [Schema 编辑功能指南](../92-schema-edit-feature-guide.md) - Schema 编辑功能使用指南
- [Schema 导入能力 4 指南](../93-schema-import-capability-4-guide.md) - Schema 导入能力增强

#### 示例与工具
- [S3 完整 Schema](../94-s3-complete-schema.json) - S3 模块完整 Schema 示例
- [TF 文件解析器指南](../95-tf-file-parser-guide.md) - Terraform 文件解析器使用指南

## 📖 功能概述

### Schema 系统核心功能

1. **动态 Schema** - 支持动态生成和验证 Terraform 资源 Schema
2. **嵌套渲染** - 支持复杂嵌套结构的 Schema 渲染
3. **Schema 编辑** - 提供可视化的 Schema 编辑功能
4. **Schema 导入** - 支持从多种来源导入 Schema 定义
5. **JSON 编辑器** - 提供强大的 JSON 编辑能力

### 主要特性

- **类型验证** - 自动验证 Schema 类型和约束
- **可视化编辑** - 图形化界面编辑 Schema
- **代码编辑** - 支持直接编辑 JSON 代码
- **实时预览** - 编辑时实时预览效果
- **错误提示** - 智能的错误检测和提示

## 🚀 快速开始

1. 查看 [JSON 编辑器功能设计](json-editor-feature-design.md) 了解编辑器功能
2. 阅读 [动态 Schema 测试指南](../90-dynamic-schema-testing-guide.md) 了解测试方法
3. 参考 [嵌套 Schema 渲染指南](../91-nested-schema-rendering-guide.md) 了解渲染机制
4. 查看 [S3 完整 Schema](../94-s3-complete-schema.json) 作为示例参考

## 💡 使用场景

### Schema 定义
- 为 Terraform 资源定义 Schema
- 创建自定义资源类型
- 定义复杂的嵌套结构

### Schema 编辑
- 可视化编辑 Schema 结构
- 调整字段类型和约束
- 添加描述和默认值

### Schema 导入
- 从 Terraform Provider 导入
- 从 JSON 文件导入
- 从 TF 文件解析导入

## 📝 文档维护

- 创建新功能时，应在此目录添加相应文档
- 更新功能时，应同步更新相关文档
- 添加新的 Schema 示例时，应提供完整的说明文档

---

**返回**: [文档主目录](../README.md)
