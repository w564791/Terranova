#!/bin/bash

set -e

echo "修复 role_handler.go 中的 team ID 解析..."

# 备份文件
cp backend/internal/handlers/role_handler.go backend/internal/handlers/role_handler.go.bak2

# 修复所有 teamID ParseUint 调用
# 将 teamIDStr := c.Param("id") 后面的 ParseUint 逻辑删除,直接使用 teamIDStr
perl -i -0777 -pe 's/teamIDStr := c\.Param\("id"\)\s*teamID, err := strconv\.ParseUint\(teamIDStr, 10, 32\)\s*if err != nil \{\s*c\.JSON\(http\.StatusBadRequest, gin\.H\{\s*"code":\s*400,\s*"message":\s*"Invalid team ID",\s*"timestamp":\s*time\.Now\(\),\s*\}\)\s*return\s*\}/teamIDStr := c.Param("id")\n\tteamID := teamIDStr/g' backend/internal/handlers/role_handler.go

# 将所有 uint(teamID) 改为 teamID
perl -i -pe 's/uint\(teamID\)/teamID/g' backend/internal/handlers/role_handler.go

echo " role_handler.go 中的 team ID 解析已修复"
echo "备份文件: backend/internal/handlers/role_handler.go.bak2"
