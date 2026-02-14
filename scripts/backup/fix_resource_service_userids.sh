#!/bin/bash

# 修复 resource_service.go 中的 userID 类型
# 将 userID uint 改为 userID string

FILE="backend/services/resource_service.go"

# 备份原文件
cp "$FILE" "$FILE.bak"

# 使用 perl 进行替换,因为它支持多行匹配
perl -i -pe 's/userID uint,/userID string,/g' "$FILE"
perl -i -pe 's/userID uint\)/userID string)/g' "$FILE"

echo " 已修复 resource_service.go 中的 userID 类型"
echo "原文件备份为: $FILE.bak"
