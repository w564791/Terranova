#!/bin/bash

set -e

echo "精确修复剩余问题..."

# 1. 修复 role_handler.go 中的变量名不一致问题
# 将第二个 userIDStr := 改为 assignedByStr :=
perl -i -pe '
    if (/assignedByInterface, _ := c\.Get\("user_id"\)/) {
        $_ .= <>;  # 读取下一行
        s/userIDStr := fmt\.Sprintf/assignedByStr := fmt.Sprintf/;
    }
' backend/internal/handlers/role_handler.go

# 2. 删除未使用的 userIDStr 变量声明(第894行附近)
perl -i -ne 'print unless /^\s*userIDStr := fmt\.Sprintf.*assignedByInterface/' backend/internal/handlers/role_handler.go

# 3. 修复 resource_service.go 中的 CreatedBy/UpdatedBy/DeletedBy
# 这些字段在 WorkspaceResource 中是 *uint 类型,需要保持不变
# 但我们传入的 userID 现在是 string,需要转换

echo " 修复完成"
echo ""
echo "注意: resource_service.go 中的 CreatedBy/UpdatedBy/DeletedBy 字段"
echo "在 WorkspaceResource 模型中仍然是 *uint 类型"
echo "需要检查 WorkspaceResource 模型定义"
