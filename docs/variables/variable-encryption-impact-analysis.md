# 变量加密存储影响分析

## 分析时间
2025-01-03 19:58

## 背景

用户要求确认：如果实现变量加密存储，是否会影响现有的流程。

## 当前状态分析

### 1. 变量存储现状

**数据库模型** (`backend/internal/models/variable.go`):
```go
type WorkspaceVariable struct {
    ID           uint         `json:"id" gorm:"primaryKey"`
    WorkspaceID  string       `json:"workspace_id"`
    Key          string       `json:"key"`
    Value        string       `json:"value,omitempty" gorm:"type:text"` // 明文存储
    VariableType VariableType `json:"variable_type"`
    Sensitive    bool         `json:"sensitive"`
    // ...
}
```

**关键点**:
-  `Value`字段是`string`类型，存储明文
-  `Sensitive`字段标记是否敏感，但不影响存储方式
-  API响应时会隐藏敏感变量的值（`ToResponse()`方法）

### 2. 变量使用流程

#### 2.1 Terraform变量 (VariableTypeTerraform)

**使用位置**:
1. `generateVariablesTFJSON()` - 生成variables.tf.json
2. `generateVariablesTFVars()` - 生成variables.tfvars

**流程**:
```
数据库 → GetWorkspaceVariables() → 直接使用v.Value → 写入tfvars文件
```

#### 2.2 环境变量 (VariableTypeEnvironment)

**使用位置**:
1. `buildEnvironmentVariables()` - 构建环境变量数组

**流程**:
```
数据库 → GetWorkspaceVariables() → 直接使用v.Value → 注入到cmd.Env
```

#### 2.3 Snapshot快照

**使用位置**:
1. `CreateResourceVersionSnapshot()` - 创建快照
2. `GenerateConfigFilesFromSnapshot()` - 从快照恢复

**流程**:
```
数据库 → GetWorkspaceVariables() → 完整对象存入snapshot_variables → 从快照读取 → 使用v.Value
```

## 加密存储影响分析

### 方案A: 透明加密（推荐）

**实现方式**:
- 在GORM的`BeforeSave`钩子中加密
- 在`AfterFind`钩子中解密
- 对应用层完全透明

**代码示例**:
```go
type WorkspaceVariable struct {
    // ... 其他字段
    Value string `json:"value,omitempty" gorm:"type:text"`
}

// BeforeSave 保存前加密
func (v *WorkspaceVariable) BeforeSave(tx *gorm.DB) error {
    if v.Sensitive && v.Value != "" {
        encrypted, err := encrypt(v.Value)
        if err != nil {
            return err
        }
        v.Value = encrypted
    }
    return nil
}

// AfterFind 查询后解密
func (v *WorkspaceVariable) AfterFind(tx *gorm.DB) error {
    if v.Sensitive && v.Value != "" {
        decrypted, err := decrypt(v.Value)
        if err != nil {
            return err
        }
        v.Value = decrypted
    }
    return nil
}
```

**影响评估**:
-  **对现有流程零影响** - 应用层代码无需修改
-  **Terraform变量流程** - 无影响，自动解密后使用
-  **环境变量流程** - 无影响，自动解密后注入
-  **Snapshot流程** - 无影响，快照中存储的是解密后的对象
-  **API响应** - 无影响，`ToResponse()`继续工作
-  **性能影响** - 每次读写都需要加解密，但影响很小

**优点**:
1. 实现简单，代码改动最小
2. 对现有流程完全透明
3. 易于测试和回滚

**缺点**:
1. 内存中是明文（但这是必须的，terraform需要明文）
2. 日志中可能泄露（已通过前面的修复解决）

### 方案B: 显式加密

**实现方式**:
- 添加`EncryptedValue`字段存储密文
- `Value`字段仅用于内存操作
- 需要显式调用加解密函数

**代码示例**:
```go
type WorkspaceVariable struct {
    // ... 其他字段
    Value          string `json:"value,omitempty" gorm:"-"` // 不存储到数据库
    EncryptedValue string `json:"-" gorm:"type:text;column:value"` // 存储密文
}

// 需要在所有使用处显式解密
func (s *Service) GetWorkspaceVariables(...) {
    // ... 查询
    for i := range variables {
        if variables[i].Sensitive {
            variables[i].Value = decrypt(variables[i].EncryptedValue)
        }
    }
}
```

**影响评估**:
- ❌ **需要修改所有使用变量的代码**
- ❌ **Terraform变量流程** - 需要修改生成逻辑
- ❌ **环境变量流程** - 需要修改注入逻辑
- ❌ **Snapshot流程** - 需要特殊处理
- ❌ **测试复杂度增加**

**优点**:
1. 更明确的加密控制
2. 可以选择性加密

**缺点**:
1. 代码改动大
2. 容易遗漏解密步骤
3. 维护成本高

## 推荐方案：透明加密

### 实现步骤

#### 1. 创建加密工具类

```go
// backend/internal/crypto/variable_crypto.go
package crypto

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "errors"
    "io"
    "os"
)

var encryptionKey []byte

func init() {
    // 从环境变量读取密钥
    keyStr := os.Getenv("VARIABLE_ENCRYPTION_KEY")
    if keyStr == "" {
        // 开发环境使用默认密钥（生产环境必须设置）
        keyStr = "default-32-byte-key-for-dev!!"
    }
    encryptionKey = []byte(keyStr)
}

func EncryptValue(plaintext string) (string, error) {
    if plaintext == "" {
        return "", nil
    }
    
    block, err := aes.NewCipher(encryptionKey)
    if err != nil {
        return "", err
    }
    
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }
    
    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return "", err
    }
    
    ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
    return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func DecryptValue(ciphertext string) (string, error) {
    if ciphertext == "" {
        return "", nil
    }
    
    data, err := base64.StdEncoding.DecodeString(ciphertext)
    if err != nil {
        return "", err
    }
    
    block, err := aes.NewCipher(encryptionKey)
    if err != nil {
        return "", err
    }
    
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }
    
    nonceSize := gcm.NonceSize()
    if len(data) < nonceSize {
        return "", errors.New("ciphertext too short")
    }
    
    nonce, ciphertext := data[:nonceSize], data[nonceSize:]
    plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
    if err != nil {
        return "", err
    }
    
    return string(plaintext), nil
}
```

#### 2. 修改模型添加钩子

```go
// backend/internal/models/variable.go
import "iac-platform/internal/crypto"

// BeforeSave 保存前加密敏感变量
func (v *WorkspaceVariable) BeforeSave(tx *gorm.DB) error {
    if v.Sensitive && v.Value != "" {
        encrypted, err := crypto.EncryptValue(v.Value)
        if err != nil {
            return fmt.Errorf("failed to encrypt variable: %w", err)
        }
        v.Value = encrypted
    }
    return nil
}

// AfterFind 查询后解密敏感变量
func (v *WorkspaceVariable) AfterFind(tx *gorm.DB) error {
    if v.Sensitive && v.Value != "" {
        decrypted, err := crypto.DecryptValue(v.Value)
        if err != nil {
            return fmt.Errorf("failed to decrypt variable: %w", err)
        }
        v.Value = decrypted
    }
    return nil
}
```

#### 3. 数据迁移脚本

```sql
-- scripts/encrypt_existing_sensitive_variables.sql
-- 注意：这个脚本需要在Go程序中执行，因为需要调用加密函数

-- 1. 备份现有数据
CREATE TABLE workspace_variables_backup AS 
SELECT * FROM workspace_variables WHERE sensitive = true;

-- 2. 在Go程序中执行加密迁移
-- 见下方Go代码
```

```go
// scripts/encrypt_variables.go
package main

import (
    "database/sql"
    "fmt"
    "log"
    
    _ "github.com/lib/pq"
    "iac-platform/internal/crypto"
)

func main() {
    db, err := sql.Open("postgres", "postgresql://...")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()
    
    // 查询所有敏感变量
    rows, err := db.Query(`
        SELECT id, value 
        FROM workspace_variables 
        WHERE sensitive = true AND value != ''
    `)
    if err != nil {
        log.Fatal(err)
    }
    defer rows.Close()
    
    // 加密并更新
    for rows.Next() {
        var id uint
        var value string
        if err := rows.Scan(&id, &value); err != nil {
            log.Printf("Error scanning row: %v", err)
            continue
        }
        
        encrypted, err := crypto.EncryptValue(value)
        if err != nil {
            log.Printf("Error encrypting variable %d: %v", id, err)
            continue
        }
        
        _, err = db.Exec(`
            UPDATE workspace_variables 
            SET value = $1 
            WHERE id = $2
        `, encrypted, id)
        if err != nil {
            log.Printf("Error updating variable %d: %v", id, err)
            continue
        }
        
        log.Printf("Encrypted variable %d", id)
    }
    
    log.Println("Migration completed")
}
```

### 测试计划

#### 1. 单元测试

```go
func TestVariableEncryption(t *testing.T) {
    // 测试加密解密
    original := "my-secret-value"
    encrypted, err := crypto.EncryptValue(original)
    assert.NoError(t, err)
    assert.NotEqual(t, original, encrypted)
    
    decrypted, err := crypto.DecryptValue(encrypted)
    assert.NoError(t, err)
    assert.Equal(t, original, decrypted)
}

func TestVariableModelHooks(t *testing.T) {
    // 测试GORM钩子
    db := setupTestDB()
    
    variable := &models.WorkspaceVariable{
        Key:       "test_key",
        Value:     "secret-value",
        Sensitive: true,
    }
    
    // 保存（应该加密）
    err := db.Create(variable).Error
    assert.NoError(t, err)
    
    // 直接查询数据库（应该是密文）
    var rawValue string
    db.Raw("SELECT value FROM workspace_variables WHERE id = ?", variable.ID).Scan(&rawValue)
    assert.NotEqual(t, "secret-value", rawValue)
    
    // 通过GORM查询（应该自动解密）
    var loaded models.WorkspaceVariable
    err = db.First(&loaded, variable.ID).Error
    assert.NoError(t, err)
    assert.Equal(t, "secret-value", loaded.Value)
}
```

#### 2. 集成测试

```go
func TestTerraformVariableFlow(t *testing.T) {
    // 测试完整的Terraform变量流程
    // 1. 创建敏感变量
    // 2. 执行Plan任务
    // 3. 验证variables.tfvars包含正确的值
    // 4. 验证terraform能正常执行
}

func TestEnvironmentVariableFlow(t *testing.T) {
    // 测试完整的环境变量流程
    // 1. 创建敏感环境变量
    // 2. 执行Plan任务
    // 3. 验证环境变量正确注入
    // 4. 验证terraform能读取到环境变量
}

func TestSnapshotFlow(t *testing.T) {
    // 测试快照流程
    // 1. 创建敏感变量
    // 2. 创建Plan任务（生成快照）
    // 3. 修改变量
    // 4. 执行Apply（使用快照）
    // 5. 验证使用的是快照中的值
}
```

### 回滚计划

如果加密实现出现问题，可以快速回滚：

```sql
-- 1. 恢复备份数据
UPDATE workspace_variables 
SET value = backup.value
FROM workspace_variables_backup backup
WHERE workspace_variables.id = backup.id;

-- 2. 删除备份表
DROP TABLE workspace_variables_backup;
```

```go
// 3. 移除模型钩子（注释掉BeforeSave和AfterFind方法）
```

## 结论

### 对现有流程的影响

使用**透明加密方案（方案A）**：

 **零影响** - 所有现有流程无需修改
- Terraform变量生成流程
- 环境变量注入流程
- Snapshot快照流程
- API响应处理

 **向后兼容** - 可以平滑迁移
- 新变量自动加密
- 旧变量逐步迁移
- 支持回滚

 **性能影响极小**
- 加解密操作很快（AES-GCM）
- 只对敏感变量加密
- 内存中缓存解密后的值

### 实施建议

1. **分阶段实施**:
   - 阶段1: 实现加密工具类和单元测试
   - 阶段2: 添加模型钩子和集成测试
   - 阶段3: 在测试环境验证
   - 阶段4: 生产环境迁移现有数据
   - 阶段5: 监控和优化

2. **风险控制**:
   - 完整备份数据库
   - 准备回滚脚本
   - 灰度发布
   - 监控错误日志

3. **密钥管理**:
   - 生产环境使用环境变量
   - 考虑使用AWS KMS或HashiCorp Vault
   - 定期轮换密钥

### 最终答案

**变量加密存储不会影响现有流程**，前提是使用透明加密方案（GORM钩子）。这是最安全、最简单、影响最小的实现方式。
