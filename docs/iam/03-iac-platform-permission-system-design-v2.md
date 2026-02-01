# IaC Platform 权限系统设计方案（优化版）

> 基于 Terraform Enterprise 三层权限模型，结合第一版方案优化

---

## 📋 变更说明

**相比原方案的主要优化**：
1.  明确权限继承规则：拒绝优先级 > workspace > project > org
2.  补充权限预设（Preset）完整实现
3.  统一数据类型设计，确保兼容性
4.  完善缓存失效策略实现细节
5.  明确临时权限与常规权限的整合逻辑
6.  添加工作空间类型定义
7.  补充批量操作实现细节

---

## 1. 核心权限继承规则（重要）

### 1.1 权限优先级规则

```
优先级从高到低：
1. NONE（显式拒绝）- 最高优先级，任何层级的NONE都会拒绝访问
2. Workspace 级权限 - 最精确的权限
3. Project 级权限 - 中等精确度
4. Organization 级权限 - 最宽泛的权限

规则说明：
- 越精确的作用域，优先级越高
- 但 NONE 权限超越所有层级，具有最高优先级
- 同一层级内，取最高权限等级
```

### 1.2 权限计算算法

```go
// 权限计算伪代码
func CalculateEffectivePermission(grants []PermissionGrant) PermissionLevel {
    // 步骤1: 检查是否存在 NONE（任何层级）
    for _, grant := range grants {
        if grant.Level == NONE && !grant.IsExpired() {
            return NONE  // 立即拒绝
        }
    }
    
    // 步骤2: 按作用域分组
    workspaceGrants := filterByScope(grants, WORKSPACE)
    projectGrants := filterByScope(grants, PROJECT)
    orgGrants := filterByScope(grants, ORGANIZATION)
    
    // 步骤3: 按优先级计算（精确度优先）
    if len(workspaceGrants) > 0 {
        return maxLevel(workspaceGrants)  // Workspace 最优先
    }
    
    if len(projectGrants) > 0 {
        return maxLevel(projectGrants)    // Project 次优先
    }
    
    if len(orgGrants) > 0 {
        return maxLevel(orgGrants)        // Organization 最后
    }
    
    return NONE  // 无任何权限，默认拒绝
}
```

### 1.3 权限继承示例

**场景1：正常继承**
```
Organization: ml_engineers 团队 → ALL_PROJECTS = READ
Project: alice 个人 → PROJECT_WORKSPACES = WRITE
Workspace: 无

结果: alice 对该 workspace 的权限 = WRITE（Project 优先级高于 Organization）
```

**场景2：显式拒绝**
```
Organization: ml_engineers 团队 → ALL_PROJECTS = ADMIN
Project: alice 个人 → PROJECT_WORKSPACES = WRITE
Workspace: alice 个人 → TASK_DATA_ACCESS = NONE

结果: alice 对该 workspace 的权限 = NONE（NONE 最高优先级）
```

**场景3：同层级多个权限**
```
Workspace: 
  - ml_engineers 团队 → TASK_DATA_ACCESS = READ
  - data_team 团队 → TASK_DATA_ACCESS = WRITE
  - alice 同时属于两个团队

结果: alice 的权限 = WRITE（同层级取最高）
```

---

## 2. 数据库设计优化

### 2.1 核心实体表（统一使用 SERIAL）

#### 2.1.1 组织表（organizations）

```sql
CREATE TABLE organizations (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    display_name VARCHAR(200),
    description TEXT,
    is_active BOOLEAN DEFAULT TRUE,
    settings JSONB,
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_organizations_name ON organizations(name);
CREATE INDEX idx_organizations_active ON organizations(is_active);
```

#### 2.1.2 项目表（projects）

```sql
CREATE TABLE projects (
    id SERIAL PRIMARY KEY,
    org_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    display_name VARCHAR(200),
    description TEXT,
    is_default BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    settings JSONB,
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE(org_id, name)
);

CREATE INDEX idx_projects_org ON projects(org_id);
CREATE INDEX idx_projects_org_active ON projects(org_id, is_active);
CREATE INDEX idx_projects_default ON projects(is_default);
```

#### 2.1.3 工作空间表（workspaces）- 兼容现有表

```sql
-- 如果是新建表
CREATE TABLE workspaces (
    id SERIAL PRIMARY KEY,
    name VARCHAR(200) NOT NULL,
    display_name VARCHAR(200),
    workspace_type VARCHAR(50) DEFAULT 'GENERAL',  -- 新增类型字段
    description TEXT,
    config JSONB,
    is_locked BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 如果是扩展现有表（不修改现有字段）
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS workspace_type VARCHAR(50) DEFAULT 'GENERAL';
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS is_locked BOOLEAN DEFAULT FALSE;

-- 工作空间类型定义
COMMENT ON COLUMN workspaces.workspace_type IS 
'工作空间类型: GENERAL(通用), TASK_POOL(任务池), DATASET(数据集), MODULE(模块库), API_SERVICE(API服务), TRAINING(训练环境), TESTING(测试环境)';
```

#### 2.1.4 工作空间-项目关联表

```sql
CREATE TABLE workspace_project_relations (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE(workspace_id)  -- 一个workspace只能属于一个project
);

CREATE INDEX idx_wpr_workspace ON workspace_project_relations(workspace_id);
CREATE INDEX idx_wpr_project ON workspace_project_relations(project_id);
```

### 2.2 权限预设表（新增）

#### 2.2.1 权限预设表（permission_presets）

```sql
CREATE TABLE permission_presets (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL,  -- READ, WRITE, ADMIN
    scope_level VARCHAR(20) NOT NULL,  -- ORGANIZATION, PROJECT, WORKSPACE
    display_name VARCHAR(200),
    description TEXT,
    is_system BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE(name, scope_level)
);

CREATE INDEX idx_presets_scope ON permission_presets(scope_level);
```

#### 2.2.2 权限预设详情表（preset_permissions）

```sql
CREATE TABLE preset_permissions (
    id SERIAL PRIMARY KEY,
    preset_id INTEGER NOT NULL REFERENCES permission_presets(id) ON DELETE CASCADE,
    permission_id INTEGER NOT NULL REFERENCES permission_definitions(id),
    permission_level INTEGER NOT NULL,  -- 0:NONE, 1:READ, 2:WRITE, 3:ADMIN
    
    UNIQUE(preset_id, permission_id)
);

CREATE INDEX idx_preset_perms_preset ON preset_permissions(preset_id);
```

### 2.3 初始化权限预设数据

```sql
-- =============================================
-- 初始化权限预设
-- =============================================

-- 组织级预设
INSERT INTO permission_presets (name, scope_level, display_name, description) VALUES
('READ', 'ORGANIZATION', '组织只读', '查看组织信息和项目列表'),
('WRITE', 'ORGANIZATION', '组织编辑', '管理组织资源（不含用户管理）'),
('ADMIN', 'ORGANIZATION', '组织管理员', '完全控制组织');

-- 项目级预设
INSERT INTO permission_presets (name, scope_level, display_name, description) VALUES
('READ', 'PROJECT', '项目只读', '查看项目信息和工作空间'),
('WRITE', 'PROJECT', '项目编辑', '管理项目工作空间'),
('ADMIN', 'PROJECT', '项目管理员', '完全控制项目');

-- 工作空间级预设
INSERT INTO permission_presets (name, scope_level, display_name, description) VALUES
('READ', 'WORKSPACE', '工作空间只读', '查看数据和配置'),
('WRITE', 'WORKSPACE', '工作空间编辑', '读写数据和执行操作'),
('ADMIN', 'WORKSPACE', '工作空间管理员', '完全控制工作空间');

-- =============================================
-- 配置预设包含的权限
-- =============================================

-- 组织级 READ 预设
INSERT INTO preset_permissions (preset_id, permission_id, permission_level)
SELECT p.id, pd.id, 1  -- READ level
FROM permission_presets p
CROSS JOIN permission_definitions pd
WHERE p.name = 'READ' AND p.scope_level = 'ORGANIZATION'
  AND pd.scope_level = 'ORGANIZATION'
  AND pd.name = 'all_projects';

-- 组织级 WRITE 预设
INSERT INTO preset_permissions (preset_id, permission_id, permission_level)
SELECT p.id, pd.id, 2  -- WRITE level
FROM permission_presets p
CROSS JOIN permission_definitions pd
WHERE p.name = 'WRITE' AND p.scope_level = 'ORGANIZATION'
  AND pd.scope_level = 'ORGANIZATION'
  AND pd.name IN ('all_projects', 'organization_settings');

-- 组织级 ADMIN 预设
INSERT INTO preset_permissions (preset_id, permission_id, permission_level)
SELECT p.id, pd.id, 3  -- ADMIN level
FROM permission_presets p
CROSS JOIN permission_definitions pd
WHERE p.name = 'ADMIN' AND p.scope_level = 'ORGANIZATION'
  AND pd.scope_level = 'ORGANIZATION';

-- 项目级预设（类似配置）
-- ... 省略，按相同模式配置

-- 工作空间级预设（类似配置）
-- ... 省略，按相同模式配置
```

---

## 3. 权限检查器优化实现

### 3.1 权限检查器核心逻辑

```go
package service

import (
    "context"
    "time"
)

// PermissionCheckerImpl 权限检查器实现（优化版）
type PermissionCheckerImpl struct {
    permissionRepo repository.PermissionRepository
    teamRepo       repository.TeamRepository
    workspaceRepo  repository.WorkspaceRepository
    projectRepo    repository.ProjectRepository
    cache          cache.PermissionCache
    auditRepo      repository.AuditRepository
    logger         logger.Logger
}

// CheckPermission 检查权限（优化版）
func (c *PermissionCheckerImpl) CheckPermission(
    ctx context.Context,
    req *CheckPermissionRequest,
) (*CheckPermissionResult, error) {
    startTime := time.Now()
    
    // 1. 检查缓存
    cacheKey := c.getCacheKey(req)
    if cached, err := c.cache.GetPermissionResult(ctx, cacheKey); err == nil {
        cached.CacheHit = true
        return cached, nil
    }
    
    // 2. 检查系统管理员
    if c.isSystemAdmin(ctx, req.UserID) {
        result := &CheckPermissionResult{
            IsAllowed:      true,
            EffectiveLevel: PermissionLevelAdmin,
            CacheHit:       false,
        }
        c.cache.SetPermissionResult(ctx, cacheKey, result, 5*time.Minute)
        return result, nil
    }
    
    // 3. 获取用户所属团队
    userTeams, err := c.getUserTeams(ctx, req.UserID)
    if err != nil {
        return nil, err
    }
    
    // 4. 收集所有权限授予（按层级）
    grants, err := c.collectAllGrants(ctx, req, userTeams)
    if err != nil {
        return nil, err
    }
    
    // 5. 计算有效权限（使用优化的算法）
    effectiveLevel := c.calculateEffectiveLevelOptimized(grants)
    
    // 6. 判定结果
    isAllowed := effectiveLevel >= req.RequiredLevel && effectiveLevel != PermissionLevelNone
    
    result := &CheckPermissionResult{
        IsAllowed:      isAllowed,
        EffectiveLevel: effectiveLevel,
        Grants:         grants,
        DenyReason:     c.getDenyReason(effectiveLevel, req.RequiredLevel),
        CacheHit:       false,
    }
    
    // 7. 记录访问日志（异步）
    go c.logAccess(ctx, req, result, time.Since(startTime))
    
    // 8. 缓存结果
    c.cache.SetPermissionResult(ctx, cacheKey, result, 5*time.Minute)
    
    return result, nil
}

// calculateEffectiveLevelOptimized 计算有效权限（优化版）
// 规则：拒绝优先级 > workspace > project > org
func (c *PermissionCheckerImpl) calculateEffectiveLevelOptimized(
    grants []*entity.PermissionGrant,
) PermissionLevel {
    // 步骤1: 过滤过期权限
    validGrants := c.filterExpiredGrants(grants)
    
    // 步骤2: 检查 NONE（最高优先级）
    for _, grant := range validGrants {
        if grant.PermissionLevel == PermissionLevelNone {
            return PermissionLevelNone  // 立即拒绝
        }
    }
    
    // 步骤3: 按作用域分组
    workspaceGrants := c.filterByScope(validGrants, ScopeTypeWorkspace)
    projectGrants := c.filterByScope(validGrants, ScopeTypeProject)
    orgGrants := c.filterByScope(validGrants, ScopeTypeOrganization)
    
    // 步骤4: 按精确度优先级计算
    // Workspace 最精确，优先级最高
    if len(workspaceGrants) > 0 {
        return c.maxLevel(workspaceGrants)
    }
    
    // Project 次精确
    if len(projectGrants) > 0 {
        return c.maxLevel(projectGrants)
    }
    
    // Organization 最宽泛
    if len(orgGrants) > 0 {
        return c.maxLevel(orgGrants)
    }
    
    // 无任何权限
    return PermissionLevelNone
}

// collectAllGrants 收集所有权限授予（优化版）
func (c *PermissionCheckerImpl) collectAllGrants(
    ctx context.Context,
    req *CheckPermissionRequest,
    userTeams []string,
) ([]*entity.PermissionGrant, error) {
    var allGrants []*entity.PermissionGrant
    
    // 获取资源的层级信息
    scopeInfo, err := c.getScopeInfo(ctx, req.ScopeType, req.ScopeID)
    if err != nil {
        return nil, err
    }
    
    // 1. 收集 Organization 级权限
    if scopeInfo.OrgID > 0 {
        orgGrants, err := c.collectOrgLevelGrants(
            ctx, req.UserID, userTeams, req.ResourceType, scopeInfo.OrgID,
        )
        if err != nil {
            return nil, err
        }
        allGrants = append(allGrants, orgGrants...)
    }
    
    // 2. 收集 Project 级权限
    if scopeInfo.ProjectID > 0 {
        projGrants, err := c.collectProjectLevelGrants(
            ctx, req.UserID, userTeams, req.ResourceType, scopeInfo.ProjectID,
        )
        if err != nil {
            return nil, err
        }
        allGrants = append(allGrants, projGrants...)
    }
    
    // 3. 收集 Workspace 级权限
    if req.ScopeType == ScopeTypeWorkspace {
        wsGrants, err := c.collectWorkspaceLevelGrants(
            ctx, req.UserID, userTeams, req.ResourceType, req.ScopeID,
        )
        if err != nil {
            return nil, err
        }
        allGrants = append(allGrants, wsGrants...)
    }
    
    return allGrants, nil
}

// getScopeInfo 获取作用域层级信息
func (c *PermissionCheckerImpl) getScopeInfo(
    ctx context.Context,
    scopeType ScopeType,
    scopeID int,
) (*ScopeInfo, error) {
    info := &ScopeInfo{}
    
    switch scopeType {
    case ScopeTypeOrganization:
        info.OrgID = scopeID
        
    case ScopeTypeProject:
        project, err := c.projectRepo.GetByID(ctx, scopeID)
        if err != nil {
            return nil, err
        }
        info.OrgID = project.OrgID
        info.ProjectID = scopeID
        
    case ScopeTypeWorkspace:
        // 通过关联表获取项目ID
        projectID, err := c.workspaceRepo.GetProjectID(ctx, scopeID)
        if err != nil {
            return nil, err
        }
        info.ProjectID = projectID
        
        // 获取组织ID
        project, err := c.projectRepo.GetByID(ctx, projectID)
        if err != nil {
            return nil, err
        }
        info.OrgID = project.OrgID
        info.WorkspaceID = scopeID
    }
    
    return info, nil
}

// filterByScope 按作用域过滤权限
func (c *PermissionCheckerImpl) filterByScope(
    grants []*entity.PermissionGrant,
    scopeType ScopeType,
) []*entity.PermissionGrant {
    var filtered []*entity.PermissionGrant
    for _, grant := range grants {
        if grant.ScopeType == scopeType {
            filtered = append(filtered, grant)
        }
    }
    return filtered
}

// maxLevel 获取权限列表中的最高等级
func (c *PermissionCheckerImpl) maxLevel(
    grants []*entity.PermissionGrant,
) PermissionLevel {
    maxLevel := PermissionLevelNone
    for _, grant := range grants {
        if grant.PermissionLevel > maxLevel {
            maxLevel = grant.PermissionLevel
        }
    }
    return maxLevel
}

// filterExpiredGrants 过滤过期权限
func (c *PermissionCheckerImpl) filterExpiredGrants(
    grants []*entity.PermissionGrant,
) []*entity.PermissionGrant {
    now := time.Now()
    var valid []*entity.PermissionGrant
    
    for _, grant := range grants {
        if grant.ExpiresAt == nil || grant.ExpiresAt.After(now) {
            valid = append(valid, grant)
        }
    }
    
    return valid
}

type ScopeInfo struct {
    OrgID       int
    ProjectID   int
    WorkspaceID int
}
```

---

## 4. 缓存失效策略优化

### 4.1 缓存失效实现

```go
package cache

// InvalidateUser 使用户缓存失效
func (c *RedisCacheImpl) InvalidateUser(
    ctx context.Context,
    userID string,
) error {
    // 1. 删除用户的所有权限检查结果
    pattern := fmt.Sprintf("%sperm:result:%s:*", c.keyPrefix, userID)
    if err := c.deleteByPattern(ctx, pattern); err != nil {
        return err
    }
    
    // 2. 删除用户团队缓存
    teamKey := c.generateUserTeamsKey(userID)
    if err := c.client.Del(ctx, teamKey).Err(); err != nil {
        return err
    }
    
    c.logger.Info("invalidated user cache", "user_id", userID)
    return nil
}

// InvalidateTeam 使团队缓存失效
func (c *RedisCacheImpl) InvalidateTeam(
    ctx context.Context,
    teamID string,
) error {
    // 1. 查询团队所有成员
    members, err := c.teamRepo.ListMembers(ctx, teamID)
    if err != nil {
        return err
    }
    
    // 2. 使每个成员的缓存失效
    for _, member := range members {
        if err := c.InvalidateUser(ctx, member.UserID); err != nil {
            c.logger.Error("failed to invalidate user cache", 
                "user_id", member.UserID, "error", err)
        }
    }
    
    c.logger.Info("invalidated team cache", "team_id", teamID, 
        "members_count", len(members))
    return nil
}

// InvalidateScope 使作用域缓存失效
func (c *RedisCacheImpl) InvalidateScope(
    ctx context.Context,
    scopeType valueobject.ScopeType,
    scopeID string,
) error {
    // 删除该作用域相关的所有权限检查结果
    pattern := fmt.Sprintf("%sperm:result:*:%s:%s:*", 
        c.keyPrefix, scopeType, scopeID)
    
    if err := c.deleteByPattern(ctx, pattern); err != nil {
        return err
    }
    
    c.logger.Info("invalidated scope cache", 
        "scope_type", scopeType, "scope_id", scopeID)
    return nil
}

// InvalidatePermissionChange 权限变更时的缓存失效
func (c *RedisCacheImpl) InvalidatePermissionChange(
    ctx context.Context,
    scopeType valueobject.ScopeType,
    scopeID string,
    principalType entity.PrincipalType,
    principalID string,
) error {
    // 1. 如果是用户权限变更
    if principalType == entity.PrincipalTypeUser {
        return c.InvalidateUser(ctx, principalID)
    }
    
    // 2. 如果是团队权限变更
    if principalType == entity.PrincipalTypeTeam {
        return c.InvalidateTeam(ctx, principalID)
    }
    
    // 3. 如果是应用权限变更
    if principalType == entity.PrincipalTypeApplication {
        // 应用权限变更，失效该应用的缓存
        pattern := fmt.Sprintf("%sperm:app:%s:*", c.keyPrefix, principalID)
        return c.deleteByPattern(ctx, pattern)
    }
    
    return nil
}

// deleteByPattern 按模式删除缓存
func (c *RedisCacheImpl) deleteByPattern(
    ctx context.Context,
    pattern string,
) error {
    iter := c.client.Scan(ctx, 0, pattern, 100).Iterator()
    
    var keys []string
    for iter.Next(ctx) {
        keys = append(keys, iter.Val())
    }
    
    if err := iter.Err(); err != nil {
        return err
    }
    
    if len(keys) > 0 {
        if err := c.client.Del(ctx, keys...).Err(); err != nil {
            return err
        }
        c.logger.Debug("deleted cache keys", "pattern", pattern, "count", len(keys))
    }
    
    return nil
}
```

### 4.2 权限变更时自动失效缓存

```go
// GrantPermission 授予权限（带缓存失效）
func (s *PermissionServiceImpl) GrantPermission(
    ctx context.Context,
    req *GrantPermissionRequest,
) error {
    // 1. 验证授权人权限
    // ... 省略
    
    // 2. 插入权限记录
    grant := &entity.PermissionGrant{
        // ... 填充字段
    }
    
    if err := s.permissionRepo.GrantPermission(ctx, grant); err != nil {
        return err
    }
    
    // 3. 记录审计日志
    // ... 省略
    
    // 4. 使相关缓存失效（关键步骤）
    if err := s.cache.InvalidatePermissionChange(
        ctx,
        req.ScopeType,
        req.ScopeID,
        req.PrincipalType,
        req.PrincipalID,
    ); err != nil {
        s.logger.Error("failed to invalidate cache", "error", err)
        // 不返回错误，避免影响主流程
    }
    
    return nil
}
```

---

## 5. 临时权限与常规权限整合

### 5.1 整合逻辑

```go
// CheckPermissionWithTemporary 检查权限（包含临时权限）
func (c *PermissionCheckerImpl) CheckPermissionWithTemporary(
    ctx context.Context,
    req *CheckPermissionRequest,
    taskID *int,  // 如果是任务相关操作，传入任务ID
) (*CheckPermissionResult, error) {
    // 1. 检查常规权限
    regularResult, err := c.CheckPermission(ctx, req)
    if err != nil {
        return nil, err
    }
    
    // 2. 如果常规权限已允许，直接返回
    if regularResult.IsAllowed {
        return regularResult, nil
    }
    
    // 3. 如果常规权限拒绝，检查是否有临时权限
    if taskID != nil {
        hasTemp, err := c.checkTemporaryPermission(ctx, req.UserID, *taskID, req.ResourceType)
        if err != nil {
            return nil, err
        }
        
        if hasTemp {
            // 临时权限允许
            return &CheckPermissionResult{
                IsAllowed:      true,
                EffectiveLevel: req.RequiredLevel,  // 临时权限满足需求即可
                Source:         "temporary",
                CacheHit:       false,
            }, nil
        }
    }
    
    // 4. 两种权限都不满足，返回拒绝
    return regularResult, nil
}

// checkTemporaryPermission 检查临时权限
func (c *PermissionCheckerImpl) checkTemporaryPermission(
    ctx context.Context,
    userID string,
    taskID int,
    resourceType valueobject.ResourceType,
) (bool, error) {
    // 查询临时权限
    var tempPerm TaskTemporaryPermission
    
    err := c.db.Where(
        "task_id = ? AND user_email = ? AND permission_type = ? AND expires_at > ? AND is_used = ?",
        taskID,
        c.getUserEmail(ctx, userID),
        c.mapResourceToPermType(resourceType),
        time.Now(),
        false,
    ).First(&tempPerm).Error
    
    if err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return false, nil  // 无临时权限
        }
        return false, err
    }
    
    // 标记已使用
    tempPerm.IsUsed = true
    now := time.Now()
    tempPerm.UsedAt = &now
    c.db.Save(&tempPerm)
    
    return true, nil
}
```

### 5.2 临时权限特点

```yaml
临时权限设计原则:
  1. 独立判断: 不受 NONE 权限影响
  2. 或关系: 常规权限 OR 临时权限，任一满足即可
  3. 一次性: 使用后标记 is_used = true
  4. 自动过期: 基于 expires_at 字段
  5. 任务绑定: 只对特定任务有效
  6. 不缓存: 临时权限实时查询，不缓存

使用场景:
  - Apply 操作需要审批
  - Cancel 操作需要授权
  - 临时授予特定任务的访问权限
```

---

## 6. 批量操作优化

### 6.1 批量权限检查

```go
// CheckBatchPermissions 批量检查权限（优化版）
func (c *PermissionCheckerImpl) CheckBatchPermissions(
    ctx context.Context,
    reqs []*CheckPermissionRequest,
) ([]*CheckPermissionResult, error) {
    results := make([]*CheckPermissionResult, len(reqs))
    
    // 1. 批量检查缓存
    cacheKeys := make([]string, len(reqs))
    cachedResults := make(map[int]*CheckPermissionResult)
    
    for i, req := range reqs {
        cacheKeys[i] = c.getCacheKey(req)
        if cached, err := c.cache.GetPermissionResult(ctx, cacheKeys[i]); err == nil {
            cached.CacheHit = true
            cachedResults[i] = cached
        }
    }
    
    // 2. 收集需要查询的请求
    var needCheckReqs []*CheckPermissionRequest
    var needCheckIndexes []int
    
    for i, req := range reqs {
        if _, cached := cachedResults[i]; !cached {
            needCheckReqs = append(needCheckReqs, req)
            needCheckIndexes = append(needCheckIndexes, i)
        }
    }
    
    // 3. 批量查询数据库（优化：一次查询获取所有用户的团队和权限）
    if len(needCheckReqs) > 0 {
        // 收集所有用户ID
        userIDs := make(map[int]bool)
        for _, req := range needCheckReqs {
            userIDs[req.UserID] = true
        }
        
        // 批量获取用户团队
        userTeamsMap, err := c.batchGetUserTeams(ctx, userIDs)
        if err != nil {
            return nil, err
        }
        
        // 批量检查权限
        for idx, i := range needCheckIndexes {
            req := needCheckReqs[idx]
            userTeams := userTeamsMap[req.UserID]
            
            // 收集权限
            grants, err := c.collectAllGrants(ctx, req, userTeams)
            if err != nil {
                return nil, err
            }
            
            // 计算有效权限
            effectiveLevel := c.calculateEffectiveLevelOptimized(grants)
            isAllowed := effectiveLevel >= req.RequiredLevel && effectiveLevel != 0
            
            result := &CheckPermissionResult{
                IsAllowed:      isAllowed,
                EffectiveLevel: effectiveLevel,
                DenyReason:     c.getDenyReason(effectiveLevel, req.RequiredLevel),
                Source:         "regular",
                CacheHit:       false,
            }
            
            results[i] = result
            
            // 缓存结果
            c.cache.SetPermissionResult(ctx, cacheKeys[i], result, 5*time.Minute)
        }
    }
    
    // 4. 填充缓存结果
    for i, cached := range cachedResults {
        results[i] = cached
    }
    
    return results, nil
}

// batchGetUserTeams 批量获取用户团队
func (c *PermissionCheckerImpl) batchGetUserTeams(
    ctx context.Context,
    userIDs map[int]bool,
) (map[int][]string, error) {
    userTeamsMap := make(map[int][]string)
    
    // 批量查询
    var userIDList []int
    for userID := range userIDs {
        userIDList = append(userIDList, userID)
    }
    
    teams, err := c.teamRepo.BatchGetUserTeams(ctx, userIDList)
    if err != nil {
        return nil, err
    }
    
    for userID, teamList := range teams {
        userTeamsMap[userID] = teamList
    }
    
    return userTeamsMap, nil
}
```

---

## 7. API 接口设计

### 7.1 权限检查接口

```go
// POST /api/v1/permissions/check
// 检查单个权限
type CheckPermissionRequest struct {
    UserID        int    `json:"user_id" binding:"required"`
    ResourceType  string `json:"resource_type" binding:"required"`
    ScopeType     string `json:"scope_type" binding:"required"`
    ScopeID       int    `json:"scope_id" binding:"required"`
    RequiredLevel int    `json:"required_level" binding:"required"`
}

type CheckPermissionResponse struct {
    IsAllowed      bool   `json:"is_allowed"`
    EffectiveLevel int    `json:"effective_level"`
    DenyReason     string `json:"deny_reason,omitempty"`
    CacheHit       bool   `json:"cache_hit"`
}

// POST /api/v1/permissions/check-batch
// 批量检查权限
type BatchCheckRequest struct {
    Checks []CheckPermissionRequest `json:"checks" binding:"required"`
}

type BatchCheckResponse struct {
    Results []CheckPermissionResponse `json:"results"`
}
```

### 7.2 权限管理接口

```go
// POST /api/v1/permissions/grant
// 授予权限
type GrantPermissionRequest struct {
    ScopeType       string `json:"scope_type" binding:"required"`
    ScopeID         int    `json:"scope_id" binding:"required"`
    PrincipalType   string `json:"principal_type" binding:"required"`
    PrincipalID     int    `json:"principal_id" binding:"required"`
    PermissionID    int    `json:"permission_id" binding:"required"`
    PermissionLevel int    `json:"permission_level" binding:"required"`
    ExpiresAt       *string `json:"expires_at,omitempty"`
    Reason          string  `json:"reason,omitempty"`
}

// POST /api/v1/permissions/grant-preset
// 授予预设权限集（READ/WRITE/ADMIN）
type GrantPresetRequest struct {
    ScopeType     string `json:"scope_type" binding:"required"`
    ScopeID       int    `json:"scope_id" binding:"required"`
    PrincipalType string `json:"principal_type" binding:"required"`
    PrincipalID   int    `json:"principal_id" binding:"required"`
    PresetName    string `json:"preset_name" binding:"required"` // READ/WRITE/ADMIN
    Reason        string `json:"reason,omitempty"`
}

// DELETE /api/v1/permissions/{id}
// 撤销权限

// GET /api/v1/permissions/{scope_type}/{scope_id}
// 列出指定作用域的所有权限分配
```

### 7.3 团队管理接口

```go
// POST /api/v1/teams
// 创建团队
type CreateTeamRequest struct {
    OrgID       int    `json:"org_id" binding:"required"`
    Name        string `json:"name" binding:"required"`
    DisplayName string `json:"display_name"`
    Description string `json:"description"`
}

// POST /api/v1/teams/{id}/members
// 添加团队成员
type AddTeamMemberRequest struct {
    UserID int    `json:"user_id" binding:"required"`
    Role   string `json:"role" binding:"required"` // MEMBER, MAINTAINER
}

// GET /api/v1/teams/{id}/members
// 列出团队成员

// DELETE /api/v1/teams/{id}/members/{user_id}
// 移除团队成员
```

---

## 8. 实施路线图

### 8.1 第一阶段：基础架构（2周）

**目标：** 建立三层结构 + 基础权限

**任务清单：**
- [ ] 创建 Organization、Project 表和关联关系
- [ ] 创建 workspace_project_relations 关联表
- [ ] 迁移现有 Workspace 到 default Project
- [ ] 创建 Teams 和团队成员表
- [ ] 创建权限定义和权限分配表
- [ ] 创建权限预设表（Preset）
- [ ] 实现基础权限检查服务
- [ ] 添加权限检查中间件
- [ ] 创建审计日志表

**交付物：**
- 数据库迁移脚本
- 权限检查服务（PermissionChecker）
- 基础 API 接口

### 8.2 第二阶段：团队管理（1周）

**目标：** 实现团队管理功能

**任务清单：**
- [ ] 团队 CRUD API
- [ ] 团队成员管理 API
- [ ] 团队权限继承逻辑
- [ ] 用户-组织关系管理
- [ ] 团队权限管理界面（前端）

**交付物：**
- 团队管理完整功能
- 前端团队管理页面

### 8.3 第三阶段：应用授权（1周）

**目标：** 实现 Application/Agent 授权

**任务清单：**
- [ ] 创建 Applications 表
- [ ] 迁移现有 Agents 数据
- [ ] 实现 API Key 认证
- [ ] Application 权限管理
- [ ] Application 管理界面

**交付物：**
- Application 管理系统
- API Key 认证机制

### 8.4 第四阶段：临时权限（2周）

**目标：** 实现基于 Webhook 的临时权限

**任务清单：**
- [ ] 创建临时权限相关表
- [ ] 实现 Webhook 配置管理
- [ ] 实现 Webhook 回调处理
- [ ] 临时权限检查逻辑
- [ ] 权限过期清理任务
- [ ] Webhook 管理界面

**交付物：**
- 完整的临时权限系统
- Webhook 集成功能

### 8.5 第五阶段：优化和完善（1周）

**目标：** 性能优化和功能完善

**任务清单：**
- [ ] Redis 缓存优化
- [ ] 批量权限检查接口
- [ ] 权限报表和分析
- [ ] OAuth 认证预留接口
- [ ] 性能测试和优化
- [ ] 完整的权限管理界面

**交付物：**
- 优化后的权限系统
- 完整的管理界面
- 性能测试报告

---

## 9. 关键实施要点

### 9.1 数据迁移策略

1. **保持向后兼容**
   - 不修改现有表结构
   - 通过关联表扩展功能
   - API 保持兼容性

2. **默认数据初始化**
   ```sql
   -- 1. 创建默认组织
   INSERT INTO organizations (name, display_name) 
   VALUES ('default', 'Default Organization');
   
   -- 2. 创建默认项目
   INSERT INTO projects (org_id, name, is_default) 
   VALUES (1, 'default', TRUE);
   
   -- 3. 关联现有工作空间
   INSERT INTO workspace_project_relations (workspace_id, project_id)
   SELECT id, 1 FROM workspaces;
   
   -- 4. 创建系统团队
   INSERT INTO teams (org_id, name, is_system) VALUES
   (1, 'owners', TRUE),
   (1, 'admins', TRUE);
   ```

### 9.2 性能优化建议

1. **缓存策略**
   - 权限检查结果缓存 5 分钟
   - 用户团队关系缓存 10 分钟
   - 权限变更时精准失效

2. **数据库优化**
   - 合理使用复合索引
   - 访问日志按月分区
   - 定期清理过期权限

3. **查询优化**
   - 批量权限检查
   - 预加载用户团队
   - 使用数据库连接池

### 9.3 安全考虑

1. **认证安全**
   - API Key 加密存储
   - Webhook 签名验证
   - OAuth 支持（后期）

2. **权限安全**
   - 最小权限原则
   - 显式拒绝优先
   - 完整审计日志

3. **数据安全**
   - 租户数据隔离
   - 敏感信息加密
   - 定期安全审计

---

## 10. 总结

本权限系统设计方案整合了初版和v1版本的优点：

### 核心优势

 **明确的权限继承规则**：拒绝优先级 > workspace > project > org  
 **完整的三层模型**：Organization → Project → Workspace  
 **权限预设功能**：快速授予 READ/WRITE/ADMIN 权限集  
 **灵活的授权方式**：支持 User、Team、Application 多种主体  
 **创新的临时权限**：基于 Webhook 的任务级临时授权  
 **不破坏现有系统**：通过关联表扩展，保持向后兼容  
 **高性能设计**：多级缓存、批量操作、分区表  
 **完整的审计**：所有操作可追溯

### 实施建议

1. **分阶段实施**：先实现基础权限，再逐步完善
2. **保持兼容性**：确保现有功能不受影响
3. **重视测试**：每个阶段都要充分测试
4. **文档先行**：API 文档和使用指南要及时更新
5. **监控先行**：从一开始就建立监控体系

### 后续扩展

- **OAuth 集成**：支持企业 SSO
- **细粒度权限**：资源级别的权限控制
- **权限委托**：临时委托权限给其他用户
- **AI 辅助**：智能权限推荐和异常检测

通过这个权限系统，IaC Platform 将具备企业级的权限管理能力，为多租户、多团队协作提供坚实的基础。
