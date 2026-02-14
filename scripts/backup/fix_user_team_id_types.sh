#!/bin/bash

# =============================================
# User和Team ID类型批量修复脚本
# 将所有Service和Handler层的uint类型改为string
# =============================================

set -e

echo "开始批量修复User和Team ID类型..."

# 进入backend目录
cd "$(dirname "$0")/../backend"

# =============================================
# 1. 修复permission_checker.go
# =============================================

echo "修复 permission_checker.go..."

# 修复GetUserTeams接口定义
sed -i '' 's/GetUserTeams(ctx context.Context, userID uint) (\[\]uint, error)/GetUserTeams(ctx context.Context, userID string) ([]string, error)/g' internal/application/service/permission_checker.go

# 修复GetUserTeams实现
sed -i '' 's/func (c \*PermissionCheckerImpl) GetUserTeams(ctx context.Context, userID uint) (\[\]uint, error)/func (c *PermissionCheckerImpl) GetUserTeams(ctx context.Context, userID string) ([]string, error)/g' internal/application/service/permission_checker.go

# 修复CheckPermissionRequest中的UserID
sed -i '' 's/UserID        uint/UserID        string/g' internal/application/service/permission_checker.go

# 修复collectOrgLevelGrants等方法的userID参数
sed -i '' 's/userID uint,$/userID string,/g' internal/application/service/permission_checker.go
sed -i '' 's/userTeams \[\]uint,$/userTeams []string,/g' internal/application/service/permission_checker.go

# 修复validateRequest中的UserID检查
sed -i '' 's/if req.UserID == 0 {/if req.UserID == "" {/g' internal/application/service/permission_checker.go

# 修复logAccess中的UserID
sed -i '' 's/UserID:         req.UserID,/UserID:         req.UserID,/g' internal/application/service/permission_checker.go

echo "✓ permission_checker.go 修复完成"

# =============================================
# 2. 修复team_token_service.go
# =============================================

echo "修复 team_token_service.go..."

# 修复CreateTeamTokenRequest
sed -i '' 's/TeamID     uint/TeamID     string/g' internal/application/service/team_token_service.go
sed -i '' 's/CreatedBy  uint/CreatedBy  string/g' internal/application/service/team_token_service.go

# 修复RevokeTeamTokenRequest
sed -i '' 's/RevokedBy  uint/RevokedBy  string/g' internal/application/service/team_token_service.go

# 修复TeamTokenResponse
sed -i '' 's/TeamID     uint/TeamID     string/g' internal/application/service/team_token_service.go
sed -i '' 's/CreatedBy  uint/CreatedBy  string/g' internal/application/service/team_token_service.go
sed -i '' 's/RevokedBy  \*uint/RevokedBy  *string/g' internal/application/service/team_token_service.go

# 修复TeamTokenClaims
sed -i '' 's/TeamID    uint/TeamID    string/g' internal/application/service/team_token_service.go

# 修复接口方法签名
sed -i '' 's/CreateTeamToken(ctx context.Context, req \*CreateTeamTokenRequest) (\*models.TeamToken, string, error)/CreateTeamToken(ctx context.Context, req *CreateTeamTokenRequest) (*models.TeamToken, string, error)/g' internal/application/service/team_token_service.go
sed -i '' 's/ListTeamTokens(ctx context.Context, teamID uint) (\[\]\*models.TeamToken, error)/ListTeamTokens(ctx context.Context, teamID string) ([]*models.TeamToken, error)/g' internal/application/service/team_token_service.go
sed -i '' 's/RevokeTeamToken(ctx context.Context, tokenID uint, req \*RevokeTeamTokenRequest) error/RevokeTeamToken(ctx context.Context, tokenID uint, req *RevokeTeamTokenRequest) error/g' internal/application/service/team_token_service.go

# 修复实现方法
sed -i '' 's/func (s \*TeamTokenServiceImpl) ListTeamTokens(ctx context.Context, teamID uint)/func (s *TeamTokenServiceImpl) ListTeamTokens(ctx context.Context, teamID string)/g' internal/application/service/team_token_service.go

# 修复验证方法
sed -i '' 's/if req.TeamID == 0 {/if req.TeamID == "" {/g' internal/application/service/team_token_service.go
sed -i '' 's/if req.CreatedBy == 0 {/if req.CreatedBy == "" {/g' internal/application/service/team_token_service.go
sed -i '' 's/if req.RevokedBy == 0 {/if req.RevokedBy == "" {/g' internal/application/service/team_token_service.go

echo "✓ team_token_service.go 修复完成"

echo ""
echo "========================================="
echo "批量修复完成!"
echo "========================================="
echo ""
echo "请运行以下命令验证:"
echo "  cd backend && go build"
echo ""
echo "如果还有错误,请手动修复剩余的文件"
echo ""
