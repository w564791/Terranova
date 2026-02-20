# Frontend API Authorization 诊断报告

## 问题描述

用户报告以下API返回错误:
1. `/api/v1/dashboard/compliance` 返回 401 Unauthorized
2. `/api/v1/auth/me` 返回 404 Not Found

## 诊断结果

### 1. 后端路由配置  正常

**`/api/v1/auth/me` 路由存在且配置正确:**
```go
// backend/internal/router/router.go:52
api.GET("/auth/me", middleware.JWTAuth(), handlers.NewAuthHandler(db).GetMe)
```

**`/api/v1/dashboard/*` 路由存在且配置正确:**
```go
// backend/internal/router/router.go:75-82
dashboard := api.Group("/dashboard")
dashboard.Use(middleware.JWTAuth())
dashboard.Use(middleware.AuditLogger(db))
{
    dashboardCtrl := controllers.NewDashboardController(db)
    dashboard.GET("/overview",
        iamMiddleware.RequirePermission("ORGANIZATION", "ORGANIZATION", "READ"),
        dashboardCtrl.GetOverviewStats)
    dashboard.GET("/compliance",
        iamMiddleware.RequirePermission("ORGANIZATION", "ORGANIZATION", "READ"),
        dashboardCtrl.GetComplianceStats)
}
```

### 2. 前端API配置  正常

**API拦截器正确配置:**
```typescript
// frontend/src/services/api.ts
api.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token');
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);
```

**Dashboard和AuthProvider正确使用api实例:**
- `frontend/src/components/AuthProvider.tsx` - 使用 `api.get('/auth/me')`
- `frontend/src/pages/Dashboard.tsx` - 使用 `api.get('/dashboard/overview')` 和 `api.get('/dashboard/compliance')`

### 3. 根本原因分析

基于代码审查,**前端代码本身没有问题**。可能的原因包括:

#### A. Token不存在或已过期
- 用户未登录或token已过期
- localStorage中没有有效的token

#### B. Token格式问题
- 后端JWT中间件期望的token格式与实际不符
- User ID类型变更(uint → string)可能导致旧token无效

#### C. 权限问题
- Dashboard路由需要 `ORGANIZATION.ORGANIZATION.READ` 权限
- 用户可能没有被授予此权限

#### D. 404错误的特殊情况
- `/auth/me` 返回404可能是因为:
  - 前端请求的URL路径不正确(例如多了前缀)
  - 后端路由注册顺序问题
  - Gin路由匹配问题

## 解决方案

### 立即行动项

1. **检查浏览器控制台**
   - 打开开发者工具 → Network标签
   - 查看实际发送的请求URL
   - 检查Request Headers中是否有Authorization字段
   - 查看完整的错误响应

2. **验证Token存在性**
   ```javascript
   // 在浏览器控制台执行
   console.log('Token:', localStorage.getItem('token'));
   ```

3. **重新登录**
   - 清除localStorage: `localStorage.clear()`
   - 重新登录获取新token
   - 新token应该包含string类型的user_id

4. **检查用户权限**
   ```sql
   -- 查询用户权限
   SELECT * FROM iam_role_assignments WHERE principal_id = 'user-xxxxxxxxxx';
   SELECT * FROM iam_role_policies WHERE role_id IN (
     SELECT role_id FROM iam_role_assignments WHERE principal_id = 'user-xxxxxxxxxx'
   );
   ```

### 代码修复建议

虽然前端代码本身正确,但可以添加更好的错误处理:

#### 1. 增强AuthProvider错误处理

```typescript
// frontend/src/components/AuthProvider.tsx
useEffect(() => {
  const verifyToken = async () => {
    if (token) {
      try {
        console.log('Verifying token...', token.substring(0, 20) + '...');
        const response = await api.get('/auth/me');
        console.log('Token verified, user:', response.data);
        dispatch(loginSuccess({
          user: response.data,
          token: token
        }));
      } catch (error) {
        console.error('Token verification failed:', error);
        // Token无效，清除登录状态
        dispatch(logout());
      }
    }
  };
  // ...
}, [token, dispatch]);
```

#### 2. 增强Dashboard错误处理

```typescript
// frontend/src/pages/Dashboard.tsx
const loadStats = async () => {
  try {
    setLoading(true);
    console.log('Loading dashboard stats...');
    const overview = await api.get('/dashboard/overview');
    console.log('Overview loaded:', overview);
    const compliance = await api.get('/dashboard/compliance');
    console.log('Compliance loaded:', compliance);
    setOverviewStats(overview);
    setComplianceStats(compliance);
  } catch (err) {
    console.error('Failed to load dashboard stats:', err);
    // 显示具体错误信息
    if (err.response) {
      console.error('Response status:', err.response.status);
      console.error('Response data:', err.response.data);
    }
  } finally {
    setLoading(false);
  }
};
```

### 后端验证建议

检查JWT中间件是否正确处理string类型的user_id:

```go
// backend/internal/middleware/middleware.go
func JWTAuth() gin.HandlerFunc {
    return func(c *gin.Context) {
        // ... token解析逻辑
        
        // 确保user_id作为string处理
        if userID, ok := claims["user_id"].(string); ok {
            c.Set("user_id", userID)
        } else {
            // 兼容旧的uint类型
            if userIDFloat, ok := claims["user_id"].(float64); ok {
                c.Set("user_id", fmt.Sprintf("%.0f", userIDFloat))
            }
        }
        
        c.Next()
    }
}
```

## 测试步骤

1. **清除旧数据并重新登录**
   ```bash
   # 浏览器控制台
   localStorage.clear()
   # 然后访问 /login 重新登录
   ```

2. **验证Token**
   ```bash
   # 浏览器控制台
   const token = localStorage.getItem('token');
   console.log('Token:', token);
   
   # 手动测试API
   fetch('http://localhost:8080/api/v1/auth/me', {
     headers: { 'Authorization': `Bearer ${token}` }
   }).then(r => r.json()).then(console.log);
   ```

3. **检查权限**
   ```bash
   # 浏览器控制台
   fetch('http://localhost:8080/api/v1/dashboard/overview', {
     headers: { 'Authorization': `Bearer ${localStorage.getItem('token')}` }
   }).then(r => r.json()).then(console.log);
   ```

## 预期结果

修复后应该看到:
-  `/api/v1/auth/me` 返回用户信息(包含string类型的user_id)
-  `/api/v1/dashboard/overview` 返回统计数据
-  `/api/v1/dashboard/compliance` 返回合规数据
-  所有请求都包含正确的Authorization header

## 总结

**前端代码本身没有问题**,API拦截器和请求都配置正确。问题很可能是:
1. Token不存在或已过期(需要重新登录)
2. 用户没有必要的权限(需要授予ORGANIZATION权限)
3. 旧token与新的user_id格式不兼容(需要重新登录)

建议用户先清除localStorage并重新登录,这应该能解决大部分问题。
