#!/bin/bash

set -e

echo "开始修复所有 userID 类型问题..."

# 1. 恢复备份(如果存在)
if [ -f "backend/services/resource_service.go.bak" ]; then
    cp backend/services/resource_service.go.bak backend/services/resource_service.go
    echo " 已恢复 resource_service.go 备份"
fi

# 2. 修复 resource_service.go - 将所有 userID uint 改为 userID string
perl -i -pe 's/userID uint,/userID string,/g' backend/services/resource_service.go
perl -i -pe 's/userID uint\)/userID string)/g' backend/services/resource_service.go
echo " 已修复 resource_service.go"

# 3. 修复 audit_handler.go
# 添加 fmt 导入
if ! grep -q '"fmt"' backend/internal/handlers/audit_handler.go; then
    perl -i -pe 's/(import \()/\1\n\t"fmt"/' backend/internal/handlers/audit_handler.go
fi
# 修复类型转换
perl -i -pe 's/req\.UserID = userID\.\(string\)/req.UserID = fmt.Sprintf("%d", userID)/g' backend/internal/handlers/audit_handler.go
perl -i -pe 's/PerformerID: performerID\.\(string\)/PerformerID: fmt.Sprintf("%d", performerID)/g' backend/internal/handlers/audit_handler.go
echo " 已修复 audit_handler.go"

# 4. 修复 role_handler.go
# 添加 fmt 导入
if ! grep -q '"fmt"' backend/internal/handlers/role_handler.go; then
    perl -i -pe 's/(import \()/\1\n\t"fmt"/' backend/internal/handlers/role_handler.go
fi
# 修复 UserID 转换
perl -i -pe 's/UserID:\s+userID\.\(string\)/UserID:     fmt.Sprintf("%d", userID)/g' backend/internal/handlers/role_handler.go
# 修复 assignedBy 变量
perl -i -pe 's/assignedBy := assignedByInterface\.\(uint\)/assignedByStr := fmt.Sprintf("%d", assignedByInterface.(uint64))/' backend/internal/handlers/role_handler.go
perl -i -pe 's/AssignedBy: &userIDStr/AssignedBy: \&assignedByStr/g' backend/internal/handlers/role_handler.go
echo " 已修复 role_handler.go"

echo ""
echo " 所有修复完成！"
echo ""
echo "现在测试编译..."
cd backend && go build -o /dev/null ./...
