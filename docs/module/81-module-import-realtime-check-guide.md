# Module导入实时名称检查功能指南

## 📋 功能概述

在Module导入页面，当用户输入模块名称时，系统会自动检查该名称是否已存在，并实时显示检查结果，避免用户提交后才发现名称冲突。

**实现日期**: 2025-09-30  
**相关页面**: `/modules/import`  
**涉及文件**:
- `frontend/src/pages/ImportModule.tsx`
- `frontend/src/pages/ImportModule.module.css`

## ✨ 功能特性

### 1. 实时检查
- 用户输入模块名称后500ms自动触发检查
- 防抖机制避免频繁请求
- 异步检查不阻塞用户操作

### 2. 智能匹配
- 检查`name + provider`的组合是否存在
- 切换提供商时自动重新检查
- 精确匹配，避免误判

### 3. 视觉反馈
- ⏳ **检查中** - 显示旋转动画
- ✓ **可用** - 绿色边框 + 绿色提示
- ✗ **已存在** - 红色边框 + 红色提示

## 🎯 用户体验

### 场景1：名称可用
```
用户输入: "new-module-001"
系统检查: 不存在
显示: ✓ 模块名称可用 (绿色)
用户操作: 继续填写其他信息
```

### 场景2：名称已存在
```
用户输入: "existing-module"
系统检查: 已存在
显示: ✗ 该模块名称已存在，请使用其他名称 (红色)
用户操作: 修改名称为 "existing-module-v2"
系统重新检查: 不存在
显示: ✓ 模块名称可用 (绿色)
```

### 场景3：切换提供商
```
用户输入: "test-module" (AWS)
显示: ✗ 已存在
用户切换: AWS → Azure
系统重新检查: "test-module" + "Azure"
显示: ✓ 模块名称可用 (绿色)
```

## 💻 技术实现

### 状态管理

```typescript
// 检查状态类型
type CheckStatus = 'idle' | 'checking' | 'available' | 'exists';

// 状态变量
const [nameCheckStatus, setNameCheckStatus] = useState<CheckStatus>('idle');
const [checkTimeout, setCheckTimeout] = useState<number | null>(null);
```

### 防抖机制

```typescript
const handleModuleNameChange = (name: string) => {
  setModuleName(name);
  
  // 清除之前的定时器
  if (checkTimeout) {
    clearTimeout(checkTimeout);
  }
  
  // 500ms后触发检查
  const timeout = window.setTimeout(() => {
    checkModuleName(name, provider);
  }, 500);
  
  setCheckTimeout(timeout);
};
```

### 检查逻辑

```typescript
const checkModuleName = async (name: string, prov: string) => {
  if (!name.trim()) {
    setNameCheckStatus('idle');
    return;
  }

  try {
    setNameCheckStatus('checking');
    const response = await moduleService.getModules();
    
    // 检查是否存在完全匹配的模块（name + provider）
    const responseData: any = response.data;
    const modules = Array.isArray(responseData) ? responseData : (responseData.items || []);
    const exists = modules.some(
      (m: any) => m.name === name && m.provider === prov
    );
    
    setNameCheckStatus(exists ? 'exists' : 'available');
  } catch (err) {
    // 如果查询失败，不影响用户继续操作
    setNameCheckStatus('idle');
  }
};
```

### UI渲染

```tsx
<div className={styles.inputWrapper}>
  <input
    type="text"
    value={moduleName}
    onChange={(e) => handleModuleNameChange(e.target.value)}
    className={`${styles.input} ${
      nameCheckStatus === 'exists' ? styles.inputError : 
      nameCheckStatus === 'available' ? styles.inputSuccess : ''
    }`}
    placeholder="例如: s3-bucket"
    required
  />
  {nameCheckStatus === 'checking' && (
    <span className={styles.checkingIcon}>⏳</span>
  )}
  {nameCheckStatus === 'available' && (
    <span className={styles.availableIcon}>✓</span>
  )}
  {nameCheckStatus === 'exists' && (
    <span className={styles.existsIcon}>✗</span>
  )}
</div>
{nameCheckStatus === 'exists' && (
  <p className={styles.errorHint}>
    该模块名称已存在，请使用其他名称
  </p>
)}
{nameCheckStatus === 'available' && (
  <p className={styles.successHint}>
    模块名称可用
  </p>
)}
```

## 🎨 CSS样式

### 输入框状态

```css
.inputWrapper {
  position: relative;
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.inputWrapper .input {
  flex: 1;
}

.inputError {
  border-color: var(--color-red-500) !important;
}

.inputSuccess {
  border-color: var(--color-green-500) !important;
}
```

### 图标样式

```css
.checkingIcon,
.availableIcon,
.existsIcon {
  font-size: 18px;
  min-width: 24px;
  text-align: center;
}

.checkingIcon {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

.availableIcon {
  color: var(--color-green-500);
  font-weight: bold;
}

.existsIcon {
  color: var(--color-red-500);
  font-weight: bold;
}
```

### 提示文本

```css
.errorHint {
  margin: 4px 0 0 0;
  font-size: var(--font-size-sm);
  color: var(--color-red-600);
}

.successHint {
  margin: 4px 0 0 0;
  font-size: var(--font-size-sm);
  color: var(--color-green-600);
}
```

## 🔧 错误处理

### 网络错误
- 检查失败不影响用户继续操作
- 状态重置为`idle`
- 用户仍可提交（后端会再次验证）

### 性能优化
- 500ms防抖避免频繁请求
- 异步检查不阻塞UI
- 只在必要时触发检查

## 📊 API调用

### 获取模块列表
```typescript
GET /api/v1/modules
Response: {
  code: 200,
  data: [
    {
      id: 1,
      name: "module-name",
      provider: "AWS",
      ...
    }
  ]
}
```

### 检查逻辑
```typescript
// 检查是否存在匹配的模块
const exists = modules.some(
  m => m.name === inputName && m.provider === selectedProvider
);
```

## 🎯 优势

### 用户体验
-  即时反馈，无需等待提交
-  清晰的视觉提示
-  避免无效提交
-  提升用户体验

### 技术优势
-  防抖机制减少服务器压力
-  异步检查不阻塞UI
-  错误处理完善
-  代码可维护性高

## 🚀 使用指南

### 用户操作流程

1. **访问导入页面**
   ```
   http://localhost:5173/modules/import
   ```

2. **输入模块名称**
   - 在"模块名称"字段输入名称
   - 等待500ms后自动检查

3. **查看检查结果**
   - ⏳ 检查中 - 等待结果
   - ✓ 可用 - 可以继续
   - ✗ 已存在 - 需要修改

4. **切换提供商（可选）**
   - 选择不同的提供商
   - 系统自动重新检查

5. **继续填写表单**
   - 确认名称可用后
   - 填写其他必填信息
   - 点击"导入模块"

### 开发者集成

如果需要在其他页面实现类似功能：

1. **复制状态管理代码**
   ```typescript
   const [nameCheckStatus, setNameCheckStatus] = useState<'idle' | 'checking' | 'available' | 'exists'>('idle');
   const [checkTimeout, setCheckTimeout] = useState<number | null>(null);
   ```

2. **复制检查函数**
   ```typescript
   const checkModuleName = async (name: string, prov: string) => {
     // ... 检查逻辑
   };
   
   const handleModuleNameChange = (name: string) => {
     // ... 防抖逻辑
   };
   ```

3. **复制UI组件**
   ```tsx
   <div className={styles.inputWrapper}>
     {/* ... 输入框和图标 */}
   </div>
   ```

4. **复制CSS样式**
   ```css
   .inputWrapper { /* ... */ }
   .inputError { /* ... */ }
   .inputSuccess { /* ... */ }
   ```

## 📝 注意事项

### 性能考虑
- 防抖时间设置为500ms，平衡用户体验和服务器压力
- 只在名称或提供商变化时触发检查
- 检查失败不影响用户继续操作

### 用户体验
- 提供清晰的视觉反馈
- 错误提示友好且具体
- 不阻塞用户的其他操作

### 安全性
- 前端检查仅用于提升用户体验
- 后端仍需进行完整的验证
- 数据库约束是最终保障

## 🔄 未来优化

### 可能的改进
1. **缓存机制** - 缓存已检查的名称，减少重复请求
2. **建议名称** - 当名称已存在时，自动建议可用的替代名称
3. **批量检查** - 支持一次检查多个名称
4. **历史记录** - 显示用户最近使用的模块名称

### 扩展功能
1. **名称规则验证** - 检查名称是否符合命名规范
2. **相似度检查** - 提示与现有名称相似的模块
3. **智能补全** - 根据输入提供名称建议

## 📚 相关文档

- [Module导入功能指南](./schema-import-capability-4-guide.md)
- [项目状态文档](./project-status.md)
- [开发指南](./development-guide.md)
- [API规范](./api-specification.md)

---

**最后更新**: 2025-09-30  
**功能状态**:  已完成并可用
