# 功能开关使用指南

## 🎛️ 功能开关机制

为了避免新功能导致页面白屏或系统不稳定，所有新功能都必须通过功能开关控制。

## 📋 使用规范

### 1. 前端功能开关

#### 配置文件
```typescript
// src/config/features.ts
export const FEATURES = {
  TOAST_NOTIFICATIONS: false,  // Toast通知系统
  ADVANCED_FORMS: false,       // 高级表单功能
  AI_PARSING: false,           // AI解析功能
  REAL_TIME_UPDATES: false,    // 实时更新
  DARK_MODE: false,            // 暗色主题
} as const;
```

#### 使用方式
```typescript
import { FEATURES } from '../config/features';

const MyComponent = () => {
  return (
    <div>
      {FEATURES.TOAST_NOTIFICATIONS ? (
        <Toast message="新功能已启用" type="success" />
      ) : (
        <div>使用传统alert通知</div>
      )}
    </div>
  );
};
```

### 2. 开发流程

#### 新功能开发步骤
1. **创建功能开关**: 在features.ts中添加新功能开关，默认为false
2. **条件渲染**: 使用功能开关包装新功能代码
3. **渐进式启用**: 先在开发环境测试，再逐步启用
4. **回滚机制**: 出现问题时可立即禁用功能
5. **清理代码**: 功能稳定后移除开关，合并到主流程

#### 代码示例模板
```typescript
// ❌ 错误做法 - 直接添加新功能
const MyComponent = () => {
  const { toast, success, error } = useToast(); // 可能导致白屏
  return <Toast />;
};

//  正确做法 - 使用功能开关
import { FEATURES } from '../config/features';

const MyComponent = () => {
  if (FEATURES.TOAST_NOTIFICATIONS) {
    const { toast, success, error } = useToast();
    return <Toast />;
  }
  
  // 回退到传统方式
  const showMessage = (msg: string) => alert(msg);
  return <div>传统通知方式</div>;
};
```

### 3. 功能开关管理

#### 开关命名规范
- 使用大写字母和下划线
- 功能描述要清晰明确
- 避免过于细粒度的开关

#### 开关生命周期
1. **实验阶段**: 默认关闭，仅开发环境启用
2. **测试阶段**: 部分用户启用，收集反馈
3. **发布阶段**: 全量启用，监控稳定性
4. **稳定阶段**: 移除开关，合并到主代码

### 4. 紧急回滚

#### 快速禁用功能
```typescript
// 修改 src/config/features.ts
export const FEATURES = {
  TOAST_NOTIFICATIONS: false, // 立即禁用有问题的功能
  // ... 其他功能
} as const;
```

#### 环境变量控制（可选）
```bash
# .env 文件
VITE_FEATURE_TOAST_NOTIFICATIONS=false
VITE_FEATURE_ADVANCED_FORMS=false
```

## 🛡️ 最佳实践

### 1. 防止白屏问题
```typescript
// 使用try-catch包装新功能
const SafeNewFeature = () => {
  if (!FEATURES.NEW_FEATURE) {
    return <FallbackComponent />;
  }
  
  try {
    return <NewFeatureComponent />;
  } catch (error) {
    console.error('New feature error:', error);
    return <FallbackComponent />;
  }
};
```

### 2. 渐进式增强
```typescript
// 基础功能 + 可选增强
const MyComponent = () => {
  return (
    <div>
      {/* 基础功能始终可用 */}
      <BasicFeature />
      
      {/* 增强功能可选启用 */}
      {FEATURES.ADVANCED_FEATURE && (
        <AdvancedFeature />
      )}
    </div>
  );
};
```

### 3. 性能考虑
```typescript
// 懒加载新功能组件
const LazyNewFeature = React.lazy(() => 
  FEATURES.NEW_FEATURE 
    ? import('./NewFeature')
    : Promise.resolve({ default: () => null })
);
```

## 📊 功能开关状态

### 当前功能状态
| 功能 | 状态 | 说明 |
|------|------|------|
| TOAST_NOTIFICATIONS | 🔴 禁用 | 导致白屏问题，暂时禁用 |
| ADVANCED_FORMS | 🔴 禁用 | 开发中 |
| AI_PARSING | 🔴 禁用 | 开发中 |
| REAL_TIME_UPDATES | 🔴 禁用 | 开发中 |
| DARK_MODE | 🔴 禁用 | 开发中 |

### 启用计划
1. **Phase 1**: 修复Toast通知系统，启用TOAST_NOTIFICATIONS
2. **Phase 2**: 完成高级表单，启用ADVANCED_FORMS
3. **Phase 3**: 集成AI解析，启用AI_PARSING

## 🔧 调试工具

### 开发环境调试
```typescript
// 在开发环境显示功能开关状态
if (import.meta.env.DEV) {
  console.log('🎛️ Feature Flags:', FEATURES);
}
```

### 运行时切换（仅开发环境）
```typescript
// 在浏览器控制台动态切换功能
if (import.meta.env.DEV) {
  (window as any).toggleFeature = (feature: string) => {
    // 实现运行时切换逻辑
  };
}
```

## 📝 检查清单

### 新功能开发前
- [ ] 在features.ts中添加功能开关
- [ ] 功能开关默认设置为false
- [ ] 准备回退方案（传统实现）

### 功能开发中
- [ ] 使用功能开关包装所有新代码
- [ ] 添加错误边界和异常处理
- [ ] 测试开关开启和关闭两种状态

### 功能发布前
- [ ] 确认回退方案正常工作
- [ ] 准备紧急禁用方案
- [ ] 更新功能开关文档

### 功能稳定后
- [ ] 移除功能开关代码
- [ ] 清理相关配置
- [ ] 更新文档状态

记住：**功能开关是为了保证系统稳定性，不是为了增加复杂度！**