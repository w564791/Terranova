# Module 间引用设计文档

## 需求概述

在 Manifest 编辑器中，实现 Module 之间的连线和变量引用功能：

1. **Module 间连线**：A → B 表示 B 依赖 A，B 可以使用 A 的 output 作为参数值
2. **变量引用**：在参数输入时，用户输入 `/` 触发变量选择器
3. **自动渲染**：选择后自动渲染为 `module.A.output_name` 格式
4. **HCL 导入**：导入时自动识别 `module.xxx.xxx` 引用并创建连线

## 数据结构设计

### Edge 类型扩展

```typescript
interface ManifestEdge {
  id: string;
  type: 'dependency' | 'variable_binding';
  source: {
    node_id: string;
    port_id?: string;  // output 名称
  };
  target: {
    node_id: string;
    port_id?: string;  // input 参数名称
  };
  expression?: string;  // 完整表达式，如 "module.A.arn"
}
```

### Node 扩展 - 添加 outputs 信息

```typescript
interface ManifestNode {
  // ... 现有字段
  outputs?: ModuleOutput[];  // Module 的 outputs 定义
}

interface ModuleOutput {
  name: string;
  type: string;
  description?: string;
}
```

## 实现方案

### 1. 变量引用选择器组件

创建 `ModuleReferenceInput` 组件：
- 监听输入框中的 `/` 字符
- 弹出下拉菜单显示可用的 Module 节点
- 选择节点后显示该节点的 outputs
- 选择 output 后自动插入 `module.{instance_name}.{output_name}`

### 2. 连线自动创建

当用户通过选择器插入引用时：
- 自动创建 `variable_binding` 类型的 Edge
- source: 被引用的节点
- target: 当前节点
- expression: 完整的引用表达式

### 3. HCL 导入时解析引用

在后端 `parseHCLContent` 中：
- 扫描配置值中的 `module.xxx.xxx` 模式
- 记录引用关系
- 创建对应的 Edge

### 4. HCL 导出时保留引用

在 `generateHCL` 中：
- 检查配置值是否为引用表达式
- 如果是，直接输出表达式（不加引号）

## UI 交互流程

1. 用户在参数输入框中输入 `/`
2. 弹出模块选择下拉框，显示画布中**所有其他 Module 节点**（不限制必须先连线）
3. 用户选择一个节点（如 "module_a"）
4. 下拉框更新为该节点的 outputs 列表
5. 用户选择一个 output（如 "arn"）
6. 输入框自动填入 `module.module_a.arn`
7. **自动创建**从 module_a 到当前节点的 `variable_binding` 类型连线

## 设计决策

### 引用与连线的关系

**设计原则：先引用，后自动连线**

- 用户可以在表单中引用**任意其他 Module 节点**的 outputs，无需先手动连线
- 选择引用后，系统会**自动创建连线**（`variable_binding` 类型）
- 如果两个节点之间已有连线，会在现有连线上添加新的参数映射（bindings）

**实现方式：**

在 `ManifestEditor.tsx` 中，传递给 `ModuleFormRenderer` 的 `connectedNodeIds` 设置为 `undefined`：

```typescript
manifest={{
  currentNodeId: selectedNode.id,
  // 不限制 connectedNodeIds，允许引用任意节点
  // 选择引用后会自动创建连线（通过 onAddEdge 回调）
  connectedNodeIds: undefined,
  nodes: nodes.filter(...),
  onAddEdge: (sourceNodeId, targetNodeId, sourceOutput, targetInput) => {
    // 自动创建或更新连线
  },
}}
```

**连线类型区分：**

| 类型 | 颜色 | 创建方式 | 说明 |
|------|------|---------|------|
| `dependency` | 蓝色 (#1890ff) | 手动拖拽连线 | 表示依赖关系 |
| `variable_binding` | 绿色 (#52c41a) | 通过 `/` 引用自动创建 | 表示变量绑定 |

## 文件修改清单

1. `frontend/src/components/OpenAPIFormRenderer/widgets/TextWidget.tsx` - 添加引用选择功能
2. `frontend/src/components/ManifestEditor/ModuleReferencePopover.tsx` - 新建引用选择器组件
3. `frontend/src/pages/admin/ManifestEditor.tsx` - 传递节点信息到表单
4. `backend/internal/handlers/manifest_handler.go` - HCL 导入时解析引用
5. `frontend/src/services/manifestApi.ts` - 添加获取 Module outputs 的 API
