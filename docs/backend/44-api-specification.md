# IaC平台API规范

## 1. 通用规范

### 1.1 基础URL
```
开发环境: http://localhost:8080/api/v1
生产环境: https://iac-platform.example.com/api/v1
```

### 1.2 资源ID格式规范

所有资源ID采用统一的字符串格式：`{type}-{20位随机字符}`

示例：
- Terraform版本: `tfver-ax12jxao191x01s8x9ka`
- 变量: `var-bx23kybo202y12t9y0lb`
- Workspace: `ws-cx34lzcp313z23u0z1mc`
- Run/Task: `run-dx45maaq424a34v1a2nd`
- Module: `mod-ex56nbbr535b45w2b3oe`

详细规范请参考：[资源ID规范文档](./id-specification.md)

### 1.3 认证方式
使用JWT Bearer Token认证：
```
Authorization: Bearer <token>
```

### 1.4 响应格式
```json
{
  "code": 200,
  "message": "success",
  "data": {},
  "timestamp": "2024-01-01T00:00:00Z"
}
```

### 1.5 错误码定义
```
200 - 成功
400 - 请求参数错误
401 - 未认证
403 - 权限不足
404 - 资源不存在
409 - 资源冲突
422 - 数据验证失败
500 - 服务器内部错误
```

## 2. 认证相关API

### 2.1 用户登录
```http
POST /auth/login
Content-Type: application/json

{
  "username": "admin",
  "password": "password123"
}
```

响应：
```json
{
  "code": 200,
  "message": "登录成功",
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "expires_at": "2024-01-02T00:00:00Z",
    "user": {
      "id": 1,
      "username": "admin",
      "email": "admin@example.com",
      "role": "admin"
    }
  }
}
```

### 2.2 用户注册
```http
POST /auth/register
Content-Type: application/json

{
  "username": "newuser",
  "email": "user@example.com",
  "password": "password123"
}
```

### 2.3 刷新Token
```http
POST /auth/refresh
Authorization: Bearer <token>
```

### 2.4 用户登出
```http
POST /auth/logout
Authorization: Bearer <token>
```

## 3. 模块管理API

### 3.1 获取模块列表
```http
GET /modules?page=1&size=20&provider=aws&search=s3
```

响应：
```json
{
  "code": 200,
  "data": {
    "items": [
      {
        "id": 1,
        "name": "s3",
        "provider": "aws",
        "source": "terraform-aws-modules/s3-bucket/aws",
        "version": "3.15.1",
        "description": "AWS S3 bucket module",
        "created_at": "2024-01-01T00:00:00Z"
      }
    ],
    "total": 1,
    "page": 1,
    "size": 20
  }
}
```

### 3.2 模块导入 (优化后)
```http
POST /modules/import
Content-Type: multipart/form-data

# 方式1: URL导入
{
  "import_type": "url",
  "source_url": "https://github.com/terraform-aws-modules/terraform-aws-s3-bucket",
  "branch": "main",
  "name": "s3-bucket",
  "provider": "aws",
  "description": "AWS S3 bucket module",
  "schema_option": "ai_generate"
}

# 方式2: 文件上传
{
  "import_type": "upload",
  "module_file": <ZIP文件>,
  "name": "s3-bucket",
  "provider": "aws", 
  "description": "AWS S3 bucket module",
  "schema_option": "user_provided",
  "schema_data": { ... }
}
```

响应：
```json
{
  "code": 200,
  "message": "模块导入成功",
  "data": {
    "module": {
      "id": 1,
      "name": "s3-bucket",
      "provider": "aws",
      "import_type": "url",
      "sync_status": "completed"
    },
    "schema": {
      "id": 1,
      "status": "active",
      "ai_generated": true
    }
  }
}
```

### 3.3 创建模块 (传统方式)
```http
POST /modules
Content-Type: application/json

{
  "name": "s3",
  "provider": "aws",
  "source": "terraform-aws-modules/s3-bucket/aws",
  "version": "3.15.1",
  "description": "AWS S3 bucket module",
  "vcs_provider_id": 1,
  "repository_url": "https://github.com/terraform-aws-modules/terraform-aws-s3-bucket",
  "branch": "v3.15.1",
  "path": "/"
}
```

### 3.3 同步模块文件
```http
POST /modules/{id}/sync
```

响应：
```json
{
  "code": 200,
  "data": {
    "sync_status": "syncing",
    "message": "开始同步模块文件",
    "module_files": {
      "variables.tf": "variable \"name\" {\n  type = string\n  description = \"Bucket name\"\n}",
      "main.tf": "resource \"aws_s3_bucket\" \"this\" {\n  bucket = var.name\n}",
      "outputs.tf": "output \"bucket_id\" {\n  value = aws_s3_bucket.this.id\n}"
    }
  }
}
```

### 3.4 获取模块文件内容
```http
GET /modules/{id}/files
```

响应：
```json
{
  "code": 200,
  "data": {
    "module_files": {
      "variables.tf": "variable content...",
      "main.tf": "main configuration...",
      "outputs.tf": "output definitions..."
    },
    "last_sync_at": "2024-01-01T00:00:00Z",
    "sync_status": "synced"
  }
}
```

### 3.5 获取模块详情
```http
GET /modules/{id}
```

### 3.6 更新模块
```http
PUT /modules/{id}
Content-Type: application/json

{
  "description": "Updated description",
  "branch": "v3.16.0"
}
```

### 3.7 删除模块
```http
DELETE /modules/{id}
```

## 4. Schema管理API

### 4.1 获取模块的Schema列表
```http
GET /modules/{module_id}/schemas?status=active
```

响应：
```json
{
  "code": 200,
  "data": [
    {
      "id": 1,
      "module_id": 1,
      "version": "1.0.0",
      "status": "active",
      "ai_generated": true,
      "created_at": "2024-01-01T00:00:00Z",
      "schema_data": {
        "name": {
          "type": "string",
          "required": true,
          "description": "Bucket name"
        }
      }
    }
  ]
}
```

### 4.2 创建Schema
```http
POST /modules/{module_id}/schemas
Content-Type: application/json

{
  "version": "1.0.0",
  "schema_data": {
    "name": {
      "type": "string",
      "required": true,
      "description": "Bucket name"
    }
  }
}
```

### 4.3 获取Schema详情
```http
GET /schemas/{id}
```

### 4.4 更新Schema
```http
PUT /schemas/{id}
Content-Type: application/json

{
  "schema_data": { ... },
  "status": "active"
}
```

### 4.5 手动创建Schema
```http
POST /modules/{module_id}/schema
Content-Type: application/json

{
  "schema_data": {
    "name": {
      "type": "string",
      "required": true,
      "description": "Bucket name"
    },
    "tags": {
      "type": "object",
      "required": true,
      "properties": {
        "Environment": {
          "type": "string",
          "options": ["dev", "staging", "prod"]
        }
      }
    }
  },
  "version": "1.0.0"
}
```

### 4.6 AI生成Schema
```http
POST /modules/{module_id}/generate-schema
Content-Type: application/json

{
  "ai_provider": "openai",
  "model": "gpt-4",
  "options": {
    "include_advanced": true,
    "generate_defaults": true,
    "use_template": true
  }
}
```

响应：
```json
{
  "code": 201,
  "message": "Schema generated successfully",
  "data": {
    "id": 1,
    "module_id": 1,
    "version": "1.0.0",
    "status": "active",
    "ai_generated": true,
    "schema_data": {
      "name": {
        "type": "string",
        "required": false,
        "description": "S3存储桶名称",
        "hiddenDefault": false
      },
      "tags": {
        "type": "object",
        "required": true,
        "description": "资源标签",
        "properties": {
          "Environment": {
            "type": "string",
            "required": true,
            "options": ["dev", "staging", "prod"]
          }
        }
      },
      "lifecycle_rule": {
        "type": "array",
        "required": false,
        "hiddenDefault": true,
        "description": "生命周期规则配置",
        "items": {
          "type": "object",
          "properties": {
            "enabled": {
              "type": "boolean",
              "default": true
            },
            "expiration": {
              "type": "object",
              "properties": {
                "days": {
                  "type": "number",
                  "default": 30
                }
              }
            }
          }
        }
      }
    },
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

### 4.6 验证Schema
```http
POST /schemas/{id}/validate
Content-Type: application/json

{
  "config_data": {
    "name": "my-bucket",
    "tags": {
      "Environment": "dev"
    }
  }
}
```

响应：
```json
{
  "code": 200,
  "data": {
    "valid": true,
    "errors": []
  }
}
```

## 5. AI解析API

### 5.1 解析模块生成Schema
```http
POST /modules/{id}/parse
Content-Type: application/json

{
  "ai_provider": "openai",
  "model": "gpt-4",
  "options": {
    "include_advanced": true,
    "generate_defaults": true
  }
}
```

响应：
```json
{
  "code": 200,
  "data": {
    "task_id": "task_123",
    "status": "processing"
  }
}
```

### 5.2 获取解析任务状态
```http
GET /ai-tasks/{task_id}
```

响应：
```json
{
  "code": 200,
  "data": {
    "id": "task_123",
    "status": "completed",
    "processing_time": 30,
    "output_schema": { ... },
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

## 6. 工作空间API

### 6.1 获取工作空间列表
```http
GET /workspaces
```

### 6.2 创建工作空间
```http
POST /workspaces
Content-Type: application/json

{
  "name": "dev-environment",
  "description": "Development workspace",
  "state_backend": "s3",
  "state_config": {
    "bucket": "terraform-state-bucket",
    "key": "dev/terraform.tfstate",
    "region": "us-west-2"
  },
  "terraform_version": "1.6.0",
  "execution_mode": "k8s",
  "agent_pool_id": 1
}
```

### 6.3 获取工作空间详情
```http
GET /workspaces/{id}
```

### 6.4 更新工作空间
```http
PUT /workspaces/{id}
```

### 6.5 删除工作空间
```http
DELETE /workspaces/{id}
```

### 6.6 获取工作空间变量
```http
GET /workspaces/{id}/variables
```

响应：
```json
{
  "code": 200,
  "data": [
    {
      "id": 1,
      "key": "AWS_REGION",
      "value": "us-west-2",
      "is_sensitive": false,
      "is_system": false,
      "description": "AWS区域"
    },
    {
      "id": 2,
      "key": "AWS_ACCESS_KEY_ID",
      "value": "***",
      "is_sensitive": true,
      "is_system": false,
      "description": "AWS访问密钥"
    }
  ]
}
```

### 6.7 创建工作空间变量
```http
POST /workspaces/{id}/variables
Content-Type: application/json

{
  "key": "AWS_REGION",
  "value": "us-west-2",
  "is_sensitive": false,
  "description": "AWS区域"
}
```

### 6.8 更新工作空间变量
```http
PUT /workspaces/{workspace_id}/variables/{variable_id}
Content-Type: application/json

{
  "value": "us-east-1",
  "description": "更新的AWS区域"
}
```

### 6.9 删除工作空间变量
```http
DELETE /workspaces/{workspace_id}/variables/{variable_id}
```

## 7. 部署管理API

### 7.1 获取部署列表
```http
GET /deployments?workspace_id=1&status=success&page=1&size=20
```

### 7.2 创建部署
```http
POST /deployments
Content-Type: application/json

{
  "workspace_id": 1,
  "module_id": 1,
  "schema_id": 1,
  "name": "my-s3-bucket",
  "config_data": {
    "name": "my-unique-bucket-name",
    "tags": {
      "Environment": "dev",
      "Project": "iac-platform"
    }
  }
}
```

响应：
```json
{
  "code": 200,
  "data": {
    "id": 1,
    "status": "pending",
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

### 7.3 获取部署详情
```http
GET /deployments/{id}
```

响应：
```json
{
  "code": 200,
  "data": {
    "id": 1,
    "workspace_id": 1,
    "module_id": 1,
    "schema_id": 1,
    "name": "my-s3-bucket",
    "status": "success",
    "config_data": { ... },
    "terraform_config": { ... },
    "terraform_output": {
      "bucket_id": {
        "value": "my-unique-bucket-name"
      }
    },
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:05:00Z"
  }
}
```

### 7.4 执行部署计划
```http
POST /deployments/{id}/plan
```

### 7.5 应用部署
```http
POST /deployments/{id}/apply
```

### 7.6 销毁部署
```http
POST /deployments/{id}/destroy
```

### 7.7 获取部署日志
```http
GET /deployments/{id}/logs
```

## 8. 检测API

### 8.1 执行安全检测
```http
POST /deployments/{id}/scan
Content-Type: application/json

{
  "scan_types": ["security", "cost", "compliance"]
}
```

### 8.2 获取检测结果
```http
GET /deployments/{id}/scan-results?scan_type=security
```

响应：
```json
{
  "code": 200,
  "data": [
    {
      "id": 1,
      "scan_type": "security",
      "score": 85,
      "passed": true,
      "results": {
        "high_severity": 0,
        "medium_severity": 2,
        "low_severity": 5,
        "issues": [
          {
            "severity": "medium",
            "rule": "S3_BUCKET_PUBLIC_ACCESS",
            "message": "S3 bucket allows public access",
            "resource": "aws_s3_bucket.this"
          }
        ]
      },
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

## 9. 系统配置API

### 9.1 获取系统配置
```http
GET /system/configs
```

### 9.2 更新系统配置
```http
PUT /system/configs
Content-Type: application/json

{
  "ai_provider": "openai",
  "ai_model": "gpt-4",
  "enable_cost_estimation": true
}
```

## 10. 审计日志API

### 10.1 获取审计日志
```http
GET /audit-logs?action=deploy&resource_type=deployment&start_date=2024-01-01&end_date=2024-01-31
```

## 11. VCS集成API

### 11.1 获取VCS提供商列表
```http
GET /vcs-providers
```

### 11.2 创建VCS提供商
```http
POST /vcs-providers
Content-Type: application/json

{
  "name": "github",
  "base_url": "https://api.github.com",
  "api_token": "ghp_xxxxxxxxxxxx",
  "webhook_secret": "webhook_secret_123"
}
```

### 11.3 测试VCS连接
```http
POST /vcs-providers/{id}/test
```

### 11.4 获取仓库列表
```http
GET /vcs-providers/{id}/repositories?search=terraform
```

### 11.5 获取仓库分支
```http
GET /vcs-providers/{provider_id}/repositories/{repo_name}/branches
```

## 12. Agent管理API

### 12.1 获取Agent池列表
```http
GET /agent-pools
```

### 12.2 创建Agent池
```http
POST /agent-pools
Content-Type: application/json

{
  "name": "k8s-pool",
  "description": "Kubernetes Agent池",
  "pool_type": "k8s",
  "k8s_config": {
    "namespace": "iac-platform",
    "image": "iac-platform/terraform-agent:latest",
    "resources": {
      "cpu": "500m",
      "memory": "1Gi"
    }
  }
}
```

### 12.3 获取Agent列表
```http
GET /agents?pool_id=1&status=online
```

### 12.4 Agent注册
```http
POST /agents/register
Content-Type: application/json

{
  "pool_id": 1,
  "name": "agent-001",
  "capabilities": {
    "terraform_versions": ["1.5.0", "1.6.0"],
    "providers": ["aws", "azure", "gcp"]
  }
}
```

响应：
```json
{
  "code": 200,
  "data": {
    "agent_id": 1,
    "token": "agent_token_xxxxxxxxxxxx"
  }
}
```

### 12.5 Agent心跳
```http
POST /agents/{id}/heartbeat
Authorization: Bearer <agent_token>
Content-Type: application/json

{
  "status": "online",
  "metadata": {
    "ip": "192.168.1.100",
    "version": "1.0.0",
    "load": 0.5
  }
}
```

### 12.6 获取Agent任务
```http
GET /agents/{id}/tasks
Authorization: Bearer <agent_token>
```

### 12.7 更新任务状态
```http
PUT /agents/{agent_id}/tasks/{task_id}
Authorization: Bearer <agent_token>
Content-Type: application/json

{
  "status": "running",
  "logs": "Terraform init completed...",
  "output": { ... }
}
```

## 13. 后台管理API

### 13.1 获取用户列表
```http
GET /admin/users?page=1&size=20&role=user
```

### 13.2 创建用户
```http
POST /admin/users
Content-Type: application/json

{
  "username": "newuser",
  "email": "user@example.com",
  "password": "password123",
  "role": "user"
}
```

### 13.3 更新用户
```http
PUT /admin/users/{id}
Content-Type: application/json

{
  "role": "admin",
  "is_active": false
}
```

### 13.4 删除用户
```http
DELETE /admin/users/{id}
```

### 13.5 获取系统统计
```http
GET /admin/stats
```

响应：
```json
{
  "code": 200,
  "data": {
    "total_users": 150,
    "total_workspaces": 45,
    "total_deployments": 1200,
    "active_agents": 8,
    "success_rate": 0.95,
    "avg_deployment_time": 180
  }
}
```

### 13.6 获取系统日志
```http
GET /admin/logs?level=error&start_date=2024-01-01&end_date=2024-01-31
```

## 14. WebSocket API

### 14.1 部署状态实时推送
```javascript
// 连接WebSocket
const ws = new WebSocket('ws://localhost:8080/ws/deployments/{deployment_id}');

// 接收消息
ws.onmessage = function(event) {
  const data = JSON.parse(event.data);
  console.log('Deployment status:', data.status);
  console.log('Logs:', data.logs);
};
```

消息格式：
```json
{
  "type": "status_update",
  "deployment_id": 1,
  "status": "applying",
  "logs": "Terraform is applying changes...",
  "timestamp": "2024-01-01T00:00:00Z"
}
```

### 14.2 Agent状态实时推送
```javascript
// 连接WebSocket
const ws = new WebSocket('ws://localhost:8080/ws/agents');

// 接收消息
ws.onmessage = function(event) {
  const data = JSON.parse(event.data);
  console.log('Agent status:', data);
};
```

消息格式：
```json
{
  "type": "agent_status",
  "agent_id": 1,
  "status": "online",
  "pool_id": 1,
  "timestamp": "2024-01-01T00:00:00Z"
}
```
