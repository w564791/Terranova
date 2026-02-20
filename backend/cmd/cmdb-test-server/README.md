# CMDB Test Server

模拟的外部CMDB API服务器，用于测试外部数据源集成功能。

## 启动服务器

```bash
cd backend/cmd/cmdb-test-server
go run main.go
```

服务器将在端口 **11112** 上启动。

## API信息

### 认证
- **Header**: `X-API-Token`
- **Token**: `test-cmdb-token-12345`

### 端点

#### 1. 健康检查（无需认证）
```bash
curl http://localhost:11112/health
```

#### 2. 获取所有资源
```bash
curl -H 'X-API-Token: test-cmdb-token-12345' http://localhost:11112/api/v1/resources
```

#### 3. 获取单个资源
```bash
curl -H 'X-API-Token: test-cmdb-token-12345' http://localhost:11112/api/v1/resources/i-0123456789abcdef0
```

## 配置外部数据源

在CMDB页面的"External Sources"Tab中创建数据源：

1. **名称**: Test CMDB
2. **API端点**: `http://localhost:11112/api/v1/resources`
3. **HTTP方法**: GET
4. **认证Headers**:
   - Key: `X-API-Token`
   - Value: `test-cmdb-token-12345`
5. **响应数据路径**: `$.data`
6. **主键字段**: `$.id`
7. **字段映射**:
   - resource_type: `$.type`
   - resource_name: `$.name`
   - cloud_resource_id: `$.id`
   - cloud_resource_name: `$.name`
   - cloud_resource_arn: `$.arn`
   - description: `$.description`
   - tags: `$.tags`
8. **云提供商**: aws
9. **账户ID**: 123456789012
10. **账户名称**: Production Account
11. **区域**: us-east-1

## 模拟数据

服务器返回6个模拟资源：
- 2个EC2实例
- 1个安全组
- 1个VPC
- 1个S3存储桶
- 1个RDS数据库实例
