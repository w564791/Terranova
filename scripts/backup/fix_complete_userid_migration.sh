#!/bin/bash

set -e

echo "========================================="
echo "完整修复 User ID 类型迁移"
echo "========================================="

# 1. 修复 workspace_resource.go 模型
echo "1. 修复 workspace_resource.go 模型..."
perl -i -pe 's/CreatedBy\s+\*uint/CreatedBy        *string/g' backend/internal/models/workspace_resource.go
perl -i -pe 's/UpdatedBy\s+\*uint/UpdatedBy        *string/g' backend/internal/models/workspace_resource.go
perl -i -pe 's/DeletedBy\s+\*uint/DeletedBy        *string/g' backend/internal/models/workspace_resource.go
echo " workspace_resource.go 已修复"

# 2. 修复 role_handler.go
echo "2. 修复 role_handler.go..."
# 读取文件并修复
perl -i -pe '
    # 修复第一处 assignedByStr 声明
    s/(assignedByInterface, _ := c\.Get\("user_id"\)\n\t)userIDStr := fmt\.Sprintf/\1assignedByStr := fmt.Sprintf/;
    # 删除重复的 userIDStr 声明
    next if /^\s*userIDStr := fmt\.Sprintf.*assignedByInterface/;
' backend/internal/handlers/role_handler.go
echo " role_handler.go 已修复"

echo ""
echo "========================================="
echo " 所有修复完成!"
echo "========================================="
echo ""
echo "现在测试编译..."
cd backend && go build -o /dev/null ./...
