# Terraform Empty Block Fix

## 问题描述

Workspace在首次运行时，terraform init失败并报错：
```
Error: Error refreshing state: Unsupported state file format: The state file does not have a "version" attribute
```

## 根本原因

前端代码在保存provider配置时，当没有版本约束时会创建一个空的terraform块：

```typescript
// frontend/src/pages/ProviderSettings.tsx - buildSaveData()
return {
  provider: providerMap,
  terraform: Object.keys(requiredProviders).length > 0 ? [
    { required_providers: [requiredProviders] }
  ] : []  // 问题：空数组
};
```

这导致数据库中的provider_config包含：
```json
{
  "provider": {"aws": [{"region": "ap-northeast-1"}]},
  "terraform": []  // 空的terraform块
}
```

当这个空的terraform块被写入provider.tf.json时，Terraform会将其解释为terraform配置块，并尝试初始化backend，进而尝试读取state文件。由于首次运行时没有state文件，就会报错。

## 解决方案

### 1. 前端修复（根本解决方案）

修改`frontend/src/pages/ProviderSettings.tsx`的`buildSaveData`函数：

**修改前：**
```typescript
return {
  provider: providerMap,
  terraform: Object.keys(requiredProviders).length > 0 ? [
    { required_providers: [requiredProviders] }
  ] : []
};
```

**修改后：**
```typescript
// 只有在有版本约束时才包含terraform块
// 空的terraform块会导致Terraform尝试读取backend state，在首次运行时会失败
const result: any = {
  provider: providerMap
};

if (Object.keys(requiredProviders).length > 0) {
  result.terraform = [{ required_providers: [requiredProviders] }];
}

return result;
```

### 2. 后端防御性修复（保留作为安全措施）

在`backend/services/terraform_executor.go`中添加`cleanProviderConfig`函数，在生成provider.tf.json时自动移除空的terraform块：

```go
func (s *TerraformExecutor) cleanProviderConfig(providerConfig map[string]interface{}) map[string]interface{} {
    if providerConfig == nil {
        return providerConfig
    }

    cleaned := make(map[string]interface{})
    for key, value := range providerConfig {
        // 如果是terraform块
        if key == "terraform" {
            // 检查是否为空数组或空对象
            if arr, ok := value.([]interface{}); ok && len(arr) == 0 {
                // 跳过空的terraform块
                log.Printf("Skipping empty terraform block in provider config")
                continue
            }
            if obj, ok := value.(map[string]interface{}); ok && len(obj) == 0 {
                // 跳过空的terraform对象
                log.Printf("Skipping empty terraform object in provider config")
                continue
            }
        }
        cleaned[key] = value
    }

    return cleaned
}
```

应用到3个地方：
- `GenerateConfigFiles` - Local模式
- `GenerateConfigFilesWithLogging` - 所有模式的详细日志版本
- `GenerateConfigFilesFromSnapshot` - Apply阶段从快照生成

### 3. 其他防御性措施

在`TerraformInitWithLogging`中：
- 清理旧的`.terraform`目录
- 添加`-reconfigure`标志
- 在`PrepareStateFileWithLogging`中删除旧的state文件

## 修复现有数据

对于已经存在空terraform块的workspace，可以执行SQL脚本修复：

```sql
-- scripts/fix_empty_terraform_block.sql
UPDATE workspaces 
SET provider_config = jsonb_set(
    provider_config,
    '{terraform}',
    'null'::jsonb
) - 'terraform'
WHERE provider_config ? 'terraform' 
  AND (
    provider_config->'terraform' = '[]'::jsonb 
    OR provider_config->'terraform' = '{}'::jsonb
  );
```

## 测试步骤

1. 重新编译前端：`cd frontend && npm run build`
2. 对于已存在的workspace，执行SQL脚本修复数据库
3. 创建新的workspace，不设置provider版本约束
4. 运行plan任务，验证不再出现state文件错误

## 影响范围

- 所有执行模式（Local、Agent、K8s-agent）
- 所有新创建的workspace
- 所有更新provider配置的操作

## 相关文件

- `frontend/src/pages/ProviderSettings.tsx` - 前端修复
- `backend/services/terraform_executor.go` - 后端防御性修复
- `scripts/fix_empty_terraform_block.sql` - 数据修复脚本
