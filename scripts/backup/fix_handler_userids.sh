#!/bin/bash

# 修复 handler 文件中的 userID 类型转换

# 修复 audit_handler.go
perl -i -pe 's/req\.UserID = uint\(userID\)/req.UserID = userID.(string)/g' backend/internal/handlers/audit_handler.go
perl -i -pe 's/PerformerID: uint\(performerID\)/PerformerID: performerID.(string)/g' backend/internal/handlers/audit_handler.go

# 修复 role_handler.go  
perl -i -pe 's/UserID:\s+uint\(userID\)/UserID:     userID.(string)/g' backend/internal/handlers/role_handler.go
perl -i -pe 's/AssignedBy: &assignedBy/AssignedBy: &userIDStr/g' backend/internal/handlers/role_handler.go

echo " 已修复 handler 文件中的 userID 类型转换"
