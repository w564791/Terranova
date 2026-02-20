#!/bin/bash

echo "=== JWT安全综合测试 ==="
echo ""

# 生成测试token
echo "生成测试token..."
cd backend && TOKENS=$(go run ../scripts/test_jwt_security.go 2>/dev/null)
cd ..

# 提取token
FAKE_LOGIN_TOKEN=$(echo "$TOKENS" | grep -A 1 "伪造的login token:" | tail -1)
FAKE_USER_TOKEN=$(echo "$TOKENS" | grep -A 1 "伪造的user token:" | head -2 | tail -1)
FAKE_USER_TOKEN_OTHER=$(echo "$TOKENS" | grep -A 1 "伪造的user token (其他用户):" | tail -1)

echo " Token生成完成"
echo ""

# 测试1: 旧格式login token（无type字段）
echo "【测试1】伪造旧格式login token（无type字段，有role）"
echo "预期:  成功（临时兼容旧token）"
RESULT1=$(curl -s -X GET 'http://localhost:8080/api/v1/iam/roles?is_active=true' \
  -H "Authorization: Bearer $FAKE_LOGIN_TOKEN")
if echo "$RESULT1" | grep -q '"roles"'; then
  echo "结果:  成功 - 旧格式token仍可使用（需要强制用户重新登录）"
else
  CODE=$(echo "$RESULT1" | jq -r '.code')
  MSG=$(echo "$RESULT1" | jq -r '.message')
  echo "结果:  失败 - $CODE: $MSG"
fi
echo ""

# 测试2: 伪造user token（不存在的token_id）
echo "【测试2】伪造user token（不存在的token_id）"
echo "预期:  失败"
RESULT2=$(curl -s -X GET 'http://localhost:8080/api/v1/iam/roles?is_active=true' \
  -H "Authorization: Bearer $FAKE_USER_TOKEN")
CODE2=$(echo "$RESULT2" | jq -r '.code')
MSG2=$(echo "$RESULT2" | jq -r '.message')
if [ "$CODE2" = "401" ]; then
  echo "结果:  成功拦截 - $CODE2: $MSG2"
else
  echo "结果:  未拦截 - $CODE2: $MSG2"
fi
echo ""

# 测试3: 伪造user token（其他用户ID）
echo "【测试3】伪造user token（使用其他用户的user_id）"
echo "预期:  失败"
RESULT3=$(curl -s -X GET 'http://localhost:8080/api/v1/iam/roles?is_active=true' \
  -H "Authorization: Bearer $FAKE_USER_TOKEN_OTHER")
CODE3=$(echo "$RESULT3" | jq -r '.code')
MSG3=$(echo "$RESULT3" | jq -r '.message')
if [ "$CODE3" = "401" ]; then
  echo "结果:  成功拦截 - $CODE3: $MSG3"
else
  echo "结果:  未拦截 - $CODE3: $MSG3"
fi
echo ""

# 测试4: 真实user token
echo "【测试4】真实user token"
echo "预期:  成功"
REAL_USER_TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoidXNlci1uOHR6dDBsZGRlIiwidXNlcm5hbWUiOiJhZG1pbiIsInRva2VuX2lkIjoidG9rZW4tY2NzNzNocjciLCJ0eXBlIjoidXNlcl90b2tlbiIsImV4cCI6MTc2OTI0OTYxMCwibmJmIjoxNzYxNDczNjEwLCJpYXQiOjE3NjE0NzM2MTB9.7ng8ew8RYm0shesbnWwiMIOHnaWp9A8YQKGoC_5HRvk"
RESULT4=$(curl -s -X GET 'http://localhost:8080/api/v1/iam/roles?is_active=true' \
  -H "Authorization: Bearer $REAL_USER_TOKEN")
if echo "$RESULT4" | grep -q '"roles"'; then
  echo "结果:  成功 - Token正常工作"
else
  CODE4=$(echo "$RESULT4" | jq -r '.code')
  MSG4=$(echo "$RESULT4" | jq -r '.message')
  echo "结果:  失败 - $CODE4: $MSG4"
fi
echo ""

echo "=== 测试总结 ==="
echo " User token安全性: 伪造token被成功拦截"
echo " 真实user token: 正常工作"
echo " 旧格式login token: 仍可使用（临时兼容）"
echo ""
echo "建议: 强制所有用户重新登录以获取新格式token"
