# Demo选择器集成到添加资源流程

## 功能概述

在添加资源（Add Resources）流程的"配置资源"步骤中，添加Demo选择功能，允许用户快速使用预定义的Demo配置来填充表单数据。

## 需求背景

用户在添加资源时，如果Module有预定义的Demo配置，可以直接选择Demo来快速填充表单，避免手动输入所有字段，提高效率。

## 功能位置

**页面**: `/workspaces/:id/resources/add`
**步骤**: 步骤2 - 配置资源
**位置**: "基于Module Schema自动生成的配置表单"文字的右侧

## UI设计

### 1. Demo选择器按钮

**默认状态**:
- 有Demo: 显示"可用Demo X个"（X为Demo数量）
- 无Demo: 显示"该module暂无可用Demo"（禁用状态）

**样式**:
```css
- padding: 8px 16px
- border: 1px solid #dee2e6
- border-radius: 6px
- background: white
- cursor: pointer
- font-size: 14px
- 带下拉箭头图标
```

### 2. 自定义下拉菜单

**下拉选项格式**（多行显示）:
```
┌─────────────────────────────────────┐
│ Demo名称                             │
│ 描述: Demo描述信息 | 更新: 2025/10/17│
├─────────────────────────────────────┤
│ 另一个Demo                           │
│ 描述: 另一个描述 | 更新: 2025/10/16  │
└─────────────────────────────────────┘
```

**样式规格**:
- Demo名称: 14px, 加粗, #111827
- 描述信息: 13px, 正常, #6B7280（浅灰色）
- 更新时间: 13px, 正常, #6B7280（浅灰色）
- 选项hover: 背景色 #F3F4F6
- 选项padding: 12px 16px
- 下拉菜单最大高度: 400px（超出滚动）

### 3. 确认对话框

**触发条件**: 用户已填写部分字段，选择Demo时弹出

**对话框内容**:
- 标题: "确认使用Demo配置"
- 消息: "选择Demo将覆盖当前已填写的表单数据，是否继续？"
- 确认按钮: "确认使用"
- 取消按钮: "取消"
- 类型: warning

## 技术实现

### 1. 组件结构

#### DemoSelector组件
```typescript
// frontend/src/components/DemoSelector.tsx
interface DemoSelectorProps {
  moduleId: number;
  onSelectDemo: (demoData: any) => void;
  hasFormData: boolean; // 是否已有表单数据
}
```

**功能**:
- 加载指定Module的Demo列表
- 显示自定义下拉菜单
- 处理Demo选择事件
- 管理下拉菜单的打开/关闭状态

#### DemoSelector.module.css
```css
.selectorButton { /* 按钮样式 */ }
.dropdown { /* 下拉菜单容器 */ }
.dropdownItem { /* 下拉选项 */ }
.demoName { /* Demo名称 */ }
.demoMeta { /* 描述和时间 */ }
```

### 2. 集成到AddResources页面

**文件**: `frontend/src/pages/AddResources.tsx`

**修改位置**: 步骤2的配置区域

**集成代码**:
```typescript
// 在"基于Module Schema自动生成的配置表单"文字右侧添加
<div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
  <p className={styles.stepDesc}>
    基于Module Schema自动生成的配置表单
  </p>
  
  {selectedModule && (
    <DemoSelector
      moduleId={selectedModule.id}
      onSelectDemo={handleSelectDemo}
      hasFormData={Object.keys(formData).length > 0}
    />
  )}
</div>
```

### 3. 数据流程

#### 加载Demo列表
```typescript
// 使用已有的API
const demos = await moduleDemoService.getDemosByModuleId(moduleId);
```

#### 选择Demo处理
```typescript
const handleSelectDemo = async (demoId: number) => {
  // 1. 检查是否有表单数据
  if (hasFormData) {
    // 显示确认对话框
    setShowDemoConfirmDialog(true);
    setPendingDemoId(demoId);
    return;
  }
  
  // 2. 直接应用Demo数据
  await applyDemoData(demoId);
};

const applyDemoData = async (demoId: number) => {
  // 1. 获取Demo详情
  const demo = await moduleDemoService.getDemoById(demoId);
  
  // 2. 填充表单数据
  setFormData(demo.current_version.config_data);
  
  // 3. 显示成功提示
  showToast(`已应用Demo "${demo.name}" 的配置`, 'success');
};
```

### 4. 状态管理

**新增状态**:
```typescript
const [demos, setDemos] = useState<ModuleDemo[]>([]);
const [showDemoConfirmDialog, setShowDemoConfirmDialog] = useState(false);
const [pendingDemoId, setPendingDemoId] = useState<number | null>(null);
```

## 实现步骤

### 步骤1: 创建DemoSelector组件
- [ ] 创建组件文件和样式文件
- [ ] 实现自定义下拉菜单UI
- [ ] 实现Demo列表加载
- [ ] 实现选择事件处理

### 步骤2: 集成到AddResources页面
- [ ] 在步骤2添加DemoSelector组件
- [ ] 实现handleSelectDemo函数
- [ ] 添加确认对话框
- [ ] 实现applyDemoData函数

### 步骤3: 测试
- [ ] 测试有Demo的情况
- [ ] 测试无Demo的情况
- [ ] 测试表单数据覆盖确认
- [ ] 测试Demo数据填充

## API依赖

使用已有的API:
- `GET /api/v1/modules/:id/demos` - 获取Module的Demo列表
- `GET /api/v1/demos/:id` - 获取Demo详情

## 注意事项

1. **性能优化**: Demo列表在选择Module后立即加载，避免点击时等待
2. **错误处理**: 加载失败时显示友好提示
3. **用户体验**: 
   - 下拉菜单点击外部自动关闭
   - 选择Demo后自动关闭下拉菜单
   - 加载状态显示
4. **数据验证**: 确保Demo的config_data与当前Schema兼容

## 开发文档完成

所有需求细节已明确，可以开始开发。
