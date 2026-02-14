#!/bin/bash

# Workspace Variables 版本控制功能测试脚本

# 配置
API_BASE="http://localhost:8080/api/v1"
WORKSPACE_ID="ws-0yrm628p8h3f9mw0"  # 替换为实际的workspace_id
TOKEN="your_jwt_token_here"          # 替换为实际的JWT token

echo "========================================="
echo "Workspace Variables 版本控制测试"
echo "========================================="
echo ""

# 测试1: 创建变量
echo "测试1: 创建变量"
echo "-------------------"
CREATE_RESPONSE=$(curl -s -X POST "${API_BASE}/workspaces/${WORKSPACE_ID}/variables" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "key": "TEST_VERSION_VAR",
    "value": "version_1",
    "variable_type": "terraform",
    "sensitive": false,
    "description": "测试版本控制"
  }')

echo "响应: ${CREATE_RESPONSE}"
VARIABLE_ID=$(echo ${CREATE_RESPONSE} | jq -r '.data.variable_id')
VERSION=$(echo ${CREATE_RESPONSE} | jq -r '.data.version')
echo "创建的变量ID: ${VARIABLE_ID}"
echo "初始版本: ${VERSION}"
echo ""

# 测试2: 更新变量（版本号+1）
echo "测试2: 更新变量（版本号应该从1变为2）"
echo "-------------------"
UPDATE_RESPONSE=$(curl -s -X PUT "${API_BASE}/workspaces/${WORKSPACE_ID}/variables/${VARIABLE_ID}" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{
    \"version\": ${VERSION},
    \"value\": \"version_2\",
    \"description\": \"第二个版本\"
  }")

echo "响应: ${UPDATE_RESPONSE}"
NEW_VERSION=$(echo ${UPDATE_RESPONSE} | jq -r '.version_info.new_version')
OLD_VERSION=$(echo ${UPDATE_RESPONSE} | jq -r '.version_info.old_version')
echo "版本变更: ${OLD_VERSION} -> ${NEW_VERSION}"
echo ""

# 测试3: 版本冲突测试（使用旧版本号）
echo "测试3: 版本冲突测试（使用旧版本号，应该返回409）"
echo "-------------------"
CONFLICT_RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" -X PUT "${API_BASE}/workspaces/${WORKSPACE_ID}/variables/${VARIABLE_ID}" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "version": 1,
    "value": "should_fail"
  }')

echo "响应: ${CONFLICT_RESPONSE}"
HTTP_CODE=$(echo "${CONFLICT_RESPONSE}" | grep "HTTP_CODE" | cut -d':' -f2)
if [ "${HTTP_CODE}" = "409" ]; then
  echo " 版本冲突检测正常（返回409）"
else
  echo "❌ 版本冲突检测失败（应该返回409，实际返回${HTTP_CODE}）"
fi
echo ""

# 测试4: 查询版本历史
echo "测试4: 查询版本历史"
echo "-------------------"
VERSIONS_RESPONSE=$(curl -s "${API_BASE}/workspaces/${WORKSPACE_ID}/variables/${VARIABLE_ID}/versions" \
  -H "Authorization: Bearer ${TOKEN}")

echo "响应: ${VERSIONS_RESPONSE}"
TOTAL_VERSIONS=$(echo ${VERSIONS_RESPONSE} | jq -r '.total')
echo "总版本数: ${TOTAL_VERSIONS}"
echo ""

# 测试5: 查询指定版本
echo "测试5: 查询指定版本（version 1）"
echo "-------------------"
VERSION_1_RESPONSE=$(curl -s "${API_BASE}/workspaces/${WORKSPACE_ID}/variables/${VARIABLE_ID}/versions/1" \
  -H "Authorization: Bearer ${TOKEN}")

echo "响应: ${VERSION_1_RESPONSE}"
V1_VALUE=$(echo ${VERSION_1_RESPONSE} | jq -r '.data.value')
echo "Version 1 的值: ${V1_VALUE}"
echo ""

# 测试6: 删除变量（软删除）
echo "测试6: 删除变量（软删除，应该创建删除版本）"
echo "-------------------"
DELETE_RESPONSE=$(curl -s -X DELETE "${API_BASE}/workspaces/${WORKSPACE_ID}/variables/${VARIABLE_ID}" \
  -H "Authorization: Bearer ${TOKEN}")

echo "响应: ${DELETE_RESPONSE}"
echo ""

# 测试7: 验证删除后的版本历史
echo "测试7: 验证删除后的版本历史"
echo "-------------------"
FINAL_VERSIONS=$(curl -s "${API_BASE}/workspaces/${WORKSPACE_ID}/variables/${VARIABLE_ID}/versions" \
  -H "Authorization: Bearer ${TOKEN}")

echo "响应: ${FINAL_VERSIONS}"
FINAL_TOTAL=$(echo ${FINAL_VERSIONS} | jq -r '.total')
echo "删除后总版本数: ${FINAL_TOTAL}"
echo "最新版本应该标记为 is_deleted=true"
echo ""

# 测试8: 验证变量列表不包含已删除变量
echo "测试8: 验证变量列表不包含已删除变量"
echo "-------------------"
LIST_RESPONSE=$(curl -s "${API_BASE}/workspaces/${WORKSPACE_ID}/variables" \
  -H "Authorization: Bearer ${TOKEN}")

echo "变量列表中是否包含 TEST_VERSION_VAR:"
echo ${LIST_RESPONSE} | jq '.data[] | select(.key=="TEST_VERSION_VAR")'
echo "（应该为空，因为已被软删除）"
echo ""

echo "========================================="
echo "测试完成"
echo "========================================="
echo ""
echo "总结:"
echo "- 创建变量: 生成 variable_id 和 version 1"
echo "- 更新变量: 版本号自动+1，返回版本变更信息"
echo "- 版本冲突: 使用旧版本号更新时返回 409"
echo "- 版本历史: 可以查询所有历史版本"
echo "- 软删除: 删除操作创建删除版本"
echo "- 列表查询: 不显示已删除的变量"
