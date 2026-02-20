#!/bin/bash

set -e

echo "修复最后的4个编译错误..."

# 1. 修复 terraform_executor.go - lockWorkspace 函数签名
echo "1. 修复 terraform_executor.go..."
perl -i -pe 's/func \(s \*TerraformExecutor\) lockWorkspace\(\s*workspaceID uint,\s*userID uint,/func (s *TerraformExecutor) lockWorkspace(\n\tworkspaceID uint,\n\tuserID string,/g' backend/services/terraform_executor.go
echo " terraform_executor.go 已修复"

# 2. 修复 workspace_lifecycle.go - userID 参数类型
echo "2. 修复 workspace_lifecycle.go..."
perl -i -pe 's/userID uint\)/userID string)/g' backend/services/workspace_lifecycle.go
echo " workspace_lifecycle.go 已修复"

echo ""
echo " 所有错误已修复!"
echo ""
echo "测试编译..."
cd backend && go build -o /dev/null ./...
