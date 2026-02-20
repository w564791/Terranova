# 前端表单风格规范

## 概述
本文档定义了IAC Platform前端表单的统一设计规范和实现标准，确保所有表单组件在视觉和交互上保持一致性。

## 设计原则

### 1. 视觉层次
- **色彩系统**：统一使用灰色系（gray-50 到 gray-900）
- **背景层次**：
  - 顶层容器：`background: white`
  - 嵌套容器：`background: var(--color-gray-50)`
  - 内部元素：`background: white`
- **边框样式**：`border: 1px solid var(--color-gray-200)`
- **圆角规范**：`border-radius: var(--radius-md)`

### 2. 间距系统
```css
--spacing-xs: 4px;
--spacing-sm: 8px;
--spacing-md: 16px;
--spacing-lg: 24px;
```

## 组件规范

### 1. 输入框（Input）
```css
.input {
  padding: var(--spacing-sm);
  border: 1px solid var(--color-gray-300);
  border-radius: var(--radius-md);
  font-size: 14px;
  transition: border-color 0.2s;
}

.input:focus {
  outline: none;
  border-color: var(--color-blue-500);
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
}
```

### 2. 数字输入框（Number Input）
**特殊处理**：
- 隐藏浏览器默认箭头
- 添加自定义增减按钮

```css
/* 隐藏默认箭头 */
.numberInput {
  -webkit-appearance: textfield;
  -moz-appearance: textfield;
  appearance: textfield;
}

.numberInput::-webkit-inner-spin-button,
.numberInput::-webkit-outer-spin-button {
  -webkit-appearance: none;
  margin: 0;
}

/* 自定义箭头 */
.numberControls {
  position: absolute;
  right: 1px;
  top: 1px;
  bottom: 1px;
  width: 20px;
  display: flex;
  flex-direction: column;
}
```

### 3. 布尔开关（Boolean Switch）
**设计规范**：
- 尺寸：44px × 24px
- 颜色：启用 `#10b981`，禁用 `#d1d5db`
- 动画：`transition: all 0.3s`

```css
.switchButton {
  width: 44px;
  height: 24px;
  border-radius: 12px;
  position: relative;
  cursor: pointer;
}

.switchThumb {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: white;
  transition: transform 0.3s;
}
```

### 4. 可搜索选择框（Searchable Select）
**功能特性**：
- 支持实时搜索
- 不区分大小写
- 高亮选中项

```css
.selectDropdown {
  position: absolute;
  background: white;
  border: 1px solid var(--color-gray-300);
  border-radius: var(--radius-md);
  box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
  z-index: 1000;
}
```

### 5. 数组字段（Array Field）
**视觉规范**：
- 容器背景：`var(--color-gray-50)`
- 数组项背景：`white`
- 连接线：左侧6px横线
- 缩进：`margin-left: var(--spacing-sm)`

### 6. Map字段（Key-Value Pairs）
**交互规范**：
- 必填键：背景 `#fef2f2`，不可编辑键名
- 普通键：可自由添加/删除
- 删除按钮：仅在非必填键显示

### 7. 嵌套Object字段
**高级功能设计**：
- 基础字段：`required === true && !hidden_default`
- 高级字段：`hidden_default || required !== true`
- 添加按钮：虚线边框，灰色背景

```css
.propSelector {
  padding: var(--spacing-sm);
  border: 1px dashed var(--color-gray-300);
  border-radius: var(--radius-sm);
  text-align: center;
}
```

**选择器面板**：
```css
.propSelectorPanel {
  background: white;
  border: 1px solid var(--color-gray-300);
  border-radius: var(--radius-md);
  padding: 0;
  overflow: hidden;
}

/* 搜索框贯穿整个面板 */
.searchBox {
  padding: 0 var(--spacing-md);
  width: 100%;
}
```

## 交互规范

### 1. 焦点状态
- 边框颜色：`var(--color-blue-500)`
- 阴影：`0 0 0 3px rgba(59, 130, 246, 0.1)`

### 2. 悬停状态
- 背景变化：增加10%明度
- 边框加深：使用更深的灰色
- 过渡动画：`transition: all 0.2s`

### 3. 禁用状态
- 背景：`#f3f4f6`
- 文字：`var(--color-gray-400)`
- 鼠标：`cursor: not-allowed`

### 4. 错误状态
- 边框：`#ef4444`
- 背景：`#fef2f2`（仅必填项）
- 提示文字：`var(--color-red-500)`

## 按钮规范

### 1. 主要按钮（Primary）
```css
.addButton {
  background: var(--color-blue-500);
  color: white;
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
  width: 100%;
}
```

### 2. 删除按钮（Danger）
```css
.removeButton {
  background: var(--color-red-500);
  color: white;
  padding: 2px 8px;
  border-radius: var(--radius-sm);
  font-size: 12px;
}
```

### 3. 取消按钮（Secondary）
```css
.cancelButton {
  background: var(--color-gray-100);
  color: var(--color-gray-700);
  border: 1px solid var(--color-gray-300);
  padding: 4px 12px;
  border-radius: var(--radius-sm);
}
```

## 特殊标识

### 1. 必填标识
- 符号：`*`
- 颜色：`var(--color-red-500)`
- 位置：紧跟字段名称

### 2. Force New警告
- 图标：``
- 颜色：`#ff9800`
- 提示：悬停显示"修改此字段将强制重建资源"

### 3. Must Include提示
- 背景：`#fef2f2`
- 文字：`var(--color-red-500)`
- 样式：`font-weight: 500`

## 响应式设计

### 移动端适配
- 最小宽度：320px
- 触摸目标：最小44px × 44px
- 间距调整：移动端使用`--spacing-sm`

### 深色模式（预留）
- 背景反转
- 边框使用更深的颜色
- 文字对比度调整

## 性能优化

### 1. CSS优化
- 使用CSS变量统一管理
- 避免深层嵌套选择器
- 使用`will-change`优化动画

### 2. 交互优化
- 防抖搜索：300ms
- 虚拟滚动：超过50项
- 懒加载：高级选项按需加载

## 可访问性

### 1. ARIA属性
- `aria-pressed`：布尔开关状态
- `aria-required`：必填字段
- `aria-invalid`：错误状态

### 2. 键盘导航
- Tab：字段间切换
- Space：激活按钮/开关
- Escape：关闭下拉菜单

### 3. 屏幕阅读器
- 使用语义化HTML
- 提供描述性标签
- 错误信息关联到字段

## 实施示例

### 完整表单字段实现
```tsx
<div className={styles.field}>
  <label className={styles.label}>
    {name}
    {required && <span className={styles.required}>*</span>}
    {force_new && <span className={styles.forceNew}></span>}
  </label>
  {renderInput()}
  {description && (
    <div className={styles.description}>{description}</div>
  )}
  {error && <div className={styles.error}>{error}</div>}
</div>
```

## 版本历史

### v2.0.0 (2024-09)
- 添加嵌套Object高级功能支持
- 优化数字输入框箭头
- 改进选择器面板样式

### v1.0.0 (2024-08)
- 初始版本
- 基础表单组件
- 统一视觉规范

## 相关文档
- [组件开发指南](./development-guide.md)
- [动态Schema测试指南](./dynamic-schema-testing-guide.md)
- [前端调试指南](./frontend-debugging-guide.md)
