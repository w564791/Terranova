# Schema Management - JSON 编辑器功能设计

## 概述

在 Schema Management 页面添加 JSON 编辑功能，允许用户在"配置表单"和"配置JSON"两种模式之间切换，实现表单数据和 JSON 的双向转换。

## 需求背景

- **当前状态**：用户只能通过动态表单填写配置数据
- **用户需求**：希望能直接编辑 JSON 格式的配置数据
- **使用场景**：
  1. 高级用户更习惯直接编辑 JSON
  2. 批量修改配置时 JSON 更高效
  3. 复制粘贴配置更方便
  4. 可以快速查看完整的配置结构

## 功能设计

### 1. Tab 切换 UI

#### 布局
```
┌─────────────────────────────────────────────┐
│  配置表单  │  配置JSON                       │
├─────────────────────────────────────────────┤
│                                             │
│  [当前激活的 Tab 内容]                       │
│                                             │
└─────────────────────────────────────────────┘
```

#### Tab 状态
- 使用 React state 管理：`activeTab: 'form' | 'json'`
- 默认显示"配置表单" Tab
- 点击 Tab 切换内容区域

### 2. 配置表单 Tab（现有功能）

**显示内容**：
- DynamicForm 组件（现有）
- 表单字段根据 Schema 动态生成
- 数据存储在 `formValues` 状态

**功能**：
- 填写表单字段
- 实时验证
- 预览配置
- 生成配置

### 3. 配置JSON Tab（新功能）

**显示内容**：
- JSON 编辑器（使用 `<textarea>` 或第三方库）
- 格式化的 JSON 文本
- 实时语法验证
- 错误提示

**功能**：
- 显示 `formValues` 的 JSON 格式
- 编辑 JSON 内容
- 自动格式化
- 语法错误提示

## 技术实现

### 1. 数据流

```
┌──────────────┐
│  formValues  │ ← 共享状态
└──────┬───────┘
       │
   ┌───┴────┐
   │        │
   ▼        ▼
┌─────┐  ┌──────┐
│表单 │  │ JSON │
└─────┘  └──────┘
```

### 2. 表单 → JSON 转换

**实现方式**：
```typescript
// 直接使用 formValues 状态
const jsonString = JSON.stringify(formValues, null, 2);
```

**特点**：
- 实时转换
- 自动格式化（2空格缩进）
- 保留数据类型

### 3. JSON → 表单转换

**实现方式**：
```typescript
try {
  const parsed = JSON.parse(jsonString);
  setFormValues(parsed);
} catch (error) {
  // 显示错误提示
  showToast('JSON 格式错误', 'error');
}
```

**验证规则**：
- JSON 格式必须有效
- 数据结构必须符合 Schema 定义
- 字段类型必须匹配

### 4. 双向同步机制

#### 切换到 JSON Tab
1. 读取当前 `formValues`
2. 转换为格式化的 JSON 字符串
3. 显示在编辑器中

#### 切换到表单 Tab
1. 尝试解析 JSON 编辑器中的内容
2. 如果格式有效，更新 `formValues`
3. 如果格式无效，显示错误，保持在 JSON Tab

#### 实时同步
- 表单修改 → 不自动更新 JSON（避免干扰编辑）
- JSON 修改 → 不自动更新表单（避免干扰编辑）
- 只在切换 Tab 时同步数据

### 5. JSON 编辑器选择

#### 方案 A：使用 `<textarea>`（推荐）
**优点**：
- 无需额外依赖
- 轻量级
- 易于实现

**缺点**：
- 无语法高亮
- 无自动补全

**实现**：
```tsx
<textarea
  value={jsonString}
  onChange={(e) => setJsonString(e.target.value)}
  className={styles.jsonEditor}
  spellCheck={false}
/>
```

#### 方案 B：使用 Monaco Editor
**优点**：
- 语法高亮
- 自动补全
- 错误提示
- 专业编辑体验

**缺点**：
- 需要额外依赖（~2MB）
- 配置较复杂

**实现**：
```tsx
import Editor from '@monaco-editor/react';

<Editor
  height="600px"
  language="json"
  value={jsonString}
  onChange={(value) => setJsonString(value || '')}
  options={{
    minimap: { enabled: false },
    fontSize: 14,
  }}
/>
```

**推荐**：先使用方案 A（textarea），如果用户需要更好的编辑体验，再升级到方案 B。

## UI 设计

### Tab 样式

```css
.tabs {
  display: flex;
  border-bottom: 2px solid #e5e7eb;
  margin-bottom: 24px;
}

.tab {
  padding: 12px 24px;
  border: none;
  background: none;
  cursor: pointer;
  font-size: 14px;
  font-weight: 500;
  color: #6b7280;
  border-bottom: 2px solid transparent;
  margin-bottom: -2px;
  transition: all 0.2s;
}

.tab:hover {
  color: #3b82f6;
}

.tab.active {
  color: #3b82f6;
  border-bottom-color: #3b82f6;
}
```

### JSON 编辑器样式

```css
.jsonEditor {
  width: 100%;
  min-height: 600px;
  padding: 16px;
  border: 1px solid #d1d5db;
  border-radius: 8px;
  font-family: 'Monaco', 'Menlo', 'Courier New', monospace;
  font-size: 14px;
  line-height: 1.6;
  resize: vertical;
}

.jsonEditor:focus {
  outline: none;
  border-color: #3b82f6;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
}

.jsonError {
  color: #ef4444;
  font-size: 13px;
  margin-top: 8px;
}
```

## 实现步骤

### 步骤 1：添加状态管理
```typescript
const [activeTab, setActiveTab] = useState<'form' | 'json'>('form');
const [jsonString, setJsonString] = useState('');
const [jsonError, setJsonError] = useState<string | null>(null);
```

### 步骤 2：实现 Tab 切换逻辑
```typescript
const handleTabChange = (tab: 'form' | 'json') => {
  if (tab === 'json' && activeTab === 'form') {
    // 切换到 JSON：将表单数据转换为 JSON
    setJsonString(JSON.stringify(formValues, null, 2));
    setJsonError(null);
  } else if (tab === 'form' && activeTab === 'json') {
    // 切换到表单：解析 JSON 并更新表单
    try {
      const parsed = JSON.parse(jsonString);
      setFormValues(parsed);
      setJsonError(null);
    } catch (error) {
      setJsonError('JSON 格式错误，请修正后再切换');
      return; // 阻止切换
    }
  }
  setActiveTab(tab);
};
```

### 步骤 3：渲染 Tab UI
```tsx
<div className={styles.tabs}>
  <button
    className={`${styles.tab} ${activeTab === 'form' ? styles.active : ''}`}
    onClick={() => handleTabChange('form')}
  >
    配置表单
  </button>
  <button
    className={`${styles.tab} ${activeTab === 'json' ? styles.active : ''}`}
    onClick={() => handleTabChange('json')}
  >
    配置JSON
  </button>
</div>

<div className={styles.tabContent}>
  {activeTab === 'form' && (
    <DynamicForm
      schema={activeSchema.schema_data}
      values={formValues}
      onChange={setFormValues}
    />
  )}
  
  {activeTab === 'json' && (
    <div className={styles.jsonEditorContainer}>
      <textarea
        value={jsonString}
        onChange={(e) => setJsonString(e.target.value)}
        className={styles.jsonEditor}
        spellCheck={false}
        placeholder="在此编辑 JSON 配置..."
      />
      {jsonError && (
        <div className={styles.jsonError}>{jsonError}</div>
      )}
    </div>
  )}
</div>
```

### 步骤 4：添加工具按钮（可选）

在 JSON Tab 中添加辅助功能：
- **格式化按钮**：美化 JSON 格式
- **压缩按钮**：移除空格和换行
- **验证按钮**：检查 JSON 格式
- **复制按钮**：复制 JSON 到剪贴板

```tsx
<div className={styles.jsonToolbar}>
  <button onClick={formatJSON}>格式化</button>
  <button onClick={validateJSON}>验证</button>
  <button onClick={copyJSON}>复制</button>
</div>
```

## 错误处理

### 1. JSON 格式错误
- **检测时机**：切换到表单 Tab 时
- **处理方式**：显示错误提示，阻止切换
- **错误信息**：显示具体的语法错误位置

### 2. 数据类型不匹配
- **检测时机**：解析 JSON 后
- **处理方式**：显示警告，但允许切换
- **错误信息**：提示哪些字段类型不匹配

### 3. 必填字段缺失
- **检测时机**：提交配置时
- **处理方式**：表单验证
- **错误信息**：高亮缺失的字段

## 用户体验优化

### 1. 数据同步提示
- 切换 Tab 时显示"正在同步数据..."
- 同步成功后显示"数据已同步"
- 同步失败时显示错误信息

### 2. 未保存提示
- 如果 JSON 有修改但未切换到表单
- 离开页面时提示"有未保存的修改"

### 3. 快捷键支持
- `Ctrl/Cmd + S`：保存（格式化 JSON）
- `Ctrl/Cmd + F`：在 JSON 中查找
- `Tab`：在 JSON 编辑器中插入缩进

## 测试场景

### 场景 1：表单 → JSON
1. 在表单中填写数据
2. 点击"配置JSON" Tab
3. 验证：JSON 正确显示表单数据

### 场景 2：JSON → 表单
1. 在 JSON 编辑器中修改数据
2. 点击"配置表单" Tab
3. 验证：表单正确显示 JSON 数据

### 场景 3：JSON 格式错误
1. 在 JSON 编辑器中输入无效 JSON
2. 点击"配置表单" Tab
3. 验证：显示错误提示，阻止切换

### 场景 4：数据持久化
1. 在 JSON Tab 修改数据
2. 切换到表单 Tab
3. 点击"生成配置"
4. 验证：使用最新的数据生成配置

## 技术细节

### 状态管理
```typescript
interface SchemaManagementState {
  activeTab: 'form' | 'json';           // 当前激活的 Tab
  formValues: Record<string, any>;      // 表单数据（共享）
  jsonString: string;                   // JSON 字符串
  jsonError: string | null;             // JSON 错误信息
  isDirty: boolean;                     // 是否有未保存的修改
}
```

### 数据转换函数

#### formToJSON
```typescript
const formToJSON = (values: Record<string, any>): string => {
  return JSON.stringify(values, null, 2);
};
```

#### jsonToForm
```typescript
const jsonToForm = (jsonStr: string): Record<string, any> => {
  try {
    return JSON.parse(jsonStr);
  } catch (error) {
    throw new Error(`JSON 解析失败: ${error.message}`);
  }
};
```

#### validateJSON
```typescript
const validateJSON = (jsonStr: string): { valid: boolean; error?: string } => {
  try {
    JSON.parse(jsonStr);
    return { valid: true };
  } catch (error) {
    return { 
      valid: false, 
      error: `第 ${error.lineNumber} 行: ${error.message}` 
    };
  }
};
```

## 实现优先级

### P0（必须实现）
-  Tab 切换 UI
-  表单 → JSON 转换
-  JSON → 表单转换
-  JSON 格式验证
-  错误提示

### P1（建议实现）
- ⭕ 格式化按钮
- ⭕ 复制按钮
- ⭕ 语法高亮（使用 textarea）

### P2（可选实现）
- ⭕ Monaco Editor 集成
- ⭕ 快捷键支持
- ⭕ 未保存提示
- ⭕ 自动保存

## 兼容性考虑

### 现有功能不受影响
-  Schema 导入功能正常
-  AI 生成 Schema 功能正常
-  Schema 编辑功能正常
-  预览功能正常
-  生成配置功能正常

### 数据一致性
-  两个 Tab 共享同一个 `formValues` 状态
-  切换 Tab 时自动同步数据
-  提交时使用最新的 `formValues`

## 文件修改清单

### 需要修改的文件
1. `frontend/src/pages/SchemaManagement.tsx`
   - 添加 Tab 状态管理
   - 添加 Tab 切换逻辑
   - 添加 JSON 编辑器组件
   - 添加数据转换函数

2. `frontend/src/pages/SchemaManagement.module.css`
   - 添加 Tab 样式
   - 添加 JSON 编辑器样式
   - 添加错误提示样式

### 不需要修改的文件
- ❌ 后端 API（无需修改）
- ❌ DynamicForm 组件（无需修改）
- ❌ Schema 数据结构（无需修改）

## 实现时间估算

- Tab UI 实现：30 分钟
- 数据转换逻辑：30 分钟
- 错误处理：20 分钟
- 样式调整：20 分钟
- 测试验证：20 分钟

**总计**：约 2 小时

## 风险和注意事项

### 风险
1. **JSON 格式错误**：用户可能输入无效 JSON
   - 缓解：切换前验证，显示错误提示

2. **数据类型不匹配**：JSON 中的类型可能与 Schema 不符
   - 缓解：表单组件会自动验证类型

3. **大型 JSON 性能**：非常大的配置可能导致卡顿
   - 缓解：使用 textarea 而非富文本编辑器

### 注意事项
1. 保持 `formValues` 作为唯一数据源
2. JSON 编辑器只是另一种编辑方式
3. 最终提交时使用 `formValues`
4. 不要在两个 Tab 之间自动同步（避免数据丢失）

## 后续优化

### 短期优化
1. 添加 JSON 格式化按钮
2. 添加复制到剪贴板功能
3. 改进错误提示的可读性

### 长期优化
1. 集成 Monaco Editor（更好的编辑体验）
2. 添加 JSON Schema 验证
3. 支持导入/导出 JSON 文件
4. 添加历史记录功能
5. 支持 JSON 差异对比

## 总结

这是一个纯前端功能，通过添加 Tab 切换和 JSON 编辑器，为用户提供两种配置编辑方式：
- **表单模式**：适合普通用户，直观易用
- **JSON 模式**：适合高级用户，灵活高效

两种模式共享同一个数据源（`formValues`），确保数据一致性。
