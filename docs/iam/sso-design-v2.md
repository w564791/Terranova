# SSO 单点登录设计方案 v2（多 Provider 支持）

## 一、概述

本文档整合了 `sso.md` 和 `sso-muti-provider.md` 的设计方案，结合当前 `users` 表结构，提供一个支持多 Provider 的 SSO 实现方案。

### 设计目标

1. **多 Provider 支持**：支持 Auth0、Azure AD、Google、GitHub、Okta 等多种身份提供商
2. **用户身份关联**：同一用户可绑定多个 SSO 账号
3. **企业级支持**：支持企业客户直接对接其 IdP
4. **向后兼容**：保持与现有用户系统的兼容性
5. **灵活扩展**：易于添加新的身份提供商

---

## 二、当前用户表分析

### 现有 users 表结构

```go
type User struct {
    ID            string     `gorm:"column:user_id;primaryKey;type:varchar(20)"`
    Username      string     `gorm:"uniqueIndex;not null"`
    Email         string     `gorm:"uniqueIndex;not null"`
    PasswordHash  string     `gorm:"not null"`
    Role          string     `gorm:"default:user"`
    IsActive      bool       `gorm:"default:true"`
    IsSystemAdmin bool       `gorm:"default:false"`
    OAuthProvider string     `gorm:"column:oauth_provider;type:varchar(50)"`  // 当前仅支持单个
    OAuthID       string     `gorm:"column:oauth_id;type:varchar(200)"`       // 当前仅支持单个
    LastLoginAt   *time.Time
    CreatedAt     time.Time
    UpdatedAt     time.Time
    // MFA 相关字段...
}
```

### 现有设计的局限性

1. **单 Provider 限制**：`OAuthProvider` 和 `OAuthID` 字段只能存储一个身份提供商
2. **无法多账号绑定**：用户无法同时绑定 Google 和 GitHub 账号
3. **企业 SSO 困难**：无法支持多租户企业 Azure AD

---

## 三、数据库设计方案

### 方案选择：独立身份表 + Provider 配置表

为了支持多 Provider，我们采用独立的身份关联表设计，同时保留 users 表的 `OAuthProvider` 和 `OAuthID` 字段作为主要登录方式的快速查询。

### 3.1 新增表：user_identities（用户身份关联表）

```sql
CREATE TABLE user_identities (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(20) NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    
    -- Provider 信息
    provider VARCHAR(50) NOT NULL,           -- 'auth0', 'azure_ad', 'google', 'github', 'okta'
    provider_user_id VARCHAR(255) NOT NULL,  -- Provider 返回的唯一标识
    provider_email VARCHAR(255),             -- Provider 返回的邮箱
    provider_name VARCHAR(255),              -- Provider 返回的显示名称
    provider_avatar VARCHAR(500),            -- Provider 返回的头像 URL
    
    -- 元数据
    raw_data JSONB,                          -- 原始用户信息（调试用）
    access_token_encrypted TEXT,             -- 加密存储的 access_token（可选）
    refresh_token_encrypted TEXT,            -- 加密存储的 refresh_token（可选）
    token_expires_at TIMESTAMP,              -- Token 过期时间
    
    -- 状态
    is_primary BOOLEAN DEFAULT FALSE,        -- 是否为主要登录方式
    is_verified BOOLEAN DEFAULT TRUE,        -- 邮箱是否已验证
    last_used_at TIMESTAMP,                  -- 最后使用时间
    
    -- 审计
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    -- 唯一约束：同一 provider 的同一用户只能绑定一次
    UNIQUE(provider, provider_user_id)
);

CREATE INDEX idx_user_identities_user_id ON user_identities(user_id);
CREATE INDEX idx_user_identities_provider ON user_identities(provider);
CREATE INDEX idx_user_identities_email ON user_identities(provider_email);

COMMENT ON TABLE user_identities IS '用户身份关联表，支持多 Provider 绑定';
COMMENT ON COLUMN user_identities.provider IS '身份提供商：auth0, azure_ad, google, github, okta 等';
COMMENT ON COLUMN user_identities.provider_user_id IS 'Provider 返回的唯一用户标识（如 Auth0 的 sub）';
COMMENT ON COLUMN user_identities.is_primary IS '是否为主要登录方式，用于显示和默认选择';
```

### 3.2 新增表：sso_providers（SSO 配置表）

```sql
CREATE TABLE sso_providers (
    id SERIAL PRIMARY KEY,
    
    -- 基本信息
    provider_key VARCHAR(50) NOT NULL UNIQUE,  -- 唯一标识，用于路由：'auth0', 'azure_company_a'
    provider_type VARCHAR(30) NOT NULL,        -- 类型：'auth0', 'azure_ad', 'google', 'github', 'okta', 'saml'
    display_name VARCHAR(100) NOT NULL,        -- 显示名称：'使用 Google 登录'
    description TEXT,                          -- 描述
    icon VARCHAR(50),                          -- 图标名称：'google', 'microsoft', 'github'
    
    -- OAuth 配置（JSONB 存储，便于不同 Provider 的差异化配置）
    oauth_config JSONB NOT NULL,
    -- 示例：
    -- Auth0: {"domain": "xxx.auth0.com", "client_id": "...", "client_secret_encrypted": "..."}
    -- Azure AD: {"tenant_id": "...", "client_id": "...", "client_secret_encrypted": "..."}
    -- Google: {"client_id": "...", "client_secret_encrypted": "..."}
    
    -- 端点配置（可选，某些 Provider 需要自定义）
    authorize_endpoint VARCHAR(500),
    token_endpoint VARCHAR(500),
    userinfo_endpoint VARCHAR(500),
    
    -- 回调配置
    callback_url VARCHAR(500) NOT NULL,        -- 回调 URL
    allowed_callback_urls TEXT[],              -- 允许的回调 URL 列表
    
    -- 用户管理配置
    auto_create_user BOOLEAN DEFAULT TRUE,     -- 是否自动创建用户
    default_role VARCHAR(50) DEFAULT 'user',   -- 默认角色
    allowed_domains TEXT[],                    -- 允许的邮箱域名（企业 SSO 用）
    
    -- 属性映射（不同 Provider 返回字段名不同）
    attribute_mapping JSONB DEFAULT '{"user_id": "sub", "email": "email", "name": "name", "avatar": "picture"}',
    
    -- 状态
    is_enabled BOOLEAN DEFAULT TRUE,
    is_enterprise BOOLEAN DEFAULT FALSE,       -- 是否为企业专用
    organization_id VARCHAR(20),               -- 关联的组织（企业 SSO）
    
    -- 排序和显示
    display_order INT DEFAULT 0,               -- 显示顺序
    show_on_login_page BOOLEAN DEFAULT TRUE,   -- 是否在登录页显示
    
    -- 审计
    created_by VARCHAR(20),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE sso_providers IS 'SSO 身份提供商配置表';
COMMENT ON COLUMN sso_providers.provider_key IS '唯一标识，用于 API 路由';
COMMENT ON COLUMN sso_providers.provider_type IS 'Provider 类型，决定使用哪个处理器';
COMMENT ON COLUMN sso_providers.oauth_config IS 'OAuth 配置，JSON 格式存储';
COMMENT ON COLUMN sso_providers.attribute_mapping IS '用户属性映射配置';
```

### 3.3 新增表：sso_login_logs（SSO 登录日志表）

```sql
CREATE TABLE sso_login_logs (
    id BIGSERIAL PRIMARY KEY,
    
    -- 关联信息
    user_id VARCHAR(20),                       -- 登录成功后的用户 ID
    identity_id INT,                           -- 关联的 user_identities.id
    provider_key VARCHAR(50) NOT NULL,         -- 使用的 Provider
    
    -- 登录信息
    provider_user_id VARCHAR(255),             -- Provider 返回的用户 ID
    provider_email VARCHAR(255),               -- Provider 返回的邮箱
    
    -- 状态
    status VARCHAR(20) NOT NULL,               -- 'success', 'failed', 'user_created', 'user_linked'
    error_message TEXT,                        -- 失败原因
    
    -- 请求信息
    ip_address VARCHAR(45),
    user_agent TEXT,
    
    -- 时间
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_sso_login_logs_user_id ON sso_login_logs(user_id);
CREATE INDEX idx_sso_login_logs_provider ON sso_login_logs(provider_key);
CREATE INDEX idx_sso_login_logs_created_at ON sso_login_logs(created_at);

COMMENT ON TABLE sso_login_logs IS 'SSO 登录日志，用于审计和问题排查';
```

### 3.4 修改 users 表

保留现有字段，添加注释说明其用途：

```sql
-- 添加注释
COMMENT ON COLUMN users.oauth_provider IS '主要 OAuth 提供商（快速查询用），详细信息见 user_identities 表';
COMMENT ON COLUMN users.oauth_id IS '主要 OAuth 用户 ID（快速查询用），详细信息见 user_identities 表';

-- 可选：添加字段标识用户是否为 SSO 用户
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_sso_user BOOLEAN DEFAULT FALSE;
COMMENT ON COLUMN users.is_sso_user IS '是否为 SSO 用户（无密码登录）';
```

---

## 四、数据模型设计

### 4.1 Go 模型定义

```go
// backend/internal/models/user_identity.go

package models

import (
    "time"
    "encoding/json"
)

// UserIdentity 用户身份关联
type UserIdentity struct {
    ID                    int64           `json:"id" gorm:"primaryKey"`
    UserID                string          `json:"user_id" gorm:"type:varchar(20);not null;index"`
    
    // Provider 信息
    Provider              string          `json:"provider" gorm:"type:varchar(50);not null"`
    ProviderUserID        string          `json:"provider_user_id" gorm:"type:varchar(255);not null"`
    ProviderEmail         string          `json:"provider_email" gorm:"type:varchar(255)"`
    ProviderName          string          `json:"provider_name" gorm:"type:varchar(255)"`
    ProviderAvatar        string          `json:"provider_avatar" gorm:"type:varchar(500)"`
    
    // 元数据
    RawData               json.RawMessage `json:"-" gorm:"type:jsonb"`
    AccessTokenEncrypted  string          `json:"-" gorm:"type:text"`
    RefreshTokenEncrypted string          `json:"-" gorm:"type:text"`
    TokenExpiresAt        *time.Time      `json:"-"`
    
    // 状态
    IsPrimary             bool            `json:"is_primary" gorm:"default:false"`
    IsVerified            bool            `json:"is_verified" gorm:"default:true"`
    LastUsedAt            *time.Time      `json:"last_used_at"`
    
    // 审计
    CreatedAt             time.Time       `json:"created_at"`
    UpdatedAt             time.Time       `json:"updated_at"`
    
    // 关联（注意：User 模型的主键列名是 user_id）
    User                  *User           `json:"user,omitempty" gorm:"foreignKey:UserID;references:user_id"`
}

func (UserIdentity) TableName() string {
    return "user_identities"
}

// UniqueIndexes 定义唯一索引
// 1. (provider, provider_user_id) - 同一 Provider 的同一用户只能存在一条记录
// 2. (user_id, provider) - 同一用户在同一 Provider 只能绑定一个账号（可选，根据业务需求）
```

```go
// backend/internal/models/sso_login_log.go

package models

import "time"

// SSOLoginLog SSO 登录日志
type SSOLoginLog struct {
    ID             int64      `json:"id" gorm:"primaryKey"`
    UserID         string     `json:"user_id" gorm:"type:varchar(20);index"`
    IdentityID     *int64     `json:"identity_id"`
    ProviderKey    string     `json:"provider_key" gorm:"type:varchar(50);not null;index"`
    ProviderUserID string     `json:"provider_user_id" gorm:"type:varchar(255)"`
    ProviderEmail  string     `json:"provider_email" gorm:"type:varchar(255)"`
    Status         string     `json:"status" gorm:"type:varchar(20);not null"` // success, failed, user_created, user_linked
    ErrorMessage   string     `json:"error_message" gorm:"type:text"`
    IPAddress      string     `json:"ip_address" gorm:"type:varchar(45)"`
    UserAgent      string     `json:"user_agent" gorm:"type:text"`
    CreatedAt      time.Time  `json:"created_at"`
}

func (SSOLoginLog) TableName() string {
    return "sso_login_logs"
}
```

```go
// backend/internal/models/sso_provider.go

package models

import (
    "time"
    "encoding/json"
)

// SSOProvider SSO 配置
type SSOProvider struct {
    ID                  int64           `json:"id" gorm:"primaryKey"`
    
    // 基本信息
    ProviderKey         string          `json:"provider_key" gorm:"type:varchar(50);uniqueIndex;not null"`
    ProviderType        string          `json:"provider_type" gorm:"type:varchar(30);not null"`
    DisplayName         string          `json:"display_name" gorm:"type:varchar(100);not null"`
    Description         string          `json:"description" gorm:"type:text"`
    Icon                string          `json:"icon" gorm:"type:varchar(50)"`
    
    // OAuth 配置
    OAuthConfig         json.RawMessage `json:"-" gorm:"type:jsonb;not null"`
    
    // 端点配置
    AuthorizeEndpoint   string          `json:"authorize_endpoint" gorm:"type:varchar(500)"`
    TokenEndpoint       string          `json:"token_endpoint" gorm:"type:varchar(500)"`
    UserinfoEndpoint    string          `json:"userinfo_endpoint" gorm:"type:varchar(500)"`
    
    // 回调配置
    CallbackURL         string          `json:"callback_url" gorm:"type:varchar(500);not null"`
    AllowedCallbackURLs []string        `json:"allowed_callback_urls" gorm:"type:text[]"`
    
    // 用户管理配置
    AutoCreateUser      bool            `json:"auto_create_user" gorm:"default:true"`
    DefaultRole         string          `json:"default_role" gorm:"type:varchar(50);default:user"`
    AllowedDomains      []string        `json:"allowed_domains" gorm:"type:text[]"`
    
    // 属性映射
    AttributeMapping    json.RawMessage `json:"attribute_mapping" gorm:"type:jsonb"`
    
    // 状态
    IsEnabled           bool            `json:"is_enabled" gorm:"default:true"`
    IsEnterprise        bool            `json:"is_enterprise" gorm:"default:false"`
    OrganizationID      string          `json:"organization_id" gorm:"type:varchar(20)"`
    
    // 显示
    DisplayOrder        int             `json:"display_order" gorm:"default:0"`
    ShowOnLoginPage     bool            `json:"show_on_login_page" gorm:"default:true"`
    
    // 审计
    CreatedBy           string          `json:"created_by" gorm:"type:varchar(20)"`
    CreatedAt           time.Time       `json:"created_at"`
    UpdatedAt           time.Time       `json:"updated_at"`
}

func (SSOProvider) TableName() string {
    return "sso_providers"
}

// OAuthConfigData OAuth 配置结构
type OAuthConfigData struct {
    ClientID              string   `json:"client_id"`
    ClientSecretEncrypted string   `json:"client_secret_encrypted"`
    Domain                string   `json:"domain,omitempty"`      // Auth0 特有
    Audience              string   `json:"audience,omitempty"`    // Auth0 特有
    TenantID              string   `json:"tenant_id,omitempty"`   // Azure AD 特有
    OrgURL                string   `json:"org_url,omitempty"`     // Okta 特有
    Scopes                []string `json:"scopes,omitempty"`
}

// AttributeMappingData 属性映射结构
type AttributeMappingData struct {
    UserID  string `json:"user_id"`  // 默认 "sub"
    Email   string `json:"email"`    // 默认 "email"
    Name    string `json:"name"`     // 默认 "name"
    Avatar  string `json:"avatar"`   // 默认 "picture"
}
```

---

## 五、认证流程设计

### 5.1 整体流程图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              SSO 登录流程                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────────────────┐  │
│  │  前端    │    │  后端    │    │ Provider │    │      数据库          │  │
│  └────┬─────┘    └────┬─────┘    └────┬─────┘    └──────────┬───────────┘  │
│       │               │               │                     │              │
│       │ 1. 获取 Provider 列表         │                     │              │
│       │──────────────>│               │                     │              │
│       │               │ 查询 sso_providers                  │              │
│       │               │────────────────────────────────────>│              │
│       │               │<────────────────────────────────────│              │
│       │<──────────────│               │                     │              │
│       │               │               │                     │              │
│       │ 2. 用户点击登录按钮           │                     │              │
│       │──────────────>│               │                     │              │
│       │               │ 生成授权 URL   │                     │              │
│       │<──────────────│               │                     │              │
│       │               │               │                     │              │
│       │ 3. 重定向到 Provider          │                     │              │
│       │──────────────────────────────>│                     │              │
│       │               │               │ 用户登录            │              │
│       │               │               │                     │              │
│       │ 4. Provider 回调（带 code）   │                     │              │
│       │<──────────────────────────────│                     │              │
│       │               │               │                     │              │
│       │ 5. 发送 code 到后端           │                     │              │
│       │──────────────>│               │                     │              │
│       │               │ 6. 用 code 换 token                 │              │
│       │               │──────────────>│                     │              │
│       │               │<──────────────│                     │              │
│       │               │               │                     │              │
│       │               │ 7. 获取用户信息                     │              │
│       │               │──────────────>│                     │              │
│       │               │<──────────────│                     │              │
│       │               │               │                     │              │
│       │               │ 8. 查询/创建用户                    │              │
│       │               │────────────────────────────────────>│              │
│       │               │<────────────────────────────────────│              │
│       │               │               │                     │              │
│       │               │ 9. 创建/更新 user_identity          │              │
│       │               │────────────────────────────────────>│              │
│       │               │<────────────────────────────────────│              │
│       │               │               │                     │              │
│       │ 10. 返回 JWT + 用户信息       │                     │              │
│       │<──────────────│               │                     │              │
│       │               │               │                     │              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.2 API 端点设计

```
# SSO 相关 API

## 公开端点
GET  /api/auth/sso/providers              # 获取可用的 SSO Provider 列表
GET  /api/auth/sso/:provider/login        # 获取授权 URL 并重定向
GET  /api/auth/sso/:provider/callback     # OAuth 回调处理
POST /api/auth/sso/:provider/callback     # OAuth 回调处理（POST 方式）

## 需要认证的端点
GET  /api/auth/sso/identities             # 获取当前用户绑定的身份列表
POST /api/auth/sso/identities/link        # 绑定新的 SSO 身份
DELETE /api/auth/sso/identities/:id       # 解绑 SSO 身份
PUT  /api/auth/sso/identities/:id/primary # 设置主要登录方式

## 管理端点（需要管理员权限）
GET    /api/admin/sso/providers           # 获取所有 Provider 配置
POST   /api/admin/sso/providers           # 创建 Provider 配置
PUT    /api/admin/sso/providers/:id       # 更新 Provider 配置
DELETE /api/admin/sso/providers/:id       # 删除 Provider 配置
GET    /api/admin/sso/logs                # 获取 SSO 登录日志
```

### 5.3 Provider 接口抽象

```go
// backend/internal/services/sso/provider.go

package sso

import "context"

// StandardUserInfo 标准化的用户信息
type StandardUserInfo struct {
    ProviderUserID string                 `json:"provider_user_id"`
    Email          string                 `json:"email"`
    EmailVerified  bool                   `json:"email_verified"`
    Name           string                 `json:"name"`
    Avatar         string                 `json:"avatar"`
    RawData        map[string]interface{} `json:"raw_data"`
}

// OAuthToken OAuth Token
type OAuthToken struct {
    AccessToken  string `json:"access_token"`
    RefreshToken string `json:"refresh_token"`
    TokenType    string `json:"token_type"`
    ExpiresIn    int    `json:"expires_in"`
    IDToken      string `json:"id_token,omitempty"`
}

// Provider SSO Provider 接口
type Provider interface {
    GetType() string
    GetAuthorizationURL(state string, redirectURL string) (string, error)
    ExchangeCode(ctx context.Context, code string, redirectURL string) (*OAuthToken, error)
    GetUserInfo(ctx context.Context, token *OAuthToken) (*StandardUserInfo, error)
    RefreshToken(ctx context.Context, refreshToken string) (*OAuthToken, error)
    ValidateToken(ctx context.Context, token string) (bool, error)
}

// ProviderFactory Provider 工厂
type ProviderFactory interface {
    CreateProvider(config *models.SSOProvider) (Provider, error)
}
```

---

## 六、各 Provider 实现要点

### 6.1 Auth0 Provider

| 配置项 | 值 |
|--------|-----|
| authorize | `https://{domain}/authorize` |
| token | `https://{domain}/oauth/token` |
| userinfo | `https://{domain}/userinfo` |
| user_id 字段 | `sub` (格式: `auth0\|abc123`) |

### 6.2 Azure AD Provider

| 配置项 | 值 |
|--------|-----|
| authorize | `https://login.microsoftonline.com/{tenant_id}/oauth2/v2.0/authorize` |
| token | `https://login.microsoftonline.com/{tenant_id}/oauth2/v2.0/token` |
| userinfo | `https://graph.microsoft.com/v1.0/me` |
| user_id 字段 | `oid` (不是 sub) |

### 6.3 Google Provider

| 配置项 | 值 |
|--------|-----|
| authorize | `https://accounts.google.com/o/oauth2/v2/auth` |
| token | `https://oauth2.googleapis.com/token` |
| userinfo | `https://www.googleapis.com/oauth2/v3/userinfo` |
| user_id 字段 | `sub` |

### 6.4 GitHub Provider

| 配置项 | 值 |
|--------|-----|
| authorize | `https://github.com/login/oauth/authorize` |
| token | `https://github.com/login/oauth/access_token` |
| userinfo | `https://api.github.com/user` |
| user_id 字段 | `id` (数字类型) |
| 注意 | 邮箱可能为 null，需要额外请求 `/user/emails` |

### 6.5 Okta Provider

| 配置项 | 值 |
|--------|-----|
| authorize | `https://{org_url}/oauth2/default/v1/authorize` |
| token | `https://{org_url}/oauth2/default/v1/token` |
| userinfo | `https://{org_url}/oauth2/default/v1/userinfo` |
| user_id 字段 | `sub` |

---

## 七、用户绑定与登录逻辑

### 7.1 登录流程决策树

```
收到 Provider 回调
    │
    ▼
提取 provider_user_id 和 email
    │
    ▼
查询 user_identities 表 (provider + provider_user_id)
    │
    ├─── 找到记录 ──────────────────────────────┐
    │                                           │
    │                                           ▼
    │                                     获取关联的 User
    │                                           │
    │                                           ▼
    │                                     更新 last_used_at
    │                                           │
    │                                           ▼
    │                                     生成 JWT，返回登录成功
    │
    └─── 未找到记录 ─────────────────────────────┐
                                                │
                                                ▼
                                          查询 users 表（按 email）
                                                │
                    ┌───────────────────────────┴───────────────────────────┐
                    │                                                       │
                    ▼                                                       ▼
              找到用户                                                  未找到用户
                    │                                                       │
                    ▼                                                       ▼
              创建 user_identity                                    检查 auto_create_user
              关联到现有用户                                                │
                    │                                   ┌───────────────────┴───────────────────┐
                    ▼                                   │                                       │
              生成 JWT                                  ▼                                       ▼
              返回登录成功                        允许自动创建                             不允许自动创建
                                                        │                                       │
                                                        ▼                                       ▼
                                                  创建新用户                               返回错误
                                                  创建 user_identity                       "用户未注册"
                                                        │
                                                        ▼
                                                  生成 JWT
                                                  返回登录成功
```

### 7.2 核心登录处理逻辑

```go
// HandleCallback 处理 SSO 回调
func (s *AuthService) HandleCallback(ctx context.Context, providerKey string, code string) (*LoginResult, error) {
    // 1. 获取 Provider 配置
    providerConfig, err := s.getProviderConfig(providerKey)
    if err != nil {
        return nil, err
    }
    
    // 2. 创建 Provider 实例
    provider, err := s.providerFactory.CreateProvider(providerConfig)
    if err != nil {
        return nil, err
    }
    
    // 3. 用 code 换取 token
    token, err := provider.ExchangeCode(ctx, code, providerConfig.CallbackURL)
    if err != nil {
        return nil, err
    }
    
    // 4. 获取用户信息
    userInfo, err := provider.GetUserInfo(ctx, token)
    if err != nil {
        return nil, err
    }
    
    // 5. 验证邮箱域名（企业 SSO）
    if len(providerConfig.AllowedDomains) > 0 {
        if !s.isEmailDomainAllowed(userInfo.Email, providerConfig.AllowedDomains) {
            return nil, errors.New("email domain not allowed")
        }
    }
    
    // 6. 查找或创建用户
    user, identity, isNewUser, err := s.findOrCreateUser(ctx, providerConfig, userInfo)
    if err != nil {
        return nil, err
    }
    
    // 7. 更新身份信息
    identity.LastUsedAt = timePtr(time.Now())
    s.db.Save(identity)
    
    // 8. 记录登录日志
    s.logLogin(ctx, user, identity, providerKey, isNewUser)
    
    // 9. 生成 JWT
    jwtToken, err := s.jwtService.GenerateToken(user)
    if err != nil {
        return nil, err
    }
    
    return &LoginResult{
        Token:     jwtToken,
        User:      user,
        IsNewUser: isNewUser,
    }, nil
}
```

---

## 八、账号绑定功能

### 8.1 绑定新身份

用户登录后，可以在账号设置页面绑定其他 SSO 账号：

```go
// LinkIdentity 为已登录用户绑定新的 SSO 身份
func (s *AuthService) LinkIdentity(ctx context.Context, userID string, providerKey string, code string) error {
    // 1. 获取 Provider 配置
    providerConfig, _ := s.getProviderConfig(providerKey)
    
    // 2. 创建 Provider 实例并获取用户信息
    provider, _ := s.providerFactory.CreateProvider(providerConfig)
    token, _ := provider.ExchangeCode(ctx, code, providerConfig.CallbackURL)
    userInfo, _ := provider.GetUserInfo(ctx, token)
    
    // 3. 检查该身份是否已被其他用户绑定
    var existingIdentity models.UserIdentity
    err := s.db.Where("provider = ? AND provider_user_id = ?", 
        providerKey, userInfo.ProviderUserID).First(&existingIdentity).Error
    if err == nil {
        if existingIdentity.UserID != userID {
            return errors.New("this identity is already linked to another account")
        }
        return errors.New("this identity is already linked to your account")
    }
    
    // 4. 创建新的身份关联
    identity := models.UserIdentity{
        UserID:         userID,
        Provider:       providerKey,
        ProviderUserID: userInfo.ProviderUserID,
        ProviderEmail:  userInfo.Email,
        ProviderName:   userInfo.Name,
        ProviderAvatar: userInfo.Avatar,
        IsVerified:     userInfo.EmailVerified,
        IsPrimary:      false,
    }
    
    return s.db.Create(&identity).Error
}
```

### 8.2 解绑身份

```go
// UnlinkIdentity 解绑 SSO 身份
func (s *AuthService) UnlinkIdentity(ctx context.Context, userID string, identityID int64) error {
    // 1. 查询身份记录
    var identity models.UserIdentity
    if err := s.db.First(&identity, identityID).Error; err != nil {
        return err
    }
    
    // 2. 验证所有权
    if identity.UserID != userID {
        return errors.New("identity does not belong to this user")
    }
    
    // 3. 检查是否为唯一登录方式
    var count int64
    s.db.Model(&models.UserIdentity{}).Where("user_id = ?", userID).Count(&count)
    
    // 检查用户是否有密码
    var user models.User
    s.db.First(&user, "user_id = ?", userID)
    
    if count == 1 && user.PasswordHash == "" {
        return errors.New("cannot unlink the only login method")
    }
    
    // 4. 如果是主要方式，需要先设置其他方式为主要
    if identity.IsPrimary && count > 1 {
        var otherIdentity models.UserIdentity
        s.db.Where("user_id = ? AND id != ?", userID, identityID).First(&otherIdentity)
        otherIdentity.IsPrimary = true
        s.db.Save(&otherIdentity)
    }
    
    // 5. 删除身份记录
    return s.db.Delete(&identity).Error
}
```

---

## 九、安全考虑

### 9.1 State 参数防 CSRF

```go
// 生成 state 参数
func generateState() string {
    b := make([]byte, 32)
    rand.Read(b)
    return base64.URLEncoding.EncodeToString(b)
}

// 存储 state（使用 Redis）
func (s *AuthService) storeState(state string, data StateData) error {
    return s.redis.Set(ctx, "sso_state:"+state, data, 10*time.Minute).Err()
}

// 验证 state
func (s *AuthService) validateState(state string) (*StateData, error) {
    var data StateData
    err := s.redis.Get(ctx, "sso_state:"+state).Scan(&data)
    if err != nil {
        return nil, errors.New("invalid or expired state")
    }
    s.redis.Del(ctx, "sso_state:"+state) // 使用后删除
    return &data, nil
}
```

### 9.2 Token 加密存储

**重要**：SSO Token 加密必须使用项目现有的 `internal/crypto` 包，该包基于 JWT_SECRET 派生 AES-256-GCM 加密密钥，与其他敏感数据（如 workspace variables、MFA secret、notification secret 等）使用相同的加密方案。

```go
// 使用项目现有的 crypto 包加密 OAuth Token
import "iac-platform/internal/crypto"

// 加密 OAuth Token（存储到数据库）
func encryptOAuthToken(token string) (string, error) {
    return crypto.EncryptValue(token)
}

// 解密 OAuth Token（从数据库读取）
func decryptOAuthToken(encryptedToken string) (string, error) {
    return crypto.DecryptValue(encryptedToken)
}

// 示例：保存用户身份时加密 token
func (s *AuthService) saveUserIdentity(identity *models.UserIdentity, token *OAuthToken) error {
    // 加密 access_token
    if token.AccessToken != "" {
        encrypted, err := crypto.EncryptValue(token.AccessToken)
        if err != nil {
            return fmt.Errorf("failed to encrypt access token: %w", err)
        }
        identity.AccessTokenEncrypted = encrypted
    }
    
    // 加密 refresh_token
    if token.RefreshToken != "" {
        encrypted, err := crypto.EncryptValue(token.RefreshToken)
        if err != nil {
            return fmt.Errorf("failed to encrypt refresh token: %w", err)
        }
        identity.RefreshTokenEncrypted = encrypted
    }
    
    return s.db.Save(identity).Error
}
```

**crypto 包说明**（`backend/internal/crypto/variable_crypto.go`）：

```go
// 加密方案：AES-256-GCM
// 密钥来源：SHA256(JWT_SECRET) -> 32 字节密钥
// 输出格式：Base64(nonce + ciphertext)

// 主要函数：
crypto.EncryptValue(plaintext string) (string, error)  // 加密
crypto.DecryptValue(ciphertext string) (string, error) // 解密
crypto.IsEncrypted(value string) bool                  // 检查是否已加密
```

**注意事项**：
1. 所有敏感数据（OAuth Token、Client Secret 等）必须使用 `crypto.EncryptValue()` 加密后存储
2. 读取时使用 `crypto.DecryptValue()` 解密
3. 加密密钥由 JWT_SECRET 派生，更换 JWT_SECRET 会导致无法解密旧数据
4. 该加密方案已在以下场景使用：
   - Workspace Variables（敏感变量）
   - MFA Secret（双因素认证密钥）
   - Notification Secret（通知配置密钥）
   - Run Task HMAC Key（运行任务签名密钥）
   - CMDB External Source Headers（CMDB 外部源认证头）

### 9.3 邮箱域名验证

```go
// 验证邮箱域名
func (s *AuthService) isEmailDomainAllowed(email string, allowedDomains []string) bool {
    if len(allowedDomains) == 0 {
        return true
    }
    parts := strings.Split(email, "@")
    if len(parts) != 2 {
        return false
    }
    domain := strings.ToLower(parts[1])
    for _, allowed := range allowedDomains {
        if strings.ToLower(allowed) == domain {
            return true
        }
    }
    return false
}
```

---

## 十、实施计划

### 第一阶段：基础架构（1 周）

- [ ] 创建数据库表（user_identities, sso_providers, sso_login_logs）
- [ ] 实现 Provider 接口和工厂模式
- [ ] 实现 Auth0 Provider
- [ ] 实现基础的登录回调处理

### 第二阶段：核心功能（1 周）

- [ ] 实现用户查找/创建逻辑
- [ ] 实现 JWT 生成和验证
- [ ] 实现前端登录页面
- [ ] 实现回调处理页面
- [ ] 集成测试

### 第三阶段：账号管理（1 周）

- [ ] 实现账号绑定功能
- [ ] 实现账号解绑功能
- [ ] 实现主要登录方式设置
- [ ] 前端账号设置页面

### 第四阶段：企业功能（1-2 周）

- [ ] 实现 Azure AD Provider
- [ ] 实现 Google Provider
- [ ] 实现 GitHub Provider
- [ ] 管理员 Provider 配置界面
- [ ] 邮箱域名限制功能

### 第五阶段：优化和安全（1 周）

- [ ] State 参数防 CSRF
- [ ] Token 加密存储
- [ ] 登录日志和审计
- [ ] 性能优化
- [ ] 文档完善

---

## 十一、数据库迁移脚本

```sql
-- scripts/create_sso_tables.sql

-- 1. 创建 user_identities 表
CREATE TABLE IF NOT EXISTS user_identities (
    id BIGSERIAL PRIMARY KEY,                -- 使用 BIGSERIAL 与 Go 模型 int64 一致
    user_id VARCHAR(20) NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    provider VARCHAR(50) NOT NULL,
    provider_user_id VARCHAR(255) NOT NULL,
    provider_email VARCHAR(255),
    provider_name VARCHAR(255),
    provider_avatar VARCHAR(500),
    raw_data JSONB,
    access_token_encrypted TEXT,
    refresh_token_encrypted TEXT,
    token_expires_at TIMESTAMP,
    is_primary BOOLEAN DEFAULT FALSE,
    is_verified BOOLEAN DEFAULT TRUE,
    last_used_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    -- 唯一约束1：同一 provider 的同一 provider_user_id 只能存在一条记录
    UNIQUE(provider, provider_user_id),
    -- 唯一约束2：同一用户在同一 provider 只能绑定一个账号
    UNIQUE(user_id, provider)
);

CREATE INDEX IF NOT EXISTS idx_user_identities_user_id ON user_identities(user_id);
CREATE INDEX IF NOT EXISTS idx_user_identities_provider ON user_identities(provider);
CREATE INDEX IF NOT EXISTS idx_user_identities_email ON user_identities(provider_email);

-- 2. 创建 sso_providers 表
CREATE TABLE IF NOT EXISTS sso_providers (
    id SERIAL PRIMARY KEY,
    provider_key VARCHAR(50) NOT NULL UNIQUE,
    provider_type VARCHAR(30) NOT NULL,
    display_name VARCHAR(100) NOT NULL,
    description TEXT,
    icon VARCHAR(50),
    oauth_config JSONB NOT NULL,
    authorize_endpoint VARCHAR(500),
    token_endpoint VARCHAR(500),
    userinfo_endpoint VARCHAR(500),
    callback_url VARCHAR(500) NOT NULL,
    allowed_callback_urls TEXT[],
    auto_create_user BOOLEAN DEFAULT TRUE,
    default_role VARCHAR(50) DEFAULT 'user',
    allowed_domains TEXT[],
    attribute_mapping JSONB DEFAULT '{"user_id": "sub", "email": "email", "name": "name", "avatar": "picture"}',
    is_enabled BOOLEAN DEFAULT TRUE,
    is_enterprise BOOLEAN DEFAULT FALSE,
    organization_id VARCHAR(20),
    display_order INT DEFAULT 0,
    show_on_login_page BOOLEAN DEFAULT TRUE,
    created_by VARCHAR(20),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 3. 创建 sso_login_logs 表
CREATE TABLE IF NOT EXISTS sso_login_logs (
    id BIGSERIAL PRIMARY KEY,
    user_id VARCHAR(20),
    identity_id BIGINT,                      -- 与 user_identities.id (BIGSERIAL) 一致
    provider_key VARCHAR(50) NOT NULL,
    provider_user_id VARCHAR(255),
    provider_email VARCHAR(255),
    status VARCHAR(20) NOT NULL,
    error_message TEXT,
    ip_address VARCHAR(45),
    user_agent TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sso_login_logs_user_id ON sso_login_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_sso_login_logs_provider ON sso_login_logs(provider_key);
CREATE INDEX IF NOT EXISTS idx_sso_login_logs_created_at ON sso_login_logs(created_at);

-- 4. 修改 users 表
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_sso_user BOOLEAN DEFAULT FALSE;

-- 5. 添加注释
COMMENT ON TABLE user_identities IS '用户身份关联表，支持多 Provider 绑定';
COMMENT ON TABLE sso_providers IS 'SSO 身份提供商配置表';
COMMENT ON TABLE sso_login_logs IS 'SSO 登录日志，用于审计和问题排查';
```

---

## 十二、总结

本设计方案通过引入 `user_identities` 表和 `sso_providers` 表，实现了：

1. **多 Provider 支持**：用户可以绑定多个 SSO 账号（Google、GitHub、Azure AD 等）
2. **灵活配置**：管理员可以动态添加/配置 Provider，无需修改代码
3. **企业级功能**：支持邮箱域名限制、多租户、自动创建用户等
4. **向后兼容**：保留 users 表的 OAuth 字段，支持平滑迁移
5. **安全性**：State 参数防 CSRF、Token 加密存储、登录日志审计
6. **可扩展性**：Provider 接口抽象，易于添加新的身份提供商

### 与原有文档的关系

- `sso.md`：提供了 Auth0 集成的详细方案，本文档在此基础上扩展为多 Provider 支持
- `sso-muti-provider.md`：提供了多 Provider 扩展性分析，本文档将其具体化为可实施的方案

### 下一步行动

1. 评审本设计方案
2. 创建数据库迁移脚本
3. 按阶段实施开发计划
