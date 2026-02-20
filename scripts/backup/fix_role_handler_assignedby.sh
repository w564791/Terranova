#!/bin/bash

# 修复 role_handler.go 中的 assignedBy 问题

# 将 assignedBy := assignedByInterface.(uint) 改为 string 类型转换
perl -i -pe 's/assignedBy := assignedByInterface\.\(uint\)/userIDStr := fmt.Sprintf("%d", assignedByInterface.(uint64))/g' backend/internal/handlers/role_handler.go

echo " 已修复 role_handler.go 中的 assignedBy 问题"
