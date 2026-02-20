# User和Team ID迁移状态报告

## 当前状态: 数据库迁移完成,代码适配进行中

###  已完成工作 (80%)

#### 1. 数据库迁移 (100%完成)
-  创建新表users/teams,使用VARCHAR(20)类型的user_id/team_id
-  迁移所有数据,生成新ID格式
-  更新关联表: team_members, user_organizations, team_tokens
-  切换表名,删除旧表
-  重建所有外键约束
-  清理临时映射表和函数

**新ID格式示例**:
```
Users:
- user-n8tzt0ldde (admin)
- user-08i8pobce0 (ken)

Teams:
- team-yu9ipso75b (owners)
- team-evwitr96eg (admins)  
- team-tsohd0pkw8 (ops)
```

#### 2. Model层代码 (100%完成)
-  `backend/internal/models/user.go`
-  `backend/internal/domain/entity/team.go`
-  `backend/internal/domain/entity/team.go` (TeamMember)
-  `backend/internal/models/team_token.go`

#### 3. Repository层代码 (100%完成)
-  `backend/internal/domain/repository/team_repository.go` (接口)
-  `backend/internal/infrastructure/persistence/team_repository_impl.go` (实现)

#### 4. ID生成器 (100%完成)
-  `backend/internal/infrastructure/id_generator.go`
  - GenerateUserID()
  - GenerateTeamID()
  - ValidateUserID()
  - ValidateTeamID()

###  进行中的工作 (20%)

#### Service层代码适配 (需要继续)

根据编译错误,以下文件需要修复:

1. **backend/internal/application/service/team_service.go**  (已部分修复)
   - CreateTeamRequest.CreatedBy: uint → string 
   - AddTeamMemberRequest: TeamID/UserID/AddedBy: uint → string 
   - 接口方法签名已更新 

2. **backend/internal/application/service/organization_service.go** ❌
   - Line 212: createdBy类型不匹配
   - Line 224: createdBy类型不匹配
   - 需要将创建团队时的createdBy从uint改为string

3. **backend/internal/application/service/permission_checker.go** ❌
   - Line 196: GetUserTeams返回类型不匹配
   - 需要将[]uint改为[]string
   - userID参数从uint改为string

4. **其他Service文件** (可能需要修复)
   - team_token_service.go
   - user_service.go
   - 等等

#### Handler层代码适配 (未开始)

需要修复的文件:
- backend/internal/handlers/team_handler.go
- backend/internal/handlers/user_handler.go
- backend/internal/handlers/organization_handler.go
- 等等

主要修改:
- API参数解析: `strconv.ParseUint()` → 直接使用string
- 路径参数类型: uint → string
- 请求/响应结构体中的ID字段

## 剩余工作清单

### 高优先级 (必须完成才能运行)

- [ ] 修复organization_service.go中的类型错误
- [ ] 修复permission_checker.go中的类型错误
- [ ] 修复team_token_service.go中的类型错误
- [ ] 修复所有Handler层的类型错误
- [ ] 修复中间件中的user_id类型
- [ ] 成功编译并运行

### 中优先级 (功能完整性)

- [ ] 更新所有涉及user_id/team_id的Service方法
- [ ] 更新JWT token中的user_id字段
- [ ] 更新session中的user_id字段
- [ ] 测试所有IAM相关功能

### 低优先级 (优化和清理)

- [ ] 前端TypeScript类型适配
- [ ] API文档更新
- [ ] 性能测试和优化
- [ ] 删除旧的迁移脚本

## 编译错误汇总

当前编译错误(来自go run main.go):

```
internal/application/service/organization_service.go:212:16: 
  cannot use &createdBy (value of type *uint) as *string value in struct literal

internal/application/service/organization_service.go:224:16: 
  cannot use &createdBy (value of type *uint) as *string value in struct literal

internal/application/service/permission_checker.go:196:9: 
  cannot use c.teamRepo.GetUserTeams(ctx, userID) (value of type []string) as []uint value in return statement

internal/application/service/permission_checker.go:196:38: 
  cannot use userID (variable of type uint) as string value in argument to c.teamRepo.GetUserTeams

internal/application/service/team_service.go:112:16: 
  cannot use &req.CreatedBy (value of type *uint) as *string value in struct literal
  (已修复)

internal/application/service/team_service.go:126:37: 
  cannot use teamID (variable of type uint) as string value in argument
  (已修复)
```

## 修复策略

### 方案1: 逐个文件修复 (推荐)
按照编译错误提示,逐个修复Service文件:
1. organization_service.go
2. permission_checker.go
3. team_token_service.go
4. 其他Service文件
5. Handler文件

### 方案2: 全局搜索替换 (风险较高)
使用正则表达式批量替换:
- `userID uint` → `userID string`
- `teamID uint` → `teamID string`
- `UserID uint` → `UserID string`
- `TeamID uint` → `TeamID string`

但需要注意:
- 不是所有uint都需要改(如orgID仍是uint)
- 需要人工review每个修改

## 下一步行动

1. **立即修复**: organization_service.go和permission_checker.go
2. **编译测试**: 修复后尝试编译
3. **继续修复**: 根据新的编译错误继续修复
4. **功能测试**: 编译成功后测试核心功能
5. **文档更新**: 更新API文档和开发指南

## 回滚方案

如果遇到无法解决的问题,可以回滚:

```sql
-- 注意: 旧表已被删除,需要从备份恢复
-- 如果有数据库备份,可以恢复到迁移前的状态
```

**建议**: 在生产环境实施前,务必:
1. 完整备份数据库
2. 在测试环境完整验证
3. 准备详细的回滚计划

---

**更新时间**: 2025-10-25 17:34
**完成度**: 80%
**预计剩余时间**: 2-3小时(修复所有编译错误)
