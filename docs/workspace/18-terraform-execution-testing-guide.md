# Terraform执行引擎功能测试指南

> **文档版本**: v1.0  
> **创建日期**: 2025-10-11  
> **状态**: 测试指南  
> **目标**: 验证Terraform执行引擎和资源版本管理功能

## 📋 测试概述

本文档提供Terraform执行引擎和资源级别版本管理的完整测试流程。

## 🎯 测试目标

1. 验证Terraform Plan/Apply流程
2. 验证State版本管理
3. 验证资源级别版本控制
4. 验证选择性部署
5. 验证容错机制

## 🚀 测试准备

### 1. 启动服务

```bash
# 1. 确保数据库运行
docker ps | grep postgres

# 2. 启动后端服务
cd backend && go run main.go

# 3. 验证服务启动
curl http://localhost:8080/health
# 应返回: {"status":"ok"}
```

### 2. 获取认证Token

```bash
# 登录获取token
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "admin123"
  }'

# 保存返回的token
export TOKEN="<your_token_here>"
```

## 📝 测试场景

### 场景1: 传统方式 - 使用workspace.TFCode

#### 1.1 创建Workspace

```bash
curl -X POST http://localhost:8080/api/v1/workspaces \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-workspace-traditional",
    "description": "测试传统方式",
    "execution_mode": "local",
    "state_backend": "local",
    "tf_code": {
      "resource": {
        "null_resource": {
          "test": {
            "triggers": {
              "timestamp": "${timestamp()}"
            }
          }
        }
      }
    },
    "provider_config": {
      "terraform": [{
        "required_version": ">= 1.0"
      }]
    }
  }'

# 保存返回的workspace_id
export WS_ID=<workspace_id>
```

#### 1.2 执行Plan

```bash
curl -X POST http://localhost:8080/api/v1/workspaces/$WS_ID/tasks/plan \
  -H "Authorization: Bearer $TOKEN"

# 保存返回的task_id
export PLAN_TASK_ID=<task_id>
```

#### 1.3 查看Plan任务状态

```bash
# 查询任务状态
curl http://localhost:8080/api/v1/workspaces/$WS_ID/tasks/$PLAN_TASK_ID \
  -H "Authorization: Bearer $TOKEN"

# 查看Plan日志
curl http://localhost:8080/api/v1/workspaces/$WS_ID/tasks/$PLAN_TASK_ID/logs \
  -H "Authorization: Bearer $TOKEN"
```

#### 1.4 执行Apply

```bash
curl -X POST http://localhost:8080/api/v1/workspaces/$WS_ID/tasks/apply \
  -H "Authorization: Bearer $TOKEN"

# 保存返回的task_id
export APPLY_TASK_ID=<task_id>
```

#### 1.5 验证State版本

```bash
# 查看State版本列表
curl http://localhost:8080/api/v1/workspaces/$WS_ID/state-versions \
  -H "Authorization: Bearer $TOKEN"

# 应该看到新创建的State版本
```

### 场景2: 资源级别方式 - 使用资源管理

#### 2.1 创建Workspace

```bash
curl -X POST http://localhost:8080/api/v1/workspaces \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-workspace-resources",
    "description": "测试资源级别版本管理",
    "execution_mode": "local",
    "state_backend": "local",
    "provider_config": {
      "terraform": [{
        "required_version": ">= 1.0"
      }]
    }
  }'

export WS_ID2=<workspace_id>
```

#### 2.2 添加资源

```bash
# 添加第一个资源
curl -X POST http://localhost:8080/api/v1/workspaces/$WS_ID2/resources \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "resource_type": "null_resource",
    "resource_name": "resource1",
    "tf_code": {
      "resource": {
        "null_resource": {
          "resource1": {
            "triggers": {
              "name": "resource1"
            }
          }
        }
      }
    },
    "description": "第一个测试资源"
  }'

export RES1_ID=<resource_id>

# 添加第二个资源
curl -X POST http://localhost:8080/api/v1/workspaces/$WS_ID2/resources \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "resource_type": "null_resource",
    "resource_name": "resource2",
    "tf_code": {
      "resource": {
        "null_resource": {
          "resource2": {
            "triggers": {
              "name": "resource2"
            }
          }
        }
      }
    },
    "description": "第二个测试资源"
  }'

export RES2_ID=<resource_id>
```

#### 2.3 查看资源列表

```bash
curl http://localhost:8080/api/v1/workspaces/$WS_ID2/resources \
  -H "Authorization: Bearer $TOKEN"

# 应该看到2个资源
```

#### 2.4 执行Plan（全部资源）

```bash
curl -X POST http://localhost:8080/api/v1/workspaces/$WS_ID2/tasks/plan \
  -H "Authorization: Bearer $TOKEN"
```

#### 2.5 执行Apply

```bash
curl -X POST http://localhost:8080/api/v1/workspaces/$WS_ID2/tasks/apply \
  -H "Authorization: Bearer $TOKEN"
```

#### 2.6 更新资源

```bash
# 更新resource1
curl -X PUT http://localhost:8080/api/v1/workspaces/$WS_ID2/resources/$RES1_ID \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "tf_code": {
      "resource": {
        "null_resource": {
          "resource1": {
            "triggers": {
              "name": "resource1",
              "updated": "true"
            }
          }
        }
      }
    },
    "change_summary": "添加updated触发器"
  }'
```

#### 2.7 查看版本历史

```bash
curl http://localhost:8080/api/v1/workspaces/$WS_ID2/resources/$RES1_ID/versions \
  -H "Authorization: Bearer $TOKEN"

# 应该看到2个版本
```

#### 2.8 选择性部署（只部署resource1）

```bash
curl -X POST http://localhost:8080/api/v1/workspaces/$WS_ID2/resources/deploy \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "resource_ids": ['$RES1_ID']
  }'

# 这将创建一个带-target参数的Plan任务
```

#### 2.9 回滚资源

```bash
# 回滚resource1到版本1
curl -X POST http://localhost:8080/api/v1/workspaces/$WS_ID2/resources/$RES1_ID/versions/1/rollback \
  -H "Authorization: Bearer $TOKEN"

# 这将创建版本3（内容是版本1的）
```

#### 2.10 创建快照

```bash
curl -X POST http://localhost:8080/api/v1/workspaces/$WS_ID2/snapshots \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "snapshot_name": "v1.0.0",
    "description": "稳定版本"
  }'

export SNAPSHOT_ID=<snapshot_id>
```

#### 2.11 恢复快照

```bash
curl -X POST http://localhost:8080/api/v1/workspaces/$WS_ID2/snapshots/$SNAPSHOT_ID/restore \
  -H "Authorization: Bearer $TOKEN"
```

### 场景3: 资源导入

#### 3.1 从TF代码批量导入资源

```bash
curl -X POST http://localhost:8080/api/v1/workspaces/$WS_ID2/resources/import \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "tf_code": {
      "resource": {
        "null_resource": {
          "imported1": {
            "triggers": {"name": "imported1"}
          },
          "imported2": {
            "triggers": {"name": "imported2"}
          },
          "imported3": {
            "triggers": {"name": "imported3"}
          }
        }
      }
    }
  }'

# 应该返回导入了3个资源
```

### 场景4: 依赖关系管理

#### 4.1 设置资源依赖

```bash
# 设置resource2依赖resource1
curl -X PUT http://localhost:8080/api/v1/workspaces/$WS_ID2/resources/$RES2_ID/dependencies \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "depends_on": ['$RES1_ID']
  }'
```

#### 4.2 查看依赖关系

```bash
curl http://localhost:8080/api/v1/workspaces/$WS_ID2/resources/$RES2_ID/dependencies \
  -H "Authorization: Bearer $TOKEN"

# 应该看到depends_on包含resource1
```

##  验证清单

### Terraform执行引擎
- [ ] Plan任务创建成功
- [ ] Plan任务异步执行
- [ ] Plan输出正确保存
- [ ] Plan JSON生成成功
- [ ] Apply任务创建成功
- [ ] Apply使用Plan文件
- [ ] State版本正确创建
- [ ] 任务日志完整记录
- [ ] 任务可以取消

### 资源版本管理
- [ ] 资源创建成功
- [ ] 资源列表查询正确
- [ ] 资源更新创建新版本
- [ ] 版本历史查询正确
- [ ] 资源回滚成功
- [ ] 版本对比功能正常
- [ ] 快照创建成功
- [ ] 快照恢复成功
- [ ] 依赖关系设置成功
- [ ] 资源导入成功

### 选择性部署
- [ ] 选择性部署创建Plan任务
- [ ] Plan任务包含-target参数
- [ ] 只部署选定的资源
- [ ] 其他资源不受影响

### 容错机制
- [ ] Plan数据保存失败不阻塞
- [ ] State保存失败自动备份
- [ ] State保存失败自动锁定workspace
- [ ] 备份文件正确创建

## 🐛 常见问题

### 问题1: Terraform未安装

**症状**: terraform init失败，提示command not found

**解决**:
```bash
# macOS
brew install terraform

# 验证安装
terraform version
```

### 问题2: 权限不足

**症状**: 无法创建工作目录或备份目录

**解决**:
```bash
# 创建必要的目录
sudo mkdir -p /tmp/iac-platform/workspaces
sudo mkdir -p /var/backup/states
sudo mkdir -p /var/cache/terraform/plugins

# 设置权限
sudo chmod 777 /tmp/iac-platform/workspaces
sudo chmod 700 /var/backup/states
sudo chmod 755 /var/cache/terraform/plugins
```

### 问题3: Plan数据过大

**症状**: Plan数据保存失败

**解决**: 这是预期行为，Plan数据保存失败不会阻塞任务，会记录警告日志

### 问题4: State保存失败

**症状**: Apply成功但State保存失败

**解决**: 
1. 检查备份目录：`ls -la /var/backup/states/`
2. Workspace会自动锁定
3. 从备份文件手动恢复State

## 📊 性能基准

### 预期性能指标

| 操作 | 预期时间 | 说明 |
|------|----------|------|
| terraform init | < 30s | 首次较慢，后续有缓存 |
| terraform plan | < 30s | 取决于资源数量 |
| terraform apply | < 2min | 取决于资源类型 |
| State保存 | < 5s | 包含重试时间 |
| API响应 | < 200ms | 异步任务立即返回 |

## 🎓 测试技巧

### 1. 使用null_resource测试

null_resource不会创建真实资源，适合测试：

```json
{
  "resource": {
    "null_resource": {
      "test": {
        "triggers": {
          "timestamp": "${timestamp()}"
        }
      }
    }
  }
}
```

### 2. 查看实时日志

```bash
# 持续查询任务状态
watch -n 2 "curl -s http://localhost:8080/api/v1/workspaces/$WS_ID/tasks/$TASK_ID \
  -H 'Authorization: Bearer $TOKEN' | jq '.task.status'"
```

### 3. 查看生成的配置文件

```bash
# Plan任务执行时会创建临时目录
ls -la /tmp/iac-platform/workspaces/$WS_ID/$TASK_ID/

# 查看生成的文件
cat /tmp/iac-platform/workspaces/$WS_ID/$TASK_ID/main.tf.json
```

### 4. 验证State备份

```bash
# 查看备份文件
ls -la /var/backup/states/

# 查看备份内容
cat /var/backup/states/ws_${WS_ID}_task_${TASK_ID}_*.tfstate | jq
```

## 📋 测试报告模板

```markdown
# Terraform执行引擎测试报告

**测试日期**: 2025-10-11  
**测试人员**: [姓名]  
**测试环境**: 开发环境

## 测试结果

### 场景1: 传统方式
- [ ] Workspace创建: /❌
- [ ] Plan执行: /❌
- [ ] Apply执行: /❌
- [ ] State保存: /❌

### 场景2: 资源级别
- [ ] 资源创建: /❌
- [ ] 资源更新: /❌
- [ ] 版本管理: /❌
- [ ] 选择性部署: /❌
- [ ] 快照管理: /❌

### 场景3: 容错机制
- [ ] Plan数据保存重试: /❌
- [ ] State保存重试: /❌
- [ ] 自动备份: /❌
- [ ] 自动锁定: /❌

## 发现的问题

1. [问题描述]
   - 严重程度: 高/中/低
   - 复现步骤: ...
   - 预期结果: ...
   - 实际结果: ...

## 性能数据

| 操作 | 实际时间 | 预期时间 | 状态 |
|------|----------|----------|------|
| terraform init | Xs | <30s | /❌ |
| terraform plan | Xs | <30s | /❌ |
| terraform apply | Xs | <2min | /❌ |

## 总结

- 通过的测试: X/Y
- 发现的问题: X个
- 整体评价: 优秀/良好/需改进
```

## 🔗 相关文档

- [15-terraform-execution-detail.md](./15-terraform-execution-detail.md) - 执行流程设计
- [17-resource-level-version-control.md](./17-resource-level-version-control.md) - 资源版本管理设计
- [terraform-execution-development-progress.md](./terraform-execution-development-progress.md) - 开发进度

---

**下一步**: 根据测试结果进行优化和bug修复
