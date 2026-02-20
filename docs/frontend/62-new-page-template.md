# 新页面开发模板

## 📋 标准页面模板

### 1. 基础页面结构
```typescript
import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage, logError } from '../utils/errorHandler';
import { yourService } from '../services/yourService';
import styles from './YourPage.module.css';

const YourPage: React.FC = () => {
  const navigate = useNavigate();
  const [loading, setLoading] = useState(false);
  const { success, error, warning, info } = useToast();

  const handleAction = async () => {
    setLoading(true);
    
    try {
      const result = await yourService.doSomething();
      success('操作成功！');
      // 可选：导航到其他页面
      // setTimeout(() => navigate('/target'), 1500);
    } catch (err: any) {
      logError('操作', err);
      error('操作失败: ' + extractErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className={styles.container}>
      {/* 页面内容 */}
    </div>
  );
};

export default YourPage;
```

### 2. 表单页面模板
```typescript
import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage, logError } from '../utils/errorHandler';
import { yourService } from '../services/yourService';
import styles from './CreateYourResource.module.css';

const CreateYourResource: React.FC = () => {
  const navigate = useNavigate();
  const [loading, setLoading] = useState(false);
  const { success, error } = useToast();
  const [formData, setFormData] = useState({
    name: '',
    description: ''
  });

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);

    try {
      await yourService.create(formData);
      success('创建成功！');
      setTimeout(() => navigate('/your-resources'), 1500);
    } catch (err: any) {
      logError('创建资源', err);
      error('创建失败: ' + extractErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => {
    setFormData({
      ...formData,
      [e.target.name]: e.target.value
    });
  };

  return (
    <div className={styles.container}>
      <form onSubmit={handleSubmit}>
        {/* 表单内容 */}
        <button type="submit" disabled={loading}>
          {loading ? '创建中...' : '创建'}
        </button>
      </form>
    </div>
  );
};

export default CreateYourResource;
```

### 3. 列表页面模板
```typescript
import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage, logError } from '../utils/errorHandler';
import { yourService } from '../services/yourService';
import styles from './YourResourceList.module.css';

const YourResourceList: React.FC = () => {
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [items, setItems] = useState([]);
  const { success, error, warning } = useToast();

  const loadItems = async () => {
    try {
      const response = await yourService.getList();
      setItems(response.data.items || response.data);
    } catch (err: any) {
      logError('加载列表', err);
      error('加载失败: ' + extractErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async (id: number) => {
    if (!confirm('确定要删除吗？')) return;
    
    try {
      await yourService.delete(id);
      success('删除成功！');
      loadItems(); // 重新加载列表
    } catch (err: any) {
      logError('删除资源', err);
      error('删除失败: ' + extractErrorMessage(err));
    }
  };

  useEffect(() => {
    loadItems();
  }, []);

  return (
    <div className={styles.container}>
      {/* 列表内容 */}
    </div>
  );
};

export default YourResourceList;
```

## 🎯 自动应用机制

### 全局通知系统
 **自动应用** - 通过ToastProvider在App.tsx中全局提供
- 任何新页面只需导入`useToast()`即可使用
- 功能开关自动控制Toast/Alert切换

### 错误处理工具
 **手动导入** - 需要在新页面中导入使用
```typescript
import { extractErrorMessage, logError } from '../utils/errorHandler';
```

### 路由配置
❌ **手动添加** - 需要在App.tsx中添加新路由
```typescript
<Route path="new-page" element={<NewPage />} />
```

### 导航菜单
❌ **手动添加** - 需要在Layout组件中添加菜单项

## 📝 开发检查清单

### 新页面开发前
- [ ] 复制对应的页面模板代码
- [ ] 导入必要的依赖（useToast, errorHandler）
- [ ] 在App.tsx中添加路由配置
- [ ] 在Layout中添加导航菜单（如需要）

### 错误处理标准
- [ ] 使用`logError()`记录详细错误信息
- [ ] 使用`extractErrorMessage()`提取用户友好的错误信息
- [ ] 使用`error()`显示错误通知
- [ ] 在finally块中重置loading状态

### 成功处理标准
- [ ] 使用`success()`显示成功通知
- [ ] 适当延迟后导航（1500ms）
- [ ] 重新加载相关数据（如列表页面）

## 🔧 快速创建新页面

### 1. 复制模板
```bash
# 复制对应模板到新文件
cp template.tsx src/pages/NewPage.tsx
```

### 2. 修改内容
- 替换组件名称
- 修改service调用
- 调整表单字段
- 更新样式文件名

### 3. 添加路由
```typescript
// App.tsx
<Route path="new-page" element={<NewPage />} />
```

### 4. 添加导航（可选）
```typescript
// Layout.tsx 或相关导航组件
<NavItem to="/new-page">新页面</NavItem>
```

这样，所有新页面都会自动继承：
-  全局通知系统
-  统一错误处理
-  标准化用户体验
-  功能开关保护