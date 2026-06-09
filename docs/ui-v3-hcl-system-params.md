# HCL 编辑器系统参数与额外字段处理实现说明

## 实现时间
2026-06-09

## 功能概述

实现了 HCL 编辑器对 Terraform 系统参数的智能处理，以及对 Schema 未定义字段的检测和提示功能。

## 核心功能

### 1. 系统参数自动识别与分离

**识别的系统参数：**
- `source` - 模块来源
- `version` - 模块版本
- `for_each` - 循环创建
- `count` - 计数创建
- `depends_on` - 显式依赖
- `providers` - 提供者映射
- `lifecycle` - 生命周期配置

**处理方式：**
- 在 HCL → 表单转换时，系统参数被自动识别并分离到 `systemParams` 中
- 系统参数不会显示在表单中，也不会触发"额外字段"警告
- 在表单 → HCL 转换时，系统参数会被保留并输出到生成的 HCL 中

### 2. 额外字段检测与提示

**触发条件：**
当 HCL 中包含 Schema 未定义的字段时（排除系统参数）

**用户交互：**
- 显示警告提示栏，列出所有额外字段
- 提供两个操作按钮：
  - **保留** - 将额外字段保留在表单数据中，保存时会写入配置
  - **丢弃** - 从表单数据中移除额外字段，HCL 重新生成时不包含这些字段

**UI 设计：**
- 警告栏使用黄色主题（背景 #422006，边框 #78350f）
- 额外字段名称加粗显示
- 保留按钮为蓝色（#3b82f6），丢弃按钮为灰色（#475569）

## 技术实现

### 文件修改清单

#### 1. `frontend/src/utils/hclParser.ts`
```typescript
// 新增常量
export const TF_SYSTEM_PARAMS = new Set([
  'source', 'version', 'for_each', 'count', 
  'depends_on', 'providers', 'lifecycle'
]);

// 新增接口
export interface HCLParseResult {
  moduleName: string;
  systemParams: Record<string, any>;
  userConfig: Record<string, any>;
}

// 修改函数
export function parseHCLModule(hcl: string): HCLParseResult
export function detectExtraFields(
  userConfig: Record<string, any>, 
  schema?: any
): string[]
```

**关键改动：**
- `parseHCLModule` 现在返回 `{ moduleName, systemParams, userConfig }`
- 新增 `detectExtraFields` 函数，对比 userConfig 和 schema 找出额外字段

#### 2. `frontend/src/utils/hclFormatter.ts`
```typescript
// 新增选项
interface HCLFormatOptions {
  // ... 原有选项
  systemParams?: Record<string, any>;
}
```

**关键改动：**
- `jsonToHCL` 现在接受 `systemParams` 参数
- 系统参数在 `source`/`version` 之后、用户配置之前输出
- 保持原有的格式化逻辑不变

#### 3. `frontend/src/components/HCLEditor/HCLEditor.tsx`
```typescript
// 新增 props
interface HCLEditorProps {
  // ... 原有 props
  onExtraFields?: (fields: string[]) => Promise<boolean>;
}

// 新增 state
const [systemParams, setSystemParams] = useState<Record<string, any>>({});
const [pendingExtra, setPendingExtra] = useState<string[] | null>(null);
const [extraKept, setExtraKept] = useState<Record<string, any>>({});
```

**关键改动：**
- 新增 `parseAndNotify` 函数，解析 HCL 并检测额外字段
- 新增 `handleKeepExtra` 和 `handleDiscardExtra` 处理函数
- 渲染额外字段警告栏（当 `pendingExtra` 不为空时）
- 将 `systemParams` 传递给 `jsonToHCL`

#### 4. `frontend/src/components/HCLEditor/HCLEditor.module.css`
```css
/* 新增样式 */
.extraFieldsBar { /* 警告栏容器 */ }
.extraFieldsIcon { /* 警告图标 */ }
.extraFieldsText { /* 警告文本 */ }
.extraBtnKeep { /* 保留按钮 */ }
.extraBtnDiscard { /* 丢弃按钮 */ }
```

**关键改动：**
- 新增 5 个 CSS 类用于额外字段警告栏
- 使用深黄色主题与编辑器整体风格保持一致

## 数据流

### HCL → 表单
```
用户编辑 HCL
  ↓
parseHCLModule(hcl)
  ↓
{ moduleName, systemParams, userConfig }
  ↓
detectExtraFields(userConfig, schema)
  ↓
├─ 无额外字段 → onChange(userConfig)
└─ 有额外字段 → setPendingExtra(extra)
                  ↓
                显示警告栏
                  ↓
                用户选择保留/丢弃
                  ↓
                ├─ 保留 → 数据已在 formData 中
                └─ 丢弃 → 过滤后 onChange(filteredConfig)
```

### 表单 → HCL
```
formData 更新
  ↓
jsonToHCL(data, { systemParams, ... })
  ↓
输出 module 块:
  module "name" {
    source  = "..."
    version = "..."
    
    # systemParams (如果有)
    for_each = ...
    depends_on = [...]
    
    # userConfig
    bucket = "..."
    acl    = "private"
  }
```

## 使用示例

### 场景 1：包含系统参数
```hcl
module "s3_bucket" {
  source  = "terraform-aws-modules/s3-bucket/aws"
  version = "3.15.1"
  
  for_each = var.buckets
  
  bucket = each.value.name
  acl    = "private"
}
```
- `source` 和 `version` 被识别为系统参数
- `for_each` 被识别为系统参数
- `bucket` 和 `acl` 进入表单
- 不会触发额外字段警告

### 场景 2：包含额外字段
```hcl
module "s3_bucket" {
  source  = "terraform-aws-modules/s3-bucket/aws"
  version = "3.15.1"
  
  bucket = "my-bucket"
  custom_tag = "extra"  # Schema 未定义
}
```
- 显示警告：`⚠ 发现 1 个 Schema 未定义的字段：custom_tag`
- 用户点击"保留" → `custom_tag` 保留在配置中
- 用户点击"丢弃" → `custom_tag` 被移除

## 注意事项

1. **Schema 依赖**：额外字段检测依赖于 Schema 定义，如果 Schema 为空或不包含 `properties`，则不会触发警告

2. **系统参数优先级**：系统参数始终被识别和分离，不受 Schema 影响

3. **保留 vs 丢弃**：
   - 保留：额外字段会保存在数据库中，下次加载时仍会显示
   - 丢弃：额外字段立即从 formData 移除，HCL 重新生成

4. **编辑体验**：
   - 警告栏只在非编辑状态显示
   - 编辑状态下用户可以自由修改 HCL，包括额外字段
   - 退出编辑后（点击外部或按 Esc）会重新检测

## 测试建议

1. **基础功能测试**
   - 编辑包含 `for_each` 的 HCL，验证系统参数被正确识别
   - 添加 Schema 未定义的字段，验证警告栏显示
   - 分别测试"保留"和"丢弃"功能

2. **边界情况测试**
   - 空 Schema 时编辑 HCL
   - 所有字段都是额外字段
   - 只有系统参数，无用户配置

3. **集成测试**
   - 在 EditResource 页面完整流程测试
   - 保存后重新加载，验证配置正确性
   - 切换 v2/v3 版本，验证功能一致性

## 后续优化建议

1. **字段类型推断**：对于额外字段，可以尝试推断其类型并在表单中显示为通用输入框

2. **批量操作**：支持一次性保留/丢弃所有额外字段

3. **字段映射**：提供字段名映射功能，将额外字段映射到 Schema 中的标准字段

4. **历史记录**：记录用户的选择（保留/丢弃），下次遇到相同字段时自动应用
