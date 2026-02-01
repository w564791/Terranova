#!/bin/bash

# 测试Schema API返回的数据
echo "=== 测试Schema API ==="
echo ""

# 获取token（假设已登录）
TOKEN=$(cat ~/.iac-platform-token 2>/dev/null || echo "your-token-here")

# 测试获取某个模块的schemas
MODULE_ID=1
echo "1. 获取Module $MODULE_ID 的Schemas:"
curl -s -X GET "http://localhost:8080/api/v1/modules/$MODULE_ID/schemas" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" | jq '.'

echo ""
echo "=== 检查source_type字段 ==="
curl -s -X GET "http://localhost:8080/api/v1/modules/$MODULE_ID/schemas" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" | jq '.[].source_type'
