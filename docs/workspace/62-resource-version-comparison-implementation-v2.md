# 资源版本对比功能实现总结 V2

## 概述
重新设计并实现了资源版本历史查看和版本对比功能，采用页面内tab切换的方式，提供更流畅的用户体验。

## 设计变更

### V1 → V2 主要改进
1. **移除弹窗设计** - 改为页面内tab切换（当前版本/历史版本/版本对比）
2. **修复查看历史版本** - 确保API调用正确，数据提取逻辑完善
3. **优化差异对比展示** - 参考Terraform风格：
   - 未更改字段在原位置显示折叠提示
   - 点击展开时字段位置不变
   - 显示"··· 1 unchanged attribute hidden"提示

## 实现的功能

### 1. 三个视图模式（Tab切换）

#### 当前版本
- 显示资源基本信息（资源ID、创建时间、更新时间、变更摘要）
- 显示当前版本配置（支持表单/JSON视图切换）
- 使用FormPreview组件渲染

#### 历史版本
- 列表显示所有历史版本
- 每个版本显示：版本号、Latest标识、创建时间、变更摘要
- 点击"查看"按钮加载该版本配置
- 在列表下方显示选中版本的配置（支持表单/JSON视图）

#### 版本对比
- 提供From/To版本选择器
- 实时对比两个版本的差异
- 差异展示特性：
  - **新增字段**：绿色背景，显示"added"标签
  - **删除字段**：红色背景，显示"removed"标签
  - **修改字段**：黄色背景，显示"modified"标签，显示"~"符号
  - **未更改字段**：白色背景，默认折叠显示"··· 1 unchanged attribute hidden"
  - 点击未更改字段可展开/折叠，**位置始终保持不变**

### 2. 数据流程

```typescript
// 查看历史版本
1. 用户切换到"历史版本"tab
2. 自动加载版本列表（如果未加载）
3. 用户点击某个版本的"查看"按钮
4. 调用API: GET /workspaces/:id/resources/:resource_id/versions/:version
5. 提取tf_code.module配置
6. 更新versionData状态
7. FormPreview渲染该版本配置

// 版本对比
1. 用户切换到"版本对比"tab
2. 自动加载版本列表（如果未加载）
3. 用户选择From版本和To版本
4. 并行调用两个版本的API
5. 提取两个版本的module配置
6. 计算差异（calculateDiff函数）
7. 渲染差异列表（未更改字段在原位置折叠）
```

### 3. 差异计算逻辑

```typescript
const calculateDiff = (oldConfig: any, newConfig: any): DiffField[] => {
  const fields: DiffField[] = [];
  const allKeys = new Set([...Object.keys(oldConfig), ...Object.keys(newConfig)]);
  
  allKeys.forEach(key => {
    const oldValue = oldConfig[key];
    const newValue = newConfig[key];
    
    const oldExists = key in oldConfig;
    const newExists = key in newConfig;
    
    if (!oldExists && newExists) {
      // 新增字段
      fields.push({ field: key, type: 'added', newValue, expanded: false });
    } else if (oldExists && !newExists) {
      // 删除字段
      fields.push({ field: key, type: 'removed', oldValue, expanded: false });
    } else if (JSON.stringify(oldValue) !== JSON.stringify(newValue)) {
      // 修改字段
      fields.push({ field: key, type: 'modified', oldValue, newValue, expanded: false });
    } else {
      // 未更改字段
      fields.push({ field: key, type: 'unchanged', oldValue, newValue, expanded: false });
    }
  });
  
  return fields;
};
```

## 用户界面设计

### Tab切换
```
[当前版本] [历史版本] [版本对比]
```

### 差异对比展示（参考Terraform风格）

```
┌─────────────────────────────────────────────────────┐
│ id: "sgr-07c3b81b56ba4d449"                         │ ← 未更改字段（白色）
│ ▶ ··· 1 unchanged attribute hidden                  │ ← 折叠提示，可点击展开
└─────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────┐
│ ~ referenced_security_group_id:              [modified] │ ← 修改字段（黄色）
│   "57390226123/sg-0fb1c85d82e83ab32" →              │
│   "sg-0fb1c85d82e83ab32"                            │
└─────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────┐
│ tags: {                                              │ ← 未更改字段（白色）
│ ▼ ··· 1 unchanged attribute hidden                  │ ← 展开状态
│   { ... }                                            │ ← 显示完整内容
└─────────────────────────────────────────────────────┘
```

### 关键特性
1. **字段位置固定** - 所有字段按原始顺序排列，展开/折叠不改变位置
2. **视觉区分** - 使用背景色区分不同类型的变更
3. **折叠提示** - 未更改字段显示"··· 1 unchanged attribute hidden"
4. **交互友好** - 只有未更改字段可点击展开/折叠

## 技术实现

### 状态管理
```typescript
const [viewMode, setViewMode] = useState<ViewMode>('current');
const [dataViewMode, setDataViewMode] = useState<DataViewMode>('form');
const [versions, setVersions] = useState<Version[]>([]);
const [selectedVersion, setSelectedVersion] = useState<number | null>(null);
const [compareFromVersion, setCompareFromVersion] = useState<number | null>(null);
const [compareToVersion, setCompareToVersion] = useState<number | null>(null);
const [diffFields, setDiffFields] = useState<DiffField[]>([]);
const [versionData, setVersionData] = useState<any>({});
```

### 核心函数
- `loadVersions()` - 加载版本列表
- `extractModuleConfig()` - 从tf_code提取module配置
- `handleViewVersion()` - 查看指定版本
- `calculateDiff()` - 计算两个版本的差异
- `handleCompareVersions()` - 对比两个版本
- `toggleFieldExpansion()` - 切换字段展开/折叠状态

### API端点
```
GET /api/v1/workspaces/:id/resources/:resource_id/versions
GET /api/v1/workspaces/:id/resources/:resource_id/versions/:version
```

## 文件清单

### 修改文件
1. `frontend/src/pages/ViewResource.tsx` - 完全重写，实现新的tab切换设计

### 移除依赖
- 不再使用ResourceVersionHistory组件（弹窗）
- 不再使用ResourceVersionDiff组件（弹窗）
- 所有功能集成在ViewResource页面内

## 样式设计

### 颜色方案
```css
/* 新增字段 */
background: var(--color-green-50);
color: var(--color-green-700);

/* 删除字段 */
background: var(--color-red-50);
color: var(--color-red-700);

/* 修改字段 */
background: var(--color-yellow-50);
color: var(--color-yellow-700);

/* 未更改字段 */
background: white;
color: var(--color-gray-700);

/* 折叠提示 */
color: var(--color-gray-500);
```

### 布局
- 使用现有的AddResources.module.css样式
- 内联样式处理特殊布局需求
- 响应式设计，适配不同屏幕尺寸

## 测试建议

### 功能测试
1.  Tab切换（当前版本/历史版本/版本对比）
2.  历史版本列表加载
3.  查看历史版本配置
4.  版本对比选择器
5.  差异计算和展示
6.  未更改字段折叠/展开
7.  字段位置保持不变

### 边界情况
1. 只有一个版本的资源
2. 版本间无差异
3. 版本间所有字段都变更
4. 复杂嵌套对象的对比
5. 大量字段的性能

### UI/UX测试
1. Tab切换流畅性
2. 版本列表滚动
3. 差异列表滚动
4. 长文本显示
5. 移动端适配

## 后续优化建议

### 功能增强
1. 支持版本回滚
2. 支持版本标签/备注
3. 支持版本搜索和过滤
4. 支持导出对比报告
5. 支持并排对比视图

### 性能优化
1. 版本列表虚拟滚动
2. 差异计算优化
3. 大文件处理优化

### 用户体验
1. 添加键盘快捷键
2. 记住用户的视图偏好
3. 添加加载动画
4. 优化错误提示

## 总结

V2版本完全重新设计了用户界面，采用页面内tab切换的方式，提供了更流畅和直观的用户体验。差异对比功能参考Terraform风格，未更改字段在原位置折叠显示，点击展开时位置不变，符合用户的使用习惯。

### 完成情况
-  移除弹窗设计
-  实现tab切换
-  修复查看历史版本功能
-  优化差异对比展示
-  字段位置固定不变
-  TypeScript类型检查通过
- ⏳ 功能测试（待进行）

### 下一步
建议进行完整的功能测试，验证所有用户场景是否正常工作。
