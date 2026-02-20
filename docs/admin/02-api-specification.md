# Admin API规范文档

## 概述

本文档详细说明Admin管理模块的API接口规范，包括Terraform版本管理等功能。

---

## 基础信息

- **Base URL**: `/api/v1/admin`
- **认证方式**: JWT Token (Bearer Token)
- **Content-Type**: `application/json`

## ID格式规范

所有资源ID采用字符串格式：`{type}-{20位随机字符}`

- Terraform版本ID格式：`tfver-` + 20位随机字符（小写字母+数字）
- 示例：`tfver-ax12jxao191x01s8x9ka`

详细规范请参考：[ID规范文档](../id-specification.md)

---

## Terraform版本管理API

### 1. 获取所有Terraform版本

获取所有已配置的Terraform版本列表，支持按enabled和deprecated过滤。

**请求**

```
GET /api/v1/admin/terraform-versions
```

**Query参数**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| enabled | boolean | 否 | 过滤启用状态 (true/false) |
| deprecated | boolean | 否 | 过滤弃用状态 (true/false) |

**请求示例**

```bash
# 获取所有版本
curl -X GET "http://localhost:8080/api/v1/admin/terraform-versions" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"

# 只获取启用的版本
curl -X GET "http://localhost:8080/api/v1/admin/terraform-versions?enabled=true" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"

# 获取已弃用的版本
curl -X GET "http://localhost:8080/api/v1/admin/terraform-versions?deprecated=true" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

**响应**

```json
{
  "items": [
    {
      "id": "tfver-ax12jxao191x01s8x9ka",
      "version": "1.5.0",
      "download_url": "https://releases.hashicorp.com/terraform/1.5.0/terraform_1.5.0_linux_amd64.zip",
      "checksum": "ad0c696c870c8525357b5127680cd79c0bdf58179af9acd091d43b1d6482da4a",
      "enabled": true,
      "deprecated": false,
      "created_at": "2025-01-01T00:00:00Z",
      "updated_at": "2025-01-01T00:00:00Z"
    },
    {
      "id": "tfver-bx23kybo202y12t9y0lb",
      "version": "1.4.6",
      "download_url": "https://releases.hashicorp.com/terraform/1.4.6/terraform_1.4.6_linux_amd64.zip",
      "checksum": "3e9c46d6f37338e90d5018c156d89961b0ffb0f355249679593aff99f9abe2a2",
      "enabled": true,
      "deprecated": false,
      "created_at": "2025-01-01T00:00:00Z",
      "updated_at": "2025-01-01T00:00:00Z"
    }
  ],
  "total": 2
}
```

**状态码**

- `200 OK`: 成功
- `401 Unauthorized`: 未认证
- `500 Internal Server Error`: 服务器错误

---

### 2. 获取单个Terraform版本

根据ID获取Terraform版本详情。

**请求**

```
GET /api/v1/admin/terraform-versions/:id
```

**路径参数**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | string | 是 | 版本ID（格式：tfver-{20位随机字符}） |

**请求示例**

```bash
curl -X GET "http://localhost:8080/api/v1/admin/terraform-versions/tfver-ax12jxao191x01s8x9ka" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

**响应**

```json
{
  "id": "tfver-ax12jxao191x01s8x9ka",
  "version": "1.5.0",
  "download_url": "https://releases.hashicorp.com/terraform/1.5.0/terraform_1.5.0_linux_amd64.zip",
  "checksum": "ad0c696c870c8525357b5127680cd79c0bdf58179af9acd091d43b1d6482da4a",
  "enabled": true,
  "deprecated": false,
  "created_at": "2025-01-01T00:00:00Z",
  "updated_at": "2025-01-01T00:00:00Z"
}
```

**状态码**

- `200 OK`: 成功
- `400 Bad Request`: 无效的ID
- `401 Unauthorized`: 未认证
- `404 Not Found`: 版本不存在
- `500 Internal Server Error`: 服务器错误

---

### 3. 创建Terraform版本

创建新的Terraform版本配置。

**请求**

```
POST /api/v1/admin/terraform-versions
```

**请求体**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| version | string | 是 | 版本号（如1.5.0） |
| download_url | string | 是 | 下载链接（必须是有效URL） |
| checksum | string | 是 | SHA256校验和（64位十六进制） |
| enabled | boolean | 否 | 是否启用（默认false） |
| deprecated | boolean | 否 | 是否弃用（默认false） |

**请求示例**

```bash
curl -X POST "http://localhost:8080/api/v1/admin/terraform-versions" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "version": "1.6.0",
    "download_url": "https://releases.hashicorp.com/terraform/1.6.0/terraform_1.6.0_linux_amd64.zip",
    "checksum": "abc123def456789abc123def456789abc123def456789abc123def456789abcd",
    "enabled": true,
    "deprecated": false
  }'
```

**响应**

```json
{
  "id": "tfver-cx34lzcp313z23u0z1mc",
  "version": "1.6.0",
  "download_url": "https://releases.hashicorp.com/terraform/1.6.0/terraform_1.6.0_linux_amd64.zip",
  "checksum": "abc123def456789abc123def456789abc123def456789abc123def456789abcd",
  "enabled": true,
  "deprecated": false,
  "created_at": "2025-01-02T10:00:00Z",
  "updated_at": "2025-01-02T10:00:00Z"
}
```

**状态码**

- `201 Created`: 创建成功
- `400 Bad Request`: 请求参数错误
- `401 Unauthorized`: 未认证
- `500 Internal Server Error`: 服务器错误（如版本已存在）

**错误示例**

```json
{
  "error": "version 1.6.0 already exists"
}
```

---

### 4. 更新Terraform版本

更新Terraform版本配置。

**请求**

```
PUT /api/v1/admin/terraform-versions/:id
```

**路径参数**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | string | 是 | 版本ID（格式：tfver-{20位随机字符}） |

**请求体**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| download_url | string | 否 | 下载链接 |
| checksum | string | 否 | SHA256校验和 |
| enabled | boolean | 否 | 是否启用 |
| deprecated | boolean | 否 | 是否弃用 |

**注意**: 版本号(version)不可修改

**请求示例**

```bash
# 启用版本
curl -X PUT "http://localhost:8080/api/v1/admin/terraform-versions/tfver-ax12jxao191x01s8x9ka" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true
  }'

# 标记为弃用
curl -X PUT "http://localhost:8080/api/v1/admin/terraform-versions/tfver-bx23kybo202y12t9y0lb" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "deprecated": true
  }'

# 更新下载链接和校验和
curl -X PUT "http://localhost:8080/api/v1/admin/terraform-versions/tfver-cx34lzcp313z23u0z1mc" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "download_url": "https://new-url.com/terraform.zip",
    "checksum": "new123def456789abc123def456789abc123def456789abc123def456789abcd"
  }'
```

**响应**

```json
{
  "id": "tfver-ax12jxao191x01s8x9ka",
  "version": "1.5.0",
  "download_url": "https://releases.hashicorp.com/terraform/1.5.0/terraform_1.5.0_linux_amd64.zip",
  "checksum": "ad0c696c870c8525357b5127680cd79c0bdf58179af9acd091d43b1d6482da4a",
  "enabled": true,
  "deprecated": false,
  "created_at": "2025-01-01T00:00:00Z",
  "updated_at": "2025-01-02T10:30:00Z"
}
```

**状态码**

- `200 OK`: 更新成功
- `400 Bad Request`: 请求参数错误
- `401 Unauthorized`: 未认证
- `404 Not Found`: 版本不存在
- `500 Internal Server Error`: 服务器错误

---

### 5. 删除Terraform版本

删除Terraform版本配置。如果有workspace正在使用该版本，则无法删除。

**请求**

```
DELETE /api/v1/admin/terraform-versions/:id
```

**路径参数**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | string | 是 | 版本ID（格式：tfver-{20位随机字符}） |

**请求示例**

```bash
curl -X DELETE "http://localhost:8080/api/v1/admin/terraform-versions/tfver-cx34lzcp313z23u0z1mc" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

**响应**

成功时返回空响应体，状态码204。

**状态码**

- `204 No Content`: 删除成功
- `400 Bad Request`: 版本正在被使用
- `401 Unauthorized`: 未认证
- `404 Not Found`: 版本不存在
- `500 Internal Server Error`: 服务器错误

**错误示例**

```json
{
  "error": "version is in use by workspaces"
}
```

---

## 数据模型

### TerraformVersion

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string | 主键ID（格式：tfver-{20位随机字符}） |
| version | string | 版本号（如1.5.0） |
| download_url | string | 下载链接 |
| checksum | string | SHA256校验和（64位） |
| enabled | boolean | 是否启用 |
| deprecated | boolean | 是否弃用 |
| created_at | string | 创建时间（ISO 8601格式） |
| updated_at | string | 更新时间（ISO 8601格式） |

---

## 错误处理

### 错误响应格式

```json
{
  "error": "错误描述信息"
}
```

### 常见错误码

| 状态码 | 说明 |
|--------|------|
| 400 | 请求参数错误 |
| 401 | 未认证或Token无效 |
| 403 | 无权限访问 |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |

---

## 认证

所有Admin API都需要JWT认证。在请求头中包含Bearer Token：

```
Authorization: Bearer YOUR_JWT_TOKEN
```

获取Token的方式：

```bash
curl -X POST "http://localhost:8080/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "your_password"
  }'
```

响应：

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": 1,
    "username": "admin"
  }
}
```

---

## 使用示例

### 完整工作流程

```bash
# 1. 登录获取Token
TOKEN=$(curl -X POST "http://localhost:8080/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"password"}' \
  | jq -r '.token')

# 2. 获取所有版本
curl -X GET "http://localhost:8080/api/v1/admin/terraform-versions" \
  -H "Authorization: Bearer $TOKEN"

# 3. 创建新版本
curl -X POST "http://localhost:8080/api/v1/admin/terraform-versions" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "version": "1.6.0",
    "download_url": "https://releases.hashicorp.com/terraform/1.6.0/terraform_1.6.0_linux_amd64.zip",
    "checksum": "abc123...",
    "enabled": true
  }'

# 4. 更新版本
curl -X PUT "http://localhost:8080/api/v1/admin/terraform-versions/1" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"deprecated": true}'

# 5. 删除版本
curl -X DELETE "http://localhost:8080/api/v1/admin/terraform-versions/3" \
  -H "Authorization: Bearer $TOKEN"
```

---

## 前端集成示例

### TypeScript/React示例

```typescript
// services/admin.ts
import api from './api';

export interface TerraformVersion {
  id: string;  // tfver-{20位随机字符}
  version: string;
  download_url: string;
  checksum: string;
  enabled: boolean;
  deprecated: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateTerraformVersionRequest {
  version: string;
  download_url: string;
  checksum: string;
  enabled?: boolean;
  deprecated?: boolean;
}

export interface UpdateTerraformVersionRequest {
  download_url?: string;
  checksum?: string;
  enabled?: boolean;
  deprecated?: boolean;
}

export const adminService = {
  // 获取所有版本
  getTerraformVersions: async (params?: {
    enabled?: boolean;
    deprecated?: boolean;
  }) => {
    const response = await api.get('/admin/terraform-versions', { params });
    return response.data;
  },

  // 获取单个版本
  getTerraformVersion: async (id: string) => {
    const response = await api.get(`/admin/terraform-versions/${id}`);
    return response.data;
  },

  // 创建版本
  createTerraformVersion: async (data: CreateTerraformVersionRequest) => {
    const response = await api.post('/admin/terraform-versions', data);
    return response.data;
  },

  // 更新版本
  updateTerraformVersion: async (
    id: string,
    data: UpdateTerraformVersionRequest
  ) => {
    const response = await api.put(`/admin/terraform-versions/${id}`, data);
    return response.data;
  },

  // 删除版本
  deleteTerraformVersion: async (id: string) => {
    await api.delete(`/admin/terraform-versions/${id}`);
  },
};
```

### 使用示例

```typescript
// 在组件中使用
import { adminService } from '../services/admin';

// 获取所有启用的版本
const versions = await adminService.getTerraformVersions({ enabled: true });

// 创建新版本
const newVersion = await adminService.createTerraformVersion({
  version: '1.6.0',
  download_url: 'https://...',
  checksum: 'abc123...',
  enabled: true,
});

// 更新版本
await adminService.updateTerraformVersion('tfver-ax12jxao191x01s8x9ka', {
  deprecated: true,
});

// 删除版本
await adminService.deleteTerraformVersion('tfver-cx34lzcp313z23u0z1mc');
```

---

## 测试脚本

### 测试所有API端点

```bash
#!/bin/bash

BASE_URL="http://localhost:8080/api/v1"

# 登录
echo "=== 登录 ==="
TOKEN=$(curl -s -X POST "$BASE_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"password"}' \
  | jq -r '.token')

echo "Token: $TOKEN"

# 获取所有版本
echo -e "\n=== 获取所有版本 ==="
curl -s -X GET "$BASE_URL/admin/terraform-versions" \
  -H "Authorization: Bearer $TOKEN" | jq

# 创建版本
echo -e "\n=== 创建版本 ==="
curl -s -X POST "$BASE_URL/admin/terraform-versions" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "version": "1.6.0",
    "download_url": "https://releases.hashicorp.com/terraform/1.6.0/terraform_1.6.0_linux_amd64.zip",
    "checksum": "abc123def456789abc123def456789abc123def456789abc123def456789abcd",
    "enabled": true
  }' | jq

# 获取单个版本
echo -e "\n=== 获取版本详情 ==="
curl -s -X GET "$BASE_URL/admin/terraform-versions/tfver-ax12jxao191x01s8x9ka" \
  -H "Authorization: Bearer $TOKEN" | jq

# 更新版本
echo -e "\n=== 更新版本 ==="
curl -s -X PUT "$BASE_URL/admin/terraform-versions/tfver-ax12jxao191x01s8x9ka" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"deprecated": true}' | jq

# 删除版本
echo -e "\n=== 删除版本 ==="
curl -s -X DELETE "$BASE_URL/admin/terraform-versions/tfver-cx34lzcp313z23u0z1mc" \
  -H "Authorization: Bearer $TOKEN"

echo -e "\n=== 测试完成 ==="
```

---

## 注意事项

1. **版本号唯一性**: 版本号必须唯一，不能重复
2. **Checksum格式**: 必须是64位SHA256哈希值（小写十六进制）
3. **URL验证**: download_url必须是有效的URL格式
4. **删除限制**: 如果有workspace使用该版本，无法删除
5. **权限控制**: 所有Admin API需要管理员权限（未来实现RBAC）

---

## 更新日志

| 版本 | 日期 | 变更说明 |
|------|------|----------|
| v1.0 | 2025-10-09 | 初始版本，Terraform版本管理API |

---

## 相关文档

- [01-requirements.md](./01-requirements.md) - Admin管理功能需求
- [README.md](./README.md) - Admin模块总览
- [development-progress.md](./development-progress.md) - 开发进度
- [../workspace/09-api-specification.md](../workspace/09-api-specification.md) - Workspace API规范
