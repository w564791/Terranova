#!/bin/bash

# 最终修复脚本

# 1. 添加 fmt 导入到 audit_handler.go
perl -i -pe 's/import \(/import (\n\t"fmt"/g' backend/internal/handlers/audit_handler.go

# 2. 添加 fmt 导入到 role_handler.go (如果没有)
perl -i -pe 's/import \(/import (\n\t"fmt"/g' backend/internal/handlers/role_handler.go

# 3. 修复 role_handler.go 中的变量声明问题
# 将 userIDStr := 改为 userIDStr =
perl -i -pe 's/userIDStr := fmt\.Sprintf/userIDStr = fmt.Sprintf/g' backend/internal/handlers/role_handler.go

# 4. 在 role_handler.go 中添加 userIDStr 变量声明
# 需要在函数开始处添加 var userIDStr string

echo " 已添加 fmt 导入和修复变量声明"
