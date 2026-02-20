# 资源ID规范文档

## 概述

本文档定义了IaC平台中所有资源的ID格式规范。所有资源ID采用统一的字符串格式，便于识别、追踪和管理。

---

## ID格式规范

### 基本格式

```
{type}-{random_string}
```

- **type**: 资源类型前缀（2-5个字符）
- **random_string**: 20位随机字符串（小写字母+数字）

### 示例

```
tfver-ax12jxao191x01s8x9ka  # Terraform版本
var-bx23kybo202y12t9y0lb    # 变量
ws-cx34lzcp313z23u0z1mc     # Workspace
run-dx45maaq424a34v1a2nd    # Run/Task
mod-ex56nbbr535b45w2b3oe    # Module
```

---

## 资源类型前缀

| 资源类型 | 前缀 | 说明 | 示例 |
|---------|------|------|------|
| Terraform Version | `tfver` | Terraform版本 | `tfver-ax12jxao191x01s8x9ka` |
| Variable | `var` | 工作空间变量 | `var-bx23kybo202y12t9y0lb` |
| Workspace | `ws` | 工作空间 | `ws-cx34lzcp313z23u0z1mc` |
| Run/Task | `run` | 运行任务 | `run-dx45maaq424a34v1a2nd` |
| Module | `mod` | 模块 | `mod-ex56nbbr535b45w2b3oe` |
| State Version | `sv` | 状态版本 | `sv-fx67occs646c56x3c4pf` |
| Agent Pool | `apool` | Agent池 | `apool-gx78pddt757d67y4d5qg` |
| Agent | `agent` | Agent | `agent-hx89qeeu868e78z5e6rh` |
| User | `user` | 用户 | `user-ix90rffv979f89a6f7si` |
| Organization | `org` | 组织 | `org-jx01sggw080g90b7g8tj` |
| Team | `team` | 团队 | `team-kx12thhx191h01c8h9uk` |
| VCS Provider | `vcs` | VCS提供商 | `vcs-lx23uiiy202i12d9i0vl` |
| Schema | `schema` | Schema | `schema-mx34vjjz313j23e0j1wm` |

---

## 生成规则

### 随机字符串生成

- **长度**: 20个字符
- **字符集**: 小写字母(a-z) + 数字(0-9)
- **随机性**: 使用加密安全的随机数生成器
- **唯一性**: 确保全局唯一

### Go实现示例

```go
package utils

import (
    "crypto/rand"
    "fmt"
    "math/big"
)

const (
    charset = "abcdefghijklmnopqrstuvwxyz0123456789"
    idLength = 20
)

// GenerateID 生成资源ID
// prefix: 资源类型前缀，如 "tfver", "var", "ws" 等
func GenerateID(prefix string) (string, error) {
    randomStr, err := generateRandomString(idLength)
    if err != nil {
        return "", err
    }
    return fmt.Sprintf("%s-%s", prefix, randomStr), nil
}

// generateRandomString 生成指定长度的随机字符串
func generateRandomString(length int) (string, error) {
    result := make([]byte, length)
    charsetLen := big.NewInt(int64(len(charset)))
    
    for i := 0; i < length; i++ {
        num, err := rand.Int(rand.Reader, charsetLen)
        if err != nil {
            return "", err
        }
        result[i] = charset[num.Int64()]
    }
    
    return string(result), nil
}

// 使用示例
func ExampleUsage() {
    // 生成Terraform版本ID
    tfverID, _ := GenerateID("tfver")
    // 输出: tfver-ax12jxao191x01s8x9ka
    
    // 生成变量ID
    varID, _ := GenerateID("var")
    // 输出: var-bx23kybo202y12t9y0lb
    
    // 生成Workspace ID
    wsID, _ := GenerateID("ws")
    // 输出: ws-cx34lzcp313z23u0z1mc
}
```

### TypeScript实现示例

```typescript
const CHARSET = 'abcdefghijklmnopqrstuvwxyz0123456789';
const ID_LENGTH = 20;

/**
 * 生成资源ID
 * @param prefix 资源类型前缀
 * @returns 格式化的资源ID
 */
export function generateID(prefix: string): string {
  const randomStr = generateRandomString(ID_LENGTH);
  return `${prefix}-${randomStr}`;
}

/**
 * 生成随机字符串
 * @param length 字符串长度
 * @returns 随机字符串
 */
function generateRandomString(length: number): string {
  const array = new Uint8Array(length);
  crypto.getRandomValues(array);
  
  return Array.from(array)
    .map(x => CHARSET[x % CHARSET.length])
    .join('');
}

// 使用示例
const tfverID = generateID('tfver'); // tfver-ax12jxao191x01s8x9ka
const varID = generateID('var');     // var-bx23kybo202y12t9y0lb
const wsID = generateID('ws');       // ws-cx34lzcp313z23u0z1mc
```

---

## 数据库设计

### 字段类型

```sql
-- 所有资源ID字段使用VARCHAR类型
CREATE TABLE terraform_versions (
    id VARCHAR(30) PRIMARY KEY,  -- tfver- + 20字符 = 26字符，留余量30
    version VARCHAR(50) NOT NULL UNIQUE,
    download_url TEXT NOT NULL,
    checksum VARCHAR(64) NOT NULL,
    enabled BOOLEAN DEFAULT false,
    deprecated BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE workspaces (
    id VARCHAR(30) PRIMARY KEY,  -- ws- + 20字符 = 23字符
    name VARCHAR(255) NOT NULL,
    terraform_version VARCHAR(30),  -- 外键引用terraform_versions.id
    -- ...其他字段
);

CREATE TABLE variables (
    id VARCHAR(30) PRIMARY KEY,  -- var- + 20字符 = 24字符
    workspace_id VARCHAR(30) NOT NULL,  -- 外键引用workspaces.id
    key VARCHAR(255) NOT NULL,
    value TEXT,
    -- ...其他字段
);
```

### 索引建议

```sql
-- 主键自动创建索引
-- 外键字段建议创建索引
CREATE INDEX idx_workspaces_terraform_version ON workspaces(terraform_version);
CREATE INDEX idx_variables_workspace_id ON variables(workspace_id);
```

---

## API响应示例

### Terraform版本

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

### Workspace

```json
{
  "id": "ws-cx34lzcp313z23u0z1mc",
  "name": "production",
  "terraform_version": "tfver-ax12jxao191x01s8x9ka",
  "state": "completed",
  "created_at": "2025-01-01T00:00:00Z",
  "updated_at": "2025-01-02T10:00:00Z"
}
```

### Variable

```json
{
  "id": "var-bx23kybo202y12t9y0lb",
  "workspace_id": "ws-cx34lzcp313z23u0z1mc",
  "key": "AWS_REGION",
  "value": "us-east-1",
  "sensitive": false,
  "created_at": "2025-01-01T00:00:00Z",
  "updated_at": "2025-01-01T00:00:00Z"
}
```

---

## 验证规则

### 正则表达式

```regex
# Terraform版本ID
^tfver-[a-z0-9]{20}$

# 变量ID
^var-[a-z0-9]{20}$

# Workspace ID
^ws-[a-z0-9]{20}$

# Run/Task ID
^run-[a-z0-9]{20}$

# 通用格式（任意前缀）
^[a-z]{2,5}-[a-z0-9]{20}$
```

### Go验证函数

```go
import "regexp"

var idPattern = regexp.MustCompile(`^[a-z]{2,5}-[a-z0-9]{20}$`)

// ValidateID 验证ID格式
func ValidateID(id string) bool {
    return idPattern.MatchString(id)
}

// ValidateIDWithPrefix 验证ID格式和前缀
func ValidateIDWithPrefix(id, prefix string) bool {
    pattern := regexp.MustCompile(fmt.Sprintf(`^%s-[a-z0-9]{20}$`, prefix))
    return pattern.MatchString(id)
}
```

### TypeScript验证函数

```typescript
const ID_PATTERN = /^[a-z]{2,5}-[a-z0-9]{20}$/;

/**
 * 验证ID格式
 */
export function validateID(id: string): boolean {
  return ID_PATTERN.test(id);
}

/**
 * 验证ID格式和前缀
 */
export function validateIDWithPrefix(id: string, prefix: string): boolean {
  const pattern = new RegExp(`^${prefix}-[a-z0-9]{20}$`);
  return pattern.test(id);
}
```

---

## 迁移指南

### 从数字ID迁移到字符串ID

1. **数据库迁移**

```sql
-- 1. 添加新的字符串ID列
ALTER TABLE terraform_versions ADD COLUMN new_id VARCHAR(30);

-- 2. 为现有记录生成新ID
UPDATE terraform_versions SET new_id = CONCAT('tfver-', generate_random_string(20));

-- 3. 更新外键引用
ALTER TABLE workspaces ADD COLUMN new_terraform_version VARCHAR(30);
UPDATE workspaces w 
SET new_terraform_version = (
    SELECT new_id FROM terraform_versions t WHERE t.id = w.terraform_version
);

-- 4. 删除旧列，重命名新列
ALTER TABLE terraform_versions DROP COLUMN id;
ALTER TABLE terraform_versions RENAME COLUMN new_id TO id;
ALTER TABLE terraform_versions ADD PRIMARY KEY (id);

ALTER TABLE workspaces DROP COLUMN terraform_version;
ALTER TABLE workspaces RENAME COLUMN new_terraform_version TO terraform_version;
```

2. **后端代码更新**

```go
// 修改Model定义
type TerraformVersion struct {
    ID          string    `json:"id" gorm:"primaryKey;type:varchar(30)"`  // 从int改为string
    Version     string    `json:"version" gorm:"uniqueIndex;not null"`
    DownloadURL string    `json:"download_url" gorm:"not null"`
    Checksum    string    `json:"checksum" gorm:"not null"`
    Enabled     bool      `json:"enabled" gorm:"default:false"`
    Deprecated  bool      `json:"deprecated" gorm:"default:false"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

// 创建时生成ID
func (s *TerraformVersionService) Create(req *CreateRequest) (*TerraformVersion, error) {
    id, err := utils.GenerateID("tfver")
    if err != nil {
        return nil, err
    }
    
    version := &TerraformVersion{
        ID:          id,
        Version:     req.Version,
        DownloadURL: req.DownloadURL,
        Checksum:    req.Checksum,
        Enabled:     req.Enabled,
        Deprecated:  req.Deprecated,
    }
    
    err = s.db.Create(version).Error
    return version, err
}
```

3. **前端代码更新**

```typescript
// 修改接口定义
export interface TerraformVersion {
  id: string;  // 从number改为string
  version: string;
  download_url: string;
  checksum: string;
  enabled: boolean;
  deprecated: boolean;
  created_at: string;
  updated_at: string;
}

// API调用更新
getTerraformVersion: async (id: string) => {  // 参数类型从number改为string
  const response = await api.get(`/admin/terraform-versions/${id}`);
  return response.data;
},
```

---

## 最佳实践

1. **始终使用生成函数**: 不要手动构造ID，使用统一的生成函数
2. **验证ID格式**: 在API入口处验证ID格式
3. **日志记录**: 在日志中包含完整的资源ID便于追踪
4. **错误处理**: ID生成失败时应该有明确的错误处理
5. **文档同步**: 确保API文档中的示例使用正确的ID格式

---

## 常见问题

### Q: 为什么使用字符串ID而不是数字ID？

A: 字符串ID的优势：
- 可读性更好，包含资源类型信息
- 避免ID冲突和猜测
- 便于分布式系统中的ID生成
- 符合行业标准（如Terraform Cloud、AWS等）

### Q: 20位随机字符串的碰撞概率？

A: 使用36个字符（26个字母+10个数字）的20位随机字符串，总共有36^20 ≈ 1.3×10^31种可能性，碰撞概率极低，可以忽略不计。

### Q: 如何处理现有的数字ID？

A: 参考"迁移指南"章节，逐步迁移数据库和代码。建议在新功能中使用新格式，旧功能逐步迁移。

---

## 参考资料

- [Terraform Cloud API Documentation](https://developer.hashicorp.com/terraform/cloud-docs/api-docs)
- [AWS Resource IDs](https://docs.aws.amazon.com/general/latest/gr/aws-arns-and-namespaces.html)
- [UUID vs Custom IDs](https://blog.twitter.com/engineering/en_us/a/2010/announcing-snowflake)

---

## 更新日志

| 版本 | 日期 | 变更说明 |
|------|------|----------|
| v1.0 | 2025-10-09 | 初始版本，定义ID规范 |
