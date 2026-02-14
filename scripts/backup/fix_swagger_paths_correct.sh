#!/bin/bash

# 修复Swagger路径问题 - 正确版本
# 问题：handler的Swagger注解缺少 /v1 前缀
# 实际路由：/api/v1/... (router.go中定义 api := r.Group("/api/v1"))
# Swagger注解：/api/... (缺少v1)
# 解决：将 @Router /api/ 替换为 @Router /api/v1/

echo "开始修复Swagger路径问题..."

# 需要修复的文件 - 只修复那些路径缺少 /v1 的
files=(
    "backend/internal/handlers/role_handler.go"
)

# 备份并修复每个文件
for file in "${files[@]}"; do
    if [ -f "$file" ]; then
        echo "处理文件: $file"
        # 创建备份
        cp "$file" "${file}.bak_correct"
        # 替换 @Router /api/iam/ 为 @Router /api/v1/iam/
        sed -i '' 's|@Router /api/iam/|@Router /api/v1/iam/|g' "$file"
        echo "  ✓ 已修复"
    else
        echo "  ✗ 文件不存在: $file"
    fi
done

echo ""
echo "修复完成！"
echo ""
echo "修复的路径："
echo "  /api/iam/roles -> /api/v1/iam/roles"
echo ""
echo "下一步："
echo "1. 检查修复后的文件"
echo "2. 重新生成Swagger文档: cd backend && swag init"
echo "3. 重启后端服务"
echo "4. 测试: curl -X 'GET' 'http://localhost:8080/api/v1/iam/roles?is_active=true' -H 'Authorization: Bearer YOUR_TOKEN'"
