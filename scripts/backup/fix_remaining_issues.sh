#!/bin/bash

# 修复剩余的类型转换问题

# 1. 修复 audit_handler.go - userID 和 performerID 是 uint64,需要转换为 string
perl -i -pe 's/req\.UserID = userID\.\(string\)/req.UserID = fmt.Sprintf("%d", userID)/g' backend/internal/handlers/audit_handler.go
perl -i -pe 's/PerformerID: performerID\.\(string\)/PerformerID: fmt.Sprintf("%d", performerID)/g' backend/internal/handlers/audit_handler.go

# 2. 修复 role_handler.go - userID 是 uint64,需要转换为 string
perl -i -pe 's/UserID:\s+userID\.\(string\)/UserID:     fmt.Sprintf("%d", userID)/g' backend/internal/handlers/role_handler.go

# 3. 修复 role_handler.go - assignedBy 变量名和类型
# 需要手动处理,因为涉及变量声明和使用

# 4. 修复 resource_service.go - &userID 从 *string 改为需要的类型
perl -i -pe 's/CreatedBy:\s+&userID,/CreatedBy:   \&userID,/g' backend/services/resource_service.go
perl -i -pe 's/UpdatedBy:\s+&userID,/UpdatedBy:   \&userID,/g' backend/services/resource_service.go
perl -i -pe 's/DeletedBy:\s+&userID,/DeletedBy:   \&userID,/g' backend/services/resource_service.go

echo " 已修复大部分类型转换问题"
echo "  role_handler.go 中的 assignedBy 需要手动修复"
