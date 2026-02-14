#!/bin/bash

set -e

echo "批量修复所有剩余的 uint 类型断言..."

# 1. 搜索并修复所有 userID.(uint) 断言
find backend -name "*.go" -type f -exec sed -i '' 's/userID\.\(uint\)/userID.(string)/g' {} \;

# 2. 搜索并修复所有 uid.(uint) 断言
find backend -name "*.go" -type f -exec sed -i '' 's/uid\.\(uint\)/uid.(string)/g' {} \;

# 3. 搜索并修复所有方法签名中的 userID uint 参数
find backend -name "*.go" -type f -exec sed -i '' 's/userID uint,/userID string,/g' {} \;
find backend -name "*.go" -type f -exec sed -i '' 's/userID uint)/userID string)/g' {} \;

echo " 所有修复完成!"
echo ""
echo "重新编译..."
cd backend && go build -o /dev/null ./...
