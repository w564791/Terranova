# Swagger UI 显示问题修复指南

## 问题描述
Swagger UI中curl命令和Response body的长文本内容超出显示边界,导致内容被截断。

## 已实施的解决方案

### 1. 创建自定义CSS文件
位置: `backend/static/swagger-custom.css`

该CSS文件包含以下修复:
- curl命令自动换行
- Response body长文本处理
- Request URL显示优化

### 2. 配置静态文件服务
在 `backend/internal/router/router.go` 中添加:
```go
r.Static("/static", "./static")
```

### 3. 使用方法

#### 方法A: 浏览器开发者工具注入CSS (推荐)
1. 打开Swagger页面: http://localhost:8080/swagger/index.html
2. 按F12打开开发者工具
3. 在Console中执行以下代码:

```javascript
// 加载自定义CSS
const link = document.createElement('link');
link.rel = 'stylesheet';
link.href = '/static/swagger-custom.css';
document.head.appendChild(link);
```

#### 方法B: 浏览器扩展
安装"Stylus"或"Stylish"浏览器扩展,为 `localhost:8080/swagger` 添加自定义CSS:
- CSS文件内容: 复制 `backend/static/swagger-custom.css` 的内容

#### 方法C: 使用书签工具
创建一个浏览器书签,URL设置为:
```javascript
javascript:(function(){const link=document.createElement('link');link.rel='stylesheet';link.href='/static/swagger-custom.css';document.head.appendChild(link);})();
```
在Swagger页面点击该书签即可应用样式。

## 验证
1. 重启后端服务
2. 访问 http://localhost:8080/swagger/index.html
3. 使用上述任一方法应用CSS
4. 测试 `/api/v1/iam/roles` 接口
5. 验证curl命令和Response body正确换行显示

## 注意事项
- 自定义CSS需要手动加载(通过开发者工具或浏览器扩展)
- 每次刷新页面后需要重新加载CSS
- 建议使用浏览器扩展实现自动加载

## 相关文件
- `backend/static/swagger-custom.css` - 自定义CSS样式
- `backend/internal/router/router.go` - 静态文件服务配置
- `backend/internal/handlers/role_handler.go` - Swagger路径修复
