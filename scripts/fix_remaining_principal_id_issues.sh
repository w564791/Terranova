#!/bin/bash

set -e

echo "修复剩余的 principal_id 类型问题..."

# 1. 修复 audit_repository.go 接口
sed -i '' 's/principalID uint,$/principalID string,/g' backend/internal/domain/repository/audit_repository.go

# 2. 修复 audit_repository_impl.go 实现
sed -i '' 's/principalID uint,$/principalID string,/g' backend/internal/infrastructure/persistence/audit_repository_impl.go

# 3. 修复 permission_repository.go 接口中的 principalIDs []uint
sed -i '' 's/principalIDs \[\]uint,$/principalIDs []string,/g' backend/internal/domain/repository/permission_repository.go

# 4. 修复 permission_repository_impl.go 实现中的 principalIDs []uint
sed -i '' 's/principalIDs \[\]uint,$/principalIDs []string,/g' backend/internal/infrastructure/persistence/permission_repository_impl.go

# 5. 修复 permission_service.go 第390行的类型断言
sed -i '' 's/PrincipalID:   principalID,$/PrincipalID:   principalID.(string),/g' backend/internal/application/service/permission_service.go

echo " 所有修复完成!"
echo ""
echo "现在重新编译..."
cd backend && go build -o /dev/null ./...
